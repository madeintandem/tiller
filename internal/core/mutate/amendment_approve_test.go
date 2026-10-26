package mutate_test

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- fixtures ---

// amendDeps is testDeps with a sequence minter and a real git window. An
// amendment's approve mints ids for the nodes it materializes, so the shared
// fixed-token minter (every node gets <prefix>-aaaaaa) would exhaust mintIn's
// retry against the tree's existing task; and its preconditions and basis
// re-pins are read through git, so the window has to be real. EV ids stay
// random rather than sequenced, so two deps built in one test can never plan
// the same event path twice.
func amendDeps(root string) mutate.Deps {
	seq := 0
	return amendDepsCounting(root, &seq)
}

// amendDepsCounting is amendDeps with the caller owning the sequential mint
// counter, so a fixture copied from a template can resume the template's
// numbering instead of minting ids the template already holds.
func amendDepsCounting(root string, seq *int) mutate.Deps {
	d := testDeps()
	d.Mint = func(prefix string) string {
		if prefix == "EV" {
			return id.Mint(prefix)
		}
		*seq++
		return fmt.Sprintf("%s-%06d", prefix, *seq)
	}
	d.Rev = vcs.New(root)
	return d
}

// greenEvalMD is a one-criterion acceptance contract whose command binding
// passes, so the fixture's evals gate is recorded green and a revise can be
// SEEN to stale it.
func greenEvalMD(featureID string) string {
	return "---\nnode: " + featureID + "\ncriteria:\n" +
		"  - id: C1\n    statement: A logged session completes in under three taps\n" +
		"    polarity: introduce\n    binding: { type: command, run: \"true\" }\n    required: true\n---\n\n# Acceptance\n"
}

// lanternFixture lays the field case this build is aimed at (the driver doc's Lantern
// case, 2026-08-06): an active feature whose spec and contract a human
// approved, one task under it sitting at check, and a green evals gate on the
// feature, in a real git repo so the basis pins and the precondition content
// hashes both read through git.
// lanternFixture hands each test its own copy of the laid fixture. Laying it
// costs a dozen mutations, a gate run, and ten git processes, and eighty tests
// want it; so it is a template, laid once and copied into a fresh TempDir per
// caller, and every caller resumes the id counter where the lay stopped (issue
// #87). The fixture's clock is fixed, so a copy is indistinguishable from a
// fresh lay.
func lanternFixture(t *testing.T) (root string, deps mutate.Deps, featureID, taskID string) {
	t.Helper()
	root, laid := testgit.CopyWithResult(t, &lanternTemplate, func(t testing.TB, dir string) lanternLaid {
		var laid lanternLaid
		laid.seq, laid.featureID, laid.taskID = layLanternFixture(t, dir)
		return laid
	})
	seq := laid.seq
	deps = amendDepsCounting(root, &seq)
	return root, deps, laid.featureID, laid.taskID
}

var lanternTemplate = testgit.Template{Name: "lantern"}

// lanternLaid is what the lay returned, kept with the snapshot it describes.
type lanternLaid struct {
	seq       int
	featureID string
	taskID    string
}

// layLanternFixture drives a gated repo to the field case's own state through
// the real verbs, laid into root, and returns the id counter it ended on and the
// ids.
func layLanternFixture(t testing.TB, root string) (seq int, featureID, taskID string) {
	t.Helper()
	layGatedRepo(t, root, "true", "true")
	deps := amendDepsCounting(root, &seq)

	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "app-experience", Title: "App experience"})
	require.NoError(t, err)
	feat, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/fast-logging", Title: "Fast logging", ChangeType: "create"})
	require.NoError(t, err)
	featureID = feat.ID

	dir := filepath.Join(root, lanternFeatureDir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Fast logging\n\nLog a session in under three taps, on iPhone and on iPad.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(greenEvalMD(featureID)), 0o644))

	task, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "app-experience/fast-logging/ipad-logging", Title: "iPad logging surface"})
	require.NoError(t, err)
	taskID = task.ID
	_, err = mutate.ClaimTask(load(t, root), deps, taskID, "")
	require.NoError(t, err)
	writeTaskSpecAt(t, root, "app-experience", "fast-logging", "ipad-logging",
		"## Approach\n\nBuild the iPad-specific logging surface.")

	driveWith(t, root, deps, store.NounEpic, "app-experience", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, "app-experience/fast-logging", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, "app-experience/fast-logging", mutate.TransitionInput{Verb: "approve"})

	// The task rides the plan gate to ready; drive it to check, the state the
	// field case's casualty was sitting in.
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, vcs.New(root), tree.Tasks[taskID], mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed, "the fixture's gates are bound to true and must pass")
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status)

	// Record the feature's evals gate, so the revise op's staling is observable
	// rather than a write onto a gate nobody had run.
	tree = load(t, root)
	_, err = mutate.Eval(tree, deps, vcs.New(root), tree.Features[featureID], mutate.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, store.GateStatusPass, load(t, root).Features[featureID].Gates["evals"].Status,
		"the fixture starts with a green evals gate or its staling proves nothing")

	gitCommitAllM(t, root, "lay the tree")
	gitRunM(t, root, "branch", "-M", "main")
	return seq, featureID, taskID
}

// lanternFeatureDir is the fixture feature's repo-relative folder.
var lanternFeatureDir = filepath.Join(".tiller", "epics", "app-experience", "features", "fast-logging")

var (
	lanternSpecRel = filepath.Join(lanternFeatureDir, "feature.md")
	lanternEvalRel = filepath.Join(lanternFeatureDir, "feature.eval.md")
)

// reshapeTheSpec is the human's edit: iPad support leaves the feature's spec and
// its acceptance contract. The amendment's revise operation is the record that a
// human reviewed exactly this content.
func reshapeTheSpec(t *testing.T, root, featureID string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, lanternSpecRel),
		[]byte("# Fast logging\n\nLog a session in under three taps, on iPhone. iPad is out of v1.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, lanternEvalRel),
		[]byte(strings.Replace(greenEvalMD(featureID),
			"A logged session completes in under three taps",
			"A logged session completes in under three taps on iPhone", 1)), 0o644))
}

// snapshotDir fingerprints every regular file under root by a digest of its
// bytes AND its modification time, keyed by the path relative to root. A
// refused approve must leave it identical (requireNoWrites is the comparison),
// which is what covers "no writes of ANY kind" rather than only the files a
// test thought to name. The whole root is walked, .git included, since nothing
// here may write there either.
//
// The mtime leg detects writes even when the resulting bytes are unchanged.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		info, err := d.Info()
		require.NoError(t, err)
		out[rel] = fmt.Sprintf("%x@%d", sha256.Sum256(data), info.ModTime().UnixNano())
		return nil
	})
	require.NoError(t, err)
	return out
}

// requireNoWrites compares two snapshotDir fingerprints and, on a difference,
// names only the paths that moved. Comparing the maps directly would dump the
// whole repository (a few hundred files, .git included) to say that one of them
// was rewritten, which buries the finding the assertion exists to surface.
func requireNoWrites(t *testing.T, before, after map[string]string) {
	t.Helper()
	var changed []string
	for rel, fingerprint := range after {
		switch prior, known := before[rel]; {
		case !known:
			changed = append(changed, rel+" (created)")
		case prior != fingerprint:
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

// openAmendment opens one proposed amendment on the fixture tree and returns the
// loaded node, the handle every stage and the approve take.
func openAmendment(t *testing.T, root string, deps mutate.Deps, key string) *store.Amendment {
	t.Helper()
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{
		Key: key, Title: "Remove native iPad support from v1"})
	require.NoError(t, err)
	return loadAmendment(t, root, res.ID)
}

// stage appends one operation to an open amendment and hands back the reloaded
// amendment, so a caller can chain stages without re-resolving.
func stage(t *testing.T, root string, deps mutate.Deps, a *store.Amendment, in mutate.StageInput) *store.Amendment {
	t.Helper()
	_, err := mutate.StageOperation(load(t, root), deps, a, in)
	require.NoError(t, err)
	return loadAmendment(t, root, a.ID)
}

// approve runs the gate and requires it to succeed.
func approve(t *testing.T, root string, deps mutate.Deps, a *store.Amendment) mutate.ApproveResult {
	t.Helper()
	res, err := mutate.ApproveAmendment(load(t, root), deps, a)
	require.NoError(t, err)
	return res
}

// tryApprove runs the gate and returns its error alone, the shape the refusal
// assertions read.
func tryApprove(t *testing.T, root string, deps mutate.Deps, a *store.Amendment) error {
	t.Helper()
	_, err := mutate.ApproveAmendment(load(t, root), deps, a)
	return err
}

// writeAmendmentOps replaces an amendment's operations list on disk. Staging
// validates what it writes, so a shape only a hand edit or a partially applied
// batch can produce (a target that has since vanished, a cancel with no reason)
// has to be laid down directly, the same way setTaskStatus lays a pre-guard
// task status.
func writeAmendmentOps(t *testing.T, root string, a *store.Amendment, ops []store.AmendmentOp) *store.Amendment {
	t.Helper()
	path := filepath.Join(root, ".tiller", "amendments", a.ID+"-"+deref(t, a.Key)+".yml")
	var doc store.Amendment
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Operations = ops
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return loadAmendment(t, root, a.ID)
}

func deref(t *testing.T, s *string) string {
	t.Helper()
	require.NotNil(t, s)
	return *s
}

// --- the happy path: the Lantern shape ---

// TestApproveAmendmentAppliesTheLanternShape is the acceptance test for the field
// case: one amendment carrying a revise of the active feature, a cancel of the
// task at check naming its replacement, and the add-task that replaces it, all
// applied in one act.
func TestApproveAmendmentAppliesTheLanternShape(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	priorSpec := load(t, root).Features[featureID].AcceptedAgainst.Spec.Value
	priorEval := load(t, root).Features[featureID].AcceptedAgainst.Eval.Value

	reshapeTheSpec(t, root, featureID)

	a := openAmendment(t, root, deps, "iphone-only")
	_, err := mutate.StageAmendsDesign(load(t, root), deps, a, []string{"design/04-platform-and-constraints.md"})
	require.NoError(t, err)
	a = loadAmendment(t, root, a.ID)

	const newTaskKey = "app-experience/fast-logging/iphone-acceptance"
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: newTaskKey, Title: "iPhone-only fast logging acceptance",
		Preconditions: map[string]string{"key_absent": newTaskKey}})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "superseded under the iPhone-only decision",
		SupersededBy: newTaskKey, Preconditions: map[string]string{"target_status": "check"}})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
		Preconditions: map[string]string{
			"spec_revision": blobOf(t, root, lanternSpecRel),
			"eval_revision": blobOf(t, root, lanternEvalRel),
		}})

	res := approve(t, root, deps, a)

	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Len(t, res.Applied, 3, "every staged operation is reported back")
	require.Equal(t, []string{store.AmendmentOpAddTask, store.AmendmentOpCancel, store.AmendmentOpRevise},
		[]string{res.Applied[0].Op, res.Applied[1].Op, res.Applied[2].Op},
		"the result reports the operations in the order the human staged them")

	tree := load(t, root)

	// The amendment itself is terminal, and its staged record survives the flip.
	after := tree.Amendments[a.ID]
	require.Equal(t, store.AmendmentStatusApproved, after.Status)
	require.Len(t, after.Operations, 3, "approving does not rewrite the staged batch")
	require.Equal(t, []string{"design/04-platform-and-constraints.md"}, after.AmendsDesign,
		"amends_design rides through the status flip untouched")

	// The add op materialized a proposed task under the live feature.
	newID := res.Applied[0].NodeID
	newTask := tree.Tasks[newID]
	require.NotNil(t, newTask, "the staged add-task materialized")
	require.Equal(t, "proposed", newTask.Status)
	require.Equal(t, newTaskKey, deref(t, newTask.Key))
	require.Equal(t, featureID, newTask.FeatureID)
	require.FileExists(t, filepath.Join(root, store.TaskRelDir("app-experience", "fast-logging", "iphone-acceptance"), store.TaskProseFile))

	// The cancel executed and the supersession thread landed in BOTH directions,
	// its replacement resolved from the staged key the amendment named. Task
	// grain here; the feature grain is TestApproveAmendmentSupersedesDoneWork...
	require.Equal(t, "cancelled", tree.Tasks[taskID].Status)
	require.Equal(t, []string{taskID}, newTask.Supersedes,
		"the replacement records what it replaced")
	require.NotNil(t, tree.Tasks[taskID].SupersededBy,
		"the retired task records what replaced it, so the tree answers that from either end")
	require.Equal(t, newID, *tree.Tasks[taskID].SupersededBy)

	// The revise re-pinned the feature's basis onto the reviewed content and
	// staled the gate the contract change invalidated.
	f := tree.Features[featureID]
	require.Equal(t, blobOf(t, root, lanternSpecRel), f.AcceptedAgainst.Spec.Value)
	require.Equal(t, blobOf(t, root, lanternEvalRel), f.AcceptedAgainst.Eval.Value)
	require.NotEqual(t, priorSpec, f.AcceptedAgainst.Spec.Value, "the reviewed spec edit is re-recorded")
	require.NotEqual(t, priorEval, f.AcceptedAgainst.Eval.Value, "the reviewed contract edit is re-recorded")
	require.Equal(t, store.GateStatusStale, f.Gates["evals"].Status,
		"a changed contract stales the prior evals result, exactly as refresh-basis stales it")
	require.Equal(t, store.CriterionStatusStale, f.Gates["evals"].Criteria["C1"].Status)
	require.Equal(t, "active", f.Status, "revise re-pins a basis and moves no state")

}

