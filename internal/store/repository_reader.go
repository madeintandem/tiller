package store

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/rootpath"
	"gopkg.in/yaml.v3"
)

// repositoryReaderWalker is the walker every RepositoryReader descends with.
// Production leaves it zero; store tests set its hooks to count descents.
var repositoryReaderWalker = rootpath.Walker{}

// RepositoryReader is ReadRepositoryFile, ReadRepositoryDirectory, and
// RepositoryPathInfo for a read of many files. It anchors the checkout once and
// verifies each directory once (rootpath.Reader), with the same refusal of a
// symlink at any component. Use one for one bounded read on one goroutine and
// Close it; it keeps a bounded number of directories open.
type RepositoryReader struct {
	anchor *os.Root
	reader *rootpath.Reader
}

// OpenRepositoryReader anchors root for a bounded multi-file read.
func OpenRepositoryReader(root string) (*RepositoryReader, error) {
	anchor, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &RepositoryReader{anchor: anchor, reader: rootpath.NewReader(anchor, repositoryReaderWalker)}, nil
}

// ReadFile reads a regular repository-relative file. A missing lazy file keeps
// ErrNotExist.
func (r *RepositoryReader) ReadFile(relative string) ([]byte, error) {
	if err := checkCanonicalTarget(relative); err != nil {
		return nil, err
	}
	file, err := r.reader.Open(relative, false)
	if err != nil {
		return nil, rootedPathError(err)
	}
	defer file.Close()
	return io.ReadAll(file)
}

// ReadDirectory lists a repository-relative directory. Each entry's metadata
// is an Lstat taken through the anchored directory, so Info never falls back
// to a later pathname lookup.
func (r *RepositoryReader) ReadDirectory(relative string) ([]fs.DirEntry, error) {
	if err := checkCanonicalTarget(relative); err != nil {
		return nil, err
	}
	directory, err := r.reader.Dir(relative)
	if err != nil {
		return nil, rootedPathError(err)
	}
	entries, err := fs.ReadDir(directory.FS(), ".")
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		info, err := directory.Lstat(entry.Name())
		if err != nil {
			return nil, err
		}
		entries[i] = fs.FileInfoToDirEntry(info)
	}
	return entries, nil
}

// PathInfo inspects a leaf without following it or any symlink above it.
func (r *RepositoryReader) PathInfo(relative string) (fs.FileInfo, error) {
	if err := checkCanonicalTarget(relative); err != nil {
		return nil, err
	}
	parent, err := r.reader.Dir(filepath.Dir(relative))
	if err != nil {
		return nil, rootedPathError(err)
	}
	return parent.Lstat(filepath.Base(relative))
}

// Close releases every retained directory and the anchor.
func (r *RepositoryReader) Close() error {
	return errors.Join(r.reader.Close(), r.anchor.Close())
}

// stateFile, stateDirectory, stateYAML, and stateFileExists are the loader's
// reads. During a load they go through the tree's one RepositoryReader, so each
// directory under .tiller/ is verified once per load. A tree with no reader
// (an in-memory tree, or a call after Load returned) falls back to the
// one-shot helpers.
func (t *Tree) stateFile(path string) ([]byte, error) {
	if t.reader == nil {
		return readStateFile(t.Root, path)
	}
	relative, err := RepositoryRelativePath(t.Root, path)
	if err != nil {
		return nil, err
	}
	return t.reader.ReadFile(relative)
}

func (t *Tree) stateDirectory(path string) ([]fs.DirEntry, error) {
	if t.reader == nil {
		return readStateDirectory(t.Root, path)
	}
	relative, err := RepositoryRelativePath(t.Root, path)
	if err != nil {
		return nil, err
	}
	return t.reader.ReadDirectory(relative)
}

func (t *Tree) stateYAML(path string, out any) error {
	data, err := t.stateFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func (t *Tree) stateFileExists(path string) bool {
	if t.reader == nil {
		return fileExists(t.Root, path)
	}
	relative, err := RepositoryRelativePath(t.Root, path)
	if err != nil {
		return false
	}
	info, err := t.reader.PathInfo(relative)
	return err == nil && info.Mode().IsRegular()
}
