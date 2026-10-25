package mutate_test

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func renameDeps() mutate.Deps {
	d := testDeps()
	d.Mint = func(prefix string) string {
		switch prefix {
		case "E":
			return "E-8wqkyr"
		case "F":
			return "F-ap1234"
		case "T":
			return "T-sc1234"
		default:
			return id.Mint(prefix)
		}
	}
	return d
}

func layRenameTree(t *testing.T) (string, mutate.Deps) {
	t.Helper()
	root := writeOwnedRepo(t)
	d := renameDeps()

	_, err := mutate.AddEpic(load(t, root), d, mutate.EpicInput{
		Key: "foundation", Title: "Phase 0 Foundation", Source: "design/platform.md",
	})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), d, mutate.FeatureInput{
		Key: "foundation/app", Title: "App", ChangeType: "create", Source: "design/app.md",
	})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), d, mutate.TaskInput{
		Key: "foundation/app/scaffold", Title: "Scaffold",
	})
	require.NoError(t, err)

	epicPath := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	var epic store.Epic
	readYAMLTest(t, epicPath, &epic)
	epic.Order = []string{"F-ap1234"}
	writeYAMLTest(t, epicPath, epic)
	return root, d
}

func readYAMLTest(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, out))
}

func writeYAMLTest(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestRenameTitleRetitlesAcmeShapedEpicByIDAndOnlyChangesStateTitle(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)
	ymlPath := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	mdPath := filepath.Join(root, ".tiller", "epics", "foundation", "epic.md")

	var before store.Epic
	readYAMLTest(t, ymlPath, &before)
	mdBefore, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	eventsBefore, err := events.List(load(t, root))
	require.NoError(t, err)

	res, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "E-8wqkyr", Title: "Platform Foundation",
	})
	require.NoError(t, err)
	require.Equal(t, mutate.RenameTitleResult{
		Noun: store.NounEpic, ID: "E-8wqkyr", Key: "foundation",
		OldTitle: "Phase 0 Foundation", Title: "Platform Foundation", Changed: true,
	}, res)

	var after store.Epic
	readYAMLTest(t, ymlPath, &after)
	require.Equal(t, "Platform Foundation", after.Title)
	before.Title, after.Title = "", ""
	require.Equal(t, before, after, "id, key, status, source, order, and every other state field stay identical")
	projected := gen.Tree(load(t, root))
	require.NotEmpty(t, projected.Nodes)
	require.Equal(t, "Platform Foundation", projected.Nodes[0].Title, "tree/read projection carries the new display title")

	mdAfter, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	require.Equal(t, mdBefore, mdAfter, "rename changes state yml, never Markdown prose")

	eventsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsAfter, len(eventsBefore)+1)
	var renameEvent *events.Event
	for i := range eventsAfter {
		if eventsAfter[i].Type == "rename.title.epic" {
			renameEvent = &eventsAfter[i]
		}
	}
	require.NotNil(t, renameEvent)
	require.Equal(t, map[string]any{"title": "Phase 0 Foundation"}, renameEvent.Before)
	require.Equal(t, map[string]any{"title": "Platform Foundation"}, renameEvent.After)
	require.Equal(t, []events.FileChange{{Path: filepath.Join(".tiller", "epics", "foundation", "epic.yml")}}, renameEvent.Files)

}

func TestRenameTitleRetitlesFeatureAndTaskByCanonicalKey(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)

	feature, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounFeature, Node: "foundation/app", Title: "Application Platform",
	})
	require.NoError(t, err)
	require.Equal(t, "F-ap1234", feature.ID)
	require.Equal(t, "foundation/app", feature.Key)
	require.Equal(t, "App", feature.OldTitle)
	require.Equal(t, "Application Platform", feature.Title)
	require.True(t, feature.Changed)

	task, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounTask, Node: "foundation/app/scaffold", Title: "Scaffold the app",
	})
	require.NoError(t, err)
	require.Equal(t, "T-sc1234", task.ID)
	require.Equal(t, "foundation/app/scaffold", task.Key)
	require.Equal(t, "Scaffold", task.OldTitle)
	require.Equal(t, "Scaffold the app", task.Title)
	require.True(t, task.Changed)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	types := make(map[string]bool, len(evs))
	for _, ev := range evs {
		types[ev.Type] = true
	}
	require.True(t, types["rename.title.feature"])
	require.True(t, types["rename.title.task"])
}

