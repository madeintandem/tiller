---
name: execute
description: Use for authorized ready, in_progress, or checked work, requested changes or acceptance re-entry, a coarse review finish line, a reported merge, or explicitly authorized historical aggregate review recovery. Completes related checks, review preparation, submission or integration-branch closeout, stopping at the exact human decision or real blocker.
---

# execute

## Trigger

A task, bug, chore, or tweak is `ready`, `in_progress`, or `check` and owned by you; reviewed work has requested changes; a human acceptance decision resumes the same review; a coarse review has reached its finish line; or the developer reports a merge of the related authorized work. Checked work re-enters without `start` or `resume`: complete outstanding checks and submission on its recorded branch. A reported merge authorizes bounded closeout of that work, not a new feature or unrelated planning. Inspect state and the merge, then continue on the integration branch through applicable feature acceptance, ordinary sync, state publication, and readback. A pure orient request still briefs and stops.

Explicit authorization to recover a named feature/epic review also enters execute. A reported merge alone authorizes ordinary bounded closeout, not creation of missing historical PR/head/result identity; follow the missing aggregate recovery section below before recording it.

## Reasoning

This skill is the thin one, and its thinness is the point. Its Tiller-specific content is the discipline of the loop, not the building.

Apply the managed-project independence rule from `.tiller/orientation.md` before changing setup, project commands, CI, or deployment. Keep project commands directly runnable without Tiller or `.tiller/` inputs; run local `tiller check` around those commands to record evidence, never from inside them. An approved plan that introduces Tiller into those operational paths still conflicts with this rule: surface the conflict and propose an independent alternative rather than installing Tiller, adding an optional CI job, or requesting release credentials. Preserve applicable project checks and their real failures; correction of accepted contracts still needs the applicable human review.

The developer's instruction to execute authorizes ordinary code and state commits, pushes, PR creation or reuse, and Tiller submission for that work, including related post-merge closeout. An explicit instruction not to publish or to stop earlier controls. An already-authorized run does not need another go between checks, review preparation, and publication. Orient's briefing stop applies when choosing a seam, not within this loop. Stop for the actual human decision, not for another permission to do ordinary publication. Tiller itself neither pushes nor creates PRs; you perform those operations with ordinary Git and the project's existing host tools.

### Worktrees and other agents

Follow the worktree rule in `.tiller/orientation.md`. Unless the developer's or project's own instructions say otherwise, execute each unit of work in its own Git worktree and leave the primary checkout as the developer left it; their instructions win over this default. A unit of work is what one review covers, so review grain sets it: the task at task grain, the whole feature at feature grain, the whole epic at epic grain, and always the bug, chore, or tweak itself, which owns its review at every grain. Assume other agents and developers may be working this repository at the same time in their own worktrees.

- Every worktree you add needs its own identity: `.tiller/identity.yml` is per checkout and gitignored, so a new worktree refuses mutations with `contributor_unset` until `tiller join --as <you>` runs inside it. Join also refreshes generated guidance when the committed generation is older than this binary. Check `git status` after join: restore any tracked file it changed with `git restore` so the refresh stays out of task commits, and tell the developer the project needs `tiller upgrade --project-only` on its integration branch. Passing `--contributor <you>` on every mutation is the alternative that writes nothing.
- One worktree carries the whole unit. At feature or epic grain, check out each riding task branch and the recorded aggregate in turn inside that one worktree: build, check, and submit a task on its own branch, merge it into the aggregate there, then move to the next task. Feature checks, the draft PR, coarse submit, and requested changes run in the same worktree. Commit before every switch; never carry uncommitted work from one branch to another. One join covers the unit.
- Parallel work inside a unit gets its own worktrees. When you hand riding tasks to subagents or otherwise build them at the same time, give each parallel task a temporary worktree of its own, added and joined the same way, while the unit's worktree assembles their submitted branches into the aggregate. Remove each one once its branch is committed, pushed when remote-backed, and merged into the aggregate. Never let two builders share a worktree.
- Entry from `ready`: safely update the start base first. For a unit's first task, add a detached worktree at it and join inside; for a later task in a unit whose worktree you already hold, detach that worktree at the start base with `git switch --detach "$start_base"` instead. Then claim, start, and create the recorded branch there, so start's state follows onto that branch. Commit and share that state on the new branch.
- Re-entry at `in_progress` or `check`, and requested changes from `in_review`: use the unit's worktree when it is yours and check out the recorded branch there, or add one with `git worktree add "$worktree" <recorded-branch>` and join inside it. Run `resume` only inside that worktree, never in the primary checkout, because its state must land on the recorded branch. When Git reports the branch is checked out elsewhere, someone is using it: never force the checkout, and find out whose it is.
- Aggregate assembly runs in the unit's worktree, since it holds the aggregate between tasks. When another worktree already holds the aggregate branch, that worktree is someone else's: stop and coordinate rather than committing into it.
- Integration-branch closeout runs where the integration branch is checked out, usually the primary checkout, since Git checks out a branch in only one worktree. Commit there only when that checkout is on the integration branch and holds no changes this run did not make; stage only this run's delta. Otherwise stop and name what is in the way rather than switching its branch or committing around it.
- A worktree, branch, or uncommitted change you did not create for this work belongs to someone else: never remove, reset, clean, stash, commit into, or check out over it. Another agent's claim and start state lands on its task branch, not on the integration branch the CLI reads, so a claim made elsewhere may not show in Tiller's own state. Before claiming, look for the work's canonical branch in `git worktree list`, local branches, and the remote, and for observed activity in the console. When one exists or Git surfaces a conflicting claim or start, stop for the human rather than choosing a winner. The ranked list sees in-flight work only on local branches and worktrees; fetch to see teammates' branches.
- Remove only worktrees you created, and only after their work is committed and, when remote-backed, pushed. A unit's worktree outlives its tasks and stays until the unit's review is published.

### Shelf work

