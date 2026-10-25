package views

import (
	"sort"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// ProjectSummary is the JSON payload of the bare status command: the project
// identity, its contributors, and the node counts across the tree. It is the stable
// glanceable picture before per-node detail and per-contributor rollups arrive.
type ProjectSummary struct {
	Project      ProjectInfo       `json:"project"`
	Contributors []ContributorInfo `json:"contributors"`
	Counts       TreeCounts        `json:"counts"`
	// ActiveEpics names the approved (stored 'ready') epics that are derived-active:
	// being worked because a child feature is in flight. It is a display-only
	// rollup; the epics' stored status stays 'ready' (see gen.EpicActivity).
	ActiveEpics []string `json:"active_epics,omitempty"`
	// WhoseMove is the project-wide tally of task A2's NextOwner projection
	// across every non-terminal node in the tree (see WhoseMoveOf). It carries
	// no omitempty, the same always-present shape as Counts: a project with
	// nothing left to move still reports zeroes rather than an absent key.
	WhoseMove WhoseMoveCounts `json:"whose_move"`

	// ActiveEpicRefs carries the same list as human references ("<key-path>
	// (<id>)", gen.Ref) for the plain-text renderer only. The JSON payload keeps
	// the bare keys above: machine output carries id and key structurally, never
	// the composed human label.
	ActiveEpicRefs []string `json:"-"`

	// Order is the recorded root soft order (store.ProjectOrder.Epics,
	// .tiller/order.yml): carriage only, the epic sequence `tiller order
	// epic` last wrote, verbatim and possibly partial, never the full
	// computed epic order gen.DependencyOrder.Epics() already renders
	// elsewhere. Absent until a root order is first recorded.
	Order []string `json:"order,omitempty"`
	// OrderRefs carries Order's targets as human references for the
	// plain-text renderer only, the same json:"-" posture ActiveEpicRefs
	// above already keeps and for the identical reason: machine output
	// carries ids structurally, never the composed human label.
	OrderRefs []string `json:"-"`

	// UncommittedState names the repo-relative paths under .tiller/ that git
	// sees as modified or untracked (issue #44): the field case was two
	// attest commands that wrote feature.yml plus two event files, and the
	// state sat uncommitted for a day with no surface naming it. It is
	// informational only, additive JSON (omitempty): readiness and every
	// gate stay untouched, and a clean tree's envelope is byte-identical to
	// before this field existed.
	//
	// Summary below never sets it. The comparison is a live git read
	// (vcs.DirtyUnder), which internal/views performs none of; the CLI fills
	// it in after loading the tree (status.go), the same posture ActiveBasis
	// and UnpushedReview already take on NodeStatusView.
	UncommittedState []string `json:"uncommitted_state,omitempty"`
}

// ProjectInfo identifies the project: its name, key, and archetype.
type ProjectInfo struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

// ContributorInfo is one registered project contributor as surfaced to a caller.
type ContributorInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TreeCounts is the node tally across the loaded tree.
type TreeCounts struct {
	Epics     int `json:"epics"`
	Features  int `json:"features"`
	Tasks     int `json:"tasks"`
	Questions int `json:"questions"`
	Bugs      int `json:"bugs"`
	Chores    int `json:"chores"`
	Tweaks    int `json:"tweaks"`
}

// NodeStatusView is the typed oracle payload for `status <noun> <node>`: the
// node's identity, its current state, the transitions legal from here, and its
// recorded gate results. It is leaner than the full NodeView that `show`
// prints, because the oracle answers "where is this node, what is legal next,
// and what has been checked", not "everything about it". LegalNext is always
// present, an empty array at a terminal state, so a caller parses one shape.
type NodeStatusView struct {
	Noun             string                    `json:"noun"`
	ID               string                    `json:"id"`
	Key              string                    `json:"key,omitempty"`
	Status           string                    `json:"status"`
	LegalNext        []string                  `json:"legal_next"`
	Workflow         *workflow.Advice          `json:"workflow,omitempty"`
	RecoveredReview  *workflow.RecoveredReview `json:"recovered_review,omitempty"`
	ReviewMissing    bool                      `json:"review_missing,omitempty"`
	PolicyViolations []PolicyViolationView     `json:"policy_violations,omitempty"`
	// Activity is the node's derived working status: an epic's "active" or
	// "idle" (computed from its children, gen.EpicActivity), a feature's
	// rollup phase ("awaiting_check" / "awaiting_sync", gen.FeatureActivity),
	// or a bug's or chore's carrier phase ("carried" / "carrier_cancelled",
	// gen.WorkActivity). None is stored: an approved epic with work in flight
	// reads "active" while its stored Status stays "ready", an active feature
	// whose tasks are all done reads its phase while its stored Status stays
	// "active", and a bug reads its carrier's fate while its own status never
	// moves. Empty for a task, a question, and wherever the noun's phase does
	// not apply.
	Activity     string              `json:"activity,omitempty"`
	Blocked      bool                `json:"blocked,omitempty"`
	Owner        string              `json:"owner,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
	// Title is a task's, amendment's, or question's short title; Statement
	// retains a question's full detail. An epic's and a feature's own handle is
	// the key path their reference already renders.
	Title     string              `json:"title,omitempty"`
	Statement string              `json:"statement,omitempty"`
	Gates     map[string]GateView `json:"gates,omitempty"`
	// Relationships carries only the question edges relevant to the status
	// oracle: a question's durable forward Blocks, or a work node's operative
	// reverse BlockedByQuestions. Nil when neither applies so the additive
	// object is omitted rather than serialized empty.
	Relationships *Relationships `json:"relationships,omitempty"`

	// ActiveBasis names the pinned files whose current working-tree content no
	// longer matches the node's accepted basis, the drift `start`, `check`, and
	// `submit` refuse on (cli-surface.md, "Specifying a feature": status, serve,
	// and drift report active_basis). It is absent when nothing drifted.
	//
	// NodeStatus below never sets it. The comparison hashes files through git,
	// which is I/O, and internal/views performs none; the CLI fills it in from
	// mutate.BasisDrift, the guards' own predicate (status.go's
	// statusBasisDrift). Ref above is filled here rather than by the caller
	// because gen.Ref is pure.
	ActiveBasis []string `json:"active_basis,omitempty"`

	// Ref is the node's human reference ("<key-path> (<id>)", gen.Ref) for the
	// plain-text renderer only; the JSON payload carries id and key structurally.
	Ref string `json:"-"`

	// Held names the nearest retired ancestor this node sits under, nil when
	// none (gen.RetiredAncestor), the same additive fact NodeView.Held
	// carries for `show`. Like the rest of this file's own fields it is
	// filled by NodeStatus below rather than left for the CLI to add on:
	// unlike ActiveBasis, this comparison needs no git or filesystem I/O,
	// so views can answer it directly.
	Held *HeldView `json:"held,omitempty"`

	// UnpushedReview names the review-state warning (issue #42) when this
	// task's recorded feature branch holds a commit its PR does not yet show:
	// the local tip is not on the remote a reviewer sees. Like ActiveBasis it
	// is never set by NodeStatus below; the CLI fills it in after a live git
	// read (status.go), absent when the task is not in review, records no PR,
	// or its branch is already pushed even with its upstream.
	UnpushedReview *UnpushedReviewView `json:"unpushed_review,omitempty"`
}

// UnpushedReviewView is the additive unpushed_review object the task status
// oracle and the submit confirmation envelope both carry (issue #42): the
// short sha sitting on the recorded feature branch's local tip, the branch
// itself, and what it was compared against. It is absent (a nil pointer)
// exactly when nothing is unpushed, so an unaffected caller's envelope is
// byte-identical to before this field existed. NodeStatus below never sets
// it, the same reason ActiveBasis is left to the CLI: naming it requires a
// live git read, which views performs none of.
type UnpushedReviewView struct {
	Commit   string `json:"commit"`
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
}

// NodeStatus projects the typed oracle for one node: its state, legal next
// moves, and recorded gates.
func NodeStatus(tree *store.Tree, n store.Node, in workflow.Input) NodeStatusView {
	advice := workflow.AdviceOf(tree, n, in)
	var recovered *workflow.RecoveredReview
	missing := false
	if advice != nil && advice.RecoveredReview != nil {
		recovered = advice.RecoveredReview
	} else {
		recovered, missing = workflow.ReviewStateOf(tree, n)
	}
	return NodeStatusView{
		Noun:             n.Noun(),
		ID:               n.NodeID(),
		Key:              Key(n),
		Status:           n.NodeStatus(),
		LegalNext:        legalNext(n),
		Workflow:         advice,
		RecoveredReview:  recovered,
		ReviewMissing:    missing,
		Activity:         activity(tree, n),
		Blocked:          Blocked(n),
		Owner:            Owner(n),
		Contributors:     Contributors(tree, n),
		Title:            StatusTitle(n),
		PolicyViolations: NodePolicyViolations(n),
		Statement:        Statement(n),
		Gates:            NodeGates(n),
		Ref:              gen.Ref(tree, n),
		Held:             heldViewOf(gen.RetiredAncestor(tree, n)),
		Relationships:    statusRelationships(tree, n),
	}
}

// statusRelationships projects the bidirectional question edges without
// widening status into the full node-page relationship hub. Forward targets
// remain recorded after a question is answered. Reverse targets reuse the
// operative blocker dispatch, preserving ancestor inheritance, assumption
// handling, liveness, deduplication, and ordering from gen.
func statusRelationships(tree *store.Tree, n store.Node) *Relationships {
	var relationships Relationships
	switch node := n.(type) {
	case *store.Question:
		relationships.Blocks = relatedByIDs(tree, node.Blocks)
	case *store.Epic, *store.Feature, *store.Task:
		relationships.BlockedByQuestions = blockingQuestions(tree, n)
	}
	if len(relationships.Blocks) == 0 && len(relationships.BlockedByQuestions) == 0 {
		return nil
	}
	return &relationships
}

// activity is the node's derived working status for the oracle: an epic's
// active/idle (computed from its children), a feature's rollup phase
// (awaiting_check / awaiting_sync, gen.FeatureActivity), a bug's or chore's
// carrier phase (carried / carrier_cancelled, gen.WorkActivity), empty
// elsewhere and for a feature with no phase.
func activity(tree *store.Tree, n store.Node) string {
	switch node := n.(type) {
	case *store.Epic:
		return gen.EpicActivity(tree, node)
	case *store.Feature:
		return gen.FeatureActivity(tree, node)
	default:
		if w, ok := store.AsWorkNode(n); ok {
			return gen.WorkActivity(tree, w)
		}
	}
	return ""
}

// legalNext is the lifecycle engine's legal-next verb set for a node, always a
// non-nil slice so it renders as a JSON array (empty at a terminal state)
// rather than null.
func legalNext(n store.Node) []string {
	moves := lifecycle.NextMoves(n)
	if moves == nil {
		return []string{}
	}
	return moves
}

// Summary rolls the loaded tree into the project summary payload.
func Summary(t *store.Tree) ProjectSummary {
	contributors := make([]ContributorInfo, 0, len(t.Manifest.Contributors))
	for _, o := range t.Manifest.Contributors {
		contributors = append(contributors, ContributorInfo{ID: o.ID, Name: o.Name})
	}

	var activeEpics, activeEpicRefs []string
	for _, e := range t.Epics {
		if gen.EpicActivity(t, e) == gen.EpicActivityActive {
			label := Key(e)
			if label == "" {
				label = e.ID
			}
			activeEpics = append(activeEpics, label)
			activeEpicRefs = append(activeEpicRefs, gen.Ref(t, e))
		}
	}
	sort.Strings(activeEpics)
	sort.Strings(activeEpicRefs)

	var orderRefs []string
	for _, id := range t.ProjectOrder.Epics {
		orderRefs = append(orderRefs, gen.RefByID(t, id))
	}

	return ProjectSummary{
		Project: ProjectInfo{
			Name: t.Manifest.Project.Name,
			Key:  t.Manifest.Project.Key,
			Kind: t.Manifest.Project.Kind,
		},
		Contributors: contributors,
		Counts: TreeCounts{
			Epics:     len(t.Epics),
			Features:  len(t.Features),
			Tasks:     len(t.Tasks),
			Questions: len(t.Questions),
			Bugs:      len(t.Bugs),
			Chores:    len(t.Chores),
			Tweaks:    len(t.Tweaks),
		},
		ActiveEpics:    activeEpics,
		WhoseMove:      WhoseMoveOf(t),
		ActiveEpicRefs: activeEpicRefs,
		Order:          t.ProjectOrder.Epics,
		OrderRefs:      orderRefs,
	}
}
