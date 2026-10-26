package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- fixtures: submit_node.go's own, distinct names to avoid colliding with
// the rest of the package's fixture zoo (rg for Review Grain) ---

// rgAddFeature writes a feature specification and a green acceptance binding.
func rgAddFeature(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey string) (featureID string) {
	t.Helper()
	res, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: epicKey + "/" + featureKey, Title: "Feature " + featureKey, ChangeType: "create"})
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# "+featureKey+"\n\nReal spec prose for "+featureKey+".\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(greenEvalMD(res.ID)), 0o644))
	return res.ID
}

// rgAddTask writes and explicitly claims one task for the acting developer.
func rgAddTask(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey string) {
	t.Helper()
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: epicKey + "/" + featureKey + "/" + taskKey, Title: "Task " + taskKey})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), deps, epicKey+"/"+featureKey+"/"+taskKey, "")
	require.NoError(t, err)
	writeTaskSpecAt(t, root, epicKey, featureKey, taskKey, "## Approach\n\nImplementation for "+taskKey+".")
}

// rgLayFeature approves an epic and feature plan, leaving claimed tasks ready.
func rgLayFeature(t *testing.T, deps mutate.Deps, epicKey, featureKey string, taskKeys ...string) (root string) {
	t.Helper()
	root = writeGatedRepo(t, "true", "true")
	gitRunM(t, root, "branch", "-M", "main")
	setIntegrationBranch(t, root, "main")
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: epicKey, Title: "Epic " + epicKey})
	require.NoError(t, err)
	rgAddFeature(t, root, deps, epicKey, featureKey)
	for _, tk := range taskKeys {
		rgAddTask(t, root, deps, epicKey, featureKey, tk)
	}
	driveWith(t, root, deps, store.NounEpic, epicKey, mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, epicKey+"/"+featureKey, mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, epicKey+"/"+featureKey, mutate.TransitionInput{Verb: "approve"})
	return root
}

// rgDriveTaskToInReview executes a task on its own branch, commits its submission
// state, and assembles it into the aggregate with an ordinary merge.
func rgDriveTaskToInReview(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey string, refSeed int) {
	t.Helper()
	canon := epicKey + "/" + featureKey + "/" + taskKey
	driveWith(t, root, deps, store.NounTask, canon, mutate.TransitionInput{Verb: "approve"})
	started := driveWith(t, root, deps, store.NounTask, canon, mutate.TransitionInput{Verb: "start"})
	if started.AggregateBranch != "" {
		exists, err := vcs.New(root).RefExists(vcs.BranchRef(started.AggregateBranch))
		require.NoError(t, err)
		if !exists {
			gitRunM(t, root, "branch", started.AggregateBranch)
		}
	}
	gitRunM(t, root, "checkout", "-b", started.Branch)
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, canon)
	res, err := mutate.Check(tree, deps, stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed, "the fixture's build/tests are bound to true and must pass")
	gitCommitAllM(t, root, fmt.Sprintf("task %d checked", refSeed))
	driveWith(t, root, deps, store.NounTask, canon, mutate.TransitionInput{Verb: "submit"})
	gitCommitAllM(t, root, fmt.Sprintf("task %d submitted", refSeed))
	if started.AggregateBranch != "" {
		gitRunM(t, root, "checkout", started.AggregateBranch)
		gitRunM(t, root, "merge", "--no-ff", "-m", "assemble task", started.Branch)
	}
}

// rgDriveTaskToInReviewWithPR stages the pre-hardening task-grain history a
// coarse submit must refuse after a legacy tree's grain is hand-changed.
func rgDriveTaskToInReviewWithPR(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey string, pr int) {
	t.Helper()
	rgDriveTaskToInReview(t, root, deps, epicKey, featureKey, taskKey, pr)
	canon := epicKey + "/" + featureKey + "/" + taskKey
	task := resolveNode(t, load(t, root), store.NounTask, canon).(*store.Task)
	task.PR = &pr
	task.ReviewRef = nil
	data, err := yaml.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey, "tasks", taskKey, "task.yml"), data, 0o644))
}

// rgCheckFeatureGreen runs `check feature` and requires every required gate
// to have passed, the state submit's own gate refusal reads.
func rgCheckFeatureGreen(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey string) {
	t.Helper()
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, epicKey+"/"+featureKey)
	res, err := mutate.Check(tree, deps, stubRev(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed, "the fixture's gates are bound to true and must pass")
}

// --- TestSubmitFeatureRecordsThePROnTheFeature ---

