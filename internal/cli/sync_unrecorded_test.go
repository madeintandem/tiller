package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

var unrecordedMergeFixture = []mutate.UnrecordedMerge{{
	Noun: store.NounBug, ID: "B-aaaaaa", Key: "bugs/alpha", Status: "ready",
	Commits: []mutate.NamingCommit{
		{Commit: "1234567890abcdef1234567890abcdef12345678", Subject: "B-aaaaaa Preserve Inbox capture order"},
	},
}}

const unrecordedMergeLine = `  - bug bugs/alpha (B-aaaaaa) is ready with no recorded submission, but commit 1234567 ` +
	`"B-aaaaaa Preserve Inbox capture order" names it; next: tiller status bug B-aaaaaa`

// The run that used to print the clean-run line over an unrecorded merge now
// names the node, its status, the commit, and the next step, and drops the
// line that read as success.
func TestSyncHumanWarnsOfAnUnrecordedMergeInsteadOfTheCleanRunLine(t *testing.T) {
	out := humanSync(&store.Tree{}, mutate.SyncResult{Into: "main", UnrecordedMerges: unrecordedMergeFixture})
	require.NotContains(t, out, "Sync: no merged branches to record")
	require.Contains(t, out, "Warning: merged into main with no recorded submission (1):")
	require.Contains(t, out, unrecordedMergeLine)
	require.Contains(t, out, "Sync recorded nothing for these and moved no status")
	require.Contains(t, out, "its own start, check, and submit")
	require.Contains(t, out, "--recover-merged")
}

func TestSyncHumanWarnsOfAnUnrecordedMergeBesideCrossings(t *testing.T) {
	taskKey := "form-engine/certification-form-loading/transcribe-the-schema"
	tree := &store.Tree{Tasks: map[string]*store.Task{"T-aaaaaa": {ID: "T-aaaaaa", Key: &taskKey}}}
	out := humanSync(tree, mutate.SyncResult{
		Into:             "main",
		Crossings:        []mutate.Crossing{{Noun: store.NounTask, ID: "T-aaaaaa", Key: taskKey, Status: "done", Basis: "ancestry"}},
		UnrecordedMerges: unrecordedMergeFixture,
	})
	require.Contains(t, out, "Sync: recorded 1 done crossing(s):")
	require.Contains(t, out, unrecordedMergeLine)
}

func TestSyncHumanOmitsTheUnrecordedMergeBlockWhenEmpty(t *testing.T) {
	out := humanSync(&store.Tree{}, mutate.SyncResult{Into: "main"})
	require.Equal(t, "Sync: no merged branches to record.", out)
}

func TestSyncViewUnrecordedMergesJSON(t *testing.T) {
	raw, err := json.Marshal(syncView(mutate.SyncResult{UnrecordedMerges: unrecordedMergeFixture}))
	require.NoError(t, err)
	var got struct {
		UnrecordedMerges []map[string]any `json:"unrecorded_merges"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, []map[string]any{{
		"noun":   "bug",
		"id":     "B-aaaaaa",
		"key":    "bugs/alpha",
		"status": "ready",
		"commits": []any{map[string]any{
			"commit":  "1234567890abcdef1234567890abcdef12345678",
			"subject": "B-aaaaaa Preserve Inbox capture order",
		}},
		"next": "tiller status bug B-aaaaaa",
		"warning": `bug bugs/alpha (B-aaaaaa) is ready with no recorded submission, but commit 1234567 ` +
			`"B-aaaaaa Preserve Inbox capture order" names it; next: tiller status bug B-aaaaaa`,
	}}, got.UnrecordedMerges)

	emptyRaw, err := json.Marshal(syncView(mutate.SyncResult{}))
	require.NoError(t, err)
	require.NotContains(t, string(emptyRaw), "unrecorded_merges", "an ordinary sync's envelope is unchanged")
}

// TestSyncUnrecordedMergeCLI is the report end to end: an approved bug nobody
// started, a fix commit naming it on main, and a sync that used to answer "no
// merged branches to record". Sync warns in text and JSON, the readout shows
// the same finding on a later run, nothing moves, and the warning stops when
// the node is cancelled.
func TestSyncUnrecordedMergeCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--severity", "medium", "--contributor", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "bugs", "alpha", "bug.md"),
		[]byte("# Alpha\n\n## Reproduction\n\nx\n"), 0o644))
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "Approve one bug")
	var bugID string
	for id := range loadCwdTree(t).Bugs {
		bugID = id
	}

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Sync: no merged branches to record.", "an approval commit is state, not a merge")
	require.NotContains(t, out, "no recorded submission")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fix\n"), 0o644))
	gitRun(t, dir, "add", "fix.txt")
	gitRun(t, dir, "commit", "-m", bugID+" Preserve Inbox capture order")

	finding := "bug bugs/alpha (" + bugID + ") is ready with no recorded submission, but commit "
	next := "tiller status bug " + bugID
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, "a warning is a true report of the world, not a refusal")
	require.NotContains(t, out, "no merged branches to record")
	require.Contains(t, out, "Warning: merged into main with no recorded submission (1):")
	require.Contains(t, out, finding)
	require.Contains(t, out, `"`+bugID+` Preserve Inbox capture order" names it; next: `+next)
	require.Contains(t, out, "**Needs attention**", "the gate's trailing readout carries the finding too")

	jout, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, `"unrecorded_merges"`)
	require.Contains(t, jout, `"status": "ready"`)
	require.Contains(t, jout, `"next": "`+next+`"`)
	require.Contains(t, jout, `"crossings": []`)
	require.Equal(t, "ready", loadCwdTree(t).Bugs[bugID].Status, "the warning moves no status")

	readout, _, code := execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.Contains(t, readout, "**Needs attention**")
	require.Contains(t, readout, "- "+finding)
	require.Contains(t, readout, "names it · `"+next+"`")

	mustRun(t, "cancel", "bug", "alpha", "--reason", "fixed out of band", "--resolution", "wontfix", "--contributor", "jc")
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "no recorded submission")
	readout, _, code = execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.NotContains(t, readout, "no recorded submission")
}
