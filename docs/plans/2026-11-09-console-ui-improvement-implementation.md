# Console UI improvement hunt: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the six defects and build the seven improvements and sixteen smaller wins recorded in the console UI improvement hunt, so the console never shows a raw enum, leads with titles, loads every node quickly, and keeps its counts, statuses, and timestamps consistent across pages.

**Architecture:** Three shared display modules land first (status labels, event phrases, timestamps) and every later task uses them, so no surface formats these on its own again. Where a page lacks data it needs (titles, criterion statements, plan counts, display status), the Go projection in `internal/views` or `internal/serve` gains an additive JSON field and `ui/src/api.ts` mirrors it. Everything else is React and CSS inside the existing views. The work ships as seven batches, each of which passes the full gates and can be fast-forwarded to main on its own.

**Tech Stack:** Go (net/http handlers, testify, `internal/testgit` fixtures), React + TypeScript (Vitest, Testing Library, jsdom), plain CSS with `?raw` regex tests.

**Spec:** `docs/plans/2026-11-09-console-ui-improvement-hunt.md`. Executors read both. Item numbers below (D1 to D6, I1 to I7, and the smaller wins by name) refer to it. `docs/spec/ui-surface.md` owns the console contract and changes in the same commit as the behavior it describes.

## Rulings assumed

The hunt left seven questions open. This plan is written against the answers below. Each is JC's to overrule before its batch starts; the task that depends on it is named.

| # | Question | Assumed answer | Task |
|---|---|---|---|
| R1 | Attest command | Show `--status <pass|fail>`, the way bug triage shows `--severity <...>`. An attestation is the human gate and should not be a blind copy. Ruled by JC 2026-11-09. | 9 |
| R2 | Catching up | A button. Viewing History no longer moves the horizon. | 25 |
| R3 | Milestones in History | Lifecycle crossings, attest and eval, question asked, answered, and cancelled, amendment events, review recovery, shelf resolve, and any event that asked the reader. Link, unlink, order, rename, remove, configure, claim, release, reassign, transfer, move, grade, migrate, roster changes, and plain adds are bookkeeping. Linking and reordering are never milestones. | 22 |
| R4 | Where Drift lives | A Home tab beside Branches and Recovered reviews. `/drift` redirects to it. | 28 |
| R5 | Empty board columns | Collapse to a slim rail with the name and a zero. | 18 |
| R6 | Global search | A topbar field on every page, focused by `/`. It replaces the navigator's search box and Search mode's own input. | 29 |
| R7 | Decision card density | Attest cards show the criterion statement clamped to three lines. Plan cards show one line of counts. Nothing more goes on the card. | 9, 10 |

Building everything in the hunt also overturns sentences the console spec states today. Each is a ruling of the same weight as the seven above.

| # | Spec today (`docs/spec/ui-surface.md`) | This plan | Task |
|---|---|---|---|
| R8 | Report "has no export" | A Print button and a print stylesheet. The browser prints; the server exports nothing. | 35 |
| R9 | A question's "full statement is not used as a row handle" | The Questions ledger shows the statement as a clamped body under the title. The title stays the handle. | 31 |
| R10 | "A node's whole reading pane, header included, scrolls together" | The tab rail and a one-line header stay pinned while the pane scrolls. | 13 |
| R11 | Up Next: "the command stays off the row" (code comment, `Pulse.tsx:315`) | Up Next rows carry a copy control. | 11 |
| R12 | Drift: "`stale_spec` and `orphan_eval` have no invented command" | Unchanged. Those findings get a static next-step sentence, never a command. | 27 |
| R13 | The workflow reason string is shared by the CLI readout | The sentence is rewritten in Go, so `tiller status` reads the same new words. | 16 |

Two hunt inferences did not survive reading the code, and the plan follows the code:

- **"1000 events" is not a read cap.** `views.RecapOf` sets `Total` before any cap and `/api/recap` has no limit. Task 6 checks the number on Acme and changes nothing if it is the true count.
- **Filter mode has no free-text inputs.** Each axis is a `FilterCombobox` fed by server facets. Task 32 makes it show its options on focus and adds the column header.

## Global Constraints

- The console stays read-only: every `/api` route answers GET and HEAD only. No task adds a write route. The History catch-up button reuses the existing `X-Tiller-Visit: 1` GET.
- New JSON fields are additive and `omitempty`. The CLI's `tree`, `show`, `status`, `ls`, and `search` JSON never carry a board `column` (JC ruling 2026-10-20): display status is a console projection.
- No raw status token, event type, drift marker, or drift reason reaches the screen. A string containing `_` or `.` in visible status or verb position is a defect.
- Titles lead and keys follow in mono, on every row and card.
- Status is never color alone: every status mark keeps its shape glyph or its text (JC is color blind).
- Supported viewports are laptop width and up. Check layouts at 1512 by 900 and 1024 wide, in light and dark, and in Safari as well as Chromium.
- A change under `ui/src` ships with a rebuilt `ui/dist` from `make ui` in the same commit.
- Doc and copy style: plain, no em dashes. Console copy says "decisions", never "forks".
- No file grows past 1000 lines. `History.tsx` is at 984: Task 21 moves code out before Tasks 22 to 25 add any.
- Build in the `console-ui-hunt` worktree. Commit with `git commit -s`. JC fast-forwards main; do not open a PR, stamp a version, or install a binary unless he asks.
- Batch gate, run before the last commit of every batch: `make lint`, `make test`, `cd ui && npm test`, `make ui`, then the `/verify` skill against a scratch project.

## Review Focus

1. **A tab with a hidden window.** Refetch on focus (Task 36) must never advance the History horizon and must not fire a second request while one is in flight. Test in Task 36.
2. **A node with no title.** Every title-first row falls back to the key, then the id, and never renders an empty link or a dangling separator. Test in Task 11.
3. **A status or verb this build has never seen.** `statusLabel` and `eventPhrase` must humanize it (no `_`, no `.`) instead of printing it raw or throwing. Tests in Tasks 2 and 3.
4. **A board whose every column is empty, or whose only cards sit in one column.** Collapsed rails must not leave a zero-width board or hide the column a filter just emptied. Test in Task 18.
5. **A stored URL or localStorage value from before this build** (`/drift`, `?mode=search` with no topbar field mounted yet, a navigator width, a History link with `window=visit`). Each must land on the new surface with its meaning intact. Tests in Tasks 28, 29, 17, and 25.

---

## Batch A: shared display layer and defects

### Task 1: Observed task pages load in under two seconds (D1)

Diagnosed 2026-11-09 with a goroutine dump and a logging `git` shim. `NodeHandler` (`internal/serve/node.go:64`) calls `observedEvents` (`internal/serve/execution.go:38`), which reaches `(*source).events` (`internal/execution/source.go:479-505`). That function reads every `.tiller/events/**/*.yml` in the task's source and only then filters on the task id. For a committed source (`local_branch`, `remote_tracking`) each read is its own `git cat-file blob` process: 6,987 spawns for `T-xv4mtr`, about 65 seconds, to keep a handful of events. One `git cat-file --batch` over the same blobs took 0.11 seconds. The trigger is a live observation on a committed branch, not in-review status as such. The request also keeps running after the client disconnects, because the read takes no context.

**Files:**
- Modify: `internal/execution/git.go` (new batch reader), `internal/execution/source.go:380,479-505`, `internal/execution/snapshot.go:455` (`Events` takes a context), `internal/serve/execution.go:35-50`, `internal/serve/node.go:35-143`
- Modify: `ui/src/api.ts` (`nodePagePath`), `ui/src/views/NodeDetail.tsx:49`, `ui/src/views/node/HistorySection.tsx`, `docs/spec/ui-surface.md:354`
- Test: `internal/execution/observation_test.go` (beside `TestObservationGitCallsStayBounded`, line 83), `internal/serve/execution_test.go` (beside `TestConsoleTaskHistoryReadsExecutionEventsAndDisclosesFailure`, line 183), `ui/src/components/__tests__/instrument.test.tsx`

**Interfaces:**
- Produces: `gitBatchBlobs(ctx context.Context, root string, oids []string) (map[string][]byte, error)` in `git.go`: one `git cat-file --batch` process, oids on stdin, with the same size bound `readAtMost` enforces per blob.
- Produces: `(*Snapshot).Events(ctx context.Context, taskID string)` and `(*source).events(ctx, taskID)`; the serve layer passes `r.Context()`.
- Produces: `GET /api/node/<noun>/<id>?history=integration` skips `observedEvents` and answers with integration history only plus `history_pending: true` when the node has an observation. Without the parameter the response is what it is today.

