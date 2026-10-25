package vcs

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// BaselineSource resolves a regular-file blob at the already observed project
// commit. It never reads working-tree bytes: a translated source may already
// have been retired there, while its recorded blob remains recoverable by Git.
func BaselineSource(root string, commit store.RevisionRef, source string, at time.Time) (store.RevisionRef, error) {
	if commit.Kind != store.KindGitCommit || !strings.HasPrefix(commit.Value, "git:") {
		return store.RevisionRef{}, fmt.Errorf("baseline source needs an observed project commit")
	}
	hash := strings.TrimPrefix(commit.Value, "git:")
	if len(hash) < 4 || len(hash) > 64 {
		return store.RevisionRef{}, fmt.Errorf("invalid project commit %q", commit.Value)
	}
	for _, c := range hash {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return store.RevisionRef{}, fmt.Errorf("invalid project commit %q", commit.Value)
		}
	}
	if source == "" || source == "." || source == ".." || strings.HasPrefix(source, "../") || path.IsAbs(source) || path.Clean(source) != source || strings.ContainsAny(source, "\\\x00\r\n") {
		return store.RevisionRef{}, fmt.Errorf("invalid project source path %q", source)
	}
	git := New(root)
	// Literal pathspecs preserve commas, brackets and leading colons. -z avoids
	// quoting filenames, and project-relative names keep nested projects aligned
	// with source. The exact-name check excludes descendant matches.
	out, err := git.runRaw("--literal-pathspecs", "ls-tree", "-z", hash, "--", source)
	if err != nil {
		return store.RevisionRef{}, git.asEnv(err)
	}
	for _, entry := range strings.Split(out, "\x00") {
		metadata, name, ok := strings.Cut(entry, "\t")
		if !ok || name != source {
			continue
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return store.RevisionRef{}, &NotVersionedError{Path: source, Msg: fmt.Sprintf("baseline source %q is not a regular file at %s", source, commit.Value)}
		}
		return store.RevisionRef{Kind: store.KindGitBlob, Value: "git:" + fields[2], Path: source, ObservedAt: at}, nil
	}
	return store.RevisionRef{}, &NotVersionedError{Path: source, Msg: fmt.Sprintf("baseline source %q is not a committed regular file at %s; commit the source before recording provenance", source, commit.Value)}
}
