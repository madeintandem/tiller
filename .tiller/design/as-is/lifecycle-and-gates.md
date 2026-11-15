# Lifecycle and gates: the rules that move work

This is a pointer page. The contract lives in `docs/spec/artifact-schema.md` (lifecycles, gates, the eval model, amendments, questions, drift) and in the behavior sections of `docs/spec/cli-surface.md`. This page says which sections apply, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0, schema 15).

## What this surface is

The core that decides whether a change to the work tree is legal, applies it, and derives the facts every surface reads back. It never prints and never parses argv; the CLI and the console call it. Its parts:

- `internal/core/lifecycle` is the pure state machine: one transition table per noun and one verb-to-actor table (`table.go`), with `Legal`, `NextMoves`, `Apply`, and the gate-staling helpers.
- `internal/core/mutate` holds the state-changing operations (all but init's manifest write, join's identity write, and usage capture, which live in `internal/cli` and `internal/store`): add, approve and the other transitions, claim and transfer, check, eval, attest, submit, sync, questions, amendments (stage, propose, approve, withdraw), link and unlink, rename, remove, order, configure, resolve, grade, the design baseline verbs, review recovery, and also event compaction and schema migration. Each validates against a loaded tree, plans its files and its events, and commits them through the store. Most commit one transaction per command; `sync` commits one per crossing or reopening, so a sync that fails partway keeps every earlier crossing; compaction commits one per day and records no event; and `check` and `eval` write evidence files before and outside the transaction.
- `internal/core/gates` runs one command gate and captures its evidence, evaluates acceptance criteria, derives the `evals` gate status, and builds and validates the rubric handoff.
- `internal/core/drift` is the read-only detector for the seven drift markers.
- `internal/core/readiness` is the readiness gate and the dependency-cycle diagnostics.
- `internal/core/resolve` turns an id or key plus its noun into exactly one node.
- `internal/core/review` reports a submitted task whose branch carries commits its pushed counterpart lacks.
- `internal/workflow` classifies shared closeout for a reviewed feature or epic into one piece of advice (phase, actor, action, command).
- The projection half of `internal/gen`: dependency order and soft order (`order.go`, `order_cycles.go`), cycle classification (`execution_cycles.go`), derived activity and board status (`activity.go`, `workitem.go`), the held-by-retired-ancestor and held-by-amendment predicates (`held.go`, `amendment_hold.go`), operative question holds (`question_blockers.go`), claim eligibility and execution preference (`preference.go`), the "key path (id)" label (`label.go`), and tree and prose path derivation (`tree.go`, `prose.go`).
- `internal/readinput` gathers the local observations pure read projections need (parsed contracts, basis and assembly checks, recent events, observed execution, the relationship index inputs). `internal/evalref` is one predicate, `IsPathShaped`, used by drift and the relationship index.

Two things a reader would expect here live elsewhere. The ranked next engine (`views.NextOf`, `internal/views/next*.go`) and the Queue projection (`internal/views/queue.go`) belong to the console and read-models page; this page covers what they consume. Git observation (ancestry, patch comparison, code identity, the working-tree fingerprint) is `internal/vcs`.

## Data it owns

No persisted format of its own. It writes the shapes the store defines (see the store and events page) and the event stream. The one thing it writes outside `.tiller/` is evidence: command output under `artifacts/tiller/gates/` and retained rubric output under `artifacts/tiller/evals/`, both named by the SHA-256 of their bytes (`docs/spec/artifact-schema.md`, "Gates", "The stored gate block", and "The eval model", "Retained rubric evidence").

