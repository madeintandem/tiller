package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/stretchr/testify/require"
)

func TestCheckReadinessInterleavingAndCompletionDeadlock(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	files := map[string]string{
		"tiller.yml":                              readyManifest,
		"design/index.md":                         "# Design\n",
		"epics/office/epic.yml":                   "id: E-office\nkey: office\nstatus: ready\n",
		"epics/domain/epic.yml":                   "id: E-domain\nkey: domain\nstatus: ready\n",
		"epics/office/features/early/feature.yml": "id: F-early1\nkey: office/early\nstatus: done\n",
		"epics/office/features/later/feature.yml": "id: F-later1\nkey: office/later\nstatus: draft\ndepends_on: [F-domain]\n",
		"epics/domain/features/work/feature.yml":  "id: F-domain\nkey: domain/work\nstatus: draft\ndepends_on: [F-early1]\n",
	}
	for path, content := range files {
		full := filepath.Join(root, ".tiller", path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	out, stderr, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 0, code, stderr)
	var answer struct {
		OK   bool             `json:"ok"`
		Data readiness.Report `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &answer))
	require.True(t, answer.OK)
	require.True(t, answer.Data.Ready)
	require.Empty(t, answer.Data.Gaps)
	require.Len(t, answer.Data.Advisories, 1)
	require.Equal(t, readiness.ReasonProjectedEpicOrderCycle, answer.Data.Advisories[0].Reason)
	out, stderr, code = execCLI(t, "check", "readiness")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Advisories (nonblocking)")

	path := filepath.Join(root, ".tiller/epics/domain/features/work/feature.yml")
	require.NoError(t, os.WriteFile(path, []byte("id: F-domain\nkey: domain/work\nstatus: draft\ndepends_on: [E-office]\n"), 0o644))
	out, stderr, code = execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 1, code, stderr)
	require.NoError(t, json.Unmarshal([]byte(out), &answer))
	require.False(t, answer.OK)
	require.False(t, answer.Data.Ready)
	require.Len(t, answer.Data.Gaps, 1)
	require.Equal(t, readiness.ReasonDependencyCycle, answer.Data.Gaps[0].Reason)
	require.Empty(t, answer.Data.Advisories)
}
