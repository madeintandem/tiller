package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codeIdentityRepo lays the repo every CodeIdentity test starts from: a
// committed project state (.tiller/tiller.yml) and one committed source file.
func codeIdentityRepo(t *testing.T) (string, *Git) {
	t.Helper()
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema: 1\n"))
	require.NoError(t, writeRepoFile(dir, "src/a.go", "package src\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "lay project")
	return dir, g
}

// identity reads g's code identity, failing the test on error.
func identity(t *testing.T, g *Git) string {
	t.Helper()
	id, err := g.CodeIdentity()
	require.NoError(t, err)
	return id
}

func TestCodeIdentity_Shape(t *testing.T) {
	t.Parallel()
	_, g := codeIdentityRepo(t)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, identity(t, g))
}

func TestCodeIdentity_SameContentCommittedOrDirty(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	clean := identity(t, g)

	require.NoError(t, writeRepoFile(dir, "src/a.go", "package src\n\nvar x = 1\n"))
	dirty := identity(t, g)
	assert.NotEqual(t, clean, dirty)

	run(t, dir, "commit", "-am", "edit a")
	assert.Equal(t, dirty, identity(t, g), "committing the same bytes must not move the identity")
}

func TestCodeIdentity_ConversionHiddenBytesDisableReuse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		autocrlf  string
		attribute string
		filter    string
	}{
		{name: "autocrlf", autocrlf: "true"},
		{name: "autocrlf input", autocrlf: "input"},
		{name: "text", attribute: "text"},
		{name: "text auto", attribute: "text=auto"},
		{name: "eol", attribute: "eol=crlf"},
		{name: "clean filter", attribute: "filter=stripcr", filter: "stripcr"},
		{name: "filter named unset", attribute: "filter=unset", filter: "unset"},
		{name: "filter named unspecified", attribute: "filter=unspecified", filter: "unspecified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, g := codeIdentityRepo(t)
			run(t, dir, "config", "core.autocrlf", "false")
			if tc.autocrlf != "" {
				run(t, dir, "config", "core.autocrlf", tc.autocrlf)
			}
			if tc.attribute != "" {
				require.NoError(t, writeRepoFile(dir, ".gitattributes", "probe.sh "+tc.attribute+"\n"))
			}
			if tc.filter != "" {
				run(t, dir, "config", "filter."+tc.filter+".clean", `tr -d '\r'`)
			}
			require.NoError(t, writeRepoFile(dir, "probe.sh", "exit 0\n"))
			run(t, dir, "add", "-A")
			run(t, dir, "commit", "-m", "lay converted source")
			require.Empty(t, identity(t, g), "conversion cannot certify raw byte equivalence")

			require.NoError(t, writeRepoFile(dir, "probe.sh", "exit 0\r\n"))
			diff, err := g.runRaw("diff", "--name-only", "HEAD", "--", "probe.sh")
			require.NoError(t, err)
			require.Empty(t, diff, "Git must hide the actual byte change for this regression")
			require.Empty(t, identity(t, g), "a hidden unstaged change cannot reuse a pass")

			run(t, dir, "add", "probe.sh")
			diff, err = g.runRaw("diff", "--name-only", "HEAD", "--", "probe.sh")
			require.NoError(t, err)
			require.Empty(t, diff)
			require.Empty(t, identity(t, g), "staging normalized bytes must not restore reuse")
		})
	}
}

func TestCodeIdentity_ResolvesCurrentAttributesAndIndexFallback(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	run(t, dir, "config", "core.autocrlf", "false")
	require.NoError(t, writeRepoFile(dir, ".gitattributes", "src/a.go -text\n"))
	run(t, dir, "add", ".gitattributes")
	run(t, dir, "commit", "-m", "disable conversion")
	base := identity(t, g)
	require.NotEmpty(t, base)

	require.NoError(t, writeRepoFile(dir, ".gitattributes", "src/a.go text\n"))
	require.Empty(t, identity(t, g), "unstaged attributes enable normalization")
	run(t, dir, "add", ".gitattributes")
	require.Empty(t, identity(t, g), "staged attributes enable normalization")
	require.NoError(t, os.Remove(filepath.Join(dir, ".gitattributes")))
	require.Empty(t, identity(t, g), "missing worktree attributes fall back to the index")

	require.NoError(t, writeRepoFile(dir, ".gitattributes", "src/a.go -text\n"))
	require.Equal(t, base, identity(t, g), "working attributes override the index")
	require.NoError(t, writeRepoFile(dir, "src/a.go", "package src\r\n"))
	require.NotEqual(t, base, identity(t, g), "explicitly disabled conversion exposes raw changes")
}

