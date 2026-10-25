package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

var locateNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func locateEvent(id, typ, contributor string, daysAgo int, targets ...string) events.Event {
	return events.Event{
		ID:          id,
		Type:        typ,
		At:          locateNow.Add(-time.Duration(daysAgo) * 24 * time.Hour),
		Contributor: contributor,
		Targets:     targets,
	}
}

// locateTree carries two epics (epic-0, epic-1), each with one ready feature
// and one task: F-aaaaa0 under epic-0, F-aaaaa1 under epic-1.
func locateTree(t *testing.T) (*store.Tree, *store.Task, *store.Task) {
	t.Helper()
	tree := treeWithDraftEpics(t, 2)
	_, t0 := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	_, t1 := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/work", "epic-1")
	return tree, t0, t1
}

func TestLocateCurrentFeatureIsTheMostRecentCountedEvent(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 3, t0.ID),
		locateEvent("b", "approve.feature", "jc", 1, "F-aaaaa1"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa1", got.CurrentFeature)
	require.Equal(t, "epic-1", got.CurrentEpic)
	require.Equal(t, []views.Touch{
		{FeatureID: "F-aaaaa1", EpicKey: "epic-1", At: locateNow.Add(-24 * time.Hour)},
		{FeatureID: "F-aaaaa0", EpicKey: "epic-0", At: locateNow.Add(-72 * time.Hour)},
	}, got.Touched)
}

func TestLocateKeepsOnlyTheNewestTouchPerFeature(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("old", "claim.task", "jc", 5, t0.ID),
		locateEvent("new", "start.task", "jc", 2, t0.ID),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, []views.Touch{{FeatureID: "F-aaaaa0", EpicKey: "epic-0", At: locateNow.Add(-48 * time.Hour)}}, got.Touched)
}

func TestLocateIgnoresOtherContributorsAndUncountedTypes(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "start.task", "other", 1, t1.ID),
		locateEvent("c", "sync.task", "jc", 1, t1.ID),
		locateEvent("d", "migrate.schema", "jc", 0, t1.ID),
		locateEvent("e", "contributor.add", "jc", 0, t1.ID),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Len(t, got.Touched, 1)
}

func TestLocateCountsEveryListedVerb(t *testing.T) {
	tree, t0, _ := locateTree(t)
	for _, typ := range []string{
		"claim.task", "start.task", "submit.task", "check.feature", "spec.feature",
		"approve.feature", "answer.question", "attest.task", "link.design",
		"link.surface.feature", "link.attach.task", "reassign.task", "release.task",
	} {
		got := views.LocateOf(tree, []events.Event{locateEvent("a", typ, "jc", 1, t0.ID)}, "jc", locateNow)
		require.Equal(t, "F-aaaaa0", got.CurrentFeature, typ)
	}
}

func TestLocateAmendmentEventsDoNotLocalize(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "amendment_opened", "jc", 1, "A-aaaaa0"),
		locateEvent("c", "amendment_staged", "jc", 0, "A-aaaaa0"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Len(t, got.Touched, 1)
}

func TestLocateMapsQuestionEventsThroughBlocks(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	holdClaimableNode(tree, "Q-aaaaa0", "gone-node")
	tree.Questions["Q-aaaaa0"].Blocks = []string{"gone-node", t1.ID}
	holdClaimableNode(tree, "Q-aaaaa1", t0.ID)
	tree.Questions["Q-aaaaa1"].Blocks = nil
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "answer.question", "jc", 2, "Q-aaaaa0"),
		locateEvent("c", "answer.question", "jc", 1, "Q-aaaaa1"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa1", got.CurrentFeature)
	require.Equal(t, "epic-1", got.CurrentEpic)
}

func TestLocateEpicEventSetsEpicOnly(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 4, t0.ID),
		locateEvent("b", "approve.epic", "jc", 1, "E-aaaaa1"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Equal(t, "epic-0", got.CurrentEpic)
	require.Equal(t, []views.Touch{
		{EpicKey: "epic-1", At: locateNow.Add(-24 * time.Hour)},
		{FeatureID: "F-aaaaa0", EpicKey: "epic-0", At: locateNow.Add(-96 * time.Hour)},
	}, got.Touched)
}

func TestLocateEpicOnlyReaderCurrentEpicIsTheNewestEpicTouch(t *testing.T) {
	tree, _, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "approve.epic", "jc", 3, "E-aaaaa0"),
		locateEvent("b", "approve.epic", "jc", 1, "E-aaaaa1"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Empty(t, got.CurrentFeature)
	require.Equal(t, "epic-1", got.CurrentEpic)
}

func TestLocateSkipsUnresolvableTargets(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "add.feature", "jc", 3, "F-removed"),
		locateEvent("c", "link.design", "jc", 2, ".tiller/design/x.md"),
		locateEvent("d", "approve.amendment", "jc", 1, "A-aaaaa0"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Len(t, got.Touched, 1)
}

