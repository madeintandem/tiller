package views_test

// These tests pin the recap derivation (ui-surface.md, "The recap"; plan
// 2026-07-05, serve pass 1 lane B): grouping by owning epic and acting stream
// resolved from the first target and the event's owner, the reader-scoped ask
// flags, the claim-churn filter, the single newest-first timeline inside a
// group, the verbatim window and reader echoes, the zero-since emptiness rule,
// and the per-group routine cap that never touches an ask.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// recapTree is an in-memory tree with two owners, two epics, a feature under
// each, two tasks under one feature split across the owners, and an unowned
// question, enough to exercise every grouping path and both sides of the
// reader-scoped ask rule.
func recapTree() *store.Tree {
	formEngine := "form-engine"
	checkout := "checkout"
	schemaEngine := "form-engine/schema-engine"
	cart := "checkout/cart"
	taskA := "form-engine/schema-engine/registry-loader"
	taskB := "form-engine/schema-engine/field-parser"
	taskC := "checkout/cart/line-items"
	tree := &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{
			{ID: "jc", Name: "JC Grubbs"},
			{ID: "mg", Name: "Mera Grubbs"},
		}},
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &formEngine, Title: "Form Engine", Status: "draft"},
			"E-bbbbbb": {ID: "E-bbbbbb", Key: &checkout, Title: "Checkout", Status: "draft"},
		},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &schemaEngine, Status: "specced", EpicKey: "form-engine"},
			"F-bbbbbb": {ID: "F-bbbbbb", Key: &cart, Status: "active", EpicKey: "checkout"},
		},
		// A PR always represents a task, so the node that reaches in_review under
		// checkout/cart is T-cccccc, not the feature.
		Tasks: map[string]*store.Task{
			"T-aaaaaa": {ID: "T-aaaaaa", Key: &taskA, Status: "in_progress", FeatureID: "F-aaaaaa", Owner: "jc", Started: true},
			"T-bbbbbb": {ID: "T-bbbbbb", Key: &taskB, Status: "in_progress", FeatureID: "F-aaaaaa", Owner: "mg", Started: true},
			"T-cccccc": {ID: "T-cccccc", Key: &taskC, Status: "in_review", FeatureID: "F-bbbbbb", Owner: "mg", Started: true},
		},
		Questions: map[string]*store.Question{
			"Q-aaaaaa": {ID: "Q-aaaaaa", Title: "Canonical schema", Statement: "Which schema is canonical and how should every implementation migrate without losing compatibility?", Status: "open"},
		},
	}
	tree.ProjectContributors()
	return tree
}

func recapAt(hour, min int) time.Time {
	return time.Date(2026, 7, 1, hour, min, 0, 0, time.UTC)
}

// recapEvents is the stream the golden digests: deliberately unsorted, with
// one event before the horizon, a claim.feature and a release.feature on the
// same feature (which now render like any other event, task 1's review,
// carried here), and both owners acting inside the form-engine epic so the
// (epic, stream) grouping has something to split.
func recapEvents() []events.Event {
	return []events.Event{
		{ID: "EV-000006", Type: "submit.task", At: recapAt(13, 0), Contributor: "mg",
			Command: "submit task T-cccccc", Targets: []string{"T-cccccc"},
			After: map[string]any{"status": "in_review"}},
		{ID: "EV-000001", Type: "add.epic", At: recapAt(10, 0), Contributor: "jc",
			Command: "add epic form-engine", Targets: []string{"E-aaaaaa"},
			After: map[string]any{"status": "draft"}},
		{ID: "EV-000000", Type: "add.epic", At: time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC), Contributor: "mg",
			Command: "add epic checkout", Targets: []string{"E-bbbbbb"},
			After: map[string]any{"status": "draft"}},
		{ID: "EV-000004", Type: "claim.feature", At: recapAt(12, 30), Contributor: "mg",
			Command: "claim feature F-bbbbbb", Targets: []string{"F-bbbbbb"}},
		{ID: "EV-000003", Type: "start.task", At: recapAt(12, 0), Contributor: "jc",
			Command: "start task T-aaaaaa", Targets: []string{"T-aaaaaa"},
			After: map[string]any{"status": "in_progress"}},
		{ID: "EV-000005", Type: "release.feature", At: recapAt(12, 31), Contributor: "mg",
			Command: "release feature F-bbbbbb", Targets: []string{"F-bbbbbb"}},
		{ID: "EV-000002", Type: "spec.feature", At: recapAt(11, 0), Contributor: "jc",
			Command: "spec feature F-aaaaaa", Targets: []string{"F-aaaaaa"},
			After: map[string]any{"status": "specced"}},
		{ID: "EV-000008", Type: "start.task", At: recapAt(12, 45), Contributor: "mg",
			Command: "start task T-bbbbbb", Targets: []string{"T-bbbbbb"},
			After: map[string]any{"status": "in_progress"}},
		{ID: "EV-000007", Type: "add.question", At: recapAt(14, 0), Contributor: "mg",
			Command: "add question", Targets: []string{"Q-aaaaaa"},
			After: map[string]any{"status": "open"}},
	}
}

