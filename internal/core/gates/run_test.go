package gates

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// recordedEvent is one Progress callback invocation, captured in call order so
// a test can assert both content and sequence.
type recordedEvent struct {
	kind    string // "started", "still_running", or "finished"
	name    string
	detail  string // the command for started, the status for finished, "" for still_running
	elapsed time.Duration
}

// recordingProgress is a test fake implementing Progress. It runs on the same
// goroutine RunGate's select loop drives, so it needs no locking.
type recordingProgress struct {
	events []recordedEvent
}

func (r *recordingProgress) GateStarted(name, command string) {
	r.events = append(r.events, recordedEvent{kind: "started", name: name, detail: command})
}

func (r *recordingProgress) GateStillRunning(name string, elapsed time.Duration) {
	r.events = append(r.events, recordedEvent{kind: "still_running", name: name, elapsed: elapsed})
}

func (r *recordingProgress) GateFinished(name, status string, elapsed time.Duration) {
	r.events = append(r.events, recordedEvent{kind: "finished", name: name, detail: status, elapsed: elapsed})
}

// countKind counts how many recorded events carry the given kind.
func (r *recordingProgress) countKind(kind string) int {
	n := 0
	for _, e := range r.events {
		if e.kind == kind {
			n++
		}
	}
	return n
}

// withHeartbeatInterval overrides the package's heartbeat tick for the
// duration of a test and restores it on cleanup. heartbeatEvery is test-only
// mutable state: production never assigns to it.
func withHeartbeatInterval(t *testing.T, d time.Duration) {
	t.Helper()
	prev := heartbeatEvery
	heartbeatEvery = d
	t.Cleanup(func() { heartbeatEvery = prev })
}

// TestRunGatePass confirms a gate whose command exits 0 records pass, writes its
// evidence file under artifacts/tiller/gates/, and names that file after the
// gate and the against sha (artifact-schema.md shows build-abc1234.txt).
func TestRunGatePass(t *testing.T) {
	dir := t.TempDir()
	res, err := RunGate(GateConfig{Name: "build", Required: true, Kind: store.GateKindCommand, Run: "true", Against: "abc1234"}, dir, nil)
	require.NoError(t, err)

	require.Equal(t, store.GateStatusPass, res.Status)
	require.Equal(t, 0, res.ExitCode)
	expectedRef := fmt.Sprintf("artifacts/tiller/gates/build-abc1234-out-%x.txt", sha256.Sum256(nil))
	require.Equal(t, expectedRef, res.EvidenceRef)
	require.FileExists(t, filepath.Join(dir, filepath.FromSlash(expectedRef)))
}

// TestRunGateFail confirms a non-zero command records fail, carries the exit
// code, and still persists evidence so a red gate is as inspectable as a green
// one.
func TestRunGateFail(t *testing.T) {
	dir := t.TempDir()
	res, err := RunGate(GateConfig{Name: "tests", Required: true, Kind: store.GateKindCommand, Run: "exit 3", Against: "abc1234"}, dir, nil)
	require.NoError(t, err)

	require.Equal(t, store.GateStatusFail, res.Status)
	require.Equal(t, 3, res.ExitCode)
	expectedRef := fmt.Sprintf("artifacts/tiller/gates/tests-abc1234-out-%x.txt", sha256.Sum256(nil))
	require.FileExists(t, filepath.Join(dir, filepath.FromSlash(expectedRef)))
}

// TestRunGateCapturesOutput confirms the evidence file holds the command's
// combined stdout and stderr.
func TestRunGateCapturesOutput(t *testing.T) {
	dir := t.TempDir()
	res, err := RunGate(GateConfig{Name: "build", Run: "echo out; echo err 1>&2", Against: "abc1234"}, dir, nil)
	require.NoError(t, err)

	require.Equal(t, store.GateStatusPass, res.Status)
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(res.EvidenceRef)))
	require.NoError(t, err)
	require.Contains(t, string(data), "out")
	require.Contains(t, string(data), "err")
}

// TestRunGateNoAgainst confirms the evidence file falls back to the bare gate
// name when no against sha is supplied.
func TestRunGateNoAgainst(t *testing.T) {
	dir := t.TempDir()
	res, err := RunGate(GateConfig{Name: "build", Run: "true"}, dir, nil)
	require.NoError(t, err)
	expectedRef := fmt.Sprintf("artifacts/tiller/gates/build-out-%x.txt", sha256.Sum256(nil))
	require.Equal(t, expectedRef, res.EvidenceRef)
}

// TestRunGateEvidenceNameHasNoColon pins the Windows-safe evidence naming (dry
// run paper cut): a scheme-qualified against value like sha256:<hash> becomes a
// sha256-<short> token, the recorded ref carries no colon, and the ref always
// names the file actually written.
func TestRunGateEvidenceNameHasNoColon(t *testing.T) {
	dir := t.TempDir()
	res, err := RunGate(GateConfig{
		Name: "build", Required: true, Kind: store.GateKindCommand, Run: "true",
		Against: "sha256:0f2f65f91fe2e87f0b1231385c25e838a895e1e07a15ecbcf1a30cc4de0f7e70",
	}, dir, nil)
	require.NoError(t, err)

	require.NotContains(t, res.EvidenceRef, ":", "a colon is unrepresentable on Windows filesystems")
	expectedRef := fmt.Sprintf("artifacts/tiller/gates/build-sha256-0f2f65f91fe2-out-%x.txt", sha256.Sum256(nil))
	require.Equal(t, expectedRef, res.EvidenceRef)
	require.FileExists(t, filepath.Join(dir, filepath.FromSlash(res.EvidenceRef)),
		"the recorded ref names the file actually written")
}

