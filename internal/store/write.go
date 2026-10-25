package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// This file holds the write side of the store: atomic per-file writes, the
// repository mutation lock, and the operation journal under .tiller/.cache/
// (cli-surface.md, Working-tree posture). A single-file write is atomic on its
// own (temp then rename); a multi-file mutation takes the lock, journals its
// intended file set, applies, and clears the journal.

// ReasonLocked is the EnvError discriminator for a refused mutation: another
// Tiller mutation already holds the repository lock.
const ReasonLocked = "mutation_locked"

// cacheDirName and the file names under it. The cache is generated and
// git-ignored (artifact-schema.md, Directory layout).
const (
	cacheDirName       = ".cache"
	lockFileName       = "lock"
	journalName        = "journal.yml"
	generationFileName = "generation"
)

// The task folder's name under a feature and the two files inside it
// (artifact-schema.md, Directory layout). Exported so the loader, the writers,
// and the generated views all name the same paths.
const (
	TaskDirName   = "tasks"
	TaskProseFile = "task.md"
	TaskStateFile = "task.yml"
)

// TaskRelDir returns the repo-relative folder holding one task's task.md and
// task.yml. Each segment is the LOCAL key at its grain, not the canonical key
// (artifact-schema.md, Keys and paths), so a caller holding a canonical key
// passes its last segment.
func TaskRelDir(epicKey, featureKey, taskKey string) string {
	return filepath.Join(".tiller", "epics", epicKey, "features", featureKey, TaskDirName, taskKey)
}

// writeAtomicWithDirectorySync writes path through a synced temp file, renames
// it over path, and syncs the parent directory. It is pathname-based and
// follows symlinks, so it serves only unanchored test journals and the commit
// failure-injection seams. Repository files go through WriteRepositoryFileAtomic.
func writeAtomicWithDirectorySync(
	path string,
	write func(io.Writer) error,
	sync func(string) error,
) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tiller-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if err := write(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := syncFile(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return sync(dir)
}

// syncDirectory is the pathname-based directory sync paired with
// writeAtomicWithDirectorySync; rooted IO syncs through rootedTransaction.syncPath.
func syncDirectory(path string) error {
	if !fsyncEnabled() {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if syncErr := dir.Sync(); syncErr != nil {
		if closeErr := dir.Close(); closeErr != nil {
			return errors.Join(syncErr, closeErr)
		}
		return syncErr
	}
	return dir.Close()
}

// journalPreimage is the durable prior state of one planned destination.
type journalPreimage struct {
	Path    string `yaml:"path"`
	Existed bool   `yaml:"existed"`
	Mode    uint32 `yaml:"mode,omitempty"`
	Data    []byte `yaml:"data,omitempty"`
}

type journalRemovedDirectory struct {
	Path string `yaml:"path"`
	Mode uint32 `yaml:"mode"`
}

// journal is the operation journal for a multi-file mutation. Preimages make
// an interrupted mutation recoverable even when in-process rollback fails.
type journal struct {
	Operation   string                    `yaml:"operation"`
	StartedAt   time.Time                 `yaml:"started_at"`
	Files       []string                  `yaml:"files"`
	Preimages   []journalPreimage         `yaml:"preimages,omitempty"`
	CreatedDirs []string                  `yaml:"created_dirs,omitempty"`
	RemovedDirs []journalRemovedDirectory `yaml:"removed_dirs,omitempty"`

	// Runtime-only anchors keep journal IO on the transaction's cache directory.
	path   string
	rooted *rootedTransaction
}

func (j *journal) checkpoint() error {
	if j.rooted == nil {
		return j.checkpointWithDirectorySync(syncDirectory)
	}
	return j.checkpointWithDirectorySync(func(path string) error {
		relative, err := j.rooted.relativePath(path)
		if err != nil {
			return err
		}
		return j.rooted.syncPath(relative)
	})
}

func (j *journal) checkpointWithDirectorySync(sync func(string) error) error {
	data, err := yaml.Marshal(j)
	if err != nil {
		return err
	}
	if j.rooted != nil {
		relative, err := j.rooted.relativePath(j.path)
		if err != nil {
			return err
		}
		return j.rooted.writeAtomicWithMode(relative, func(w io.Writer) error {
			_, err := w.Write(data)
			return err
		}, nil, func(*os.Root) error { return sync(filepath.Dir(j.path)) })
	}
	return writeAtomicWithDirectorySync(j.path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}, sync)
}

// withLock is the low-level lock seam retained for store tests. Production
// mutations use Commit or CommitDerived so snapshots, journaling, rollback, and
// generation checks cannot be bypassed.
func (t *Tree) withLock(fn func(*journal) error) (result error) {
	rooted, err := openRootedTransaction(t.Root, rootOperationHooks{})
	if err != nil {
		return err
	}
	defer rooted.close()
	sync := func(path string) error {
		relative, err := rooted.relativePath(path)
		if err != nil {
			return err
		}
		return rooted.syncPath(relative)
	}
	currentGeneration, release, err := t.acquireMutationLock(rooted)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			result = joinMutationErrors(result, releaseErr)
		}
	}()

	if err := t.refuseRetainedJournal(rooted); err != nil {
		return err
	}

	if currentGeneration != t.generation {
		return mutationEnvError(
			ReasonMutationStale,
			"Tiller state changed after it was loaded; reload and retry",
		)
	}

	token, err := mutationToken()
	if err != nil {
		return mutationEnvError("", "create mutation generation: %v", err)
	}
	if err := rooted.writeFileAtomic(filepath.Join(".tiller", cacheDirName, generationFileName), []byte(token)); err != nil {
		return mutationEnvError("", "write mutation generation: %v", err)
	}
	t.generation = token

	journalPath := t.journalPath()
	j := &journal{StartedAt: time.Now().UTC(), path: journalPath, rooted: rooted}
	defer func() {
		if removeErr := clearJournalDurably(j, sync, restoreJournalCheckpoint); removeErr != nil {
			result = joinMutationErrors(result, mutationEnvError(
				ReasonMutationRecovery,
				"clear mutation journal %s: %v",
				journalPath,
				removeErr,
			))
		}
	}()

	return fn(j)
}

func mutationToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (t *Tree) cachePath(name string) string {
	return filepath.Join(t.Root, ".tiller", cacheDirName, name)
}

func (t *Tree) generationPath() string { return t.cachePath(generationFileName) }
func (t *Tree) journalPath() string    { return t.cachePath(journalName) }
func (t *Tree) lockPath() string       { return t.cachePath(lockFileName) }

func (t *Tree) refuseRetainedJournal(rooted *rootedTransaction) error {
	exists, err := rooted.exists(filepath.Join(".tiller", cacheDirName, journalName))
	if err != nil {
		return mutationEnvError("", "inspect mutation journal %s: %v", t.journalPath(), err)
	}
	if exists {
		return mutationEnvError(
			ReasonMutationRecovery,
			"unfinished Tiller mutation requires recovery (.tiller/.cache/journal.yml is present)",
		)
	}
	return nil
}

func mutationEnvError(reason, format string, args ...any) *EnvError {
	return &EnvError{Code: 3, Reason: reason, Msg: fmt.Sprintf(format, args...)}
}

func lockedEnvError(msg string) *EnvError {
	return mutationEnvError(ReasonLocked, "%s", msg)
}

func joinMutationErrors(primary, secondary error) error {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return errors.Join(primary, secondary)
}

// removeIfExists and pathExists are the pathname-based journal operations for
// unanchored test journals; production journals use their rooted transaction.
func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// acquireMutationLock takes the repository lock without blocking and returns
// the source generation observed only after ownership was established.
func (t *Tree) acquireMutationLock(rooted *rootedTransaction) (generation string, release func() error, result error) {
	cacheDir := filepath.Join(".tiller", cacheDirName)
	if err := rooted.mkdirAll(cacheDir, 0o755); err != nil {
		return "", nil, mutationEnvError("", "create mutation cache %s: %v", cacheDir, err)
	}

	lockPath := t.lockPath()
	parent, transient, err := rooted.openParent(cacheDir)
	if err != nil {
		return "", nil, err
	}
	if transient {
		// release removes the lock through parent after this function returns,
		// so parent must be the retained handle mkdirAll leaves behind.
		return "", nil, errors.Join(
			mutationEnvError("", "mutation cache %s was not retained by its transaction", cacheDir),
			parent.Close())
	}
	lock, err := parent.OpenFile(lockFileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", nil, lockedEnvError("another Tiller mutation is in progress (.tiller/.cache/lock is held)")
		}
		return "", nil, mutationEnvError("", "acquire mutation lock %s: %v", lockPath, err)
	}

	cleanupLock := func() error {
		if err := parent.Remove(lockFileName); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return mutationEnvError(ReasonLocked, "release mutation lock %s: %v", lockPath, err)
		}
		return nil
	}
	failLock := func(cause error) (string, func() error, error) {
		closeErr := lock.Close()
		removeErr := cleanupLock()
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close mutation lock: %w", closeErr))
		}
		if removeErr != nil {
			cause = errors.Join(cause, removeErr)
		}
		return "", nil, mutationEnvError("", "initialize mutation lock %s: %v", lockPath, cause)
	}

	if _, err := fmt.Fprintf(lock, "pid: %d\nat: %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return failLock(err)
	}
	if err := syncFile(lock); err != nil {
		return failLock(err)
	}
	if err := lock.Close(); err != nil {
		if removeErr := cleanupLock(); removeErr != nil {
			err = errors.Join(err, removeErr)
		}
		return "", nil, mutationEnvError("", "close mutation lock %s: %v", lockPath, err)
	}

	data, _, err := rooted.readSnapshot(filepath.Join(cacheDir, generationFileName), false)
	generation = string(data)
	if err != nil {
		if removeErr := cleanupLock(); removeErr != nil {
			err = errors.Join(err, removeErr)
		}
		return "", nil, mutationEnvError("", "read mutation generation: %v", err)
	}
	return generation, cleanupLock, nil
}

