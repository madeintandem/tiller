package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// skeleton is the set of directories tiller init must lay down under .tiller/
// (artifact-schema.md, Directory layout).
var skeleton = []string{
	"design",
	"epics",
	"plans",
	"amendments",
	"decisions/adr",
	"events",
}

// TestInitScaffoldsAndIsIdempotent is the core init acceptance: a first run in a
// fresh directory writes the manifest plus the skeleton, and a second run is a
// no-op success that does not clobber the (possibly human-edited) manifest. init
// is now a pure scaffold: it seeds no owner, so the roster ships empty.
func TestInitScaffoldsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code, "first init succeeds")
	require.Contains(t, out, "\"created\": true")

	require.FileExists(t, filepath.Join(dir, ".tiller", "tiller.yml"))
	for _, d := range skeleton {
		require.DirExists(t, filepath.Join(dir, ".tiller", d))
	}

	// The scaffolded manifest must load and carry the flags it was given, with an
	// empty roster: identity is recorded by `tiller join`, not by init.
	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Equal(t, "Test Project", m.Project.Name)
	require.Equal(t, "test", m.Project.Key)
	require.Equal(t, "greenfield", m.Project.Kind)
	require.Empty(t, m.Contributors, "init seeds no owner; the roster starts empty")
	require.Equal(t, "main", m.Git.IntegrationBranch,
		"init seeds the branch merges land on, so sync never has to fall back to the branch you are standing on")

	before, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)

	out2, _, code2 := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code2, "second init is an idempotent success")
	require.Contains(t, out2, "\"created\": false")

	after, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "re-run must not clobber the manifest")
}

func initCommittedCodeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)
	writeFixture(t, dir, "app/code", "existing code\n")
	gitRun(t, dir, "add", "app/code")
	gitRun(t, dir, "commit", "-m", "existing code")
	return dir
}

func TestInitExistingCodeObservesHead(t *testing.T) {
	for _, kind := range []string{store.KindBrownfield, store.KindLegacyMigration} {
		for _, state := range []string{"committed", "detached", "dirty"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				dir := initCommittedCodeRepo(t)
				if state == "detached" {
					gitRun(t, dir, "checkout", "--detach")
				}
				if state == "dirty" {
					writeFixture(t, dir, "app/code", "uncommitted changes\n")
				}
				head := gitOutput(t, dir, "rev-parse", "--short", "HEAD")
				content, err := os.ReadFile(filepath.Join(dir, "app", "code"))
				require.NoError(t, err)
				t.Chdir(dir)
				_, stderr, code := execCLI(t, "init", "--name", "Existing", "--key", "existing", "--kind", kind)
				require.Equal(t, 0, code, stderr)

				m, err := store.LoadManifest(dir)
				require.NoError(t, err)
				require.Equal(t, []store.CodeRoot{{Root: ".", Mode: store.CodeModeSurface}}, m.Inputs.Code)
				require.NotNil(t, m.Project.Adopted)
				require.Equal(t, "git:"+head, m.Project.Adopted.Commit)
				at, err := time.Parse(time.RFC3339, m.Project.Adopted.At)
				require.NoError(t, err)
				require.Equal(t, at.UTC().Format(time.RFC3339), m.Project.Adopted.At)
				require.Equal(t, head, gitOutput(t, dir, "rev-parse", "--short", "HEAD"))
				after, err := os.ReadFile(filepath.Join(dir, "app", "code"))
				require.NoError(t, err)
				require.Equal(t, content, after, "HEAD observation never changes the working code")
			})
		}
	}
}

