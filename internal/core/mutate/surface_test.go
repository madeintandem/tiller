package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the surface write path (cli-surface.md, the link row):
// surface.paths is the existing code an epic or a feature touches, and until
// SetSurface existed nothing could write it. Unioning mirrors --depends-on
// exactly (docs/plans/2026-07-28-drift-design-decisions.md, decision 5),
// because surface is machine-owned state on the same .yml side of the
// prose/state split depends.go's own header argues from.

// TestSetSurfaceUnions pins the union write: a first call records the given
// paths, and a second call with one repeat and one new path keeps the old set
// and adds only what is new.
func TestSetSurfaceUnions(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	for _, path := range []string{"app/models/calc", "app/services", "lib/calc"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, path), 0o755))
	}
	tree := load(t, root)

	res, err := mutate.SetSurface(tree, testDeps(), tree.Features["F-aaaaaa"],
		[]string{"app/models/calc", "app/services"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"app/models/calc", "app/services"},
		load(t, root).Features["F-aaaaaa"].Surface.Paths)

	// Union, not overwrite: a repeat with one new path keeps the old ones and
	// dedupes the repeat.
	res, err = mutate.SetSurface(load(t, root), testDeps(), tree.Features["F-aaaaaa"],
		[]string{"app/services", "lib/calc"})
	require.NoError(t, err)
	require.Equal(t, []string{"app/models/calc", "app/services", "lib/calc"},
		load(t, root).Features["F-aaaaaa"].Surface.Paths)
}

// TestSetSurfaceRecordsItsOwnEventType pins the event type SetSurface writes:
// link.surface.epic / link.surface.feature, distinct from depends_on's
// link.epic / link.feature (fix round 2, drift design). A consumer that
// groups or phrases a row off Type alone, without parsing the payload, needs
// the two facts to carry different types: the recap's burst collapsing
// (ui/src/lib/bursts.ts) and a node's own timeline
// (internal/views/nodepage.go's historyOf) both do exactly that, and sharing
// link.feature made a surface-only recording read as "features linked" and
// made a dependency edge indistinguishable from a surface recording on a
// node's timeline.
func TestSetSurfaceRecordsItsOwnEventType(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app/models"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs/design"), 0o755))
	tree := load(t, root)

	_, err := mutate.SetSurface(tree, testDeps(), tree.Features["F-aaaaaa"], []string{"app/models"})
	require.NoError(t, err)

	tree = load(t, root)
	featEv := findEvent(t, tree, "link.surface.feature")
	require.False(t, hasEvent(t, tree, "link.feature"),
		"a surface-only call must not also record depends_on's own type")
	require.Equal(t, []any{"app/models"}, featEv.After["surface"], "the delta still rides in After")

	// Pin a distinct deterministic id for the epic event so the two grain
	// assertions remain easy to inspect on disk.
	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-epicev" }
	_, err = mutate.SetSurface(load(t, root), d, load(t, root).Epics["E-aaaaaa"], []string{"docs/design"})
	require.NoError(t, err)

	tree = load(t, root)
	epicEv := findEvent(t, tree, "link.surface.epic")
	require.False(t, hasEvent(t, tree, "link.epic"),
		"a surface-only call must not also record depends_on's own type")
	require.Equal(t, []any{"docs/design"}, epicEv.After["surface"])
}

