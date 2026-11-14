---
name: orient
description: Use when you enter a Tiller repo or need to choose unrelated next work. Reads project and node state, relays the ranked next moves, and ends the reply for the developer's redirect or go. A reported merge or acceptance decision for authorized work re-enters execute for bounded closeout instead.
---

# orient

This skill assumes `.tiller/orientation.md`. Read `.tiller/orientation.md` first if it is not in context. If it lacks a section this skill names, it is stale: tell the developer the project needs `tiller upgrade --project-only`.

## Trigger

You begin work in a Tiller repo, or you just finished a step and need to choose the next unrelated work. This is the broadest moment in the set and the only one not tied to a single node's state. When in doubt about what to do, start here. A developer reporting a merge or acceptance decision for already-authorized work instead re-enters `execute` for that work's bounded closeout; it is not permission to start unrelated work.

An explicit request for a UI/UX design brief or prototype handoff for an epic, feature, or task instead loads `tiller guide design-brief` directly. It sits outside the ranked next list, even when the list names a gate or other work. The request already authorizes the read-only export, not specification, approval, or execution. Follow that guide and end with its Markdown deliverable, without orient's readout or next-work handoff. An ordinary next-work request, draft feature, or completed step does not trigger it.

For that export, prefix every Tiller read, including status and guide, with `env GIT_OPTIONAL_LOCKS=0`, and use `git --no-optional-locks` for direct Git inspection. Tiller reads may invoke Git internally; disable optional index writes from the first read, not only after loading the skill.

An explicit request for a code review of one epic, feature, task, bug, chore, or tweak, or a request naming rows of an existing review to log, fix on their branch, drop, or commit, instead loads `tiller guide code-review` directly. It sits outside the ranked next list the same way. The request authorizes only the read-only review or the named disposition, never approval, claiming, or starting. Follow that guide and end with its reply, without orient's readout or next-work handoff. An ordinary next-work request or completed step does not trigger it.

An explicit request to upgrade Tiller, migrate this checkout's schema, or refresh its generated guidance instead loads `tiller guide upgrade` directly. It sits outside the ranked next list the same way. The request authorizes only the maintenance it names, never lifecycle work. Follow that guide and end with its phase report, without orient's readout or next-work handoff. Entering a project, an ordinary next-work request, or a completed step does not trigger it.

## Reasoning

Apply the developer authorship rule in `.tiller/orientation.md` throughout every seam. The developer owns and is responsible for the work. Never identify yourself, your agent, model, provider, or harness as an author or contributor in code, comments, docs, commits, PRs, issues, reviews, reports, or replies. No generated-by notices, agent signatures, badges, footers, or agent co-author trailers. Describe the work and observed verification, not the tool that produced it.

Read the lay of the land, then relay the ranked list to the developer and end the reply. You are not doing the work at the seam in this reply, and you do not cross into it either: the crossing into the seam skill comes after the developer's next message, which redirects you, or starts the top agent move only when it says to.

That stop is the briefing before a seam is authorized. Once the developer has instructed execute, its run continues through checks, review preparation, and publication, and after human acceptance through formal submission, without a second go; a reported merge re-enters it for bounded closeout. `tiller guide execute` carries that sequence. Pure orient never opens a review, and explicit no-publish or earlier-stop instructions control.

A few reads tell you where things stand. `tiller status` with no node gives the project summary: the project, its registered contributors, and how many nodes of each noun exist. `tiller queue` gives the current attention entries from the Queue projection; use `--kind <queue-kind>` and `--noun <noun>` only to drill into the count summaries the readout names. `tiller ls bugs`, `tiller ls chores`, and `tiller ls tweaks` show the three standing shelves in soft order. The readout always carries the exact standing line `queue: N bugs, M chores, K tweaks ready`, with the actual counts and singular nouns when a count is one. `tiller status <noun> <node>` gives one node's state, the `next` moves the lifecycle allows from there, its recorded gates, and, when a spec or contract has been edited out from under approved work, an `active_basis` line naming the file and the command that re-pins it. That one-node oracle is the only surface that reports basis drift; the project summary does not, so when a gate refuses on a basis you did not expect to have moved, ask the node. `tiller ls features --contributor <you> --status active` shows active features you have contributed to, `tiller ls tasks --status check` finds checked work still awaiting submission, and `tiller ls tasks --status in_review` shows task submissions whose resolved review may still need publication.

