# Bug severity and Fix board sorting: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bugs carry an optional `severity` (required at approval), a new `tiller grade` verb changes it, and the Fix board sorts by shelf order, severity, or date added and filters by severity.

**Architecture:** One optional `Severity` field on the shared `store.WorkItem`, validated bug-only at parse and at every write. The approve transition gains a `Severity *string` input, the way it already carries `ReviewGrain`. A new `mutate.Grade` follows `RenameTitle`'s single-file-write shape. Views expose `severity` on `ListItem`, `NodeView`, and search `NodeResult`, plus an event-derived `added_at` on Fix cards. The console does all sorting and filtering client-side in `views/workitems/board.ts`.

**Tech Stack:** Go (cobra CLI, yaml.v3, testify), React + TypeScript (Vitest), embedded skills Markdown.

**Spec:** `docs/plans/2026-11-01-bug-severity-design.md`. Executors read both. Decision numbers below (D1 to D14) refer to it.

## Global Constraints

- Severity values, stored lowercase, in this order: `blocker`, `important`, `medium`, `minor`. Display labels: `Blocker`, `Important`, `Medium`, `Minor`, and `Unrated` for none.
- Severity is bug only. A chore or tweak carrying one is a parse error; any write that would set one is refused with `severity applies to bugs`.
- Routing, `tiller next`, the readout's ordering, and the approve queue's ordering never read severity (D1).
- No code path writes a severity nobody chose; the migration only bumps the stamp (D5, D13).
- Schema 12 to 13; orientation generation 58 to 59; skill count stays eleven.
- Severity in the UI is always text; any tint is decoration (D11).
- Doc and copy style: no em dashes.
- Build in a worktree; commit with `-s`; JC fast-forwards main.

## Review Focus

1. **Transfer drops the grade.** `transfer.go` builds the replacement `WorkItem` field by field; a ready graded bug handed to a new owner must keep its severity. Test in Task 3.
2. **Batch approve with one unrated bug.** `tiller approve bug a b` where only `a` is graded must cross `a` and refuse `b` with the `--severity` move, the batch's existing per-node behavior. Test in Task 4.
3. **Hand-edited `bug.yml` with `severity: High`.** It must fail to parse with a message naming the four values, not load as unrated. Test in Task 2.
4. **Closed column under a sort with more than `ROW_CAP` closed items.** Choosing Severity must not change *which* closed items show, only their order. Test in Task 7.
5. **A stored filter query from before this build** (no `sort`, no `severity`) must restore as Shelf order and All severities, and a hand-edited `?sort=bogus` or `?severity=high` must fold to the defaults. Test in Task 7.

---

### Task 1: Schema 13 with a no-op migration

**Files:**
- Modify: `internal/store/manifest.go:70` (`CurrentSchemaVersion = 13`)
- Modify: `internal/core/mutate/migrate.go:131-135` (append the `from: 12` entry)
- Modify: every test fixture carrying `schema_version: 12` (`grep -rl "schema_version: 12" internal`)
- Test: `internal/core/mutate/migrate_test.go`

**Interfaces:**
- Produces: schema 13 trees; `migrate` from 12 stamps 13 and changes nothing else.

- [ ] **Step 1: Write the failing test** `TestMigrate12To13StampsOnly` in `migrate_test.go`: lay a schema-12 tree holding one bug, run `mutate.Migrate`, assert `tiller.yml` reads `schema_version: 13`, the bug's `bug.yml` bytes are unchanged, and the inspection notes include `"stamp schema_version 13; bug severity is additive; preserve work, events, and active reviews without drain"`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate -run TestMigrate12To13StampsOnly`. Expected: FAIL (no migration from 12 / version mismatch).
- [ ] **Step 3: Implement.** Bump the constant; append `{from: 12, note: <the note above>, apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 13) }}`; then `grep -rl "schema_version: 12" internal | xargs sed -i '' 's/schema_version: 12/schema_version: 13/'`. Leave fixtures that deliberately exercise older stamps (migration tests asserting a from-version) alone: review each `migrate*_test.go` hit by hand.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Schema: stamp 13 for bug severity`.

### Task 2: The `severity` field and its parse validation

**Files:**
- Create: `internal/store/severity.go`
- Modify: `internal/store/workitem.go:18-47` (field), `:248` (parse check)
- Test: `internal/store/severity_test.go`

**Interfaces:**
- Produces:
  - `const SeverityBlocker, SeverityImportant, SeverityMedium, SeverityMinor = "blocker", "important", "medium", "minor"`
  - `func Severities() []string` (most severe first)
  - `func ValidSeverity(s string) bool`
  - `func SeverityRank(s string) int` (blocker 0 … minor 3, `""` and anything else 4)
  - `WorkItem.Severity string \`yaml:"severity,omitempty"\``, placed after `Title`.

