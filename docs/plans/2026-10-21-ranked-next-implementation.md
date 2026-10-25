# Ranked Next Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One deterministic engine, `views.NextOf`, ranks every legal next move for the reading contributor and every surface renders its list: `tiller next`, bare `tiller guide`, the readout's **Next:**, a short **Up next** block after `submit` and `check`, and Home's Up next. The single route walk (`RouteDecisionOf`) and its notes are retired.

**Architecture:** Three pure parts in `internal/views`: enumerate (every arm of today's walk run to exhaustion, plus every `QueueOfInput` entry), locate (the reader's counted events in a fourteen-day window give a current feature and a recently touched list), and rank (five tiers, plan-order tie-break, dedupe by node, kept-Continue rule). The only new I/O is `events.ListSince`, which reads only the date partitions inside the window. Build 1 delivers the engine, CLI, skills, and orientation, and keeps `/api/route` alive as a thin projection of the top move. Build 2 replaces `/api/route` with `/api/next` and rebuilds Home.

**Tech Stack:** Go, cobra, testify, `internal/testgit`; React/TypeScript console in `ui/` with vitest.

**Spec:** `docs/plans/2026-10-21-ranked-next-design.md` (decisions 1 to 12). Read it first; this plan cites decisions by number.

## Rulings for JC

The design leaves these open or rests on a fact the code contradicts. Each has a default the plan builds; overturn any before execution.

1. **There is no batch `add`.** Every `tiller add` writes one `add.<noun>` event with one target. Decision 12's "more than a few targets" becomes: an event naming more than three targets is not counted. No counted type writes that many today, so the cap is a guard against future multi-target events.
2. **Amendment events do not localize.** `amendment_opened` and `amendment_staged` count as deliberate acts, but an amendment id maps to no feature, so they never move the current feature.
3. **Plan-order tie-break across kinds.** Inside a tier, candidates sort by the old walk's arm order first (the "arm-order cases survive as plan-order tie-break cases" line of the build sequence), then by the arm's own order (`gen.SortExecutionTasks`, queue order, flow order).
4. **A claimed but unstarted task is Continue.** Decision 4's tier 2 names in-progress and check-pending work; the plan also counts the reader's own `ready` task, which the old walk's owned pass already treated as theirs.
5. **Deadlines rank without identity.** With no reader, tiers 2 to 4 are empty but a question due within two days still enters tier 1 (decision 8, "wherever it sits").
6. **Caught up keeps the pool explanation.** "Caught up: nothing is legal to do right now." is followed by `PoolSignpost`'s sentence when held unclaimed work exists, and by the empty-tree hint when the tree has no epics.
7. **The orient readout drops its `Route` check line.** The ranked **Next:** carries the same answer.
8. **Readout JSON changes shape.** `next` becomes the `Next` object below and `route_advisory` is removed. Text skills are unaffected.
9. **`/api/route` survives build 1 as an adapter** over the top candidate so the console keeps working if build 1 is stamped alone; build 2 deletes it.
10. **Generation.** Orientation moves to the next free generation: 54, or 55 if the gate reuse build (`docs/plans/2026-10-21-gate-reuse-implementation.md`, also 53 to 54) lands first.

## Global Constraints

- Tiller never calls a language model; the ranking is a fixed rule (decision 1).
- The list holds at most five; fewer when fewer exist. `--all` lists every candidate (decision 11).
- One row per node; a node keeps its highest-ranked reason (decision 10).
- Tier order: 1 Unblock nearby, 2 Continue, 3 Nearby (feature, then epic), 4 Recently touched, 5 Everything else (decision 4).
- Recency window: fourteen days. Deadline window: overdue or due within two days (decisions 8, 12).
- Counted event types (decision 12): `claim.*`, `start.*`, `submit.*`, `check.*`, `spec.*`, `approve.*`, `answer.*`, `attest.*`, `add.*`, `link.*`, `reassign.*`, `release.*`, `amendment_opened`, `amendment_staged`; only events whose `Contributor` is the reader. Never `sync.*`, `migrate.*`, `contributor.*`, `join`, `upgrade`.
- `views` stays pure: no file, git, or clock reads. Time arrives as `NextInput.Now`.
- A failed event read degrades to no events (unpersonalized recency), the way `readinput.WorkflowObserved` degrades.
- Copy: no em dashes in Go strings, docs, skills, or comments. Run `grep -n "—"` on every touched file before committing.
- Commits: `git commit -s`, message `Area: imperative summary`, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Build in the `ranked-next` worktree; main is fast-forwarded only when JC says so.
- Run `make test` and `make lint` before each commit; build 2 also runs `cd ui && npm test` and `make ui`, committing `ui/dist`.

## Review Focus

1. **A reader with no identity** (`TILLER_CONTRIBUTOR` unset, no `.tiller/identity.yml`). The list must fall back to plan order, `tiller next` and Home must say it is not personalized, and nothing may claim "your". Tests in Tasks 4, 5, and 13.
2. **Event targets that no longer resolve:** a removed draft feature, a design path (`link.design` targets `.tiller/design/x.md`), a question with empty `blocks`. Locate must skip them, not panic or pick a stale current feature. Test in Task 2.
3. **The UTC window edge.** Partitions are dated by UTC; a reader at 23:30 local time west of UTC must still see the event they wrote ten minutes ago. Test in Task 1.
4. **Question due values that are hints or malformed** (`due: {hint: "before launch"}`, `due: {date: "next week"}`). No deadline boost and no error. Test in Task 4.
5. **One node admitted by several arms:** an in-review task with an unmet `attest_gate` and a merge wait; a specced feature that is both the reader's current feature and `approve_feature`. It must appear once, at its best tier. Test in Task 4.

