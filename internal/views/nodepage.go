package views

// This file derives the console's uniform node page (ui-surface.md, "Node
// detail"): the full node view, its human reference, the relationship hub that
// links every adjacent node, and the lightweight history timeline read from
// the event stream. Every list row, queue item, recap entry, and search hit
// lands here, so it is one consistent surface rather than a layout per type.
// Like everything in views it is a pure function of loaded state.

import (
	"slices"
	"sort"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// NodePage is the /api/node payload: one shape for every noun. Node is exactly
// the full-node view `show --json` renders, untouched; Ref carries the human
// reference explicitly because NodeView keeps its own Ref out of JSON for the
// CLI goldens' sake. Prose points at the node's prose file where one exists
// and is null where none does; the key always serializes, so a renderer
// asks one question. History is always an array, empty rather than null, for
// the same reason. Eval is the node's authored acceptance contract: null,
// EvalContractView, or EvalContractErrorView. This pure projection has no
// filesystem access, so it always leaves Eval nil (serialized "eval": null);
// the serve handler, which already owns the prose file read, loads and
// validates the *.eval.md file at gen.EvalPath and fills this in. Decomposition
// is an epic's full feature breakdown: every child feature projected through
// the same full node view (Node), so an epic's approval review has each
// feature's source, change type, surface, and gates without a second fetch.
// It is present for every epic page, the empty features list for a childless
// epic rather than a null one, and nil, so the JSON omits the key, for every
// other noun.
type NodePage struct {
	Node          NodeView       `json:"node"`
	Ref           string         `json:"ref"`
	Prose         *ProseRef      `json:"prose"`
	Relationships Relationships  `json:"relationships"`
	History       []HistoryEntry `json:"history"`
	Backlinks     BacklinksView  `json:"backlinks"`
	// Eval is nil | EvalContractView | EvalContractErrorView.
	Eval any `json:"eval"`
	// Decomposition is filled for an epic only; nil, and so omitted, for
	// every other noun.
	Decomposition *DecompositionView `json:"decomposition,omitempty"`
	// Amendment is filled for a NounAmendment page only; nil, and so
	// omitted, for every other noun. It carries the staged reshape's own
	// content, the structure NeedsYou and the relationship hub have no place
	// for: every operation, its targets resolved to a RelatedNode where the
	// live tree can, and the two design-doc lists.
	Amendment *AmendmentPageView `json:"amendment,omitempty"`
	// Workflow is node-specific advice, separate from lifecycle moves.
	Workflow *workflow.Advice `json:"workflow,omitempty"`
	// ActiveBasis is the node's drift report, absent when nothing drifted.
	// cli-surface.md ("Specifying a feature") names `serve` alongside `status`
	// and `drift` as a reporter of active_basis, and the console is where the
	// human who has to review the edit and refresh the pin actually sits. Like
	// Eval it is left nil by this pure projection, because the comparison hashes
	// working-tree files through git; the serve handler fills it.
	ActiveBasis *ActiveBasisView `json:"active_basis,omitempty"`
	// Drift is the node's Drifted-chip badges (ui-surface.md, the node-page
	// chip sentence): one entry per DISTINCT marker drift.ForNode found for
	// this node, never one per candidate (DriftBadgesOf's own doc comment).
	// Absent, never an empty array, when nothing drifted, the same
	// omit-rather-than-empty convention ActiveBasis and Decomposition already
	// keep. Like ActiveBasis this is left nil by the pure projection and
	// filled by the serve handler, which is the one place drift.ForNode's own
	// git and filesystem reads are allowed to happen.
	Drift []DriftBadgeView `json:"drift,omitempty"`
	// DriftDetail is the drift panel behind the chip (JC's ask, 2026-08-06):
	// the same ForNode call's candidates at candidate grain, uncovered rows
	// included, beside the one-per-marker badge list above. Filled by the
	// serve handler together with Drift, and nil together with it on a
	// ForNode error, so the chip and the panel it opens can never disagree.
	DriftDetail *NodeDriftView `json:"drift_detail,omitempty"`
	// UnpushedReview names the review-state warning (issue #42) for a task
	// in review whose recorded feature branch holds a commit its PR does not
	// yet show: the identical additive object NodeStatusView.UnpushedReview
	// already carries (views.UnpushedReviewView, summary.go), reused here
	// rather than a second shape for the same fact. Like ActiveBasis and
	// Drift it is left nil by this pure projection, because naming it needs
	// a live git read; the serve handler fills it by calling
	// internal/core/review.ForTask (extracted in 5d4f9ef), the ONE live-git
	// computation every surface that names this warning shares - submit's
	// own confirmation, the task status oracle, the execute readout, and
	// this page - rather than a second, uncentralized copy of the same
	// `tiller status task` read (review.ForTask's own package doc names why
	// it lives in internal/core rather than internal/cli: internal/serve
	// needs the identical read and cannot import internal/cli, which already
	// imports internal/serve for the `serve` command). Absent for every noun
	// but a task, and for a task that is not in review, records no PR, has
	// no recorded feature branch, or whose branch already sits pushed even
	// with its upstream.
	UnpushedReview *UnpushedReviewView `json:"unpushed_review,omitempty"`
}

// DriftBadgeView is one marker's presence on this node's Drifted chip: the
// marker name and whether it currently refuses a verb, the one fact that
// decides the chip's palette register (ui-surface.md, the node-page chip
// sentence). An Uncovered row from the same drift.ForNode call never becomes
// a badge (a badge asserts drift; an uncovered node asserts the check could
// not run), so this type carries no
// reason field the way UncoveredView does.
type DriftBadgeView struct {
	Marker   string `json:"marker"`
	Refusing bool   `json:"refusing"`
}

// DriftBadgesOf projects one drift.ForNode call's candidates onto n's badge
// list: one badge per DISTINCT marker represented, never one per candidate,
// so a task whose active_basis drifted at BOTH its own grain and its parent
// feature's (drift's own TestActiveBasisNamesTaskOwnDriftBesideParentDrift,
// a two-candidate shape for one marker) still folds to the ONE active_basis
// badge the resolution 3 asks the chip for ("ONE chip...
// regardless of how many markers touch the node"). Order follows
// drift.KnownMarkers, the schema's own fixed order every other drift
// projection in this package already renders in (drift.go's own
// DriftReportOf), so the chip's title attribute (built by the console
// ranging over this slice) lists markers in that same order rather than the
// incidental one candidates happened to arrive in.
//
// n is the PAGE's own node, not necessarily any given candidate's own
// Noun/ID: active_basis and answered_question can both name an ancestor
// (mutate.BasisDrift's task-checks-its-parent-first walk; the identical
// walk-up answeredQuestionForNode makes for a task's own assumption review),
// and the Refusing verdict is about what viewing THIS page, at n's own
// current status, actually means for the verbs run from here, never about
// whichever node a candidate happens to name.
func DriftBadgesOf(candidates []drift.Candidate, n store.Node) []DriftBadgeView {
	present := map[string]bool{}
	for _, c := range candidates {
		present[c.Marker] = true
	}
	var badges []DriftBadgeView
	for _, m := range drift.KnownMarkers {
		if present[m] {
			badges = append(badges, DriftBadgeView{Marker: m, Refusing: driftRefusing(m, n)})
		}
	}
	return badges
}

// driftRefusing is the ONE function the spec sentence's rule lives in
// (ui-surface.md, the node-page chip sentence): "the chip takes the blocked
// palette register only where the condition actually refuses verbs,
// active_basis anywhere and an unreviewed answered assumption at a
// submittable task, and the neutral register the stale gates use otherwise."
//
// active_basis always refuses: start, check, and submit all refuse on it
// (artifact-schema.md's own active_basis bullet: "The one marker whose
// condition also refuses verbs"), for every node status the marker's own
// sweep ever reaches, so a badge that exists at all is always refusing here.
//
// answered_question refuses ONLY submit (artifact-schema.md's own
// answered_question bullet: "refuses submit alone"), and only while the node
// is a TASK sitting in the submittable window: in_progress (still working
// toward check) or check (one verb away from submit) both still have a
// submit ahead of them that would refuse today. in_review is deliberately
// OUT of that window even though the underlying feature-level drift can be
// identical: the task has already shipped its claim (a PR is open, or a
// review_ref is recorded), so a blocked read there would flag a fact already
// past tense, and the fix at that point is resume back to in_progress, where
// the badge flags again if the assumption is still unreviewed then. A
// feature page never refuses on this marker either, however drifted: the
// rule names a task specifically, since assumption review only ever gates
// the task verb (submit), never a feature one.
//
// Every other marker (stale_spec, missing_surface, orphan_eval) never
// refuses a verb at all, so it never earns the blocked register, however
// drifted the node reads: the neutral register a stale gate already uses.
func driftRefusing(marker string, n store.Node) bool {
	switch marker {
	case drift.MarkerActiveBasis:
		return true
	case drift.MarkerAnsweredQuestion:
		tk, ok := n.(*store.Task)
		return ok && (tk.Status == "in_progress" || tk.Status == "check")
	default:
		return false
	}
}

// DecompositionView is an epic's full feature breakdown: every child feature
// projected through the same full node view (Node) so an approval review has
// each feature's source, change type, surface, and gates without a second
// fetch. Features has no omitempty: a childless epic still marshals features
// as an empty list, never null, so a renderer never special cases an empty
// decomposition.
type DecompositionView struct {
	Features []NodeView `json:"features"`
}

// ProseRef names a node's prose file: the repo-relative path (the same drill
// target `tree --files` prints) and its content. Views fill Path only, because
// a view is a pure function of loaded state and never reads a file; the serve
// handler fills Content when it renders the page. The folder-backed nodes carry
// one; a question page carries none, its prose handle (the statement and
// answer) already riding on the node itself.
type ProseRef struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// Stub reports that Content is still only the scaffold `tiller add`
	// generated, with nothing written under its headings
	// (mutate.IsProseUnwritten). The console collapses that to a single line
	// naming the move rather than typesetting boilerplate as content. It is
	// strictly narrower than the sentinel rule the `tiller spec feature` gate
	// and the epic acceptance bar refuse on, deliberately: drawing none of a
	// file needs more evidence than refusing a transition does, so a
	// half-written spec that still carries the generated paragraph renders in
	// full here while the gate still refuses it. Like Content, only the handler
	// that read the file can fill it; a view leaves it false.
	Stub bool `json:"stub,omitempty"`
}

