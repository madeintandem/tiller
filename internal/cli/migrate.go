package cli

import (
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "migrate", Short: "Migrate this checkout and refresh its generated instructions.", Long: "Apply registered forward schema migrations, then refresh orientation,\nskill shims and harness wiring. Current-schema refresh needs no identity;\nsource migration requires a registered actor. Review and commit changed files.\nThis command never changes the installed binary or checkout identity.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun && !jsonOut(cmd) {
			return emitFailure(cmd, "migrate", &usageError{reason: "invalid_migrate_scope", msg: "--dry-run requires --json; it reports ownership review without writing any project files"})
		}
		root, err := maintenanceRoot()
		if err != nil {
			return emitFailure(cmd, "migrate", err)
		}
		if root == "" {
			return emitFailure(cmd, "migrate", &store.EnvError{Code: 3, Reason: store.ReasonNotARepo, Msg: "run migrate inside a Tiller checkout"})
		}
		if dryRun {
			review, err := migrationReview(cmd)
			if err != nil {
				return emitFailure(cmd, "migrate", err)
			}
			inspection, err := mutate.InspectMigration(root, mutate.Deps{}, review)
			if err != nil {
				return emitFailure(cmd, "migrate", err)
			}
			return WriteSuccess(cmd.OutOrStdout(), "migrate", inspection)
		}
		result, err := localMaintenance(cmd, root, "migrate", false, "")
		view := migrateView{MigrateResult: mutate.MigrateResult{From: result.Project.SchemaBefore, To: result.Project.SchemaAfter, Outcome: mutate.MigrateOutcome(result.Project.Migration.Status), Files: result.Project.Migration.Files, Notes: result.Project.Notes}, Refresh: result.Refresh, RefreshNote: result.RefreshNote, Complete: result.Complete, Project: result.Project}
		if err != nil {
			body := maintenanceFailureBody(err)
			view.Failure = &maintenanceFailure{Code: body.Code, Reason: body.Reason, Message: body.Message}
		}
		return emitMaintenance(cmd, "migrate", result, err, view)
	}}
	addContributorFlag(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "inspect migration and emit a snapshot-bound ownership review without writes (requires --json)")
	cmd.Flags().String("ownership-review", "", "human-reviewed ownership JSON saved from migrate --dry-run --json")
	return cmd
}

type migrateView struct {
	mutate.MigrateResult
	Refresh     string                   `json:"refresh"`
	RefreshNote string                   `json:"refresh_note,omitempty"`
	Complete    bool                     `json:"complete"`
	Project     projectMaintenanceResult `json:"project"`
	Failure     *maintenanceFailure      `json:"failure,omitempty"`
}
