package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// emDash is the byte the house style forbids in any authored prose.
const emDash = "—"

// harnessTools is the denylist proving each skill names only a filesystem, a
// shell, and the tiller CLI, never a harness-specific tool (skill-set.md, the
// harness-agnostic hard rule). The match is case-insensitive.
var harnessTools = []string{
	"bash", "read tool", "edit tool", "write tool", "grep tool",
	"glob tool", "view tool", "str_replace", "multiedit", "notebookedit",
	"apply_patch",
}

// TestNamesAreTheShippedSkills keeps the discoverable catalog in lifecycle order.
func TestNamesAreTheShippedSkills(t *testing.T) {
	require.Equal(t, []string{
		"orient", "derive-baseline", "decompose", "specify", "execute", "capture", "mend-drift", "amend", "intake", "design-brief", "code-review",
	}, Names())
	require.False(t, Has("break-down"), "break-down is retired into specify")
}

func TestCaptureWritesAProposalAndStopsBeforeTriage(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	for _, want := range []string{
		"name: capture",
		"tiller add bug",
		"tiller add chore",
		"--attach",
		"--source",
		"## Reproduction",
		"## Fixed when",
		"## What",
		"## Done when",
		"never approve",
		"status remains `proposed`",
		"`cancel` remains human-only",
		"`resolve` is delegable only on an explicit human instruction naming the shelf node and either `--clear` or the replacement carrier target",
	} {
		require.Containsf(t, text, want, "capture pins %q", want)
	}
	require.NotContains(t, text, "human-only cancel and resolve")
	start := strings.Index(text, "## Choreography")
	handoff := strings.Index(text, "## Hand-off")
	require.Positive(t, start)
	require.Greater(t, handoff, start)
	choreography := text[start:handoff]
	for _, forbidden := range []string{"tiller approve", "tiller claim", "tiller start", "tiller resolve", "tiller cancel"} {
		require.NotContains(t, choreography, forbidden)
	}
}

// readoutSeams are the skills with a rendered seam-exit block. mend-drift
// instead leaves a drafted fix for the human; capture stops at a shelf proposal.
var readoutSeams = []string{"orient", "derive-baseline", "decompose", "specify", "execute", "amend"}

// reviewArmChoreography is the review arm's slice of the code-review
// Choreography: from "## Choreography" up to the disposition arm's heading or,
// before that arm exists, the Hand-off.
func reviewArmChoreography(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, "## Choreography")
	require.Positive(t, start)
	rest := text[start:]
	end := len(rest)
	for _, marker := range []string{"\n### Disposition arm", "\n## Hand-off"} {
		if i := strings.Index(rest, marker); i > 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// The review arm writes one review file and nothing else: no state, no Git
// writes, no test runs, and every finding graded and anchored the same way.
func TestCodeReviewArmIsReadOnlyAndWritesOneReviewFile(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"name: code-review",
		"epic, feature, task, bug, chore, or tweak",
		"A review is never a suggested next move",
		".tiller/reviews/<YYYY-MM-DD>-<node-id>.md",
		"never overwrite a review",
		"env GIT_OPTIONAL_LOCKS=0 tiller",
		"git --no-optional-locks",
		"--no-ext-diff --no-textconv",
		"--grep=<unit-id>",
		"under Limitations",
		"Judge findings against the code as it reads now",
		"| Blocker |", "| Important |", "| Medium |", "| Minor |",
		"Bug, always a blocker",
		"Traced", "Suspected",
		"Found in",
		"Already tracked", "Previously declined",
		"regression of <id>",
		"Never reopen done work",
		"Never sample, never cap",
		"--gate <gate-key>",
		"Never add a gate",
		"There is no `tiller readout code-review`",
		// A merged head is its own merge-base with integration, so done work is
		// diffed from the first parent of the integration merge that brought it in.
		"diff --no-ext-diff --no-textconv <merge>^1...<head>",
		"Found in names each node by key path with its ID",
		"drop the `git:` prefix",
	} {
		require.Containsf(t, text, want, "code-review pins %q", want)
	}
	require.NotContains(t, text, "merge-base <integration> <review-head>",
		"a merged head diffed from its own merge-base is empty")
	require.NotContains(t, text, "--ancestry-path",
		"the first merge on the ancestry path can be an unrelated merge after a fast-forward, or a later re-assembly")
	choreography := reviewArmChoreography(t, text)
	for _, forbidden := range []string{
		"tiller check", "tiller eval", "tiller sync", "tiller attest", "tiller claim",
		"tiller start", "tiller approve", "git fetch", "git checkout", "git commit", "git stash",
	} {
		require.NotContainsf(t, choreography, forbidden, "the review arm never runs %q", forbidden)
	}
}

// The disposition arm resolves the whole selection first, rechecks moved code,
// records only through capture and intake shapes, commits once, and leaves every
// approval to the human.
func TestCodeReviewDispositionLogsThroughExistingShapesAndCommitsOnce(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"### Disposition arm",
		"tiller guide capture",
		"tiller guide intake",
		"tiller guide execute",
		"write nothing",
		"\"the rest\"",
		"log B6 as a tweak",
		"drop CH1",
		"commit the review",
		"attach to <node>",
		"rechecked at <commit>",
		"no longer reproduces at <commit>",
		"Never recheck an unselected row",
		"tiller add bug <key> --title",
		"--source .tiller/reviews/<file>.md#L<n>-L<m>",
		"-plan-changes.md",
		"tiller intake <distillation> --contributor <you>",
		"--source .tiller/feedback/<distillation>.md#L<n>-L<m>",
		"one distillation, one intake, one amendment",
		"does not own",
		"git commit -m \"code-review: <node-id> log B1 TW1 P1, drop CH1\"",
		"Never push",
		"tiller approve bug|chore|tweak <node>",
		"tiller approve amendment <amendment-key>",
	} {
		require.Containsf(t, text, want, "code-review disposition pins %q", want)
	}
	start := strings.Index(text, "### Disposition arm\n\n#### 1.")
	handoff := strings.Index(text, "## Hand-off")
	require.Positive(t, start, "the disposition arm has its own choreography")
	require.Greater(t, handoff, start)
	choreography := text[start:handoff]
	for _, forbidden := range []string{
		"tiller approve", "tiller claim", "tiller start", "tiller resolve",
		"tiller cancel", "tiller attest", "git push",
	} {
		require.NotContainsf(t, choreography, forbidden, "the disposition arm never runs %q", forbidden)
	}
}

// orient sends an explicit review or disposition request straight to the skill.
func TestOrientRoutesExplicitCodeReviews(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "loads `tiller guide code-review` directly")
	require.Contains(t, text, "For a `code-review` review, recovery is limited to corrected reads.")
}

