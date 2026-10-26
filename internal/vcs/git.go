// Package vcs is Tiller's read-only window onto git. It is the only package
// that shells to the git binary, and it never writes: it observes the current
// branch, HEAD, blob and tree fingerprints, branch ancestry, first-parent
// topology, merge bases, and patch content, so the gate and sync layers can pin
// work to revisions and notice a merge (a local --no-ff merge commit, or a squash
// whose ancestry is permanently false) without Tiller ever creating a branch,
// committing, pushing, or merging.
//
// Read-only is a hard stance, not an aspiration. Every command this package runs
// (rev-parse, symbolic-ref, status, hash-object without -w, ls-tree, ls-files,
// remote, merge-base, rev-list, diff-tree, log, patch-id) computes and reports.
// None of them writes an object, a ref, an index, or a stash. In particular the
// squash observation deliberately does not synthesize a commit with commit-tree to
// compare against: it compares patch-ids and patch text, both of which are
// computed and discarded.
//
// Layering: this package imports internal/store for the RevisionRef and
// EnvError primitives it returns, and nothing from internal/core or
// internal/cli depends on it the other way around.
package vcs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
)

// ReasonGitUnavailable is the store.EnvError discriminator for a git that is
// missing, not on PATH, or erroring (including a directory that is not a git
// repository). It maps to exit 3, the environment class.
const ReasonGitUnavailable = "git_unavailable"

// Revisions is the read-only git observation surface the gate and sync layers
// depend on. They take a Revisions rather than a *Git so their tests can stub
// git entirely; *Git is the one concrete implementation that shells out.
type Revisions interface {
	// Branch reports the current branch name (or "HEAD" when detached).
	Branch() (string, error)
	// Head reports HEAD's short commit sha.
	Head() (string, error)
	// BlobHash reports the git blob hash of a path's committed content at HEAD.
	// Every committed-tree read (BlobHash, TreeHash, and their commit-pinned
	// forms) takes project-relative paths, resolved from the Tiller project
	// directory rather than the repository root.
	BlobHash(path string) (string, error)
	// WorktreeBlobHash reports the blob id `git add` would store for a path's
	// current working-tree content (checkin conversion and clean filters
	// applied, nothing written), so an uncommitted edit is detectable
	// immediately and an unedited file hashes to its committed blob. It is the
	// correct basis for "current feature.md" drift, before any commit.
	WorktreeBlobHash(path string) (string, error)
	// WorktreeBlobMatches reports whether a path's working-tree content is
	// raw-identical to the blob hash, or checks in as hash. The raw arm needs
	// no git process and also accepts pins older binaries recorded raw.
	WorktreeBlobMatches(path, hash string) (bool, error)
	// TreeHash reports the git tree hash of a committed subtree at HEAD; an
	// empty path is the project directory's tree.
	TreeHash(path string) (string, error)
	// CurrentCommit returns HEAD as a git_commit RevisionRef.
	CurrentCommit() (store.RevisionRef, error)
	// IsDirty reports whether the working tree has any uncommitted change
	// (tracked edits, staged changes, or untracked non-ignored files), i.e.
	// `git status --porcelain` is non-empty. It is how the gate layer tells
	// whether HEAD actually contains the code a gate ran over.
	IsDirty() (bool, error)
	// WorktreeFingerprint returns a deterministic sha256 RevisionRef over the
	// current working-tree state, the honest provenance for a gate run on a dirty
	// tree (where HEAD does not contain the tested code).
	WorktreeFingerprint() (store.RevisionRef, error)
	// CodeIdentity returns "sha256:<64 hex>", a content digest of the project's
	// code that is the same whether the files are committed or dirty. It leaves
	// out Tiller's own writes, the project's `.tiller/` state tree and its
	// `artifacts/tiller/` gate evidence, so recording a gate result never changes
	// the identity it was recorded under. Gate reuse compares it.
	CodeIdentity() (string, error)
	// RefCommit resolves ref to its full commit object ID, or "" when the ref
	// does not exist. It is how sync pins its integration branch to one commit.
	RefCommit(ref string) (string, error)
	// CommitRef returns commit as a git_commit RevisionRef, the commit-pinned
	// counterpart of CurrentCommit.
	CommitRef(commit string) (store.RevisionRef, error)
	// BlobHashAt and TreeHashAt are BlobHash and TreeHash read from commit
	// instead of HEAD, with the same path resolution.
	BlobHashAt(commit, path string) (string, error)
	TreeHashAt(commit, path string) (string, error)
	// TreeInventoryAt lists the file paths under a subtree of commit, sorted,
	// so a recorded surface fingerprint carries the inventory of files the
	// tree covered at that commit.
	TreeInventoryAt(commit, path string) ([]string, error)
	// IsMergedInto reports whether branch's tip is contained in into, i.e.
	// whether branch is an ancestor of into. This is how a local merge is
	// observed without orchestrating one.
	IsMergedInto(branch, into string) (bool, error)
	// FirstParentContains reports whether commit lies on ref's own
	// first-parent line, i.e. whether ref reaches it without ever stepping
	// onto a merge's second-parent side.
	FirstParentContains(ref, commit string) (bool, error)
	// HeadBranch reports the branch HEAD points at, or "" when HEAD is
	// detached. It is distinct from Branch, which reports the literal string
	// "HEAD" in that case and so cannot be compared against a branch name.
	HeadBranch() (string, error)
	// RefExists reports whether ref resolves to an object in this repository.
	RefExists(ref string) (bool, error)
	// MergeBase reports the best common ancestor of a and b, or "" when they
	// have none (unrelated histories) or either ref does not resolve.
	MergeBase(a, b string) (string, error)
	// PatchID reports the stable patch-id of the tree-to-tree diff from -> to,
	// or "" when that diff is empty.
	PatchID(from, to string) (string, error)
	// PatchDigest reports a whitespace-faithful digest of the same diff, or ""
	// when the diff is empty or either ref does not resolve. It is what
	// confirms a patch-id match, which by design ignores whitespace.
	PatchDigest(from, to string) (string, error)
	// FirstParentPatchIDs returns the stable patch-ids of the newest limit
	// first-parent commits of ref, newest first.
	FirstParentPatchIDs(ref string, limit int) ([]CommitPatch, error)
}

