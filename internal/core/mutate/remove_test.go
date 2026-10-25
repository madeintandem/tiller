package mutate_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type removeFixture struct {
	root                         string
	deps                         mutate.Deps
	epic1, epic2                 string
	feature1, feature2, feature3 string
	task1, task2                 string
}

func layRemoveFixture(t *testing.T) removeFixture {
	t.Helper()
	root := writeOwnedRepo(t)
	counts := map[string]int{}
	ids := map[string][]string{
		"E": {"E-rem001", "E-rem002"},
		"F": {"F-rem001", "F-rem002", "F-rem003"},
		"T": {"T-rem001", "T-rem002", "T-rem003"},
	}
	d := testDeps()
	d.Mint = func(prefix string) string {
		if seq, ok := ids[prefix]; ok {
			i := counts[prefix]
			counts[prefix]++
			return seq[i]
		}
		return fixedMint(prefix)
	}
	e1, err := mutate.AddEpic(load(t, root), d, mutate.EpicInput{Key: "proposal", Title: "Proposal Epic"})
	require.NoError(t, err)
	e2, err := mutate.AddEpic(load(t, root), d, mutate.EpicInput{Key: "survivor", Title: "Survivor Epic"})
	require.NoError(t, err)
	f1, err := mutate.AddFeature(load(t, root), d, mutate.FeatureInput{Key: "proposal/obsolete", Title: "Obsolete Feature", ChangeType: "create"})
	require.NoError(t, err)
	f2, err := mutate.AddFeature(load(t, root), d, mutate.FeatureInput{Key: "proposal/keeper", Title: "Keeper Feature", ChangeType: "create"})
	require.NoError(t, err)
	f3, err := mutate.AddFeature(load(t, root), d, mutate.FeatureInput{Key: "survivor/client", Title: "Client Feature", ChangeType: "create"})
	require.NoError(t, err)
	t1, err := mutate.AddTask(load(t, root), d, mutate.TaskInput{Key: "proposal/obsolete/remove-me", Title: "Obsolete Task"})
	require.NoError(t, err)
	t2, err := mutate.AddTask(load(t, root), d, mutate.TaskInput{Key: "proposal/obsolete/keep-me", Title: "Keeper Task"})
	require.NoError(t, err)
	return removeFixture{root: root, deps: d, epic1: e1.ID, epic2: e2.ID, feature1: f1.ID, feature2: f2.ID, feature3: f3.ID, task1: t1.ID, task2: t2.ID}
}

func removalEvents(t *testing.T, root string) []events.Event {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var out []events.Event
	for _, ev := range evs {
		if strings.HasPrefix(ev.Type, "remove.") {
			out = append(out, ev)
		}
	}
	return out
}

func setOpenQuestions(t *testing.T, root string, questions ...*store.Question) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	writeYAMLTest(t, path, store.QuestionsFile{Questions: questions})
}

func assertRemovedFromReadSurfaces(t *testing.T, root, id, key, title string) {
	t.Helper()
	tree := load(t, root)
	_, epic := tree.Epics[id]
	_, feature := tree.Features[id]
	_, task := tree.Tasks[id]
	require.False(t, epic || feature || task)
	model, err := yaml.Marshal(gen.Tree(tree))
	require.NoError(t, err)
	require.NotContains(t, string(model), id)
	require.NotContains(t, string(model), key)
	res, err := search.Search(root, tree, title, search.Options{})
	require.NoError(t, err)
	require.Zero(t, res.Count)
}

