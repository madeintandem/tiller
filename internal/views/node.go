// Package views holds the pure read projections: store.Tree and store.Node in,
// stable JSON view structs out. The CLI's --json envelopes and the serve
// console's API render these one source of truth, which is what keeps the two
// surfaces honest (ui-surface.md, "How it maps to the CLI"). Everything here
// is a pure function of loaded state; no reasoning, no mutation, no rendering.
// The shapes were lifted from internal/cli intact, pinned byte-for-byte by the
// CLI's JSON goldens (plan 2026-07-05, serve pass 1).
package views

import (
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// NodeView is the full-node JSON payload of `show`. It is one stable shape
// across every noun: fields that do not apply to a noun are omitted, so an epic
// carries no change_type and a task no key. Later phases extend it additively
// (a feature's verified_against at done, a task's evidence) by adding fields,
// never by changing the ones here. The order of fields is the print order.
//
// The integrity surfaces a human audits live here too: the gates block (each
// gate's status plus the evals gate's per-criterion map), the human
// attestations, the merge-time policy_violations, and the accepted/verified
// basis. They are omitted when empty, so a fresh draft shows none of them and
// a node at done shows all that apply.
type NodeView struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	// Activity is the derived phase this node carries: a feature's rollup
	// phase (awaiting_check / awaiting_sync, gen.FeatureActivity), an epic's
	// derived active/idle working status (gen.EpicActivity), or a bug's or
	// chore's carrier phase (carried / carrier_cancelled, gen.WorkActivity),
	// the same answer the status oracle and the list row give
	// (views.activity). Empty for a task, a question, and wherever the noun's
	// phase does not apply.
	Activity     string              `json:"activity,omitempty"`
	Blocked      bool                `json:"blocked,omitempty"`
	Owner        string              `json:"owner,omitempty"`
	Started      bool                `json:"started,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
	ChangeType   string              `json:"change_type,omitempty"`   // feature
	Epic         string              `json:"epic,omitempty"`          // feature: owning epic key
	Feature      string              `json:"feature,omitempty"`       // task: parent feature id
	AttachedTo   string              `json:"attached_to,omitempty"`   // bug, chore: where it was found (provenance)
	ResolvedBy   string              `json:"resolved_by,omitempty"`   // bug, chore: the carrier whose merge fixes it
	Resolution   string              `json:"resolution,omitempty"`    // bug, chore: on cancelled only
	DuplicateOf  string              `json:"duplicate_of,omitempty"`  // bug, chore: with resolution duplicate
	Statement    string              `json:"statement,omitempty"`     // question
	Answer       *string             `json:"answer,omitempty"`        // question: nil until answered
	CancelReason *string             `json:"cancel_reason,omitempty"` // question: set only when cancelled
	Branch       string              `json:"branch,omitempty"`        // feature
	PR           *int                `json:"pr,omitempty"`            // task; epic, feature at coarser review grain
	// ReviewGrain is the review grain in force for work under this node
	// (store.ResolvedReviewGrain): "task", "feature", or "epic". Present for
	// an epic and a feature, the two review-grain homes the design names
	// besides the project default (docs/plans/2026-08-26-review-grain-design.md,
	// "The setting"); absent for every other noun, none of which owns work of
	// its own to resolve a grain for. ReviewGrainSource names which of the
	// three homes the resolved value actually came from: "feature", "epic",
	// "project" (the manifest's execution.review_grain), or "default" when
	// none of the three is set and the value is the hardcoded task fallback.
	ReviewGrain       string                `json:"review_grain,omitempty"`
	ReviewGrainSource string                `json:"review_grain_source,omitempty"`
	DependsOn         []string              `json:"depends_on,omitempty"`
	Gates             map[string]GateView   `json:"gates,omitempty"`
	Attestations      []AttestationView     `json:"attestations,omitempty"`
	PolicyViolations  []PolicyViolationView `json:"policy_violations,omitempty"`
	AcceptedAgainst   *BasisView            `json:"accepted_against,omitempty"` // feature, task, bug, chore
	VerifiedAgainst   *VerifiedAgainstView  `json:"verified_against,omitempty"` // feature, epic
	// DoneBasis is what the node's done crossing rested on (store's own
	// vocabulary: ancestry, content, human_attested, carried for a task or a
	// shelf node; rollup for a feature or an epic), empty until the node is
	// done. ProposedBy is who captured a task or a shelf node at add, the only
	// person a still-proposed one has. Both were stored and never sent until
	// the node header pass (2026-08-30) needed them beside the state.
	DoneBasis  string `json:"done_basis,omitempty"`  // epic, feature, task, bug, chore
	ProposedBy string `json:"proposed_by,omitempty"` // task, bug, chore
	// BranchURL is the branch's page on the origin host, serve-filled like
	// PRURL (GitHub origin only, absent otherwise); the CLI never emits it.
	BranchURL string `json:"branch_url,omitempty"`

	Source          string                    `json:"source,omitempty"`     // epic, feature, question
	Surface         *SurfaceInfoView          `json:"surface,omitempty"`    // epic, feature
	ReviewRef       *ReviewRefView            `json:"review_ref,omitempty"` // task; epic, feature at coarser review grain
	RecoveredReview *workflow.RecoveredReview `json:"recovered_review,omitempty"`
	ReviewMissing   bool                      `json:"review_missing,omitempty"`
	PRURL           string                    `json:"pr_url,omitempty"` // serve fills; CLI never emits

	// Blocks is a question's block targets, id plus key (BlockRef), resolved
	// through the SAME relatedByIDs computation the node page's own
	// Relationships.Blocks calls (nodepage.go's relationshipsOf, *store.Question
	// arm) rather than a second lookup that could drift from it. This flat
	// field is the raw claim (what the question, on its own record, holds);
	// Relationships.Blocks is the resolved graph edge the node page's hub
	// walks (full RelatedNode: noun, ref, status, title). Both carrying refs on
	// the same page is fine, because they answer different questions: this one
	// "what does this question's own record say it holds", that one "show me
	// the adjacent node". Nil, and so omitted, for a question with nothing
	// recorded and for every other noun.
	Blocks []BlockRef `json:"blocks,omitempty"` // question

	// NextOwner is whose move this node is next, "human" or "agent", empty at
	// a terminal state with no legal next verb (views.NextOwner, task A2's
	// projection). It is a pure derivation of the lifecycle engine's
	// legal-next verb set and its verb-actor table: every noun carries it.
	NextOwner string `json:"next_owner,omitempty"`

	// Ref is the node's human reference ("<key-path> (<id>)", gen.Ref) for the
	// plain-text renderer only; the JSON payload carries id and key structurally.
	Ref string `json:"-"`

	// Held names the nearest retired ancestor this node sits under (a
	// cancelled feature, or a cancelled-or-archived epic), nil when none
	// (gen.RetiredAncestor). It is additive and orthogonal to LegalNext and
	// NextOwner above, which stay pure lifecycle projections per the
	// "annotate, don't subtract" decision (issue #36): the doing verbs
	// already refuse under a retired ancestor (mutate.guardParentState);
	// this field says why, before a human runs the verb that would refuse.
	Held *HeldView `json:"held,omitempty"`

	// Supersedes and SupersededBy record replacement lineage for features,
	// tasks, bugs, and chores. The envelope carries immutable IDs; readers
	// resolve them against the same snapshot.
	//
	// They are here because the schema's promise about a reshape is that the
	// tree answers it structurally from either end, "what replaced this" from
	// the retired node and "what did this replace" from the replacement
	// (artifact-schema.md, Evolving a project). The console's node page
	// projected both from the start; this is the CLI read catching up, so a
	// human auditing a reshape through `tiller show` gets the same answer.
	// Appended at the end, the same additive way Held was.
	Supersedes   []string `json:"supersedes,omitempty"`    // feature, task, bug, chore
	SupersededBy string   `json:"superseded_by,omitempty"` // feature, task, bug, chore

	// Order is the recorded soft order of this node's own children: an
	// epic's feature sequence or a feature's task sequence (store.Epic.Order,
	// store.Feature.Order; docs/plans/2026-08-08-soft-order-design.md). This
	// is carriage only: the field states what was recorded, verbatim and
	// possibly partial, never the full computed sibling order
	// gen.DependencyOrder already renders through
	// Relationships.Features/Tasks. Absent for a task, a question, and an
	// amendment (leaf or childless nouns with nothing to sequence), and for
	// an epic or feature that never ran `tiller order`.
	Order []string `json:"order,omitempty"` // epic: feature ids; feature: task ids
	// OrderRefs carries Order's targets as human references, the same
	// machine-ids-plus-human-refs pairing streams.go's
	// BlockedItem.DependsOn/DependsOnRefs already uses, index-aligned to
	// Order and preserving its recorded sequence rather than sorting by
	// reference: the point of a soft order is the stated sequence itself. A
	// dangling id degrades to its bare id (gen.RefByID's own rule) rather
	// than disappearing.
	OrderRefs []string `json:"order_refs,omitempty"`
}

// BlockRef is one target a question's Blocks names: the minimal id-plus-key
// pair, deliberately lighter than RelatedNode (nodepage.go), which also
// carries noun, a composed human ref, status, and title. That fuller shape is
// the node page's Relationships.Blocks job (the browsable graph edge); this
// flat field only needs enough to say what the question's own record points
// at, so it stays a raw claim rather than dragging the whole related-node
// weight into every `show question` read. Key is empty for a dangling id
// (relatedByIDs' own degrade rule, blockRefsOf below), never a synthesized
// placeholder.
type BlockRef struct {
	ID  string `json:"id"`
	Key string `json:"key,omitempty"`
}

// blockRefsOf resolves a question's recorded Blocks ids to BlockRef rows,
// through relatedByIDs (nodepage.go), the IDENTICAL resolution
// Relationships.Blocks already performs for the same list
// (relationshipsOf's *store.Question arm): one computation, reused here
// rather than a second lookup that could disagree with it. nil for an empty
// list, so NodeView.Blocks' omitempty drops the key entirely.
func blockRefsOf(t *store.Tree, ids []string) []BlockRef {
	related := relatedByIDs(t, ids)
	if len(related) == 0 {
		return nil
	}
	out := make([]BlockRef, len(related))
	for i, r := range related {
		out[i] = BlockRef{ID: r.ID, Key: r.Key}
	}
	return out
}

// HeldView is the read envelope's face of gen.Held: the nearest retired
// ancestor a live node sits under, named the same noun-id-ref way
// RelatedNode names an adjacent node, plus that ancestor's own retired state
// ("cancelled" or "archived").
type HeldView struct {
	Noun  string `json:"noun"`
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	State string `json:"state"`
}

// heldViewOf projects a gen.Held onto the read envelope, nil through to nil
// so NodeView.Held's omitempty drops the key entirely rather than a null.
func heldViewOf(h *gen.Held) *HeldView {
	if h == nil {
		return nil
	}
	return &HeldView{Noun: h.Noun, ID: h.ID, Ref: h.Ref, State: h.State}
}

// GateView is one recorded gate result in the read envelope, the JSON face of
// store.Gate (artifact-schema.md, The stored gate block). The against revision
// is rendered as its compact token, and the run timestamp as RFC3339 in UTC, so
// the shape is stable to golden against. A command gate carries its own
// Evidence (the command's recorded output), while the evals gate additionally
// carries its per-criterion Criteria map, the per-criterion statuses (pass,
// fail, pending, needs_attestation, stale) and their recorded evidence, so a
// human auditing sees exactly which acceptance criterion is and is not
// satisfied.
type GateView struct {
	Required bool                           `json:"required"`
	Kind     string                         `json:"kind,omitempty"`
	Status   string                         `json:"status,omitempty"`
	RunBy    string                         `json:"run_by,omitempty"`
	RunAt    string                         `json:"run_at,omitempty"`
	Against  string                         `json:"against,omitempty"`
	Evidence []EvidenceView                 `json:"evidence,omitempty"`
	Criteria map[string]CriterionStatusView `json:"criteria,omitempty"`
}

// CriterionStatusView is one eval criterion's recorded result under the evals
// gate's criteria map: its status and the evidence trail behind it.
type CriterionStatusView struct {
	Status   string         `json:"status,omitempty"`
	Evidence []EvidenceView `json:"evidence,omitempty"`
}

// EvidenceView is a typed pointer to a stored artifact (a command output, a
// recorded rubric output), the JSON face of store.Evidence.
type EvidenceView struct {
	Kind string `json:"kind,omitempty"`
	Ref  string `json:"ref,omitempty"`
	// URL and Unavailable are filled only by the console for source-qualified evidence.
	URL         string `json:"url,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}

// AttestationView is one recorded human sign-off in the read envelope, the JSON
// face of store.Attestation: the target it attested, the status, who and when,
// and the evidence it points back at (empty for a manual sign-off with no
// artifact).
type AttestationView struct {
	ID          string `json:"id"`
	TargetKind  string `json:"target_kind,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	Status      string `json:"status,omitempty"`
	By          string `json:"by,omitempty"`
	At          string `json:"at,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

// PolicyViolationView is one merge-time policy violation in the read envelope,
// the JSON face of store.PolicyViolation: a required gate that was not green
// when the node crossed to done under advisory enforcement.
type PolicyViolationView struct {
	Gate               string   `json:"gate,omitempty"`
	Observed           string   `json:"observed,omitempty"`
	Status             string   `json:"status,omitempty"`
	EvidenceRef        string   `json:"evidence_ref,omitempty"`
	SourceRevision     string   `json:"source_revision,omitempty"`
	UnresolvedCriteria []string `json:"unresolved_criteria,omitempty"`
	EvidenceRefs       []string `json:"evidence_refs,omitempty"`
}

// BasisView is the spec/eval revision pair a feature was accepted against, the
// JSON face of store.BasisRefs (accepted_against).
type BasisView struct {
	Spec string `json:"spec,omitempty"`
	Eval string `json:"eval,omitempty"`
	// Commit is HEAD at approval (store.BasisRefs.Commit), the code baseline
	// the plan was approved against. Spec and Eval are blob fingerprints of
	// the two files, which no hosting page can show; the commit is the one
	// revision on a node page a reader can follow to the repository, which is
	// why the node header pass (2026-08-30) put it on the wire.
	Commit string `json:"commit,omitempty"`
	// CommitURL is the commit's page on the origin host, serve-filled the
	// same way NodeView.PRURL is (a GitHub origin only, absent otherwise);
	// the CLI never emits it.
	CommitURL string `json:"commit_url,omitempty"`
}

// VerifiedAgainstView is the completion record set at done, the JSON face of
// store.VerifiedAgainst: the verified spec and eval revisions plus the
// code-surface fingerprints. Surfaces is always present (the empty list for a
// create feature), matching the stored schema.
type VerifiedAgainstView struct {
	Spec string `json:"spec,omitempty"`
	Eval string `json:"eval,omitempty"`
	// Commit is HEAD at the done crossing (store.VerifiedAgainst.Commit); see
	// BasisView.Commit for why it rides the wire and CommitURL for the link.
	Commit    string        `json:"commit,omitempty"`
	CommitURL string        `json:"commit_url,omitempty"`
	Surfaces  []SurfaceView `json:"surfaces"`
}

// SurfaceView is one verified surface fingerprint: the path, the fingerprint
// kind, and its value token.
type SurfaceView struct {
	Path  string `json:"path,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value,omitempty"`
}

// SurfaceInfoView is the existing code a node touches, the JSON face of
// store.Surface: empty for a greenfield create, populated for a brownfield
// modify/replace/migrate (artifact-schema.md, change_type and surface).
type SurfaceInfoView struct {
	Paths []string `json:"paths,omitempty"`
}

// ReviewRefView is the offline review target recorded on a task at submit when
// the repository has no git remote, the JSON face of store.ReviewRef: the branch
// the change sits on and its head commit at submission. Head is the stored
// compact token ("git:e5f9d28") verbatim; with a remote, PR is the review target
// instead, so a task never carries both (artifact-schema.md, Tasks).
type ReviewRefView struct {
	Branch string `json:"branch,omitempty"`
	Head   string `json:"head,omitempty"`
}

// Node projects each noun's stored fields and snapshot-derived context.
func Node(tree *store.Tree, n store.Node) NodeView {
	v := NodeView{
		Noun:         n.Noun(),
		ID:           n.NodeID(),
		Key:          Key(n),
		Status:       n.NodeStatus(),
		Activity:     activity(tree, n),
		Contributors: Contributors(tree, n),
	}
	v.RecoveredReview, v.ReviewMissing = workflow.ReviewStateOf(tree, n)
	switch t := n.(type) {
	case *store.Epic:
		// No gates, attestations, or policy violations: an epic carries no gate
		// block, because its acceptance bar is the sum of its features'
		// contracts (artifact-schema.md, Epics). verified_against is the one
		// record it does keep, written by sync at the rollup crossing.
		v.Title = t.Title
		v.Blocked = t.Blocked
		v.DependsOn = t.DependsOn
		v.Order = t.Order
		v.VerifiedAgainst = VerifiedAgainstOf(t.VerifiedAgainst)
		v.Source = t.Source
		v.Branch = t.Branch
		v.Surface = SurfaceOf(t.Surface)
		v.DoneBasis = t.DoneBasis
		v.ReviewGrain, v.ReviewGrainSource = reviewGrainOf(tree, t, nil)
		v.PR = t.PR
		v.ReviewRef = ReviewRefOf(t.ReviewRef)
	case *store.Feature:
		v.Title = t.Title
		v.Blocked = t.Blocked
		v.ChangeType = t.ChangeType
		v.Epic = t.EpicKey
		v.Branch = t.Branch
		v.DependsOn = t.DependsOn
		v.Order = t.Order
		v.Supersedes = t.Supersedes
		v.SupersededBy = deref(t.SupersededBy)
		v.Gates = Gates(t.GatesBlock)
		v.Attestations = Attestations(t.GatesBlock)
		v.PolicyViolations = PolicyViolations(t.GatesBlock)
		v.AcceptedAgainst = BasisOf(t.AcceptedAgainst)
		v.VerifiedAgainst = VerifiedAgainstOf(t.VerifiedAgainst)
		v.Source = t.Source
		v.Surface = SurfaceOf(t.Surface)
		v.DoneBasis = t.DoneBasis
		v.ReviewGrain, v.ReviewGrainSource = reviewGrainOf(tree, recapEpicByKey(tree, t.EpicKey), t)
		v.PR = t.PR
		v.ReviewRef = ReviewRefOf(t.ReviewRef)
	case *store.Task:
		v.Title = t.Title
		v.Blocked = t.Blocked
		v.Owner = t.Owner
		v.Started = t.Started
		v.Feature = t.FeatureID
		v.Branch = t.Branch
		v.DependsOn = t.DependsOn
		v.Supersedes = t.Supersedes
		v.SupersededBy = deref(t.SupersededBy)
		v.PR = t.PR
		v.Gates = Gates(t.GatesBlock)
		v.Attestations = Attestations(t.GatesBlock)
		v.PolicyViolations = PolicyViolations(t.GatesBlock)
		if t.AcceptedAgainst != nil {
			v.AcceptedAgainst = BasisOf(*t.AcceptedAgainst)
		}
		v.DoneBasis = t.DoneBasis
		v.ProposedBy = t.ProposedBy
		v.ReviewRef = ReviewRefOf(t.ReviewRef)
	case *store.Question:
		v.Title = gen.QuestionTitle(t)
		v.Owner = t.Owner
		v.Statement = t.Statement
		v.Answer = t.Answer
		if t.CancelReason != "" {
			reason := t.CancelReason
			v.CancelReason = &reason
		}
		v.Source = t.Source
		v.Blocks = blockRefsOf(tree, t.Blocks)
	case *store.Amendment:
		// An amendment carries no owner, no gate block, and no parentage the
		// way a folder-backed node does (store/amendment.go's own doc
		// comment: "it carries no parent linkage and no gates block"); its
		// title is the one field this switch has to fill for it.
		v.Title = t.Title
	default:
		if w, ok := store.AsWorkNode(n); ok {
			b := w.Body()
			v.Title = b.Title
			v.Blocked = b.Blocked
			v.Owner = b.Owner
			v.Started = b.Started
			v.AttachedTo = b.AttachedTo
			v.ResolvedBy = b.ResolvedBy
			v.Resolution = b.Resolution
			v.DuplicateOf = b.DuplicateOf
			v.DependsOn = b.DependsOn
			v.Supersedes = b.Supersedes
			v.SupersededBy = deref(b.SupersededBy)
			v.Source = b.Source
			v.Branch = b.Branch
			v.PR = b.PR
			v.ReviewRef = ReviewRefOf(b.ReviewRef)
			v.Gates = Gates(b.GatesBlock)
			v.Attestations = Attestations(b.GatesBlock)
			v.PolicyViolations = PolicyViolations(b.GatesBlock)
			if b.AcceptedAgainst != nil {
				v.AcceptedAgainst = BasisOf(*b.AcceptedAgainst)
			}
			v.DoneBasis = b.DoneBasis
			v.ProposedBy = b.ProposedBy
		}
	}
	v.NextOwner = NextOwner(n)
	v.Held = heldViewOf(gen.RetiredAncestor(tree, n))
	v.OrderRefs = orderRefsOf(tree, v.Order)
	return v
}

// orderRefsOf resolves a recorded soft order's ids to human references,
// index-aligned to the input and preserving its recorded sequence rather
// than sorting: the point of the list is the stated sequence itself. nil for
// an empty list, so OrderRefs' omitempty drops the key together with Order.
func orderRefsOf(t *store.Tree, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = gen.RefByID(t, id)
	}
	return refs
}

// Gates projects a stored gate block onto the read envelope, keyed by gate
// name. encoding/json sorts map keys, so the output is deterministic. An empty
// block yields nil, which omitempty drops from the envelope.
func Gates(b store.GatesBlock) map[string]GateView {
	if len(b.Gates) == 0 {
		return nil
	}
	out := make(map[string]GateView, len(b.Gates))
	for name, g := range b.Gates {
		out[name] = GateView{
			Required: g.Required,
			Kind:     g.Kind,
			Status:   g.Status,
			RunBy:    g.RunBy,
			RunAt:    formatTime(g.RunAt),
			Against:  g.Against.Value,
			Evidence: evidenceViews(g.Evidence),
			Criteria: criterionViews(g.Criteria),
		}
	}
	return out
}

// criterionViews projects the evals gate's per-criterion map onto the read
// envelope. encoding/json sorts the map keys, so the output is deterministic.
// An empty map yields nil, dropped by omitempty.
func criterionViews(m map[string]store.Criterion) map[string]CriterionStatusView {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]CriterionStatusView, len(m))
	for id, c := range m {
		out[id] = CriterionStatusView{Status: c.Status, Evidence: evidenceViews(c.Evidence)}
	}
	return out
}

