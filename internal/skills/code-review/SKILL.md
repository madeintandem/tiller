---
name: code-review
description: Use only when the developer explicitly asks for a code review of one Tiller epic, feature, task, bug, chore, or tweak, or names rows of an existing review to log, fix on their branch, drop, or commit. Reviews the node's net change read-only, writes one review file under .tiller/reviews/, and replies with a summary table. A later disposition logs the selected rows as proposed bugs, tweaks, chores, or one proposed amendment and commits once, never approving anything.
---

# code-review

## Trigger

This skill has two arms, and each needs its own explicit request.

The review arm runs when the developer asks for a code review of one Tiller node, for example "code review feature F-rnte2m", "give me a code review for epic E-2hjdhh", or "review bugs/blank-on-reload before I merge it". The target is one epic, feature, task, bug, chore, or tweak, named by key, ID, or a reference the conversation makes unambiguous. Otherwise ask which node is meant, naming the candidates from reads rather than guessing. A request for the whole project, an arbitrary diff, or a path is not a Tiller review; say so and leave it to whatever general-purpose reviewer the developer prefers.

The disposition arm runs when the developer names rows of an existing review file: "log B1, B3, and TW1-3", "log all blockers", "log B6 as a tweak", "fix B2 on its branch", "drop CH1", "log the rest", or "commit the review".

Entering a project, finishing work, reaching a gate, or asking what to do next triggers neither arm. A review is never a suggested next move. The request authorizes only the review or the named disposition: never approve, claim, start, attest, resolve, or cancel anything.

## Reasoning

### Review arm

#### Scope is the node's net change

Gather every unit under the target that is not cancelled: the task itself, a feature's tasks, or every feature's tasks for an epic. A bug, chore, or tweak is its own single unit. Read each unit's state file for `status`, `branch`, `pr`, and `review_ref`, and read the integration branch from `.tiller/tiller.yml`.

Locate each unit's change with local, read-only Git:

| Unit | Where its change is |
|---|---|
| In flight: its head is not yet an ancestor of integration | The recorded branch head against its merge-base with the integration branch. Use the local branch, or its remote-tracking ref when only that exists. When the local branch and its remote-tracking ref differ, review the local branch and say so under Limitations. |
| Merged: done, or merged but not yet synced (its head is already an ancestor of integration while its status still reads in flight) | Diff from the first parent of the merge that brought the head in: the first merge on integration's first-parent line that contains the head while its own first parent does not. A fast-forward, squash, or rebase leaves no such merge; then use the integration-branch commits whose messages carry the unit's ID, which every execution commit does. |
| Not started, or no locatable change | Not reviewed. It goes under Limitations with the reason. |

The unit's head is its recorded branch tip if the ref still exists, else its `review_ref.head` if that commit is present. Decide merged or in flight from Git, never from status: a branch whose PR the human merged reads in flight until someone runs `tiller sync`. An empty located diff means the change was not located, never that the unit changed nothing; it goes under Limitations.

A target with no started unit gets a one-line reply that there is nothing to review yet, and no file.

The diffs locate what changed. Judge findings against the code as it reads now: the integration branch for merged work, the branch head for in-flight work. A defect one unit introduced and a later unit fixed is not a finding. Read surrounding code for context, but anchor every finding in lines this node's work changed. Record the integration commit and each in-flight branch head you read; these pins are what keep the file's line references meaningful after the code moves on.

#### What to look for

| Lens | Lands as |
|---|---|
| Correctness | Bug |
| Acceptance contract: `feature.md`, the `feature.eval.md` criteria, and `preserve` criteria | Bug, always a blocker |
| Security and permission boundaries | Bug |
| Test trustworthiness | Important bug for a test that proves nothing; chore for missing coverage of a risky path |
| Maintainability: duplication, unclear names, dead code | Tweak or chore |
| Design: a wrong boundary, an abstraction that does not earn its keep, a shape that will not scale | Plan change |
| Performance, only where the cost is evident in the code | Bug or plan change |

