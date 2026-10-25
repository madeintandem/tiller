package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/rootpath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFileAtomic is the pathname writer the commit and recovery
// failure-injection seams take. It follows symlinks; repository code writes
// through WriteRepositoryFileAtomic.
func writeFileAtomic(path string, data []byte) error {
	return writeAtomic(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func writeAtomic(path string, write func(io.Writer) error) error {
	return writeAtomicWithDirectorySync(path, write, syncDirectory)
}

func TestWriteFileAtomic_Writes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "node.yml")

	require.NoError(t, writeFileAtomic(path, []byte("hello\n")))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(got))

	// No temp file is left behind in the destination directory.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "node.yml", entries[0].Name())
}

// A mid-write failure leaves the destination untouched and no temp file behind:
// the write goes to a temp file that is never renamed into place.
func TestWriteAtomic_NoPartialOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.yml")
	require.NoError(t, os.WriteFile(path, []byte("original\n"), 0o644))

	boom := errors.New("boom")
	err := writeAtomic(path, func(w io.Writer) error {
		_, _ = w.Write([]byte("partial")) // some bytes land in the temp file
		return boom
	})
	require.ErrorIs(t, err, boom)

	// The destination still holds the original content, not a partial write.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(got))

	// No temp file lingers in the directory.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "node.yml", entries[0].Name())
}

func TestWriteAtomicSyncsTheDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.yml")
	var synced []string

	err := writeAtomicWithDirectorySync(
		path,
		func(w io.Writer) error {
			_, err := w.Write([]byte("durable\n"))
			return err
		},
		func(path string) error {
			synced = append(synced, path)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{dir}, synced)
}

// lockTree builds a minimal Tree rooted at a writable temp dir, enough for the
// lock and journal which only need Root.
func lockTree(t *testing.T) *Tree {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	return &Tree{Root: root}
}

// mutationTree writes the smallest schema-version-4 repository a load-backed
// mutation test needs.
func mutationTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tillerDir := filepath.Join(root, ".tiller")
	require.NoError(t, os.MkdirAll(tillerDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(tillerDir, "tiller.yml"),
		[]byte("schema_version: 12\n"),
		0o644,
	))
	return root
}

// withLock records the intended file set, persists the journal mid-operation,
// and clears it on return.
func TestWithLock_WritesAndClearsJournal(t *testing.T) {
	tree := lockTree(t)
	journalPath := filepath.Join(tree.Root, ".tiller", ".cache", "journal.yml")

	err := tree.withLock(func(j *journal) error {
		j.Operation = "approve epic"
		j.Files = append(j.Files, "epics/form-engine/epic.yml", "epics/form-engine/features/x/feature.yml")
		require.NoError(t, j.checkpoint())

		// The journal exists on disk while the mutation is in progress.
		data, statErr := os.ReadFile(journalPath)
		require.NoError(t, statErr)
		assert.Contains(t, string(data), "approve epic")
		assert.Contains(t, string(data), "epics/form-engine/epic.yml")
		return nil
	})
	require.NoError(t, err)

	// The journal is cleared once the mutation returns.
	_, statErr := os.Stat(journalPath)
	assert.True(t, os.IsNotExist(statErr), "journal cleared after WithLock")

	// The lock is released too.
	_, statErr = os.Stat(filepath.Join(tree.Root, ".tiller", ".cache", "lock"))
	assert.True(t, os.IsNotExist(statErr), "lock released after WithLock")
}

// A second caller is refused with a recoverable env error while the first holds
// the lock, rather than blocking or corrupting state.
func TestWithLock_RefusesSecondCaller(t *testing.T) {
	tree := lockTree(t)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- tree.withLock(func(j *journal) error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started // the first caller now holds the lock

	err := tree.withLock(func(j *journal) error {
		t.Error("second caller should not have run its body")
		return nil
	})
	require.Error(t, err)
	var ee *EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonLocked, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())

	close(release)
	require.NoError(t, <-done)

	// Once released, a fresh caller acquires the lock again.
	require.NoError(t, tree.withLock(func(j *journal) error { return nil }))
}

// The journal is cleared even when the mutation body fails, so a failed
// precondition does not leave a stale journal behind.
func TestWithLock_ClearsJournalOnError(t *testing.T) {
	tree := lockTree(t)
	journalPath := filepath.Join(tree.Root, ".tiller", ".cache", "journal.yml")
	boom := errors.New("precondition stale")

	err := tree.withLock(func(j *journal) error {
		j.Files = append(j.Files, "a.yml")
		require.NoError(t, j.checkpoint())
		return boom
	})
	require.ErrorIs(t, err, boom)

	_, statErr := os.Stat(journalPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestWithLockRejectsAStaleTreeGeneration(t *testing.T) {
	root := mutationTree(t)
	current, err := Load(root)
	require.NoError(t, err)
	stale, err := Load(root)
	require.NoError(t, err)

	path := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, current.Commit("source change", []PlannedWrite{
		PlanWrite(path, AbsentSnapshot(), []byte("questions: []\n")),
	}))

	called := false
	err = stale.withLock(func(*journal) error {
		called = true
		return nil
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
	require.False(t, called)
}

func TestCommitRejectsAStaleTreeGeneration(t *testing.T) {
	root := mutationTree(t)
	first, err := Load(root)
	require.NoError(t, err)
	stale, err := Load(root)
	require.NoError(t, err)

	path := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, first.Commit("first", []PlannedWrite{
		PlanWrite(path, AbsentSnapshot(), []byte("questions: []\n")),
	}))

	err = stale.Commit("stale", []PlannedWrite{
		PlanWrite(path, AbsentSnapshot(), []byte("questions:\n  - id: Q-aaaaaa\n")),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "questions: []\n", string(data))
}

func TestCommitRejectsAStaleFileSnapshot(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "orientation.md")
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o644))
	_, before, err := ReadRegularFileSnapshot(root, path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("human edit\n"), 0o644))
	err = tree.Commit("orientation", []PlannedWrite{
		PlanWrite(path, before, []byte("generated\n")),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "human edit\n", string(data))
}

func TestCommitReadGuardChecksSnapshotWithoutWritingGuardedFile(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	guarded := filepath.Join(root, ".tiller", "orientation.md")
	require.NoError(t, os.WriteFile(guarded, []byte("unchanged\n"), 0o644))
	_, before, err := ReadRegularFileSnapshot(root, guarded)
	require.NoError(t, err)
	infoBefore, err := os.Stat(guarded)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")

	require.NoError(t, tree.Commit("guarded write", []PlannedWrite{
		PlanReadGuard(guarded, before),
		PlanWrite(destination, AbsentSnapshot(), []byte("questions: []\n")),
	}))

	data, err := os.ReadFile(guarded)
	require.NoError(t, err)
	require.Equal(t, "unchanged\n", string(data))
	infoAfter, err := os.Stat(guarded)
	require.NoError(t, err)
	require.Equal(t, infoBefore.ModTime(), infoAfter.ModTime())
}

func TestCommitReadGuardRefusesChangedSnapshotWithoutWritingDestinations(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	guarded := filepath.Join(root, ".tiller", "orientation.md")
	require.NoError(t, os.WriteFile(guarded, []byte("before\n"), 0o644))
	_, before, err := ReadRegularFileSnapshot(root, guarded)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(guarded, []byte("human edit\n"), 0o644))
	destination := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")

	err = tree.Commit("guarded write", []PlannedWrite{
		PlanReadGuard(guarded, before),
		PlanWrite(destination, AbsentSnapshot(), []byte("questions: []\n")),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
	_, statErr := os.Stat(destination)
	require.True(t, os.IsNotExist(statErr))
}

func TestCommitLineageInventoryGuardRefusesAddedOrRemovedNodesWithoutWritingDestinations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before bool
		mutate func(string) error
	}{
		{
			name: "addition",
			mutate: func(path string) error {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("id: F-added\n"), 0o644)
			},
		},
		{
			name:   "removal",
			before: true,
			mutate: func(path string) error { return os.RemoveAll(filepath.Dir(path)) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mutationTree(t)
			epicsDir := filepath.Join(root, ".tiller", "epics")
			nodePath := filepath.Join(epicsDir, "app", "features", "logging", "feature.yml")
			if tc.before {
				require.NoError(t, os.MkdirAll(filepath.Dir(nodePath), 0o755))
				require.NoError(t, os.WriteFile(nodePath, []byte("id: F-original\n"), 0o644))
			}
			inventory, err := ReadLineageInventory(root, epicsDir)
			require.NoError(t, err)
			require.NoError(t, tc.mutate(nodePath))

			destination := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
			tree := &Tree{Root: root}
			err = tree.Commit("guarded write", []PlannedWrite{
				PlanLineageInventoryGuard(epicsDir, inventory),
				PlanWrite(destination, AbsentSnapshot(), []byte("questions: []\n")),
			})

			require.Error(t, err)
			var env *EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, ReasonMutationStale, env.Reason)
			_, statErr := os.Stat(destination)
			require.True(t, os.IsNotExist(statErr))
		})
	}
}