func TestRenameTitleAllowsDoneTaskAndArchivedEpic(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)
	epicPath := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	taskPath := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold", "task.yml")

	var epic store.Epic
	readYAMLTest(t, epicPath, &epic)
	epic.Status = "archived"
	writeYAMLTest(t, epicPath, epic)
	var task store.Task
	readYAMLTest(t, taskPath, &task)
	task.Status = "done"
	task.Started = true
	writeYAMLTest(t, taskPath, task)

	staleTree := load(t, root)
	epicRes, err := mutate.RenameTitle(staleTree, d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "foundation", Title: "Archived Foundation",
	})
	require.NoError(t, err)
	require.True(t, epicRes.Changed)

	taskRes, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounTask, Node: "foundation/app/scaffold", Title: "Completed scaffold",
	})
	require.NoError(t, err)
	require.True(t, taskRes.Changed)

	reloaded := load(t, root)
	require.Equal(t, "archived", reloaded.Epics["E-8wqkyr"].Status)
	require.Equal(t, "Archived Foundation", reloaded.Epics["E-8wqkyr"].Title)
	require.Equal(t, "done", reloaded.Tasks["T-sc1234"].Status)
	require.Equal(t, "Completed scaffold", reloaded.Tasks["T-sc1234"].Title)
}

func TestRenameTitleRefusesBlankWithoutWriting(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)
	path := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	eventsBefore, err := events.List(load(t, root))
	require.NoError(t, err)

	_, err = mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "foundation", Title: " \t\n ",
	})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "invalid_title", reason.Reason())

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	eventsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsAfter, len(eventsBefore))
}

func TestRenameTitleSameTitleIsNoOpWithoutEvent(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)
	path := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	eventsBefore, err := events.List(load(t, root))
	require.NoError(t, err)

	res, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "foundation", Title: "Phase 0 Foundation",
	})
	require.NoError(t, err)
	require.False(t, res.Changed)
	require.Equal(t, "Phase 0 Foundation", res.OldTitle)
	require.Equal(t, "Phase 0 Foundation", res.Title)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	eventsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsAfter, len(eventsBefore))
}

func TestRenameTitleRefusesStaleLoadedTreeWithoutOverwritingNewerBytes(t *testing.T) {
	t.Parallel()
	root, d := layRenameTree(t)
	path := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
	staleTree := load(t, root)

	concurrent, err := mutate.RenameTitle(load(t, root), d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "E-8wqkyr", Title: "Concurrent Foundation",
	})
	require.NoError(t, err)
	require.True(t, concurrent.Changed)
	newer, err := os.ReadFile(path)
	require.NoError(t, err)
	eventsBefore, err := events.List(load(t, root))
	require.NoError(t, err)

	_, err = mutate.RenameTitle(staleTree, d, mutate.RenameTitleInput{
		Noun: store.NounEpic, Node: "E-8wqkyr", Title: "Platform Foundation",
	})
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)

	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, newer, after, "the independently committed bytes win intact")
	var epic store.Epic
	readYAMLTest(t, path, &epic)
	require.Equal(t, "Concurrent Foundation", epic.Title)
	eventsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsAfter, len(eventsBefore), "the refused stale rename writes no event")
}

func TestRenameTitleRefusesFreshCanonicalIdentityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		noun      string
		node      string
		title     string
		path      func(string) string
		changeKey func(*testing.T, string)
	}{
		{
			name:  "epic key",
			noun:  store.NounEpic,
			node:  "E-8wqkyr",
			title: "Phase 0 Foundation",
			path: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
			},
			changeKey: func(t *testing.T, path string) {
				var doc store.Epic
				readYAMLTest(t, path, &doc)
				key := "renamed-foundation"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name:  "feature key",
			noun:  store.NounFeature,
			node:  "F-ap1234",
			title: "Must not land",
			path: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "feature.yml")
			},
			changeKey: func(t *testing.T, path string) {
				var doc store.Feature
				readYAMLTest(t, path, &doc)
				key := "foundation/renamed-app"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name:  "task local key",
			noun:  store.NounTask,
			node:  "T-sc1234",
			title: "Must not land",
			path: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold", "task.yml")
			},
			changeKey: func(t *testing.T, path string) {
				var doc store.Task
				readYAMLTest(t, path, &doc)
				key := "foundation/app/renamed-scaffold"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name:  "task parent key",
			noun:  store.NounTask,
			node:  "T-sc1234",
			title: "Must not land",
			path: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold", "task.yml")
			},
			changeKey: func(t *testing.T, path string) {
				var doc store.Task
				readYAMLTest(t, path, &doc)
				key := "foundation/other/scaffold"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := layRenameTree(t)
			staleTree := load(t, root)
			path := tt.path(root)
			tt.changeKey(t, path)
			newer, err := os.ReadFile(path)
			require.NoError(t, err)
			eventsBefore, err := events.List(staleTree)
			require.NoError(t, err)

			_, err = mutate.RenameTitle(staleTree, d, mutate.RenameTitleInput{
				Noun: tt.noun, Node: tt.node, Title: tt.title,
			})
			var env *store.EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, store.ReasonMutationStale, env.Reason)

			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, newer, after, "the renamed key and original title remain byte-for-byte intact")
			eventsAfter, listErr := events.List(staleTree)
			require.NoError(t, listErr)
			require.Len(t, eventsAfter, len(eventsBefore), "identity mismatch writes no rename event")
		})
	}
}

