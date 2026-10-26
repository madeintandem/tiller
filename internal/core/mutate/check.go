package mutate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// This file holds the check mutation: run a node's required command-backed gates,
// record the schema's gates block onto the node, and on green advance a task to
// the lifecycle check state (cli-surface.md, "Gates, evals, and attestation"; the
// done crossing; artifact-schema.md, "Gates"). It is the sibling of Transition: it
// runs the same structural-blocker guard, writes the same way (atomic file plus
// one event under the repository mutation lock), and reuses the lifecycle engine,
// while adding the gate run and the recorded block.
//
// The gates are run through internal/core/gates.RunGate (the pure runner) and the
// `against` revision comes from internal/vcs (read-only git). An unsatisfied
// required gate is a recorded result with Passed=false, not a Go error. The CLI
// renders it as a blocked business answer (exit 1).
// Go errors cover the guard refusal (BlockedByQuestionsError) and task legality
// (lifecycle.IllegalTransitionError), both blocked answers the CLI already routes,
// plus the shared tool failures (bad owner, git unavailable, a bad write).
//
// CheckInput carries the optional guard inputs check shares with start and submit:
// an --assume-question recorded with its --reason lets a known-risk check proceed
// past an open-question blocker (cli-surface.md, the active-basis guard). Progress
// is the optional gate lifecycle sink: nil runs silent, exactly as
// check did before progress existed; the CLI supplies a stderr-writing one in
// human mode and nil under --json, so the recorded evidence never carries it.
//
// Rerun is `check --rerun`: every command gate runs for real this invocation,
// even one whose recorded pass could be reused, for a suspected flake or an
// environment change. Gates is `check --gate <name>`, the gates this invocation
// may run.
type CheckInput struct {
	AssumeQuestion string
	Reason         string
	Progress       gates.Progress
	Rerun          bool
	Gates          []string
}

// GateOutcome is one recorded gate result the CLI renders back to the caller: the
// gate name, whether it is required, its kind, the resulting status, the revision
// it ran against (the compact value token), and the evidence path. Criteria
// carries required per-criterion statuses behind a derived evals gate (id to
// status), so a blocked check can name outstanding acceptance without another
// eval. It is empty for other gates and stays out of the JSON envelope.
//
// Reused reports that this check stood a recorded pass in for a fresh run
// (design 2026-10-21-gate-reuse): ReusedRunAt is when that pass actually ran,
// and ReusedFrom names the node it came from when that is not this node.
// NotRun reports a gate this invocation was not allowed to run and could not
// reuse (`check --gate`).
type GateOutcome struct {
	Name        string
	Required    bool
	Kind        string
	Status      string
	Against     string
	EvidenceRef string
	Criteria    map[string]string
	Reused      bool
	ReusedFrom  string
	ReusedRunAt time.Time
	NotRun      bool
}

// CheckResult is the outcome of a check: the resulting node state, the gate
// outcomes in name order, whether every required gate is green (Passed), and
// whether the node file changed. Warnings carries advisory lines the check
// still succeeded under, such as shelf work no command gate applies to.
type CheckResult struct {
	Noun     string
	ID       string
	Key      string
	Status   string
	Gates    []GateOutcome
	Passed   bool
	Changed  bool
	Warnings []string
	// AggregateBranch is the recorded aggregate branch of a coarse-grain
	// feature check, where the human's attest must be committed.
	AggregateBranch string
}

// Check runs a node's required command-backed gates, records the gates block, and
// on green advances a task to the lifecycle check state. It resolves the acting
// owner first (the exit-3 refusal every mutation shares), then dispatches on the
// node type. The PR grains carry gates: a feature, a task, or a shelf node (a bug
// or chore, running the task-grade gates); any other noun is a clear misuse. rev
// supplies the `against` revision and is the one git seam, stubbed in tests.
func Check(tree *store.Tree, deps Deps, rev vcs.Revisions, node store.Node, in CheckInput) (CheckResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return CheckResult{}, err
	}
	return dispatchCheck(tree, deps, owner, rev, node, in)
}

