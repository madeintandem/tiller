package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// preAnchoringGitignoreText is the block init and join wrote before issue #104.
// Existing repositories must retain these bytes when the new patterns land.
const preAnchoringGitignoreText = "# Tiller-generated, not source. Safe to delete; Tiller re-runs the gates and\n" +
	"# regenerates the cache. Keep these out of your code diff.\n" +
	"artifacts/\n" +
	".tiller/.cache/\n" +
	".bin/\n" +
	"# Per-checkout identity (who you are here), written by `tiller join`; never commit it.\n" +
	".tiller/identity.yml\n" +
	"\n" +
	"# Tiller skill shims (installed per machine by `tiller join`; not source).\n" +
	".claude/skills/tiller-*/\n"

func TestUpsertIsIdempotent(t *testing.T) {
	once := upsertIgnoreGroups("node_modules/\n", tillerIgnoreGroups)
	require.Equal(t, once, upsertIgnoreGroups(once, tillerIgnoreGroups))
}

func TestUpsertAppendsMissingGroupsAfterExistingContent(t *testing.T) {
	groups := []ignoreGroup{{comment: "# generated", patterns: []string{"/.bin/"}}}
	for _, existing := range []string{"", "node_modules/\n*.log\n", "*.log"} {
		t.Run(existing, func(t *testing.T) {
			want := existing
			if want != "" {
				if !strings.HasSuffix(want, "\n") {
					want += "\n"
				}
				want += "\n"
			}
			want += "# generated\n/.bin/\n"
			require.Equal(t, want, upsertIgnoreGroups(existing, groups))
		})
	}
}

// A file missing only some patterns of a group gets that group's comment and
// the missing patterns only; covering legacy lines count as present.
func TestUpsertAddsOnlyTheMissingPatternsOfAGroup(t *testing.T) {
	existing := "artifacts/\n.tiller/identity.yml\n.claude/skills/tiller-*/\n"
	groups := []ignoreGroup{
		{comment: "# generated", patterns: []string{"/artifacts/tiller/", "/.tiller/.cache/", "/.bin/"}},
		{comment: "# identity", patterns: []string{"/.tiller/identity.yml"}},
		{comment: "# shims", patterns: []string{"/.claude/skills/tiller-*/"}},
	}
	got := upsertIgnoreGroups(existing, groups)
	want := existing + "\n# generated\n/.tiller/.cache/\n/.bin/\n"
	require.Equal(t, want, got)
}

func TestUpsertRecognizesCoveringIgnoreLines(t *testing.T) {
	for _, tt := range []struct {
		name, existing, pattern string
		covered                 bool
	}{
		{"trimmed exact line", "a/\n  /.bin/  \n", "/.bin/", true},
		{"unanchored twin", ".bin/\n", "/.bin/", true},
		{"anchored parent", "/artifacts/\n", "/artifacts/tiller/", true},
		{"legacy parent", "artifacts/\n", "/artifacts/tiller/", true},
		{"parent of file", ".tiller/\n", "/.tiller/identity.yml", true},
		{"same glob", ".claude/skills/tiller-*/\n", "/.claude/skills/tiller-*/", true},
		{"child is not parent", ".bin/x\n", "/.bin/", false},
		{"directory marker required", "artifacts\n", "/artifacts/tiller/", false},
		{"negation", "!artifacts/\n", "/artifacts/tiller/", false},
		{"comment", "# artifacts/\n", "/artifacts/tiller/", false},
		{"glob is not plain parent", "art*/\n", "/artifacts/tiller/", false},
		{"empty file", "", "/.bin/", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			groups := []ignoreGroup{{comment: "# generated", patterns: []string{tt.pattern}}}
			got := upsertIgnoreGroups(tt.existing, groups)
			if tt.covered {
				require.Equal(t, tt.existing, got, "a covering line leaves the file untouched")
			} else {
				require.True(t, strings.HasPrefix(got, tt.existing), "existing content is preserved")
				require.Contains(t, strings.Split(strings.TrimPrefix(got, tt.existing), "\n"), tt.pattern)
			}
		})
	}
}
