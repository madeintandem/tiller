# Agent instruction prose: review findings and implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the agent-facing instruction prose (the generated orientation and the embedded skills) so it produces better epics, features, tasks, bugs, chores, and tweaks, says each rule once, reads clearly to a cold reader, and contains no contradictions, dead ends, or unbounded loops.

**Architecture:** Three passes over the same files. First, settle the rules that change agent behavior (run scope, consent, retry, human-only verbs), because later passes move that text. Second, add the missing quality guidance skill by skill. Third, give every shared rule one home and replace its copies with pointers, moving rare procedures into an on-demand `upgrade` skill. The pinning tests in `internal/skills/skills_test.go` and `internal/gen/orientation_test.go` are the ledger that proves no rule was lost: each relocated rule is pinned in its home before its copies are removed.

**Tech Stack:** Markdown skills embedded in Go (`internal/skills`), a Go string constant for orientation (`internal/gen/orientation.go`), testify pins.

**Spec:** The Findings section of this document. The analysis it records exists nowhere else, so executors read it in full. Finding IDs (Q, S, C, I) are cited by every task.

**Status.** Written 2026-11-13 from a full read of the ten skills and the orientation generator at commit `ecccd4a`. Not started. CLI help text, readout renderers, and seeded stubs were not reviewed and are out of scope.

## Global Constraints

- Prose only. The only non-prose changes are the skill registry for the new `upgrade` skill, the orientation generation constant, tests, and docs. No CLI behavior changes.
- No rule is removed without a surviving home. Before deleting a copy, add a `require.Contains` pin for the rule in its home file. Where a duplicate leaves a file, add a `require.NotContains` for its distinctive phrase there, the way `TestOrientationDescribesRankedNext` already lists `gone` strings.
- Line numbers are as of commit `ecccd4a` and shift as tasks land. Locate every edit by its quoted text.
- Skill file paths are `internal/skills/<skill>/SKILL.md`, cited below as `<skill>:<line>`. Orientation is the `orientationBody` constant in `internal/gen/orientation.go`, cited as `orientation.go:<line>`.
- The five-part anatomy stays: every skill keeps `## Reasoning`, `## Choreography`, and `## Hand-off`. The six readout seams (`orient`, `derive-baseline`, `decompose`, `specify`, `execute`, `amend`) keep `## Readout`, the literal `tiller readout <name>`, and the literal `Notes from this run`.
- Skills contain no em dash, no harness-specific tool name, and no `docs/spec` path (`TestSkillsAreNeutralProse`, `TestSkillsStayInsideTheProject`).
- Skill `description` frontmatter stays a single-line plain YAML scalar.
- `orientationHeader` is not reworded. `orientationSchemaGen` moves 61 to 62 exactly once, in Task 17.
- Specs change in the same commit as the prose they describe: `docs/spec/skill-set.md` first, then the site mirror through `site/scripts/sync-docs.mjs`.
- Test fixtures under `internal/store/testdata/` and CLI behavior tests keep their node names. Only agent-facing prose and the pins that quote it change.
- Doc and copy style: plain, concrete, no em dashes.
- Build in a worktree; commit with `-s`; JC fast-forwards main.

## Review Focus

1. **A rule deleted while its home copy is narrower.** The copy that survives must carry every clause of the copies removed. Each dedupe task diffs the removed passages against the home before deleting, and pins the home. Tests in Tasks 11 to 15.
2. **A project still on generation 61.** The binary serves generation 62 skills that point at orientation sections (`## Terms`, the external references rule) an older committed `orientation.md` lacks. Every skill's opening line says what a missing section means and names `tiller upgrade --project-only`. Test in Task 11.
3. **A skill loaded without orientation.** A harness can run `tiller guide execute` without ever reading the pointer block. The same opening line tells the agent to read orientation first. Test in Task 11.
4. **A half-renamed running example.** One file keeping the old epic key teaches two shapes. Task 6 ends with a grep that must return nothing.
5. **Consent wording that creates a new stall.** The stricter go rule must not make an agent re-brief a developer who said "keep going" inside a seam. Orientation keeps "Work already moving inside a seam keeps moving" beside the new rule, and Task 1 pins both.

## Decisions

Each finding below has a resolution. These are the ones that are rulings rather than edits. The plan builds on the stated default; JC can overrule any one before the task that depends on it starts.

- **D1. One home per rule.** Orientation holds every rule that applies in more than one skill. A skill holds only what is particular to it. Procedures needed in one seam live in that seam's skill, and orientation keeps a short statement of the boundary plus a pointer.
- **D2. Run scope.** A run is the work the developer's instruction named, and nothing else. Naming a task, bug, chore, or tweak scopes the run to that unit and its closeout. Naming a feature or epic that reviews at feature or epic grain scopes it to that unit, through its finish line. A go on a ranked row scopes the run to that row's node.
- **D3. Consent.** The top agent move starts only when the developer's reply says to: a go, a yes, or an instruction naming that row. A reply about something else is answered and the list still waits.
- **D4. Retry bound.** After the same gate fails the same way following two different fixes, or a failure does not reproduce when the gate's command is run directly, stop and report.
- **D5. Persisting drift.** A `stale_spec` candidate on the spec or surface leg that already has a mend commit is named once as persisting and is not redrafted. A drift acknowledgment record is a product gap, listed under Follow-ups, not built here.
- **D6. Cancel.** `cancel` on a work node is human-only, even on instruction. `cancel question`, `unlink question`, and `withdraw amendment` are human decisions the agent may record on an explicit instruction naming the target.
- **D7. Gate vocabulary.** There are two human gate verbs, `approve` and `attest`, and no `accept`.
- **D8. Planning commits.** Planning state commits on the integration branch, in the checkout that holds it. An agent never switches a checkout's branch to get there.
- **D9. Questions.** A product decision is a recorded question. A clarification of the request itself is asked in chat.
- **D10. Chore and tweak.** A change to code or tests, however small, is a tweak. A chore changes no code.
- **D11. Contract severity.** Only a violated approved criterion or `preserve` criterion is automatically a blocker. Other spec violations grade by impact.
- **D12. Running example.** The epic key `form-engine` becomes `submit-certification`, and decompose's amendment example epic `hardening` becomes `secure-form-release`. Feature and task keys and all ids stay. JC may pick different names; Task 6 is mechanical either way.
- **D13. Maintenance guidance.** Schema, upgrade, and Homebrew guidance moves to an eleventh skill, `upgrade`, loaded only on an explicit maintenance request, the way `design-brief` and `code-review` are. The fallback, if JC does not want an eleventh skill, is to keep one copy in `orient` and remove the orientation copy.
- **D14. Working plans.** `.tiller/plans/` holds optional working notes committed on the unit's branch. The approved `task.md` stays the plan of record.

## Findings

### Output quality (Q)