// dispatchCheck runs the gate evaluation on the concrete node grain. The PR
// grains carry gates: a feature, a task, or a shelf node (a bug or chore); any
// other noun is a clear misuse. rev supplies the `against` revision and is the
// one git seam.
func dispatchCheck(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, node store.Node, in CheckInput) (CheckResult, error) {
	switch n := node.(type) {
	case *store.Feature:
		return checkFeature(tree, deps, owner, rev, n, in)
	case *store.Task:
		return checkTask(tree, deps, owner, rev, n, in)
	default:
		if w, ok := store.AsWorkNode(node); ok {
			return checkWorkItem(tree, deps, owner, rev, w, in)
		}
		return CheckResult{}, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("check does not run gates on a %s", node.Noun()),
		}
	}
}

// checkFeature records the feature-grain gates onto feature.yml. A feature has no
// `check` lifecycle transition (check records, it does not move a feature), so the
// state is unchanged; the structural-blocker guard still runs first.
func checkFeature(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, treeFeat *store.Feature, in CheckInput) (CheckResult, error) {
	if treeFeat.Key == nil {
		return CheckResult{}, fmt.Errorf("mutate: feature %s has no canonical key", treeFeat.ID)
	}
	relDir := featureRelDir(treeFeat)
	ymlRel := filepath.Join(relDir, "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return CheckResult{}, err
	}
	doc.EpicKey = treeFeat.EpicKey
	state := doc.Status

	if _, err := guardFeature(tree, treeFeat, &doc, TransitionInput{
		Verb: "check", AssumeQuestion: in.AssumeQuestion, Reason: in.Reason,
	}, rev); err != nil {
		return CheckResult{}, err
	}

	against, err := gateAgainst(rev)
	if err != nil {
		return CheckResult{}, err
	}
	ef, err := scopedEvalFile(tree, store.NounFeature, filepath.Join(relDir, "feature.eval.md"))
	if err != nil {
		return CheckResult{}, err
	}
	run, err := newGateRun(rev, against, owner, deps, in, &doc.GatesBlock)
	if err != nil {
		return CheckResult{}, err
	}
	// A feature may also reuse a riding task's pass over the same code.
	run.sources = append(run.sources, ridingSources(tree, treeFeat.ID)...)
	block, outcomes, passed, err := evaluateGates(tree, store.NounFeature, treeFeat.ID, ef, &doc.GatesBlock, run)
	if err != nil {
		return CheckResult{}, err
	}
	mergeGates(&doc.GatesBlock, block, tree.Manifest, store.NounFeature)

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return CheckResult{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "check", ReusedGates: reusedGates(outcomes, block, run.at)},
		store.NounFeature, doc.ID, ymlRel, ydata, snapshot, state, state, ""); err != nil {
		return CheckResult{}, err
	}

	aggregate, _, _ := aggregateForTask(tree, treeFeat)
	return CheckResult{
		Noun: store.NounFeature, ID: doc.ID, Key: deref(doc.Key),
		Status: state, Gates: outcomes, Passed: passed, Changed: true, AggregateBranch: aggregate,
	}, nil
}

