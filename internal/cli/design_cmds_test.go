package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func layDesignCLI(t *testing.T) string {
	t.Helper()
	initOwnedRepo(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	manifest := strings.Replace(ownedManifest, "kind: greenfield", "kind: brownfield", 1) + "inputs:\n  code:\n    - { root: app, mode: surface }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), []byte(manifest), 0o644))
	for _, dir := range []string{".tiller/design/as-is", "app", "docs"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}
	for path, text := range map[string]string{
		".tiller/design/as-is/one.md": "# One\nCurrent behavior.\n",
		".tiller/design/as-is/two.md": "# Two\nOther behavior.\n",
		"app/one,two.go":              "package app\n",
		"app/other.go":                "package app\n",
		"docs/one,two.md":             "# Original\nRecover this source.\n",
		"docs/second.md":              "# Second original\nAdditional source.\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(text), 0o644))
	}
	gitRun(t, root, "add", ".tiller/tiller.yml", ".tiller/design", "app", "docs")
	gitRun(t, root, "commit", "-m", "baseline inputs")
	return root
}

func designCLIData(t *testing.T, args ...string) map[string]any {
	t.Helper()
	args = append(args, "--contributor", "jc", "--json")
	out, stderr, code := execCLI(t, args...)
	require.Equal(t, 0, code, out+stderr)
	return jsonData(t, out)
}

func TestDesignLinkLiteralPathsAndIdempotency(t *testing.T) {
	root := layDesignCLI(t)
	args := []string{"link", "design", "as-is/one.md", "--describes", "app/one,two.go", "--describes", "app/other.go", "--from", "docs/one,two.md", "--from", "docs/second.md"}
	data := designCLIData(t, args...)
	require.Equal(t, "design", data["noun"])
	require.Equal(t, "as-is/one.md", data["path"])
	require.Equal(t, true, data["changed"])
	require.NotContains(t, data, "id")
	require.NotContains(t, data, "status")
	require.NotContains(t, data, "readout")
	require.Equal(t, map[string]any{"id": "jc", "source": "--contributor"}, data["actor"])
	ledger, err := store.LoadBaseline(root)
	require.NoError(t, err)
	require.Len(t, ledger.Docs, 1)
	require.Equal(t, []string{"app/one,two.go", "app/other.go"}, ledger.Docs[0].Describes)
	require.Len(t, ledger.Docs[0].DerivedFrom, 2)
	require.Equal(t, "docs/one,two.md", ledger.Docs[0].DerivedFrom[0].Path)
	require.Equal(t, "docs/second.md", ledger.Docs[0].DerivedFrom[1].Path)
	require.Nil(t, ledger.Docs[0].Confirmed)

	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	data = designCLIData(t, args...)
	require.Equal(t, false, data["changed"])
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))

	data = designCLIData(t, "unlink", "design", "as-is/one.md", "--describes", "app/one,two.go", "--from", "docs/one,two.md")
	require.Equal(t, true, data["changed"])
	ledger, err = store.LoadBaseline(root)
	require.NoError(t, err)
	require.Equal(t, []string{"app/other.go"}, ledger.Docs[0].Describes)
	require.Len(t, ledger.Docs[0].DerivedFrom, 1)
	require.Equal(t, "docs/second.md", ledger.Docs[0].DerivedFrom[0].Path)
	before = snapshotTree(t, filepath.Join(root, ".tiller"))
	data = designCLIData(t, "unlink", "design", "as-is/one.md", "--describes", "app/one,two.go", "--from", "docs/one,two.md")
	require.Equal(t, false, data["changed"])
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
}

func TestDesignJSONReferencesRecoverSourcesAfterRetirement(t *testing.T) {
	root := layDesignCLI(t)
	data := designCLIData(t, "link", "design", "as-is/one.md", "--describes", "app/one,two.go", "--from", "docs/one,two.md")
	source := data["derived_from"].([]any)[0].(map[string]any)
	value, ok := source["value"].(string)
	require.True(t, ok, "the JSON source reference must expose its recoverable value")
	gitRun(t, root, "rm", "--", "docs/one,two.md")
	gitRun(t, root, "commit", "-m", "Retire translated source")
	recovered, err := exec.Command("git", "-C", root, "show", strings.TrimPrefix(value, "git:")).Output()
	require.NoError(t, err)
	require.Equal(t, "# Original\nRecover this source.\n", string(recovered))

	approved := designCLIData(t, "approve", "design", "as-is/one.md")
	doc := approved["docs"].([]any)[0].(map[string]any)
	confirmation := doc["confirmed"].(map[string]any)
	commit, ok := confirmation["commit"].(map[string]any)
	require.True(t, ok, "the JSON confirmation must expose its commit")
	head, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	require.NoError(t, err)
	require.Equal(t, "git:"+strings.TrimSpace(string(head)), commit["value"])
	surface := confirmation["surfaces"].([]any)[0].(map[string]any)
	require.Equal(t, []any{"app/one,two.go"}, surface["inventory"])
}

