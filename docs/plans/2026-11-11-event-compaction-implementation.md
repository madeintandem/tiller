# Event compaction: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fold event files older than a per-project delay into immutable, content-named JSONL day segments, through `tiller compact`, `tiller sync`, and the schema 14 migration, without any console or CLI read answering differently.

**Architecture:** The `events` package gains a segment codec and its readers take the union of loose YAML files and segments, counting each event id once. A new `mutate.Compact` plans one journaled transaction per day: write the segment, delete the loose files it now holds, remove the emptied directory. Three callers run it: the `compact` verb, a best-effort step at the end of `sync`, and the maintenance flow right after the 13-to-14 stamp. Four places that assumed one file per event are taught the second form first, so compaction is never reachable before every reader understands it.

**Tech Stack:** Go 1.27 (`encoding/json`, `gopkg.in/yaml.v3`, cobra, testify), the store's journaled `Tree.Commit`, `internal/testgit` fixtures.

**Spec:** `docs/plans/2026-11-11-event-compaction-design.md`. Executors read both. `docs/spec/artifact-schema.md` owns the on-disk model and `docs/spec/cli-surface.md` the commands; each changes in the same commit as the behavior it describes.

## Rulings assumed

JC settled the seven decisions in the design. These smaller rules were proposed without objection and are his to overrule before the task that depends on them starts.

| # | Rule | Task |
|---|---|---|
| R1 | One event id with two different records refuses the read and names both paths. | 2, 5 |
| R2 | Segments are never rewritten or deleted, including under `--all`. | 6 |
| R3 | Eligibility is by age alone. Whether a loose file is committed does not matter. | 6 |
| R4 | An event with a key the shape does not define, a value JSON cannot hold, or content that does not round-trip stays loose and is counted as skipped. | 1, 6 |
| R5 | `events.compact_after_days` must be 1 or more. There is no "never" and no "always" value; `--all` covers the one-off. | 6 |
| R6 | With no `git.integration_branch` declared, compaction is allowed on any branch, as usage capture is. | 7, 8 |
| R7 | The migration is the stamp transaction followed by one compaction transaction per day, in the same command. A failed or interrupted compaction is a warning; the next sync finishes it. | 9 |
| R8 | `migrate` and `upgrade --project-only` on a tree already at schema 14 do not compact. That is sync's job. | 9 |
| R9 | Migration inspection (`--inspect`, `migrate --dry-run`) carries a note about compaction, not thousands of planned paths. | 9 |
| R10 | The orientation's "never a whole directory" staging rule gains one exception for the paths sync and compact name, so the orientation generation moves from 59 to 60. | 10 |

## Global Constraints

- The write path does not change: every mutation still writes one YAML file at `.tiller/events/YYYY/MM/DD/<id>.yml`.
- No code path opens an existing segment for writing or deletes one.
- Compaction records no event and needs no contributor identity.
- Compaction is invisible outside the output of `sync`, `compact`, `migrate`, and `upgrade`. No other command or console route may change its output because a day was compacted.
- A segment path is `.tiller/events/YYYY/MM/DD-<hash>.jsonl`, where `<hash>` is the first 12 lowercase hex characters of the SHA-256 of the file's bytes.
- The default delay is 7 whole UTC days. The manifest key is `events.compact_after_days`.
- Managed projects never depend on Tiller: no `.gitattributes` entry, merge driver, or hook.
- New JSON fields are additive and `omitempty`.
- No file grows past 1000 lines. `internal/cli/sync.go` (1081) and `internal/vcs/git.go` (1153) are already over, so nothing is added to them: new code goes in the new files each task names, and Task 8 moves code out of `sync.go`.
- TDD: write the failing test, watch it fail, then write the code.
- Doc and copy style: plain, no em dashes. Say "decisions", never "forks".
- Build in an `event-compaction` worktree. Commit with `git commit -s`. JC fast-forwards main; do not open a PR, stamp a version, or install a binary unless he asks.
- Gate before the last commit of every task: `make lint` and `make test`. Before Task 12's last commit, also the `/verify` skill against a scratch project.
- `site/src/content/docs` is generated at release time. Leave it alone.

## Review Focus