1. A bug, chore, or tweak owns its claim. Run `tiller claim bug|chore|tweak <key>`; do not claim or start an attached feature.
2. Read its own `bug.md`, `chore.md`, or `tweak.md`. If `attached_to` names an epic, read `epic.md`; if it names a feature, read `feature.md` and `feature.eval.md`; if it names a task, read that task's `task.md` plus its parent feature's `feature.md` and `feature.eval.md`. Those files are context, never an inherited contract or gate.
3. `start` is only for a shelf node in `ready`; it records the canonical shelf branch, cut from the integration branch, and then you create that new branch with `git checkout -b` inside the node's own worktree, joined first. For an already `in_progress` or `check` shelf node, skip both `start` and `resume` and check out the recorded branch without `-b`. At `check`, inspect the recorded gates and finish any outstanding checks or submission. The node ignores `review_grain` and always owns its review surface. A bug uses `tiller/bugs/<key>`; a chore uses `tiller/chores/<key>`; a tweak uses `tiller/tweaks/<key>`.
4. A bug fix includes a regression test that would fail on the reproduced defect. A chore or tweak makes the configured tests gate honest for its work; a code-changing tweak includes meaningful coverage for the adjustment. Execute runs neither `tiller eval` nor `tiller approve` for shelf work.
5. Follow the publication sequence below on the shelf branch: commit the fix, check and commit gate state, push, reuse or create the shelf PR, submit its actual number, commit and push submission state, read it back, then stop for human review and merge. Without a remote, submit a review ref and commit the state locally instead. Every commit while executing this shelf work carries its own bug, chore, or tweak id.
6. Run `resume bug|chore|tweak` only from `in_review` after requested changes, inside the node's worktree with its existing shelf branch checked out without `-b`. Fix the requested changes, rerun the gates, and resubmit using the same PR and publication sequence.
7. A live carrier makes `resolved_by` read `carried`, so the shelf node is not an execute target. `carrier_cancelled` returns the node to its own queue and does not block the owned work loop; its needs-attention row lets the human clear or replace the stale pointer, while `sync` treats the cancelled carrier as uncarried and observes the node's own branch.

Tasks arrive already `ready`. One `approve` at the plan gate took the feature to `ready` and its tasks with it, so there is nothing left to propose or approve here: read the plan `specify` wrote in the task's `task.md` and work from it. If that spec is thin or wrong in scope or approach, record that as a question before you start (below) rather than quietly re-deriving one; a wrong fact found while building is a departure, below.

While executing a task, a factual claim in its `task.md` that the code proves wrong, such as a name, path, signature, format, value type, or dialect, is a departure, not a plan correction. Build against what the code actually does and leave the accepted `task.md` unedited. Record the departure, what the plan said, what the code does, and where you read it, in the task's implementation commit message, which carries the task id and travels with the work at every grain and without a remote. Repeat it in the task's PR body when one exists, and in **Notes from this run**. Editing approved task prose would trip its accepted basis and cost the human a re-pin for a fact the reviewer can check in the diff. The departure is bounded: when the corrected fact changes the feature's behavior in `feature.md`, an acceptance criterion or its binding, the task's scope, a dependency, or drops a test or edge case the `task.md` plans, it is no longer a departure but a change to approved prose that needs the human's decision: record it as a question, below, and route its answer through the correction table in `tiller guide specify`. The rule covers only the task being executed; a planned task not yet started takes the ordinary same-scope prose correction.

Record a finding that needs a human decision and keep building: `tiller add question "<statement>" --title "<short title>" --blocks <node>`, where `--blocks` names the narrowest node the answer could change: the task being built when only it depends on the answer, a later task when only that one does, the feature only when a criterion is in doubt. When the question holds the task being built, commit the task's work and the question's state to its branch and leave the task where it stands. The question lands on the branch it is recorded on: integration's queue, the console, and any checkout cut from integration or detached at a later task's start base cannot see it until that branch merges. Push the held branch when the project is remote-backed, and keep its worktree. When the question holds a later task or the feature, hold that work yourself for the rest of the run rather than trusting routing in another checkout: do not start a task it holds, and do not carry the feature to acceptance past it. Continue with the next legal work in the run; routing in the checkout that carries the question skips a task it holds. The human answers on the same branch, in the worktree that holds it, the way a coarse attest runs on the recorded aggregate branch: `tiller answer question <Q-id> --answer "..."`, or an answer given in chat that you record with that command on their instruction, and the answer is committed on that branch. Never ask the question only in chat, where no gate or later session can see it. A wrong fact the code settles stays a departure, built through and noted, not recorded as a question. `tiller cancel question <Q-id> --reason "..."` (no decision needed) and `tiller unlink question <Q-id> --blocks <node>` are the human's moves: run them only on their instruction, otherwise name the exact command in **Notes from this run**. You may widen a question that reaches further than recorded with `tiller link question <Q-id> --blocks <node>`, and say so in the notes. Recording a question never stops a run by itself: stop only when the run has no legal work left, or at a stop it already makes (a review handoff, an attestation), whichever comes first.

An answer that fits the approved plan as written, picking between options the plan already left open, resumes the held task; note the answer in its implementation commit message the way a departure is noted. An answer that changes approved prose (a criterion, `feature.md`, or a `task.md`) takes the correction table in `tiller guide specify`, the route an out-of-bounds departure takes: write the answer into the prose and commit it, and the human clears the moved basis with `tiller approve <feature|task> <node> --refresh-basis`, the task form for a `task.md`, often in the same sitting as the answer. Work the moved basis holds waits for that refresh like any other hold.

Start a task only from `ready`, after claiming that task. `tiller start task <key>` atomically fixes its owner, records its working branch, activates a ready parent feature, and initializes the applicable aggregate branch when needed. A failed start changes none of those facts. At task grain the feature records no branch; feature and epic aggregates use `reviews/features/<epic>/<feature>` and `reviews/epics/<epic>` through the configured branch pattern. Task branches use the full canonical task key at every grain. Read the recorded names. Tiller never creates branches: use ordinary Git to create independent task branches from configured integration, preserving unrelated work. Task-grain dependencies wait for integration, never stacked task PRs. A coarse dependent may branch from the aggregate after submitted prerequisites and submission-state commits are assembled there. Keep task claim/start state available on its own branch without taking another developer's task or implementation.

For an already `in_progress` or `check` task, skip both `start` and `resume` and check out that task's recorded branch without `-b`. Requested changes use `resume` only from `in_review` and keep the same task branch and existing review. A task's branch is immutable on resume. At `check`, inspect status and finish outstanding checks or submission. For multi-task or unusually risky work, write a dated plan under `.tiller/plans/` before coding; routine single-task work uses its own approved `task.md`. Defer implementation craft to the project's `AGENTS.md`, `CLAUDE.md`, and harness skills.

Read back task start's `branch` and `base_branch`; use those recorded results, not a guessed name or the branch you happened to be on. A first task after a grain change may record a missing aggregate. A reopened coarse round rotates the aggregate to its canonical name plus `-<new-task-id>` instead of reusing a merged ref; historical refs remain untouched. A dependent coarse start points at the aggregate containing prerequisites; an independent start points at integration. Make the actual new branch from that base, keeping its CLI-written start state with the new task.

