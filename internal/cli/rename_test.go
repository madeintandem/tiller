package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func layRenameCLI(t *testing.T) (epicID, featureID, taskID string) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "foundation", "--title", "Phase 0 Foundation", "--source", "design/platform.md", "--contributor", "jc")
	mustRun(t, "add", "feature", "foundation/app", "--title", "App", "--source", "design/app.md", "--contributor", "jc")
	mustRun(t, "add", "task", "foundation/app/scaffold", "--title", "Scaffold", "--contributor", "jc")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for id, epic := range tree.Epics {
		if epic.Key != nil && *epic.Key == "foundation" {
			epicID = id
		}
	}
	for id, feature := range tree.Features {
		if feature.Key != nil && *feature.Key == "foundation/app" {
			featureID = id
		}
	}
	for id, task := range tree.Tasks {
		if task.Key != nil && *task.Key == "foundation/app/scaffold" {
			taskID = id
		}
	}
	require.NotEmpty(t, epicID)
	require.NotEmpty(t, featureID)
	require.NotEmpty(t, taskID)
	return epicID, featureID, taskID
}

func TestRenameTitleHumanOutputNamesDisplayTitleAndFullReference(t *testing.T) {
	epicID, featureID, taskID := layRenameCLI(t)

	out, stderr, code := execCLI(t, "rename", "epic", epicID, "--title", "Platform Foundation", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Renamed epic foundation (%s) display title from %q to %q.\n", epicID, "Phase 0 Foundation", "Platform Foundation"), out)

	out, stderr, code = execCLI(t, "rename", "feature", "foundation/app", "--title", "Application Platform", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Renamed feature foundation/app (%s) display title from %q to %q.\n", featureID, "App", "Application Platform"), out)

	out, stderr, code = execCLI(t, "rename", "task", "foundation/app/scaffold", "--title", "Scaffold the app", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Renamed task foundation/app/scaffold (%s) display title from %q to %q.\n", taskID, "Scaffold", "Scaffold the app"), out)
}

func TestRenameTitleJSONKeepsIdentityAndTitlesSeparate(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "epic", epicID, "--title", "Platform Foundation", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)

	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Noun     string `json:"noun"`
			ID       string `json:"id"`
			Key      string `json:"key"`
			OldTitle string `json:"old_title"`
			Title    string `json:"title"`
			Changed  bool   `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, "rename", env.Command)
	require.Equal(t, "epic", env.Data.Noun)
	require.Equal(t, epicID, env.Data.ID)
	require.Equal(t, "foundation", env.Data.Key)
	require.Equal(t, "Phase 0 Foundation", env.Data.OldTitle)
	require.Equal(t, "Platform Foundation", env.Data.Title)
	require.True(t, env.Data.Changed)
}

func TestRenameTitleSameTitleIsHumanNoChange(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "epic", epicID, "--title", "Phase 0 Foundation", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Epic foundation (%s) display title is already %q; unchanged.\n", epicID, "Phase 0 Foundation"), out)
}

func TestRenameTitleRefusesInvalidGrammarAtExit2(t *testing.T) {
	_, _, _ = layRenameCLI(t)

	_, _, code := execCLI(t, "rename", "question", "Q-aaaaaa", "--title", "No", "--contributor", "jc")
	require.Equal(t, 2, code)
	_, _, code = execCLI(t, "rename", "epic", "foundation", "--contributor", "jc")
	require.Equal(t, 2, code)
	_, _, code = execCLI(t, "rename", "epic", "foundation", "extra", "--title", "No", "--contributor", "jc")
	require.Equal(t, 2, code)

	out, _, code := execCLI(t, "rename", "epic", "foundation", "--title", " \t ", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "invalid_title"`)
	require.Contains(t, out, "display title")
}

func TestRenameKeyHumanOutputNamesOldNewKeyIdentityAndEveryMove(t *testing.T) {
	_, _, taskID := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "task", taskID, "--key", "foundation/app/bootstrap", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf(
		"Renamed task key from foundation/app/scaffold (%s) to foundation/app/bootstrap (%s); identity preserved.\n"+
			"Moved files:\n"+
			"  .tiller/epics/foundation/features/app/tasks/bootstrap/task.md\n"+
			"  .tiller/epics/foundation/features/app/tasks/bootstrap/task.yml\n",
		taskID, taskID,
	), out)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(cwd, ".tiller", "epics", "foundation", "features", "app", "tasks", "scaffold"))
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Equal(t, taskID, tree.Tasks[taskID].ID)
	require.Equal(t, "foundation/app/bootstrap", *tree.Tasks[taskID].Key)
}