type renameCallbackClock struct {
	run func()
}

func (c renameCallbackClock) Now() time.Time {
	if c.run != nil {
		c.run()
	}
	return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
}

func writeRenameKeyFixture(t *testing.T) (string, mutate.Deps) {
	t.Helper()
	root := writeOwnedRepo(t)
	d := renameDeps()

	foundationKey := "foundation"
	foundation := store.Epic{
		ID: "E-found1", Key: &foundationKey, Title: "Foundation", Status: "draft",
		Source: "design/platform.md", Order: []string{"F-app001", "F-side01"},
	}
	growthKey := "growth"
	growth := store.Epic{
		ID: "E-growth", Key: &growthKey, Title: "Growth", Status: "draft",
		Source: "design/growth.md", Order: []string{"F-other1"},
	}
	appKey := "foundation/app"
	app := store.Feature{
		ID: "F-app001", Key: &appKey, Title: "App", Status: "draft",
		Source: "design/app.md", DependsOn: []string{"F-base01"}, Order: []string{"T-never1"},
	}
	sideKey := "foundation/sidecar"
	side := store.Feature{
		ID: "F-side01", Key: &sideKey, Title: "Sidecar", Status: "draft",
		Source: "design/sidecar.md",
	}
	otherKey := "growth/other"
	other := store.Feature{
		ID: "F-other1", Key: &otherKey, Title: "Other", Status: "draft",
		Source: "design/other.md",
	}

	writeRenameNode := func(relDir, stateName, proseName string, state any, prose []byte) {
		t.Helper()
		dir := filepath.Join(root, relDir)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		writeYAMLTest(t, filepath.Join(dir, stateName), state)
		require.NoError(t, os.WriteFile(filepath.Join(dir, proseName), prose, 0o644))
	}
	writeRenameNode(filepath.Join(".tiller", "epics", "foundation"), "epic.yml", "epic.md", foundation, []byte("# Foundation\n\nDo not rewrite this heading.\n"))
	writeRenameNode(filepath.Join(".tiller", "epics", "growth"), "epic.yml", "epic.md", growth, []byte("# Growth\n"))
	writeRenameNode(filepath.Join(".tiller", "epics", "foundation", "features", "app"), "feature.yml", "feature.md", app, []byte("# App\n"))
	writeRenameNode(filepath.Join(".tiller", "epics", "foundation", "features", "sidecar"), "feature.yml", "feature.md", side, []byte("# Sidecar\n"))
	writeRenameNode(filepath.Join(".tiller", "epics", "growth", "features", "other"), "feature.yml", "feature.md", other, []byte("# Other\n"))

	appDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "feature.eval.md"), []byte("node: F-app001\nsource: foundation/app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "companions", "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "companions", "nested", "opaque.bin"), []byte{0, 1, 2, 0xff}, 0o644))
	return root, d
}

func writeRenameTask(t *testing.T, root string, status string) {
	t.Helper()
	key := "foundation/app/scaffold"
	task := store.Task{
		ID: "T-scaf01", Key: &key, Title: "Scaffold", Status: status,
		ProposedBy: "jc", DependsOn: []string{"T-base01"},
	}
	dir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	writeYAMLTest(t, filepath.Join(dir, "task.yml"), task)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task.md"), []byte("# Scaffold\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.txt"), []byte("keep exactly\n"), 0o644))
}