---

## Build 1: Engine and CLI

### Task 1: Windowed event reader

**Files:**
- Modify: `internal/core/events/events.go` (beside `List`)
- Test: `internal/core/events/events_test.go`

**Interfaces:**
- Produces: `func ListSince(tree *store.Tree, since time.Time) ([]Event, error)`: events with `At` after `since`, sorted like `List`, reading only `YYYY/MM/DD` directories whose UTC date is on or after `since.UTC()`'s date. Same process cache, same confined reader, same missing-directory rule as `List`.

- [ ] **Step 1: Write the failing tests**
  - `TestListSinceSkipsOlderPartitions`: write events dated 2026-10-01, 2026-10-10, 2026-10-20 (UTC). `ListSince(tree, 2026-10-07T00:00Z)` returns the last two. Then replace the 2026-10-01 file with invalid YAML; `ListSince` still succeeds (proves the old partition is never read) while `List` fails.
  - `TestListSinceUsesTheUTCPartitionForTheBoundaryDay`: an event at `2026-10-21T06:30Z`, `since` = `2026-10-20T23:00-08:00` (which is 2026-10-21T07:00Z, after the event) excludes it; `since` = `2026-10-20T22:00-08:00` includes it.
  - `TestListSinceMissingEventsDirIsEmpty`: no `.tiller/events/` returns `nil, nil`.

- [ ] **Step 2: Run** `go test ./internal/core/events -run TestListSince -v`. Expected: FAIL, `undefined: ListSince`.

- [ ] **Step 3: Implement `ListSince`.** Factor `List`'s walk into `listWhere(tree, keep func(relDir string) bool)`; `List` passes a keep-all, `ListSince` parses the three directory levels and prunes a year, month, or day directory that ends before `since`'s UTC date. Filter `e.At.After(since)` after decoding.

- [ ] **Step 4: Run** `go test ./internal/core/events -v`. Expected: PASS.

- [ ] **Step 5: Commit** `Events: read only the partitions inside a time window`

---

### Task 2: Locate the reader's work

**Files:**
- Create: `internal/views/next_locate.go`, `internal/views/next_locate_test.go`

**Interfaces:**
- Consumes: `events.Event`.
- Produces:
  ```go
  const recentWindow = 14 * 24 * time.Hour
  type Locality struct {
      CurrentFeature string   // feature id, "" when none
      CurrentEpic    string   // epic key, "" when none
      Touched        []Touch  // most recent first, CurrentFeature included
  }
  type Touch struct { FeatureID, EpicKey string; At time.Time }
  func LocateOf(t *store.Tree, evs []events.Event, reader string, now time.Time) Locality
  func countedEvent(ev events.Event, reader string) bool
  ```

- [ ] **Step 1: Write the failing tests** (build trees with the existing `treeWithDraftEpics`, `readyFeatureWithTask` helpers):
  - `TestLocateCurrentFeatureIsTheMostRecentCountedEvent`: reader's `claim.task` on a task of F1 at day -3, `approve.feature` on F2 at day -1: `CurrentFeature == F2`, `CurrentEpic == F2.EpicKey`, `Touched` is `[F2, F1]`.
  - `TestLocateIgnoresOtherContributorsAndUncountedTypes`: another contributor's later `start.task`, and the reader's later `sync.task` and `migrate.schema`, leave `CurrentFeature == F1`.
  - `TestLocateMapsQuestionEventsThroughBlocks`: reader's `answer.question` on Q whose `blocks` names task T in F3: current is F3. A question with empty `blocks` maps to nothing.
  - `TestLocateEpicEventSetsEpicOnly`: reader's latest event is `approve.epic` on E2 and an earlier one touches F1 in E1: `CurrentFeature == F1`, and `Touched` holds an epic-only touch `{EpicKey: E2}` ahead of F1.
  - `TestLocateSkipsUnresolvableTargets` (Review Focus 2): targets for a removed feature id, `.tiller/design/x.md`, and an amendment id produce no touch and do not become current.
  - `TestLocateDropsEventsOutsideTheWindow`: an event at `now - 15 days` is ignored.
  - `TestLocateSkipsEventsWithMoreThanThreeTargets` (ruling 1).
  - `TestLocateWithNoReaderIsEmpty`: `reader == ""` returns the zero `Locality`.

- [ ] **Step 2: Run** `go test ./internal/views -run TestLocate -v`. Expected: FAIL, `undefined: views.LocateOf`.

