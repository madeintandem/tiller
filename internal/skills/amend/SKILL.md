---
name: amend
description: Use when the developer names a change large enough to reshape the plan rather than extend it, when tiller drift reports a cascade too big to mend one marker at a time, or when the code has shifted under planned but unbuilt work. Sorts corrections from casualties, stages the reshape as one amendment, and stops for the human's approve.
---

# amend

## Trigger

The developer judges a change large enough to reshape the plan rather than extend it. Or `tiller drift` reports a cascade out of one upstream change, a renamed column that stale-specs a dozen done features, too large to mend marker by marker. Or the code has shifted under planned but unbuilt work in ways no marker can see, and the developer knows roughly what moved and roughly which part of the tree it touches. This is the deliberate counterpart to `decompose`: decompose builds the tree from inputs, and amend reshapes a tree that already exists with work hanging off it.

Nothing in recorded state says a reshape is due, so this seam is never a move the ranked list computes. It starts from the developer's judgment, or from an actual `tiller drift` run, and the picture you reason from is one you go and read rather than one handed to you.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

**The blast radius.** Read it first. Frame the change as brownfield against the team's own recent code, and work out which features, contracts, and epics it actually touches before you decide anything. Recorded state makes that a query rather than a memory exercise. `tiller show feature <key> --json` names the feature's `surface`, the existing code it touches, and the revisions its plan was accepted against; a done feature carries `verified_against` with the surfaces its completion recorded. Each node's own commit anchor bounds the reading: diff that node's surface from its anchor to HEAD and judge what changed there, instead of reading the whole codebase against the whole tree. When `tiller drift` is what brought you here, its candidates are input too: read each one's `files` and `since` and note which nodes the cascade names.

Use `tiller query --from path:<repo-relative-path> --follow covered_by --json` to discover recorded exact and enclosing-scope claims, then inspect each source: work surface, baseline description, and acceptance binding mean different things and none proves implementation coverage. `--from <work-id> --follow depended_on_by,amended_by,mentioned_by --json` finds recorded neighbors and document citations; raise depth deliberately, since follow names are a union at every hop rather than ordered stages. Read diagnostics and `complete` before treating the radius as exhaustive. Generated mentions are labeled, unknown references stay unresolved, and proposed amendment effects are not applied lineage. Historical key reuse must never connect an old operation to today's holder. Current-file navigation does not recover a pinned historical revision; use the recorded evidence and report unavailable history. Querying is read-only and changes no lifecycle gate.

**Structural feasibility before placement.** Load the installed `tiller guide amend` and inspect `tiller rename --help` and `tiller amend --help` before promising a move, naming replacement work, or negotiating detailed placement. Read `tiller show <noun> <ref> --json`, `tiller status <noun> <ref> --json`, and `tiller query` for the affected feature, every descendant task, and both current and proposed parents. Inspect lifecycle and prior starts, recorded branches and reviews, accepted bases, dependencies, effective direct/inherited question holds, and inherited review grain. Use read-only Git inspection where history needs clarification. Current ready status alone does not prove that work never started. Do not probe with a mutation or invent a `--dry-run` flag.

Before product questions, state separately the recommended outcome, what this installed version can apply, and the smallest safe alternative. Resolve repository facts yourself; ask only about unresolved product tradeoffs. The correction rules in `.tiller/orientation.md` distinguish direct proposal rename, reviewed unstarted feature moves, and genuine scope replacement. If the installed help lacks the reviewed move operation, disclose that accepted unstarted relocation is unsupported in that version rather than promising it. Do not install or upgrade without direction.

**Placement and scope.** Recommend the affected delivery progression from documented actors, goals, and outcomes before choosing containers. Extend an existing epic or feature when the work serves the same bounded outcome; propose a new increment for a distinct outcome or delivery phase, even when it touches the same subsystem. A shared subsystem name and a parent's completion status are neither sufficient nor disqualifying placement criteria. Bound shared implementation to its consuming increment; retain known later needs in their later features instead of prebuilding them here. Keep genuine technical prerequisites, including bounded migration or compatibility work justified by concrete risk.

Reshape only the requested change and its affected unfinished scope. Explain wider consequences, preserve unrelated approved and completed work, and ask for developer direction before a broader replan. A planning request authorizes proposal work, not approval or arbitrary reordering of the accepted tree. When consequential product information is missing, ask a focused question rather than inventing an outcome or falling back to subsystem buckets.

