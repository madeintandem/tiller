# Prohibition review of the agent-facing instruction prose

Scope: the eleven embedded skills (`internal/skills/<skill>/SKILL.md`) and the generated orientation text (the `orientationBody` constant in `internal/gen/orientation.go`). Status: inventory complete; rewrites applied in `design-brief` and `derive-baseline` only; every duplicate and residue row below waits on the project owner's ruling.

## What this is

A review found the instruction prose dense with prohibitions ("never", "do not", "don't", "must not"). A stronger model applies each one literally and broadly, so a stack of unexplained prohibitions causes over-caution. The remedy keeps real boundaries and says why, states a rule positively where a positive instruction says exactly the same thing, and puts duplicates and incident residue in front of the owner. Deleting anything is the owner's decision.

Every sentence containing one of the four words was classified as one of:

- **Boundary**: something that must not be crossed whatever the context: a human-only verb, a destructive or irreversible action, another person's work, a read-only limit, or an integrity rule (no hand-edited machine state, no invented URL, command, commit, or history, no false claim).
- **Restatable**: a positive instruction would say the same thing. Where the negative names a specific tempting mistake the positive form would drop, the row says so.
- **Duplicate**: the same rule is stated elsewhere in the same file, or in orientation, which every skill assumes.
- **Residue**: it answers one specific past incident, and a general rule already covers it.

A sentence that fits two classes takes the more conservative one: boundary over restatable, restatable over duplicate, duplicate over residue. Several boundary sentences also repeat orientation; by that rule they count as boundaries and are not listed as duplicates.

Two conventions:

- **Not a prohibition** counts sentences where the word states a fact about the system ("Tiller never commits them for you", "the CLI never disagrees with itself") and the sentence repeats nothing. They need no decision. A statement of fact that repeats orientation or its own file is listed as a duplicate.
- A boundary **states its reason** when the sentence, or the paragraph, table cell, or list item it sits in, says why. An integrity rule that names the invention or false claim it forbids carries its own reason. The boundary lists below name only the sentences whose reason is stated nowhere in the skill or in orientation.

Sentences are identified by file and a few distinctive words, not line numbers.

### What was applied, and what waits

Applied in this pass, in `design-brief` and `derive-baseline` only:

- A restatable sentence became its positive form only where the positive form is an exact equivalent. Four sentences changed this way.
- A boundary sentence whose reason is not stated beside it, but is stated elsewhere in the same skill or in orientation, gained that reason in a short clause. Eight sentences changed this way. No reason was invented.

Nothing else changed. No duplicate or residue sentence was edited or removed anywhere. The other nine skills and orientation are unchanged; their restatable rows below are proposals.

## Summary

| File | Words | Sentences | Boundary | Restatable | Duplicate | Residue | Not a prohibition |
|---|---|---|---|---|---|---|---|
| design-brief | 3,237 | 35 | 22 | 7 (1 applied) | 4 | 0 | 2 |
| derive-baseline | 3,600 | 37 | 21 | 8 (3 applied) | 7 | 0 | 1 |
| amend | 5,293 | 26 | 13 | 1 | 3 | 1 | 8 |
| capture | 1,140 | 7 | 3 | 3 | 1 | 0 | 0 |
| code-review | 4,250 | 27 | 12 | 8 | 4 | 0 | 3 |
| decompose | 6,398 | 36 | 12 | 3 | 15 | 0 | 6 |
| execute | 9,999 | 69 | 39 | 10 | 14 | 1 | 5 |
| mend-drift | 2,183 | 11 | 9 | 1 | 1 | 0 | 0 |
| orient | 5,177 | 34 | 9 | 1 | 14 | 3 | 7 |
| specify | 4,600 | 28 | 7 | 6 | 8 | 1 | 6 |
| upgrade | 1,419 | 12 | 7 | 0 | 4 | 0 | 1 |
| orientation | 7,778 | 75 | 41 | 10 | 5 | 0 | 19 |
| **Total** | **55,074** | **397** | **195** | **58 (4 applied)** | **80** | **6** | **58** |

Word counts are whole-file counts as of this review, before this pass's edits; orientation counts the `orientationBody` text. Sentences are counted once each, however many prohibition words they hold.

## design-brief

