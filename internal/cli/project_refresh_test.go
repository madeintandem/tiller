package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

type refreshDiskEntry struct {
	Data     string
	Mode     fs.FileMode
	Modified time.Time
}

func refreshDiskState(t *testing.T, root string, timestamps bool) map[string]refreshDiskEntry {
	t.Helper()
	state := map[string]refreshDiskEntry{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := refreshDiskEntry{Mode: info.Mode()}
		if timestamps {
			item.Modified = info.ModTime()
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			item.Data, err = os.Readlink(path)
		} else if !info.IsDir() {
			var data []byte
			data, err = os.ReadFile(path)
			item.Data = string(data)
		}
		state[filepath.ToSlash(relative)] = item
		return err
	}))
	return state
}

func putRefreshFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestProjectRefreshNeedsNoIdentityAndRepeatDoesNotWrite(t *testing.T) {
	root := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "not-on-the-roster")
	manifest := readFile(t, root, ".tiller", "tiller.yml")
	generationPath := filepath.Join(root, ".tiller", ".cache", "generation")
	_, generation, err := store.ReadRegularFileSnapshot(root, generationPath)
	require.NoError(t, err)
	before := refreshDiskState(t, root, true)
	inspection, err := refreshProject(root, true)
	require.NoError(t, err)
	require.Equal(t, before, refreshDiskState(t, root, true), "inspection cannot create shims, locks, cache entries, or identity")
	require.Contains(t, inspection.Files, "AGENTS.md")
	require.Contains(t, inspection.Files, "CLAUDE.md")
	applied, err := refreshProject(root, false)
	require.NoError(t, err)
	require.Equal(t, inspection, applied)
	require.Equal(t, gen.OrientationGeneration(), applied.After)
	require.Equal(t, manifest, readFile(t, root, ".tiller", "tiller.yml"))
	_, afterGeneration, err := store.ReadRegularFileSnapshot(root, generationPath)
	require.NoError(t, err)
	require.Equal(t, generation, afterGeneration)
	require.NoFileExists(t, filepath.Join(root, ".tiller", "identity.yml"))
	for _, skill := range skills.Names() {
		description, ok := skills.Description(skill)
		require.True(t, ok)
		require.Equal(t, gen.SkillShim(skill, description), readFile(t, root, ".claude", "skills", gen.SkillShimName(skill), "SKILL.md"))
	}
	// A malformed existing identity is equally irrelevant to artifact-only work.
	putRefreshFile(t, root, ".tiller/identity.yml", "not: [valid identity yaml\n")

	// Fixed timestamps catch redundant atomic writes even on coarse filesystems.
	stamp := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	for relative, entry := range refreshDiskState(t, root, false) {
		if entry.Mode.IsRegular() {
			require.NoError(t, os.Chtimes(filepath.Join(root, filepath.FromSlash(relative)), stamp, stamp))
		}
	}
	before = refreshDiskState(t, root, true)
	repeated, err := refreshProject(root, false)
	require.NoError(t, err)
	require.Empty(t, repeated.Files)
	require.Empty(t, repeated.ShimsAdded)
	require.Empty(t, repeated.ShimsRemoved)
	require.Equal(t, before, refreshDiskState(t, root, true), "a no-op must not even create and remove a cache lock")
}

func TestProjectRefreshAndSourceWriteIgnoreRetiredBoard(t *testing.T) {
	for _, kind := range []string{"stale", "invalid-utf8", "nul", "unreadable", "directory", "outside-symlink", "broken-symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			epicID, _, _ := layRenameCLI(t)
			root, err := repoRoot()
			require.NoError(t, err)
			path := filepath.Join(root, ".tiller", "board.md")
			require.NoFileExists(t, path)
			switch kind {
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o755))
				putRefreshFile(t, root, ".tiller/board.md/legacy.md", "Legacy content\n")
			case "outside-symlink", "broken-symlink":
				target := filepath.Join(t.TempDir(), "outside.md")
				if kind == "outside-symlink" {
					require.NoError(t, os.WriteFile(target, []byte("Outside content\n"), 0o644))
				}
				require.NoError(t, os.Symlink(target, path))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(path, 0o600))
			default:
				content := []byte("Stale T-abc123 keyword\n")
				if kind == "invalid-utf8" {
					content = []byte{0xff}
				} else if kind == "nul" {
					content = []byte{'a', 0, 'b'}
				}
				require.NoError(t, os.WriteFile(path, content, 0o644))
				if kind == "unreadable" {
					require.NoError(t, os.Chmod(path, 0))
					t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
				}
			}
			before, err := os.Lstat(path)
			require.NoError(t, err)
			for _, inspect := range []bool{true, false} {
				result, err := refreshProject(root, inspect)
				require.NoError(t, err)
				require.NotContains(t, result.Files, ".tiller/board.md")
			}
			events := correctionEventCount(t)
			out, stderr, code := execCLI(t, "rename", "epic", epicID, "--title", "Updated foundation", "--contributor", "jc", "--json")
			require.Equal(t, 0, code, stderr)
			require.NotContains(t, jsonData(t, out), "warning")
			tree, err := store.Load(root)
			require.NoError(t, err)
			require.Equal(t, "Updated foundation", tree.Epics[epicID].Title)
			require.Equal(t, events+1, correctionEventCount(t))
			after, err := os.Lstat(path)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after), "retired entry must not be replaced")
			require.Equal(t, before.Mode(), after.Mode())
			require.Equal(t, before.ModTime(), after.ModTime())
			require.Equal(t, before.Size(), after.Size())
		})
	}
}

