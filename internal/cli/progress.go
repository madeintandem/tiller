package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/spf13/cobra"
)

// This file renders gate lifecycle events (gates.Progress) as plain-text lines
// for a terminal reader (issue #45): a long native build or test suite prints
// nothing until it finishes today, which makes a healthy multi-minute run
// indistinguishable from a wedged one. The lines are human feedback only. They
// go to stderr, never stdout, and never reach the recorded evidence file:
// gates.RunGate writes exactly what the gated command emitted.
//
// --json is machine-clean: newProgress returns nil under --json, so no line is
// ever rendered and stderr stays empty.

// stderrProgress writes gate lifecycle lines to w. Every check and eval call
// site builds one only in human mode; --json passes nil instead.
type stderrProgress struct {
	w io.Writer
}

// newProgress builds the human-mode gate progress sink for cmd, or nil under
// --json, where a progress line would contaminate the machine-clean envelope.
func newProgress(cmd *cobra.Command) gates.Progress {
	if jsonOut(cmd) {
		return nil
	}
	return &stderrProgress{w: cmd.ErrOrStderr()}
}

// GateStarted announces a gate has begun running its command: "gate build:
// running (bin/ci build)...".
func (p *stderrProgress) GateStarted(name, command string) {
	fmt.Fprintf(p.w, "gate %s: running (%s)...\n", name, command)
}

// GateStillRunning is the heartbeat: "gate build: still running (45s)". It
// names elapsed time so a wedged run is distinguishable from a healthy long
// one, rather than reporting nothing until the gate finishes.
func (p *stderrProgress) GateStillRunning(name string, elapsed time.Duration) {
	fmt.Fprintf(p.w, "gate %s: still running (%s)\n", name, elapsed)
}

// GateFinished announces a gate's terminal status and total elapsed time:
// "gate build: pass (2m10s)".
func (p *stderrProgress) GateFinished(name, status string, elapsed time.Duration) {
	fmt.Fprintf(p.w, "gate %s: %s (%s)\n", name, status, elapsed)
}
