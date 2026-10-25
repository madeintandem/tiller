package mutate_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// mustAddEpic adds an epic under root and fails the test on any error, the
// setup shorthand TestConfigure* uses to lay down a tree to configure against.
func mustAddEpic(t *testing.T, root, key string) {
	t.Helper()
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: key})
	require.NoError(t, err)
}

// mustAddFeature adds a feature under root by its canonical <epic>/<feature>
// key and fails the test on any error.
func mustAddFeature(t *testing.T, root, canonicalKey string) {
	t.Helper()
	_, err := mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: canonicalKey})
	require.NoError(t, err)
}

// featureByKey walks tree.Features for the one whose recorded key equals
// canonicalKey. tree.Features is id-keyed, so a canonical-key lookup has to
// walk it, the same shape mutate's own unexported epicByKey (transition.go)
// takes for epics.
func featureByKey(tree *store.Tree, canonicalKey string) *store.Feature {
	for _, f := range tree.Features {
		if f.Key != nil && *f.Key == canonicalKey {
			return f
		}
	}
	return nil
}

// setFeatureReviewRefOnDisk rewrites a feature.yml to carry a recorded
// review_ref, the cheap way to stage a feature whose own review surface
// already exists without driving the full submit path. Mirrors
// setFeatureStatusOnDisk (transition_test.go).
func setFeatureReviewRefOnDisk(t *testing.T, root, epicKey, featKey string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := string(data) + "review_ref:\n  branch: tiller/" + epicKey + "/" + featKey + "\n  head:\n    kind: git_commit\n    value: \"git:abc\"\n"
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

func TestConfigureFeatureRecordsGrainAndEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/checkout")
	res, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "feature", featureByKey(load(t, root), "pay/checkout").ReviewGrain)

	// idempotent second set: same value, no change, no event (symmetric with
	// the project arm's own no-op event-count assertion below).
	evsDir := filepath.Join(root, ".tiller", "events")
	beforeCount := countFiles(t, evsDir)
	res2, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)
	require.False(t, res2.Changed)
	require.Equal(t, beforeCount, countFiles(t, evsDir), "a no-op set records no event")
}

// TestConfigureEpicRecordsGrainAndEvent is the epic-grain twin of
// TestConfigureFeatureRecordsGrainAndEvent: a successful set persists
// review_grain on disk and records exactly one configure.epic event carrying
// before ""/after the set value, and a same-value re-set is Changed false
// with no new event (fix round 2: the epic arm had no success test at all,
// before or after fix round 1's feature-arm no-op parity).
func TestConfigureEpicRecordsGrainAndEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	res, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
	require.NoError(t, err)
	require.True(t, res.Changed)

	tree := load(t, root)
	var epic *store.Epic
	for _, e := range tree.Epics {
		if e.Key != nil && *e.Key == "pay" {
			epic = e
		}
	}
	require.NotNil(t, epic, "expected the epic pay to load")
	require.Equal(t, "epic", epic.ReviewGrain)

	evs, err := events.List(tree)
	require.NoError(t, err)
	var configureEv *events.Event
	for i := range evs {
		if evs[i].Type == "configure.epic" {
			configureEv = &evs[i]
		}
	}
	require.NotNil(t, configureEv, "expected one configure.epic event")
	require.Equal(t, "", configureEv.Before["review_grain"])
	require.Equal(t, "epic", configureEv.After["review_grain"])

	// idempotent second set: same value, no change, no event.
	evsDir := filepath.Join(root, ".tiller", "events")
	beforeCount := countFiles(t, evsDir)
	res2, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
	require.NoError(t, err)
	require.False(t, res2.Changed)
	require.Equal(t, beforeCount, countFiles(t, evsDir), "a no-op set records no event")
}

func TestConfigureValidatesValuePerNoun(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/checkout")
	_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "sideways"})
	require.Error(t, err)
	_, err = mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "sideways"})
	require.Error(t, err)
}