type preparedWrite struct {
	planned  PlannedWrite
	relative string
	preimage journalPreimage
}

func validatePlannedBeforeMode(rooted *rootedTransaction, item preparedWrite) error {
	if !item.planned.checkBeforeMode {
		return nil
	}
	info, err := rooted.lstat(item.relative)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != item.planned.beforeMode.Perm() {
		return mutationEnvError(
			ReasonMutationStale,
			"%s permission mode changed after the mutation was planned; reload and retry",
			item.relative,
		)
	}
	return nil
}

func (t *Tree) prepareWritePaths(writes []PlannedWrite) ([]preparedWrite, error) {
	seen := make(map[string]struct{}, len(writes))
	prepared := make([]preparedWrite, 0, len(writes))
	for _, write := range writes {
		if write.Path == "" {
			return nil, mutationEnvError("", "planned write path is empty")
		}
		absolute, err := filepath.Abs(write.Path)
		if err != nil {
			return nil, mutationEnvError("", "resolve planned write %q: %v", write.Path, err)
		}
		relative, err := RepositoryRelativePath(t.Root, absolute)
		if err != nil {
			return nil, mutationEnvError("", "resolve planned write: %v", err)
		}
		if _, duplicate := seen[absolute]; duplicate {
			return nil, mutationEnvError("", "planned write destination %q appears more than once", write.Path)
		}
		seen[absolute] = struct{}{}
		write.Path = absolute
		prepared = append(prepared, preparedWrite{planned: write, relative: relative})
	}
	return prepared, nil
}

func (t *Tree) transactionCreatedDirectories(rooted *rootedTransaction, prepared []preparedWrite) ([]string, []string, error) {
	root, err := filepath.Abs(t.Root)
	if err != nil {
		return nil, nil, mutationEnvError("", "resolve repository root %q: %v", t.Root, err)
	}

	seen := make(map[string]struct{})
	var absoluteDirs []string
	var relativeDirs []string
	for _, item := range prepared {
		var dir string
		switch item.planned.operation {
		case plannedWriteFile:
			dir = filepath.Dir(item.planned.Path)
		case plannedSetCreatedDirectoryMode:
			dir = item.planned.Path
		default:
			continue
		}
		var missing []string
		for ; dir != root; dir = filepath.Dir(dir) {
			relative, err := filepath.Rel(root, dir)
			if err != nil {
				return nil, nil, mutationEnvError("", "resolve created directory %s: %v", dir, err)
			}
			info, err := rooted.lstat(relative)
			if err == nil {
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return nil, nil, mutationEnvError("", "planned destination parent %s is not a directory", dir)
				}
				break
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, mutationEnvError("", "inspect planned destination parent %s: %v", dir, err)
			}
			missing = append(missing, dir)
		}

		for i := len(missing) - 1; i >= 0; i-- {
			dir := missing[i]
			if _, exists := seen[dir]; exists {
				continue
			}
			relative, err := filepath.Rel(root, dir)
			if err != nil {
				return nil, nil, mutationEnvError("", "resolve created directory %s: %v", dir, err)
			}
			seen[dir] = struct{}{}
			absoluteDirs = append(absoluteDirs, dir)
			relativeDirs = append(relativeDirs, relative)
		}
	}
	return absoluteDirs, relativeDirs, nil
}

func (t *Tree) Commit(operation string, writes []PlannedWrite) error {
	return t.commitWithDestinationWriter(operation, writes, true, nil)
}