func TestRemoveDraftProposedTaskCleansReferencesDeletesFilesAndPreservesEvent(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	featurePath := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
	var feature store.Feature
	readYAMLTest(t, featurePath, &feature)
	feature.Order = []string{f.task1, f.task2}
	writeYAMLTest(t, featurePath, feature)
	siblingPath := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "keep-me", "task.yml")
	var sibling store.Task
	readYAMLTest(t, siblingPath, &sibling)
	sibling.DependsOn = []string{f.task1}
	writeYAMLTest(t, siblingPath, sibling)
	setOpenQuestions(t, f.root, &store.Question{ID: "Q-rem001", Title: "Task question", Blocks: []string{f.task1, f.task2}, Status: "open"})
	taskDir := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me")
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "agent-notes.txt"), []byte("delete me\n"), 0o644))

	res, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "duplicate proposal"})
	require.NoError(t, err)
	require.Equal(t, store.NounTask, res.Noun)
	require.Equal(t, f.task1, res.ID)
	require.Equal(t, "proposal/obsolete/remove-me", res.Key)
	require.Equal(t, "duplicate proposal", res.Reason)
	require.Equal(t, []string{f.task1}, res.RemovedIDs)
	require.NotEmpty(t, res.ChangedFiles)
	require.NoDirExists(t, taskDir)

	after := load(t, f.root)
	require.Equal(t, []string{f.task2}, after.Features[f.feature1].Order)
	require.Empty(t, after.Tasks[f.task2].DependsOn)
	require.Equal(t, []string{f.task2}, after.Questions["Q-rem001"].Blocks)
	assertRemovedFromReadSurfaces(t, f.root, f.task1, "proposal/obsolete/remove-me", "Obsolete Task")

	evs := removalEvents(t, f.root)
	require.Len(t, evs, 1)
	require.Equal(t, "remove.proposed.task", evs[0].Type)
	require.Equal(t, []string{f.task1}, evs[0].Targets)
	require.Equal(t, "duplicate proposal", evs[0].After["reason"])
	var eventPaths []string
	for _, file := range evs[0].Files {
		eventPaths = append(eventPaths, filepath.ToSlash(file.Path))
	}
	sort.Strings(eventPaths)
	require.Contains(t, eventPaths, ".tiller/epics/proposal/features/obsolete/tasks/remove-me/agent-notes.txt")
	require.FileExists(t, filepath.Join(f.root, ".tiller", "events", "2026", "06", "27", evs[0].ID+".yml"))
}

func TestRemoveDraftReturnedReadyTaskLeavesReturnAndRemovalEventsDistinct(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	epicPath := filepath.Join(f.root, ".tiller", "epics", "proposal", "epic.yml")
	featurePath := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
	taskPath := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
	var epic store.Epic
	readYAMLTest(t, epicPath, &epic)
	epic.Status = "ready"
	writeYAMLTest(t, epicPath, epic)
	var feature store.Feature
	readYAMLTest(t, featurePath, &feature)
	feature.Status = "ready"
	writeYAMLTest(t, featurePath, feature)
	var task store.Task
	readYAMLTest(t, taskPath, &task)
	task.Status = "ready"
	writeYAMLTest(t, taskPath, task)

	tree := load(t, f.root)
	_, err := mutate.Transition(tree, f.deps, tree.Tasks[f.task1], mutate.TransitionInput{Verb: "return", Reason: "revise the proposed task set"})
	require.NoError(t, err)
	res, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "no longer needed"})
	require.NoError(t, err)
	require.Equal(t, []string{f.task1}, res.RemovedIDs)
	require.Equal(t, "ready", load(t, f.root).Features[f.feature1].Status)

	evs, err := events.List(load(t, f.root))
	require.NoError(t, err)
	var returnCount, removeCount int
	for _, ev := range evs {
		if ev.Type == "return.task" {
			returnCount++
		}
		if ev.Type == "remove.proposed.task" {
			removeCount++
		}
	}
	require.Equal(t, 1, returnCount)
	require.Equal(t, 1, removeCount)
}

