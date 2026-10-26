# Tiller Skill Set

**Scope.** This document defines the skills: the reasoning procedures an AI coding agent runs against the Tiller structure, their CLI choreography, and their stopping points. It is the counterpart to [cli-surface.md](./cli-surface.md). That document defines the operations that record or expose the agent's inputs; this one defines the reasoning, normally recorded through mutations, with the explicitly requested `design-brief` export and the `code-review` review arm as read-only exceptions. The CLI surface stays canonical for the commands themselves. Where this document and that one disagree about a command, the CLI surface wins and this one gets fixed.

**Status.** Current reference for the eleven skills embedded in Tiller v0.21.2 (schema 12, orientation generation 57). A checkout picks up these skills when `tiller join` or `tiller upgrade --project-only` refreshes its generated instructions. The [roadmap](../roadmap.md) says which surfaces are not built yet.

---

## What a skill is

A skill is the reasoning half of the agent surface, and the CLI is the recording and reading half. Each skill answers one question: how to think about a Tiller-shaped moment well, paired with exact `tiller` commands. Lifecycle reasoning normally lands through CLI mutations; `orient` briefs and stops, and the explicitly requested `design-brief` export returns a derived document without recording state. The two documents are a matched pair. One holds the structure; the other holds the reasoning that fills or explains it.

Three claims fix what belongs in a skill and what does not.

**Skills own the structure-aware reasoning; the CLI records state.** Decomposing inputs into a tree, writing acceptance criteria that bind to checks, breaking a feature into tasks, drafting a drift fix: these are reasoning, and they are the skill's. State changes land through a `tiller` command, never hand-edited machine state. The `design-brief` exception gathers recorded facts through existing reads and expresses them in a portable document; it does not manufacture a mutation to record its reasoning. The reasoning never lives in the CLI. Structure, gates, and state remain separate from the agent's synthesis.

**Skills are opinionated about process, silent on craft.** This is the line that sizes every skill in the set. Tiller's skills hold an opinion about the shape and gating of the work: how it decomposes, where acceptance lives, that every criterion binds to a check, that an agent drives to the edge of done and stops. They hold no opinion about the content of the implementation: how you write the test, the TDD rhythm, the architecture. That craft lives in the project's `AGENTS.md`, `CLAUDE.md`, and the supporting instruction files, which are not Tiller artifacts. Tiller neither generates nor owns them. When a skill needs the agent to actually build something, it hands off to that craft layer and to whatever general engineering skills the host harness already carries. The test, stated once and applied everywhere below: Tiller's skills are opinionated about the shape and gating of the work, and silent on the content of the implementation.

For `design-brief`, this boundary means gathering and expressing requirements, not prescribing design craft. Applicable project design constraints are source facts; any extra layout or interaction recommendations are labeled prototype suggestions. The receiving tool chooses the design within the recorded behavior and scope.

**Plan product progression, not permanent subsystem buckets.** Infer a defensible delivery sequence from documented actors, goals, workflows, and outcomes, including developer, operator, and API consumers. Cover the full known initial scope with bounded epic and feature increments; specify and create tasks just in time. Bound shared implementation by its first consuming increment while retaining known later needs in later features. A small foundation or concrete migration/compatibility risk can justify a technical prerequisite, not speculative reuse. Record actual execution prerequisites as `depends_on` and preferred sibling sequence with `tiller order`; eventual architectural integration is not automatically a prerequisite to useful work. Accept incremental extension and refactoring. Explain material tradeoffs in existing prose and run notes, not a new gate. Ask only for missing product information that would materially change boundaries or order.

**A skill never decides a gate.** Lifecycle skills drive work to the edge of done and stop for the human's decision. An explicitly delegated `approve` may be typed by the agent; `attest`, `sync --accept-fast-forward`, and the final merge into configured integration remain human-only. Ordinary task-to-aggregate merges are authorized execution, not gate crossings. `design-brief` ends at its document, with no gate or lifecycle transition.

---

## The anatomy of a skill

Skills use the following anatomy, with a narrow output exception for `design-brief`: its seven-part Markdown deliverable replaces the lifecycle readout, not the Reasoning, Choreography, or Hand-off boundaries.

**Trigger.** The frontmatter description normally names a lifecycle moment the CLI surfaced: a draft feature, drift candidates, or ready work. It makes the harness reach for the skill at the right seam and lets `orient` pick the next move by reading state. `design-brief` instead names an explicit developer request for a UI/UX brief or prototype handoff for an epic, feature, or task. Entering a project, encountering draft work, or finishing work never triggers that export.

**Reasoning.** The opinionated SDLC guidance for this seam, with what-good-looks-like made concrete. The decompose skill's "features land at the grain where one eval contract can express their acceptance." The specify skill's "every criterion binds to a check, never a hope." This is where Tiller's process opinion actually bites, and it is the bulk of the skill.

**Choreography.** The exact `tiller` operations the reasoning uses, in order, with their flags. Lifecycle reasoning normally ends recorded through the CLI, with the stopping point and next human gate named. `design-brief` uses only existing reads and read-only filesystem and Git inspection, then returns the brief. Only an explicitly requested deliverable file may be written.

**Readout.** At a lifecycle seam with a readout, the last command in the choreography is `tiller readout <seam>`. Reproduce its block verbatim rather than compose another account of the run. The agent's commentary belongs in a `Notes from this run` list ahead of it, for what the block cannot know: side effects outside `.tiller/`, a skipped step and why. Nothing follows the block, so its `**Next:**` ranked list closes the reply. The agent reproduces the list as written; when it would take a different row first, it says so in its notes ahead of the block, never by rewriting the list. A seam exit gets the full block; a correction or small action inside a gate gets `tiller readout <seam> --since <sha>`, where `<sha>` is the commit the last block reported. The correction block contains recorded events since that commit plus the same queue line and ranked Next list, still reproduced verbatim. The **Up next:** block `submit` and a passing `check` print mid-run is for the agent's own bearings; the closing readout's list is the one reproduced. `design-brief` is a read-only export, not a lifecycle seam: its document is the entire deliverable, with no terminal readout or invented `tiller readout design-brief`. See [the CLI surface](./cli-surface.md), "The agent readout."

**Hand-off.** Where the skill stops, named explicitly so it cannot drift into doing more. Every skill has up to two exits. The gate it must not cross hands up to the human. The craft it must not absorb hands out to `AGENTS.md`, `CLAUDE.md`, and the harness's own skills. A skill that names both exits is a skill that knows its own edges.

Five conventions are shared across the catalog rather than repeated in each. The first is the **recovery protocol.** Skills read the CLI's exit codes and the `tiller status` oracle to correct themselves without a human. A `1` is a business answer to reason about, usually a red gate or an illegal transition, and the lifecycle response is to fix the work and try again, not to escalate. A `2` or a `3` is a mistake in how the command was called or in the environment, and the response is to fix the call. For a work node, `tiller status <noun> <node>` names its current state and legal next moves with nothing executed. Design docs are path-addressed ledger entries, not work nodes; baseline readout and readiness expose their review state. `design-brief` inherits error interpretation, not authorization to repair sources or mutate the environment: retry only safe reads, retain reachable requirements, and report unavailable inputs or a material blocker. It never runs instruction refresh, gates, source repair, or another lifecycle skill to make an export succeed. These rules are also carried in `.tiller/orientation.md`.

The second is the **delegation model.** The gate is the human's decision, not the human's keyboard. A skill surfaces a start gate and stops for the decision; on an explicit human instruction naming the nodes, or an unambiguous set ("approve all nine epics" after the agent listed them), the agent may run the `approve` itself, stating exactly what it is about to record before running and reporting what it recorded after. A declarative past-tense claim ("they're all approved") is authorization to record only when the set is unambiguous: verify the recorded state first, then record, then report the delta. The terminal crossings, `attest`, `sync --accept-fast-forward`, and the merge to done, are never the agent's to run, even on instruction; the skill hands the human the exact command. This is guidance, not enforcement, per the non-coercion principle.

The third is the **node reference convention.** When a skill names a node to the human, it uses the key path with the id in parentheses: `form-engine/certification-form-loading (F-79dh3q)`, and a task its own three-segment path, `form-engine/certification-form-loading/schema-transcription (T-k3p9w6)`; a question is its id plus short title on scanning surfaces, with the full statement when the question itself is the subject. In command position bare ids stay valid, and questions are addressed by id only. The convention is stated once in `orient`; the other skills follow it. Design docs use their path relative to `.tiller/design/`, without an invented id.

The fourth is **developer authorship.** The developer owns and is responsible for the work. No agent identifies itself, a model, provider, or harness as an author or contributor in any output, including code, comments, docs, commit messages, PR titles and descriptions, issues, reviews, reports, and replies. Generated-by notices, agent signatures, badges, footers, and agent co-author trailers are prohibited. Use the developer's configured Git identity, never an agent author or committer identity or an invented human identity. Describe the work, its reasoning, and observed verification rather than the tool that produced it. Preserve necessary technical references, real human credits, and required third-party notices. Every skill inherits this rule from `.tiller/orientation.md`; `orient` introduces it and `execute` checks authored content and publication text, including template and host-tool defaults, before committing or posting. This is agent guidance, not CLI enforcement or a rewrite of existing history.

The fifth is **external references.** The canonical writing rule is stated once in the **External references** section of `tiller guide orient`; decompose, specify, capture, intake, and amend point to that section rather than copying it. The generated orientation points there as well. The rule governs provenance of authored links, not implementation craft, lifecycle approval, or a rewrite of accepted history.

Guidance must be complete in the skill text the binary serves. The main file carries the trigger and Reasoning, Choreography, and Hand-off sections, including any lifecycle readout instructions. `design-brief` carries its complete seven-part output contract in that single file. No reference-file loader or extra dependency is introduced for this export.

---

## Correction routing

Approval accepts a basis and preserves structural history; it does not turn every later correction into an amendment. Skills classify explicit human review feedback by what changes and by the affected node's state:

Before negotiating structural placement, load the relevant installed guide and supported command help. Use read-only show/status/query and history inspection to check affected nodes, every descendant's prior starts and execution/review history, source and destination parents, accepted bases, dependencies, effective direct/inherited question holds, and review grain. Never discover capability by trial mutation or an invented dry-run flag. Separate the recommended product outcome, what the installed binary can apply, and the smallest safe alternative before asking unresolved product questions. Repository facts are not questions for the developer. Amend and intake perform this preflight before placement; decompose, specify, and orient reuse the same rules.

| Correction class | Route |
|---|---|
| Draft epic or feature proposal | Re-enter `decompose`. Direct feature key rename requires a draft, task-free feature; changing parents additionally requires draft source and destination epics. Epic key rename requires a draft epic with only draft, task-free features. Apply eligible proposal corrections, commit, rerun the readout with `--since <sha>`, and return to the same epic gate. |
| Specced but unapproved feature prose plan or proposed task | Re-enter `specify`. Edit human-owned prose; use title, order, eligible link/unlink, proposed-task key rename/removal. Return to the same feature approval. A feature key or parent correction uses the reviewed move row, not cancellation and redecomposition of unchanged scope. `return` only replays ready-plan approval. |
| Display title at any state | Run `tiller rename <noun> <node> --title "<display title>"`; title metadata is cosmetic and does not alter accepted prose or identity. |
| Approved epic narrative | Edit `epic.md` directly and commit it. Git is the audit record because epic approval accepts decomposition shape, not an epic prose basis. |
| Approved non-terminal feature or task prose, same scope (a wrong factual claim found while executing a task is instead an execute departure) | Edit the pinned Markdown and commit it, observe `active_basis` with `tiller status`, then stop for the human's `tiller approve <noun> <node> --refresh-basis`. A refresh accepts current bytes without replaying lifecycle approval. |
| Ready feature or task plan whose approval should replay | Run `tiller return <noun> <node> --reason "<why the plan is being revised>"`, revise the plan, and return to the ordinary approval gate. `return` is the pre-work undo; reapproval is a new gate decision. |
| One more task under an approved feature | Run `tiller add task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>"`, write the proposed `task.md`, and stop for `tiller approve task <node>`. Additive child work has its own task gate and needs no amendment. |
| Preferred sequence | Run `tiller order <noun> <node> [<node> ...]`. Soft order is direct because it steers presentation and recommendations but never gates work. |
| Required sequence before affected work starts | Use `tiller link <noun> <node> --depends-on <target>` or `tiller unlink <noun> <node> --depends-on <target>`. A real hard-edge change is direct only before the affected work or branch starts; the one exception is that a started task may add (never remove) an edge whose target task is done. The refusal names the edge rule, never an amendment. |
| Unchanged unstarted feature relocation or local-key correction outside direct rename | Use `amend --move-feature <feature-ref> --to <epic>/<key> --reason "<why>"`, inspect the proposal, commit it, and stop at human amendment approval. Draft, specced, and ready features with proposed/ready tasks qualify only without prior execution/review history throughout the subtree. Claims survive; both parents must be draft or ready. A destination epic can be staged earlier. Original IDs, status, content, accepted provenance, and effective review grain survive; changed effective inherited prerequisites/question holds or affected open coarse reviews refuse. A specced plan or draft destination still needs its ordinary gate afterward. |
| Done feature whose eval bindings are stale or point at moved files | Edit only the bindings in `feature.eval.md`, then stage `amend --correct-eval <feature> --reason "<why>"` with `--retire <id>` for each deliberately removed criterion, and stop at human amendment approval, which re-pins `verified_against.eval`. A changed statement, polarity, required flag, or added criterion is forward work, not a correction. |
| Genuine replacement or cancellation after accepted work starts | Route to `amend`, with preconditions and one amendment approval. Record prerequisites on eligible fresh replacement work, never manufacture a replacement to evade `dependency_change_locked`. |
| In-place dependency edit on unchanged started work, or moving a feature with started descendants | Unsupported, except a started task adding an edge to a done task. Name the blocking work/history precisely; do not edit state, weaken prerequisites, or manufacture replacements. Amendment has no link, unlink, or order operation; `revise` only re-pins prose. |
| Done or cancelled feature or task historical prose | Preserve the accepted Markdown. Add forward work for the same bounded outcome where the parent remains executable, or propose genuine replacement through `amend` with supersession where appropriate. |

