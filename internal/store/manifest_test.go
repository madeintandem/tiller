package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// tempRepo builds a temp repo root with .tiller/tiller.yml copied from the
// named testdata fixture. When withDesign is true it also lays a non-empty
// design/ dir, so Validate's design check passes.
func tempRepo(t *testing.T, fixture string, withDesign bool) string {
	t.Helper()
	root := t.TempDir()
	tillerDir := filepath.Join(root, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))

	data, err := os.ReadFile(filepath.Join("testdata", fixture))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), data, 0o644))

	if withDesign {
		designDir := filepath.Join(tillerDir, "design")
		require.NoError(t, os.MkdirAll(designDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(designDir, "00-index.md"), []byte("# index\n"), 0o644))
	}
	return root
}

func asEnvError(t *testing.T, err error) *EnvError {
	t.Helper()
	require.Error(t, err)
	var ee *EnvError
	require.True(t, errors.As(err, &ee), "expected *EnvError, got %T: %v", err, err)
	return ee
}

// scaffoldTreeWithSchemaVersion builds a temp repo root with a minimal
// .tiller/tiller.yml stamped at the given schema_version, so the schema_version
// refusal can be exercised at a version this package's other fixtures do not
// carry.
func scaffoldTreeWithSchemaVersion(t *testing.T, version int) string {
	t.Helper()
	root := t.TempDir()
	tillerDir := filepath.Join(root, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))
	manifest := fmt.Sprintf("schema_version: %d\nschema_epoch: initial\n", version)
	require.NoError(t, os.WriteFile(filepath.Join(tillerDir, "tiller.yml"), []byte(manifest), 0o644))
	return root
}

func TestLoadManifest_Valid(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)

	m, err := LoadManifest(root)
	require.NoError(t, err)

	assert.Equal(t, CurrentSchemaVersion, m.SchemaVersion)
	assert.Equal(t, "initial", m.SchemaEpoch)
	assert.Equal(t, "Acme Platform", m.Project.Name)
	assert.Equal(t, "acme", m.Project.Key)
	assert.Equal(t, "greenfield", m.Project.Kind)
	assert.Equal(t, ">= 1.0", m.Project.RequiresTiller)
	require.Len(t, m.Contributors, 2)
	assert.Equal(t, Contributor{ID: "jc", Name: "JC Grubbs"}, m.Contributors[0])
	assert.Equal(t, "pat", m.Contributors[1].ID)

	assert.Equal(t, []string{"design/"}, m.Inputs.Docs)
	assert.Empty(t, m.Inputs.Code)

	// Required gate `on` is a sequence; optional gate `on` is a scalar. Both
	// normalize to a StringList.
	build, ok := m.Gates.Required["build"]
	require.True(t, ok)
	assert.Equal(t, "command", build.Kind)
	assert.Equal(t, "bin/rails runner 'true'", build.Run)
	assert.Equal(t, StringList{"feature", "task"}, build.On)

	cr, ok := m.Gates.Optional["code_review"]
	require.True(t, ok)
	assert.Equal(t, "attestation", cr.Kind)
	assert.Equal(t, StringList{"pr"}, cr.On)

	assert.Equal(t, "tiller/${node.key}", m.Git.BranchPattern)
	assert.True(t, m.Git.PRCoherence)
	assert.Contains(t, m.Drift.Checks, "stale_spec")

	assert.Equal(t, root, m.Root())
	require.NoError(t, m.Validate())
	assert.Empty(t, m.Gaps())
}