The local console can observe execution state in validated task worktrees without copying `.tiller/` state into integration. It can also read committed local task branches and configured remote-tracking tips. Publish machine state through the ordinary task commits and pushes in the publication sequence below, not by hand-copying records or making source-specific workflow commits. Another developer fetches remote refs externally, then navigates or manually refreshes the console; Tiller never fetches for visibility. Observed activity does not authorize an early start, change check/push timing, or make a dependency ready on integration. Queue, ranked list, workflow, and CLI eligibility still use the served tree. The console is not a real-time team server.

When adding tasks beneath a completed coarse review, run `tiller sync` from integration first to reopen the parent and clear the consumed review. `aggregate_round_closed` means this observation is still due. Retry start afterward and use its newly recorded aggregate branch.

What execute carries is the protocol around the building. When the work moves, renames, or deletes code its feature or epic records as surface, record the code's current location with `tiller link feature <feature> --surface <new-path>` and drop the old path with `tiller unlink feature <feature> --surface <old-path>` (use `epic` for an epic's surface), committing that state with the work; a recorded path missing from the integration commit leaves nothing to fingerprint, so the rollup cannot record done. Run `tiller check` and `tiller submit task` on the task's own recorded branch. At task grain submit records its actual PR; its title is the task's full reference and its body names the parent feature. At feature or epic grain submit records a review ref, with no task PR. Commit that submission state on the task branch, then merge its current tip into the recorded aggregate with ordinary Git before continuing dependent work. A coarse PR represents exactly the resolved feature or epic; its title uses that node's full reference and its body identifies riding tasks and, at epic grain, features. Task commits carry their task id; aggregate finish commits name the riding task ids they concern. Only final integration merge, human attestation, `sync --accept-fast-forward`, and done decisions remain human-only. Intermediate task-to-aggregate merges are explicitly agent-authorized.

### Aggregate assembly

Create a newly recorded aggregate from the configured integration branch; reuse it thereafter. Merge each submitted task branch, including its submission-state commit, into that aggregate using an ordinary ancestry-preserving `git merge` (a merge commit is fine). Never cherry-pick, squash, or copy task code into an aggregate. Before feature checks, draft PR preparation, or coarse submission, assemble every current riding task tip, not merely the older recorded review head. Tiller verifies both the recorded heads and current task tips are ancestors when submitting the aggregate. The agent performs branch creation, assembly, mechanical conflict resolution, gates, and maintenance of one aggregate PR, not the developer.

Git assembly must preserve nonoverlapping `.tiller/` task records, evidence, and immutable event files from every branch. State records for unrelated tasks are not an either/or choice. If an independent branch lacks another task's record, merge the history containing it rather than reconstructing machine state by hand. Do not hand-edit state YAML to settle a real decision conflict: stop and name the competing decisions for the human, then record the resolution through Tiller.

Keep claims and machine state through normal command writes and Git merges. Resolve mechanical conflicts without dropping another task's state, evidence, events, or implementation. The question ledger, `.tiller/decisions/open-questions.yml`, is one file several task branches now edit: resolve its conflicts by keeping every question from both sides, never dropping one. Semantic merge conflicts needing a product or acceptance decision stop on the spot for that exact human decision, since they block the aggregate every task shares; a decision found while building one task is instead recorded as a question. Requested changes return to each affected task's own branch, resume/check/resubmit there, commit its new submission state, merge its new tip back into the aggregate, rerun affected feature gates, and update the same PR. Integration remains untouched until the human's final merge.

Execution ownership and review responsibility are separate. Different developers can start different tasks in the same feature at every review grain. Any registered developer can perform authorized aggregate assembly, checks, and submission without owning the tasks. Coordinate one aggregate updater at a time through the team and Git; there is no coordinator claim or distributed lock. Requested changes belong to the affected task's original owner and branch. Shared closeout appears once per feature or epic, including feature acceptance after the last task-grain merge. Surface that next step without assigning it to the last finisher or every historical contributor; a ranked move is a suggestion, not execution authority.

Legacy feature-prefix refs can obstruct a task branch, for example `tiller/form-engine/certification-form-loading` blocks `tiller/form-engine/certification-form-loading/schema-transcription`. Name the desired and conflicting refs and inspect their local/remote status without altering them. Preserve historical refs and choose a non-conflicting configured branch pattern for unstarted work, or ask the human to archive/rename the collision deliberately. Never automatically delete or rename local or remote refs, force-push, or rewrite recorded task branches.

### Publication sequence

Resolve the reviewed node and recorded branch before publishing. On re-entry, inspect the recorded PR and look for an existing matching draft even when formal submission is not recorded; confirm repository, base, head, open/draft state, and published tip before making commits. Pure reads, riding task refs, and the absence of a warning do not establish host state. A merged branch is not a place to resume work or open another PR. Read the configured integration branch and intended remote from this project's configuration and Git setup; do not assume `main` or `origin`. Use the project's existing host tooling and credentials, not a new dependency.

Apply the developer authorship rule from `.tiller/orientation.md` to all work, not only publication. Before committing or posting, inspect the staged content, commit message, and PR title/body, including text supplied by templates or host tools. Remove agent self-attribution: generated-by notices, signatures, badges, footers, and agent co-author trailers. Never identify yourself, a model, provider, or harness as an author or contributor. Use the developer's configured Git identity, never an agent author or committer identity or an invented human identity. Preserve real human credits and required third-party notices. Code, comments, docs, review replies, and reports follow the same rule; the developer owns and is responsible for the changes.

