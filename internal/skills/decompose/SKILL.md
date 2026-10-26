---
name: decompose
description: Use when to-be design inputs exist under .tiller/design/ (files the baseline ledger neither registers as as-is docs nor cites as sources) but the epic and feature tree does not yet reflect them, after the existing-code baseline gate is satisfied, or when the developer explicitly names decompose. Turns inputs into a draft epic and feature tree for the human to approve.
---

# decompose

## Trigger

To-be inputs exist under `.tiller/design/`, files the baseline ledger neither registers as as-is docs nor cites as sources, but the epic and feature tree does not reflect them; or a new design doc landed that the tree does not yet reflect; or the developer explicitly names decompose. Registered as-is docs describe what exists, not new work: an empty tree with only as-is docs has nothing to decompose. Report that no work is planned and stop rather than inventing epics from the baseline or from old specs.

On brownfield or legacy-migration, first inspect `.tiller/design/baseline.yml`. If no doc is confirmed, the preceding seam is `derive-baseline`, even if prose or work already exists. Existing pending docs need inspection or human confirmation, not a duplicate derivation. Do not use decompose to bypass that readiness gap. One confirmed doc releases this startup gate; do not impose an all-doc confirmation requirement.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

Read the inputs first: the design docs, the current-state layer, and the code roots named in `.tiller/tiller.yml`. Brownfield surface roots name writable code to understand before changing it. Legacy-migration reference roots name the old, read-only system whose behavior informs the new system, not code to extend. Every declared root needs an explicit `mode: surface` or `mode: reference`. Assess readiness, reason about delivery progression, then choose the tree and its prerequisites.

Read registered as-is docs as the current-state layer, their described paths, and any source/code questions before planning changes. Baseline confirmation is a human judgment about described code, not proof every source requirement survived translation. Keep intended future design separate from that account. If baseline prose needs a requested correction, reconcile it through derive-baseline's in-place protocol rather than silently rewrite the past or create competing live specs.

First, the readiness judgment. `tiller check readiness` is mechanical and confirms the inputs are present and the manifest is coherent. Whether the docs support defensible outcome boundaries and delivery order is reasoning, and it is yours. When missing product information would materially change those decisions, record a focused question rather than inventing an answer: `tiller add question "<statement>" --title "<short title>"` puts the statement and scan title on the ledger, and `--blocks <node>` names the affected work. Do not turn every unspecified detail into a blocking question. If supported boundaries and prerequisites do not depend on the answer, leave precise formats, interface conventions, test cases, and implementation choices for the relevant feature's just-in-time specification; name that deferred detail in its narrative.

Start with the documented actors, goals, workflows, and outcomes and recommend a delivery progression through the full known initial scope. Product users include developers, operators, and API consumers; this is neither a screens-first rule nor a requirement to follow literal user chronology. Propose a defensible sequence from the inputs. Ask focused questions when missing product information would materially change boundaries or order; do not invent journeys or silently fall back to subsystem-first planning.

Choose bounded epic and feature increments to express that progression. An epic is a coherent delivery increment, not the permanent home of a subsystem. Related capabilities may appear in several epics at different points in ordinary sibling order; features and tasks stay with the outcome they serve, not an arbitrary cross-epic execution sequence. Cover every known initial requirement in the draft epic/feature tree, including later increments. Leave detailed specifications, acceptance contracts, and task creation just in time; do not drop later requirements or invent speculative future work.

Interleave product outcomes with bounded platform needs. Design shared machinery with known later consumers in mind, but implement what the first consuming increment needs; locate later extensions in their later features. Workflow-specific behavior belongs with its consumer. A small foundation can precede useful work without making a substantial eventual subsystem a blanket prerequisite. Accept incremental extension and some refactoring. Concrete migration, compatibility, or throwaway-work risks can justify a bounded technical prerequisite; explain that tradeoff in the existing review rather than adding a gate.

Only then identify actual execution prerequisites. A dependency in the finished architecture need not prevent useful work from beginning. Record true prerequisites as `depends_on` and recommended build order as `tiller order`. Reconsider a boundary that creates an unnecessarily broad barrier: split meaningful, independently acceptable outcomes, not a true prerequisite or an acceptance promise. Keep one unfinished feature when no sensible independent acceptance boundary exists; a simulated experience requires explicit prototype intent. Features land at the grain where one eval contract can express their acceptance. Tasks, created during specify, may be technical implementation units with genuine technical dependencies; they need not each be independently useful to an end user. Decompose work too large for one engineer rather than sharing a task owner.

Record dependencies between features as state, never only as prose; the section below gives the known-edge completeness bar. For any feature changing existing code, including a greenfield project's own shipped code, set `change_type` explicitly rather than leaving it to default. Record known surfaces with `tiller link feature <key> --surface <path>` (repeat `--surface` for several paths) as part of laying the tree down; it writes `surface.paths` into `feature.yml`, so never hand-edit that file. A surface names an existing file or directory, with no symlink component, and cannot equal, sit beneath, or contain a declared reference root, including a root declared through an alias. Choose the actual writable paths rather than a parent directory that includes reference code. The surface tells the agent what to read, reviewers the blast radius, and the eval layer what to preserve. `tiller spec feature` requires a recorded surface for `replace` and `migrate`; `modify` may omit one when it is not yet honestly knowable. No project kind restricts those change types. Name the surface in `feature.md` and in any preserve criteria as well, as prose beside the record. Features and epics have no owner or claim. Every new task is unclaimed regardless of its creator or approver; a developer claims that individual task at execute time. Started-task owners later appear as Contributors on ancestors, never as exclusive claimants. Lay the tree down in `draft`, commit it, and stop: the draft tree is the proposal, the committed git diff is the review surface, and the human's `approve epic` is the approval. There is no separate proposal artifact, so an uncommitted tree leaves the human nothing to review. Tiller writes the state files and never commits them; committing them is yours, with plain `git`, directly on the integration branch: the tree is `.tiller/`-only state, and a diff that touches only `.tiller/` never needs a pull request.