The distinction is operational. **Direct** corrections stay inside the command's proposal/state window. **Refresh** accepts a same-scope prose correction on non-terminal work without moving its state. **Return and reapprove** deliberately replay a ready plan's pre-work gate. **Amend** reviews eligible identity-preserving feature moves and genuine structural reshapes as a batch. Cancellation remains for genuinely abandoned or replaced unfinished scope; supersession preserves shipped history. Relocation alone never justifies replacement identities.

Explicit review feedback that fits a row is already the developer's go for that row. The agent does not ask the human to translate feedback into Tiller vocabulary. It re-enters `decompose` or `specify` for proposal and pre-work corrections, or `amend` for an active reshape, applies only the mapped direct command or human-owned prose edit, reads the result back, commits it, reruns the current readout with `--since <sha>` naming the commit the last block reported, and stops at the row's approval, refresh, reapproval, task-approval, or amendment gate.

A planning request authorizes its proposal, not approval, arbitrary reordering of an accepted tree, or a broader replan. Show proposed changes and their legal actions. A fresh proposed task under an active feature may still receive legal sibling prerequisites: the parent's status does not make that task started. New children may continue a done epic or feature, but not a cancelled or archived ancestor. Adding a child does not immediately rewrite the parent's stored status; ordinary sync reopens the rollup, clears current completion/review fields and stales feature gates as appropriate, preserving completed descendants.

Bounded epics may have interleaved features. A projected epic ordering cycle is a nonblocking advisory about grouping, not proof of bad product decomposition. Inspect its feature/edge evidence. Real prerequisite cycles, including explicit epic dependencies and mixed feature/epic completion deadlocks, still block readiness and execution. Do not remove valid prerequisites just to improve grouped rendering. Workflow administration versus oversight is planning advice, not a universal requirement for a permanent sink epic.

**Terminal prose boundary.** A done or cancelled feature or task is accepted history. Its `feature.md`, `feature.eval.md`, or `task.md` is not refreshed or rewritten to make the past read like the new decision. Epic narrative is different because it has no accepted prose basis. For terminal feature or task wording, create forward work and preserve the old accepted bytes, with an amendment and supersession when replacement history is part of the correction.

---

## The skills

One skill dispatches; eight carry lifecycle/operator seams; design-brief is an explicitly requested read-only export; code-review is an explicitly requested review whose disposition records only what the developer names. Refresh an existing checkout's instructions only on explicit maintenance direction, review the generated diff, and reload instructions.

| Skill | Activates when | Carries | Ends in |
|---|---|---|---|
| `orient` | you enter a Tiller repo or choose unrelated next work | how to read state, relay the ranked next list, and pick the top agent move | a briefing that ends the reply on the list; the developer's next message redirects or lets the top agent move start; reported merges/acceptance decisions re-enter related execute instead |
| `derive-baseline` | brownfield or legacy-migration has no confirmed baseline doc | undocumented code to as-is docs, or existing material to reconciled translations or pointers with provenance | committed docs and ledger, complete baseline readout, then human per-doc confirmation |
| `decompose` | to-be inputs exist but the tree does not reflect them | inputs to a draft epic and feature tree | `add epic` / `add feature`, left in `draft` |
| `specify` | the human pulls a `draft` feature forward | the specification, the acceptance contract, and the task explosion | `spec feature`, to `specced` |
| `execute` | authorized ready, in-progress, or checked work; requested changes or acceptance re-entry; coarse finish line or reported merge | the protocol and discipline of the bounded work loop | prepared acceptance review, published formal submission, or integration-branch closeout, stopping at the exact human decision or real blocker |
| `capture` | an investigation or developer description identifies a concrete bug, chore, or tweak | one complete shelf contract with provenance, attachment, and shelf dependencies where applicable | a committed `proposed` shelf node awaiting human triage |
| `mend-drift` | `tiller drift` flags a candidate | a drift candidate to a drafted fix | a diff for the human to approve |
| `amend` | a change reshapes the plan, a drift cascade is too big to mend, or the code shifted under planned work | the existing tree and code to corrections plus a staged reshape | edits cleared by `approve --refresh-basis`, and a `proposed` amendment awaiting the human's `approve amendment` |
| `intake` | a document written outside the tree critiques or redirects the work, or `tiller intake` already landed one with nothing staged | the feedback, mapped onto a staged explosion of epics, features, tasks, and questions, every operation sourced | a proposed, sourced amendment awaiting the human's `approve amendment` |
| `design-brief` | the developer explicitly requests a UI/UX design brief or prototype handoff for an epic, feature, or task | scoped requirements, source authority, journeys, states, constraints, and acceptance coverage | one self-contained Markdown snapshot, with no lifecycle mutation or terminal readout |
| `code-review` | the developer explicitly requests a code review of one epic, feature, task, bug, chore, or tweak, or names rows of an existing review | the node's net change reviewed read-only into graded findings, then named rows recorded through capture and intake shapes | one uncommitted review file and a summary table; on disposition, one commit of `proposed` shelf nodes and at most one `proposed` amendment |

### orient

**Trigger.** An agent begins in a Tiller repo or needs to choose unrelated next work. A reported merge or recorded acceptance decision for already-authorized work instead re-enters `execute` for bounded related closeout, never unrelated execution.

An explicit UI/UX brief or prototype-handoff request goes to `tiller guide design-brief` outside the ranked next list. It authorizes only the bounded export, not the list's next lifecycle move. Ordinary orientation still briefs with the ranked list.

Maintenance is an explicit developer operation, not permission to cross the ranked list's next lifecycle gate. Normal `tiller upgrade` updates the installed binary and the current checkout; `tiller upgrade --project-only` maintains that checkout offline with the running binary; `--binary-only` deliberately excludes it. Join is onboarding or deliberate identity selection, still the remedy for missing identity. Current-schema artifact refresh requires no identity; a real migration resolves a registered actor through ordinary precedence without rewriting identity. No maintenance operation automatically drains, cancels, approves, merges, or syncs work.

A real schema migration can need a human-reviewed ownership mapping or drained work before it applies. For pre-release adoption the orient skill carries the reviewed ownership round trip (`tiller migrate --dry-run --json`, a human-filled `confirmed_owner` per started unfinished unit, then `--ownership-review <file>`), the drain needed only when crossing from schemas 2 to 4 into schema 5, and coordinated Git adoption across every participant's checkout. The maintenance contract, including every refusal, is in [the CLI surface](./cli-surface.md#migrating-and-upgrading-migrate-upgrade); the team adoption procedure for older schemas is in [the distribution contract](./distribution.md#pre-release-ownership-adoption).

**Reasoning.** Read the lay of the land, relay the ranked next list, and end the reply. `tiller status` gives the project summary; `tiller queue` gives exact attention entries; shelf lists show standing work in soft order; node status gives lifecycle moves and separate workflow advice. Every readout and bare guide carries `queue: N bugs, M chores ready`, with actual counts and singular nouns at one. Preserve the baseline startup gate, checked-before-other owned task ordering, task-before-shelf ordering, and checked shelf priority with bugs before chores before tweaks in each phase. Preserve dependency, ownership, question, basis, and retired-node diagnostics. Inside a tier, shared task-grain feature closeout and coarse finish lines rank ahead of claims and unrelated planning. Recommendation order is not proof of full mutation-guard parity.

An unclaimed shelf node is never a ranked row. The human chooses queue work and the agent claims the top eligible node on that shelf rather than auto-claiming it. Live carried work waits on its carrier. Owned `carrier_cancelled` work returns to its own shelf and stays in Needs attention until the stale pointer is cleared or replaced. An uncarried shelf node with a current review is a human merge wait. `approve` and `resolve` may be delegated only on an explicit instruction naming the node and target; `cancel` is human-only, while `attest`, `sync --accept-fast-forward`, and merge are never delegated.

The dispatch is the ranked list `tiller next` prints, the same list the readout's **Next:** and bare `tiller guide` carry (see [the CLI surface](./cli-surface.md#ranked-next)). Orient relays it as written. If any human move ranks above the top agent move, it names that move first. Then it starts the top agent move unless the developer redirects, in its next turn, once the developer's reply does not redirect: the reply ends on the list, so the developer's next message is the redirect or the go. The agent may say it would take a different row first and why; it never rewrites the list. A ranked move is a suggestion, not authorization, and an explicit instruction naming nodes overrides the order for exactly those nodes, never a refusal. Approval neither claims nor starts work.

