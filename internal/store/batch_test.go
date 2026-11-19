package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func batchTreeFixture(t *testing.T) string {
	t.Helper()
	root := writeManifestFixture(t, "schema_version: 16\ncontributors: [{id: ada, name: Ada}]\n")
	writeFile(t, filepath.Join(root, ".tiller/batches/core-model/batch.yml"), "id: BT-aaaaaa\nkey: batches/core-model\ntitle: Core model fixes\nstatus: ready\nblocked: false\nstarted: false\n")
	writeFile(t, filepath.Join(root, ".tiller/bugs/beta/bug.yml"), "id: B-bbbbbb\nkey: bugs/beta\nstatus: ready\nbatch: BT-aaaaaa\n")
	writeFile(t, filepath.Join(root, ".tiller/bugs/alpha/bug.yml"), "id: B-aaaaaa\nkey: bugs/alpha\nstatus: proposed\nbatch: BT-aaaaaa\n")
	writeFile(t, filepath.Join(root, ".tiller/tweaks/wording/tweak.yml"), "id: TW-aaaaaa\nkey: tweaks/wording\nstatus: ready\nbatch: BT-aaaaaa\n")
	writeFile(t, filepath.Join(root, ".tiller/chores/alone/chore.yml"), "id: CH-aaaaaa\nkey: chores/alone\nstatus: ready\n")
	return root
}

func TestBatchIsLaidOutLikeAShelfWithoutBeingOne(t *testing.T) {
	require.Equal(t, "batches", ShelfDir(NounBatch))
	require.Equal(t, "batch.yml", ShelfStateFile(NounBatch))
	require.Equal(t, "batches/core-model", ShelfCanonicalKey(NounBatch, "core-model"))
	require.False(t, IsWorkNoun(NounBatch))
	require.NotContains(t, ShelfNouns(), NounBatch)
	require.True(t, IsReservedEpicKey("batches"))
	_, shelf := ShelfOfRef("BT-aaaaaa")
	require.False(t, shelf, "a batch id is not a bug id")
	n, err := NewWorkNode(NounBatch)
	require.NoError(t, err)
	_, ok := AsWorkNode(n)
	require.True(t, ok)
}

func TestLoadReadsBatchesAndTheirMembers(t *testing.T) {
	tree, err := Load(batchTreeFixture(t))
	require.NoError(t, err)
	batch := tree.Batches["BT-aaaaaa"]
	require.NotNil(t, batch)
	require.Equal(t, "Core model fixes", batch.Title)
	require.True(t, BatchLive(batch))

	var keys []string
	for _, member := range tree.BatchMembers(batch.ID) {
		keys = append(keys, *member.NodeKey())
	}
	require.Equal(t, []string{"bugs/alpha", "bugs/beta", "tweaks/wording"}, keys)
	require.Same(t, batch, tree.BatchOf(tree.Bugs["B-aaaaaa"]))
	require.Nil(t, tree.BatchOf(tree.Chores["CH-aaaaaa"]))
	_, onShelf := tree.WorkNode(batch.ID)
	require.False(t, onShelf, "a batch is not shelf work")
}

func TestLoadRefusesMalformedBatchRecords(t *testing.T) {
	for name, tc := range map[string]struct{ path, body, want string }{
		"proposed batch":   {".tiller/batches/core-model/batch.yml", "id: BT-aaaaaa\nkey: batches/core-model\nstatus: proposed\n", "status"},
		"shelf-only field": {".tiller/batches/core-model/batch.yml", "id: BT-aaaaaa\nkey: batches/core-model\nstatus: ready\nseverity: minor\n", "a batch carries no severity"},
		"wrong id class":   {".tiller/batches/core-model/batch.yml", "id: B-zzzzzz\nkey: batches/core-model\nstatus: ready\n", "must start with BT-"},
		"wrong folder key": {".tiller/batches/core-model/batch.yml", "id: BT-aaaaaa\nkey: bugs/core-model\nstatus: ready\n", "canonical"},
		"dangling member":  {".tiller/chores/alone/chore.yml", "id: CH-aaaaaa\nkey: chores/alone\nstatus: ready\nbatch: BT-zzzzzz\n", "names batch BT-zzzzzz"},
	} {
		t.Run(name, func(t *testing.T) {
			root := batchTreeFixture(t)
			writeFile(t, filepath.Join(root, tc.path), tc.body)
			_, err := Load(root)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestBatchedDoneNeedsNoStartMarker(t *testing.T) {
	root := batchTreeFixture(t)
	writeFile(t, filepath.Join(root, ".tiller/batches/core-model/batch.yml"), "id: BT-aaaaaa\nkey: batches/core-model\nstatus: done\nstarted: true\nowner: ada\nbranch: tiller/batches/core-model\ndone_basis: ancestry\n")
	writeFile(t, filepath.Join(root, ".tiller/bugs/beta/bug.yml"), "id: B-bbbbbb\nkey: bugs/beta\nstatus: done\ndone_basis: batched\nbatch: BT-aaaaaa\n")
	_, err := Load(root)
	require.NoError(t, err)

	writeFile(t, filepath.Join(root, ".tiller/batches/core-model/batch.yml"), "id: BT-aaaaaa\nkey: batches/core-model\nstatus: in_progress\n")
	_, err = Load(root)
	require.ErrorContains(t, err, "BT-aaaaaa has execution history without started")
}