func TestInitOmitsUnavailableOrInapplicableAdoption(t *testing.T) {
	cases := []struct {
		kind string
		git  string
	}{
		{store.KindBrownfield, "none"},
		{store.KindLegacyMigration, "none"},
		{store.KindBrownfield, "unborn"},
		{store.KindLegacyMigration, "unborn"},
		{store.KindGreenfield, "committed"},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+tc.git, func(t *testing.T) {
			dir := t.TempDir()
			if tc.git == "committed" {
				dir = initCommittedCodeRepo(t)
			} else if tc.git == "unborn" {
				gitRun(t, dir, "init")
			}
			t.Chdir(dir)
			_, stderr, code := execCLI(t, "init", "--name", "Original", "--key", "original", "--kind", tc.kind)
			require.Equal(t, 0, code, stderr)
			m, err := store.LoadManifest(dir)
			require.NoError(t, err)
			require.Nil(t, m.Project.Adopted)
			if tc.kind == store.KindGreenfield {
				require.Empty(t, m.Inputs.Code)
			} else {
				require.Equal(t, []store.CodeRoot{{Root: ".", Mode: store.CodeModeSurface}}, m.Inputs.Code)
			}
			before, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
			require.NoError(t, err)

			if tc.git == "none" {
				gitRun(t, dir, "init")
			}
			gitRun(t, dir, "config", "user.email", "test@example.com")
			gitRun(t, dir, "config", "user.name", "Tiller Test")
			gitRun(t, dir, "config", "commit.gpgsign", "false")
			gitRun(t, dir, "config", "gc.auto", "0")
			gitRun(t, dir, "config", "maintenance.auto", "false")
			writeFixture(t, dir, "later-code", "later code\n")
			gitRun(t, dir, "add", "later-code")
			gitRun(t, dir, "commit", "-m", "later HEAD")
			_, stderr, code = execCLI(t, "init", "--name", "Other", "--key", "other", "--kind", store.KindBrownfield)
			require.Equal(t, 0, code, stderr)
			after, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
			require.NoError(t, err)
			require.Equal(t, before, after, "reruns never backfill adoption, roots, or kind")
			m, err = store.LoadManifest(dir)
			require.NoError(t, err)
			require.Nil(t, m.Project.Adopted)
		})
	}
}

func TestInitRerunPreservesManifestAndReportsStoredIdentity(t *testing.T) {
	dir := initCommittedCodeRepo(t)
	src := t.TempDir()
	writeFixture(t, src, "design.md", "original design\n")
	t.Chdir(dir)
	_, stderr, code := execCLI(t, "init", "--name", "Stored Name", "--key", "stored", "--kind", store.KindBrownfield, "--design", src)
	require.Equal(t, 0, code, stderr)
	path := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), "{ root: ., mode: surface }", "{ root: app, mode: surface }", 1) + "\n# Human-owned project notes.\n")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	original, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.NotNil(t, original.Project.Adopted)
	importedBefore := snapshotTree(t, filepath.Join(dir, ".tiller", "design"))
	writeFixture(t, src, "design.md", "changed external design\n")
	writeFixture(t, src, "new.md", "not imported on rerun\n")
	sourceBefore := snapshotTree(t, src)
	writeFixture(t, dir, "app/code", "later code\n")
	gitRun(t, dir, "add", "app/code")
	gitRun(t, dir, "commit", "-m", "advance HEAD")
	require.NotEqual(t, original.Project.Adopted.Commit, "git:"+gitOutput(t, dir, "rev-parse", "--short", "HEAD"))

	args := []string{"init", "--name", "Flag Name", "--key", "flag", "--kind", store.KindGreenfield, "--design", src}
	out, stderr, code := execCLI(t, append(args, "--json")...)
	require.Equal(t, 0, code, stderr)
	var envelope struct {
		Data initResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Equal(t, ProjectInfo{Name: "Stored Name", Key: "stored", Kind: store.KindBrownfield}, envelope.Data.Project)
	require.False(t, envelope.Data.Created)
	require.Nil(t, envelope.Data.Design)
	var shape struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &shape))
	require.Len(t, shape.Data, 3, "rerun envelope remains project, path, created")
	require.Contains(t, shape.Data, "project")
	require.Contains(t, shape.Data, "path")
	require.Contains(t, shape.Data, "created")
	human, stderr, code := execCLI(t, args...)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, human, `"Stored Name"`)
	require.NotContains(t, human, `"Flag Name"`)
	require.NotContains(t, human, "**Next**", "reruns do not restart onboarding")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, after, "anchor, roots, identity, provenance, and human prose remain unchanged")
	require.Equal(t, importedBefore, snapshotTree(t, filepath.Join(dir, ".tiller", "design")))
	require.Equal(t, sourceBefore, snapshotTree(t, src))
}

