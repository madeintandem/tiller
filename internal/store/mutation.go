package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/madeintandem/tiller/internal/rootpath"
)

const (
	ReasonMutationStale    = "mutation_stale"
	ReasonMutationRecovery = "mutation_recovery_needed"
)

// FileSnapshot identifies the exact contents expected at a planned write's
// destination. An absent file is a valid snapshot.
type FileSnapshot struct {
	exists bool
	digest [sha256.Size]byte
}

func (s FileSnapshot) Exists() bool { return s.exists }

func AbsentSnapshot() FileSnapshot { return FileSnapshot{} }

// ReadGuardedFileSnapshot reads a regular file through an anchored path walk,
// refusing symlink components and nonregular leaves without opening them. A
// missing path, including a missing intermediate directory, is an absent
// snapshot. A directory leaf is represented as an existing snapshot so an
// absent destination guard detects it without reading directory bytes.
func ReadGuardedFileSnapshot(root, path string) ([]byte, FileSnapshot, error) {
	return readAnchoredSnapshot(root, path, true)
}

// ReadRegularFileSnapshot reads the file a planned write will replace, path
// being absolute beneath root. A missing file is an absent snapshot.
// O_NOFOLLOW and O_NONBLOCK ensure a symlink or FIFO swap cannot redirect or
// stall the snapshot read.
func ReadRegularFileSnapshot(root, path string) ([]byte, FileSnapshot, error) {
	return readAnchoredSnapshot(root, path, false)
}

// PathExistsNoSymlinks reports whether path exists beneath root without opening
// or reading the leaf. Intermediate directories are anchored one segment at a
// time, and intermediate or final symlinks are refused.
func PathExistsNoSymlinks(root, path string) (bool, error) {
	return pathExistsNoSymlinksWithWalker(root, path, rootpath.Walker{})
}

func pathExistsNoSymlinksWithWalker(root, path string, walker rootpath.Walker) (bool, error) {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return false, err
	}
	if relative == "." {
		return true, nil
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer anchored.Close()
	parent := anchored
	if parentName := filepath.Dir(relative); parentName != "." {
		parent, err = walker.OpenRoot(anchored, parentName)
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		defer parent.Close()
	}
	info, err := parent.Lstat(filepath.Base(relative))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: %s is a symlink", rootpath.ErrUnsafe, relative)
	}
	return true, nil
}

// snapshotWalker is the walker every planning snapshot read descends with.
var snapshotWalker = rootpath.Walker{}

func readAnchoredSnapshot(root, path string, allowDirectory bool) ([]byte, FileSnapshot, error) {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	defer anchored.Close()
	file, err := snapshotWalker.Open(anchored, relative, allowDirectory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, FileSnapshot{}, nil
	}
	if errors.Is(err, rootpath.ErrChanged) {
		// Planning reads take no lock, so a concurrent writer can replace the
		// file mid-open; the plan is stale and a retry is safe (as #151 ruled
		// for the generation read).
		return nil, FileSnapshot{}, mutationEnvError(ReasonMutationStale,
			"another Tiller mutation changed %s while it was being read; retry", relative)
	}
	if err != nil {
		return nil, FileSnapshot{}, err
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

// DirectorySnapshot identifies the exact existence, mode, and entry names
// expected at a planned directory removal. An absent directory is valid.
type DirectorySnapshot struct {
	Exists bool
	Mode   os.FileMode
	Names  []string
}

// ReadDirectorySnapshot reads the directory at path, absolute beneath root,
// without following symlinks. A missing directory is an absent snapshot.
func ReadDirectorySnapshot(root, path string) (DirectorySnapshot, error) {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return DirectorySnapshot{}, err
	}
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	if err != nil {
		return DirectorySnapshot{}, err
	}
	defer rooted.close()
	return rooted.directorySnapshot(relative)
}

func directorySnapshotsEqual(left, right DirectorySnapshot) bool {
	if left.Exists != right.Exists || left.Mode != right.Mode || len(left.Names) != len(right.Names) {
		return false
	}
	for i := range left.Names {
		if left.Names[i] != right.Names[i] {
			return false
		}
	}
	return true
}

// LineageStateFile is one epic, feature, or task state file in the guarded
// node inventory. Path is relative to the .tiller/epics directory passed to
// ReadLineageInventory.
type LineageStateFile struct {
	Path string
	Noun string
}

// LineageInventory is the set of epic, feature, and task state files represented
// by the current directory layout. Its digest is private so callers consume the
// typed file list while Commit owns snapshot comparison.
type LineageInventory struct {
	Files  []LineageStateFile
	digest [sha256.Size]byte
}

// ReadLineageInventory enumerates epic, feature, and task state paths claimed by
// the directories beneath epicsDir, absolute beneath root. Missing lazy feature
// and task directories contribute no files.
func ReadLineageInventory(root, epicsDir string) (LineageInventory, error) {
	relative, err := RepositoryRelativePath(root, epicsDir)
	if err != nil {
		return LineageInventory{}, err
	}
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	if err != nil {
		return LineageInventory{}, err
	}
	defer rooted.close()
	return rooted.readLineageInventory(relative)
}

func newLineageInventory(files []LineageStateFile) LineageInventory {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == files[j].Path {
			return files[i].Noun < files[j].Noun
		}
		return files[i].Path < files[j].Path
	})
	hash := sha256.New()
	for _, file := range files {
		_, _ = hash.Write([]byte(file.Noun))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.Path))
		_, _ = hash.Write([]byte{0})
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return LineageInventory{Files: files, digest: digest}
}

