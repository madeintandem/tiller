---
name: decompose
description: Use when a design document needs turning into work: to-be inputs under .tiller/design/ the tree does not yet reflect, a draft amendment that `tiller intake` opened on a later document, a document written outside the tree that critiques or redirects the work, or when the developer names decompose. Maps the document into a draft tree, or into drafts one amendment holds, and stops for the human's approve.
---

# decompose

## Trigger

Two doors lead here, and the door decides how the result is recorded.

The founding design: to-be inputs exist under `.tiller/design/`, files the baseline ledger neither registers as as-is docs nor cites as sources, but the epic and feature tree does not reflect them; or a new design doc landed that the tree does not yet reflect; or the developer explicitly names decompose. Registered as-is docs describe what exists, not new work: an empty tree with only as-is docs has nothing to decompose. Report that no work is planned and stop rather than inventing epics from the baseline or from old specs. A bare `tiller init` on a greenfield project has no design yet; `tiller intake --no-amendment <file or folder>` lands one while the project has no epics, and that landing is the same founding design.

A later document: the developer hands you a document written outside the tree that critiques or redirects the work (a code review, a product critique, user research, any prose that makes a case for change), or `tiller intake <file or folder>` has already run. It landed the document under `.tiller/design/` and opened a `draft` amendment carrying it as `Source` with nothing staged, which `tiller guide` names as this move. This door also covers a source that a forward-work question cites, once the human answers that they want it built, even on a tree with no epics yet. An amendment the developer opened by hand has no source document and is `amend`'s, not this skill's.

On brownfield or legacy-migration, first inspect `.tiller/design/baseline.yml`. If no doc is confirmed, the preceding seam is `derive-baseline`, even if prose or work already exists. Existing pending docs need inspection or human confirmation, not a duplicate derivation. Do not use decompose to bypass that readiness gap. One confirmed doc releases this startup gate; do not impose an all-doc confirmation requirement.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

Read the inputs first, and read each document whole before mapping a single line of it: a judgment call on line 40 can narrow what line 200 reads as in isolation, and a mapping made line by line never gets the benefit of that context. A folder is read as one body, since its files are related and one amendment covers them all. Read the design docs, the current-state layer, and the code roots named in `.tiller/tiller.yml`. Brownfield surface roots name writable code to understand before changing it. Legacy-migration reference roots name the old, read-only system whose behavior informs the new system, not code to extend. Every declared root needs an explicit `mode: surface` or `mode: reference`. Assess readiness, reason about delivery progression, then choose the tree and its prerequisites.

Read registered as-is docs as the current-state layer, their described paths, and any source/code questions before planning changes. Baseline confirmation is a human judgment about described code, not proof every source requirement survived translation. Keep intended future design separate from that account. If baseline prose needs a requested correction, reconcile it through derive-baseline's in-place protocol rather than silently rewrite the past or create competing live specs.

First, the readiness judgment. `tiller check readiness` is mechanical and confirms the inputs are present and the manifest is coherent. Whether the docs support defensible outcome boundaries and delivery order is reasoning, and it is yours. When missing product information would materially change those decisions, record a focused question rather than inventing an answer: `tiller add question "<statement>" --title "<short title>"` puts the statement and scan title on the ledger, and `--blocks <node>` names the affected work. Do not turn every unspecified detail into a blocking question. If supported boundaries and prerequisites do not depend on the answer, leave precise formats, interface conventions, test cases, and implementation choices for the relevant feature's just-in-time specification; name that deferred detail in its narrative.

Start with the documented actors, goals, workflows, and outcomes and recommend a delivery progression through the full known initial scope. Product users include developers, operators, and API consumers; this is neither a screens-first rule nor a requirement to follow literal user chronology. Propose a defensible sequence from the inputs. Ask focused questions when missing product information would materially change boundaries or order; do not invent journeys or silently fall back to subsystem-first planning.

Choose bounded epic and feature increments to express that progression. An epic is a coherent delivery increment, not the permanent home of a subsystem. Related capabilities may appear in several epics at different points in ordinary sibling order; features and tasks stay with the outcome they serve, not an arbitrary cross-epic execution sequence. Cover every known initial requirement in the draft epic/feature tree, including later increments. Leave detailed specifications, acceptance contracts, and task creation just in time; do not drop later requirements or invent speculative future work.

