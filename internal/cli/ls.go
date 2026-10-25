package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file implements `tiller ls <noun> [filters]`, the list read
// (cli-surface.md, "Inspecting and maintaining"). The noun is plural here
// (`ls features`, `ls tasks`), the one place the grammar reads as a collection.
// Filters narrow by the node fields a caller browses on: --status, --owner, and
// parentage. Output is sorted so two runs print the same bytes.

// pluralNouns maps the collection noun ls accepts onto the singular node class.
// Plans are addressed by filename rather than as a node and are not listed here
// yet; `ls plans` lands when plan files do.
var pluralNouns = map[string]string{
	"epics":      store.NounEpic,
	"features":   store.NounFeature,
	"tasks":      store.NounTask,
	"questions":  store.NounQuestion,
	"amendments": store.NounAmendment,
	"bugs":       store.NounBug,
	"chores":     store.NounChore,
	"tweaks":     store.NounTweak,
}

// resolveListNoun resolves the noun ls was given to its canonical singular node
// class, accepting both the plural collection form (`features`) and the singular
// (`feature`), so the noun grammar is uniform with add/show/status/spec,
// which all take the singular.
func resolveListNoun(noun string) (string, bool) {
	if sing, ok := pluralNouns[noun]; ok {
		return sing, true
	}
	return singularNoun(noun)
}

// The list shapes and their projections live in internal/views, one source
// the CLI and the serve console both render.
type (
	ListResult = views.ListResult
	ListItem   = views.ListItem
)

// lsFilters is the shared filter set (views.ListFilters): empty fields mean
// no filter on that axis, and the parentage filters resolve through
// views.ResolveParentFilters before matching.
type lsFilters = views.ListFilters

func newLsCmd() *cobra.Command {
	var f lsFilters
	cmd := &cobra.Command{
		Use:   "ls <noun> [flags]",
		Short: "List nodes of a noun, optionally filtered.",
		Long: "ls lists the nodes of a noun (features, tasks, epics, questions,\n" +
			"amendments), in either the plural or the singular form, narrowed by\n" +
			"--status, --owner, --contributor, or a parent. Epics, features, and tasks are listed\n" +
			"in dependency order, key order breaking ties; questions by id;\n" +
			"amendments by key. The same tree always lists in the same order.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd, args[0], f)
		},
	}
	cmd.Flags().StringVar(&f.Status, "status", "", "only nodes in this lifecycle state (unfinished selects nonterminal task, bug, chore, or tweak work)")
	cmd.Flags().StringVar(&f.Owner, "owner", "", "only nodes owned by this owner id")
	cmd.Flags().StringVar(&f.Contributor, "contributor", "", "only epics or features with started-task contributions by this contributor id")
	cmd.Flags().StringVar(&f.Feature, "feature", "", "only tasks under this parent feature (id or key)")
	cmd.Flags().StringVar(&f.Epic, "epic", "", "only features under this epic (id or key)")
	return cmd
}

func runLs(cmd *cobra.Command, nounArg string, f lsFilters) error {
	noun, ok := resolveListNoun(nounArg)
	if !ok {
		return emitFailure(cmd, "ls", &usageError{
			reason: "unknown_noun",
			msg:    fmt.Sprintf("unknown noun %q (want one of epic(s), feature(s), task(s), question(s), amendment(s), bug(s), chore(s), tweak(s))", nounArg),
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "ls", err)
	}

	// Keep the filters as the caller typed them for the empty-result phrasing;
	// views.ResolveParentFilters rewrites f to resolved identities below.
	typed := f

	// Resolve the parentage filters to the parent's stable identity (a feature id, an
	// epic key) so a caller can name the parent by key or id and the per-node match is
	// a plain comparison. An unresolved parent is misuse (exit 2), the same as any bad
	// address.
	if err := views.ResolveParentFilters(tree, &f); err != nil {
		return emitFailure(cmd, "ls", err)
	}

	items := []ListItem{}
	for _, n := range views.Collect(tree, noun) {
		if !views.Matches(tree, n, f) {
			continue
		}
		items = append(items, views.Item(tree, n))
	}
	views.SortListItems(tree, noun, items)

	result := ListResult{Noun: noun, Count: len(items), Nodes: items}
	return emitSuccess(cmd, "ls", result, renderListHuman(result, typed))
}

// filterClauses names the active filters as short clauses ("status ready",
// "owner jc") in a fixed axis order, the detail an empty result reports.
func filterClauses(f lsFilters) []string {
	var clauses []string
	add := func(axis, value string) {
		if value != "" {
			clauses = append(clauses, axis+" "+value)
		}
	}
	add("status", f.Status)
	add("owner", f.Owner)
	add("contributor", f.Contributor)
	add("feature", f.Feature)
	add("epic", f.Epic)
	return clauses
}

// pluralNoun is the collection form of a singular node class, for the empty-list
// phrasing ("No features ...").
func pluralNoun(sing string) string {
	for plural, s := range pluralNouns {
		if s == sing {
			return plural
		}
	}
	return sing + "s"
}

// renderListHuman is the plain-text list for a terminal reader. An empty result
// names the filters that produced it (as the caller typed them), so "nothing
// here" reads as an answer rather than a shrug; with no filters it falls back to
// the plain not-found line.
func renderListHuman(r ListResult, f lsFilters) string {
	if r.Count == 0 {
		if clauses := filterClauses(f); len(clauses) > 0 {
			return fmt.Sprintf("No %s with %s.", pluralNoun(r.Noun), strings.Join(clauses, ", "))
		}
		return fmt.Sprintf("No %s found.", pluralNoun(r.Noun))
	}
	label := labelColumn(r.Nodes)
	lines := make([]string, 0, len(r.Nodes))
	for _, it := range r.Nodes {
		// Trimmed per row rather than over the joined block: a node with no
		// title is an interior row whose padding would otherwise ride out as
		// trailing whitespace, and trimming the whole string only ever reaches
		// the last one. Question rows always arrive with gen.QuestionTitle's
		// result in Title; this renderer never re-derives a statement fallback.
		lines = append(lines, strings.TrimRight(
			fmt.Sprintf("%-10s %s%s", it.Status, col(it.Ref, label), it.Title), " "))
	}
	return strings.Join(lines, "\n")
}

// labelColumn is the width of the reference column: this list's own widest
// reference plus a gutter, measured in display runes.
//
// It is computed rather than fixed for the half of `tiller tree`'s sizing rule
// that applies here (cli-surface.md, "The tree map sizes itself"): the label
// column is the widest label in the thing being rendered, and a label is an
// address, so it never truncates. The constant this replaced was 44, chosen when
// the longest thing a list could carry was a two-segment feature key with its
// id. A task's canonical reference is three segments plus that id
// (cli-surface.md, Human references) and runs past 44 on ordinary keys, which
// left a long row pushing its own title right while every shorter row stayed at
// 44 (task-9-brief.md, inherited item 1).
//
// tree's other half, truncating titles into a line budget, is not adopted and
// `ls` has no budget at all: a tree renders nested rows whose indent competes
// with the title for a fixed width, while a list row is flat and its title is
// the last column, so a long one wraps in the terminal rather than pushing
// anything out of place. A reader who wants the full node reads `show`.
//
// col (tree.go) guarantees at least one space of separation when a value meets
// or overruns the width, so a single-row list still separates its columns.
func labelColumn(items []ListItem) int {
	widest := 0
	for _, it := range items {
		if n := utf8.RuneCountInString(it.Ref); n > widest {
			widest = n
		}
	}
	return widest + 2
}