func TestCommitLineageInventoryGuardCoversEpicAddRemoveAndRekey(t *testing.T) {
	for _, operation := range []string{"add", "remove", "rekey"} {
		t.Run(operation, func(t *testing.T) {
			root := mutationTree(t)
			epicsDir := filepath.Join(root, ".tiller", "epics")
			original := filepath.Join(epicsDir, "app")
			if operation != "add" {
				require.NoError(t, os.MkdirAll(original, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(original, "epic.yml"), []byte("id: E-app001\nkey: app\n"), 0o644))
			}
			inventory, err := ReadLineageInventory(root, epicsDir)
			require.NoError(t, err)
			switch operation {
			case "add":
				require.NoError(t, os.MkdirAll(original, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(original, "epic.yml"), []byte("id: E-app001\nkey: app\n"), 0o644))
			case "remove":
				require.NoError(t, os.RemoveAll(original))
			case "rekey":
				require.NoError(t, os.Rename(original, filepath.Join(epicsDir, "renamed")))
			}

			destination := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
			tree := &Tree{Root: root}
			err = tree.Commit("guarded epic inventory", []PlannedWrite{
				PlanLineageInventoryGuard(epicsDir, inventory),
				PlanWrite(destination, AbsentSnapshot(), []byte("questions: []\n")),
			})

			var env *EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, ReasonMutationStale, env.Reason)
			require.NoFileExists(t, destination)
		})
	}
}

func TestCommitWithOnlyALineageInventoryGuardIsANoOp(t *testing.T) {
	root := mutationTree(t)
	epicsDir := filepath.Join(root, ".tiller", "epics")
	require.NoError(t, os.MkdirAll(epicsDir, 0o755))
	inventory, err := ReadLineageInventory(root, epicsDir)
	require.NoError(t, err)
	generationPath := filepath.Join(root, ".tiller", ".cache", "generation")
	_, generationBefore, err := ReadRegularFileSnapshot(root, generationPath)
	require.NoError(t, err)

	tree := &Tree{Root: root}
	require.NoError(t, tree.Commit("guard only", []PlannedWrite{
		PlanLineageInventoryGuard(epicsDir, inventory),
	}))

	_, generationAfter, err := ReadRegularFileSnapshot(root, generationPath)
	require.NoError(t, err)
	require.Equal(t, generationBefore, generationAfter)
	_, journalErr := os.Stat(filepath.Join(root, ".tiller", ".cache", "journal.yml"))
	require.True(t, os.IsNotExist(journalErr))
}

func TestCommitRejectsARetainedRecoveryJournal(t *testing.T) {
	for _, derived := range []bool{false, true} {
		name := "source"
		if derived {
			name = "derived"
		}
		t.Run(name, func(t *testing.T) {
			root := mutationTree(t)
			tree, err := Load(root)
			require.NoError(t, err)

			journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
			require.NoError(t, os.MkdirAll(filepath.Dir(journalPath), 0o755))
			journalData := []byte("operation: interrupted\n")
			require.NoError(t, os.WriteFile(journalPath, journalData, 0o644))

			path := filepath.Join(root, ".tiller", "orientation.md")
			writes := []PlannedWrite{
				PlanWrite(path, AbsentSnapshot(), []byte("# Orientation\n")),
			}
			if derived {
				err = tree.CommitDerived("orientation", writes)
			} else {
				err = tree.Commit("orientation", writes)
			}

			require.Error(t, err)
			var env *EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, ReasonMutationRecovery, env.Reason)
			require.NoFileExists(t, path)
			preserved, readErr := os.ReadFile(journalPath)
			require.NoError(t, readErr)
			require.Equal(t, journalData, preserved)
		})
	}
}

func TestWithLockRejectsARetainedRecoveryJournal(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)

	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath), 0o755))
	journalData := []byte("operation: interrupted\n")
	require.NoError(t, os.WriteFile(journalPath, journalData, 0o644))

	called := false
	err = tree.withLock(func(*journal) error {
		called = true
		return nil
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
	require.False(t, called)

	preserved, readErr := os.ReadFile(journalPath)
	require.NoError(t, readErr)
	require.Equal(t, journalData, preserved)
}

func TestCommitRollsBackEarlierFilesWhenALaterWriteFails(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	first := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	blockedDir := filepath.Join(root, ".tiller", "events", "2026", "07", "29")
	require.NoError(t, os.MkdirAll(blockedDir, 0o755))
	require.NoError(t, os.Chmod(blockedDir, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(blockedDir, 0o755)) })
	second := filepath.Join(blockedDir, "EV-aaaaaa.yml")

	err = tree.Commit("add epic", []PlannedWrite{
		PlanWrite(first, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
		PlanWrite(second, AbsentSnapshot(), []byte("id: EV-aaaaaa\n")),
	})
	require.Error(t, err)
	require.NoFileExists(t, first)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "new"))
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics"))
	require.NoFileExists(t, second)
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
}

func TestCommitJournalsTransactionCreatedDirectories(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	first := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	second := filepath.Join(root, ".tiller", "events", "EV-aaaaaa.yml")
	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")

	var journalDuringWrite []byte
	var journalReadErr error
	err = tree.commitWithDestinationWriter("journal directories", []PlannedWrite{
		PlanWrite(first, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
		PlanWrite(second, AbsentSnapshot(), []byte("id: EV-aaaaaa\n")),
	}, true, func(path string, data []byte) error {
		if path == first {
			journalDuringWrite, journalReadErr = os.ReadFile(journalPath)
			return writeFileAtomic(path, data)
		}
		return errors.New("stop after first destination")
	})
	require.Error(t, err)
	require.NoError(t, journalReadErr)
	require.Contains(t, string(journalDuringWrite), "created_dirs:")
	require.Contains(t, string(journalDuringWrite), filepath.Join(".tiller", "epics"))
	require.Contains(t, string(journalDuringWrite), filepath.Join(".tiller", "epics", "new"))
}

func TestCommitPreservesJournalWhenRollbackFails(t *testing.T) {
	root := mutationTree(t)
	first := filepath.Join(root, ".tiller", "orientation.md")
	second := filepath.Join(root, ".tiller", "events", "EV-aaaaaa.yml")
	oldData := []byte("old orientation\n")
	require.NoError(t, os.WriteFile(first, oldData, 0o600))

	tree, err := Load(root)
	require.NoError(t, err)
	_, firstBefore, err := ReadRegularFileSnapshot(root, first)
	require.NoError(t, err)
	lockPath := tree.lockPath()
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(lockPath)) })

	writeFailure := errors.New("destination write failed")
	restoreFailure := errors.New("rollback restore failed")
	firstWrites := 0
	err = tree.commitWithDestinationWriter("rollback failure", []PlannedWrite{
		PlanWrite(first, firstBefore, []byte("new orientation\n")),
		PlanWrite(second, AbsentSnapshot(), []byte("id: EV-aaaaaa\n")),
	}, true, func(path string, data []byte) error {
		switch path {
		case first:
			firstWrites++
			if firstWrites == 2 {
				if err := os.Remove(lockPath); err != nil {
					return err
				}
				if err := os.Mkdir(lockPath, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(lockPath, "block"), []byte("held\n"), 0o644); err != nil {
					return err
				}
				return restoreFailure
			}
			return writeFileAtomic(path, data)
		case second:
			return writeFailure
		default:
			return writeFileAtomic(path, data)
		}
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
	require.ErrorContains(t, err, writeFailure.Error())
	require.ErrorContains(t, err, restoreFailure.Error())

	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	require.FileExists(t, journalPath)
	var got journal
	require.NoError(t, readYAMLFile(root, journalPath, &got))
	require.NotEmpty(t, got.Preimages)
	require.Equal(t, filepath.Join(".tiller", "orientation.md"), got.Preimages[0].Path)
	require.True(t, got.Preimages[0].Existed)
	require.Equal(t, uint32(0o600), got.Preimages[0].Mode)
	require.Equal(t, oldData, got.Preimages[0].Data)
}

func TestCommitDerivedDoesNotAdvanceSourceGeneration(t *testing.T) {
	root := mutationTree(t)
	source, err := Load(root)
	require.NoError(t, err)
	derived, err := Load(root)
	require.NoError(t, err)

	path := filepath.Join(root, ".tiller", "orientation.md")
	require.NoError(t, derived.CommitDerived("orientation", []PlannedWrite{
		PlanWrite(path, AbsentSnapshot(), []byte("# Orientation\n")),
	}))

	questionsPath := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, source.Commit("question", []PlannedWrite{
		PlanWrite(questionsPath, AbsentSnapshot(), []byte("questions: []\n")),
	}))
}