- [ ] **Step 3: Implement.** Map each target: task to its feature and epic, feature to itself and epic, epic to an epic-only touch, question through its first `blocks` entry that resolves (use the tree's id and key lookup the readout already uses, `resolveWorkNode`). Walk events newest first; the first feature touch sets `CurrentFeature`; `CurrentEpic` is that feature's epic key, else the newest epic-only touch. Each feature or epic appears once in `Touched` at its newest time.

- [ ] **Step 4: Run** `go test ./internal/views -run TestLocate -v`. Expected: PASS.

- [ ] **Step 5: Commit** `Views: locate the reader's current and recent work from events`

---

### Task 3: Enumerate every legal move

**Files:**
- Create: `internal/views/next.go` (types), `internal/views/next_enumerate.go`, `internal/views/next_enumerate_test.go`
- Modify: `internal/views/readout.go` (add `frontierFeatures` beside `FrontierSeam`)

**Interfaces:**
- Consumes: `QueueOfInput`, `ownedExecutionCandidates`, `gen.UnclaimedTaskCandidates`, `firstRoutableShelfNode`'s filter, `workflow.AdviceOf`, `inReviewWaitReason`, `firstShelfMergeWait`'s filter, `baselineRequired`, `emptyTreeRoute`'s decompose test, `observedTask`.
- Produces:
  ```go
  type NextNode struct { Noun, ID, Key, Ref, Title string }       // json: noun,id,key,ref,title (key,title omitempty)
  type NextCandidate struct {
      Actor    string           // "human" | "agent"           json:"actor"
      Kind     string           // see table                   json:"kind"
      Action   string           // short verb phrase           json:"action"
      Node     *NextNode        //                             json:"node,omitempty"
      Command  string           //                             json:"command,omitempty"
      Skill    string           // agent moves only            json:"skill,omitempty"
      Reason   string           //                             json:"reason"
      Tier     int              // 1..5, set by rank           json:"tier"
      Branch   string           // own observed task           json:"branch,omitempty"
      Workflow *workflow.Advice //                             json:"workflow,omitempty"
      arm, pos int              // tie-break, unexported
      due      string           // question due date, unexported
  }
  type NextInput struct {
      Reader   string
      Workflow workflow.Input
      Queue    QueueInput
      Recent   []events.Event
      Now      time.Time
  }
  func candidatesOf(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate
  func frontierFeatures(t *store.Tree, order *gen.Order) []*store.Feature
  ```
  Kinds, with arm rank (tie-break), actor, and command or skill:

  | arm | kind | actor | command / skill |
  |---|---|---|---|
  | 0 | `confirm_design` | human | `tiller approve design <path>` |
  | 0 | `derive_baseline` | agent | skill `derive-baseline` |
  | 1 | `continue` (own task: `check`, then `in_progress`/`ready`; then own shelf work) | agent | skill `execute`; none when observed on another branch |
  | 2 | `approve_amendment` | human | queue command |
  | 3 | `workflow_closeout`, `feature_check` | entry actor | queue command; skill `execute` when agent |
  | 4 | `claim_task` | agent | `tiller claim task <addr>`, skill `execute` |
  | 5 | `resolve_blocker` | human | none |
  | 6 | `approve_feature`, `approve_tasks` | human | queue command |
  | 7 | `approve_epic` | human | queue command |
  | 8 | `specify` | agent | skill `specify` |
  | 9 | `merge_task` (task or shelf `in_review` human wait) | human | `tiller sync` |
  | 10 | `merge_feature`, `merge_epic` | human | queue command (`tiller sync`) |
  | 11 | `answer_question` | human | queue command |
  | 12 | `attest_gate`, `attest_criterion` | human | queue command |
  | 13 | `approve_bug`, `approve_chore`, `approve_tweak`, `carrier_cancelled` | human | queue command |
  | 14 | `decompose` | agent | skill `decompose` |

- [ ] **Step 1: Write the failing tests**, one per row group, each asserting kind, actor, node id, and command or skill on `candidatesOf`'s output:
  - `TestEnumerateBaselineIsExclusive`: brownfield, unconfirmed: only `derive_baseline`; with two registered pending docs, only two `confirm_design`.
  - `TestEnumerateOwnWorkCheckedFirst`: reader owns a `check` task and an `in_progress` task, plus an owned bug: three `continue` candidates, check first, bug last.
  - `TestEnumerateOwnObservedTaskCarriesItsBranch`: `in.Workflow.Observed[task] = {Branch: "t/x", Owner: reader}` on an unclaimed eligible task: one `continue` with `Branch == "t/x"` and empty `Command`.
  - `TestEnumerateExcludesOthersInFlightWork`: the same observation owned by someone else: the task is absent.
  - `TestEnumerateClaimableListsEveryEligibleTask`: three eligible unclaimed tasks: three `claim_task`, in `SortExecutionTasks` order.
  - `TestEnumerateQuestionHeldTaskYieldsOnlyItsQuestion`: the held task is absent; its question is present as `answer_question`.
  - `TestEnumerateDependencyBlockedWorkIsAbsent`: a task blocked by an unfinished dependency is not a candidate.
  - `TestEnumerateEveryQueueEntryIsACandidate`: a tree producing one entry of each `QueueKinds` kind maps each to a candidate of the same kind and command.
  - `TestEnumerateSpecifiesEveryFrontierDraft`: two draft features with satisfied dependencies and one blocked: two `specify`.
  - `TestEnumerateInReviewMergeWaits`: a task-grain `in_review` task with a PR, and an `in_review` chore: two `merge_task`. A coarse-grain task whose feature has no review recorded: absent.
  - `TestEnumerateEmptyTree`: no epics plus a to-be doc: one `decompose`. No epics and no doc: no candidates.

- [ ] **Step 2: Run** `go test ./internal/views -run TestEnumerate -v`. Expected: FAIL, undefined.

- [ ] **Step 3: Implement.** One function per arm returning `[]NextCandidate` with `arm` set and `pos` its index; `candidatesOf` concatenates them. Baseline short-circuits as tabled. `frontierFeatures` returns every feature `FrontierSeam` would accept, in its rank order; `FrontierSeam` becomes `frontierFeatures(t, order)[0]` when non-empty. Queue entries map one-to-one (`Actor` defaults to `human`; `Node.Title` from `Prose` for approvals). `Action` copy: `continue`, `claim and start`, `specify`, `decompose`, `derive the baseline for`, `confirm design doc`, `approve amendment`, `approve epic`, `approve the spec for`, `approve the tasks for`, `triage`, `answer`, `attest`, `merge`, `resolve the blocker on`, and the advice's `Action` for closeout. `Reason` here is the base reason (the old walk's reason text for that arm, or the queue prose); Task 4 replaces it when a tier gives a better one.