// CommitChecked revalidates external read-only prerequisites under the source
// mutation lock before any destination writes. Git is not locked by this lock:
// callers must not claim this is a transaction with external Git operations.
func (t *Tree) CommitChecked(operation string, writes []PlannedWrite, check func() error) error {
	return t.commitWithDestinationWriterSyncRestoreAndRootHooks(
		operation, writes, true, nil, nil, restoreJournalCheckpoint, rootOperationHooks{}, check,
	)
}

func (t *Tree) CommitDerived(operation string, writes []PlannedWrite) error {
	return t.commitWithDestinationWriter(operation, writes, false, nil)
}

func (t *Tree) commitWithDestinationWriter(
	operation string,
	writes []PlannedWrite,
	advanceGeneration bool,
	write func(string, []byte) error,
) error {
	var sync func(string) error
	if write != nil {
		sync = syncDirectory
	}
	return t.commitWithDestinationWriterAndSync(
		operation,
		writes,
		advanceGeneration,
		write,
		sync,
	)
}

func (t *Tree) commitWithDestinationWriterAndSync(
	operation string,
	writes []PlannedWrite,
	advanceGeneration bool,
	write func(string, []byte) error,
	sync func(string) error,
) error {
	return t.commitWithDestinationWriterSyncAndRestore(
		operation,
		writes,
		advanceGeneration,
		write,
		sync,
		restoreJournalCheckpoint,
	)
}

func (t *Tree) commitWithDestinationWriterSyncAndRestore(
	operation string,
	writes []PlannedWrite,
	advanceGeneration bool,
	write func(string, []byte) error,
	sync func(string) error,
	restore func(*journal, func(string) error) error,
) error {
	return t.commitWithDestinationWriterSyncRestoreAndRootHooks(
		operation, writes, advanceGeneration, write, sync, restore, rootOperationHooks{},
	)
}

func (t *Tree) commitWithRootOperationHooks(
	operation string,
	writes []PlannedWrite,
	hooks rootOperationHooks,
) error {
	return t.commitWithDestinationWriterSyncRestoreAndRootHooks(
		operation, writes, true, nil, nil, restoreJournalCheckpoint, hooks,
	)
}

