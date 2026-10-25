package store

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/id"
	"github.com/stretchr/testify/require"
)

// TestShelvesCoverEveryShelfType pins the table against the typed nodes the
// loader stores: every concrete shelf type names a noun in the table, and
// every table row builds a node of its own noun.
func TestShelvesCoverEveryShelfType(t *testing.T) {
	typed := []WorkNode{&Bug{}, &Chore{}, &Tweak{}}
	shelves := Shelves()
	require.Len(t, shelves, len(typed))
	for i, node := range typed {
		require.Equal(t, node.Noun(), shelves[i].Noun, "table order is bug, chore, tweak")
	}
	for _, s := range shelves {
		node, err := NewWorkNode(s.Noun)
		require.NoError(t, err)
		require.Equal(t, s.Noun, node.Noun())
		prefix, err := id.PrefixFor(s.Noun)
		require.NoError(t, err)
		require.Equal(t, prefix, s.IDPrefix, "the id package mints %s ids", s.Noun)
		require.True(t, id.Valid(id.Mint(s.IDPrefix)))
		tree := &Tree{}
		require.NotPanics(t, func() { tree.WorkNodesOf(s.Noun) }, "Tree.WorkNodesOf knows %s", s.Noun)
		require.NotPanics(t, func() { tree.ShelfOrderOf(s.Noun) }, "Tree.ShelfOrderOf knows %s", s.Noun)
	}
	require.Equal(t, []string{NounBug, NounChore, NounTweak}, ShelfNouns())
}

func TestShelvesReturnsACopy(t *testing.T) {
	Shelves()[0].Dir = "mutated"
	require.Equal(t, "bugs", Shelves()[0].Dir)
}

// TestShelfIDPrefixesNeverOverlap proves no shelf id can be mistaken for any
// other class: every "<prefix>-" is distinct and none starts another, so
// the hyphen keeps task T- apart from tweak TW-.
func TestShelfIDPrefixesNeverOverlap(t *testing.T) {
	all := []string{"E", "F", "T", "Q", "A", "ADR", "EV", "AT"}
	for _, s := range Shelves() {
		all = append(all, s.IDPrefix)
	}
	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			require.NotEqual(t, a, b)
			require.False(t, strings.HasPrefix(a+"-", b+"-"), "%s- starts with %s-", a, b)
		}
	}
}

func TestShelfDirsAreUniqueReservedEpicKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Shelves() {
		require.False(t, seen[s.Dir], "dir %s repeats", s.Dir)
		seen[s.Dir] = true
		require.True(t, IsReservedEpicKey(s.Dir))
		require.Equal(t, s.Dir, ShelfDir(s.Noun))
	}
	require.False(t, IsReservedEpicKey("epics"))
	require.False(t, IsReservedEpicKey("bug"))
}

func TestShelfFor(t *testing.T) {
	s, ok := ShelfFor(NounTweak)
	require.True(t, ok)
	require.Equal(t, Shelf{Noun: "tweak", Dir: "tweaks", IDPrefix: "TW"}, s)
	_, ok = ShelfFor(NounTask)
	require.False(t, ok)
	_, ok = ShelfFor("bugs")
	require.False(t, ok)
}

func TestShelfOfRef(t *testing.T) {
	cases := []struct {
		raw  string
		noun string // "" when the ref names no shelf
	}{
		{"B-abc123", NounBug},
		{"CH-abc123", NounChore},
		{"TW-abc123", NounTweak},
		{"bugs/blank-on-reload", NounBug},
		{"chores/bump-rails", NounChore},
		{"tweaks/nudge", NounTweak},
		{"T-abc123", ""},
		{"F-abc123", ""},
		{"E-abc123", ""},
		{"Q-abc123", ""},
		{"A-abc123", ""},
		{"ADR-abc123", ""},
		{"AT-abc123", ""},
		{"EV-abc123", ""},
		{"TWEAKS/x", ""},
		{"tw-abc123", ""},
		{"tweaks", ""},
		{"tweak/x", ""},
		{"epic/feature", ""},
		{"B", ""},
		{"", ""},
	}
	for _, c := range cases {
		s, ok := ShelfOfRef(c.raw)
		if c.noun == "" {
			require.False(t, ok, "%q names no shelf, got %s", c.raw, s.Noun)
			continue
		}
		require.True(t, ok, "%q names the %s shelf", c.raw, c.noun)
		require.Equal(t, c.noun, s.Noun, "%q", c.raw)
	}
}

// TestUnknownShelfNounNeverFallsBackToBug pins every former silent bug
// fallback: a noun the table does not know panics (a caller's programming
// error) rather than reading or writing the bug shelf.
func TestUnknownShelfNounNeverFallsBackToBug(t *testing.T) {
	tree := &Tree{}
	root := t.TempDir()
	for name, call := range map[string]func(noun string){
		"ShelfDir":          func(noun string) { ShelfDir(noun) },
		"ShelfProseFile":    func(noun string) { ShelfProseFile(noun) },
		"ShelfStateFile":    func(noun string) { ShelfStateFile(noun) },
		"ShelfRelDir":       func(noun string) { ShelfRelDir(noun, "k") },
		"ShelfCanonicalKey": func(noun string) { ShelfCanonicalKey(noun, "k") },
		"ShelfOrderRelPath": func(noun string) { ShelfOrderRelPath(noun) },
		"LoadShelfOrder":    func(noun string) { _, _ = LoadShelfOrder(root, noun) },
		"Tree.WorkNodesOf":  func(noun string) { tree.WorkNodesOf(noun) },
		"Tree.ShelfOrderOf": func(noun string) { tree.ShelfOrderOf(noun) },
	} {
		for _, noun := range []string{NounTask, "bugs", ""} {
			want := `store: "` + noun + `" is not a shelf noun`
			require.PanicsWithValue(t, want, func() { call(noun) }, "%s(%q)", name, noun)
		}
	}
	_, err := NewWorkNode(NounTask)
	require.EqualError(t, err, `store: "task" is not a shelf noun`)
}