// TestRecapOfPinsGroupingAsksAndOrder is the payload golden for reader jc:
// the pre-horizon event is gone, a task climbs to its feature's epic, the
// form-engine epic splits into a jc lane and an mg lane, groups sort by epic
// key then stream with the project-level group last, each group head carries
// its epic's title as well as its key, and inside a group the entries are
// one newest-first timeline with jc's 11:00 ask sitting in its real place
// behind the 12:00 task start. mg's task arriving at in_review is not jc's
// ask; mg's new question asks the whole team, so it stays flagged. mg's
// claim and release of F-bbbbbb render as routine entries in their own
// place in the checkout/mg timeline, the carried fix (task 1's review):
// claim.feature and release.feature are real events now, not churn to drop.
func TestRecapOfPinsGroupingAsksAndOrder(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	recap := views.RecapOf(recapTree(), recapEvents(), since, "visit", "jc")
	require.Equal(t, 8, recap.Total)
	require.Len(t, recap.Groups, 4)
	type grouping struct {
		epic, contributor string
	}
	var groups []grouping
	for _, group := range recap.Groups {
		epic := ""
		if group.Epic != nil {
			epic = group.Epic.Key
		}
		groups = append(groups, grouping{epic, group.Stream})
	}
	require.Equal(t, []grouping{
		{"checkout", "mg"}, {"form-engine", "jc"}, {"form-engine", "mg"}, {"", "mg"},
	}, groups)
	var types []string
	for _, entry := range recap.Groups[0].Entries {
		types = append(types, entry.Type)
		require.Equal(t, "mg", entry.Contributor)
		require.False(t, entry.Ask)
	}
	require.Equal(t, []string{"submit.task", "release.feature", "claim.feature"}, types)
	require.Equal(t, "mg", recap.Groups[0].Entries[0].Targets[0].Owner)
	require.Equal(t, "start.task", recap.Groups[1].Entries[0].Type)
	require.False(t, recap.Groups[1].Entries[0].Ask)
	require.Equal(t, "spec.feature", recap.Groups[1].Entries[1].Type)
	require.True(t, recap.Groups[1].Entries[1].Ask)
	require.Equal(t, "add.question", recap.Groups[3].Entries[0].Type)
	require.True(t, recap.Groups[3].Entries[0].Ask)
}

// TestRecapRendersAllThreeOwnerLifecycleEvents pins the carried fix (task 1's
// review, Important): claim.feature, release.feature, and reassign.feature
// each render as an ordinary entry, none of them dropped as "claim churn"
// (the filter this file used to carry, written when ui-surface.md:81 called
// the concept aspirational; the owner lifecycle build made all three real).
// One of each event type is enough to pin that no prefix match still
// swallows any of them.
func TestRecapRendersAllThreeOwnerLifecycleEvents(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := []events.Event{
		{ID: "EV-000050", Type: "claim.feature", At: recapAt(9, 0), Contributor: "jc",
			Command: "claim feature F-aaaaaa", Targets: []string{"F-aaaaaa"}},
		{ID: "EV-000051", Type: "release.feature", At: recapAt(10, 0), Contributor: "jc",
			Command: "release feature F-aaaaaa", Targets: []string{"F-aaaaaa"}},
		{ID: "EV-000052", Type: "reassign.feature", At: recapAt(11, 0), Contributor: "jc",
			Command: "reassign feature F-aaaaaa --to mg --reason \"handoff\"", Targets: []string{"F-aaaaaa"}},
	}

	r := views.RecapOf(recapTree(), evs, since, "visit", "jc")
	require.Len(t, r.Groups, 1)

	types := make([]string, 0, 3)
	for _, e := range r.Groups[0].Entries {
		types = append(types, e.Type)
	}
	require.ElementsMatch(t, []string{"claim.feature", "release.feature", "reassign.feature"}, types,
		"all three owner-lifecycle events render; none is filtered as claim churn")
}

