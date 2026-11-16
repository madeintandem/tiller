// Package cli is the thin cobra shell for the tiller binary. It translates
// argv into core calls and renders results to stdout. It holds no domain
// logic and is the only package permitted to depend on cobra.
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// version is reported by `tiller --version` and is the number issues are
// tracked against. Stamped by hand at each release tag until release tooling
// exists; a build from an untagged commit still reports the last stamp.
const version = "1.0.4"

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
		newUsageCmd(),
		newCompactCmd(),
		newDriftCmd(),
		newMigrateCmd(),
		newUpgradeCmd(),
		newLinkCmd(),
		newUnlinkCmd(),
		newResolveCmd(),
		newRenameCmd(),
		newGradeCmd(),
		newRemoveCmd(),
		newOrderCmd(),
		newServeCmd(),
		newGuideCmd(),
		newNextCmd(),
		newReadoutCmd(),
		newAmendCmd(),
		newProposeCmd(),
		newWithdrawCmd(),
		newIntakeCmd(),
	)

	// Cobra's generated completion and help commands carry Shorts without the
	// closing period every other command's Short has. Build them now, which
	// cobra's own lazy init then leaves alone, and finish their sentences.
	root.InitDefaultCompletionCmd()
	root.InitDefaultHelpCmd()
	for _, c := range root.Commands() {
		switch c.Name() {
		case "completion":
			c.Short = "Generate the autocompletion script for the specified shell."
			for _, sub := range c.Commands() {
				if !strings.HasSuffix(sub.Short, ".") {
					sub.Short += "."
				}
			}
		case "help":
			c.Short = "Help about any command."
		}
	}
	refuseUnknownSubcommands(root)
	return root
}

// refuseUnknownSubcommands makes a word that names none of a command group's
// subcommands a misuse, across the whole tree below cmd. Cobra checks that
// only at the root: below it, a group with no argument validator accepts any
// positionals, and a group with no run function answers everything with its
// help page, so `tiller add bogus` printed help and exited 0.
//
// A command that has subcommands and declares no Args of its own takes no
// positionals (cobra.NoArgs, whose refusal names the unknown word). One that
// cannot run, which is cobra's generated completion command, prints its help
// when named alone, so cobra reaches the validator instead of stopping at
// help. A group that takes positionals of its own (check, remove) declares its
// Args and is left as it is.
func refuseUnknownSubcommands(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		refuseUnknownSubcommands(sub)
		if !sub.HasSubCommands() {
			continue
		}
		if sub.Args == nil {
			sub.Args = cobra.NoArgs
		}
		if !sub.Runnable() {
			sub.RunE = func(group *cobra.Command, _ []string) error { return group.Help() }
		}
	}
}

// Run executes the tiller CLI with the given arguments (os.Args[1:]) and
// returns the process exit code in the Tiller scheme: 0 success, 1 blocked,
// 2 misuse, 3 environment.
func Run(args []string) int {
	return execute(NewRootCmd(), args)
}

// execute is the one execution and rendering boundary: Run hands it a fresh
// root, and the tests hand it a root whose writers they captured, so both see
// the same output and exit codes. A command that already emitted its envelope
// returns an exitError carrying the mapped code; any other error is one cobra
// raised before a command ran, which is misuse (2) and is rendered here.
func execute(root *cobra.Command, args []string) int {
	// cobra reads a nil slice as "use os.Args", so no arguments stays explicit.
	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	resolved, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return renderParserFailure(root, resolved, args, err)
}

// renderParserFailure reports a refusal cobra raised before any command's RunE
// ran: an unknown command, an unknown flag, a flag value that does not parse,
// a missing required flag, or a wrong positional count. Human mode keeps its
// one stderr line. Under --json it writes the same error envelope a command
// failure does, with discriminators fixed here rather than read out of cobra's
// wording: reason unknown_command when no command matched and bad_args
// otherwise, and command the top-level command cobra resolved.
//
// The parser may have stopped before it reached --json (an unknown command
// parses no flags at all), so the request is read from the raw arguments.
func renderParserFailure(root, resolved *cobra.Command, args []string, err error) int {
	if !jsonRequested(args) {
		fmt.Fprintln(root.ErrOrStderr(), "tiller:", err)
		return 2
	}
	reason := reasonBadArgs
	// Find is the lookup ExecuteC ran, and it fails only for an unmatched
	// command. Asking it again keeps cobra's prose out of the classification:
	// a surplus positional on a no-argument command also reads "unknown
	// command" in cobra's message, and that one is bad_args.
	if _, _, findErr := root.Find(args); findErr != nil {
		reason = reasonUnknownCommand
	}
	code, _ := WriteError(root.OutOrStdout(), topLevelCommandName(resolved), &usageError{reason: reason, msg: err.Error()})
	return code
}

// topLevelCommandName names cmd by its top-level command (`add` for `add
// epic`), the word that command's own envelopes lead with. The root itself,
// which is what cobra resolves when no command matched, is `tiller`.
func topLevelCommandName(cmd *cobra.Command) string {
	for cmd.HasParent() && cmd.Parent().HasParent() {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}
