package gen

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOrientationStampGenerationRoundTrips confirms the stamp parser reads
// back exactly the generation Orientation wrote.
func TestOrientationStampGenerationRoundTrips(t *testing.T) {
	got, ok := OrientationStampGeneration([]byte(Orientation()))
	require.True(t, ok, "the emitted file carries a recognizable stamp")
	require.Equal(t, orientationSchemaGen, got)
}

// TestOrientationStampGenerationReadsAnAgedStamp confirms the parser reads a
// stamp for a generation other than the current one, the aged-file case
// `tiller check readiness` compares against.
func TestOrientationStampGenerationReadsAnAgedStamp(t *testing.T) {
	aged := strings.Replace(Orientation(),
		"orientation generation "+strconv.Itoa(orientationSchemaGen),
		"orientation generation 1", 1)
	got, ok := OrientationStampGeneration([]byte(aged))
	require.True(t, ok)
	require.Equal(t, 1, got)
}

// TestOrientationStampGenerationRejectsUnrecognizedInput confirms the parser
// reports ok=false, not a guessed generation, for input carrying no Tiller
// stamp, the missing-or-foreign orientation.md case readiness must not
// misread as a mismatch.
func TestOrientationStampGenerationRejectsUnrecognizedInput(t *testing.T) {
	_, ok := OrientationStampGeneration([]byte("# Not a Tiller file\n"))
	require.False(t, ok)
}

// TestOrientationSeamCatalogNeedsToBeInputs keeps the skill catalog agreeing
// with the route rule: as-is docs alone are inputs, but not ones decompose acts on.
func TestOrientationSeamCatalogNeedsToBeInputs(t *testing.T) {
	text := Orientation()
	require.NotContains(t, text, "`decompose`: inputs exist but the tree does not.")
	require.Contains(t, text, "`decompose`: to-be inputs exist but the tree does not reflect them.")
}

