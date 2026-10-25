package gen

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// acceptVerb matches the retired verb in every inflection a sentence could carry
// it in, and no other word: "acceptance" ends past the boundary and is the one
// legal use of the stem in the block.
var acceptVerb = regexp.MustCompile(`\baccept(s|ed|ing)?\b`)

// TestPointerBlockIsFencedAndComplete pins the managed AGENTS.md region: it opens
// and closes with the exact markers tiller join looks for, names Tiller, carries
// the one hard rule inline in the delegation-model formulation (drive to the edge
// and stop; the gate is the human's decision, not the human's keyboard; approve
// only on an explicit instruction naming the nodes; attesting a human gate and
// crossing work to done never, even on instruction; the merge is the human's),
// points at orientation.md, and tells the agent to run tiller status before each
// session. House style: no em dashes anywhere in the generated prose.
//
// The accept-verb assertion is the load-bearing half after the single-tree
// refactor: `accept` is not a verb this build has, so a prohibition naming it
// would teach every project's AGENTS.md a command that does not exist. It is a
// word-boundary regexp rather than a substring check because the block legally
// says "acceptance gates" one line above, and a bare NotContains on "accept"
// fails on that noun while missing nothing the verb would add.
func TestPointerBlockIsFencedAndComplete(t *testing.T) {
	block := PointerBlock()

	require.True(t, strings.HasPrefix(block, PointerBegin), "block opens with the begin marker")
	require.True(t, strings.HasSuffix(block, PointerEnd), "block closes with the end marker")

	require.Contains(t, block, "Tiller")
	require.Contains(t, block, ".tiller/")
	require.Contains(t, block, "drive work to the edge of done and stop")
	require.Contains(t, block, "not the human's keyboard", "states decision versus keyboard")
	require.Contains(t, block, "explicit human instruction naming the nodes", "approve is delegable only on an explicit instruction")
	require.Contains(t, block, "never attest a human gate")
	require.Contains(t, block, "never cross work to done")
	require.Contains(t, block, "even on instruction", "terminal crossings stay human-typed even on instruction")
	require.Contains(t, block, "commits directly, with no pull request", "a .tiller/-only diff skips the PR ceremony")
	require.Contains(t, block, ".tiller/orientation.md")
	require.Contains(t, block, "tiller status")

	require.NotRegexp(t, acceptVerb, block, "the block names no verb this build lacks; accept left with the computed rollup")
	require.NotContains(t, block, "never approve a plan", "the blanket never-approve is retired at the pointer-block layer too (delegation model)")
	require.NotContains(t, block, "—", "no em dashes in the pointer block")
}

// TestUpsertPointerBlockCreatesWhenAbsent confirms an empty target yields just the
// block plus a trailing newline, the file tiller join writes when AGENTS.md does
// not exist yet.
func TestUpsertPointerBlockCreatesWhenAbsent(t *testing.T) {
	got, err := UpsertPointerBlock("")
	require.NoError(t, err)
	require.Equal(t, PointerBlock()+"\n", got)
}

// TestUpsertPointerBlockAppendsAndPreservesHumanContent confirms that with prior
// human content and no managed region, the block is appended after a blank line
// and the human bytes are preserved untouched.
func TestUpsertPointerBlockAppendsAndPreservesHumanContent(t *testing.T) {
	human := "# My Project\n\nHand-written notes.\n"
	got, err := UpsertPointerBlock(human)
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(got, human), "human content is preserved at the head")
	require.Contains(t, got, PointerBegin)
	require.Contains(t, got, PointerEnd)
	require.Contains(t, got, "Hand-written notes.")
}