// TestRecapGroupEntriesAreOneNewestFirstTimeline is the regression for the bug
// the full-width recap view exposed. A group used to emit its asks as a
// partition ahead of the rest, so one feature's own lifecycle came back
// submitted, specced, started, added: two of those rows out of time order, on
// a view that prints a timestamp per row and collapses ADJACENT same-type runs.
// The digest is one timeline now, so an ask sits where it happened.
func TestRecapGroupEntriesAreOneNewestFirstTimeline(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	// One feature walked through its gates and then its task to review: spec and
	// submit are jc's asks (they land the node on a human gate), start and add
	// are routine.
	evs := []events.Event{
		{ID: "EV-000041", Type: "add.feature", At: recapAt(9, 0), Contributor: "jc",
			Command: "add feature form-engine/schema-engine", Targets: []string{"F-aaaaaa"},
			After: map[string]any{"status": "draft"}},
		{ID: "EV-000042", Type: "spec.feature", At: recapAt(10, 0), Contributor: "jc",
			Command: "spec feature F-aaaaaa", Targets: []string{"F-aaaaaa"},
			After: map[string]any{"status": "specced"}},
		{ID: "EV-000043", Type: "start.feature", At: recapAt(11, 0), Contributor: "jc",
			Command: "start feature F-aaaaaa", Targets: []string{"F-aaaaaa"},
			After: map[string]any{"status": "active"}},
		{ID: "EV-000044", Type: "submit.task", At: recapAt(12, 0), Contributor: "jc",
			Command: "submit task T-aaaaaa", Targets: []string{"T-aaaaaa"},
			After: map[string]any{"status": "in_review"}},
	}

	got := views.RecapOf(recapTree(), evs, since, "visit", "jc")
	require.Len(t, got.Groups, 1)
	g := got.Groups[0]

	types := make([]string, 0, len(g.Entries))
	stamps := make([]string, 0, len(g.Entries))
	for _, e := range g.Entries {
		types = append(types, e.Type)
		stamps = append(stamps, e.At)
	}
	require.Equal(t, []string{"submit.task", "start.feature", "spec.feature", "add.feature"}, types)
	require.Equal(t, []string{
		"2026-07-01T12:00:00Z", "2026-07-01T11:00:00Z",
		"2026-07-01T10:00:00Z", "2026-07-01T09:00:00Z",
	}, stamps)
	// The flags still ride, so a consumer picks its asks out by filtering
	// rather than by trusting a position.
	require.Equal(t, []bool{true, false, true, false},
		[]bool{g.Entries[0].Ask, g.Entries[1].Ask, g.Entries[2].Ask, g.Entries[3].Ask})
}

// Ancestor gate arrivals ask every reader, not just a past contributor.
func TestRecapAncestorAsksAreShared(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := []events.Event{
		{ID: "EV-000020", Type: "spec.feature", At: recapAt(11, 0), Contributor: "mg",
			Command: "spec feature F-bbbbbb", Targets: []string{"F-bbbbbb"},
			After: map[string]any{"status": "specced"}},
	}
	askFor := func(reader string) bool {
		r := views.RecapOf(recapTree(), evs, since, "visit", reader)
		require.Len(t, r.Groups, 1)
		require.Len(t, r.Groups[0].Entries, 1)
		require.Equal(t, reader, r.Reader)
		return r.Groups[0].Entries[0].Ask
	}
	require.True(t, askFor("unrelated-reviewer"))
	require.True(t, askFor("mg"))
	require.True(t, askFor(""))
}