func TestConfigureProjectWritesTheManifestSurgically(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	before, _ := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	res, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: "project", ReviewGrain: "feature"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	m := load(t, root).Manifest
	require.Equal(t, "feature", m.Execution.ReviewGrain)
	after, _ := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.Contains(t, string(after), "review_grain: feature")
	require.Contains(t, string(after), strings.Split(string(before), "execution:")[0][:40], "the file's head is untouched")

	// idempotent second set: same value, no change, no event.
	evsDir := filepath.Join(root, ".tiller", "events")
	beforeCount := countFiles(t, evsDir)
	res2, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: "project", ReviewGrain: "feature"})
	require.NoError(t, err)
	require.False(t, res2.Changed)
	require.Equal(t, beforeCount, countFiles(t, evsDir), "a no-op set records no event")
}

// manifestWithEmptyExecutionBlock carries an execution: block with no
// review_grain key yet, the shape RenderExecutionReviewGrain's insert-inside
// arm handles: Task 1 shipped that render path untested, and this fixture
// pins it (Task 1 review finding).
const manifestWithEmptyExecutionBlock = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
execution:
  other_setting: kept
`

func writeRepoWithEmptyExecutionBlock(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), []byte(manifestWithEmptyExecutionBlock), 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
	return dir
}

func TestConfigureProjectInsertsIntoAnExistingExecutionBlockWithNoReviewGrainKey(t *testing.T) {
	t.Parallel()
	root := writeRepoWithEmptyExecutionBlock(t)
	res, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: "project", ReviewGrain: "epic"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	after, err := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Contains(t, string(after), "execution:\n  review_grain: epic", "the key lands inside the existing block")
	require.Contains(t, string(after), "other_setting: kept", "the rest of the block survives")
}

func TestConfigureProjectRefusesFlowExecutionWithoutWriting(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	before = append(bytes.TrimRight(before, "\n"), []byte("\nexecution: {review_grain: task}\n")...)
	require.NoError(t, os.WriteFile(path, before, 0o644))

	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))
	_, err = mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: "project", ReviewGrain: "feature",
	})
	require.Error(t, err)

	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, before, after)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
	require.Contains(t, err.Error(), "block mapping")
}

func TestConfigureProjectRefusesUnsupportedExecutionShapesBeforeParseOrNoOp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		line  string
		grain string
	}{
		{name: "scalar before parse", line: "execution: feature", grain: "task"},
		{name: "equal flow mapping before no-op", line: "execution: {review_grain: feature}", grain: "feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeOwnedRepo(t)
			tree := load(t, root)
			path := filepath.Join(root, ".tiller", "tiller.yml")
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			before = append(bytes.TrimRight(before, "\n"), []byte("\n"+tc.line+"\n")...)
			require.NoError(t, os.WriteFile(path, before, 0o644))
			eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

			_, err = mutate.Configure(tree, testDeps(), mutate.ConfigureInput{
				Noun: "project", ReviewGrain: tc.grain,
			})
			require.Error(t, err)

			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, before, after)
			require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
			require.Contains(t, err.Error(), "block mapping")
		})
	}
}

func TestConfigureRefusesOnceTheReviewSurfaceExistsOrTerminal(t *testing.T) {
	t.Parallel()
	t.Run("feature review_ref already recorded", func(t *testing.T) {
		root := writeOwnedRepo(t)
		mustAddEpic(t, root, "pay")
		mustAddFeature(t, root, "pay/checkout")
		// Simulate the recorded surface: set the feature's ReviewRef by hand-editing
		// through the store API the test owns (write the yml with a review_ref), or
		// drive the full arc; the cheap honest route is a direct yml rewrite the way
		// setFeatureStatusOnDisk (transition_test.go) rewrites status.
		setFeatureReviewRefOnDisk(t, root, "pay", "checkout")
		_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "task"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "review surface", "the refusal names why")
	})

	// The terminal-status half of the guard: a feature that reached done or
	// cancelled has nothing left to submit, so its grain is locked exactly as
	// a recorded review_ref locks it, even though no PR or review_ref was
	// ever written (refuseReviewSurfaceLocked's terminal arm, configure.go).
	for _, status := range []string{"done", "cancelled"} {
		t.Run("feature terminal status "+status, func(t *testing.T) {
			root := writeOwnedRepo(t)
			mustAddEpic(t, root, "pay")
			mustAddFeature(t, root, "pay/checkout")
			setFeatureStatusOnDisk(t, root, "pay", "checkout", status)
			_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
				Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "task"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "review surface", "the refusal names why")
			require.Contains(t, err.Error(), status, "the refusal names the terminal status")
		})
	}

	// Same guard, one level up: an epic that reached cancelled or archived.
	for _, status := range []string{"cancelled", "archived"} {
		t.Run("epic terminal status "+status, func(t *testing.T) {
			root := writeOwnedRepo(t)
			mustAddEpic(t, root, "pay")
			setEpicStatusOnDisk(t, root, "pay", "draft", status)
			_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
				Noun: store.NounEpic, Key: "pay", ReviewGrain: "task"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "review surface", "the refusal names why")
			require.Contains(t, err.Error(), status, "the refusal names the terminal status")
		})
	}

	t.Run("terminal and review_ref recorded: the surface message wins", func(t *testing.T) {
		root := writeOwnedRepo(t)
		mustAddEpic(t, root, "pay")
		mustAddFeature(t, root, "pay/checkout")
		setFeatureStatusOnDisk(t, root, "pay", "checkout", "done")
		setFeatureReviewRefOnDisk(t, root, "pay", "checkout")
		_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "task"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "review surface", "the refusal names why")
		require.Contains(t, err.Error(), "review_ref branch", "the recorded surface, not the terminal status, is named")
		require.NotContains(t, err.Error(), "is terminal", "refuseReviewSurfaceLocked checks PR/ReviewRef before terminal, so the surface message wins")
	})
}

func TestConfigureFeatureRefusesAffectedTaskWorkInFlightWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_progress", "check", "in_review"} {
		t.Run(status, func(t *testing.T) {
			deps := seqDeps()
			root := writeOwnedRepo(t)
			mustAddEpic(t, root, "pay")
			mustAddFeature(t, root, "pay/checkout")
			rgAddTask(t, root, deps, "pay", "checkout", "charge-card")
			taskID := onlyTaskID(t, load(t, root))
			setTaskStatusOnDisk(t, root, taskID, status)

			featurePath := filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml")
			beforeFeature, err := os.ReadFile(featurePath)
			require.NoError(t, err)
			eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

			_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
				Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature,
			})

			var blocked *mutate.ReviewGrainChangeBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
			require.Equal(t, taskID, blocked.BlockerID)
			require.Equal(t, status, blocked.BlockerStatus)
			afterFeature, readErr := os.ReadFile(featurePath)
			require.NoError(t, readErr)
			require.Equal(t, beforeFeature, afterFeature)
			require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
		})
	}
}

func TestConfigureEpicRefusesAffectedChildReviewSurfaceWithoutWriting(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/checkout")
	setFeatureReviewRefOnDisk(t, root, "pay", "checkout")

	epicPath := filepath.Join(root, ".tiller", "epics", "pay", "epic.yml")
	beforeEpic, err := os.ReadFile(epicPath)
	require.NoError(t, err)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	_, err = mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: store.ReviewGrainFeature,
	})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, store.NounFeature, blocked.BlockerNoun)
	require.Equal(t, featureByKey(load(t, root), "pay/checkout").ID, blocked.BlockerID)
	afterEpic, readErr := os.ReadFile(epicPath)
	require.NoError(t, readErr)
	require.Equal(t, beforeEpic, afterEpic)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
}

func TestConfigureProjectRefusesOnlyAffectedDescendantsWithoutWriting(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/unaffected")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/unaffected", ReviewGrain: store.ReviewGrainTask,
	})
	require.NoError(t, err)
	rgAddTask(t, root, deps, "pay", "unaffected", "live-task")
	unaffectedTaskID := onlyTaskID(t, load(t, root))
	setTaskStatusOnDisk(t, root, unaffectedTaskID, "in_review")

	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "pay/affected", Title: "Affected", ChangeType: "create",
	})
	require.NoError(t, err)
	setFeatureReviewRefOnDisk(t, root, "pay", "affected")
	affectedID := featureByKey(load(t, root), "pay/affected").ID

	manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
	beforeManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: "project", ReviewGrain: store.ReviewGrainFeature,
	})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, affectedID, blocked.BlockerID, "the explicit task override is unaffected")
	afterManifest, readErr := os.ReadFile(manifestPath)
	require.NoError(t, readErr)
	require.Equal(t, beforeManifest, afterManifest)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
}

func TestConfigureFeatureAllowsTaskToFeatureAfterPriorTasksAreDone(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/checkout")
	rgAddTask(t, root, deps, "pay", "checkout", "charge-card")
	setTaskStatusOnDisk(t, root, onlyTaskID(t, load(t, root)), "done")

	res, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: store.ReviewGrainFeature,
	})

	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, store.ReviewGrainFeature, featureByKey(load(t, root), "pay/checkout").ReviewGrain)
}

func TestConfigureRefusesCrossingEpicBoundaryAfterTaskActivationWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		startEpic bool
		desired   string
	}{
		{name: "into epic", desired: store.ReviewGrainEpic},
		{name: "out of epic", startEpic: true, desired: store.ReviewGrainTask},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := seqDeps()
			root := laySpeccedPlan(t, deps, "alfa")
			grain := store.ReviewGrainFeature
			if tc.startEpic {
				grain = store.ReviewGrainEpic
			}
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
				Noun: store.NounEpic, Key: "form-engine", ReviewGrain: grain,
			})
			require.NoError(t, err)
			driveWith(t, root, deps, store.NounFeature, "form-engine/nrca-form-loading",
				mutate.TransitionInput{Verb: "approve"})
			driveWith(t, root, deps, store.NounTask, "form-engine/nrca-form-loading/alfa",
				mutate.TransitionInput{Verb: "start"})
			setTaskStatusOnDisk(t, root, onlyTaskID(t, load(t, root)), "done")
			require.Equal(t, "active", featureByKey(load(t, root), "form-engine/nrca-form-loading").Status)

			epicPath := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
			beforeEpic, err := os.ReadFile(epicPath)
			require.NoError(t, err)
			eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

			_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
				Noun: store.NounEpic, Key: "form-engine", ReviewGrain: tc.desired,
			})

			var blocked *mutate.ReviewGrainChangeBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, mutate.ReasonReviewGrainBranchLocked, blocked.Reason)
			afterEpic, readErr := os.ReadFile(epicPath)
			require.NoError(t, readErr)
			require.Equal(t, beforeEpic, afterEpic)
			require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
		})
	}
}

func TestConfigureFeatureUsesParentOfFreshDocumentWhenGuardingEpicBoundary(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "form-engine", ReviewGrain: store.ReviewGrainEpic,
	})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounFeature, "form-engine/nrca-form-loading",
		mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounTask, "form-engine/nrca-form-loading/alfa",
		mutate.TransitionInput{Verb: "start"})
	setTaskStatusOnDisk(t, root, onlyTaskID(t, load(t, root)), "done")

	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "form-engine/nrca-form-loading", ReviewGrain: store.ReviewGrainTask,
	})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainBranchLocked, blocked.Reason)
	require.Equal(t, store.ReviewGrainEpic, blocked.BeforeGrain)
	require.Equal(t, store.ReviewGrainTask, blocked.AfterGrain)
}

// countFiles counts the regular files under dir, recursively; a missing dir
// counts as zero rather than an error, since the events ledger does not exist
// until the first event is written.
func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}