// Merged-ness is a Git fact, not a status: the merge that brought a head in is
// the first first-parent integration merge containing it whose first parent
// does not, and an empty located diff is never "no change".
func TestCodeReviewLocatesMergedWorkByFirstParentMerge(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"git --no-optional-locks merge-base --is-ancestor <head> <integration>",
		"rev-list --first-parent --merges --reverse <integration>",
		"merge-base --is-ancestor <head> <merge>^1",
		"merged but not yet synced",
		"An empty located diff means the change was not located",
	} {
		require.Containsf(t, text, want, "code-review pins %q", want)
	}
}

// --attach takes only an epic, feature, or task, so a shelf target's findings
// attach where that shelf node was found, or nowhere.
func TestCodeReviewShelfTargetsAttachHonestly(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "`--attach` accepts only an epic, feature, or task")
	require.Contains(t, text, "its own `attached_to`, or omit `--attach`")
	require.NotContains(t, text, "For a bug, chore, or tweak target, Found in is the target.")
}

// A disposition commits only the paths it wrote, never other uncommitted reviews
// or stray state.
func TestCodeReviewStagesOnlyItsOwnPaths(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.NotContains(t, text, "git add .tiller\n")
	require.Contains(t, text, "git add <review-file> <each-path-this-disposition-wrote>")
	require.Contains(t, text, "Stage only the paths this disposition wrote")
}

// A row handed to its unit's owner stays fixable by that owner.
func TestCodeReviewHandedOffRowStaysFixableByOwner(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "A row annotated `requested change on <unit>` stays open to \"fix\" by that unit's owner")
	require.Contains(t, text, "the unit's `owner` against the checkout's identity")
}

// The target's own contract is read, not only its feature's.
func TestCodeReviewReadsTheTargetsOwnContract(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "a task target's own `task.md`")
	require.Contains(t, text, "a bug's Fixed when, or a chore's or tweak's Done when")
}

// A new session finds its review file deterministically and asks rather than guessing.
func TestCodeReviewFindsTheReviewFileDeterministically(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "Order candidates by date, then suffix number")
	require.Contains(t, text, "When more than one candidate exists, ask which")
	require.Contains(t, text, "names no node and none is in this conversation, ask which review")
	require.NotContains(t, text, "use the newest file under")
}

// The attest line names the reviewed node's own noun.
func TestCodeReviewAttestLineNamesTheTargetsNoun(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "tiller attest <feature|task> <id> --gate <gate-key>")
	require.NotContains(t, text, "tiller attest feature <F-id>")
}

// Remote-tracking refs are read when they are the only copy; only a branch absent
// from both goes to Limitations, and a divergence is reported.
func TestCodeReviewRemoteBranchRulesAgree(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "absent locally and from remote-tracking refs")
	require.Contains(t, text, "When the local branch and its remote-tracking ref differ")
	require.NotContains(t, text, "A branch present only on the remote goes under Limitations")
}

// Located diffs leave out Tiller's own state, which the ID grep also matches.
func TestCodeReviewExcludesTillerStateFromLocatedDiffs(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "-- . ':(exclude).tiller'")
	require.Contains(t, text, "Skip `code-review:` disposition commits")
}

// A revise needs its prose edit drafted before staging, as amend requires.
func TestCodeReviewReviseNeedsTheDraftedEdit(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "write the drafted prose edit to the target's spec or contract before staging `--revise`")
}

// Deferring to capture and intake borrows their shapes, never their commits, and
// the one commit lands on the integration branch.
func TestCodeReviewBorrowsShapesNotCommits(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "except their commits")
	require.Contains(t, text, "Commit on the integration branch")
}

// TestLookupReturnsEachSkill confirms every named skill is embedded and non-empty,
// and that an unknown name is a clean miss rather than a panic. With amend
// embedded there is no unbuilt seam left to carry the clean-miss assertion, so a
// plainly bogus name carries it.
func TestLookupReturnsEachSkill(t *testing.T) {
	for _, name := range Names() {
		text, ok := Lookup(name)
		require.Truef(t, ok, "skill %q is embedded", name)
		require.NotEmpty(t, text, "skill %q has a body", name)
	}
	_, ok := Lookup("nonsense")
	require.False(t, ok)
}

// TestSkillsHaveFivePartAnatomy confirms each skill carries the frontmatter
// trigger plus the reasoning, choreography, and hand-off sections
// (skill-set.md, "The anatomy of a skill"). Readout is the fifth part, added
// alongside the `tiller readout <seam>` verb (task-7-brief.md): the section
// that tells the agent to reproduce the seam's block verbatim rather than
// compose its own account of what happened. It is pinned separately below,
// scoped to readoutSeams, because mend-drift's product is a drafted fix for
// the human to approve, not a seam block the CLI renders, so it carries no
// Readout section (task-13-brief.md, Step 1).
func TestSkillsHaveFivePartAnatomy(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.Containsf(t, text, "name: "+name, "%s frontmatter names the skill", name)
		require.Containsf(t, text, "description:", "%s frontmatter carries the trigger", name)
		require.Containsf(t, text, "## Reasoning", "%s has a Reasoning section", name)
		require.Containsf(t, text, "## Choreography", "%s has a Choreography section", name)
		require.Containsf(t, text, "## Hand-off", "%s has a Hand-off section", name)
		// The choreography terminates in tiller commands.
		require.Containsf(t, text, "tiller ", "%s choreography drives the tiller CLI", name)
	}
	for _, name := range readoutSeams {
		text, _ := Lookup(name)
		require.Containsf(t, text, "## Readout", "%s has a Readout section", name)
	}
}

// TestSkillsCarryAReadoutSectionAndEndTheirChoreographyWithIt pins the two
// concrete facts TestSkillsHaveFivePartAnatomy's bare-section check cannot:
// each of the four readout-bearing skills' Choreography names the exact
// `tiller readout <seam>` command for ITS OWN seam (not a copy-pasted one from
// another skill), and each names where the agent's own account goes
// (task-7-brief.md, Step 1). mend-drift is scoped out the same way
// TestSkillsHaveFivePartAnatomy scopes it out; TestMendDriftHasNoReadoutSection
// pins its absence explicitly.
func TestSkillsCarryAReadoutSectionAndEndTheirChoreographyWithIt(t *testing.T) {
	for _, name := range readoutSeams {
		text, _ := Lookup(name)
		require.Containsf(t, text, "## Readout", "%s has a Readout section", name)
		require.Containsf(t, text, "tiller readout "+name,
			"%s choreography ends in its readout command", name)
		require.Containsf(t, text, "Notes from this run",
			"%s names where the agent's own account goes", name)
	}
}