- [ ] **Step 1: Write the failing bound test** `TestCommittedSourceEventsGitCallsStayBounded`: build a committed source the way `TestCommittedLocalAndConfiguredRemoteTracking` does (`execution_test.go:172`: `fixture`, `worktree`, `executing`, `put`, commit, `git worktree remove`), once with 5 event files and once with 60. With the package's PATH shim counting git calls, assert `s.Events(ctx, firstID)` makes the same number of calls at both sizes and returns only that task's events.
- [ ] **Step 2: Write the failing cancellation test** `TestSourceEventsStopOnCancelledContext`: a cancelled context returns `context.Canceled` and makes no git call.
- [ ] **Step 3: Run** `go test ./internal/execution -run 'TestCommittedSourceEvents|TestSourceEventsStop'`. Expected: FAIL (call count grows with the file count; `Events` takes no context).
- [ ] **Step 4: Implement.** For a source whose kind is not `working_tree`, collect the oids already held in `s.entries`, read them in one batch, and keep the blob, mode, and parent-tree checks `readAtMost` makes. The working-tree path keeps its per-file read but hashes each file once.
- [ ] **Step 5: Run** `go test ./internal/execution ./internal/serve`. Expected: PASS.
- [ ] **Step 6: Write the failing split tests.** Go, `TestNodeIntegrationHistorySkipsSourceRead`: with the git shim, `?history=integration` on an observed task makes no `cat-file` call for event blobs and sets `history_pending`. UI: the node page requests `?history=integration`, renders the header and spec from it, and the History tab of a node with `history_pending` then requests the full path, shows `Reading branch history...` as its last row until that answers, and shows the merged rows after. A node without `history_pending` makes one request.
- [ ] **Step 7: Implement the split,** then run both suites. Expected: PASS.
- [ ] **Step 8: Measure on Acme.** Build to the scratch directory, serve the Acme checkout with no other load on the machine, and time `curl` on an observed task for each source kind present. A committed source must answer the full path in under 2 seconds. Record the working-tree number in the commit body; if it is over 2 seconds, memoise parsed events per `(repo.common, commit oid)` beside `commitTrees` in `internal/execution/treecache.go` and measure again.
- [ ] **Step 9: Commit** `Execution: read a source's events in one batch; node pages no longer wait on branch history`.

Not covered here: `newerIntegrationOwnership` (`snapshot.go:246`) calls the same `events` inside `execution.Load`. It did not fire in the diagnosis and it inherits the batch read.

### Task 2: One status label for every surface (D3)

**Files:**
- Create: `ui/src/lib/statusLabel.ts`, `ui/src/lib/statusLabel.test.ts`, `ui/src/lib/vocabulary.json`
- Create: `internal/views/vocabulary_internal_test.go`
- Modify: `ui/src/components/StateGlyph.tsx:75-85` (delete its `statusLabel`, import the lib one), `ui/src/components/Chip.tsx:58`, `ui/src/components/CriterionRow.tsx` (delete `statusWord`), `ui/src/views/explore/FilterMode.tsx:16-25`
- Test: `ui/src/components/__tests__/primitives.test.tsx`, `ui/src/components/__tests__/build.test.tsx`

**Interfaces:**
- Produces: `statusLabel(status: string): string`. Sentence case, underscores to spaces, with these overrides: `specced` to `Specified`, `needs_attestation` to `Needs attestation`, `in_review` to `In review`, `in_progress` to `In progress`.
- Produces: `vocabulary.json` with `statuses: string[]`, `event_types: string[]`, `drift_markers: string[]`, `drift_reasons: string[]`. Later tasks append to it and test against it.

- [ ] **Step 1: Write the failing tests.** In `statusLabel.test.ts`: every entry of `vocabulary.statuses` yields a label matching `/^[A-Z][a-z]+( [a-z]+)*$/`; `statusLabel("in_review") === "In review"`; `statusLabel("some_new_state") === "Some new state"` (Review Focus 3). In `primitives.test.tsx`: `<Chip status="in_review" />` shows `In review`. In `build.test.tsx`, block "expanded task rows: readable status": no element text equals `in_review`.
- [ ] **Step 2: Write the Go guard** `TestVocabularyCoversEveryStatus` in `vocabulary_internal_test.go`: read `../../ui/src/lib/vocabulary.json`; assert it contains every value in `statusOrder` (`facets.go:44-53`), every gate status and criterion status in `internal/store/gates.go:13-31`, and `open`, `answered`, `cancelled`.
- [ ] **Step 3: Run** `cd ui && npx vitest run src/lib/statusLabel.test.ts` and `go test ./internal/views -run TestVocabulary`. Expected: FAIL (module and file missing).
- [ ] **Step 4: Implement.** Fill `vocabulary.json` statuses from the Go sources named in Step 2. `Chip` renders `children ?? statusLabel(status)`. Filter mode's noun options read `Epics`, `Features`, `Tasks`, `Questions`, `Amendments`, `Bugs`, `Chores`, `Tweaks`.
- [ ] **Step 5: Run** `cd ui && npm test` and fix every assertion that pinned a raw status word in a chip. Expected: PASS.
- [ ] **Step 6: Commit** `Console: one status label for every surface`.

### Task 3: One phrase for every event type (D3)

**Files:**
- Create: `ui/src/lib/eventPhrase.ts`, `ui/src/lib/eventPhrase.test.ts`
- Create: `internal/core/lifecycle/vocabulary_internal_test.go`
- Modify: `ui/src/lib/bursts.ts:85-195` (move `PARTICIPLE`, `TYPE_PARTICIPLE`, `TYPE_PHRASE`, `burstPhrase`, `pluralize` out), `ui/src/views/History.tsx:170`, `ui/src/views/home/Pulse.tsx:22`, `ui/src/views/node/HistorySection.tsx:43`, `ui/src/lib/vocabulary.json`

**Interfaces:**
- Produces: `eventPhrase(type: string, count: number): { subject: string; verb: string }`, replacing `burstPhrase` with the same return shape.

- [ ] **Step 1: Write the failing tests** in `eventPhrase.test.ts`: for every entry of `vocabulary.event_types` and for counts 1 and 3, `subject` and `verb` match `/^[a-z][a-z ]*$/`. Pinned cases: `rename.title.task` gives `{subject: "task retitled", verb: "retitled"}` and `"tasks retitled"` at 3; `rename.key.feature` gives `rekeyed`; `link.attach.bug` gives `attached`; `unlink.attach.bug` gives `detached`; `remove.proposed.task` gives `removed`; `order.features` gives subject `features reordered` at any count; `amendment_opened` gives `amendment opened`; `correct-eval.feature` gives verb `eval corrected`; `recover_review.epic` gives verb `review recovered`; `migrate.schema` gives `schema migrated`; an unknown `frobnicate.widget` gives `{subject: "widget frobnicate", verb: "frobnicate"}` and an unknown `odd_thing` gives `odd thing` in both slots (Review Focus 3).
- [ ] **Step 2: Write the Go guard** `TestVocabularyCoversEveryTransition`: for each noun and row in `transitions` (`table.go:63`), assert `row verb + "." + noun` is in `vocabulary.json` `event_types`.
- [ ] **Step 3: Run both.** Expected: FAIL.
- [ ] **Step 4: Implement.** Resolution order: full-type override table, then `verb.qualifier.noun` (qualifier table: `rename.title` retitled, `rename.key` rekeyed, `link.attach` attached, `unlink.attach` detached, `remove.proposed` and `remove.draft` removed, existing surface rows), then `verb.noun`, then the humanizing fallback. Add participles: claim claimed, release released, reassign reassigned, resolve resolved, grade graded, order reordered, rename renamed, remove removed, configure configured, transfer transferred, move moved, review reviewed, migrate migrated, revise revised, superseded superseded. A noun that is already plural (`features`, `tasks`, `bugs`, `chores`, `tweaks`) is not pluralized again. Fill `event_types` with every type listed in the hunt research: the transition types from the lifecycle table, plus the `add`, `answer`, `cancel.question`, `claim`, `release`, `reassign`, `resolve`, `link`, `unlink`, surface, attach, `rename`, `remove`, `order`, `configure`, roster, amendment, design, `migrate.schema`, `grade.bug`, `review.assumption`, `transfer`, `submit`, `recover_review`, `sync`, and legacy `accept` and `owner` families found by `grep -rn 'Type:' internal/core/mutate`.
- [ ] **Step 5: Use it on the node History tab.** `HistorySection` renders `eventPhrase(e.type, 1).verb` in `.history-type`. The outcome column is Task 26.
- [ ] **Step 6: Run** `cd ui && npm test && go test ./internal/core/lifecycle`. Expected: PASS.
- [ ] **Step 7: Commit** `Console: every event type has a phrase`.

