// Package cli is the thin cobra shell for the tiller binary. It translates
// argv into core calls and renders results to stdout. It holds no domain
// logic and is the only package permitted to depend on cobra.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// version is reported by `tiller --version` and is the number issues are
// tracked against. Stamped by hand at each release tag until release tooling
// exists; a build from an untagged commit still reports the last stamp.
const version = "0.21.2"

// NewRootCmd builds the root cobra command for the tiller CLI. Subcommands are
// registered onto the returned command by later wiring; pass 1 starts with the
// bare root plus the version flag.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tiller",
		Short: "Tiller drives an SDLC workflow for humans and AI coding agents.",
		Long: "Tiller is a single-binary SDLC workflow tool. It guides humans and AI\n" +
			"coding agents through a guided build loop over a file-backed workspace.",
		Version: version,
		// The CLI owns exit-code mapping in Run, so cobra should neither print
		// usage on every error nor print the error itself.
		SilenceUsage:  true,
		SilenceErrors: true,
		// The requires_tiller advisory (requires.go) runs ahead of every
		// subcommand; no subcommand defines its own PersistentPreRun, so this
		// one is never shadowed. It writes to stderr only and never fails.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			warnRequiresTiller(cmd.ErrOrStderr())
		},
		// Args is deliberately nil: with no validator of its own, cobra's Find
		// reports an unmatched first token as an unknown command BEFORE any flag
		// parsing runs, so `tiller bogus <args> --bogusflag` names the missing
		// command rather than the stray flag (the dry run's 1.4 fix: `unknown
		// flag: --reason` falsely confirmed a command that did not exist). A
		// matched subcommand still runs its own Args validator and flag parse,
		// and a stray flag on the bare root is still a misuse (exit 2).
		// With no subcommand, print help and exit cleanly. This also makes the
		// root "runnable", so cobra renders the Usage block under --help even
		// before any subcommands are registered.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// --json is shared by every read and mutate command (cli-surface.md, Output
	// and exit codes), so it lives as a persistent flag on the root and the
	// subcommands read it through the inherited flag set.
	root.PersistentFlags().Bool("json", false, "emit machine-readable JSON instead of human-readable text")

	root.AddCommand(
		newInitCmd(),
		newConfigureCmd(),
		newJoinCmd(),
		newStatusCmd(),
		newQueueCmd(),
		newCheckCmd(),
		newAddCmd(),
		newShowCmd(),
		newLsCmd(),
		newSearchCmd(),
		newQueryCmd(),
		newTreeCmd(),
		newApproveCmd(),
		newClaimCmd(),
		newReleaseCmd(),
		newReassignCmd(),
		newTransferCmd(),
		newSpecCmd(),
		newStartCmd(),
		newReturnCmd(),
		newResumeCmd(),
		newCancelCmd(),
		newArchiveCmd(),
		newAnswerCmd(),
		newSubmitCmd(),
		newEvalCmd(),
		newAttestCmd(),
		newSyncCmd(),
		newDriftCmd(),
		newMigrateCmd(),
		newUpgradeCmd(),
		newLinkCmd(),
		newUnlinkCmd(),
		newResolveCmd(),
		newRenameCmd(),
		newRemoveCmd(),
		newOrderCmd(),
		newServeCmd(),
		newGuideCmd(),
		newNextCmd(),
		newReadoutCmd(),
		newAmendCmd(),
		newWithdrawCmd(),
		newIntakeCmd(),
	)
	return root
}

// Run executes the tiller CLI with the given arguments (os.Args[1:]) and
// returns the process exit code in the Tiller scheme: 0 success, 1 blocked,
// 2 misuse, 3 environment. A command that already emitted its envelope returns
// an exitError carrying the mapped code; any other error is a cobra-level parse
// or usage failure, which is misuse (2).
func Run(args []string) int {
	root := NewRootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		fmt.Fprintln(root.ErrOrStderr(), "tiller:", err)
		return 2
	}
	return 0
}