// TestOrientNamesCountsOnlyNeedsAttentionAndQueueDrillDown pins the readout
// publication contract: one singular attention list summarizes Queue counts,
// and enumeration happens through the read-only queue command rather than
// inline node rows or mutation commands.
func TestOrientNamesCountsOnlyNeedsAttentionAndQueueDrillDown(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Equal(t, 1, strings.Count(text, "`Needs attention`"),
		"orient names one singular Needs attention list")
	for _, want := range []string{
		"It is counts-only",
		"inline `tiller queue` drill-down",
		"Do not expand it into concrete node rows, question",
		"The queue command is the one place to enumerate a count",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	require.NotContains(t, text, "needs-you headers",
		"orient no longer describes per-kind inline mutation sections")
}

// The block now ends on the ranked list, so the skills that say what the
// developer reads last name the list, not a single Next line, and still have
// the agent reproduce it verbatim.
func TestReadoutSkillsEndOnTheRankedList(t *testing.T) {
	for _, name := range []string{"orient", "derive-baseline", "decompose", "specify", "execute", "amend"} {
		raw, _ := Lookup(name)
		text := strings.Join(strings.Fields(raw), " ") // skills hard-wrap prose
		require.Containsf(t, text, "ends on its **Next:** ranked list", "%s names the list as the block's end", name)
		require.NotContainsf(t, text, "always the block's **Next:** line", "%s no longer names a single Next line", name)
	}
}

// TestMendDriftHasNoReadoutSection locks in the deliberate exception to the
// five-part anatomy: `tiller readout <seam>` is wired for the four skills
// readoutSeams names (internal/cli/readout.go), and extending it to mend-drift
// is no part of this task's scope. mend-drift's edge of done is a drafted fix
// left for the human's approval, not a rendered seam block, so its structure
// stays frontmatter, Reasoning, Choreography, Hand-off (task-13-brief.md, Step
// 1). Asserting the absence explicitly, rather than only omitting a positive
// check, keeps a future stray "## Readout" heading here from going unnoticed.
func TestMendDriftHasNoReadoutSection(t *testing.T) {
	text, ok := Lookup("mend-drift")
	require.True(t, ok)
	require.NotContains(t, text, "## Readout", "mend-drift renders no seam block; it drafts a fix for the human")
}

// TestOrientScopesTheCannotDisagreeClaim keeps orient's claim of agreeing with
// the CLI honest. The ranked list (views.Next, shared by `tiller next`, bare
// `tiller guide`, and the readout) carries no drift awareness, so a skill-less
// harness is never pointed at mend-drift by the list alone. The caveat scopes
// the claim to what the list computes from recorded tree state and names
// mend-drift as this skill's own addition, confirmed only by an actual
// `tiller drift` run.
func TestOrientScopesTheCannotDisagreeClaim(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "cannot disagree for every move the list computes from the recorded tree",
		"the claim is scoped to what the list actually computes")
	require.Contains(t, text, "this skill's own addition", "mend-drift is named as the skill's own addition, not the list's")
	require.Contains(t, text, "takes an actual `tiller drift` run", "knowing it applies takes a live sweep, not a stored-state read")
}

// TestSkillsAreNeutralProse is the hard-rule assertion: no skill contains an em
// dash, and none names a harness-specific tool. This is what keeps the surface
// identical no matter who drives it.
func TestSkillsAreNeutralProse(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.NotContainsf(t, text, emDash, "skill %q must contain no em dash (house style)", name)
		lower := strings.ToLower(text)
		for _, tool := range harnessTools {
			require.NotContainsf(t, lower, tool, "skill %q names harness-specific tool %q", name, tool)
		}
	}
}

func TestSpecifyAcceptsOnlyExplicitBatches(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	for _, want := range []string{
		"explicit instruction naming several features",
		"every feature in a named epic",
		"soft order",
		"one `specify:` commit per feature",
		"one final `tiller readout specify`",
		"never runs `approve`",
		"ranked list suggests waiting",
	} {
		require.Containsf(t, text, want, "specify pins %q", want)
	}
}

func TestExecuteCarriesTheShelfWorkLoop(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"tiller claim bug bugs/blank-on-reload",
		"tiller start bug bugs/blank-on-reload",
		"git checkout -b tiller/bugs/blank-on-reload",
		"tiller check bug bugs/blank-on-reload",
		"tiller submit bug bugs/blank-on-reload",
		"regression test",
		"ignores `review_grain`",
		"attached_to",
		"feature.md",
		"feature.eval.md",
		"task.md",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "tiller eval bug")
	require.NotContains(t, text, "tiller approve bug")
}

// TestHandoffsDisambiguateApproveAccept pins the vocabulary line at the moment of
// handoff (issue #12), corrected for a build with no accept verb: approve
// green-lights a proposed plan to start and is the only gate verb, completed
// work crosses to done on the human's merge as `tiller sync` observes it, and
// the agent translates a colloquial "accept this" by the node's state rather
// than guessing. Naming `accept` as a command here would send an agent at a verb
// that exits 2.
func TestHandoffsDisambiguateApproveAccept(t *testing.T) {
	for _, name := range []string{"decompose", "specify"} {
		text, ok := Lookup(name)
		require.True(t, ok)
		require.Containsf(t, text, "green-lights a proposed plan", "%s carries the approve side", name)
		require.Containsf(t, text, "There is no `accept`", "%s says the accept verb is gone", name)
		require.Containsf(t, text, "when the human merges and `tiller sync` observes it",
			"%s names what actually crosses completed work", name)
		require.Containsf(t, text, "never guess", "%s translates by state, never guesses", name)
	}
}

// TestOrientCarriesNodeReferenceConvention pins the reference convention (issue
// #10) as a stated-once rule in orient: human-facing prose names a node by its
// key path with the id in parentheses, a task carries its own three-segment key
// path (folder-backed tasks are keyed in their own right now), and the other
// skills inherit the convention rather than restating it.
func TestOrientCarriesNodeReferenceConvention(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "## Node references", "orient states the convention once")
	require.Contains(t, text, "form-engine/certification-form-loading (F-79dh3q)", "the key path carries the id in parentheses")
	require.Contains(t, text, "form-engine/certification-form-loading/schema-transcription (T-k3p9w6)",
		"a task carries its own three-segment key path")
	require.Contains(t, text, "This convention is inherited by every skill", "the other skills only point back to it")
}

// TestDecomposeAsksForNoEpicContract replaces the epic.eval breadcrumb this test
// used to pin. An epic has no acceptance contract in this build: its bar is the
// sum of its features' contracts and it reaches done when they do, observed by
// sync. decompose telling an agent to author epic.eval.md sent it to write a file
// nothing reads, and `tiller eval epic` / `check epic` / `attest epic` all exit 2.
func TestDecomposeAsksForNoEpicContract(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	require.NotContains(t, text, "epic.eval", "an epic carries no acceptance contract")
	require.Contains(t, text, "an epic has no acceptance contract of its own in this build",
		"decompose says so rather than leaving it unstated")
	require.Contains(t, text, "epic.md", "the epic's one authored artifact is still named")
}

