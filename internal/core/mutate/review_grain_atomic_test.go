package mutate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEpicSubmitReadSetStalesWhenCheckedTaskChangesBeforeCommit(t *testing.T) {
	t.Parallel()
	tree, epic, feature, task := writeReviewGrainAtomicTree(t)
	_, _, guards, err := freshEpicSubmitFeatures(tree, "pay")
	require.NoError(t, err)

	task.Status = "in_progress"
	writeReviewGrainAtomicYAML(t, reviewGrainAtomicTaskPath(tree.Root), task)
	epicPath := filepath.Join(tree.Root, ".tiller", "epics", "pay", "epic.yml")
	_, epicSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, epicPath)
	require.NoError(t, err)
	epic.PR = intPtrAtomic(77)
	epicData, err := yaml.Marshal(epic)
	require.NoError(t, err)

	err = tree.Commit("submit.epic", append(guards, store.PlanWrite(epicPath, epicSnapshot, epicData)))
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.Nil(t, readReviewGrainAtomicEpic(t, tree.Root).PR)
	_ = feature
}

func TestEpicSubmitReadSetStalesWhenTaskInventoryChangesBeforeCommit(t *testing.T) {
	t.Parallel()
	tree, epic, _, _ := writeReviewGrainAtomicTree(t)
	_, _, guards, err := freshEpicSubmitFeatures(tree, "pay")
	require.NoError(t, err)

	lateKey := "pay/checkout/late"
	late := &store.Task{ID: "T-late", Key: &lateKey, Status: "in_progress", Owner: "jc", Started: true}
	latePath := filepath.Join(tree.Root, ".tiller", "epics", "pay", "features", "checkout",
		store.TaskDirName, "late", store.TaskStateFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(latePath), 0o755))
	writeReviewGrainAtomicYAML(t, latePath, late)
	epicPath := filepath.Join(tree.Root, ".tiller", "epics", "pay", "epic.yml")
	_, epicSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, epicPath)
	require.NoError(t, err)
	epic.PR = intPtrAtomic(77)
	epicData, err := yaml.Marshal(epic)
	require.NoError(t, err)

	err = tree.Commit("submit.epic", append(guards, store.PlanWrite(epicPath, epicSnapshot, epicData)))
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.Nil(t, readReviewGrainAtomicEpic(t, tree.Root).PR)
}

func TestReviewGrainChangeReadSetStalesWhenDescendantStartsBeforeCommit(t *testing.T) {
	t.Parallel()
	tree, epic, _, task := writeReviewGrainAtomicTree(t)
	task.Status = "ready"
	task.Started = false
	writeReviewGrainAtomicYAML(t, reviewGrainAtomicTaskPath(tree.Root), task)
	guards, err := guardReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounEpic, Epic: epic, Desired: store.ReviewGrainFeature,
	})
	require.NoError(t, err)

	task.Status = "in_progress"
	task.Started = true
	writeReviewGrainAtomicYAML(t, reviewGrainAtomicTaskPath(tree.Root), task)
	epicPath := filepath.Join(tree.Root, ".tiller", "epics", "pay", "epic.yml")
	_, epicSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, epicPath)
	require.NoError(t, err)
	epic.ReviewGrain = store.ReviewGrainFeature
	epicData, err := yaml.Marshal(epic)
	require.NoError(t, err)

	err = tree.Commit("configure epic", append(guards, store.PlanWrite(epicPath, epicSnapshot, epicData)))
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.Equal(t, store.ReviewGrainEpic, readReviewGrainAtomicEpic(t, tree.Root).ReviewGrain)
}

func TestReviewGrainChangeReadSetStalesWhenAffectedNodeChangesBeforeCommit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, string, *store.Epic, *store.Feature)
	}{
		{
			name: "epic submit",
			mutate: func(t *testing.T, root string, epic *store.Epic, _ *store.Feature) {
				epic.PR = intPtrAtomic(41)
				writeReviewGrainAtomicYAML(t, filepath.Join(root, ".tiller", "epics", "pay", "epic.yml"), epic)
			},
		},
		{
			name: "feature submit",
			mutate: func(t *testing.T, root string, _ *store.Epic, feature *store.Feature) {
				feature.PR = intPtrAtomic(42)
				writeReviewGrainAtomicYAML(t, reviewGrainAtomicFeaturePath(root, "checkout"), feature)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, epic, feature, task := writeReviewGrainAtomicTree(t)
			tree.Manifest.Execution.ReviewGrain = store.ReviewGrainTask
			epic.ReviewGrain = ""
			writeReviewGrainAtomicYAML(t, filepath.Join(tree.Root, ".tiller", "epics", "pay", "epic.yml"), epic)
			task.Status = "ready"
			task.Started = false
			writeReviewGrainAtomicYAML(t, reviewGrainAtomicTaskPath(tree.Root), task)
			guards, err := guardReviewGrainChange(tree, reviewGrainChange{
				Noun: "project", Desired: store.ReviewGrainFeature,
			})
			require.NoError(t, err)

			tc.mutate(t, tree.Root, epic, feature)
			manifestPath := filepath.Join(tree.Root, ".tiller", "tiller.yml")
			err = tree.Commit("configure project", append(guards,
				store.PlanWrite(manifestPath, store.AbsentSnapshot(), []byte("schema_version: 6\n"))))

			var env *store.EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, store.ReasonMutationStale, env.Reason)
			require.NoFileExists(t, manifestPath)
		})
	}
}

func reviewGrainAtomicFeaturePath(root, key string) string {
	return filepath.Join(root, ".tiller", "epics", "pay", "features", key, "feature.yml")
}

func writeReviewGrainAtomicTree(t *testing.T) (*store.Tree, *store.Epic, *store.Feature, *store.Task) {
	t.Helper()
	root := t.TempDir()
	epicKey := "pay"
	featureKey := "pay/checkout"
	taskKey := "pay/checkout/charge"
	epic := &store.Epic{ID: "E-atomic", Key: &epicKey, Status: "ready", ReviewGrain: store.ReviewGrainEpic}
	feature := &store.Feature{ID: "F-atomic", Key: &featureKey, EpicKey: epicKey, Status: "active"}
	task := &store.Task{ID: "T-atomic", Key: &taskKey, FeatureID: feature.ID, Status: "in_review", Owner: "jc", Started: true}

	require.NoError(t, os.MkdirAll(filepath.Dir(reviewGrainAtomicTaskPath(root)), 0o755))
	writeReviewGrainAtomicYAML(t, filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml"), epic)
	writeReviewGrainAtomicYAML(t, filepath.Join(root, ".tiller", "epics", epicKey, "features", "checkout", "feature.yml"), feature)
	writeReviewGrainAtomicYAML(t, reviewGrainAtomicTaskPath(root), task)

	tree := &store.Tree{
		Root:         root,
		Manifest:     &store.Manifest{Execution: store.ExecutionBlock{ReviewGrain: store.ReviewGrainTask}},
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{feature.ID: feature},
		Tasks:        map[string]*store.Task{task.ID: task},
		FeatureTasks: map[string][]*store.Task{feature.ID: {task}},
	}
	return tree, epic, feature, task
}

func reviewGrainAtomicTaskPath(root string) string {
	return filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", store.TaskDirName, "charge", store.TaskStateFile)
}

func writeReviewGrainAtomicYAML(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func readReviewGrainAtomicEpic(t *testing.T, root string) *store.Epic {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", "pay", "epic.yml"))
	require.NoError(t, err)
	var epic store.Epic
	require.NoError(t, yaml.Unmarshal(data, &epic))
	return &epic
}

func intPtrAtomic(value int) *int { return &value }
