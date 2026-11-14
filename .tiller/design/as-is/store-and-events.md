# Store and events: the on-disk model and its storage layer

This is a pointer page. The contract lives in `docs/spec/artifact-schema.md`, with the locking and journal rules in `docs/spec/cli-surface.md`, "Working-tree posture", and the schema-version rules in `docs/spec/distribution.md`, "Versioning" and "Migration". This page says which sections apply, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0, schema 15).

## What this surface is

The code that knows what a `.tiller/` directory looks like and how to read and write it safely. It has four parts. `internal/store` defines a Go type for every state file, loads the whole tree into one validated in-memory snapshot (`store.Load`), confines every read and write beneath the checkout root, and commits a planned batch of file writes as one locked, journaled, generation-checked transaction (`Tree.Commit`, `Tree.CommitChecked`, `Tree.CommitDerived`). `internal/core/events` defines the event record, where a loose event file lands, the JSONL segment encoding, and the readers that take the union of loose files and segments. `internal/id` mints and validates ids. `internal/rootpath` opens a relative path one checked component at a time under an `os.Root`, which is what the confinement is built on.

It decides nothing about whether a change is legal. That belongs to the lifecycle and gates page. Two writers of this page's formats also live there: event compaction (`internal/core/mutate/compact.go`) and schema migration (`internal/core/mutate/migrate.go`, `migrate_ownership.go`). The usage files under `.tiller/usage/` and their cursor belong to `internal/usage`. The text of `.tiller/orientation.md` belongs to `internal/gen`.

Every CLI command and every console request reads through `store.Load`. Every state change goes through `internal/core/mutate`, which plans writes and hands them to `Tree.Commit`.

The principle and the layout: `docs/spec/artifact-schema.md`, "The principle this schema serves", "Greenfield is the special case", "Project archetypes", and "Directory layout".

## Data it owns

Persisted state is everything under `.tiller/` except usage files and the generated orientation text. The store owns the YAML state files and the machine-owned parts of the layout. It reads prose Markdown only for the acceptance contract's frontmatter (`internal/store/eval.go`) and never writes a human's prose. The shapes, field by field, are in `docs/spec/artifact-schema.md`:

- The manifest, local identity, and local settings: "The root manifest: `tiller.yml`", "Local identity: `identity.yml`", "Local settings: `local.yml`".
- The design layer and its ledger: "Design layer: `design/`" and "Baseline ledger: `design/baseline.yml`".
- Work nodes: "Epics", "Features" (with "Merged aggregate review recovery"), "Tasks", "Bugs, chores, and tweaks".
- Acceptance contracts and stored results: "The eval model" and, under "Gates", the paragraphs "The stored gate block" and "Gate reuse".
- Questions and amendments: "Decisions and open questions" and, under "Evolving a project", "Amendments" (the YAML example and the nine operation kinds).
- Events: "Event stream".
- The transaction files under `.tiller/.cache/` (`lock`, `journal.yml`, `generation`): the `.cache/` entries in "Directory layout", and `docs/spec/cli-surface.md`, "Working-tree posture".

Identity. An id is `<prefix>-<six lowercase base36 characters>` from cryptographic randomness; `internal/id` knows eleven prefixes (`E`, `F`, `T`, `Q`, `A`, `ADR`, `B`, `CH`, `TW`, plus the non-node `EV` and `AT`). Keys are path-derived and renameable. A revision reference is the one shape used to pin content. See "IDs" (including "Keys and paths"), "Revisions", and "References and IDs".

Schema version. `CurrentSchemaVersion` is 15 and `MinMigratableSchemaVersion` is 2 (`internal/store/manifest.go`). The version classes and the read and write rule are in `docs/spec/distribution.md`, "Versioning" and "Migration", with "Pre-release schema boundaries".

In-memory state. A loaded `store.Tree` is a per-read snapshot: the nodes indexed by id, the question ledger, amendments, the baseline ledger, the soft orders, a derived index of which open amendment holds which draft (`Tree.AmendmentHolds`), and the generation token it was read under. It lives for one command or one request. `internal/core/events/cache.go` is a parsed-event cache keyed by file path that only `tiller serve` turns on; `docs/spec/ui-surface.md`, "Liveness", paragraph "Read cost", documents it, and the artifact schema does not mention it.

## Behavior to preserve

Load is strict and fails whole. A malformed id, a prefix that does not match its noun, a duplicate id, a recorded key that is not the key its folders spell, a symlink where a node folder should be, an illegal `review_grain`, a malformed recovered review, a null question entry, an obsolete `claiming` block, or a schema stamp other than the one this binary writes refuses the load instead of yielding a partial tree. See `docs/spec/artifact-schema.md`, "IDs" (third paragraph and "Keys and paths"), "Directory layout" (the "State path confinement" paragraphs), "Epics" ("Review grain"), and "The root manifest" (the paragraphs after the block table). The stamp refusal and its remedies are in `docs/spec/distribution.md`, "Versioning", "The safety check".

Path confinement. Every read, snapshot, and write beneath `.tiller/` is descriptor-relative from the checkout root, and a symlink in those paths is refused, never followed. This is confinement, not a sandbox. See "Directory layout", "State path confinement".

