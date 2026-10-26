package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// This file is intake's end-to-end net, built on e2e_amend_test.go's own
// conventions (task-9-brief.md): no hand-set fixtures, every field an
// assertion reads is written by the real command flow through execCLI, and
// the successful arc is followed by its refusal sibling in the same test,
// because "the refused approve wrote nothing and the retry succeeds" only
// means something read against a batch that would otherwise have applied.
//
// It replays the field case intake itself was built for
// (docs/spec/cli-surface.md), at the one-batch shape a later
// field case forced it to grow into (docs/spec/cli-surface.md):
// an Acme-shaped review lands on a tree that already has an epic and a
// feature in flight, and the explosion stages every op kind the design's
// mapping rules distinguish, plus the same-batch epic parenting the Acme
// first real intake needed and did not have (a new "hardening" epic and a
// feature under it, staged and approved as ONE amendment): new scope
// becomes a new epic, a statable contract under that same-batch epic and
// another under a pre-existing one both become features, a correction
// inside an existing feature's scope becomes a task on it, a declined
// judgment call becomes a question blocking the same-batch feature, each
// citing the line range it came from. The fixture's own key names
// (`hardening`, `csp-headers`, `csrf-token-rotation`, the null-zone parse
// guard, the image-parser question) are the design doc's own worked
// examples, not invented ones.

// intakeE2EReviewBody is the landed feedback artifact, five numbered
// findings whose line ranges the staged ops cite. Line numbers are
// load-bearing: the test's own --source flags name exact ranges into this
// string, so a reader can open the landed file and see the words behind
// every citation, the point of the whole feature.
const intakeE2EReviewBody = `# Acme field review

## Findings

1. No epic in the tree covers infrastructure hardening: CSP,
   secrets rotation, and dependency pinning have no home. This
   is new scope; recommend a "hardening" epic.

2. The Content-Security-Policy header is missing from every
   response the form engine serves. This is a standalone,
   contract-worthy feature under the new hardening epic:
   headers present, script-src enforced.

3. The CSRF token issued at login never rotates. This is a
   standalone, contract-worthy feature under the existing
   form-engine epic: token rotates on a fixed interval, the
   old token rejected once rotated.

4. The certification form loader throws on a null zone value instead of
   rejecting the record cleanly. Add a regression test guarding
   the null-zone parse path; a correction inside the loader's
   existing scope, not new scope.

5. Keep or trim the legacy image parsers? The review flags them
   as dead weight but does not recommend removal outright; that
   call is the team's to make once the CSP work lands.
`

const (
	intakeE2EEpicKey         = "hardening"
	intakeE2EEpicFeatureKey  = "hardening/csp-headers"
	intakeE2EFeatureKey      = "form-engine/csrf-token-rotation"
	intakeE2ETaskKey         = "form-engine/certification-form-loading/guard-null-zone"
	intakeE2EQuestion        = "Keep or trim the legacy image parsers?"
	intakeE2EQuestionTitle   = "Legacy image parsers"
	intakeE2EEpicCite        = ".tiller/feedback/opus-review.md#L5-L7"
	intakeE2EEpicFeatureCite = ".tiller/feedback/opus-review.md#L9-L12"
	intakeE2EFeatureCite     = ".tiller/feedback/opus-review.md#L14-L17"
	intakeE2ETaskCite        = ".tiller/feedback/opus-review.md#L19-L22"
	intakeE2EQuestCite       = ".tiller/feedback/opus-review.md#L24-L26"
)