// retiredModel is the vocabulary the single-tree refactor removed. A skill that
// still names any of it does not merely read stale: `--intent` exits 2, `tiller
// schedule` and `tiller accept` are not commands, `epic.eval.md` is a file
// nothing reads, and `break-down` is a skill `tiller guide` refuses. Skill prose
// has no compiler, so this denylist is the only thing standing between a
// half-finished rewrite and an agent confidently running a command that fails.
var retiredModel = []string{
	"blitz", "break-down", "--intent", "epic.eval", "tiller schedule",
	"tiller accept", "accept epic", "accept feature", "tasks.yml",
}

// TestSkillsNameNothingFromTheRetiredModel runs that denylist over every shipped
// skill, frontmatter included, since the description is what a harness triggers
// on and what `tiller join` copies onto the shim.
func TestSkillsNameNothingFromTheRetiredModel(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		lower := strings.ToLower(text)
		for _, token := range retiredModel {
			require.NotContainsf(t, lower, token, "skill %q names the retired %q", name, token)
		}
	}
}

// TestExecuteSaysTasksArriveReady closes the other side of the same merge. With
// the plan gate approving the tasks alongside the spec, there is no second
// approval in the work loop, and a task's own implementation spec is already
// written for the agent to work from. An execute that still expected to propose
// or approve would stall on a gate that already passed.
func TestExecuteSaysTasksArriveReady(t *testing.T) {
	ex, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, ex, "Tasks arrive already `ready`", "no second approval in the loop")
	require.Contains(t, ex, "read the plan `specify` wrote in the task's `task.md`",
		"the loop works from the written implementation spec")
	require.NotContains(t, ex, "tiller approve task", "execute never approves a task")
}

// evalCriterionInSpecifyContract returns the id of the one eval-kind criterion in
// the contract specify prints, by scanning its criterion blocks for the eval
// binding. It reads the skill rather than hard-coding "C4" on purpose: the point
// of the test below is that the two files agree, and a constant here would just
// move the disagreement into the test.
func evalCriterionInSpecifyContract(t *testing.T) string {
	t.Helper()
	sp, ok := Lookup("specify")
	require.True(t, ok)
	var id, found string
	for _, line := range strings.Split(sp, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- id: C") {
			id = strings.TrimPrefix(trimmed, "- id: ")
		}
		if strings.Contains(trimmed, "binding: { type: eval") {
			require.Empty(t, found, "the contract shows exactly one eval-kind criterion")
			found = id
		}
	}
	require.NotEmpty(t, found, "specify's contract shows an eval-kind criterion")
	return found
}

// TestExecuteGradesTheContractsEvalCriterion is a cross-skill invariant with no
// compiler behind it and one real failure already to its name. execute's rubric
// lines are the set's ONLY --record-rubric-output example, and they named C3
// while the contract specify calls "the complete contract surface this build
// accepts" declares C3 as a `manual` binding and C4 as its only `eval` one. An
// agent following both skills verbatim got exit 2, "criterion C3 is a manual
// binding, not an eval binding". Nothing but this test can notice the two files
// drifting apart again.
func TestExecuteGradesTheContractsEvalCriterion(t *testing.T) {
	want := evalCriterionInSpecifyContract(t)

	ex, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, ex, "--criterion "+want,
		"execute grades the criterion specify's contract actually binds to eval")
	require.Contains(t, ex, "artifacts/tiller/evals/F-79dh3q-"+want+".json",
		"and the output path follows the criterion")
	require.Contains(t, ex, `"criterion": "`+want+`"`,
		"as does the rubric_output document it shows")

	// The manual-binding criterion must appear in no rubric position at all,
	// which is the specific shape of the defect: a plausible id in a line that
	// cannot run.
	require.NotContains(t, ex, "--criterion C3", "the manual criterion is never graded")
	require.NotContains(t, ex, `"criterion": "C3"`, "nor recorded as rubric output")
}

// TestOrientStatesTheAddressingRuleTheCLIActuallyHas replaces a convention that
// was stated as absolute and is not. "In command position bare ids are always
// valid" holds for every verb that addresses an EXISTING node, and fails for the
// add verbs, which parse their argument as the key path the new node will have:
// `tiller add feature <E-id>/two` exits 2 with `feature key "E-id/two" must be
// <epic-key>/<feature-key>`. That is exactly where an agent would reach for an
// id, having just read the parent's id off the confirmation line.
func TestOrientStatesTheAddressingRuleTheCLIActuallyHas(t *testing.T) {
	o, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, o, "In command position an existing node takes either form",
		"the rule is scoped to existing nodes")
	require.Contains(t, o, "The `add` verbs are the exception",
		"and the exception is named")
	require.Contains(t, o, "refuse a parent addressed by id with exit `2`",
		"with the observed behavior, not a hedge")
	require.NotContains(t, o, "In command position bare ids are always valid",
		"the absolute claim is retired")
}

// TestSkillsStayInsideTheProject is the #17 sweep at the skill surface: no skill
// references a source-repo document, by path or by name, because the agent
// surface must be complete inside the project and the project's resolved binary
// may not match any local Tiller source checkout anyway. The bare doc names are
// guarded too: the regression that prompted the issue said "artifact-schema.md"
// without any docs/spec prefix.
func TestSkillsStayInsideTheProject(t *testing.T) {
	outside := []string{
		"docs/spec",
		"artifact-schema",
		"cli-surface",
		"skill-set.md",
		"ui-surface",
		"distribution.md",
	}
	for _, name := range Names() {
		text, _ := Lookup(name)
		for _, token := range outside {
			require.NotContainsf(t, text, token, "skill %q points outside the project (%q)", name, token)
		}
	}
}

// TestEmbeddedHandoffsUseRunnableCommands keeps product guidance on the exact
// CLI grammar instead of shorthand an agent cannot execute.
func TestEmbeddedHandoffsUseRunnableCommands(t *testing.T) {
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, decompose, `tiller answer question <Q-id> --answer "<text>"`)

	specify, ok := Lookup("specify")
	require.True(t, ok)
	for _, command := range []string{
		"tiller add task <epic>/<feature>/<task-key> --title",
		"tiller link task <key> --depends-on <sibling>",
		"tiller spec feature form-engine/certification-form-loading",
	} {
		require.Containsf(t, specify, command, "specify carries runnable command %q", command)
	}
	// The epic-grain contract trio used to be pinned here. All three exit 2 now:
	// `eval`, `check`, and `attest` answer for features and tasks only.
	for _, gone := range []string{"tiller eval epic", "tiller check epic", "tiller attest epic"} {
		require.NotContainsf(t, specify, gone, "specify must not name %q, which exits 2", gone)
	}

	execute, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, execute, "tiller attest feature <F-id> --criterion <C-id> --status pass")
}