The list holds at most five legal moves, human decisions included, one row per node, each led by its actor (`You` or `Agent`). Tiller ranks by a fixed rule in five tiers: unblock nearby (human decisions holding the current feature, plus any question overdue or due within two days), continue (the reader's own unfinished work, whose top move is always kept), nearby (the current feature, then its epic), recently touched (features and epics the reader acted on in the last fourteen days, most recent day first), and everything else in plan order. The current feature comes from the reader's most recent deliberate act that names one; adding work only touches its parent for recency, and sync, migration, join, and other upkeep never count. With no identity the personal tiers are skipped and plan order ranks the rest. Bare guide suggests the skill for the highest-ranked agent move, since the top row is often the human's. The frontier offers only eligible drafts under approved epics, but explicitly named drafts remain legal to specify. One feature just in time remains the default, with the bounded explicit batch exception below.

The frontier includes only draft features whose own and owning epic's recorded dependencies are satisfied. Among eligible features, the owning epic's soft ordinal and then the feature's own ordinal choose first, with feature-flow position breaking ties. A draft waiting on unfinished work is no move and not a row; when every draft waits, the tree is still not empty, and dependency diagnostics (Home, `tiller check readiness`) and the caught-up hint explain the held work. Explicitly named drafts remain legal to specify even when the suggestion skips them. Execution responsibility is per task; historical ancestor contribution is not a reserved lane.

Owned execution and the unclaimed pool rank eligible candidates within their existing priority class by applicable recorded sibling preference. Epic order compares epics; feature order compares only siblings under one epic; task order compares only siblings under one feature. Unrecorded ties use the previous deterministic traversal. That preference is the plan order that breaks ties inside a tier, after the kind of move; where the reader has been working now comes from the tiers, not from task ownership, and the retired `Continuity note` has no successor because the reader's current feature already ranks its own remaining work as nearby. Preferred ineligible work never blocks a legal alternative, and completed prerequisite depth does not outrank preference. No preference preserves prior fallback; historical tree order and dependency edges do not change.

A brownfield or legacy-migration ledger with no confirmed doc limits the list to the baseline moves, even when the tree already has work: `derive-baseline` when no doc is registered, otherwise the human's confirmation of each pending doc. With only those human confirmations legal, bare guide suggests orient, and orient explains that review state instead of telling the agent to regenerate the docs. One confirmed doc releases this startup gate; remaining unconfirmed docs stay confirmation rows, not an all-doc gate. The readout presents baseline review explicitly even when Queue is empty, without inventing a queue entry or a console approval action. On a tree with no epics, owned shelf work, a proposed amendment, and a shelf merge wait are ordinary rows. `decompose` is a row only when a to-be design doc exists; with none, the list carries the no-work-planned hint while its only rows are open questions or design doc confirmations, and orient reports that no work is planned, names the open-question count and the ways work enters, and does not propose epics.

A question-held task is not a row; the holding question is, as the human's answer with its executable command, so the retired `Order note` has no successor. Claimable work is an eligible unclaimed ready task under an approved ready or active feature, not an entire feature. Caught up means no move is legal: `Caught up: nothing is legal to do right now.` Its hint, the unclaimed-work diagnostic, may count proposed, held, or otherwise ineligible unfinished unclaimed tasks and explain the first by dependency order, without a claim/start command; this is not the eligible ready-task pool.

Workflow advice uses the authored feature contract, current applicable required gates, and recorded results rather than one green boolean. Missing/unconfigured, failed, or stale machine evidence calls for checks/repair; required ungraded/stale rubrics call for grading/recording; only completed machine/rubric work permits a human-only acceptance wait. Invalid or unavailable contracts are uncertainty, not inferred acceptance. Optional criteria do not become blockers. Epic advice names the epic review and the specific feature contract; no epic eval exists. Task-grain features with all live tasks done need acceptance and ordinary sync on integration. Done-with-violations work remains done and visible without starving live work.

The pure ranked list, status, and readout cannot establish draft existence, host state, or successful publication. “Formal submission not recorded” must not become “no PR exists.” A riding task ref establishes neither feature acceptance nor formal coarse review. Authorized execute prepares and verifies the draft/offline surface before its acceptance handoff and supplies external observations in run notes; pure orient still briefs and ends the reply on the list.

**Choreography.** Reads, not mutations. Orient's output is the relayed ranked list rather than a state change. Human rows stay the developer's: the gate commands they name run only on the developer's explicit instruction, never off the list itself. The top agent move starts in the next turn unless the developer redirects. The separate `design-brief` export also reads without mutations but returns a document rather than this readout.

```
tiller next                    # the ranked next moves for this checkout
tiller status                  # the project summary: who holds what, what is where
tiller queue                   # current Queue entries; use --kind or --noun only to drill down
tiller ls bugs                 # the bug shelf in soft order
tiller ls chores               # the chore shelf in soft order
tiller ls tasks --owner <me>
tiller ls features --contributor <me>
tiller drift                   # are there candidates waiting
tiller status task T-k3p9w6    # one node's state and its legal next moves
tiller readout orient
```

**Hand-off.** Up to the developer, always. Every orient run ends the same way: surface the picture and the ranked list, naming any human move that ranks above the top agent move, then end the reply. The developer's next message is the crossing into the top agent move's skill, or a redirect, or a correction of the picture with added context. The stop is the briefing moment: the developer's opportunity to fix what the model has learned about the project and to add the context the design docs do not carry (commit and PR workflow preferences, where the front-end design system lives), with `AGENTS.md` the durable home for such conventions per the craft split above, so what is said once flows into every downstream seam rather than living in one conversation. Some rows are already a human gate, and those are instances of the same stop, not a separate list: everything sitting at `in_review` waiting on a merge, or work blocked on an open question the human owns, is a wait `orient` surfaces rather than spins on. The stop is scoped to orient alone: in-lane crossings (execute's start, check, submit loop; specify carrying one feature through specification, contract, and task explosion in a single pass) keep moving, since a pause at every seam would reintroduce the friction the skills exist to remove, and pausing at the re-entry point paces the whole arc.

### derive-baseline

**Trigger.** A `brownfield` or `legacy-migration` project has no confirmed ledger doc. On re-entry with pending docs, inspect and reconcile the existing docs or hand them back for review, not another duplicate derivation. Automatic uncovered-root routing is not implemented. Baseline drift is detected by `tiller drift` and repaired through `mend-drift`, not by reopening this startup gate. A developer may also ask for a standalone forward-work scan on an already-confirmed baseline; nothing in recorded state names it, and the agent never starts it unasked.

**Reasoning.** Read existing `.tiller/design/` material and the repository's README, agent instructions, and specs before writing. For brownfield read declared surface roots; for legacy-migration read declared reference roots, including explicitly named external trees. Derive undocumented code, or translate existing material at module grain. Each doc explains what the surface is, the data it owns, behavior to preserve, its edges, and its described paths. A file listing is too fine and a whole-repo monolith usually too coarse.

Translate is a refactor, not a rewrite. Preserve meaning and wording as structure allows; do not summarize away requirements or invent behavior. Ask once whether ownership moves into `.tiller/design/` or an existing consumer requires a pointer page. In the move arm retire the in-repo source as part of the translation; in the pointer arm keep one live source and describe which sections apply. Never silently leave duplicate live specs. Repoint repo instructions that name retired paths. External `--design` imports remain source-preserving copies; never move or delete external originals.

Use an available `project.adopted` commit as historical context for this project, not an external legacy baseline or a cleanliness claim. State absent or unavailable adoption history rather than inventing it. Reconcile current working-tree code before confirmation as well. When source and code disagree, say what current code does, record the disagreement in notes, and open a question with the existing `tiller add question "<statement>" --title "<short title>"` command.

Record forward work as questions. Sources often describe capabilities the code never got: an unbuilt spec, an unbuilt section of a built one, a roadmap. The as-is doc keeps its "not implemented" note; the decision to build belongs to the human. Open one question per unbuilt capability, not per file, citing every source that describes it with `--source`. A code TODO counts only when a document corroborates it, and an old implementation plan only where no spec covers the capability. Read the question ledger first to avoid reworded duplicates. A defect-shaped finding is a question too, never a captured bug: the seam reads, it does not reproduce. An answer of "build it" leads to intake on the cited source or a to-be design; "it's a bug" leads to capture; the seam names that move and stops. The standalone arm runs only this step on a confirmed baseline, writing no as-is prose and approving nothing.

**Choreography.** Establish the integration branch explicitly and preserve unrelated changes. If source material, including local copies of imports, is uncommitted, commit those exact inputs first so HEAD holds recoverable blobs. Draft docs, record every `--describes` relation and translated doc's `--from` provenance, and only then move or retire tracked sources. `--from` records this project's HEAD blob, not untracked bytes or an external Git blob. Commit only the reviewed move, split, docs, ledger, questions, and event delta; do not use broad `git add -A`.

```sh
tiller check readiness
# Read docs and code; confirm the integration branch and source-ownership choice.
# Commit exact uncommitted input paths before translating them.
# Draft .tiller/design/as-is/editor.md against current code.
tiller link design as-is/editor.md --describes app/editor
tiller link design as-is/editor.md --from docs/editor-spec.md
# Record any actual questions, then retire only the agreed in-repo source.
tiller add question "<what the sources describe and what the code has>" --title "<capability>: unbuilt" --source "docs/roadmap.md#sync"
# Stage exact reviewed paths and commit the translation and ledger.
tiller readout derive-baseline
```

**Hand-off.** Reproduce the readout verbatim and stop for human review against sources and code. The block exposes all recorded source path plus blob mappings, every ledger doc, described paths, and confirmation facts, without a three-doc cap. Those are linkage facts, not a semantic conservation test. In separate run notes name dropped, merged, and unplaceable material, source/code disagreements, and questions actually opened. `Open questions` in the block is the current project count; no ledger-derived dropped count exists.

The human edits these human-owned docs in place or rejects the translation, then runs `tiller approve design <doc>...` for named docs that match. The skill never decides approval; it may type the command only on explicit human instruction naming those docs. Pending baseline full and correction readouts end with one HUMAN review action rather than another derive step. One accepted doc clears the baseline-confirmation gap, not independent readiness gaps. Ordinary ranked moves then return; an undecomposed project moves to `decompose` only when a to-be design doc exists, and otherwise reaches the no-work-planned end state, whose open-question count includes forward-work questions.

**Craft exit.** Reading code is input analysis, not permission to change it. Deriving the baseline changes prose and recorded relationships, not application behavior. The baseline drift markers, touched-doc sync notices, and read-only console details keep that recorded layer inspectable as code changes.

### decompose

**Trigger.** To-be inputs exist under `design/`, files the baseline ledger neither registers nor cites as sources, but the epic and feature tree does not reflect them; or a new design doc landed that the tree does not yet reflect; or the developer explicitly names decompose. An empty tree with only as-is docs has nothing to decompose.

**Reasoning.** Read the design docs, the `current_state` layer, and brownfield `code` surfaces. `tiller check readiness` checks input presence and manifest coherence, not product richness. Infer supported actors, outcomes, and delivery progression through the full known initial scope before choosing containers. Propose a defensible sequence; record consequential missing information as questions rather than inventing journeys or defaulting to subsystems. Epics are bounded delivery increments, not permanent ownership buckets; related capabilities may recur in later epics. Features carry independently acceptable outcomes and contracts satisfiable within their own scope. Keep one unfinished feature where no honest acceptance boundary exists; a simulated experience requires explicit prototype intent. Tasks may be technical execution units, not individually end-user-useful products.

Ask at decomposition only when the missing product answer could materially change a boundary, prerequisite, or delivery decision, and name that consequence. When the tree is defensible without the answer, leave precise formats, interface conventions, tests, and implementation choices in the relevant feature's narrative for just-in-time specification instead of creating a blocking question merely because the detail is unwritten.

Interleave product increments with bounded platform work. Design for known later consumers without implementing all their needs first. Workflow-specific behavior belongs with its consuming increment; concrete migration, compatibility, or throwaway-work risks can justify a bounded prerequisite, explained in the existing review. Reconsider boundaries that create an unnecessarily broad barrier without deleting true prerequisites. Keep all known later requirements located in the tree and leave detailed specification and tasks just in time.

Feature interleaving across bounded epic increments is valid when actual prerequisites permit it. Readiness's projected epic-cycle advisory explains the grouping limitation; genuine task/feature, explicit epic, and mixed completion deadlocks remain blocking. Decompose must inspect the shared diagnostic, not infer that every epic projection loop is a bad product boundary.

Materialize the draft tree before recording edges whose targets must exist. Audit every actual prerequisite, including cross-epic edges, with `tiller link`; preferred sibling order uses `tiller order`, never a fabricated hard edge. A scoped lane records all resolvable edges and names unresolved targets; the lane creating the final nodes sweeps the whole tree. Later discovery is legitimate but does not excuse omitting known edges now. For brownfield, record `change_type` and `surface`. Epics and features carry no owner; new tasks remain unclaimed. Parallel source slices still use ordinary epic folders, not permanent subsystem lanes.

For brownfield and legacy-migration, consume the ledger-backed as-is docs and complete the baseline-confirmation gate before ordinary decomposition. Imported files alone are not confirmed input. If pending docs already exist, return them for reconciliation or human review rather than deriving duplicates. Confirmation does not settle open questions or prove the docs rich enough to decompose.

**Choreography.** Lay the tree or scoped slice down in `draft`, sweep the cross-epic edges in with `tiller link`, record the recommended build sequence with `tiller order`, commit it, and stop. The draft tree is the proposal; the committed diff is the review surface; the human's `approve epic` is the approval. There is no separate proposal artifact, so an uncommitted tree leaves the human nothing to review.

Every execution prerequisite asserted in epic prose must also exist as a recorded edge. Inspect the whole graph, since no `ls` read carries it. Zero cross-epic edges is valid when the requirements permit independence; counts do not prove planning quality. Brownfield surface paths are recorded through `tiller link feature <key> --surface <path>` alongside the dependency sweep, not left only in prose.

```
tiller check readiness
# After reading the inputs, propose the full known scope. This small example
# assumes a documented operator needs a reliable single import, then bulk use.
tiller add epic import-foundation --title "Safe import foundation"
tiller add feature import-foundation/validated-record --title "Validate one imported record"
tiller add epic first-import --title "Complete one import"
tiller add feature first-import/run --title "Run and inspect one import"
tiller add epic bulk-import --title "Operate bulk imports"
tiller add feature bulk-import/batch --title "Run a recoverable batch"
# Record genuine prerequisites after all targets exist.
tiller link feature first-import/run --depends-on import-foundation/validated-record
tiller link feature bulk-import/batch --depends-on first-import/run
# Audit every epic and feature's recorded edges, including scoped-lane omissions.
tiller order epic import-foundation first-import bulk-import
# Commit the reviewed tree and explain boundaries/tradeoffs in existing prose and notes.
# Stop for the human's epic approval; features remain draft until specified and approved.
```

**Hand-off.** Up to the human, who reviews the draft tree and approves it. `decompose` never approves. Approval is the first gate of the arc, not its end: once the human approves the epic tree, the next move is theirs, choosing from the frontier which feature to pull forward, and `specify` runs against only what they name, just in time, never the whole draft tree. There is no craft exit: decomposition reads code to understand it and never writes any.

### specify

