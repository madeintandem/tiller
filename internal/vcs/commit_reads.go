package vcs

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// CommitRef returns commit as a git_commit RevisionRef in the compact
// "git:<short-sha>" value form, observed now. It is CurrentCommit for a commit
// named by the caller, so a completion record can pin the integration commit
// sync observed rather than whatever the checkout's HEAD happens to be.
func (g *Git) CommitRef(commit string) (store.RevisionRef, error) {
	short, err := g.run("rev-parse", "--short", "--verify", "--quiet", commit+"^{commit}")
	if err != nil {
		return store.RevisionRef{}, g.classifyPathErr(err, commit)
	}
	return store.RevisionRef{
		Kind:       store.KindGitCommit,
		Value:      "git:" + short,
		ObservedAt: g.now(),
	}, nil
}

// BlobHashAt reports the git blob hash of path's content in commit. path is
// project-relative: it resolves from g.Dir, the Tiller project directory, the
// same coordinates recorded surfaces and prose pins are stored in. In a nested
// project (a Tiller root below the repository root) that keeps a project path
// from ever naming a repository-root sibling of the same name. A path that is
// absent or is not a blob yields a NotVersionedError.
func (g *Git) BlobHashAt(commit, path string) (string, error) {
	return g.objectHashAt(commit, path, "blob")
}

// TreeHashAt reports the git tree hash of a subtree of commit, with the same
// project-relative resolution as BlobHashAt. An empty path (or ".") is the
// project directory's tree, which is commit's whole tree for a project at the
// repository root. A path that is absent or is not a tree yields a
// NotVersionedError.
func (g *Git) TreeHashAt(commit, path string) (string, error) {
	return g.objectHashAt(commit, path, "tree")
}

// projectObject spells path for `<commit>:<path>` from g.Dir. Git resolves a
// "./"-prefixed object name from the working directory instead of the
// repository root.
func projectObject(commit, path string) string {
	if path == "." {
		path = ""
	}
	return commit + ":./" + path
}

// objectHashAt resolves commit:path to its object id and verifies the object's
// type is wantType, so BlobHashAt and TreeHashAt never silently return the
// wrong kind of hash for a mismatched path.
func (g *Git) objectHashAt(commit, path, wantType string) (string, error) {
	out, err := g.run("rev-parse", "--verify", "--quiet", projectObject(commit, path))
	if err != nil {
		return "", g.classifyPathErr(err, path)
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", &NotVersionedError{Path: path}
	}

	typ, terr := g.run("cat-file", "-t", sha)
	if terr != nil {
		return "", g.classifyPathErr(terr, path)
	}
	if got := strings.TrimSpace(typ); got != wantType {
		return "", &NotVersionedError{
			Path: path,
			Msg:  fmt.Sprintf("vcs: path %q is a %s at %s, not a %s", path, got, commit, wantType),
		}
	}
	return sha, nil
}

// TreeInventoryAt lists the file paths under a subtree of commit, sorted. An
// empty path inventories the whole tree visible from g.Dir. It shells `git
// ls-tree -r -z --name-only` with a literal pathspec, a read-only enumeration
// whose NUL-delimited names are kept verbatim, so core.quotepath and unusual
// bytes in a filename cannot change what the inventory records.
func (g *Git) TreeInventoryAt(commit, path string) ([]string, error) {
	args := []string{"--literal-pathspecs", "ls-tree", "-r", "-z", "--name-only", commit}
	if path != "" && path != "." {
		args = append(args, "--", path)
	}
	out, err := g.runRaw(args...)
	if err != nil {
		return nil, g.classifyPathErr(err, path)
	}
	var names []string
	for _, name := range bytes.Split([]byte(out), []byte{0}) {
		if len(name) > 0 {
			names = append(names, string(name))
		}
	}
	sort.Strings(names)
	return names, nil
}
