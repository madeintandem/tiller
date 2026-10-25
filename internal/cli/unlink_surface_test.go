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
	"gopkg.in/yaml.v3"
)

// These tests pin `tiller unlink --surface`, the one route that removes a
// recorded surface path. A replace or migrate feature that moves or deletes
// its own surface otherwise can never record done: sync fingerprints every
// recorded path at the integration commit, and a moved path resolves to
// nothing there.

func layUnlinkSurfaceCLI(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	require.NoError(t, os.MkdirAll("app/models", 0o755))
	require.NoError(t, os.MkdirAll("app/views", 0o755))
	mustRun(t, "link", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--surface", "app/views", "--contributor", "jc")
}

func TestUnlinkSurfaceRemovesARecordedPathEvenAfterItIsGone(t *testing.T) {
	layUnlinkSurfaceCLI(t)
	require.NoError(t, os.RemoveAll("app/models"))
	before, err := events.List(loadTestTree(t))
	require.NoError(t, err)

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "./app/models/", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, true, data["changed"])
	require.Equal(t, []any{"app/views"}, data["surface"])
	require.Equal(t, []any{"app/models"}, data["surface_removed"])
	require.Equal(t, []string{"app/views"}, loadTestTree(t).Features[featureIDByKey(t, "form-engine/schema-engine")].Surface.Paths)

	after, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	require.Len(t, after, len(before)+1, "one event for the one changed axis")
	ev := after[len(after)-1]
	require.Equal(t, "unlink.surface.feature", ev.Type)
	require.Equal(t, []any{"app/models", "app/views"}, ev.Before["surface"])
	require.Equal(t, []any{"app/views"}, ev.After["surface"])
	require.Equal(t, []any{"app/models"}, ev.After["removed"])

	out, stderr, code = execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "No requested surface paths were recorded on feature form-engine/schema-engine (F-")
	require.Contains(t, out, "surface is now app/views (no change).")
	again, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	require.Len(t, again, len(after), "a repeat records nothing")
}

func TestUnlinkSurfaceHumanOutputNamesWhatRemains(t *testing.T) {
	layUnlinkSurfaceCLI(t)

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--surface", "app/views", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Removed app/models, app/views from feature form-engine/schema-engine (F-")
	require.Contains(t, out, "surface; no surface remains.")

	out, stderr, code = execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, []any{}, data["surface"], "an empty surface still rides when --surface was named")
	require.Equal(t, []any{}, data["surface_removed"])

	out, stderr, code = execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--depends-on", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data = jsonData(t, out)
	require.NotContains(t, data, "surface", "a call without --surface carries no surface fields")
	require.NotContains(t, data, "surface_removed")
}

func TestUnlinkSurfaceOnAnEpicAndItsArchivedLock(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.NoError(t, os.MkdirAll("app/models", 0o755))
	require.NoError(t, os.MkdirAll("app/views", 0o755))
	mustRun(t, "link", "epic", "form-engine", "--surface", "app/models", "--surface", "app/views", "--contributor", "jc")
	eid := soleEpicID(t)

	out, stderr, code := execCLI(t, "unlink", "epic", "form-engine", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, []any{"app/views"}, jsonData(t, out)["surface"])
	evs, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	require.Equal(t, "unlink.surface.epic", evs[len(evs)-1].Type)

	for _, status := range []string{"done", "archived"} {
		setEpicStatusCLI(t, "form-engine", status)
		out, _, code = execCLI(t, "unlink", "epic", "form-engine", "--surface", "app/views", "--contributor", "jc", "--json")
		require.Equalf(t, 2, code, "%s: %s", status, out)
		require.Contains(t, out, `"reason": "surface_locked"`)
		require.Equal(t, []string{"app/views"}, loadTestTree(t).Epics[eid].Surface.Paths)
	}
}