// TestRecapAskIsAnArrivalNotAStatus pins the crossing rule. commitTransition
// writes the node's status on every event it records, so `tiller attest` and
// `tiller eval` (mutate's saveEvalTarget) land with before and after both
// in_review: settling evidence leaves the node standing exactly where it was.
// Reading the after-status alone counted those as fresh gate arrivals, and on a
// real tree the reader's own four attestations plus one eval run made the recap
// page's headline read "5 asked you" about five things that reader had just
// done. An arrival needs a before-status that DIFFERS, absent (a node minted
// straight onto a ledger at proposed) or genuinely other.
func TestRecapAskIsAnArrivalNotAStatus(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	askOf := func(ev events.Event) bool {
		r := views.RecapOf(recapTree(), []events.Event{ev}, since, "visit", "mg")
		require.Len(t, r.Groups, 1)
		require.Len(t, r.Groups[0].Entries, 1)
		return r.Groups[0].Entries[0].Ask
	}
	standingStill := func(typ string) events.Event {
		return events.Event{ID: "EV-000030", Type: typ, At: recapAt(11, 0), Contributor: "mg",
			Command: typ + " feature F-bbbbbb", Targets: []string{"F-bbbbbb"},
			Before: map[string]any{"status": "in_review"},
			After:  map[string]any{"status": "in_review"}}
	}
	require.False(t, askOf(standingStill("attest.feature")), "an attestation on a gate already reached asks nobody")
	require.False(t, askOf(standingStill("eval.feature")), "an eval run that moves nothing asks nobody")

	// The real crossing onto that same gate still asks, so the fix narrows the
	// rule rather than deleting it.
	require.True(t, askOf(events.Event{ID: "EV-000031", Type: "submit.task", At: recapAt(11, 0), Contributor: "mg",
		Command: "submit task T-cccccc", Targets: []string{"T-cccccc"},
		Before: map[string]any{"status": "check"},
		After:  map[string]any{"status": "in_review"}}))

	// And so does an arrival with NO before-status: add.task mints a task onto
	// its feature's ledger at proposed, which is a human gate the node was
	// never anywhere else for (internal/core/mutate/add.go writes After only).
	// T-cccccc hangs under the active checkout/cart, so it is the mid-flight
	// addition the human approves on its own.
	require.True(t, askOf(events.Event{ID: "EV-000032", Type: "add.task", At: recapAt(11, 0), Contributor: "mg",
		Command: "add task F-bbbbbb", Targets: []string{"T-cccccc"},
		After: map[string]any{"status": "proposed", "feature": "F-bbbbbb"}}))

	// The same arrival under a feature still short of its plan gate is not its
	// own ask: T-bbbbbb hangs under the specced form-engine/schema-engine, whose
	// approve flips the initial set in one act, and that feature's own specced
	// arrival is the ask (cli-surface.md, "Specifying a feature"). Counting both
	// would report two asks for one decision.
	require.False(t, askOf(events.Event{ID: "EV-000033", Type: "add.task", At: recapAt(11, 0), Contributor: "mg",
		Command: "add task F-aaaaaa", Targets: []string{"T-bbbbbb"},
		After: map[string]any{"status": "proposed", "feature": "F-aaaaaa"}}))
}

func TestRecapAddQuestionAskRequiresOpenStatus(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	askOf := func(status string) bool {
		r := views.RecapOf(recapTree(), []events.Event{{
			ID:          "EV-000034",
			Type:        "add.question",
			At:          recapAt(11, 0),
			Contributor: "mg",
			Command:     "add question",
			Targets:     []string{"Q-aaaaaa"},
			After:       map[string]any{"status": status},
		}}, since, "visit", "mg")
		require.Len(t, r.Groups, 1)
		require.Len(t, r.Groups[0].Entries, 1)
		return r.Groups[0].Entries[0].Ask
	}

	require.True(t, askOf(""), "a legacy blank status is semantically open and remains a team ask")
	require.True(t, askOf("open"), "a fresh or open-duplicate question event remains a team ask")
	require.False(t, askOf("answered"), "an answered title backfill is metadata repair, not a new ask")
}

// TestRecapZeroSinceIsEmpty pins the first-visit rule: no horizon means an
// empty recap, never everything, with the reader still echoed.
func TestRecapZeroSinceIsEmpty(t *testing.T) {
	got, err := json.Marshal(views.RecapOf(recapTree(), recapEvents(), time.Time{}, "visit", "jc"))
	require.NoError(t, err)
	require.JSONEq(t, `{"since":"","window":"visit","reader":"jc","total":0,"groups":[]}`, string(got))
}