Interleave product outcomes with bounded platform needs. Design shared machinery with known later consumers in mind, but implement what the first consuming increment needs; locate later extensions in their later features. Workflow-specific behavior belongs with its consumer. A small foundation can precede useful work without making a substantial eventual subsystem a blanket prerequisite. Accept incremental extension and some refactoring. Concrete migration, compatibility, or throwaway-work risks can justify a bounded technical prerequisite; explain that tradeoff in the existing review rather than adding a gate.

## What becomes what

One set of rules maps a document onto the tree, whichever door it came through. Map every finding without silently dropping scope; a finding may justify several increments, each with an honest citation to the supporting lines. Do not manufacture distinct source claims merely to fit one finding to one node.

**Work with a statable acceptance contract becomes a feature**, under an existing epic when it serves that epic's bounded outcome, or under a new epic for a distinct outcome or delivery phase. A later increment may extend the same capability without belonging to a permanent subsystem bucket. Completion status alone does not decide placement. A missing Content-Security-Policy is a feature: its eval is crisp (headers present, script-src enforced), and it earns the specify ceremony a task never gets.

**A correction inside an existing feature's scope becomes a task on that feature.** A one-line test backstop does not earn feature ceremony; it earns a task under the feature whose acceptance it serves. Same-outcome additions may target an existing done feature or epic: completed tasks stay complete historical work, and new children reopen parent rollups through `tiller sync`, not immediately on add. Never rewrite terminal feature or task prose to make new behavior read as already shipped; genuine replacement uses the cancellation and supersession rules in `tiller guide amend`, not a history edit. A task is added only under a feature that is already approved. A new feature's tasks do not exist yet: they come from `specify` when the human pulls that feature forward.

**A judgment call the document explicitly declines to make becomes a question.** Keep or trim a legacy parser, which build scripts to prune, how to choose an append-only mechanism: questions are precisely that shape. They block work, the human answers, and the answer is the recorded decision. Turning a reviewer's open judgment into a feature or task decides something nobody decided, and the tree should not carry that as settled fact.

**A concrete defect, chore, or tweak goes straight onto its shelf.** When the document yields one, add it with `tiller add bug|chore|tweak <key> --source <citation>`, exactly as `capture` writes one: the complete contract, `--severity` on a bug, `--attach` only where it honestly came from, and the finding left `proposed` for its own triage. Shelf items are never staged on an amendment, and approving or withdrawing the amendment never triages or removes them. Load `tiller guide capture` for the contract shapes. The edge between a task and a shelf item is the same one capture draws: substantive planned work is a task or a feature, and a minor adjustment to a known defect is a tweak.

**An ordering claim is never a node.** Determine whether it is an actual execution prerequisite or merely a preferred sequence or eventual integration. Keep true prerequisites, including bounded technical work justified by concrete migration or compatibility risk; do not invent a blanket platform barrier or erase a real prerequisite to make the sequence look product-oriented. Record a true prerequisite with `tiller link <noun> <dependent> --depends-on <prerequisite>` and a preferred sequence with `tiller order`, both as state, never as prose alone. Name the distinction in the narrative.

Infer the affected progression from the document's actors, goals, and workflows, including developers, operators, and API consumers. Technical, security, and platform requests can be legitimate outcomes themselves: do not invent customer-facing scope to disguise them. On a tree that already has work, keep the proposal focused on the document and the unfinished work it affects. Identify impacts on existing work, but leave unrelated approved and completed scope intact; a broader replan or arbitrary reorder of the accepted tree needs developer direction. Ask only for missing product information that would materially change the boundaries or sequence.

## Recording a founding design

The founding design is a document `tiller init --design` or `tiller intake --no-amendment` landed while the project had no epics. It becomes a draft tree and no amendment is involved: `tiller intake --no-amendment` is refused once any epic exists (`amendment_required`), and a later document comes in through plain `tiller intake`. The human approves the tree epic by epic.

