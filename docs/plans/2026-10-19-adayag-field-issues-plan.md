# Allen's field-run issues (#146 to #150): verification and resolution plan

Status: VERIFIED 2026-10-19 against main a5c604a4 (19 commits past the filing base ff1f1634). Corrections posted on each issue 2026-10-19. ALL RULINGS SETTLED 2026-10-19 (grill). NOT BUILT.

All five issues are real and unaddressed on main. None of the 19 intervening commits touch the defects; they only shift line numbers. The #147 reproduction test passes on main (the deadlock reproduces), and a throwaway probe reproduced #146.

## The chain

The issues are one field incident plus two independent findings:

- #146 (uncommitted grain change) is the only way into #147's state.
- #147 (hand-built aggregate deadlock) is what pushed the agent to drop an edge.
- #148 (dependency-lock remedy is a dead end) is what blocked restoring it.
- #149 (CLI Next claims in-flight work) and #150 (attest names no checkout) stand alone. Both share a theme with #146: the integration checkout is blind to state that lives in other checkouts and branches.

## Corrections to the issues

Line numbers below are current main.

### #146
- Accurate throughout. Small refinements:
  - The proposed warning is not "the same predicate `evaluateReviewGrainChange` uses". That check compares grain before and after a change. The proposal is a different after-the-fact check.
  - "Nothing detects it" is slightly strong. `submit feature` catches it late (`branch_missing`, or `TaskReviewOpenError` at `submit_node.go:652`), without naming the cause.
  - The proposed start/check/submit warning would not have fired in the field run (the worktree still resolved task grain), and it goes quiet once any sibling start records an aggregate. A probe showed the more common silent path: a sibling start mints a fresh aggregate that lacks the stale task.
  - `prRequiredRefusal` is also reached from `offlineReviewRef` for feature and epic submits.

### #147
- `transition.go:688` is now `:699`.
- `TaskIncludedInAggregate` spans `task_branches.go:195-214`, not `:195-217`.
- `cli-surface.md:635` is now `:637`.
- `review_grain_change.go:125-127` is `:126-128`.
- Understated: the second refusal's remedy names an aggregate that is only set in memory and never written, so following it leads back to the first refusal. That loop is the deadlock.
- New finding: the code locks any grain change once `feature.branch` is set (`review_grain_change.go:126-128`). The specs (`artifact-schema.md:375`, `cli-surface.md:637`) lock only changes entering or leaving epic grain.
- Epic grain has the same gap (`epic.branch` has the same single writer).

### #148
- `cli-surface.md:161` is now `:162`.
- `tiller-commands.md:366-369` is now `:368-371`.
- `project-history.md:736` is now `:847`.
- Link and unlink share one function, `reportDependencyChangeError` (`unlink.go:156-174`). There is no copy in link.
- Missed emitter: `DependencyChangeLockedError.Error()` at `depends.go:83` says "or stage an amendment once work is active".
- The claim "an added edge to finished work changes no start base and no routing":
  - It holds for a task edge.
  - It is false for a feature or epic edge whose target is submitted but not done. Every ready task under it would stop starting.

### #149
- Execute `SKILL.md:31` is now `:33`.
- The analogy "filled in the way the uncommitted-state line is" is wrong. That line is set after `views.Summary` runs and is never an input. The right pattern is an input field such as `ReadoutInput.UnpushedReviews` or `workflow.Input.Eligibility`.
- The console only mutes. It does not fall through to the next eligible work as the issue implies.
- The existing console mute has no test.
- Additional surface: `tiller guide`'s `suggestSkill` (`guide.go:97`) has its own claim wording, and `readout --since` (`correction.go:66`) routes too.

