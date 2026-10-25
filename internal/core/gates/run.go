// Package gates runs the command-backed gates a node's acceptance depends on and
// turns each run into a stored gate result. It shells the configured command,
// captures the combined output as evidence under artifacts/tiller/gates/, and
// reports pass or fail (artifact-schema.md, "Gates"; cli-surface.md, "Gates,
// evals, and attestation").
//
// It reasons about nothing and never writes to stdout. RunGate is the one pure
// primitive here: it runs a single gate. The orchestration that selects a node's
// gates, records the gates block, and advances state lives in internal/core/mutate,
// and the CLI shell is internal/cli.
//
// Layering: this package imports only internal/store for the gate-status enum and
// the atomic writer; nothing in store or the other core packages depends on it.
package gates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// evidenceDir is the repo-relative directory gate evidence lands in. The schema's
// evidence refs point into artifacts/tiller/ (artifact-schema.md, The stored gate
// block).
const evidenceDir = "artifacts/tiller/gates"

// ReasonEvidenceUnwritable identifies a completed gate whose command output
// could not be durably persisted.
const ReasonEvidenceUnwritable = "evidence_unwritable"

// GateConfig is one command-backed gate resolved from the manifest's required
// gates, ready to run: its name, whether it is required, its kind, the shell
// command, and the short commit sha the run is pinned to. Against is used only to
// name the evidence file so a recorded result is traceable to the revision it ran
// over; it carries no "git:" prefix.
type GateConfig struct {
	Name     string
	Required bool
	Kind     string
	Run      string
	Against  string
	// Reuse is the manifest's reuse opt-in for this gate (GateSpec.Reusable):
	// false means a passing record is never reused and the command always runs.
	Reuse bool
}

// GateResult is the outcome of running one gate: the gate name, a pass/fail
// status drawn from the store enum, the repo-relative evidence path the captured
// output was written to, and the command's process exit code.
type GateResult struct {
	Name        string
	Status      string
	EvidenceRef string
	ExitCode    int
}

// Progress receives gate lifecycle events while a check or eval runs. A nil
// Progress is silent. Events are human feedback, never evidence: the recorded
// output file contains exactly what the command emitted.
type Progress interface {
	GateStarted(name, command string)
	GateStillRunning(name string, elapsed time.Duration)
	GateFinished(name, status string, elapsed time.Duration)
}

// heartbeatEvery is how often a still-running gate reports. Long native test
// suites run many minutes; a wedged command is visible within one interval.
// It is a var rather than a const only so a test can override it (the package
// has no injected-clock convention to hook instead); production never assigns
// to it.
var heartbeatEvery = 15 * time.Second

// RunGate shells g.Run via `sh -c` in workdir, captures the combined stdout and
// stderr, and classifies the command as pass or fail. It then writes the output
// to an evidence file under workdir/artifacts/tiller/gates/. workdir is the repo
// root, so the returned EvidenceRef is repo-relative. The command result remains
// available for diagnostics when evidence persistence fails, but callers must
// not record it.
//
// p receives the run's lifecycle: Started when the command launches,
// StillRunning on every heartbeatEvery tick while it is still running, and
// Finished once it exits, classified but before evidence is written (a
// finished gate reports even when the following evidence write then fails). A
// nil p makes RunGate exactly as quiet as it was before progress existed.
func RunGate(g GateConfig, workdir string, p Progress) (GateResult, error) {
	res := GateResult{Name: g.Name}

	var buf bytes.Buffer
	cmd := exec.Command("sh", "-c", g.Run)
	cmd.Dir = workdir
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	start := time.Now()
	if p != nil {
		p.GateStarted(g.Name, g.Run)
	}

	var err error
	if startErr := cmd.Start(); startErr != nil {
		// Classified below exactly like today's non-exit CombinedOutput failure
		// (not an *exec.ExitError, so ExitCode -1): there is no process to wait
		// on, so the heartbeat loop is skipped entirely.
		err = startErr
	} else {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for waiting := true; waiting; {
			select {
			case waitErr := <-done:
				err = waitErr
				waiting = false
			case <-ticker.C:
				if p != nil {
					p.GateStillRunning(g.Name, time.Since(start).Round(time.Second))
				}
			}
		}
	}
	out := buf.Bytes()

	if err == nil {
		res.Status = store.GateStatusPass
		res.ExitCode = 0
	} else {
		res.Status = store.GateStatusFail
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}

	if p != nil {
		p.GateFinished(g.Name, res.Status, time.Since(start).Round(time.Second))
	}

	ref := evidenceRef(g, out)
	res.EvidenceRef = ref
	if writeErr := store.WriteRepositoryFileAtomic(workdir, filepath.FromSlash(ref), out); writeErr != nil {
		return res, &store.EnvError{
			Code:   3,
			Reason: ReasonEvidenceUnwritable,
			Msg:    fmt.Sprintf("gate %s finished but its evidence could not be written to %s: %v", g.Name, ref, writeErr),
		}
	}
	return res, nil
}

// evidenceRef is the repo-relative path a gate's captured output is written to:
// artifacts/tiller/gates/<name>[-<against>]-out-<sha256>.txt. The against token
// makes the evidence traceable to the revision, and the full output digest
// prevents distinct runs at one revision from overwriting each other.
func evidenceRef(g GateConfig, output []byte) string {
	name := g.Name
	if tok := againstToken(g.Against); tok != "" {
		name += "-" + tok
	}
	digest := sha256.Sum256(output)
	return evidenceDir + "/" + name + "-out-" + hex.EncodeToString(digest[:]) + ".txt"
}

// againstToken converts an against revision value into a filename-safe token. A
// bare git short sha passes through unchanged; a scheme-qualified fingerprint
// like sha256:<hash> becomes sha256-<short>, because a colon is unrepresentable
// on Windows filesystems and the full digest is noise in a filename.
func againstToken(against string) string {
	scheme, rest, found := strings.Cut(against, ":")
	if !found {
		return against
	}
	const short = 12
	if len(rest) > short {
		rest = rest[:short]
	}
	return scheme + "-" + rest
}
