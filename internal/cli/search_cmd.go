package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/spf13/cobra"
)

// This file implements `tiller search <query>`, the keyword read
// (cli-surface.md, "Inspecting and maintaining") behind the console's Explore
// search (ui-surface.md, "Explore"): a lookup by id, key, or reference plus a
// case-insensitive scan over .tiller/ markdown and the tree's
// human-meaningful fields, answered as one ranked result per node and one per
// unowned document. It is a project-level read, so it takes no owner.

func newSearchCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Keyword search across .tiller/.",
		Long: "search finds nodes and documents under .tiller/. A query that is a\n" +
			"node's id, id suffix, key, or full reference names that node, and it\n" +
			"comes first. Otherwise the query is a case-insensitive keyword matched\n" +
			"against titles, keys, question statements and answers, and every\n" +
			"regular UTF-8 markdown file under .tiller/.\n\n" +
			"Results are one per node, ranked id, key, title, key fragment, then\n" +
			"prose-only by how often the node matches, each with its first few\n" +
			"matching lines. Prose no node owns is grouped by file after the nodes.\n" +
			"--kind keeps one kind: epic, feature, task, bug, chore, tweak,\n" +
			"question, amendment, or document. An unsafe or unreadable corpus\n" +
			"refuses the whole search; partial results are never returned.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], kind)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "keep only results of this kind")
	return cmd
}

func runSearch(cmd *cobra.Command, query, kind string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "search", err)
	}
	result, err := search.Search(tree.Root, tree, query, search.Options{Kind: kind})
	if stableErr := tree.CheckStable(); stableErr != nil {
		return emitFailure(cmd, "search", stableErr)
	}
	if err != nil {
		return emitFailure(cmd, "search", err)
	}
	return emitSuccess(cmd, "search", result, renderSearchHuman(result))
}

// renderSearchHuman prints one block per result, blank-line separated, in the
// ranked order. A node's head line is its reference, its title, and a bracket
// naming its kind, status, and how it matched; a document's head line is its
// path and match count. Beneath each, its kept hits print as path:line and
// the snippet (a ledger field prints its field name instead of a line), then
// how many more matches the result holds. An empty result names the query,
// an answer rather than a shrug (exit 0), and a capped result says so last.
func renderSearchHuman(r search.Result) string {
	if r.Count == 0 {
		return fmt.Sprintf("No matches for %q.", r.Query)
	}
	blocks := make([]string, 0, len(r.Nodes)+len(r.Documents)+1)
	for _, n := range r.Nodes {
		head := n.Ref
		if n.Title != "" {
			head += "  " + n.Title
		}
		head += fmt.Sprintf("  [%s · %s · %s match]", n.Noun, n.Status, strings.ReplaceAll(n.Match, "_", " "))
		blocks = append(blocks, searchBlock(head, n.Hits, n.Count))
	}
	for _, d := range r.Documents {
		head := fmt.Sprintf("%s  [document · %s]", d.Path, matchCount(d.Count))
		blocks = append(blocks, searchBlock(head, d.Hits, d.Count))
	}
	if r.Truncated {
		blocks = append(blocks, fmt.Sprintf("(showing %d of %d results; narrow the query or pass --kind to see the rest)",
			len(r.Nodes)+len(r.Documents), r.Count))
	}
	return strings.Join(blocks, "\n\n")
}

func searchBlock(head string, hits []search.Hit, count int) string {
	lines := []string{head}
	for _, h := range hits {
		at := fmt.Sprintf("%s:%d", h.Path, h.Line)
		if h.Field != "" {
			at = h.Field
		}
		lines = append(lines, fmt.Sprintf("    %s  %s", at, h.Snippet))
	}
	if more := count - len(hits); more > 0 {
		lines = append(lines, fmt.Sprintf("    (+%d more)", more))
	}
	return strings.Join(lines, "\n")
}

// matchCount is "1 match" or "N matches".
func matchCount(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}