- **Q1.** The running example `form-engine/...` appears 97 times across the skills and orientation. It is a subsystem-named epic, which is the shape the prose forbids, and models copy examples over prose. Decompose's amendment example also mints an epic keyed `hardening` (decompose:161).
- **Q2.** Decompose says what `epic.md` carries (decompose:58) but never what a good `feature.md` narrative carries. The guidance is scattered asides (decompose:26, :46, :56).
- **Q3.** Founding features cite no source, so "cover every known initial requirement" (decompose:30) cannot be checked. `tiller add epic` and `tiller add feature` already accept `--source` (`internal/cli/add.go:87`, `:106`); the skill never mentions it.
- **Q4.** The rubric example is unfalsifiable: "Grade whether the copy reads naturally for a homeowner" (specify:83). The agent later grades it itself.
- **Q5.** The advice that the first feature needs a task wiring the build and tests gates sits in execute:143, after the plan is approved. Specify never says it.
- **Q6.** Code-review findings must anchor in changed lines (code-review:38), so required behavior that was never built has nothing to anchor to.
- **Q7.** Every bug block needs Reproduction (code-review:225), the review may run nothing (code-review:82), and a Suspected bug has no traced path (code-review:64). Capture's trigger says "reproduced" (capture:10).
- **Q8.** Missing test coverage lands as a chore (code-review:47), but a chore is "housekeeping without code side effects" (capture:16). Separately, any `feature.md` violation is "always a blocker" (code-review:45) while the grading text limits that to approved criteria (code-review:60).
- **Q9.** Capture has no duplicate check before `tiller add`, though code-review:72 and derive-baseline:50 both have one. It says nothing about what makes a contract section good. "Always pass `--severity`" is followed two sentences later by "leave the bug unrated" (capture:31). Its trigger omits tweaks (capture:10).
- **Q10.** Prohibitions run 4 to 11 per 1,000 words, highest in design-brief and derive-baseline. Many are single-incident patches a stronger model applies literally and broadly.

### Length (S)

Each row is one rule set told in several places. Word counts are measured.

| ID | Content | Told in | Words |
|---|---|---|---|
| S1 | Ranking tiers and move catalogue | orientation.go:769-933; orient:32-36, 68-93 | 2,170 + 2,197 |
| S2 | Correction routing, move-feature rules | orientation.go:681-767; orient:99-114; specify:217-237; decompose:242-259; amend:50-54 | 930 + 627 + 757 + 433 + 345 |
| S3 | Publication sequence | orientation.go:349-422; execute:78-103; orient:24, 64-66 | 904 + 1,451 |
| S4 | Branches and worktrees | orientation.go:230-292; execute:22-34, 54-76; orient:46-48 | 789 + 1,932 + 178 |
| S5 | Schema 6, 8, 9 notes, upgrade, Homebrew | orientation.go:293-348; orient:50-62 | 610 + 624 |
| S6 | Transfer, missing aggregate recovery | orientation.go:619-643, 400-414; execute:149-159, 119-135; orient:42; amend:30 | 271 + 385 + 335 |
| S7 | Readout boilerplate | six skills' `## Readout`; orientation.go:453-472 | about 2,100 + 300 |

Deduplication is lossless only if the surviving home is loaded. Orientation arrives through an `AGENTS.md` pointer and skills through `tiller guide`, so neither is guaranteed (Review Focus 2 and 3).

### Clarity (C)

- **C1.** Coined terms are never defined in one place: seam, riding, coarse, aggregate, review ref, carrier, operative question, live, rollup, closeout, finish line, accepted basis, frontier, held.
- **C2.** Over-compressed paragraphs: execute:50 packs about a dozen rules into 378 words, decompose:56 covers a dozen topics in 307, and orient:24 is slash-joined noun stacks.
- **C3.** Structure hides steps. Execute:137-147 is six unrelated paragraphs under the "Missing aggregate review after merge" heading. Specify's question pass happens before `tiller spec feature` but sits after Choreography (specify:239), with a second pre-spec pass at specify:160. Decompose:54 opens "Only then" with no antecedent.
- **C4.** Residue from earlier versions: "This skill is the thin one" (execute:16, the longest skill); "What the old notes described" (orient:89); "Workflow administration versus oversight ... sink epic" (orientation.go:765, amend:56); "Never tell the developer the frontier shows only their own work" inside the closeout bullet (orientation.go:846); six "in this build" hedges (decompose 2, derive-baseline 2, specify 1, orient 1).
- **C5.** The External references rule is about 110 words in orient:126. Four skills and orientation point to it, so specify must load orient to read it.
- **C6.** Scattered "there is no `tiller X`" notes patch single guesses. Orientation has no general rule to confirm an unfamiliar command with `--help`.

### Contradictions, loops, and dead ends (I)

- **I1.** `cancel` is "human-only ... rather than run it" (orientation.go:184), but `cancel question` is "run only on their instruction" (orientation.go:869, specify:249, execute:50).
- **I2.** Derive-baseline:150 and orientation.go:575 say there are no baseline drift markers or touched-doc sync notices. Mend-drift:26 and orientation.go:975 describe both, and the code has both (`internal/cli/sync.go:800`, `internal/core/drift/drift.go`).
- **I3.** "`approve` is the only gate verb ... there is no `accept`" (decompose:263, specify:275), yet `attest` is the acceptance gate throughout execute.
- **I4.** Code-review:137 says "switch to the integration branch first"; execute:32 says stop "rather than switching its branch". Decompose, specify, capture, amend, and mend-drift say "directly on the integration branch" and nothing about a checkout that is elsewhere.
- **I5.** "Never a line of chat" (specify:270, execute:401) against "Questions and commentary are yours to add freely" (decompose:239, amend:197). Amend:24 and :28 say "ask".
- **I6.** Shelf claims list bug and chore but not tweak (orientation.go:647). "Owners" and "acting owner" survive the contributor rename (orientation.go:1012, derive-baseline:146).
- **I7.** Run scope is undefined. "Continue with the next legal work in the run ... stop only when the run has no legal work left" (execute:50), "without another go" (execute:111), and the Up next list "is for you mid-run" (execute:386).
- **I8.** Consent by absence: "Start it in your next turn, once the developer's reply does not redirect" (orient:32, orientation.go:799). Execute authorizes pushes and PRs (execute:20), so any non-redirecting reply can end in a published PR.
- **I9.** "Fix the work and try again, do not escalate" (orient:118, orientation.go:423) has no convergence bound. Execute:141 lists blockers but not a gate that stays red.
- **I10.** A spec-leg or surface-leg `stale_spec` persists after its mend (mend-drift:36). Orient runs `tiller drift` every run and routes any candidate to mend-drift (orient:83, :157). No acknowledgment exists, so every session re-enters mend-drift for the same candidate.
- **I11.** A correction inside an existing feature's scope becomes a task (decompose:40), but `--add-task` refuses under a draft or specced feature (decompose:80). Recovery says "moving the add to where it belongs" (decompose:92) without saying where.
- **I12.** Code-review's disposition borrows decompose's amendment recording, which ends in a mandatory `tiller readout amend`; code-review:363 does not say whether that applies. It also writes a distillation "outside the repository" with no location (code-review:129).
- **I13.** Orient "lays nothing down" (orient:171) but records deferred items as questions (orient:192), with no identity or commit steps.
- **I14.** Execute:56 asks for a dated plan under `.tiller/plans/` for "multi-task or unusually risky work" with no branch, commit, or relation to `task.md`.

---

## Part 1: Rules that change behavior

These land first because Parts 2 and 3 move the text they touch.

### Task 1: Run scope and consent (I7, I8; D2, D3)

