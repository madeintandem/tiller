package vcs

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"

	"github.com/madeintandem/tiller/internal/store"
)

// ReasonWorktreeUnreadable is the store.EnvError discriminator for a changed
// working-tree path that exists but could not be read while fingerprinting it.
// It maps to exit 3, the environment class, and is never folded into the
// deletion sentinel.
const ReasonWorktreeUnreadable = "worktree_unreadable"

// Entry identities that are not blob hashes. A deleted path is "-"; a directory
// standing where Git reports a path (a submodule or an untracked nested
// repository) is "tree". Neither can collide with a 40-hex blob id.
const (
	deletedIdentity   = "-"
	directoryIdentity = "tree"
)

// WorktreeFingerprint returns a deterministic sha256 RevisionRef over the current
// working-tree state, the honest provenance for a gate that ran over a dirty tree
// (where HEAD does not contain the tested code).
//
// The digest covers the whole repository in one coordinate system, the
// repository root, whichever directory inside it g observes from. It hashes the
// HEAD sha together with the sorted list of (path, working-tree identity) pairs
// for every path that differs from HEAD (`git diff --name-only --no-renames -z
// HEAD`, so a rename contributes both its deletion and its addition) plus every
// untracked, non-ignored file (`git ls-files --others --exclude-standard -z`).
// Both lists are read NUL-delimited and untrimmed, so core.quotepath, non-ASCII
// bytes, tabs, newlines, and edge spaces cannot alter a path's identity. Each
// path is hashed against the repository root, so a nested project never reads
// a similarly named child path in place of the real one.
//
// A file's identity is its Git blob id computed in-process (nothing is written
// to the object store), a symlink's is the blob id of its target, as Git stores
// it. A path that no longer exists, or that now sits beyond a symlink or a file
// where a directory was, is recorded with the deletion sentinel. A path that
// exists but cannot be read is a worktree_unreadable error, never a deletion.
// No index, object, stash, or ref is written.
func (g *Git) WorktreeFingerprint() (store.RevisionRef, error) {
	top, err := g.runRaw("rev-parse", "--show-toplevel")
	if err != nil {
		return store.RevisionRef{}, g.asEnv(err)
	}
	repo := &Git{Dir: strings.TrimSuffix(top, "\n"), Clock: g.Clock}

	head, err := repo.run("rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		// An unborn HEAD (no commits yet) has no base sha; fingerprint the working
		// tree alone rather than failing.
		head = ""
	}

	paths := map[string]struct{}{}
	if head != "" {
		diff, err := repo.runRaw("--no-optional-locks", "diff", "--name-only", "--no-renames", "-z", "HEAD")
		if err != nil {
			return store.RevisionRef{}, g.asEnv(err)
		}
		addNULPaths(paths, diff)
	}
	others, err := repo.runRaw("--no-optional-locks", "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return store.RevisionRef{}, g.asEnv(err)
	}
	addNULPaths(paths, others)

	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	root, err := os.OpenRoot(repo.Dir)
	if err != nil {
		return store.RevisionRef{}, unreadable(".", err)
	}
	defer root.Close()

	h := sha256.New()
	fmt.Fprintf(h, "head\x00%s\n", head)
	for _, p := range sorted {
		identity, err := worktreeIdentity(root, p)
		if err != nil {
			return store.RevisionRef{}, unreadable(p, err)
		}
		fmt.Fprintf(h, "%s\x00%s\n", p, identity)
	}

	return store.RevisionRef{
		Kind:       store.KindSHA256,
		Value:      "sha256:" + hex.EncodeToString(h.Sum(nil)),
		ObservedAt: g.now(),
	}, nil
}

// addNULPaths adds each non-empty field of a NUL-terminated path list verbatim.
func addNULPaths(paths map[string]struct{}, out string) {
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths[p] = struct{}{}
		}
	}
}

// worktreeIdentity reports the working-tree identity of one repository-relative
// path read beneath root, following Git's own view of the tree: no component
// before the leaf may be a symlink or a non-directory.
func worktreeIdentity(root *os.Root, name string) (string, error) {
	name = strings.TrimSuffix(name, "/")
	segments := strings.Split(name, "/")
	for i := 1; i < len(segments); i++ {
		info, err := root.Lstat(path.Join(segments[:i]...))
		if missing(err) {
			return deletedIdentity, nil
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return deletedIdentity, nil
		}
	}

	info, err := root.Lstat(name)
	if missing(err) {
		return deletedIdentity, nil
	}
	if err != nil {
		return "", err
	}
	switch mode := info.Mode(); {
	case mode&fs.ModeSymlink != 0:
		target, err := root.Readlink(name)
		if err != nil {
			return "", err
		}
		return blobID([]byte(target)), nil
	case mode.IsDir():
		return directoryIdentity, nil
	case !mode.IsRegular():
		// Git does not track sockets, devices, or pipes; one standing where a
		// file was is that file's absence.
		return deletedIdentity, nil
	}

	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return blobID(data), nil
}

// missing reports a path that does not exist, including one whose parent is no
// longer a directory.
func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// blobID is the Git blob object id of data, computed without writing an object.
func blobID(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func unreadable(name string, err error) error {
	return &store.EnvError{
		Code:   3,
		Reason: ReasonWorktreeUnreadable,
		Msg:    fmt.Sprintf("working-tree fingerprint could not read %q: %v", name, err),
	}
}