func TestCommitDerivedRejectsStaleSourceGeneration(t *testing.T) {
	root := mutationTree(t)
	derived, err := Load(root)
	require.NoError(t, err)
	source, err := Load(root)
	require.NoError(t, err)

	questionsPath := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, source.Commit("question", []PlannedWrite{
		PlanWrite(questionsPath, AbsentSnapshot(), []byte("questions: []\n")),
	}))

	path := filepath.Join(root, ".tiller", "orientation.md")
	err = derived.CommitDerived("orientation", []PlannedWrite{
		PlanWrite(path, AbsentSnapshot(), []byte("# Orientation\n")),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
	require.NoFileExists(t, path)
}

func TestCommitSyncsDestinationsBeforeClearingJournalAndCacheAfter(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	cacheDir := filepath.Dir(journalPath)
	destinationDir := filepath.Dir(destination)

	var synced []string
	destinationSyncedWithJournal := false
	cacheSyncedAfterJournalRemoval := false
	err = tree.commitWithDestinationWriterAndSync(
		"durable commit",
		[]PlannedWrite{
			PlanWrite(destination, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
		},
		true,
		writeFileAtomic,
		func(path string) error {
			synced = append(synced, path)
			_, statErr := os.Stat(journalPath)
			switch path {
			case destinationDir:
				destinationSyncedWithJournal = statErr == nil
			case cacheDir:
				cacheSyncedAfterJournalRemoval = errors.Is(statErr, os.ErrNotExist)
			}
			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, destinationSyncedWithJournal)
	require.True(t, cacheSyncedAfterJournalRemoval)
	require.Equal(t, []string{
		cacheDir,
		destinationDir,
		filepath.Dir(destinationDir),
		filepath.Join(root, ".tiller"),
		cacheDir,
	}, synced)
}

func TestCommitRollsBackADestinationChangedBeforeWriterFailure(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	writeFailure := errors.New("directory sync failed after rename")

	err = tree.commitWithDestinationWriter(
		"late writer failure",
		[]PlannedWrite{
			PlanWrite(destination, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
		},
		true,
		func(path string, data []byte) error {
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, data, 0o644))
			return writeFailure
		},
	)
	require.ErrorContains(t, err, writeFailure.Error())
	require.NoFileExists(t, destination)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics"))
}

func TestCommitRollsBackExplicitlyCreatedEmptyDirectoryAfterLaterFailure(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	epic := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	empty := filepath.Join(root, ".tiller", "epics", "new", "empty")
	later := filepath.Join(root, ".tiller", "events", "EV-aaaaaa.yml")
	writeFailure := errors.New("later destination failed")
	failedWriteAttempted := false

	err = tree.commitWithDestinationWriter(
		"empty directory rollback",
		[]PlannedWrite{
			PlanWrite(epic, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
			PlanSetCreatedDirectoryMode(empty, 0o710),
			PlanWrite(later, AbsentSnapshot(), []byte("id: EV-aaaaaa\n")),
		},
		true,
		func(path string, data []byte) error {
			if path == later {
				failedWriteAttempted = true
				return writeFailure
			}
			return writeFileAtomic(path, data)
		},
	)
	require.ErrorContains(t, err, writeFailure.Error())
	require.True(t, failedWriteAttempted, "the failure must occur after the empty directory is created")
	require.NoFileExists(t, epic)
	require.NoDirExists(t, empty)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics"))
}

func TestCommitSyncsRollbackBeforeClearingJournal(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	first := filepath.Join(root, ".tiller", "epics", "new", "epic.yml")
	second := filepath.Join(root, ".tiller", "events", "EV-aaaaaa.yml")
	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	firstDir := filepath.Dir(first)

	rollbackSyncedWithJournal := false
	err = tree.commitWithDestinationWriterAndSync(
		"durable rollback",
		[]PlannedWrite{
			PlanWrite(first, AbsentSnapshot(), []byte("id: E-aaaaaa\n")),
			PlanWrite(second, AbsentSnapshot(), []byte("id: EV-aaaaaa\n")),
		},
		true,
		func(path string, data []byte) error {
			if path == first {
				return writeFileAtomic(path, data)
			}
			return errors.New("second destination failed")
		},
		func(path string) error {
			if path == firstDir {
				_, statErr := os.Stat(journalPath)
				rollbackSyncedWithJournal = statErr == nil
			}
			return nil
		},
	)
	require.Error(t, err)
	require.True(t, rollbackSyncedWithJournal)
	require.NoFileExists(t, first)
	require.NoFileExists(t, journalPath)
}

func TestCommitRetainsRecoveryJournalWhenUnlinkSyncFails(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "orientation.md")
	journalPath := tree.journalPath()
	cacheSyncs := 0

	err = tree.commitWithDestinationWriterAndSync(
		"unlink sync failure",
		[]PlannedWrite{PlanWrite(destination, AbsentSnapshot(), []byte("orientation\n"))},
		false,
		writeFileAtomic,
		func(path string) error {
			if path == filepath.Dir(journalPath) {
				cacheSyncs++
				if cacheSyncs == 2 {
					return errors.New("cache sync failed")
				}
			}
			return nil
		},
	)
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
	require.FileExists(t, journalPath)

	_, err = Load(root)
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
}

func TestCommitClassifiesLandedJournalAfterCheckpointSyncFailure(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "orientation.md")
	journalPath := tree.journalPath()

	err = tree.commitWithDestinationWriterAndSync(
		"checkpoint sync failure",
		[]PlannedWrite{PlanWrite(destination, AbsentSnapshot(), []byte("orientation\n"))},
		false,
		writeFileAtomic,
		func(path string) error {
			if path == filepath.Dir(journalPath) {
				return errors.New("checkpoint sync failed")
			}
			return nil
		},
	)
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
	require.FileExists(t, journalPath)
	require.NoFileExists(t, destination)
}

func TestCommitKeepsLockWhenRecoveryJournalCannotBeRestored(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "orientation.md")
	journalPath := tree.journalPath()
	cacheDir := filepath.Dir(journalPath)
	cacheSyncs := 0

	err = tree.commitWithDestinationWriterSyncAndRestore(
		"lost recovery marker",
		[]PlannedWrite{PlanWrite(destination, AbsentSnapshot(), []byte("orientation\n"))},
		false,
		writeFileAtomic,
		func(path string) error {
			if path == cacheDir {
				cacheSyncs++
				if cacheSyncs == 2 {
					return errors.New("cache unlink sync failed")
				}
			}
			return nil
		},
		func(*journal, func(string) error) error {
			return errors.New("restore failed before rename")
		},
	)
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationRecovery, env.Reason)
	require.NoFileExists(t, journalPath)
	require.FileExists(t, tree.lockPath())

	_, err = Load(root)
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonLocked, env.Reason)
	err = tree.CommitDerived("blocked after lost marker", nil)
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonLocked, env.Reason)
}

