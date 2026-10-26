package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// rootOperationHooks are deterministic test seams around descriptor-relative
// mutation and transaction parent retention. Production transactions leave
// them empty.
type rootOperationHooks struct {
	AfterOpenParent     func(operation, target string)
	AfterPreopenParents func(parents []*os.Root)
}

// rootedTransaction keeps one descriptor anchored to the repository root. Every
// mutation opens the target's parent beneath that descriptor and acts on only the
// leaf name, so a later pathname swap cannot redirect the syscall elsewhere.
type rootedTransaction struct {
	root    *os.Root
	walk    rootpath.Walker
	hooks   rootOperationHooks
	mu      sync.Mutex
	parents map[string]*os.Root
}

func openRootedTransaction(root string, hooks rootOperationHooks) (*rootedTransaction, error) {
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &rootedTransaction{root: anchored, hooks: hooks, parents: make(map[string]*os.Root)}, nil
}

func (t *rootedTransaction) close() error {
	t.mu.Lock()
	parents := t.parents
	t.parents = make(map[string]*os.Root)
	t.mu.Unlock()
	var closeErr error
	for _, parent := range parents {
		closeErr = errors.Join(closeErr, parent.Close())
	}
	return errors.Join(closeErr, t.root.Close())
}

// checkCanonicalTarget refuses an absolute, uncleaned, or escaping target
// before any descriptor work, for transactions and RepositoryReader alike.
func checkCanonicalTarget(target string) error {
	if target == "" || filepath.IsAbs(target) || filepath.Clean(target) != target || target == "." ||
		target == ".." || len(target) > 2 && target[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("repository path %q is not a canonical relative path", target)
	}
	return nil
}

func (t *rootedTransaction) withParent(operation, target string, apply func(*os.Root, string) error) (result error) {
	if err := checkCanonicalTarget(target); err != nil {
		return err
	}
	parent, transient, err := t.openParent(filepath.Dir(target))
	if err != nil {
		return err
	}
	if transient {
		defer func() {
			result = errors.Join(result, parent.Close())
		}()
	}
	if t.hooks.AfterOpenParent != nil {
		t.hooks.AfterOpenParent(operation, target)
	}
	return apply(parent, filepath.Base(target))
}

func (t *rootedTransaction) openParent(name string) (*os.Root, bool, error) {
	t.mu.Lock()
	parent := t.parents[name]
	t.mu.Unlock()
	if parent != nil {
		return parent, false, nil
	}
	opened, err := t.walk.OpenRoot(t.root, name)
	if err != nil {
		return nil, false, rootedPathError(err)
	}
	return opened, true, nil
}

func (t *rootedTransaction) retainParent(name string) error {
	t.mu.Lock()
	exists := t.parents[name] != nil
	t.mu.Unlock()
	if exists {
		return nil
	}
	opened, err := t.walk.OpenRoot(t.root, name)
	if err != nil {
		return rootedPathError(err)
	}
	return t.retainDirectory(name, opened)
}

func (t *rootedTransaction) retainDirectory(name string, opened *os.Root) error {
	t.mu.Lock()
	if t.parents[name] == nil {
		t.parents[name] = opened
		opened = nil
	}
	t.mu.Unlock()
	if opened != nil {
		return opened.Close()
	}
	return nil
}

func (t *rootedTransaction) evictDirectory(name string) error {
	t.mu.Lock()
	var evicted []*os.Root
	for cached, root := range t.parents {
		if cached == name || strings.HasPrefix(cached, name+string(filepath.Separator)) {
			evicted = append(evicted, root)
			delete(t.parents, cached)
		}
	}
	t.mu.Unlock()
	var closeErr error
	for _, root := range evicted {
		closeErr = errors.Join(closeErr, root.Close())
	}
	return closeErr
}

func (t *rootedTransaction) preopenParents(targets []string) error {
	seen := make(map[string]struct{})
	for _, target := range targets {
		for parent := filepath.Dir(target); ; parent = filepath.Dir(parent) {
			if _, ok := seen[parent]; ok {
				break
			}
			seen[parent] = struct{}{}
			if err := t.retainParent(parent); err == nil {
				break
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if parent == "." {
				return fs.ErrNotExist
			}
		}
	}
	if t.hooks.AfterPreopenParents != nil {
		t.mu.Lock()
		parents := make([]*os.Root, 0, len(t.parents))
		for _, parent := range t.parents {
			parents = append(parents, parent)
		}
		t.mu.Unlock()
		t.hooks.AfterPreopenParents(parents)
	}
	return nil
}

func (t *rootedTransaction) openTarget(target string, allowDirectory bool) (*os.File, error) {
	var file *os.File
	err := t.withParent("inspect", target, func(parent *os.Root, leaf string) error {
		var err error
		file, err = t.walk.Open(parent, leaf, allowDirectory)
		return err
	})
	if err != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return nil, rootedPathError(err)
	}
	return file, nil
}