func TestRemoveDraftTaskFreeFeatureCleansParentInboundDependenciesAndQuestions(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	// Remove fixture tasks so the feature under test is task-free.
	for _, task := range []string{"remove-me", "keep-me"} {
		_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
		require.NoError(t, err)
	}
	epicPath := filepath.Join(f.root, ".tiller", "epics", "proposal", "epic.yml")
	var epic store.Epic
	readYAMLTest(t, epicPath, &epic)
	epic.Order = []string{f.feature1, f.feature2}
	writeYAMLTest(t, epicPath, epic)
	clientPath := filepath.Join(f.root, ".tiller", "epics", "survivor", "features", "client", "feature.yml")
	var client store.Feature
	readYAMLTest(t, clientPath, &client)
	client.DependsOn = []string{f.feature1}
	writeYAMLTest(t, clientPath, client)
	survivorEpicPath := filepath.Join(f.root, ".tiller", "epics", "survivor", "epic.yml")
	var survivor store.Epic
	readYAMLTest(t, survivorEpicPath, &survivor)
	survivor.DependsOn = []string{f.feature1}
	writeYAMLTest(t, survivorEpicPath, survivor)
	setOpenQuestions(t, f.root, &store.Question{ID: "Q-rem002", Blocks: []string{f.feature1, f.feature2}, Status: "open"})
	featureDir := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete")

	res, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounFeature, Node: f.feature1, Reason: "decomposition simplified"})
	require.NoError(t, err)
	require.Equal(t, []string{f.feature1}, res.RemovedIDs)
	require.NoDirExists(t, featureDir)
	after := load(t, f.root)
	require.Equal(t, []string{f.feature2}, after.Epics[f.epic1].Order)
	require.Empty(t, after.Epics[f.epic2].DependsOn)
	require.Empty(t, after.Features[f.feature3].DependsOn)
	require.Equal(t, []string{f.feature2}, after.Questions["Q-rem002"].Blocks)
	assertRemovedFromReadSurfaces(t, f.root, f.feature1, "proposal/obsolete", "Obsolete Feature")
	foundFeatureRemoval := false
	for _, ev := range removalEvents(t, f.root) {
		if ev.Type == "remove.draft.feature" {
			foundFeatureRemoval = true
		}
	}
	require.True(t, foundFeatureRemoval)
}

func TestRemoveDraftEpicRemovesDraftTaskFreeSubtreeAndAllInboundReferences(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	for _, task := range []string{"remove-me", "keep-me"} {
		_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
		require.NoError(t, err)
	}
	orderPath := filepath.Join(f.root, store.OrderRelPath)
	writeYAMLTest(t, orderPath, store.ProjectOrder{Epics: []string{f.epic1, f.epic2}})
	survivorEpicPath := filepath.Join(f.root, ".tiller", "epics", "survivor", "epic.yml")
	var survivor store.Epic
	readYAMLTest(t, survivorEpicPath, &survivor)
	survivor.DependsOn = []string{f.epic1, f.feature1}
	writeYAMLTest(t, survivorEpicPath, survivor)
	clientPath := filepath.Join(f.root, ".tiller", "epics", "survivor", "features", "client", "feature.yml")
	var client store.Feature
	readYAMLTest(t, clientPath, &client)
	client.DependsOn = []string{f.feature2}
	writeYAMLTest(t, clientPath, client)
	setOpenQuestions(t, f.root, &store.Question{ID: "Q-rem003", Blocks: []string{f.epic1, f.feature1, f.feature3}, Status: "open"})

	res, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounEpic, Node: f.epic1, Reason: "proposal replaced"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{f.epic1, f.feature1, f.feature2}, res.RemovedIDs)
	require.NoDirExists(t, filepath.Join(f.root, ".tiller", "epics", "proposal"))
	after := load(t, f.root)
	require.Equal(t, []string{f.epic2}, after.ProjectOrder.Epics)
	require.Empty(t, after.Epics[f.epic2].DependsOn)
	require.Empty(t, after.Features[f.feature3].DependsOn)
	require.Equal(t, []string{f.feature3}, after.Questions["Q-rem003"].Blocks)
	assertRemovedFromReadSurfaces(t, f.root, f.epic1, "proposal", "Proposal Epic")
	assertRemovedFromReadSurfaces(t, f.root, f.feature1, "proposal/obsolete", "Obsolete Feature")
	assertRemovedFromReadSurfaces(t, f.root, f.feature2, "proposal/keeper", "Keeper Feature")
	evs := removalEvents(t, f.root)
	foundEpicRemoval := false
	for _, ev := range evs {
		if ev.Type == "remove.draft.epic" {
			foundEpicRemoval = true
			require.ElementsMatch(t, res.RemovedIDs, ev.Targets)
		}
	}
	require.True(t, foundEpicRemoval)
}

func TestRemoveDraftRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		prepare    func(*testing.T, removeFixture) (mutate.RemoveInput, string)
		wantReason string
	}{
		{name: "blank reason", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			return mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: " \t "}, filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
		}, wantReason: "remove_reason_required"},
		{name: "task not proposed", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			p := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
			var doc store.Task
			readYAMLTest(t, p, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, p, doc)
			return mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "wrong state"}, p
		}, wantReason: "remove_state_locked"},
		{name: "feature not draft", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			p := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
			var doc store.Feature
			readYAMLTest(t, p, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, p, doc)
			return mutate.RemoveInput{Noun: store.NounFeature, Node: f.feature1, Reason: "wrong state"}, p
		}, wantReason: "remove_state_locked"},
		{name: "feature has tasks", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			return mutate.RemoveInput{Noun: store.NounFeature, Node: f.feature1, Reason: "still occupied"}, filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
		}, wantReason: "remove_state_locked"},
		{name: "epic not draft", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			p := filepath.Join(f.root, ".tiller", "epics", "proposal", "epic.yml")
			var doc store.Epic
			readYAMLTest(t, p, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, p, doc)
			return mutate.RemoveInput{Noun: store.NounEpic, Node: f.epic1, Reason: "wrong state"}, p
		}, wantReason: "remove_state_locked"},
		{name: "epic has tasks", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			return mutate.RemoveInput{Noun: store.NounEpic, Node: f.epic1, Reason: "still occupied"}, filepath.Join(f.root, ".tiller", "epics", "proposal", "epic.yml")
		}, wantReason: "remove_state_locked"},
		{name: "epic child not draft", prepare: func(t *testing.T, f removeFixture) (mutate.RemoveInput, string) {
			for _, task := range []string{"remove-me", "keep-me"} {
				_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
				require.NoError(t, err)
			}
			p := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "keeper", "feature.yml")
			var doc store.Feature
			readYAMLTest(t, p, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, p, doc)
			return mutate.RemoveInput{Noun: store.NounEpic, Node: f.epic1, Reason: "unsafe"}, p
		}, wantReason: "remove_state_locked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := layRemoveFixture(t)
			in, watched := tt.prepare(t, f)
			before, err := os.ReadFile(watched)
			require.NoError(t, err)
			beforeEvents, err := events.List(load(t, f.root))
			require.NoError(t, err)
			_, err = mutate.RemoveDraft(load(t, f.root), f.deps, in)
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			require.Equal(t, tt.wantReason, reasoned.Reason())
			after, readErr := os.ReadFile(watched)
			require.NoError(t, readErr)
			require.Equal(t, before, after)
			afterEvents, listErr := events.List(load(t, f.root))
			require.NoError(t, listErr)
			require.Len(t, afterEvents, len(beforeEvents))
		})
	}
}

func TestRemoveDraftRefusesEveryProposedAmendmentReferenceToRemovedID(t *testing.T) {
	t.Parallel()
	fields := []struct {
		name string
		op   store.AmendmentOp
	}{
		{"target", store.AmendmentOp{Op: store.AmendmentOpRevise, Target: "T-rem001"}},
		{"by", store.AmendmentOp{Op: store.AmendmentOpSupersede, By: "T-rem001"}},
		{"superseded_by", store.AmendmentOp{Op: store.AmendmentOpCancel, SupersededBy: "T-rem001"}},
		{"blocks", store.AmendmentOp{Op: store.AmendmentOpAddQuestion, Blocks: []string{"T-rem001"}}},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			f := layRemoveFixture(t)
			dir := filepath.Join(f.root, ".tiller", "amendments")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			key := "remove-ref"
			path := filepath.Join(dir, "A-rem001-remove-ref.yml")
			writeYAMLTest(t, path, store.Amendment{ID: "A-rem001", Key: &key, Status: store.AmendmentStatusProposed, Operations: []store.AmendmentOp{field.op}})
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "referenced"})
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			require.Equal(t, "proposed_amendment_reference", reasoned.Reason())
			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, before, after)
			require.Contains(t, load(t, f.root).Tasks, f.task1)
		})
	}
}