Only then identify actual execution prerequisites. A dependency in the finished architecture need not prevent useful work from beginning. Record true prerequisites as `depends_on` and recommended build order as `tiller order`. Reconsider a boundary that creates an unnecessarily broad barrier: split meaningful, independently acceptable outcomes, not a true prerequisite or an acceptance promise. Keep one unfinished feature when no sensible independent acceptance boundary exists; a simulated experience requires explicit prototype intent. Features land at the grain where one eval contract can express their acceptance. Tasks, created during specify, may be technical implementation units with genuine technical dependencies; they need not each be independently useful to an end user. Decompose work too large for one engineer rather than sharing a task owner.

Record dependencies between features as state, never only as prose; the section below gives the known-edge completeness bar. For any feature changing existing code, including a greenfield project's own shipped code, set `change_type` explicitly rather than leaving it to default. Record known surfaces with `tiller link feature <key> --surface <path>` (repeat `--surface` for several paths) as part of laying the tree down; it writes `surface.paths` into `feature.yml`, so never hand-edit that file. A surface names an existing file or directory, with no symlink component, and cannot equal, sit beneath, or contain a declared reference root, including a root declared through an alias. Choose the actual writable paths rather than a parent directory that includes reference code. The surface tells the agent what to read, reviewers the blast radius, and the eval layer what to preserve. `tiller spec feature` requires a recorded surface for `replace` and `migrate`; `modify` may omit one when it is not yet honestly knowable. No project kind restricts those change types. Name the surface in `feature.md` and in any preserve criteria as well, as prose beside the record. Features and epics have no owner or claim. Every new task is unclaimed regardless of its creator or approver; a developer claims that individual task at execute time. Started-task owners later appear as Contributors on ancestors, never as exclusive claimants. Lay the tree down in `draft`, commit it, and stop: the draft tree is the proposal, the committed git diff is the review surface, and the human's `approve epic` is the approval. There is no separate proposal artifact, so an uncommitted tree leaves the human nothing to review. Tiller writes the state files and never commits them; committing them is yours, with plain `git`, directly on the integration branch: the tree is `.tiller/`-only state, and a diff that touches only `.tiller/` never needs a pull request.

Write the epic prose while you write the epic. `epic.md` carries the intent, scope, and definition of done, and it is the epic's only authored artifact: an epic has no acceptance contract of its own in this build. Its bar is the sum of its features' contracts, and it reaches `done` when every live feature under it is done, observed by `tiller sync` rather than asserted by anyone. So keep the epic prose at the release grain, saying what the epic is for and when it is finished, and put every criterion that has to be checked into the feature that can satisfy it.

Decomposing one lane of a wider tree needs no mechanism beyond the tree itself. Epics are the lanes: two engineers decompose different source slices into different epics, and the folder layout keeps them from colliding. A scoped lane does not scope the dependency bar away; the section below says what it asks of a lane run.

### The dependency completeness bar

The tree is not laid down until every justified known dependency is recorded as state. `depends_on` is what the frontier and every dependency-ordered read consume for prerequisites. A dependency that lives only in `epic.md` prose reaches none of them, and an incomplete graph still looks authoritative. The epic's Dependencies prose is the why behind the recorded what, never the record itself. Neither a sparse nor a dense graph proves planning quality.

Recording takes two passes, and the second one is structural, not a matter of taste. At `add` time `--depends-on <node>` can only name work that already exists, and resolution is all-or-nothing: a reference to a feature you have not created yet does not quietly drop the edge, it refuses the whole command with exit `2` and the feature is not created at all. So an edge is recordable on the add only when its target is already down, which in practice means the intra-epic ones and the occasional back-reference to an earlier epic. Every edge pointing forward, into an epic or feature you have not written yet, is impossible to record at that moment by construction.

Those edges come from a second sweep with `tiller link`, run once the whole tree exists. That sweep is the final step of recording the known dependency graph. Do not defer known cross-epic edges to `specify` or the human, or merely note them in prose. Specification may legitimately discover new prerequisites later; that does not excuse an incomplete decomposition audit now. Walk the tree when every epic and feature is down and record each remaining justified edge with `tiller link feature <key> --depends-on <target>`, repeating `--depends-on` to name several targets in one call. Re-recording an existing edge is an idempotent no-op, so the sweep is safe to re-run and safe to run wider than you think you need.