func TestCodeIdentity_ResolvesAttributeOverrides(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"local", "global"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			dir, g := codeIdentityRepo(t)
			run(t, dir, "config", "core.autocrlf", "false")
			require.NoError(t, writeRepoFile(dir, ".gitattributes", "src/a.go -text\n"))
			run(t, dir, "add", ".gitattributes")
			run(t, dir, "commit", "-m", "disable conversion")
			if source == "local" {
				require.NoError(t, writeRepoFile(dir, ".git/info/attributes", "src/a.go text\n"))
			} else {
				// Global attributes apply to paths not overridden by .gitattributes.
				attributes := filepath.Join(t.TempDir(), "attributes")
				require.NoError(t, os.WriteFile(attributes, []byte("app/main.go text\n"), 0o644))
				run(t, dir, "config", "core.attributesFile", attributes)
			}
			require.Empty(t, identity(t, g), "effective conversion outside the worktree is still unsafe")
		})
	}
}

func TestCodeIdentity_ConversionOnlyInExcludedStateAllowsReuse(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	run(t, dir, "config", "core.autocrlf", "false")
	require.NoError(t, writeRepoFile(dir, ".gitattributes", ".tiller/** text\nartifacts/tiller/** filter=state\n"))
	run(t, dir, "add", ".gitattributes")
	run(t, dir, "commit", "-m", "state attributes")
	base := identity(t, g)
	require.NotEmpty(t, base)
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema: 1\r\n"))
	require.Equal(t, base, identity(t, g))
}

func TestCodeIdentity_IgnoresTillerState(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	base := identity(t, g)

	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema: 2\n"))
	assert.Equal(t, base, identity(t, g), "editing tracked tiller state")

	require.NoError(t, writeRepoFile(dir, ".tiller/epics/x.yml", "id: x\n"))
	assert.Equal(t, base, identity(t, g), "adding untracked tiller state")

	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "tiller state")
	assert.Equal(t, base, identity(t, g), "committing tiller state")
}

func TestCodeIdentity_IgnoresEvidenceWithoutIgnoreFile(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	base := identity(t, g)

	require.NoError(t, writeRepoFile(dir, "artifacts/tiller/gates/build-x-out-y.txt", "output\n"))
	assert.Equal(t, base, identity(t, g), "gate evidence is Tiller's own write")

	require.NoError(t, writeRepoFile(dir, "artifacts/other.txt", "not evidence\n"))
	assert.NotEqual(t, base, identity(t, g), "other artifacts are code")
}

func TestCodeIdentity_SeesCodeChanges(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	require.NoError(t, writeRepoFile(dir, ".gitignore", "ignored.log\n"))
	run(t, dir, "add", ".gitignore")
	run(t, dir, "commit", "-m", "ignore")
	base := identity(t, g)

	require.NoError(t, writeRepoFile(dir, "ignored.log", "noise\n"))
	assert.Equal(t, base, identity(t, g), "a gitignored file is not code")

	require.NoError(t, writeRepoFile(dir, "src/a.go", "package src\n// edited\n"))
	assert.NotEqual(t, base, identity(t, g), "content edit")
	run(t, dir, "checkout", "src/a.go")
	assert.Equal(t, base, identity(t, g), "reverting the edit restores it")

	require.NoError(t, writeRepoFile(dir, "src/new.go", "package src\n"))
	assert.NotEqual(t, base, identity(t, g), "untracked non-ignored file")
	require.NoError(t, os.Remove(filepath.Join(dir, "src/new.go")))
	assert.Equal(t, base, identity(t, g))

	require.NoError(t, os.Remove(filepath.Join(dir, "src/a.go")))
	assert.NotEqual(t, base, identity(t, g), "deletion")
	run(t, dir, "checkout", "src/a.go")
	assert.Equal(t, base, identity(t, g))

	require.NoError(t, os.Chmod(filepath.Join(dir, "src/a.go"), 0o755))
	assert.NotEqual(t, base, identity(t, g), "executable bit")
}

func TestCodeIdentity_NestedProjectExcludesOnlyItsOwnState(t *testing.T) {
	t.Parallel()
	dir, _ := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "svc/.tiller/x.yml", "a\n"))
	require.NoError(t, writeRepoFile(dir, "svc/main.go", "package main\n"))
	require.NoError(t, writeRepoFile(dir, "svc/sub/b.go", "package sub\n"))
	require.NoError(t, writeRepoFile(dir, "other/.tiller/x.yml", "a\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "nested project")

	g := New(filepath.Join(dir, "svc"))
	deep := New(filepath.Join(dir, "svc", "sub"))
	base := identity(t, g)
	assert.Equal(t, base, identity(t, deep), "same value from any directory in the project")

	require.NoError(t, writeRepoFile(dir, "svc/.tiller/x.yml", "b\n"))
	assert.Equal(t, base, identity(t, g), "the project's own state")

	require.NoError(t, writeRepoFile(dir, "other/.tiller/x.yml", "b\n"))
	changed := identity(t, g)
	assert.NotEqual(t, base, changed, "another project's state is code to this one")

	require.NoError(t, writeRepoFile(dir, "svc/main.go", "package main\n// edit\n"))
	edited := identity(t, g)
	assert.NotEqual(t, changed, edited, "the project's code")
	assert.Equal(t, edited, identity(t, deep))
}

func TestCodeIdentity_UnbornHead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	g := New(dir)

	empty := identity(t, g)
	require.NoError(t, writeRepoFile(dir, "a.go", "package a\n"))
	assert.NotEqual(t, empty, identity(t, g))
}

