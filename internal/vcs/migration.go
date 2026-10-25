package vcs

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// CheckMigrationInputs requires a committed, clean baseline only for existing
// transformation inputs. Create-only outputs must instead use absence guards in
// the source transaction. All observations leave the raw Git index untouched.
func (g *Git) CheckMigrationInputs(paths []string) error {
	conflicts, err := g.runRaw("ls-files", "--unmerged", "-z")
	if err != nil {
		return g.asEnv(err)
	}
	if conflicts != "" {
		return &store.EnvError{Code: 1, Reason: "migration_git_conflicts",
			Msg: "resolve unresolved Git conflicts before migrating; no migration files were changed"}
	}
	for _, path := range paths {
		committed, err := g.runRaw("--literal-pathspecs", "ls-tree", "-z", "HEAD", "--", path)
		if err != nil {
			// An unborn repository has no recoverable source baseline either.
			if _, repoErr := g.run("rev-parse", "--git-dir"); repoErr != nil {
				return g.asEnv(repoErr)
			}
			return &store.EnvError{Code: 1, Reason: "migration_inputs_dirty",
				Msg: fmt.Sprintf("migration input %s needs a committed Git baseline before migrating: %v", path, err)}
		}
		entry := strings.TrimSuffix(committed, "\x00")
		metadata, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(metadata)
		if !ok || name != path || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return &store.EnvError{Code: 1, Reason: "migration_inputs_dirty",
				Msg: fmt.Sprintf("migration input %s has no committed regular-file baseline; review and commit this input before migrating", path)}
		}
		// Git status deliberately hides worktree edits behind these index bits.
		// Refuse rather than clearing user flags or trusting a false clean result.
		flags, err := g.runRaw("--literal-pathspecs", "ls-files", "-v", "-z", "--", path)
		if err != nil {
			return g.asEnv(err)
		}
		if flags != "H "+path+"\x00" {
			return &store.EnvError{Code: 1, Reason: "migration_inputs_dirty",
				Msg: fmt.Sprintf("migration input %s has hidden or unsupported index state; clear assume-unchanged/skip-worktree for this input and review its changes before migrating", path)}
		}
		dirty, err := g.runRaw("--no-optional-locks", "--literal-pathspecs", "status", "--porcelain", "-z", "--untracked-files=all", "--", path)
		if err != nil {
			return g.asEnv(err)
		}
		if dirty != "" {
			return &store.EnvError{Code: 1, Reason: "migration_inputs_dirty",
				Msg: fmt.Sprintf("migration input %s has staged or unstaged changes; review and commit this input before migrating (unrelated dirty files may remain)", path)}
		}
	}
	return nil
}
