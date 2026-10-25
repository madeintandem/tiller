package views_test

// These tests pin task A2 of the console chart-table pass: a small additive
// read-model projection answering "whose move is it", per node and
// project-wide. NextOwner adds no rules of its own; it strictly folds the
// lifecycle engine's two existing answers, NextMoves (the legal-next verb set
// the status oracle already reports as legal_next) and ActorFor (the
// verb-actor table #19 added), into the single bit a chart or table needs.
//
// The table below is hand-transcribed from
// internal/core/lifecycle/table.go's transitions and verbActors, one entry
// per documented (noun, state), the same executable-spec idiom
// lifecycle_test.go's expectedNextMoves uses. It matters because "cancel" (and
// on a ready node, "return") is a legal, human-only verb from nearly every
// non-terminal state: a naive "any legal verb is human-only" reading would
// answer "human" almost everywhere and "agent" nowhere. NextOwner answers
// "agent" whenever at least one legal verb is the agent's own assertion or a
// shared either verb, "human" only when literally every legal verb is
// human-only, and "" at a terminal state with no legal verb at all.
import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestNextOwnerPerState table-drives views.NextOwner over one node per
// documented state of every noun, including the question ledger's bespoke
// open -> [answer] life (NextMoves' documented special case, outside the
// generic transitions table). "ready task" and "done" are the brief's own
// worked examples; "draft feature" is checked against the real transitions
// table (draft feature's only legal verbs are spec and cancel; there is no
// draft -> approve row for a feature, only for an epic).
func TestNextOwnerPerState(t *testing.T) {
	cases := []struct {
		name string
		node store.Node
		want string
	}{
		// Epic: every non-terminal state offers human-only verbs alone
		// (approve, cancel, archive) with no either/agent verb ever, so
		// NextOwner is "human" whenever the epic has any legal move at all,
		// including "done", which is not terminal for an epic: archive is
		// still legal. The epic's forward progress into done is a rollup sync
		// records rather than a verb, so it never shows up here.
		{"epic draft awaits approve, human-only", &store.Epic{ID: "E-1", Status: "draft"}, "human"},
		{"epic ready awaits cancel alone, human-only", &store.Epic{ID: "E-2", Status: "ready"}, "human"},
		{"epic done still awaits archive, human-only", &store.Epic{ID: "E-3", Status: "done"}, "human"},
		{"epic cancelled is terminal", &store.Epic{ID: "E-4", Status: "cancelled"}, ""},
		{"epic archived is terminal", &store.Epic{ID: "E-5", Status: "archived"}, ""},

		// Feature: draft's legal verbs are spec (agent) and cancel (human):
		// mixed, so agent, even though the brief's prose paraphrase suggested
		// otherwise (draft feature has no approve verb in the real table).
		{"draft feature: agent's to spec, though a human can still cancel", &store.Feature{ID: "F-1", Status: "draft"}, "agent"},
		{"specced feature awaits approve, human-only", &store.Feature{ID: "F-2", Status: "specced"}, "human"},
		{"ready feature activates through a task; its own legal moves remain human decisions", &store.Feature{ID: "F-3", Status: "ready"}, "human"},
		// An active feature's only remaining verb is cancel: a PR always represents
		// a task, so the work moves through the tasks beneath it and its done is
		// the rollup sync records rather than a verb anyone runs. The next move at
		// this grain really is the human's, and the answer says so.
		{"active feature: nothing left but the human's cancel; work moves through its tasks", &store.Feature{ID: "F-4", Status: "active"}, "human"},
		{"done feature is terminal", &store.Feature{ID: "F-6", Status: "done"}, ""},
		{"cancelled feature is terminal", &store.Feature{ID: "F-7", Status: "cancelled"}, ""},

		// Task: proposed is the pure human gate; ready is the brief's explicit
		// worked example ("ready task -> agent start -> agent").
		{"proposed task awaits approve, human-only", &store.Task{ID: "T-1", Status: "proposed"}, "human"},
		{"ready task: agent's to start (the brief's own example)", &store.Task{ID: "T-2", Status: "ready"}, "agent"},
		{"in_progress task: agent's to check, though a human can still cancel", &store.Task{ID: "T-3", Status: "in_progress"}, "agent"},
		{"check task: agent's to submit, though a human can still cancel", &store.Task{ID: "T-4", Status: "check"}, "agent"},
		{"in_review task: agent's to resume or sync, though a human can still cancel", &store.Task{ID: "T-5", Status: "in_review"}, "agent"},
		{"done task is terminal (the brief's own example)", &store.Task{ID: "T-6", Status: "done"}, ""},
		{"cancelled task is terminal", &store.Task{ID: "T-7", Status: "cancelled"}, ""},

		// Question: the ledger's bespoke two-state life, outside the generic
		// transitions table (NextMoves' documented special case).
		{"open question awaits answer, human-only", &store.Question{ID: "Q-1", Status: "open"}, "human"},
		{"cancelled question is terminal", &store.Question{ID: "Q-3", Status: "cancelled"}, ""},
		{"answered question is terminal", &store.Question{ID: "Q-2", Status: "answered"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, views.NextOwner(tc.node))
		})
	}
}

