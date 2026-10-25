package mutate

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/store"
)

func readMutationDirectory(root, path string) ([]fs.DirEntry, error) {
	relative, err := store.RepositoryRelativePath(root, path)
	if err != nil {
		return nil, err
	}
	return store.ReadRepositoryDirectory(root, relative)
}

func mutationPathInfo(root, path string) (fs.FileInfo, error) {
	relative, err := store.RepositoryRelativePath(root, path)
	if err != nil {
		return nil, err
	}
	return store.RepositoryPathInfo(root, relative)
}

// walkMutationState confines both enumeration and the metadata delivered to the
// planner. Each subsequent content read retains its own anchored snapshot.
func walkMutationState(root, path string, visit fs.WalkDirFunc) error {
	info, err := mutationPathInfo(root, path)
	if err != nil {
		return visit(path, nil, err)
	}
	var walk func(string, fs.DirEntry) error
	walk = func(path string, entry fs.DirEntry) error {
		if err := visit(path, entry, nil); err != nil {
			// fs.WalkDirFunc: SkipDir on a directory skips it; on a file it is
			// returned to the parent loop, which skips the remaining siblings.
			if errors.Is(err, fs.SkipDir) && entry.IsDir() {
				return nil
			}
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		entries, err := readMutationDirectory(root, path)
		if err != nil {
			if err := visit(path, entry, err); err != nil && !errors.Is(err, fs.SkipDir) {
				return err
			}
			return nil
		}
		for _, child := range entries {
			if err := walk(filepath.Join(path, child.Name()), child); err != nil {
				if errors.Is(err, fs.SkipDir) {
					return nil
				}
				return err
			}
		}
		return nil
	}
	err = walk(path, fs.FileInfoToDirEntry(info))
	if errors.Is(err, fs.SkipAll) || errors.Is(err, fs.SkipDir) {
		return nil
	}
	return err
}
