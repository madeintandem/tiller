package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func activeLegacyMaintenanceFixture(t *testing.T) string {
	t.Helper()
	root := legacyMaintenanceFixture(t, 5)
	path := filepath.Join(root, ".tiller/bugs/active/bug.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("id: B-aaaaaa\nkey: bugs/active\nstatus: in_review\nowner: jc\nbranch: tiller/bugs/active\npr: 42\n"), 0o644))
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "active schema five work")
	return root
}

func TestMigrateDryRunLeavesSourceCacheIdentityAndArtifactsUntouched(t *testing.T) {
	root := activeLegacyMaintenanceFixture(t)
	beforeFiles, beforeDirs := snapshotWholeTree(t, root)
	out, _, code := execCLI(t, "migrate", "--dry-run", "--json")
	require.Equal(t, 0, code, out)
	var report struct {
		OK   bool                       `json:"ok"`
		Data mutate.MigrationInspection `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.True(t, report.OK)
	require.Equal(t, []mutate.OwnershipAssignment{{ID: "B-aaaaaa", Noun: "bug", Ref: "bugs/active", ObservedOwner: "jc"}}, report.Data.OwnershipReview.Active)
	afterFiles, afterDirs := snapshotWholeTree(t, root)
	require.Equal(t, beforeFiles, afterFiles)
	require.Equal(t, beforeDirs, afterDirs)
}

func TestMaintenanceConsumesReviewedDryRunEnvelopeAndPreservesReview(t *testing.T) {
	for _, command := range [][]string{{"migrate"}, {"upgrade", "--project-only"}} {
		t.Run(command[0], func(t *testing.T) {
			root := activeLegacyMaintenanceFixture(t)
			out, _, code := execCLI(t, "migrate", "--dry-run", "--json")
			require.Equal(t, 0, code, out)
			var report struct {
				OK      bool                       `json:"ok"`
				Command string                     `json:"command"`
				Data    mutate.MigrationInspection `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &report))
			report.Data.OwnershipReview.Active[0].ConfirmedOwner = "jc"
			reviewPath := filepath.Join(t.TempDir(), "review.json")
			reviewData, err := json.Marshal(report)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(reviewPath, reviewData, 0o600))
			args := append(append([]string(nil), command...), "--ownership-review", reviewPath, "--contributor", "jc", "--json")
			out, _, code = execCLI(t, args...)
			require.Equal(t, 0, code, out)
			tree, err := store.Load(root)
			require.NoError(t, err)
			require.Equal(t, store.CurrentSchemaVersion, tree.Manifest.SchemaVersion)
			require.True(t, tree.Bugs["B-aaaaaa"].Started)
			require.Equal(t, "jc", tree.Bugs["B-aaaaaa"].Owner)
			require.Equal(t, "in_review", tree.Bugs["B-aaaaaa"].Status)
			require.Equal(t, 42, *tree.Bugs["B-aaaaaa"].PR)
			require.Equal(t, "tiller/bugs/active", tree.Bugs["B-aaaaaa"].Branch)
		})
	}
}

func TestOwnershipReviewPreflightRefusesBeforeMaintenance(t *testing.T) {
	root := activeLegacyMaintenanceFixture(t)
	beforeFiles, beforeDirs := snapshotWholeTree(t, root)
	out, _, code := execCLI(t, "upgrade", "--project-only", "--inspect", "--contributor", "jc", "--json")
	require.Equal(t, 3, code, out)
	require.Equal(t, store.ReasonOwnershipReviewRequired, readMaintenanceEnvelope(t, out).Error.Reason)
	afterFiles, afterDirs := snapshotWholeTree(t, root)
	require.Equal(t, beforeFiles, afterFiles)
	require.Equal(t, beforeDirs, afterDirs)
	out, _, code = execCLI(t, "upgrade", "--ownership-review", "unread.json", "--json")
	require.Equal(t, 2, code, out)
	require.Equal(t, "invalid_upgrade_scope", readMaintenanceEnvelope(t, out).Error.Reason)
}