// A file staged before the first commit is in the index but in neither
// `diff HEAD` nor `ls-files --others`; it must still count as code.
func TestCodeIdentity_UnbornHeadSeesStagedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	g := New(dir)

	empty := identity(t, g)
	require.NoError(t, writeRepoFile(dir, "a.go", "package a\n"))
	run(t, dir, "add", "a.go")
	staged := identity(t, g)
	assert.NotEqual(t, empty, staged, "staging a file")

	require.NoError(t, writeRepoFile(dir, "a.go", "package a\n// edited\n"))
	edited := identity(t, g)
	assert.NotEqual(t, staged, edited, "editing the staged file")

	require.NoError(t, os.Remove(filepath.Join(dir, "a.go")))
	assert.Equal(t, empty, identity(t, g), "a staged file deleted from the worktree is gone")
}

func TestCodeIdentity_UnreadableFileIsAnEnvironmentError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	dir, g := codeIdentityRepo(t)
	require.NoError(t, os.Chmod(filepath.Join(dir, "src/a.go"), 0o000))
	_, err := g.CodeIdentity()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read")
}

// nestedRepo lays a git repository with one commit at rel inside dir.
func nestedRepo(t *testing.T, dir, rel string) string {
	t.Helper()
	nested := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(nested, 0o755))
	testgit.Init(t, nested)
	require.NoError(t, writeRepoFile(nested, "lib.go", "package lib\n"))
	run(t, nested, "add", "-A")
	run(t, nested, "commit", "-m", "lib")
	return nested
}

// An untracked nested repository reads as one opaque directory, so edits
// inside it would never move the identity: there is no identity, and nothing
// is reused.
func TestCodeIdentity_UntrackedNestedRepoHasNoIdentity(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	nestedRepo(t, dir, "vendor/lib")
	assert.Empty(t, identity(t, g))
}

// A submodule's gitlink records only its commit, so new files inside it are
// invisible: a HEAD tree carrying a gitlink has no identity.
func TestCodeIdentity_SubmoduleHasNoIdentity(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	upstream := nestedRepo(t, t.TempDir(), "up")
	run(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", upstream, "sub")
	run(t, dir, "commit", "-m", "add submodule")
	assert.Empty(t, identity(t, g))
}

// A gitlink staged on a born HEAD is no less opaque, and must be seen even
// when the project's config tells diff to ignore submodules.
func TestCodeIdentity_StagedGitlinkUnderIgnoreSubmodulesHasNoIdentity(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	run(t, dir, "config", "diff.ignoreSubmodules", "all")
	nestedRepo(t, dir, "lib")
	run(t, dir, "add", "lib")
	assert.Empty(t, identity(t, g))
}

// On an unborn HEAD the index is the tree: a staged gitlink has no identity
// either.
func TestCodeIdentity_UnbornHeadStagedGitlinkHasNoIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testgit.Init(t, dir)
	g := New(dir)
	nestedRepo(t, dir, "lib")
	run(t, dir, "add", "lib")
	assert.Empty(t, identity(t, g))
}

func TestCodeIdentity_HiddenIndexChangesDisableReuse(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			dir, g := codeIdentityRepo(t)
			require.NotEmpty(t, identity(t, g))
			run(t, dir, "update-index", flag, "src/a.go")
			require.NoError(t, writeRepoFile(dir, "src/a.go", "changed executable input\n"))
			before := gitWriteState(t, dir)
			require.Empty(t, identity(t, g))
			require.Equal(t, before, gitWriteState(t, dir), "observation must preserve the user's index flags")
		})
	}
}

func TestCodeIdentity_SuppressedExecutableModeDisablesReuse(t *testing.T) {
	t.Parallel()
	dir, g := codeIdentityRepo(t)
	require.NoError(t, writeRepoFile(dir, "probe.sh", "#!/bin/sh\nexit 0\n"))
	require.NoError(t, os.Chmod(filepath.Join(dir, "probe.sh"), 0o755))
	run(t, dir, "add", "probe.sh")
	run(t, dir, "commit", "-m", "executable probe")
	require.NotEmpty(t, identity(t, g))
	run(t, dir, "config", "core.filemode", "false")
	require.NoError(t, os.Chmod(filepath.Join(dir, "probe.sh"), 0o644))
	require.Empty(t, identity(t, g), "Git's ignored executable-mode changes cannot certify a passing command")
}
