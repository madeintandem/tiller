package mutate

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/store"
)

// Capture before loading the validation tree, not afterward: a hand edit does
// not advance Tiller's generation. File writes must retain these preimages;
// read-only inputs are checked by Commit under the same mutation lock.
func captureMoveReadGuards(root string) ([]store.PlannedWrite, error) {
	var guards []store.PlannedWrite
	err := walkMutationState(root, filepath.Join(root, ".tiller"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == filepath.Join(root, ".tiller", ".cache") {
				return filepath.SkipDir
			}
			snapshot, err := store.ReadDirectorySnapshot(root, path)
			if err != nil {
				return err
			}
			guards = append(guards, store.PlanDirectoryGuard(path, snapshot))
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nonRegularMoveEntry(path)
		}
		_, snapshot, err := store.ReadRegularFileSnapshot(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		guards = append(guards, store.PlanReadGuardWithMode(path, snapshot, info.Mode()))
		return nil
	})
	return guards, err
}
