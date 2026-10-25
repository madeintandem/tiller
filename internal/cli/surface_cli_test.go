package cli

import (
	"encoding/json"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
	"os"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/stretchr/testify/require"
)

// These tests pin the surface write path at the CLI surface (cli-surface.md,
// the link row): `tiller link --surface`, the only writer surface.paths has.
// They mirror depends_cli_test.go's structure for the sibling flag.

// TestLinkSurfaceUnionsAndDedupes pins the post-hoc writer end to end: a
// repeated path in the same invocation dedupes, the JSON envelope carries the
// resulting list, and a second invocation naming the same path is a no-change
// success that still leaves the path recorded.
func TestLinkSurfaceUnionsAndDedupes(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	require.NoError(t, os.MkdirAll("app/models", 0o755))

	out, _, code := execCLI(t, "link", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"surface"`)
	require.Regexp(t, `"surface": \[\s*"app/models"\s*\]`, out, "the repeat within one invocation dedupes")
	require.Contains(t, out, `"changed": true`)

	out, _, code = execCLI(t, "link", "feature", "form-engine/schema-engine",
		"--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"changed": false`)

	out, _, code = execCLI(t, "show", "feature", "form-engine/schema-engine", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"paths": \[\s*"app/models"\s*\]`, out, "the recorded surface persists")
}

// TestLinkSurfaceRefusesWrongGrain pins the grain rule at the CLI surface: a
// task carries no Surface field (artifact-schema.md, change_type and
// surface), so --surface on one refuses as misuse rather than silently doing
// nothing.
func TestLinkSurfaceRefusesWrongGrain(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/first", "--title", "First", "--contributor", "jc")

	out, _, code := execCLI(t, "link", "task", "form-engine/schema-engine/first",
		"--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "surface_target"`)
}

// TestLinkSurfaceRefusesBadPaths pins the path hygiene refusal at the CLI
// surface: an absolute path is misuse, and nothing is written by the refusal.
func TestLinkSurfaceRefusesBadPaths(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")

	out, _, code := execCLI(t, "link", "feature", "form-engine/schema-engine",
		"--surface", "/etc/passwd", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "surface_path"`)

	out, _, code = execCLI(t, "show", "feature", "form-engine/schema-engine", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, out, `"surface"`, "nothing recorded by a refused link")
}

// TestLinkNeitherFlagIsMisuse pins the replacement for the old single
// required --depends-on flag: link now takes either flag on its own, but
// naming neither is still refused rather than silently doing nothing.
func TestLinkNeitherFlagIsMisuse(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")

	out, _, code := execCLI(t, "link", "epic", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "link_target"`)
}

