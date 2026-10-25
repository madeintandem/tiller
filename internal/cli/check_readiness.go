package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/spf13/cobra"
)

// This file wires the `check` command tree and implements its one project-level
// subcommand, `tiller check readiness` (cli-surface.md, "Bootstrapping a
// project"): the mechanical gate that runs before any nodes exist. The
// node-grain verb `tiller check <noun> <node>`, which runs and records a node's
// gates, lives in check.go and is wired onto this command's RunE below.

func newCheckCmd() *cobra.Command {
	var assume, reason string
	var rerun bool
	var gateNames []string
	cmd := &cobra.Command{
		Use:   "check [<noun> <node>]",
		Short: "Run and record a node's gates, or check project readiness.",
		Long: "check runs a node's required command-backed gates (build, tests, evals)\n" +
			"and records the result onto the node. A task advances to check only when\n" +
			"every required gate passes. Failed or pending required acceptance is a\n" +
			"blocked answer (exit 1), even though its results were recorded. A gate\n" +
			"that already passed on unchanged code under an unchanged command is\n" +
			"reused, not rerun; --rerun runs every command gate for real, and\n" +
			"--gate <name> (repeatable) runs only the named gates. With no\n" +
			"node it hosts the readiness gate (`tiller check readiness`).",
		// The node-grain form runs through this RunE: `check readiness` matches the
		// subcommand, while `check feature <node>` falls through to here.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if len(args) != 2 {
				return emitFailure(cmd, "check", &usageError{
					reason: "bad_args",
					msg:    "check takes a noun and a node (or the readiness subcommand)",
				})
			}
			return runCheck(cmd, args[0], args[1], assume, reason, rerun, gateNames)
		},
	}
	cmd.Flags().StringVar(&assume, "assume-question", "", "proceed under an open question, recording it in assumption_basis")
	cmd.Flags().StringVar(&reason, "reason", "", "justification for the assumption (with --assume-question)")
	cmd.Flags().BoolVar(&rerun, "rerun", false, "run every command gate for real, even one whose recorded pass could be reused")
	cmd.Flags().StringArrayVar(&gateNames, "gate", nil, "run only the named gate (repeatable); another gate counts only if its recorded pass is current, and one that is not is marked stale")
	addContributorFlag(cmd)
	cmd.AddCommand(newCheckReadinessCmd())
	return cmd
}

func newCheckReadinessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "readiness",
		Short: "Verify the project inputs and manifest are ready, and name the next move.",
		Long: "check readiness verifies the inputs are present and the manifest is\n" +
			"coherent: design/ is non-empty, project.kind and contributors are set, and every\n" +
			"declared code root exists. Genuine execution dependency cycles are gaps;\n" +
			"projected epic grouping cycles are nonblocking advisories. It reports\n" +
			"ready, or names the gaps. It does not judge whether the input docs are\n" +
			"rich enough; that is the agent's work.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheckReadiness(cmd)
		},
	}
	// --rerun and --gate belong to the node-grain check. They are declared here,
	// hidden, only so naming them on readiness is a usage error with a reason
	// rather than cobra's bare unknown-flag failure.
	cmd.Flags().Bool("rerun", false, "")
	cmd.Flags().StringArray("gate", nil, "")
	_ = cmd.Flags().MarkHidden("rerun")
	_ = cmd.Flags().MarkHidden("gate")
	return cmd
}

func runCheckReadiness(cmd *cobra.Command) error {
	if cmd.Flags().Changed("rerun") || cmd.Flags().Changed("gate") {
		return emitFailure(cmd, "check-readiness", &usageError{
			reason: "readiness_flag",
			msg:    "--rerun and --gate apply to a node's gates; check readiness runs none",
		})
	}
	// The full tree, not just the manifest: the dependency-cycle check (the
	// routing batch's fix 3) needs the recorded graph, and a repo whose tree
	// cannot load fails every other command through this same helper, so the
	// gate failing the same way is consistency, not a new hazard.
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "check-readiness", err)
	}

	report := readiness.Check(tree.Manifest, tree)
	code := 0
	if !report.Ready {
		// A red gate is a normal negative business answer: exit 1, with the
		// gaps carried as structured data the agent can act on.
		code = 1
	}
	skill, reason := "", ""
	if report.Ready {
		// A ready project's Next is guide's own suggestion from the shared
		// route, so readiness never names a seam the route would not.
		skill, reason = suggestSkill(tree, resolveActingContributor(cmd, tree.Root))
	}
	return emitReport(cmd, "check-readiness", report.Ready, report, renderReadinessHuman(report, skill, reason), code)
}

// renderReadinessHuman is the plain-text rendering of a readiness report.
// A ready project closes on the route's suggestion, the same skill and
// reason `tiller guide` prints.
func renderReadinessHuman(r readiness.Report, skill, reason string) string {
	var b strings.Builder
	if r.Ready {
		fmt.Fprintf(&b, "Ready: all readiness checks pass.\nNext (tiller guide %s): %s", skill, reason)
	} else {
		fmt.Fprintf(&b, "Not ready: %d gap(s):", len(r.Gaps))
		for _, g := range r.Gaps {
			fmt.Fprintf(&b, "\n  - %s", g.Message)
		}
	}
	if len(r.Advisories) > 0 {
		b.WriteString("\nAdvisories (nonblocking):")
		for _, advisory := range r.Advisories {
			fmt.Fprintf(&b, "\n  - %s: %s", advisory.Reason, advisory.Message)
		}
	}
	return b.String()
}