- [ ] **Step 4: Run** `go test ./internal/views -run 'TestEnumerate|TestFrontier' -v`. Expected: PASS.

- [ ] **Step 5: Commit** `Views: enumerate every legal next move`

---

### Task 4: Rank and `NextOf`

**Files:**
- Create: `internal/views/next_rank.go`, `internal/views/next_rank_test.go`
- Modify: `internal/views/next.go`

**Interfaces:**
- Consumes: `candidatesOf`, `LocateOf`, `PoolSignpost`, `unplannedReason`, `readiness.DependencyDiagnostics`.
- Produces:
  ```go
  const shortlistSize = 5
  type Next struct {
      Reader                string            `json:"reader"`
      Candidates            []NextCandidate   `json:"candidates"`   // full ranked list, never null
      CaughtUp              bool              `json:"caught_up"`
      Unplanned             bool              `json:"unplanned,omitempty"`
      Hint                  string            `json:"hint,omitempty"`
      DependencyDiagnostics *readiness.Report `json:"dependency_diagnostics,omitempty"`
  }
  func NextOf(t *store.Tree, in NextInput) Next
  func (n Next) Shortlist() []NextCandidate     // top five with the kept-Continue rule
  func (n Next) TopAgent() (NextCandidate, bool)
  ```

- [ ] **Step 1: Write the failing tests** (`now` fixed at `2026-10-21T12:00Z`; reader events built with `events.Event{Type, Contributor, Targets, At}`):
  - `TestRankUnblockNearbyLeads`: current feature F holds a question Q on its task and a specced sibling task batch: `answer_question` Q and `approve_tasks` F are tier 1 ahead of the reader's `continue`. Reason for Q: `holds <task ref> in your current feature`.
  - `TestRankDeadlinesOutrankLocality`: a question in an unrelated epic with `due.date` `2026-10-22` is tier 1, sorted ahead of a current-feature question with no date; one dated `2026-10-19` sorts first with reason `overdue since 2026-10-19`; `2026-10-21` reads `due today`, `2026-10-22` `due tomorrow`, `2026-10-23` `due 2026-10-23`; `2026-10-30` gets no boost.
  - `TestRankIgnoresHintAndMalformedDue` (Review Focus 4): `{hint: "before launch"}` and `{date: "next week"}` stay in their locality tier.
  - `TestRankKeepsTopContinue`: six tier-1 candidates and one `continue`: `Shortlist()` holds five, the fifth is the `continue`.
  - `TestRankNearbyFeatureThenEpic`: claimable tasks in the current feature (reason `next in your current feature`) rank before one elsewhere in its epic (`in your current epic`), both ahead of an earlier-ordered epic's task.
  - `TestRankRecentlyTouchedMostRecentFirst`: features touched 2 and 9 days ago: the 2-day one first, reason `in <feature ref>, which you worked on 2 days ago`; 1 day reads `yesterday`, 0 reads `today`.
  - `TestRankEverythingElseKeepsArmThenPlanOrder` (ruling 3): with no reader and no events, an amendment, a specced feature, and a draft feature rank `approve_amendment`, `approve_feature`, `specify`.
  - `TestRankOneRowPerNode` (Review Focus 5): an in-review task with an unmet `attest_gate` and a merge wait yields one row, the better-ranked; a current specced feature yields one `approve_feature` row in tier 1.
  - `TestNextWithoutReaderIsPlanOrder` (Review Focus 1): `Reader == ""`, events present: `Next.Reader == ""`, no tier 2 to 4, no reason contains `your`.
  - `TestNextCaughtUp`: nothing legal, held pool: `CaughtUp`, `Hint` is the `PoolSignpost` sentence. No epics and no doc: `Unplanned`, `Hint` is `unplannedReason`.
  - `TestTopAgentSkipsHumanMoves`: top two human, third agent: `TopAgent()` returns the third.

- [ ] **Step 2: Run** `go test ./internal/views -run 'TestRank|TestNext|TestTopAgent' -v`. Expected: FAIL.

- [ ] **Step 3: Implement.** `NextOf` calls `candidatesOf`, `LocateOf(t, in.Recent, in.Reader, in.Now)`, assigns tiers, sorts, dedupes, and fills `Hint`/`DependencyDiagnostics` (only when it has gaps or advisories). Tier 1 human candidates "hold the current feature" when their node is the current feature, one of its tasks, its epic, a question returned by `FeatureQuestionBlockers` or `TaskQuestionBlockers` for it or its tasks, or a review unit (`merge_*`, closeout) for it or its epic. Sort key: tier; inside tier 1, dated questions by date ascending, then questions holding the current feature, then the rest; inside tiers 4, touch recency; then `arm`, then `pos`. Parse `due.date` with `time.Parse("2006-01-02", ...)` against `in.Now`'s UTC date; a parse failure is no date. Dedupe keeps the first row per `Node.ID`. `Shortlist` applies decision 4's kept-Continue rule when the first five are all tier 1.

