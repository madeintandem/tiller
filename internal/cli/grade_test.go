package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func layGradeCLI(t *testing.T, severity string) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "bug", "blank-on-reload", "--title", "Blank on reload", "--severity", severity, "--contributor", "jc")
}

func TestGradeHumanLine(t *testing.T) {
	layGradeCLI(t, "minor")
	out, stderr, code := execCLI(t, "grade", "bug", "blank-on-reload", "important", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "severity minor → important.")
	require.Contains(t, out, "Bug bugs/blank-on-reload")

	out, stderr, code = execCLI(t, "grade", "bug", "blank-on-reload", "important", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "unchanged")
}

func TestGradeJSONPayload(t *testing.T) {
	layGradeCLI(t, "minor")
	out, stderr, code := execCLI(t, "grade", "bug", "blank-on-reload", "blocker", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Noun        string `json:"noun"`
			ID          string `json:"id"`
			Key         string `json:"key"`
			OldSeverity string `json:"old_severity"`
			Severity    string `json:"severity"`
			Changed     bool   `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, "grade", env.Command)
	require.Equal(t, "bug", env.Data.Noun)
	require.Equal(t, "bugs/blank-on-reload", env.Data.Key)
	require.Equal(t, "minor", env.Data.OldSeverity)
	require.Equal(t, "blocker", env.Data.Severity)
	require.True(t, env.Data.Changed)
}

func TestGradeChoreIsUnknownNoun(t *testing.T) {
	layGradeCLI(t, "minor")
	out, _, code := execCLI(t, "grade", "chore", "x", "minor", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "bad_args"`)
	require.Contains(t, out, "bug")
}

func TestGradeRefusesBadLevelAndArity(t *testing.T) {
	layGradeCLI(t, "minor")
	out, _, code := execCLI(t, "grade", "bug", "blank-on-reload", "urgent", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "invalid_severity"`)
	_, _, code = execCLI(t, "grade", "bug", "blank-on-reload", "--contributor", "jc")
	require.Equal(t, 2, code)
}