func TestInitFreshReadinessGapsByKind(t *testing.T) {
	for _, kind := range []string{store.KindGreenfield, store.KindBrownfield, store.KindLegacyMigration} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			_, stderr, code := execCLI(t, "init", "--name", "Fresh", "--key", "fresh", "--kind", kind)
			require.Equal(t, 0, code, stderr)
			out, stderr, code := execCLI(t, "check", "readiness", "--json")
			require.Equal(t, 1, code, stderr)
			var result struct {
				Data struct {
					Gaps []struct {
						Reason string `json:"reason"`
					} `json:"gaps"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			var reasons []string
			for _, gap := range result.Data.Gaps {
				reasons = append(reasons, gap.Reason)
			}
			want := []string{store.ReasonMissingContributors, store.ReasonMissingDesign}
			if kind == store.KindLegacyMigration {
				want = append(want, store.ReasonNoReferenceRoot)
			}
			if kind != store.KindGreenfield {
				want = append(want, "no_confirmed_baseline")
			}
			require.Equal(t, want, reasons)
			_, stderr, code = execCLI(t, "add", "contributor", "jc", "--name", "JC")
			require.Equal(t, 0, code, stderr)
			writeFixture(t, dir, ".tiller/design/design.md", "reviewed design\n")
			if kind == store.KindLegacyMigration {
				_, stderr, code = execCLI(t, "check", "readiness", "--json")
				require.Equal(t, 1, code, stderr, "docs and an owner do not supply the old-system declaration")
				require.NoError(t, os.Mkdir(filepath.Join(dir, "legacy"), 0o755))
				path := filepath.Join(dir, ".tiller", "tiller.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				data = []byte(strings.Replace(string(data), "    - { root: ., mode: surface }",
					"    - { root: ., mode: surface }\n    - { root: legacy, mode: reference }", 1))
				require.NoError(t, os.WriteFile(path, data, 0o644))
			}
			out, stderr, code = execCLI(t, "check", "readiness", "--json")
			if kind == store.KindGreenfield {
				require.Equal(t, 0, code, stderr)
			} else {
				require.Equal(t, 1, code, stderr)
				require.NoError(t, json.Unmarshal([]byte(out), &result))
				require.Len(t, result.Data.Gaps, 1)
				require.Equal(t, "no_confirmed_baseline", result.Data.Gaps[0].Reason)
			}
		})
	}
}

// snapshotWholeTree walks root and returns every regular file's contents keyed
// by its path relative to root, plus every directory's relative path, so a
// test can assert nothing anywhere in the whole tree changed rather than
// checking a handful of named files. This also catches scaffold directories
// and ignore seeding before a refusal. Unlike snapshotTree below, this includes
// directories as well as regular files.
func snapshotWholeTree(t *testing.T, root string) (files map[string][]byte, dirs []string) {
	t.Helper()
	files = map[string][]byte{}
	err := filepath.Walk(root, func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if info.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[rel] = data
		return nil
	})
	require.NoError(t, err)
	return files, dirs
}

// TestInitOnAVersionOneTreeRefusesWithoutRewritingAnything confirms a refused
// init is a clean stop across the whole tree, not just the three files a
// round-1 version of this test happened to name. Before round 1's fix,
// writeGenerated (init.go) wrote orientation.md and only then called
// store.Load; round 1 fixed that ordering but scaffold itself still ran its
// skeleton-directory MkdirAll loop and seeded a root .gitignore before ever
// validating a pre-existing manifest, so a refused re-run over a real
// version-1 tree still created five new directories and a new tracked
// .gitignore. scaffold now validates before any of its own writes, so this
// snapshots the entire tree (every file's contents, every directory) before
// and after the refused run and asserts both are identical.
func TestInitOnAVersionOneTreeRefusesWithoutRewritingAnything(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))
	manifest := "schema_version: 1\nschema_epoch: initial\nproject: { name: Old, key: old, kind: greenfield }\nowners: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), []byte(manifest), 0o644))

	beforeFiles, beforeDirs := snapshotWholeTree(t, dir)

	out, _, code := execCLI(t, "init", "--name", "Old", "--key", "old", "--kind", "greenfield", "--json")
	require.Equal(t, 3, code, "the schema_version refusal is an environment error")
	require.Contains(t, out, `"reason": "schema_version_unsupported"`)

	afterFiles, afterDirs := snapshotWholeTree(t, dir)
	require.ElementsMatch(t, beforeDirs, afterDirs, "a refused init must create no new directory anywhere in the repo")
	require.Equal(t, beforeFiles, afterFiles, "a refused init must write no file anywhere in the repo, the manifest and any repo-root file included")
}

// TestInitRendersEmptyRosterAndNoOwnerEnvelope confirms the manifest ships the
// empty roster `contributors: []` with the join comment, and the success envelope
// carries no owner (identity belongs to join now).
func TestInitRendersEmptyRosterAndNoOwnerEnvelope(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	data, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	manifest := string(data)
	require.Contains(t, manifest, "contributors: []", "the roster ships empty")
	require.Contains(t, manifest, "tiller add contributor", "the owners comment points at add contributor, the only command that registers one")
	// The seeded file is self-contained (issue #17): its breadcrumb points at
	// the in-project orientation, never at the Tiller source repo's docs.
	require.Contains(t, manifest, "Inline comments describe each block; .tiller/orientation.md covers where things live.")
	require.NotContains(t, manifest, "docs/spec", "the seeded manifest never points outside the project")

	require.NotContains(t, out, "\"owner\"", "the init envelope carries no owner block")
	require.NotContains(t, out, "\"id\"", "the init envelope carries no owner id")
}

// TestInitSeedsTheIntegrationBranchItWasScaffoldedOn confirms the seeded
// git.integration_branch follows the repo rather than a hardcoded default: a
// project inited on master seeds master, so a team that does not integrate on
// main never gets a manifest that contradicts their own repo. It is the explicit
// half of the resolution order; without it sync falls back to the branch the
// checkout is standing on, which is what the self-merge refusal exists to catch.
func TestInitSeedsTheIntegrationBranchItWasScaffoldedOn(t *testing.T) {
	dir := t.TempDir()
	testgit.Init(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "init")
	gitRun(t, dir, "branch", "-M", "master")
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "Legacy", "--key", "legacy", "--kind", "brownfield", "--json")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Equal(t, "master", m.Git.IntegrationBranch, "the seed follows the repo, never a hardcoded main")

	data, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Contains(t, string(data), `integration_branch: "master"`,
		"the value is written as a quoted scalar, like the neighbouring branch_pattern")
}

// TestInitSeedsMainOutsideAGitRepo confirms the detection degrades rather than
// failing: init must scaffold in a directory that is not a git repo at all, and
// there the honest answer is the conventional main, which the human edits.
func TestInitSeedsMainOutsideAGitRepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Equal(t, "main", m.Git.IntegrationBranch)
}

// TestInitHasNoOwnerFlags confirms init no longer accepts --contributor or
// --contributor-name: identity moved to join, so these are unknown flags (misuse).
func TestInitHasNoOwnerFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--contributor", "jc")
	require.Equal(t, 2, code, "--contributor is no longer an init flag")

	_, _, code = execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--contributor-name", "JC")
	require.Equal(t, 2, code, "--contributor-name is no longer an init flag")
}

// TestInitSucceedsWithoutOwner confirms init no longer refuses an ownerless
// project: as a pure scaffold it succeeds with no owner resolvable at all.
func TestInitSucceedsWithoutOwner(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("TILLER_CONTRIBUTOR", "")

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code, "init scaffolds without any owner")

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Empty(t, m.Contributors)
}

// TestInitSeedsEvalsGateActive confirms init wires the acceptance contract into
// check/sync by default at feature grain: the canonical evals required gate is
// active on features only, so the contract gates the feature while tasks gate on
// build and tests (issue #11). The project-specific build and tests gates stay
// commented templates the team fills in, and those templates keep task grain.
func TestInitSeedsEvalsGateActive(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)

	evals, ok := m.Gates.Required["evals"]
	require.True(t, ok, "the evals required gate is active by default")
	require.Equal(t, "command", evals.Kind)
	require.Equal(t, "tiller eval ${node.noun} ${node.id}", evals.Run)
	require.Equal(t, store.StringList{"feature"}, evals.On,
		"the seeded evals gate scopes to feature grain: the contract gates the feature")

	// The commented build/tests templates keep task grain, so a team that fills
	// them in gets the task loop gating on build and tests.
	data, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	manifest := string(data)
	require.Contains(t, manifest, `# build: { kind: command, run: "<build command>", on: [feature, task] }`)
	require.Contains(t, manifest, `# tests: { kind: command, run: "<test command>",  on: [feature, task] }`)

	// build and tests stay commented templates: their commands are project-specific.
	_, hasBuild := m.Gates.Required["build"]
	require.False(t, hasBuild, "build stays a commented template")
	_, hasTests := m.Gates.Required["tests"]
	require.False(t, hasTests, "tests stays a commented template")
}

// Fresh scaffolds enable tracked baseline drift, leaving noisy content drift opt-in.
func TestInitSeedsDefaultDriftChecks(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Equal(t,
		[]string{"stale_spec", "active_basis", "answered_question", "missing_surface", "orphan_eval", "stale_design"},
		m.Drift.Checks)
	require.NotContains(t, m.Drift.Checks, "current_state_match")
	require.NotContains(t, m.Drift.Checks, "shifted_design")
}

// The generated ignores must hide Tiller's root-local files without hiding
// a project's own artifacts or nested directories of the same names (#104).
func TestInitSeedsGitignoreWithArtifacts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	gitRun(t, dir, "init", "-q")

	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	ignored := gitOut(t, dir, "-c", "core.excludesFile=/dev/null", "check-ignore", "--no-index", "--",
		"artifacts/tiller/gates/result.json", ".tiller/.cache/index", ".bin/tiller", ".tiller/identity.yml",
		".tiller/local.yml", ".tiller/board.md", ".tiller/orientation.md",
		"artifacts/project/result.json", "src/artifacts/tiller/source.go", "src/.bin/tool",
		"src/.tiller/.cache/index", "src/.tiller/identity.yml", "src/.tiller/local.yml", "src/.tiller/board.md")
	require.ElementsMatch(t, []string{
		"artifacts/tiller/gates/result.json", ".tiller/.cache/index", ".bin/tiller", ".tiller/identity.yml",
		".tiller/local.yml",
	}, strings.Fields(ignored))
}

// TestInitDoesNotClobberExistingGitignore confirms init leaves a project's own
// .gitignore untouched, the same conservative posture it takes with the manifest.
func TestInitDoesNotClobberExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	existing := "node_modules/\n*.log\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644))

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, existing, string(data), "a human-owned .gitignore is not clobbered")
}

