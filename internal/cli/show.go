package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/spf13/cobra"
)

// This file implements `tiller show <noun> <node>`, the full-node read
// (cli-surface.md, "Inspecting and maintaining"). It is a thin shell: validate
// the noun, resolve the address through internal/core/resolve, and render the
// node as the stable JSON envelope. It also holds the small view types and node
// extractors that ls and the typed status oracle reuse, so the three read
// commands print one coherent shape.

// The read-view shapes and their projections live in internal/views, one
// source of truth the CLI's --json envelopes and the serve console's API both
// render (serve pass 1). The aliases keep this package's renderers and tests
// reading naturally.
type (
	NodeView            = views.NodeView
	GateView            = views.GateView
	CriterionStatusView = views.CriterionStatusView
	EvidenceView        = views.EvidenceView
	AttestationView     = views.AttestationView
	PolicyViolationView = views.PolicyViolationView
	BasisView           = views.BasisView
	VerifiedAgainstView = views.VerifiedAgainstView
	SurfaceView         = views.SurfaceView
)

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <noun> <node>",
		Short: "Print a node in full.",
		Long: "show resolves a node by id or canonical key and prints its full state.\n" +
			"The noun is required and validated against the node it names.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args[0], args[1])
		},
	}
}

func runShow(cmd *cobra.Command, noun, node string) error {
	// An amendment's full render is its own shape (title, base revision, the
	// staged operations with LIVE precondition validity), not views.Node's
	// epic/feature/task/question projection, so it is intercepted here rather
	// than threaded through resolveNodeWithTree and renderNodeHuman below.
	if noun == store.NounAmendment {
		return runShowAmendment(cmd, node)
	}
	tree, n, err := resolveNodeWithTree(cmd, "show", noun, node)
	if err != nil {
		return err
	}
	view := views.Node(tree, n)
	view.Ref = gen.Ref(tree, n)
	return emitSuccess(cmd, "show", view, renderNodeHuman(tree, view))
}

// resolveNode is the shared front half of the node read commands: it validates
// the noun, loads the tree, and resolves the address to one node. It already
// emits the failure envelope on any error and returns the wrapped exitError, so
// a caller just propagates a non-nil error.
func resolveNode(cmd *cobra.Command, command, noun, node string) (store.Node, error) {
	_, n, err := resolveNodeWithTree(cmd, command, noun, node)
	return n, err
}