func setEpicStatusCLI(t *testing.T, epicKey, status string) {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Epic
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestUnlinkSurfaceRefusesMisuseAndWritesNothing(t *testing.T) {
	layUnlinkSurfaceCLI(t)
	mustRun(t, "add", "task", "form-engine/schema-engine/first", "--title", "First", "--contributor", "jc")
	mustRun(t, "add", "bug", "crash", "--title", "Crash", "--contributor", "jc")
	before, err := events.List(loadTestTree(t))
	require.NoError(t, err)

	for _, tc := range []struct {
		args   []string
		reason string
	}{
		{[]string{"unlink", "task", "form-engine/schema-engine/first", "--surface", "app/models"}, "surface_target"},
		{[]string{"unlink", "bug", "crash", "--surface", "app/models", "--detach"}, "surface_target"},
		{[]string{"unlink", "feature", "form-engine/schema-engine", "--surface", "/abs/app"}, "surface_path"},
		{[]string{"unlink", "feature", "form-engine/schema-engine", "--surface", "../outside"}, "surface_path"},
		{[]string{"unlink", "design", "as-is/editor.md", "--surface", "app/models"}, "bad_args"},
	} {
		out, _, code := execCLI(t, append(tc.args, "--contributor", "jc", "--json")...)
		require.Equalf(t, 2, code, "%v: %s", tc.args, out)
		var envelope Envelope
		require.NoError(t, json.Unmarshal([]byte(out), &envelope))
		require.NotNil(t, envelope.Error, tc.args)
		require.Equalf(t, tc.reason, envelope.Error.Reason, "%v", tc.args)
	}
	require.Equal(t, []string{"app/models", "app/views"}, loadTestTree(t).Features[featureIDByKey(t, "form-engine/schema-engine")].Surface.Paths)
	after, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestUnlinkSurfaceRefusesADoneNodesSurface(t *testing.T) {
	layUnlinkSurfaceCLI(t)
	setFeatureStatusCLI(t, "form-engine", "schema-engine", "done")

	out, _, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "surface_locked"`)
	require.Contains(t, out, "its completion record pins its surface")
	require.Equal(t, []string{"app/models", "app/views"}, loadTestTree(t).Features[featureIDByKey(t, "form-engine/schema-engine")].Surface.Paths)

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/elsewhere", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, false, jsonData(t, out)["changed"], "naming an unrecorded path stays a no-op")
}

func TestUnlinkSurfaceKeepsASpecifiedMigrateFeaturesLastPath(t *testing.T) {
	layUnlinkSurfaceCLI(t)
	setFeatureFieldsCLI(t, "form-engine", "schema-engine", "active", "migrate")
	mustRun(t, "unlink", "feature", "form-engine/schema-engine", "--surface", "app/views", "--contributor", "jc")

	out, _, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	var envelope Envelope
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Equal(t, "surface_required", envelope.Error.Reason)
	require.Contains(t, envelope.Error.Message, "tiller link feature form-engine/schema-engine --surface <path>")
	require.Equal(t, []string{"app/models"}, loadTestTree(t).Features[featureIDByKey(t, "form-engine/schema-engine")].Surface.Paths)

	require.NoError(t, os.MkdirAll("app/core", 0o755))
	mustRun(t, "link", "feature", "form-engine/schema-engine", "--surface", "app/core", "--contributor", "jc")
	mustRun(t, "unlink", "feature", "form-engine/schema-engine", "--surface", "app/models", "--contributor", "jc")
	require.Equal(t, []string{"app/core"}, loadTestTree(t).Features[featureIDByKey(t, "form-engine/schema-engine")].Surface.Paths)
}

func TestUnlinkSurfaceAndDependencyCommitTogetherOrNotAtAll(t *testing.T) {
	layUnlinkSurfaceCLI(t)
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")
	mustRun(t, "link", "feature", "form-engine/schema-engine", "--depends-on", "form-engine/rendering-modes", "--contributor", "jc")
	fid := featureIDByKey(t, "form-engine/schema-engine")

	// Once work starts, the dependency arm locks; the surface arm must not
	// land on its own.
	setFeatureFieldsCLI(t, "form-engine", "schema-engine", "active", "")
	out, _, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--depends-on", "form-engine/rendering-modes", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "dependency_change_locked")
	require.Equal(t, []string{"app/models", "app/views"}, loadTestTree(t).Features[fid].Surface.Paths)

	setFeatureFieldsCLI(t, "form-engine", "schema-engine", "ready", "")
	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/schema-engine",
		"--depends-on", "form-engine/rendering-modes", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, []any{}, data["depends_on"])
	require.Equal(t, []any{"app/views"}, data["surface"])
	fresh := loadTestTree(t).Features[fid]
	require.Empty(t, fresh.DependsOn)
	require.Equal(t, []string{"app/views"}, fresh.Surface.Paths)
}

// TestMigrateFeatureThatMovesItsSurfaceCrossesAfterUnlink is the reported
// strand end to end: a migrate feature whose task `git mv`s its recorded
// surface is skipped by sync with the unlink remedy named, and crosses to done
// once the new location is linked and the old path unlinked.
func TestMigrateFeatureThatMovesItsSurfaceCrossesAfterUnlink(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app", "legacy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "legacy", "parse.go"), []byte("package legacy\n"), 0o644))
	gitRun(t, dir, "add", "app")
	gitRun(t, dir, "commit", "-m", "legacy code")

	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "NRCA form loading", "--change-type", "migrate", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, stderr, code := execCLI(t, "link", "feature", cliFeatureKey, "--surface", "app/legacy", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	for _, argv := range [][]string{
		{"approve", "epic", "form-engine", "--contributor", "jc"},
		{"spec", "feature", cliFeatureKey, "--contributor", "jc"},
		{"approve", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}
	tid := taskIDByKey(t, cliTaskKey)
	_, stderr, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	branch := checkoutTaskBranchCLI(t, tid)
	gitRun(t, dir, "mv", "app/legacy", "app/core")
	gitRun(t, dir, "commit", "-m", "move the legacy code")
	for _, argv := range [][]string{
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v: %s", argv, stderr)
	}
	mergeTaskBranchCLI(t, dir, branch)
	fid := featureIDByKey(t, cliFeatureKey)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, fmt.Sprintf("  - feature %s (%s): git could not resolve app/legacy", cliFeatureKey, fid))
	require.Contains(t, out, "the work itself moved or removed will never resolve again")
	require.Contains(t, out, "    tiller unlink feature "+cliFeatureKey+" --surface app/legacy")
	require.Equal(t, "active", loadTestTree(t).Features[fid].Status)

	quiet, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var env struct {
		Data struct {
			Skipped []struct {
				Path    string `json:"path"`
				Surface bool   `json:"surface"`
				Remedy  string `json:"remedy"`
			} `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(quiet), &env))
	require.Len(t, env.Data.Skipped, 1)
	require.True(t, env.Data.Skipped[0].Surface)
	require.Equal(t, "tiller unlink feature "+cliFeatureKey+" --surface app/legacy", env.Data.Skipped[0].Remedy)

	_, stderr, code = execCLI(t, "link", "feature", cliFeatureKey, "--surface", "app/core", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "unlink", "feature", cliFeatureKey, "--surface", "app/legacy", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "could not be recorded")
	feat := loadTestTree(t).Features[fid]
	require.Equal(t, "done", feat.Status)
	require.Len(t, feat.VerifiedAgainst.Surfaces, 1)
	require.Equal(t, "app/core", feat.VerifiedAgainst.Surfaces[0].Path)
}

// setFeatureStatusCLI rewrites one feature's status on disk.
func setFeatureStatusCLI(t *testing.T, epicKey, localKey, status string) {
	t.Helper()
	setFeatureFieldsCLI(t, epicKey, localKey, status, "")
}

// setFeatureFieldsCLI rewrites one feature's status and, when changeType is
// non-empty, its change_type on disk.
func setFeatureFieldsCLI(t *testing.T, epicKey, localKey, status, changeType string) {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", localKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	if changeType != "" {
		doc.ChangeType = changeType
	}
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
