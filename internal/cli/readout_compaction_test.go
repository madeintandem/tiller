package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// TestReadoutSinceKeepsANewEventBesideACompactedTwin pins that a compaction
// deleting a loose event does not hide a new loose event of the same type and
// target. The two files are nearly identical, so git with rename detection on
// pairs them as a rename and an added-files filter drops the new one.
func TestReadoutSinceKeepsANewEventBesideACompactedTwin(t *testing.T) {
	layDraftEpicTree(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	link := []string{"epic", "billing", "--depends-on", "form-engine", "--contributor", "jc"}
	_, _, code := execCLI(t, append([]string{"link"}, link...)...)
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, append([]string{"unlink"}, link...)...)
	require.Equal(t, 0, code)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "link and unlink billing")
	since := gitOutput(t, dir, "rev-parse", "--short", "HEAD")

	compactEventFiles(t, dir, looseEventFiles(t, dir))
	_, _, code = execCLI(t, append([]string{"link"}, link...)...)
	require.Equal(t, 0, code)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "compact, then link billing again")

	out, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)
	lines := changedLines(out)
	require.Len(t, lines, 1, out)
	require.Contains(t, lines[0], "linked", out)
}

// TestReadoutSinceReadsAProjectInARepositorySubdirectory pins the correction
// readout for a project that is not at the repository root: git names paths
// relative to the project, and a compaction between the sha and HEAD still
// leaves only the new event in Changed.
func TestReadoutSinceReadsAProjectInARepositorySubdirectory(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, "app")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(ownedManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("repo\n"), 0o644))
	testgit.Init(t, repo)
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "init")
	t.Chdir(dir)
	for _, key := range []string{"form-engine", "billing"} {
		_, _, code := execCLI(t, "add", "epic", key, "--title", key, "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "lay down two events")
	since := gitOutput(t, repo, "rev-parse", "--short", "HEAD")

	compactEventFiles(t, dir, looseEventFiles(t, dir))
	_, _, code := execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "compact, then one new event")

	out, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)
	lines := changedLines(out)
	require.Len(t, lines, 1, out)
	require.Contains(t, lines[0], "payments")
}

// TestReadoutSinceIgnoresAStrayJSONLFile pins that only a segment, a .jsonl
// directly in a month directory with a segment name, is read as events. A
// stray notes.jsonl anywhere else under .tiller/events is not.
func TestReadoutSinceIgnoresAStrayJSONLFile(t *testing.T) {
	layDraftEpicTree(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay down two events")
	since := gitOutput(t, dir, "rev-parse", "--short", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "events", "notes.jsonl"), []byte("not an event\n"), 0o644))
	_, _, code := execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "a stray file and one new event")

	out, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)
	lines := changedLines(out)
	require.Len(t, lines, 1, out)
	require.Contains(t, lines[0], "payments")
}

// TestReadoutSinceReadsCommittedEventsAnUncommittedCompactionFolded pins that
// the correction block reads committed history: after a compaction folds the
// committed loose events and before it is committed, the loose files are gone
// from the working tree, yet every event git names as added still reads.
func TestReadoutSinceReadsCommittedEventsAnUncommittedCompactionFolded(t *testing.T) {
	dir, since := layCorrectedDraftTree(t)
	before, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)

	out, errOut, code := execCLI(t, "compact", "--all")
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "Compacted 1 day of events")
	require.Empty(t, looseEventFilesOnDisk(t, dir), "the compaction folded every loose file")

	after, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)
	require.Equal(t, changedLines(before), changedLines(after))
	require.Len(t, changedLines(after), 4, after)
}

// looseEventFilesOnDisk lists the loose event files in the working tree.
func looseEventFilesOnDisk(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(filepath.Join(dir, ".tiller", "events"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(p) == ".yml" {
			found = append(found, p)
		}
		return err
	})
	require.NoError(t, err)
	return found
}

