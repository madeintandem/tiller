# Code review

Designed and settled 2026-10-08 through a one-fork-at-a-time review. Not built.

## Problem

A developer wants to say "code review feature F-rnte2m" or "give me a code review for epic E-2hjdhh" and get back a list of problems in the work that node delivered, then turn the ones worth keeping into Tiller records with a second short instruction such as "log bugs 1, 3, and 7, chore 2, and tweaks 1-3." Today the agent can review code with whatever its harness offers, but nothing ties the review to the node's recorded work, nothing grades findings the same way twice, and turning findings into bugs, chores, tweaks, or plan changes is a hand-carried, one-at-a-time capture with no record of what was reviewed, logged, or declined.

## Shape

One new skill, `code-review`, installed as `/tiller-code-review` and loaded with `tiller guide code-review`. It has two arms.

- **The review arm** resolves a node to the code its work changed, reviews that code read-only, writes one review file under `.tiller/reviews/`, and replies with a summary table and the file's path.
- **The disposition arm** acts on a later instruction naming rows of an existing review file: log them as Tiller records, send in-flight ones back to their task's branch, decline them, or simply commit the review. Every disposition ends in one commit.

No CLI change. The review is agent reasoning, which principles 1 and 4 keep out of the binary, and every write the disposition arm needs already exists: `tiller add bug|chore|tweak`, `tiller intake`, the amendment surface, and execute's requested-changes re-entry. The `design-brief` skill is the nearest precedent: an explicit request, an export, no lifecycle mutation.

## Decisions

1. **Severity lives only in the review.** Bugs are grouped blocker, important, medium, minor in the review file and the reply, for the developer's benefit. Bug, chore, and tweak records keep their current shape, no severity field is added, and logging never touches the shelf order queues. A logged bug may carry its severity in `bug.md` prose.
2. **Review files live in `.tiller/reviews/<date>-<node-id>.md`.** A second review of the same node on the same day takes a suffix and never overwrites. `.tiller/reviews/` is a prose-only directory, a sibling of `.tiller/feedback/`, with no state file and no schema version change. The loader ignores it; source citations into it already resolve through the shared citation reader, so the console opens them.
3. **Pure skill.** No landing command, no console view of reviews, and no recorded link from a review row to the node it became. The row-to-node link is a prose annotation in the review file.
4. **Scope is the node's net change.** The skill gathers every non-cancelled task under the target (every feature's tasks, for an epic). In-flight tasks contribute their recorded branch head against its merge-base with the integration branch. A done task records no merge commit (only `recovered_review` carries `head` and `merge_commit`), so its change is located from, in order: its recorded branch if the ref still exists, its `review_ref.head` if that commit is still present, or the integration-branch commits whose messages carry the task's ID, which execution commits always do. A done task whose change cannot be located goes under Limitations. The diffs locate what changed; findings are judged against the code as it reads now, so a defect introduced by one task and fixed by a later one is not reported. Surrounding code is read for context only, and every finding is anchored in lines this node's work changed. The file pins the integration commit and each in-flight branch head it read. A node with no started work gets a one-line "nothing to review" and stops. In-flight work is reviewable.
5. **Four sections.** Bugs (B), Tweaks (TW), Chores (CH), and Plan changes (P). A plan change proposes a new task under a named feature, a new feature under a named epic, a new epic, or a revision of an existing node's spec or acceptance contract. Bugs, tweaks, and chores are logged through capture's contracts; plan changes are logged through intake as one proposed amendment, so a reshape of the tree still crosses the human's `approve amendment` (principle 12).
6. **The intake source is a distillation.** Logging plan changes writes a new self-contained feedback document holding only the selected plan changes, headed by the review file's path and the row numbers it came from, and lands it with `tiller intake`. The review file in `reviews/` is the mutable working ledger; the landed copy in `feedback/` is the frozen evidence the amendment's fingerprint pins. Each disposition request that includes plan changes is one distillation, one intake, one amendment; later plan changes from the same review become their own amendment.
7. **Numbering and the selection grammar.** Numbering is per section with prefixes. Bug numbers run continuously through the severity groups, most severe first, so every number is unique. A request resolves plain numbers through the noun it uses ("bugs 1, 3" means B1 and B3) and accepts lists, ranges, and "all" ("all blockers", "all tweaks"). Reclassification at log time is allowed ("log B6 as a tweak", "log TW2 as a plan change"); the agent writes the chosen kind's contract and the row records both the original and the new kind. "Drop B5" declines a row and annotates it so a later "log the rest" skips it; rows nobody mentions stay open. A request that names a missing or already-dispositioned row refuses as a whole and writes nothing until the developer confirms.
8. **Attachment is the narrowest anchoring node.** Each row carries a Found in value: the task whose diff introduced the anchored lines, the feature when the finding spans that feature's tasks, the epic when it spans features. Logging passes it as `--attach`, and the developer may override per row ("log B3, attach to F-abc"). `--attach` accepts only an epic, feature, or task, so a finding from a bug, chore, or tweak target attaches to that target's own `attached_to`, or to nothing, and names the reviewed node in its prose.
9. **In-flight findings can go back to their branch.** A shelf node's branch is cut from the integration branch, where an unmerged defect does not yet exist, so a logged bug against unmerged code cannot start until that task merges. Rows anchored in unmerged code show their task and status in Found in and are flagged in the summary. "Fix B2 on its branch" annotates the row `requested change on T-x` and hands off to execute's requested-changes re-entry on that task. Logging such a row is still allowed, with a warning that the new node waits on the task's merge. The review arm itself never fixes anything.
10. **The severity rubric is fixed, and grades by impact.**

    | Severity | Meaning |
    |---|---|
    | Blocker | Data loss or corruption, a broken security or permission boundary, a crash or hang on a main path, or any violation of an approved acceptance criterion or `preserve` criterion of the reviewed node. Should not merge, or should not stay shipped. |
    | Important | Wrong behavior on a realistic path, without data loss. Includes a test that passes while proving nothing about the behavior it claims to cover. |
    | Medium | Wrong behavior on an edge case or unusual input, or a fragile construct likely to break under a plausible change. |
    | Minor | A real defect with negligible impact: a wrong message, a misleading log line, an off-by-one in a display-only count. |

    The review reads `feature.md`, `feature.eval.md`, and the `preserve` criteria explicitly, and an acceptance violation is always a blocker. Style and naming are never bugs.