Boundary: 22. Restatable: 7, 1 applied. Duplicate: 4. Not a prohibition: 2.

### Applied

| Original | Now | Class |
|---|---|---|
| "Do not change canonical prose, source code, ... Git history, refs, or index." | Adds ": the request authorizes only the export." | Boundary, reason added (Trigger: "it authorizes an export, not crossing that gate"; orientation: "The request already authorizes this read-only export") |
| "Do not enter `specify`, `mend-drift`, or another lifecycle skill to repair missing or contradictory sources." | Adds ", because your work here is to expose that uncertainty." | Boundary, reason added (Reasoning: "Your work is to ... expose uncertainty") |
| "Required behavior must not become optional merely because it is inconvenient to prototype." | "Required behavior stays required even when it is inconvenient to prototype." | Restatable |
| "Do not create a mandatory output directory, YAML sidecar, ... or automatic refresh mechanism." | Adds ", since the requested deliverable is the only write permitted." | Boundary, reason added (Keep generation read-only: "Only context reads and the explicitly requested deliverable write are permitted") |
| "Do not upload, submit to an external tool, install a design tool, or begin prototype construction." | Adds ": the developer decides where the brief goes, and the receiving tool owns prototype craft." | Boundary, reason added (Hand-off: "The developer decides whether to hand it to a designer or design tool"; "The receiving tool owns prototype craft") |

### Restatable, not applied: not an exact equivalent

| Sentence | Proposed positive form | Why it was left |
|---|---|---|
| "Do not prescribe a new design methodology, component architecture, or implementation process." | "Leave design methodology, component architecture, and implementation process to the receiving designer." | Drops "new". The brief still carries the project's existing components and architecture as constraints (section 5), which the positive form would seem to hand away. |
| "Do not silently redirect the request to its replacement or revive retired screens." | "Brief the retired target as historical scope, with its replacement as context only." | Drops the two named mistakes, redirecting and reviving. |
| "Do not silently choose code over prose, newer prose over a recorded decision, or a detailed task over a conflicting parent requirement." | "Mark a conflict between code, prose, a recorded decision, and a task as Unresolved at the affected behavior." | Drops the three named precedence shortcuts. Also overlaps "Surface material source conflicts ... without inventing a precedence rule" later in the same section. |
| "Do not filter the initial descendant inventory to active or ready work, since that would hide draft and retired nodes." | "Keep every descendant, draft and retired included, in the initial inventory." | Drops the named filter. A form that says only "keep draft and retired nodes" would allow other filters the original forbids. |
| "Do not invent every conceivable state." | "Cover the states the requirements establish, and treat an important unspecified one as the next sentence says." | Drops the named over-reach. |
| "Unsupported layout choices, sample values, and provisional treatments must not read as product requirements." | "Unsupported layout choices, sample values, and provisional treatments read as suggestions, not product requirements." | The skill labels these two ways (Suggested for the prototype, and provisional), so naming one label is narrower than the original. |

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Never silently sample a subset." (Epic row of the scope table) | Same cell: "Enumerate every included feature"; Reasoning: "Keep a scope inventory ... so every included epic feature is accounted for" | Delete here. |
| "Do not silently substitute historical requirements for the developer's checkout." | Same paragraph: "Default to the current working-tree requirements." | Keep. It adds that leaving the default must be said, which the default alone does not. |
| "Do not assume an environment assignment in one shell call persists into the next." | Preceding sentence: "use `env GIT_OPTIONAL_LOCKS=0 tiller ...` on each invocation"; orientation's design-brief paragraph | Keep. It is the reason the per-invocation prefix matters. |
| "Recorded path coverage does not verify implementation, and question targets do not establish effective inherited holds" (fact) | Orientation, How to work: "These are recorded claims, not verified code coverage or effective question holds." | Delete here. |

### Boundaries with no reason found

- "Never export real customer records, credentials, secrets, or private production payloads to make examples realistic." The skill says to review for them and substitute, never why.
- "Do not copy sensitive payloads out of a source just because the source is a requirement." Same gap; it also repeats the sentence above.

## derive-baseline

Boundary: 21. Restatable: 8, 3 applied. Duplicate: 7. Not a prohibition: 1.

### Applied

