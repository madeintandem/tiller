package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// This file is the amendment's end-to-end net, built on e2e_done_test.go's
// conventions and holding its one rule: no hand-set fixtures. Every field the
// assertions read (the feature's accepted basis, the task's status at check, the
// supersession pair, the re-pinned basis, the staled gate) is written by the real
// command flow through execCLI, so a build that stopped producing one of them
// fails here rather than agreeing with a fixture that asserted it into place.
//
// It replays the field case the amendment surface was built for (the Lantern
// reshape, docs/spec/cli-surface.md): an approved feature
// is under way, one task sits at check, and a design change arrives that
// re-scopes the feature and replaces that task with a different one. The reshape
// runs through one amendment and one human gate, and the questions the tree has
// to answer afterwards are what changed, what replaced what, and why.

// amendE2EForwardFeature is the second feature in the fixture: a draft that
// nothing in the reshape touches. It stands for the work the frontier moves on
// to, which is what makes "Route moved on" after the apply an observable claim
// rather than the absence of one.
const amendE2EForwardFeature = "form-engine/table-backed-forms"

// amendE2EReplacementKey is the task the reshape adds, named by its canonical
// key while it is only staged: an add operation's key is the only handle it has
// until approve materializes it and mints its id.
const amendE2EReplacementKey = "form-engine/certification-form-loading/table-aware-rendering"

// amendE2EDesignDoc is the design document the reshape answers. It is a real
// file in the fixture repo, because --triggered-by and --amends-design record
// paths a human is expected to be able to open.
const amendE2EDesignDoc = "design/04-form-engine.md"