// RelatedNode names one adjacent node with enough to render a link. Title is
// always the short scan handle; a question also retains its full Statement for
// structured clients that need detail without another lookup.
type RelatedNode struct {
	Noun      string `json:"noun"`
	ID        string `json:"id"`
	Key       string `json:"key,omitempty"`
	Ref       string `json:"ref"`
	Status    string `json:"status"`
	Title     string `json:"title,omitempty"`
	Statement string `json:"statement,omitempty"`
}

// Relationships is the node page's relationship hub, how the graph is walked
// one page at a time (ui-surface.md, "Node detail"). Fields that do not apply
// to a noun stay empty: pointers are nil when absent and empty lists are
// omitted, so a task page carries a feature and an epic while an epic page
// carries features.
type Relationships struct {
	// Epic is a feature's owning epic, or a task's grandparent epic through
	// its parent feature.
	Epic *RelatedNode `json:"epic,omitempty"`
	// Feature is a task's parent feature.
	Feature *RelatedNode `json:"feature,omitempty"`
	// Features are an epic's children, in the order gen.Order ranks them
	// (childFeatures below reads order.Features), not by key: siblings rank on
	// the shared dependency sort with key order breaking ties (cli-surface.md,
	// issue #27).
	Features []RelatedNode `json:"features,omitempty"`
	// Tasks are a feature's task folders in the cached sibling dependency order.
	Tasks []RelatedNode `json:"tasks,omitempty"`
	// AttachedTo is the tree node where a bug or chore was discovered.
	AttachedTo *RelatedNode `json:"attached_to,omitempty"`
	// ResolvedBy is the tree node carrying a bug or chore to resolution.
	ResolvedBy *RelatedNode `json:"resolved_by,omitempty"`
	// DuplicateOf is the surviving shelf node named by a duplicate cancellation.
	DuplicateOf *RelatedNode `json:"duplicate_of,omitempty"`
	// AttachedWork is the shelf work attached directly to this tree node or
	// rolled up from its descendants, in shelf dependency order.
	AttachedWork []RelatedNode `json:"attached_work,omitempty"`
	// DependsOn are the node's recorded dependencies, sorted by reference.
	DependsOn []RelatedNode `json:"depends_on,omitempty"`
	// DependedOnBy is the reverse edge, every node whose depends_on names this
	// node, sorted by reference.
	DependedOnBy []RelatedNode `json:"depended_on_by,omitempty"`
	// Supersedes are the same-noun nodes this node replaced, sorted by reference.
	Supersedes []RelatedNode `json:"supersedes,omitempty"`
	// SupersededBy is the same-noun node that replaced this one. The store records
	// at most one, but the edge stays a list so both supersession directions
	// render the same shape.
	SupersededBy []RelatedNode `json:"superseded_by,omitempty"`
	// BlockedByQuestions are the open questions whose blocks name this node,
	// each with its short title for scanning and full statement retained.
	BlockedByQuestions []RelatedNode `json:"blocked_by_questions,omitempty"`
	// Blocks are a question's targets, in the ledger's recorded order.
	Blocks                  []RelatedNode       `json:"blocks,omitempty"`
	Amendments              []AmendmentRelation `json:"amendments,omitempty"`
	AmendmentHistoryWarning string              `json:"amendment_history_warning,omitempty"`
}