// TestSetSurfaceIdempotentNoChange pins the no-op arm against SetDependsOn's
// own contract (Link, "a repeat with nothing new is a no-change success"): a
// second call recording exactly what is already on disk changes nothing
// there and appends no event to the stream.
func TestSetSurfaceIdempotentNoChange(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app/models/calc"), 0o755))

	_, err := mutate.SetSurface(load(t, root), testDeps(), load(t, root).Features["F-aaaaaa"],
		[]string{"app/models/calc"})
	require.NoError(t, err)

	before, err := events.List(load(t, root))
	require.NoError(t, err)

	// Pin a distinct event id for the repeat. If the no-op guard regresses,
	// the unexpected event must target a new immutable path and make the count
	// assertion below fail.
	repeatDeps := testDeps()
	repeatDeps.Mint = func(prefix string) string { return prefix + "-repeat" }

	res, err := mutate.SetSurface(load(t, root), repeatDeps, load(t, root).Features["F-aaaaaa"],
		[]string{"app/models/calc"})
	require.NoError(t, err)
	require.False(t, res.Changed, "repeating a recorded path is a no-op success")
	require.Equal(t, []string{"app/models/calc"}, res.Surface, "the no-op still reports what is on disk")

	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, after, len(before), "a no-op union records no event")

	require.Equal(t, []string{"app/models/calc"}, load(t, root).Features["F-aaaaaa"].Surface.Paths,
		"nothing changed on disk either")
}

// TestSetSurfaceRefusesWrongGrainAndBadPaths pins the two refusal axes named
// in the brief. A task or question target is a usageError (reason
// "surface_target"): the schema puts surface on epics and features only. An
// absolute path or one escaping the repo ("../x") is a usageError (reason
// "surface_path"): the field holds repo-relative paths the drift sweep can
// stat under root. Both axes refuse all-or-nothing, before anything is
// written, the same posture resolveDependsOn takes on its own targets.
func TestSetSurfaceRefusesWrongGrainAndBadPaths(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lib/good"), 0o755))
	tree := load(t, root)
	_, err := mutate.AddTask(tree, testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/probe", Title: "Probe",
	})
	require.NoError(t, err)
	_, err = mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{Title: "Canonical schema version", Statement: "Which schema?"})
	require.NoError(t, err)

	tree = load(t, root)
	for _, node := range []store.Node{tree.Tasks["T-aaaaaa"], tree.Questions["Q-aaaaaa"]} {
		_, err := mutate.SetSurface(tree, testDeps(), node, []string{"app/models"})
		require.Error(t, err, "target noun %s", node.Noun())
		var ue interface{ Reason() string }
		require.ErrorAs(t, err, &ue)
		require.Equal(t, "surface_target", ue.Reason())
	}

	for _, bad := range []string{"", "/etc/passwd", "../escape"} {
		_, err := mutate.SetSurface(load(t, root), testDeps(), load(t, root).Features["F-aaaaaa"],
			[]string{bad})
		require.Error(t, err, "path %q", bad)
		var ue interface{ Reason() string }
		require.ErrorAs(t, err, &ue)
		require.Equal(t, "surface_path", ue.Reason())
	}

	// All-or-nothing: a good path ahead of a bad one buys nothing.
	_, err = mutate.SetSurface(load(t, root), testDeps(), load(t, root).Features["F-aaaaaa"],
		[]string{"lib/good", "../escape"})
	require.Error(t, err)

	require.Empty(t, load(t, root).Features["F-aaaaaa"].Surface.Paths, "nothing written by any refusal")
}

func TestSetSurfacePathBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		paths       []string
		root        string
		mode        string
		reason      string
		environment bool
	}{
		{name: "missing leaf", paths: []string{"app/missing.go"}, reason: "surface_missing"},
		{name: "missing intermediate", paths: []string{"absent/nested/code"}, reason: "surface_missing"},
		{name: "valid then missing", paths: []string{"app/current", "absent"}, reason: "surface_missing"},
		{name: "empty mode", paths: []string{"app/current"}, root: ".", reason: "invalid_code_root_mode", environment: true},
		{name: "misspelled mode", paths: []string{"app/current"}, root: ".", mode: "refrence", reason: "invalid_code_root_mode", environment: true},
		{name: "reference itself", paths: []string{"app/legacy"}, root: "app/legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "reference descendant", paths: []string{"app/legacy/code"}, root: "app/legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "reference parent", paths: []string{"app"}, root: "app/legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "repository parent surface", paths: []string{"."}, root: "app/legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "normalized reference", paths: []string{"app/legacy/code"}, root: "app/current/../legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "absolute reference", paths: []string{"app/legacy/code"}, root: "$repo/app/legacy", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "ancestor reference", paths: []string{"app/current"}, root: "..", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "final symlink", paths: []string{"alias"}, reason: "surface_path"},
		{name: "intermediate symlink", paths: []string{"alias/code"}, reason: "surface_path"},
		{name: "reference alias", paths: []string{"app/legacy/code"}, root: "alias", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "alias parent surface", paths: []string{"app"}, root: "alias", mode: store.CodeModeReference, reason: "surface_reference"},
		{name: "dangling reference", paths: []string{"app/current"}, root: "dangling", mode: store.CodeModeReference, reason: "surface_unavailable", environment: true},
		{name: "missing reference", paths: []string{"app/current"}, root: "absent", mode: store.CodeModeReference, reason: "surface_unavailable", environment: true},
		{name: "non directory component", paths: []string{"plain/child"}, reason: "surface_unavailable", environment: true},
		{name: "sibling prefix", paths: []string{"app/legacy-old"}, root: "app/legacy", mode: store.CodeModeReference},
		{name: "outside declared surface", paths: []string{"plain", "app/current"}, root: "app/legacy", mode: store.CodeModeSurface},
		{name: "external disjoint reference", paths: []string{"app/current"}, root: "$external", mode: store.CodeModeReference},
		{name: "cleaned untracked paths", paths: []string{"app/./current", "plain"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := layEpicFeature(t)
			for _, path := range []string{"app/current", "app/legacy", "app/legacy-old"} {
				require.NoError(t, os.MkdirAll(filepath.Join(root, path), 0o755))
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "plain"), []byte("code\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "app/legacy/code"), []byte("legacy\n"), 0o644))
			require.NoError(t, os.Symlink("app/legacy", filepath.Join(root, "alias")))
			require.NoError(t, os.Symlink("absent", filepath.Join(root, "dangling")))
			tree := load(t, root)
			if tt.root != "" {
				ref := strings.ReplaceAll(tt.root, "$repo", root)
				ref = strings.ReplaceAll(ref, "$external", t.TempDir())
				tree.Manifest.Inputs.Code = []store.CodeRoot{
					{Root: ".", Mode: store.CodeModeSurface},
					{Root: ref, Mode: tt.mode},
				}
			}
			nodePath := filepath.Join(root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml")
			before, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			beforeEvents, err := events.List(tree)
			require.NoError(t, err)
			res, err := mutate.SetSurface(tree, seqDeps(), tree.Features["F-aaaaaa"], tt.paths)
			if tt.reason == "" {
				require.NoError(t, err)
				want := make([]string, len(tt.paths))
				for i, path := range tt.paths {
					want[i] = filepath.Clean(path)
				}
				require.Equal(t, want, res.Surface)
				require.Equal(t, want, load(t, root).Features["F-aaaaaa"].Surface.Paths)
				return
			}
			if tt.environment {
				var env *store.EnvError
				require.ErrorAs(t, err, &env)
				require.Equal(t, tt.reason, env.Reason)
				if tt.reason == "surface_unavailable" {
					require.Contains(t, env.Msg, tt.paths[0])
					if tt.root != "" {
						require.Contains(t, env.Msg, tt.root)
					}
				}
				require.Equal(t, 3, env.ExitCode())
			} else {
				var reason interface{ Reason() string }
				require.ErrorAs(t, err, &reason)
				require.Equal(t, tt.reason, reason.Reason())
			}
			after, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterEvents, err := events.List(load(t, root))
			require.NoError(t, err)
			require.Equal(t, beforeEvents, afterEvents)
		})
	}
}

func TestSetSurfaceRevalidatesOnlyRequestedPaths(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	for _, path := range []string{"old", "new"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte("code\n"), 0o644))
	}
	d := seqDeps()
	tree := load(t, root)
	_, err := mutate.SetSurface(tree, d, tree.Features["F-aaaaaa"], []string{"old"})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "old")))
	tree = load(t, root)
	before, err := events.List(tree)
	require.NoError(t, err)
	_, err = mutate.SetSurface(tree, d, tree.Features["F-aaaaaa"], []string{"old", "old", "new"})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "surface_missing", reason.Reason())
	require.Equal(t, []string{"old"}, load(t, root).Features["F-aaaaaa"].Surface.Paths)
	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Equal(t, before, after)
	tree = load(t, root)
	res, err := mutate.SetSurface(tree, d, tree.Features["F-aaaaaa"], []string{"new"})
	require.NoError(t, err)
	require.Equal(t, []string{"old", "new"}, res.Surface)
}