- [ ] **Step 4: Run** `go test ./internal/views -v`. Expected: PASS (the old walk still exists alongside).

- [ ] **Step 5: Commit** `Views: rank next moves by tier and plan order`

---

### Task 5: `tiller next`

**Files:**
- Create: `internal/cli/next.go`, `internal/cli/next_test.go`, `internal/cli/next_render.go`
- Modify: `internal/cli/root.go` (register `newNextCmd()` beside `newGuideCmd()`), `internal/readinput/workflow.go`, `docs/tiller-commands.md` (`## Orient and guidance`)

**Interfaces:**
- Consumes: `views.NextOf`, `Next.Shortlist`, `events.ListSince`, `addContributorFlag`, `resolveActingContributor`.
- Produces:
  - `func readinput.ForNext(t *store.Tree, reader string, now time.Time) views.NextInput`: `WorkflowObserved(t)`, `events.ListSince(t, now.Add(-14*24*time.Hour))` (error degrades to nil).
  - `func renderNextLines(n views.Next, all bool) string` and `func renderNextHeading(n views.Next) string`, used by Tasks 6, 7, and 9.
  - Line copy (decision 11; `You` for human, `Agent` for agent):
    ```
    1. You: approve the spec for checkout/alpha (F-ab12). Holds checkout/alpha/t1 (T-cd34) in your current feature. `tiller approve feature checkout/alpha`
    2. Agent: continue checkout/alpha/t0 (T-ef56). Your task is in progress. `tiller guide execute`
    ```
    An agent move with no command prints `` `tiller guide <skill>` ``; an observed task prints `Continue it on <branch>.` with no command.
  - Headings: `Up next for <reader>:`; `Up next (not personalized; run `tiller join`):`. Caught up: `Caught up: nothing is legal to do right now.` then `Hint` on its own line. Below a cut list with more candidates: `<n> more legal moves; `tiller next --all` lists them.`

- [ ] **Step 1: Write the failing tests** in `next_test.go` using the CLI harness the guide tests use (`internal/cli/guide_test.go`):
  - `TestNextPrintsTheShortlistForTheIdentity`: identity file `contributor: alice`, a tree with seven candidates: heading `Up next for alice:`, five numbered lines, the `2 more legal moves` line.
  - `TestNextAllPrintsEveryCandidate`: `--all`: seven lines, no "more" line.
  - `TestNextContributorFlagRanksForAnother`: `--contributor bob` heads `Up next for bob:` and ranks bob's own task as tier 2.
  - `TestNextWithoutIdentitySaysNotPersonalized` (Review Focus 1).
  - `TestNextJSONMatchesTheNextPayload`: `--json` data decodes to `views.Next` with five `candidates` (all with `--all`).
  - `TestNextCaughtUpAndUnplanned`: both terminal texts.

- [ ] **Step 2: Run** `go test ./internal/cli -run TestNext -v`. Expected: FAIL.

- [ ] **Step 3: Implement** `newNextCmd()` (`Use: "next"`, `--all`, `addContributorFlag`), read-only, `emitSuccess(cmd, "next", payload, human)`, where the JSON payload is `views.Next` with `Candidates` cut to the shortlist unless `--all`. Add the `tiller next` entry to `docs/tiller-commands.md`.

- [ ] **Step 4: Run** `go test ./internal/cli -run TestNext -v`, then `make test`. Expected: PASS.

- [ ] **Step 5: Commit** `CLI: add tiller next`

---

### Task 6: Readout and correction render the ranked list

**Files:**
- Modify: `internal/views/readout.go` (`Readout`, `ReadoutInput`, `ReadoutOfInput`, `orientContent`), `internal/views/correction.go`, `internal/cli/readout.go` (`gatherReadoutInput`, `renderReadoutFor`, `renderCorrectionOf`), `internal/cli/readout_render.go` (`renderReadout`, `renderCorrection`, delete `renderNext`, `ownerClause`)
- Test: `internal/views/readout_test.go`, `internal/cli/readout_render_test.go`, goldens `internal/cli/testdata/readout_correction.golden.md`, `readout_decompose.golden.md`

**Interfaces:**
- Consumes: `NextOf`, `renderNextLines`, `readinput.ForNext`.
- Produces: `Readout.Next views.Next` (`json:"next"`) and `Correction.Next views.Next`; `ReadoutInput.Recent []events.Event`, `ReadoutInput.Now time.Time`; the same two fields on `CorrectionInput`. `Readout.RouteAdvisory` and `Correction.RouteAdvisory` are deleted (ruling 8). The orient `Route` check is deleted (ruling 7). Rendered block ends:
  ```
  **Next:**
  1. ...
  ```
  with the shortlist and the "more" line, after which `GrainNotes` render as today.