1. Commit implementation and task records on the task's recorded branch, with required task or shelf ids. For coarse review, assemble submitted task tips and their committed state on the recorded aggregate before its checks and publication. Stage only this work, not unrelated changes.
2. Run the applicable `tiller check`; at feature acceptance this runs the contract and the feature's other configured gates with check's basis, question, and parent guards. Run the project's suite through `tiller check` rather than by hand and then again through check. A gate already passed on unchanged code is reused, which is normal and needs no `--rerun` to be sure; use `--rerun` only for a suspected flake or environment change. To iterate on one failing gate, run `tiller check <noun> <node> --gate <name>`; a gate left out whose pass is not current is marked stale, and the node advances only on a full green set. Inspect every result and immediately commit each actual `.tiller/` evidence/state delta, including on exit `1`. Never chain check to evidence commit or the rest of the workflow with `&&`, or let fail-fast shell behavior discard that work. Commit code before checking so clean-worktree evidence records commit provenance. Fix missing, unconfigured, failed, or stale required mechanical work. A pending manual criterion must not prevent the other checks or their state commit.
3. Grade any required ungraded or stale rubric separately: emit its packet, inspect the implementation against it, write the graded output, and record it through `tiller eval --record-rubric-output`. Commit actual recording deltas. Default eval is not a grading loop and need not precede check. Required rubric grades are evidence for human attestation, never the acceptance decision. Optional criteria do not become blockers; an unreadable or invalid contract is uncertainty to repair, not a human-only wait.
4. When only human acceptance remains, push the implementation and evidence, then create or reuse the matching **draft** PR for the resolved feature or epic. Verify the actual repository, base, head, open/draft state, and that the published tip includes these commits. Put the verified URL and tip evidence in **Notes from this run**. Do not call formal submit yet, hand-edit PR fields, or describe the draft as a Tiller submission. If no human acceptance remains, proceed through the same host identity/publication verification to step 6.
5. Stop for the exact human action: name each required criterion and the feature carrying it, the review evidence, and `tiller attest feature <feature> --criterion <criterion> --status pass` if accepted, or `--status fail` if rejected. For a required attestation-backed gate use `--gate <gate>` instead. Only the human runs those commands, even if they ask you to type them. At feature or epic grain the human runs the attest on the recorded aggregate branch, in the worktree that holds it, and commits it there before submit; tiller refuses it elsewhere, so name that branch in the handoff. At epic grain name the reviewed epic and the particular feature(s) whose acceptance they must decide.
6. After the human records the decision, read back acceptance, commit its actual state delta, and complete required rechecks on the same unmerged review branch. Inspect failures and commit evidence as in step 2. Requested changes resume affected in-review tasks or shelf nodes on their own branches, repair the work, rerun checks, and commit state there. At coarse grain, resubmit affected tasks, merge their new tips into the aggregate, and rerun affected feature gates and invalidated rubrics. Reuse the established review. Once every applicable required gate passes, push updated evidence, make the same PR ready if still draft, and verify identity/state/tip again. Run `tiller submit <noun> <node> --pr <actual-number>` for the resolved review. Do not submit epic-riding features separately.
7. If submit wrote a state delta, immediately commit it and push it to that same review branch. An unchanged successful submit needs no empty commit or redundant push; verify the already-published state instead.
8. Read back `tiller status <noun> <node>` and verify the remote branch contains the submission-state commit. Report the actual PR and factual warnings in run notes, render the execute readout, and stop for human review and final integration merge.

Commit the state Tiller wrote even if a later step refuses. The review ref is captured before submit writes its own delta, so it cannot contain that delta: direct the reviewer to the branch's current state, not just that earlier commit. If PR creation succeeds but submit refuses, name the open PR and the still-unsubmitted Tiller state separately; reuse that PR after recovery. If submit succeeds but the state push fails, retain `in_review` and name the unpublished commit. Do not roll state back, claim a completed hand-off, or create a second PR.

Commit only state a command actually changed. A successful feature or epic submit of the same PR can report `changed: false` and write nothing, including after requested changes whose riding task refs and gates were already committed and pushed. That is a completed record operation, not a publishing failure. Skip the state commit and extra push in that case, never manufacture an empty commit, and still verify the existing published tip. The same actual-delta rule applies to evaluation and other no-op commands.

Recheck only what current work or recorded failures require, not merely because an attestation state commit changed HEAD. A required attestation-backed gate is different from an attested criterion: `check` currently resets that gate to pending. Complete needed mechanical rechecks before asking for that gate's human attestation, and do not erase a fresh gate acceptance with another blanket check. If new work requires rechecking afterward, record the new evidence and ask the human to attest the gate again before submission. Never restore it yourself or bypass check's guards through standalone eval.

With no configured remote, omit host operations and pushes. Before a human-acceptance handoff, prepare a committed local branch-and-tip review surface with the implementation and evidence, name it in run notes, and stop for the same exact human decision; do not submit a coarse review before acceptance. Once acceptance passes, submit without `--pr` to record the permitted branch-and-commit review ref, commit the submission state locally, and read it back. Do not require a host account, create a remote, or use offline mode to disguise failed remote-backed publication. Under a coarse grain a riding task also submits without `--pr`, even with a remote: commit its review-ref state and continue the approved branch loop. That ref does not prove a feature or epic PR exists, and a missing formal submission does not prove no draft exists.

Every commit on the reviewed task or aggregate branch must be included in the human's final integration merge. Never commit to that branch after its final merge; intermediate task-to-aggregate assembly does not close a task's requested-changes path. Final squash merges can be observed by net-change matching, so preserve local refs until sync. Prefer a final merge commit (`--no-ff`) or squash over an unobservable fast-forward. If a fast-forward already landed, hand the human `tiller sync --accept-fast-forward <reviewed-node>`, naming the task at task grain, feature/epic at coarse grain, or shelf node. Never run the assertion yourself. Inspect sync's actual crossings and warnings, even on exit `0`.

### Feature acceptance and merge closeout

Acceptance lives on features. Use `tiller check feature <feature>` for the guarded mechanical workflow, not redundant default eval followed by check. Standalone `tiller eval` does not apply check's basis, question, or parent guards; it is not a refusal bypass. Rubric emission, grading, and recording are separate operations. Criterion attestation does not enforce every intended rubric prerequisite, so prepare the grade before asking the human rather than claiming the CLI guarantees that ordering. Task submit relies on the task lifecycle state; unlike coarse submit it does not explicitly revalidate the current required-gate set. Keep task checks current as protocol, not as a claimed submit guard. There is no epic contract: `tiller eval epic`, `tiller check epic`, and `tiller attest epic` are misuse.

Inspect targeted test bindings when executing the authored contract. A ref-only binding appends the selector to the configured tests command; an aggregate CI wrapper may ignore it and repeat the whole suite. Preserve the full configured CI gate, use the approved targeted `binding.run` for the criterion, and verify the selected scope and necessary environment/preparation. If an accepted binding is wrong, report the evidence and follow human accepted-basis review for its correction, never quietly rewrite the approved contract.

