package gen

import (
	"fmt"
	"strings"
)

// This file holds the pure content `tiller join` writes when it wires a machine's
// harness to a Tiller project (distribution.md, "Harness integration"): the
// managed AGENTS.md pointer block, the managed CLAUDE.md redirect to it, and the
// thin Claude Code skill shims. All are pure functions of their inputs with no
// cobra and no stdout, so the same call always yields the same bytes and
// re-running join is a no-op diff. The CLI does the file IO; this package only
// produces the text and the in-place merge.

// PointerBegin and PointerEnd delimit the managed region tiller join owns inside
// the harness instruction file. Everything between them is Tiller's and is
// regenerated; everything around them is the human's and is never touched
// (distribution.md, "The pointer block"). The begin marker carries the managed
// notice so a human reading the file knows edits inside are overwritten.
const (
	PointerBegin = "<!-- tiller:begin (managed by `tiller join`; edits inside are overwritten) -->"
	PointerEnd   = "<!-- tiller:end -->"
)

// PointerBlock returns the managed AGENTS.md region: the begin marker, the thin
// pointer prose, and the end marker, with no trailing newline so the markers are
// exact. It is deliberately thin, carrying only what is true across every version
// (the one hard rule inline, the orientation pointer, the per-session status
// nudge), so the committed region is stable and re-running join does not churn
// it. The hard rule stays inline because it is the one thing that must never be
// missed; the detail is delegated to orientation.md, which the agent reads on
// demand. House style: plain, concrete, no em dashes.
//
// The hard rule names only verbs this build has. It used to forbid "accepting"
// completed work, from the era when `accept` was a verb; the single-tree
// refactor made feature and epic done a computed rollup that `sync` records off
// the human's merge, so the old wording sent every project's AGENTS.md a
// prohibition on a command that does not exist. distribution.md holds that a
// pointer-block wording change is a schema-era decision rather than a
// patch-release convenience, because otherwise two developers on different
// patches churn the same committed region; this change is taken in exactly such
// an era, the one where schema_version moved to 2 and the orientation generation
// moved to 8, so the region settles once and stays settled.
//
// The PRs-are-for-code line was added pre-1.0 with no era move and no generation
// bump (JC's call, 2026-07-31): every workspace already rides the v2 era, so the
// region settles once more and the flip-flop the era rule guards against cannot
// arise between two pre-1.0 binaries that both carry the line.
func PointerBlock() string {
	return strings.Join([]string{
		PointerBegin,
		"## Tiller",
		"",
		"This project uses Tiller to run its SDLC workflow. Specs, decomposition, planning, and acceptance gates live in `.tiller/`, under git, beside the code they describe.",
		"",
		"One hard rule: drive work to the edge of done and stop. You may propose, build, attach evidence, run gates, and open PRs. Authorized execution includes creating task branches and merging them into the recorded feature or epic aggregate, never into the configured integration branch. A gate is the human's decision, not the human's keyboard: run `approve` only on an explicit human instruction naming the nodes. You never attest a human gate and never cross work to done, even on instruction; the final integration merge is the human's, and Tiller records the crossing from it.",
		"",
		"PRs are for code: a change that touches only `.tiller/` commits directly, with no pull request.",
		"",
		"Read `.tiller/orientation.md` for the full rules and how to work. Run `tiller status` before each session to see where things stand.",
		PointerEnd,
	}, "\n")
}

// RedirectBlock returns the managed CLAUDE.md region: the same begin and end
// markers as the AGENTS.md block around a one-liner that redirects to AGENTS.md.
// It exists because Claude Code auto-loads CLAUDE.md but not AGENTS.md, so
// without it the pointer block never enters the session's context. It is a
// redirect, not a copy: AGENTS.md stays the pointer block's only home, so the
// two files can never drift apart. House style: plain, concrete, no em dashes.
func RedirectBlock() string {
	return strings.Join([]string{
		PointerBegin,
		"Project instructions and the Tiller pointer block live in [AGENTS.md](./AGENTS.md). Read that.",
		PointerEnd,
	}, "\n")
}

// UpsertPointerBlock returns the harness instruction file's content with the
// managed pointer block present exactly once, via the shared managed-region
// merge.
func UpsertPointerBlock(existing string) (string, error) {
	return upsertManagedRegion(existing, PointerBlock())
}

// UpsertRedirectBlock returns CLAUDE.md's content with the managed redirect
// region present exactly once, via the shared managed-region merge.
func UpsertRedirectBlock(existing string) (string, error) {
	return upsertManagedRegion(existing, RedirectBlock())
}

