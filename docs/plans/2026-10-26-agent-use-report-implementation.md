# Agent Use Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture per-work-item agent token usage from Claude Code, Codex, and Pi/OhMyPi session logs into committed `.tiller/usage/` files, and show it on a new "Agent use" tab of the console's Report page.

**Architecture:** A new `internal/usage` package holds the neutral record format, a harness-neutral replay that credits model calls to work items and stages from the Tiller commands each session ran, one small reader per harness, and an incremental capture that plans store writes. `tiller usage` and a time-budgeted hook in `tiller sync` run capture. `views.UsageReportOf` projects the files into a ready-to-draw payload served at `GET /api/report/usage`, and the Report page gains Project and Agent use tabs built from the existing report chart parts.

**Tech Stack:** Go (cobra, `gopkg.in/yaml.v3`, testify), React + TypeScript + Vitest/RTL, hand-written SVG charts.

**Spec:** `docs/plans/2026-10-26-agent-use-report-design.md` (approved by JC 2026-10-26). Read it before any task; this plan argues from it.

## Global Constraints

- No metric on a person: usage files, the payload, and every chart carry no contributor, owner, or name of a person. Capture records no event.
- Missing usage is unknown, never zero.
- Capture is off unless `tiller.yml` has `usage: { capture: true }`; `tiller init --capture-usage` writes that block.
- Capture reads only `~/.claude/projects`, `~/.codex/sessions`, `~/.omp/agent/sessions`, `~/.pi/agent/sessions` (honoring `CLAUDE_CONFIG_DIR`, `CODEX_HOME`), read-only, only sessions whose working directory is the checkout root, a `git worktree list` path, or beneath one.
- Usage file path: `.tiller/usage/<harness>/<session-id>.yml`; harness ids `claude-code`, `codex`, `pi`.
- Capture never deletes a usage file and never rewrites one with fewer calls than it records.
- Capture skips sessions whose last call is under 30 minutes old, writes only on `git.integration_branch` (anywhere when none is configured), and ignores sessions whose first call precedes the project's earliest event.
- Budgets: no-change capture under 100 ms; first full capture of CALCTP-sized logs under 10 s; sync's capture budget 2 s; `/api/report/usage` under 500 ms cold, 150 ms warm on CALCTP.
- `/api/report` and its payload do not change.
- Stage keys and page labels, in display order: `spec_writing` Spec writing, `planning` Planning, `implementation` Implementation, `checks` Checks, `acceptance_evals` Acceptance evals, `code_review` Code review, `publishing` Publishing, `closeout` Closeout, `other` Other.
- Page copy: tabs "Project" and "Agent use" (URL `?tab=agent-use`, rail label "Report view"); empty tab "No agent usage recorded yet"; legend and tooltip label "Partly measured"; note "Measured: Claude Code since Sep 30 · Pi since Sep 1"; harness labels Claude Code, Codex, Pi. No node ids, keys, status names, or command names on the page.
- Series are never told apart by color alone (JC is color blind).
- Docs: plain prose, no em dashes. Commits signed off (`git commit -s`) with the session's Co-Authored-By trailer. Work in a worktree; JC fast-forwards main.

## Review Focus

1. A log line of several megabytes (a large tool result): the reader skips it without buffering it whole and without failing the session. Test in Task 4.
2. A session that ran in a task worktree under `.worktrees/x` and whose worktree has since been removed: still found by the root-prefix match. Test in Task 7.
3. A harness log deleted or truncated after an earlier capture: the committed file is left exactly as it was. Test in Task 7.
4. Two engineers' sessions with overlapping hours on the same node: the report sums them and nothing is attributed to either person. Test in Task 9.
5. A project with usage files but no events (a fresh ledger), or events but no usage files: the payload renders every empty state with no `NaN` and no division by zero. Test in Task 9 and Task 11.

---

## File Structure

- `internal/store/manifest.go`: `UsageBlock` on `Manifest`.
- `internal/cli/init.go`: `--capture-usage`.
- `internal/usage/record.go`: `File`, `Row`, `Tokens`, encode and decode, path helpers, `LoadFiles`.
- `internal/usage/stage.go`: stage keys and labels.
- `internal/usage/markers.go`: Tiller command and skill marker detection.
- `internal/usage/replay.go`: `Session`, `Call`, `Credit`.
- `internal/usage/reader.go`: `Reader`, `LogRef`, `Roots`, line scanning helper.
- `internal/usage/claude.go`, `internal/usage/codex.go`, `internal/usage/pi.go`: the three readers.
- `internal/usage/cursor.go`: the `.tiller/.cache/usage.yml` cursor.
- `internal/usage/capture.go`: `Capture`.
- `internal/usage/cache.go`: process cache for parsed usage files.
- `internal/vcs/git.go`: `WorktreePaths`.
- `internal/corpus/corpus.go`: skip `.tiller/usage/`.
- `internal/cli/usage.go`, `internal/cli/sync.go`, `internal/cli/root.go`.
- `internal/views/report_usage.go`: `UsageReportOf`.
- `internal/serve/report_usage.go`, `internal/serve/serve.go`.
- `ui/src/api.ts`; `ui/src/views/Report.tsx`; `ui/src/views/report/ProjectTab.tsx`, `AgentUseTab.tsx`, `UsageWeekCharts.tsx`, `StageCharts.tsx`, `FeatureUsageChart.tsx`, `ItemUsageChart.tsx`; `ui/src/views/report/KpiStrip.tsx` (export `Tile`).
- Specs and docs in Task 14.

---

### Task 1: The capture setting and `init --capture-usage`

**Files:**
- Modify: `internal/store/manifest.go` (the `Manifest` struct at line 98)
- Modify: `internal/cli/init.go` (`newInitCmd` at line 77, `scaffold` at 268, `renderManifest` at 384)
- Test: `internal/store/manifest_test.go`, `internal/cli/init_test.go`