- [ ] **Step 1: Update the failing tests first.** In `readout_test.go` and every other test that reads a readout or correction (`grep -rln 'RouteAdvisory\|\.Next\[' internal`, including `internal/cli/readout_observed_test.go`), rewrite every assertion on `r.Next[0].Text`, `r.RouteAdvisory`, and the orient `Route` check as assertions on `r.Next.Shortlist()[0]` kind, node, and reason. Add `TestReadoutNextIsTheShortlist` (a gate readout's `Next` equals `NextOf` for the same input) and `TestOrientReadoutHasNoRouteCheck`. In `readout_render_test.go` add `TestRenderReadoutEndsOnTheRankedList`. Regenerate the goldens after Step 3 and review the diff by hand.

- [ ] **Step 2: Run** `go test ./internal/views ./internal/cli -run 'Readout|Correction' -v`. Expected: FAIL to compile on the new fields.

- [ ] **Step 3: Implement.** `ReadoutOfInput` builds `NextInput{Reader: in.Contributor, Workflow: in.Workflow, Queue: in.QueueInput, Recent: in.Recent, Now: in.Now}` and sets `r.Next = NextOf(...)`; delete the `nextAction` call. `gatherReadoutInput` and the gate path fill `Recent`/`Now` through `readinput.ForNext`. Delete `nextAction`, `baselineNext`, `approveEpicNext` once nothing calls them.

- [ ] **Step 4: Run** `make test`. Expected: PASS. The walk still exists for guide and `/api/route`, so its own tests are untouched until Task 8.

- [ ] **Step 5: Commit** `Readout: end every block on the ranked next list`

---

### Task 7: Bare `tiller guide` prints the list and suggests the top agent move

**Files:**
- Modify: `internal/cli/guide.go` (`runGuideSuggest`, `suggestSkill`, `SuggestView`)
- Test: `internal/cli/guide_test.go`

**Interfaces:**
- Consumes: `NextOf`, `Next.TopAgent`, `renderNextLines`, `readinput.ForNext`.
- Produces: `SuggestView{Skill, Reason, Queue string; Next views.Next}` (`json:"next"`, shortlist). Human output: the `tiller next` block, a blank line, then `Suggested skill: <skill>`, `<reason>`, the queue line, `` Run `tiller guide <skill>` to load it. `` The suggestion is `TopAgent()`'s skill and reason; with no agent move it is `orient` with `No agent move is legal right now; the moves above are yours.`; caught up keeps `orient` with the `Hint`.

- [ ] **Step 1: Rewrite the failing tests.** Port each `suggestSkill` case in `guide_test.go` to assert the skill chosen from the top agent move. Add `TestGuideSuggestsTheTopAgentMoveBelowHumanMoves` (top move is `approve_feature`, suggestion is `execute` for the reader's task) and `TestGuidePrintsTheListFirst`. Delete assertions on `Order note`, `Continuity note`, `Execution note`, `Selected route:`, and the parallel line.

- [ ] **Step 2: Run** `go test ./internal/cli -run TestGuide -v`. Expected: FAIL.

- [ ] **Step 3: Implement** with the per-skill reason wording kept local to `guide.go` as today.

- [ ] **Step 4: Run** `go test ./internal/cli -run TestGuide -v`. Expected: PASS.

- [ ] **Step 5: Commit** `Guide: print the ranked list and suggest the top agent move`

---

### Task 8: Retire the walk; `/api/route` as an adapter

**Files:**
- Modify: `internal/views/readout.go` (delete `RouteDecision`, `ObservedTask` if unused, `RouteQuestionHold`, `Explanation`, `JoinNote`, `Advisory`, `Route`, `RouteTarget`, `RouteDecisionOf`, `routeDecisionOf`, `routeQuestionHoldNote`, `sharedQuestionTarget`, `noRouteDue`), `internal/views/route_claimable.go` (delete `claimableWalk`, `claimableExecution`, `observedRoute`, `readerAffinity`, `continuityNote`, `ClaimableExecution`, `observedMuteReason`, `observedNotes`), `internal/views/route_epic_gate.go` (delete `EpicApprovalCommand`), `internal/views/workflow.go` (delete `workflowRoute`), `internal/views/workitem_route.go` (delete `firstRoutableShelfNode`, `firstShelfMergeWait` once Task 3's enumerators replace them), `frontierBlocked` and `FrontierSeam` in `readout.go` (the frontier-blocked wait is not a legal move, decision 2), `internal/views/zero_day.go` (`emptyTreeRoute` returns what Task 3 needs), `internal/serve/route.go`
- Test: delete `internal/views/route_continuity_test.go`; port `readout_test.go` walk cases, `route_claimable_test.go`, `route_observed_test.go`, `route_review_grain_test.go`, `route_epic_gate_test.go`, `workitem_route_test.go`, `workflow_test.go`, `execute_handoff_test.go`, `baseline_test.go`, `zero_day_test.go`, `recovered_review_test.go`, `mirror_drift_test.go`, `internal/serve/route_test.go`, `internal/cli/requires_test.go`, `internal/cli/readout_observed_test.go`, and the remaining single references (`gen/execution_cycles_test.go`, `gen/execution_review_cycles_test.go`, `core/readiness/execution_cycles_test.go`, `core/mutate/started_grain_test.go`, `views/readout_execution_cycles_test.go`)

**Interfaces:**
- Consumes: `NextOf`, `Shortlist`.
- Produces: a test helper `topMove(t *testing.T, tree *store.Tree, reader string, in workflow.Input) views.NextCandidate` in `internal/views/next_helpers_test.go`; `/api/route` unchanged on the wire, filled from `Shortlist()[0]`: `seam` = its `Skill` (empty for human moves), `reason` = its `Reason`, `node`, `workflow`; `dependency_diagnostics` from `Next`.

- [ ] **Step 1: Port the tests.** Rules for each old assertion:
  - `decision.Seam`/`decision.Node` for an unpersonalized reader becomes `topMove(...).Kind`/`.Node.ID` (ruling 3 keeps arm order, so most cases keep their expected node).
  - `ClaimableExecution()` becomes `Kind == "claim_task"`.
  - `SkippedQuestionHold` becomes "the held task is absent and its question is a candidate".
  - `Observed` becomes `Kind == "continue"` with `Branch`; `SkippedObserved` becomes absence; `Parallel` becomes the next `claim_task` in the list.
  - Note text assertions (`Order note`, `Continuity note`, `Execution note`, `Next eligible`) are deleted; where the note proved a ranking fact, assert the ranking instead.
  - Serve tests keep their names and assert the adapter's fields.

- [ ] **Step 2: Run** `make test`. Expected: FAIL only where code still references the walk.

- [ ] **Step 3: Delete the walk** and switch `serve/route.go` to the adapter (observations from `executionSnapshot(r)` as today, `Recent` from `events.ListSince` with the process cache, `Now` from `time.Now()`).

- [ ] **Step 4: Run** `make test && make lint`. Expected: PASS. `grep -rn 'RouteDecisionOf\|Continuity note\|Order note\|Execution note\|Next eligible' internal` returns only skill and orientation text, which Task 10 rewrites.

- [ ] **Step 5: Commit** `Views: retire the route walk for ranked next`

---

### Task 9: `submit` and `check` print a short Up next block

**Files:**
- Modify: `internal/cli/lifecycle_cmds.go` (`runTransition` submit tail, `runSubmitNode`), `internal/cli/check.go` (`runCheck` pass path)
- Test: `internal/cli/next_after_test.go`

**Interfaces:**
- Consumes: `readinput.ForNext`, `NextOf`, `renderNextLines`.
- Produces: `func appendUpNext(cmd *cobra.Command, root string, human string) (string, views.Next)`: the success text plus `\n\n**Up next:**\n` and the shortlist lines; JSON gains `"next"` beside the existing data. A failed or blocked `check` and a refused `submit` print nothing new. `start`, `claim`, and `resume` are untouched (decision 9).

- [ ] **Step 1: Write the failing tests:** `TestSubmitPrintsUpNext`, `TestCheckPassPrintsUpNext`, `TestCheckFailurePrintsNoUpNext`, `TestStartPrintsNoUpNext`, `TestSubmitJSONCarriesNext`.

- [ ] **Step 2: Run** `go test ./internal/cli -run 'UpNext|CarriesNext' -v`. Expected: FAIL.

- [ ] **Step 3: Implement** `appendUpNext` in `next_render.go` and call it from the two success paths. The tree is reloaded after the mutation so the list reflects the new state.

- [ ] **Step 4: Run** `make test`. Expected: PASS.

- [ ] **Step 5: Commit** `CLI: print the ranked list after submit and check`

---

### Task 10: Skills, orientation, generation

**Files:**
- Modify: `internal/gen/orientation.go` (`orientationSchemaGen`, its history comment, "Report through the readout", the route-is-guidance paragraph, "How to work", `### Recorded-state route` rewritten as `### Ranked next`, the Order note paragraph removed), `internal/skills/orient/SKILL.md` (:32, :80, :88, :164-171), `internal/skills/execute/SKILL.md` (`## Readout`)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

**Interfaces:**
- Produces: orientation and skill text stating decision 6's rule. Orient: "Run `tiller next` (or read the readout's **Next:**). Relay the list as written. If any human move ranks above the top agent move, name it first. Then start the top agent move unless the developer redirects. You may say you would take a different item first and why; never rewrite the list. A ranked move is a suggestion, not authorization." Execute and every seam skill: the block now ends on the ranked list; keep reproducing it verbatim.

- [ ] **Step 1: Rewrite the failing tests:** replace `TestOrientationRanksUnclaimedWorkByReaderHistory` with `TestOrientationDescribesRankedNext` (pins `tiller next`, the five tier names, `fourteen days`, and that the walk's `first match wins` is gone); replace `TestOrientRelaysTheContinuityNote` with `TestOrientRelaysTheRankedList` (pins "Relay the list as written" and "never rewrite the list"); keep `TestOrientScopesTheCannotDisagreeClaim` passing by rewording the claim to the list.

- [ ] **Step 2: Run** `go test ./internal/gen ./internal/skills -v`. Expected: FAIL.

- [ ] **Step 3: Implement** the text and bump `orientationSchemaGen` (ruling 10) with a history line: `ranked next replaces the route walk`.

- [ ] **Step 4: Run** `make test`. Expected: PASS. Run `grep -rn "—" internal/gen/orientation.go internal/skills/orient internal/skills/execute`: no hits in changed lines.

- [ ] **Step 5: Commit** `Skills: relay the ranked list and start the top agent move`

---

### Task 11: Build 1 specs, docs, verification

**Files:**
- Modify: `docs/spec/cli-surface.md` (`## The agent readout` :306-341, shared route :895-915 rewritten as ranked next, `tiller next` entry, command table :1140-1141, `check` narration :557), `docs/spec/skill-set.md` (anatomy :37, orient :108-155, execute readout :346), `docs/tiller-commands.md` (guide and readout at :154-168), `docs/plans/STATUS.md`, `docs/project-history.md` (append under `## October 9 readout continuity` that "Owner affinity, no flag" and "Route only" are overturned by ranked next, linking the design)

- [ ] **Step 1: Edit the docs** to match what shipped; record rulings 1 to 10 as built in the CLI spec's ranked next section.
- [ ] **Step 2: Run** `grep -n "—"` on each touched doc and `make test`.
- [ ] **Step 3: Run the `verify` skill** against a scratch project: two contributors, a draft and a specced feature, an open question due tomorrow; confirm `tiller next`, `tiller next --all --json`, bare `tiller guide`, `tiller approve feature` readout, `tiller submit` and `tiller check` output, and that the console Home still renders the route strip through the adapter.
- [ ] **Step 4: Commit** `Docs: describe ranked next`

---

## Build 2: Console

### Task 12: `/api/next` replaces `/api/route`

**Files:**
- Create: `internal/serve/next.go`, `internal/serve/next_test.go`
- Delete: `internal/serve/route.go`, `internal/serve/route_test.go`
- Modify: `internal/serve/serve.go:82`, `internal/serve/coalesce_test.go:249`

**Interfaces:**
- Produces: `func NextHandler(root string) http.HandlerFunc` serving `views.Next` with `Candidates` cut to `Shortlist()` (the same payload as `tiller next --json`), reader from `actingContributor(root)`. `/api/route` returns the catch-all 404.

- [ ] **Step 1: Write the failing tests**, porting the route tests' fixtures: `TestNextHandlerRanksForTheServedIdentity`, `TestNextHandlerViaIdentityFile`, `TestNextHandlerWithoutIdentityIsUnpersonalized`, `TestNextHandlerCarriesDependencyDiagnostics`, `TestNextHandlerContinuesReadersObservedTask`, `TestNextHandlerOmitsOthersObservedTask`, `TestNextHandlerRefusesWrites`, `TestRouteEndpointIsGone`, `TestNextHandlerMatchesTheCLIPayload` (decode both and compare).
- [ ] **Step 2: Run** `go test ./internal/serve -run 'Next|RouteEndpoint' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** and swap the mux registration.
- [ ] **Step 4: Run** `make test`. Expected: PASS.
- [ ] **Step 5: Commit** `Serve: replace /api/route with /api/next`

---

### Task 13: Home renders the ranked Up next

**Files:**
- Modify: `ui/src/api.ts` (delete `routePath`, `RouteNode`, `RouteAnswer`, `fetchRoute`; add `nextPath = "/api/next"`, `NextNode`, `NextCandidate`, `NextAnswer` mirroring the Go JSON), `ui/src/views/Home.tsx` (delete `RouteStrip` and the `route` fetch; fetch `nextPath`; `DependencyDiagnostics` reads `next.data?.dependency_diagnostics`; Recovered reviews from `recoveredOwners` only), `ui/src/views/home/Pulse.tsx` (`UpNextColumn`)
- Delete: `ui/src/views/home/routeLead.ts`, `ui/src/views/home/routeLead.test.ts`
- Test: `ui/src/api.test.ts`, `ui/src/components/__tests__/home.test.tsx` (`baseRoutes` uses `"/api/next": emptyNext()`; delete the "Home: route strip" block and the strip's recovered link test; rewrite "Home pulse: Up next")

**Interfaces:**
- Consumes: `/api/next`.
- Produces: `UpNextColumn({ state: FetchState<NextAnswer> })`. Header `Up next · for <reader>`, or `Up next` plus a muted line `Not personalized; run `tiller join`.` when `reader` is empty. Each row: actor label `You` or `Agent`, the action, a `RefLink` to the node, and the reason through `ProseInline`; no command. Caught up shows `Caught up: nothing is legal to do right now.` and the hint. No "show all" (decision 11).

- [ ] **Step 1: Write the failing tests** in `home.test.tsx`: `renders the ranked candidates with actor labels`, `names the reader in the header`, `says not personalized without a reader`, `shows caught up with its hint`, `renders dependency diagnostics from /api/next`, `has no route strip`, `Recovered reviews tab counts only list records`, and in the horizon guard, that Home fetches `/api/next` and never `/api/route`. Delete the due-question rows test (deadline questions now arrive ranked).
- [ ] **Step 2: Run** `cd ui && npm test`. Expected: FAIL.
- [ ] **Step 3: Implement.** In flight stays as is.
- [ ] **Step 4: Run** `cd ui && npm test && npm run build`, then `make ui`. Expected: PASS; `ui/dist` rebuilt.
- [ ] **Step 5: Commit** `Console: rank Home's Up next and drop the route strip` (include `ui/dist`).

---

### Task 14: Build 2 specs and verification

**Files:**
- Modify: `docs/spec/ui-surface.md` (`### Home` :82-105: delete the strip bullet :93, diagnostics via `/api/next` :94, In flight's "same candidates" :99, Up next :100 rewritten for decisions 5 and 7; `/api/next` in `## How it maps to the CLI` :369), `docs/plans/STATUS.md`, `docs/project-history.md` (one ranked next entry for both builds; the design and this plan are retired from `docs/plans/` per the repo convention once JC lands it)

- [ ] **Step 1: Edit the docs.**
- [ ] **Step 2: Run the `verify` skill:** serve the scratch project from Task 11; check Home in both themes with and without an identity, the reader header, actor labels, diagnostics, the Recovered reviews tab, and that `/api/route` 404s.
- [ ] **Step 3: Run** `make test && make lint && (cd ui && npm test)`.
- [ ] **Step 4: Commit** `Docs: describe Home's ranked Up next`
