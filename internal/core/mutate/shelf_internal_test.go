package mutate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/madeintandem/tiller/internal/store"
)

func TestEnglishList(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", englishList("or"))
	require.Equal(t, "bug", englishList("or", "bug"))
	require.Equal(t, "bug or chore", englishList("or", "bug", "chore"))
	require.Equal(t, "bugs, chores, and tweaks", englishList("and", "bugs", "chores", "tweaks"))
}

// TestShelfFactsComeFromTheTable pins the mutate-side derivations to the
// store's shelf table: every shelf gets its own prose stub and order event,
// and an unknown noun panics instead of borrowing the bug's.
func TestShelfFactsComeFromTheTable(t *testing.T) {
	t.Parallel()
	for _, s := range store.Shelves() {
		require.Contains(t, renderWorkItemMD(s.Noun, "Title"), "`tiller add "+s.Noun+"`")
		require.Equal(t, "order."+s.Dir, shelfOrderEvent(s.Noun))
	}
	require.Equal(t, "order.bugs", shelfOrderEvent(store.NounBug))
	require.Equal(t, "order.tweaks", shelfOrderEvent(store.NounTweak))
	require.Panics(t, func() { renderWorkItemMD(store.NounTask, "Title") })
	require.Panics(t, func() { shelfOrderEvent(store.NounTask) })
}