func (t *rootedTransaction) openDirectoryRoot(target string) (*os.Root, error) {
	var root *os.Root
	err := t.withParent("inspect", target, func(parent *os.Root, leaf string) error {
		var err error
		root, err = t.walk.OpenRoot(parent, leaf)
		return err
	})
	if err != nil {
		if root != nil {
			err = errors.Join(err, root.Close())
		}
		return nil, rootedPathError(err)
	}
	return root, nil
}

func (t *rootedTransaction) lstat(target string) (fs.FileInfo, error) {
	var info fs.FileInfo
	err := t.withParent("inspect", target, func(parent *os.Root, leaf string) error {
		var err error
		info, err = parent.Lstat(leaf)
		return err
	})
	return info, err
}

func (t *rootedTransaction) syncPath(target string) error {
	if !fsyncEnabled() {
		return nil
	}
	t.mu.Lock()
	directory := t.parents[target]
	t.mu.Unlock()
	if directory != nil {
		return syncRootDirectory(directory)
	}
	file, err := t.openTarget(target, true)
	if err != nil {
		return rootedPathError(err)
	}
	defer file.Close()
	return syncFile(file)
}

func (t *rootedTransaction) readSnapshot(target string, allowDirectory bool) ([]byte, FileSnapshot, error) {
	file, err := t.openTarget(target, allowDirectory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, FileSnapshot{}, nil
	}
	if err != nil {
		return nil, FileSnapshot{}, rootedPathError(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	if info.IsDir() {
		return nil, FileSnapshot{exists: true}, nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	return data, FileSnapshot{exists: true, digest: sha256.Sum256(data)}, nil
}

func (t *rootedTransaction) exists(target string) (bool, error) {
	info, err := t.lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: %s is a symlink", rootpath.ErrUnsafe, target)
	}
	return true, nil
}

func (t *rootedTransaction) directorySnapshot(target string) (DirectorySnapshot, error) {
	file, err := t.openTarget(target, true)
	if errors.Is(err, fs.ErrNotExist) {
		return DirectorySnapshot{}, nil
	}
	if err != nil {
		return DirectorySnapshot{}, rootedPathError(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return DirectorySnapshot{}, err
	}
	if !info.IsDir() {
		return DirectorySnapshot{}, fmt.Errorf("%s is not a directory", target)
	}
	entries, err := file.ReadDir(-1)
	if err != nil {
		return DirectorySnapshot{}, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	sort.Strings(names)
	return DirectorySnapshot{Exists: true, Mode: info.Mode(), Names: names}, nil
}

func (t *rootedTransaction) readLineageInventory(target string) (LineageInventory, error) {
	var files []LineageStateFile
	epics, err := t.readDirectories(target)
	if err != nil {
		return LineageInventory{}, err
	}
	for _, epic := range epics {
		files = append(files, LineageStateFile{
			Path: filepath.Join(epic.Name(), "epic.yml"), Noun: NounEpic,
		})
		features, err := t.readDirectories(filepath.Join(target, epic.Name(), "features"))
		if err != nil {
			return LineageInventory{}, err
		}
		for _, feature := range features {
			featureRoot := filepath.Join(target, epic.Name(), "features", feature.Name())
			files = append(files, LineageStateFile{
				Path: filepath.Join(epic.Name(), "features", feature.Name(), "feature.yml"),
				Noun: NounFeature,
			})
			tasks, err := t.readDirectories(filepath.Join(featureRoot, TaskDirName))
			if err != nil {
				return LineageInventory{}, err
			}
			for _, task := range tasks {
				files = append(files, LineageStateFile{
					Path: filepath.Join(epic.Name(), "features", feature.Name(), TaskDirName, task.Name(), TaskStateFile),
					Noun: NounTask,
				})
			}
		}
	}
	return newLineageInventory(files), nil
}

func (t *rootedTransaction) readDirectories(target string) ([]fs.DirEntry, error) {
	root, err := t.openDirectoryRoot(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, rootedPathError(err)
	}
	entries, readErr := fs.ReadDir(root.FS(), ".")
	closeErr := root.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	directories := entries[:0]
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s is a symlink", rootpath.ErrUnsafe, filepath.Join(target, entry.Name()))
		}
		if entry.IsDir() {
			directories = append(directories, entry)
		}
	}
	return directories, nil
}

func (t *rootedTransaction) writeFileAtomic(target string, data []byte) error {
	return t.writeFileAtomicWithMode(target, data, nil)
}

func (t *rootedTransaction) writeFileAtomicWithMode(target string, data []byte, mode *os.FileMode) error {
	return t.writeAtomicWithMode(target, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}, mode, nil)
}

func (t *rootedTransaction) writeAtomicWithMode(target string, write func(io.Writer) error, mode *os.FileMode, sync func(*os.Root) error) error {
	return t.withParent("write", target, func(parent *os.Root, leaf string) (result error) {
		info, err := parent.Lstat(leaf)
		if err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("symbolic link or nonregular destination: %s", target)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		temp, tempName, err := createRootTemp(parent, leaf)
		if err != nil {
			return err
		}
		defer func() {
			if temp != nil {
				result = errors.Join(result, temp.Close())
			}
			if tempName != "" {
				if removeErr := parent.Remove(tempName); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
					result = errors.Join(result, removeErr)
				}
			}
		}()
		if err := write(temp); err != nil {
			return err
		}
		if mode != nil {
			if err := temp.Chmod(*mode); err != nil {
				return err
			}
		}
		if err := syncFile(temp); err != nil {
			return err
		}
		if err := temp.Close(); err != nil {
			temp = nil
			return err
		}
		temp = nil
		if err := parent.Rename(tempName, leaf); err != nil {
			return err
		}
		tempName = ""
		if sync != nil {
			return sync(parent)
		}
		return syncRootDirectory(parent)
	})
}

