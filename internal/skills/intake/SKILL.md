---
name: intake
description: Use when the developer hands you a document written outside the tree that critiques or redirects the work, a code review, a product critique, user research, any prose making a case for change, or when `tiller intake` has already landed one and opened an amendment with nothing staged. Reads the artifact whole, stages the explosion as epics, features, tasks, and questions behind that one amendment, every operation citing the lines it came from, and stops for the human's approve.
---

# intake

## Trigger

The developer hands you a document written outside the tree that critiques or redirects the work: a code review, a product critique, user research, any prose that makes a case for change rather than describing work already agreed. Or `tiller intake <file>` has already run, landed the document under `.tiller/feedback/`, and opened a proposed amendment carrying it as `Source` with nothing staged yet: the landing already happened, and the explosion is what is left. This trigger covers a source that a forward-work question cites, once the human answers that they want it built, even on a tree with no epics yet; stage each `add-epic` before its features.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

Land the document first if nobody has. `tiller intake <file> --contributor <id>` copies it into `.tiller/feedback/`, fingerprints the landed copy, and mints a proposed amendment whose `Source` cites that artifact. A basename intake already landed refuses re-landing (`re-intake is a new intake: rename the document or remove the old artifact`), because the old artifact's fingerprint is what keeps the old amendment's citations honest; a changed document is a new intake, never an edit to the old one.

Read the artifact whole before mapping a single line of it. A judgment call on line 40 can narrow what line 200 reads as in isolation, and a mapping made line by line as you go never gets the benefit of that context. Read start to finish, then map.

**Preflight structural feasibility before placement.** Load `tiller guide amend` and the correction rules in `.tiller/orientation.md`, then inspect installed `tiller amend --help` and `tiller rename --help`. Before proposing names, placement, or an approval sequence, read affected features, all descendant tasks, and source/destination epics with `tiller show`, node `tiller status`, and `tiller query`. Check lifecycle and prior starts, branches/reviews, accepted bases, actual dependencies, effective direct/inherited question holds, and inherited review settings. Current ready status is not proof of no execution history. Use read-only history inspection where necessary, never a trial mutation or invented `--dry-run`.

Explain the recommended product outcome, installed capability, and smallest safe alternative separately before asking product questions. Resolve repository facts without asking the developer. Eligible unchanged features use the reviewed move route below; a started descendant must produce a specific refusal, not replacement work or a weakened edge. If installed help lacks the operation, disclose that limitation before detailed negotiation. Genuine abandoned/replaced scope still uses cancellation/supersession, not relocation disguised as replacement.

Map the requested outcomes onto bounded delivery increments, then choose the operations below. Infer the affected progression from the document's actors, goals, and workflows, including developers, operators, and API consumers. Technical, security, and platform requests can be legitimate outcomes themselves: do not invent customer-facing scope to disguise them. Bound shared implementation to the consuming increment while retaining the feedback's known later requirements in later features. Map every finding without silently dropping scope; a finding may justify several increments or operations, each with an honest citation to the supporting lines. Do not manufacture distinct source claims merely to fit one finding to one operation.

Keep the proposal focused on the feedback and affected unfinished work. Identify impacts on existing work, but leave unrelated approved and completed scope intact. A broader replan or arbitrary reorder of the accepted tree needs developer direction; an intake request authorizes its sourced proposal, not approval. Ask only for missing product information that would materially change the boundaries or sequence.

**Work with a statable acceptance contract becomes a feature**, under an existing epic when it serves that epic's bounded outcome, or under a new epic for a distinct outcome or delivery phase. A later increment may extend the same capability without belonging to a permanent subsystem bucket. Completion status alone does not decide placement. A missing Content-Security-Policy is a feature: its eval is crisp (headers present, script-src enforced), and it earns the specify ceremony a task never gets. When the epic itself is new to this batch, stage its `add-epic` op first and its features afterward in the same amendment; a feature staged ahead of its own epic refuses as an unknown epic.

**A correction inside an existing feature's scope becomes a task on that feature**, landing `proposed` behind the same `approve task` gate any later-added task clears. A one-line test backstop does not earn feature ceremony; it earns a task under the feature whose acceptance it serves. Same-outcome additions may target an existing done feature or epic: completed tasks stay complete historical work, and new children reopen parent rollups through `tiller sync`, not immediately on add. Sync clears the parent's current completion/review fields and stales feature gates as appropriate; a completed coarse review must reopen before a new round starts. Cancelled or archived ancestors are not executable continuation targets. Never rewrite terminal feature/task prose to make new behavior read as already shipped; genuine replacement uses the existing amendment cancellation/supersession rules, not a history edit.

**A judgment call the feedback explicitly declines to make becomes a question.** Keep or trim a legacy parser, which build scripts to prune, how to choose an append-only mechanism: questions are precisely that shape. They block work, the human answers, and the answer is the recorded decision. Turning a reviewer's open judgment into a staged feature or task decides something nobody decided, and the tree should not carry that as settled fact.

