// Package rootpath opens relative paths without ever resolving more than one
// unchecked path component at a time.
package rootpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrUnsafe identifies a symlink or identity change observed while binding a
// path component to an anchored Root. Missing and non-regular objects retain
// their underlying error class so callers can preserve their existing status
// distinctions.
var ErrUnsafe = errors.New("unsafe anchored path")

// ErrChanged identifies a regular leaf replaced by another regular file between
// its lstat and its open, the shape of a concurrent atomic rename. It is also
// ErrUnsafe; a symlink at the leaf is never ErrChanged.
var ErrChanged = errors.New("anchored path changed while it was opened")

// changedError keeps ErrUnsafe's message shape while also matching ErrChanged.
type changedError struct{ name string }

func (e changedError) Error() string {
	return fmt.Sprintf("%v: %s changed while it was opened", ErrUnsafe, e.name)
}

func (e changedError) Is(target error) bool { return target == ErrUnsafe || target == ErrChanged }

// Walker performs segment-by-segment opens beneath an already anchored Root.
// AfterOpenRoot and BeforeOpenFile are deterministic security-test seams.
// Production walkers leave both nil.
type Walker struct {
	AfterOpenRoot  func(name string)
	BeforeOpenFile func(name string)
}

// OpenRoot returns a new Root anchored at the named directory. Every segment is
// lstat-checked relative to its anchored parent, opened as a child Root, statted
// through that child, then lstat-checked again from the parent. Traversal only
// continues when all three observations identify the same directory.
func (w Walker) OpenRoot(root *os.Root, name string) (*os.Root, error) {
	segments, err := split(name)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		segments = []string{"."}
	}

	current := root
	owned := false
	openedName := ""
	for _, segment := range segments {
		openedName = join(openedName, segment)
		child, _, err := w.descend(current, segment, openedName)
		if err != nil {
			if owned {
				current.Close()
			}
			return nil, err
		}
		if owned {
			current.Close()
		}
		current = child
		owned = true
	}
	return current, nil
}

// Open returns the descriptor that callers must consume. Intermediate
// directories are permanently anchored before the next segment is examined.
// A regular leaf is lstat/open/fstat/lstat identity-checked with O_NOFOLLOW and
// O_NONBLOCK. When allowDirectory is true, a directory leaf is opened as one
// final child Root and the returned descriptor is opened at that child's dot.
func (w Walker) Open(root *os.Root, name string, allowDirectory bool) (*os.File, error) {
	segments, err := split(name)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		child, expected, err := w.descend(root, ".", ".")
		if err != nil {
			return nil, err
		}
		defer child.Close()
		if !allowDirectory {
			return nil, fmt.Errorf("%s is not a regular file", name)
		}
		return openDirectory(child, name, expected)
	}

	parent := root
	owned := false
	parentName := ""
	for _, segment := range segments[:len(segments)-1] {
		parentName = join(parentName, segment)
		child, _, err := w.descend(parent, segment, parentName)
		if err != nil {
			if owned {
				parent.Close()
			}
			return nil, err
		}
		if owned {
			parent.Close()
		}
		parent = child
		owned = true
	}
	if owned {
		defer parent.Close()
	}

	leaf := segments[len(segments)-1]
	expected, err := parent.Lstat(leaf)
	if err != nil {
		return nil, err
	}
	if expected.Mode()&fs.ModeSymlink != 0 {
		return nil, unsafe(name, "is a symlink")
	}
	if expected.IsDir() {
		if !allowDirectory {
			return nil, fmt.Errorf("%s is not a regular file", name)
		}
		child, opened, err := w.descend(parent, leaf, name)
		if err != nil {
			return nil, err
		}
		defer child.Close()
		return openDirectory(child, name, opened)
	}
	if !expected.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}

	if w.BeforeOpenFile != nil {
		w.BeforeOpenFile(name)
	}
	file, err := parent.OpenFile(leaf, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	confirmed, err := parent.Lstat(leaf)
	if err != nil {
		file.Close()
		return nil, err
	}
	if confirmed.Mode()&fs.ModeSymlink != 0 || !actual.Mode().IsRegular() {
		file.Close()
		return nil, unsafe(name, "changed while it was opened")
	}
	if !os.SameFile(expected, actual) || !os.SameFile(actual, confirmed) {
		file.Close()
		return nil, changedError{name: name}
	}
	return file, nil
}

func (w Walker) descend(parent *os.Root, segment, name string) (*os.Root, fs.FileInfo, error) {
	expected, err := parent.Lstat(segment)
	if err != nil {
		return nil, nil, err
	}
	if expected.Mode()&fs.ModeSymlink != 0 {
		return nil, nil, unsafe(name, "is a symlink")
	}
	if !expected.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", name)
	}
	child, err := parent.OpenRoot(segment)
	if err != nil {
		return nil, nil, err
	}
	opened, err := child.Stat(".")
	if err != nil {
		child.Close()
		return nil, nil, err
	}
	if w.AfterOpenRoot != nil {
		w.AfterOpenRoot(name)
	}
	confirmed, err := parent.Lstat(segment)
	if err != nil {
		child.Close()
		return nil, nil, err
	}
	if confirmed.Mode()&fs.ModeSymlink != 0 ||
		!opened.IsDir() ||
		!os.SameFile(expected, opened) ||
		!os.SameFile(opened, confirmed) {
		child.Close()
		return nil, nil, unsafe(name, "changed while it was opened")
	}
	return child, opened, nil
}

func openDirectory(root *os.Root, name string, expected fs.FileInfo) (*os.File, error) {
	file, err := root.OpenFile(".", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	confirmed, err := root.Lstat(".")
	if err != nil {
		file.Close()
		return nil, err
	}
	if !actual.IsDir() ||
		!os.SameFile(expected, actual) ||
		!os.SameFile(actual, confirmed) {
		file.Close()
		return nil, unsafe(name, "changed while it was opened")
	}
	return file, nil
}

func split(name string) ([]string, error) {
	if name == "." {
		return nil, nil
	}
	if name == "" || filepath.IsAbs(name) {
		return nil, unsafe(name, "is not a canonical relative path")
	}
	slash := filepath.ToSlash(name)
	if filepath.ToSlash(filepath.Clean(name)) != slash || !fs.ValidPath(slash) {
		return nil, unsafe(name, "is not a valid canonical relative path")
	}
	return strings.Split(slash, "/"), nil
}

func join(parent, segment string) string {
	if parent == "" {
		return segment
	}
	return filepath.Join(parent, segment)
}

func unsafe(name, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrUnsafe, name, reason)
}
