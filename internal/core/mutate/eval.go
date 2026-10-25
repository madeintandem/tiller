package mutate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// This file holds the eval handoff mutation the `tiller eval` verb drives
// (cli-surface.md, "Gates, evals, and attestation"). It is the sibling of Check:
// it reads a node's eval criteria, runs the mechanical ones, and records the
// per-criterion results into the node's `evals` gate, plus the two extra modes the
// handoff needs. --emit-rubrics is a read-only build of the rubric packet for the
// agent to grade; --record-rubric-output validates the agent's graded output and
// moves a required criterion to needs_attestation as evidence awaiting attestation.
// The pure rubric/criteria logic lives in internal/core/gates (eval.go); this layer
// loads the node doc, writes it atomically under the mutation lock, and records one
// event, exactly as Transition and Check do.
//
// A required eval or manual criterion is NEVER satisfied here: the agent's recorded
// rubric is evidence, and only a human `tiller attest --criterion` flips it to pass.

// EvalInput selects the eval verb's mode. With no flag it runs every mechanical
// criterion and records the evals gate. EmitRubrics builds the rubric packet and
// writes nothing. RecordRubricOutput names a file holding the agent's graded
// RubricOutput for the criterion in Criterion, which it validates and records.
// Progress is the optional gate lifecycle sink (issue #45), reaching only the
// default run mode: emit and record never shell a command gate. Follows the
// same nil-is-silent convention as mutate.CheckInput.Progress. Rerun forces
// every command and test criterion to run even where a passed record over
// unchanged code could be reused (design 2026-10-21-gate-reuse).
type EvalInput struct {
	EmitRubrics        bool
	Criterion          string
	RecordRubricOutput string
	Progress           gates.Progress
	Rerun              bool
}

// CriterionOutcome is one criterion in the eval result the CLI renders: its id, the
// binding kind, the resulting per-criterion status, whether it is required, and
// whether its recorded pass was reused from an earlier run rather than run now.
type CriterionOutcome struct {
	ID       string
	Binding  string
	Status   string
	Required bool
	Reused   bool
}

// EvalResult is the outcome of an eval run. Packet is set in the emit mode and is
// the payload the CLI prints. Recorded names the criterion the record mode touched.
// Passed is true when no required criterion failed (a pending, needs_attestation,
// or stale required criterion does not fail the run, it is "not yet"); the CLI
// maps a false Passed to a blocked exit 1.
type EvalResult struct {
	Noun     string
	ID       string
	Key      string
	Status   string
	Criteria []CriterionOutcome
	Packet   *gates.RubricPacket
	Recorded string
	Passed   bool
	Changed  bool
}

// evalTarget abstracts the two node grains eval runs on: a feature records onto
// feature.yml, a task onto its own task.yml, but both read the same
// feature-grain eval file and write the same `evals` gate. save re-marshals the
// whole owning document so the atomic write reflects the mutated gates block.
// The eval contract enters the tree at feature grain and nowhere else
// (artifact-schema.md, Gates), so there is no third grain here.
type evalTarget struct {
	noun     string
	id       string
	key      string
	status   string
	evalFile *store.EvalFile
	gates    *store.GatesBlock
	ymlRel   string
	before   store.FileSnapshot
	save     func() ([]byte, error)
}

