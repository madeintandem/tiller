package cli

import "strings"

// ignoreGroup is one commented run of .gitignore patterns Tiller owns: the
// comment lines (newline-joined, no trailing newline) and the patterns they
// explain, one per line.
type ignoreGroup struct {
	comment  string
	patterns []string
}

// tillerIgnoreGroups is every repo-root .gitignore line Tiller owns. init seeds
// the first three groups when no .gitignore exists; join and project maintenance
// upsert all groups without rewriting existing entries.
//
// Root anchors keep these patterns from hiding a project's nested directories.
// Only artifacts/tiller/ belongs to Tiller, not the rest of artifacts/ (#104).
var tillerIgnoreGroups = []ignoreGroup{
	{
		comment: "# Tiller-generated, not source. Safe to delete; Tiller re-runs the gates and\n" +
			"# regenerates the cache. Keep these out of your code diff.",
		patterns: []string{"/artifacts/tiller/", "/.tiller/.cache/", "/.bin/"},
	},
	{
		comment:  "# Per-checkout identity (who you are here), written by `tiller join`; never commit it.",
		patterns: []string{"/.tiller/identity.yml"},
	},
	{
		comment:  "# Per-checkout settings (console preferences and the like), hand-edited; never commit it.",
		patterns: []string{"/.tiller/local.yml"},
	},
	{
		comment:  "# Tiller skill shims (installed per machine by `tiller join`; not source).",
		patterns: []string{"/.claude/skills/tiller-*/"},
	},
}

// renderIgnoreGroups renders groups as a fresh .gitignore carries them: each
// comment, then its patterns one per line, groups back to back.
func renderIgnoreGroups(groups []ignoreGroup) string {
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(g.comment)
		b.WriteString("\n")
		for _, p := range g.patterns {
			b.WriteString(p)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// upsertIgnoreGroups appends whichever of groups' patterns existing lacks,
// each under its group's comment, after one blank line, and returns existing
// untouched when nothing is missing. A group whose patterns are all present
// contributes nothing; one missing some contributes its comment and only the
// missing patterns. The human's own lines are never reordered or rewritten.
func upsertIgnoreGroups(existing string, groups []ignoreGroup) string {
	var missing []ignoreGroup
	for _, g := range groups {
		var absent []string
		for _, p := range g.patterns {
			if !gitignoreCovers(existing, p) {
				absent = append(absent, p)
			}
		}
		if len(absent) > 0 {
			missing = append(missing, ignoreGroup{comment: g.comment, patterns: absent})
		}
	}
	if len(missing) == 0 {
		return existing
	}
	block := renderIgnoreGroups(missing)
	switch {
	case existing == "":
		return block
	case strings.HasSuffix(existing, "\n"):
		return existing + "\n" + block
	default:
		return existing + "\n\n" + block
	}
}

// gitignoreCovers recognizes an existing anchored or unanchored twin, or a plain
// parent directory line. In particular, the legacy artifacts/ line already
// covers /artifacts/tiller/, so re-joining never appends a redundant entry.
// This is not a full gitignore evaluator: comments and negations do not count,
// and a glob line covers only its own anchored or unanchored twin.
func gitignoreCovers(content, pattern string) bool {
	want := strings.TrimPrefix(pattern, "/")
	for raw := range strings.SplitSeq(content, "\n") {
		have := strings.TrimPrefix(strings.TrimSpace(raw), "/")
		if have == "" || strings.HasPrefix(have, "#") || strings.HasPrefix(have, "!") {
			continue
		}
		if have == want {
			return true
		}
		if strings.ContainsAny(have, "*?[") {
			continue
		}
		if strings.HasSuffix(have, "/") && strings.HasPrefix(want, have) {
			return true
		}
	}
	return false
}
