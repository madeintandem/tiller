package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// amendmentHoldFixture is a tree with one open amendment holding one draft
// feature, and a second feature nothing holds. The hold index is set by hand
// here, the way store.Load derives it from the amendment's add operations.
func amendmentHoldFixture() (*store.Tree, *store.Feature, *store.Feature) {
	heldKey, freeKey, amendmentKey := "hardening/csp-headers", "hardening/rate-limits", "hardening"
	held := &store.Feature{ID: "F-held01", Key: &heldKey, Status: "draft", EpicKey: "hardening"}
	free := &store.Feature{ID: "F-free01", Key: &freeKey, Status: "draft", EpicKey: "hardening"}
	amendment := &store.Amendment{ID: "A-hold01", Key: &amendmentKey, Status: store.AmendmentStatusDraft}
	tree := &store.Tree{
		Features:       map[string]*store.Feature{held.ID: held, free.ID: free},
		Amendments:     map[string]*store.Amendment{amendment.ID: amendment},
		AmendmentHolds: map[string]string{held.ID: amendment.ID},
	}
	return tree, held, free
}

// TestAmendmentHold names the amendment holding a node, and nil for a node no
// amendment holds.
func TestAmendmentHold(t *testing.T) {
	tree, held, free := amendmentHoldFixture()

	got := AmendmentHold(tree, held)
	require.NotNil(t, got)
	require.Equal(t, "A-hold01", got.ID)
	require.Nil(t, AmendmentHold(tree, free))
}

// An index entry naming an amendment the tree does not carry holds nothing,
// and a tree with no index at all (one built by hand) answers nil rather than
// panicking.
func TestAmendmentHoldDegradesToNil(t *testing.T) {
	tree, held, _ := amendmentHoldFixture()
	tree.AmendmentHolds[held.ID] = "A-gone01"
	require.Nil(t, AmendmentHold(tree, held))

	require.Nil(t, AmendmentHold(&store.Tree{}, held))
	require.Nil(t, AmendmentHold(tree, nil))
}
