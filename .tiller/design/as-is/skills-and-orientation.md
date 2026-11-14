# Skills and orientation: the agent guidance Tiller ships

This is a pointer page. The contract lives in `docs/spec/skill-set.md` and in the harness-integration sections of `docs/spec/distribution.md`; this page says which sections apply, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0, orientation generation 62, eleven skills).

## What this surface is

The fixed guidance text the Tiller binary carries for coding agents, and the pure functions that produce the files a checkout needs in order to find it. It has three parts. `internal/skills` embeds eleven neutral skills, one `SKILL.md` per directory, and looks them up by name. `internal/gen/orientation.go` returns the text of the generated `.tiller/orientation.md` and reads and writes its generation stamp. `internal/gen/join.go` returns what `join` and the maintenance refresh place for a harness: the managed pointer block for `AGENTS.md`, the redirect block for `CLAUDE.md`, and the thin Claude Code skill shims. None of this code reads project state, touches the filesystem, or prints. The CLI does the IO.

Its users today are `tiller guide`, `tiller init`, `tiller join`, the shared refresh behind `tiller upgrade` and `tiller migrate`, and `tiller check readiness`.

What a skill is, what belongs in one, and each skill's contract: `docs/spec/skill-set.md`, "What a skill is", "The anatomy of a skill", "The skills" (one subsection per skill, `orient` through `upgrade`), "Correction routing", and "What skills must not do". How the files reach a harness: `docs/spec/distribution.md`, "Harness integration" with its subsections "The pointer block", "orientation.md", "Graceful degradation", and "First pass: the Claude Code adapter". Why none of it may point outside the project: `docs/spec/distribution.md`, "Self-contained in the repo", third paragraph.

One boundary a reader should know. The rest of `internal/gen` (ordering, derived activity, the tree model, node labels, holds, question blockers, claim eligibility, prose paths) is read-side projection code. It is not agent guidance, and `.tiller/design/as-is/lifecycle-and-gates.md` accounts for it, along with `internal/evalref`.

## Data it owns

It owns no persisted state. What it owns is compiled into the binary: the eleven embedded `SKILL.md` files, the catalog order (`order` in `internal/skills/skills.go`), the orientation text and its generation integer (`orientationSchemaGen`, 62), and the marker strings `PointerBegin`, `PointerEnd`, and `SkillShimOwner`. The files produced from these are written by the CLI and live in the managed project: `.tiller/orientation.md` and the `AGENTS.md` and `CLAUDE.md` regions are committed; `.claude/skills/tiller-<skill>/SKILL.md` is gitignored and per checkout.

Identity is by name. A skill is its directory name, which equals its frontmatter `name`; its shim is `tiller-<skill>`. The embed directive and the `order` slice must list the same skills: `Lookup` panics on a listed skill whose file is missing, treating it as a build error. Each frontmatter `description` is a single-line plain scalar, because `Description` reads it line by line without a YAML parser.

The footprint and what is committed: `docs/spec/distribution.md`, "What Tiller places on a machine". The generation counter and why it is separate from the binary version: `docs/spec/distribution.md`, "orientation.md". The file format: `docs/spec/skill-set.md`, "Format, packaging, and portability".

## Behavior to preserve

Skill lookup. `Names` returns a copy in lifecycle order (orient, derive-baseline, decompose, specify, execute, capture, mend-drift, amend, design-brief, code-review, upgrade). `Lookup` returns the whole file and reports false for an unknown name, which `tiller guide` turns into an `unknown_skill` misuse naming the valid skills. The served text is complete in the one file: no reference loader exists. See `docs/spec/skill-set.md`, "Format, packaging, and portability" and the last paragraph of "The anatomy of a skill".

Orientation. `Orientation` is a constant for a generation, so every binary in a generation emits identical bytes. The header carries the generation and `OrientationStampGeneration` reads it back, reporting not-ok for a missing, foreign, or mangled header instead of guessing. The header's literal prefix is a parse contract across generations: rewording it breaks stale detection for every committed file (the code comment on `orientationHeader` says so; no document does). The generation moves only when the stated rules change. The body has five `##` parts (What Tiller is, Terms, The rules you must follow, How to work, Where things live), seventeen defined terms, and fifteen bold rule labels that skills point at. See `docs/spec/distribution.md`, "orientation.md", and `docs/spec/skill-set.md`, "The anatomy of a skill", paragraph "One home per rule".

Managed regions. The pointer and redirect blocks share one merge: an absent file becomes the block plus a newline; a file with no region gets the block appended after exactly one blank line; a file with one well-formed region has only that region replaced. Missing, duplicated, nested, reversed, damaged, or inline markers refuse with an error and return no content. Rerunning is byte-stable. See `docs/spec/distribution.md`, "The pointer block" (which also holds the exact block text and the rule that a wording change is a schema-era decision) and "First pass: the Claude Code adapter".