func TestLocateDropsEventsOutsideTheWindow(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 13, t0.ID),
		locateEvent("b", "claim.task", "jc", 15, t1.ID),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Len(t, got.Touched, 1)
}

func TestLocateSkipsEventsWithMoreThanThreeTargets(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "claim.task", "jc", 1, t1.ID, t1.ID, t1.ID, t1.ID),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)

	three := []events.Event{locateEvent("c", "claim.task", "jc", 1, t1.ID, t1.ID, t1.ID)}
	require.Equal(t, "F-aaaaa1", views.LocateOf(tree, three, "jc", locateNow).CurrentFeature)
}

func TestLocateOrdersUnsortedEventsNewestFirst(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	evs := []events.Event{
		locateEvent("new", "claim.task", "jc", 1, t1.ID),
		locateEvent("old", "claim.task", "jc", 4, t0.ID),
	}
	require.Equal(t, "F-aaaaa1", views.LocateOf(tree, evs, "jc", locateNow).CurrentFeature)
	require.Equal(t, "F-aaaaa1", views.LocateOf(tree, []events.Event{evs[1], evs[0]}, "jc", locateNow).CurrentFeature)
}

func TestLocateWithNoReaderIsEmpty(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{locateEvent("a", "claim.task", "", 1, t0.ID)}
	require.Equal(t, views.Locality{}, views.LocateOf(tree, evs, "", locateNow))
}

// A decompose-like burst of adds says where work was laid down, not where the
// reader is working: an added feature touches its epic, an added epic touches
// nothing, and neither sets the current feature or epic (fix round ruling 1).
func TestLocateAddBurstSetsNoCurrentFeature(t *testing.T) {
	tree, _, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "add.epic", "jc", 1, "E-aaaaa0"),
		locateEvent("b", "add.epic", "jc", 1, "E-aaaaa1"),
		locateEvent("c", "add.feature", "jc", 0, "F-aaaaa0"),
		locateEvent("d", "add.feature", "jc", 0, "F-aaaaa1"),
	}
	evs[2].At = evs[2].At.Add(-time.Minute)
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Empty(t, got.CurrentFeature)
	require.Empty(t, got.CurrentEpic)
	require.Equal(t, []views.Touch{
		{EpicKey: "epic-1", At: locateNow},
		{EpicKey: "epic-0", At: locateNow.Add(-time.Minute)},
	}, got.Touched)
}

// An added task, or an added question, touches the place it was added for
// recency without making that feature current.
func TestLocateAddTaskTouchesItsFeatureWithoutSettingIt(t *testing.T) {
	tree, t0, t1 := locateTree(t)
	holdClaimableNode(tree, "Q-aaaaa0", t1.ID)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 5, t0.ID),
		locateEvent("b", "add.task", "jc", 1, t1.ID),
		locateEvent("c", "add.question", "jc", 0, "Q-aaaaa0"),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Equal(t, "epic-0", got.CurrentEpic)
	require.Equal(t, []views.Touch{
		{FeatureID: "F-aaaaa1", EpicKey: "epic-1", At: locateNow},
		{FeatureID: "F-aaaaa0", EpicKey: "epic-0", At: locateNow.Add(-120 * time.Hour)},
	}, got.Touched)
}

// A newer add on a feature never hides an older deliberate act there: the
// act still makes the feature current, and the touch keeps the newer time.
func TestLocateAddDoesNotHideAnOlderActOnTheSameFeature(t *testing.T) {
	tree, t0, _ := locateTree(t)
	evs := []events.Event{
		locateEvent("a", "claim.task", "jc", 3, t0.ID),
		locateEvent("b", "add.task", "jc", 1, t0.ID),
	}
	got := views.LocateOf(tree, evs, "jc", locateNow)
	require.Equal(t, "F-aaaaa0", got.CurrentFeature)
	require.Equal(t, "epic-0", got.CurrentEpic)
	require.Equal(t, []views.Touch{{FeatureID: "F-aaaaa0", EpicKey: "epic-0", At: locateNow.Add(-24 * time.Hour)}}, got.Touched)
}

// An added task since removed from the tree still places its feature through
// the event's recorded parent.
func TestLocateAddTaskFallsBackToTheRecordedFeature(t *testing.T) {
	tree, _, _ := locateTree(t)
	ev := locateEvent("a", "add.task", "jc", 1, "T-gone00")
	ev.After = map[string]any{"feature": "F-aaaaa1"}
	got := views.LocateOf(tree, []events.Event{ev}, "jc", locateNow)
	require.Empty(t, got.CurrentFeature)
	require.Equal(t, []views.Touch{{FeatureID: "F-aaaaa1", EpicKey: "epic-1", At: locateNow.Add(-24 * time.Hour)}}, got.Touched)
}
