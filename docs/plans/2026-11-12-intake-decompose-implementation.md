# One door for documents: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `tiller intake` becomes the one way a document enters a project after `init`: it lands a file or folder in `.tiller/design/` and opens a draft amendment whose new work is real draft nodes the amendment holds, handed over with `tiller propose amendment` and approved once.

**Architecture:** An amendment gains a `draft` status ahead of `proposed`, and its add operations record the id of the node they created at staging. The tree loader derives one index of held nodes from open amendments; a single predicate in `internal/gen` reads it, and every place a draft can act (lifecycle verbs, rollup reopen, approval queues, the specify frontier) consults that predicate. Approval promotes held nodes in place where it used to create them; withdraw reuses the existing removal planner. The `intake` skill folds into `decompose`.

**Tech Stack:** Go (cobra CLI, yaml.v3, testify), React + TypeScript (Vitest), embedded skills Markdown.

**Spec:** `docs/plans/2026-11-12-intake-decompose-design.md`. Executors read both. Decision numbers below (D1 to D13) refer to it.

## Global Constraints

- Schema 13 to 14, stamp-only. Orientation generation 59 to 60. Skill count eleven to ten.
- Amendment statuses, in lifecycle order: `draft`, `proposed`, then `approved` or `withdrawn`. "Open" means `draft` or `proposed`.
- The new verb is `tiller propose amendment <addr>`; its event type is `amendment_proposed`.
- A file lands at `.tiller/design/<basename>`; a folder lands at `.tiller/design/<folder name>/` with its relative paths kept (D3, D11). Nothing new is written under `.tiller/feedback/`, and nothing there is moved.
- Refusal reasons, exact: `amendment_required`, `amendment_not_proposed`, `amendment_empty`, `held_by_amendment`, `parent_held_elsewhere`, `held_node_missing`. Existing reasons keep their names (`artifact_exists`, `invalid_source`, `amendment_not_open`, `key_exists`).
- The existing type `gen.Held` and wire field `held` mean a retired ancestor. The new concept is `AmendmentHold` in code and `amendment_hold` on the wire; never reuse the bare name `Held` or `held` for it.
- A held node is refused by every lifecycle verb and is skipped by rollup reopen, the approval queues, and the specify frontier. It is shown, marked, on every read surface (D7).
- An amendment's operations list stays append-only. Removing a held draft marks its operation `removed`; it never deletes the entry, so operation indexes stay stable.
- Amendments staged before schema 14 (add operations with no `node`) keep today's approval behavior (D12).
- Any marker in the UI carries a text label; color is decoration.
- Doc and copy style: no em dashes.
- A change under `ui/src` ships with a rebuilt `ui/dist` from `make ui` in the same commit series.
- Build in a worktree; commit with `-s`; JC fast-forwards main.

## Review Focus

1. **A held draft under finished work.** A held draft feature under a done epic, or a held proposed task under a done feature, must not reopen it on `tiller sync`. Test in Task 8.
2. **Approval interrupted and run again.** If approval dies after promoting some nodes, the amendment is still `proposed`; a second `approve amendment` must finish, treating an already-ready epic or task as done, not as an error. Test in Task 11.
3. **A held draft renamed before approval.** Promotion and the hold must follow the node id on the operation, never the staged key. Test in Task 9 and Task 11.
4. **A file added to a landed folder.** A folder source must read stale when a file is added under it, not only when one changes or disappears. Test in Task 4.
5. **A hand-edited or merged amendment naming a node that no longer exists.** The tree must still load, the hold index must skip the entry, and `show amendment` must report it; `propose` and `approve` refuse with `held_node_missing`. Tests in Task 7, Task 11, and Task 12.

---

## Part 1: Landing and routing

### Task 1: Schema 14 with a stamp-only migration

**Files:**
- Modify: `internal/store/manifest.go:70` (`CurrentSchemaVersion = 14`)
- Modify: `internal/core/mutate/migrate.go` (append the `from: 13` entry after the `from: 12` one)
- Modify: every test fixture carrying `schema_version: 13` (`grep -rl "schema_version: 13" internal`)
- Test: `internal/core/mutate/migrate_test.go`

**Interfaces:**
- Produces: schema 14 trees; `migrate` from 13 stamps 14 and changes nothing else.

