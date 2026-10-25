package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

var upgradeVersionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

const maintenanceTimeout = 60 * time.Second

type upgradeOptions struct {
	version, contributor, expectedDigest string
	projectOnly, binaryOnly, inspect     bool
	handoff                              bool
}

func newUpgradeCmd() *cobra.Command {
	var o upgradeOptions
	cmd := &cobra.Command{Use: "upgrade", Short: "Update Tiller and maintain the current checkout.", Long: "Upgrade the installed binary through its installation owner, then migrate and\nrefresh only the current checkout. --project-only performs offline maintenance\nwith this binary; --binary-only excludes project writes. Join remains identity\nselection. Review and commit changed project files; restart existing serve and\nharness sessions yourself. Other checkouts are never discovered or changed.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return runUnifiedUpgrade(cmd, o) }}
	cmd.Flags().StringVar(&o.version, "version", "", "select an exact self-managed release (vX.Y.Z)")
	cmd.Flags().BoolVar(&o.projectOnly, "project-only", false, "maintain this checkout with the running binary, without network access")
	cmd.Flags().BoolVar(&o.binaryOnly, "binary-only", false, "update only the installed binary")
	cmd.Flags().StringVar(&o.contributor, "contributor", "", "registered actor for source migration; does not change checkout identity")
	cmd.Flags().BoolVar(&o.inspect, "inspect", false, "inspect project maintenance without writes")
	cmd.Flags().String("ownership-review", "", "reviewed schema-6 ownership mapping; requires --project-only")
	cmd.Flags().StringVar(&o.expectedDigest, "expected-executable-sha256", "", "guard the selected executable during handoff")
	cmd.Flags().BoolVar(&o.handoff, "upgrade-handoff", false, "preserve the parent upgrade command's migration provenance")
	_ = cmd.Flags().MarkHidden("upgrade-handoff")
	_ = cmd.Flags().MarkHidden("inspect")
	_ = cmd.Flags().MarkHidden("expected-executable-sha256")
	return cmd
}

func runUnifiedUpgrade(cmd *cobra.Command, o upgradeOptions) error {
	pinned, tag, err := parseUpgradeVersionFlag(o.version)
	if err != nil {
		return emitFailure(cmd, "upgrade", err)
	}
	if (o.projectOnly && o.binaryOnly) || (o.projectOnly && cmd.Flags().Changed("version")) || (o.inspect && (!o.projectOnly || !jsonOut(cmd))) || (o.expectedDigest != "" && !o.projectOnly) || (o.handoff && (!o.projectOnly || !jsonOut(cmd))) {
		return emitFailure(cmd, "upgrade", &usageError{reason: "invalid_upgrade_scope", msg: "--project-only excludes --binary-only and --version; --inspect requires --project-only --json"})
	}
	if cmd.Flags().Changed("ownership-review") && !o.projectOnly {
		return emitFailure(cmd, "upgrade", &usageError{reason: "invalid_upgrade_scope", msg: "--ownership-review requires --project-only; inspect with tiller migrate --dry-run --json before offline project maintenance"})
	}
	root := ""
	if !o.binaryOnly {
		root, err = maintenanceRoot()
		if err != nil {
			return emitFailure(cmd, "upgrade", err)
		}
	}
	if o.projectOnly {
		if root == "" {
			return emitFailure(cmd, "upgrade", &store.EnvError{Code: 3, Reason: store.ReasonNotARepo, Msg: "--project-only requires a Tiller checkout"})
		}
		command := "upgrade --project-only"
		if o.handoff {
			command = "upgrade"
		}
		result, err := localMaintenance(cmd, root, command, o.inspect, o.expectedDigest)
		return emitMaintenance(cmd, "upgrade", result, err, nil)
	}
	r := newMaintenanceResult(root, false)
	r.Outcome = "pending"
	r.To = ""
	r.InstallationOwner = "self_managed"
	r.Binary.Status = "pending"
	if root == "" {
		r.Project.Status = "not_applicable"
		if o.binaryOnly {
			r.Project.Status = "excluded"
		}
		r.Project.Migration.Status = r.Project.Status
		r.Project.Artifacts.Status = r.Project.Status
	}
	fail := func(err error) error {
		if r.Binary.Status == "pending" {
			phaseFailure(&r.Binary, err)
		}
		return emitMaintenance(cmd, "upgrade", r, err, nil)
	}
	target, err := selfupdate.ExecutablePath()
	if err != nil {
		return fail(maintenanceError("executable_unresolved", err.Error()))
	}
	r.Path = target
	ctx := cmd.Context()
	if pinned && selfupdate.HomebrewManaged(target) {
		return fail(&usageError{reason: "homebrew_version_unsupported", msg: "exact --version selection is unsupported for Homebrew installations"})
	}
	brew, err := selfupdate.DetectHomebrew(ctx, target)
	if err != nil {
		return fail(maintenanceError("installation_owner_unverified", err.Error()))
	}
	if brew != nil {
		r.InstallationOwner = "homebrew_" + brew.Kind
		if pinned {
			return fail(&usageError{reason: "homebrew_version_unsupported", msg: "exact --version selection is unsupported for Homebrew installations"})
		}
		return runHomebrewUpgrade(cmd, o, root, target, *brew, r)
	}
	outgoing, err := selfupdate.ReadExecutable(ctx, target)
	if err != nil {
		return fail(maintenanceError("outgoing_identity_invalid", err.Error()))
	}
	r.From = outgoing.Version
	if outgoing.Version != version {
		return fail(maintenanceError("executable_changed", "the installed version differs from the running process; restart the installed Tiller command before upgrading"))
	}
	client := selfupdate.NewClient()
	var release selfupdate.Release
	if pinned {
		release, err = client.ReleaseByTag(ctx, tag)
	} else {
		release, err = client.Latest(ctx)
	}
	if err != nil {
		return fail(envUpgradeNetworkError("release_lookup_failed", err))
	}
	r.Tag = release.Tag
	to := strings.TrimPrefix(release.Tag, "v")
	toVersion, err := parseUpgradeVersion(to)
	if err != nil {
		return fail(maintenanceError("release_version_unparseable", err.Error()))
	}
	fromVersion, err := parseUpgradeVersion(version)
	if err != nil {
		return fail(maintenanceError("running_version_unparseable", err.Error()))
	}
	if !pinned && compareUpgradeVersions(toVersion, fromVersion) <= 0 {
		r.Outcome = "already_current"
		r.To = version
		r.Binary.Status = "already_current"
		if err = selfupdate.CheckExecutable(outgoing); err != nil {
			return fail(maintenanceError("executable_changed", err.Error()))
		}
		if root != "" {
			local, err := localMaintenance(cmd, root, "upgrade", false, "")
			mergeProjectMaintenance(&r, local)
			if err != nil {
				return fail(err)
			}
		}
		r.Complete = true
		return emitMaintenance(cmd, "upgrade", r, nil, nil)
	}
	r.To = to
	scratch, err := os.MkdirTemp("", "tiller-upgrade-*")
	if err != nil {
		return fail(maintenanceError("scratch_dir_failed", err.Error()))
	}
	defer os.RemoveAll(scratch)
	asset := selfupdate.AssetName(release.Tag, runtime.GOOS, runtime.GOARCH)
	archive := filepath.Join(scratch, asset)
	checksums := filepath.Join(scratch, "checksums.txt")
	if err = client.FetchAsset(ctx, release, asset, archive); err != nil {
		return fail(envUpgradeNetworkError("asset_download_failed", err))
	}
	if err = client.FetchAsset(ctx, release, "checksums.txt", checksums); err != nil {
		return fail(envUpgradeNetworkError("asset_download_failed", err))
	}
	if err = selfupdate.VerifyChecksum(archive, checksums, asset); err != nil {
		return fail(maintenanceError("checksum_mismatch", err.Error()))
	}
	candidate, err := selfupdate.ExtractBinary(archive, scratch)
	if err != nil {
		return fail(maintenanceError("extract_failed", err.Error()))
	}
	selected, err := selfupdate.ReadExecutable(ctx, candidate)
	if err != nil {
		return fail(maintenanceError("target_identity_invalid", err.Error()))
	}
	if selected.Version != to {
		return fail(maintenanceError("target_version_mismatch", fmt.Sprintf("release %s contains executable %s", to, selected.Version)))
	}
	if root != "" {
		inspected, err := runMaintenanceChild(ctx, selected, root, o.contributor, true)
		mergeProjectMaintenance(&r, inspected)
		if err != nil {
			return fail(err)
		}
	}
	retained, err := selfupdate.RetainExecutable(ctx, outgoing)
	if retained.Path != "" {
		r.Retained = &retained
	}
	if err != nil {
		return fail(maintenanceError("retention_failed", err.Error()))
	}
	if err = selfupdate.CheckExecutable(outgoing); err != nil {
		return fail(maintenanceError("executable_changed", err.Error()))
	}
	if err = selfupdate.CheckExecutable(selected); err != nil {
		return fail(maintenanceError("executable_changed", err.Error()))
	}
	if err = selfupdate.SwapBinary(candidate, target); err != nil {
		return fail(maintenanceError("swap_failed", err.Error()))
	}
	r.Outcome = "upgraded"
	r.Binary.Status = "upgraded"
	installed, err := selfupdate.ReadExecutable(ctx, target)
	if err != nil {
		return fail(maintenanceError("installed_identity_unknown", err.Error()))
	}
	if installed.Version != selected.Version || installed.SHA256 != selected.SHA256 {
		return fail(maintenanceError("executable_changed", "installed executable differs from the verified candidate"))
	}
	if root != "" {
		applied, err := runMaintenanceChild(ctx, installed, root, o.contributor, false)
		mergeProjectMaintenance(&r, applied)
		if err != nil {
			return fail(err)
		}
	}
	r.Complete = true
	return emitMaintenance(cmd, "upgrade", r, nil, nil)
}

func mergeProjectMaintenance(parent *upgradeResult, child upgradeResult) {
	parent.Project = child.Project
	parent.Refresh = child.Refresh
	parent.RefreshNote = child.RefreshNote
	parent.Executing = child.Executing
}

func runHomebrewUpgrade(cmd *cobra.Command, o upgradeOptions, root, target string, brew selfupdate.HomebrewInstall, r upgradeResult) error {
	const ownershipBoundary = "Homebrew owns metadata refresh and required dependencies; installed-dependent checks and automatic cleanup are disabled for this invocation."
	fail := func(err error) error {
		r.Project.Warnings = append(r.Project.Warnings, ownershipBoundary)
		return emitMaintenance(cmd, "upgrade", r, err, nil)
	}
	ctx := cmd.Context()
	// Current rules can reject known blockers, but cannot predict the next brew
	// release. That external installation boundary is deliberately not atomic.
	if root != "" {
		inspection, err := localMaintenance(cmd, root, "upgrade", true, "")
		mergeProjectMaintenance(&r, inspection)
		if err != nil {
			reason := classify(err).Reason
			newerSchema := reason == store.ReasonSchemaVersionUnsupported && inspection.Project.SchemaBefore > store.CurrentSchemaVersion
			if !newerSchema && reason != "orientation_newer" {
				return fail(err)
			}
			// Only the installed target can inspect a checkout ahead of this code.
			// All known drain, attribution, dirty-input and recovery holds still stop.
			r.Project.Status = "pending"
		}
	}
	outgoing, err := selfupdate.ReadExecutable(ctx, target)
	if err != nil {
		return fail(maintenanceError("outgoing_identity_invalid", err.Error()))
	}
	r.From = outgoing.Version
	retained, err := selfupdate.RetainExecutable(ctx, outgoing)
	if retained.Path != "" {
		r.Retained = &retained
	}
	if err != nil {
		return fail(maintenanceError("retention_failed", err.Error()))
	}
	if err = selfupdate.CheckExecutable(outgoing); err != nil {
		return fail(maintenanceError("executable_changed", err.Error()))
	}
	resultPath, err := selfupdate.UpgradeHomebrew(ctx, &brew)
	if err != nil {
		r.Binary.Status = "unknown"
		return fail(maintenanceError("homebrew_upgrade_failed", err.Error()))
	}
	r.Path = resultPath
	installed, err := selfupdate.ReadExecutable(ctx, resultPath)
	if err != nil {
		r.Binary.Status = "unknown"
		return fail(maintenanceError("installed_identity_unknown", err.Error()))
	}
	r.To = installed.Version
	r.Binary.Status = "upgraded"
	r.Outcome = "upgraded"
	if installed.SHA256 == outgoing.SHA256 {
		r.Binary.Status = "already_current"
		r.Outcome = "already_current"
	}
	if root != "" {
		inspected, err := runMaintenanceChild(ctx, installed, root, o.contributor, true)
		mergeProjectMaintenance(&r, inspected)
		if err != nil {
			return fail(err)
		}
		applied, err := runMaintenanceChild(ctx, installed, root, o.contributor, false)
		mergeProjectMaintenance(&r, applied)
		if err != nil {
			return fail(err)
		}
	}
	r.Complete = true
	r.Project.Warnings = append(r.Project.Warnings, ownershipBoundary)
	return emitMaintenance(cmd, "upgrade", r, nil, nil)
}

func parseUpgradeVersionFlag(flag string) (bool, string, error) {
	v := strings.TrimSpace(flag)
	if v == "" {
		return false, "", nil
	}
	if !upgradeVersionPattern.MatchString(v) {
		return false, "", &usageError{reason: "invalid_version", msg: fmt.Sprintf("--version %q is not a version: want vX.Y.Z", v)}
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return true, v, nil
}
func parseUpgradeVersion(v string) ([3]int, error) {
	var out [3]int
	if !upgradeVersionPattern.MatchString(v) {
		return out, fmt.Errorf("version %q is not X.Y.Z", v)
	}
	for i, p := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, err
		}
		out[i] = n
	}
	return out, nil
}
func compareUpgradeVersions(a, b [3]int) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
func envUpgradeNetworkError(reason string, err error) error {
	return &store.EnvError{Code: 3, Reason: reason, Msg: fmt.Sprintf("%v (private releases require GITHUB_TOKEN; use tiller upgrade --project-only for offline checkout maintenance)", err)}
}