1. **The same event present loose and in a segment** (a partial commit, a revert, a cherry-pick). Every reader counts it once, and the next compaction removes the loose copy without writing a new segment. Tests in Tasks 2, 5, and 6.
2. **Two segments for one day that overlap** (two checkouts compacted from different tips). Readers count each event once and neither segment is touched. Tests in Tasks 2 and 6.
3. **A loose event compaction cannot carry faithfully** (an unknown key, a non-string map key, a value that does not round-trip). It stays loose, the rest of the day compacts, and the run reports it instead of failing. Tests in Tasks 1 and 6.
4. **A work branch seen from a compacted integration checkout** (the execution view reads another branch's events, and the integration's newest ownership event may now sit in a segment while the branch still holds it loose, or the reverse). The task's history and ownership read the same as before. Tests in Task 5.
5. **A day at the UTC boundary.** An event written late in the evening local time belongs to the next UTC day, and eligibility must follow the UTC date the directory already uses. Test in Task 6.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/core/events/segment.go` (new) | Segment wire format: encode, decode, naming, value equality, strict loose decode, type normalization. |
| `internal/core/events/events.go` | `List`, `ListSince`, new `ListLocated`: union of both forms, one event per id. |
| `internal/core/events/cache.go` | Process cache holds the events of a file, not one event. |
| `internal/core/mutate/compact.go` (new) | `Compact`: eligibility, per-day plan, verify, commit. |
| `internal/store/manifest.go` | `events.compact_after_days`, schema 14. |
| `internal/cli/compact.go` (new) | The `compact` verb and the shared human line. |
| `internal/cli/integration_branch.go` (new) | `onIntegrationBranch`, shared by `usage` and `compact`. |
| `internal/cli/sync_followups.go` (new) | Sync's two best-effort steps: usage capture (moved) and compaction. |
| `internal/cli/maintenance.go` | Compaction after the 13-to-14 stamp. |
| `internal/vcs/commit_reads.go` | `PathsAt`, `FileAt`. |
| `internal/cli/readout.go` | `--since` across a compaction. |
| `internal/execution/segments.go` (new) | The execution source's segment reads. |
| `internal/core/mutate/depends.go` | Read guards by the file an event came from. |

---

### Task 1: Segment codec

**Files:**
- Create: `internal/core/events/segment.go`, `internal/core/events/segment_test.go`
- Modify: `internal/core/events/events.go:305-311` (`decode` normalizes)

**Interfaces:**
- Produces:
  - `func EncodeSegment(evs []Event) ([]byte, error)`: sorts a copy with `Less`, writes one JSON object per line, each ending in `\n`, HTML escaping off. Refuses an empty id, a zero `At`, and two events with one id.
  - `func DecodeSegment(relative string, raw []byte) ([]Event, error)`: errors read `events: parse <relative> line <n>: <cause>`, lines counted from 1. A blank line and a last line without `\n` are errors.
  - `func SegmentRelPath(day time.Time, data []byte) string`: `.tiller/events/2026/10/13-3f9a2c41d0aa.jsonl`.
  - `func SegmentDay(name string) (day int, ok bool)`: `"13-3f9a2c41d0aa.jsonl"` gives `13, true`; anything else gives `0, false`.
  - `func Equal(a, b Event) bool`: value equality; timestamps compare with `time.Time.Equal`.
  - `func DecodeLooseStrict(relative string, raw []byte) (Event, error)`: `decode`, plus a refusal of any key the shape does not define at the top level (`id type at contributor owner command targets before after files`), in a file change (`path before after`), or in a revision (`kind value path observed_at`), and of a second YAML document.
- The wire format uses private structs with JSON tags in `segment.go`. Do not add JSON tags to `Event`, `FileChange`, or `store.RevisionRef`; other JSON surfaces embed those types. Key order per line: `id type at contributor command targets before after files`. A file change: `path before after`. A revision: `kind value path observed_at`. Empty and zero fields are omitted (`omitempty`, and `omitzero` for the revision and its time).
- Normalization, applied by both `decode` (YAML) and `DecodeSegment` to `Before` and `After`, recursively through `[]any` and `map[string]any`: a `time.Time` becomes its `RFC3339Nano` string in UTC; a number with no fractional part that fits `int64` becomes `int`; any other number is `float64`. `DecodeSegment` decodes with `UseNumber`. Both set `At` and every `ObservedAt` to `.UTC()`.

- [ ] **Step 1: Write the failing tests** in `segment_test.go` (the `at` helper is in `events_test.go`):

```go
func TestEncodeSegmentWritesOneSortedLinePerEvent(t *testing.T) {
	later := Event{ID: "EV-bbbbbb", Type: "submit.task", At: at("2026-10-13T09:30:00.5Z"), Contributor: "lantern",
		Command: "submit task T-000001", Targets: []string{"T-000001"},
		Before: map[string]any{"status": "check"}, After: map[string]any{"status": "in_review", "pr": 167},
		Files: []FileChange{{Path: ".tiller/x/task.yml"}}}
	earlier := Event{ID: "EV-aaaaaa", Type: "add.task", At: at("2026-10-13T08:00:00Z"), Contributor: "jc",
		Command: "add task", Targets: []string{"T-000001"}}

	data, err := EncodeSegment([]Event{later, earlier})

	require.NoError(t, err)
	require.Equal(t, `{"id":"EV-aaaaaa","type":"add.task","at":"2026-10-13T08:00:00Z","contributor":"jc","command":"add task","targets":["T-000001"]}
{"id":"EV-bbbbbb","type":"submit.task","at":"2026-10-13T09:30:00.5Z","contributor":"lantern","command":"submit task T-000001","targets":["T-000001"],"before":{"status":"check"},"after":{"pr":167,"status":"in_review"},"files":[{"path":".tiller/x/task.yml"}]}
`, string(data))
}