// TestInitRejectsBadKind confirms an unknown archetype is misuse (exit 2).
func TestInitRejectsBadKind(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "spaceship", "--json")
	require.Equal(t, 2, code)
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "tiller.yml"))
}

// TestInitRejectsBadKey confirms a key that breaks the segment grammar is misuse.
func TestInitRejectsBadKey(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "Bad_Key", "--kind", "greenfield", "--json")
	require.Equal(t, 2, code)
}

// TestInitRequiresFlags confirms missing required flags are misuse (exit 2),
// caught by cobra before the command body runs.
func TestInitRequiresFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 2, code)
}

// writeFixture writes content to root/rel, creating parent directories. It is the
// builder for the --design import fixtures below.
func writeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

// snapshotTree maps every regular file under root to its content (keyed by path
// relative to root). It captures a design source before and after an import so a
// test can prove the copy-out never mutated the source.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		out[rel] = string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}

// TestInitDesignImportsTreeWithProvenance is the core --design acceptance: the
// contents of the source folder land under .tiller/design/ preserving the
// subfolder structure, the manifest records the import provenance, the success
// envelope carries the imported-file count, and the source is byte-identical
// afterward (copy-out-only, never mutated).
func TestInitDesignImportsTreeWithProvenance(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	writeFixture(t, src, "00-index.md", "# Index\n")
	writeFixture(t, src, "discovery-analysis/01-domain.md", "domain model\n")
	writeFixture(t, src, "solution-design/02-architecture.md", "architecture\n")

	before := snapshotTree(t, src)

	t.Chdir(dir)
	out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", src, "--json")
	require.Equal(t, 0, code)

	// The contents land under .tiller/design/ with structure and bytes preserved.
	designDir := filepath.Join(dir, ".tiller", "design")
	for rel, content := range before {
		got, err := os.ReadFile(filepath.Join(designDir, rel))
		require.NoError(t, err, "imported %s", rel)
		require.Equal(t, content, string(got))
	}

	// The source fixture is never mutated by the copy-out.
	require.Equal(t, before, snapshotTree(t, src), "source fixture must not be mutated")

	// The manifest records the import provenance and stays loadable.
	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Inputs.ImportedFrom, 1)
	absSrc, err := filepath.Abs(src)
	require.NoError(t, err)
	require.Equal(t, absSrc, m.Inputs.ImportedFrom[0].Path)
	require.NotEmpty(t, m.Inputs.ImportedFrom[0].At, "import timestamp is recorded")
	require.Equal(t, []string{"design/"}, []string(m.Inputs.Docs), "design/ stays the docs root")

	// The success envelope gains the imported-docs count.
	require.Contains(t, out, "\"design\"")
	require.Contains(t, out, "\"files\": 3")
}

