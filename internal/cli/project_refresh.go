package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
)

// refreshResult describes planned changes in inspection mode and committed
// changes in apply mode. An unrecognized or missing orientation has generation 0.
type refreshResult struct {
	Before       int      `json:"before"`
	After        int      `json:"after"`
	Files        []string `json:"files"`
	ShimsAdded   []string `json:"shims_added"`
	ShimsRemoved []string `json:"shims_removed"`
	Warnings     []string `json:"warnings"`
}

type projectRefreshPlan struct {
	tree   *store.Tree
	writes []store.PlannedWrite
	result refreshResult
}

// refreshProject neither resolves nor writes checkout identity. Inspection may
// read a supported older schema so candidate preflight can validate the complete
// artifact bundle before migration. Apply always requires the current schema.
func refreshProject(root string, inspect bool) (refreshResult, error) {
	plan, err := planProjectRefresh(root, inspect)
	if err != nil {
		result := plan.result
		result.After = result.Before
		result.Files, result.ShimsAdded, result.ShimsRemoved = nil, nil, nil
		return result, err
	}
	if inspect || len(plan.result.Files) == 0 {
		return plan.result, nil
	}
	if err := plan.tree.CommitDerived("refresh project artifacts", plan.writes); err != nil {
		result := plan.result
		result.After = result.Before
		result.Files, result.ShimsAdded, result.ShimsRemoved = nil, nil, nil
		var env *store.EnvError
		if errors.As(err, &env) && env.Reason == store.ReasonMutationRecovery {
			// The transaction journal, not a guessed unchanged observation, owns
			// readback until recovery establishes which destinations are durable.
			result.After = 0
		}
		return result, err
	}
	return plan.result, nil
}

// refuseNewerOrientation is the one downgrade guard every orientation writer
// shares: a checkout whose orientation a newer binary generated refuses with
// orientation_newer rather than being rewritten to this binary's older text.
// An unrecognized or missing orientation is generation 0 and never refuses.
func refuseNewerOrientation(generation int) error {
	if generation <= gen.OrientationGeneration() {
		return nil
	}
	return &store.EnvError{Code: 3, Reason: "orientation_newer", Msg: fmt.Sprintf(
		".tiller/orientation.md is generation %d, newer than this binary's generation %d; upgrade the Tiller binary before refreshing this checkout",
		generation, gen.OrientationGeneration())}
}

func planProjectRefresh(root string, inspect bool) (projectRefreshPlan, error) {
	plan := projectRefreshPlan{}
	// Read the manifest through the same confined path rules as destinations.
	manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
	manifestData, manifestSnapshot, err := store.ReadRegularFileSnapshot(root, manifestPath)
	if err != nil {
		return plan, err
	}
	if !manifestSnapshot.Exists() {
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonNotARepo, Msg: fmt.Sprintf("not a Tiller repo: %s not found", manifestPath)}
	}
	manifest, err := store.ParseManifest(root, manifestData)
	if err != nil && inspect {
		version, versionErr := store.SchemaVersionOf(manifestPath, manifestData)
		if versionErr == nil && version >= store.MinMigratableSchemaVersion && version < store.CurrentSchemaVersion {
			manifest, err = store.ParseManifestUnchecked(root, manifestData)
		}
	}
	if err != nil {
		return plan, err
	}
	plan.tree, err = store.UncheckedTree(root, manifest)
	if err != nil {
		return plan, err
	}
	plan.writes = append(plan.writes, store.PlanReadGuard(manifestPath, manifestSnapshot))

	orientationPath := filepath.Join(root, ".tiller", "orientation.md")
	orientation, before, err := store.ReadRegularFileSnapshot(root, orientationPath)
	if err != nil {
		return plan, err
	}
	plan.result.Before, _ = gen.OrientationStampGeneration(orientation)
	plan.result.After = plan.result.Before
	if err := refuseNewerOrientation(plan.result.Before); err != nil {
		return plan, err
	}
	plan.replace(".tiller/orientation.md", orientation, before, []byte(gen.Orientation()))

	for _, target := range []struct {
		path   string
		render func(string) (string, error)
	}{
		{"AGENTS.md", gen.UpsertPointerBlock},
		{"CLAUDE.md", gen.UpsertRedirectBlock},
		{".gitignore", func(content string) (string, error) {
			return upsertIgnoreGroups(content, tillerIgnoreGroups), nil
		}},
	} {
		data, snapshot, err := store.ReadRegularFileSnapshot(root, filepath.Join(root, target.path))
		if err != nil {
			return plan, err
		}
		rendered, err := target.render(string(data))
		if err != nil {
			return plan, &store.EnvError{Code: 3, Reason: "managed_region_malformed", Msg: target.path + ": " + err.Error()}
		}
		plan.replace(target.path, data, snapshot, []byte(rendered))
	}

	if err := plan.planShims(); err != nil {
		return plan, err
	}
	if err := plan.tree.CheckStable(); err != nil {
		return plan, err
	}
	// The unchecked loader uses the already-parsed manifest, so close that
	// read boundary explicitly even when this is a write-free inspection.
	_, currentManifest, err := store.ReadRegularFileSnapshot(root, manifestPath)
	if err != nil {
		return plan, err
	}
	if currentManifest != manifestSnapshot {
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonMutationStale, Msg: ".tiller/tiller.yml changed while project refresh was planned; retry"}
	}
	plan.result.After = gen.OrientationGeneration()
	sort.Strings(plan.result.Files)
	sort.Strings(plan.result.ShimsAdded)
	sort.Strings(plan.result.ShimsRemoved)
	return plan, nil
}