**Ownership-only continuation is not a scope amendment.** Before start, use human-directed `tiller reassign task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`. For started unfinished work, an explicit human instruction naming work and recipient may authorize `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`. It preserves approved scope and original history while creating an approved, unstarted replacement with fresh identity and no inherited review/evidence. A scope change still uses the existing review/amendment paths. A started unfinished dependent or unsafe question/carrier relationship refuses before writes; explain the required human-reviewed structural change, never invent a shelf amendment operation or a transfer override. Follow execute for recipient start, old-PR closure or coarse aggregate update, fresh checks, and affected human acceptance. The original remains historical, not relabeled.

**Corrections and casualties, and the lightweight exit first.** A planned node still right in scope and intent but wrong in detail can take a same-scope prose correction while non-terminal. Edit `feature.md`, `feature.eval.md`, or `task.md`, which trips `active_basis` on accepted work; the human clears it with `tiller approve feature <key> --refresh-basis` or `tiller approve task <id> --refresh-basis` after review. A wrong factual claim in the `task.md` of a task already being executed is an execute departure, not a prose correction: leave that `task.md` unedited and let execute record the departure. Proposal corrections and ready plans whose approval should replay use their existing correction routes in `.tiller/orientation.md`. One more task serving an existing feature can use `tiller add task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>"`, write its proposed prose, and stop for `tiller approve task <id>`. A human-directed preferred sequence uses `tiller order`, not an amendment or a hard edge. None of these lightweight changes needs a structural batch. Say which exit you are taking, and why, before opening anything; never use a prose correction to rewrite terminal history.

**The casualties.** When scope is genuinely abandoned or replaced, choose each amendment operation honestly. Unchanged relocation uses the reviewed move route below, not a casualty operation.

Done work being replaced is superseded, never rewritten. `tiller amend <A-id> --supersede <target> --by <replacement>` records that lineage because the merges beneath a done node do not reverse, and its spec is the truthful record of what shipped. Staging may happen before the target reaches done, but approval requires done intrinsically. The forward replacement rides a new `replace` or `migrate` feature instead.

Same-outcome continuation is different: a draft feature may be added under an existing done epic, or a proposed task under an existing done feature, without a reopen or supersede operation on that parent. Adding the child does not immediately change the stored parent status. `tiller sync` recomputes rollups, returning a done feature with unfinished tasks to active and a done epic with unfinished features to ready, clearing current completion/review fields and staling feature gates as appropriate. A completed coarse review must reopen before a new execution round starts. Completed tasks remain complete, and terminal feature/task prose is not rewritten to claim the new work already shipped. Cancelled or archived ancestors are not executable continuation targets. Normal specification and approval still apply to new work.

Not-yet-done work the change makes pointless is cancelled with a reason, `--cancel <target> --reason "<why>"`, and with `--superseded-by <replacement>` when something takes its place, so the tree answers what replaced it rather than leaving that in the conversation. What separates the two operations is the state the target is in, not the size of the change: supersede is for work whose result shipped, and cancel with a supersession is for work replaced before it ever finished.

A spec or contract on non-terminal work that the change re-scopes rather than retires is revised, `--revise <target>`. You write the edit in the working tree, the same prose edit a correction would be; the operation records that the human reviewed it as part of this reshape, and the apply re-pins that node's accepted basis from what you edited, so the batch never lands leaving an `active_basis` refusal standing on work the human just approved. Eval-gate results already recorded go `stale` exactly as a refreshed basis stales them. A revise cannot re-pin done or cancelled work; changes after a terminal result are new forward work.

A done feature's eval bindings can be corrected in place, without superseding it, when only how its criteria are checked changed: a test moved or split across files, a command renamed, a baseline artifact relocated. Edit the bindings in `feature.eval.md`, then stage `--correct-eval <feature> --reason "<why>"`, adding `--retire <criterion-id>` for each criterion you remove on purpose. Staging pins the `eval_revision` of the file as you edited it. The human's approval re-pins the feature's `verified_against.eval`, so the eval leg of `stale_spec` clears, and records the old and new eval blobs, the reason, and the retired ids on a `correct-eval.feature` event. Approval refuses unless the target is a done feature whose corrected contract changes only bindings and baseline artifacts; a changed statement, polarity, required flag, or baseline surface, a new criterion, or a removed one that `--retire` does not name is changed acceptance, which is forward work. It never touches the spec leg or the surface leg of `stale_spec`.

