package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestProposedAtByAmendmentUsesTheEarliestOpenedEvent(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 11, 3, h, 0, 0, 0, time.UTC) }
	got := views.ProposedAtByAmendment([]events.Event{
		{Type: "amendment_opened", At: at(9), Targets: []string{"A-1"}},
		{Type: "amendment_opened", At: at(7), Targets: []string{"A-1"}},
		{Type: "amendment_staged", At: at(1), Targets: []string{"A-1"}},
		{Type: "amendment_opened", At: at(8), Targets: []string{"A-2"}},
		{Type: "add.epic", At: at(2), Targets: []string{"E-1"}},
	})
	require.Equal(t, map[string]string{
		"A-1": "2026-11-03T07:00:00Z",
		"A-2": "2026-11-03T08:00:00Z",
	}, got)
}

func TestQuestionCommandsMirrorTheQueue(t *testing.T) {
	require.Equal(t, `tiller answer question Q-1 --answer "..."`, views.QuestionAnswerCommand("Q-1"))
	require.Equal(t, `tiller cancel question Q-1 --reason "..."`, views.QuestionCancelCommand("Q-1"))
	require.Equal(t, "tiller unlink question Q-1 --blocks F-1", views.QuestionUnlinkCommand("Q-1", "F-1"))
}

// An unresolved block target passes through raw from a hand-edited ledger,
// so the copyable command single-quotes anything that is not a plain token,
// the same quoting the drift remedies use.
func TestQuestionUnlinkCommandQuotesAnUnresolvedTarget(t *testing.T) {
	require.Equal(t, `tiller unlink question Q-1 --blocks 'old feature'"'"'s key'`, views.QuestionUnlinkCommand("Q-1", "old feature's key"))
	require.Equal(t, `tiller unlink question Q-1 --blocks 'a;rm -rf $HOME'`, views.QuestionUnlinkCommand("Q-1", "a;rm -rf $HOME"))
	require.Equal(t, "tiller unlink question Q-1 --blocks checkout/alpha_2.x", views.QuestionUnlinkCommand("Q-1", "checkout/alpha_2.x"))
}