// TestSubmitFeatureRecordsThePROnTheFeature is the happy path: a feature
// resolved to grain feature, both its tasks in_review with a green feature
// check recorded, submits its own PR without moving its status.
func TestSubmitFeatureRecordsThePROnTheFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)

	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "bravo", 102)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")

	tree := load(t, root)
	res, err := mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.NotNil(t, res.PR)
	require.Equal(t, 7, *res.PR)
	require.Equal(t, store.NounFeature, res.Noun)
	require.Equal(t, "pay/checkout", res.Key)

	reloaded := featureByKey(load(t, root), "pay/checkout")
	require.NotNil(t, reloaded.PR)
	require.Equal(t, 7, *reloaded.PR)
	require.Equal(t, "active", reloaded.Status, "submit is record-only: the feature's own status never moves")

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	ev := eventOfType(evs, "submit.feature")
	require.NotNil(t, ev, "expected a submit.feature event")
	require.Equal(t, 7, ev.After["pr"])
}

// TestSubmitFeatureRefusals covers submitFeature's refusal order: the wrong
// resolved grain, a task not yet in_review or done, required gates not
// recorded green, and the idempotent-versus-changed-payload pair on an
// otherwise-successful submit.
func TestSubmitFeatureRefusals(t *testing.T) {
	t.Parallel()
	t.Run("resolved grain is task", func(t *testing.T) {
		deps := seqDeps()
		root := writeOwnedRepo(t)
		_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "pay", Title: "Pay"})
		require.NoError(t, err)
		rgAddFeature(t, root, deps, "pay", "checkout")

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolved review grain is task")
	})

	t.Run("a task still in_progress refuses naming it", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
		require.NoError(t, err)
		driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
		driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})

		tree := load(t, root)
		taskID := onlyTaskID(t, tree)
		_, err = mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), taskID)
		require.Contains(t, err.Error(), "in_progress")
	})

	t.Run("required gates unrecorded refuses naming tiller check feature", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
		require.NoError(t, err)
		rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
		// Deliberately never runs `check feature`: no gates block recorded.

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "tiller check feature")
	})

	t.Run("a blocked --gate recheck stales the excluded gate and refuses", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
		require.NoError(t, err)
		rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
		rgCheckFeatureGreen(t, root, deps, "pay", "checkout")

		// The stub has no code identity, so tests' pass can never be shown
		// current: a `--gate build` recheck marks it stale.
		tree := load(t, root)
		feat := resolveNode(t, tree, store.NounFeature, "pay/checkout")
		res, err := mutate.Check(tree, deps, stubRev(root), feat, mutate.CheckInput{Gates: []string{"build"}})
		require.NoError(t, err)
		require.False(t, res.Passed)
		require.Equal(t, store.GateStatusStale, featureByKey(load(t, root), "pay/checkout").Gates["tests"].Status)

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "tiller check feature")
		require.Nil(t, featureByKey(load(t, root), "pay/checkout").PR)
	})

	t.Run("same PR is a no-op and replacement is refused", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
		require.NoError(t, err)
		rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
		rgCheckFeatureGreen(t, root, deps, "pay", "checkout")

		res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
		require.NoError(t, err)
		require.True(t, res.Changed)

		evsDir := filepath.Join(root, ".tiller", "events")
		beforeCount := countFiles(t, evsDir)
		res2, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
		require.NoError(t, err)
		require.False(t, res2.Changed, "the same PR is a no-op")
		require.Equal(t, beforeCount, countFiles(t, evsDir), "a no-op submit records no event")

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(9)})
		var blocked *mutate.BranchSafetyError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, "branch_review_identity", blocked.Reason)
		require.Equal(t, beforeCount, countFiles(t, evsDir))
		require.Equal(t, 7, *featureByKey(load(t, root), "pay/checkout").PR)
	})
}

// --- TestSubmitEpicRecordsOnTheEpic ---

// rgLayEpicWithTwoFeatures approves two feature plans under an epic-grain epic.
// The caller starts each task and performs the aggregate checks.
func rgLayEpicWithTwoFeatures(t *testing.T, deps mutate.Deps, epicKey string) (root string) {
	t.Helper()
	root = writeGatedRepo(t, "true", "true")
	setIntegrationBranch(t, root, "main")
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: epicKey, Title: "Epic " + epicKey})
	require.NoError(t, err)
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: epicKey, ReviewGrain: "epic"})
	require.NoError(t, err)

	for _, fk := range []string{"alpha", "beta"} {
		rgAddFeature(t, root, deps, epicKey, fk)
		rgAddTask(t, root, deps, epicKey, fk, "only-task")
	}
	driveWith(t, root, deps, store.NounEpic, epicKey, mutate.TransitionInput{Verb: "approve"})
	for _, fk := range []string{"alpha", "beta"} {
		driveWith(t, root, deps, store.NounFeature, epicKey+"/"+fk, mutate.TransitionInput{Verb: "spec"})
		driveWith(t, root, deps, store.NounFeature, epicKey+"/"+fk, mutate.TransitionInput{Verb: "approve"})
	}
	return root
}