// evidenceViews projects a stored evidence list onto the read envelope,
// preserving order. An empty list yields nil, dropped by omitempty.
func evidenceViews(ev []store.Evidence) []EvidenceView {
	if len(ev) == 0 {
		return nil
	}
	out := make([]EvidenceView, len(ev))
	for i, e := range ev {
		out[i] = EvidenceView{Kind: e.Kind, Ref: e.Ref}
	}
	return out
}

// Attestations projects a node's recorded attestations onto the read envelope,
// preserving their recorded order. An empty list yields nil, dropped by
// omitempty.
func Attestations(b store.GatesBlock) []AttestationView {
	if len(b.Attestations) == 0 {
		return nil
	}
	out := make([]AttestationView, len(b.Attestations))
	for i, a := range b.Attestations {
		out[i] = AttestationView{
			ID:          a.ID,
			TargetKind:  a.Target.Kind,
			TargetID:    a.Target.ID,
			Status:      a.Status,
			By:          a.By,
			At:          formatTime(a.At),
			EvidenceRef: a.EvidenceRef,
		}
	}
	return out
}

// PolicyViolations projects a node's merge-time policy violations onto the
// read envelope, preserving their recorded order. An empty list yields nil,
// dropped by omitempty.
func PolicyViolations(b store.GatesBlock) []PolicyViolationView {
	if len(b.PolicyViolations) == 0 {
		return nil
	}
	out := make([]PolicyViolationView, len(b.PolicyViolations))
	for i, pv := range b.PolicyViolations {
		out[i] = PolicyViolationView{
			Gate:               pv.Gate,
			Observed:           pv.Observed.Value,
			Status:             pv.Status,
			EvidenceRef:        pv.EvidenceRef,
			SourceRevision:     pv.SourceRevision.Value,
			UnresolvedCriteria: pv.UnresolvedCriteria,
			EvidenceRefs:       pv.EvidenceRefs,
		}
	}
	return out
}