Write the epic prose while you write the epic. `epic.md` carries the intent, scope, and definition of done, and it is the epic's only authored artifact: an epic has no acceptance contract of its own in this build. Its bar is the sum of its features' contracts, and it reaches `done` when every live feature under it is done, observed by `tiller sync` rather than asserted by anyone. So keep the epic prose at the release grain, saying what the epic is for and when it is finished, and put every criterion that has to be checked into the feature that can satisfy it.

Decomposing one lane of a wider tree needs no mechanism beyond the tree itself. Epics are the lanes: two engineers decompose different source slices into different epics, and the folder layout keeps them from colliding. A scoped lane does not scope the dependency bar away; the section below says what it asks of a lane run.

## The dependency completeness bar

The tree is not laid down until every justified known dependency is recorded as state. `depends_on` is what the frontier and every dependency-ordered read consume for prerequisites. A dependency that lives only in `epic.md` prose reaches none of them, and an incomplete graph still looks authoritative. The epic's Dependencies prose is the why behind the recorded what, never the record itself. Neither a sparse nor a dense graph proves planning quality.

Recording takes two passes, and the second one is structural, not a matter of taste. At `add` time `--depends-on <node>` can only name work that already exists, and resolution is all-or-nothing: a reference to a feature you have not created yet does not quietly drop the edge, it refuses the whole command with exit `2` and the feature is not created at all. So an edge is recordable on the add only when its target is already down, which in practice means the intra-epic ones and the occasional back-reference to an earlier epic. Every edge pointing forward, into an epic or feature you have not written yet, is impossible to record at that moment by construction.

Those edges come from a second sweep with `tiller link`, run once the whole tree exists. That sweep is the final step of recording the known dependency graph. Do not defer known cross-epic edges to `specify` or the human, or merely note them in prose. Specification may legitimately discover new prerequisites later; that does not excuse an incomplete decomposition audit now. Walk the tree when every epic and feature is down and record each remaining justified edge with `tiller link feature <key> --depends-on <target>`, repeating `--depends-on` to name several targets in one call. Re-recording an existing edge is an idempotent no-op, so the sweep is safe to re-run and safe to run wider than you think you need.

Decomposing one lane of a wider tree does not lift the bar, it splits it. Sweep everything that is on disk when your lane is down, the edges leaving your lane for a lane laid down before yours included, and name in your hand-off every edge you could not record yet along with the nodes it points at. The run that lays the last lane down sweeps the whole tree rather than only its own lane. A lane that records nothing and says nothing leaves an invisible gap; later discovery is not a substitute for this known-edge hand-off.

The bar: every prerequisite you asserted in any epic's Dependencies prose exists as a recorded edge, and no feature that cannot start until another feature lands is missing that edge. Check a node's recorded edges with `tiller show feature <key> --json`, which lists `depends_on`. Read the graph with `tiller query --from <epic-id> --follow contains,depends_on --depth 3 --json`, repeating `--from` for every epic in the sweep rather than running `show` on every feature. Follow names are a union at every hop, not ordered stages; inspect the returned canonical dependency edges and source fields, not just the matched vertices. Raise depth when following longer chains. Read diagnostics and completeness before treating the sweep as exhaustive. Zero cross-epic edges is valid when the requirements genuinely permit independence. Audit the reasoning and every known prerequisite, not a minimum edge count.

Last, record the delivery sequence already reasoned out: `tiller order epic <first> <second> ...` and `tiller order feature ...` for siblings within one epic. The soft order is intent, not constraint: it makes the tree read, and the ranked list tie-break, in the recommended order, while `depends_on` stays the prerequisite gate. Each command replaces that parent's supplied order list; partial lists are legal. A new planning request permits its proposal work, not arbitrary reordering of an accepted tree. Show proposed changes to existing work and apply human-directed corrections through their established route.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`.

## Choreography

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

Before the block, add a short **Notes from this run** list for what it cannot know: the recommended
product progression, material boundary or platform tradeoffs, how later known scope remains covered,
a richness gap judged not worth a recorded question, and, on a scoped lane, exactly which cross-epic
edges you could not record yet and who inherits the rest of the sweep. Keep it to facts about this run. In the notes and in everything else you
write to the developer, name every epic, feature, and task the way the block itself does, key path with
the id in parentheses, never a bare id: an id alone sends the developer off to look up what it points
at. Questions and commentary are yours to
add freely, with the notes ahead of the block, never after it.

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

## Hand-off

Up to the human, who reviews the committed draft tree and approves it; decompose never approves. Hand it over only once the tree is committed and the link sweep has run over everything on disk, which on a scoped lane means the edges you could reach are recorded and the ones you could not are named for the lane that finishes the tree. Approval should review the complete known graph; legitimate later discoveries use the applicable correction route, not a deferred initial audit. One vocabulary trap at this gate: `approve` green-lights a proposed plan to start, and it is the only gate verb in this build. There is no `accept`: completed work crosses to done when the human merges and `tiller sync` observes it, so when someone says "accept this", translate by the node's state, never guess. Approval is the first gate of the arc, not its end. Once the human approves the epic tree (`tiller approve epic <key>`), the next move is theirs: they choose from the frontier which feature to pull forward, and `specify` runs against only that feature or an explicitly named batch, just in time, never the whole draft tree by default. There is no craft exit: decomposition reads code to understand it and never writes any.
