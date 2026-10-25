package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func layRemoveCLI(t *testing.T, withTask bool) (epicID, featureID, taskID string) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "proposal", "--title", "Proposal Epic", "--contributor", "jc")
	mustRun(t, "add", "feature", "proposal/obsolete", "--title", "Obsolete Feature", "--contributor", "jc")
	if withTask {
		mustRun(t, "add", "task", "proposal/obsolete/remove-me", "--title", "Obsolete Task", "--contributor", "jc")
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for id, epic := range tree.Epics {
		if epic.Key != nil && *epic.Key == "proposal" {
			epicID = id
		}
	}
	for id, feature := range tree.Features {
		if feature.Key != nil && *feature.Key == "proposal/obsolete" {
			featureID = id
		}
	}
	for id, task := range tree.Tasks {
		if task.Key != nil && *task.Key == "proposal/obsolete/remove-me" {
			taskID = id
		}
	}
	require.NotEmpty(t, epicID)
	require.NotEmpty(t, featureID)
	if withTask {
		require.NotEmpty(t, taskID)
	}
	return
}

func TestRemoveDraftHumanOutputNamesEveryRemovedNodeAndReadoutGuidance(t *testing.T) {
	epicID, featureID, _ := layRemoveCLI(t, false)
	mustRun(t, "add", "feature", "proposal/second", "--title", "Second Feature", "--contributor", "jc")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	var secondID string
	for id, feature := range tree.Features {
		if feature.Key != nil && *feature.Key == "proposal/second" {
			secondID = id
		}
	}

	out, stderr, code := execCLI(t, "remove", "epic", epicID, "--reason", "replace the proposed decomposition", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, fmt.Sprintf("Removed draft epic proposal (%s)", epicID))
	require.Contains(t, out, fmt.Sprintf("feature proposal/obsolete (%s)", featureID))
	require.Contains(t, out, fmt.Sprintf("feature proposal/second (%s)", secondID))
	require.Contains(t, out, "Reason: replace the proposed decomposition")
	require.Contains(t, out, "Agent: re-run the proposal readout with `tiller readout decompose --since <sha>`, naming the commit the last block you reported, before continuing.",
		"a remove is a correction inside the gate, so the hint names the --since form (issue #94)")
}

