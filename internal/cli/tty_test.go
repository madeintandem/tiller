package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// forceTTY pins the detection for a test: the suite runs piped, so rendering
// only ever happens when a test asks for it, and the whole existing suite is
// itself the proof that piped output is byte-identical to before.
func forceTTY(t *testing.T, on bool, noColor bool) {
	t.Helper()
	prevDetect := ttyDetect
	ttyDetect = func() bool { return on }
	t.Cleanup(func() { ttyDetect = prevDetect })
	if noColor {
		t.Setenv("NO_COLOR", "1")
	} else {
		t.Setenv("NO_COLOR", "")
	}
}

func TestRenderHumanTTYBoldsTheMarkersAndStripsThem(t *testing.T) {
	forceTTY(t, true, false)
	got := renderHumanTTY("**Approval recorded**\n\n- Moved to ready: 1 of 1")
	require.NotContains(t, got, "**", "markers render, never print")
	require.Contains(t, got, "\x1b[1mApproval recorded\x1b[0m")
	require.Contains(t, got, "- Moved to ready: 1 of 1")
}

func TestRenderHumanTTYPaintsNeedsAttentionHeadingInTheLoudInk(t *testing.T) {
	// There is one loud heading for the whole list. Count bullets remain plain;
	// the old per-kind count headers are no longer recognized.
	forceTTY(t, true, false)
	got := renderHumanTTY("**Needs attention**\n\n- 1 epic awaits your approval · `tiller queue --kind approve_epic`\n\n**Next:** specify.")
	require.Contains(t, got, "\x1b[35;1mNeeds attention\x1b[0m")
	require.Contains(t, got, "- 1 epic awaits your approval · tiller queue --kind approve_epic")
	require.NotContains(t, got, "\x1b[35;1m1 epic", "only the singular umbrella heading gets loud ink")
	require.Contains(t, got, "\x1b[1mNext:\x1b[0m specify.")
	require.NotContains(t, got, "**")
}

func TestRenderHumanTTYKeepsOrdinaryBoldOutOfTheLoudInk(t *testing.T) {
	// Near matches and ordinary labels stay bold at most; recognition is the
	// exact singular "**Needs attention**" heading.
	forceTTY(t, true, false)
	got := renderHumanTTY("**Needs attentions**\n\n**Verification**\n\n**waiting on your approval since Tuesday**")
	require.Contains(t, got, "\x1b[1mNeeds attentions\x1b[0m")
	require.Contains(t, got, "\x1b[1mVerification\x1b[0m")
	require.Contains(t, got, "\x1b[1mwaiting on your approval since Tuesday\x1b[0m")
	require.NotContains(t, got, "\x1b[35;1m")
}

func TestRenderHumanTTYIndentsFencesAndDropsTheirMarkers(t *testing.T) {
	forceTTY(t, true, false)
	got := renderHumanTTY("**Commands**\n\n```bash\ntiller sync\n```\ntail")
	require.NotContains(t, got, "```")
	require.Contains(t, got, "  tiller sync")
	require.Contains(t, got, "tail")
}

func TestRenderHumanTTYStripsInlineBackticksOutsideFences(t *testing.T) {
	forceTTY(t, true, false)
	got := renderHumanTTY("committed at `f70abe3`\n\n```bash\necho `kept`\n```")
	require.Contains(t, got, "committed at f70abe3")
	require.Contains(t, got, "echo `kept`", "fence content is verbatim command text")
}

func TestRenderHumanTTYHonorsNoColorButKeepsTheStructure(t *testing.T) {
	// NO_COLOR suppresses escape codes; it does not un-render the page. The
	// human still gets clean text rather than raw markers.
	forceTTY(t, true, true)
	got := renderHumanTTY("**Needs attention**\n\n- 1 task review gate awaits your attestation · `tiller queue --kind attest_gate`")
	require.NotContains(t, got, "\x1b[")
	require.NotContains(t, got, "**")
	require.NotContains(t, got, "```")
	require.Contains(t, got, "Needs attention")
	require.Contains(t, got, "- 1 task review gate awaits your attestation · tiller queue --kind attest_gate")
}

func TestRenderHumanTTYIsPassthroughWhenPiped(t *testing.T) {
	forceTTY(t, false, false)
	src := "**Needs attention**\n\n- 1 epic awaits your approval · `tiller queue --kind approve_epic`"
	require.Equal(t, src, renderHumanTTY(src),
		"piped output is the wire format agents reproduce; it never renders")
}

func TestRenderHumanTTYLeavesPlainTextAlone(t *testing.T) {
	forceTTY(t, true, false)
	src := "epic e (E-1) is now ready."
	require.Equal(t, src, renderHumanTTY(src))
	require.False(t, strings.Contains(renderHumanTTY(src), "\x1b["))
}