func TestRemoveDraftRefusesSymlinkAndNonRegularSubtreeEntriesWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			f := layRemoveFixture(t)
			dir := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me")
			path := filepath.Join(dir, kind)
			if kind == "symlink" {
				require.NoError(t, os.Symlink("task.md", path))
			} else {
				require.NoError(t, syscall.Mkfifo(path, 0o600))
			}
			beforeEvents, err := events.List(load(t, f.root))
			require.NoError(t, err)
			_, err = mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "unsafe entry"})
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			require.Equal(t, "non_regular_remove_entry", reasoned.Reason())
			require.FileExists(t, filepath.Join(dir, "task.yml"))
			afterEvents, listErr := events.List(load(t, f.root))
			require.NoError(t, listErr)
			require.Len(t, afterEvents, len(beforeEvents))
		})
	}
}

func TestRemoveDraftStaleStateInventoryAndReferenceChangesWriteNothing(t *testing.T) {
	t.Parallel()
	t.Run("state", func(t *testing.T) {
		f := layRemoveFixture(t)
		stale := load(t, f.root)
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
		var doc store.Task
		readYAMLTest(t, path, &doc)
		doc.Title = "changed concurrently"
		writeYAMLTest(t, path, doc)
		_, err := mutate.RemoveDraft(stale, f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "stale"})
		var envErr *store.EnvError
		require.True(t, errors.As(err, &envErr))
		require.Equal(t, store.ReasonMutationStale, envErr.Reason)
		require.FileExists(t, path)
	})
	t.Run("inventory", func(t *testing.T) {
		f := layRemoveFixture(t)
		stale := load(t, f.root)
		_, err := mutate.AddTask(load(t, f.root), f.deps, mutate.TaskInput{Key: "proposal/keeper/concurrent", Title: "Concurrent"})
		require.NoError(t, err)
		before := len(removalEvents(t, f.root))
		_, err = mutate.RemoveDraft(stale, f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "stale"})
		var envErr *store.EnvError
		require.True(t, errors.As(err, &envErr))
		require.Equal(t, store.ReasonMutationStale, envErr.Reason)
		require.Equal(t, before, len(removalEvents(t, f.root)))
		require.Contains(t, load(t, f.root).Tasks, f.task1)
	})
	t.Run("reference", func(t *testing.T) {
		f := layRemoveFixture(t)
		stale := load(t, f.root)
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "keep-me", "task.yml")
		var doc store.Task
		readYAMLTest(t, path, &doc)
		doc.DependsOn = []string{f.task1}
		writeYAMLTest(t, path, doc)
		_, err := mutate.RemoveDraft(stale, f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "stale"})
		var envErr *store.EnvError
		require.True(t, errors.As(err, &envErr))
		require.Equal(t, store.ReasonMutationStale, envErr.Reason)
		require.Contains(t, load(t, f.root).Tasks, f.task1)
	})
}

func TestRemoveDraftGuardsConcurrentChildlessEpicInventory(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	stale := load(t, f.root)
	taskPath := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
	beforeEvents := len(removalEvents(t, f.root))
	lateDir := filepath.Join(f.root, ".tiller", "epics", "late-arrival")
	latePath := filepath.Join(lateDir, "epic.yml")
	lateKey := "late-arrival"
	late := store.Epic{
		ID: "E-late01", Key: &lateKey, Title: "Late arrival", Status: "draft",
		DependsOn: []string{f.task1},
	}
	d := f.deps
	d.Clock = &removeInterleavingClock{
		at: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
		change: func() {
			require.NoError(t, os.Mkdir(lateDir, 0o755))
			writeYAMLTest(t, latePath, late)
		},
	}

	_, err := mutate.RemoveDraft(stale, d, mutate.RemoveInput{
		Noun: store.NounTask, Node: f.task1, Reason: "interleaved epic addition",
	})

	var envErr *store.EnvError
	require.True(t, errors.As(err, &envErr))
	require.Equal(t, store.ReasonMutationStale, envErr.Reason)
	require.FileExists(t, taskPath)
	require.Equal(t, beforeEvents, len(removalEvents(t, f.root)))
	var preserved store.Epic
	readYAMLTest(t, latePath, &preserved)
	require.Equal(t, []string{f.task1}, preserved.DependsOn)
}

