---
title: Skills
description: The guidance tiller serves to a coding agent, and how a harness finds it.
---

Tiller's agent surface is the CLI plus a small set of skills, and the binary itself serves them:

```sh
tiller next             # your ranked next moves, top five
tiller guide            # the same list, then the skill for the top agent move
tiller guide execute    # prints that skill in full
```

There are nine skills in v0.11.0. `orient` reads state, names the next move, and stops. `derive-baseline` derives or translates as-is docs and stops at human per-doc confirmation. `decompose` lays a draft epic and feature tree for the human to approve. `specify` writes one feature's specification, acceptance contract, and tasks, then runs `tiller spec feature`. `execute` runs the task loop and stops at the resolved task, feature, or epic review surface. `capture` writes one complete proposed bug or chore and stops before human triage. `mend-drift` reads a drift candidate and drafts the smallest honest fix. `amend` stages a reshape for human approval. `intake` lands a document written outside the tree and opens the amendment it implies.

Current source carries eleven skills, including the explicitly requested `design-brief` and `code-review`, schema 12, orientation generation 57, and version stamp 0.21.2. The latest published release is v0.18.4. Earlier releases retain their historical inventories. Source guidance alone does not establish installation or checkout refresh; the build status records observed adoption.

In current source, `orient` relays the ranked next list: every legal move for your checkout, human decisions included, ranked by where you have been working, at most five. It names any of your decisions that rank above the top agent move, ends its reply on the list, and starts that agent move once your reply does not redirect. Tiller ranks by a fixed rule and never calls a model; the agent may say it would take a different row first, but it reproduces the list as printed.

`capture` never approves, claims, starts, resolves, or cancels what it wrote; triage is the human's `tiller approve bug`, `tiller approve chore`, or `tiller approve tweak`. Once a bug, chore, or tweak is ready and claimed, `execute` runs it the way it runs a task, on its own branch, with the attached node's context read first, a regression test where the work is a fix, task-grade gates, and a stop at the human's merge. Shelf work ignores Review Grain and runs no eval.

## Plan outcomes through the known scope

The four planning seams share one principle: recommend bounded delivery increments from documented actors, goals, workflows, and outcomes, not permanent subsystem buckets. Consumers may be developers, operators, or API clients; product-driven does not mean screens-first. Keep genuine engineering prerequisites, but implement shared machinery for its first consuming increment rather than prebuilding every later capability. Concrete migration or compatibility risk can justify a bounded foundation; explain the tradeoff in the existing review.

- **Decompose** proposes the epic and feature tree through the full known initial scope, including later requirements. Infer the progression before choosing containers, then audit every actual prerequisite after targets exist. Record prerequisites as `depends_on` and preferred sibling order with `tiller order`. Independent epics need no invented cross-epic edges.
- **Specify** writes the selected feature's complete spec, acceptance contract, and task breakdown just in time. Technical tasks may have real dependencies without each being independently useful to an end user. Shared tasks serve this increment, not speculative later needs. Discoveries that affect future scope or order use existing correction routes without silently widening the named batch. Each concrete claim the plan rests on about code it does not own cites the file and line or documentation section it was read from, with the version, tag, or commit for code outside the repository, and specify re-reads those citations before `tiller spec feature`. A fact only building can settle is written as an open fact for the task to determine, never a provisional answer marked as assumed, and the run notes list those open facts for the human's approval.
- **Amend** reshapes only the requested affected scope. Extend prior containers for the same bounded outcome; choose a new increment for a distinct outcome or delivery phase, even within the same subsystem. Completion alone neither requires nor forbids continuation. Preserve terminal prose and completed descendants; broader replanning needs developer direction.
- **Intake** reads the whole feedback document, preserves technical and security requests, and maps findings onto bounded outcomes with honest source citations on every operation. Splitting findings must retain coverage. Stage an epic before same-batch features; tasks under a new feature follow materialization and ordinary specification.

These are recommendations for the existing review, not a new gate. Missing information warrants questions only when it materially changes boundaries or sequence. Planning permission does not approve the proposal, authorize arbitrary reordering of an accepted tree, or start work. Amendment approval appends new epics to root soft order; intended placement is a separate explicit `tiller order` after materialization. New hard links remain subject to the dependent's eligibility.

