package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// These tests cover Git routing inherited from the process environment: the
// variables that tell Git which repository, work tree, index, or object store
// to use instead of discovering them from its working directory. An
// observation of checkout A must never answer for repository B, and a routed
// environment that agrees with A (a Git hook, a linked worktree) must observe
// exactly what discovery observes.

// routedRepos builds two unrelated repositories that both sit on branch main:
// selected is the checkout Tiller was asked to observe, and other is the one an
// inherited environment points at. Each carries a nested proj/ directory and
// one uncommitted edit, so every observation has dirty content to read.
func routedRepos(t *testing.T) (selected, other string) {
	t.Helper()
	build := func(name string) string {
		dir, _ := newRepo(t)
		physical, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		require.NoError(t, writeRepoFile(physical, "proj/code.go", "package "+name+"\n"))
		run(t, physical, "add", "-A")
		run(t, physical, "commit", "-m", name)
		require.NoError(t, writeRepoFile(physical, "proj/code.go", "package "+name+" // dirty\n"))
		return physical
	}
	return build("selected"), build("other")
}

// observation is everything one checkout answers through the routed process
// paths: plain commands, the repository-root rerooting CodeIdentity and
// WorktreeFingerprint do, the two-process pipeline, and the surface inventory.
type observation struct {
	Head, Code, Fingerprint, Surface string
	Dirty                            bool
	Patches                          []CommitPatch
}

func observe(dir string) (observation, error) {
	g := New(dir)
	g.Clock = fixedClock{t: clockAt}
	var seen observation
	var err error
	if seen.Head, err = g.Head(); err != nil {
		return seen, err
	}
	if seen.Dirty, err = g.IsDirty(); err != nil {
		return seen, err
	}
	if seen.Code, err = g.CodeIdentity(); err != nil {
		return seen, err
	}
	fingerprint, err := g.WorktreeFingerprint()
	if err != nil {
		return seen, err
	}
	seen.Fingerprint = fingerprint.Value
	if seen.Patches, err = g.FirstParentPatchIDs("main", 5); err != nil {
		return seen, err
	}
	surface, err := WorktreeSurface(dir, ".", ".", clockAt)
	if err != nil {
		return seen, err
	}
	seen.Surface = surface.Value
	return seen, nil
}

func requireRoutingConflict(t *testing.T, err error) {
	t.Helper()
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, "git_routing_conflict", env.Reason)
	require.Equal(t, 3, env.Code)
}

// Serial: t.Setenv routes Git through the process environment.
func TestRoutedEnvironmentCannotSupplyAnotherRepositorysIdentity(t *testing.T) {
	selected, other := routedRepos(t)
	otherIdentity, err := New(other).CodeIdentity()
	require.NoError(t, err)

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)

	got, err := New(selected).CodeIdentity()
	require.Errorf(t, err, "the selected checkout answered with identity %s; the other repository's identity is %s", got, otherIdentity)
	requireRoutingConflict(t, err)
	require.Empty(t, got)
}

// Serial: t.Setenv routes Git through the process environment.
func TestRoutingThatContradictsTheSelectedCheckoutRefuses(t *testing.T) {
	selected, other := routedRepos(t)
	cases := map[string]map[string]string{
		"repository and work tree":   {"GIT_DIR": filepath.Join(other, ".git"), "GIT_WORK_TREE": other},
		"repository alone":           {"GIT_DIR": filepath.Join(other, ".git")},
		"work tree alone":            {"GIT_WORK_TREE": other},
		"common directory":           {"GIT_COMMON_DIR": filepath.Join(other, ".git")},
		"index":                      {"GIT_INDEX_FILE": filepath.Join(other, ".git", "index")},
		"index that does not exist":  {"GIT_INDEX_FILE": filepath.Join(selected, "elsewhere", "index")},
		"object directory":           {"GIT_OBJECT_DIRECTORY": filepath.Join(other, ".git", "objects")},
		"repository that is missing": {"GIT_DIR": filepath.Join(selected, "missing-repository")},
		"relative repository":        {"GIT_DIR": "no-such-directory/.git"},
		"own repository, other tree": {"GIT_DIR": filepath.Join(selected, ".git"), "GIT_WORK_TREE": other},
	}
	for name, environment := range cases {
		t.Run(name, func(t *testing.T) {
			for variable, value := range environment {
				t.Setenv(variable, value)
			}
			for _, dir := range []string{selected, filepath.Join(selected, "proj")} {
				seen, err := observe(dir)
				require.Errorf(t, err, "observing %s answered %+v", dir, seen)
				requireRoutingConflict(t, err)
			}
		})
	}
}

