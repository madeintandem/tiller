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
		"`tiller guide` serves eleven skills:",
		"- `code-review`: the developer explicitly asks",
		"`.tiller/reviews/`",
		"Do not invent a `tiller readout code-review` command.",
		"During `design-brief` or a `code-review` review, recover only through corrected reads.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// Orientation keeps only the maintenance boundary: the three scopes in one
// bullet, an explicit request routed to the upgrade skill outside the ranked
// list, and the identity rules an agent needs without loading that skill. The
// schema notes, Homebrew ownership, adoption, and phase recovery moved to
// `tiller guide upgrade` (TestUpgradeSkillCarriesMaintenance in package skills).
func TestOrientationKeepsOnlyTheMaintenanceBoundary(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ") // the file hard-wraps prose
	for _, want := range []string{
		"Maintenance is an explicit request. `tiller upgrade` updates the binary and this checkout; `tiller upgrade --project-only` refreshes this checkout with the running binary; `tiller join` selects identity and is never the refresh remedy. For anything more, load `tiller guide upgrade`.",
		"`tiller guide` serves eleven skills:",
		"- `upgrade`: the developer explicitly asks to upgrade Tiller",
		"goes directly to `tiller guide upgrade`, outside the ranked list.",
		"`upgrade` is not a lifecycle seam either",
		"Do not invent a `tiller readout upgrade` command.",
		"Missing identity still calls for `tiller join --as <you>` after human registration.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"Schema 6 freezes execution ownership",
		"Schema 8 adds the Tweak shelf",
		"`--binary-only` deliberately excludes project maintenance",
		"Verified Homebrew installs delegate",
		"A schema fence is local",
		"Schema 9 renames",
	} {
		require.NotContainsf(t, text, gone, "orientation leaves %q to the upgrade skill", gone)
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
		"Any other reply is answered or acknowledged, and the list still waits for its go.",
		"Start the top agent move in your next turn only when the developer's reply agrees to it: a go, a yes, any plain assent such as \"sounds good\", or an instruction naming that row.",
		"Work already moving inside a seam keeps moving",
		"starts the top agent move only when it agrees to it.",
		"A reply naming a different row or other work redirects you to that.",
		"never rewrite the list",
		"The list holds only legal moves, so every row can be acted on as printed.",
		"`queue: N bugs, M chores, K tweaks ready`",
		"`tiller guide orient` explains how the list is ranked and what each move leads to.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	require.NotContains(t, text, "only when it says to")
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
// default, the developer's checkout instructions win, and other agents may
// share the repository. The worktree procedure lives in execute.
func TestOrientationExecutesInAWorktreeByDefault(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"Execute each unit of work in its own Git worktree by default",
		"A unit of work is what one review covers, so review grain sets it",
		"the whole feature at feature grain, the whole epic at epic grain",
		"instructions about checkouts and worktrees take precedence over this default",
		"`.tiller/identity.yml` is per checkout and gitignored, so a new worktree has no identity until you run `tiller join --as <you>` inside it.",
		"other agents and developers may be working this repository at the same",
		"not create for this work is someone else's",
		"never remove, reset, clean, stash, commit into, or check out over it.",
		"look for the work's canonical branch in local worktrees, branches, and the remote",
		"than choosing a winner",
		"work is committed and, when remote-backed, pushed",
		"`tiller guide execute` carries the worktree procedure under **Worktrees and other agents**.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"the unit's one worktree holds each riding task branch and the aggregate in turn",
		"Tasks built in parallel within a unit, by subagents or otherwise",
		"tracked file join's guidance refresh changed rather than committing it",
		"the recorded branch, or add and join one for it, and run `resume` there",
		"Integration-branch closeout runs where that branch is checked out",
		"Another agent's claim lands on its task branch",
		"never force a checkout of a branch Git reports is checked out elsewhere",
	} {
		require.NotContainsf(t, text, gone, "the worktree procedure left orientation for execute: %q", gone)
	}
}

// Orientation keeps the branch boundary and points at execute for the branch
// procedure: naming, bases, assembly, and legacy-ref collisions.
func TestOrientationKeepsTheBranchBoundary(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"Every started task has its own recorded branch, unchanged on resume.",
		"Tiller records and verifies branch structure; it does not create, switch, or merge refs.",
		"Read the recorded names rather than inventing them, and never delete, rename, or rewrite local or remote history to make a branch fit: `tiller sync` matches merges against the recorded branches, and rewritten history discards commits other people hold.",
		"Merging task branches into their recorded aggregate is agent work; the final integration merge is the human's.",
		"`tiller guide execute` carries the branch procedure under **Claiming and starting** and **Aggregate assembly**.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"Never infer the next round's branch from an old PR.",
		"No cherry-pick or squash assembly",
		"Read back start's `branch` and `base_branch` before creating refs.",
		"A retained legacy feature-prefix ref may block a new task ref",
	} {
		require.NotContainsf(t, text, gone, "the branch procedure left orientation for execute: %q", gone)
	}
}

