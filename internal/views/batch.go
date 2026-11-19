package views

import (
	"slices"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// BatchRefView is the batch a shelf node names, as its own page shows it: the
// batch's identity and status, and the branch and review it records for every
// member. It stays on a member the batch crossed to done, as history.
type BatchRefView struct {
	ID        string         `json:"id"`
	Key       string         `json:"key,omitempty"`
	Title     string         `json:"title,omitempty"`
	Status    string         `json:"status"`
	Owner     string         `json:"owner,omitempty"`
	Branch    string         `json:"branch,omitempty"`
	PR        *int           `json:"pr,omitempty"`
	ReviewRef *ReviewRefView `json:"review_ref,omitempty"`

	// Ref is the batch's human reference, for the plain-text renderers only.
	Ref string `json:"-"`
}

// BatchMemberView is one member as its batch's page lists it, with the status
// the member itself stores.
type BatchMemberView struct {
	Noun     string `json:"noun"`
	ID       string `json:"id"`
	Key      string `json:"key,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`

	Ref string `json:"-"`
}

// batchRefOf projects the batch a shelf node names, nil for a node naming
// none and for a batch itself.
func batchRefOf(t *store.Tree, n store.Node) *BatchRefView {
	w, ok := store.AsWorkNode(n)
	if !ok {
		return nil
	}
	batch := t.BatchOf(w)
	if batch == nil {
		return nil
	}
	return &BatchRefView{
		ID: batch.ID, Key: Key(batch), Title: batch.Title, Status: batch.Status, Owner: batch.Owner,
		Branch: batch.Branch, PR: batch.PR, ReviewRef: ReviewRefOf(batch.ReviewRef), Ref: gen.Ref(t, batch),
	}
}

// batchMembersOf lists a batch's members, nil for every other noun.
func batchMembersOf(t *store.Tree, n store.Node) []BatchMemberView {
	batch, ok := n.(*store.Batch)
	if !ok {
		return nil
	}
	out := []BatchMemberView{}
	for _, member := range t.BatchMembers(batch.ID) {
		b := member.Body()
		out = append(out, BatchMemberView{
			Noun: member.Noun(), ID: b.ID, Key: Key(member), Title: b.Title, Status: b.Status, Severity: b.Severity,
			Ref: gen.Ref(t, member),
		})
	}
	return out
}

// severityOf is the severity a node displays: a bug's own grade, and for a
// batch the highest among its members.
func severityOf(t *store.Tree, n store.Node) string {
	switch node := n.(type) {
	case *store.Bug:
		return node.Severity
	case *store.Batch:
		return gen.BatchSeverity(t, node)
	}
	return ""
}

// batchOwnedVerbs are the lifecycle verbs a batch runs for its members, so a
// member's own oracle does not offer them.
var batchOwnedVerbs = []string{"start", "check", "submit", "resume"}

// legalNextIn is legalNext for a node read in its tree: a member of a live
// batch loses the verbs its batch owns, and loses return once the batch has
// started, the same refusals the mutation gives.
func legalNextIn(t *store.Tree, n store.Node) []string {
	moves := legalNext(n)
	w, ok := store.AsWorkNode(n)
	if !ok || !gen.Batched(t, w) {
		return moves
	}
	started := t.BatchOf(w).Started
	return slices.DeleteFunc(moves, func(verb string) bool {
		return slices.Contains(batchOwnedVerbs, verb) || (verb == "return" && started)
	})
}

// liveBatches lists the batches that are still work, in key order, as the
// execution units the shelf routing treats them as.
func liveBatches(t *store.Tree) []store.WorkNode {
	var out []store.WorkNode
	for _, batch := range t.BatchesInOrder() {
		if store.BatchLive(batch) {
			out = append(out, batch)
		}
	}
	return out
}

// shelfExecutionUnits lists the shelf work that answers for itself: every
// live batch first, then each shelf's nodes that have not left the queue, in
// shelf order. A batch is one unit, and its members are not listed.
func shelfExecutionUnits(t *store.Tree) []store.WorkNode {
	units := liveBatches(t)
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			if !gen.WorkLeftQueue(t, work) {
				units = append(units, work)
			}
		}
	}
	return units
}

// runsShelfLifecycle reports the nouns that share the shelf's in-review
// lifecycle and gate set: the shelf nouns and the batch that reviews them.
func runsShelfLifecycle(noun string) bool {
	return store.IsWorkNoun(noun) || noun == store.NounBatch
}