// TestRequiresTillerOfIsLenient: the advisory probe reads project.requires_tiller
// through the loosest possible parse, so it still answers on a tree whose
// schema stamp this binary refuses (a binary behind the floor is exactly the
// one most likely to be behind the schema), and answers "" for no manifest,
// no field, or a document that is not a mapping at all.
func TestRequiresTillerOfIsLenient(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)
	assert.Equal(t, ">= 1.0", RequiresTillerOf(root))

	assert.Equal(t, "", RequiresTillerOf(t.TempDir()), "no manifest, no floor")

	dir := t.TempDir()
	tillerDir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))
	manifest := filepath.Join(tillerDir, "tiller.yml")

	require.NoError(t, os.WriteFile(manifest, []byte("schema_version: 999\nproject:\n  requires_tiller: \">= 2.0\"\n"), 0o644))
	assert.Equal(t, ">= 2.0", RequiresTillerOf(dir), "a stamp this binary refuses still yields the floor")

	require.NoError(t, os.WriteFile(manifest, []byte("schema_version: 6\nproject:\n  name: X\n"), 0o644))
	assert.Equal(t, "", RequiresTillerOf(dir), "no field, no floor")

	require.NoError(t, os.WriteFile(manifest, []byte("- not\n- a mapping\n"), 0o644))
	assert.Equal(t, "", RequiresTillerOf(dir), "an unreadable document is silent, never an error")
}

