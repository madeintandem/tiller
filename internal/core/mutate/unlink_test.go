package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUnlinkRemovesResolvedIDsAndPreservesSurvivorOrderAtEveryGrain(t *testing.T) {
	t.Parallel()
	t.Run("epic by key and id", func(t *testing.T) {
		root := dependsRepo(t)
		addEpicWithDependencyMint(t, root, "payments", "dddddd")
		addEpicWithDependencyMint(t, root, "billing", "eeeeee")
		addEpicWithDependencyMint(t, root, "delivery", "ffffff")
		_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
			Noun: store.NounEpic, Node: "delivery", DependsOn: []string{"form-engine", "payments", "billing"},
		})
		require.NoError(t, err)

		res, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
			Noun: store.NounEpic, Node: "delivery", DependsOn: []string{"E-dddddd", "form-engine"},
		})
		require.NoError(t, err)
		require.True(t, res.Changed)
		require.Equal(t, []string{"E-eeeeee"}, res.DependsOn)
		require.Equal(t, []string{"E-aaaaaa", "E-dddddd"}, res.Removed,
			"removed ids follow their stable stored order, not request order")
		require.Equal(t, []string{"E-eeeeee"}, load(t, root).Epics["E-ffffff"].DependsOn)
	})

	t.Run("feature by key and id", func(t *testing.T) {
		root := dependsRepo(t)
		addEpicWithDependencyMint(t, root, "payments", "dddddd")
		_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
			Noun: store.NounFeature, Node: "form-engine/nrca-form-loading",
			DependsOn: []string{"form-engine/schema-engine", "form-engine", "payments"},
		})
		require.NoError(t, err)

		res, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
			Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"E-aaaaaa", "form-engine/schema-engine"},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"E-dddddd"}, res.DependsOn)
		require.Equal(t, []string{"F-bbbbbb", "E-aaaaaa"}, res.Removed)
		require.Equal(t, []string{"E-dddddd"}, load(t, root).Features["F-cccccc"].DependsOn)
	})

	t.Run("sibling task by key and id", func(t *testing.T) {
		root := siblingTaskRepo(t)
		addSiblingTaskWithDependencyMint(t, root, "third", "dddddd")
		addSiblingTaskWithDependencyMint(t, root, "fourth", "eeeeee")
		_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
			Noun: store.NounTask, Node: "form-engine/schema-engine/first",
			DependsOn: []string{"form-engine/schema-engine/second", "form-engine/schema-engine/third", "form-engine/schema-engine/fourth"},
		})
		require.NoError(t, err)

		res, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
			Noun: store.NounTask, Node: "T-first0", DependsOn: []string{"form-engine/schema-engine/fourth", "T-second"},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"T-dddddd"}, res.DependsOn)
		require.Equal(t, []string{"T-second", "T-eeeeee"}, res.Removed)
		require.Equal(t, []string{"T-dddddd"}, load(t, root).Tasks["T-first0"].DependsOn)
	})
}

func TestUnlinkResolvesEveryTargetBeforeWritingAndInheritsDependencyRefusals(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/nrca-form-loading", DependsOn: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)

	for _, targets := range [][]string{
		{"form-engine/schema-engine", "F-zzzzzz"},
		{"form-engine/nrca-form-loading"},
		{"B-aaaaaa"},
	} {
		before := snapshotDir(t, root)
		_, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
			Noun: store.NounFeature, Node: "form-engine/nrca-form-loading", DependsOn: targets,
		})
		require.Error(t, err)
		requireNoWrites(t, before, snapshotDir(t, root))
	}

	taskRoot := siblingTaskRepo(t)
	addSiblingTaskWithDependencyMint(t, taskRoot, "third", "dddddd")
	_, err = mutate.Link(load(t, taskRoot), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "form-engine/schema-engine/first", DependsOn: []string{"form-engine/schema-engine/second"},
	})
	require.NoError(t, err)
	before := snapshotDir(t, taskRoot)
	_, err = mutate.Unlink(load(t, taskRoot), testDeps(), mutate.UnlinkInput{
		Noun: store.NounTask, Node: "form-engine/schema-engine/first",
		DependsOn: []string{"form-engine/schema-engine/second", "form-engine/nrca-form-loading/other"},
	})
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, taskRoot))
}

