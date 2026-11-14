package vcs

import "path/filepath"

// ProjectWorktreePaths lists the directory Dir occupies in every worktree, in
// WorktreePaths order: each worktree's root joined with Dir's path inside the
// repository (`git rev-parse --show-prefix`). For a project at the repository
// root these are the worktree roots themselves. For one at apps/a they are
// each worktree's apps/a, never the root that also holds its siblings. A
// worktree whose branch does not carry the directory is still listed.
func (g *Git) ProjectWorktreePaths() ([]string, error) {
	prefix, err := g.run("rev-parse", "--show-prefix")
	if err != nil {
		return nil, g.asEnv(err)
	}
	roots, err := g.WorktreePaths()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(roots))
	for i, root := range roots {
		paths[i] = filepath.Join(root, filepath.FromSlash(prefix))
	}
	return paths, nil
}