// TestApproveAmendmentRecordsEveryOperationAsAnEvent pins the history trail: the
// apply emits the per-node event each operation implies, each naming the
// amendment as its provenance, plus one amendment_approved.
func TestApproveAmendmentRecordsEveryOperationAsAnEvent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	reshapeTheSpec(t, root, featureID)

	a := openAmendment(t, root, deps, "iphone-only")
	const newTaskKey = "app-experience/fast-logging/iphone-acceptance"
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: newTaskKey, Title: "iPhone-only fast logging acceptance"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "superseded under the iPhone-only decision",
		SupersededBy: newTaskKey})
	a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID})

	res := approve(t, root, deps, a)
	newID := res.Applied[0].NodeID

	evs, err := events.List(load(t, root))
	require.NoError(t, err)

	added := eventsOfType(evs, "add.task")
	require.Len(t, added, 2, "the fixture's own task plus the materialized one")
	var materialized *events.Event
	for i := range added {
		if containsStr(added[i].Targets, newID) {
			materialized = &added[i]
		}
	}
	require.NotNil(t, materialized, "the materialized task has its own add.task event")
	require.Contains(t, materialized.Targets, a.ID, "the event carries the amendment as provenance")
	require.Equal(t, a.ID, materialized.After["amendment"])

	cancelled := eventsOfType(evs, "cancel.task")
	require.Len(t, cancelled, 1)
	require.Contains(t, cancelled[0].Targets, taskID)
	require.Equal(t, "cancelled", cancelled[0].After["status"])
	require.Equal(t, "superseded under the iPhone-only decision", cancelled[0].After["reason"])
	require.Equal(t, newID, cancelled[0].After["superseded_by"], "the cancel event carries the supersession")
	require.Equal(t, a.ID, cancelled[0].After["amendment"])

	revised := eventsOfType(evs, "revise.feature")
	require.Len(t, revised, 1)
	require.Contains(t, revised[0].Targets, featureID)
	require.Equal(t, a.ID, revised[0].After["amendment"])

	approved := eventsOfType(evs, "amendment_approved")
	require.Len(t, approved, 1)
	require.Contains(t, approved[0].Targets, a.ID)
	require.Equal(t, store.AmendmentStatusProposed, approved[0].Before["status"])
	require.Equal(t, store.AmendmentStatusApproved, approved[0].After["status"])
}

func TestApproveAmendmentRefusesSupersedingWorkThatIsNotDoneWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/table-backed-logging", Title: "Table-backed logging", ChangeType: "replace"})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "table-backed-logging")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})

	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	require.Equal(t, "supersede_target_not_done", reasonOf(t, err))
	require.ErrorContains(t, err, "is active; supersede requires done work")
	requireNoWrites(t, before, snapshotDir(t, root))

	tree := load(t, root)
	require.Equal(t, "active", tree.Features[featureID].Status)
	require.Nil(t, tree.Features[featureID].SupersededBy)
	require.Empty(t, tree.Features[replacement.ID].Supersedes)
}

func TestApproveAmendmentReportsStaleTargetStatusBeforeSupersedeStateRule(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/table-backed-logging", ChangeType: "replace"})
	require.NoError(t, err)
	a := openAmendment(t, root, deps, "stale-before-state")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID,
		Preconditions: map[string]string{"target_status": "done"}})

	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale)
	require.Len(t, stale.Stale, 1)
	require.Equal(t, "active", stale.Stale[0].Actual)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveAmendmentReportsStaleTargetStatusBeforeSupersessionGraphInvalidity(t *testing.T) {
	t.Parallel()
	assertStale := func(t *testing.T, root string, deps mutate.Deps, a *store.Amendment, actual string) {
		t.Helper()
		before := snapshotDir(t, root)
		err := tryApprove(t, root, deps, a)
		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Equal(t, "target_status", stale.Stale[0].Precondition)
		require.Equal(t, actual, stale.Stale[0].Actual)
		requireNoWrites(t, before, snapshotDir(t, root))
	}

	t.Run("cross grain", func(t *testing.T) {
		root, deps, featureID, taskID := lanternFixture(t)
		a := openAmendment(t, root, deps, "stale-before-cross-grain")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featureID, By: taskID,
			Preconditions: map[string]string{"target_status": "done"}})
		assertStale(t, root, deps, a, "active")
	})

	t.Run("duplicate target", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		first, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/first-replacement", ChangeType: "replace"})
		require.NoError(t, err)
		second, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/second-replacement", ChangeType: "replace"})
		require.NoError(t, err)
		a := openAmendment(t, root, deps, "stale-before-duplicate-target")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featureID, By: first.ID,
			Preconditions: map[string]string{"target_status": "done"}})
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featureID, By: second.ID})
		assertStale(t, root, deps, a, "active")
	})

	t.Run("existing conflict", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
		first, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/first-replacement", ChangeType: "replace"})
		require.NoError(t, err)
		second, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/second-replacement", ChangeType: "replace"})
		require.NoError(t, err)
		setFeatureLineageOnDisk(t, root, "fast-logging", nil, first.ID)
		setFeatureLineageOnDisk(t, root, "first-replacement", []string{featureID}, "")
		a := openAmendment(t, root, deps, "stale-before-conflict")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featureID, By: second.ID,
			Preconditions: map[string]string{"target_status": "active"}})
		assertStale(t, root, deps, a, "done")
	})

	t.Run("cycle", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
		replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/replacement", ChangeType: "replace"})
		require.NoError(t, err)
		setFeatureStatusOnDisk(t, root, "app-experience", "replacement", "done")
		setFeatureLineageOnDisk(t, root, "fast-logging", nil, replacement.ID)
		setFeatureLineageOnDisk(t, root, "replacement", []string{featureID}, "")
		a := openAmendment(t, root, deps, "stale-before-cycle")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: replacement.ID, By: featureID,
			Preconditions: map[string]string{"target_status": "active"}})
		assertStale(t, root, deps, a, "done")
	})
}

// TestApproveAmendmentSupersedesDoneWorkWithoutMovingIt is the supersede
// operation's own shape at feature grain: both directions of the lineage
// recorded, the done target's status and completion record left exactly as they
// were, and the target's own event naming both nodes.
func TestApproveAmendmentSupersedesDoneWorkWithoutMovingIt(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")

	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/table-backed-logging", Title: "Table-backed logging", ChangeType: "replace"})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "table-backed-logging")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID,
		Preconditions: map[string]string{"target_status": "done"}})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, replacement.ID, res.Applied[0].Replacement)

	tree := load(t, root)
	require.Equal(t, []string{featureID}, tree.Features[replacement.ID].Supersedes,
		"the supersedes link is written on the replacement")
	require.NotNil(t, tree.Features[featureID].SupersededBy,
		"and the superseded_by link on the target, the direction drift's stale_spec exemption reads")
	require.Equal(t, replacement.ID, *tree.Features[featureID].SupersededBy)
	require.Equal(t, "done", tree.Features[featureID].Status,
		"the target's status does not move: done is a merge nobody un-merges")
	require.Empty(t, tree.Features[featureID].Supersedes)
	require.Nil(t, tree.Features[replacement.ID].SupersededBy, "the replacement is not itself retired")

	evs, err := events.List(tree)
	require.NoError(t, err)
	sup := eventsOfType(evs, "superseded")
	require.Len(t, sup, 1)
	require.Contains(t, sup[0].Targets, featureID)
	require.Contains(t, sup[0].Targets, replacement.ID)
	require.Equal(t, replacement.ID, sup[0].After["superseded_by"])
	require.Equal(t, a.ID, sup[0].After["amendment"])
}

// TestApproveAmendmentSupersedesUnionIsIdempotent covers the recovery contract's
// first arm directly: a supersedes link already on the replacement is not
// duplicated by a second amendment recording the same lineage.
func TestApproveAmendmentSupersedesUnionIsIdempotent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/table-backed-logging", Title: "Table-backed logging", ChangeType: "replace"})
	require.NoError(t, err)

	first := openAmendment(t, root, deps, "table-backed-logging")
	first = stage(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	approve(t, root, deps, first)

	second := openAmendment(t, root, deps, "table-backed-logging-again")
	second = stage(t, root, deps, second, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	approve(t, root, deps, second)

	tree := load(t, root)
	require.Equal(t, []string{featureID}, tree.Features[replacement.ID].Supersedes,
		"re-recording the same lineage unions rather than duplicating")
	require.Equal(t, replacement.ID, *tree.Features[featureID].SupersededBy,
		"and the backward leg is set to the same value again")
}

func TestApproveAmendmentRefusesCrossGrainSupersessionWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")

	cases := []mutate.StageInput{
		{Op: store.AmendmentOpSupersede, Target: featureID, By: taskID},
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced", SupersededBy: featureID},
	}
	for i, in := range cases {
		a := openAmendment(t, root, deps, fmt.Sprintf("grain-%d", i))
		a = stage(t, root, deps, a, in)
		before := snapshotDir(t, root)
		err := tryApprove(t, root, deps, a)
		require.Equal(t, "supersession_noun_mismatch", reasonOf(t, err))
		requireNoWrites(t, before, snapshotDir(t, root))
	}
}

func TestApproveAmendmentRefusesTwoReplacementsForOneTargetWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	first, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/first-replacement", ChangeType: "replace"})
	require.NoError(t, err)
	second, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/second-replacement", ChangeType: "replace"})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "two-replacements")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: first.ID})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: second.ID})

	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	require.Equal(t, "supersession_target_reused", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveAmendmentChecksTheNounOfAStagedReplacementWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	const stagedTask = "app-experience/fast-logging/staged-replacement"
	a := openAmendment(t, root, deps, "staged-grain")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: stagedTask, Title: "Wrong-grain replacement"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: stagedTask})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "supersession_noun_mismatch", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveAmendmentRefusesConflictingExistingSupersessionWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacementOne, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement-one", ChangeType: "replace"})
	require.NoError(t, err)
	replacementTwo, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement-two", ChangeType: "replace"})
	require.NoError(t, err)

	first := openAmendment(t, root, deps, "first-lineage")
	first = stage(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacementOne.ID})
	approve(t, root, deps, first)

	second := openAmendment(t, root, deps, "conflicting-lineage")
	second = stage(t, root, deps, second, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacementTwo.ID})
	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, second)
	require.Equal(t, "supersession_conflict", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))

	tree := load(t, root)
	require.Equal(t, replacementOne.ID, *tree.Features[featureID].SupersededBy)
	require.Equal(t, []string{featureID}, tree.Features[replacementOne.ID].Supersedes)
	require.Empty(t, tree.Features[replacementTwo.ID].Supersedes)
}

func TestApproveAmendmentRefusesSupersessionCycleWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement", ChangeType: "replace"})
	require.NoError(t, err)
	setFeatureStatusOnDisk(t, root, "app-experience", "replacement", "done")

	first := openAmendment(t, root, deps, "forward-lineage")
	first = stage(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	approve(t, root, deps, first)

	second := openAmendment(t, root, deps, "cyclic-lineage")
	second = stage(t, root, deps, second, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: replacement.ID, By: featureID})
	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, second)
	require.Equal(t, "supersession_cycle", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveAmendmentRefusesCycleFromCurrentOnDiskLineageWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement", ChangeType: "replace"})
	require.NoError(t, err)
	setFeatureStatusOnDisk(t, root, "app-experience", "replacement", "done")

	a := openAmendment(t, root, deps, "stale-loaded-lineage")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	stale := load(t, root)
	amendment := stale.Amendments[a.ID]

	setFeatureLineageOnDisk(t, root, "replacement", nil, featureID)
	before := snapshotDir(t, root)
	_, err = mutate.ApproveAmendment(stale, deps, amendment)
	require.Equal(t, "supersession_cycle", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))

	tree := load(t, root)
	require.Nil(t, tree.Features[featureID].SupersededBy)
	require.Equal(t, featureID, *tree.Features[replacement.ID].SupersededBy)
	require.Empty(t, tree.Features[replacement.ID].Supersedes)
}

func TestApproveAmendmentRefusesAClaimFromANodeAddedAfterTreeLoadWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement", ChangeType: "replace"})
	require.NoError(t, err)
	a := openAmendment(t, root, deps, "late-lineage-claim")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	stale := load(t, root)
	amendment := stale.Amendments[a.ID]

	late := *stale.Features[replacement.ID]
	late.ID = "F-999999"
	lateKey := "late-claimant"
	late.Key = &lateKey
	late.Supersedes = []string{featureID}
	late.SupersededBy = nil
	lateData, err := yaml.Marshal(&late)
	require.NoError(t, err)
	latePath := filepath.Join(root, ".tiller", "epics", "app-experience", "features", lateKey, "feature.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(latePath), 0o755))
	require.NoError(t, os.WriteFile(latePath, lateData, 0o644))

	before := snapshotDir(t, root)
	_, err = mutate.ApproveAmendment(stale, deps, amendment)
	require.Equal(t, "supersession_conflict", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))

	tree := load(t, root)
	require.Nil(t, tree.Features[featureID].SupersededBy)
	require.Empty(t, tree.Features[replacement.ID].Supersedes)
	require.Equal(t, []string{featureID}, tree.Features[late.ID].Supersedes)
}

