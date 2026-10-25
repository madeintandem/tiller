package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRootHelpExecutes is the scaffold smoke test: the root command's --help
// must succeed and the help text must name the binary.
func TestRootHelpExecutes(t *testing.T) {
	cmd := NewRootCmd()

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	require.NoError(t, err, "--help should exit cleanly")
	require.Contains(t, out.String(), "tiller", "help output should name the tiller binary")
}

// TestRunHelpExitZero confirms the Run helper maps --help to exit code 0.
func TestRunHelpExitZero(t *testing.T) {
	require.Equal(t, 0, Run([]string{"--help"}))
}

// TestRunNoArgsExitZero confirms a bare invocation prints help and exits 0.
func TestRunNoArgsExitZero(t *testing.T) {
	require.Equal(t, 0, Run(nil))
}

// TestRunUnknownArgIsMisuse confirms a stray token exits 2 (misuse).
func TestRunUnknownArgIsMisuse(t *testing.T) {
	require.Equal(t, 2, Run([]string{"bogus"}))
}

func TestRetiredGenCommandsAreUnknown(t *testing.T) {
	for _, args := range [][]string{{"gen"}, {"gen", "board"}} {
		cmd := NewRootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown command")
	}
}

// TestUnknownCommandBeatsUnknownFlag is the 1.4 regression guard from the
// full-arc dry run: `tiller cancel task X --reason y` reported `unknown flag:
// --reason` while cancel did not exist, falsely confirming the command was
// real. An unknown subcommand must be reported as an unknown command even when
// unknown flags ride along.
func TestUnknownCommandBeatsUnknownFlag(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"bogus", "task", "T-k3p9w6", "--bogusflag", "y"})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown command "bogus"`, "the unknown subcommand is the reported fault")
	require.NotContains(t, err.Error(), "unknown flag", "the stray flag never wins over the missing command")
}

// TestUnknownFlagOnRealCommandStillMisuse confirms the unknown-command fix does
// not swallow flag errors on commands that do exist.
func TestUnknownFlagOnRealCommandStillMisuse(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"ls", "features", "--bogusflag"})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown flag: --bogusflag")
}

// TestUnknownFlagOnBareRootIsMisuse confirms a stray root-level flag stays a
// misuse (exit 2), not a silent help.
func TestUnknownFlagOnBareRootIsMisuse(t *testing.T) {
	require.Equal(t, 2, Run([]string{"--bogusflag"}))
}

func TestRetiredOwnerSurfaceIsRejected(t *testing.T) {
	for _, args := range [][]string{
		{"owner", "add", "jc", "--name", "JC"},
		{"owner", "remove", "jc"},
		{"contributor", "add", "jc", "--name", "JC"},
		{"add", "epic", "example", "--owner", "jc"},
		{"remove", "contributor", "jc", "--owner", "pat"},
		{"migrate", "--owner", "jc"},
		{"upgrade", "--project-only", "--owner", "jc"},
	} {
		t.Run(args[0]+" "+args[1], func(t *testing.T) {
			_, stderr, code := execCLI(t, args...)
			require.Equal(t, 2, code, stderr)
			require.Contains(t, stderr, "unknown")
		})
	}
}