Decomposing one lane of a wider tree does not lift the bar, it splits it. Sweep everything that is on disk when your lane is down, the edges leaving your lane for a lane laid down before yours included, and name in your hand-off every edge you could not record yet along with the nodes it points at. The run that lays the last lane down sweeps the whole tree rather than only its own lane. A lane that records nothing and says nothing leaves an invisible gap; later discovery is not a substitute for this known-edge hand-off.

The bar: every prerequisite you asserted in any epic's Dependencies prose exists as a recorded edge, and no feature that cannot start until another feature lands is missing that edge. Check a node's recorded edges with `tiller show feature <key> --json`, which lists `depends_on`. Read the graph with `tiller query --from <epic-id> --follow contains,depends_on --depth 3 --json`, repeating `--from` for every epic in the sweep rather than running `show` on every feature. Follow names are a union at every hop, not ordered stages; inspect the returned canonical dependency edges and source fields, not just the matched vertices. Raise depth when following longer chains. Read diagnostics and completeness before treating the sweep as exhaustive. Zero cross-epic edges is valid when the requirements genuinely permit independence. Audit the reasoning and every known prerequisite, not a minimum edge count.

Last, record the delivery sequence already reasoned out: `tiller order epic <first> <second> ...` and `tiller order feature ...` for siblings within one epic. The soft order is intent, not constraint: it makes the tree read, and the ranked list tie-break, in the recommended order, while `depends_on` stays the prerequisite gate. Each command replaces that parent's supplied order list; partial lists are legal. A new planning request permits its proposal work, not arbitrary reordering of an accepted tree. Show proposed changes to existing work and apply human-directed corrections through their established route.

## Recording an amendment

A later document is recorded as one amendment. `tiller intake <file or folder>` lands it under `.tiller/design/` (a file at `.tiller/design/<basename>`, a folder at `.tiller/design/<folder name>/` with its relative paths kept), fingerprints it, and opens a `draft` amendment whose `Source` cites it. A name already landed refuses the whole intake before a byte is written (`re-intake is a new intake: rename the document or remove the old artifact`): a changed document is a new intake, never an edit to the old one, because the old fingerprint keeps the old amendment's citations honest. Documents landed by earlier intakes stay where they are, and their citations keep opening.

**Stage each add with `tiller amend`.** `tiller amend <A> --add-epic`, `--add-feature`, and `--add-task` each create the real node at once, in the tree, held by the amendment: an epic and a feature as `draft`, a task as `proposed`. A held node is inert. Every lifecycle verb refuses it (`held_by_amendment`, naming the amendment and `tiller approve amendment`), it changes no rollup, and it appears in no approval queue and no specify frontier. It is shown on every read surface, marked as held. Stage the epic before its features; a feature under an existing epic needs no ordering. Three refusals bound where a task may go. `--add-task` only goes under an approved feature (ready, active, or done); under a draft or specced feature it refuses (`feature_not_approved`), and under a feature this same amendment holds it refuses (`task_under_held_feature`), because a new feature's tasks come from specify. A parent held by a different amendment refuses (`parent_held_elsewhere`), and so does a plain `tiller add feature` or `tiller add task` under a held parent: put the add on the amendment that holds the parent. `--add-question` is the exception to all of this. A question blocks work the moment it exists, so it stays an operation created at approval, and its `--blocks` may name a held draft.

**Cite every operation.** Every operation on an intake-minted amendment carries `--source`, free text by convention `path#Lstart-Lend` against the landed document (`.tiller/design/opus-review.md#L12-L18`); for a folder, one file per operation (`.tiller/design/specs/auth.md#L4-L9`). An operation drawn from separate passages of the same file lists each range after a comma (`.tiller/design/opus-review.md#L12-L18, #L40`). `tiller amend` warns when a value written as a citation will not resolve; an op with none refuses (`intake amendments trace every operation: pass --source citing the feedback document`). The human reading the batch follows each citation into the document to judge the mapping for themselves, so make each an honest line reference. Changes to existing work (`--cancel`, `--supersede`, `--revise`, `--move-feature`, `--correct-eval`) are staged through the same surface under the rules in `tiller guide amend`, and cannot be edited once staged: fixing one means withdrawing the amendment and restaging it, which is the human's call.

