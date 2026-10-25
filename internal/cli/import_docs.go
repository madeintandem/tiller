package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/store"
)

// This file holds the copy-out-only import operation tiller init --design and
// tiller intake share, with its source checks.

// dirHasFiles reports whether dir contains at least one regular file anywhere in
// its tree, the test for a non-empty import source (a tree of only empty
// subdirectories, or of nothing but OS junk, would copy nothing).
func dirHasFiles(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && !junkFile(d.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// junkFile reports whether a directory entry is OS junk the design import
// ignores (.DS_Store, Thumbs.db). Junk filter only; everything else copies.
func junkFile(name string) bool {
	return name == ".DS_Store" || name == "Thumbs.db"
}

// importDocs copies an explicitly selected external source into destDir beneath
// the checkout root. A directory preserves its subfolders; a single feedback
// document lands under its basename. Both init and intake use this copy-out-only
// operation. Nonregular directory entries and OS junk are skipped. The result
// counts copied regular files; no destination operation follows a symlink. A
// directory source overlapping its destination is refused before anything is
// written, so the walk can never read its own output; a single file cannot.
func importDocs(root, srcPath, destDir string) (int, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		// A single-file copy cannot recurse into its own output, so a file
		// already filed under the destination (an archived feedback document)
		// copies like any other; only a directory walk needs the overlap guard.
		if !info.Mode().IsRegular() || junkFile(info.Name()) {
			return 0, nil
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return 0, err
		}
		if err := store.WriteRepositoryFileAtomic(root, filepath.Join(destDir, filepath.Base(srcPath)), data); err != nil {
			return 0, err
		}
		return 1, nil
	}

	overlaps, err := importOverlaps(root, srcPath, destDir)
	if err != nil {
		return 0, err
	}
	if overlaps {
		return 0, fmt.Errorf("import source %s overlaps its destination %s", srcPath, destDir)
	}
	count := 0
	err = filepath.WalkDir(srcPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return store.MkdirRepositoryAll(root, target, 0o755)
		}
		if !d.Type().IsRegular() || junkFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := store.WriteRepositoryFileAtomic(root, target, data); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// importOverlaps reports whether copying the directory src into destDir beneath root would
// read its own output: the destination is the source or lies beneath it, or the
// source lies at or beneath an existing destination. Paths are compared by
// filesystem identity along their symlink-resolved ancestor chains, so neither
// a symlinked ancestor nor a case variant of the checkout hides the overlap.
// Only metadata is inspected; no directory is enumerated.
func importOverlaps(root, src, destDir string) (bool, error) {
	srcPath, err := filepath.EvalSymlinks(src)
	if err != nil {
		return false, err
	}
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false, err
	}
	dest := filepath.Join(root, destDir)
	destPath, err := nearestExistingPath(dest)
	if err != nil {
		return false, err
	}
	if within, err := hasAncestorIdentity(destPath, srcInfo); within || err != nil {
		return within, err
	}
	if resolved, err := filepath.EvalSymlinks(dest); err == nil {
		destInfo, err := os.Stat(resolved)
		if err != nil {
			return false, err
		}
		return hasAncestorIdentity(srcPath, destInfo)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return false, nil
}

// nearestExistingPath resolves path, or its nearest existing ancestor when
// path does not exist yet, to a symlink-free absolute path.
func nearestExistingPath(path string) (string, error) {
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return resolved, nil
		}
		parent := filepath.Dir(path)
		if !errors.Is(err, fs.ErrNotExist) || parent == path {
			return "", err
		}
		path = parent
	}
}

// hasAncestorIdentity reports whether path or any of its ancestors is the
// same file as target.
func hasAncestorIdentity(path string, target os.FileInfo) (bool, error) {
	for {
		info, err := os.Stat(path)
		if err != nil {
			return false, err
		}
		if os.SameFile(info, target) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, nil
		}
		path = parent
	}
}