// TestSubmitEpicRecordsOnTheEpic pins the epic-grain arc: two features under
// an epic-grained epic, each with its task in_review and its own required
// gates recorded pass, submits the epic's own PR. The refusal subtest mirrors
// the feature's one level up: any live feature with a task not yet in_review
// or done refuses naming that feature.
func TestSubmitEpicRecordsOnTheEpic(t *testing.T) {
	t.Parallel()
	t.Run("both features ready records the epic PR", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayEpicWithTwoFeatures(t, deps, "pay")
		rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
		rgDriveTaskToInReview(t, root, deps, "pay", "beta", "only-task", 202)
		rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
		rgCheckFeatureGreen(t, root, deps, "pay", "beta")

		res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
		require.NoError(t, err)
		require.True(t, res.Changed)
		require.NotNil(t, res.PR)
		require.Equal(t, 50, *res.PR)

		epic := epicByKey(load(t, root), "pay")
		require.NotNil(t, epic.PR)
		require.Equal(t, 50, *epic.PR)
		require.Equal(t, "ready", epic.Status, "submit is record-only: the epic's own status never moves")

		evs, err := events.List(load(t, root))
		require.NoError(t, err)
		ev := eventOfType(evs, "submit.epic")
		require.NotNil(t, ev, "expected a submit.epic event")
		require.Equal(t, 50, ev.After["pr"])
	})

	t.Run("a live feature with a task not yet in_review refuses naming it", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayEpicWithTwoFeatures(t, deps, "pay")
		rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
		rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
		// beta's task never leaves ready: nothing driven for it at all.

		_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pay/beta")
	})
}

// Starting tasks in separate features initializes one shared epic aggregate.
func TestStartTasksUnderEpicGrainShareTheEpicAggregate(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "pay", Title: "Pay"})
	require.NoError(t, err)
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
	require.NoError(t, err)

	for _, fk := range []string{"alpha", "beta"} {
		rgAddFeature(t, root, deps, "pay", fk)
		rgAddTask(t, root, deps, "pay", fk, "only-task")
	}
	driveWith(t, root, deps, store.NounEpic, "pay", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, "pay/alpha", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, "pay/alpha", mutate.TransitionInput{Verb: "approve"})

	wantBranch := "tiller/reviews/epics/pay"
	resAlpha := driveWith(t, root, deps, store.NounTask, "pay/alpha/only-task", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, wantBranch, resAlpha.AggregateBranch)
	require.Equal(t, wantBranch, featureByKey(load(t, root), "pay/alpha").Branch)
	require.Equal(t, wantBranch, epicByKey(load(t, root), "pay").Branch,
		"the first feature to start under an epic-grained epic stamps the epic's own branch")

	driveWith(t, root, deps, store.NounFeature, "pay/beta", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, "pay/beta", mutate.TransitionInput{Verb: "approve"})
	resBeta := driveWith(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, wantBranch, resBeta.AggregateBranch)
	require.Equal(t, wantBranch, featureByKey(load(t, root), "pay/beta").Branch)
	require.Equal(t, wantBranch, epicByKey(load(t, root), "pay").Branch,
		"a later start is idempotent: the epic's recorded branch does not change")

	t.Run("under feature grain the branch stays the feature's own", func(t *testing.T) {
		deps := seqDeps()
		root := writeOwnedRepo(t)
		_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "ship", Title: "Ship"})
		require.NoError(t, err)
		rgAddFeature(t, root, deps, "ship", "checkout")
		_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "ship/checkout", ReviewGrain: "feature"})
		require.NoError(t, err)
		rgAddTask(t, root, deps, "ship", "checkout", "only-task")
		driveWith(t, root, deps, store.NounEpic, "ship", mutate.TransitionInput{Verb: "approve"})
		driveWith(t, root, deps, store.NounFeature, "ship/checkout", mutate.TransitionInput{Verb: "spec"})
		driveWith(t, root, deps, store.NounFeature, "ship/checkout", mutate.TransitionInput{Verb: "approve"})
		res := driveWith(t, root, deps, store.NounTask, "ship/checkout/only-task", mutate.TransitionInput{Verb: "start"})

		require.Equal(t, "tiller/reviews/features/ship/checkout", res.AggregateBranch)
		require.Empty(t, epicByKey(load(t, root), "ship").Branch, "an epic outside epic grain never records a branch")
	})

	t.Run("under task grain the feature has no working branch", func(t *testing.T) {
		root := layActiveFeature(t)
		require.Empty(t, featureByKey(load(t, root), "form-engine/certification-form-loading").Branch)
		require.Empty(t, epicByKey(load(t, root), "form-engine").Branch, "an epic outside epic grain never records a branch")
	})
}

func intPtr(n int) *int { return &n }

