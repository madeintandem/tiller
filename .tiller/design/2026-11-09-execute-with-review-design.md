# Execute with review (shelved design)

Status: SHELVED 2026-11-09. Not built, and no plan to build it yet. This file keeps the thinking from a design grill so a later attempt does not start over. Seven forks were opened. Six were ruled, one of those rulings was then put in doubt by the shelving discussion, and the seventh was left unanswered. Each section below says which is which.

## The idea

A skill, `/tiller-execute-with-review`, wraps a cycle that is currently run by hand: execute a unit of work, code review it immediately, log every finding, and fix every bug of Medium severity or higher on the same branch, so the original PR holds both the implementation and those fixes. The human still merges.

Today the cycle is about three prompts: "execute", "code review X", then "fix the Medium and above on its branch and log the rest".

## Why it was shelved

Partway through the grill, the design had collected enough overrides of existing skills to raise a fair question: was this fighting the architecture, or fighting Tiller's philosophy?

The assessment was neither, for the core cycle. Execute, review, fix, and hand over a PR is a skill doing the agent's reasoning while the human keeps the merge, which is principles 1, 4, and 5 as written. Most of the strain came from one decision, which was making fixed bugs into auto-approved shelf nodes. An agent approving a bug it found in its own code, before any human has looked, is the one real conflict with "the human approves at the gates".

The smaller version below removes that conflict. The remaining question is value. If the annoyance is typing three prompts, a skill saves little. If the annoyance is that the review only happens when someone remembers to ask, and sometimes after the PR is open, the skill earns its place. That is the test to apply before picking this up again.

## Rulings

**Form: a new twelfth skill, explicit invocation only.** It is a thin composer that reads `tiller guide execute` and `tiller guide code-review` for the mechanics and owns only the seams. Orient never suggests it, which preserves the rule that a review is never a suggested next move. No CLI verb and no manifest setting. Rejected: an arm inside `execute`, and a manifest switch that changes `execute` for every contributor. Name: `execute-with-review`, shim `/tiller-execute-with-review`.

**Scope: one review per review surface.** This comes from `execute`, where a unit of work is what one review covers: the task at task grain, the feature or epic at coarse grain, and the bug, chore, or tweak itself for shelf work. Executing a feature at task grain therefore means one review before each task's PR.

**Insertion point: after required mechanical checks are green, before rubric grading and any publication.** The review reads the committed local branch head. Fixes land and checks rerun while the work is still at `check`, and only then does the run grade, push, and open the PR. At coarse grain this means after assembly and `tiller check feature`. The human's first look is the finished surface and nobody attests code the cycle is about to change. The accepted cost is that nothing is pushed until the review finishes.

**Records: everything commits on the review branch.** The review file, its annotations, the proposed nodes, and any amendment ride the PR and reach integration at merge. The run never touches the primary checkout, so a dirty one cannot stall it. If the PR is abandoned, the log goes with it, which is right because the findings were about that code. Standalone `code-review` dispositions keep committing on integration.

**Medium and above get shelf nodes as a record.** First ruled as review rows only, then corrected: each such bug should exist as a node recording that it was found and fixed, and its fix merges through the original PR. Below the threshold, Minor bugs, tweaks, and chores become `proposed` shelf nodes and plan changes become one `proposed` amendment. Rows under Already tracked and Previously declined stay in the review file and are not re-logged.

**Coarse grain: fixes commit directly on the aggregate branch.** Commits name the bug ID and the task IDs they concern. Tasks stay `in_review` with no `resume`. This is an override inside this skill only: human requested changes still return to each task's own branch.

**`code-review` locator change.** Its locator diffs each task's branch, so it does not see commits made only on the aggregate. For feature and epic targets it would add the aggregate's own commits to the located change. See Existing gap below.

## The node mechanism: ruled, then reopened

The ruling was three commands per bug: `tiller add bug`, `tiller approve bug`, and `tiller resolve bug --by <reviewed node>`, with invocation of the skill as standing delegation for the two judgment verbs. The carrier is the reviewed node, the one whose PR this is. A full bug lifecycle on the shared branch was rejected because `start` records a separate `tiller/bugs/<key>` branch, which means a second PR.