- [ ] **Step 1: Write the failing tests.** `TestValidSeverity` (four values true; `""`, `High`, `critical` false). `TestSeverityRank` (order above). `TestLoadRefusesSeverityOnChore`: a chore.yml with `severity: minor` fails `store.Load` with an error containing `severity applies to bugs`. `TestLoadRefusesUnknownSeverity`: a bug.yml with `severity: High` fails with an error containing `blocker, important, medium, or minor`. `TestLoadReadsBugSeverity`: `severity: important` loads as `"important"`.
- [ ] **Step 2: Run** `go test ./internal/store -run 'Severity'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** in `severity.go`, and in the shelf loader next to the resolution check: refuse a non-bug noun with any severity (`parse %s: severity applies to bugs`), and a bug with an invalid one (`parse %s: severity %q is not blocker, important, medium, or minor`).
- [ ] **Step 4: Run** `go test ./internal/store`. Expected: PASS.
- [ ] **Step 5: Commit** `Store: add bug severity`.

### Task 3: `add bug --severity`, and transfer keeps it

**Files:**
- Modify: `internal/core/mutate/add_workitem.go` (`WorkItemInput.Severity`, validation, record, event `after`)
- Modify: `internal/core/mutate/transfer.go:216` (copy `Severity: w.Severity`)
- Modify: `internal/core/mutate/workitem.go` or wherever `Result` lives (add `Severity string`)
- Modify: `internal/cli/add.go:206-256,286-300` (`--severity` flag; `nodeResult.Severity` with `json:"severity,omitempty"`)
- Test: `internal/core/mutate/add_workitem_test.go` (create if absent), `internal/core/mutate/transfer_test.go`, `internal/cli/add_test.go`

**Interfaces:**
- Consumes: `store.ValidSeverity`.
- Produces: `WorkItemInput.Severity string`; `mutate.Result.Severity string`; the shared refusal helper `func severityUsage(noun, value string) error` in `internal/core/mutate/severity.go` returning `usageError` reason `severity_not_bug` (`severity applies to bugs`) or `invalid_severity` (`severity %q is not blocker, important, medium, or minor`), nil when valid. Tasks 4 and 5 call it.

- [ ] **Step 1: Write the failing tests.** `TestAddBugRecordsSeverity`: add with `Severity: "blocker"` → `bug.yml` has `severity: blocker`, `add.bug` event `After["severity"] == "blocker"`. `TestAddBugWithoutSeverityIsUnrated`: no `severity` key in the file, none in the event. `TestAddChoreRefusesSeverity` (reason `severity_not_bug`, nothing written). `TestAddBugRefusesInvalidSeverity` (`High`, reason `invalid_severity`). `TestTransferKeepsSeverity`: a ready, claimed bug with `severity: important` transferred → replacement has `important`. CLI: `TestAddBugSeverityFlag` asserts the JSON payload's `severity`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run 'Severity'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Validate in `planAddWorkItem` before minting. A re-add with a different severity stays the idempotent no-op (it reports the stored value; `grade` is the change verb). Add the flag only on the bug command: `newAddWorkItemCmd` registers `--severity` when `noun == store.NounBug`, so `add chore --severity` is cobra's unknown-flag error.
- [ ] **Step 4: Run** `go test ./internal/core/mutate ./internal/cli`. Expected: PASS.
- [ ] **Step 5: Commit** `Add: grade a bug at add time`.

### Task 4: Severity required at `approve bug`

**Files:**
- Modify: `internal/core/mutate/transition.go:76-110` (`TransitionInput.Severity *string`), `transitionEvent` (~`:960`)
- Modify: `internal/core/mutate/transition_workitem.go:104` (gate + regrade)
- Modify: `internal/cli/lifecycle_cmds.go:337-470` (`--severity` flag)
- Modify: every bug approval in tests that now needs a grade (`grep -rln "approve bug\|NounBug.*Verb: \"approve\"" internal`; also `layBug` in `transition_workitem_test.go:28` gains `Severity: "medium"`)
- Modify: `site/screenshots/acme.sh:174` (`--severity important`)
- Test: `internal/core/mutate/transition_workitem_test.go`, `internal/cli/lifecycle_cmds_test.go`

**Interfaces:**
- Consumes: `severityUsage` (Task 3).
- Produces: `TransitionInput.Severity *string` (approve only); `*SeverityRequiredError{NodeID, Key string}` with reason `severity_required` and message `bug <ref> has no severity; approve it with tiller approve bug <key> --severity <blocker|important|medium|minor>`; event `Before["severity"]`/`After["severity"]` whenever approve set or changed it.

- [ ] **Step 1: Write the failing tests.** `TestApproveUnratedBugRefuses` (reason `severity_required`, status stays proposed, no event). `TestApproveBugWithSeverityGrades` (ready, `severity: minor`, event before `""` after `minor`). `TestApproveRatedBugNeedsNoFlag`. `TestApproveBugRegrades` (rated `minor`, approve with `blocker` → `blocker`, event before `minor`). `TestApproveChoreRefusesSeverity` (`severity_not_bug`). CLI: `TestApproveSeverityRefusesWithRefreshBasis` (`bad_args`), `TestApproveBatchCrossesGradedRefusesUnrated` (Review Focus 2), `TestApproveSeverityAppliesToEveryNamedBug` (`approve bug a b --severity medium` grades both).
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run 'Approve.*(Severity|Rated|Unrated|Regrade)'`. Expected: FAIL.
- [ ] **Step 3: Implement.** In `transitionWorkItem`'s `approve && before == proposed` arm, before `recordWorkBasis`: if `in.Severity != nil`, validate and set `w.Severity`; then if `noun == bug && w.Severity == ""`, return `SeverityRequiredError`. A non-nil `Severity` on any non-approve verb, on `--refresh-basis`, or on a tree noun is refused (`bad_args` at the CLI; `severityUsage` in mutate). In `transitionEvent`, add the before/after pair only when `in.Verb == "approve" && in.Severity != nil`; thread the old value the way `reviewGrainBefore` is threaded. Register `--severity` on `approve`; refuse it for any noun but `bug`.
- [ ] **Step 4: Fix every test that approves an unrated bug** by grading it at add (`Severity: "medium"`) or passing `--severity medium`. Do not weaken the gate.
- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 6: Commit** `Approve: require a severity to triage a bug`.