// resolveNodeWithTree is resolveNode that also returns the loaded tree, for a
// read that derives a value from sibling nodes. It emits the failure envelope
// on any error, so a caller just propagates a non-nil error.
func resolveNodeWithTree(cmd *cobra.Command, command, noun, node string) (*store.Tree, store.Node, error) {
	sing, ok := singularNoun(noun)
	if !ok {
		return nil, nil, emitFailure(cmd, command, &usageError{
			reason: "unknown_noun",
			msg:    fmt.Sprintf("unknown noun %q (want one of epic, feature, task, question, amendment, bug, chore, tweak)", noun),
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: sing, Raw: node})
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	return tree, n, nil
}

// singularNoun reports whether noun is one of the seven addressable node
// classes the read commands accept, the validated set the grammar's checksum
// rests on. amendment joined the set in task 4 (`tiller status amendment
// <addr>`, `tiller ls amendment`) and bug and chore with the shelves
// (2026-08-28); show's own amendment arm (runShow above)
// intercepts before reaching here, since an amendment's full render is its
// own shape rather than views.Node's.
func singularNoun(noun string) (string, bool) {
	if sing, ok := pluralNouns[noun]; ok {
		noun = sing
	}
	switch noun {
	case store.NounEpic, store.NounFeature, store.NounTask, store.NounQuestion, store.NounAmendment,
		store.NounBug, store.NounChore, store.NounTweak:
		return noun, true
	default:
		return "", false
	}
}

// The node projection itself is views.Node; this file keeps only the shell
// and the plain-text renderers.

// keyOf forwards to the shared projection; kept as a local name because half
// the package reads it.
func keyOf(n store.Node) string { return views.Key(n) }

// renderNodeHuman is the plain-text view of a node for a terminal reader. Beyond
// the identity and status it surfaces the integrity block a human audits through
// the CLI: the recorded supersession, the recorded gates (with the evals gate's
// per-criterion statuses), the human attestations, the merge-time
// policy_violations, and the accepted/verified basis. Each block is shown only
// when present, so a fresh draft stays terse.
//
// tree is threaded in for the supersession block alone, which prints the
// reference convention (key path with the id in parentheses) rather than the
// bare ids the payload carries, and only the tree can resolve one id into that.
func renderNodeHuman(tree *store.Tree, v NodeView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", v.Noun, v.Ref)
	if v.Noun == store.NounQuestion && v.Title != "" {
		fmt.Fprintf(&b, ": %q", v.Title)
	}
	fmt.Fprint(&b, "\n")
	if v.Title != "" && v.Noun != store.NounQuestion {
		fmt.Fprintf(&b, "  title:  %s\n", v.Title)
	}
	if v.Statement != "" {
		fmt.Fprintf(&b, "  statement: %s\n", v.Statement)
	}
	fmt.Fprintf(&b, "  status: %s", v.Status)
	if v.Activity == gen.ActivityCarrierCancelled {
		// The one derived phase this view renders, and it earns the line: the
		// carried-by line below still names a carrier, and without this the
		// reader would take a bug parked behind cancelled work for one on its
		// way to done.
		fmt.Fprint(&b, " (carrier cancelled)")
	}
	if v.Owner != "" {
		fmt.Fprintf(&b, "\n  owner:  %s", v.Owner)
	}
	if v.Contributors != nil {
		ids := make([]string, len(v.Contributors))
		for i, contributor := range v.Contributors {
			ids[i] = contributor.ID
		}
		label := strings.Join(ids, ", ")
		if label == "" {
			label = "(none)"
		}
		fmt.Fprintf(&b, "\n  contributors: %s", label)
	}
	if v.AttachedTo != "" {
		fmt.Fprintf(&b, "\n  attached to: %s", gen.RefByID(tree, v.AttachedTo))
	}
	if v.ResolvedBy != "" {
		fmt.Fprintf(&b, "\n  carried by: %s", gen.RefByID(tree, v.ResolvedBy))
	}
	if v.Resolution != "" {
		fmt.Fprintf(&b, "\n  resolution: %s", v.Resolution)
		if v.DuplicateOf != "" {
			fmt.Fprintf(&b, " (duplicate of %s)", gen.RefByID(tree, v.DuplicateOf))
		}
	}
	if v.Blocked {
		fmt.Fprint(&b, "\n  blocked")
	}
	renderSupersessionHuman(&b, tree, v)
	renderOrderHuman(&b, v)
	if v.ReviewMissing {
		fmt.Fprintf(&b, "\n  %s", workflow.MissingReviewNotice)
	}
	if v.RecoveredReview != nil {
		fmt.Fprintf(&b, "\n  %s", workflow.RecoverySummary(v.RecoveredReview))
	}
	renderGatesHuman(&b, v.Gates)
	renderAttestationsHuman(&b, v.Attestations)
	renderPolicyViolationsHuman(&b, v.PolicyViolations, v.Gates)
	renderBasisHuman(&b, v.AcceptedAgainst, v.VerifiedAgainst)
	return b.String()
}

// renderSupersessionHuman writes the two directions of a recorded supersession,
// each side named in the reference convention. It is what lets a human reading a
// retired node through the CLI answer "what replaced this" without opening the
// amendment that decided it, the structural answer the schema promises from
// either end (artifact-schema.md, Evolving a project). A node in no supersession
// prints nothing.
func renderSupersessionHuman(b *strings.Builder, tree *store.Tree, v NodeView) {
	if len(v.Supersedes) > 0 {
		refs := make([]string, 0, len(v.Supersedes))
		for _, target := range v.Supersedes {
			refs = append(refs, gen.RefByID(tree, target))
		}
		fmt.Fprintf(b, "\n  supersedes: %s", strings.Join(refs, ", "))
	}
	if v.SupersededBy != "" {
		fmt.Fprintf(b, "\n  superseded_by: %s", gen.RefByID(tree, v.SupersededBy))
	}
}

// renderOrderHuman writes the recorded soft order as one line of refs, in the
// recorded sequence (never sorted: the point of the list is the stated
// sequence itself). It is carriage only, the same show-only-what-is-there
// rule renderSupersessionHuman above already follows: an epic or feature that
// never ran `tiller order`, and every other noun, which carries no children
// to sequence, prints nothing.
func renderOrderHuman(b *strings.Builder, v NodeView) {
	if len(v.OrderRefs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n  order: %s", strings.Join(v.OrderRefs, ", "))
}

// renderGatesHuman writes the recorded gates in name order, each with its status,
// and indents the evals gate's per-criterion statuses beneath it.
func renderGatesHuman(b *strings.Builder, gates map[string]GateView) {
	if len(gates) == 0 {
		return
	}
	fmt.Fprint(b, "\n  gates:")
	names := make([]string, 0, len(gates))
	for name := range gates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := gates[name]
		fmt.Fprintf(b, "\n    %s: %s", name, g.Status)
		critIDs := make([]string, 0, len(g.Criteria))
		for id := range g.Criteria {
			critIDs = append(critIDs, id)
		}
		sort.Strings(critIDs)
		for _, id := range critIDs {
			fmt.Fprintf(b, "\n      %s: %s", id, g.Criteria[id].Status)
		}
	}
}

// renderAttestationsHuman writes the recorded human attestations, one per line.
func renderAttestationsHuman(b *strings.Builder, atts []AttestationView) {
	if len(atts) == 0 {
		return
	}
	fmt.Fprint(b, "\n  attestations:")
	for _, a := range atts {
		fmt.Fprintf(b, "\n    %s %s %s by %s", a.TargetKind, a.TargetID, a.Status, a.By)
	}
}

// renderPolicyViolationsHuman writes the merge-time policy violations, one per
// line. A policy_violations entry is a merge-time record (store.PolicyViolation:
// a required gate that was not green when the node crossed to done under
// advisory enforcement) and is never rewritten after the fact, so a gate that
// has since gone green leaves the violation reading exactly as it did at the
// crossing: "evals: fail" beside a gates block that now reads "evals: pass"
// with nothing saying the two are consistent, not contradictory (issue #44).
//
// gates is the same view's own Gates map (NodeView carries both together), so
// when a violation names a gate whose CURRENT status here is pass, the line
// gains a suffix dating the record to the crossing. The comparison is
// render-local: no store or schema change, and the JSON payload is untouched.
func renderPolicyViolationsHuman(b *strings.Builder, pvs []PolicyViolationView, gates map[string]GateView) {
	if len(pvs) == 0 {
		return
	}
	fmt.Fprint(b, "\n  policy_violations:")
	for _, pv := range pvs {
		fmt.Fprintf(b, "\n    %s: %s", pv.Gate, pv.Status)
		if pv.SourceRevision != "" {
			fmt.Fprintf(b, " (historical source %s)", pv.SourceRevision)
		}
		if len(pv.UnresolvedCriteria) > 0 {
			fmt.Fprintf(b, "; unresolved criteria: %s", strings.Join(pv.UnresolvedCriteria, ", "))
		}
		if len(pv.EvidenceRefs) > 0 {
			fmt.Fprintf(b, "; evidence: %s", strings.Join(pv.EvidenceRefs, ", "))
		}
		if gates[pv.Gate].Status == store.GateStatusPass {
			fmt.Fprint(b, " (recorded at the crossing; gate now pass)")
		}
	}
}

// renderBasisHuman writes the accepted and verified basis pointers when present.
//
// The eval leg is written only when the node carries one, the same show-only-
// what-is-there rule every renderer above follows. The acceptance contract
// enters the tree at feature grain (artifact-schema.md, Gates), so a task's
// accepted_against has a spec leg and no eval leg, and an epic's completion
// record is its prose and its surfaces; both used to render a dangling
// "eval " with nothing after it.
func renderBasisHuman(b *strings.Builder, accepted *BasisView, verified *VerifiedAgainstView) {
	if accepted != nil {
		fmt.Fprintf(b, "\n  accepted_against: spec %s", accepted.Spec)
		if accepted.Eval != "" {
			fmt.Fprintf(b, ", eval %s", accepted.Eval)
		}
	}
	if verified != nil {
		fmt.Fprintf(b, "\n  verified_against: spec %s", verified.Spec)
		if verified.Eval != "" {
			fmt.Fprintf(b, ", eval %s", verified.Eval)
		}
		fmt.Fprintf(b, ", %d surface(s)", len(verified.Surfaces))
	}
}