// rgSetFeatureStatus hand-lays a feature status onto feature.yml, the
// fix-round-1 twin of setEpicStatus (transition_test.go) for a feature: a
// terminal submit refusal has to bite on whatever status a tree already
// holds on disk, whether or not a real rollup put it there.
func rgSetFeatureStatus(t *testing.T, root, epicKey, featureKey, from, to string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqual(t, string(data), out, "expected a status: "+from+" line to rewrite")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

func rgSetFeatureReviewGrain(t *testing.T, root, epicKey, featureKey, grain string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	needle := "status: "
	at := strings.Index(string(data), needle)
	require.NotEqual(t, -1, at)
	lineEnd := strings.Index(string(data)[at:], "\n")
	require.NotEqual(t, -1, lineEnd)
	insertAt := at + lineEnd + 1
	out := string(data[:insertAt]) + "review_grain: " + grain + "\n" + string(data[insertAt:])
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

func rgSetEpicReviewGrain(t *testing.T, root, epicKey, grain string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "\nreview_grain:")
	require.NoError(t, os.WriteFile(path, append(data, []byte("review_grain: "+grain+"\n")...), 0o644))
}

func rgSetFeaturePR(t *testing.T, root, epicKey, featureKey string, pr int) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "\npr:")
	require.NoError(t, os.WriteFile(path, append(data, []byte(fmt.Sprintf("pr: %d\n", pr))...), 0o644))
}

func rgSubmitRefusalWithoutWrites(t *testing.T, root string, deps mutate.Deps, noun, key string) error {
	t.Helper()
	before := snapshotDir(t, root)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))
	_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: noun, Key: key, PR: intPtr(99),
	})
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")),
		"a refused coarse submit records no event")
	return err
}

// --- Fix round 1 ---

// TestSubmitFeatureRefusesOnATerminalFeature is IMPORTANT 1: a feature that
// closed, done by rollup or cancelled by a human, has no review left to
// record, mirroring Configure's own terminal guard (configureFeature's
// refuseReviewSurfaceLocked, configure.go). Before this fix, a done or
// cancelled feature whose gates still recorded pass from before it closed
// sailed through every other check and silently rewrote pr/review_ref.
func TestSubmitFeatureRefusesOnATerminalFeature(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
				Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
			require.NoError(t, err)
			rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
			rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
			rgSetFeatureStatus(t, root, "pay", "checkout", "active", status)

			evsDir := filepath.Join(root, ".tiller", "events")
			before := countFiles(t, evsDir)
			_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
				Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(1)})
			require.Error(t, err)
			require.Contains(t, err.Error(), status)

			reloaded := featureByKey(load(t, root), "pay/checkout")
			require.Nil(t, reloaded.PR, "a refused submit records nothing")
			require.Equal(t, status, reloaded.Status, "a refused submit must not move the feature")
			require.Equal(t, before, countFiles(t, evsDir), "a refused submit records no event")
		})
	}
}

// TestSubmitEpicRefusesOnATerminalEpic is IMPORTANT 1's epic-grain twin:
// done, cancelled, or archived, an epic has no review left to record.
// done is included here even though Configure's own terminal set omits it
// (fix round 2): Configure's omission is harmless only because its guard
// has an independent PR-recorded arm that catches a done epic anyway (a
// done epic almost always already carries a PR), but submit's terminal
// guard is the ONLY thing standing between a closed node and a rewrite.
// With done excluded, an epic-grain epic submitted once, then hand-rolled
// to done, could be submitted again with a different PR: every other arm
// waves it through (grain still resolves epic, done is not draft,
// epicLiveFeatures excludes only cancelled so a done feature still counts
// as live, that feature's tasks are all done, its gates still record pass
// from before, and the no-op check only stops an IDENTICAL resubmit). A
// real next round only becomes submittable after sync's reopenEpic moves
// status back to ready, so refusing done here costs nothing legitimate.
func TestSubmitEpicRefusesOnATerminalEpic(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"done", "cancelled", "archived"} {
		t.Run(status, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayEpicWithTwoFeatures(t, deps, "pay")
			rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
			rgDriveTaskToInReview(t, root, deps, "pay", "beta", "only-task", 202)
			rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
			rgCheckFeatureGreen(t, root, deps, "pay", "beta")
			setEpicStatus(t, root, "pay", "ready", status)

			evsDir := filepath.Join(root, ".tiller", "events")
			before := countFiles(t, evsDir)
			_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
				Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
			require.Error(t, err)
			require.Contains(t, err.Error(), status)

			reloaded := epicByKey(load(t, root), "pay")
			require.Nil(t, reloaded.PR, "a refused submit records nothing")
			require.Equal(t, status, reloaded.Status, "a refused submit must not move the epic")
			require.Equal(t, before, countFiles(t, evsDir), "a refused submit records no event")
		})
	}
}