Updated skill bodies come from the running binary. For existing checkouts, refresh shared orientation and discovery descriptions with the intended binary's `tiller upgrade --project-only`, review its generated diff, and reload agent instructions. This does not rewrite or replan accepted work.

A rich harness discovers the skills on its own. `tiller join` installs thin shims where the harness looks (for Claude Code today, `.claude/skills/tiller-<skill>/SKILL.md`), and each shim tells the agent to run `tiller guide <skill>` and follow what it prints, so the text always comes from the project's own binary. A harness with no skill system reads the managed block in `AGENTS.md`, then `.tiller/orientation.md`, and reaches every skill through `tiller guide` the same way. The full design, including the delegation model and what an agent may and may not do at each gate, is in the [skill set spec](/tiller/spec/skill-set/).

## Request a design brief

Ask the agent directly:

> Please give me a design brief for feature F-9oymbc.

An epic or task works too: "Generate a prototype handoff for epic E-t73jkv" or "Generate a design brief for this task" when the conversation identifies the task unambiguously. Keys and IDs are accepted. Bugs, chores, and tweaks are outside this export's target scope.

```sh
tiller guide design-brief
tiller guide design-brief --json
```

These commands print the skill instructions, not a generated brief, and take no target-node argument. The agent reads the selected work, applicable ancestry and descendants, acceptance criteria, design documents, and accessible UI sources. An epic accounts for every included feature; a feature excludes unrelated siblings; a task includes only the surrounding experience needed to demonstrate its change. Draft and retired inputs are identified, not silently approved or revived.

The result is one self-contained Markdown brief with seven parts: purpose, source basis, and scope; users and operating context; experience and journeys; screen and interaction requirements; visual and interaction constraints; prototype instructions and sample data; and coverage and open decisions. It carries the requirements, not just repository links, so a receiving design tool can work from the brief alone. Assets that need separate delivery are named.

Current working-tree requirements are the default. The brief distinguishes required behavior, reversible prototype suggestions, and unresolved inputs, preserving draft or approved status and identifying discoverable changes from feature and task `accepted_against` records. An approval commit does not necessarily contain the approved dirty prose, and epic approval accepts tree shape, not epic prose. Missing history, source conflicts, and inaccessible references stay visible. Existing behavior, intended changes, and preservation requirements remain distinct.

UI acceptance criteria map to reachable scenarios; non-visual criteria remain outside what the prototype can prove. The prototype uses synthetic data and simulated state, never real customer records, secrets, production writes, live integrations, or real authentication. A non-UI task produces an honest no-UI finding and indirect constraints, not invented screens; insufficient evidence is not the same as no UI change.

The brief belongs to the developer and is a derived snapshot, not a replacement specification or acceptance decision. By default it is returned in the reply. An explicit file request permits only that deliverable write; an existing human-edited brief is preserved unless replacement is explicitly authorized or a new snapshot destination is used. No mandatory storage directory or automatic refresh is added.

Generation changes no source requirements, Tiller state, claims, gates, events, or Git history. It neither uploads the brief nor starts building the prototype. It does not repair sources, run gates, refresh instructions, or enter another lifecycle skill to fill a gap. There is no export command or `design-brief` readout seam, and no terminal lifecycle readout follows the brief. Product changes discovered in design return separately to human-reviewed planning.

Only an explicit request triggers the export. Entering the project, finding a draft feature, or finishing work does not. Ordinary orientation, next-work routing, and every human gate remain unchanged; a design brief is never an execution prerequisite.

## Establish an existing-code baseline

On brownfield and legacy-migration, the baseline moves are the only next moves until one ledger doc is confirmed, even if work already exists: `derive-baseline` when no doc is registered, otherwise your confirmation of each pending doc. The skill reads existing docs first, then brownfield surface roots or legacy reference roots. It derives undocumented code or translates existing material at module grain: what the surface is, data it owns, behavior to preserve, edges, and described paths.

The skill asks once whether in-repo source ownership moves into `.tiller/design/` or another consumer needs a pointer page. It never silently leaves duplicate live specs. External `--design` imports remain source-preserving; external originals are never retired. Commit exact uncommitted inputs first, record their recoverable project-HEAD blobs through `link design --from`, then retire only the agreed in-repo sources. Commit the reviewed translation and ledger on the explicit integration branch without staging unrelated work. `project.adopted` is historical context for this project; reconcile current code too, and say when adoption history is unavailable.

