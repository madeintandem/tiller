package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/spf13/cobra"
)

// This file implements `tiller status`, the one node command with a bare form
// (cli-surface.md, "Inspecting and maintaining"): with no node it prints the
// project rollup, and `status <noun> <node>` is the typed per-node oracle. The
// oracle reports the node's state, the transitions legal from here (legal_next,
// computed by the lifecycle engine), and its recorded gate results.

// The summary and oracle shapes and their projections live in internal/views,
// one source the CLI and the serve console both render. Aliases keep the
// renderers here reading naturally.
type (
	ProjectSummary  = views.ProjectSummary
	ProjectInfo     = views.ProjectInfo
	ContributorInfo = views.ContributorInfo
	TreeCounts      = views.TreeCounts
	NodeStatusView  = views.NodeStatusView
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [<noun> <node>]",
		Short: "Show the project summary, or the typed oracle for one node.",
		Long: "status with no node prints the project summary: identity, contributors, and\n" +
			"node counts. status <noun> <node> is the typed oracle for one node: its\n" +
			"state, the moves legal from here, its recorded gate results, and any\n" +
			"active_basis drift (a spec or contract edit since approval, which start,\n" +
			"check, and submit refuse until the basis is refreshed). It reports; it\n" +
			"never gates.",
		// Two shapes only: the bare project summary (no args) and the typed
		// oracle (<noun> <node>). A lone noun with no node is arg-count misuse.
		Args: statusArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runStatusSummary(cmd)
			}
			return runStatusNode(cmd, args[0], args[1])
		},
	}
}

// statusArgs accepts exactly the two legal shapes: no args (project summary) or
// a noun and a node (the oracle). One arg is rejected as misuse the same way
// cobra rejects a wrong arg count elsewhere.
func statusArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || len(args) == 2 {
		return nil
	}
	return fmt.Errorf("status takes either no arguments (project summary) or <noun> <node>")
}

// runStatusNode resolves the addressed node and prints the typed oracle: its
// state, legal next moves, recorded gates, and active-basis drift.
func runStatusNode(cmd *cobra.Command, noun, node string) error {
	tree, n, err := resolveNodeWithTree(cmd, "status", noun, node)
	if err != nil {
		return err
	}
	view := views.NodeStatus(tree, n, readinput.WorkflowForNode(tree, n))
	drift, line := statusBasisDrift(tree, n)
	view.ActiveBasis = drift
	// The review-state warning: a live git read, so it is filled
	// in here rather than by views.NodeStatus, the same ActiveBasis posture
	// this file already follows. review.ForTask itself narrows to a task in
	// review with a recorded PR, so this is a harmless no-op lookup for every
	// other noun status can address.
	unpushed := review.ForTask(tree, n.NodeID(), vcs.New(tree.Root))
	view.UnpushedReview = unpushedReviewView(unpushed)
	return emitSuccess(cmd, "status", view, renderStatusNodeHuman(view, line, unpushed))
}

// statusBasisDrift answers the reporting half of the active-basis rule for the
// oracle: the drifted file names for the --json payload and the one human line
// for the terminal, both nil/"" when nothing drifted.
//
// The comparison hashes working-tree files through git, which is I/O, so it runs
// here rather than in views.NodeStatus (internal/views performs none). It calls
// mutate.BasisDrift, the identical predicate guardFeature and guardTask run
// ahead of start, check, and submit, so `status` cannot report a node clean that
// those verbs would refuse (cli-surface.md, "Specifying a feature").
//
// A non-ActiveBasisError from the comparison is a git that could not answer, and
// it leaves both returns empty rather than failing the command: status, ls, and
// check readiness all answer with no git at all, and a read whose whole job is
// to report must not become the one command a repo without a working git cannot
// run. What it does not do is assert clean; the drift line is printed when there
// IS drift, and its absence is the same silence a node with no pin produces.
func statusBasisDrift(tree *store.Tree, n store.Node) ([]string, string) {
	var ab *mutate.ActiveBasisError
	if err := mutate.BasisDrift(tree, n, vcs.New(tree.Root)); errors.As(err, &ab) {
		// The line leads with the files rather than with the node, because the
		// reader already has the node: it is the header two lines above.
		//
		// It names a node only when the pin that moved belongs to a DIFFERENT
		// one than the node asked about, which happens on a task, whose guard
		// compares its parent feature's spec and contract before its own
		// task.md (mutate.guardTask). There the grain is the whole point: it is
		// the node the remedy addresses, and a reader looking at a task's status
		// would otherwise be handed a feature's command with nothing saying why.
		if ab.NodeID == n.NodeID() {
			return ab.Drifted, fmt.Sprintf("%s; %s.", activeBasisDrifted(ab), activeBasisRemedy(tree, ab))
		}
		return ab.Drifted, fmt.Sprintf("%s on %s %s; %s.",
			activeBasisDrifted(ab), ab.Noun, gen.RefByID(tree, ab.NodeID), activeBasisRemedy(tree, ab))
	}
	return nil, ""
}

