# Specify Guidance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Specify writes criteria as rules, plans shared helpers once through a new `## Reuses` section, and keeps `preserve` for behavior at risk, with `spec feature` warning (never blocking) when a preserve criterion only re-runs what the tests gate already runs.

**Architecture:** Mostly skill text. Code changes are one stub heading in `renderTaskMD`, one pure function in `internal/core/gates` that finds preserve criteria the tests gate covers, a `Warnings` field carried from the `spec` transition to the CLI, and doc updates.

**Tech Stack:** Go, cobra, testify.

**Spec:** `docs/plans/2026-10-21-specify-guidance-design.md` (decisions 1 to 5 and the "Settled while writing the plan" list). Read it first.

## Global Constraints

- The CLI never judges prose: no check reads `task.md` headings or criterion wording.
- The warning fires only when the required `tests` gate exists, its `on` list includes `feature`, and the preserve criterion's resolved command is exactly the tests gate's `run` (string comparison after trimming surrounding whitespace) or the criterion is a `test` binding with no `run`. Nothing else warns.
- The warning never blocks `spec feature` or `approve`, and is not shown on `approve`.
- Exact warning text, exact-command case: `criterion <ID> (preserve) runs the tests gate's own command, which already runs on every check: give it a targeted check, drop it, or say in Notes from this run why it stays`
- Exact warning text, ref-only case: `criterion <ID> (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays`
- Orientation generation stays 54; the orientation text does not change.
- Prose (skills, specs, comments): no em dashes; run `grep -n "—"` on every touched doc.
- Commits: `git commit -s`, subject `Area: imperative summary`, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Stage files by name. Build in a worktree.

## Review Focus

1. **A no-change re-spec** (`spec` on an already specced feature, `changed: false`) must still carry the warnings, exit 0. Test in Task 2.
2. **A tests gate whose `on` is a bare scalar** (`on: feature`, which `StringList` accepts) must count as including feature. Test in Task 2.
3. **A ref-only preserve test binding with a list of refs** is still the ref-only case. Test in Task 2.
4. **An optional preserve criterion** warns like a required one. Test in Task 2.
5. **Existing test fixtures** that bind C2 to `nrca_golden_spec.rb` (`internal/store/testdata/...`, `internal/store/eval_test.go`, `internal/views/eval_test.go`) are data, not guidance: leave them unchanged.

---

### Task 1: The `## Reuses` stub heading

**Files:**
- Modify: `internal/core/mutate/add.go` (`renderTaskMD`)
- Test: `internal/core/mutate/add_test.go`

**Interfaces:**
- Produces: a fresh `task.md` stub reads `## Approach`, `## Reuses`, `## Surfaces`, `## Tests`, `## Edge cases`, in that order, with the stub marker paragraph unchanged.

- [ ] **Step 1: Write the failing test** `TestAddTaskStubCarriesReuses`: after `AddTask`, the `task.md` contains `"## Approach\n\n## Reuses\n\n## Surfaces\n"`, and still contains `taskStubMarker` text so `spec feature` treats it as a stub.
- [ ] **Step 2: Run it to see it fail:** `go test ./internal/core/mutate -run TestAddTaskStubCarriesReuses -v`. Expected: FAIL on the missing heading.
- [ ] **Step 3: Add the heading in `renderTaskMD`** and update its doc comment to say the Reuses heading is where specify records what the task calls instead of rewriting.
- [ ] **Step 4: Run** `go test ./internal/core/mutate ./internal/serve ./internal/cli`. Expected: PASS (fix any test that pinned the exact old stub).
- [ ] **Step 5: Commit** `Tasks: seed a Reuses heading in the task.md stub`

---

### Task 2: Find covered preserve criteria and carry them on `spec`

**Files:**
- Create: `internal/core/gates/preserve.go`, `internal/core/gates/preserve_test.go`
- Modify: `internal/core/mutate/transition.go` (`TransitionResult`, the `spec` arm near the `validateSpec` call, and the no-change return at ~511)
- Test: `internal/core/mutate/transition_test.go` (or a new `spec_warnings_test.go`)

**Interfaces:**
- Produces:
  - `type PreserveCoverage struct { CriterionID string; ExactCommand bool }` and `func (p PreserveCoverage) Warning() string` returning the Global Constraints text for its case.
  - `func CoveredPreserveCriteria(ef *store.EvalFile, m *store.Manifest) []PreserveCoverage`, in contract order. Empty when `ef` or `m` is nil, when there is no required `tests` gate, or when its `On` does not include `feature`. A criterion counts only when `Polarity == store.PolarityPreserve` and its binding is `command` or `test`; `ExactCommand` when `strings.TrimSpace(binding.Run) == strings.TrimSpace(tests.Run)`, ref-only when it is a `test` binding with empty `Run` and a non-empty `Ref`.
  - `mutate.TransitionResult` gains `Warnings []string`, filled only by `spec`.

- [ ] **Step 1: Write the failing table test** `TestCoveredPreserveCriteria` with cases and expectations:
  - command binding, run equal to tests run: exact
  - command run equal after surrounding whitespace: exact
  - test binding whose run equals tests run: exact
  - test binding, ref only: ref-only
  - test binding, ref list of two, no run: ref-only
  - test binding with a targeted run: none
  - optional (required: false) preserve, ref only: ref-only
  - introduce polarity, ref only: none
  - manual preserve: none
  - tests gate `on: [task]`: none
  - tests gate `on: feature` (scalar): counts
  - no tests gate: none
  
  and `TestPreserveCoverageWarning` asserting both exact strings with `C5`.
