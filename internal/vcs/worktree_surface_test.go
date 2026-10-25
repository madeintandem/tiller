package vcs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestWorktreeSurfaceMatchesGitTreesAndSortedInventory(t *testing.T) {
	dir, _ := newRepo(t)
	for name, body := range map[string]string{
		"code/z.go": "last\n", "code/a/x.go": "nested\n", "code/a.go": "first\n",
		"code/a-/x.go": "before directory\n", "code/a0.go": "after directory\n", "code/ space\n.go": "unusual name\n",
	} {
		require.NoError(t, writeRepoFile(dir, name, body))
	}
	require.NoError(t, os.Chmod(filepath.Join(dir, "code/z.go"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "code/empty/nested"), 0o755))
	run(t, dir, "add", "code")
	gitTree := run(t, dir, "write-tree")
	want := "git:" + run(t, dir, "rev-parse", gitTree+":code")
	got, err := WorktreeSurface(dir, "code", "../legacy/code", clockAt)
	require.NoError(t, err)
	require.Equal(t, store.KindGitTree, got.Kind)
	require.Equal(t, want, got.Value)
	require.Equal(t, []string{"../legacy/code/ space\n.go", "../legacy/code/a-/x.go", "../legacy/code/a.go", "../legacy/code/a/x.go", "../legacy/code/a0.go", "../legacy/code/z.go"}, got.Inventory)
}

func TestWorktreeSurfaceIncludesDirtyAddedAndNotDeletedFiles(t *testing.T) {
	dir, _ := newRepo(t)
	before, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.NoError(t, writeRepoFile(dir, "app/main.go", "dirty bytes\n"))
	dirty, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, before.Value, dirty.Value)
	require.NoError(t, writeRepoFile(dir, "app/new.go", "untracked\n"))
	added, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, dirty.Value, added.Value)
	require.Equal(t, []string{"app/main.go", "app/new.go"}, added.Inventory)
	require.NoError(t, os.Remove(filepath.Join(dir, "app/main.go")))
	deleted, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, added.Value, deleted.Value)
	require.Equal(t, []string{"app/new.go"}, deleted.Inventory)
	run(t, dir, "add", "-A", "app")
	gitTree := run(t, dir, "write-tree")
	require.Equal(t, "git:"+run(t, dir, "rev-parse", gitTree+":app"), deleted.Value)
}

func TestWorktreeSurfaceHonorsIgnoreRulesExceptExplicitFiles(t *testing.T) {
	dir, _ := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".gitignore", "*.go\n!keep.go\nignored/\n"))
	require.NoError(t, writeRepoFile(dir, "app/skip.go", "ignored\n"))
	require.NoError(t, writeRepoFile(dir, "app/keep.go", "included\n"))
	require.NoError(t, writeRepoFile(dir, "app/ignored/secret", "ignored directory\n"))
	got, err := WorktreeSurface(filepath.Join(dir, "app"), ".", "app", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"app/keep.go", "app/main.go"}, got.Inventory)
	file, err := WorktreeSurface(dir, "app/skip.go", "app/skip.go", clockAt)
	require.NoError(t, err)
	require.Equal(t, store.KindGitBlob, file.Kind)
	require.Equal(t, "git:"+run(t, dir, "hash-object", "--no-filters", "app/skip.go"), file.Value)
}

