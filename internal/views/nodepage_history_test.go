package views

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/stretchr/testify/require"
)

// These tests pin the History tab's Outcome. The keys relied on are the ones
// the mutations really write (transitionEvent in core/mutate): Before["status"]
// and After["status"]. check, attest, and eval events record the node's
// unchanged status and no gate or criterion result, so they carry no outcome.

func outcomeEvent(typ string, before, after map[string]any) events.Event {
	return events.Event{
		ID: "EV-" + typ, Type: typ, Contributor: "jc", Command: "tiller " + typ,
		At:      time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		Targets: []string{"F-111111"}, Before: before, After: after,
	}
}

func TestHistoryOutcomeIsTheStatusTheEventProduced(t *testing.T) {
	cases := []struct {
		name string
		ev   events.Event
		want string
	}{
		{"start moves to in_progress", outcomeEvent("start.task", map[string]any{"status": "ready"}, map[string]any{"status": "in_progress", "started": true}), "in_progress"},
		{"submit moves to in_review", outcomeEvent("submit.task", map[string]any{"status": "in_progress"}, map[string]any{"status": "in_review"}), "in_review"},
		{"an add records the status it created", outcomeEvent("add.feature", nil, map[string]any{"status": "draft", "key": "a/b"}), "draft"},
		{"a passing check that advanced the task", outcomeEvent("check.task", map[string]any{"status": "in_progress"}, map[string]any{"status": "check"}), "check"},
		{"a check that left the status alone has none", outcomeEvent("check.feature", map[string]any{"status": "active"}, map[string]any{"status": "active", "reused": []string{"lint"}}), ""},
		{"attest records no gate result", outcomeEvent("attest.task", map[string]any{"status": "check"}, map[string]any{"status": "check"}), ""},
		{"eval records no criterion result", outcomeEvent("eval.feature", map[string]any{"status": "active"}, map[string]any{"status": "active"}), ""},
		{"a link has no status", outcomeEvent("link.task", nil, nil), ""},
		{"a non-string status is ignored", outcomeEvent("x.task", nil, map[string]any{"status": 3}), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, eventOutcome(c.ev))
		})
	}
}

func TestHistoryEntriesCarryOutcomeOnTheWire(t *testing.T) {
	started := outcomeEvent("start.task", map[string]any{"status": "ready"}, map[string]any{"status": "in_progress"})
	started.Targets = []string{"T-333333"}
	linked := outcomeEvent("link.task", nil, nil)
	linked.Targets = []string{"T-333333"}
	linked.At = started.At.Add(time.Minute)
	got := historyOf([]events.Event{started, linked}, "T-333333")
	require.Len(t, got, 2)
	require.Equal(t, "", got[0].Outcome)
	require.Equal(t, "in_progress", got[1].Outcome)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"outcome":"in_progress"`)
	require.Equal(t, 1, strings.Count(string(raw), `"outcome"`), "an empty outcome is omitted")
}