func TestDesignCommandsRejectMixedAndExplicitEmptyFlagsWithoutWrites(t *testing.T) {
	root := layDesignCLI(t)
	designCLIData(t, "link", "design", "as-is/one.md", "--describes", "app")
	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	for _, args := range [][]string{
		{"link", "design", "as-is/one.md", "--describes", "app", "--depends-on="},
		{"link", "design", "as-is/one.md", "--describes", "app", "--surface="},
		{"link", "design", "as-is/one.md", "--describes", "app", "--attach="},
		{"unlink", "design", "as-is/one.md", "--describes", "app", "--depends-on="},
		{"unlink", "design", "as-is/one.md", "--describes", "app", "--detach=false"},
		{"approve", "design", "as-is/one.md", "--refresh-basis=false"},
		{"approve", "design", "as-is/one.md", "--feature="},
		{"approve", "design", "as-is/one.md", "--reviewed-assumption="},
		{"approve", "design", "as-is/one.md", "--review-grain="},
		{"approve", "design"},
		{"link", "design", "as-is/one.md"},
		{"unlink", "design", "as-is/one.md"},
		{"link", "design", "as-is/one.md", "--describes="},
		{"link", "design", "as-is/one.md", "--from="},
		{"unlink", "design", "as-is/one.md", "--describes="},
		{"unlink", "design", "as-is/one.md", "--from="},
		{"link", "designs", "as-is/one.md", "--describes", "app"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, stderr, code := execCLI(t, append(args, "--contributor", "jc", "--json")...)
			require.Equal(t, 2, code, out+stderr)
			require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
			var envelope map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &envelope))
			require.Equal(t, false, envelope["ok"])
			require.NotContains(t, out, `"readout"`)
		})
	}
	// Unknown flags are rejected by Cobra before the command envelope exists.
	out, stderr, code := execCLI(t, "approve", "design", "as-is/one.md", "--all=false", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out+stderr)
	require.Contains(t, stderr, "--all")
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
}

func TestDesignFlagsCannotBeSilentlyDroppedByWorkNouns(t *testing.T) {
	root := layDesignCLI(t)
	mustRun(t, "add", "epic", "one", "--title", "One", "--contributor", "jc")
	mustRun(t, "add", "epic", "two", "--title", "Two", "--contributor", "jc")
	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	for _, verb := range []string{"link", "unlink"} {
		for _, flag := range []string{"--describes=", "--from="} {
			out, stderr, code := execCLI(t, verb, "epic", "one", "--depends-on", "two", flag, "--contributor", "jc", "--json")
			require.Equal(t, 2, code, out+stderr)
			require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
		}
	}
	data := designCLIData(t, "link", "epic", "one", "--depends-on", "two")
	require.Equal(t, true, data["changed"])
	data = designCLIData(t, "link", "epic", "one", "--depends-on", "two")
	require.Equal(t, false, data["changed"])
	data = designCLIData(t, "unlink", "epic", "one", "--depends-on", "two")
	require.Equal(t, true, data["changed"])
}

func TestApproveDesignBatchIsAtomicAndUsesPaths(t *testing.T) {
	root := layDesignCLI(t)
	for _, doc := range []string{"as-is/one.md", "as-is/two.md"} {
		designCLIData(t, "link", "design", doc, "--describes", "app")
	}
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller/design/as-is/two.md")))
	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	out, stderr, code := execCLI(t, "approve", "design", "as-is/one.md", "as-is/two.md", "--contributor", "jc", "--json")
	require.NotEqual(t, 0, code, out+stderr)
	require.NotContains(t, out, `"readout"`)
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")), "a later invalid doc must not confirm the first")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/design/as-is/two.md"), []byte("# Two\nOther behavior.\n"), 0o644))
	prose := snapshotTree(t, filepath.Join(root, ".tiller/design/as-is"))
	data := designCLIData(t, "approve", "design", "as-is/one.md", "as-is/two.md")
	require.Equal(t, true, data["changed"])
	require.Equal(t, map[string]any{"id": "jc", "source": "--contributor"}, data["actor"])
	docs := data["docs"].([]any)
	require.Len(t, docs, 2)
	for i, path := range []string{"as-is/one.md", "as-is/two.md"} {
		doc := docs[i].(map[string]any)
		require.Equal(t, path, doc["path"])
		require.Equal(t, true, doc["changed"])
		require.NotContains(t, doc, "id")
		require.NotContains(t, doc, "status")
	}
	readout := data["readout"].(string)
	require.Contains(t, readout, "as-is/one.md")
	require.Contains(t, readout, "as-is/two.md")
	require.NotContains(t, readout, "moved to ready")
	require.Equal(t, prose, snapshotTree(t, filepath.Join(root, ".tiller/design/as-is")))
	ledger, err := store.LoadBaseline(root)
	require.NoError(t, err)
	for _, doc := range ledger.Docs {
		require.NotNil(t, doc.Confirmed)
		require.Equal(t, "jc", doc.Confirmed.By)
	}
	require.Equal(t, ledger.Docs[0].Confirmed.Commit, ledger.Docs[1].Confirmed.Commit)
	evs, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	var targets []string
	for _, ev := range evs {
		if ev.Type == "approve.design" {
			targets = append(targets, ev.Targets...)
		}
	}
	require.ElementsMatch(t, []string{".tiller/design/as-is/one.md", ".tiller/design/as-is/two.md"}, targets)

	before = snapshotTree(t, filepath.Join(root, ".tiller"))
	out, stderr, code = execCLI(t, "approve", "design", "as-is/one.md", "as-is/two.md", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, ".tiller/design/as-is/one.md")
	require.Contains(t, out, ".tiller/design/as-is/two.md")
	require.NotContains(t, out, "is now ready")
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
	data = designCLIData(t, "approve", "design", "as-is/one.md", "as-is/two.md")
	require.Equal(t, false, data["changed"])
	require.Equal(t, 1, strings.Count(out, data["readout"].(string)), "one shared readout follows all path confirmations")
	for _, raw := range data["docs"].([]any) {
		require.Equal(t, false, raw.(map[string]any)["changed"])
	}
	// An idempotent coverage retry must not silently revoke acceptance.
	data = designCLIData(t, "link", "design", "as-is/one.md", "--describes", "app")
	require.Equal(t, false, data["changed"])
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
}

