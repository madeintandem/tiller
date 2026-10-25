package views_test

// These tests pin the correction block (issue #94): the lighter readout a
// correction inside a gate renders through `tiller readout <seam> --since
// <sha>`. Changed is derived from the event stream alone, one line per event
// in stream order, so the block reports what Tiller recorded rather than
// what the agent says it did.

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// correctionTree lays two draft epics, three draft features (one carrying two
// edges, one of them cross-epic), and one open question, the shape a decompose
// correction loop works over.
func correctionTree(t *testing.T) *store.Tree {
	t.Helper()
	foundation, technician := "foundation", "technician-application"
	repoKey := "foundation/repo-and-local-dev"
	adminKey := "technician-application/admin-landing-and-search"
	profileKey := "technician-application/technician-profile"
	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &foundation, Title: "Foundation", Status: "draft"},
			"E-bbbbbb": {ID: "E-bbbbbb", Key: &technician, Title: "Technician application", Status: "draft"},
		},
		Features: map[string]*store.Feature{
			"F-111111": {ID: "F-111111", Key: &repoKey, EpicKey: foundation, Title: "Repo and local dev", Status: "draft"},
			"F-222222": {ID: "F-222222", Key: &adminKey, EpicKey: technician, Title: "Admin landing and search", Status: "draft",
				DependsOn: []string{"F-111111", "F-333333"}},
			"F-333333": {ID: "F-333333", Key: &profileKey, EpicKey: technician, Title: "Technician profile", Status: "draft"},
		},
		Questions: map[string]*store.Question{
			"Q-qqqqqq": {ID: "Q-qqqqqq", Title: "Which auth provider?", Statement: "Which auth provider does the admin landing use?", Status: "open", Blocks: []string{"F-111111"}},
		},
	}
}

// eventAt builds one stream event stamped at a fixed instant plus offset
// seconds, so a test can lay events in a chosen chronological order.
func eventAt(offset int, typ string, targets []string, before, after map[string]any) events.Event {
	return events.Event{
		ID:          "EV-" + string(rune('a'+offset)) + "aaaaa",
		Type:        typ,
		At:          time.Date(2026, 8, 30, 12, 0, offset, 0, time.UTC),
		Contributor: "jc",
		Targets:     targets,
		Before:      before,
		After:       after,
	}
}

func correctionOf(t *testing.T, tree *store.Tree, m views.Moment, evs ...events.Event) views.Correction {
	t.Helper()
	return views.CorrectionOfInput(tree, m, views.CorrectionInput{
		Commit: "a244b05", Since: "33fd465", Contributor: "jc", Events: evs,
	})
}

func TestCorrectionHeadlinesSpeakInTheReadoutsVoice(t *testing.T) {
	tree := correctionTree(t)
	for m, want := range map[views.Moment]string{
		views.MomentDecompose: "Decomposition corrected",
		views.MomentSpecify:   "Specs corrected",
		views.MomentOrient:    "Project corrected",
		views.MomentExecute:   "Work corrected",
		views.MomentAmend:     "Amendments corrected",
	} {
		require.Equal(t, want, correctionOf(t, tree, m).Headline, "moment %s", m)
	}
}

// TestCorrectionArtifactFollowsTheFullBlocksRule pins that the committed-at
// clause is computed exactly as the full block computes it: the commit only
// when the tree is clean, and the since sha carried through untouched.
func TestCorrectionArtifactFollowsTheFullBlocksRule(t *testing.T) {
	tree := correctionTree(t)
	clean := views.CorrectionOfInput(tree, views.MomentDecompose, views.CorrectionInput{Commit: "a244b05", Since: "33fd465"})
	require.Equal(t, "a244b05", clean.Artifact)
	require.Equal(t, "33fd465", clean.Since)

	dirty := views.CorrectionOfInput(tree, views.MomentDecompose, views.CorrectionInput{Commit: "a244b05", Dirty: true, Since: "33fd465"})
	require.Empty(t, dirty.Artifact, "a dirty tree names no commit, the same as the full block")
	require.Equal(t, "33fd465", dirty.Since)
}

