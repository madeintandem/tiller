# Gate reuse

Designed and settled 2026-10-21 from issue #152, item 4 ("Run each gate once per commit, not once per task"). Not built.

## Problem

On KoPanda, a solo project built at feature review grain, gate runs took 21.5 hours of wall clock. 514 of them repeated the previous run of the same gate with no edit in between, about 4.2 hours, and 72 of 210 `tiller check` runs repeated a manual run of the same suite. The code under test had not changed; Tiller ran it again anyway.

Three things in the code cause it.

**`check` never skips.** `evaluateGates` (`internal/core/mutate/check.go`) runs every required command gate that applies to the node's noun, every time. It records an `against` revision on each result but never reads one back to decide anything.

**`resume` stales every gate.** `sideEffects` in `internal/core/lifecycle/lifecycle.go` calls `StaleGates` on resume. Its SEAM comment says this was the conservative stand-in until read-only Git existed, and that the real rule narrows to gates whose `against` no longer matches. The spec already states the narrow rule (`docs/spec/cli-surface.md`, the `resume` row; `docs/spec/artifact-schema.md`, the resume rule under Lifecycles), so code and spec disagree today.

**At feature grain the full suite runs once per task and again for the feature.** The manifest `init` writes carries build and tests as commented templates with `on: [feature, task]`, which projects uncomment as written. A five-task feature runs the suite at least six times.

The obvious fix, "skip a gate already green against the current commit," does not work, because Tiller's own bookkeeping moves the commit. Execute's publication sequence is: commit code, run `check`, commit the `.tiller/` state `check` wrote.

```
commit code             HEAD = A
tiller check            against = git:A, writes .tiller/.../task.yml
commit .tiller state    HEAD = B
tiller check            against = git:B, which is not git:A, so it reruns
```

Skip the state commit and the second `check` sees a dirty tree: its `against` is a working-tree fingerprint, which covers the first run's `.tiller/` writes, so it misses again. The same trap catches the spec's resume rule. By the time a review requests changes, HEAD carries the submission-state commit, so "stale unless `against` still matches" stales everything, exactly as the stand-in does.

## What gate reuse is

Gate reuse means a command-backed gate, or a command or test criterion, that already passed on this exact code under this exact command is not run again. Its pass stands, the output says it was reused, and the record names the run it came from. The rule in one line: run a gate on code it has not passed on, skip it on code it has.

It refuses to reuse anything but a pass. A fail, a pending result, or a stale one always reruns.

It refuses to reuse across a changed command. Editing a gate's `run` in the manifest, or a criterion's binding in `feature.eval.md`, reruns it.

It refuses a machine-local cache. Reuse reads only gate records committed in the tree it is checking, so two developers on the same commit see the same behavior, and anyone reading the repository can verify a reuse (the self-contained principle: nothing lives outside the repo).

It refuses to claim a fresh run. A reused record keeps the original run's time, runner, revision, and evidence, and adds when and from where it was reused.

It leaves human verdicts alone. Attestation gates and `manual` and `eval`-kind criteria already carry their settled verdicts forward and are unchanged.

## Decisions

