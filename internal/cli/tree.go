package cli

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// This file implements `tiller tree [<noun> <node>]`, the read-only navigable map
// of the work tree (epic -> feature -> task). It is a thin shell: load the
// tree, build the model in gen (the pure projection), and render it as a human
// ASCII tree or the structured --json envelope. Like status, it is bare-or-node:
// bare maps the whole project, an optional epic or feature roots the subtree.

// treePipedWidth is the line budget when stdout is not a terminal, so piped
// and redirected output (and the goldens) stays deterministic. At a terminal
// the budget is the terminal's own width (real trees average 144
// columns under the old fixed layout and drowned a 13-inch screen).
const treePipedWidth = 120

// treeFlags are the rendering options. --files expands each node to its full file
// footprint; --status annotates each node with its status (and an epic's derived
// activity). Neither changes the --json model, which always carries both.
type treeFlags struct {
	files  bool
	status bool
}

func newTreeCmd() *cobra.Command {
	var f treeFlags
	cmd := &cobra.Command{
		Use:   "tree [<noun> <node>]",
		Short: "Print the work tree: epics, features, and tasks (--files adds paths).",
		Long: "tree with no node maps the whole project: every epic, its features, and\n" +
			"their tasks, in dependency order with key order breaking ties, each line\n" +
			"carrying the node's local key segment and title, sized to the terminal\n" +
			"(piped output uses a fixed 120-column budget). tree <noun> <node> roots\n" +
			"the map at one epic or feature.\n" +
			"--files is the drill view: it expands each node to its full file\n" +
			"footprint, its own prose .md first, then its state .yml, then a\n" +
			"feature's contract. --status annotates each node with its status.",
		Args: treeArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTree(cmd, args, f)
		},
	}
	cmd.Flags().BoolVar(&f.files, "files", false, "expand each node to its full file footprint")
	cmd.Flags().BoolVar(&f.status, "status", false, "annotate each node with its status")
	return cmd
}

// treeArgs accepts exactly the two legal shapes: no args (whole project) or a
// noun and a node (the rooted subtree). A lone noun is arg-count misuse, the same
// as status.
func treeArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || len(args) == 2 {
		return nil
	}
	return fmt.Errorf("tree takes either no arguments (whole project) or <noun> <node>")
}

func runTree(cmd *cobra.Command, args []string, f treeFlags) error {
	if len(args) == 0 {
		tree, err := loadTree(cmd)
		if err != nil {
			return emitFailure(cmd, "tree", err)
		}
		model := gen.Tree(tree)
		return emitSuccess(cmd, "tree", model, renderTreeHuman(model, f, treeWidth(cmd)))
	}

	tree, n, err := resolveNodeWithTree(cmd, "tree", args[0], args[1])
	if err != nil {
		return err
	}
	model, ok := gen.Subtree(tree, n)
	if !ok {
		return emitFailure(cmd, "tree", &usageError{
			reason: "unscoped_noun",
			msg:    fmt.Sprintf("tree roots at an epic or a feature, not a %s", n.Noun()),
		})
	}
	return emitSuccess(cmd, "tree", model, renderTreeHuman(model, f, treeWidth(cmd)))
}