// TestEndToEndAmendmentReplaysTheLanternReshape drives the whole amendment arc
// through the CLI on one tree: the Lantern shape, the staging, the review surfaces,
// the stale refusal, the apply, and the questions the tree answers afterwards.
//
// It is one test rather than six because the claims are about one arc. The stale
// refusal only means something against a batch that would otherwise have
// applied; "the queue row is gone" only means something against a row that was
// there; and the supersession pair, the re-pinned basis, and the staled gate are
// three halves of a single apply that either happened atomically or did not
// happen. Splitting them would need the same fixture driven five more times and
// would still not say that one approve did all of it.
//
// The order below is the arc's own, with one inversion worth naming: the stale
// arm runs BEFORE the successful approve rather than after it, because approve
// is terminal on the amendment (a second one refuses amendment_not_open), so
// "the refusal wrote nothing and the retry succeeds" is only observable in that
// order.
func TestEndToEndAmendmentReplaysTheLanternReshape(t *testing.T) {
	dir := layAmendE2ELanternShape(t)
	featureID := featureIDByKey(t, cliFeatureKey)
	retiredTask := soleTaskID(t)

	// --- the shape the reshape arrives into ---
	//
	// This is the field case's own state, and each line is a precondition of a
	// later assertion rather than a restatement of the fixture: the feature is
	// active (so the revise's target_status guard has something true to assert),
	// the task is at check (so the cancel is of work in flight rather than of
	// work that finished), the second feature is still draft (so Route has
	// somewhere to move on to), and the feature's evals gate is recorded green
	// (so the revise staling it afterwards is a change and not a first write).
	shape, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "active", shape.Features[featureID].Status)
	require.Equal(t, "check", shape.Tasks[retiredTask].Status)
	require.Equal(t, "draft", featureByKey(t, shape, amendE2EForwardFeature).Status)
	require.Equal(t, store.GateStatusPass, shape.Features[featureID].Gates["evals"].Status,
		"the fixture starts with a green evals gate, or the revise staling it proves nothing")
	priorSpecPin := shape.Features[featureID].AcceptedAgainst.Spec.Value
	priorEvalPin := shape.Features[featureID].AcceptedAgainst.Eval.Value
	require.NotEmpty(t, priorSpecPin, "the plan gate pinned the basis the reshape re-pins")

	// --- the reshape's own edits, then the amendment that records them ---
	//
	// The prose is edited FIRST and the preconditions are then read off the real
	// working tree. That order is the operation's meaning: a revise asserts the
	// human reviewed the content that is there now, and approve re-pins the basis
	// to exactly the bytes the preconditions named.
	reshapeAmendE2ESpec(t, featureID)
	specRevision := worktreeBlob(t, dir, filepath.Join(amendE2EFeatureRel(), "feature.md"))
	evalRevision := worktreeBlob(t, dir, filepath.Join(amendE2EFeatureRel(), "feature.eval.md"))
	require.NotEqual(t, priorSpecPin, specRevision, "the edit moved the spec off its pin")

	_, stderr, code := execCLI(t, "add", "amendment", "form-engine-rework",
		"--title", "Table-backed forms replace embedded JSON",
		"--triggered-by", amendE2EDesignDoc, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	amendmentID := amendmentIDByKey(t, "form-engine-rework")
	proposeAmendmentCLI(t, amendmentID)

	// The authoring order a human reaches for first is refused, and saying so
	// here is the point: a --superseded-by naming a staged addition resolves only
	// against operations ALREADY on the amendment, so the destructive-first order
	// that reads naturally in prose exits 2. The refusal reads like a missing
	// node, which is why the arc pins it rather than quietly avoiding it.
	_, stderr, code = execCLI(t, "amend", amendmentID, "--cancel", retiredTask,
		"--reason", "embedded JSON rendering is replaced by the table model",
		"--superseded-by", amendE2EReplacementKey, "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stderr, `"`+amendE2EReplacementKey+`" names no feature or task this amendment can target`)
	require.Empty(t, loadCwdTree(t).Amendments[amendmentID].Operations, "a refused stage appends nothing")

	// Staged in the order the surface requires: the revise of the feature, then
	// the addition, then the cancel that names it.
	for _, argv := range [][]string{
		{"amend", amendmentID, "--revise", cliFeatureKey,
			"--precondition", "target_status=active",
			"--precondition", "spec_revision=" + specRevision,
			"--precondition", "eval_revision=" + evalRevision, "--contributor", "jc"},
		{"amend", amendmentID, "--add-task", amendE2EReplacementKey,
			"--title", "Render from the table model",
			"--precondition", "key_absent=" + amendE2EReplacementKey, "--contributor", "jc"},
		{"amend", amendmentID, "--cancel", retiredTask,
			"--reason", "embedded JSON rendering is replaced by the table model",
			"--superseded-by", amendE2EReplacementKey,
			"--precondition", "target_status=check", "--contributor", "jc"},
		{"amend", amendmentID, "--amends-design", amendE2EDesignDoc, "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	// --- the review surfaces, before anything is applied ---
	//
	// show amendment is the batch beside which a human reviews the diff, and its
	// live-validity column is the part that cannot be read off the yml: every
	// precondition is compared against the tree as it stands right now.
	batch, _, code := execCLI(t, "show", "amendment", "form-engine-rework")
	require.Equal(t, 0, code)
	require.Contains(t, batch, "status: proposed")
	require.Contains(t, batch, "triggered_by: "+amendE2EDesignDoc)
	require.Contains(t, batch, "amends_design: "+amendE2EDesignDoc)
	require.Contains(t, batch, "0. revise "+cliFeatureKey+" ("+featureID+")")
	require.Contains(t, batch, "1. add-task "+amendE2EReplacementKey+`: "Render from the table model"`)
	// Staging created the replacement as a held draft, so the cancel names
	// it by its id like any other task.
	require.Contains(t, batch, "2. cancel form-engine/certification-form-loading/transcribe-the-schema ("+retiredTask+
		"): embedded JSON rendering is replaced by the table model, superseded by "+
		amendE2EReplacementKey+" ("+taskIDByKey(t, amendE2EReplacementKey)+")")
	require.Equal(t, 5, strings.Count(batch, "[valid]"),
		"all five staged preconditions read valid against the tree as staged")
	require.NotContains(t, batch, "[stale]")

	// The amendment seam's readout surveys the same batch, reports the two facts
	// a reader needs before crossing the gate, and leaves enumeration in Needs
	// attention behind the read-only queue seam; the ranked Next list is where
	// the mutation command appears, once, as the top move.
	amendReadout, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, amendReadout, "form-engine-rework ("+amendmentID+") · 3 operations staged")
	require.Contains(t, amendReadout, "Operations staged: 3")
	require.Contains(t, amendReadout, "Preconditions: 5 valid, 0 stale")
	require.Contains(t, amendReadout, "- 1 proposed amendment awaits your approval · `tiller queue --kind approve_amendment`")
	attention, next := splitAtNext(t, amendReadout)
	require.NotContains(t, attention, "tiller approve amendment form-engine-rework")
	require.Contains(t, next, "1. You: approve amendment form-engine-rework ("+amendmentID+").")

	// Orient's own readout carries the wait, which is the ranking claim: a
	// proposed amendment outranks the frontier, so the reader is sent to the gate
	// ahead of the draft feature that is otherwise next. The frontier is still a
	// legal move, so it is listed, ranked below the gate.
	forwardRef := amendE2EForwardFeature + " (" + featureIDByKey(t, amendE2EForwardFeature) + ")"
	orient, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	require.Contains(t, orient, "- 1 proposed amendment awaits your approval · `tiller queue --kind approve_amendment`")
	attention, next = splitAtNext(t, orient)
	require.NotContains(t, attention, "tiller approve amendment form-engine-rework")
	require.NotContains(t, attention, forwardRef, "the frontier is no part of the block above the list")
	require.Contains(t, next, "1. You: approve amendment form-engine-rework ("+amendmentID+").")
	require.Less(t, strings.Index(next, "approve amendment"), strings.Index(next, forwardRef),
		"the staged reshape outranks the frontier")

	// --- the stale arm ---
	//
	// The tree moves under the staged batch: the acceptance contract is edited
	// again after the human reviewed it. The batch's own eval_revision guard is
	// what notices, and the whole batch is refused rather than the one operation,
	// because the point of staging a batch is reviewing it as one.
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"),
		[]byte(amendE2EEvalMD(featureID, "The table model renders every form, on every surface")), 0o644))

	before := fingerprintRepo(t, dir)
	staleJSON, _, code := execCLI(t, "approve", "amendment", "form-engine-rework", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "a stale precondition is a blocked answer, not a crash and not a misuse")
	require.Contains(t, staleJSON, `"ok": false`)
	require.Contains(t, staleJSON, `"reason": "amendment_preconditions_stale"`)
	require.Contains(t, staleJSON, `"op": "revise"`)
	require.Contains(t, staleJSON, `"precondition": "eval_revision"`)
	require.Contains(t, staleJSON, `"expected": "`+evalRevision+`"`)
	require.Contains(t, staleJSON, `"actual": "`+
		worktreeBlob(t, dir, filepath.Join(amendE2EFeatureRel(), "feature.eval.md"))+`"`)
	require.NotContains(t, staleJSON, `"error"`)

	staleHuman, staleErr, code := execCLI(t, "approve", "amendment", "form-engine-rework", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, staleHuman+staleErr, "operation 0 (revise) "+cliFeatureKey+" ("+featureID+
		"): eval_revision expected")

	// Zero writes, over the whole repository and not only the files this test
	// thought to name. Rewriting identical bytes is still a write, and a refused
	// gate performs none (fingerprintRepo's own comment).
	requireRepoUnwritten(t, before, fingerprintRepo(t, dir))
	require.Equal(t, store.AmendmentStatusProposed, loadCwdTree(t).Amendments[amendmentID].Status,
		"a refused approve leaves the amendment open")

	// The recovery: the contract goes back to the bytes the batch was reviewed
	// against. There is no verb that edits a staged precondition, so restoring
	// the content the human approved (or withdrawing and re-opening) is the whole
	// of it, and the first is what a human who did not mean the second edit does.
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"),
		[]byte(amendE2EEvalMD(featureID, "The table model renders every form")), 0o644))
	require.Equal(t, evalRevision,
		worktreeBlob(t, dir, filepath.Join(amendE2EFeatureRel(), "feature.eval.md")),
		"the restored contract hashes to exactly what the precondition pinned")

	// --- the apply ---
	applied, stderr, code := execCLI(t, "approve", "amendment", "form-engine-rework", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	replacementTask := taskIDByKey(t, amendE2EReplacementKey)

	require.Contains(t, applied, "Approved amendment form-engine-rework ("+amendmentID+
		"): 3 operation(s) applied.",
		"the batch's one mint is a task, which carries no ownership clause at all: its real "+
			"landing owner is not something Op alone can honestly report")
	require.Contains(t, applied, "operation 0 (revise) "+cliFeatureKey+" ("+featureID+"): active")
	require.Contains(t, applied, "operation 1 (add-task): approved task "+amendE2EReplacementKey+" ("+replacementTask+")")
	require.Contains(t, applied, "operation 2 (cancel) form-engine/certification-form-loading/transcribe-the-schema ("+
		retiredTask+"): cancelled, replaced by "+amendE2EReplacementKey+" ("+replacementTask+")")
	require.Contains(t, applied, "Operations applied: 3")
	require.Contains(t, applied, "By kind: 1 revise, 1 cancel, 1 add-task")

	after, err := store.Load(dir)
	require.NoError(t, err)

	// Both directions of the one supersession, which is the pair that lets the
	// tree answer the question from either end (artifact-schema.md, Evolving a
	// project). The cancel wrote them onto work that never reached done, the
	// field case's own shape.
	retired := after.Tasks[retiredTask]
	replacement := after.Tasks[replacementTask]
	require.Equal(t, "cancelled", retired.Status)
	require.NotNil(t, retired.SupersededBy)
	require.Equal(t, replacementTask, *retired.SupersededBy, "the retired task names what replaced it")
	require.Equal(t, []string{retiredTask}, replacement.Supersedes, "the replacement names what it replaced")
	require.Equal(t, "ready", replacement.Status, "the held task is approved with its amendment (D9)")
	require.Equal(t, featureID, replacement.FeatureID, "the replacement is under the feature the reshape revised")
	require.Nil(t, replacement.SupersededBy)

	// The basis re-pinned to the reviewed content, and the evals gate staled
	// because the contract itself moved. The two together are what keeps the
	// reshape from landing with an active_basis refusal standing over work a
	// human just approved, while still refusing to carry a green verdict about a
	// contract that changed.
	revised := after.Features[featureID]
	require.Equal(t, specRevision, revised.AcceptedAgainst.Spec.Value, "the spec leg re-pinned to the reshaped prose")
	require.Equal(t, evalRevision, revised.AcceptedAgainst.Eval.Value, "and the eval leg to the reviewed contract")
	require.NotEqual(t, priorSpecPin, revised.AcceptedAgainst.Spec.Value)
	require.NotEqual(t, priorEvalPin, revised.AcceptedAgainst.Eval.Value)
	require.Equal(t, store.GateStatusStale, revised.Gates["evals"].Status,
		"a verdict recorded about the old contract does not survive the new one")
	require.Equal(t, "active", revised.Status, "a revise re-pins a basis and moves no state")

	// The amendment is terminal and the queue row it put there is gone.
	require.Equal(t, store.AmendmentStatusApproved, after.Amendments[amendmentID].Status)

	closedAmend, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.NotContains(t, closedAmend, "awaits your approval")
	require.Contains(t, closedAmend, "Operations staged: 0")

	proposed, _, code := execCLI(t, "ls", "amendments", "--status", "proposed", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, proposed, `"count": 0`)

	// Next moved on, and to something nameable: with no proposed amendment
	// left, the list names the draft feature nothing in the reshape touched.
	movedOn, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	require.NotContains(t, movedOn, "awaits your approval")
	_, next = splitAtNext(t, movedOn)
	require.Contains(t, next, "Agent: specify "+forwardRef+".")

	guide, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, guide, "amendment")
	require.Contains(t, guide, `"skill": "specify"`)

	// The guard the re-pin exists to satisfy: with the basis matching the working
	// tree, the doing verbs run again rather than refusing active_basis over the
	// spec the human just approved.
	_, stderr, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	// --- the events the apply recorded ---
	//
	// The event stream is where the WHY of a cancel lives: the lifecycle engine
	// does not persist a reason onto the node, so the cancel event is the record.
	// Every event the apply wrote also carries the amendment, on its targets and
	// in its after block, and the command it names is the gate that ran rather
	// than a cancel verb nobody typed.
	cancelEvent := soleAmendmentEvent(t, after, amendmentID, "cancel.task")
	require.Equal(t, "embedded JSON rendering is replaced by the table model", cancelEvent.After["reason"])
	require.Equal(t, replacementTask, cancelEvent.After["superseded_by"])
	require.Equal(t, amendmentID, cancelEvent.After["amendment"])
	require.Equal(t, "check", cancelEvent.Before["status"])
	require.Equal(t, "cancelled", cancelEvent.After["status"])
	require.Contains(t, cancelEvent.Targets, retiredTask)
	require.Contains(t, cancelEvent.Targets, amendmentID)
	require.Equal(t, "approve amendment "+amendmentID, cancelEvent.Command)

	// The held task keeps the add.task event staging wrote when it created
	// it, with the amendment added as provenance. Selecting it by the
	// amendment is what tells it apart from the fixture's own add.task, and
	// that the two are distinguishable at all is the provenance working. The
	// approval's promotion is its own approve.task event, naming the gate.
	addEvent := soleAmendmentEvent(t, after, amendmentID, "add.task")
	require.Equal(t, amendE2EReplacementKey, addEvent.After["key"])
	require.Equal(t, "amend "+amendmentID+" --add-task", addEvent.Command,
		"the command recorded is the staging that created the draft")
	promoteEvent := soleAmendmentEvent(t, after, amendmentID, "approve.task")
	require.Equal(t, "approve amendment "+amendmentID, promoteEvent.Command,
		"the command recorded is the gate that ran, not an approve verb nobody typed")
	require.Equal(t, "proposed", promoteEvent.Before["status"])
	require.Equal(t, "ready", promoteEvent.After["status"])

	reviseEvent := soleAmendmentEvent(t, after, amendmentID, "revise.feature")
	require.Equal(t, priorSpecPin, reviseEvent.Before["spec"])
	require.Equal(t, specRevision, reviseEvent.After["spec"])

	approvedEvent := soleAmendmentEvent(t, after, amendmentID, "amendment_approved")
	require.Equal(t, store.AmendmentStatusProposed, approvedEvent.Before["status"])
	require.Equal(t, store.AmendmentStatusApproved, approvedEvent.After["status"])
	require.Equal(t, 3, approvedEvent.After["operations"])

	// --- the acceptance question: the tree alone answers it ---
	//
	// Three reads, all of the working tree, none of any transcript: the retired
	// task says what replaced it, the replacement says what it replaced, and the
	// amendment says why and what else the same decision changed. Nothing here
	// consults the event stream, a chat log, or a PR description, which is the
	// whole claim (AGENTS.md principle 12: a late change is recorded forward work,
	// and the record lives with the code).
	retiredShow, _, code := execCLI(t, "show", "task", retiredTask)
	require.Equal(t, 0, code)
	require.Contains(t, retiredShow, "status: cancelled")
	require.Contains(t, retiredShow, "superseded_by: "+amendE2EReplacementKey+" ("+replacementTask+")")

	replacementShow, _, code := execCLI(t, "show", "task", replacementTask)
	require.Equal(t, 0, code)
	require.Contains(t, replacementShow, "title:  Render from the table model")
	require.Contains(t, replacementShow,
		"supersedes: form-engine/certification-form-loading/transcribe-the-schema ("+retiredTask+")")

	whyShow, _, code := execCLI(t, "show", "amendment", "form-engine-rework")
	require.Equal(t, 0, code)
	require.Contains(t, whyShow, "status: approved")
	require.Contains(t, whyShow, "title:  Table-backed forms replace embedded JSON")
	require.Contains(t, whyShow, "triggered_by: "+amendE2EDesignDoc)
	require.Contains(t, whyShow, "amends_design: "+amendE2EDesignDoc)
	require.Contains(t, whyShow, "embedded JSON rendering is replaced by the table model")

	// And the narrative the human wrote about the reshape is a file in the repo,
	// under git with everything else, which is what the three reads above point a
	// reader at when the recorded fields are not enough.
	require.FileExists(t, filepath.Join(dir, ".tiller", "amendments",
		amendmentID+"-form-engine-rework.md"))
}