### Task 5: The `grade` verb

**Files:**
- Create: `internal/core/mutate/grade.go`, `internal/cli/grade.go`
- Modify: `internal/cli/root.go` (register), `docs/tiller-commands.md` (next to `rename` at `:426` and the shelf block at `:788`)
- Regenerate: `site/src/content/docs/cli.md` (generated from `docs/tiller-commands.md`; use the existing generator, discard CRLF-only churn in notices)
- Test: `internal/core/mutate/grade_test.go`, `internal/cli/grade_test.go`

**Interfaces:**
- Consumes: `severityUsage`, `resolve.NewResolver`, `loadWorkItemFile`, `commitMutation` (as `RenameTitle` uses them, `rename.go:64-200`).
- Produces: `type GradeInput struct{ Noun, Node, Severity string }`; `type GradeResult struct{ ID, Key, OldSeverity, Severity string; Changed bool }`; `func Grade(tree *store.Tree, deps Deps, in GradeInput) (GradeResult, error)`; event type `grade.bug` with `Before{"severity"}`, `After{"severity"}`, summary `grade bug <id> <level>`.

- [ ] **Step 1: Write the failing tests.** `TestGradeSetsSeverityAndRecordsEvent` (proposed bug, `important`). `TestGradeSameLevelIsNoop` (`Changed` false, no event, no write). `TestGradeRefusesTerminalBug` for done and cancelled (reason `terminal_grade`, message names the status). `TestGradeRefusesInvalidLevel`. `TestGradeAllowsInReview`. CLI: `TestGradeChoreIsUnknownNoun` (`tiller grade chore x minor` refuses as bad args naming `bug`), `TestGradeJSONPayload` (`old_severity`, `severity`, `changed`), and the human line `Bug <ref>: severity minor → important.`. Without `--contributor`, the acting identity resolves as for `rename`: any contributor may grade.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run Grade`. Expected: FAIL.
- [ ] **Step 3: Implement** `Grade` and the cobra command `grade <bug> <node> <level>` (`Args: cobra.ExactArgs(3)`, `Short: "Set a bug's severity (blocker, important, medium, minor)."`, Long says any contributor may run it and that severity never changes routing). Add the commands-doc entry: `tiller grade bug <node> <blocker|important|medium|minor>   # any contributor: set or change a non-terminal bug's severity; never routing`.
- [ ] **Step 4: Run** `go test ./...` and the site doc generator's check. Expected: PASS.
- [ ] **Step 5: Commit** `Grade: add the bug severity verb`.

