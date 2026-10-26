package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestParseRequiresTiller(t *testing.T) {
	good := map[string][3]int{
		">= 1.0":    {1, 0, 0},
		"1.0":       {1, 0, 0},
		"v0.11.2":   {0, 11, 2},
		">=0.10.3":  {0, 10, 3},
		" >= 2.1 ":  {2, 1, 0},
		">= v1.2.3": {1, 2, 3},
	}
	for in, want := range good {
		got, ok := parseRequiresTiller(in)
		require.True(t, ok, "%q parses", in)
		require.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "latest", "> 1.0", "1", "1.0.0.0", "one.two", "^1.0"} {
		_, ok := parseRequiresTiller(bad)
		require.False(t, ok, "%q is not a floor", bad)
	}
}

func TestRequiresTillerAdvisoryOnlyBelowTheFloor(t *testing.T) {
	require.Empty(t, requiresTillerAdvisory("0.10.3", ""), "no floor, no advisory")
	require.Empty(t, requiresTillerAdvisory("0.10.3", ">= 0.10.3"), "at the floor is fine")
	require.Empty(t, requiresTillerAdvisory("0.10.3", ">= 0.9"), "above the floor is fine")

	line := requiresTillerAdvisory("0.10.3", ">= 1.0")
	require.Contains(t, line, "warning:")
	require.Contains(t, line, "v0.10.3")
	require.Contains(t, line, ">= 1.0")
	require.Contains(t, line, "tiller upgrade")

	bad := requiresTillerAdvisory("0.10.3", "latest")
	require.Contains(t, bad, "not a version floor", "an unparseable floor is named, not ignored")
	require.Contains(t, bad, "latest")
}

// setRequiresTiller turns the commented example init writes into a live floor.
func setRequiresTiller(t *testing.T, dir, floor string) {
	t.Helper()
	path := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	const template = `  # requires_tiller: ">= 1.0"   # optional advisory minimum binary`
	require.Contains(t, string(data), template, "the init template still carries the commented example")
	out := strings.Replace(string(data), template, fmt.Sprintf("  requires_tiller: %q", floor), 1)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestCommandsWarnWhenBinaryIsBelowRequiresTiller is the acceptance:
// a floor above the running binary produces one stderr line naming tiller
// upgrade, the command still succeeds, stdout is untouched, and a --json
// caller still gets a parseable envelope.
func TestCommandsWarnWhenBinaryIsBelowRequiresTiller(t *testing.T) {
	dir := initRepo(t)
	setRequiresTiller(t, dir, ">= 99.0")

	out, errOut, code := execCLI(t, "status")
	require.Equal(t, 0, code, "advisory only: the command still succeeds")
	require.Contains(t, errOut, "below the project's requires_tiller floor (>= 99.0)")
	require.Contains(t, errOut, "tiller upgrade")
	require.NotContains(t, out, "warning", "the advisory never lands on stdout")
	require.Contains(t, out, "Project:", "the command's own output is unchanged")

	jout, jerr, jcode := execCLI(t, "status", "--json")
	require.Equal(t, 0, jcode)
	require.Contains(t, jerr, "requires_tiller floor")
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(jout), &envelope), "stdout stays a parseable envelope")
}

func TestCommandsStayQuietAtOrAboveRequiresTiller(t *testing.T) {
	dir := initRepo(t)
	setRequiresTiller(t, dir, ">= 0.1")
	_, errOut, code := execCLI(t, "status")
	require.Equal(t, 0, code)
	require.NotContains(t, errOut, "requires_tiller")
}

func TestCommandsStayQuietWithNoFloorOrNoRepo(t *testing.T) {
	initRepo(t)
	_, errOut, code := execCLI(t, "status")
	require.Equal(t, 0, code)
	require.NotContains(t, errOut, "requires_tiller", "the commented template is not a floor")

	empty := t.TempDir()
	t.Chdir(empty)
	_, errOut, _ = execCLI(t, "status")
	require.NotContains(t, errOut, "requires_tiller", "outside a repo there is nothing to compare")
}

// The nudge prints ahead of the wall: on a tree stamped past this binary the
// schema refusal still wins the exit code, and the advisory still names the
// floor that explains why.
func TestRequiresTillerAdvisoryFiresAheadOfASchemaRefusal(t *testing.T) {
	dir := initRepo(t)
	setRequiresTiller(t, dir, ">= 99.0")
	path := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	bumped := strings.Replace(string(data),
		fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion),
		fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion+1), 1)
	require.NotEqual(t, string(data), bumped, "the stamp line was found and bumped")
	require.NoError(t, os.WriteFile(path, []byte(bumped), 0o644))

	_, errOut, code := execCLI(t, "status")
	require.Equal(t, 3, code, "the schema wall still refuses")
	require.Contains(t, errOut, "requires_tiller floor", "the nudge printed ahead of the wall")
	require.Contains(t, errOut, "schema_version", "and the wall's own refusal followed it")
}
