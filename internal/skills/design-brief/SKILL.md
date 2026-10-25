---
name: design-brief
description: Use only when the developer explicitly requests a UI/UX design brief or prototype handoff for an epic, feature, or task. Reads the selected work and its context, returns one self-contained Markdown snapshot for a non-production prototype, and stops without changing source requirements or Tiller state.
---

# design-brief

## Trigger

The developer explicitly asks for a UI/UX design brief or prototype handoff for an epic, feature, or task. Accept a canonical key or ID. Resolve conversational references such as "this task" only when the conversation identifies one target unambiguously. Otherwise ask which target they mean, using available node reads to name the candidates rather than guessing.

Merely entering a project, encountering a draft feature, reaching a gate, finishing work, or asking what to do next does not trigger this skill. Bugs, chores, and tweaks are outside its target scope. An explicit brief request can be served regardless of the ranked list's top move or the target's lifecycle gate; it authorizes an export, not crossing that gate.

## Reasoning

### Gather requirements, not design authority

Tiller supplies the recorded structure and facts. Your work is to select the requested experience, carry its substantive requirements into a portable brief, and expose uncertainty. The receiving designer or design tool owns visual composition and prototype construction. Do not prescribe a new design methodology, component architecture, or implementation process. Carry the project's actual constraints; label any reversible choices needed to make the prototype concrete as suggestions.

The brief is a derived snapshot, not a new canonical specification, an approval, or a prerequisite for execution. A detailed brief does not imply a detailed or approved source contract. Draft and narrative-only work are eligible: retain their maturity and missing detail rather than entering `specify` to fill them in.

### Enumerate the target scope before synthesis

Ancestry supplies intent and applicable constraints, not permission to expand the request.

| Target | Required inspection and prototype scope |
|---|---|
| Epic | Enumerate every included feature, with its state, specification maturity, and retirement or replacement context. Read each feature's narrative and contract where present, plus relevant task detail. Describe the coherent experience, navigation, and cross-feature journeys. Never silently sample a subset. |
| Feature | Read the feature narrative, full acceptance contract where present, epic context, and relevant task specifications. Cover the complete feature experience, not unrelated sibling screens. |
| Task | Read its implementation spec and state, parent feature narrative and contract, and epic context. Map the task to relevant parent criteria. Include only the surrounding UI needed to demonstrate its specific change, not the whole feature as implementation scope. |

Account for cancelled and superseded descendants instead of treating them as current requirements. An archived epic or explicitly requested retired target is historical scope; identify that fact and the recorded replacement context. Do not silently redirect the request to its replacement or revive retired screens. Retain applicable existing behavior from completed work without treating every historical detail as a new requirement.

Dependencies and adjacent work matter only where they affect entry points, navigation, displayed information, prerequisites, or shared behavior. A data dependency does not automatically bring its screens into scope. Keep a scope inventory while gathering so every included epic feature is accounted for, even when its experience cannot yet be specified.

For a task with no direct UI change, report the supported no-UI finding and any indirect user-visible constraints. Keep the seven-part structure, adapting screen and prototype sections to explain why no new surface or interaction is required. Do not invent screens to fill it. If the available sources do not establish whether UI changes are needed, mark that as unresolved instead of claiming no UI is required.

### Establish source authority separately from state

Use these labels at the affected behavior or constraint, not only in an opening disclaimer:

- **Required:** supported by an applicable source requirement or recorded decision. Qualify draft, approved, or discoverably unreviewed content; "Required (draft source)" is not approved scope.
- **Suggested for the prototype:** a reversible choice such as sample content, an unspecified layout, or a simulated treatment. It is not a new product requirement.
- **Unresolved:** missing information, unsupported assumptions, or conflicting sources that could materially change the experience.

For example, a confirmation step can be Required, a modal Suggested for the prototype, and who may confirm Unresolved. Preserve those separate meanings.

Distinguish what exists now, what must change, and what must remain unchanged. Read `introduce` and `preserve` criteria with their required flags and any preservation baseline. Observed implementation is evidence of current behavior, not automatic authority over intended behavior. A design document describes intent or a recorded baseline; it does not by itself prove that a screen exists. Do not silently choose code over prose, newer prose over a recorded decision, or a detailed task over a conflicting parent requirement.

Default to the current working-tree requirements. Identify the current revision context and relevant staged, unstaged, added, or deleted source files where discoverable. A HEAD commit alone does not identify dirty source content. Do not silently substitute historical requirements for the developer's checkout.

