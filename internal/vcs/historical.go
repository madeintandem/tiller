package vcs

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// HistoricalRevisions is an optional immutable-tree observation capability.
// Keeping it separate from Revisions preserves the ordinary gate/sync surface.
// Paths are canonical forward-slash paths relative to Git.Dir, even when that
// project is nested inside a larger repository. No method reads checkout bytes.
// Missing, ambiguous, non-commit, unsafe, or oversized inputs return a
// NotVersionedError; unavailable Git remains a git_unavailable EnvError.
// ReadFileAt accepts regular files only and is bounded to 4 MiB. FilesAt returns
// sorted recursive regular-file paths and refuses symlinks and gitlinks rather
// than silently omitting potential historical state. ReadBlob reads one blob by
// object ID (optionally "git:"-prefixed) under the same bound.
type HistoricalRevisions interface {
	ResolveCommit(oid string) (string, error)
	CommitParents(oid string) ([]string, error)
	ReadFileAt(commit, path string) ([]byte, store.RevisionRef, error)
	FilesAt(commit, dir string) ([]string, error)
	ReadBlob(oid string) ([]byte, error)
}

var _ HistoricalRevisions = (*Git)(nil)

const (
	maxHistoricalFileBytes = 4 * 1024 * 1024
	maxHistoricalTreeBytes = 16 * 1024 * 1024
)

var errHistoricalOutputLimit = errors.New("historical git output exceeds size limit")

type historicalOutput struct {
	buffer    bytes.Buffer
	remaining int
	exceeded  bool
}

func (b *historicalOutput) Write(p []byte) (int, error) {
	if len(p) > b.remaining {
		b.exceeded = true
		return 0, errHistoricalOutputLimit
	}
	b.remaining -= len(p)
	return b.buffer.Write(p)
}

// historicalRead uses the existing runner's error convention while bounding
// stdout during the read, not after allocating an arbitrarily large blob/tree.
// The shared runner disables replacement objects and lazy fetch, so historical
// reads agree with ancestry and patch primitives on the actual local objects.
func (g *Git) historicalRead(subject string, limit int, args ...string) ([]byte, error) {
	out := historicalOutput{remaining: limit}
	args = append([]string{"--literal-pathspecs"}, args...)
	err := g.runOutput(&out, args...)
	if out.exceeded {
		return nil, &NotVersionedError{Path: subject, Msg: fmt.Sprintf("vcs: historical input %q exceeds the %d-byte read limit", subject, limit)}
	}
	if err != nil {
		return nil, g.classifyPathErr(err, subject)
	}
	return out.buffer.Bytes(), nil
}

// IsHexObjectID reports whether oid is 4 to 64 hexadecimal digits, the only
// form a historical commit argument may take: never a branch name or a
// revision expression.
func IsHexObjectID(oid string) bool {
	if len(oid) < 4 || len(oid) > 64 {
		return false
	}
	for _, c := range oid {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func historicalFullOID(oid string) bool {
	return (len(oid) == 40 || len(oid) == 64) && IsHexObjectID(oid)
}

// ResolveCommit resolves an object-ID prefix, never a ref or revision expression.
// Uniqueness is across all object kinds; a tag is not peeled into a commit.
func (g *Git) ResolveCommit(oid string) (string, error) {
	if !IsHexObjectID(oid) {
		return "", &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: commit %q must be a hexadecimal object ID of at least four digits", oid)}
	}
	if err := g.ensureRepo(); err != nil {
		return "", err
	}
	out, err := g.historicalRead(oid, maxHistoricalTreeBytes, "rev-parse", "--disambiguate="+strings.ToLower(oid))
	if err != nil {
		return "", err
	}
	matches := strings.Fields(string(out))
	if len(matches) != 1 || !historicalFullOID(matches[0]) {
		return "", &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: commit %q is missing or ambiguous; restore the object or provide its full object ID", oid)}
	}
	full := matches[0]
	typ, err := g.historicalRead(oid, 128, "cat-file", "-t", full)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(typ)) != "commit" {
		return "", &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: object %q is not a commit", oid)}
	}
	return full, nil
}

// CommitParents returns the literal commit object's parent IDs in stored order,
// including all parents of an octopus merge and none for a root commit. Reading
// the object header avoids grafts/shallow traversal rewriting this evidence.
func (g *Git) CommitParents(oid string) ([]string, error) {
	full, err := g.ResolveCommit(oid)
	if err != nil {
		return nil, err
	}
	out, err := g.historicalRead(oid, maxHistoricalFileBytes, "cat-file", "commit", full)
	if err != nil {
		return nil, err
	}
	headers, _, _ := bytes.Cut(out, []byte("\n\n"))
	var parents []string
	for _, line := range bytes.Split(headers, []byte("\n")) {
		if parent, ok := strings.CutPrefix(string(line), "parent "); ok {
			if !historicalFullOID(parent) {
				return nil, &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: commit %q has an invalid parent object ID", oid)}
			}
			parents = append(parents, parent)
		}
	}
	return parents, nil
}

func historicalPath(name string, directory bool) bool {
	if directory && (name == "" || name == ".") {
		return true
	}
	return name != "" && name != "." && name != ".." && !path.IsAbs(name) && !strings.HasPrefix(name, "../") && path.Clean(name) == name && !strings.ContainsAny(name, "\\\x00\r\n\t")
}

type historicalEntry struct {
	mode string
	kind string
	oid  string
	name string
}

func parseHistoricalEntry(record string) (historicalEntry, error) {
	metadata, name, ok := strings.Cut(record, "\t")
	fields := strings.Fields(metadata)
	if !ok || len(fields) != 3 || !historicalFullOID(fields[2]) || !historicalPath(name, false) {
		return historicalEntry{}, &NotVersionedError{Path: name, Msg: "vcs: historical tree contains an invalid entry"}
	}
	return historicalEntry{mode: fields[0], kind: fields[1], oid: fields[2], name: name}, nil
}

func (entry historicalEntry) regular() bool {
	return entry.kind == "blob" && (entry.mode == "100644" || entry.mode == "100755")
}

// historicalEntryAt names paths from the repository root explicitly. In a
// nested project neither the root's similarly named document nor a sibling
// project is a valid substitute. Literal pathspecs preserve brackets/colons.
func (g *Git) historicalEntryAt(commit, name string) (historicalEntry, error) {
	prefixBytes, err := g.historicalRead(name, maxHistoricalFileBytes, "rev-parse", "--show-prefix")
	if err != nil {
		return historicalEntry{}, err
	}
	prefix := strings.TrimSuffix(string(prefixBytes), "\n")
	if name == "." {
		name = ""
	}
	repoPath := strings.TrimSuffix(prefix+name, "/")
	if repoPath == "" {
		return historicalEntry{mode: "040000", kind: "tree", oid: commit}, nil
	}
	if !historicalPath(repoPath, false) {
		return historicalEntry{}, &NotVersionedError{Path: name, Msg: "vcs: historical project path is unsafe"}
	}
	out, err := g.historicalRead(name, maxHistoricalTreeBytes, "ls-tree", "--full-tree", "-z", commit, "--", repoPath)
	if err != nil {
		return historicalEntry{}, err
	}
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		entry, err := parseHistoricalEntry(string(record))
		if err != nil {
			return historicalEntry{}, err
		}
		if entry.name == repoPath {
			return entry, nil
		}
	}
	return historicalEntry{}, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: path %q is absent from historical commit %s", name, commit)}
}