Leave out spec, doc, and code drift: `tiller drift` and `mend-drift` own it, so a suspicion goes under Limitations as "run `tiller drift`". Leave out formatter or linter style, accessibility, and every optional review gate.

#### Classify each finding

Use capture's kinds exactly. A bug is behavior that violates expected product or system behavior, including any violated requirement. A tweak is a minor adjustment that may change code. A chore is housekeeping without code side effects. Anything larger, work that needs a feature's acceptance contract or a change to the tree, is a plan change: a new task under a named feature, a new feature under a named epic, a new epic, or a revision of a non-terminal feature or task. A change to done or cancelled work is new forward work, never a revision.

#### Grade bugs by impact

| Severity | Meaning |
|---|---|
| Blocker | Data loss or corruption, a broken security or permission boundary, a crash or hang on a main path, or any violation of an approved acceptance criterion or `preserve` criterion of the reviewed node. Should not merge, or should not stay shipped. |
| Important | Wrong behavior on a realistic path without data loss, including a test that passes while proving nothing about the behavior it claims to cover. |
| Medium | Wrong behavior on an edge case or unusual input, or a fragile construct likely to break under a plausible change. |
| Minor | A real defect with negligible impact, such as a wrong message, a misleading log line, or an off-by-one in a display-only count. |

Grade by impact, never by the size of the fix. Style and naming are never bugs. Severity is review prose only: it never becomes a record field or a queue position.

Label every bug's confidence. Traced means you followed the failing path through the code and can name each step. Suspected means the pattern is wrong but the path is unproven. Proof by execution comes later, when the fix's regression test is written.

#### Attach each finding where it was found

Each finding records Found in, and Found in names each node by key path with its ID, as every reply to the developer does: the unit whose diff introduced its anchored lines, the feature when it spans that feature's tasks, or the epic when it spans features. For a bug, chore, or tweak target, Found in is the target itself. `--attach` accepts only an epic, feature, or task, so a finding logged from a bug, chore, or tweak target attaches where that target was found, its own `attached_to`, or omit `--attach` when it has none; name the reviewed node in the contract's prose instead. When the code is unmerged, Found in carries the unit's status, for example `form-engine/form-reload/save-retry (T-d4e5f6), in_review, unmerged`.

#### Respect what Tiller already knows

Before writing, read the bugs, chores, and tweaks that are neither done nor cancelled and are attached to any in-scope node, the proposed amendments touching the scope, and earlier review files for in-scope nodes under `.tiller/reviews/`.

- A finding that matches an open bug, chore, or tweak, or a staged plan change, goes in Already tracked, unnumbered, citing that node.
- A finding the developer declined in an earlier review that still holds goes in Previously declined, unnumbered, citing that review.
- A defect that returns after its bug reached done is a new finding marked `regression of <id>`. Never reopen done work.

Matching is your judgment, so always cite what you matched against; the developer may overrule it.

#### Keep the review read-only

The only write is the one new review file. Do not change prose, source code, state, the manifest, claims, gates, events, questions, Git history, refs, or the index. Do not run `check`, `eval`, `sync`, `attest`, `claim`, `join`, `migrate`, or any other state-writing command. Do not fetch, check out, stash, or build, and do not run the project's tests or scripts. Read branches with `git show` and `git diff`. Prefix every Tiller invocation with `env GIT_OPTIONAL_LOCKS=0`, use `git --no-optional-locks` for direct Git reads, and pass `--no-ext-diff --no-textconv` to every diff. A branch absent locally and from remote-tracking refs goes under Limitations with the exact `git fetch` to run, so the developer can fetch and ask again.

#### Cover everything or say what you missed