Under coarse review, each checked task submits its riding ref on its own branch, commits state, and is merged into the aggregate before continuing eligible approved work without another go. At feature grain, every live task being `in_review` or `done` reaches feature acceptance; waiting for the riders to land in integration first would deadlock their review. At epic grain this condition holds for every riding feature, with independent overrides already done. Assemble all current task tips, check each feature on the aggregate, grade required rubrics, then follow draft preparation, human acceptance, and formal submission above. Five green task refs are not feature acceptance or a feature submission.

When the developer reports a merge, preserve local review refs until ordinary `tiller sync` observes it. Safely update and check out the configured integration branch, preserving unrelated changes; when it is already checked out in another worktree, usually the primary checkout, work there only under the worktree closeout rule above. Never commit to the merged review branch. Inspect sync's returned nodes, remaining in-review work, policy violations, and touched-doc notices even on exit `0`, commit each actual sync delta, and read back the related feature and tasks. A node sync lists under "could not be recorded" with a surface remedy had its recorded surface moved or removed: link the code's current location, run the named `tiller unlink ... --surface` command, commit that state on integration, and sync again. If this was not the last live task, close out only the reported merge; do not start unrelated work.

After the last task-grain task is done, finish feature acceptance on the integration branch: check all applicable feature gates, commit actual evidence even on exit `1`, grade/record required rubrics, and fix mechanical failures. When only human acceptance remains, publish the state to the integration branch (or name the committed local tip offline) before handing over the exact criterion decisions. This is state-only closeout: no new PR and no coarse submit. After the human's decision, commit the attestation delta, recheck and commit evidence, run ordinary `tiller sync`, commit its actual state delta, push the integration branch when remote-backed, and verify the remote tip and node state before the final readout. A clean rollup needs that final sync, not a stop after check.

Sync is advisory after a human merge and may already have recorded the feature `done` with violations. Do not resume/start terminal work or claim its done state is blocked. Complete applicable acceptance reporting and publication without erasing historical violations; a later pass does not retroactively make the merge clean. A fast-forward assertion remains the human's command, never the agent's.

### Missing aggregate review after merge

An aggregate review may have merged before `submit feature` or `submit epic` recorded it. `review_missing` means only that the review is absent: sync has not inspected its merge state. If unmerged, use ordinary assembly, checks, acceptance, and submission. If already merged, collect historical evidence and obtain explicit authorization to recover the named review. A merge report or general execute instruction does not authorize inventing its PR, historical head, or result.

For remote-backed recovery, verify the host repository, actual PR number, merged state, configured integration base, head repository/owner, historical branch, head OID, and result commit through the project's existing host tools. Tiller has no host client: the PR association is caller-supplied, while the binary checks local Git evidence. Fetch or restore missing objects through ordinary Git without resetting refs. No remote means no host account or PR requirement.

On the configured integration checkout, commit relevant manifest/work/prose inputs, preserving unrelated changes. Run the authorized record operation:

```sh
tiller submit feature|epic <node> --recover-merged \
  --head <historical-reviewed-commit> --merge-commit <integration-result-commit> \
  --reason "<why the historical review was missing>" [--pr <actual-number>]
```

The arguments name immutable historical commits, not today's aggregate tip. The result is the actual merge or squash commit, or the contained reviewed head in an unwitnessed fast-forward case. The operation records provenance and one recovery event only; it does not run gates, move lifecycle, approve criteria, or erase current drift/questions/acceptance.

Commit the actual recovery delta on integration, run ordinary `tiller sync`, inspect crossings and remaining riders, current acceptance, historical policy warnings, and touched-doc notices, then commit actual sync state. Publish the integration state when authorized and remote-backed, verify the published tip, and read back status/show/queue/readout. Identical retries write nothing, so skip empty commits. Historical pending acceptance remains historical even when the current approved contract is green. For contained/unwitnessed recovery hand the human `tiller sync --accept-fast-forward <reviewed-node>`; never run the assertion yourself. Never send work back to a finally merged branch, create an extra PR for the recovery, manufacture acceptance, or reset refs.

Review grain changes only in the safe configure window. Finish and sync submitted work before changing a review grain that would affect it. An affected feature PR or review ref and an affected task in `in_progress`, `check`, or `in_review` lock the change. Entering or leaving epic grain must happen before an affected feature records its aggregate branch. Existing task PRs must merge and sync before a later coarse submit. Never hand-edit state to evade the refusal or reinterpret historical task branches as an aggregate.

A feature overridden to task or feature grain finishes its own PR and reaches `done` before the epic PR opens. Coarse submit still enforces active basis, questions, reviewed assumptions, required gates, task readiness, review identity, and retired ancestors. Green gates do not bypass those refusals. To record a feature-only assumption on explicit human instruction use `tiller approve feature <ref> --assume-question <Q-id> --reason <text>`; this is record-only, does not move lifecycle, and does not grant an assumption to the task. Task start must satisfy its own holds.

Follow the recovery protocol before escalating a refusal. A genuine missing credential, unavailable host tool, protected-branch refusal, unresolved Git conflict, or gate needing human judgment can require a stop when it cannot be resolved safely. Name the node, exact failed step, what you tried, and the remaining local and published state. Ordinary publishing is not itself a human-only gate. Do not bypass required gates or developer no-publish limits. A mid-run stop leaves work on the current branch until the grain finishes or the blocker is resolved; it never makes a locked grain change safe.

Before `tiller check` can run the build and tests gates, their commands have to exist. The manifest, `.tiller/tiller.yml`, ships those two gates as commented templates, so set their `run` commands for the project's chosen stack before you expect them to pass. That choice belongs to the build work, not to init: at scaffold time there is no stack or code yet, so the commands cannot be known. The natural moment is the foundation slice, the first feature that stands up the project, where wiring the build and tests gates is one of its tasks. The evals gate is already active at feature grain and needs no such wiring.

The loop is re-enterable. When a PR review requests changes, run `tiller resume task <id> --reason "<what changed>"` to move the task from `in_review` back to `in_progress` and pick the loop up where you left off. The recorded PR stays, and prior gate results may now be `stale`, so re-run `tiller check` before resubmitting. Reuse the existing open PR and complete both pushes and the submission-state readback; an old PR number on resumed or checked work does not mean the new changes were submitted. At a coarser grain, continue the branch loop until the feature or epic review surface is ready, then update that same review. Work already in `check` skips `resume` as well as `start`.

