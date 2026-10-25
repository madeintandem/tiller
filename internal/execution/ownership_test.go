package execution_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/stretchr/testify/require"
)

// ownershipEvent records one task ownership event at its dated stream path.
func ownershipEvent(t *testing.T, root, id, typ, at, owner string) {
	t.Helper()
	when, err := time.Parse(time.RFC3339, at)
	require.NoError(t, err)
	path := events.RelPath(events.Event{ID: id, At: when})
	put(t, root, path, "id: "+id+"\ntype: "+typ+"\nat: "+at+"\nowner: "+owner+"\ntargets: ["+firstID+"]\n")
}

// claimedOnIntegration commits sam's claim on integration, then branches the
// task off it and starts it there, the execute skill's ordinary flow.
func claimedOnIntegration(t *testing.T) (root, wt string) {
	t.Helper()
	root = fixture(t)
	put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\ndepends_on: ["+secondID+"]\n")
	ownershipEvent(t, root, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "sam claims alfa")
	wt = worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "in_progress")
	ownershipEvent(t, wt, "EV-start1", "start.task", "2026-10-06T10:05:00Z", "sam")
	return root, wt
}

func TestNewerIntegrationOwnershipOutranksStaleBranch(t *testing.T) {
	for _, change := range []struct{ name, typ, owner string }{
		{"release", "release.task", ""},
		{"reassign", "reassign.task", "pat"},
	} {
		t.Run(change.name, func(t *testing.T) {
			root, _ := claimedOnIntegration(t)
			ownerLine := ""
			if change.owner != "" {
				ownerLine = "owner: " + change.owner + "\n"
			}
			put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\n"+ownerLine+"depends_on: ["+secondID+"]\n")
			ownershipEvent(t, root, "EV-later1", change.typ, "2026-10-06T10:10:00Z", "lead")
			s := load(t, base(t, root))
			require.NotContains(t, s.Observations, firstID)
			require.True(t, hasDiagnostic(s, "ownership_superseded"))
			require.Same(t, s.Base.Tasks[firstID], s.Tree.Tasks[firstID])
			require.Equal(t, change.owner, s.Tree.Tasks[firstID].Owner)
			require.False(t, s.Tree.Tasks[firstID].Started)
			require.NoError(t, s.CheckStable())
		})
	}
}

func TestNewerBranchOwnershipOutranksIntegration(t *testing.T) {
	root, wt := claimedOnIntegration(t)
	put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\ndepends_on: ["+secondID+"]\n")
	ownershipEvent(t, root, "EV-later1", "release.task", "2026-10-06T10:10:00Z", "sam")
	// Sam picks the task back up on the branch after the integration release.
	ownershipEvent(t, wt, "EV-start2", "start.task", "2026-10-06T10:15:00Z", "sam")
	s := load(t, base(t, root))
	require.Contains(t, s.Observations, firstID)
	require.Equal(t, "sam", s.Tree.Tasks[firstID].Owner)
	require.Equal(t, "in_progress", s.Tree.Tasks[firstID].Status)
	require.False(t, hasDiagnostic(s, "ownership_superseded"))
}

func TestBranchCarryingIntegrationsNewestOwnershipStands(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			root := fixture(t)
			put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\n")
			ownershipEvent(t, root, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "sam claims alfa")
			wt := worktree(t, root, firstKey)
			// The branch already holds integration's claim event, so the
			// branch's own, unevented release is the newer ownership record.
			put(t, wt, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nstarted: false\n")
			if committed {
				git(t, wt, "commit", "-am", "released on the branch")
				git(t, root, "worktree", "remove", wt)
			}
			s := load(t, base(t, root))
			require.Contains(t, s.Observations, firstID)
			require.Empty(t, s.Tree.Tasks[firstID].Owner)
			require.Equal(t, "sam", s.Base.Tasks[firstID].Owner)
		})
	}
}

func TestUnreadableIntegrationOwnershipKeepsIntegration(t *testing.T) {
	root, _ := claimedOnIntegration(t)
	put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\ndepends_on: ["+secondID+"]\n")
	put(t, root, ".tiller/events/2026/10/06/EV-broken.yml", "not: [valid")
	s := load(t, base(t, root))
	require.NotContains(t, s.Observations, firstID)
	require.True(t, hasDiagnostic(s, "ownership_unresolved"))
	require.Empty(t, s.Tree.Tasks[firstID].Owner)
}