// Serial: t.Setenv routes Git through the process environment, and t.Chdir
// gives a relative GIT_DIR the directory Tiller was started in.
func TestRoutingThatAgreesWithTheSelectedCheckoutObservesIt(t *testing.T) {
	selected, _ := routedRepos(t)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(selected, alias))
	want := map[string]observation{}
	for _, dir := range []string{selected, filepath.Join(selected, "proj")} {
		seen, err := observe(dir)
		require.NoError(t, err)
		want[dir] = seen
	}
	gitDir := filepath.Join(selected, ".git")
	cases := map[string]map[string]string{
		"repository":                    {"GIT_DIR": gitDir},
		"repository and work tree":      {"GIT_DIR": gitDir, "GIT_WORK_TREE": selected},
		"repository through a symlink":  {"GIT_DIR": filepath.Join(alias, ".git"), "GIT_WORK_TREE": alias},
		"relative repository":           {"GIT_DIR": ".git"},
		"commit hook index":             {"GIT_INDEX_FILE": ".git/index"},
		"commit hook temporary index":   {"GIT_INDEX_FILE": filepath.Join(gitDir, "index.lock")},
		"common and object directories": {"GIT_COMMON_DIR": gitDir, "GIT_OBJECT_DIRECTORY": filepath.Join(gitDir, "objects")},
	}
	for name, environment := range cases {
		t.Run(name, func(t *testing.T) {
			t.Chdir(selected)
			for variable, value := range environment {
				t.Setenv(variable, value)
			}
			for dir, expected := range want {
				seen, err := observe(dir)
				require.NoError(t, err)
				require.Equal(t, expected, seen, "observing %s", dir)
			}
		})
	}
}

// Git exports GIT_DIR, and for a commit hook GIT_INDEX_FILE, to every child it
// starts inside a linked worktree, without a GIT_WORK_TREE.
//
// Serial: t.Setenv routes Git through the process environment.
func TestLinkedWorktreeRoutingBelongsToThatWorktreeOnly(t *testing.T) {
	selected, _ := routedRepos(t)
	linked := filepath.Join(filepath.Dir(selected), "linked")
	run(t, selected, "worktree", "add", "-q", "-b", "unit", linked)
	require.NoError(t, writeRepoFile(linked, "proj/code.go", "package linked // dirty\n"))
	linkedGitDir := filepath.Join(selected, ".git", "worktrees", "linked")
	want := map[string]observation{}
	for _, dir := range []string{linked, filepath.Join(linked, "proj")} {
		seen, err := observe(dir)
		require.NoError(t, err)
		want[dir] = seen
	}

	t.Setenv("GIT_DIR", linkedGitDir)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(linkedGitDir, "index"))

	for dir, expected := range want {
		seen, err := observe(dir)
		require.NoError(t, err)
		require.Equal(t, expected, seen, "observing %s", dir)
	}
	seen, err := observe(selected)
	require.Errorf(t, err, "the primary checkout answered %+v under the linked worktree's routing", seen)
	requireRoutingConflict(t, err)
}

// A directory with no repository of its own is an external tree. An inherited
// override must not adopt it into the repository the override names, whose
// index would then decide which of its files count.
//
// Serial: t.Setenv routes Git through the process environment.
func TestRoutingCannotAdoptADirectoryWithNoRepository(t *testing.T) {
	_, other := routedRepos(t)
	plain := t.TempDir()
	require.NoError(t, writeRepoFile(plain, "code.go", "package plain\n"))
	want, err := WorktreeSurface(plain, ".", ".", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"code.go"}, want.Inventory)

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	seen, err := WorktreeSurface(plain, ".", ".", clockAt)
	require.Errorf(t, err, "the external tree answered %+v", seen)
	requireRoutingConflict(t, err)
	_, err = New(plain).Head()
	requireRoutingConflict(t, err)
}

// A fixture's own git commands build a repository in the directory they are
// given, whatever routing the test process inherited.
//
// Serial: t.Setenv routes Git through the process environment.
func TestFixtureGitIgnoresInheritedRouting(t *testing.T) {
	_, other := routedRepos(t)
	otherHead := run(t, other, "rev-parse", "HEAD")
	otherStatus := run(t, other, "status", "--porcelain")

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))

	fixture, _ := newRepo(t)
	require.DirExists(t, filepath.Join(fixture, ".git"))
	require.Equal(t, "init", run(t, fixture, "log", "-1", "--format=%s"))
	for _, helper := range []func(*testing.T, string, ...string) string{run, gitIn} {
		require.NotEqual(t, otherHead, helper(t, fixture, "rev-parse", "HEAD"))
	}

	require.Equal(t, testgit.RoutingVariables(), routingVariables, "fixtures and observations agree on what routes Git")
	require.Equal(t, otherHead, run(t, other, "rev-parse", "HEAD"), "a fixture commit landed in the other repository")
	require.Equal(t, otherStatus, run(t, other, "status", "--porcelain"))
}