// Explicit code review requests reach their skill outside the ranked list,
// and the review's file write is named as the one exception to state recording.
func TestOrientationRoutesExplicitCodeReviewsOutsideTheWalk(t *testing.T) {
	text := Orientation()
	for _, want := range []string{
		"`tiller guide code-review`",
		"`tiller guide` serves ten skills:",
		"- `code-review`: the developer explicitly asks",
		"`.tiller/reviews/`",
		"Do not invent a `tiller readout code-review` command.",
		"During `design-brief` or a `code-review` review, recover only through corrected reads.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// The next move is a ranked list of every legal move, not a first-match walk:
// the five tiers, the fourteen-day recency window, and the relay rule are
// stated, and the walk's wording is gone.
func TestOrientationDescribesRankedNext(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ") // the file hard-wraps prose
	for _, want := range []string{
		"### Ranked next",
		"`tiller next`",
		"Unblock nearby comes first: human decisions that hold work in your current feature, plus any question overdue or due within two days wherever it sits.",
		"Continue is your own unfinished work, and your top Continue move is always kept in the five.",
		"Nearby is the other moves in the current feature, then in its epic.",
		"Recently touched is the features and epics you acted on within the last fourteen days, most recent day first.",
		"Adding work only touches its parent for recency, an added task its feature and an added feature its epic, so a decomposition never sets it.",
		"Everything else follows in plan order.",
		"the feature touched by your most recent deliberate act that names one",
		"Acts on an epic alone and amendment steps never set it",
		"`tiller next` and bare `tiller guide` head the list as not personalized (the readout's `**Next:**` has no heading)",
		"a question overdue or due within two days still ranks first",
		"Inside a tier, moves sort by kind in a fixed order first",
		"Within a kind, plan order decides",
		"the exact command, or the branch to continue on",
		"A ranked move is a suggestion, not authorization.",
		"Start the top agent move in your next turn only when the developer's reply says to: a go, a yes, or an instruction naming that row.",
		"Work already moving inside a seam keeps moving",
		"never rewrite the list",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"first match wins",
		"once the developer's reply does not redirect",
		"### Recorded-state route",
		"`Continuity note`",
		"`Order note`",
		"the route suggests waiting",
	} {
		require.NotContainsf(t, text, gone, "orientation no longer carries %q", gone)
	}
}

// Each unit of work, scoped by review grain, runs in its own worktree by
// default, parallel tasks within a unit get their own, the developer's
// checkout instructions win, and other agents may share the repository.
func TestOrientationExecutesInAWorktreeByDefault(t *testing.T) {
	text := Orientation()
	for _, want := range []string{
		"Execute each unit of work in its own Git worktree by default",
		"A unit of work is what one\n  review covers, so review grain sets it",
		"the whole\n  feature at feature grain, the whole epic at epic grain",
		"the unit's one\n  worktree holds each riding task branch and the aggregate in turn",
		"Tasks built in parallel within a unit,\n  by subagents or otherwise, each get a temporary worktree of their own",
		"instructions about checkouts and worktrees take precedence over this default",
		"so a new worktree has no identity",
		"tracked file join's guidance refresh changed rather than committing it",
		"the recorded branch, or add and join one for it, and run `resume` there",
		"Integration-branch closeout runs where that branch is checked out",
		"other agents and developers may be working this repository at the same",
		"not create for this work is someone else's",
		"Another agent's claim lands on its task",
		"than choosing a winner",
		"work is committed and, when remote-backed, pushed",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// A done feature's eval bindings are corrected through the amendment's
// correct-eval operation, never by rewriting its criteria.
func TestOrientationCorrectsDoneEvalBindingsThroughAmend(t *testing.T) {
	text := Orientation()
	for _, want := range []string{
		"prerequisites, even under an active parent. A done feature's eval bindings,",
		"never its criteria statements, are corrected through `amend --correct-eval`.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// Generation 59 recorded bug severity: capture grades, approve bug requires a
// grade, any contributor regrades with grade, and severity never routes.
func TestOrientationGeneration59RecordsBugSeverity(t *testing.T) {
	text := Orientation()
	for _, want := range []string{
		"`tiller add bug <key> --severity <level>`",
		"`tiller approve bug` refuses an unrated bug",
		"`tiller grade bug <key> <level>`",
		"Severity never routes",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// Generation 61 keeps the one directory exception to exact-path staging: the
// paths sync, compact, migrate, or upgrade print for compacted events.
func TestOrientationNamesTheCompactionStagingException(t *testing.T) {
	require.Equal(t, 61, OrientationGeneration())
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "The one exception is compacted events: when `tiller sync`, `tiller compact`, `tiller migrate`, or `tiller upgrade` names paths to stage, stage exactly those, directories included.")
}

// Generation 61 retires the intake skill: decompose carries both routes for a
// document, and the orientation text says so without naming an intake skill.
func TestOrientationGeneration61(t *testing.T) {
	got, ok := OrientationStampGeneration([]byte(Orientation()))
	require.True(t, ok)
	require.Equal(t, 61, got)
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.NotContains(t, text, "- `intake`:", "intake is a command, not a skill")
	require.NotContains(t, text, "Use intake when")
	require.NotContains(t, text, "capture and intake shapes")
	require.Contains(t, text, "`tiller intake`")
	require.Contains(t, text, "draft amendment")
	require.Contains(t, text, "`tiller propose amendment`")
}

func TestOrientationBoundsRetries(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "after two different fixes")
}

// TestOrientationSeparatesCancelWorkFromCancelQuestion pins that only cancelling a
// work node is human-only, while the question-side decisions may be recorded on
// an explicit instruction.
func TestOrientationSeparatesCancelWorkFromCancelQuestion(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "`cancel` on a work node is human-only")
	require.Contains(t, text, "`cancel question`, `unlink question`, and `withdraw amendment`")
}

// TestQuestionsAreRecordedNotChatted pins the question rule: a product decision is
// recorded, a clarification of the request itself is asked in chat.
func TestQuestionsAreRecordedNotChatted(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "A clarification of the request itself")
	require.Contains(t, text, "recorded with `tiller add question`")
}

// TestOrientationNeverSwitchesACheckoutForPlanningState pins the one rule for
// where .tiller/-only planning commits land when this checkout is on another
// branch: the checkout that holds the integration branch, never a branch switch.
func TestOrientationNeverSwitchesACheckoutForPlanningState(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	for _, want := range []string{
		"Planning state commits on the integration branch, in the checkout that holds it.",
		"Never switch a checkout's branch to get there.",
		"use the worktree that has the integration branch checked out (`git worktree list`), or add one and join inside it",
		"Commit there only when it holds no changes you did not make; otherwise stop and name what is in the way.",
	} {
		require.Contains(t, text, want)
	}
}

// TestOrientationStatesCurrentBaselineDriftAndShelfClaims pins that orientation
// no longer denies baseline drift markers, names all three shelf claim verbs,
// and speaks of contributors rather than owners in the manifest entry.
func TestOrientationStatesCurrentBaselineDriftAndShelfClaims(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.NotContains(t, text, "No automatic baseline drift")
	require.NotContains(t, text, "the owners, the inputs")
	for _, want := range []string{
		"`tiller claim bug <key>`",
		"`tiller claim chore <key>`",
		"`tiller claim tweak <key>`",
		"the contributors, the inputs",
	} {
		require.Contains(t, text, want)
	}
}

// TestOrientationDrawsTheChoreTweakLine pins that orientation says a change to
// code or tests, however small, is a tweak.
func TestOrientationDrawsTheChoreTweakLine(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "Tweaks are minor adjustments that may change code. A change to code or tests, however small, is a tweak.")
}
