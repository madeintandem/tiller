package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// ownedManifest is a manifest with one owner, the only environment precondition
// the add verbs need.
const ownedManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// idTokenRe matches a minted node id so the golden tests can normalize the
// random token to a fixed placeholder, the sanctioned alternative to injecting a
// minter into the production command.
var idTokenRe = regexp.MustCompile(`([EFT])-[a-z0-9]{6}`)

func normalizeIDs(s string) string {
	return idTokenRe.ReplaceAllString(s, "$1-xxxxxx")
}

// initOwnedRepo lays a temp repo with an owner-bearing manifest and chdirs into
// it, so the add commands resolve a repo root and a valid owner. It is a real
// git repo with one commit from the start: approve and sync both pin HEAD as
// the commit anchor now (artifact-schema.md, "Accepted basis"), and HEAD cannot
// resolve before a first commit exists.
func initOwnedRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(ownedManifest), 0o644))
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
}

// TestAddEpicIdempotent confirms a second add of the same key is a no-op success
// (exit 0) reporting created false.
func TestAddEpicIdempotent(t *testing.T) {
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"created\": false")
}

// TestAddTaskAppendsSecond confirms a distinct intent appends a second task.
func TestAddTaskAppendsSecond(t *testing.T) {
	initOwnedRepo(t)
	execCLI(t, "add", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "add", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	_, _, code := execCLI(t, "add", "task", "form-engine/certification-form-loading/first-task", "--title", "First task", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", "form-engine/certification-form-loading/second-task", "--title", "Second task", "--contributor", "jc")
	require.Equal(t, 0, code)

	// Read the ledger straight off disk: two tasks.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Tasks, 2)
}

// TestAddOwnerUnsetExits3 confirms a missing owner is an environment refusal.
func TestAddOwnerUnsetExits3(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "")
	initOwnedRepo(t)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, store.ReasonContributorUnset)
}

// TestAddEpicUnknownOwnerExits3 confirms an owner absent from the project refuses.
func TestAddEpicUnknownOwnerExits3(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--contributor", "nobody", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, store.ReasonContributorUnknown)
}

// TestAddFeatureUnknownEpicExits2 confirms a feature under a missing epic is a
// misuse (exit 2) with the not_found reason.
func TestAddFeatureUnknownEpicExits2(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "add", "feature", "no-such/feature", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, reasonNotFound)
}

// TestAddEpicBadKeyExits2 confirms a malformed key is rejected as misuse.
func TestAddEpicBadKeyExits2(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "add", "epic", "Bad Key", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "invalid_key")
}

// TestAddOutsideRepoExits3 confirms add refuses outside a Tiller repo.
func TestAddOutsideRepoExits3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "add", "epic", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 3, code)
}

func TestAddExistingTitleMismatchSuggestsCopyableRenameForEveryNode(t *testing.T) {
	epicID, featureID, taskID := layRenameCLI(t)

	tests := []struct {
		name, noun, key, title, id, status string
	}{
		{name: "epic", noun: "epic", key: "foundation", title: "Platform Foundation", id: epicID, status: "draft"},
		{name: "feature", noun: "feature", key: "foundation/app", title: "Application Platform", id: featureID, status: "draft"},
		{name: "task", noun: "task", key: "foundation/app/scaffold", title: "Scaffold the app", id: taskID, status: "proposed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, stderr, code := execCLI(t, "add", tt.noun, tt.key, "--title", tt.title, "--contributor", "jc")
			require.Equal(t, 0, code, stderr)
			require.Equal(t, fmt.Sprintf(
				"Found existing %s %s (%s) in %s; unchanged.\nTo change its display title:\n  tiller rename %s %s --title '%s'\n",
				tt.noun, tt.key, tt.id, tt.status, tt.noun, tt.id, tt.title,
			), out)
		})
	}
}

func TestAddExistingSameTitleKeepsOneLineAndEmptyTitleDoesNotSuggestRename(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)

	out, stderr, code := execCLI(t, "add", "epic", "foundation", "--title", "Phase 0 Foundation", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Found existing epic foundation (%s) in draft.\n", epicID), out)
	require.NotContains(t, out, "tiller rename")

	out, stderr, code = execCLI(t, "add", "epic", "foundation", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, fmt.Sprintf("Found existing epic foundation (%s) in draft.\n", epicID), out)
	require.NotContains(t, out, "tiller rename")
}

func TestAddExistingTitleMismatchJSONRemainsStable(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	out, stderr, code := execCLI(t, "add", "epic", "foundation", "--title", "Platform Foundation", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, fmt.Sprintf(`"id": %q`, epicID))
	require.Contains(t, out, `"title": "Phase 0 Foundation"`)
	require.Contains(t, out, `"created": false`)
	require.NotContains(t, out, "tiller rename")
	require.NotContains(t, out, "old_title")
}