**Files:**
- Modify: `internal/gen/orientation.go` (How to work, lines 476-486; Ranked next, lines 797-803)
- Modify: `internal/skills/orient/SKILL.md` (lines 22, 32, 192)
- Modify: `internal/skills/execute/SKILL.md` (lines 20, 50, 111, 386)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Change the pins.** In `TestOrientationDescribesRankedNext`, replace the pinned string ending "once the developer's reply does not redirect." with "Start the top agent move in your next turn only when the developer's reply says to: a go, a yes, or an instruction naming that row." and add "Work already moving inside a seam keeps moving". Move the old sentence to the `gone` list. Add `TestExecuteDefinesTheRun` pinning "A run is the work the developer's instruction named, and nothing else." in execute. Where `TestOrientRelaysTheRankedList` pins the old sentence in orient, change it the same way.
- [ ] **Step 2: Run** `go test ./internal/gen/ ./internal/skills/ -run 'RankedNext|DefinesTheRun|RelaysTheRankedList'`. Expected: FAIL.
- [ ] **Step 3: Write the consent rule** in orientation.go and orient:32, replacing "Then start the top agent move unless the developer redirects. Start it in your next turn, once the developer's reply does not redirect.":

  > Then end the reply. Start the top agent move in your next turn only when the developer's reply says to: a go, a yes, or an instruction naming that row. A reply that asks something else is answered, and the list still waits for its go. A reply naming a different row or other work redirects you to that.

  Align orient:22 and orient:192 ("lets the top agent move start") to the same wording.
- [ ] **Step 4: Write the run definition** as a new paragraph after execute:20:

  > A run is the work the developer's instruction named, and nothing else. Naming a task, bug, chore, or tweak scopes the run to that unit and its closeout. Naming a feature or epic that reviews at feature or epic grain scopes it to that unit: every task in it you can lawfully claim, in dependency and soft order, through its finish line. A go on a ranked row scopes the run to that row's node. When the run's work is done or held, stop with the readout.

  In execute:111, change "continuing eligible approved work without another go" to "continuing the unit's eligible work without another go". In execute:386 and orientation.go:459-461, replace "that list is for you mid-run" with "that list shows what the tree now allows; inside your run's scope it is your queue, and outside it, it is not authorization".
- [ ] **Step 5: Run** the same tests. Expected: PASS. Then `go test ./internal/skills/ ./internal/gen/`.
- [ ] **Step 6: Commit** `git commit -s -m "Skills: define a run and require a go for the top agent move"`.

### Task 2: Retry bound and persisting drift (I9, I10; D4, D5)

**Files:**
- Modify: `internal/gen/orientation.go` (recovery bullet, lines 423-434)
- Modify: `internal/skills/execute/SKILL.md` (line 141), `internal/skills/mend-drift/SKILL.md` (line 36), `internal/skills/orient/SKILL.md` (line 83)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestOrientationBoundsRetries` pins "after two different fixes" in orientation. `TestMendDriftRecognizesAPersistingCandidate` pins "persists by design" and "git log --oneline --grep='^mend-drift:'" in mend-drift, and "persists by design is not a move" in orient.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Append to the recovery bullet** in orientation.go, after "do not escalate":

  > Stop and report when the same gate fails the same way after two different fixes, or when a failure does not reproduce when you run the gate's own command directly: name the gate, the failure, what you tried, and the evidence commits.

  Add "a gate that stays red under the retry bound in `.tiller/orientation.md`" to the blocker list in execute:141.
- [ ] **Step 4: Add to mend-drift**, at the end of the paragraph at line 36:

  > Before drafting for a `stale_spec` candidate on the spec or surface leg, check whether it was already mended: `git log --oneline --grep='^mend-drift:' -- <the candidate's files>`. A mend commit newer than the candidate's `since` means it persists by design. Do not redraft it. Name it once in your notes as persisting, with the commit that mended it.

  Add to orient:83: "A candidate mend-drift already answered that persists by design is not a move."
- [ ] **Step 5: Run** the tests. Expected: PASS.
- [ ] **Step 6: Commit** `git commit -s -m "Skills: bound gate retries and stop re-mending a persisting drift candidate"`.

### Task 3: Human-only verbs, gate vocabulary, questions (I1, I3, I5; D6, D7, D9)

**Files:**
- Modify: `internal/gen/orientation.go` (lines 177-188, 683-687, 869-872)
- Modify: `internal/skills/decompose/SKILL.md` (lines 239-240, 263), `internal/skills/specify/SKILL.md` (lines 249, 270-271, 275), `internal/skills/amend/SKILL.md` (lines 24, 28, 197-198), `internal/skills/capture/SKILL.md` (line 77), `internal/skills/execute/SKILL.md` (lines 50, 401-402), `internal/skills/orient/SKILL.md` (lines 38, 79, 114)
- Test: `internal/skills/skills_test.go`, `internal/gen/orientation_test.go`

- [ ] **Step 1: Add pins.** Extend `TestHandoffsDisambiguateApproveAccept` to also require "`attest` records the human's acceptance" and to refuse "the only gate verb". Add `TestOrientationSeparatesCancelWorkFromCancelQuestion` pinning "`cancel` on a work node is human-only" and "`cancel question`, `unlink question`, and `withdraw amendment`". Add `TestQuestionsAreRecordedNotChatted` pinning "A clarification of the request itself" in orientation and refusing "Questions and commentary are yours to add freely" in decompose and amend.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Rewrite the cancel sentence** at orientation.go:184. Confirm the nouns with `tiller cancel --help` first.

  > `cancel` on a work node is human-only: hand the exact `cancel` command to the human rather than run it, even on instruction. `cancel question`, `unlink question`, and `withdraw amendment` are the human's decisions that you may record on an explicit instruction naming the target, as with `approve`.

  Make orientation.go:685, capture:77, and orient:38 and :114 say "cancel on a work node". Leave the four question-cancel passages as they are; they now agree.