func TestLoadManifest_NotARepo(t *testing.T) {
	root := t.TempDir() // no .tiller/

	_, err := LoadManifest(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonNotARepo, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

// TestLoadRefusesAVersionOneTree pins distribution.md's Migration section: the
// 1-to-2 boundary of the single-tree refactor ships as a clean refusal, not a
// migration, because no released binary or surviving tree exists on the
// version-1 side of it. The message names the move that actually clears the
// refusal, removing .tiller/ and running tiller init, not a bare "re-init":
// tiller init's own write-if-absent guard means running it without removing
// .tiller/ first hits this exact refusal again and exits 3 with nothing
// rewritten.
func TestLoadRefusesAVersionOneTree(t *testing.T) {
	root := scaffoldTreeWithSchemaVersion(t, 1)
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
	assert.Contains(t, ee.Msg, "remove .tiller/", "names the removal, not just the doomed re-run")
	assert.Contains(t, ee.Msg, "tiller init")
}

// TestLoadRefusesANewerSchemaVersionTree pins the other direction distribution.md's
// Versioning section calls out: a tree stamped past what this binary reads is a
// hard refuse with the opposite remedy from an older tree, upgrade the binary,
// since removing .tiller/ here would discard project data already ahead of this
// binary. Also guards against a later narrowing of the comparison (say, to
// "< CurrentSchemaVersion", implementing distribution.md's eventual read-range)
// that would keep TestLoadRefusesAVersionOneTree green while silently admitting
// a future tree at this binary's semantics.
func TestLoadRefusesANewerSchemaVersionTree(t *testing.T) {
	root := scaffoldTreeWithSchemaVersion(t, CurrentSchemaVersion+1)
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
	assert.NotContains(t, ee.Msg, "remove .tiller/", "a newer tree must never be told to delete its own data")
}

func TestLoadRefusesSchemaSixBeforeReadingMoveProposals(t *testing.T) {
	root := scaffoldTreeWithSchemaVersion(t, 6)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = Load(root)
	refusal := asEnvError(t, err)
	require.Equal(t, ReasonSchemaVersionUnsupported, refusal.Reason)
	require.Contains(t, refusal.Msg, "tiller upgrade --project-only")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestLoadRefusesAMissingSchemaVersionKey pins that an absent key is named as
// absent, and that this case is NOT the destructive old-tree remedy (round-3
// finding): a genuine version-1 tree always carried the key (the pre-refactor
// manifest struct declared SchemaVersion int, and every version-1 fixture in
// this codebase's history wrote schema_version: 1), so a missing key is not
// evidence of an old tree. It could just as well be a current-format project
// that lost the line to a hand-edit or a bad merge, so this must never say to
// remove anything.
func TestLoadRefusesAMissingSchemaVersionKey(t *testing.T) {
	root := writeManifestFixture(t, "project: { name: P, key: p, kind: greenfield }\n")
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Contains(t, ee.Msg, "no schema_version set")
	assert.NotContains(t, ee.Msg, "schema_version 0")
	assert.Contains(t, ee.Msg, "set schema_version by hand")
	assert.NotContains(t, ee.Msg, "remove .tiller/", "a missing stamp is not evidence of an old tree; it must never be told to delete its own data")
}

// TestLoadRefusesAnEmptySchemaVersionValue pins the sibling of the missing-key
// case: the key is present but carries no value, which yaml.v3 resolves as a
// null and decodes into the Go zero value, 0, with no error at all (round-2
// finding: "schema_version: with an empty value still prints 'is stamped
// schema_version 0'"). refuseSchemaVersion checks the node's tag for this
// case rather than trusting a decoded 0, so this gets the same "no
// schema_version set" message as a fully absent key, not a false "stamped 0".
// Round 3: it must also get the same neutral, non-destructive remedy as a
// missing key, for the identical reason (a genuine version-1 tree always
// wrote the key with a value; an empty one is not evidence of an old tree).
func TestLoadRefusesAnEmptySchemaVersionValue(t *testing.T) {
	root := writeManifestFixture(t, "schema_version:\nproject: { name: P, key: p, kind: greenfield }\n")
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Contains(t, ee.Msg, "no schema_version set")
	assert.NotContains(t, ee.Msg, "schema_version 0")
	assert.Contains(t, ee.Msg, "set schema_version by hand")
	assert.NotContains(t, ee.Msg, "remove .tiller/", "an empty stamp is not evidence of an old tree; it must never be told to delete its own data")
}

// TestLoadRefusesANonIntegerSchemaVersion pins that a schema_version that does
// not parse as an integer is classified as this same environment refusal
// (exit 3) rather than falling through to the generic parse error a raw
// yaml.Unmarshal into Manifest.SchemaVersion's int field would otherwise
// produce (exit 2, misuse). It must NOT get the old-tree remedy: this binary
// cannot tell whether "banana" belongs to a tree older or newer than what it
// reads, or is simply corrupted, so removing .tiller/ is not a safe answer.
func TestLoadRefusesANonIntegerSchemaVersion(t *testing.T) {
	root := writeManifestFixture(t, "schema_version: banana\nproject: { name: P, key: p, kind: greenfield }\n")
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
	assert.Contains(t, ee.Msg, `"banana"`)
	assert.Contains(t, ee.Msg, "not a valid integer")
	assert.Contains(t, ee.Msg, "correct schema_version by hand")
	assert.NotContains(t, ee.Msg, "remove .tiller/", "an unparseable value must never get the destructive old-tree remedy")
}

// TestLoadRefusesAQuotedSchemaVersionWithoutDestructiveAdvice is the round-2
// Important finding, reproduced directly: a hand-edit or any YAML formatter
// that quotes scalars turns `schema_version: 3` into `schema_version: "3"`, a
// string yaml.v3 will not decode into int even though the number is right
// there. Before this fix that fell into the same branch as "banana" and
// inherited the old-tree remedy, so a genuinely current-format tree carrying
// nothing but a quoting difference was told to delete .tiller/ entirely. It
// must instead get the neutral "correct it by hand" remedy, the same as any
// other unparseable value, and never the destructive one.
func TestLoadRefusesAQuotedSchemaVersionWithoutDestructiveAdvice(t *testing.T) {
	root := writeManifestFixture(t, "schema_version: \"3\"\nproject: { name: P, key: p, kind: greenfield }\n")
	_, err := Load(root)
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonSchemaVersionUnsupported, ee.Reason)
	assert.Contains(t, ee.Msg, `"3"`)
	assert.Contains(t, ee.Msg, "not a valid integer")
	assert.NotContains(t, ee.Msg, "remove .tiller/", "a quoted current-format stamp must never be told to delete its own data")
}

// TestLoadOnANonMappingDocumentNamesTheType pins that a malformed top-level
// document (a sequence, here, which cannot carry a schema_version key at all)
// reports a clean parse error naming schemaVersionProbe's own type, not an
// anonymous struct literal with its field and yaml tag spelled out, escaped
// quotes and all, leaking into user-facing text (and escaped a second time
// once wrapped into the --json envelope).
func TestLoadOnANonMappingDocumentNamesTheType(t *testing.T) {
	root := writeManifestFixture(t, "- a\n- b\n")
	_, err := Load(root)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), `yaml:\"schema_version\"`, "no escaped Go struct tag syntax in user-facing text")
	assert.Contains(t, err.Error(), "schemaVersionProbe")
}