func TestAddExistingTitleMismatchShellQuotesSuggestedRename(t *testing.T) {
	epicID, _, _ := layRenameCLI(t)
	tests := []struct {
		name, title, quoted string
	}{
		{name: "command substitution", title: "$(printf PWNED)", quoted: "'$(printf PWNED)'"},
		{name: "backticks", title: "`printf PWNED`", quoted: "'`printf PWNED`'"},
		{name: "dollar expansion", title: "$HOME", quoted: "'$HOME'"},
		{name: "whitespace", title: "Platform Foundation", quoted: "'Platform Foundation'"},
		{name: "apostrophe", title: "Builder's Platform", quoted: "'Builder'\"'\"'s Platform'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, stderr, code := execCLI(t, "add", "epic", "foundation", "--title", tt.title, "--contributor", "jc")
			require.Equal(t, 0, code, stderr)
			require.Equal(t, fmt.Sprintf(
				"Found existing epic foundation (%s) in draft; unchanged.\n"+
					"To change its display title:\n  tiller rename epic %s --title %s\n",
				epicID, epicID, tt.quoted,
			), out)

			command := strings.TrimSpace(strings.Split(out, "\n")[2])
			arg := strings.TrimPrefix(command, fmt.Sprintf("tiller rename epic %s --title ", epicID))
			reproduced, err := exec.Command("sh", "-c", "printf '%s' "+arg).Output()
			require.NoError(t, err)
			require.Equal(t, tt.title, string(reproduced), "the displayed POSIX argument reproduces the title literally")
		})
	}
}

func TestAddBugAndChoreCommands(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/loading", "--title", "Loading", "--contributor", "jc")

	out, stderr, code := execCLI(t, "add", "bug", "blank-on-reload", "--title", "Blank on reload",
		"--attach", "form-engine/loading", "--source", "docs/feedback/reload.md#L1-9", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "bug", data["noun"])
	require.Equal(t, "bugs/blank-on-reload", data["key"])
	require.Regexp(t, `^F-[a-z0-9]{6}$`, data["attached_to"])

	// The human line names the attachment by the shared reference convention,
	// key path with the id in parentheses, not the bare stored id.
	out, stderr, code = execCLI(t, "add", "bug", "blank-on-reload", "--title", "Blank on reload",
		"--attach", "form-engine/loading", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, fmt.Sprintf("(attached to form-engine/loading (%s))", data["attached_to"]))

	out, stderr, code = execCLI(t, "add", "chore", "bump-rails", "--title", "Bump Rails", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "chores/bump-rails (CH-")

	_, _, code = execCLI(t, "add", "bug", "bugs/x", "--title", "x", "--contributor", "jc")
	require.Equal(t, 2, code)
	out, _, code = execCLI(t, "add", "epic", "bugs", "--title", "Bugs", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "reserved_key"`)
}

func TestTweakCommandsPreserveShelfPeers(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "product", "--title", "Product", "--contributor", "jc")
	mustRun(t, "add", "feature", "product/button", "--title", "Button", "--contributor", "jc")
	mustRun(t, "add", "bug", "defect", "--title", "Defect", "--contributor", "jc")
	mustRun(t, "add", "chore", "upkeep", "--title", "Upkeep", "--contributor", "jc")
	bugBefore := readFile(t, ".tiller/bugs/defect/bug.yml")
	choreBefore := readFile(t, ".tiller/chores/upkeep/chore.yml")
	out, stderr, code := execCLI(t, "add", "tweaks", "spacing", "--title", "Adjust spacing", "--attach", "product/button", "--depends-on", "bugs/defect", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "tweak", data["noun"])
	require.Equal(t, "tweaks/spacing", data["key"])
	require.Regexp(t, `^TW-[a-z0-9]{6}$`, data["id"])
	require.FileExists(t, ".tiller/tweaks/spacing/tweak.md")
	for _, noun := range []string{"tweak", "tweaks"} {
		out, stderr, code = execCLI(t, "ls", noun, "--json")
		require.Equal(t, 0, code, stderr)
		require.Contains(t, out, "tweaks/spacing")
	}
	out, stderr, code = execCLI(t, "show", "tweak", data["id"].(string), "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Adjust spacing")
	mustRun(t, "unlink", "tweak", "spacing", "--depends-on", "bugs/defect", "--contributor", "jc")
	mustRun(t, "link", "tweak", "spacing", "--depends-on", "chores/upkeep", "--contributor", "jc")
	mustRun(t, "order", "tweak", "spacing", "--contributor", "jc")
	mustRun(t, "rename", "tweak", "spacing", "--key", "button-spacing", "--contributor", "jc")
	mustRun(t, "resolve", "tweak", "button-spacing", "--by", "product/button", "--contributor", "jc")
	mustRun(t, "resolve", "tweak", "button-spacing", "--clear", "--contributor", "jc")
	require.Equal(t, bugBefore, readFile(t, ".tiller/bugs/defect/bug.yml"))
	require.Equal(t, choreBefore, readFile(t, ".tiller/chores/upkeep/chore.yml"))
}