// renderStatusNodeHuman is the plain-text oracle for a terminal reader. The
// header names the node by the shared reference convention (gen.Ref), and
// echoes the short prose handle the reference does not already carry. A
// question's full statement remains in the body because status on one subject
// is a detail surface, not a scan. basis is the active-basis drift line
// statusBasisDrift built, "" when there is none. unpushed is the review-state
// warning's facts, nil for every node that is not an in-review task
// with an unpushed recorded PR branch; the same shared wording submit's own
// confirmation renders (unpushedReviewWarning).
func renderStatusNodeHuman(v NodeStatusView, basis string, unpushed *UnpushedReviewFacts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", v.Noun, v.Ref)
	if v.Title != "" {
		fmt.Fprintf(&b, ": %q", v.Title)
	}
	fmt.Fprint(&b, "\n")
	if v.Statement != "" {
		fmt.Fprintf(&b, "  statement: %s\n", v.Statement)
	}
	fmt.Fprintf(&b, "  state: %s", v.Status)
	if v.Activity == gen.EpicActivityActive {
		// Make it obvious an approved epic is being worked while it is stored ready.
		fmt.Fprint(&b, " (active: work in flight)")
	}
	if v.Activity == gen.FeatureActivityAwaitingCheck {
		if v.RecoveredReview != nil {
			fmt.Fprint(&b, " (all tasks done; current feature gates remain independently unresolved)")
		} else {
			fmt.Fprint(&b, " (awaiting check: all tasks done, feature gates unrecorded)")
		}
	}
	if v.Activity == gen.FeatureActivityAwaitingSync && (v.Workflow == nil || v.Workflow.Phase == "sync") {
		fmt.Fprint(&b, " (awaiting sync: rollup earned, the next sync records it)")
	}
	if v.Activity == gen.ActivityCarrierCancelled {
		fmt.Fprint(&b, " (carrier cancelled: it will never merge; resolve --clear or --by re-points it)")
	}
	if v.Blocked {
		fmt.Fprint(&b, " (blocked)")
	}
	// The review-state warning sits directly under state, the
	// status this task carries while the warning applies, in the identical
	// wording submit's own confirmation renders (unpushedReviewWarning): a
	// reader who lands on this oracle to see why a PR looks stalled meets the
	// same sentence either way.
	if unpushed != nil {
		fmt.Fprintf(&b, "\n  %s", unpushedReviewWarning(unpushed.Commit, unpushed.Branch))
	}
	// held sits between state and next: it says WHY the doing verbs below are
	// about to refuse (gen.RetiredAncestor), the read half of
	// mutate.guardParentState's grandparent walk, never a replacement for
	// legal_next or next_owner, which stay pure lifecycle projections either
	// way (the "annotate, don't subtract" decision).
	if v.Held != nil {
		fmt.Fprintf(&b, "\n  held:  %s %s is %s; the doing verbs will refuse",
			v.Held.Noun, v.Held.Ref, v.Held.State)
	}
	renderStatusQuestionRelationships(&b, v.Relationships)
	if v.Workflow != nil {
		fmt.Fprintf(&b, "\n  workflow: %s (%s): %s", v.Workflow.Phase, v.Workflow.Actor, v.Workflow.Reason)
		if v.Workflow.Command != "" {
			fmt.Fprintf(&b, "; `%s`", v.Workflow.Command)
		}
	}
	if v.Workflow == nil && v.RecoveredReview != nil {
		fmt.Fprintf(&b, "\n  %s", workflow.RecoverySummary(v.RecoveredReview))
	}
	if v.Workflow == nil && v.ReviewMissing {
		fmt.Fprintf(&b, "\n  %s", workflow.MissingReviewNotice)
	}
	if len(v.LegalNext) == 0 {
		fmt.Fprint(&b, "\n  next:  (none, terminal)")
	} else {
		fmt.Fprintf(&b, "\n  next:  %s", strings.Join(v.LegalNext, ", "))
	}
	if len(v.Gates) == 0 {
		fmt.Fprint(&b, "\n  gates: (none recorded)")
	} else {
		names := make([]string, 0, len(v.Gates))
		for name := range v.Gates {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "\n  gate %s: %s", name, v.Gates[name].Status)
		}
	}
	renderPolicyViolationsHuman(&b, v.PolicyViolations, v.Gates)
	// The drift line prints only when there is drift, the same rule the blocked
	// flag and the gates block already follow: the oracle is terse about what is
	// unremarkable. It leads with "active_basis" because that is the reason
	// string the refusal carries and the drift marker the schema names, so a
	// reader meets one word across the read and the refusal.
	if basis != "" {
		fmt.Fprintf(&b, "\n  active_basis: %s", basis)
	}
	return b.String()
}