func TestCommitDeletesFileAndAdvancesGeneration(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "orientation.md")
	require.NoError(t, os.WriteFile(path, []byte("obsolete\n"), 0o600))
	_, before, err := ReadRegularFileSnapshot(root, path)
	require.NoError(t, err)
	beforeGeneration := tree.generation

	require.NoError(t, tree.Commit("delete orientation", []PlannedWrite{
		PlanDeleteFile(path, before),
	}))

	require.NoFileExists(t, path)
	require.NotEqual(t, beforeGeneration, tree.generation)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitDeleteRefusesStaleChangedOrMissingPreimageBeforeDestinations(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  func(*testing.T, string)
		current string
	}{
		{
			name: "changed",
			mutate: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, []byte("human edit\n"), 0o644))
			},
			current: "human edit\n",
		},
		{
			name: "missing",
			mutate: func(t *testing.T, path string) {
				require.NoError(t, os.Remove(path))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := mutationTree(t)
			tree, err := Load(root)
			require.NoError(t, err)
			source := filepath.Join(root, ".tiller", "source.yml")
			destination := filepath.Join(root, ".tiller", "destination.yml")
			require.NoError(t, os.WriteFile(source, []byte("original\n"), 0o644))
			_, before, err := ReadRegularFileSnapshot(root, source)
			require.NoError(t, err)
			beforeGeneration := tree.generation
			test.mutate(t, source)

			err = tree.Commit("stale delete", []PlannedWrite{
				PlanWrite(destination, AbsentSnapshot(), []byte("must not land\n")),
				PlanDeleteFile(source, before),
			})
			require.Error(t, err)
			var env *EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, ReasonMutationStale, env.Reason)
			require.NoFileExists(t, destination)
			require.Equal(t, beforeGeneration, tree.generation)
			require.NoFileExists(t, tree.journalPath())
			if test.current == "" {
				require.NoFileExists(t, source)
			} else {
				data, readErr := os.ReadFile(source)
				require.NoError(t, readErr)
				require.Equal(t, test.current, string(data))
			}
		})
	}
}

func TestCommitDeleteMovesFileAsOneTransaction(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	source := filepath.Join(root, ".tiller", "source.yml")
	destination := filepath.Join(root, ".tiller", "archive", "destination.yml")
	data := []byte("id: E-aaaaaa\n")
	require.NoError(t, os.WriteFile(source, data, 0o644))
	_, sourceBefore, err := ReadRegularFileSnapshot(root, source)
	require.NoError(t, err)

	require.NoError(t, tree.Commit("move state", []PlannedWrite{
		PlanWrite(destination, AbsentSnapshot(), data),
		PlanDeleteFile(source, sourceBefore),
	}))

	require.NoFileExists(t, source)
	require.FileExists(t, destination)
	got, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, data, got)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitRemoveDirWaitsForPlannedChildDelete(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "obsolete")
	child := filepath.Join(dir, "node.yml")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(child, []byte("old\n"), 0o600))
	_, childBefore, err := ReadRegularFileSnapshot(root, child)
	require.NoError(t, err)
	dirBefore, err := ReadDirectorySnapshot(root, dir)
	require.NoError(t, err)

	require.NoError(t, tree.Commit("remove obsolete directory", []PlannedWrite{
		PlanRemoveEmptyDir(dir, dirBefore),
		PlanDeleteFile(child, childBefore),
	}))

	require.NoDirExists(t, dir)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitRemoveDirRefusesChangedEntryNamesBeforeDestinations(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "obsolete")
	child := filepath.Join(dir, "node.yml")
	unexpected := filepath.Join(dir, "human.yml")
	destination := filepath.Join(root, ".tiller", "destination.yml")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(child, []byte("old\n"), 0o600))
	_, childBefore, err := ReadRegularFileSnapshot(root, child)
	require.NoError(t, err)
	dirBefore, err := ReadDirectorySnapshot(root, dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(unexpected, []byte("human\n"), 0o644))
	beforeGeneration := tree.generation

	err = tree.Commit("stale directory", []PlannedWrite{
		PlanWrite(destination, AbsentSnapshot(), []byte("must not land\n")),
		PlanDeleteFile(child, childBefore),
		PlanRemoveEmptyDir(dir, dirBefore),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
	require.FileExists(t, child)
	require.FileExists(t, unexpected)
	require.NoFileExists(t, destination)
	require.Equal(t, beforeGeneration, tree.generation)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitDirectoryGuardRefusesChangedEntryNamesWithoutMutatingDirectory(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "amendments")
	destination := filepath.Join(root, ".tiller", "destination.yml")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	before, err := ReadDirectorySnapshot(root, dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A-new.yml"), []byte("new\n"), 0o600))

	err = tree.Commit("guard amendment inventory", []PlannedWrite{
		PlanDirectoryGuard(dir, before),
		PlanWrite(destination, AbsentSnapshot(), []byte("must not land\n")),
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
	require.FileExists(t, filepath.Join(dir, "A-new.yml"))
	require.NoFileExists(t, destination)
	require.DirExists(t, dir)
}

func TestCommitDeleteRollbackRestoresBytesAndModeAfterLaterWriteFails(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	source := filepath.Join(root, ".tiller", "source.yml")
	destination := filepath.Join(root, ".tiller", "destination.yml")
	original := []byte("original\n")
	require.NoError(t, os.WriteFile(source, original, 0o600))
	_, sourceBefore, err := ReadRegularFileSnapshot(root, source)
	require.NoError(t, err)
	boom := errors.New("later destination failed")

	err = tree.commitWithDestinationWriter("rollback delete", []PlannedWrite{
		PlanDeleteFile(source, sourceBefore),
		PlanWrite(destination, AbsentSnapshot(), []byte("new\n")),
	}, true, func(path string, data []byte) error {
		if path == destination {
			return boom
		}
		return writeFileAtomic(path, data)
	})
	require.Error(t, err)
	require.ErrorContains(t, err, boom.Error())
	got, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	require.Equal(t, original, got)
	info, statErr := os.Stat(source)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.NoFileExists(t, destination)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitRemoveDirRollbackRecreatesDirectoryBeforeRestoringFiles(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	parent := filepath.Join(root, ".tiller")
	dir := filepath.Join(parent, "obsolete")
	child := filepath.Join(dir, "node.yml")
	original := []byte("original\n")
	require.NoError(t, os.Mkdir(dir, 0o711))
	require.NoError(t, os.WriteFile(child, original, 0o600))
	_, childBefore, err := ReadRegularFileSnapshot(root, child)
	require.NoError(t, err)
	dirBefore, err := ReadDirectorySnapshot(root, dir)
	require.NoError(t, err)
	boom := errors.New("sync after directory removal failed")
	failed := false

	err = tree.commitWithDestinationWriterAndSync(
		"rollback removed directory",
		[]PlannedWrite{
			PlanRemoveEmptyDir(dir, dirBefore),
			PlanDeleteFile(child, childBefore),
		},
		true,
		writeFileAtomic,
		func(path string) error {
			if path == parent && !failed {
				_, statErr := os.Stat(dir)
				if errors.Is(statErr, os.ErrNotExist) {
					failed = true
					return boom
				}
			}
			return nil
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, boom.Error())
	require.True(t, failed, "failure must happen after the directory was removed")
	got, readErr := os.ReadFile(child)
	require.NoError(t, readErr)
	require.Equal(t, original, got)
	fileInfo, statErr := os.Stat(child)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	dirInfo, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o711), dirInfo.Mode().Perm())
	require.NoFileExists(t, tree.journalPath())
}

func TestRecoverJournalRestoresDeletedFilesAndRemovedDirectories(t *testing.T) {
	root := mutationTree(t)
	outer := filepath.Join(root, ".tiller", "obsolete")
	inner := filepath.Join(outer, "nested")
	child := filepath.Join(inner, "node.yml")
	t.Cleanup(func() { _ = os.Chmod(outer, 0o755) })
	original := []byte("original\n")
	require.NoError(t, os.Mkdir(outer, 0o750))
	require.NoError(t, os.Mkdir(inner, 0o711))
	require.NoError(t, os.WriteFile(child, original, 0o600))
	require.NoError(t, os.Chmod(outer, os.ModeSticky|0o555))
	journalPath := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	recovery := &journal{
		Operation: "crashed delete",
		Files:     []string{filepath.Join(".tiller", "obsolete", "nested", "node.yml")},
		Preimages: []journalPreimage{{
			Path:    filepath.Join(".tiller", "obsolete", "nested", "node.yml"),
			Existed: true,
			Mode:    uint32(0o600),
			Data:    original,
		}},
		RemovedDirs: []journalRemovedDirectory{
			{Path: filepath.Join(".tiller", "obsolete", "nested"), Mode: uint32(0o711)},
			{Path: filepath.Join(".tiller", "obsolete"), Mode: uint32(os.ModeSticky | 0o555)},
		},
		path: journalPath,
	}
	require.NoError(t, recovery.checkpoint())
	require.NoError(t, os.Chmod(outer, 0o755))
	require.NoError(t, os.Remove(child))
	require.NoError(t, os.Remove(inner))
	require.NoError(t, os.Remove(outer))

	var persisted journal
	require.NoError(t, readYAMLFile(root, journalPath, &persisted))
	require.NoError(t, recoverJournal(root, &persisted, writeFileAtomic, syncDirectory))

	got, err := os.ReadFile(child)
	require.NoError(t, err)
	require.Equal(t, original, got)
	childInfo, err := os.Stat(child)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), childInfo.Mode().Perm())
	innerInfo, err := os.Stat(inner)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o711), innerInfo.Mode().Perm())
	outerInfo, err := os.Stat(outer)
	require.NoError(t, err)
	require.Equal(t, os.ModeSticky|os.FileMode(0o555), outerInfo.Mode()&(os.ModeSticky|os.ModePerm))
}