func TestRenameKeyMovesDraftEpicAndRewritesOnlyCanonicalStateKeys(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	oldRoot := filepath.Join(root, ".tiller", "epics", "foundation")
	beforeEval, err := os.ReadFile(filepath.Join(oldRoot, "features", "app", "feature.eval.md"))
	require.NoError(t, err)
	beforeOpaque, err := os.ReadFile(filepath.Join(oldRoot, "features", "app", "companions", "nested", "opaque.bin"))
	require.NoError(t, err)

	res, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounEpic, Node: "E-found1", Key: "platform-foundation",
	})
	require.NoError(t, err)
	require.Equal(t, store.NounEpic, res.Noun)
	require.Equal(t, "E-found1", res.ID)
	require.Equal(t, "foundation", res.OldKey)
	require.Equal(t, "platform-foundation", res.Key)
	require.True(t, res.Changed)
	require.NotEmpty(t, res.Moved)
	require.True(t, sort.StringsAreSorted(res.Moved))
	require.NoDirExists(t, oldRoot)

	newRoot := filepath.Join(root, ".tiller", "epics", "platform-foundation")
	require.DirExists(t, newRoot)
	afterEval, err := os.ReadFile(filepath.Join(newRoot, "features", "app", "feature.eval.md"))
	require.NoError(t, err)
	require.Equal(t, beforeEval, afterEval, "historical eval/source text is not rewritten")
	afterOpaque, err := os.ReadFile(filepath.Join(newRoot, "features", "app", "companions", "nested", "opaque.bin"))
	require.NoError(t, err)
	require.Equal(t, beforeOpaque, afterOpaque, "unknown regular companions are preserved byte-for-byte")

	reloaded := load(t, root)
	require.Equal(t, "platform-foundation", *reloaded.Epics["E-found1"].Key)
	require.Equal(t, []string{"F-app001", "F-side01"}, reloaded.Epics["E-found1"].Order)
	require.Equal(t, "design/platform.md", reloaded.Epics["E-found1"].Source)
	require.Equal(t, "platform-foundation/app", *reloaded.Features["F-app001"].Key)
	require.Equal(t, "platform-foundation/sidecar", *reloaded.Features["F-side01"].Key)
	require.Equal(t, []string{"F-base01"}, reloaded.Features["F-app001"].DependsOn)
	require.Equal(t, []string{"T-never1"}, reloaded.Features["F-app001"].Order)
	require.Equal(t, "design/app.md", reloaded.Features["F-app001"].Source)

	evs, err := events.List(reloaded)
	require.NoError(t, err)
	var ev events.Event
	for _, candidate := range evs {
		if candidate.Type == "rename.key.epic" {
			ev = candidate
		}
	}
	require.Equal(t, "rename epic E-found1 --key \"platform-foundation\"", ev.Command)
	require.Equal(t, map[string]any{"key": "foundation"}, ev.Before)
	require.Equal(t, map[string]any{"key": "platform-foundation"}, ev.After)
	require.Equal(t, []string{"E-found1", "F-app001", "F-side01"}, ev.Targets)
	childHistory := views.NodePageOf(reloaded, evs, reloaded.Features["F-app001"], nil).History
	require.Condition(t, func() bool {
		for _, entry := range childHistory {
			if entry.Type == "rename.key.epic" {
				return true
			}
		}
		return false
	}, "a child whose canonical key changed must receive the epic rename in its history")
	require.Len(t, ev.Files, len(res.Moved))
	for i := range ev.Files {
		require.Equal(t, res.Moved[i], ev.Files[i].Path)
	}
}

func TestRenameKeyPreservesMovedFileAndDirectoryModesWithoutGitModeDrift(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
	executable := filepath.Join(oldDir, "feature.eval.md")
	groupReadable := filepath.Join(oldDir, "companions", "nested", "opaque.bin")
	companionsDir := filepath.Join(oldDir, "companions")
	nestedDir := filepath.Join(companionsDir, "nested")
	require.NoError(t, os.Chmod(executable, 0o750))
	require.NoError(t, os.Chmod(groupReadable, 0o640))
	require.NoError(t, os.Chmod(companionsDir, 0o750))
	require.NoError(t, os.Chmod(nestedDir, 0o710))
	gitInitM(t, root)
	gitCommitAllM(t, root, "before rename")

	_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	require.NoError(t, err)

	newDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		require.Equal(t, want, info.Mode().Perm(), path)
	}
	assertMode(filepath.Join(newDir, "feature.eval.md"), 0o750)
	assertMode(filepath.Join(newDir, "companions", "nested", "opaque.bin"), 0o640)
	assertMode(filepath.Join(newDir, "companions"), 0o750)
	assertMode(filepath.Join(newDir, "companions", "nested"), 0o710)
	require.NotContains(t, gitOutM(t, root, "diff", "--summary", "-M"), "mode change",
		"moving an executable companion must not manufacture executable-bit drift")
}

func TestRenameKeyPreservesNestedEmptyDirectoryUnderNonWritableParent(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
	oldEmptyParent := filepath.Join(oldDir, "companions", "empty")
	oldEmpty := filepath.Join(oldEmptyParent, "nested")
	newEmptyParent := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application", "companions", "empty")
	t.Cleanup(func() {
		_ = os.Chmod(oldEmptyParent, 0o755)
		_ = os.Chmod(newEmptyParent, 0o755)
	})
	require.NoError(t, os.Mkdir(oldEmptyParent, 0o755))
	require.NoError(t, os.Mkdir(oldEmpty, 0o755))
	require.NoError(t, os.Chmod(oldEmptyParent, os.ModeSticky|0o555))
	require.NoError(t, os.Chmod(oldEmpty, 0o770))

	_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	require.NoError(t, err)

	require.NoDirExists(t, oldDir)
	newEmpty := filepath.Join(newEmptyParent, "nested")
	entries, err := os.ReadDir(newEmpty)
	require.NoError(t, err)
	require.Empty(t, entries)
	parentInfo, err := os.Stat(newEmptyParent)
	require.NoError(t, err)
	require.Equal(t, os.ModeSticky|os.FileMode(0o555), parentInfo.Mode()&(os.ModeSticky|os.ModePerm))
	info, err := os.Stat(newEmpty)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o770), info.Mode().Perm())
}