| Original | Now | Class |
|---|---|---|
| "Nothing in recorded state names this arm; never start it on your own." | "Nothing in recorded state names this arm; start it only when the developer asks." | Restatable (pinned; pin moved) |
| "Do not move or remove an input before its provenance is recorded." | Adds ", because the recorded blob ref is what recovers its bytes after retirement." | Boundary, reason added (Make provenance recoverable: "`git show <blob-object-id>` recovers the original bytes even after retirement") |
| "Do not duplicate the body." | "Keep the body only in the authoritative source." | Restatable |
| "Never hand-edit `baseline.yml` or a confirmation block." | Adds "; every change to state lands through a command, so the gate records stay honest." | Boundary, reason added (orientation: "Every change to state lands through a command, so the event stream and the gate records stay honest") |
| "Do not fabricate a dropped count, including zero, from the ledger." | Adds ", which cannot know what was dropped." | Boundary, reason added (Readout: "add **Notes from this run** for what the ledger cannot know") |
| "Do not self-approve or derive duplicates to escape the baseline move the ranked list names." | Adds ": approval is a human gate." | Boundary, reason added (Recovery: "Approval is a human gate, not a command failure to work around") |
| "Do not automatically cross into decompose or another seam." | "Cross into decompose or another seam only on the human's go." | Restatable |

### Restatable, not applied: not an exact equivalent

| Sentence | Proposed positive form | Why it was left |
|---|---|---|
| "...; do not silently choose it for convenience." (coarse monolith) | "...; leave that choice to the human." | Drops the named temptation (convenience), and "silently" allows a disclosed recommendation the positive form would bar. |
| "Compare against that code, never against a new implementation that has barely begun." | "Compare against that code alone." | Drops the named mistake, the likely one in a legacy migration. |
| "Resolve `git.integration_branch` ...; do not assume `main`, `master`, or the current branch." | "...; take its name from them alone." | Drops the three named guesses. |
| "Reproduce all of it: do not shorten its coverage map, collapse the fourth source or doc into an ellipsis, or write your own replacement table." | "Reproduce all of it: every coverage-map row, in Tiller's own table." | Drops the named ellipsis and replacement-table mistakes. "The fourth source" also reads as residue from one run. |
| "Recover retired source bytes with their recorded blob refs; do not try to read them from HEAD ..., recreate the old live specs, or rerun the translation into new filenames." | "Recover retired source bytes with their recorded blob refs, and correct the pending docs in place." | Drops three named wrong routes. |

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Do not create another set merely because guide still names this seam." | Same paragraph: "If pending docs already exist, follow **Re-entry and corrections**"; Re-entry: "Do not self-approve or derive duplicates"; orientation baseline paragraph: "not another derivation" | Keep. It names the trigger that tempts the duplicate. |
| "Do not broaden a path to a parent that contains but is outside the declared root." | Same paragraph: "`describes` paths must ... lie at or beneath a declared code root." | Delete. |
| "A defect-shaped finding is a question too, never a captured bug." | Same paragraph: "Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam." | Keep the first clause; drop "never a captured bug". Both sentences are pinned, so the pin moves with it. |
| "Do not silently leave two live copies of the same specification." | Move arm: "The committed result has one live source of truth"; orientation baseline paragraph: "never silently keep duplicate live specs" | Keep here, the rule's home; the orientation copy is the one to weigh. |
| "For an imported source, use its committed `.tiller/design/...` path in `--from`, never the external original." | Reasoning: "Record provenance from the committed local import"; "A source outside this project's HEAD cannot be passed as `--from`" | Delete the "never the external original" clause. |
| "Tiller writes state and never commits it for you." (fact) | Orientation, Committing state: "Tiller never commits them for you." | Delete. |
| "Remaining unconfirmed docs still await review but do not globally block the project." (fact) | Orientation baseline paragraph: "remaining unconfirmed docs are not a global blocker." | Keep. The Hand-off states the gate's effect where the reader acts on it. |

### Boundaries with no reason found

- "Never move or retire files outside this project." The skill says imports stay source-preserving, not why.
- "Never use force." (Choreography comment on `git rm`) A forced removal would discard uncommitted edits, but the skill does not say so.

## amend