// TestCorrectionsRerunTheReadoutWithSince pins the readout's two sizes
// (issue #94): a seam exit keeps the full block, and a correction inside a
// gate closes with `tiller readout <seam> --since <sha>`, naming the commit
// the last block reported. Each skill that carries a correction loop says
// the --since form at the sentence that used to say "rerun the readout",
// and decompose's Readout section states the split itself, since that is
// the seam where the full block was re-emitted four times in one review.
func TestCorrectionsRerunTheReadoutWithSince(t *testing.T) {
	for skill, want := range map[string][]string{
		"decompose": {
			"rerun `tiller readout decompose --since <sha>`",
			"the commit the last block you reported",
			"A seam exit gets the full block",
		},
		"specify": {
			"rerun `tiller readout specify --since <sha>`",
			"rerun the current readout with `--since <sha>`",
		},
		"orient": {
			"rerun the current readout with `--since <sha>`",
		},
	} {
		text, ok := Lookup(skill)
		require.True(t, ok)
		for _, sentence := range want {
			require.Containsf(t, text, sentence, "%s says the --since form for corrections", skill)
		}
		require.NotContainsf(t, text, "rerun the current readout,", "%s no longer tells a correction to rerun the full block", skill)
	}
}

func TestSkillsSayTillerOnlyDiffsNeedNoPR(t *testing.T) {
	for skill, want := range map[string]string{
		"decompose":  "never needs a pull request",
		"specify":    "a .tiller/-only diff needs no PR",
		"mend-drift": "commits directly on the integration branch, no PR",
		"amend":      "a .tiller/-only diff needs no PR",
		"intake":     "a .tiller/-only diff needs no PR",
	} {
		text, ok := Lookup(skill)
		require.True(t, ok)
		require.Containsf(t, text, want, "%s carries the no-PR rule for .tiller/-only diffs", skill)
	}
}

// TestAmendNamesTheShippedAmendmentSurface is the fidelity rule at this seam,
// the same guard TestExecuteDrivesVerifiedHappyPath keeps over the work loop.
// Every flag here was driven against the real binary; the denied list is the
// older sketch of this surface, which would send an agent at flags that exit 2
// (`--revise-eval` widened to `--revise`, `--reactivate` was cut because a
// rollup-backed done recomputes on its own, and there is no ADR surface in this
// build, so `tiller add adr` is not a command).
func TestAmendNamesTheShippedAmendmentSurface(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"tiller add amendment form-engine-rework",
		"--add-feature form-engine/table-backed-forms",
		"--change-type replace",
		"--add-task form-engine/rendering-modes/table-aware-rendering",
		"--supersede form-engine/certification-form-loading",
		"--by form-engine/table-backed-forms",
		"--cancel form-engine/rendering-modes/embedded-json-fallback",
		"--superseded-by form-engine/rendering-modes/table-aware-rendering",
		"--revise form-engine/rendering-modes",
		"--amends-design design/solution-design/01-overview.md",
		"--triggered-by design/solution-design/04-form-engine.md",
		"tiller show amendment form-engine-rework",
		"tiller approve amendment form-engine-rework",
		"tiller withdraw amendment form-engine-rework --reason",
	} {
		require.Containsf(t, text, want, "amend's choreography drives the shipped surface: %q", want)
	}
	for _, gone := range []string{"--revise-eval", "--reactivate", "reactivate", "tiller add adr"} {
		require.NotContainsf(t, text, gone, "amend must not name %q, which this build has no verb for", gone)
	}
}

// TestAmendCarriesThePreconditionDiscipline pins the mechanism that makes a
// staged batch reviewable rather than merely written down: every precondition
// name the CLI recognizes, the all-or-nothing refusal, and the two guard shapes
// verified against the binary that a skill stating the happy path alone would
// leave an agent trusting wrongly (a supersede stages no status guard unless one
// is asked for, and a done task carries no revision anchor to compare).
func TestAmendCarriesThePreconditionDiscipline(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"target_status", "target_revision", "key_absent", "spec_revision", "eval_revision",
		"git hash-object",
		"fails the whole approval with no writes",
		"naming every stale operation rather than only the first",
		"A supersede staging no `target_status` is not refused",
		"`target_revision` reads nothing on a done task",
	} {
		require.Containsf(t, text, want, "amend carries the precondition discipline: %q", want)
	}
}

// TestAmendSaysTheReshapeClearsTheCascadeItAnswers pins the other half of
// mend-drift's cascade hand-off. A skill that staged the reshape and left the
// developer expecting a second mending pass would send them back through
// markers the batch already answered: internal/core/drift/drift.go exempts a
// cancelled node and a superseded feature from candidate production outright,
// and a revise re-pins the accepted basis the active_basis leg compares, so
// those candidates clear on apply. The candidates the reshape does not touch
// stay mend-drift's, which is the honest half of the same sentence.
func TestAmendSaysTheReshapeClearsTheCascadeItAnswers(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "exempts a cancelled node and a superseded feature from its sweep",
		"the skill says why a cancel and a supersede clear their own candidates")
	require.Contains(t, text, "re-pins the very basis the sweep compares against",
		"and why a revise clears an active_basis candidate")
	require.Contains(t, text, "stay `mend-drift`'s, one at a time",
		"without claiming the reshape clears candidates it never touched")
}

// TestAmendCarriesThePreservationDiscipline pins what keeps a rework honest: a
// replacement's contract binds preserve criteria to characterization tests
// captured BEFORE the behavior moves. Captured after, they record the new
// behavior and prove nothing, which is the failure this paragraph exists to
// prevent. The contract shape itself stays specify's, reached through
// `tiller guide specify` rather than restated here.
func TestAmendCarriesThePreservationDiscipline(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"preserve` criteria",
		"characterization tests",
		"captured before that behavior moves",
		"records the new behavior and proves nothing",
		"tiller guide specify",
	} {
		require.Containsf(t, text, want, "amend carries the preservation discipline: %q", want)
	}
}

// TestAmendStagesAndStopsForTheHuman pins the hand-off: staging executes
// nothing, approve applies the whole batch atomically, and both terminal verbs
// run only on an explicit instruction naming the amendment (the delegation
// model). An amend that ran its own approve would be the agent performing the
// cancels and supersessions the design reserves for the human's one act.
func TestAmendStagesAndStopsForTheHuman(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"the one act that executes the supersessions, the cancels, the revisions, and the materializations this skill only ever staged",
		"applies the whole batch atomically",
		"explicit human instruction naming the amendment",
		"otherwise you hand them the exact command and stop",
	} {
		require.Containsf(t, text, want, "amend stops for the human: %q", want)
	}
	d, ok := Description("amend")
	require.True(t, ok)
	require.Contains(t, d, "stops for the human's approve",
		"the shim's own trigger line carries the stop, since that is the ambient signal a harness reads")
}