New work the reshape needs is staged too, so the whole shape arrives as one proposal: `--add-epic <epic-key>`, `--add-feature <epic-key>/<feature-key> --change-type <ct>`, and `--add-task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>"`. A staged epic may parent features staged later in the same amendment; stage the epic first. A task's parent must already be materialized, never a feature staged in that batch. Leave a new feature's task breakdown to ordinary `specify` when it is pulled forward after materialization, not a speculative second amendment. Nothing is created before approval: staged epics and features materialize in `draft`, tasks in `proposed`, with their normal gates intact.

**Prerequisites, soft order, and implemented limits.** In the narrative, distinguish actual execution prerequisites from preferred sibling order and eventual architectural integration. Independently acceptable outcomes can be split to avoid an unnecessarily broad barrier; do not erase a true prerequisite or describe incomplete behavior as complete. An amendment has no link, unlink, or order operation. `--revise` re-pins accepted prose; it does not change dependencies or unlock started work. Direct hard-edge changes obey noun-specific eligibility: tasks must be proposed or ready; features draft, specced, or ready with no branch; epics draft or ready with no branch, descendant feature branch, or descendant work past the unstarted states. A fresh task under an active feature may still receive its own legal sibling prerequisites. In-place dependency changes on unchanged started work are unsupported, except that a started task may add (never remove) an edge to a done task; a `dependency_change_locked` refusal names that edge rule, and amendment is not an edge-edit route.

**Move unchanged work without replacing it.** `tiller rename feature <ref> --key <epic>/<feature>` is the lightweight route only for a draft, task-free feature; changing parents additionally requires draft source and destination epics. Otherwise stage an eligible feature with `tiller amend <A-id> --move-feature <feature-ref> --to <destination-epic>/<feature-key> --reason "<why>"`, inspect `tiller show amendment <A-id>`, commit the proposal, and stop for the human's `tiller approve amendment <A-id>`. A destination epic may already exist or be staged earlier in this batch. Draft, specced, and ready features may move with proposed or ready tasks, including claimed ready tasks, only if the feature and all descendants have never started and have no execution or review history. Both parents must be draft or ready; done siblings alone do not disqualify an unstarted feature. An affected open coarse review refuses. A move preserves feature/task IDs, claims, status, authored content, and original acceptance provenance, not a recreated approximation. It neither approves a draft destination nor approves a specced plan.

The proposal preserves effective review grain, showing an explicit override when required. Changes to effective inherited prerequisites or question holds refuse and name the difference; correct intentional policy changes through their existing supported routes before restaging, never weaken a real prerequisite to make the move pass. Approval rechecks the captured subtree and inherited settings; a new claim, start, edit, or occupied destination can stale the proposal. Do not mix a move with cancellation, supersession, or revision of its subtree. Cross-parent moves remove the feature from source order and append it at the destination in staged order; a same-parent key change retains its position. Inspect surfaced path citations, links, and acceptance bindings for separate human review instead of globally rewriting prose or historical references.

A started descendant produces a precise refusal naming that work and its history, not replacement work, a state edit, or a dependency workaround. Cancellation remains for genuinely abandoned or replaced unfinished scope; supersession records replacement of done work. Never manufacture replacement identities for an unchanged relocation.

Features from bounded epics may interleave. A projected epic ordering cycle alone is a nonblocking advisory, not proof of bad product boundaries; inspect its implicated features and recorded edges. Genuine feature/task cycles, explicit epic prerequisites, and mixed feature/epic completion deadlocks remain blocking. Do not remove valid prerequisites just to make the grouped map sort cleanly. Workflow administration and oversight can justify different increments, but neither implies a universal permanent sink epic.

Staged additions carry no dependency edges. Name proposed post-materialization `tiller link <noun> <dependent> --depends-on <prerequisite>` actions only while the dependent remains eligible. Name preferred sequence separately with `tiller order epic|feature|task ...`; it orders siblings, not arbitrary features across epics. Approval appends fresh epic IDs to the existing root soft order in staged order, but applies no narrative placement and gives features/tasks no automatic order entry. Intended placement therefore needs an explicit, human-directed `tiller order` after materialization. The command replaces that parent's supplied list and permits partial lists: inspect existing order and preserve unrelated placement rather than accidentally omitting it.

