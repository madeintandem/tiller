package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/usage"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file wires `tiller usage` and the capture sync runs after its own work.
// Capture reads the local harness logs, writes one file per session under
// .tiller/usage/, and records no event and no person. It needs no joined
// identity. captureUsage is the one path both callers share.

const (
	reasonUsageCaptureOff     = "usage_capture_off"
	reasonUsageNotIntegration = "not_integration_branch"

	usageOperation = "capture agent usage"
)

// UsageRefusedData is the data block of a refused `tiller usage`.
type UsageRefusedData struct {
	Reason      string `json:"reason"`
	Current     string `json:"current,omitempty"`
	Integration string `json:"integration,omitempty"`
}

// UsageHarnessView is one harness's line in the `tiller usage` data block.
// tied_share is null when the harness recorded no tokens: unknown, not zero.
type UsageHarnessView struct {
	Harness   string   `json:"harness"`
	Found     bool     `json:"found"`
	Sessions  int      `json:"sessions"`
	Written   int      `json:"written"`
	Tokens    int64    `json:"tokens"`
	TiedShare *float64 `json:"tied_share"`
}

// UsageView is the data block of a successful `tiller usage`.
type UsageView struct {
	Harnesses []UsageHarnessView `json:"harnesses"`
	Partial   bool               `json:"partial"`
}

func newUsageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Capture agent token usage from local harness logs.",
		Long: "usage reads this machine's Claude Code, Codex and Pi session logs for sessions that\n" +
			"ran in this checkout or one of its worktrees, and writes one file per session under\n" +
			".tiller/usage/. It records no event and no person, and needs no joined identity.\n" +
			"Capture is off unless .tiller/tiller.yml has usage.capture: true, and it writes\n" +
			"only on the integration branch.",
		Args: cobra.NoArgs,
		RunE: runUsage,
	}
}

func runUsage(cmd *cobra.Command, _ []string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "usage", err)
	}
	if !tree.Manifest.Usage.Capture {
		return emitReport(cmd, "usage", false, UsageRefusedData{Reason: reasonUsageCaptureOff},
			"Agent usage capture is off. Set usage.capture: true in .tiller/tiller.yml to turn it on.", 1)
	}
	onIntegration, current, integration, err := usageBranch(tree)
	if err != nil {
		return emitFailure(cmd, "usage", err)
	}
	if !onIntegration {
		return emitReport(cmd, "usage", false,
			UsageRefusedData{Reason: reasonUsageNotIntegration, Current: current, Integration: integration},
			fmt.Sprintf("Agent usage is captured on the integration branch only. You are on %s; check out %s and run this again.", current, integration), 1)
	}
	res, err := captureUsage(tree, time.Now(), 0)
	if err != nil {
		var env *store.EnvError
		if errors.As(err, &env) && env.Reason == store.ReasonMutationStale {
			return emitReport(cmd, "usage", false, UsageRefusedData{Reason: store.ReasonMutationStale},
				"Tiller state changed while usage was being captured. Run tiller usage again.", 1)
		}
		return emitFailure(cmd, "usage", err)
	}
	return emitReport(cmd, "usage", true, usageView(res), humanUsage(res), 0)
}

// usageBranch says whether the checkout is on the branch capture may write
// on: the declared integration branch, or any branch when none is declared.
func usageBranch(tree *store.Tree) (ok bool, current, integration string, err error) {
	integration = tree.Manifest.Git.IntegrationBranch
	if integration == "" {
		return true, "", "", nil
	}
	current, err = vcs.New(tree.Root).Branch()
	if err != nil {
		return false, "", integration, err
	}
	return current == integration, current, integration, nil
}