// TestAmendNamesTheStagedKeyOrderingTrap pins a refusal the shipped
// implementation produces that no reading of the flag list predicts: a --by or
// --superseded-by naming a staged addition resolves only against operations
// already on the amendment, so a forward reference exits 2 as an unknown target.
// Driven live against the binary; without this the natural authoring order
// (destructive ops first, additions after) refuses and reads like the tree lost
// a node.
func TestAmendNamesTheStagedKeyOrderingTrap(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "stage the addition before the operation that names it",
		"the ordering rule is stated as an instruction")
	require.Contains(t, text, "names no feature or task this amendment can target",
		"with the exact refusal it produces, so the agent recognizes it")
}

// TestAmendStatesTheTwoWaysOutOfAStaleBatch pins the recovery guidance the
// staging surface can actually carry out. An earlier cut told the agent to
// "restage the operations it names," which nothing in the surface can do:
// `tiller amend` appends, and no verb edits or removes an operation already
// staged, so following that instruction adds a second copy of the operation
// beside the stale one and approve refuses exactly as before. The two real ways
// out are putting the tree back the way the batch was reviewed against, and
// withdrawing to reopen against the tree as it now stands. The denial below is
// what keeps the impossible instruction from creeping back.
func TestAmendStatesTheTwoWaysOutOfAStaleBatch(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "nothing edits or removes an operation already staged: staging appends",
		"the constraint that rules out editing a staged batch is stated, not implied")
	require.Contains(t, text, "put the tree back the way the batch was reviewed against",
		"the first way out")
	require.Contains(t, text, "withdraw the amendment and open a new one staged against the tree as it now stands",
		"the second way out, with the condition that picks it")
	require.NotContains(t, text, "restage the operations it names",
		"the surface has no verb that restages one operation of a batch")
	// The second way out runs a human gate, and Recovery is read on its own when
	// a refusal lands, so it restates the discipline rather than leaving it to
	// Hand-off and the choreography block: withdraw needs a --reason and needs
	// the human to have asked for it by name.
	require.Contains(t, text, "takes a required `--reason` and runs only on an explicit human instruction naming the amendment",
		"the recovery that reaches for a human gate says so where the recovery is read")
}

// TestAmendNamesTheReviseContractRefusal pins the one exit-2 refusal on this
// seam that comes from the tree rather than from how the call was made: a revise
// re-pins its target's basis by hashing the files the basis names, so a feature
// that never got its contract has nothing to re-pin
// (mutate.checkReviseTargets). Found live on a bare draft, where it used to
// surface as a raw git error.
func TestAmendNamesTheReviseContractRefusal(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "revise_target_incomplete", "the reason token the refusal carries")
	require.Contains(t, text, "Write or restore it, or drop the operation",
		"with what to do about it")
}

// TestAmendStatesTheAmendmentAddressingRule pins the rule that makes this noun
// unlike every other keyed one: a key outlives its holder, because a duplicate
// key is refused only among proposed amendments, so one key can be carried by
// several amendments over a project's life. The resolver prefers the open
// proposal (internal/core/resolve's preferOpenAmendment), which is what a key
// almost always means, and the skill has to state both halves: what the key
// reaches, and the one thing it cannot do, which is reach past a live proposal
// to a retired holder. An earlier cut of this file taught around the
// unpreferenced behavior instead ("every later call naming it exits 2,
// ambiguous amendment key"); that guidance is now false, and the denial below is
// what keeps it from creeping back.
func TestAmendStatesTheAmendmentAddressingRule(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "A key names the open proposal whenever there is one",
		"the key resolves to the amendment still open")
	require.Contains(t, text, "a key no open amendment holds still names a single retired holder",
		"and a retired sole holder stays addressable by its key")
	require.Contains(t, text, "read those by the `A-` id their own `add` printed",
		"with the one thing a key cannot do")
	require.NotContains(t, text, "ambiguous amendment key",
		"the poisoned-key trap is fixed in the resolver, so the skill must not still teach around it")
}

// TestAmendChoreographyNamesAddQuestionsBlocksReach pins the other half of
// #49: the choreography never staged a question before this fix, so an agent
// reading it had no worked example of --blocks reaching a same-batch staged
// key, the reach docs/spec/skill-set.md's amend section already documents.
func TestAmendChoreographyNamesAddQuestionsBlocksReach(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"--add-question",
		"--blocks print-layouts/pdf-export",
		"resolving a same-batch staged epic, feature, or task key as readily as",
	} {
		require.Containsf(t, text, want, "amend's choreography stages a question with a same-batch blocks reach: %q", want)
	}
}

// TestIntakeDescriptionCarriesTheStop pins the frontmatter description on its
// own, since Description() is what `tiller join` writes onto the harness
// shim: the ambient signal must say what lands the artifact and what the
// skill does with it, and it must carry the same stop every other seam's
// description carries (TestOrientDescriptionCarriesTheStop,
// TestAmendStagesAndStopsForTheHuman's own Description assertion).
func TestIntakeDescriptionCarriesTheStop(t *testing.T) {
	d, ok := Description("intake")
	require.True(t, ok)
	require.Contains(t, d, "a document written outside the tree that critiques or redirects the work",
		"the description names what triggers the skill")
	require.Contains(t, d, "tiller intake` has already landed one and opened an amendment with nothing staged",
		"and the already-landed re-entry point")
	require.Contains(t, d, "stops for the human's approve", "the description carries the stop")
}

// TestIntakeNamesItsOwnRefusals pins the two refusal messages intake mints
// itself (task-4-brief.md's artifact_exists, task-1-brief.md's
// source_required), quoted verbatim so the agent recognizes them rather than
// treating them as a generic amend staging failure.
func TestIntakeNamesItsOwnRefusals(t *testing.T) {
	text, ok := Lookup("intake")
	require.True(t, ok)
	require.Contains(t, text, "re-intake is a new intake: rename the document or remove the old artifact",
		"the artifact_exists refusal, quoted verbatim")
	require.Contains(t, text, "intake amendments trace every operation: pass --source citing the feedback document",
		"the source_required refusal, quoted verbatim")
	require.Contains(t, text, "tiller guide amend",
		"staging refusals otherwise point at amend's own guide rather than restating it")
}

// TestIntakeHandsOffToSpecifyAndExecute pins the hand-off: approve amendment
// is the one materializing act, run only on explicit instruction, and once
// nodes are real they rejoin the ordinary lifecycle every other seam already
// owns, specify for a feature, execute for a task, answer question for a
// question, rather than intake inventing a second path for work it only
// ever staged.
func TestIntakeHandsOffToSpecifyAndExecute(t *testing.T) {
	text, ok := Lookup("intake")
	require.True(t, ok)
	for _, want := range []string{
		"the one act that materializes the epics, features, tasks, and questions this skill only ever staged",
		"every precondition and the amendment's own source both validate first, and then all of it lands or none of it does",
		"Approve and withdraw each run only on an explicit human instruction naming the amendment",
		"Out to `specify` for a materialized feature once the human pulls it forward from the frontier",
		"to `execute` for a materialized task once it clears its own `approve task`",
		"a materialized question sits on the ledger until `tiller answer question` closes it",
	} {
		require.Containsf(t, text, want, "intake hands off to the ordinary lifecycle: %q", want)
	}
}

