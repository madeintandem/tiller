# Gate Reuse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A command gate or command/test criterion that already passed on unchanged code under an unchanged command is reused instead of rerun, `resume` stales only what the code no longer matches, and `check` gains `--rerun` and `--gate`.

**Architecture:** A new content code identity in `internal/vcs` (every tracked or offered path, minus the project's `.tiller/` and `artifacts/tiller/`) is recorded on each gate and criterion beside a config hash of the resolved command. `evaluateGates` and `gates.EvaluateCriteria` consult prior records (the node's own, plus a feature's riding tasks) before shelling. `resume` passes the current identity into the lifecycle engine, which stales only mismatches.

**Tech Stack:** Go, cobra, yaml.v3, testify, `internal/testgit` fixtures.

**Spec:** `docs/plans/2026-10-21-gate-reuse-design.md` (decisions 1 to 15). Read it first.

## Global Constraints

- Only a `pass` is reused; `fail`, `pending`, and `stale` always rerun.
- An empty code identity (no git, a stub, a record written before this build) never matches anything.
- Reuse reads only gate records in the loaded tree. No cache file, no other branch, no history.
- A reused record keeps the original `run_by`, `run_at`, `against`, and `evidence`; it adds `reused_at`, and `reused_from` when the source is another node.
- Attestation gates and `manual`/`eval`-kind criteria are never reused or re-keyed; they keep their current carry-forward rules.
- No schema version bump. Orientation generation moves 53 to 54.
- Prose (specs, skills, orientation, comments): no em dashes. Run `grep -n "—"` on every touched doc before committing.
- Commits: `git commit -s`, message style `Area: imperative summary`, ending with the session's `Co-Authored-By` trailer. Build in a worktree; main is fast-forwarded only when JC says so.

## Review Focus

1. **A project whose `.gitignore` lacks `/artifacts/tiller/`.** Every check writes a new evidence file; without the path exclusion it would change the identity and reuse would never fire. Test in Task 1.
2. **A nested Tiller project** (project directory below the repository root). The exclusion must use the project's own prefix, and a `.tiller/` belonging to a different directory still counts as code. Test in Task 1.
3. **Records written by the 0.20.x binary**, with no `code`/`config`. They must rerun on check and stale on resume, never pass as current. Tests in Tasks 3 and 7.
4. **A gate command that names its node** (`${node.id}`). Its task and feature configs differ, so a feature must not reuse the task's run. Test in Task 5.
5. **`--gate` on a task whose other gates are not current.** The named gate passes but the task must not advance to `check`. Test in Task 6.

---

### Task 1: Code identity in `internal/vcs`

**Files:**
- Create: `internal/vcs/code_identity.go`
- Create: `internal/vcs/code_identity_test.go`
- Modify: `internal/vcs/git.go` (the `Revisions` interface, after `WorktreeFingerprint`)
- Modify: `internal/core/mutate/check_test.go` (`stubRevisions`)

**Interfaces:**
- Produces: `func (g *Git) CodeIdentity() (string, error)` returning `"sha256:<64 hex>"`; `Revisions.CodeIdentity() (string, error)`; `stubRevisions.CodeIdentity()` returns `"", nil`.

- [ ] **Step 1: Write the failing tests** in `code_identity_test.go`, using `newRepo`, `writeRepoFile`, and `run` from `git_test.go`. Each test lays a repo with `.tiller/tiller.yml` and `src/a.go` committed, then:
  - `TestCodeIdentity_SameContentCommittedOrDirty`: identity of a dirty edit to `src/a.go` equals the identity after `git commit -am`.
  - `TestCodeIdentity_IgnoresTillerState`: editing `.tiller/tiller.yml`, adding untracked `.tiller/epics/x.yml`, then committing them leaves the identity unchanged at each step.
  - `TestCodeIdentity_IgnoresEvidenceWithoutIgnoreFile`: with no `.gitignore`, writing `artifacts/tiller/gates/build-x-out-y.txt` leaves it unchanged; writing `artifacts/other.txt` changes it.
  - `TestCodeIdentity_SeesCodeChanges`: content edit, new untracked non-ignored file, deletion, and `chmod +x src/a.go` each change it; a file matched by `.gitignore` does not.
  - `TestCodeIdentity_NestedProjectExcludesOnlyItsOwnState`: project at `svc/` (`svc/.tiller/`), `Git{Dir: <repo>/svc}`. Editing `svc/.tiller/x.yml` leaves it unchanged; editing `other/.tiller/x.yml` changes it; editing `svc/main.go` changes it; the value is the same whether `Dir` is `svc/` or `svc/sub/`.
  - `TestCodeIdentity_UnbornHead`: before the first commit it returns a value, and adding a file changes it.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/vcs -run TestCodeIdentity -v`
Expected: FAIL, `g.CodeIdentity undefined`.

- [ ] **Step 3: Implement `CodeIdentity` in `code_identity.go`**

Resolve the repository root (`rev-parse --show-toplevel`) and the project prefix (`rev-parse --show-prefix` from `g.Dir`); the excluded prefixes are `<prefix>.tiller/` and `<prefix>artifacts/tiller/`. Start from `git ls-tree -r -z --full-tree HEAD` (skip when HEAD is unborn), mapping each path to `"<mode> <oid>"`. Overlay the paths from `diff --name-only --no-renames -z HEAD` and `ls-files --others --exclude-standard -z` (reuse `addNULPaths`), each read through `worktreeIdentity` with a mode from `Lstat` (`120000` symlink, `100755` when any execute bit is set, else `100644`, `160000` for a directory); a `deletedIdentity` removes the path. Drop excluded paths, sort, and hash `path\x00mode oid\n` lines with sha256. Read-only like `WorktreeFingerprint`, same `unreadable` error. Add the method to `Revisions` with a doc comment naming both exclusions, and `func (s stubRevisions) CodeIdentity() (string, error) { return "", nil }`.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/vcs ./internal/core/mutate -run 'TestCodeIdentity|TestCheck' -v`
Expected: PASS.

- [ ] **Step 5: Commit** `vcs: add a content code identity that leaves out Tiller state`

---

### Task 2: Stored fields, manifest `reuse`, config hash

**Files:**
- Modify: `internal/store/gates.go` (`Gate`, `Criterion`)
- Modify: `internal/store/manifest.go` (`GateSpec`)
- Modify: `internal/core/gates/run.go` (`GateConfig`)
- Create: `internal/core/gates/reuse.go`, `internal/core/gates/reuse_test.go`
- Modify: `internal/core/mutate/check.go` (`requiredGateConfigs` copies `Reuse`)
- Test: `internal/store/gates_test.go`, `internal/store/manifest_test.go`

**Interfaces:**
- Produces:
  - `store.Gate` gains `Code string yaml:"code,omitempty"`, `Config string yaml:"config,omitempty"`, `ReusedAt time.Time yaml:"reused_at,omitempty"`, `ReusedFrom string yaml:"reused_from,omitempty"`, placed after `Against`.
  - `store.Criterion` gains `Code`, `Config`, `ReusedAt` with the same tags, after `Status`.
  - `store.GateSpec` gains `Reuse *bool yaml:"reuse,omitempty"` and `func (s GateSpec) Reusable() bool` (nil is true).
  - `gates.GateConfig` gains `Reuse bool`.
  - `gates.ConfigHash(kind, run string) string` returns `"sha256:" + hex(sha256(kind + "\x00" + run))`.

- [ ] **Step 1: Write the failing tests**
  - `TestGatesBlock_RoundTripReuseFields` (store): a gate with all four fields and a criterion with its three round-trips byte-stable through `yaml.Marshal`/`Unmarshal`; a gate with none of them marshals without the keys.
  - `TestManifestGateReuse` (store): `reuse: false` parses to `Reusable() == false`; an absent key is `true`.
  - `TestConfigHash` (gates): deterministic; differs when kind or run differs; `ConfigHash("command", "a b")` differs from `ConfigHash("command a", "b")`.
  - `TestRequiredGateConfigsCarryReuse` (mutate, in `gate_scoping_test.go`): a manifest with `tests: { ..., reuse: false }` yields a config with `Reuse == false` and `build` with `Reuse == true`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/store ./internal/core/gates ./internal/core/mutate -run 'ReuseFields|GateReuse|ConfigHash|CarryReuse' -v`
Expected: FAIL to compile on the missing fields.

- [ ] **Step 3: Add the fields, `Reusable`, `ConfigHash`, and copy `spec.Reusable()` into `GateConfig.Reuse` in `requiredGateConfigs`.**

- [ ] **Step 4: Run to see them pass** (same command). Expected: PASS.

- [ ] **Step 5: Commit** `Gates: record code identity and config hash fields; manifest reuse opt-out`

---

### Task 3: Command gate reuse on the node's own record, and `--rerun`

**Files:**
- Create: `internal/core/mutate/gate_reuse.go`
- Create: `internal/core/mutate/gate_reuse_test.go`
- Modify: `internal/core/mutate/check.go` (`CheckInput`, `GateOutcome`, `evaluateGates`, `checkFeature`, `checkTask`)
- Modify: `internal/core/mutate/check_workitem.go`
- Modify: `internal/core/mutate/testdata/check_task_gates.yml` (regenerated)

**Interfaces:**
- Consumes: Task 1 `rev.CodeIdentity()`; Task 2 fields and `gates.ConfigHash`.
- Produces:
  - `CheckInput` gains `Rerun bool` and `Gates []string` (Gates is wired in Task 6).
  - `GateOutcome` gains `Reused bool`, `ReusedFrom string`, `ReusedRunAt time.Time`, `NotRun bool`.
  - `type reuseSource struct { nodeID string; block *store.GatesBlock }` (empty `nodeID` is the node's own record).
  - `func findReusable(sources []reuseSource, name, code, config string) (store.Gate, string, bool)`: the first source holding `name` with `Kind == command`, `Status == pass`, non-empty `Code == code`, and `Config == config`; returns the record and the source's `nodeID`.
  - `type gateRun struct { against store.RevisionRef; code, runBy string; at time.Time; rerun bool; only map[string]bool; sources []reuseSource; progress gates.Progress }`
  - `evaluateGates(tree *store.Tree, noun, nodeID string, ef *store.EvalFile, prior *store.GatesBlock, run gateRun) (map[string]store.Gate, []GateOutcome, bool, error)` replaces the positional signature.

- [ ] **Step 1: Write the failing tests** in `gate_reuse_test.go`. They use `vcs.New(root)` (real git), and gate commands that append to a counter file in `t.TempDir()` outside the repo, e.g. `echo run >> <dir>/tests.count`. Helper `runs(t, path) int` counts lines.
  - `TestCheckReusesUnchangedCode`: `layGatedTask` with counting build/tests; commit everything; check; commit the `.tiller/` delta; check again. Each counter reads 1; the second result's build/tests outcomes have `Reused == true`; the stored record's `ReusedAt` is set, `RunAt` and `Evidence` equal the first run's.
  - `TestCheckRerunsChangedCode`: as above, but edit a tracked non-`.tiller` file before the second check. Counters read 2; `Reused == false`; stored `Code` differs from the first.
  - `TestCheckRerunsChangedCommand`: change the tests `run` in `.tiller/tiller.yml` between checks (a `.tiller/` edit, so code identity is unchanged). Tests counter reads 2, build reads 1.
  - `TestCheckNeverReusesAFail`: tests command `false` the first time; with no edit, the second check runs it again (count 2 via `echo run >> f; false`).
  - `TestCheckHonorsReuseFalseAndRerun`: `reuse: false` on tests reruns tests only; `CheckInput{Rerun: true}` reruns both.
  - `TestCheckDoesNotReuseLegacyRecords`: hand-write a prior `pass` record with no `code`/`config` into `task.yml`; the check runs the gate (Review Focus 3).
  - `TestCheckWithStubIdentityAlwaysRuns`: with `stubRev(root)` (empty identity), two checks run each gate twice.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/core/mutate -run 'TestCheck(Reuses|Reruns|NeverReuses|Honors|DoesNotReuse|WithStub)' -v`
Expected: FAIL (compile error on `Rerun`, then assertion failures).

- [ ] **Step 3: Implement**

In each check path, read `code, err := rev.CodeIdentity()` right after `gateAgainst` and build `gateRun` with `sources: []reuseSource{{block: prior}}`. In `evaluateGates`, for each command config with a non-empty `Run`: compute `config := gates.ConfigHash(c.Kind, c.Run)` after interpolation; when `!run.rerun && c.Reuse && run.code != ""` and `findReusable` hits, store the copy with `Required = c.Required`, `ReusedAt = run.at`, and `ReusedFrom` set to the source id when non-empty (otherwise keep the copy's own); set the outcome's `Reused`, `ReusedFrom`, `ReusedRunAt`. Otherwise run as today and set `Code` and `Config` on the record. Attestation and pending branches are unchanged. Regenerate the golden with `UPDATE_GOLDEN=1 go test ./internal/core/mutate -run TestCheckTaskGreenAdvancesAndRecordsGolden` and confirm the only diff is the added `config:` lines (the stub's identity is empty, so no `code:`).

- [ ] **Step 4: Run the package**

Run: `go test ./internal/core/mutate`
Expected: PASS.

- [ ] **Step 5: Commit** `Check: reuse a passed gate on unchanged code`

---

### Task 4: Criterion reuse in evals, and `eval --rerun` in the core

**Files:**
- Modify: `internal/core/gates/eval.go` (`EvaluateCriteria`)
- Modify: `internal/core/gates/eval_test.go`
- Modify: `internal/core/mutate/check.go` (the evals branch of `evaluateGates`)
- Modify: `internal/core/mutate/eval.go` (`EvalInput`, `CriterionOutcome`, `runEval`, `evalsGate`, `setEvalsGate`)

**Interfaces:**
- Consumes: Task 2 fields, `gates.ConfigHash`; Task 3 `gateRun`.
- Produces:
  - `type CriteriaRun struct { Against, Code string; Reuse bool; At time.Time; Progress Progress }`
  - `EvaluateCriteria(ef *store.EvalFile, prior map[string]store.Criterion, m *store.Manifest, root string, run CriteriaRun) (map[string]store.Criterion, error)`
  - `EvalInput` gains `Rerun bool`; `CriterionOutcome` gains `Reused bool`; `evalsGate` gains `code string`, written to the evals gate's `Code`.

- [ ] **Step 1: Write the failing tests** in `eval_test.go` (update the three existing calls to the new signature):
  - `TestEvaluateCriteriaReusesPassedCommand`: prior `C1` with `Status: pass`, `Code: "sha256:x"`, `Config: ConfigHash("command", <resolved C1 command>)`; with `CriteriaRun{Code: "sha256:x", Reuse: true}` the command (a counter append) does not run and `C1.ReusedAt == run.At`.
  - `TestEvaluateCriteriaRerunsOnChange`: each of a different `Code`, a changed binding `run`, a prior `fail`, `Reuse: false`, and an empty `Code` runs the command and records `Code`/`Config`.
  - `TestEvaluateCriteriaLeavesHumanCriteria`: a `manual` criterion attested `pass` and an `eval`-kind `needs_attestation` come through unchanged with no `Code`.
  - In `internal/core/mutate`, `TestEvalRerunFlag`: two `Eval` runs on unchanged code run a counting criterion once; `EvalInput{Rerun: true}` runs it again; `CriterionOutcome.Reused` reports each.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/core/gates ./internal/core/mutate -run 'EvaluateCriteria|TestEvalRerun' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.** In the command/test arm, resolve the command as today, compute its `ConfigHash(store.GateKindCommand, run)`, and reuse the prior criterion when `run.Reuse && run.Code != ""` and prior is `pass` with matching `Code` and `Config`. Callers pass `Reuse: !rerun && <evals GateConfig>.Reuse` (evals gate `reuse: false` turns it off for every criterion), and set the evals gate's `Code` to the run's identity.

- [ ] **Step 4: Run** `go test ./internal/core/gates ./internal/core/mutate`. Expected: PASS.

- [ ] **Step 5: Commit** `Evals: reuse a passed criterion on unchanged code`

---

### Task 5: A feature check reuses its riding tasks' runs

**Files:**
- Modify: `internal/core/mutate/gate_reuse.go` (add `ridingSources`)
- Modify: `internal/core/mutate/check.go` (`checkFeature`)
- Test: `internal/core/mutate/gate_reuse_test.go`

**Interfaces:**
- Consumes: Task 3 `reuseSource`, `findReusable`.
- Produces: `func ridingSources(tree *store.Tree, featureID string) []reuseSource`, one source per task in `tree.FeatureTasks[featureID]`, sorted by task id, each pointing at that task's `GatesBlock`.

- [ ] **Step 1: Write the failing tests**
  - `TestFeatureCheckReusesRidingTaskRun`: counting build/tests; the task checks green and its state is committed; then `Check` on the feature with no code change. Counters read 1; the feature's `tests` record has `ReusedFrom == gatedTaskID` and the task's `RunBy`/`RunAt`.
  - `TestFeatureCheckRunsAfterAggregateChange`: same, with a tracked file edit before the feature check. Counters read 2.
  - `TestFeatureCheckDoesNotReuseNodeNamedCommand`: tests `run: "echo ${node.id} >> <counter>"`. The feature check runs it (Review Focus 4).

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/core/mutate -run 'TestFeatureCheck' -v`
Expected: FAIL on the counters.

- [ ] **Step 3: Implement.** `checkFeature` builds `sources` as its own block followed by `ridingSources(tree, treeFeat.ID)`. Task and shelf checks keep only their own. Evals criteria keep reusing only the node's own record.

- [ ] **Step 4: Run** `go test ./internal/core/mutate`. Expected: PASS.

- [ ] **Step 5: Commit** `Check: a feature reuses a riding task's run on the same code`

---

### Task 6: `--gate`, the advance rule, the event, and the shelf warning (core)

**Files:**
- Modify: `internal/core/mutate/check.go` (`evaluateGates`, `CheckResult`, `checkTask`, `checkFeature`)
- Modify: `internal/core/mutate/check_workitem.go`
- Modify: `internal/core/mutate/transition.go` (`TransitionInput`, `transitionEvent`)
- Test: `internal/core/mutate/gate_reuse_test.go`

**Interfaces:**
- Consumes: Tasks 3 and 4.
- Produces:
  - `CheckResult` gains `Warnings []string`.
  - `TransitionInput` gains `ReusedGates []string`; `transitionEvent` writes `after["reused"]` (sorted) when the verb is `check` and the list is non-empty. Entries are gate names, and `evals/<criterion id>` for reused criteria.
  - Unknown `--gate` name: `&usageError{reason: "unknown_gate", msg: "<name> is not a required gate on this <noun>; applicable: <sorted names>"}`, returned before any gate runs.

- [ ] **Step 1: Write the failing tests**
  - `TestCheckGateRunsOnlyNamed`: `CheckInput{Gates: []string{"tests"}}` runs tests and not build; build's outcome is `NotRun` when it has no current record, and `Reused` when it has one.
  - `TestCheckGateDoesNotAdvanceWithStaleSiblings`: after an edit, `--gate tests` passes tests but `Passed == false` and the task stays `in_progress` (Review Focus 5); a following plain check advances it.
  - `TestCheckGateUnknownName`: `Gates: []string{"lint"}` returns the `unknown_gate` usage error and writes nothing.
  - `TestCheckEventRecordsReused`: the second check's event (read from `.tiller/events/`) carries `reused: [build, tests]`; the first carries no `reused` key.
  - `TestCheckWarnsShelfWithoutCommandGates`: a manifest whose build/tests are `on: [feature]`; a bug check returns one warning naming the bug noun and `.tiller/tiller.yml`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/core/mutate -run 'TestCheckGate|TestCheckEvent|TestCheckWarns' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.** Validate `run.only` against the applicable config names plus `evals` when evals applies. A gate outside `only` is never shelled and not written: it reports `Reused` when its prior record is current (command kind, `pass`, non-empty `Code == run.code`), else `NotRun` with its prior status (or `pending`). Compute `passed` after evaluation as: every required command gate's resulting record (fresh, reused, or untouched prior) is `pass` with `Code == run.code`; required attestation gates keep today's rule; evals keeps its derived status plus the code match. With no `--gate` this equals today's rule plus the code match on fresh records. When `run.code` is empty (no identity: the stub, or no git), `passed` is today's rule over this run's results, so every existing stub-driven test keeps its meaning. Pass `ReusedGates` into `TransitionInput` on all three check paths. `checkWorkItem` adds the warning when `requiredGateConfigs(tree.Manifest, noun)` holds no command gate with a non-empty `Run`, worded: `no build or test gate applies to this <noun>: bugs, chores, and tweaks run the gates whose on: list names task in .tiller/tiller.yml, and none does`.

- [ ] **Step 4: Run** `go test ./internal/core/mutate`. Expected: PASS.

- [ ] **Step 5: Commit** `Check: --gate runs named gates; record reused gates on the event; warn on ungated shelf work`

---

### Task 7: `resume` stales only what no longer matches

**Files:**
- Modify: `internal/core/lifecycle/lifecycle.go` (`Payload`, `sideEffects`, new function)
- Modify: `internal/core/lifecycle/lifecycle_test.go`
- Modify: `internal/core/mutate/transition.go` (`TransitionInput`, `payloadFor`, the task resume arm before `lifecycle.Apply`)
- Modify: `internal/core/mutate/transition_workitem.go` (the shelf resume arm before `lifecycle.Apply`)
- Test: `internal/core/mutate/check_workitem_test.go`

**Interfaces:**
- Consumes: Task 1 `rev.CodeIdentity()`; Task 3 records carrying `Code`.
- Produces: `lifecycle.Payload` gains `CodeIdentity string`; `TransitionInput` gains `CodeIdentity string` (set by mutate, not by callers); `func StaleGatesUnlessCode(node store.Node, code string)`.

- [ ] **Step 1: Write the failing tests**
  - Replace `TestApply_ResumeStalesGates` with `TestApply_ResumeStalesOnlyMismatchedGates`: gates `build` (command, pass, `Code: "sha256:a"`), `tests` (command, pass, `Code: "sha256:b"`), `legacy` (command, pass, no code), and `code_review` (attestation, pass). Resume with `Payload{CodeIdentity: "sha256:a"}`: build stays pass; tests, legacy, and code_review go stale. With an empty `CodeIdentity` everything goes stale.
  - `TestResumeKeepsGatesOnUnchangedCode` (mutate, modeled on `TestSubmitAndResumeBug`, with `vcs.New(root)`): check green, commit, submit, commit, resume. The bug's build/tests stay `pass`; a following check reuses both (counters stay 1); after an edit, check reruns both.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/core/lifecycle ./internal/core/mutate -run 'Resume' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.** `StaleGatesUnlessCode` stales every attestation gate and every command gate whose `Code` is empty or differs from `code`, via `StaleGate`. `sideEffects` calls it on `resume` with `payload.CodeIdentity`, and the SEAM comment is replaced by one sentence citing the resume rule. `payloadFor` copies `in.CodeIdentity`. Both resume arms set `in.CodeIdentity` from `rev.CodeIdentity()` when `in.Verb == "resume"`, before `lifecycle.Apply`. `StaleGates` stays for sync and transfer.

- [ ] **Step 4: Run** `go test ./internal/core/lifecycle ./internal/core/mutate`. Expected: PASS.

- [ ] **Step 5: Commit** `Resume: stale only gates whose code no longer matches`

---

### Task 8: CLI flags and output

**Files:**
- Modify: `internal/cli/check_readiness.go` (the `check` command's flags)
- Modify: `internal/cli/check.go` (`runCheck`, `CheckView`, `GateResultView`, `humanCheck`)
- Modify: `internal/cli/eval.go` (`--rerun`, `CriterionView`, `humanEval`)
- Test: `internal/cli/check_test.go`, `internal/cli/eval_test.go`

**Interfaces:**
- Consumes: Tasks 3, 4, 6 (`CheckInput.Rerun`, `CheckInput.Gates`, outcome fields, `CheckResult.Warnings`, `EvalInput.Rerun`).
- Produces:
  - Flags: `check --rerun` (bool), `check --gate <name>` (`StringArrayVar`, repeatable); `eval --rerun`.
  - `GateResultView` gains `Reused bool json:"reused,omitempty"`, `ReusedFrom string json:"reused_from,omitempty"`, `NotRun bool json:"not_run,omitempty"`. `CheckView` gains `Warnings []string json:"warnings,omitempty"`. `CriterionView` gains `Reused bool json:"reused,omitempty"`.
  - Usage errors (exit 2): `check readiness` with `--rerun` or `--gate` is `reason: "readiness_flag"`; `eval --rerun` with `--emit-rubrics` or `--record-rubric-output` is `reason: "eval_mode_conflict"`.

- [ ] **Step 1: Write the failing tests**
  - `TestCheckHumanNamesReusedGates`: after a second unchanged check, human output contains `Reused: build (run <time>), tests (run <time>).` where `<time>` is `ReusedRunAt.UTC().Format("2006-01-02 15:04 UTC")`; a riding-task reuse reads `tests (T-xxxxxx, run <time>)`.
  - `TestCheckHumanNamesNotRun`: a `--gate tests` run prints `Not run: build (excluded by --gate).`
  - `TestCheckJSONReuseFields`: `--json` carries `reused`, `reused_from`, `not_run`, and `warnings`.
  - `TestCheckFlagsRefusedOnReadiness` and `TestEvalRerunConflicts`: exit 2 with the reasons above.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/cli -run 'TestCheck(Human|JSON|Flags)|TestEvalRerun' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.** The `Reused:` and `Not run:` lines follow the existing first line in both the green and blocked renderings; each warning prints as `Warning: <text>` on its own line.

- [ ] **Step 4: Run** `go test ./internal/cli`. Expected: PASS.

- [ ] **Step 5: Commit** `CLI: check --rerun and --gate, eval --rerun, reuse in output`

---

### Task 9: Specs, command reference, skills, orientation generation 54

**Files:**
- Modify: `docs/spec/artifact-schema.md` (The stored gate block, ~762; the resume rule, ~560; The eval model's criterion fields; the manifest gates section)
- Modify: `docs/spec/cli-surface.md` (the `check`, `eval`, `resume` rows ~106-108; the resume paragraph ~631; the check and eval sections)
- Modify: `docs/tiller-commands.md` (~486, ~516, ~593-597)
- Modify: `internal/cli/init.go` (the gates comment above `required:`, ~424-428)
- Modify: `internal/skills/execute/SKILL.md` (step 2 of the publication sequence, ~81)
- Modify: `internal/gen/orientation.go` (generation 54, its history comment, one rule line in the execution section near ~230)
- Modify: every current-source line naming generation 53: `docs/spec/skill-set.md:5`, `docs/spec/distribution.md:5` and `:398`, `site/src/content/docs/cli.md:9`, `site/src/content/docs/skills.md:15`. Leave `site/src/content/docs/spec/` alone; `site/scripts/sync-docs.mjs` regenerates it.

**Interfaces:** none new. The text states what Tasks 1 to 8 built.

- [ ] **Step 1: Write the text**
  - Schema: the four gate fields and three criterion fields; the reuse rule (pass only, code identity plus config hash, sources in the tree, `reuse: false`, `--rerun`); the code identity's definition and blind spots from the design; `stale` redefined as "a recorded result whose code identity no longer matches"; the resume rule restated on code identity.
  - CLI surface: `--rerun` and `--gate` on check, `--rerun` on eval, resume staling by code identity, the shelf warning.
  - `init` comment: replace "so mid-feature task PRs flow one reviewable diff at a time" with: task gates catch a broken task before dependents build on it, at every review grain, and a gate already passed on unchanged code is reused rather than rerun.
  - Execute, publication step 2, and the orientation line: run the project's suite through `tiller check` rather than by hand and then again through check; a reused result is normal and needs no `--rerun`; `--rerun` is for a suspected flake or environment change; `--gate <name>` iterates on one failing gate, and the node advances only on a full green set.
  - Orientation history comment: `Generation 54 reuses a gate already passed on unchanged code and says when to rerun (issue #152).`

- [ ] **Step 2: Check them**

Run: `grep -rn "—" docs/spec/artifact-schema.md docs/spec/cli-surface.md docs/tiller-commands.md internal/skills/execute/SKILL.md internal/gen/orientation.go; grep -rn "generation 53" docs/spec site/src/content/docs/*.md`
Expected: no output.

- [ ] **Step 3: Run the suite**

Run: `make lint && make test`
Expected: PASS. Fix any test that pins generation 53 or the old `init` comment (expected: `internal/cli/init_orientation_test.go`, `internal/core/readiness/readiness_test.go`, and the init golden if one exists).

- [ ] **Step 4: Commit** `Docs: gate reuse in the specs, execute, and orientation generation 54`

---

### Task 10: End-to-end verification

**Files:** none changed unless a defect turns up.

- [ ] **Step 1: Run the project's `verify` skill**, driving a scratch project at feature grain with build and tests whose commands append to a counter outside the repo:
  1. Claim and start a task, commit code, `tiller check task`: counters 1.
  2. Commit the `.tiller/` delta, `tiller check task` again: counters still 1, output says `Reused:`.
  3. Edit a source file, check: counters 2.
  4. `tiller check task --rerun`: counters 3.
  5. Submit, `tiller resume task`, then `tiller show task`: gates still `pass`.
  6. Start a second dependent task from the aggregate, finish it, merge it into the aggregate with a fast-forward, `tiller check feature`: build/tests report `reused` from the second task.
  7. Delete `/artifacts/tiller/` from `.gitignore`, run two checks with no edit: the second reuses.

- [ ] **Step 2: Report** each step's observed counter and output against the expectation. Any mismatch goes back to the owning task.