11. **Known findings are not renumbered.** Before writing, the skill reads shelf nodes that are neither done nor cancelled and are attached to any in-scope node, proposed amendments touching the scope, and earlier review files for in-scope nodes. A finding matching an open shelf node or staged plan change appears unnumbered, marked `tracked as <id>`. A finding the developer declined in an earlier review appears in an unnumbered Previously declined list citing that review; "log the declined X as a bug" revives and numbers it. A defect that returns after its bug reached done is a new finding marked `regression of <id>`, never a reopen. Matching is the agent's judgment, so each mark cites the existing node and the developer can overrule it.
12. **The review never attests.** When the reviewed node is a feature or task and the project's manifest already declares an optional code review attestation gate, the hand-off prints `tiller attest <feature|task> <node> --gate <gate-key> --status <pass|fail> --reason "reviewed: <review path>"` beside the blocker count, with the status left to the human. `<gate-key>` is the manifest's own key verbatim: `tiller init` seeds `code_review`, while older docs and tests spell it `code-review`, and attest records whatever key it is given. An epic review, a shelf review, or a project without that gate prints nothing about it. Requesting a review never adds a gate to `tiller.yml` or implies one applies.
13. **The reply is one table with every numbered row.** No cap on rows. A clean review still writes its file and replies in one line with the path.
14. **Findings are pre-shaped contracts.** Each finding is one contiguous block under its own heading, so `--source .tiller/reviews/<file>#L<n>-L<m>` cites exactly that block. Developer edits to the file before a disposition carry through, because the disposition arm reads the current file. Plan changes stop at problem and proposed outcome; `specify` writes the spec later through the ordinary path.
15. **The review arm is read-only.** It follows `design-brief`'s discipline: `env GIT_OPTIONAL_LOCKS=0` before every Tiller invocation, `git --no-optional-locks`, no external diff or text-conversion helpers, no fetch or checkout, no state-writing Tiller commands, and no running the project's tests, builds, or scripts. Branches are read with `git show` and `git diff`. An in-flight branch absent locally goes under Limitations with the exact `git fetch` to run. Every bug carries a confidence label: **traced** when the agent followed the failing path through the code and can name each step, **suspected** when the pattern is wrong but the path is unproven. Proof by execution arrives when the bug is worked, since execute already requires a regression test.
16. **Six reviewable kinds.** Epic, feature, task, bug, chore, and tweak, by key, ID, or an unambiguous conversational target. A shelf node's Found in defaults to itself. The whole project, an arbitrary diff, or a path is not a Tiller target and is left to the harness's own review tooling; an unresolvable target gets a clarifying question.
17. **One skill, two arms, namespaced.** The skill is `code-review`, delivered as the `tiller-code-review` shim so it never collides with a harness's own `/code-review`. Its trigger description claims review requests that name a Tiller node and disposition requests that name review rows. The disposition arm points at `tiller guide capture` and `tiller guide intake` for contract and staging shapes and hands "fix" to `execute`, rather than restating them. `orient` routes an explicit review or disposition request to `code-review` outside the state walk, as it does for `design-brief`; a review is never a suggested next move. The disposition arm finds its file from the conversation, or in a new session from the node named in the request ("log B1 from the F-rnte2m review"), asking when more than one candidate file exists.
18. **Every disposition commits; the review run does not.** Log, fix, drop, and an explicit "commit the review" each end in one commit holding the annotated review file, any new shelf nodes, and any feedback snapshot and amendment state, with a message like `code-review: F-rnte2m log B1 TW1 P1, drop CH1`. It lands where capture's commit would land today and is never pushed. A fix commits the annotation before execute moves to the task's branch, so the review commit never lands on a task branch. The review arm's hand-off states the file is uncommitted and names the verbs that commit it.
19. **Large reviews are complete or say what they missed.** The review runs as feature-sized passes merged into one file, numbered after the merge so severity order holds across passes, followed for an epic by a cross-feature pass over the seams. A harness that can run passes in parallel may; the skill text never names a mechanism. Nothing is sampled and nothing is capped. Any task not reviewed is listed under Limitations with its reason.
20. **Lenses.**

    | Lens | Lands as |
    |---|---|
    | Correctness | Bugs |
    | Acceptance contract and `preserve` criteria | Bugs, always blockers |
    | Security and permission boundaries | Bugs |
    | Test trustworthiness | Important bugs, or chores for missing coverage |
    | Maintainability: duplication, unclear names, dead code | Tweaks or chores |
    | Design: wrong boundary, an abstraction that does not earn its keep, a shape that will not scale | Plan changes |
    | Performance, only where the cost is evident in the code | Bugs or plan changes |

    Out of scope: spec, doc, and code drift, which `tiller drift` and `mend-drift` own (a suspicion goes under Limitations as "run `tiller drift`"); the other optional gates, which follow decision 12; formatter or linter-level style; and accessibility, which needs a running app the read-only review does not have.