// CommitPatch pairs one commit with the stable patch-id of its own diff. It is
// what FirstParentPatchIDs reports, so a caller that matches a patch-id can name
// the commit that carried it.
type CommitPatch struct {
	Commit  string
	PatchID string
}

// Clock yields the current time. It is the single seam CurrentCommit reads wall
// time through, so a test can inject a fixed instant and the ObservedAt stamp
// becomes deterministic. A nil Clock falls back to time.Now in UTC.
type Clock interface {
	Now() time.Time
}

// Git is the concrete, git-shelling Revisions implementation. It runs every git
// command in Dir (any path inside the repository).
type Git struct {
	// Dir is the working directory git commands run in; any path within the
	// repository works.
	Dir string
	// Clock is the injected time source for CurrentCommit's ObservedAt. When
	// nil, the wall clock (time.Now in UTC) is used.
	Clock Clock
}

// New returns a *Git rooted at dir with the default wall clock.
func New(dir string) *Git { return &Git{Dir: dir} }

// compile-time assertion that *Git satisfies Revisions.
var _ Revisions = (*Git)(nil)

// NotVersionedError is a typed not-found: git could not resolve the path for the
// read that was attempted. It is distinct from a git_unavailable environment
// error, which means git itself could not be run.
//
// The default message reports the failed read and stops there rather than
// claiming the path is not under version control, because that claim is about
// git's index and only the HEAD readers (BlobHash, TreeHash) support it.
// WorktreeBlobHash hashes the file as it sits on disk, so a file git tracks at
// HEAD and that is merely deleted or renamed in the working tree fails that read
// while `git ls-files` still lists it, and the old wording told the reader the
// opposite of the truth. A caller with a more precise fact to state sets Msg,
// which is how objectHashAt reports a path that resolves to the wrong object kind.
type NotVersionedError struct {
	Path string
	Msg  string
}

func (e *NotVersionedError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("vcs: git could not resolve path %q", e.Path)
}

// gitError wraps a git invocation that ran but exited non-zero. It is an
// internal classification step: callers turn it into either a meaningful
// boolean (merge-base --is-ancestor's exit 1), a NotVersionedError, or a
// git_unavailable EnvError.
type gitError struct {
	args     []string
	exitCode int
	stderr   string
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s",
		strings.Join(e.args, " "), e.exitCode, strings.TrimSpace(e.stderr))
}

func (g *Git) now() time.Time {
	if g.Clock != nil {
		return g.Clock.Now().UTC()
	}
	return time.Now().UTC()
}

// run executes git with args in g.Dir. A binary that cannot be launched (missing
// or not on PATH) is a git_unavailable EnvError; a non-zero exit is returned as
// a *gitError for the caller to classify; success returns trimmed stdout.
func (g *Git) run(args ...string) (string, error) {
	out, err := g.runRaw(args...)
	if err != nil {
		return out, err
	}
	return strings.TrimSpace(out), nil
}

// runRaw is run without whitespace trimming, for patches, NUL-separated paths,
// and historical file bytes.
func (g *Git) runRaw(args ...string) (string, error) {
	var stdout bytes.Buffer
	err := g.runOutput(&stdout, args...)
	return stdout.String(), err
}

// runOutput shares process and error handling with bounded historical reads.
func (g *Git) runOutput(stdout io.Writer, args ...string) error {
	return g.runCmd(nil, stdout, args...)
}

// runCmd is runOutput with an optional stdin.
func (g *Git) runCmd(stdin io.Reader, stdout io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.Dir
	cmd.Stdin = stdin
	// Diagnostics use a stable locale. Every observation reads the actual local
	// objects: replacement refs cannot rewrite evidence, and missing promisor
	// objects must not trigger fetching or writes as a side effect of a read.
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &gitError{
				args:     args,
				exitCode: exitErr.ExitCode(),
				stderr:   stderr.String(),
			}
		}
		// The binary could not be launched at all.
		return g.unavailable(err.Error())
	}
	return nil
}

func (g *Git) unavailable(msg string) error {
	return &store.EnvError{
		Code:   3,
		Reason: ReasonGitUnavailable,
		Msg:    "git unavailable: " + msg,
	}
}

// asEnv maps any error from run into a git_unavailable EnvError. It is used by
// the helpers whose only failure mode is the environment (no meaningful
// per-path or boolean answer to recover).
func (g *Git) asEnv(err error) error {
	var ee *store.EnvError
	if errors.As(err, &ee) {
		return ee
	}
	return g.unavailable(err.Error())
}