// BasisOf projects a feature's accepted_against onto the read envelope,
// returning nil when no basis was recorded (the block is then omitted).
func BasisOf(b store.BasisRefs) *BasisView {
	if b.Spec.Value == "" && b.Eval.Value == "" && b.Commit.Value == "" {
		return nil
	}
	return &BasisView{Spec: b.Spec.Value, Eval: b.Eval.Value, Commit: b.Commit.Value}
}

// VerifiedAgainstOf projects a node's verified_against onto the read envelope,
// returning nil when no completion record was set (a node that never reached
// done). When materialized it always carries surfaces as a list, the empty
// list for a create feature that touched no surface.
func VerifiedAgainstOf(v store.VerifiedAgainst) *VerifiedAgainstView {
	if v.Spec.Value == "" && v.Eval.Value == "" && len(v.Surfaces) == 0 {
		return nil
	}
	surfaces := make([]SurfaceView, 0, len(v.Surfaces))
	for _, s := range v.Surfaces {
		surfaces = append(surfaces, SurfaceView{Path: s.Path, Kind: s.Kind, Value: s.Value})
	}
	return &VerifiedAgainstView{Spec: v.Spec.Value, Eval: v.Eval.Value, Commit: v.Commit.Value, Surfaces: surfaces}
}

// SurfaceOf projects a node's work surface onto the read envelope, returning
// nil when no paths were recorded (a greenfield create then omits surface
// whole rather than printing an empty list).
func SurfaceOf(s store.Surface) *SurfaceInfoView {
	if len(s.Paths) == 0 {
		return nil
	}
	return &SurfaceInfoView{Paths: s.Paths}
}