// Orientation keeps what an instruction to execute authorizes and what stays
// the human's, and points at execute for the publication sequence.
func TestOrientationPointsAtExecuteForPublication(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"An instruction to execute includes ordinary implementation and state commits, pushes, PR creation or reuse, and submission for the authorized work.",
		"it does not authorize unrelated execution. A pure orient request still briefs and stops.",
		"Explicit developer no-publish or earlier-stop instructions control.",
		"Tiller does not push or create PRs",
		"Never attest, never assert a fast-forward, and never perform the final integration merge.",
		"Never force-push, bypass a gate, invent a PR, or commit to a review branch after its final integration merge: a force-push rewrites history and discards commits other people hold, and a commit after that merge is in no review and never reaches integration.",
		"Pure reads verify neither host state nor pushes.",
		"recording the missing historical review needs explicit authorization naming it",
		"`tiller guide execute` carries the sequence under **Publication sequence**",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"a gate already passed on unchanged code is reused",
		"At feature or epic grain the human runs it on the recorded aggregate branch",
		"--recover-merged",
		"A draft with acceptance outstanding, a refused formal submit, and an unpublished submission-state commit are distinct facts.",
		"Task-grain last-task closeout belongs on the integration branch",
	} {
		require.NotContainsf(t, text, gone, "the publication sequence left orientation for execute: %q", gone)
	}
}

// Orientation keeps who may authorize a transfer and points at execute for
// what transfer does.
func TestOrientationKeepsTheTransferBoundary(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"Before start, a reservation changes hands through `reassign`.",
		"For already-started unfinished work, only an explicit human instruction naming the work and recipient authorizes `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`.",
		"under **Exceptional human-directed transfer**.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"Transfer cancels the original with its owner",
		"`transfer_review_unobservable`",
		"At task/bug/chore/tweak PR grain close the old PR without merging",
	} {
		require.NotContainsf(t, text, gone, "the transfer procedure left orientation for execute: %q", gone)
	}
}

// Skills point into orientation's ownership, contributors, and review-grain
// paragraphs by bold label, so each label must exist verbatim.
func TestOrientationLabelsOwnershipContributorsAndReviewGrain(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"**Ownership.** Every new task is unclaimed",
		"Never take another contributor's reservation by changing `--contributor`, or your acting identity, to theirs.",
		"**Contributors.** Contributors on a feature or epic",
		"**Review grain.** Review does not have to land once per task.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
}

// Every skill reads a missing named section as stale orientation, so every
// passage a skill points at carries its bold lead-in label verbatim. Each label
// sits before the opening words other pins already hold.
func TestOrientationLabelsEveryPointerTarget(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"- **Managed-project independence.** Managed projects must never depend on Tiller:",
		"- **Developer authorship.** The developer owns and is responsible for the work.",
		"- **External references.** In authored prose",
		"- **Gates and delegation.** Drive work to the edge of done and stop.",
		"- **Committing state.** Commit the state Tiller writes.",
		"- **Worktrees.** Execute each unit of work in its own Git worktree by default",
		"- **Recovery protocol.** Follow the recovery protocol when a command refuses.",
		"- **Node references.** Name a node to the human by its key path",
		"- **Readout.** Report through the readout.",
		"**Ownership.** Every new task is unclaimed",
		"**Contributors.** Contributors on a feature or epic",
		"**Review grain.** Review does not have to land once per task.",
		"**Review grain changes.** Invalid stored grain values refuse the tree rather than defaulting.",
		"**Structural-change preflight.** Before negotiating a structural change",
		"**Correction table.**",
	} {
		require.Containsf(t, text, want, "orientation labels %q", want)
	}
}

