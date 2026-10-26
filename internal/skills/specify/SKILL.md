---
name: specify
description: Use when the human pulls a draft feature forward from the frontier. Writes the feature's specification, its acceptance contract, and its task breakdown in one pass, then records the move to specced.
---

# specify

## Trigger

The human has named a `draft` feature from the frontier. This is the second decompose stage, run one feature at a time, just in time, against only the feature they named, unless the human explicitly names a batch of draft features.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

One pass produces the feature's whole plan: the specification, the acceptance contract, and the task explosion. They are one act with one output and one gate, because the task breakdown is the evidence that the specification is right-sized. Do not stop between them.

Apply the managed-project independence rule from `.tiller/orientation.md` while writing the plan and acceptance bindings. Project commands must run without Tiller; local Tiller may invoke them, never the reverse. Do not turn planning-only CI skips into replacement Tiller checks, even optional ones. If an existing or approved plan requires Tiller installation, invocation, or release credentials in the project's operational path, name the conflict and propose ordinary project checks and conservative change classification instead. Correct accepted contracts only through the applicable human review, not by silently changing their bindings.

An explicit instruction naming several features, or an instruction to specify every feature in a named epic, forms a batch. The named-set form may include only `draft` features; the epic-wide form selects every draft feature in that one named epic. The batch overrides the ranked list's pacing for exactly that named set. Read its features in Tiller's rendered soft order. Complete the full specification, contract, and task explosion for one feature, run `tiller spec feature`, then commit that feature alone. A batch has one `specify:` commit per feature. Continue to the next named feature without stopping on an intermediate wait the ranked list suggests, and do not add unnamed siblings. Specify never runs `approve` inside the batch. Finish the batch with one final `tiller readout specify`.

**The specification.** `feature.md` already carries the narrative from decompose; extend it with the precise behavior the feature must produce, drafted as prose the human owns and will edit.

Check scope before expanding the plan. The selected feature is a bounded outcome in a delivery progression, not a permanent subsystem bucket. Every task must serve this increment, and shared implementation should cover this increment's needs while allowing known later extensions. Keep later requirements located in their later features rather than turning them into speculative tasks here. Account for all requirements of the selected increment; a narrower first implementation must not silently weaken its promised acceptance.

**The contract.** `feature.eval.md` carries the teeth. Every criterion binds to a check, never a hope: a `test`, a `command`, a `manual` sign-off, or an `eval`-kind rubric. Set polarity with care, because it is the brownfield hinge: `introduce` asserts new behavior is present, `preserve` asserts existing behavior survives, and every required `preserve` criterion carries the pre-change baseline it was captured from. An `eval`-kind rubric can produce evidence, but when it is required a human still attests the recorded rubric output before it satisfies acceptance. The criteria name the checks that must hold; they do not write the code or the tests that prove them. That is craft, and it is handed out.

**The explosion.** Break the feature into tasks at the grain where code actually gets written. `tiller add task <epic>/<feature>/<task-key> --title "<what it accomplishes>"` creates a folder holding a `task.md` implementation spec, and writing that spec is the point of the step: it says how the work will be done, the approach, the surfaces it touches, the tests it adds, so the executing loop works from a written plan instead of re-deriving one from the feature. A good task is a single completable unit that yields one reviewable diff and its own evidence; the failure mode to avoid runs in both directions, a task so large it hides several diffs, or so small it is a checklist item. Technical tasks and genuine technical prerequisites are valid; tasks need not each deliver independent end-user value. Prefer observable feature progress early. Record a true sibling prerequisite with `--depends-on` on the add, or `tiller link task <key> --depends-on <sibling>` afterwards. For a recommended sequence that is not a prerequisite, use `tiller order task <first> <second> ...`; folders carry no order of their own. Soft order replaces the parent's order list, permits partial lists, and never gates work.