// ReviewRefOf projects a node's offline review target onto the read envelope,
// returning nil when none was recorded. Head carries the stored RevisionRef's
// compact token verbatim (already prefixed "git:"), so no reformatting is
// needed here.
func ReviewRefOf(rr *store.ReviewRef) *ReviewRefView {
	if rr == nil {
		return nil
	}
	return &ReviewRefView{Branch: rr.Branch, Head: rr.Head.Value}
}

// reviewGrainOf resolves the review grain in force for work under e or f
// together with which of the three homes it came from, by consuming
// store.ResolvedReviewGrainWithSource directly rather than re-deriving the
// source from the same fields in a second switch (Fix round 1, Important 1:
// the two used to be a parallel re-derivation that could silently fall out
// of step if the store's own precedence ever changed without a matching edit
// here). e or f may be nil (an epic's own call passes f nil; a feature with
// no resolvable epic passes e nil), and tree.Manifest may be nil too, on the
// same zero-tree fixtures the store function itself already tolerates.
func reviewGrainOf(tree *store.Tree, e *store.Epic, f *store.Feature) (grain string, source string) {
	return store.ResolvedReviewGrainWithSource(tree.Manifest, e, f)
}

// formatTime renders a timestamp as RFC3339 in UTC, or empty for the zero time
// so an unrun gate carries no run_at.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Key returns a node's canonical key, or "" when the node carries none.
func Key(n store.Node) string {
	if k := n.NodeKey(); k != nil {
		return *k
	}
	return ""
}