An epic is not a `revise`, `supersede`, or `cancel` target. It reshapes through its features; `--add-epic` can stage a new increment.

When a drift cascade is what brought you here, the reshape is also what clears it. `tiller drift` exempts a cancelled node and a superseded feature from its sweep, and a revise re-pins the very basis the sweep compares against, so the candidates the cascade raised clear as the batch applies rather than wanting a second pass afterwards. Candidates the reshape does not touch stay `mend-drift`'s, one at a time.

**Preservation.** The discipline that makes a rework safe is preservation: the replacement's contract binds `preserve` criteria to characterization tests that pin the behavior users depend on, captured before that behavior moves. Captured afterwards it records the new behavior and proves nothing, so the capture belongs ahead of the rework rather than inside it. The contract itself is `specify`'s to write when the human pulls the new feature forward, and `tiller guide specify` carries its shape. Amend owes something narrower: name, node by node, the behavior the replacement has to keep, in the notes the human reads before approving the batch.

**The durable intent.** A change this size usually obliges edits to the design docs under `.tiller/design/`, and nothing else tracks that they happened. `--triggered-by` on `tiller add amendment` names the docs whose change prompted the reshape; `tiller amend <A-id> --amends-design <path>` names the docs this reshape obliges you to edit. Tiller never reads a design doc's meaning, so neither flag validates prose: the record is the accountability, and it is what a later read has to compare against. The rationale itself goes in the amendment's own `.md`, which `tiller add amendment` seeds with Why, What changes, and What to watch; that narrative is what the human reads beside the batch.

**Preconditions.** Each one pins what an operation was reasoned against, and one rides every destructive operation. `target_status=<status>` is the state you read the target in. `target_revision=git:<commit>` is the target's own recorded commit anchor. `key_absent=<key>` is an add's claim that nothing holds the key yet. `spec_revision` and `eval_revision` pin the exact `feature.md` and `feature.eval.md` content a revise was reasoned against, written as `git:` plus the blob hash `git hash-object <path>` prints. Approve validates every one against the working tree before it writes anything, and a single stale precondition fails the whole approval with no writes, naming every stale operation rather than only the first. That is what makes a batch reviewable rather than merely written down: it applies as reasoned, or it does not apply.

Two shapes of that guard are worth knowing before you rely on it. A supersede staging no `target_status` is not refused, because staging asserts nothing about lifecycle state. Approval still requires the target to be done even without that precondition. Stage `--precondition target_status=done` as the recommended stale guard when the reviewed batch should also prove the state has not moved; `tiller show amendment` explains the distinction when a supersede stages none. And `target_revision` reads nothing on a done task, whose done is the merge rather than a pinned record, so guard a done task with `target_status=done` alone. For the anchor value itself: `tiller show` renders the spec and eval legs of a basis and not the commit anchor, so read `accepted_against.commit`, or `verified_against.commit` on a done feature, out of the node's own `feature.yml`. Reading that file is correct; editing it is not.