// --- fixture ---

// layAmendE2ELanternShape drives a git-backed project to the field case's own
// state through the real verbs: an approved epic, a specified and plan-approved
// feature that has been started, its one task worked to check, the feature's own
// gates recorded green, and a second feature left in draft. It returns the repo
// root and leaves the process in it.
//
// Every station is a real command. The task reaches check by start then check,
// not by a status poked onto its yml, because the cancel the reshape stages is
// legal from check and would be a different act from any other state.
func layAmendE2ELanternShape(t *testing.T) string {
	t.Helper()
	dir := initGatedGitRepo(t, "true")
	for _, argv := range [][]string{
		{"add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc"},
		{"add", "feature", cliFeatureKey, "--title", "Certification form loading",
			"--change-type", "create", "--contributor", "jc"},
		{"add", "feature", amendE2EForwardFeature, "--title", "Table backed forms",
			"--change-type", "create", "--contributor", "jc"},
		{"approve", "epic", "form-engine", "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliTaskKey, "jc")

	for _, argv := range [][]string{
		{"spec", "feature", cliFeatureKey, "--contributor", "jc"},
		{"approve", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	tid := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}

	// The design document --triggered-by and --amends-design name. It is written
	// and committed with the rest, so the paths the amendment records point at a
	// file that is really there.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, amendE2EDesignDoc),
		[]byte("# Form engine\n\nForms are rendered from discrete tables, not embedded JSON.\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay the tree and the design docs")
	return dir
}

// amendE2EFeatureRel is the fixture feature's repo-relative folder, the base the
// working-tree blob reads are taken against.
func amendE2EFeatureRel() string {
	return filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading")
}

// amendE2EEvalMD is the fixture's acceptance contract with one criterion. The
// statement is a parameter so a test can move the contract's content (and so its
// blob) by one meaningful word, and the node id is one so the contract names the
// feature the real flow actually minted, the way mutate's own greenEvalMD does
// (amendment_approve_test.go). Nothing validates the node field today
// (store.EvalFile.ValidateContract never reads it), which is exactly why a
// fixture that hardcodes an id nothing minted can sit there indefinitely looking
// like something the flow produced.
func amendE2EEvalMD(featureID, statement string) string {
	return "---\nnode: " + featureID + "\ncriteria:\n  - id: C1\n    statement: " + statement + "\n" +
		"    polarity: introduce\n    binding: { type: test, ref: spec/form_spec.rb }\n---\n\n# Acceptance\n"
}

// reshapeAmendE2ESpec is the human's own edit: the feature is re-scoped onto the
// table model, in its prose and in its acceptance contract both. It writes the
// working tree and nothing else; the amendment is what records that a human
// reviewed it.
func reshapeAmendE2ESpec(t *testing.T, featureID string) {
	t.Helper()
	dir := featureDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nRender the FORM-A-100 form from discrete tables.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte(amendE2EEvalMD(featureID, "The table model renders every form")), 0o644))
}

// --- readers ---

// worktreeBlob is the git blob hash of a working-tree file in the compact token
// form the schema records, read through git the same way the basis system and
// the preconditions both read it. A test computing a precondition value any
// other way would be asserting against its own arithmetic.
func worktreeBlob(t *testing.T, dir, rel string) string {
	t.Helper()
	return "git:" + gitOut(t, dir, "hash-object", rel)
}

// amendmentIDByKey reads an amendment's minted A-id off disk by its key.
func amendmentIDByKey(t *testing.T, key string) string {
	t.Helper()
	for id, a := range loadCwdTree(t).Amendments {
		if a.Key != nil && *a.Key == key {
			return id
		}
	}
	t.Fatalf("amendment %q not found", key)
	return ""
}

// featureByKey reads one feature out of a loaded tree by its canonical key.
func featureByKey(t *testing.T, tree *store.Tree, key string) *store.Feature {
	t.Helper()
	for _, f := range tree.Features {
		if f.Key != nil && *f.Key == key {
			return f
		}
	}
	t.Fatalf("feature %q not found", key)
	return nil
}

// soleAmendmentEvent returns the one event of a type that this amendment wrote,
// found by the amendment id on the event's own targets. Every event the apply
// records carries it there, which is what tells the amendment's add.task apart
// from the ordinary `tiller add task` that laid the fixture's own.
//
// Requiring exactly one is part of the assertion rather than convenience: it
// catches a batch that recorded an operation twice, which is the shape a partial
// apply followed by a re-approve would leave.
func soleAmendmentEvent(t *testing.T, tree *store.Tree, amendmentID, typ string) events.Event {
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
	require.Lenf(t, found, 1, "expected exactly one %s event carrying %s", typ, amendmentID)
	require.Equal(t, amendmentID, found[0].After["amendment"],
		"every event an apply writes carries the amendment in its after block too")
	return found[0]
}

// fingerprintRepo digests every regular file under dir by its bytes AND its
// modification time, keyed by the path relative to dir, .git included since
// nothing a refused gate does may write there either.
//
// Rewriting identical bytes is still a write, and a content-only fingerprint
// calls that clean. This mirrors mutate's own snapshotDir
// (amendment_approve_test.go), which the same reasoning produced one layer down.
func fingerprintRepo(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		require.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		info, err := d.Info()
		require.NoError(t, err)
		out[rel] = fmt.Sprintf("%x@%d", sha256.Sum256(data), info.ModTime().UnixNano())
		return nil
	}))
	return out
}