// ensureRepo confirms Dir is inside a git repository, so a path-shaped failure
// can be told apart from "git could not run here at all."
func (g *Git) ensureRepo() error {
	if _, err := g.run("rev-parse", "--git-dir"); err != nil {
		return g.asEnv(err)
	}
	return nil
}

// classifyPathErr decides whether a failed path lookup is a git_unavailable
// environment error or a NotVersionedError. A genuine git failure (binary or
// repo) wins; otherwise the path simply is not in the committed tree.
func (g *Git) classifyPathErr(err error, path string) error {
	var ee *store.EnvError
	if errors.As(err, &ee) {
		return ee
	}
	if repoErr := g.ensureRepo(); repoErr != nil {
		return repoErr
	}
	return &NotVersionedError{Path: path}
}

// Branch reports the current branch name, or "HEAD" when the repo is in a
// detached-HEAD state.
func (g *Git) Branch() (string, error) {
	out, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", g.asEnv(err)
	}
	return out, nil
}

// Head reports HEAD's short commit sha.
func (g *Git) Head() (string, error) {
	out, err := g.run("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", g.asEnv(err)
	}
	return out, nil
}

// CurrentCommit returns HEAD as a git_commit RevisionRef in the compact
// "git:<short-sha>" value form. A bare commit carries no path (artifact-schema.md,
// "Revisions").
func (g *Git) CurrentCommit() (store.RevisionRef, error) {
	head, err := g.Head()
	if err != nil {
		return store.RevisionRef{}, err
	}
	return store.RevisionRef{
		Kind:       store.KindGitCommit,
		Value:      "git:" + head,
		ObservedAt: g.now(),
	}, nil
}

// IsDirty reports whether the working tree carries any uncommitted change. It
// shells `git status --porcelain`, whose output is empty exactly when the tree
// is clean (no tracked edits, no staged changes, no untracked non-ignored
// files). It is read-only.
func (g *Git) IsDirty() (bool, error) {
	// A read must not refresh cached stat data in the caller's index.
	out, err := g.run("--no-optional-locks", "status", "--porcelain")
	if err != nil {
		return false, g.asEnv(err)
	}
	return strings.TrimSpace(out) != "", nil
}

// DirtyUnder lists repo-relative paths under prefix that are modified or
// untracked per `git status --porcelain -- <prefix>`. Empty means clean.
//
// It is IsDirty narrowed to one subtree: the uncommitted-state line (issue
// #44) reports what tiller itself wrote under .tiller/ and left uncommitted
// (a feature.yml, an event file), and IsDirty's whole-tree boolean cannot
// name which paths, nor scope out a dirty file elsewhere in the working
// tree that has nothing to do with tiller's own state. The -- separator
// scopes git's own comparison, so a file outside prefix never appears
// regardless of what else is dirty.
//
// --untracked-files=all is load-bearing, not decoration. Its default,
// "normal", reports an untracked directory as one line naming the directory
// rather than descending into it, so a first-ever write into a directory
// that was never committed (.tiller/events/, empty at scaffold time and
// never itself tracked) would collapse every file inside it into a single
// entry. The field case this closes wrote a feature.yml plus two separate
// event files; the reader needs those counted as three, not folded into
// one directory line. The recursion is scoped by -- prefix, so it costs
// only what is under .tiller/, never the whole repository.
//
// -z is equally load-bearing, not a formatting preference. The plain
// porcelain format quotes any path with an "unusual" byte and, under the
// default core.quotepath=true, C-style octal-escapes every non-ASCII byte in
// it (a literal `\303\251` for an e-acute), so a naive strip-the-quotes
// parse hands back the escaped literal rather than the real path, which is
// neither the file that exists on disk nor a path a caller can act on. -z
// bypasses that quoting entirely: each entry is NUL-terminated and its path
// bytes are written verbatim, regardless of core.quotepath. parsePorcelainZ
// is where the resulting NUL-delimited stream is decoded.
//
// Not part of the Revisions interface, the same posture HasRemote and
// OriginURL already take (git.go's own precedent): the gate and sync layers
// never ask a scoped question like this, only the status summary does, and
// it already holds a concrete *Git.
func (g *Git) DirtyUnder(prefix string) ([]string, error) {
	// runRaw, not run: run trims the WHOLE output, which under -z could eat a
	// trailing empty path field (a delete recorded with nothing after its
	// final NUL). PatchDigest keeps the identical posture for the same
	// reason: a raw byte stream is not text to be whitespace-trimmed.
	// A read must not refresh cached stat data in the caller's index.
	out, err := g.runRaw("--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all", "--", prefix)
	if err != nil {
		return nil, g.asEnv(err)
	}
	paths := parsePorcelainZ(out)
	sort.Strings(paths)
	return paths, nil
}

// parsePorcelainZ parses the NUL-terminated entry stream `git status
// --porcelain -z` writes into the current path of each entry, verbatim
// (see DirtyUnder's own comment on why -z, not the plain quoted format).
//
// Each ordinary entry is one field, "XY path". A rename or copy (X or Y is
// 'R' or 'C') is followed by a SECOND field carrying the old path alone,
// with no XY prefix; that field is consumed here and discarded, both
// because DirtyUnder's contract is the CURRENT paths under prefix and
// because the old path is not even guaranteed to sit under prefix at all (a
// file renamed INTO the scoped subtree from outside it), so emitting it
// would leak a result the -- pathspec was supposed to exclude.
func parsePorcelainZ(out string) []string {
	fields := strings.Split(out, "\x00")
	var paths []string
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 3 {
			// The empty trailing field after the last NUL, or a malformed
			// entry too short to carry an XY code and a path.
			continue
		}
		status, path := entry[:2], entry[3:]
		paths = append(paths, path)
		if strings.ContainsAny(status, "RC") {
			// The next field is the old path; skip it without emitting.
			i++
		}
	}
	return paths
}