func TestCommitRemoveDirRemovesDeepestFirstAndSyncsEveryParent(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	base := filepath.Join(root, ".tiller")
	outer := filepath.Join(base, "obsolete")
	inner := filepath.Join(outer, "nested")
	child := filepath.Join(inner, "node.yml")
	require.NoError(t, os.Mkdir(outer, 0o750))
	require.NoError(t, os.Mkdir(inner, 0o711))
	require.NoError(t, os.WriteFile(child, []byte("old\n"), 0o600))
	_, childBefore, err := ReadRegularFileSnapshot(root, child)
	require.NoError(t, err)
	outerBefore, err := ReadDirectorySnapshot(root, outer)
	require.NoError(t, err)
	innerBefore, err := ReadDirectorySnapshot(root, inner)
	require.NoError(t, err)
	var synced []string

	require.NoError(t, tree.commitWithDestinationWriterAndSync(
		"remove nested directories",
		[]PlannedWrite{
			PlanRemoveEmptyDir(outer, outerBefore),
			PlanDeleteFile(child, childBefore),
			PlanRemoveEmptyDir(inner, innerBefore),
		},
		true,
		writeFileAtomic,
		func(path string) error {
			synced = append(synced, path)
			switch path {
			case inner:
				require.NoFileExists(t, child)
				require.DirExists(t, inner)
			case outer:
				require.NoDirExists(t, inner)
				require.DirExists(t, outer)
			case base:
				require.NoDirExists(t, outer)
			}
			return nil
		},
	))

	require.Contains(t, synced, inner)
	require.Contains(t, synced, outer)
	require.Contains(t, synced, base)
	require.Equal(t, filepath.Dir(tree.journalPath()), synced[len(synced)-1])
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitRefusesContradictoryWriteAndDeleteForOnePath(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "orientation.md")
	original := []byte("original\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))
	_, before, err := ReadRegularFileSnapshot(root, path)
	require.NoError(t, err)

	err = tree.Commit("contradictory path", []PlannedWrite{
		PlanWrite(path, before, []byte("replacement\n")),
		PlanDeleteFile(path, before),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "appears more than once")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, original, got)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitDeleteRejectsSymlinkEscape(t *testing.T) {
	for _, test := range []struct {
		name     string
		relative string
		path     func(*testing.T, string, string) string
	}{
		{
			name:     "intermediate component",
			relative: filepath.Join(".tiller", "escape", "source.yml"),
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "escape")
				require.NoError(t, os.RemoveAll(link))
				require.NoError(t, os.Symlink(external, link))
				return filepath.Join(link, "source.yml")
			},
		},
		{
			name:     "final component",
			relative: filepath.Join(".tiller", "source.yml"),
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "source.yml")
				require.NoError(t, os.RemoveAll(link))
				require.NoError(t, os.Symlink(filepath.Join(external, "source.yml"), link))
				return link
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := mutationTree(t)
			external := t.TempDir()
			externalPath := filepath.Join(external, "source.yml")
			original := []byte("external\n")
			require.NoError(t, os.WriteFile(externalPath, original, 0o600))
			path := filepath.Join(root, test.relative)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, original, 0o600))
			_, before, err := ReadRegularFileSnapshot(root, path)
			require.NoError(t, err)
			tree, err := Load(root)
			require.NoError(t, err)
			path = test.path(t, root, external)

			err = tree.Commit("reject symlink delete", []PlannedWrite{
				PlanDeleteFile(path, before),
			})
			require.Error(t, err)
			got, readErr := os.ReadFile(externalPath)
			require.NoError(t, readErr)
			require.Equal(t, original, got)
			require.NoFileExists(t, tree.journalPath())
		})
	}
}

func TestCommitRemoveDirRejectsSymlinkEscape(t *testing.T) {
	for _, test := range []struct {
		name     string
		relative string
		path     func(*testing.T, string, string) string
	}{
		{
			name:     "intermediate component",
			relative: filepath.Join(".tiller", "escape", "empty"),
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "escape")
				require.NoError(t, os.RemoveAll(link))
				require.NoError(t, os.Symlink(external, link))
				return filepath.Join(link, "empty")
			},
		},
		{
			name:     "final component",
			relative: filepath.Join(".tiller", "empty"),
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "empty")
				require.NoError(t, os.RemoveAll(link))
				require.NoError(t, os.Symlink(filepath.Join(external, "empty"), link))
				return link
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := mutationTree(t)
			external := t.TempDir()
			externalDir := filepath.Join(external, "empty")
			require.NoError(t, os.Mkdir(externalDir, 0o711))
			path := filepath.Join(root, test.relative)
			require.NoError(t, os.MkdirAll(path, 0o711))
			before, err := ReadDirectorySnapshot(root, path)
			require.NoError(t, err)
			tree, err := Load(root)
			require.NoError(t, err)
			path = test.path(t, root, external)

			err = tree.Commit("reject symlink directory removal", []PlannedWrite{
				PlanRemoveEmptyDir(path, before),
			})
			require.Error(t, err)
			info, statErr := os.Stat(externalDir)
			require.NoError(t, statErr)
			require.True(t, info.IsDir())
			require.Equal(t, os.FileMode(0o711), info.Mode().Perm())
			require.NoFileExists(t, tree.journalPath())
		})
	}
}

func TestRecoverJournalDeleteRejectsSymlinkedFileEscape(t *testing.T) {
	root := mutationTree(t)
	external := t.TempDir()
	externalPath := filepath.Join(external, "source.yml")
	original := []byte("external\n")
	require.NoError(t, os.WriteFile(externalPath, original, 0o600))
	require.NoError(t, os.Symlink(external, filepath.Join(root, ".tiller", "escape")))
	recovery := &journal{Preimages: []journalPreimage{{
		Path:    filepath.Join(".tiller", "escape", "source.yml"),
		Existed: true,
		Mode:    uint32(0o600),
		Data:    []byte("recovered over external\n"),
	}}}

	err := recoverJournal(root, recovery, writeFileAtomic, syncDirectory)
	require.Error(t, err)
	require.ErrorContains(t, err, "symbolic link")
	got, readErr := os.ReadFile(externalPath)
	require.NoError(t, readErr)
	require.Equal(t, original, got)
}

