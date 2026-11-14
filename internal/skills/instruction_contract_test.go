package skills

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/stretchr/testify/require"
)

// These tests check that an instruction does not contradict the code or the
// orientation rule it sits under. Each one names the behavior it rests on and
// the test that pins that behavior, and asserts the instruction's route, not
// its wording beyond the route.

// skillSection returns one "## heading" section of a skill, up to the next
// second-level heading.
func skillSection(t *testing.T, text, heading string) string {
	t.Helper()
	start := strings.Index(text, "\n## "+heading+"\n")
	require.NotEqualf(t, -1, start, "the skill has no %q section", heading)
	rest := text[start+1:]
	if end := strings.Index(rest[1:], "\n## "); end != -1 {
		return rest[:end+1]
	}
	return rest
}

// clausesNaming returns the clauses of text that contain phrase, splitting
// sentences at their semicolons. Skill prose keeps one rule to a clause, so a
// contradiction check can ask what the clause about a thing tells the agent to
// do with it.
func clausesNaming(text, phrase string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, sentence := range strings.SplitAfter(line, ". ") {
			for _, clause := range strings.SplitAfter(sentence, "; ") {
				if strings.Contains(clause, phrase) {
					out = append(out, clause)
				}
			}
		}
	}
	return out
}

// A landed intake document is provenance. Every amendment minted from it pins
// its fingerprint, and approval refuses source_changed or source_vanished once
// the artifact is replaced or gone (mutate's amendment approve tests pin both).
// So a name collision is recovered by giving the incoming document a fresh
// name, and no instruction removes the landed artifact to make room.
func TestDecomposeCollisionRecoveryKeepsTheLandedSource(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)

	recovery := skillSection(t, text, "Recovery")
	collision := clausesNaming(recovery, "`artifact_exists`")
	require.Len(t, collision, 1, "Recovery clears the name collision in one clause")
	require.Contains(t, collision[0], "giving the incoming document a fresh name")
	require.Contains(t, collision[0], "rerunning `tiller intake`")
	require.Contains(t, recovery, "`source_changed`")
	require.Contains(t, recovery, "`source_vanished`")

	// The CLI's own refusal is quoted so the agent recognizes it, and it still
	// offers removal. Outside that quote, no clause about a landed or old
	// artifact tells the agent to remove it.
	const cliMessage = "re-intake is a new intake: rename the document or remove the old artifact"
	for _, phrase := range []string{"old artifact", "landed artifact"} {
		for _, clause := range clausesNaming(strings.ReplaceAll(text, cliMessage, ""), phrase) {
			for _, removal := range []string{"remov", "delet"} {
				require.NotContainsf(t, clause, removal,
					"decompose never clears a collision by removing a landed source: %q", clause)
			}
		}
	}

	// code-review lands its distillation through the same door and already
	// takes a fresh suffix.
	review, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, review, "means choosing the next suffix for the distillation")
}

// orientationCorrectionRow returns the orientation correction table row whose
// first cell is label.
func orientationCorrectionRow(t *testing.T, label string) string {
	t.Helper()
	for _, line := range strings.Split(gen.Orientation(), "\n") {
		if strings.HasPrefix(line, "| "+label+" |") {
			return line
		}
	}
	t.Fatalf("orientation's correction table has no %q row", label)
	return ""
}

// A done or cancelled feature's prose is accepted history. Orientation's
// correction table routes its correction into forward work, and drift compares
// a done feature's feature.md with the bytes verified at the done crossing, so
// another edit never clears a spec-leg stale_spec candidate (package drift's
// stale_spec tests pin that comparison). mend-drift therefore drafts no edit to
// terminal prose: it routes through that row and proposes restoring the
// verified file. The eval-binding correction, surface repair, non-terminal
// basis refresh, and named-doc design confirmation keep their own routes.
func TestMendDriftRoutesTerminalProseThroughTheCorrectionTable(t *testing.T) {
	row := orientationCorrectionRow(t, "Done or cancelled feature or task historical prose")
	require.Contains(t, row, "Do not edit the accepted Markdown in place")
	require.Contains(t, row, "forward additive or replacement work")

	text, ok := Lookup("mend-drift")
	require.True(t, ok)
	reasoning := skillSection(t, text, "Reasoning")
	choreography := skillSection(t, text, "Choreography")

	// The one place the skill offers a prose edit scopes it to live work.
	require.Contains(t, reasoning, "a doc edit to the `feature.md` or `task.md` of work that is not done or cancelled")
	require.NotContains(t, reasoning, "a doc edit to `feature.md` or `task.md`,")

	// The spec leg of a done feature follows the table, not another edit.
	specLeg := clausesNaming(reasoning, "On the spec leg")
	require.Len(t, specLeg, 1)
	require.Contains(t, specLeg[0], "the done or cancelled row of the **Correction table** in `.tiller/orientation.md`")
	require.Contains(t, reasoning, "into forward work as that row says")
	require.Contains(t, reasoning, "propose restoring the verified `feature.md`")
	require.Contains(t, choreography, "the done or cancelled row of the Correction table")
	for _, defect := range []string{
		"Draft that edit anyway",
		"the edit is still the right fix",
		"a forward-going correction to a done feature's `feature.md`",
	} {
		require.NotContainsf(t, text, defect, "mend-drift no longer drafts an edit to terminal prose: %q", defect)
	}

	// What the candidate honestly does afterwards.
	require.Contains(t, reasoning, "say plainly that the candidate persists")
	require.Contains(t, reasoning, "persists by design")

	// The routes that stay.
	for _, kept := range []string{
		// The reviewed eval-binding correction on a done feature.
		"edit only the bindings",
		"`tiller amend <A-id> --correct-eval <feature> --reason \"<why>\"`",
		// Surface and relationship repairs, which edit no accepted prose.
		"`tiller link --surface`",
		"`tiller unlink --surface`",
		"`tiller link design` and `tiller unlink design`",
		// Non-terminal basis refresh and assumption review.
		"tiller approve feature submit-certification/certification-form-loading --refresh-basis",
		"--reviewed-assumption Q-h2n7m5",
		// Named-doc design confirmation.
		"only explicit human instruction naming the docs delegates the keystroke",
		"tiller approve design 'as-is/workspaces.md'",
	} {
		require.Containsf(t, text, kept, "mend-drift keeps %q", kept)
	}
}