**Finish each held draft with the ordinary commands.** Write each held epic's `epic.md` and each held feature's `feature.md` narrative exactly as for a draft tree, and record dependencies and surfaces with `tiller link`, the dependency completeness bar above included: the sweep covers the held drafts together with the approved tree they depend on. A held task's `task.md` is written the same way, since approval promotes it without a second look. Correct in place: `tiller rename` for a title or key, and `tiller remove <noun> <key> --reason "<why>"` for a draft that should not be proposed. A removal marks its operation `removed` on the amendment and never deletes the entry, so the record of what was staged and the numbering of operations do not change. The human's partial acceptance is the same move: remove what they do not want, approve the rest. Ordering claims stay narrative or become `tiller link` and `tiller order` on the held nodes; the amendment has no operation for either.

**Write the narrative.** The amendment's own `.md` carries the seeded Why, What changes, and What to watch sections. Write the document's distillation there: the requested outcomes and their source coverage, why each addition extends an existing bounded outcome or forms a distinct increment, affected existing work and what remains untouched, true prerequisites separately from proposed sibling order, and any shelf items you filed from the document and where they wait. That narrative and the held drafts are what the human reads together before approving.

**Hand it over with `tiller propose amendment <A>`**, including when nothing was staged. An amendment can come out of decomposition empty, because the document held nothing to build or only shelf items, and that is a result worth recording: say so in the readout, and the human sees it as "nothing proposed from this document" with the withdraw command beside it. Propose checks that the source still matches its fingerprint, that every operation that needs a citation has one, that every staged precondition can be evaluated, and that every node the amendment holds still exists; it does not judge prose. It is idempotent, and hand-over is a signal, not a lock: staging and corrections stay legal on a proposed amendment, so a correction the human asks for needs no trip back to `draft`. The human's queue shows only proposed amendments, so the batch they approve is one that is finished being staged.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`. On the amendment route the staging surface is `tiller amend`'s own, and `tiller guide amend` reads every staging refusal: a forward reference to a key staged later, a stale precondition, the two ways out of a batch staged against a tree that has since moved. The refusals particular to this route are `artifact_exists` or the `re-intake is a new intake` message for a name already landed, cleared by renaming the document or removing the old artifact and rerunning `tiller intake`; `amendment_required` for `--no-amendment` once an epic exists, cleared by plain `tiller intake`; `source_required` for an op staged with no `--source`, cleared by staging it again with one; `held_by_amendment` for a lifecycle verb on a held draft, cleared by approving the amendment or removing the draft; and `feature_not_approved`, `task_under_held_feature`, and `parent_held_elsewhere` for a task or feature added where the rules above forbid it, each cleared by moving the add to where it belongs. `amendment_empty` is the human's refusal at `approve amendment`, and it names withdraw.

## Choreography

Inspect `git diff --cached --name-status` and the working tree before you start. Preserve unrelated staged and unstaged work, including other proposals. Stage only exact reviewed paths from this run: the landed document, authored prose, command-written state, and events. Do not stage `.tiller/` or a whole directory. Commit with that same exact path list to exclude unrelated indexed files. If a named file also carries unrelated edits, isolate authorized hunks with a reviewed index or stop; `--only` takes the whole named working-tree file.

### A founding design

Reason about progression first; then lay the tree or scoped slice down in `draft`, sweep the justified edges in, record ordinary sibling order, commit, and stop. This illustration assumes documented consumers need to inspect one result first and compare retained results later; it is not a universal product sequence.

