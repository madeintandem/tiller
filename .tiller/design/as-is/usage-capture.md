# Usage capture: agent token usage from harness session logs

This is a pointer page. The command is specified in `docs/spec/cli-surface.md`, "Capturing agent usage: usage"; the files and the cursor in `docs/spec/artifact-schema.md`, "Agent usage: `usage/`"; and the exception to the self-contained rule in `docs/spec/distribution.md`, "Self-contained in the repo". This page says which parts apply to the package, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0, reader version 1).

## What this surface is

`internal/usage` turns the session logs coding agents leave on a developer's machine into committed token counts, credited to the work item and workflow stage each model call served. It is opt-in: nothing here runs unless the project's `tiller.yml` sets `usage.capture: true`.

The package has four parts. Three readers (`NewClaudeReader`, `NewCodexReader`, `NewPiReader`, the last covering Pi and OhMyPi) each know one harness's folders, path encoding, and log lines, and produce a neutral `Session` of `Call` values with their `Marker` signals. `Credit` replays a session in time order and aggregates its calls into rows. `Capture` discovers this project's logs, decides which sessions are new, settled, and in range, and plans the files to write; it takes no lock and writes nothing. `Encode`, `Decode`, `LoadFiles`, and `Path` are the file format, and `Cursor` is capture's memory between runs.

Its callers are few. `internal/cli` runs capture for `tiller usage` and, best effort, after `tiller sync`; it supplies the project paths, the earliest event, the node resolver, the time budget, the commit, and the cursor save. `internal/serve` and `internal/views` read the files back for the Report page's Agent use tab. No skill or orientation text asks an agent to run it.

What it is for and why it may read outside the repository: `docs/spec/distribution.md`, "Self-contained in the repo" (the paragraph beginning "Agent usage capture is a second, bounded exception"). The settled rulings: `docs/spec/artifact-schema.md`, "Decisions settled so far" (the agent usage list), and `docs/spec/cli-surface.md`, "Decisions settled so far" (the two usage lines). What is built and what is not: `docs/roadmap.md`, "Built" (Agent usage) and "Not built yet" (More agent usage readers). The Report tab's metrics belong to the console page.

## Data it owns

Two things, both under `.tiller/`.

The usage files, `.tiller/usage/<harness>/<session-id>.yml`: committed, one per top-level session with subagents folded in, created and grown but never shrunk or deleted. Each holds the harness id, the session id, the reader version, `captured_at`, the first and last call, and rows keyed by UTC hour, node, stage, and model with four non-overlapping token counts plus informational `reasoning`. It carries no person and no content. The shape, the nine stage keys, and the rules are `docs/spec/artifact-schema.md`, "Agent usage: `usage/`", under "No person, no content", "Stages", "Never lose data", and "Reading it". In code: `File`, `Row`, `Tokens`, and `ReaderVersion` in `internal/usage/record.go`, and `Stages` in `internal/usage/stage.go`.

The cursor, `.tiller/.cache/usage.yml`: gitignored, per checkout, holding the reader version, each log's size and modification time as last seen, whether its session has a usage file, and the working directories that never change. Those include other projects' absolute paths: discovery records the working directory of every log it has to judge, which for Codex is every rollout on the machine. The no-person, no-content guarantee covers the committed usage files, not this cache (`docs/spec/artifact-schema.md`, "The cursor"). A missing, corrupt, or other-version cursor reads as empty. See the same section, "The cursor". In code: `internal/usage/cursor.go`.

In memory only: `tiller serve` turns on a process cache of parsed usage files (`EnableProcessCache` in `internal/usage/cache.go`), reused while a file's listing is unchanged; one-shot commands never enable it. `docs/spec/ui-surface.md`, "Agent use", mentions the reuse. The stage display labels are also compiled in here (`Stage.Label`), although the schema describes them as console page copy; they are not in the file format.

## Behavior to preserve

Scope of the read. Read-only, four folders under the user's home (`~/.claude/projects` or `$CLAUDE_CONFIG_DIR/projects`, `~/.codex/sessions` or `$CODEX_HOME/sessions`, `~/.omp/agent/sessions`, `~/.pi/agent/sessions`), never harness configuration, and only sessions whose recorded working directory is a project path or lies beneath one by whole path segments. A folder name only nominates; the working directory in the log decides. See `docs/spec/cli-surface.md`, "Capturing agent usage: usage", under "Where it looks", and `docs/spec/distribution.md`, "Self-contained in the repo". In code: `folderMatcher` and `sessionLogs` in `internal/usage/reader.go`, and each reader's `Discover`.

Tolerance. A folder or log that cannot be read is skipped, counted, kept out of the cursor, and tried again, except a Codex rollout whose first line cannot be opened or parsed, which is skipped and retried without being counted; a line over 8 MiB is drained without being held; a malformed line is skipped. Same section, "Where it looks" (its last sentences), and `docs/spec/artifact-schema.md`, "What capture leaves out".

What is left out. A session active in the last 30 minutes (`SettleAfter`), one that began before the project's first timestamped event, one with no model calls, and everything while the project has no event at all. See `docs/spec/artifact-schema.md`, "What capture leaves out".