// TestEndToEndIntakeExplodesFeedbackIntoTheTree drives the whole intake arc
// through the real CLI: land the document, stage the five-op explosion every
// op kind the mapping rules distinguish (including same-batch epic
// parenting: a feature staged under the epic this same amendment also
// stages, epic before feature), review the batch, approve it, prove every
// landed node and event carries its provenance, prove the routing walk
// reaches the epic the batch just minted through the FEATURE that same
// batch gave it, then run the honesty arc as the successful arc's refusal
// sibling: a second intake, the same tree, an edit under the staged
// batch's feet.
func TestEndToEndIntakeExplodesFeedbackIntoTheTree(t *testing.T) {
	// tiller guide carries no --contributor flag (guide.go's own doc comment), so it
	// resolves the acting contributor through TILLER_CONTRIBUTOR or .tiller/identity.yml.
	// This fixture writes neither by default (layIntakeE2EShape, like
	// initOwnedRepo, skips the real `tiller join`), the same gap
	// guide_test.go's own fixtures close with t.Setenv. Not load-bearing for
	// the routing assertion below AS WRITTEN (layIntakeE2EShape's own cancel
	// of the pre-existing feature's ready task already empties the
	// routableTask pool regardless of who guide resolves as owner, verified
	// by removing this line alone and re-running: still green), but kept
	// because it is the honest owner resolution a real, joined project
	// would give guide, and a future edit that reintroduces a ready/
	// in_progress task in this fixture should see the real routableTask
	// scoping rather than the silent empty-owner fallback that would
	// otherwise mask it (routableTask's own doc comment: "an empty owner
	// means... no task can be shown to be the reader's").
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	dir := layIntakeE2EShape(t)

	// --- land the document, stage the explosion ---
	src := writeFeedbackDoc(t, "opus-review.md", intakeE2EReviewBody)
	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, aid)

	// Every op cites the finding it traces to, in the order the mapping rules
	// state them: new scope, a statable contract under that SAME-BATCH epic
	// (epic staged first is what makes this legal), a statable contract
	// under the pre-existing form-engine epic, a correction on the
	// pre-existing feature, a declined judgment call blocking the same-batch
	// feature. Each stage call also carries a key_absent guard, the same
	// optional stale-proof revise's own preconditions give a reviewer
	// (except add-question, which stages no key to guard).
	for _, argv := range [][]string{
		{"amend", aid, "--add-epic", intakeE2EEpicKey, "--title", "Infrastructure hardening",
			"--precondition", "key_absent=" + intakeE2EEpicKey,
			"--source", intakeE2EEpicCite, "--contributor", "jc"},
		{"amend", aid, "--add-feature", intakeE2EEpicFeatureKey, "--change-type", "create",
			"--title", "Content-Security-Policy headers",
			"--precondition", "key_absent=" + intakeE2EEpicFeatureKey,
			"--source", intakeE2EEpicFeatureCite, "--contributor", "jc"},
		{"amend", aid, "--add-feature", intakeE2EFeatureKey, "--change-type", "create",
			"--title", "CSRF token rotation",
			"--precondition", "key_absent=" + intakeE2EFeatureKey,
			"--source", intakeE2EFeatureCite, "--contributor", "jc"},
		{"amend", aid, "--add-task", intakeE2ETaskKey, "--title", "Guard the null-zone parse path",
			"--precondition", "key_absent=" + intakeE2ETaskKey,
			"--source", intakeE2ETaskCite, "--contributor", "jc"},
		{"amend", aid, "--add-question", intakeE2EQuestion, "--title", intakeE2EQuestionTitle,
			"--blocks", intakeE2EEpicFeatureKey, "--source", intakeE2EQuestCite, "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	// --- the review surfaces, before anything is applied ---
	//
	// Human and JSON both, per the brief: headlines carry each op's citation
	// as the trailing " · " suffix (renderOperationHeadline), and validity
	// covers both tiers intake's source honesty adds: the per-op key_absent
	// preconditions AND the amendment-level Source's own live read.
	human, stderr, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, human, "status: proposed")
	require.Contains(t, human, "0. add-epic "+intakeE2EEpicKey+`: "Infrastructure hardening" · `+intakeE2EEpicCite)
	require.Contains(t, human, "1. add-feature "+intakeE2EEpicFeatureKey+" (change_type: create) · "+intakeE2EEpicFeatureCite)
	require.Contains(t, human, "2. add-feature "+intakeE2EFeatureKey+" (change_type: create) · "+intakeE2EFeatureCite)
	require.Contains(t, human, `3. add-task `+intakeE2ETaskKey+`: "Guard the null-zone parse path" · `+intakeE2ETaskCite)
	require.Contains(t, human, `4. add-question `+intakeE2EQuestionTitle+`: "`+intakeE2EQuestion+`" · `+intakeE2EQuestCite)
	require.Contains(t, human, "live: [valid], actual git:", "the amendment's own Source reads valid against the freshly landed artifact")
	require.Equal(t, 5, strings.Count(human, "[valid]"),
		"the four staged key_absent preconditions (epic, both features, task) plus the amendment-level Source, all valid against the tree as staged")
	require.NotContains(t, human, "[stale]")
	require.NotContains(t, human, "[unknown]")

	jsonOut, stderr, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, jsonOut)
	ops, ok := data["operations"].([]any)
	require.True(t, ok)
	require.Len(t, ops, 5)
	questionOp, ok := ops[4].(map[string]any)
	require.True(t, ok)
	require.Equal(t, intakeE2EQuestionTitle, questionOp["title"])
	require.Equal(t, intakeE2EQuestion, questionOp["statement"])
	source, ok := data["source"].(map[string]any)
	require.True(t, ok, "the amendment carries its intake-minted source: %s", jsonOut)
	require.Equal(t, "valid", source["status"])
	recordedRevision, _ := source["revision"].(string)
	require.Regexp(t, `^git:[0-9a-f]{40}$`, recordedRevision)

	// The amend seam's own readout surveys the same batch: five ops staged,
	// and only the four key_absent guards are validity-bearing (a question
	// carries none).
	amendReadout, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, amendReadout, "Operations staged: 5")
	require.Contains(t, amendReadout, "Preconditions: 4 valid, 0 stale")

	// --- the apply ---
	//
	// --json gives back mutate.ApproveResult's own Applied list, index and op
	// kind per entry: the direct way to pin "AppliedOperation order mirrors
	// staged order" at the real CLI surface, without guessing at ids approve
	// itself has not minted yet.
	appliedOut, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	appliedData := jsonData(t, appliedOut)
	require.Equal(t, "approved", appliedData["status"])
	appliedOps, ok := appliedData["applied"].([]any)
	require.True(t, ok)
	require.Len(t, appliedOps, 5)

	wantOrder := []string{"add-epic", "add-feature", "add-feature", "add-task", "add-question"}
	appliedIDs := make([]string, len(appliedOps))
	for i, want := range wantOrder {
		op, ok := appliedOps[i].(map[string]any)
		require.True(t, ok)
		require.Equal(t, float64(i), op["index"], "AppliedOperation order mirrors staged order")
		require.Equal(t, want, op["op"], "AppliedOperation order mirrors staged order")
		id, _ := op["id"].(string)
		require.NotEmpty(t, id)
		appliedIDs[i] = id
	}
	epicID, epicFeatureID, existingEpicFeatureID, taskID, questionID :=
		appliedIDs[0], appliedIDs[1], appliedIDs[2], appliedIDs[3], appliedIDs[4]

	tree := loadCwdTree(t)
	require.Equal(t, epicID, epicIDByKey(t, tree, intakeE2EEpicKey))
	require.Equal(t, epicFeatureID, featureIDByKey(t, intakeE2EEpicFeatureKey))
	require.Equal(t, existingEpicFeatureID, featureIDByKey(t, intakeE2EFeatureKey))
	require.Equal(t, taskID, taskIDByKey(t, intakeE2ETaskKey))
	question := questionByStatement(t, tree, intakeE2EQuestion)
	require.Equal(t, questionID, question.ID)

	// --- every landed node carries its citation as its own provenance ---
	//
	// Epic and feature both carry a node-level `source` field
	// (artifact-schema.md), and materialize wires the staged op's Source onto
	// it for both (amendment_approve.go's materialize, mirroring add-epic's
	// own wiring onto add-feature's). A task carries no such field at all:
	// task.yml's own schema names no `source` key, so a task op's citation
	// stays exactly where every op's citation stays regardless of kind: the
	// amendment's own operations list, asserted below through `show
	// amendment --json` rather than a node field that does not exist.
	require.Equal(t, "draft", tree.Epics[epicID].Status)
	require.Equal(t, intakeE2EEpicCite, tree.Epics[epicID].Source,
		"the epic materialized with its op's citation as its own node source")

	// The key assertion for same-batch epic parenting: the feature this
	// batch staged under the epic this SAME batch also staged reloads with
	// its EpicKey naming that epic, not a pre-existing one.
	require.Equal(t, "draft", tree.Features[epicFeatureID].Status)
	require.Equal(t, "hardening", tree.Features[epicFeatureID].EpicKey,
		"the feature landed under the SAME-BATCH epic this amendment also staged, staged epic-first")
	require.Equal(t, intakeE2EEpicFeatureCite, tree.Features[epicFeatureID].Source,
		"the feature carries its own op's citation as its own node source")

	require.Equal(t, "draft", tree.Features[existingEpicFeatureID].Status)
	require.Equal(t, "form-engine", tree.Features[existingEpicFeatureID].EpicKey,
		"the other feature landed under the PRE-EXISTING form-engine epic, resolved the ordinary way")
	require.Equal(t, intakeE2EFeatureCite, tree.Features[existingEpicFeatureID].Source,
		"the feature carries its own op's citation as its own node source")

	preExistingFeatureID := featureIDByKey(t, "form-engine/certification-form-loading")
	require.Equal(t, "proposed", tree.Tasks[taskID].Status)
	require.Equal(t, preExistingFeatureID, tree.Tasks[taskID].FeatureID,
		"the task landed on the PRE-EXISTING feature the mapping rules name: task grain stays closed to same-batch parenting")

	freshOut, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	freshOps, _ := jsonData(t, freshOut)["operations"].([]any)
	require.Len(t, freshOps, 5)
	taskOp, ok := freshOps[3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, intakeE2ETaskCite, taskOp["source"],
		"the task op's own citation persists on the amendment's operations list, the record a task's own missing source field cannot carry")

	// --- the question's blocks resolved to the materialized id, of the
	// SAME-BATCH staged feature it named ---
	require.Equal(t, "open", question.Status)
	require.Equal(t, intakeE2EQuestionTitle, question.Title)
	require.Equal(t, intakeE2EQuestion, question.Statement)
	require.Equal(t, intakeE2EQuestCite, question.Source)
	require.Equal(t, []string{epicFeatureID}, question.Blocks,
		"the same-batch staged key resolved to the id materialize just minted for it, add-question's own reach into the batch (unchanged by same-batch feature parenting)")

	// --- every event amendment-tagged ---
	addEpicEvent := soleAmendmentEvent(t, tree, aid, "add.epic")
	require.Equal(t, intakeE2EEpicKey, addEpicEvent.After["key"])
	require.Equal(t, "approve amendment "+aid, addEpicEvent.Command)

	// Two add-feature ops staged on this one amendment (the new shape this
	// task adds), so soleAmendmentEvent's own exactly-one assertion no
	// longer fits this op kind; amendmentEventsByType is its unenforced
	// sibling, added beside it rather than loosening it, since every OTHER
	// op kind in this arc still gets exactly one event and still goes
	// through soleAmendmentEvent unchanged.
	addFeatureEvents := amendmentEventsByType(t, tree, aid, "add.feature")
	require.Len(t, addFeatureEvents, 2, "one add.feature event per staged add-feature op")
	addFeatureByKey := map[string]events.Event{}
	for _, e := range addFeatureEvents {
		key, _ := e.After["key"].(string)
		addFeatureByKey[key] = e
		require.Equal(t, "approve amendment "+aid, e.Command)
	}
	require.Contains(t, addFeatureByKey, intakeE2EEpicFeatureKey)
	require.Contains(t, addFeatureByKey, intakeE2EFeatureKey)

	addTaskEvent := soleAmendmentEvent(t, tree, aid, "add.task")
	require.Equal(t, intakeE2ETaskKey, addTaskEvent.After["key"])
	require.Equal(t, "approve amendment "+aid, addTaskEvent.Command)

	addQuestionEvent := soleAmendmentEvent(t, tree, aid, "add.question")
	require.Equal(t, "approve amendment "+aid, addQuestionEvent.Command)

	// --- the route walks the new epic, through the feature THIS SAME BATCH
	// gave it ---
	//
	// tiller guide's own per-seam text is deliberately generic, so its
	// suggestion alone cannot NAME the node it routed to. What it shares with
	// `readout orient` is the ranked list (views.NextOf, one computation both
	// surfaces call), so the two reads together are one proof: guide names the
	// skill, and both print the list that names the nodes.
	//
	// The new epic already carries its own feature, hardening/csp-headers,
	// staged in this same batch and materialized alongside it: no extra live
	// `tiller add feature` step is needed to prove the epic intake minted is
	// not a routing dead end, the whole point of same-batch parenting. A
	// soft-order ranking still keeps the frontier's pick unambiguous, since
	// this batch also materialized a second fresh draft feature
	// (form-engine/csrf-token-rotation) under the pre-existing epic, and
	// without a recorded order between the two epics the pick would be an
	// implementation detail of flow order this test would otherwise be
	// guessing at.
	_, stderr, code = execCLI(t, "order", "epic", intakeE2EEpicKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	// The minted epic lands draft, so its decomposition gate is the human's
	// move (issue #93) and both surfaces list it. Guide suggests the skill
	// for the highest agent move beside it, specifying the draft feature
	// under the pre-existing approved epic, and never a feature under the
	// draft epic.
	guideJSON, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, "specify", jsonData(t, guideJSON)["skill"])
	require.Contains(t, guideJSON, `"command": "tiller approve epic `+intakeE2EEpicKey+`"`)
	require.NotContains(t, guideJSON, `"key": "`+intakeE2EEpicFeatureKey+`"`,
		"nothing under the draft epic is a spec move before the epic is approved")

	orient, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, orient)
	require.Contains(t, next, "You: approve epic "+intakeE2EEpicKey+" ("+epicID+").",
		"the list reaches the epic this batch minted")
	require.NotContains(t, next, "specify "+intakeE2EEpicFeatureKey,
		"nothing under the draft epic is a spec move before the epic is approved")

	// The human's approve clears that gate, and the frontier reaches into
	// the minted epic through the feature this same batch staged under it.
	_, stderr, code = execCLI(t, "approve", "epic", intakeE2EEpicKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	guideJSON, _, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, "specify", jsonData(t, guideJSON)["skill"])

	orient, _, code = execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next = splitAtNext(t, orient)
	require.Contains(t, next, "Agent: specify "+intakeE2EEpicFeatureKey+" ("+epicFeatureID+").",
		"the frontier walk reaches into the epic this batch minted through the feature that same batch staged under it, not a node added by a separate live step")

	// --- the honesty arc, as the successful arc's refusal sibling ---
	//
	// A second intake, landing on the SAME tree the explosion above just
	// reshaped, not a fresh fixture: the field case is a second review
	// arriving after the first one already landed. One op, sourced, then the
	// artifact moves under the staged batch's feet.
	followup := writeFeedbackDoc(t, "followup-note.md", "1. Tighten the request timeout on form\n   submission; it currently has none.\n")
	out, stderr, code = execCLI(t, "intake", followup, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid2, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, aid2)

	_, stderr, code = execCLI(t, "amend", aid2, "--add-task",
		"form-engine/certification-form-loading/tighten-request-timeout",
		"--title", "Tighten the request timeout",
		"--source", ".tiller/feedback/followup-note.md#L1-L2", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	landedPath := filepath.Join(dir, ".tiller", "feedback", "followup-note.md")
	original, err := os.ReadFile(landedPath)
	require.NoError(t, err)

	// Fresh: the source reads valid.
	showJSON, _, code := execCLI(t, "show", "amendment", aid2, "--json")
	require.Equal(t, 0, code)
	src2, _ := jsonData(t, showJSON)["source"].(map[string]any)
	require.Equal(t, "valid", src2["status"])
	recorded2, _ := src2["revision"].(string)
	require.NotEmpty(t, recorded2)

	// Edited: show says stale, naming the live hash.
	require.NoError(t, os.WriteFile(landedPath, []byte("1. Tighten the request timeout on form\n   submission; EDITED after review.\n"), 0o644))
	showJSON, _, code = execCLI(t, "show", "amendment", aid2, "--json")
	require.Equal(t, 0, code)
	src2, _ = jsonData(t, showJSON)["source"].(map[string]any)
	require.Equal(t, "stale", src2["status"])
	live2, _ := src2["actual"].(string)
	require.NotEmpty(t, live2)
	require.NotEqual(t, recorded2, live2)

	// Approve refuses, naming both hashes, exit 2 (a tree fact, not a staged
	// precondition, cli-surface.md's own approve-amendment paragraph).
	refused, stderr, code := execCLI(t, "approve", "amendment", aid2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, stderr)
	require.Contains(t, refused, `"reason": "source_changed"`)
	require.Contains(t, refused, recorded2)
	require.Contains(t, refused, live2)

	// A refused approve wrote nothing: the amendment stays proposed.
	require.Equal(t, "proposed", loadCwdTree(t).Amendments[aid2].Status)

	// Restored: approve lands.
	require.NoError(t, os.WriteFile(landedPath, original, 0o644))
	landed, stderr, code := execCLI(t, "approve", "amendment", aid2, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "approved", jsonData(t, landed)["status"])
}

