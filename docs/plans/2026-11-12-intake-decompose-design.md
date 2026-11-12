# One door for documents: intake and decompose design

**Scope.** This plan changes how a document becomes work. It makes `tiller intake` the one way a document enters a project after `init`, lands every document in `.tiller/design/`, folds the `intake` skill into `decompose`, and changes what an amendment's new work is while it waits for approval: real draft nodes the amendment holds, approved with it. The owning specs are [artifact-schema.md](../spec/artifact-schema.md) (amendments, the design layer, provenance), [cli-surface.md](../spec/cli-surface.md) (`intake`, `amend`, `approve amendment`, `withdraw`, routing), [skill-set.md](../spec/skill-set.md) (`decompose`, `intake`, `amend`, `capture`), and [ui-surface.md](../spec/ui-surface.md) (Home's queue, the amendment page, held drafts on Explore and Build). This plan changes each of them.

**Status.** Designed 2026-11-11 to 2026-11-12 with JC in dialogue. Thirteen decisions are settled and listed below; the last four came from this document's first draft, where they stood as open questions and JC took the recommendation on each. Nothing is built. The section "Analysis and recommendations" covers the areas the dialogue did not reach (the store, the router, the console, drift, the schema, and the build order). It is my recommendation for how to build the settled design, from reading the code, not a set of rulings.

## Why

A developer writes one page about a feature and wants Tiller to turn it into work. On v0.23.0 there are three routes, and none of them is right.

- **Hand-place the file.** A Markdown file put under `.tiller/design/` makes decompose due and produces a draft tree. Nobody should have to write into `.tiller/` by hand, and the route only works on an empty tree: `decomposeCandidates` returns nothing once any epic exists, so a second document placed there later is never noticed. The CLI spec says as much: "A provenance-recording re-import that preserves a human-edited manifest remains unbuilt."
- **`tiller init --design`.** It works, once, at scaffold time.
- **`tiller intake`.** It takes a file from anywhere and opens an amendment, which is the right door. Four things are wrong behind it.
  - After intake, bare `tiller guide` tells the human to approve an amendment with nothing in it ("Its staged changes await your approval") and says no agent move is legal. The router has no move for an amendment that has a source and has not been decomposed. Only intake's own output names the next step.
  - A new epic staged in an amendment lands as a draft when the amendment is approved, and then needs `approve epic`. The human approves the same pieces twice.
  - A staged add is a key, a title, a change type, and a citation. The epic's narrative is never written, the intake skill never mentions surfaces, dependencies are a by-hand `tiller link` after approval, and a staged operation cannot be edited or dropped, so fixing a proposal means withdrawing it and staging everything again. A draft tree gives the human all of that before approval.
  - The intake skill never mentions the shelves, so a defect found in a review becomes a feature or a task.

Underneath those is a matrix nobody chose: three ways a document arrives, two approval regimes, two landing folders, two skills that teach the same judgment. JC's framing on 2026-11-11 was that the content of the incoming file is immaterial and the agent's action is always the same, so the matrix should collapse.

## Goals

- One command brings a document in after `init`, from anywhere on disk, as a file or a folder.
- Nobody writes into `.tiller/` by hand.
- `tiller guide` always names the next move after a document lands.
- The agent does one thing with a landed document, taught by one skill.
- The human approves the shape of new work once.
- Before approving, the human sees what a draft tree shows: narratives, dependencies, surfaces, and a proposal that can be corrected in place.

## Non-goals

- Editing or dropping a staged cancel, supersede, revise, move, or eval correction. Those keep the withdraw-and-restage rule.
- Re-intake of a changed document. A basename already landed is still refused.
- Approving one held piece ahead of its amendment (decision 7).
- Amendment operations for bugs, chores, and tweaks (decision 6).
- Moving or renaming existing files under `.tiller/feedback/` (decision 3).
- A record of which founding documents have been decomposed. The empty-tree rule is enough for them (decision 2).

## Anti-goals

- **A second approval of the same decision.** Wherever the human has seen a piece in full and approved the unit that carries it, no further gate asks again.
- **A draft that acts before it is approved.** A held draft changes no rollup, enters no queue, and takes no lifecycle step until its amendment is approved.
- **A route the guide cannot see.** Every state a document can be in has a named next move.
- **A regime the user has to choose by judging content.** The door picks it.