### Task 4: One timestamp formatter (smaller win: timestamps)

**Files:**
- Create: `ui/src/lib/time.ts`, `ui/src/lib/time.test.ts`, `ui/src/components/Time.tsx`
- Modify: `ui/src/views/History.tsx:973-984`, `ui/src/views/home/justHappened.ts:69-76`, `ui/src/components/ExecutionSources.tsx:38`, `ui/src/views/node/HistorySection.tsx:42`, `ui/src/components/IntegrityNotes.tsx:26`, `ui/src/components/RecoveredReview.tsx:25`, `ui/src/components/relationships/SourceCitation.tsx:49`, `ui/src/views/FileDetail.tsx:109,124`, `ui/src/views/node/AmendmentRelationsPanel.tsx:67-69`

**Interfaces:**
- Produces: `formatWhen(iso: string, now: Date): string`: `just now` under a minute, `12m ago` under an hour, `3h ago` under 24 hours, `Nov 9, 2:06 PM` in the current year, `Nov 9, 2025, 2:06 PM` otherwise, and the input unchanged when it does not parse.
- Produces: `formatClock(iso: string): string` giving `2:06 PM`, and `formatAbsolute(iso: string): string` giving `Nov 9, 2026, 2:06:28 PM`.
- Produces: `<Time iso={string} mode?: "when" | "clock" />` rendering `<time dateTime={iso} title={formatAbsolute(iso)}>`.

- [ ] **Step 1: Write the failing tests** in `time.test.ts` with a fixed `now` of `2026-11-09T20:06:00Z`: one assertion per branch above, plus `formatWhen("not a date", now) === "not a date"`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement,** then replace every site in the Files list with `<Time>`. Leave `views/report/format.ts` (week labels and the As-of line are chart copy) and `lib/added.ts` alone.
- [ ] **Step 4: Run** `cd ui && npm test`; update assertions that pinned `2:10p` or a raw RFC3339 string. Expected: PASS.
- [ ] **Step 5: Commit** `Console: one timestamp formatter`.

### Task 5: The navigator reveals the current node (D2)

