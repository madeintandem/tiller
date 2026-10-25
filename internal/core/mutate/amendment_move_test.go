package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func moveFixture(t *testing.T) (string, mutate.Deps, string, string) {
	t.Helper()
	root := writeGatedRepo(t, "true", "true")
	deps := amendDeps(root)
	for _, key := range []string{"source", "destination"} {
		_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: key, Title: key})
		require.NoError(t, err)
	}
	f, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "source/feature", Title: "Feature", ChangeType: "create"})
	require.NoError(t, err)
	task, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: "source/feature/task", Title: "Task"})
	require.NoError(t, err)
	return root, deps, f.ID, task.ID
}

func TestReviewedMovePreservesIdentityAndProse(t *testing.T) {
	t.Parallel()
	root, deps, fid, tid := moveFixture(t)
	a := openAmendment(t, root, deps, "relocate")
	oldDir := filepath.Join(root, ".tiller/epics/source/features/feature")
	before, err := os.ReadFile(filepath.Join(oldDir, "feature.md"))
	require.NoError(t, err)
	_, err = mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Keep one outcome together"})
	require.NoError(t, err)
	require.DirExists(t, oldDir)
	require.NoDirExists(t, filepath.Join(root, ".tiller/epics/destination/features/renamed"))
	a = loadAmendment(t, root, a.ID)
	_, err = mutate.ApproveAmendment(load(t, root), deps, a)
	require.NoError(t, err)
	tree := load(t, root)
	require.Equal(t, "destination/renamed", *tree.Features[fid].Key)
	require.Equal(t, "destination/renamed/task", *tree.Tasks[tid].Key)
	require.Equal(t, "proposed", tree.Tasks[tid].Status)
	require.Equal(t, fid, tree.Tasks[tid].FeatureID)
	require.NoDirExists(t, oldDir)
	after, err := os.ReadFile(filepath.Join(root, ".tiller/epics/destination/features/renamed/feature.md"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, store.AmendmentStatusApproved, tree.Amendments[a.ID].Status)
	_, err = mutate.ApproveAmendment(tree, deps, tree.Amendments[a.ID])
	require.Error(t, err)
}

func TestReviewedMoveStaleClaimRefusesWithoutWrites(t *testing.T) {
	t.Parallel()
	root, deps, fid, tid := moveFixture(t)
	a := openAmendment(t, root, deps, "relocate")
	_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), deps, tid, "")
	require.NoError(t, err)
	before := snapshotDir(t, root)
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestReviewedMoveRollsBackWholeDirectory(t *testing.T) {
	t.Parallel()
	root, deps, fid, _ := moveFixture(t)
	a := openAmendment(t, root, deps, "relocate")
	_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
	require.NoError(t, err)
	eventDir := filepath.Join(root, ".tiller/events/2026/06/27")
	require.NoError(t, os.MkdirAll(eventDir, 0o755))
	before := snapshotDir(t, root)
	require.NoError(t, os.Chmod(eventDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(eventDir, 0o755) })
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.Error(t, err)
	require.NoError(t, os.Chmod(eventDir, 0o755))
	after := snapshotDir(t, root)
	delete(before, filepath.Join(".tiller", ".cache", "generation"))
	delete(after, filepath.Join(".tiller", ".cache", "generation"))
	// Rollback restores file contents and inventory, not modification times.
	for path, value := range before {
		before[path], _, _ = strings.Cut(value, "@")
	}
	for path, value := range after {
		after[path], _, _ = strings.Cut(value, "@")
	}
	require.Equal(t, before, after)
	require.DirExists(t, filepath.Join(root, ".tiller/epics/source/features/feature"))
	require.NoDirExists(t, filepath.Join(root, ".tiller/epics/destination/features/renamed"))
}

func writeMoveState(t *testing.T, root, rel string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), data, 0o644))
}

