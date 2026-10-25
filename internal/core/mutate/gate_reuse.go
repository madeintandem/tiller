package mutate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// This file holds command gate reuse (docs/plans/2026-10-21-gate-reuse-design.md):
// a command gate that already passed over the same code identity under the
// same resolved command stands in for a fresh run. Only a pass is reused, an
// empty code identity never matches, and reuse reads only gate records in the
// loaded tree (decision 4), never a cache, another branch, or history.

// reuseSource is one gates block a check may reuse a recorded pass from. An
// empty nodeID is the checked node's own record; any other id names the node
// the block belongs to, which a reused copy records as its reused_from.
type reuseSource struct {
	nodeID string
	block  *store.GatesBlock
}

// gateRun carries what one evaluateGates call needs beyond the node: the
// revision the run is pinned to, the current code identity (empty when there
// is none, which disables reuse), who runs it and when, whether --rerun forces
// real runs, the --gate set this invocation may run (nil runs every gate), the
// blocks a pass may be reused from, and the progress sink.
type gateRun struct {
	against  store.RevisionRef
	code     string
	runBy    string
	at       time.Time
	rerun    bool
	only     map[string]bool
	sources  []reuseSource
	progress gates.Progress
}

// newGateRun builds the gateRun every check path shares: pinned to against,
// run by owner at the deps clock's now, under the current code identity, with
// the node's own gates block as its one reuse source. A path that may reuse
// from other nodes appends their blocks to sources.
func newGateRun(rev vcs.Revisions, against store.RevisionRef, owner store.Contributor, deps Deps, in CheckInput, own *store.GatesBlock) (gateRun, error) {
	code, err := rev.CodeIdentity()
	if err != nil {
		return gateRun{}, err
	}
	var only map[string]bool
	if len(in.Gates) > 0 {
		only = make(map[string]bool, len(in.Gates))
		for _, name := range in.Gates {
			only[name] = true
		}
	}
	return gateRun{
		against:  against,
		code:     code,
		runBy:    owner.ID,
		at:       deps.clock().Now().UTC(),
		rerun:    in.Rerun,
		only:     only,
		sources:  []reuseSource{{block: own}},
		progress: in.Progress,
	}, nil
}

// held reports whether gate name sits outside this invocation's --gate set:
// it is never shelled, only reused or reported, with a pass that cannot be
// reused marked stale (decision 10, notRunOutcome).
func (run gateRun) held(name string) bool {
	return run.only != nil && !run.only[name]
}

// validateOnly refuses a --gate name that is not one of the gates this node
// grain evaluates (the applicable config names, plus evals when the eval
// contract applies), before any gate runs.
func validateOnly(run gateRun, configs []gates.GateConfig, evals bool, noun string) error {
	if run.only == nil {
		return nil
	}
	known := map[string]bool{}
	for _, c := range configs {
		if !isEvalRecursion(c) {
			known[c.Name] = true
		}
	}
	if evals {
		known["evals"] = true
	}
	applicable := make([]string, 0, len(known))
	for name := range known {
		applicable = append(applicable, name)
	}
	sort.Strings(applicable)
	asked := make([]string, 0, len(run.only))
	for name := range run.only {
		asked = append(asked, name)
	}
	sort.Strings(asked)
	for _, name := range asked {
		if !known[name] {
			return &usageError{
				reason: "unknown_gate",
				msg: fmt.Sprintf("%s is not a required gate on this %s; applicable: %s",
					name, noun, strings.Join(applicable, ", ")),
			}
		}
	}
	return nil
}

// notRunOutcome reports a held gate that could not be reused, without running
// it, and returns its resulting record. A prior pass no longer speaks for this
// code (it was not reusable), so the gate is marked stale, keeping its
// evidence and provenance, and staleIt is true: the caller writes it, or the
// old pass would stay on record for submit, sync, and readout, which read
// status alone. Its criteria are left as they were; the evals caller stales
// only the ones a full check would rerun (gates.StaleRerunCriteria), so a
// human's verdict survives. A prior fail, pending, or stale is carried as it
// is and not rewritten; with no prior the gate reports pending and nothing is
// written.
func notRunOutcome(name string, required bool, kind string, prior *store.GatesBlock) (o GateOutcome, g store.Gate, staleIt bool) {
	o = GateOutcome{Name: name, Required: required, Kind: kind, Status: store.GateStatusPending, NotRun: true}
	ok := false
	if prior != nil {
		g, ok = prior.Gates[name]
	}
	if !ok {
		return o, g, false
	}
	if g.Status == store.GateStatusPass {
		g.Status = store.GateStatusStale
		g.Required = required
		staleIt = true
	}
	o.Status, o.Against, o.EvidenceRef = g.Status, g.Against.Value, commandEvidenceRef(g)
	return o, g, staleIt
}

