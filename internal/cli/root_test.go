package cli

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestRootHelpExecutes is the scaffold smoke test: the root command's --help
// must succeed and the help text must name the binary.
func TestRootHelpExecutes(t *testing.T) {
	out, stderr, code := execCLI(t, "--help")
	require.Equal(t, 0, code, "--help should exit cleanly")
	require.Empty(t, stderr)
	require.Contains(t, out, "tiller", "help output should name the tiller binary")
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
		out, stderr, code := execCLI(t, args...)
		require.Equal(t, 2, code)
		require.Empty(t, out)
		require.Contains(t, stderr, `unknown command "gen"`)
	}
}

// TestUnknownCommandBeatsUnknownFlag is the 1.4 regression guard from the
// full-arc dry run: `tiller cancel task X --reason y` reported `unknown flag:
// --reason` while cancel did not exist, falsely confirming the command was
// real. An unknown subcommand must be reported as an unknown command even when
// unknown flags ride along.
func TestUnknownCommandBeatsUnknownFlag(t *testing.T) {
	out, stderr, code := execCLI(t, "bogus", "task", "T-k3p9w6", "--bogusflag", "y")
	require.Equal(t, 2, code)
	require.Empty(t, out)
	require.Contains(t, stderr, `unknown command "bogus"`, "the unknown subcommand is the reported fault")
	require.NotContains(t, stderr, "unknown flag", "the stray flag never wins over the missing command")
}

// TestUnknownFlagOnRealCommandStillMisuse confirms the unknown-command fix does
// not swallow flag errors on commands that do exist.
func TestUnknownFlagOnRealCommandStillMisuse(t *testing.T) {
	out, stderr, code := execCLI(t, "ls", "features", "--bogusflag")
	require.Equal(t, 2, code)
	require.Empty(t, out)
	require.Equal(t, "tiller: unknown flag: --bogusflag\n", stderr)
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

// parserFailure is one invocation Cobra refuses before any command's RunE
// runs, with the discriminators the JSON envelope must carry for it.
type parserFailure struct {
	name    string
	args    []string
	command string
	reason  string
	message string
}

// parserFailures covers each parser-stage refusal (a missing or surplus
// positional, a command's own argument validator, a missing required flag, an
// unknown flag, an invalid flag value, an unknown command, an unknown
// subcommand under a command group) with --json in each position a caller can
// put it: before the command, after it, and after the token that failed.
var parserFailures = []parserFailure{
	{"missing node, json before the command", []string{"--json", "show"}, "show", "bad_args", "accepts 2 arg(s), received 0"},
	{"missing node, json after the command", []string{"show", "--json"}, "show", "bad_args", "accepts 2 arg(s), received 0"},
	{"missing node, json last", []string{"show", "epic", "--json"}, "show", "bad_args", "accepts 2 arg(s), received 1"},
	{"surplus positionals", []string{"--json", "show", "epic", "a", "b", "c"}, "show", "bad_args", "accepts 2 arg(s), received 4"},
	{"a command's own validator", []string{"status", "epic", "--json"}, "status", "bad_args", "status takes either no arguments"},
	{"missing node under a noun subcommand", []string{"add", "epic", "--json"}, "add", "bad_args", "accepts 1 arg(s), received 0"},
	{"surplus positional on a no-argument command", []string{"check", "readiness", "extra", "--json"}, "check", "bad_args", "extra"},
	{"missing required flag, json first", []string{"--json", "add", "bug", "broken-save", "--contributor", "jc"}, "add", "bad_args", `required flag(s) "title" not set`},
	{"missing required flag, json last", []string{"add", "bug", "broken-save", "--contributor", "jc", "--json"}, "add", "bad_args", `required flag(s) "title" not set`},
	{"unknown flag, json before the command", []string{"--json", "ls", "features", "--bogusflag"}, "ls", "bad_args", "unknown flag: --bogusflag"},
	{"unknown flag, json before the failing flag", []string{"ls", "features", "--json", "--bogusflag"}, "ls", "bad_args", "unknown flag: --bogusflag"},
	{"unknown flag, json after the failing flag", []string{"ls", "features", "--bogusflag", "--json"}, "ls", "bad_args", "unknown flag: --bogusflag"},
	{"invalid flag value", []string{"submit", "task", "x", "--pr", "abc", "--json"}, "submit", "bad_args", `invalid argument "abc" for "--pr" flag`},
	{"invalid json value still asks for json", []string{"ls", "features", "--json=maybe"}, "ls", "bad_args", `invalid argument "maybe" for "--json" flag`},
	{"unknown flag on the bare root, json first", []string{"--json", "--bogusflag"}, "tiller", "bad_args", "unknown flag: --bogusflag"},
	{"unknown flag on the bare root, json last", []string{"--bogusflag", "--json"}, "tiller", "bad_args", "unknown flag: --bogusflag"},
	{"unknown command, json first", []string{"--json", "bogus"}, "tiller", "unknown_command", `unknown command "bogus" for "tiller"`},
	{"unknown command, json last", []string{"bogus", "--json"}, "tiller", "unknown_command", `unknown command "bogus" for "tiller"`},
	{"unknown command with stray flags", []string{"bogus", "task", "T-k3p9w6", "--bogusflag", "y", "--json"}, "tiller", "unknown_command", `unknown command "bogus" for "tiller"`},
	{"last json value wins when it turns json on", []string{"--json=false", "show", "--json"}, "show", "bad_args", "accepts 2 arg(s), received 0"},
	{"unknown subcommand, json first", []string{"--json", "add", "bogus"}, "add", "bad_args", `unknown command "bogus" for "tiller add"`},
	{"unknown subcommand, json last", []string{"add", "bogus", "--json"}, "add", "bad_args", `unknown command "bogus" for "tiller add"`},
	{"unknown subcommand with a node", []string{"claim", "bogus", "X", "--json"}, "claim", "bad_args", `unknown command "bogus" for "tiller claim"`},
	{"unknown subcommand under a generated group", []string{"completion", "bogus", "--json"}, "completion", "bad_args", `unknown command "bogus" for "tiller completion"`},
}

// soleEnvelope decodes stdout as exactly one JSON envelope: a second document,
// or any text around the first, fails the test.
func soleEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var env map[string]any
	require.NoError(t, dec.Decode(&env), "stdout must be a JSON envelope: %q", stdout)
	var extra any
	require.Error(t, dec.Decode(&extra), "stdout must hold exactly one envelope: %q", stdout)
	return env
}