### Task 6: Read surfaces (views, queue, search)

**Files:**
- Modify: `internal/views/list.go:25-60,259-263` (`ListItem.Severity`), `internal/views/node.go:30-60,430` (`NodeView.Severity`)
- Modify: `internal/views/workitems.go` (`WorkItemCard.AddedAt`, `AddedEvent`; `addStamps`)
- Modify: `internal/views/workitem_queue.go:25` (command for unrated bugs)
- Modify: `internal/search/search.go:86-98`, `internal/search/rank.go:52` (`NodeResult.Severity`)
- Modify: `internal/cli/show.go:175-183` (text line `  severity: <level>` for a bug, `  severity: unrated` when none)
- Modify: `ui/src/api.ts` (`ListRow.severity?`, `NodeView.severity?`, `WorkItemCard.added_at?`, `added_event?`, `SearchNodeResult.severity?`, and `type Severity = "blocker" | "important" | "medium" | "minor"`)
- Test: `internal/views/workitems_test.go`, `internal/views/workitem_queue_test.go`, `internal/search/search_test.go`, `internal/serve/workitems_test.go`

**Interfaces:**
- Consumes: `store.WorkItem.Severity`.
- Produces (JSON): `severity` (omitempty) on list rows, node views, search node results; `added_at` (RFC3339 via `formatTime`) and `added_event` (EV id) on Fix cards; `func addStamps(evs []events.Event) map[string]terminalStamp` beside `terminalStamps`.