Review in feature-sized passes and merge them into one file. For an epic, finish with one cross-feature pass over the seams between features. Number findings only after the merge, so severity order holds across passes. Passes may run in parallel when your environment allows it, but the result is one file. Never sample, never cap findings, and list every unit not reviewed under Limitations with its reason.

### Disposition arm

#### Find the review file

Use the review from this conversation. In a new session, the candidates are the files under `.tiller/reviews/` whose name carries the node the request names ("log B1 from the F-rnte2m review"). Order candidates by date, then suffix number, so `-2` follows the unsuffixed file. When more than one candidate exists, ask which, naming each with its date and open-row count. When the request names no node and none is in this conversation, ask which review the developer means.

#### Resolve the whole selection before writing anything

Plain numbers resolve through the noun the developer used: "bugs 1, 3" is B1 and B3, "tweaks 1-3" is TW1 through TW3, "plan change 2" is P2. Lists, ranges, and "all" work ("all blockers", "all tweaks"). "the rest" means every numbered row with no disposition yet. A row whose heading carries a disposition annotation is closed. A row annotated `requested change on <unit>` stays open to "fix" by that unit's owner, so a hand-off never ends in a refusal. If any named row does not exist, is closed, or is ambiguous, write nothing: name every problem at once and wait for the developer.

#### The verbs

| Request | Effect |
|---|---|
| "log B1, TW2, P1" | Bug, tweak, and chore rows become `proposed` shelf nodes. Plan-change rows become one `proposed` amendment. |
| "log B6 as a tweak" | Reclassify, then log as the chosen kind with that kind's contract. The row records both kinds. |
| "log B3, attach to <node>" | Replace the row's Found in for this attachment. |
| "B-k2x9ab is different, log it" | Override an Already tracked match, number the finding into its section, then log it. |
| "log the declined X as a bug" | Revive a Previously declined finding, number it into its section, then log it. |
| "fix B2 on its branch" | Only for a row whose Found in is unmerged. Nothing is minted; the row goes back to its unit through execute. |
| "drop CH1" | Decline the row so a later "the rest" skips it. Optional: a row nobody mentions stays open. |
| "commit the review" | Commit the review file with no other change, for a clean review or one where nothing is logged. |

A new number takes the next free number in its section, after every existing one. Existing numbers never change.

#### Recheck moved code

Compare the file's pins with the current heads. If nothing in scope moved, proceed. Otherwise re-read each selected row's anchors in the current code, still read-only:

- The finding holds: proceed, and annotate the row `rechecked at <commit>`.
- The finding holds but its lines moved: proceed with corrected anchors, and correct the row in the same commit.
- The finding is gone: stop the whole batch, report "<row> no longer reproduces at <commit>", and wait for "drop it" or "log anyway".

Never recheck an unselected row. A fix always rechecks, because execute is about to work from it.

#### Log bugs, tweaks, and chores through capture's shapes

Follow `tiller guide capture` for keys, contracts, and its stopping point, except their commits: this disposition makes the one commit. Pass the row's Found in as `--attach` and cite the row's block in the current review file as `--source`, so the developer's edits to the file carry through. Write the contract from the block: a bug's Reproduction, Expected, Actual, and Fixed when; a tweak's or chore's What, Why, and Done when. Keep the review's severity and confidence as the opening line of `bug.md`; it is prose, never a field. For a row in unmerged code, warn that the new node cannot start until its unit merges, because a shelf branch is cut from the integration branch.

#### Log plan changes through intake's shapes

Follow `tiller guide intake` for staging and the amendment narrative, except their commits, which this disposition replaces with its one commit. Write a distillation outside the repository named `<YYYY-MM-DD>-<node-id>-plan-changes.md` (add `-2`, `-3`, and so on when `.tiller/feedback/` already holds that name). It holds only the selected plan changes, each as its own block with its problem and proposed outcome, under a header naming the review file and the row numbers it came from. Land it with `tiller intake`, then stage one operation per plan change, each citing its own block in the landed copy under `.tiller/feedback/`. A revision targets only a non-terminal feature or task: write the drafted prose edit to the target's spec or contract before staging `--revise`, because the revise records that edit for approval. A change to done work stages forward work instead. Each request that logs plan changes is one distillation, one intake, one amendment; a later request never stages onto an earlier review's amendment.

