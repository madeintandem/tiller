package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// Discovery must distinguish an absent project from an unreadable or malformed
// marker. In particular, it never skips a broken nested project for its parent.
func maintenanceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", maintenanceError("project_discovery_failed", err.Error())
	}
	for {
		marker := filepath.Join(dir, ".tiller")
		info, err := os.Lstat(marker)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", maintenanceError("project_marker_invalid", fmt.Sprintf("%s must be a real directory", marker))
			}
			path := filepath.Join(marker, "tiller.yml")
			_, snapshot, err := store.ReadRegularFileSnapshot(dir, path)
			if err != nil {
				return "", maintenanceError(store.ReasonProjectMarkerUnreadable, err.Error())
			}
			if !snapshot.Exists() {
				return "", maintenanceError("project_marker_invalid", fmt.Sprintf("%s is missing", path))
			}
			return dir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", maintenanceError("project_discovery_failed", err.Error())
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func executingIdentity() (*selfupdate.ExecutableIdentity, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	digest, err := executableDigest(path)
	if err != nil {
		return nil, err
	}
	return &selfupdate.ExecutableIdentity{Path: path, Version: version, SHA256: digest}, nil
}
func executableDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newMaintenanceResult(root string, inspect bool) upgradeResult {
	p := projectMaintenanceResult{Root: root, Status: "pending", Migration: maintenancePhase{Status: "pending"}, Artifacts: maintenancePhase{Status: "pending"}}
	return upgradeResult{Protocol: maintenanceProtocol, Inspect: inspect, Outcome: "project_only", From: version, To: version, Refresh: "skipped", InstallationOwner: "not_applicable", Binary: maintenancePhase{Status: "excluded"}, Project: p}
}

func localMaintenance(cmd *cobra.Command, root, command string, inspect bool, expectedDigest string) (upgradeResult, error) {
	r := newMaintenanceResult(root, inspect)
	identity, err := executingIdentity()
	if err != nil {
		return r, maintenanceError("executable_unresolved", err.Error())
	}
	r.Executing = identity
	checkExecutable := func() error {
		digest, err := executableDigest(identity.Path)
		if err != nil {
			return err
		}
		if digest != identity.SHA256 || (expectedDigest != "" && digest != expectedDigest) {
			return maintenanceError("executable_changed", "selected executable changed before project maintenance; inspect the installation and retry")
		}
		return nil
	}
	if err = checkExecutable(); err != nil {
		return r, err
	}
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, _, err := store.ReadRegularFileSnapshot(root, path)
	if err != nil {
		return r, err
	}
	schema, refusal := store.SchemaVersionOf(path, data)
	if refusal != nil {
		return r, refusal
	}
	r.Project.SchemaBefore = schema
	r.Project.SchemaAfter = schema
	var d mutate.Deps
	var a actor
	if schema < store.CurrentSchemaVersion && schema >= store.MinMigratableSchemaVersion {
		id, source, resolveErr := maintenanceActor(cmd, root)
		if resolveErr != nil {
			phaseFailure(&r.Project.Migration, resolveErr)
			return r, resolveErr
		}
		d.Contributor = id
		a = actor{ID: id, Source: source}
	}
	review, err := migrationReview(cmd)
	if err != nil {
		phaseFailure(&r.Project.Migration, err)
		return r, err
	}
	migration, err := mutate.InspectMigration(root, d, review)
	r.Project.PlannedSourceFiles = migration.Files
	r.Project.Notes = migration.Notes
	r.Project.Blockers = migration.Blockers
	if err != nil {
		phaseFailure(&r.Project.Migration, err)
		return r, err
	}
	manifest, err := store.ParseManifestUnchecked(root, data)
	if err == nil {
		err = mutate.RequireOwnershipReview(migration.OwnershipReview, review, manifest)
	}
	if err != nil {
		phaseFailure(&r.Project.Migration, err)
		return r, err
	}
	if schema < store.CurrentSchemaVersion {
		r.Project.Actor = &a
		r.Project.Migration.Status = "planned"
	} else {
		r.Project.Migration.Status = "current"
	}
	identityRename, err := legacyIdentityRename(root, schema, manifest)
	if err != nil {
		phaseFailure(&r.Project.Migration, err)
		return r, err
	}
	if identityRename != nil {
		r.Project.PlannedSourceFiles = append(r.Project.PlannedSourceFiles, localIdentityPath)
		r.Project.Notes = append(r.Project.Notes, "rename the legacy owner key to contributor in this checkout's gitignored "+localIdentityPath)
	}
	refresh, err := refreshProject(root, true)
	r.Project.OrientationBefore = refresh.Before
	r.Project.OrientationAfter = refresh.Before
	r.Project.PlannedArtifactFiles = refresh.Files
	r.Project.Warnings = refresh.Warnings
	r.Project.PlannedShimsAdded = refresh.ShimsAdded
	r.Project.PlannedShimsRemoved = refresh.ShimsRemoved
	if err != nil {
		phaseFailure(&r.Project.Artifacts, err)
		return r, err
	}
	warnings, warnErr := mutate.MigrationExecutionWarnings(root)
	if warnErr != nil {
		r.Project.Warnings = append(r.Project.Warnings, "Local execution-ref inspection unavailable: "+warnErr.Error())
	} else {
		r.Project.Warnings = append(r.Project.Warnings, warnings...)
	}
	r.Project.Artifacts.Status = "planned"
	if inspect {
		r.Project.Status = "inspected"
		r.Complete = true
		return r, nil
	}
	if err = checkExecutable(); err != nil {
		return r, err
	}
	migrated, err := mutate.MigrateWithCommand(root, d, command, review)
	if err != nil {
		r.Project.Blockers = migrated.Blockers
		phaseFailure(&r.Project.Migration, err)
		r.Project.Status = r.Project.Migration.Status
		if r.Project.Migration.Status == "recovery_required" {
			r.Project.SchemaAfter = 0
		}
		return r, err
	}
	r.Project.SchemaAfter = migrated.To
	r.Project.Migration.Files = migrated.Files
	r.Project.Migration.Status = string(migrated.Outcome)
	if identityRename != nil {
		if err = store.WriteRepositoryFileAtomic(root, localIdentityPath, identityRename); err != nil {
			phaseFailure(&r.Project.Migration, err)
			r.Project.Status = r.Project.Migration.Status
			return r, err
		}
		r.Project.Migration.Files = append(r.Project.Migration.Files, localIdentityPath)
	}
	if err = checkExecutable(); err != nil {
		phaseFailure(&r.Project.Artifacts, err)
		return r, err
	}
	refresh, err = refreshProject(root, false)
	if err != nil {
		phaseFailure(&r.Project.Artifacts, err)
		r.Project.Status = r.Project.Artifacts.Status
		r.Refresh = "failed"
		r.RefreshNote = err.Error()
		if r.Project.Artifacts.Status == "recovery_required" {
			r.Project.OrientationAfter = 0
		}
		return r, err
	}
	r.Project.Artifacts = maintenancePhase{Status: "refreshed", Files: refresh.Files}
	if len(refresh.Files) == 0 {
		r.Project.Artifacts.Status = "current"
	}
	r.Project.OrientationAfter = refresh.After
	r.Project.ShimsAdded = refresh.ShimsAdded
	r.Project.ShimsRemoved = refresh.ShimsRemoved
	r.Project.Status = "complete"
	r.Refresh = "ran"
	r.RefreshNote = fmt.Sprintf("generation %d", refresh.After)
	r.Complete = true
	return r, nil
}

