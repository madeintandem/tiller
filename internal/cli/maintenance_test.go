package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// dirtyMigrationInput edits the committed manifest of a legacy checkout without
// committing it, the guarded business hold a real migration refuses on.
func dirtyMigrationInput(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, ".tiller", "tiller.yml")
	dirty := readFile(t, path) + "\n# human work\n"
	require.NoError(t, os.WriteFile(path, []byte(dirty), 0o644))
	return dirty
}

// TestMaintenanceRefusalKeepsTheValidatedActor drives the F21 scenario at the
// real surface: inspection validates the acting contributor against the roster
// and only then refuses the dirty migration input, so the partial result still
// names who was acting and which source resolved them, whichever source won.
func TestMaintenanceRefusalKeepsTheValidatedActor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		arrange func(t *testing.T, root string)
		source  string
	}{
		{name: "explicit flag", args: []string{"--contributor", "jc"}, source: contributorSourceFlag},
		{name: "environment", source: contributorSourceEnv, arrange: func(t *testing.T, root string) {
			t.Setenv("TILLER_CONTRIBUTOR", "jc")
		}},
		{name: "identity file", source: contributorSourceIdentity, arrange: func(t *testing.T, root string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "identity.yml"), []byte("owner: jc\n"), 0o644))
		}},
	} {
		for _, command := range [][]string{{"upgrade", "--project-only"}, {"migrate"}} {
			t.Run(tc.name+"/"+command[0], func(t *testing.T) {
				root := legacyMaintenanceFixture(t, 4)
				if tc.arrange != nil {
					tc.arrange(t, root)
				}
				dirty := dirtyMigrationInput(t, root)

				args := append(append(append([]string(nil), command...), tc.args...), "--json")
				out, _, code := execCLI(t, args...)

				require.Equal(t, 1, code, out)
				// migrate and upgrade share these fields; their legacy
				// from/to fields differ in type, so decode only the shared part.
				var e struct {
					Data *struct {
						Complete bool                     `json:"complete"`
						Failure  *maintenanceFailure      `json:"failure"`
						Project  projectMaintenanceResult `json:"project"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &e), out)
				require.NotNil(t, e.Data, "a guarded business hold answers in data")
				require.False(t, e.Data.Complete)
				require.Equal(t, "migration_inputs_dirty", e.Data.Failure.Reason)
				project := e.Data.Project
				require.Equal(t, "failed", project.Migration.Status)
				require.Equal(t, "migration_inputs_dirty", project.Migration.Failure.Reason)
				require.Equal(t, &actor{ID: "jc", Source: tc.source}, project.Actor)
				require.Equal(t, 4, project.SchemaBefore)
				require.Equal(t, 4, project.SchemaAfter)
				require.Equal(t, dirty, readFile(t, root, ".tiller", "tiller.yml"), "the refused source is untouched")
			})
		}
	}
}

// TestMaintenanceOwnershipReviewRefusalKeepsTheValidatedActor covers the other
// refusal that lands after inspection validated the contributor: a required
// ownership review that was never supplied.
func TestMaintenanceOwnershipReviewRefusalKeepsTheValidatedActor(t *testing.T) {
	activeLegacyMaintenanceFixture(t)

	out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")

	require.Equal(t, 3, code, out)
	failure := readMaintenanceEnvelope(t, out).Error
	require.Equal(t, store.ReasonOwnershipReviewRequired, failure.Reason)
	require.Equal(t, &actor{ID: "jc", Source: contributorSourceFlag}, failure.PartialResult.Project.Actor)
}

// TestMaintenanceNeverReportsAnUnvalidatedActor pins the other half: a
// contributor the roster does not hold, or none at all, is never reported as
// the actor of the partial result, with clean or dirty inputs. With no
// contributor a dirty input refuses first, since inspection has nobody to
// validate; that refusal names no actor either.
func TestMaintenanceNeverReportsAnUnvalidatedActor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		dirty  bool
		code   int
		reason string
	}{
		{"unknown contributor, clean input", []string{"--contributor", "missing"}, false, 3, store.ReasonContributorUnknown},
		{"unknown contributor, dirty input", []string{"--contributor", "missing"}, true, 3, store.ReasonContributorUnknown},
		{"no contributor, clean input", nil, false, 3, store.ReasonContributorUnset},
		{"no contributor, dirty input", nil, true, 1, "migration_inputs_dirty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := legacyMaintenanceFixture(t, 4)
			if tc.dirty {
				dirtyMigrationInput(t, root)
			}

			args := append([]string{"upgrade", "--project-only", "--json"}, tc.args...)
			out, _, code := execCLI(t, args...)

			require.Equal(t, tc.code, code, out)
			e := readMaintenanceEnvelope(t, out)
			result := e.Data
			if e.Error != nil {
				result = e.Error.PartialResult
			}
			require.NotNil(t, result, out)
			require.Equal(t, tc.reason, result.Failure.Reason)
			require.Nil(t, result.Project.Actor)
			require.Equal(t, 4, result.Project.SchemaAfter)
		})
	}
}

// TestCompletedMigrationStillNamesItsActor guards the success path the refusal
// fix shares: a real migration that completes reports who ran it.
func TestCompletedMigrationStillNamesItsActor(t *testing.T) {
	legacyMaintenanceFixture(t, 4)

	out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	project := readMaintenanceEnvelope(t, out).Data.Project
	require.Equal(t, "migrated", project.Migration.Status)
	require.Equal(t, &actor{ID: "jc", Source: contributorSourceFlag}, project.Actor)
}

// TestInspectionWithoutAContributorNamesNoActor covers the read-only protocol:
// inspecting a legacy checkout needs no contributor, and with none to validate
// the plan reports no actor at all, not an empty one with a source.
func TestInspectionWithoutAContributorNamesNoActor(t *testing.T) {
	legacyMaintenanceFixture(t, 4)

	out, _, code := execCLI(t, "upgrade", "--project-only", "--inspect", "--json")

	require.Equal(t, 0, code, out)
	project := readMaintenanceEnvelope(t, out).Data.Project
	require.Equal(t, "planned", project.Migration.Status)
	require.Nil(t, project.Actor)
	require.NotContains(t, out, `"actor"`)
}

// TestArtifactOnlyRefreshStaysIdentityFree confirms a current-schema refresh
// reports no actor even when every identity source names a registered
// contributor: no migration ran, so nobody acted.
func TestArtifactOnlyRefreshStaysIdentityFree(t *testing.T) {
	root := maintenanceFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "identity.yml"), []byte("contributor: jc\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "orientation.md"), []byte("stale\n"), 0o644))

	out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.True(t, e.Data.Complete)
	require.Equal(t, "current", e.Data.Project.Migration.Status)
	require.Equal(t, "refreshed", e.Data.Project.Artifacts.Status)
	require.Nil(t, e.Data.Project.Actor)
}

// TestBinaryOnlyAndOutsideProjectUpgradesStayIdentityFree confirms an upgrade
// that touches no checkout reports no actor, even when a contributor is named
// on the command line and in the environment.
func TestBinaryOnlyAndOutsideProjectUpgradesStayIdentityFree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		status string
	}{
		{"binary only", []string{"--binary-only"}, "excluded"},
		{"outside a project", nil, "not_applicable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			target := upgradeTarget(t, t.TempDir())
			server, _ := newUpgradeServer(t, "/repos/madeintandem/tiller/releases/tags/v0.1.0", "v0.1.0", versionScript("0.1.0"), false)
			setUpgradeSeams(t, server, target)
			t.Setenv("TILLER_CONTRIBUTOR", "jc")

			args := append(append([]string{"upgrade"}, tc.args...), "--version", "0.1.0", "--contributor", "jc", "--json")
			out, _, code := execCLI(t, args...)

			require.Equal(t, 0, code, out)
			e := readMaintenanceEnvelope(t, out)
			require.True(t, e.Data.Complete)
			require.Equal(t, tc.status, e.Data.Project.Status)
			require.Nil(t, e.Data.Project.Actor)
		})
	}
}

// TestUpgradeHandoffPreservesTheChildRefusalAndItsActor runs a combined upgrade
// against a real compiled candidate. The candidate's inspection validates the
// contributor and refuses the dirty input with exit 1; the parent must carry
// that nonzero result through, actor included, and leave the binary alone.
func TestUpgradeHandoffPreservesTheChildRefusalAndItsActor(t *testing.T) {
	body := buildTillerCandidate(t)
	root := legacyMaintenanceFixture(t, 4)
	dirty := dirtyMigrationInput(t, root)
	target := upgradeTarget(t, t.TempDir())
	installed := readFile(t, target)
	server, _ := newUpgradeServer(t, "/repos/madeintandem/tiller/releases/tags/v"+version, "v"+version, body, false)
	setUpgradeSeams(t, server, target)

	out, _, code := execCLI(t, "upgrade", "--version", version, "--contributor", "jc", "--json")

	require.Equal(t, 1, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.NotNil(t, e.Data)
	require.False(t, e.Data.Complete)
	require.Equal(t, "migration_inputs_dirty", e.Data.Failure.Reason)
	require.Equal(t, "failed", e.Data.Project.Migration.Status)
	require.Equal(t, &actor{ID: "jc", Source: contributorSourceFlag}, e.Data.Project.Actor)
	require.Equal(t, 4, e.Data.Project.SchemaAfter)
	require.Equal(t, dirty, readFile(t, root, ".tiller", "tiller.yml"))
	require.Equal(t, installed, readFile(t, target), "a refused inspection never replaces the binary")
}