The task set is part of the specification's own review. A large set may reveal that the feature hides several independently acceptable outcomes; one vague task may show that the plan is not concrete enough. Judge boundaries, not a task-count threshold. If no sensible independent acceptance boundary exists, keep one unfinished feature rather than deleting a real prerequisite or presenting incomplete behavior as complete. A simulated experience requires explicit prototype intent. Concrete migration, compatibility, or throwaway-work risk can justify a bounded technical prerequisite; explain the tradeoff rather than prebuilding speculative capability.

You propose the tasks; the human approves them, along with the spec and the contract, in one act. Each task must be a single-engineer execution unit. New tasks remain unclaimed, including tasks added under an active feature; creation and approval assign nobody. Execution claims each task independently, and first start permanently fixes that task’s owner while atomically activating a ready parent. Features and epics carry Contributors derived from started tasks, not exclusive claims. The store records `proposed_by` as the acting contributor, the developer you act as, because there is no separate agent identity to attribute to.

Decompose remains responsible for auditing and recording every known feature-level prerequisite, including cross-epic edges; specify is not a deferred initial sweep. Specification and later task discovery can nevertheless reveal a new prerequisite, misplaced requirement, broader platform need, or future sequencing change. Explain the affected outcomes, retained scope, and proposed correction in the existing prose and Notes from this run. Distinguish actual prerequisites from preferred order and eventual integration. A prerequisite must ultimately be state, not just a prose note: use `tiller link feature <key> --depends-on <target>` when the dependent remains eligible and the change is within authorized planning or explicit correction work. Use ordinary sibling `tiller order` for preference, never a hard edge.

Do not silently widen the selected feature or named batch, specify future features, reorder an accepted tree, or replan unrelated work. Show consequential outside-scope changes and the applicable route under **Correcting the feature plan**, and obtain developer direction before performing the wider reshape. Extend an existing epic or feature when the work serves the same bounded outcome; propose a new increment for a distinct outcome or delivery phase, even if it touches the same subsystem. Sharing a subsystem name and being marked done are neither sufficient nor disqualifying placement criteria. A done parent can receive new children through existing additions and normal approval; cancelled or archived ancestors are not executable continuation targets. Completed tasks remain completed history. New unfinished children can reopen parent rollups through `tiller sync`, not by rewriting terminal feature/task prose or asserting an immediate parent transition at add time.

Direct hard-edge changes obey the existing noun-specific state and branch checks. A fresh proposed task under an active feature can still receive legal sibling prerequisites; the parent's state does not mean the new task has started. Amendment has no link, unlink, or order operation: `revise` re-pins accepted prose, not dependencies, and cannot unlock unchanged started work. A genuine replacement can use cancellation or supersession and fresh unstarted work with its own legal dependencies; never manufacture replacement merely to evade a lock. Unchanged eligible feature relocation instead uses reviewed `move-feature`, following `.tiller/orientation.md` and `tiller guide amend`. Run amend's read-only structural preflight before promising a parent/key correction or negotiating placement.

`tiller spec feature` is the assertion that the whole stage-2 output exists and is well-formed, and it checks all three parts. A missing or still-stub `feature.md`, `feature.eval.md`, or `task.md` is a blocked answer naming the file (exit `1`); a contract that is written but malformed, no criteria, or a criterion missing its binding or a required field, is misuse naming the offender (exit `2`). It never silently accepts a thin plan, so author the shapes exactly. After editing a specced feature's output, run it again: it re-validates, answering "already specced (no change)" only when the edited output still passes, and the plan gate applies the same checks before approving.

## The acceptance contract shape

`feature.eval.md` is YAML frontmatter followed by prose. Every criterion needs an `id`, a `statement`, and a `binding` whose `type` is one of `test`, `command`, `manual`, or `eval`. A `command` binding needs a `run`; a `test` binding needs a `ref` (or a `run`); an `eval` binding needs a rubric in `run` or `ref`; a `manual` binding needs nothing more. A required `preserve` criterion also carries a `baseline`, and its `artifact` names ONE repo-relative file (it is optional; omit it rather than inventing one). Never record a list of paths there: the paths a preserve criterion runs belong in the binding's own `run` or `ref`, and `tiller spec feature` refuses a list-shaped artifact naming the criterion.