// TestSubmitEpicRefusals is MINOR 3: the epic-grain refusal arms
// TestSubmitFeatureRefusals already covers one level down, structurally
// mirrored here (wrong grain, not started, a live feature's gates not
// green, and the no-op/changed idempotence pair).
func TestSubmitEpicRefusals(t *testing.T) {
	t.Parallel()
	t.Run("resolved grain is task", func(t *testing.T) {
		deps := seqDeps()
		root := writeOwnedRepo(t)
		_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "pay", Title: "Pay"})
		require.NoError(t, err)

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolved review grain is task")
	})

	t.Run("a draft epic has nothing to review yet", func(t *testing.T) {
		deps := seqDeps()
		root := writeOwnedRepo(t)
		_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "pay", Title: "Pay"})
		require.NoError(t, err)
		_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
		require.NoError(t, err)

		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(1)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "draft")
	})

	t.Run("a live feature with gates not green refuses naming it", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayEpicWithTwoFeatures(t, deps, "pay")
		rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
		rgDriveTaskToInReview(t, root, deps, "pay", "beta", "only-task", 202)
		rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
		// beta's task reaches in_review, but `check feature` never runs for
		// it, so its own required gates carry no recorded result.

		_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pay/beta")
		require.Contains(t, err.Error(), "gate")
	})

	t.Run("same PR is a no-op and replacement is refused", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayEpicWithTwoFeatures(t, deps, "pay")
		rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
		rgDriveTaskToInReview(t, root, deps, "pay", "beta", "only-task", 202)
		rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
		rgCheckFeatureGreen(t, root, deps, "pay", "beta")

		res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
		require.NoError(t, err)
		require.True(t, res.Changed)

		evsDir := filepath.Join(root, ".tiller", "events")
		beforeCount := countFiles(t, evsDir)
		res2, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
		require.NoError(t, err)
		require.False(t, res2.Changed, "the same PR is a no-op")
		require.Equal(t, beforeCount, countFiles(t, evsDir), "a no-op submit records no event")

		before := snapshotDir(t, root)
		_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
			Noun: store.NounEpic, Key: "pay", PR: intPtr(60)})
		var blocked *mutate.BranchSafetyError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, "branch_review_identity", blocked.Reason)
		requireNoWrites(t, before, snapshotDir(t, root))
		require.Equal(t, beforeCount, countFiles(t, evsDir))
		require.Equal(t, 50, *epicByKey(load(t, root), "pay").PR)
	})
}

// TestSubmitFeatureRefusesWhenResolvedGrainIsEpic is MINOR 4: a feature
// under an epic-grained epic (no override of its own) resolves to grain
// epic by inheritance, so `submit feature` on it directly refuses the same
// way a plain task-grain feature does, naming epic rather than task as the
// resolved grain the caller must submit through instead.
func TestSubmitFeatureRefusesWhenResolvedGrainIsEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")

	_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: "pay/alpha", PR: intPtr(1)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved review grain is epic; submit the epic's own PR")
}

// --- Final-review fix round ---

// TestSubmitEpicNamesNoLiveTaskRatherThanTasksIncompleteWording is item 6: a
// live feature under an epic-grain submit with zero tasks at all (draft,
// never spec'd) used to get FeatureNotSubmittableError's generic
// "has a task not yet in_review or done" wording, which is simply false when
// there is no task to be short of in_review. featureTasksSubmittable's own
// no-tasks case (blockerID=="") now carries through to the error the same
// way TaskNotSubmittableError already mirrors it one grain down
// (TestSubmitFeatureRefusals never drives a zero-task feature through
// `submit feature` directly, but the message the CLI already renders for
// that case, "has no live task to review", is the wording this test proves
// FeatureNotSubmittableError.Error() now matches).
func TestSubmitEpicNamesNoLiveTaskRatherThanTasksIncompleteWording(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeGatedRepo(t, "true", "true")
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "pay", Title: "Pay"})
	require.NoError(t, err)
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
	require.NoError(t, err)

	rgAddFeature(t, root, deps, "pay", "alpha")
	rgAddTask(t, root, deps, "pay", "alpha", "only-task")
	// beta is added and left draft, never spec'd, approved, or started: a
	// live (non-cancelled) feature carrying zero tasks, the exact state
	// featureTasksSubmittable answers false on with blockerID=="".
	rgAddFeature(t, root, deps, "pay", "beta")

	driveWith(t, root, deps, store.NounEpic, "pay", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, "pay/alpha", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, "pay/alpha", mutate.TransitionInput{Verb: "approve"})

	rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
	rgCheckFeatureGreen(t, root, deps, "pay", "alpha")

	_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(50)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pay/beta")
	require.Contains(t, err.Error(), "has no live task to review")
	require.NotContains(t, err.Error(), "not yet in_review or done",
		"a feature with zero tasks is not short of in_review; it has nothing to review at all")

	var featBlk *mutate.FeatureNotSubmittableError
	require.ErrorAs(t, err, &featBlk)
	require.Equal(t, "tasks_incomplete", featBlk.Reason)
	require.Empty(t, featBlk.TaskID, "no live task means no blocker task id to name")
}