func TestApproveAmendmentTreatsReverseOnlySupersessionAsProspectiveEdge(t *testing.T) {
	t.Parallel()
	t.Run("repairs the exact partial edge", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
		replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/replacement", ChangeType: "replace"})
		require.NoError(t, err)
		setFeatureLineageOnDisk(t, root, "replacement", []string{featureID}, "")

		a := openAmendment(t, root, deps, "repair-reverse-lineage")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
		approve(t, root, deps, a)

		tree := load(t, root)
		require.Equal(t, replacement.ID, *tree.Features[featureID].SupersededBy)
		require.Equal(t, []string{featureID}, tree.Features[replacement.ID].Supersedes)
	})

	t.Run("refuses the inverse cycle with no writes", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
		replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
			Key: "app-experience/replacement", ChangeType: "replace"})
		require.NoError(t, err)
		setFeatureStatusOnDisk(t, root, "app-experience", "replacement", "done")
		setFeatureLineageOnDisk(t, root, "replacement", []string{featureID}, "")

		a := openAmendment(t, root, deps, "inverse-reverse-lineage")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: replacement.ID, By: featureID})
		before := snapshotDir(t, root)
		err = tryApprove(t, root, deps, a)
		require.Equal(t, "supersession_cycle", reasonOf(t, err))
		requireNoWrites(t, before, snapshotDir(t, root))
	})
}

func TestApproveAmendmentRefusesInverseCycleWithAmbiguousReverseClaimsWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	first, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/first-claimant", ChangeType: "replace"})
	require.NoError(t, err)
	second, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/second-claimant", ChangeType: "replace"})
	require.NoError(t, err)
	setFeatureStatusOnDisk(t, root, "app-experience", "first-claimant", "done")
	setFeatureLineageOnDisk(t, root, "first-claimant", []string{featureID}, "")
	setFeatureLineageOnDisk(t, root, "second-claimant", []string{featureID}, "")

	a := openAmendment(t, root, deps, "ambiguous-reverse-cycle")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: first.ID, By: featureID})
	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	require.Equal(t, "supersession_cycle", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))

	tree := load(t, root)
	require.Nil(t, tree.Features[first.ID].SupersededBy)
	require.Equal(t, []string{featureID}, tree.Features[first.ID].Supersedes)
	require.Equal(t, []string{featureID}, tree.Features[second.ID].Supersedes)
	require.Empty(t, tree.Features[featureID].Supersedes)
}

func TestApproveAmendmentReportsTheDeterministicallyFirstCyclicBranch(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/replacement", ChangeType: "replace"})
	require.NoError(t, err)
	first, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/first-branch", ChangeType: "replace"})
	require.NoError(t, err)
	firstReturn, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/first-return", ChangeType: "replace"})
	require.NoError(t, err)
	second, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/second-branch", ChangeType: "replace"})
	require.NoError(t, err)
	secondReturn, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "app-experience/second-return", ChangeType: "replace"})
	require.NoError(t, err)

	setFeatureLineageOnDisk(t, root, "first-branch", []string{replacement.ID, firstReturn.ID}, "")
	setFeatureLineageOnDisk(t, root, "first-return", []string{first.ID}, "")
	setFeatureLineageOnDisk(t, root, "second-branch", []string{replacement.ID, secondReturn.ID}, "")
	setFeatureLineageOnDisk(t, root, "second-return", []string{second.ID}, "")

	a := openAmendment(t, root, deps, "deterministic-cycle")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: replacement.ID})
	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)

	expected := first.ID
	if second.ID < first.ID {
		expected = second.ID
	}
	require.Equal(t, "supersession_cycle", reasonOf(t, err))
	require.ErrorContains(t, err, "revisits "+expected)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentSupersedesATask is the supersede operation at task grain,
// the grain the field case's first casualty sat at. store.Task carried
// supersedes but no superseded_by until this build, so the backward leg is the
// half that had no writer AND no field.
func TestApproveAmendmentSupersedesATask(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	setTaskStatusOnDisk(t, root, taskID, "done")

	replacement, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "app-experience/fast-logging/iphone-acceptance", Title: "iPhone acceptance"})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: taskID, By: replacement.ID})
	approve(t, root, deps, a)

	tree := load(t, root)
	require.Equal(t, []string{taskID}, tree.Tasks[replacement.ID].Supersedes)
	require.NotNil(t, tree.Tasks[taskID].SupersededBy)
	require.Equal(t, replacement.ID, *tree.Tasks[taskID].SupersededBy)
	require.Equal(t, "done", tree.Tasks[taskID].Status, "a done task's status is untouched by the record")
}

// TestApproveAmendmentResolvesAStagedAddFeatureKeyAsTheReplacement pins the
// staged-key resolution on the supersede side: a --by naming an add-feature
// staged in the same amendment resolves to the node that add materialized.
func TestApproveAmendmentResolvesAStagedAddFeatureKeyAsTheReplacement(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	setFeatureStatusToDone(t, root, "app-experience", "fast-logging")

	const stagedKey = "app-experience/table-backed-logging"
	a := openAmendment(t, root, deps, "table-backed-logging")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: stagedKey, Title: "Table-backed logging", ChangeType: "replace"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: stagedKey})

	res := approve(t, root, deps, a)
	newID := res.Applied[0].NodeID
	require.Equal(t, newID, res.Applied[1].Replacement, "the staged key resolved to the materialized node")

	tree := load(t, root)
	require.Equal(t, "draft", tree.Features[newID].Status, "a materialized feature lands draft")
	require.Equal(t, "replace", tree.Features[newID].ChangeType)
	require.Equal(t, []string{featureID}, tree.Features[newID].Supersedes)
}

// TestApproveAmendmentResolvesAStagedAddTaskKeyAsTheReplacement is the task
// grain of TestApproveAmendmentResolvesAStagedAddFeatureKeyAsTheReplacement,
// above, and the cancel-side mirror of TestApproveAmendmentAppliesTheLanternShape's
// own staged-task resolution: a --by naming an add-task staged in the same
// amendment resolves to the node that add materialized, the same as the
// cancel/superseded_by direction the Lantern shape already pins.
func TestApproveAmendmentResolvesAStagedAddTaskKeyAsTheReplacement(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	setTaskStatusOnDisk(t, root, taskID, "done")

	const stagedKey = "app-experience/fast-logging/iphone-acceptance"
	a := openAmendment(t, root, deps, "iphone-acceptance")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: stagedKey, Title: "iPhone acceptance"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: taskID, By: stagedKey})

	res := approve(t, root, deps, a)
	newID := res.Applied[0].NodeID
	require.Equal(t, newID, res.Applied[1].Replacement, "the staged key resolved to the materialized node")

	tree := load(t, root)
	require.Equal(t, "proposed", tree.Tasks[newID].Status, "a materialized task lands proposed")
	require.Equal(t, []string{taskID}, tree.Tasks[newID].Supersedes)
}

// TestApproveAmendmentRefusesAStagedAddEpicKeyAsAReplacementWithNoWrites is the
// approve-side mirror of resolveStageReplacement's own deliberate epic
// exclusion (amendment.go:729-740, "Deliberately not store.AmendmentOpAddEpic
// here"): a supersede or a cancel only ever targets a feature or a task, so a
// staged add-epic key is never a legal replacement, live or staged. Staging
// refuses this at stage time (unknown_target: its loop never even looks at
// add-epic ops), so the only way the shape reaches approve at all is a hand
// edit, the same class of defect every other re-validation in this file
// guards against. resolveAmendment's staged map still carries add-epic
// entries (its own duplicate-staged-key check needs them, and add-question's
// blocks resolution needs epics resolvable too, just through a different map
// entirely — b.staged at materialize time, not this one), so without a guard
// at the replacement-resolution site specifically, the map's generic
// staged[raw] != "" fallback would resolve the key anyway.
func TestApproveAmendmentRefusesAStagedAddEpicKeyAsAReplacementWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)

	cases := []struct {
		name string
		ops  []store.AmendmentOp
	}{
		{
			name: "supersede",
			ops: []store.AmendmentOp{
				{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"},
				{Op: store.AmendmentOpSupersede, Target: featureID, By: "hardening"},
			},
		},
		{
			name: "cancel",
			ops: []store.AmendmentOp{
				{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"},
				{Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced", SupersededBy: "hardening"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := openAmendment(t, root, deps, "epic-replacement-"+c.name)
			a = writeAmendmentOps(t, root, a, c.ops)

			before := snapshotDir(t, root)
			err := tryApprove(t, root, deps, a)
			require.Equal(t, "staged_epic_replacement", reasonOf(t, err))
			require.Contains(t, err.Error(), "operation 1",
				"the refusal names the offending operation's index, the supersede/cancel, not the add-epic")
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

// TestApproveAmendmentMaterializesAnAddEpic pins add-epic's own materialize
// arm: the epic lands draft, carrying the op's Source as its
// own node provenance, with its add.epic event tagged with the amendment
// exactly as add-feature's and add-task's own materialized events are.
func TestApproveAmendmentMaterializesAnAddEpic(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hardening-epic")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
		Source: "docs/feedback/opus-review.md#L1-9",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, store.AmendmentOpAddEpic, res.Applied[0].Op)
	require.Equal(t, store.NounEpic, res.Applied[0].Noun)
	require.Equal(t, "draft", res.Applied[0].Status)
	newID := res.Applied[0].NodeID

	orderPath := filepath.Join(root, ".tiller", "order.yml")
	require.FileExists(t, orderPath, "approving one add-epic creates the root order")

	tree := load(t, root)
	epic := tree.Epics[newID]
	require.NotNil(t, epic, "the staged add-epic materialized")
	require.Equal(t, "draft", epic.Status)
	require.Equal(t, "hardening", deref(t, epic.Key))
	require.Equal(t, "docs/feedback/opus-review.md#L1-9", epic.Source,
		"the op's Source lands as the materialized epic's own node provenance")
	require.Equal(t, []string{newID}, tree.ProjectOrder.Epics,
		"the new epic is the sole explicit root-order member; approval does not fill older unlisted epics")

	evs, err := events.List(tree)
	require.NoError(t, err)
	added := eventsOfType(evs, "add.epic")
	require.Len(t, added, 2, "the fixture's own epic plus the materialized one")
	var materialized *events.Event
	for i := range added {
		if containsStr(added[i].Targets, newID) {
			materialized = &added[i]
		}
	}
	require.NotNil(t, materialized, "the materialized epic has its own add.epic event")
	require.Contains(t, materialized.Targets, a.ID, "the event carries the amendment as provenance")
	require.Equal(t, a.ID, materialized.After["amendment"])
	require.Equal(t, "approve amendment "+a.ID, materialized.Command,
		"the command recorded is the gate that ran, not an add verb nobody typed")
}

func TestApproveAmendmentAppendsAddedEpicsToPartialProjectOrderInStagedOrder(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	olderUnlistedID := resolveNode(t, load(t, root), store.NounEpic, "app-experience").NodeID()

	alpha, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{
		Key: "alpha-existing", Title: "Alpha existing",
	})
	require.NoError(t, err)
	zulu, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{
		Key: "zulu-existing", Title: "Zulu existing",
	})
	require.NoError(t, err)
	_, err = mutate.SetOrder(load(t, root), deps, mutate.SetOrderInput{
		Noun:  store.NounEpic,
		Nodes: []string{"zulu-existing", "alpha-existing"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{zulu.ID, alpha.ID}, load(t, root).ProjectOrder.Epics,
		"the fixture starts with a deliberately reversed partial list")

	a := openAmendment(t, root, deps, "ordered-epics")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Authentication provider",
		Statement: "Which auth provider?", Blocks: []string{featureID},
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Replacement provider title",
		Statement: "Which auth provider?", Blocks: []string{taskID},
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "observability", Title: "Observability",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 4)
	require.Equal(t, store.AmendmentOpAddEpic, res.Applied[0].Op)
	require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[1].Op)
	require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[2].Op)
	require.Equal(t, store.AmendmentOpAddEpic, res.Applied[3].Op)
	require.Equal(t, res.Applied[1].NodeID, res.Applied[2].NodeID,
		"the mixed question operations fold onto one question")

	order := load(t, root).ProjectOrder.Epics
	require.Equal(t, []string{zulu.ID, alpha.ID, res.Applied[0].NodeID, res.Applied[3].NodeID}, order,
		"existing entries stay byte-order stable and new epics append in staged order")
	require.NotContains(t, order, olderUnlistedID,
		"approval preserves a legal partial list instead of filling an older unlisted epic")
	require.NotContains(t, order, res.Applied[1].NodeID,
		"folded add-question results never enter the epic order")
}

func TestApproveAmendmentWithoutAddEpicDoesNotCreateOrRewriteProjectOrder(t *testing.T) {
	t.Parallel()
	t.Run("does not create an absent order", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		orderPath := filepath.Join(root, ".tiller", "order.yml")
		require.NoFileExists(t, orderPath)

		a := openAmendment(t, root, deps, "question-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpAddQuestion, Title: "Authentication provider",
			Statement: "Which auth provider?", Blocks: []string{featureID},
		})
		res := approve(t, root, deps, a)
		require.Len(t, res.Applied, 1)
		require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[0].Op)

		require.NoFileExists(t, orderPath,
			"a batch with no add-epic does not create root order state")
		require.Empty(t, load(t, root).ProjectOrder.Epics)
	})

	t.Run("does not rewrite an existing order", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		epicID := resolveNode(t, load(t, root), store.NounEpic, "app-experience").NodeID()
		_, err := mutate.SetOrder(load(t, root), deps, mutate.SetOrderInput{
			Noun: store.NounEpic, Nodes: []string{"app-experience"},
		})
		require.NoError(t, err)

		orderPath := filepath.Join(root, ".tiller", "order.yml")
		beforeBytes, err := os.ReadFile(orderPath)
		require.NoError(t, err)
		beforeFingerprint := snapshotDir(t, root)[filepath.Join(".tiller", "order.yml")]

		a := openAmendment(t, root, deps, "question-with-recorded-order")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpAddQuestion, Title: "Authentication provider",
			Statement: "Which auth provider?", Blocks: []string{featureID},
		})
		approve(t, root, deps, a)

		afterBytes, err := os.ReadFile(orderPath)
		require.NoError(t, err)
		require.Equal(t, beforeBytes, afterBytes, "the existing order bytes are untouched")
		require.Equal(t, beforeFingerprint, snapshotDir(t, root)[filepath.Join(".tiller", "order.yml")],
			"the existing order is not rewritten with identical bytes")
		require.Equal(t, []string{epicID}, load(t, root).ProjectOrder.Epics)
	})
}