A criterion states one observable behavior as a rule that holds, for example "page numbers are continuous across assembled forms", never a captured value, a golden output, or a pixel or paint budget. Those values belong inside the tests a criterion binds to, where a deliberate change updates one fixture instead of the contract. A look judged by eye is a `manual` criterion.

`preserve` is for one specific existing behavior this feature puts at risk, bound to a check that targets it. The tests gate already runs the whole suite on every check, so a preserve criterion that only re-runs the suite, or one spec of it, adds a baseline to refresh and proves nothing the gate does not. A targeted check exercises the at-risk behavior against its baseline, such as a characterization or comparison run like C5, not a re-run of a spec the suite already holds. When `tiller spec feature` warns that a preserve criterion runs the tests gate's own command, give it a targeted check, drop it, or keep it and say why in Notes from this run. When it warns that a criterion is a test the tests gate already runs, rewriting the ref as an explicit `run` of the same spec is not a fix: drop the criterion, or keep it and say why. A kept criterion warns again on every re-spec, and the Notes from this run answer it each time.

A `test` binding's `ref` is one repo-relative path or test id, or a YAML list of them when the criterion is proven by several files. Write the list form, never several paths joined in one string:

```yaml
binding: { type: test, ref: "spec/a_spec.rb" }
binding: { type: test, ref: ["spec/a_spec.rb", "spec/b_spec.rb"] }
```

Tiller checks that each path in the list exists and, without a `run`, appends every item to the tests command. A string such as `"spec/a_spec.rb spec/b_spec.rb"` is read as one ref, cannot be checked, and shows up as unresolvable in drift. An `eval` binding's rubric `ref` names one file.

The ref-only test binding below assumes the configured tests command accepts that spec path as an argument. Check that assumption using the command-selection guidance after the example.

```yaml
---
node: F-79dh3q
criteria:
  - id: C1
    statement: The form validates against its JSON Schema
    polarity: introduce
    binding: { type: test, ref: "spec/form_definitions/cert_lti_02a_spec.rb" }
    required: true
  - id: C2
    statement: Page numbers are continuous when a parent and child render to a single PDF
    polarity: introduce
    binding: { type: command, run: "bundle exec rspec spec/pdf/page_numbering_spec.rb" }
    required: true
  - id: C3
    statement: A reviewer confirms the rendered PDF matches the source form layout
    polarity: introduce
    binding: { type: manual }
    required: true
  - id: C4
    statement: The generated homeowner copy reads naturally
    polarity: introduce
    binding: { type: eval, run: "Grade whether the copy reads naturally for a homeowner." }
    required: true
  - id: C5
    statement: Existing single-zone projects compute identical results to pre-change
    polarity: preserve
    binding: { type: command, run: "bundle exec rspec spec/calc/single_zone_characterization_spec.rb" }
    baseline:
      surface: "app/models/calc"
      captured_at: { kind: git_tree, value: "git:3f6a9d2", path: app/models/calc, observed_at: "2026-06-23T00:00:00Z" }
      artifact: "spec/fixtures/calc/single_zone_2026-07-01.json"
    required: true
---

# Acceptance
```

The shape above and the binding rules in this section are the complete contract surface this build accepts; you need nothing outside this skill to author a contract. Acceptance lives at feature grain and nowhere else: a task has no contract, and an epic has none either, since its bar is the sum of its features' contracts.

### Selecting a test command

Read `gates.required.tests.run` in `.tiller/tiller.yml` and inspect the command it invokes, including any wrapper's argument forwarding. A test runner and an aggregate CI command are not interchangeable. Verify selector support from the implementation or documented behavior, not the executable's name.