### #150
- Execute `SKILL.md:83` is now `:85`, `:308-330` is now `:313-335`, and `:30` is now `:32`.
- `submit_node.go:162` reads the current checkout's tree, not "the aggregate's". The branch is enforced only at `:168`, so a submit from integration reports missing gates rather than the wrong branch.
- Epic grain has the same problem: the aggregate is `epic.branch`. The proposed trigger "the feature records an aggregate" is wrong there.
- Task grain must be exempt, because feature acceptance runs on integration by design there.
- Since 9816d075 the agent's unit worktree holds the aggregate branch, and Git allows a branch in only one worktree. So "run in a checkout of <branch>" cannot be followed in the primary checkout. The handoff should name the worktree path (`vcs.Git.WorktreeFor`).
- More surfaces print the bare attest command:
  - `recovered_closeout.go:110`
  - `gen/orientation.go:347`
  - `cli/check.go:167` (the line humans copy)
  - code-review `SKILL.md:354`
  - `views/queue.go:372` and `:425`, mirrored in `ui/src/lib/attentionFallback.ts:72`
- Side find: `ui/src/components/__tests__/instrument.test.tsx:1174` uses `tiller attest epic ... --gate`, which the CLI does not accept.

## Resolution pass

### Batch 1: no rulings needed (one branch, one gen bump)

1. **#148 interim.**
   - Drop the amendment remedy from the dependency-lock refusal. Say plainly that the edge cannot change in this state.
   - Files: `internal/cli/unlink.go` (`reportDependencyChangeError`, the `Remedy` field) and `depends.go:83`.
   - Update `depends_cli_test.go:253-293` (rename it off `...WithAmendmentRemedy`).
   - Docs: `cli-surface.md:160,162`, `tiller-commands.md:368-371` (regenerate site cli.md), and `amend/SKILL.md:48`.
2. **#146 messages.**
   - `configure --review-grain` output (`cli/configure.go` `humanConfigure`, only when `res.Changed`) says the change applies to new work only once it is committed to integration, and to commit before starting tasks.
   - The `pr_required` refusal (`lifecycle_cmds.go` `offlineReviewRefForTask`/`prRequiredRefusal`) names the resolved grain and its source via `ResolvedReviewGrainWithSource`. The spec says that text is byte-identical across nouns, so update `cli-surface.md` (around line 502) and the pinning test.
3. **#147 messages.**
   - When a `branch_ref_conflict` names the aggregate's own derived name, say "branch X exists but is not recorded on feature F-y" and point at recovery.
   - The dependency refusal must not name an aggregate that was never recorded.
   - Files: `task_branches.go`, `cli/branch_errors.go`.
4. **#150 handoff text.**
   - At coarse grain only, every attest handoff names the recorded aggregate branch, plus its worktree path when one exists, and says to commit before submit.
   - Surfaces: `workflow/workflow.go:267`, `recovered_closeout.go:110`, `cli/check.go:167`, `gen/orientation.go:347`, execute `SKILL.md:84` and `:313-335`, code-review `SKILL.md:354`.
   - Keep the queue wording (`views/queue.go`, `attentionFallback.ts`) in step.
5. **#149 groundwork.** Add a serve test that pins today's `/api/route` mute before any route change.

Batch 1 changes skills and orientation, so it bumps orientation to gen 53 and adds a project-history entry.

### Batch 2: behavior changes, each gated on a ruling

| Issue | Change | Ruling needed |
|---|---|---|
| #147 | Explicit recovery path that records an existing aggregate | R1, R2 |
| #150 | Grain-aware branch guard on `attest feature` (feature and epic grain; exempt task grain, done or cancelled nodes, and recovered reviews; pass in a linked worktree; reuse `BranchSafetyError` with a new `attest_branch_mismatch` reason) | R3 |
| #149 | Shared route-observation input: an `InFlight` (task ID to branch) field on `workflow.Input`, gathered by a new `readinput` helper over `execution.Load`. CLI readout, correction, `guide` and serve all consume it, and the mute string lives once in views. | R4, R5 |
| #146 | Mixed-grain detection | R6 |
| #148 | Allow adding an edge on started work | R7 |

Recommended order: #147, then #150, then #149, then #146, then #148. #147 removes the main reason anyone needs #148, and #146's detection design depends on #147's remedy.

## Settled rulings (JC, 2026-10-19)