func TestReviewedMovePreservesReadyClaimsBasisAndEpicGrain(t *testing.T) {
	t.Parallel()
	root, deps, fid, tid := moveFixture(t)
	tree := load(t, root)
	var source *store.Epic
	for _, epic := range tree.Epics {
		if *epic.Key == "source" {
			source = epic
		}
	}
	source.Status = "ready"
	source.ReviewGrain = "epic"
	writeMoveState(t, root, ".tiller/epics/source/epic.yml", source)
	f := tree.Features[fid]
	f.Status = "ready"
	f.AcceptedAgainst.Spec = store.RevisionRef{Kind: "git_blob", Value: "git:abc123", Path: ".tiller/epics/source/features/feature/feature.md"}
	writeMoveState(t, root, ".tiller/epics/source/features/feature/feature.yml", f)
	task := tree.Tasks[tid]
	task.Status = "ready"
	task.AcceptedAgainst = &store.BasisRefs{Spec: store.RevisionRef{Kind: "git_blob", Value: "git:def456", Path: ".tiller/epics/source/features/feature/tasks/task/task.md"}}
	writeMoveState(t, root, ".tiller/epics/source/features/feature/tasks/task/task.yml", task)
	_, err := mutate.ClaimTask(load(t, root), deps, tid, "")
	require.NoError(t, err)
	before := load(t, root)
	a := openAmendment(t, root, deps, "relocate")
	staged, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
	require.NoError(t, err)
	require.True(t, staged.Op.Move.MaterializeReviewGrain)
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.NoError(t, err)
	after := load(t, root)
	require.Equal(t, before.Features[fid].AcceptedAgainst, after.Features[fid].AcceptedAgainst)
	require.Equal(t, before.Tasks[tid].AcceptedAgainst, after.Tasks[tid].AcceptedAgainst)
	require.Equal(t, before.Tasks[tid].Owner, after.Tasks[tid].Owner)
	require.Equal(t, "ready", after.Tasks[tid].Status)
	require.Equal(t, "epic", after.Features[fid].ReviewGrain)
}

func TestReviewedMoveMultipleIntoEarlierStagedEpic(t *testing.T) {
	t.Parallel()
	root, deps, fid, _ := moveFixture(t)
	other, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "source/second", Title: "Second", ChangeType: "create"})
	require.NoError(t, err)
	a := openAmendment(t, root, deps, "relocate")
	_, err = mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "add-epic", Key: "new-destination", Title: "Delivery"})
	require.NoError(t, err)
	for _, row := range []struct{ id, key string }{{fid, "new-destination/one"}, {other.ID, "new-destination/two"}} {
		_, err = mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, a.ID), mutate.StageInput{Op: "move-feature", Target: row.id, Key: row.key, Reason: "Regroup"})
		require.NoError(t, err)
	}
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.NoError(t, err)
	tree := load(t, root)
	for _, epic := range tree.Epics {
		if *epic.Key == "new-destination" {
			require.Equal(t, []string{fid, other.ID}, epic.Order)
			require.Equal(t, "draft", epic.Status)
		}
	}
	require.Equal(t, "new-destination/one", *tree.Features[fid].Key)
	require.Equal(t, "new-destination/two", *tree.Features[other.ID].Key)
}