// AddedSince reports the files that commits after `commit` up to HEAD added
// under path, as forward-slash paths relative to Dir, in git's own sorted
// order. It shells `git diff --name-only --diff-filter=A <commit> HEAD -- <path>`.
// It is the read the correction readout (`tiller readout <seam> --since
// <sha>`) makes to find exactly the event files a correction committed: the
// event stream is append-only, one immutable file per event, so an added file
// under .tiller/events is one recorded mutation and nothing else changes there.
// --relative keeps the answer relative to Dir rather than the repository root,
// the same shape the caller joins onto its own tree root. A commit that does
// not resolve, or any other git failure, is a git_unavailable EnvError: the
// caller validates the commit ahead of this read, so a failure here is the
// environment, not the argument.
func (g *Git) AddedSince(commit, path string) ([]string, error) {
	out, err := g.run("diff", "--name-only", "--relative", "--diff-filter=A", commit, "HEAD", "--", path)
	if err != nil {
		return nil, g.asEnv(err)
	}
	return nonEmptyLines(out), nil
}

// nonEmptyLines splits trimmed git output into its non-empty lines.
func nonEmptyLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			lines = append(lines, s)
		}
	}
	return lines
}

// BlobHash reports the git blob hash of path's committed content at HEAD. path
// is project-relative, as for BlobHashAt. A path that is absent or is not a blob
// (e.g. a directory) yields a NotVersionedError.
func (g *Git) BlobHash(path string) (string, error) {
	return g.BlobHashAt("HEAD", path)
}

// WorktreeBlobHash reports the blob id `git add` would store for path's current
// working-tree content. Unlike BlobHash, which reads the committed content at
// HEAD, this hashes the file as it sits on disk, so an uncommitted edit changes
// the hash immediately. path is project-relative and resolves against g.Dir; an
// unsafe, non-regular, or unreadable path yields a NotVersionedError.
//
// The content goes through the repository's checkin conversion for that path
// (eol normalization under text=auto, eol=crlf, or core.autocrlf, and any
// clean filter such as Git LFS) via `git hash-object --stdin --path`, without
// -w, so nothing is written to the object database. That puts the hash in the
// same space as committed blobs: an unchanged CRLF checkout of an LF blob
// hashes to that blob.
func (g *Git) WorktreeBlobHash(path string) (string, error) {
	data, err := g.readWorktreeFile(path)
	if err != nil {
		return "", err
	}
	return g.checkinBlobID(path, data)
}

// WorktreeBlobMatches reports whether path's current working-tree content is
// raw-identical to the blob hash, or checks in as hash. It reads the file once
// and first compares its raw blob id in process, so the common unedited file
// costs no git process (a routing pass compares every approved basis, issue
// #87's process-per-path cost), and a pin an older binary recorded as the raw
// blob id keeps matching its unedited file. Only a raw mismatch pays for the
// conversion-aware hash. A missing or unsafe path yields a NotVersionedError.
func (g *Git) WorktreeBlobMatches(path, hash string) (bool, error) {
	data, err := g.readWorktreeFile(path)
	if err != nil {
		return false, err
	}
	if blobID(data) == hash {
		return true, nil
	}
	converted, err := g.checkinBlobID(path, data)
	if err != nil {
		return false, err
	}
	return converted == hash, nil
}

// readWorktreeFile reads a regular file beneath g.Dir, refusing symlink
// components the same way every working-tree observation does.
func (g *Git) readWorktreeFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(g.Dir)
	if err != nil {
		return nil, &NotVersionedError{Path: path}
	}
	defer root.Close()
	file, err := (rootpath.Walker{}).Open(root, filepath.ToSlash(path), false)
	if err != nil {
		return nil, &NotVersionedError{Path: path}
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, &NotVersionedError{Path: path}
	}
	return data, nil
}

