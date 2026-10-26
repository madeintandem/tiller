package cli

// These tests pin the shelf nouns' read surface (docs/spec/cli-surface.md): `ls
// bugs`/`ls chores` list the shelves the same way any other plural noun lists,
// `show bug <addr>` resolves by id or key, and `status bug <addr>` answers the
// typed oracle. Task 4 lands `add bug`; until then the shelf files are laid
// directly.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// layShelfFilesCLI lays one bug directly on disk (bug.yml plus bug.md) in a
// fresh owned repo, standing in for `add bug` until Task 4 lands it.
func layShelfFilesCLI(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	dir := filepath.Join(".tiller", "bugs", "blank-on-reload")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bug.yml"),
		[]byte("id: B-7k2p9x\nkey: bugs/blank-on-reload\ntitle: Blank on reload\nstatus: proposed\nblocked: false\nproposed_by: jc\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bug.md"), []byte("# Blank on reload\n"), 0o644))
}

// TestLsAndShowAcceptShelfNouns pins the three read verbs over the shelf
// nouns: ls bugs lists the laid bug, show bug resolves it by local key, and
// status bug answers the typed oracle. ls chores confirms an empty shelf
// lists as an empty array, not an error.
func TestLsAndShowAcceptShelfNouns(t *testing.T) {
	layShelfFilesCLI(t)
	out, stderr, code := execCLI(t, "ls", "bugs")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "bugs/blank-on-reload (B-7k2p9x)")
	out, stderr, code = execCLI(t, "show", "bug", "blank-on-reload", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "bug", data["noun"])
	require.Equal(t, "bugs/blank-on-reload", data["key"])
	out, stderr, code = execCLI(t, "status", "bug", "B-7k2p9x", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, `"legal_next"`)
	out, _, code = execCLI(t, "ls", "chores", "--json")
	require.Equal(t, 0, code)
	// ListResult's array field is "nodes" (views.ListResult), not "items";
	// the brief's snippet named the field "items", a slip against the
	// envelope's actual shape, corrected here to what ls --json really prints.
	require.Contains(t, out, `"nodes": []`)
}

func TestListContributorStartsAtExecutionBoundary(t *testing.T) {
	layApprovedFeatureWithTask(t)
	readCount := func(args ...string) float64 {
		t.Helper()
		out, stderr, code := execCLI(t, append(args, "--json")...)
		require.Equal(t, 0, code, stderr)
		return jsonData(t, out)["count"].(float64)
	}
	require.Equal(t, float64(1), readCount("ls", "tasks", "--owner", "jc", "--status", "unfinished"))
	require.Zero(t, readCount("ls", "features", "--contributor", "jc"), "a reservation is not a contribution")
	mustRun(t, "start", "task", soleTaskID(t), "--contributor", "jc")
	require.Equal(t, float64(1), readCount("ls", "features", "--contributor", "jc"))
	require.Equal(t, float64(1), readCount("ls", "epics", "--contributor", "jc"))
	require.Zero(t, readCount("ls", "features", "--owner", "jc"))
}
