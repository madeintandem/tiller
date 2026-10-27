# Tiller Artifact Schema

**Scope.** This document defines the on-disk representation: every file Tiller reads or writes, its shape, and the rules that bind them together. It is the foundation the CLI and the local web server operate over. Both are operations on what's defined here. The CLI command surface, the skill set, and the `tiller serve` UI each get their own document.

**Status.** Current reference for the on-disk model, maintained alongside the shipped behavior. Where the two disagree, the code is authoritative and this document gets corrected with the change. Unbuilt design is labeled explicitly; the [roadmap](../roadmap.md#not-built-yet) tracks remaining scope, and the [design history](../design-history.md) records the decisions behind this contract.

---

## The principle this schema serves

Tiller holds the structure, the gates, and the state. The agent does the reasoning. The human approves at the gates.

Every file here is an instance of that split, and the split shows up physically: prose files carry intent and acceptance criteria, state files carry status and ownership and gate results, and the two never share a file. Prose is human-owned and agent-drafted; it changes rarely and meaningfully. State is CLI-owned and machine-mutated; it churns constantly. Keeping them apart means the CLI can rewrite status a hundred times a day without ever touching a human's words, merge conflicts stay localized to the layer that actually changed, and a person can edit a spec without fear that the next `tiller` command clobbers it.

## Greenfield is the special case

The schema is designed for brownfield and treats an empty greenfield project as the special case. At adoption it has no current-state code, so its initial features usually create code and introduce behavior. Once that code ships, later features can modify, replace, or migrate it and preserve existing behavior without changing the project's kind. Brownfield fills in the same fields from day one: an as-is layer that includes existing code, named surfaces, and `preserve` criteria that assert behavior survives the change.

Two rules hold in both cases, and they hold hardest in brownfield:

- **Existing code is input to be understood, never a convention to inherit.** Prior patterns get documented in the as-is layer so the team and the agent know what they're working against. They do not change how Tiller decomposes, gates, or tracks. The forward process is uniform whether the repo is empty or a decade old.
- **The as-is layer describes reality; the to-be layer describes intent.** A design doc can hold both. Decomposition reads both.

---

## Project archetypes

`project.kind` records the relationship to code that predates Tiller's adoption. It does not restrict a feature's later `change_type` or mechanically choose eval polarity. All three kinds allow `create`, `modify`, `replace`, and `migrate`; the feature's actual change determines its contract.

Every declared code root must carry an explicit `mode: surface` or `mode: reference`. `surface` names writable code a feature may change. `reference` names code read to understand behavior, not code a recorded surface may overlap. There is no implicit mode, even on greenfield projects. These tags guard surface recording, not writes by a developer or editor.

| Archetype | Relationship at adoption | Scaffold and readiness | Baseline requirement |
|---|---|---|---|
| `greenfield` | no pre-existing system to adopt | `code: []`; no adopted anchor or reference-root requirement; design inputs and contributors still required | no confirmed-baseline gate or baseline routing |
| `brownfield` | this repository's existing code is the system being changed | explicit `.` surface root; optional adopted anchor; design inputs and contributors still required | confirmed as-is doc required; route to `derive-baseline` until confirmed |
| `legacy-migration` | a new system replaces an old, read-only reference system | explicit `.` surface root; optional adopted anchor; an existing reference root must be declared, in addition to design inputs and contributors | confirmed as-is doc required; route to `derive-baseline` until confirmed |

A reference root declared on any kind has the same surface protection. For legacy-migration, preserve criteria can name the legacy artifact, revision, or golden output they were captured from. The baseline ledger, confirmation gate, drift markers, and read-only console details are optional: a project without them still loads.

---

## Directory layout

Everything Tiller owns lives under `.tiller/` at the repo root, beside the code it describes.

```
<repo-root>/
  .tiller/
    tiller.yml                     # root manifest: project, gates, contributors, inputs, conventions
    design/                        # durable "what": as-is + to-be design docs
      00-index.md
      baseline.yml                 # optional CLI-owned as-is ledger
      <design docs…>
    epics/                         # the work tree: epics > features > tasks
      <epic-key>/
        epic.md                    # prose: the epic narrative: intent, scope, definition of done
        epic.yml                   # state: id, status, depends_on, surface; contributors are derived
        features/
          <feature-key>/
            feature.md             # prose: the feature narrative and specification
            feature.yml            # state: id, status, change_type, surface, refs; contributors are derived
            feature.eval.md        # acceptance criteria (introduce + preserve), bindings
            tasks/                 # laid down by the second decompose stage
              <task-key>/
                task.md            # prose: the implementation spec
                task.yml           # state: id, status, owner, gates, pr, evidence
    bugs/                          # the bug shelf: leaf work nodes workable on their own
      order.yml                    # the shelf's soft order; written only by `tiller order bug`
      <bug-key>/
        bug.md                     # prose: reproduction, expected, actual, fixed-when
        bug.yml                    # state: id, status, owner, attached_to, resolved_by, gates, pr
    chores/                        # the chore shelf, same shape
      order.yml
      <chore-key>/
        chore.md                   # prose: what, why, done-when
        chore.yml
    tweaks/                        # the tweak shelf, same shape as chores
      order.yml
      <tweak-key>/
        tweak.md                   # prose: what, why, done-when
        tweak.yml
    plans/                         # semi-ephemeral execution plans
      YYYY-MM-DD-<slug>.md
    amendments/                    # reviewed, recorded changes to the plan
      <amendment-id>-<key>.md       # prose: the narrative of a large change
      <amendment-id>-<key>.yml      # state: status, staged operations, links
    reviews/                       # code-review files: prose only, no state, written by the code-review skill
      YYYY-MM-DD-<node-id>.md
    decisions/
      adr/
        <adr-id>-<slug>.md          # designed ADR location; ADR node surface is not built
      open-questions.yml           # owned, date-gated questions
    events/                         # one event file per mutation, for recaps, history, and stale-click protection
    usage/                          # agent token usage, one file per harness session; written by capture, never by hand
      <harness>/
        <session-id>.yml
    orientation.md                 # GENERATED for the agent: what Tiller is and the rules to follow; do not hand-edit
    order.yml                      # root soft order: recorded epic sequence; created by `tiller order` or an amendment approval that mints an epic
    identity.yml                   # LOCAL, git-ignored: who I am on this checkout (contributor: <id>); written by `tiller join`
    local.yml                      # LOCAL, git-ignored, optional: my own preferences on this checkout; hand-edited, never written by Tiller
    .cache/                        # GENERATED transaction metadata, git-ignored
      lock                         # exclusive repository mutation lock; present only while held
      journal.yml                  # recoverable operation plan and preimages; present only during or after an unfinished mutation
      generation                   # persistent source-generation token used by readers and writers
      usage.yml                    # usage capture's cursor: what it last saw of each harness log
```

**State path confinement.** Store and consumer reads, snapshots, and writes beneath `.tiller/`, including the manifest, node and question state, event streams, generated orientation, local identity and preferences, and cache lock, journal, and generation files, use descriptor-relative operations anchored at the selected checkout root. Re-add checks, lifecycle prose gates, readouts, and mutation inventory walks use the same boundary. Directory entries carry metadata captured under their anchor rather than reopening a pathname later. Symlinks in these paths are refused rather than read as state or followed for writes. Initialization and design imports also confine destination directory creation. Temporary files, replacement, lock release, journal cleanup, rollback, and recovery retain their directory anchors, so a pathname swap cannot redirect them to a symlink target. Missing lazy state remains ordinary absence. Explicit design and feedback import sources, configured external reference code roots, gate execution inputs and evidence outputs, saved human ownership reviews, and home installation retain their separate boundaries.

Node inventories reject symlink entries before filtering ordinary non-directory files. A symlink at an individual epic, feature, task, bug, chore, or tweak home is unsafe state, never an absent node. Unrelated regular files such as shelf `order.yml` remain outside the node inventory.

This is path confinement, not a sandbox against a user who controls the filesystem. The selected checkout root and its ancestors are trusted; hard links, mount changes, and moving an already-open directory outside the checkout are not prevented by these anchors.

**Why a folder per epic, feature, and task.** Separate task records let engineers work on different tasks in the same feature. Features and epics group scope and acceptance, not exclusive ownership. Shared aggregate records and code still require ordinary Git coordination and deliberate semantic conflict resolution; the folder layout is not a distributed lock.

**Lazy files.** A feature starts life as `feature.md` + `feature.yml`, holding the narrative the first decompose stage wrote. The eval file and the `tasks/` subtree appear together at the second stage, when the feature is specified and exploded into tasks in one pass. You don't pay the file-count cost for features you haven't reached. The three shelves are lazy in the same way: `tiller init` scaffolds none, and the first `add bug`, `add chore`, or `add tweak` creates the one it needs.

---

## IDs

Every Tiller node id is a type-prefixed, collision-resistant token, not a counter. The shape is `<prefix>-<token>`, where the prefix is the node type (`E`, `F`, `T`, `Q`, `A`, `ADR`, `B`, `CH`, or `TW`) and the token follows the compact random token rule. `B` is a bug, `CH` a chore, and `TW` a tweak; `C` alone was passed over because eval rubric output already labels criteria `C1`, `C2`, `C3`.

**Compact random token.** A compact random token is exactly six characters from lowercase base36 (`[a-z0-9]{6}`), minted from cryptographic randomness. `F-79dh3q` is a feature id; `T-k3p9w6` is a task id.

The token carries no owner, branch, date, order, or path. That opacity is deliberate: two agents on two branches can mint ids without reading or updating a shared counter, and a rename never changes the id. On creation, Tiller retries if the generated id already exists in the visible working tree; if two branches still merge the same id, the duplicate is a validation failure and one side regenerates. Human-facing order lives in lists, timestamps, and generated views, not in ids.

A null entry in `open-questions.yml` fails the load with an error naming the ledger path and entry position, and no partial question list is kept. Tree load validates every addressable epic, feature, task, question, bug, and chore id before assigning the node to an index. Each id must satisfy the compact token syntax, carry the prefix for its noun, and be unique across the whole tree. A malformed id, noun-prefix mismatch, or duplicate id makes the load fail rather than leaving a node unreachable or allowing one node to replace another in memory.

**Keys and paths.** A `key` is built from local key segments, each matching `[a-z0-9]+(?:-[a-z0-9]+)*`. An epic's key is a single segment, and it lives at `.tiller/epics/<epic-key>/`. A feature's canonical key is the two-segment `<epic-key>/<feature-key>`, and it lives at `.tiller/epics/<epic-key>/features/<feature-key>/`. The directory segment under `features/` is the LOCAL `<feature-key>` alone, while the `key` field in `feature.yml` and any branch interpolation use the CANONICAL `<epic-key>/<feature-key>`. That is why the layout shows a `certification-form-loading/` folder while the feature's `key` is `form-engine/certification-form-loading`: the folder name is the local segment, and the canonical key prepends the owning epic's key. A task follows the same rule one level down: its directory segment under `tasks/` is the local `<task-key>`, and its canonical key is the three-segment `<epic-key>/<feature-key>/<task-key>`. A bug's canonical key prepends its shelf the same way, `bugs/<bug-key>`, and a chore's is `chores/<chore-key>`; the directory segment is the local key alone, and `bugs`, `chores`, and `tweaks` are reserved epic keys, since an epic folder sits beside the shelves. The rendering convention for human output is defined in [cli-surface.md](./cli-surface.md), this document staying canonical for the id/key model alone.

A Tweak's canonical key is `tweaks/<tweak-key>`. It lives at `.tiller/tweaks/<tweak-key>/`, uses `tweak.md` and `tweak.yml`, and resolves by its local key when the `tweak` noun selects the shelf. `tweaks` is reserved as an epic key, alongside `bugs` and `chores`.

### Derived relationship identities

The relationship index used by `query` and the console is request-local derived data, not a persisted artifact or schema change. Work, question, and amendment vertices retain their existing immutable IDs. Each lowercase Markdown file under `.tiller/` has a distinct `doc:<repo-relative-path>` identity, including node-owned prose, eval contracts, loose documents, ADR prose, and generated Markdown. Ownership is an attribute and `has_document` relationship; it does not replace the document's identity.

Recorded local file/directory coordinates use `path:<repo-relative-path>` with slash-separated, segment-normalized paths. Declared external baseline coordinates use the separate `external_path:` namespace and cannot become repository prose links. A coordinate need not still exist. Documents join their file coordinate through `at_path`, not a coverage assertion. Unresolved recorded IDs keep their raw spelling and provenance without borrowing the title, lifecycle, owner, or destination of current work. Opaque or unsupported provenance uses source-scoped unresolved references rather than guessed paths.

Edges retain their source file, field/operation, available line/criterion, and any recorded revision or evidence. Reciprocal supersession assertions describe one logical edge with both sources; contradictions remain diagnostics. Amendment operation roles and proposed/applied evidence remain distinct, and historical key reuse never substitutes today's identity for an unprovable old one. A path's current contents are not its pinned historical bytes.

`covers` distinguishes work `surface.paths`, baseline `describes`, and path-shaped test acceptance refs. It is a declaration, not inspected implementation coverage, task-scope inheritance, or a dependency between work sharing files. Soft order remains container metadata. Owner IDs, arbitrary URLs, commands, revision pins, historical inventories, and node-local gate/criterion/attestation/evidence references do not automatically become addressable entities. Design frontmatter stays human convention, not authoritative state. The full catalog, traversal, path matching, and completeness contract lives in [Relationship queries](./cli-surface.md#relationship-queries); no relationship files or cache are written under `.tiller/`.


## Proposal corrections, moves, and removal

A node has two names with different contracts. Its `title` is display metadata in the machine-owned YAML; `tiller rename <noun> <node> --title "<display title>"` may change it at any lifecycle state and never reads or writes the companion Markdown. Its `key` is the canonical address and the source of its directory path. `tiller rename <noun> <node> --key <canonical-key>` preserves the stable id and every id-backed relationship while moving and rewriting the proposal subtree atomically.

Direct `rename --key` moves stop at the proposal boundary. A draft epic may move only while every child feature is also draft and task-free; its feature keys receive the new epic prefix. A draft, task-free feature may move within its epic or reparent between two draft epics; reparenting removes its id from the source epic's soft order and does not invent placement in the destination. A proposed task may change only its local key and must remain under the same feature. A proposed bug, chore, or tweak may likewise change only its local key, on the shelf it is already on; there is no move between shelves. The journaled move preserves permission bits for every regular file and for each corresponding destination directory the transaction creates; ownership and other metadata are not preserved. Reviewed `move-feature` amendments provide the separate route for eligible approved but unstarted features described below. Neither route moves started, terminal, branched, or otherwise execution-historical work or rewrites branch/review history.

`depends_on` stores ids. `tiller unlink` subtracts the resolved target ids while preserving the order of the survivors; an already absent edge changes no file and records no event. The lifecycle boundary for a real edge change belongs to the CLI contract (a started task may add, never remove, an edge whose target task is done; the refusal names the edge rule, never an amendment), but the stored effect is the same at all three grains: only the addressed node's `depends_on` list changes.

`tiller remove` is deletion, not cancellation. It permanently erases only unapproved proposal material: one proposed task, one task-free draft feature, one draft epic whose child features are all draft and task-free, or one proposed bug, chore, or tweak. Removing an epic removes that draft feature subtree with it. The command also removes the deleted ids from every surviving `depends_on` and soft `order` list, from root `.tiller/order.yml`, from each shelf's own `order.yml`, and from every question's `blocks` list, and it clears any `attached_to`, `resolved_by`, or `duplicate_of` on a bug, chore, or tweak that named a removed node, naming each cleared node in the answer. A surviving `supersedes` or `superseded_by` edge is historical lineage and is never silently cleaned; it blocks removal. A proposed amendment operation that names a removed node also blocks removal, while approved and withdrawn amendments remain immutable history.

Reference cleanup, file deletion, empty-directory removal, and the surviving audit event are one journaled source mutation. The journal records preimages for files that will be deleted and the modes of directories that will be removed, so an in-process failure restores both rewritten references and erased proposal files. An unfinished rollback retains `.tiller/.cache/journal.yml` and blocks later readers and writers with `mutation_recovery_needed`.

Correction events name the axis and grain: `rename.title.epic|feature|task|bug|chore|tweak`, `rename.key.epic|feature|task|bug|chore|tweak`, `link.epic|feature|task|bug|chore|tweak` and `unlink.epic|feature|task|bug|chore|tweak` for a dependency edge added or removed, `link.surface.epic|feature` and `unlink.surface.epic|feature` for surface paths recorded or removed, and `link.attach.bug|chore|tweak` and `unlink.attach.bug|chore|tweak` for the attachment arm. A key-rename event keeps the addressed node first, then deterministically names every other node whose persisted state changed: child features rewritten by an epic key move, and a source or destination epic whose soft order changed during feature reparenting. Targets are deduplicated, and unchanged descendants or parents are omitted, so node History receives exactly the relevant correction. Removal leaves one event outside the erased subtree: `remove.proposed.task`, `remove.draft.feature`, `remove.draft.epic`, `remove.proposed.bug`, or `remove.proposed.chore`. Its targets contain every removed id, `before.removed` preserves each removed node's noun, id, key, and title, and `after` records the required reason, removed ids, and complete changed-file list. A deleted file has an empty `after` revision in the event's `files` list. Equal title or key renames and absent-edge unlinks are successful no-ops and write no event.

---

## Revisions

A revision reference is the other identifier primitive: the one shape every field uses to pin a file, tree, or artifact to a point in history. Wherever the schema records what a thing was reasoned against, accepted against, verified against, or captured from, it stores a **RevisionRef** with this shape:

```yaml
kind: git_blob | git_commit | git_tree | sha256
value: "git:31fd9c2"        # or "sha256:<hex>"
path: epics/form-engine/features/certification-form-loading/feature.md
observed_at: "2026-06-23T00:00:00Z"
```

`kind` says what the `value` addresses: a git blob (a single file's content), a git commit, a git tree (a subtree of paths), or a `sha256` content hash for artifacts that live outside git. `value` is the `git:<short-sha>` or `sha256:<hex>` token. `path` is the repo-relative path the ref was taken against, present for blob and tree kinds and omitted for a bare commit. `observed_at` is when Tiller recorded the ref, which is what lets a stale-review check tell "the file changed" from "we last looked a while ago." Every revision-bearing field below stores a RevisionRef rather than a bare string, and the short `git:<sha>` forms shown in earlier examples are RevisionRefs written in their compact value-only form.

---

## The root manifest: `tiller.yml`

```yaml
schema_version: 10            # current on-disk format version; moved forward by reviewed `tiller migrate`
schema_epoch: initial         # changed only by Class 3 migrations; used as the branch fence

project:
  name: Acme Platform
  key: acme
  kind: greenfield            # relationship to code that predates Tiller adoption
  requires_tiller: ">= 0.21"   # optional, advisory: the minimum binary the team expects; Tiller warns on drift

contributors:                       # the humans who hold work; each developer's agent acts as that developer
  - { id: robin, name: Robin Hale }
  - { id: dana, name: Dana Whitfield }

inputs:                       # provenance: what the decomposition was generated from
  docs:
    - design/                 # the design layer, ingested + maintained
  current_state: []           # as-is docs describing the existing system(s)
  code: []                    # greenfield has no code roots at first scaffold
    # Existing-code projects instead start with: [{ root: ., mode: surface }]
    # legacy-migration also needs an existing root explicitly tagged mode: reference
  imported_from:              # set by `tiller init --design`; provenance for the imported design layer
    - { path: /abs/path/to/source-docs, at: "2026-06-29T21:29:29Z" }

gates:
  required:
    build:  { kind: command, run: "bin/rails runner 'true'",  on: [feature, task] }
    tests:  { kind: command, run: "bundle exec rspec",         on: [feature, task] }
    evals:  { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature] }
  optional:                   # visible but non-blocking, the "encouragement" gates
    code_review:          { kind: attestation, on: pr }
    security_review:      { kind: attestation, on: pr }
    performance_review:   { kind: attestation, on: pr }
    accessibility_review: { kind: attestation, on: pr }

git:
  branch_pattern: "tiller/${node.key}"   # task canonical keys; reviews/features/<key> or reviews/epics/<key> for aggregates
  integration_branch: "main"             # the branch merges land on; `sync` observes done crossings into it
  pr_coherence: true                     # warn when a PR crosses unrelated subtrees

execution:
  review_grain: task           # task | feature | epic; the project's own default, most specific wins (see Review grain, under Epics)

drift:                        # load-bearing: `tiller drift` runs exactly the listed checks
  checks: [stale_spec, active_basis, answered_question, missing_surface, orphan_eval, stale_design]

usage:                        # optional; capture is off when absent
  capture: true               # record agent token usage from local harness logs (see Agent usage)
```

| Block | Purpose |
|---|---|
| `schema_version` / `schema_epoch` / `project.requires_tiller` | The on-disk format version, the migration epoch, and the optional advisory minimum binary the team expects. `schema_version` bumps only on a breaking change and is moved forward by `tiller migrate`; `schema_epoch` is a simple scalar token (it starts at `initial`) that advances on each Class 3 migration so branches can prove they have crossed the migration fence; a binary refuses to silently write a tree stamped older than it writes. Each Class 3 migration that advances `schema_epoch` also records a migration event carrying the introduced-at revision (a RevisionRef, see Revisions) that the branch fence checks. The fence record and the full model are in [distribution.md](./distribution.md). |
| `project.kind` | The relationship to code that predates Tiller adoption: `greenfield`, `brownfield`, or `legacy-migration`. It selects first-scaffold guidance, the legacy reference requirement, and the existing-code baseline gate and route, not later `change_type` values or eval polarity. See Project archetypes. |
| `project.adopted` | Optional `{ commit: "git:<short-sha>", at: "<UTC RFC3339>" }`. On the first brownfield or legacy-migration scaffold, init observes this repository's HEAD and records the observation time. It does not fingerprint a dirty working tree, assert a clean tree, or record an external reference repository's HEAD. Greenfield omits it. An old manifest or an unavailable HEAD observation, including non-Git and unborn repositories, can also leave it absent; absence does not prove there was no existing code. |
| `contributors` | The shared roster of humans who can hold work on this project. It starts empty at `tiller init` (`contributors: []`); `tiller add contributor <id> --name "<name>"` registers the first contributor and each later one; `tiller remove contributor <id>` retires eligible entries. Both are deliberate, human-gated acts. `tiller join --as <id>` only ever selects among the contributors already registered here; it writes no roster entry. A developer's agent acts as that developer, so there is no separate agent identity and every action attributes to a human. The human-only gates (approve, accept, attest, the merge to done) are guidance the agent is told to honor, not refusals Tiller enforces. The roster is the committed half of identity; the gitignored `.tiller/identity.yml` is the per-checkout half (see Local identity). The actor model is in [cli-surface.md](./cli-surface.md). |
| `inputs` | Where decomposition came from. `current_state` plus `code` are the as-is additions. Each code root requires an explicit `mode: surface` or `mode: reference`; no archetype supplies a missing mode. Roots are absolute or relative to the repository. `imported_from` records the absolute source `path` and import `at` timestamp when `tiller init --design <dir>` copies an external folder into `.tiller/design/` on the first scaffold. It is provenance, not confirmation that the docs match code. |
| `execution` | The project's own review-grain default, `execution.review_grain` (`task` \| `feature` \| `epic`), the widest of Review Grain's three homes; an epic's or a feature's own explicit setting beats it. Absence means `task`. The stored shape is unchanged. The loader refuses any other value, including one introduced by a hand edit. Written by `tiller configure project --review-grain <g>`, or by hand: the manifest stays the one human-owned yml. `configure project` requires `execution` to be absent or a block mapping; a flow mapping or scalar refuses before any write and must be rewritten as block-form YAML. See Review grain, under Epics. |
| `drift` | The enabled drift checks. Load-bearing: `tiller drift` runs exactly this list, and a check removed from it is named in the report's footer rather than silently skipped, so disabling is deliberate, in-repo, and versioned. Fresh `init` seeds the five work-node markers plus `stale_design`; `shifted_design` is opt-in. Existing manifests are never backfilled. See Drift control hooks. |
| `usage` | The opt-in agent usage setting. `usage.capture: true` lets `tiller usage` and `tiller sync` read this machine's coding-agent session logs and write `.tiller/usage/`; absent, or any other value, means off, and then neither reads a log. It is a project setting so the team decides together and every checkout behaves the same. `tiller init --capture-usage` writes the block on a fresh scaffold; an existing project adds it by hand. Older binaries decode the manifest loosely and ignore the block, so turning it on needs no schema bump or migration. See [Agent usage](#agent-usage-usage). |
| `gates` | Required vs optional. Command-backed (Tiller can run and verify) vs attestation-backed (an external signal sets it; Tiller never runs the reviewer). The canonical required set is `build`, `tests`, and `evals`. `evals` is project-agnostic (it always shells `tiller eval ${node.noun} ${node.id}`), so `tiller init` seeds it active by default at feature grain (`on: [feature]`), wiring each feature's acceptance contract into `check`: the contract gates the feature, while tasks, bugs, chores, and tweaks gate on build and tests. `build` and `tests` are project-specific, so init leaves them as commented templates (at `on: [feature, task]`) the team fills in. Optional attestation-backed gates scoped to PR review can be recorded on a bug, chore, or tweak through its existing `gates` and `attestations` fields. Shelf nodes carry no criteria or eval contract. A required command gate may declare `reuse: false` to opt out of gate reuse (The stored gate block), for a result that depends on something outside the repository, such as an integration suite against a live service; on the `evals` gate it turns reuse off for every criterion. In advisory mode a human merge can still be recorded, but any red required gate is preserved as a policy violation rather than hidden. |

The manifest is human-owned after its first scaffold. A rerun uses its recorded project identity, kind, roots, and adopted anchor rather than reclassifying it from flags. Neither rerun, `join`, nor `migrate` backfills an absent anchor or injects roots. Baseline reasoning states when adoption history is absent or unavailable; it never substitutes a later HEAD as the adoption commit.

Readiness reports `invalid_code_root_mode` for every empty or invalid mode, naming the root, value, manifest, and legal modes. `legacy-migration` also reports `no_reference_root` unless a root explicitly carries `reference`; declaring a missing reference path still leaves the existing missing-code-root gap. Other independent gaps remain visible, including `missing_contributors` and `missing_design` after a bare init. Mode validation also runs at the surface writer without requiring full readiness first.

Readiness reports `no_confirmed_baseline` for exactly `brownfield` and `legacy-migration` when the loaded ledger has no confirmed doc, and `missing_design_doc` in every kind for each ledger doc whose prose is missing, confirmed or not. A ledger by itself does not satisfy the existing non-empty-design check. Readiness does not inspect described paths for drift or judge whether prose is accurate. One confirmed doc clears only the baseline-confirmation gap; contributor, design, root, reference, and orientation gaps remain independent.

The adopted anchor and baseline ledger are optional. Manifests without an anchor and projects without a ledger still load; absence is not a migration or a reason to rewrite the manifest. Every declared code root still requires a valid explicit mode. Previously tolerated empty or misspelled modes need a human correction in `.tiller/tiller.yml`, even on greenfield projects. No migration rewrites that input or retroactively removes recorded surfaces. `project.requires_tiller` remains an advisory minimum binary, not a schema fence or automatic migration.

**Ownership is scalar only on execution units and questions.** Tasks, bugs, chores, and tweaks retain `owner`; questions retain asker attribution. Epics and features have no stored owner, including no compatibility alias. Creation and approval leave new tasks unclaimed; `proposed_by` and event actors remain separate attribution.

`claim`, `release`, and human-directed `reassign` change only the named live, unstarted task, bug, or chore. A successful first start atomically sets durable `started: true` and permanently fixes its owner. Resume, checks, review, cancellation, and completion never clear the marker. Active execution state must not evade this invariant by omitting it. Every terminal owner is immutable, including cancellation before start. Claiming one's own existing claim may be a no-op; no ownership change cascades to ancestors or siblings.

Features and epics expose derived `contributors: [{id, name}]` on reads, never persisted owner lists. Build this projection once per loaded snapshot: distinct started-task owners, sorted by ID, with roster display names and ID fallback for historical owners no longer registered. Empty ancestors emit `[]`. Started done/cancelled tasks count, including transferred originals and tasks beneath cancelled features. Ready reservations, aggregate reviewers, creators, and attached shelf owners do not establish contribution. Personal work is instead every nonterminal task/bug/chore/tweak assignment, including proposed, blocked, checked, and in-review work.

The obsolete manifest `claiming` configuration is absent from fresh scaffolds. Current-schema input containing it refuses with migration/correction guidance rather than selecting another ownership policy. Schema migration removes legacy ancestor ownership only after recording needed provenance. Question owners, event actors, document ownership, and human-versus-agent workflow actor fields retain their distinct meanings.

---

## Local identity: `identity.yml`

`.tiller/identity.yml` records who you are on this checkout. `tiller join --as <id>` writes it, it is git-ignored, and it is never committed. A fresh clone carries the `contributors` roster but no identity file, so every checkout runs its own `join`. This is the per-checkout half of identity; the committed `contributors` roster in `tiller.yml` is the shared half.

```yaml
contributor: robin                     # the id you joined as; must match a contributors entry
```

| Field | Purpose |
|---|---|
| `contributor` | The acting contributor id for this checkout. Written by `tiller join`, validated against the `contributors` roster, and read as the last step of contributor resolution after the `--contributor` flag and `TILLER_CONTRIBUTOR`. Never committed, so identity does not travel with a clone. |

**Contributor terminology.** The shared manifest roster is `contributors:` and checkout-local identity is `contributor:`; older trees spelled them `owners:` and `owner:`. Migration from the older spelling preserves contributor IDs and names, the selected local identity, task/bug/chore/tweak claims and lifecycle, branches, reviews, accepted bases, evidence, and historical event bytes. An absent identity stays absent. Local identity is gitignored, so a migration renames only its own checkout's file; after pulling the migrated manifest, each other checkout's `tiller upgrade --project-only` or bare `tiller join` renames a legacy `owner:` key in place, provided that ID is on the roster. Other commands refuse a legacy key as `identity_unreadable` and name that remedy. New roster events are `contributor.add` and `contributor.remove`; historical `owner.add` and `owner.remove` events are not rewritten. There are no compatibility command, flag, environment, or current-schema key aliases. Acting identity uses `--contributor`, `TILLER_CONTRIBUTOR`, then local identity; execution/question `owner` fields and scalar ownership filters remain unchanged. New event actor attribution uses `contributor:`; historical events with `owner:` remain readable without rewriting their bytes.

---

## Local settings: `local.yml`

`.tiller/local.yml` holds a developer's own preferences for this checkout. It is the third home for configuration beside the committed `tiller.yml`, which the team shares, and `identity.yml`, which says who you are here. Like identity it is git-ignored and never committed, so a preference never travels with a clone or lands in a teammate's diff. Unlike identity, Tiller never writes it: a human creates and edits it by hand, and an absent file means every default. `tiller init` seeds its ignore pattern beside identity's and `tiller join` upserts it into an existing `.gitignore`. There is still no global, user, or machine-level config; a per-developer preference lives in the repo like everything else.

```yaml
console:
  open: false                 # tiller serve does not open a browser on this checkout
```

| Field | Purpose |
|---|---|
| `console.open` | Whether `tiller serve` opens the console in a browser once its port is bound. Absent means yes. An explicit `--open` or `--open=false` on the command line beats this value, and this value beats `BROWSER=none` in the environment. See [tiller serve](./cli-surface.md#tiller-serve). |

Keys this binary does not know are tolerated, so an older binary never refuses a newer file. A file that is present but cannot be parsed is a real error the reading command refuses, `local_settings_unreadable`, rather than a silent fall back to defaults that would hide the typo.

---

## Design layer: `design/`

The durable "what." Numbered markdown with a `00-index.md` entry point. Each doc carries light frontmatter:

```yaml
---
title: Form Engine
layer: to-be            # to-be | as-is | mixed
source: [discovery-analysis/04-domain-model.md, "(6/10 meeting)"]
covers: [E-j8m4q2]          # which epics this doc grounds
---
```

In greenfield, design docs are mostly `to-be`. In brownfield, the `as-is` docs describe the existing system: its modules, its data, and behavior to preserve. Frontmatter is a human convention; Tiller does not read `layer`, `source`, or `covers` as a catalog. The prose remains human-owned and agent-drafted. A design doc is registered as as-is state only through the ledger below, not through its frontmatter. The two baseline drift markers read this ledger.

### Baseline ledger: `design/baseline.yml`

This optional CLI-owned file is separate from the Markdown. Missing state loads as an empty ledger. Malformed state refuses with file context rather than becoming an empty or confirmed baseline.

```yaml
docs:
  - path: as-is/editor.md
    describes: [app/editor]
    derived_from:
      - kind: git_blob
        value: "git:9f86d08"
        path: docs/editor-spec.md
        observed_at: "2026-09-05T18:00:00Z"
    confirmed:
      commit: { kind: git_commit, value: "git:5694f37", observed_at: "2026-09-05T18:30:00Z" }
      surfaces:
        - path: app/editor
          kind: git_tree
          value: "git:31fd9c2"
          inventory: [app/editor/model.go, app/editor/view.go]
          observed_at: "2026-09-05T18:30:00Z"
      by: robin
```

`docs` contains unique canonical paths relative to `.tiller/design/`. Each doc is a regular Markdown file beneath that directory, with no escaping or symlink components; `baseline.yml` is reserved state, not a prose target. Entries have nonempty `describes`. `derived_from` is optional for docs derived directly from code. `confirmed` is absent until approval; an empty confirmation block is invalid. References require complete values, paths where applicable, and observation times. `derived_from` supports `git_blob`; confirmation uses a `git_commit` and `git_blob` or `git_tree` surfaces. Historical confirming owners need not remain on the current roster. Missing prose does not prevent loading, so readiness can name it and unlink can repair the ledger.

`describes` paths must equal or lie beneath a declared valid code root. Containing or merely overlapping a root is not membership. Unlike a work node's `surface`, a design doc may describe reference code, including an explicitly declared external root. Coordinates use forward slashes and are project-relative where possible, including `../legacy/...` only through declared membership; a different volume that cannot be made relative retains normalized absolute coordinates. Canonical declaration aliases may anchor their resolved root, but remaining symlinks and special files refuse. Observation never traverses outside the chosen declared root.

`derived_from` pins a recoverable regular-file blob at this project's observed HEAD. It is not an untracked working-tree hash or a blob borrowed from an external repository. A source retired from the working tree can still resolve at HEAD. Provenance is upserted by source path: the same blob is a no-op, a changed HEAD blob replaces that path's prior ref, and other refs stay. Unlink removes provenance by path without requiring the source to exist or Git to resolve it. An imported external source first becomes a committed local input before it can supply this provenance. `inputs.imported_from` still records the external import; node `source` still records design-to-work lineage.

Confirmation records the acting contributor, one project HEAD and one observation time for the approval batch, and fingerprints of current working-tree code. HEAD is the project history anchor, not a claim that those bytes are committed, not `project.adopted`, and not an external repository's HEAD. The doc's own prose is neither hashed nor persisted in confirmation. Code observation reuses the existing `SurfaceFingerprint` shape, but does not change `sync`'s committed surface observations, which read the integration commit.

**Working-tree inventory.** Files use compatible Git SHA1 blob framing; directories use Git tree framing, executable modes, and sorted names. Inventories list files in sorted order. A directory inside Git includes tracked files still present plus untracked nonignored files, honoring Git ignore rules for untracked files. Changed bytes, added files, and removed files affect the fingerprint. An explicitly named file is observed even if ignored; its blob fingerprint does not encode executable mode independently. A declared external tree without Git includes all regular files. Both policies prune `.git` and `.tiller` metadata, so describing `.` cannot fingerprint its own ledger or events. Empty nested directories are omitted like Git; an explicitly empty root uses the empty-tree fingerprint. No Git index or object writes occur, and a Git failure never becomes an empty or clean observation. `shifted_design` reuses this exact inventory and root-safety policy. Observation is not a lock against arbitrary editors or external Git.

A real change to `describes` clears confirmation; an idempotent retry does not. Provenance-only changes preserve confirmation. Removing the last described path removes the entry, never its prose. Approval can be unchanged when owner, project HEAD, and fingerprint facts match, ignoring a fresh observation time. A matching feature done crossing observed later at that same HEAD makes reapproval meaningful: the confirmation observation advances, clearing `stale_design` without requiring an extra code commit.

`link.design`, `unlink.design`, and `approve.design` events name `.tiller/design/<doc>` as the target and `.tiller/design/baseline.yml` in `files`. `data.doc` is design-relative. Link and unlink record added or removed path lists (`describes_added`, `describes_removed`, `from_added`, `from_removed`, with `from_updated` for a provenance replacement where applicable), plus `describes` and `derived_from` after-state. Approval records `data.commit` as the commit value. There is one event per changed doc and none for a no-op. Design docs have no work-node ids or lifecycle transitions. The [CLI contract](./cli-surface.md) owns command refusals, atomicity, and approval output; the [skill contract](./skill-set.md) owns translation and human review.

---

## Epics

A coherent body of work owned over weeks.

`epic.yml`:

```yaml
id: E-j8m4q2
key: form-engine
title: Form Engine
status: ready            # draft | ready | done | archived | cancelled
blocked: false
depends_on: [E-b6x9t1]       # Foundation
order: [F-79dh3q]            # this epic's own recorded feature order; intent, not constraint; written only by `tiller order`
review_grain: null           # task | feature | epic; absent inherits the project default (see Review grain, below)
branch: null                 # aggregate from reviews/epics/<epic-key>, initialized by first riding task start
pr: null                     # set only when the epic itself is the reviewed node (review_grain: epic)
review_ref: null             # the remote-less fallback for the epic's own pr
recovered_review: null      # optional immutable historical binding; see Merged aggregate review recovery
surface:                 # brownfield: existing code this epic operates on, written by `tiller link --surface`
  paths: []
verified_against:
  spec: { kind: git_blob, value: "git:9f86d08", path: epics/form-engine/epic.md, observed_at: "2026-06-23T00:00:00Z" }
  commit: { kind: git_commit, value: "git:7bc1a20", observed_at: "2026-06-23T00:00:00Z" }   # the integration commit sync observed: the since-when anchor
  surfaces:
    - path: app/models/calc
      kind: git_tree
      value: "git:3f6a9d2"
      inventory: [app/models/calc/project.rb, app/models/calc/zone.rb]
```

The `spec` value is a RevisionRef (see Revisions), recorded by `sync` at the rollup crossing from the `epic.md` blob in the integration commit that run resolved; every leg of the record, surfaces included, reads that one commit, never the checkout's HEAD or working tree. The `commit` leg pins that integration commit, so a later drift candidate can say since when rather than only that something moved, and the agent mending it can read a bounded diff instead of guessing a range. `surfaces` is a fingerprint per surface path: a `git_tree` with the inventory of files the tree covered for a directory path, a `git_blob` whose inventory is the file itself for a file path, so a later comparison can tell which recorded files no longer exist (the candidate) apart from which merely changed (context; see Drift control hooks). A materialized `verified_against` always records `surfaces` as a list, the empty list `[]` for an epic that touched no surface (a greenfield create), so the completion record never silently drops the key; an epic that never reached `done` has no `verified_against` block at all. An epic carries no `gates:` block and no eval file of its own: its acceptance bar is the sum of its features' contracts, each enforced at feature grain.

`epic.md` holds the narrative: intent, scope, and the definition of done in prose. There is no epic-level eval file; the bar for an epic is that every feature under it met its own contract.

**States.** `draft` (proposed by decomposition) → `ready` (the human approved the decomposition tree) → `done` (at least one non-cancelled feature exists and every one of them is done) → `archived` (the human's deliberate close). One human gate: `approve` moves an epic `draft → ready`. The `done` crossing is not a gate at all: epic done is a computed fact, recorded by `sync` as a rollup observation when the last non-cancelled feature crosses, with `done_basis: rollup`, and there is no `accept` verb. The rollup refuses to fire vacuously, exactly as the feature's does one level down: an epic with no features, or with none that are not cancelled, has nothing to roll up and stays where it is rather than crossing on an empty set. The record recomputes: a feature added under a done epic returns the epic to `ready` on the next sync, which is how a done epic reopens, with no ceremony and no amendment operation. The human's judgment moment at this grain is `archive`, the human-only move from `done` to `archived` that says this body of work is closed and put away. There is no stored `active` epic state. Whether an approved epic is being worked is a DERIVED display status, computed from its children rather than stored and mutated: an epic stored `ready` is shown as `active` when at least one child feature is in flight (a feature being worked, or one with a task in progress or in review), and `idle` otherwise. The stored `status` stays `ready` either way, so the file is never rewritten to track moment-to-moment effort. `cancelled` is the terminal rest state for an epic the human rejects or abandons before it completes; the `--reason` is required and recorded on the event, and cancellation is refused once the epic contains done features, because merged work is preserved through supersession instead. `blocked` is an orthogonal flag with a reason.

**Review grain.** The epic's explicit `review_grain` accepts `task`, `feature`, or `epic`, inheriting the project default when absent; feature overrides are more specific. At epic grain the first riding task start records an aggregate by interpolating `reviews/epics/<epic-key>` through `branch_pattern`, or, with `start task --adopt-aggregate`, adopts an existing local ref at exactly that derived name, and later riding features retain it. Every task still owns a separate full-key-derived working branch. The agent ordinary-merges submitted task tips and their state into the aggregate, never cherry-picks or squashes them. `pr` or remote-less `review_ref` on the epic names the one aggregate review. Independent feature overrides complete their own review first. The final integration merge, not assembly, is the human done gate.

Load validates project, epic, and feature `review_grain` as `task`, `feature`, or `epic`; invalid values refuse rather than default. A change considers only descendants whose resolved grain would differ. An affected feature review or task in `in_progress`, `check`, or `in_review` locks the change. A recorded aggregate branch locks every review grain change on that feature, so a change in any direction refuses with `review_grain_branch_locked` once an affected feature holds one. Existing task PRs remain authoritative and must merge and sync before a later coarse submission; no historical branch is reinterpreted as an aggregate. Start records the grain a task resolved in its `task.yml` `started_grain`, and load refuses any value other than `task`, `feature`, or `epic`. A task that started at task grain keeps task grain after a later coarse setting reaches its checkout, typically by a merge that `configure` could not lock: `submit task` takes its own PR (or `review_ref`), `sync` observes that task's own merge, and the aggregate never absorbs it, so a dependent waits for that merge. A task without `started_grain`, started before the field existed, resolves through its parents.

---

## Features

The grain where acceptance lives.

`feature.yml`:

```yaml
id: F-79dh3q
key: form-engine/certification-form-loading
title: Certification form loading (2022 + 2025)
status: active           # draft | specced | ready | active | done | cancelled
blocked: false
change_type: create      # create | modify | replace | migrate
surface:                 # what existing code is touched (empty for create), written by `tiller link --surface`
  paths: []
depends_on: [F-c2m8n5]       # the form-engine core
order: [T-k3p9w6]            # this feature's own recorded task order; intent, not constraint; written only by `tiller order`
review_grain: null           # task | feature | epic; absent inherits the epic, then the project default
supersedes: []           # features this one replaces; written only by an applied amendment
superseded_by: null      # the feature that replaced this one, if any; same single writer
branch: null                 # absent at task grain; reviews/features/<feature-key> at feature grain, or the recorded epic aggregate
pr: null                     # set only when the feature itself is the reviewed node (review_grain: feature)
review_ref: null             # the remote-less fallback for the feature's own pr
recovered_review: null           # optional immutable historical binding; see Merged aggregate review recovery
accepted_against:                 # RevisionRefs (see Revisions): recorded at approve from current (working-tree) feature.md/feature.eval.md content
  spec: { kind: git_blob, value: "git:31fd9c2", path: epics/form-engine/features/certification-form-loading/feature.md, observed_at: "2026-06-23T00:00:00Z" }
  eval: { kind: git_blob, value: "git:b71a322", path: epics/form-engine/features/certification-form-loading/feature.eval.md, observed_at: "2026-06-23T00:00:00Z" }
  commit: { kind: git_commit, value: "git:7bc1a20", observed_at: "2026-06-23T00:00:00Z" }   # HEAD at approval: the code baseline the plan was approved against
assumption_basis: {}         # question ids this node rests on; `approve --reviewed-assumption` stamps the answer revision once a human reviews it in
verified_against:                 # RevisionRefs, recorded by sync at the done crossing from the feature.md/feature.eval.md blobs in the integration commit that run resolved; commit pins that integration commit; surfaces is a fingerprint per surface path, a git_tree with the inventory of files the tree covered for a directory path, a git_blob whose inventory is the file itself for a file path
  spec: null
  eval: null
  commit: null
  surfaces: []
done_basis: null                  # rollup | human_attested; what the done crossing rested on, written by sync
gates:                            # written by `tiller check`; the stored gate results for this grain
  build:
    required: true
    kind: command
    status: pass                  # pass | fail | pending | skipped | stale
    run_by: dana
    run_at: "2026-06-23T00:00:00Z"
    against: { kind: git_commit, value: "git:abc1234" }
    code: "sha256:9f2c0000000000000000000000000000000000000000000000000000000000aa"      # the code identity it ran over; see The stored gate block
    config: "sha256:51aa0000000000000000000000000000000000000000000000000000000000bb"    # the gate's kind and resolved run
    evidence: [{ kind: command_output, ref: artifacts/tiller/gates/build-abc1234-out-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.txt }]
  evals:
    required: true
    kind: command
    status: pending               # not yet green: the eval ran, but C3 still awaits attestation
    run_by: dana
    run_at: "2026-06-23T00:00:00Z"
    against: { kind: git_commit, value: "git:abc1234" }
    criteria:
      C3:
        status: needs_attestation   # pass | fail | pending | needs_attestation | stale
        evidence: [{ kind: rubric_output, ref: artifacts/tiller/evals/F-79dh3q-C3.json }]
attestations:                     # human sign-off, one entry per attested target
  - id: AT-4m9x2q
    target: { kind: criterion, id: C3 }
    status: pass
    by: robin
    at: "2026-06-23T00:00:00Z"
    evidence_ref: artifacts/tiller/evals/F-79dh3q-C3.json
policy_violations: []       # red required gates observed after a human merge under advisory enforcement
```

`feature.md` carries the narrative and the specification: the intent and scope a reader starts at, and the precise behavior the work must produce. `feature.eval.md` is the acceptance contract (below). `tasks/` holds the task folders (below).

**`change_type` and `surface` describe the feature's change, not its project's age.** A `create` feature writes new code and usually has no surface. A `modify` feature can name the existing paths it changes, telling the agent what to read, reviewers the blast radius, and the eval layer what to preserve. `replace` rewrites a surface; `migrate` moves one from one pattern to another. `tiller spec feature` requires at least one recorded surface for `replace` and `migrate`. `modify` may omit it when the honest surface is not yet knowable. None of these change types is restricted by `project.kind`, including on a greenfield project's own shipped code.

Surface paths are machine-owned state, recorded through `tiller link --surface` with union semantics and removed only through `tiller unlink --surface` before the node is done, never hand-edited. Requested paths must be existing files or directories relative to the Tiller project root (the directory holding `.tiller/`), with no symlink component. In a nested project, one whose Tiller root sits below the git repository root, that is the project directory, not the repository root. A path cannot equal, sit beneath, or contain a reference root; a containing directory includes the protected descendants. Comparisons cover both declared and resolved reference-root identities, so an absolute path, normalized relative path, ancestor root, or symlink alias in a reference declaration cannot bypass the rule. An overlapping surface root never overrides reference protection. An existing untracked path outside all declared code roots is still allowed if it has no reference overlap; no Git fingerprint is required to record it. Cleaned project-relative paths are persisted, not canonical absolute paths. Every later read of a surface path, sync's fingerprint and inventory from the integration commit and drift's re-read at HEAD, resolves it in those same project coordinates, so a nested project never reads a repository-root path of the same name.

Every explicitly requested path is checked, including one already recorded. A deleted path therefore refuses when requested again; unrelated recorded paths are not revalidated on each addition. Later deletion remains `missing_surface` drift, not an automatic state rewrite; when the work itself moved or removed the code, link the current location and unlink the old path before the node is done, since sync cannot fingerprint a recorded path the integration commit lacks. The refusal reasons and combined dependency/surface transaction are specified in [cli-surface.md](./cli-surface.md).

**Branches and activation.** A successful `start task` records the task's full-key branch, sets `started: true`, and activates its ready parent in one guarded transaction. Task-grain features record no branch. Feature-grain tasks initialize `reviews/features/<epic-key>/<feature-key>`; epic-grain tasks initialize/reuse `reviews/epics/<epic-key>` on the epic and riding feature. When a ref already exists at that derived name and nothing records it, the first riding task start refuses `aggregate_unrecorded`; `start task --adopt-aggregate` records that existing ref instead, in the same transaction, and the ordinary dependency and inclusion checks then run against it. Adoption never infers an aggregate from history and never takes a task's branch. Starting under an already-active feature leaves activation unchanged. Parent dependencies, accepted bases, questions, retired ancestors, and aggregate review-round guards still apply; failures leave no partial activation, aggregate identity, or frozen owner. Terminal pre-schema-5 refs stay historical.

Task start reports `base_branch` alongside its recorded `branch`: integration for independent work, or the recorded aggregate for a coarse dependency whose submitted prerequisites are included. It can stamp a missing aggregate after a safe grain change. Reopening a completed coarse review records a fresh aggregate named by the canonical aggregate name plus `-<new-task-id>`, preserving the merged branch and historical review. Consumers read back the current records rather than reconstructing an old round's name.

New tasks do not consume a completed coarse review. Until `sync` reopens its parent and clears the old review, start refuses with `aggregate_round_closed`; the next start records the new aggregate while retaining the old Git ref.

Tiller records branch identities and verifies review provenance; the executing model creates branches, commits, merges task branches into the aggregate, runs gates, and maintains the single resolved PR through ordinary Git and host tools. Independent tasks start from the configured integration branch. Task-PR dependencies wait for integration, never stacked task PRs. A coarse dependent task may start from the aggregate after submitted prerequisites and their submission-state commits have been merged there. Assembly uses ancestry-preserving merges, never cherry-pick or squash. Before feature checks, draft publication, and coarse submit, the aggregate includes every current riding task tip as well as each recorded review head. Requested changes resume the same task branch, resubmit there, reassemble, rerun affected checks, and update the existing aggregate PR. Mechanical conflicts are agent work; semantic conflicts, acceptance, and the final integration merge remain human decisions. Intermediate assembly never crosses work to done.

The integration branch comes from `--into`, then `git.integration_branch`, then the checkout branch for ordinary sync. Sync must not compare a review branch with itself. A retained legacy prefix ref may block a new task branch; diagnose both names and preserve history. Use a non-conflicting branch pattern for unstarted work or have the human deliberately archive/rename the collision. Neither migration nor branch creation automatically deletes or renames historical local or remote refs.

**Review grain.** `review_grain` is the feature's explicit `task`, `feature`, or `epic` override. Absent, it inherits the owning epic's `review_grain`, then the project's `execution.review_grain` default, then `task`; an explicit feature value beats both. `pr` and `review_ref` are the feature's own review record, set only when the feature itself is the reviewed node: `submit feature` records one or the other once every non-cancelled task under it is `in_review` or done, with at least one, and the feature's required gates are all recorded pass. At task grain each task carries its own review; at epic grain the epic carries the aggregate review. `configure feature` and `approve feature --review-grain` record deliberate policy changes; reviewed moves materialize the previously effective grain only when destination inheritance would change it. The CLI contract states the ordinary change guards.

An existing task PR is authoritative history: it must merge and sync before a later feature- or epic-grain submit can record its coarser review surface. Under epic grain, a feature with an explicit `task` or `feature` override does not ride the epic PR; it completes its own PR and reaches `done` first.

### Merged aggregate review recovery

Features and epics carry an optional typed `recovered_review`. `submit feature|epic --recover-merged` records a missing historical review after its human integration merge without moving lifecycle, running gates, or manufacturing acceptance. Ordinary `pr` remains the remote review identity; `review_ref` remains the no-remote representation. Recovery never overloads remote `review_ref`.

Questions carry an optional `cancel_reason`, described under Open questions. Tasks carry an optional `started_grain`, described under Review grain.

| Field | Stored fact |
|---|---|
| `branch` | Existing aggregate branch identity for this review round |
| `head`, `merge_commit` | Full historical reviewed head and integration result as `git_commit` RevisionRefs |
| `integration_branch` | Configured integration used for local validation |
| `basis` | `ancestry`, `content`, or `contained`; contained is not done evidence |
| `recovered_by`, `recovered_at`, `reason` | Registered acting contributor, actual recording time, and nonblank explanation |
| `tasks` | Sorted intended riding task IDs, historical task paths, and `git_blob` state refs |
| `policy_snapshots` | Historical manifest and applicable task/feature gate-state blob refs with derived nonpassing required-gate observations |

Historical task blobs pin owner, branch, immutable review head, accepted task-spec basis, and submitted lifecycle. The record does not copy complete task or gate documents. Historical policy observations retain node, gate, recorded status, unresolved required criteria, source revision, and existing evidence references. They describe recorded state, not a new run or approval. Missing required results are pending; invalid or unavailable policy is unknown or a named refusal, never all-clear.

Load validates record shape and consistency with its owning review identity. Malformed recovery cannot silently select live-branch behavior. Recovery requires a matching historical aggregate and intended current riders, with submitted state in the reviewed tree and included task heads/retained tips. Changed feature acceptance is allowed and remains independent; changed approved task scope cannot ride the historical submission.

Sync validates the frozen head/result and riders against recorded configured integration, not today's aggregate tip. Deleted/advanced aggregate refs do not invalidate historical evidence; later unmerged task work cannot be completed. The exact result must remain on integration's first-parent history. Ordinary submissions keep their live-tip inclusion predicate. A contained binding without a merge witness awaits the existing human fast-forward assertion and crosses only on `human_attested`.

One `recover_review.feature` or `recover_review.epic` event preserves the full binding, historical refs, checked task set, old identity, real actor, and reason. It is recorded now, never backdated from commit author time. Identical retry writes nothing and retains actor/time/reason. Reopening clears the current recovery with PR/review-ref, while the event preserves the old round; later task starts rotate aggregates normally.


**Done basis.** `sync` alone records `done_basis`. At task grain it observes the task's own branch, never a sibling or parent branch; under a coarse review it observes the recorded aggregate and crosses its submitted riding tasks together. `ancestry` requires containment in integration plus an observable merge; `content` records a byte-for-byte squash match; `human_attested` records the human's explicit fast-forward assertion. The final merge into configured integration is human-only; task-to-aggregate assembly is not a done crossing. Features and epics otherwise complete by `rollup` over their live children. Historical records without this field remain valid; event `basis`, contributor, and timestamp distinguish observation from assertion.

**States.** `draft` (narrative) → `specced` (specification, acceptance contract, initial tasks complete) → `ready` (human approval also readies initial tasks) → `active` (first successful descendant task start) → `done`. There is no feature-start command. A feature remains active through task and aggregate review; sync rolls it up only with at least one non-cancelled task, all live tasks done, and feature gates checked. All-cancelled tasks never satisfy rollup vacuously.

An active feature with all live tasks done has the derived phase `awaiting check` while any required feature gate has no recorded result, or `awaiting sync` when every required gate has one. A recorded fail or stale result counts as checked, matching advisory rollup; clean done additionally requires green gates and accepted criteria, otherwise sync records policy violations. The stored state remains active until sync and no derived phase needs clearing. New tasks beneath a done feature reopen it on sync; this extension differs from replacing shipped behavior through a new `replace` feature and supersession. Coarse aggregate merges may cross many tasks together, but feature and epic completion still roll up their children. `cancelled` is terminal with a reason.

A PR review that requests changes routes the affected task back to work, not the feature: the task moves `in_review → in_progress` (resume; the verb is in [cli-surface.md](./cli-surface.md)) while the feature simply stays `active`. `return` remains the feature's own pre-work reverse, the `ready → specced` step a human takes before any code exists.

**Accepted basis.** `accepted_against` is the feature's active-work contract: the spec and eval revisions the human approved before work began. It is recorded at `approve` (specced to ready) from the current `feature.md` and `feature.eval.md`, captured as `git_blob` RevisionRefs over the working-tree content (the blob id `git add` would store, with the path's checkin conversion and clean filters applied, so it shares the committed-blob space `verified_against` pins), so the basis is pinned the moment the human approves and an edit is detected before any commit. `status`, `serve`, `start`, `check`, and `submit` compare the current working-tree `feature.md` and `feature.eval.md` against that basis for any `ready` or `active` feature. If either file changed, Tiller reports `active_basis` drift and `start`/`check`/`submit` refuse until a human reviews the change and refreshes the basis with `approve --refresh-basis`. The record also carries a `commit` RevisionRef pinning HEAD at the moment of approval: not the state files, which are usually uncommitted then, but the code baseline the plan was approved against, the anchor drift's since-when and an amendment's diff range read from. `verified_against` remains the completion record for `done` drift; it is not the active-work signal.

**Assumption basis.** When decomposition, speccing, or task breakdown proceeds while an open question still affects the shape of the work, the affected node records that question in `assumption_basis`, with the human's reason for proceeding. An open assumption is a known risk, not an error. When the question is answered, Tiller reports `answered_question` drift for every node whose entry has not been reviewed against that answer, and `submit` refuses the task, checking the submitting task and its parent feature through the same walk-up the basis guard makes, until a human reviews the answer in with `approve <noun> <node> --reviewed-assumption <Q-id>`, which stamps the answer's revision into the node's entry. `start` and `check` stay unrefused, so the escape hatch keeps its purpose: work may proceed under a recorded assumption, it just may not ship unreviewed. The stamp records what was reviewed, not that nothing changed: if the answer changes the plan, the fix still routes through a normal edit or an amendment by blast radius.

---

## Tasks

The level where code gets written, and the terminus of the prose chain: a task's `task.md` is the implementation spec, the most concrete writing in the tree. Tasks are laid down just in time by the second decompose stage, when their feature is specified, not up front. Each task is a folder under its feature, `tasks/<task-key>/`, holding `task.md` and `task.yml`, the same prose-and-state split every other node keeps.

`task.md` says how the work will be done: the approach, the existing code it reuses (a `## Reuses` heading the `add task` stub seeds, which the CLI never reads for content), the files and surfaces it expects to touch, the tests it will add, the edge cases it must cover. It is what the human reads at the approve gate and what the executing agent works from, so the plan survives the gap between the reasoning that wrote it and the loop that performs it. A good task is a single completable unit that yields one reviewable diff.

`task.yml`:

```yaml
id: T-k3p9w6
key: form-engine/certification-form-loading/schema-transcription
title: Transcribe CERT-LTI-02-A (2025) into JSON Schema
status: done            # proposed | ready | in_progress | check | in_review | done | cancelled
blocked: false
owner: dana
started: true           # durable first-start boundary; never cleared, including after cancellation
proposed_by: dana
depends_on: []           # sibling tasks this one follows, as ids
supersedes: []           # same-noun predecessors; applied amendment or explicit transfer
branch: tiller/form-engine/certification-form-loading/schema-transcription  # recorded once at task start; unchanged on resume
started_grain: task      # task | feature | epic; the review grain resolved at first start, kept through later grain changes
superseded_by: null      # at most one reciprocal, acyclic same-noun successor
accepted_against:        # the task.md blob the human approved, pinned at approve
  spec: { kind: git_blob, value: "git:5c1e8a9", path: epics/form-engine/features/certification-form-loading/tasks/schema-transcription/task.md, observed_at: "2026-06-23T00:00:00Z" }
  commit: { kind: git_commit, value: "git:7bc1a20", observed_at: "2026-06-23T00:00:00Z" }   # HEAD at approval, same anchor the feature record carries
evidence:
  - { kind: diff, ref: "abc1234" }
  - { kind: test, ref: "spec/form_definitions/cert_lti_02a_spec.rb" }
gates:                  # same shape as feature.yml gates, written by `tiller check`
  build:
    required: true
    kind: command
    status: pass        # pass | fail | pending | skipped | stale
    run_by: dana
    run_at: "2026-06-23T00:00:00Z"
    against: { kind: git_commit, value: "git:abc1234" }
    evidence: [{ kind: command_output, ref: artifacts/tiller/gates/build-abc1234-out-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.txt }]
pr: 41
review_ref: null         # set by a remote-less submit instead of pr: the branch plus the head commit as a git_commit RevisionRef
done_basis: ancestry     # ancestry | content | human_attested; what the done crossing rested on
policy_violations: []
```

**Task branch and review provenance.** `branch` is optional only before start and in preserved terminal history from older schemas. New task starts record their own full-key-derived branch. Submission runs on that branch; offline task review and coarse riding review record its branch and HEAD in `review_ref`. Assembly includes the branch's current tip as well as the recorded review head, because the latter precedes submission-state commits. A task is never implemented on an aggregate.

**Pre-release schemas.** `tiller migrate` on a project older than schema 5 refuses while any task is `in_progress`, `check`, or `in_review`, or a nonterminal epic/feature carries a PR or review ref; finish and sync that work with the older binary first. Crossing into schema 6 converts active ownership only through a reviewed `data.ownership_review` from `migrate --dry-run --json`, applied with `migrate --ownership-review <file>`. [distribution.md](./distribution.md#pre-release-ownership-adoption) owns both procedures.

The review binds manifest, roster, inventory, and relevant source snapshots. Stale, incomplete, duplicate-ID, conflicting, or unknown-owner input refuses before writes. Conversion atomically records durable starts, removes ancestor/manifest ownership fields, and writes migration provenance including reviewed before/after corrections, preserving unrelated and unknown fields. Ready reservations remain mutable and do not create contribution. Completed tasks count as started; cancelled tasks require reliable event/state/branch evidence. Carrier-completed shelves do not acquire an invented independent start. Historical missing owners and malformed optional historical events preserve gaps/bytes rather than block safe live conversion or infer Git authorship. Legacy active features without started tasks remain active; ready parents over started tasks require explicit reconciliation.

**Ordering.** Task folders carry no inherent order, so sequence is recorded in `depends_on`, sibling task ids resolved the way every other dependency edge is. Sibling tasks render in dependency order through the same layered topological sort epics and features use, with key order breaking ties, so the execution sequence is recorded state rather than a convention of file layout. Where the recorded graph leaves that choice open, a soft order steps in: a recorded, machine-owned sequence at each grain, a feature's `order` field naming its own recorded task order, an epic's `order` field naming its own recorded feature order, and the root `.tiller/order.yml` naming the recorded epic order. `tiller order` is the only explicit replace or clear writer at all three grains and the sole writer at feature and task grain. Its rerun replaces the named parent's list wholesale, since the list states current intent rather than accumulating like `depends_on` does. Amendment approval is the one automatic exception at root grain: in the same journaled transaction that materializes staged additions, it appends only the newly materialized epic ids, in staged order, after every entry already in the root list. It never sorts or fills the list, inserts an older unlisted epic, removes a dangling id, or canonicalizes an existing entry; an approval with no `add-epic` operation does not read or write `.tiller/order.yml`. A partial list is legal and stays partial, and the siblings it names lead their unlisted peers wherever the tiers above still leave the choice open. The full precedence stack, run identically wherever siblings render or the routing walk picks a next node, is: recorded `depends_on` edges, never violated; then the soft order; then, inside an epic cycle group alone, the flow-derived rank the base feature flow implies; and canonical key ascending last. A soft order arranges rendering and steers recommendations; it never gates, so a recorded dependency edge always wins over it where the two disagree, and no legal action is ever refused for working out of the stated sequence.

**States, and who drives them.** The agent proposes a task by writing its folder (`proposed`), and a human approves it into the plan (`ready`): the initial task set rides the feature's own approval, while a task added mid-flight is approved individually, once the parent feature has crossed its plan gate. Approving pins the `task.md` blob in `accepted_against`, the same working-tree `git_blob` capture the feature gate makes, so an implementation spec edited after approval surfaces as `active_basis` drift rather than silently diverging from what the human read. The agent works it (`in_progress`), records evidence and runs the command-backed gates (`check`), and opens or attaches a PR (`in_review`); in a repo with no git remote, `submit` records `review_ref` (the branch plus head commit) instead of `pr`, the narrow fallback for dry runs and local exploration. A task whose resolved review grain is feature or epic (see Features and Epics, Review grain) records that same `review_ref` fallback even with a remote configured, because the task itself never carries a PR once its own feature or epic is the reviewed node; an explicit PR there is refused instead. The task's lifecycle is unchanged either way, `in_review` included: what changes under a coarser grain is only what that `in_review` reads as riding, the task's own PR at task grain or the feature's or the epic's PR one or two grains up. Only the human merge moves it to `done`, and `sync` records that crossing after observing the PR. If the merge crossed with red required gates under advisory enforcement, the task is still `done` and carries `policy_violations`. Approval is the human's decision, not the human's keyboard: the agent may run `approve` only on an explicit human instruction naming the nodes, and it never attests, runs `sync --accept-fast-forward`, or marks anything done (the delegation model is in [cli-surface.md](./cli-surface.md)). That rule encodes the whole stance: agent proposes, human approves, merge is the done gate.

A requested-changes review on a task PR routes it back: `in_review → in_progress` (resume; the verb is in [cli-surface.md](./cli-surface.md)), distinct from the pre-work `return` that moves `ready → proposed`. On resume the task's `pr` stays and its `gates:` entries flip to `stale` unless their recorded code identity still equals the current one (The stored gate block, below). Every attestation gate stales regardless, a gate with no recorded `code` stales, and inside a gate that is kept, each criterion with no recorded `code` or a different one stales and takes the gate with it, so a manual or rubric verdict still expires on resume.

**Supersession and completion.** Tasks carry reciprocal, same-noun, acyclic `supersedes`/`superseded_by`, with at most one successor, written by amendment application or transfer. Shelf execution units carry the same pair for transfer. Tasks do not carry `verified_against`; feature completion owns content verification and task completion records `done_basis`. Amendment `target_revision` therefore has no done-task anchor and refuses rather than comparing against nothing.

**Evidence** is how a closed task survives scrutiny. A diff, a test reference, a command output, a link. It's what makes "the agent did the work" verifiable instead of asserted.

---

## Bugs, chores, and tweaks

Choose a **Bug** for a defect: observed behavior violates an agreed expectation. Choose a **Chore** for housekeeping without code side effects, such as documentation or administrative upkeep. Choose a **Tweak** for a minor adjustment that may change code. Choose a **Task** under a feature for substantive planned work. These are choice guidelines, not file-diff restrictions: Tiller does not enforce a no-code rule for Chores.

For example, changing a completed button from "Save" to "Save changes" to improve the wording is a Tweak, even when the label lives in application code. If the button text violates its agreed requirement, record a Bug instead. A new button or a broader interaction redesign belongs in planned Task work.

Keep changes forward-only. Never reopen or rewrite done work to express a new request. Add a new Bug, Chore, Tweak, or Task with attachment or source context as appropriate; the completed record stays history. Adding planned descendants may recompute feature and epic rollups, but it does not reopen merge-backed done execution nodes.
A bug is a **work node**, not a record. It reaches `done` the one way everything else does: a PR that represents exactly one node, merged by a human, observed by `sync`. The rejected alternative, a record noun beside questions that closed by pointing at the task that fixed it, could not deliver what the queue exists for, fixing a one-line regression in a `done` feature without first minting a feature and a task under it. A bug is its own unit of work, and it ends in its own PR.

A bug is a **leaf**. It never has children. A bug that turns out to be a feature in disguise does not grow: the feature is minted through the amendment machinery and the bug is carried by it (Resolution, below).

A bug is a **sub-class of task**. It carries the task's lifecycle, the task's contract pin, the task's gates, and the task's review record, and it differs in exactly two ways: it lives on its own shelf rather than under a feature, and it may point at any node in the tree, or at nothing. The lifecycle body is one shared record the three shelf nouns embed, so `approve`, `claim`, `start`, `check`, `submit`, `sync`, and `cancel` gain a receiver rather than a grammar.

A bug is **not an amendment**. An amendment changes the work tree; a bug, chore, and tweak do not. They enter directly, they stage nothing, and `intake` and `amend` do not learn about them in v1. An agent therefore cannot propose a cancel on a bug, since there is no `amend --cancel` reaching one: it says so in prose and the human runs `cancel bug`, the accepted cost of keeping amendments out of the shelf path.

**Three nouns, one body.** `bug`, `chore`, and `tweak` are separate nouns embedding the same execution body. `bug.md` states reproduction, expected behavior, actual behavior, and fixed-when. `chore.md` and `tweak.md` state what, why, and done-when; `tweak.yml` has the same state body as `chore.yml`. Tweaks share Chore ownership, contract pinning, task-grade gates, independent review, attachment, ordering, shelf dependencies, carrier resolution, transfer, and lifecycle rules. Existing Bug and Chore records are not reclassified or rewritten.

**Tweaks.** The Tweak shelf sits beside Bugs and Chores, and a binary that predates it refuses the tree rather than silently ignoring the shelf. Tweak IDs use `TW-`; keys are `tweaks/<tweak-key>` and state/prose paths are `.tiller/tweaks/<tweak-key>/tweak.yml` and `tweak.md`. Because `tweaks` becomes a reserved epic key, migration refuses a tree holding an epic keyed `tweaks` (`migration_reserved_epic_key`) rather than rewriting keys; only an older binary can rename that epic, and only while it is draft.

**Where they live.** Each noun has its own shelf at the top of `.tiller/`, beside `epics/` and `feedback/`, and a bug lives there whether or not it is attached to anything (Directory layout). The manifest's `branch_pattern` interpolates the canonical key unchanged, so the default yields `tiller/bugs/<bug-key>`, which cannot collide with an epic's `tiller/<epic-key>`, and `bugs`, `chores`, and `tweaks` are refused as epic keys for the same reason (Keys and paths). An attached bug is deliberately not placed under the node it points at: two homes would make attachment a path, reattachment a folder move, and a mid-flight bug's branch derivation a moving target, against the rule that key moves stop at the proposal boundary. A field is a field.

**Lifecycle, contract, gates.** The states are the task's, verbatim. The `proposed` set is the triage inbox and the `ready` set is the queue. `add bug` writes the folder in `proposed`, by agent or human. `approve bug` is triage, and it pins the `bug.md` blob into `accepted_against` exactly as `approve task` pins `task.md`, so a contract edited after triage surfaces as `active_basis` drift rather than diverging silently; `return` reverses a mistaken triage. The lighter lifecycle with no triage and no pinned contract is refused: triage is where "is this a bug, is it a duplicate, does it belong to a feature" gets decided, and without a pinned contract the agent has nothing to be held to.

`start` moves `ready` to `in_progress` and records the node's own branch, computed from `branch_pattern` over the canonical key and cut from the integration branch, never from a workbench. `check` validates the active basis and runs the command-backed gates a task runs. The manifest's `on` list is read at task grain for all three shelf nouns, so a gate scoped `on: [feature, task]` applies to a bug, chore, and tweak unchanged, and the `evals` gate never does: a shelf node carries no `feature.eval.md`, and fixed-when is satisfied by a regression test, which is a `tests` gate. `submit --pr <n>` records the node's own `pr`, or its `review_ref` in a repo with no remote, and moves it to `in_review`; `resume` re-enters work after a requested-changes review. `sync` observes the merge against the node's own branch and the integration branch, ancestry first and content as the squash fallback, and records `done` with the basis it observed. Enforcement stays advisory: a merge with red required gates records `done` with `policy_violations`.

Optional review gates can be attested at bug, chore, or tweak grain because each shelf node owns its review surface. `tiller attest bug|chore|tweak --gate` writes the same existing `gates` and `attestations` shapes shown for features and tasks. No shelf criterion or eval exists: `attest bug|chore|tweak --criterion` and every `eval bug|chore|tweak` form remain unsupported. Attestation uses the existing gate state shape.

A bug ignores `review_grain` entirely. The grain calibrates oversight over planned work under an epic, and a bug is one unit with one PR at every setting.

`bug.yml`:

```yaml
id: B-7k2p9x
key: bugs/certification-form-blank-on-reload
title: Certification form renders blank after a reload mid-entry
status: ready              # proposed | ready | in_progress | check | in_review | done | cancelled
blocked: false
owner: dana
started: false             # set at first independent start; never cleared
proposed_by: dana
attached_to: F-79dh3q      # where it was found: an epic, feature, or task id; optional; provenance only
resolved_by: null          # the carrier: the epic, feature, or task whose merge fixes it; optional
resolution: null           # set only on cancelled: wontfix | invalid | duplicate | transferred
duplicate_of: null         # set only with resolution: duplicate; the surviving bug
depends_on: []             # other bugs or chores only, as ids
source: docs/feedback/2026-08-27-reload-investigation.md#L12-40   # free-text citation, the amend convention
accepted_against:          # the bug.md blob the human triaged, pinned at approve
  spec: { kind: git_blob, value: "git:5c1e8a9", path: bugs/certification-form-blank-on-reload/bug.md, observed_at: "2026-08-28T00:00:00Z" }
  commit: { kind: git_commit, value: "git:7bc1a20", observed_at: "2026-08-28T00:00:00Z" }
evidence: []
gates: {}                  # same shape as task.yml gates, written by `tiller check`
branch: null               # tiller/bugs/certification-form-blank-on-reload once started; cut from the integration branch
pr: null
review_ref: null
done_basis: null           # ancestry | content | human_attested | carried
supersedes: []             # same-noun transferred predecessor IDs
superseded_by: null        # reciprocal successor ID, at most one
policy_violations: []
```

**Attachment.** `attached_to` is exactly one optional id naming an epic, a feature, or a task. It is provenance: where the bug was found, and where the developer and the agent should look for related documentation. It drives two things and nothing else. The execute packet for a bug includes the attached node's prose (`feature.md`, `feature.eval.md`, `task.md`) as reading context, and the derived views roll it up, a bug attached to a task counting toward that task's feature and epic wherever a surface lists a node's bugs, computed on read the way an epic's `active` is and never stored.

It touches nothing else: not the branch, not the review grain, not claims, routing order, or gates. An open bug attached to a feature does not hold that feature's `done` or its epic's `archive`. It is mutable at any state including after `done`, through `link --attach` and `unlink --detach` ([cli-surface.md](./cli-surface.md)). Attaching to a cancelled or superseded node is legal and stays put, because "found in" is history. When `tiller remove` deletes the target, the reference clears to absent with a warning in the same journaled cleanup that scrubs `depends_on`, `order`, and `blocks`; unapproved proposal material never shipped, so nothing was found in it. Multi-target attachment is refused: a bug spanning two features attaches to their common ancestor, the epic, which keeps the documentation hint focused and the per-node counts honest. Targets never include questions, amendments, or other bugs, chores, and tweaks, since "duplicate of" and "regression of" are different relations with different consequences.

**Resolution.** A task has two exits, its observed merge and `cancelled`. A bug has more, and `sync` stays the only writer of `done` and `done_basis` across all of them.

Fixed by its own PR is the ordinary case: `done`, basis `ancestry` or `content`, observed. Fixed by other work in the tree uses `resolved_by`, a live carrier pointer to the epic, feature, or task whose merge fixes the bug, set by `tiller resolve`. The bug keeps its stored state, takes the derived display phase `carried`, and drops out of the queue; in the same run that records the carrier's `done`, `sync` crosses the bug `done` with `done_basis: carried`. A cancelled or archived carrier is refused at the moment it is named, since its merge will never come, and a carrier cancelled afterwards returns the bug to the queue rather than letting it vanish, as `resolve --clear` also does. Closing a bug the day its feature is minted is refused for the same reason: a promise is not a fix, and `carried` recomputes on the carrier's fate the way a rollup `done` recomputes. That recompute has a name of its own. A `resolved_by` whose carrier is cancelled, archived, or no longer in the tree derives the phase `carrier_cancelled` instead of `carried`, rendered beside the node's status as "carrier cancelled" the same way a feature's rollup phases are; `sync` then treats the node as having no carrier at all, so an `in_review` one is observed on its own branch and listed as awaiting again. The pointer itself stays on the record: clearing or re-pointing it is `tiller resolve --clear` or `--by`, a human judgment, not something a read or an observation pass decides. A terminal bug refuses `resolve` outright, there being nothing left to carry.

Not a fix is `cancel` with `--resolution wontfix | invalid | duplicate`, which a shelf node requires and no other noun accepts, and `duplicate` additionally requires `--duplicate-of` naming another node of the same noun. Duplicates are a cancel resolution rather than a carrier because "tracked over there" is a human ruling and the survivor's fate is visible on the survivor; letting duplicates ride carriers invites cascades. A regression of a `done` bug is a new bug; whether it records the earlier one is one of the bug-specific fields left to drift.

**Queue and dependencies.** The queue is a soft order at shelf grain and nothing more, `.tiller/bugs/order.yml`, `.tiller/chores/order.yml`, and `.tiller/tweaks/order.yml`, written only by `tiller order bug`, `tiller order chore`, and `tiller order tweak`, with the precedence every other order carries: recorded `depends_on` edges first, then the list, then canonical key ascending for the unlisted, and it never gates. No `severity` or `priority` enum ships in v1, since an ordered list is a priority. Dependencies stay inside the shelves: a bug may `depends_on` other bugs, chores, and tweaks only, and no tree node may depend on one, because a cross-graph edge would let the frontier be gated by the queue. "The feature must wait for this fix" is a question with `blocks`, which already exists.

**States, and who drives them.** `proposed` (captured) → `ready` (triaged) → `in_progress` → `check` → `in_review` → `done`, plus `ready → proposed` on return, `in_review → in_progress` on resume, and `cancelled` with a resolution. `done` arrives by the node's own observed merge (basis `ancestry`, `content`, or `human_attested`) or by its carrier's (`carried`), and `sync` writes it either way.

**Transfer.** `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>` is a human decision, explicitly delegable only for named work and recipient. It requires started unfinished work, a different owner, unchanged approved prose and applicable parent contract. In one guarded transaction it cancels the original (shelves use `resolution: transferred`), retains original identity/prose/owner/review/evidence/proposer, and creates a same-noun ready replacement with a new ID/key, recipient claim, copied prose, new accepted-basis path, and reciprocal lineage. The replacement is unstarted and has no branch, review, checks, attestations, evidence results, or completion basis.

Prerequisites and live position survive. Unstarted dependents redirect with deduplication; terminal dependents retain historical edges; a started unfinished dependent refuses the whole transfer. Direct question targets retain the original and carry to the replacement; ancestor holds, pauses, attachment context, and both directions of carrier relationships remain operative. No transfer grants assumptions, reviewed answers, or automatic unblocking. Authored references/events remain historical and `duplicate_of` is not replacement lineage. Unsafe relationships refuse before any writes. A repeated transfer identifies its existing successor rather than creating another.

Tiller never changes Git refs or host PRs during transfer. Task/shelf PRs must be closed without merging and replaced; coarse reviews retain the unmerged aggregate identity and require replacement completion and fresh affected checks/acceptance. Existing aggregate commits and host approvals do not disappear automatically. A review already merged requires sync and forward work instead.

---

## The eval model

An eval file is a structured contract of acceptance criteria. Each criterion has a polarity and a binding, expressed in frontmatter, with the human-readable elaboration in the body.

`feature.eval.md` for a greenfield feature:

```yaml
---
node: F-79dh3q
criteria:
  - id: C1
    statement: CERT-LTI-02-A (2025) validates against its JSON Schema
    polarity: introduce
    binding: { type: test, ref: "spec/form_definitions/cert_lti_02a_spec.rb" }
    required: true
  - id: C2
    statement: Page numbers are continuous when a parent and child render to a single PDF
    polarity: introduce
    binding: { type: command, run: "bundle exec rspec spec/pdf/page_numbering_spec.rb" }
    required: true
  - id: C3
    statement: A reviewer confirms the rendered PDF matches the source form layout
    polarity: introduce
    binding: { type: manual }
    required: true
---
```

`feature.eval.md` for a brownfield `modify` feature, where preservation is the hard part:

```yaml
---
node: F-x5q1z8
criteria:
  - id: C1
    statement: Multi-zone calculations produce correct per-zone sampling counts
    polarity: introduce
    binding: { type: test, ref: "spec/calc/multi_zone_spec.rb" }
    required: true
  - id: C2
    statement: Existing single-zone projects compute identical results to pre-change
    polarity: preserve
    binding: { type: command, run: "bundle exec rspec spec/calc/single_zone_characterization_spec.rb" }
    baseline:
      surface: "app/models/calc"
      captured_at: { kind: git_tree, value: "git:3f6a9d2", path: app/models/calc, observed_at: "2026-06-23T00:00:00Z" }
      artifact: "spec/fixtures/calc/single_zone_2026-07-01.json"
    required: true
---
```

**Polarity.** `introduce` asserts new behavior is present. `preserve` asserts existing behavior is unchanged. Greenfield is almost all `introduce`. Brownfield leans on `preserve`, and that's where "don't break production" becomes a first-class, gated acceptance condition rather than a hope. A required `preserve` criterion carries an explicit pre-change baseline: the source surface, the commit or content fingerprint where it was captured, and the fixture, golden output, characterization test, or command output that embodies the old behavior. `tiller spec feature` and `tiller check` reject a required preserve criterion that lacks that baseline. The baseline's `artifact` names ONE repo-relative file, never a list: the paths a preserve criterion runs belong in its binding's own `run` or `ref`. `tiller spec feature` refuses an artifact carrying a separator or whitespace as misuse naming the criterion, and the drift sweep's orphan_eval check reads a pre-existing list shape as unresolvable, an uncovered row rather than a false missing candidate.

**Binding.** `test` (test files or ids), `command` (a check to run), `manual` (a human verifies), or `eval` (an AI-graded rubric the agent evaluates outside Tiller). The binding is what connects a criterion to a gate. `tiller spec feature` validates the contract before it records it: the file must carry at least one criterion, and every criterion must have an `id`, a `statement`, a recognized binding `type`, and the field that binding requires (a `command` needs a `run`, a `test` a `ref` or `run`, an `eval` a rubric in `run` or `ref`). Criterion ids must be unique within the contract, because results are keyed by id and a repeated id would let one criterion's result hide another's, including a failure. A contract with no criteria, a repeated criterion id, an unrecognized binding, or a criterion that is just prose is refused as misuse (`eval_contract_invalid`) naming the offending criterion, not silently accepted. Every path that reads a feature's contract (`spec`, `check`, `eval`, rubric recording, and criterion attestation) applies the same validation before running a binding or writing state. `tiller eval <noun> <node>` reads the criteria, runs the `test` and `command` ones, marks `manual` as pending-human, and emits a rubric packet for each `eval` criterion. It may record agent-supplied rubric output as evidence, but it never grades that output itself. A required `eval` criterion still needs human attestation after the human reviews the rubric evidence. A node is clean only when every required criterion is satisfied; an agent-recorded eval result by itself never satisfies a required criterion.

**Binding refs.** A `test` binding's `ref` is one repo-relative path or test id, or a YAML list of them (`ref: [spec/a_spec.rb, spec/b_spec.rb]`). A single ref is written back as a scalar, so one-file contracts are unchanged on disk. A blank list item is refused as misuse naming the criterion. An `eval` binding's rubric `ref` names one file, and a list of two or more is refused. `command` and `manual` bindings ignore `ref`. A whitespace- or semicolon-joined string such as `"spec/a_spec.rb spec/b_spec.rb"` is still read as one ref, and drift reports it as unresolvable rather than guessing at the split.

**Test command resolution.** A `test` binding's nonempty `run` is the complete command, even when `ref` is also present; Tiller does not append the ref to an explicit run. Without `run`, Tiller composes `gates.required.tests.run`, a space, and every item of the binding's `ref`, space-joined in order. With no tests runner, each item is recorded as its own pending `test` evidence row. A bare test ref with no configured tests runner remains `pending`. The shell runs the resolved command from the project root. This is command composition, not test-runner detection: Tiller does not interpret a filename's extension or establish that a configured wrapper forwards test selectors.

Ref-only bindings therefore require a tests command that accepts the given file or test id, such as `bundle exec rspec`. An aggregate `bin/ci` wrapper that ignores arguments would rerun the full pipeline for every criterion. Keep that aggregate command as the ordinary tests gate and use a targeted binding instead, for example `{ type: test, ref: "spec/form_spec.rb", run: "bin/rspec spec/form_spec.rb" }`. Include any necessary environment, database, fixture, or asset preparation in that explicit command or the existing runner; acceptance must not rely on another criterion or a prior CI run preparing it. The [specify skill](./skill-set.md#specify) owns checking these assumptions while authoring the contract.

**Where results land.** The eval criteria are the contract; the results of evaluating them are stored, not in the eval file, but in the node's `gates:` block under the `evals` gate. That gate carries a `criteria` map keyed by criterion id, each entry holding a per-criterion `status` (`pass | fail | pending | needs_attestation | stale`) and its `evidence`, plus the gate-level `status` DERIVED from that map: `pass` when every required criterion is satisfied (an explicit `pass`, or an attested `pass`), `fail` when a required criterion failed, and `pending` otherwise (a required criterion still `pending`, `needs_attestation`, or `stale`). `needs_attestation` is the status unique to this map: it marks a required `manual` or `eval` criterion whose machine-checkable part is settled but which still awaits human sign-off. `stale` is the reopen marker. When work lands beneath a node whose criteria were already settled, the gate stales and every criterion under it stales with it, so a human's attestation never certifies work it did not see. A resume that finds the node's code unchanged is the one narrower case: a command or test criterion whose recorded `code` still equals the current identity keeps its `pass`, while any criterion with no recorded `code`, a manual or rubric one among them, stales and takes the gate with it. The next `check` recomputes a machine-bound criterion and returns a required manual one to `needs_attestation`, which is the honest state: someone has to look again. That sign-off is recorded in the node's `attestations` list, each entry naming the `target` (a criterion or a gate), the attesting human in `by`, the `status` they set, the timestamp `at`, and an `evidence_ref`. Each entry also carries an `id` prefixed `AT-`, minted by the compact token rule as a separate non-node identifier class, the same way the event stream uses `EV-`. An attestation flipping a criterion to `pass` is what clears its `needs_attestation`. Both `tiller eval` and `tiller check` produce this same criteria-aware result for the `evals` gate, preserving any human-settled criterion across re-runs, so `check` never clobbers an attested `evals` result back to `pending`. A `command` or `test` criterion also records the `code`, `config`, and `reused_at` fields described under The stored gate block, and `check` and `eval` reuse a passing one on unchanged code and an unchanged resolved command; `manual` and `eval`-kind criteria carry no such fields and keep their carry-forward rules. The contract lives at feature grain only: an epic has no eval file and no gate of its own, its bar being that every feature under it satisfied its contract. The schema is canonical for these stored shapes; the verbs that write them are in [cli-surface.md](./cli-surface.md).

The accepted `feature.eval.md` blob is also part of the result basis. When `approve feature --refresh-basis`, or an amendment's `revise` of the feature, accepts different eval content, Tiller expires the prior eval gate and every stored criterion by setting them to `stale`, even when a criterion keeps the same id. The same transaction expires the `evals` gate of every unfinished child task (any status but `done` or `cancelled`, read from the current task file, and in an amendment after the batch's own cancels), since a task's criterion verdicts were recorded against the same contract. Evidence and attestations remain as history, but they cannot satisfy the changed contract. The next evaluation recomputes mechanical criteria, returns required manual criteria to `needs_attestation`, and returns rubric criteria to `pending` until new rubric evidence is recorded and attested. A spec-only change expires nothing, and done or cancelled tasks keep their recorded verdicts. The refreshing event names each expired child in its targets and files and lists their ids under `acceptance_expired`; a child changed concurrently refuses the whole refresh.

**The rubric handoff.** An `eval`-binding criterion is graded by the agent's harness model, not by Tiller, so the two exchange two small JSON payloads, each carrying a stable `schema` id a consumer branches on rather than guessing the shape. `tiller eval <noun> <node> --emit-rubrics` writes a rubric packet, one entry per `eval`-binding criterion, naming the `output_path` the agent must write its graded result to. The packet is `tiller.rubric_packet.v1`:

```json
{
  "schema": "tiller.rubric_packet.v1",
  "node": "F-79dh3q",
  "noun": "feature",
  "generated_at": "2026-06-23T00:00:00Z",
  "criteria": [
    {
      "id": "C4",
      "statement": "The generated copy reads naturally for a homeowner",
      "polarity": "introduce",
      "required": true,
      "rubric": "Grade whether the copy reads naturally and addresses the homeowner directly.",
      "output_path": "artifacts/tiller/evals/F-79dh3q-C4.json"
    }
  ]
}
```

The agent grades each criterion and writes one `tiller.rubric_output.v1` document to that `output_path`, then records it with `tiller eval <noun> <node> --criterion <id> --record-rubric-output <file>`:

```json
{
  "schema": "tiller.rubric_output.v1",
  "node": "F-79dh3q",
  "criterion": "C4",
  "verdict": "pass",
  "rationale": "The copy reads naturally and addresses the homeowner directly.",
  "graded_by": "harness-model",
  "graded_at": "2026-06-23T00:00:00Z"
}
```

`schema`, `criterion`, and `verdict` (`pass | fail`) are required; `node` is optional but, when present, must match the node being graded; `rationale`, `graded_by`, and `graded_at` are optional. Tiller validates the output against the contract (the `schema` id matches, the named `criterion` is an `eval` binding on this node, the `verdict` is `pass` or `fail`) and rejects a malformed one as misuse the agent fixes by regrading. A `pass` verdict moves the required criterion to `needs_attestation`, never straight to `pass`: the recorded output is stored under the criterion as `{ kind: rubric_output, ref: <output_path> }` evidence, and only a human `tiller attest --criterion` clears it. The recorded rubric output is the only artifact an attestation's `evidence_ref` points at; a `manual` criterion has no graded file, so its attestation carries no `evidence_ref` rather than a path to a file that was never written.

---

## Gates

Gates aggregate per node. Command gates run at the grains the manifest's `on` lists name, the acceptance contract enters at feature grain, and review gates follow the node that owns the review surface. A bug, chore, or tweak always owns its own review surface.

- **Required gates define clean completion.** Build, tests, evals. Each is command-backed: Tiller can run a configured check and read the result. The agent runs these in its own loop and hands Tiller the result plus evidence, and Tiller can re-run on demand when it does not trust the recorded result. Shelf work uses its task-grade build and tests gates and has no eval contract.
- **Optional gates are visible but non-blocking.** Code review, security review, performance review, and accessibility review. Each is attestation-backed: an external tool or a human sets the status, and Tiller never invokes the reviewer. They can be attested on a feature, task, bug, or chore review surface. A red optional gate does not stop a merge. It just sits there un-green, which is the nudge.
- **Enforcement is advisory.** Tiller shows red and green and trusts the humans to honor it. If a node crosses to `done` while a required gate is not green, `sync` records the merge-backed `done` state and also records a `policy_violations` entry naming the gate, the observed commit, the observed status, and the evidence. "Not green" is read literally against the required gate set: a failing build or test, and equally a `pending` `evals` gate whose required criterion still awaits attestation, each records a violation. A clean `done` has every required gate `pass` and records none. Tiller-backed CI and branch-protection checks are rejected by the [managed-project independence contract](./distribution.md); project commands remain independently runnable, while local Tiller may invoke them to record evidence.

**The stored gate block.** `tiller check` writes the result of each gate into a `gates:` block on the node it runs against: `feature.yml`, `task.yml`, `bug.yml`, or `chore.yml`. The schema is canonical for that block. Each gate carries `required`, its `kind` (`command` or `attestation`), a `status` from the enum `pass | fail | pending | skipped | stale`, the `run_by` human and `run_at` time, the `against` RevisionRef naming the revision the gate ran over, and an `evidence` list of `{ kind, ref }` pointers into `artifacts/tiller/`. Because a gate runs over the working tree, `against` is honest about what that tree was: when the tree is clean it is the HEAD `git_commit` (the commit contains exactly what ran); when the tree is dirty, which is the usual case for `check` since the code under test is uncommitted, it is a `sha256` working-tree fingerprint, so the recorded provenance never claims a gate ran over code a commit does not contain. That fingerprint covers the whole repository in repository-root coordinates even when the Tiller project is nested: HEAD plus every path that differs from HEAD (a rename counts as its deletion and its addition) and every untracked, nonignored file, each with its Git blob id (a symlink's is its target's). Paths are read NUL-delimited and verbatim, so `core.quotepath`, non-ASCII bytes, tabs, newlines, and edge spaces cannot change a path's identity. A deleted path records a deletion sentinel; a changed path that exists but cannot be read fails the run with `worktree_unreadable` rather than reading as deleted. No index, object, stash, or ref is written. `pending` is a gate not yet green, either not yet run or run but still awaiting a required attestation; `skipped` a non-required gate the run passed over; and `stale` a recorded result whose code identity no longer matches the current one (see the resume rule under Lifecycles), or a `pass` that a `check --gate` run left out and could not reuse (Gate reuse, below). Feature/task eval support additionally carries the per-criterion `criteria` map described under The eval model and a status derived from it, and records its evidence there per criterion rather than in a gate-level `evidence` list. Bugs, chores, and tweaks never carry that criteria map because they have no eval contract. `policy_violations` entries are the matching record for a required gate that was not green at the crossing: each names the gate, an observed RevisionRef, the observed `status`, and an `evidence_ref`.

For a command gate, the stored result and its `command_output` evidence reference are one persistence invariant. Tiller writes the captured output before committing the node and event batch. If the evidence write fails, the command returns `evidence_unwritable` and leaves the prior gate state unchanged, so a `pass` can never name a missing evidence file. The evidence filename has the shape `<gate>[-<against>]-out-<full-output-sha256>.txt`; the full output digest prevents concurrent runs at one revision with different output from overwriting each other.

**Gate reuse.** A command gate carries four more optional fields: `code`, the sha256 code identity it ran over; `config`, the sha256 hash of its `kind` and its resolved `run` (after `${node.*}` interpolation); `reused_at`, set when a later `check` reused the result instead of running it; and `reused_from`, the source node id when the reused record came from another node. A `command` or `test` criterion under the `evals` gate carries `code`, `config`, and `reused_at` the same way, its `config` hashing the resolved command, which for a `test` binding includes the tests runner a ref-only binding appends to. `against` keeps its meaning, the revision the command actually ran over; `code` is what reuse compares. A reused record keeps the original run's `run_by`, `run_at`, `against`, and `evidence`, so it never claims a fresh run.

`check` reuses a recorded result instead of running its command when the result is a `pass` and both its `code` and its `config` equal the current ones. Only a `pass` is ever reused: a `fail`, a `pending`, and a `stale` result always rerun. The sources are the node's own `gates:` block and, for a feature check, the `gates:` blocks of its tasks, which are present on the aggregate because task branches merge in with their state commits; no other branch, node, or history is read, and no cache lives outside the tree. A gate declaring `reuse: false` in the manifest never reuses, and `check --rerun` or `eval --rerun` forces real runs for one invocation. The `evals` gate is never shelled as a whole, so reuse there is per criterion, and the gate's status stays derived from its criteria. A record written before this field existed has no `code` and is simply not reusable. A gate that `check --gate` leaves out is never shelled: it is reused under the same rule or, when it cannot be and its recorded result is a `pass`, that result turns `stale`, keeping its evidence and provenance; inside an `evals` gate only the command and test criteria a full check would rerun turn `stale`, and a manual or rubric verdict is kept as a full check keeps it. A status-only reader (`submit`, `sync`, the readout) therefore never takes it for a pass over the current code. Reuse adds no schema version: the fields are optional, and an older binary that rewrites a node drops them, which costs a reuse and nothing else.

**Code identity.** The code identity is a domain-versioned sha256 over the sorted list of (path, mode, Git blob id) for every file in the checkout that Git tracks or would offer to track, excluding paths under the project's own `.tiller/` directory and its `artifacts/tiller/` evidence directory, in repository-root coordinates like the fingerprint above. The project's directory is the nearest ancestor of the working directory that holds `.tiller/`. It is built from HEAD's tree overlaid with every path that differs from HEAD and every untracked, nonignored file, each hashed in-process the way the fingerprint is; a deleted path drops out, and on an unborn HEAD the staged files count. It reads content, not commits, so when an identity is available the same files give the same identity whether they are committed, staged, or dirty, and Tiller's own state commits between two runs do not move it. Nothing is written to the index, object store, or refs. Its blind spots are the fingerprint's and are deliberate: ignored files (`.env`, dependency directories, build caches), environment variables, tool versions, and the network are invisible to it, while a tracked lockfile makes a dependency bump visible. Git's normalized diff can hide working-tree byte changes caused by check-in conversion. Reuse is therefore conservatively disabled when `core.autocrlf` enables conversion or effective attributes on nonexcluded HEAD paths permit text, line-ending, filter, ident, or working-tree-encoding conversion. Configured clean or process filter drivers literally named `unset` or `unspecified` also disable reuse, since Git's attribute output cannot distinguish those names from its disabled-attribute sentinels. New indexed files on an unborn HEAD and untracked files are read raw. Conversion-enabled repositories may rerun gates even with unchanged bytes rather than risk reusing a pass over different executable input. The identity domain version retires records produced before this conversion check existed. `reuse: false` and `--rerun` cover deliberate blind spots. A repository inside the checkout is a blind spot it does not accept: a submodule's gitlink records only a commit, and an untracked nested repository reads as one directory, so edits inside either would never move the identity. When HEAD's tree (or, on an unborn HEAD, the index) carries a gitlink outside the excluded directories, or an untracked nested repository stands in the checkout, there is no identity, so every gate runs and a resume stales every gate. Without Git there is no identity, and an empty identity never matches anything, so nothing is reused.

The HEAD shortcut is also disabled when `core.filemode` is false or a nonexcluded indexed path has hidden or unsupported index flags, including assume-unchanged and skip-worktree. Tiller never clears those flags. The identity domain retires passes written before these checks, even after the unsafe settings have been removed. Before the first commit, working-tree provenance includes indexed paths as well as untracked nonignored paths and reads their current bytes, not merely their staged blobs.

Recovered merge-backed crossings also retain historical warnings from the reviewed head. `policy_violations` adds `source_revision`, `unresolved_criteria`, and `evidence_refs` alongside the current crossing's `observed` revision. Historical and current crossing-time violations are separate facts; deduplication uses source revision, owning node, and gate. Task observations attach to the task crossing; feature observations attach to participating features, including delayed rollup. Epic owns no gates. Later approved criterion removal/current pass neither erases nor restores the historical criterion, and completion fingerprints still describe integration at sync.


---

## Plans (semi-ephemeral)

Dated, intent-named execution plans. A plan is how an owner intends to take a chunk of features-to-tasks through a stretch of work. It is not canonical, it is not a spec, and it is allowed to go stale and get deleted. The execute skill writes one when work spans multiple tasks or carries unusual risk; `orient` may read active plans as context; `tiller ls plans` and `tiller show plan <file>` expose them, but no lifecycle command depends on them. Light frontmatter links a plan to the nodes it touches:

```yaml
---
date: 2026-07-01
title: Form engine kickoff
touches: [E-j8m4q2, F-c2m8n5, F-79dh3q]
status: active        # active | done | abandoned
---
```

---

## Decisions and open questions

Two artifacts.

**ADRs, unbuilt design.** `decisions/adr/<adr-id>-<slug>.md` is the designed location for a short decision record, its context, and its consequences. The ADR node surface is not implemented; amendment `implements` remains optional.

**Open questions** in `open-questions.yml`, the owned, date-gated discipline, the thing most teams leave as buried prose:

```yaml
questions:
  - id: Q-h2n7m5
    title: Audit selection unit
    statement: Does the audit selection count projects or project-phases?
    owner: robin
    due: { date: 2026-08-15, hint: "end of week 6" }   # forms: date (YYYY-MM-DD), hint, in either mix
    blocks: [E-a7c0f3, E-d3g6j9]          # Audit Engine, Payments (an epic, feature, or task)
    status: open                      # open | answered | cancelled
    answer: null
    answer_revision: null             # content revision of the accepted answer, set by `tiller answer`
    source: "(99-open-questions.md 1.10)"
  - id: Q-k4p8w2
    title: Legacy export format
    statement: Does the audit export keep the legacy CSV format?
    blocks: [E-a7c0f3]
    status: cancelled
    cancel_reason: The legacy export was retired.   # why no decision is needed, set by `tiller cancel question`; absent otherwise
```

`question.blocks` is the only durable question-to-work relationship. It records forward edges in
ledger order and remains present after the question is answered or cancelled.

A question leaves `open` one of two ways. `tiller answer question` records a decision: `status: answered`, the `answer`, and an `answer_revision` that dependent features review in. `tiller cancel question` records that no decision is needed: `status: cancelled` and the required `cancel_reason`, with no answer and no revision. Both are final, and only an open question takes either. A cancelled question, its `blocks`, and any feature `assumption_basis` entry naming it stay on record but count for nothing: it holds no plan, and it raises no `answered_question` drift because no answer ever arrives. `tiller link question` and `tiller unlink question` add or remove `blocks` targets on an open question only; once a question is answered or cancelled its `blocks` is history. `add question` deduplicates by exact statement against open and answered entries but skips cancelled ones, so re-adding a cancelled question's statement mints a new open question. No reverse relationship is stored
on an epic, feature, or task.

The generated blocker projection owns the reverse relationship. It derives operative
`blocked_by_questions` edges from open questions only, inherits an epic hold through its feature
and task descendants and a feature hold through its tasks, deduplicates one question that names
several levels of the same ancestry, and orders the result by question id. At feature grain, that
feature's recorded `assumption_basis` suppresses the corresponding reverse edge. Task grain
deliberately does not inherit its parent feature's assumptions, so the same question can remain an
operative task hold. Each operative hold appears against the current execution assignment it
holds and in the addressed node's status. Held unclaimed tasks are diagnostic work, not eligible
execution candidates. `status`, `start`, `check`, and `submit` consume that same projection, so forward
and reverse surfaces cannot disagree about whether a hold is operative at the grain being read.

Every new question write requires both fields: a non-blank, single-line `title` short enough to scan and the full non-blank `statement`. Direct `add question` and amendment `add-question` operations refuse a missing title rather than deriving and storing one. Legacy rows without `title` remain loadable. Read projections render a display-only fallback by collapsing the statement's whitespace to one line, retaining at most 80 Unicode code points, and ending a truncated value with a Unicode ellipsis. The fallback never rewrites the ledger, so loading or displaying old state cannot silently invent persisted content.

---

## Evolving a project

A project of any complexity changes its mind mid-flight. A design proves wrong, scope surfaces that discovery missed, an implementation needs a structural rework. Tiller meets a large late-breaking change with the same machinery it uses for everything else, and the framing is the brownfield principle turned inward: the change is brownfield, where the brownfield is the team's own recent code. The form engine built three weeks ago becomes the as-is layer, the redesign becomes the to-be, and the change decomposes into features with `change_type: replace` or `migrate` that name the existing surface and carry `preserve` criteria alongside the `introduce` ones. The forward process is not special. The tool eats its own output as input, which is what "brownfield is the general case" was always promising.

Two rules keep that safe.

**History is append-only.** Done work is never rewritten to accommodate a change of mind. Merge-backed done, a task, stays done, because done is a merge and you do not un-merge. A change to behavior a task already shipped is new forward work: new tasks, or a new feature with `change_type: replace` that names the old feature's surface and records `supersedes` against it. Rollup-backed done, a feature or an epic, is not an exception so much as a computation: the rollup recomputes, so new work landing beneath a done node returns it to work with no ceremony. Reopening and superseding stay distinct on purpose. Reopening extends a node's own contract with more work; superseding replaces shipped behavior and records the lineage, and only the second keeps history honest about what replaced what.

**Preservation is the safety net.** The hard part of a rework is not breaking what already works. `preserve` criteria are how that becomes gated rather than hoped: each binds to a characterization test that captures the current behavior before the change, so the refactor proves it kept faith with the behavior users depend on.

### Amendments

A small change needs no ceremony: add a feature, approve it, work it. A large change is different, because it operates on existing nodes, cancelling not-yet-done work, superseding done work, revising a non-terminal feature or task's spec or acceptance contract in place, and those operations have no `draft` state to stage in the way a brand-new node does. The **amendment** is the artifact that holds them, a reviewed and recorded change to the plan. It is the proposal unit for a reshape the way the draft tree is the proposal unit for an initial decomposition. Unlike the epic, feature, and task it operates on, an amendment is not folder-backed: a staged reshape is one reviewed document rather than a subtree, so it lives as a flat file pair under `amendments/`, `<A-id>-<key>.yml` for the state and `<A-id>-<key>.md` for the narrative.

`amendments/A-d6w3f9-form-engine-rework.yml`:

```yaml
id: A-d6w3f9
key: form-engine-rework
title: Restructure form engine from embedded JSON to discrete tables
status: proposed              # proposed | approved | withdrawn
implements:                   # optional: the ADR this carries out, left empty until an ADR surface exists
triggered_by: [design/04-form-engine.md]      # the revised design whose change prompted the reshape
amends_design: [design/01-overview.md, design/04-form-engine.md]   # the durable design docs this approval keeps current
base_revision: git:3f6a9d2    # repository revision the staged operations were reasoned against, recorded at open from HEAD
# no `source:` key: this amendment was opened by hand (`tiller add amendment`), not minted by `tiller intake`.
# An amendment `tiller intake` mints instead carries source: { path: .tiller/feedback/opus-review.md, revision: git:9f86d08 }.
operations:
  - op: add-epic
    key: print-layouts
    title: Printable form layouts
    source: docs/feedback/opus-review.md#L5-9   # legal on every op kind; required once the amendment above carries source:
  - op: add-feature
    key: form-engine/table-backed-forms
    title: Table backed forms
    change_type: replace
    preconditions: { key_absent: form-engine/table-backed-forms }
  - op: add-task
    key: form-engine/rendering-modes/table-aware-rendering
    title: Render from the table model
    preconditions: { key_absent: form-engine/rendering-modes/table-aware-rendering }
  - op: add-question
    title: Legacy offline fallback
    statement: Keep the legacy embedded-JSON fallback for offline mode, or drop it once the table model ships?
    blocks: [form-engine/table-backed-forms]
  - op: supersede
    target: F-79dh3q
    by: F-x5q1z8
    preconditions: { target_status: done, target_revision: git:7bc1a20 }
  - op: cancel
    target: T-p7kknm
    reason: "subsumed by the table model"
    superseded_by: form-engine/rendering-modes/table-aware-rendering
    preconditions: { target_status: check }
  - op: revise
    target: F-n0c3v7
    preconditions: { spec_revision: git:3e9293c, eval_revision: git:1172ab9 }
  - op: correct-eval
    target: F-79dh3q
    reason: "loader tests moved into two files"
    retire: [C-4]             # criterion ids this correction deliberately removes; omitted when none
    preconditions: { eval_revision: git:5c0d2e1 }
```

The companion `A-d6w3f9-form-engine-rework.md` carries the narrative: why the reshape, what changes at a glance, what the team should watch. The prose-and-state split holds here as everywhere, and with `implements` optional the narrative is where the rationale actually lives.

**Nine operations, at epic, feature, and task grain.** `revise` marks non-terminal work whose spec or contract the reshape re-scopes rather than retires, its preconditions pinning the content the change was reasoned against. It re-pins prose, never dependencies, and never re-pins done or cancelled work; changes after a terminal result are new forward work. `supersede` retires done work and names its replacement. `cancel` closes unfinished work with a required reason and optional `superseded_by`; unfinished replacement uses cancellation, not supersession. These three operations target features or tasks, never epics. `correct-eval` is the ninth, described below, and targets a feature.

`add-feature` and `add-task` stage nodes shaped like direct additions for approval to materialize. `add-epic` stages a root with a key and title, no parent, and no need for a `key_absent` precondition beyond its own key. `add-question` stages an open question with a required full `statement` and short `title`, no key, and `blocks` references to existing nodes or keys of add operations already staged in this amendment, resolved to minted IDs on materialization.

An `add-task` parent must already be a materialized feature. A new feature lands first; its tasks then follow through ordinary just-in-time specification, not a mandatory second amendment. An `add-feature` or `move-feature` may name a live epic or an `add-epic` staged earlier in the same amendment. Reverse staging order refuses as though the epic did not exist. There are no amendment link, unlink, or order operations.

**Reviewed moves preserve identity and acceptance history.** `move-feature` records `target` as the existing feature ID, `key` as the destination canonical epic/feature address, and a required `reason`. Draft, specced, and ready features may move with proposed or ready tasks only when neither the feature nor any descendant has execution/review history, including retained start events. Task claims stay intact. Both parents must be draft or ready and have no open affected aggregate review; completed siblings and their historical branches do not prevent moving an eligible feature.

The typed `move` snapshot records `source_key`, `source_epic_id`, `destination_epic_id` (or `destination_staged` plus `destination_epic_key`), effective `review_grain`, `materialize_review_grain`, `order_effect`, subtree `files`, inherited `policies`, and `reference_warnings`. File fingerprints cover contents, membership, accepted bases, task claims, and modes. Approval compares the snapshot again and rejects changed constraints, policy, claims, starts, occupied or unsafe destinations, conflicting pending operations, and actual execution cycles. Grouping-only epic cycles remain advisory. Multiple disjoint moves and an earlier staged destination epic are legal; duplicate moves and revise/cancel/supersede/add-task operations on the same moved subtree are not.

A move preserves lifecycle states, stable IDs, local task keys, owners, dependency identities, source provenance, lineage, authored Markdown and accepted revision pins. Historical `RevisionRef.Path` values are not rewritten: active contract reads resolve the stable node identity and contract slot to its current path. A move neither approves a destination epic or specced plan nor clears prose drift. The effective review grain remains unchanged, materialized as an explicit feature override when needed, including `epic`. Effective inherited prerequisites and question holds must remain identical; correct those constraints separately before restaging.

Cross-parent moves remove the feature ID from source soft order and append it to destination order in staged move order; same-parent local-key changes preserve position. Ordinary sync recomputes rollups later, without manufacturing completion. Files, current locator remaps, parent order, `move.feature` events and amendment approval share the existing generation-checked rollback journal. Move events retain old/new canonical keys, stable parent and feature IDs, descendant task addresses, actor, reason, amendment ID and operation index. Relationship effects distinguish the source and destination parent identities from today's ancestor rollup. Authored current path citations, relative links and acceptance bindings are surfaced for review, not rewritten; historical pins and captured inventories are not mistaken for broken current locators.

**A reviewed eval correction re-pins the verified contract.** A done feature's completion record pins the contract it was verified against (`verified_against.eval`), so editing `feature.eval.md` afterwards, even only to move a test, leaves `stale_spec` reporting the eval leg. `correct-eval` is the reviewed way out. It records `target` (a feature), a required `reason`, and `retire`, the criterion ids the correction deliberately removes (staging refuses `retire` on any other operation, as `retire_misplaced`). Staging refuses a non-feature target (`correct_eval_not_feature`), a feature with no `feature.eval.md` in the working tree (`correct_eval_no_contract`), and a blank reason (`reason_required`). Approval judges, in this order, after a stale `eval_revision` precondition (the exit `1` answer):

1. A feature target (`correct_eval_not_feature` otherwise, a hand-edit shape) that is done (`correct_eval_target_not_done`).
2. One correction per feature per amendment (`correct_eval_duplicate_target`).
3. The staged `eval_revision` pin on the operation. Without it (`correct_eval_unpinned`, a hand-edit shape), nothing ties the judged bytes to the reviewed ones, so the human withdraws and restages.
4. A completion record that pins an eval contract (`correct_eval_unverified`).
5. The verified contract read back from Git by the pinned blob. If Git cannot read it, because it is not in the object store or is not a readable blob, approval refuses with `correct_eval_base_unreadable`. Commit an uncommitted earlier correction or done crossing first.
6. A `feature.eval.md` in the working tree (`correct_eval_no_contract`) whose hash still equals the staged pin. A contract that moved during approval is the exit `1` stale answer.
7. A contract that differs from the verified one (`correct_eval_no_change`).
8. Both contracts parse: `correct_eval_base_unreadable` for the verified one, and `eval_contract_invalid` for a corrected one that does not parse or is not a valid contract.
9. Only what a correction may change: bindings and `baseline.artifact`, plus the Markdown body. The node, the set of criterion ids, and each kept criterion's statement, polarity, required flag, `baseline.surface` and `baseline.captured_at` must be unchanged, and a criterion may leave the contract only when `retire` names it, and `retire` may name only a criterion that left (`correct_eval_changes_contract` for any violation). A new criterion is changed acceptance and belongs to forward work.

When it passes, approval re-pins `verified_against.eval` to the working-tree blob and emits one `correct-eval.feature` event recording the old and new eval blobs, the reason, and any retired ids. Nothing else on the node moves: status, `accepted_against`, and gates stay as they are, and no gate result is expired. The drift sweep then finds the eval leg matching again; the spec leg and the surface leg of `stale_spec` are unaffected.

**Provenance on the amendment runs both ways, and a third field runs backward again.** `triggered_by` names the docs whose change prompted the reshape, the same backward-looking provenance a node's `source` records. `amends_design` names the durable design docs the reshape obliges, the intent edits a human folds in alongside the tree changes. `source`, present only on an amendment `tiller intake` minted, is a third and more literal kind of backward provenance: the one fingerprinted feedback document (`path` plus a `git_blob`-style `revision` token) the whole batch was exploded from, distinct from `triggered_by`'s looser "this design changed" pointer. An intake-minted amendment requires every one of its operations to carry its own `source` too, free text by convention `path#Lstart-Lend` against that same artifact, so the batch traces at both grains: which document prompted the reshape, and which lines in it justify each staged op. Approve validates none of the three semantically, because Tiller never parses a design doc or a feedback document: the record is the accountability, and it is what a later read has to compare against. `source`'s revision is the one exception that IS compared, not parsed: approve refuses if the landed artifact's current content no longer matches the fingerprint intake recorded (see Evolving a project's own provenance thread and [cli-surface.md](./cli-surface.md)).

**Preconditions guard the reviewed assumptions, and they name five things.** `target_status` is the state the operation was reasoned against. For a supersede, `target_status=done` is an optional stale guard; approval requires the target to be done whether or not that precondition was staged. `target_revision` is the target's own recorded commit anchor, `verified_against.commit` on a done feature and `accepted_against.commit` on anything earlier. `key_absent` is an add's claim that nothing holds the key yet. `spec_revision` and `eval_revision` pin the working-tree content of the target's `feature.md` and `feature.eval.md` (a task's spec leg is its `task.md`, and a task has no eval leg at all). Staging a `correct-eval` pins `eval_revision` itself when none is supplied. A precondition that cannot be compared is reported as its own reason rather than as a blank mismatch, which is what keeps a stale report honest.

One limit is structural and stated rather than worked around: a **done task records no completion anchor**, since a task's `done` is the merge itself and the completion record lives one grain up at the feature (see Tasks). So `target_revision` staged against a done task can never hold, and the refusal says exactly that; use `target_status=done` when the reviewed batch should also prove that a done task's state has not moved. Closing the gap is a schema decision this document deliberately does not make here, a task-grain completion anchor written at the merge-observed crossing, which would give the marker something to compare and would widen what `stale_spec` can sweep at task grain at the same time.

**Staging is a proposal; approve is the only act that executes.** While an amendment is `proposed`, every operation recorded against it is a proposal. Approving validates all preconditions before writes and reports stale snapshots. Adds materialize first, followed by supersessions, cancellations, revisions and reviewed moves, all as in-memory planning for one journaled transaction. New epic IDs append to root soft order in staged order. Contradictory move operations refuse rather than depending on operation order. Event writes precede the amendment's final approved status marker.

**Atomicity, said honestly.** The apply is one journaled transaction against concurrent mutations and against its own write errors: every file is planned, written atomically, and rolled back as a set if any single write fails. It is not a defense against the machine dying mid-write, and the ordering above is what bounds that case: because the amendment's status write is last, the only half-state a killed process can leave is operations landed under an amendment still `proposed`. The recovery is to approve again. Every operation that already landed refuses loudly rather than reporting work this run did not do, an add whose node now exists and a cancel of an already-cancelled node each refusing by name, while a supersession and a basis re-pin are idempotent and land the same values again.

**A non-terminal revision re-pins the basis in the same act.** The approval is the human review of the edited spec and contract, so applying a `revise` to non-terminal work re-records the target's `accepted_against` from the current working tree, and prior eval-gate results stale exactly as a basis refresh stales them, unfinished child tasks included (the verb is in [cli-surface.md](./cli-surface.md)). Without that the batch would land leaving an `active_basis` refusal standing on work the human had just approved. Nothing about a revision moves lifecycle state, at either grain. Done and cancelled work cannot be revised; a change after either terminal state is recorded as new forward work.

**The `supersedes` link has exactly one write path, and so does `superseded_by`.** An applied amendment writes both directions of one supersession edge in the same act, the target onto the replacement's `supersedes` and the replacement onto the target's own `superseded_by`, at feature grain and at task grain alike. The two directions must agree. Every edge stays at one grain, every target has at most one replacement, and the complete lineage graph must remain acyclic. Approval refuses a disagreement between the two directions, a cross-grain edge, a second replacement for one target, or a cycle before it writes anything. Nothing else in Tiller writes either field, and hand-editing one is the same machine-owned-state violation as every other `.yml` edit. Two operations record such an edge: `supersede`, for done work whose result shipped, and `cancel` carrying `superseded_by`, for work replaced before it ever finished. Both leave the tree answering "what replaced this" from the retired node, which is the question a reader of a retired node has no other way to answer.

**A cancel's reason and amendment are event provenance, not node fields.** A cancelled node carries its `cancelled` status and, where the amendment named one, its `superseded_by`. The reason and deciding amendment remain on the cancellation event and the retained amendment operations. Node pages derive forward and reverse amendment relationships from those records; they store no manual backlinks or separate applied-results ledger.

**Identity evidence survives key reuse.** Keyed-add application events join by amendment ID, add noun, and original key to their immutable first target ID. Cancellation and supersession events record the replacement ID in `after.superseded_by`; cancellation need not include that replacement in `targets`. Newly recorded application events also carry `after.operation_index`, the zero-based index used by the existing staging event's `after.index`. Staging a keyed addition records its original `after.key` and, when already resolvable, its existing parent's immutable `after.parent` ID. Same-batch parents remain staged references until materialization. Feature-add application events record the resolved parent epic in `after.epic`, matching task-add events' existing `after.feature`. Question application events carry `after.outcome` (`created` or `updated`): the existing `add.question` event kind also describes folds, so that kind alone does not prove creation. A genuine no-op records no application event. These are additive event metadata; amendment YAML, lifecycle states, and historical events are not migrated.

**Relationship projection is conservative about history.** One identity projection supplies amendment detail links and node backlinks. It preserves proposed, approved, and withdrawn amendment IDs, with proposed-first ordering followed by canonical key and immutable ID. Effects distinguish direct targets, replacements, created nodes, addition parents, and question-block intent; current descendant rollups are separate from a proven historical addition-parent association. An unchanged sibling is not directly amended, and several affected descendants still count as one amendment. Effect operation numbers are one-based for display; a legacy application event without an exact retained-operation join leaves the number absent. Dates come only from actual opening, approval, or withdrawal events.

Legacy parent resolution uses only bounded add, key-rename, and remove identity facts. It never treats today's holder of an old key, a question statement, a filesystem timestamp, or random event-ID ordering as historical identity evidence. Missing or contradictory evidence, unrecorded key edits, and indistinguishable same-timestamp ownership remain unresolved. A deleted known node retains its immutable ID as non-clickable historical text; an unmaterialized addition has no invented live ID. Question-block relationships retain the operation's intent without claiming a folded/no-op operation changed the question. Event-read failures and incomplete identity evidence leave known direct links usable and add a narrow amendment-history warning; clients must not present incomplete relationships as a definitive zero count. Application evidence remains discoverable even when an interrupted application leaves its amendment proposed.

**States.** `proposed` (the agent has drafted the reshape) → `approved` (the human approved it, and approving applied it). `withdrawn` is the terminal escape for a reshape abandoned before approval. There is no separate applied state, because approve and apply are the same human act. Both terminal states release the key: a duplicate key is refused only among `proposed` amendments, so one key can be carried by several amendments over a project's life, and how an address resolves against that history is in [cli-surface.md](./cli-surface.md).

#### Deferred amendment input validation

This boundary is partially unimplemented and must be rebased against the current nine operation kinds before execution. It is retained scope, not a fresh defect inventory or a claim that lifecycle, lineage, and transaction guards provide strict decoding. Malformed amendment files and invalid construction must refuse before any repository write, whether supplied through the CLI, direct core calls, hand edits, merge resolutions, or a file changed after initial tree loading.

Reuse the same key and feature change-type vocabulary across direct adds, staging, decoding, and approval. Reject path-special key segments and unknown change types at their owning boundaries. One amendment decoder must serve both initial loading and fresh mutation reads: reject unknown YAML fields and extra YAML documents; preserve ID-shape, noun-prefix, and collision checks; and enforce single-segment amendment keys, filename-to-identity coherence, recognized statuses, and nonempty `git:<revision>` bases. Strict unknown-field rejection is scoped to amendment YAML, not every state reader.

Establish explicit required, optional, and forbidden fields for all nine operations, including `add-epic` and `add-question`, with deterministic refusals rather than a reflection-derived field matrix. Preserve question title, statement, block-target checks, typed move preconditions, per-operation sources, intake-source requirements, and legal metadata-only amendments. Check store-side requirements for add-task titles, cancellation reasons, supersession replacements, and feature change types. Precondition names and values must be known, nonempty, and compatible with their operation. Staged `order` and `link` remain separate deferred scope.

The [CLI amendment contract](./cli-surface.md#large-changes-amendments) owns explicit flag-presence rules and refusal output. Before calling this boundary complete, reconcile the specs, command inventory, and embedded guidance with the implemented validator. Reassess compatibility against today's persisted fields before deciding whether a schema bump is needed.

Verification must exercise store, mutation, and CLI boundaries in a disposable initialized Git project. Snapshot file inventory and content hashes after preparing each invalid input and before invoking Tiller; refusals must preserve amendment status, operations, events, and would-be nodes. Cover malformed keys and change types, forbidden or missing operation fields, unknown YAML fields, multiple documents, invalid identities and bases, mismatched filenames, and post-load edits. A handwritten add-task without a title must refuse without relying on the CLI constructor. Cover required/forbidden fields and precondition compatibility for the whole operation set, then prove a valid amendment still stages, displays, and approves through the real CLI. Run the affected regressions and malformed-file smoke; these checks have not been performed by retiring the old plan.

---

## References and IDs

Every node carries a stable `id`; whether it also carries a human `key` depends on its class. A node with a file of its own (`epic`, `feature`, `task`, `amendment`, `adr`) carries both `id` and `key`, because a name on disk needs to be human-readable; for the first three that file sits in a folder of its own, while an amendment and an ADR are a named file pair and a named file. The one ledger item, `question`, carries `id` only, because it lives as an entry in a list rather than as a named file. The id follows the type-prefixed rule in [IDs](#ids), as in `E-j8m4q2`, `F-79dh3q`, `T-k3p9w6`, `Q-h2n7m5`, `A-d6w3f9`, and `ADR-v7b2d5`, and it never changes. The `key`, where present, is the path-derived label and can change. References across the dependency graph and the evals use the `id`, so renaming a feature with `tiller rename` updates the key and leaves every reference intact. This costs a little human readability in the raw files and buys a graph that doesn't silently break on a rename.

---

## Lifecycles, consolidated

| Node | States | Agent may drive to | Human-only transition |
|---|---|---|---|
| Epic | draft → ready → done → archived, plus draft/ready → cancelled; `done` is a `sync` rollup observation and recomputes when new work lands beneath it; `active` is a derived display status (from children), not stored | draft tree proposal, derived active-work tracking | approve to `ready`; cancel before done work exists; archive |
| Feature | draft → specced → ready → active → done, plus ready → specced on return; `done` is a `sync` rollup observation over its tasks and recomputes; an `active` feature whose non-cancelled tasks are all done carries a derived display phase (`awaiting check`, then `awaiting sync`), computed from its tasks and gates, not stored | specced (the stage-2 output, tasks included), active, checked | approve to `ready` (the feature plan, initial tasks included); return to `specced` before work starts; cancel |
| Task | proposed → ready → in_progress → check → in_review → done, plus ready → proposed on return and in_review → in_progress on resume | proposed, in_progress, check, in_review, resume to `in_progress` on a requested-changes review | approve to `ready`; return before work starts; cancel; merge-backed `done` observed by `sync` |
| Bug, chore | the task's states verbatim; done by the node's own observed merge, or carried by its resolved_by carrier's done | proposed, in_progress, check, in_review, resume | approve (triage) to ready; return; cancel with a resolution; resolve (agent only on instruction) |
| Amendment | proposed → approved, or withdrawn | proposed (drafts the reshape with preconditions) | approve, which validates and applies the batch |

The invariant across them all: the agent can take work right up to the edge of `done`, attaching evidence and turning gates green, but only a human merge crosses it, and every `done` above a task is arithmetic over merges the human performed, recorded by `sync`. The amendment follows the same shape one level up, where the agent stages a reshape and only the human's approve validates and applies it.

---

## Provenance

Generated nodes carry `source` references back to the input docs they came from. It lets you trace any epic or feature to its origin, and it gives drift control a thread to pull: when a source doc changes, the nodes it generated are candidates for review. `design/` is the genesis shelf this feeds from; `feedback/` (below) is the ongoing one.

An amendment carries the same idea at reshape grain, and in both directions: `triggered_by` for the docs whose change prompted it, `amends_design` for the docs its approval obliges (see Amendments). `tiller intake` adds a third, more literal thread at the same grain: the amendment's own `source` names the one fingerprinted feedback document the whole batch traces to, landed under `.tiller/feedback/` (a sibling of `design/`, since genesis inputs and ongoing inputs are different shelves), and every operation staged onto it carries its own `source` citation into that document, so a human reviewing the batch reads each op beside the line that justified it rather than trusting the mapping on faith.

`.tiller/reviews/` holds code review files, a sibling of `feedback/`. Each is prose the `code-review` skill writes, read by no loader and carrying no state, so the directory needs no schema version. A shelf node logged from a review cites its finding's block as `source`; plan changes logged from a review travel through `tiller intake`, whose landed distillation in `feedback/` is the amendment's fingerprinted source. Review files are committed by the disposition that acts on them, and an uncommitted one appears only in the informational uncommitted-state line of `tiller status`.

A source citation is stored exactly as authored and has one of two forms: an `http://` or `https://` URL, or a repo-relative path ending in lowercase `.md`, optionally followed by `#L<n>` for one positive 1-based line or `#L<n>-<m>` for an inclusive line range, and several such locators of the same document may follow one another after commas (`#L3-L58, #L321`). A URL opens externally. The local arm uses the shared source-citation reader: `design/...` is the human-facing alias for `.tiller/design/...` on disk, `.tiller/...` resolves canonically beneath the repository's `.tiller/` directory, and every other accepted path, such as `docs/...`, resolves beneath the repository root. Resolution does not rewrite provenance. The stored citation remains verbatim, and the `/file/...` address retains the human-authored path spelling while a range opens at its first line.

---

## Drift control hooks

The schema carries the markers `tiller drift` reads, so active drift assistance has something to work with. A candidate exists only where a fix exists, and a clean answer asserts coverage: a node or doc the sweep could not compare is reported `uncovered` with its reason, never folded into silence. Work-node surface drift keeps the vanish-only rule: modified code rides as context, because later edits do not falsify a record of completed work. Baseline drift instead reviews an as-is description's present-tense claim. Working-tree comparisons catch edits before commit; recorded project commit anchors bound history only where ancestry is available.

The seven markers:

- **active_basis**: work is proceeding against a spec, eval, or implementation spec the human has since changed. `accepted_against` blobs compared against the current working-tree `feature.md`, `feature.eval.md`, and `task.md`. Sweeps ready and active features and approved non-terminal tasks; the status filter lives in the sweep, because `accepted_against` is never cleared at done and an unfiltered sweep would misfile `stale_spec`'s business under this marker. The one marker whose condition also refuses verbs (`start`, `check`, `submit`), cleared by `approve --refresh-basis`.
- **stale_spec**: a node is `done` but the spec, eval, or code surface no longer matches what was verified. `verified_against` compared against the working tree: the spec and eval legs by content, the surface leg by the vanish rule, a candidate only when a file in a recorded `inventory` no longer exists, with the tree-hash difference reported beside it as context. Sweeps done nodes; a superseded feature and an archived epic are exempt, the one drifted by construction and the other deliberately closed. The recorded spec and eval legs are the integration commit's committed blobs (sync reads them there), and the comparison hashes the working-tree file the way `git add` would store it: the path's checkin conversion (`text=auto`, `eol=crlf`, or `core.autocrlf` normalization) and any clean filter such as Git LFS are applied, and no Git object, index entry, or ref is written (an LFS clean filter may still write to its own `.git/lfs` cache). An unedited file checked out with CRLF line endings, or an LFS file, therefore compares equal to its committed blob, and only a content change reports `stale_spec`. The eval leg is re-pinned by an approved `correct-eval` amendment operation (see Amendments), which is how a done feature's bindings are corrected without a byte-identical revert. Nothing re-pins the spec leg or the surface leg: those clear on a revert, or when the node reopens and crosses to done again under new work.
- **answered_question**: the plan was made under an assumption whose answer has arrived and has not been reviewed into the node. `assumption_basis` entries compared against answered questions and their answer revisions. Sweeps non-terminal nodes; the done case is deliberately out, because its fix is an amendment, a human judgment. Cleared by `approve --reviewed-assumption`; refuses `submit` alone (see Assumption basis).
- **missing_surface**: a recorded surface path no longer exists in the working tree. Sweeps non-terminal features and epics carrying surfaces. The same vanish rule as `stale_spec`'s surface leg pointed at planned work instead of done work, named separately because the remediation differs: a spec corrected before the work is built, rather than a completed claim re-examined.
- **orphan_eval**: a criterion points at a test or artifact that no longer exists. Existence checks over path-shaped refs: each path-shaped item of a test binding's `ref`, checked one at a time so every missing file is named on its own, and a preserve criterion's `baseline.artifact`. Polarity-aware in time: `preserve` criteria are checked from `accepted_against` onward, since they name what exists before the work, while `introduce` criteria are checked only once `verified_against` exists, because a contract naming a test before the test is written is the workflow working, not drift. Command bindings are out of scope, since a shell string offers nothing to stat and running it is `check`'s job; each binding kind's coverage is stated in the report rather than implied.
- **stale_design**: a confirmed ledger doc describes code overlapped by a done feature whose `verified_against.commit` is after the doc's `confirmed.commit`. Prefix overlap is separator-aware in both directions and uses normalized recording coordinates, including declaration aliases. Equality of commit anchors alone is not a later crossing: at equal commits, the crossing's `observed_at` must be later than confirmation. A vanished `describes` path is also a candidate. Reports every matching feature's id, key, repository-relative `feature.md` path and crossing commit, plus the affected described paths. The range ends at the latest matching crossing only when ancestry establishes a single upper bound for all work; missing or diverged history is uncovered, never a fabricated range. Reconfirmation clears already-reviewed crossings.
- **shifted_design**: opt-in comparison of confirmed surface fingerprints against current working-tree code, including dirty bytes, additions, removals, directory-entry executable modes, and declared external reference trees. Any fingerprint difference or vanished described path is a candidate, whether or not tracked work caused it. `since` is the confirmation commit; `until` is current project HEAD only when history establishes the range. This history is context, not a promise that the confirmed bytes can be recovered from Git: confirmation may have observed dirty or external code that never existed in those commits. There are no invented feature references or external HEAD anchors.

Both baseline markers report linked-but-unconfirmed docs as `uncovered` with reason `unconfirmed`. Unsafe or unreadable described paths and unavailable or divergent ancestry remain uncovered, even alongside a real candidate found on another path. Neither marker reads `project.adopted`, updates the ledger, or edits prose. Their remedy is human review and correction of the named doc followed by shell-quoted `tiller approve design <doc>`. A missing path must be restored or its ledger coverage repaired before approval can succeed.

Shelf-node `accepted_against` drift sweeps are not built. The bug, chore, and tweak work commands still enforce their accepted contract through the active-basis guard. Automatic uncovered-root baseline routing is also unbuilt; baseline derivation and prose reconciliation remain agent and human work.

Baseline candidate and uncovered rows carry `noun: design`, an empty `id`, and the canonical design-relative doc path in `key`. Candidate `files` are affected described paths; optional `since` and `until` are structured RevisionRefs; optional `features` rows contain `id`, `key`, `spec`, and `commit`. These are design-path rows, not synthetic work nodes. `compared` remains the distinct work-node count; `compared_design` separately counts distinct docs fully compared by at least one enabled baseline marker. Per-marker baseline coverage uses docs, work-node markers use nodes, and `orphan_eval` uses criteria. A failed doc-marker comparison counts unresolvable, never checked; success under another marker may still count the doc once in `compared_design`.

The as-is markers deliberately differ from done-work surface drift's vanish-only rule: done work records a built intent, while an as-is doc claims the present. `stale_design` constrains change detection to tracked done crossings; `shifted_design` admits broader churn explicitly. Fresh scaffolds enable only the first. Existing check lists stay unchanged, with omitted markers visible as Disabled.

A node is exempt from a sweep only when a human recorded a decision that makes the drift expected (superseded, archived, cancelled), never because the tool failed to record a baseline, and every exemption is counted in the report's footer, so scope itself is visible rather than silently assumed. The command, its envelope, and its exit semantics are in [cli-surface.md](./cli-surface.md).

Detection is Tiller's; the proposed fix is the agent's (a doc, test, or eval diff drafted via the drift skill); approval is yours. Detect, propose, approve. When a single upstream change invalidates a whole cluster of nodes at once, the candidates are not mended one marker at a time but gathered into a single amendment (see Evolving a project), so a cascade is reviewed and applied as one reshape. The counterpart discovery, the code shifting under planned-but-unbuilt work in ways no mechanical marker can see, is `amend`'s own trigger: the developer notices, and the skill reads the bounded diff the `accepted_against` commit anchors make possible ([skill-set.md](./skill-set.md)).

---

## Event stream

The event stream is stored as one immutable YAML file per logical event under `events/`, partitioned by date: `events/YYYY/MM/DD/<event-id>.yml`. The event id uses the compact random token rule from [IDs](#ids), prefixed as `EV-`, and the file records `type`, timestamp, acting `contributor`, literal command, target node ids, opaque before and after maps, and affected files. Each file change carries its repo-relative path plus optional before and after RevisionRefs: a create may omit `before`, and a delete may omit `after`.
Events are not the source of truth for current state; the node files are. An event records the node status its own checkout saw when it was written, so an `attest` event written on a branch that still saw the feature as `active` stays `active` in the stream after a later sync moved the node file to `done`. That is the stream working as designed, not a conflict to repair; read current state from the node file. Events are the source of truth for recaps, node timelines, and stale review protection, because commit history can squash several state transitions into one commit and uncommitted working-tree changes have no commit yet. Event creation participates in the same journaled transaction as the source-state writes and deletions it describes. Sorting is by timestamp, then event id. Appending an event creates a new file rather than editing a shared log, so parallel branches merge event histories by adding files.

---

## Agent usage: `usage/`

`.tiller/usage/` holds an approximation of the model tokens the project's coding agents spent, credited to the work item and the stage of the workflow each model call served. It feeds the console Report's [Agent use tab](./ui-surface.md#report). Capture writes it, through `tiller usage` or best effort inside `tiller sync` (see [the CLI surface](./cli-surface.md#capturing-agent-usage-usage)), and only when the manifest's `usage.capture` is `true`. The bar is "close and calculated consistently": the same method applied the same way every time, not an exact bill.

There is one file per top-level harness session, with its subagents folded in:

```
.tiller/usage/<harness>/<session-id>.yml
```

```yaml
harness: claude-code          # claude-code | codex | pi (Pi and OhMyPi share one reader and format)
session: 165906e8-0e15-454a-a5b0-60b3affcce0d
reader: 1                     # reader version; a later version may recompute the file
captured_at: 2026-10-26T19:02:11Z
first_call: 2026-10-20T14:03:55Z
last_call: 2026-10-20T18:41:02Z
rows:
  - hour: 2026-10-20T14:00:00Z  # the UTC hour the calls fall in
    node: T-d8icjq               # omitted when the calls are not tied to a work item
    stage: implementation
    model: claude-opus-5-5       # as the harness reports it, provider prefix kept when it gives one
    calls: 41
    input: 1203                  # fresh input, excluding cache reads
    cache_write: 290114
    cache_read: 3920551
    output: 18220
    reasoning: 4410              # only when the harness reports it; part of output, never added to a total
```

A row is the calls of one model in one UTC hour credited to one node, or to none, and one stage. Rows are written sorted by hour, node, stage, and model, and every timestamp is UTC. The hour bucket lets the console place usage on its local-time week axis with at most an hour's error. Each reader maps its harness's fields into the four counts so they never overlap: Codex's `input_tokens` includes its cached tokens, so `input` is `input_tokens` minus `cached_input_tokens` and `cache_read` is the cached part. Each call is counted once. Codex sometimes writes a `token_count` again with its running `total_token_usage` unchanged, repeating the previous call's usage, and that repeat is skipped. Claude Code logs a response as several lines sharing its message id, and a subagent log can repeat a response another log of the same session already holds; a session counts each message and request id once, in the first log that holds it, the top-level log first. A row's total is `input + cache_write + cache_read + output`; `reasoning` is informational.

**No person, no content.** The harness id and the session id are the only identifiers. A usage file carries no contributor, owner, path, command, prompt, response, or tool output, only counts. Git history still shows who committed a usage file; that is Git, not a metric surface Tiller controls, and no Tiller read or payload attaches a usage figure to a person.

**Stages.** `stage` is one of nine keys, in display order: `spec_writing`, `planning`, `implementation`, `checks`, `acceptance_evals`, `code_review`, `publishing`, `closeout`, and `other`. An unknown value reads as `other`. The console's labels for them are page copy and live in the [console contract](./ui-surface.md#report).

**How calls are credited.** Each session is replayed in time order, tracking what it is working on now. The current node and stage change only on markers found in the session's tool calls:

| Marker | Sets the stage to | Sets the node to |
|---|---|---|
| `tiller start`, `claim`, `resume`, `transfer` | `implementation` | the node named |
| `tiller check` | `checks` | the node named |
| `tiller eval`, `attest` | `acceptance_evals` | the node named |
| `tiller submit` | `publishing` | the node named |
| `tiller spec`, `return` | `spec_writing` | the node named |
| `tiller add`, `link`, `order`, `amend`, `intake`, `withdraw`, `unlink`, `rename`, `remove`, `configure`, `resolve` | `planning` | the node named |
| `tiller approve`, `answer` | `other` | the node named |
| `tiller sync`, `archive`, `cancel` | `closeout` | the node named |
| loading the execute, specify, code-review, capture, amend, intake, decompose, orient, mend-drift, or derive-baseline skill, by `tiller guide <skill>` or the harness's own skill mechanism | `implementation` for execute, `spec_writing` for specify, `code_review` for code-review, `planning` for capture, amend, intake, and decompose, `other` for the rest | the node named, if any, else unchanged |
| a read-only command (`show`, `status`, `readout`, `next`, `queue`, `ls`, `tree`, `search`, `query`, `drift`, or `guide` without a skill) | unchanged, or `other` if none is set yet | the node named, only when none is set |

A mutating verb that names no node, such as a bare `tiller sync`, clears the node. Every argument after the verb that is not a flag, after a leading noun word, is a candidate for the node named, since a flag's value (`--pr 4`, `--contributor jc`) cannot be told from a node by its shape alone. Candidates are resolved against the tree at capture time, by id or key (a shelf key with or without its `bugs/`, `chores/`, or `tweaks/` prefix), and the first that resolves is the node named, stored as an id so a later rename cannot orphan it. When none resolves, the first shaped like a node id is kept as written, so an id from a since-changed ledger is not dropped; otherwise the marker names no node, so `tiller sync --contributor jc` still clears it. The call that issues a marker is credited to the state the marker sets. A call made before any marker is `other`. When no node is set, a Claude Code call on a `tiller/<key>` Git branch is credited to that key's node. A subagent starts from its parent's state as of the subagent's first call and then follows its own markers. A marker is `tiller <verb>` as a word in a shell command, at the start of the text or after whitespace, a quote, a backtick, or one of `;&|(`, so `.tiller/` and `./cmd/tiller` never match. Root flags such as `--json` may sit between `tiller` and the verb. Commands often arrive inside JSON or JS text, as Codex's exec tool sends them, so the escapes `\n`, `\t`, `\"`, and `\'` are read as the characters they stand for first (an escaped newline separates two commands), and each argument is cut at its first backslash and trimmed at both ends to the letters, digits, and `/_.-` an id or key uses, which drops a trailing quote, bracket, or comma. Each reader decides which of its harness's tool calls are shell commands and which are skill loads; the replay itself is shared and knows no harness.

This is accepted roughness: a stage is sticky until the next marker, so fixing a failing check after `tiller check` counts as `checks`, and chatter between items stays with the previous item. It is consistent, which is the bar.

**Never lose data.** Capture creates and grows usage files; it never deletes one. When a session's log is gone, or now holds fewer calls than the file records, as after Claude Code's 30-day transcript cleanup or a truncated log, the file is left exactly as it is. A rewrite that would change only `captured_at` is skipped, so recapturing an unchanged session writes nothing. Session ids are unique, so two engineers never write the same file and merges never conflict on one.

**What capture leaves out.** A session is captured only once it has settled: a session whose newest log file was modified in the last 30 minutes is not opened, and one whose last call is under 30 minutes old is not written. A running session is captured once it goes quiet, and the session running `sync` is never captured by its own sync. A session whose first call is earlier than the project's earliest event is ignored, since agent work from before the ledger began is not Tiller work; so is a session with no model calls. The earliest event is the first one with a timestamp. While the project has no timestamped event at all, capture writes nothing and records none of those sessions in the cursor, so they are judged once the first event exists rather than baked in as earlier history. A folder or log capture cannot read is skipped and counted, never fatal: it stays out of the cursor and is tried again on the next run, and a session whose subagent folder cannot be read is left out rather than captured without its subagents. Capture writes only when the checkout is on `git.integration_branch`, because it scans every worktree, and a session file written from a task branch would land on two branches with different contents. With no integration branch configured, it writes wherever it runs.

**Written without an event.** Usage files are written under the store's path confinement and the mutation lock, in one transaction whose snapshot guards refuse a usage file changed since capture planned it and whose generation check refuses the whole write as `mutation_stale` when another Tiller write landed meanwhile. Capture records no event. Every event carries a required contributor, and target-less events surface in History and on Home with a person's name on them, which would put "this person captured these token counts" on a surface Tiller controls. Usage files are not lifecycle state, and each carries its own `captured_at`. So capture needs no joined identity.

**The cursor.** `.tiller/.cache/usage.yml` is capture's gitignored memory between runs. It records the reader version and, per absolute log path, the size and modification time last seen and whether that session has a usage file; a log whose size and modification time are unchanged is not opened again. It also records working directories that never change, so each is read once: each Codex rollout's, from the file's first line, and each Claude Code or Pi folder's whose name only begins like the root's encoding, from the first line in its logs that records one. A missing, unreadable, or corrupt cursor, or one from another reader version, reads as empty, so every log is examined afresh and each file is rewritten only when its result differs, subject to the never-lose rule. The cursor is saved only after the capture's writes commit, or when there was nothing to write, and never when capture is skipped or refused.

**Nothing else walks it.** `.tiller/usage/` is token counts, not prose or lifecycle state. Keyword search and its Markdown corpus skip it, the drift sweep, node inventories, and the event cache never read it, and an amendment move's read guards skip it, so a capture landing mid-move cannot make the move stale. Code identity already excludes all of `.tiller/`. A new walker over `.tiller/` states whether it includes `usage/`. Nothing in a managed project's build, CI, or merge eligibility reads usage files.

**Reading it.** A reader loads every `.tiller/usage/<harness>/*.yml`. A missing `usage/` directory is no usage. A file that does not parse fails the whole read, naming its path; usage files are machine-written, so a broken one is a defect to surface rather than to hide.

---


## Decisions settled so far

The decision record is in the [design history](../design-history.md), especially [one tree of epics, features, and tasks](../design-history.md#one-tree-of-epics-features-and-tasks), [change is forward work](../design-history.md#change-is-forward-work), and [human gates and review grain](../design-history.md#human-gates-and-review-grain). Current invariants live in the owning sections above.

For agent usage, settled 2026-10-26 (rationale in [agent usage, aggregate and never personal](../design-history.md#agent-usage-aggregate-and-never-personal)):

- Usage is summed across the team through committed files, and no usage file, payload, or chart attaches a metric to a person. (JC)
- Raw token counts with cache reads kept apart from fresh input, cache writes, and output; no cost weighting and no dollars. (JC)
- One committed file per top-level session, subagents folded in, created and grown but never shrunk or deleted.
- Calls are credited by replaying the Tiller commands each session ran, with subagents inheriting their parent's state, a method a spike over CALCTP's real logs validated against Tiller's own event stream.
- Nine stages, with display labels kept out of the file format.
- Capture records no event and needs no identity, skips sessions active in the last 30 minutes, writes only on the integration branch, and ignores sessions from before the ledger began.
- Capture is off by default and turned on per project in `tiller.yml`. (JC)

---

## What's next

This is the artifact layer. The [CLI command surface](./cli-surface.md), [skill set](./skill-set.md), and [console behavior](./ui-surface.md) define the operations and guidance over it. The [roadmap](../roadmap.md) tracks unfinished scope; the [design history](../design-history.md) preserves the rationale.