**An ordering claim is never an op.** Determine whether it is an actual execution prerequisite or merely a preferred sequence or eventual integration. Keep true prerequisites, including bounded technical work justified by concrete migration or compatibility risk; do not invent a blanket platform barrier or erase a real prerequisite to make the sequence look product-oriented. Name the distinction in the amendment narrative. After materialization, an eligible dependent can receive `tiller link <noun> <dependent> --depends-on <prerequisite>`; preferred order uses `tiller order epic|feature|task ...` among siblings. Neither is an amendment operation; prose alone applies neither edges nor soft order.

Hard links remain subject to the existing noun-specific state and branch checks after approval. An unchanged started dependent cannot acquire or lose an edge through amendment, and `--revise` only re-pins prose. A fresh task under an active feature can receive its own legal sibling prerequisites. For unchanged relocation, use `tiller amend <A-id> --move-feature <feature-ref> --to <epic>/<feature-key> --reason "<why>" --source "<citation>"` under amend's eligibility rules, preserving identity and stopping at amendment approval. This includes eligible specced plans and accepted ready features; it does not approve their ordinary lifecycle gates. Only a draft, task-free feature between draft epics uses direct `rename --key`. Started descendants, affected open coarse reviews, or changed effective inherited prerequisites/question holds refuse a move. Do not invent replacements or state edits to evade those limits.

Bounded epic features may interleave. A projected epic ordering cycle is advisory; genuine prerequisite cycles, including explicit epic and mixed feature/epic completion deadlocks, still block. Read the diagnostic's features and edges rather than deleting valid prerequisites to improve grouped rendering. Workflow administration versus oversight is a product distinction to reason about, not a rule requiring a permanent sink epic.

Approval does give each epic it freshly materializes one default ordering fact automatically: its id is appended at the end of the existing root soft order, in this batch's staged order and in the same all-or-nothing transaction. That append preserves a partial list as partial and does not infer intended product placement. Record the proposed sequence in the narrative; carry out a human-directed `tiller order` only after materialization. It replaces the supplied parent's list, permits partial lists, and must preserve unrelated placement deliberately. Features and tasks receive no automatic soft-order entry.

**Nothing is homeless; a staged epic can parent features, but a staged feature cannot parent tasks.** `add-feature`'s epic resolves against the current tree or an `add-epic` op staged earlier in the same amendment. Stage the epic first, then its features; the reverse order refuses (`not_found`, `"<key>" names no epic in the tree, and no add-epic staged earlier on this amendment`). `add-task`'s feature must already be materialized, never staged in that batch. Preserve the source requirement in the new feature's proposal and leave its task breakdown to ordinary `specify` when the feature is pulled forward after materialization, rather than planning a speculative second amendment. `add-question`'s `--blocks` can name an existing epic, feature, or task, or any of those staged earlier in this amendment: a question can hold work that does not yet exist.

Every operation cites the lines it came from. `--source` is free text, by convention `path#Lstart-Lend` against the landed artifact (`.tiller/feedback/opus-review.md#L12-L18`). An operation drawn from separate passages of the same artifact lists each range after a comma (`.tiller/feedback/opus-review.md#L12-L18, #L40`); cite one artifact per operation. `tiller amend` warns when a value written as a citation will not resolve. Check the form before staging, because nothing edits a staged operation. The amendment `tiller intake` opened carries its own `Source`, so every op staged onto it requires one: an op with none refuses (`intake amendments trace every operation: pass --source citing the feedback document`). The citation's presence is what the CLI checks; making it an honest line reference is yours, since the human reading the batch follows it into the artifact to judge the mapping for themselves.

The amendment's own `.md` carries the narrative `tiller intake` seeded with placeholder Why, What changes, and What to watch sections. Write the feedback's distillation there: the requested outcomes and their source coverage, why each addition extends an existing bounded outcome or forms a distinct increment, affected existing work and what remains untouched, true prerequisite claims separately from proposed sibling order, and the legal post-materialization actions needed to record them. Explain any active-edge or reparent limitation rather than offering an unsupported command. That narrative and the sourced operations are what the human reads together before approving.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`. The staging surface here is `tiller amend`'s own: a forward reference to a key staged later in the same call, a stale precondition, the two ways out of a batch staged against a tree that has since moved, all read exactly as `tiller guide amend` describes, since intake stages through that same surface and invents no vocabulary of its own beyond `add-epic` and `add-question`. Two refusals belong to `intake` itself: `artifact_exists` for a basename already landed, cleared by renaming the document or removing the old artifact and re-running `tiller intake`; and `source_required` for an op staged with no `--source`, cleared by staging the same op again with one.

## Choreography

Inspect `git diff --cached --name-status` and the working tree before intake. Preserve unrelated staged and unstaged work, including other proposals. Stage only exact reviewed paths from this run: the landed feedback, authored amendment narrative, command-written amendment state, and events. Do not stage `.tiller/` or a whole directory. Commit with that same exact path list to exclude unrelated indexed files. If a named file also carries unrelated edits, isolate authorized hunks with a reviewed index or stop; `--only` takes the whole named working-tree file.

Land the document if it is not landed, read it whole, map each finding to its rule, cite every op, write the narrative, then stop for the human.

```
tiller intake docs/feedback/opus-review.md --contributor robin
# lands .tiller/feedback/opus-review.md and opens amendment opus-review with
# Source set and nothing staged; read .tiller/feedback/opus-review.md whole
# before mapping anything below