#### Fix on its branch

Requested changes return to the unit's owner and branch. Compare the unit's `owner` against the checkout's identity (`.tiller/identity.yml`). When the acting developer does not own the unit, annotate the row `requested change on <unit>` and hand it to the owner without executing anything. Otherwise annotate, commit, and follow `tiller guide execute` for that unit: the requested-changes re-entry for in-review work, or the ordinary loop for work still in progress.

#### Annotate and commit once

Append each disposition to the end of its row's heading line, so no line moves and every line-range citation stays exact: `→ B-k2x9ab`, `→ amendment <amendment-key>`, `→ requested change on <unit>`, `declined`, `logged as tweak TW-...`, `rechecked at <commit>`. Then make one commit holding the annotated review file, any new shelf nodes, and any feedback copy and amendment state. Stage only the paths this disposition wrote: other reviews and stray state may sit uncommitted beside them, and if `git status --short .tiller` shows any, name them to the developer and leave them out. Commit on the integration branch, as other `.tiller/`-only planning changes are; a `.tiller/`-only diff needs no PR. If the checkout sits on a task branch, switch to the integration branch first. Never push.

## Recovery

On a refusal, follow `.tiller/orientation.md`. In the review arm, recovery is limited to corrected reads: a missing branch, an unreadable diff, or stale instructions go under Limitations, never become a reason to repair, fetch, refresh, or mutate anything.

In the disposition arm, a `tiller add` refusal means fixing the key, attachment, or citation and rerunning; re-adding the same key is an idempotent readback. An `artifact_exists` refusal from `tiller intake` means choosing the next suffix for the distillation. Commit nothing until every selected row is recorded.

## Choreography

### Review arm

#### 1. Resolve the target and inventory its units

```sh
env GIT_OPTIONAL_LOCKS=0 tiller show <epic|feature|task|bug|chore|tweak> <key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller tree epic <epic-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller tree feature <feature-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller ls tasks --feature <feature-key-or-id>
```

Run only the forms that apply. Read each unit's state file for `branch`, `pr`, and `review_ref`, and read each in-scope feature's `feature.md` and `feature.eval.md` in full. Read the target's own contract too: a task target's own `task.md`, or a bug's Fixed when, or a chore's or tweak's Done when, which is that node's acceptance bar. For a task, also read the departures execute recorded in its implementation commit messages and PR body: a factual claim in `task.md` that the code proved wrong. Where a departure is recorded, judge the diff against the recorded departure, not the stale fact, and do not stage a revision to correct that fact. A departure that changes behavior, acceptance, scope, a dependency, or a planned test is not lawful; report it as a finding.

#### 2. Locate each unit's change

```sh
# <head> is the recorded branch tip, else review_ref.head
# (its value reads git:<sha>; drop the `git:` prefix)
git --no-optional-locks cat-file -e <head>^{commit}
git --no-optional-locks merge-base --is-ancestor <head> <integration>
# exit 1: in flight. Diff the branch against its merge-base with integration
git --no-optional-locks merge-base <integration> <head>
git --no-optional-locks diff --no-ext-diff --no-textconv <merge-base> <head> -- . ':(exclude).tiller'

# exit 0: merged. Walk integration's first-parent merges, oldest first
git --no-optional-locks rev-list --first-parent --merges --reverse <integration>
# <merge> is the first one where both of these hold:
git --no-optional-locks merge-base --is-ancestor <head> <merge>
! git --no-optional-locks merge-base --is-ancestor <head> <merge>^1
git --no-optional-locks diff --no-ext-diff --no-textconv <merge>^1...<head> -- . ':(exclude).tiller'

# no head, or no such merge (fast-forward, squash, rebase): commits carrying the unit's ID
git --no-optional-locks log --format=%H --grep=<unit-id> <integration>
git --no-optional-locks show --no-ext-diff --no-textconv <commit> -- . ':(exclude).tiller'
# Skip `code-review:` disposition commits and commits that touch only .tiller/

# pins for the file header
git --no-optional-locks rev-parse <integration> <each-in-flight-branch>
```

