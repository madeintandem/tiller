# Settle Questions Before Approve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Plan gates refuse while an open question holds the plan, surfaces stop offering that approve, specify settles and records questions before approve, and execute records mid-run questions, builds around them, and asks them together.

**Architecture:** One plan-blocker helper in `internal/gen`, a refusal in the two plan-gate approve arms reusing `BlockedByQuestionsError`, a verb-aware human line, two queue filters (routing and the console read the queue), one orientation bullet, and skill and spec text.

**Tech Stack:** Go, cobra, testify.

**Spec:** `docs/plans/2026-10-22-settle-questions-design.md` (decisions 1 to 9 and "Settled while writing"). Read it first.

## Global Constraints

- The refusal applies only to `approve feature` from `specced` and `approve task` from `proposed`. `approve feature` with `--refresh-basis`, `--review-grain`, or `--assume-question` on a ready or active feature, the idempotent re-approve of a ready feature, `approve epic`, and bug/chore/tweak approval are unchanged.
- A plan's blockers: the union, deduplicated by question id in first-seen order, of `gen.FeatureQuestionBlockers` for the feature and `gen.TaskQuestionBlockers` for each of its tasks whose status is not `cancelled`. Only `gen.QuestionOpen` questions count.
- Refusal order at the feature plan gate: retired epic, then `validateSpec`, then the question refusal, then `approveFeaturePlan`. Nothing is written on refusal.
- The refusal is `*mutate.BlockedByQuestionsError` (reason `open_question_blocker`, exit 1). Its human line for an approve is exactly: `Blocked: <noun> <ref> is held by <n> open question(s): <handles>; answer them with tiller answer question <Q-id> --answer "...", fold the answers into the plan, and retry.` It never mentions `--assume-question`. The `start` line is unchanged.
- `approve task --feature <key>` keeps the batch convention: each held task is a blocked entry, unheld tasks approve, exit 1 if any was blocked.
- Orientation generation stays 54. The orientation text's plan-gate bullet changes.
- No CLI check reads prose. No new flags, fields, or schema.
- Prose (skills, specs, comments): no em dashes; run `grep -n "—"` on every touched doc.
- Commits: `git commit -s`, subject `Area: imperative summary`, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Stage files by name. Build in a worktree.

## Review Focus

1. **`approve feature --refresh-basis` on an active feature** whose one task is held by an open question still succeeds. Test in Task 1.
2. **A question on a cancelled task** does not hold the plan. Test in Task 1.
3. **A question blocking two tasks** appears once in the refusal. Test in Task 1.
4. **An answered question** (status `answered`) does not hold the plan, and `approve` then succeeds. Test in Task 1.
5. **The idempotent re-approve** of a ready feature with an open question on it stays a no-change success. Test in Task 1.

---

### Task 1: Refuse plan gates while a question holds the plan

**Files:**
- Modify: `internal/gen/question_blockers.go`, `internal/core/mutate/transition.go` (the `approve` case's specced arm near `approveFeaturePlan`, and the task approve arm after `guardParentState`), `internal/cli/lifecycle_cmds.go` (`transitionBlockedView`'s `BlockedByQuestionsError` line)
- Test: `internal/gen/question_blockers_test.go`, `internal/core/mutate/plan_gate_questions_test.go` (new), `internal/cli/plan_gate_questions_test.go` (new)

**Interfaces:**
- Produces: `func PlanQuestionBlockers(tree *store.Tree, feature *store.Feature) []QuestionBlocker` in `internal/gen`, per Global Constraints. `BlockedByQuestionsError` gains `Verb string`; the approve arms set `Verb: "approve"`, existing callers leave it empty.

- [ ] **Step 1: Write the failing gen test** `TestPlanQuestionBlockers`: cases for a question on the feature, on its epic, on one task, on two tasks (one entry), on a cancelled task (none), an answered question (none), and no questions (nil).
- [ ] **Step 2: Run** `go test ./internal/gen -run TestPlanQuestionBlockers -v`. Expected: FAIL, undefined.
- [ ] **Step 3: Implement `PlanQuestionBlockers`.**
- [ ] **Step 4: Write the failing mutate tests:**
  - `TestApproveFeatureRefusesHeldPlan`: question on a task, `approve feature` returns `*BlockedByQuestionsError` with `NodeID` the feature, `Verb` `"approve"`, `CurrentState` `"specced"`; the feature and its tasks stay specced/proposed and no event is written.
  - `TestApproveFeatureRefusesEpicQuestion`: question on the epic refuses the same way.
  - `TestApproveFeatureAfterAnswer`: answer the question, `approve` succeeds to ready.
  - `TestApproveFeatureBrokenSpecReportedFirst`: a held plan whose spec fails `validateSpec` returns the spec error, not the question error.
  - `TestRefreshBasisIgnoresQuestions`: active feature, open question on one task, moved basis, `approve --refresh-basis` succeeds.
  - `TestReapproveReadyFeatureWithQuestion`: ready feature with an open question on it, plain `approve` is a no-change success.
  - `TestApproveTaskRefusesHeldTask`: a proposed task under a ready feature with a question on that task refuses with `Verb` `"approve"`; a sibling proposed task with no question approves.
