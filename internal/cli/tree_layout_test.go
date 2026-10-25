package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/stretchr/testify/require"
)

// These tests pin the tree layout at real-tree scale (issue #18): the label
// column sizes itself to this tree's longest label instead of a fixture-sized
// constant, titles truncate to the line budget while labels never do, the
// prose paths live in the --files drill view rather than on every row, and
// vertical bars carry the spine under a parent that has later siblings.
// renderTreeHuman is pure in (model, flags, width), so the layouts are pinned
// directly, without a terminal.

// layoutModel builds a two-epic model shaped like the field data that broke
// the fixed columns: one long real-world feature key, one sentence title.
func layoutModel() gen.TreeModel {
	return gen.TreeModel{
		Project: "acme",
		Nodes: []gen.TreeNode{
			{
				Noun: "epic", ID: "E-aaaaaa", Key: "audit-engine", Title: "Audit Engine", Status: "draft",
				Path:  ".tiller/epics/audit-engine/epic.md",
				Files: []string{".tiller/epics/audit-engine/epic.md", ".tiller/epics/audit-engine/epic.yml"},
				Children: []gen.TreeNode{
					{
						Noun: "feature", ID: "F-bbbbbb", Key: "audit-engine/audit-lifecycle-state-machine",
						Title:  "aasm audit lifecycle with policy guards and audit-log events",
						Status: "draft",
						Path:   ".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/feature.md",
						Files:  []string{".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/feature.md"},
						Children: []gen.TreeNode{
							{Noun: "task", ID: "T-cccccc", Key: "audit-engine/audit-lifecycle-state-machine/wire-the-state-machine",
								Title: "Wire the state machine", Status: "proposed",
								Path: ".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/tasks/wire-the-state-machine/task.md",
								Files: []string{
									".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/tasks/wire-the-state-machine/task.md",
									".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/tasks/wire-the-state-machine/task.yml",
								},
								Children: []gen.TreeNode{}},
						},
					},
				},
			},
			{
				Noun: "epic", ID: "E-dddddd", Key: "form-engine", Title: "Form Engine", Status: "draft",
				Path:     ".tiller/epics/form-engine/epic.md",
				Files:    []string{".tiller/epics/form-engine/epic.md"},
				Children: []gen.TreeNode{},
			},
		},
	}
}

// TestTreeDefaultOmitsPaths pins the issue-#18 core: the default map carries
// no file paths (the nesting and labels already encode them), and every path
// appears under --files instead, the prose .md included.
func TestTreeDefaultOmitsPaths(t *testing.T) {
	m := layoutModel()

	def := renderTreeHuman(m, treeFlags{}, 120)
	require.NotContains(t, def, ".tiller/", "the default map spends no columns on paths")

	files := renderTreeHuman(m, treeFlags{files: true}, 120)
	for _, n := range []string{
		".tiller/epics/audit-engine/epic.md",
		".tiller/epics/audit-engine/epic.yml",
		".tiller/epics/audit-engine/features/audit-lifecycle-state-machine/feature.md",
	} {
		require.Contains(t, files, n, "--files is the drill view and carries the full footprint")
	}
}

// TestTreeSizesLabelColumnToItsOwnTree pins the per-tree column: titles start
// two columns after this tree's widest left part, so real keys align instead
// of overflowing a fixture-sized constant.
func TestTreeSizesLabelColumnToItsOwnTree(t *testing.T) {
	out := renderTreeHuman(layoutModel(), treeFlags{}, 200)
	lines := strings.Split(out, "\n")

	// The widest left part is the long feature row:
	// "   ├─ audit-lifecycle-state-machine (F-bbbbbb)" = 46 runes, titles at 48.
	want := utf8.RuneCountInString("   ├─ audit-lifecycle-state-machine (F-bbbbbb)") + 2
	for _, l := range lines {
		for _, title := range []string{"Audit Engine", "Form Engine", "aasm audit lifecycle"} {
			if i := strings.Index(l, title); i >= 0 {
				require.Equal(t, want, utf8.RuneCountInString(l[:i]),
					"every title starts at the shared column: %q", l)
			}
		}
	}
}

// TestTreeTruncatesTitlesNeverLabels pins the width budget: at a narrow width
// the sentence title is cut with an ellipsis so the line fits, while the label
// (the address) always renders whole.
func TestTreeTruncatesTitlesNeverLabels(t *testing.T) {
	out := renderTreeHuman(layoutModel(), treeFlags{}, 60)
	require.Contains(t, out, "audit-lifecycle-state-machine (F-bbbbbb)", "labels are addresses and never truncate")
	require.Contains(t, out, "…", "a title that cannot fit is cut with an ellipsis")
	require.NotContains(t, out, "policy guards and audit-log events", "the long title no longer fits at width 60")
	for _, l := range strings.Split(out, "\n") {
		require.LessOrEqual(t, utf8.RuneCountInString(l), 60, "no line exceeds the budget: %q", l)
	}
}

// TestTreeCarriesContinuationBars pins the spine: descendants of a parent with
// later siblings render under a vertical bar, and descendants of the last
// sibling indent with plain spaces.
func TestTreeCarriesContinuationBars(t *testing.T) {
	out := renderTreeHuman(layoutModel(), treeFlags{}, 120)
	require.Contains(t, out, "│  └─ audit-lifecycle-state-machine", "audit-engine has a later sibling, so its children ride the bar")
	require.Contains(t, out, "│     └─ wire-the-state-machine", "the epic's bar carries through, and the last-child feature adds spaces, not a bar")
	require.NotContains(t, out, "\n│  └─ form-engine", "a top-level last sibling carries no bar of its own")
}

// TestTreeStatusColumnSizesToItsTree pins --status at the new layout: the
// status column sits between the labels and the titles, sized to this tree's
// widest status text.
func TestTreeStatusColumnSizesToItsTree(t *testing.T) {
	out := renderTreeHuman(layoutModel(), treeFlags{status: true}, 200)
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "T-cccccc") {
			require.Contains(t, l, "proposed", "the task row carries its status")
			require.Regexp(t, `proposed\s+Wire the state machine`, l, "title follows the status column")
		}
	}
}

// TestTreePipedBudgetIs120 pins the documented piped line budget through the
// real command path: with stdout not a terminal, a title crossing column 120
// is cut with the ellipsis at exactly 120 runes. This is the determinism
// promise redirected output and the goldens rest on; the fixture goldens are
// too narrow to exercise it on their own.
func TestTreePipedBudgetIs120(t *testing.T) {
	layMapTree(t)
	mustRun(t, "add", "feature", "foundation/wide", "--title", strings.Repeat("wide title ", 14), "--contributor", "jc")

	out, _, code := execCLI(t, "tree")
	require.Equal(t, 0, code)
	var hit bool
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "wide (F-") {
			hit = true
			require.Equal(t, 120, utf8.RuneCountInString(l), "the piped budget is the documented 120 runes: %q", l)
			require.True(t, strings.HasSuffix(l, "…"), "the cut is marked: %q", l)
		}
	}
	require.True(t, hit, "the wide-title row rendered")
}

// TestTreeFilesRideTheBars pins the drill view's shape: a node's files indent
// beneath it and inherit the vertical bar context, so the spine stays readable
// with the footprint expanded.
func TestTreeFilesRideTheBars(t *testing.T) {
	out := renderTreeHuman(layoutModel(), treeFlags{files: true}, 120)
	require.Contains(t, out, "│     .tiller/epics/audit-engine/epic.md", "a non-last node's files ride its bar")
	require.Contains(t, out, "\n      .tiller/epics/form-engine/epic.md", "the last node's files indent with plain spaces")
}