A good amendment reads as one coherent story in a single pass: done work superseded rather than edited, few destructive operations, each carrying its reason and the precondition it was reasoned against, and additions that bring the preservation the rework needs. The narrative says why and the staged batch says what, and until the human approves it nothing in the tree has moved.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md` and inspect the named reason. Move staging checks eligibility before recording the proposal; approval revalidates the captured subtree, inherited settings, and destination. A stale or ineligible move must leave the tree unchanged. Explain the specific started descendant, review, changed hold, or collision instead of presenting approval as an override. Unknown targets, malformed flags, and calls against approved or withdrawn amendments require correcting the call. Existing revise/supersede/cancel/correct-eval operations also retain their intrinsic lifecycle and lineage checks.

Two ways forward from it, and only two, because nothing edits or removes an operation already staged: staging appends. Read every operation the refusal names, then either put the tree back the way the batch was reviewed against, which is right when the change that moved it was not the one you meant, or withdraw the amendment and open a new one staged against the tree as it now stands, which is right when the newer content is what you want. Then hand the gate back. Only the first of those is yours to run: `tiller withdraw amendment` takes a required `--reason` and runs only on an explicit human instruction naming the amendment, so propose the withdrawal and say what it would cost.

For a stale move, never erase execution/review history or undo a real start to recreate eligibility. Keep the feature at its current address and explain the precise blocking descendant or policy change. A title-only correction or human-directed sibling preference may be a safe alternative when it serves the request; neither changes parentage. Withdraw and restage only after the human directs that recovery and the new proposal is genuinely eligible.

For `supersede_target_not_done`, choose by intent. If the current work should finish and then be replaced, wait for it to reach done and hand the same approval gate back; `target_status=done` remains the recommended stale guard. If it must be replaced before completion, the staged operation is the wrong one: propose withdrawing the amendment, then open a fresh amendment and restage the edge as `--cancel <target> --reason "<why>" --superseded-by <replacement>`. Nothing edits an operation in place.

For `revise_target_terminal`, do not re-pin the terminal record. Done and cancelled work is append-only. Propose withdrawing and restaging without the revise, then create new forward work that records the change instead.

For a `correct_eval_changes_contract` refusal at approval, the message names the criterion and the field. Restore that statement, polarity, required flag, or baseline surface, or name a removed criterion in `--retire` (withdraw and restage, since nothing edits an operation in place); if the acceptance really changed, drop the correct-eval and take the change as forward work. `correct_eval_target_not_done` means the feature is unfinished, where `--revise` is the route. `correct_eval_duplicate_target` means the batch corrects one feature twice; withdraw and stage one correction. `correct_eval_unpinned` means the operation lost the `eval_revision` pin staging writes; withdraw and restage so the reviewed bytes are pinned again. `correct_eval_unverified` means the completion record pins no contract to correct. `correct_eval_base_unreadable` means Git cannot read the pinned blob as the verified contract; if an earlier correction or done crossing is uncommitted, commit it, then ask for approval again. `correct_eval_no_contract` means `feature.eval.md` is missing from the working tree. `correct_eval_no_change` means the eval file still matches the verified contract. `eval_contract_invalid` means the corrected file does not parse or is not a valid contract; fix it to the shape `tiller guide specify` shows, then withdraw and restage, since the staged pin names the old bytes. A stale `eval_revision` (exit 1) means the file changed after staging; withdraw and restage over the bytes the human should review. Staging refuses a non-feature with `correct_eval_not_feature`, a feature with no `feature.eval.md` with `correct_eval_no_contract`, a missing `--reason` with `reason_required`, and `--retire` on any other operation with `retire_misplaced`.

One more `2` comes from the tree rather than from the call. A `--revise` re-pins its target's basis by hashing the files that basis names, so a target with no `feature.eval.md` (a feature that never got its contract) or a spec deleted since staging refuses with `revise_target_incomplete`, naming the file. Write or restore it, or drop the operation, which is the withdraw-and-reopen path above and carries the same instruction.

One ordering rule refuses in a way that reads like a missing node. A `--by` or a `--superseded-by` may name a staged addition by its key, and that resolves only against operations already on the amendment, so stage the addition before the operation that names it. A forward reference exits `2` with `"<key>" names no feature or task this amendment can target`.

One addressing rule is particular to this noun. An amendment answers to its key or to its minted `A-` id, and a withdrawn or approved amendment keeps its key, so one key can be carried by several amendments over a project's life. A key names the open proposal whenever there is one, which is what you almost always mean by it, and a key no open amendment holds still names a single retired holder. What a key cannot do is reach past a live proposal to a retired one, or choose between two retired ones: read those by the `A-` id their own `add` printed.

## Choreography

First run the structural preflight above, before choosing placement or opening a proposal. These examples show the lightweight correction exit and a genuine scope replacement; unchanged relocation instead stages `--move-feature` under the same inspect, commit, and human-approval boundary.

```
# corrections only: no amendment, no staging, no ceremony
# edit only same-scope, non-terminal feature.md, feature.eval.md, or task.md, then hand the human
# (or run on their explicit instruction naming the node):
tiller approve feature form-engine/certification-form-loading --refresh-basis
tiller approve task T-k3p9w6 --refresh-basis
```

The second is the structural reshape: open it, stage it, show it, and stop.

```
tiller drift                                               # when a cascade brought you here, read it first
tiller show feature form-engine/certification-form-loading --json   # the surface and the basis that bound the reading
tiller add amendment form-engine-rework \
  --title "Restructure form engine from embedded JSON to discrete tables" \
  --triggered-by design/solution-design/04-form-engine.md
# write the reshape's narrative over the stub at:
#   .tiller/amendments/<A-id>-form-engine-rework.md
# stage the additions first: a --by, --superseded-by, or --blocks naming a
# staged key resolves only against operations already on this amendment