func TestWorktreeSurfaceExternalTreeAndMetadataExclusion(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRepoFile(dir, "code.go", "hello\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/design/baseline.yml", "ledger\n"))
	require.NoError(t, writeRepoFile(dir, "nested/.git/private", "metadata\n"))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))
	before, err := WorktreeSurface(dir, ".", "../legacy", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"../legacy/code.go"}, before.Inventory)
	require.NoError(t, writeRepoFile(dir, ".tiller/design/baseline.yml", "new ledger\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/events.jsonl", "event\n"))
	after, err := WorktreeSurface(dir, ".", "../legacy", clockAt)
	require.NoError(t, err)
	require.Equal(t, before.Value, after.Value)
	empty, err := WorktreeSurface(dir, "empty", "../legacy/empty", clockAt)
	require.NoError(t, err)
	require.Equal(t, "git:4b825dc642cb6eb9a060e54bf8d69288fbee4904", empty.Value)
	require.Empty(t, empty.Inventory)
	// Installing Git metadata must not itself change the fingerprint.
	run(t, dir, "init")
	run(t, dir, "add", "code.go")
	require.Equal(t, "git:"+run(t, dir, "write-tree"), before.Value)
	underGit, err := WorktreeSurface(dir, ".", "../legacy", clockAt)
	require.NoError(t, err)
	require.Equal(t, before.Value, underGit.Value)
}

func TestWorktreeSurfaceExecutableModeChangesTree(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRepoFile(dir, "run", "#!/bin/sh\n"))
	plain, err := WorktreeSurface(dir, ".", ".", clockAt)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(dir, "run"), 0o755))
	executable, err := WorktreeSurface(dir, ".", ".", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, plain.Value, executable.Value)
	run(t, dir, "init")
	run(t, dir, "add", "run")
	require.Equal(t, "git:"+run(t, dir, "write-tree"), executable.Value)
}

func TestWorktreeSurfaceRefusesUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, writeRepoFile(outside, "secret", "outside\n"))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escape")))
	for _, rel := range []string{"../secret", filepath.Join(outside, "secret"), "escape", "escape/secret", "./escape/secret"} {
		_, err := WorktreeSurface(dir, rel, "surface", clockAt)
		require.ErrorIs(t, err, rootpath.ErrUnsafe, rel)
	}
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	require.ErrorIs(t, err, rootpath.ErrUnsafe)
	require.NoError(t, os.Remove(filepath.Join(dir, "escape")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))
	_, err = WorktreeSurface(dir, "pipe", "pipe", clockAt)
	require.Error(t, err)
	_, err = WorktreeSurface(dir, ".", ".", clockAt)
	require.Error(t, err)
}

func TestWorktreeSurfaceRefusesBrokenGitRatherThanChangingInventory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRepoFile(dir, ".git", "gitdir: missing\n"))
	require.NoError(t, writeRepoFile(dir, "code.go", "code\n"))
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonGitUnavailable, env.Reason)
}

func TestWorktreeSurfaceRefusesUnavailableGit(t *testing.T) {
	dir, _ := newRepo(t)
	t.Setenv("PATH", t.TempDir())
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonGitUnavailable, env.Reason)
}

func TestWorktreeSurfaceTracksFileReplacedByDirectory(t *testing.T) {
	dir, _ := newRepo(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "app/main.go")))
	require.NoError(t, writeRepoFile(dir, "app/main.go/child.go", "replacement directory\n"))
	got, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"app/main.go/child.go"}, got.Inventory)
	run(t, dir, "add", "-A", "app")
	tree := run(t, dir, "write-tree")
	require.Equal(t, "git:"+run(t, dir, "rev-parse", tree+":app"), got.Value)
}

func TestWorktreeSurfaceSubmoduleRequiresExplicitRoot(t *testing.T) {
	dir, _ := newRepo(t)
	source, _ := newRepo(t)
	run(t, dir, "clone", "--local", source, "module")
	head := run(t, source, "rev-parse", "HEAD")
	run(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",module")
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	require.Error(t, err)
	explicit, err := WorktreeSurface(dir, "module", "module", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"module/README.md", "module/app/main.go"}, explicit.Inventory)
	require.Equal(t, "git:"+run(t, source, "rev-parse", "HEAD^{tree}"), explicit.Value)
	require.NoError(t, writeRepoFile(dir, "module/app/main.go", "dirty module\n"))
	dirty, err := WorktreeSurface(dir, "module", "module", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, explicit.Value, dirty.Value)
}