- [ ] **Step 1: Write the failing test** `TestMigrate13To14StampsOnly`: lay a schema-13 tree holding one proposed amendment with one `add-feature` operation, run `mutate.Migrate`, assert `tiller.yml` reads `schema_version: 14`, the amendment's yml bytes are unchanged, and the inspection notes include `"stamp schema_version 14; amendment draft status, held-draft node ids, and folder sources are additive; preserve work, events, and active reviews without drain"`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate -run TestMigrate13To14StampsOnly`. Expected: FAIL.
- [ ] **Step 3: Implement.** Bump the constant; append `{from: 13, note: <the note above>, apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 14) }}`; then rewrite the fixtures with `sed`. Leave fixtures that assert a from-version alone: review each `migrate*_test.go` hit by hand.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Schema: stamp 14 for held drafts and draft amendments`.

### Task 2: The `draft` amendment status

**Files:**
- Modify: `internal/store/amendment.go:20-24` (constant), `:205-220` (`ValidateAmendment`), new helper
- Modify: `internal/core/mutate/amendment.go:167,187,212` (open as draft), `:275,426,930,1005` (open checks), `:350` (`amendmentNotOpenErr` wording)
- Modify: `internal/core/mutate/amendment_approve.go:264`, `internal/core/mutate/amendment_move.go:331`, `internal/core/mutate/remove.go:595`, `internal/core/resolve/resolve.go:195`
- Modify: `internal/views/facets.go:54`, `internal/views/amendment_relations.go:43`, `internal/relationships/amendments.go:75,205`
- Test: `internal/store/amendment_test.go`, `internal/core/mutate/amendment_test.go`, `internal/core/mutate/amendment_approve_test.go`

**Interfaces:**
- Produces:
  - `const store.AmendmentStatusDraft = "draft"`
  - `func store.AmendmentOpen(a *Amendment) bool` (true for `draft` and `proposed`)
  - `mutate.AddAmendment` returns `Status: "draft"`.
  - `mutate.ApproveAmendment` on a draft returns a `usageError` with reason `amendment_not_proposed` and message `amendment <id> is draft; hand it over first: tiller propose amendment <key>`.

- [ ] **Step 1: Write the failing tests.**
  - `TestValidateAmendmentAcceptsDraft` (store): status `draft` validates; `drafting` is refused naming the four statuses.
  - `TestAddAmendmentOpensAsDraft` (mutate): result and on-disk status are `draft`; the `amendment_opened` event's `after.status` is `draft`.
  - `TestStageAndWithdrawWorkOnDraftAndProposed`: `StageOperation` and `WithdrawAmendment` succeed in both open states (use `setAmendmentStatus`), and refuse `amendment_not_open` on `approved`.
  - `TestOpenKeyCollisionCoversDraft`: a second `AddAmendment` with a key a draft amendment holds is refused.
  - `TestApproveRefusesDraft`: reason `amendment_not_proposed`, no writes.
- [ ] **Step 2: Run** `go test ./internal/store ./internal/core/mutate -run 'Draft|OpenKeyCollision'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Replace each "is proposed" test that means "is open" with `store.AmendmentOpen` (staging, amends-design, withdraw, key collision, the resolver's open preference, the move conflict scan, removal's reference scan, the two relationship sorts). Keep `== AmendmentStatusProposed` only in `ApproveAmendment`. Add `draft` first in the amendment facet list. Update the existing tests that asserted `proposed` after `AddAmendment`; where a test needs a proposed amendment, call `setAmendmentStatus(t, root, id, store.AmendmentStatusProposed)` until Task 3 supplies the verb.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Amendments open as draft`.

### Task 3: `tiller propose amendment`, and approve refuses an empty one

**Files:**
- Create: `internal/core/mutate/amendment_propose.go`
- Modify: `internal/store/amendment.go` (`AmendmentEmpty`), `internal/core/mutate/amendment_approve.go` (empty refusal, before `resolveAmendment`)
- Modify: `internal/cli/amendment_cmds.go` (new `newProposeCmd`, next to `newWithdrawCmd`), `internal/cli/root.go:104` (register), `internal/usage/markers.go:39` (`"propose": Planning`)
- Test: `internal/core/mutate/amendment_propose_test.go`, `internal/cli/amendment_cmds_test.go`

**Interfaces:**
- Consumes: `store.AmendmentStatusDraft`, `store.AmendmentOpen` (Task 2).
- Produces:
  - `type mutate.ProposeResult struct { ID, Key, Status string; Empty bool; Warnings []string }`
  - `func mutate.ProposeAmendment(tree *store.Tree, deps Deps, amendment *store.Amendment) (ProposeResult, error)`
  - `func store.AmendmentEmpty(a *Amendment) bool` (no operation, or every operation carries `Removed`; `Removed` arrives in Task 7, so until then it is `len(a.Operations) == 0`)
  - `ApproveAmendment` on an empty proposed amendment returns reason `amendment_empty` and message `amendment <id> proposes nothing; drop it: tiller withdraw amendment <key> --reason "<why>"`.

- [ ] **Step 1: Write the failing tests.**
  - `TestProposeMovesDraftToProposed`: status becomes `proposed`, one `amendment_proposed` event with `after.status`, and `after.empty` false for an amendment with one staged cancel.
  - `TestProposeIsIdempotent`: a second call on a proposed amendment succeeds, writes nothing, emits no event.
  - `TestProposeRefusesStaleSource`: an intake amendment whose landed document changed is refused with the same reason `checkSource` gives at approval; status stays `draft`.
  - `TestProposeRefusesMissingCitation`: an intake amendment with a hand-edited sourceless operation is refused as `checkOpSources` refuses it.
  - `TestProposeWarnsOnStalePrecondition`: a stale `target_status` precondition proposes successfully and returns one warning naming the operation index.
  - `TestProposeAcceptsEmpty`: an amendment with no operations proposes with `Empty: true`.
  - `TestApproveRefusesEmpty`: reason `amendment_empty`, no writes.
  - CLI: `tiller propose amendment <key> --json` exits 0 with `status: proposed`; `tiller propose epic x` is refused `unknown_noun`; human output for an empty one reads `Proposed <ref>. It proposes nothing; withdraw it with: tiller withdraw amendment <key> --reason "<why>"`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run 'Propose|ApproveRefusesEmpty'`. Expected: FAIL.
