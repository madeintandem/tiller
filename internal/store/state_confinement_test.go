package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateReadersRefuseOutsideSymlinks(t *testing.T) {
	for _, coordinate := range []string{
		".tiller", ".tiller/tiller.yml", ".tiller/decisions",
		".tiller/decisions/open-questions.yml", ".tiller/epics",
		".tiller/epics/form-engine/epic.yml", ".tiller/amendments",
		".tiller/order.yml", ".tiller/.cache", ".tiller/.cache/generation",
	} {
		t.Run(coordinate, func(t *testing.T) {
			root := copyTree(t, fixtureTreeRoot)
			outside := copyTree(t, fixtureTreeRoot)
			rel := filepath.FromSlash(coordinate)
			insidePath, outsidePath := filepath.Join(root, rel), filepath.Join(outside, rel)
			if coordinate == ".tiller/order.yml" || coordinate == ".tiller/.cache/generation" {
				require.NoError(t, os.MkdirAll(filepath.Dir(outsidePath), 0o755))
				data := "external-sentinel\n"
				if coordinate == ".tiller/order.yml" {
					data = "order: []\n"
				}
				require.NoError(t, os.WriteFile(outsidePath, []byte(data), 0o644))
			}
			if coordinate == ".tiller/.cache" {
				require.NoError(t, os.MkdirAll(outsidePath, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(outsidePath, "generation"), []byte("external-sentinel"), 0o644))
			}
			require.NoError(t, os.RemoveAll(insidePath))
			require.NoError(t, os.MkdirAll(filepath.Dir(insidePath), 0o755))
			require.NoError(t, os.Symlink(outsidePath, insidePath))
			tree, err := Load(root)
			require.Error(t, err, "outside state must not become repository state")
			require.Nil(t, tree)
		})
	}
}

func TestLocalStateAndSnapshotsRefuseOutsideSymlinks(t *testing.T) {
	for _, name := range []string{"identity.yml", "local.yml"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
			outside := filepath.Join(t.TempDir(), name)
			require.NoError(t, os.WriteFile(outside, []byte("owner: outside\nconsole:\n  theme: dark\n"), 0o644))
			path := filepath.Join(root, ".tiller", name)
			require.NoError(t, os.Symlink(outside, path))
			if name == "identity.yml" {
				owner, err := ReadLocalIdentity(root)
				require.Error(t, err)
				require.Empty(t, owner)
			} else {
				_, err := ReadLocalSettings(root)
				require.Error(t, err)
			}
			data, _, err := ReadRegularFileSnapshot(root, path)
			require.Error(t, err)
			require.Nil(t, data)
		})
	}
}

func TestMutationCacheRefusesOutsideSymlinks(t *testing.T) {
	for _, coordinate := range []string{".tiller", ".tiller/.cache", ".tiller/.cache/lock", ".tiller/.cache/journal.yml", ".tiller/.cache/generation"} {
		t.Run(coordinate, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", ".cache"), 0o755))
			names := []string{"generation", "console.json"}
			if coordinate == ".tiller/.cache/lock" || coordinate == ".tiller/.cache/journal.yml" {
				names = append(names, filepath.Base(coordinate))
			}
			for _, name := range names {
				require.NoError(t, os.WriteFile(filepath.Join(outside, name), []byte("external-sentinel"), 0o640))
			}
			inside := filepath.Join(root, filepath.FromSlash(coordinate))
			target := outside
			if coordinate != ".tiller" && coordinate != ".tiller/.cache" {
				target = filepath.Join(outside, filepath.Base(coordinate))
			}
			require.NoError(t, os.RemoveAll(inside))
			require.NoError(t, os.Symlink(target, inside))
			tree := &Tree{Root: root}
			if coordinate == ".tiller/.cache" || coordinate == ".tiller/.cache/generation" {
				tree.generation = "external-sentinel"
			}
			err := tree.Commit("confinement", []PlannedWrite{PlanWrite(filepath.Join(root, ".tiller", "orientation.md"), AbsentSnapshot(), []byte("inside"))})
			require.Error(t, err)
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			require.Len(t, entries, len(names), "no outside lock, journal, or temporary files may be created")
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join(outside, entry.Name()))
				require.NoError(t, err)
				require.Equal(t, "external-sentinel", string(data))
				info, err := entry.Info()
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
			}
		})
	}
}

func TestMutationCacheRetainsAnchorDuringDirectorySwap(t *testing.T) {
	t.Setenv("TILLER_FSYNC", "on")
	root, outside := t.TempDir(), t.TempDir()
	cache := filepath.Join(root, ".tiller", ".cache")
	require.NoError(t, os.MkdirAll(cache, 0o755))
	for _, name := range []string{"lock", "journal.yml", "generation"} {
		require.NoError(t, os.WriteFile(filepath.Join(outside, name), []byte("external-sentinel"), 0o644))
	}
	swapped := false
	err := (&Tree{Root: root}).commitWithRootOperationHooks("swap cache", []PlannedWrite{
		PlanWrite(filepath.Join(root, ".tiller", "orientation.md"), AbsentSnapshot(), []byte("inside")),
	}, rootOperationHooks{AfterOpenParent: func(operation, target string) {
		if swapped || operation != "write" || target != filepath.Join(".tiller", ".cache", "journal.yml") {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(cache, cache+"-retained"))
		require.NoError(t, os.Symlink(outside, cache))
	}})
	require.True(t, swapped, "exercise the journal write boundary")
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(cache+"-retained", "lock"))
	require.NoFileExists(t, filepath.Join(cache+"-retained", "journal.yml"))
	for _, name := range []string{"lock", "journal.yml", "generation"} {
		data, err := os.ReadFile(filepath.Join(outside, name))
		require.NoError(t, err)
		require.Equal(t, "external-sentinel", string(data))
	}
}