func TestRecoverJournalDeleteRejectsSymlinkedDirectoryEscape(t *testing.T) {
	root := mutationTree(t)
	external := t.TempDir()
	externalDir := filepath.Join(external, "removed")
	require.NoError(t, os.Mkdir(externalDir, 0o711))
	require.NoError(t, os.Symlink(external, filepath.Join(root, ".tiller", "escape")))
	recovery := &journal{RemovedDirs: []journalRemovedDirectory{{
		Path: filepath.Join(".tiller", "escape", "removed"),
		Mode: uint32(0o700),
	}}}

	err := recoverJournal(root, recovery, writeFileAtomic, syncDirectory)
	require.Error(t, err)
	require.ErrorContains(t, err, "symbolic link")
	info, statErr := os.Stat(externalDir)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o711), info.Mode().Perm())
}

func TestCommitDeleteFirstWriteFailureLeavesLaterSourceUntouched(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	destination := filepath.Join(root, ".tiller", "destination.yml")
	source := filepath.Join(root, ".tiller", "source.yml")
	original := []byte("original\n")
	require.NoError(t, os.WriteFile(source, original, 0o600))
	oldTime := time.Unix(946684800, 0)
	require.NoError(t, os.Chtimes(source, oldTime, oldTime))
	beforeInfo, err := os.Stat(source)
	require.NoError(t, err)
	_, sourceBefore, err := ReadRegularFileSnapshot(root, source)
	require.NoError(t, err)
	boom := errors.New("first write failed")
	sourceTouched := false

	err = tree.commitWithDestinationWriter("failed move", []PlannedWrite{
		PlanWrite(destination, AbsentSnapshot(), []byte("partial\n")),
		PlanDeleteFile(source, sourceBefore),
	}, true, func(path string, data []byte) error {
		if path == source {
			sourceTouched = true
			return errors.New("later source is not writable")
		}
		require.Equal(t, destination, path)
		require.NoError(t, os.WriteFile(path, data, 0o644))
		return boom
	})
	require.Error(t, err)
	require.ErrorContains(t, err, boom.Error())
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Empty(t, env.Reason)
	require.False(t, sourceTouched)
	require.NoFileExists(t, destination)
	afterInfo, statErr := os.Stat(source)
	require.NoError(t, statErr)
	require.True(t, os.SameFile(beforeInfo, afterInfo))
	require.Equal(t, beforeInfo.Mode(), afterInfo.Mode())
	require.Equal(t, beforeInfo.ModTime(), afterInfo.ModTime())
	got, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	require.Equal(t, original, got)
	require.NoFileExists(t, tree.journalPath())
}

func TestCommitRemoveDirFirstWriteFailureLeavesUnattemptedDirectoryUntouched(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	parent := filepath.Join(root, ".tiller", "untouched")
	dir := filepath.Join(parent, "empty")
	destination := filepath.Join(root, ".tiller", "destination.yml")
	require.NoError(t, os.MkdirAll(dir, 0o711))
	beforeInfo, err := os.Stat(dir)
	require.NoError(t, err)
	dirBefore, err := ReadDirectorySnapshot(root, dir)
	require.NoError(t, err)
	boom := errors.New("first write failed")
	directoryTouched := false

	err = tree.commitWithDestinationWriterAndSync(
		"failed before directory removal",
		[]PlannedWrite{
			PlanWrite(destination, AbsentSnapshot(), []byte("partial\n")),
			PlanRemoveEmptyDir(dir, dirBefore),
		},
		true,
		func(path string, data []byte) error {
			require.Equal(t, destination, path)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			return boom
		},
		func(path string) error {
			if path == parent {
				directoryTouched = true
				return errors.New("unattempted directory is not writable")
			}
			return nil
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, boom.Error())
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Empty(t, env.Reason)
	require.False(t, directoryTouched)
	require.NoFileExists(t, destination)
	afterInfo, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	require.True(t, os.SameFile(beforeInfo, afterInfo))
	require.Equal(t, beforeInfo.Mode(), afterInfo.Mode())
	require.Equal(t, beforeInfo.ModTime(), afterInfo.ModTime())
	require.NoFileExists(t, tree.journalPath())
}

func TestRecoverJournalDeleteSyncsRestoredModeInodesBeforeParents(t *testing.T) {
	root := mutationTree(t)
	parent := filepath.Join(root, ".tiller")
	dir := filepath.Join(parent, "removed")
	child := filepath.Join(dir, "source.yml")
	recovery := &journal{
		Preimages: []journalPreimage{{
			Path:    filepath.Join(".tiller", "removed", "source.yml"),
			Existed: true,
			Mode:    uint32(0o600),
			Data:    []byte("original\n"),
		}},
		RemovedDirs: []journalRemovedDirectory{{
			Path: filepath.Join(".tiller", "removed"),
			Mode: uint32(0o711),
		}},
	}
	var synced []string

	require.NoError(t, recoverJournal(root, recovery, writeFileAtomic, func(path string) error {
		synced = append(synced, path)
		return nil
	}))

	require.Equal(t, []string{dir, parent, child, dir}, synced)
}

func TestCommitDeleteRecoveryRetainsJournalWhenRestoredModeInodeSyncFails(t *testing.T) {
	for _, test := range []struct {
		name   string
		target func(string, string) string
	}{
		{name: "directory", target: func(dir, _ string) string { return dir }},
		{name: "file", target: func(_, child string) string { return child }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := mutationTree(t)
			tree, err := Load(root)
			require.NoError(t, err)
			parent := filepath.Join(root, ".tiller")
			dir := filepath.Join(parent, "removed")
			child := filepath.Join(dir, "source.yml")
			original := []byte("original\n")
			require.NoError(t, os.Mkdir(dir, 0o711))
			require.NoError(t, os.WriteFile(child, original, 0o600))
			_, childBefore, err := ReadRegularFileSnapshot(root, child)
			require.NoError(t, err)
			dirBefore, err := ReadDirectorySnapshot(root, dir)
			require.NoError(t, err)
			startRollback := errors.New("post-removal parent sync failed")
			inodeSyncFailure := errors.New("restored mode inode sync failed")
			rollbackStarted := false
			inodeSyncFailed := false
			target := test.target(dir, child)

			err = tree.commitWithDestinationWriterAndSync(
				"retain after mode sync failure",
				[]PlannedWrite{
					PlanDeleteFile(child, childBefore),
					PlanRemoveEmptyDir(dir, dirBefore),
				},
				true,
				writeFileAtomic,
				func(path string) error {
					if path == parent && !rollbackStarted {
						_, statErr := os.Stat(dir)
						if errors.Is(statErr, os.ErrNotExist) {
							rollbackStarted = true
							return startRollback
						}
					}
					if rollbackStarted && path == target && !inodeSyncFailed {
						inodeSyncFailed = true
						return inodeSyncFailure
					}
					return nil
				},
			)
			require.Error(t, err)
			var env *EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, ReasonMutationRecovery, env.Reason)
			require.ErrorContains(t, err, startRollback.Error())
			require.ErrorContains(t, err, inodeSyncFailure.Error())
			require.True(t, inodeSyncFailed)
			require.FileExists(t, tree.journalPath())
			got, readErr := os.ReadFile(child)
			require.NoError(t, readErr)
			require.Equal(t, original, got)
			fileInfo, statErr := os.Stat(child)
			require.NoError(t, statErr)
			require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
			dirInfo, statErr := os.Stat(dir)
			require.NoError(t, statErr)
			require.Equal(t, os.FileMode(0o711), dirInfo.Mode().Perm())
		})
	}
}

func TestCommitReadGuardRejectsDestinationSymlinkAncestorsBeforeMoveWrites(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string, string) string
	}{
		{
			name: "intermediate",
			setup: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "destination-parent")
				require.NoError(t, os.Symlink(external, link))
				return filepath.Join(link, "node")
			},
		},
		{
			name: "final",
			setup: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "destination")
				require.NoError(t, os.Symlink(external, link))
				return link
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := mutationTree(t)
			tree, err := Load(root)
			require.NoError(t, err)
			external := t.TempDir()
			sentinel := filepath.Join(external, "sentinel.txt")
			require.NoError(t, os.WriteFile(sentinel, []byte("outside\n"), 0o600))
			destinationRoot := tt.setup(t, root, external)
			destination := filepath.Join(destinationRoot, "state.yml")

			err = tree.Commit("guard destination path", []PlannedWrite{
				PlanReadGuard(destinationRoot, AbsentSnapshot()),
				PlanWrite(destination, AbsentSnapshot(), []byte("inside\n")),
			})
			require.Error(t, err)
			require.NoFileExists(t, filepath.Join(external, "state.yml"))
			data, readErr := os.ReadFile(sentinel)
			require.NoError(t, readErr)
			require.Equal(t, []byte("outside\n"), data)
		})
	}
}