// checkTask records the task-grain gates onto the task's own task.yml and, on
// green, advances the task in_progress -> check via the lifecycle engine. The task
// must be in a state check is legal from; a task in the wrong state is refused with
// the engine's typed illegal-transition error before any gate runs, so a gates
// block is never recorded on a task that cannot move.
func checkTask(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, treeTask *store.Task, in CheckInput) (CheckResult, error) {
	feat := tree.Features[treeTask.FeatureID]
	if feat == nil || feat.Key == nil {
		return CheckResult{}, fmt.Errorf("mutate: task %s has no resolvable parent feature", treeTask.ID)
	}
	relDir := featureRelDir(feat)
	target, ymlRel, snapshot, err := loadTaskFile(tree, feat, treeTask)
	if err != nil {
		return CheckResult{}, err
	}
	before := target.Status

	if legal, _ := lifecycle.Legal(target, "check"); !legal {
		return CheckResult{}, &lifecycle.IllegalTransitionError{
			Noun:  store.NounTask,
			From:  before,
			Verb:  "check",
			Legal: lifecycle.NextMoves(target),
		}
	}

	if err := guardTask(tree, feat, target, TransitionInput{
		Verb: "check", AssumeQuestion: in.AssumeQuestion, Reason: in.Reason,
	}, rev); err != nil {
		return CheckResult{}, err
	}

	against, err := gateAgainst(rev)
	if err != nil {
		return CheckResult{}, err
	}
	ef, err := scopedEvalFile(tree, store.NounTask, filepath.Join(relDir, "feature.eval.md"))
	if err != nil {
		return CheckResult{}, err
	}
	run, err := newGateRun(rev, against, owner, deps, in, &target.GatesBlock)
	if err != nil {
		return CheckResult{}, err
	}
	block, outcomes, passed, err := evaluateGates(tree, store.NounTask, target.ID, ef, &target.GatesBlock, run)
	if err != nil {
		return CheckResult{}, err
	}
	mergeGates(&target.GatesBlock, block, tree.Manifest, store.NounTask)

	after := before
	if passed {
		if err := lifecycle.Apply(target, "check", lifecycle.Payload{}); err != nil {
			return CheckResult{}, err
		}
		after = target.Status
	}

	ydata, err := yaml.Marshal(target)
	if err != nil {
		return CheckResult{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "check", ReusedGates: reusedGates(outcomes, block, run.at)},
		store.NounTask, target.ID, ymlRel, ydata, snapshot, before, after, ""); err != nil {
		return CheckResult{}, err
	}

	return CheckResult{
		Noun: store.NounTask, ID: target.ID, Key: deref(target.Key),
		Status: after, Gates: outcomes, Passed: passed, Changed: true,
	}, nil
}

// gateAgainst chooses the revision a gate run is honestly pinned to. Gates run
// over the WORKING TREE, so when that tree is dirty (the code under test is
// uncommitted, the usual case for check and eval) the provenance is a sha256
// working-tree fingerprint; HEAD does not contain the tested code, and recording
// it would claim a gate ran over code the commit does not hold. Only when the
// tree is clean does HEAD contain exactly what ran, in which case the HEAD
// git_commit is recorded as before (cli-surface.md, "Gates, evals, and
// attestation"; artifact-schema.md, The stored gate block).
func gateAgainst(rev vcs.Revisions) (store.RevisionRef, error) {
	dirty, err := rev.IsDirty()
	if err != nil {
		return store.RevisionRef{}, err
	}
	if dirty {
		return rev.WorktreeFingerprint()
	}
	return rev.CurrentCommit()
}