func TestSetSurfaceCanonicalizesRepositoryAlias(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "legacy"), 0o755))
	alias := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, os.Symlink(root, alias))
	tree := load(t, alias)
	canonicalRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	tree.Manifest.Inputs.Code = []store.CodeRoot{{Root: filepath.Join(canonicalRoot, "legacy"), Mode: store.CodeModeReference}}
	_, err = mutate.SetSurface(tree, seqDeps(), tree.Features["F-aaaaaa"], []string{"legacy"})
	var reason interface{ Reason() string }
	require.ErrorAs(t, err, &reason)
	require.Equal(t, "surface_reference", reason.Reason())
	require.NoError(t, os.MkdirAll(filepath.Join(root, "current"), 0o755))
	res, err := mutate.SetSurface(tree, seqDeps(), tree.Features["F-aaaaaa"], []string{"current"})
	require.NoError(t, err)
	require.Equal(t, []string{"current"}, res.Surface)
}

func TestSetSurfaceRefusesReferenceIdentityAliases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		path      string
		reference string
		caseAlias bool
	}{
		{"hard link", "linked-code", "app/legacy/code", false},
		{"case descendant", "app/LEGACY/code", "app/legacy", true},
		{"case parent", "APP", "app/legacy", true},
		{"case reference", "app/legacy/code", "app/LEGACY", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := layEpicFeature(t)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "app/legacy"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "app/legacy/code"), []byte("legacy\n"), 0o644))
			require.NoError(t, os.Link(filepath.Join(root, "app/legacy/code"), filepath.Join(root, "linked-code")))
			if tc.caseAlias {
				_, err := os.Stat(filepath.Join(root, "APP/LEGACY/code"))
				if os.IsNotExist(err) {
					t.Skip("filesystem is case-sensitive")
				}
				require.NoError(t, err)
			}
			tree := load(t, root)
			tree.Manifest.Inputs.Code = []store.CodeRoot{{Root: tc.reference, Mode: store.CodeModeReference}}
			nodePath := filepath.Join(root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml")
			before, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			beforeEvents, err := events.List(tree)
			require.NoError(t, err)
			_, err = mutate.SetSurface(tree, seqDeps(), tree.Features["F-aaaaaa"], []string{tc.path})
			var reason interface{ Reason() string }
			require.ErrorAs(t, err, &reason)
			require.Equal(t, "surface_reference", reason.Reason())
			after, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterEvents, err := events.List(load(t, root))
			require.NoError(t, err)
			require.Equal(t, beforeEvents, afterEvents)
		})
	}
}

