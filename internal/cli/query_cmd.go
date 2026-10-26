package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/spf13/cobra"
)

func newQueryCmd() *cobra.Command {
	var from, follow, depth, where []string
	var help strings.Builder
	help.WriteString("Traverse recorded relationships without changing project or Git state.\n\n" +
		"--from repeats; --follow is a comma-separated union used at every hop,\n" +
		"not an ordered traversal program. --depth defaults to 1; 0 inspects seeds.\n" +
		"--where repeats exact, case-sensitive kind/status/owner/contributor clauses.\n" +
		"Filters select matches after traversal, so bridge vertices remain visible.\n" +
		"Path queries include recorded enclosing scopes, not verified code coverage.\n" +
		"Recorded question blockers are not an eligibility decision.\n\n" +
		"Directions (forward / inverse):\n")
	for _, kind := range (&relationships.Index{}).Catalog() {
		fmt.Fprintf(&help, "  %s / %s\n", kind.Name, kind.Inverse)
	}
	cmd := &cobra.Command{
		Use:   "query --from <address> --follow <directions>",
		Short: "Traverse recorded work, document, and path relationships.",
		Long:  help.String(),
		// Parse here so a malformed flag still receives the same error envelope
		// even when --json occurs after the failing token. Cobra stops parsing
		// at that token before its ordinary flag-error callback can see --json.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Flags().AddFlagSet(cmd.InheritedFlags())
			if err := cmd.Flags().Parse(args); err != nil {
				return querySyntaxFailure(cmd, args, err)
			}
			if requested, _ := cmd.Flags().GetBool("help"); requested {
				return cmd.Help()
			}
			if cmd.Flags().NArg() != 0 {
				return emitFailure(cmd, "query", &usageError{reason: "query_syntax", msg: "query accepts flags, not positional arguments"})
			}
			input, err := relationships.ParseQuery(from, follow, depth, where)
			if err != nil {
				return emitFailure(cmd, "query", err)
			}
			return runQuery(cmd, input)
		},
	}
	cmd.Flags().StringArrayVar(&from, "from", nil, "start address: immutable ID, canonical key, doc:path, or path:path (repeatable, required)")
	cmd.Flags().StringArrayVar(&follow, "follow", nil, "comma-separated forward/inverse relationship `directions`, given once (required)")
	cmd.Flags().StringArrayVar(&depth, "depth", nil, "maximum hop distance, one nonnegative integer `n` given once (default 1)")
	cmd.Flags().StringArrayVar(&where, "where", nil, "exact kind=value, status=value, owner=value, or contributor=value filter (repeatable, ANDed)")
	return cmd
}

func querySyntaxFailure(cmd *cobra.Command, args []string, err error) error {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			_ = cmd.Flags().Set("json", "true")
		} else if raw, ok := strings.CutPrefix(arg, "--json="); ok {
			value, parseErr := strconv.ParseBool(raw)
			if parseErr != nil {
				value = true
			}
			_ = cmd.Flags().Set("json", strconv.FormatBool(value))
		}
	}
	return emitFailure(cmd, "query", &usageError{reason: "query_syntax", msg: err.Error()})
}

// Supplemental failures are environment failures even if their underlying I/O
// error has no CLI-aware type. Resolver failures from Query retain their usual
// not_found/ambiguous_key exit-2 classification instead.
type queryReadError struct{ err error }

func (err *queryReadError) Error() string  { return err.err.Error() }
func (err *queryReadError) Unwrap() error  { return err.err }
func (err *queryReadError) ExitCode() int  { return 3 }
func (err *queryReadError) Reason() string { return "query_read_failed" }

func runQuery(cmd *cobra.Command, input relationships.QueryInput) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "query", &queryReadError{err})
	}
	index, err := readinput.Relationships(cmd.Context(), tree, readinput.RelationshipOptions{})
	if err != nil {
		if stableErr := tree.CheckStable(); stableErr != nil {
			err = stableErr
		}
		return emitFailure(cmd, "query", &queryReadError{err})
	}
	result, queryErr := relationships.Query(cmd.Context(), index, input)
	if stableErr := tree.CheckStable(); stableErr != nil {
		return emitFailure(cmd, "query", &queryReadError{stableErr})
	}
	if queryErr != nil {
		if cmd.Context().Err() != nil {
			queryErr = &queryReadError{queryErr}
		}
		return emitFailure(cmd, "query", queryErr)
	}
	human := ""
	if !jsonOut(cmd) {
		human = renderQueryHuman(result)
	}
	return emitSuccess(cmd, "query", result, human)
}

func renderQueryHuman(result relationships.QueryResult) string {
	var out strings.Builder
	if !result.Complete {
		out.WriteString("Relationship extraction is incomplete; matches may be missing.\n")
	}
	if len(result.Matched) == 0 {
		out.WriteString("No matching relationships.\n")
	}
	matched := make(map[string]bool, len(result.Matched))
	for _, id := range result.Matched {
		matched[id] = true
	}
	edgesByVertex := make(map[string][]relationships.Edge)
	for _, edge := range result.Edges {
		if matched[edge.From] {
			edgesByVertex[edge.From] = append(edgesByVertex[edge.From], edge)
		}
		if edge.To != edge.From && matched[edge.To] {
			edgesByVertex[edge.To] = append(edgesByVertex[edge.To], edge)
		}
	}
	for _, vertex := range result.Vertices {
		if !matched[vertex.ID] {
			continue
		}
		reference := vertex.ID
		if vertex.Key != "" {
			reference = vertex.Key + " (" + vertex.ID + ")"
		}
		role := ""
		if vertex.Seed {
			role = "  seed"
		}
		if vertex.Unresolved {
			role += "  unresolved"
		}
		fmt.Fprintf(&out, "%s  %s  hop %d%s\n", vertex.Kind, reference, vertex.Depth, role)
		seen := map[string]bool{}
		for _, edge := range edgesByVertex[vertex.ID] {
			for _, source := range edge.Sources {
				location := source.File
				if source.Line > 0 {
					location += ":" + strconv.Itoa(source.Line)
				}
				if source.Field != "" {
					location += " [" + source.Field + "]"
				}
				if source.Criterion != "" {
					location += " criterion=" + source.Criterion
				}
				if source.Basis != "" {
					location += " basis=" + source.Basis
				}
				if source.Revision != nil {
					location += " revision=" + source.Revision.Value
				}
				if edge.Derived {
					location += " derived from scope=" + edge.DeclaredScope
				}
				line := "  " + edge.Kind + "  " + location + "\n"
				if !seen[line] {
					out.WriteString(line)
					seen[line] = true
				}
			}
		}
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic [%s/%s]: %s", diagnostic.Facet, diagnostic.Code, diagnostic.Message)
		if diagnostic.Source != nil {
			fmt.Fprintf(&out, " (%s", diagnostic.Source.File)
			if diagnostic.Source.Line > 0 {
				fmt.Fprintf(&out, ":%d", diagnostic.Source.Line)
			}
			out.WriteString(")")
		}
		out.WriteByte('\n')
	}
	return strings.TrimSuffix(out.String(), "\n")
}