func TestWorktreeSurfaceRefusesCorruptIndex(t *testing.T) {
	dir, _ := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".git/index", "broken index"))
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonGitUnavailable, env.Reason)
}

func TestWorktreeSurfaceRefusesGitEnvironmentFailureInExternalTree(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRepoFile(dir, "code.go", "code\n"))
	t.Setenv("GIT_DIR", filepath.Join(dir, "missing-repository"))
	_, err := WorktreeSurface(dir, ".", ".", clockAt)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonGitUnavailable, env.Reason)
}

func TestWorktreeSurfacePrunesMetadataCaseAliasesNotDistinctCode(t *testing.T) {
	for _, name := range []string{".TILLER", ".GIT"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, writeRepoFile(dir, "code.go", "code\n"))
			target := filepath.Join("nested", name, "payload")
			require.NoError(t, writeRepoFile(dir, target, "before\n"))
			actual, err := os.Stat(filepath.Join(dir, "nested", name))
			require.NoError(t, err)
			canonical, err := os.Stat(filepath.Join(dir, "nested", strings.ToLower(name)))
			alias := err == nil && os.SameFile(actual, canonical)
			if err != nil {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			before, err := WorktreeSurface(dir, ".", ".", clockAt)
			require.NoError(t, err)
			explicit, explicitErr := WorktreeSurface(dir, target, filepath.ToSlash(target), clockAt)
			require.NoError(t, writeRepoFile(dir, target, "after\n"))
			after, err := WorktreeSurface(dir, ".", ".", clockAt)
			require.NoError(t, err)
			if alias {
				require.Equal(t, []string{"code.go"}, before.Inventory)
				require.Equal(t, before.Value, after.Value)
				require.Error(t, explicitErr)
			} else {
				require.Equal(t, []string{"code.go", filepath.ToSlash(target)}, before.Inventory)
				require.NotEqual(t, before.Value, after.Value)
				require.NoError(t, explicitErr)
				require.Equal(t, []string{filepath.ToSlash(target)}, explicit.Inventory)
			}
		})
	}
}

func TestWorktreeSurfaceRejectsMetadataAnchorsAndResolvedAliases(t *testing.T) {
	for _, name := range []string{".git", ".tiller", ".GIT", ".TILLER"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, writeRepoFile(dir, name+"/nested/payload", "private\n"))
			actual, err := os.Stat(filepath.Join(dir, name))
			require.NoError(t, err)
			canonical, err := os.Stat(filepath.Join(dir, strings.ToLower(name)))
			metadata := err == nil && os.SameFile(actual, canonical)
			if err != nil {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			for _, suffix := range []string{"", "nested"} {
				anchor := filepath.Join(dir, name, suffix)
				alias := filepath.Join(t.TempDir(), "legacy")
				require.NoError(t, os.Symlink(anchor, alias))
				for _, declared := range []string{anchor, alias} {
					relative := "nested/payload"
					if suffix != "" {
						relative = "payload"
					}
					for _, selected := range []string{".", relative} {
						surface, err := WorktreeSurface(declared, selected, "../legacy", clockAt)
						if metadata {
							require.Error(t, err, "%s: %s", declared, selected)
						} else {
							require.NoError(t, err)
							inventory := []string{"../legacy"}
							if selected == "." {
								inventory[0] += "/" + relative
							}
							require.Equal(t, inventory, surface.Inventory)
						}
					}
				}
			}
		})
	}
}

func TestWorktreeSurfaceAllowsExternalDeclarationAlias(t *testing.T) {
	external := t.TempDir()
	require.NoError(t, writeRepoFile(external, "src/code.go", "external code\n"))
	alias := filepath.Join(t.TempDir(), "legacy")
	require.NoError(t, os.Symlink(external, alias))
	direct, err := WorktreeSurface(external, ".", "../legacy", clockAt)
	require.NoError(t, err)
	observed, err := WorktreeSurface(alias, ".", "../legacy", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"../legacy/src/code.go"}, observed.Inventory)
	require.Equal(t, direct, observed)
}
