package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDesign(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "design", filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLoadListsUnregisteredInputsAsToBeDesign(t *testing.T) {
	root := epiclessFixture(t)
	writeDesign(t, root, "plans/next/sync.md", "# Sync\n")
	writeDesign(t, root, "README.MD", "# Readme\n")
	writeDesign(t, root, ".draft.md", "# Draft\n")
	writeDesign(t, root, ".hidden/idea.md", "# Idea\n")
	writeDesign(t, root, "notes.txt", "notes\n")
	writeDesign(t, root, "diagram.png", "png")

	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, []string{"00-index.md", "README.MD", "diagram.png", "notes.txt", "plans/next/sync.md"}, tree.ToBeDesign,
		"any regular non-dot file is a design input, as readiness counts it; only dotfiles are skipped")
}

func TestLoadExcludesRegisteredDocsFromToBeDesign(t *testing.T) {
	root := epiclessFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	writeDesign(t, root, "modules/api.md", "# API\n") // registered and confirmed in validBaseline
	writeDesign(t, root, "baseline.yml", validBaseline+`  - path: modules/pending.md
    describes: [app]
`)
	writeDesign(t, root, "modules/pending.md", "# Pending\n") // registered, unconfirmed

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign, "as-is docs, confirmed or pending, are never to-be inputs")
}

func TestLoadIgnoresSymlinkedDesignEntries(t *testing.T) {
	root := epiclessFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "dir", "escape.md"), []byte("# x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.md"), []byte("# x\n"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, ".tiller", "design", "linked")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "file.md"), filepath.Join(root, ".tiller", "design", "linked.md")))

	tree, err := Load(root)
	require.NoError(t, err, "nothing under design/ may fail the tree load")
	require.Nil(t, tree.ToBeDesign)
}

func TestLoadWithoutADesignDirHasNoToBeDesign(t *testing.T) {
	root := epiclessFixture(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "design")))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign)
}

// A source an as-is doc cites through --from, left under design/ by the
// pointer arm or an unretired import, is the old spec the baseline was
// derived from, not new work.
func TestLoadExcludesCitedSourcesFromToBeDesign(t *testing.T) {
	root := epiclessFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	writeDesign(t, root, "spec.md", "# Old spec\n")
	writeDesign(t, root, "as-is/app.md", "# App\n")
	writeDesign(t, root, "baseline.yml", `docs:
  - path: as-is/app.md
    describes: [app]
    derived_from:
      - kind: git_blob
        value: git:1234567
        path: .tiller/design/spec.md
        observed_at: 2026-08-28T12:00:00Z
`)

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign)
}

// On a case-insensitive filesystem link design accepts as-is/app.md for a
// file spelled as-is/App.md; that registered doc is still not a to-be input.
func TestLoadMatchesRegisteredDocsWithoutCase(t *testing.T) {
	root := epiclessFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	writeDesign(t, root, "as-is/App.md", "# App\n")
	writeDesign(t, root, "baseline.yml", `docs:
  - path: as-is/app.md
    describes: [app]
`)

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign)
}

// epiclessFixture is the store fixture tree with its epics removed, the only
// tree shape whose load gathers ToBeDesign.
func epiclessFixture(t *testing.T) string {
	t.Helper()
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "epics")))
	return root
}

// The route consults ToBeDesign only on an epic-less tree, so loading skips
// the walk once epics exist; the fixture tree has one epic.
func TestLoadSkipsTheToBeWalkWhenEpicsExist(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	writeDesign(t, root, "sync.md", "# Sync\n")

	tree, err := Load(root)
	require.NoError(t, err)
	require.NotEmpty(t, tree.Epics)
	require.Nil(t, tree.ToBeDesign)
}
