package execution_test

import (
	"context"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/stretchr/testify/require"
)

func eventIDs(evs []events.Event) []string {
	ids := make([]string, len(evs))
	for i, event := range evs {
		ids[i] = event.ID
	}
	return ids
}

// TestIntegrationEventsReadOnce pins the memo: the second call answers from
// the first read, so an event file written after it is not seen, and a
// caller's changes to the returned slice do not reach the next caller.
func TestIntegrationEventsReadOnce(t *testing.T) {
	root := fixture(t)
	ownershipEvent(t, root, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
	s := load(t, base(t, root))
	first, err := s.IntegrationEvents(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"EV-claim1"}, eventIDs(first))
	ownershipEvent(t, root, "EV-later1", "release.task", "2026-10-06T10:10:00Z", "sam")
	second, err := s.IntegrationEvents(context.Background())
	require.NoError(t, err)
	require.Equal(t, first, second, "a repeat call answers from the first read")
	second[0] = events.Event{}
	third, err := s.IntegrationEvents(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"EV-claim1"}, eventIDs(third))
}

// TestIntegrationEventsCancelledReadIsNotKept pins that a cancelled first call
// leaves nothing behind: the next call with a live context reads the stream.
func TestIntegrationEventsCancelledReadIsNotKept(t *testing.T) {
	root := fixture(t)
	ownershipEvent(t, root, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
	s := load(t, base(t, root))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	evs, err := s.IntegrationEvents(cancelled)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, evs)
	ownershipEvent(t, root, "EV-later1", "release.task", "2026-10-06T10:10:00Z", "sam")
	evs, err = s.IntegrationEvents(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"EV-claim1", "EV-later1"}, eventIDs(evs))
}

// TestOwnershipCheckSharesIntegrationEvents pins the saving: when an owner
// disagreement makes Load read the integration stream, a fork's
// IntegrationEvents answers from that read instead of reading it again.
func TestOwnershipCheckSharesIntegrationEvents(t *testing.T) {
	root, wt := claimedOnIntegration(t)
	put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\ndepends_on: ["+secondID+"]\n")
	ownershipEvent(t, root, "EV-later1", "release.task", "2026-10-06T10:10:00Z", "sam")
	ownershipEvent(t, wt, "EV-start2", "start.task", "2026-10-06T10:15:00Z", "sam")
	s := load(t, base(t, root))
	require.Contains(t, s.Observations, firstID, "the branch's newer start stands")
	ownershipEvent(t, root, "EV-after1", "claim.task", "2026-10-06T10:20:00Z", "pat")
	evs, err := s.Fork().IntegrationEvents(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"EV-claim1", "EV-later1"}, eventIDs(evs))
}