- [ ] **Step 5: Run** `go test ./internal/core/mutate -run 'Approve|RefreshBasis|Reapprove' -v`. Expected: the new tests FAIL.
- [ ] **Step 6: Add the refusal** in the specced arm after `validateSpec` and before `approveFeaturePlan`, and in the task approve arm only when `before == statusProposed`, after `guardParentState`, using `gen.TaskQuestionBlockers`.
- [ ] **Step 7: Write the failing CLI tests** `TestApproveFeatureHeldHumanLine` (human output is the Global Constraints line, contains no `--assume-question`, exit 1) and `TestApproveTasksByFeaturePerTask` (`approve task --feature` with one held and one unheld proposed task: JSON summary has one blocked and one approved, exit 1). Then make them pass in `transitionBlockedView`.
- [ ] **Step 8: Run** `go test ./internal/gen ./internal/core/mutate ./internal/cli`. Expected: PASS.
- [ ] **Step 9: Commit** `Approve: refuse a plan gate while an open question holds the plan`

---

### Task 2: Stop offering an approve that would refuse

**Files:**
- Modify: `internal/views/queue.go` (`queueApproveFeatures`, `queueApproveTaskBatches`), `internal/gen/orientation.go` (the plan-gate bullet at ~830), `internal/skills/orient/SKILL.md` (the plan-gate route line at ~78)
- Test: `internal/views/queue_test.go`, `internal/views/next_enumerate_test.go` or the nearest routing test file

**Interfaces:**
- Consumes: Task 1's `gen.PlanQuestionBlockers`.

- [ ] **Step 1: Write the failing tests:**
  - `TestQueueHidesHeldFeaturePlan`: a specced feature with an open question on one task has no `approve_feature` row, and the question's `answer_question` row is present.
  - `TestQueueApproveTasksRowNeedsOneUnheld`: a ready feature with two proposed tasks, one held, keeps its `approve_tasks` row; with both held, the row is gone.
  - A routing test: the held specced feature's top candidate is the `answer_question` move, not `approve the spec for`.
- [ ] **Step 2: Run** `go test ./internal/views -run 'Queue|Next' -v`. Expected: the new tests FAIL.
- [ ] **Step 3: Filter both queue groups** with a comment citing the #36/#37 rule ("a queue whose commands fail when copied is worse than silence").
- [ ] **Step 4: Rewrite the orientation plan-gate bullet** to: a feature in `specced` awaiting its plan gate with no open question holding its plan is a human move; name `tiller approve feature` and stop. A plan held by an open question routes to the question: name `tiller answer question <Q-id> --answer "..."`, then specify folds the answer into the plan before approve. Keep `orientationSchemaGen` at 54. Mirror the same rule in the orient skill's line 78.
- [ ] **Step 5: Run** `go test ./internal/views ./internal/gen ./internal/skills ./internal/cli`. Expected: PASS (update any test pinning the old bullet text).
- [ ] **Step 6: Commit** `Queue: hide plan approvals an open question holds`

---

### Task 3: Specify and execute guidance, and the specs

**Files:**
- Modify: `internal/skills/specify/SKILL.md`, `internal/skills/execute/SKILL.md`, `docs/spec/cli-surface.md` (approve's verb row and the "Specifying a feature" section, and the queue's approve rows), `docs/spec/skill-set.md` (specify and execute summaries), `docs/tiller-commands.md` (the `approve feature` and `approve task` lines)

**Interfaces:** none. The text states decisions 2 to 9 and what Tasks 1 and 2 built.

- [ ] **Step 1: Specify skill.**
  - A new "Settle questions before approve" section ahead of the readout: the final pass (criteria against each other and `feature.md`, each `task.md` against the code it names, task plans against each other) and the three-way sort of decision 2, with the narrowest-`--blocks` rule and the reason (a feature-wide pin holds the whole approval). A chat-only question is not allowed.
  - Folding answers (decision 3): after the human answers, by `tiller answer question` or in chat (record it on their instruction), write each answer into the prose it changes, re-run `spec feature`, and say in Notes which answer changed which file. `approve` refuses while any question holding the plan is open, and there is no `--assume-question` escape at the plan gate.
  - Readout Notes: list each recorded question by id and title with what its answer would change; the **Next:** line says to answer them when any are open. Replace "Questions and commentary are yours to add freely" so it no longer invites chat-only questions.
- [ ] **Step 2: Execute skill.**
  - Mid-run questions (decision 4), placed after the departure paragraph: a finding past a departure that needs a human decision is recorded with `tiller add question --blocks <narrowest node>`; commit the held task's work and state, leave it, continue with the next legal work. Departures and semantic merge conflicts are unchanged; reword the "Stop and route that change" sentence and the semantic-conflict sentence only as far as needed to agree.
  - The stop rule (decision 5): stop only when no legal work is left in the run or at a stop the run already makes; Notes list every question the run recorded and every open question holding its work, each with id, title, held node, and what the answer would change.
  - Answers after (decision 6): an answer that fits the plan resumes the held task, noted in its commit message; one that changes approved prose takes the correction table in `tiller guide specify` and `approve feature --refresh-basis`.
- [ ] **Step 3: Specs and command reference:** `cli-surface.md` says the specced-to-ready approve and the proposed-to-ready task approve refuse while an open question holds the plan (blocker set per Global Constraints), the refusal is `open_question_blocker` exit 1, the batch reports per task, and the queue hides held approve rows; `tiller-commands.md` says the same in brief; `skill-set.md` mentions the final pass and recorded questions in specify's summary and record-and-continue in execute's.
- [ ] **Step 4: Check** `grep -n "—" internal/skills/specify/SKILL.md internal/skills/execute/SKILL.md docs/spec/cli-surface.md docs/spec/skill-set.md docs/tiller-commands.md` prints nothing.
- [ ] **Step 5: Run** `make lint && make test`. Expected: PASS (fix any skill-text pin in `internal/skills/skills_test.go`).
- [ ] **Step 6: Commit** `Docs: specify settles questions before approve and execute builds around them`