// Closeout is the agent's bounded work; it stops at the human's acceptance
// rather than listing acceptance among the work it carries.
func TestOrientationCloseoutStopsAtHumanAcceptance(t *testing.T) {
	text := normalizedOrientation()
	require.Contains(t, text, "- **closeout**: the bounded work that carries a reviewed unit to done after its finish line or its reported merge: feature checks, evidence, and review preparation up to the human's acceptance, then submission, sync, and the state commits that record them.")
	require.NotContains(t, text, "feature checks and evidence, human acceptance, submission")
}

// The frontier rule lives in orient's frontier bullet.
func TestOrientationLeavesTheFrontierRuleToOrient(t *testing.T) {
	require.NotContains(t, normalizedOrientation(), "Never tell the developer the frontier shows only their own work.")
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
	require.GreaterOrEqual(t, OrientationGeneration(), 61)
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "The one exception is compacted events: when `tiller sync`, `tiller compact`, `tiller migrate`, or `tiller upgrade` names paths to stage, stage exactly those, directories included.")
}

// Generation 61 retired the intake skill: decompose carries both routes for a
// document, and the orientation text says so without naming an intake skill.
// Generation 62 gives each shared rule one home: orientation defines its terms
// under ## Terms, and maintenance guidance moves to the upgrade skill.
// Generation 63 extends missing-review recovery to the shelf nouns.
// Generation 64 adds the batch.
func TestOrientationGeneration64(t *testing.T) {
	got, ok := OrientationStampGeneration([]byte(Orientation()))
	require.True(t, ok)
	require.Equal(t, 64, got)
	require.Contains(t, normalizedOrientation(), "If an aggregate, or the fix for a bug, chore, or tweak, finally merged before its Tiller review was recorded, a merge report alone does not authorize recording it")
	text := strings.Join(strings.Fields(Orientation()), " ")
	require.Contains(t, text, "## Terms")
	require.Contains(t, text, "`tiller guide upgrade`")
	require.NotContains(t, text, "- `intake`:", "intake is a command, not a skill")
	require.NotContains(t, text, "Use intake when")
	require.NotContains(t, text, "capture and intake shapes")
	require.Contains(t, text, "`tiller intake`")
	require.Contains(t, text, "draft amendment")
	require.Contains(t, text, "`tiller propose amendment`")
}