func TestRenameKeyRenamesDraftFeatureLocallyAndPreservesCompanions(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
	opaqueBefore, err := os.ReadFile(filepath.Join(oldDir, "companions", "nested", "opaque.bin"))
	require.NoError(t, err)

	res, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	require.NoError(t, err)
	require.Equal(t, mutate.RenameKeyResult{
		Noun: store.NounFeature, ID: "F-app001", OldKey: "foundation/app",
		Key: "foundation/application", Moved: res.Moved, Changed: true,
	}, res)
	require.NoDirExists(t, oldDir)
	newDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
	opaqueAfter, err := os.ReadFile(filepath.Join(newDir, "companions", "nested", "opaque.bin"))
	require.NoError(t, err)
	require.Equal(t, opaqueBefore, opaqueAfter)

	reloaded := load(t, root)
	require.Equal(t, "F-app001", reloaded.Features["F-app001"].ID)
	require.Equal(t, "foundation/application", *reloaded.Features["F-app001"].Key)
	require.Equal(t, "foundation", reloaded.Features["F-app001"].EpicKey)
	require.Equal(t, []string{"F-app001", "F-side01"}, reloaded.Epics["E-found1"].Order)
}

func TestRenameKeyReparentsDraftFeatureAndCleansOnlySourceOrder(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	res, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "foundation/app", Key: "growth/application",
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "foundation/app", res.OldKey)
	require.Equal(t, "growth/application", res.Key)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))

	reloaded := load(t, root)
	feature := reloaded.Features["F-app001"]
	require.Equal(t, "F-app001", feature.ID)
	require.Equal(t, "growth/application", *feature.Key)
	require.Equal(t, "growth", feature.EpicKey)
	require.Equal(t, []string{"F-side01"}, reloaded.Epics["E-found1"].Order)
	require.Equal(t, []string{"F-other1"}, reloaded.Epics["E-growth"].Order, "destination order stays otherwise untouched")

	evs, err := events.List(reloaded)
	require.NoError(t, err)
	var ev events.Event
	for _, candidate := range evs {
		if candidate.Type == "rename.key.feature" {
			ev = candidate
		}
	}
	require.Equal(t, "rename feature F-app001 --key \"growth/application\"", ev.Command)
	require.Equal(t, map[string]any{"key": "foundation/app", "epic": "foundation"}, ev.Before)
	require.Equal(t, map[string]any{"key": "growth/application", "epic": "growth"}, ev.After)
	require.Equal(t, []string{"F-app001", "E-found1"}, ev.Targets)
	require.NotContains(t, ev.Targets, "E-growth", "the unchanged destination order is not an event target")
	sourceHistory := views.NodePageOf(reloaded, evs, reloaded.Epics["E-found1"], nil).History
	require.Condition(t, func() bool {
		for _, entry := range sourceHistory {
			if entry.Type == "rename.key.feature" {
				return true
			}
		}
		return false
	}, "the source epic whose order changed must receive the feature rename in its history")
	require.Contains(t, eventPaths(ev), filepath.Join(".tiller", "epics", "foundation", "epic.yml"), "the source order rewrite is in the event")
}

func TestRenameKeyRenamesProposedTaskWithinItsFeature(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	writeRenameTask(t, root, "proposed")
	oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold")

	res, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounTask, Node: "T-scaf01", Key: "foundation/app/bootstrap",
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "foundation/app/scaffold", res.OldKey)
	require.Equal(t, "foundation/app/bootstrap", res.Key)
	require.NoDirExists(t, oldDir)

	reloaded := load(t, root)
	task := reloaded.Tasks["T-scaf01"]
	require.Equal(t, "T-scaf01", task.ID)
	require.Equal(t, "foundation/app/bootstrap", *task.Key)
	require.Equal(t, "F-app001", task.FeatureID)
	require.Equal(t, []string{"T-base01"}, task.DependsOn)
	companion, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "bootstrap", "review.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("keep exactly\n"), companion)
	evs, err := events.List(reloaded)
	require.NoError(t, err)
	var ev events.Event
	for _, candidate := range evs {
		if candidate.Type == "rename.key.task" {
			ev = candidate
		}
	}
	require.Equal(t, "rename task T-scaf01 --key \"foundation/app/bootstrap\"", ev.Command)
	require.Equal(t, map[string]any{"key": "foundation/app/scaffold"}, ev.Before)
	require.Equal(t, map[string]any{"key": "foundation/app/bootstrap"}, ev.After)
	require.Equal(t, res.Moved, eventPaths(ev))
}

func TestRenameKeySameCanonicalKeyIsNoChangeWithoutEvent(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	before := snapshotDir(t, root)
	evsBefore, err := events.List(load(t, root))
	require.NoError(t, err)

	res, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/app",
	})
	require.NoError(t, err)
	require.False(t, res.Changed)
	require.Empty(t, res.Moved)
	require.Equal(t, before, snapshotDir(t, root))
	evsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, evsAfter, len(evsBefore))
}