// HistoryEntry is one event on the node's timeline: when (RFC3339 UTC), the
// event type, the acting contributor, and the literal command.
type HistoryEntry struct {
	At          string `json:"at"`
	Type        string `json:"type"`
	Contributor string `json:"contributor"`
	Command     string `json:"command"`
}

// NodePageOf assembles the uniform node page from the loaded tree, the event
// stream, and the resolved node. DependencyOrder is computed once here (issue
// #27) and threaded into every epic/feature ordering below it.
func NodePageOf(tree *store.Tree, evs []events.Event, n store.Node, index *relationships.Index) NodePage {
	if index == nil {
		index = relationships.BuildStructural(tree)
		relationships.AddAmendments(tree, index, evs, "")
		index.Finalize()
	}
	var prose *ProseRef
	if p, ok := gen.ProsePath(tree, n); ok {
		prose = &ProseRef{Path: p}
	}
	order := gen.DependencyOrder(tree)
	page := NodePage{
		Node:          Node(tree, n),
		Ref:           gen.Ref(tree, n),
		Prose:         prose,
		Relationships: relationshipsOf(tree, order, n, index),
		Backlinks:     BacklinksOf(index, n.NodeID()),
		History:       historyOf(evs, n.NodeID()),
	}
	amendments := amendmentViewsOf(tree, index.Amendments)
	page.Relationships.Amendments = amendments.relations[n.NodeID()]
	if n.Noun() == store.NounEpic || n.Noun() == store.NounFeature || n.Noun() == store.NounTask || n.Noun() == store.NounAmendment {
		page.Relationships.AmendmentHistoryWarning = amendments.warning
	}
	if e, ok := n.(*store.Epic); ok {
		page.Decomposition = decompositionOf(tree, order, e)
	}
	if a, ok := n.(*store.Amendment); ok {
		page.Amendment = amendmentPageOf(a, amendments.operations[a.ID])
	}
	return page
}

