package views

import (
	"sort"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// ListResult is the --json payload of ls: the homogeneous noun, the match
// count, and the matching nodes in a stable order. Nodes is always an array,
// empty rather than null when nothing matches, so a caller parses one shape.
type ListResult struct {
	Noun  string     `json:"noun"`
	Count int        `json:"count"`
	Nodes []ListItem `json:"nodes"`
}

// ListItem is one row of a list: the addressing identity plus the few fields a
// caller scans. It is intentionally leaner than the full NodeView; a caller
// that wants a node in full follows up with `show`. Fields that do not apply
// to a noun are omitted.
type ListItem struct {
	ID        string `json:"id"`
	Key       string `json:"key,omitempty"`
	Title     string `json:"title,omitempty"`
	Statement string `json:"statement,omitempty"` // question
	Status    string `json:"status"`
	// Activity is the derived phase this row carries: a feature's rollup phase
	// (awaiting_check / awaiting_sync, gen.FeatureActivity), an epic's derived
	// active/idle working status (gen.EpicActivity), or a bug's or chore's
	// carrier phase (carried / carrier_cancelled, gen.WorkActivity), the same
	// answer the status oracle gives (views.activity). Empty for a task, a
	// question, and wherever the noun's phase does not apply.
	Activity     string              `json:"activity,omitempty"`
	Owner        string              `json:"owner,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
	// AttachedTo is a bug or chore's provenance: where it was found (a
	// feature, task, or epic id), empty for every other noun.
	AttachedTo                string                       `json:"attached_to,omitempty"`
	RecoveredReview           *workflow.RecoveredReview    `json:"recovered_review,omitempty"`
	ReviewMissing             bool                         `json:"review_missing,omitempty"`
	Gates                     map[string]GateView          `json:"gates,omitempty"`
	CurrentAcceptance         *workflow.CurrentAcceptance  `json:"current_acceptance,omitempty"`
	ParticipatingAcceptance   []workflow.CurrentAcceptance `json:"participating_acceptance,omitempty"`
	RecoveredCrossingsPending *bool                        `json:"recovered_crossings_pending,omitempty"`
	HistoricalViolations      *int                         `json:"historical_violations,omitempty"`
	CrossingViolations        *int                         `json:"crossing_violations,omitempty"`

	// Answer is a question's recorded answer, nil until answered: the same
	// *string NodeView.Answer already carries for the full node view
	// (node.go), reused here rather than a second shape for the identical
	// fact. Empty for every other noun.
	Answer *string `json:"answer,omitempty"`
	// CancelReason is a cancelled question's recorded reason, nil otherwise.
	CancelReason *string `json:"cancel_reason,omitempty"`
	// Due is a question's due hint, gen.QuestionDue's date-or-hint answer,
	// the same field the queue and streams rows already carry (queue.go,
	// streams.go). Empty for every other noun and for a question with none
	// recorded.
	Due string `json:"due,omitempty"`
	// BlocksRefs resolves a question's Blocks edges to link rows through
	// relatedByIDs (nodepage.go), the same degrade-to-bare-id-row posture
	// every other edge on the node page already applies to a dangling
	// target: nothing hidden, least of all a broken edge. Empty for every
	// other noun.
	BlocksRefs []RelatedNode `json:"blocks_refs,omitempty"`

	// Ref is the node's human reference ("<key-path> (<id>)", gen.Ref) for the
	// plain-text renderer only; the JSON payload carries id and key structurally.
	Ref string `json:"-"`
}

// ListFilters carries the optional narrowing predicates. An empty field is "no
// filter on this axis". Feature and Epic narrow by parentage: a feature filter
// lists one feature's tasks, an epic filter lists one epic's features. Both
// name their target by key or id; ResolveParentFilters rewrites them to the
// stable identity the match compares against (the parent feature's id, the
// parent epic's key) before the match runs.
type ListFilters struct {
	Status      string
	Owner       string
	Contributor string
	Feature     string
	Epic        string
}

// Collect gathers every node of the named class from the tree as the common
// Node view, so the filter and projection run uniformly.
func Collect(t *store.Tree, noun string) []store.Node {
	var out []store.Node
	switch noun {
	case store.NounEpic:
		for _, e := range t.Epics {
			out = append(out, e)
		}
	case store.NounFeature:
		for _, fe := range t.Features {
			out = append(out, fe)
		}
	case store.NounTask:
		for _, tk := range t.Tasks {
			out = append(out, tk)
		}
	case store.NounQuestion:
		for _, q := range t.Questions {
			out = append(out, q)
		}
	case store.NounAmendment:
		for _, a := range t.Amendments {
			out = append(out, a)
		}
	case store.NounBug, store.NounChore, store.NounTweak:
		for _, w := range t.WorkNodesOf(noun) {
			out = append(out, w)
		}
	}
	return out
}

// ResolveParentFilters rewrites the feature and epic filter values from
// whatever the caller named (a key or an id) to the stable identity the
// per-node match compares against: a feature filter becomes the parent
// feature's id (the link a task carries), an epic filter becomes the epic's
// canonical key (the link a feature carries). It returns the resolver's typed
// not-found/mismatch error (exit 2 at the CLI) when a named parent does not
// resolve.
func ResolveParentFilters(tree *store.Tree, f *ListFilters) error {
	r := resolve.NewResolver(tree)
	if f.Feature != "" {
		id, err := resolveParent(r, store.NounFeature, f.Feature)
		if err != nil {
			return err
		}
		f.Feature = id
	}
	if f.Epic != "" {
		key, err := resolveParent(r, store.NounEpic, f.Epic)
		if err != nil {
			return err
		}
		f.Epic = key
	}
	return nil
}

// ResolveParentFiltersLeniently resolves each parent independently for the
// console. Unresolved values remain raw and are reported by axis. The returned
// map is always nonnil; the CLI continues to use ResolveParentFilters.
func ResolveParentFiltersLeniently(tree *store.Tree, f *ListFilters) map[string]string {
	unresolved := map[string]string{}
	r := resolve.NewResolver(tree)
	if f.Feature != "" {
		if id, err := resolveParent(r, store.NounFeature, f.Feature); err == nil {
			f.Feature = id
		} else {
			unresolved[AxisFeature] = f.Feature
		}
	}
	if f.Epic != "" {
		if key, err := resolveParent(r, store.NounEpic, f.Epic); err == nil {
			f.Epic = key
		} else {
			unresolved[AxisEpic] = f.Epic
		}
	}
	return unresolved
}

// resolveParent returns the identity Matches compares against.
func resolveParent(r *resolve.Resolver, noun, raw string) (string, error) {
	n, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
	if err != nil {
		return "", err
	}
	if noun == store.NounEpic {
		return Key(n), nil
	}
	return n.NodeID(), nil
}

// Matches reports whether a node passes every active filter. A filter on an
// axis a noun does not carry (an owner filter on a node with no owner, a
// feature filter on anything but a task) simply excludes it, which is the
// honest answer to a filter the noun cannot answer.
func Matches(tree *store.Tree, n store.Node, f ListFilters) bool {
	if f.Status == "unfinished" {
		switch n.Noun() {
		case store.NounTask, store.NounBug, store.NounChore, store.NounTweak:
			if !gen.UnfinishedAssignment(n.NodeStatus()) {
				return false
			}
		default:
			return false
		}
	} else if f.Status != "" && n.NodeStatus() != f.Status {
		return false
	}
	if f.Owner != "" && Owner(n) != f.Owner {
		return false
	}
	if f.Contributor != "" {
		found := false
		for _, contributor := range Contributors(tree, n) {
			if contributor.ID == f.Contributor {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.Feature != "" && FeatureID(n) != f.Feature {
		return false
	}
	if f.Epic != "" && EpicKey(n) != f.Epic {
		return false
	}
	return true
}

// Item projects a node onto a list row.
func Item(tree *store.Tree, n store.Node) ListItem {
	item := ListItem{
		ID:           n.NodeID(),
		Key:          Key(n),
		Status:       n.NodeStatus(),
		Owner:        Owner(n),
		Ref:          gen.Ref(tree, n),
		Contributors: Contributors(tree, n),
	}
	item.Activity = activity(tree, n)
	item.RecoveredReview, item.ReviewMissing = workflow.ReviewStateOf(tree, n)
	switch t := n.(type) {
	case *store.Epic:
		item.Title = t.Title
	case *store.Feature:
		if item.RecoveredReview != nil {
			item.Gates = Gates(t.GatesBlock)
		}
		item.Title = t.Title
	case *store.Task:
		item.Title = t.Title
	case *store.Question:
		item.Title = gen.QuestionTitle(t)
		item.Statement = t.Statement
		item.Answer = t.Answer
		if t.CancelReason != "" {
			reason := t.CancelReason
			item.CancelReason = &reason
		}
		item.Due = gen.QuestionDue(t)
		item.BlocksRefs = relatedByIDs(tree, t.Blocks)
	case *store.Amendment:
		item.Title = t.Title
	default:
		if w, ok := store.AsWorkNode(n); ok {
			item.Title = w.Body().Title
			item.AttachedTo = w.Body().AttachedTo
		}
	}
	return item
}

// SortListItems orders rows canonically: bugs and chores by their shelf's own
// ranking (gen.OrderWorkNodes: edges, then the soft order, then key), epics,
// features, and tasks by DependencyOrder (issue #27), every other noun by key
// with id fallback. A list here is one noun, so the whole slice ranks through
// one comparator.
func SortListItems(t *store.Tree, noun string, items []ListItem) {
	if noun == store.NounAmendment {
		sort.Slice(items, func(i, j int) bool {
			return amendmentLess(items[i].Status, items[i].Key, items[i].ID, items[j].Status, items[j].Key, items[j].ID)
		})
		return
	}
	if store.IsWorkNoun(noun) {
		ranked := gen.OrderWorkNodes(t, t.WorkNodesOf(noun), t.ShelfOrderOf(noun))
		pos := make(map[string]int, len(ranked))
		for i, w := range ranked {
			pos[w.NodeID()] = i
		}
		sort.SliceStable(items, func(i, j int) bool { return pos[items[i].ID] < pos[items[j].ID] })
		return
	}
	if gen.RankedNoun(noun) {
		order := gen.DependencyOrder(t)
		sort.SliceStable(items, func(i, j int) bool {
			return order.Less(noun, items[i].ID, items[j].ID)
		})
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return sortKey(items[i]) < sortKey(items[j])
	})
}

func sortKey(it ListItem) string {
	if it.Key != "" {
		return it.Key
	}
	return it.ID
}

// recoveredRowOwner resolves a recovered row's review owner, or nil.
func recoveredRowOwner(t *store.Tree, noun string, item ListItem) store.Node {
	if item.RecoveredReview == nil {
		return nil
	}
	switch noun {
	case store.NounFeature:
		if feature := t.Features[item.ID]; feature != nil {
			return workflow.ReviewOwnerOf(t, feature)
		}
	case store.NounEpic:
		if epic := t.Epics[item.ID]; epic != nil {
			return workflow.ReviewOwnerOf(t, epic)
		}
	}
	return nil
}

// RecoveredOwners names the distinct review owners of recovered rows, the
// only nodes EnrichRecoveredList classifies and therefore observes.
func RecoveredOwners(t *store.Tree, noun string, items []ListItem) []store.Node {
	var owners []store.Node
	seen := map[string]bool{}
	for _, item := range items {
		owner := recoveredRowOwner(t, noun, item)
		if owner == nil || seen[owner.NodeID()] {
			continue
		}
		seen[owner.NodeID()] = true
		owners = append(owners, owner)
	}
	return owners
}

// EnrichRecoveredList projects observed current acceptance for recovered
// aggregate rows. The caller gathers observations once; advice is classified
// once per owning aggregate, even when several features ride an epic.
func EnrichRecoveredList(t *store.Tree, noun string, items []ListItem, in workflow.Input) {
	adviceByOwner := make(map[string]*workflow.Advice)
	for i := range items {
		item := &items[i]
		owner := recoveredRowOwner(t, noun, *item)
		if owner == nil {
			continue
		}
		advice, seen := adviceByOwner[owner.NodeID()]
		if !seen {
			advice = workflow.AdviceOf(t, owner, in)
			adviceByOwner[owner.NodeID()] = advice
		}
		if advice == nil {
			continue
		}
		item.RecoveredCrossingsPending = &advice.RecoveredCrossingsPending
		item.HistoricalViolations = &advice.HistoricalViolations
		item.CrossingViolations = &advice.CrossingViolations
		if noun == store.NounFeature && len(advice.CurrentAcceptance) > 0 {
			for j := range advice.CurrentAcceptance {
				if advice.CurrentAcceptance[j].NodeID == item.ID {
					item.CurrentAcceptance = &advice.CurrentAcceptance[j]
					break
				}
			}
		} else {
			item.CurrentAcceptance = &workflow.CurrentAcceptance{NodeID: item.ID,
				ContractStatus: advice.ContractStatus, AcceptanceReady: advice.AcceptanceReady, Outstanding: advice.Outstanding}
		}
		if noun == store.NounEpic {
			item.ParticipatingAcceptance = advice.CurrentAcceptance
		}
	}
}