21. **Moved code is rechecked for selected rows only.** The disposition arm compares the file's pins to current heads. If nothing in scope moved, it proceeds. Otherwise it re-reads each selected finding's anchors, still read-only: a finding that holds is logged and its row notes `rechecked at <commit>`; one whose lines shifted is logged with corrected anchors, and the row is corrected in the same commit; one that no longer holds stops the batch and asks the developer to drop it or log anyway. Unselected rows are never rechecked. "Fix on its branch" always rechecks.

## The review file

```
# Code review: <kind> <id> "<title>"

Reviewed <date> by <actor>. Target, ancestry, and scope: each in-scope task with its
status. Pins: integration commit, each in-flight branch head. Contract files read.
State that this is a derived review, uncommitted until a disposition.

## Summary
<the same table the reply shows>

## Bugs
### Blocker
#### B1 <title>                         (one contiguous citable block per finding)
Severity, confidence (traced | suspected), Found in, anchors with short excerpts,
criterion violated if any.
Reproduction / Expected / Actual / Fixed when

### Important ... ### Medium ... ### Minor ...

## Tweaks
#### TW1 <title>
Found in, anchors. What / Why / Done when

## Chores
#### CH1 <title>
Found in, anchors. What / Why / Done when

## Plan changes
#### P1 <title>
Proposed shape (new task under F-x | new feature under E-x | new epic | revise <node>).
Problem / Proposed outcome / criteria it would add or change

## Already tracked
Unnumbered findings and the node each matches.

## Previously declined
Unnumbered findings, each citing the review that declined it.

## Limitations
Tasks not reviewed and why, branches missing locally with the fetch to run,
suspected drift, anything unreadable.
```