// Eval reads a node's eval criteria and runs the requested mode. It dispatches on
// the node grain (feature or task; any other noun is a misuse) and then on the
// input mode. rev supplies the `against` revision for the default run and is the one
// git seam; the emit and record modes do not touch git.
func Eval(tree *store.Tree, deps Deps, rev vcs.Revisions, node store.Node, in EvalInput) (EvalResult, error) {
	target, err := loadEvalTarget(tree, node)
	if err != nil {
		return EvalResult{}, err
	}

	if in.EmitRubrics {
		packet := gates.BuildPacket(target.noun, target.id, target.evalFile, deps.clock().Now().UTC())
		return EvalResult{
			Noun: target.noun, ID: target.id, Key: target.key, Status: target.status,
			Packet: &packet, Passed: true,
		}, nil
	}

	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return EvalResult{}, err
	}

	var res EvalResult
	if in.RecordRubricOutput != "" {
		res, err = recordRubricOutput(tree, deps, owner, target, in)
	} else {
		res, err = runEval(tree, deps, owner, rev, target, in)
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

// runEval is the default mode: evaluate every mechanical criterion, record the
// evals gate, and report whether any required criterion failed. p is the
// optional gate progress sink from EvalInput.Progress. A command or test
// criterion that already passed over the same code identity under the same
// command is reused unless in.Rerun or the manifest's evals gate `reuse: false`
// says otherwise, exactly as check treats it.
func runEval(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, target *evalTarget, in EvalInput) (EvalResult, error) {
	against, err := gateAgainst(rev)
	if err != nil {
		return EvalResult{}, err
	}
	code, err := rev.CodeIdentity()
	if err != nil {
		return EvalResult{}, err
	}
	at := deps.clock().Now().UTC()
	prior := priorCriteria(target.gates)
	criteria, err := gates.EvaluateCriteria(target.evalFile, prior, tree.Manifest, tree.Root, gates.CriteriaRun{
		Against:  shortSHA(against),
		Code:     code,
		Reuse:    !in.Rerun && evalsReuse(tree.Manifest, target.noun),
		At:       at,
		Progress: in.Progress,
	})
	if err != nil {
		return EvalResult{}, err
	}

	setEvalsGate(target.gates, evalsGate{
		required: evalsGateRequired(tree.Manifest),
		runBy:    owner.ID,
		runAt:    at,
		against:  against,
		code:     code,
		criteria: criteria,
		status:   gates.GateStatusFromCriteria(target.evalFile, criteria),
	})

	if err := saveEvalTarget(tree, deps, owner, target, "eval"); err != nil {
		return EvalResult{}, err
	}
	return evalResult(target, "", gates.AllRequiredSatisfied(target.evalFile, criteria)), nil
}

// recordRubricOutput is the record mode: read and validate the agent's graded
// output, persist it as the canonical evidence file, and move the criterion to
// needs_attestation (a pass verdict) or fail. The criterion is never satisfied here.
func recordRubricOutput(tree *store.Tree, deps Deps, owner store.Contributor, target *evalTarget, in EvalInput) (EvalResult, error) {
	data, err := os.ReadFile(in.RecordRubricOutput)
	if err != nil {
		return EvalResult{}, &usageError{
			reason: "rubric_output_unreadable",
			msg:    fmt.Sprintf("cannot read rubric output %q: %v", in.RecordRubricOutput, err),
		}
	}
	out, err := gates.ParseRubricOutput(data)
	if err != nil {
		return EvalResult{}, err
	}
	if in.Criterion != "" && out.Criterion != in.Criterion {
		return EvalResult{}, &usageError{
			reason: "rubric_output_criterion",
			msg:    fmt.Sprintf("--criterion %q does not match the rubric output's criterion %q", in.Criterion, out.Criterion),
		}
	}
	if err := gates.ValidateRubricOutput(out, target.id, target.evalFile); err != nil {
		return EvalResult{}, err
	}

	evidenceRel := gates.EvidencePath(target.id, out.Criterion)
	if err := store.WriteRepositoryFileAtomic(tree.Root, filepath.FromSlash(evidenceRel), data); err != nil {
		return EvalResult{}, err
	}

	recordCriterion(target.gates, evalsGateRequired(tree.Manifest), out.Criterion,
		gates.RecordedCriterionStatus(out.Verdict),
		store.Evidence{Kind: "rubric_output", Ref: evidenceRel},
		owner.ID, deps.clock().Now().UTC(), target.evalFile)

	if err := saveEvalTarget(tree, deps, owner, target, "eval"); err != nil {
		return EvalResult{}, err
	}
	criteria := target.gates.Gates["evals"].Criteria
	return evalResult(target, out.Criterion, gates.AllRequiredSatisfied(target.evalFile, criteria)), nil
}

// loadEvalTarget reads the node's owning document into an evalTarget: a feature from
// feature.yml, a task from its own task.yml. Both bind to the feature-grain eval
// file. A node grain that carries no eval is a misuse, and the epic is one of them:
// an epic has no eval file of its own, because its acceptance bar is the sum of its
// features' contracts, each enforced at feature grain (artifact-schema.md, Epics).
func loadEvalTarget(tree *store.Tree, node store.Node) (*evalTarget, error) {
	switch n := node.(type) {
	case *store.Feature:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: feature %s has no canonical key", n.ID)
		}
		relDir := filepath.Join(".tiller", "epics", n.EpicKey, "features", localKey(*n.Key))
		ymlRel := filepath.Join(relDir, "feature.yml")
		var doc store.Feature
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
		if err != nil {
			return nil, err
		}
		doc.EpicKey = n.EpicKey
		ef, err := loadEvalContract(tree.Root, filepath.Join(relDir, "feature.eval.md"))
		if err != nil {
			return nil, err
		}
		return &evalTarget{
			noun: store.NounFeature, id: doc.ID, key: deref(doc.Key),
			status: doc.Status, evalFile: ef, gates: &doc.GatesBlock, ymlRel: ymlRel, before: snapshot,
			save: func() ([]byte, error) { return yaml.Marshal(doc) },
		}, nil
	case *store.Task:
		feat := tree.Features[n.FeatureID]
		if feat == nil || feat.Key == nil {
			return nil, fmt.Errorf("mutate: task %s has no resolvable parent feature", n.ID)
		}
		relDir := filepath.Join(".tiller", "epics", feat.EpicKey, "features", localKey(*feat.Key))
		target, ymlRel, snapshot, err := loadTaskFile(tree, feat, n)
		if err != nil {
			return nil, err
		}
		ef, err := loadEvalContract(tree.Root, filepath.Join(relDir, "feature.eval.md"))
		if err != nil {
			return nil, err
		}
		return &evalTarget{
			noun: store.NounTask, id: target.ID, key: deref(target.Key), status: target.Status,
			evalFile: ef, gates: &target.GatesBlock, ymlRel: ymlRel, before: snapshot,
			save: func() ([]byte, error) { return yaml.Marshal(target) },
		}, nil
	default:
		return nil, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("eval does not run on a %s", node.Noun()),
		}
	}
}