Read each relevant feature's and task's `accepted_against`, including full RevisionRefs in the state YAML when the CLI projection is insufficient. Feature approval pins current `feature.md` and `feature.eval.md` content; a task pins its own `task.md`. Acceptance contracts exist only at feature grain. A task has no separate eval contract and an epic has none.

The `accepted_against.commit` is HEAD at approval, not necessarily the approved prose: approval can fingerprint dirty working-tree bytes. The `spec` and `eval` blob references are the content basis. Epic approval accepts decomposition shape, not an approved epic prose revision. Node state such as `ready` or `done` must not be used to assert that today's edited prose is approved.

Inspect node-status `active_basis` where applicable, including the parent feature separately for a task. A task status may name its parent's drift first. Absence of a drift line is not proof of unchanged approved content: there may be no pin, a terminal-node exemption, or an unavailable comparison. For completed work, `verified_against` is completion context, not a replacement for the approval basis or proof of prototype acceptance.

Use read-only Git inspection to compare current requirements with recorded content when available. Strip the `git:` display prefix before giving an object ID to Git. A recorded blob hash may not exist in the local object store, especially when approval fingerprinted uncommitted prose. A missing object, shallow history, unavailable external revision, absent pin, or missing file remains an explicit limitation. Never reconstruct an approved text by assuming the approval commit contains it, and never claim a comparison you could not make.

Follow recorded decisions, open questions, answers, and `assumption_basis` for the target and relevant ancestors or descendants. Distinguish an assumption under which work proceeded from a settled product decision. An answered question may not yet have been reviewed into the plan. Surface material source conflicts at the affected interaction and in open decisions without inventing a precedence rule.

Missing references and inaccessible assets are explicit gaps. Search hits, design-document frontmatter, and index files are discovery aids, not a mechanically complete catalog. Follow applicable prose references and inspect relevant documents. Do not claim fidelity to something you did not inspect. Ask for clarification only when a material ambiguity prevents a useful bounded brief; preserve all reachable requirements and explain the blocked decision.

### Keep generation read-only

Only context reads and the explicitly requested deliverable write are permitted. Do not change canonical prose, source code, state YAML, the manifest, claims, gates, events, questions, approvals, acceptance criteria, Git history, refs, or index. Do not invoke `join`, `migrate`, `eval`, `check`, `sync`, or other state-writing commands. Do not claim, start, specify, approve, attest, record evidence, or commit anything. Instruction refresh is a separate developer operation, never an export prerequisite to perform here.

Do not enter `specify`, `mend-drift`, or another lifecycle skill to repair missing or contradictory sources. Read-command recovery is limited to correcting the read invocation or using accessible source files. If the CLI reports an environment, schema, or recovery problem, report the limitation and continue with safely readable sources where useful; do not repair the project to make the export work.

Inspect accessible implementation, components, design guidance, and assets through filesystem reads. Do not launch application operations, install dependencies, build, run tests, execute acceptance bindings, or run scripts that could change the project merely to gather context. Git inspection must remain local and read-only: no checkout, fetch, stash, index refresh, object writes, or basis refresh. Do not follow a recommended mutation printed by a status command.

## Choreography

### 1. Resolve and read

Use the existing delivered CLI. Confirm unfamiliar forms with `tiller <command> --help`, not design-only commands copied from old documentation. `show` exposes node state; read the prose and full contract files from the filesystem as well. The available forms for this work are:

Disable Git's optional index writes for every Tiller read as well as direct Git inspection: use `env GIT_OPTIONAL_LOCKS=0 tiller ...` on each invocation. Tiller reads can invoke Git internally, and a plain `git status` may refresh index metadata even though no source content changes. Do not assume an environment assignment in one shell call persists into the next.

```sh
env GIT_OPTIONAL_LOCKS=0 tiller status
env GIT_OPTIONAL_LOCKS=0 tiller show <epic|feature|task> <key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller status <epic|feature|task> <key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller tree epic <epic-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller tree feature <feature-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller ls features --epic <epic-key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller ls tasks --feature <feature-key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller ls questions
env GIT_OPTIONAL_LOCKS=0 tiller show question <question-id>
env GIT_OPTIONAL_LOCKS=0 tiller query --from <work-id> --follow mentioned_by,sourced_from --json
```