Boundary: 13. Restatable: 1. Duplicate: 3. Residue: 1. Not a prohibition: 8.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Nothing in recorded state says a reshape is due, so this seam is never a move the ranked list computes." | Orientation: "no field in the tree says a reshape is due" | Keep. The Trigger is where the reader needs it. |
| "Current ready status alone does not prove that work never started." | Orientation, Structural-change preflight: "Current ready state does not erase execution history." | Delete. |
| "A task's parent must already be materialized, never a feature staged in that batch." | Same paragraph: "`--add-task` goes only under an approved feature (... `task_under_held_feature` ...)" | Delete. The later sentence is precise and names the refusals. |

### Residue

| Sentence | General rule | Recommendation |
|---|---|---|
| "Do not restate the staged operation count or the precondition tally" (Readout) | Orientation, Readout: "Do not restate the block in prose around it." | Delete. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "Do not mix a move with cancellation, supersession, or revision of its subtree." | "Stage a move on its own, apart from any cancellation, supersession, or revision of its subtree." |

### Boundaries with no reason found

- "Do not probe with a mutation." Neither the skill nor orientation's preflight says why a trial mutation is barred (it writes append-only events).

## capture

Boundary: 3. Restatable: 3. Duplicate: 1.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Severity never routes: it sorts and filters the console's Fix board" | Orientation, bug severity paragraph, nearly word for word | Delete here. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "Do not write an implementation plan here." | "Keep the contract to the outcome; the implementation plan belongs to execution." |
| "Fixed when and Done when are conditions a test or a reader can check, never "the bug is fixed"." | "Fixed when and Done when are conditions a test or a reader can check, each naming an observable result." Drops the named bad example. |
| "Grade by impact, never by the size of the fix." | "Grade by impact alone." Drops the named mistake. |

Boundaries with no reason found: none.

## code-review

Boundary: 12. Restatable: 8. Duplicate: 4. Not a prohibition: 3.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "A review is never a suggested next move." | Preceding sentence: "... or asking what to do next triggers neither arm"; orientation: code review goes "outside the ranked list" | Delete. |
| "Style and naming are never bugs." | The lens table (unclear names land as Tweak) and "Leave out formatter or linter style" | Delete. |
| "...; it never becomes a queue position." (severity) | Orientation: "Severity never routes" | Delete the clause. |
| "Never cap the table." | Preceding sentence: "one table holding every numbered row" | Delete. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "Decide merged or in flight from Git, never from status: ..." | "Decide merged or in flight from Git alone: ..." |
| "An empty located diff means the change was not located, never that the unit changed nothing" | "An empty located diff means only that the change was not located" |
| "Grade by impact, never by the size of the fix." | "Grade by impact alone." Drops the named mistake. |
| "Never sample, never cap findings, and list every unit not reviewed under Limitations" | "Review every unit, report every finding, and list every unit not reviewed under Limitations" |
| "Existing numbers never change." | "Existing numbers stay fixed." |
| "Never recheck an unselected row." | "Recheck selected rows only." |
| "...; a later request never stages onto an earlier review's amendment." | "...; a later request opens its own amendment." |
| "...and do not stage a revision to correct that fact." (recorded departure) | "...; the recorded departure is that fact's correction, so the prose stays as it is." |

### Boundaries with no reason found

- "Never push." The skill gives no reason the disposition commit stays local.

## decompose

