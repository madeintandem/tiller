# Field-run issues #146 to #150: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve Allen's five field-run issues per the settled rulings R1 to R7.

**Architecture:**
- Mutate gains four things:
  - an adopt path for an existing aggregate ref;
  - a grain-aware attest branch guard;
  - a persisted `started_grain` on tasks;
  - an add-only dependency arm for done targets.
- Views routing consumes execution observations through `workflow.Input`. The CLI and serve fill that input, so both surfaces route identically.
- Message, handoff, skill and spec text move with the behavior.
- One orientation generation bump (52 to 53) and one schema bump (10 to 11) land in the last task.

**Tech Stack:** Go (cobra CLI, testify), the embedded skills and orientation markdown, and the docs under `docs/`.

**Spec:** `docs/plans/2026-10-19-adayag-field-issues-plan.md` (section "Settled rulings"). Read it before any task.

## Global Constraints

- Work in a worktree on branch `field-issues-146-150`, made with `git worktree add ../tiller-field-issues -b field-issues-146-150 main`. Commit with `-s`; JC fast-forwards main.
- Every commit message ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Tiller never deletes or renames Git refs. Every message keeps "Tiller will not delete refs" where that sentence appears today.
- Never reinterpret a task branch as an aggregate. The adopt path refuses any name that is a task's `branch` or `review_ref.branch`.
- The spec house style applies to docs and skills: no em dashes.
- Run `/verify` before each commit that touches CLI, serve, views, gen, store or core packages.
- The mutate package tests can exceed 10 minutes under load. Run them with `go test ./internal/core/mutate -run <Name> -timeout 20m`.

## Review Focus

1. **Detached HEAD at attest.** `Branch()` returns `HEAD`. The guard must refuse with the check-out message and must not crash or pass. The test belongs to Task 4.
2. **Adopt with a prefix-conflicting ref.** For example, `tiller/reviews/features/pay` exists while the aggregate is `tiller/reviews/features/pay/checkout`. Adopt must still refuse with `branch_ref_conflict`; only an exactly equal name is adoptable. The test belongs to Task 3.
3. **An observed task's owner is empty on the branch,** for example started but with the owner unset. Treat it as "someone else", which means fall through and never mute. The test belongs to Task 5.
4. **An old task.yml with no `started_grain` under a feature now at coarse grain.** It must resolve exactly as today, with no new notes and no behavior change. The test belongs to Task 6.
5. **A done target given as a key, not an ID, in `link task --depends-on`.** The done-target arm resolves keys the same way the existing link does. The test belongs to Task 1.

---

### Task 1: Dependency lock. Drop the amendment remedy and add an edge to a done target on a started task (#148, R7)

