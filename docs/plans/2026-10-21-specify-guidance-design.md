# Specify guidance: rules, reuse, and preserve

Designed and settled 2026-10-21 from issue #152, items 2, 3, and 6. Not built. It ships in the same release as gate reuse (`docs/plans/2026-10-21-gate-reuse-design.md`, item 4).

## Problem

On KoPanda, a solo project built at feature review grain, three habits in the plans specify wrote cost more than the code they described.

**Criteria pinned values.** 55 of 318 criteria bound to golden captures and 59 pinned literal values, so every visual change broke a criterion, and golden and paint-budget work cost 7.6 agent-hours. The specify skill teaches the habit: its own contract example binds criterion C2 to `bundle exec rspec spec/pdf/nrca_golden_spec.rb` (`internal/skills/specify/SKILL.md`, The acceptance contract shape), and nothing says a criterion states one observable behavior.

**Tasks rebuilt the same helper.** Cold-started task builders rewrote one helper up to nine times on KoPanda, and Theme Studio needed nine cleanup tweaks to merge duplicates. The `task.md` stub carries Approach, Surfaces, Tests, and Edge cases (`renderTaskMD`, `internal/core/mutate/add.go`), and nothing asks specify to look for what already exists or to say which task owns a helper several tasks need.

**Preserve criteria restated the suite.** KoPanda carries 64 preserve criteria, most restating specs the tests gate already runs on every check, each with a baseline to refresh. The skill explains `preserve` mechanically (polarity, baseline, one artifact) but never says when a preserve criterion earns its place.

## What this is

Three changes to what specify writes, mostly in the specify skill's own text, plus one stub heading and one non-blocking warning.

It refuses to judge prose in the CLI. `spec feature` decides a `task.md` is written only by the generated stub paragraph being gone (`mdIsStub`, `internal/core/mutate/transition.go`), never by reading its headings, because judging prose belongs to the agent and the human reviewing the diff (AGENTS.md, principle 1). Nothing here changes that.

It refuses to guess. The one warning fires only where the manifest proves the redundancy, never on a name match.

It refuses to block. The warning never stops `spec feature` or the plan gate.

## Decisions settled

1. **Criteria state rules, not values, through guidance only.** No CLI check: detecting a golden or snapshot binding means matching names like `golden`, `snapshot`, or `fixture` in a path, which flags legitimate characterization tests and misses a pinned literal in an ordinarily named spec. A warning wrong half the time teaches people to ignore warnings, and decision 4 needs that channel trusted.
2. **The Reuses section is a stub heading plus guidance, with no CLI check.** `tiller add task` writes a `## Reuses` heading; the specify skill says what goes under it. Existing tasks are untouched; only new stubs carry the heading, so nothing migrates.
3. **A helper several tasks need is planned once, in one owning task.** When one task naturally owns it, it is built there and every other task that needs it gets `--depends-on` to that task and names it in its Reuses. Only when no task naturally owns it does it become its own small prerequisite task that the others depend on. Always splitting it out would produce the checklist-sized tasks the skill already warns against; the KoPanda failure was two plans that never said who owned the helper.
4. **`spec feature` warns about a preserve criterion only where the manifest proves the tests gate already covers it.** Two cases: the criterion's resolved command is exactly the required `tests` gate's `run`, or it is a `test` binding with no `run`, which Tiller resolves to the tests runner plus its ref, a subset of what the tests gate runs on every check. A preserve criterion with its own targeted `run` (a characterization run, a baseline comparison) never warns. The warning applies to a required or an optional preserve criterion alike, since an optional one still runs on every check. With no required `tests` gate, nothing warns.
5. **The warning is non-blocking and appears on `spec feature` only.** It prints on the first spec and on the re-validation of a specced feature, as a `Warning:` line in human output and a `warnings` list in the JSON envelope, the shape `check` uses for its shelf warning. It is not repeated on `approve`. The specify skill tells the agent to fix the criterion or say why it stays in its Notes from this run, which the human reads at approval.

## The guidance