func TestUnlinkRecordsCompleteEvent(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/nrca-form-loading",
		DependsOn: []string{"form-engine/schema-engine", "form-engine"},
	})
	require.NoError(t, err)

	res, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
		Noun: store.NounFeature, Node: "form-engine/nrca-form-loading", DependsOn: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"F-bbbbbb"}, res.Removed)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	unlinked := eventsOfType(evs, "unlink.feature")
	require.Len(t, unlinked, 1)
	require.Equal(t, []any{"F-bbbbbb", "E-aaaaaa"}, unlinked[0].Before["depends_on"])
	require.Equal(t, []any{"E-aaaaaa"}, unlinked[0].After["depends_on"])
	require.Equal(t, []any{"F-bbbbbb"}, unlinked[0].After["removed"])
}

func TestUnlinkStaleSnapshotRefusesWithoutStateOrEventWrite(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/nrca-form-loading", DependsOn: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)
	featurePath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading", "feature.yml")

	d := testDeps()
	d.Clock = dependencyMutatingClock{mutate: func() {
		var doc store.Feature
		data, readErr := os.ReadFile(featurePath)
		require.NoError(t, readErr)
		require.NoError(t, yaml.Unmarshal(data, &doc))
		doc.Source = "concurrent-edit"
		data, readErr = yaml.Marshal(doc)
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(featurePath, data, 0o644))
	}}
	beforeEvents := countFiles(t, filepath.Join(root, ".tiller", "events"))

	_, err = mutate.Unlink(load(t, root), d, mutate.UnlinkInput{
		Noun: store.NounFeature, Node: "form-engine/nrca-form-loading", DependsOn: []string{"form-engine/schema-engine"},
	})
	require.Error(t, err)
	require.Equal(t, []string{"F-bbbbbb"}, load(t, root).Features["F-cccccc"].DependsOn)
	require.Equal(t, beforeEvents, countFiles(t, filepath.Join(root, ".tiller", "events")))
}

func addEpicWithDependencyMint(t *testing.T, root, key, suffix string) {
	t.Helper()
	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-" + suffix }
	_, err := mutate.AddEpic(load(t, root), d, mutate.EpicInput{Key: key, Title: key})
	require.NoError(t, err)
}

func addSiblingTaskWithDependencyMint(t *testing.T, root, key, suffix string) {
	t.Helper()
	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-" + suffix }
	_, err := mutate.AddTask(load(t, root), d, mutate.TaskInput{
		Key: "form-engine/schema-engine/" + key, Title: key,
	})
	require.NoError(t, err)
}

type dependencyMutatingClock struct {
	mutate func()
}

func (c dependencyMutatingClock) Now() time.Time {
	c.mutate()
	return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
}

func setDependencyNodeState(t *testing.T, root, noun, ref, status, branch string) {
	t.Helper()
	node := resolveNode(t, load(t, root), noun, ref)
	var path string
	switch doc := node.(type) {
	case *store.Epic:
		path = filepath.Join(root, ".tiller", "epics", *doc.Key, "epic.yml")
		var fresh store.Epic
		readDependencyYAML(t, path, &fresh)
		fresh.Status = status
		fresh.Branch = branch
		writeDependencyYAML(t, path, fresh)
	case *store.Feature:
		parts := strings.Split(*doc.Key, "/")
		require.Len(t, parts, 2)
		path = filepath.Join(root, ".tiller", "epics", parts[0], "features", parts[1], "feature.yml")
		var fresh store.Feature
		readDependencyYAML(t, path, &fresh)
		fresh.Status = status
		fresh.Branch = branch
		writeDependencyYAML(t, path, fresh)
	case *store.Task:
		path = taskYMLPath(t, root, doc.ID)
		var fresh store.Task
		readDependencyYAML(t, path, &fresh)
		fresh.Status = status
		if status == "in_progress" || status == "check" || status == "in_review" || status == "done" {
			fresh.Started = true
			fresh.Owner = "jc"
		}
		writeDependencyYAML(t, path, fresh)
	default:
		t.Fatalf("unsupported dependency node %T", node)
	}
}

func readDependencyYAML(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, out))
}

func writeDependencyYAML(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