// saveEvalTarget marshals the mutated document and writes it with one event under
// the repository mutation lock, the same posture as a lifecycle transition.
func saveEvalTarget(tree *store.Tree, deps Deps, owner store.Contributor, target *evalTarget, verb string) error {
	data, err := target.save()
	if err != nil {
		return err
	}
	return commitTransition(tree, deps, owner, TransitionInput{Verb: verb},
		target.noun, target.id, target.ymlRel, data, target.before, target.status, target.status, "")
}

// evalResult builds the CLI-facing result, projecting the recorded criteria in eval
// file order.
func evalResult(target *evalTarget, recorded string, passed bool) EvalResult {
	var outcomes []CriterionOutcome
	criteria := target.gates.Gates["evals"].Criteria
	if target.evalFile != nil {
		for _, c := range target.evalFile.Criteria {
			// A criterion the current mode did not touch (a manual or eval
			// criterion in the record mode, say) has no recorded status; report it
			// as pending rather than an empty string, which is not a valid status.
			status := criteria[c.ID].Status
			if status == "" {
				status = store.CriterionStatusPending
			}
			outcomes = append(outcomes, CriterionOutcome{
				ID: c.ID, Binding: c.Binding.Type,
				Status: status, Required: c.Required,
				Reused: !criteria[c.ID].ReusedAt.IsZero(),
			})
		}
	}
	return EvalResult{
		Noun: target.noun, ID: target.id, Key: target.key, Status: target.status,
		Criteria: outcomes, Recorded: recorded, Passed: passed, Changed: true,
	}
}

// evalsGate is the field set setEvalsGate writes onto the node's evals gate.
type evalsGate struct {
	required bool
	runBy    string
	runAt    time.Time
	against  store.RevisionRef
	code     string
	criteria map[string]store.Criterion
	status   string
}

// setEvalsGate writes the whole evals gate (the default run replaces the criteria
// map and the run provenance).
func setEvalsGate(gb *store.GatesBlock, g evalsGate) {
	if gb.Gates == nil {
		gb.Gates = map[string]store.Gate{}
	}
	gb.Gates["evals"] = store.Gate{
		Required: g.required,
		Kind:     store.GateKindCommand,
		Status:   g.status,
		RunBy:    g.runBy,
		RunAt:    g.runAt,
		Against:  g.against,
		Code:     g.code,
		Criteria: g.criteria,
	}
}

// recordCriterion sets one criterion's status and evidence inside the evals gate,
// creating the gate when the record mode reaches a node eval never ran on, and
// recomputes the gate roll-up status.
func recordCriterion(gb *store.GatesBlock, required bool, critID, status string, evidence store.Evidence, runBy string, at time.Time, ef *store.EvalFile) {
	if gb.Gates == nil {
		gb.Gates = map[string]store.Gate{}
	}
	g, ok := gb.Gates["evals"]
	if !ok {
		g = store.Gate{Required: required, Kind: store.GateKindCommand}
	}
	g.RunBy = runBy
	g.RunAt = at
	if g.Criteria == nil {
		g.Criteria = map[string]store.Criterion{}
	}
	g.Criteria[critID] = store.Criterion{Status: status, Evidence: []store.Evidence{evidence}}
	g.Status = gates.GateStatusFromCriteria(ef, g.Criteria)
	gb.Gates["evals"] = g
}

// priorCriteria returns the node's already-recorded evals criteria map, so a
// re-run can preserve human-settled statuses.
func priorCriteria(gb *store.GatesBlock) map[string]store.Criterion {
	if gb == nil || gb.Gates == nil {
		return nil
	}
	return gb.Gates["evals"].Criteria
}

// loadEvalContract loads and validates a feature.eval.md whenever an eval or
// check path uses it. Absence remains legal for draft-stage authoring; every
// existing unreadable, unparseable, or invalid file fails closed.
func loadEvalContract(root, relative string) (*store.EvalFile, error) {
	ef, err := store.LoadEvalFile(root, relative)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &store.EvalContractError{
			Msg: fmt.Sprintf("feature.eval.md is not a readable acceptance contract: %v", err),
		}
	}
	if err := ef.ValidateContract(); err != nil {
		return nil, err
	}
	return ef, nil
}

// evalsReuse reports whether the manifest lets the evals gate's command and
// test criteria be reused on this grain: true unless the gate is declared with
// `reuse: false`. A manifest with no evals gate for the grain leaves reuse on.
func evalsReuse(m *store.Manifest, noun string) bool {
	for _, c := range requiredGateConfigs(m, noun) {
		if isEvalRecursion(c) {
			return c.Reuse
		}
	}
	return true
}

// evalsGateRequired reports whether the manifest marks the evals gate required.
func evalsGateRequired(m *store.Manifest) bool {
	if m == nil {
		return false
	}
	_, ok := m.Gates.Required["evals"]
	return ok
}