func TestCorrectionChangedRendersEachEventKindAsOneLine(t *testing.T) {
	tree := correctionTree(t)
	for name, tc := range map[string]struct {
		event events.Event
		want  string
	}{
		"add epic": {
			eventAt(0, "add.epic", []string{"E-aaaaaa"}, nil, map[string]any{"status": "draft", "key": "foundation"}),
			"added foundation (E-aaaaaa)",
		},
		"add feature": {
			eventAt(0, "add.feature", []string{"F-222222"}, nil, map[string]any{"status": "draft", "key": "technician-application/admin-landing-and-search"}),
			"added technician-application/admin-landing-and-search (F-222222)",
		},
		"add feature since removed names the key the event recorded": {
			eventAt(0, "add.feature", []string{"F-999999"}, nil, map[string]any{"status": "draft", "key": "foundation/old-thing"}),
			"added foundation/old-thing (F-999999)",
		},
		"add question": {
			eventAt(0, "add.question", []string{"Q-qqqqqq"}, nil, map[string]any{"title": "Which auth provider?", "status": "open", "blocks": []any{"F-111111"}}),
			`added question Q-qqqqqq ("Which auth provider?"); blocks foundation/repo-and-local-dev (F-111111)`,
		},
		"rename title": {
			eventAt(0, "rename.title.feature", []string{"F-222222"}, map[string]any{"title": "Admin landing"}, map[string]any{"title": "Admin landing and search"}),
			`renamed technician-application/admin-landing-and-search (F-222222) title from "Admin landing" to "Admin landing and search"`,
		},
		"rename key": {
			eventAt(0, "rename.key.feature", []string{"F-222222"}, map[string]any{"key": "technician-application/admin-landing", "epic": "technician-application"}, map[string]any{"key": "technician-application/admin-landing-and-search", "epic": "technician-application"}),
			"renamed technician-application/admin-landing-and-search (F-222222) key from technician-application/admin-landing to technician-application/admin-landing-and-search",
		},
		"remove names the node from the event, not the tree": {
			eventAt(0, "remove.draft.feature", []string{"F-999999"},
				map[string]any{"removed": []any{map[string]any{"noun": "feature", "id": "F-999999", "key": "foundation/old-thing", "title": "Old thing"}}},
				map[string]any{"reason": "duplicate", "removed_ids": []any{"F-999999"}}),
			"removed foundation/old-thing (F-999999)",
		},
		"remove cascade counts what fell with it": {
			eventAt(0, "remove.draft.epic", []string{"E-zzzzzz", "F-888888", "F-777777"},
				map[string]any{"removed": []any{
					map[string]any{"noun": "epic", "id": "E-zzzzzz", "key": "legacy", "title": "Legacy"},
					map[string]any{"noun": "feature", "id": "F-888888", "key": "legacy/one", "title": "One"},
					map[string]any{"noun": "feature", "id": "F-777777", "key": "legacy/two", "title": "Two"},
				}},
				map[string]any{"reason": "out of scope", "removed_ids": []any{"E-zzzzzz", "F-888888", "F-777777"}}),
			"removed legacy (E-zzzzzz) and 2 nodes beneath it",
		},
		"link counts the edges it added and the cross-epic ones": {
			eventAt(0, "link.feature", []string{"F-222222"}, map[string]any{"depends_on": []any{}}, map[string]any{"depends_on": []any{"F-111111", "F-333333"}, "added": []any{"F-111111", "F-333333"}}),
			"linked 2 edges (1 cross-epic) on technician-application/admin-landing-and-search (F-222222)",
		},
		"link with an end the tree cannot resolve drops the cross-epic clause": {
			eventAt(0, "link.feature", []string{"F-222222"}, map[string]any{"depends_on": []any{}}, map[string]any{"depends_on": []any{"F-999999"}, "added": []any{"F-999999"}}),
			"linked 1 edge on technician-application/admin-landing-and-search (F-222222)",
		},
		"link between epics carries no cross-epic clause": {
			eventAt(0, "link.epic", []string{"E-bbbbbb"}, map[string]any{"depends_on": []any{}}, map[string]any{"depends_on": []any{"E-aaaaaa"}, "added": []any{"E-aaaaaa"}}),
			"linked 1 edge on technician-application (E-bbbbbb)",
		},
		"legacy link event without an added list falls through to the generic line": {
			eventAt(0, "link.feature", []string{"F-222222"}, nil, map[string]any{"depends_on": []any{"F-111111"}}),
			"link.feature: technician-application/admin-landing-and-search (F-222222)",
		},
		"unlink of a surface names the removed paths, not edges": {
			eventAt(0, "unlink.surface.feature", []string{"F-222222"}, map[string]any{"surface": []any{"app/legacy", "app/views"}}, map[string]any{"surface": []any{"app/views"}, "removed": []any{"app/legacy"}}),
			"removed surface app/legacy from technician-application/admin-landing-and-search (F-222222)",
		},
		"unlink counts the edges it removed": {
			eventAt(0, "unlink.feature", []string{"F-222222"}, map[string]any{"depends_on": []any{"F-111111", "F-333333"}}, map[string]any{"depends_on": []any{"F-333333"}, "removed": []any{"F-111111"}}),
			"unlinked 1 edge (1 cross-epic) on technician-application/admin-landing-and-search (F-222222)",
		},
		"order features": {
			eventAt(0, "order.features", []string{"E-bbbbbb"}, nil, map[string]any{"order": []any{"F-333333", "F-222222"}}),
			"ordered 2 features under technician-application (E-bbbbbb)",
		},
		"order tasks": {
			eventAt(0, "order.tasks", []string{"F-222222"}, nil, map[string]any{"order": []any{"T-aaaaaa"}}),
			"ordered 1 task under technician-application/admin-landing-and-search (F-222222)",
		},
		"order bugs": {
			eventAt(0, "order.bugs", []string{"B-aaaaaa", "B-bbbbbb"}, nil, map[string]any{"order": []any{"B-aaaaaa", "B-bbbbbb"}}),
			"ordered 2 bugs",
		},
		"anything else renders its type and targets": {
			eventAt(0, "approve.epic", []string{"E-aaaaaa"}, map[string]any{"status": "draft"}, map[string]any{"status": "ready"}),
			"approve.epic: foundation (E-aaaaaa)",
		},
		"anything else with no target renders its type alone": {
			eventAt(0, "migrate.schema", nil, nil, nil),
			"migrate.schema",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := correctionOf(t, tree, views.MomentDecompose, tc.event)
			require.Equal(t, []string{tc.want}, c.Changed)
		})
	}
}