// TestInitWithoutDesignLeavesDesignEmpty confirms plain init (no --design) is
// unchanged: design/ is empty, the manifest records no import, and the envelope
// carries no design block.
func TestInitWithoutDesignLeavesDesignEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--json")
	require.Equal(t, 0, code)

	entries, err := os.ReadDir(filepath.Join(dir, ".tiller", "design"))
	require.NoError(t, err)
	require.Empty(t, entries, "plain init leaves design/ empty")
	require.NotContains(t, out, "\"design\"", "no design block without --design")

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Empty(t, m.Inputs.ImportedFrom, "no import provenance without --design")
}

// TestInitDesignRerunImportsNothingAndSaysSo pins the honest half of issue
// #89: a --design re-run over an existing manifest still imports nothing
// (scaffold gates the copy on the manifest being written, to keep the import
// and its provenance in lockstep), but the output now says the import was
// skipped and where to put the docs instead of "skeleton ensured" alone.
// TestInitExistingCodeNextNamesNoFixedDecompose pins the brownfield zero-day
// design at init: an existing-code project learns its next seam from
// `tiller guide` after the baseline, never from a hard-coded decompose step,
// because a confirmed baseline with no to-be doc has nothing to decompose.
func TestInitExistingCodeNextNamesNoFixedDecompose(t *testing.T) {
	for _, kind := range []string{"brownfield", "legacy-migration"} {
		t.Run(kind, func(t *testing.T) {
			t.Chdir(t.TempDir())
			out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", kind)
			require.Equal(t, 0, code)
			require.Contains(t, out, "tiller guide derive-baseline")
			require.NotContains(t, out, "tiller guide decompose")
			require.Contains(t, out, "tiller guide\n", "plain guide lists the ranked next moves")
		})
	}
	t.Run("greenfield", func(t *testing.T) {
		t.Chdir(t.TempDir())
		out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
		require.Equal(t, 0, code)
		require.Contains(t, out, "tiller guide decompose", "greenfield's first step copies design inputs, which make decompose due")
	})
}

func TestInitDesignRerunImportsNothingAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "brownfield")
	require.Equal(t, 0, code)

	src := t.TempDir()
	writeFixture(t, src, "spec.md", "# spec\n")

	out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "brownfield", "--design", src)
	require.Equal(t, 0, code, "a re-run is still not a refusal")
	require.Contains(t, out, "skeleton ensured", "the re-run headline is unchanged")
	require.Contains(t, out, "Design import skipped", "the re-run names the skipped import")
	require.Contains(t, out, "Copy the docs under .tiller/design/ by hand", "and says what to do instead")

	entries, err := os.ReadDir(filepath.Join(dir, ".tiller", "design"))
	require.NoError(t, err)
	require.Empty(t, entries, "nothing was imported")
	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Empty(t, m.Inputs.ImportedFrom, "no provenance was recorded")

	// The notice is human output only; the JSON envelope carries no design block.
	jout, _, jcode := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "brownfield", "--design", src, "--json")
	require.Equal(t, 0, jcode)
	require.NotContains(t, jout, "\"design\"", "no design block when nothing was imported")
	require.NotContains(t, jout, "skipped", "the notice never enters the envelope")
}

func TestInitDesignRejectsForeignLedgerBeforeWriting(t *testing.T) {
	const confirmed = `docs:
  - path: as-is.md
    describes: [src]
    confirmed:
      by: foreign-owner
      commit: {kind: git_commit, value: "git:abc1234", observed_at: "2026-09-05T00:00:00Z"}
      surfaces:
        - {path: src, kind: git_tree, value: "git:def5678", observed_at: "2026-09-05T00:00:00Z", inventory: [src/app.go]}
`
	for _, tc := range []struct{ name, file, ledger string }{
		{"foreign confirmation", "baseline.yml", confirmed},
		{"malformed ledger", "baseline.yml", "docs: ["},
		{"portable reserved name", "BASELINE.YML", confirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, src := t.TempDir(), t.TempDir()
			writeFixture(t, src, "as-is.md", "# Existing system\n")
			writeFixture(t, src, tc.file, tc.ledger)
			before := snapshotTree(t, src)
			t.Chdir(root)
			_, stderr, code := execCLI(t, "init", "--name", "New", "--key", "new", "--kind", "brownfield", "--design", src, "--json")
			require.Equal(t, 2, code, stderr)
			require.NoDirExists(t, filepath.Join(root, ".tiller"))
			require.NoFileExists(t, filepath.Join(root, ".gitignore"))
			require.Equal(t, before, snapshotTree(t, src))
		})
	}
}