```
tiller check readiness
tiller add epic result-foundation --title "Represent one computed result"
tiller add feature result-foundation/result-record --change-type create
tiller add epic inspect-result --title "Inspect one computed result"
tiller add feature inspect-result/read-result --change-type create
tiller add epic compare-results --title "Compare retained results"
tiller add feature compare-results/retain-results --change-type create
tiller add feature compare-results/compare-retained --change-type create
# write every epic.md (intent, scope, definition of done) and feature.md narrative.
# the first record representation covers inspection's needs, not all retention
# and comparison machinery. later known requirements stay in compare-results.
# ... include every other known initial epic and feature before the final sweep ...

# once all nodes exist, audit the whole graph and record true prerequisites:
tiller link feature inspect-result/read-result --depends-on result-foundation/result-record
tiller link feature compare-results/retain-results --depends-on result-foundation/result-record
tiller link feature compare-results/compare-retained --depends-on compare-results/retain-results
# inspection need not land before retention can begin: that preference stays soft.
# on a scoped lane, sweep every reachable edge now and name unresolved targets;
# the last lane sweeps the entire tree.
# for existing-code changes, also record known writable surfaces:
#   tiller link feature <key> --surface <existing-writable-path>
# replace and migrate require a recorded surface at spec feature;
# modify may omit one when not yet honestly knowable.
tiller query --from <foundation-epic-id> --from <inspection-epic-id> \
  --from <comparison-epic-id> --follow contains,depends_on --depth 3 --json
# inspect canonical dependency edges and diagnostics, not just vertices.
tiller show feature inspect-result/read-result --json

# record the recommendation after materialization, without turning it into gates:
tiller order epic result-foundation inspect-result compare-results
tiller order feature compare-results/retain-results compare-results/compare-retained

# Inspect the existing index and preserve unrelated staged/unstaged work.
# Review exact authored prose and command-written state/event paths, not directories.
# If a named file also has unrelated edits, stop rather than commit the whole file.
git diff --cached --name-status
git add -- <each-reviewed-proposal-and-output-path>
git diff --cached -- <same-exact-paths>
git commit --only -m "decompose: result delivery increments" -- <same-exact-paths>
# directly on the integration branch: a .tiller/-only diff needs no PR.
# stop for human review and `tiller approve epic result-foundation`.
# approval leaves features draft; the human chooses which to specify just in time.
tiller readout decompose   # reproduce this block verbatim as the reply's last content
```

### A later document

Land the document if nobody has, read it whole, map each finding by the rules above, cite every op, write the narrative, propose, and stop for the human.

```
tiller intake docs/feedback/opus-review.md --contributor robin
# lands .tiller/design/opus-review.md and opens draft amendment opus-review with
# Source set and nothing staged; read the landed document whole before mapping.
# a folder lands as .tiller/design/<folder name>/ and opens one amendment.

# a bounded secure-release increment the review names: stage the epic FIRST,
# so its features can name it as their parent in this very same amendment
tiller amend opus-review --add-epic hardening --title "Secure form release" \
  --source .tiller/design/opus-review.md#L1-9
tiller amend opus-review --add-feature hardening/csp-headers \
  --change-type create --title "CSP headers" \
  --source .tiller/design/opus-review.md#L10-15

# a statable contract serving the EXISTING form-engine epic's bounded outcome
tiller amend opus-review --add-feature form-engine/csrf-token-rotation \
  --change-type create --title "CSRF token rotation" \
  --source .tiller/design/opus-review.md#L16-21

# a correction inside the scope of an APPROVED feature: a task, held as proposed.
# Under csp-headers above it would refuse (task_under_held_feature); its tasks
# come from specify after approval.
tiller amend opus-review --add-task form-engine/certification-form-loading/csp-regression-test \
  --title "Add a CSP regression test" \
  --source .tiller/design/opus-review.md#L34-36

# a judgment call the review explicitly declines: a question, created at
# approval, not a decision you invent. --blocks may name a held draft
tiller amend opus-review --add-question "Keep or trim the legacy image parsers?" \
  --title "Legacy image parsers" \
  --blocks hardening/csp-headers \
  --source .tiller/design/opus-review.md#L48-52

# a concrete defect the review reports goes straight onto its shelf, never staged
tiller add bug blank-on-reload --title "Form is blank after reload" --severity important \
  --source .tiller/design/opus-review.md#L60-66
# replace .tiller/bugs/blank-on-reload/bug.md with the complete contract

# the held drafts are real nodes: write each epic.md and feature.md, then link
tiller link feature hardening/csp-headers --surface src/server/headers.ts
tiller link feature hardening/csp-headers --depends-on form-engine/csrf-token-rotation
# correct in place, or drop a draft the review does not support:
tiller rename feature hardening/csp-headers --title "Content security policy headers"
tiller remove feature <epic>/<feature> --reason "<why this draft is not proposed>"
# write the distilled narrative over the stub at:
#   .tiller/amendments/<A-id>-opus-review.md

tiller show amendment opus-review          # the whole batch, every op sourced and its held node
tiller propose amendment opus-review       # also when nothing was staged
git add -- <each-reviewed-document-proposal-and-output-path>
git diff --cached -- <same-exact-paths>
git commit --only -m "decompose: opus review into amendment opus-review" -- <same-exact-paths>
#   (directly on the integration branch: a .tiller/-only diff needs no PR)
tiller readout amend   # reproduce this block verbatim as the reply's last content
# STOP. the human reads the narrative and the held drafts beside the git diff. on an
# explicit instruction naming the amendment you may run the gate yourself, stating what
# it is about to apply first; otherwise hand them the exact command:
#   tiller approve amendment opus-review
# the escape is theirs the same way, when the batch is wrong or overtaken:
#   tiller withdraw amendment opus-review --reason "<why it is dropped>"
```