Compiled-in data: the transition tables and actor table, the list of drift markers, the nine amendment operation kinds and five precondition names, and the event type names. In-memory and per process: the dependency `Order` computed once per loaded tree, a bounded cache of parsed contracts keyed by content digest (`internal/readinput/contract_cache.go`), and a bounded cache of recovered-review evidence checks that passed (`internal/core/mutate/recovered_evidence_cache.go`). Both caches are documented in `docs/spec/ui-surface.md`, "Liveness", "Read cost". Derived facts (an epic's `active`, a feature's `awaiting check`, a shelf node's `carried`, every hold) are computed on read and never stored.

## Behavior to preserve

States and who drives them. `docs/spec/artifact-schema.md`, "Lifecycles, consolidated", and the "States" paragraphs under "Epics", "Features", "Tasks", "Bugs, chores, and tweaks", and "Amendments"; `docs/spec/cli-surface.md`, "The actor model", "Who may run what", and "The done crossing". The table in `internal/core/lifecycle/table.go` matches them: the done crossing is the `sync` verb, a feature's `active` and every rollup `done` are written by mutate and not by a table row, and amendment status writes bypass the engine. Only `sync` writes `done` and `done_basis`. For epic, feature, task, and shelf nouns the engine treats a verb whose target equals the current state as legal and moves no status, but the verb's guards still run first, so a retried `start` can still refuse on a moved basis, an open question, or another contributor's claim, and `check` re-runs its gates and writes. Amendment `approve` and `withdraw` are not idempotent (a second one refuses `amendment_not_open`); only `propose` is.

Guards before work moves. Parent plan gate, retired ancestors, the active basis, open questions, assumptions, and held drafts: `docs/spec/cli-surface.md`, "Gates, evals, and attestation" (the paragraph beginning "Before `start`, `check`, or `submit`"), "Specifying a feature", and "The work loop"; `docs/spec/artifact-schema.md`, "Features" ("Accepted basis", "Assumption basis", "Branches and activation") and "Decisions and open questions". Ownership, claims, and transfer: "The root manifest" (the ownership paragraphs) and "Bugs, chores, and tweaks" ("Transfer").

Gates, evals, attestation. `docs/spec/artifact-schema.md`, "The eval model" and "Gates" ("The stored gate block", "Gate reuse", "Code identity"); `docs/spec/cli-surface.md`, "Gates, evals, and attestation". Enforcement is advisory: a human merge of red work records `done` plus `policy_violations`.

Sync, rollups, review grain. `docs/spec/cli-surface.md`, "The done crossing", "Review grain", "Recovering an aggregate review recorded after merge", and "Observing merges and reading holds: sync, status, ls"; `docs/spec/artifact-schema.md`, "Epics" ("Review grain"), "Features" ("Done basis", "States", "Merged aggregate review recovery"). The squash fallback scans the newest fifty first-parent commits (`contentScanLimit` in `internal/core/mutate/sync.go`).

Amendments and held drafts. `docs/spec/artifact-schema.md`, "Evolving a project" and "Amendments" (held drafts, reviewed moves, eval correction, preconditions, atomicity, states); `docs/spec/cli-surface.md`, "Large changes: amendments". Corrections outside an amendment: "Proposal corrections, moves, and removal" and `docs/spec/cli-surface.md`, "Proposal correction commands" and "Recording surfaces and dependencies". Baseline verbs: "Design baseline commands".

Drift and readiness. `docs/spec/artifact-schema.md`, "Drift control hooks"; `docs/spec/cli-surface.md`, "Detecting drift: drift". Readiness gaps: "The root manifest" (the readiness paragraphs) and `docs/spec/cli-surface.md`, "Bootstrapping a project". Detection writes nothing and refuses nothing.

Order, cycles, holds, labels (the `internal/gen` half). Precedence is recorded dependencies, then soft order, then flow rank inside an epic cycle, then canonical key; a cycle never errors and renders as one group. `docs/spec/artifact-schema.md`, "Tasks" ("Ordering"); `docs/spec/cli-surface.md`, "Rendering the tree and dependency order: tree, check readiness". Question holds: "Decisions and open questions" (the blocker projection paragraph). Derived activity: "Epics" and "Features" ("States"), "Bugs, chores, and tweaks" ("Resolution"). The label convention and address resolution: `docs/spec/cli-surface.md`, "Grammar". Prose paths: "Directory layout". The board status that shows a stored-ready feature with a task in flight as active (`FeatureBoardStatus`) is console display: `docs/spec/ui-surface.md`, "Build".

Next moves. The classifier and eligibility rules here feed the list specified in `docs/spec/cli-surface.md`, "Ranked next", "Guidance and next moves: guide, next, readout", and "Review grain" ("Ranked next and the queue"). A failed observation or event read degrades to less personal ranking, never an error.

Failure behavior. A blocked answer is exit 1 with the current state and legal moves; misuse is exit 2; persistence is exit 3. A refused transition writes nothing. A red `check` is the exception among exit 1 answers: it still records the gate results, an event, and the evidence files. See `docs/spec/cli-surface.md`, "Output and exit codes". Observed and undocumented: a gate command runs through `sh -c` from the project root with no timeout, and a command that cannot be started records `fail` (`internal/core/gates/run.go`).

Events and migration. Event type names are given in the same sections as the rules they record. Compaction and migration behavior: `docs/spec/cli-surface.md`, "Compacting events: compact" and "Migrating and upgrading: migrate, upgrade"; `docs/spec/distribution.md`, "Migration".

## Edges

Layering, production imports only: `lifecycle` and `gates` import the store alone, and `resolve` adds `internal/id`; `workflow` and `readiness` add `internal/gen`; `mutate` imports `lifecycle`, `gates`, `resolve`, `events`, `gen`, `workflow`, `vcs`, `id`, and `rootpath`, and also `corpus` and `relationships` (for workflow observation and the citations a reviewed move surfaces); `drift` imports `mutate`, `evalref`, `gen`, and `vcs`; `readinput` sits above them and imports `views`, `execution`, `relationships`, and `corpus`. The projection files in `internal/gen` import the store and `relationships`.

Callers are `internal/cli` and `internal/serve` directly, and `internal/views` for every read projection. The CLI resolves the acting contributor and passes it in as `mutate.Deps` with a clock, an id minter, and a Git reader. The contract back is a result struct or a typed error; mutate names many refusal reasons itself, and the CLI assigns others when it renders a typed error (for example `open_question_blocker` and `parent_plan_ungated` are set in `internal/cli/lifecycle_cmds.go`).

External processes: gate and criterion commands through the shell, and local Git through `internal/vcs`. No network, no PR host. Tiller creates, moves, and deletes no Git ref.

## Described paths

- `internal/core/mutate`
- `internal/core/gates`
- `internal/core/lifecycle`
- `internal/core/drift`
- `internal/core/readiness`
- `internal/core/resolve`
- `internal/core/review`
- `internal/workflow`
- `internal/readinput`
- `internal/evalref`
- `internal/gen/activity.go`
- `internal/gen/activity_test.go`
- `internal/gen/amendment_hold.go`
- `internal/gen/amendment_hold_test.go`
- `internal/gen/execution_cycles.go`
- `internal/gen/execution_cycles_test.go`
- `internal/gen/execution_review_cycles_test.go`
- `internal/gen/held.go`
- `internal/gen/held_test.go`
- `internal/gen/label.go`
- `internal/gen/label_test.go`
- `internal/gen/order.go`
- `internal/gen/order_test.go`
- `internal/gen/order_cycles.go`
- `internal/gen/preference.go`
- `internal/gen/prose.go`
- `internal/gen/prose_test.go`
- `internal/gen/question_blockers.go`
- `internal/gen/question_blockers_test.go`
- `internal/gen/tree.go`
- `internal/gen/tree_test.go`
- `internal/gen/workitem.go`
- `internal/gen/workitem_test.go`

## Discrepancies and gaps

- **Manual pause.** `docs/spec/artifact-schema.md`, "Epics", "States", describes `blocked` as a flag with a reason, and the sources speak of manual pauses surviving transfer and moves. The code reads the flag (`internal/gen/preference.go`, `internal/workflow/workflow.go`) and copies it on transfer, but no command sets or clears it and no reason is stored. `block` and `unblock` are on the roadmap as not built.
- **Removal event for tweaks.** "Proposal corrections, moves, and removal" lists the removal events and stops at `remove.proposed.chore`. The code also emits `remove.proposed.tweak` (`internal/core/mutate/remove.go`).
- **Idempotence.** `docs/spec/cli-surface.md`, "Output and exit codes", says mutations are idempotent: a transition whose target equals the current state is a no-op success, so retries are safe. The engine honors that only after a verb's guards pass, `check` re-runs its gates and writes on a task already in `check`, and amendment `approve` and `withdraw` refuse a repeat with `amendment_not_open` (`internal/core/mutate/transition.go`, `check.go`, `amendment_approve.go`, `amendment.go`). Read from the code, not run.
- **Actor for `resolve`.** `docs/spec/cli-surface.md`, "Who may run what", puts `tiller resolve` in the human-only approve class. The actor table in `internal/core/lifecycle/table.go` has no entry for `resolve`, so `ActorFor("resolve")` answers either. Its one consumer is `internal/views/nextowner.go`; whether that is ever asked about `resolve` was not traced.
- **Lists that predate tweaks.** "Lifecycles, consolidated" and `docs/spec/cli-surface.md`, "Who may run what", have a "Bug, chore" row only. The code gives tweaks the same rows (`workItemRows` in `internal/core/lifecycle/table.go`), as the prose under "Bugs, chores, and tweaks" says.
- **Amendment input validation.** "Deferred amendment input validation" is labeled partially unimplemented in the source and is on the roadmap. The code does not reject unknown YAML fields in an amendment file; what it checks at approval was not audited against that section.
- **Shelf drift sweeps and uncovered-root routing.** "Drift control hooks" labels both unbuilt, and the code has neither. The roadmap lists both.
- **Task completion anchor.** "Amendments" says a done task records no completion anchor, so `target_revision` against a done task always refuses, and leaves the fix as an unmade schema decision. That matches the code. The roadmap does not list it.
- **Not checked.** Sync's ancestry and content proofs, the order of approval checks for each amendment operation, transfer's relationship rules, review recovery, and the migration steps were confirmed to exist with the documented refusal reasons, not traced line by line. Ranked next tiers were read only as far as their inputs here.