// upsertManagedRegion preserves all bytes outside one well-formed region.
// Ambiguous or incomplete markers refuse instead of guessing where human text ends.
func upsertManagedRegion(existing, block string) (string, error) {
	begins := strings.Count(existing, "<!-- tiller:begin")
	ends := strings.Count(existing, "<!-- tiller:end")
	if begins != 0 || ends != 0 {
		begin := strings.Index(existing, PointerBegin)
		end := strings.Index(existing, PointerEnd)
		if begins != 1 || ends != 1 || begin < 0 || end < begin ||
			!markerOnOwnLine(existing, begin, len(PointerBegin)) ||
			!markerOnOwnLine(existing, end, len(PointerEnd)) {
			return "", fmt.Errorf("malformed Tiller managed region: expected one complete, ordered begin/end marker pair on separate lines; repair the markers while preserving human text")
		}
		return existing[:begin] + block + existing[end+len(PointerEnd):], nil
	}

	if existing == "" {
		return block + "\n", nil
	}

	// No region present: append after the existing content, preceded by exactly
	// one blank line. The existing bytes are never modified.
	sep := "\n\n"
	switch {
	case strings.HasSuffix(existing, "\n\n"):
		sep = ""
	case strings.HasSuffix(existing, "\n"):
		sep = "\n"
	}
	return existing + sep + block + "\n", nil
}

func markerOnOwnLine(content string, start, length int) bool {
	if start > 0 && content[start-1] != '\n' {
		return false
	}
	end := start + length
	return end == len(content) || content[end] == '\n' || strings.HasPrefix(content[end:], "\r\n")
}

// SkillShimOwner is a stable ownership marker, not a generation stamp. A
// tiller-prefixed directory alone never authorizes deleting its SKILL.md.
const SkillShimOwner = "<!-- tiller:generated-skill-shim -->"

// OwnsSkillShim recognizes explicitly marked shims and exact legacy generated
// shims. Legacy files with user edits are deliberately ambiguous and preserved.
func OwnsSkillShim(shimName, content string) bool {
	skill, ok := strings.CutPrefix(shimName, "tiller-")
	if !ok || skill == "" {
		return false
	}
	rest, ok := strings.CutPrefix(content, "---\nname: "+shimName+"\ndescription: ")
	if !ok {
		return false
	}
	description, body, ok := strings.Cut(rest, "\n---\n")
	if !ok || description == "" || strings.Contains(description, "\n") {
		return false
	}
	if strings.Contains(body, "\n"+SkillShimOwner+"\n") {
		return true
	}
	legacy := "\n# " + shimName + "\n\n" +
		"This is a thin shim installed by `tiller join`. The full, version-correct skill\n" +
		"lives in the Tiller binary resolved for this project, not in this file, so it never\n" +
		"drifts from the installed version.\n\n" +
		"When this skill fits the moment, run:\n\n```\ntiller guide " + skill + "\n```\n\n" +
		"Then follow exactly what it prints. The printed guidance is the source of truth; do\n" +
		"not act from this file.\n"
	return body == legacy
}

// SkillShimName is the discoverable Claude Code skill name for a Tiller skill: the
// skill name with a tiller- prefix, so the harness shows a clear Tiller
// association (tiller-orient, tiller-decompose) and the shims share a glob that
// .gitignore can exclude.
func SkillShimName(skill string) string {
	return "tiller-" + skill
}

// SkillShim returns the SKILL.md text for a thin Claude Code shim that delegates
// to the binary. It carries frontmatter (the tiller-prefixed name and the skill's
// trigger description, so the harness can auto-activate it) and a short body that
// tells the agent to run `tiller guide <skill>` and follow exactly what it
// prints. It deliberately does not inline the skill body: the project-resolved
// binary holds the version-correct text, so the shim cannot drift from it. House
// style: plain, concrete, no em dashes.
func SkillShim(skill, description string) string {
	shimName := SkillShimName(skill)
	return strings.Join([]string{
		"---",
		"name: " + shimName,
		"description: " + description,
		"---",
		"",
		SkillShimOwner,
		"",
		"# " + shimName,
		"",
		"This is a thin shim generated by Tiller. The full, version-correct skill",
		"lives in the Tiller binary resolved for this project, not in this file, so it never",
		"drifts from the installed version.",
		"",
		"When this skill fits the moment, run:",
		"",
		"```",
		"tiller guide " + skill,
		"```",
		"",
		"Then follow exactly what it prints. The printed guidance is the source of truth; do",
		"not act from this file.",
		"",
	}, "\n")
}