// historyOf filters the stream to the events targeting the node and orders
// them newest first, the timeline read a human scans top-down. The result is
// always an array, empty rather than nil.
func historyOf(evs []events.Event, nodeID string) []HistoryEntry {
	var mine []events.Event
	for _, e := range evs {
		if slices.Contains(e.Targets, nodeID) {
			mine = append(mine, e)
		}
	}
	events.Sort(mine)
	out := make([]HistoryEntry, 0, len(mine))
	for i := len(mine) - 1; i >= 0; i-- {
		e := mine[i]
		out = append(out, HistoryEntry{
			At:          formatTime(e.At),
			Type:        e.Type,
			Contributor: e.Contributor,
			Command:     e.Command,
		})
	}
	return out
}

// relationshipsOf projects structural facts; effective holds stay in gen.
func relationshipsOf(t *store.Tree, order *gen.Order, n store.Node, index *relationships.Index) Relationships {
	rel := Relationships{
		DependsOn:          relatedEdges(t, index, n.NodeID(), "depends_on"),
		DependedOnBy:       relatedEdges(t, index, n.NodeID(), "depended_on_by"),
		Supersedes:         relatedEdges(t, index, n.NodeID(), "supersedes"),
		SupersededBy:       relatedEdges(t, index, n.NodeID(), "superseded_by"),
		BlockedByQuestions: blockingQuestions(t, n),
	}
	switch node := n.(type) {
	case *store.Epic:
		rel.Features = childFeatures(t, order, node)
	case *store.Feature:
		rel.Epic = relatedEdgePointer(t, index, node.ID, "contained_by")
		rel.Tasks = featureTasks(t, order, node.ID)
	case *store.Task:
		rel.Feature = relatedEdgePointer(t, index, node.ID, "contained_by")
		if rel.Feature != nil {
			rel.Epic = relatedEdgePointer(t, index, rel.Feature.ID, "contained_by")
		}
	case *store.Question:
		rel.Blocks = relatedEdges(t, index, node.ID, "blocks")
		// Keep the ledger's presentation order while resolving only through
		// the shared facts, including legacy keys and unresolved targets.
		recordedOrder := make(map[string]int, len(node.Blocks))
		for i := len(node.Blocks) - 1; i >= 0; i-- {
			recordedOrder[node.Blocks[i]] = i
		}
		targetOrder := make(map[string]int, len(rel.Blocks))
		for _, edge := range index.Adjacent(node.ID, "blocks") {
			target := edge.To
			if vertex := index.Vertices[target]; vertex.Unresolved {
				target = vertex.Raw
			}
			for _, source := range edge.Sources {
				position, known := recordedOrder[source.Raw]
				previous, seen := targetOrder[target]
				if known && (!seen || position < previous) {
					targetOrder[target] = position
				}
			}
		}
		sort.SliceStable(rel.Blocks, func(i, j int) bool {
			return targetOrder[rel.Blocks[i].ID] < targetOrder[rel.Blocks[j].ID]
		})
	case *store.Bug, *store.Chore, *store.Tweak:
		rel.AttachedTo = relatedEdgePointer(t, index, n.NodeID(), "attached_to")
		rel.ResolvedBy = relatedEdgePointer(t, index, n.NodeID(), "resolved_by")
		rel.DuplicateOf = relatedEdgePointer(t, index, n.NodeID(), "duplicate_of")
	}
	switch n.(type) {
	case *store.Epic, *store.Feature, *store.Task:
		rel.AttachedWork = attachedRelationshipWork(t, index, n.NodeID())
	}
	return rel
}