func createRootTemp(parent *os.Root, leaf string) (*os.File, string, error) {
	var token [8]byte
	for range 32 {
		if _, err := rand.Read(token[:]); err != nil {
			return nil, "", err
		}
		name := "." + leaf + ".tmp-" + hex.EncodeToString(token[:])
		file, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, name, err
	}
	return nil, "", fmt.Errorf("create temporary file for %s: too many name collisions", leaf)
}

func (t *rootedTransaction) removeFile(target string) error {
	return t.removeFileWithSync(target, true)
}

func (t *rootedTransaction) removeFileWithSync(target string, sync bool) error {
	return t.withParent("remove-file", target, func(parent *os.Root, leaf string) error {
		info, err := parent.Lstat(leaf)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", target)
		}
		if err := parent.Remove(leaf); err != nil {
			return err
		}
		if !sync {
			return nil
		}
		return syncRootDirectory(parent)
	})
}

func (t *rootedTransaction) mkdir(target string, mode os.FileMode) error {
	return t.withParent("mkdir", target, func(parent *os.Root, leaf string) error {
		info, err := parent.Lstat(leaf)
		if err == nil {
			if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("%s is not a directory", target)
			}
			return nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := parent.Mkdir(leaf, mode); err != nil {
			return err
		}
		if err := syncRootDirectory(parent); err != nil {
			return err
		}
		opened, err := t.walk.OpenRoot(parent, leaf)
		if err != nil {
			return rootedPathError(err)
		}
		return t.retainDirectory(target, opened)
	})
}

func (t *rootedTransaction) removeDirectory(target string) error {
	return t.withParent("remove-directory", target, func(parent *os.Root, leaf string) error {
		info, err := parent.Lstat(leaf)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s is not a directory", target)
		}
		if err := parent.Remove(leaf); err != nil {
			return err
		}
		if err := syncRootDirectory(parent); err != nil {
			return err
		}
		return t.evictDirectory(target)
	})
}

func (t *rootedTransaction) chmodRegular(target string, mode os.FileMode) error {
	return t.withParent("chmod-file", target, func(parent *os.Root, leaf string) error {
		expected, err := parent.Lstat(leaf)
		if err != nil {
			return err
		}
		if expected.Mode()&fs.ModeSymlink != 0 || !expected.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", target)
		}
		file, err := parent.OpenFile(leaf, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		confirmed, err := parent.Lstat(leaf)
		if err != nil {
			return err
		}
		if confirmed.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			!os.SameFile(expected, info) || !os.SameFile(info, confirmed) {
			return fmt.Errorf("%s changed while it was opened", target)
		}
		if err := file.Chmod(mode); err != nil {
			return err
		}
		return syncFile(file)
	})
}

func (t *rootedTransaction) chmodDirectory(target string, mode os.FileMode) error {
	return t.withParent("chmod-directory", target, func(parent *os.Root, leaf string) error {
		child, err := t.walk.OpenRoot(parent, leaf)
		if err != nil {
			return rootedPathError(err)
		}
		defer child.Close()
		file, err := child.OpenFile(".", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := file.Chmod(mode); err != nil {
			return err
		}
		return syncFile(file)
	})
}

func syncRootDirectory(root *os.Root) error {
	if !fsyncEnabled() {
		return nil
	}
	file, err := root.OpenFile(".", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return syncFile(file)
}

func rootedPathError(err error) error {
	if errors.Is(err, rootpath.ErrUnsafe) {
		return fmt.Errorf("symbolic link or changed path identity: %w", err)
	}
	return err
}

func recoverJournalWithRootOperationHooks(root string, journal *journal, hooks rootOperationHooks) (result error) {
	rooted, err := openRootedTransaction(root, hooks)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, rooted.close())
	}()
	return recoverJournalRooted(rooted, journal)
}