func TestDesignApprovalIgnoresRetiredBoardObstruction(t *testing.T) {
	root := layDesignCLI(t)
	designCLIData(t, "link", "design", "as-is/one.md", "--describes", "app")
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller/board.md"), 0o755))
	data := designCLIData(t, "approve", "design", "as-is/one.md")
	require.NotContains(t, data, "warning")
	require.DirExists(t, filepath.Join(root, ".tiller/board.md"))
	ledger, err := store.LoadBaseline(root)
	require.NoError(t, err)
	require.NotNil(t, ledger.Docs[0].Confirmed)
}

func TestDesignLinkAndUnlinkDoNotRequireGit(t *testing.T) {
	root := layDesignCLI(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	designCLIData(t, "link", "design", "as-is/one.md", "--describes", "app")
	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	out, stderr, code := execCLI(t, "approve", "design", "as-is/one.md", "--contributor", "jc", "--json")
	require.NotEqual(t, 0, code, out+stderr)
	require.NotContains(t, out, `"readout"`)
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "app")))
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller/design/as-is/one.md")))
	data := designCLIData(t, "unlink", "design", "as-is/one.md", "--describes", "app")
	require.Equal(t, true, data["changed"])
	ledger, err := store.LoadBaseline(root)
	require.NoError(t, err)
	require.Empty(t, ledger.Docs)
}

func TestApproveDesignDistinguishesUnchangedAndNewConfirmations(t *testing.T) {
	root := layDesignCLI(t)
	for _, doc := range []string{"as-is/one.md", "as-is/two.md"} {
		designCLIData(t, "link", "design", doc, "--describes", "app")
	}
	designCLIData(t, "approve", "design", "as-is/one.md")
	before, err := store.LoadBaseline(root)
	require.NoError(t, err)
	data := designCLIData(t, "approve", "design", "as-is/one.md", "as-is/two.md")
	require.Equal(t, true, data["changed"])
	docs := data["docs"].([]any)
	require.Len(t, docs, 2)
	require.Equal(t, "as-is/one.md", docs[0].(map[string]any)["path"])
	require.Equal(t, false, docs[0].(map[string]any)["changed"])
	require.Equal(t, "as-is/two.md", docs[1].(map[string]any)["path"])
	require.Equal(t, true, docs[1].(map[string]any)["changed"])
	after, err := store.LoadBaseline(root)
	require.NoError(t, err)
	require.Equal(t, before.Docs[0].Confirmed, after.Docs[0].Confirmed, "unchanged approval keeps the original observation")
	require.NotNil(t, after.Docs[1].Confirmed)
}

func TestDesignFlagsPreserveDashLedDocumentIdentity(t *testing.T) {
	root := layDesignCLI(t)
	doc := "-owner's, notes.md"
	require.NoError(t, os.Rename(filepath.Join(root, ".tiller/design/as-is/one.md"), filepath.Join(root, ".tiller/design", doc)))
	out, stderr, code := execCLI(t, "link", "design", "--describes", "app", "--contributor", "jc", "--json", "--", doc)
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, doc, jsonData(t, out)["path"])
	out, stderr, code = execCLI(t, "approve", "design", "--contributor", "jc", "--json", "--", doc)
	require.Equal(t, 0, code, out+stderr)
	data := jsonData(t, out)
	require.Equal(t, doc, data["docs"].([]any)[0].(map[string]any)["path"])
	ledger, err := store.LoadBaseline(root)
	require.NoError(t, err)
	require.Equal(t, doc, ledger.Docs[0].Path)
	require.NotNil(t, ledger.Docs[0].Confirmed)
}