For discovery, use `tiller search <keyword>` for prose and `tiller query --from <address> --follow <directions> --json` for recorded relationships; `query --help` lists the complete forward/inverse catalog. A work ID with `--follow mentioned_by` finds document citations, including labeled generated mentions. A `path:<repo-relative-path>` start with `--follow covered_by` finds exact or enclosing recorded scopes, not verified implementation coverage. Repeat `--from` for several starts; follow names are a union at every hop, depth defaults to one, and exact kind/status/owner filters select after traversal without hiding bridges.

Read query diagnostics and `complete` before calling discovery exhaustive. Missing eval/history input or ambiguous historical identity is incomplete; unsafe corpus or unstable-generation reads fail. Unknown references do not become current nodes, and opening a current file does not retrieve pinned historical bytes. Question `blocks` edges are recorded targets, not effective inherited holds or assumption waivers: keep status/readout as the authority for legal work. These reads require no identity and write no files, events, or persistent cache; they never run bindings or refresh guidance.

From that picture the dispatch is a ranked list. Run `tiller next` (or read the readout's **Next:**). Relay the list as written. If any human move ranks above the top agent move, name it first. Then end the reply. Start the top agent move in your next turn only when the developer's reply says to: a go, a yes, or an instruction naming that row. A reply that asks something else is answered, and the list still waits for its go. A reply naming a different row or other work redirects you to that. You may say you would take a different item first and why; never rewrite the list. A ranked move is a suggestion, not authorization.

The list holds at most five legal moves for this checkout, human decisions included, one row per node. Each row leads with its actor, `You` or `Agent`, then the action, the reason, and the exact command, or the branch to continue on. Tiller ranks the moves by a fixed rule and never calls a model. The rule has five tiers, in this order, with the kind of move and then plan order breaking ties inside each. Unblock nearby comes first: human decisions that hold work in your current feature, plus any question overdue or due within two days wherever it sits. Continue is your own unfinished work, and your top Continue move is always kept in the five. Nearby is the rest of the current feature, then its epic. Recently touched is the features and epics you acted on within the last fourteen days, most recent day first. Everything else follows in plan order. The current feature is the feature touched by your most recent deliberate act that names one: a claim, start, submit, check, spec, approval, answer, attest, link, reassign, or release. Acts on an epic alone and amendment steps never set it, and sync, migrate, join, and other upkeep never count. Adding work only touches its parent for recency, an added task its feature and an added feature its epic, so a decomposition never sets it. `tiller next --all` lists every legal move, `--json` prints the payload, and `--contributor` ranks for someone else. With no identity, `tiller next` and bare `tiller guide` head the list as not personalized (the readout's **Next:** has no heading), the personal tiers are skipped, and plan order ranks the rest, though a question overdue or due within two days still ranks first; `tiller join` fixes that. When nothing is legal it prints `Caught up: nothing is legal to do right now.`

Every row can be acted on as printed. Work in flight on another contributor's branch, work blocked by an unmet dependency, and an unclaimed shelf node are not rows. The standing queue line reports ready, non-carried shelf work, which a human chooses before any claim. An unclaimed task matches no personal assignment until its claim succeeds.

Bare `tiller guide` prints the same list, then names the skill for the highest-ranked agent move, since the top row is often the human's. Your pick and the CLI's cannot disagree for every move the list computes from the recorded tree. The `mend-drift` seam is this skill's own addition: knowing it applies takes an actual `tiller drift` run, which the list never makes on its own, so neither `tiller next` nor `tiller guide` suggests it. The `amend` seam is another such addition, for a kindred reason: a reshape is named by the developer's judgment, or by reading a drift cascade whole, never by a field in the tree. `capture` also sits outside the list: when an investigation or developer description identifies a concrete bug, chore, or tweak, the operator initiates capture rather than reaching it through a ranked move. The amendment wait below is not an addition of that kind: the list does name it, because a proposed amendment is recorded state like any other.

Ownership gates execution, not judgment. Claim the task with `tiller claim task <key>` before starting, submitting, or resuming it; claim bugs, chores, and tweaks individually after the human selects shelf work. For reservations, the owner a first start fixes, and the judgment verbs open to every registered contributor, follow **Ownership.** in `.tiller/orientation.md`.

Features and epics display Contributors, who started work beneath them: that is history, not a current assignment. `tiller ls features --contributor <id>` finds that history; `tiller ls tasks --owner <id>` finds scalar assignments. Who counts follows **Contributors.** in `.tiller/orientation.md`.

Started unfinished work changes hands only through `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`, explicitly authorized by a human naming the work and recipient. What it preserves, refuses, and leaves to Git and host work is under **Exceptional human-directed transfer** in `tiller guide execute`.

Treat review-grain configuration as a gate decision and name its safe timing in the briefing: where grain is set, what locks a change, and how a refusal recovers follow **Review grain.** in `.tiller/orientation.md`. Do not propose a state-file edit as recovery.

Different owners may execute tasks in one feature or epic review, and any registered developer may perform its authorized aggregate assembly, checks, and submission, with no coordinator claim. How they coordinate and what a coarse finish line still checks are under **Aggregate assembly** and **Review grain and coarse submit guards** in `tiller guide execute`.

Every task owns its recorded working branch, even when it rides a coarse review. Authorized execute does the routine branch work and aggregate assembly itself, under **Aggregate assembly** in `tiller guide execute`; do not hand it to the developer. Only the final integration merge, acceptance, or a semantic conflict requiring judgment is a human stop.

Workflow advice is separate from the node's lifecycle `next` moves. Brief a feature's finish from its contract bindings and required gates beside the recorded results: missing, unconfigured, failed, or stale machine work and an ungraded or stale rubric are agent work, and only once those are complete can the wait be the human's acceptance. At epic grain name both the epic review and the particular feature contract needing work, never an epic eval. The rest is under **Publication sequence** and **Feature acceptance and merge closeout** in `tiller guide execute`.

The ranked list is pure: it cannot verify drafts, host state, or pushes. Say "formal submission not recorded," not "no PR exists." Execute supplies verified host facts in run notes before its handoff; `tiller guide execute` carries how. Recommendation order is not full mutation-guard parity: respect named basis, question, dependency, ownership, and retired-node diagnostics rather than advertise a known-refusing command as clean work.

These are the moves the list can hold and the seam each one leads to:

- The repo is not a Tiller project yet (no `.tiller/`, or `tiller init` has not run), this is the onboarding entry. Scaffold with `tiller init --key <key> --kind <archetype> --name "<project name>"`, adding `--design <dir>` to import a design folder in the same move; the three flags are required and init refuses without them. Registering a person and selecting your identity are separate deliberate acts: a human runs `tiller add contributor <you> --name "<full name>"` (an agent only on explicit instruction naming both), then `tiller join --as <you>` selects that registered identity. Without them mutations lack an acting contributor and readiness reports an empty roster. Tiller never runs your agent. Init runs now; name the applicable first seam below and stop for the developer's go. Greenfield prepares design inputs for decompose. Brownfield and legacy-migration first derive or reconcile a baseline; an import alone does not confirm one.
- `project.kind` is `brownfield` or `legacy-migration` and `.tiller/design/baseline.yml` has no confirmed doc, the seam is `derive-baseline`, even when epics or work already exist. Read the ledger and pending prose before proposing new writing. Existing unconfirmed docs may already be a complete committed proposal: inspect/reconcile them or hand them back for human review with `tiller approve design <doc>...`, never blindly regenerate duplicates. The baseline review is not a work-node queue entry. One confirmed doc releases this startup move; remaining pending docs are not a global gate. Contributors, legacy reference declarations, root modes, and other readiness gaps remain independent.
- The tree already has epics, and inputs exist under `.tiller/design/` that it does not yet reflect: the seam is `decompose`, after the existing-code baseline move above. A later document comes in through `tiller intake <file or folder>`, which lands it under `.tiller/design/` and opens a `draft` amendment: the amendment is the unit of work, and its draft is an agent move.
- A draft amendment is waiting on the agent. When it carries a source document, the seam is `decompose`, which stages the document as drafts the amendment holds and ends at `tiller propose amendment`. When a developer opened it by hand and it has no source, the seam is `amend`. Either way it ends at `tiller propose amendment`, which hands it to the human; the amendment is not the human's until it is proposed.
- You hold a task in `check`, `ready`, or `in_progress` and no operative direct or inherited question blocks it: the seam is `execute`, and the list files it under Continue. Re-enter checked work on its recorded branch without start/resume and finish its outstanding checks and publication or submission, and preserve dependency and soft order. Shelf work you hold in those states is the same move. An unclaimed shelf node is not suggested, and live carried work waits on its carrier. `carrier_cancelled` means owned work has returned to its shelf; it remains a human needs-attention row with `resolve --clear` or `resolve --by`. Ownership is per unit, never inherited from a feature. Assignment visibility includes non-executable proposed/blocked and in-review work without advertising it as ready.
- A task held by an operative open question is not a move; the holding question is, as a human answer. Name `tiller answer question <Q-id> --answer "..."` as the developer's move. Answering one question may leave other blockers.
- A proposed amendment awaits the human's approval: that is `tiller approve amendment`, the human's move, not a seam. A proposed amendment with nothing staged reads as "nothing proposed from this document" and carries the withdraw command instead. Name it and stop. `tiller ls amendments --status proposed` names what is open, and several can be proposed at once, each its own row.
- Shared feature/epic closeout appears once per reviewed unit, never per contributor. A coarse finish line arrives when every live riding task is `in_review` or `done`; at epic grain every independent override is already done. A task-grain feature whose live tasks are all done needs feature acceptance and ordinary sync on integration. Do not assign this work to the last finisher or historical contributors. A ranked closeout move offers related closeout but does not authorize it; once authorized, `execute` derives the checks, rubric work, human acceptance, and submission from the feature contracts and required gates. Pure reads cannot claim a draft exists; authorized execute prepares the real review surface before handing off acceptance. Terminal done work keeps its acceptance warnings and historical violations without starving live work.
- An unclaimed eligible `ready` task under an approved ready or active feature and an epic past decomposition, with no unmet recorded dependency or operative question hold: the seam is `execute`, naming `tiller claim task <key>` for that individual unit. Preserve epic/feature dependencies, approved bases, operative questions, and task order. Approval itself neither claims nor starts a task; the suggestion is not authorization.
- A feature sits `specced` awaiting its plan gate with no open question holding its plan: that is `tiller approve feature`, the human's move, not a seam. Name it and stop. A plan held by an open question (on the feature, its epic, or any live task under it) is not that move; the question is, as a human answer. Name `tiller answer question <Q-id> --answer "..."`, after which specify folds the answer into the plan before the approve. An open question leaves hold in one of two ways: an answer, which records a decision, or `tiller cancel question <Q-id> --reason "..."`, which records that none is needed. Cancel and `tiller unlink question <Q-id> --blocks <node>` are human moves: run them only on the human's instruction, otherwise name the exact command in Notes. You may `tiller link question <Q-id> --blocks <node>` to widen a question that reaches further than recorded, and say so in Notes.
- An epic sits `draft` with live features under it, awaiting its decomposition gate: that is `tiller approve epic <key>`, the human's move, not a seam. Several epics list in recorded soft order, the order `tiller queue --kind approve_epic` shows. `tiller spec feature` stays legal on any draft the developer names; ordering alone is not a mutation guard, and the gate shapes only the suggestion.
- The frontier is the project's live draft features whose recorded `depends_on` targets and whose own epic's recorded dependencies are all settled (`done` or `cancelled`); an epic-grain edge gates the epic's features exactly as a feature-grain edge gates the feature. The seam is `specify`. Among the eligible, recorded soft order chooses first: the owning epic's place in it, then the feature's own. Features and epics have no owner, so never present the frontier as one person's reserved work. Explicitly named drafts remain legal to specify even when the list does not show them. A draft waiting on unfinished work is no move and never a row. When every draft waits, the tree is still not empty: dependency diagnostics (Home, `tiller check readiness`) and the caught-up hint explain the held work.
- A task, bug, chore, or tweak sits in `in_review` because its PR review requested changes, the seam is `execute`, which re-enters through that noun's `resume <noun> <node> --reason "..."` and reuses the existing open PR. An old PR number on resumed or checked work does not mean the changes were resubmitted. Nothing in the tree records that a review asked for changes, because that lives on the PR, so this arm is read off the conversation rather than off state, as the `amend` arm below is.
- `tiller drift` reports a candidate and nothing in the list already claimed the moment, the seam is `mend-drift`. A candidate mend-drift already answered that persists by design is not a move.
- The change in front of you reshapes the plan rather than extends it, whether the developer says so or a `tiller drift` run reports one upstream change cascading across a dozen nodes, and the seam is `amend`. Nothing in state names this one, so it is reached by judgment: say what you think it is and let the developer confirm before anything is staged.
- A node sits cleanly in `in_review` only waiting on a merge: a human wait, not a seam. A live carried shelf node waits on its carrier instead. At coarse grain a riding task's `in_review` alone proves neither acceptance nor formal review; inspect the resolved feature or epic. Outstanding acceptance may still be a human decision, while absent machine evidence or an ungraded rubric remains agent work. When formal review is recorded, name only that recorded identity and do not infer its current host state. Surface the wait and stop once submission state is published when remote-backed. If an authorized run still owes ordinary state publication, finish it rather than hand the second push to the human.
- No epics are recorded and no move above applies: decompose is due only when a to-be design doc exists, a file under `.tiller/design/` (dotfiles excepted) that `baseline.yml` neither registers as an as-is doc nor cites as a source, and then the seam is `decompose`. With none, no work is planned yet, a human wait: name the open-question count and the ways work enters (`tiller intake` a document that makes a case for change, capture a bug, chore, or tweak, or add a to-be design) and stop, without proposing epics from as-is docs or old specs. On a tree with no epics, owned shelf work in `check`, `ready`, or `in_progress`, a proposed amendment, and a shelf merge wait still list; unclaimed shelf work never does.
- Nothing is on the frontier or in the executable work loop: the list prints `Caught up`, which is not necessarily empty. No eligible unclaimed ready task remains. A diagnostic can still count unfinished unclaimed tasks that are proposed, held, or otherwise ineligible, explaining the first dependency-ranked task without a claim/start command. The diagnostic makes no ownership claim and is omitted when no unfinished unclaimed task remains. This differs from the eligible ready-task pool. An eligible unclaimed task is a claim move, not caught-up; a held reservation, past contribution, or draft feature is not a claimable task.

The list needs no side notes. What the old notes described is now a row of its own: the holding question, your own in-flight branch, the neighbouring feature, and the next eligible task each rank as moves. Do not add a note that steers toward or away from a ranked row beyond saying you would take another row first and why.

The lifecycle is the queue: `draft` is the backlog, `specced` and `ready` are on deck, `active` is in progress, and only the human's go pulls a feature forward. Plan the known scope as bounded product and platform increments, not permanent subsystem buckets, then specify each feature just in time. Related capabilities can recur in later increments. Record true prerequisites as `depends_on`, preferred sibling sequence with `tiller order`, and execution responsibility per task. Soft order arranges the render and the list among peers the graph leaves unordered and never gates execution. Independent chains permit parallel work, and there is no layer of planning between the tree and the work.

A move that is not legal does not appear, so an ineligible preferred node never blocks a legal alternative. Inside a tier, moves sort by kind in a fixed order first: the baseline, your own continuing work, amendment review, closeout, claiming, resolving a blocker, plan approval, epic approval, specify, merges, answers, attests, triage, then decompose. Within a kind, plan order decides: epic order, feature order within an epic, and task order within a feature. Tier 1 sorts by due date first, overdue first. A task's local ordinal is not global priority over unrelated features. Completed prerequisite depth does not outrank a live preference; historical tree rendering and stored dependency edges remain unchanged.

One feature just in time remains specify's default. An explicit instruction naming exactly several draft features, or every draft feature in one named epic, creates a bounded batch that overrides the list's pacing only for that set. Follow rendered soft order. Complete the full specification, contract, and tasks for one feature, run `tiller spec feature`, and make its separate feature-only commit before continuing. Add no unnamed siblings, never approve, and finish the batch with one final `tiller readout specify`.

The ranked list is guidance, not command authority. An explicit human instruction naming nodes overrides the list's order for exactly those nodes. When recorded state leaves a gate or merge outstanding, say `the ranked list suggests waiting`. Describe a wait as a Tiller requirement only when a command actually refused. Only exit `1` is Tiller refusing with a business answer; exit `2` or `3` means correct the call or environment.

## Correction routing

Classify explicit human feedback before choosing a seam, then route it by the correction table in `.tiller/orientation.md`.

## Recovery protocol

Follow the recovery protocol in `.tiller/orientation.md`.

For `design-brief`, recovery is limited to corrected reads. Report missing sources, drift, and unavailable instructions as limitations; do not repair sources, refresh instructions, record questions, or enter another skill to fill the gaps.

For a `code-review` review, recovery is limited to corrected reads. A disposition records through `tiller` commands and recovers by the ordinary protocol above.

## Node references

Name and address nodes by the node reference rule in `.tiller/orientation.md`.

## Choreography

Reads, plus the one scaffold and any question the briefing defers. This is the skill whose output is a decision rather than a state change. On a bare repo `tiller init` is orient's to run; the stop lands after it, before the applicable first seam: derive-baseline for an unconfirmed existing-code project, decompose when a to-be design doc exists, and otherwise the no-work-planned end state. A delegated `approve` runs only on an explicit human instruction naming the nodes or design docs, after the stop, never as the next line of an orient.

```
# cold start only, on a repo with no .tiller/: the scaffold, then the stop.
# init is the one command this dispatch runs for you; add contributor and join are
# shown for reference; the developer runs those, on a human's explicit go.
tiller init --key <key> --kind <archetype> --name "<project name>"   # add --design <dir> to import inputs
tiller add contributor <you> --name "<full name>"       # a human registers you; without it join and every mutation refuse
tiller join --as <you>                                 # selects the identity add contributor just registered; without it add/start/check exit 3

# every run: the reads the dispatch is made from
tiller status                                          # the project summary: the project, its contributors, its counts
tiller queue                                           # exact current attention entries; add --kind or --noun to drill down
tiller ls bugs                                        # the standing bug shelf in its soft order
tiller ls chores                                      # the standing chore shelf in its soft order
tiller ls tweaks                                      # the standing tweak shelf in its soft order
tiller status feature submit-certification/certification-form-loading    # one node's state, its next moves, its gates, any active_basis drift
tiller ls tasks --owner robin                            # your scalar task assignments
tiller ls features --contributor robin                    # started-task contribution history
tiller ls tasks --status check                         # checked work still awaiting submission
tiller ls tasks --status in_review                     # current task submissions; coarse refs may still need the resolved review
tiller drift                                           # are there candidates waiting; a marker names the seam mend-drift
tiller ls amendments --status proposed                 # is a staged reshape waiting on the human's approve
# optional relationship discovery; these do not choose or authorize work
tiller query --from F-79dh3q --follow mentioned_by --json
tiller query --from path:app/models/calc --follow covered_by --json
# On an existing-code startup, also inspect the baseline ledger and its docs.
# If a proposal already awaits confirmation, inspect it rather than derive again.
# only when the briefing defers a decision: record it, then commit exactly its paths
tiller add question "<statement>" --title "<short title>" [--blocks <node>]
git add -- <question-ledger-and-event-paths>
git commit --only -m "orient: record <short title>" -- <same-exact-paths>
tiller readout orient                                  # the picture, rendered as the one block every harness renders the same way
```

`tiller ls` takes epics, features, tasks, bugs, chores, tweaks, questions, and amendments.

## Readout

orient lays no work down, so its block is not a report of work done, it is the picture itself: the same
survey the Reasoning section above just walked, rendered the one way every harness renders it. Run
`tiller readout orient`.
Reproduce the block verbatim and last, per the readout rule in `.tiller/orientation.md`.

The block carries one singular `Needs attention` list. It is counts-only: each bullet states an exact
Queue count and an inline `tiller queue` drill-down. Do not expand it into concrete node rows, question
rows, gates, criteria, or mutation commands. The queue command is the one place to enumerate a count.
Baseline docs awaiting confirmation have their own review presentation, not a fabricated `approve_design` queue kind. Keep the rendered baseline review and human next action intact; an empty work-node queue does not mean those docs need no human.
Your own pick, if it differs from the ranked list, goes in the notes ahead of the block.

Before the block, add a short **Notes from this run** list for what it cannot know: context from earlier
in this conversation that changes the read, a correction the developer should make to the picture, or a
question worth recording rather than assuming an answer to.
Name nodes by key path with the id in parentheses.

## Hand-off

Up to the developer, always. Surface the picture and the ranked list, naming any human move that ranks above the top agent move, then end the reply. Their next message redirects you, or starts the top agent move only when it says to, through its seam (`derive-baseline`, `decompose`, `specify`, `execute`, `mend-drift`, or `amend`). The briefing is where they correct your understanding and add project context; conventions worth keeping belong in `AGENTS.md`. A deferred briefing item becomes a recorded question (`tiller add question "<statement>" --title "<short title>"`, with `--blocks` naming held work), committed as the Choreography shows, not chat history. Some moves are human gates: pending baseline confirmation, a staged amendment, draft epic, feature plan, open question, or final merge wait. Surface the applicable gate and stop. Run `approve` only on explicit human instruction naming nodes or docs; baseline derivation never authorizes its own confirmation. `attest`, `sync --accept-fast-forward`, and final integration merge remain the human's keyboard, not ordinary aggregate assembly.