func TestSubmitFeatureRefusesActiveBasisDriftBeforeReadiness(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature})
	require.NoError(t, err)
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "only-task", 101)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")

	spec := filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.md")
	require.NoError(t, os.WriteFile(spec, []byte("# checkout\n\nMaterially different behavior.\n"), 0o644))

	err = rgSubmitRefusalWithoutWrites(t, root, deps, store.NounFeature, "pay/checkout")
	var basis *mutate.ActiveBasisError
	require.ErrorAs(t, err, &basis)
	require.Equal(t, []string{"feature.md"}, basis.Drifted)
	require.Nil(t, featureByKey(load(t, root), "pay/checkout").PR)
}

func TestSubmitFeatureRefusesParentQuestionAndAssumptionGuards(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "archived"} {
		t.Run("retired parent epic "+status, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
				Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature})
			require.NoError(t, err)
			driveWith(t, root, deps, store.NounTask, "pay/checkout/only-task", mutate.TransitionInput{Verb: "start"})
			setEpicStatus(t, root, "pay", "ready", status)

			err = rgSubmitRefusalWithoutWrites(t, root, deps, store.NounFeature, "pay/checkout")
			var retired *mutate.ParentEpicRetiredError
			require.ErrorAs(t, err, &retired)
			require.Equal(t, status, retired.EpicState)
		})
	}

	t.Run("operative question", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature})
		require.NoError(t, err)
		driveWith(t, root, deps, store.NounTask, "pay/checkout/only-task", mutate.TransitionInput{Verb: "start"})
		featureID := featureByKey(load(t, root), "pay/checkout").ID
		writeOpenQuestion(t, root, "Q-coarse", featureID)

		err = rgSubmitRefusalWithoutWrites(t, root, deps, store.NounFeature, "pay/checkout")
		var questions *mutate.BlockedByQuestionsError
		require.ErrorAs(t, err, &questions)
		require.Equal(t, "Q-coarse", questions.Blockers[0].Question)
	})

	t.Run("answered unreviewed assumption", func(t *testing.T) {
		deps := seqDeps()
		root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
		_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature})
		require.NoError(t, err)
		driveWith(t, root, deps, store.NounTask, "pay/checkout/only-task", mutate.TransitionInput{Verb: "start"})
		added, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
			Title: "Canonical schema", Statement: "Which schema version should this feature use?"})
		require.NoError(t, err)
		driveWith(t, root, deps, store.NounFeature, "pay/checkout", mutate.TransitionInput{Verb: "approve", AssumeQuestion: added.ID, Reason: "Proceed against the documented assumption."})
		_, err = mutate.AnswerQuestion(load(t, root), deps, stubRev(root), mutate.AnswerInput{
			Node: added.ID, Answer: "Use the current schema."})
		require.NoError(t, err)

		err = rgSubmitRefusalWithoutWrites(t, root, deps, store.NounFeature, "pay/checkout")
		var assumption *mutate.AnsweredQuestionError
		require.ErrorAs(t, err, &assumption)
		require.Equal(t, added.ID, assumption.Questions[0].QuestionID)
	})

}

func TestSubmitEpicRefusesRidingFeatureWorkGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		block func(*testing.T, string, mutate.Deps)
		check func(*testing.T, error)
	}{
		{
			name: "active basis",
			block: func(t *testing.T, root string, _ mutate.Deps) {
				spec := filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.md")
				require.NoError(t, os.WriteFile(spec, []byte("# checkout\n\nChanged after approval.\n"), 0o644))
			},
			check: func(t *testing.T, err error) {
				var basis *mutate.ActiveBasisError
				require.ErrorAs(t, err, &basis)
			},
		},
		{
			name: "operative question",
			block: func(t *testing.T, root string, _ mutate.Deps) {
				writeOpenQuestion(t, root, "Q-eeeeee", featureByKey(load(t, root), "pay/checkout").ID)
			},
			check: func(t *testing.T, err error) {
				var questions *mutate.BlockedByQuestionsError
				require.ErrorAs(t, err, &questions)
			},
		},
		{
			name: "answered unreviewed assumption",
			block: func(t *testing.T, root string, deps mutate.Deps) {
				added, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
					Title: "Canonical schema", Statement: "Which schema version should this feature use?"})
				require.NoError(t, err)
				driveWith(t, root, deps, store.NounFeature, "pay/checkout", mutate.TransitionInput{Verb: "approve", AssumeQuestion: added.ID, Reason: "Proceed against the documented assumption."})
				_, err = mutate.AnswerQuestion(load(t, root), deps, stubRev(root), mutate.AnswerInput{
					Node: added.ID, Answer: "Use the current schema."})
				require.NoError(t, err)
			},
			check: func(t *testing.T, err error) {
				var assumption *mutate.AnsweredQuestionError
				require.ErrorAs(t, err, &assumption)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
			rgSetEpicReviewGrain(t, root, "pay", store.ReviewGrainEpic)
			driveWith(t, root, deps, store.NounTask, "pay/checkout/only-task", mutate.TransitionInput{Verb: "start"})
			tc.block(t, root, deps)

			err := rgSubmitRefusalWithoutWrites(t, root, deps, store.NounEpic, "pay")
			tc.check(t, err)
		})
	}
}

