package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/stretchr/testify/require"
)

// syncClock pins the clock sync compacts by to a fixed day for the test.
func syncClock(t *testing.T, now string) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, now)
	require.NoError(t, err)
	old := syncNow
	syncNow = func() time.Time { return at }
	t.Cleanup(func() { syncNow = old })
}

func TestSyncCompactsEligibleDaysAndPrintsTheStageCommand(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "one event")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Compacted 1 day of events (1 event) into 1 file. Stage them with:")
	require.Contains(t, out, "git add -A -- .tiller/events/2026/11/01")

	loose := layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-02T10:00:00Z")
	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	compacted, ok := jsonData(t, out)["compacted"].(map[string]any)
	require.True(t, ok, out)
	require.Len(t, compacted["days"], 1)
	require.NoFileExists(t, loose)
}

func TestSyncSaysNothingAboutCompactionWhenNoDayIsEligible(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-10T10:00:00Z")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "Compacted")

	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, jsonData(t, out), "compacted")
	require.NotContains(t, jsonData(t, out), "compact_warning")
}

// TestSyncSaysNothingWhenOnlySkippedEventsRemain pins that sync stays silent
// when it compacted nothing, even when an eligible day holds an event no
// segment can carry: tiller compact is where that is reported.
func TestSyncSaysNothingWhenOnlySkippedEventsRemain(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	layUncarriableEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "ompact")

	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, jsonData(t, out), "compacted")
	require.NotContains(t, jsonData(t, out), "compact_warning")
}

// TestSyncNamesSkippedEventsBesideADayItCompacted pins the other half: once a
// day folds, the report carries the skipped-events line too.
func TestSyncNamesSkippedEventsBesideADayItCompacted(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	layUncarriableEvent(t, dir, "EV-bbbbbb", "2026-11-01T11:00:00Z")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Compacted 1 day of events (1 event) into 1 file.")
	require.Contains(t, out, "1 event could not be compacted and stays as its own file.")
}

func TestSyncOffTheIntegrationBranchDoesNotCompact(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	declareIntegrationBranchCLI(t, dir)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	loose := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	gitRun(t, dir, "checkout", "-b", "scratch")

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--into", "main")
	require.Equal(t, 0, code, out)
	require.FileExists(t, loose)
	require.NotContains(t, out, "ompact")
}

func TestSyncSurvivesACompactionFailure(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	_, err := mutate.Compact(dir, mutate.CompactOptions{Now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	// The same event comes back loose with different content: the two copies disagree.
	bad := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	data, err := os.ReadFile(bad)
	require.NoError(t, err)
	changed := strings.Replace(string(data), "tiller add task", "tiller add feature", 1)
	require.NotEqual(t, string(data), changed)
	require.NoError(t, os.WriteFile(bad, []byte(changed), 0o644))

	clean, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, clean)
	data2 := jsonData(t, clean)
	require.Contains(t, data2["compact_warning"], "Events not compacted:")
	require.Empty(t, data2["crossings"])
	require.FileExists(t, bad)
}

// TestSyncCompactsNewerDaysPastARefusedDay pins that a day whose copies
// disagree warns but does not stop sync from compacting the days after it.
func TestSyncCompactsNewerDaysPastARefusedDay(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	bad := layDisagreeingDay(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	alsoBad := layDisagreeingDay(t, dir, "EV-cccccc", "2026-11-03T10:00:00Z")
	newer := layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-02T10:00:00Z")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Compacted 1 day of events (1 event) into 1 file. Stage them with:")
	require.Regexp(t, `(?m)^Events not compacted: mutate: compact 2026-11-01: [^\n]*; mutate: compact 2026-11-03: `, out,
		"every refused day on the one warning line")
	require.FileExists(t, bad)
	require.FileExists(t, alsoBad)
	require.NoFileExists(t, newer)
}

// panickingCompaction makes the compaction sync and the upgrade run panic
// for the rest of the test.
func panickingCompaction(t *testing.T) {
	t.Helper()
	old := compactEvents
	compactEvents = func(string, mutate.CompactOptions) (mutate.CompactResult, error) {
		panic("segment reader exploded")
	}
	t.Cleanup(func() { compactEvents = old })
}

func TestSyncSurvivesACompactionPanic(t *testing.T) {
	dir := compactRepo(t)
	registerContributor(t, "jc", "JC")
	syncClock(t, "2026-11-11T12:00:00Z")
	loose := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	panickingCompaction(t)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "Events not compacted: segment reader exploded", jsonData(t, out)["compact_warning"])
	require.FileExists(t, loose)
}