```sh
tiller guide derive-baseline
tiller link design as-is/editor.md --describes app/editor
tiller link design as-is/editor.md --from docs/editor-spec.md
tiller readout derive-baseline
# Human, or an agent explicitly instructed to approve this named doc:
tiller approve design as-is/editor.md
```

The readout shows every recorded source path and blob mapping, every ledger doc, described paths, and confirmation facts. It does not prove that all source meaning survived. Separate run notes identify dropped, merged, and unplaceable material, source/code disagreements, and questions actually opened. `Open questions` is the current project count; no dropped count can be computed from the ledger.

Reproduce the readout verbatim and stop for human review against sources and working-tree code. The completed baseline readout names one HUMAN next action, not another derivation. If docs are pending on re-entry, inspect and reconcile them or return them for review instead of regenerating them. Approval records current code fingerprints and this project's HEAD, not an adopted or external HEAD and not prose bytes. One accepted doc releases the startup route; other readiness gaps remain independent, and remaining unconfirmed docs are not a global blocker.

After code changes, `tiller drift` detects tracked-work baseline candidates through `stale_design`
and broader working-tree changes through opt-in `shifted_design`. Sync only names touched docs;
it never updates the ledger. `mend-drift` reads feature specs and bounded history for tracked
crossings, or the current working tree for broader changes. It corrects the actual prose,
including a pointer page's source, commits the proposal, and stops for human review and
`tiller approve design <doc>...`. A commit anchor does not promise that dirty or external
confirmation bytes were stored in Git. Automatic uncovered-root routing is not implemented.

## Execute at the resolved review grain

`execute` reads Review Grain from the project, epic, and feature state the binary already loaded. It accepts owned work in `ready`, `in_progress`, or `check`, requested changes to reviewed work, and coarse review finish lines. Checked work re-enters without `start` or `resume`; green gates are intermediate, not a completed handoff. Task grain finishes with the task's recorded and published review. Feature and epic grain keep going until the resolved node's own gates, acceptance work, and publication are complete. A riding task records its permitted review ref without a task PR; name the resolved feature or epic, but do not claim its PR exists until it is recorded. Shelf work always owns its branch and review, and live carried items do not enter their own loop.

Every task has its own recorded working branch, unchanged on resume. Independent tasks start from integration; task-PR dependencies wait for integration rather than stacking PRs. Feature/epic reviews use recorded aggregates under `reviews/features/<epic>/<feature>` or `reviews/epics/<epic>` in the configured branch pattern. The agent creates these branches and ordinary-merges each submitted task tip, including submission-state commits, into the aggregate. A coarse dependent task may start from that aggregate once its prerequisites are included. Before feature checks, draft publication, and coarse submit, assemble all current riding task tips and their recorded heads. No cherry-pick or squash assembly. Requested changes return to the affected task branch, then reassemble and recheck the same aggregate PR. Mechanical conflicts are agent work; semantic conflicts and acceptance need human decisions. Only the final integration merge is human-only.

## Claim one execution unit

Execute claims tasks individually with `tiller claim task <ref>`, just as it claims bugs, chores, and tweaks. New tasks are unclaimed regardless of creation/approval actor. Before start, reservations can be released or human-directed reassigned; taking another reservation needs its actual holder in --from and explicit handoff. A first successful start atomically activates a ready feature and permanently fixes the task owner. Resume, review, checks, cancellation, and completion never reset ownership; all terminal ownership is immutable.

Features/epics have Contributors derived from started tasks, not exclusive owners. Past contribution is not unfinished responsibility: personal work includes nonterminal task/shelf assignments, including proposed/blocked/checked/in-review. Any registered developer may perform authorized aggregate assembly/checks/submission. Shared closeout appears once per feature/epic, not once per contributor or last finisher; coordinate one updater at a time through Git. A suggestion is not authorization. Judgment actor and human-gate rules remain unchanged. Feature-only assumptions use `tiller approve feature <ref> --assume-question <Q-id> --reason <text>` without a lifecycle move or automatic task inheritance.

## Transfer only on explicit human instruction