func maintenanceActor(cmd *cobra.Command, root string) (string, string, error) {
	flag, _ := cmd.Flags().GetString("contributor")
	if id := strings.TrimSpace(flag); id != "" {
		return id, contributorSourceFlag, nil
	}
	id, source, err := resolveMigrationContributorFallback(root)
	if err != nil {
		return "", "", identityUnreadableError(err)
	}
	return id, source, nil
}

const localIdentityPath = ".tiller/identity.yml"

// legacyIdentityRename returns the renamed identity file when a current-schema
// checkout still spells its id with the pre-schema-9 owner key. The file is
// gitignored, so the checkout that migrated could not rename it here. The id
// must be on the roster, exactly as migration requires of its actor.
func legacyIdentityRename(root string, schema int, manifest *store.Manifest) ([]byte, error) {
	if schema != store.CurrentSchemaVersion {
		return nil, nil
	}
	id, renamed, err := store.LegacyLocalIdentity(root)
	if err != nil {
		return nil, identityUnreadableError(err)
	}
	if renamed == nil {
		return nil, nil
	}
	if _, err := store.ResolveContributor(manifest, id); err != nil {
		var refusal *store.EnvError
		if errors.As(err, &refusal) && refusal.Reason == store.ReasonContributorUnknown {
			return nil, &store.EnvError{Code: refusal.Code, Reason: refusal.Reason, Msg: fmt.Sprintf(
				"%s names contributor %q, who is not in the project contributors: "+
					"run `tiller join --as <id>` to select a listed contributor, or `tiller add contributor %s --name <name>` to register them",
				localIdentityPath, id, id)}
		}
		return nil, err
	}
	return renamed, nil
}

// Migration alone may read a checkout identity written by an older schema.
// The legacy spelling never becomes a fallback for ordinary commands.
func resolveMigrationContributorFallback(root string) (id, source string, err error) {
	if v := strings.TrimSpace(os.Getenv("TILLER_CONTRIBUTOR")); v != "" {
		return v, "TILLER_CONTRIBUTOR", nil
	}
	id, err = store.ReadLocalIdentityForMigration(root)
	return id, ".tiller/identity.yml", err
}