func TestSubmitFeatureRefusesTaskWithItsOwnOpenPR(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
	rgDriveTaskToInReviewWithPR(t, root, deps, "pay", "checkout", "only-task", 41)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
	rgSetFeatureReviewGrain(t, root, "pay", "checkout", store.ReviewGrainFeature)

	err := rgSubmitRefusalWithoutWrites(t, root, deps, store.NounFeature, "pay/checkout")
	var open *mutate.TaskReviewOpenError
	require.ErrorAs(t, err, &open)
	require.Equal(t, store.NounFeature, open.ReviewedNoun)
	require.Equal(t, 41, *open.PR)
	require.Nil(t, featureByKey(load(t, root), "pay/checkout").PR)
}

func TestSubmitEpicRefusesTaskWithItsOwnOpenPR(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
	rgDriveTaskToInReviewWithPR(t, root, deps, "pay", "checkout", "only-task", 41)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
	rgSetEpicReviewGrain(t, root, "pay", store.ReviewGrainEpic)

	err := rgSubmitRefusalWithoutWrites(t, root, deps, store.NounEpic, "pay")
	var open *mutate.TaskReviewOpenError
	require.ErrorAs(t, err, &open)
	require.Equal(t, store.NounEpic, open.ReviewedNoun)
	require.Equal(t, 41, *open.PR)
	require.Nil(t, epicByKey(load(t, root), "pay").PR)
}

func TestSubmitEpicWaitsForOverriddenFeatureOwnReviewToReachDone(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainTask, store.ReviewGrainFeature} {
		t.Run(grain, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayEpicWithTwoFeatures(t, deps, "pay")
			rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
			rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
			rgSetFeatureReviewGrain(t, root, "pay", "beta", grain)
			driveWith(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
			if grain == store.ReviewGrainFeature {
				rgSetFeaturePR(t, root, "pay", "beta", 88)
			}

			err := rgSubmitRefusalWithoutWrites(t, root, deps, store.NounEpic, "pay")
			var independent *mutate.FeatureNotSubmittableError
			require.ErrorAs(t, err, &independent)
			require.Equal(t, mutate.ReasonIndependentReviewOpen, independent.Reason)
			require.Equal(t, featureByKey(load(t, root), "pay/beta").ID, independent.FeatureID)

			rgSetFeatureStatus(t, root, "pay", "beta", "active", "done")
			res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
				Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})
			require.NoError(t, err, "a completed override no longer rides or blocks the epic review")
			require.True(t, res.Changed)
		})
	}
}

func TestSubmitEpicReloadsCachedCancelledChildBeforeEligibility(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
	rgCheckFeatureGreen(t, root, deps, "pay", "alpha")

	tree := load(t, root)
	featureByKey(tree, "pay/beta").Status = "cancelled"
	before := snapshotDir(t, root)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	_, err := mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})

	var blocked *mutate.FeatureNotSubmittableError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "tasks_incomplete", blocked.Reason)
	require.Equal(t, featureByKey(load(t, root), "pay/beta").ID, blocked.FeatureID)
	require.Nil(t, epicByKey(load(t, root), "pay").PR)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
}

func TestSubmitFeatureAllowsDoneTaskRetainingHistoricalPR(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "only-task")
	rgDriveTaskToInReviewWithPR(t, root, deps, "pay", "checkout", "only-task", 41)
	setTaskStatusOnDisk(t, root, onlyTaskID(t, load(t, root)), "done")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature})
	require.NoError(t, err)
	rgAddTask(t, root, deps, "pay", "checkout", "next-task")
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "next-task", 42)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")

	res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(99)})

	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, 99, *featureByKey(load(t, root), "pay/checkout").PR)
}

func TestSubmitEpicAllowsDoneTaskRetainingHistoricalPR(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 201)
	rgDriveTaskToInReviewWithPR(t, root, deps, "pay", "beta", "only-task", 41)
	betaTask := resolveNode(t, load(t, root), store.NounTask, "pay/beta/only-task")
	setTaskStatusOnDisk(t, root, betaTask.NodeID(), "done")
	rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
	rgCheckFeatureGreen(t, root, deps, "pay", "beta")

	res, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})

	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, 99, *epicByKey(load(t, root), "pay").PR)
}