A unit none of these locate goes under Limitations.

#### 3. Read what is already known

```sh
env GIT_OPTIONAL_LOCKS=0 tiller ls bugs
env GIT_OPTIONAL_LOCKS=0 tiller ls chores
env GIT_OPTIONAL_LOCKS=0 tiller ls tweaks
env GIT_OPTIONAL_LOCKS=0 tiller ls amendments --status proposed
ls .tiller/reviews/
```

Read `attached_to` for each open bug, chore, or tweak, each proposed amendment's staged operations, and the earlier review files for in-scope nodes.

#### 4. Write one review file

Write `.tiller/reviews/<YYYY-MM-DD>-<node-id>.md`. When that path exists, add `-2`, `-3`, and so on: never overwrite a review. Every finding is one contiguous block under its own heading, so a line range cites exactly that block. Use this shape:

```markdown
# Code review: <kind> <key path> (<id>) "<title>"

Reviewed <YYYY-MM-DD> by <contributor>. A derived review, uncommitted until a disposition.
Scope: each unit by key path, ID, and status.
Pins: integration <branch> at <commit>; <in-flight branch> at <commit>.
Contract read: the feature.md, feature.eval.md, and preserve criteria read.

## Summary

<the reply's table>

## Bugs

### Blocker

#### B1 <title, at most 60 characters>

Severity: blocker. Confidence: traced. Found in: <key path> (<id>).
Anchors: `<path>:<line>`, each with a short excerpt. Violates: <criterion ID>, when one applies.
**Reproduction** <steps>
**Expected** <behavior>
**Actual** <behavior>
**Fixed when** <observable condition>

### Important

### Medium

### Minor

## Tweaks

#### TW1 <title>

Found in: <key path> (<id>). Anchors: `<path>:<line>`.
**What** <change> **Why** <reason> **Done when** <observable condition>

## Chores

#### CH1 <title>

Found in: <key path> (<id>). Anchors: `<path>:<line>`.
**What** <change> **Why** <reason> **Done when** <observable condition>

## Plan changes

#### P1 <title>

Proposed shape: new task under <feature> | new feature under <epic> | new epic | revise <feature or task>.
**Problem** <what is wrong> **Proposed outcome** <what should be true>
Criteria it would add or change, when any.

## Already tracked

## Previously declined

## Limitations
```

Number per section with prefixes: B, TW, CH, P. Bug numbers run straight through the severity groups, most severe first, so every number is unique. Omit an empty severity group or section, except Limitations, which says "None" when nothing was missed. A clean review still writes its file.

#### 5. Reply and stop

Reply with the target, the review path, and one table holding every numbered row, then the counts. Never cap the table. For example:

```text
Code review: feature form-engine/form-reload (F-rnte2m), 4 tasks, 2 in flight
Full review: .tiller/reviews/2026-10-08-F-rnte2m.md

| #   | Severity  | Finding                            | Found in             | Where            |
|-----|-----------|------------------------------------|----------------------|------------------|
| B1  | blocker   | Draft lost on reload (violates C3) | form-engine/form-reload/load-draft (T-a1b2c3) | form/load.go:88  |
| B2  | important | Retry swallows 409                 | form-engine/form-reload/save-retry (T-d4e5f6), unmerged | form/save.go:141 |
| TW1 | tweak     | "Save" to "Save changes"           | form-engine/form-reload/load-draft (T-a1b2c3) | ui/Form.tsx:52   |
| CH1 | chore     | Stale doc comment on loadDraft     | form-engine/form-reload/load-draft (T-a1b2c3) | form/load.go:70  |
| P1  | plan      | New task: offline draft cache      | form-engine/form-reload (F-rnte2m) | -                |

2 bugs (1 blocker, 1 important), 1 tweak, 1 chore, 1 plan change
Also: 2 already tracked, 1 previously declined (details in the file)
Unmerged: "fix B2 on its branch" sends it back to form-engine/form-reload/save-retry (T-d4e5f6)
Log with, for example: "log B1, TW1, P1", "fix B2 on its branch", "drop CH1"
```

