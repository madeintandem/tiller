package rootpath

import (
	"container/list"
	"errors"
	"os"
	"path/filepath"
)

// retainedDirectoryLimit bounds how many verified directories a Reader keeps
// open at once, so a read of a very large tree cannot exhaust the process's
// file descriptors. The least recently used directory is closed first.
const retainedDirectoryLimit = 64

// Reader verifies each directory beneath one anchored Root once and keeps it
// open, so a read of many files under the same directories descends each
// directory one time instead of once per file. Every directory is still bound
// segment by segment with Walker's lstat/open/stat/lstat check, and a retained
// directory stays anchored to the object that check confirmed: a later rename
// or symlink swap of its pathname cannot redirect a read made through it. A
// Reader serves one bounded read, such as a tree load, one event listing, or
// one corpus walk. It does not own root; Close releases only the directories
// it opened.
//
// At most retainedDirectoryLimit directories stay open; past that the least
// recently used one is closed and verified again if it is needed later. A Root
// returned by Dir is valid only until the next call on the same Reader, and a
// Reader serves one goroutine at a time.
type Reader struct {
	walk  Walker
	root  *os.Root
	dirs  map[string]*list.Element
	order *list.List // front is most recently used; values are *retainedDirectory
}

type retainedDirectory struct {
	name string
	root *os.Root
}

// NewReader returns a Reader beneath root. Production readers pass Walker{};
// the walker's hooks are the same deterministic test seams Walker documents.
func NewReader(root *os.Root, walk Walker) *Reader {
	return &Reader{walk: walk, root: root, dirs: map[string]*list.Element{}, order: list.New()}
}

// Dir returns the retained Root for a canonical relative directory, verifying
// only the segments below its nearest retained ancestor. Every retained
// directory on the way counts as a use, so a depth-first walk keeps its
// ancestors open. The Root belongs to the Reader; callers must not close it,
// and it is valid only until the next call on the same Reader.
func (r *Reader) Dir(name string) (*os.Root, error) {
	segments, err := split(name)
	if err != nil {
		return nil, err
	}
	current, opened := r.root, ""
	for _, segment := range segments {
		opened = join(opened, segment)
		if element := r.dirs[opened]; element != nil {
			r.order.MoveToFront(element)
			current = element.Value.(*retainedDirectory).root
			continue
		}
		child, _, err := r.walk.descend(current, segment, opened)
		if err != nil {
			return nil, err
		}
		r.dirs[opened] = r.order.PushFront(&retainedDirectory{name: opened, root: child})
		current = child
	}
	// Evict only after the walk, so the Root being returned (the most recently
	// used) and every ancestor it was reached through are still open above.
	// A close error on a read-only directory descriptor is not actionable.
	for r.order.Len() > retainedDirectoryLimit {
		evicted := r.order.Remove(r.order.Back()).(*retainedDirectory)
		delete(r.dirs, evicted.name)
		_ = evicted.root.Close()
	}
	return current, nil
}

// Open opens a canonical relative file through its retained parent with
// Walker.Open's leaf checks. When allowDirectory is true a directory leaf
// opens too, exactly as Walker.Open allows.
func (r *Reader) Open(name string, allowDirectory bool) (*os.File, error) {
	segments, err := split(name)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return r.walk.Open(r.root, ".", allowDirectory)
	}
	parentName := "."
	if len(segments) > 1 {
		parentName = filepath.Join(segments[:len(segments)-1]...)
	}
	parent, err := r.Dir(parentName)
	if err != nil {
		return nil, err
	}
	return r.walk.Open(parent, segments[len(segments)-1], allowDirectory)
}

// Close releases every retained directory.
func (r *Reader) Close() error {
	var closeErr error
	for element := r.order.Front(); element != nil; element = element.Next() {
		closeErr = errors.Join(closeErr, element.Value.(*retainedDirectory).root.Close())
	}
	r.dirs = map[string]*list.Element{}
	r.order.Init()
	return closeErr
}