func TestApproveAmendmentAddedEpicProjectOrderRollback(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	existingID := resolveNode(t, load(t, root), store.NounEpic, "app-experience").NodeID()
	_, err := mutate.SetOrder(load(t, root), deps, mutate.SetOrderInput{
		Noun: store.NounEpic, Nodes: []string{"app-experience"},
	})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "rollback-ordered-epic")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})

	orderPath := filepath.Join(root, ".tiller", "order.yml")
	beforeOrderBytes, err := os.ReadFile(orderPath)
	require.NoError(t, err)
	beforeTree := load(t, root)
	beforeAmendment := beforeTree.Amendments[a.ID]
	beforeEvents, err := events.List(beforeTree)
	require.NoError(t, err)
	beforeSnapshot := snapshotDir(t, root)
	// The mutation generation is an intentionally advanced cache token. The
	// restored order has its original bytes and membership, asserted below,
	// but atomic restoration necessarily gives it a new mtime.
	delete(beforeSnapshot, filepath.Join(".tiller", ".cache", "generation"))
	delete(beforeSnapshot, store.OrderRelPath)

	// Match the existing transaction-failure fixture: event writes are planned
	// after node and order writes, so refusing the destination exercises the
	// widest partial-apply window while leaving rollback sources writable.
	eventDir := filepath.Join(root, ".tiller", "events", "2026", "06", "27")
	require.NoError(t, os.MkdirAll(eventDir, 0o755))
	require.NoError(t, os.Chmod(eventDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(eventDir, 0o755) })

	err = tryApprove(t, root, deps, a)
	require.Error(t, err, "the forced event-write failure must abort the approval transaction")
	require.NoError(t, os.Chmod(eventDir, 0o755))

	afterOrderBytes, err := os.ReadFile(orderPath)
	require.NoError(t, err)
	require.Equal(t, beforeOrderBytes, afterOrderBytes, "rollback restores the exact prior order bytes")

	afterTree := load(t, root)
	require.Equal(t, []string{existingID}, afterTree.ProjectOrder.Epics,
		"rollback restores the exact prior root-order membership")
	require.Equal(t, beforeTree.Epics, afterTree.Epics, "no materialized epic survives the failed commit")
	require.Equal(t, beforeAmendment, afterTree.Amendments[a.ID],
		"the amendment remains proposed with its staged operations intact")
	require.Equal(t, store.AmendmentStatusProposed, afterTree.Amendments[a.ID].Status)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "hardening"))

	afterEvents, err := events.List(afterTree)
	require.NoError(t, err)
	require.Equal(t, beforeEvents, afterEvents, "no add or approval event survives the failed commit")
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
	afterSnapshot := snapshotDir(t, root)
	delete(afterSnapshot, filepath.Join(".tiller", ".cache", "generation"))
	delete(afterSnapshot, store.OrderRelPath)
	requireNoWrites(t, beforeSnapshot, afterSnapshot)
}

// TestApproveAmendmentMaterializesAnAddFeatureWithSource is add-epic's own
// TestApproveAmendmentMaterializesAnAddEpic, one grain down: materialize's
// AmendmentOpAddFeature arm builds FeatureInput without forwarding r.op.Source
// (a gap this test found rather than one wired deliberately,
// unlike add-epic's own arm, which task 2 built to carry Source through by
// name). FeatureInput has carried a Source field since before intake existed
// (`tiller add feature --source` is live, add.go), so the fix is one field on
// one struct literal in amendment_approve.go's materialize switch, mirroring
// the add-epic arm exactly; this test is the failing-first pin for it.
func TestApproveAmendmentMaterializesAnAddFeatureWithSource(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "csp-headers-feature")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "app-experience/csp-headers", Title: "CSP headers",
		ChangeType: "create", Source: "docs/feedback/opus-review.md#L9-11",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, store.AmendmentOpAddFeature, res.Applied[0].Op)
	newID := res.Applied[0].NodeID

	tree := load(t, root)
	feat := tree.Features[newID]
	require.NotNil(t, feat, "the staged add-feature materialized")
	require.Equal(t, "draft", feat.Status)
	require.Equal(t, "create", feat.ChangeType)
	require.Equal(t, "docs/feedback/opus-review.md#L9-11", feat.Source,
		"the op's Source lands as the materialized feature's own node provenance, the same wiring add-epic already gets")
}

// Amendment-created tasks do not inherit a sibling's implementation ownership.
func TestApproveAmendmentMintsUnclaimedTasks(t *testing.T) {
	t.Parallel()
	root, deps, _, existingTaskID := lanternFixture(t)
	require.Equal(t, "jc", load(t, root).Tasks[existingTaskID].Owner)

	a := openAmendment(t, root, deps, "log-export-unclaimed")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "app-experience/fast-logging/export-the-log", Title: "Export the log"})
	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	unclaimedTaskID := res.Applied[0].NodeID
	require.Empty(t, load(t, root).Tasks[unclaimedTaskID].Owner)

	_, err := mutate.ClaimTask(load(t, root), deps, unclaimedTaskID, "")
	require.NoError(t, err)

	b := openAmendment(t, root, deps, "log-export-claimed")
	b = stage(t, root, deps, b, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "app-experience/fast-logging/archive-the-log", Title: "Archive the log"})
	res2 := approve(t, root, deps, b)
	require.Len(t, res2.Applied, 1)
	claimedTaskID := res2.Applied[0].NodeID
	require.Empty(t, load(t, root).Tasks[claimedTaskID].Owner)
	require.Equal(t, "jc", load(t, root).Tasks[unclaimedTaskID].Owner)
}

// --- same-batch parenting at materialize ---

// TestApproveAmendmentMaterializesAFeatureUnderASameBatchStagedEpic pins
// materialize's own same-batch parenting allowance: an add-epic staged
// earlier in this same amendment has already materialized into b.staged by
// the time a later add-feature naming it is materialized, so
// planAddFeature's live-tree epic check is rescued the same way staging's own
// resolveStageParentEpic already rescues it at stage time (Task 1). One
// approve call materializes the epic, then the feature, in staged order, and
// the feature's own folder lands under the epic this same batch just minted.
func TestApproveAmendmentMaterializesAFeatureUnderASameBatchStagedEpic(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hardening-same-batch")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
		Source: "docs/feedback/opus-review.md#L1-3",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers",
		ChangeType: "create", Source: "docs/feedback/opus-review.md#L4-6",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 2)
	require.Equal(t, store.AmendmentOpAddEpic, res.Applied[0].Op, "AppliedOperation order mirrors staged order")
	require.Equal(t, store.AmendmentOpAddFeature, res.Applied[1].Op)
	epicID := res.Applied[0].NodeID
	featureID := res.Applied[1].NodeID

	tree := load(t, root)
	epic := tree.Epics[epicID]
	require.NotNil(t, epic, "the staged add-epic materialized")
	require.Equal(t, "hardening", deref(t, epic.Key))
	require.Equal(t, "docs/feedback/opus-review.md#L1-3", epic.Source, "the epic carries its own op's Source")

	feat := tree.Features[featureID]
	require.NotNil(t, feat, "the staged add-feature materialized under the same-batch epic")
	require.Equal(t, "hardening", feat.EpicKey, "tree reloaded shows the feature's epic key naming the epic this batch just minted")
	require.Equal(t, "hardening/csp-headers", deref(t, feat.Key))
	require.Equal(t, "docs/feedback/opus-review.md#L4-6", feat.Source, "the feature carries its own op's Source")

	dir := filepath.Join(root, ".tiller", "epics", "hardening", "features", "csp-headers")
	require.DirExists(t, dir, "the feature's folder lands under the new epic's key")
	require.FileExists(t, filepath.Join(dir, "feature.yml"))
	require.FileExists(t, filepath.Join(dir, "feature.md"))
}

// TestApproveAmendmentSameBatchEpicCollisionAbortsBeforeItsFeature is the
// atomicity pin for the new allowance: an add-epic op that collides at
// approve (another actor lands the same key live between stage and approve)
// refuses key_exists on its OWN op, which materialize's loop reaches before
// the add-feature staged after it, so the feature never lands and nothing at
// all is written — a same-batch feature never lands without its own epic.
func TestApproveAmendmentSameBatchEpicCollisionAbortsBeforeItsFeature(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hardening-collision")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create"})

	// Another actor mints the same epic key live, between stage and approve.
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "hardening", Title: "Hardening (already landed)"})
	require.NoError(t, err)

	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "key_exists", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentRefusesAHandEditedAddFeatureWithNoLiveOrStagedEpic pins
// materialize's own named, op-indexed refusal for the shape only a
// hand-edited amendment can reach: an add-feature op whose epic segment names
// no parent, live or staged. StageOperation refuses this before it ever
// reaches the amendment's own yml (resolveStageParentEpic's own
// parentUnknownErr, amendment.go), so the only way it lands here is a direct
// edit (writeAmendmentOps), the same class of otherwise-unreachable defect
// this file's own reviseTargetIncompleteErr guards elsewhere.
func TestApproveAmendmentRefusesAHandEditedAddFeatureWithNoLiveOrStagedEpic(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hand-edited")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpAddFeature, Key: "phantom/csp-headers", Title: "CSP headers", ChangeType: "create"},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "not_found", reasonOf(t, err))
	require.ErrorContains(t, err, "operation 0")
	require.ErrorContains(t, err, "phantom")
	require.ErrorContains(t, err, "staged")
	var nf *resolve.ErrNotFound
	require.ErrorAs(t, err, &nf, "the live resolver's *resolve.ErrNotFound still rides through Unwrap")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentAddFeatureEpicNounMismatchNeverRescuedByStagedParent
// pins a review finding: planAddFeature's stagedParent escape triggers
// only on *resolve.ErrNotFound, the same narrow gate resolveStageParentEpic
// uses at stage time (amendment.go) — any other resolver error must
// propagate unrescued. An id-shaped epic segment that resolves to a LIVE
// node of the wrong noun is *resolve.ErrNounMismatch, a different failure
// than "no such epic anywhere", and staging already refuses it unrescued
// (resolveStageParentEpic's own identical dispatch). The hand-edited batch
// below stages an add-epic under the fixture's own live feature id as its
// (unusual, but not forbidden at this layer) key, then an add-feature naming
// that same string as its epic segment: before the fix, b.stagedNoun would
// answer true for that key (the add-epic op DID materialize under it) and
// wrongly rescue the noun mismatch into a materialized feature; the fix
// gates the rescue on the error's concrete type first, so it never gets that
// far.
func TestApproveAmendmentAddFeatureEpicNounMismatchNeverRescuedByStagedParent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "noun-mismatch")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: featureID, Title: "Mislabeled"},
		{Op: store.AmendmentOpAddFeature, Key: featureID + "/csp-headers", Title: "CSP headers", ChangeType: "create"},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	var nm *resolve.ErrNounMismatch
	require.ErrorAs(t, err, &nm, "a noun mismatch must propagate unrescued, not fold into the staged-parent escape")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentRefusesDuplicateStagedAddEpicKeysWithNoWrites pins