// TestSkillsRequireShortQuestionTitlesForNewWrites pins both question creation
// paths. Direct questions and amendment-staged questions keep the full
// statement, but every new write also supplies the short title scanning
// surfaces render. A missing title is never taught as a valid command.
func TestSkillsRequireShortQuestionTitlesForNewWrites(t *testing.T) {
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, `tiller add question "<statement>" --title "<short title>"`,
		"orient's deferred briefing item carries a separate title")

	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, decompose, `tiller add question "<statement>" --title "<short title>"`,
		"decompose's direct question carries a separate title")
	require.Contains(t, decompose, `tiller add question "<statement>" --title "<short title>" --blocks <node>`,
		"decompose's blocking question carries a separate title")

	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "--add-question \"Keep the legacy embedded-JSON fallback for offline mode, or drop it once the table model ships?\" \\\n  --title \"Legacy offline fallback\"",
		"amend's staged question carries a separate title")

	intake, ok := Lookup("intake")
	require.True(t, ok)
	require.Contains(t, intake, "--add-question \"Keep or trim the legacy image parsers?\" \\\n  --title \"Legacy image parsers\"",
		"intake's staged question carries a separate title")
}

// TestDeriveBaselineRecordsForwardWorkAsQuestions pins the brownfield zero-day
// design's forward-work pass (decisions 1, 3, 4, 6, 7): one question per
// unbuilt capability, cited sources, an explicit-only standalone arm, no
// captured bugs, and a hand-off that names the next door without opening it.
func TestDeriveBaselineRecordsForwardWorkAsQuestions(t *testing.T) {
	text, ok := Lookup("derive-baseline")
	require.True(t, ok)
	for _, want := range []string{
		"### Record forward work",
		"Open one question per unbuilt capability, not per file.",
		`--title "<capability>: unbuilt" --source "<path>#<section>"`,
		"A code comment or TODO counts only when a document corroborates it",
		"Read the question ledger, proposed amendments, and the shelves first",
		"never a captured bug",
		"Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam.",
		"**Standalone arm.**",
		"Nothing in recorded state names this arm; never start it on your own.",
		"Name that next move and stop",
		"scan existing sources for unbuilt work",
		// A rerun must see declined and redirected leads, not only open ones.
		"covered by any question, open or answered, by a proposed amendment, or by a bug, chore, or tweak",
		// The decision has to be recorded for that check to see it.
		`tiller answer question <Q-id> --answer "<text>"`,
		// Legacy sources compare against the old system, not the new surface.
		"the code the as-is docs describe: surface roots for brownfield, reference roots for legacy-migration",
		// Intake needs the source on disk; a moved source comes back from its blob.
		"recover a retired source with `git show <blob-object-id>` before intake",
	} {
		require.Containsf(t, text, want, "derive-baseline carries %q", want)
	}
	require.NotContains(t, text, "an empty tree ordinarily reaches decompose",
		"an empty tree reaches decompose only when a to-be design doc exists")
}

// TestEmptyTreeSkillsNeedToBeInputs pins the brownfield zero-day route in the
// skills a harness auto-selects or an agent follows on a fresh tree: decompose
// is due only with to-be inputs, orient's onboarding stop says so, and the
// doors a forward-work answer opens accept it.
func TestEmptyTreeSkillsNeedToBeInputs(t *testing.T) {
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.NotContains(t, decompose, "or when the tree is still empty")
	require.NotContains(t, decompose, "or the project is fresh and nothing has been decomposed")
	require.Contains(t, decompose, "an empty tree with only as-is docs has nothing to decompose")

	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.NotContains(t, orient, "derive-baseline for an unconfirmed existing-code project or decompose otherwise")
	require.Contains(t, orient, "decompose when a to-be design doc exists")

	intake, ok := Lookup("intake")
	require.True(t, ok)
	require.Contains(t, intake, "a source that a forward-work question cites, once the human answers that they want it built")

	capture, ok := Lookup("capture")
	require.True(t, ok)
	require.Contains(t, capture, "A human answer to a forward-work question that names a defect is that description.")
}

// orient relays the ranked list as written, names a human move that outranks
// the top agent move, and starts the top agent move unless redirected.
func TestOrientRelaysTheRankedList(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	for _, want := range []string{
		"Run `tiller next`",
		"Relay the list as written.",
		"If any human move ranks above the top agent move, name it first.",
		"never rewrite the list",
		"A ranked move is a suggestion, not authorization.",
		"Start it in your next turn, once the developer's reply does not redirect.",
		"or the branch to continue on",
		"the feature touched by your most recent deliberate act that names one",
		"Within a kind, plan order decides",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	for _, gone := range []string{"take the first match", "`Continuity note`", "`Order note`", "the route suggests waiting"} {
		require.NotContainsf(t, text, gone, "orient no longer carries %q", gone)
	}
}

// specify grounds the claims a plan rests on in the code they describe, and
// leaves what only building reveals as an open fact (issue #140).
func TestSpecifyGroundsClaimsAboutCodeItDoesNotOwn(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	for _, want := range []string{
		"## Grounding claims about code",
		"cite where it was read",
		"the version, tag, or commit",
		"write it as an open fact for the task to determine",
		"Never write a provisional answer marked as assumed",
		"re-read each cited claim against its source",
		"the open facts this plan leaves for execution to determine",
		"A wrong factual claim found while executing a task is an execute departure",
	} {
		require.Containsf(t, text, want, "specify pins %q", want)
	}
}

// execute records a wrong factual claim in task.md as a departure rather
// than editing accepted prose, and bounds that departure (issue #140).
func TestExecuteRecordsFactualCorrectionsAsDepartures(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"is a departure, not a plan correction",
		"leave the accepted `task.md` unedited",
		"in the task's implementation commit message",
		"Repeat it in the task's PR body when one exists",
		"drops a test or edge case the `task.md` plans",
		"The rule covers only the task being executed",
		"it is no longer a departure",
		"gathered from the riding tasks' implementation commit messages",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
}

// specify settles questions at the plan gate and execute records mid-run
// questions and builds around them (issue #152, item 1).
func TestSpecifySettlesQuestionsAndExecuteBuildsAroundThem(t *testing.T) {
	specify, ok := Lookup("specify")
	require.True(t, ok)
	for _, want := range []string{
		"## Settle questions before approve",
		"the criteria against each other and against `feature.md`",
		`tiller add question "<statement>" --title "<short title>" --blocks <node>`,
		"`--blocks` names the narrowest node the answer could change",
		"A question asked only in chat is not allowed",
		"There is no `--assume-question` escape at a plan gate.",
		"Write each answer into the prose it changes",
		"which answer changed which file",
	} {
		require.Containsf(t, specify, want, "specify pins %q", want)
	}
	require.NotContains(t, specify, "Questions and commentary are yours to add freely",
		"specify no longer invites chat-only questions")

	execute, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"Record a finding that needs a human decision and keep building",
		"leave the task where it stands",
		"Recording a question never stops a run by itself",
		"stop only when the run has no legal work left, or at a stop it already makes",
		"every question the run\nrecorded and every open question still holding the run's work",
		"resumes the held task; note the answer in its implementation commit message",
		"the route an out-of-bounds departure takes",
		"Semantic merge conflicts needing a product or acceptance decision stop on the spot",
		// A mid-run question lives on the branch it is recorded on, so the run
		// holds the work it names itself and the human answers on that branch.
		"The question lands on the branch it is recorded on",
		"Push the held branch when the project is remote-backed, and keep its worktree.",
		"hold that work yourself for the rest of the run rather than trusting routing in another checkout",
		"the answer is committed on that branch",
		"the branch and worktree that carry it",
		"The question ledger, `.tiller/decisions/open-questions.yml`, is one file several task branches now edit",
	} {
		require.Containsf(t, execute, want, "execute pins %q", want)
	}
	// The specify choreography runs the final pass before spec feature and
	// commits the question ledger with the plan.
	for _, want := range []string{
		"# final pass (Settle questions before approve)",
		"git add .tiller/epics/form-engine/features/certification-form-loading .tiller/decisions/open-questions.yml",
	} {
		require.Containsf(t, specify, want, "specify choreography pins %q", want)
	}
	require.NotContains(t, execute, "Stop and route that change",
		"a finding past a departure is recorded, not a full stop")
}

