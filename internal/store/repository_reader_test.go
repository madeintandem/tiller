package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

func writeReaderFixture(t *testing.T, root, path, data string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

// Load used to open a fresh repository anchor and re-verify every directory
// from the root for each node file. One load now verifies each node directory
// exactly once, however many files sit beneath it.
func TestLoadVerifiesEachNodeDirectoryOnce(t *testing.T) {
	root := t.TempDir()
	writeReaderFixture(t, root, ".tiller/tiller.yml", fmt.Sprintf("schema_version: %d\nschema_epoch: initial\nproject: {name: Reader, key: reader, kind: greenfield}\ncontributors:\n  - {id: jc, name: JC}\n", CurrentSchemaVersion))
	writeReaderFixture(t, root, ".tiller/epics/pay/epic.yml", "id: E-aaaaaa\nkey: pay\nstatus: ready\n")
	writeReaderFixture(t, root, ".tiller/epics/pay/features/checkout/feature.yml", "id: F-aaaaaa\nkey: pay/checkout\nstatus: ready\n")
	for i, key := range []string{"one", "two", "three"} {
		id := []string{"T-aaaaaa", "T-bbbbbb", "T-cccccc"}[i]
		writeReaderFixture(t, root, ".tiller/epics/pay/features/checkout/tasks/"+key+"/task.yml", "id: "+id+"\nkey: pay/checkout/"+key+"\nstatus: ready\n")
	}
	descents := map[string]int{}
	previous := repositoryReaderWalker
	repositoryReaderWalker = rootpath.Walker{AfterOpenRoot: func(name string) { descents[name]++ }}
	t.Cleanup(func() { repositoryReaderWalker = previous })

	tree, err := Load(root)
	require.NoError(t, err)
	require.Len(t, tree.Tasks, 3)
	for _, dir := range []string{
		".tiller/epics",
		".tiller/epics/pay",
		".tiller/epics/pay/features",
		".tiller/epics/pay/features/checkout",
		".tiller/epics/pay/features/checkout/tasks",
		".tiller/epics/pay/features/checkout/tasks/one",
		".tiller/epics/pay/features/checkout/tasks/two",
		".tiller/epics/pay/features/checkout/tasks/three",
	} {
		require.Equal(t, 1, descents[filepath.FromSlash(dir)], "descents of %s: %v", dir, descents)
	}
	require.Nil(t, tree.reader, "a loaded tree must not keep its load-time reader")
}

func TestRepositoryReaderRefusesSymlinksAndTraversal(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeReaderFixture(t, outside, "task.yml", "id: T-outside\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "epics"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "epics", "pay")))
	reader, err := OpenRepositoryReader(root)
	require.NoError(t, err)
	defer reader.Close()

	_, err = reader.ReadFile(filepath.Join(".tiller", "epics", "pay", "task.yml"))
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
	_, err = reader.ReadDirectory(filepath.Join(".tiller", "epics", "pay"))
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
	_, err = reader.ReadFile(filepath.Join("..", "task.yml"))
	require.ErrorContains(t, err, "not a canonical relative path")
	_, err = reader.ReadFile(filepath.Join(".tiller", "missing.yml"))
	require.True(t, errors.Is(err, os.ErrNotExist), "error: %v", err)
}