// Generation 64: the orientation defines the batch, puts creating one in the
// resolve delegation class, and sends a shared-review request to it.
func TestOrientationDefinesTheBatch(t *testing.T) {
	text := strings.Join(strings.Fields(Orientation()), " ")
	for _, want := range []string{
		"- **batch**, **batched**: a batch is one review of several bugs, chores, or tweaks: one claim, one branch, one set of gates, one pull request.",
		"Creating or changing a batch (`tiller add batch`, `tiller batch`) sits in the same class: run it only on an explicit instruction that names the members and asks for one review.",
		"On a batch's branch every commit carries the id of the member it serves.",
		"When the developer asks for several bugs, chores, or tweaks under one review, that is a batch, never a hand-named branch",
		"a `batched` node waits on its batch: it is never claimed, started, checked, or submitted on its own.",
		"`batches/<batch-key>/` holds a batch's machine-owned `batch.yml` state and no prose",
	} {
		require.Contains(t, text, want)
	}
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

// Where things live says plans/ holds optional working plans, not execution
// plans.
func TestOrientationNamesPlansAsOptionalWorkingPlans(t *testing.T) {
	text := normalizedOrientation()
	require.Contains(t, text, "hold optional working plans, decision records")
	require.NotContains(t, text, "hold execution plans")
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
		"Run the planning command itself in that checkout",
		"Never switch a checkout's branch to get there.",
		"use the worktree that has the integration branch checked out (`git worktree list`), or add one and join inside it",
		"In a checkout you moved to for this, commit only when it holds no changes you did not make; otherwise stop and name what is in the way.",
		"and questions recorded while orienting. Claiming and starting a unit is execution, not planning: that state follows the unit onto its recorded branch.",
		"That checkout is usually the primary checkout, since Git checks out a branch in only one worktree.",
	} {
		require.Contains(t, text, want)
	}
	require.NotContains(t, text, "Commit there only when it holds no changes you did not make")
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

// The prohibition review's duplicates ruling (ruling 2 of the 2026-11-14
// rulings recorded in skill-set.md, "Decisions settled so far") removed two sentences from
// orientation, each repeating the sentence beside it. Each row names one by a
// phrase no surviving text carries, so neither creeps back.
func TestOrientationDropsTheRuledDuplicates(t *testing.T) {
	text := normalizedOrientation()
	for _, row := range []struct{ gone, home string }{
		{"ahead of the block, never after it", "The block closes the reply, nothing follows it"},
		{"For other mapped actions, do not ask which mutation path to use.", "The correction is already the developer's go for any other mapped action"},
	} {
		require.NotContainsf(t, text, row.gone, "orientation no longer carries %q", row.gone)
		require.Containsf(t, text, row.home, "orientation keeps the rule in %q", row.home)
	}
}

// The prohibition review's reasons ruling (ruling 3) gave each orientation
// boundary that had none its reason, and its positive-forms ruling (ruling 4)
// restated seven prohibitions positively. Each row pins a phrase of the new
// text; the old negative forms are gone.
func TestOrientationStatesRuledReasonsAndPositiveForms(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		// Ruling 3: reasons.
		"`attest` and `sync --accept-fast-forward` each record the human's own verdict or testimony, and the CLI cannot tell your keystroke from the human's, so this rule is all that keeps the record true.",
		"Managed projects must never depend on Tiller: each must build, test, and ship for people and machines without Tiller, and be able to drop it.",
		"to make a branch fit: `tiller sync` matches merges against the recorded branches, and rewritten history discards commits other people hold.",
		"after its final integration merge: a force-push rewrites history and discards commits other people hold, and a commit after that merge is in no review and never reaches integration.",
		"never a trial mutation: every mutation appends events that stay in the record.",
		// Strengthened so the skills' copies could leave without loss.
		"It authorizes only the maintenance named; never claim, start, specify, or approve anything.",
		"Each search or query rebuilds from local files without identity, writes, or a persistent cache",
		// Ruling 4: positive forms.
		"is recorded with `tiller add question`, where a gate and later sessions can see it, even when it is also discussed in chat.",
		"fix the work and try again; escalate only under the stop rule that follows.",
		"every epic, feature, or task id appears with its key path, because a bare id sends the developer off to look up what it points at.",
		"Look for Tiller documentation and schema shapes only inside the project.",
		"Take both steps: without them there is no acting contributor here",
		"Answer repository facts yourself by reading the repository.",
		"Work that only extends the accepted plan proceeds without an amendment.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	for _, gone := range []string{
		"never asked only in chat",
		"try again, do not escalate",
		"id never appears without its key path",
		"Never go hunting outside the project directory",
		"Do not skip either step",
		"Do not ask for facts the repository can answer.",
		"Do not invent an amendment for work that only extends the accepted plan.",
	} {
		require.NotContainsf(t, text, gone, "orientation states %q positively now", gone)
	}
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
	require.Contains(t, text, "A riding task's review ref is also called its riding ref.")
	require.Contains(t, text, "explains what Tiller is, its terms, the rules you must follow, how to work, and where things live")
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
	require.Contains(t, text, "never a trial mutation: every mutation appends events that stay in the record.")
	require.NotContains(t, text, "invented dry-run flag")
}