func TestRemoveDraftTaskJSONCarriesRemovedIDsChangedFilesAndReason(t *testing.T) {
	_, _, taskID := layRemoveCLI(t, true)
	out, stderr, code := execCLI(t, "remove", "task", "proposal/obsolete/remove-me", "--reason", "duplicate task", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Noun         string   `json:"noun"`
			ID           string   `json:"id"`
			Key          string   `json:"key"`
			Reason       string   `json:"reason"`
			RemovedIDs   []string `json:"removed_ids"`
			ChangedFiles []string `json:"changed_files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, "remove", env.Command)
	require.Equal(t, store.NounTask, env.Data.Noun)
	require.Equal(t, taskID, env.Data.ID)
	require.Equal(t, "proposal/obsolete/remove-me", env.Data.Key)
	require.Equal(t, "duplicate task", env.Data.Reason)
	require.Equal(t, []string{taskID}, env.Data.RemovedIDs)
	require.Contains(t, env.Data.ChangedFiles, ".tiller/epics/proposal/features/obsolete/tasks/remove-me/task.yml")
	require.Contains(t, env.Data.ChangedFiles, ".tiller/epics/proposal/features/obsolete/tasks/remove-me/task.md")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(cwd, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me"))
	listOut, listErr, listCode := execCLI(t, "ls", "task")
	require.Equal(t, 0, listCode, listErr)
	require.NotContains(t, listOut, taskID)
	require.NotContains(t, listOut, "proposal/obsolete/remove-me")
	treeOut, treeErr, treeCode := execCLI(t, "tree")
	require.Equal(t, 0, treeCode, treeErr)
	require.NotContains(t, treeOut, taskID)
	require.NotContains(t, treeOut, "proposal/obsolete/remove-me")
	searchOut, searchErr, searchCode := execCLI(t, "search", taskID)
	require.Equal(t, 0, searchCode, searchErr)
	require.NotContains(t, searchOut, "proposal/obsolete/remove-me")
}

func TestRemoveDraftBlockedStateIsExit1WithReturnAndAmendmentGuidance(t *testing.T) {
	_, _, taskID := layRemoveCLI(t, true)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(cwd, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me", "task.yml")
	var task store.Task
	readYAMLCLI(t, path, &task)
	task.Status = "ready"
	writeYAMLCLI(t, path, task)

	out, stderr, code := execCLI(t, "remove", "task", taskID, "--reason", "not needed", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, stderr)
	require.Contains(t, out, `"ok": false`)
	require.Contains(t, out, `"reason": "remove_state_locked"`)
	require.Contains(t, out, "tiller return task proposal/obsolete/remove-me --reason")
	require.Contains(t, out, "tiller add amendment")
	require.FileExists(t, path)
}

func TestRemoveDraftProposedAmendmentReferenceIsExit1AndWritesNothing(t *testing.T) {
	_, _, taskID := layRemoveCLI(t, true)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(cwd, ".tiller", "amendments")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	key := "task-removal"
	amendmentPath := filepath.Join(dir, "A-rem001-task-removal.yml")
	writeYAMLCLI(t, amendmentPath, store.Amendment{ID: "A-rem001", Key: &key, Status: store.AmendmentStatusProposed, Operations: []store.AmendmentOp{{Op: store.AmendmentOpRevise, Target: taskID}}})
	before, err := os.ReadFile(amendmentPath)
	require.NoError(t, err)

	_, stderr, code := execCLI(t, "remove", "task", taskID, "--reason", "referenced", "--contributor", "jc")
	require.Equal(t, 1, code, stderr)
	require.Contains(t, stderr, "Blocked")
	require.Contains(t, stderr, taskID)
	require.Contains(t, stderr, "proposed amendment")
	after, readErr := os.ReadFile(amendmentPath)
	require.NoError(t, readErr)
	require.Equal(t, before, after)
}

func TestRemoveDraftGrammarAndReasonMisuseExit2(t *testing.T) {
	_, _, taskID := layRemoveCLI(t, true)
	cases := [][]string{
		{"remove", "task", taskID, "--contributor", "jc"},
		{"remove", "task", taskID, "--reason", "   ", "--contributor", "jc"},
		{"remove", "question", "Q-aaaaaa", "--reason", "no", "--contributor", "jc"},
		{"remove", "task", taskID, "extra", "--reason", "no", "--contributor", "jc"},
	}
	for _, args := range cases {
		_, _, code := execCLI(t, args...)
		require.Equal(t, 2, code, args)
	}
	out, _, code := execCLI(t, "remove", "task", taskID, "--reason", " ", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "remove_reason_required"`)
}

func TestRemoveBugCommand(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "bug", "alpha", "--title", "A", "--contributor", "jc")
	out, stderr, code := execCLI(t, "remove", "bug", "alpha", "--reason", "dup", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Removed proposed bug bugs/alpha (B-")
	_, _, code = execCLI(t, "show", "bug", "alpha")
	require.Equal(t, 2, code)
}

func readYAMLCLI(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, out))
}

func writeYAMLCLI(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestRemoveDraftKeylessFeatureByIDIsMisuseWithoutPanicOrWrite(t *testing.T) {
	_, featureID, _ := layRemoveCLI(t, false)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(cwd, ".tiller", "epics", "proposal", "features", "obsolete", "feature.yml")
	var feature store.Feature
	readYAMLCLI(t, path, &feature)
	feature.Key = nil
	writeYAMLCLI(t, path, feature)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	var out, stderr string
	var code int
	require.NotPanics(t, func() {
		out, stderr, code = execCLI(t, "remove", "feature", featureID, "--reason", "keyless", "--contributor", "jc", "--json")
	})
	require.Equal(t, 2, code, stderr)
	require.Contains(t, out, `"reason": "error"`)
	require.Contains(t, out, "feature "+featureID+" carries no key, so its folder cannot be named")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