func (g *Git) ReadFileAt(commit, name string) ([]byte, store.RevisionRef, error) {
	if !historicalPath(name, false) {
		return nil, store.RevisionRef{}, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: unsafe historical file path %q", name)}
	}
	full, err := g.ResolveCommit(commit)
	if err != nil {
		return nil, store.RevisionRef{}, err
	}
	entry, err := g.historicalEntryAt(full, name)
	if err != nil {
		return nil, store.RevisionRef{}, err
	}
	if !entry.regular() {
		return nil, store.RevisionRef{}, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: historical path %q is not a regular file (mode %s)", name, entry.mode)}
	}
	sizeBytes, err := g.historicalRead(name, 128, "cat-file", "-s", entry.oid)
	if err != nil {
		return nil, store.RevisionRef{}, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeBytes)), 10, 64)
	if err != nil || size < 0 || size > maxHistoricalFileBytes {
		return nil, store.RevisionRef{}, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: historical file %q exceeds the %d-byte read limit or has invalid size", name, maxHistoricalFileBytes)}
	}
	data, err := g.historicalRead(name, maxHistoricalFileBytes, "cat-file", "blob", entry.oid)
	if err != nil {
		return nil, store.RevisionRef{}, err
	}
	if int64(len(data)) != size {
		return nil, store.RevisionRef{}, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: historical file %q has inconsistent blob size", name)}
	}
	return data, store.RevisionRef{Kind: store.KindGitBlob, Value: "git:" + entry.oid, Path: name, ObservedAt: g.now()}, nil
}

// ReadBlob reads one blob by object ID, the form a git_blob RevisionRef carries.
// An optional "git:" prefix is stripped. The ID must be hexadecimal, never a
// ref or revision expression, and the object must be a blob of at most 4 MiB.
// A missing, ambiguous, non-blob, or oversized object is a NotVersionedError.
func (g *Git) ReadBlob(oid string) ([]byte, error) {
	id := strings.ToLower(strings.TrimPrefix(oid, "git:"))
	if !IsHexObjectID(id) {
		return nil, &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: blob %q must be a hexadecimal object ID of at least four digits", oid)}
	}
	if err := g.ensureRepo(); err != nil {
		return nil, err
	}
	typ, err := g.historicalRead(oid, 128, "cat-file", "-t", id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(typ)) != "blob" {
		return nil, &NotVersionedError{Path: oid, Msg: fmt.Sprintf("vcs: object %q is not a blob", oid)}
	}
	return g.historicalRead(oid, maxHistoricalFileBytes, "cat-file", "blob", id)
}

func (g *Git) FilesAt(commit, dir string) ([]string, error) {
	if !historicalPath(dir, true) {
		return nil, &NotVersionedError{Path: dir, Msg: fmt.Sprintf("vcs: unsafe historical directory path %q", dir)}
	}
	full, err := g.ResolveCommit(commit)
	if err != nil {
		return nil, err
	}
	entry, err := g.historicalEntryAt(full, dir)
	if err != nil {
		return nil, err
	}
	if entry.kind != "tree" || entry.mode != "040000" {
		return nil, &NotVersionedError{Path: dir, Msg: fmt.Sprintf("vcs: historical path %q is not a directory", dir)}
	}
	out, err := g.historicalRead(dir, maxHistoricalTreeBytes, "ls-tree", "--full-tree", "-r", "-z", entry.oid)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		child, err := parseHistoricalEntry(string(record))
		if err != nil {
			return nil, err
		}
		name := path.Join(dir, child.name)
		if !child.regular() {
			return nil, &NotVersionedError{Path: name, Msg: fmt.Sprintf("vcs: historical path %q is not a regular file (mode %s)", name, child.mode)}
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}