// TestReadoutSinceRefusesTwoDifferentRecordsOfOneEvent pins that the
// correction block counts an event once by the same rule every reader keeps:
// a loose file and a segment that hold one id with different content refuse
// the block, naming both files, rather than one copy silently winning.
func TestReadoutSinceRefusesTwoDifferentRecordsOfOneEvent(t *testing.T) {
	layDraftEpicTree(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay down two events")
	since := gitOutput(t, dir, "rev-parse", "--short", "HEAD")

	before := looseEventFiles(t, dir)
	_, _, code := execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, dir, "add", "-A")
	var added string
	for _, rel := range looseEventFiles(t, dir) {
		if !slices.Contains(before, rel) {
			added = rel
		}
	}
	require.NotEmpty(t, added)
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(added)))
	require.NoError(t, err)
	ev, err := events.DecodeLoose(added, raw)
	require.NoError(t, err)
	ev.Command = "tiller add epic payments --title Other"
	data, err := events.EncodeSegment([]events.Event{ev})
	require.NoError(t, err)
	segment := events.SegmentRelPath(ev.At, data)
	require.NoError(t, os.WriteFile(filepath.Join(dir, segment), data, 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "one event, held twice with different content")

	_, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 3, code, errOut)
	require.Contains(t, errOut, ev.ID+" differs between")
	require.Contains(t, errOut, added)
	require.Contains(t, errOut, filepath.ToSlash(segment))
}

func TestReadoutSinceChecksHistoricalRecordsBeforeExcludingCompactedEvents(t *testing.T) {
	for _, source := range []string{"loose", "segment"} {
		for _, conflicting := range []bool{false, true} {
			name := source + "/equal"
			if conflicting {
				name = source + "/conflicting"
			}
			t.Run(name, func(t *testing.T) {
				dir := compactRepo(t)
				loose := []string{
					layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z"),
					layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-03T11:00:00Z"),
				}
				var recorded []events.Event
				historical := relTo(t, dir, loose[0])
				for i, p := range loose {
					raw, err := os.ReadFile(p)
					require.NoError(t, err)
					ev, err := events.DecodeLoose(relTo(t, dir, p), raw)
					require.NoError(t, err)
					recorded = append(recorded, ev)
					if source == "segment" {
						data, err := events.EncodeSegment([]events.Event{ev})
						require.NoError(t, err)
						rel := events.SegmentRelPath(ev.At, data)
						require.NoError(t, os.WriteFile(filepath.Join(dir, rel), data, 0o644))
						require.NoError(t, os.Remove(p))
						if i == 0 {
							historical = filepath.ToSlash(rel)
						}
					}
				}
				commitAll(t, dir, "historical events")
				since := gitOutput(t, dir, "rev-parse", "--short", "HEAD")

				// A new combined segment overlaps the historical records. Equal
				// copies are compaction only; changed content must not disappear
				// merely because its id or loose path existed at the baseline.
				if conflicting {
					recorded[0].Command = "tiller add task --title Changed"
				}
				data, err := events.EncodeSegment(recorded)
				require.NoError(t, err)
				segment := events.SegmentRelPath(recorded[0].At, data)
				require.NoError(t, os.WriteFile(filepath.Join(dir, segment), data, 0o644))
				if source == "loose" {
					for _, p := range loose {
						require.NoError(t, os.Remove(p))
					}
				}
				commitAll(t, dir, "compact historical events")

				evs, err := readoutEventsSince(vcs.New(dir), since)
				if conflicting {
					var env *store.EnvError
					require.ErrorAs(t, err, &env)
					require.Equal(t, 3, env.Code)
					require.Equal(t, "event_unreadable", env.Reason)
					require.Contains(t, err.Error(), recorded[0].ID+" differs between")
					require.Contains(t, err.Error(), historical)
					require.Contains(t, err.Error(), filepath.ToSlash(segment))
					require.Nil(t, evs)
				} else {
					require.NoError(t, err)
					require.Empty(t, evs, "compaction alone records no change")
				}
			})
		}
	}
}