# a bounded secure-release increment the review names: stage the epic FIRST,
# so its features can name it as their parent in this very same amendment
tiller amend opus-review --add-epic hardening --title "Secure form release" \
  --source .tiller/feedback/opus-review.md#L1-9

# a statable contract under the epic just staged above: epic before feature,
# in this one amendment, is the order that makes this legal. Staged in the
# reverse order this refuses ("hardening" names no epic in the tree, and no
# add-epic staged earlier on this amendment)
tiller amend opus-review --add-feature hardening/csp-headers \
  --change-type create --title "CSP headers" \
  --source .tiller/feedback/opus-review.md#L10-15

# a statable contract serving the EXISTING form-engine epic's bounded outcome:
# use the live parent because of that outcome, not merely shared code
tiller amend opus-review --add-feature form-engine/csrf-token-rotation \
  --change-type create --title "CSRF token rotation" \
  --source .tiller/feedback/opus-review.md#L16-21

# a correction inside an existing feature's scope: a task, on the PRE-EXISTING
# feature form-engine/certification-form-loading (F-79dh3q), never on either feature
# staged above, since a new feature's task explosion is specify's own act
tiller amend opus-review --add-task form-engine/certification-form-loading/csp-regression-test \
  --title "Add a CSP regression test" \
  --source .tiller/feedback/opus-review.md#L34-36

# a judgment call the review explicitly declines: a question, not a decision
# you invent. --blocks may name a key staged earlier in this same call, at
# any grain, even the same-batch epic's own feature above
tiller amend opus-review --add-question "Keep or trim the legacy image parsers?" \
  --title "Legacy image parsers" \
  --blocks hardening/csp-headers \
  --source .tiller/feedback/opus-review.md#L48-52

# ordering claims are narrative, not ops. Show legal follow-through after
# materialization; these commands are not authorization to approve or reorder.
# If loading cannot execute acceptably until CSRF rotation is complete, loading
# is the DEPENDENT, but this edge is legal only while loading remains unstarted
# and eligible under the feature state/branch checks:
#   tiller link feature form-engine/certification-form-loading --depends-on form-engine/csrf-token-rotation
# If loading is already active or done, approval does not unlock that edge.
# Explain the limit; genuine replacement needs fresh work, not a lock bypass.
# A preferred release sequence without an execution prerequisite stays soft.
# The reviewer's intended epic order (assuming these are the relevant root
# entries) needs an explicit human-directed action after materialization:
#   tiller order epic hardening form-engine
# Preserve unrelated root entries when supplying the replacement order list.
# write the distilled narrative over the stub at:
#   .tiller/amendments/<A-id>-opus-review.md

tiller show amendment opus-review          # the whole batch, every op sourced
git add -- <each-reviewed-feedback-proposal-and-output-path>
git diff --cached -- <same-exact-paths>
git commit --only -m "intake: stage the opus review explosion" -- <same-exact-paths>
#   (directly on the integration branch: a .tiller/-only diff needs no PR)
# STOP. the human reads the narrative and the batch beside the git diff, for
# opus-review. on an explicit instruction naming the amendment you may run the
# gate yourself, stating what it is about to apply first; otherwise hand them
# the exact command:
#   tiller approve amendment opus-review
# the escape is theirs the same way, when the batch is wrong or overtaken:
#   tiller withdraw amendment opus-review --reason "<why it is dropped>"
```

## Hand-off

Up to the human. `tiller approve amendment` is the one act that materializes the epics, features, tasks, and questions this skill only ever staged, atomically: every precondition and the amendment's own source both validate first, and then all of it lands or none of it does. A staged epic and a staged feature each materialize in `draft`, a staged task in `proposed`, the same statuses `tiller add epic`, `add feature`, and `add task` land in by hand, cleared the same way; a materialized task lands unclaimed, same as one added by hand. Features and epics have no owner; amendment approval does not assign execution to its actor. Approve and withdraw each run only on an explicit human instruction naming the amendment; otherwise hand them the exact command and stop.

The same transaction gives every freshly materialized epic its default final root ordinal by appending its id to the existing root list in staged order. Existing entries and omissions do not move. An explicit intended placement still waits for `tiller order epic ...` after approval; the amendment operation set has no ordering op.

Out to `specify` for a materialized feature once the human pulls it forward from the frontier, and to `execute` for a materialized task once it clears its own `approve task`; a materialized epic clears `approve epic` before either starts, and a materialized question sits on the ledger until `tiller answer question` closes it, the same as any question does regardless of how it landed. In the narrative and in everything else you write to the developer, name every epic, feature, and task by key path with the id in parentheses, never a bare id, since an id alone sends the developer off to look up what it points at. Name a question by its id plus its short title, using the full statement when the question itself is the subject.