func gitStatus(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// TestParserStageFailuresRenderOneJSONEnvelope pins the output contract
// (cli-surface.md, "Output and exit codes") for a failure Cobra raises before
// a command runs: with --json requested anywhere before a bare `--`, stdout is
// exactly one error envelope carrying exit code 2, a stable reason, and the
// command name; stderr stays empty; nothing is written to the project.
func TestParserStageFailuresRenderOneJSONEnvelope(t *testing.T) {
	for _, tc := range parserFailures {
		t.Run(tc.name, func(t *testing.T) {
			initOwnedRepo(t)

			stdout, stderr, code := execCLI(t, tc.args...)

			require.Equal(t, 2, code, "stdout=%q stderr=%q", stdout, stderr)
			require.Empty(t, stderr, "a JSON failure writes the envelope alone")
			env := soleEnvelope(t, stdout)
			require.Equal(t, false, env["ok"])
			require.Equal(t, tc.command, env["command"])
			require.NotContains(t, env, "data", "a tool failure carries an error block, never data")
			body, ok := env["error"].(map[string]any)
			require.True(t, ok, "envelope carries an error block: %s", stdout)
			require.EqualValues(t, 2, body["code"])
			require.Equal(t, tc.reason, body["reason"])
			require.Contains(t, body["message"], tc.message)
			require.Empty(t, gitStatus(t), "a refused invocation writes nothing")
		})
	}
}

// TestParserStageFailuresStayHumanWithoutJSON pins the other half: without
// --json the same refusals keep their one stderr line and an empty stdout.
// A --json after a bare `--` is a positional, and an explicit false turns the
// request back off, so neither asks for the envelope.
func TestParserStageFailuresStayHumanWithoutJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		stderr string
	}{
		{"missing node", []string{"show"}, "tiller: accepts 2 arg(s), received 0\n"},
		{"missing required flag", []string{"add", "bug", "broken-save", "--contributor", "jc"}, "tiller: required flag(s) \"title\" not set\n"},
		{"unknown flag", []string{"ls", "features", "--bogusflag"}, "tiller: unknown flag: --bogusflag\n"},
		{"unknown command", []string{"bogus"}, "tiller: unknown command \"bogus\" for \"tiller\"\n"},
		{"unknown subcommand", []string{"add", "bogus"}, "tiller: unknown command \"bogus\" for \"tiller add\"\n"},
		{"unknown subcommand with a node", []string{"claim", "bogus", "X"}, "tiller: unknown command \"bogus\" for \"tiller claim\"\n"},
		{"unknown subcommand under a generated group", []string{"completion", "bogus"}, "tiller: unknown command \"bogus\" for \"tiller completion\"\n"},
		{"json after the terminator is a positional", []string{"ls", "features", "--bogusflag", "--", "--json"}, "tiller: unknown flag: --bogusflag\n"},
		{"json explicitly off", []string{"show", "--json=false"}, "tiller: accepts 2 arg(s), received 0\n"},
		{"last json value wins when it turns json off", []string{"--json", "show", "--json=false"}, "tiller: accepts 2 arg(s), received 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initOwnedRepo(t)

			stdout, stderr, code := execCLI(t, tc.args...)

			require.Equal(t, 2, code)
			require.Empty(t, stdout)
			require.Equal(t, tc.stderr, stderr)
			require.Empty(t, gitStatus(t), "a refused invocation writes nothing")
		})
	}
}