// TestInitDesignRefusesMissingSource confirms a --design path that does not exist
// is refused cleanly with no half-scaffolded repo.
func TestInitDesignRefusesMissingSource(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", filepath.Join(dir, "no-such-dir"), "--json")
	require.NotEqual(t, 0, code)
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "tiller.yml"))
}

// TestInitDesignRefusesEmptySource confirms an empty --design source is refused
// cleanly with no half-scaffolded repo.
func TestInitDesignRefusesEmptySource(t *testing.T) {
	dir := t.TempDir()
	empty := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", empty, "--json")
	require.NotEqual(t, 0, code)
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "tiller.yml"))
}

// TestInitDesignRefusesNonDirSource confirms a --design that points at a file,
// not a directory, is refused cleanly with no half-scaffolded repo.
func TestInitDesignRefusesNonDirSource(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	file := filepath.Join(src, "a-file.md")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", file, "--json")
	require.NotEqual(t, 0, code)
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "tiller.yml"))
}

// TestInitDesignImportSkipsOSJunk confirms the --design import filters OS junk
// (.DS_Store, Thumbs.db) instead of committing it into the design tree, and
// that a source holding nothing but junk is still an empty source (misuse).
func TestInitDesignImportSkipsOSJunk(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	writeFixture(t, src, "00-index.md", "# Index\n")
	writeFixture(t, src, ".DS_Store", "junk\n")
	writeFixture(t, src, "solution-design/.DS_Store", "junk\n")
	writeFixture(t, src, "solution-design/Thumbs.db", "junk\n")
	writeFixture(t, src, "solution-design/02-architecture.md", "architecture\n")

	t.Chdir(dir)
	out, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", src, "--json")
	require.Equal(t, 0, code)

	designDir := filepath.Join(dir, ".tiller", "design")
	require.FileExists(t, filepath.Join(designDir, "00-index.md"))
	require.FileExists(t, filepath.Join(designDir, "solution-design", "02-architecture.md"))
	require.NoFileExists(t, filepath.Join(designDir, ".DS_Store"))
	require.NoFileExists(t, filepath.Join(designDir, "solution-design", ".DS_Store"))
	require.NoFileExists(t, filepath.Join(designDir, "solution-design", "Thumbs.db"))
	require.Contains(t, out, "\"files\": 2", "the imported count excludes the junk")
}