func (t *Tree) commitWithDestinationWriterSyncRestoreAndRootHooks(
	operation string,
	writes []PlannedWrite,
	advanceGeneration bool,
	write func(string, []byte) error,
	sync func(string) error,
	restore func(*journal, func(string) error) error,
	hooks rootOperationHooks,
	checks ...func() error,
) (result error) {
	prepared, err := t.prepareWritePaths(writes)
	if err != nil {
		return err
	}
	rooted, err := openRootedTransaction(t.Root, hooks)
	if err != nil {
		return mutationEnvError("", "open repository root %s: %v", t.Root, err)
	}
	defer func() {
		if closeErr := rooted.close(); closeErr != nil {
			result = joinMutationErrors(result, closeErr)
		}
	}()
	var parentTargets []string
	for i := range prepared {
		if prepared[i].planned.operation.mutatesFilesystem() {
			parentTargets = append(parentTargets, prepared[i].relative)
		}
	}
	if err := rooted.preopenParents(parentTargets); err != nil {
		return mutationEnvError("", "open planned transaction parents: %v", err)
	}
	// write and sync are failure-injection seams for tests. Production callers
	// pass nil for both. A non-nil write goes through a plain pathname outside
	// the rooted transaction, so never wire one to a production caller.
	useRootedRecovery := write == nil && sync == nil
	writeDestination := func(path string, data []byte) error {
		if write != nil {
			return write(path, data)
		}
		relative, err := rooted.relativePath(path)
		if err != nil {
			return err
		}
		return rooted.writeFileAtomic(relative, data)
	}
	syncDestination := func(path string) error {
		if sync != nil {
			return sync(path)
		}
		relative, err := rooted.relativePath(path)
		if err != nil {
			return err
		}
		return rooted.syncPath(relative)
	}

	currentGeneration, release, err := t.acquireMutationLock(rooted)
	if err != nil {
		return err
	}
	defer func() {
		if recoveryMarkerMissing(result, rooted, t.journalPath()) {
			return
		}
		if releaseErr := release(); releaseErr != nil {
			result = joinMutationErrors(result, releaseErr)
		}
	}()

	if err := t.refuseRetainedJournal(rooted); err != nil {
		return err
	}

	if currentGeneration != t.generation {
		return mutationEnvError(
			ReasonMutationStale,
			"Tiller state changed after it was loaded; reload and retry",
		)
	}

	for _, item := range prepared {
		if guard := item.planned.directoryGuard; guard != nil {
			current, err := rooted.directorySnapshot(item.relative)
			if err != nil {
				return mutationEnvError("", "read directory guard %s: %v", item.relative, err)
			}
			if !directorySnapshotsEqual(current, *guard) {
				return mutationEnvError(ReasonMutationStale, "%s changed after validation; reload and retry", item.relative)
			}
		}
		switch item.planned.operation {
		case plannedReadLineage:
			current, err := rooted.readLineageInventory(item.relative)
			if err != nil {
				return mutationEnvError("", "read lineage inventory %s: %v", item.planned.Path, err)
			}
			if current.digest != *item.planned.lineageInventory {
				return mutationEnvError(
					ReasonMutationStale,
					"%s lineage inventory changed after the mutation was planned; reload and retry",
					item.relative,
				)
			}
		case plannedRemoveDirectory, plannedReadDirectory, plannedSetCreatedDirectoryMode:
			current, err := rooted.directorySnapshot(item.relative)
			if err != nil {
				return mutationEnvError("", "read planned directory %s: %v", item.planned.Path, err)
			}
			if !directorySnapshotsEqual(current, item.planned.directoryBefore) {
				return mutationEnvError(
					ReasonMutationStale,
					"%s changed after the mutation was planned; reload and retry",
					item.relative,
				)
			}
		default:
			_, current, err := readPlannedFileSnapshotRooted(rooted, item)
			if err != nil {
				if item.planned.Before.Exists() {
					return mutationEnvError(
						ReasonMutationStale,
						"%s changed kind or identity after the mutation was planned; reload and retry: %v",
						item.relative,
						err,
					)
				}
				return mutationEnvError("", "inspect planned destination %s: %v", item.planned.Path, err)
			}
			if current != item.planned.Before {
				return mutationEnvError(
					ReasonMutationStale,
					"%s changed after the mutation was planned; reload and retry",
					item.relative,
				)
			}
			if err := validatePlannedBeforeMode(rooted, item); err != nil {
				return err
			}
		}
	}

	for _, check := range checks {
		if check != nil {
			if err := check(); err != nil {
				return err
			}
		}
	}

	mutations := prepared[:0]
	for _, item := range prepared {
		if item.planned.operation.mutatesFilesystem() {
			mutations = append(mutations, item)
		}
	}
	prepared = mutations
	if len(prepared) == 0 {
		return nil
	}

	files := make([]preparedWrite, 0, len(prepared))
	removedDirectories := make([]preparedWrite, 0, len(prepared))
	createdDirectoryModes := make([]preparedWrite, 0, len(prepared))
	for i := range prepared {
		item := &prepared[i]
		if item.planned.operation == plannedRemoveDirectory {
			current, err := rooted.directorySnapshot(item.relative)
			if err != nil {
				return mutationEnvError("", "read planned directory %s: %v", item.planned.Path, err)
			}
			if !directorySnapshotsEqual(current, item.planned.directoryBefore) {
				return mutationEnvError(
					ReasonMutationStale,
					"%s changed after the mutation was planned; reload and retry",
					item.relative,
				)
			}
			removedDirectories = append(removedDirectories, *item)
			continue
		}
		if item.planned.operation == plannedSetCreatedDirectoryMode {
			createdDirectoryModes = append(createdDirectoryModes, *item)
			continue
		}

		data, current, err := readPlannedFileSnapshotRooted(rooted, *item)
		if err != nil {
			if item.planned.Before.Exists() {
				return mutationEnvError(
					ReasonMutationStale,
					"%s changed kind or identity after the mutation was planned; reload and retry: %v",
					item.relative,
					err,
				)
			}
			return mutationEnvError("", "inspect preimage %s: %v", item.planned.Path, err)
		}
		if current != item.planned.Before {
			return mutationEnvError(
				ReasonMutationStale,
				"%s changed after the mutation was planned; reload and retry",
				item.relative,
			)
		}
		item.preimage = journalPreimage{
			Path:    item.relative,
			Existed: current.Exists(),
			Data:    data,
		}
		if current.Exists() {
			info, err := rooted.lstat(item.relative)
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return mutationEnvError(
					ReasonMutationStale,
					"%s changed kind or identity after the mutation was planned; reload and retry",
					item.relative,
				)
			}
			if item.planned.checkBeforeMode && info.Mode().Perm() != item.planned.beforeMode.Perm() {
				return mutationEnvError(
					ReasonMutationStale,
					"%s permission mode changed after the mutation was planned; reload and retry",
					item.relative,
				)
			}
			item.preimage.Mode = uint32(info.Mode())
		}
		files = append(files, *item)
	}

	createdDirs, relativeCreatedDirs, err := t.transactionCreatedDirectories(rooted, prepared)
	if err != nil {
		return err
	}

	journal := &journal{
		Operation:   operation,
		StartedAt:   time.Now().UTC(),
		Files:       make([]string, len(files)),
		Preimages:   make([]journalPreimage, len(files)),
		CreatedDirs: relativeCreatedDirs,
		path:        t.journalPath(),
		rooted:      rooted,
	}
	for i, item := range files {
		journal.Files[i] = item.relative
		journal.Preimages[i] = item.preimage
	}
	for _, item := range removedDirectories {
		if !item.planned.directoryBefore.Exists {
			continue
		}
		journal.RemovedDirs = append(journal.RemovedDirs, journalRemovedDirectory{
			Path: item.relative,
			Mode: uint32(item.planned.directoryBefore.Mode),
		})
	}
	if err := journal.checkpointWithDirectorySync(syncDestination); err != nil {
		exists, inspectErr := journal.exists()
		if inspectErr != nil {
			return mutationEnvError("", "write mutation journal %s: %v; inspect landed journal: %v", journal.path, err, inspectErr)
		}
		if exists {
			return mutationEnvError(
				ReasonMutationRecovery,
				"mutation journal landed at %s but its durability is uncertain: %v",
				journal.path,
				err,
			)
		}
		return mutationEnvError("", "write mutation journal %s: %v", journal.path, err)
	}

	nextGeneration := currentGeneration
	if advanceGeneration {
		nextGeneration, err = mutationToken()
		if err != nil {
			if removeErr := clearJournalDurably(journal, syncDestination, restore); removeErr != nil {
				return mutationEnvError(
					ReasonMutationRecovery,
					"create mutation generation: %v; clear journal: %v",
					err,
					removeErr,
				)
			}
			return mutationEnvError("", "create mutation generation: %v", err)
		}
		var generationErr error
		if write == nil {
			generationErr = writeDestination(t.generationPath(), []byte(nextGeneration))
		} else {
			generationErr = rooted.writeFileAtomic(filepath.Join(".tiller", cacheDirName, generationFileName), []byte(nextGeneration))
		}
		if generationErr != nil {
			if removeErr := clearJournalDurably(journal, syncDestination, restore); removeErr != nil {
				return mutationEnvError(
					ReasonMutationRecovery,
					"write mutation generation: %v; clear journal: %v",
					generationErr,
					removeErr,
				)
			}
			return mutationEnvError("", "write mutation generation: %v", generationErr)
		}
	}

	rollbackJournal := newRollbackJournal(journal.path)
	recordedCreatedDirs := make(map[string]struct{}, len(createdDirs))
	captureCreatedDirs := func() error {
		for i, path := range createdDirs {
			if _, recorded := recordedCreatedDirs[path]; recorded {
				continue
			}
			info, err := rooted.lstat(relativeCreatedDirs[i])
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect transaction-created directory %s: %w", path, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("transaction-created directory %s is a symbolic link", path)
			}
			if !info.IsDir() {
				return fmt.Errorf("transaction-created path %s is not a directory", path)
			}
			recordedCreatedDirs[path] = struct{}{}
			rollbackJournal.CreatedDirs = append(rollbackJournal.CreatedDirs, relativeCreatedDirs[i])
		}
		return nil
	}
	finish := func(cause error) error {
		return finishFailedMutation(cause, journal, rollbackJournal, writeDestination, syncDestination, restore, rooted, useRootedRecovery)
	}
	// Materialize every missing destination while its ancestors are writable,
	// capturing each successful mkdir for in-process rollback.
	for i, dir := range createdDirs {
		info, err := rooted.lstat(relativeCreatedDirs[i])
		if errors.Is(err, fs.ErrNotExist) {
			err = rooted.mkdir(relativeCreatedDirs[i], 0o755)
		} else if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			err = fmt.Errorf("transaction-created path %s is not a directory", dir)
		}
		if captureErr := captureCreatedDirs(); captureErr != nil {
			err = errors.Join(err, captureErr)
		}
		if err != nil {
			return finish(err)
		}
	}

	// A copied tree may itself be read-only. Stage only source directories that
	// cannot delete descendants; the durable and rollback journals retain modes.
	sort.SliceStable(removedDirectories, func(i, j int) bool {
		left := strings.Count(removedDirectories[i].planned.Path, string(filepath.Separator))
		right := strings.Count(removedDirectories[j].planned.Path, string(filepath.Separator))
		return left < right
	})
	for _, item := range removedDirectories {
		if !item.planned.directoryBefore.Exists {
			continue
		}
		stagingMode := chmodModeBits(item.planned.directoryBefore.Mode) | 0o300
		if stagingMode == chmodModeBits(item.planned.directoryBefore.Mode) {
			continue
		}
		rollbackJournal.RemovedDirs = append(rollbackJournal.RemovedDirs, journalRemovedDirectory{
			Path: item.relative,
			Mode: uint32(item.planned.directoryBefore.Mode),
		})
		if err := rooted.chmodDirectory(item.relative, stagingMode); err != nil {
			return finish(fmt.Errorf("make planned directory writable %s: %w", item.relative, err))
		}
	}

	for _, item := range prepared {
		var applyErr error
		switch item.planned.operation {
		case plannedWriteFile:
			rollbackJournal.Preimages = append(rollbackJournal.Preimages, item.preimage)
			if write == nil && item.planned.setWriteMode {
				mode := item.planned.writeMode.Perm()
				applyErr = rooted.writeFileAtomicWithMode(item.relative, item.planned.Data, &mode)
			} else {
				applyErr = writeDestination(item.planned.Path, item.planned.Data)
				if applyErr == nil && item.planned.setWriteMode {
					applyErr = rooted.chmodRegular(item.relative, item.planned.writeMode.Perm())
				}
			}
		case plannedDeleteFile:
			rollbackJournal.Preimages = append(rollbackJournal.Preimages, item.preimage)
			applyErr = rooted.removeFile(item.relative)
		case plannedSetCreatedDirectoryMode:
			continue
		}
		if captureErr := captureCreatedDirs(); captureErr != nil {
			applyErr = errors.Join(applyErr, captureErr)
		}
		if applyErr != nil {
			return finish(applyErr)
		}
	}

	if sync != nil {
		if err := syncMutationDirectories(files, createdDirs, syncDestination); err != nil {
			return finish(err)
		}
	}

	sort.SliceStable(removedDirectories, func(i, j int) bool {
		left := strings.Count(removedDirectories[i].planned.Path, string(filepath.Separator))
		right := strings.Count(removedDirectories[j].planned.Path, string(filepath.Separator))
		return left > right
	})
	for _, item := range removedDirectories {
		if !item.planned.directoryBefore.Exists {
			continue
		}
		if chmodModeBits(item.planned.directoryBefore.Mode)|0o300 == chmodModeBits(item.planned.directoryBefore.Mode) {
			rollbackJournal.RemovedDirs = append(rollbackJournal.RemovedDirs, journalRemovedDirectory{
				Path: item.relative,
				Mode: uint32(item.planned.directoryBefore.Mode),
			})
		}
		if err := rooted.removeDirectory(item.relative); err != nil {
			return finish(fmt.Errorf("remove directory %s: %w", item.relative, err))
		}
		if sync != nil {
			if err := syncExistingDirectory(rooted, filepath.Dir(item.relative), filepath.Dir(item.planned.Path), syncDestination); err != nil {
				return finish(fmt.Errorf("sync removed directory %s: %w", item.relative, err))
			}
		}
	}

	// Apply preserved destination modes deepest-first only after all descendant
	// work is complete, so an ancestor cannot block a later chmod.
	sort.SliceStable(createdDirectoryModes, func(i, j int) bool {
		left := strings.Count(createdDirectoryModes[i].planned.Path, string(filepath.Separator))
		right := strings.Count(createdDirectoryModes[j].planned.Path, string(filepath.Separator))
		return left > right
	})
	for _, item := range createdDirectoryModes {
		info, err := rooted.lstat(item.relative)
		if err != nil {
			return finish(fmt.Errorf("inspect transaction-created directory %s: %w", item.relative, err))
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return finish(fmt.Errorf("transaction-created path %s is not a directory", item.relative))
		}
		if err := rooted.chmodDirectory(item.relative, item.planned.writeMode); err != nil {
			return finish(fmt.Errorf("set transaction-created directory mode %s: %w", item.relative, err))
		}
		if sync != nil {
			if err := syncDestination(item.planned.Path); err != nil {
				return finish(fmt.Errorf("sync transaction-created directory mode %s: %w", item.relative, err))
			}
		}
	}

	if err := clearJournalDurably(journal, syncDestination, restore); err != nil {
		return mutationEnvError(
			ReasonMutationRecovery,
			"mutation completed but journal could not be cleared durably at %s: %v",
			journal.path,
			err,
		)
	}
	if advanceGeneration {
		t.generation = nextGeneration
	}
	return nil
}
func readPlannedFileSnapshot(root string, planned PlannedWrite) ([]byte, FileSnapshot, error) {
	if planned.Before.Exists() {
		return ReadGuardedFileSnapshot(root, planned.Path)
	}
	exists, err := PathExistsNoSymlinks(root, planned.Path)
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	return nil, FileSnapshot{exists: exists}, nil
}