// TestWhoseMoveOfCountsAcrossTheTree pins the project-wide rollup: a tree
// carrying nodes across all three answers tallies human and agent
// separately, the terminal nodes contributing to neither count.
func TestWhoseMoveOfCountsAcrossTheTree(t *testing.T) {
	epicKey := "form-engine"
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-1": {ID: "E-1", Key: &epicKey, Status: "ready"}, // human: cancel
		},
		Features: map[string]*store.Feature{
			"F-1": {ID: "F-1", Status: "ready"}, // human: return or cancel; task start activates it
			"F-2": {ID: "F-2", Status: "done"},  // terminal
		},
		Tasks: map[string]*store.Task{
			"T-1": {ID: "T-1", Status: "proposed"},  // human: approve
			"T-2": {ID: "T-2", Status: "cancelled"}, // terminal
		},
		Questions: map[string]*store.Question{
			"Q-1": {ID: "Q-1", Status: "open"}, // human: answer
		},
	}

	proposedKey, carriedKey, returnedKey, choreKey := "bugs/proposed", "bugs/carried", "bugs/returned", "chores/bump"
	tree.Bugs = map[string]*store.Bug{
		"B-prop00": {WorkItem: store.WorkItem{ID: "B-prop00", Key: &proposedKey, Status: "proposed"}},
		"B-carry0": {WorkItem: store.WorkItem{ID: "B-carry0", Key: &carriedKey, Status: "ready", ResolvedBy: "F-1"}},
		"B-return": {WorkItem: store.WorkItem{ID: "B-return", Key: &returnedKey, Status: "ready", ResolvedBy: "F-gone00"}},
	}
	tree.Chores = map[string]*store.Chore{
		"CH-bump00": {WorkItem: store.WorkItem{ID: "CH-bump00", Key: &choreKey, Status: "ready"}},
	}

	got := views.WhoseMoveOf(tree)
	require.Equal(t, views.WhoseMoveCounts{Human: 5, Agent: 2}, got)
}

// TestSummaryCarriesWhoseMoveCounts pins the ProjectSummary wiring: the
// additive whose_move field marshals the same tally WhoseMoveOf computes, and
// carries no omitempty of its own (the same always-present shape as counts),
// so a project with nothing left to move still renders zeroes rather than an
// absent key.
func TestSummaryCarriesWhoseMoveCounts(t *testing.T) {
	tree := &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Name: "Test", Key: "test", Kind: "greenfield"}},
		Tasks: map[string]*store.Task{
			"T-1": {ID: "T-1", Status: "proposed"}, // human
			"T-2": {ID: "T-2", Status: "ready"},    // agent
			"T-3": {ID: "T-3", Status: "done"},     // terminal
		},
	}

	summary := views.Summary(tree)
	require.Equal(t, views.WhoseMoveCounts{Human: 1, Agent: 1}, summary.WhoseMove)

	raw, err := json.Marshal(summary)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"whose_move":{"human":1,"agent":1}`)
}
