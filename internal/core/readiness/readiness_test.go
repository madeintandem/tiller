package readiness

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// writeRepo lays down a minimal .tiller/ with the given manifest body and
// optional design doc, returning the loaded manifest.
func loadRepo(t *testing.T, manifest string, withDesign bool) *store.Manifest {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, store.WriteRepositoryFileAtomic(dir, filepath.Join(".tiller", "tiller.yml"), []byte(manifest)))
	if withDesign {
		require.NoError(t, store.WriteRepositoryFileAtomic(dir, filepath.Join(".tiller", "design", "00-index.md"), []byte("# index\n")))
	}
	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	return m
}

const completeManifest = `schema_version: 12
project: { name: P, key: p, kind: greenfield }
contributors:
  - { id: jc, name: JC Grubbs }
`

const noOwnersManifest = `schema_version: 12
project: { name: P, key: p, kind: greenfield }
contributors: []
`

// TestCheckReady confirms a coherent project reports ready with no gaps.
func TestCheckReady(t *testing.T) {
	rep := Check(loadRepo(t, completeManifest, true), nil)
	require.True(t, rep.Ready)
	require.Empty(t, rep.Gaps)
}

// TestCheckNamesGaps confirms each shortfall is named with its stable reason and
// that the gap order is deterministic (owners before design, per store.Gaps).
func TestCheckNamesGaps(t *testing.T) {
	rep := Check(loadRepo(t, noOwnersManifest, false), nil)
	require.False(t, rep.Ready)
	require.Len(t, rep.Gaps, 2)
	require.Equal(t, store.ReasonMissingContributors, rep.Gaps[0].Reason)
	require.Equal(t, store.ReasonMissingDesign, rep.Gaps[1].Reason)
	require.NotEmpty(t, rep.Gaps[0].Message)
}

// restampedOrientation is gen.Orientation() with its header stamp rewritten to
// a different generation, the on-disk state a checkout carries when it
// disagrees with the running binary, whichever side moved.
func restampedOrientation(t *testing.T, to int) string {
	t.Helper()
	current := strconv.Itoa(gen.OrientationGeneration())
	restamped := strings.Replace(gen.Orientation(), "orientation generation "+current,
		"orientation generation "+strconv.Itoa(to), 1)
	require.NotEqual(t, gen.Orientation(), restamped, "the replace actually changed the stamp")
	return restamped
}

// A behind orientation generation remains a readiness gap.
func TestCheckNamesStaleOrientationGapBehind(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller", "orientation.md"),
		[]byte(restampedOrientation(t, 1))))

	rep := Check(m, nil)
	require.False(t, rep.Ready, "a stale orientation generation is a red gate")
	require.Len(t, rep.Gaps, 1)
	require.Contains(t, rep.Gaps[0].Message, "1", "the gap names the on-disk generation")
	require.Contains(t, rep.Gaps[0].Message, strconv.Itoa(gen.OrientationGeneration()), "and the binary's generation")
}

// A newer orientation generation is also a gap, never a reason to downgrade it.
func TestCheckNamesStaleOrientationGapAhead(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	ahead := gen.OrientationGeneration() + 1
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller", "orientation.md"),
		[]byte(restampedOrientation(t, ahead))))

	rep := Check(m, nil)
	require.False(t, rep.Ready, "a stale orientation generation is a red gate")
	require.Len(t, rep.Gaps, 1)
	require.Contains(t, rep.Gaps[0].Message, strconv.Itoa(ahead), "the gap names the on-disk generation")
	require.Contains(t, rep.Gaps[0].Message, strconv.Itoa(gen.OrientationGeneration()), "and the binary's generation")
}

// TestCheckCurrentOrientationHasNoGap confirms an orientation.md already
// stamped with the binary's generation carries no gap.
func TestCheckCurrentOrientationHasNoGap(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller", "orientation.md"), []byte(gen.Orientation())))

	rep := Check(m, nil)
	require.True(t, rep.Ready)
	require.Empty(t, rep.Gaps)
}

// TestCheckMissingOrientationKeepsExistingAnswer confirms a project with no
// orientation.md at all is not penalized for it: readiness names no new
// failure for a missing file (the design is explicit that this stays whatever
// answer readiness already gives).
func TestCheckMissingOrientationKeepsExistingAnswer(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	require.NoFileExists(t, filepath.Join(m.Root(), ".tiller", "orientation.md"))

	rep := Check(m, nil)
	require.True(t, rep.Ready)
	require.Empty(t, rep.Gaps)
}