For a `test` binding, an explicit `run` is the complete command: Tiller does not append `ref` when both are present. Without `run`, Tiller appends a space and `ref` to `gates.required.tests.run`. If neither an explicit command nor a configured tests runner exists, a bare test ref stays pending. Tiller does not infer a runner from a file extension or verify that an arbitrary shell command honors its arguments.

For example, `bundle exec rspec` accepts a spec path, so a ref-only binding can select `spec/form_definitions/cert_lti_02a_spec.rb`. A `bin/ci` wrapper that ignores arguments instead repeats its entire pipeline for every ref-only criterion. Keep that full CI command as the required tests gate, but give each test-bound criterion an explicit targeted `run`. Retain `ref` for traceability; it is not a second argument list.

Commands must work from the project root without depending on another criterion or an earlier CI run. Include required environment, test database, fixture, and compiled-asset preparation in the command or the project's existing targeted runner. For a Rails project whose selected spec needs test database and stylesheet preparation, the binding could be:

```yaml
binding:
  type: test
  ref: spec/form_definitions/cert_lti_02a_spec.rb
  run: >-
    RAILS_ENV=test bin/rails db:test:prepare tailwindcss:build &&
    bin/rspec spec/form_definitions/cert_lti_02a_spec.rb
```

Use the project's actual prerequisites and runner, not these example commands by default. Where the referenced tests already exist, verify the resolved command's output selects those tests rather than the whole CI pipeline. If an `introduce` test or its runner is still to be built, put that scope check in the implementing task's `Tests` section for execute to perform; do not implement tests during specify or claim a future command was verified.

Correcting an already approved binding changes accepted prose. For a non-terminal feature, preserve the criterion's intent and required flag, then use the human accepted-basis review in **Correcting the feature plan** below. Terminal contracts remain historical. Installing new guidance never rewrites or reapproves existing contracts.

## The task implementation spec shape

`tiller add task` seeds `task.md` with a stub carrying the headings below, and a stub is exactly what `tiller spec feature` refuses. Replace the body under each heading with the real plan.

```markdown
# Transcribe CERT-LTI-02-A (2025) into JSON Schema

## Approach

How the work gets done, in enough detail that someone else could pick it up.

## Reuses

What was searched, what already exists (file:line) and what this task calls instead of writing anew. If nothing was found, "None found" naming what was searched.

## Surfaces

The files and modules this task touches.

## Tests

The tests this task adds or changes, including the ones a contract criterion binds to.

## Edge cases

What could go wrong and what the task does about it.
```

Before planning new code, search the repository for existing helpers, components, and fixtures the task can call, and list each under Reuses with its file:line and what the task uses it for. A helper two tasks in the feature need is planned once. Build it in the task that naturally owns it, give every other task that needs it `--depends-on` to that task, and have both tasks' Reuses say so. It becomes its own small prerequisite task only when no task naturally owns it, so the work does not shrink into checklist-sized tasks.

## Grounding claims about code

Plans fail most often on confident guesses about code the plan does not own: a third-party library or plugin, a vendored or fetched dependency, generated output, or project code outside this feature's surfaces. For each concrete claim the plan rests on, such as a language or dialect, an entry point, a key or pattern, a variable's role, a value's type or format, a signature, or a default, read the code first and cite where it was read: the file and line, or the documentation section, plus the version, tag, or commit for code outside the repository, because outside code moves. This is the rule **Selecting a test command** applies to runners: verify from the implementation or documented behavior, not from a name. Cite only the claims the plan depends on; do not annotate every name it mentions.

When reading cannot settle a fact before the code that reveals it exists, write it as an open fact for the task to determine, naming the candidates and how to tell them apart: "determine the parser dialect from the plugin sources: Lua 5.1, or LuaJIT if any file uses `goto`". Never write a provisional answer marked as assumed. Finding a provisional answer wrong changes accepted prose; an open fact stays true whatever execution finds. When the outside code is unfamiliar, read it during specify rather than planning around a guess. A separate investigation task is justified only when reading cannot settle the approach, and it still yields its own reviewable output.