// reuseHeldEvals returns the node's prior evals record stamped as reused, for
// an evals gate held out by --gate: it counts only when reuse is allowed (no
// --rerun, no `reuse: false`), the record is a pass over this run's non-empty
// code identity, and every passed command or test criterion in it is still
// keyed to the command it resolves to now (gates.CriteriaCurrent). The evals
// gate has no command of its own, so the config half of the reuse key lives
// on its criteria: a moved tests runner retires a test binding's pass.
func reuseHeldEvals(prior *store.GatesBlock, ef *store.EvalFile, m *store.Manifest, required, reuse bool, run gateRun) (store.Gate, bool) {
	if prior == nil || run.rerun || !reuse || run.code == "" {
		return store.Gate{}, false
	}
	g, ok := prior.Gates["evals"]
	if !ok || g.Kind != store.GateKindCommand || g.Status != store.GateStatusPass || g.Code != run.code {
		return store.Gate{}, false
	}
	if !gates.CriteriaCurrent(ef, g.Criteria, m, run.code) {
		return store.Gate{}, false
	}
	g.Required = required
	g.ReusedAt = run.at
	return g, true
}

// gateResult is one required-set member's resulting record after a check's
// evaluation: fresh, reused, or a held gate's prior (staled if it was a
// pass). codeBound marks a command-backed gate (and evals), whose pass counts
// only over this run's code identity; notRun marks a held gate with no
// current record.
type gateResult struct {
	required  bool
	record    store.Gate
	codeBound bool
	notRun    bool
}

// gatesGreen is the advance rule (decision 10): every required gate's
// resulting record is a pass, and a command-backed one's is over the current
// code identity, so a partial --gate run never advances work on a sibling's
// stale pass. A gate held out with no current record is not green. With no
// code identity (a stub, or no git) the code match is skipped and the rule is
// the plain status check over this run's results.
func gatesGreen(results []gateResult, code string) bool {
	for _, r := range results {
		if !r.required {
			continue
		}
		if r.notRun || r.record.Status != store.GateStatusPass {
			return false
		}
		if code != "" && r.codeBound && r.record.Code != code {
			return false
		}
	}
	return true
}

// reusedGates names what a check reused for its event: each gate reported
// reused, and evals/<criterion id> for each criterion this run reused (its
// reused_at is this run's instant), sorted.
func reusedGates(outcomes []GateOutcome, block map[string]store.Gate, at time.Time) []string {
	var names []string
	for _, o := range outcomes {
		if o.Reused {
			names = append(names, o.Name)
		}
	}
	for id, c := range block["evals"].Criteria {
		if !c.ReusedAt.IsZero() && c.ReusedAt.Equal(at) {
			names = append(names, "evals/"+id)
		}
	}
	sort.Strings(names)
	return names
}

// ridingSources returns one reuse source per task riding feature featureID,
// sorted by task id so the first matching pass is deterministic. A feature
// check reuses a task's pass when the aggregate's code identity equals the code
// that task checked (design decision 4); the match is still keyed on the
// resolved command, so a command that names the node never crosses grains.
func ridingSources(tree *store.Tree, featureID string) []reuseSource {
	tasks := append([]*store.Task(nil), tree.FeatureTasks[featureID]...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	sources := make([]reuseSource, 0, len(tasks))
	for _, task := range tasks {
		sources = append(sources, reuseSource{nodeID: task.ID, block: &task.GatesBlock})
	}
	return sources
}

// reuseCommandGate returns the record that stands in for running command gate
// c this check, when reuse is allowed: not forced off by --rerun, not opted out
// by the manifest's `reuse: false`, and a current code identity to compare
// (an empty one never matches). The copy keeps the original run's run_by,
// run_at, against, and evidence (decision 15), is marked reused_at now, takes
// the manifest's current required flag, and names its source node in
// reused_from when the pass came from another node; a pass reused from the
// node's own record keeps whatever reused_from it already carried.
func reuseCommandGate(c gates.GateConfig, run gateRun) (store.Gate, bool) {
	if run.rerun || !c.Reuse || run.code == "" {
		return store.Gate{}, false
	}
	g, from, ok := findReusable(run.sources, c.Name, run.code, gates.ConfigHash(c.Kind, c.Run))
	if !ok {
		return store.Gate{}, false
	}
	g.Required = c.Required
	g.ReusedAt = run.at
	if from != "" {
		g.ReusedFrom = from
	}
	return g, true
}

// commandEvidenceRef is the command output a gate record points at, the
// evidence ref a reused gate's outcome reports in place of a fresh run's.
func commandEvidenceRef(g store.Gate) string {
	for _, e := range g.Evidence {
		if e.Kind == "command_output" {
			return e.Ref
		}
	}
	return ""
}

// findReusable returns the first source's record for gate name that may stand
// in for a fresh run: a command-kind pass whose stored code identity is
// non-empty and equal to code and whose config hash equals config. A record
// written before this build carries neither field and never matches. It also
// returns the id of the source the record came from.
func findReusable(sources []reuseSource, name, code, config string) (store.Gate, string, bool) {
	for _, src := range sources {
		if src.block == nil {
			continue
		}
		g, ok := src.block.Gates[name]
		if !ok {
			continue
		}
		if g.Kind == store.GateKindCommand && g.Status == store.GateStatusPass &&
			g.Code != "" && g.Code == code && g.Config == config {
			return g, src.nodeID, true
		}
	}
	return store.Gate{}, "", false
}