Skill shims. A shim carries frontmatter with the prefixed name and the embedded description, the ownership marker, and a body that says to run `tiller guide <skill>`; it never inlines the skill. `OwnsSkillShim` is true only for a file with that exact frontmatter shape and either the marker or the exact legacy body. A `tiller-` directory name alone proves nothing. See `docs/spec/distribution.md`, "orientation.md" (the paragraph on trigger descriptions and shims) and "First pass: the Claude Code adapter".

Tests that pin instruction contracts. The wording of skills and orientation is under test, so a guidance edit usually needs a test edit in the same change. `internal/skills/skills_test.go` pins the catalog and order, valid YAML frontmatter for skills and generated shims, the required sections, the six readout seams, the harness-tool denylist and the no-em-dash rule, the ban on naming spec documents, that every skill opens by assuming orientation, that every pointer into orientation names a label that exists there, that `tiller guide <skill>` pointers name real headings, and the ruled repeats and deletions. `internal/skills/instruction_contract_test.go` checks that an instruction does not contradict the behavior it rests on. `internal/skills/code_review_selection_test.go` builds real Git histories and runs the commands the code-review skill documents. `internal/gen/orientation_test.go` pins the stamp round trip, the terms, each label, and the shared rules; `internal/gen/join_test.go` pins the block text, the merge, its refusals, and shim ownership. The rules these enforce are in `docs/spec/skill-set.md`, "The anatomy of a skill", "Format, packaging, and portability", and "Decisions settled so far".

## Edges

Inward, the guidance code depends on the standard library only. `internal/skills` tests import `internal/gen` and `internal/testgit`.

Callers. `internal/cli/guide.go` serves skill text. `internal/cli/project_refresh.go` is the one writer shared by join, upgrade, and migrate: it writes orientation, upserts both regions, installs current shims, and removes obsolete owned ones; `internal/cli/init.go` writes orientation on its own path. Both refuse `orientation_newer` when the file on disk is a higher generation than the binary's. `internal/core/readiness/readiness.go` reports a stale-orientation gap in either direction. Those callers belong to other pages; the commands are in `docs/spec/cli-surface.md`, "Guidance and next moves: guide, next, readout" and "Migrating and upgrading: migrate, upgrade".

Outward contracts. Skills name exact `tiller` commands, so the CLI's verbs are a contract between the two layers: `docs/spec/skill-set.md`, "Staying in sync with the CLI". The harness contract is file placement only, Claude Code's `.claude/skills/<name>/SKILL.md` frontmatter and its auto-loaded `CLAUDE.md`: `docs/spec/distribution.md`, "Harness integration". Skills hand implementation craft out to the project's own instruction files: `docs/spec/skill-set.md`, "What a skill is".

## Described paths

- `internal/skills`
- `internal/gen/orientation.go`
- `internal/gen/orientation_test.go`
- `internal/gen/join.go`
- `internal/gen/join_test.go`

## Discrepancies and gaps

No contradiction was found between the two sources and this code on skill count, catalog order, generation number, block text, merge rules, or shim format. What follows is what a source omits or only plans.

An existing file at a current shim path that `OwnsSkillShim` does not recognize makes the refresh refuse with `skill_shim_unowned` (exit 3) in `internal/cli/project_refresh.go`. `docs/spec/distribution.md`, "orientation.md", mentions only the warning for ambiguous legacy ownership, which in code applies to obsolete shim names. No document names the refusal.

`docs/spec/distribution.md`, "What Tiller places on a machine", says generated artifacts carry a "do not hand-edit" header. Orientation does, and the pointer block's begin marker says edits are overwritten. The shim from `SkillShim` carries an ownership marker and "do not act from this file", but no edit notice.

`docs/spec/skill-set.md`, "The anatomy of a skill", names Trigger, Reasoning, Choreography, Readout, and Hand-off, and lists three skills that replace the readout. In code, `capture` and `mend-drift` also have no Readout section (six readout seams in all), and ten skills (all but `design-brief`) carry a Recovery section the anatomy does not name.

Planned, not built: more harness adapters, auto-detection, and a `--harness` selector (`docs/spec/distribution.md`, "Harness integration" and "First pass: the Claude Code adapter"; `docs/spec/skill-set.md`, "What's next"); naming the suggested skill in `tiller status` output (`docs/spec/skill-set.md`, "Staying in sync with the CLI"). `docs/spec/distribution.md`, "orientation.md", says instruction acceptance needs scenarios where a real agent consumes the guide in a disposable project; this repository's tests are text pins plus the one Git-history test above, and no such scenario driver was found in the described paths.