// TestInitDesignJunkOnlySourceIsEmpty confirms a design source holding only OS
// junk is refused the same way an empty folder is.
func TestInitDesignJunkOnlySourceIsEmpty(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	writeFixture(t, src, ".DS_Store", "junk\n")

	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield",
		"--design", src, "--json")
	require.Equal(t, 2, code, "a junk-only source is an empty source (misuse)")
}

// --- importDocs (the importDesign generalization task-4-brief.md asks for) ---

// TestImportDocsCopiesASingleFileToItsBasenameUnderDestDir pins the new branch
// importDesign's rename to importDocs adds: a srcPath naming a plain file
// lands at destDir/<basename>, the shape `tiller intake` needs
// (.tiller/feedback/<basename>), rather than importDesign's own directory-walk
// arithmetic (which, unchanged, would compute a blank relative path and write
// the file's bytes over destDir itself).
func TestImportDocsCopiesASingleFileToItsBasenameUnderDestDir(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "review.md")
	require.NoError(t, os.WriteFile(src, []byte("feedback body\n"), 0o644))

	destDir := t.TempDir()
	n, err := importDocs(destDir, src, ".")
	require.NoError(t, err)
	require.Equal(t, 1, n)

	got, err := os.ReadFile(filepath.Join(destDir, "review.md"))
	require.NoError(t, err)
	require.Equal(t, "feedback body\n", string(got))
}

// TestImportDocsStillWalksADirectoryPreservingStructure pins the directory
// branch unchanged: importDesign's own contract (subfolder structure
// preserved, junk skipped, non-regular entries skipped, count returned) must
// survive the rename and the new file branch added beside it.
func TestImportDocsStillWalksADirectoryPreservingStructure(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("a\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub", "b.md"), []byte("b\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, ".DS_Store"), []byte("junk\n"), 0o644))

	destDir := t.TempDir()
	n, err := importDocs(destDir, srcDir, "docs")
	require.NoError(t, err)
	require.Equal(t, 2, n)

	require.FileExists(t, filepath.Join(destDir, "docs", "a.md"))
	require.FileExists(t, filepath.Join(destDir, "docs", "sub", "b.md"))
	require.NoFileExists(t, filepath.Join(destDir, "docs", ".DS_Store"))
}