func TestRenameKeyRefusesInvalidCanonicalKeyFormsWithoutWriting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		noun string
		node string
		key  string
	}{
		{name: "epic segment count", noun: store.NounEpic, node: "E-found1", key: "new/epic"},
		{name: "epic grammar", noun: store.NounEpic, node: "E-found1", key: "New-Epic"},
		{name: "feature segment count", noun: store.NounFeature, node: "F-app001", key: "application"},
		{name: "feature grammar", noun: store.NounFeature, node: "F-app001", key: "foundation/App"},
		{name: "task segment count", noun: store.NounTask, node: "T-scaf01", key: "foundation/bootstrap"},
		{name: "task grammar", noun: store.NounTask, node: "T-scaf01", key: "foundation/app/BootStrap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			before := snapshotDir(t, root)
			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: tt.noun, Node: tt.node, Key: tt.key,
			})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "invalid_key", reason.Reason())
			require.Equal(t, before, snapshotDir(t, root))
		})
	}
}

// TestRenameKeyRefusesReservedEpicKeys closes the back door around
// store.IsReservedEpicKey, which `add epic` already refuses: an epic keyed
// bugs or chores would put .tiller/epics/bugs/ under a name whose feature
// keys (bugs/<f>) collide with shelf keys in the tree's by-key index and
// whose branch names (tiller/bugs/<f>) collide with a bug's own branch.
// Refusing at add and allowing at rename would leave the collision one
// command away.
func TestRenameKeyRefusesReservedEpicKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"bugs", "chores"} {
		t.Run(key, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			before := snapshotDir(t, root)
			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: store.NounEpic, Node: "E-found1", Key: key,
			})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "reserved_key", reason.Reason())
			require.Equal(t, before, snapshotDir(t, root))
		})
	}

	// A feature or a bug may still be keyed bugs: only the epic key is the
	// shelf folder name, and a feature's own segment lives one level down.
	root, d := writeRenameKeyFixture(t)
	_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/bugs",
	})
	require.NoError(t, err)
}

func TestRenameKeyRefusesDestinationCollisionWithoutPartialPath(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	collision := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
	require.NoError(t, os.WriteFile(collision, []byte("mine\n"), 0o644))
	before := snapshotDir(t, root)

	_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "rename_destination_exists", reason.Reason())
	require.Equal(t, before, snapshotDir(t, root))
}

func TestRenameKeyRefusesLockedLifecycleShapesWithoutWriting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		noun   string
		node   string
		key    string
	}{
		{
			name: "epic not draft", noun: store.NounEpic, node: "E-found1", key: "platform",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, ".tiller", "epics", "foundation", "epic.yml")
				var doc store.Epic
				readYAMLTest(t, path, &doc)
				doc.Status = "ready"
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name: "epic child not draft", noun: store.NounEpic, node: "E-found1", key: "platform",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "feature.yml")
				var doc store.Feature
				readYAMLTest(t, path, &doc)
				doc.Status = "ready"
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name: "epic descendant task", noun: store.NounEpic, node: "E-found1", key: "platform",
			mutate: func(t *testing.T, root string) { writeRenameTask(t, root, "proposed") },
		},
		{
			name: "feature not draft", noun: store.NounFeature, node: "F-app001", key: "foundation/application",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "feature.yml")
				var doc store.Feature
				readYAMLTest(t, path, &doc)
				doc.Status = "ready"
				writeYAMLTest(t, path, doc)
			},
		},
		{
			name: "feature has task", noun: store.NounFeature, node: "F-app001", key: "foundation/application",
			mutate: func(t *testing.T, root string) { writeRenameTask(t, root, "proposed") },
		},
		{
			name: "task not proposed", noun: store.NounTask, node: "T-scaf01", key: "foundation/app/bootstrap",
			mutate: func(t *testing.T, root string) { writeRenameTask(t, root, "ready") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			tt.mutate(t, root)
			before := snapshotDir(t, root)
			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: tt.noun, Node: tt.node, Key: tt.key,
			})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "key_rename_locked", reason.Reason())
			require.Equal(t, before, snapshotDir(t, root))
		})
	}
}

func TestRenameKeyRefusesReparentUnlessBothEpicsAreDraft(t *testing.T) {
	t.Parallel()
	for _, epic := range []string{"foundation", "growth"} {
		t.Run(epic, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			path := filepath.Join(root, ".tiller", "epics", epic, "epic.yml")
			var doc store.Epic
			readYAMLTest(t, path, &doc)
			doc.Status = "ready"
			writeYAMLTest(t, path, doc)
			before := snapshotDir(t, root)

			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: store.NounFeature, Node: "F-app001", Key: "growth/application",
			})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "key_rename_locked", reason.Reason())
			require.Equal(t, before, snapshotDir(t, root))
		})
	}
}