// cycleEpic and cycleFeature are tiny constructors for the dependency-cycle
// tests, mirroring gen's own order_test fixtures.
func cycleEpic(id, key string) *store.Epic {
	k := key
	return &store.Epic{ID: id, Key: &k, Title: key, Status: "ready"}
}

func cycleFeature(id, key, epicKey string, dependsOn ...string) *store.Feature {
	k := key
	return &store.Feature{ID: id, Key: &k, EpicKey: epicKey, Title: key, Status: "draft", DependsOn: dependsOn}
}

// TestCheckAcyclicTreeAddsNoCycleGap pins the quiet side, and the nil tree
// (a caller with no tree in hand) skips the check entirely rather than
// inventing an answer.
func TestCheckAcyclicTreeAddsNoCycleGap(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-solo": cycleEpic("E-solo", "solo")},
		Features: map[string]*store.Feature{
			"F-one": cycleFeature("F-one", "solo/one", "solo"),
		},
	}

	require.True(t, Check(m, tree).Ready)
	require.True(t, Check(m, nil).Ready)
}

func TestCheckNamesOnlyUnfinishedAssignmentGaps(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-parent": {ID: "E-parent", Status: "ready"}},
		Tasks: map[string]*store.Task{
			"T-live":      {ID: "T-live", Status: "in_progress", Started: true, Owner: "ghost"},
			"T-done":      {ID: "T-done", Status: "done", Started: true, Owner: "ghost"},
			"T-cancelled": {ID: "T-cancelled", Status: "cancelled", Owner: "ghost"},
		},
		Bugs: map[string]*store.Bug{
			"B-ready": {WorkItem: store.WorkItem{ID: "B-ready", Status: "ready", Owner: "ghost"}},
			"B-done":  {WorkItem: store.WorkItem{ID: "B-done", Status: "done", Owner: "ghost"}},
		},
		Chores: map[string]*store.Chore{
			"CH-proposed": {WorkItem: store.WorkItem{ID: "CH-proposed", Status: "proposed", Owner: "ghost"}},
		},
		Questions: map[string]*store.Question{
			"Q-open":     {ID: "Q-open", Statement: "Still open?", Owner: "ghost", Status: "open"},
			"Q-answered": {ID: "Q-answered", Statement: "Answered", Owner: "ghost", Status: "answered"},
		},
	}
	rep := Check(m, tree)
	require.False(t, rep.Ready)
	require.Len(t, rep.Gaps, 4)
	for i, id := range []string{"T-live", "B-ready", "CH-proposed", "Q-open"} {
		require.Equal(t, ReasonDanglingOwner, rep.Gaps[i].Reason)
		require.Contains(t, rep.Gaps[i].Message, id)
		require.Contains(t, rep.Gaps[i].Message, "`tiller add contributor ghost --name \"<name>\"`", "the remedy names the current registration command")
	}
}

func TestRemovingAncestorCreatorDoesNotLeaveAssignmentGap(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, store.WriteRepositoryFileAtomic(root, filepath.Join(".tiller", "tiller.yml"), []byte(completeManifest)))
	require.NoError(t, store.WriteRepositoryFileAtomic(root, filepath.Join(".tiller", "design", "00-index.md"), []byte("# index\n")))
	load := func() *store.Tree {
		tree, err := store.Load(root)
		require.NoError(t, err)
		return tree
	}
	_, err := mutate.AddContributor(load(), mutate.Deps{Contributor: "jc"}, store.Contributor{ID: "sam", Name: "Sam"})
	require.NoError(t, err)
	_, err = mutate.AddEpic(load(), mutate.Deps{Contributor: "sam"}, mutate.EpicInput{Key: "widgets", Title: "Widgets"})
	require.NoError(t, err)
	_, err = mutate.RemoveContributor(load(), mutate.Deps{Contributor: "jc"}, "sam")
	require.NoError(t, err)
	tree := load()
	rep := Check(tree.Manifest, tree)
	require.True(t, rep.Ready)
	require.Empty(t, rep.Gaps)
}