// layIntakeE2EShape drives a git-backed project to the fixture's own state
// through the real verbs: an approved epic, a feature specced and approved
// (ready, not draft, not specced) so it competes for neither the plan gate
// nor the frontier once the explosion lands its own draft structure beside
// it. It returns the repo root and leaves the process in it.
func layIntakeE2EShape(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(ownedManifest), 0o644))
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")

	for _, argv := range [][]string{
		{"add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc"},
		{"approve", "epic", "form-engine", "--contributor", "jc"},
		{"add", "feature", "form-engine/certification-form-loading", "--title", "Certification form loading",
			"--change-type", "create", "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")

	for _, argv := range [][]string{
		{"spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc"},
		{"approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}
	require.Equal(t, "ready", loadCwdTree(t).Features[featureIDByKey(t, "form-engine/certification-form-loading")].Status,
		"ready, past both the plan gate and the frontier, so the explosion's own draft feature is the frontier's only candidate")

	// Approving the feature's plan also rides its one initial task to ready
	// (artifact-schema.md, Features: States), and a ready task in hand
	// outranks the frontier on every surface (the reader's own work ranks
	// first, ranked-next design decision 4). Left alone, that pre-existing task, not
	// the epic this arc mints, would be what `tiller guide` and `readout
	// orient` name at the end, which a live run against the real binary
	// caught: the same fixture minus this cancel routed to
	// transcribe-the-schema, not to the new epic's own feature. Cancelling
	// it here (unrelated to anything the explosion below stages or asserts)
	// is what makes the routing proof at the end of this test actually about
	// the new epic rather than a coincidence of an empty frontier.
	_, stderr, code := execCLI(t, "cancel", "task", soleTaskID(t),
		"--reason", "not needed for the intake routing arc", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	return dir
}

// epicIDByKey reads one epic's minted id off a loaded tree by its canonical
// key, the epic-grain companion to featureIDByKey/taskIDByKey.
func epicIDByKey(t *testing.T, tree *store.Tree, key string) string {
	t.Helper()
	for id, e := range tree.Epics {
		if e.Key != nil && *e.Key == key {
			return id
		}
	}
	t.Fatalf("epic %q not found", key)
	return ""
}

// questionByStatement reads one question off a loaded tree by its statement,
// the ledger's own idempotency key (question.go): a question carries no key
// field at all, so a statement is the only handle a test has before it knows
// the minted id.
func questionByStatement(t *testing.T, tree *store.Tree, statement string) *store.Question {
	t.Helper()
	for _, q := range tree.Questions {
		if q.Statement == statement {
			return q
		}
	}
	t.Fatalf("question %q not found", statement)
	return nil
}

// amendmentEventsByType is soleAmendmentEvent's own filter (e2e_amend_test.go)
// without the exactly-one assertion, for an operation kind a batch stages
// more than once: this arc's two add-feature ops, one per epic parent, now
// that feature grain can name a same-batch staged epic. soleAmendmentEvent
// itself is untouched and still pins every other op kind in this arc to
// exactly one event; this is a sibling for the one kind that no longer fits
// its "sole" assumption, not a loosening of it.
//
// Like soleAmendmentEvent, every event this returns must also carry the
// amendment on its own After["amendment"] (#50): Targets alone proves
// linkage, not provenance, and the two are meant to never disagree (this
// file's own materialize loop sets both in the same breath).
func amendmentEventsByType(t *testing.T, tree *store.Tree, amendmentID, typ string) []events.Event {
	t.Helper()
	all, err := events.List(tree)
	require.NoError(t, err)
	var found []events.Event
	for _, e := range all {
		if e.Type != typ {
			continue
		}
		for _, target := range e.Targets {
			if target == amendmentID {
				found = append(found, e)
				break
			}
		}
	}
	for _, e := range found {
		require.Equal(t, amendmentID, e.After["amendment"],
			"every event an apply writes carries the amendment in its after block too")
	}
	return found
}