func readPlannedFileSnapshotRooted(rooted *rootedTransaction, planned preparedWrite) ([]byte, FileSnapshot, error) {
	if planned.planned.Before.Exists() {
		return rooted.readSnapshot(planned.relative, true)
	}
	exists, err := rooted.exists(planned.relative)
	if err != nil {
		return nil, FileSnapshot{}, err
	}
	return nil, FileSnapshot{exists: exists}, nil
}

func restoreJournalCheckpoint(journal *journal, sync func(string) error) error {
	return journal.checkpointWithDirectorySync(sync)
}

func clearJournalDurably(
	journal *journal,
	sync func(string) error,
	restore func(*journal, func(string) error) error,
) error {
	if err := journal.remove(); err != nil {
		return err
	}
	if err := sync(filepath.Dir(journal.path)); err != nil {
		if restoreErr := restore(journal, sync); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore recovery journal: %w", restoreErr))
		}
		return err
	}
	return nil
}

func recoveryMarkerMissing(err error, rooted *rootedTransaction, journalPath string) bool {
	var env *EnvError
	if !errors.As(err, &env) || env.Reason != ReasonMutationRecovery {
		return false
	}
	relative, pathErr := rooted.relativePath(journalPath)
	if pathErr != nil {
		return true
	}
	exists, inspectErr := rooted.exists(relative)
	return inspectErr != nil || !exists
}