Boundary: 12. Restatable: 3. Duplicate: 15. Not a prohibition: 6.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "...; do not impose an all-doc confirmation requirement." | Same sentence: "One confirmed doc releases this startup gate"; orientation: "remaining unconfirmed docs are not a global blocker" | Delete the clause. |
| "Do not turn every unspecified detail into a blocking question." | The sentences on either side, in the same paragraph | Delete. |
| "...; do not invent journeys or silently fall back to subsystem-first planning." | Orientation's opening planning paragraph ("not an invented journey"; "not permanent subsystem buckets") and the readiness paragraph | Delete the sentence. |
| "...; do not drop later requirements or invent speculative future work." | Same paragraph: "Cover every known initial requirement ..., including later increments"; orientation: "retaining known later needs in later features" | Delete the clause. |
| "Shelf items are never staged on an amendment, and approving or withdrawing the amendment never triages or removes them." | Hand-off: "Shelf items you filed wait for their own triage" | Keep here, the mapping rule; the Hand-off copy can go. |
| "Record dependencies between features as state, never only as prose" | What becomes what ("both as state, never as prose alone") and the dependency completeness bar section | Delete. |
| "Started-task owners later appear as Contributors on ancestors, never as exclusive claimants." | Orientation, Ownership and Contributors | Delete. The paragraph repeats orientation's Ownership. |
| "Tiller writes the state files and never commits them; ... never needs a pull request." | Orientation, Committing state | Delete. |
| "The epic's Dependencies prose is the why behind the recorded what, never the record itself." | Same paragraph: "A dependency that lives only in `epic.md` prose reaches none of them" | Keep. It states why the bar exists. |
| "# a concrete defect the review reports goes straight onto its shelf, never staged" (comment) | "Shelf items are never staged on an amendment" | Keep. A comment at the command. |
| "Do not ask whether to open an amendment when this list already maps the feedback." | Same section: "Treat explicit feedback that maps to one of these classes as authorization"; orientation, Correction routing | Delete. |
| "A correction to a staged change on existing work is never one of them; that is withdraw and restage." | Cite every operation: "cannot be edited once staged: fixing one means withdrawing the amendment and restaging it" | Keep. The correction section is where the reader lands. |
| "..., never guess." ("accept this") | Same sentence: "translate by the node's state" | Delete "never guess". |
| "..., never the whole draft tree by default." | Same sentence: "only that feature or an explicitly named batch" | Delete the clause. |
| "..., never a bare id, ..." (Hand-off naming line) | Orientation, Node references, which asks each skill to repeat the naming half | Keep the sentence; the "never a bare id" clause can go. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "**An ordering claim is never a node.**" | "**An ordering claim becomes a dependency edge or a soft order on existing nodes.**" |
| "..., both as state, never as prose alone." | "..., both as state, with any prose beside the record." |
| "Do not defer known cross-epic edges to `specify` or the human, or merely note them in prose." | "Record every known cross-epic edge in this sweep yourself, as state." |

Boundaries with no reason found: none.

## execute

Boundary: 39. Restatable: 10. Duplicate: 14. Residue: 1. Not a prohibition: 5.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "...; run local `tiller check` around those commands to record evidence, never from inside them." | Orientation, Managed-project independence: "that command must never call Tiller back" | Keep. Its first half is execute's own step. |
| "...; never carry uncommitted work from one branch to another." | Same sentence: "Commit before every switch" | Delete the clause. |
| "Never let two builders share a worktree." | Preceding sentence: "give each parallel task a temporary worktree of its own" | Delete. |
| "Run `resume` only inside that worktree, never in the primary checkout, ..." | Same sentence: "only inside that worktree" | Keep. It names the likely wrong place. |
| "Creation and approval never assign tasks." | Orientation, Ownership: "Every new task is unclaimed, regardless of who created or approved it." | Delete. |
| "Features and epics have no exclusive owner, and claims never cascade." | Orientation, Ownership | Delete. |
| "Tiller records and verifies branch structure and never creates, switches, or merges refs: use ordinary Git ..." | Orientation's branch bullet, nearly word for word | Keep the "use ordinary Git" instruction; drop the repeated first clause. |
| "Never infer the next round's branch from an old PR." | Same section: "use those recorded results, not a guessed name" | Delete. |
| "Never ask the question only in chat, where no gate or later session can see it." | Orientation's product-decision bullet | Delete. |
| "Pure reads, riding task refs, and the absence of a warning do not establish host state." | Orientation: "Pure reads verify neither host state nor pushes." | Keep. It adds riding refs and a missing warning. |
| "...; do not start unrelated work." (reported merge) | Same sentence: "close out only the reported merge"; orientation: "it does not authorize unrelated execution" | Delete the clause. |
| "...and never adds commits to a branch after its final merge." (Choreography) | "Never commit to that branch after its final merge" | Delete the clause. |
| "Do not submit riding features separately or run epic eval/check." | Publication step 6: "Do not submit epic-riding features separately"; "There is no epic contract: ... are misuse" | Keep. The Choreography restates it at the step. |
| "Green task gates, a riding ref, or an unpublished state commit never proves the resolved review's completion." | "Five green task refs are not feature acceptance or a feature submission." | Delete. |

### Residue