func TestRunGateRefusesWhenPassingEvidenceCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o755)) })

	res, err := RunGate(GateConfig{Name: "build", Run: "true", Against: "abc1234"}, root, nil)
	require.Error(t, err)
	require.Equal(t, store.GateStatusPass, res.Status, "the command result remains available for diagnostics")
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonEvidenceUnwritable, env.Reason)
	require.NoFileExists(t, filepath.Join(root, filepath.FromSlash(res.EvidenceRef)))
}

// TestRunGateRefusesEvidenceSymlinkEscape confirms gate evidence stays inside
// the checkout: an artifacts directory that is a symlink to an outside
// directory refuses the write instead of following it.
func TestRunGateRefusesEvidenceSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "artifacts")))

	res, err := RunGate(GateConfig{Name: "build", Run: "true", Against: "abc1234"}, root, nil)
	require.Error(t, err)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonEvidenceUnwritable, env.Reason)
	require.Equal(t, store.GateStatusPass, res.Status)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries, "no evidence, directory, or temporary file may land outside the checkout")
}

// TestRunGateProgressReportsStartedAndFinishedOnPass confirms a passing gate
// emits Started (carrying the gate name and its exact command string) then
// Finished (carrying pass), in that order, and nothing else.
func TestRunGateProgressReportsStartedAndFinishedOnPass(t *testing.T) {
	dir := t.TempDir()
	p := &recordingProgress{}
	res, err := RunGate(GateConfig{Name: "build", Run: "true", Against: "abc1234"}, dir, p)
	require.NoError(t, err)
	require.Equal(t, store.GateStatusPass, res.Status)

	require.Len(t, p.events, 2, "a fast gate reports exactly started and finished")
	require.Equal(t, recordedEvent{kind: "started", name: "build", detail: "true"}, p.events[0])
	require.Equal(t, "finished", p.events[1].kind)
	require.Equal(t, "build", p.events[1].name)
	require.Equal(t, store.GateStatusPass, p.events[1].detail)
}

// TestRunGateProgressReportsFinishedOnFail confirms a failing gate's Finished
// event carries fail, not pass.
func TestRunGateProgressReportsFinishedOnFail(t *testing.T) {
	dir := t.TempDir()
	p := &recordingProgress{}
	_, err := RunGate(GateConfig{Name: "tests", Run: "exit 3", Against: "abc1234"}, dir, p)
	require.NoError(t, err)

	require.NotEmpty(t, p.events)
	last := p.events[len(p.events)-1]
	require.Equal(t, "finished", last.kind)
	require.Equal(t, store.GateStatusFail, last.detail)
}

// TestRunGateNilProgressRunsUnaffected confirms a nil Progress is silent and
// changes nothing about the run: same status, exit code, and evidence ref as
// the identical command run with a recording Progress attached.
func TestRunGateNilProgressRunsUnaffected(t *testing.T) {
	cfg := GateConfig{Name: "build", Run: "echo out; echo err 1>&2", Against: "abc1234"}

	dir := t.TempDir()
	withNil, err := RunGate(cfg, dir, nil)
	require.NoError(t, err)

	dir2 := t.TempDir()
	withProgress, err := RunGate(cfg, dir2, &recordingProgress{})
	require.NoError(t, err)

	require.Equal(t, withNil.Status, withProgress.Status)
	require.Equal(t, withNil.ExitCode, withProgress.ExitCode)
	require.Equal(t, withNil.EvidenceRef, withProgress.EvidenceRef,
		"the evidence ref is content-addressed, so an identical command produces an identical ref regardless of Progress")
}

// TestRunGateHeartbeatTicksForALongRunningCommand confirms a command that
// outlives the heartbeat interval reports at least one StillRunning event.
// heartbeatEvery is overridden to a small test-only interval (no injected-clock
// convention exists in this package to hook instead) so the test stays fast; a
// real `sh -c` command still does the actual outliving.
func TestRunGateHeartbeatTicksForALongRunningCommand(t *testing.T) {
	withHeartbeatInterval(t, 30*time.Millisecond)

	dir := t.TempDir()
	p := &recordingProgress{}
	_, err := RunGate(GateConfig{Name: "slow", Run: "sleep 0.3", Against: "abc1234"}, dir, p)
	require.NoError(t, err)

	require.GreaterOrEqual(t, p.countKind("still_running"), 1,
		"a command outliving the interval reports at least one heartbeat")
}

// TestRunGateHeartbeatSkipsAFastCommand confirms a command that finishes before
// the real 15-second user-facing heartbeat threshold reports no StillRunning
// event. The command completes immediately, so completion stops the ticker
// without making the test wait for that threshold.
func TestRunGateHeartbeatSkipsAFastCommand(t *testing.T) {

	dir := t.TempDir()
	p := &recordingProgress{}
	_, err := RunGate(GateConfig{Name: "fast", Run: "true", Against: "abc1234"}, dir, p)
	require.NoError(t, err)

	require.Equal(t, 0, p.countKind("still_running"),
		"a command finishing inside one interval reports no heartbeat")
}

func TestEvidenceRefSeparatesDifferentOutputsAtTheSameRevision(t *testing.T) {
	g := GateConfig{Name: "build", Against: "sha256:abc1234"}
	first := evidenceRef(g, []byte("first output\n"))
	second := evidenceRef(g, []byte("second output\n"))

	require.NotEqual(t, first, second)
	require.Equal(t, first, evidenceRef(g, []byte("first output\n")))
}
