package vcs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

// HeadSnapshot answers TreeHash and BlobHash from one inventory of HEAD's tree
// instead of two git processes per path. A drift sweep asks for every recorded
// surface fingerprint, and a project with dozens of surfaces paid dozens of
// process pairs per sweep. The inventory is read on first use and pinned for
// the snapshot's life, so one sweep reads one consistent HEAD; build a new
// snapshot per sweep. Paths are project-relative, as they are for Git: the
// inventory is keyed by repository-root path, and a lookup joins the project's
// prefix onto the path it is asked for. Every other Revisions method is the
// embedded Git's.
type HeadSnapshot struct {
	*Git
	once    sync.Once
	prefix  string
	entries map[string]headObject
	err     error
}

type headObject struct{ kind, oid string }

// NewHeadSnapshot wraps g for one read-only sweep.
func NewHeadSnapshot(g *Git) *HeadSnapshot { return &HeadSnapshot{Git: g} }

func (h *HeadSnapshot) load() {
	h.once.Do(func() {
		prefix, err := h.run("rev-parse", "--show-prefix")
		if err != nil {
			h.err = err
			return
		}
		tree, err := h.run("rev-parse", "--verify", "--quiet", "HEAD^{tree}")
		if err != nil {
			h.err = err
			return
		}
		out, err := h.runRaw("ls-tree", "-r", "-t", "-z", "--full-tree", tree)
		if err != nil {
			h.err = err
			return
		}
		// The repository root is a tree too, keyed "" like the project-relative
		// lookup below spells it for a project at the root.
		entries := map[string]headObject{"": {kind: "tree", oid: tree}}
		for _, record := range bytes.Split([]byte(out), []byte{0}) {
			if len(record) == 0 {
				continue
			}
			metadata, name, ok := strings.Cut(string(record), "\t")
			fields := strings.Fields(metadata)
			if !ok || len(fields) != 3 {
				h.err = errors.New("vcs: invalid HEAD tree inventory")
				return
			}
			entries[name] = headObject{kind: fields[1], oid: fields[2]}
		}
		h.prefix, h.entries = strings.TrimSuffix(prefix, "/"), entries
	})
}

// TreeHash is Git.TreeHash answered from the pinned HEAD inventory.
func (h *HeadSnapshot) TreeHash(path string) (string, error) {
	return h.object(path, "tree")
}

// BlobHash is Git.BlobHash answered from the pinned HEAD inventory.
func (h *HeadSnapshot) BlobHash(path string) (string, error) {
	return h.object(path, "blob")
}

// object resolves a project-relative path, Git's own coordinates, against the
// repository-root inventory by joining the project's prefix onto it.
func (h *HeadSnapshot) object(path, want string) (string, error) {
	// Anything that is not a plain relative path (a trailing slash, ./ or ../
	// forms) keeps Git's own resolution.
	if path != "" && !fs.ValidPath(path) {
		if want == "tree" {
			return h.Git.TreeHash(path)
		}
		return h.Git.BlobHash(path)
	}
	h.load()
	if h.err != nil {
		return "", h.classifyPathErr(h.err, path)
	}
	key := h.prefix
	if path != "" && path != "." {
		key = strings.TrimPrefix(h.prefix+"/"+path, "/")
	}
	entry, ok := h.entries[key]
	if !ok {
		return "", &NotVersionedError{Path: path}
	}
	// A gitlink names a commit this repository does not hold; Git's cat-file
	// fails on it, which Git reports as a plain NotVersionedError.
	if entry.kind == "commit" {
		return "", &NotVersionedError{Path: path}
	}
	if entry.kind != want {
		return "", &NotVersionedError{Path: path, Msg: fmt.Sprintf("vcs: path %q is a %s at HEAD, not a %s", path, entry.kind, want)}
	}
	return entry.oid, nil
}