Claim only the task, bug, chore, or tweak you will execute. `tiller claim task <key>` takes an unclaimed task; another holder requires `--from <id>` restating the actual holder and an explicit human handoff instruction. Before start, `tiller release task <key>` returns your reservation to the pool; `tiller reassign task <key> --to <contributor> --reason <text>` is a human-directed handoff. The same commands support `bug`, `chore`, and `tweak`. First successful start permanently fixes ownership. Do not release completed work or change ownership on resume, checks, review, or cancellation; even cancelled-before-start ownership is terminal history. Creation and approval never assign tasks. Features and epics have no exclusive owner, and claims never cascade. Judgment verbs remain available to registered contributors under their existing actor and human-gate rules. There is no `tiller add evidence`; evidence is recorded by `tiller check` and `tiller eval`.

### Exceptional human-directed transfer

Before start use ordinary reassignment. For started unfinished work, require an explicit human instruction naming the work and recipient before running `tiller transfer task|bug|chore|tweak <ref> --to <registered-contributor> --reason <text>`. Never infer this authority from a general execute instruction or another developer's availability. Same-owner and terminal transfers refuse. If the old review already merged, sync first and use forward work.

For a recorded review, `transfer_review_unobservable` means neither its local branch nor a usable submitted head can establish merge state. Restore/fetch the review branch through ordinary Git, sync, and inspect before deciding again; never treat unavailable history as unmerged. Observed merges or squash-matched submitted history refuse as `transfer_review_merged`. Follow the response's exact `start_command` and `follow_through` list rather than reconstructing recipient or host instructions.

Transfer preserves approved scope: current prose and applicable parent contract must still match accepted bases. A scope change belongs in existing human review/amendment workflow, not a transfer flag or invented amendment operation. The transaction cancels the original but retains its ID/key, owner, branch, review, evidence, proposer, and prose; it creates a new same-noun ready replacement with a distinct ID/key, recipient, new accepted basis path, and reciprocal lineage. The replacement is not started and inherits no branch, PR/review ref, green checks, attestations, evidence, or completion basis.

Read the exact returned replacement reference and continuation facts. Preserve prerequisites and live order; unstarted dependents redirect, terminal dependents keep history, and an unfinished started dependent refuses the entire transaction. Direct question holds, ancestor holds, manual pauses, shelf attachments, and carrier relationships remain operative. An unsafe relationship refuses before writes; report the required human-reviewed structural change without inventing a command. Transfer approval grants no assumption or reviewed answer.