Disposition annotations are appended on the row's heading line, so line-range citations never shift: `→ B-k2x9ab`, `→ amendment <key>`, `→ requested change on T-x`, `declined`, `logged as tweak TW-…`, `rechecked at <commit>`.

## The reply

```
Code review: F-rnte2m "Form reload" (4 tasks, 2 in flight)
Full review: .tiller/reviews/2026-10-08-F-rnte2m.md

| #   | Severity  | Finding                            | Found in             | Where            |
|-----|-----------|------------------------------------|----------------------|------------------|
| B1  | blocker   | Draft lost on reload (violates C3) | T-a1b2c3             | form/load.go:88  |
| B2  | important | Retry swallows 409                 | T-d4e5f6 ⚑ in_review | form/save.go:141 |
| TW1 | tweak     | "Save" → "Save changes"            | T-a1b2c3             | ui/Form.tsx:52   |
| CH1 | chore     | Stale doc comment on loadDraft     | T-a1b2c3             | form/load.go:70  |
| P1  | plan      | New task: offline draft cache      | F-rnte2m             | -                |

2 bugs (1 blocker, 1 important) · 1 tweak · 1 chore · 1 plan change
Also: 2 already tracked, 1 previously declined (details in the file)
⚑ unmerged: "fix B2 on its branch" sends it back to T-d4e5f6
Attest when ready: tiller attest feature F-rnte2m --gate code_review --status <pass|fail> --reason "..."
Log with e.g. "log B1, TW1, P1" · "fix B2 on its branch" · "drop CH1"
```

Titles stay within 60 characters. Where shows the first anchor; the file holds them all. The attest line appears only under decision 12.

## Disposition walk

1. Resolve the review file and every named row. Refuse the whole request on any missing, already-dispositioned, or ambiguous row.
2. Compare pins to current heads and recheck the selected rows (decision 21).
3. For bugs, tweaks, and chores: `tiller add bug|tweak|chore <key> --title ... --attach <Found in> --source <review path>#L<n>-L<m>`, then write the contract from the row's block in the shape `tiller guide capture` names. Each lands `proposed`.
4. For plan changes: write the distillation, `tiller intake <distillation>`, then stage each selected plan change onto the new amendment through the ordinary `tiller amend` surface with `--source` citing its line in the landed feedback copy, in the shape `tiller guide intake` names. The amendment stays `proposed`.
5. Annotate each row, commit once (decision 18), and hand off: each new shelf node with its exact `tiller approve bug|chore|tweak <node>` command, and the amendment with its exact `tiller approve amendment <key>` command, none of them run.
6. For a fix: annotate, commit, then hand off to execute's requested-changes re-entry on the named task.

Capture's and intake's boundaries hold throughout: nothing is approved, claimed, started, resolved, or cancelled by this skill.

## Changes this design obliges

- `internal/skills/code-review/` with the embedded skill, and its entry in the skill catalog and `tiller guide`.
- `docs/spec/skill-set.md`: the skills table row, a `code-review` section in the existing Trigger, Reasoning, Choreography, Hand-off shape, and the dispatcher count.
- `orient`: the routing line for explicit review and disposition requests.
- `docs/spec/artifact-schema.md`: one paragraph declaring `.tiller/reviews/` a prose-only directory beside `.tiller/feedback/`.
- `docs/spec/distribution.md`: the shim catalog grows from ten to eleven.
- `internal/gen/orientation.go`: `orientationSchemaGen` advances one past whatever main holds at build time, since generated instructions change. Main holds 44 today; the unmerged `brownfield-zero-day` branch claims 45 and 46.
- `docs/plans/STATUS.md` once built.

## Out of scope

A severity or priority field on any record. Automatic ordering of logged nodes. A CLI command or console surface for reviews. Running tests or fetching during a review. Attesting any gate. Reviewing anything that is not a single Tiller node. Drift detection and accessibility review.