func TestSubmitEpicDiscoversFeatureAddedAfterTreeLoad(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	for i, feature := range []string{"alpha", "beta"} {
		rgDriveTaskToInReview(t, root, deps, "pay", feature, "only-task", 301+i)
		rgCheckFeatureGreen(t, root, deps, "pay", feature)
	}
	tree := load(t, root)
	rgLayUncachedEpicFeature(t, root, tree, "alpha", "gamma", "in_progress")

	_, err := mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})

	var blocked *mutate.FeatureNotSubmittableError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "tasks_incomplete", blocked.Reason)
	require.Equal(t, "pay/gamma", blocked.FeatureKey)
	require.Nil(t, rgReadEpicYAML(t, root, "pay").PR)
}

func TestSubmitEpicDiscoversTaskAddedAfterTreeLoad(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	for i, feature := range []string{"alpha", "beta"} {
		rgDriveTaskToInReview(t, root, deps, "pay", feature, "only-task", 311+i)
		rgCheckFeatureGreen(t, root, deps, "pay", feature)
	}
	tree := load(t, root)
	alpha := featureByKey(tree, "pay/alpha")
	rgLayUncachedTask(t, root, alpha, tree.FeatureTasks[alpha.ID][0], "late-task", "in_progress")

	_, err := mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})

	var blocked *mutate.FeatureNotSubmittableError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "tasks_incomplete", blocked.Reason)
	require.Equal(t, alpha.ID, blocked.FeatureID)
	require.Nil(t, rgReadEpicYAML(t, root, "pay").PR)
}

func TestSubmitEpicUsesTaskStatusAdvancedAfterTreeLoad(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayEpicWithTwoFeatures(t, deps, "pay")
	rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 321)
	rgCheckFeatureGreen(t, root, deps, "pay", "alpha")
	rgCheckFeatureGreen(t, root, deps, "pay", "beta")
	tree := load(t, root)
	betaTask := tree.FeatureTasks[featureByKey(tree, "pay/beta").ID][0]
	require.Equal(t, "ready", betaTask.Status)
	rgDriveTaskToInReview(t, root, deps, "pay", "beta", "only-task", 322)

	res, err := mutate.SubmitNode(tree, deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})

	var stale *store.EnvError
	require.ErrorAs(t, err, &stale)
	require.Equal(t, store.ReasonMutationStale, stale.Reason)
	require.Nil(t, epicByKey(load(t, root), "pay").PR)
	res, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounEpic, Key: "pay", PR: intPtr(99)})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, 99, *epicByKey(load(t, root), "pay").PR)
}

func rgLayUncachedEpicFeature(t *testing.T, root string, tree *store.Tree, sourceKey, newKey, taskStatus string) {
	t.Helper()
	source := featureByKey(tree, "pay/"+sourceKey)
	doc := *source
	doc.ID = "F-uncached"
	key := "pay/" + newKey
	doc.Key = &key
	doc.EpicKey = ""
	featureDir := filepath.Join(root, ".tiller", "epics", "pay", "features", newKey)
	require.NoError(t, os.MkdirAll(featureDir, 0o755))
	data, err := yaml.Marshal(&doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "feature.yml"), data, 0o644))
	for _, name := range []string{"feature.md", "feature.eval.md"} {
		sourceData, readErr := os.ReadFile(filepath.Join(root, ".tiller", "epics", "pay", "features", sourceKey, name))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, name), sourceData, 0o644))
	}
	rgLayUncachedTask(t, root, &doc, tree.FeatureTasks[source.ID][0], "only-task", taskStatus)
}

func rgLayUncachedTask(t *testing.T, root string, feature *store.Feature, source *store.Task, taskKey, status string) {
	t.Helper()
	require.NotNil(t, feature.Key)
	featureKey := *feature.Key
	doc := *source
	doc.ID = "T-uncached-" + strings.ReplaceAll(featureKey, "/", "-") + "-" + taskKey
	key := featureKey + "/" + taskKey
	doc.Key = &key
	doc.FeatureID = ""
	doc.Status = status
	parts := strings.Split(featureKey, "/")
	require.Len(t, parts, 2)
	taskDir := filepath.Join(root, ".tiller", "epics", parts[0], "features",
		parts[1], store.TaskDirName, taskKey)
	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	data, err := yaml.Marshal(&doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, store.TaskStateFile), data, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "task.md"), []byte("# Task\n"), 0o644))
}

func rgReadEpicYAML(t *testing.T, root, epicKey string) *store.Epic {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml"))
	require.NoError(t, err)
	var epic store.Epic
	require.NoError(t, yaml.Unmarshal(data, &epic))
	return &epic
}
