package views

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Aggregate merge asks are queue entries like any other: each grain counts on
// its own and drills into its own queue slice, after the closeout group they
// follow in the queue. An empty queue stays an empty attention list.
func TestNeedsYouCountsAggregateMergeAsksPerGrain(t *testing.T) {
	q := Queue{Entries: []QueueEntry{
		{Kind: "workflow_closeout", Noun: store.NounFeature, ID: "F-close0"},
		{Kind: "merge_epic", Noun: store.NounEpic, ID: "E-merge0"},
		{Kind: "merge_feature", Noun: store.NounFeature, ID: "F-merge0"},
		{Kind: "merge_feature", Noun: store.NounFeature, ID: "F-merge1"},
	}}
	got := needsYouFromQueue(q)
	commands := make([]string, 0, len(got))
	for _, action := range got {
		require.NotEmpty(t, action.Text)
		commands = append(commands, action.Command)
	}
	require.Equal(t, []string{
		"tiller queue --kind workflow_closeout",
		"tiller queue --kind merge_epic",
		"tiller queue --kind merge_feature",
	}, commands)
	require.Contains(t, got[2].Text, "2 ", "both feature merge asks are counted")

	require.Empty(t, needsYouFromQueue(Queue{}))
}