func TestALooseEventSurvivesTheSegmentRoundTripByValue(t *testing.T) {
	loose, err := decode("x.yml", []byte("id: EV-cccccc\ntype: submit.task\nat: 2026-10-08T04:43:03.534897Z\n"+
		"contributor: lantern\ncommand: submit task T-0z03r7\ntargets:\n    - T-0z03r7\n"+
		"after:\n    pr: 167\n    ratio: 1.5\n    seen: 2026-10-08T04:43:03Z\n    nested:\n        list: [a, 2]\n"+
		"files:\n    - path: a/task.yml\n      after:\n        kind: blob\n        value: git:abc1234\n        observed_at: 2026-10-08T04:43:03Z\n"))
	require.NoError(t, err)

	data, err := EncodeSegment([]Event{loose})
	require.NoError(t, err)
	back, err := DecodeSegment("2026/10/08-x.jsonl", data)
	require.NoError(t, err)

	require.Len(t, back, 1)
	require.True(t, Equal(loose, back[0]))
	require.Equal(t, 167, loose.After["pr"])
	require.Equal(t, 167, back[0].After["pr"])
	require.Equal(t, 1.5, back[0].After["ratio"])
	require.Equal(t, "2026-10-08T04:43:03Z", loose.After["seen"])
	require.Equal(t, "2026-10-08T04:43:03Z", back[0].After["seen"])
	require.Equal(t, []any{"a", 2}, back[0].After["nested"].(map[string]any)["list"])
}
```

Add, in the same style:
  - `TestALegacyOwnerEncodesAsContributor`: a loose `owner: jc` event encodes to a line that contains `"contributor":"jc"` and does not contain `"owner"`.
  - `TestDecodeSegmentNamesTheFileAndLine`: a segment whose second line is `{not json` errors with a message containing `2026/10/13-x.jsonl line 2`.
  - `TestDecodeSegmentRefusesABlankOrTruncatedLine`: `"{...}\n\n"` and a final line with no `\n` both error.
  - `TestSegmentRelPathIsNamedByDayAndContent`: for `data := []byte("x\n")` and day `2026-10-13`, the path equals `filepath.Join(".tiller", "events", "2026", "10", "13-"+hex.EncodeToString(sum[:])[:12]+".jsonl")` with `sum := sha256.Sum256(data)`.
  - `TestSegmentDayReadsOnlySegmentNames`: `13-3f9a2c41d0aa.jsonl` gives 13; `EV-aaaaaa.yml`, `13.jsonl`, and `xx-abc.jsonl` give `ok == false`.
  - `TestDecodeLooseStrictRefusesAKeyTheShapeDoesNotDefine`: `surprise: 1` at the top level errors; so does an unknown key inside a `files` entry; a legacy `owner:` event decodes with `Contributor` set.
  - `TestEncodeSegmentRefusesWhatJSONCannotCarry`: an event whose `Before` is `map[string]any{"m": map[any]any{1: "x"}}` errors.

- [ ] **Step 2: Run them and watch them fail.** `go test ./internal/core/events/ -run 'Segment|Loose|Legacy' -count=1`. Expected: build failure, `undefined: EncodeSegment`.
- [ ] **Step 3: Implement `segment.go`** and make `decode` normalize. Existing tests that asserted a `time.Time` inside `Before` or `After` (if any) now see the string; change the assertion, not the rule.
- [ ] **Step 4: Run the package.** `go test ./internal/core/events/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/core/events && git commit -s -m "Events: a segment codec for compacted days"`

### Task 2: Readers take both forms

**Files:**
- Modify: `internal/core/events/events.go:139-292`, `internal/core/events/cache.go:31-109`
- Test: `internal/core/events/events_test.go`, `internal/core/events/cache_test.go`

**Interfaces:**
- Consumes: Task 1's `DecodeSegment`, `SegmentDay`, `Equal`, `EncodeSegment`, `SegmentRelPath`.
- Produces:
  - `type Located struct { Event Event; Path string }`, `Path` being the repository-relative file the event was read from.
  - `func ListLocated(tree *store.Tree) ([]Located, error)`: the deduplicated stream in stream order. When an id is held by several files, `Path` is a segment in preference to a loose file, and the lexically first segment among several.
  - `List` and `ListSince` keep their signatures and now return the deduplicated union.
- The walk reads `*.yml` inside day directories as today and `*.jsonl` files in month directories for which `SegmentDay` answers. Anything else is ignored. A symlink is still refused.
- Two records with one id that are `Equal` are one event. Two that differ return `events: <id> differs between <pathA> and <pathB>` (R1).
- `ListSince` skips a segment whose day, taken from its month directory and `SegmentDay`, is before `since`'s UTC date.
- The cache keys by path as today but holds every event of the file: rename `cachedEvent` to `cachedFile` with `events []Event`. `misses()` counts files parsed.

- [ ] **Step 1: Write the failing tests.** Add a test helper to `events_test.go`:

```go
// writeSegment encodes evs into a segment for day (2026-10-13) and returns its
// repository-relative path.
func writeSegment(t *testing.T, root, day string, evs ...Event) string
```

Then, using it with `writeEventAt` and `eventIDs` (already in `events_test.go`):
  - `TestListReadsLooseAndCompactedEventsInStreamOrder`: a segment for `2026-10-13` holding `EV-aaaaaa` (08:00) and `EV-cccccc` (10:00), plus a loose `EV-bbbbbb` on `2026-10-13` at 09:00. `eventIDs(List)` is `[EV-aaaaaa EV-bbbbbb EV-cccccc]`.
  - `TestListCountsAnEventOnceWhenLooseAndCompacted`: `EV-aaaaaa` loose and in a segment; `List` has length 1.
  - `TestListCountsAnEventOnceAcrossOverlappingSegments`: segments `{a, b}` and `{b, c}` for one day; `eventIDs(List)` is `[a b c]`.
  - `TestListRefusesOneIDWithTwoDifferentRecords`: `EV-aaaaaa` loose with type `add.task` and in a segment with type `start.task`; the error contains `EV-aaaaaa differs between`, the loose path, and the segment path.
  - `TestListSinceSkipsOlderSegments`: with the cache on (`useCache`), segments for `2026-10-01` and `2026-10-20`; `ListSince(tree, at("2026-10-15T00:00:00Z"))` returns only the later day's events and `misses()` is 1.
  - `TestListLocatedNamesTheSegmentThatSuppliedAnEvent`: an event loose and compacted reports the segment's path.
  - `TestCachedListFollowsACompaction` in `cache_test.go`: with the cache on, `List` over two loose events; then write a segment holding both, remove the day directory, and `List` again. The events are `Equal` to the first read, and a third `List` leaves `misses()` unchanged.
- [ ] **Step 2: Run them and watch them fail.** `go test ./internal/core/events/ -run 'List|Cached' -count=1`.
- [ ] **Step 3: Implement.** Keep `listWhere` as the single walk; have it return located records and let `List`, `ListSince`, and `ListLocated` share one dedupe function.
- [ ] **Step 4: Run the package and the consumers.** `go test ./internal/core/events/ ./internal/views/ ./internal/serve/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -s -m "Events: readers take loose files and segments as one stream"`

### Task 3: Started-task read guards follow the event's file

`startedTaskHistory` (`internal/core/mutate/depends.go:545`) guards each `start.task` event's file by recomputing its dated loose path. A compacted event has no such file.

**Files:**
- Modify: `internal/core/mutate/depends.go:545-570`
- Test: `internal/core/mutate/depends_test.go`

**Interfaces:**
- Consumes: `events.ListLocated`.
- `startedTaskHistory` keeps its signature. It returns one `store.PlanReadGuard` per distinct file that supplied a `start.task` event.

- [ ] **Step 1: Write the failing tests:**
  - `TestStartedTaskHistoryGuardsACompactedStartEvent`: in a `writeOwnedRepo(t)` tree, write a segment holding one `start.task` event targeting `T-000001`. `started["T-000001"]` is true and the single guard's `Path` is the segment's absolute path.
  - `TestStartedTaskHistoryGuardsASegmentOnce`: two `start.task` events in one segment give one guard.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/core/mutate/ -run StartedTaskHistory -count=1`. Expected: the first fails with a not-exist error from the guard read.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** `go test ./internal/core/mutate/ -run 'StartedTaskHistory|Depends|Link|Unlink' -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -s -m "Depends: started-task guards follow the file an event came from"`

### Task 4: `readout --since` across a compaction

Today the correction block lists every event file git names as added since the sha. A compaction between the sha and `HEAD` adds a segment full of old events, which would all be reported as changed.

**Files:**
- Modify: `internal/cli/readout.go:335-359`, `internal/vcs/commit_reads.go`
- Test: `internal/cli/readout_test.go`; create `internal/vcs/commit_reads_test.go`
- Modify: `docs/spec/cli-surface.md:333` (the sentence beginning "The event files git names as added")

**Interfaces:**
- Produces, in `commit_reads.go`:
  - `func (g *Git) PathsAt(commit, path string) ([]string, error)`: `git ls-tree -r --name-only <commit> -- <path>` from `Dir`, forward-slash paths relative to `Dir`.
  - `func (g *Git) FileAt(commit, path string) ([]byte, error)`: `git show <commit>:./<path>` from `Dir`.
- `readoutEventsSince` returns the events recorded since the sha:
  1. Take `AddedSince(since, ".tiller/events")`.
  2. An added `*.yml` is one event, loaded as today.
  3. An added segment contributes only the events that were not present at the sha. An event was present at the sha when its loose path is in `PathsAt(since, ".tiller/events")`, or when a segment for the same day at the sha holds its id (read with `FileAt` and `DecodeSegment`).
  4. Call `PathsAt` only when step 1 named a segment, so a correction with no compaction behind it costs what it costs today.
  5. Return each id once.

- [ ] **Step 1: Write the failing tests** in `readout_test.go`, beside `TestReadoutSinceRendersTheCorrectionBlock`:
  - `TestReadoutSinceIgnoresEventsACompactionMoved`: commit two loose events on `2026-10-13` and take that sha. Replace them with a segment, add one new loose event with `tiller add`, commit. The `**Changed**` section holds exactly the one new line.
  - `TestReadoutSinceKeepsAnEventCompactedAfterTheSha`: take a sha, add one event, commit, replace that event's file with a segment holding it, commit. `**Changed**` holds that event's line.
  - In `commit_reads_test.go`: `TestPathsAtListsACommitsFiles` and `TestFileAtReadsACommitsBytes`, each also run with the project in a subdirectory of the repository.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/cli/ -run ReadoutSince -count=1`.
- [ ] **Step 3: Implement,** then reword the spec sentence: Changed is the events recorded since the sha, whether they are still loose files or already compacted.
- [ ] **Step 4: Run.** `go test ./internal/cli/ -run Readout -count=1 && go test ./internal/vcs/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -s -m "Readout: the correction block reads through a compaction"`

### Task 5: The execution source reads segments

The execution view reads another branch's events from its committed tree or its worktree (`internal/execution/source.go:455-602`). It collects `*.yml` only, requires the filename to equal the id, and answers `hasEvent` from the dated loose path.

**Files:**
- Create: `internal/execution/segments.go`, `internal/execution/segments_test.go`
- Modify: `internal/execution/source.go:455-602`

**Interfaces:**
- Consumes: `events.DecodeSegment`, `events.SegmentDay`, `events.Equal`, `events.EncodeSegment`, `events.SegmentRelPath`.
- `(*source).events(ctx, taskID)` keeps its signature and returns the task's events from both forms, each id once. Two differing records for one id are an error naming both paths (R1). The filename-equals-id check applies to loose files only.
- `(*source).hasEvent(event)` is true when the dated loose path exists, or when a segment in the source for the event's day holds its id.
- Committed segment blobs go through `gitBatchBlobs` with the loose blobs, in the same single Git process, under the same `maxBatchBlobBytes` bound.

- [ ] **Step 1: Write the failing tests** in `segments_test.go`, using the package's `fixture`, `put`, `git`, `base`, `worktree`, `load`, `ownershipEvent`, and `claimedOnIntegration` helpers:
  - `TestCommittedSourceReadsATasksEventsFromASegment`: a task branch whose events for the task are committed as one segment; `Events(ctx, id)` returns them in stream order.
  - `TestWorktreeSourceReadsATasksEventsFromASegment`: the same through an uncommitted worktree.
  - `TestSourceCountsAnEventOnceWhenLooseAndCompacted`: one event in both forms; length 1.
  - `TestBranchHoldingIntegrationsOwnershipLooseStandsWhenIntegrationCompactedIt`: mirror `TestBranchCarryingIntegrationsNewestOwnershipStands` (`ownership_test.go:72`) with the integration's event in a segment and the branch's copy loose. The branch's ownership stands.
  - `TestBranchHoldingIntegrationsOwnershipInASegmentStands`: the reverse.
  - Extend `TestCommittedSourceEventsGitCallsStayBounded` (`observation_test.go:171`) with a source that holds segments; the Git call count does not grow.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/execution/ -run 'Segment|Ownership' -count=1`.
- [ ] **Step 3: Implement** the segment reads in `segments.go`; `source.go` changes only where it lists and decodes.
- [ ] **Step 4: Run.** `go test ./internal/execution/ ./internal/serve/ -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -s -m "Execution: a branch's events read from segments too"`

### Task 6: Compaction

**Files:**
- Create: `internal/core/mutate/compact.go`, `internal/core/mutate/compact_test.go`
- Modify: `internal/store/manifest.go:98-120` (struct), `:493` (`Validate`); `internal/store/manifest_test.go`
- Modify: `docs/spec/artifact-schema.md`: the layout line at `:93`, the manifest example at `:224` and its table at `:237`, and the Event stream section at `:1048-1051`

**Interfaces:**
- Produces, in `store`:

```go
const DefaultCompactAfterDays = 7

type EventsBlock struct {
	CompactAfterDays *int `yaml:"compact_after_days,omitempty"`
}

// Manifest gains: Events EventsBlock `yaml:"events,omitempty"`
func (m *Manifest) CompactAfterDays() int // the set value, or DefaultCompactAfterDays
```

  `Validate` refuses a set value below 1 with `events.compact_after_days must be a whole number of days, 1 or more` (R5).
- Produces, in `mutate`:

```go
type CompactOptions struct {
	Now    time.Time
	All    bool
	DryRun bool
}

type CompactedDay struct {
	Day       string `json:"day"`               // "2026-11-03"
	Events    int    `json:"events"`            // loose event files removed
	Directory string `json:"directory"`         // ".tiller/events/2026/11/03"
	Segment   string `json:"segment,omitempty"` // empty when every event was already in a segment
}

type CompactResult struct {
	Days    []CompactedDay `json:"days"`
	Events  int            `json:"events"`
	Skipped int            `json:"skipped,omitempty"`
	Stage   []string       `json:"stage,omitempty"`
}

func Compact(root string, opts CompactOptions) (CompactResult, error)
```

- Behavior:
  1. Load the tree with `store.Load(root)`. The delay is `tree.Manifest.CompactAfterDays()`.
  2. A day directory is eligible when `opts.All` is set, or when its date plus the delay is on or before the UTC date of `opts.Now`. With a delay of 7 on 2026-11-11, 2026-11-04 is eligible and 2026-11-05 is not.
  3. Days are handled oldest first, one `tree.Commit("compact", writes)` each.
  4. For each loose `*.yml` in an eligible day: read it with `store.ReadGuardedFileSnapshot`, decode it with `events.DecodeLooseStrict`, and check that `EncodeSegment` then `DecodeSegment` gives back an `events.Equal` event. One that fails either stays where it is and adds to `Skipped` (R4).
  5. An event that an existing segment for the day already holds, `Equal`, is deleted and not re-encoded. One that an existing segment holds with different content stops the run with an error naming both paths, and that day writes nothing (R1).
  6. The remaining events become one new segment at `events.SegmentRelPath`. If that path already holds the same bytes, it is not written again. An existing segment is never written or deleted (R2).
  7. The day's transaction is the segment write, a `store.PlanDeleteFile` per compacted loose file, and a `store.PlanRemoveEmptyDir` for the day directory when nothing remains in it.
  8. `Stage` lists, per day in order, the directory then the segment if one was written, with forward slashes.
  9. `DryRun` plans everything, commits nothing, and returns the same result.
  10. An error returns the days already committed along with the error.
  11. No event is recorded. With nothing eligible, nothing is written and the generation does not move.

- [ ] **Step 1: Write the failing tests** in `compact_test.go`. Build trees with `writeOwnedRepo(t)` and write loose events with a local helper `looseEvent(t, root, id, typ, when string)` that writes the YAML at `events.RelPath`.

```go
func TestCompactFoldsOnlyDaysOlderThanTheDelay(t *testing.T) {
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T11:00:00Z")
	looseEvent(t, root, "EV-cccccc", "add.task", "2026-11-04T10:00:00Z")
	looseEvent(t, root, "EV-dddddd", "add.task", "2026-11-05T10:00:00Z")
	looseEvent(t, root, "EV-eeeeee", "add.task", "2026-11-11T09:00:00Z")
	before := listEvents(t, root)

	res, err := Compact(root, CompactOptions{Now: at("2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 2)
	require.Equal(t, "2026-11-03", res.Days[0].Day)
	require.Equal(t, 2, res.Days[0].Events)
	require.Equal(t, "2026-11-04", res.Days[1].Day)
	require.Equal(t, 3, res.Events)
	require.Equal(t, []string{
		".tiller/events/2026/11/03", res.Days[0].Segment,
		".tiller/events/2026/11/04", res.Days[1].Segment,
	}, res.Stage)
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/03"))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/04"))
	require.FileExists(t, filepath.Join(root, res.Days[0].Segment))
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/05/EV-dddddd.yml"))
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/11/EV-eeeeee.yml"))
	requireSameEvents(t, before, listEvents(t, root))
}
```

Add, in the same style:
  - `TestCompactHonorsTheManifestDelay`: with `events:\n  compact_after_days: 2` in the manifest and the same events, `2026-11-05` compacts too.
  - `TestCompactAllIncludesToday`: `All: true` leaves no day directory.
  - `TestCompactUsesTheUTCDay` (Review Focus 5): one event at `2026-11-04T23:59:00Z`. With `Now` at `2026-11-10T23:59:00Z` nothing compacts; with `Now` at `2026-11-11T00:01:00Z` it does.
  - `TestCompactLeavesAnEventItCannotCarryLoose` (Review Focus 3): one good event and one whose file carries `surprise: 1`, same day. `Skipped` is 1, the odd file and its directory remain, and the good event is in the segment.
  - `TestCompactRemovesALooseCopyAlreadyInASegment` (Review Focus 1): an event loose and in a segment. `Events` is 1, `Segment` is empty, the loose file is gone, and the segment's bytes are unchanged.
  - `TestCompactAddsAStragglerSegmentWithoutTouchingTheFirst` (Review Focus 2): a day with a segment, plus a new loose event for that day. A second segment appears, the first one's bytes are unchanged, and the day's events read once each.
  - `TestCompactRefusesADayWhoseCopiesDisagree`: the error names both paths and no file changed.
  - `TestCompactIsANoOpWhenNothingIsEligible`: `Days` is empty and `.tiller/.cache/generation` holds the same bytes as before.
  - `TestCompactDryRunWritesNothing`: the result names the day and the loose files are still there.
  - `TestCompactRecordsNoEvent`: the event count is the same before and after.
  - `TestTwoCheckoutsCompactingTheSameDayWriteTheSameSegment`: two copies of one tree compact independently; segment name and bytes match.
  - In `manifest_test.go`: `TestCompactAfterDaysDefaultsToSeven`, `TestCompactAfterDaysReadsTheManifest` (14), `TestCompactAfterDaysRefusesZero`.

`listEvents` is `events.List` over a freshly loaded tree; `requireSameEvents` asserts equal length and `events.Equal` pairwise. `at` parses RFC 3339 and fails the test on a bad value; the `mutate` tests have no such helper yet, so define all three in `compact_test.go`.

- [ ] **Step 2: Run and watch them fail.** `go test ./internal/core/mutate/ -run Compact -count=1`. Expected: `undefined: Compact`.
- [ ] **Step 3: Implement** the manifest setting, then `compact.go`.
- [ ] **Step 4: Update `artifact-schema.md`.** The Event stream section says: an event is written as one immutable YAML file; days older than `events.compact_after_days` (default 7) are folded into immutable JSONL segments named by day and content; a reader counts each event id once across both forms; segments are never rewritten or removed; compaction normalizes a legacy `owner:` key and otherwise preserves content. Add `events.compact_after_days` to the manifest example and table, and a segment line to the layout tree. No em dashes.
- [ ] **Step 5: Run.** `go test ./internal/core/mutate/ ./internal/store/ -count=1`. Expected: PASS.
- [ ] **Step 6: Commit.** `git commit -s -m "Compact: fold old event days into immutable segments"`

### Task 7: The `compact` verb

**Files:**
- Create: `internal/cli/compact.go`, `internal/cli/compact_test.go`, `internal/cli/integration_branch.go`
- Modify: `internal/cli/root.go:88` (register after `newUsageCmd()`), `internal/cli/usage.go:101-113` (use the shared helper)
- Modify: `docs/tiller-commands.md` (a `## Compact` section after `## Agent usage` at `:897`), `docs/spec/cli-surface.md` (a section after "Capturing agent usage" at `:845`, and a row in the verb table near `:1241`)

**Interfaces:**
- Consumes: `mutate.Compact`, `mutate.CompactOptions`, `mutate.CompactResult`.
- Produces:
  - `func onIntegrationBranch(tree *store.Tree) (ok bool, current, integration string, err error)` in `integration_branch.go`, which is `usageBranch` moved and renamed. `usage.go` calls it.
  - `func newCompactCmd() *cobra.Command`: `tiller compact [--all] [--dry-run]`, no positional arguments, no contributor flag.
  - `func humanCompaction(res mutate.CompactResult, dryRun bool) string`, also used by Tasks 8 and 9. It returns `""` when `res.Days` is empty. Exact copy:
    - `Compacted 2 days of events (3 events) into 2 files. Stage them with:` then a second line `  git add -A -- ` followed by `res.Stage` joined with spaces.
    - Singular forms: `1 day`, `1 event`, `1 file`.
    - When no segment was written: `Removed 2 loose events already held in segments. Stage them with:` and the same second line.
    - When `res.Skipped > 0`, one more line: `1 event could not be compacted and stays as its own file.` (plural: `2 events could not be compacted and stay as their own files.`)
    - Dry run: `Would compact 2 days of events (3 events) into 2 files.` and no second line.
- Off the integration branch (R6): exit `1`, reason `not_integration_branch`, human `Events are compacted on the integration branch only. You are on <current>; check out <integration> and run this again.`
- With nothing eligible: exit `0`, human `Nothing to compact.`
- JSON data is the `CompactResult` plus `"dry_run": true` on a dry run.
- A `mutation_stale` refusal is exit `1` with `Tiller state changed while events were being compacted. Run tiller compact again.`

- [ ] **Step 1: Write the failing tests** in `compact_test.go`, using `initRepo(t)` and `execCLI`, with a package variable `compactNow = time.Now` the tests set:
  - `TestCompactVerbFoldsEligibleDaysAndPrintsTheStageCommand`: two old days; stdout equals the two-line form above with the real paths; exit 0.
  - `TestCompactVerbSaysNothingToCompact`.
  - `TestCompactVerbAllIncludesToday`.
  - `TestCompactVerbDryRunWritesNothing`: the `Would compact` line, and the loose files remain.
  - `TestCompactVerbRefusesOffTheIntegrationBranch`: mirror `TestUsageRefusesOffIntegrationBranch` (`usage_test.go:119`); exit 1 and the exact sentence.
  - `TestCompactVerbJSONCarriesDaysAndStage`.
  - `TestCompactVerbNeedsNoIdentity`: it succeeds in a checkout with no `.tiller/identity.yml`.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/cli/ -run CompactVerb -count=1`.
- [ ] **Step 3: Implement,** then write the two doc sections: what the verb does, the delay and where it is set, `--all`, `--dry-run`, the integration-branch refusal, and that it records no event.
- [ ] **Step 4: Run.** `go test ./internal/cli/ -run 'CompactVerb|Usage' -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git commit -s -m "CLI: tiller compact"`

### Task 8: Sync compacts

**Files:**
- Create: `internal/cli/sync_followups.go`, `internal/cli/sync_compact_test.go`
- Modify: `internal/cli/sync.go:96-110` (view fields), `:375-386` (call site), and remove `syncCaptureUsage` and `withUsageWarning` at `:1037-1081`, which move to `sync_followups.go` unchanged
- Modify: `docs/spec/cli-surface.md` (a paragraph after "Inside sync." at `:857`), `docs/tiller-commands.md` (the sync section)

**Interfaces:**
- Consumes: `mutate.Compact`, `onIntegrationBranch`, `humanCompaction`.
- Produces:
  - `SyncView` gains `Compacted *mutate.CompactResult` with tag `json:"compacted,omitempty"` and `CompactWarning string` with tag `json:"compact_warning,omitempty"`.
  - `func syncCompactEvents(tree *store.Tree, now time.Time) (res *mutate.CompactResult, warning string)`: runs after usage capture; returns `nil, ""` off the integration branch and when no day was compacted; recovers a panic into a warning; any error becomes `Events not compacted: <reason>` beside whatever days were committed.
  - The human output appends `humanCompaction`'s text as its own block, then the warning if there is one.
  - A package variable `syncNow = time.Now` for tests.
- Sync's exit code and what it recorded never change because of compaction.

- [ ] **Step 1: Write the failing tests** in `sync_compact_test.go`, on a repo built as the existing sync tests build theirs:
  - `TestSyncCompactsEligibleDaysAndPrintsTheStageCommand`: loose events ten days before `syncNow`; stdout contains the `Compacted 1 day of events` line and the `git add -A --` line; `--json` carries `compacted.days` of length 1.
  - `TestSyncSaysNothingAboutCompactionWhenNoDayIsEligible`: stdout does not contain `Compacted` and `--json` has no `compacted` key.
  - `TestSyncOffTheIntegrationBranchDoesNotCompact`: the loose files remain and stdout is silent about it.
  - `TestSyncSurvivesACompactionFailure`: a day whose loose and compacted copies disagree; exit code 0, `compact_warning` begins `Events not compacted:`, and sync's own crossings are as without the bad day.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/cli/ -run 'SyncCompact|SyncSays|SyncOff|SyncSurvives' -count=1`.
- [ ] **Step 3: Implement.** Confirm `wc -l internal/cli/sync.go` is lower than 1081 afterwards.
- [ ] **Step 4: Write the spec paragraph:** after its own work succeeds, sync compacts event days older than the delay; best effort, never fails sync, one line plus the stage command when it compacted something, silent otherwise and off the integration branch, no event recorded.
- [ ] **Step 5: Run.** `go test ./internal/cli/ -run Sync -count=1`. Expected: PASS.
- [ ] **Step 6: Commit.** `git commit -s -m "Sync: compact old event days after its own work"`

### Task 9: Schema 14 and the migration's compaction

**Files:**
- Modify: `internal/store/manifest.go:70` (`CurrentSchemaVersion = 14`), `internal/core/mutate/migrate.go:85` (registry), `internal/cli/maintenance.go:191-210`, `internal/cli/maintenance_result.go:27` and its human rendering
- Modify: every test fixture pinned to schema 13. Commit `456512b` ("Schema: stamp 13 for bug severity") is the template: the same roughly 35 files change the same way.
- Test: `internal/core/mutate/migrate_recovery_test.go`, `internal/cli/migrate_test.go`
- Modify: `docs/spec/artifact-schema.md:179`, `docs/spec/cli-surface.md` ("What migrate rewrites", near `:914`), `docs/spec/distribution.md` (the Migration section near `:209` and each "historical event bytes" phrase)

**Interfaces:**
- Consumes: `mutate.Compact`, `humanCompaction`.
- A registry entry `{from: 13, note: "stamp schema_version 14; event history older than the compaction delay moves into day segments", apply: RenderSchemaVersion(d, 14)}`.
- `projectMaintenanceResult` gains `Compaction *mutate.CompactResult` with tag `json:"compaction,omitempty"`.
- In `localMaintenance`, after `MigrateWithCommand` succeeds with `migrated.From < 14` and outcome `migrated`: call `mutate.Compact(root, mutate.CompactOptions{Now: time.Now()})` with no integration-branch check, and store the result. An error appends `Event history was not fully compacted: <reason>. The next tiller sync on the integration branch finishes it.` to `r.Project.Warnings`; maintenance still succeeds and continues to the refresh (R7).
- The human output prints the first line of `humanCompaction` only, without the stage command; the maintenance output already tells the human to review and commit.
- A tree already at 14 is not compacted by `migrate` or `upgrade --project-only` (R8). Inspection reports the registry note and plans no event paths (R9).

- [ ] **Step 1: Write the failing tests:**
  - `TestMigrateFromThirteenStampsFourteen` in `migrate_recovery_test.go`, shaped like the 12-to-13 test `456512b` added there.
  - `TestMigrateFromThirteenCompactsOldEvents` in `internal/cli/migrate_test.go`: a schema 13 fixture with one loose event ten days old and one from today. After `tiller migrate`: exit 0; the manifest reads 14; the old day is a segment; today's event and the new `migrate.schema` event are loose; `--json` has `project.compaction.days` of length 1.
  - `TestMigrateOnACurrentTreeDoesNotCompact`: a schema 14 tree with a ten-day-old loose event; `tiller migrate` leaves it loose and reports no compaction.
  - `TestMigrateSurvivesACompactionFailure`: a schema 13 fixture with a day whose copies disagree; exit 0, schema 14, and a warning that begins `Event history was not fully compacted:`.
- [ ] **Step 2: Run and watch them fail.** `go test ./internal/core/mutate/ ./internal/cli/ -run Migrate -count=1`.
- [ ] **Step 3: Bump the schema and fix the pinned fixtures.** `git show --stat 456512b` lists them; `grep -rln 'schema_version: 13' internal` finds the literals.
- [ ] **Step 4: Implement** the registry entry and the maintenance step.
- [ ] **Step 5: Update the specs.** Schema 14 in the manifest example. In `cli-surface.md` and `distribution.md`, migrate preserves event content, not event bytes, and the 13-to-14 migration compacts history under the ordinary rules, on the branch that runs it, one transaction per day after the stamp.
- [ ] **Step 6: Run everything.** `make test`. Expected: PASS.
- [ ] **Step 7: Commit.** `git commit -s -m "Schema: stamp 14 and compact event history on upgrade"`

### Task 10: The orientation and the execute skill say how to commit a compaction

**Files:**
- Modify: `internal/gen/orientation.go:209-210` and the `orientationSchemaGen` constant (59 to 60); `internal/gen/orientation_test.go` and any golden it pins
- Modify: `internal/skills/execute/SKILL.md:135`, `:306`, `:316`
- Modify: the generation and schema numbers in `docs/spec/distribution.md:5`, `:305`, `:405`, `docs/spec/skill-set.md:5`, and `docs/tiller-commands.md:3`

**Interfaces:**
- Orientation text, inserted after "never a whole directory.": `The one exception is compacted events: when` `` `tiller sync` `` `or` `` `tiller compact` `` `names paths to stage, stage exactly those, directories included.`
- Execute skill, at each of the three places that tell the agent to commit a sync delta: `If sync reports compacted events, run the git add line it prints and commit those paths with the sync delta. Never commit the deletions without the new segment files.`

- [ ] **Step 1: Write the failing test.** In `orientation_test.go`, `TestOrientationNamesTheCompactionStagingException`: the rendered orientation contains `The one exception is compacted events` and `OrientationGeneration()` is 60.
- [ ] **Step 2: Run and watch it fail.** `go test ./internal/gen/ -run Orientation -count=1`.
- [ ] **Step 3: Make the edits,** regenerate any golden the package pins, and update the schema and generation numbers in the four docs. Leave the version stamp `0.23.0` in those lines alone: it moves with the release stamp, and `scripts/check-doc-versions.sh` enforces that.
- [ ] **Step 4: Run.** `make test && scripts/check-doc-versions.sh`. Expected: PASS. Tests in `internal/cli` and `internal/core/readiness` that pin generation 59 move to 60.
- [ ] **Step 5: Commit.** `git commit -s -m "Orientation: staging a compaction is the one directory exception (generation 60)"`

### Task 11: The remaining docs

**Files:**
- Modify: `docs/spec/ui-surface.md` (Liveness, "Read cost"), `docs/spec/artifact-schema.md` and `docs/spec/cli-surface.md` (their "Decisions settled so far" lists), `docs/roadmap.md`, `docs/design-history.md`

- [ ] **Step 1: `ui-surface.md`.** The read-cost sentence says the process cache holds each loose event file and each segment by path, and reuses a file while the directory listing still reports it unchanged. Nothing else in the console contract changes.
- [ ] **Step 2: Settled decisions.** Move the design's seven decisions into the owning specs: the storage ones (immutable content-named segments, JSONL, one event per id, the delay) into `artifact-schema.md`; the command ones (integration only, sync's best-effort step, no event, the stage command) into `cli-surface.md`.
- [ ] **Step 3: Roadmap and history.** In `docs/roadmap.md`, list event compaction as built and list the three items under "Not built here" in the design as designed but not built. Add a short entry to `docs/design-history.md` on why segments are immutable and content-named and why they are JSONL.
- [ ] **Step 4: Check the prose.** `git diff main -- docs | grep '^+' | grep -c "$(printf '\342\200\224')"` prints `0`, so this work added no em dash. `grep -rn 'historical event bytes' docs/spec` returns nothing.
- [ ] **Step 5: Commit.** `git commit -s -m "Docs: event compaction in the specs, roadmap, and design history"`