// requireRepoUnwritten compares two fingerprints and, on a difference, names
// only the paths that moved. Comparing the maps directly would dump the whole
// repository to say that one file was rewritten, burying the finding.
func requireRepoUnwritten(t *testing.T, before, after map[string]string) {
	t.Helper()
	var changed []string
	for rel, digest := range after {
		switch prior, known := before[rel]; {
		case !known:
			changed = append(changed, rel+" (created)")
		case prior != digest:
			changed = append(changed, rel+" (rewritten)")
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			changed = append(changed, rel+" (removed)")
		}
	}
	sort.Strings(changed)
	require.Empty(t, changed, "a refused approve writes nothing at all")
}

// TestEndToEndApprovePromotesHeldDrafts drives held drafts through the CLI:
// stage an epic, a feature under it, and a task under an approved feature,
// hand the amendment over, and approve it. The confirmation says what the
// approval did to each draft: the epic and the task are approved (D8, D9), and
// the feature stays a draft with specify next.
func TestEndToEndApprovePromotesHeldDrafts(t *testing.T) {
	layAmendmentTree(t)
	approveFixtureFeatureOnDisk(t)
	_, stderr, code := execCLI(t, "add", "amendment", "hardening", "--title", "Harden the forms", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := amendmentIDByKey(t, "hardening")

	for _, args := range [][]string{
		{"--add-epic", "hardening", "--title", "Hardening"},
		{"--add-feature", "hardening/csp-headers", "--title", "CSP headers", "--change-type", "create"},
		{"--add-task", "form-engine/certification-form-loading/load-offline", "--title", "Load offline"},
	} {
		_, stderr, code = execCLI(t, append(append([]string{"amend", aid}, args...), "--contributor", "jc")...)
		require.Equal(t, 0, code, stderr)
	}
	proposeAmendmentCLI(t, aid)

	tree := loadCwdTree(t)
	ops := tree.Amendments[aid].Operations
	epicID, featureID, taskID := ops[0].Node, ops[1].Node, ops[2].Node

	out, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "approved epic hardening ("+epicID+")")
	require.Contains(t, out, "feature hardening/csp-headers ("+featureID+") stays draft; specify is next")
	require.Contains(t, out, "approved task form-engine/certification-form-loading/load-offline ("+taskID+")")
	require.NotContains(t, out, "minted", "staging created these drafts; approval minted nothing")

	tree = loadCwdTree(t)
	require.Equal(t, "approved", tree.Amendments[aid].Status)
	require.Equal(t, "ready", tree.Epics[epicID].Status)
	require.Equal(t, "draft", tree.Features[featureID].Status)
	require.Equal(t, "ready", tree.Tasks[taskID].Status)
}