`tiller transfer task|bug|chore|tweak <ref> --to <registered-contributor> --reason <text>` is for started unfinished work, explicitly named with its recipient by the human. Before start use reassign. Transfer preserves approved scope and original identity/prose/owner/review/evidence, cancels the original, and creates an approved unstarted successor with a fresh ID/key/basis and reciprocal lineage. No branch, review, green checks, attestations, or evidence carries forward. Same-owner/terminal/scope-drift/started-unfinished-dependent cases refuse atomically; questions, pauses, ordering, dependencies, shelf attachments, and carriers stay operative.

Execute reads the returned replacement ref, starts it as recipient, and uses ordinary Git to preserve implementation without relabeling authorship. Close old task/shelf PRs without merging; preserve an unmerged aggregate PR and rerun affected checks/acceptance. Old commits/approvals do not vanish. Tiller changes no refs/host APIs, so failed external follow-through stays incomplete rather than rolled back. If already merged, sync and use forward work. Amend explains scope changes and unsafe relationships without inventing a transfer override or shelf amendment command.

## Publish the authorized work

An instruction to execute includes ordinary publication unless the developer explicitly says to stop earlier or not publish. The agent commits implementation on the recorded branch, runs checks and commits their state, pushes the branch, and creates or reuses the resolved PR with the project's host tools. It then records the actual PR number with `tiller submit`, commits that submission state, and pushes again. Both pushes belong to execute. Verify the recorded review and published state before handing off to the human. Tiller itself records only; it neither pushes nor creates PRs. The full sequence and host examples come from `tiller guide execute`.

Commit only state that actually changed. Re-submitting the same feature or epic PR can succeed unchanged after the riding work and gates were already pushed. In that case, skip the empty commit and redundant push and verify the existing published state instead.

No fresh authorization is needed merely to move from green checks to ordinary publication. Missing credentials, an unavailable host tool, a protected-branch refusal, or an unresolved conflict can still require human help. Never force-push, rewrite shared history, or alter a remote to bypass a refusal. If the PR exists but submission refuses, report both facts. If submission succeeds but its state push fails, retain the recorded state and identify the unpublished commit. Do not claim a completed handoff or create another PR to hide a partial failure.

A project with no remote keeps its local branch-and-commit review-ref path, without a host account or push; never use it to evade a remote-backed refusal. Requested changes resume the same task branch and established review, then recheck, resubmit, and reassemble at coarse grain. The old PR number is not proof that new changes were submitted. Approval decisions, attestation, `sync --accept-fast-forward`, and the final integration merge remain human decisions; explicitly delegated approval keystrokes and ordinary task-to-aggregate assembly may be performed by the agent.

## Recover checked work and read its status

`tiller next`, bare guide, orient, and both readout forms share one ranked list. Within your own work, owned checked tasks lead other owned ready or in-progress tasks, with dependency and soft order preserved inside each group. Tree tasks stay ahead of shelves. Shelves prefer checked work before their other candidates, with bugs before chores before tweaks within each group. Owner and parent checks, operative-question holds, order advisories, and live-carrier exclusions remain. A ranked move is guidance, not proof that submission will pass its guards. Orient still ends its reply on the list after its briefing; an already-authorized execute run does not stop for another publication go-ahead.

`tiller readout execute` uses the heading **Execution status**, reporting project-wide tasks and uncarried shelf nodes in `in_progress`, `check`, or `in_review`. Checked work leads the capped display, then in-progress, then in-review work. **Gates green with evidence** and **Current submissions recorded** use the same population, including withheld entities. Missing, failed, stale, or unevidenced required gates do not verify a node. Current submission means `in_review` with the appropriate recorded PR or review ref; an old PR on resumed or checked work does not count. Coarse refs identify the resolved review without inventing its PR. An empty report is not successful verification, and factual unpushed-state warnings remain. Next is the acting contributor's ranked list even though the status population is project-wide.

An aggregate review missing after its final merge needs explicit authorization to recover that named historical review, not another ordinary submission from the merged branch. A merge report alone does not authorize inventing PR/head/result identity. Verify the host repository, real PR number, merged state, configured base, head repository/branch/OID, and result externally. Tiller validates local Git only.

From configured integration with relevant source inputs committed, run `tiller submit feature|epic <node> --recover-merged --head <historical-oid> --merge-commit <result-oid> --reason <explanation> [--pr <actual-number>]`. A remote requires the actual positive PR number. Commit the actual recovery delta, run ordinary sync, inspect current acceptance and historical policy independently, commit actual sync state, publish when authorized, and read it back. Recovery changes no lifecycle/gates/approval. Contained work needs the human fast-forward assertion. Never reset refs, create an extra PR, or commit to the finally merged branch.