## The model

Every dimension follows from one distinction: the founding design, or a later document.

| | The founding design | Every later document |
|---|---|---|
| Arrives by | `tiller init --design`, or `tiller intake --no-amendment` while the project has no epics | `tiller intake <file or folder>` |
| Lands in | `.tiller/design/` | `.tiller/design/` |
| The agent | runs `decompose` | runs `decompose` |
| Becomes | a draft tree | drafts held by one amendment, plus staged changes to existing work |
| The human approves | epic by epic | the amendment, once |

Bugs, chores, and tweaks found in either kind of document go straight onto their shelves and are triaged on their own.

## Settled decisions

1. **The door picks the route.** Documents that arrive with `init` are decomposed into a draft tree. Documents that arrive through `tiller intake` are decomposed into an amendment by default, on every project kind, whether or not the document turns out to contain work. This was JC's counterproposal on 2026-11-11 and it replaced my first proposal, which chose the route from what the document touched (drafts for new work, an amendment only for changes to existing work). His rule is known when the document lands, gives every document one tracking unit with one approval and one withdraw, and makes the Amendments ledger the record of every document-driven change after the first plan.

2. **`--no-amendment` is the founding design, and only while there are no epics.** `tiller intake --no-amendment <file or folder>` lands the document and opens no amendment; decompose then lays a draft tree, as it does after `init --design`. It is for a greenfield project that ran a bare `init`, or for a large to-be design brought to a brownfield or migration project before any work exists. Once any epic exists the flag is refused, naming plain `tiller intake`. The draft route keeps no record of which documents have been decomposed, and the router's existing empty-tree rule is enough for a founding design, so nothing new is built for it.

3. **Everything lands in `.tiller/design/`.** Intake stops writing to `.tiller/feedback/`. JC asked on 2026-11-11 why the two shelves were split at all, and nothing the split does depends on the folder: Tiller knows a document is an amendment's source from the amendment record, and the fingerprint check follows the stored path. Existing files under `.tiller/feedback/` stay where they are; their amendments store their own source paths, so nothing migrates and old citations keep opening. A name that already exists in `design/` is refused, as intake refuses a collision today. One gain beyond tidiness: readiness reports "design/ is empty" until something lands there, and with two shelves a greenfield project begun with a bare `init` and a plain `intake` would report that forever.

4. **A folder opens one amendment per intake call.** `tiller intake ./specs` lands every file and opens one amendment that covers all of them. Each staged operation still cites one file and its lines. One intake is one unit, with one approval and one withdraw, and the agent has to read a folder of related documents together to map them properly. A large folder on a project that already has work becomes one large amendment; that is accepted.

5. **One skill, `decompose`.** It carries one set of rules for what becomes an epic, a feature, a task, a question, or a shelf item, and two ways to record the result: a draft tree for the founding design, and an amendment for a later document. The `intake` skill is retired, and `intake` remains only as the command a person runs. Today the rule for what counts as a feature is written in two skills, and a third (`capture`) covers shelf items, which lets them drift apart. The cost is a rewrite of the skill text, an orientation generation bump (so projects re-join), and a skill count that drops from eleven to ten.

6. **Bugs, chores, and tweaks go straight onto the shelves.** When a document yields one, the agent adds it as a proposed shelf item citing the document, as `capture` does, and it waits for its own triage approval. There are no amendment operations for shelf items. Triage is a different decision from approving the shape of work, and a bug's write-up has to exist before anyone can triage it. This is an accepted exception to decision 4's "one unit": approving the amendment does not triage those items, and withdrawing it leaves them to be removed one at a time.