// TestApproveAmendmentReportsARemovedDraftApart: a draft removed before
// approval is named as removed and is not counted among the applied
// operations.
func TestApproveAmendmentReportsARemovedDraftApart(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "two-epics", "Two new epics")
	for _, key := range []string{"hardening", "observability"} {
		_, stderr, code := execCLI(t, "amend", aid, "--add-epic", key, "--title", key, "--contributor", "jc")
		require.Equal(t, 0, code, stderr)
	}
	removedID := loadCwdTree(t).Amendments[aid].Operations[1].Node
	_, stderr, code := execCLI(t, "remove", "epic", "observability", "--reason", "not this quarter", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, ": 1 operation(s) applied, 1 removed.")
	require.Contains(t, out, "operation 1 (add-epic) observability ("+removedID+"): removed")
	require.Contains(t, out, "Operations applied: 1")
}

// The arcs below drive the held-draft model through the real CLI (docs/plans/
// 2026-11-12-intake-decompose-design.md, "Verification"). Each is one test, named
// for what it proves.

// e2eRefusal reads a blocked or misuse answer's reason: the envelope puts it in
// error.reason for misuse (exit 2) and in data.reason for a blocked answer
// (exit 1).
func e2eRefusal(t *testing.T, out string, wantCode, gotCode int) string {
	t.Helper()
	require.Equal(t, wantCode, gotCode, out)
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env), out)
	for _, m := range []map[string]any{env.Data, env.Error} {
		if r, ok := m["reason"].(string); ok {
			return r
		}
	}
	t.Fatalf("no reason in %s", out)
	return ""
}