## Review ownership before adoption

Use `tiller migrate --dry-run --json`; save the report or data.ownership_review outside source state. The human fills each confirmed_owner for started unfinished task/bug/chore/tweak/tweak work, keeping all other inspected snapshot fields unchanged. Apply with `tiller migrate --ownership-review <file>` or `tiller upgrade --project-only --ownership-review <file>`. Stale/incomplete reviews refuse. Schema-5 active reviews/branches/evidence survive; only crossing schemas 2–4 into schema 5 requires shared-branch drain.

Pause mutations, reconcile owners/state, retain a compatible binary and committed recovery baseline, and migrate once in a designated checkout. Publish through Git, upgrade every participant, and merge migration into every continuing branch while preserving task progress. Refresh each checkout, restart consoles, reload agents, and inspect identity/schema/owner/branch/legal next action before resuming. Do not independently migrate stale clones or blindly restore old files after new work. The intended September 21 window does not authorize managed-project adoption or waive verification.

Schema 8→9 is a terminology migration: the committed roster becomes `contributors:` and local identity becomes `contributor:`, preserving IDs, selected identity, work claims/lifecycle, branches, reviews, evidence, and historical events. It needs no work drain or ownership review. Older migration requirements remain unchanged. Acting identity uses `--contributor` or `TILLER_CONTRIBUTOR`; task/bug/chore/tweak and question `owner` fields and ownership filters remain unchanged. New event actor attribution uses `contributor:`; historical events with `owner:` remain readable without rewriting their bytes.

Schema 9→10 adds typed immutable aggregate recovery and historical policy provenance. Its stamp-only migration preserves work, historical events, ownership, and active reviews without draining work or reconstructing missing history. Older binaries refuse the newer schema instead of dropping recovery fields.


## Maintain guidance

`tiller upgrade` updates the binary and the current checkout's supported schema, orientation,
discovery shims, harness wiring, and required ignore entries. `tiller upgrade --project-only`
does the checkout work offline with the running binary, including after a separate install or in
another checkout. `--binary-only` explicitly excludes the checkout. Already-current binaries still
refresh stale artifacts. Join remains onboarding or deliberate identity selection; maintenance
never silently joins. Current-schema artifact refresh needs no identity, while a real migration
resolves a registered actor.

Before migration from supported schemas 2 through 4, use a verified compatible binary to drain
active features, unfinished aggregate reviews, and tasks in `in_progress`, `check`, or `in_review`,
including human final merges and sync. Upgrade reports blockers rather than cancelling or
completing work. A retained executable is not proof of compatibility: verify it, then prepend its
reported directory to PATH in a temporary drain shell so gates use the same build. Leave that
shell and retry the upgraded executable explicitly afterward. Never use an older build to mutate
a migrated checkout. Migration preserves terminal history and refs; retained-prefix collisions
are execution warnings, not permission to delete history. For unstarted work use a non-conflicting
configured branch pattern or ask the human to archive/rename the collision deliberately.

Read phase results on failure: a binary update or migration may already have completed. Repair
the named cause before project-only retry, and recover any retained transaction journal first.
Do not remove caches or infer unchanged files from an unknown result. Review/commit the reported
diff. Other checkouts maintain themselves; existing serve and harness sessions need restart/reload
as appropriate. For the first installation from v0.12.2 or older, keep a compatible build beforehand
if needed and run the new binary's project-only command afterward: the old process cannot gain
new preflight/retention logic mid-run.