// checkinBlobID hashes data as if it were checked in at path, applying the
// attributes and filters git would for that path. It hashes the bytes already
// read rather than letting git reopen the path, so the safety checks above
// govern exactly what is hashed.
func (g *Git) checkinBlobID(path string, data []byte) (string, error) {
	var stdout bytes.Buffer
	if err := g.runCmd(bytes.NewReader(data), &stdout, "hash-object", "--stdin", "--path="+filepath.ToSlash(path)); err != nil {
		return "", g.asEnv(err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// TreeHash reports the git tree hash of a committed subtree at HEAD. path is
// project-relative, as for TreeHashAt; an empty path (or ".") is the project
// directory's tree. A path that is absent or is not a tree (e.g. a file) yields
// a NotVersionedError.
func (g *Git) TreeHash(path string) (string, error) {
	return g.TreeHashAt("HEAD", path)
}

// HasRemote reports whether the repository has any git remote configured. It
// shells `git remote`, a purely local read of the config (no network), and any
// listed remote name means true. It is not part of the Revisions interface: the
// gate and sync layers never ask it; only the CLI's submit command does, to
// decide whether a PR is the expected review surface (cli-surface.md, "The work
// loop") or the branch-ref fallback applies because there is nowhere to open one.
func (g *Git) HasRemote() (bool, error) {
	out, err := g.run("remote")
	if err != nil {
		return false, g.asEnv(err)
	}
	return len(nonEmptyLines(out)) > 0, nil
}

// OriginURL returns the origin remote's configured URL, or "" when the
// repository has no origin. It shells `git remote get-url origin`, a purely
// local config read (no network). A non-zero exit means simply "no origin"
// (a local-only repo, or a repo with remotes but none named origin), which is
// the absence of a review surface, not an environment fault, so it answers
// ("", nil): a page deriving a PR link over it omits the link rather than
// failing. Only a git that cannot be launched at all is surfaced as a
// git_unavailable EnvError, which the console still survives (it skips the
// link). It is not part of the Revisions interface; only the serve console's
// node page asks it, to map a recorded PR onto its GitHub page.
func (g *Git) OriginURL() (string, error) {
	out, err := g.run("remote", "get-url", "origin")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) {
			// A non-zero exit is the absence of an origin, not an error.
			return "", nil
		}
		// The git binary could not be launched at all.
		return "", g.asEnv(err)
	}
	return out, nil
}

// IsMergedInto reports whether branch's tip is contained in into, i.e. whether
// branch is an ancestor of into. It shells `git merge-base --is-ancestor`, whose
// documented contract is exactly this: exit 0 means yes, exit 1 means no. A
// branch ref that does not resolve (never created, or merged and then deleted)
// is "not merged" from sync's observation standpoint, not an environment fault,
// so it is reported as false; any other non-zero exit on a ref that does resolve
// is a real error surfaced as a git_unavailable EnvError.
func (g *Git) IsMergedInto(branch, into string) (bool, error) {
	_, err := g.run("merge-base", "--is-ancestor", branch, into)
	if err == nil {
		return true, nil
	}
	var ge *gitError
	if errors.As(err, &ge) {
		if ge.exitCode == 1 {
			return false, nil
		}
		// A non-1 exit can mean the branch ref simply does not exist. Probe it:
		// an unresolvable branch is observed as not merged, while a genuinely
		// broken repo (the probe also fails) keeps the environment error.
		if exists, perr := g.RefExists(branch); perr == nil && !exists {
			return false, nil
		}
	}
	return false, g.asEnv(err)
}

// FirstParentContains reports whether commit lies on ref's own first-parent
// line: the line ref reaches by always taking a merge's first parent, which is
// the integration branch's own trunk. It is the merge witness sync needs on top
// of ancestry, because containment alone does not say how the work arrived.
//
// It walks `git rev-list --first-parent --parents commit..ref`, which descends
// ref's trunk and stops at the first commit already reachable from commit. The
// last line of that walk is the lowest trunk commit ref still owns, so its FIRST
// parent is exactly where the trunk crossed into commit's history. When that
// parent is commit itself, commit sits on the trunk; when it is some earlier
// ancestor, commit arrived through a merge's second-parent side instead. The
// walk costs the distance from ref down to the join, not the whole history.
//
// It first proves commit is reachable from ref. An empty walk then means ref and
// commit name the same commit, trivially on its own trunk. Without that ancestry
// check, a ref reset behind commit also yields an empty walk. An absent ref has
// no trunk, which is an absent observation rather than an environment fault.
func (g *Git) FirstParentContains(ref, commit string) (bool, error) {
	want, err := g.run("rev-parse", "--verify", "--quiet", commit+"^{commit}")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && ge.exitCode == 1 {
			return false, nil
		}
		return false, g.asEnv(err)
	}
	want = strings.TrimSpace(want)
	if want == "" {
		return false, nil
	}
	contained, err := g.IsMergedInto(want, ref)
	if err != nil {
		if exists, refErr := g.RefExists(ref); refErr == nil && !exists {
			return false, nil
		}
		return false, err
	}
	if !contained {
		return false, nil
	}

	out, err := g.run("rev-list", "--first-parent", "--parents", commit+".."+ref)
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) {
			refOK, rerr := g.RefExists(ref)
			if rerr == nil && !refOK {
				return false, nil
			}
		}
		return false, g.asEnv(err)
	}
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		return true, nil
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 2 {
		// A root commit has no parent, so the trunk ran out before reaching
		// commit at all.
		return false, nil
	}
	return fields[1] == want, nil
}

// RefExists reports whether ref resolves to an object in this repository. It
// shells `git rev-parse --verify --quiet`, whose exit 1 means "does not resolve";
// any other failure is surfaced as a git_unavailable EnvError. sync reads it to
// tell "this branch was never merged" from "this branch was merged and then
// deleted", which look identical through ancestry alone.
func (g *Git) RefExists(ref string) (bool, error) {
	_, err := g.run("rev-parse", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}
	var ge *gitError
	if errors.As(err, &ge) && ge.exitCode == 1 {
		return false, nil
	}
	return false, g.asEnv(err)
}