### Task 12: Acceptance on a copy of a real project

The bar from the design: no read answers differently because history was compacted. This task proves it on real history and records how long the first compaction takes. The copy never leaves this machine.

**Files:**
- Create: `scripts/compaction-acceptance.sh` (kept, so the check can be rerun before a release)

- [ ] **Step 1: Write the script.** It takes the built binary and a source repository and:
  1. Clones the source with `git clone --no-hardlinks`, removes every remote from the clone, and checks out its integration branch.
  2. Runs `tiller upgrade --project-only --contributor <id>` with the new binary, timing it, and commits the result.
  3. Captures set A, compacted: `tiller status --json`, `tiller ls tasks --json`, `tiller ls features --json`, `tiller next --json`, `tiller readout execute`; and from `tiller serve` on a free port: `/api/recap`, `/api/report`, `/api/report/usage`, `/api/events`, `/api/board`, `/api/build`, `/api/tree`, `/api/work-items`, `/api/queue`, and the node page of every twentieth task id.
  4. Rebuilds the loose form at schema 14: `git checkout HEAD~1 -- .tiller/events`, then deletes every `*.jsonl` under `.tiller/events`. Captures set B.
  5. Restores the segments beside the loose files with `git checkout HEAD -- '.tiller/events/*.jsonl'`, so every old event is in both forms. Captures set C.
  6. Diffs A against B and A against C and exits nonzero on any difference.
- [ ] **Step 2: Run it.** `go build -o "$TMPDIR/tiller-compaction" ./cmd/tiller && scripts/compaction-acceptance.sh "$TMPDIR/tiller-compaction" <path-to-a-real-project>`. Expected: both diffs empty. If a payload carries a clock-derived field, normalize that one field with `jq` in the script and name it in the commit message.
- [ ] **Step 3: Record the numbers** in the commit message: event files before and after, segments written, events skipped, and the wall time of the upgrade.
- [ ] **Step 4: Run `/verify`** against a scratch project: write events, `tiller compact --all`, `tiller compact --dry-run`, a `tiller sync` that compacts, and the console History and node timelines before and after.
- [ ] **Step 5: Commit.** `git commit -s -m "Compaction: an acceptance check against real history"`