// TestRecapUnknownTargetsAndNoTargets pins the degraded shapes for an
// identity-less reader: an id the tree no longer knows keeps its bare id as
// the reference (no noun, no owner) and lands in the project-level group, and
// an event with no targets renders an empty targets array in the same group.
func TestRecapUnknownTargetsAndNoTargets(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := []events.Event{
		{ID: "EV-000010", Type: "cancel.feature", At: recapAt(15, 0), Contributor: "jc",
			Command: "cancel feature F-zzzzzz", Targets: []string{"F-zzzzzz"},
			Before: map[string]any{"status": "draft"}, After: map[string]any{"status": "cancelled"}},
		{ID: "EV-000011", Type: "sync", At: recapAt(16, 0), Contributor: "jc", Command: "sync"},
	}
	recap := views.RecapOf(recapTree(), evs, since, "visit", "")
	require.Len(t, recap.Groups, 1)
	group := recap.Groups[0]
	require.Nil(t, group.Epic)
	require.Equal(t, "jc", group.Stream)
	require.Len(t, group.Entries, 2)
	require.Equal(t, "sync", group.Entries[0].Type)
	require.Empty(t, group.Entries[0].Targets)
	require.Equal(t, "jc", group.Entries[0].Contributor)
	require.Equal(t, "cancel.feature", group.Entries[1].Type)
	require.Equal(t, "jc", group.Entries[1].Contributor)
	require.Len(t, group.Entries[1].Targets, 1)
	unknown := group.Entries[1].Targets[0]
	require.Equal(t, "F-zzzzzz", unknown.ID)
	require.Equal(t, unknown.ID, unknown.Ref)
	require.Empty(t, unknown.Noun)
	require.Empty(t, unknown.Owner)
}

// recapCap mirrors views.recapRoutineCap, which is unexported. The two tests
// below are about the cap's BEHAVIOR at its edge, so they have to flood past
// whatever the number is; keeping the mirror in one place makes the next raise
// a one-line edit.
const recapCap = 200

// recapFlood is n routine events in the form-engine / jc cell, one a minute
// from 08:00 so the order is unambiguous and every one of them is plain
// activity rather than an ask.
func recapFlood(n int) []events.Event {
	out := make([]events.Event, 0, n)
	for i := range n {
		out = append(out, events.Event{
			ID: fmt.Sprintf("EV-1%05d", i), Type: "start.task", At: recapAt(8, i), Contributor: "jc",
			Command: "start task T-aaaaaa", Targets: []string{"T-aaaaaa"},
			After: map[string]any{"status": "in_progress"},
		})
	}
	return out
}

// TestRecapCapsRoutineEntriesAndSaysHowMany pins the per-group cap: a cell with
// more routine entries than the cap serves the cap's worth, and reports the
// drop count so nothing is hidden silently. Two properties the chronological
// merge must not have cost: the entries it drops are the OLDEST routine ones,
// and Total stays the true pre-cap number.
func TestRecapCapsRoutineEntriesAndSaysHowMany(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := recapFlood(recapCap + 15)

	got := views.RecapOf(recapTree(), evs, since, "visit", "jc")
	require.Len(t, got.Groups, 1)
	g := got.Groups[0]

	require.Equal(t, recapCap+15, got.Total, "Total is what the window held, not what the cap served")
	require.Equal(t, 15, g.Omitted, "the 15 dropped routine entries are stated, not swallowed")
	require.Len(t, g.Entries, recapCap)
	require.Equal(t, len(g.Entries)+g.Omitted, recapCap+15,
		"served plus omitted is the cell's real routine count")

	// The flood runs 08:00 upward, so newest-first means the last event first
	// and the cap bites at the bottom: the 15 it refused are 08:00 to 08:14.
	require.Equal(t, recapAt(8, recapCap+14).Format(time.RFC3339), g.Entries[0].At)
	require.Equal(t, recapAt(8, 15).Format(time.RFC3339), g.Entries[len(g.Entries)-1].At,
		"the cap drops the oldest routine entries, never the newest")
	for i := 1; i < len(g.Entries); i++ {
		require.Less(t, g.Entries[i].At, g.Entries[i-1].At, "one newest-first timeline")
	}
}