func TestProjectRefreshInspectionValidatesOlderSchemaWithoutCacheWrites(t *testing.T) {
	root := t.TempDir()
	putRefreshFile(t, root, ".tiller/tiller.yml", "schema_version: 4\nproject:\n  name: Old\n  key: old\n  kind: greenfield\n")
	before := refreshDiskState(t, root, true)
	result, err := refreshProject(root, true)
	require.NoError(t, err)
	require.Equal(t, 0, result.Before)
	require.Equal(t, gen.OrientationGeneration(), result.After)
	require.Contains(t, result.Files, ".tiller/orientation.md")
	require.NotContains(t, result.Files, ".tiller/board.md")
	require.Equal(t, before, refreshDiskState(t, root, true))
	require.NoDirExists(t, filepath.Join(root, ".tiller", ".cache"))

	_, err = refreshProject(root, false)
	require.Error(t, err, "apply cannot bypass migration")
	require.Equal(t, before, refreshDiskState(t, root, true))
	putRefreshFile(t, root, "CLAUDE.md", gen.PointerBegin+"\nhuman text without an end marker\n")
	before = refreshDiskState(t, root, true)
	_, err = refreshProject(root, true)
	require.Error(t, err, "older-schema candidate inspection must still validate artifact destinations")
	require.Equal(t, before, refreshDiskState(t, root, true))
}

func TestProjectRefreshPreservesHumanTextAndUnrelatedSkills(t *testing.T) {
	root := initRepo(t)
	header, footer := "# Human instructions\n\n", "\n\nKeep these project rules verbatim.\n"
	putRefreshFile(t, root, "AGENTS.md", header+gen.PointerBegin+"\nOld managed content\n"+gen.PointerEnd+footer)
	putRefreshFile(t, root, "CLAUDE.md", "# Claude's human notes\n")
	putRefreshFile(t, root, ".gitignore", "# Existing rules\nnode_modules/\n")
	putRefreshFile(t, root, ".claude/skills/personal/SKILL.md", "My unrelated skill.\n")
	putRefreshFile(t, root, ".claude/skills/tiller-custom/SKILL.md", "My similarly named custom skill.\n")
	putRefreshFile(t, root, ".claude/skills/tiller-retired/SKILL.md", gen.SkillShim("retired", "Old generated discovery."))
	putRefreshFile(t, root, ".claude/skills/tiller-extra/SKILL.md", gen.SkillShim("extra", "Old generated discovery."))
	putRefreshFile(t, root, ".claude/skills/tiller-extra/notes.txt", "A human's extra file.\n")

	result, err := refreshProject(root, false)
	require.NoError(t, err)
	require.Equal(t, header+gen.PointerBlock()+footer, readFile(t, root, "AGENTS.md"))
	require.True(t, strings.HasPrefix(readFile(t, root, "CLAUDE.md"), "# Claude's human notes\n"))
	require.True(t, strings.HasPrefix(readFile(t, root, ".gitignore"), "# Existing rules\nnode_modules/\n"))
	require.Equal(t, "My unrelated skill.\n", readFile(t, root, ".claude", "skills", "personal", "SKILL.md"))
	require.Equal(t, "My similarly named custom skill.\n", readFile(t, root, ".claude", "skills", "tiller-custom", "SKILL.md"))
	require.Equal(t, "A human's extra file.\n", readFile(t, root, ".claude", "skills", "tiller-extra", "notes.txt"))
	require.NoFileExists(t, filepath.Join(root, ".claude", "skills", "tiller-extra", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, ".claude", "skills", "tiller-retired"))
	require.Equal(t, []string{"tiller-extra", "tiller-retired"}, result.ShimsRemoved)
	require.Len(t, result.Warnings, 1)
	require.Contains(t, result.Warnings[0], ".claude/skills/tiller-custom/SKILL.md")
}