// TestUpsertPointerBlockReplacesRegionInPlace confirms an existing managed region
// is replaced with the canonical block while content before and after it is left
// byte-for-byte untouched.
func TestUpsertPointerBlockReplacesRegionInPlace(t *testing.T) {
	header := "# My Project\n\nIntro paragraph.\n\n"
	stale := PointerBegin + "\n## Tiller\nSTALE BODY THAT MUST BE OVERWRITTEN\n" + PointerEnd
	footer := "\n\n## My own section\n\nKeep me.\n"
	existing := header + stale + footer

	got, err := UpsertPointerBlock(existing)
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(got, header), "header preserved")
	require.True(t, strings.HasSuffix(got, footer), "footer preserved")
	require.Contains(t, got, "drive work to the edge of done and stop", "canonical body restored")
	require.NotContains(t, got, "STALE BODY THAT MUST BE OVERWRITTEN", "stale region overwritten")
	require.Equal(t, 1, strings.Count(got, PointerBegin), "exactly one managed region")
}

// TestUpsertPointerBlockIsIdempotent confirms re-running the upsert over its own
// output is a no-op: the bytes are stable, so re-running join never churns the
// committed AGENTS.md.
func TestUpsertPointerBlockIsIdempotent(t *testing.T) {
	cases := []string{
		"",
		"# Project\n\nNotes.\n",
		"# Project\n\nNotes.",     // no trailing newline
		"# Project\n\nNotes.\n\n", // doubled trailing newline
		PointerBlock() + "\n",     // already just the block
	}
	for _, in := range cases {
		once, err := UpsertPointerBlock(in)
		require.NoError(t, err)
		twice, err := UpsertPointerBlock(once)
		require.NoError(t, err)
		require.Equal(t, once, twice, "upsert is idempotent for %q", in)
	}
}

// TestRedirectBlockIsFencedAndThin pins the managed CLAUDE.md region: the same
// begin/end markers as the AGENTS.md block around a one-liner that redirects to
// AGENTS.md. It is a redirect, not a copy: none of the pointer block's own prose
// (the hard rule, the orientation pointer, the status nudge) may appear, so the
// pointer block has exactly one home. House style: no em dashes.
func TestRedirectBlockIsFencedAndThin(t *testing.T) {
	block := RedirectBlock()

	require.True(t, strings.HasPrefix(block, PointerBegin), "block opens with the begin marker")
	require.True(t, strings.HasSuffix(block, PointerEnd), "block closes with the end marker")

	require.Contains(t, block, "AGENTS.md", "the redirect points at AGENTS.md")

	body := strings.TrimSuffix(strings.TrimPrefix(block, PointerBegin), PointerEnd)
	require.Equal(t, 1, len(strings.Split(strings.TrimSpace(body), "\n")), "the body is a one-liner")

	require.NotContains(t, block, "drive work to the edge of done", "the redirect does not copy the hard rule")
	require.NotContains(t, block, "orientation.md", "the redirect does not copy the orientation pointer")
	require.NotContains(t, block, "tiller status", "the redirect does not copy the status nudge")
	require.NotContains(t, block, "—", "no em dashes in the redirect")
}

// TestUpsertRedirectBlockSharesTheMergeMachinery confirms the redirect upsert
// behaves exactly like the pointer upsert: create when empty, append after human
// content without touching it, replace a stale region in place, and stay
// byte-stable when fed its own output.
func TestUpsertRedirectBlockSharesTheMergeMachinery(t *testing.T) {
	empty, err := UpsertRedirectBlock("")
	require.NoError(t, err)
	require.Equal(t, RedirectBlock()+"\n", empty, "an empty target yields just the block")

	human := "# My Project\n\nHand-written Claude notes.\n"
	appended, err := UpsertRedirectBlock(human)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(appended, human), "human content is preserved at the head")
	require.Contains(t, appended, RedirectBlock())

	header := "# Notes\n\n"
	stale := PointerBegin + "\nSTALE REDIRECT\n" + PointerEnd
	footer := "\n\n## Mine\n\nKeep me.\n"
	replaced, err := UpsertRedirectBlock(header + stale + footer)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(replaced, header), "header preserved")
	require.True(t, strings.HasSuffix(replaced, footer), "footer preserved")
	require.NotContains(t, replaced, "STALE REDIRECT", "stale region overwritten")
	require.Equal(t, 1, strings.Count(replaced, PointerBegin), "exactly one managed region")

	for _, in := range []string{"", human, header + stale + footer} {
		once, err := UpsertRedirectBlock(in)
		require.NoError(t, err)
		twice, err := UpsertRedirectBlock(once)
		require.NoError(t, err)
		require.Equal(t, once, twice, "upsert is idempotent for %q", in)
	}
}