// resolveAmendment's duplicate-staged-key guard extended to add-epic
// (mirroring the guard add-feature/add-task already had): two add-epic ops
// sharing one key in the same amendment refuse in phase 1, before
// materialize runs, with the same named duplicate_staged_key reason the
// other two add kinds get, and nothing is written.
//
// Without this guard, both ops would reach materialize with planAddEpic
// reporting Created: true for each (neither epic is on disk yet mid-batch),
// planning two writes at the identical epic.md/epic.yml destination.
// store.Tree's own write-planning still catches that duplicate destination
// and refuses the whole transaction before any write lands (an untyped
// store.EnvError at prepare, "planned write destination ... appears more
// than once", exit 3), so atomicity already held either way; what this
// guard adds is the same named, phase-1, exit-2 duplicate_staged_key
// refusal the other add kinds get, instead of a late untyped one.
func TestApproveAmendmentRefusesDuplicateStagedAddEpicKeysWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "dup-epic")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening one"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening two"})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "duplicate_staged_key", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentLeavesAnEarlierMaterializedAddEpicUnwrittenWhenALaterOpFails
// is the converse of TestApproveAmendmentSameBatchEpicCollisionAbortsBeforeItsFeature
// above and the pin the amendment After test names: that test fails on the FIRST op, so
// nothing before it ever ran. Here the FIRST op, a brand-new add-epic,
// genuinely succeeds all the way through materialize (registered into
// b.staged, its event appended, exactly as a batch that approved clean
// would), and only the SECOND op, another add-epic whose key a live epic
// already holds, fails. Nothing this batch touched reaches disk regardless:
// materialize fills only in-memory state and applyAmendment's one
// tree.Commit runs once, after the whole loop (this file's own header
// comment), so the first op's own success is never itself a write. Structural
// today; the arc same-batch epic parenting newly makes reachable, since
// before it an add-epic materializing successfully ahead of a later failing
// op had no op that could follow it.
func TestApproveAmendmentLeavesAnEarlierMaterializedAddEpicUnwrittenWhenALaterOpFails(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "converse-atomicity")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "app-experience", Title: "App experience (already landed)"})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "key_exists", reasonOf(t, err))
	require.Contains(t, err.Error(), "operation 1",
		"the SECOND op is the one that fails; the first op's own materialize already succeeded before this")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "hardening"),
		"the first op's own successful materialize never reached disk")
}

// setFeatureStatusToDone hand-lays done onto a feature.yml. Feature done is a
// rollup sync records over merged tasks, so a fixture that needs a done target
// for a supersede lays it rather than driving a full merge observation.
func setFeatureStatusToDone(t *testing.T, root, epicKey, featKey string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: active", "status: done", 1)
	require.NotEqual(t, string(data), out, "the replace must land or the fixture lies")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

func setFeatureLineageOnDisk(t *testing.T, root, featKey string, supersedes []string, supersededBy string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", "app-experience", "features", featKey, "feature.yml")
	var doc store.Feature
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Supersedes = supersedes
	if supersededBy == "" {
		doc.SupersededBy = nil
	} else {
		doc.SupersededBy = &supersededBy
	}
	out, err := yaml.Marshal(&doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// TestApproveAmendmentWithNoOperationsJustFlipsTheStatus is the degenerate case
// worth pinning: an amendment nobody staged onto still approves, which is what
// isolates the status write and its event from the operation passes.
func TestApproveAmendmentWithNoOperationsJustFlipsTheStatus(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")

	res := approve(t, root, deps, a)
	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Empty(t, res.Applied)
	require.Equal(t, store.AmendmentStatusApproved, load(t, root).Amendments[a.ID].Status)
}

func TestApproveAmendmentWithNoOperationsDoesNotReadOrRewriteLineageInputs(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "design-only")
	stale := load(t, root)
	amendment := stale.Amendments[a.ID]
	featurePath := filepath.Join(root, lanternFeatureDir, "feature.yml")
	var feature store.Feature
	data, err := os.ReadFile(featurePath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &feature))
	feature.ID = "F-999999"
	data, err = yaml.Marshal(&feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(featurePath, data, 0o644))
	featureRel := filepath.Join(lanternFeatureDir, "feature.yml")
	taskRel, err := filepath.Rel(root, taskYMLPath(t, root, taskID))
	require.NoError(t, err)
	before := snapshotDir(t, root)

	res, err := mutate.ApproveAmendment(stale, deps, amendment)
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusApproved, res.Status)

	after := snapshotDir(t, root)
	require.Equal(t, before[featureRel], after[featureRel], "a design-only approval must not read or rewrite unrelated feature state")
	require.Equal(t, before[taskRel], after[taskRel], "a design-only approval must not read or rewrite unrelated task state")
	require.Equal(t, store.AmendmentStatusApproved, load(t, root).Amendments[a.ID].Status)
}

// --- phase 1: preconditions ---

// TestApproveAmendmentRefusesAStalePreconditionWithNoWrites is the core
// atomicity promise: one stale precondition fails the whole approval, and the
// repository is byte-identical afterward.
func TestApproveAmendmentRefusesAStalePreconditionWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted",
		Preconditions: map[string]string{"target_status": "in_progress"}})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)

	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale)
	require.Equal(t, a.ID, stale.AmendmentID)
	require.Len(t, stale.Stale, 1)
	require.Equal(t, 0, stale.Stale[0].Index)
	require.Equal(t, store.AmendmentOpCancel, stale.Stale[0].Op)
	require.Equal(t, taskID, stale.Stale[0].Target)
	require.Equal(t, "target_status", stale.Stale[0].Precondition)
	require.Equal(t, "in_progress", stale.Stale[0].Expected)
	require.Equal(t, "check", stale.Stale[0].Actual)

	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status)
}

// TestApproveAmendmentCollectsEveryStalePrecondition is the never-first-only
// rule: three stale preconditions across two operations all come back, so the
// human fixes the batch in one pass instead of one refusal at a time.
func TestApproveAmendmentCollectsEveryStalePrecondition(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted",
		Preconditions: map[string]string{"target_status": "in_progress", "target_revision": "git:deadbee"}})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
		Preconditions: map[string]string{"spec_revision": "git:0000000"}})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)

	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale)
	require.Len(t, stale.Stale, 3, "every stale precondition on every operation is named")
	require.Equal(t, []string{"target_status", "target_revision", "spec_revision"},
		[]string{stale.Stale[0].Precondition, stale.Stale[1].Precondition, stale.Stale[2].Precondition},
		"the report is deterministic: operation order, then the fixed precondition order")
	require.Equal(t, []int{0, 0, 1}, []int{stale.Stale[0].Index, stale.Stale[1].Index, stale.Stale[2].Index})
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentPreconditionArms walks each precondition's own comparison,
// the satisfied case and the stale case, so the reading of each name is pinned
// rather than inferred from the collection test above.
func TestApproveAmendmentPreconditionArms(t *testing.T) {
	t.Parallel()
	t.Run("target_revision reads the accepted anchor of a live target", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		anchor := load(t, root).Features[featureID].AcceptedAgainst.Commit.Value
		require.NotEmpty(t, anchor)

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID,
			Preconditions: map[string]string{"target_revision": anchor}})
		approve(t, root, deps, a)
	})

	t.Run("target_revision reads the verified anchor of a done target", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		accepted := load(t, root).Features[featureID].AcceptedAgainst.Commit.Value
		verified := "git:1111111"
		setFeatureVerifiedCommit(t, root, featureID, verified)

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID,
			Preconditions: map[string]string{"target_revision": accepted}})

		err := tryApprove(t, root, deps, a)
		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Equal(t, verified, stale.Stale[0].Actual,
			"a done target is compared against verified_against.commit, not the accepted anchor")
	})

	t.Run("target_revision on a target with no anchor is named as such", func(t *testing.T) {
		root, deps, _, _ := lanternFixture(t)
		// A task added mid-flight carries no accepted_against until its own
		// approve, so it is the honest no-anchor case.
		added, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
			Key: "app-experience/fast-logging/iphone-acceptance", Title: "iPhone acceptance"})
		require.NoError(t, err)
		require.Nil(t, load(t, root).Tasks[added.ID].AcceptedAgainst)

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpCancel, Target: added.ID, Reason: "wrong shape",
			Preconditions: map[string]string{"target_revision": "git:abc1234"}})

		err = tryApprove(t, root, deps, a)
		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Empty(t, stale.Stale[0].Actual)
		require.NotEmpty(t, stale.Stale[0].Note, "a missing anchor is named rather than reported as a blank mismatch")
	})

	t.Run("key_absent refuses a key a live node already holds", func(t *testing.T) {
		root, deps, _, taskID := lanternFixture(t)
		held := deref(t, load(t, root).Tasks[taskID].Key)

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpAddTask, Key: "app-experience/fast-logging/iphone-acceptance",
			Title: "iPhone acceptance", Preconditions: map[string]string{"key_absent": held}})

		err := tryApprove(t, root, deps, a)
		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Equal(t, "key_absent", stale.Stale[0].Precondition)
		require.Equal(t, taskID, stale.Stale[0].Actual, "the refusal names the node that holds the key")
	})

	// key_absent is the one precondition kind add-epic applies,
	// and an epic key is a live node's key exactly as a feature's or a task's
	// is: keyHolder has to see epics too, or this precondition would report a
	// held epic key as free.
	t.Run("key_absent refuses an epic key a live node already holds", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		held := load(t, root).Features[featureID].EpicKey

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpAddEpic, Key: "hardening",
			Title: "Hardening", Preconditions: map[string]string{"key_absent": held}})

		err := tryApprove(t, root, deps, a)
		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Equal(t, "key_absent", stale.Stale[0].Precondition)
		var epicID string
		for id, e := range load(t, root).Epics {
			if e.Key != nil && *e.Key == held {
				epicID = id
			}
		}
		require.NotEmpty(t, epicID, "the fixture's own epic must resolve by key")
		require.Equal(t, epicID, stale.Stale[0].Actual, "the refusal names the epic that holds the key")
	})

	t.Run("spec_revision and eval_revision hash the current working tree", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		reshapeTheSpec(t, root, featureID)

		a := openAmendment(t, root, deps, "iphone-only")
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID,
			Preconditions: map[string]string{
				"spec_revision": blobOf(t, root, lanternSpecRel),
				"eval_revision": blobOf(t, root, lanternEvalRel),
			}})
		// Pinned against the current content, the operation applies.
		approve(t, root, deps, a)
	})

	t.Run("an edit after staging stales the pinned revision", func(t *testing.T) {
		root, deps, featureID, _ := lanternFixture(t)
		a := openAmendment(t, root, deps, "iphone-only")
		pinned := blobOf(t, root, lanternSpecRel)
		a = stage(t, root, deps, a, mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID,
			Preconditions: map[string]string{"spec_revision": pinned}})

		reshapeTheSpec(t, root, featureID)
		before := snapshotDir(t, root)
		err := tryApprove(t, root, deps, a)

		var stale *mutate.AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Len(t, stale.Stale, 1)
		require.Equal(t, pinned, stale.Stale[0].Expected)
		require.Equal(t, blobOf(t, root, lanternSpecRel), stale.Stale[0].Actual)
		requireNoWrites(t, before, snapshotDir(t, root))
	})
}

// TestApproveAmendmentReadsTheTargetStateThatIsCurrentNotTheOneLoaded is the
// difference between "validate against the tree" and "validate against the
// CURRENT tree". A caller holds a loaded tree while it stages and reviews; if
// the target moves in between (a parallel worker, a second agent, a human at
// another terminal), the precondition has to see the move. Nothing about the
// loaded snapshot is a lie, it is simply not the question the gate asks.
func TestApproveAmendmentReadsTheTargetStateThatIsCurrentNotTheOneLoaded(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted",
		Preconditions: map[string]string{"target_status": "check"}})

	// The tree the gate is handed, loaded BEFORE the target moves.
	stale := load(t, root)
	amendment := stale.Amendments[a.ID]
	require.Equal(t, "check", stale.Tasks[taskID].Status)

	setTaskStatusOnDisk(t, root, taskID, "in_progress")

	before := snapshotDir(t, root)
	_, err := mutate.ApproveAmendment(stale, deps, amendment)

	var staleErr *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &staleErr, "the precondition must be read against the current target, not the loaded copy")
	require.Len(t, staleErr.Stale, 1)
	require.Equal(t, "target_status", staleErr.Stale[0].Precondition)
	require.Equal(t, "in_progress", staleErr.Stale[0].Actual)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentAppliedIsKeyedByStagedIndex is the differentiating case