| Sentence | General rule | Recommendation |
|---|---|---|
| "...; do not claim or start an attached feature." (Shelf work) | Orientation, Ownership: "Features and epics have no owner or claim"; this file: "There is no separate feature start." | Delete the clause. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "Task-grain dependencies wait for integration, never stacked task PRs." | "A task-grain dependent starts from integration once its prerequisite merges there." |
| "Those files are context, never an inherited contract or gate." | "Those files are reading context; the shelf node's own contract and gates are the ones that apply." |
| "Recording a question never stops a run by itself: stop only when ..." | "A run keeps going after it records a question: stop only when ..." |
| "...; do not assume `main` or `origin`." | "Take the integration branch and remote names from this project's configuration and Git setup alone." Drops the named guesses. |
| "Never chain check to evidence commit or the rest of the workflow with `&&`, ..." | "Run check and each evidence commit as separate commands, so a failing check still commits its evidence." |
| "A pending manual criterion must not prevent the other checks or their state commit." | "Run the other checks and commit their state even while a manual criterion is pending." |
| "Do not submit epic-riding features separately." | "At epic grain, submit the epic alone." |
| "Do not execute placeholder values." | "Substitute real values for every placeholder before running a command." |
| "A shelf PR names its own shelf reference ..., never substitutes the attached feature as the reviewed node." | "A shelf PR names its own shelf reference as the reviewed node, with any attached context beside it." |
| "Do not advance to submit with outstanding required work." | "Advance to submit only when no required work is outstanding." |

### Boundaries with no reason found

- "Never automatically delete or rename local or remote refs, force-push, or rewrite recorded task branches." No reason in the skill or orientation.
- "never commit the deletions without the new segment files" (merge closeout, and the two Choreography comments that repeat it). Neither says the old events would be lost.

## mend-drift

Boundary: 9. Restatable: 1. Duplicate: 1.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Confirmation fingerprinted working-tree bytes that may never have been committed, and it wrote no Git objects." (fact) | Orientation drift paragraph: "A confirmation may cover dirty bytes never committed" | Keep. It is the premise for the `shifted_design` reading steps beside it. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "Do not redraft it." | "Leave that mend standing: name it once in your notes as persisting, with the commit that mended it." (joins the next sentence) |

Boundaries with no reason found: none.

## orient

Boundary: 9. Restatable: 1. Duplicate: 14. Residue: 3. Not a prohibition: 7.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "You are not doing the work at the seam in this reply, and you do not cross into it either" | Same paragraph: "relay the ranked list to the developer and end the reply"; orientation, Ranked next | Delete. |
| "Pure orient never opens a review, and explicit no-publish or earlier-stop instructions control." | Orientation's execute-instruction bullet, nearly word for word | Delete. |
| "Unknown references do not become current nodes, and opening a current file does not retrieve pinned historical bytes." | Orientation, How to work: "Unknown references stay unresolved. A current file is not the historical contents of a pinned revision." | Delete. |
| "These reads require no identity and write no files, events, or persistent cache; they never run bindings or refresh guidance." | Orientation, How to work: "Each query rebuilds from local files without identity, writes, or a persistent cache" | Delete. |
| "You may say you would take a different item first and why; never rewrite the list." | Orientation, Ranked next: the whole paragraph from "Relay the list as written" to "not authorization" repeats it word for word | Delete the repeated paragraph here. |
| "The `mend-drift` seam is this skill's own addition: ... which the list never makes on its own" | Orientation: "`tiller drift` is a check the ranked list does not run on its own" | Keep here, where the list is explained; weigh the orientation copy. |
| "The `amend` seam is another such addition ...: ... never by a field in the tree." | Orientation: "no field in the tree says a reshape is due"; this file's amend bullet: "Nothing in state names this one" | Keep here; the amend bullet's copy can go. |
| "...; do not hand it to the developer." (aggregate assembly) | Same sentence: "Authorized execute does the routine branch work and aggregate assembly itself" | Delete the clause. |
| "..., never blindly regenerate duplicates." (baseline bullet) | Orientation baseline paragraph: "not another derivation" | Delete the clause. |
| "Ownership is per unit, never inherited from a feature." | Orientation, Ownership: "Features and epics have no owner or claim, and claim changes never cascade." | Delete. |
| "A draft waiting on unfinished work is no move and never a row." | This file: "work blocked by an unmet dependency ... are not rows" | Delete. |
| "...; unclaimed shelf work never does." (no-epics bullet) | This file: "an unclaimed shelf node are not rows"; orientation: "An unclaimed shelf node never enters the ranked list." | Delete the clause. |
| "Soft order arranges the render and the list ... and never gates execution." | Orientation, Correction table: "soft order never gates" | Keep. The planning paragraph reads incomplete without it. |
| "Do not expand it into concrete node rows, question rows, gates, criteria, or mutation commands." | Same paragraph: "It is counts-only" and "The queue command is the one place to enumerate a count." | Delete. |