**Criteria (decision 1).** Under "The contract" in the specify skill: each criterion states one observable behavior as a rule that holds, for example "page numbers are continuous across assembled forms", never a captured value, a golden output, or a pixel or paint budget. Those values belong inside the tests a criterion binds to, where a deliberate change updates one fixture, not the contract. A look judged by eye is a `manual` criterion. The contract example's C2 becomes a rule-shaped statement with a targeted test binding, keeping the example's other four criteria as they are.

**Reuses (decisions 2 and 3).** `renderTaskMD` writes `## Reuses` after `## Approach`. The skill's task implementation spec section shows the heading and says: before planning new code, search the repository for existing helpers, components, and fixtures this task can call; list each with its file:line and what the task uses it for; "None found" names what was searched. When two tasks in the feature need the same new helper, plan it once under decision 3 and say so in both tasks' Reuses.

**Preserve (decisions 4 and 5).** Under "The contract": `preserve` is for one specific existing behavior this feature puts at risk, bound to a check that targets it. The tests gate already runs the whole suite on every check, so a preserve criterion that only re-runs the suite, or one spec of it, adds a baseline to refresh and nothing the gate does not already prove. When `spec feature` warns, either give the criterion a targeted check or drop it, or keep it and say why in Notes from this run.

## The warning

A new function in `internal/core/gates`, beside `criterionCommand` and `requiredTestsRunner`, returns the preserve criteria the tests gate covers, each with its reason. `spec feature` calls it after `validateSpec` passes and carries the result on the transition result; the CLI renders each as `Warning: <text>` and in `warnings`. Suggested text for the exact-command case: `criterion C5 (preserve) runs the tests gate's own command, which already runs on every check: give it a targeted check, drop it, or say in Notes from this run why it stays`. The ref-only case's text is settled below. The builder may tighten the wording; it must name the criterion id, the polarity, and the reason.

Settled while writing the plan (builder questions, answered here):

- **The ref-only case warns by design.** A ref names one spec, which looks targeted, but the tests gate already runs that spec on every check, which is exactly what KoPanda's 64 preserve criteria restated (decision 4). Its remedy text says so instead of asking for "a targeted check": `criterion C5 (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays`. The exact-command case keeps the "give it a targeted check, drop it, or say why it stays" tail.
- **Only a tests gate that runs where the criterion runs counts** (my call). The criterion runs at feature check, so the warning fires only when the required `tests` gate's `on` list includes `feature`. A tests gate scoped to tasks alone does not run at feature check, so a feature preserve criterion is not redundant with it.
- **"Exactly the tests gate's command" is a string comparison after trimming surrounding whitespace** (my call). No `${node.*}` interpolation applies: criterion commands carry none, and a tests `run` that interpolates a node never equals a criterion's command anyway.
- **The warning also prints on a no-change re-spec** (decision 5). `spec` on an already specced feature re-validates and returns early with `changed: false`; it carries the warnings on that path too, exit 0.

## Surfaces

The specify skill (`internal/skills/specify/SKILL.md`): the contract section, the example, the task implementation spec section, and the readout notes. `renderTaskMD` (`internal/core/mutate/add.go`). The warning function (`internal/core/gates`), the spec arm (`internal/core/mutate/transition.go`, which already runs `validateSpec` for draft and specced features), the transition result, and the CLI transition rendering (`internal/cli/lifecycle_cmds.go`, and the batch path if `spec` can run in a batch). Specs: `docs/spec/cli-surface.md` (the `spec` row and Specifying a feature), `docs/spec/artifact-schema.md` (its own contract example binds C2 to the same golden spec, so it gets the same rewrite; and the task.md headings, if it lists them), `docs/spec/skill-set.md` (specify's summary), and `docs/tiller-commands.md`.

The orientation text does not change, so the generation stays 54, already bumped in this release by gate reuse.

## Left to the builder

The exact skill wording, within the rules above. The warning's final wording and where in the transition result it lives. Whether the C2 example becomes a `test` or a `command` binding. Tests for `renderTaskMD`'s new heading and for each warning case, including the two non-warning cases (a targeted `run`, and no `tests` gate).

## What's next

Write the build plan, then build in a worktree.