- [ ] **Step 2: Run to see it fail:** `go test ./internal/core/gates -run 'Preserve' -v`. Expected: FAIL, undefined.
- [ ] **Step 3: Implement `preserve.go`.**
- [ ] **Step 4: Write the failing mutate test** `TestSpecFeatureWarnsOnCoveredPreserve`: a feature whose contract has a ref-only preserve criterion and a manifest whose tests gate is `on: [feature, task]`. `spec` succeeds (status `specced`) with one warning naming the criterion; a second `spec` (no change, `Changed == false`) carries the same warning; a contract with only a targeted-run preserve criterion yields no warnings.
- [ ] **Step 5: Run to see it fail**, then fill `Warnings` in the `spec` arm after `validateSpec` passes, from `CoveredPreserveCriteria(ef, tree.Manifest)` mapped through `Warning()`, on both the committing path and the no-change return. Load the contract the way `validateSpec` does (`store.LoadEvalFile`).
- [ ] **Step 6: Run** `go test ./internal/core/gates ./internal/core/mutate`. Expected: PASS.
- [ ] **Step 7: Commit** `Spec: warn when a preserve criterion repeats the tests gate`

---

### Task 3: Render the warnings on `tiller spec feature`

**Files:**
- Modify: `internal/cli/lifecycle_cmds.go` (`TransitionView`, `transitionView`, `humanTransition`)
- Test: `internal/cli/lifecycle_cmds_test.go`

**Interfaces:**
- Consumes: Task 2 `TransitionResult.Warnings`.
- Produces: `TransitionView.Warnings []string` with tag `json:"warnings,omitempty"`; human output appends one line per warning, `Warning: <text>`, after the existing first line, on both the changed and the "already ... (no change)" renderings.

- [ ] **Step 1: Write the failing tests** `TestSpecFeatureHumanWarning` (a real project, `tiller spec feature` prints `Warning: criterion C1 (preserve) is a test the tests gate already runs on every check: ...`, exit 0) and `TestSpecFeatureJSONWarnings` (`--json` carries `warnings` with that text; a contract with no covered preserve criterion omits the key).
- [ ] **Step 2: Run to see them fail:** `go test ./internal/cli -run 'TestSpecFeature(Human|JSON)Warning' -v`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/cli`. Expected: PASS.
- [ ] **Step 5: Commit** `CLI: show spec feature's preserve warnings`

---

### Task 4: Specify guidance and docs

**Files:**
- Modify: `internal/skills/specify/SKILL.md`
- Modify: `docs/spec/artifact-schema.md` (the contract example's C2 at ~670, and the task.md heading list if it names one)
- Modify: `docs/spec/cli-surface.md` (the `spec` row in the verb table, and "Specifying a feature")
- Modify: `docs/spec/skill-set.md` (specify's summary)
- Modify: `docs/tiller-commands.md` (the `spec feature` line)

**Interfaces:** none. The text states what Tasks 1 to 3 built and the design's guidance.

- [ ] **Step 1: Write the skill text.**
  - "The contract": a criterion states one observable behavior as a rule that holds; captured values, golden outputs, pixel or paint budgets belong inside the tests it binds to; a look judged by eye is `manual`. Then the preserve rule: `preserve` is for one specific existing behavior this feature puts at risk, bound to a check that targets it; the tests gate already runs the whole suite on every check; when `spec feature` warns, give the criterion a targeted check, drop it, or keep it and say why in Notes from this run.
  - The contract example: C2 keeps its statement and binds to a rule-shaped targeted spec, e.g. `bundle exec rspec spec/pdf/page_numbering_spec.rb`, no `golden` in the name. Same change in `artifact-schema.md`'s example.
  - "The task implementation spec shape": the stub block shows `## Reuses` after `## Approach`, described as: what was searched, what exists (file:line) and what this task calls instead of writing anew, or "None found" naming what was searched. Then the owning-task rule: a helper two tasks need is planned once, built in the task that naturally owns it with `--depends-on` from the others, and becomes its own small prerequisite task only when no task naturally owns it; both tasks' Reuses say so.
  - "Readout" Notes from this run: add each `spec feature` warning's resolution.
- [ ] **Step 2: Write the spec and command-reference sentences:** the `spec` row and section say `spec feature` warns, without blocking, when a preserve criterion repeats the tests gate (the two cases), on first spec and re-spec; the `tiller-commands.md` line says the same in brief; `skill-set.md` mentions rules-not-values, Reuses, and preserve-at-risk in specify's summary.
- [ ] **Step 3: Check** `grep -n "—" internal/skills/specify/SKILL.md docs/spec/artifact-schema.md docs/spec/cli-surface.md docs/spec/skill-set.md docs/tiller-commands.md` prints nothing, and `grep -rn "nrca_golden" internal/skills docs/spec` prints nothing.
- [ ] **Step 4: Run** `make lint && make test`. Expected: PASS (fix any skill-text pin in `internal/skills/skills_test.go`).
- [ ] **Step 5: Commit** `Docs: specify writes rules, plans reuse, and keeps preserve for behavior at risk`