**Trigger.** The human has pulled a `draft` feature forward from the frontier. This second decompose stage runs one feature at a time, just in time, unless the human explicitly names a bounded batch of draft features.

**Reasoning.** One pass produces the selected feature's specification, acceptance contract, and complete task breakdown. Extend `feature.md` with precise behavior and write `feature.eval.md` so every criterion binds to a `test`, `command`, `manual` sign-off, or `eval` rubric. Acceptance must be satisfiable by this feature, not deferred to a later consumer. Set `introduce` and `preserve` polarity deliberately; required preserve criteria carry pre-change baselines. Required eval rubrics still need human attestation of recorded input/output.

Tasks are concrete, reviewable implementation units with approaches, touched surfaces, and evidence, not a speculative shared subsystem hidden beneath a narrow feature. Shared machinery implements this increment's needs; known later requirements stay in their later features. Technical tasks and genuine sibling prerequisites are valid; use `depends_on` for those and `tiller order task` for preferences. Prefer observable feature progress early without demanding end-user value from every task.

Criteria state rules that hold, never captured values, golden outputs, or pixel and paint budgets; those live inside the tests a criterion binds to, and a look judged by eye is `manual`. Each task's `## Reuses` lists what the repository already offers (file:line) or names what was searched, and a helper two tasks need is planned once in the task that naturally owns it, with `depends_on` from the others, becoming its own prerequisite task only when no task owns it. `preserve` is for one existing behavior the feature puts at risk, bound to a targeted check, because the tests gate already runs the suite on every check; a targeted check exercises the at-risk behavior against its baseline, not a re-run of a spec the suite already holds. When `spec feature` warns that a preserve criterion runs that gate's own command, the agent gives it a targeted check, drops it, or says in the run notes why it stays; when it warns that a ref-only test binding is already run by the gate, the agent drops it or says why it stays, since rewriting the ref as an explicit `run` of the same spec is no fix. A kept criterion warns on every re-spec, and the run notes answer it each time.

If specification reveals misplaced requirements, broader platform needs, or future ordering changes, explain the affected scope and use the correction route. Record newly discovered hard edges only while the dependent remains eligible; a prose note alone cannot enforce them. Do not silently expand the named feature or batch, specify future features, alter accepted history, or convert preference into a prerequisite. Decomposition remains responsible for auditing edges already known there. Criteria and task specs describe the checks and implementation; executing them belongs to craft.