// evaluateGates resolves the required gates that apply to this node grain, runs
// each command-backed one, and builds both the stored gates block and the CLI
// outcomes. Every required gate must pass; pending acceptance is recorded but
// does not pass the check or advance task/shelf work. A required attestation
// gate is never run: its settled verdict in prior (settledAttestation) is
// carried forward unchanged, and it is recorded pending only when no
// compatible verdict exists.
//
// The evals gate is special and is NOT run as a bare command: its recorded result
// is the per-criterion `criteria` map plus a status DERIVED from the criteria,
// evaluated exactly as `tiller eval` does, preserving any human-settled criterion
// status carried in prior. This is what keeps `tiller check` from clobbering an
// attested evals result (artifact-schema.md, The eval model). It applies when the
// manifest's evals gate names this grain in its `on` list; the caller passes the
// contract through scopedEvalFile, which returns nil otherwise, and every grain
// now routes through that one scoping rule.
//
// A command gate whose recorded pass in run.sources matches the current code
// identity and its resolved command's config hash is reused rather than run
// (reuseCommandGate), unless run.rerun forces a real run or the manifest opts
// the gate out with `reuse: false` (design 2026-10-21-gate-reuse). A fresh run
// records the code identity and config hash it ran under, so a later check can
// reuse it. The evals gate's command and test criteria follow the same rule
// inside gates.EvaluateCriteria: --rerun or the evals gate's `reuse: false` turns
// it off for every criterion, and the gate record carries the run's code identity.
//
// run.only is `check --gate`. A name outside the gates this
// grain evaluates is refused (unknown_gate) before anything runs. A runnable
// command gate, or evals, outside the set is held: it is never shelled, and is
// evaluated reuse-only. When the reuse rule above would reuse it (the same
// code identity and config hash, reuse allowed, no --rerun) it is reused and
// stamped like any reuse; otherwise it is reported not run, and a prior pass
// is marked stale and written, so no consumer that reads status alone
// (submit, sync, readout) takes it for a pass over this code (for evals, the
// gate and only the criteria a full check would rerun, so a human's verdict
// survives); a prior fail, pending, or stale is left as it is (notRunOutcome). Attestation and runless
// gates are never shelled, so --gate does not change them. The returned passed is gatesGreen over every
// required gate's resulting record, so a partial run never advances work on a
// held gate's pass over older code.
func evaluateGates(
	tree *store.Tree,
	noun, nodeID string,
	ef *store.EvalFile,
	prior *store.GatesBlock,
	run gateRun,
) (map[string]store.Gate, []GateOutcome, bool, error) {
	against, at := run.against, run.at
	configs := applicableGates(tree.Manifest, noun, nodeID, shortSHA(against))

	block := map[string]store.Gate{}
	outcomes := make([]GateOutcome, 0, len(configs)+1)
	var results []gateResult

	evalsApplies := ef != nil
	evalsRequired := false
	evalsReusable := true
	for _, c := range configs {
		if isEvalRecursion(c) {
			evalsApplies = true
			evalsRequired = c.Required
			evalsReusable = c.Reuse
		}
	}
	if err := validateOnly(run, configs, evalsApplies, noun); err != nil {
		return nil, nil, false, err
	}

	for _, c := range configs {
		if isEvalRecursion(c) {
			// The evals gate is handled below from the eval contract, never shelled.
			continue
		}
		runnable := c.Kind == store.GateKindCommand && strings.TrimSpace(c.Run) != ""

		g := store.Gate{
			Required: c.Required,
			Kind:     c.Kind,
			RunBy:    run.runBy,
			RunAt:    at,
			Against:  against,
		}
		var evidenceRef string
		reused := false

		if settled, ok := settledAttestation(prior, c); ok {
			// A required attestation gate is set by `tiller attest`, never run
			// here: its recorded verdict and provenance stand as they are.
			g = settled
			g.Required = c.Required
		} else if c.Kind != store.GateKindCommand || strings.TrimSpace(c.Run) == "" {
			g.Status = store.GateStatusPending
		} else if hit, ok := reuseCommandGate(c, run); ok {
			g, reused = hit, true
			evidenceRef = commandEvidenceRef(g)
		} else if run.held(c.Name) {
			o, prev, staleIt := notRunOutcome(c.Name, c.Required, c.Kind, prior)
			if staleIt {
				block[c.Name] = prev
			}
			outcomes = append(outcomes, o)
			results = append(results, gateResult{required: c.Required, record: prev, notRun: true, codeBound: true})
			continue
		} else {
			res, err := gates.RunGate(c, tree.Root, run.progress)
			if err != nil {
				return block, outcomes, false, err
			}
			g.Status = res.Status
			g.Code = run.code
			g.Config = gates.ConfigHash(c.Kind, c.Run)
			evidenceRef = res.EvidenceRef
			g.Evidence = []store.Evidence{{Kind: "command_output", Ref: res.EvidenceRef}}
		}

		results = append(results, gateResult{required: c.Required, record: g, codeBound: runnable})
		block[c.Name] = g
		o := GateOutcome{
			Name: c.Name, Required: c.Required, Kind: c.Kind,
			Status: g.Status, Against: g.Against.Value, EvidenceRef: evidenceRef,
		}
		if reused {
			o.Reused, o.ReusedFrom, o.ReusedRunAt = true, g.ReusedFrom, g.RunAt
		}
		outcomes = append(outcomes, o)
	}

	if evalsApplies && run.held("evals") {
		required := evalsRequired || evalsGateRequired(tree.Manifest)
		if g, ok := reuseHeldEvals(prior, ef, tree.Manifest, required, evalsReusable, run); ok {
			block["evals"] = g
			results = append(results, gateResult{required: required, record: g, codeBound: true})
			outcomes = append(outcomes, GateOutcome{
				Name: "evals", Required: required, Kind: store.GateKindCommand,
				Status: g.Status, Against: g.Against.Value, Criteria: criterionStatuses(ef, g.Criteria),
				Reused: true, ReusedRunAt: g.RunAt,
			})
		} else {
			o, prev, staleIt := notRunOutcome("evals", required, store.GateKindCommand, prior)
			if staleIt {
				// Stale only what a full check would rerun: a human's manual or
				// rubric verdict is kept, as EvaluateCriteria keeps it.
				prev.Criteria = gates.StaleRerunCriteria(ef, prev.Criteria, tree.Manifest, gates.CriteriaRun{
					Code:  run.code,
					Reuse: !run.rerun && evalsReusable,
				})
				block["evals"] = prev
			}
			o.Criteria = criterionStatuses(ef, prev.Criteria)
			outcomes = append(outcomes, o)
			results = append(results, gateResult{required: required, record: prev, notRun: true, codeBound: true})
		}
	} else if evalsApplies {
		required := evalsRequired || evalsGateRequired(tree.Manifest)
		criteria, err := gates.EvaluateCriteria(ef, priorCriteria(prior), tree.Manifest, tree.Root, gates.CriteriaRun{
			Against:  shortSHA(against),
			Code:     run.code,
			Reuse:    !run.rerun && evalsReusable,
			At:       at,
			Progress: run.progress,
		})
		if err != nil {
			return block, outcomes, false, err
		}
		status := gates.GateStatusFromCriteria(ef, criteria)
		block["evals"] = store.Gate{
			Required: required,
			Kind:     store.GateKindCommand,
			Status:   status,
			RunBy:    run.runBy,
			RunAt:    at,
			Against:  against,
			Code:     run.code,
			Criteria: criteria,
		}
		results = append(results, gateResult{required: required, record: block["evals"], codeBound: true})
		outcomes = append(outcomes, GateOutcome{
			Name: "evals", Required: required, Kind: store.GateKindCommand,
			Status: status, Against: against.Value, Criteria: criterionStatuses(ef, criteria),
		})
	}

	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Name < outcomes[j].Name })
	return block, outcomes, gatesGreen(results, run.code), nil
}