### Residue

| Sentence | General rule | Recommendation |
|---|---|---|
| "Do not assign this work to the last finisher or historical contributors." | Orientation: "Any registered developer may assemble, check, and submit an eligible aggregate; there is no coordinator claim"; "a historical contribution assigns nothing" | Delete. |
| "Features and epics have no owner, so never present the frontier as one person's reserved work." | Same bullet: "The frontier is the project's live draft features"; orientation, Ownership | Delete. Its orientation sibling has already been removed as residue. |
| "Do not add a note that steers toward or away from a ranked row beyond saying you would take another row first and why." | "You may say you would take a different item first and why; never rewrite the list." | Delete. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "At epic grain name both the epic review and the particular feature contract needing work, never an epic eval." | "At epic grain name both the epic review and the particular feature contract needing work; an epic has no eval of its own." |

Boundaries with no reason found: none.

## specify

Boundary: 7. Restatable: 6. Duplicate: 8. Residue: 1. Not a prohibition: 6.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Do not stop between them." | Same paragraph: "one act with one output and one gate"; Choreography: "Do not hand a feature over between the contract and the tasks" | Delete here. The Choreography copy carries the reason. |
| "Project commands must run without Tiller; local Tiller may invoke them, never the reverse." | Orientation, Managed-project independence: "that command must never call Tiller back"; the pointer just before it | Delete. |
| "Do not turn planning-only CI skips into replacement Tiller checks, even optional ones." | Orientation: "invent replacement Tiller checks"; "Optional or advisory Tiller CI jobs are not an alternative" | Delete. |
| "Do not silently widen the selected feature or named batch, specify future features, reorder an accepted tree, or replan unrelated work." | Next sentence: "obtain developer direction before performing the wider reshape"; orientation: "Propose wider changes for developer direction, not as an unrequested replan" | Keep. It names the four forms where they tempt. |
| "Never record a list of paths there: ..." (preserve artifact) | Preceding sentence: "its `artifact` names ONE repo-relative file" | Keep. It says where the paths go instead. |
| "...; do not annotate every name it mentions." | Same sentence: "Cite only the claims the plan depends on" | Delete the clause. |
| "Do not hand a feature over between the contract and the tasks; the human reviews all three together." | Reasoning: "Do not stop between them" and "one act with one output and one gate" | Keep. It carries the reason. |
| "..., never guess." ("accept this") | Same sentence: "translate by the node's state" | Delete "never guess". |

### Residue

| Sentence | General rule | Recommendation |
|---|---|---|
| "Do not restate the criteria and their binding types" (Readout) | Orientation, Readout: "Do not restate the block in prose around it." | Delete. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "..., and do not add unnamed siblings." (batch) | "..., and keep the batch to the named features." |
| "...; a narrower first implementation must not silently weaken its promised acceptance." | "...; a narrower first implementation keeps its promised acceptance unless the plan says plainly that it narrows it." |
| "Every criterion binds to a check, never a hope: ..." | "Every criterion binds to a check: ..." The contrast is a flourish. |
| "Use ordinary sibling `tiller order` for preference, never a hard edge." | "Use ordinary sibling `tiller order` for preference, and a hard edge only for a true prerequisite." |
| "A criterion states one observable behavior as a rule that holds, ..., never a captured value, a golden output, or a pixel or paint budget." | "...; captured values, golden outputs, and pixel or paint budgets go inside the tests it binds to." The next sentence already says where they go. |
| "Write the list form, never several paths joined in one string:" | "Write the list form, one path per item:" |

Boundaries with no reason found: none.

## upgrade