Run only the applicable forms; `tree` roots at an epic or feature, not a task. The existing global `--json` option can expose structured read results. Do not filter the initial descendant inventory to active or ready work, since that would hide draft and retired nodes. Read relevant ancestors and descendants by their resolved keys or IDs. In the brief, name nodes with title and canonical key path plus ID in parentheses, not a bare ID.

Read `.tiller/tiller.yml` for project identity, inputs, and relevant conventions. Use the node file paths from the tree and read the target prose, state, ancestor prose, and feature acceptance contract. A draft feature may legitimately have no `feature.eval.md` or task subtree. Read applicable `.tiller/design/` documents and their prose references, recorded decisions and questions, relevant assumptions and dependencies, recorded surface paths, and accessible current UI, components, design-system rules, and assets. Respect declared external reference roots as read-only context.

Relationship queries are discovery, not a replacement for reading sources. Follow names form a union at every hop; filters select after traversal. Inspect source file/line, owning work, generated-document labels, and the response's diagnostics and `complete` before treating a reference inventory as exhaustive. Recorded path coverage does not verify implementation, and question targets do not establish effective inherited holds or assumption waivers. Unknown targets and ambiguous historical identities remain unresolved; opening a current file does not retrieve its pinned historical bytes. Report incomplete facets as brief limitations without repairing state, refreshing guidance, or executing bindings.

### 2. Inventory scope and compare authority

Apply the target-grain table before writing journeys. Account for every epic feature and its maturity or retirement; select relevant task detail without importing unrelated siblings. Record the source paths and sections that establish each important behavior, preservation rule, and unresolved issue.

Use local Git reads where available, with optional index writes disabled and external diff or text-conversion helpers disabled:

```sh
git --no-optional-locks rev-parse HEAD
git --no-optional-locks status --porcelain=v1 --untracked-files=all
git --no-optional-locks diff --no-ext-diff --no-textconv -- <source-path>
git --no-optional-locks diff --cached --no-ext-diff --no-textconv -- <source-path>
git --no-optional-locks cat-file -p <recorded-blob-object-id>
```

Compare recovered blob content to the current file without writing a temporary source copy. If a current raw-content fingerprint helps, `git --no-optional-locks hash-object --no-filters -- <source-path>` computes one without writing it; never add `-w`. Inspect relevant untracked files directly because ordinary diffs omit their content. Resolve recorded paths against the actual project layout rather than assuming every stored path includes `.tiller/`. Report unavailable history or content comparison honestly and retain the working-tree default.

### 3. Synthesize one seven-part Markdown brief

Return a single self-contained document with the following seven numbered sections. Adapt depth to the target. Put most detail into journeys, surfaces, interactions, and state-dependent behavior rather than project background. Carry the substantive requirements into the text: source links provide traceability, never a repository-access prerequisite for the receiving tool.

#### 1. Purpose, source basis, and scope

Name the project, target title, canonical key and ID, ancestry, user outcome, and boundaries. For an epic, include the feature inventory and its maturity or retirement. Separate in-scope work from contextual surroundings and exclusions. Identify the source revision context, relevant working-tree changes, approval bases and comparison limitations, and whether historical scope was requested.

State that this is a derived handoff snapshot, not an approved replacement for source specifications. Provide source paths with section or criterion references for important requirements, and the substance those references support. Identify inspected sources and gaps without claiming unseen assets or documents were incorporated. The snapshot may become stale; source identification is not automatic drift tracking.

#### 2. Users and operating context

Describe actors, relevant permissions, entry points, prerequisites, and the goals users bring to the experience. Carry established product terminology into the brief and explain it sufficiently for someone without repository access. Mark unknown permissions or actor distinctions as Unresolved rather than selecting a convenient policy.

#### 3. Experience overview and journeys

Describe necessary screens or surfaces, navigation between them, and end-to-end journeys the prototype must demonstrate. Identify the start, meaningful actions, transitions, and expected visible outcome of each journey. Distinguish in-scope work from surrounding UI rendered only to make a journey understandable. For epic scope, represent supported cross-feature journeys without fabricating connections to narrative-only work.

#### 4. Detailed screen and interaction requirements

For each relevant surface, describe its purpose, information hierarchy, displayed data, controls, actions, validation, feedback, navigation, and conditional behavior. Distinguish existing behavior, intended changes, and behavior to preserve. Include applicable loading, empty, error, success, permission, and destructive-action states, with recovery paths where the requirements establish them.

Do not invent every conceivable state. Identify an important unspecified state as Unresolved or give a clearly labeled Suggested for the prototype treatment. Label Required, Suggested for the prototype, and Unresolved at the behavior concerned, retaining draft or unreviewed qualifications.