// deref reads an optional string field as a plain value, nil as "". It is what
// lets SupersededBy be a *string on disk (a node in no supersession carries
// none, a different fact from carrying an empty one) and a plain string in the
// envelope, where omitempty draws that same distinction.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Contributors reads the snapshot projection only for ancestor nodes. A
// nonnil empty slice distinguishes an ancestor without contributions from a
// noun where contribution rollups do not apply.
func Contributors(tree *store.Tree, n store.Node) []store.Contributor {
	if n.Noun() != store.NounEpic && n.Noun() != store.NounFeature {
		return nil
	}
	if owners := tree.Contributors[n.NodeID()]; owners != nil {
		return owners
	}
	return []store.Contributor{}
}

// Owner reads a node's owner across the concrete types.
func Owner(n store.Node) string {
	switch t := n.(type) {
	case *store.Task:
		return t.Owner
	case *store.Question:
		return t.Owner
	default:
		if w, ok := store.AsWorkNode(n); ok {
			return w.Body().Owner
		}
	}
	return ""
}

// Blocked reads the orthogonal blocked flag, carried by epic, feature, and task.
func Blocked(n store.Node) bool {
	switch t := n.(type) {
	case *store.Epic:
		return t.Blocked
	case *store.Feature:
		return t.Blocked
	case *store.Task:
		return t.Blocked
	default:
		if w, ok := store.AsWorkNode(n); ok {
			return w.Body().Blocked
		}
	}
	return false
}