// RefCommit resolves ref to its full commit object ID, or "" when the ref does
// not exist.
func (g *Git) RefCommit(ref string) (string, error) {
	out, err := g.run("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	var ge *gitError
	if errors.As(err, &ge) && ge.exitCode == 1 {
		return "", nil
	}
	return "", g.asEnv(err)
}

// LocalBranches lists local branch names without resolving tags or changing refs.
// Mutation guards use it to diagnose both directions of a ref-prefix collision.
func (g *Git) LocalBranches() ([]string, error) {
	out, err := g.run("for-each-ref", "--format=%(refname:strip=2)", "refs/heads/")
	if err != nil {
		return nil, g.asEnv(err)
	}
	return nonEmptyLines(out), nil
}

// CommitsBetween lists the commits tip holds beyond base, newest first, as
// "<short sha> <subject>" lines. It reads history only and never changes refs.
func (g *Git) CommitsBetween(base, tip string) ([]string, error) {
	out, err := g.run("log", "--format=%h %s", base+".."+tip, "--")
	if err != nil {
		return nil, g.asEnv(err)
	}
	return nonEmptyLines(out), nil
}

// BranchRef fully qualifies a bare local branch name as refs/heads/<name>
// before it reaches a git read that resolves a name by git's own precedence
// rules, so the read asks about the BRANCH and not about whatever else that
// name happens to designate.
//
// git's rev-parse precedence resolves a bare name from refs/tags before
// refs/heads (mutate/sync.go's own branchRef documents the identical hazard
// for observeMerge, the precedent this follows): a tag sharing a feature
// branch's name answers for the branch. UnpushedTip's merge-base arm and the
// review-state warning's ShortSHA call (internal/cli/unpushed_review.go)
// are the two places this package's own callers ask git about a task's
// recorded feature branch by name, so both go through this.
//
// Already-qualified input (anything already starting with refs/, whether
// refs/heads/, refs/remotes/, or refs/tags/) and the empty string pass
// through unchanged, so a caller need not know which form it already holds.
func BranchRef(branch string) string {
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return branch
	}
	return "refs/heads/" + branch
}

// UnpushedTip reports whether branch's local tip holds commits its
// remote-tracking counterpart does not: the local tip is not an ancestor of
// the upstream ref. The upstream is branch's configured @{upstream} when one
// exists, else origin/<branch> when that ref resolves. ok=false (with no
// error) when the branch has no remote counterpart at all, which is the
// offline path and never warns.
//
// It is not part of the Revisions interface, the same posture HasRemote and
// OriginURL already take: sync's and drift's git-observation callers never
// need this, only the surfaces that report a task's review state (the CLI's
// submit confirmation, execute readout, and status oracle, plus the
// console's node page, all four through internal/core/review's ForTask) do,
// all of which already hold a concrete *Git.
//
// merge-base --is-ancestor is the identical primitive IsMergedInto already
// shells (git.go's own IsMergedInto, the precedent this follows): exit 0
// means the local tip IS reachable from upstream (pushed, not unpushed), exit
// 1 means it is not (unpushed), and any other exit is a real git fault
// surfaced as a git_unavailable EnvError.
//
// The LOCAL side of that probe is qualified through BranchRef before it
// reaches git (final review finding 1, batch 42-45): git's rev-parse
// precedence resolves a bare name from refs/tags before refs/heads, so an
// unqualified branch here would let a same-named tag answer for it, exactly
// the hazard sync.go's own branchRef guards observeMerge against. upstream is
// left as resolveUpstream returned it: it is always already a remote-tracking
// name (an @{upstream} abbrev-ref or the origin/<branch> fallback), never a
// bare branch name, so it carries none of this ambiguity and qualifying it
// with refs/heads/ would name the wrong ref entirely (branchRef's own doc
// comment makes the identical call for into).
func (g *Git) UnpushedTip(branch string) (unpushed bool, upstream string, ok bool, err error) {
	upstream, ok, err = g.resolveUpstream(branch)
	if err != nil || !ok {
		return false, "", ok, err
	}

	_, mbErr := g.run("merge-base", "--is-ancestor", BranchRef(branch), upstream)
	if mbErr == nil {
		return false, upstream, true, nil
	}
	var ge *gitError
	if errors.As(mbErr, &ge) && ge.exitCode == 1 {
		return true, upstream, true, nil
	}
	return false, "", false, g.asEnv(mbErr)
}

// resolveUpstream finds branch's remote-tracking counterpart: its configured
// @{upstream} first (`git rev-parse --abbrev-ref <branch>@{upstream}`), else
// origin/<branch> when that ref resolves, the ordinary shape a branch pushed
// without -u still leaves behind. ok is false with no error when neither
// resolves, which is the offline path (no remote at all) as well as a branch
// name that was never actually pushed or checked out; UnpushedTip's own doc
// comment is where that silence is a deliberate contract, not a gap.
func (g *Git) resolveUpstream(branch string) (upstream string, ok bool, err error) {
	out, uerr := g.run("rev-parse", "--abbrev-ref", branch+"@{upstream}")
	if uerr == nil {
		return out, true, nil
	}
	var ge *gitError
	if !errors.As(uerr, &ge) {
		// The git binary could not be launched at all.
		return "", false, g.asEnv(uerr)
	}

	fallback := "origin/" + branch
	exists, rerr := g.RefExists(fallback)
	if rerr != nil {
		return "", false, rerr
	}
	if !exists {
		return "", false, nil
	}
	return fallback, true, nil
}

// ShortSHA reports ref's short commit sha, the same form Head reports for
// HEAD specifically. It is how a caller names the exact commit an arbitrary
// branch sits at, which UnpushedTip's own boolean answer does not carry: the
// review-state warning needs to name the commit on a task's
// recorded feature branch, and that branch is not necessarily the one this
// process has checked out (status, in particular, may run from anywhere in
// the working tree), so Head, which only ever reads HEAD, cannot answer it.
func (g *Git) ShortSHA(ref string) (string, error) {
	out, err := g.run("rev-parse", "--short", ref)
	if err != nil {
		return "", g.asEnv(err)
	}
	return out, nil
}