// for how the result is assembled: an amendment whose cancel is staged BEFORE
// the add it names as the replacement. Apply order and staged order diverge
// there (adds always materialize first), so a result appended in apply order
// would report the two operations swapped against the amendment file the human
// reviewed. Staging refuses this order (a staged key must already be on the
// amendment when the cancel names it), so the batch is laid down directly.
func TestApproveAmendmentAppliedIsKeyedByStagedIndex(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	const newTaskKey = "app-experience/fast-logging/iphone-acceptance"

	a := openAmendment(t, root, deps, "iphone-only")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced", SupersededBy: newTaskKey},
		{Op: store.AmendmentOpAddTask, Key: newTaskKey, Title: "iPhone acceptance"},
	})

	res := approve(t, root, deps, a)

	require.Equal(t, store.AmendmentOpCancel, res.Applied[0].Op, "index 0 is the operation staged first")
	require.Equal(t, 0, res.Applied[0].Index)
	require.Equal(t, taskID, res.Applied[0].NodeID)
	require.Equal(t, store.AmendmentOpAddTask, res.Applied[1].Op)
	require.Equal(t, 1, res.Applied[1].Index)

	// And the add still ran FIRST, which is what let the cancel resolve its
	// replacement at all.
	newID := res.Applied[1].NodeID
	require.Equal(t, newID, res.Applied[0].Replacement)
	tree := load(t, root)
	require.Equal(t, []string{taskID}, tree.Tasks[newID].Supersedes)
	require.Equal(t, newID, *tree.Tasks[taskID].SupersededBy)
}

// TestApproveAmendmentRefusesASelfSupersession covers the apply-side half of the
// guard staging already applies: a hand-laid batch naming a node as its own
// replacement would write supersedes and superseded_by pointing at one id, which
// every consumer reads as a human's recorded decision (drift exempts a
// superseded feature from stale_spec on that field alone) while recording
// nothing anyone decided.
func TestApproveAmendmentRefusesASelfSupersession(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpSupersede, Target: featureID, By: featureID},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "self_supersession", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// setFeatureVerifiedCommit lays a done feature carrying a verified_against
// commit anchor, the leg a done target's target_revision is compared against.
func setFeatureVerifiedCommit(t *testing.T, root, featureID, commit string) {
	t.Helper()
	path := filepath.Join(root, lanternFeatureDir, "feature.yml")
	var doc store.Feature
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Equal(t, featureID, doc.ID)
	doc.Status = "done"
	doc.VerifiedAgainst.Commit = store.RevisionRef{Kind: store.KindGitCommit, Value: commit}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// --- usage refusals ---

// TestApproveAmendmentRefusesWhenNotProposed pins the one state the gate runs
// from. Approving twice is the common way to reach it, which is also what makes
// a re-approve after a completed batch safe.
func TestApproveAmendmentRefusesWhenNotProposed(t *testing.T) {
	t.Parallel()
	for _, status := range []string{store.AmendmentStatusApproved, store.AmendmentStatusWithdrawn} {
		t.Run(status, func(t *testing.T) {
			root, deps, _, _ := lanternFixture(t)
			a := openAmendment(t, root, deps, "iphone-only")
			setAmendmentStatus(t, root, a.ID, status)
			a = loadAmendment(t, root, a.ID)

			err := tryApprove(t, root, deps, a)
			require.Error(t, err)
			require.Equal(t, "amendment_not_open", reasonOf(t, err))
			require.Contains(t, err.Error(), status, "the refusal names the actual status")
		})
	}
}

// TestApproveAmendmentRefusesACancelWithNoReason holds cancel's own discipline
// at the amendment grain: a cancelled node with no reason is a dead branch
// nobody can explain, and the amendment's approve is the act that cancels.
func TestApproveAmendmentRefusesACancelWithNoReason(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "   "},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "missing_reason", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesAVanishedTarget covers the tree moving under a
// staged batch in the one way a precondition cannot describe: the target is
// gone, so there is no status or revision left to compare.
func TestApproveAmendmentRefusesAVanishedTarget(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted"})

	require.NoError(t, os.RemoveAll(filepath.Join(root, store.TaskRelDir("app-experience", "fast-logging", "ipad-logging"))))

	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "unknown_target", reasonOf(t, err))
}

// TestApproveAmendmentRefusesAnUnresolvableReplacement is the all-or-nothing
// half of staged-key resolution: a superseded_by naming neither a live node nor
// an add op in this batch fails the whole approval rather than dropping the
// lineage silently.
func TestApproveAmendmentRefusesAnUnresolvableReplacement(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced",
			SupersededBy: "app-experience/fast-logging/never-staged"},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "unknown_target", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesAPreconditionItCannotEvaluate closes the gap
// between the two validators: store.ValidateAmendment judges an operation's
// fields and not its precondition map, so a hand-edited amendment could name a
// precondition approve would silently skip, and a guard nobody compares reads on
// the review surface as a guard that held.
func TestApproveAmendmentRefusesAPreconditionItCannotEvaluate(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted",
			Preconditions: map[string]string{"target_sha": "git:abc1234"}},
	})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "precondition_invalid", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// reasonOf reads the stable reason discriminator off a usage refusal, the token
// the CLI keys its exit-2 envelope on.
func reasonOf(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	type reasoner interface{ Reason() string }
	var r reasoner
	require.ErrorAs(t, err, &r, "expected a refusal carrying a reason: %v", err)
	return r.Reason()
}

// --- the recovery contract ---

// TestApproveAmendmentRefusesAnAddWhoseKeyIsAlreadyHeld is the recovery
// contract's second arm: an add op whose node already exists refuses loudly
// rather than folding into the idempotent no-op `tiller add` gives, because an
// applied batch must never report work it did not do.
func TestApproveAmendmentRefusesAnAddWhoseKeyIsAlreadyHeld(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	const newTaskKey = "app-experience/fast-logging/iphone-acceptance"

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: newTaskKey, Title: "iPhone acceptance"})

	// The batch is interrupted after this op landed, then re-approved.
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: newTaskKey, Title: "iPhone acceptance"})
	require.NoError(t, err)

	before := snapshotDir(t, root)
	err = tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "key_exists", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentRefusesAnAddEpicWhoseKeyIsAlreadyHeld is add-epic's own
// arm of the recovery contract's second case, mirroring
// TestApproveAmendmentRefusesAnAddWhoseKeyIsAlreadyHeld one grain up: epics
// are roots, so the collision here is with the fixture's own epic rather than
// with a re-interrupted batch.
func TestApproveAmendmentRefusesAnAddEpicWhoseKeyIsAlreadyHeld(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "app-experience", Title: "App experience, again"})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "key_exists", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentMaterializesAnAddQuestion pins add-question's own
// materialize arm: the question lands open on the shared
// ledger, carrying the op's Source as its own provenance and its Blocks
// resolved to the existing feature's id, with its add.question event tagged
// with the amendment exactly as the other materialized add kinds' events are.
func TestApproveAmendmentMaterializesAnAddQuestion(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hardening-question")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Legacy parser", Statement: "Keep the legacy parser?",
		Blocks: []string{featureID}, Source: "docs/feedback/opus-review.md#L20-24",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[0].Op)
	require.Equal(t, store.NounQuestion, res.Applied[0].Noun)
	require.Equal(t, "open", res.Applied[0].Status)
	qid := res.Applied[0].NodeID

	tree := load(t, root)
	q := tree.Questions[qid]
	require.NotNil(t, q, "the staged add-question materialized")
	require.Equal(t, "Legacy parser", q.Title)
	require.Equal(t, "Keep the legacy parser?", q.Statement)
	require.Equal(t, "open", q.Status)
	require.Equal(t, []string{featureID}, q.Blocks)
	require.Equal(t, "docs/feedback/opus-review.md#L20-24", q.Source,
		"the op's Source lands as the materialized question's own node provenance")

	evs, err := events.List(tree)
	require.NoError(t, err)
	added := eventsOfType(evs, "add.question")
	require.Len(t, added, 1)
	require.Contains(t, added[0].Targets, a.ID, "the event carries the amendment as provenance")
	require.Equal(t, "Legacy parser", added[0].After["title"])
	require.Equal(t, a.ID, added[0].After["amendment"])
	require.Equal(t, "approve amendment "+a.ID, added[0].Command,
		"the command recorded is the gate that ran, not add question nobody typed")
}

// TestApproveAmendmentAddQuestionBlocksAFeatureAddedInTheSameBatch is the
// acceptance's heart: a question can block a node the SAME
// amendment adds. The add-feature materializes first (materialize's own
// two-pass order), so by the time the add-question op folds, its staged-key
// Blocks entry resolves to the feature's freshly minted id, the same
// same-batch allowance a supersede's --by already gets against a staged key.
func TestApproveAmendmentAddQuestionBlocksAFeatureAddedInTheSameBatch(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	const stagedKey = "app-experience/table-backed-logging"
	a := openAmendment(t, root, deps, "table-backed-logging")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: stagedKey, Title: "Table-backed logging", ChangeType: "create"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Storage engine", Statement: "Which storage engine?",
		Blocks: []string{stagedKey}})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 2)
	newFeatureID := res.Applied[0].NodeID
	require.Equal(t, store.AmendmentOpAddFeature, res.Applied[0].Op)
	qid := res.Applied[1].NodeID
	require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[1].Op)

	tree := load(t, root)
	require.NotNil(t, tree.Features[newFeatureID], "the staged feature materialized")
	q := tree.Questions[qid]
	require.NotNil(t, q, "the staged question materialized")
	require.Equal(t, "Storage engine", q.Title)
	require.Equal(t, "Which storage engine?", q.Statement)
	require.Equal(t, []string{newFeatureID}, q.Blocks,
		"the staged key resolved to the materialized feature's id")
}

// TestApproveAmendmentAddQuestionBlocksAnEpicAddedInTheSameBatch mirrors
// TestApproveAmendmentAddQuestionBlocksAFeatureAddedInTheSameBatch immediately
// above with the blocks target swapped from a staged feature to a staged
// epic (#57's carried note: no pin exercised an add-question blocking a
// staged EPIC key through approve specifically). The add-epic materializes
// first (materialize's own two-pass order, the identical shape the feature
// case above rests on), so by the time the add-question op folds, its
// staged-key Blocks entry resolves to the epic's freshly minted id.
func TestApproveAmendmentAddQuestionBlocksAnEpicAddedInTheSameBatch(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	const stagedKey = "hardening"
	a := openAmendment(t, root, deps, "hardening-question")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: stagedKey, Title: "Hardening"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Hardening timing", Statement: "Land this epic now?",
		Blocks: []string{stagedKey}})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 2)
	newEpicID := res.Applied[0].NodeID
	require.Equal(t, store.AmendmentOpAddEpic, res.Applied[0].Op)
	qid := res.Applied[1].NodeID
	require.Equal(t, store.AmendmentOpAddQuestion, res.Applied[1].Op)

	tree := load(t, root)
	require.NotNil(t, tree.Epics[newEpicID], "the staged epic materialized")
	q := tree.Questions[qid]
	require.NotNil(t, q, "the staged question materialized")
	require.Equal(t, "Hardening timing", q.Title)
	require.Equal(t, "Land this epic now?", q.Statement)
	require.Equal(t, []string{newEpicID}, q.Blocks,
		"the staged key resolved to the materialized epic's id")
}

// TestApproveAmendmentAddQuestionDuplicateStatementUnionsBlocksAgainstALiveQuestion
// pins one arm of the acceptance's duplicate rule: a
// staged add-question naming a statement that already exists LIVE on the
// ledger is not a batch lying about what it did (unlike add-epic/
// add-feature/add-task's own key_exists refusal); it follows AddQuestion's
// own idempotency rule, unioning the new block into the existing open entry
// and reporting THAT entry's id.
func TestApproveAmendmentAddQuestionDuplicateStatementUnionsBlocksAgainstALiveQuestion(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	existing, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
	})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "auth-provider")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Replacement provider title", Statement: "Which auth provider?",
		Blocks: []string{featureID},
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, existing.ID, res.Applied[0].NodeID, "the duplicate statement resolved to the existing question")
	require.Equal(t, mutate.AppliedUpdated, res.Applied[0].Outcome, "a real union happened")
	require.Empty(t, res.Applied[0].Note, "a note only explains a no-op, and this genuinely changed something")

	tree := load(t, root)
	require.Len(t, tree.Questions, 1, "no duplicate ledger entry")
	require.Equal(t, []string{featureID}, tree.Questions[existing.ID].Blocks, "the new block unioned in")
	require.Equal(t, "Authentication provider", tree.Questions[existing.ID].Title,
		"a duplicate statement never overwrites an existing title")
}

// TestApproveAmendmentAddQuestionReasksACancelledQuestion pins the cancelled
// arm of the duplicate rule: a staged add-question whose statement matches a
// cancelled entry is not a duplicate, so approve mints a fresh open question
// and leaves the cancelled one on record.
func TestApproveAmendmentAddQuestionReasksACancelledQuestion(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	old, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
		Title: "Authentication provider", Statement: "Which auth provider?",
	})
	require.NoError(t, err)
	_, err = mutate.CancelQuestion(load(t, root), deps, mutate.CancelInput{Node: old.ID, Reason: "moot"})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "auth-provider-again")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Authentication provider", Statement: "Which auth provider?",
		Blocks: []string{featureID},
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.NotEqual(t, old.ID, res.Applied[0].NodeID, "a cancelled match mints a new question")

	tree := load(t, root)
	require.Len(t, tree.Questions, 2)
	require.Equal(t, "cancelled", tree.Questions[old.ID].Status)
	require.Equal(t, "open", tree.Questions[res.Applied[0].NodeID].Status)
}