// NodeGates reads the recorded gate block, carried on the two grains that have
// one, feature and task. An epic has no gate block at all: gates run at the
// grains the manifest's `on` lists name and the acceptance contract enters at
// feature grain (artifact-schema.md, Gates), so an epic answers nil here.
func NodeGates(n store.Node) map[string]GateView {
	switch t := n.(type) {
	case *store.Feature:
		return Gates(t.GatesBlock)
	case *store.Task:
		return Gates(t.GatesBlock)
	default:
		if w, ok := store.AsWorkNode(n); ok {
			return Gates(w.Body().GatesBlock)
		}
	}
	return nil
}

// StatusTitle is the short prose handle the status oracle echoes beside the
// node's reference. Tasks and amendments use their persisted title; questions
// use gen.QuestionTitle so legacy untitled records receive the one shared
// bounded fallback. Epic and feature references already carry their key path.
func StatusTitle(n store.Node) string {
	switch t := n.(type) {
	case *store.Task:
		return t.Title
	case *store.Question:
		return gen.QuestionTitle(t)
	case *store.Amendment:
		return t.Title
	}
	return ""
}

// Statement returns a question's full statement for detail and structured
// review contracts; empty for every other node type.
func Statement(n store.Node) string {
	if q, ok := n.(*store.Question); ok {
		return q.Statement
	}
	return ""
}

