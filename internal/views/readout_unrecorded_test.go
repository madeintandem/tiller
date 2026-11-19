package views

import (
	"fmt"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func unrecordedBug(n int) mutate.UnrecordedMerge {
	id := fmt.Sprintf("B-aaaaa%d", n)
	return mutate.UnrecordedMerge{
		Noun: store.NounBug, ID: id, Key: fmt.Sprintf("bugs/bug-%d", n), Status: "ready",
		Commits: []mutate.NamingCommit{{Commit: "1234567890abcdef1234567890abcdef12345678", Subject: id + " fix"}},
	}
}

// The readout's Needs attention list shows the finding sync warned about, on
// any moment, with the next step as the line's command.
func TestReadoutNeedsAttentionShowsUnrecordedMerges(t *testing.T) {
	tree := &store.Tree{Manifest: &store.Manifest{}}
	for _, moment := range []Moment{MomentOrient, MomentExecute, MomentSync} {
		r := ReadoutOfInput(tree, moment, ReadoutInput{UnrecordedMerges: []mutate.UnrecordedMerge{unrecordedBug(1)}})
		require.Equal(t, []ReadoutAction{{
			Text:    `bug bugs/bug-1 (B-aaaaa1) is ready with no recorded submission, but commit 1234567 "B-aaaaa1 fix" names it`,
			Command: "tiller status bug B-aaaaa1",
		}}, r.NeedsYou, string(moment))
	}

	require.Empty(t, ReadoutOfInput(tree, MomentExecute, ReadoutInput{}).NeedsYou)
}

func TestUnrecordedMergeActionsNameAFewAndCountTheRest(t *testing.T) {
	var merges []mutate.UnrecordedMerge
	for n := 1; n <= 7; n++ {
		merges = append(merges, unrecordedBug(n))
	}
	actions := unrecordedMergeActions(merges)
	require.Len(t, actions, entityCap+1)
	require.Equal(t, "tiller status bug B-aaaaa3", actions[entityCap-1].Command)
	require.Equal(t, ReadoutAction{Text: "4 more shelf items merged with no recorded submission"}, actions[entityCap])

	require.Len(t, unrecordedMergeActions(merges[:entityCap]), entityCap, "exactly the cap needs no remainder line")
	require.Equal(t, "1 more shelf item merged with no recorded submission",
		unrecordedMergeActions(merges[:entityCap+1])[entityCap].Text)
}