- [ ] **Step 4: Rewrite the vocabulary trap** in decompose:263 and specify:275, keeping the four already-pinned phrases:

  > One vocabulary trap at this gate: there are two human gate verbs and neither is `accept`. `approve` green-lights a proposed plan to start. `attest` records the human's acceptance of a criterion or an attestation-backed gate on built work. There is no `accept`: completed work crosses to done when the human merges and `tiller sync` observes it, so when someone says "accept this", translate by the node's state (a proposed plan takes `approve`, a pending criterion takes the human's `attest`, reviewed work takes the merge), never guess.

- [ ] **Step 5: Add the question rule** to orientation's rules list:

  > A product decision only the human can make (behavior, acceptance, scope, or order) is recorded with `tiller add question`, never asked only in chat, where no gate and no later session can see it. A clarification of the request itself (which node, which document, which of two readings) is asked in chat.

  In decompose:239 and amend:197, change "Questions and commentary are yours to add freely" to "Commentary is yours to add freely" and add "a question for the human is a recorded question". In amend:24 and :28, change "ask" to "record a focused question about". Shorten specify:270-271 and execute:401-402 to the same single clause.
- [ ] **Step 6: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 7: Commit** `git commit -s -m "Skills: one rule each for cancel, gate verbs, and recorded questions"`.

### Task 4: Where planning commits land (I4, I13, I14; D8, D14)

**Files:**
- Modify: `internal/gen/orientation.go` (commit bullet, lines 206-229)
- Modify: `internal/skills/code-review/SKILL.md` (line 137), `internal/skills/derive-baseline/SKILL.md` (line 76), `internal/skills/execute/SKILL.md` (lines 32, 56), `internal/skills/orient/SKILL.md` (lines 136, 171, 192, choreography block)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestOrientationNeverSwitchesACheckoutForPlanningState` pins "Never switch a checkout's branch to get there." in orientation and refuses "switch to the integration branch first" in code-review. `TestOrientCommitsTheQuestionItRecords` pins `tiller add question` inside orient's Choreography. `TestExecuteWorkingPlanIsNotThePlanOfRecord` pins "stays the plan of record" in execute.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Add to the commit bullet** in orientation, after "so commit them directly":

  > Planning state commits on the integration branch, in the checkout that holds it. Never switch a checkout's branch to get there. When this checkout is on another branch, use the worktree that has the integration branch checked out (`git worktree list`), or add one and join inside it. Commit there only when it holds no changes you did not make; otherwise stop and name what is in the way.

- [ ] **Step 4: Align the skills.** Code-review:137: replace "If the checkout sits on a task branch, switch to the integration branch first." with "When this checkout is not on the integration branch, follow the planning-commit rule in `.tiller/orientation.md`; the review file is untracked, so move it to that checkout first." Derive-baseline:76: replace "Inspect the current branch, index, and working-tree changes before switching or staging" with the same pointer, keeping the sentences about preserving unrelated work. Execute:32 becomes a one-sentence pointer to the rule. Decompose, specify, capture, amend, and mend-drift need no edit: their "directly on the integration branch" now reads against the orientation rule.
- [ ] **Step 5: Give orient's question a choreography.** Change orient:171 "orient lays nothing down" to "orient lays no work down". Add to the choreography block, before `tiller readout orient`:

  ```
  # only when the briefing defers a decision: record it, then commit exactly its paths
  tiller add question "<statement>" --title "<short title>" [--blocks <node>]
  git add -- <question-ledger-and-event-paths>
  git commit --only -m "orient: record <short title>" -- <same-exact-paths>
  ```

- [ ] **Step 6: Replace the plans sentence** at execute:56 ("For multi-task or unusually risky work ... approved `task.md`."):

  > For a coarse unit of several tasks, or unusually risky work, you may write a dated working plan under `.tiller/plans/` before coding and commit it on the unit's branch with the work. It is working notes: each task's approved `task.md` stays the plan of record and wins where they differ.

- [ ] **Step 7: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 8: Commit** `git commit -s -m "Skills: planning commits never switch a checkout's branch"`.

### Task 5: Stale statements (I2, I6)

**Files:**
- Modify: `internal/gen/orientation.go` (lines 575-576, 647, 1012)
- Modify: `internal/skills/derive-baseline/SKILL.md` (lines 146, 150)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** Refuse "No automatic baseline drift" in orientation and "There are no automatic baseline drift markers" in derive-baseline. Require "`tiller claim tweak <key>`" in orientation's shelves paragraph. Refuse "the owners, the inputs" in orientation and "acting owner" in derive-baseline.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Edit.** Delete orientation.go:575-576 "No automatic baseline drift or touched-doc sync behavior is implied." Replace the last sentence of derive-baseline:150 with: "After confirmation, `tiller drift` reports `stale_design` when tracked feature work crosses to done on a described path and `shifted_design` on request, and `tiller sync` may print an `as-is docs touched` notice. `mend-drift` owns those." Add `tiller claim tweak <key>` at orientation.go:647. Change "the owners" to "the contributors" at orientation.go:1012 and "acting owner" to "acting contributor" at derive-baseline:146.
- [ ] **Step 4: Run** the tests. Expected: PASS.
- [ ] **Step 5: Commit** `git commit -s -m "Skills: correct stale baseline-drift and contributor wording"`.

## Part 2: Output quality

### Task 6: Rename the running example (Q1; D12)

**Files:**
- Modify: every `internal/skills/*/SKILL.md`, `internal/gen/orientation.go`
- Modify: `internal/skills/skills_test.go`, `internal/gen/orientation_test.go`, `internal/cli/init_orientation_test.go` (only pins that quote the prose)
- Modify: `docs/spec/skill-set.md` (33 lines quote the example)

- [ ] **Step 1: Replace** `form-engine` with `submit-certification` in the files above. Leave `form-engine-rework` (amend's amendment key) and the prose "form engine" in amend's example title alone: that example is a technical restructure and reads correctly. In decompose's amendment example, replace epic key `hardening` with `secure-form-release`.
- [ ] **Step 2: Fix the one sentence the rename makes odd.** Decompose:167 "a statable contract serving the EXISTING form-engine epic's bounded outcome" keeps its meaning with the new key; reread the comment and adjust only if it no longer parses.
- [ ] **Step 3: Verify nothing is half-renamed.** Run `grep -rn 'form-engine' internal/skills internal/gen/orientation.go | grep -v 'form-engine-rework'`. Expected: no output. Run `grep -rn 'hardening' internal/skills/decompose`. Expected: no output.
- [ ] **Step 4: Run** `go test ./internal/skills/ ./internal/gen/ ./internal/cli/`. Expected: PASS. A failure in a CLI behavior test means a fixture name was renamed by mistake; revert that file.
- [ ] **Step 5: Commit** `git commit -s -m "Skills: the running example is a delivery increment, not a subsystem"`.

### Task 7: Decompose (Q2, Q3, I11, C2, C3)

**Files:**
- Modify: `internal/skills/decompose/SKILL.md` (lines 40, 54, 56, 58, 92, founding choreography 103-110, Readout 233-236)
- Test: `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestDecomposeSaysWhatAFeatureNarrativeCarries` pins "It is narrative, not a specification". `TestDecomposeCitesFoundingSources` pins "--source" inside the founding choreography block and "coverage map" in the Readout section. `TestDecomposeRoutesACorrectionInsideAnUnapprovedFeature` pins "When the feature is still `draft`" and refuses "moving the add to where it belongs".
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Add the feature narrative paragraph** after decompose:58:

  > Write each feature's narrative to the same standard. `feature.md` says who gets what outcome when the feature is done, what is in this increment and what is deliberately left to a later one, which existing behavior or code it touches, why each recorded prerequisite is one, and what detail is deferred to `specify`. It is narrative, not a specification: no criteria, no task list, no interface detail.

  Remove the three asides this replaces only where they repeat it (the "name that deferred detail in its narrative" clause at :26 stays, since it is the instruction's trigger).
- [ ] **Step 4: Add founding traceability.** Confirm the accepted form with `tiller add feature --help`. In "Recording a founding design", add: "Pass `--source` on every `tiller add epic` and `tiller add feature`, citing the design doc and section the node was drawn from, so the human can check coverage." Add `--source .tiller/design/<doc>.md#<section>` to each add in the founding choreography. In the Readout notes list, add "a coverage map: each requirement-bearing section of the inputs and the features that carry it, and anything left unmapped with the reason".
- [ ] **Step 5: Close the dead end.** Confirm that plain `tiller add task` is legal under a specced, unapproved feature. Then add to decompose:40, after "A task is added only under a feature that is already approved.":

  > When the feature is still `draft`, write the correction into its `feature.md` narrative, so `specify` plans it. When it is `specced` and unapproved, correct its plan directly by the route `tiller guide specify` gives for an unapproved plan. Either way the change is outside the amendment's operations, so cite the finding for it in the amendment's narrative.

  In decompose:92, replace "each cleared by moving the add to where it belongs" with "each cleared by the placement rules in **What becomes what**".
- [ ] **Step 6: Fix structure.** Decompose:54: replace "Only then identify actual execution prerequisites." with "Once the tree's shape is chosen, identify actual execution prerequisites." Split decompose:56 into four paragraphs at its existing topic boundaries (dependencies and change type; surfaces; ownership; commit and stop) without changing a sentence.
- [ ] **Step 7: Run** `go test ./internal/skills/`. Expected: PASS.
- [ ] **Step 8: Commit** `git commit -s -m "Decompose: feature narrative shape, founding sources, and the unapproved-feature route"`.

### Task 8: Specify (Q4, Q5, C3)

**Files:**
- Modify: `internal/skills/specify/SKILL.md` (lines 26, 28, 48, 83, 160, 239-251, choreography comment at 187)
- Modify: `internal/skills/execute/SKILL.md` (line 143)
- Test: `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestSpecifyRubricStatesPassConditions` pins "so two graders reach the same verdict" and refuses "Grade whether the copy reads naturally for a homeowner." `TestSpecifyPlansGateWiring` pins "still a commented template" in specify. `TestSpecifySettlesQuestionsBeforeChoreography` asserts the index of "## Settle questions before approve" is less than the index of "## Choreography".
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Fix the rubric.** Add to specify:26, after the sentence introducing `eval`-kind rubrics: "A rubric states its pass conditions as observable checks, so two graders reach the same verdict." Replace C4's binding in the example with:

  ```yaml
  binding: { type: eval, run: "Pass only if all hold: every regulatory term is defined on first use; the copy states what the homeowner must do next; no sentence runs past 30 words. Otherwise fail, naming the first condition that fails." }
  ```

- [ ] **Step 4: Plan gate wiring.** Confirm the manifest keys in a fresh `tiller init` scratch project. Add to specify:28: "When the build or tests gate `run` in `.tiller/tiller.yml` is still a commented template, the feature that stands up the project plans a task that sets it; no later task can pass `tiller check` before then." Shorten execute:143 to: "Before `tiller check` can run the build and tests gates, their `run` commands must be set in `.tiller/tiller.yml`. Specify plans that task in the feature that stands up the project; when you find a gate still unconfigured, that is missing machine work to repair, not a human wait. The evals gate is already active at feature grain."
- [ ] **Step 5: Reorder.** Move the whole "## Settle questions before approve" section to sit directly after "## Grounding claims about code". Move the paragraph at specify:160 ("Before running `tiller spec feature`, re-read each cited claim ...") to be the first paragraph of that section, so the one final pass has two steps in order: verify cited claims, then sort every finding. Text is otherwise unchanged. Specify:48 references `C5` before the example defines it; add "(in the example below)".
- [ ] **Step 6: Run** `go test ./internal/skills/`. Expected: PASS.
- [ ] **Step 7: Commit** `git commit -s -m "Specify: falsifiable rubrics, planned gate wiring, one final pass in order"`.

### Task 9: Code-review (Q6, Q7, Q8, I12; D10, D11)

**Files:**
- Modify: `internal/skills/code-review/SKILL.md` (lines 38, 45, 47, 48, 64, 129, review template 224-228, Hand-off 363)
- Test: `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestCodeReviewFindsWhatWasNeverBuilt` pins "Anchors: absent". `TestCodeReviewReproductionsAreDerivedByReading` pins "derived by reading, not run". `TestCodeReviewClassifiesCoverageAsATweak` pins "tweak for missing coverage" and refuses "chore for missing coverage". `TestCodeReviewDispositionHasNoReadout` pins "no `tiller readout amend` follows" in the disposition Hand-off.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Admit omissions.** Add to code-review:38, after "anchor every finding in lines this node's work changed": "The exception is required behavior the change never built: cite the criterion or `feature.md` line that requires it, write `Anchors: absent`, and set Found in to the feature."
- [ ] **Step 4: Make reproductions honest.** Add to code-review:64: "Either way the Reproduction is derived by reading, not run. For a Traced bug it is the path through the code, step by step. For a Suspected bug it is the trigger you believe and the step you could not prove." In the template, change `**Reproduction** <steps>` to `**Reproduction** <derived by reading, not run: the path, or the believed trigger and the unproven step>`.
- [ ] **Step 5: Align classification and severity.** Code-review:45: "Bug; a violated approved criterion or `preserve` criterion is a blocker, anything else grades by impact". Code-review:47: "Important bug for a test that proves nothing; tweak for missing coverage of a risky path". Code-review:48: "Tweak; chore when only comments or docs change".
- [ ] **Step 6: Settle the disposition's edges.** Code-review:129: replace "Write a distillation outside the repository named" with "Write a distillation in a temporary directory outside the repository, so the project's only copy is the one `tiller intake` lands, and delete the temporary file after intake. Name it". Add to the disposition Hand-off: "A disposition is not a lifecycle seam: no `tiller readout amend` follows a logged plan change, though decompose's own route ends in one."
- [ ] **Step 7: Run** `go test ./internal/skills/`. Expected: PASS.
- [ ] **Step 8: Commit** `git commit -s -m "Code review: omissions, unrun reproductions, and consistent kinds and severity"`.

### Task 10: Capture (Q8, Q9; D10)

**Files:**
- Modify: `internal/skills/capture/SKILL.md` (lines 10, 16, 20, 31)
- Modify: `internal/gen/orientation.go` (lines 578-584)
- Test: `internal/skills/skills_test.go`

- [ ] **Step 1: Add pins.** `TestCaptureChecksForDuplicates` pins "Before adding, read the shelves". `TestCaptureSeverityRuleDoesNotContradictItself` refuses "Always pass `--severity`". `TestCaptureDrawsTheChoreTweakLine` pins "A change to code or tests, however small, is a tweak." in capture and orientation.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Edit the trigger and kinds.** Capture:10: "a concrete defect or maintenance obligation" becomes "a concrete defect, maintenance obligation, or minor adjustment". Capture:16 and orientation.go:578-581: append "A change to code or tests, however small, is a tweak."
- [ ] **Step 4: Add the duplicate check** as a new paragraph before capture:20:

  > Before adding, read the shelves (`tiller ls bugs`, `tiller ls chores`, `tiller ls tweaks`), the open questions, and the proposed amendments. A finding that matches an open item is not added again: name the existing node to the developer. A defect that returns after its bug reached done is a new bug that says `regression of <id>`.

- [ ] **Step 5: Say what a good contract section is.** Add to capture:20:

  > Reproduction gives the smallest steps from a named starting state, with the commit or version where it was observed. Expected names its source: a criterion, a spec line, or the rule the developer stated. Actual is what was observed, with error text verbatim. Fixed when and Done when are conditions a test or a reader can check, never "the bug is fixed".

- [ ] **Step 6: Fix the severity sentence** at capture:31: replace "Always pass `--severity` when filing a bug. When its impact can't be known yet, leave the bug unrated" with "Pass `--severity` whenever the impact is known. When it cannot be known yet, omit it".
- [ ] **Step 7: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 8: Commit** `git commit -s -m "Capture: duplicate check, contract quality, and one severity rule"`.

## Part 3: One home per rule

Every task in this part follows the same four steps, called the move procedure below:

1. List each distinct rule in the passages being removed.
2. Confirm the home states every one, adding any clause it lacks.
3. Pin each rule in its home, and add `require.NotContains` for one distinctive phrase per removed passage.
4. Delete the copies and leave a one-sentence pointer.

### Task 11: Orientation becomes the home for shared rules (C1, C5, C6, S7; D1)

**Files:**
- Modify: `internal/gen/orientation.go`
- Modify: every `internal/skills/*/SKILL.md` (opening line; `## Readout` in the six readout seams; orient's `## Recovery protocol`, `## External references`, `## Node references`)
- Test: `internal/skills/skills_test.go`, `internal/gen/orientation_test.go`

- [ ] **Step 1: Add pins.** `TestEverySkillAssumesOrientation` requires, in each skill, "Read `.tiller/orientation.md` first if it is not in context" and "`tiller upgrade --project-only`". `TestOrientationDefinesItsTerms` requires `## Terms` and each term below as a bolded entry. `TestOrientationCarriesTheExternalReferencesRule` pins "Never guess a URL" in orientation and refuses "## External references" in orient. Update `TestOrientCarriesNodeReferenceConvention` and `TestOrientStatesTheAddressingRuleTheCLIActuallyHas` to read orientation.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Add the opening line** under each skill's title:

  > This skill assumes `.tiller/orientation.md`. Read `.tiller/orientation.md` first if it is not in context. If it lacks a section this skill names, it is stale: tell the developer the project needs `tiller upgrade --project-only`.

- [ ] **Step 4: Add `## Terms`** to orientation, after "What Tiller is". One line per term. Verify each definition against `docs/spec/artifact-schema.md` and `docs/spec/cli-surface.md` before writing it; the drafts below are from the skills' usage.
  - **seam**: a point in the lifecycle where one skill's work begins and ends.
  - **live**: not cancelled.
  - **shelf**: the standing list of bugs, chores, or tweaks.
  - **frontier**: the live draft features whose dependencies are settled.
  - **review grain, coarse**: where review lands (`task`, `feature`, or `epic`); coarse means feature or epic.
  - **unit of work**: what one review covers.
  - **run**: the work the developer's instruction named (D2).
  - **aggregate**: the feature or epic review branch that submitted task branches merge into at coarse grain.
  - **riding task**: a task whose review rides its feature's or epic's review; it submits a review ref, not its own PR.
  - **review ref**: the branch and commit a submit records when the node has no PR of its own.
  - **finish line**: at coarse grain, every live riding task `in_review` or `done`.
  - **closeout**: the work that carries a reviewed unit to done after its merge or finish line.
  - **rollup**: a feature's or epic's status computed from its children by `tiller sync`.
  - **accepted basis**: the spec, contract, or task prose pinned at approval; `active_basis` is drift from it.
  - **operative question**: an open question that currently holds a node, directly or through an ancestor.
  - **held draft**: a node an open amendment created and holds inert until approval.
  - **carrier, carried**: a shelf node is carried while other live work, its carrier, is recorded as resolving it.
- [ ] **Step 5: Move the external references rule** (move procedure). Replace orientation.go:174-176 with the full text of orient:126. Delete orient's `## External references`. In decompose:20, specify:14, amend:16, and capture:14, change "in `tiller guide orient`" to "in `.tiller/orientation.md`".
- [ ] **Step 6: Add the general command rule** to orientation's rules list: "Confirm an unfamiliar command or flag with `tiller <command> --help` before running it. Never guess one." Then delete the notes it covers that carry no other fact: orient:167 (`tiller ls plans`), and "or invent a `--dry-run` flag" at amend:22 and orientation.go:696. At execute:147 keep "evidence is recorded by `tiller check` and `tiller eval`" and drop "There is no `tiller add evidence`;". Keep the notes that state a design fact: no epic eval, check, or attest; no readout for `design-brief` or `code-review`.
- [ ] **Step 7: Collapse the three orient sections that restate orientation** (move procedure). `## Recovery protocol` becomes "Follow the recovery protocol in `.tiller/orientation.md`." plus the two skill-specific limits already there. The command-position addressing paragraph in `## Node references` moves into orientation's naming bullet; the bug, chore, tweak, and severity paragraph is deleted as a copy of orientation.go:578-593. Fix `TestOrientCarriesNodeReferenceConvention`'s pin "This convention is inherited by every skill" to read orientation.
- [ ] **Step 8: Shrink the six `## Readout` sections** (move procedure). Each keeps: its `tiller readout <name>` command and `--since` form, the sentence "Reproduce the block verbatim and last, per the readout rule in `.tiller/orientation.md`.", its own **Notes from this run** list, and "Name nodes by key path with the id in parentheses." Remove the repeated rationale ("The block is Tiller's, not yours ...", "an id alone sends the developer off ..."), which orientation.go:453-472 and :435-448 already state.
- [ ] **Step 9: Run** `go test ./internal/skills/ ./internal/gen/ ./internal/cli/`. Expected: PASS.
- [ ] **Step 10: Commit** `git commit -s -m "Orientation: terms, external references, and shared rules have one home"`.

### Task 12: Orient owns the ranking (S1)

**Files:**
- Modify: `internal/gen/orientation.go` (lines 769-933)
- Modify: `internal/skills/orient/SKILL.md` (lines 32-36, 68-93)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Move the pins.** Split `TestOrientationDescribesRankedNext`: the tier, sort-kind, and current-feature strings move to a new `TestOrientDescribesRankedNext` over the orient skill. Orientation keeps pins for "`tiller next`", "A ranked move is a suggestion, not authorization.", the consent rule from Task 1, "never rewrite the list", and gains "`tiller guide orient` explains how the list is ranked".
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Merge the two catalogues into orient** (move procedure). Orient's bullets at lines 68-87 and orientation's at lines 813-924 describe the same moves with different clauses. For each move, keep the union of clauses in orient. The closeout bullet (orientation.go:827-847) is the largest difference: its gate and rubric detail is execute's and already stated there, so keep in orient only what a pure orient needs to say about a closeout row.
- [ ] **Step 4: Reduce orientation's `### Ranked next`** to: what `tiller next`, bare `tiller guide`, the readout's **Next:**, and **Up next:** are; the row format; the relay and consent rules; the standing queue line; the seams outside the list (`mend-drift`, `amend`, `capture`, `design-brief`, `code-review`; Task 15 adds `upgrade`); and the pointer to `tiller guide orient`. Target about 300 words.
- [ ] **Step 5: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 6: Commit** `git commit -s -m "Orient owns how the list is ranked; orientation keeps how to relay it"`.

### Task 13: One correction table (S2)

**Files:**
- Modify: `internal/gen/orientation.go` (lines 681-767)
- Modify: `internal/skills/specify/SKILL.md` (217-237), `internal/skills/orient/SKILL.md` (99-114), `internal/skills/decompose/SKILL.md` (242-259), `internal/skills/amend/SKILL.md` (32, 50-54), `internal/skills/execute/SKILL.md` (48, 52)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Move the pins.** Read `TestCorrectionsRerunTheReadoutWithSince` and every test that pins a correction-table row; each pin follows its sentence to the file that keeps it. Add `TestSkillsPointAtTheCorrectionTable` requiring "the correction table in `.tiller/orientation.md`" in orient, decompose, specify, amend, and execute.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Build the one table in orientation** (move procedure). Start from specify's table (specify:223-235), which is the most complete, and add the rows it lacks from orientation's classes: pending baseline proposal, unapproved proposal (decompose's list at decompose:250-255), feature address correction for epic, task, and shelf keys (orientation.go:716-719). The move-feature eligibility detail (orientation.go:720-738, amend:50-54) stays in amend, which owns the operation; the table row names the route and points there.
- [ ] **Step 4: Replace the copies with pointers.** Specify keeps the first paragraph of "Correcting the feature plan" (the unapproved-plan route) and one sentence pointing at the table. Decompose keeps its unapproved-proposal command list, since Task 7 routes to it, and drops the restated move-feature rules. Orient's `## Correction routing` becomes the classification sentence plus the pointer. Execute:48 and :52 change "the correction table in `tiller guide specify`" to "the correction table in `.tiller/orientation.md`".
- [ ] **Step 5: Remove the two residue sentences** (C4): "Workflow administration versus oversight ..." at orientation.go:765-767 and amend:56. The rule they carried, no catch-all epic, is the bounded-increments rule already stated in both files.
- [ ] **Step 6: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 7: Commit** `git commit -s -m "Corrections route through one table in orientation"`.

### Task 14: Execute owns its procedures, and reads in order (S3, S4, S6, C2, C3, C4)

**Files:**
- Modify: `internal/gen/orientation.go` (lines 230-292, 349-422, 619-643)
- Modify: `internal/skills/execute/SKILL.md`
- Modify: `internal/skills/orient/SKILL.md` (lines 24, 38-48, 64-66), `internal/skills/amend/SKILL.md` (line 30)
- Test: `internal/gen/orientation_test.go`, `internal/skills/skills_test.go`

- [ ] **Step 1: Move the pins.** `TestOrientationExecutesInAWorktreeByDefault` keeps only the boundary pins (default worktree, developer's instructions win, never touch what you did not create); procedure pins move to `TestExecuteWorksInAWorktreeBesideOtherAgents`. Add `TestOrientationPointsAtExecuteForPublication` requiring "`tiller guide execute` carries the sequence". Add `TestExecuteSectionsAreInOrder` asserting the heading order given in Step 4.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Reduce three orientation bullets to boundaries** (move procedure, home is execute).
  - Branches (orientation.go:230-260): keep that every started task has its own recorded branch, that Tiller records names and never creates refs, that names are read not invented, and that history is never rewritten to make a branch fit.
  - Worktrees (orientation.go:261-292): keep the default, the developer-instructions override, per-checkout identity, and the others'-work rule.
  - Execution (orientation.go:349-422): keep what an instruction to execute authorizes, what stays human-only, that a pure orient still stops, that pure reads prove neither host state nor pushes, and that recording a missing historical review needs explicit authorization naming it.
  - Transfer (orientation.go:619-643): keep that it needs an explicit human instruction naming the work and recipient, and the pointer.
- [ ] **Step 4: Reorder execute** under these headings, moving paragraphs whole: Trigger; Reasoning (authorization and run scope); Worktrees and other agents; Claiming and starting (execute:54-58, :147); Shelf work; Working from the plan (execute:46, :56); Departures (execute:48); Questions found while building (execute:50, :52); Aggregate assembly; Publication sequence; Feature acceptance and merge closeout; Review grain and coarse submit guards (execute:137, :139); Gate commands (execute:143); Missing aggregate review after merge; Exceptional human-directed transfer; Recovery (absorbing execute:141). Delete execute:145, a copy of execute:56 and :43, after confirming each clause survives.
- [ ] **Step 5: Split the questions paragraph** (execute:50) into short paragraphs at its topic boundaries: what to record and where `--blocks` points; a question that holds the task being built; a question that holds later work; how and where the human answers; cancel, unlink, and widen; when a question stops a run. No sentence changes.
- [ ] **Step 6: Trim orient's copies** (move procedure). Orient:24, :46, :48, and :64-66 restate execute; keep the one sentence each that tells a pure orient what to say, and point at execute. Orient:42 and amend:30 become one-sentence pointers to execute's transfer section. Delete "This skill is the thin one, and its thinness is the point." at execute:16, keeping the sentence after it.
- [ ] **Step 7: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS.
- [ ] **Step 8: Commit** `git commit -s -m "Execute owns publication, branches, and transfer, in reading order"`.

### Task 15: The `upgrade` skill (S5; D13)

**Files:**
- Create: `internal/skills/upgrade/SKILL.md`
- Modify: `internal/skills/skills.go` (package comment, `//go:embed`, `order`)
- Modify: `internal/gen/orientation.go` (lines 293-348, the skills list at 935-973)
- Modify: `internal/skills/orient/SKILL.md` (lines 50-62)
- Test: `internal/skills/skills_test.go`, `internal/gen/orientation_test.go`, any CLI test that counts shims or skills

- [ ] **Step 1: Change the pins.** `TestNamesAreTheShippedSkills` expects eleven names with `upgrade` last. Add `TestUpgradeSkillCarriesMaintenance` pinning "`tiller upgrade --project-only`", "`tiller migrate --ownership-review <file>`", "Never overwrite Cellar/Caskroom files", and "Schema 9 renames". Add `TestOrientationKeepsOnlyTheMaintenanceBoundary` refusing "Schema 6 freezes execution ownership" in orientation.
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Write the skill** (move procedure; sources are orientation.go:293-348 and orient:50-62, which overlap). Frontmatter description: `Use only when the developer explicitly asks to upgrade Tiller, migrate this checkout's schema, or refresh its generated guidance. Runs the maintenance the request names, reports each phase's outcome, and stops for the developer to review and commit the diff.` Sections: Trigger; Reasoning (scopes of `upgrade`, `--project-only`, `--binary-only`; identity; Homebrew ownership; team adoption; schema notes for 6, 8, and 9); Recovery (phase outcomes, `mutation_recovery_needed`); Choreography; Hand-off. Where the two sources disagree on which nouns the schema 6 ownership review covers (orientation.go:297 lists tasks, bugs, and chores; orient:54 adds tweaks), read the migration code and state what it does.
- [ ] **Step 4: Register it.** Add `upgrade/SKILL.md` to the embed directive and `"upgrade"` to `order`. Update the package comment's count.
- [ ] **Step 5: Leave the boundary behind.** Orientation.go:293-348 becomes one bullet: "Maintenance is an explicit request. `tiller upgrade` updates the binary and this checkout; `tiller upgrade --project-only` refreshes this checkout with the running binary; `tiller join` selects identity and is never the refresh remedy. For anything more, load `tiller guide upgrade`." Orient:50-62 becomes a routing paragraph beside the design-brief and code-review ones. Orientation's list becomes "serves eleven skills" with an `upgrade` entry.
- [ ] **Step 6: Run** `go test ./...`. Expected: PASS after updating any shim-count or skill-count expectation the run names.
- [ ] **Step 7: Commit** `git commit -s -m "Skills: maintenance guidance moves to an on-demand upgrade skill"`.

### Task 16: Prohibition and residue pass (Q10, C4)

**Files:**
- Modify: every `internal/skills/*/SKILL.md`, `internal/gen/orientation.go`
- Test: `internal/skills/skills_test.go`

- [ ] **Step 1: Inventory.** Run `grep -noiE '[^.]*\b(never|do not|must not)\b[^.]*\.' internal/skills/*/SKILL.md` and classify every sentence as one of: boundary (a human-only verb, a destructive action, another person's work), restatable (a positive instruction says the same thing), duplicate (the rule is already in this file or in orientation), or residue (it answers one past incident and a general rule now covers it).
- [ ] **Step 2: Hand JC the classified list for the duplicate and residue rows** before deleting any. This is the one step in the plan that waits on review, because it is the only place a deletion is a judgment.
- [ ] **Step 3: Apply.** Boundary sentences stay, each with its reason in the same sentence. Restatable sentences become the positive instruction. Approved duplicate and residue sentences are deleted. Start with design-brief (36 prohibitions) and derive-baseline (39).
- [ ] **Step 4: Remove the named residue.** Orient:89: delete "The list needs no side notes. What the old notes described is now a row of its own: the holding question, your own in-flight branch, the neighbouring feature, and the next eligible task each rank as moves." and keep the sentence after it. Orientation.go:846-847: delete "Never tell the developer the frontier shows only their own work."; the frontier bullet already says the feature is the project's. For each of the six "in this build" hedges, drop the three words and keep the fact.
- [ ] **Step 5: Run** `go test ./internal/skills/ ./internal/gen/`. Expected: PASS. A failing pin on a deleted sentence means the sentence was a pinned rule: restore it and reclassify it as boundary.
- [ ] **Step 6: Commit** `git commit -s -m "Skills: prohibitions state boundaries with reasons; incident residue removed"`.

## Part 4: Close

### Task 17: Generation 62, specs, and reference docs

**Files:**
- Modify: `internal/gen/orientation.go` (lines 18-48)
- Modify: `internal/gen/orientation_test.go`
- Modify: `docs/spec/skill-set.md`, `docs/spec/distribution.md` (lines 5, 411), `docs/tiller-commands.md` (line 3), `site/src/content/docs/skills.md`
- Regenerate: `site/src/content/docs/spec/*` and `site/src/content/docs/cli.md`

- [ ] **Step 1: Pin the generation.** Rename `TestOrientationGeneration61` to `TestOrientationGeneration62`, expect `62`, and add pins for "## Terms" and "`tiller guide upgrade`".
- [ ] **Step 2: Run** it. Expected: FAIL.
- [ ] **Step 3: Bump.** Set `orientationSchemaGen = 62` and add to its comment: "Generation 62 gives each shared rule one home: orientation defines its terms and carries the cross-skill rules, each seam's procedures live in its skill, a run and the go that starts it are defined, and maintenance moves to the upgrade skill."
- [ ] **Step 4: Update the specs.** In `docs/spec/skill-set.md`: the status line (eleven skills, generation 62), a new `### upgrade` section, the Correction routing section's statement of where the table lives, and each skill section this plan changed. Add to its settled decisions: D1 through D14, one line each. In `docs/spec/distribution.md` and `docs/tiller-commands.md`: the generation and the skill count.
- [ ] **Step 5: Regenerate the site copies** with the site's docs sync, and update `site/src/content/docs/skills.md` by hand (eleven skills; three run only on request). Discard any CRLF-only rewrite of notices the site build produces.
- [ ] **Step 6: Run** `make test` and `make lint`. Expected: PASS.
- [ ] **Step 7: Commit** `git commit -s -m "Orientation generation 62: one home per rule, eleven skills"`.

### Task 18: Measure and verify end to end

- [ ] **Step 1: Measure.** Prose words, fenced code excluded:

  ```sh
  for f in internal/skills/*/SKILL.md; do
    printf '%s %s\n' "$(awk '/^```/{c=!c;next} !c' "$f" | wc -w)" "$f"
  done
  sed -n '/^const orientationBody/,$p' internal/gen/orientation.go | wc -w
  ```

  Baseline at `ecccd4a`: orientation 11,110; execute 8,749; orient 6,488; decompose 5,511; amend 4,882; specify 4,595; derive-baseline 3,316; code-review 3,295; design-brief 3,105; mend-drift 1,589; capture 837; total 53,477. Expected after: orientation under 7,500, orient under 5,000, total under 48,500 including the new skill. These are measures, not gates: a count above target with every rule accounted for is acceptable, and a count below it with a rule missing is not.
- [ ] **Step 2: Verify at the real surface** with the repo's `verify` skill: build the binary, `tiller init` a scratch project, confirm `tiller guide` lists eleven skills, `tiller guide upgrade` and `tiller guide orient` print, the generated `.tiller/orientation.md` carries generation 62 and `## Terms`, and `tiller upgrade --project-only` refreshes a generation 61 checkout.
- [ ] **Step 3: Cold-read check.** Give `tiller guide execute` plus the generated orientation to a fresh agent with no other context and ask it: what a run covers, whether "sounds good" starts work, and where a `.tiller/`-only commit lands when the checkout is on a task branch. Its answers must match D2, D3, and D8.
- [ ] **Step 4: Move durable decisions and delete this plan** per the repo convention: D1 through D14 are in `docs/spec/skill-set.md` after Task 17, so the plan is deleted in the final commit.

## Follow-ups outside this plan

- **Drift acknowledgment.** D5 is a prose workaround for a product gap: `tiller drift` has no record that a persisting candidate was reviewed. File an issue in `madeintandem/tiller` when JC agrees.
- **Unreviewed surfaces.** CLI help text, readout renderers, and seeded stubs were not read. The `tiller add feature` stub in particular should seed the headings Task 7 describes, if it seeds any.
- **Managed projects.** Each managed project stays on generation 61 until it runs `tiller upgrade --project-only`.

## Finding coverage

| Finding | Task | Finding | Task | Finding | Task |
|---|---|---|---|---|---|
| Q1 | 6 | S3 | 14 | I3 | 3 |
| Q2 | 7 | S4 | 14 | I4 | 4 |
| Q3 | 7 | S5 | 15 | I5 | 3 |
| Q4 | 8 | S6 | 14 | I6 | 5 |
| Q5 | 8 | S7 | 11 | I7 | 1 |
| Q6 | 9 | C1 | 11 | I8 | 1 |
| Q7 | 9 | C2 | 7, 14 | I9 | 2 |
| Q8 | 9, 10 | C3 | 7, 8, 14 | I10 | 2 |
| Q9 | 10 | C4 | 13, 14, 16 | I11 | 7 |
| Q10 | 16 | C5 | 11 | I12 | 9 |
| S1 | 12 | C6 | 11 | I13 | 4 |
| S2 | 13 | I1 | 3 | I14 | 4 |
| | | I2 | 5 | | |