7. **An amendment's new work is real drafts it holds.** Staging an added epic, feature, or task creates the node at once, in the tree, inert, marked as held by its amendment. The agent then uses the ordinary commands on it: it writes the narrative, links dependencies and surfaces, renames it, or removes it. Approving the amendment lifts the hold; withdrawing the amendment removes every node it still holds.
   - **The hold.** No lifecycle command moves a held draft forward. `approve epic`, `spec feature`, `approve feature`, and `approve task` on a held node are refused with a blocked answer that names the amendment and `tiller approve amendment <key>`, in the shape the question hold already uses. The hold has to cover features as well as epics: a held draft feature under an epic that is already approved could otherwise be specified and plan-approved while its amendment waits.
   - **Corrections stay open.** Rename, remove, link, and prose edits are legal on a held draft. Removing one marks its operation on the amendment as removed, with the reason; the entry stays on the list, so the record of what was staged and the numbering of operations do not change.
   - **Partial acceptance** is removing the drafts you do not want and approving the rest.
   - **One stated rule changes.** "Staging executes nothing" becomes "staging creates inert drafts; only approval moves anything."
   - JC chose this on 2026-11-12 over two alternatives: keeping staged adds thin with an agent follow-up after approval, and adding narrative, dependency, and surface fields to each staged operation. Both left the human approving less than a draft tree shows, or duplicated commands that already exist.

8. **Approving an amendment approves its new epics.** A held epic goes from draft to ready with its amendment; there is no second `approve epic`. `approve epic` is only that status change plus an optional `--review-grain`, so an epic added this way takes the project's default grain, changeable afterwards with `tiller configure`.

9. **Approving an amendment approves the tasks it adds to an approved feature.** A held task goes from proposed to ready with its amendment. Under decision 7 its `task.md` exists and is read before approval, so a separate `approve task` would repeat the decision. A held feature is different: it stays a draft when the hold lifts and goes through specify and its own plan approval, because its spec, contract, and tasks do not exist when the shape is approved.

10. **An amendment is handed over.** A new amendment opens as `draft` while the agent works on it, whether `tiller intake` or `tiller add amendment` opened it. When the agent has finished, `tiller propose amendment <A>` validates it and moves it to `proposed`, as `tiller spec feature` hands a feature over. The states are `draft`, `proposed`, then `approved` or `withdrawn`.
    - **Why it is needed.** An amendment can come out of decomposition empty, because the document held nothing to build or only shelf items, so "has no operations" cannot mean "not yet decomposed".
    - **The router** offers the agent a move for a draft amendment (decompose when it has a source, `amend` when a developer opened it by hand) and offers the human approve only for a proposed one. The queue no longer asks for approval of a batch that is still being staged.
    - **An empty proposed amendment** reads to the human as "nothing proposed from this document" and carries the withdraw command. `approve amendment` refuses an amendment with nothing in it and names withdraw.
    - **Hand-over is a signal, not a lock.** Staging and corrections stay legal on a proposed amendment, so a correction the human asks for needs no trip back to `draft`. Approval validates everything again in any case. `propose` is idempotent.
    - **What `propose` checks:** the source still matches its fingerprint, every operation that needs a citation has one, every staged precondition can be evaluated, and every node the amendment holds still exists. It does not judge prose.
    - **Withdraw** is legal from `draft` and from `proposed`. A key is refused while another amendment in either state holds it.
    - Amendments that are `proposed` when a project migrates stay `proposed`.

11. **A folder lands under its own name.** `tiller intake ./specs` lands as `.tiller/design/specs/` with its relative paths kept, in both intake modes. A file lands at `.tiller/design/<basename>`, as now. Any name that already exists refuses the whole intake before a byte is written. `init --design` keeps copying a folder's contents into the root of `design/`; on a fresh scaffold there is nothing there to collide with.

12. **Amendments staged before the migration keep the old rule.** Their add operations carry no node id. They are created at approval, land as a draft or a proposed task, and pass the ordinary gates afterwards, so decisions 8 and 9 do not apply to them. That is what their reviewer was told, and it keeps the migration from minting nodes. `show amendment` says which rule an amendment is under.

13. **A staged question stays staged.** `--add-question` remains an operation created at approval. A question blocks work the moment it exists, so creating it at staging would hold real nodes on the strength of an unapproved proposal. Its `--blocks` can name a held draft directly.

## Analysis and recommendations

Everything in this section is mine. I read the two specs, the skills, the intake command, the amendment staging, approval, and withdraw code, the removal code, the rollup in sync, the queue, and the router. I did not run a prototype.

