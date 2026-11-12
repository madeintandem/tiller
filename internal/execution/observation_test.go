package execution_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRelatedTipsReportFreshnessOnTheRecordNotAsDiagnostics(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "execution")
	older := git(t, wt, "rev-parse", "HEAD")
	git(t, wt, "commit", "--allow-empty", "-m", "newer local work")
	git(t, root, "worktree", "remove", wt)
	git(t, root, "config", "remote.origin.url", "/not-contacted")
	git(t, root, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	git(t, root, "update-ref", "refs/remotes/origin/tiller/"+firstKey, older)
	s := load(t, base(t, root))
	require.Equal(t, "local_branch", s.Observations[firstID].Source)
	require.Equal(t, "remote_tracking_behind", s.Observations[firstID].Freshness)
	require.Empty(t, s.Diagnostics)
}

func TestUncommittedReflectsOnlyTillerState(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "execution")
	put(t, wt, "src/edit.go", "package edit\n")
	s := load(t, base(t, root))
	require.False(t, s.Observations[firstID].Uncommitted, "a code edit is not uncommitted Tiller state")
	put(t, wt, ".tiller/events/2026/10/06/EV-cccccc.yml", "id: EV-cccccc\ntype: check.task\nat: 2026-10-06T12:00:00Z\nowner: sam\ntargets: ["+firstID+"]\n")
	s = load(t, base(t, root))
	require.True(t, s.Observations[firstID].Uncommitted)
}

func TestAnotherTasksInvalidEventDoesNotBreakTaskHistory(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	put(t, wt, ".tiller/events/2026/10/06/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: 2026-10-06T12:00:00Z\nowner: sam\ntargets: ["+firstID+"]\n")
	put(t, wt, ".tiller/events/2026/10/06/EV-bbbbbb.yml", "id: EV-mismatch\ntype: start.task\nat: 2026-10-06T12:00:01Z\nowner: pat\ntargets: ["+secondID+"]\n")
	evs, err := load(t, base(t, root)).Events(context.Background(), firstID)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	put(t, wt, ".tiller/events/2026/10/06/EV-dddddd.yml", "id: EV-mismatch\ntype: check.task\nat: 2026-10-06T12:00:02Z\nowner: sam\ntargets: ["+firstID+"]\n")
	_, err = load(t, base(t, root)).Events(context.Background(), firstID)
	require.Error(t, err, "this task's own invalid event still fails its history")
}

func TestSettleDropsOnlyTheMovedSource(t *testing.T) {
	root := fixture(t)
	first := worktree(t, root, firstKey)
	executing(t, first, firstID, firstKey, "sam", "check")
	second := worktree(t, root, secondKey)
	executing(t, second, secondID, secondKey, "pat", "in_progress")
	s := load(t, base(t, root))
	require.Len(t, s.Observations, 2)
	git(t, first, "add", ".")
	git(t, first, "commit", "-m", "agent commits mid-request")
	requireSettledAway(t, s, firstID, "", "")
	require.Contains(t, s.Observations, secondID)
	require.Equal(t, "pat", s.Tree.Tasks[secondID].Owner)
	for _, task := range s.Tree.FeatureTasks["F-aaaaaa"] {
		require.Same(t, s.Tree.Tasks[task.ID], task)
	}
	require.Equal(t, []store.Contributor{{ID: "pat", Name: "Pat"}}, s.Tree.Contributors["F-aaaaaa"])
}