Ground each concrete claim the plan rests on about code it does not own (third-party, vendored, fetched, or generated code, or project code outside the feature's surfaces) in that code, citing file and line, or the documentation section, plus the outside code's version, tag, or commit. A fact that only building can settle is written as an open fact for the task to determine, naming the candidates and how to tell them apart, never as a provisional answer marked as assumed: an open fact stays true whatever execution finds, while a wrong provisional answer changes accepted prose. Read unfamiliar outside code during specify; a separate investigation task needs a reviewable output and a reason reading cannot settle the approach. Re-read cited claims against their sources before `spec feature`, through an independent reviewer where the harness has one, and list the plan's open facts in the run notes for the human's approval.

Before `spec feature`, a final pass reads the criteria against each other and against `feature.md`, each `task.md` against the code it names, and the task plans against each other. Each finding is fixed in the prose when reading settles it, written as an open fact when only later code can, or, when only the human can decide it (product behavior, acceptance, or scope), recorded with `tiller add question "<statement>" --title "<short title>" --blocks <node>` on the narrowest node its answer could change: the one task when only its plan depends on it, the feature only when a criterion does. A question asked only in chat is not allowed, since no gate sees it. The plan gate refuses while such a question is open, with no `--assume-question` escape. The human answers with `tiller answer question`, or in chat, where the agent records the answer on their instruction; specify writes each answer into the prose it changes, re-runs `spec feature`, and the run notes list each recorded question with what its answer would change and say which answer changed which file.

Before choosing a ref-only test binding, inspect the configured tests command and any wrapper's selector handling. Use an explicit targeted `binding.run` when the ordinary gate is an aggregate CI command; preserve that full CI gate separately. Account for environment and preparation so the criterion can run independently. For existing tests, verify the actual selected scope; for tests not yet built, put that verification in the implementing task rather than doing implementation during specify. The command-resolution contract belongs to [the eval model](./artifact-schema.md#the-eval-model), and the embedded skill supplies the runner-versus-wrapper example. Correcting an approved non-terminal binding still requires human accepted-basis review; new guidance does not rewrite existing contracts.

Specify applies the [managed-project independence contract](./distribution.md) before proposing infrastructure or acceptance bindings. Project commands remain runnable without Tiller; local Tiller may invoke them, never the reverse. Planning-only CI classification must not acquire replacement Tiller checks, optional jobs, or release credentials. A conflicting existing or approved plan must be surfaced with an independent alternative, with accepted-contract corrections returned to the applicable human review.

An explicit instruction naming several features, or every draft feature in one named epic, forms a bounded batch. The named set contains only draft features; the epic-wide form selects only draft features in that epic. Process them in Tiller's rendered soft order. For each feature, complete the specification, acceptance contract, and full task explosion, run `tiller spec feature`, and make one feature-only `specify:` commit, carrying the question ledger when the final pass recorded a question, before moving to the next named feature. Do not add unnamed siblings, do not run `approve`, and do not stop on an intermediate ranked wait. Finish the whole batch with one final `tiller readout specify`. This exception changes the skill's pacing, not the engine: `tiller spec feature` remains legal on any draft feature the developer names.

**Choreography.** The prose files are written directly, since the schema owns their shapes, which the skill shows in full so the agent authors well-formed files rather than reverse-engineering them. For a `replace` or `migrate` feature, record what it replaces before asserting, `tiller link feature <key> --surface <path>`, because the gate refuses a replace that cannot say what it replaces. `tiller spec feature` is the assertion that the whole stage-2 output exists and is well-formed: a contract with no criteria, a criterion missing its binding or a required field, a feature with no task, or a `replace`/`migrate` feature with no recorded surface path is refused as misuse naming the offender, never silently accepted.

```
# first named feature: extend feature.md, write feature.eval.md, and complete every task.md
tiller add task form-engine/certification-form-loading/schema-transcription \
  --title "Transcribe CERT-LTI-02-A (2025) into JSON Schema"
# final pass: record each decision only the human can make, pinned to the narrowest node
tiller add question "<statement>" --title "<short title>" \
  --blocks form-engine/certification-form-loading/schema-transcription
tiller spec feature form-engine/certification-form-loading
git add .tiller/epics/form-engine/features/certification-form-loading .tiller/decisions/open-questions.yml
git commit -m "specify: certification-form-loading"

# the ranked list suggests waiting; the explicit batch already selected the next named feature
# second named feature: complete its specification, contract, and full task set
tiller add task form-engine/pdf-assembly/continuous-page-numbering \
  --title "Render continuous page numbers across assembled forms"
tiller spec feature form-engine/pdf-assembly
git add .tiller/epics/form-engine/features/pdf-assembly
git commit -m "specify: pdf-assembly"

# never approve inside specify; stop for the human's decision on the complete plans
tiller readout specify
```

**Hand-off.** Up to the human, who owns the spec prose, edits it, answers its recorded questions, and approves the feature plan to `ready`, tasks included, once specify has folded those answers in. Out to craft for the tests the bindings name and the code the task specs describe: specify says what must be true and how the work is shaped, and execute with the project's instruction files does the building.

### execute

**Trigger.** Authorized owned task or shelf work is `ready`, `in_progress`, or `check`; review requests changes; the human records an acceptance decision; a coarse review reaches its finish line; or the developer reports a merge of that work. A reported merge is bounded continuation through related acceptance, sync, and state publication, not authorization to start unrelated execution. A pure orientation request still briefs and stops. Re-enter checked work without `start` or `resume`.

**Reasoning.** Execute supplies the protocol around building, not craft. Claim each task/bug/chore/tweak independently; another pre-start reservation requires its actual holder in --from and explicit human handoff. Before start only, release/reassign can change a reservation. First successful start permanently fixes ownership, including after resume, review, checks, cancellation, and completion; all terminal ownership is immutable. Creation and approval do not assign tasks. Judgment verbs retain existing actor and human-gate rules, not owner exclusivity. Work that moves, renames, or deletes recorded surface code links the current location and unlinks the old path with the work, since sync cannot fingerprint a recorded path the integration commit lacks; a sync skip that names a surface remedy is repaired the same way.

`tiller start task` atomically sets started, fixes its own owner, records its branch, activates a ready parent feature, and initializes applicable aggregate identity. No separate feature start exists. Parent dependencies, bases, operative questions, retired ancestors, and review-round guards still apply; failure changes none of these facts. Task branches use their full canonical key, feature aggregates reviews/features/<feature-key>, and epic aggregates reviews/epics/<epic-key>, through the configured pattern. Independent tasks start from integration; coarse dependents may start from assembled prerequisites. Requested changes return to the original task owner and branch.

Unless the developer's or project's own instructions say otherwise (they win), execute each unit of work in its own Git worktree, leaving the primary checkout as the developer left it. A unit of work is what one review covers, so review grain sets it: the task at task grain, the whole feature at feature grain, the whole epic at epic grain, and always the bug, chore, or tweak itself. Add the worktree detached at the updated start base and run `tiller join --as <you>` inside it, because identity is per checkout; restore any tracked file join's guidance refresh changed rather than committing it, and name the needed `tiller upgrade --project-only`. Then claim, start, and create the recorded branch there so start's state follows onto it. At coarse grain the unit's one worktree holds each riding task branch and the aggregate in turn: a later task detaches it at its start base, every switch follows a commit, and assembly, checks, the draft PR, submission, and requested changes run there. Tasks built in parallel within a unit, by subagents or otherwise, each get a temporary worktree of their own, removed once committed, pushed when remote-backed, and merged into the aggregate; two builders never share one. Re-entry uses, or adds and joins, the unit's worktree holding the recorded branch, and `resume` runs inside it. Another worktree holding the aggregate means stop and coordinate. Integration-branch closeout runs where that branch is checked out, usually the primary checkout, only when it holds no changes this run did not make. Assume other agents and developers share the repository in their own worktrees: never remove, reset, clean, stash, commit into, check out over, or force-check-out a worktree, branch, or change this run did not create. Another agent's claim lands on its task branch, not integration, so before claiming look for the work's canonical branch in worktrees, branches, and the remote; stop for the human when one exists or Git surfaces a conflicting claim or start. Remove only worktrees this run created, after their work is committed and, when remote-backed, pushed.

Read back task start's `branch` and `base_branch`. Task start can stamp a missing aggregate after a safe grain change; reopened coarse rounds rotate the aggregate to its canonical name plus `-<new-task-id>`, never reuse a merged ref. Preserve nonoverlapping `.tiller/` records and events during Git assembly. Never hand-edit machine state; stop for conflicting real decisions.

While executing a task, a factual claim in its approved `task.md` that the code proves wrong is a departure, not a plan correction: build against the actual code, leave the accepted prose unedited, and record what the plan said, what the code does, and where it was read in the task's implementation commit message, repeated in its PR body when one exists and in run notes. A feature or epic review PR, or an offline review surface, gathers its riding tasks' departures from those commits. A departure never changes the feature's behavior, an acceptance criterion or binding, the task's scope, a dependency, or a planned test or edge case; a correction that does needs the human's decision. A planned task not yet started takes the ordinary prose correction. Code review judges a task's diff against its recorded departures rather than the stale fact.

A question the agent judges moot, or pinned wider than the work it touches, is the human's to release: cancel and unlink are human moves, so the agent names the exact `tiller cancel question <Q-id> --reason "..."` or `tiller unlink question <Q-id> --blocks <node>` in its run notes and does not run it unless the human instructs it to. The agent may `tiller link question <Q-id> --blocks <node>` to widen a question that reaches further than recorded, and says so in the notes. The same rule holds in specify and orient. A finding that needs a human decision is recorded and built around, never asked only in chat: `tiller add question --blocks` names the narrowest node the answer could change (the task being built, a later task, or the feature only when a criterion is in doubt). The agent commits the held task's work and state, leaves the task where it stands, and continues with the next legal work, which routing in that checkout offers by skipping held tasks. The question lands only on the branch it is recorded on, so integration's queue, the console, and checkouts cut from integration cannot see it until that branch merges: the agent pushes the held branch when remote-backed, keeps its worktree, and itself holds a later task or the feature the question names for the rest of the run rather than trusting routing in another checkout. The human answers on that branch, in that worktree, as a coarse attest runs on the recorded aggregate, with `tiller answer question` or in chat recorded by the agent on their instruction, and the answer is committed there. The question ledger is one file several task branches edit; its mechanical conflicts keep every question. Recording a question never stops the run; it stops when no legal work is left or at a stop it already makes, such as a review handoff or an attestation, and the run notes then list every question it recorded and every open question still holding its work, each with id, title, held node, what the answer would change, and the branch and worktree that carry it. Semantic merge conflicts still stop on the spot. An answer that fits the approved plan resumes the held task, noted in its implementation commit message; one that changes approved prose takes the specify correction table, and the human clears the moved basis with `approve --refresh-basis`.

Configure the project's actual build/tests commands as part of implementation, not init. Check current required gates, not only old stored Required flags. Task submit relies on lifecycle state rather than explicitly revalidating the current required set like coarse submit; keeping task checks current is protocol, not a claimed submit guard.

At task grain the PR title is the full task reference, for example `form-engine/certification-form-loading/schema-transcription (T-k3p9w6)`, and its body names the parent feature. Coarse PRs name the reviewed feature or epic and all riding work. Every implementation/state commit carries the task id; coarse finish commits name the riding ids concerned. Shelf commits carry their shelf id. Follow developer authorship, preserve unrelated changes, and stage only this run's reviewed paths.

**Shelf loop.** A bug, chore, or tweak owns its claim, branch, gates, and review irrespective of review grain. Claim/start the shelf, not an attached feature. Read its own contract and attachment context: epic prose for an epic, feature prose/contract for a feature, or task prose plus parent feature prose/contract for a task. Attachment supplies context, not inherited acceptance. Ready shelf work claims/starts then creates its recorded branch; in-progress/checked work checks out the existing branch without start/resume; requested changes use `resume bug|chore|tweak` from in-review and reuse the same PR. A bug includes a regression reproducing the defect; chore tests honestly cover its maintenance. Shelf execute runs neither eval nor approve. Live carried work waits on its carrier; owned `carrier_cancelled` work returns to its own status/branch while the human clears or replaces the pointer.

Execute applies that same independence contract to setup, project commands, CI, and deployment. Local `tiller check` records the results of independently runnable commands; no project command calls back into Tiller or consumes `.tiller/` as an operational input. An approved plan is not permission to introduce that dependency or work around it with a token. Surface the conflict and propose an independent alternative while preserving applicable checks and their failures.

**Acceptance.** Feature acceptance uses `tiller check feature <node>`: it evaluates the contract and the other feature gates under check's basis/question/parent guards. Do not prescribe redundant default eval then check, and run the project's suite through `tiller check` rather than by hand and then again through check: a gate already passed on unchanged code is reused and needs no `--rerun`; use `--rerun` only for a suspected flake or environment change. `--gate <name>` iterates on one failing gate, a gate left out whose pass is not current is marked stale, and the node advances only on a full green set. Standalone eval does not carry those guards and must not be used to bypass a refusal. Inspect every result, including exit `1`, and commit each actual evidence/state delta before continuing; never put the evidence commit behind `&&` or a fail-fast shell. Pending manual acceptance must not skip the other checks. Repair missing, unconfigured, failed, or stale required machine work; invalid or unreadable contracts require repair, never inferred acceptance.

Rubric emission, grading, and recording are separate operations, not a default-eval loop:

```
tiller eval feature <feature> --emit-rubrics
# Grade the authored eval-kind packet against implementation; write its output_path.
tiller eval feature <feature> --criterion <criterion> --record-rubric-output <output-path>
# Inspect and commit actual recording state. A successful record is not acceptance.
```

Prepare required ungraded/stale rubrics before a human-only handoff. A required pass grade leaves `needs_attestation`; only the human's decision makes it accepted. Criterion attestation does not enforce every intended grading prerequisite, so the skill must not claim it does. Optional criteria do not become blockers. Preserve targeted-runner guidance: independently verify the approved targeted binding's selected scope, environment, and preparation; aggregate CI wrappers may ignore ref-only selectors. Preserve the full CI gate, and route corrections to accepted bindings through human basis review rather than quietly rewrite approved contracts.

**Coarse finish.** Check and submit each riding task without `--pr` on its own recorded branch, commit its review-ref/submission state, then ordinary-merge that branch's current tip into the recorded aggregate before continuing dependent work. No cherry-pick or squash assembly. Before feature checks, draft publication, and coarse submit, assemble every riding task's current tip and recorded review head; coarse submit verifies ancestry of both. Feature acceptance begins when every live task is `in_review` or `done`; waiting for riders to reach integration would deadlock review. Epic grain repeats this across riding features after independent overrides are done. Requested changes resume on their own task branches, check/resubmit there, commit state, reassemble, rerun affected feature gates, and update the same aggregate PR. Resolve mechanical conflicts; ask the human only for semantic decisions. An intermediate assembly merge never marks work done.

Retained legacy feature-prefix refs can obstruct task refs. Diagnose both names without deleting or renaming history or remote refs. Choose a non-conflicting configured branch pattern for unstarted work, or ask the human to archive/rename the collision deliberately; never rewrite recorded branches to bypass it.

Review grain changes only in its safe window. Finish and sync affected in-progress/check/in-review task work and feature reviews before changing grain; entering/leaving epic grain must precede the affected feature's branch. Existing task PRs must merge/sync before a later coarse submit. Coarse submit requires applicable required feature gates recorded pass and retains retired-parent, basis, question, reviewed-assumption, readiness, and review-identity guards. Never hand-edit state around them or submit epic-riding features separately. There is no epic contract or epic eval/check/attest.

**Shared ownership and closeout.** Contributors are distinct owners of started descendant tasks, including terminal/cancelled/transferred originals, not ready reservations, reviewers, creators, or attached shelves. Personal work means unfinished task/bug/chore/tweak assignments, including proposed/blocked/checked/in-review work. Shared assembly, checks, rubrics, acceptance, and submission appear once per applicable feature/epic; last-task completion surfaces that next step without assigning it to the finisher or every contributor. Any registered developer may perform authorized aggregate work. Coordinate one updater at a time through Git and the team, never a persistent coordinator claim; ranked suggestions are not authorization. Feature-only assumptions use `tiller approve feature <ref> --assume-question <Q-id> --reason <text>` on explicit human instruction, without lifecycle movement or silent inheritance by tasks.

**Explicit transfer.** Only a human instruction naming started unfinished work and recipient authorizes `tiller transfer task|bug|chore|tweak <ref> --to <registered-contributor> --reason <text>`. Before start use reassign. Transfer preserves approved scope and original identity/prose/owner/branch/review/evidence/proposer, cancels the original, and creates a same-noun ready unstarted successor with fresh identity/basis and reciprocal lineage. Same-owner, terminal, scope-drift, started-unfinished-dependent, and unsafe-relationship cases refuse atomically. Dependencies/order, questions, pauses, shelf attachments, and carriers remain truthful; no assumptions or reviewed answers are granted. No branch, review, green checks, attestations, or evidence carries forward. Never invent a shelf amendment or override command to bypass refusal.

Read back the replacement ref and recipient start command. Preserve implementation through ordinary Git without relabeling original authorship. Close old task/shelf PRs without merging and create replacements; keep an unmerged feature/epic aggregate PR, assemble the replacement, and rerun affected checks/rubrics/human acceptance. Old commits/host approvals are not removed automatically. Tiller changes no Git refs/host APIs; failed external continuation is incomplete, not a rolled-back transaction. An already-merged review requires sync and forward work.

**Remote-backed publication and human decisions.** Execute authorization includes ordinary commits, pushes, review creation/reuse, submission, and state readback for the named work, subject to explicit no-publish or earlier-stop limits. Tiller does not push or open PRs; the agent uses ordinary Git and existing host tools.

1. Resolve configured remote/integration and recorded task or aggregate review branch. On re-entry inspect the recorded PR and discover matching open drafts even without a formal submission. A branch already finally merged into integration must receive no further commits; intermediate assembly does not forbid task requested changes.
2. Commit implementation and riding task records. Check applicable feature gates and commit actual evidence even on exit `1`; resolve machine work and grade/record required rubrics. Task/shelf reviews use their own checks, not a feature contract inherited from context.
3. Once only human acceptance remains, push implementation/evidence and create or reuse one matching **draft** PR for the resolved feature or epic. Verify actual repository, base, head, open/draft state, and published tip through host tools. Match head repository/owner as well as branch names; ambiguous matches are a blocker, not a guess. Put the verified URL and publication evidence in **Notes from this run**. A retained non-draft with pending acceptance is reported truthfully, never treated as acceptance. No early formal submit or state-file PR edits.
4. Hand over each exact human decision, naming the feature, criterion, and evidence: `tiller attest feature <feature> --criterion <criterion> --status pass` if accepted or `--status fail` if rejected; use `--gate <gate>` for a required attestation-backed gate. At epic grain name both the reviewed epic and each feature needing acceptance. The human runs attest, never the agent even on instruction. The draft is review preparation, not a recorded submission.
5. On acceptance re-entry, read the human's records, commit actual attestation state, complete required rechecks, and commit their actual evidence. Requested changes resume affected in-review work, repair it, regrade invalidated rubrics, and reuse the established PR. Push the new evidence. With required acceptance passed, make the same PR ready if draft and verify identity/state/tip again. When acceptance was already complete, proceed here directly after checks and host verification.
6. Formally submit the resolved node with `tiller submit <noun> <node> --pr <actual-number>`, using the host's real number. Commit any actual submission-state delta and push it to the same review branch. An unchanged successful coarse submit needs no empty commit or redundant push; verify its already-published state.
7. Read back node status and verify the remote includes the submission-state commit. Matching `git rev-parse HEAD` and `git ls-remote <remote> refs/heads/<branch>` tips prove publication; if the remote advanced, fetch and verify ancestry rather than overwrite it. Render the final readout and stop for human review/merge.

Rechecks are conditional on current work and failures, not merely an attestation commit changing HEAD. `check` preserves settled criteria but resets a required attestation-backed gate to pending. Finish needed mechanical checks before that gate's human sign-off; do not erase it with a redundant blanket check. If new work requires another check, commit its evidence and obtain fresh human gate attestation afterward. The agent never restores a human gate itself or uses standalone eval to bypass guards.

On GitHub the existing `gh pr list --repo <repo> --state open --base <base> --head <branch>` discovers candidates, `gh pr create ... --draft` creates only when none matches, `gh pr view <number> --repo <repo> --json number,url,state,isDraft,baseRefName,headRefName,headRefOid,headRepository,headRepositoryOwner` verifies the surface, and `gh pr ready <number> --repo <repo>` makes that same draft ready after acceptance/rechecks. Task/shelf reviews with no acceptance wait can open ready. Other hosts use their own tools; `gh` is not a Tiller dependency. Exact runnable choreography lives in the embedded `tiller guide execute`, not installed copied skill bodies.

**Offline and incomplete work.** No configured remote means a committed local branch/tip review surface before acceptance, followed by a permitted branch-and-commit formal review ref after acceptance passes (`submit` without `--pr`), actual state commits, and status readback. Omit host operations/pushes, never invent a remote or require an account. A riding task ref in a remote-backed coarse review is not offline work and proves nothing about whether a PR exists. No-publish instructions, unavailable host access, protected-branch refusals, and genuine conflicts retain their exact boundary; never use offline mode as a workaround, force-push, change remotes, roll back successful state, or invent publication. Recover safely first; report the failed step, attempts, actual local/published commits, existing draft/PR, and outstanding work. A PR created before submit refused and a recorded submission whose state push failed are distinct incomplete outcomes, not duplicate-PR invitations.

**Reported merge and task-grain closeout.** A developer's report of the related merge re-enters execute, bounded to that work. Preserve the local review ref until ordinary sync observes it, safely update/check out the configured integration branch (where it is already checked out in another worktree, usually the primary checkout, only when that checkout holds no changes this run did not make), and run `tiller sync`. Inspect crossings, still-in-review nodes, policy violations, and touched-doc notices even on exit `0`; commit actual sync state and read back related tasks/feature. Never commit to the merged branch. Non-final task merges close out only that merge, not unrelated work.

After the last live task is done, finish feature acceptance on integration: `tiller check feature`, inspect/commit actual evidence including exit `1`, resolve machine work, and grade/record required rubrics. When only human acceptance remains, push the integration state or name its local committed tip offline, then hand over the exact human decisions. After they record acceptance, commit its delta, rerun required checks and commit evidence, run ordinary `tiller sync` for clean rollup, commit its actual delta, push integration, and verify remote tip plus feature/task state before `tiller readout execute`. No new PR for this state-only closeout and no coarse submit. Do not stop after clean checks with sync still outstanding.

Sync remains advisory and may record `done` with policy violations after a human final integration merge. Done stays done; acceptance warnings and history remain visible without suggesting resume/start or claiming done is blocked. A final merge commit or squash supplies observable evidence; a plain fast-forward needs the human's `tiller sync --accept-fast-forward <reviewed-node>`, task at task grain or feature/epic at coarse grain, never the agent's assertion.

**Merged aggregate review recovery.** An unrecorded review is not an observed merge: sync reports `review_missing` and has not inspected merge state. Use ordinary submission if unmerged. If already merged, collect evidence and obtain explicit authorization for the named feature/epic recovery; a merge report alone does not authorize inventing historical identity. Verify repository, actual PR number, merged state, base, head repository/branch/OID, and result externally with ordinary host tools. Tiller verifies local Git only and labels PR association caller-supplied.

On configured integration with relevant source inputs committed, run `tiller submit feature|epic <node> --recover-merged --head <historical-oid> --merge-commit <result-oid> --reason <explanation> [--pr <actual-number>]`. A remote requires a positive real PR number; offline recovery retains its permitted review ref. Commit the actual binding/audit delta, run ordinary sync, inspect crossings/current acceptance/historical warnings separately, commit actual sync state, publish integration when authorized, and read back related status/show/queue/readout. Skip empty commits on identical retries. Recovery changes no lifecycle/gates/acceptance. Contained/unwitnessed work still needs the human's fast-forward assertion. Never reset refs, manufacture acceptance, create an extra PR, or commit to the finally merged branch. Current green acceptance cannot erase a historical pending criterion.


**Readout.** Reproduce `tiller readout execute` verbatim and last; its block ends on the **Next:** ranked list, and nothing follows it. The **Up next:** block `submit` and a passing `check` print during the run is for the agent's bearings, not the list to reproduce. `Execution status`, gate-evidence counts, and current-submission counts retain the project-wide live task/uncarried-shelf population, including capped entities. Coarse riding refs remain task records, not feature submissions. The resolved feature/epic review's acceptance and formal record are independently visible even under a cap. Workflow advice is separate from lifecycle moves. Empty execution is not successful verification. In run notes supply verified URL, repository/base/head, open/draft state, tip evidence, exact human decisions, and factual local/unpublished/offline limits the pure readout cannot observe. “Formal submission not recorded” never means “no PR exists.”

**Hand-off.** Craft owns implementation. The human owns answers to recorded questions, acceptance/attestation, merge, and fast-forward assertion. Acceptance handoff requires prepared verified draft or offline review; formal-review handoff requires submission plus state publication/readback; post-merge closeout completes applicable check-to-sync-to-publication/readback on integration. Green gates alone, riding refs alone, or unpublished state do not finish those arcs. A genuine blocker or explicit stop limit is a truthful incomplete run.

### capture

**Trigger.** An investigation has reproduced a defect, or the developer has described concrete housekeeping or a minor adjustment that should remain visible beyond this conversation. Capture records the finding and does not triage it. A human answer to a forward-work question that names a defect is that description.

**Reasoning.** Choose `bug` when observed behavior violates expected product or system behavior. Choose `chore` for housekeeping without code side effects, such as documentation or administrative upkeep. Choose `tweak` for a minor adjustment that may change code, such as refining a button from "Save" to "Save changes"; if that label violates an agreed requirement, choose Bug instead. Chore classification is guidance, not an enforced restriction on changed files. Substantive planned work belongs in a Task under a feature; work needing a feature acceptance contract or a changed work tree routes to `decompose`, `specify`, or `amend`. Never reopen done work: capture a new forward-work node and retain the completed record as history. Write one contract per independently triageable outcome. A Bug records Reproduction, Expected, Actual, and Fixed when. A Chore or Tweak records What, Why, and Done when in `chore.md` or `tweak.md`. `--attach` records honest epic, feature, or task provenance and reading context, not parenthood or a gate. `--source` cites the narrowest durable artifact. Shelf dependencies name only Bugs, Chores, or Tweaks that must settle first. The result remains `proposed`; capture never approves, claims, starts, resolves, or cancels the node.

**Choreography.**

```
tiller add bug blank-on-reload --title "Form is blank after reload" \
  --attach form-engine/certification-form-loading \
  --source docs/investigations/2026-08-29-reload.md#L12-L44
# replace bug.md with the complete Reproduction, Expected, Actual, and Fixed when contract

tiller add tweak button-text --title "Clarify save button text" \
  --source docs/feedback/button-wording.md#L8-L16
# replace tweak.md with the complete What, Why, and Done when contract

tiller show bug bugs/blank-on-reload
git add .tiller && git commit -m "capture: blank-on-reload"
# stop with status proposed; never approve inside capture
```

**Hand-off.** Up to the human for triage. Hand them the full shelf reference, recorded source and attachment, and the exact `tiller approve bug|chore|tweak <node>` command without running it. On rejection or redirection, `cancel` remains human-only. `resolve` is delegable only on an explicit human instruction naming the shelf node and either `--clear` or the replacement carrier target.

### mend-drift

**Trigger.** `tiller drift` reports a `stale_spec`, `active_basis`, `answered_question`, `missing_surface`, `orphan_eval`, `stale_design`, or `shifted_design` candidate. A sync touched-doc notice only prompts the sweep; drift alone decides whether there is a candidate.

**Reasoning.** Detection is the CLI's and the fix is the skill's. Read the candidate's files, history anchors, and context before proposing the smallest honest doc, eval, binding, surface, accepted-basis, or reviewed-assumption correction. Show the evidence and reasoning for human review; never auto-apply. An `uncovered` entry is not a candidate or a clean result: report its exact failed comparison and recovery prerequisite instead of inventing a semantic fix. Unconfirmed docs need review, unavailable history needs recovery or an explicit account of its limits, and unsafe or unreadable paths need safe access repair before another sweep. A cascade is surfaced as one finding for the developer to judge as a plan reshape, not automatically staged.

For `stale_design`, read the as-is doc and each candidate feature's `feature.md`, then the bounded confirmation-to-latest-crossing diff on the affected described paths. `since` and `until` are project history anchors; the feature references explain tracked work rather than unrelated later HEAD churn. A missing described path is also a stale candidate and may have no feature or upper anchor: propose restoring it or correcting coverage through `link design` and `unlink design`, not a ledger hand edit.

For opt-in `shifted_design`, inspect working-tree code, staged and unstaged changes, relevant untracked files, and stamp-to-HEAD history only where available. This is deliberately noisy and need not name tracked work. Confirmation fingerprinted working-tree bytes that may never have been committed; neither its project anchor nor a fingerprint guarantees a recoverable old code blob. External trees may have no usable Git history, and the project anchor is not their HEAD. State those limitations rather than invent history. Follow pointer docs to the live source and propose corrections there without duplicating the specification; external originals remain untouched absent explicit authorization. `derived_from` blobs recover translated source documents, not necessarily old code.

**Choreography.** The fix is usually a diff to a doc, test, or eval, left for human review. Recorded corrections use the owning command: `tiller link --surface` for surface paths, with `tiller unlink --surface` dropping a moved or deleted path once its current location is linked, `tiller rename --title` for display metadata, and `tiller rename --key` only in its proposal window. Unchanged eligible feature relocation uses reviewed `move-feature`; moving started work remains unsupported. Genuine replacement routes to `amend`. A revised unapproved prose plan returns to its feature gate; no correction licenses hand-editing machine state. A `stale_spec` on the eval leg of a done feature, or an `orphan_eval` on one, is mended by editing only the bindings in `feature.eval.md` and staging `amend --correct-eval <feature> --reason "<why>"`, with `--retire <criterion-id>` for each check removed on purpose, then stopping for the human's `approve amendment`, which re-pins the eval leg. Spec-leg and surface-leg `stale_spec` have no such command: the skill drafts the edit and says plainly that the candidate persists until a byte-identical revert or a reopen and re-cross to done.

For either design marker, propose the smallest prose correction against current code and stop with the diff, evidence, and exact design-relative doc paths. If the prose already matches, show that review rather than manufacturing an edit. Only explicit human instruction naming docs authorizes `tiller approve design <doc>...`; permission to inspect, draft, commit, or mend is not approval. The command records working-tree confirmation for those docs only. Prose edits and repeated links do not reconfirm; changed describes invalidates the prior stamp and needs review. Reconfirmation can clear stale tracked-work candidates without another code commit. Neither sync nor drift writes the ledger.

```
tiller drift                              # read the candidate Tiller flagged
# draft the corrective diff to the doc, eval, or test, with the reasoning shown
# stop. the human reviews and approves the fix.
# baseline candidates hand over: tiller approve design 'as-is/workspaces.md'
# run it only on explicit human instruction naming that doc, then rerun drift
```

**Hand-off.** Up to the human, who approves the drafted fix. Out to craft when the drift is "the code moved and the test did not," because writing the corrected test is craft; mend-drift identifies the drift and drafts the reconciliation, and hands the deep implementation to execute and the project's instruction files.

### amend

**Trigger.** The developer judges a change large enough to reshape the plan rather than extend it; or `tiller drift` surfaces a cascade too large to mend marker by marker; or the code has shifted under planned-but-unbuilt work in ways no mechanical marker can see, the developer knowing roughly what moved and roughly which part of the tree it touches. This is the deliberate, developer-initiated counterpart to `decompose`: where decompose builds the tree from inputs, amend reshapes a tree that already exists and has work hanging off it. Nothing in recorded state says a reshape is due, so this is the one seam the ranked list never names on its own, and the picture the skill reasons from is one it goes and reads rather than one handed to it.

**Reasoning.** Frame the change as brownfield against the team's own recent code, and read the blast radius first, which features, contracts, and epics it actually touches. Recorded state makes that a query rather than a memory exercise, and each node's own commit anchor bounds the reading: diff that node's surface from its anchor to HEAD and judge what changed there, instead of reading the whole codebase against the whole tree. When a `tiller drift` run is what brought you here, its candidates are input too. Then sort what the radius holds into corrections and casualties, because most of it is usually the first kind. A planned node still right in intent and wrong in detail, a task naming a column that has since been split, is a correction: edit the prose, which trips `active_basis`, and the human clears it with `approve --refresh-basis`. The exception is a task already being executed, whose wrong fact is an execute departure recorded with the work. No amendment, no staging, no ceremony; the lightweight exit is the common exit, and the skill says which exit it is taking before it opens anything.

Choose placement by bounded outcome: extend an existing epic or feature when the work serves that outcome; create a new increment for a distinct outcome or delivery phase, even when it touches the same subsystem. Neither a shared subsystem name nor completion status decides placement. Adding children beneath a done parent is continuation, not rewriting shipped work. Ordinary sync reopens its rollup and review round as needed while completed descendants remain history; adding the child alone does not update stored parent status. Cancelled or archived ancestors are not executable continuation targets.

Choose amendment operations by the actual change and target state. Supersede done work only for genuine replacement; it remains historical, and replacement or migration features record their surfaces and preservation obligations. Cancel unfinished work made pointless, with a reason and replacement lineage where appropriate. `revise` accepts edited non-terminal prose by re-pinning its basis; it never changes dependencies, unlocks started work, or re-pins done/cancelled prose. `correct-eval` corrects only how a done feature's contract is checked (bindings and baseline artifacts, never a criterion's statement, polarity, required flag, or baseline surface), names each deliberately removed criterion in `--retire`, and on approval re-pins `verified_against.eval`; changed acceptance is forward work. `revise`, `correct-eval`, `supersede`, and `cancel` do not target epics. Stage additions and unresolved questions as needed, with a staged epic before features naming it. Tasks require an already materialized feature; after a new feature lands, ordinary just-in-time specification creates its tasks, not a compulsory second amendment.