The recipient starts the replacement with `tiller start <noun> <replacement-ref>` on its own distinct branch, reusing existing implementation through ordinary Git without rewriting attribution. For task/bug/chore/tweak PRs close the old PR without merging and open a new one. For feature/epic grain retain the unmerged aggregate PR, assemble replacement work, and rerun affected checks, rubrics, and human acceptance. Old commits remain in the aggregate; cancelling a record does not remove them or revoke host approvals. The incomplete replacement prevents successful aggregate submission. Tiller touches no Git refs or host APIs: complete and report that external work separately, and never describe an external failure as a rolled-back state transaction.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`. An exit `1` from check is a business result to inspect, not permission to skip evidence commits or proof that the only work left is human. Separate machine/configuration failures, ungraded rubrics, human acceptance, and guards; continue all safely executable related work and stop only at the actual decision or blocker. A refusal naming the accepted basis means someone edited a spec or contract out from under approved work; `tiller status <noun> <node>` names the file and `--refresh-basis` remedy, and reviewing that change before re-pinning is the human's call.

For shelf work, a red `check` leaves the bug, chore, or tweak in its owned loop: fix the work and rerun the gate. Requested changes re-enter through `resume bug|chore|tweak`. A live carrier leaves no shelf work to recover here; after `carrier_cancelled`, follow the node's own status and branch instead of the cancelled carrier.

## Choreography

The remote examples below share one host step. Set `remote`, `base`, and `branch` to the project's configured remote, integration branch, and recorded review branch, and `host_repo` to its actual host repository. Set `worktree` to a new path outside the repository for this unit of work, and `start_base` to the updated integration branch tip or, for a coarse dependent task, the aggregate holding its submitted prerequisites. Do not execute placeholder values. Before making commits on re-entry, inspect any recorded PR and discover matching open drafts even without a formal Tiller record. On GitHub use existing `gh` tooling:

```
gh pr list --repo "$host_repo" --state open --base "$base" --head "$branch" --json number,url,isDraft,baseRefName,headRefName,headRefOid,headRepository,headRepositoryOwner
# Reuse the exact matching open PR. Only if none exists, create one.
# At a coarse human-acceptance boundary include --draft; omit it for a ready task/shelf review.
gh pr create --repo "$host_repo" --base "$base" --head "$branch" --draft --title "$title" --body "$body"
# Set pr to the actual number; inspect repository identity as well as branch names:
gh pr view "$pr" --repo "$host_repo" --json number,url,state,isDraft,baseRefName,headRefName,headRefOid,headRepository,headRepositoryOwner
```

Set `title` to the resolved node's full key path and id, and `body` to its review scope and verification, naming the parent feature for a task, riding tasks for a feature, and riding features and tasks for an epic. The body also names every departure from the reviewed tasks' `task.md` plans, gathered from the riding tasks' implementation commit messages when the PR is a feature or epic review, and an offline review surface names them the same way. For the task example, the title is `form-engine/certification-form-loading/schema-transcription (T-k3p9w6)` and the body names `form-engine/certification-form-loading (F-79dh3q)`. A shelf PR names its own shelf reference and any attached context, never substitutes the attached feature as the reviewed node. On another host, use that project's available tool to perform the same lookup/create/inspect operations and obtain the actual number. `gh` is an example, not a Tiller runtime requirement.

Compare the host's head repository/owner to the configured push destination, not only the head branch name; a fork can share that name. The selected `--repo` and returned URL identify the base repository. Verify the head OID contains the intended pushed tip. If several candidates are ambiguous, stop rather than choose an arbitrary number. An existing non-draft PR while acceptance is outstanding is not evidence of acceptance: report that actual state, do not silently alter host review state or submit early. After accepted decisions and required rechecks, `gh pr ready "$pr" --repo "$host_repo"` makes the same draft ready; inspect it again before formal submit.

For each remote-backed finish below, verify the second push by comparing `git rev-parse HEAD` with `git ls-remote "$remote" "refs/heads/$branch"` immediately afterward. Matching tips prove this state commit is published. If the remote advanced, fetch that branch and verify the submission commit is an ancestor of its tip; do not overwrite someone else's work to make the tips match.

Every check below is a separately inspected command, not a fail-fast script. The following commit runs for an actual state delta even on exit `1`; skip it when unchanged. Stage only this run's reviewed `.tiller/` paths. Do not advance to submit with outstanding required work. Code blocks show the selected successful path, not permission to run through a human stop or refusal.

Shelf work has its own review surface regardless of the configured review grain. This example uses a bug. For a chore or tweak, substitute its noun for `bug`; its canonical branch is `tiller/chores/<key>` or `tiller/tweaks/<key>`.

Inspect the shelf node and its reading context before choosing exactly one entry sequence:

```
tiller show bug bugs/blank-on-reload                       # read bug.md, attached_to, source, and current status
# read the attached feature/task prose named above when attached_to is set
```

For a shelf node in `ready`, add its worktree, join there, then claim it, start it, and create the newly recorded branch:

```
git worktree add --detach "$worktree" "$base"              # updated base; unless the developer's instructions say otherwise
cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim bug bugs/blank-on-reload                      # shelf-grain claim
tiller start bug bugs/blank-on-reload                      # ready -> in_progress, records the shelf branch
git checkout -b tiller/bugs/blank-on-reload                # cut from the integration branch
```

For an already `in_progress` or `check` shelf node, skip `start` and `resume` and check out the recorded branch without `-b`. At `check`, inspect status and run only the checks or submission still needed.

```
git checkout tiller/bugs/blank-on-reload                   # in this node's worktree; else git worktree add "$worktree" <branch>, then join
```

Run `resume bug|chore|tweak` only from `in_review` after requested changes. Check out its existing branch in its own worktree first (adding and joining one as above), then resume there:

```
git checkout tiller/bugs/blank-on-reload                   # resume never creates a new branch
tiller resume bug bugs/blank-on-reload --reason "address review: tighten reload state"
```

After requested changes, fix the work, rerun the gates, and resubmit. Every entry path joins this shelf finish; on a checked re-entry with current green gates and committed implementation/gate state, continue at the first push:

```
# fix the defect and add the regression test; stage only this work
git add <changed-code-and-test-paths>
git commit -m "B-7k2p9x fix reload state"
tiller check bug bugs/blank-on-reload                      # build and tests; no evals gate
git add .tiller && git commit -m "B-7k2p9x record gate evidence"
# remote-backed finish: branch=tiller/bugs/blank-on-reload
git push -u "$remote" "$branch"
# perform the host step above for this bug; pr is its actual PR number
tiller submit bug bugs/blank-on-reload --pr "$pr"
git add .tiller && git commit -m "B-7k2p9x in review"
git push "$remote" "$branch"                               # publish submission state
tiller status bug bugs/blank-on-reload
git rev-parse HEAD
git ls-remote "$remote" "refs/heads/$branch"                # verify the state commit reached the remote
tiller readout execute                                    # reproduce verbatim, then stop
```

Without a remote, replace the shelf's remote-backed finish with `tiller submit bug bugs/blank-on-reload` without `--pr`, commit the state, read back status, and render the readout. Omit both pushes and host operations, not the state commit.

Choose exactly one task entry path after reading the task and its feature:

```
# Unless the developer's instructions say otherwise, work in the unit's worktree: the task's own
# at task grain, the feature's or epic's at coarse grain. Start base:
# Independent task: the configured integration branch.
# Coarse dependent task: the aggregate containing submitted prerequisites and their state.
# A later task in a coarse unit whose worktree you hold: git switch --detach "$start_base" there, no new join.
git worktree add --detach "$worktree" "$start_base"
cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim task form-engine/certification-form-loading/schema-transcription  # ready and unclaimed only
# First successful task start activates a ready parent and initializes any aggregate.
tiller start task T-k3p9w6 --json                           # ready only; read branch and base_branch
# Set task_branch and task_base to those actual returned values, not placeholders.
git checkout -b "$task_branch" "$task_base"                 # task state follows onto its own branch
# Commit and share task-specific claim/start state on the new task branch.
# in_progress or check: skip both start and resume, check out this task's existing branch
#   in the unit's worktree, or git worktree add "$worktree" <recorded-branch>, then join
# requested changes from in_review only, inside that same worktree:
# git checkout <recorded-branch>
# tiller resume task T-k3p9w6 --reason "address review: tighten validation"
# ... implement from task.md, following project and harness craft guidance ...
git add <changed-code-and-test-paths>
git commit -m "T-k3p9w6 implement schema transcription"
tiller check task T-k3p9w6                                  # required build and tests; green -> check
git add .tiller && git commit -m "T-k3p9w6 record gate evidence"
```

For a checked task whose implementation and current green gate state are already committed, skip that entry/build sequence and finish submission. At task grain with a remote:

```
git push -u "$remote" "$branch"
# perform the GitHub or other-host step above; reuse the task's PR on requested changes
tiller submit task T-k3p9w6 --pr "$pr"                      # actual task PR -> in_review
git add .tiller && git commit -m "T-k3p9w6 in review"
git push "$remote" "$branch"                               # submission state is part of this PR
tiller status task T-k3p9w6
git rev-parse HEAD
git ls-remote "$remote" "refs/heads/$branch"
tiller readout execute                                    # after readback and publication verification, then stop
```

At task grain without a remote, or for a task riding a coarse review even with a remote:

```
tiller submit task T-k3p9w6                                # records the permitted review ref, no task PR
git add .tiller && git commit -m "T-k3p9w6 record review ref"
tiller status task T-k3p9w6
# offline task grain: render tiller readout execute and stop for the human review
# coarse grain: checkout recorded aggregate; git merge <recorded-task-branch>
# Include submission state, then continue eligible work on its own task branch.
```
The human performs the final merge into integration with a merge commit or squash, preserves local refs until sync, and never adds commits to a branch after its final merge. Intermediate assembly never uses squash. If a task-grain fast-forward already happened, hand them `tiller sync --accept-fast-forward form-engine/certification-form-loading/schema-transcription`. Sync's later `.tiller/`-only delta belongs directly on the integration branch, without a new PR.

At task grain, a reported last-task merge enters integration-branch closeout. Resolve and safely update `base` before this sequence; preserve the local review ref until sync observes it:

```
git checkout "$base"                                       # where base is checked out, usually the primary checkout
tiller sync                                                # inspect actual crossings and violations
# Commit this run's actual sync delta on base before checking.
tiller status feature F-79dh3q
# Once every live task is done:
tiller check feature F-79dh3q                              # includes contract; inspect exit 1 too
# Commit actual evidence; resolve machine work and grade required rubrics below.
# If only acceptance remains: push base, verify its tip, name exact HUMAN decisions, readout and STOP.
# On re-entry after human acceptance: commit its delta; recheck only if needed.
# Required attestation-backed gates must be attested AFTER the last needed check.
# Inspect results and commit actual evidence; continue only with acceptance passed.
tiller sync                                                # ordinary clean rollup, never --accept-fast-forward
# Commit actual sync delta directly on base; no new state-only PR.
git push "$remote" "$base"                                 # omit only in a genuinely offline project
tiller status feature F-79dh3q
git rev-parse HEAD
git ls-remote "$remote" "refs/heads/$base"                   # verify state publication
tiller readout execute
```

At feature grain, the finish line is every live task `in_review` or `done`, before the final integration merge. Use the recorded aggregate, not integration:

```
# Checkout the recorded feature aggregate and ordinary-merge every current riding task tip.
# Repeat assembly and affected checks after requested changes before reusing this PR.
tiller check feature F-79dh3q                              # contract plus other feature gates
# Inspect every result; commit actual evidence even on exit 1.
# Resolve machine failures and grade/record required rubrics before a human-only handoff.
git push -u "$remote" "$branch"
# Discover/reuse or create the matching draft using the host step; verify its tip and URL.
# If acceptance remains, name the exact human commands and this aggregate branch (the human attests
# there, in its worktree, and commits there before submit), render readout and STOP.
# On re-entry: commit human records; recheck only if needed, then commit evidence and push.
# Every applicable required feature gate must now be recorded pass.
# Reuse the same PR; make it ready only if still draft, then verify it again.
tiller submit feature F-79dh3q --pr "$pr" --json
# Only when data.changed is true: commit the actual review-state delta, then:
git push "$remote" "$branch"
# When data.changed is false, skip that commit and redundant push.
tiller status feature F-79dh3q
git rev-parse HEAD
git ls-remote "$remote" "refs/heads/$branch"
tiller readout execute                                    # final formal-review handoff after readback
```

Without a remote, substitute the committed local branch/tip surface before acceptance, then omit `--pr` on formal submit after acceptance passes; retain actual state commits and readback. Epic grain applies this same finish to every feature riding the epic branch after independent overrides are `done`: check each feature, prepare/grade required rubrics, publish one matching epic draft, name each feature's exact human acceptance action, and reuse that epic PR after rechecks for `tiller submit epic <epic-key> --pr "$pr"`. Do not submit riding features separately or run epic eval/check. Commit and publish actual epic submission state and read back the epic. Offline epic review retains the same acceptance boundary and records its local review ref afterward.

For each required ungraded or stale eval-kind criterion, use the authored contract's actual id:

```
tiller eval feature F-79dh3q --emit-rubrics
# Grade each packet against the implementation and write its reported output_path.
tiller eval feature F-79dh3q --criterion C4 \
  --record-rubric-output artifacts/tiller/evals/F-79dh3q-C4.json
