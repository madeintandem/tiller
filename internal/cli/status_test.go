package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// readyManifest is a complete manifest (contributors and kind set) used to drive the
// ready path of `check readiness`.
const readyManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Ready Project
  key: ready
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
inputs:
  docs:
    - design/
  current_state: []
  code: []
`

// Project status reports the contributor roster without selecting an actor.
func TestStatusSummaryContributorRoster(t *testing.T) {

	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield")
	require.Equal(t, 0, code)
	// init is a pure scaffold now; a human registers jc, then join records
	// identity.
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, []any{map[string]any{"id": "jc", "name": "JC Grubbs"}}, data["contributors"])
	require.NotContains(t, data, "owners")
	require.NotContains(t, data, "actor")
	counts := data["counts"].(map[string]any)
	for _, noun := range []string{"epics", "features", "tasks", "questions", "bugs", "chores", "tweaks"} {
		require.Zero(t, counts[noun], noun)
	}

	out, _, code = execCLI(t, "status")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Contributors: jc\n")
	require.NotContains(t, out, "Owners:")
}

// TestStatusSummaryReportsUncommittedTillerState pins the uncommitted-state
// status line (issue #44): the field case was two attest commands that wrote
// feature.yml plus two event files, and the state sat uncommitted for a day
// with no surface naming it. The line is a live git read scoped to .tiller/,
// so it renders only once something under it is actually dirty, and the
// clean-tree case carries no uncommitted-state field.
func TestStatusSummaryReportsUncommittedTillerState(t *testing.T) {
	initOwnedRepo(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Committed and clean: no line, no JSON key.
	out, _, code := execCLI(t, "status")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "uncommitted tiller state")

	jout, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, jout, "uncommitted_state")

	// A modified tracked file and a brand-new untracked file under .tiller/,
	// mirroring the field case: tiller wrote feature.yml plus two event
	// files without a commit following either write.
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".tiller", "tiller.yml"),
		[]byte(ownedManifest+"# edited, uncommitted\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, ".tiller", "events"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".tiller", "events", "E-000001.yml"),
		[]byte("kind: attest\n"), 0o644))

	out, _, code = execCLI(t, "status")
	require.Equal(t, 0, code)
	require.Contains(t, out, "uncommitted tiller state: 2 file(s) under .tiller/; commit when convenient")

	jout, _, code = execCLI(t, "status", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, "\"uncommitted_state\": [")
	require.Contains(t, jout, ".tiller/events/E-000001.yml")
	require.Contains(t, jout, ".tiller/tiller.yml")

	// A dirty file OUTSIDE .tiller/ never counts, the scoping DirtyUnder's
	// own unit test already pins at the vcs layer; this confirms the CLI
	// wiring inherits that scope rather than falling back to the whole-tree
	// IsDirty question.
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "README.md"), []byte("unrelated edit\n"), 0o644))
	out, _, code = execCLI(t, "status")
	require.Equal(t, 0, code)
	require.Contains(t, out, "uncommitted tiller state: 2 file(s) under .tiller/; commit when convenient",
		"a dirty file outside .tiller/ does not change the count")
}

// TestStatusOutsideRepoExits3 confirms the project summary refuses with an
// environment error (exit 3) outside a Tiller repo.
func TestStatusOutsideRepoExits3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"code\": 3")
	require.Contains(t, out, store.ReasonNotARepo)
}

// TestStatusRefusesAVersionOneTree pins the schema_version refusal at the CLI
// surface (task-5 fix round 1, minor 6): internal/store's own tests cover
// LoadManifest directly, but nothing before this asserted the exit code or the
// --json envelope's reason for a real command, only a hand-run transcript in a
// report. status is the read-only command every reason above was verified
// against by hand; this makes one of those checks a real test.
func TestStatusRefusesAVersionOneTree(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"),
		[]byte("schema_version: 1\nschema_epoch: initial\n"), 0o644))

	out, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"code\": 3")
	require.Contains(t, out, store.ReasonSchemaVersionUnsupported)
	require.Contains(t, out, "remove .tiller/")
}

// TestCheckReadinessNamesGapsOnFreshRepo confirms readiness is a red gate
// (exit 1) on a freshly inited, un-joined repo and names both gaps: init is a
// pure scaffold now, so the roster is empty and the missing-contributors gap points
// the human at `tiller add contributor`, alongside the empty design/.
func TestCheckReadinessNamesGapsOnFreshRepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 1, code, "an unready project is a red gate")
	require.Contains(t, out, "\"ready\": false")
	require.Contains(t, out, store.ReasonMissingDesign)
	require.Contains(t, out, store.ReasonMissingContributors, "a fresh, un-joined repo has an empty roster")
	require.Contains(t, out, "tiller add contributor", "the missing-owners gap points at add contributor, the only command that registers one")
}

// TestCheckReadinessReady confirms a coherent project passes the gate (exit 0).
func TestCheckReadinessReady(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(filepath.Join(tillerDir, "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), []byte(readyManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "design", "00-index.md"), []byte("# index\n"), 0o644))

	out, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ready\": true")
}

// A stale orientation stamp is an exit-1 readiness gap at the CLI.
func TestCheckReadinessNamesStaleOrientationGap(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(filepath.Join(tillerDir, "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), []byte(readyManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "design", "00-index.md"), []byte("# index\n"), 0o644))
	current := strconv.Itoa(gen.OrientationGeneration())
	aged := strings.Replace(gen.Orientation(), "orientation generation "+current, "orientation generation 1", 1)
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "orientation.md"), []byte(aged), 0o644))

	out, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 1, code, "a stale orientation generation is a red gate")
	require.Contains(t, out, "\"ready\": false")
}

// TestCheckReadinessCurrentOrientationHasNoGap confirms an orientation.md
// already stamped with the binary's generation carries no gap.
func TestCheckReadinessCurrentOrientationHasNoGap(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(filepath.Join(tillerDir, "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), []byte(readyManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "design", "00-index.md"), []byte("# index\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "orientation.md"), []byte(gen.Orientation()), 0o644))

	out, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ready\": true")
}

// TestCheckReadinessOutsideRepoExits3 confirms readiness refuses outside a repo.
func TestCheckReadinessOutsideRepoExits3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 3, code)
}

// layApprovedFeatureWithTask drives a git-backed project to an approved feature
// plan: the epic approved, the spec and contract and one task written, the
// feature specced, and the plan gate run, which pins the feature's spec/eval
// pair and the task's own task.md as accepted_against. It returns the repo root.
func layApprovedFeatureWithTask(t *testing.T) string {
	t.Helper()
	dir := initGatedGitRepo(t, "true")
	for _, argv := range [][]string{
		{"add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc"},
		{"add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc"},
		{"approve", "epic", "form-engine", "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	for _, argv := range [][]string{
		{"spec", "feature", cliFeatureKey, "--contributor", "jc"},
		{"approve", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}
	return dir
}

// TestStatusReportsActiveBasisDriftAtBothGrains pins the reporting half of the
// active-basis rule. cli-surface.md ("Specifying a feature") says status, serve,
// and drift report active_basis while start, check, and submit refuse it; the
// refusals landed in task 7 and the status report is this task's.
//
// The bar is agreement with the refusal, not merely the presence of a line: each
// leg below reads status and then runs the verb the guard sits in front of, so a
// node status calls clean and start refuses would fail here.
func TestStatusReportsActiveBasisDriftAtBothGrains(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	featDir := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	tid := soleTaskID(t)

	// Clean: nothing drifted, and the verb the guard fronts agrees.
	out, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "active_basis")

	// A bare working-tree edit of feature.md, no commit.
	require.NoError(t, os.WriteFile(filepath.Join(featDir, "feature.md"),
		[]byte("# Certification form loading\n\nRevised: scope changed since acceptance.\n"), 0o644))

	out, _, code = execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code, "status reports, it never gates")
	require.Contains(t, out, "active_basis: feature.md")
	require.Contains(t, out, "--refresh-basis")

	// The --json side grows one additive field inside data, the envelope's three
	// top-level keys untouched (cli-surface.md, "The JSON envelope").
	jout, _, code := execCLI(t, "status", "feature", cliFeatureKey, "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, "\"ok\": true")
	require.Contains(t, jout, "\"active_basis\": [")
	require.Contains(t, jout, "\"feature.md\"")

	blocked, _, code := execCLI(t, "start", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, blocked, "\"reason\": \"active_basis\"")

	// The same drift reaches the TASK's status, because the task's own guard
	// compares its parent feature's pins before its own (mutate.guardTask): a
	// task reading clean here while start refuses it is exactly the disagreement
	// this test exists to prevent.
	out, _, code = execCLI(t, "status", "task", tid)
	require.Equal(t, 0, code)
	require.Contains(t, out, "active_basis: feature.md")
	// The grain assertion has to be about the grain. An earlier version of this
	// line asserted the substring "feature", which "feature.md" one assertion up
	// already satisfies, so it pinned nothing (task-9-findings-round-1.md,
	// Important 3). What is load-bearing is that a task's line names the FEATURE
	// whose pin moved and hands over that feature's command: refreshing the
	// task's own pin would not clear a feature's drift.
	require.Contains(t, out, "on feature "+cliFeatureKey,
		"a task's drift line names the grain that owns the pin")
	require.Contains(t, out, "tiller approve feature "+cliFeatureKey+" --refresh-basis")
	require.NotContains(t, out, "tiller approve task ",
		"the remedy addresses the feature, since that is the pin that moved")

	blocked, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, blocked, "\"reason\": \"active_basis\"")

	// Refreshing the feature's pin clears both readings.
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	out, _, code = execCLI(t, "status", "task", tid)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "active_basis")

	// A task's own task.md is the other pin, and status names that one at task
	// grain.
	taskMD := filepath.Join(featDir, store.TaskDirName, "transcribe-the-schema", store.TaskProseFile)
	require.NoError(t, os.WriteFile(taskMD, []byte("# Transcribe the schema\n\n## Approach\n\nRevised after approval.\n"), 0o644))

	out, _, code = execCLI(t, "status", "task", tid)
	require.Equal(t, 0, code)
	require.Contains(t, out, "active_basis: "+store.TaskProseFile)

	// The feature above it is untouched by a task's own spec edit, and its
	// status says so.
	out, _, code = execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "active_basis")
}

// TestStatusReportsADeletedPinAsDriftNamingARestore pins the deleted-file arm at
// the reporting surface. A pinned file that is gone is drift (mutate.checkBasis),
// so status must report it, and the move it names has to be one that works:
// --refresh-basis re-records from the working tree and cannot hash a file that is
// not there.
func TestStatusReportsADeletedPinAsDriftNamingARestore(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	featDir := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	require.NoError(t, os.Remove(filepath.Join(featDir, "feature.eval.md")))

	out, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, out, "active_basis: feature.eval.md")
	require.Contains(t, out, "restore")
}

// driveFeatureToAwaitingCheckCLI drives a git-backed feature to the
// awaiting-check rollup phase: active, its one task crossed to done by an
// observed local merge, and the required feature-grain evals gate
// (initGatedGitRepo's manifest declares it on: [feature, task]) left unrecorded
// because `check feature` is deliberately never run. It leaves the checkout on
// main.
func driveFeatureToAwaitingCheckCLI(t *testing.T, dir string) {
	t.Helper()
	driveFeatureToReadyCLI(t)

	tid := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}

	// The feature's recorded branch merges locally; sync observes it by
	// ancestry and crosses the task to done. `check feature` never runs, so the
	// required evals gate stays unrecorded on the feature itself, the state
	// awaiting-check reports on.
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	_, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "done", taskStatusCLI(t, tid), "the task crossed on the observed merge")
}

// The last task's real merge leaves related feature acceptance and sync work,
// and status and guide must not misroute that closeout to unrelated planning.
func TestStatusAndGuideContinueTaskGrainPostMergeCloseout(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToAwaitingCheckCLI(t, dir)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	raw, stderr, code := execCLI(t, "status", "feature", cliFeatureKey, "--json")
	require.Equal(t, 0, code, stderr)
	status := jsonData(t, raw)
	require.Equal(t, "active", status["status"])
	require.Equal(t, "check", status["workflow"].(map[string]any)["action"])
	raw, stderr, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "execute", jsonData(t, raw)["skill"])

	_, stderr, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	raw, stderr, code = execCLI(t, "status", "feature", cliFeatureKey, "--json")
	require.Equal(t, 0, code, stderr)
	status = jsonData(t, raw)
	require.Equal(t, "sync", status["workflow"].(map[string]any)["action"])
	require.NotContains(t, status["legal_next"], "sync")
	_, stderr, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "done", soleFeatureStatus(t))

	// Adding and approving more authorized work under the recorded rollup
	// does not require waiting for a later sync to reopen the feature first.
	const lateKey = cliFeatureKey + "/late-task"
	_, stderr, code = execCLI(t, "add", "task", lateKey, "--title", "Late task", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), store.TaskDirName, "late-task", store.TaskProseFile), []byte("# Late task\n\n## Approach\n\nImplement the newly approved behavior.\n"), 0o644))
	_, stderr, code = execCLI(t, "approve", "task", lateKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "done", soleFeatureStatus(t), "approval does not reopen the recorded rollup")
	_, stderr, code = execCLI(t, "claim", "task", lateKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	raw, stderr, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "execute", jsonData(t, raw)["skill"])
	_, stderr, code = execCLI(t, "start", "task", lateKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	raw, stderr, code = execCLI(t, "status", "task", lateKey, "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "in_progress", jsonData(t, raw)["status"])
}

// TestStatusIsSilentAboutBasisWhereThereIsNoPin covers the two nouns that carry
// no accepted basis at all: an epic never has one, and a feature before its plan
// gate has nothing pinned yet, which is a different fact from a pin that matches.
func TestStatusIsSilentAboutBasisWhereThereIsNoPin(t *testing.T) {
	initGatedGitRepo(t, "true")
	for _, argv := range [][]string{
		{"add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc"},
		{"add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}

	for _, argv := range [][]string{
		{"status", "epic", "form-engine"},
		{"status", "feature", cliFeatureKey},
	} {
		out, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		require.NotContainsf(t, out, "active_basis", "%v", argv)
	}
}

func TestStatusQuestionRelationshipsAreBidirectional(t *testing.T) {
	layReadTree(t)
	featureID := soleFeatureID(t)
	statement := "Which schema dialect should every generated form use across the complete validation pipeline?"
	const title = "Choose the schema dialect"

	added, _, code := execCLI(t, "add", "question", statement,
		"--title", title, "--blocks", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	questionID := extractID(t, added, "Q-")

	featureOut, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, featureOut, `  blocked by questions: `+questionID+` ("`+title+`")`)
	require.NotContains(t, featureOut, statement,
		"a reverse relationship is a scan row with the short title, never the full statement")

	questionOut, _, code := execCLI(t, "status", "question", questionID)
	require.Equal(t, 0, code)
	require.Contains(t, questionOut, `question `+questionID+`: "`+title+`"`)
	require.Contains(t, questionOut, "  statement: "+statement,
		"the full statement belongs on the question subject's own detail")
	require.Contains(t, questionOut, "  blocks: "+cliFeatureKey+" ("+featureID+")")

	_, _, code = execCLI(t, "answer", "question", questionID,
		"--answer", "Use JSON Schema 2020-12.", "--contributor", "jc")
	require.Equal(t, 0, code)

	featureOut, _, code = execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.NotContains(t, featureOut, "blocked by questions:")
	require.NotContains(t, featureOut, questionID,
		"an answered question is no longer an operative reverse hold")

	questionOut, _, code = execCLI(t, "status", "question", questionID)
	require.Equal(t, 0, code)
	require.Contains(t, questionOut, "  state: answered")
	require.Contains(t, questionOut, "  statement: "+statement)
	require.Contains(t, questionOut, "  blocks: "+cliFeatureKey+" ("+featureID+")",
		"answering never erases the question's durable forward relationship")
}