func TestCommitDeleteKindChangeToFIFONeverBlocksOrDeletes(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	source := filepath.Join(root, ".tiller", "source.yml")
	require.NoError(t, os.WriteFile(source, []byte("source\n"), 0o600))
	_, before, err := ReadRegularFileSnapshot(root, source)
	require.NoError(t, err)
	require.NoError(t, os.Remove(source))
	require.NoError(t, syscall.Mkfifo(source, 0o600))

	done := make(chan error, 1)
	go func() {
		done <- tree.Commit("reject changed source kind", []PlannedWrite{
			PlanDeleteFile(source, before),
		})
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(250 * time.Millisecond):
		writer, openErr := os.OpenFile(source, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if openErr == nil {
			_ = writer.Close()
		}
		<-done
		t.Fatal("commit blocked opening a FIFO that replaced the planned regular source")
	}
	info, err := os.Lstat(source)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeNamedPipe)
}

func TestPathExistsNoSymlinksDoesNotOpenOrReadExistingSparseRegularFile(t *testing.T) {
	root := mutationTree(t)
	path := filepath.Join(root, ".tiller", "large-collision.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(8<<30))
	require.NoError(t, file.Close())
	opened := false

	exists, err := pathExistsNoSymlinksWithWalker(root, path, rootpath.Walker{
		BeforeOpenFile: func(string) {
			opened = true
		},
	})
	require.NoError(t, err)
	require.True(t, exists)
	require.False(t, opened, "existence checks never open or read the regular leaf")
}

func TestPathExistsNoSymlinksRejectsIntermediateAndFinalSymlinks(t *testing.T) {
	for _, tt := range []struct {
		name string
		path func(*testing.T, string, string) string
	}{
		{
			name: "intermediate",
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "external")
				require.NoError(t, os.Symlink(external, link))
				return filepath.Join(link, "collision")
			},
		},
		{
			name: "final",
			path: func(t *testing.T, root, external string) string {
				link := filepath.Join(root, ".tiller", "collision")
				require.NoError(t, os.Symlink(external, link))
				return link
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := mutationTree(t)
			external := t.TempDir()
			path := tt.path(t, root, external)
			_, err := pathExistsNoSymlinksWithWalker(root, path, rootpath.Walker{})
			require.Error(t, err)
		})
	}
}

func TestRootedTransactionOperationsStayOnOpenedParentAfterSymlinkSwap(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		external func(*testing.T, string)
		run      func(*testing.T, *rootedTransaction, string)
		assert   func(*testing.T, string, string)
	}{
		{
			name:   "atomic write and rename",
			target: filepath.Join(".tiller", "inside", "state.yml"),
			external: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "state.yml"), []byte("outside\n"), 0o600))
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.writeFileAtomic(target, []byte("inside\n")))
			},
			assert: func(t *testing.T, held, external string) {
				requireFileData(t, filepath.Join(held, "state.yml"), []byte("inside\n"))
				requireFileData(t, filepath.Join(external, "state.yml"), []byte("outside\n"))
			},
		},
		{
			name:   "file deletion",
			target: filepath.Join(".tiller", "inside", "state.yml"),
			external: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "state.yml"), []byte("outside\n"), 0o600))
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.removeFile(target))
			},
			assert: func(t *testing.T, held, external string) {
				require.NoFileExists(t, filepath.Join(held, "state.yml"))
				requireFileData(t, filepath.Join(external, "state.yml"), []byte("outside\n"))
			},
		},
		{
			name:   "directory creation",
			target: filepath.Join(".tiller", "inside", "created"),
			external: func(*testing.T, string) {
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.mkdir(target, 0o755))
			},
			assert: func(t *testing.T, held, external string) {
				require.DirExists(t, filepath.Join(held, "created"))
				require.NoDirExists(t, filepath.Join(external, "created"))
			},
		},
		{
			name:   "directory removal",
			target: filepath.Join(".tiller", "inside", "empty"),
			external: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o711))
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.removeDirectory(target))
			},
			assert: func(t *testing.T, held, external string) {
				require.NoDirExists(t, filepath.Join(held, "empty"))
				require.DirExists(t, filepath.Join(external, "empty"))
			},
		},
		{
			name:   "regular file mode",
			target: filepath.Join(".tiller", "inside", "state.yml"),
			external: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "state.yml"), []byte("outside\n"), 0o600))
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.chmodRegular(target, 0o640))
			},
			assert: func(t *testing.T, held, external string) {
				requirePathMode(t, filepath.Join(held, "state.yml"), 0o640)
				requirePathMode(t, filepath.Join(external, "state.yml"), 0o600)
			},
		},
		{
			name:   "directory mode",
			target: filepath.Join(".tiller", "inside", "empty"),
			external: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o711))
			},
			run: func(t *testing.T, tx *rootedTransaction, target string) {
				require.NoError(t, tx.chmodDirectory(target, 0o750))
			},
			assert: func(t *testing.T, held, external string) {
				requirePathMode(t, filepath.Join(held, "empty"), 0o750)
				requirePathMode(t, filepath.Join(external, "empty"), 0o711)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := mutationTree(t)
			inside := filepath.Join(root, ".tiller", "inside")
			require.NoError(t, os.Mkdir(inside, 0o755))
			switch filepath.Base(tt.target) {
			case "state.yml":
				require.NoError(t, os.WriteFile(filepath.Join(inside, "state.yml"), []byte("before\n"), 0o600))
			case "empty":
				require.NoError(t, os.Mkdir(filepath.Join(inside, "empty"), 0o700))
			}
			external := t.TempDir()
			tt.external(t, external)
			held := inside + "-held"
			swapped := false
			tx, err := openRootedTransaction(root, rootOperationHooks{
				AfterOpenParent: func(operation, target string) {
					if swapped || target != tt.target {
						return
					}
					swapped = true
					require.NoError(t, os.Rename(inside, held))
					require.NoError(t, os.Symlink(external, inside))
				},
			})
			require.NoError(t, err)
			defer tx.close()

			tt.run(t, tx, tt.target)

			require.True(t, swapped)
			tt.assert(t, held, external)
		})
	}
}

func TestRootedTransactionRejectsFIFOAndSymlinkLeavesWithoutBlocking(t *testing.T) {
	root := mutationTree(t)
	inside := filepath.Join(root, ".tiller", "inside")
	require.NoError(t, os.Mkdir(inside, 0o755))
	fifo := filepath.Join(inside, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	require.NoError(t, os.Symlink("fifo", filepath.Join(inside, "link")))
	tx, err := openRootedTransaction(root, rootOperationHooks{})
	require.NoError(t, err)
	defer tx.close()

	done := make(chan error, 2)
	go func() { done <- tx.chmodRegular(filepath.Join(".tiller", "inside", "fifo"), 0o640) }()
	go func() { done <- tx.chmodRegular(filepath.Join(".tiller", "inside", "link"), 0o640) }()
	for range 2 {
		select {
		case err := <-done:
			require.Error(t, err)
		case <-time.After(250 * time.Millisecond):
			t.Fatal("descriptor-relative mode operation blocked on a FIFO or followed a symlink leaf")
		}
	}
	requirePathMode(t, fifo, 0o600)
}

func requireFileData(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func requirePathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, want, info.Mode().Perm())
}

func TestCommitUsesRootedOperationsAfterPlannedParentSwap(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	inside := filepath.Join(root, ".tiller", "inside")
	require.NoError(t, os.Mkdir(inside, 0o755))
	destination := filepath.Join(inside, "state.yml")
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "state.yml"), []byte("outside\n"), 0o600))
	held := inside + "-held"
	swapped := false

	err = tree.commitWithRootOperationHooks("rooted race", []PlannedWrite{
		PlanWrite(destination, AbsentSnapshot(), []byte("inside\n")),
	}, rootOperationHooks{AfterOpenParent: func(operation, target string) {
		if swapped || operation != "write" || target != filepath.Join(".tiller", "inside", "state.yml") {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(inside, held))
		require.NoError(t, os.Symlink(external, inside))
	}})

	require.NoError(t, err)
	require.True(t, swapped)
	requireFileData(t, filepath.Join(held, "state.yml"), []byte("inside\n"))
	requireFileData(t, filepath.Join(external, "state.yml"), []byte("outside\n"))
	require.NoFileExists(t, tree.journalPath())
}