**Files:**
- Modify: `internal/core/mutate/depends.go` (`DependencyChangeLockedError.Error()` near :83, `guardDependencyChangeOpen` at :403, Link's call at :155)
- Modify: `internal/core/mutate/unlink.go` (call at :133; it passes no added edges)
- Modify: `internal/cli/unlink.go` (`reportDependencyChangeError` :156-174, `dependencyChangeLockedView` :29)
- Test: `internal/core/mutate/depends_test.go`, `internal/cli/depends_cli_test.go`
- Docs:
  - `docs/spec/cli-surface.md:160,162`
  - `docs/spec/skill-set.md:71,75`
  - `docs/tiller-commands.md:368-372`
  - `internal/skills/amend/SKILL.md:48`
  - `site/src/content/docs/source-quickstart.md:344`
  - the dependency boundary paragraph in `docs/spec/artifact-schema.md`

**Interfaces:**
- Produces: `func guardDependencyChangeOpen(tree *store.Tree, node store.Node, added []string) ([]store.PlannedWrite, error)`. `added` holds the resolved target IDs being added; Unlink passes `nil`.
- Produces: in the JSON, the `dependencyChangeLockedView` field `Remedy` is replaced by `Detail string json:"detail"`.

- [ ] **Step 1: Write the failing tests.**
  - `TestLinkTaskAddsEdgeToDoneTargetWhileInProgress`: task A is `in_progress` and target B is `done`. `mutate.Link` succeeds and A's `depends_on` contains B's ID.
  - `TestLinkTaskAddsEdgeToDoneTargetByKey`: the same, with the target given by its key.
  - `TestLinkTaskRefusesEdgeToInReviewTargetWhileInProgress`: B is `in_review`. Expect `*mutate.DependencyChangeLockedError`.
  - `TestUnlinkTaskStillLockedWhileInProgress`: B is `done`, and unlink is refused.
  - `TestLinkFeatureStillLockedWithDoneTarget`: an active feature with a branch, and a done target feature, is refused.
  - In `depends_cli_test.go`, rename the old test to `TestDependencyChangeLockedIsBlockedForLinkAndUnlinkWithoutAmendmentRemedy`:
    - JSON: assert `"reason": "dependency_change_locked"` and `"detail":`; NotContains `tiller add amendment` and `"remedy"`.
    - Human: Contains `cannot change in active`; NotContains `amendment`.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/core/mutate -run 'TestLinkTask|TestUnlinkTaskStill|TestLinkFeatureStill' -timeout 20m` and `go test ./internal/cli -run TestDependencyChangeLocked`. They should fail on the arm and on the remedy text.
- [ ] **Step 3: Implement.**
  - **Task case in `guardDependencyChangeOpen`:** when the status is not `proposed` or `ready`, the call is still allowed when all of these hold:
    - `len(added) > 0`;
    - the edit removes nothing (Link only, since Unlink passes `nil`);
    - every ID in `added` is a task whose status is `done`.

    Otherwise return the locked error. Feature, epic and shelf cases are unchanged.
  - **Link:** pass `addedDependencies(edit.before, edit.after)`.
  - **`Error()`** becomes `"mutate: %s %s is %s; dependency changes are locked once work starts (return ready %s work before it starts; a started task may only add an edge to a done task)"`.
  - **CLI human text** becomes `"Blocked: dependencies on %s %s cannot change in %s. Only an added edge from a started task to a done task is allowed; amendments cannot change dependencies."`. `Detail` carries the same sentence minus the "Blocked:" prefix.
- [ ] **Step 4: Run the tests and confirm they pass.** Same commands as Step 2.
- [ ] **Step 5: Docs.**
  - **Lock boundary text** (`cli-surface.md:160`, `skill-set.md:71`, `artifact-schema.md`): the boundary gains "a started task may add (never remove) an edge whose target task is done; the refusal names the edge rule, never an amendment".
  - **"Unsupported" text** (`cli-surface.md:162`, `skill-set.md:75`): replace "the amendment hint in that refusal" with the new rule. In-place edits on started work remain unsupported except that arm.
  - **Hint text** (`tiller-commands.md:368-372`, `amend/SKILL.md:48`, `source-quickstart.md:344`): drop the hint language and state the done-target arm.
- [ ] **Step 6: Commit** with the message `Depends: done-target edges on started tasks; drop the dead amendment remedy (#148)`.

### Task 2: `configure --review-grain` tells you to commit before starting tasks (#146)

**Files:**
- Modify: `internal/cli/configure.go` (`humanConfigure` at :165)
- Test: `internal/cli/configure_test.go`
- Docs: `docs/tiller-commands.md` (configure section) and `docs/spec/cli-surface.md` (Safe configure window, :637)

- [ ] **Step 1: Write the failing test.**
  - `TestConfigureReviewGrainChangeSaysCommitBeforeStarting`: human output for a changed grain contains `Commit this change to integration before starting tasks`.
  - The no-change case does not contain it.
- [ ] **Step 2: Run it and confirm it fails.** Run `go test ./internal/cli -run TestConfigureReviewGrain`.
- [ ] **Step 3: Implement.** When `res.Changed`, after the "resolved grain here is now" line, append `"\nCommit this change to integration before starting tasks: work started in another checkout or worktree resolves the grain it can see."`.
- [ ] **Step 4: Run it and confirm it passes.**
- [ ] **Step 5: Docs.** Add the same sentence to both doc sites.
- [ ] **Step 6: Commit** with the message `Configure: say to commit a grain change before starting tasks (#146)`.

### Task 3: Aggregate messages and `start task --adopt-aggregate` (#147, R1)

**Files:**
- Modify: `internal/core/mutate/task_branches.go` (`BranchSafetyError` and its `Error()`, `TaskDependencyBlockedError`, `taskStartBase` near :327)
- Modify: `internal/core/mutate/task_aggregate.go` (`prepareTaskAggregate`)
- Modify: `internal/core/mutate/transition.go` (`TransitionInput`, `TransitionResult`, the start block :698-717)
- Modify: `internal/vcs/git.go` (new method)
- Modify: `internal/cli/lifecycle_cmds.go` (`newStartCmd` :581, `humanTransition` :1776, `TransitionView`)
- Modify: `internal/cli/branch_errors.go` (:27-44)
- Test: `internal/core/mutate/task_aggregate_adopt_test.go` (new), `internal/cli/lifecycle_cmds_test.go`
- Docs:
  - `docs/spec/artifact-schema.md:373-375` (who records the aggregate)
  - `docs/spec/cli-surface.md` (the start section and its command table row)
  - `docs/tiller-commands.md` (start)

**Interfaces:**
- Produces:
  - `TransitionInput.AdoptAggregate bool`
  - `TransitionResult.AdoptedCommits []string`, exposed in JSON as `adopted_commits,omitempty`
  - `BranchSafetyError` reasons:
    - `aggregate_unrecorded`: a ref exists at exactly the derived aggregate name, with no flag.
    - `aggregate_adopt_missing`: the flag is set, but no ref exists.
    - `aggregate_adopt_recorded`: a branch is already recorded.
    - `aggregate_adopt_task_branch`: the name equals a task's `branch` or `review_ref.branch`.
    - `aggregate_adopt_grain`: the node resolves to task grain, or the noun is not a task.
  - `TaskDependencyBlockedError.AggregateUnrecorded bool`
  - `func (g *Git) CommitsBetween(base, tip string) ([]string, error)`. It runs `git log --format=%h %s base..tip`, newest first. Mutate reaches it through an optional interface assertion on `vcs.Revisions`, the same way `LocalBranches` is reached.

- [ ] **Step 1: Write the failing tests.** Seed each with the reproduction from issue #147, using `seqDeps`, `rgLayFeature`, `rgDriveTaskToInReview`, `gitRunM` and `gitCommitAllM`.
  - `TestStartRefusesUnrecordedAggregateByName`:
    - The ref exists at the derived name, and the plain start runs.
    - `safety.Reason == "aggregate_unrecorded"`.
    - The message contains `exists but is not recorded on feature` and `--adopt-aggregate`.
  - `TestStartAdoptAggregateRecordsAndStartsDependent`:
    - The hand-built ref holds alfa's merged branch, and the start passes `AdoptAggregate: true`.
    - The start succeeds, and `feature.Branch` equals the derived name.
    - The result's `BaseBranch` is the aggregate, and `AdoptedCommits` is non-empty.
  - `TestStartAdoptAggregateStillRequiresPrerequisite`: the hand-built ref is empty. Expect `*TaskDependencyBlockedError` with `AggregateUnrecorded == false`, because the aggregate is now recorded.
  - `TestStartAdoptAggregateRefusesMissingRef`: expect `aggregate_adopt_missing`.
  - `TestStartAdoptAggregateRefusesRecordedBranch`: expect `aggregate_adopt_recorded`.
  - `TestStartAdoptAggregateRefusesTaskGrain`: expect `aggregate_adopt_grain`.
  - `TestStartAdoptAggregateRefusesPrefixConflict`: the ref is `tiller/reviews/features/pay`. Expect `branch_ref_conflict` (Review Focus 2).
  - `TestStartAdoptAggregateRefusesTaskBranchName`: a task records the derived name as its `branch`. Expect `aggregate_adopt_task_branch`.
  - `TestStartAdoptAggregateAtEpicGrain`: the epic-grain variant records `epic.Branch`.
  - `TestStartDependencyRefusalNamesUnrecordedAggregate`: the issue's second refusal, where the ref was renamed aside. Expect `AggregateUnrecorded == true`, and the CLI human text contains `is not recorded` and `--adopt-aggregate`.
  - In the CLI test `TestStartAdoptAggregateFlag`:
    - `--json` output carries `"aggregate_branch"` and `"adopted_commits"`.
    - Human output contains `Adopted existing aggregate`.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/core/mutate -run 'TestStartAdopt|TestStartRefusesUnrecorded|TestStartDependencyRefusalNames' -timeout 20m` and `go test ./internal/cli -run TestStartAdoptAggregateFlag`.
- [ ] **Step 3: Implement.**
  - **`prepareTaskAggregate`:**
    - Thread `adopt bool` into it.
    - Grain check:
      - A refusal for `adopt` on a non-task noun, or at task grain, happens before any other work. The non-task noun case is checked in `Transition`.
      - With `adopt` set and a branch already recorded: `aggregate_adopt_recorded`.
    - When the branch is empty:
      - Without `adopt`: the existing `guardNewBranch` path, plus one change. A `branch_ref_conflict` whose `Conflict == branch` becomes `aggregate_unrecorded`.
      - With `adopt`: run the same reservation checks with exact-equality ref conflicts ignored (prefix conflicts still refuse). Then require `RefExists(vcs.BranchRef(branch))`, and refuse names held as a task's `branch` or `review_ref.branch`.
    - After the adopt checks pass, record the branch exactly as today.
  - **`Transition`:** when the start adopted, fill `AdoptedCommits` with `CommitsBetween(<integration>, branch)`. Leave it empty if the interface is absent.
  - **`taskStartBase`:** sets `AggregateUnrecorded` when the aggregate it names was assigned in this start and was not on the feature before the start.
  - **`BranchSafetyError` messages:**
    - `aggregate_unrecorded`: `"branch %q exists but is not recorded on %s %s; review its history against integration (git log <integration>..%s) and, if it is this aggregate, rerun start with --adopt-aggregate; otherwise rename it aside with Git. Tiller will not delete refs"`
    - The remaining adopt reasons each get one plain sentence naming the condition that failed.
  - **CLI:**
    - `--adopt-aggregate` goes on `start` only. Its help text: "record the existing ref at this feature's or epic's derived aggregate name as its aggregate; review its history first".
    - `humanTransition` adds `" Adopted existing aggregate %s; it holds %d commit(s) beyond integration:"`, followed by the commit lines.
    - `branch_errors.go:44` gains the `AggregateUnrecorded` variant: `"Blocked: task %s requires %s; aggregate %s is not recorded and does not exist. Build it containing the prerequisite's complete branch, then start with --adopt-aggregate."`
- [ ] **Step 4: Run the tests and confirm they pass.** Same commands as Step 2.
- [ ] **Step 5: Docs.**
  - `artifact-schema.md:373-375`: the first riding task start records the aggregate, or adopts an existing ref at the derived name with `--adopt-aggregate`.
  - The `cli-surface.md` start section and its command table row: the flag, its refusal reasons, and that agents may pass it.
  - `tiller-commands.md`: the start entry.
- [ ] **Step 6: Commit** with the message `Start: adopt an existing aggregate at its derived name; name unrecorded aggregates (#147)`.

### Task 4: Attest branch guard (#150, R3)

**Files:**
- Create: `internal/core/mutate/attest_branch.go`
- Modify: `internal/core/mutate/attest.go` (`Attest` at :65), `internal/core/mutate/task_branches.go` (`BranchSafetyError`), `internal/cli/attest.go` (`runAttest` at :73), `internal/cli/branch_errors.go`
- Test: `internal/core/mutate/attest_branch_test.go` (new), `internal/cli/attest_test.go`
- Docs: the attest section of `docs/spec/cli-surface.md`, `docs/tiller-commands.md` (attest)

**Interfaces:**
- Consumes: `aggregateForTask(tree, feature) (branch, noun, id string)`, `aggregateRecovery(tree, id)`, `epicByKey`, `deps.rev(tree.Root)`.
- Produces: `func guardAttestBranch(tree *store.Tree, rev vcs.Revisions, feature *store.Feature) error`, which returns `*BranchSafetyError{Reason: "attest_branch_mismatch"}`.
- Produces: `BranchSafetyError` gains `Worktree string` and `RefMissing bool`.

- [ ] **Step 1: Write the failing tests.** Use real git roots built with the `rgLayFeature` helpers. The feature is at feature grain and its aggregate is recorded by a first task start.
  - `TestAttestFeatureRefusedOffAggregateBranch`:
    - `--criterion` run on integration.
    - Reason `attest_branch_mismatch`; `Branch` is the aggregate and `CurrentBranch` is `main`.
    - `feature.yml` is unchanged.
  - `TestAttestFeatureGateRefusedOffAggregateBranch`: the same check for `--gate`.
  - `TestAttestFeaturePassesOnAggregateBranch`: run after checking out the aggregate.
  - `TestAttestFeaturePassesInLinkedWorktreeOnAggregate`: `git worktree add` on the aggregate, with the tree loaded from that worktree.
  - `TestAttestFeatureRefusalNamesWorktree`: the aggregate is checked out in a linked worktree and the attest runs from main. `Worktree` equals that path.
  - `TestAttestFeatureRefusalWhenAggregateRefMissing`: the ref is deleted locally. `RefMissing` is true, and the message contains `git fetch && git switch`.
  - `TestAttestFeatureRefusedOnDetachedHead`: Review Focus 1.
  - `TestAttestFeatureAllowedAtTaskGrain`: run on integration, at task grain.
  - `TestAttestFeatureAllowedForDoneFeature`.
  - `TestAttestFeatureAllowedForRecoveredReview`.
  - `TestAttestFeatureRefusedAtEpicGrain`: the epic aggregate is checked.
  - `TestAttestTaskUnguarded`: a task attest run on main at coarse grain succeeds.
  - CLI test `TestAttestOffAggregateIsBlocked`: exit 1, `"reason": "attest_branch_mismatch"`, and `"branch":`.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/core/mutate -run 'TestAttestFeature|TestAttestTaskUnguarded' -timeout 20m` and `go test ./internal/cli -run TestAttestOffAggregate`.
- [ ] **Step 3: Implement `guardAttestBranch`** and call it from `Attest` for `*store.Feature` nodes, after input validation and before any write.
  - **Steps:**
    1. Resolve the grain with `store.ResolvedReviewGrain(tree.Manifest, epicByKey(tree, feature.EpicKey), feature)`; at task grain, return nil.
    2. Get the aggregate and its owner from `aggregateForTask`. Return nil if the branch is empty, if the owner node is done or cancelled, or if `aggregateRecovery(tree, id) != nil`.
    3. Compare `rev.Branch()` to the aggregate; equal means pass.
    4. Otherwise fill `Worktree` via an optional `interface{ WorktreeFor(string) (string, bool, error) }`, and set `RefMissing` from `RefExists(vcs.BranchRef(branch))`.
  - **`Error()` for `attest_branch_mismatch`:** shared prefix `"feature %s acceptance is recorded on its aggregate branch %q, not %q; "`, then by case:
    - worktree: `"run this attest in the worktree at %s, then commit there before submit"`
    - local, not checked out: `"check out %q, run this attest there, and commit before submit"`
    - missing: `"run git fetch && git switch %q, run this attest there, and commit and push before submit"`
  - **CLI:** `runAttest` routes errors through `branchBlockedView` before `emitFailure`, emitting with `emitReport(cmd, "attest", false, data, human, 1)`. Add `worktree` to `branchBlockedData`.
- [ ] **Step 4: Run the tests and confirm they pass.** Same commands as Step 2.
- [ ] **Step 5: Docs.** The attest sections in `cli-surface.md` and `tiller-commands.md` cover the guard, its exemptions, and the three messages.
- [ ] **Step 6: Commit** with the message `Attest: refuse feature acceptance off its coarse aggregate branch (#150)`.

### Task 5: Route guidance honors execution observations (#149, R4 and R5)

**Files:**
- Modify: `internal/workflow/workflow.go` (`Input` at :15)
- Modify: `internal/views/readout.go` (`RouteDecision` at :1249, `RouteDecisionOf` at :1312, `Advisory`)
- Modify: `internal/views/route_claimable.go` (`claimableExecution`, `ownedExecutionCandidates`, `ClaimableExecution` at :156)
- Modify: `internal/readinput/workflow.go`
- Modify: `internal/serve/route.go` (:57-91)
- Modify: `internal/cli/readout.go` (`gatherReadoutInput` :147, `renderCorrectionOf` :253), `internal/cli/guide.go` (`suggestSkill` :95)
- Modify: `internal/skills/execute/SKILL.md` (near :33)
- Test: `internal/views/route_observed_test.go` (new), `internal/serve/route_test.go`, `internal/cli/readout_observed_test.go` (new)
- Docs: `docs/spec/ui-surface.md:294` and the routing section of `docs/spec/cli-surface.md`

**Interfaces:**
- Produces:
  - `type workflow.Observation struct { Branch, Owner string }`
  - `workflow.Input.Observed map[string]Observation`, keyed by task ID
- Produces:
  - `type views.ObservedTask struct { Task *store.Task; Branch, Owner string }`
  - `RouteDecision` gains:
    - `Observed *ObservedTask`: the reader's own in-flight task, which mutes the route.
    - `SkippedObserved []ObservedTask`
    - `Parallel *store.Task`: the next eligible task when the route is muted.
- Produces: `func readinput.ObservationsFrom(s *execution.Snapshot) map[string]workflow.Observation` and `func readinput.WorkflowObserved(t *store.Tree) workflow.Input`. The second calls `Workflow(t)`, then `execution.Load` plus `Settle`, and sets `Observed`. On any error it leaves `Observed` nil.

- [ ] **Step 1: Write the failing views tests.** Use the `readyFeatureWithTask` fixtures and `workflow.Input{Observed: ...}`.
  - `TestRouteMutesReadersOwnObservedTask`:
    - T1 is ready and unclaimed on integration; observed with owner `jc`, reader `jc`.
    - `Seam == "execute"`, `Observed.Task == T1`, and `ClaimableExecution()` is false.
    - `Reason` contains `is in flight on tiller/...; continue it there`.
  - `TestRouteMutedOwnTaskOffersParallelNext`: T2 is also claimable. `Parallel == T2`, and `Advisory` contains `Next eligible if you're working in parallel: ` with T2's claim command.
  - `TestRouteSkipsObservedTaskOwnedByOther`:
    - T1 is observed with owner `allen`, reader `jc`, and T2 is claimable.
    - The route claims T2, and `SkippedObserved[0].Task == T1`.
    - `Advisory` contains `was skipped because it is in flight on` and `(allen)`.
  - `TestRouteSkipsObservedTaskWithEmptyOwner`: Review Focus 3.
  - `TestRouteObservedOwnTaskBeatsEarlierClaimable`: an unobserved claimable comes earlier in order. The muted own task is still primary, and the earlier task is `Parallel`.
  - `TestRouteSkipsAllObservedFallsThroughToLaterArms`: the only claimable is observed and someone else's. The decision comes from the later arms and carries the skip note.
  - `TestRouteObservationsKeepContinuityAndQuestionHold`: the #139 continuity note and `SkippedQuestionHold` still render alongside the observed notes.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/views -run TestRoute.*Observed`.
- [ ] **Step 3: Implement in views.**
  - **Owned pass and claimable walk:** both consult `in.Observed[task.ID]`.
    - Observed with `Owner == owner` (and owner non-empty): the first such task, in walk order, becomes `Observed`, and the walk continues to find the first unobserved eligible task for `Parallel`. The muted decision returns ahead of any claim.
    - Observed otherwise: append the task to `SkippedObserved` and continue.
  - **Muted text:**
    - Reason: `"%s is in flight on %s; continue it there, not from this integration view"` (ref, branch).
    - `Workflow` is nil.
  - **`Advisory` appends:**
    - per skip: `"Execution note: %s was skipped because it is in flight on %s (%s)."`
    - with `Parallel`: `"Next eligible if you're working in parallel: %s; claim it with `tiller claim task %s`, then start it with `tiller start task %s`."`
  - **`ClaimableExecution()`** additionally requires `d.Observed == nil`.
- [ ] **Step 4: Run the views tests and confirm they pass.**
- [ ] **Step 5: Write the failing surface tests.**
  - In `route_test.go`, `TestRouteHandlerMutesReadersObservedTask`:
    - Reuse `observationFixture` from `execution_test.go`, writing an owner `jc` task.yml in the worktree.
    - `/api/route` `reason` contains `continue it there`.
  - `TestRouteHandlerSkipsOthersObservedTask`: owner `allen`.
  - In `readout_observed_test.go`, `TestReadoutNextDropsClaimForObservedTask`:
    - A git fixture whose task branch is in a worktree, carrying claim and start, while integration is still `ready`.
    - Neither `tiller readout` Next nor `tiller guide` contains `claim it with`.
    - Both contain `continue it there`.
- [ ] **Step 6: Implement the surfaces.**
  - **Serve:** `RouteHandler` builds the input as `in := readinput.Workflow(t); in.Observed = readinput.ObservationsFrom(executionSnapshot(r))`, then deletes its inline mute block.
  - **CLI:** `gatherReadoutInput` (all moments), `renderCorrectionOf` and `suggestSkill` use `readinput.WorkflowObserved`. In `suggestSkill`, when `decision.Observed != nil`, the reason is `"Your task is in flight on <branch>; continue it there."`.
- [ ] **Step 7: Run the surface tests and confirm they pass.** Run `go test ./internal/serve -run TestRouteHandler` and `go test ./internal/cli -run 'TestReadoutNextDropsClaim'`.
- [ ] **Step 8: Skill and docs.**
  - **Execute SKILL.md near :33:** add "Route guidance sees in-flight work only on local branches and worktrees; fetch to see teammates' branches."
  - **`ui-surface.md:294`:** route guidance honors observations through the shared walk; eligibility still uses the served tree.
  - **`cli-surface.md` routing:** state the same rule, including the parallel line and the skip note.
- [ ] **Step 9: Commit** with the message `Route: honor execution observations on CLI and console alike (#149)`.

### Task 6: Persist the started grain; guidance for mixed-grain features (#146, R6)

**Files:**
- Modify: `internal/store/node.go` (`Task` at :250), `internal/store/reviewgrain.go`, and the load-time validation beside `validateStoredReviewGrain`
- Modify: `internal/core/mutate/transition.go` (the start block, plus every task-grain decision in check and submit)
- Modify: `internal/core/mutate/task_branches.go` (`taskStartBase`, `TaskDependencyBlockedError`)
- Modify: `internal/core/mutate/submit_node.go` (`TaskReviewOpenError` :654)
- Modify: `internal/cli/lifecycle_cmds.go` (`offlineReviewRefForTask` :941 and its callers, the `TaskReviewOpenError` rendering :1114-1123)
- Modify: `internal/cli/branch_errors.go`
- Modify: `internal/views/readout.go` (`Readout`), `internal/cli/readout_render.go`
- Test: `internal/core/mutate/started_grain_test.go` (new), `internal/store/reviewgrain_test.go`, `internal/cli/lifecycle_cmds_test.go`, `internal/views/readout_test.go`
- Docs: the task.yml fields in `docs/spec/artifact-schema.md` and the review grain paragraph (:375); `docs/spec/cli-surface.md` (around :502, the pr_required text, and :637)

**Interfaces:**
- Produces: `Task.StartedGrain string yaml:"started_grain,omitempty"`. Load refuses values outside task, feature and epic, as `review_grain` does.
- Produces: `func store.TaskReviewGrain(m *store.Manifest, e *store.Epic, f *store.Feature, task *store.Task) (grain, source string)`. It returns `(task.StartedGrain, "started")` when set, and `ResolvedReviewGrainWithSource(m, e, f)` otherwise.
- Produces: `TaskDependencyBlockedError.DependencyStartedGrain string`.
- Produces: `Readout.GrainNotes []string`, exposed in JSON as `grain_notes,omitempty`.
- Produces: `func offlineReviewRefForTask(cmd *cobra.Command, grain, source, taskRef string) (*store.ReviewRef, error)`.

- [ ] **Step 1: Write the failing tests.**
  - `TestTaskReviewGrainPrefersStartedGrain` and `TestTaskReviewGrainFallsBackWithoutStartedGrain` (Review Focus 4) in the store package.
  - `TestStartRecordsStartedGrain`: start at feature grain writes `started_grain: feature` to task.yml.
  - `TestStaleTaskGrainTaskSubmitsAsTaskGrainAfterGrainMerge`, built from the #146 timeline:
    - alfa starts at task grain; then a commit sets the feature to feature grain.
    - Offline `submit task` with a remote returns `pr_required`, and its message contains `started at task grain`.
    - With `--pr` it records a PR.
  - `TestDependentOfStaleTaskGrainPrerequisiteSaysWaitForMerge`:
    - Bravo depends on alfa, which started at task grain and is now `in_review` with a PR.
    - Bravo's start refusal has `DependencyStartedGrain == "task"`.
    - The CLI human text contains `wait for its PR to merge and sync` and not `assemble`.
  - `TestSubmitFeatureTaskReviewOpenNamesStartedGrain`: the CLI text contains `started at task grain`.
  - `TestReadoutGrainNotesNameStaleTask`: `GrainNotes` contains `rides task grain; its PR must merge before submit feature`.
  - `TestReadoutGrainNotesEmptyForLegacyTask`: a legacy task without the field gives no note.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/store -run TestTaskReviewGrain`, then `go test ./internal/core/mutate -run 'TestStartRecordsStartedGrain|TestStaleTaskGrain|TestDependentOfStale' -timeout 20m`, then `go test ./internal/cli -run 'TestSubmitFeatureTaskReviewOpen'` and `go test ./internal/views -run TestReadoutGrainNotes`.
- [ ] **Step 3: Implement.**
  - **Start:** set `target.StartedGrain` in the `in.Verb == "start" && target.Branch == ""` block, from the resolved grain. Tasks only, not bugs, chores or tweaks.
  - **Task-grain decisions:** every place in `Transition` (check and submit for tasks), and in `lifecycle_cmds.go`, that resolves a task's grain calls `store.TaskReviewGrain`. Find them with `grep -n ResolvedReviewGrain internal/core/mutate/transition.go internal/cli/lifecycle_cmds.go`.
  - **Aggregate lookup:** `aggregateForTask` callers on the task path return no aggregate when the task's started grain is task.
  - **Dependency refusal:** `taskStartBase` sets `DependencyStartedGrain` on a dependency's refusal, and the CLI renders `"Blocked: task %s requires %s, which started at task grain; wait for its PR to merge and sync before starting."`.
  - **`pr_required` for tasks:** the message gains `"; task %s resolves to review grain task (%s)"`. The source label is `started at task grain` for `started`, and otherwise `feature %s in this checkout`, `epic %s in this checkout`, `project setting` or `default`. The feature and epic `offlineReviewRef` text stays unchanged.
  - **`TaskReviewOpenError` rendering:** when the task's started grain is task, append `" Task %s started at task grain, so it takes its own PR and must merge before this submit."`.
  - **Readout `GrainNotes`:** for each feature that resolves to coarse grain, take each non-done task whose `StartedGrain == "task"`. Each note reads `"%s rides task grain; its PR must merge before submit feature %s."`. Render the notes in the readout text after the route advisory.
- [ ] **Step 4: Run the tests and confirm they pass.** Same commands as Step 2.
- [ ] **Step 5: Docs.**
  - `artifact-schema.md` documents `started_grain`.
  - `cli-surface.md`:
    - `:502`: the pr_required text is no longer byte-identical across nouns; give the task form.
    - `:637`: a task started at task grain keeps it through later coarse settings.
- [ ] **Step 6: Commit** with the message `Grain: persist the grain a task started under; guide mixed-grain features (#146)`.

### Task 7: Handoffs, R2 spec, schema 11, generation 53, records

**Files:**
- Modify: `internal/workflow/workflow.go` (`Classify` :257-268)
- Modify: `internal/core/mutate/check.go` (add `AggregateBranch string` to `CheckResult`, filled for coarse-grain feature checks via `aggregateForTask`)
- Modify: `internal/cli/check.go` (`unmetGateDetail` :167 and its caller `humanCheck`)
- Modify: `internal/gen/orientation.go` (body near :347; `orientationSchemaGen` :34 goes to 53 with a "Generation 53 ..." comment line)
- Modify: `internal/skills/execute/SKILL.md` (:84, :313-335), `internal/skills/code-review/SKILL.md` (:354)
- Modify: `internal/store/manifest.go` (`CurrentSchemaVersion = 11`), `internal/core/mutate/migrate.go` (`schemaMigrations`)
- Modify: every test fixture that pins `schema_version: 10`. Find them with `grep -rln "schema_version: 10" internal`, following 3ed2cb49.
- Docs:
  - `docs/spec/artifact-schema.md` (R2 at :375, schema 11 note beside :468)
  - `docs/spec/cli-surface.md:637` (R2)
  - `docs/spec/distribution.md:5,226,298,398`
  - `docs/spec/skill-set.md:5`
  - `docs/tiller-commands.md:7-8`
  - `site/src/content/docs/skills.md:15`
  - `docs/plans/STATUS.md:3,91`
  - `docs/project-history.md` (new top entry)
- Test: `internal/workflow` Classify tests (or `internal/core/mutate/workflow_*_test.go`), `internal/cli/check_test.go`, `internal/core/mutate/migrate_test.go`

The queue entries (`views/queue.go`, `ui/src/lib/attentionFallback.ts`) deliberately stay as they are. The console reads the primary checkout, and Task 4's refusal names the branch for anyone who copies the command.

- [ ] **Step 1: Write the failing tests.**
  - `TestClassifyCoarseHandoffNamesAggregateBranch`: at feature grain with `f.Branch` set, `Reason` contains `on aggregate branch tiller/reviews/features/` and `commit there, then submit`.
  - `TestClassifyTaskGrainHandoffOmitsBranch`.
  - `TestCheckHumanReviewLineNamesAggregate`: `humanCheck` output for a coarse feature contains `on branch tiller/reviews/features/`.
  - `TestMigrate10To11StampsOnly`: tiller.yml goes to `schema_version: 11`; work files are byte-identical.
- [ ] **Step 2: Run them and confirm they fail.**
- [ ] **Step 3: Implement.**
  - **`Classify`:** when the grain is not task and the review node's branch is non-empty, append `"; record each decision on aggregate branch <branch> (in its worktree if checked out there), commit there, then submit"`.
  - **`unmetGateDetail`:** gains `aggregate string`. When it is set, the line ends `" on branch %s"`.
  - **Migration:** `{from: 10, note: "stamp schema_version 11; task started_grain is additive; preserve work, events, and active reviews without drain", apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 11) }}`.
  - **Orientation and the skills (execute step 5, the skeleton, code-review :354):** at feature or epic grain, the human runs the attest on the recorded aggregate branch, in the worktree that holds it, and it is committed there before submit. Tiller refuses it elsewhere.
- [ ] **Step 4: Run the full suite.** Run `go test ./... -timeout 30m`. Expected: PASS. Then run `/verify`.
- [ ] **Step 5: Docs and records.**
  - R2 text at both spec sites: "A recorded aggregate branch locks every review grain change on that feature".
  - Update the schema and generation pins listed under Files.
  - Add a project-history entry for #146 to #150.
  - In STATUS.md, set schema 11 and generation 53.
  - Run `scripts/check-doc-versions.sh`.
- [ ] **Step 6: Commit** with the message `Handoffs name the aggregate branch; schema 11, generation 53 (#146-#150)`.