Before running `tiller spec feature`, re-read each cited claim against its source, through an independent reviewer when the harness provides one. A correction now is a free edit to unapproved prose; after approval, each one costs the human a re-pin.

## The contract-grain rule

Acceptance criteria must be satisfiable by the feature that carries them. The contract gates the feature, so a criterion the feature cannot green on its own deadlocks it. Deployment and cross-cutting criteria live in the feature that can satisfy them, not in whichever feature mentions them first.

Bad: an app-scaffold feature carrying "deployed to staging with Sentry capturing errors". That depends on another epic's infrastructure, so the scaffold feature can never satisfy its own contract. Good: that criterion lives in the observability/deployment feature, and app-scaffold carries "the app boots and bin/ci is green", which the scaffold itself proves.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`.

## Choreography

One feature, one pass, one gate remains the default. Do not hand a feature over between the contract and the tasks; the human reviews all three together. This two-feature example applies only when the human explicitly names both features as one batch.

```
# first named feature, form-engine/certification-form-loading (F-79dh3q): extend the specification at:
#   .tiller/epics/form-engine/features/certification-form-loading/feature.md
# write its acceptance contract (criteria, polarity, bindings) at:
#   .tiller/epics/form-engine/features/certification-form-loading/feature.eval.md
tiller add task form-engine/certification-form-loading/schema-transcription \
  --title "Transcribe CERT-LTI-02-A (2025) into JSON Schema"
tiller add task form-engine/certification-form-loading/computed-field-bindings \
  --title "Bind the computed fields" \
  --depends-on form-engine/certification-form-loading/schema-transcription
# write each task's implementation spec over its task.md stub
# final pass (Settle questions before approve): record each decision only the human can make
tiller add question "<statement>" --title "<short title>" \
  --blocks form-engine/certification-form-loading/computed-field-bindings
tiller spec feature form-engine/certification-form-loading
# a .tiller/-only diff needs no PR; commit only this feature's plan and the questions it recorded;
# Tiller never commits:
#   git add .tiller/epics/form-engine/features/certification-form-loading .tiller/decisions/open-questions.yml
#   git commit -m "specify: certification-form-loading"

# the ranked list suggests waiting; the explicit batch instruction already selected the next feature
# second named feature, form-engine/pdf-assembly (F-p4d8x2): extend feature.md and write feature.eval.md at:
#   .tiller/epics/form-engine/features/pdf-assembly/
tiller add task form-engine/pdf-assembly/continuous-page-numbering \
  --title "Render continuous page numbers across assembled forms"
# write the task's implementation spec over its task.md stub
tiller spec feature form-engine/pdf-assembly
# a .tiller/-only diff needs no PR; commit only this feature's plan:
#   git add .tiller/epics/form-engine/features/pdf-assembly
#   git commit -m "specify: pdf-assembly"

# stop for the human's decision on both complete plans; never run approve inside the batch
tiller readout specify   # the one final block that closes your reply, reproduced verbatim
```

`approve` remains outside the specify batch. After the stop, run `tiller approve feature form-engine/certification-form-loading` only on an explicit instruction naming the node; otherwise hand that exact command to the human. While a recorded question holds a plan, its answer comes first (**Settle questions before approve**).

## Correcting the feature plan