func TestReviewedMoveRejectsLifecycleHistoryAndConstraints(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"started", "prior-start-event", "feature-active", "parent-done", "parent-review", "parent-dependency", "parent-question", "unsafe-path", "collision"} {
		t.Run(scenario, func(t *testing.T) {
			root, deps, fid, tid := moveFixture(t)
			tree := load(t, root)
			feature := tree.Features[fid]
			task := tree.Tasks[tid]
			var source, destination *store.Epic
			for _, epic := range tree.Epics {
				if *epic.Key == "source" {
					source = epic
				} else {
					destination = epic
				}
			}
			key := "destination/renamed"
			switch scenario {
			case "started":
				_, err := mutate.ClaimTask(tree, deps, tid, "")
				require.NoError(t, err)
				task = load(t, root).Tasks[tid]
				task.Started = true
				task.Status = "in_progress"
				writeMoveState(t, root, ".tiller/epics/source/features/feature/tasks/task/task.yml", task)
			case "prior-start-event":
				eventDir := filepath.Join(root, ".tiller/events/2026/06/27")
				require.NoError(t, os.MkdirAll(eventDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(eventDir, "EV-zzzzzz.yml"), []byte("id: EV-zzzzzz\ntype: start.task\nat: 2026-06-27T14:00:00Z\ntargets: ["+tid+"]\n"), 0o644))
			case "feature-active":
				feature.Status = "active"
				writeMoveState(t, root, ".tiller/epics/source/features/feature/feature.yml", feature)
			case "parent-done":
				source.Status = "done"
				writeMoveState(t, root, ".tiller/epics/source/epic.yml", source)
			case "parent-review":
				pr := 12
				destination.PR = &pr
				writeMoveState(t, root, ".tiller/epics/destination/epic.yml", destination)
			case "parent-dependency":
				source.DependsOn = []string{destination.ID}
				writeMoveState(t, root, ".tiller/epics/source/epic.yml", source)
			case "parent-question":
				writeMoveState(t, root, ".tiller/decisions/open-questions.yml", &store.QuestionsFile{Questions: []*store.Question{{ID: "Q-zzzzzz", Status: "open", Blocks: []string{source.ID}}}})
			case "unsafe-path":
				key = "../renamed"
			case "collision":
				_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{Key: key, Title: "Occupied", ChangeType: "create"})
				require.NoError(t, err)
			}
			a := openAmendment(t, root, deps, "relocate")
			before := snapshotDir(t, root)
			_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: key, Reason: "Regroup"})
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestReviewedMoveConflictingLaterOperationRefuses(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"revise", "cancel", "add-task", "add-question", "move-feature"} {
		t.Run(op, func(t *testing.T) {
			root, deps, fid, tid := moveFixture(t)
			a := openAmendment(t, root, deps, "relocate")
			_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
			require.NoError(t, err)
			in := mutate.StageInput{Op: op, Target: tid, Reason: "Conflicting"}
			switch op {
			case "add-task":
				in.Target = ""
				in.Key = "source/feature/new"
				in.Title = "New"
			case "add-question":
				in.Target = ""
				in.Title = "Barrier"
				in.Statement = "Should it move?"
				for _, e := range load(t, root).Epics {
					if *e.Key == "destination" {
						in.Blocks = []string{e.ID}
					}
				}
			case "move-feature":
				in.Target = fid
				in.Key = "destination/again"
			}
			before := snapshotDir(t, root)
			_, err = mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, a.ID), in)
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestReviewedMoveStaleApprovalBoundaries(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"task-added", "parent-policy", "occupied-destination", "prose-edited"} {
		t.Run(change, func(t *testing.T) {
			root, deps, fid, _ := moveFixture(t)
			a := openAmendment(t, root, deps, "relocate")
			_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
			require.NoError(t, err)
			switch change {
			case "task-added":
				_, err = mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: "source/feature/another", Title: "Another"})
				require.NoError(t, err)
			case "parent-policy":
				for _, epic := range load(t, root).Epics {
					if *epic.Key == "destination" {
						epic.ReviewGrain = "feature"
						writeMoveState(t, root, ".tiller/epics/destination/epic.yml", epic)
					}
				}
			case "occupied-destination":
				_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "destination/renamed", Title: "Occupied", ChangeType: "create"})
				require.NoError(t, err)
			case "prose-edited":
				require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/source/features/feature/feature.md"), []byte("changed acceptance prose"), 0o644))
			}
			before := snapshotDir(t, root)
			_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestReviewedMoveSameParentKeepsSiblingPosition(t *testing.T) {
	t.Parallel()
	root, deps, fid, _ := moveFixture(t)
	other, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "source/second", Title: "Second", ChangeType: "create"})
	require.NoError(t, err)
	for _, epic := range load(t, root).Epics {
		if *epic.Key == "source" {
			epic.Order = []string{other.ID, fid}
			writeMoveState(t, root, ".tiller/epics/source/epic.yml", epic)
		}
	}
	a := openAmendment(t, root, deps, "rename")
	_, err = mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "source/renamed", Reason: "Clearer name"})
	require.NoError(t, err)
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.NoError(t, err)
	for _, epic := range load(t, root).Epics {
		if *epic.Key == "source" {
			require.Equal(t, []string{other.ID, fid}, epic.Order)
		}
	}
}