// captureUsage runs one capture against tree and commits what it planned. The
// cursor is saved only after the commit succeeds, or when there was nothing to
// commit. It is best effort for sync and the whole job for `tiller usage`.
func captureUsage(tree *store.Tree, now time.Time, budget time.Duration) (usage.Result, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return usage.Result{}, err
	}
	project := []string{tree.Root}
	if paths, err := vcs.New(tree.Root).WorktreePaths(); err == nil {
		for _, p := range paths {
			p = filepath.Clean(p)
			if !samePath(p, tree.Root) {
				project = append(project, p)
			}
		}
	}
	var since time.Time
	evs, err := events.List(tree)
	if err != nil {
		return usage.Result{}, err
	}
	if len(evs) > 0 {
		since = evs[0].At
	}
	res, cursor, err := usage.Capture(usage.Options{
		Root:  tree.Root,
		Roots: usage.Roots{Home: home, Env: os.Getenv, Project: project},
		Readers: []usage.Reader{
			usage.NewClaudeReader(), usage.NewCodexReader(), usage.NewPiReader(),
		},
		Resolve: usageResolver(tree),
		Now:     now,
		Since:   since,
		Budget:  budget,
	})
	if err != nil {
		return usage.Result{}, err
	}
	if len(res.Writes) > 0 {
		if err := tree.Commit(usageOperation, res.Writes); err != nil {
			return usage.Result{}, err
		}
	}
	if err := cursor.Save(tree.Root); err != nil {
		return usage.Result{}, err
	}
	return res, nil
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// usageResolver maps a node id to itself and a node key to its id; a shelf
// key answers with or without its bugs/, chores/ or tweaks/ prefix. An unknown
// ref answers "".
func usageResolver(tree *store.Tree) usage.Resolver {
	m := map[string]string{}
	add := func(n store.Node) {
		id := n.NodeID()
		m[id] = id
		if k := n.NodeKey(); k != nil && *k != "" {
			m[*k] = id
			for _, noun := range []string{store.NounBug, store.NounChore, store.NounTweak} {
				if local, ok := strings.CutPrefix(*k, store.ShelfDir(noun)+"/"); ok {
					m[local] = id
				}
			}
		}
	}
	for _, n := range tree.Epics {
		add(n)
	}
	for _, n := range tree.Features {
		add(n)
	}
	for _, n := range tree.Tasks {
		add(n)
	}
	for _, n := range tree.Bugs {
		add(n)
	}
	for _, n := range tree.Chores {
		add(n)
	}
	for _, n := range tree.Tweaks {
		add(n)
	}
	return func(ref string) string { return m[ref] }
}

func usageView(res usage.Result) UsageView {
	v := UsageView{Harnesses: []UsageHarnessView{}, Partial: res.Partial}
	for _, h := range res.Harnesses {
		hv := UsageHarnessView{Harness: h.Harness, Found: h.Found, Sessions: h.Sessions, Written: h.Written, Tokens: h.Tokens}
		if h.Tokens > 0 {
			share := float64(h.Tied) / float64(h.Tokens)
			hv.TiedShare = &share
		}
		v.Harnesses = append(v.Harnesses, hv)
	}
	return v
}

var usageHarnessLabel = map[string]string{"claude-code": "Claude Code", "codex": "Codex", "pi": "Pi"}

func humanUsage(res usage.Result) string {
	var lines []string
	for _, h := range res.Harnesses {
		label := usageHarnessLabel[h.Harness]
		if label == "" {
			label = h.Harness
		}
		if !h.Found {
			lines = append(lines, label+": no session logs found")
			continue
		}
		noun := "sessions"
		if h.Sessions == 1 {
			noun = "session"
		}
		line := fmt.Sprintf("%s: %d %s, %d written, %s tokens", label, h.Sessions, noun, h.Written, formatTokens(h.Tokens))
		if h.Tokens > 0 {
			line += fmt.Sprintf(", %d%% tied to a work item", int(float64(h.Tied)*100/float64(h.Tokens)+0.5))
		}
		lines = append(lines, line)
	}
	if res.Partial {
		lines = append(lines, "Capture ran out of time; run tiller usage again to finish.")
	}
	return strings.Join(lines, "\n")
}

// formatTokens renders a token count compactly: 950, 412K, 8.9M, 186M, 1.2B.
// A value under 10 of its unit keeps one decimal.
func formatTokens(n int64) string {
	units := []struct {
		div    float64
		suffix string
	}{{1, ""}, {1e3, "K"}, {1e6, "M"}, {1e9, "B"}}
	f := float64(n)
	i := 0
	for i < len(units)-1 && f >= units[i+1].div*0.9995 {
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d", n)
	}
	v := f / units[i].div
	if v < 9.95 {
		return fmt.Sprintf("%.1f%s", v, units[i].suffix)
	}
	return fmt.Sprintf("%.0f%s", v, units[i].suffix)
}
