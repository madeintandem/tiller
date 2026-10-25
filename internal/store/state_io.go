package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Repository-owned state, prose, cache, and evidence IO names its checkout
// root explicitly: ReadRepositoryFile, ReadRepositoryDirectory,
// RepositoryPathInfo, MkdirRepositoryAll and WriteRepositoryFileAtomic take
// (root, relative), and the snapshot readers take (root, path) to pair with
// PlanWrite's absolute paths. Each refuses symlink escapes beneath root.

// RepositoryRelativePath converts path to a path relative to the checkout root,
// refusing one that lies outside it. The check is lexical; the rooted IO that
// consumes the result refuses symlink components. Callers holding absolute
// paths built from root use it to reach the (root, relative) helpers.
func RepositoryRelativePath(root, path string) (string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootAbsolute, absolute)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside repository root %s", path, root)
	}
	return relative, nil
}

// ReadRepositoryFile reads a regular repository-relative state file without
// following symlinks at any component. Missing lazy files retain ErrNotExist.
func ReadRepositoryFile(root, relative string) ([]byte, error) {
	reader, err := OpenRepositoryReader(root)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.ReadFile(relative)
}

// readStateFile and readStateDirectory are the loader's forms of
// ReadRepositoryFile and ReadRepositoryDirectory for a path built from root;
// the loader keeps the absolute path for its error messages.
func readStateFile(root, path string) ([]byte, error) {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return nil, err
	}
	return ReadRepositoryFile(root, relative)
}

func readStateDirectory(root, path string) ([]fs.DirEntry, error) {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return nil, err
	}
	return ReadRepositoryDirectory(root, relative)
}

// ReadRepositoryDirectory reads directory entries beneath one checkout anchor.
// Entry metadata is captured before closing the directory, so Info never falls
// back to a later pathname lookup.
func ReadRepositoryDirectory(root, relative string) ([]fs.DirEntry, error) {
	reader, err := OpenRepositoryReader(root)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.ReadDirectory(relative)
}

// RepositoryPathInfo inspects a leaf without following it or any symlink
// component above it. Callers can reject symlink and nonregular leaf modes.
func RepositoryPathInfo(root, relative string) (fs.FileInfo, error) {
	reader, err := OpenRepositoryReader(root)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.PathInfo(relative)
}

// MkdirRepositoryAll creates directories through retained checkout anchors.
func MkdirRepositoryAll(root, relative string, mode fs.FileMode) error {
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	if err != nil {
		return err
	}
	defer rooted.close()
	return rooted.mkdirAll(relative, mode)
}

// WriteRepositoryFileAtomic confines directory creation, temporary-file writes,
// replacement and directory sync to one repository anchor.
func WriteRepositoryFileAtomic(root, relative string, data []byte) error {
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	if err != nil {
		return err
	}
	defer rooted.close()
	if err := rooted.mkdirAll(filepath.Dir(relative), 0o755); err != nil {
		return err
	}
	return rooted.writeFileAtomic(relative, data)
}

func (t *rootedTransaction) mkdirAll(target string, mode os.FileMode) error {
	if target == "." {
		return t.retainParent(".")
	}
	if target == "" || filepath.IsAbs(target) || filepath.Clean(target) != target ||
		target == ".." || strings.HasPrefix(target, ".."+string(filepath.Separator)) {
		return &os.PathError{Op: "mkdir", Path: target, Err: fs.ErrInvalid}
	}
	parent := filepath.Dir(target)
	if err := t.mkdirAll(parent, mode); err != nil {
		return err
	}
	if err := t.retainParent(target); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return t.mkdir(target, mode)
}

func (t *rootedTransaction) relativePath(path string) (string, error) {
	return RepositoryRelativePath(t.root.Name(), path)
}

// A journal without a rooted transaction is an unanchored test fixture; every
// production journal carries the mutation's rooted transaction.
func (j *journal) exists() (bool, error) {
	if j.rooted == nil {
		return pathExists(j.path)
	}
	relative, err := j.rooted.relativePath(j.path)
	if err != nil {
		return false, err
	}
	return j.rooted.exists(relative)
}

func (j *journal) remove() error {
	if j.rooted == nil {
		return removeIfExists(j.path)
	}
	relative, err := j.rooted.relativePath(j.path)
	if err != nil {
		return err
	}
	// clearJournalDurably owns the subsequent sync and checkpoint restoration.
	return j.rooted.removeFileWithSync(relative, false)
}
