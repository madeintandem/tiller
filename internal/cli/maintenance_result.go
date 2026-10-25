package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/spf13/cobra"
)

const maintenanceProtocol = 1

// Each phase describes its own durable boundary, not the whole operation.
type maintenancePhase struct {
	Status  string              `json:"status"`
	Files   []string            `json:"files,omitempty"`
	Failure *maintenanceFailure `json:"failure,omitempty"`
}

type maintenanceFailure struct {
	Code    int    `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type projectMaintenanceResult struct {
	Root                 string                    `json:"root,omitempty"`
	Status               string                    `json:"status"`
	SchemaBefore         int                       `json:"schema_before,omitempty"`
	SchemaAfter          int                       `json:"schema_after,omitempty"`
	OrientationBefore    int                       `json:"orientation_before,omitempty"`
	OrientationAfter     int                       `json:"orientation_after,omitempty"`
	Migration            maintenancePhase          `json:"migration"`
	Artifacts            maintenancePhase          `json:"artifacts"`
	PlannedSourceFiles   []string                  `json:"planned_source_files,omitempty"`
	PlannedArtifactFiles []string                  `json:"planned_artifact_files,omitempty"`
	Notes                []string                  `json:"notes,omitempty"`
	Blockers             []mutate.MigrationBlocker `json:"blockers,omitempty"`
	Warnings             []string                  `json:"warnings,omitempty"`
	PlannedShimsAdded    []string                  `json:"planned_shims_added,omitempty"`
	PlannedShimsRemoved  []string                  `json:"planned_shims_removed,omitempty"`
	ShimsAdded           []string                  `json:"shims_added,omitempty"`
	ShimsRemoved         []string                  `json:"shims_removed,omitempty"`
	Actor                *actor                    `json:"actor,omitempty"`
}

// upgradeResult is shared by the parent and exact-target child. Legacy fields
// remain binary facts; Complete alone describes the entire requested scope.
type upgradeResult struct {
	Protocol          int                            `json:"protocol"`
	Complete          bool                           `json:"complete"`
	Inspect           bool                           `json:"inspect,omitempty"`
	Outcome           string                         `json:"outcome"`
	From              string                         `json:"from"`
	To                string                         `json:"to,omitempty"`
	Tag               string                         `json:"tag,omitempty"`
	Path              string                         `json:"path,omitempty"`
	Refresh           string                         `json:"refresh"`
	RefreshNote       string                         `json:"refresh_note,omitempty"`
	InstallationOwner string                         `json:"installation_owner"`
	Executing         *selfupdate.ExecutableIdentity `json:"executing,omitempty"`
	Retained          *selfupdate.RetainedExecutable `json:"retained,omitempty"`
	Binary            maintenancePhase               `json:"binary"`
	Project           projectMaintenanceResult       `json:"project"`
	Failure           *maintenanceFailure            `json:"failure,omitempty"`
}

func maintenanceError(reason, message string) error {
	return &maintenanceFailureError{reason: reason, message: message}
}

type maintenanceFailureError struct{ reason, message string }

func (e *maintenanceFailureError) Error() string  { return e.message }
func (e *maintenanceFailureError) ExitCode() int  { return 3 }
func (e *maintenanceFailureError) Reason() string { return e.reason }

func maintenanceFailureBody(err error) *ErrorBody {
	body := classify(err)
	if body.Code == 2 && body.Reason == reasonError {
		body.Code = 3
		body.Reason = "maintenance_failed"
	}
	if body.Reason == "" {
		body.Reason = "maintenance_failed"
	}
	return body
}

func phaseFailure(phase *maintenancePhase, err error) {
	body := maintenanceFailureBody(err)
	phase.Status = "failed"
	if body.Reason == "mutation_recovery_needed" {
		phase.Status = "recovery_required"
	}
	phase.Failure = &maintenanceFailure{Code: body.Code, Reason: body.Reason, Message: body.Message}
}

// Maintenance bypasses automatic --contributor attribution: refresh and binary-only
// operations do not resolve identity. Only the migration stage supplies Actor.
func emitMaintenance(cmd *cobra.Command, command string, result upgradeResult, err error, data any) error {
	if data == nil {
		data = result
	}
	if err == nil {
		return writeReport(cmd, command, true, data, renderMaintenanceHuman(result), 0)
	}
	body := maintenanceFailureBody(err)
	result.Complete = false
	result.Failure = &maintenanceFailure{Code: body.Code, Reason: body.Reason, Message: body.Message}
	if command == "upgrade" {
		data = result
	}
	if body.Code == 1 {
		return writeReport(cmd, command, false, data, renderMaintenanceHuman(result), 1)
	}
	body.PartialResult = &result
	if jsonOut(cmd) {
		_ = writeEnvelope(cmd.OutOrStdout(), Envelope{OK: false, Command: command, Error: body})
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), renderMaintenanceHuman(result))
	}
	return &exitError{code: body.Code, msg: err.Error()}
}

func renderMaintenanceHuman(r upgradeResult) string {
	var b strings.Builder
	if r.Inspect && r.Failure == nil {
		b.WriteString("Maintenance inspection complete; no files changed.\n")
	} else if r.Complete {
		b.WriteString("Maintenance complete for the requested scope.\n")
	} else {
		b.WriteString("Maintenance incomplete.\n")
	}
	fmt.Fprintf(&b, "Binary: %s", r.Binary.Status)
	if r.To != "" {
		fmt.Fprintf(&b, " (%s)", r.To)
	}
	b.WriteByte('\n')
	if r.Outcome == "upgraded" && r.From != "" {
		fmt.Fprintf(&b, "Installed version: %s → %s\n", r.From, r.To)
	}
	if r.Tag != "" {
		fmt.Fprintf(&b, "Selected release: %s\n", r.Tag)
	}
	fmt.Fprintf(&b, "Installation owner: %s\n", r.InstallationOwner)
	if r.Path != "" {
		fmt.Fprintf(&b, "Installed executable: %s\n", r.Path)
	}
	if r.Project.Root != "" {
		fmt.Fprintf(&b, "Checkout: %s\nMigration: %s (schema %s → %s)\nArtifacts: %s (generation %s → %s)\n", r.Project.Root, r.Project.Migration.Status, maintenanceObservation(r.Project.SchemaBefore), maintenanceObservation(r.Project.SchemaAfter), r.Project.Artifacts.Status, maintenanceObservation(r.Project.OrientationBefore), maintenanceObservation(r.Project.OrientationAfter))
	} else {
		fmt.Fprintf(&b, "Project: %s. Other checkouts were not touched.\n", r.Project.Status)
	}
	if r.Retained != nil {
		fmt.Fprintf(&b, "Retained executable: %s\nRetained directory: %s\n", r.Retained.Path, r.Retained.Directory)
	}
	for _, blocker := range r.Project.Blockers {
		fmt.Fprintf(&b, "Drain blocker: %s (%s), status %s\n", blocker.Ref, blocker.Path, blocker.Status)
	}
	for _, warning := range r.Project.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", warning)
	}
	writeFiles := func(files []string) {
		for _, file := range files {
			if strings.HasPrefix(file, ".claude/") || file == localIdentityPath {
				fmt.Fprintf(&b, "Local artifact: %s\n", file)
			} else {
				fmt.Fprintf(&b, "Review/commit: %s\n", file)
			}
		}
	}
	writeFiles(r.Project.Migration.Files)
	writeFiles(r.Project.Artifacts.Files)
	if r.Failure != nil {
		fmt.Fprintf(&b, "Failure [%s]: %s\n", r.Failure.Reason, r.Failure.Message)
		if r.Project.Migration.Status == "recovery_required" || r.Project.Artifacts.Status == "recovery_required" {
			b.WriteString("Resolve the retained mutation recovery journal before retrying maintenance.\n")
		} else if r.Project.Root != "" {
			b.WriteString("After resolving the named cause, run tiller upgrade --project-only for this checkout.\n")
		} else {
			b.WriteString("Resolve the installation failure and retry tiller upgrade with the same scope.\n")
		}
	}
	if len(r.Project.Blockers) > 0 {
		b.WriteString("Finish authorized legacy work with a compatible release, have the human resolve final acceptance/merges, and run sync. Do not use an older binary on an already-migrated checkout. A retained binary is not proof of schema compatibility. Once compatibility is verified, prepend its directory to PATH for the entire drain session, including child commands; leave that session and retry the upgraded executable explicitly.\n")
		b.WriteString("If no verified compatible copy is available, obtain a compatible release before draining.\n")
	}
	if r.Complete && !r.Inspect {
		b.WriteString("Restart existing tiller serve processes and reload harness instructions as needed; no running process was restarted. Each other checkout runs tiller upgrade --project-only after pulling; it refreshes that checkout's local artifacts and renames a legacy owner key in its gitignored identity file.\n")
	}
	return strings.TrimSpace(b.String())
}

func maintenanceObservation(value int) string {
	if value == 0 {
		return "unknown"
	}
	return fmt.Sprint(value)
}