func TestApproveAmendmentAddQuestionBackfillsLegacyTitleWithoutReopeningOrOverwriting(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	ledger := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(ledger), 0o755))
	require.NoError(t, os.WriteFile(ledger, []byte(
		"questions:\n"+
			"  - id: Q-legacy\n"+
			"    statement: Which schema version is canonical?\n"+
			"    owner: jc\n"+
			"    status: answered\n"+
			"    answer: The 2025 revision.\n"), 0o644))

	a := openAmendment(t, root, deps, "schema-version")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op:        store.AmendmentOpAddQuestion,
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, "Q-legacy", res.Applied[0].NodeID)
	require.Equal(t, mutate.AppliedUpdated, res.Applied[0].Outcome)

	q := load(t, root).Questions["Q-legacy"]
	require.Equal(t, "Canonical schema version", q.Title)
	require.Equal(t, "Which schema version is canonical?", q.Statement)
	require.Equal(t, "answered", q.Status, "title backfill does not reopen a closed question")
	require.NotNil(t, q.Answer)
	require.Equal(t, "The 2025 revision.", *q.Answer)
}

// TestApproveAmendmentAddQuestionSameBatchDuplicateStatementFoldsIntoOneEntry
// pins the other arm: two add-question ops staged in ONE batch sharing a
// statement is not a key collision (a question has no key to collide on,
// unlike two add-epics sharing a key, which DOES refuse duplicate_staged_key)
// but the same union-by-statement rule, landing exactly one ledger entry
// with both ops' blocks combined.
func TestApproveAmendmentAddQuestionSameBatchDuplicateStatementFoldsIntoOneEntry(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)

	a := openAmendment(t, root, deps, "dup-question")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Authentication provider", Statement: "Which auth provider?", Blocks: []string{featureID}})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Replacement provider title", Statement: "Which auth provider?", Blocks: []string{taskID}})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 2)
	require.Equal(t, res.Applied[0].NodeID, res.Applied[1].NodeID,
		"the second op's statement matched the first's own just-created entry")
	require.Equal(t, mutate.AppliedCreated, res.Applied[0].Outcome, "the first op minted a fresh question")
	require.Equal(t, mutate.AppliedUpdated, res.Applied[1].Outcome, "the second op unioned a genuinely new block into it")

	tree := load(t, root)
	require.Len(t, tree.Questions, 1, "no duplicate ledger entry")
	require.ElementsMatch(t, []string{featureID, taskID}, tree.Questions[res.Applied[0].NodeID].Blocks,
		"both ops' blocks landed on the one entry")
	require.Equal(t, "Authentication provider", tree.Questions[res.Applied[0].NodeID].Title,
		"the later same-batch duplicate does not overwrite the first title")
}

// TestApproveAmendmentAddQuestionNoOpReadGuardsTheLedger pins finding #1 of
// the review: a staged add-question folding onto a live CLOSED (answered)
// same-statement question must not read as if it did anything.
// AppliedOperation.Outcome reports AppliedNoChange rather than
// AppliedCreated or AppliedUpdated, its Note names the staged Blocks and
// Source that never landed, the ledger's own bytes are unchanged (the
// no-op read-guard branch, applyAmendment's own comment on b.questions),
// and approve still succeeds — the batch honestly reports doing nothing
// rather than either lying about it or refusing an amendment that staged
// nothing wrong.
func TestApproveAmendmentAddQuestionNoOpReadGuardsTheLedger(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	answered, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
	})
	require.NoError(t, err)
	_, err = mutate.AnswerQuestion(load(t, root), deps, deps.Rev, mutate.AnswerInput{
		Node: answered.ID, Answer: "Devise.",
	})
	require.NoError(t, err)

	ledgerPath := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	before, err := os.ReadFile(ledgerPath)
	require.NoError(t, err)
	beforeSnapshot := snapshotDir(t, root)

	a := openAmendment(t, root, deps, "already-answered")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Replacement provider title", Statement: "Which auth provider?",
		Blocks: []string{featureID}, Source: "docs/feedback/opus-review.md#L1-2",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, answered.ID, res.Applied[0].NodeID)
	require.Equal(t, "answered", res.Applied[0].Status)
	require.Equal(t, mutate.AppliedNoChange, res.Applied[0].Outcome, "nothing landed on an already-answered question")
	require.Contains(t, res.Applied[0].Note, "blocks", "the note names the dropped blocks")
	require.Contains(t, res.Applied[0].Note, "source", "the note names the dropped source")
	require.Contains(t, res.Applied[0].Note, "answered", "the note explains why: the question is closed")

	// The ledger is untouched — not just unchanged in content, but never
	// even opened for a write (requireNoWrites' mtime leg would catch a
	// rewrite of identical bytes, matching the read-guard-not-write
	// discipline every other undirtied node in this batch already gets).
	after, err := os.ReadFile(ledgerPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "the ledger's bytes are untouched")
	afterSnapshot := snapshotDir(t, root)
	require.Equal(t, beforeSnapshot[".tiller/decisions/open-questions.yml"], afterSnapshot[".tiller/decisions/open-questions.yml"],
		"the ledger was read-guarded, not rewritten with identical bytes (the mtime leg snapshotDir exists for)")

	// approve still succeeded and flipped the amendment's own status: a
	// no-op fold is not a refusal, it is an honest report of nothing to do.
	require.Equal(t, store.AmendmentStatusApproved, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentRefusesADoubleMintedQuestionID pins the double-mint
// guard (question.go's foldQuestion): a broken or deterministic minter
// handing back the SAME Q- token for two different add-question ops in one
// batch is caught with a named, typed mint_collision refusal (the same
// shape mintIn's own neighboring mint_exhausted refusal carries, add.go)
// rather than an untyped error, and nothing is written.
func TestApproveAmendmentRefusesADoubleMintedQuestionID(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	seq := 0
	deps.Mint = func(prefix string) string {
		switch prefix {
		case "Q":
			return "Q-collide"
		case "EV":
			return id.Mint(prefix)
		default:
			seq++
			return fmt.Sprintf("%s-%06d", prefix, seq)
		}
	}

	a := openAmendment(t, root, deps, "double-mint")
	a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "First", Statement: "First question?"})
	a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Second", Statement: "Second question?"})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "mint_collision", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
}

// TestApproveAmendmentRefusesACancelOfACancelledNode is the recovery contract's
// third arm. The lifecycle engine reads a second cancel as an idempotent no-op,
// which is right for a human typing the verb twice and wrong here: a re-approved
// batch that silently accepted it would claim to have cancelled work this run
// never touched.
func TestApproveAmendmentRefusesACancelOfACancelledNode(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted"})

	driveWith(t, root, deps, store.NounTask, taskID,
		mutate.TransitionInput{Verb: "cancel", Reason: "the interrupted batch got this far"})
	require.Equal(t, "cancelled", load(t, root).Tasks[taskID].Status)

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "already_cancelled", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesAnIllegalCancel keeps the amendment inside the same
// state machine the human-only cancel verb obeys: only the actor differs, never
// the legality. A done task has no cancel edge, and the amendment cannot invent
// one.
func TestApproveAmendmentRefusesAnIllegalCancel(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted"})

	setTaskStatusOnDisk(t, root, taskID, "done")

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	var illegal *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &illegal)
	require.Equal(t, "cancel", illegal.Verb)
	require.Equal(t, "done", illegal.From)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRevisePinIsIdempotent is the fourth recovery arm: a
// re-applied revise re-records the same content hash, so an interrupted batch
// re-approved after its revise landed changes nothing about the pin.
func TestApproveAmendmentRevisePinIsIdempotent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	reshapeTheSpec(t, root, featureID)

	first := openAmendment(t, root, deps, "iphone-only")
	first = stage(t, root, deps, first, mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID})
	approve(t, root, deps, first)
	pinned := load(t, root).Features[featureID].AcceptedAgainst

	second := openAmendment(t, root, deps, "iphone-only-again")
	second = stage(t, root, deps, second, mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID})
	approve(t, root, deps, second)

	after := load(t, root).Features[featureID].AcceptedAgainst
	require.Equal(t, pinned.Spec.Value, after.Spec.Value)
	require.Equal(t, pinned.Eval.Value, after.Eval.Value)
}

// TestApproveAmendmentRevisesATaskSpec covers the other grain a revise reaches:
// a task's own task.md pin, re-recorded the way approve task --refresh-basis
// records it.
func TestApproveAmendmentRevisesATaskSpec(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	priorPin := load(t, root).Tasks[taskID].AcceptedAgainst.Spec.Value

	writeTaskSpecAt(t, root, "app-experience", "fast-logging", "ipad-logging",
		"## Approach\n\nBuild the iPhone-only logging surface instead.")
	rel := filepath.Join(store.TaskRelDir("app-experience", "fast-logging", "ipad-logging"), store.TaskProseFile)

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: taskID,
		Preconditions: map[string]string{"spec_revision": blobOf(t, root, rel)}})
	approve(t, root, deps, a)

	tk := load(t, root).Tasks[taskID]
	require.Equal(t, blobOf(t, root, rel), tk.AcceptedAgainst.Spec.Value)
	require.NotEqual(t, priorPin, tk.AcceptedAgainst.Spec.Value)
	require.Equal(t, "check", tk.Status, "revise re-pins a basis and moves no state")
}

func TestApproveAmendmentRefusesRevisingTerminalWorkWithNoWrites(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		noun   string
		status string
	}{
		{name: "done feature", noun: store.NounFeature, status: "done"},
		{name: "cancelled feature", noun: store.NounFeature, status: "cancelled"},
		{name: "done task", noun: store.NounTask, status: "done"},
		{name: "cancelled task", noun: store.NounTask, status: "cancelled"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, deps, _, taskID := lanternFixture(t)
			target := taskID
			if tc.noun == store.NounTask {
				setTaskStatusOnDisk(t, root, taskID, tc.status)
			} else {
				terminal, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
					Key: "app-experience/terminal-revise-target", Title: "Terminal revise target"})
				require.NoError(t, err)
				target = terminal.ID
				setFeatureStatusOnDisk(t, root, "app-experience", "terminal-revise-target", tc.status)
			}

			a := openAmendment(t, root, deps, "terminal-revise")
			a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: target})
			before := snapshotDir(t, root)
			err := tryApprove(t, root, deps, a)

			require.Equal(t, "revise_target_terminal", reasonOf(t, err))
			require.ErrorContains(t, err, "cannot revise")
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestApproveAmendmentRefusesCancellingAndRevisingSameWorkWithNoWrites(t *testing.T) {
	t.Parallel()
	grains := []struct {
		name       string
		task       bool
		wantStatus string
	}{
		{name: "feature", wantStatus: "active"},
		{name: "task", task: true, wantStatus: "check"},
	}
	orders := []struct {
		name        string
		reviseFirst bool
	}{
		{name: "cancel then revise"},
		{name: "revise then cancel", reviseFirst: true},
	}

	for _, grain := range grains {
		for _, order := range orders {
			t.Run(grain.name+"/"+order.name, func(t *testing.T) {
				root, deps, featureID, taskID := lanternFixture(t)
				target := featureID
				if grain.task {
					target = taskID
				}

				a := openAmendment(t, root, deps, "cancel-and-revise")
				cancel := mutate.StageInput{
					Op: store.AmendmentOpCancel, Target: target, Reason: "replaced by forward work"}
				revise := mutate.StageInput{Op: store.AmendmentOpRevise, Target: target}
				if order.reviseFirst {
					a = stage(t, root, deps, a, revise)
					a = stage(t, root, deps, a, cancel)
				} else {
					a = stage(t, root, deps, a, cancel)
					a = stage(t, root, deps, a, revise)
				}

				before := snapshotDir(t, root)
				err := tryApprove(t, root, deps, a)
				require.Equal(t, "revise_target_terminal", reasonOf(t, err))
				require.ErrorContains(t, err, "is cancelled; terminal work is append-only")
				requireNoWrites(t, before, snapshotDir(t, root))

				tree := load(t, root)
				if grain.task {
					require.Equal(t, grain.wantStatus, tree.Tasks[target].Status)
				} else {
					require.Equal(t, grain.wantStatus, tree.Features[target].Status)
				}
			})
		}
	}
}