Titles stay within 60 characters, and Where shows the first anchor. A clean review replies in one line with the path.

### Disposition arm

#### 1. Resolve rows and pins

```sh
git --no-optional-locks rev-parse <integration> <each-in-flight-branch>
```

Compare with the file's pins and recheck the selected rows when anything moved.

#### 2. Log bug, tweak, and chore rows

```sh
tiller add bug <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/bugs/<key>/bug.md with the contract from the row's block

tiller add tweak <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/tweaks/<key>/tweak.md with What, Why, and Done when

tiller add chore <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/chores/<key>/chore.md with What, Why, and Done when
```

#### 3. Log plan-change rows

```sh
tiller intake <distillation> --contributor <you>
# lands .tiller/feedback/<distillation>.md and opens its amendment

tiller amend <amendment-key> --add-task <epic>/<feature>/<task-key> --title "<title>" \
  --source .tiller/feedback/<distillation>.md#L<n>-L<m>
# or --add-feature, --add-epic, or --revise, per the row's proposed shape
# write the narrative over .tiller/amendments/<A-id>-<amendment-key>.md

tiller show amendment <amendment-key>
```

#### 4. Annotate and commit once

Inspect the existing index and preserve unrelated staged and unstaged work. Review exact paths this disposition wrote, not directories. The path-limited commit excludes unrelated index entries; if a named file contains unrelated edits, stop or isolate only authorized hunks with a reviewed index rather than commit its whole working-tree contents.

```sh
git diff --cached --name-status
git add -- <review-file> <each-path-this-disposition-wrote>
git diff --cached -- <same-exact-paths>
git commit --only -m "code-review: <node-id> log B1 TW1 P1, drop CH1" -- <same-exact-paths>
```

For a fix, commit the annotation alone first (`code-review: <node-id> fix B2 on <unit>`), then follow `tiller guide execute`.

## Hand-off

### Review arm

Stop after the reply. State that the review file is uncommitted and that any disposition, including "commit the review", commits it; deleting an unwanted review is just deleting the file.

When the target is a feature or task and `.tiller/tiller.yml` already declares an optional code review attestation gate, add one line for the human with the manifest's own gate key verbatim (`tiller init` seeds `code_review`) and the blocker count beside it:

```text
Attest when ready (1 blocker open): tiller attest <feature|task> <id> --gate <gate-key> --status <pass|fail> --reason "reviewed: .tiller/reviews/<file>.md"
```

At feature or epic grain the human runs that attest on the recorded aggregate branch, in the worktree that holds it, and commits it there before submit; tiller refuses it elsewhere. Leave the status to the human and never run it. Print nothing about gates for an epic, a bug, chore, or tweak, or a project without that gate. Never add a gate to the manifest because a review was requested.

The review is not a lifecycle seam. There is no `tiller readout code-review`, and no lifecycle readout follows the reply.

### Disposition arm

Stop after the commit. Name each new bug, chore, or tweak by key path with its ID, with the source and attachment recorded and the exact `tiller approve bug|chore|tweak <node>` command for triage. Name the amendment with its exact `tiller approve amendment <amendment-key>` command. Run none of them. Repeat any unmerged-code warning. A fix hands over to execute, or to the owner named on the row.