There are no amendment link, unlink, or order operations. Eligible unchanged feature relocation uses reviewed `move-feature` under the correction matrix, not cancellation and recreation. An in-place dependency edit on unchanged started work remains unsupported, even if a CLI refusal hints at amendment. A genuine replacement may record prerequisites on eligible fresh work; never create replacement merely to evade a lock. Approval appends new epic IDs to root soft order. A cross-parent feature move removes the source entry and appends at the destination in staged order; same-parent key changes preserve position. A different intended sequence requires explicit `tiller order` afterward. Hard links remain legal only while the dependent is eligible; narrative does not apply them automatically.

Reshape only the requested affected unfinished scope. Explain wider effects and seek developer direction for a broader replan, leaving unrelated accepted work intact. A proposal is not authorization to approve it or arbitrarily reorder an accepted tree.

Three disciplines carry the rest. **Preconditions**, one on every destructive operation, pinning the state or the content the operation was reasoned against, because a batch that applies as reasoned or not at all is what makes it reviewable rather than merely written down. **Preservation**, the safety net of any rework: the replacement's contract binds `preserve` criteria to characterization tests captured before the behavior moves, and amend's narrower job is to name, node by node, the behavior the replacement has to keep. **The durable intent**, because a change this size obliges edits to the design docs and nothing else tracks that they happened: the amendment records the docs that prompted it and the docs its approval obliges, and its own narrative carries the rationale, which is the prose-and-state split doing the job an ADR would. What good looks like: done work superseded and not edited, the destructive operations few and each carrying its reason and its precondition, additions that bring the preservation the rework needs, and the whole amendment readable as one coherent story in a single pass.

