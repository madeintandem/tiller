package cli

import (
	"encoding/json"
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file holds the batch arms of the high-repetition verbs (issue #14, the
// first full-arc dry run): approve over several nodes, and approve task
// --feature. The contract is all-or-nothing
// resolution, then per-node application: every address must resolve before
// anything mutates (one typo cannot leave a batch half-applied, exit 2), then
// each node is applied in argument order and a blocked node does not stop the
// rest. Exit 0 only when every node succeeded (a no-change idempotent success
// counts), exit 1 when any node was blocked.
//
// A single-node invocation never routes here, so its envelope stays byte-
// identical to the pre-batch shape. A multi-node invocation (2+ positional
// nodes, or --feature) emits data.results, one entry per node in argument
// order carrying the same fields the single-node data block carries plus a
// per-node ok flag, and data.summary with the outcome counts.

// BatchSummary counts the per-node outcomes of one batch invocation.
type BatchSummary struct {
	Total     int `json:"total"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Blocked   int `json:"blocked"`
}

// BatchData is the data block of a batch envelope: one result entry per node
// in argument order, plus the outcome summary.
type BatchData struct {
	Results []map[string]any `json:"results"`
	Summary BatchSummary     `json:"summary"`
}

// runTransitionBatch is the multi-node arm of a lifecycle verb: validate the
// noun once, resolve every address (all-or-nothing), then apply the transition
// per node in argument order.
func runTransitionBatch(cmd *cobra.Command, command, noun string, nodeArgs []string, allowed []string, in mutate.TransitionInput) error {
	sing, err := validNoun(cmd, command, noun, allowed)
	if err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	nodes, err := resolveAll(tree, sing, nodeArgs)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	return applyTransitionBatch(cmd, command, tree, nodes, in)
}

// runApproveTasksByFeature is the `approve task --feature <key>` arm: resolve
// the feature, collect its proposed tasks in sibling order, and approve them
// all. Zero proposed tasks is a success, not an error.
func runApproveTasksByFeature(cmd *cobra.Command, featureAddr string, in mutate.TransitionInput) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounFeature, Raw: featureAddr})
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	feat := n.(*store.Feature)
	// Resolve the acting contributor before either arm below, the zero-proposed-tasks
	// one included: applyTransitionBatch's own deps call (below) already
	// refuses a corrupt identity file or a bad --contributor before it mutates
	// anything, but the zero-tasks arm used to return its "nothing to do"
	// success before deps() ever ran, so that one arm alone let a corrupt
	// .tiller/identity.yml through silently (issue #58). The resolved value
	// itself is unused here; applyTransitionBatch resolves it again
	// independently when there are nodes to apply, which is deterministic
	// within the same invocation, so this hoist exists purely to make the
	// refusal fire uniformly across the whole verb.
	if _, err := deps(cmd, tree.Root); err != nil {
		return emitFailure(cmd, "approve", err)
	}
	tasks := mutate.ProposedTasks(tree, feat)
	if len(tasks) == 0 {
		data := BatchData{Results: []map[string]any{}, Summary: BatchSummary{}}
		// Zero proposed tasks is still a completed `approve` invocation (ok
		// true, code 0), so it carries the readout the same as every other
		// approve outcome; the confirmation just has nothing to report beyond
		// the feature named. act carries no Nodes, so approveContent
		// (internal/views/readout.go) renders no Verification section at
		// all rather than a false claim about a transition that never ran.
		//
		// BasisRefresh still rides along, because which act this was is a fact
		// about the invocation and stays true when the invocation reached no
		// node: hardcoding it false here printed "Approval recorded" over a
		// `--refresh-basis` run that approved nothing
		// (task-9-findings-round-1.md, Important 2).
		return emitReportWithReadout(cmd, "approve", true, data,
			fmt.Sprintf("No proposed tasks on feature %s.", gen.Ref(tree, n)), 0,
			gateMomentFor("approve"), tree.Root, &views.GateAct{BasisRefresh: in.RefreshBasis})
	}
	nodes := make([]store.Node, len(tasks))
	for i, tk := range tasks {
		nodes[i] = tk
	}
	return applyTransitionBatch(cmd, "approve", tree, nodes, in)
}

// applyTransitionBatch applies one lifecycle verb to already-resolved nodes in
// order, collecting a per-node entry for each: the success view for a
// transition (a no-change idempotent success included), or the blocked data
// block for an illegal move, which does not stop the remaining nodes. A tool
// failure (a bad owner, a write fault) aborts the batch with an error envelope.
func applyTransitionBatch(cmd *cobra.Command, command string, tree *store.Tree, nodes []store.Node, in mutate.TransitionInput) error {
	results := make([]map[string]any, 0, len(nodes))
	sum := BatchSummary{Total: len(nodes)}
	var lines []string
	// act.Nodes accumulates one entry per node that actually transitioned
	// (a blocked node is never appended: it never reached the target, so
	// approveContent's "moved to ready" count must not count it), the
	// batch's own "what just happened" for the readout that follows every
	// per-node line (round-1 review findings 1 and 2, fixed the same way
	// the single-node path was: carry the mutation result forward instead
	// of asking the block to re-derive it from a tree tally).
	// BasisRefresh is a property of the invocation, not of any one node's
	// result, so it is set once here: `in` is the same TransitionInput for
	// every node in the batch (this function's own parameter).
	act := &views.GateAct{BasisRefresh: in.RefreshBasis}
	// One owner resolution serves the whole batch (a corrupt identity file
	// refuses once here, not once per node), the same rule resolveActor's own
	// comment above states for the batch's attribution.
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	for _, n := range nodes {
		res, err := mutate.Transition(tree, d, n, in)
		if err != nil {
			data, human, matched := transitionBlockedView(tree, n, err)
			if !matched {
				return emitFailure(cmd, command, err)
			}
			sum.Blocked++
			results = append(results, batchEntry(false, data))
			lines = append(lines, human)
			continue
		}
		if res.Changed {
			sum.Changed++
		} else {
			sum.Unchanged++
		}
		results = append(results, batchEntry(true, transitionView(res)))
		// The review-state warning (issue #42) only ever applies to submit,
		// which has no batch arm (approve is the only verb runTransitionBatch
		// ever serves), so this passes nil unconditionally.
		lines = append(lines, humanTransition(tree, command, in, res, gen.Ref(tree, n), nil))
		// A plan gate moves the feature and its initial tasks, so each entry
		// contributes every node it moved; transitionAct is where that walk lives,
		// shared with the single-node arm so the two cannot count differently.
		act.Nodes = append(act.Nodes, transitionAct(in, res).Nodes...)
	}
	// approve is the only verb with a batch arm today (lifecycle_cmds.go's
	// newApproveCmd is the one caller of runTransitionBatch, and
	// runApproveTasksByFeature always passes "approve"), so gateMomentFor
	// resolves this to MomentApprove every time it actually runs; it stays a
	// lookup rather than a literal so a future batchable gate does not have
	// to touch this line to pick up its own readout.
	return emitBatch(cmd, command, BatchData{Results: results, Summary: sum}, lines, gateMomentFor(command), tree.Root, act)
}

// resolveAll resolves every address before anything mutates: the all-or-nothing
// rule, so one bad address in a batch is exit 2 misuse with nothing applied.
func resolveAll(tree *store.Tree, noun string, raws []string) ([]store.Node, error) {
	r := resolve.NewResolver(tree)
	nodes := make([]store.Node, 0, len(raws))
	for _, raw := range raws {
		n, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// emitBatch renders a batch outcome: the ok flag is true only when no node was
// blocked, and a blocked batch exits 1 after every node was attempted. The
// human output is the per-node lines in argument order (all on stdout, so the
// transcript reads in order), plus one summary line when the outcomes are
// mixed.
//
// m is the human-gate moment the batch's own command resolves to ("" for a
// batch verb that is not a gate), root is the tree's root for the fresh
// reload the block needs, and act is the batch's own accumulated "what just
// happened" (applyTransitionBatch builds one Nodes entry per node that
// actually transitioned; nil for a non-gate batch). Both non-gate arms are
// currently unreached: `approve` is the only batch verb left, so m always
// names a moment and act is always non-nil. They stay because the signature
// is the general one every batch verb enters through, not because a caller
// exercises them today. The block prints once, after every per-node line
// and the mixed-outcome summary, never once per node: a batch approve's
// lines already are the per-node record, and one readout at the end is "the
// block follows them," not "the block follows each of them". It renders only
// when the batch is fully ok: a blocked node has already been reported as such
// in its own line, and this function's own error return covers the exit code,
// so there is nothing a readout would add over a failed batch that the existing
// per-node blocked lines have not already said.
func emitBatch(cmd *cobra.Command, command string, data BatchData, lines []string, m views.Moment, root string, act *views.GateAct) error {
	ok := data.Summary.Blocked == 0
	var block string
	if ok && m != "" {
		b, err := renderReadoutFor(root, m, act, resolveActingContributor(cmd, root))
		if err != nil {
			// Same rule as emitReportWithReadout: the batch already succeeded,
			// so a reporting problem here is logged and the block is omitted
			// rather than turned into a batch failure.
			fmt.Fprintf(cmd.ErrOrStderr(), "tiller: readout for %s not available: %v\n", command, err)
		} else {
			block = b
		}
	}
	// A batch invocation attributes to one acting contributor for the whole batch
	// (one --contributor/env/identity resolution serves every node), so the actor
	// merges in once at the top level, the same additive rule a single-node
	// report follows (envelope.go's emitAttributed).
	a := resolveActor(cmd)
	if jsonOut(cmd) {
		payload := withActor(any(data), a)
		if block != "" {
			payload = withReadout(payload, block)
		}
		_ = WriteResult(cmd.OutOrStdout(), command, ok, payload)
	} else {
		w := cmd.OutOrStdout()
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		if s := data.Summary; mixedOutcomes(s) {
			fmt.Fprintf(w, "Summary: %d total, %d changed, %d unchanged, %d blocked.\n",
				s.Total, s.Changed, s.Unchanged, s.Blocked)
		}
		if echo := actorEcho(a); echo != "" {
			fmt.Fprintln(w, echo)
		}
		if block != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, block)
		}
	}
	if !ok {
		return &exitError{code: 1, msg: fmt.Sprintf("%d of %d node(s) blocked", data.Summary.Blocked, data.Summary.Total)}
	}
	return nil
}

// batchEntry flattens one node's single-shape payload (the success view or the
// blocked data block) into a results entry, adding the per-node ok flag. Going
// through JSON keeps the entry's fields identical to the single-node envelope
// by construction.
func batchEntry(ok bool, payload any) map[string]any {
	m := map[string]any{"ok": ok}
	b, err := json.Marshal(payload)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	m["ok"] = ok
	return m
}

// mixedOutcomes reports whether a batch produced more than one outcome kind,
// which is when the human summary line earns its place.
func mixedOutcomes(s BatchSummary) bool {
	kinds := 0
	for _, n := range []int{s.Changed, s.Unchanged, s.Blocked} {
		if n > 0 {
			kinds++
		}
	}
	return kinds > 1
}
