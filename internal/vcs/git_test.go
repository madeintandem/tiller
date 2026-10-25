package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedClock is the injected time source that makes CurrentCommit's ObservedAt
// deterministic in tests.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var clockAt = time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

// run executes a git command in dir and fails the test on error. It is the test
// harness's own raw git, used both to build fixtures and to compute the
// expected hashes the helpers are checked against.
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// writeRepoFile writes rel (a repo-relative path) under dir, creating parent
// directories as needed.
func writeRepoFile(dir, rel, content string) error {
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// newRepo builds a fresh single-commit git repo on branch main with a README and
// an app/ subtree, and returns its root plus a *Git wired to a fixed clock.
func newRepo(t *testing.T) (string, *Git) {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)

	require.NoError(t, writeRepoFile(dir, "README.md", "hello\n"))
	require.NoError(t, writeRepoFile(dir, "app/main.go", "package main\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "init")

	g := New(dir)
	g.Clock = fixedClock{t: clockAt}
	return dir, g
}

func TestBranch(t *testing.T) {
	_, g := newRepo(t)
	branch, err := g.Branch()
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestLocalBranchesExcludesTagsAndPreservesNestedNames(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "branch", "tiller/pay/checkout/alfa")
	run(t, dir, "tag", "tiller/pay/checkout")
	before := run(t, dir, "show-ref")
	branches, err := g.LocalBranches()
	require.NoError(t, err)
	require.Equal(t, []string{"main", "tiller/pay/checkout/alfa"}, branches)
	require.Equal(t, before, run(t, dir, "show-ref"), "enumeration never changes refs")
}

func TestCommitsBetweenListsTipCommitsNewestFirst(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "checkout", "-b", "tiller/reviews/features/pay/checkout")
	for _, name := range []string{"first", "second"} {
		require.NoError(t, writeRepoFile(dir, name+".txt", name+"\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", name+" commit")
	}
	before := run(t, dir, "show-ref")
	commits, err := g.CommitsBetween("main", "tiller/reviews/features/pay/checkout")
	require.NoError(t, err)
	require.Len(t, commits, 2)
	require.Regexp(t, `^[0-9a-f]{7,40} second commit$`, commits[0])
	require.Regexp(t, `^[0-9a-f]{7,40} first commit$`, commits[1])
	require.Equal(t, before, run(t, dir, "show-ref"), "listing never changes refs")

	none, err := g.CommitsBetween("tiller/reviews/features/pay/checkout", "main")
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestHead(t *testing.T) {
	dir, g := newRepo(t)
	head, err := g.Head()
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{7,40}$`), head)
	assert.Equal(t, run(t, dir, "rev-parse", "--short", "HEAD"), head)
}

func TestCurrentCommit(t *testing.T) {
	dir, g := newRepo(t)
	ref, err := g.CurrentCommit()
	require.NoError(t, err)

	assert.Equal(t, store.KindGitCommit, ref.Kind)
	assert.Equal(t, "git:"+run(t, dir, "rev-parse", "--short", "HEAD"), ref.Value)
	assert.Empty(t, ref.Path, "a bare commit carries no path")
	assert.True(t, ref.ObservedAt.Equal(clockAt), "ObservedAt comes from the injected clock")
}

func TestBlobHash(t *testing.T) {
	dir, g := newRepo(t)
	got, err := g.BlobHash("README.md")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "rev-parse", "HEAD:README.md"), got)
	assert.Equal(t, run(t, dir, "hash-object", "README.md"), got)
}

func TestBlobHash_NotVersioned(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.BlobHash("does/not/exist.md")
	require.Error(t, err)
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	assert.Equal(t, "does/not/exist.md", nv.Path)
}

func TestBlobHash_OnDirectoryIsNotVersioned(t *testing.T) {
	_, g := newRepo(t)
	// app/ is a tree, not a blob: asking for its blob hash is a typed not-found,
	// not a silently-wrong tree hash.
	_, err := g.BlobHash("app")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
}

func TestWorktreeBlobHash(t *testing.T) {
	dir, g := newRepo(t)

	// For committed, unedited content the working-tree hash equals the committed
	// blob hash: hash-object and rev-parse HEAD:path agree.
	got, err := g.WorktreeBlobHash("README.md")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "hash-object", "README.md"), got)
	assert.Equal(t, run(t, dir, "rev-parse", "HEAD:README.md"), got)

	// A bare working-tree edit (no commit) changes the working-tree hash while the
	// committed blob hash is unchanged: this is what the active-basis guard reads.
	require.NoError(t, writeRepoFile(dir, "README.md", "hello, edited\n"))
	edited, err := g.WorktreeBlobHash("README.md")
	require.NoError(t, err)
	assert.NotEqual(t, got, edited, "an uncommitted edit changes the working-tree blob hash")
	committed, err := g.BlobHash("README.md")
	require.NoError(t, err)
	assert.Equal(t, got, committed, "the committed blob hash is unchanged before any commit")
}

func TestWorktreeBlobHash_Untracked(t *testing.T) {
	dir, g := newRepo(t)
	// A brand-new, never-committed file still has a working-tree blob hash.
	require.NoError(t, writeRepoFile(dir, "new.txt", "fresh\n"))
	got, err := g.WorktreeBlobHash("new.txt")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "hash-object", "new.txt"), got)
}

func TestWorktreeBlobHash_Missing(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.WorktreeBlobHash("does/not/exist.md")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
}

// TestNotVersionedErrorDoesNotClaimGitIsUnawareOfTheFile pins the honesty of the
// default message on the read that can contradict it. WorktreeBlobHash hashes
// the file as it sits on disk, so a file git tracks at HEAD and that is merely
// deleted or renamed in the working tree fails this read while `git ls-files`
// still lists it. The old message said the path "is not under version control",
// a claim about git's index that only the HEAD readers support, and here it was
// simply false. The message now reports the read that failed and stops there.
// internal/cli/sync.go settled the same wording at the user-facing surface
// (TestSyncSkipNamesThePathWithoutClaimingGitDoesNotTrackIt); this pins it at
// the source so the two cannot drift apart.
func TestNotVersionedErrorDoesNotClaimGitIsUnawareOfTheFile(t *testing.T) {
	dir, g := newRepo(t)

	// README.md is committed at HEAD and then removed from the working tree only.
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))
	require.Contains(t, run(t, dir, "ls-files"), "README.md",
		"git still tracks the file, which is what makes the old wording a false claim")

	_, err := g.WorktreeBlobHash("README.md")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	assert.NotContains(t, nv.Error(), "not under version control",
		"the message must not assert git does not track a file git tracks")
	assert.Contains(t, nv.Error(), "README.md", "the message names the path, since the path is the fix")

	// The HEAD readers share the message and stay just as honest: a path git
	// genuinely has never seen fails the same way and reads the same way.
	_, err = g.BlobHash("does/not/exist.md")
	require.ErrorAs(t, err, &nv)
	assert.NotContains(t, nv.Error(), "not under version control")
}

func TestTreeHash(t *testing.T) {
	dir, g := newRepo(t)
	got, err := g.TreeHash("app")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "rev-parse", "HEAD:app"), got)
}

func TestTreeHash_Root(t *testing.T) {
	dir, g := newRepo(t)
	got, err := g.TreeHash("")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "rev-parse", "HEAD^{tree}"), got)
}

func TestTreeHash_OnFileIsNotVersioned(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.TreeHash("README.md")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
}

func TestIsDirty(t *testing.T) {
	dir, g := newRepo(t)

	// A freshly committed repo is clean.
	dirty, err := g.IsDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "a fully committed working tree is clean")

	// A tracked-file edit makes it dirty.
	require.NoError(t, writeRepoFile(dir, "README.md", "hello, edited\n"))
	dirty, err = g.IsDirty()
	require.NoError(t, err)
	assert.True(t, dirty, "an uncommitted edit makes the tree dirty")

	// Commit the edit; clean again. A brand-new untracked file then makes it dirty.
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "edit")
	dirty, err = g.IsDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "committing the edit returns the tree to clean")

	require.NoError(t, writeRepoFile(dir, "untracked.txt", "new\n"))
	dirty, err = g.IsDirty()
	require.NoError(t, err)
	assert.True(t, dirty, "an untracked, non-ignored file makes the tree dirty")
}

// TestDirtyUnder pins the scoped dirty-paths helper (issue #44): a modified
// tracked file and an untracked file under the prefix both surface, sorted,
// while a dirty file OUTSIDE the prefix never appears, and a clean prefix
// answers empty.
func TestDirtyUnder(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema_version: 6\n"))
	require.NoError(t, writeRepoFile(dir, "outside.txt", "committed\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "seed .tiller")

	// A clean prefix answers empty.
	paths, err := g.DirtyUnder(".tiller")
	require.NoError(t, err)
	assert.Empty(t, paths, "nothing under .tiller/ has changed yet")

	// A tracked-file edit under the prefix, an untracked file under the
	// prefix, and a dirty file OUTSIDE the prefix.
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema_version: 6\nschema_epoch: initial\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/events/E-000001.yml", "kind: attest\n"))
	require.NoError(t, writeRepoFile(dir, "outside.txt", "edited, but not under .tiller/\n"))

	paths, err = g.DirtyUnder(".tiller")
	require.NoError(t, err)
	assert.Equal(t, []string{".tiller/events/E-000001.yml", ".tiller/tiller.yml"}, paths,
		"only the prefix's own modified and untracked paths surface, sorted")
}

func TestDirtyUnderPreservesIndexBytes(t *testing.T) {
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema_version: 6\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "seed .tiller")
	indexPath := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(indexPath)
	require.NoError(t, err)

	// Unchanged content with stale stat data normally makes git status rewrite
	// the index. Reading Tiller state must not perform that optional write.
	old := time.Unix(1, 0)
	require.NoError(t, os.Chtimes(filepath.Join(dir, ".tiller", "tiller.yml"), old, old))
	paths, err := g.DirtyUnder(".tiller")
	require.NoError(t, err)
	require.Empty(t, paths)
	after, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "a scoped status read must leave the index byte-identical")
}

// TestDirtyUnder_NonASCIIPathIsReportedExactly is a regression test: the
// plain `git status --porcelain` format quotes any path with an "unusual"
// byte and, under core.quotepath's default of true, C-style octal-escapes
// every non-ASCII byte in it, so a naive parse that only strips the
// surrounding quotes hands back the literal escape sequence rather than the
// real path. DirtyUnder shells -z specifically to bypass that quoting; this
// pins that an untracked file with a non-ASCII name comes back as the exact
// path that exists on disk, not git's escaped rendering of it.
func TestDirtyUnder_NonASCIIPathIsReportedExactly(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "config", "core.quotepath", "true")
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema_version: 6\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "seed .tiller")

	require.NoError(t, writeRepoFile(dir, ".tiller/café.yml", "kind: attest\n"))

	paths, err := g.DirtyUnder(".tiller")
	require.NoError(t, err)
	assert.Equal(t, []string{".tiller/café.yml"}, paths,
		"the real UTF-8 path is returned, not core.quotepath's octal escape")
}

// TestDirtyUnder_RenameWithinPrefixReportsOnlyTheNewPath is a regression
// test for the -z rename entry shape: a rename or copy is TWO NUL-delimited
// fields, "XY new-path" followed by a bare old-path field with no XY code,
// and a parse that does not know to skip that second field would either
// misread it as its own "entry" (an out-of-bounds slice on a status-code cut
// that assumes every field starts with one) or, worse, surface an old path
// that sits outside prefix as if it were one of DirtyUnder's own results. A
// rename entirely within the scoped prefix is what exercises the two-field
// shape at all: git's own pathspec scoping already downgrades a rename that
// crosses the prefix boundary to a plain add/delete pair, so this stays
// inside .tiller/ on both sides to actually reach the R status code.
func TestDirtyUnder_RenameWithinPrefixReportsOnlyTheNewPath(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".tiller/tiller.yml", "schema_version: 6\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/events/E-000001.yml", "kind: attest\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "seed .tiller")

	run(t, dir, "mv", ".tiller/events/E-000001.yml", ".tiller/events/E-000002.yml")

	// Confirm this is really exercising a rename (R), not an add/delete pair,
	// before trusting what DirtyUnder reports about it.
	require.Contains(t, run(t, dir, "status", "--porcelain", "--", ".tiller"), "R  ")

	paths, err := g.DirtyUnder(".tiller")
	require.NoError(t, err)
	assert.Equal(t, []string{".tiller/events/E-000002.yml"}, paths,
		"only the new path surfaces; the old path is consumed, not reported, "+
			"and tiller.yml is untouched so it stays out of the result")
}

func TestWorktreeFingerprint(t *testing.T) {
	dir, g := newRepo(t)

	// Dirty the tree so the fingerprint has content to digest.
	require.NoError(t, writeRepoFile(dir, "app/main.go", "package main\n\nfunc main() {}\n"))

	ref, err := g.WorktreeFingerprint()
	require.NoError(t, err)
	assert.Equal(t, store.KindSHA256, ref.Kind, "a working-tree fingerprint is a sha256 ref")
	assert.True(t, strings.HasPrefix(ref.Value, "sha256:"), "the value carries the sha256 prefix")
	assert.Empty(t, ref.Path, "a content fingerprint carries no path")
	assert.True(t, ref.ObservedAt.Equal(clockAt), "ObservedAt comes from the injected clock")

	// Stable across two reads with no change in between.
	again, err := g.WorktreeFingerprint()
	require.NoError(t, err)
	assert.Equal(t, ref.Value, again.Value, "the fingerprint is deterministic for an unchanged tree")

	// An edit to the working-tree content changes the fingerprint.
	require.NoError(t, writeRepoFile(dir, "app/main.go", "package main\n\nfunc main() { _ = 1 }\n"))
	edited, err := g.WorktreeFingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, ref.Value, edited.Value, "editing tracked content changes the fingerprint")

	// A new untracked, non-ignored file also changes the fingerprint.
	require.NoError(t, writeRepoFile(dir, "extra.txt", "more\n"))
	withUntracked, err := g.WorktreeFingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, edited.Value, withUntracked.Value, "an untracked file changes the fingerprint")
}

func TestIsMergedInto(t *testing.T) {
	dir, g := newRepo(t)

	// A feature branch with a commit, merged back into main, is contained in main.
	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "feat.txt", "feature work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "feature commit")
	run(t, dir, "checkout", "main")
	run(t, dir, "merge", "--no-ff", "-m", "merge feature", "feature")

	merged, err := g.IsMergedInto("feature", "main")
	require.NoError(t, err)
	assert.True(t, merged, "feature was merged into main")

	// A solo branch with a commit main has never seen is not contained in main,
	// but main is contained in it.
	run(t, dir, "checkout", "-b", "solo", "main")
	require.NoError(t, writeRepoFile(dir, "solo.txt", "solo work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "solo commit")

	notMerged, err := g.IsMergedInto("solo", "main")
	require.NoError(t, err)
	assert.False(t, notMerged, "solo's commit is not in main")

	ancestor, err := g.IsMergedInto("main", "solo")
	require.NoError(t, err)
	assert.True(t, ancestor, "main is an ancestor of solo")
}

func TestIsMergedInto_NonexistentBranchIsNotMerged(t *testing.T) {
	_, g := newRepo(t)
	// A branch ref that does not resolve (never created, or merged and deleted) is
	// observed as not merged, not an environment error: sync calls this with the
	// recorded branch, which may not exist for a feature whose work has not branched.
	merged, err := g.IsMergedInto("tiller/never-created", "main")
	require.NoError(t, err)
	assert.False(t, merged)
}

// newRepoWithBareRemote builds on newRepo with a bare "remote" repository
// alongside it and main pushed and tracked, the scratch harness the review-
// state warning (issue #42) needs: a real push boundary to advance ahead of
// or stay even with, with no GitHub or network involved.
func newRepoWithBareRemote(t *testing.T) (dir string, bareDir string, g *Git) {
	t.Helper()
	dir, g = newRepo(t)
	bareDir = t.TempDir()
	run(t, bareDir, "init", "--bare")
	run(t, dir, "remote", "add", "origin", bareDir)
	run(t, dir, "push", "-u", "origin", "main")
	return dir, bareDir, g
}

// TestUnpushedTip_LocalAheadOfUpstream is the warning's own trigger: main is
// pushed and tracked, then advanced with one more local commit that never
// reaches the bare remote. The local tip is not an ancestor of its upstream,
// so unpushed is true and upstream names the configured @{upstream}.
func TestUnpushedTip_LocalAheadOfUpstream(t *testing.T) {
	dir, _, g := newRepoWithBareRemote(t)

	require.NoError(t, writeRepoFile(dir, "advance.txt", "local only\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "advance past the remote")

	unpushed, upstream, ok, err := g.UnpushedTip("main")
	require.NoError(t, err)
	require.True(t, ok, "a branch with a configured upstream resolves")
	assert.True(t, unpushed, "the local tip holds a commit the remote does not")
	assert.Equal(t, "origin/main", upstream)
}

// TestUnpushedTip_PushedEven is the all-clear: main is pushed and tracked, and
// nothing has advanced past that push, so the local tip is exactly its
// upstream (an ancestor of itself) and unpushed is false.
func TestUnpushedTip_PushedEven(t *testing.T) {
	_, _, g := newRepoWithBareRemote(t)

	unpushed, upstream, ok, err := g.UnpushedTip("main")
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, unpushed, "the local tip is already on the remote")
	assert.Equal(t, "origin/main", upstream)
}

// TestUnpushedTip_NoRemoteIsSilent is the offline path the global constraint
// names: a repo with no remote at all (a dry run, local exploration) has
// nothing to compare a branch's tip against, so ok is false and err is nil,
// never a warning and never a failure.
func TestUnpushedTip_NoRemoteIsSilent(t *testing.T) {
	_, g := newRepo(t)

	unpushed, upstream, ok, err := g.UnpushedTip("main")
	require.NoError(t, err)
	assert.False(t, ok, "no remote counterpart at all is the offline path")
	assert.False(t, unpushed)
	assert.Empty(t, upstream)
}

// TestUnpushedTip_FallsBackToOriginBranchWithNoConfiguredUpstream covers the
// second resolution arm the brief names: a branch pushed without -u (so
// branch.main.merge is never set, and main@{upstream} does not resolve) still
// has an ordinary origin/main to compare against, and UnpushedTip finds it.
func TestUnpushedTip_FallsBackToOriginBranchWithNoConfiguredUpstream(t *testing.T) {
	repoDir, g := newRepo(t)
	bare := t.TempDir()
	run(t, bare, "init", "--bare")
	run(t, repoDir, "remote", "add", "origin", bare)
	run(t, repoDir, "push", "origin", "main") // no -u: no configured upstream

	require.NoError(t, writeRepoFile(repoDir, "advance.txt", "local only\n"))
	run(t, repoDir, "add", "-A")
	run(t, repoDir, "commit", "-m", "advance past the remote")

	unpushed, upstream, ok, err := g.UnpushedTip("main")
	require.NoError(t, err)
	require.True(t, ok, "origin/main resolves even with no configured @{upstream}")
	assert.True(t, unpushed)
	assert.Equal(t, "origin/main", upstream)
}

// TestUnpushedTip_UnresolvableBranchIsNoRemoteCounterpart covers a branch name
// that never existed locally at all (the recorded branch on a feature whose
// work never actually checked one out): main@{upstream} does not resolve and
// neither does origin/<branch>, so this reads the same as no remote, ok=false
// with no error, rather than surfacing a git fault for a name that was never
// real.
func TestUnpushedTip_UnresolvableBranchIsNoRemoteCounterpart(t *testing.T) {
	_, _, g := newRepoWithBareRemote(t)

	unpushed, upstream, ok, err := g.UnpushedTip("tiller/never-checked-out")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.False(t, unpushed)
	assert.Empty(t, upstream)
}

// TestUnpushedTip_TagShadowsBranchName is the tag-shadow hazard the batch
// review flagged (final review finding 1, issue #42): git's rev-parse
// precedence resolves a bare name from refs/tags before refs/heads
// (mutate/sync.go's own branchRef doc comment names the identical hazard for
// observeMerge). A tag sharing the feature branch's name, pointing at the
// commit already on the remote, must never make an unpushed branch read as
// pushed, and the commit named in the review warning must be the branch's
// own, not the tag's.
func TestUnpushedTip_TagShadowsBranchName(t *testing.T) {
	dir, _, g := newRepoWithBareRemote(t)

	run(t, dir, "checkout", "-b", "feat")
	run(t, dir, "push", "-u", "origin", "feat")

	// A tag named identically to the branch, planted at the commit already on
	// the remote: git's own precedence resolves a bare "feat" from this tag
	// before the branch, so an unqualified read of "feat" sees the tag's tip
	// (fully pushed) instead of the branch's own.
	run(t, dir, "tag", "feat")

	require.NoError(t, writeRepoFile(dir, "advance.txt", "local only\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "advance past the remote")
	branchSHA := run(t, dir, "rev-parse", "--short", "refs/heads/feat")
	tagSHA := run(t, dir, "rev-parse", "--short", "refs/tags/feat")
	require.NotEqual(t, branchSHA, tagSHA, "the fixture only proves the hazard if the two names diverge")

	unpushed, upstream, ok, err := g.UnpushedTip("feat")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, unpushed, "the BRANCH's tip is unpushed even though the same-named tag is fully pushed")
	assert.Equal(t, "origin/feat", upstream)

	commit, err := g.ShortSHA(BranchRef("feat"))
	require.NoError(t, err)
	assert.Equal(t, branchSHA, commit, "the review warning names the branch's own commit, not the shadowing tag's")
}

func TestShortSHA(t *testing.T) {
	dir, g := newRepo(t)
	got, err := g.ShortSHA("main")
	require.NoError(t, err)
	assert.Equal(t, run(t, dir, "rev-parse", "--short", "main"), got)
}

func TestShortSHA_UnresolvableRefIsGitUnavailable(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.ShortSHA("does-not-exist")
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

// TestWorktreeFor_FindsWorktreeHoldingBranch is the positive case the
// accept-fast-forward remediation depends on: a branch checked out in a
// second worktree (the field shape, a branch parked there so the human's
// primary checkout can stay on main) is found by name, and the path reported
// is exactly where `git worktree add` put it.
func TestWorktreeFor_FindsWorktreeHoldingBranch(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "branch", "feat")
	wtDir := t.TempDir()
	run(t, dir, "worktree", "add", wtDir, "feat")

	path, ok, err := g.WorktreeFor("feat")
	require.NoError(t, err)
	require.True(t, ok)
	// git reports the worktree's resolved path (macOS's /tmp is itself a
	// symlink into /private), so the comparison resolves both sides rather
	// than asserting a spelling neither this test nor WorktreeFor controls.
	want, err := filepath.EvalSymlinks(wtDir)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestWorktreeFor_MissesUnheldBranch is the negative case: a branch that
// exists but is checked out nowhere (the ordinary state of a feature branch
// between `start` and any checkout of it) reports ok=false rather than a
// stale or guessed path.
func TestWorktreeFor_MissesUnheldBranch(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "branch", "feat")

	path, ok, err := g.WorktreeFor("feat")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, path)
}

// TestWorktreeFor_PrimaryWorktreeCounts is the nuance the doc comment names:
// git's own porcelain output lists the primary worktree exactly like any
// other, so a branch checked out in the ordinary working directory (never
// touched by `git worktree add` at all) is found here too. This is what
// keeps the remediation from prescribing a bare `git branch -f` against the
// one worktree every project already has, which would refuse identically to
// a linked one.
func TestWorktreeFor_PrimaryWorktreeCounts(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "checkout", "-b", "feat")

	path, ok, err := g.WorktreeFor("feat")
	require.NoError(t, err)
	require.True(t, ok)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGitUnavailable_NotARepo(t *testing.T) {
	dir := t.TempDir() // a real dir that is not a git repo
	g := New(dir)
	_, err := g.Branch()
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

func TestGitUnavailable_BinaryMissing(t *testing.T) {
	t.Setenv("PATH", "") // hide the git binary from exec.LookPath
	g := New(t.TempDir())
	_, err := g.Head()
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

func TestHasRemote(t *testing.T) {
	dir, g := newRepo(t)

	// A fresh local repo has no remote: the offline case submit's branch-ref
	// fallback serves (dry runs, local exploration).
	has, err := g.HasRemote()
	require.NoError(t, err)
	assert.False(t, has, "a fresh local repo has no remote")

	// Any configured remote flips the answer; the path never needs to resolve,
	// because `git remote` is a purely local listing (no network).
	run(t, dir, "remote", "add", "origin", "/tmp/anything.git")
	has, err = g.HasRemote()
	require.NoError(t, err)
	assert.True(t, has, "a configured remote is detected without any network")
}

func TestHasRemote_NotARepo(t *testing.T) {
	g := New(t.TempDir())
	_, err := g.HasRemote()
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

func TestOriginURL(t *testing.T) {
	dir, g := newRepo(t)

	// A fresh repo has no origin: `git remote get-url origin` exits non-zero,
	// which OriginURL reports as the absence of an origin (empty, no error), not
	// an environment fault. A page deriving a PR link over it simply omits one.
	url, err := g.OriginURL()
	require.NoError(t, err, "a missing origin is not an error the page must survive")
	assert.Empty(t, url)

	// A configured origin echoes back verbatim; the URL never needs to resolve,
	// because get-url is a purely local config read (no network).
	run(t, dir, "remote", "add", "origin", "git@github.com:madeintandem/tiller.git")
	url, err = g.OriginURL()
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:madeintandem/tiller.git", url)
}

func TestOriginURL_NotARepo(t *testing.T) {
	// A directory that is not a git repository exits non-zero on get-url, the
	// same "no origin" answer: the console never 500s over a non-repo root.
	g := New(t.TempDir())
	url, err := g.OriginURL()
	require.NoError(t, err)
	assert.Empty(t, url)
}

func TestOriginURL_BinaryMissing(t *testing.T) {
	// A git that cannot be launched at all is the one environment error
	// OriginURL surfaces; the caller (serve) treats any error as "no PR link".
	t.Setenv("PATH", "")
	g := New(t.TempDir())
	_, err := g.OriginURL()
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

// --- the merge-observation surface sync's squash fallback reads ---

func TestHeadBranch(t *testing.T) {
	dir, g := newRepo(t)

	branch, err := g.HeadBranch()
	require.NoError(t, err)
	assert.Equal(t, "main", branch, "HEAD on a branch reports the branch name")

	// Detached HEAD is the absence of a branch name, not an error: sync needs to
	// tell that posture apart so it can refuse rather than guess an integration
	// branch. Branch() would answer the literal "HEAD" here.
	run(t, dir, "checkout", "--detach", "HEAD")
	branch, err = g.HeadBranch()
	require.NoError(t, err)
	assert.Empty(t, branch, "a detached HEAD has no branch name")
}

func TestHeadBranch_Unborn(t *testing.T) {
	// A repo with no commits still points HEAD at a branch that does not exist
	// yet. symbolic-ref answers it; rev-parse --abbrev-ref (what Branch uses)
	// exits 128 here, which is why HeadBranch is a separate method.
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Tiller Test")
	run(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")

	branch, err := New(dir).HeadBranch()
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestHeadBranch_NotARepo(t *testing.T) {
	_, err := New(t.TempDir()).HeadBranch()
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

func TestRefExists(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "branch", "feature")

	exists, err := g.RefExists("feature")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = g.RefExists("tiller/never-created")
	require.NoError(t, err)
	assert.False(t, exists, "a ref that does not resolve is absent, not an error")
}

func TestMergeBase(t *testing.T) {
	dir, g := newRepo(t)
	base := run(t, dir, "rev-parse", "HEAD")

	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "feat.txt", "feature work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "feature commit")
	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "main.txt", "main work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "main commit")

	got, err := g.MergeBase("feature", "main")
	require.NoError(t, err)
	assert.Equal(t, base, got, "the branch point is the common ancestor")
}

func TestMergeBase_NoCommonAncestor(t *testing.T) {
	// Unrelated histories exit 1 with no output. That is the absence of a base,
	// which the content fallback reads as "nothing to compare", not a failure.
	dir, g := newRepo(t)
	run(t, dir, "checkout", "--orphan", "unrelated")
	require.NoError(t, writeRepoFile(dir, "other.txt", "other\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "unrelated root")

	got, err := g.MergeBase("unrelated", "main")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMergeBase_MissingRef(t *testing.T) {
	// A ref that does not resolve exits 128, which merge-base cannot distinguish
	// from a broken repo on its own. The ref probe settles it: absent means no
	// base, not an environment error.
	_, g := newRepo(t)
	got, err := g.MergeBase("tiller/never-created", "main")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMergeBase_NotARepo(t *testing.T) {
	_, err := New(t.TempDir()).MergeBase("a", "b")
	require.Error(t, err)
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

// squashRepo builds the topology GitHub's default merge button leaves behind: a
// two-commit feature branch whose net change lands on main as one squash commit,
// with main also carrying an unrelated commit of its own. Ancestry is
// permanently false afterwards. It returns the repo, its *Git, the merge base,
// and the squash commit's full sha.
func squashRepo(t *testing.T) (string, *Git, string, string) {
	t.Helper()
	dir, g := newRepo(t)
	baseCommit := run(t, dir, "rev-parse", "HEAD")

	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "feat.txt", "feature work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "feature one")
	require.NoError(t, writeRepoFile(dir, "app/main.go", "package main\n\nfunc main() {}\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "feature two")

	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "unrelated.txt", "elsewhere\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "unrelated main work")
	run(t, dir, "merge", "--squash", "feature")
	run(t, dir, "commit", "-m", "squash merge feature (#12)")

	return dir, g, baseCommit, run(t, dir, "rev-parse", "HEAD")
}

func TestPatchID_MatchesASquashMerge(t *testing.T) {
	_, g, base, squash := squashRepo(t)

	merged, err := g.IsMergedInto("feature", "main")
	require.NoError(t, err)
	require.False(t, merged, "a squash merge leaves ancestry permanently false")

	want, err := g.PatchID(base, "feature")
	require.NoError(t, err)
	require.NotEmpty(t, want, "the branch's net change against the merge base has a patch-id")

	pairs, err := g.FirstParentPatchIDs("main", 50)
	require.NoError(t, err)
	byPatch := map[string]string{}
	for _, p := range pairs {
		byPatch[p.PatchID] = p.Commit
	}
	assert.Equal(t, squash, byPatch[want],
		"the squash commit carries the branch's whole net change under the same patch-id")

	// The unrelated main commit is present too, under its own id, and does not
	// collide with the branch's.
	assert.Len(t, pairs, 3, "init, the unrelated commit, and the squash")
}

func TestPatchID_EmptyDiffHasNoID(t *testing.T) {
	// A branch whose tip equals its merge base carries no net change. patch-id
	// emits nothing, and the caller must never read that as a match.
	dir, g := newRepo(t)
	run(t, dir, "branch", "feature")
	head := run(t, dir, "rev-parse", "HEAD")

	id, err := g.PatchID(head, "feature")
	require.NoError(t, err)
	assert.Empty(t, id)
}

func TestFirstParentPatchIDs_OrderAndLimit(t *testing.T) {
	dir, g := newRepo(t)
	for i := 1; i <= 3; i++ {
		require.NoError(t, writeRepoFile(dir, "log.txt", strings.Repeat("x", i)+"\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "step")
	}
	head := run(t, dir, "rev-parse", "HEAD")

	all, err := g.FirstParentPatchIDs("main", 50)
	require.NoError(t, err)
	require.Len(t, all, 4, "three steps plus the initial commit")
	assert.Equal(t, head, all[0].Commit, "the walk is newest first")

	limited, err := g.FirstParentPatchIDs("main", 2)
	require.NoError(t, err)
	require.Len(t, limited, 2, "the bound is honoured")
	assert.Equal(t, all[0], limited[0])
	assert.Equal(t, all[1], limited[1])

	none, err := g.FirstParentPatchIDs("main", 0)
	require.NoError(t, err)
	assert.Empty(t, none, "a zero bound scans nothing")
}

func TestFirstParentPatchIDs_SkipsTheSecondParentSide(t *testing.T) {
	// Only the integration line's own commits are candidates. Work carried in on
	// a merge's second-parent side is not evidence that some other branch landed.
	dir, g := newRepo(t)
	run(t, dir, "checkout", "-b", "side")
	require.NoError(t, writeRepoFile(dir, "side.txt", "side\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "side commit")
	sideCommit := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "main")
	run(t, dir, "merge", "--no-ff", "-m", "merge side", "side")

	pairs, err := g.FirstParentPatchIDs("main", 50)
	require.NoError(t, err)
	for _, p := range pairs {
		assert.NotEqual(t, sideCommit, p.Commit, "the second-parent commit is not on the first-parent line")
	}
}

func TestFirstParentPatchIDs_RenameMatchesOnlyWithNoRenames(t *testing.T) {
	// The two sides of the comparison must agree about renames. git log is
	// porcelain and honours diff.renames; git diff-tree is plumbing and does not.
	// With --no-renames pinned on both sides a renamed file's squash still
	// matches, which is what this asserts; without it the ids would differ.
	dir, g, base, squash := renameSquashRepo(t)
	run(t, dir, "config", "diff.renames", "true")

	want, err := g.PatchID(base, "feature")
	require.NoError(t, err)
	require.NotEmpty(t, want)

	pairs, err := g.FirstParentPatchIDs("main", 50)
	require.NoError(t, err)
	var found string
	for _, p := range pairs {
		if p.PatchID == want {
			found = p.Commit
		}
	}
	assert.Equal(t, squash, found, "a renamed file still matches because both sides pass --no-renames")
}

// renameSquashRepo squashes a branch whose only change is a file rename, the
// case where a rename-detecting diff and a plumbing diff disagree.
func renameSquashRepo(t *testing.T) (string, *Git, string, string) {
	t.Helper()
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "old.txt", strings.Repeat("content line\n", 20)))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "add old.txt")
	base := run(t, dir, "rev-parse", "HEAD")

	run(t, dir, "checkout", "-b", "feature")
	run(t, dir, "mv", "old.txt", "new.txt")
	run(t, dir, "commit", "-m", "rename")

	run(t, dir, "checkout", "main")
	run(t, dir, "merge", "--squash", "feature")
	run(t, dir, "commit", "-m", "squash the rename")

	return dir, g, base, run(t, dir, "rev-parse", "HEAD")
}

// --- the merge witness ---

// TestFirstParentContains_TellsAMergeCommitFromAFastForward pins the observation
// the sync layer leans on. A --no-ff merge puts the branch tip on the second-parent
// side, off main's trunk, which is a merge that happened. A fast-forward puts it on
// the trunk, where it is indistinguishable from a branch cut off main that never
// committed anything, which is no merge at all.
func TestFirstParentContains_TellsAMergeCommitFromAFastForward(t *testing.T) {
	// A merge commit: the branch tip is contained, and off the trunk.
	dir, g := newRepo(t)
	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "feat.txt", "feature work\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "feature work")
	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "other.txt", "main moves on\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "unrelated main work")
	run(t, dir, "merge", "--no-ff", "-m", "merge feature", "feature")

	merged, err := g.IsMergedInto("feature", "main")
	require.NoError(t, err)
	require.True(t, merged)
	onTrunk, err := g.FirstParentContains("main", "feature")
	require.NoError(t, err)
	assert.False(t, onTrunk, "a merge commit puts the branch tip off main's first-parent line")

	// Main advancing afterwards does not change the answer.
	require.NoError(t, writeRepoFile(dir, "later.txt", "later\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "later main work")
	onTrunk, err = g.FirstParentContains("main", "feature")
	require.NoError(t, err)
	assert.False(t, onTrunk, "the witness survives main moving on")

	// A fast-forward, and a branch that never diverged, answer alike: on the trunk.
	ffDir, ff := newRepo(t)
	run(t, ffDir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(ffDir, "feat.txt", "feature work\n"))
	run(t, ffDir, "add", "-A")
	run(t, ffDir, "commit", "-m", "feature work")
	run(t, ffDir, "checkout", "main")
	run(t, ffDir, "merge", "--ff-only", "feature")
	onTrunk, err = ff.FirstParentContains("main", "feature")
	require.NoError(t, err)
	assert.True(t, onTrunk, "a fast-forward leaves the branch tip on main's own line")

	emptyDir, empty := newRepo(t)
	run(t, emptyDir, "branch", "feature")
	onTrunk, err = empty.FirstParentContains("main", "feature")
	require.NoError(t, err)
	assert.True(t, onTrunk, "and so does a branch that was created and never committed to")
}

// TestFirstParentContains_UnresolvableRefIsNotAnError keeps a missing ref an
// absent observation rather than an environment fault, matching the rest of the
// merge-observation surface.
func TestFirstParentContains_UnresolvableRefIsNotAnError(t *testing.T) {
	_, g := newRepo(t)
	onTrunk, err := g.FirstParentContains("main", "no/such/branch")
	require.NoError(t, err)
	assert.False(t, onTrunk)
}

// --- the whitespace-faithful confirmation ---

// TestPatchDigest_SeparatesWhatPatchIDCollapses is the constraint that keeps the
// content fallback from fabricating. patch-id strips whitespace before hashing, so
// nesting a YAML key under its parent and outdenting the same key to the top level
// reduce to one id even though they build two different files. The digest, which
// keeps whitespace, tells them apart.
func TestPatchDigest_SeparatesWhatPatchIDCollapses(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "config.yml", "root:\n  a: 1\n  b: 2\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "add config")
	base := run(t, dir, "rev-parse", "HEAD")

	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "config.yml", "root:\n  a: 1\n    b: 2\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "nest b under a")

	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "config.yml", "root:\n  a: 1\nb: 2\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "outdent b")

	branchID, err := g.PatchID(base, "feature")
	require.NoError(t, err)
	mainID, err := g.PatchID(base, "main")
	require.NoError(t, err)
	require.Equal(t, branchID, mainID, "patch-id throws the whitespace away and collides")

	branchText, err := g.PatchDigest(base, "feature")
	require.NoError(t, err)
	mainText, err := g.PatchDigest(base, "main")
	require.NoError(t, err)
	assert.NotEmpty(t, branchText)
	assert.NotEqual(t, branchText, mainText, "the patch text does not")
}

// TestPatchDigest_SurvivesTheSameChangeLandingElsewhere is the other half: the
// digest must still match a genuine squash, which lands the same change at a
// different offset in a file the integration branch also touched. Hunk ranges and
// blob hashes both move; neither is content, so neither is hashed.
func TestPatchDigest_SurvivesTheSameChangeLandingElsewhere(t *testing.T) {
	dir, g := newRepo(t)
	body := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
	require.NoError(t, writeRepoFile(dir, "file.txt", body))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "add file")
	base := run(t, dir, "rev-parse", "HEAD")

	// The branch appends a line at the end.
	run(t, dir, "checkout", "-b", "feature")
	require.NoError(t, writeRepoFile(dir, "file.txt", body+"BRANCH\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "branch appends")

	// Main independently prepends a line far from the branch's hunk, then takes
	// the branch's change as a squash.
	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "file.txt", "TOP\n"+body))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "main prepends")
	require.NoError(t, writeRepoFile(dir, "file.txt", "TOP\n"+body+"BRANCH\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "squash the branch")

	branchText, err := g.PatchDigest(base, "feature")
	require.NoError(t, err)
	squashText, err := g.PatchDigest("main^1", "main")
	require.NoError(t, err)
	assert.Equal(t, branchText, squashText,
		"the same change, landed at a different offset over a different base, is the same patch")
}

// TestPatchDigest_EmptyAndUnresolvableAnswerEmpty keeps the two absences the
// caller reads as "cannot confirm" out of the error path.
func TestPatchDigest_EmptyAndUnresolvableAnswerEmpty(t *testing.T) {
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")

	got, err := g.PatchDigest(head, head)
	require.NoError(t, err)
	assert.Empty(t, got, "a commit against itself is no patch at all")

	got, err = g.PatchDigest("no/such/ref", head)
	require.NoError(t, err)
	assert.Empty(t, got, "an unresolvable ref is an absent observation, not a fault")
}

// TestAddedSince reports only the files a range added under one path, as
// forward-slash paths relative to Dir: the read `tiller readout <seam>
// --since <sha>` makes to find the event files a correction committed.
func TestAddedSince(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, ".tiller/events/2026/06/27/EV-aaaaaa.yml", "id: EV-aaaaaa\n"))
	require.NoError(t, writeRepoFile(dir, "README.md", "hello again\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "first event")
	since := run(t, dir, "rev-parse", "HEAD")

	require.NoError(t, writeRepoFile(dir, ".tiller/events/2026/06/28/EV-cccccc.yml", "id: EV-cccccc\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/events/2026/06/27/EV-bbbbbb.yml", "id: EV-bbbbbb\n"))
	require.NoError(t, writeRepoFile(dir, ".tiller/events/2026/06/27/EV-aaaaaa.yml", "id: EV-aaaaaa\nedited: true\n"))
	require.NoError(t, writeRepoFile(dir, "app/other.go", "package main\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "correction")

	got, err := g.AddedSince(since, ".tiller/events")
	require.NoError(t, err)
	assert.Equal(t, []string{
		".tiller/events/2026/06/27/EV-bbbbbb.yml",
		".tiller/events/2026/06/28/EV-cccccc.yml",
	}, got, "only additions under the path, never a modified file or one outside it")
}

func TestAddedSince_NothingAddedIsEmptyNotError(t *testing.T) {
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	got, err := g.AddedSince(head, ".tiller/events")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAddedSince_UnresolvableCommitIsGitUnavailable(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.AddedSince("nonsense", ".tiller/events")
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonGitUnavailable, ee.Reason)
}

func TestRefCommitResolvesBranchTipsAndAbsentRefs(t *testing.T) {
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	got, err := g.RefCommit("refs/heads/main")
	require.NoError(t, err)
	require.Equal(t, head, got)
	got, err = g.RefCommit("refs/heads/missing")
	require.NoError(t, err)
	require.Empty(t, got)
}