// treeWidth is the human map's line budget: the terminal's width when stdout
// is a terminal, and the fixed piped default otherwise, so non-interactive
// output stays deterministic.
func treeWidth(cmd *cobra.Command) int {
	if f, ok := cmd.OutOrStdout().(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return treePipedWidth
}

// renderTreeHuman renders the model as a plain ASCII tree sized to itself and
// to the line budget: the project key on the first line, then each top-level
// node and its descendants. The title column starts after this tree's widest
// label (per-tree column math, not a constant, since real keys defeat any
// fixture-sized width), titles truncate to the budget, and file paths appear
// only under --files, indented beneath their node (the nesting and the labels
// already encode the path, so the default map spends no columns on it).
// emitSuccess adds the trailing newline, so the rendered string carries none.
func renderTreeHuman(m gen.TreeModel, f treeFlags, width int) string {
	l := measureTree(m, f)
	lines := []string{m.Project}
	for i, n := range m.Nodes {
		lines = appendTreeLines(lines, n, "", i == len(m.Nodes)-1, f, l, width)
	}
	return strings.Join(lines, "\n")
}

// treeLayout is the per-tree column math: where the status text and the titles
// start, computed from the widest rows of the tree being rendered.
type treeLayout struct {
	statusStart int // column where the status text starts (--status only)
	titleStart  int // column where titles start
}

// measureTree walks the model the same way the renderer will and sizes the
// columns to this tree's own widest label (and, under --status, its widest
// status text), with a two-column gutter between columns.
func measureTree(m gen.TreeModel, f treeFlags) treeLayout {
	maxLeft, maxStatus := 0, 0
	var walk func(n gen.TreeNode, depth int)
	walk = func(n gen.TreeNode, depth int) {
		// indent and connector are three runes per level.
		left := 3*depth + 3 + utf8.RuneCountInString(treeLabel(n))
		if left > maxLeft {
			maxLeft = left
		}
		if s := utf8.RuneCountInString(treeStatusText(n)); s > maxStatus {
			maxStatus = s
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, n := range m.Nodes {
		walk(n, 0)
	}
	l := treeLayout{statusStart: maxLeft + 2, titleStart: maxLeft + 2}
	if f.status {
		l.titleStart = l.statusStart + maxStatus + 2
	}
	return l
}

// appendTreeLines appends a node's line (and, under --files, its file lines)
// and then recurses into its children. Connectors mark the last sibling, and a
// parent with later siblings carries a vertical bar under it so the spine
// stays readable at depth.
func appendTreeLines(lines []string, n gen.TreeNode, indent string, last bool, f treeFlags, l treeLayout, width int) []string {
	connector := "├─ "
	if last {
		connector = "└─ "
	}
	lines = append(lines, formatTreeLine(indent+connector+treeLabel(n), n, f, l, width))

	childIndent := indent + "│  "
	if last {
		childIndent = indent + "   "
	}

	if f.files {
		// The drill view: the node's full footprint, the prose path first,
		// indented beneath the node and riding its bar context. Paths are the
		// point here, so they render whole and may overrun a narrow budget.
		for _, file := range n.Files {
			lines = append(lines, childIndent+"   "+file)
		}
	}

	for i, c := range n.Children {
		lines = appendTreeLines(lines, c, childIndent, i == len(n.Children)-1, f, l, width)
	}
	return lines
}

// formatTreeLine lays one node line into the tree's computed columns: the tree
// label, the status (under --status), and the title, truncated to the line
// budget. The label is the node's address and never truncates. Trailing
// padding is trimmed so lines carry no dangling whitespace.
func formatTreeLine(left string, n gen.TreeNode, f treeFlags, l treeLayout, width int) string {
	var b strings.Builder
	if f.status {
		b.WriteString(col(left, l.statusStart))
		b.WriteString(col(treeStatusText(n), l.titleStart-l.statusStart))
	} else {
		b.WriteString(col(left, l.titleStart))
	}
	title := n.Title
	if len(n.Contributors) > 0 {
		ids := make([]string, len(n.Contributors))
		for i, contributor := range n.Contributors {
			ids[i] = contributor.ID
		}
		title += " [contributors: " + strings.Join(ids, ", ") + "]"
	}
	b.WriteString(truncate(title, width-utf8.RuneCountInString(b.String())))
	return strings.TrimRight(b.String(), " ")
}

// truncate fits s into width display runes, marking a cut with an ellipsis.
// Only prose passes through here; a label is an address and is never cut.
func truncate(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width <= 0 {
		return ""
	}
	// A one-column budget still gets the ellipsis: the cut is signalled, the
	// title never silently vanishes.
	r := []rune(s)
	return string(r[:width-1]) + "…"
}

// treeLabel is the human handle for a node: its local key (the last segment, so
// a feature reads as its own name rather than the epic-qualified key, since the
// nesting already shows the path) with the id in parentheses, per the shared
// reference convention. A node with no key stays its bare id.
func treeLabel(n gen.TreeNode) string {
	if n.Key == "" {
		return n.ID
	}
	local := n.Key
	if i := strings.LastIndex(n.Key, "/"); i >= 0 {
		local = n.Key[i+1:]
	}
	return fmt.Sprintf("%s (%s)", local, n.ID)
}

// treeStatusText is the --status annotation: a node's stored status, with an
// epic's derived working status appended ("ready, active") when it is being
// worked, matching the status oracle.
func treeStatusText(n gen.TreeNode) string {
	if n.Noun == store.NounEpic && n.Activity == gen.EpicActivityActive {
		return n.Status + ", active"
	}
	return n.Status
}

// col left-justifies s into a fixed-width column, measured in display runes so
// the box-drawing connectors count as one column each. It guarantees at least one
// space of separation when s meets or overruns the width, so neighboring columns
// never run together.
func col(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s + " "
}