func TestRecoverJournalUsesRootedOperationsAfterParentSwap(t *testing.T) {
	root := mutationTree(t)
	inside := filepath.Join(root, ".tiller", "inside")
	require.NoError(t, os.Mkdir(inside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(inside, "state.yml"), []byte("mutated\n"), 0o600))
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "state.yml"), []byte("outside\n"), 0o640))
	held := inside + "-held"
	swapped := false

	err := recoverJournalWithRootOperationHooks(root, &journal{Preimages: []journalPreimage{{
		Path:    filepath.Join(".tiller", "inside", "state.yml"),
		Existed: true,
		Mode:    uint32(0o600),
		Data:    []byte("recovered\n"),
	}}}, rootOperationHooks{AfterOpenParent: func(operation, target string) {
		if swapped || operation != "write" || target != filepath.Join(".tiller", "inside", "state.yml") {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(inside, held))
		require.NoError(t, os.Symlink(external, inside))
	}})

	require.NoError(t, err)
	require.True(t, swapped)
	requireFileData(t, filepath.Join(held, "state.yml"), []byte("recovered\n"))
	requirePathMode(t, filepath.Join(held, "state.yml"), 0o600)
	requireFileData(t, filepath.Join(external, "state.yml"), []byte("outside\n"))
	requirePathMode(t, filepath.Join(external, "state.yml"), 0o640)
}

func TestCommitRetainsPlannedParentsForFullRollbackAfterSwap(t *testing.T) {
	root := mutationTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	inside := filepath.Join(root, ".tiller", "inside")
	empty := filepath.Join(inside, "empty")
	require.NoError(t, os.MkdirAll(empty, 0o755))
	emptyBefore, err := ReadDirectorySnapshot(root, empty)
	require.NoError(t, err)
	first := filepath.Join(inside, "first.yml")
	second := filepath.Join(inside, "second.yml")
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "first.yml"), []byte("outside first\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(external, "second.yml"), []byte("outside second\n"), 0o600))
	held := inside + "-held"
	swapped := false

	err = tree.commitWithRootOperationHooks("rooted rollback race", []PlannedWrite{
		PlanWrite(first, AbsentSnapshot(), []byte("inside first\n")),
		PlanWrite(second, AbsentSnapshot(), []byte("inside second\n")),
		PlanRemoveEmptyDir(empty, emptyBefore),
	}, rootOperationHooks{AfterOpenParent: func(operation, target string) {
		if swapped || operation != "write" || target != filepath.Join(".tiller", "inside", "first.yml") {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(inside, held))
		require.NoError(t, os.Symlink(external, inside))
		require.NoError(t, os.WriteFile(filepath.Join(held, "empty", "concurrent"), []byte("block removal\n"), 0o600))
	}})

	require.Error(t, err)
	require.True(t, swapped)
	require.NoFileExists(t, filepath.Join(held, "first.yml"))
	require.NoFileExists(t, filepath.Join(held, "second.yml"))
	require.FileExists(t, filepath.Join(held, "empty", "concurrent"))
	requireFileData(t, filepath.Join(external, "first.yml"), []byte("outside first\n"))
	requireFileData(t, filepath.Join(external, "second.yml"), []byte("outside second\n"))
	require.NoFileExists(t, tree.journalPath())
}

func TestRootedSyncHonorsDurabilityGate(t *testing.T) {
	root := mutationTree(t)
	tx, err := openRootedTransaction(root, rootOperationHooks{})
	require.NoError(t, err)
	defer tx.close()
	missing := filepath.Join(".tiller", "missing")

	t.Setenv("TILLER_FSYNC", "off")
	require.NoError(t, tx.syncPath(missing), "disabled durability does not open a path")
	t.Setenv("TILLER_FSYNC", "on")
	require.Error(t, tx.syncPath(missing), "enabled durability opens and syncs the path")
}

func TestLineageInventoryClosesTransientParentsAndKeepsRetainedSetBounded(t *testing.T) {
	root := mutationTree(t)
	epicsDir := filepath.Join(root, ".tiller", "epics")
	for i := range 64 {
		epic := filepath.Join(epicsDir, fmt.Sprintf("epic-%02d", i))
		feature := filepath.Join(epic, "features", "feature", TaskDirName, "task")
		require.NoError(t, os.MkdirAll(feature, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(epic, "epic.yml"), []byte("id: E-epic00\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(epic, "features", "feature", "feature.yml"), []byte("id: F-feat00\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(feature, TaskStateFile), []byte("id: T-task00\n"), 0o600))
	}
	tx, err := openRootedTransaction(root, rootOperationHooks{})
	require.NoError(t, err)
	defer tx.close()
	require.NoError(t, tx.preopenParents([]string{filepath.Join(".tiller", "epics")}))

	inventory, err := tx.readLineageInventory(filepath.Join(".tiller", "epics"))
	require.NoError(t, err)
	require.Len(t, inventory.Files, 64*3)
	tx.mu.Lock()
	retained := len(tx.parents)
	tx.mu.Unlock()
	require.Equal(t, 1, retained, "only the explicitly preopened .tiller parent stays retained")
}

func TestCommitRetainsOnlyMutationParentsWithManyReadGuards(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "success"
		if stale {
			name = "stale guard"
		}
		t.Run(name, func(t *testing.T) {
			root := mutationTree(t)
			tree, err := Load(root)
			require.NoError(t, err)

			const guardCount = 32
			writes := make([]PlannedWrite, 0, guardCount*3+2)
			var stalePath string
			for i := range guardCount {
				fileGuard := filepath.Join(root, ".tiller", "guards", fmt.Sprintf("file-%02d", i), "guard.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(fileGuard), 0o755))
				require.NoError(t, os.WriteFile(fileGuard, []byte("unchanged\n"), 0o600))
				_, fileBefore, err := ReadRegularFileSnapshot(root, fileGuard)
				require.NoError(t, err)
				writes = append(writes, PlanReadGuard(fileGuard, fileBefore))
				if i == 0 {
					stalePath = fileGuard
				}

				directoryGuard := filepath.Join(root, ".tiller", "guards", fmt.Sprintf("directory-%02d", i), "guarded")
				require.NoError(t, os.MkdirAll(directoryGuard, 0o755))
				directoryBefore, err := ReadDirectorySnapshot(root, directoryGuard)
				require.NoError(t, err)
				writes = append(writes, PlanDirectoryGuard(directoryGuard, directoryBefore))

				lineageGuard := filepath.Join(root, ".tiller", "guards", fmt.Sprintf("lineage-%02d", i), "epics")
				require.NoError(t, os.MkdirAll(lineageGuard, 0o755))
				lineageBefore, err := ReadLineageInventory(root, lineageGuard)
				require.NoError(t, err)
				writes = append(writes, PlanLineageInventoryGuard(lineageGuard, lineageBefore))
			}

			for _, name := range []string{"first", "second"} {
				destination := filepath.Join(root, ".tiller", "mutations", name, "state.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
				writes = append(writes, PlanWrite(destination, AbsentSnapshot(), []byte(name+"\n")))
			}
			if stale {
				require.NoError(t, os.WriteFile(stalePath, []byte("changed\n"), 0o600))
			}

			var retained []*os.Root
			err = tree.commitWithRootOperationHooks("bounded guard descriptors", writes, rootOperationHooks{
				AfterPreopenParents: func(parents []*os.Root) {
					retained = append(retained, parents...)
				},
			})
			if stale {
				require.Error(t, err)
				var env *EnvError
				require.ErrorAs(t, err, &env)
				require.Equal(t, ReasonMutationStale, env.Reason)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, retained, 2, "only mutation apply and rollback parents remain retained")
			for _, parent := range retained {
				_, err := parent.Lstat(".")
				require.ErrorIs(t, err, os.ErrClosed, "transaction closes retained parents on every exit")
			}
		})
	}
}