// FeatureID reads a task's parent feature id; only tasks carry it.
func FeatureID(n store.Node) string {
	if t, ok := n.(*store.Task); ok {
		return t.FeatureID
	}
	return ""
}

// EpicKey is the key of the epic a node sits under, the axis `ls --epic`
// filters on: a feature's recorded EpicKey, and for a task the first segment of
// its own canonical key, which carries the whole path (artifact-schema.md, Keys
// and paths). An epic answers "" because the filter names a parent and an epic
// has none; so does a question, which sits outside the work tree, and a task
// whose task.yml records no key.
func EpicKey(n store.Node) string {
	switch v := n.(type) {
	case *store.Feature:
		return v.EpicKey
	case *store.Task:
		if v.Key == nil {
			return ""
		}
		if i := strings.Index(*v.Key, "/"); i > 0 {
			return (*v.Key)[:i]
		}
	}
	return ""
}

// NodePolicyViolations exposes recorded crossing warnings without projecting
// or observing the rest of the node.
func NodePolicyViolations(n store.Node) []PolicyViolationView {
	switch node := n.(type) {
	case *store.Feature:
		return PolicyViolations(node.GatesBlock)
	case *store.Task:
		return PolicyViolations(node.GatesBlock)
	default:
		if work, ok := store.AsWorkNode(n); ok {
			return PolicyViolations(work.Body().GatesBlock)
		}
	}
	return nil
}