// settledAttestation returns the node's recorded verdict for an attestation
// gate the manifest still declares as attestation-backed on this grain, so
// check carries a human's pass, fail, or expired (stale) verdict forward
// rather than resetting it to pending. A record of another kind (the gate was
// command-backed when it was written) is never a human verdict and is not
// inherited; the caller records the gate pending instead. Gates the manifest
// no longer scopes to the grain never reach here (applicableGates) and are
// dropped by mergeGates.
func settledAttestation(prior *store.GatesBlock, c gates.GateConfig) (store.Gate, bool) {
	if prior == nil || c.Kind != store.GateKindAttestation {
		return store.Gate{}, false
	}
	g, ok := prior.Gates[c.Name]
	if !ok || g.Kind != store.GateKindAttestation || g.Status == store.GateStatusPending {
		return store.Gate{}, false
	}
	return g, true
}

// mergeGates folds the freshly evaluated gate results into the node's gates map.
// A gate this run touched is updated. A prior record the run did not touch is
// kept only while it is still a legitimate record for this grain: a gate the
// manifest requires stays only while its `on` list includes the noun, so a
// manifest rescope plus re-check drops the stale record and unblocks the node
// (no pinned required: true outlives the config), and a gate the manifest does
// not know stays only when it is attestation-backed (an optional review gate set
// by `tiller attest`). A command-kind record whose manifest config is gone
// follows the manifest and is dropped.
func mergeGates(gb *store.GatesBlock, block map[string]store.Gate, m *store.Manifest, noun string) {
	if gb.Gates == nil {
		gb.Gates = map[string]store.Gate{}
	}
	for name, prior := range gb.Gates {
		if _, fresh := block[name]; fresh {
			continue
		}
		if !gateStillApplies(m, noun, name, prior) {
			delete(gb.Gates, name)
		}
	}
	for name, g := range block {
		gb.Gates[name] = g
	}
}

// gateStillApplies reports whether a prior gate record the current run did not
// re-evaluate is still legitimate for this node grain (see mergeGates).
func gateStillApplies(m *store.Manifest, noun, name string, prior store.Gate) bool {
	if _, known := m.Gates.Required[name]; known {
		for _, c := range requiredGateConfigs(m, noun) {
			if c.Name == name {
				return true
			}
		}
		return false
	}
	return prior.Kind == store.GateKindAttestation
}