#### 5. Visual and interaction constraints

Carry applicable design-system rules, existing components, assets, responsive requirements, and accessibility constraints. Explain their relevant substance rather than relying on repository links. Name inspected assets and any assets that must be supplied separately; a local asset path does not give the receiving tool the file. If an asset is unavailable, state the gap and label any substitute as provisional.

Default to strict behavior and scope with freedom in visual composition unless the project constrains it. Additional design recommendations are suggestions, not inherited requirements. Do not claim an inaccessible design system was followed.

#### 6. Prototype instructions and sample data

Define what must be clickable, which state transitions must be simulated, and how a reviewer reaches each significant scenario. Provide coherent synthetic data that exercises the required distinctions and edge cases. Label sample values and unspecified treatments as Suggested for the prototype. Never export real customer records, credentials, secrets, or private production payloads to make examples realistic.

The deliverable requested from the receiving tool is a non-production prototype. Local interactions and simulated state are allowed. It must not require a production backend, real authentication, live integrations, or production writes. Represent roles with simulated contexts rather than actual login. Distinguish a displayed or simulated success from actual completion of a domain operation. Include only the simulation needed for the required experience, not a production implementation plan.

#### 7. Coverage and open decisions

Map every UI-relevant acceptance criterion to identifiable, reachable screens or scenarios, naming the source feature and criterion ID. Preserve introduce/preserve polarity and required/optional distinctions. For task scope, identify the relevant parent criteria and which portions the bounded task prototype can demonstrate; account for other parent criteria as outside this task's scope rather than expanding the work. For epic scope, cover the included features' contracts and explicitly name missing contracts or narrative-only areas.

Account for non-visual criteria as outside the prototype's proof. A simulated journey does not prove backend correctness, security enforcement, performance, production integration, or a passing acceptance binding. Do not drop those criteria or claim the prototype satisfies them.

List unresolved questions, material source conflicts, missing references, unsupported assumptions, and prototype-specific suggestions. Separate reversible decisions the designer may make from product decisions requiring human resolution. Missing requirements remain gaps, not new approved criteria. Include supported no-UI findings and indirect constraints where applicable.

### 4. Review coverage and export safety

Before delivery, check the brief against the gathered scope and sources. Every included feature is accounted for, UI-relevant criteria have reachable scenarios or explicitly named unresolved blockers, preservation rules remain visible, and non-visual criteria are outside proof. Required behavior must not become optional merely because it is inconvenient to prototype. Unsupported layout choices, sample values, and provisional treatments must not read as product requirements.

Review all exported prose, examples, URLs, and asset references for customer data, credentials, and secrets. Use synthetic substitutes while preserving the meaningful behavior distinctions. Do not copy sensitive payloads out of a source just because the source is a requirement. Explain any material redaction without exposing the protected value. Ensure the receiving tool can understand the requirements without opening repository links or unavailable assets.

### 5. Deliver and stop

By default, return the complete Markdown brief in the reply without creating any file. If the developer explicitly requests a file, write only that deliverable at the requested destination and identify it in the reply. The file must contain the whole brief, not a wrapper referring back to the conversation.

A file request does not authorize replacing canonical prose, source requirements, or state. Check the destination before writing, including whether a symlink would redirect the write. Preserve an existing human-edited brief unless the developer explicitly authorizes replacing that output. Regeneration alone is not replacement authorization: use a new explicitly requested snapshot destination or return the new snapshot in the reply and ask for a destination if needed. Never silently overwrite an existing file.

Do not create a mandatory output directory, YAML sidecar, generated index, reference loader, extra asset copy, or automatic refresh mechanism. Do not upload, submit to an external tool, install a design tool, or begin prototype construction. No vendor-specific API, account, installation, or receiving-tool dependency belongs in generation.

## Hand-off

The one Markdown brief is the export. There is no `tiller readout design-brief` seam, no lifecycle readout to append, and no mutation or event to manufacture. This narrow read-only exception does not change any lifecycle skill's route, recovery responsibilities, or human gates.

Stop after delivering the brief. The developer decides whether to hand it to a designer or design tool and which explicitly identified assets to supply. The receiving tool owns prototype craft. Product changes or conflicts discovered through design return separately to the existing human-reviewed planning process; generation neither records them nor approves them. Do not continue into specification, execution, source repair, design approval, or the next queued work.