Counting. Each call once, with the per-harness field mapping that keeps the four counts from overlapping, Codex's repeated `token_count` skipped, and a response repeated across a session's logs counted once: `docs/spec/artifact-schema.md`, "Agent usage: `usage/`" (the paragraph after the example file).

Crediting. The marker table, how a node is picked from a command's arguments, the branch fallback for Claude Code, subagents inheriting their parent's state, and the accepted roughness: `docs/spec/artifact-schema.md`, "How calls are credited". In code: `verbStages`, `readVerbs`, `skillStages`, and `ShellMarkers` in `internal/usage/markers.go`, and `Credit` in `internal/usage/replay.go`. Three details the table does not state. A verb in neither list, such as `release`, `reassign`, `grade`, `compact`, `usage`, `migrate`, or `upgrade`, is no marker and changes nothing. A node is an epic, feature, task, bug, chore, or tweak; a question or amendment id never becomes one. The retired `intake` skill name still counts as a planning skill.

Writing. Never delete, never shrink, skip a rewrite that would change only `captured_at`; plan the write and let the caller commit it under the mutation lock with snapshot and generation guards; record no event; save the cursor only after the commit, or when there was nothing to write. See `docs/spec/artifact-schema.md`, "Never lose data", "Written without an event", and "The cursor".

The command's side of the contract, which lives in `internal/cli` and is listed here because later work on capture must keep it: the three exit `1` refusals (`usage_capture_off`, `not_integration_branch`, `mutation_stale`), the per-harness output lines and `--json` fields, the 2-second budget inside `sync` with the first changed session always read, and the warning lines that never fail sync. See `docs/spec/cli-surface.md`, "Capturing agent usage: usage", under "What it prints", "Cost", "Inside sync", and "Turning it on mid-project"; `docs/tiller-commands.md`, "Agent usage".

Isolation. Nothing else walks `.tiller/usage/`: `docs/spec/artifact-schema.md`, "Nothing else walks it". A new walker over `.tiller/` has to say whether it includes it.

## Edges

Inputs from outside the repository: the four harness log folders, in formats their vendors own and may change. Each reader isolates one format, and `ReaderVersion` exists so a later reader can recompute files. Two environment variables move a folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`).

Inside Tiller, the package imports only `internal/store`, for confined reads, guarded snapshots, planned writes, and the atomic cursor write. Its caller supplies everything else: project paths (from `git worktree list` through `internal/vcs`), the earliest event (from `internal/core/events`), a resolver from id or key to node id, the clock, and the budget. `internal/cli` commits the planned writes through the tree's ordinary commit.

Readers of its output: `internal/serve` calls `LoadFiles`, which fails the whole read on one file that does not parse, and hands the files to `internal/views`, which uses the stage and token types. Nothing in a managed project's build or CI reads these files.

## Described paths

- `internal/usage`

## Discrepancies and gaps

A skill load with arguments that name no node clears the node. `docs/spec/artifact-schema.md`, "How calls are credited", says a skill load sets the node to "the node named, if any, else unchanged". In `apply` (`internal/usage/replay.go`), a skill marker whose argument text leaves at least one candidate, after flags and a leading noun word are dropped, sets the node to whatever resolves, which is empty when nothing does. A bare noun or flags-only text leaves the node alone, and an unresolved token shaped like a node id is kept as written. Only Claude Code's skill calls pass argument text (`internal/usage/claude.go`); `tiller guide <skill>` and Pi pass none and leave the node alone. Read from the code, not run.

A malformed usage file is not an environment failure. `docs/spec/cli-surface.md`, "Capturing agent usage: usage", says any other failure, "such as a malformed usage file already in the repo", is an environment failure. Capture returns a plain error for a usage file that does not decode (`session` in `internal/usage/capture.go`), and the CLI's `classify` (`internal/cli/envelope.go`) maps an untyped error to exit `2` with reason `error`. The path is reached only for a session whose log is new or changed, settled, and in range. Read from the code, not run: `TestSyncCaptureFailureIsAWarning` exercises the same failure through `sync`, and no test asserts the `tiller usage` exit code.

An unreadable Codex rollout is not counted. `docs/spec/cli-surface.md`, "Capturing agent usage: usage", and `docs/spec/artifact-schema.md`, "What capture leaves out", say a log that cannot be read is counted. Codex discovery returns without counting when a rollout's first line cannot be opened or parsed (`internal/usage/codex.go`); Claude Code and Pi logs, folders, and parse failures are counted. Read from the code, not run.

Nothing else found in what was checked. Nothing a source describes is unbuilt here: the further readers (OpenCode, Gemini CLI, OpenTelemetry) and dollar costs appear only in `docs/roadmap.md`, "Not built yet", and the specs do not describe them as existing.

What was checked: `capture.go`, `cursor.go`, `record.go`, `markers.go`, `replay.go`, and `stage.go` in full; the reader interface, folder matching, and working-directory rule in `reader.go`; each reader's folders and `Discover`; and the command wiring in `internal/cli/usage.go` and `sync_followups.go`. Not checked line by line: the three log parsers' field mappings, the 8 MiB line scanner's internals, the stated time budgets (nothing was run), and any test file.