func syncMutationDirectories(
	prepared []preparedWrite,
	createdDirs []string,
	sync func(string) error,
) error {
	seen := make(map[string]struct{})
	syncOnce := func(path string) error {
		if _, exists := seen[path]; exists {
			return nil
		}
		seen[path] = struct{}{}
		if err := sync(path); err != nil {
			return fmt.Errorf("sync directory %s: %w", path, err)
		}
		return nil
	}

	for _, item := range prepared {
		if item.planned.operation != plannedWriteFile || !item.planned.setWriteMode {
			continue
		}
		if err := sync(item.planned.Path); err != nil {
			return fmt.Errorf("sync file mode %s: %w", item.planned.Path, err)
		}
	}

	for i := len(createdDirs) - 1; i >= 0; i-- {
		dir := createdDirs[i]
		if err := syncOnce(dir); err != nil {
			return err
		}
		if err := syncOnce(filepath.Dir(dir)); err != nil {
			return err
		}
	}
	for _, item := range prepared {
		if err := syncOnce(filepath.Dir(item.planned.Path)); err != nil {
			return err
		}
	}
	return nil
}

func newRollbackJournal(path string) *journal {
	return &journal{path: path}
}

func finishFailedMutation(
	cause error,
	journal *journal,
	rollbackJournal *journal,
	write func(string, []byte) error,
	sync func(string) error,
	restore func(*journal, func(string) error) error,
	rooted *rootedTransaction,
	useRootedRecovery bool,
) error {
	var rollbackErr error
	if useRootedRecovery {
		rollbackErr = recoverJournalRooted(rooted, rollbackJournal)
	} else {
		rollbackErr = recoverJournalRootedWithIO(rooted, rollbackJournal, write, sync)
	}
	if rollbackErr != nil {
		return mutationEnvError(
			ReasonMutationRecovery,
			"mutation failed: %v; rollback failed: %v; recovery journal retained at %s",
			cause,
			rollbackErr,
			journal.path,
		)
	}
	if clearErr := clearJournalDurably(journal, sync, restore); clearErr != nil {
		return mutationEnvError(
			ReasonMutationRecovery,
			"mutation failed: %v; rollback completed but journal could not be cleared durably: %v",
			cause,
			clearErr,
		)
	}
	return mutationEnvError("", "mutation failed: %v", cause)
}

// recoverJournal reverses a persisted mutation. Removed directories are
// recreated in writable parent-first form before file preimages are restored;
// exact modes return deepest-first after descendant recovery completes.
func recoverJournal(root string, journal *journal, write func(string, []byte) error, sync func(string) error) error {
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	if err != nil {
		return err
	}
	defer rooted.close()
	return recoverJournalRootedWithIO(rooted, journal, write, sync)
}

// syncExistingDirectory syncs path when the transaction sees a real directory at
// relative. It inspects only the leaf, refusing a symlink or non-directory.
func syncExistingDirectory(rooted *rootedTransaction, relative, path string, sync func(string) error) error {
	if relative != "." {
		info, err := rooted.lstat(relative)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return &os.PathError{Op: "sync", Path: path, Err: errors.New("not a directory")}
		}
	}
	return sync(path)
}
