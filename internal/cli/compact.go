package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file wires `tiller compact`. Compaction folds old loose event files
// into immutable day segments, records no event, and needs no joined identity.

const reasonCompactNotIntegration = "not_integration_branch"

// compactNow is the verb's clock, a seam for tests.
var compactNow = time.Now

// CompactRefusedData is the data block of a refused `tiller compact`.
type CompactRefusedData struct {
	Reason      string `json:"reason"`
	Current     string `json:"current,omitempty"`
	Integration string `json:"integration,omitempty"`
}

// CompactView is the data block of a `tiller compact` that folded days: the
// compaction result, plus dry_run on a dry run, and refused naming the days
// that refused beside them.
type CompactView struct {
	mutate.CompactResult
	DryRun  bool     `json:"dry_run,omitempty"`
	Refused []string `json:"refused,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

func newCompactCmd() *cobra.Command {
	var all, dryRun bool
	cmd := &cobra.Command{
		Use:   "compact",
		Short: "Fold old event files into immutable day segments.",
		Long: "compact folds each day of loose event files older than events.compact_after_days\n" +
			"(default 7 whole UTC days) in .tiller/tiller.yml into one immutable segment beside\n" +
			"the day directory, and removes the loose files. It records no event, needs no joined\n" +
			"identity, and runs on the integration branch only. It leaves the changes unstaged\n" +
			"and prints the git add command to stage them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCompact(cmd, all, dryRun)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "compact every day, today included, ignoring the delay")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be compacted and write nothing")
	return cmd
}

func runCompact(cmd *cobra.Command, all, dryRun bool) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "compact", err)
	}
	onIntegration, current, integration, err := onIntegrationBranch(tree)
	if err != nil {
		return emitFailure(cmd, "compact", err)
	}
	if !onIntegration {
		return emitReport(cmd, "compact", false,
			CompactRefusedData{Reason: reasonCompactNotIntegration, Current: current, Integration: integration},
			fmt.Sprintf("Events are compacted on the integration branch only. You are on %s; check out %s and run this again.", current, integration), 1)
	}
	res, err := compactEvents(tree.Root, mutate.CompactOptions{Now: compactNow(), All: all, DryRun: dryRun})
	if err != nil {
		var env *store.EnvError
		if errors.As(err, &env) && env.Reason == store.ReasonMutationStale {
			human := "Tiller state changed while events were being compacted. Run tiller compact again."
			if len(res.Days) == 0 {
				return emitReport(cmd, "compact", false, CompactRefusedData{Reason: store.ReasonMutationStale}, human, 1)
			}
			res = stageable(tree.Root, res)
			return emitReport(cmd, "compact", false,
				CompactView{CompactResult: res, DryRun: dryRun, Refused: refusals(err), Reason: store.ReasonMutationStale},
				humanCompaction(res, dryRun)+"\n"+human, 1)
		}
		if len(res.Days) == 0 {
			return emitFailure(cmd, "compact", err)
		}
		// Newer days compacted past the ones that refused: the command fails,
		// but still hands over what it folded and the line that stages it.
		res = stageable(tree.Root, res)
		refused := refusals(err)
		human := humanCompaction(res, dryRun) + "\nSome days were not compacted: " + strings.Join(refused, "; ")
		return emitReport(cmd, "compact", false, CompactView{CompactResult: res, DryRun: dryRun, Refused: refused}, human, classify(err).Code)
	}
	res = stageable(tree.Root, res)
	human := humanCompaction(res, dryRun)
	if len(res.Days) == 0 {
		human = strings.TrimSpace("Nothing to compact.\n" + human)
	}
	return emitReport(cmd, "compact", true, CompactView{CompactResult: res, DryRun: dryRun}, human, 0)
}

// stageable makes res.Stage the paths a person can paste into git add from
// where tiller ran. It drops each removed path, a day directory or a loose
// file, that git tracks nothing at: one that was never committed has no
// deletion to stage, and naming it would make git add refuse the whole
// command, segments included. A git that cannot answer keeps every path. It
// then names each path from the working directory (stagePath), so the line
// runs as printed when the project sits below the repository root or tiller
// ran below the project root. Every caller filters before printing or
// returning the list, so human and JSON agree.
func stageable(root string, res mutate.CompactResult) mutate.CompactResult {
	var removed []string
	for _, p := range res.Stage {
		if !events.IsSegmentPath(p) {
			removed = append(removed, p)
		}
	}
	tracked, err := vcs.New(root).Tracked(removed)
	var stage []string
	for _, p := range res.Stage {
		if err == nil && !events.IsSegmentPath(p) && !tracked[p] {
			continue
		}
		stage = append(stage, stagePath(root, p))
	}
	res.Stage = stage
	return res
}

// refusals splits a compaction error into one message per refused day, so a
// report can list them or keep them on one line.
func refusals(err error) []string {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	for _, e := range joined.Unwrap() {
		out = append(out, e.Error())
	}
	return out
}

// stagePath names p, relative to root, from the process's working directory,
// or leaves it as it is when the two cannot be related.
func stagePath(root, p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, filepath.Join(root, filepath.FromSlash(p)))
	if err != nil {
		return p
	}
	return filepath.ToSlash(rel)
}

// restage names the stage paths of a compaction a maintenance child reported
// from this process's working directory. The child ran in root, so it named
// them from there.
func restage(root string, c *mutate.CompactResult) {
	if c == nil {
		return
	}
	for i, p := range c.Stage {
		c.Stage[i] = stagePath(root, p)
	}
}

// humanCompaction renders a compaction for people: what was folded and the git
// add command that stages it, or, on a dry run, what would be, then how many
// events stay loose. It is "" when no day was eligible and nothing was
// skipped. sync, migrate and upgrade print it after their own work.
func humanCompaction(res mutate.CompactResult, dryRun bool) string {
	var lines []string
	switch {
	case len(res.Days) > 0 && !dryRun && len(res.Stage) > 0:
		words := make([]string, len(res.Stage))
		for i, p := range res.Stage {
			words[i] = shellWord(p)
		}
		lines = append(lines, compactionHeadline(res, dryRun)+" Stage them with:", "  git add -A -- "+strings.Join(words, " "))
	case len(res.Days) > 0:
		lines = append(lines, compactionHeadline(res, dryRun))
	}
	switch {
	case res.Skipped == 1:
		lines = append(lines, "1 event could not be compacted and stays as its own file.")
	case res.Skipped > 1:
		lines = append(lines, fmt.Sprintf("%d events could not be compacted and stay as their own files.", res.Skipped))
	}
	return strings.Join(lines, "\n")
}

// compactionHeadline is the first line of a compaction that folded days.
func compactionHeadline(res mutate.CompactResult, dryRun bool) string {
	segments := 0
	for _, d := range res.Days {
		if d.Segment != "" {
			segments++
		}
	}
	switch {
	case segments == 0 && dryRun:
		return fmt.Sprintf("Would remove %s already held in segments.", count(res.Events, "loose event"))
	case segments == 0:
		return fmt.Sprintf("Removed %s already held in segments.", count(res.Events, "loose event"))
	case dryRun:
		return fmt.Sprintf("Would compact %s of events (%s) into %s.",
			count(len(res.Days), "day"), count(res.Events, "event"), count(segments, "file"))
	}
	return fmt.Sprintf("Compacted %s of events (%s) into %s.",
		count(len(res.Days), "day"), count(res.Events, "event"), count(segments, "file"))
}

// count renders n with noun, pluralized with an s unless n is 1.
func count(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
