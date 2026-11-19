package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
)

// unrecordedMergeActions turns sync's unrecorded-merge findings into Needs
// attention lines: one per node, naming it, its status, and the commit, with
// the next step as the line's command. Unlike the queue's count lines these
// name the node, because the finding is about one specific merge and has no
// queue slice to drill into. The list names entityCap nodes and counts the
// rest, the same bound the readout's entity list keeps.
func unrecordedMergeActions(merges []mutate.UnrecordedMerge) []ReadoutAction {
	var actions []ReadoutAction
	for i, m := range merges {
		if i == entityCap {
			rest := len(merges) - entityCap
			actions = append(actions, ReadoutAction{Text: fmt.Sprintf(
				"%d more shelf %s merged with no recorded submission", rest, plural(rest, "item", "items"))})
			break
		}
		actions = append(actions, ReadoutAction{Text: m.Finding(), Command: m.NextStep()})
	}
	return actions
}