The shelving discussion found that `approve` is not needed. The proposal below drops it. The proposal was made but not ruled on before the work was shelved.

## The smaller version

About a page of skill text:

1. Run `execute` until checks are green.
2. Run the `code-review` review arm.
3. For each Medium or higher row, write a regression test that fails on the defect, then run `tiller add bug` and `tiller resolve bug <key> --by <reviewed node>`, then fix it on the PR's branch and annotate the row.
4. Log everything else as `proposed`.
5. Commit the review on that branch and let `execute` finish publishing.

There is no `approve` and no change to the approval model. The only delegation left is that invoking the skill authorizes those `resolve` calls, which needs one sentence in the actor model at `docs/spec/cli-surface.md` near line 1266. Each fixed bug goes from `proposed` to `done` with basis `carried` and the carrier named in the event.

On a run that reviews a bug, chore, or tweak there is no legal carrier, because `resolve` accepts only an epic, feature, or task. There the annotated review row is the record and no node is minted.

## Unanswered fork: which rows get fixed

The proposal was to prove first, then mint. Every Medium or higher row enters the loop, Traced or Suspected, and starts with a failing regression test. If it reproduces, mint, carry, and fix. If it does not reproduce or cannot be tested in the project, make no fix and log a `proposed` bug at its graded severity with the reason in `bug.md`. Alternatives were fixing Traced rows only, or minting first as graded. The threshold would be fixed skill text with no manifest setting. This was not ruled.

## Forks never reached

- **Re-review after fixes.** Whether fixes get a delta review, and a cap on rounds so the loop terminates.
- **Reviewer independence.** The author reviewing its own code in the same context is weak. `AGENTS.md` asks for an independent reviewer, and principle 11 forbids requiring a capability only some harnesses have, so this could only be "a fresh context when the environment allows it".
- **Fixes that exceed the plan.** A fix that would change approved prose, a criterion, or scope, and a fix that fails or that the agent cannot complete. Presumably a recorded question or a stop, as in `execute`.
- **Re-entry.** How an interrupted run detects that a review for this head already exists and picks up mid-loop.
- **Already tracked rows at Medium or above.** Whether the run fixes a bug someone else already logged.
- **Hand-off.** How findings appear in the PR body and in **Notes from this run**, and the existing `code_review` attest line.

## What the code already says

These were read from the source on 2026-11-09. None were exercised end to end.

- `tiller resolve` moves no status and works on a `proposed` or `ready` node (`internal/core/mutate/resolve.go:46`). It accepts only a live epic, feature, or task as carrier (`resolve.go:84`).
- A `proposed` node with a live carrier leaves the triage queue (`internal/views/workitem_queue.go:20`).
- Sync crosses a carried node to `done` from any non-terminal status, in the same run that records the carrier's `done`, with the carrier named in the event (`internal/core/mutate/sync_workitem.go:64`, `:148`). A carried node records no policy violations, since it never ran a gate of its own.
- `start` refuses a node while a live carrier owns it (`internal/core/mutate/transition_workitem.go:71`).
- Branch guards exist on submit and attest. No branch guard was found on `add`, `intake`, or `amend`, so recording these on a review branch should be legal.
- Coarse submit checks that recorded and current task tips are ancestors of the aggregate (`internal/core/mutate/task_branches.go:250`), so extra commits on the aggregate do not break it.
- Reruns on the aggregate depend on the manifest. `tiller init` seeds commented `build` and `tests` templates with `on: [feature, task]` (`internal/cli/init.go:439`). A project that scopes them to `[task]` alone would get no test rerun from `tiller check feature` after a fix, so the skill would need to handle that case.

## Existing gap, independent of this feature

`code-review` misses commits made only on an aggregate branch, such as conflict resolutions. The located diffs for merged work end at each task's head. This is worth fixing whether or not the skill is ever built. It has not been filed as an issue.

## What building it would touch

`internal/skills/execute-with-review/SKILL.md`, the embed and `order` in `internal/skills/skills.go` and its package comment (the catalog goes to twelve), `internal/skills/skills_test.go`, the `code-review` locator text and its test, `docs/spec/skill-set.md`, the actor model in `docs/spec/cli-surface.md`, the orientation generation bump in `internal/gen/orientation.go`, and the site's skills page.