// e2eStage stages one operation on an amendment and returns the answer's JSON.
func e2eStage(t *testing.T, aid string, args ...string) string {
	t.Helper()
	out, stderr, code := execCLI(t, append(append([]string{"amend", aid}, args...), "--contributor", "jc", "--json")...)
	require.Equal(t, 0, code, out+stderr)
	return out
}

// TestEndToEndDocumentThatReplacesWork: one amendment adds a feature and cancels
// the existing feature it replaces, naming the held feature as the replacement.
// Approval cancels the old feature with the lineage pointing at the new one,
// which stays a draft awaiting specify.
func TestEndToEndDocumentThatReplacesWork(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "replace-loading", "Replace certification loading")
	oldFeature := "form-engine/certification-form-loading"
	newFeature := "form-engine/certification-v2"

	out := e2eStage(t, aid, "--add-feature", newFeature, "--title", "Certification v2", "--change-type", "create")
	newID := heldNodeOf(t, out)
	e2eStage(t, aid, "--cancel", oldFeature, "--reason", "the document replaces it", "--superseded-by", newFeature)
	oldID := featureByKey(t, loadCwdTree(t), oldFeature).ID

	// The cancel is staged, not applied: the old feature is untouched.
	require.Equal(t, "draft", loadCwdTree(t).Features[oldID].Status)

	out, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	tree := loadCwdTree(t)
	require.Equal(t, "approved", tree.Amendments[aid].Status)
	require.Equal(t, "cancelled", tree.Features[oldID].Status)
	require.Equal(t, "draft", tree.Features[newID].Status, "the replacement stays a draft; specify is next")
	require.NotNil(t, tree.Features[oldID].SupersededBy)
	require.Equal(t, newID, *tree.Features[oldID].SupersededBy, "the old feature records what replaced it")
}

// TestEndToEndPartialAcceptance: the human takes most of a batch. One held draft
// is removed, its operation is marked removed and stays on the record, and the
// rest approves.
func TestEndToEndPartialAcceptance(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "two-epics", "Two epics")
	keep := heldNodeOf(t, e2eStage(t, aid, "--add-epic", "hardening", "--title", "Hardening"))
	drop := heldNodeOf(t, e2eStage(t, aid, "--add-epic", "observability", "--title", "Observability"))

	out, stderr, code := execCLI(t, "remove", "epic", "observability", "--reason", "not this quarter", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	tree := loadCwdTree(t)
	require.NotContains(t, tree.Epics, drop)
	ops := tree.Amendments[aid].Operations
	require.Len(t, ops, 2, "removal never deletes the operation entry")
	require.NotEmpty(t, ops[1].Removed)

	out, stderr, code = execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "removed")

	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	require.Contains(t, out, ": 1 operation(s) applied, 1 removed.")
	tree = loadCwdTree(t)
	require.Equal(t, "approved", tree.Amendments[aid].Status)
	require.Equal(t, "ready", tree.Epics[keep].Status)
	require.NotContains(t, tree.Epics, drop)
}

// TestEndToEndNothingToBuild: a document that yields no work is proposed empty.
// approve refuses it (amendment_empty), the human's queue offers the withdraw,
// and the withdraw closes it.
func TestEndToEndNothingToBuild(t *testing.T) {
	initOwnedRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	src := writeFeedbackDoc(t, "praise.md", "A fine product, nothing to change.\n")
	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	aid, _ := jsonData(t, out)["id"].(string)

	out, stderr, code = execCLI(t, "propose", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "It proposes nothing; withdraw it with: tiller withdraw amendment praise")

	out, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, "amendment_empty", e2eRefusal(t, out, 2, code))

	_, human := e2eGuideSkill(t)
	require.Contains(t, human, "withdraw amendment praise")
	require.NotContains(t, human, "tiller approve amendment")

	out, stderr, code = execCLI(t, "withdraw", "amendment", aid, "--reason", "nothing to build", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, "withdrawn", loadCwdTree(t).Amendments[aid].Status)
	_, human = e2eGuideSkill(t)
	require.NotContains(t, human, "withdraw amendment praise")
}

// TestEndToEndFolderGoesStale: a folder intake is one tracking unit. Change one
// file under the landed folder and show amendment reads stale and approve
// refuses; restoring the file lets the approval land.
func TestEndToEndFolderGoesStale(t *testing.T) {
	initOwnedRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	src := filepath.Join(t.TempDir(), "specs")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.md"), []byte("alpha\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.md"), []byte("beta\n"), 0o644))
	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	e2eStage(t, aid, "--add-epic", "specs-epic", "--title", "Specs epic", "--source", ".tiller/design/specs/a.md#L1")
	proposeAmendmentCLI(t, aid)

	out, _, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	source, _ := jsonData(t, out)["source"].(map[string]any)
	require.Equal(t, "valid", source["status"])

	changed := filepath.Join(".tiller", "design", "specs", "sub", "b.md")
	require.NoError(t, os.WriteFile(changed, []byte("beta, edited\n"), 0o644))
	out, _, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	source, _ = jsonData(t, out)["source"].(map[string]any)
	require.Equal(t, "stale", source["status"], "one changed file stales the whole folder")

	out, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, "source_changed", e2eRefusal(t, out, 2, code))
	require.Equal(t, "proposed", loadCwdTree(t).Amendments[aid].Status)

	require.NoError(t, os.WriteFile(changed, []byte("beta\n"), 0o644))
	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, "approved", loadCwdTree(t).Amendments[aid].Status)
}