Transactions. A single file is written to a temporary file and renamed. A mutation batch takes the repository lock, checks that the generation still matches the one the tree was loaded under and that each destination still matches the snapshot the plan captured, journals preimages, applies, advances the generation, and clears the journal. A failed write rolls the batch back. Readers read the generation before and after loading and refuse if it moved or if a lock or journal is present. The refusals are `mutation_locked`, `mutation_stale`, and `mutation_recovery_needed`, all exit 3. See `docs/spec/cli-surface.md`, "Working-tree posture" and "Output and exit codes" (the exit code table), and `docs/spec/artifact-schema.md`, "Proposal corrections, moves, and removal" (the journal paragraph, for deletes and directory removal). Derived writes (`CommitDerived`) use the same lock and checks but do not advance the generation.

Two details of the lock are in the code and in no document. The lock is a file created exclusively with the process id and time inside it; a second writer does not wait, it refuses at once. Nothing expires a lock or replays a journal left by a process that died: journal recovery runs only inside the process whose write failed (`finishFailedMutation` in `internal/store/write.go`). See Discrepancies and gaps.

Events. One immutable YAML file per event, dated by UTC timestamp, written in the same transaction as the state it describes; sorting is by timestamp then id; a reader counts each id once across loose files and segments and refuses an id held twice with different content. A legacy `owner:` key is read as `contributor:` and rewritten only by compaction. Segments are content-named, immutable, and never removed. See `docs/spec/artifact-schema.md`, "Event stream" (including "Compacted segments") and, under "Decisions settled so far", the event compaction list.

Tolerance. `local.yml` tolerates unknown keys and refuses an unparseable file (`local_settings_unreadable`). A missing baseline ledger, adopted anchor, soft order, or shelf is ordinary absence. See "Local settings: `local.yml`", "Baseline ledger", and "Directory layout" ("Lazy files").

## Edges

`internal/store` imports only `internal/id` and `internal/rootpath`; `internal/core/events` imports `internal/id` and `internal/store`. Neither runs Git. Revision values, blob hashes, and code identities are computed by `internal/vcs` and handed in by the callers.

Callers are `internal/core/mutate` (the only production caller of the commit entry points besides the CLI's init, join, refresh, and usage paths), every read projection in `internal/views`, `internal/gen`, `internal/relationships`, and `internal/search`, and the console in `internal/serve`. `internal/rootpath` is also used directly by `internal/corpus`, `internal/execution`, `internal/vcs`, and `internal/selfupdate`.

The contract across the boundary is the typed error. A persistence or environment failure is a `store.EnvError` carrying an exit code and a stable `Reason`; the CLI turns it into the JSON envelope. The reasons and the envelope are in `docs/spec/cli-surface.md`, "Output and exit codes". The only external dependency is the local filesystem.

## Described paths

- `internal/store`
- `internal/core/events`
- `internal/id`
- `internal/rootpath`

## Discrepancies and gaps

- **Schema number.** `docs/spec/cli-surface.md`, "Migrating and upgrading: migrate, upgrade", first paragraph, says `schema_version` is "currently 14". The code writes 15 (`CurrentSchemaVersion` in `internal/store/manifest.go`), and `docs/spec/artifact-schema.md` and `docs/spec/distribution.md` agree with the code.
- **A dead process leaves the project locked.** No source says how to clear `.tiller/.cache/lock` or a retained `journal.yml` after a killed process. `docs/spec/cli-surface.md`, "Migrating and upgrading", says to "recover existing transactions before retry" and not to delete `.tiller/.cache`, but names no procedure. The code has no stale-lock check and no recovery command: `acquireMutationLock` (`internal/store/write.go`) and `readableGeneration` (`internal/store/tree.go`) refuse for as long as either file exists. The roadmap lists `doctor` as not built.
- **`blocked` has no reason.** `docs/spec/artifact-schema.md`, "Epics", "States", calls `blocked` "an orthogonal flag with a reason". The stored field is a bare boolean on epics, features, tasks, and shelf nodes (`internal/store/node.go`, `internal/store/workitem.go`), with no reason field and no command that writes it. The roadmap lists `block` and `unblock` as not built.
- **Schema fence.** "The root manifest", block table, first row, describes `schema_epoch` and a branch fence in the present tense. The code stores and seeds `schema_epoch` and never reads it. `docs/spec/distribution.md`, "Migration", labels the fence unbuilt. The roadmap does not list it.
- **`git.pr_coherence`.** The manifest example comments it as "warn when a PR crosses unrelated subtrees". The field is decoded (`internal/store/manifest.go`) and seeded by `init`, and nothing reads it.
- **Fields with no writer.** A task's or shelf node's own `evidence` list ("Tasks", "Evidence") is decoded but never written; only gate-level evidence is. `add evidence` is on the roadmap as not built. The ADR location ("Decisions and open questions") is labeled unbuilt in the source and has no loader.
- **Lists that predate later nouns.** "IDs", third paragraph, says load validates epic, feature, task, question, bug, and chore ids; the code also validates amendment and tweak ids (`internal/store/tree.go`, `internal/store/workitem.go`). "References and IDs" omits the shelf nouns from the keyed classes.
- **Not checked.** The byte-level layout of `journal.yml`, the rollback paths for directory removal, the baseline ledger validator, and the recovered-review validator were not read line by line.