// TestObservationGitCallsStayBounded guards the per-request cost: discovery is
// a fixed handful of Git calls, each worktree source adds a few to capture and
// a few to confirm, and repeated reads from a source add none.
func TestObservationGitCallsStayBounded(t *testing.T) {
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	root := fixture(t)
	const sources = 4
	for i := range sources {
		key := fmt.Sprintf("pay/checkout/t%d", i)
		put(t, root, taskPath(key), fmt.Sprintf("id: T-cost%02d\nkey: %s\nstatus: ready\n", i, key))
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "more tasks")
	for i := range sources {
		key := fmt.Sprintf("pay/checkout/t%d", i)
		wt := worktree(t, root, key)
		executing(t, wt, fmt.Sprintf("T-cost%02d", i), key, "sam", "check")
		put(t, wt, evidencePath, "evidence\n")
	}
	tree := base(t, root)
	shim := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho \"$*\" >> \"" + calls + "\"\nexec \"" + realGit + "\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(shim, "git"), []byte(script), 0o755))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := load(t, tree)
	require.Len(t, s.Observations, sources)
	for id, record := range s.Observations {
		for range 5 {
			_, err := s.ReadFile(id, record.Revision, evidencePath, evidenceLimit)
			require.NoError(t, err)
		}
	}
	require.NoError(t, s.CheckStable())
	data, err := os.ReadFile(calls)
	require.NoError(t, err)
	// Discovery 5, capture up to 4 per source, confirmation 2 plus 2 per source.
	require.LessOrEqual(t, strings.Count(string(data), "\n"), 7+6*sources, "git calls:\n%s", data)
}

// countGitCalls puts a logging git first on PATH and returns a function that
// reports how many git processes ran since the shim went in.
func countGitCalls(t *testing.T) func() int {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	shim := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho \"$*\" >> \"" + calls + "\"\nexec \"" + realGit + "\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(shim, "git"), []byte(script), 0o755))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(calls)
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
		return strings.Count(string(data), "\n")
	}
}

// committedEventSource commits count event files on the first task's branch,
// alternating between the two fixture tasks, and serves it as a committed
// local branch with no worktree.
func committedEventSource(t *testing.T, count int) *execution.Snapshot {
	t.Helper()
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "in_review")
	for i := range count {
		target := firstID
		if i%2 == 1 {
			target = secondID
		}
		eventID := fmt.Sprintf("EV-%06d", i)
		put(t, wt, ".tiller/events/2026/10/06/"+eventID+".yml", fmt.Sprintf("id: %s\ntype: check.task\nat: 2026-10-06T12:%02d:%02dZ\ncontributor: sam\ntargets: [%s]\n", eventID, i/60, i%60, target))
	}
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "submitted execution")
	git(t, root, "worktree", "remove", wt)
	s := load(t, base(t, root))
	require.Equal(t, "local_branch", s.Observations[firstID].Source)
	return s
}

// TestCommittedSourceEventsGitCallsStayBounded guards the node page's history
// read on a committed source: reading its events costs the same handful of
// Git calls whether the branch carries five event files or sixty.
func TestCommittedSourceEventsGitCallsStayBounded(t *testing.T) {
	callsFor := func(count int) int {
		s := committedEventSource(t, count)
		calls := countGitCalls(t)
		evs, err := s.Events(context.Background(), firstID)
		require.NoError(t, err)
		require.Len(t, evs, (count+1)/2)
		for _, event := range evs {
			require.Equal(t, []string{firstID}, event.Targets)
		}
		return calls()
	}
	small, large := callsFor(5), callsFor(60)
	require.Positive(t, small)
	require.Equal(t, small, large, "event reads must not grow with the number of event files")

	segmentCallsFor := func(days int) int {
		s := committedSegmentSource(t, days, 5)
		calls := countGitCalls(t)
		evs, err := s.Events(context.Background(), firstID)
		require.NoError(t, err)
		require.Len(t, evs, days*5)
		return calls()
	}
	fewDays, manyDays := segmentCallsFor(2), segmentCallsFor(12)
	require.Equal(t, small, fewDays, "segments must not cost more Git calls than loose files")
	require.Equal(t, fewDays, manyDays, "event reads must not grow with the number of segments")
}

func TestSourceEventsStopOnCancelledContext(t *testing.T) {
	s := committedEventSource(t, 5)
	calls := countGitCalls(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Events(ctx, firstID)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls())
}
