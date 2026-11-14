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

// The next move is a ranked list of every legal move, not a first-match walk.
// Orientation says what the list is, how a row reads, and how to relay it, and
// points at the orient skill for how it is ranked; the walk's wording is gone.
func TestOrientationDescribesRankedNext(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ") // the file hard-wraps prose
	for _, want := range []string{
		"### Ranked next",
		"`tiller next`",
		"Bare `tiller guide` and the readout's `**Next:**` print the same list, and `submit` and a passing `check` append it as `**Up next:**`",
		"the exact command, or the branch to continue on",
		"A ranked move is a suggestion, not authorization.",
		"Start the top agent move in your next turn only when the developer's reply says to: a go, a yes, or an instruction naming that row.",
		"Work already moving inside a seam keeps moving",
		"never rewrite the list",
		"The list holds only legal moves, so every row can be acted on as printed.",
		"`queue: N bugs, M chores, K tweaks ready`",
		"`tiller guide orient` explains how the list is ranked and what each move leads to.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	// How the list is ranked and what each move leads to live in the orient
	// skill (TestOrientDescribesRankedNext in package skills); one phrase per
	// passage that moved there.
	for _, gone := range []string{
		"first match wins",
		"once the developer's reply does not redirect",
		"### Recorded-state route",
		"`Continuity note`",
		"`Order note`",
		"the route suggests waiting",
		"head the list as not personalized",
		"Unblock nearby comes first",
		"Work in flight on another contributor's branch",
		"The moves, and the seam each leads to:",
		"Brownfield or legacy-migration with no confirmed baseline doc",
		"`carrier_cancelled` means owned work has returned to its",
		"Shared closeout, once per reviewed unit",
		"A `draft` amendment is the agent's.",
		"A `proposed` amendment awaiting approval",
		"under an approved ready or active feature and an epic past decomposition",
		"An open question leaves hold in one of two ways",
		"An epic still in `draft` with live features awaiting its decomposition gate",
		"an epic-grain edge gates the epic's features exactly as a feature-grain edge gates the feature",
		"is a human merge wait rather than a seam",
		"Drafts exist but every one waits on unfinished dependencies",
		"this is the true caught-up terminal",
		"Inside a tier, moves sort by kind in a fixed order first",
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
	// The readout seams' shared notes rule moved here from the skills.
	require.Contains(t, text, "Commentary is yours to add freely")
	require.Contains(t, text, "a question for the human is a recorded question")
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

// normalizedOrientation is the orientation text with its hard wraps collapsed.
func normalizedOrientation() string {
	return strings.Join(strings.Fields(Orientation()), " ")
}

// Orientation defines the terms the skills use, in one section between what
// Tiller is and the rules.
func TestOrientationDefinesItsTerms(t *testing.T) {
	text := normalizedOrientation()
	terms := strings.Index(text, "## Terms")
	require.GreaterOrEqual(t, terms, 0)
	require.Greater(t, terms, strings.Index(text, "## What Tiller is"))
	require.Less(t, terms, strings.Index(text, "## The rules you must follow"))
	for _, term := range []string{
		"seam", "live", "shelf", "frontier", "review grain", "coarse", "unit of work",
		"run", "aggregate", "riding task", "review ref", "finish line", "closeout",
		"rollup", "accepted basis", "operative question", "held draft", "carrier", "carried",
	} {
		require.Containsf(t, text, "**"+term+"**", "orientation defines %q", term)
	}
	require.Contains(t, text, "- **run**: the work the developer's instruction named, and nothing else.")
}

// The external references writing rule lives in orientation, not in orient.
func TestOrientationCarriesTheExternalReferencesRule(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"**External references.** In authored prose, link the first mention of an external library, framework, service, standard, tool, or other reference",
		"Use a URL only when it is present in the inputs or was fetched live in this session and returned a page identifying the intended reference.",
		"A failed fetch or a search snippet is not verification.",
		"Never guess a URL",
		"If neither source is available, write the name as plain text.",
		"This house rule applies across skills and hand-offs; it does not authorize fetching unrelated sources or changing accepted historical prose.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	require.NotContains(t, text, "follow the External references section in")
}

// One general rule replaces the per-skill notes about commands that do not exist.
func TestOrientationConfirmsUnfamiliarCommands(t *testing.T) {
	text := normalizedOrientation()
	require.Contains(t, text, "Confirm an unfamiliar command or flag with `tiller <command> --help` before running it. Never guess one.")
	require.Less(t, strings.Index(text, "- Mutate state only through"), strings.Index(text, "- Confirm an unfamiliar command or flag"))
	require.Contains(t, text, "never a trial mutation.")
	require.NotContains(t, text, "invented dry-run flag")
}

// The recovery protocol lives once, in orientation, and every skill inherits it.
func TestOrientationCarriesTheRecoveryProtocol(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"Follow the recovery protocol when a command refuses.",
		"Every skill inherits it and corrects itself from the CLI's exit codes without a human.",
		"fix the work and try again, do not escalate",
		"Stop and report when the same gate fails the same way after two different fixes",
		"Exit `2` or `3` means correct the call or environment.",
		"names the legal next moves with nothing executed",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// The node reference convention, naming and command-position addressing, is
// stated once in orientation and inherited by every skill.
func TestOrientationCarriesNodeReferenceConvention(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"`submit-certification/certification-form-loading (F-79dh3q)`",
		"`submit-certification/certification-form-loading/schema-transcription (T-k3p9w6)`",
		"This convention is inherited by every skill",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// In command position an existing node takes either form, and the add verbs
// refuse a parent addressed by id.
func TestOrientationStatesTheAddressingRuleTheCLIActuallyHas(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"In command position an existing node takes either form: a bare id is always valid, and a keyed noun also takes its key path.",
		"The `add` verbs are the exception",
		"refuse a parent addressed by id with exit `2`",
		"A question is id-only in command position, since it is never keyed.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	require.NotContains(t, text, "In command position bare ids are always valid")
	require.NotContains(t, text, "Bare ids belong inside commands, where they are always valid.")
}

// Orientation carries what orient's copy of the shelf paragraph said and its
// own did not.
func TestOrientationCarriesTheWholeSeverityRule(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"so hand the human `tiller approve bug <key> --severity <level>` for one",
		"Like `rename` and `link`, `tiller grade bug <key> <level>` is a direct command",
		"from `proposed` through `in_review`",
		"Severity never routes: it sorts and filters the console's Fix board",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// The readout rule states every clause the six readout skills used to repeat.
func TestOrientationCarriesTheWholeReadoutRule(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"reproduce it verbatim as the last thing in your reply",
		"The block closes the reply, nothing follows it, and it ends on its `**Next:**` ranked list",
		"the one place the developer looks to learn what happens next",
		"with no list of your own after it",
		"Do not restate the block in prose around it.",
		"Keep the notes to facts about this run.",
		"Commentary is yours to add freely, with the notes ahead of the block, never after it; a question for the human is a recorded question.",
		"inside your run's scope it is your queue, and outside it, it is not authorization",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// Correction routing is told once: the general rules, the structural-change
// preflight, and one correction table the skills point at. The rows carry what
// specify's table, orient's classes, and orientation's own bullets each said.
func TestOrientationCarriesTheCorrectionTable(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		// General rules.
		"An explicit human correction that maps to a class authorizes the mapped action except `cancel` on a work node.",
		"hand the exact `cancel` command to the human rather than run it",
		"For other mapped actions, do not ask which mutation path to use.",
		"The correction is already the developer's go for that action: carry it out without asking the human to translate their correction into Tiller vocabulary.",
		"Change machine state only through `tiller`, never hand-edit machine state or node YAML, and edit only human-owned Markdown.",
		"Then read the result back, commit it, rerun the current readout with `--since <sha>` naming the commit the last block you reported, and return to the row's existing approval, refresh, reapproval, task-approval, or amendment gate.",
		"A seam exit keeps the full readout; a correction inside a gate gets that lighter block.",
		// The structural-change preflight.
		"Before negotiating a structural change, load the installed `tiller guide amend`",
		"Current ready state does not erase execution history.",
		"smallest safe alternative separately before asking product questions",
		"If installed help lacks a move operation, disclose the limitation",
		// What approval does not freeze.
		"Approval does not freeze display titles, epic prose, additive child work, soft order, or same-scope prose on a non-terminal feature or task.",
		"Accepted prose on a done or cancelled feature or task is historical and is not revised in place.",
		"Accepted basis, terminal history, and active structural work still have teeth.",
		"Do not invent an amendment for work that only extends the accepted plan.",
		// The table and its rows.
		"**Correction table.**",
		"| Correction | Route |",
		"| Pending baseline proposal | Re-enter `derive-baseline`",
		"Do not regenerate duplicate docs or open a work-node amendment for that correction. |",
		"| Unapproved proposal (draft tree) | Re-enter `decompose`",
		"and return to the same epic approval gate, not `amend`. |",
		"| Specced but unapproved feature plan (pre-work prose plan) | Re-enter `specify`",
		"| Epic, feature, or task display title, at any state | Run `tiller rename <noun> <node> --title \"<display title>\"`; no amendment. |",
		"| Approved epic narrative wording | Edit `epic.md` and commit it",
		"| Approved non-terminal feature or task prose, same scope | A wrong factual claim found while executing a task is an execute departure",
		"| Done or cancelled feature or task historical prose | Do not edit the accepted Markdown in place",
		"| Ready feature or task plan whose approval should replay | Run `tiller return <noun> <node>",
		"stop for the human to approve again before work starts. |",
		"| Approved feature needs one more task | Run `tiller add task <epic-key>/<feature-key>/<task-key>",
		"| Preferred sequence | Run `tiller order <noun> <node> [<node> ...]`",
		"| Required sequence before affected work starts | Add the hard edge with `tiller link <noun> <node> --depends-on <target>`",
		"| Dependency change after affected work starts | Explain that an in-place edge edit is unsupported.",
		"| Genuine structural reshape (scope genuinely abandoned or replaced, including after accepted work starts) | Route the structural reshape to `amend`",
		"Propose wider replanning for developer direction. |",
		"| Feature key or parent, draft and task-free feature | Run `tiller rename feature <ref> --key <epic>/<key>`; changing parents additionally requires both source and destination epics to be draft. |",
		"| Epic key | Run `tiller rename epic <ref> --key <canonical-key>` only on a draft epic with only draft, task-free features. |",
		"| Task key | Run `tiller rename task <ref> --key <canonical-key>` only on a proposed task staying under its current feature. |",
		"| Proposed bug, chore, or tweak key | Run `tiller rename bug <ref> --key <canonical-key>`, or the same for a chore or tweak, using one local key segment. |",
		"A key rename never renames a branch or rewrites review history.",
		"| Unchanged feature key or parent correction outside the draft, task-free window (reviewed unstarted feature move) | Run amend's preflight;",
		"name it and its history as the refusal; do not manufacture replacement or edit state around the restriction",
		"The eligibility detail, what survives a move, what refuses, and ordering, is in `tiller guide amend`. |",
		// Interleaving, with orient's clauses folded in.
		"A projected epic ordering cycle is a nonblocking advisory about grouped rendering, not an execution deadlock;",
		"Never delete a valid prerequisite, or tell the developer to, merely to make the epic map sort cleanly.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"sink epic",
		"- **Reviewed unstarted feature move:**",
		"- **Accepted, lightweight correction:**",
		"- **Genuine structural reshape:**",
	} {
		require.NotContainsf(t, text, gone, "orientation no longer carries %q", gone)
	}
}