func recoverJournalRooted(rooted *rootedTransaction, journal *journal) error {
	return recoverJournalRootedWithIO(rooted, journal, nil, nil)
}

// Optional IO callbacks are test-only failure-injection seams; production
// recovery passes nil. A non-nil write restores preimages through a plain
// pathname, so it must never reach a production caller. All inspection,
// directory mutation and mode changes still use the retained descriptors.
func recoverJournalRootedWithIO(rooted *rootedTransaction, journal *journal, write func(string, []byte) error, sync func(string) error) error {
	syncAfter := func(target string) error {
		if sync == nil {
			return nil
		}
		if target == "." {
			return sync(rooted.root.Name())
		}
		_, err := rooted.lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return sync(filepath.Join(rooted.root.Name(), target))
	}
	var recoveryErr error
	removed := append([]journalRemovedDirectory(nil), journal.RemovedDirs...)
	sort.SliceStable(removed, func(i, j int) bool {
		return strings.Count(removed[i].Path, string(filepath.Separator)) <
			strings.Count(removed[j].Path, string(filepath.Separator))
	})
	for _, directory := range removed {
		info, err := rooted.lstat(directory.Path)
		if errors.Is(err, fs.ErrNotExist) {
			err = rooted.mkdir(directory.Path, 0o700)
		} else if err == nil && (info.Mode()&fs.ModeSymlink != 0 || !info.IsDir()) {
			err = fmt.Errorf("%s is not a directory", directory.Path)
		}
		if err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("recreate removed directory %s: %w", directory.Path, err))
			continue
		}
		stagingMode := chmodModeBits(os.FileMode(directory.Mode)) | 0o300
		if err := rooted.chmodDirectory(directory.Path, stagingMode); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("make removed directory writable %s: %w", directory.Path, err))
		}
		if err := syncAfter(directory.Path); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync staged directory mode for %s: %w", directory.Path, err))
		}
		if err := syncAfter(filepath.Dir(directory.Path)); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync recreated directory %s: %w", directory.Path, err))
		}
	}

	for _, relative := range journal.CreatedDirs {
		info, err := rooted.lstat(relative)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("inspect created directory %s: %w", relative, err))
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("created directory %s changed kind during recovery", relative))
			continue
		}
		if err := rooted.chmodDirectory(relative, 0o700); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("make created directory %s removable: %w", relative, err))
		}
	}

	for i := len(journal.Preimages) - 1; i >= 0; i-- {
		preimage := journal.Preimages[i]
		if preimage.Existed {
			mode := os.FileMode(preimage.Mode)
			var err error
			if write == nil {
				err = rooted.writeFileAtomicWithMode(preimage.Path, preimage.Data, &mode)
			} else if _, pathErr := rooted.exists(preimage.Path); pathErr != nil {
				err = pathErr
			} else {
				err = write(filepath.Join(rooted.root.Name(), preimage.Path), preimage.Data)
				if err == nil {
					err = rooted.chmodRegular(preimage.Path, mode)
				}
			}
			if err != nil {
				recoveryErr = errors.Join(recoveryErr, fmt.Errorf("restore %s: %w", preimage.Path, err))
				continue
			}
			if err := syncAfter(preimage.Path); err != nil {
				recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync restored file mode for %s: %w", preimage.Path, err))
			}
			if err := syncAfter(filepath.Dir(preimage.Path)); err != nil {
				recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync restored destination %s: %w", preimage.Path, err))
			}
			continue
		}
		if err := rooted.removeFile(preimage.Path); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("remove %s: %w", preimage.Path, err))
		}
		if err := syncAfter(filepath.Dir(preimage.Path)); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync removed destination %s: %w", preimage.Path, err))
		}
	}

	for i := len(journal.CreatedDirs) - 1; i >= 0; i-- {
		if err := rooted.removeDirectory(journal.CreatedDirs[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("remove created directory %s: %w", journal.CreatedDirs[i], err))
		}
		if err := syncAfter(filepath.Dir(journal.CreatedDirs[i])); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync removed directory %s: %w", journal.CreatedDirs[i], err))
		}
	}
	for i := len(removed) - 1; i >= 0; i-- {
		directory := removed[i]
		originalMode := chmodModeBits(os.FileMode(directory.Mode))
		if originalMode|0o300 == originalMode {
			continue
		}
		if err := rooted.chmodDirectory(directory.Path, originalMode); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("restore mode for directory %s: %w", directory.Path, err))
		}
		if err := syncAfter(directory.Path); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("sync restored directory mode for %s: %w", directory.Path, err))
		}
	}
	return recoveryErr
}