func TestCorrectionChangedIsInStreamOrder(t *testing.T) {
	tree := correctionTree(t)
	later := eventAt(2, "add.feature", []string{"F-222222"}, nil, map[string]any{"key": "technician-application/admin-landing-and-search"})
	earlier := eventAt(1, "add.feature", []string{"F-111111"}, nil, map[string]any{"key": "foundation/repo-and-local-dev"})
	c := correctionOf(t, tree, views.MomentDecompose, later, earlier)
	require.Equal(t, []string{
		"added foundation/repo-and-local-dev (F-111111)",
		"added technician-application/admin-landing-and-search (F-222222)",
	}, c.Changed, "the block reads in the order Tiller recorded the events, whatever order the caller handed them in")
}

func TestCorrectionWithNoEventsHasAnEmptyChangedList(t *testing.T) {
	c := correctionOf(t, correctionTree(t), views.MomentDecompose)
	require.Empty(t, c.Changed)
	require.NotNil(t, c.Changed, "an empty list, not a nil one: the renderer's placeholder line is the only thing that says nothing changed")
}

// TestCorrectionKeepsTheQueueLineAndNextOfTheFullBlock pins that the two
// lines the correction block keeps are the full block's own, produced by the
// same code, so a developer reads the same Next at the same gate whichever
// block closed the reply.
func TestCorrectionKeepsTheQueueLineAndNextOfTheFullBlock(t *testing.T) {
	tree := correctionTree(t)
	full := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{Contributor: "jc"})
	c := correctionOf(t, tree, views.MomentDecompose)
	require.Equal(t, full.Queue, c.Queue)
	require.Equal(t, full.Next, c.Next)
	require.NotEmpty(t, c.Next.Candidates, "the block still closes on a ranked move")
	require.Contains(t, c.Queue, "queue: 0 bugs, 0 chores, 0 tweaks ready")
}