### Held drafts in the store

**How a draft is marked as held.** I recommend deriving it. Each add operation on an amendment records the id of the node it created (`node: E-j8m4q2`), and the tree loader builds one index from the add operations of every amendment that is still `draft` or `proposed`. There is no new field on `epic.yml`, `feature.yml`, or `task.yml`. The amendment stays the one place that says what it holds, and the hold lifts everywhere in the same write that marks the amendment approved, which is already the last write of an approval. The operation needs the node id in any case, because a held draft can be renamed and its key on the operation would then be stale. The alternative, a `held_by` field on each node, makes every node self-describing but adds a second copy of the fact and a cleanup write on every held feature at approval.

Two naming and scoping notes for the build. The codebase already uses `Held` for a node under a retired ancestor (`gen.Held`, the `held` field the console reads), so this concept needs its own name there; the implementation plan uses `AmendmentHold`. And an add staged under a parent that a different open amendment holds should be refused, naming that amendment: otherwise approving or withdrawing one amendment would move or delete nodes the other is still proposing.

**Where "inert" has to be enforced.** A draft is not inert by default today. These are the places I found where a draft or proposed node has an effect, and each needs the same held check:

- `workLandedUnderEpic` and `workLandedUnderFeature` in `internal/core/mutate/sync.go`. Any feature that is not done reopens a done epic on the next sync, and any task that is not done reopens a done feature. A held draft must not, or an unapproved proposal would reopen finished work.
- `queueApproveEpics` and `queueApproveTaskBatches` in `internal/views/queue.go`. A held node's approval belongs to its amendment's queue entry, not its own.
- `specifyCandidates` and the frontier it reads in `internal/views/next_enumerate.go`. A held feature under an approved epic must not be offered for specify.
- The lifecycle transitions in `internal/core/mutate/transition.go` (decision 7's refusals).

I recommend one predicate in `internal/gen`, used by all four, with a test that fails if a new consumer of draft status skips it. Display surfaces are the opposite case: the tree, `ls`, `status`, Explore, Build, and the map should all show held drafts, marked.

**Approval.** Approval today plans every add in memory and writes it in one journaled transaction. With held drafts the adds are already on disk, so approval gets simpler: it validates, flips each held epic to ready, runs each held task through the same code path `approve task` uses (so its `task.md` is pinned as `accepted_against`, or the `active_basis` guard would misread it later), applies the cancels, supersessions, revisions, moves, and eval corrections as now, and writes the amendment's status last. A process killed mid-way leaves some nodes promoted under an amendment still open; approving again must complete it, with promotion idempotent. The root order append for new epics can stay at approval.

**Withdraw.** Withdrawing has to remove the held nodes. `RemoveDraft` in `internal/core/mutate/remove.go` already plans a removal with its references: questions that block the node, the root order, shelf items attached to it. Its shape rules are satisfied by construction, since a held feature cannot be specified and so has no tasks. I recommend withdraw call that planner for every held node inside its own transaction and record one event naming what was removed. A shelf item that cited a removed node keeps its citation to the document and loses only its attachment. One rule in that planner has to bend: it refuses to remove a node that a proposed amendment references, and a held draft's own amendment must be the exception, both for withdraw and for removing a single held draft by hand. The refusal should still stand when any other operation names the node, such as a cancel that lists it as the replacement, because staged changes to existing work cannot be edited.

**The staging command.** I recommend keeping `tiller amend <A> --add-epic`, `--add-feature`, and `--add-task` as the way to create held drafts, with no new flag on `tiller add`. The amendment surface keeps its shape, and the rule that every operation on an intake-minted amendment carries `--source` stays where it is enforced now.

### The intake command

- **Argument.** One path, a regular file or a folder.
- **Landing.** Decision 11. The key and title that default from a file's basename default from the folder's name.
- **Fingerprint.** The amendment's `source` stays a single entry. For a folder it names the folder and carries the fingerprint of each file inside it, each computed the way intake fingerprints one file today (`WorktreeBlobHash`). A changed, added, or removed file then reads as stale, and the report can say which one. This keeps one source per amendment. The baseline ledger's directory fingerprint is the nearer idiom, but its helper (`vcs.WorktreeSurface`) refuses any path inside `.tiller/` on purpose, so it cannot be reused here as it stands.
- **`--no-amendment`.** It copies and stops. It is refused when any epic exists. I recommend it record an event with the source path and time and leave the manifest alone, since the manifest is human-owned after its first scaffold.

### The router

- **A new agent move** in the amendment arm, for a draft amendment (decision 10): decompose when it has a source, `amend` when it was opened by hand.
- **`queueApproveAmendments` lists only proposed amendments,** and gives an empty one the withdraw command. Today it lists every open amendment, including one opened a second ago.
- **`loadToBeDesign` skips any document an amendment names as its source,** or a later document landed on an empty tree would make both routes due at once (decision 3).
- **The "no work planned" hint** names `intake` and `capture` only. A file placed under `.tiller/design/` by hand on an empty tree still makes decompose due, because Tiller cannot tell it from one `init` landed, but it is no longer advertised.
- **Order.** On brownfield and migration projects the baseline arm already outranks the amendment arm, so a document landed before the baseline is confirmed waits for it. That is right: decompose reads the as-is docs.

### The skill

I recommend three parts, so a one-page feature idea does not load the founding-design rules: the shared mapping rules (what becomes an epic, a feature, a task, a question, a shelf item; an ordering claim is never a node); recording a founding design (today's decompose choreography); and recording an amendment (stage the adds, finish them with the ordinary commands, stage changes to existing work through the `amend` skill's surface, then `tiller propose amendment`). The mapping rules now in `intake` and the shelf rules now in `capture` move into the first part, and `capture` keeps its own trigger for a defect found outside any document. `orient` and the generated orientation change with it.

### The console

No new page is needed.

- **Home.** The amendment keeps its one decision card at the head of the queue. Held drafts and held tasks do not appear as their own approval rows.
- **The Amendments ledger** gains a Draft tab beside Proposed, Approved, and Withdrawn, and a draft amendment's page says the agent is still working on it.
- **The amendment page.** Each add operation already has a `created` link in its view model, filled after approval. It can be filled from staging on, so the page lists the amendment's drafts and each one opens.
- **Build, Explore, the map, and the node header.** A held draft shows where its status puts it (the Draft column, the tree, the map) with a flag naming its amendment. The UI spec already treats "held" as a condition drawn on the card, never a column, for question holds, and this follows it. The flag needs a text label, not color alone.
- **Counts.** I recommend held drafts count as drafts in Home's tiles and in `status`. They are drafts. Report's burn-up counts approved scope and is unaffected, and its epic progress chart already has a "not yet approved" band, which is where held drafts belong.
- **API.** Node views gain the holding amendment's id, key, and title.

### Drift

No new marker is needed.

- `active_basis` sweeps ready and active features and approved tasks. Held nodes are neither until approval, and promotion through the `approve task` path gives a promoted task the basis the sweep expects.
- `missing_surface` sweeps non-terminal features that carry surfaces, which now includes a held feature whose surfaces the agent has linked. Reporting a vanished path there is useful, and the fix is the same correction as on any draft.
- `stale_spec`, `answered_question`, and `orphan_eval` do not reach held drafts.
- `stale_design` and `shifted_design` read only the baseline ledger, so landing later documents in `design/` does not touch them.
- A landed document edited after intake is already caught: `show amendment` reports the source as stale and approval refuses. With every document now beside editable prose in `design/` that check matters more, and it is enough.

### Schema and migration

- **Schema 14.** Three things change on disk: an add operation gains a node id, an amendment's source may name a folder with a fingerprint for each file, and an amendment gains the `draft` status. An older binary would treat a held draft as an ordinary draft, approve it on its own, and let it reopen a done epic, so the stamp has to move and older binaries have to refuse the tree. The distribution spec treats an additive field as needing a migration for the same reason.
- **The migration is stamp-only.** It changes the manifest stamp and records the migration event. It moves no file and rewrites no amendment.
- **Amendments staged before the migration.** Decision 12. The old approval path stays in the binary for them and can be removed once no project carries one.
- **`.tiller/feedback/`** is left alone. `init` never created it; intake made it on first use, and now stops.
- **Orientation generation** bumps once, with the skill.

### Build order and size

I recommend one release with one schema bump and one generation bump, built in three steps on one branch so each can be reviewed on its own.

1. **Landing and routing.** Intake lands in `design/`, accepts a folder, and fingerprints it; `--no-amendment`; the to-be detection rule; the `draft` status and `tiller propose amendment`; the router move and the queue rule; the hint. It is medium, and it is the step that removes the hand-placed route and the empty-approval prompt.
2. **Held drafts.** Staging creates nodes; the hold and its refusals; the held check in rollups, queues, and the frontier; the withdraw cascade; promotion at approval; the console flags. It is the large step. The risk sits in `amendment_approve.go` (about 2,250 lines), `amendment.go`, `remove.go`, and the rollup in `sync.go`, and I have read those less closely than the rest.
3. **One skill.** The merged `decompose`, the shelf routing, `orient`, the specs, the command reference, and the site's skills page. It is medium and mostly prose, and it comes last because the skill has to describe step 2's behavior.

**Verification.** Each of these is an arc driven through the real CLI in a disposable project:

- a founding design through `init --design`, and through `intake --no-amendment`;
- `--no-amendment` refused once an epic exists;
- a one-page idea on a brownfield project with a confirmed baseline and no epics: the guide names decompose, the amendment holds one epic and one feature, approval leaves the epic ready and the feature a draft, and specify is next;
- a document that adds a feature and cancels the work it replaces;
- each hold refusal, and a held draft under a done epic that does not reopen it on sync;
- removing one held draft and approving the rest;
- withdrawing an amendment and finding its drafts gone, with questions and shelf attachments detached;
- a folder intake that reads stale after one file changes;
- a document with nothing to build: the amendment is proposed empty, approval refuses it, and the queue offers withdraw;
- the queue staying silent about a draft amendment until `propose`;
- an amendment staged before the migration, approved after it.

## Touch points

- `internal/cli/intake.go`: the landing folder, folder sources, `--no-amendment`, key and title defaults for a folder.
- `internal/cli/init.go`: the copy it shares with intake.
- `internal/store/amendment.go`: the `draft` status; the node id on add operations; a folder source with its per-file fingerprints; validation.
- `internal/store/tobe_design.go` and `internal/store/tree.go`: skipping amendment sources; the held index.
- `internal/store/manifest.go`: `CurrentSchemaVersion = 14` and the registered migration.
- `internal/core/mutate/amendment.go`: amendments open as `draft`; staging creates the node; `propose`; withdraw removes held nodes.
- `internal/cli`: the `propose` verb and its entry in the command reference.
- `internal/core/mutate/amendment_approve.go`: promotion in place of materialization; the path kept for amendments staged before the migration.
- `internal/core/mutate/amendment_validity.go`: source validity for a folder.
- `internal/core/mutate/transition.go`: the hold refusals; task promotion through the `approve task` path.
- `internal/core/mutate/remove.go`: removing a held draft drops it from its amendment.
- `internal/core/mutate/sync.go`: the held check in both reopen tests.
- `internal/gen`: the held predicate.
- `internal/views/queue.go`, `next_enumerate.go`, `zero_day.go`, `readout.go`, `nodepage.go`: the queue rules, the decompose move, the hint, the readout, the held flag and the amendment page's links.
- `ui/src`: the held flag on Build cards, the Explore tree, the map, and the node header; the amendment page; the ledger's Draft tab.
- `internal/skills/decompose`, `intake` (removed), `capture`, `amend`, `orient`; the generated orientation; the skills tests.
- Specs: artifact-schema.md, cli-surface.md, skill-set.md, ui-surface.md, distribution.md (schema 14).
- `docs/tiller-commands.md` (the site's `cli.md` is generated from it) and the site's skills and console pages.

## Open

None. JC settled all thirteen decisions.

Deferred, with no decision asked for: editing or dropping a staged change to existing work; re-intake of a changed document; approving one held piece early; removing the pre-migration approval path.