var nodeLineRe = regexp.MustCompile(`(?m)^\s+node: .*\n`)

// TestEndToEndLegacyAmendment: an amendment staged before schema 15 (a proposed
// amendment whose add operations carry no node) is migrated and then approved
// under the old rule: approval mints the nodes, since no draft was ever held.
func TestEndToEndLegacyAmendment(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "legacy", "Legacy reshape")
	e2eStage(t, aid, "--add-epic", "hardening", "--title", "Hardening")
	e2eStage(t, aid, "--add-feature", "hardening/csp-headers", "--title", "CSP headers", "--change-type", "create")

	// Rewind to what schema 14 wrote: no held nodes on disk, no node on the
	// operations, the manifest stamped 14.
	require.NoError(t, os.RemoveAll(filepath.Join(".tiller", "epics", "hardening")))
	matches, err := filepath.Glob(filepath.Join(".tiller", "amendments", "A-*-legacy.yml"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	body, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	stripped := nodeLineRe.ReplaceAll(body, nil)
	require.NotEqual(t, string(body), string(stripped), "the fixture must drop the node lines")
	require.NoError(t, os.WriteFile(matches[0], stripped, 0o644))
	manifest := filepath.Join(".tiller", "tiller.yml")
	mbody, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Contains(t, string(mbody), "schema_version: 15")
	require.NoError(t, os.WriteFile(manifest, []byte(strings.Replace(string(mbody), "schema_version: 15", "schema_version: 14", 1)), 0o644))
	gitRun(t, ".", "add", "-A")
	gitRun(t, ".", "commit", "-m", "schema 14 fixture")

	out, stderr, code := execCLI(t, "migrate", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, 15, migratedManifestSchema(t, "."))
	tree := loadCwdTree(t)
	require.Equal(t, "proposed", tree.Amendments[aid].Status)
	for _, op := range tree.Amendments[aid].Operations {
		require.Empty(t, op.Node, "migration is stamp-only; it holds nothing")
	}
	for _, e := range tree.Epics {
		require.NotEqual(t, "hardening", *e.Key)
	}

	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	tree = loadCwdTree(t)
	require.Equal(t, "approved", tree.Amendments[aid].Status)
	require.Equal(t, "draft", tree.Epics[epicIDByKey(t, tree, "hardening")].Status, "the old rule mints the epic as a draft for its own gate")
	require.Contains(t, out, "minted", "the old rule creates the nodes at approval")
	require.Equal(t, "draft", featureByKey(t, tree, "hardening/csp-headers").Status)
}

// TestEndToEndNoAmendmentRefusedByAHeldEpic: a held draft epic is an epic, so a
// project whose only epic is staged on an open amendment is no longer founding.
// The flag refuses with amendment_required and lands nothing.
func TestEndToEndNoAmendmentRefusedByAHeldEpic(t *testing.T) {
	initOwnedRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	out, stderr, code := execCLI(t, "intake", writeFeedbackDoc(t, "idea.md", "an idea\n"), "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	e2eStage(t, aid, "--add-epic", "ideas", "--title", "Ideas", "--source", ".tiller/design/idea.md#L1")

	src := writeFeedbackFolder(t, "specs")
	out, _, code = execCLI(t, "intake", src, "--no-amendment", "--contributor", "jc", "--json")
	require.Equal(t, "amendment_required", e2eRefusal(t, out, 2, code))
	require.NoDirExists(t, filepath.Join(".tiller", "design", "specs"))
}

// TestEndToEndHoldRefusals: each way the hold refuses, through the CLI, with the
// reason the spec names, and each refusal writes nothing.
func TestEndToEndHoldRefusals(t *testing.T) {
	layAmendmentTree(t)
	approveFixtureFeatureOnDisk(t)
	_, stderr, code := execCLI(t, "add", "feature", "form-engine/draft-sibling", "--title", "Draft sibling", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := openAmendmentCLI(t, "reshape", "Reshape")
	e2eStage(t, aid, "--add-epic", "hardening", "--title", "Hardening")
	e2eStage(t, aid, "--add-feature", "hardening/csp-headers", "--title", "CSP headers", "--change-type", "create")
	before := fingerprintRepo(t, ".")

	// A lifecycle verb on a held node: held_by_amendment, a blocked answer.
	for _, args := range [][]string{
		{"approve", "epic", "hardening"},
		{"approve", "feature", "hardening/csp-headers"},
		{"spec", "feature", "hardening/csp-headers"},
		{"cancel", "feature", "hardening/csp-headers", "--reason", "no"},
		{"cancel", "epic", "hardening", "--reason", "no"},
	} {
		out, _, code := execCLI(t, append(args, "--contributor", "jc", "--json")...)
		require.Equal(t, "held_by_amendment", e2eRefusal(t, out, 1, code), strings.Join(args, " "))
	}

	// A plain add under a held parent belongs to the amendment, not to a loose command.
	out, _, code := execCLI(t, "add", "feature", "hardening/other", "--title", "Other", "--change-type", "create", "--contributor", "jc", "--json")
	require.Equal(t, "parent_held_elsewhere", e2eRefusal(t, out, 2, code))

	// A task is staged only under an approved feature, and never under a held one.
	out, _, code = execCLI(t, "amend", aid, "--add-task", "form-engine/draft-sibling/t1", "--title", "T1", "--contributor", "jc", "--json")
	require.Equal(t, "feature_not_approved", e2eRefusal(t, out, 2, code))
	out, _, code = execCLI(t, "amend", aid, "--add-task", "hardening/csp-headers/t1", "--title", "T1", "--contributor", "jc", "--json")
	require.Equal(t, "task_under_held_feature", e2eRefusal(t, out, 2, code))

	require.Equal(t, before, fingerprintRepo(t, "."), "every refusal left the repo untouched")

	// A held node deleted by hand is caught before the human is asked.
	require.NoError(t, os.RemoveAll(filepath.Join(".tiller", "epics", "hardening")))
	out, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, "held_node_missing", e2eRefusal(t, out, 2, code))
	require.Equal(t, "proposed", loadCwdTree(t).Amendments[aid].Status)
}

// e2eDoneEpic drives one epic with one feature and one task all the way to done
// through the real CLI and returns the epic's and the feature's ids.
func e2eDoneEpic(t *testing.T) (dir, epicID string) {
	t.Helper()
	dir = initGatedGitRepo(t, "true")
	for _, args := range [][]string{
		{"add", "epic", "form-engine", "--title", "Form Engine"},
		{"add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create"},
		{"approve", "epic", "form-engine"},
	} {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
	}
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	for _, args := range [][]string{{"spec", "feature", cliFeatureKey}, {"approve", "feature", cliFeatureKey}} {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
	}
	tid := soleTaskID(t)
	claimTaskAsOwner(t, tid, "jc")
	for _, args := range [][]string{{"start", "task", tid}} {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
	}
	branch := checkoutTaskBranchCLI(t, tid)
	for _, args := range [][]string{{"check", "task", tid}, {"submit", "task", tid, "--pr", "41"}, {"check", "feature", cliFeatureKey}} {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree and task work")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("work\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "feature work")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge feature", branch)
	out, stderr, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	tree := loadCwdTree(t)
	epic := soleEpic(t, tree)
	require.Equal(t, "done", epic.Status)
	return dir, epic.ID
}

// TestEndToEndHeldDraftUnderADoneEpicDoesNotReopenIt: new work under a done epic
// reopens it on the next sync, but a draft an amendment holds is not work yet.
// sync leaves the epic done while the draft is held, and the approved draft then
// reopens it the ordinary way.
func TestEndToEndHeldDraftUnderADoneEpicDoesNotReopenIt(t *testing.T) {
	dir, epicID := e2eDoneEpic(t)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state after sync")
	aid := openAmendmentCLI(t, "more-forms", "More forms")
	heldID := heldNodeOf(t, e2eStage(t, aid, "--add-feature", "form-engine/table-forms", "--title", "Table forms", "--change-type", "create"))
	require.Equal(t, "draft", loadCwdTree(t).Features[heldID].Status)

	out, stderr, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	tree := loadCwdTree(t)
	require.Equal(t, "done", tree.Epics[epicID].Status, "a held draft keeps a done epic done")
	require.Equal(t, "draft", tree.Features[heldID].Status)

	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	out, stderr, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, "ready", loadCwdTree(t).Epics[epicID].Status, "once approved, the new feature reopens the epic")
}

// TestEndToEndWithdrawRemovesDraftsAndDetachesTheirReferences: withdrawing an
// amendment removes the drafts it held, detaches a question that blocked one and
// a shelf item attached to one, and frees the key.
func TestEndToEndWithdrawRemovesDraftsAndDetachesTheirReferences(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "reshape", "Reshape")
	epicID, featureID := stageHeldEpicCLI(t, aid)

	out, stderr, code := execCLI(t, "add", "question", "Which audit sink?", "--title", "Audit sink", "--blocks", "hardening/audit-log", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	out, stderr, code = execCLI(t, "add", "bug", "audit-gap", "--title", "Audit gap", "--attach", "hardening/audit-log", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	tree := loadCwdTree(t)
	q := questionByStatement(t, tree, "Which audit sink?")
	require.Equal(t, []string{featureID}, q.Blocks)
	var bugID string
	for id, b := range tree.Bugs {
		require.Equal(t, featureID, b.AttachedTo)
		bugID = id
	}
	require.NotEmpty(t, bugID)

	out, stderr, code = execCLI(t, "withdraw", "amendment", aid, "--reason", "not now", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	removed, _ := jsonData(t, out)["removed_ids"].([]any)
	require.ElementsMatch(t, []any{epicID, featureID}, removed)

	tree = loadCwdTree(t)
	require.NotContains(t, tree.Epics, epicID)
	require.NotContains(t, tree.Features, featureID)
	require.Equal(t, "withdrawn", tree.Amendments[aid].Status)
	require.Empty(t, questionByStatement(t, tree, "Which audit sink?").Blocks, "the question no longer blocks a node that is gone")
	require.Empty(t, tree.Bugs[bugID].AttachedTo, "the bug is detached, and kept")
	require.NoDirExists(t, filepath.Join(".tiller", "epics", "hardening"))

	// The key is free again.
	out, stderr, code = execCLI(t, "add", "amendment", "reshape", "--title", "Reshape again", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
}

// TestEndToEndQueueIsSilentAboutADraftAmendmentUntilPropose: the human's queue
// carries an approval only once the agent hands the amendment over.
func TestEndToEndQueueIsSilentAboutADraftAmendmentUntilPropose(t *testing.T) {
	layAmendmentTree(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	_, stderr, code := execCLI(t, "add", "amendment", "reshape", "--title", "Reshape", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := amendmentIDByKey(t, "reshape")
	e2eStage(t, aid, "--add-epic", "hardening", "--title", "Hardening")

	for _, cmd := range [][]string{{"guide"}, {"queue"}, {"next"}, {"readout", "amend"}} {
		out, stderr, code := execCLI(t, cmd...)
		require.Equal(t, 0, code, stderr)
		require.NotContains(t, out, "approve amendment", strings.Join(cmd, " "))
	}
	queueJSON, _, code := execCLI(t, "queue", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, queueJSON, "approve_amendment")

	proposeAmendmentCLI(t, aid)
	_, human := e2eGuideSkill(t)
	require.Contains(t, human, "approve amendment reshape")
	queueJSON, _, code = execCLI(t, "queue", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, queueJSON, "approve_amendment")
}