// plannedWriteOperation distinguishes file writes and guards from destructive
// operations without changing PlannedWrite's public construction surface.
type plannedWriteOperation uint8

const (
	plannedWriteFile plannedWriteOperation = iota
	plannedReadFile
	plannedDeleteFile
	plannedReadLineage
	plannedRemoveDirectory
	plannedReadDirectory
	plannedSetCreatedDirectoryMode
)

func (operation plannedWriteOperation) mutatesFilesystem() bool {
	switch operation {
	case plannedWriteFile, plannedDeleteFile, plannedRemoveDirectory, plannedSetCreatedDirectoryMode:
		return true
	default:
		return false
	}
}

// PlannedWrite couples one mutation or guard with the exact state on which it
// was planned. The zero operation remains a file write for source compatibility.
type PlannedWrite struct {
	Path             string
	Data             []byte
	Before           FileSnapshot
	operation        plannedWriteOperation
	directoryBefore  DirectorySnapshot
	lineageInventory *[sha256.Size]byte
	directoryGuard   *DirectorySnapshot
	beforeMode       os.FileMode
	writeMode        os.FileMode
	checkBeforeMode  bool
	setWriteMode     bool
}

func chmodModeBits(mode os.FileMode) os.FileMode {
	return mode & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
}

func PlanWrite(path string, before FileSnapshot, data []byte) PlannedWrite {
	return PlannedWrite{Path: path, Before: before, Data: data}
}

// PlanWriteWithMode writes a regular file and applies the supplied permission
// bits before the journal is cleared.
func PlanWriteWithMode(path string, before FileSnapshot, data []byte, mode os.FileMode) PlannedWrite {
	return PlannedWrite{
		Path: path, Before: before, Data: data,
		writeMode: mode.Perm(), setWriteMode: true,
	}
}

func PlanReadGuard(path string, before FileSnapshot) PlannedWrite {
	return PlannedWrite{Path: path, Before: before, operation: plannedReadFile}
}

// PlanReadGuardWithMode retains the regular file's permission precondition too.
func PlanReadGuardWithMode(path string, before FileSnapshot, mode os.FileMode) PlannedWrite {
	return PlannedWrite{Path: path, Before: before, operation: plannedReadFile, beforeMode: mode.Perm(), checkBeforeMode: true}
}

// ConstrainByReadGuard merges an earlier validation observation into a later
// write plan without rebasing its preimage. Read and write may share a path,
// but they must describe the same original file or directory.
func (p PlannedWrite) ConstrainByReadGuard(guard PlannedWrite) (PlannedWrite, error) {
	stale := func() (PlannedWrite, error) {
		return PlannedWrite{}, mutationEnvError(ReasonMutationStale, "%s changed after validation; reload and retry", p.Path)
	}
	if p.Path != guard.Path {
		return stale()
	}
	switch guard.operation {
	case plannedReadFile:
		if p.Before != guard.Before {
			return stale()
		}
		if guard.checkBeforeMode {
			if p.checkBeforeMode && p.beforeMode != guard.beforeMode {
				return stale()
			}
			p.checkBeforeMode = true
			p.beforeMode = guard.beforeMode
		}
	case plannedReadDirectory:
		switch p.operation {
		case plannedReadDirectory, plannedRemoveDirectory, plannedSetCreatedDirectoryMode:
			if !directorySnapshotsEqual(p.directoryBefore, guard.directoryBefore) {
				return stale()
			}
		case plannedReadLineage:
			snapshot := guard.directoryBefore
			p.directoryGuard = &snapshot
		default:
			return stale()
		}
	default:
		return stale()
	}
	return p, nil
}

func PlanDeleteFile(path string, before FileSnapshot) PlannedWrite {
	return PlannedWrite{Path: path, Before: before, operation: plannedDeleteFile}
}

// PlanDeleteFileWithMode deletes a regular file only if both its content and
// permission bits still match the state observed while planning.
func PlanDeleteFileWithMode(path string, before FileSnapshot, mode os.FileMode) PlannedWrite {
	return PlannedWrite{
		Path: path, Before: before, operation: plannedDeleteFile,
		beforeMode: mode.Perm(), checkBeforeMode: true,
	}
}

func PlanRemoveEmptyDir(path string, before DirectorySnapshot) PlannedWrite {
	before.Names = append([]string(nil), before.Names...)
	return PlannedWrite{Path: path, operation: plannedRemoveDirectory, directoryBefore: before}
}

// PlanDirectoryGuard validates a directory's exact existence, mode, and entry
// names under the mutation lock without removing it.
func PlanDirectoryGuard(path string, before DirectorySnapshot) PlannedWrite {
	before.Names = append([]string(nil), before.Names...)
	return PlannedWrite{Path: path, operation: plannedReadDirectory, directoryBefore: before}
}

// PlanSetCreatedDirectoryMode ensures a transaction-created destination
// directory exists and applies its chmod-supported mode bits before the journal
// is cleared. The directory must be absent when the transaction validates its plan.
func PlanSetCreatedDirectoryMode(path string, mode os.FileMode) PlannedWrite {
	return PlannedWrite{
		Path: path, operation: plannedSetCreatedDirectoryMode,
		directoryBefore: DirectorySnapshot{}, writeMode: chmodModeBits(mode),
	}
}

// PlanLineageInventoryGuard validates the epic/feature/task directory inventory
// under the mutation lock without adding a journal preimage or destination.
func PlanLineageInventoryGuard(epicsDir string, before LineageInventory) PlannedWrite {
	digest := before.digest
	return PlannedWrite{
		Path:             epicsDir,
		operation:        plannedReadLineage,
		lineageInventory: &digest,
	}
}