func relatedEdges(t *store.Tree, index *relationships.Index, id, direction string) []RelatedNode {
	var out []RelatedNode
	for _, edge := range index.Adjacent(id, direction) {
		target := edge.To
		if edge.To == id {
			target = edge.From
		}
		vertex := index.Vertices[target]
		if vertex.Unresolved {
			out = append(out, RelatedNode{ID: vertex.Raw, Ref: vertex.Raw})
		} else {
			out = append(out, relatedByIDs(t, []string{target})...)
		}
	}
	sortByRef(out)
	return out
}

func relatedEdgePointer(t *store.Tree, index *relationships.Index, id, direction string) *RelatedNode {
	// A missing structural parent is absent, unlike a shelf's recorded
	// unresolved attachment, resolution, or duplicate pointer.
	if direction == "contained_by" {
		for _, edge := range index.Adjacent(id, direction) {
			if index.Vertices[edge.From].Unresolved {
				return nil
			}
		}
	}
	rows := relatedEdges(t, index, id, direction)
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func attachedRelationshipWork(t *store.Tree, index *relationships.Index, id string) []RelatedNode {
	targets := map[string]bool{id: true}
	queue := []string{id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range index.Adjacent(current, "contains") {
			if !targets[edge.To] {
				targets[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
	}
	sources := map[string]bool{}
	for target := range targets {
		for _, edge := range index.Adjacent(target, "attached_work") {
			sources[edge.From] = true
		}
	}
	var out []RelatedNode
	for _, node := range mapShelf(t) {
		if sources[node.NodeID()] {
			out = append(out, relatedOf(t, node))
		}
	}
	return out
}

// relatedOf projects a node onto its link row.
func relatedOf(t *store.Tree, n store.Node) RelatedNode {
	row := RelatedNode{
		Noun:   n.Noun(),
		ID:     n.NodeID(),
		Key:    Key(n),
		Ref:    gen.Ref(t, n),
		Status: n.NodeStatus(),
		Title:  handleOf(n),
	}
	if q, ok := n.(*store.Question); ok {
		row.Statement = q.Statement
	}
	return row
}

// handleOf is the node's short prose handle. Questions always go through the
// shared helper so legacy statements stay bounded on every scan.
func handleOf(n store.Node) string {
	switch v := n.(type) {
	case *store.Epic:
		return v.Title
	case *store.Feature:
		return v.Title
	case *store.Task:
		return v.Title
	case *store.Bug:
		return v.Title
	case *store.Chore:
		return v.Title
	case *store.Tweak:
		return v.Title
	case *store.Amendment:
		return v.Title
	case *store.Question:
		return gen.QuestionTitle(v)
	}
	return ""
}

// relatedByIDs resolves a list of recorded refs to link rows, preserving the
// input order (callers that want reference order sort after). Canonical refs
// go through the general resolver first; legacy epic, feature, and task keys
// fall back to resolveWorkNode, the same compatibility path the question
// blocker predicates use. An unresolved or ambiguous ref degrades to a bare
// row rather than disappearing: nothing is hidden, least of all a broken edge.
func relatedByIDs(t *store.Tree, ids []string) []RelatedNode {
	if len(ids) == 0 {
		return nil
	}
	r := resolve.NewResolver(t)
	out := make([]RelatedNode, 0, len(ids))
	for _, id := range ids {
		n, err := r.Resolve(resolve.Ref{Raw: id})
		if err != nil {
			n = resolveWorkNode(t, id)
		}
		if n == nil {
			out = append(out, RelatedNode{ID: id, Ref: id})
			continue
		}
		out = append(out, relatedOf(t, n))
	}
	return out
}

// owningEpic finds the epic a key names, nil when the layout link dangles.
func owningEpic(t *store.Tree, epicKey string) *RelatedNode {
	if epicKey == "" {
		return nil
	}
	for _, e := range t.Epics {
		if e.Key != nil && *e.Key == epicKey {
			row := relatedOf(t, e)
			return &row
		}
	}
	return nil
}

// childFeatures gathers an epic's features by the layout link (the feature's
// EpicKey), in DependencyOrder (issue #27).
func childFeatures(t *store.Tree, order *gen.Order, e *store.Epic) []RelatedNode {
	if e.Key == nil || *e.Key == "" {
		return nil
	}
	var out []RelatedNode
	for _, id := range order.Features(*e.Key) {
		if f, ok := t.Features[id]; ok {
			out = append(out, relatedOf(t, f))
		}
	}
	return out
}

// decompositionOf gathers an epic's full feature breakdown: every feature the
// layout link names (the feature's EpicKey), projected through the complete
// Node view rather than childFeatures' lightweight relationship row, so an
// epic's approval review carries each feature's source, change type, surface,
// and gates from one fetch. Ordered by DependencyOrder (issue #27), the same
// order childFeatures applies. Present for every epic page: a childless epic,
// or one with a nil or empty key (the degenerate case that should not exist
// for a folder-backed noun, so childFeatures' own guard never even tries to
// match it), still returns the empty features list rather than a nil
// DecompositionView.
func decompositionOf(t *store.Tree, order *gen.Order, e *store.Epic) *DecompositionView {
	features := make([]NodeView, 0, len(t.Features))
	if e.Key != nil && *e.Key != "" {
		for _, id := range order.Features(*e.Key) {
			if f, ok := t.Features[id]; ok {
				features = append(features, Node(t, f))
			}
		}
	}
	return &DecompositionView{Features: features}
}

// AmendmentPageView is a proposed reshape's own node-page content, filling
// NodePage.Amendment for a NounAmendment page only: every staged operation in
// staged order, the two design-doc lists that name why the amendment exists
// (TriggeredBy) and what its approval keeps current (AmendsDesign), and
// Source, the amendment-level fingerprinted feedback artifact an
// intake-minted amendment carries. The op-level Source and
// Blocks fields are AmendmentOperationPageView's own turf, below.
type AmendmentPageView struct {
	Operations   []AmendmentOperationPageView `json:"operations"`
	TriggeredBy  []string                     `json:"triggered_by,omitempty"`
	AmendsDesign []string                     `json:"amends_design,omitempty"`
	Source       *AmendmentSourcePageView     `json:"source,omitempty"`
	// ValidityError names the reason(s) live validity could not be computed
	// against the current tree (#57), the SAME vocabulary the CLI's own
	// `show amendment --json` envelope already carries (validity_error,
	// cli/amendment_cmds.go's runShowAmendment). Left empty by this pure
	// projection; node.go's fillAmendmentValidity is the one place the git
	// reads that can produce this error are allowed to run, and it is the
	// only caller that ever sets it, from EITHER of its two fallible reads
	// (mutate.EvaluateAmendmentPreconditions or mutate.EvaluateAmendmentSource),
	// joined and deduped by fillAmendmentValidity's own
	// joinAmendmentValidityErrors when both fail (the two evaluators share
	// one loadAmendmentDoc read, so a doc-load failure surfaces from both
	// identically and is named once, not twice).
	//
	// The two reads are independent, and so is what goes blank: a
	// precondition's own Live slot blanks only when the PRECONDITIONS read
	// itself failed, and Source's own Live slot blanks only when the SOURCE
	// read itself failed. ValidityError non-empty is not a claim that every
	// Live slot on the page is blank; it names only that at least one of the
	// two reads failed. TestNodePageAmendmentSourceErrorAloneStillFillsPreconditionLive
	// (internal/serve/amendment_test.go) pins the case that makes this
	// concrete: ValidityError set from Source's own failure alone, while
	// every precondition's Live stays filled from the healthy preconditions
	// read beside it.
	ValidityError string `json:"validity_error,omitempty"`
}

// AmendmentOperationPageView is one staged operation, structured the same way
// AmendmentData/AmendmentOperationView already carry it in the CLI's own
// amendment surface (cli/amendment_cmds.go), reprojected here so the console
// needs no second read of the amendment's yml. Target, By, and SupersededBy
// resolve through the shared amendment identity index, as do Created, Parent,
// and Blocks. A staged key with no proven materialized identity carries its
// reference but no fabricated ID. IdentityNotes explain missing or ambiguous
// historical endpoints. Title and Statement retain the question's independent
// scanning and full-text fields. SourceMissing and ShapeError are this operation's own
// pre-flight, the console-side twin of AmendmentOperationView's identically
// named CLI fields (cli/amendment_cmds.go): SourceMissing true only when this
// operation's own Source is blank on an intake-minted amendment
// (mutate.EvaluateAmendmentOpSources), and ShapeError the reason token
// (mutate.ReasonDuplicateStagedKey or mutate.ReasonStagedEpicReplacement)
// when resolveAmendment would refuse on this operation's own shape
// (mutate.EvaluateAmendmentShape), blank otherwise. Both evaluators take no
// tree and never fail (their own doc comments, amendment_validity.go), so
// amendmentPageOf fills these two fields directly, unconditionally, rather
// than leaving them for the serve handler the way Live is left: unlike a
// precondition or Source's own live read, there is no git here to defer.
type AmendmentOperationPageView struct {
	Index         int                             `json:"index"`
	Op            string                          `json:"op"`
	Target        *RelatedNode                    `json:"target,omitempty"`
	By            *RelatedNode                    `json:"by,omitempty"`
	Key           string                          `json:"key,omitempty"`
	Title         string                          `json:"title,omitempty"`
	Statement     string                          `json:"statement,omitempty"`
	ChangeType    string                          `json:"change_type,omitempty"`
	Reason        string                          `json:"reason,omitempty"`
	SupersededBy  *RelatedNode                    `json:"superseded_by,omitempty"`
	Blocks        []RelatedNode                   `json:"blocks,omitempty"`
	Retire        []string                        `json:"retire,omitempty"`
	Source        string                          `json:"source,omitempty"`
	SourceMissing bool                            `json:"source_missing,omitempty"`
	ShapeError    string                          `json:"shape_error,omitempty"`
	Created       *RelatedNode                    `json:"created,omitempty"`
	Parent        *RelatedNode                    `json:"parent,omitempty"`
	IdentityNotes []string                        `json:"identity_notes,omitempty"`
	Preconditions []AmendmentPreconditionPageView `json:"preconditions,omitempty"`
	Move          *FeatureMovePageView            `json:"move,omitempty"`
}

// FeatureMovePageView shows reviewed effects without exposing snapshot hashes.
type FeatureMovePageView struct {
	SourceKey              string   `json:"source_key"`
	ReviewGrain            string   `json:"review_grain"`
	MaterializeReviewGrain bool     `json:"materialize_review_grain,omitempty"`
	OrderEffect            string   `json:"order_effect"`
	ReferenceWarnings      []string `json:"reference_warnings,omitempty"`
}

// AmendmentPreconditionPageView is one staged precondition: the structural
// name and staged expected value, the only two facts this pure projection can
// read off the tree alone. Live is left nil here, the same typed-nil-slot
// pattern NodePage.Eval and NodePage.ActiveBasis already use on this page:
// comparing a precondition against the CURRENT tree (spec_revision and
// eval_revision hash working-tree files through git,
// mutate.EvaluateAmendmentPreconditions) is I/O this package performs none
// of, so the serve handler fills it in, mirroring how it fills Eval and
// ActiveBasis.
type AmendmentPreconditionPageView struct {
	Name     string                         `json:"name"`
	Expected string                         `json:"expected"`
	Live     *AmendmentPreconditionLiveView `json:"live,omitempty"`
}

// AmendmentPreconditionLiveView is a staged precondition's live read against
// the current tree, mutate.PreconditionValidity's own Status/Actual/Note
// projected for the console. Nil until the serve handler fills it (see
// AmendmentPreconditionPageView's own doc comment). AmendmentSourcePageView's
// own Live field reuses this exact shape rather than a bespoke twin: an
// amendment-level Source and a per-op precondition both read out as the same
// Status/Actual/Note triple, mutate.PreconditionValidity and
// mutate.AmendmentSourceValidity's own parallel shapes (amendment_validity.go).
type AmendmentPreconditionLiveView struct {
	Status string `json:"status,omitempty"`
	Actual string `json:"actual,omitempty"`
	Note   string `json:"note,omitempty"`
}

// AmendmentSourcePageView is an intake-minted amendment's own provenance
// artifact, additive on the amendment page: Path and Revision are the
// structural fingerprint intake recorded, carried here with no live comparison,
// the same pure-projection posture AmendmentPreconditionPageView's own doc
// comment states; Live is the typed nil slot the serve handler fills with
// mutate.EvaluateAmendmentSource's own read, mirroring how it fills every
// precondition's Live and NodePage's own ActiveBasis/Eval. Nil whole for a
// hand-opened amendment (AmendmentPageView.Source omits the key), the same
// convention store.Amendment.Source itself keeps.
type AmendmentSourcePageView struct {
	Path     string                         `json:"path"`
	Revision string                         `json:"revision"`
	Live     *AmendmentPreconditionLiveView `json:"live,omitempty"`
}

// amendmentPageOf projects an amendment's own staged content onto the page,
// views' pure half of it: every operation's targets resolved against the
// ALREADY-LOADED tree, its preconditions carried structurally with no live
// comparison (AmendmentPreconditionPageView's own doc comment), and its own
// Source, structurally, with the same no-live-comparison posture
// (AmendmentSourcePageView's own doc comment). SourceMissing and ShapeError
// are the one exception to "no live comparison": mutate.EvaluateAmendmentOpSources
// and mutate.EvaluateAmendmentShape are pure over a already in hand (no tree,
// no error, amendment_validity.go's own doc comments), so they run right
// here, unconditionally, computed once for the whole batch and folded into
// each operation by index — the identical index-keyed join
// cli/amendment_cmds.go's runShowAmendment performs, just run inline rather
// than injected from a caller, since nothing here needs the tree or a live
// git read to produce them.
func amendmentPageOf(a *store.Amendment, identities []amendmentOperationIdentity) *AmendmentPageView {
	opSourceMissing := make(map[int]bool, len(a.Operations))
	for _, osv := range mutate.EvaluateAmendmentOpSources(a) {
		opSourceMissing[osv.Index] = true
	}
	opShape := make(map[int]string, len(a.Operations))
	for _, sf := range mutate.EvaluateAmendmentShape(a) {
		opShape[sf.Index] = sf.Kind
	}
	ops := make([]AmendmentOperationPageView, len(a.Operations))
	for i, op := range a.Operations {
		ops[i] = amendmentOperationPageOf(i, op, identities[i], opSourceMissing[i], opShape[i])
	}
	return &AmendmentPageView{
		Operations:   ops,
		TriggeredBy:  a.TriggeredBy,
		AmendsDesign: a.AmendsDesign,
		Source:       amendmentSourcePageOf(a.Source),
	}
}

// amendmentSourcePageOf projects a.Source, or nil when the amendment carries
// none (every hand-opened amendment), mirroring cli/amendment_cmds.go's own
// amendmentSourceView for the identical nil-means-hand-opened convention.
func amendmentSourcePageOf(s *store.AmendmentSource) *AmendmentSourcePageView {
	if s == nil {
		return nil
	}
	return &AmendmentSourcePageView{Path: s.Path, Revision: s.Revision}
}

// amendmentOperationPageOf projects one staged operation onto its page row.
// Preconditions walk mutate.PreconditionNames()'s fixed order rather than
// ranging the stored map directly, the same deterministic order show
// amendment's own render already uses (cli/amendment_cmds.go's
// amendmentOperationView), so two renders of the same operation never
// disagree about which precondition comes first. sourceMissing and
// shapeError are amendmentPageOf's own per-index findings, joined in here
// (AmendmentOperationPageView's own doc comment explains why this needs no
// tree or live read to compute).
func amendmentOperationPageOf(index int, op store.AmendmentOp, identity amendmentOperationIdentity, sourceMissing bool, shapeError string) AmendmentOperationPageView {
	v := AmendmentOperationPageView{
		Index: index, Op: op.Op, Key: op.Key, Title: op.Title, Statement: op.Statement,
		ChangeType:    op.ChangeType,
		Reason:        op.Reason,
		Target:        identity.target,
		By:            identity.by,
		SupersededBy:  identity.supersededBy,
		Blocks:        identity.blocks,
		Retire:        op.Retire,
		Created:       identity.created,
		Parent:        identity.parent,
		IdentityNotes: identity.notes,
		Source:        op.Source,
		SourceMissing: sourceMissing,
		ShapeError:    shapeError,
	}
	if op.Move != nil {
		v.Move = &FeatureMovePageView{
			SourceKey: op.Move.SourceKey, ReviewGrain: op.Move.ReviewGrain,
			MaterializeReviewGrain: op.Move.MaterializeReviewGrain,
			OrderEffect:            op.Move.OrderEffect, ReferenceWarnings: op.Move.ReferenceWarnings,
		}
		v.Preconditions = append(v.Preconditions, AmendmentPreconditionPageView{Name: "move", Expected: "reviewed move snapshot"})
	}
	for _, name := range mutate.PreconditionNames() {
		expected, ok := op.Preconditions[name]
		if !ok {
			continue
		}
		v.Preconditions = append(v.Preconditions, AmendmentPreconditionPageView{Name: name, Expected: expected})
	}
	return v
}

// featureTasks projects the complete sibling group already ranked by Order.
func featureTasks(t *store.Tree, order *gen.Order, featureID string) []RelatedNode {
	var out []RelatedNode
	for _, tk := range order.Tasks(featureID) {
		out = append(out, relatedOf(t, tk))
	}
	return out
}

// blockingQuestions projects the same operative question blockers that guard
// work onto a node page. Feature and task pages therefore include inherited
// ancestor holds, while feature assumptions clear only the feature's own view.
func blockingQuestions(t *store.Tree, node store.Node) []RelatedNode {
	blockers := questionBlockersForNode(t, node)
	out := make([]RelatedNode, 0, len(blockers))
	for _, blocker := range blockers {
		if question := t.Questions[blocker.Question]; question != nil {
			out = append(out, relatedOf(t, question))
		}
	}
	sortByRef(out)
	return out
}

// questionBlockersForNode is the views package's noun dispatch over gen's one
// blocker predicate. A task needs its feature to evaluate inherited targets;
// a missing parent makes the task inoperable and yields no separate question
// claim here.
func questionBlockersForNode(t *store.Tree, node store.Node) []gen.QuestionBlocker {
	switch node := node.(type) {
	case *store.Epic:
		return gen.EpicQuestionBlockers(t, node)
	case *store.Feature:
		return gen.FeatureQuestionBlockers(t, node)
	case *store.Task:
		return gen.TaskQuestionBlockers(t, t.Features[node.FeatureID], node)
	default:
		return nil
	}
}

// sortByRef orders link rows by their composed reference, the order every
// human-facing list here shares.
func sortByRef(items []RelatedNode) {
	sort.Slice(items, func(i, j int) bool { return items[i].Ref < items[j].Ref })
}
