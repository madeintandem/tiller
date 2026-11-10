package views

import (
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// ProposedAtByAmendment maps each amendment id to the RFC3339 UTC instant its
// earliest amendment_opened event was recorded, the moment it was proposed.
// An amendment with no retained opened event has no entry.
func ProposedAtByAmendment(evs []events.Event) map[string]string {
	earliest := map[string]time.Time{}
	for _, ev := range evs {
		if ev.Type != "amendment_opened" {
			continue
		}
		for _, id := range ev.Targets {
			if at, ok := earliest[id]; !ok || ev.At.Before(at) {
				earliest[id] = ev.At
			}
		}
	}
	out := make(map[string]string, len(earliest))
	for id, at := range earliest {
		out[id] = at.UTC().Format(time.RFC3339)
	}
	return out
}

// AttachProposedAt fills ProposedAt on amendment rows from the event stream.
// A history that cannot be read leaves every row undated rather than failing
// the list: the date is context, never a gate.
func AttachProposedAt(tree *store.Tree, noun string, items []ListItem) {
	if noun != store.NounAmendment || len(items) == 0 {
		return
	}
	evs, err := events.List(tree)
	if err != nil {
		return
	}
	proposed := ProposedAtByAmendment(evs)
	for i := range items {
		items[i].ProposedAt = proposed[items[i].ID]
	}
}

// QuestionAnswerCommand is the command that answers an open question. The
// answer text is the human's, so it carries a "..." placeholder. The queue's
// answer_question entry uses the same builder.
func QuestionAnswerCommand(id string) string {
	return `tiller answer question ` + id + ` --answer "..."`
}

// QuestionCancelCommand is the command that cancels an open question.
func QuestionCancelCommand(id string) string {
	return `tiller cancel question ` + id + ` --reason "..."`
}

// QuestionUnlinkCommand is the command that drops one blocked target from an
// open question.
func QuestionUnlinkCommand(id, target string) string {
	return "tiller unlink question " + id + " --blocks " + target
}