**Interfaces:**
- Produces: `type UsageBlock struct { Capture bool \`yaml:"capture"\` }`; `Manifest.Usage UsageBlock \`yaml:"usage,omitempty"\``. Later tasks read `tree.Manifest.Usage.Capture`.

- [ ] **Step 1: Write the failing tests**

`TestLoadManifestReadsUsageCapture`: a manifest with `usage:\n  capture: true\n` parses with `m.Usage.Capture == true`; one without the block parses with `false`.

`TestInitCaptureUsageWritesTheBlock`: `execCLI(t, "init", "--name", "P", "--key", "p", "--kind", "greenfield", "--capture-usage")` exits 0 and `.tiller/tiller.yml` contains `usage:\n  capture: true`; reloaded, `Manifest.Usage.Capture` is true. `TestInitWithoutCaptureUsageWritesNoUsageBlock`: the same without the flag leaves no `usage:` key.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/store/ ./internal/cli/ -run 'UsageCapture|CaptureUsage' -v`
Expected: FAIL (unknown flag, missing field).

- [ ] **Step 3: Implement**

Add `UsageBlock` and the field. Add `cmd.Flags().Bool("capture-usage", false, "record agent token usage from local harness logs (writes usage.capture: true)")`, thread a `captureUsage bool` through `scaffold` into `renderManifest`, which appends the block with a one-line comment above it. Do not change `ParseManifest`'s decoding (it is already non-strict, which is what lets older binaries ignore the block).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ ./internal/cli/ -run 'UsageCapture|CaptureUsage|TestInit' -v`
Expected: PASS, and the existing init tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/store/manifest.go internal/store/manifest_test.go internal/cli/init.go internal/cli/init_test.go
git commit -s -m "Usage: add the usage.capture setting and init --capture-usage"
```

---

### Task 2: The usage record, stages, loading, and corpus exclusion

**Files:**
- Create: `internal/usage/record.go`, `internal/usage/stage.go`, `internal/usage/record_test.go`
- Modify: `internal/corpus/corpus.go` (the `fs.WalkDir` callback near line 111)
- Test: `internal/corpus/corpus_test.go`

**Interfaces:**
- Produces:
  - `type Stage string` with constants `SpecWriting = "spec_writing"`, `Planning`, `Implementation`, `Checks`, `AcceptanceEvals`, `CodeReview`, `Publishing`, `Closeout`, `Other`; `var Stages = []Stage{...}` in display order; `func (s Stage) Label() string`.
  - `type Tokens struct { Input, CacheWrite, CacheRead, Output, Reasoning int64 }` with yaml tags `input`, `cache_write`, `cache_read`, `output`, `reasoning,omitempty`; `func (t Tokens) Total() int64` (Input+CacheWrite+CacheRead+Output, never Reasoning); `func (t *Tokens) Add(o Tokens)`.
  - `type Row struct { Hour time.Time \`yaml:"hour"\`; Node string \`yaml:"node,omitempty"\`; Stage Stage \`yaml:"stage"\`; Model string \`yaml:"model"\`; Calls int \`yaml:"calls"\`; Tokens \`yaml:",inline"\` }`
  - `type File struct { Harness, Session string; Reader int; CapturedAt, FirstCall, LastCall time.Time; Rows []Row }` with the yaml keys in the design doc's example.
  - `func (f File) Calls() int`; `func Path(harness, session string) string` returning `.tiller/usage/<harness>/<session>.yml`; `func Encode(f File) ([]byte, error)` (rows sorted by hour, node, stage, model); `func Decode(data []byte) (File, error)`.
  - `func LoadFiles(root string) ([]File, error)`: reads `.tiller/usage/*/*.yml` through `store.ReadRepositoryDirectory` and `store.ReadRepositoryFile`; a missing `.tiller/usage` is an empty result.
  - `const ReaderVersion = 1`.

- [ ] **Step 1: Write the failing tests**

`TestEncodeDecodeRoundTrip`: a `File` with two rows (one with `Node: ""`) survives `Encode` then `Decode` unchanged, and the encoded bytes contain no `node:` line for the empty-node row and no `reasoning:` line when it is zero.
`TestTotalExcludesReasoning`: `Tokens{Input:1, CacheWrite:2, CacheRead:3, Output:4, Reasoning:100}.Total() == 10`.
`TestPathShape`: `Path("claude-code", "abc") == ".tiller/usage/claude-code/abc.yml"`.
`TestLoadFilesMissingDirIsEmpty` and `TestLoadFilesReadsEveryHarness` (two harness folders, three files, returns three).
`TestStagesDisplayOrderAndLabels`: `Stages` equals the nine keys in Global Constraints order and each `Label()` equals its page label.
`TestWalkSkipsUsageDirectory` in corpus: a `.tiller/usage/pi/x.md` file is never visited.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ ./internal/corpus/ -v`
Expected: FAIL (package and symbols missing; corpus visits the file).

- [ ] **Step 3: Implement** the types and functions above. In `corpus.Walk`, return `fs.SkipDir` when `p == "usage"` and the entry is a directory, with a comment naming the design doc's "Nothing else walks the usage files."

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ ./internal/corpus/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage internal/corpus
git commit -s -m "Usage: the neutral usage record and its loader"
```

---

### Task 3: Marker detection and the crediting replay

**Files:**
- Create: `internal/usage/markers.go`, `internal/usage/replay.go`, `internal/usage/markers_test.go`, `internal/usage/replay_test.go`

**Interfaces:**
- Consumes: `Stage`, `Row`, `Tokens` (Task 2).
- Produces:
  - `type MarkerKind int` with `MarkVerb`, `MarkSkill`, `MarkRead`.
  - `type Marker struct { Kind MarkerKind; Stage Stage; Ref string; Verb string }` (Ref is the raw id or key token, "" when none; for `MarkRead` Stage is "").
  - `func ShellMarkers(command string) []Marker`
  - `func SkillMarker(skill, args string) (Marker, bool)` (skill names with or without the `tiller-` prefix).
  - `type Call struct { At time.Time; Model string; Tokens Tokens; Markers []Marker; Branch string }`
  - `type Session struct { Harness, ID string; Calls []Call; Subagents []Session }`
  - `type Resolver func(ref string) string` (returns a node id, or "" when the ref names nothing in the tree).
  - `func Credit(s Session, resolve Resolver) []Row` (subagents folded in, rows aggregated per UTC hour, node, stage, model). When `resolve` returns "" for a ref matching `^(E|F|T|B|CH|TW)-[a-z0-9]{6}$`, Credit stores the ref as written, so an id from a since-changed ledger is kept rather than dropped.

The verb and skill tables are exactly the design doc's "How usage gets credited" table. `ShellMarkers` matches `tiller <verb>` only where `tiller` is preceded by start of string, whitespace, or one of `;&|(\`'"`, so `.tiller/` and `./cmd/tiller` never match. A `MarkVerb` with no Ref clears the node. `tiller guide <skill>` is a `MarkSkill`; `guide` alone is a `MarkRead`.

The replay algorithm, which the signature does not determine:

```
state := (node "", stage "")
for each call in time order:
    for each marker in call.Markers:
        verb:  stage = m.Stage; node = resolve(m.Ref)
        skill: stage = m.Stage; if m.Ref != "" { node = resolve(m.Ref) }
        read:  if node == "" && m.Ref != "" { node = resolve(m.Ref) }; if stage == "" { stage = Other }
    if node == "" && strings.HasPrefix(call.Branch, "tiller/") { node = resolve(strings.TrimPrefix(call.Branch, "tiller/")) }
    credit call to (hour(call.At), node, stage or Other, call.Model)
subagent: start from the parent's state as of the subagent's first call, then run the same loop
```

- [ ] **Step 1: Write the failing tests**

`TestShellMarkersVerbs` (table): `"tiller start task T-abc123"` gives one `MarkVerb` with Stage `Implementation`, Ref `T-abc123`; `"cd x && tiller check T-abc123 --json"` gives `Checks`; `"tiller submit task T-abc123 --pr 4"` gives `Publishing`; `"tiller sync"` gives `Closeout` with Ref ""; `"tiller show T-abc123"` gives `MarkRead`; `"tiller guide specify"` gives `MarkSkill` `SpecWriting`; `"cat .tiller/tiller.yml"` and `"go run ./cmd/tiller status"` give none.
`TestSkillMarker`: `SkillMarker("tiller-code-review", "")` is `CodeReview`; `SkillMarker("brainstorming", "")` is not ok.
`TestCreditFollowsMarkers`: three calls (start T-a, plain, submit T-a) credit two rows to `T-a`, one `implementation` with two calls and one `publishing`.
`TestCreditMarkerCallBelongsToNewState`: the call issuing `tiller start T-b` after work on `T-a` is credited to `T-b`.
`TestCreditBareMutatingVerbClearsNode`: after `tiller sync` the next call has `Node ""`, stage `closeout`.
`TestCreditBranchFallback`: a call with no markers on branch `tiller/e/f/t` resolves that key.
`TestCreditSubagentInheritsParentState`: a subagent whose first call follows the parent's `start T-a` credits to `T-a`.
`TestCreditUnresolvedIDKeptAsWritten`: resolver returns "" for everything; a ref `T-zzzzzz` is still stored as `T-zzzzzz` (resolver contract: id-shaped refs pass through).
`TestCreditBucketsByUTCHour`: calls at 14:05 and 14:55 UTC share a row; 15:01 starts a new one.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ -run 'Marker|Credit' -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement** the functions above.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ -run 'Marker|Credit' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage
git commit -s -m "Usage: credit model calls to work items and stages from session markers"
```

---

### Task 4: The reader interface and the Claude Code reader

**Files:**
- Create: `internal/usage/reader.go`, `internal/usage/claude.go`, `internal/usage/claude_test.go`, `internal/usage/testdata/claude/` (synthetic fixtures)

**Interfaces:**
- Consumes: `Session`, `Call`, `Marker`, `ShellMarkers`, `SkillMarker` (Task 3).
- Produces:
  - `type LogRef struct { Harness, Path, Session, Parent string }` (Parent is "" for a top-level log).
  - `type Roots struct { Home string; Env func(string) string; Project []string }` (Project is the checkout root plus worktree paths, absolute and cleaned).
  - `type Reader interface { Harness() string; Discover(r Roots) ([]LogRef, error); Parse(logs []LogRef) (Session, error) }` (Parse receives one top-level log and its subagent logs).
  - `func scanLines(path string, keep func(line []byte) bool, visit func(line []byte) error) error`: bufio with a 1 MiB working buffer; a line longer than `maxLineBytes = 8 << 20` is skipped by draining to the newline, never buffered whole.
  - `func NewClaudeReader() Reader` (harness `claude-code`).

Claude Code specifics, pinned by the fixtures: project folders are `<home or $CLAUDE_CONFIG_DIR>/.claude/projects/<encoded>` where `<encoded>` replaces every `/` and `.` in the absolute path with `-`; a folder matches when it equals or starts with the encoding of a Roots.Project path followed by `-`. Top-level logs are `<folder>/<session>.jsonl`; subagents are `<folder>/<session>/subagents/*.jsonl`. A call is an `"type":"assistant"` line with `message.usage`; lines sharing `message.id` are one call (usage counted once, tool_use blocks unioned); `message.model == "<synthetic>"` is skipped. Tokens: `input_tokens` to Input, `cache_creation_input_tokens` to CacheWrite, `cache_read_input_tokens` to CacheRead, `output_tokens` to Output, `output_tokens_details.thinking_tokens` to Reasoning. `Bash` tool_use `input.command` feeds `ShellMarkers`; `Skill` tool_use `input.skill`/`input.args` feeds `SkillMarker`. `gitBranch` fills `Call.Branch`. The keep-filter is `bytes.Contains(line, []byte("\"type\":\"assistant\""))`.

- [ ] **Step 1: Write the failing tests**

`TestClaudeDiscoverMatchesRootAndWorktreePrefix`: a temp home with folders for `/p/app`, `/p/app/.worktrees/x` (no longer in Roots), and `/p/other` discovers the first two only, with the subagent log carrying Parent.
`TestClaudeParseDedupesByMessageID`: a fixture with one message split across three lines yields one call with the usage counted once and both tool calls' markers.
`TestClaudeParseMapsTokens`: the five token fields land as listed above.
`TestClaudeParseSkipsSynthetic`.
`TestClaudeParseSkipsOversizedLine`: a fixture line of 9 MiB is skipped and the calls on either side are parsed (write the big line in the test, not in testdata).
`TestClaudeHonorsConfigDir`: with `Env("CLAUDE_CONFIG_DIR")` set, discovery looks there instead of `~/.claude`.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ -run Claude -v`
Expected: FAIL.

- [ ] **Step 3: Implement** `reader.go` and `claude.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ -run Claude -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage
git commit -s -m "Usage: the Claude Code session reader"
```

---

### Task 5: The Pi and OhMyPi reader

**Files:**
- Create: `internal/usage/pi.go`, `internal/usage/pi_test.go`, `internal/usage/testdata/pi/`

**Interfaces:**
- Consumes: Task 4's `Reader`, `LogRef`, `Roots`, `scanLines`.
- Produces: `func NewPiReader() Reader` (harness `pi`).

Pi specifics: roots `~/.omp/agent/sessions` and `~/.pi/agent/sessions`. A folder name encodes the working directory: OhMyPi uses the home-relative path with `/` replaced by `-` (`/Users/jc/dev/app` under home `/Users/jc` is `-dev-app`); Pi wraps the full path as `--Users-jc-dev-app--`. A folder matches on equality or on the root's encoding followed by `-`. Top-level logs are `<folder>/*.jsonl`; subagents are `<folder>/<parent-file-stem>/*.jsonl`. The session id is the file stem. A call is a line whose `message.role == "assistant"` with `message.usage`: `input` to Input, `cacheWrite` to CacheWrite, `cacheRead` to CacheRead, `output` to Output, `reasoningTokens` to Reasoning; model from `message.model`. `toolCall` content blocks named `bash` feed `arguments.command` to `ShellMarkers`; blocks named `read` whose `arguments.path` starts `skill://` feed the skill name to `SkillMarker`. Keep-filter: `"role":"assistant"`.

- [ ] **Step 1: Write the failing tests**

`TestPiDiscoverBothEncodings`: home `/h`, root `/h/dev/app`; folders `-dev-app` under `.omp` and `--h-dev-app--` under `.pi` are both found, `-dev-application` is not.
`TestPiParseMapsTokensAndMarkers`: a fixture with a bash `tiller start task T-abc123` call and a `read skill://tiller-execute` call yields the two markers and the mapped tokens.
`TestPiSubagentFolderIsParentStem`.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ -run Pi -v`
Expected: FAIL.

- [ ] **Step 3: Implement** `pi.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ -run Pi -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage
git commit -s -m "Usage: the Pi and OhMyPi session reader"
```

---

### Task 6: The Codex reader

**Files:**
- Create: `internal/usage/codex.go`, `internal/usage/codex_test.go`, `internal/usage/testdata/codex/`

**Interfaces:**
- Consumes: Task 4's interface.
- Produces: `func NewCodexReader() Reader` (harness `codex`); `func codexSessionCwd(path string) (string, error)` reading only the first line.

Codex specifics: root `$CODEX_HOME/sessions` or `~/.codex/sessions`, files `YYYY/MM/DD/rollout-*.jsonl`. The working directory is `payload.cwd` on the first line (`type: session_meta`); a file matches when that cwd equals or is beneath a Roots.Project path. The session id is the file stem. `turn_context` lines set the current model (`payload.model`). Each `event_msg` with `payload.type == "token_count"` and non-null `payload.info` is one call from `info.last_token_usage`: Input is `input_tokens - cached_input_tokens`, CacheRead is `cached_input_tokens`, CacheWrite is `cache_write_input_tokens`, Output is `output_tokens`, Reasoning is `reasoning_output_tokens`. `response_item` lines with `payload.type` `function_call` or `custom_tool_call` contribute `ShellMarkers` over their `arguments` or `input` string, attached to the next call.

- [ ] **Step 1: Write the failing tests**

`TestCodexMatchesByFirstLineCwd`: of two rollout files, only the one whose `session_meta` cwd is the root is discovered.
`TestCodexInputExcludesCached`: `input_tokens 22647, cached_input_tokens 6016` gives Input 16631, CacheRead 6016.
`TestCodexMarkersAttachToNextTokenCount`.
`TestCodexHonorsCodexHome`.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ -run Codex -v`
Expected: FAIL.

- [ ] **Step 3: Implement** `codex.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ -run Codex -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage
git commit -s -m "Usage: the Codex session reader"
```

---

### Task 7: Capture, the cursor, and worktree discovery

**Files:**
- Create: `internal/usage/cursor.go`, `internal/usage/capture.go`, `internal/usage/capture_test.go`
- Modify: `internal/vcs/git.go` (beside `WorktreeFor` at line 865)
- Test: `internal/vcs/git_test.go`

**Interfaces:**
- Consumes: Tasks 2 to 6; `store.PlanWrite`, `store.ReadGuardedFileSnapshot`, `store.WriteRepositoryFileAtomic`.
- Produces:
  - `func (g *Git) WorktreePaths() ([]string, error)` (every `worktree <path>` stanza of `git worktree list --porcelain`).
  - `type cursorEntry struct { Size int64; ModTime time.Time; Match bool }` and `type Cursor map[string]cursorEntry` keyed by absolute log path, stored at `.tiller/.cache/usage.yml`; `func LoadCursor(root string) Cursor` (a missing or unreadable cursor is empty); `func (c Cursor) Save(root string) error`.
  - `type Options struct { Root string; Roots Roots; Readers []Reader; Resolve Resolver; Now time.Time; Since time.Time; Budget time.Duration }` (Since is the earliest event time; Budget zero means none).
  - `type HarnessResult struct { Harness string; Found bool; Sessions int; Written int; Tokens int64; Tied int64 }`
  - `type Result struct { Harnesses []HarnessResult; Writes []store.PlannedWrite; Partial bool }` (Partial is true when the budget stopped it).
  - `func Capture(o Options) (Result, Cursor, error)`: discovery and parsing hold no lock; the caller commits `Result.Writes` through `tree.Commit` and then saves the cursor.
  - `const SettleAfter = 30 * time.Minute`.

Capture rules (all from the design doc): skip a log whose size and modification time match the cursor; group subagent logs with their parent and treat the group as changed if any member changed; skip a session whose last call is after `Now - SettleAfter`; skip a session whose first call is before `Since`; when the target file exists with `Calls()` greater than the new file's, plan nothing for it; otherwise plan `store.PlanWrite(Path(...), snapshot, Encode(file))` with `Reader: ReaderVersion` and `CapturedAt: Now`; stop at a session boundary when `Budget` is exceeded and set `Partial`. A harness whose roots do not exist reports `Found: false`.

- [ ] **Step 1: Write the failing tests**

`TestWorktreePathsListsEveryStanza` (vcs, using testgit with one added worktree).
`TestCaptureWritesSettledSessions`: a fixture Claude log whose last call is 2 hours before `Now` produces one planned write at `.tiller/usage/claude-code/<id>.yml` with the credited rows.
`TestCaptureSkipsActiveSession`: last call 10 minutes before `Now` plans nothing.
`TestCaptureSkipsSessionsBeforeSince`.
`TestCaptureNeverShrinks`: an existing file recording 50 calls and a log now holding 20 plans nothing; a log gone entirely plans nothing and deletes nothing.
`TestCaptureUnchangedLogIsNotOpened`: with a cursor entry matching size and mtime, the reader's Parse is never called (use a counting fake reader).
`TestCaptureFindsRemovedInRootWorktree`: a Claude folder encoded for `<root>/.worktrees/gone` with `gone` absent from Roots.Project is still captured.
`TestCaptureBudgetStopsAtSessionBoundary`: a fake reader that sleeps; with `Budget: 1ms` the result is `Partial` and holds whole sessions only.
`TestCaptureReportsMissingHarness`: no `.codex` dir gives `HarnessResult{Harness:"codex", Found:false}`.
`TestCaptureNoChangeIsFast`: with 200 fixture logs all matching the cursor, `Capture` returns in under 100 ms.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/usage/ ./internal/vcs/ -run 'Capture|WorktreePaths' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** `WorktreePaths`, `cursor.go`, and `capture.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usage/ ./internal/vcs/ -run 'Capture|WorktreePaths' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage internal/vcs
git commit -s -m "Usage: incremental capture with the settled, since-ledger, and never-shrink rules"
```

---

### Task 8: `tiller usage` and the sync hook

**Files:**
- Create: `internal/cli/usage.go`, `internal/cli/usage_test.go`
- Modify: `internal/cli/root.go` (the `root.AddCommand` list at line 59), `internal/cli/sync.go`
- Test: `internal/cli/sync_test.go`

**Interfaces:**
- Consumes: `usage.Capture`, `usage.LoadCursor`, `vcs.(*Git).WorktreePaths`, `vcs.(*Git).Branch`, `events.List`, `tree.Commit`.
- Produces: `func newUsageCmd() *cobra.Command`; `func captureUsage(tree *store.Tree, now time.Time, budget time.Duration) (usage.Result, error)` shared by both callers (builds Roots from the home directory, `os.Getenv`, the root, and worktree paths; builds the Resolver from the tree's ids and keys, accepting a shelf key with or without its `bugs/`, `chores/`, or `tweaks/` prefix; Since is the earliest event's time; commits writes under operation name `capture agent usage`; saves the cursor only after the commit succeeds).

Refusals for `tiller usage` follow the CLI's existing envelope (exit 1, `ok:false`, a data block with `reason`): `usage_capture_off` ("Agent usage capture is off. Set usage.capture: true in .tiller/tiller.yml to turn it on.") and `not_integration_branch` (names the current and the integration branch). Success output, human: one line per harness (`Claude Code: 12 sessions, 4 written, 1.2B tokens, 76% tied to a work item`, or `Codex: no session logs found`). JSON data: `{"harnesses":[{"harness","found","sessions","written","tokens","tied_share"}],"partial":false}`. Neither path needs a joined identity.

Sync: after sync's own work succeeds, when `Manifest.Usage.Capture` is true and the branch is the integration branch (or none is configured), reload the tree (sync's own commit advanced the generation, so the old tree's commit would be stale) and call `captureUsage(reloaded, now, 2*time.Second)`. Any error becomes one warning line and a `usage_warning` string in sync's JSON data; sync's exit code and records are unchanged. A partial capture adds the line "Agent usage capture ran out of time; run tiller usage to finish." Off the integration branch, sync adds "Agent usage not captured: not on <integration>."

- [ ] **Step 1: Write the failing tests**

`TestUsageRefusesWhenCaptureOff`: exit 1, `"reason": "usage_capture_off"`.
`TestUsageRefusesOffIntegrationBranch`: on a task branch, `"reason": "not_integration_branch"`.
`TestUsageWritesFilesAndNoEvent`: with `HOME` pointed at a temp dir holding a settled Claude fixture whose cwd is the test repo, exit 0, one file under `.tiller/usage/claude-code/`, and the events directory's file count is unchanged.
`TestUsageNeedsNoIdentity`: with no `.tiller/identity.yml`, it still succeeds.
`TestSyncCapturesWhenEnabled` and `TestSyncCaptureFailureIsAWarning` (an unreadable home folder makes sync exit 0 with `usage_warning` set).
`TestSyncSkipsCaptureWhenOff`: no `.tiller/usage` directory appears.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'Usage|SyncCapture' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** `usage.go`, register `newUsageCmd()` after `newSyncCmd()`, and add the sync hook.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'Usage|Sync' -v`
Expected: PASS, including every existing sync test.

- [ ] **Step 5: Commit**

```bash
git add internal/cli
git commit -s -m "Usage: tiller usage and best-effort capture in sync"
```

---

### Task 9: The usage report view and its endpoint

**Files:**
- Create: `internal/views/report_usage.go`, `internal/views/report_usage_test.go`, `internal/usage/cache.go`, `internal/serve/report_usage.go`, `internal/serve/report_usage_test.go`
- Modify: `internal/serve/serve.go` (beside `mux.HandleFunc("/api/report", ...)` at line 86)

**Interfaces:**
- Consumes: `usage.File`, `usage.Stages`, the views package's `weekStarts`, `newReplay`, `featureStart`, `median`, `medianWindow`, `(*replay).lastAt`, `(*replay).firstAt`, `realChange`, `changeFrom`.
- Produces:
  - `func usage.EnableProcessCache()` and `LoadFiles` consulting it: parsed files keyed by path, size, and modification time, the same pattern as `internal/core/events/cache.go`.
  - `type UsageReportInput struct { Files []usage.File; Events []events.Event; Now time.Time; Location *time.Location }`
  - `func UsageReportOf(t *store.Tree, in UsageReportInput) UsageReport`
  - The payload (JSON tags in snake case, token counts as int64):

```go
type UsageReport struct {
	Weeks          []string            `json:"weeks"`           // identical to Report.Weeks for the same events and clock
	WeekBoundaries []string            `json:"week_boundaries"`
	Harnesses      []UsageHarness      `json:"harnesses"`       // harness, label, since (YYYY-MM-DD, local)
	FullCoverage   string              `json:"full_coverage"`   // latest harness since; "" with no files
	Coverage       []string            `json:"coverage"`        // per week: "none" | "partial" | "full"
	KPIs           UsageKPIs           `json:"kpis"`            // total, cache_read, input, cache_write, output, median_per_item (*int64), items, tied_share
	Weekly         []UsageWeek         `json:"weekly"`          // input, cache_write, cache_read, output per week
	Stages         []UsageStage        `json:"stages"`          // key, label, total, cache_read; all nine, in order
	Models         []string            `json:"models"`          // top four by total, then "Other" when any remain
	ModelStages    []UsageModelStage   `json:"model_stages"`    // stage key, totals aligned to Models
	Features       []UsageFeature      `json:"features"`        // title, done (RFC3339), total, median (rolling, last 5)
	FeaturesLeftOut int                `json:"features_left_out"`
	ItemBands      []UsageBand         `json:"item_bands"`      // label, count; the eight bands in order
	ItemsLeftOut   int                 `json:"items_left_out"`
}
```

Rules, from the design doc: weeks use the Project report's axis (first event, `minReportWeeks`, `maxReportWeeks`); a row's week is its hour converted to `Location`. A week is `none` before the earliest harness `since`, `partial` from there until the week holding `FullCoverage`, `full` after. A work item is a done task, bug, chore, or tweak with its own rows whose first start (`firstAt(id, "in_progress", changeFrom("start.<noun>"))`) is on or after `FullCoverage`; `items_left_out` counts done leaves with rows that fail the date rule. A feature's total is its own rows plus its tasks'; it is plotted when done (`lastAt(fid, "done", realChange)`) and `featureStart` is on or after `FullCoverage`. Bands: `<100K`, `100K–300K`, `300K–1M`, `1M–3M`, `3M–10M`, `10M–30M`, `30M–100M`, `>100M` by `Total()`. `tied_share` is the share of all tokens on rows with a node. The handler mirrors `ReportHandler`: `GET /api/report/usage`, `events.List` on the base tree, `usage.LoadFiles(root)`, `time.Now()`, `time.Local`.

- [ ] **Step 1: Write the failing tests**

`TestUsageReportSharesTheProjectAxis`: for the same events and clock, `UsageReportOf(...).Weeks` equals `ReportOf(...).Weeks`.
`TestUsageReportCoveragePartialBeforeLatestHarness`: Pi rows from Sep 1 and Claude rows from Sep 30 give `FullCoverage` `2026-09-30`, weeks before Sep 1 `none`, weeks Sep 1 to Sep 22 `partial`, the rest `full`.
`TestUsageReportItemsLeaveOutEarlyStarts`: a done task started before `FullCoverage` is counted in `ItemsLeftOut`, not in a band.
`TestUsageReportSumsSessionsWithoutPeople`: two files (two sessions) with overlapping hours on one task sum, and the marshaled JSON contains neither contributor id of the tree.
`TestUsageReportTopFourModelsThenOther`: six models collapse to four plus `Other`, ordered by total.
`TestUsageReportFeatureTotalsIncludeTasks`.
`TestUsageReportEmpty`: no files and no events gives zeros, `median_per_item` null, four weeks, and `encoding/json` output with no `NaN`.
`TestUsageReportHandlerServesJSON` (serve): 200 and a `weeks` array.
`BenchmarkUsageReportOf` over 100 synthetic files of 25 rows, for the Task 14 measurement.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/views/ ./internal/serve/ ./internal/usage/ -run 'UsageReport' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** the cache, `UsageReportOf`, the handler, and the route; call `usage.EnableProcessCache()` where `serve` already enables the event cache.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/views/ ./internal/serve/ ./internal/usage/ -v`
Expected: PASS, and `go test ./internal/views/ -run Report` (the Project report) still passes unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/views internal/serve internal/usage
git commit -s -m "Usage: the agent use report view and /api/report/usage"
```

---

### Task 10: Report tabs

**Files:**
- Create: `ui/src/views/report/ProjectTab.tsx`, `ui/src/views/report/AgentUseTab.tsx` (a stub that fetches and renders the empty state)
- Modify: `ui/src/views/Report.tsx`, `ui/src/api.ts` (beside `reportPath` at line 1681 and `Report` at 1546)
- Test: `ui/src/components/__tests__/report-page.test.tsx`

**Interfaces:**
- Consumes: Task 9's payload.
- Produces: TypeScript `UsageReport` and its parts mirroring the Go JSON; `export const usageReportPath = "/api/report/usage"`; `ProjectTab({ report }: { report: Report })` holding today's KPI strip and five charts unchanged; `AgentUseTab()` which calls `useFetch<UsageReport>(usageReportPath)` itself, so the usage endpoint is fetched only when the tab renders.

`Report.tsx` keeps the header and off-branch notice, then `NodeTabs` with `label="Report view"`, `active={search.get("tab") ?? "project"}`, tabs `{ key: "project", label: "Project" }` and `{ key: "agent-use", label: "Agent use" }`, selecting through `useSearchParams` as `Home.tsx` does.

- [ ] **Step 1: Write the failing tests**

`it("shows the Project tab by default and does not fetch usage")`: the existing charts render, and the fetch spy was never called with `/api/report/usage`.
`it("opens Agent use from ?tab=agent-use")`: rendering at `/report?tab=agent-use` with usage stubbed empty shows "No agent usage recorded yet".
`it("reads an unknown tab as Project")`.

- [ ] **Step 2: Run them to verify they fail**

Run: `cd ui && npx vitest run src/components/__tests__/report-page.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement** the tabs, the move of today's content into `ProjectTab`, the API types, and the stub.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd ui && npx vitest run src/components/__tests__/report-page.test.tsx src/components/__tests__/report-charts.test.tsx`
Expected: PASS, every existing report test included.

- [ ] **Step 5: Commit**

```bash
git add ui/src
git commit -s -m "Console: Project and Agent use tabs on the Report page"
```

---

### Task 11: Agent use KPI strip, coverage note, and weekly charts

**Files:**
- Create: `ui/src/views/report/UsageWeekCharts.tsx`, `ui/src/components/__tests__/report-usage.test.tsx`
- Modify: `ui/src/views/report/AgentUseTab.tsx`, `ui/src/views/report/KpiStrip.tsx` (export `Tile`), `ui/src/views/report/format.ts`

**Interfaces:**
- Consumes: `ChartFrame`, `weekPlot`, `WeekAxis`, `WeekHits`, `weekRows`, `HatchPattern`, `usePatternId`, `columnPath`, `useTooltip`, `ChartCard`, `Tile`.
- Produces: `export function formatTokens(n: number): string` in `format.ts` (`6.03B`, `186M`, `8.9M`, `412K`, `950`); `CacheReadsChart` and `FreshTokensChart` taking `{ usage: UsageReport }`.

The note above the strip renders "Measured: " then each harness as "<label> since <Mon D>", joined by " · ". The five tiles are the design doc's: Total tokens; Cache reads with "<pct>% of all" beneath; Fresh input and cache writes; Output; Median per work item with "<items> items · <pct>% of tokens tied to a work item" beneath, or "Not yet" muted when null. The two weekly charts sit side by side like Throughput and Bugs. `FreshTokensChart` stacks Fresh input, Cache writes, Output bottom-up with Cache writes hatched. A `partial` week is drawn with the hatch the page already uses for the partial current week and its tooltip leads with "Partly measured"; a `none` week draws no bar and its tooltip reads "Not measured". The legend gains a "Partly measured" hatched entry only when a partial week is present.

- [ ] **Step 1: Write the failing tests**

`it("renders the coverage note with each harness and date")`: expects text "Measured: Claude Code since Sep 30 · Pi since Sep 1".
`it("renders the five tiles with cache reads split out")`: "6.03B", "5.84B", "96.8% of all".
`it("marks partial weeks and adds the legend entry")`: a payload with one partial week shows the "Partly measured" legend item; one with none does not.
`it("draws no bar for an unmeasured week")`.
`it("formats token counts")` for `formatTokens` with the five examples above.
`it("draws no NaN for an empty payload")`: no attribute value in the SVG contains `NaN` or `Infinity`.

- [ ] **Step 2: Run them to verify they fail**

Run: `cd ui && npx vitest run src/components/__tests__/report-usage.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement** the note, strip, and both charts inside `ChartCard`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd ui && npx vitest run src/components/__tests__/report-usage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/src
git commit -s -m "Console: Agent use headline figures and weekly token charts"
```

---

### Task 12: Stage and model charts

**Files:**
- Create: `ui/src/views/report/StageCharts.tsx`
- Modify: `ui/src/views/report/AgentUseTab.tsx`, `ui/src/components/__tests__/report-usage.test.tsx`

**Interfaces:**
- Produces: `StageChart` and `ModelStageChart`, each `{ usage: UsageReport }`; `export const MODEL_COLORS = ["var(--st-active)", "var(--st-in_review)", "var(--st-specced)", "var(--st-done)", "var(--ink-3)"]`, assigned by index into `usage.models`, so a model keeps its color in every chart.

`StageChart`: one horizontal bar per stage with tokens (all nine stages, zero-token stages drawn as an empty track labeled "0"), the total as a direct label, and the cache-read share in a right-hand column. `ModelStageChart`: a 100 percent bar per stage with a nonzero total, segments in `usage.models` order with a 2 px panel gap, a percent label inside each segment at least 12 percent wide, and a legend naming every model. The two sit side by side. Hover shows the stage, each model, and its share.

- [ ] **Step 1: Write the failing tests**

`it("lists every stage with its cache-read share")`: nine row labels in order, and "94.9%" beside Checks for the fixture.
`it("labels wide model segments and names every model in the legend")`.
`it("keeps a model's color the same across charts")`: the swatch color for `usage.models[1]` equals its segment fill.

- [ ] **Step 2: Run them to verify they fail**

Run: `cd ui && npx vitest run src/components/__tests__/report-usage.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement** both charts.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd ui && npx vitest run src/components/__tests__/report-usage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/src
git commit -s -m "Console: Agent use stage and model charts"
```

---

### Task 13: Feature and work-item charts, and the rebuilt bundle

**Files:**
- Create: `ui/src/views/report/FeatureUsageChart.tsx`, `ui/src/views/report/ItemUsageChart.tsx`
- Modify: `ui/src/views/report/AgentUseTab.tsx`, `ui/src/views/report/scale.ts`, `ui/src/components/__tests__/report-usage.test.tsx`, `ui/dist/` (rebuilt)

**Interfaces:**
- Produces: `export function logScale(domain: [number, number], range: [number, number]): (v: number) => number` in `scale.ts`; `FeatureUsageChart` and `ItemUsageChart`, each `{ usage: UsageReport }`.

`FeatureUsageChart` is full width on the shared week axis, placed by `dateX` like the lead and cycle chart: a filled dot per feature at `done`, a solid line through each feature's rolling `median`, a log y axis with ticks at powers of ten and the axis title "Tokens (log scale)", hover naming the feature title and its total, and beneath it "N earlier features not shown" when `features_left_out > 0`. Empty state: "No features completed yet". `ItemUsageChart` is full width: one column per band in order with its count as a direct label, "Median <formatTokens> · <items> items" beneath, and "N earlier items not shown" when `items_left_out > 0`. Empty state: "No work items finished yet".

- [ ] **Step 1: Write the failing tests**

`it("plots features on a labeled log axis")`: the axis title text is present and three dots render for a three-feature fixture.
`it("notes features left out")`.
`it("shows every band with its count")`: eight band labels in order.
`it("logScale maps decades evenly")`: `logScale([1e6, 1e8], [0, 100])(1e7) === 50`.

- [ ] **Step 2: Run them to verify they fail**

Run: `cd ui && npx vitest run src/components/__tests__/report-usage.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement** both charts and `logScale`.

- [ ] **Step 4: Run all UI tests and rebuild the bundle**

Run: `cd ui && npm test && cd .. && make ui`
Expected: every test passes; `ui/dist` is rebuilt.

- [ ] **Step 5: Commit**

```bash
git add ui/src ui/dist
git commit -s -m "Console: Agent use feature and work-item charts"
```

---

### Task 14: Specs, docs, and the performance check on CALCTP

**Files:**
- Modify: `docs/spec/artifact-schema.md` (directory layout, root manifest, a new "Agent usage: `usage/`" section, settled decisions), `docs/spec/cli-surface.md` (`tiller usage`, sync, `init --capture-usage`), `docs/tiller-commands.md`, `docs/spec/ui-surface.md` (Report: tabs and an "Agent use" subsection; page-wide rules cover both tabs), `docs/spec/distribution.md` (the second self-contained exception), `docs/roadmap.md` (built; OpenCode and Gemini CLI readers not built)
- Delete: `docs/plans/2026-10-26-agent-use-report-design.md`, `docs/plans/2026-10-26-agent-use-report-implementation.md` (AGENTS.md: plans are deleted when built, with durable decisions folded into the owning specs first)

- [ ] **Step 1: Fold the design into the specs.** Each spec section states the behavior the code now has, in the house style (plain prose, no em dashes). Move the design doc's settled decisions into the owning specs' "Decisions settled so far".

- [ ] **Step 2: Verify the docs build and contain no em dashes**

Run: `grep -rn "$(printf '\342\200\224')" docs/spec docs/tiller-commands.md docs/roadmap.md | wc -l` (expect the same count as before this task) and `cd site && npm run build`.
Expected: no new em dashes; the site builds.

- [ ] **Step 3: Measure the budgets on CALCTP's real logs, read-only**

Build the binary, then in a throwaway clone of `~/dev/calctp-platform` with `usage: { capture: true }` (never in the real checkout; nothing from this run is committed or pushed anywhere): time `tiller usage` cold (expect under 10 s), then again (expect under 100 ms); time `tiller sync` with nothing new (expect no more than its pre-change time plus 100 ms); run `tiller serve` and time `/api/report/usage` cold and warm with `curl -w '%{time_total}'` (expect under 0.5 s and 0.15 s) and `/api/report` (expect unchanged from before). Also check that the share of tokens tied to a work item for 2026-09-05 to 2026-10-22 lands near the spike's 74 percent.
Expected: every budget met. A missed budget is fixed before this task closes, not recorded as a follow-up.

- [ ] **Step 4: Run the full gates**

Run: `make lint && make test && (cd ui && npm test)`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add docs
git commit -s -m "Docs: agent usage capture and the Report's Agent use tab"
```