func TestCombinedLinkIsAtomic(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing surface", "reference surface", "invalid mode", "invalid dependency", "locked dependency", "event write rollback", "both valid"} {
		t.Run(scenario, func(t *testing.T) {
			root := dependsRepo(t)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "current"), 0o755))
			if scenario == "locked dependency" {
				setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "active")
			}
			tree := load(t, root)
			in := mutate.LinkInput{Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"}, Surface: []string{"current"}}
			d := seqDeps()
			switch scenario {
			case "missing surface":
				in.Surface = []string{"missing"}
			case "reference surface":
				tree.Manifest.Inputs.Code = []store.CodeRoot{{Root: "current", Mode: store.CodeModeReference}}
			case "invalid mode":
				tree.Manifest.Inputs.Code = []store.CodeRoot{{Root: ".", Mode: "refrence"}}
			case "invalid dependency":
				in.DependsOn = []string{"no-such-node"}
			case "event write rollback":
				// A valid destination leaf fits NAME_MAX, but its atomic temp
				// name does not. The node and first event precede this failed
				// second event write, exercising the real journal rollback.
				// Establish that the event's final leaf itself is writable.
				eventDir := filepath.Join(root, ".tiller/events/2026/06/27")
				require.NoError(t, os.MkdirAll(eventDir, 0o755))
				finalPath := filepath.Join(eventDir, "EV-"+strings.Repeat("x", 240)+".yml")
				require.NoError(t, os.WriteFile(finalPath, []byte("probe\n"), 0o644))
				require.NoError(t, os.Remove(finalPath))
				minted := 0
				d.Mint = func(prefix string) string {
					minted++
					if minted == 2 {
						return prefix + "-" + strings.Repeat("x", 240)
					}
					return fmt.Sprintf("%s-link%02d", prefix, minted)
				}
			}
			nodePath := filepath.Join(root, ".tiller/epics/form-engine/features/certification-form-loading/feature.yml")
			before, err := os.ReadFile(nodePath)
			require.NoError(t, err)
			beforeEvents, err := events.List(tree)
			require.NoError(t, err)
			res, err := mutate.Link(tree, d, in)
			if scenario != "both valid" {
				require.Error(t, err)
				after, readErr := os.ReadFile(nodePath)
				require.NoError(t, readErr)
				require.Equal(t, before, after)
				afterEvents, listErr := events.List(load(t, root))
				require.NoError(t, listErr)
				require.Equal(t, beforeEvents, afterEvents)
				require.NoFileExists(t, filepath.Join(root, ".tiller/.cache/journal.yml"))
				return
			}
			require.NoError(t, err)
			require.True(t, res.DependenciesChanged)
			require.True(t, res.SurfaceChanged)
			fresh := load(t, root)
			require.Equal(t, []string{"F-bbbbbb"}, fresh.Features["F-cccccc"].DependsOn)
			require.Equal(t, []string{"current"}, fresh.Features["F-cccccc"].Surface.Paths)
			evs, err := events.List(fresh)
			require.NoError(t, err)
			require.Len(t, evs, len(beforeEvents)+2)
			require.Equal(t, []any{"F-bbbbbb"}, findEvent(t, fresh, "link.feature").After["added"])
			require.Equal(t, []any{"current"}, findEvent(t, fresh, "link.surface.feature").After["surface"])
			res, err = mutate.Link(fresh, d, in)
			require.NoError(t, err)
			require.False(t, res.Changed)
			afterEvents, err := events.List(load(t, root))
			require.NoError(t, err)
			require.Equal(t, evs, afterEvents)
		})
	}
}

func TestCombinedLinkAllowsNoOpDependencyAfterStart(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	d := seqDeps()
	_, err := mutate.Link(load(t, root), d, mutate.LinkInput{Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"}})
	require.NoError(t, err)
	setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "active")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "current"), 0o755))
	tree := load(t, root)
	before, err := events.List(tree)
	require.NoError(t, err)
	res, err := mutate.Link(tree, d, mutate.LinkInput{Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"}, Surface: []string{"current"}})
	require.NoError(t, err)
	require.False(t, res.DependenciesChanged)
	require.True(t, res.SurfaceChanged)
	fresh := load(t, root)
	require.Equal(t, []string{"current"}, fresh.Features["F-cccccc"].Surface.Paths)
	after, err := events.List(fresh)
	require.NoError(t, err)
	require.Len(t, after, len(before)+1)
	require.Equal(t, []any{"current"}, findEvent(t, fresh, "link.surface.feature").After["surface"])
}