type removeInterleavingClock struct {
	at     time.Time
	change func()
}

func (c *removeInterleavingClock) Now() time.Time {
	c.change()
	return c.at
}

func assertRemovalInterleavingStale(t *testing.T, f removeFixture, noun, node, path string, change func()) {
	t.Helper()
	stale := load(t, f.root)
	beforeEvents := len(removalEvents(t, f.root))
	d := f.deps
	d.Clock = &removeInterleavingClock{at: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC), change: change}
	_, err := mutate.RemoveDraft(stale, d, mutate.RemoveInput{Noun: noun, Node: node, Reason: "interleaved correction"})
	var envErr *store.EnvError
	require.True(t, errors.As(err, &envErr))
	require.Equal(t, store.ReasonMutationStale, envErr.Reason)
	require.FileExists(t, path)
	require.Equal(t, beforeEvents, len(removalEvents(t, f.root)))
}

func TestRemoveDraftCarriesEligibilityAndLineageSnapshotsThroughCommit(t *testing.T) {
	t.Parallel()
	t.Run("proposed task becomes ready", func(t *testing.T) {
		f := layRemoveFixture(t)
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
		assertRemovalInterleavingStale(t, f, store.NounTask, f.task1, path, func() {
			var doc store.Task
			readYAMLTest(t, path, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, path, doc)
		})
	})
	t.Run("draft feature becomes ready", func(t *testing.T) {
		f := layRemoveFixture(t)
		for _, task := range []string{"remove-me", "keep-me"} {
			_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
			require.NoError(t, err)
		}
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
		assertRemovalInterleavingStale(t, f, store.NounFeature, f.feature1, path, func() {
			var doc store.Feature
			readYAMLTest(t, path, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, path, doc)
		})
	})
	t.Run("draft epic becomes ready", func(t *testing.T) {
		f := layRemoveFixture(t)
		for _, task := range []string{"remove-me", "keep-me"} {
			_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
			require.NoError(t, err)
		}
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "epic.yml")
		assertRemovalInterleavingStale(t, f, store.NounEpic, f.epic1, path, func() {
			var doc store.Epic
			readYAMLTest(t, path, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, path, doc)
		})
	})
	t.Run("task gains supersession lineage", func(t *testing.T) {
		f := layRemoveFixture(t)
		path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
		assertRemovalInterleavingStale(t, f, store.NounTask, f.task1, path, func() {
			var doc store.Task
			readYAMLTest(t, path, &doc)
			doc.Supersedes = []string{f.task2}
			writeYAMLTest(t, path, doc)
			predecessorPath := filepath.Join(filepath.Dir(filepath.Dir(path)), "keep-me", "task.yml")
			var predecessor store.Task
			readYAMLTest(t, predecessorPath, &predecessor)
			predecessor.SupersededBy = &f.task1
			writeYAMLTest(t, predecessorPath, predecessor)
		})
	})
}

func TestRemoveDraftKeylessFeatureByIDRefusesWithoutPanicOrWrite(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	for _, task := range []string{"remove-me", "keep-me"} {
		_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
		require.NoError(t, err)
	}
	path := filepath.Join(f.root, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
	var doc store.Feature
	readYAMLTest(t, path, &doc)
	doc.Key = nil
	writeYAMLTest(t, path, doc)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	beforeEvents := len(removalEvents(t, f.root))
	var gotErr error
	require.NotPanics(t, func() {
		_, gotErr = mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounFeature, Node: f.feature1, Reason: "keyless"})
	})
	require.ErrorContains(t, gotErr, "feature "+f.feature1+" carries no key, so its folder cannot be named")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, beforeEvents, len(removalEvents(t, f.root)))
}