Record consequential product gaps as questions, not unresolved guesses hidden in prose: `tiller add question "<statement>" --title "<short title>" --blocks <node>`. Name the boundary, prerequisite, or delivery decision that the answer could change, and block only affected work. Ordinary specification detail does not become a decomposition blocker merely because it is not written yet. An open question holds its targets at `start`, `check`, and `submit` until the human answers with `tiller answer question <Q-id> --answer "<text>"` or work proceeds explicitly under the assumption (`--assume-question`).

## Readout

End by running `tiller readout decompose` and reproducing its block verbatim as the last thing in your
reply to the developer. The block has two sizes. A seam exit gets the full block. A correction inside the
approval gate (the next section) closes its reply with `tiller readout decompose --since <sha>` instead,
where `<sha>` is the commit the last block you reported named: only what changed since then, still
Tiller's rendering, still reproduced verbatim and last. The block is Tiller's, not yours: it renders the
same whoever is driving, which
is what lets a developer learn to read it once instead of learning your habits. Do not restate it in
prose around it, the completeness bar this skill spends a whole section on included: the edge count and
the cross-epic count are the block's to report, not yours to narrate again. The block closes the reply:
nothing follows it. It ends on its **Next:** ranked list, the one place the developer looks to learn
what happens next, so reproduce the list as written and never add a list of your own after it.

On the amendment route the closing command is `tiller readout amend`, with the same rules: reproduce its block verbatim and last, and keep your own account in the **Notes from this run** list ahead of it.

Before the block, add a short **Notes from this run** list for what it cannot know: the recommended
product progression, material boundary or platform tradeoffs, how later known scope remains covered,
a richness gap judged not worth a recorded question, and, on a scoped lane, exactly which cross-epic
edges you could not record yet and who inherits the rest of the sweep. Keep it to facts about this run. In the notes and in everything else you
write to the developer, name every epic, feature, and task the way the block itself does, key path with
the id in parentheses, never a bare id: an id alone sends the developer off to look up what it points
at. Questions and commentary are yours to
add freely, with the notes ahead of the block, never after it. On the amendment route the notes also say what each held draft extends or why it forms a distinct increment, which shelf items you filed from the document and which finding each came from, and when the amendment is empty, that nothing in the document became work and why.

## Correcting the draft

For any structural correction, reuse the correction rules in `.tiller/orientation.md` and the read-only feasibility preflight in `tiller guide amend` before discussing detailed placement. Check installed help and affected history, not just current status. Direct feature key changes require a draft, task-free feature; changing parents additionally requires draft source and destination epics. An eligible unchanged feature outside that window uses the reviewed `move-feature` amendment route; started descendants refuse rather than becoming replacement work. Cancellation/supersession remains for genuine scope replacement.