func TestRenameKeyRefusesTaskReparentWithoutWriting(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	writeRenameTask(t, root, "proposed")
	before := snapshotDir(t, root)
	_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounTask, Node: "T-scaf01", Key: "growth/other/bootstrap",
	})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "task_reparent_not_supported", reason.Reason())
	require.Equal(t, before, snapshotDir(t, root))
}

func TestRenameKeyRefusesSymlinkAndNonRegularCompanions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		add  func(*testing.T, string)
	}{
		{
			name: "symlink",
			add: func(t *testing.T, dir string) {
				require.NoError(t, os.Symlink("feature.md", filepath.Join(dir, "alias.md")))
			},
		},
		{
			name: "fifo",
			add: func(t *testing.T, dir string) {
				require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			source := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
			tt.add(t, source)
			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
			})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "non_regular_move_entry", reason.Reason())
			require.DirExists(t, source)
			require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "application"))
		})
	}
}

func TestRenameKeyCommitGuardsSourceDestinationAndInventoryWithoutPartialPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*testing.T, string)
	}{
		{
			name: "source read",
			change: func(t *testing.T, root string) {
				path := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "feature.md")
				require.NoError(t, os.WriteFile(path, []byte("concurrent source\n"), 0o644))
			},
		},
		{
			name: "destination absence",
			change: func(t *testing.T, root string) {
				dir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "concurrent.txt"), []byte("mine\n"), 0o644))
			},
		},
		{
			name: "lineage inventory",
			change: func(t *testing.T, root string) {
				dir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "concurrent")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				key := "foundation/concurrent"
				writeYAMLTest(t, filepath.Join(dir, "feature.yml"), store.Feature{ID: "F-concur", Key: &key, Status: "draft"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			fired := false
			d.Clock = renameCallbackClock{run: func() {
				if fired {
					return
				}
				fired = true
				tt.change(t, root)
			}}
			_, err := mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
				Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
			})
			var env *store.EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, store.ReasonMutationStale, env.Reason)
			require.DirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))
			require.NoFileExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "application", "feature.yml"))
		})
	}
}

func eventPaths(ev events.Event) []string {
	paths := make([]string, len(ev.Files))
	for i := range ev.Files {
		paths[i] = ev.Files[i].Path
	}
	return paths
}

func TestRenameKeyReparentRejectsIntermediateAndFinalDestinationSymlinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{
			name: "intermediate features symlink",
			setup: func(t *testing.T, root, external string) {
				features := filepath.Join(root, ".tiller", "epics", "growth", "features")
				other := filepath.Join(external, "other")
				require.NoError(t, os.MkdirAll(other, 0o755))
				for _, name := range []string{"feature.yml", "feature.md"} {
					data, err := os.ReadFile(filepath.Join(features, "other", name))
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(other, name), data, 0o644))
				}
				require.NoError(t, os.RemoveAll(features))
				require.NoError(t, os.Symlink(external, features))
			},
		},
		{
			name: "final destination symlink",
			setup: func(t *testing.T, root, external string) {
				link := filepath.Join(root, ".tiller", "epics", "growth", "features", "application")
				require.NoError(t, os.Symlink(external, link))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			staleTree := load(t, root)
			external := t.TempDir()
			sentinel := filepath.Join(external, "sentinel.txt")
			require.NoError(t, os.WriteFile(sentinel, []byte("outside\n"), 0o600))
			tt.setup(t, root, external)

			_, err := mutate.RenameKey(staleTree, d, mutate.RenameKeyInput{
				Noun: store.NounFeature, Node: "F-app001", Key: "growth/application",
			})
			require.Error(t, err)
			require.DirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))
			require.NoFileExists(t, filepath.Join(external, "application", "feature.yml"))
			data, readErr := os.ReadFile(sentinel)
			require.NoError(t, readErr)
			require.Equal(t, []byte("outside\n"), data)
		})
	}
}