// Skills that read task.md as a contract or correct it in place honor a
// recorded execution departure (issue #140 review).
func TestReviewAndAmendHonorExecutionDepartures(t *testing.T) {
	review, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, review, "judge the diff against the recorded departure")
	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "is an execute departure, not a prose correction")
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, "a wrong fact found while executing a task is an execute departure")
}

// TestSkillsRepairAMovedSurfaceThroughUnlink pins the moved-surface route: a
// migrate feature that moves its own recorded surface can only record done
// once the current location is linked and the old path unlinked.
func TestSkillsRepairAMovedSurfaceThroughUnlink(t *testing.T) {
	execute, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"`tiller unlink feature <feature> --surface <old-path>`",
		"a recorded path missing from the integration commit leaves nothing to fingerprint",
		"with a surface remedy had its recorded surface moved or removed",
	} {
		require.Containsf(t, execute, want, "execute pins %q", want)
	}
	mend, ok := Lookup("mend-drift")
	require.True(t, ok)
	require.Contains(t, mend, "dropped through `tiller unlink --surface` once the current location is linked")
	require.Contains(t, mend, "tiller unlink feature form-engine/rendering-modes --surface")
}

// execute works each unit of work, scoped by review grain, in its own
// worktree by default, gives parallel tasks within a unit their own, defers to
// the developer's checkout instructions, joins inside every new worktree, and
// leaves other agents' worktrees alone.
func TestExecuteWorksInAWorktreeBesideOtherAgents(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"### Worktrees and other agents",
		"execute each unit of work in its own Git worktree",
		"A unit of work is what one review covers, so review grain sets it",
		"the whole feature at feature grain, the whole epic at epic grain",
		"One worktree carries the whole unit.",
		"Commit before every switch",
		"give each parallel task a temporary worktree of its own",
		"Never let two builders share a worktree.",
		`git switch --detach "$start_base"`,
		"Aggregate assembly runs in the unit's worktree",
		"A unit's worktree outlives its tasks",
		"Unless the developer's or project's own instructions say otherwise",
		"their instructions win over this default",
		"other agents and developers may be working this repository at the same time",
		"so a new worktree refuses mutations with `contributor_unset` until `tiller join --as <you>` runs inside it",
		"restore any tracked file it changed with `git restore`",
		"Run `resume` only inside that worktree, never in the primary checkout",
		"Integration-branch closeout runs where the integration branch is checked out",
		"never force the checkout",
		"belongs to someone else",
		"look for the work's canonical branch in `git worktree list`",
		"Remove only worktrees you created",
		"Set `worktree` to a new path outside the repository",
		`git worktree add --detach "$worktree" "$start_base"`,
		`git worktree add --detach "$worktree" "$base"`,
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	// In both entry blocks the worktree is added and joined before the claim
	// writes state, and claim/start state is committed only on the new branch.
	order := func(earlier, later string) {
		t.Helper()
		i, j := strings.Index(text, earlier), strings.Index(text, later)
		require.NotEqualf(t, -1, i, "missing %q", earlier)
		require.NotEqualf(t, -1, j, "missing %q", later)
		require.Lessf(t, i, j, "%q must precede %q", earlier, later)
	}
	order(`git worktree add --detach "$worktree" "$start_base"`, "tiller claim task form-engine/certification-form-loading/schema-transcription")
	order(`cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim task`, "tiller claim task form-engine/certification-form-loading/schema-transcription")
	order(`git worktree add --detach "$worktree" "$base"`, "tiller claim bug bugs/blank-on-reload")
	order(`cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim bug`, "tiller claim bug bugs/blank-on-reload")
	order(`git checkout -b "$task_branch" "$task_base"`, "# Commit and share task-specific claim/start state on the new task branch.")
}

// A done feature's eval leg is mended by a reviewed correct-eval amendment,
// while the spec and surface legs keep their persistence wording (issue #141).
func TestSkillsCorrectADoneFeatureEvalThroughAmend(t *testing.T) {
	mend, ok := Lookup("mend-drift")
	require.True(t, ok)
	for _, want := range []string{
		"On the eval leg of a done feature, and for an `orphan_eval` on a done feature",
		"`tiller amend <A-id> --correct-eval <feature> --reason \"<why>\"`",
		"stop for the human's `tiller approve amendment <A-id>`",
		"Add `--retire <criterion-id>` for each check you deliberately removed",
		"re-pins `verified_against.eval`",
		"The spec leg and the surface leg of `stale_spec` have no such command",
		"tiller amend eval-fix --correct-eval form-engine/certification-form-loading",
	} {
		require.Containsf(t, mend, want, "mend-drift pins %q", want)
	}
	require.NotContains(t, mend, "no command in this build re-pins it")

	amend, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"A done feature's eval bindings can be corrected in place",
		"`--correct-eval <feature> --reason \"<why>\"`",
		"`--retire <criterion-id>`",
		"`correct-eval.feature` event",
		"`correct_eval_changes_contract`",
		"`correct_eval_not_feature`",
		"`retire_misplaced`",
		"tiller amend form-engine-rework --correct-eval form-engine/certification-form-loading",
	} {
		require.Containsf(t, amend, want, "amend pins %q", want)
	}
}