**Files:**
- Modify: `ui/src/components/Navigator.tsx:133` (tree ref and effect), `:357,426,436,508,521,547,550` (`aria-current="page"` on the selected row's link)
- Test: `ui/src/components/__tests__/navigator-reveal.test.tsx` (new)

- [ ] **Step 1: Write the failing tests.** Stub `Element.prototype.scrollIntoView = vi.fn()`. (a) Render Explore at `/node/feature/F-2` with the fetch stubs from `explore.test.tsx:70-126`: `scrollIntoView` is called once, on the element holding the `aria-current="page"` link, with `{ block: "nearest" }`. (b) After that, clicking a chevron on another epic does not call it again. (c) Navigating to `/node/task/T-9` calls it once more. (d) A bug page reveals its shelf row.
- [ ] **Step 2: Run.** Expected: FAIL (never called).
- [ ] **Step 3: Implement.** Keep a `pendingReveal` ref holding the selection key (`noun:addr`). Set it when `selected` changes. A layout effect with no dependency list looks for `[aria-current="page"]` inside the tree ref and, when found while a reveal is pending, scrolls it and clears the ref. The pending ref is what lets the reveal wait for Explore's `autoOpen` to open the ancestors on a later render.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Navigator: scroll the current node into view on arrival`.

### Task 6: Attention card says why, or stays away; counts name their scope (D4, D5)

**Files:**
- Modify: `ui/src/lib/attentionFallback.ts:68-121`, `ui/src/lib/attentionFallback.test.ts`, `ui/src/components/NodeDetail.tsx:312-319`
- Modify: `ui/src/views/workitems/board.ts`, `ui/src/views/WorkItems.tsx:106-110,139`, `ui/src/views/home/HeaderBand.tsx:94-96,104-127`, `ui/src/views/report/KpiStrip.tsx`, `docs/spec/ui-surface.md:92` (the funnel caption and Fix tile wording)
- Test: `ui/src/views/workitems/board.test.ts`, `ui/src/components/__tests__/workitems.test.tsx:205-209`, `ui/src/components/__tests__/home.test.tsx` (header band, shelf tile), `ui/src/components/__tests__/report-page.test.tsx`

**Interfaces:**
- Produces: `needsAttentionFallback(...)` returns `{ label: string; why: string; command: string } | null`.
- Produces: `boardScope(columns): { open: Record<"bug" | "chore" | "tweak", number>; closedShown: number }` in `board.ts`.

- [ ] **Step 1: Write the failing tests.** Fallback: the last fallthrough returns `null`, so no card names `tiller show`; attestation gives why `Criterion C2 is waiting on your sign-off.`; answer gives `This <noun> is waiting on your answer to "<question title>".`; approval gives `This <noun> is waiting on your approval.` Fix toolbar reads `25 open: 1 bug, 21 chores, 3 tweaks. 5 recently closed shown.` and drops the second sentence at zero closed. Home's Fix tile caption starts `open: `. Home's funnel caption reads `85 features in flow, 67 retired · 443 tasks`. Report's first tile carries the sub-line `of approved scope`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** `FallbackBox` renders the why sentence above the command.
- [ ] **Step 4: Check the events count on Acme.** `curl -s localhost:<port>/api/recap/peek` from a server on the Acme checkout and count events after its `since` with `tiller` or `ls .tiller/events`. If `total` matches, record "true count, no change" in the commit body. If it does not, stop and report: that is a server defect this plan does not cover.
- [ ] **Step 5: Run** `cd ui && npm test`. Expected: PASS.
- [ ] **Step 6: Commit** `Console: attention card says why; counts name their scope`.

### Task 7: One displayed status on every surface (D6)

The feature half of this exists as commit `4926090a` in `~/dev/tiller-history` (branch `explore-board-status`, 38 files). Port it, then extend the same rule to epics and to the list filter.

**Files:**
- Port: `internal/gen/activity.go`, `internal/gen/tree.go`, `internal/relationships/types.go`, `internal/search/search.go`, `internal/serve/search.go`, `internal/views/map.go`, `internal/views/node.go`, `internal/views/nodepage.go`, `internal/views/relationship_graph.go`, `internal/views/summary.go`, their tests, `ui/src/api.ts`, `ui/src/lib/displayStatus.ts` (+ test), `ui/src/lib/activityPhrase.ts`, `Navigator.tsx`, `NodeHeader.tsx`, `FilterMode.tsx`, `SearchResults.tsx`, `map/MapCanvas.tsx`, `map/MapCard.tsx`, `map/RelationshipDetails.tsx`, `views/home/stat.ts`, `views/node/EpicBody.tsx`, `views/node/RelatedList.tsx`, `docs/spec/ui-surface.md`
- Modify beyond the port: `internal/gen/activity.go` (epic rule), `internal/views/build.go:152` (`BuildEpic.Column`), `internal/serve/list.go:91-93` (epic column, status filter), `internal/views/facets.go:120` (status counts)

**Interfaces:**
- Produces: `gen.BoardStatus(t *store.Tree, n store.Node) string`: for a feature, `FeatureBoardStatus`; for an epic, `"active"` when stored `ready` and `EpicActivity` is `"active"`; otherwise the stored status.
- Produces: `column?: string` on `TreeNode`, `NodeView`, `RelatedNode`, `MapNode`, `SearchNodeResult`, `ListRow`, `BuildEpic`, for epics and features.
- Produces: `displayStatus(node: { status: string; column?: string }): string` in `ui/src/lib/displayStatus.ts`. Tasks 30 and 33 rely on it.

- [ ] **Step 1: Apply the old commit without its bundle.** `git --git-dir=/Users/jcgrubbs/dev/tiller-history/.git show 4926090a -- . ':!ui/dist' | git apply --3way` and, where three-way has no base, `git apply --reject` and resolve each `.rej` by hand against the commit's intent (its message is the spec).
- [ ] **Step 2: Run** `go test ./internal/... && cd ui && npm test`. Expected: PASS, the ported tests included.
- [ ] **Step 3: Write the failing epic tests.** `internal/gen/activity_test.go`: a `ready` epic with one task `in_progress` gives `BoardStatus == "active"`; a `ready` epic with no task in flight gives `"ready"`; `done` stays `done`. `internal/views/nodepage_test.go`: that epic's page has `node.column == "active"`, and a task under it has `relationships.epic.column == "active"`. `internal/serve/list_test.go`: `/api/ls?noun=feature&status=active` returns a stored-`ready` feature with a task in flight and `status=ready` does not; the status facet counts agree. `internal/serve/serve_test.go`: the CLI-facing JSON (`views.statusRelationships`) still has no `column`.
- [ ] **Step 4: Run.** Expected: FAIL.
- [ ] **Step 5: Implement.** The list filter and the facet counts match on display status for epics and features on the serve path only; `tiller ls --status` keeps matching the stored status.
- [ ] **Step 6: UI test** in `nodeheader.test.tsx`: an epic page with `column: "active"` shows the hero chip `Active`, and a task's breadcrumb chip for that epic shows `Active`.
- [ ] **Step 7: Run everything, then the batch gate.** Expected: PASS.
- [ ] **Step 8: Commit** `Console: one displayed status for epics and features on every surface`.

---

## Batch B: decision cards and title-first rows (I1, I2)

### Task 8: The queue carries what is being decided

**Files:**
- Modify: `internal/views/queue.go:88-101` (entry), `:136` (thread `in.Workflow.Contracts`), `:218-262` (`queueApproveFeatures`), `:350-440` (attest builders)
- Modify: `ui/src/api.ts:1306`
- Test: `internal/views/queue_test.go`

**Interfaces:**
- Produces on `QueueEntry`: `Title string` (`title`, the node's title on every kind whose node has one), `Statement string` (`statement`, the criterion's statement, `attest_criterion` only), `Criterion string` (`criterion`, the criterion id, `attest_criterion` only), `Plan *QueuePlan` (`plan`, `approve_feature` only).
- Produces: `type QueuePlan struct { Tasks, Criteria, Dependencies int }` with JSON `tasks`, `criteria`, `dependencies`.
- `Prose` is unchanged, so existing consumers keep working.

- [ ] **Step 1: Write the failing tests** `TestQueueAttestCriterionCarriesStatement` (entry has the feature's title, `criterion: "C2"`, and the statement text from the fixture's eval file) and `TestQueueApproveFeatureCarriesPlanCounts` (a feature with 7 tasks, 15 criteria, and 10 `depends_on` gives `plan == {7, 15, 10}`; a feature with no contract gives `criteria: 0`).
- [ ] **Step 2: Run** `go test ./internal/views -run TestQueue`. Expected: FAIL.
- [ ] **Step 3: Implement** by passing `in.Workflow.Contracts` to the two builders. Counts come from `t.FeatureTasks[f.ID]`, the contract's `Criteria`, and `f.DependsOn`.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Queue: carry the criterion statement and the plan counts`.

### Task 9: Attest commands make the reader choose (R1)

**Files:**
- Modify: `internal/views/queue.go:385,438`, `ui/src/lib/attentionFallback.ts:72-84`, `docs/spec/ui-surface.md` ("Acting at the gates"), `docs/spec/cli-surface.md` wherever the readout quotes the attest command
- Test: `internal/views/queue_test.go`, `ui/src/lib/attentionFallback.test.ts`, and every Go test that pins `--status pass` in a queue or readout expectation (`grep -rn -- '--status pass' internal --include=*_test.go`)

- [ ] **Step 1: Change the expectations** to `--status <pass|fail>` in the tests the grep finds that assert a queue, next, or readout command. Leave tests that run a real attest alone.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement** the two format strings and the client fallback string.
- [ ] **Step 4: Run** `make test && cd ui && npm test`. Expected: PASS.
- [ ] **Step 5: Commit** `Attest: the offered command asks for pass or fail`.

### Task 10: Decision cards show the decision

**Files:**
- Modify: `ui/src/components/DecisionCard.tsx:39-181`, `ui/src/components/DecisionBand.tsx:44-67`, `ui/src/views/Home.tsx:140-149`, `ui/src/views/home.css:256-265` and the `.dcard` rules, `ui/src/components/CriterionRow.tsx`
- Test: `ui/src/components/__tests__/home.test.tsx` ("DecisionCard" at 765, bands at 439), `ui/src/views/home.css.test.ts`

**Interfaces:**
- Consumes: Task 8's `title`, `statement`, `criterion`, `plan`.
- Produces: `DecisionCard({ entry, tabbed }: { entry: QueueEntry; tabbed?: boolean })`.
- Produces: a criterion deep link `/node/<noun>/<id>?tab=evals&criterion=<id>`. `CriterionRow` adds `id="criterion-<id>"`, and when the `criterion` search param names it, adds the class `is-target` and scrolls itself into view once on mount.

- [ ] **Step 1: Write the failing tests.** (a) An `attest_criterion` card leads with the node title, then the mono key, then `Criterion C2`; it shows the statement in a `.dcard-statement` element that links to the criterion deep link; the text `needs_attestation` appears nowhere. (b) An `approve_feature` card shows `7 tasks, 15 criteria, 10 dependencies` (singular forms at 1, a zero part omitted). (c) With `tabbed`, the card has no `.kind` eyebrow unless the actor is `agent`, in which case it reads `Agent`. (d) A `BandRow` carries a copy control for its own `entry.command`. (e) A Needs You tab whose kind has a `batchCommand` and two or more entries shows one `Copy all` control for that command, in card form as well as band form. (f) CSS: `.dcard` padding is `12px 14px` and `.dcard-statement` clamps at three lines.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** `WhoLine` uses `entry.title` for every kind and drops its attest special case. `whyLine` for `attest_criterion` returns `""` when a statement is present and `This criterion needs your attestation.` otherwise. Home passes `tabbed` when a Needs You tab rail is showing.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Home: decision cards show what is being decided`.

### Task 11: Titles on the wire and in every row

**Files:**
- Modify: `internal/views/streams.go:13-80` (`StreamItem`, `GatesItem`, `BlockedItem`, `UnclaimedItem`), `internal/serve/execution.go` (title on `/api/execution` rows), `ui/src/api.ts`
- Create: `ui/src/components/TitleRef.tsx`, `ui/src/views/home/inFlight.ts`, `ui/src/views/home/inFlight.test.ts`
- Modify: `ui/src/views/home/Pulse.tsx:175-231,318-342`, `ui/src/views/home/nextRow.ts:10-15`, `ui/src/components/ExecutionSources.tsx:73-87`, `ui/src/views/Home.tsx:347-375`, `ui/src/views/WorkItems.tsx:345-349`, `ui/src/views/node/BacklinksPanel.tsx:53-86`
- Test: `internal/views/streams_test.go`, `internal/serve/execution_test.go`, `ui/src/components/__tests__/home.test.tsx` (In flight 1586, Up next 1948), `execution.test.tsx`, `workitems.test.tsx`, `ui/src/views/node/BacklinksPanel.test.tsx`

**Interfaces:**
- Produces on the three stream items: `title`, and for tasks `feature_id` and `feature_title`. On `UnclaimedItem`: `feature_title`. On each `/api/execution` observation: `title`, added by a serve-layer wrapper so `internal/execution` stays free of the tree.
- Produces: `TitleRef({ node, title, addr }: { node: RefTarget; title?: string; addr: string })`: a `RefLink` labelled `title ?? addr`, followed by `<span className="mono title-ref-key">{addr}</span>` only when a title exists.
- Produces: `groupByFeature<T extends { feature_id?: string; feature_title?: string }>(items: T[]): { featureId: string | null; featureTitle: string | null; items: T[] }[]`, preserving input order and grouping only adjacent-or-not items of the same feature into the first group seen.

- [ ] **Step 1: Write the failing Go tests:** a stream task item carries its title and its feature's id and title; an observation row carries the task title.
- [ ] **Step 2: Write the failing UI tests.** In flight: seven tasks of one feature render one `.pulse-feature` heading with the feature title and seven rows that lead with task titles; the path `reviewed-audits/selection-and-queues/` appears at most once. A stream item with no title renders its key once, with no empty link and no dangling key span (Review Focus 2). Up next: a row leads with the node title and carries a `CopyCmd` for `candidate.command`. Branches rows, Recovered reviews headings, and Fix's "Attached to" line lead with the title, with the key in mono after it. Backlinks groups lead with the owner's title and put the file path second; a `provenance: unsupported recorded reference` diagnostic renders inside a closed `<details>` titled `Integrity notes` with the line `A recorded reference uses a form this version cannot resolve.`
- [ ] **Step 3: Run.** Expected: FAIL.
- [ ] **Step 4: Implement.** Delete the comment at `Pulse.tsx:315-317` along with the rule it states (R11).
- [ ] **Step 5: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 6: Commit** `Console: titles lead, keys follow`.

---

## Batch C: the node page and the navigator (I3, I4)

### Task 12: A ready task shows how to start it

**Files:**
- Modify: `internal/views/next_enumerate.go:297-312` (extract the builder), `internal/views/nodepage.go:41-102,293`, `ui/src/api.ts:811`, `ui/src/components/NodeDetail.tsx:321-377`
- Test: `internal/views/nodepage_test.go`, `ui/src/components/__tests__/instrument.test.tsx`

**Interfaces:**
- Produces: `claimMove(task *store.Task) (command, reason string)` in `next_enumerate.go`, used by `claimCandidates` and by the node page, so both surfaces print the same command.
- Produces: `NodePage.Start *NodeStart` (`start`, omitted when nil) with `Actor`, `Action`, `Command`, `Then` (JSON `actor`, `action`, `command`, `then`). Set for a `ready`, unclaimed, eligible task: action `claim and start`, command `tiller claim task <addr>`, then `tiller start task <addr>`. Set for a `ready` claimed task with only the start command.

- [ ] **Step 1: Write the failing tests.** Go: the page for a ready unclaimed task has `start.command == "tiller claim task <addr>"` and `start.then == "tiller start task <addr>"`; an in-progress task has none; a task under a held feature has none. UI: the attention stack renders a box labelled `Next · Agent` with both commands as copy controls, and renders it only when there is no workflow box and no ask.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Node page: a ready task shows its claim and start commands`.

### Task 13: Pinned tabs and a one-line header (R10)

**Files:**
- Create: `ui/src/lib/useScrolledPast.ts`
- Modify: `ui/src/components/NodeTabs.tsx:33-78`, `ui/src/components/NodeDetail.tsx:673-751`, `ui/src/components/instrument.css`, `ui/vite.config.ts:18-50` (add `instrument.css` and `explore.css` to `css.include`), `docs/spec/ui-surface.md:171`
- Test: `ui/src/components/__tests__/nodetabs.test.tsx`, `ui/src/components/instrument.css.test.ts` (new)

**Interfaces:**
- Produces: `useScrolledPast(ref: RefObject<Element | null>): boolean`, true once the element has left its scroll container's viewport. Returns false where `IntersectionObserver` is undefined.
- Produces: `NodeTabs` prop `pinned?: ReactNode`, rendered inside a `.node-pin` wrapper above the tab buttons.

- [ ] **Step 1: Write the failing tests.** With `IntersectionObserver` stubbed to report the header as not intersecting, the page renders `.node-pin .node-compact` holding the noun pill, the status chip, and the title on one line; when intersecting, `.node-compact` is absent. CSS: `.node-pin { position: sticky; top: -22px; z-index: 2; background: var(--surface) }` (the offset cancels `.explore-detail`'s 22px top padding; use the pane's actual background token), and `.node-compact` is a single non-wrapping flex row with `text-overflow: ellipsis` on the title.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Flat nouns (question, amendment) have no tabs and get no pin.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Node page: tabs and a one-line header stay in reach`.

### Task 14: One decision at a time, and an outline beside the spec

**Files:**
- Modify: `ui/src/components/NodeDetail.tsx:321-377`, `ui/src/components/Prose.tsx`, `ui/src/views/node/ProseSection.tsx`, `ui/src/styles/prose.css`
- Create: `ui/src/lib/outline.ts`, `ui/src/lib/outline.test.ts`, `ui/src/components/ProseOutline.tsx`
- Test: `ui/src/components/__tests__/instrument.test.tsx`, `ui/src/views/node/ProseSection.test.tsx`

**Interfaces:**
- Produces: `outlineOf(markdown: string): { id: string; text: string; depth: 2 | 3 }[]`. Ids are lowercase, hyphenated, and suffixed `-2`, `-3` on repeats. Headings inside fenced code are ignored.
- Produces: `Prose` gives each `h2` and `h3` the matching `id`.
- Produces: `ProseOutline({ items })`: `<nav aria-label="On this page">` with one in-page link per item.

- [ ] **Step 1: Write the failing tests.** Attention stack: with a workflow box and two asks, only the first box renders and a button reads `2 more`; pressing it shows all three. With exactly one box there is no button. Outline: the pinned cases above. `ProseSection`: a spec with three or more outline items renders the nav; one with fewer does not.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Layout: `.prose-with-outline` is a two-column grid, `minmax(0, 72ch) 220px`, that collapses to one column under a 1100px container width, hiding the outline. The outline is `position: sticky` below the pinned tabs.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Node page: fold extra decisions; outline the spec`.

### Task 15: Evidence links read as labels

**Files:**
- Create: `ui/src/lib/evidenceLabel.ts`, `ui/src/lib/evidenceLabel.test.ts`
- Modify: `ui/src/components/GateCard.tsx:67-88`, `ui/src/components/CriterionRow.tsx:94-107`, `ui/src/components/IntegrityNotes.tsx:27,38,41`

**Interfaces:**
- Produces: `evidenceLabel(input: { ref: string; kind?: string; criterion?: string }): string`: `eval C1 output` when a criterion id is known, `<kind> evidence` when only a kind is, `evidence` otherwise.

- [ ] **Step 1: Write the failing tests:** the three branches; and in `instrument.test.tsx`, a criterion row's evidence link text is `eval C1 output`, its `title` is the full ref, and its `href` is unchanged.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Node page: evidence links read as labels`.

### Task 16: The workflow note is a sentence (R13)

**Files:**
- Modify: `internal/workflow/workflow.go:203-215,308,327`, `internal/workflow/recovered_closeout.go:33`, `docs/spec/cli-surface.md` where the readout quotes the old reason
- Test: `internal/workflow/workflow_test.go`, plus every test pinning the old text (`grep -rn 'remains recorded' internal`)

- [ ] **Step 1: Change the expectations.** A done node with two human attestations outstanding and one recorded violation reads: `Done is recorded. Acceptance is still open: C2 and C5 need your attestation. 1 historical policy violation remains on record.` Plural agreement holds at 1 and at 2. An item an agent owes reads `<id> needs the agent's <work kind>`. The review-phase variant uses the same item sentence.
- [ ] **Step 2: Run** `go test ./internal/workflow`. Expected: FAIL.
- [ ] **Step 3: Implement** in `workflowItems` and the three call sites. `Advice.Outstanding` is unchanged, so JSON consumers are unaffected.
- [ ] **Step 4: Run** `make test`. Expected: PASS after the pinned strings are updated.
- [ ] **Step 5: Commit** `Workflow: the closeout reason reads as a sentence`.

### Task 17: A readable navigator (I4)

**Files:**
- Modify: `ui/src/components/Navigator.tsx:360,436,521,550`, `ui/src/views/explore.css:583-591` (new `.explore-task-title`), `ui/src/views/explore/splitSize.ts:20-24`, `ui/src/views/explore/splitStorage.ts`, `ui/src/views/Explore.tsx:449`
- Test: `ui/src/components/__tests__/navigator-status.test.tsx`, `ui/src/views/explore/splitSize.test.ts`, `ui/src/views/explore.css.test.ts` (new)

**Interfaces:**
- Produces: `defaultNavWidth(viewportWidth: number): number`: 300 under 1600, 360 at 1600 and above. A stored width always wins (Review Focus 5).

- [ ] **Step 1: Write the failing tests.** Each of the four rows' `title` attribute is `<title> · <key>`, or just the key when there is no title. `defaultNavWidth(1512) === 300`, `defaultNavWidth(1920) === 360`, and a stored 280 is returned unchanged at 1920. CSS: `.explore-task-title` has `-webkit-line-clamp: 2` and no `white-space: nowrap`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Only task rows wrap; epic and feature rows keep one line.
- [ ] **Step 4: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 5: Commit** `Navigator: full titles on hover, two-line task titles, a wider default on wide windows`.

---

## Batch D: the boards (I6)

### Task 18: Empty columns collapse to a rail (R5)

**Files:**
- Modify: `ui/src/components/BoardGrid.tsx:36-38`, `ui/src/components/board.css:65-108`, `ui/src/views/Build.tsx:255-274,385-415`, `ui/src/views/WorkItems.tsx:322-323`
- Test: `ui/src/components/__tests__/build.test.tsx`, `workitems.test.tsx`, `ui/src/components/board.css.test.ts`

**Interfaces:**
- Produces: `BoardLane { key; head; body; empty: boolean; label: string }`. `BoardGrid` sets `grid-template-columns` inline: `44px` for an empty lane, `minmax(var(--board-col-min), 1fr)` otherwise, and sets `--board-min-width` to the matching sum so the scroll container's minimum width follows.

- [ ] **Step 1: Write the failing tests.** A Build board with cards only in Ready and In review renders four lanes with `data-collapsed="true"`, each showing its label and `0`, and the inline template reads `44px 44px minmax(...) 44px minmax(...) 44px`. A board with no cards at all keeps every lane at full width and shows the existing empty text (Review Focus 4). A Fix board with a filter that empties In progress collapses it.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** A collapsed head writes its label vertically (`writing-mode: vertical-rl`) with the count beneath.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Boards: empty columns collapse to a rail`.

### Task 19: Build cards show progress and readable tasks

**Files:**
- Modify: `ui/src/views/build/mix.ts`, `ui/src/views/build/columns.ts`, `ui/src/views/Build.tsx:429-581`, `ui/src/views/build.css:286-295,415-438`
- Test: `ui/src/views/build/mix.test.ts`, `columns.test.ts`, `ui/src/components/__tests__/build.test.tsx`

**Interfaces:**
- Produces: `taskMix(tasks: readonly { status: string }[]): StateMixEntry[]` and `taskMixCaption(tasks): string` (for seven tasks in review: `0 of 7 done, 7 in review`).
- Produces: `showEpicEyebrow(cards: readonly BuildFeature[], index: number, selectedEpicCount: number): boolean`: false when one epic is selected, false when the card above in the same column has the same epic, true otherwise.

- [ ] **Step 1: Write the failing tests.** The pure cases above. Rendered: a card with tasks shows a `StateBar` whose caption is the mix caption, and no bare `0 / 7`; a card with no contributors has no `.contributors-chip`; an epic row in the rail with none has no `Contributors:` line; an expanded task row puts a `StateGlyph` before the title and has no trailing chip; three stacked cards of one epic show the eyebrow once.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** `.trow` becomes glyph then title, title at full width.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Build: cards show task progress and readable task rows`.

### Task 20: Name the scroll toggle; fold finished epics

**Files:**
- Modify: `ui/src/components/BoardToolbar.tsx:31-39`, `ui/src/views/Build.tsx:283-367`, `ui/src/views/build/selection.ts`
- Create: `ui/src/lib/boardScroll.test.ts`
- Test: `ui/src/components/__tests__/build.test.tsx`, `ui/src/views/build/selection.test.ts`

**Interfaces:**
- Produces: `splitEpics(epics: readonly BuildEpic[]): { live: BuildEpic[]; finished: BuildEpic[] }`, finished meaning status `done` or `archived`.

- [ ] **Step 1: Write the failing tests.** The toggle's group label is `Scrolling`, its buttons read `Whole board` and `Each column`, and each has a `title`: `The board scrolls as one page` and `Each column scrolls on its own`. `boardScroll` round-trips both values through `tiller-board-scroll` and reads a stored `board` or `columns` from before this build. The rail lists live epics, then a closed disclosure `9 finished`; the disclosure starts open when a finished epic is in the selection.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Stored values stay `board` and `columns`; only the labels change.
- [ ] **Step 4: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 5: Commit** `Boards: name the scroll toggle; fold finished epics`.

---

## Batch E: History (I5)

### Task 21: Move History's rows out of the view

`History.tsx` is 984 lines. Move before adding.

**Files:**
- Create: `ui/src/views/history/BurstRow.tsx` (from `History.tsx:714-889`), `ui/src/views/history/RecapGroupBlock.tsx` (from `:424-664`), `ui/src/views/history/copy.ts` (from `:940-984`)
- Modify: `ui/src/views/History.tsx`

- [ ] **Step 1: Move the code** with no behavior change.
- [ ] **Step 2: Run** `cd ui && npx vitest run src/components/__tests__/history.test.tsx`. Expected: PASS, untouched.
- [ ] **Step 3: Commit** `History: split the view into row, group, and copy modules`.

### Task 22: Milestones by default (R3)

**Files:**
- Create: `ui/src/lib/eventTier.ts`, `ui/src/lib/eventTier.test.ts`
- Modify: `ui/src/lib/bursts.ts`, `ui/src/lib/bursts.test.ts`, `ui/src/views/History.tsx`, `ui/src/views/history/BurstRow.tsx`, `ui/src/views/home/justHappened.ts:34-39`
- Test: `ui/src/components/__tests__/history.test.tsx`, `ui/src/views/home/justHappened.test.ts`

**Interfaces:**
- Produces: `eventTier(type: string, ask: boolean): "milestone" | "bookkeeping"`. An ask is always a milestone. Bookkeeping verbs: `link`, `unlink`, `order`, `rename`, `remove`, `configure`, `claim`, `release`, `reassign`, `transfer`, `move`, `grade`, `migrate`, `contributor`, `owner`, and `add` for every noun except `question`. Everything else, an unknown type included, is a milestone.
- Produces: `tierBursts(bursts: Burst[], showAll: boolean): (Burst | BookkeepingRun)[]` where `BookkeepingRun { kind: "bookkeeping"; bursts: Burst[]; count: number; verbs: string[] }` folds each maximal run of adjacent bookkeeping bursts.

- [ ] **Step 1: Write the failing tests.** Every entry in `vocabulary.event_types` classifies without hitting the unknown branch (assert against an exported `KNOWN_BOOKKEEPING_VERBS` and `KNOWN_MILESTONE_VERBS` whose union covers every verb in the vocabulary). `tierBursts`: three adjacent bookkeeping bursts become one run with `verbs: ["linked", "reordered", "retitled"]`; a milestone between two bookkeeping bursts gives two runs; `showAll` returns the input unchanged. Rendered: History shows a `Milestones` and `Everything` segmented control riding `?show=all`, defaulting to milestones; a run renders one row `14 bookkeeping changes (linked, reordered, retitled)` that opens to its bursts. Just happened drops bookkeeping.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `History: milestones by default, bookkeeping folded`.

### Task 23: Filter History by actor, kind, and epic

**Files:**
- Create: `ui/src/views/history/filter.ts`, `ui/src/views/history/filter.test.ts`
- Modify: `ui/src/views/History.tsx:224-255,344-360`, `ui/src/views/history/copy.ts`
- Test: `ui/src/components/__tests__/history.test.tsx`

**Interfaces:**
- Produces: `historyFacets(recap: RecapModel): { actors: string[]; kinds: string[]; epics: { id: string; title: string }[] }`. A kind is the phrase verb of an event type present (`specced`, `approved`), plus the special value `asked` for entries whose `ask` flag is set.
- Produces: `filterRecap(recap: RecapModel, filter: { actor?: string; kind?: string; epic?: string }): RecapModel`, recomputing each group's entries and dropping empty groups. `total` is left as the unfiltered count; a new `shown` holds the filtered count.

- [ ] **Step 1: Write the failing tests.** The pure cases. Rendered: three `select.control` elements labelled Actor, Kind, Epic ride `?actor=`, `?kind=`, `?epic=`; with a filter on, the head reads `120 of 1000 events`; an unknown value in the URL is ignored. `asked you` becomes a link that sets `?kind=asked`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `History: filter by actor, kind, and epic`.

### Task 24: Day headings, time-only rows, verb first

**Files:**
- Modify: `ui/src/views/history/BurstRow.tsx`, `ui/src/views/history/RecapGroupBlock.tsx`, `ui/src/views/history.css`, `ui/src/views/home/Pulse.tsx:85-109`
- Test: `ui/src/components/__tests__/history.test.tsx` (row anatomy at 466), `ui/src/views/history.css.test.ts`

**Interfaces:**
- Consumes: `formatClock`, `<Time>` from Task 4.

- [ ] **Step 1: Write the failing tests.** A group with one day still renders its day heading (`Monday · Nov 9`). A lone row's children are, in order, `.t` (clock only, `2:06 PM`), `.verb`, `.what` (the title), `.who`. A run's row reads `.t`, then `4 features specced` spanning the verb and title columns. No row contains `Nov 9`. CSS: `.hrow .verb` has a fixed `width: 9ch` and bold weight. Home's Just happened rows use the same order.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `History: day headings, time-only rows, the verb first`.

### Task 25: Catching up is a button (R2)

**Files:**
- Modify: `ui/src/views/History.tsx:219-221`, `ui/src/views/history/copy.ts`, `ui/src/api.ts:1680`, `docs/spec/ui-surface.md:161-165`
- Test: `ui/src/components/__tests__/history.test.tsx`, `internal/serve/visit_marker_test.go` (unchanged, must still pass)

- [ ] **Step 1: Write the failing tests.** Opening History on the visit window sends no `X-Tiller-Visit` header. The head reads `1000 events since Nov 9, 2:06 PM` and shows a `Mark caught up` button. Pressing it sends exactly one GET to `/api/recap` carrying `X-Tiller-Visit: 1`, after which the button is replaced by `Caught up. Your next visit starts here.` and the list stays as it was. The fixed windows show no button. A stored link `/history?window=visit` behaves the same as `/history` (Review Focus 5).
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement** with a one-shot `fetch` on press, not a `useFetch` header swap, so a re-render can never send the marker twice. The server is unchanged.
- [ ] **Step 4: Rewrite the spec paragraph:** the mark moves when the reader says so, and only the console's marked request moves it.
- [ ] **Step 5: Run.** Expected: PASS.
- [ ] **Step 6: Commit** `History: catching up is the reader's call`.

### Task 26: The node History tab shows what happened

**Files:**
- Modify: `internal/views/nodepage.go:283-288,329`, `ui/src/api.ts:601-606`, `ui/src/views/node/HistorySection.tsx:37-47`, `ui/src/views/node.css:87-125`
- Test: `internal/views/nodepage_test.go`, `ui/src/components/__tests__/instrument.test.tsx`

**Interfaces:**
- Produces: `HistoryEntry.Outcome string` (`outcome`, omitted when empty), derived from the event's `Before` and `After` maps: for a status change, the new status token; for an attest or eval event, the recorded gate or criterion status; empty otherwise.

- [ ] **Step 1: Write the failing tests.** Go: a `check.feature` event whose `After` records a failed gate gives `outcome: "fail"`; a `start.task` event gives `outcome: "in_progress"`; a `link.task` event gives none. UI: a row reads time, the phrase verb (`checked`), and an outcome chip (`Failed`, or `In progress` through `statusLabel`); the raw command is the row's `title` and is no longer printed; the raw type appears nowhere.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Read two or three real events in a test fixture first to pin which `After` keys carry the gate result; write the derivation against those keys only.
- [ ] **Step 4: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 5: Commit** `Node history: each row shows its outcome`.

---

## Batch F: Drift (I7)

### Task 27: Drift reads like the rest of the console

**Files:**
- Modify: `internal/views/drift.go:47-100,163-183`, `ui/src/api.ts:1488-1546`, `ui/src/lib/drift.ts`, `ui/src/views/Drift.tsx:33-129`, `ui/src/components/DriftFinding.tsx`, `ui/src/views/drift.css`, `ui/src/lib/vocabulary.json`
- Create: `ui/src/lib/drift.test.ts`, `internal/core/drift/vocabulary_internal_test.go`
- Test: `internal/views/drift_view_test.go`, `ui/src/components/__tests__/drift.test.tsx`

**Interfaces:**
- Produces: `title` on `CandidateView` and `UncoveredView` (the node's title; empty for design rows).
- Produces in `lib/drift.ts`: `MARKER_COPY: Record<string, { name: string; meaning: string; next: string }>` and `REASON_COPY: Record<string, { cause: string; fix: string }>`, plus `groupUncovered(rows: UncoveredView[]): { marker: string; reason: string; rows: UncoveredView[] }[]`.

Marker names: `stale_spec` Spec changed after the work, `active_basis` Work started from an older basis, `answered_question` An answered question was not reviewed, `missing_surface` No surface recorded, `orphan_eval` Eval no longer matches its spec, `stale_design` Design doc is behind the code, `shifted_design` Design doc may have shifted. Each gets one sentence of meaning and one of next step, written at implementation time from the marker's doc comment in `internal/core/drift/drift.go:45-150`. `ref_unreadable` reads: cause `The eval's ref could not be read as a file. A ref that names several files must be a YAML list, not one space-separated string.`; fix `Rewrite ref as a list, then run the check again.`

- [ ] **Step 1: Write the failing tests.** Go guard: every marker in `drift.KnownMarkers` is in `vocabulary.drift_markers`. UI: every marker and every reason in the vocabulary has copy with non-empty fields. Rendered: section headings are the plain names with the meaning beneath, never `stale_spec` or `STALE_SPEC`; rows lead with the title and show the key in mono; a candidate with a `remedy` shows it as a copy control, and one without shows the marker's next-step sentence and no command (R12); nine uncovered rows sharing `orphan_eval` and `ref_unreadable` render one group with the cause and fix stated once and nine linked titles beneath; no `[orphan_eval]` bracket token remains.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Fill `drift_reasons` from the reasons named in `drift.go:117-150` and `design.go:48-165`.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Drift: plain names, titles, causes stated once`.

### Task 28: Drift is a Home tab (R4)

**Files:**
- Modify: `ui/src/views/Home.tsx:228-255`, `ui/src/views/Drift.tsx` (export the body), `ui/src/App.tsx:175-181`, `ui/src/views/home/HeaderBand.tsx:138-154`, the node drift panel's link to `/drift` (`grep -rn '"/drift"' ui/src`), `docs/spec/ui-surface.md` ("Home", "Drift report")
- Test: `ui/src/components/__tests__/home.test.tsx` (tabs at 2119), `drift.test.tsx`, `ui/src/components/__tests__/routing.test.tsx` or the file that tests `FixRedirect`

**Interfaces:**
- Produces: `DriftReport({ state }: { state: FetchState<DriftReportView> })` exported from `Drift.tsx`, with no `h1`.

- [ ] **Step 1: Write the failing tests.** Home's rail has a `Drift` tab whose count is candidates plus uncovered, warning-toned when candidates exist, and absent when both are zero. `/?tab=drift` renders the report from the `/api/drift` response Home already holds, with no second request. `/drift` redirects to `/?tab=drift`, keeping any hash (Review Focus 5). The Drift stat tile and the node panel link to `/?tab=drift`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 5: Commit** `Drift: a Home tab, with the old address redirecting`.

---

## Batch G: smaller wins

### Task 29: Search from anywhere (R6)

**Files:**
- Create: `ui/src/components/TopbarSearch.tsx`, `ui/src/lib/useSlashFocus.ts`
- Modify: `ui/src/App.tsx:81-115`, `ui/src/components/Navigator.tsx:231-263` (remove `NavSearch`), `ui/src/views/explore/SearchMode.tsx:72-81` (remove its form), `ui/src/views/explore/SearchResults.tsx`, `internal/search/rank.go:75-91`, `docs/spec/ui-surface.md:171`, `docs/spec/cli-surface.md` (search ordering)
- Test: `internal/search/search_test.go`, `ui/src/components/__tests__/topbar-search.test.tsx` (new), `explore.test.tsx`

**Interfaces:**
- Produces: `useSlashFocus(ref: RefObject<HTMLInputElement | null>): void`: a window `keydown` listener that focuses the input on `/` with no modifier, unless the event target is an `input`, `textarea`, `select`, or `contenteditable` element.
- Produces: rank order `rank class, live before retired, hit count, kind, ref, id`, retired meaning stored status `cancelled` or `archived`.

- [ ] **Step 1: Write the failing tests.** Go: two cancelled epics and one live feature all matching `renewal` by title return the feature first; an exact id match on a cancelled node still ranks first. UI: the header holds one `input[aria-label="Search"]` on every route; `/` focuses it from Home; `/` typed inside a filter combobox does not; submitting navigates to `/explore?mode=search&q=renewal`; on that page the field shows `renewal`; a direct `?mode=search&q=x` URL fills the field (Review Focus 5); Search mode renders no second input and Browse's navigator has none. With Hide cancelled on, search rows with status `cancelled` are not rendered and the summary says how many were hidden.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** The field is `width: 260px` and never moves.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Search: one field in the topbar, live work ranked first`.

### Task 30: Home's stat band links, wraps, and sits under its title

**Files:**
- Modify: `ui/src/views/home/HeaderBand.tsx:35-192`, `ui/src/views/Home.tsx:179-255`, `ui/src/views/home.css:35-77`
- Test: `ui/src/components/__tests__/home.test.tsx` (header band at 1188), `ui/src/views/home.css.test.ts`

**Interfaces:**
- Consumes: Task 7's display-status list filter, so a funnel number and the list it opens agree.

- [ ] **Step 1: Write the failing tests.** The Epics tile links to `/explore?mode=filter&noun=epics`; Questions to `/questions?status=open`; each funnel segment to `/explore?mode=filter&noun=features&status=<column>`. The `h1` and its meta line render above the `Home view` tab rail and on every tab. CSS: under `1200px`, `.home-head` is a column and `.stats` is `flex-wrap: nowrap` with `justify-content: flex-start`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Move the title block out of `HeaderBand` into a `HomeTitle` rendered by `Home` above `NodeTabs`.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Home: every stat links to its list; the title sits above the tabs`.

### Task 31: The ledgers (R9)

**Files:**
- Modify: `ui/src/views/explore/ledgerStatus.ts:34-37`, `ui/src/views/explore/QuestionsPane.tsx:64-160`, `ui/src/views/explore/AmendmentsPane.tsx:67-82`, `internal/views/list.go:25-77,229`, `ui/src/api.ts:927-1006`, `ui/src/views/explore.css`, `docs/spec/ui-surface.md:175`
- Test: `ui/src/views/explore/QuestionsPane.test.tsx`, `internal/views/list_test.go`, `ui/src/components/__tests__/explore.test.tsx`

**Interfaces:**
- Produces: `activeLedgerStatus(params, statuses, fallback?: string)`: with no `?status=`, returns `fallback` when a row holds that status, else All.
- Produces: `ListItem.ProposedAt string` (`proposed_at`, amendments only).

- [ ] **Step 1: Write the failing tests.** Questions opens on Open when any question is open and on All when none is. An open card shows the statement clamped to four lines, the answer command, and a closed `More commands` disclosure holding cancel and each unlink. Blocked features render as a list of linked titles, not a comma-joined mono run. Amendments: rows are a four-column grid (title, key, date, status), the caption reads `Proposed first, then newest`, rows sort that way, and the date uses `<Time>`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Amendment titles that are file names are data, not rendering: leave them.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Ledgers: questions open on Open and show the question; amendments get dates`.

### Task 32: Explore's empty state and Filter mode

**Files:**
- Create: `ui/src/lib/recentNodes.ts`, `ui/src/lib/recentNodes.test.ts`
- Modify: `ui/src/views/Explore.tsx:581-590`, `ui/src/views/NodeDetail.tsx:49`, `ui/src/views/explore/FilterMode.tsx:125-164`, `ui/src/views/explore/FilterCombobox.tsx`
- Test: `ui/src/components/__tests__/explore.test.tsx`, `ui/src/views/explore/FilterCombobox.test.tsx`

**Interfaces:**
- Produces: `recordRecent(projectKey: string, node: { noun: string; id: string; title: string })` and `readRecent(projectKey: string): RecentNode[]`, newest first, deduplicated by id, capped at 8, under `tiller-explore-recent:<projectKey>`, tolerant of a missing or corrupt value.

- [ ] **Step 1: Write the failing tests.** The pure cases. Rendered: with nothing selected the pane shows `Recently opened` with the stored nodes as title-first links, `N decisions need you` linking to Home from `/api/queue`, and `N in flight` linking to Build from `/api/board`; with no history it shows the two links and the old sentence. Filter mode: focusing a combobox opens its option list with nothing typed; results have a header row (`Title`, `Key`, `Status`, `Contributors` or `Owner`) and no row repeats the word `Contributors:`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Explore: a useful empty state; Filter shows its choices`.

### Task 33: The map shows status and order

**Files:**
- Modify: `ui/src/views/explore/map/MapCanvas.tsx:598-636`, `ui/src/views/map.css`
- Test: the existing `ui/src/views/explore/map/*.test.tsx`

**Interfaces:**
- Consumes: `displayStatus` and the map node's `column` from Task 7.

- [ ] **Step 1: Write the failing tests.** A collapsed epic card renders a `StateGlyph` for its display status before the title, and an ordinal (`3`) in `.map-ordinal` matching its position in the served epic order, whichever side it sits on. Shelf branches get neither.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** `sidesOf` is untouched.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Map: epic cards show status and order`.

### Task 34: Branches and Recovered reviews

**Files:**
- Modify: `ui/src/components/ExecutionSources.tsx:30-87`, `ui/src/components/RecoveredReview.tsx`, `ui/src/views/Home.tsx:347-375`
- Test: `ui/src/components/__tests__/execution.test.tsx`, `recovered-review.test.tsx`

**Interfaces:**
- Produces: `ExecutionProvenance` prop `showObserved?: boolean` (default true, so the node page is unchanged).
- Produces: `RecoveredReview` prop `titles?: ReadonlyMap<string, string>` mapping task id to title.

- [ ] **Step 1: Write the failing tests.** Branches: `Observed <time>` appears once, above the list, and no row repeats it; the two explanatory paragraphs sit in a closed `<details>` titled `How branch observation works`. Recovered reviews: the visible body is the reason sentence, the PR link, and riding tasks by title (id when no title is known); the head, merge commit, basis, and policy snapshots sit in a closed `<details>` titled `Details`.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Home builds `titles` from the task list it already fetched.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Home: Branches and Recovered reviews lead with what matters`.

### Task 35: Report labels, order, range, and print (R8)

**Files:**
- Modify: `ui/src/views/report/KpiStrip.tsx:25`, `ThroughputChart.tsx:21-25`, `ChartFrame.tsx:334-383`, `EpicProgressChart.tsx`, `ProjectTab.tsx:13-42`, `ui/src/views/Report.tsx:46-52`, `ui/src/views/report.css`, `ui/src/views/report/charts.css`, `docs/spec/ui-surface.md:257`
- Create: `ui/src/views/report/range.ts`, `ui/src/views/report/range.test.ts`
- Test: `ui/src/components/__tests__/report-page.test.tsx`, `report-charts.test.tsx`

**Interfaces:**
- Produces: `sliceWeeks(report: Report, weeks: number | "all"): Report`, trimming `weeks`, `week_boundaries`, `burn_up`, `throughput`, `bugs`, and `kpis.open_questions_by_week` to the last N weeks and leaving the other KPIs alone.
- Produces: `sortEpics(epics: ReportEpic[]): { open: ReportEpic[]; finished: ReportEpic[] }`: in progress, then not started, then planned; finished separately. Order within each class is the served order.

- [ ] **Step 1: Write the failing tests.** The tile reads `Items shipped, last 7 days` with the sub-line `tasks, bugs, chores, tweaks`. Throughput's y axis is titled `items per week`. Its legend has a hatched swatch labelled `Week so far`. Epic progress lists unfinished epics first and one row `9 epics complete` that opens to their bars. A `Range` select (`Last 8 weeks`, `Last 26 weeks`, `All`) rides `?weeks=` and slices the weekly charts. A `Print` button calls `window.print`. CSS: `@media print` hides `.site-header`, the range select, the print button, and the tab rail, and sets `.report-charts` to one column with `break-inside: avoid` on each chart card.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** Add `report.css` to the Vitest `css.include` list for the print test.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Report: units, unfinished first, a range, and print`.

### Task 36: Reread when the window comes back

**Files:**
- Create: `ui/src/lib/useRefetchOnFocus.ts`, `ui/src/lib/useRefetchOnFocus.test.ts`
- Modify: `ui/src/App.tsx`, `ui/src/api.ts:256-275`, `docs/spec/ui-surface.md` ("Liveness")

**Interfaces:**
- Produces: `useRefetchOnFocus(minAgeMs = 30_000): void`: on `visibilitychange` to visible or window `focus`, calls `bumpFetchGeneration()` when `getLastFetchAt()` is older than `minAgeMs`.

- [ ] **Step 1: Write the failing tests** with fake timers. Focus after 31 seconds bumps the generation once; focus after 5 seconds does not; `visibilitychange` and `focus` firing together bump once; a hidden document never bumps (Review Focus 1). In `history.test.tsx`: a focus refetch on History sends no `X-Tiller-Visit` header.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** `Console: reread when the window regains focus`.

### Task 37: Tabs are tabs

**Files:**
- Modify: `ui/src/components/NodeTabs.tsx:49-78`
- Test: `ui/src/components/__tests__/nodetabs.test.tsx`, and every test that queries `aria-current` on a tab (`grep -rn 'aria-current' ui/src --include=*.test.tsx`)

- [ ] **Step 1: Write the failing tests.** The rail is `role="tablist"` with its `aria-label`; each button is `role="tab"` with `aria-selected` and `aria-controls`; only the selected tab has `tabIndex=0`; ArrowRight and ArrowLeft move focus and selection and wrap; Home and End jump to the ends; the body is `role="tabpanel"` labelled by its tab.
- [ ] **Step 2: Run.** Expected: FAIL.
- [ ] **Step 3: Implement.** The wrapper stops being a `nav`. Keep the `mode-nav` class so the styling holds.
- [ ] **Step 4: Run, then the batch gate.** Expected: PASS.
- [ ] **Step 5: Commit** `Console: tab rails are announced and keyed as tabs`.

---

## After the last batch

- [ ] Reshoot the site's console screenshots that changed (Home, Fix, Build, Explore, History) from the Acme stand-in project.
- [ ] Walk every page at 1512 by 900 and at 1024 wide, light and dark, in Safari and Chromium, against the hunt's list. Record anything that did not land.
- [ ] Move the rulings that held into the settled decisions of `docs/spec/ui-surface.md`, then delete this plan and the hunt document.