# Commit actual rubric-recording state. This is evidence, not human acceptance.
```

The graded rubric output file you write for each `eval`-kind criterion has a fixed shape. `--emit-rubrics` tells you the `output_path` to write each one to; write that path a single JSON document shaped `tiller.rubric_output.v1`:

```json
{
  "schema": "tiller.rubric_output.v1",
  "node": "F-79dh3q",
  "criterion": "C4",
  "verdict": "pass",
  "rationale": "one or two sentences on why",
  "graded_by": "your model",
  "graded_at": "2026-06-27T00:00:00Z"
}
```

`schema`, `criterion`, and `verdict` (`pass` or `fail`) are required; `node` is optional but must match if included; `rationale`, `graded_by`, and `graded_at` are optional. `--record-rubric-output` validates the document and exits `0` on a successful record. A required `pass` grade becomes `needs_attestation`; the human decides with `tiller attest feature <F-id> --criterion <C-id> --status pass|fail`, substituting their actual decision. A successful record is not a passing feature check: pending required acceptance remains non-passing on `tiller check`. Neither record exit `0` nor draft publication proves acceptance.

## Readout

End by running `tiller readout execute` and reproducing its block verbatim as the last thing in your
reply to the developer. The block is Tiller's, not yours: it renders the same whoever is driving, which
is what lets a developer learn to read it once instead of learning your habits. Do not restate it in
prose around it. The block closes the reply: nothing follows it. It ends on its **Next:** ranked list,
the one place the developer looks to learn what happens next, so keep reproducing the list as written
and never add a list of your own after it. `tiller submit` and a passing `tiller check` print an
**Up next:** list when they finish; that list is for you mid-run, and the closing readout's list is the
one you reproduce.

Before the block, add a short **Notes from this run** list for what it cannot know: what you changed
outside `.tiller/`, file by file where that helps a reviewer, what you deliberately left out of this
task's diff, where the work departed from the task's own `task.md` plan and why, every question the run
recorded and every open question still holding the run's work (each with its id and title, the node it
holds, what the answer would change, and the branch and worktree that carry it, where the human answers
it), and any caveat about the
branch or the merge basis the human should carry into the merge (the ordering rules above name the shapes
that can silently strand a node, and the block cannot see which one you are walking into). Keep it to
facts about this run. In the notes and in everything else you write to the developer, name every epic,
feature, and task the way the block itself does, key path with the id in parentheses, never a bare id:
an id alone sends the developer off to look up what it points at. Commentary is yours to add freely,
with the notes ahead of the block, never after it; a question for the human is a recorded question,
never a line of chat.

Always include the observed review URL, host open/draft state, repository/base/head, and publication tip evidence when handing over a remote review. Before acceptance, explicitly say formal submission is not yet recorded and name each exact human decision. For offline work name the local branch and committed tip. For a failed/no-publish run say which local commits exist, what is and is not published, the exact blocked step, and the remaining acceptance or submission work. Tiller's pure readout cannot verify these external facts.

## Hand-off

Out to craft for the actual building. The agent handles task branches, aggregate assembly, mechanical conflicts, gates, and maintenance of the same review PR. Up to the human at the actual decision: semantic conflicts, the questions the run recorded, required acceptance after verified draft/offline review preparation, or final integration merge after submission and state publication/readback. The first is an acceptance handoff, not a completed submission. Task-grain post-merge closeout finishes applicable checks, ordinary sync, and integration-branch state publication/readback without a new PR. Green task gates, a riding ref, or an unpublished state commit never proves the resolved review's completion. Report real blockers or explicit stop limits as incomplete; no agent attests, asserts a fast-forward, or performs the final integration merge.
