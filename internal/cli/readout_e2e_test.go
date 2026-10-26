package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// This file drives one scratch project from init through the done crossing,
// exercising every seam and human-gate readout against real persisted state.
// At every moment it pins the bounded block shape, Next reachability where a
// route exists, and byte parity between terminal output and the JSON readout.
// The high-cardinality fixture also reaches concurrent attention categories,
// so the arc proves the new counts-only list on a real tree rather than only
// through renderer inputs.
//
// The eight moments are the four seams `tiller readout` serves (orient,
// decompose, specify, execute) and the four human gates that emit
// their own block (approve, answer question, attest, sync; gateMomentFor in
// readout_gate.go). The arc below reaches all
// eight: the four seams as the tree is decomposed, specced, and
// executed; approve at all three of its nouns, in the order the model puts
// them (the epic's tree gate, then the feature's plan gate carrying its
// initial task with it, then a task added mid-flight, which is the only
// task an individual approve is its own decision for now); answer question
// against a real blocker the arc itself records; sync at the done crossing;
// and attest against one of the feature's own optional review gates, which
// is the grain attestation lives at. The epic needs no moment of its own at
// the end: sync rolls it up in the same pass its last feature crosses.
// No moment is skipped quietly: every one
// of the eight has a call site below, named in a comment at that call site,
// and every one now carries at least one assertion tied to what that
// specific moment is supposed to prove (not just the generic shape checks
// every moment gets), per task 10's own review finding that orient, the
// three approve calls, answer, and attest were previously checked only
// generically.

// The block remains a glanceable chart rather than a scroll. The count-only
// attention list is constant-size per category; this bound now principally
// guards Produced and the rest of each moment's own content from runaway row
// growth while leaving room for the real multi-kind execute state below.
const maxReadoutBlockLines = 65

// The bulk epics make decompose's Produced cap observable. The bulk questions
// give the attention list a nontrivial exact count without adding item rows;
// both remain open until execute so that moment carries three real categories
// concurrently.
const (
	bulkDecomposeEpics     = 10
	bulkDecomposeQuestions = 15
)