// WorktreeFor reports the worktree path that has branch checked out, if any.
// It parses `git worktree list --porcelain`, which names each worktree's
// branch ref; ok=false when no worktree holds the branch.
//
// This is the read the accept-fast-forward remediation needs
// before it can prescribe a fix: `git branch -f` refuses outright when the
// branch it would move is checked out anywhere, the primary worktree
// included, so a caller has to know whether that command would work before
// naming it. The primary worktree is not special-cased; git's own porcelain
// output lists it exactly like any other stanza, and this reads it the same
// way, which is why a branch checked out in the ordinary working directory
// is found here too, not only one parked in a `git worktree add` checkout.
//
// branch is matched by name against each stanza's own "branch
// refs/heads/<name>" line, accepting either a bare name or one already
// qualified with refs/heads/, so a caller need not know which form the rest
// of this package happens to be passing around. A worktree with a detached
// HEAD carries no branch line at all and can never match.
func (g *Git) WorktreeFor(branch string) (path string, ok bool, err error) {
	out, rerr := g.run("worktree", "list", "--porcelain")
	if rerr != nil {
		return "", false, g.asEnv(rerr)
	}
	want := "branch refs/heads/" + strings.TrimPrefix(strings.TrimSpace(branch), "refs/heads/")

	var current string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = strings.TrimPrefix(line, "worktree ")
		case line == want:
			return current, true, nil
		}
	}
	return "", false, nil
}

// HeadBranch reports the branch HEAD points at, or "" when HEAD is detached. It
// shells `git symbolic-ref --quiet --short HEAD`, whose exit 1 means exactly
// "HEAD is not a symbolic ref", i.e. detached. --quiet is load-bearing: without
// it the same state exits 128 with a fatal, which is indistinguishable from a
// broken repo.
//
// It is deliberately not Branch. Branch shells `rev-parse --abbrev-ref HEAD`,
// which answers the literal string "HEAD" when detached (a value that cannot be
// compared against a branch name without lying) and exits 128 on an unborn
// branch (a repo with no commits, where symbolic-ref still answers correctly).
// sync resolves the integration branch through this method, so both of those
// postures have to be answerable.
func (g *Git) HeadBranch() (string, error) {
	out, err := g.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && ge.exitCode == 1 {
			return "", nil
		}
		return "", g.asEnv(err)
	}
	return strings.TrimSpace(out), nil
}

// MergeBase reports the best common ancestor of a and b. Two refs with no common
// ancestor (unrelated histories) exit 1 with no output, and a ref that does not
// resolve exits 128; both are the absence of a base rather than an environment
// fault, so both answer ("", nil) and let the caller conclude "nothing to
// compare". A non-zero exit where both refs do resolve is a real git failure and
// keeps the git_unavailable EnvError.
func (g *Git) MergeBase(a, b string) (string, error) {
	out, err := g.run("merge-base", a, b)
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) {
			if ge.exitCode == 1 {
				return "", nil
			}
			aOK, aErr := g.RefExists(a)
			bOK, bErr := g.RefExists(b)
			if aErr == nil && bErr == nil && (!aOK || !bOK) {
				return "", nil
			}
		}
		return "", g.asEnv(err)
	}
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

// PatchID reports the stable patch-id of the tree-to-tree diff from -> to, or ""
// when that diff is empty. It is the branch side of the squash observation: the
// net change a branch carries against its merge base, reduced to an id that
// survives being replayed at a different point in history.
//
// The flags are not decoration. --no-renames is mandatory on BOTH sides of the
// comparison: git log is porcelain and honours a user's diff.renames config,
// git diff-tree is plumbing and does not, and a renamed file hashes to a
// different patch-id under -M than under --no-renames, so pinning both to off is
// the only way the two sides stay comparable across arbitrary user config.
// --no-ext-diff and --no-textconv neutralize a configured external differ or
// textconv filter, either of which would feed patch-id something that is not a
// patch. Nothing here writes: patch-id computes an id and discards it.
func (g *Git) PatchID(from, to string) (string, error) {
	out, err := g.runPipe(
		[]string{"diff-tree", "-p", "--no-commit-id", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", from, to},
		[]string{"patch-id", "--stable"},
	)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

// PatchDigest reports a whitespace-faithful digest of the same diff PatchID
// reduces, over the same flags so the two always describe one patch. It exists
// because `git patch-id` deliberately strips whitespace before hashing, so three
// different indentations of one line collapse to a single id. On a YAML file, a
// Python file, or a Makefile that is a semantic difference, and a fallback that
// treated it as a match would record a done crossing for a change the
// integration branch never received. PatchID narrows the candidates cheaply;
// this confirms the one it found.
//
// It answers "" when the diff is empty or either ref does not resolve, so a
// caller that cannot confirm concludes "no match" rather than trusting the id
// alone.
func (g *Git) PatchDigest(from, to string) (string, error) {
	out, err := g.runRaw(
		"diff-tree", "-p", "--no-commit-id", "--no-color", "--no-ext-diff",
		"--no-textconv", "--no-renames", from, to)
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) {
			fromOK, ferr := g.RefExists(from)
			toOK, terr := g.RefExists(to)
			if ferr == nil && terr == nil && (!fromOK || !toOK) {
				return "", nil
			}
		}
		return "", g.asEnv(err)
	}
	if strings.TrimSpace(out) == "" {
		return "", nil
	}
	return patchDigest(out), nil
}