Features from bounded epics may interleave. `tiller check readiness` reports grouping-only projected epic cycles as nonblocking advisories with implicated features and recorded edges. Genuine prerequisite cycles still block, including explicit epic dependencies and mixed feature/epic completion deadlocks. Do not erase a valid edge or reshape a defensible product boundary merely to make epic grouping sort cleanly.

Human feedback that fits a direct proposal correction stays inside decompose and returns to the same approval gate. It is an ordinary correction to an unapproved proposal, not an amendment. Treat explicit feedback that maps to one of these classes as authorization to apply that correction:

- A display title changes with `tiller rename <noun> <node> --title "<display title>"`.
- A canonical key changes with `tiller rename <noun> <node> --key <canonical-key>` only inside its proposal window. A feature must be draft and task-free; changing parents additionally requires both epics to be draft. Other eligible unstarted feature moves use the reviewed route above.
- Epic or feature narrative changes in the human-owned `epic.md` or `feature.md`.
- A preferred sequence changes with `tiller order <epic|feature> <node> [<node> ...]`.
- A required dependency is added with `tiller link <noun> <node> --depends-on <target>` and a mistaken edge is removed with `tiller unlink <noun> <node> --depends-on <target>`.
- An unwanted unapproved node is removed with `tiller remove <noun> <node> --reason "<why this proposal is removed>"`.

Machine state changes only through these commands. Never hand-edit the node YAML to make a correction. Read a surviving corrected node back with `tiller show <noun> <node>` and the whole result with `tiller tree`, commit the corrected `.tiller/` diff, rerun `tiller readout decompose --since <sha>` naming the commit the last block you reported ("committed at `<sha>`"), and stop at the same `tiller approve epic <key>` gate. Do not ask whether to open an amendment when this list already maps the feedback.

On an amendment the same commands are legal on a held draft, with one addition: `tiller remove` marks the operation removed rather than deleting it. A correction to a staged change on existing work is never one of them; that is withdraw and restage.

## Hand-off

Up to the human, who reviews the committed draft tree and approves it; decompose never approves. Hand it over only once the tree is committed and the link sweep has run over everything on disk, which on a scoped lane means the edges you could reach are recorded and the ones you could not are named for the lane that finishes the tree. Approval should review the complete known graph; legitimate later discoveries use the applicable correction route, not a deferred initial audit. One vocabulary trap at this gate: `approve` green-lights a proposed plan to start, and it is the only gate verb in this build. There is no `accept`: completed work crosses to done when the human merges and `tiller sync` observes it, so when someone says "accept this", translate by the node's state, never guess. Approval is the first gate of the arc, not its end. Once the human approves the epic tree (`tiller approve epic <key>`), the next move is theirs: they choose from the frontier which feature to pull forward, and `specify` runs against only that feature or an explicitly named batch, just in time, never the whole draft tree by default. There is no craft exit: decomposition reads code to understand it and never writes any.


On the amendment route the hand-off is the same stop with a different gate. `tiller approve amendment` is the one act that lifts the hold, and the human runs it once for the whole document after `tiller propose amendment` hands it over: it promotes each held epic and each held task to `ready`, running the same open-question gate `approve task` runs, leaves each held feature `draft` for `specify`, and applies any cancels, supersessions, revisions, moves, and eval corrections as `tiller guide amend` describes, atomically. Every precondition and the amendment's own source validate first, and then all of it lands or none of it does. `tiller withdraw amendment` removes every node the amendment still holds. Approve and withdraw each run only on an explicit human instruction naming the amendment; otherwise hand them the exact command and stop. Shelf items you filed wait for their own triage: approving or withdrawing the amendment does not touch them.

Out to `specify` for a held feature once the amendment is approved and the human pulls it forward from the frontier, and to `execute` for a promoted task; a promoted epic takes the project's default review grain, changeable afterwards with `tiller configure`. A materialized question sits on the ledger until `tiller answer question` closes it, the same as any question does regardless of how it landed. In the narrative and in everything else you write to the developer, name every epic, feature, and task by key path with the id in parentheses, never a bare id, and name a question by its id plus its short title.