func TestRenameKeyRefusesLoadedLineageIDsAtDifferentCanonicalPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(*testing.T, string)
		noun        string
		node        string
		key         string
		untouched   func(string) string
		destination func(string) string
	}{
		{
			name: "feature path",
			setup: func(t *testing.T, root string) {
				oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app")
				newDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "relocated")
				require.NoError(t, os.Rename(oldDir, newDir))
				path := filepath.Join(newDir, "feature.yml")
				var doc store.Feature
				readYAMLTest(t, path, &doc)
				key := "foundation/relocated"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
			noun: store.NounFeature, node: "F-app001", key: "foundation/product",
			untouched: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "relocated")
			},
			destination: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "product")
			},
		},
		{
			name: "task path",
			setup: func(t *testing.T, root string) {
				oldDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold")
				newDir := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "relocated")
				require.NoError(t, os.Rename(oldDir, newDir))
				path := filepath.Join(newDir, "task.yml")
				var doc store.Task
				readYAMLTest(t, path, &doc)
				key := "foundation/app/relocated"
				doc.Key = &key
				writeYAMLTest(t, path, doc)
			},
			noun: store.NounTask, node: "T-scaf01", key: "foundation/app/product",
			untouched: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "relocated")
			},
			destination: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "tasks", "product")
			},
		},
		{
			name: "epic path",
			setup: func(t *testing.T, root string) {
				oldDir := filepath.Join(root, ".tiller", "epics", "foundation")
				newDir := filepath.Join(root, ".tiller", "epics", "relocated")
				require.NoError(t, os.Rename(oldDir, newDir))
				epicPath := filepath.Join(newDir, "epic.yml")
				var epic store.Epic
				readYAMLTest(t, epicPath, &epic)
				key := "relocated"
				epic.Key = &key
				writeYAMLTest(t, epicPath, epic)
				for _, local := range []string{"app", "sidecar"} {
					path := filepath.Join(newDir, "features", local, "feature.yml")
					var feature store.Feature
					readYAMLTest(t, path, &feature)
					featureKey := "relocated/" + local
					feature.Key = &featureKey
					writeYAMLTest(t, path, feature)
				}
			},
			noun: store.NounEpic, node: "E-found1", key: "product",
			untouched: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "relocated")
			},
			destination: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "product")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			if tt.noun == store.NounTask {
				writeRenameTask(t, root, "proposed")
			}
			staleTree := load(t, root)
			tt.setup(t, root)

			_, err := mutate.RenameKey(staleTree, d, mutate.RenameKeyInput{
				Noun: tt.noun, Node: tt.node, Key: tt.key,
			})
			var env *store.EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, store.ReasonMutationStale, env.Reason)
			require.DirExists(t, tt.untouched(root))
			require.NoDirExists(t, tt.destination(root))
		})
	}
}

func TestRenameKeySourceKindChangeNeverBlocksOrPartiallyMoves(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(string, string) error
	}{
		{
			name: "symlink",
			change: func(path, external string) error {
				if err := os.Remove(path); err != nil {
					return err
				}
				return os.Symlink(external, path)
			},
		},
		{
			name: "fifo",
			change: func(path, _ string) error {
				if err := os.Remove(path); err != nil {
					return err
				}
				return syscall.Mkfifo(path, 0o600)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, d := writeRenameKeyFixture(t)
			source := filepath.Join(root, ".tiller", "epics", "foundation", "features", "app", "companions", "nested", "opaque.bin")
			external := filepath.Join(t.TempDir(), "outside.bin")
			require.NoError(t, os.WriteFile(external, []byte("outside\n"), 0o600))
			tree := load(t, root)
			var changeErr error
			fired := false
			d.Clock = renameCallbackClock{run: func() {
				if fired {
					return
				}
				fired = true
				changeErr = tt.change(source, external)
			}}

			done := make(chan error, 1)
			go func() {
				_, err := mutate.RenameKey(tree, d, mutate.RenameKeyInput{
					Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
				})
				done <- err
			}()
			select {
			case err := <-done:
				require.Error(t, err)
			// A rename that opens the fifo for reading blocks until a writer
			// arrives, which never happens on its own; so the wait only has to
			// outlast a slow rename on a loaded machine, not race a fast one.
			case <-time.After(10 * time.Second):
				writer, openErr := os.OpenFile(source, os.O_WRONLY|syscall.O_NONBLOCK, 0)
				if openErr == nil {
					_ = writer.Close()
				}
				<-done
				t.Fatal("rename blocked opening a nonregular file that replaced a planned regular companion")
			}
			require.NoError(t, changeErr)
			require.DirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))
			require.NoFileExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "application", "feature.yml"))
			outside, err := os.ReadFile(external)
			require.NoError(t, err)
			require.Equal(t, []byte("outside\n"), outside)
		})
	}
}

func TestRenameKeyExistingUnreadableRegularDestinationIsStableCollision(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	destination := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
	require.NoError(t, os.WriteFile(destination, []byte("private\n"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(destination, 0o600) })
	before, err := os.Lstat(destination)
	require.NoError(t, err)

	_, err = mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "rename_destination_exists", reason.Reason())
	after, statErr := os.Lstat(destination)
	require.NoError(t, statErr)
	require.True(t, os.SameFile(before, after))
	require.DirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))
}

func TestRenameKeyExistingLargeSparseDestinationIsStableCollision(t *testing.T) {
	t.Parallel()
	root, d := writeRenameKeyFixture(t)
	destination := filepath.Join(root, ".tiller", "epics", "foundation", "features", "application")
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(8<<30))
	require.NoError(t, file.Close())

	_, err = mutate.RenameKey(load(t, root), d, mutate.RenameKeyInput{
		Noun: store.NounFeature, Node: "F-app001", Key: "foundation/application",
	})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "rename_destination_exists", reason.Reason())
	info, statErr := os.Stat(destination)
	require.NoError(t, statErr)
	require.Equal(t, int64(8<<30), info.Size())
	require.DirExists(t, filepath.Join(root, ".tiller", "epics", "foundation", "features", "app"))
}
