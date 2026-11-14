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

var _ = gen.Orientation