- [ ] **Step 3: Implement.** `ProposeAmendment` loads the amendment as `WithdrawAmendment` does, refuses a terminal one with `amendmentNotOpenErr`, returns unchanged for `proposed`, runs `checkSource` and `checkOpSources`, collects stale preconditions from `EvaluateAmendmentPreconditions` as warnings, and commits one write with one event through `commitNodeWrites(tree, "propose.amendment", ...)`. The CLI mirrors `runWithdrawAmendment`.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add tiller propose amendment`.

### Task 4: Intake lands in `design/` and accepts a folder

**Files:**
- Modify: `internal/cli/intake.go` (`feedbackDir` constant at `:39` becomes the design folder; `statIntakeSource`; destination; fingerprint; help text)
- Modify: `internal/store/amendment.go` (`AmendmentSource`, `ValidateAmendment`)
- Modify: `internal/core/mutate/amendment_validity.go` (`evalSource`), `internal/core/mutate/amendment_approve.go:801` (`checkSource` messages)
- Test: `internal/cli/intake_test.go`, `internal/core/mutate/amendment_validity_test.go`

**Interfaces:**
- Produces:
  - `type store.AmendmentSourceFile struct { Path string \`yaml:"path"\`; Revision string \`yaml:"revision"\` }`
  - `store.AmendmentSource` gains `Files []AmendmentSourceFile \`yaml:"files,omitempty"\``. A file source carries `Revision` and no `Files`; a folder source carries `Files` (repo-relative paths, sorted) and no `Revision`. `ValidateAmendment` refuses a source with both or neither.
  - `evalSource` reports stale for a folder when any listed file changed or vanished, or when a regular file exists under `Path` that `Files` does not list. Its note names the first differing path and what happened (`changed`, `removed`, `added`).

- [ ] **Step 1: Write the failing tests.**
  - `TestIntakeLandsAFileInDesign`: `tiller intake <tmp>/Acme-review.md` lands `.tiller/design/Acme-review.md`; the envelope's `source.path` and `triggered_by` name that path; `status` is `draft`; nothing exists under `.tiller/feedback/`.
  - `TestIntakeLandsAFolderUnderItsName`: a folder `specs/` holding `a.md` and `sub/b.md` lands as `.tiller/design/specs/a.md` and `.tiller/design/specs/sub/b.md`; `source.path` is `.tiller/design/specs`; `source.files` lists both with `git:<40 hex>` revisions in sorted order; the default key is `specs` and the title `Intake: specs`.
  - `TestIntakeRefusesANameAlreadyInDesign`: both the file and the folder case are refused `artifact_exists` with nothing written.
  - `TestIntakeRefusesAnEmptyFolder`: reason `invalid_source`.
  - `TestFolderSourceStaleness` (mutate): valid when untouched; stale with a note naming the path when a listed file changes, is removed, or a new file is added under the folder (Review Focus 4).
  - Update `TestIntakeAcceptsAFileFromInsideTheFeedbackFolder`: a file under `.tiller/feedback/archive/` still intakes, now into `design/`.
- [ ] **Step 2: Run** `go test ./internal/cli ./internal/core/mutate -run 'Intake|FolderSource'`. Expected: FAIL.
- [ ] **Step 3: Implement.** A file calls `importDocs(root, src, ".tiller/design")`; a folder calls `importDocs(root, src, ".tiller/design/<base>")`. Check the destination name with `store.PathExistsNoSymlinks` before `PrecheckAddAmendment`, as the file case does today. Fingerprint each landed file with `mutate.WorktreeBlobRef`. Do not use `vcs.WorktreeSurface`: it refuses paths inside `.tiller/`. Rewrite the command's long help to say `.tiller/design/` and to describe the folder form.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Intake lands in design and accepts a folder`.

### Task 5: `tiller intake --no-amendment` for the founding design

**Files:**
- Modify: `internal/cli/intake.go`
- Create: `internal/core/mutate/intake_design.go`
- Test: `internal/cli/intake_test.go`

**Interfaces:**
- Consumes: the landing rules of Task 4.
- Produces:
  - `func mutate.RecordDesignIntake(tree *store.Tree, deps Deps, destRel string, files int) error`, which writes one event of type `design_intake` with `after.path` and `after.files` and no other file.
  - `--no-amendment` on a tree with any epic (held or not) is refused: reason `amendment_required`, message `this project already has epics; a later document comes in as an amendment: tiller intake <path>`.

- [ ] **Step 1: Write the failing tests.**
  - `TestIntakeNoAmendmentLandsAndOpensNothing`: on a greenfield repo with no epics, the folder lands under `.tiller/design/<name>/`, no file appears under `.tiller/amendments/`, one `design_intake` event exists, and `tiller guide` then names `decompose` with the existing "to-be design docs await decomposition" reason.
  - `TestIntakeNoAmendmentRefusedOnceAnEpicExists`: reason `amendment_required`, nothing written.
  - `TestIntakeNoAmendmentLeavesTheManifestAlone`: `tiller.yml` bytes are unchanged.
- [ ] **Step 2: Run** `go test ./internal/cli -run IntakeNoAmendment`. Expected: FAIL.
- [ ] **Step 3: Implement** the flag, the epic check (`len(tree.Epics) > 0`), and the event. `--key` and `--title` with `--no-amendment` are refused as misuse.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Intake --no-amendment lands a founding design`.

### Task 6: The router sees a draft amendment, and the queue waits for hand-over

**Files:**
- Modify: `internal/store/tobe_design.go` (skip amendment sources)
- Modify: `internal/views/queue.go:200` (`queueApproveAmendments`), `internal/views/next_enumerate.go` (new candidates, kind maps), `internal/views/zero_day.go:38`
- Modify: `internal/cli/guide.go:128-142` (`suggestFromNext`), `internal/cli/intake.go` (`renderIntakeHuman`), `internal/cli/readout.go:432`, `internal/views/readout.go:1658`
- Test: `internal/store/tobe_design_test.go`, `internal/views/queue_test.go`, `internal/views/next_enumerate_test.go`, `internal/cli/guide_suggest_test.go`, `internal/cli/e2e_intake_test.go`

**Interfaces:**
- Consumes: `store.AmendmentOpen`, `store.AmendmentEmpty`.
- Produces:
  - `loadToBeDesign` omits any path equal to, or beneath, the `Source.Path` of any amendment in `t.Amendments`, whatever its status.
  - Queue kind `approve_amendment` only for a `proposed`, non-empty amendment. New queue kind `withdraw_amendment` for a `proposed`, empty one, with `Command: tiller withdraw amendment <addr> --reason "<why>"`, in `armAmendment`, action `withdraw amendment`, reason `nothing was proposed from its document`.
  - New agent candidates in `armAmendment` for each `draft` amendment: kind `decompose_amendment`, skill `intake` (Task 14 renames it), when `Source != nil`; kind `stage_amendment`, skill `amend`, when it was opened by hand.
  - `suggestFromNext` returns, for `decompose_amendment`: `A document landed as amendment <ref> and has not been turned into work. Read it whole, stage what it implies, then run tiller propose amendment <key>.` For `stage_amendment`: `Amendment <ref> is still a draft. Finish staging it, then run tiller propose amendment <key>.`
  - The "no work planned" hint for brownfield and migration projects ends `bring work in through intake (any document that should change the plan) or capture (a bug, chore, or tweak)`. The greenfield hint ends `bring the founding design in with tiller intake --no-amendment <file or folder>`.

- [ ] **Step 1: Write the failing tests.**
  - `TestToBeDesignSkipsAmendmentSources`: on an epic-less tree, a document landed by intake (file and folder forms) is absent from `t.ToBeDesign`; an unrelated file in `design/` is present.
  - `TestQueueHidesDraftAmendments`, `TestQueueOffersWithdrawForAnEmptyProposedAmendment`, `TestQueueOffersApproveForAProposedAmendmentWithOperations`.
  - `TestNextOffersDecomposeForADraftIntakeAmendment` and `TestNextOffersStageForAHandOpenedDraft`: actor agent, the kinds and skills above, ranked in the amendment arm ahead of approvals.
  - `TestGuideAfterIntakeNamesTheAgentMove` (cli): after `tiller intake`, bare `tiller guide` prints the agent line and suggested skill above and does not print `approve amendment`.
  - Update the end-to-end test: intake, stage through `amend`, `propose`, then the guide shows the human approve line.
- [ ] **Step 2: Run** `go test ./internal/store ./internal/views ./internal/cli -run 'ToBeDesign|Queue|Next|Guide|EndToEndIntake'`. Expected: FAIL.
- [ ] **Step 3: Implement.** `renderIntakeHuman`'s Next block names `tiller guide` only. The readout's proposed-amendment helpers (`readoutAmendmentPreconditions`, `proposedAmendments`) keep reading `proposed` only; add draft amendments to the readout's agent-move section through the candidates above.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Route a draft amendment to the agent and hold the queue until propose`.

---

## Part 2: Held drafts

### Task 7: Staging an add creates the node, and the tree knows who holds it

**Files:**
- Modify: `internal/store/amendment.go` (`AmendmentOp`, `validateAmendmentOp`)
- Modify: `internal/store/tree.go` (index field, built after nodes and amendments load)
- Create: `internal/gen/amendment_hold.go`
- Modify: `internal/core/mutate/amendment.go:416` (`StageOperation`), `:493` (`buildStagedOp` add arms), `:824` (`resolveStageParentEpic`), and `amendment_propose.go` (the missing-node check)
- Modify: `internal/cli/amendment_cmds.go` (stage result view and human line)
- Test: `internal/core/mutate/amendment_test.go`, `internal/gen/amendment_hold_test.go`, `internal/store/tree_test.go`

**Interfaces:**
- Produces:
  - `store.AmendmentOp` gains `Node string \`yaml:"node,omitempty"\`` (legal only on `add-epic`, `add-feature`, `add-task`) and `Removed string \`yaml:"removed,omitempty"\`` (the removal reason; legal only on an add operation that carries `Node`).
  - `store.Tree.AmendmentHolds map[string]string`: node id to amendment id, built from every open amendment's add operations that carry `Node`, have no `Removed`, and name a node that exists in the tree. An entry naming a missing node is skipped, never an error.
  - `func gen.AmendmentHold(t *store.Tree, n store.Node) *store.Amendment` (nil when not held).
  - `StageOperation` for an add operation creates the node through `planAddEpic`, `planAddFeature`, or `planAddTask`, records its id in `op.Node`, and commits the node's two files, the amendment yml, the `add.<noun>` event (with `after.amendment` and `after.operation_index`, as approval writes them today), and the `amendment_staged` event in one `commitMutation`.
  - `StageResult.Op.Node` carries the id; the CLI prints `Staged add-epic <key> (<id>) as a draft held by <amendment ref>`.
  - Refusals at staging: `key_exists` when the key is already held by a live node (an amendment never re-adds); `parent_held_elsewhere` when the parent epic or feature is held by a different open amendment, message `<parent ref> is held by amendment <other ref>; stage this on that amendment`.

- [ ] **Step 1: Write the failing tests.**
  - `TestStageAddEpicCreatesAHeldDraft`: after staging, `epics/<key>/epic.yml` exists with status `draft`, the operation carries `node`, `tree.AmendmentHolds[id]` is the amendment, and `gen.AmendmentHold` returns it. One `add.epic` event and one `amendment_staged` event.
  - `TestStageAddFeatureUnderAHeldEpic` and `TestStageAddFeatureUnderAnApprovedEpic`: both create a held draft feature.
  - `TestStageAddTaskCreatesAHeldProposedTask`: under an approved feature, status `proposed`, held.
  - `TestStageAddRefusesAnExistingKey`: `key_exists`, nothing written.
  - `TestStageAddRefusesAParentHeldElsewhere`.
  - `TestHoldIndexSkipsAMissingNode`: hand-edit an operation's `node` to an id no node has; `store.Load` succeeds and the index has no entry for it (Review Focus 5).
  - `TestHoldEndsWhenTheAmendmentCloses`: set the amendment `withdrawn` on disk; the index is empty.
  - `TestProposeRefusesAMissingHeldNode`: with that same hand edit, `ProposeAmendment` refuses with reason `held_node_missing` naming the operation index; status stays `draft`.
  - `TestStageSourceCitationReachesTheNode`: an add operation's `--source` lands on the node's own `source`, as approval did.
- [ ] **Step 2: Run** `go test ./internal/store ./internal/gen ./internal/core/mutate -run 'StageAdd|HoldIndex|HoldEnds|StageSource'`. Expected: FAIL.
- [ ] **Step 3: Implement.** In `StageOperation`, plan the add after `buildStagedOp` validates the shape, then extend the single commit with the plan's writes and event. `resolveStageParentEpic` resolves against the live tree only; a parent that is a same-amendment staged key is now a live node.
- [ ] **Step 4: Run** `go test ./...`. Expected: existing approval tests that staged an add and expected no node until approval now FAIL; fix them in Task 11, and mark them with `t.Skip("held drafts: Task 11")` here so the suite is green between tasks.
- [ ] **Step 5: Commit** `Staging an add creates a draft the amendment holds`.

### Task 8: The hold

**Files:**
- Modify: `internal/core/mutate/transition.go:216` (`applyTransition`), error type beside `ParentEpicRetiredError`
- Modify: `internal/core/mutate/sync.go:1817,1833` (`workLandedUnderEpic`, `workLandedUnderFeature`)
- Modify: `internal/views/queue.go:218,286` (`queueApproveEpics`, `queueApproveTaskBatches`), `internal/views/readout.go:1432` (`frontierFeatures`)
- Modify: `internal/cli/lifecycle_cmds.go` (refusal data and human line, beside the `ParentEpicRetiredError` arm near `:1559`)
- Test: `internal/core/mutate/transition_test.go`, `internal/core/mutate/sync_test.go`, `internal/views/queue_test.go`, `internal/views/next_enumerate_test.go`, `internal/cli/lifecycle_cmds_test.go`

**Interfaces:**
- Consumes: `gen.AmendmentHold` (Task 7).
- Produces:
  - `type mutate.HeldByAmendmentError struct { Noun, NodeID, NodeKey, CurrentState, AmendmentID, AmendmentKey, Verb string }`, exit code 1, reason `held_by_amendment`.
  - CLI human line: `Blocked: <noun> <ref> is held by amendment <amendment ref>; it moves when the amendment is approved (tiller approve amendment <key>), or remove it from the proposal (tiller remove <noun> <key> --reason "<why>").` The JSON data carries `reason`, `amendment`, `amendment_key`, `current_state`, and `missing_seam: tiller approve amendment <key>`.

- [ ] **Step 1: Write the failing tests.**
  - `TestHeldNodeRefusesEveryLifecycleVerb`: table over (held epic, `approve`), (held feature under an approved epic, `spec`), (same feature, `approve`), (held task, `approve`), (held feature, `cancel`); each returns `HeldByAmendmentError`, writes nothing, emits no event.
  - `TestSyncDoesNotReopenADoneEpicForAHeldFeature` and `TestSyncDoesNotReopenADoneFeatureForAHeldTask` (Review Focus 1): the done status and `verified_against` are unchanged after `Sync`.
  - `TestQueueSkipsHeldEpicsAndHeldTasks`: neither appears as `approve_epic` or in an `approve_tasks` batch; an unheld proposed task on the same feature still does.
  - `TestFrontierSkipsAHeldFeature`: a held draft feature under a ready epic is not a `specify` candidate.
  - `TestDriftSweepsAHeldFeaturesSurfaces`: a held feature with a linked surface whose file is then deleted is reported under `missing_surface`. This pins existing behavior and may pass before any change.
  - CLI: the exit code is 1 and the envelope carries the fields above.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/views ./internal/cli -run 'Held|DoesNotReopen|Frontier'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Check `gen.AmendmentHold` at the top of `applyTransition`, before the noun switch, so every current and future lifecycle verb is covered. Add the same check as the first `continue` in each of the four read sites.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS apart from Task 7's skips.
- [ ] **Step 5: Commit** `A held draft takes no lifecycle step and reopens nothing`.

### Task 9: Corrections on a held draft

**Files:**
- Modify: `internal/core/mutate/remove.go:82` (`RemoveDraft`), `:562` (`planRemovalAmendments`)
- Test: `internal/core/mutate/remove_test.go`, `internal/core/mutate/rename_test.go`, `internal/core/mutate/depends_test.go`

**Interfaces:**
- Consumes: `AmendmentOp.Node`, `AmendmentOp.Removed`, `tree.AmendmentHolds`.
- Produces:
  - `RemoveDraft` on a held node, or on a node whose removed subtree contains held nodes, marks each matching add operation `Removed: <reason>` on its amendment in the same transaction as the removal, and returns the amendment ids it changed in `RemoveResult.ChangedFiles`.
  - The `proposed_amendment_reference` refusal no longer fires for the add operation that created the removed node. It still fires when any other operation on any open amendment names the node (`target`, `by`, `superseded_by`, `blocks`).

- [ ] **Step 1: Write the failing tests.**
  - `TestRemoveHeldFeatureMarksItsOperationRemoved`: the folder is gone, the operation keeps its index and carries `removed: "<reason>"`, the hold index no longer lists it, and `store.ValidateAmendment` passes.
  - `TestRemoveHeldEpicMarksItsFeaturesOperationsToo`.
  - `TestRemoveHeldReplacementIsStillRefused`: a held feature that the same amendment's cancel names as `superseded_by` cannot be removed; reason `proposed_amendment_reference`.
  - `TestRenameHeldDraftKeepsTheHold` (Review Focus 3): after `tiller rename feature`, `gen.AmendmentHold` still returns the amendment and the operation's `key` is the staged original.
  - `TestLinkOnAHeldDraft`: `link --depends-on` and `link --surface` both succeed on a held feature.
- [ ] **Step 2: Run** `go test ./internal/core/mutate -run 'RemoveHeld|RenameHeld|LinkOnAHeld'`. Expected: FAIL on the remove tests; the rename and link tests may already pass, which is the point of pinning them.
- [ ] **Step 3: Implement** the exemption and the amendment rewrite inside the existing removal plan, guarded by the amendment file's snapshot like every other write there.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS apart from Task 7's skips.
- [ ] **Step 5: Commit** `Removing a held draft marks its operation removed`.

### Task 10: Withdraw removes what the amendment holds

**Files:**
- Modify: `internal/core/mutate/amendment.go:992` (`WithdrawAmendment`)
- Modify: `internal/cli/amendment_cmds.go` (`AmendmentWithdrawView`, human line)
- Test: `internal/core/mutate/amendment_test.go`, `internal/cli/amendment_cmds_test.go`

**Interfaces:**
- Consumes: the removal planner as changed in Task 9.
- Produces:
  - `WithdrawResult` gains `RemovedIDs []string` and `Detached []string`.
  - One `amendment_withdrawn` event whose `after` carries `reason` and `removed` (the node ids), and whose `files` list every removed path.
  - Human line: `Withdrew <ref>. Removed <n> draft node(s) it held.`

- [ ] **Step 1: Write the failing tests.**
  - `TestWithdrawRemovesHeldNodes`: an amendment holding one epic with two features and one task under an approved feature; after withdraw the three folders and the task folder are gone, the amendment is `withdrawn`, and its operations are unchanged (no `removed` markers: the amendment's status already says they never happened).
  - `TestWithdrawDetachesReferences`: a question that blocked a held feature loses that target; a proposed bug attached to it keeps its `source` citation and loses `attached_to`; the root order drops the held epic if it listed it.
  - `TestWithdrawIsAtomic`: make one held node's file unreadable; withdraw fails and nothing is removed.
  - `TestWithdrawFromDraftAndFromProposed`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run Withdraw`. Expected: FAIL.
- [ ] **Step 3: Implement** by collecting the held node ids from the amendment's own operations and planning their removal and the status write as one mutation. Remove epics before computing their features, so a subtree is planned once.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS apart from Task 7's skips.
- [ ] **Step 5: Commit** `Withdrawing an amendment removes the drafts it held`.

### Task 11: Approval promotes held nodes

**Files:**
- Modify: `internal/core/mutate/amendment_approve.go` (`applyAmendment`, new `(*batch).promote`, `materialize` kept for operations without `Node`)
- Modify: `internal/cli/amendment_cmds.go` (approve confirmation lines)
- Test: `internal/core/mutate/amendment_approve_test.go`, `internal/cli/e2e_amend_test.go`

**Interfaces:**
- Consumes: `AmendmentOp.Node`, `AmendmentOp.Removed`, `recordTaskBasis(taskDir string, tk *store.Task, deps Deps, rev vcs.Revisions) error` (`transition.go:1650`), `store.AmendmentEmpty`.
- Produces:
  - For each add operation with `Node` and no `Removed`: a held epic goes `draft` to `ready` (D8); a held task goes `proposed` to `ready` with `accepted_against` recorded by `recordTaskBasis` (D9); a held feature is left as it is. Each promotion emits the ordinary `approve.<noun>` event with `after.amendment` and `after.operation_index`.
  - An operation with `Removed` is skipped and reported in `ApproveResult.Applied` with status `removed`.
  - An add operation without `Node` goes through the existing `materialize` path and lands `draft` or `proposed` (D12).
  - A node already at its promoted status is accepted silently.
  - New epic ids are appended to the root order at approval, as today, from the promoted epics.
  - `store.AmendmentEmpty` treats an amendment whose operations are all `Removed` as empty.
  - An add operation whose `Node` names no node in the tree refuses the whole approval: reason `held_node_missing`, naming the operation index. Nothing is written.

- [ ] **Step 1: Write the failing tests.**
  - `TestApprovePromotesAHeldEpicAndLeavesItsFeatureDraft`: epic `ready`, feature `draft`, no hold on either afterwards, root order ends with the epic id, the feature is now a `specify` candidate.
  - `TestApprovePromotesAHeldTaskWithItsBasis`: status `ready`, `accepted_against.spec` is the current `task.md` blob, and `BasisDrift` reports none.
  - `TestApproveSkipsRemovedOperations`.
  - `TestApproveFollowsARenamedHeldDraft` (Review Focus 3).
  - `TestApproveIsResumable` (Review Focus 2): promote the epic by hand-editing its status to `ready` while the amendment stays `proposed`; approve succeeds and finishes the rest.
  - `TestApproveRefusesAMissingHeldNode`: reason `held_node_missing`, nothing written.
  - `TestApproveLegacyAddStillLandsDraft`: an amendment whose `add-epic` has no `node` creates the epic at approval in `draft`, and `approve epic` is then needed.
  - `TestApproveMixedBatch`: one held feature plus a cancel naming it as `superseded_by`; both apply, lineage is written in both directions.
  - `TestStagedQuestionCanBlockAHeldDraft` (D13): `--add-question --blocks <held feature key>` stages with the feature's id resolved and creates no question; after approval the question exists and blocks that feature.
  - Remove the `t.Skip` markers from Task 7 and rewrite those tests to expect the node at staging.
  - CLI end to end: stage, propose, approve; the confirmation lists `approved epic <ref>` and `feature <ref> stays draft; specify is next`.
- [ ] **Step 2: Run** `go test ./internal/core/mutate ./internal/cli -run 'Approve'`. Expected: FAIL.
- [ ] **Step 3: Implement.** In the apply loop, branch on `op.Node != ""`. `promote` loads the node through the batch's existing `editFor`, so it joins the same journaled transaction and guard set as the cancels and supersessions. The amendment's status write stays last.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, no skips.
- [ ] **Step 5: Commit** `Approving an amendment promotes the drafts it held`.

### Task 12: Read surfaces

**Files:**
- Modify: `internal/views/node.go` (`NodeView`), `internal/views/build.go` (feature card), `internal/views/list.go` (list item), `internal/views/map.go` (map node), `internal/views/nodepage.go:734,874` (amendment operation view)
- Modify: `internal/cli/amendment_cmds.go` (`show amendment` text), `internal/cli/tree.go`, `internal/cli/status.go`
- Test: `internal/views/node_test.go`, `internal/views/build_test.go`, `internal/views/nodepage_test.go`, `internal/cli/amendment_cmds_test.go`, `internal/cli/tree_test.go`, `internal/cli/status_test.go`

**Interfaces:**
- Consumes: `gen.AmendmentHold`.
- Produces:
  - `type views.AmendmentHoldView struct { ID string \`json:"id"\`; Key string \`json:"key,omitempty"\`; Ref string \`json:"ref"\`; Title string \`json:"title"\`; Status string \`json:"status"\` }` and `func amendmentHoldViewOf(t *store.Tree, n store.Node) *AmendmentHoldView`.
  - Field `AmendmentHold *AmendmentHoldView \`json:"amendment_hold,omitempty"\`` on `NodeView`, the Build feature card, the list item, and the map node.
  - `AmendmentOperationPageView.Created` is filled from `op.Node` for an open amendment (today it is filled only from application events), and the view gains `Removed string \`json:"removed,omitempty"\``.
  - `tiller show amendment` prints, per add operation, `held draft <ref>` or `removed: <reason>`; and one line under the header, `New work: held drafts, promoted at approval` or `New work: created at approval (staged before schema 14)`.
  - `tiller tree` and `tiller status <node>` append `(in amendment <key>)` to a held node's line.

- [ ] **Step 1: Write the failing tests** `TestNodeViewCarriesAmendmentHold`, `TestBuildCardCarriesAmendmentHold`, `TestListItemCarriesAmendmentHold`, `TestMapNodeCarriesAmendmentHold`, `TestTreeMarksAHeldNode`, and `TestStatusMarksAHeldNode`, each asserting the field or text above for a held feature and its absence for an unheld one; `TestAmendmentPageLinksHeldDraftsBeforeApproval`; `TestShowAmendmentNamesItsRule` for both rules; `TestShowAmendmentReportsAMissingHeldNode`, where the operation line reads `held draft <id>: missing` and the read still succeeds.
- [ ] **Step 2: Run** `go test ./internal/views ./internal/cli -run 'AmendmentHold|AmendmentPage|ShowAmendment|TreeMarks'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Home's count tiles and `status` totals keep counting a held draft as a draft.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Read surfaces name the amendment that holds a draft`.

### Task 13: The console

**Files:**
- Modify: `ui/src/api.ts` (types: `AmendmentHoldView`, `amendment_hold` on node, card, list item, and map node; `removed` on the operation; `withdraw_amendment` queue kind; `draft` amendment status)
- Modify: `ui/src/components/NodeHeader.tsx` (chip beside `HeldChip`), `ui/src/views/Build.tsx` (card flag), `ui/src/components/Navigator.tsx` (row marker), `ui/src/views/explore/map/MapCard.tsx`
- Modify: `ui/src/views/node/AmendmentBody.tsx` (draft links, removed operations, the draft banner), `ui/src/views/explore/AmendmentsPane.tsx:23` (statuses), `ui/src/views/explore/ledgerStatus.ts`
- Modify: `ui/src/components/DecisionCard.tsx`, `ui/src/views/home/queueGroups.ts`, `ui/src/views/home/bands.ts` (the new kind never compresses into a band, like `approve_amendment`)
- Test: the matching `*.test.tsx` beside each, plus `ui/src/components/__tests__/home.test.tsx`

**Interfaces:**
- Consumes: the wire fields of Task 12 and the queue kind of Task 6.
- Produces: no new exports beyond the types.

- [ ] **Step 1: Write the failing tests.**
  - Node header: a node with `amendment_hold` shows a chip reading `In amendment` whose accessible name is `Held by amendment <title>`, linking to the amendment page; absent otherwise.
  - Build: a held feature's card in the Draft column carries the text `In amendment`; an unheld draft does not.
  - Navigator and map: a held row or card carries the same text label, not color alone.
  - Amendment page: an add operation with `created` renders its node as a link before approval; one with `removed` renders `Removed: <reason>` and no link; a `draft` amendment shows `The agent is still working on this amendment.`
  - Ledger: the tabs read All, Draft, Proposed, Approved, Withdrawn with counts; `ledgerStatus` orders draft and proposed first.
  - Home: a `withdraw_amendment` entry renders a decision card titled with the amendment and a copyable withdraw command, with the line `Nothing was proposed from this document.`
- [ ] **Step 2: Run** `cd ui && npx vitest run`. Expected: FAIL.
- [ ] **Step 3: Implement**, reusing the existing chip and flag components and their neutral register (a held draft is not blocked work).
- [ ] **Step 4: Run** `cd ui && npx vitest run && npx tsc --noEmit`, then `make ui`. Expected: PASS, and `ui/dist` changes.
- [ ] **Step 5: Commit** `Console: held drafts, draft amendments, and the withdraw card`, including `ui/dist`.

---

## Part 3: One skill

### Task 14: `decompose` absorbs `intake`

**Files:**
- Modify: `internal/skills/decompose/SKILL.md` (rewritten in three parts)
- Delete: `internal/skills/intake/SKILL.md`
- Modify: `internal/skills/skills.go:8,34,40` (embed list, `order`), `internal/skills/capture/SKILL.md`, `internal/skills/amend/SKILL.md`, `internal/skills/orient/SKILL.md`, `internal/skills/code-review/SKILL.md` (its pointer to `tiller guide intake`)
- Modify: `internal/gen/orientation.go:46` (`orientationSchemaGen = 60`, with a `// Generation 60` comment line), and its text at `:132`, `:906`, `:951-991`
- Modify: `internal/views/next_enumerate.go` (skill `intake` becomes `decompose` on `decompose_amendment`), `internal/cli/guide.go`, `internal/cli/intake.go:255` (stale comment)
- Test: `internal/skills/skills_test.go`, `internal/gen/orientation_test.go`, `internal/cli/guide_test.go`

**Interfaces:**
- Consumes: every behavior of Parts 1 and 2.
- Produces: `skills.Names()` returns ten names with no `intake`; `tiller guide intake` is refused as an unknown skill, naming `decompose`.

- [ ] **Step 1: Write the failing tests.**
  - `TestSkillNamesDropIntake`: ten names, in today's order minus `intake`.
  - `TestGuideIntakeNamesDecompose`: `tiller guide intake` exits 2 and its message contains `decompose`.
  - `TestDecomposeSkillCoversBothRoutes`: the skill text contains the headings `## What becomes what`, `## Recording a founding design`, and `## Recording an amendment`, and the strings `tiller propose amendment`, `tiller add bug`, `--source`.
  - `TestOrientationGeneration60`: the stamp parses as 60 and the text no longer names an `intake` skill.
  - Update `TestNextOffersDecomposeForADraftIntakeAmendment` to expect skill `decompose`.
- [ ] **Step 2: Run** `go test ./internal/skills ./internal/gen ./internal/cli ./internal/views`. Expected: FAIL.
- [ ] **Step 3: Write the skill.** Three parts under the headings above.
  - *What becomes what*: the mapping rules now in `intake` (a statable acceptance contract is a feature; a correction inside a feature's scope is a task; a judgment the document declines to make is a question; an ordering claim is never a node) plus the shelf rule from D6 (a concrete defect, chore, or tweak is added with `tiller add bug|chore|tweak --source`, exactly as `capture` writes one, and is not staged on the amendment).
  - *Recording a founding design*: today's decompose choreography, unchanged.
  - *Recording an amendment*: stage each add with `tiller amend`; then write each held epic's `epic.md` and each held feature's narrative, and record dependencies and surfaces with `tiller link`, exactly as for a draft tree; correct in place with `rename` and `remove`; stage changes to existing work through the `amend` skill; finish with `tiller propose amendment`, including when nothing was staged, and say so in the readout.
  - `capture` keeps its trigger for a defect found outside a document and points to `decompose` for the mapping. `amend` describes held drafts, `propose`, and that withdraw removes them. `orient` and the orientation text describe the two routes and drop the `intake` skill.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `Skills: decompose absorbs intake; generation 60`.

### Task 15: Specs and reference docs

**Files:**
- Modify: `docs/spec/artifact-schema.md` (Directory layout; Design layer; Amendments: states, `node`, `removed`, the folder source, "staging creates inert drafts"; Provenance; Decisions settled so far)
- Modify: `docs/spec/cli-surface.md` (Bootstrapping; Decomposition; Large changes: amendments; the command reference; Who may run what; Decisions settled so far)
- Modify: `docs/spec/skill-set.md`, `docs/spec/ui-surface.md` (Home's queue kinds, Build's flags, Explore's ledgers, Node detail), `docs/spec/distribution.md` (schema 14 in Migration)
- Modify: `docs/tiller-commands.md` (`intake`, `propose`, `amend`, `approve amendment`, `withdraw`, the skills line), `docs/roadmap.md`
- Modify: `site/src/content/docs/concepts.md:279,355,427`, `skills.md:24`, `console.md`, `quickstart.md`; then `cd site && npm run sync`
- Test: `scripts/check-doc-versions.sh`, `grep -rnP "\x{2014}" docs/spec docs/tiller-commands.md site/src/content/docs/*.md` (the em dash check)

**Interfaces:**
- Consumes: the built behavior. Where this plan and the code differ, describe the code and note the difference in the design doc's Status.

- [ ] **Step 1: Write the docs.** Fold D1 to D13 into each spec's "Decisions settled so far". Replace every statement that staging executes nothing with the held-draft rule. State the deferred items from the design doc's Open section in the roadmap.
- [ ] **Step 2: Run** `make lint`, `scripts/check-doc-versions.sh`, and the em dash grep. Expected: clean, no matches.
- [ ] **Step 3: Run** `cd site && npm run sync && npm run build`. Expected: builds; discard any line-ending-only rewrite of the notices file.
- [ ] **Step 4: Commit** `Docs: one door for documents, held drafts, and tiller propose`.

### Task 16: Verify end to end

**Files:**
- Modify: `internal/cli/e2e_intake_test.go`, `internal/cli/e2e_amend_test.go`
- Test: the whole suite

**Interfaces:**
- Consumes: everything.

- [ ] **Step 1: Write the arcs** as end-to-end tests through `execCLI`, one test each, named for what they prove:
  - `TestEndToEndFoundingDesignThroughInit` and `TestEndToEndFoundingDesignThroughIntakeNoAmendment`: draft tree, `approve epic`.
  - `TestEndToEndOnePagerOnBrownfield`: confirmed baseline, no epics; `intake`; `guide` names `decompose`; stage one epic and one feature; write `epic.md`; `link --surface`; `propose`; `guide` names `approve amendment`; approve; the epic is `ready`, the feature `draft`; `guide` names `specify`.
  - `TestEndToEndDocumentThatReplacesWork`: an added feature and a cancel that names it.
  - `TestEndToEndPartialAcceptance`: remove one held draft, approve the rest.
  - `TestEndToEndNothingToBuild`: `propose` an empty amendment; `approve` refuses `amendment_empty`; the queue offers withdraw; withdraw.
  - `TestEndToEndFolderGoesStale`: change one file under the landed folder; `show amendment` reads stale and `approve` refuses.
  - `TestEndToEndLegacyAmendment`: a schema-13 fixture with a proposed amendment, migrated, then approved under the old rule.
- [ ] **Step 2: Run** `make test` and `make lint`. Expected: PASS.
- [ ] **Step 3: Run the `verify` skill** (build the binary, drive it against a scratch project): repeat the one-pager arc by hand and open `tiller serve`; confirm the held flag on Build, Explore, the map, and the node header, the amendment page's links, the ledger's Draft tab, and the Home card, in light and dark.
- [ ] **Step 4: Commit** `Verify: one door for documents end to end`.
