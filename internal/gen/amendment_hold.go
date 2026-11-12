package gen

import "github.com/madeintandem/tiller/internal/store"

// AmendmentHold names the open amendment holding n: the draft or proposed
// amendment whose staged add created n and has not yet been approved or
// withdrawn (the design's decision 7). It is nil when nothing holds n. It
// reads the index store.Load derives (store.Tree.AmendmentHolds), so a tree
// built by hand with no index holds nothing.
//
// This is a different fact from Held, a node under a retired ancestor: a
// node an amendment holds is a proposal waiting on review, not work stranded
// under a cancellation.
func AmendmentHold(t *store.Tree, n store.Node) *store.Amendment {
	if t == nil || n == nil {
		return nil
	}
	amendmentID, ok := t.AmendmentHolds[n.NodeID()]
	if !ok {
		return nil
	}
	return t.Amendments[amendmentID]
}