func TestRenameKeyJSONCarriesAxisIdentityKeysAndMovedFiles(t *testing.T) {
	_, _, taskID := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "task", "foundation/app/scaffold",
		"--key", "foundation/app/bootstrap", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)

	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Axis    string   `json:"axis"`
			Noun    string   `json:"noun"`
			ID      string   `json:"id"`
			OldKey  string   `json:"old_key"`
			Key     string   `json:"key"`
			Moved   []string `json:"moved"`
			Changed bool     `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, "rename", env.Command)
	require.Equal(t, "key", env.Data.Axis)
	require.Equal(t, store.NounTask, env.Data.Noun)
	require.Equal(t, taskID, env.Data.ID)
	require.Equal(t, "foundation/app/scaffold", env.Data.OldKey)
	require.Equal(t, "foundation/app/bootstrap", env.Data.Key)
	require.Equal(t, []string{
		".tiller/epics/foundation/features/app/tasks/bootstrap/task.md",
		".tiller/epics/foundation/features/app/tasks/bootstrap/task.yml",
	}, env.Data.Moved)
	require.True(t, env.Data.Changed)
}

func TestRenameAxisRequiresExactlyOneTitleOrKey(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)

	out, _, code := execCLI(t, "rename", "epic", epicID,
		"--title", "Platform Foundation", "--key", "platform-foundation", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "multiple_rename_axes"`)
	require.Contains(t, out, "exactly one of --title or --key")

	out, _, code = execCLI(t, "rename", "epic", epicID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "missing_rename_axis"`)
	require.Contains(t, out, "exactly one of --title or --key")
}

// TestRenameKeyRefusesReservedEpicKeyCLI is the command-surface half of the
// refusal `add epic bugs` already makes: the shelf folder names are reserved
// at every writer of an epic key, not only the first one.
func TestRenameKeyRefusesReservedEpicKeyCLI(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	out, _, code := execCLI(t, "rename", "epic", epicID, "--key", "bugs", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "reserved_key"`)
}

func TestRenameTitleAxisContractIsUnchangedAfterKeyAxisAdded(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "epic", epicID, "--title", "Platform Foundation", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, out, `"axis"`)
	require.Contains(t, out, `"old_title": "Phase 0 Foundation"`)
	require.Contains(t, out, `"title": "Platform Foundation"`)
}

func TestRenameKeyNoOpJSONNormalizesMovedToEmptyArray(t *testing.T) {
	_, _, taskID := layRenameCLI(t)
	out, stderr, code := execCLI(t, "rename", "task", taskID,
		"--key", "foundation/app/scaffold", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, `"moved": []`)
	require.NotContains(t, out, `"moved": null`)
}

