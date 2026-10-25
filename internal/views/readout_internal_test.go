package views

// This file holds the tests that need package-internal access to readout.go's
// unexported detection helpers (firstInvertedPair), the same split
// eval_test.go, nodepage_test.go, and streams_test.go already use for their
// own source files: readout_test.go stays package views_test for the surface
// (Route, ReadoutOfInput) every other readout test drives.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFirstInvertedPairNamesTheLowestIndexPairFirst pins the "first such
// pair" rule (task-7-brief.md) beyond the two-element scopes
// TestDependencyOrderCheckNamesAnOverriddenSoftOrder and its sibling in
// readout_test.go exercise, where the rule and an adjacent-inversion
// reading cannot be told apart. soft = [P, Q, R, S], rendered = [Q, S, P]
// (R renders nowhere in this scope, which the function tolerates the same
// way an id soft lists but the tree never gave a node holds no place to
// invert against) has two candidate inverted pairs once restricted to the
// listed ids Q, S, P: (Q, P) at positions (0, 2) and (S, P) at positions
// (1, 2). The rule names the lowest i first and, for that i, the lowest j:
// i=0 (Q) already inverts against j=2 (P) before the scan ever reaches i=1
// (S), so (P, Q) is named even though (P, S) inverts by a wider ordinal
// gap. This is the reviewer's own probe shape (task-7 fix review), kept
// here rather than folded into the two-element tests because it is the
// smallest input that actually distinguishes "first by the (i, j) scan
// order" from every other plausible reading of "first."
func TestFirstInvertedPairNamesTheLowestIndexPairFirst(t *testing.T) {
	soft := []string{"P", "Q", "R", "S"}
	rendered := []string{"Q", "S", "P"}

	x, y, found := firstInvertedPair(soft, rendered)

	require.True(t, found, "Q ahead of P in rendered, though P is listed earlier, is an inversion")
	require.Equal(t, "P", x, "P is the soft-listed-earlier id of the pair the (i, j) scan finds first")
	require.Equal(t, "Q", y,
		"Q is named, not S: i=0 (Q) pairs with j=2 (P) before the scan ever reaches i=1 (S), "+
			"even though S also inverts against P and by a wider ordinal gap")
}

// TestFirstInvertedPairFindsNoneOnAConsistentOrder is the contrast case at
// the same three-element-plus scope: listed's ordinals already climb
// (Q < S, in soft order too), so no pair inverts and the zero value comes
// back honestly rather than naming something.
func TestFirstInvertedPairFindsNoneOnAConsistentOrder(t *testing.T) {
	soft := []string{"P", "Q", "R", "S"}
	rendered := []string{"P", "Q", "S"}

	x, y, found := firstInvertedPair(soft, rendered)

	require.False(t, found)
	require.Empty(t, x)
	require.Empty(t, y)
}

func TestEveryQueueKindHasAnAttentionCategory(t *testing.T) {
	covered := map[string]bool{}
	for _, category := range attentionCategories {
		covered[category.kind] = true
	}
	for _, kind := range QueueKinds {
		if !covered[kind] {
			t.Errorf("queue kind %q has no readout attention category", kind)
		}
	}
	for kind := range covered {
		found := false
		for _, k := range QueueKinds {
			found = found || k == kind
		}
		if !found {
			t.Errorf("attention category kind %q is not a queue kind", kind)
		}
	}
}