// scopedEvalFile loads and validates the eval contract this grain grades. A
// grain outside the manifest's eval scope returns no contract without touching
// disk; a scoped grain distinguishes legal absence from every existing invalid
// contract.
func scopedEvalFile(tree *store.Tree, noun, relPath string) (*store.EvalFile, error) {
	spec, ok := tree.Manifest.Gates.Required["evals"]
	if !ok || !onIncludes(spec.On, gateNoun(noun)) {
		return nil, nil
	}
	return loadEvalContract(tree.Root, relPath)
}

// requiredGateConfigs is the one answer to "which required gates does the
// manifest bind to this node grain": the gates whose `on` list includes the
// grain's gate noun, in name order so every reader walks them the same way.
//
// It is the single home of the shelf-node eval exclusion. A bug or chore runs
// the task-grade gates, so gateNoun reads it as a task and the manifest's
// task-scoped evals gate matches, but a shelf node has no feature.eval.md and
// the `tiller eval` recursion gate is never run on one. Deciding that here
// rather than at each call site is what keeps `check` and `sync` from
// disagreeing: check would report every required gate green while the same
// run's sync recorded a pending evals policy_violation on the node it just
// crossed.
//
// Every question about the required set goes through this: which gates to run
// (applicableGates), whether a prior record still belongs on the node
// (gateStillApplies), which gates must be green to submit
// (requiredGatesRecordedPass), and which were not green at the merge
// (recordPolicyViolations).
func requiredGateConfigs(m *store.Manifest, noun string) []gates.GateConfig {
	if m == nil {
		return nil
	}
	shelf := store.IsWorkNoun(noun)
	var configs []gates.GateConfig
	for name, spec := range m.Gates.Required {
		if !onIncludes(spec.On, gateNoun(noun)) {
			continue
		}
		c := gates.GateConfig{Name: name, Required: true, Kind: spec.Kind, Run: spec.Run, Reuse: spec.Reusable()}
		if shelf && isEvalRecursion(c) {
			continue
		}
		configs = append(configs, c)
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs
}

// applicableGates is requiredGateConfigs made runnable for one node: the same
// name-ordered set, with the node noun and id interpolated into each command
// and the run's `against` revision pinned onto it.
func applicableGates(m *store.Manifest, noun, nodeID, against string) []gates.GateConfig {
	configs := requiredGateConfigs(m, noun)
	for i := range configs {
		configs[i].Run = interpolateNode(configs[i].Run, noun, nodeID)
		configs[i].Against = against
	}
	return configs
}

// criterionStatuses projects required criteria onto the id-to-status view the
// CLI's blocked output names. Optional criteria cannot require a human action.
func criterionStatuses(ef *store.EvalFile, criteria map[string]store.Criterion) map[string]string {
	if ef == nil || len(criteria) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, c := range ef.Criteria {
		if c.Required {
			out[c.ID] = criteria[c.ID].Status
		}
	}
	return out
}

// onIncludes reports whether a gate's `on` grains include noun.
func onIncludes(on store.StringList, noun string) bool {
	for _, g := range on {
		if g == noun {
			return true
		}
	}
	return false
}

// gateNoun is the noun a manifest gate's on: list is matched against. A bug
// or chore runs the task-grade gates (docs/spec/cli-surface.md), so both read
// as task here; every tree noun is itself.
func gateNoun(noun string) string {
	if store.IsWorkNoun(noun) {
		return store.NounTask
	}
	return noun
}

// isEvalRecursion reports whether a gate shells `tiller eval`, the recursion point
// the eval handoff (T13) fills in. Until then such a gate is recorded pending.
func isEvalRecursion(c gates.GateConfig) bool {
	return strings.Contains(c.Run, "tiller eval")
}

// interpolateNode substitutes the node placeholders the manifest gate commands use
// (${node.noun}, ${node.id}) with the running node's values.
func interpolateNode(run, noun, nodeID string) string {
	run = strings.ReplaceAll(run, "${node.noun}", noun)
	run = strings.ReplaceAll(run, "${node.id}", nodeID)
	return run
}

// shortSHA strips the compact "git:" prefix from a git_commit RevisionRef value,
// leaving the bare short sha used to name evidence files. A non-git ref yields its
// value unchanged.
func shortSHA(ref store.RevisionRef) string {
	return strings.TrimPrefix(ref.Value, "git:")
}