func TestRemoveDraftRefusesCanonicalKeyReferencesInProposedAmendments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		noun     string
		node     func(removeFixture) string
		key      string
		op       func(string) store.AmendmentOp
		taskFree bool
	}{
		{name: "task target", noun: store.NounTask, node: func(f removeFixture) string { return f.task1 }, key: "proposal/obsolete/remove-me", op: func(key string) store.AmendmentOp { return store.AmendmentOp{Op: store.AmendmentOpRevise, Target: key} }},
		{name: "feature target", noun: store.NounFeature, node: func(f removeFixture) string { return f.feature1 }, key: "proposal/obsolete", taskFree: true, op: func(key string) store.AmendmentOp { return store.AmendmentOp{Op: store.AmendmentOpRevise, Target: key} }},
		{name: "epic target", noun: store.NounEpic, node: func(f removeFixture) string { return f.epic1 }, key: "proposal", taskFree: true, op: func(key string) store.AmendmentOp { return store.AmendmentOp{Op: store.AmendmentOpRevise, Target: key} }},
		{name: "task by", noun: store.NounTask, node: func(f removeFixture) string { return f.task1 }, key: "proposal/obsolete/remove-me", op: func(key string) store.AmendmentOp {
			return store.AmendmentOp{Op: store.AmendmentOpSupersede, Target: "T-other1", By: key}
		}},
		{name: "feature superseded by", noun: store.NounFeature, node: func(f removeFixture) string { return f.feature1 }, key: "proposal/obsolete", taskFree: true, op: func(key string) store.AmendmentOp {
			return store.AmendmentOp{Op: store.AmendmentOpCancel, Target: "F-other1", Reason: "replaced", SupersededBy: key}
		}},
		{name: "task blocks", noun: store.NounTask, node: func(f removeFixture) string { return f.task1 }, key: "proposal/obsolete/remove-me", op: func(key string) store.AmendmentOp {
			return store.AmendmentOp{Op: store.AmendmentOpAddQuestion, Title: "Question", Statement: "Question?", Blocks: []string{key}}
		}},
		{name: "feature blocks", noun: store.NounFeature, node: func(f removeFixture) string { return f.feature1 }, key: "proposal/obsolete", taskFree: true, op: func(key string) store.AmendmentOp {
			return store.AmendmentOp{Op: store.AmendmentOpAddQuestion, Title: "Question", Statement: "Question?", Blocks: []string{key}}
		}},
		{name: "epic blocks", noun: store.NounEpic, node: func(f removeFixture) string { return f.epic1 }, key: "proposal", taskFree: true, op: func(key string) store.AmendmentOp {
			return store.AmendmentOp{Op: store.AmendmentOpAddQuestion, Title: "Question", Statement: "Question?", Blocks: []string{key}}
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := layRemoveFixture(t)
			if tt.taskFree {
				for _, task := range []string{"remove-me", "keep-me"} {
					_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: "proposal/obsolete/" + task, Reason: "fixture cleanup"})
					require.NoError(t, err)
				}
			}
			dir := filepath.Join(f.root, ".tiller", "amendments")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			key := "key-reference"
			path := filepath.Join(dir, "A-key001-key-reference.yml")
			writeYAMLTest(t, path, store.Amendment{ID: "A-key001", Key: &key, Status: store.AmendmentStatusProposed, Operations: []store.AmendmentOp{tt.op(tt.key)}})
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: tt.noun, Node: tt.node(f), Reason: "still referenced"})
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			require.Equal(t, "proposed_amendment_reference", reasoned.Reason())
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestRemoveDraftPreservesStagedAddKeyReferencesThatDoNotResolveToRemovedNodes(t *testing.T) {
	t.Parallel()
	f := layRemoveFixture(t)
	dir := filepath.Join(f.root, ".tiller", "amendments")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	key := "replacement"
	path := filepath.Join(dir, "A-key002-replacement.yml")
	replacement := "proposal/obsolete/replacement"
	writeYAMLTest(t, path, store.Amendment{ID: "A-key002", Key: &key, Status: store.AmendmentStatusProposed, Operations: []store.AmendmentOp{
		{Op: store.AmendmentOpAddTask, Key: replacement, Title: "Replacement"},
		{Op: store.AmendmentOpAddQuestion, Title: "Replacement question", Statement: "Does replacement work?", Blocks: []string{replacement}},
	}})
	_, err := mutate.RemoveDraft(load(t, f.root), f.deps, mutate.RemoveInput{Noun: store.NounTask, Node: f.task1, Reason: "replace proposal"})
	require.NoError(t, err)
	require.FileExists(t, path)
}