// TestHelpIgnoresJSON confirms --help stays the human help page, exit 0, with
// --json on the line: help is not a failure and has no envelope.
func TestHelpIgnoresJSON(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--help"},
		{"show", "--help", "--json"},
		{"--json", "help", "show"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := execCLI(t, args...)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)
			require.Contains(t, stdout, "Usage:")
			require.False(t, json.Valid([]byte(stdout)), "help is prose, not an envelope")
		})
	}
}

// commandGroups lists, as argument paths, every command below the root that
// has subcommands.
func commandGroups(cmd *cobra.Command) [][]string {
	var groups [][]string
	for _, sub := range cmd.Commands() {
		if sub.HasSubCommands() {
			groups = append(groups, strings.Fields(sub.CommandPath())[1:])
		}
		groups = append(groups, commandGroups(sub)...)
	}
	return groups
}

// TestEveryCommandGroupRefusesAnUnknownWord walks the whole command tree, so a
// group added later is held to the same rule: a word that names none of a
// group's subcommands is misuse (exit 2), never the help page with exit 0. It
// covers the groups that only dispatch (add, claim) and the ones that also
// take positionals of their own (check, remove).
func TestEveryCommandGroupRefusesAnUnknownWord(t *testing.T) {
	groups := commandGroups(NewRootCmd())
	require.Contains(t, groups, []string{"add"})
	require.Contains(t, groups, []string{"completion"})

	for _, group := range groups {
		args := append(append([]string{}, group...), "no-such-subcommand")
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			initOwnedRepo(t)

			stdout, stderr, code := execCLI(t, args...)

			require.Equal(t, 2, code, "stdout=%q stderr=%q", stdout, stderr)
			require.Empty(t, stdout)
			require.True(t, strings.HasPrefix(stderr, "tiller: "), stderr)
			require.Equal(t, 1, strings.Count(stderr, "\n"), "one stderr line: %q", stderr)
			require.Empty(t, gitStatus(t), "a refused invocation writes nothing")
		})
		t.Run(strings.Join(args, " ")+" --json", func(t *testing.T) {
			initOwnedRepo(t)

			stdout, stderr, code := execCLI(t, append(args, "--json")...)

			require.Equal(t, 2, code, "stdout=%q stderr=%q", stdout, stderr)
			require.Empty(t, stderr, "a JSON failure writes the envelope alone")
			env := soleEnvelope(t, stdout)
			require.Equal(t, false, env["ok"])
			require.Equal(t, group[0], env["command"])
			body, ok := env["error"].(map[string]any)
			require.True(t, ok, "envelope carries an error block: %s", stdout)
			require.EqualValues(t, 2, body["code"])
			require.Equal(t, "bad_args", body["reason"])
			require.Empty(t, gitStatus(t), "a refused invocation writes nothing")
		})
	}
}

// TestCommandGroupHelpStaysExitZero pins what the unknown-word refusal must
// not take away: a group named with no subcommand, and any group under
// --help, still print the help page and exit 0, with or without --json.
func TestCommandGroupHelpStaysExitZero(t *testing.T) {
	invocations := [][]string{{"add"}, {"claim"}, {"release"}, {"reassign"}, {"check"}, {"completion"}}
	for _, group := range commandGroups(NewRootCmd()) {
		invocations = append(invocations,
			append(append([]string{}, group...), "--help"),
			append(append([]string{}, group...), "no-such-subcommand", "--help"),
		)
	}
	for _, args := range invocations {
		for _, args := range [][]string{args, append(append([]string{}, args...), "--json")} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				stdout, stderr, code := execCLI(t, args...)
				require.Equal(t, 0, code, "stdout=%q stderr=%q", stdout, stderr)
				require.Empty(t, stderr)
				require.Contains(t, stdout, "Usage:")
				require.Contains(t, stdout, "Available Commands:")
				require.False(t, json.Valid([]byte(stdout)), "help is prose, not an envelope")
			})
		}
	}
}

// TestHelpCommandStillReachesGroups confirms the refusal leaves Cobra's own
// help command working: `help` still reaches a group and its subcommand. The
// completion scripts are checked through the built binary (internal/e2e),
// since Cobra binds their writer before a test can capture it.
func TestHelpCommandStillReachesGroups(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"help", "completion"}, "tiller completion [command]"},
		{[]string{"help", "add"}, "tiller add [command]"},
		{[]string{"help", "add", "epic"}, "tiller add epic"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, stderr, code := execCLI(t, tc.args...)
			require.Equal(t, 0, code, "stdout=%q stderr=%q", stdout, stderr)
			require.Empty(t, stderr)
			require.Contains(t, stdout, tc.want)
		})
	}
}

// TestCommandFailureRendersExactlyOneEnvelope guards the boundary from the
// other side: a failure a command already rendered is not rendered again.
func TestCommandFailureRendersExactlyOneEnvelope(t *testing.T) {
	initOwnedRepo(t)

	stdout, stderr, code := execCLI(t, "show", "epic", "no-such-epic", "--json")

	require.Equal(t, 2, code)
	require.Empty(t, stderr)
	env := soleEnvelope(t, stdout)
	body, ok := env["error"].(map[string]any)
	require.True(t, ok, stdout)
	require.Equal(t, "not_found", body["reason"])
}