Human feedback on a specced but unapproved feature's prose stays at the same feature approval gate, not in an amendment. Revise the human-owned `feature.md`, `feature.eval.md`, or any proposed `task.md` directly. Use `tiller rename <noun> <node> --title "<display title>"` for display metadata, `tiller order` for preference, `tiller link` and `tiller unlink` for required sequence, and `tiller remove` for an unwanted proposed task. A specced feature is outside the draft, task-free key window: for an unchanged feature's key or parent correction, run the structural preflight from `tiller guide amend`, then stage an eligible `--move-feature` and stop at amendment approval. Do not cancel and redecompose merely to change its address. A genuinely abandoned or replaced plan still uses the human cancellation or reviewed replacement route. `tiller return` only replays a ready plan's approval and does not reopen feature key rename. Machine state changes only through the CLI, never through a hand edit to node YAML. Read the corrected plan with `tiller show` and `tiller tree`, commit the `.tiller/` diff, rerun `tiller readout specify --since <sha>` naming the commit the last block you reported, and stop at the applicable human gate. After a move a specced feature still needs `tiller approve feature <node>`; amendment approval is not feature-plan approval.

Approval does not freeze cosmetic titles, epic prose, additive child work, soft order, or same-scope prose on a non-terminal feature or task. Accepted prose on a done or cancelled feature or task is historical and is not revised in place. Accepted basis and active structural work still have teeth. After approval, map the correction by class:

| Correction | Route |
|---|---|
| Approved epic, feature, or task display title | Run `tiller rename <noun> <node> --title "<display title>"`; no amendment. |
| Approved epic narrative wording | Edit `epic.md` and commit it; Git is the audit record because epic approval pins decomposition shape, not an epic prose basis; no amendment. |
| Approved non-terminal feature or task prose, same scope | A wrong factual claim found while executing a task is an execute departure, recorded in its PR, not an edit here. Otherwise edit the pinned Markdown and commit it, read `active_basis` with `tiller status <noun> <node>`, then stop for the human to run `tiller approve <noun> <node> --refresh-basis`. |
| Done or cancelled feature or task historical prose | Do not edit the accepted Markdown in place or prescribe `--refresh-basis`. Route the correction into forward additive or replacement work; use `amend` and supersession as appropriate so accepted history stays intact. |
| Ready feature or task plan whose approval should replay | Run `tiller return <noun> <node> --reason "<why the plan is being revised>"`, revise it, and stop for the human to approve again. |
| Approved feature needs one more task | Run `tiller add task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>"`, write the proposed `task.md`, and stop for the human to run `tiller approve task <node>`; the additive task has its own approval and needs no amendment. |
| Preferred sequence | Run `tiller order <noun> <node> [<node> ...]`, where noun is epic, feature, or task; soft order never gates. |
| Required sequence before affected work starts | Add the hard edge with `tiller link <noun> <node> --depends-on <target>`; remove a mistaken edge with `tiller unlink <noun> <node> --depends-on <target>`. |
| Dependency change after affected work starts | Explain that an in-place edge edit is unsupported. Amendment does not unlock it or stage links/unlinks; only a genuine replacement can use fresh eligible work with new edges. Never manufacture replacement to evade the lock. |
| Genuine replacement or cancellation after accepted work starts | Route the structural reshape to `amend`, using its cancel or supersede operation and fresh work as appropriate, preserving accepted history. |
| Unchanged feature key or parent correction outside the draft, task-free window | Run amend's preflight; eligible draft, specced, or ready features with never-started proposed/ready descendants use reviewed `move-feature`, preserving identity and stopping at amendment approval. A started descendant refuses precisely; do not manufacture replacement. |

An explicit human correction that matches this table authorizes the direct command or prose edit in that row. Execute it without asking the human to translate their correction into Tiller vocabulary. Then read back the state, commit the correction, rerun the current readout with `--since <sha>` naming the commit the last block you reported, and return to the row's existing approval, refresh, reapproval, task-approval, or amendment gate. When the table answers, the agent does not ask which mutation path to use.

## Settle questions before approve

After the plan is written and before `tiller spec feature`, make one final pass over the whole plan: the criteria against each other and against `feature.md`, each `task.md` against the code it names, and the task plans against each other. Sort every finding three ways:

- Settled by reading: fix the prose and record nothing.
- Settleable only by code written later: write it as an open fact (**Grounding claims about code**).
- A decision only the human can make, about product behavior, acceptance, or scope: record it with `tiller add question "<statement>" --title "<short title>" --blocks <node>`.