**Choreography.** Run the read-only structural feasibility preflight before placement and classify every item through the correction matrix before opening an amendment. Lightweight corrections take their mapped direct, refresh, return and reapprove, additive-task, or soft-order route. Eligible unchanged features use reviewed moves, preserving identity and stopping at amendment approval. A genuine casualty uses cancellation/supersession; unsupported started moves or active-edge edits do not become supported by staging prose.

```bash
# lightweight examples, no amendment
tiller rename feature form-engine/certification-form-loading --title "Certification form loading"
tiller approve feature form-engine/certification-form-loading --refresh-basis
tiller add task form-engine/certification-form-loading/csp-regression \
  --title "Cover the CSP regression"
tiller approve task form-engine/certification-form-loading/csp-regression
```

For a structural reshape, open the amendment, write its narrative, stage the operations, show the batch, and stop.

```
tiller drift                                               # when a cascade brought you here, read it first
tiller show feature form-engine/certification-form-loading --json   # the surface and the basis that bound the reading
tiller add amendment form-engine-rework \
  --title "Restructure form engine from embedded JSON to discrete tables" \
  --triggered-by design/solution-design/04-form-engine.md
# write the reshape's narrative over the stub at .tiller/amendments/<A-id>-form-engine-rework.md
# stage the additions first: a --by, --superseded-by, or --blocks naming a staged key
# resolves only against operations already on this amendment
# new scope this review also names: an epic. Stage it first and a later
# add-feature in this same amendment may name it as parent (epic before
# feature is the order that makes it legal); add-task's feature must still
# be live in the tree, never a same-batch staged one
tiller amend form-engine-rework --add-epic print-layouts --title "Printable form layouts"
tiller amend form-engine-rework --add-feature print-layouts/pdf-export \
  --change-type create --title "PDF export" \
  --precondition key_absent=print-layouts/pdf-export
tiller amend form-engine-rework --add-feature form-engine/table-backed-forms \
  --change-type replace --title "Table backed forms" \
  --precondition key_absent=form-engine/table-backed-forms
tiller amend form-engine-rework --add-task form-engine/rendering-modes/table-aware-rendering \
  --title "Render from the table model" \
  --precondition key_absent=form-engine/rendering-modes/table-aware-rendering
# a judgment call the review raises but does not settle: a question, not a decision
# invented here. --blocks may name a key staged earlier in this same call
tiller amend form-engine-rework --add-question "Keep the legacy embedded-JSON fallback for offline mode, or drop it once the table model ships?" \
  --title "Legacy offline fallback" \
  --blocks print-layouts/pdf-export
tiller amend form-engine-rework --supersede form-engine/certification-form-loading \
  --by form-engine/table-backed-forms \
  --precondition target_status=done --precondition target_revision=git:7bc1a20
tiller amend form-engine-rework --cancel form-engine/rendering-modes/embedded-json-fallback \
  --reason "subsumed by the table model" \
  --superseded-by form-engine/rendering-modes/table-aware-rendering \
  --precondition target_status=ready
# a revise of non-terminal work: edit feature.md and feature.eval.md first, then pin what you edited
tiller amend form-engine-rework --revise form-engine/rendering-modes \
  --precondition spec_revision=git:3e9293c
tiller amend form-engine-rework --amends-design design/solution-design/01-overview.md
tiller show amendment form-engine-rework                   # the batch, each precondition's live validity
git add .tiller && git commit -m "amend: stage the form engine reshape"
tiller readout amend
# stop. the human reads the narrative and the batch beside the git diff; on their explicit
# instruction naming the amendment you may run the gate yourself, otherwise hand them
# `tiller approve amendment form-engine-rework`, or the escape,
# `tiller withdraw amendment form-engine-rework --reason "<why it is dropped>"`.
```

**Readout.** `tiller readout amend` closes the reply, naming what is proposed, how many operations each batch stages, and how those preconditions read against the live tree right now. The notes ahead of it carry more weight at this seam than at any other, because what the block cannot know is most of the judgment: the radius that was read and what was deliberately left standing, which nodes were sorted as corrections rather than casualties and the refresh each of those needs, the reason behind each destructive operation, the behavior a replacement has to preserve, and any precondition already reading stale that the human has to settle before the gate can land.

**Hand-off.** Up to the human, whose `approve amendment` is the one act that executes the supersessions, cancels, revisions, and materializations the skill only ever staged, and it applies them as one batch or not at all. `withdraw amendment` is the escape, on the same explicit-instruction terms. Out to craft for the rework itself, which once the amendment is applied runs through `execute` like any other work, and back through `specify` for a materialized feature when the human pulls it forward.

### intake

**Trigger.** The developer hands the agent a document written outside the tree that critiques or redirects the work, a code review, a product critique, user research, any prose making a case for change rather than describing work already agreed. Or `tiller intake <file>` has already run: the document is landed under `.tiller/feedback/`, a proposed amendment carries it as `source`, and nothing is staged yet. That includes a source a forward-work question cites, once the human answers that they want it built, even on a tree with no epics yet.

**Reasoning.** Land the document if needed and read the entire artifact before mapping it. A changed document is a new intake, not an edit to the fingerprinted source. Before placement, run the correction matrix's read-only feasibility preflight against installed help and affected history, settings, and holds. Explain outcome, installed capability, and safe alternative separately. Then map requested outcomes onto bounded increments without disguising legitimate technical, security, or platform work as invented customer-facing scope. A statable acceptance contract becomes a feature under the epic whose bounded outcome it serves, or a new increment for a distinct outcome or later delivery phase. A same-outcome correction can become a task under an existing feature; a done parent alone is not a reason to create a new container. Preserve completed history and use ordinary sync and approval for continuation, never cancelled or archived ancestors.

An unresolved judgment becomes a question, not an invented decision. Split findings across justified increments when needed without losing coverage. Every staged operation cites honest source lines with `--source`, conventionally `path#Lstart-Lend`; the CLI checks presence, while the agent and human review source fidelity. A staged epic may parent features staged later in the same amendment. A task's feature must already exist; new features receive tasks later through ordinary specification. Question `--blocks` may name earlier staged keys.

Confine reshaping to the requested affected scope; broader replanning needs developer direction. Separate preferred order from actual prerequisites in the narrative, name affected existing work, and identify legal post-materialization `tiller order` or `tiller link` actions. Amendment cannot link, unlink, or order. Eligible unchanged feature relocation uses sourced `move-feature` operations and the same refusal/approval boundary as amend; genuine scope replacement still uses cancellation/supersession. Never promise a new hard edge on unchanged started work, a move with started descendants, automatic narrative ordering, or approval without the human decision.

An `add-epic` operation has one automatic ordering consequence when approval materializes it: the fresh epic id receives a default final root ordinal, appended after the existing root soft order in staged order and in the same approval transaction. This does not turn the reviewer's narrative into intended placement, fill an existing partial list, or order features or tasks. When the feedback intends a different explicit sequence, intake still records that claim in the amendment narrative and points to `tiller order epic ...` after approval; that verb remains the replace or clear surface.

**Choreography.** Land the document if it is not landed, read it whole, map each finding to its rule, cite every op, write the narrative, then stop for the human.

```
tiller intake docs/feedback/opus-review.md --contributor robin
# lands .tiller/feedback/opus-review.md and opens amendment opus-review with source
# set and nothing staged; read the landed document whole before mapping anything below

# new scope the review names: an epic, staged FIRST, so the features it
# parents can name it as their parent in this very same amendment
tiller amend opus-review --add-epic hardening --title "Hardening" \
  --source .tiller/feedback/opus-review.md#L1-9
# a statable contract under the epic just staged above: epic before feature,
# in this one amendment, is the order that makes this legal
tiller amend opus-review --add-feature hardening/csp-headers \
  --change-type create --title "CSP headers" \
  --source .tiller/feedback/opus-review.md#L10-15
# a statable contract under the EXISTING form-engine epic: a feature,
# resolved the ordinary way, against the live tree
tiller amend opus-review --add-feature form-engine/csrf-token-rotation \
  --change-type create --title "CSRF token rotation" \
  --source .tiller/feedback/opus-review.md#L16-21
# a correction inside an existing feature's scope: a task, on the PRE-EXISTING
# feature form-engine/certification-form-loading, never on either feature staged
# above, since a new feature's task explosion is specify's own act
tiller amend opus-review --add-task form-engine/certification-form-loading/csp-regression-test \
  --title "Add a CSP regression test" \
  --source .tiller/feedback/opus-review.md#L34-36
# a judgment call the review explicitly declines: a question, not a decision
# invented here. --blocks may name a key staged earlier in this same call,
# even the same-batch epic's own feature above
tiller amend opus-review --add-question "Keep or trim the legacy image parsers?" \
  --title "Legacy image parsers" \
  --blocks hardening/csp-headers \
  --source .tiller/feedback/opus-review.md#L48-52
# an ordering claim is never an op: name it in the narrative for the human to apply
# once the batch lands (tiller link feature ... --depends-on ..., tiller order epic ...)
# write the distilled narrative over the stub at .tiller/amendments/<A-id>-opus-review.md

tiller show amendment opus-review          # the whole batch, every op sourced
git add .tiller && git commit -m "intake: stage the opus review explosion"
# stop. the human reads the narrative and the batch beside the git diff; on their explicit
# instruction naming the amendment you may run the gate yourself, otherwise hand them
# `tiller approve amendment opus-review`, or the escape,
# `tiller withdraw amendment opus-review --reason "<why it is dropped>"`.
```

**Hand-off.** Up to the human. `tiller approve amendment` is the one act that materializes the epics, features, tasks, and questions this skill only ever staged, atomically: every precondition and the amendment's own source both validate first, and then all of it lands or none of it does. A materialized task lands unclaimed, same as one added by hand; feature/epic nodes have no owner and amendment approval does not assign tasks. `withdraw amendment` is the escape, on the same explicit-instruction terms. Out to `specify` for a materialized feature once the human pulls it forward, to `execute` for a materialized task once it clears its own `approve task`, and to `tiller answer question` for a materialized question, the same as any question regardless of how it landed; a materialized epic clears `approve epic` before either starts. There is no `tiller readout intake`, the same deliberate omission `mend-drift` carries: intake's edge of done is a staged batch for the human to approve, not a rendered seam block.

Approval's root-order append is part of the same all-or-nothing hand-off: every epic this approval freshly materializes lands with its default final root ordinal, while every pre-existing entry and omission stays untouched. If the human wants one of those epics placed anywhere else, the next explicit act is `tiller order epic ...`, not a different amendment operation.

### design-brief

**Trigger.** The developer explicitly requests a UI/UX design brief or prototype handoff for an epic, feature, or task, for example "Please give me a design brief for feature F-9oymbc," "Generate a design brief for epic E-t73jkv," or "Generate a design brief for this task." Keys and IDs are accepted; a conversational target must be unambiguous. Bugs, chores, and tweaks are outside this export's target scope. The request does not claim or start work, specify a draft, approve a plan, or make a brief a prerequisite for execution.

**Reasoning.** Produce a derived snapshot that a receiving design tool can use without repository access. Carry the substantive requirements into the document, with paths and section or criterion references for developer traceability. Ancestry supplies intent and constraints, not permission to expand scope.

| Target | Inspection and prototype scope |
|---|---|
| Epic | Account for every included feature, its maturity and retirement, relevant tasks, navigation, and cross-feature journeys. Never silently sample a subset. |
| Feature | Cover the complete feature experience, parent intent, acceptance contract, and relevant task detail, not unrelated siblings. |
| Task | Map the visible change to relevant parent criteria and include only enough surrounding experience to demonstrate it. |

Identify cancelled or superseded work instead of reviving it as a current requirement. An explicit historical request names the retired scope and replacement context. A data dependency brings only relevant entry points, displayed information, navigation, or shared behavior into the brief, not all of its screens.

Read target prose and state, ancestors, relevant descendants, acceptance contracts including preservation requirements, applicable design documents, recorded decisions, questions, and assumptions. Follow prose references and recorded surface paths into accessible implementation, components, design-system guidance, and assets. Frontmatter and search are discovery aids, not a mechanically complete catalog. Bound inspection to the requested experience; do not install dependencies, run potentially mutating scripts, or launch application operations to gather context.