The refresh is idempotent and discovers every current catalog skill through thin shims. It removes
obsolete shims only with positive generated ownership evidence, preserving custom files.
Full bodies come from `tiller guide`; do not hand-edit installed shims or generated orientation.
Maintenance is separate from generating a read-only brief. Existing `drift.checks` lists stay
unchanged: add `stale_design` explicitly for tracked baseline drift, and `shifted_design` if the
broader check is wanted. The [distribution spec](/tiller/spec/distribution/#update) defines the
Homebrew ownership, retention, and partial-failure boundaries.

## How skills route correction feedback

An explicit human correction is already the go-ahead to take the matching route. The agent doesn't ask the human to translate review feedback into command vocabulary, and it doesn't push every accepted change through an amendment.

Structural requests begin with feasibility, before placement questions. The agent loads installed guides and command help, reads the affected nodes and every descendant's execution/review history, and compares parents, prerequisites, effective question holds, and inherited review settings. It separates the recommended outcome, installed capability, and smallest safe alternative. Current ready status does not prove that work never started; repository facts do not need another question to the developer.

| Feedback class | Skill route | Gate |
|---|---|---|
| Draft epic or feature proposal | Re-enter `decompose`; direct feature key rename requires a draft, task-free feature, and changing parents requires both epics to be draft. Other proposal corrections use eligible link/unlink/remove commands. | Return to the same epic approval. |
| Specced but unapproved feature prose plan | Re-enter `specify`; edit human-owned spec, contract, and proposed task prose. An unchanged key/parent correction uses the reviewed move row instead of cancel-and-redecompose. | Return to the same feature approval; moving does not approve that plan. |
| Accepted lightweight correction | Stay with `specify`'s correction map: direct title, epic prose, additive task, and soft-order changes; use `approve --refresh-basis` for same-scope non-terminal feature or task prose, or `return` when a ready plan's approval should replay. | None, basis refresh, task approval, or reapproval, according to the class. |
| Wrong factual claim in an approved `task.md`, found while executing | Stay in `execute`: build against the actual code, leave `task.md` unedited, and record the departure in the task's implementation commit message, its PR body when one exists, and run notes; coarse reviews gather riding tasks' departures from those commits. A departure that would change feature behavior, acceptance, task scope, a dependency, or a planned test is not one; use the rows above. | None; the reviewer checks the departure in the diff. |
| Genuine active structural replacement or cancellation | Enter `amend`, with preconditions and preserved terminal history. Use fresh forward work and supersession where appropriate. | One amendment approval. |
| Unchanged unstarted feature key or parent correction outside direct rename | Use `amend --move-feature` for eligible draft, specced, or ready features with never-started proposed/ready descendants, preserving IDs, claims, status, content, and acceptance history. | Stage, inspect, commit, and stop at human amendment approval. |
| In-place hard-edge edit on unchanged started work, or a move with started descendants | Unsupported. Name the blocking work and history; do not manufacture replacement, hand-edit state, or weaken a true prerequisite. Amendment has no link, unlink, or order operation; `revise` only re-pins prose. | No approval overrides this restriction. |

The direct, refresh, return, and amendment paths are different on purpose:

```sh
tiller rename task foundation/app/wire --title "Wire the application build"       # direct
tiller approve task foundation/app/wire --refresh-basis                            # refresh accepted prose
tiller return task foundation/app/wire --reason "revise the ready task plan"       # replay approval
tiller add amendment app-reshape --title "Reshape application setup"               # structural batch
```

Every route changes machine state only through `tiller` and edits only human-owned Markdown. The agent reads the result back, commits the correction, reruns the current seam's readout, and stops at the applicable gate. Done or cancelled feature and task prose is the terminal boundary. It never gets rewritten or refreshed to make old accepted work read like a later decision.

Feature interleaving across bounded epics is allowed when real prerequisites permit it. A projected epic ordering cycle is a nonblocking advisory with feature/edge evidence, not an execution deadlock. Actual cycles, including explicit epic and mixed feature/epic completion obligations, still block. Do not remove valid prerequisites to tidy grouped rendering.

## Request a code review

Ask the agent directly:

> code review feature F-rnte2m

Any epic, feature, task, bug, chore, or tweak works. The agent reviews the work that node delivered, merged and in flight, without changing anything, and writes one file under `.tiller/reviews/`. Its reply is a table of numbered findings: bugs graded blocker, important, medium, or minor; tweaks; chores; and plan changes that would add a task, feature, or epic, or revise unfinished work. Each bug says whether its failing path was traced or only suspected.

Then say what to keep:

> log bugs 1, 3, and 7, chore 2, tweaks 1-3, and plan change 1

Bugs, tweaks, and chores become `proposed` records attached to where they were found, citing the review. Plan changes become one `proposed` amendment. "fix B2 on its branch" sends a finding in unmerged work back to its task, "drop CH1" declines one, and "commit the review" keeps a review that needs nothing logged. Each request is one commit, and every approval stays with you.

```sh
tiller guide code-review
tiller guide code-review --json
```