`--blocks` names the narrowest node the answer could change: the one task when only its plan depends on the answer, the feature only when a criterion does. A question pinned to the feature that touches one task still holds the whole plan's approval. A question asked only in chat is not allowed: no gate and no later session can see it.

`tiller approve feature` refuses (`open_question_blocker`) while an open question holds the plan, one naming the feature, its epic, or any of its tasks not cancelled, and `tiller approve task` on a task added after the plan gate refuses while one names that task or an ancestor. There is no `--assume-question` escape at a plan gate. A human not ready to decide answers with the call they would make for now ("build it as planned; revisit after launch"), which folds in like any other answer and leaves a record. A question that needs no decision at all (asked in error, or overtaken) leaves by `tiller cancel question <Q-id> --reason "..."`, and `tiller unlink question <Q-id> --blocks <node>` narrows what it holds. Both are the human's moves: run them only on their instruction, otherwise name the exact command in your reply. If a question reaches further than recorded, you may widen it with `tiller link question <Q-id> --blocks <node>` and say so.

The human answers with `tiller answer question <Q-id> --answer "<text>"`, or in conversation, where you record it with that same command on their instruction. Write each answer into the prose it changes (a criterion, `feature.md`, or a `task.md`), re-run `tiller spec feature`, commit the `.tiller/` diff, and rerun `tiller readout specify --since <sha>` naming the commit the last block you reported. Execute builds from `task.md`, so an answer left only on the question leaves the plan contradicting it. Tiller never checks that an answer reached the prose; the human reviews that in the diff before approving.

## Readout

End by running `tiller readout specify` and reproducing its block verbatim as the last thing in your
reply to the developer. The block is Tiller's, not yours: it renders the same whoever is driving, which
is what lets a developer learn to read it once instead of learning your habits. Do not restate it in
prose around it, the criteria and their binding types included: the block already names them on its own
authority. The block closes the reply: nothing follows it. It ends on its **Next:** ranked list, the
one place the developer looks to learn what happens next, so reproduce the list as written and never
add a list of your own after it.

Before the block, add a short **Notes from this run** list for what it cannot know: an interpretation call
behind a criterion's binding, which criteria are satisfiable by this feature and why, how tasks stay within
this increment, material boundary or prerequisite-versus-soft-order decisions, the open facts this plan leaves for execution to determine, each question this run recorded by id and title with what its answer would change, which answer changed which file once answers are folded in, and how each `spec feature` warning was resolved (a targeted check, a dropped criterion, or why the criterion stays), so the human weighs them at approval. While a recorded question is open, the block's **Next:** list names its answer, not the approve, since the queue hides an approve an open question holds. Name later discoveries,
their affected work, and proposed correction routes without silently expanding the batch. Flag anywhere
`feature.md` still wants the human's own words. Keep it to facts about this run. In the notes and in everything else you write to the
developer, name every epic, feature, and task the way the block itself does, key path with the id in
parentheses, never a bare id: an id alone sends the developer off to look up what it points at.
Commentary is yours to add freely, with the notes ahead of the block, never after it; a question for
the human is a recorded question, never a line of chat.

## Hand-off

Up to the human, who owns the spec prose, edits it, answers its recorded questions, and approves the whole plan once specify has folded those answers in. One approve takes the feature to `ready` and its proposed tasks with it, so the tasks arrive at `execute` already ready and there is no second approval to wait for. One vocabulary trap at this gate: `approve` green-lights a proposed plan to start, and it is the only gate verb in this build. There is no `accept`: completed work crosses to done when the human merges and `tiller sync` observes it, so when someone says "accept this", translate by the node's state, never guess. Out to craft for the tests the bindings name and the code the task specs describe: specify says what must be true and how the work is shaped, and `execute` with the project's `AGENTS.md` and `CLAUDE.md` does the building.