Use current working-tree requirements by default. Read feature and task `accepted_against` and node-status drift where applicable, then compare current content with recorded references through read-only Git inspection when available. Approval can fingerprint dirty prose; the recorded HEAD commit is not necessarily the approved content. Epic approval accepts decomposition shape, not an approved epic prose revision. Lifecycle status alone does not establish source authority, and absence of reported drift is not proof that every source was compared. Missing objects, shallow history, unavailable references, and incomplete comparisons remain explicit limitations. Never check out or fetch historical content, write Git objects, refresh an approval basis, or repair source gaps as part of export.

Disable Git's optional index writes from the first export read. Prefix every Tiller invocation with `env GIT_OPTIONAL_LOCKS=0`, since Tiller reads can invoke Git internally, and use `git --no-optional-locks` for direct inspection. Disable external diff and text-conversion helpers. A content-read operation must not refresh the index as a side effect.

Label **Required**, **Suggested for the prototype**, and **Unresolved** at the affected behavior. Required behavior retains its source's draft or approved status; suggestions are reversible prototype choices, not new product requirements. Separate observed existing behavior, intended changes, and preservation requirements. Implementation is evidence of what exists, not automatic authority over intended behavior; a design document is not proof a screen exists. Surface material source conflicts without an undocumented precedence rule or substituting historical prose for the current checkout. Draft and narrative-only work remain eligible, with missing detail visible.

**Choreography.** Load `tiller guide design-brief` (or `tiller guide design-brief --json` for the same embedded guidance), resolve and enumerate scope with existing `show`, `status`, `tree`, and `ls` reads as appropriate, inspect the bounded sources, and synthesize one Markdown brief in these seven parts:

1. **Purpose, source basis, and scope.** Project, target title, key and ID, ancestry, user outcome, boundaries, revision context and discoverable working-tree changes. State that this is a derived handoff snapshot, not an approved replacement specification. A commit must not imply dirty source bytes came from it; references identify relevant sections and criteria.
2. **Users and operating context.** Actors, permissions, entry points, prerequisites, goals, and established product terminology.
3. **Experience overview and journeys.** Necessary surfaces, navigation, and end-to-end scenarios, distinguishing in-scope work from surrounding context.
4. **Detailed screen and interaction requirements.** Surface purpose, information hierarchy, displayed data, controls, actions, validation, feedback, navigation, and conditional behavior. Include loading, empty, error, success, permission, destructive-action, and recovery states where sources require them. Important unspecified states are gaps or labeled suggestions, not invented requirements.
5. **Visual and interaction constraints.** The substance of applicable design-system, component, asset, responsive, and accessibility constraints. State which assets must be supplied separately; a path does not give the receiver the asset. Keep behavior and scope strict, with freedom in visual composition unless sources constrain it.
6. **Prototype instructions and sample data.** Clickable interactions, simulated transitions, and how a reviewer reaches each significant scenario. Use coherent synthetic data exercising required distinctions, never customer records, credentials, or secrets. The non-production prototype may use local interaction and simulated state, but needs no production backend, real authentication, live integration, or production writes. Simulated success is not a completed domain operation.
7. **Coverage and open decisions.** Map every UI-relevant acceptance criterion to a screen or reachable scenario. Account for non-visual criteria as outside prototype proof, not dropped or satisfied. List unresolved questions, source conflicts, unsupported assumptions, and prototype suggestions; distinguish designer choices from product decisions needing human resolution.

Review coverage and export safety before returning the brief. For a non-UI task, adapt the structure to an evidence-supported no-UI finding and indirect user-visible constraints rather than invent screens. Insufficient evidence about UI impact is not proof that no UI change is required. Name missing references and inaccessible assets; provisional options must be labeled. Ask for clarification only when a material ambiguity prevents a useful bounded brief, retaining all reachable requirements.

**Hand-off.** Return the single self-contained Markdown document in the reply and stop. Only an explicit file request permits writing the requested deliverable; it never permits canonical prose or state replacement. Preserve an existing human-edited brief unless replacement was explicitly authorized, or use a new snapshot destination. There is no mandatory directory, sidecar, schema, index, or automatic refresh and drift tracking. The developer owns the snapshot and separately decides whether to send it to a design tool.

Generation and revision change no specifications, node state, claims, questions, events, approvals, gates, or Git history. Do not invoke `join`, `migrate`, `eval`, `check`, `sync`, or other state-writing commands, enter `specify` or `mend-drift`, upload anything, or begin prototype construction. Instruction refresh is a separate developer operation. No terminal lifecycle readout or `design-brief` readout seam exists. Product changes discovered during design return separately to the existing human-reviewed planning process.

### code-review

**Trigger.** The developer explicitly requests a code review of one epic, feature, task, bug, chore, or tweak ("code review feature F-rnte2m"), or names rows of an existing review ("log B1, B3, and TW1-3", "fix B2 on its branch", "drop CH1", "commit the review"). Keys, IDs, and unambiguous conversational targets are accepted. The whole project, an arbitrary diff, or a path is not a target. A review is never a suggested next move, and the request authorizes no approval, claim, or start.

**Reasoning.** The review covers the node's net change: every non-cancelled unit beneath it, located from in-flight branch heads, or for done work from the recorded branch, `review_ref.head`, or integration commits carrying the unit's ID. Findings are judged against current code and anchored in lines the node's work changed. Bugs are graded blocker, important, medium, or minor by impact, and any violation of an approved acceptance or `preserve` criterion is a blocker; each carries a traced or suspected confidence label. Tweaks, chores, and plan changes (new task, feature, or epic, or a revision of non-terminal work) complete the four numbered sections. Severity is review prose only, never a record field or queue position. Findings matching open shelf work or staged plan changes are marked tracked, earlier declines stay declined, and a returning defect is a regression, never a reopen. The review is read-only, like `design-brief`: no state or Git writes, no fetch, no test runs. It never samples or caps, and lists anything not reviewed under Limitations. Drift, style, accessibility, and gate attestation are out of scope.

**Choreography.** Resolve the target with `show`, `tree`, and `ls`; locate each unit's change with read-only Git; read open shelf work, proposed amendments, and earlier reviews; write `.tiller/reviews/<YYYY-MM-DD>-<node-id>.md` (suffixed, never overwritten); reply with every numbered row in one table. A disposition resolves the whole selection first and writes nothing on any bad row, rechecks only selected rows when pinned code moved, logs bugs, tweaks, and chores through `tiller add ... --attach --source <review block>`, and logs plan changes by landing a distillation of only the selected rows with `tiller intake` and staging one sourced operation each. "fix on its branch" returns unmerged findings to their unit through `execute`, or to the unit's owner. Each disposition annotates its rows in place and makes one commit; nothing is pushed.

**Hand-off.** The review arm stops with the uncommitted file's path; for a feature or task in a project that already declares an optional code review gate, it prints the attest command with the manifest's gate key and leaves the status to the human. The disposition arm stops with the exact `tiller approve bug|chore|tweak <node>` and `tiller approve amendment <key>` commands, run by no one but the human. There is no `code-review` readout seam.

---

## Format, packaging, and portability

Each skill is a directory with a `SKILL.md` at its root. The file carries frontmatter with a `name` and trigger `description`, then Reasoning, Choreography, and Hand-off sections with the applicable output instructions. `design-brief` includes its entire seven-part Markdown contract in that delivered file; it adds no reference loader. The format is plain markdown, and the content is harness-agnostic by construction: the reasoning is prose and the choreography uses the filesystem, shell, and `tiller` commands, neither specific to any one harness.

That agnosticism is a hard rule, not an aspiration. **A skill may use only what every harness has: a filesystem, a shell, and the `tiller` CLI.** It never calls or names a harness-specific tool, so it reads "run the command," never "use your Bash tool," the way the surrounding skills speak in actions rather than runtime tool names. The canonical artifact is the neutral skill itself. Where a harness wants skills packaged into a particular discovery format, that packaging is a thin adapter over the same neutral file, and no harness's format is the source of truth.

**A skill never references a guidance document outside the project.** Skill text names in-project paths, `tiller` commands, and other skills reachable through `tiller guide`, never documents in a Tiller source checkout that may be a different version. When another skill carries a needed shape, point at `tiller guide <skill>` rather than restating it or searching outside the project. Developer-directed work outside the project remains allowed.

This buys a backstop worth the discipline. Because a skill is just a file and `tiller` is just a CLI, the lowest-common-denominator harness, a local open-weights model with a shell and no skill system at all, can use Tiller completely: run `tiller`, read the skill. A richer harness gets description-triggered activation for free; a bare one loses nothing essential. To make the guidance reachable without depending on the harness at all, the CLI surfaces it directly, since `tiller guide` prints the skill for the current state, so the same reasoning a fancy harness auto-loads is one command away in any of them.

Tiller embeds eleven neutral skills. Join installs eleven gitignored catalog-derived discovery shims during onboarding; upgrade and project-only maintain them alongside orientation, harness managed regions, and required ignore entries. The shims delegate full bodies to `tiller guide`; stable generated ownership markers permit conservative obsolete-shim cleanup without deleting custom skills or extra files. Maintenance touches only this resolved checkout and can refresh current schema without identity; teammates pull a reviewed migration and maintain their own checkouts. Do not hand-edit installed shims or generated orientation.

---

## Staying in sync with the CLI

The skills name exact CLI commands, so a change to the CLI can leave a skill stale. The rule is the same one the CLI surface uses against the schema: this document is a layer over [cli-surface.md](./cli-surface.md), that document is canonical for the commands, and where the two disagree the CLI surface wins and the skill is fixed. Skills reference the CLI by its stable verbs, which are the contract between the two layers, so a flag can change without touching a skill but a renamed verb is a coordinated edit across both documents.

The two layers reinforce each other at runtime as well as on paper. `tiller status` is lifecycle-aware and names the legal next moves; the skills are how those moves get made well; and `tiller guide` surfaces the fitting skill's text directly, so the same guidance a rich harness auto-loads is one command away in any of them. Naming the suggested skill inline in `tiller status` output is the natural next step, closing the discovery loop so `orient` and the status oracle point at the same place.

---

## What skills must not do

The refusals mirror the CLI's, because the skills are the reasoning layer over the same principle.

- **A skill never decides a gate.** An explicitly delegated start-gate `approve` is permitted, but `attest`, `sync --accept-fast-forward`, and the final integration merge are human-only. Task-to-aggregate assembly is explicitly agent-authorized and crosses no done gate.
- **A skill never absorbs the craft.** No skill carries coding standards, a TDD rhythm, or architecture. It hands out to `AGENTS.md`, `CLAUDE.md`, and the harness's own skills.
- **A skill never invents canonical structure.** It mutates state only through the CLI and respects the artifact schema. Lifecycle reasoning normally lands through a `tiller` command. `design-brief` is the narrow read-only export exception: it returns a derived Markdown snapshot, writing only an explicitly requested deliverable, never a new canonical artifact or state.
- **A skill never makes Tiller orchestrate Git or the PR host.** The binary records and verifies branches, gates, and review identities. The executing model creates task and aggregate branches, merges task tips and submission state into the aggregate, resolves mechanical conflicts, runs checks, and maintains one PR through ordinary Git and host tools. It never runs a human reviewer, attests, asserts a fast-forward crossing, or merges into configured integration.

---

## Decisions settled so far

Each line is a settled decision; the sections above carry the current contracts, and the [design history](../design-history.md#harness-agnostic-skills) explains the reasoning.

- Skills are opinionated about the shape and gating of the work and silent on implementation craft, which belongs to the project's own instruction files.
- A skill never decides a gate: lifecycle skills drive to the edge of done and stop for the human.
- A skill uses only a filesystem, a shell, and the `tiller` CLI; any harness packaging is a thin adapter over the neutral file.
- `tiller guide` serves the complete skill text, so a harness without skill discovery loses nothing essential.
- Specification, acceptance contract, and task breakdown are one just-in-time step behind one feature approval.
- A lifecycle seam ends on the CLI-rendered readout, reproduced verbatim, with the agent's commentary in notes ahead of it.
- The developer owns the work: no agent, model, provider, or harness is named as an author or contributor.
- An authorized execute run commits, publishes, and submits its own work; Tiller stays record-only for Git and the PR host.
- Review feedback is routed by what changes and the affected node's state, so not every correction becomes an amendment.
- `capture` proposes one complete shelf contract and stops for human triage.
- `derive-baseline` records unbuilt capabilities as questions rather than deciding to build them.

---

## What's next

The human-facing counterpart is the [read-only console](./ui-surface.md). Additional harness discovery adapters and automatic uncovered-root baseline routing remain unbuilt; the [roadmap](../roadmap.md#not-built-yet) lists these alongside the wider product gaps.