# new scope the reshape also names: an epic, staged FIRST, so a same-batch
# add-feature below can name it as parent (epic before feature is the order
# that makes it legal; staged in the reverse order this refuses, "print-layouts"
# names no epic in the tree, and no add-epic staged earlier on this amendment)
tiller amend form-engine-rework --add-epic print-layouts --title "Printable form layouts"
tiller amend form-engine-rework --add-feature print-layouts/pdf-export \
  --change-type create --title "PDF export" \
  --precondition key_absent=print-layouts/pdf-export

# a statable contract under the EXISTING form-engine epic: resolved the
# ordinary way, against the live tree, not against anything staged above
tiller amend form-engine-rework --add-feature form-engine/table-backed-forms \
  --change-type replace --title "Table backed forms" \
  --precondition key_absent=form-engine/table-backed-forms
tiller amend form-engine-rework --add-task form-engine/rendering-modes/table-aware-rendering \
  --title "Render from the table model" \
  --precondition key_absent=form-engine/rendering-modes/table-aware-rendering

# a judgment call the reshape raises but does not settle: a question, not a
# decision invented here. --blocks reaches furthest of any field in this op
# set, resolving a same-batch staged epic, feature, or task key as readily as
# a live one, since a question can hold work that does not exist yet
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
#   git hash-object .tiller/epics/form-engine/features/rendering-modes/feature.md
tiller amend form-engine-rework --revise form-engine/rendering-modes \
  --precondition spec_revision=git:3e9293c9d2b5686c3f12421e797ace6d8acabee5
# a correct-eval of a done feature: edit only the bindings in feature.eval.md first
tiller amend form-engine-rework --correct-eval form-engine/certification-form-loading \
  --reason "loader tests moved into two files" --retire C-4
tiller amend form-engine-rework --amends-design design/solution-design/01-overview.md
tiller show amendment form-engine-rework                   # the batch, each precondition's live validity
git add .tiller && git commit -m "amend: stage the form engine reshape"
#   (directly on the integration branch: a .tiller/-only diff needs no PR)
tiller readout amend                                       # the block that closes your reply, reproduced verbatim
# STOP. the human reads the narrative and the batch beside the git diff, for
# form-engine-rework. on an explicit instruction naming the amendment you may
# run the gate yourself, stating what it is about to apply first; otherwise hand
# them the exact command:
#   tiller approve amendment form-engine-rework
# the escape is theirs the same way, when the reshape is wrong or overtaken:
#   tiller withdraw amendment form-engine-rework --reason "<why it is dropped>"
```

## Readout

End by running `tiller readout amend` and reproducing its block verbatim as the last thing in your
reply to the developer. The block is Tiller's, not yours: it renders the same whoever is driving, which
is what lets a developer learn to read it once instead of learning your habits. Do not restate it in
prose around it, the staged operation count and the precondition tally included; the block already
names them on its own authority. The block closes the reply: nothing follows it. It ends on its
**Next:** ranked list, the one place the developer looks to learn what happens next, so reproduce the
list as written and never add a list of your own after it.

Before the block, add a short **Notes from this run** list for what it cannot know: the blast radius you
read and what you deliberately left standing, which nodes you sorted as corrections rather than
casualties and the refresh-basis command each of those needs, the reasoning behind each destructive
operation in a line, why additions extend an existing bounded outcome or form a distinct increment,
the true prerequisites versus proposed soft order and legal post-materialization actions, the behavior
a replacement feature has to preserve, the design docs this reshape obliges, and any stale precondition
or unsupported active edge/move request the human must settle. Keep it to facts about this run.
In the notes and in everything else you write to the developer,
name every epic, feature, and task the way the block itself does, key path with the id in parentheses,
never a bare id: an id alone sends the developer off to look up what it points at. Questions and
commentary are yours to add freely, with the notes ahead of the block, never after it.

## Hand-off

Up to the human. `tiller approve amendment` is the one act that executes the supersessions, the cancels, the revisions, and the materializations this skill only ever staged, and it applies the whole batch atomically: every precondition is validated first, and then either all of it lands or none of it does. Approve and withdraw each run only on an explicit human instruction naming the amendment; otherwise you hand them the exact command and stop. Out to craft for the rework itself, which once the amendment is applied runs through `execute` like any other work, and back through `specify` for a materialized feature when the human pulls it forward from the frontier, a materialized task waiting on its own `tiller approve task <id>` first.