- **R1 (#147):**
  - The recording path is `tiller start task <id> --adopt-aggregate`. It accepts an existing ref only at the derived aggregate name and records it in the same transaction as today. The normal `TaskIncludedInAggregate` check then runs. No `link feature --branch`.
  - Covers epic grain too.
  - Before recording, it checks:
    - the node resolves to coarse grain;
    - no branch is recorded yet;
    - the node is not done, has no PR or `review_ref`, and is not in recovery;
    - the ref exists locally;
    - the name is not any task's `branch` or `review_ref.branch`;
    - the name does not conflict with integration or with other nodes' branches.
  - The plain-start "exists but is not recorded" refusal names the flag.
- **R1 follow-up:** agents may pass `--adopt-aggregate`; it is not human-gated.
  - The refusal tells the agent to inspect `git log <integration>..<ref>` first.
  - The adopt output lists the commits the ref holds beyond integration. The feature review PR is the backstop.
- **R2:** the spec changes to match the code. A recorded aggregate locks every grain change on that feature (`artifact-schema.md:375`, `cli-surface.md:637`). No behavior change; the edit rides batch 1. Allowing feature to task was rejected: keeping `feature.branch` leaves dependency edits locked and lets a stale aggregate be reused silently, and clearing it orphans the ref into #147's refusal.
- **R3 (#150):**
  - `attest feature` refuses, with a new reason `attest_branch_mismatch`, when all of these hold:
    - the grain is coarse;
    - the review node is not done or cancelled;
    - the node is not in recovered-review;
    - an aggregate is recorded (`feature.branch`, or `epic.branch` at epic grain);
    - HEAD is not on it.
  - A linked worktree on the aggregate passes.
  - It covers every `attest feature`, both `--criterion` and `--gate`. Task, bug, chore and tweak attests are not guarded.
  - The message depends on the case:
    - the branch is in a worktree: name the worktree path;
    - it exists locally but isn't checked out: say to check it out;
    - it is missing locally: `git fetch && git switch <branch>`.

    The refusal fires in every case, including when the branch is missing locally.
- **R4 (#149):** an observed in-flight task splits on its observed owner. The CLI and the console behave identically.
  - **Owner is the reader:** mute with "T-x is in flight on <branch>; continue it there". This applies whichever routing arm picked the task. Add a secondary line: "next eligible if you're working in parallel: T-y (claim with ...)".
  - **Owner is someone else:** fall through to the next eligible work with a skip note, like `SkippedQuestionHold`. If nothing remains, say so.
- **R5 (#149):**
  - The observation gather runs on every readout-bearing command, including gates' trailing readouts. That costs about 40 ms on the Acme stand-in. On error it degrades to no observations.
  - The remote-branch caveat ("observations see local refs and worktrees only; fetch to see teammates' branches") goes in the execute skill only, not on the line.
- **R6 (#146):**
  - **Remedy:** a task started at task grain keeps task grain. It takes its own PR or `review_ref`, merges, and syncs before `submit feature`, per `cli-surface.md:637`. The aggregate never absorbs it.
  - **Detection:** `start` persists the resolved grain on `task.yml` (a field like `started_grain`). This is a schema change and a gen bump. Tasks without it fall back to today's resolution, with no migration rewrite.
  - **Guidance only, no new refusals:**
    - `check task` and `submit task` follow the started grain, and the `pr_required` refusal says why.
    - Readout notes "T-x rides task grain; its PR must merge before `submit feature`".
    - A dependent's start refusal says "wait for T-x's PR to merge and sync", not "assemble into the aggregate".
    - The `TaskReviewOpenError` refusal at `submit feature` gains the same explanation.
  - The cross-worktree uncommitted `.tiller/` warning is SKIPPED.
- **R7 (#148):** narrow reversal of the settled rule.
  - `link task --depends-on` may add an edge on a started task only when the target is done.
  - Removing an edge, feature and epic edges, and targets that aren't done stay refused.
  - It is open to agents like the rest of `link`.
  - No dependency amendment op.
  - Spec edits: `cli-surface.md:160-162`, `skill-set.md:71,75`, `artifact-schema.md`, `tiller-commands.md:368-371`, `amend/SKILL.md:48`, and `source-quickstart.md:344`.