// TestRecapCapNeverDropsAnAskEvenWhenItIsTheOldestThing is the safety property
// the old asks-first partition existed to guarantee, restated for the single
// timeline that replaced it. The asks here are the OLDEST events in the cell,
// so a cap that simply truncated the merged list at N would drop every one of
// them. Counting only routine entries against the cap is what keeps them.
func TestRecapCapNeverDropsAnAskEvenWhenItIsTheOldestThing(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := recapFlood(recapCap + 15)
	// Three gate arrivals on jc's own feature at 07:00, before the flood, so
	// they sit at the very bottom of the newest-first timeline.
	for i := range 3 {
		evs = append(evs, events.Event{
			ID: fmt.Sprintf("EV-2%05d", i), Type: "spec.feature", At: recapAt(7, i), Contributor: "jc",
			Command: "spec feature F-aaaaaa", Targets: []string{"F-aaaaaa"},
			After: map[string]any{"status": "specced"},
		})
	}

	got := views.RecapOf(recapTree(), evs, since, "visit", "jc")
	require.Len(t, got.Groups, 1)
	g := got.Groups[0]

	require.Equal(t, recapCap+18, got.Total)
	require.Equal(t, 15, g.Omitted, "only the routine entries were capped")
	require.Len(t, g.Entries, recapCap+3, "the cap's worth of routine entries plus every ask")

	asks := 0
	for _, e := range g.Entries {
		if e.Ask {
			asks++
		}
	}
	require.Equal(t, 3, asks, "every ask survives; none is dropped to make room")
	require.True(t, g.Entries[len(g.Entries)-1].Ask,
		"and it survives in its real place, at the bottom of the timeline")
}

// TestRecapAsksSurviveWhenTheirGroupSortsLast is the second half of the same
// guarantee, at group scope. An add.question resolves to no owning epic, and
// the epic-less group sorts LAST, so in a decomposition-phase project every ask
// sits behind every routine entry in the payload. A cap that kept the first N
// entries across the digest would drop 100% of the asks. This asserts the
// opposite: the flood is capped, and all 20 asks come back whole in the group
// that sorts last.
func TestRecapAsksSurviveWhenTheirGroupSortsLast(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evs := recapFlood(recapCap + 15)
	for i := range 20 {
		evs = append(evs, events.Event{
			ID: fmt.Sprintf("EV-3%05d", i), Type: "add.question", At: recapAt(10, i), Contributor: "jc",
			Command: "add question", Targets: []string{"Q-aaaaaa"},
			After: map[string]any{"status": "open"},
		})
	}

	got := views.RecapOf(recapTree(), evs, since, "visit", "jc")
	require.Equal(t, recapCap+35, got.Total)
	require.Len(t, got.Groups, 2)

	flooded := got.Groups[0]
	require.NotNil(t, flooded.Epic, "the epic group sorts first")
	require.Len(t, flooded.Entries, recapCap)
	require.Equal(t, 15, flooded.Omitted)

	last := got.Groups[1]
	require.Nil(t, last.Epic, "the ask-bearing group is the one that sorts last")
	require.Len(t, last.Entries, 20, "every ask survives; none is dropped to make room")
	require.Equal(t, 0, last.Omitted)
	for _, e := range last.Entries {
		require.True(t, e.Ask)
	}
}

// TestRecapTargetsCarryTheirHandle is the regression for the recap's own worst
// row. The console leads a row with a name and keeps the id off it, and the
// key is that name for the nouns that have one. A question has none, so the
// hoisted asks (which on a decomposition-phase tree are almost all
// add.question) had nothing but the bare id to render: twenty rows reading
// "add.question Q-74f7tk" at the top of the rail, differing in six characters,
// while the same questions read as sentences in the decision card beside them.
// The statement rides on the target now, the way a title rides on every other
// row payload.
func TestRecapTargetsCarryTheirHandle(t *testing.T) {
	tree := recapTree()
	tree.Tasks["T-aaaaaa"].Title = "Wire the schema registry loader"
	feat := "Schema engine"
	tree.Features["F-aaaaaa"].Title = feat

	r := views.RecapOf(tree, recapEvents(), recapAt(0, 0), "visit", "jc")

	handles := map[string]views.TargetRef{}
	for _, g := range r.Groups {
		for _, e := range g.Entries {
			for _, tr := range e.Targets {
				handles[tr.ID] = tr
			}
		}
	}
	require.Equal(t, "Canonical schema", handles["Q-aaaaaa"].Title)
	require.Equal(t, "", handles["Q-aaaaaa"].Key, "a question has no key to lead with")
	require.Equal(t, "Wire the schema registry loader", handles["T-aaaaaa"].Title)
	require.Equal(t, "Schema engine", handles["F-aaaaaa"].Title)
	require.Equal(t, "form-engine/schema-engine", handles["F-aaaaaa"].Key,
		"a keyed noun still leads with its key; the title is the fallback")
}