1. **The rule is code identity, not commit identity.** A result is reusable when the code it ran over is unchanged, however many state commits sit between the runs.
2. **Task gates stay real gates at every review grain.** Each task check runs on new code, the task's own work, so it is not waste. Dependents at coarse grain branch from the aggregate after their prerequisites merge in; dropping task gates would let a broken task land there and stay hidden until feature acceptance, with no way to tell which task broke it. There is no grain-aware gate scoping. When tasks run in sequence the aggregate fast-forwards to the last task's tip, its code identity matches, and the feature check reuses that run (decision 7). When tasks run in parallel, the merged aggregate is a combination nobody has tested, and running it is correct.
3. **Reuse is on by default, with a per-gate opt-out and a force flag.** A gate whose result depends on something outside the repository (an integration suite against a live service, a check of today's date) declares `reuse: false` in the manifest. `check --rerun` forces real runs for one invocation, for a suspected flake or an environment change.
4. **Reuse reads only records in the current tree.** The sources are the node's own `gates:` block and, for a feature check, its riding tasks' `task.yml` records, which are present on the aggregate because task branches merge in with their state commits. No other branches, features, or history are searched.
5. **Code identity leaves out the project's `.tiller/` directory and its `artifacts/tiller/` evidence directory.** The managed-project independence rule in `.tiller/orientation.md` already says project commands never read `.tiller/` inputs, so Tiller state cannot change what a build or test does. The evals gate is the one command that reads `.tiller/`, and it is not shelled as a whole (decision 8). Evidence is gitignored by `join`, but a project whose ignore file lacks the pattern would otherwise see every check's own evidence file as a code change, so the identity leaves it out by path.
6. **Code identity is content, committed or not.** The same files give the same identity whether they are committed, staged, or dirty. Checking dirty, committing, and checking again reuses the first run.
7. **The reuse key is code identity plus a config hash.** The config hash covers the gate's kind and its resolved `run`, after `${node.*}` interpolation. A task's gate and its feature's gate share a key only when their resolved commands are identical, which is the usual case for build and tests and never the case for a command that names its node.
8. **Evals reuse per criterion.** The evals gate is never shelled; `EvaluateCriteria` runs each `command` and `test` criterion separately. Each of those criteria gets its own code identity and config hash, where the config is the resolved command including the tests runner a ref-only binding appends to. The gate's status stays derived from its criteria. A contract change reruns exactly the criteria whose resolved command changed. `tiller eval` shares the function, so it reuses too and takes `--rerun`.
9. **`resume` stales only what no longer matches.** A gate or criterion whose stored code identity differs from the current one goes stale; the rest keep their pass. At the moment of resume nothing has usually changed, so nothing stales, and the next `check` reruns exactly what the follow-up commits changed. A record with no stored code identity, written before this build, stales as today.
10. **`check --gate <name>` runs only the named gates.** It is repeatable, and `evals` is a valid name. Every other applicable gate is evaluated reuse-only: it counts when reusable and is reported "not run" otherwise, never shelled. The node advances only when every required gate is green for the current code identity, so `--gate` speeds up iterating on one failing gate without letting a partial run advance work.
11. **No schema version bump.** The new fields are optional. A record without them is simply not reusable, and an older binary that rewrites a node drops them, which costs a reuse and nothing else. Execute's skill text changes, so the generation bumps.
12. **`check` warns about ungated shelf work.** `gateNoun` maps bug, chore, and tweak to `task`, so a project that edits tests to `on: [feature]` silently drops tests from all shelf work, which owns its own review at every grain. `check` warns when a bug, chore, or tweak resolves no command gates, naming the manifest line. Letting `on:` name shelf nouns directly waits for a project that wants different gates for shelves.
13. **A basis change does not block reuse.** A re-approved spec with unchanged code and commands reuses, because a basis change does not change what a command does. Check's basis guards still apply on their own.
14. **Reuse has no age limit.** Age does not change code; the environment cases belong to `reuse: false`.
15. **A reused run keeps its original runner.** A feature check that reuses a riding task's run keeps that task owner's `run_by`, so a feature can show a pass run by someone other than the person checking it. The record says who ran the command, and `reused_from` explains why.

## The stored record

A gate gains four optional fields: `code`, the sha256 code identity it ran over; `config`, the sha256 config hash; `reused_at`, set when a check reused it; and `reused_from`, the source node id when the record came from another node. A command or test criterion gains `code`, `config`, and `reused_at`. Everything else is the original run's.

```yaml
gates:
  tests:
    required: true
    kind: command
    status: pass
    run_by: robin
    run_at: 2026-10-21T14:02:11Z
    against: { kind: git_commit, value: "git:4be1c0d" }
    code: "sha256:9f2c..."
    config: "sha256:51aa..."
    reused_at: 2026-10-21T16:40:03Z
    reused_from: T-k3p9w6
    evidence:
      - { kind: command_output, ref: "artifacts/tiller/..." }
```

`against` keeps its meaning: the revision the command actually ran over. `code` is what reuse compares.

## Code identity

The identity is a sha256 over the sorted list of (path, mode, blob id) for every file in the checkout that Git would track or offer to track, excluding paths under the project's `.tiller/` and `artifacts/tiller/` directories, in repository-root coordinates like the existing fingerprint. It is built from HEAD's tree listing (`git ls-tree -r -z HEAD`, which gives blob ids without reading files), overlaid with the paths that differ from HEAD and the untracked, non-ignored files, each hashed in-process the way `WorktreeFingerprint` already does. A deleted path drops out of the list. Nothing is written to the index, object store, or refs. The cost is linear in tracked files, with file reads only for changed paths. It lives beside `WorktreeFingerprint` in `internal/vcs`.

Its blind spots are the fingerprint's, and they are deliberate. Ignored files (`.env`, dependency directories, build caches), environment variables, tool versions, and the network are invisible to it. Lockfiles are tracked, so a dependency bump is seen. A dirty file whose bytes differ from its committed blob only by checkin conversion (CRLF, a clean filter) reads as changed, which costs a reuse and never wrongly grants one. `reuse: false` and `--rerun` cover the rest.

## Surfaces

**`tiller check`** gains `--rerun` and a repeatable `--gate <name>`. Its per-gate line says when a result was reused and from where, for example `tests  pass  reused (T-k3p9w6, 14:02)`, and the JSON outcome carries `reused` and `reused_from`. The check event records which gates and criteria were reused, so a later analysis like #152's can count them.

**`tiller eval`** gains `--rerun` and reuses as `check` does.

**The manifest** accepts `reuse: false` on a required gate. On the evals gate it turns reuse off for every criterion.

**`resume`** narrows as decision 9 states. The SEAM comment goes.

**`init`** keeps writing build and tests as commented templates on `[feature, task]`. Its comment's reason, "so mid-feature task PRs flow one reviewable diff at a time," only holds at task grain; it is rewritten to say task gates catch a broken task before dependents build on it, at every grain.

**Execute** tells the agent to run the suite through `tiller check` rather than by hand and then again through check; that a reused result is normal and needs no `--rerun` "to be sure"; that `--rerun` is for a suspected flake or environment change; and that `--gate` is for iterating on one failing gate.

**Specs.** `artifact-schema.md`: the stored gate block (the new fields and the reuse rule), the eval model's criterion fields, and the resume rule restated on code identity. `cli-surface.md`: the `check`, `eval`, and `resume` rows and their sections. The manifest section gains `reuse`.

## Build shape

One build, in this order: the code identity in `internal/vcs`; the store fields; reuse in `evaluateGates` and `EvaluateCriteria` with the config hash and `reuse: false`; riding-task sources for a feature check; the `resume` narrowing; the CLI flags, output, JSON, and event; then the skills, specs, `init` comment, and generation bump. Verification drives a scratch project through the publication sequence above and confirms the second check reuses, an edit reruns, a manifest command change reruns, `resume` stales nothing until code changes, and a sequential two-task feature at feature grain reuses the last task's run at acceptance.

## What's next

Write the build plan, then build in a worktree.