Boundary: 7. Duplicate: 4. Not a prohibition: 1.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "`tiller join --as <you>` remains onboarding or deliberate identity selection, never the routine refresh remedy." | Orientation's maintenance bullet: "`tiller join` selects identity and is never the refresh remedy." | Keep here, the home of maintenance; the orientation line is its pointer. |
| "Do not migrate stale branches independently." | Preceding sentence: "Adoption is coordinated Git work, not independent migrations in stale clones." | Delete. |
| "# migrate and refresh this checkout; never touches the binary" (comment) | Scopes: "it never changes the installed binary" | Keep. A comment at the command. |
| "Do not continue into the next queued work." | Trigger: "never claiming, starting, specifying, approving, or the next queued work"; Hand-off: "Then stop for the developer to review and commit" | Delete. |

### Boundaries with no reason found

- "...and never bypass private-download authentication, including after a failure." The Cellar half has its reason (brew owns those files); this half has none.
- "Never downgrade the schema or newer generated guidance, erase caches to bypass a refusal, or infer unchanged files from a lost child result." The last clause's reason is in the paragraph before; the first two have none.

## orientation

Boundary: 41. Restatable: 10. Duplicate: 5. Not a prohibition: 19.

### Duplicates

| Sentence | Also stated | Recommendation |
|---|---|---|
| "Commentary is yours to add freely ahead of the block, never after it" | Same bullet: "The block closes the reply, nothing follows it" | Delete "never after it". |
| "Do not ask for another go after checks turn green or before ordinary state publication." | Preceding sentence: "Work already moving inside a seam keeps moving, including execute's draft preparation, submission-state push ..."; the execute-instruction bullet | Delete. |
| "For other mapped actions, do not ask which mutation path to use." | Next sentence: "The correction is already the developer's go for that action: carry it out without asking" | Delete. |
| "Do not regenerate duplicate docs or open a work-node amendment for that correction." (Correction table, pending baseline row) | Baseline paragraph: "not another derivation" | Keep. Table rows are read alone. |
| "You may say you would take a different item first and why; never rewrite the list." | Same paragraph: "Relay the list as written." | Keep. It pairs the permission with its limit. |

### Restatable, not yet applied

| Sentence | Proposed positive form |
|---|---|
| "A product decision only the human can make ... is recorded with `tiller add question`, never asked only in chat, where no gate and no later session can see it." | "... is recorded with `tiller add question`, where a gate and later sessions can see it, even when it is also discussed in chat." |
| "`tiller join` selects identity and is never the refresh remedy." | "`tiller join` selects identity only; the refresh is `tiller upgrade --project-only`." |
| "Exit `1` is a business refusal ...: fix the work and try again, do not escalate." | "...: fix the work and try again; escalate only under the stop rule that follows." |
| "...: an epic, feature, or task id never appears without its key path, because a bare id ..." | "...: every epic, feature, or task id appears with its key path, because a bare id ..." |
| "Never go hunting outside the project directory for Tiller documentation or schema shapes." | "Look for Tiller documentation and schema shapes only inside the project." |
| "Do not restate the block in prose around it." | "Let the block speak for itself: the notes carry only what it cannot know." Joins the next sentence; not exact. |
| "Do not skip either step: without them ..." (onboarding) | "Take both steps: without them ..." |
| "Choose move versus pointer once, never silently keep duplicate live specs." | "Choose move versus pointer once, keeping one live copy of each spec." Drops "silently". |
| "Do not ask for facts the repository can answer." | "Answer repository facts yourself by reading the repository." |
| "Do not invent an amendment for work that only extends the accepted plan." | "Work that only extends the accepted plan proceeds without an amendment." |

### Boundaries with no reason found

- "Managed projects must never depend on Tiller." The paragraph, with "Never add Tiller packages, setup hooks, ..." and "that command must never call Tiller back", says what independence requires, not why.
- "`attest`, `sync --accept-fast-forward`, and the final merge ... are never yours to run, even on instruction". The final merge's reason is in What Tiller is; nothing says why the other two hold even on instruction.
- "never delete, rename, or rewrite local or remote history to make a branch fit". No reason given.
- "Never force-push, bypass a gate, invent a PR, or commit to a review branch after its final integration merge." The force-push and post-merge commit halves have no reason.
- "Use read-only history inspection as needed, never a trial mutation." No reason given.