// The recovery protocol lives once, in orientation, and every skill inherits it.
func TestOrientationCarriesTheRecoveryProtocol(t *testing.T) {
	text := normalizedOrientation()
	for _, want := range []string{
		"Follow the recovery protocol when a command refuses.",
		"Every skill inherits it and corrects itself from the CLI's exit codes without a human.",
		"fix the work and try again; escalate only under the stop rule that follows.",
		"Stop and report when the same gate fails the same way after two different fixes",
		"Exit `2` or `3` means correct the call or environment.",
		"Never get past a refusal by deleting `.tiller/.cache`, a lock, or the mutation journal, by downgrading the schema or the generated guidance, or by running an older binary on migrated state.",
		"A refusal naming `mutation_recovery_needed` means an unfinished mutation left its journal: keep it and stop for the developer.",
		"when a failure does not reproduce when you run the gate's own command directly",
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
		"Commentary is yours to add freely ahead of the block; a question for the human is a recorded question.",
		"inside your run's scope it is your queue, and outside it, it is not authorization",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	require.NotContains(t, text, "with the notes ahead of the block, never after it")
	// "never after it" repeated "nothing follows it", pinned above.
	require.NotContains(t, text, "ahead of the block, never after it")
}

// correctionRow returns the correction table row whose first cell is label.
func correctionRow(t *testing.T, label string) string {
	t.Helper()
	for _, line := range strings.Split(Orientation(), "\n") {
		if strings.HasPrefix(line, "| "+label+" |") {
			return line
		}
	}
	t.Fatalf("the correction table has no %q row", label)
	return ""
}

// The dependency guard leaves one edit open after a task starts: adding an edge
// whose every target is a done task (mutate's
// TestLinkStartedTaskAddsOnlyDoneTargets pins the guard). The table states that
// exception and its limits, and no line calls every in-place edge edit on
// started work unsupported without it.
func TestOrientationStatesTheStartedTaskDependencyException(t *testing.T) {
	row := correctionRow(t, "Dependency change after affected work starts")
	for _, want := range []string{
		"a started task may add an edge",
		"`tiller link task <node> --depends-on <target>`",
		"when every target it adds is a task whose status is `done`",
		"A cancelled or unfinished target",
		"any removal",
		"`dependency_change_locked`",
		"Never manufacture replacement to evade the lock.",
	} {
		require.Containsf(t, row, want, "the dependency row states %q", want)
	}
	for _, line := range strings.Split(Orientation(), "\n") {
		if !strings.Contains(line, "unsupported") {
			continue
		}
		if !strings.Contains(line, "edge") && !strings.Contains(line, "dependency") {
			continue
		}
		require.Containsf(t, line, "done task",
			"a line that calls an in-place edge edit unsupported names the done-task exception: %q", line)
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
		"The correction is already the developer's go for any other mapped action: carry it out without asking the human to translate their correction into Tiller vocabulary.",
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
		"Work that only extends the accepted plan proceeds without an amendment.",
		// The table and its rows.
		"**Correction table.**",
		"| Correction | Route |",
		"| Pending baseline proposal | Re-enter `derive-baseline`",
		"Do not regenerate duplicate docs or open a work-node amendment for that correction. |",
		"| Unapproved proposal (draft tree) | Re-enter `decompose`",
		"apply its draft correction from the command list under **Correcting the draft** in `tiller guide decompose`",
		"and return to the same epic approval gate, not `amend`. |",
		"| Specced but unapproved feature plan (pre-work prose plan) | Re-enter `specify`",
		"apply the unapproved-plan route under **Correcting the feature plan** in `tiller guide specify`",
		"| Epic, feature, or task display title, at any state | Run `tiller rename <noun> <node> --title \"<display title>\"`; no amendment. |",
		"| Approved epic narrative wording | Edit `epic.md` and commit it",
		"| Approved non-terminal feature or task prose, same scope | A wrong factual claim found while executing a task is an execute departure",
		"| Done or cancelled feature or task historical prose | Do not edit the accepted Markdown in place",
		"| Ready feature or task plan whose approval should replay | Run `tiller return <noun> <node>",
		"stop for the human to approve again before work starts. |",
		"| Approved feature needs one more task | Run `tiller add task <epic-key>/<feature-key>/<task-key>",
		"| Preferred sequence | Run `tiller order <noun> <node> [<node> ...]`",
		"| Required sequence before affected work starts | Add the hard edge with `tiller link <noun> <node> --depends-on <target>`",
		"| Dependency change after affected work starts | One edit stays direct:",
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
		// The next sentence's "already the developer's go" covers it.
		"For other mapped actions, do not ask which mutation path to use.",
		"sink epic",
		"- **Reviewed unstarted feature move:**",
		"- **Accepted, lightweight correction:**",
		"- **Genuine structural reshape:**",
	} {
		require.NotContainsf(t, text, gone, "orientation no longer carries %q", gone)
	}
}