func TestValidate_MissingOwners(t *testing.T) {
	root := tempRepo(t, "tiller-bad.yml", true)

	m, err := LoadManifest(root)
	require.NoError(t, err) // it parses; it is just incomplete

	ee := asEnvError(t, m.Validate())
	assert.Equal(t, ReasonMissingContributors, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

func TestValidate_MissingDesign(t *testing.T) {
	root := tempRepo(t, "tiller.yml", false) // no design/

	m, err := LoadManifest(root)
	require.NoError(t, err)

	ee := asEnvError(t, m.Validate())
	assert.Equal(t, ReasonMissingDesign, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

func TestValidate_MissingProjectKind(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)
	m, err := LoadManifest(root)
	require.NoError(t, err)

	m.Project.Kind = ""
	ee := asEnvError(t, m.Validate())
	assert.Equal(t, ReasonMissingProjectKind, ee.Reason)
}

func TestValidate_MissingCodeRoot(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)
	m, err := LoadManifest(root)
	require.NoError(t, err)

	m.Inputs.Code = []CodeRoot{{Root: "../does-not-exist", Mode: "reference"}}
	ee := asEnvError(t, m.Validate())
	assert.Equal(t, ReasonMissingCodeRoot, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

// Gaps reports every gap, not just the first, so check readiness can name them all.
func TestGaps_ReportsAll(t *testing.T) {
	root := tempRepo(t, "tiller.yml", false) // no design/
	m, err := LoadManifest(root)
	require.NoError(t, err)
	m.Contributors = nil
	m.Project.Kind = ""

	gaps := m.Gaps()
	assert.GreaterOrEqual(t, len(gaps), 3)
}

func TestResolveOwner(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)
	m, err := LoadManifest(root)
	require.NoError(t, err)

	got, err := ResolveContributor(m, "jc")
	require.NoError(t, err)
	assert.Equal(t, Contributor{ID: "jc", Name: "JC Grubbs"}, got)

	_, err = ResolveContributor(m, "")
	ee := asEnvError(t, err)
	assert.Equal(t, ReasonContributorUnset, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
	// A worktree or clone of an already joined repository has no identity yet;
	// the refusal says why and where join must run.
	assert.Contains(t, ee.Msg, "identity is per checkout")
	assert.Contains(t, ee.Msg, "new clone or worktree")
	assert.Contains(t, ee.Msg, "`tiller join --as <you>` runs there")

	_, err = ResolveContributor(m, "nobody")
	ee = asEnvError(t, err)
	assert.Equal(t, ReasonContributorUnknown, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

// TestGitIntegrationBranch pins the field sync resolves the branch merges land on
// from, and its omitempty. A manifest written before the field existed must load
// and round-trip unchanged: that is the whole compatibility story for a project
// already integrating on master or develop, which keeps working with no edit
// because sync falls back to the branch the checkout is standing on.
func TestGitIntegrationBranch(t *testing.T) {
	var declared GitBlock
	require.NoError(t, yaml.Unmarshal([]byte("branch_pattern: \"tiller/${node.key}\"\nintegration_branch: \"develop\"\n"), &declared))
	assert.Equal(t, "develop", declared.IntegrationBranch)

	var legacy GitBlock
	require.NoError(t, yaml.Unmarshal([]byte("branch_pattern: \"tiller/${node.key}\"\npr_coherence: true\n"), &legacy))
	assert.Empty(t, legacy.IntegrationBranch, "a manifest that predates the field declares none")

	out, err := yaml.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "integration_branch", "omitempty keeps the round trip clean")
}

func TestParseManifestNamesMigrateForAMigratableOlderStamp(t *testing.T) {
	for version := MinMigratableSchemaVersion; version < CurrentSchemaVersion; version++ {
		data := []byte(fmt.Sprintf("schema_version: %d\nschema_epoch: initial\nproject:\n  name: X\n  key: x\n  kind: greenfield\nowners: []\n", version))
		_, err := ParseManifest("/repo", data)
		var env *EnvError
		require.ErrorAs(t, err, &env)
		require.Equal(t, ReasonSchemaVersionUnsupported, env.Reason)
		require.NotContains(t, env.Msg, "remove .tiller/", "re-init is reserved for the pre-migratable boundary")
	}
}

func TestParseManifestStillNamesReinitBelowTheMigratableFloor(t *testing.T) {
	data := []byte("schema_version: 1\nowners: []\n")
	_, err := ParseManifest("/repo", data)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Contains(t, env.Msg, "remove .tiller/")
}

func TestSchemaVersionOfReadsAStampParseManifestRefuses(t *testing.T) {
	v, refusal := SchemaVersionOf("/repo/.tiller/tiller.yml", []byte("schema_version: 2\n"))
	require.Nil(t, refusal)
	require.Equal(t, 2, v)
	v, refusal = SchemaVersionOf("/repo/.tiller/tiller.yml", []byte("schema_version: 9\n"))
	require.Nil(t, refusal, "range classification is the caller's job; the probe only parses")
	require.Equal(t, 9, v)
}

func TestSchemaVersionOfRefusesMissingAndUnparseableStamps(t *testing.T) {
	_, refusal := SchemaVersionOf("/repo/.tiller/tiller.yml", []byte("project:\n  key: x\n"))
	require.NotNil(t, refusal)
	require.Contains(t, refusal.Msg, "no schema_version set")
	_, refusal = SchemaVersionOf("/repo/.tiller/tiller.yml", []byte("schema_version: \"two\"\n"))
	require.NotNil(t, refusal)
	require.Contains(t, refusal.Msg, "not a valid integer")
}

func TestRenderSchemaVersionRewritesOnlyTheStampLine(t *testing.T) {
	in := []byte("# a comment the human wrote\nschema_version: 2\nschema_epoch: initial   # keep me\n\nproject:\n  name: \"X\"\n")
	out, err := RenderSchemaVersion(in, 3)
	require.NoError(t, err)
	require.Equal(t,
		"# a comment the human wrote\nschema_version: 3\nschema_epoch: initial   # keep me\n\nproject:\n  name: \"X\"\n",
		string(out), "every byte outside the stamp line survives, comments and blank lines included")
}

func TestRenderSchemaVersionRefusesAManifestWithNoStampLine(t *testing.T) {
	_, err := RenderSchemaVersion([]byte("project:\n  key: x\n"), 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no schema_version line")
}

func TestParseManifestUncheckedReadsAnOlderTree(t *testing.T) {
	data := []byte("schema_version: 2\nowners:\n  - {\"id\": \"jc\", \"name\": \"JC\"}\n")
	m, err := ParseManifestUnchecked("/repo", data)
	require.NoError(t, err)
	require.Equal(t, 2, m.SchemaVersion)
	require.Len(t, m.Contributors, 1)
}

func TestParseManifestRefusesInvalidProjectReviewGrain(t *testing.T) {
	data := []byte("schema_version: 12\nexecution:\n  review_grain: sideways\n")
	_, err := ParseManifest("/repo", data)
	require.Error(t, err)
	require.Contains(t, err.Error(), ".tiller/tiller.yml")
	require.Contains(t, err.Error(), `execution.review_grain "sideways"`)
	require.Contains(t, err.Error(), "task, feature, or epic")
}

func TestParseManifestUncheckedStillRefusesInvalidReviewGrain(t *testing.T) {
	data := []byte("schema_version: 2\nexecution:\n  review_grain: sideways\n")
	_, err := ParseManifestUnchecked("/repo", data)
	require.Error(t, err)
	require.Contains(t, err.Error(), `execution.review_grain "sideways"`)
}

func TestManifestAdoptionRoundTrip(t *testing.T) {
	for _, anchor := range []string{"", "\n  adopted: {commit: 'git:abc1234', at: '2026-09-05T12:00:00Z'}"} {
		t.Run(anchor, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
			data := []byte("schema_version: 12\nproject:\n  kind: brownfield" + anchor + "\ninputs:\n  code: [{root: ., mode: surface}]\n")
			path := filepath.Join(root, ".tiller", "tiller.yml")
			require.NoError(t, os.WriteFile(path, data, 0o644))
			m, err := LoadManifest(root)
			require.NoError(t, err)
			encoded, err := yaml.Marshal(m)
			require.NoError(t, err)
			var before, after map[string]any
			require.NoError(t, yaml.Unmarshal(data, &before))
			require.NoError(t, yaml.Unmarshal(encoded, &after))
			assert.Equal(t, before["project"].(map[string]any)["adopted"], after["project"].(map[string]any)["adopted"])
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, data, unchanged)
		})
	}
}

func TestGapsInvalidCodeModesRemainDiscoverable(t *testing.T) {
	root := tempRepo(t, "tiller.yml", false)
	m, err := LoadManifest(root)
	require.NoError(t, err)
	m.Contributors = nil
	m.Inputs.Code = []CodeRoot{{Root: ".", Mode: ""}, {Root: "absent", Mode: "refrence"}}
	var reasons []string
	for _, gap := range m.Gaps() {
		ee := asEnvError(t, gap)
		reasons = append(reasons, ee.Reason)
		assert.Equal(t, 3, ee.ExitCode())
		if ee.Reason == "invalid_code_root_mode" {
			assert.Contains(t, ee.Msg, filepath.Join(root, ".tiller", "tiller.yml"))
			assert.Contains(t, ee.Msg, "surface")
			assert.Contains(t, ee.Msg, "reference")
		}
	}
	assert.Equal(t, []string{"missing_contributors", "missing_design", "invalid_code_root_mode", "invalid_code_root_mode", "missing_code_root"}, reasons)
}

func TestGapsLegacyReferenceDeclaration(t *testing.T) {
	root := tempRepo(t, "tiller.yml", true)
	m, err := LoadManifest(root)
	require.NoError(t, err)
	for _, kind := range []string{"greenfield", "brownfield", "legacy-migration"} {
		t.Run(kind, func(t *testing.T) {
			m.Project.Kind = kind
			m.Inputs.Code = []CodeRoot{{Root: ".", Mode: "surface"}}
			if kind == "legacy-migration" {
				gaps := m.Gaps()
				require.Len(t, gaps, 1)
				assert.Equal(t, "no_reference_root", asEnvError(t, gaps[0]).Reason)
			} else {
				assert.Empty(t, m.Gaps())
			}
			m.Inputs.Code = append(m.Inputs.Code, CodeRoot{Root: ".", Mode: "reference"})
			assert.Empty(t, m.Gaps())
			m.Inputs.Code[1].Root = "absent"
			gaps := m.Gaps()
			require.Len(t, gaps, 1)
			assert.Equal(t, "missing_code_root", asEnvError(t, gaps[0]).Reason)
		})
	}
}

func TestLoadRefusesSchemaSevenBeforeReadingTweakShelf(t *testing.T) {
	root := scaffoldTreeWithSchemaVersion(t, 7)
	writeFile(t, filepath.Join(root, ".tiller/tweaks/minor/tweak.yml"), "malformed: [\n")
	_, err := Load(root)
	refusal := asEnvError(t, err)
	require.Equal(t, ReasonSchemaVersionUnsupported, refusal.Reason)
	require.Contains(t, refusal.Msg, "tiller upgrade --project-only")
}

// A gate's reuse opt-out parses from the manifest: `reuse: false` turns reuse
// off for that gate, and an absent key leaves it on.
func TestManifestGateReuse(t *testing.T) {
	t.Parallel()
	var specs map[string]GateSpec
	require.NoError(t, yaml.Unmarshal([]byte(`
tests: { kind: command, run: "go test ./...", on: [task], reuse: false }
build: { kind: command, run: "go build ./...", on: [task] }
lint: { kind: command, run: "lint", on: [task], reuse: true }
`), &specs))
	assert.False(t, specs["tests"].Reusable())
	assert.True(t, specs["build"].Reusable())
	assert.True(t, specs["lint"].Reusable())
	assert.True(t, GateSpec{}.Reusable())
}