func TestRepositoryStateIOWorksWithLazyDirectories(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join(".tiller", ".cache", "console.json")
	require.NoError(t, WriteRepositoryFileAtomic(root, relative, []byte("inside")))
	data, err := ReadRepositoryFile(root, relative)
	require.NoError(t, err)
	require.Equal(t, "inside", string(data))
	directory, err := ReadDirectorySnapshot(root, filepath.Join(root, ".tiller", ".cache"))
	require.NoError(t, err)
	require.True(t, directory.Exists)
	require.Equal(t, []string{"console.json"}, directory.Names)
}

func TestStatePlanningReadersRefuseOutsideDirectories(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "outside-only.yml"), []byte("outside"), 0o644))
	path := filepath.Join(root, ".tiller", "epics")
	require.NoError(t, os.Symlink(outside, path))
	directory, err := ReadDirectorySnapshot(root, path)
	require.Error(t, err)
	require.Empty(t, directory.Names)
	inventory, err := ReadLineageInventory(root, path)
	require.Error(t, err)
	require.Empty(t, inventory.Files)
}

func TestStateAtomicWriteRefusesOutsideParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
	path := filepath.Join(outside, "identity.yml")
	require.NoError(t, os.WriteFile(path, []byte("external-sentinel"), 0o640))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", ".cache")))
	err := WriteRepositoryFileAtomic(root, filepath.Join(".tiller", ".cache", "identity.yml"), []byte("inside"))
	require.Error(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "external-sentinel", string(data))
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestStatePathCannotResetAnchorAtNestedTillerDirectory(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(outside, ".tiller"), 0o755))
	sentinel := filepath.Join(outside, ".tiller", "orientation.md")
	require.NoError(t, os.WriteFile(sentinel, []byte("external-sentinel"), 0o640))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "redirect")))
	path := filepath.Join(root, ".tiller", "redirect", ".tiller", "orientation.md")
	data, _, err := ReadRegularFileSnapshot(root, path)
	require.Error(t, err)
	require.Nil(t, data)
	require.Error(t, WriteRepositoryFileAtomic(root, filepath.Join(".tiller", "redirect", ".tiller", "orientation.md"), []byte("inside")))
	_, err = ReadDirectorySnapshot(root, filepath.Dir(path))
	require.Error(t, err)
	data, err = os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "external-sentinel", string(data))
}

// TestCheckoutUnderAncestorTillerDirectory anchors state IO at the checkout
// root the caller names, never at an ancestor directory that happens to be
// called .tiller. Reaching the checkout through a symlink under that ancestor
// must not make every state read and write cross an unchecked symlink.
func TestCheckoutUnderAncestorTillerDirectory(t *testing.T) {
	checkout := copyTree(t, fixtureTreeRoot)
	ancestor := filepath.Join(t.TempDir(), ".tiller")
	require.NoError(t, os.Mkdir(ancestor, 0o755))
	root := filepath.Join(ancestor, "checkout")
	require.NoError(t, os.Symlink(checkout, root))

	tree, err := Load(root)
	require.NoError(t, err)
	require.NotEmpty(t, tree.Epics)

	require.NoError(t, WriteLocalIdentity(root, "jc"))
	owner, err := ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", owner)

	notes := filepath.Join(root, ".tiller", "decisions", "ancestor.md")
	require.NoError(t, tree.Commit("ancestor", []PlannedWrite{PlanWrite(notes, AbsentSnapshot(), []byte("inside"))}))
	data, err := os.ReadFile(filepath.Join(checkout, ".tiller", "decisions", "ancestor.md"))
	require.NoError(t, err)
	require.Equal(t, "inside", string(data))
}

func TestRepositoryDirectoryEntriesRetainAnchoredMetadata(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	directory := filepath.Join(root, ".tiller", "epics")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "epic.yml"), []byte("inside"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "epic.yml"), []byte("private external state"), 0o600))
	entries, err := ReadRepositoryDirectory(root, filepath.Join(".tiller", "epics"))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	require.NoError(t, os.Rename(directory, directory+"-old"))
	require.NoError(t, os.Symlink(outside, directory))
	info, err := entries[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(len("inside")), info.Size(), "Info must not reopen the now-escaping directory name")
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	_, err = ReadRepositoryDirectory(root, filepath.Join(".tiller", "epics"))
	require.Error(t, err)
}

func TestRepositoryDirectoryCreationRefusesOutsideSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "design")))
	err := MkdirRepositoryAll(root, filepath.Join(".tiller", "design", "nested"), 0o755)
	require.Error(t, err)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}