func TestReviewedMoveRemapsOnlyCurrentLocators(t *testing.T) {
	t.Parallel()
	root, deps, fid, tid := moveFixture(t)
	other, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "destination/dependent", Title: "Dependent", ChangeType: "create"})
	require.NoError(t, err)
	tree := load(t, root)
	dependent := tree.Features[other.ID]
	dependent.DependsOn = []string{"source/feature"}
	dependent.Source = ".tiller/epics/source/features/feature/feature.md#L1"
	writeMoveState(t, root, ".tiller/epics/destination/features/dependent/feature.yml", dependent)
	task := tree.Tasks[tid]
	task.AcceptedAgainst = &store.BasisRefs{Spec: store.RevisionRef{Kind: "git_blob", Value: "git:abc123", Path: ".tiller/epics/source/features/feature/tasks/task/task.md"}}
	writeMoveState(t, root, ".tiller/epics/source/features/feature/tasks/task/task.yml", task)
	writeMoveState(t, root, ".tiller/decisions/open-questions.yml", &store.QuestionsFile{Questions: []*store.Question{{ID: "Q-zzzzzz", Status: "open", Blocks: []string{"source/feature"}}}})
	a := openAmendment(t, root, deps, "relocate")
	staged, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
	require.NoError(t, err)
	require.Contains(t, strings.Join(staged.Op.Move.ReferenceWarnings, "\n"), "source provenance")
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, a.ID))
	require.NoError(t, err)
	after := load(t, root)
	require.Equal(t, []string{fid}, after.Features[other.ID].DependsOn)
	require.Equal(t, dependent.Source, after.Features[other.ID].Source)
	require.Equal(t, task.AcceptedAgainst, after.Tasks[tid].AcceptedAgainst)
	require.Equal(t, []string{"destination/renamed"}, after.Questions["Q-zzzzzz"].Blocks)
}

func TestReviewedMoveWarnsCurrentRelativeLinksNotHistoricalPins(t *testing.T) {
	t.Parallel()
	root, deps, fid, tid := moveFixture(t)
	old := ".tiller/epics/source/features/feature"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/source/epic.md"), []byte("[Feature](features/feature/feature.md)\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, old, "feature.md"), []byte("[Parent](../../epic.md)\n[Task](tasks/task/task.md)\n"), 0o644))
	task := load(t, root).Tasks[tid]
	task.AcceptedAgainst = &store.BasisRefs{Spec: store.RevisionRef{Kind: "git_blob", Value: "git:abc123", Path: old + "/tasks/task/task.md"}}
	writeMoveState(t, root, old+"/tasks/task/task.yml", task)
	a := openAmendment(t, root, deps, "relocate")
	result, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{Op: "move-feature", Target: fid, Key: "destination/renamed", Reason: "Regroup"})
	require.NoError(t, err)
	warnings := strings.Join(result.Op.Move.ReferenceWarnings, "\n")
	require.Contains(t, warnings, "features/feature/feature.md")
	require.Contains(t, warnings, "../../epic.md")
	require.NotContains(t, warnings, "tasks/task/task.md")
	require.NotContains(t, warnings, "accepted_against")
}