func (plan *projectRefreshPlan) replace(relative string, existing []byte, before store.FileSnapshot, desired []byte) {
	path := filepath.Join(plan.tree.Root, filepath.FromSlash(relative))
	if before.Exists() && bytes.Equal(existing, desired) {
		// A concurrent newer orientation or human edit must still invalidate a
		// batch that happens to change only a different artifact.
		plan.writes = append(plan.writes, store.PlanReadGuard(path, before))
		return
	}
	plan.writes = append(plan.writes, store.PlanWrite(path, before, desired))
	plan.result.Files = append(plan.result.Files, filepath.ToSlash(relative))
}

func (plan *projectRefreshPlan) planShims() error {
	root := plan.tree.Root
	const skillsDir = ".claude/skills"
	entries, directory, err := readRefreshDirectory(root, skillsDir)
	if err != nil {
		return err
	}
	plan.writes = append(plan.writes, store.PlanDirectoryGuard(filepath.Join(root, skillsDir), directory))
	names := skills.Names()
	current := make(map[string]bool, len(names))
	for _, skill := range names {
		name := gen.SkillShimName(skill)
		current[name] = true
		description, ok := skills.Description(skill)
		if !ok || description == "" {
			return fmt.Errorf("skill %q has no trigger description", skill)
		}
		relative := skillsDir + "/" + name + "/SKILL.md"
		data, snapshot, err := store.ReadRegularFileSnapshot(root, filepath.Join(root, relative))
		if err != nil {
			return err
		}
		if snapshot.Exists() && !gen.OwnsSkillShim(name, string(data)) {
			return &store.EnvError{Code: 3, Reason: "skill_shim_unowned", Msg: relative + " is not a recognized generated Tiller shim; preserve or move that custom skill before refreshing"}
		}
		if !snapshot.Exists() {
			plan.result.ShimsAdded = append(plan.result.ShimsAdded, name)
		}
		plan.replace(relative, data, snapshot, []byte(gen.SkillShim(skill, description)))
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "tiller-") || current[name] {
			continue
		}
		relativeDir := skillsDir + "/" + name
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is an unsafe symlink", relativeDir)
		}
		if !entry.IsDir() {
			continue
		}
		_, directory, err := readRefreshDirectory(root, relativeDir)
		if err != nil {
			return err
		}
		relative := relativeDir + "/SKILL.md"
		path := filepath.Join(root, relative)
		data, snapshot, err := store.ReadRegularFileSnapshot(root, path)
		if err != nil {
			return err
		}
		plan.writes = append(plan.writes, store.PlanDirectoryGuard(filepath.Join(root, relativeDir), directory))
		if !snapshot.Exists() {
			plan.writes = append(plan.writes, store.PlanReadGuard(path, snapshot))
			continue
		}
		if !gen.OwnsSkillShim(name, string(data)) {
			plan.result.Warnings = append(plan.result.Warnings, relative+" was preserved: obsolete shim ownership is ambiguous")
			plan.writes = append(plan.writes, store.PlanReadGuard(path, snapshot))
			continue
		}
		plan.writes = append(plan.writes, store.PlanDeleteFile(path, snapshot))
		if len(directory.Names) == 1 && directory.Names[0] == "SKILL.md" {
			// Replace the guard with removal: removal itself validates the exact
			// directory inventory before deleting the owned file and empty dir.
			plan.writes[len(plan.writes)-2] = store.PlanRemoveEmptyDir(filepath.Join(root, relativeDir), directory)
		}
		plan.result.Files = append(plan.result.Files, relative)
		plan.result.ShimsRemoved = append(plan.result.ShimsRemoved, name)
	}
	return nil
}

// readRefreshDirectory consumes the descriptor from an anchored path walk, not
// a second pathname lookup that could follow a swapped symlink.
func readRefreshDirectory(root, relative string) ([]os.DirEntry, store.DirectorySnapshot, error) {
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return nil, store.DirectorySnapshot{}, err
	}
	defer anchored.Close()
	dir, err := (rootpath.Walker{}).Open(anchored, filepath.FromSlash(relative), true)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, store.DirectorySnapshot{}, nil
	}
	if err != nil {
		return nil, store.DirectorySnapshot{}, err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return nil, store.DirectorySnapshot{}, err
	}
	if !info.IsDir() {
		return nil, store.DirectorySnapshot{}, fmt.Errorf("%s is not a directory", relative)
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, store.DirectorySnapshot{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return entries, store.DirectorySnapshot{Exists: true, Mode: info.Mode(), Names: names}, nil
}