// patchDigest reduces one unified diff to a digest that is faithful to
// whitespace but tolerant of where the patch landed, which is the exact pair of
// properties the squash confirmation needs.
//
// Two line kinds are normalized away, and only two. A hunk header's ranges move
// whenever the same change lands at a different offset, which is the normal case
// for a squash: patch-id ignores them for that reason and so does this, keeping
// the hunk's own lines as the thing being compared. An index line carries the
// pre- and post-image blob hashes, which shift whenever the integration branch
// touched the same file outside the hunk, so keeping it would reject genuine
// squashes; its content is already carried by the +/- lines. The one exception is
// a binary file, whose patch body is the single line "Binary files a/x and b/x
// differ" and carries no content at all: there the blob hashes are the only
// evidence there is, so the index line is kept.
//
// Every other line, including each +/- and context line, is hashed byte for byte
// with its leading and trailing whitespace intact.
func patchDigest(patch string) string {
	lines := strings.Split(patch, "\n")
	sum := sha256.New()
	for i, line := range lines {
		if strings.HasPrefix(line, "index ") {
			binary := i+1 < len(lines) && strings.HasPrefix(lines[i+1], "Binary files ")
			if !binary {
				continue
			}
		}
		if strings.HasPrefix(line, "@@") {
			line = "@@"
		}
		sum.Write([]byte(line))
		sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// FirstParentPatchIDs returns the stable patch-ids of the newest limit
// first-parent commits of ref, newest first. It is the integration-branch side of
// the squash observation: what the branch has to be matched against.
//
// One `git log -p` streams into one `git patch-id --stable`, so a fifty-commit
// scan costs two processes rather than a hundred. --format='commit %H' keeps the
// `commit <sha>` header patch-id uses to attribute each patch while dropping the
// commit message, so a message that quotes a `diff --git` line cannot corrupt the
// mapping; --no-notes does the same for notes. --no-renames pins the porcelain
// side to match the plumbing side (see PatchID).
//
// No --diff-merges flag is passed, so the local git's default applies to merge
// commits. Both outcomes are safe. On a git that emits no patch for a merge, the
// merge commit is simply absent from the result. On a git that emits its
// first-parent diff (2.31 and later, when --first-parent is given), a merge
// commit that actually carried the branch under test is unreachable, because
// strict ancestry already answered yes before the caller ever falls back here.
//
// A commit whose diff is empty contributes no line and is omitted.
func (g *Git) FirstParentPatchIDs(ref string, limit int) ([]CommitPatch, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, err := g.runPipe(
		[]string{"log", "--first-parent", fmt.Sprintf("--max-count=%d", limit), "-p",
			"--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-notes",
			"--format=commit %H", ref},
		[]string{"patch-id", "--stable"},
	)
	if err != nil {
		return nil, err
	}
	var pairs []CommitPatch
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pairs = append(pairs, CommitPatch{Commit: fields[1], PatchID: fields[0]})
	}
	return pairs, nil
}

// runPipe runs `git <first> | git <second>` in g.Dir and returns the second
// command's trimmed stdout. The producer's stdout is wired straight onto the
// consumer's stdin as a file descriptor, so a long `log -p` streams through
// patch-id instead of being buffered whole in this process. The consumer is
// waited on first: waiting on the producer closes the read end, which would
// truncate a consumer still reading. A non-zero exit from either side is
// classified exactly as run does.
func (g *Git) runPipe(first, second []string) (string, error) {
	producer := exec.Command("git", first...)
	producer.Dir = g.Dir
	producer.Env = append(producer.Environ(), "LC_ALL=C", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	var prodErr bytes.Buffer
	producer.Stderr = &prodErr

	pipe, err := producer.StdoutPipe()
	if err != nil {
		return "", g.unavailable(err.Error())
	}

	consumer := exec.Command("git", second...)
	consumer.Dir = g.Dir
	consumer.Env = append(consumer.Environ(), "LC_ALL=C", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1")
	consumer.Stdin = pipe
	var out, consErr bytes.Buffer
	consumer.Stdout = &out
	consumer.Stderr = &consErr

	if err := producer.Start(); err != nil {
		return "", g.unavailable(err.Error())
	}
	if err := consumer.Start(); err != nil {
		_ = producer.Wait()
		return "", g.unavailable(err.Error())
	}

	consumerErr := consumer.Wait()
	producerErr := producer.Wait()

	if perr := g.pipeStageErr(first, producerErr, prodErr.String()); perr != nil {
		return "", perr
	}
	if cerr := g.pipeStageErr(second, consumerErr, consErr.String()); cerr != nil {
		return "", cerr
	}
	return strings.TrimSpace(out.String()), nil
}

// pipeStageErr classifies one stage of a runPipe: a non-zero exit and a binary
// that could not be launched both become git_unavailable EnvErrors, matching how
// the single-command path surfaces a failure the caller has no boolean reading
// for.
func (g *Git) pipeStageErr(args []string, err error, stderr string) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return g.asEnv(&gitError{args: args, exitCode: exitErr.ExitCode(), stderr: stderr})
	}
	return g.unavailable(err.Error())
}