- [ ] **Step 1: Write the failing tests.** `TestWorkItemCardCarriesSeverity`. `TestWorkItemCardAddedAtFromAddEvent`. `TestWorkItemCardAddedAtFollowsTransfer`: the replacement's `added_at` equals the original's `add.bug` time, through `transfer.bug` `After["replacement_id"]` → `After["id"]`. `TestWorkItemCardNoAddEventOmitsAddedAt`. `TestApproveQueueNamesSeverityForUnratedBug`: command `tiller approve bug <addr> --severity <blocker|important|medium|minor>`; rated bug and every chore/tweak keep the plain command. `TestSearchNodeResultCarriesSeverity`. `TestShowBugTextNamesSeverity` in `internal/cli/show_test.go` (rated and unrated; no line for a chore).
- [ ] **Step 2: Run** `go test ./internal/views ./internal/search ./internal/serve -run 'Severity|AddedAt'`. Expected: FAIL.
- [ ] **Step 3: Implement.** `addStamps` reads `add.<shelf noun>` events (target id → stamp) and then `transfer.<shelf noun>` events (copy the original's stamp to `replacement_id`); sort with `events.Sort` first so a transfer always sees its original's add. Set `AddedAt`/`AddedEvent` in `WorkItemsBoardOf`. Do not reorder anything (D1).
- [ ] **Step 4: Run** `go test ./...` and `cd ui && npm run typecheck`. Expected: PASS.
- [ ] **Step 5: Commit** `Views: carry severity and date added`.

### Task 7: Fix board sort and severity filter logic

**Files:**
- Modify: `ui/src/views/workitems/board.ts`
- Modify: `ui/src/views/workitems/filterStorage.ts` (`FILTER_AXES`, `filterQuery`)
- Test: `ui/src/views/workitems/board.test.ts`, `ui/src/views/workitems/filterStorage.test.ts`

**Interfaces:**
- Consumes: `WorkItemCard.severity`, `added_at`, `added_event` (Task 6).
- Produces:
  - `export type FixSort = "shelf" | "severity" | "added"`
  - `export const UNRATED = "unrated"`
  - `WorkItemFilters` gains `sort: FixSort` and `severity: string[]` (members of the four levels plus `UNRATED`; empty means All)
  - `setWorkItemFilter` key union gains `"sort" | "severity"`; `severity` toggles a member like `noun`; `sort` sets `severity`/`added` or deletes for `shelf`
  - `export function sortWorkItems(items: WorkItemCard[], sort: FixSort): WorkItemCard[]` (pure, stable)

- [ ] **Step 1: Write the failing tests** in `board.test.ts`:
  - `readWorkItemFilters` defaults: `sort` `"shelf"`, `severity` `[]`; `?sort=bogus` → `"shelf"`; `?severity=high,blocker` → `["blocker"]`; all five members selected folds to `[]`.
  - Severity filter: `?severity=blocker` keeps blocker bugs only and drops every chore and tweak; `?severity=unrated` keeps unrated bugs only, never chores or tweaks; `noun=chore&severity=blocker` yields empty columns.
  - `sortWorkItems(…, "severity")`: blocker, important, medium, minor, then unrated bugs and non-bugs; equal ranks keep input order (shelf order; bug → chore → tweak grouping).
  - `sortWorkItems(…, "added")`: newest `added_at` first; equal `added_at` → higher `added_event` first; missing `added_at` last.
  - Closed under a sort (Review Focus 4): with `ROW_CAP + 3` closed cards in server order, `severity` sort returns the same set as `shelf` sort's first `ROW_CAP`, reordered; with `showAllTerminal`, all of them sorted.
  - Under `shelf`, every column keeps server order exactly.

  In `filterStorage.test.ts`: `filterQuery` writes `sort=severity` and `severity=blocker,important` and omits defaults; a stored legacy value `noun=bug` restores with `sort` `shelf` and no severity (Review Focus 5); `hasFilterParams` is true for a URL naming only `sort`.
- [ ] **Step 2: Run** `cd ui && npx vitest run src/views/workitems`. Expected: FAIL.
- [ ] **Step 3: Implement.** `workItemMatchesFilters` ANDs the severity axis: empty passes; otherwise the item must be a bug whose `severity ?? UNRATED` is selected. `workItemColumns` filters, truncates closed to `ROW_CAP` in server order (unless `showAllTerminal`), then applies `sortWorkItems` to every column.
- [ ] **Step 4: Run** `cd ui && npx vitest run && npm run typecheck`. Expected: PASS.
- [ ] **Step 5: Commit** `Fix board: sort and severity filter logic`.

### Task 8: Fix board, node header, and search rendering

**Files:**
- Create: `ui/src/components/SeverityChip.tsx` (+ test `ui/src/components/__tests__/SeverityChip.test.tsx`)
- Modify: `ui/src/views/WorkItems.tsx` (Sort group, Severity group, chip, Added line), `ui/src/views/workitems.css`
- Modify: `ui/src/components/NodeHeader.tsx:274-330` (fact), `ui/src/views/explore/SearchResults.tsx` (chip on bug hits)
- Test: `ui/src/components/__tests__/search.test.tsx`, `ui/src/components/__tests__/workitems.test.tsx`

**Interfaces:**
- Consumes: Task 7's filters and `setWorkItemFilter`; Task 6's fields.
- Produces: `export function SeverityChip({ severity }: { severity?: Severity }): JSX.Element` rendering `<span className="severity-chip" data-severity={severity ?? "unrated"}>{label}</span>`; `export function formatAdded(iso: string, now: Date): string` in `ui/src/lib/added.ts` (`"Added Oct 12"`, or `"Added Oct 12, 2025"` when the year differs from `now`'s, local time, `toLocaleDateString(undefined, …)` like `History.tsx:689`).

- [ ] **Step 1: Write the failing tests.** `SeverityChip` renders each label and `Unrated` for undefined. `formatAdded` same-year and other-year cases. The Fix page: a bug card shows its chip text; a chore card shows none; every card with `added_at` shows the Added line; the Filters panel has a "Sort" group (Shelf order, Severity, Date added; Shelf order active by default) and a "Severity" group (All, Blocker, Important, Medium, Minor, Unrated), each a `FilterLink` carrying `FILTER_NAVIGATION` state. The node header lists `Severity` for a bug (`Unrated` when missing) and not for a chore. A bug search hit shows the chip.
- [ ] **Step 2: Run** `cd ui && npx vitest run`. Expected: FAIL.
- [ ] **Step 3: Implement.** Widen `FilterLink`'s `axis` union to include `"sort" | "severity"`. Place Sort first in the panel, then Noun, Severity, Attached to. Put the chip in `work-item-card-chips` ahead of the activity chip (render that row when the card is a bug). Style `.severity-chip` with the existing chip tokens; `unrated` muted; blocker/important may take the danger/warn tints, but the label always shows.
- [ ] **Step 4: Run** `cd ui && npx vitest run && npm run typecheck && npm run build`. Expected: PASS.
- [ ] **Step 5: Commit** `Fix board: render severity, date added, and the sort and severity groups`.

### Task 9: Skills, orientation generation 59, and specs

**Files:**
- Modify: `internal/skills/capture/SKILL.md` (gains the severity table and the `--severity` rule), `internal/skills/code-review/SKILL.md:58-67,130` (pointer, disposition, line 67), `internal/skills/orient/SKILL.md` (`grade` beside `rename`/`link`; approve needs a severity)
- Modify: `internal/cli/guide.go` (the capture/approve guidance text, if it names approve bug)
- Modify: `internal/gen/orientation.go:45` (`orientationSchemaGen = 59`, plus the comment line `// Generation 59 records bug severity: capture grades, approve bug requires one, and grade changes it; severity never routes.`) and the orientation body if it describes bug approval
- Modify: `docs/spec/artifact-schema.md`, `docs/spec/cli-surface.md`, `docs/spec/ui-surface.md`, `docs/spec/skill-set.md`, `docs/spec/distribution.md` (schema 13, generation 58→59 everywhere `grep -rn "generation 58\|schema 12" docs site/src` finds them)
- Test: `internal/skills/skills_test.go`, `internal/gen` orientation tests, `internal/cli/guide_test.go`

- [ ] **Step 1: Update tests first.** In `skills_test.go`, pin: capture contains `--severity` and the four-row table; code-review no longer contains `Severity is review prose only` and contains `Severity becomes the bug's \`severity\` field when captured; it never becomes a queue position.`; the code-review disposition no longer keeps severity in `bug.md`'s opening line (confidence only). Update the generation pins to 59.
- [ ] **Step 2: Run** `go test ./internal/skills ./internal/gen ./internal/cli`. Expected: FAIL.
- [ ] **Step 3: Edit the skills and specs** per D7. Capture's unrated rule: when impact can't be known yet, leave severity off and say so in `bug.md`'s Actual section. `execute` is untouched (D7).
- [ ] **Step 4: Run** `go test ./...` and the site build. Expected: PASS.
- [ ] **Step 5: Commit** `Skills: grade bugs at capture; generation 59`.

### Task 10: Verify end to end

- [ ] **Step 1: Run the `/verify` skill** against a scratch project: `add bug x --severity minor`; `add bug y`; `approve bug y` refuses naming `--severity`; `approve bug y --severity blocker`; `grade bug x important`; `grade chore` refuses; `/fix` sorts by severity (y before x), by date added (y before x), filters to Unrated (empty) and Blocker (y only); node page shows Severity; the search hit shows the chip.
- [ ] **Step 2: Migrate a copy of a schema-12 tree** (`tiller upgrade --project-only`) and confirm only `tiller.yml` changes.
- [ ] **Step 3: Request the whole-branch review** (superpowers:requesting-code-review) against the design doc; fix findings in their own commits.
