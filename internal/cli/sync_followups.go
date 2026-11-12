package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
)

// syncNow is the clock sync dates event compaction by, a seam for tests.
var syncNow = time.Now

// compactEvents is the compaction sync and the upgrade run, a seam so a test
// can make it fail or panic.
var compactEvents = mutate.Compact

// This file holds the best-effort steps sync runs after its own work: agent
// usage capture, then event compaction. Neither can fail sync.

// syncCaptureUsage runs the best-effort agent usage capture after sync's own
// work succeeded and returns the one warning line it earns, or "". When there
// is something to write, the tree is reloaded first, because sync's commit
// advanced the generation and the old tree's commit would be refused as stale.
// A failure here is a warning, never a refusal, and so is a panic: sync has
// already written its own state, so a reader bug must not crash it.
func syncCaptureUsage(tree *store.Tree) (warning string) {
	if !tree.Manifest.Usage.Capture {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			warning = fmt.Sprintf("Agent usage not captured: %v", r)
		}
	}()
	onIntegration, _, integration, err := onIntegrationBranch(tree)
	if err != nil {
		return "Agent usage not captured: " + err.Error()
	}
	if !onIntegration {
		return fmt.Sprintf("Agent usage is captured on %s; skipped here.", integration)
	}
	res, err := captureUsage(tree, time.Now(), 2*time.Second, true)
	if err != nil {
		return "Agent usage not captured: " + err.Error()
	}
	if res.Partial {
		return "Agent usage capture ran out of time; run tiller usage to finish."
	}
	unreadable := 0
	for _, h := range res.Harnesses {
		unreadable += h.Unreadable
	}
	if unreadable > 0 {
		return "Agent usage: " + unreadableLogs(unreadable) + "; run tiller usage for details."
	}
	return ""
}

// syncCompactEvents compacts the event days older than the manifest's delay
// after sync's own work succeeded. It returns nil and no warning off the
// integration branch and when no day was compacted, even if events were
// skipped (tiller compact reports those), so sync stays silent about it. Any
// failure becomes one warning beside whatever days were already committed, and
// so does a panic: sync has already written its own state.
func syncCompactEvents(tree *store.Tree, now time.Time) (res *mutate.CompactResult, warning string) {
	defer func() {
		if r := recover(); r != nil {
			warning = fmt.Sprintf("Events not compacted: %v", r)
		}
	}()
	onIntegration, _, _, err := onIntegrationBranch(tree)
	if err != nil {
		return nil, "Events not compacted: " + err.Error()
	}
	if !onIntegration {
		return nil, ""
	}
	done, err := compactEvents(tree.Root, mutate.CompactOptions{Now: now})
	if len(done.Days) > 0 {
		done = stageable(tree.Root, done)
		res = &done
	}
	if err != nil {
		warning = "Events not compacted: " + strings.Join(refusals(err), "; ")
	}
	return res, warning
}

// withFollowups appends the usage warning, the compaction report, and the
// compaction warning to sync's human output, each as its own block.
func withFollowups(human string, view SyncView) string {
	blocks := []string{view.UsageWarning, view.CompactWarning}
	if view.Compacted != nil {
		blocks = []string{view.UsageWarning, humanCompaction(*view.Compacted, false), view.CompactWarning}
	}
	for _, b := range blocks {
		if b != "" {
			human = joinSyncBlocks(human, b)
		}
	}
	return human
}