// renderStatusQuestionRelationships writes the status oracle's narrow
// relationship projection. Forward block targets use their composed work refs.
// Reverse blockers use the question's id and quoted short title; Statement is
// deliberately reserved for the question subject's own detail above.
func renderStatusQuestionRelationships(b *strings.Builder, relationships *views.Relationships) {
	if relationships == nil {
		return
	}
	if len(relationships.Blocks) > 0 {
		b.WriteString("\n  blocks: ")
		renderStatusRelationshipRefs(b, relationships.Blocks, false)
	}
	if len(relationships.BlockedByQuestions) > 0 {
		b.WriteString("\n  blocked by questions: ")
		renderStatusRelationshipRefs(b, relationships.BlockedByQuestions, true)
	}
}

func renderStatusRelationshipRefs(b *strings.Builder, nodes []views.RelatedNode, includeQuestionTitle bool) {
	for i, node := range nodes {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(node.Ref)
		if includeQuestionTitle {
			fmt.Fprintf(b, " (%q)", node.Title)
		}
	}
}

func runStatusSummary(cmd *cobra.Command) error {
	root, err := repoRoot()
	if err != nil {
		return emitFailure(cmd, "status", err)
	}
	tree, err := store.Load(root)
	if err != nil {
		return emitFailure(cmd, "status", err)
	}
	summary := views.Summary(tree)
	// The uncommitted-state line is a live git read, the same
	// reason ActiveBasis and UnpushedReview are filled in here rather than by
	// views: internal/views performs no I/O.
	summary.UncommittedState = uncommittedTillerState(root)
	return emitSuccess(cmd, "status", summary, renderStatusHuman(summary))
}

// uncommittedTillerState reports the repo-relative paths under .tiller/ that
// carry an uncommitted change, via vcs.DirtyUnder. A git that
// cannot answer (no repository, git missing) leaves the summary silent
// rather than failing the read, the same posture statusBasisDrift already
// takes for the identical reason: a read whose whole job is to report must
// not become the one command a repo without a working git cannot run.
func uncommittedTillerState(root string) []string {
	paths, err := vcs.New(root).DirtyUnder(".tiller")
	if err != nil {
		return nil
	}
	return paths
}

// renderStatusHuman is the plain-text summary for a terminal reader.
func renderStatusHuman(s ProjectSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s (%s) [%s]\n", s.Project.Name, s.Project.Key, s.Project.Kind)
	if len(s.Contributors) == 0 {
		fmt.Fprintln(&b, "Contributors: (none)")
	} else {
		ids := make([]string, len(s.Contributors))
		for i, o := range s.Contributors {
			ids[i] = o.ID
		}
		fmt.Fprintf(&b, "Contributors: %s\n", strings.Join(ids, ", "))
	}
	fmt.Fprintf(&b, "Epics %d  Features %d  Tasks %d  Questions %d",
		s.Counts.Epics, s.Counts.Features, s.Counts.Tasks, s.Counts.Questions)
	if s.Counts.Bugs > 0 || s.Counts.Chores > 0 || s.Counts.Tweaks > 0 {
		fmt.Fprintf(&b, "  Bugs %d  Chores %d  Tweaks %d", s.Counts.Bugs, s.Counts.Chores, s.Counts.Tweaks)
	}
	if len(s.ActiveEpicRefs) > 0 {
		fmt.Fprintf(&b, "\nActive epics: %s", strings.Join(s.ActiveEpicRefs, ", "))
	}
	// Order is carriage only, the recorded root epic sequence `tiller order
	// epic` last wrote, printed in that recorded sequence (never sorted, the
	// same rule the epic and feature bodies' own order: line follows in
	// show.go's renderOrderHuman). Silent, the same show-only-what-is-there
	// rule Active epics above already follows, until a root order is first
	// recorded.
	if len(s.OrderRefs) > 0 {
		fmt.Fprintf(&b, "\nOrder: %s", strings.Join(s.OrderRefs, ", "))
	}
	// The uncommitted-state line prints only when something under
	// .tiller/ is actually dirty, the same show-only-what-is-there rule Order
	// above already follows; it is informational, so it never gates.
	if len(s.UncommittedState) > 0 {
		fmt.Fprintf(&b, "\nuncommitted tiller state: %d file(s) under .tiller/; commit when convenient",
			len(s.UncommittedState))
	}
	return b.String()
}