func TestCommittedCorrectionsIgnoreRetiredBoard(t *testing.T) {
	type correctionCase struct {
		name  string
		setup func(*testing.T) ([]string, func(*testing.T))
	}
	cases := []correctionCase{
		{
			name: "rename title",
			setup: func(t *testing.T) ([]string, func(*testing.T)) {
				epicID, _, _ := layRenameCLI(t)
				return []string{"rename", "epic", epicID, "--title", "Platform Foundation", "--contributor", "jc"}, func(t *testing.T) {
					root, err := repoRoot()
					require.NoError(t, err)
					tree, err := store.Load(root)
					require.NoError(t, err)
					require.Equal(t, "Platform Foundation", tree.Epics[epicID].Title)
				}
			},
		},
		{
			name: "rename key",
			setup: func(t *testing.T) ([]string, func(*testing.T)) {
				_, _, taskID := layRenameCLI(t)
				return []string{"rename", "task", taskID, "--key", "foundation/app/bootstrap", "--contributor", "jc"}, func(t *testing.T) {
					root, err := repoRoot()
					require.NoError(t, err)
					tree, err := store.Load(root)
					require.NoError(t, err)
					require.Equal(t, "foundation/app/bootstrap", *tree.Tasks[taskID].Key)
				}
			},
		},
		{
			name: "unlink",
			setup: func(t *testing.T) ([]string, func(*testing.T)) {
				layUnlinkCLI(t)
				return []string{"unlink", "feature", "form-engine/rendering-modes",
					"--depends-on", "form-engine/schema-engine", "--contributor", "jc"}, func(t *testing.T) {
					root, err := repoRoot()
					require.NoError(t, err)
					tree, err := store.Load(root)
					require.NoError(t, err)
					for _, feature := range tree.Features {
						if feature.Key != nil && *feature.Key == "form-engine/rendering-modes" {
							targetID := correctionFeatureIDByKey(tree, "form-engine/schema-engine")
							require.NotEmpty(t, targetID)
							require.NotContains(t, feature.DependsOn, targetID)
							return
						}
					}
					require.Fail(t, "feature not found")
				}
			},
		},
		{
			name: "remove",
			setup: func(t *testing.T) ([]string, func(*testing.T)) {
				_, _, taskID := layRemoveCLI(t, true)
				return []string{"remove", "task", taskID, "--reason", "duplicate proposal", "--contributor", "jc"}, func(t *testing.T) {
					root, err := repoRoot()
					require.NoError(t, err)
					tree, err := store.Load(root)
					require.NoError(t, err)
					require.NotContains(t, tree.Tasks, taskID)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" json", func(t *testing.T) {
			args, assertCommitted := tc.setup(t)
			beforeEvents := correctionEventCount(t)
			obstructRetiredBoard(t)
			args = append(args, "--json")
			out, stderr, code := execCLI(t, args...)
			require.Equal(t, 0, code, stderr)
			data := jsonData(t, out)
			require.NotContains(t, data, "warning")
			assertCommitted(t)
			require.Equal(t, beforeEvents+1, correctionEventCount(t))

			root, err := repoRoot()
			require.NoError(t, err)
			require.DirExists(t, filepath.Join(root, ".tiller", "board.md"))
		})

		t.Run(tc.name+" human", func(t *testing.T) {
			args, assertCommitted := tc.setup(t)
			beforeEvents := correctionEventCount(t)
			obstructRetiredBoard(t)
			out, stderr, code := execCLI(t, args...)
			require.Equal(t, 0, code, stderr)
			require.NotContains(t, out, "Warning:")
			assertCommitted(t)
			require.Equal(t, beforeEvents+1, correctionEventCount(t))
		})
	}
}

func obstructRetiredBoard(t *testing.T) {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "board.md")
	require.NoFileExists(t, path)
	require.NoError(t, os.Mkdir(path, 0o755))
}

func correctionEventCount(t *testing.T) int {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	tree, err := store.Load(root)
	require.NoError(t, err)
	evs, err := events.List(tree)
	require.NoError(t, err)
	return len(evs)
}

func correctionFeatureIDByKey(tree *store.Tree, key string) string {
	for id, feature := range tree.Features {
		if feature.Key != nil && *feature.Key == key {
			return id
		}
	}
	return ""
}

// TestRenameBugCommand pins the shelf grain at the CLI surface: --title
// renames at any state, and --key takes one local segment and reports the
// canonical key back.
func TestRenameBugCommand(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "bug", "alpha", "--title", "A", "--contributor", "jc")
	out, stderr, code := execCLI(t, "rename", "bug", "alpha", "--title", "Alpha two", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Renamed bug bugs/alpha (B-")
	out, stderr, code = execCLI(t, "rename", "bug", "alpha", "--key", "beta", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "bugs/beta", jsonData(t, out)["key"])
}