// countBlockLines counts a readout block's lines, ignoring a single trailing
// newline (renderReadout always ends the block with one, and that trailing
// newline is not a line a reader scrolls past).
func countBlockLines(block string) int {
	trimmed := strings.TrimRight(block, "\n")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// readoutBlockFromJSON reads the "readout" field every `--json` envelope
// this task touches carries additively, whether the command is `readout`
// itself or one of the gates riding it through emitReportWithReadout.
func readoutBlockFromJSON(t *testing.T, raw string) string {
	t.Helper()
	var env struct {
		Data struct {
			Readout string `json:"readout"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	return env.Data.Readout
}

// assertReadoutShape requires either the single exact counts-list heading or
// the unchanged all-clear sentence, and keeps every moment under the glanceable
// block bound.
func assertReadoutShape(t *testing.T, moment, block string) {
	t.Helper()
	if !strings.Contains(block, "Nothing needs you right now.") {
		require.Containsf(t, block, needsAttentionHeading,
			"moment %s: every nonempty block carries the counts-only attention list", moment)
		require.Equalf(t, 1, strings.Count(block, needsAttentionHeading),
			"moment %s: attention uses one umbrella heading", moment)
	}
	lines := countBlockLines(block)
	require.LessOrEqualf(t, lines, maxReadoutBlockLines, block,
		"moment %s: block ran %d lines, want <= %d (design's own bar: a chart glanced at, not a scroll)",
		moment, lines, maxReadoutBlockLines)
}

// assertNextPresent asserts a block closes on a ranked move. Task 10's own
// review found this exact assertion missing from this file's first pass: a
// Next that silently went empty (the defect task 9 found shipping unnoticed
// through three tasks) left this arc test green, because nothing here read
// the Next section at all. Checked at both grains a readout renders through,
// a seam (runReadout, readout.go) and a gate (renderReadoutFor via
// emitReportWithReadout, envelope.go), since either path could go dead on its
// own without the other noticing. The two call sites below (decompose and
// approve epic) are moments where the tree guarantees a legal move.
func assertNextPresent(t *testing.T, moment, block string) {
	t.Helper()
	require.Contains(t, block, "**Next:**\n1. ", "moment %s: the block must name what comes next", moment)
}

// assertBlockIsHumanTail is the fix for the wrinkle the design doc names
// directly (docs/spec/cli-surface.md, "The agent readout": "on a gate, the
// human side prints the existing confirmation first and the block after
// it, so the JSON readout field is that same block alone, without the
// confirmation line in front of it"): a gate's human output is the
// confirmation plus the block, never the block alone, so the correct
// assertion is that the JSON block is the human output's own tail, not that
// the two strings are equal outright. For `readout` itself (a seam, no
// confirmation in front of anything) the human output already IS the block,
// so the tail check degrades to plain equality there, the same assertion
// TestReadoutJSONCarriesTheSameTextAsHuman already pins in isolation; this
// is that same promise proven at every moment the whole arc reaches rather
// than at one seam alone.
func assertBlockIsHumanTail(t *testing.T, moment, human, block string) {
	t.Helper()
	require.True(t,
		strings.HasSuffix(strings.TrimRight(human, "\n"), strings.TrimRight(block, "\n")),
		"moment %s: the --json readout block must be byte-identical to the tail of the human output\n--- human ---\n%s\n--- json block ---\n%s",
		moment, human, block)
}

// checkSeamReadout runs `tiller readout <seam>` twice, plain then --json.
// readout is a pure read (runReadout in readout.go touches no mutation), so
// calling it twice against the identical tree state always sees the
// identical answer, unlike a gate: this is the direct two-call proof
// TestReadoutJSONCarriesTheSameTextAsHuman already uses for one seam, run
// here at whichever seam the arc has just reached. It asserts the shape
// invariants (assertReadoutShape) and the json-equals-human promise, and
// returns the human text (which for a seam IS the whole block) for the
// caller's own moment-specific content checks.
func checkSeamReadout(t *testing.T, seam string) string {
	t.Helper()
	human, _, code := execCLI(t, "readout", seam)
	require.Equal(t, 0, code, "readout %s must succeed", seam)
	raw, _, code := execCLI(t, "--json", "readout", seam)
	require.Equal(t, 0, code, "readout %s --json must succeed", seam)
	block := readoutBlockFromJSON(t, raw)
	assertBlockIsHumanTail(t, seam, human, block)
	assertReadoutShape(t, seam, human)
	return human
}

// cpDir copies src's whole tree, including dotfiles (.git, .tiller), into
// the already-existing directory dst. checkGateReadout is the only caller
// (below); see its doc comment for why a gate needs a real filesystem copy
// rather than a second in-process call.
func cpDir(t *testing.T, src, dst string) {
	t.Helper()
	out, err := exec.Command("cp", "-a", src+"/.", dst).CombinedOutput()
	require.NoErrorf(t, err, "cp -a %s -> %s: %s", src, dst, out)
}

// withDir runs fn with the process cwd set to dir, restoring the prior cwd
// before returning. checkGateReadout uses this for its twin-directory
// --json run rather than t.Chdir, so the restore happens synchronously
// inside this call rather than at test cleanup: the arc keeps calling
// execCLI against the real dir immediately afterward, and a cleanup-deferred
// restore would leave every subsequent arc step running against the twin.
func withDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { require.NoError(t, os.Chdir(prev)) }()
	fn()
}

// checkGateReadout runs one of the four human gates for real against dir,
// the arc's own directory, so the mutation lands and the arc keeps going
// from it exactly as if this helper were not here. It also proves the
// json-equals-human promise for that same call, which a gate cannot do by
// simply being called twice the way checkSeamReadout does for a seam: a
// gate mutates on every call, so a second real call sees an already-
// transitioned tree (an "already ready" no-op reads a genuinely different,
// but equally valid, Verification section than the first "moved to ready"
// call). The fix is a filesystem snapshot: dir is copied whole, including
// .git (the headline's commit read) and .tiller (every id already minted),
// into a twin BEFORE the real call runs, so the twin and dir are byte-
// identical at the moment the gate is about to fire. The real call runs
// against dir; the identical command, with --json appended, runs against
// the twin. Same starting bytes, same deterministic mutation, so the two
// renders are the same block, and assertBlockIsHumanTail can compare the
// real call's human output against the twin's json block exactly as the
// design's own wrinkle describes.
func checkGateReadout(t *testing.T, dir, moment string, args ...string) string {
	t.Helper()
	twin := t.TempDir()
	cpDir(t, dir, twin)

	human, _, code := execCLI(t, args...)
	require.Equalf(t, 0, code, "gate %v must succeed", args)

	var block string
	withDir(t, twin, func() {
		jsonArgs := append(append([]string{}, args...), "--json")
		raw, _, code := execCLI(t, jsonArgs...)
		require.Equalf(t, 0, code, "twin gate %v must succeed", jsonArgs)
		block = readoutBlockFromJSON(t, raw)
	})

	assertBlockIsHumanTail(t, moment, human, block)
	assertReadoutShape(t, moment, block)
	return human
}

// wireBuildAndTestGates turns the two commented build/tests gate templates
// `tiller init` seeds (init.go's scaffold, "Fill in build and tests with the
// commands your project runs") into real command gates on [feature, task],
// the way a developer does once the stack is chosen
// (docs/spec/cli-surface.md, "Bootstrapping a project": "Configuring the
// build and tests gates ... moves to the build work, where the stack is
// chosen"). Without this, `tiller check task` has nothing required at task
// grain (the seeded evals gate is feature-only), and execute's readout would
// always read "Gates green with evidence: 0 of 0", a real answer but not the
// one that proves the check actually verifies something. "true" is the same
// always-green stand-in initGatedGitRepo (check_test.go) uses elsewhere in
// this package.
func wireBuildAndTestGates(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	s := string(data)
	before := s
	s = strings.Replace(s,
		`    # build: { kind: command, run: "<build command>", on: [feature, task] }`,
		`    build: { kind: command, run: "true", on: [feature, task] }`, 1)
	s = strings.Replace(s,
		`    # tests: { kind: command, run: "<test command>",  on: [feature, task] }`,
		`    tests: { kind: command, run: "true", on: [feature, task] }`, 1)
	require.NotEqual(t, before, s, "the seeded manifest's commented build/tests lines must still match verbatim")
	require.NoError(t, os.WriteFile(p, []byte(s), 0o644))
}

// otherTaskID reads the id of the one task that is not the one named, so the
// arc can address a mid-flight addition without parsing the random token out of
// the add output. soleTaskID cannot serve here: past the plan gate the feature
// carries two tasks on purpose.
func otherTaskID(t *testing.T, known string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Tasks, 2)
	for id := range tree.Tasks {
		if id != known {
			return id
		}
	}
	require.Fail(t, "no task other than "+known)
	return ""
}

// TestReadoutAcrossTheArc is task 10's own arc test: a scratch project
// driven for real, through the CLI, from init through the done crossing that
// rolls the epic up behind it, touching every one of the eight moments a
// readout block can render for. See this file's own package-level comment
// for the moment list and why this test exists.
func TestReadoutAcrossTheArc(t *testing.T) {
	dir := t.TempDir()
	testgit.Init(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "init")
	t.Chdir(dir)

	// init, add contributor, join: the real bootstrapping arc (docs/spec/cli-surface.md,
	// "Bootstrapping a project"), not a hand-written fixture manifest.
	_, _, code := execCLI(t, "init", "--name", "Arc Co", "--key", "arc", "--kind", "greenfield")
	require.Equal(t, 0, code)
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)
	wireBuildAndTestGates(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "tiller init + join, build/tests wired")

	// Moment 1/8: orient, before a single epic exists. With no to-be design
	// doc under .tiller/design/, the route's empty-tree arm answers "no work
	// is planned yet", and orient is the one seam the arc can reach with
	// nothing built yet. Asserted on directly (not just the generic shape
	// check), the content-specific bar every moment below now carries per the
	// review finding that orient, the four approve calls, answer, and attest
	// were previously checked only generically.
	orient := checkSeamReadout(t, "orient")
	require.Contains(t, orient, "No work is planned yet", "orient's Next hint proves nothing is planned yet")

	_, _, code = execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	// Add enough draft epics to exercise Produced's cap and enough open
	// questions to make the attention summary's exact count nontrivial. None
	// carries a feature, so the arc's own work remains scoped to form-engine.
	bulkQuestionIDs := make([]string, bulkDecomposeQuestions)
	for i := 0; i < bulkDecomposeEpics; i++ {
		_, _, code = execCLI(t, "add", "epic", fmt.Sprintf("bulk-%d", i), "--title", fmt.Sprintf("Bulk epic %d", i), "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	for i := 0; i < bulkDecomposeQuestions; i++ {
		qout, _, code := execCLI(t, "add", "question", fmt.Sprintf("Bulk open question number %d?", i), "--title", fmt.Sprintf("Bulk question %d", i), "--contributor", "jc", "--json")
		require.Equal(t, 0, code)
		bulkQuestionIDs[i] = extractID(t, qout, "Q-")
	}

	// Moment 2/8: decompose, with the full bulk counts still visible even though
	// Produced names only its bounded sample.
	decompose := checkSeamReadout(t, "decompose")
	require.Contains(t, decompose, "**Produced**", "decompose names the epic it laid down")
	require.Contains(t, decompose, fmt.Sprintf("Draft epics: %d", 1+bulkDecomposeEpics))
	require.Contains(t, decompose, "Draft features: 1")
	require.Contains(t, decompose, fmt.Sprintf("Open questions: %d", bulkDecomposeQuestions))
	assertNextPresent(t, "decompose", decompose)

	// Keep the bulk fixture open until execute. The arc then reaches draft-epic,
	// open-question, and review-gate categories concurrently and proves that
	// every category remains visible as one exact count bullet.
	// Moment 3/8: approve (epic form-engine, draft -> ready). The gate-grain
	// half of the Next-presence check: at this exact point in the arc the
	// epic's one feature is still draft, so specifying it is a legal move.
	approveEpic := checkGateReadout(t, dir, "approve", "approve", "epic", "form-engine", "--contributor", "jc")
	require.Contains(t, approveEpic, "Moved to ready: 1 of 1", "approve's Verification proves the act, not a queue tally")
	assertNextPresent(t, "approve", approveEpic)

	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// Moment 4/8: specify, the feature now specced.
	specify := checkSeamReadout(t, "specify")
	require.Contains(t, specify, "**Verification**", "specify reports contract validity")
	require.Contains(t, specify, "Contracts valid: 1 of 1")
	require.Contains(t, specify, cliFeatureKey,
		"specify's Produced list names the feature it specced, by its canonical key")

	tid := soleTaskID(t)

	// The task explosion is part of the stage-2 output, so it is already on
	// disk by the moment above and the feature's own approve is what flips it
	// (cli-surface.md, "Specifying a feature"). There is no break-down seam to
	// read it at: specify's block is the one account of the whole plan.
	_, _, code = execCLI(t, "readout", "break-down")
	require.Equal(t, 2, code, "break-down is not a seam this build serves")

	// approve a second time (feature, specced -> ready): the plan gate, the one
	// gate over the whole of stage 2. The block must account for the feature AND
	// every initial task the same command moved, so a build that stopped flipping
	// tasks, or one whose act carried only the node addressed, reads "1 of 2" or
	// "1 of 1" here and fails. It runs BEFORE the task-noun approve below, which
	// is the real order of the model: the initial explosion rides the feature's
	// gate, and only a mid-flight addition is its own decision.
	approveFeature := checkGateReadout(t, dir, "approve", "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Contains(t, approveFeature, "Moved to ready: 2 of 2",
		"the plan gate's block accounts for the feature and the initial task it carried")

	claimTaskAsOwner(t, tid, "jc")
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	// Work discovered mid-feature: a task added after the plan gate lands
	// proposed and is approved individually (cli-surface.md, "The work loop").
	// It is what keeps a task-noun approve moment in the arc now that the
	// initial explosion no longer has one of its own.
	addSpeccedTask(t, "render-the-form", "Render the form")
	mid := otherTaskID(t, tid)
	claimTaskAsOwner(t, mid, "jc")

	// A real blocker: a question the arc itself records against that new
	// task, so answering it is not a fixture shortcut but the same guard
	// (guardTask, internal/core/mutate/transition.go) that would refuse
	// start and submit on a genuinely blocked task.
	qout, _, code := execCLI(t, "add", "question", "Which schema revision is canonical?", "--title", "Canonical schema revision", "--blocks", mid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, qout, "Q-")

	// Moment 5/8: answer question, clearing the block the arc just
	// recorded. Scoped to act.Answered.Blocks (just mid), so this reads the
	// same "1 of 1" regardless of how many of the bulk questions above are
	// still open.
	answer := checkGateReadout(t, dir, "answer", "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")
	require.Contains(t, answer, "Nodes this answer cleared of every open question: 1 of 1")

	// At the proposed-task moment Needs attention reports the number of owning
	// features and routes through the queue. It does not expose task ids or the
	// mutating approve command from the old inline shape; the ranked Next list
	// is where that move's command appears.
	require.Contains(t, answer,
		"- 1 feature has added tasks awaiting your approval · `tiller queue --kind approve_tasks`",
		"the count is at feature grain because the queue batches proposed tasks by owner feature")
	attention, next := splitAtNext(t, answer)
	require.NotContains(t, attention, "tiller approve task",
		"the readout enumerates the category before anyone chooses a mutation")
	require.Contains(t, next, "`tiller approve task --feature ")

	// approve a third time (task, proposed -> ready), the same moment at a
	// third noun, proving the block is not a one-shot fluke.
	approveTask := checkGateReadout(t, dir, "approve", "approve", "task", mid, "--contributor", "jc")
	require.Contains(t, approveTask, "Moved to ready: 1 of 1")

	for i, id := range []string{tid, mid} {
		if id == mid {
			_, _, code = execCLI(t, "start", "task", id, "--contributor", "jc")
			require.Equal(t, 0, code)
		}
		checkoutTaskBranchCLI(t, id)
		_, _, code = execCLI(t, "check", "task", id, "--contributor", "jc")
		require.Equal(t, 0, code)
		_, _, code = execCLI(t, "submit", "task", id, "--pr", fmt.Sprintf("%d", 41+i), "--contributor", "jc")
		require.Equal(t, 0, code)
		mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[id].Branch)
	}

	// Moment 6/8: execute, both tasks sitting in_review with green gates and a
	// recorded PR. The still-open bulk epics and questions combine with the
	// optional review gates to prove a real multi-kind counts list.
	execute := checkSeamReadout(t, "execute")
	require.Contains(t, execute, "Gates green with evidence: 2 of 2",
		"both tasks' build and tests gates (wired above) ran and passed")
	require.Contains(t, execute, "Current submissions recorded: 2 of 2")
	// execute's Produced list still names each in-review task by canonical key;
	// concrete identities belong here, not in Needs attention.
	require.Contains(t, execute, cliFeatureKey+"/transcribe-the-schema")
	require.Contains(t, execute, cliFeatureKey+"/render-the-form")
	require.Contains(t, execute,
		fmt.Sprintf("- %d epics await your approval · `tiller queue --kind approve_epic`", bulkDecomposeEpics))
	require.Contains(t, execute,
		fmt.Sprintf("- %d questions await your answer · `tiller queue --kind answer_question`", bulkDecomposeQuestions))
	require.Contains(t, execute,
		"- 8 review gates await your attestation · `tiller queue --kind attest_gate`")
	require.Equal(t, 1, strings.Count(execute, "**Needs attention**"))
	// Everything above the ranked Next list is counts only. The list itself
	// names its few moves with their commands.
	counts, _ := splitAtNext(t, execute)
	for _, qid := range bulkQuestionIDs {
		require.NotContains(t, counts, qid, "question identities stay behind the queue seam")
	}
	require.NotContains(t, execute, "Bulk open question number 0?", "question statements stay behind the queue seam")
	for _, gate := range []string{"accessibility_review", "code_review", "performance_review", "security_review"} {
		require.NotContainsf(t, counts, gate, "review gate %q stays behind the queue seam", gate)
	}
	for _, mutation := range []string{"tiller approve epic", "tiller answer question", "tiller attest task", "```"} {
		require.NotContainsf(t, counts, mutation, "counts-only readout rejects %q", mutation)
	}

	// The bulk fixture's job (giving the arc's own NeedsYou section a real
	// multi-kind shape to hold, moments 3 through 7 above) is done; dispose
	// of it before sync and attest below, whose own assertions are about the
	// crossing mechanics those moments exist to prove, not about how much
	// unrelated bulk fixture is still sitting in the queue.
	// cancel carries no block (it is not one of the gates), so
	// disposing of the epics is silent; answering each bulk question does
	// emit its own block, and this test does not need to inspect it.
	for i := 0; i < bulkDecomposeEpics; i++ {
		_, _, code = execCLI(t, "cancel", "epic", fmt.Sprintf("bulk-%d", i), "--reason", "verification fixture, not real work", "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	for _, qid := range bulkQuestionIDs {
		_, _, code = execCLI(t, "answer", "question", qid, "--answer", "not applicable, verification fixture", "--contributor", "jc")
		require.Equal(t, 0, code)
	}

	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record feature checks")

	// Moment 7/8: sync, the done crossing itself. Four crossings across three
	// grains: sync observes BOTH tasks crossing on the merge, then rolls their
	// feature up over them and the epic up behind that, all in the same pass,
	// because the epic's last non-cancelled feature has just reached done
	// (artifact-schema.md, Epics States). Only the two task crossings are
	// observations: a PR represents a task, so the merge is theirs, and both
	// rollups rest on their children rather than on any branch.
	syncOut := checkGateReadout(t, dir, "sync", "sync", "--contributor", "jc")
	require.Contains(t, syncOut, "**Crossings recorded**")
	require.Contains(t, syncOut, "Crossings recorded: 4")
	require.Contains(t, syncOut, "2 ancestry, 2 rollup",
		"both tasks cross on the --no-ff merge; the feature and the epic roll up over them")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Features[soleFeatureID(t)].Status, "the merged feature crosses to done")

	// Moment 8/8: attest, the last of the four gates. It signs off one of the
	// feature's optional review gates, which is where attestation lives now
	// that the epic carries no gate block of its own. Scoped to this node and
	// this target (act.Attested), so the count is about code_review on this
	// feature, not about every unmet gate in the tree.
	attest := checkGateReadout(t, dir, "attest", "attest", "feature", cliFeatureKey,
		"--gate", "code_review", "--status", "pass", "--contributor", "jc")
	require.Contains(t, attest, "**Attestation recorded**")
	require.Contains(t, attest, "Gates still awaiting attestation on this node:")

	// The epic needed no ninth and tenth moment of its own: it was already
	// done at moment 8. There is no accept verb and no epic acceptance bar
	// (artifact-schema.md, "Done above a task is computed, and accept is
	// gone"); the human's remaining judgment moment at this grain is archive,
	// a deliberate close rather than a crossing.
	//
	// The tree also carries the bulk-0..9 epics, cancelled rather than
	// removed (Tiller has no delete), so this looks up form-engine by its
	// own key rather than assuming it is the tree's only epic.
	tree, err = store.Load(dir)
	require.NoError(t, err)
	var formEngine *store.Epic
	for _, e := range tree.Epics {
		if e.Key != nil && *e.Key == "form-engine" {
			formEngine = e
		}
	}
	require.NotNil(t, formEngine, "form-engine must still resolve by key")
	require.Equal(t, "done", formEngine.Status, "the epic rolled up the moment its one feature reached done")
	require.Equal(t, "rollup", formEngine.DoneBasis)
}
