package search_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the keyword search behind `tiller search` and /api/search
// (ui-surface.md, "Explore"): a case-insensitive substring match over .tiller/
// markdown and the tree's human-meaningful fields, plus exact lookup by id,
// key, or reference. Matches are grouped into one result per node and one per
// unowned document, nodes ranked by how they matched (id, key, title, key
// fragment, text), with true totals, per-kind counts, a kind filter, and the
// caller's limit with MaxResults as its ceiling. The fixture keeps node ids
// fixed while minting each immutable event onto its own stable path.
// fixed while minting each immutable event onto its own stable path.

const searchManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Search Test
  key: searchtest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fixture lays a repo with one node of every class (fixed ids, one per
// prefix) plus a design doc, and returns the root and the loaded tree.
func fixture(t *testing.T) (string, *store.Tree) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(searchManifest), 0o644))

	eventID := 0
	deps := mutate.Deps{
		Contributor: "jc",
		Clock:       fixedClock{t: time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)},
		Mint: func(prefix string) string {
			if prefix == "EV" {
				eventID++
				return fmt.Sprintf("%s-%06d", prefix, eventID)
			}
			return prefix + "-aaaaaa"
		},
	}
	reload := func() *store.Tree {
		tree, err := store.Load(root)
		require.NoError(t, err)
		return tree
	}

	_, err := mutate.AddEpic(reload(), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(reload(), deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create",
	})
	require.NoError(t, err)
	_, err = mutate.AddTask(reload(), deps, mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/wire-the-schema-registry-loader",
		Title: "Wire the schema registry loader",
	})
	require.NoError(t, err)
	_, err = mutate.AddQuestion(reload(), deps, mutate.QuestionInput{Title: "Canonical certification revision", Statement: "Which certification revision is canonical?"})
	require.NoError(t, err)

	designDir := filepath.Join(root, ".tiller", "design")
	require.NoError(t, os.MkdirAll(designDir, 0o755))
	doc := "# Form shapes\n\nThe certification form loads from uploaded PDF manifests.\nA second line about parsing.\n"
	require.NoError(t, os.WriteFile(filepath.Join(designDir, "01-form-shapes.md"), []byte(doc), 0o644))

	return root, reload()
}

// searchShelfFixture extends the ordinary corpus with one real node on each
// shelf, including the state and project-authored Markdown the loader and
// search see in production.
func searchShelfFixture(t *testing.T) (string, *store.Tree) {
	t.Helper()
	root, _ := fixture(t)

	writeShelf := func(noun, localKey, id, title, prose string) {
		t.Helper()
		dir := filepath.Join(root, store.ShelfRelDir(noun, localKey))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		state := fmt.Sprintf(
			"id: %s\nkey: %s\ntitle: %s\nstatus: proposed\nblocked: false\nproposed_by: jc\n",
			id, store.ShelfCanonicalKey(noun, localKey), title,
		)
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, store.ShelfStateFile(noun)), []byte(state), 0o644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, store.ShelfProseFile(noun)), []byte(prose), 0o644,
		))
	}
	writeShelf(
		store.NounBug,
		"blank-on-reload",
		"B-aaaaaa",
		"Blank on reload",
		"## Reproduction\n\nThe page must reload after saving.\n",
	)
	writeShelf(
		store.NounChore,
		"bump-go",
		"CH-bbbbbb",
		"Bump Go",
		"## Maintenance\n\nRefresh the Go toolchain.\n",
	)
	writeShelf(
		store.NounTweak,
		"polish-button",
		"TW-cccccc",
		"Polish button",
		"## Improvement\n\nAdjust the button spacing.\n",
	)

	tree, err := store.Load(root)
	require.NoError(t, err)
	return root, tree
}