// TestSkillShimDelegatesToGuide is the shim contract: a thin Claude Code skill
// with frontmatter carrying the tiller-prefixed name and the trigger description,
// whose body delegates to the project-resolved binary via tiller guide <skill>
// rather than inlining the skill body.
func TestSkillShimDelegatesToGuide(t *testing.T) {
	desc := "Use when you enter a Tiller repo, or when you finish a step and need to choose the next move."
	shim := SkillShim("orient", desc)

	require.True(t, strings.HasPrefix(shim, "---\n"), "shim opens with YAML frontmatter")
	require.Contains(t, shim, "name: tiller-orient")
	require.Contains(t, shim, "description: "+desc)
	require.Contains(t, shim, "tiller guide orient", "the body delegates to tiller guide")
	require.NotContains(t, shim, "—", "no em dashes in the shim")

	// The frontmatter is a closed block: two --- fences before the body.
	require.GreaterOrEqual(t, strings.Count(shim, "\n---\n")+strings.Count(shim, "---\n"), 2)

	require.Equal(t, "tiller-orient", SkillShimName("orient"))
	require.Equal(t, "tiller-decompose", SkillShimName("decompose"))
}

func TestManagedRegionRejectsAmbiguousBoundaries(t *testing.T) {
	for name, content := range map[string]string{
		"missing end":    PointerBegin + "\nhuman text\n",
		"missing begin":  "human text\n" + PointerEnd,
		"backwards":      PointerEnd + "\nhuman text\n" + PointerBegin,
		"duplicate":      PointerBlock() + "\n" + PointerBlock(),
		"nested":         PointerBegin + "\n" + PointerBlock() + "\n" + PointerEnd,
		"damaged marker": "<!-- tiller:begin -->\nhuman text\n" + PointerEnd,
		"inline marker":  "human text " + PointerBlock(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, upsert := range []func(string) (string, error){UpsertPointerBlock, UpsertRedirectBlock} {
				rendered, err := upsert(content)
				require.Error(t, err)
				require.Empty(t, rendered, "refusal must not offer truncated human content")
			}
		})
	}
}

func TestSkillShimOwnershipRequiresPositiveEvidence(t *testing.T) {
	modern := SkillShim("retired", "Use for retired work.")
	legacy := "---\nname: tiller-retired\ndescription: Use for retired work.\n---\n\n" +
		"# tiller-retired\n\n" +
		"This is a thin shim installed by `tiller join`. The full, version-correct skill\n" +
		"lives in the Tiller binary resolved for this project, not in this file, so it never\n" +
		"drifts from the installed version.\n\n" +
		"When this skill fits the moment, run:\n\n```\ntiller guide retired\n```\n\n" +
		"Then follow exactly what it prints. The printed guidance is the source of truth; do\n" +
		"not act from this file.\n"
	require.True(t, OwnsSkillShim("tiller-retired", modern))
	require.True(t, OwnsSkillShim("tiller-retired", legacy))
	require.False(t, OwnsSkillShim("tiller-other", modern), "the marker cannot authorize another skill")
	require.False(t, OwnsSkillShim("tiller-retired", legacy+"\nHuman instructions.\n"))
	require.False(t, OwnsSkillShim("tiller-retired", strings.Replace(legacy, "not act", "act", 1)))
	require.False(t, OwnsSkillShim("tiller-retired", "---\nname: tiller-retired\n---\nCustom skill\n"))
}