// TestApproveAmendmentRefusesAReviseWithNoContractOnDisk is the carried defect
// from task 7's drive: a revise whose target has no contract file in
// the working tree used to reach phase 2's re-pin, where recordBasis hashes
// feature.md and feature.eval.md through git and answered `vcs: git could not
// resolve path .../feature.eval.md`, a raw environment error for a fact about
// the tree. Every sibling refusal on this surface names what it is about, and
// the basis system itself already reads "the file is not there" as a fact
// rather than a tool failure (WorktreeBlobRef, checkBasis's own missing arm),
// so the answer belongs in phase 1 with the other operation-level requirements
// no precondition can express.
//
// The three arms are the three shapes that reach it: the bare draft feature the
// defect was found on (`tiller add feature` writes feature.md and no
// feature.eval.md at all), a target whose spec was deleted after the batch was
// staged, and the same fact one grain down on a task's task.md. Each asserts
// zero writes, so the refusal is a phase-1 answer and not a rollback.
func TestApproveAmendmentRefusesAReviseWithNoContractOnDisk(t *testing.T) {
	t.Parallel()
	taskRel := filepath.Join(store.TaskRelDir("app-experience", "fast-logging", "ipad-logging"), store.TaskProseFile)

	cases := []struct {
		name    string
		target  func(root string, featureID, taskID, draftID string) string
		vanish  func(t *testing.T, root string)
		names   string
		grainOf string
	}{
		{
			name:    "a draft feature that never got a contract",
			target:  func(_, _, _, draftID string) string { return draftID },
			vanish:  func(t *testing.T, root string) {},
			names:   "feature.eval.md",
			grainOf: "feature",
		},
		{
			name:   "a spec deleted after the batch was staged",
			target: func(_, featureID, _, _ string) string { return featureID },
			vanish: func(t *testing.T, root string) {
				require.NoError(t, os.Remove(filepath.Join(root, lanternSpecRel)))
			},
			names:   "feature.md",
			grainOf: "feature",
		},
		{
			name:   "a task whose task.md is gone",
			target: func(_, _, taskID, _ string) string { return taskID },
			vanish: func(t *testing.T, root string) {
				require.NoError(t, os.Remove(filepath.Join(root, taskRel)))
			},
			names:   store.TaskProseFile,
			grainOf: "task",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, deps, featureID, taskID := lanternFixture(t)
			draft, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
				Key: "app-experience/session-history", Title: "Session history", ChangeType: "create"})
			require.NoError(t, err)
			_, statErr := os.Stat(filepath.Join(root, ".tiller", "epics", "app-experience",
				"features", "session-history", "feature.eval.md"))
			require.True(t, os.IsNotExist(statErr), "a freshly added feature carries no contract file")

			a := openAmendment(t, root, deps, "iphone-only")
			a = stage(t, root, deps, a, mutate.StageInput{
				Op: store.AmendmentOpRevise, Target: tc.target(root, featureID, taskID, draft.ID)})
			tc.vanish(t, root)

			before := snapshotDir(t, root)
			err = tryApprove(t, root, deps, a)
			require.Error(t, err)
			require.Equal(t, "revise_target_incomplete", reasonOf(t, err))
			require.Contains(t, err.Error(), tc.names, "the refusal names the file the re-pin needs")
			require.NotContains(t, err.Error(), "vcs:", "a fact about the tree is never a raw environment error")
			require.Contains(t, err.Error(), tc.grainOf)
			requireNoWrites(t, before, snapshotDir(t, root))
			require.Equal(t, store.AmendmentStatusProposed, loadAmendment(t, root, a.ID).Status)
		})
	}
}

// TestApproveAmendmentStalePreconditionsOutrankTheReviseFileGuard pins the
// ordering ruled in checkReviseTargets' own doc comment. A human who staged an
// eval_revision guard over the very file that is missing already asked the
// richer question, and the stale answer carries expected, actual, and the "not in
// the working tree" note operation by operation, at the exit-1 lane every surface
// renders as a data block. The file guard is the backstop for a batch that staged
// no such guard, so it must not step in front of one that did.
func TestApproveAmendmentStalePreconditionsOutrankTheReviseFileGuard(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	pinned := blobOf(t, root, lanternEvalRel)
	require.NoError(t, os.Remove(filepath.Join(root, lanternEvalRel)))

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
		Preconditions: map[string]string{"eval_revision": pinned}})

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale, "the staged guard answers, not the file backstop: %v", err)
	require.Len(t, stale.Stale, 1)
	require.Equal(t, "eval_revision", stale.Stale[0].Precondition)
	require.Contains(t, stale.Stale[0].Note, "feature.eval.md is not in the working tree")
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentAppliesAReviseOnAFeatureWhoseContractIsPresent is the
// other side of the arms above: the guard is about the file being absent and
// nothing else, so an ordinary revise over a present spec and contract still
// applies. Without it, a guard that refused every revise would pass the
// refusal arms outright.
func TestApproveAmendmentAppliesAReviseOnAFeatureWhoseContractIsPresent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	reshapeTheSpec(t, root, featureID)

	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID})
	approve(t, root, deps, a)

	require.Equal(t, blobOf(t, root, lanternSpecRel), load(t, root).Features[featureID].AcceptedAgainst.Spec.Value)
}

// --- checkSource: intake's provenance honesty at approve ---

// sourceFixture lays a real feedback artifact under .tiller/feedback/ and an
// amendment whose Source cites its git-blob fingerprint, the shape `tiller
// intake` produces (intake.go's own WorktreeBlobRef-then-AddAmendment
// sequence), built directly through mutate rather than through the CLI verb
// since this package tests mutate alone. relPath is the artifact's
// repo-relative path; the returned amendment is freshly minted, its own
// Source matching the artifact currently on disk.
func sourceFixture(t *testing.T, body string) (root string, deps mutate.Deps, a *store.Amendment, relPath string) {
	t.Helper()
	root = writeOwnedRepo(t)
	deps = gitDeps(root)

	relPath = filepath.Join(".tiller", "feedback", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "feedback"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte(body), 0o644))

	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{
		Key: "review-intake", Title: "Intake: review.md",
		Source: &store.AmendmentSource{Path: relPath, Revision: blobOf(t, root, relPath)},
	})
	require.NoError(t, err)
	return root, deps, loadAmendment(t, root, res.ID), relPath
}

// TestApproveAmendmentAcceptsAFreshIntakeSource proves checkSource is a pure
// pass-through when the artifact intake fingerprinted has not moved: an
// intake-minted amendment with nothing staged approves exactly like any
// other empty batch (TestApproveAmendmentWithNoOperationsJustFlipsTheStatus's
// own case, this time carrying a Source).
func TestApproveAmendmentAcceptsAFreshIntakeSource(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")
	res := approve(t, root, deps, a)
	require.Equal(t, store.AmendmentStatusApproved, res.Status)
}

// TestApproveAmendmentRefusesAChangedIntakeSourceWithNoWrites pins the
// drift-idiom message: an edit to the
// feedback document between intake and approve refuses with both hashes
// named, and nothing is written.
func TestApproveAmendmentRefusesAChangedIntakeSourceWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := sourceFixture(t, "the review body\n")
	recorded := a.Source.Revision

	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("the EDITED review body\n"), 0o644))
	live := blobOf(t, root, relPath)
	require.NotEqual(t, recorded, live, "the fixture's edit must actually change the hash")

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.EqualError(t, err, fmt.Sprintf(
		"the feedback document changed since intake staged this amendment (recorded %s, live %s); re-read it and re-stage, or withdraw",
		recorded, live))
	require.Equal(t, "source_changed", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesAVanishedIntakeSourceWithNoWrites is the
// distinct refusal the acceptance calls for: a deleted
// artifact has no live hash to report, so it reads differently than an
// edited one rather than folding into the same message with a blank value.
func TestApproveAmendmentRefusesAVanishedIntakeSourceWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := sourceFixture(t, "the review body\n")
	require.NoError(t, os.Remove(filepath.Join(root, relPath)))

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	require.Equal(t, "source_vanished", reasonOf(t, err))
	require.NotContains(t, err.Error(), "recorded",
		"a vanished artifact reads as a distinct refusal, not the changed-document message with a blank live hash")
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesSourceBeforePreconditions pins checkSource's own
// place in the order of judgment (ApproveAmendment's doc comment): it runs
// beside, and just ahead of, checkPreconditions, so a batch whose whole
// foundation already moved is refused for THAT rather than for a staged
// guard that happens to have moved too. If it ran after instead, this
// would surface *mutate.AmendmentPreconditionsStaleError (which carries no
// Reason()), and reasonOf below would fail loudly rather than silently
// compare the wrong thing.
func TestApproveAmendmentRefusesSourceBeforePreconditions(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := sourceFixture(t, "the review body\n")

	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	feat, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)

	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: feat.ID, Source: "review.md#L1-2",
		Preconditions: map[string]string{"target_status": "active"}, // stale: the feature is actually draft
	})

	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("edited\n"), 0o644))

	err = tryApprove(t, root, deps, a)
	require.Equal(t, "source_changed", reasonOf(t, err),
		"the source drift is refused ahead of any precondition comparison")
}

// TestApproveAmendmentAddQuestionUpdatedOutcomeNotesADroppedSource is Task
// 5's inherited assignment from Task 3's review (progress.md): a Source
// staged beside a block that genuinely lands (AppliedUpdated, not a no-op)
// used to vanish with no Note at all, the Note firing only on the
// AppliedNoChange arm. foldQuestion's own union arm (question.go) never
// writes Source regardless of outcome — only the fresh-mint arm does — so a
// question's recorded provenance is fixed at creation and a later batch's
// own citation for a fold that only grows what it blocks is not a second
// origin; the fix is naming the drop here too, not landing it.
func TestApproveAmendmentAddQuestionUpdatedOutcomeNotesADroppedSource(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)

	existing, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
		Source:    "docs/feedback/original-review.md#L1-2",
	})
	require.NoError(t, err)

	a := openAmendment(t, root, deps, "auth-provider")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Replacement provider title", Statement: "Which auth provider?",
		Blocks: []string{featureID}, Source: "docs/feedback/later-review.md#L9-10",
	})

	res := approve(t, root, deps, a)
	require.Len(t, res.Applied, 1)
	require.Equal(t, existing.ID, res.Applied[0].NodeID)
	require.Equal(t, mutate.AppliedUpdated, res.Applied[0].Outcome, "the new block is a real union")
	require.NotEmpty(t, res.Applied[0].Note, "the staged source did not land and must not vanish silently")
	require.Contains(t, res.Applied[0].Note, "blocks landed", "distinct wording from the no-op case: blocks genuinely landed here")
	require.Contains(t, res.Applied[0].Note, "source did not")

	tree := load(t, root)
	require.Equal(t, "docs/feedback/original-review.md#L1-2", tree.Questions[existing.ID].Source,
		"a question's recorded source is fixed at creation; a later fold's own citation never overwrites it")
}

// --- checkOpSources: approve re-enforces per-op source citations ---

// TestApproveAmendmentRefusesAHandBlankedOpSourceWithNoWrites is the
// failing-first pin for #54: StageOperation refuses a blank --source onto an
// intake-minted amendment (sourcelessOpErr, amendment.go), but a hand edit
// (writeAmendmentOps here, standing in for a human editing the yml directly)
// reaches ApproveAmendment without ever passing through that gate, so a
// citation dropped by the edit would otherwise materialize uncited. This
// amendment stages two operations, both sourced (an add-epic and, under it,
// an add-feature, so approving the unedited batch mints two different
// prefixes rather than colliding sourceFixture's fixed minter on one); the
// hand edit blanks only the second one's source, and the refusal must name
// THAT operation's index, not the first.
func TestApproveAmendmentRefusesAHandBlankedOpSourceWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")

	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "billing/invoicing", Title: "Invoicing",
		ChangeType: "create", Source: "review.md#L3-4",
	})

	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[1].Source = ""
	a = writeAmendmentOps(t, root, a, ops)

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "op_source_required", reasonOf(t, err))
	require.Contains(t, err.Error(), "operation 1", "the refusal names the blanked operation, not the first (sourced) one")
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentRefusesAHandWhitespacedOpSourceWithNoWrites folds in
// task 2's review finding beside the blank case above: checkOpSources already
// runs strings.TrimSpace before comparing to "", so a source of "   " refuses
// exactly like an empty one, same reason, same op-indexed message, no code
// gap. This is the missing pin, not a fix.
func TestApproveAmendmentRefusesAHandWhitespacedOpSourceWithNoWrites(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")

	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "billing/invoicing", Title: "Invoicing",
		ChangeType: "create", Source: "review.md#L3-4",
	})

	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[1].Source = "   "
	a = writeAmendmentOps(t, root, a, ops)

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "op_source_required", reasonOf(t, err))
	require.Contains(t, err.Error(), "operation 1", "the refusal names the whitespaced operation, not the first (sourced) one")
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestApproveAmendmentApprovesWhenEveryOpIsSourced is the positive pin beside
// the refusal above: the identical two-operation shape, staged normally with
// neither source blanked, approves.
func TestApproveAmendmentApprovesWhenEveryOpIsSourced(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")

	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "billing/invoicing", Title: "Invoicing",
		ChangeType: "create", Source: "review.md#L3-4",
	})

	res := approve(t, root, deps, a)
	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Len(t, res.Applied, 2)
}

// TestApproveAmendmentApprovesASourcelessOpOnANonIntakeAmendment pins
// checkOpSources' own pass-through: a hand amendment (`tiller add amendment`,
// doc.Source nil) never required a per-op citation before this fix, and the
// new pass must not start requiring one now. TestApproveAmendmentAppliesTheLanternShape
// already exercises this shape incidentally (its add-task op carries no
// Source); this test pins it directly and by name.
func TestApproveAmendmentApprovesASourcelessOpOnANonIntakeAmendment(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)

	a := openAmendment(t, root, deps, "hardening-epic")
	require.Nil(t, a.Source, "a hand-opened amendment carries no Source")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})

	res := approve(t, root, deps, a)
	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Len(t, res.Applied, 1)
}
