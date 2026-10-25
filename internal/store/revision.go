package store

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RevisionRef is the one identifier primitive that pins a file, tree, or
// artifact to a point in history (artifact-schema.md, "Revisions"). Every
// revision-bearing field stores one of these rather than a bare string.
type RevisionRef struct {
	// Kind says what Value addresses: a git blob (one file's content), a git
	// commit, a git tree (a subtree of paths), or a sha256 content hash for
	// artifacts that live outside git.
	Kind string `yaml:"kind,omitempty"`
	// Value is the "git:<short-sha>" or "sha256:<hex>" token, prefix included.
	Value string `yaml:"value,omitempty"`
	// Path is the repo-relative path the ref was taken against. Present for
	// blob and tree kinds, omitted for a bare commit.
	Path string `yaml:"path,omitempty"`
	// ObservedAt is when Tiller recorded the ref. It is what lets a stale
	// review tell "the file changed" from "we last looked a while ago."
	ObservedAt time.Time `yaml:"observed_at,omitempty"`
}

// SameAs compares two RevisionRefs by what they mean rather than by their Go
// representation. Plain struct == / != is a trap here: ObservedAt is a
// time.Time, and struct equality on a time.Time compares its internal
// *Location pointer too, not just the instant. A ledger round trip can
// change that pointer without changing anything a human would call "the
// same revision": an "+00:00" offset parses to a fixed zone at a distinct
// address from the one "Z" parses to, and either can be re-marshaled as the
// other, so a value stamped once and read back compares unequal to itself
// under == forever. time.Time.Equal compares the instant alone and is true
// for two zero values, so a caller checking "no revision recorded" against
// a zero RevisionRef still gets the right answer through this method.
func (r RevisionRef) SameAs(o RevisionRef) bool {
	return r.Kind == o.Kind && r.Value == o.Value && r.Path == o.Path &&
		r.ObservedAt.Equal(o.ObservedAt)
}

// RevisionRef Kind values.
const (
	KindGitBlob   = "git_blob"
	KindGitCommit = "git_commit"
	KindGitTree   = "git_tree"
	KindSHA256    = "sha256"
)

// ValidKind reports whether kind is one of the four recognized RevisionRef kinds.
func ValidKind(kind string) bool {
	switch kind {
	case KindGitBlob, KindGitCommit, KindGitTree, KindSHA256:
		return true
	default:
		return false
	}
}

// rawRevisionRef mirrors RevisionRef without the custom unmarshaler, so the
// mapping branch can decode without recursing.
type rawRevisionRef RevisionRef

// UnmarshalYAML accepts both forms a RevisionRef appears in on disk: the full
// mapping (kind/value/path/observed_at) and the compact value-only shorthand
// ("git:31fd9c2" or "sha256:<hex>"). The compact git form, having no path, is a
// commit per the "path omitted for a bare commit" rule.
func (r *RevisionRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag == "!!null" {
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		return r.parseCompact(node.Value)
	}

	var raw rawRevisionRef
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*r = RevisionRef(raw)
	if r.Kind != "" && !ValidKind(r.Kind) {
		return fmt.Errorf("revision: unknown kind %q (want one of %s, %s, %s, %s)",
			r.Kind, KindGitBlob, KindGitCommit, KindGitTree, KindSHA256)
	}
	return nil
}

func (r *RevisionRef) parseCompact(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(s, "git:"):
		r.Kind = KindGitCommit
	case strings.HasPrefix(s, "sha256:"):
		r.Kind = KindSHA256
	default:
		return fmt.Errorf("revision: unrecognized compact ref %q (want git:<sha> or sha256:<hex>)", s)
	}
	r.Value = s
	return nil
}