func TestProjectRefreshRefusalsPreserveEveryDestination(t *testing.T) {
	for _, scenario := range []string{"newer orientation", "malformed agents", "malformed redirect", "duplicate markers", "symlink leaf", "symlink parent", "directory leaf", "unowned current shim"} {
		t.Run(scenario, func(t *testing.T) {
			root := initRepo(t)
			switch scenario {
			case "newer orientation":
				newer := strings.Replace(gen.Orientation(), "orientation generation "+strconv.Itoa(gen.OrientationGeneration()), "orientation generation "+strconv.Itoa(gen.OrientationGeneration()+1), 1)
				putRefreshFile(t, root, ".tiller/orientation.md", newer)
			case "malformed agents":
				putRefreshFile(t, root, "AGENTS.md", gen.PointerBegin+"\nDo not lose these notes.\n")
			case "malformed redirect":
				putRefreshFile(t, root, "CLAUDE.md", "Human text\n"+gen.PointerEnd)
			case "duplicate markers":
				putRefreshFile(t, root, "AGENTS.md", gen.PointerBlock()+"\nHuman text\n"+gen.PointerBlock())
			case "symlink leaf":
				require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(root, "AGENTS.md")))
			case "symlink parent":
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, ".claude")))
			case "directory leaf":
				require.NoError(t, os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755))
			case "unowned current shim":
				putRefreshFile(t, root, ".claude/skills/tiller-orient/SKILL.md", "Custom orientation, not generated.\n")
			}
			before := refreshDiskState(t, root, true)
			for _, inspect := range []bool{true, false} {
				result, err := refreshProject(root, inspect)
				require.Error(t, err)
				require.Empty(t, result.Files)
				require.Equal(t, before, refreshDiskState(t, root, true))
				if scenario == "newer orientation" {
					require.Equal(t, gen.OrientationGeneration()+1, result.Before)
					require.Equal(t, result.Before, result.After)
					require.Contains(t, err.Error(), "upgrade")
				}
			}
		})
	}
}

func TestProjectRefreshRollsBackEarlierArtifactsWhenShimWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the filesystem permission boundary")
	}
	root := initRepo(t)
	_, err := refreshProject(root, false)
	require.NoError(t, err)
	aged := strings.Replace(gen.Orientation(), "orientation generation "+strconv.Itoa(gen.OrientationGeneration()), "orientation generation 1", 1)
	putRefreshFile(t, root, ".tiller/orientation.md", aged)
	putRefreshFile(t, root, ".gitignore", "# Human rules survive rollback\n")
	putRefreshFile(t, root, "AGENTS.md", "# Human instructions survive rollback\n")
	name := skills.Names()[len(skills.Names())-1]
	directory := filepath.Join(root, ".claude", "skills", gen.SkillShimName(name))
	require.NoError(t, os.Remove(filepath.Join(directory, "SKILL.md")))
	require.NoError(t, os.Chmod(directory, 0o555))
	t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
	before := refreshDiskState(t, root, false)

	result, err := refreshProject(root, false)
	require.Error(t, err)
	require.Empty(t, result.Files, "planned writes must not be reported as completed")
	require.Equal(t, 1, result.After)
	require.Equal(t, before, refreshDiskState(t, root, false), "orientation, wiring, ignore entries, and shims share one rollback")
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
}

func TestProjectRefreshRefusesChangedObsoleteShimInventory(t *testing.T) {
	root := initRepo(t)
	putRefreshFile(t, root, ".claude/skills/tiller-retired/SKILL.md", gen.SkillShim("retired", "Old generated discovery."))
	plan, err := planProjectRefresh(root, false)
	require.NoError(t, err)
	putRefreshFile(t, root, ".claude/skills/tiller-retired/notes.txt", "Concurrent human file.\n")
	before := refreshDiskState(t, root, false)
	err = plan.tree.CommitDerived("refresh project artifacts", plan.writes)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.Equal(t, before, refreshDiskState(t, root, false))
}

func TestProjectRefreshRefusesUnreadableArtifacts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the filesystem permission boundary")
	}
	root := initRepo(t)
	path := filepath.Join(root, "AGENTS.md")
	putRefreshFile(t, root, "AGENTS.md", "Human instructions.\n")
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	for _, inspect := range []bool{true, false} {
		_, err := refreshProject(root, inspect)
		require.Error(t, err, fmt.Sprintf("inspect=%t", inspect))
		require.ErrorIs(t, err, fs.ErrPermission)
	}
	require.NoDirExists(t, filepath.Join(root, ".claude"))
	require.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, os.Chmod(path, 0o644))
	require.Equal(t, "Human instructions.\n", readFile(t, path))
}