// Both axes must persist from one source transaction, with distinct events.
func TestLinkBothFlagsInOneInvocation(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")
	require.NoError(t, os.MkdirAll("app/models", 0o755))

	out, _, code := execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var linked struct {
		Data linkView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &linked))
	require.True(t, linked.Data.Changed)
	require.Equal(t, []string{"app/models"}, linked.Data.Surface)
	require.Len(t, linked.Data.DependsOn, 1)

	out, _, code = execCLI(t, "show", "feature", "form-engine/rendering-modes", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"depends_on": \[\s*"F-[a-z0-9]{6}"`, out, "the depends_on leg landed")
	require.Regexp(t, `"paths": \[\s*"app/models"\s*\]`, out, "the surface leg landed alongside it")

	// The two facts record under two DISTINCT event types, not one shared
	// link.feature (fix round 2, drift design): a consumer that groups or
	// phrases a row off Type alone, without parsing the payload, needs to be
	// able to tell a dependency edge from a surface recording apart.
	tree := loadTestTree(t)
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(evs), 2)
	lastTwo := []string{evs[len(evs)-2].Type, evs[len(evs)-1].Type}
	require.ElementsMatch(t, []string{"link.feature", "link.surface.feature"}, lastTwo,
		"one link.feature event for depends_on, one link.surface.feature event for surface")
	out, _, code = execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--surface", "app/models", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var repeated struct {
		Data linkView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &repeated))
	require.False(t, repeated.Data.Changed)
	require.Equal(t, linked.Data.DependsOn, repeated.Data.DependsOn)
	require.Equal(t, linked.Data.Surface, repeated.Data.Surface)
	afterEvents, err := events.List(loadTestTree(t))
	require.NoError(t, err)
	require.Equal(t, evs, afterEvents)
}

func TestCombinedLinkRefusalPreservesSource(t *testing.T) {
	for _, tt := range []struct {
		name   string
		path   string
		mode   string
		root   string
		reason string
		code   int
	}{
		{name: "missing", path: "missing", reason: "surface_missing", code: 2},
		{name: "reference parent", path: "app", root: "app/legacy", mode: store.CodeModeReference, reason: "surface_reference", code: 2},
		{name: "empty mode", path: "app/current", root: ".", reason: "invalid_code_root_mode", code: 3},
		{name: "invalid mode", path: "app/current", root: ".", mode: "refrence", reason: "invalid_code_root_mode", code: 3},
		{name: "symlink", path: "alias/code", reason: "surface_path", code: 2},
		{name: "inspection failure", path: "plain/child", reason: "surface_unavailable", code: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			initOwnedRepo(t)
			mustRun(t, "add", "epic", "work", "--contributor", "jc")
			mustRun(t, "add", "feature", "work/first", "--contributor", "jc")
			mustRun(t, "add", "feature", "work/second", "--contributor", "jc")
			require.NoError(t, os.MkdirAll("app/current", 0o755))
			require.NoError(t, os.MkdirAll("app/legacy", 0o755))
			require.NoError(t, os.WriteFile("app/legacy/code", []byte("code\n"), 0o644))
			require.NoError(t, os.WriteFile("plain", []byte("code\n"), 0o644))
			require.NoError(t, os.Symlink("app/legacy", "alias"))
			tree := loadTestTree(t)
			if tt.root != "" {
				manifestPath := filepath.Join(tree.Root, ".tiller/tiller.yml")
				data, err := os.ReadFile(manifestPath)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				inputs, ok := doc["inputs"].(map[string]any)
				if !ok {
					inputs = map[string]any{}
					doc["inputs"] = inputs
				}
				inputs["code"] = []store.CodeRoot{{Root: tt.root, Mode: tt.mode}}
				data, err = yaml.Marshal(doc)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(manifestPath, data, 0o644))
			}
			nodePath := filepath.Join(tree.Root, ".tiller/epics/work/features/second/feature.yml")
			before, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			beforeEvents, err := events.List(tree)
			require.NoError(t, err)
			out, _, code := execCLI(t, "link", "feature", "work/second", "--depends-on", "work/first", "--surface", tt.path, "--contributor", "jc", "--json")
			require.Equal(t, tt.code, code, out)
			var envelope Envelope
			require.NoError(t, json.Unmarshal([]byte(out), &envelope))
			require.NotNil(t, envelope.Error)
			require.Equal(t, tt.reason, envelope.Error.Reason)
			after, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterEvents, err := events.List(loadTestTree(t))
			require.NoError(t, err)
			require.Equal(t, beforeEvents, afterEvents)
		})
	}
}

func TestLinkSurfaceUnsupportedCombinationsWriteNothing(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "work", "--contributor", "jc")
	mustRun(t, "add", "feature", "work/feature", "--contributor", "jc")
	mustRun(t, "add", "task", "work/feature/first", "--title", "First", "--contributor", "jc")
	mustRun(t, "add", "task", "work/feature/second", "--title", "Second", "--contributor", "jc")
	require.NoError(t, os.MkdirAll("current", 0o755))
	tree := loadTestTree(t)
	before, err := events.List(tree)
	require.NoError(t, err)
	for _, args := range [][]string{
		{"link", "task", "work/feature/second", "--depends-on", "work/feature/first", "--surface", "current"},
		{"link", "feature", "work/feature", "--depends-on", "work", "--surface", "current", "--attach", "work"},
	} {
		out, _, code := execCLI(t, append(args, "--contributor", "jc", "--json")...)
		require.Equal(t, 2, code)
		var envelope Envelope
		require.NoError(t, json.Unmarshal([]byte(out), &envelope))
		require.NotNil(t, envelope.Error)
		require.Equal(t, "surface_target", envelope.Error.Reason)
	}
	fresh := loadTestTree(t)
	for _, task := range fresh.Tasks {
		require.Empty(t, task.DependsOn)
	}
	for _, feature := range fresh.Features {
		require.Empty(t, feature.DependsOn)
		require.Empty(t, feature.Surface.Paths)
	}
	after, err := events.List(fresh)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