// TestRecapResolvesShelfAndAmendmentTargets drives real mutation-authored add
// events for every shelf noun and an amendment through the recap: each target
// carries its noun, id, key, and title, so Home and History can name and link
// it. A shelf review arrival stays owner-aware, and an id the tree no longer
// holds still renders as the bare id with no noun.
func TestRecapResolvesShelfAndAmendmentTargets(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(queueManifest), 0o644))
	// Amendments record the project HEAD, so the fixture is a real repository.
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=Tiller Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	deps := queueDeps(queueMint())
	for _, item := range []struct{ noun, key, title string }{
		{store.NounBug, "alpha", "Alpha bug"},
		{store.NounChore, "bump", "Bump deps"},
		{store.NounTweak, "label", "Save label"},
	} {
		_, err := mutate.AddWorkItem(queueLoad(t, root), deps, mutate.WorkItemInput{Noun: item.noun, Key: item.key, Title: item.title})
		require.NoError(t, err)
	}
	_, err := mutate.AddAmendment(queueLoad(t, root), deps, mutate.AmendmentInput{Key: "reshape", Title: "Reshape checkout"})
	require.NoError(t, err)

	tree := queueLoad(t, root)
	evs, err := events.List(tree)
	require.NoError(t, err)
	since := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)

	targets := map[string]views.TargetRef{}
	for _, g := range views.RecapOf(tree, evs, since, "visit", "jc").Groups {
		for _, e := range g.Entries {
			for _, tr := range e.Targets {
				targets[tr.ID] = tr
			}
		}
	}
	want := map[string][3]string{
		"B-aaaaaa":  {store.NounBug, "bugs/alpha", "Alpha bug"},
		"CH-aaaaaa": {store.NounChore, "chores/bump", "Bump deps"},
		"TW-aaaaaa": {store.NounTweak, "tweaks/label", "Save label"},
		"A-aaaaaa":  {store.NounAmendment, "reshape", "Reshape checkout"},
	}
	require.Len(t, targets, len(want))
	for id, w := range want {
		got := targets[id]
		require.Equal(t, w[0], got.Noun, id)
		require.Equal(t, w[1], got.Key, id)
		require.Equal(t, w[2], got.Title, id)
		require.Contains(t, got.Ref, id)
	}

	// An owned shelf node arriving at review asks its owner, not another reader.
	tree.Bugs["B-aaaaaa"].Owner = "jc"
	review := []events.Event{{ID: "EV-review", Type: "submit.bug", At: since.Add(3 * time.Hour), Contributor: "jc",
		Command: "submit bug B-aaaaaa", Targets: []string{"B-aaaaaa"},
		Before: map[string]any{"status": "in_progress"}, After: map[string]any{"status": "in_review"}}}
	askFor := func(reader string) bool {
		r := views.RecapOf(tree, review, since, "visit", reader)
		require.Len(t, r.Groups, 1)
		return r.Groups[0].Entries[0].Ask
	}
	require.True(t, askFor("jc"), "the owner's own shelf review arrival is their ask")
	require.False(t, askFor("mg"), "another reader's shelf review is not theirs")

	// A removed shelf node keeps its bare id and no noun, so nothing links.
	gone := views.RecapOf(tree, []events.Event{{ID: "EV-gone", Type: "remove.bug", At: since.Add(time.Hour), Contributor: "jc",
		Command: "remove bug B-zzzzzz", Targets: []string{"B-zzzzzz"}}}, since, "visit", "jc")
	ghost := gone.Groups[0].Entries[0].Targets[0]
	require.Equal(t, views.TargetRef{ID: "B-zzzzzz", Ref: "B-zzzzzz"}, ghost)
}
