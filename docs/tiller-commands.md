# Tiller commands

Every command Tiller v0.21.2 exposes (schema 12, orientation generation 57, eleven embedded
skills), with stubs where an id, key, or text goes. When this page and
`tiller <command> --help` disagree, the binary wins.

## Command index

| Command | What it does |
|---|---|
| [`tiller add`](#decompose) | Register a contributor or create a work node. |
| [`tiller amend`](#amend-a-reshape-of-the-plan) | Stage one operation onto a proposed amendment. |
| [`tiller answer`](#acceptance-and-gates) | Record the answer to an open question. |
| [`tiller approve`](#specify) | Approve a proposed plan across its start gate, an amendment, or a design doc. |
| [`tiller archive`](#the-done-crossing-and-rollups) | Move a done epic to archived. |
| [`tiller attest`](#acceptance-and-gates) | Record a human attestation for a gate or a criterion. |
| [`tiller cancel`](#the-done-crossing-and-rollups) | Terminate a node or an open question. |
| [`tiller check`](#execute-the-task-loop) | Run and record a node's gates, or check project readiness. |
| [`tiller claim`](#ownership) | Reserve a task, bug, chore, or tweak before it starts. |
| [`tiller completion`](#help-and-shell-completion) | Generate the shell completion script. |
| [`tiller configure`](#configure) | Record the review grain at the project, epic, or feature home. |
| [`tiller drift`](#reads) | Sweep the enabled drift checks and report what moved. |
| [`tiller eval`](#acceptance-and-gates) | Run a node's acceptance criteria and hand off the rubric evals. |
| [`tiller guide`](#orient-and-guidance) | Print a skill, or list your next moves and name the skill that fits. |
| [`tiller help`](#help-and-shell-completion) | Help about any command. |
| [`tiller init`](#onboard) | Scaffold a new `.tiller/` workspace. |
| [`tiller intake`](#amend-a-reshape-of-the-plan) | Land an external feedback document and open its amendment. |
| [`tiller join`](#onboard) | Select your identity and wire your coding agent to the project. |
| [`tiller link`](#decompose) | Record a dependency, surface, attachment, design baseline link, or question hold. |
| [`tiller ls`](#reads) | List nodes of a noun, optionally filtered. |
| [`tiller migrate`](#migrate) | Migrate this checkout and refresh its generated instructions. |
| [`tiller next`](#orient-and-guidance) | List your likely next moves, ranked. |
| [`tiller order`](#decompose) | Record the soft order among siblings. |
| [`tiller query`](#relationship-discovery) | Traverse recorded work, document, and path relationships. |
| [`tiller queue`](#reads) | List every current action-queue entry. |
| [`tiller readout`](#orient-and-guidance) | Print the readout block for a seam. |
| [`tiller reassign`](#ownership) | Hand an unstarted claim to another registered contributor. |
| [`tiller release`](#ownership) | Return an unstarted claim to the pool. |
| [`tiller remove`](#correct-a-proposal-or-accepted-plan) | Retire a contributor or remove an unapproved proposal node. |
| [`tiller rename`](#correct-a-proposal-or-accepted-plan) | Change a node's display title or proposal key. |
| [`tiller resolve`](#bugs-chores-and-tweaks) | Point a bug, chore, or tweak at the tree work whose merge fixes it. |
| [`tiller resume`](#execute-the-task-loop) | Re-enter work from review. |
| [`tiller return`](#specify) | Reverse a mistaken pre-work gate. |
| [`tiller search`](#reads) | Keyword search across `.tiller/`. |
| [`tiller serve`](#reads) | Serve the read-only local web console. |
| [`tiller show`](#reads) | Print a node in full. |
| [`tiller spec`](#specify) | Assert a feature's specify-stage output is written. |
| [`tiller start`](#execute-the-task-loop) | Start claimed work and permanently fix its owner. |
| [`tiller status`](#reads) | Show the project summary, or the typed oracle for one node. |
| [`tiller submit`](#execute-the-task-loop) | Record the review surface and move work to review. |
| [`tiller sync`](#the-done-crossing-and-rollups) | Observe merged review branches and record done crossings. |
| [`tiller transfer`](#ownership) | Replace started unfinished work with a ready assignment to another contributor. |
| [`tiller tree`](#reads) | Print the work tree. |
| [`tiller unlink`](#correct-a-proposal-or-accepted-plan) | Remove dependencies, surfaces, attachments, design baseline links, or question holds. |
| [`tiller upgrade`](#upgrade) | Update Tiller and maintain the current checkout. |
| [`tiller withdraw`](#amend-a-reshape-of-the-plan) | Drop a proposed amendment without applying it. |

## Conventions

- `<epic>` is an epic key or id (`form-engine` or `E-4smkjd`). `<feature>` is the canonical
  epic-qualified key or id (`form-engine/certification-form-loading` or `F-79dh3q`). `<task>` is the
  canonical three-segment key or id (`form-engine/certification-form-loading/transcribe-the-schema` or
  `T-k3p9w6`): a task is a folder under its feature, so it carries a key like every other grain.
  `<question>` is addressed by id only (`Q-2erhyz`): the decisions ledger carries no keys. Human scan references render that id plus the short title; question detail retains the full statement.
- `<bug>`, `<chore>`, and `<tweak>` are shelf nodes, addressed by id (`B-4k2p9x`, `CH-7m3vqz`, `TW-2p8k9m`), by the canonical
  key (`bugs/blank-on-reload`, `chores/update-readme`, `tweaks/button-text`), or by the bare local segment
  (`blank-on-reload`), since the noun already names the shelf. A shelf key is one local segment;
  the `bugs/`, `chores/`, or `tweaks/` prefix is implied wherever you write one.
- Work-node nouns are `epic`, `feature`, `task`, `question`, `amendment`, `bug`, `chore`, and `tweak`.
  `design` on link, unlink, and approve addresses `<doc>` relative to `.tiller/design/`, not a node
  id or lifecycle. There is no effort or sequencing noun. `<amendment>` is a key or minted id
  (`form-engine-rework` or `A-d6w3f9`). `bugs`, `chores`, and `tweaks` cannot be epic keys.
- Every command accepts `--json` (machine-readable envelope) as a global flag.
- Every project command finds its project by walking up to the nearest `.tiller/tiller.yml`. A
  `.tiller` that exists but cannot be inspected (no read or search permission) refuses with
  `project_marker_unreadable` (exit `3`) and writes nothing, rather than falling through to an
  outer project.
- Commands that attribute an acting contributor accept `--contributor <id>`, falling back to `TILLER_CONTRIBUTOR`,
  then the checkout's `.tiller/identity.yml`. It is omitted below. `init`, `join`, and `add contributor`
  write without an acting-contributor concept. Upgrade and migrate resolve an actor only for a real
  source migration, never artifact-only refresh or binary-only work. Ordinary attributed commands
  report a fallback as `acting as <id> (<source>)` and carry top-level JSON `actor: {id, source}`.
  Maintenance carries the actual migration actor in its structured `project.actor` result.
- **human** marks a gate the agent may run only on an explicit instruction naming the nodes.
  **human only** marks the strictest class: never the agent's to run, even on instruction.

## Onboard

```sh
tiller init --name "<project name>" --key <project-key> --kind greenfield|brownfield|legacy-migration \
  [--design <path/to/design-docs>]
tiller add contributor <contributor-id> --name "<display name>"          # human: registers the person on the roster
tiller join --as <contributor-id>                                        # selects that identity for this checkout
```

Kind describes code that predates Tiller adoption, not which later feature changes are legal.
Fresh brownfield and legacy-migration manifests declare `.` as `mode: surface` and record an
optional `project.adopted` observation of this repository's HEAD and UTC time. An unavailable HEAD
leaves the anchor absent; it is not a clean-worktree assertion or an external legacy revision.
Greenfield starts with `code: []` and no anchor. Reruns preserve the manifest's project identity,
kind, roots, and original or absent anchor; `join` and `migrate` do not backfill them.

Prepare docs before readiness. Reconcile existing docs against code; an import is not baseline
confirmation. Move in-repo docs into `.tiller/design/` when ownership moves to Tiller, or put a
pointer page there if another consumer needs the original location. External `--design` imports
are copies on the first scaffold only and never delete their sources. A valid `--design` rerun
succeeds and says the import was skipped; it does not import or change provenance.
Fresh imports refuse a root-level `baseline.yml` entry, case-insensitively, before writing any
scaffold. Import a prose-only directory, never another project's confirmation state.
A `--design` source that contains `.tiller/design/` (the checkout, an ancestor, or the same
directory through a symlink) or lies inside it refuses with `invalid_design` before any write.
An init rerun over a checkout whose orientation a newer binary generated refuses with
`orientation_newer` (exit `3`) and changes nothing; same- and older-generation reruns refresh it.

Register a contributor, then join. Every code root needs an explicit `surface` or `reference` mode.
Legacy-migration also needs an existing reference root declared in `.tiller/tiller.yml`, even
after a design import. Init success is not readiness: empty contributors and design inputs remain
`missing_contributors` and `missing_design`; missing legacy reference declarations produce
`no_reference_root`, and invalid or empty modes produce `invalid_code_root_mode`; correct an
invalid mode by hand. Readiness also reports `no_confirmed_baseline` for both existing-code kinds
until one ledger doc is confirmed, and `missing_design_doc` in every kind for each missing ledger
prose file. A ledger alone does not
satisfy non-empty design, and other gaps remain independent after a confirmation.

Init seeds a missing `.gitignore` but never changes an existing one. Join upserts all Tiller-owned
groups using root-anchored paths, including `/artifacts/tiller/` rather than all artifacts, and
leaves existing covering lines untouched. Build and test gate configuration is outside readiness:
existing-code projects can wire their current stack; greenfield wires it when the stack exists.

## Orient and guidance

```sh
tiller next                        # your ranked next moves, top five
tiller next --all                  # every legal move, ranked
tiller guide                       # the ranked list, then the skill for the top agent move
tiller guide <skill>               # orient | derive-baseline | decompose | specify | execute |
                                   # capture | mend-drift | amend | intake | design-brief |
                                   # code-review
tiller readout <seam>              # orient | derive-baseline | decompose | specify | execute | amend
tiller readout <seam> --since <sha> # the lighter correction block instead: what the event stream
                                   # recorded since that commit, the one the last block reported
tiller check readiness             # inputs and manifest ready? names the next move
```

`tiller next` prints the legal moves for this checkout, best first, top five by default. Each line
names who makes the move (You or Agent), the action and its node, why it ranks there, and the
command to run, or for an agent move without one the `tiller guide <skill>` to load. Your own task
running on another branch reads "Continue it on <branch>." instead of a command. Human decisions that
hold your current feature come first, along with any question overdue or due within two days, then
your own unfinished work, then the rest of the feature and epic you are in, then what you touched in
the last fourteen days by day, then everything else in plan order. Your current feature is the one
your latest claim, start, submit, check, spec, approval, answer, attest, link, reassign, or release
named. Adding work only counts as a recent touch on its parent (an added task on its feature, an
added feature on its epic), so a decomposition sets no current feature. A node appears once, with its best-ranked move. When more than five moves are legal, the last
line gives the count and `tiller next --all` lists them. `--contributor <id>` ranks for another
contributor. With no identity resolved the list is in plan order and the heading says it is not
personalized. When nothing is legal it prints "Caught up: nothing is legal to do right now." with any
hint on its own line. `--json` returns the ranked payload: `reader`, `candidates`, `caught_up`,
`unplanned`, `hint`, and `dependency_diagnostics` when present. It is read-only.

Bare `tiller guide` prints the same list, then names the skill for the highest-ranked agent move, which
is often not the top row: the top row is often yours. When only your moves are legal, such as
approvals or baseline docs awaiting confirmation, it suggests `orient`. The readout's `**Next:**` is
the same list, and `submit` and a passing `check` end with it as an `**Up next:**` block. `start`,
`claim`, and `resume` print nothing new. Tiller ranks by a fixed rule and never calls a model; a
ranked move is a suggestion, not authorization.

`tiller guide design-brief` and `tiller guide design-brief --json` deliver the full embedded
instructions through the existing named-guide surface. They do not take a target node or generate
the document themselves. Ask the agent explicitly, for example "Give me a UI/UX design brief for
feature F-9oymbc" or "Generate a prototype handoff for this task" when the task is unambiguous.
Epic, feature, and task keys and IDs are valid targets; bugs, chores, and tweaks are outside this export.

The agent uses existing reads to return one self-contained seven-part Markdown brief, not a
canonical specification or acceptance result. It defaults to current working-tree requirements,
labels draft or changed approval bases, required behavior, prototype suggestions, and unresolved
inputs, and preserves scope and acceptance coverage. Only an explicitly requested deliverable
file may be written; replacing an existing human-edited brief needs explicit authorization.
Generation does not refresh instructions, repair sources, claim or start work, run gates, record
state, upload the document, or build the prototype. There is no export command or `design-brief`
readout seam, and no terminal lifecycle readout accompanies the brief. Bare guide and ordinary
orientation retain their existing routing and human gates.

`tiller guide code-review` and `tiller guide code-review --json` deliver the code-review skill. They
take no target node. Ask the agent, for example "code review feature F-rnte2m" or "give me a code
review for epic E-2hjdhh". Any epic, feature, task, bug, chore, or tweak is a valid target. The agent
reviews the node's net change read-only, writes `.tiller/reviews/<date>-<node-id>.md`, and replies
with a table of numbered findings: bugs by severity (B), tweaks (TW), chores (CH), and plan changes
(P). Then say which to keep, for example "log bugs 1, 3, and 7, chore 2, and tweaks 1-3": bugs,
tweaks, and chores land `proposed` through `tiller add`, and plan changes land as one `proposed`
amendment through `tiller intake`. "fix B2 on its branch", "drop CH1", and "commit the review" are
the other verbs. Every disposition is one commit; approval stays with you. There is no review
command or `code-review` readout seam.

Brownfield and legacy-migration with no confirmed ledger doc list only the baseline moves, even when
work already exists: `derive-baseline` when no doc is registered, otherwise the human's confirmation
of each pending doc. Pending docs are reviewed or reconciled, not regenerated. One confirmation
releases this startup gate; ordinary moves then return.

`tiller next`, bare guide, and both readout forms share one ranked list. Within your own work, owned
checked tasks lead other owned ready or in-progress tasks, with dependency and soft order retained
within each group. Tree work stays ahead of shelf work; shelves prefer checked work before ready or
in-progress work, bugs before chores before tweaks within each group. Ownership, parent gates,
operative questions, and live-carrier exclusions still apply. A checked node returns to agent-owned
execute, not another start or new publication approval.

`tiller readout execute` uses `Execution status`. It includes project-wide tasks and uncarried shelf
nodes in `in_progress`, `check`, or `in_review`, with checked work displayed first, then in-progress,
then in-review work. `Gates green with evidence` and `Current submissions recorded` use that same
population, including entities beyond the display cap. A current submission needs `in_review` and
the appropriate PR or review ref; a retained PR on resumed or checked work does not count. A coarse
task ref names its resolved feature or epic review without claiming that review has a PR. Missing,
failed, stale, or unevidenced required gates do not verify a node. An empty population is not a
successful handoff. Next is the ranked list for this checkout's contributor.

`capture` writes and commits one complete proposed bug, chore, or tweak contract, with source, attachment,
and shelf dependencies where applicable, then stops before triage. It never approves, claims,
starts, resolves, or cancels the shelf node.

## Derive and confirm the baseline

```sh
tiller guide derive-baseline
tiller link design <doc> --describes <path> [--describes <path>]... [--from <source-path>]...
tiller link design <doc> --from <source-path> [--from <source-path>]...
tiller unlink design <doc> --describes <path> [--describes <path>]... [--from <source-path>]...
tiller unlink design <doc> --from <source-path> [--from <source-path>]...
tiller readout derive-baseline
tiller approve design <doc> [<doc>]...                      # human: named-doc confirmation
```

The doc is regular Markdown under `.tiller/design/`; paths cannot escape or traverse symlinks.
`baseline.yml` is reserved machine state. Repeat flags for literal paths, including paths with
commas. Design handlers refuse other nouns' flags even when explicitly empty or false; approval
has no `--all`.

Described code must exist and equal or lie beneath a valid declared code root. Reference and
explicitly declared external roots are allowed; merely containing a root is not membership.
Coordinates are normalized project-relative paths where possible, including declared `../legacy`
trees. Remaining symlinks and special files refuse. A real describes change clears confirmation;
repeat links are no-ops. Unlink can repair vanished paths. Removing the final describes path
removes the ledger entry, not the prose. Describes-only link and unlink need no Git.

`--from` pins the source's recoverable regular-file blob at this project's observed HEAD, not
untracked bytes or an external repository's blob. Commit exact uncommitted inputs first, then
record provenance before retiring an agreed in-repo source. From-only linking requires an
existing describes registration. Same source blob is a no-op; a changed HEAD blob replaces that
path's old ref. Provenance-only changes preserve confirmation; unlink removes by path without Git.

Approval observes current working-tree code and one project HEAD and timestamp for all named docs.
It is not `project.adopted`, an external HEAD, or a clean-tree assertion. Directory inventory
includes present tracked files and nonignored untracked files inside Git, or all regular files in
a declared non-Git external tree. Explicit files are observed even if ignored. `.git` and `.tiller`
metadata are excluded; changed bytes, additions, removals, and executable modes affect fingerprints.
The [artifact schema](spec/artifact-schema.md#baseline-ledger-designbaselineyml) defines the full policy.
Missing/unborn project Git refuses approval. Every target validates before one generation-checked
ledger-and-events transaction; there is no partial multi-doc approval. Observation does not lock
external editors or Git.
Unchanged owner, HEAD, and fingerprint facts can return unchanged without an event.

Read docs first. Derive undocumented code or translate at module grain, preserving meaning. Ask
move versus pointer once, never leave duplicate live specs silently, and never retire external
`--design` sources. Reconcile current code and state any missing adoption history. Commit only
the reviewed translation, ledger, and related question/event paths on the explicit integration
branch, preserving unrelated work. Do not use broad `git add -A`.

The baseline readout shows all recorded source path plus blob mappings and every doc's described
paths and confirmation facts, without a three-doc cap. Counts describe links and confirmations,
not proof every source section survived. `Open questions` is the current count. Separate run
notes name dropped, merged, or unplaceable material and questions actually opened; no ledger
dropped count exists. Reproduce the readout verbatim and stop at its one HUMAN review action.
Only explicit human instruction naming docs permits agent-recorded approval. Remaining pending
docs do not globally block work once one doc is confirmed. The console has no baseline write
surface; baseline drift runs through `tiller drift` (see Reads).

## Decompose

Lay the whole tree down in draft, sweep the cross-epic edges in with `link`, commit, and stop.
Tiller writes the state files and never commits them.

```sh
tiller add epic <epic-key> [--title "<title>"] [--source <design/doc.md>] [--depends-on <epic|feature>]...
tiller add feature <epic-key>/<feature-key> [--title "<title>"] [--source <design/doc.md>] \
  [--change-type create|modify|replace|migrate] [--depends-on <epic|feature>]...
tiller link <epic|feature> <node> --depends-on <epic|feature> [--depends-on <epic|feature>]...
# --surface unions existing project-relative files or directories into surface.paths.
# Either flag runs alone or both together in one source transaction:
tiller link <epic|feature> <node> --surface <path> [--surface <path>]...
# order records a soft order: intent, not constraint. It arranges rendering and steers
# recommendations wherever depends_on leaves the choice open; it never gates, and a rerun
# replaces the recorded list outright. Both: an agent records one only on explicit instruction.
tiller order epic <e1> <e2> ...                              # epic sequence, at the root
tiller order epic --clear                                    # erase the root list
tiller order feature <f1> <f2> ...                           # feature sequence; shared parent epic inferred
tiller order feature --clear --epic <epic>                   # erase one epic's feature list
# order bug, order chore, and order tweak rank a shelf instead: see Bugs, chores, and tweaks below
tiller add question "<statement>" --title "<short title>" [--blocks <node>]... \
  [--source <design/doc.md>] [--due YYYY-MM-DD | --due-hint "<free text>"]
tiller approve epic <epic> [<epic>]...                       # human: draft -> ready
```

Surface paths must exist and have no symlink component. They cannot equal, lie beneath, or contain
a reference root, including a root declared through an alias or absolute path. A parent directory
such as `app` refuses if `app/legacy` is reference; an overlapping surface root cannot override it.
Disjoint existing paths outside declared code roots, including untracked paths, remain legal.
`modify` may omit a surface; `replace` and `migrate` require one at `spec feature`, in every kind.

Missing requested paths refuse with `surface_missing` (exit 2); reference overlap uses
`surface_reference` (exit 2); unsafe paths, including symlinks, use `surface_path` (exit 2).
Invalid declared modes use `invalid_code_root_mode` (exit 3), even without running readiness first.
Unexpected inspection or reference-resolution errors use `surface_unavailable` (exit 3), naming
the path and cause rather than pretending it is missing. Correct the named manifest entry or
filesystem problem, or choose an existing writable path. Re-requesting a recorded path revalidates
it; unrelated old entries stay untouched, and later deletion remains `missing_surface` drift.

A combined `--depends-on`/`--surface` request validates both arms before one source commit.
Neither field nor either event changes on validation refusal or a rolled-back source write.
Changed dependencies emit `link.<grain>` and changed surfaces emit `link.surface.<grain>`;
unchanged axes emit nothing. A no-op dependency does not trigger the pre-start dependency guard.
The same single commit covers `--depends-on` with `--attach` on a bug, chore, or tweak, and
`unlink --depends-on` with `--detach`: every requested target resolves and the dependency lock
applies before anything is written, so an unknown attachment target or a locked dependency change
leaves both fields and all events untouched. A changed attachment emits `link.attach.<noun>` or
`unlink.attach.<noun>`. `unlink --json` always reports the node's surviving `depends_on` and the
`removed` edges, empty when the call removed none.

```sh
# --surface removes recorded paths literally, even ones the work moved or deleted:
tiller unlink <epic|feature> <node> --surface <path> [--surface <path>]...
```

`unlink --surface` is the only way to drop a surface path. It never checks existence, and a path
not recorded is a no-op. A done feature or epic, or an archived epic, refuses with
`surface_locked` (exit 2), since the completion record pins its surface. A specified `replace` or
`migrate` feature refuses to drop its last path with `surface_required` (exit 2): link the code's
current location first. It combines with `--depends-on` in one source commit, emits
`unlink.surface.<grain>`, and adds `surface` and `surface_removed` to `--json`. When a feature's
own work moves its surface, sync reports the node under "could not be recorded" and names the
unlink to run; after the link and unlink, the next sync records done.

## Specify

One stage and one gate. Write `feature.md`, `feature.eval.md`, and each task's `task.md` by hand,
add the task folders, assert the lot with `spec feature`, and the human's `approve feature` is the
single plan gate over the feature and its initial tasks together.

```sh
tiller add task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>" \
  [--depends-on <sibling task>]...                           # lands proposed, in its own folder
tiller link task <task> --depends-on <sibling task> [--depends-on <sibling task>]...
tiller order task <t1> <t2> ...                              # task sequence; shared parent feature inferred
tiller order task --clear --feature <feature>                # erase one feature's task list
tiller spec feature <feature>                                # draft -> specced; validates the
                                                             # specification, the contract, and the
                                                             # task explosion together; a replace or
                                                             # migrate feature with no recorded surface
                                                             # is refused (surface_required): tiller link
                                                             # feature <feature> --surface <path> is the fix;
                                                             # warns, without blocking, when a preserve
                                                             # criterion only repeats the tests gate
tiller approve feature <feature> [<feature>]...              # human: specced -> ready, and every
                                                             # proposed task on it to ready in the
                                                             # same act, each basis pinned; refused
                                                             # (open_question_blocker) while an open
                                                             # question names the feature, its epic,
                                                             # or a task not cancelled: answer it and
                                                             # fold the answer in; no --assume-question
                                                             # escape at a plan gate (see Execute below)
tiller approve <feature|task> <node> --refresh-basis         # human: re-record the accepted
                                                             # revisions without changing state; a
                                                             # changed feature contract expires the
                                                             # evals verdicts of unfinished tasks
tiller approve feature <feature> --reviewed-assumption <Q-id> # human: stamp an answered
                                                             # question's revision into the feature's
                                                             # assumption_basis; the fix for submit's
                                                             # answered_question refusal
tiller return feature <feature> --reason "<why>"             # human: ready -> specced, the mistaken-gate undo
```

A task added after the plan gate has its own approval:

```sh
tiller approve task <task> [<task>]...                       # human: proposed -> ready; refused
                                                             # (open_question_blocker) while an open
                                                             # question names the task or an ancestor
tiller approve task --feature <feature>                      # every proposed task on the feature,
                                                             # reported per task: held ones blocked,
                                                             # the rest approved, exit 1 if any blocked
tiller return task <task> --reason "<why>"                   # human: ready -> proposed
```

A held approve names its way out. The plan-gate refusal reads `answer them with tiller answer question
<Q-id> --answer "...", or cancel one that no longer needs a decision with tiller cancel question <Q-id>
--reason "...", fold the answers into the plan, and retry`; other transition refusals add cancel beside
`--assume-question`; a coarse submit refusal names cancel beside answer. A cancelled question's blocks and
any `assumption_basis` entry naming it stay on record but hold nothing and raise no drift. Re-adding a
cancelled question's exact statement mints a new open question and notes `re-asks cancelled <Q-id>`.

The queue hides an approve an open question holds: `approve_feature` is hidden while an open
question holds the plan, and `approve_tasks` only when every proposed task on the feature is held,
so the row stays while any proposed task is unheld and its batch command reports the held ones
blocked. The question's own `answer_question` row stays, so routing names the answer.

## Correct a proposal or accepted plan

`rename` has two axes. `--title` changes display metadata at any state and never edits Markdown.
`--key` moves proposal-safe folders while preserving ids: a draft epic with only draft, task-free
features; a draft, task-free feature, including reparenting between draft epics; a proposed task
that stays under its feature; or a proposed bug, chore, or tweak, which takes one local key segment with
the shelf prefix implied. `unlink` removes hard edges before affected work starts, and `--detach`
clears a bug's, chore's, or tweak's recorded attachment. `remove` permanently erases only unapproved proposal
material and is not cancellation.

Before structural placement discussion, inspect installed `rename --help` and `amend --help`,
affected descendants' execution/review history, both parents, dependencies, effective question
holds, and inherited review grain. For an unchanged eligible feature outside the direct rename
window, use the reviewed `amend --move-feature <ref> --to <epic>/<key> --reason "<why>"`
route, not cancellation and recreation. Draft, specced, and ready features with never-started
proposed/ready tasks can qualify; started descendants refuse. Stage, inspect, and stop for human
amendment approval. Ordinary feature/epic approval remains separate.

```sh
tiller rename <epic|feature|task|bug|chore|tweak> <node> --title "<display title>"
tiller rename <epic|feature|task|bug|chore|tweak> <node> --key <canonical-key>
tiller unlink <epic|feature|task|bug|chore|tweak> <node> --depends-on <target> [--depends-on <target>]...
tiller unlink <bug|chore|tweak> <node> --detach                    # clear attached_to; legal at any state,
                                                                   # since attachment is provenance
tiller remove task <task> --reason "<why this proposed task is removed>"
tiller remove feature <feature> --reason "<why this task-free draft feature is removed>"
tiller remove epic <epic> --reason "<why this task-free draft subtree is removed>"
tiller remove <bug|chore|tweak> <node> --reason "<why this proposed shelf node is removed>"
```

Both `link --depends-on` and `unlink` return `dependency_change_locked` once a real edge change
would alter started work. The one open arm is a started task adding (never removing) an edge to a done task. The locked
answer names that edge rule in `detail` and never an illegal `return` command or an amendment
remedy: amendment cannot link, unlink, or order, and `revise` re-pins prose without unlocking work. Reviewed `move-feature` supports
eligible unstarted subtrees only; relocation of started work remains unsupported. An absent-edge
unlink remains an exit-0 no-op. A key move outside its proposal window is `key_rename_locked`, exit 2.
Unsafe removal is blocked, exit 1, with
`remove_state_locked`, `proposed_amendment_reference`, `remove_lineage_reference`, or
`non_regular_remove_entry`, plus a copyable amendment remedy and, for a ready task, a return remedy.
`remove` also scrubs
across the shelves: a removed node's id leaves every surviving bug's and chore's `depends_on`, and
an `attached_to`, `resolved_by`, or `duplicate_of` that named it clears, with each affected node
reported as detached.

| When feedback lands | Correction | Example and next gate |
|---|---|---|
| Before epic approval | Cosmetic title | `tiller rename feature foundation/auth --title "Authentication and sessions"`; return to `approve epic`. |
| Before epic approval | Proposal key or draft feature parent | `tiller rename feature foundation/auth --key platform/auth`; source and destination epics must both be draft. |
| Before feature approval | Mistaken hard edge or unwanted task | `tiller unlink task foundation/app/wire --depends-on foundation/app/scaffold`, or `tiller remove task foundation/app/obsolete --reason "duplicate proposal"`; return to `approve feature`. |
| After approval, any state | Cosmetic title | `tiller rename task foundation/app/wire --title "Wire the application build"`; no approval or amendment. |
| After epic approval | Epic narrative wording | Edit `epic.md` and commit it; Git is the history because epic prose has no accepted basis. |
| After feature approval | One more task | `tiller add task foundation/app/document-setup --title "Document local setup"`, write `task.md`, then stop for `tiller approve task foundation/app/document-setup`. |
| After approval | Preferred sequence | `tiller order task foundation/app/wire foundation/app/document-setup`; soft order is direct and never gates. |
| Accepted non-terminal work | Same-scope feature or task prose | Edit the Markdown, inspect `active_basis`, then stop for `tiller approve <feature\|task> <node> --refresh-basis`. A wrong fact found while executing a task is instead a departure recorded in its commit and review, with `task.md` unedited. |
| Ready, unstarted plan | Approval should replay | `tiller return <feature\|task> <node> --reason "<why>"`, revise it, then stop for ordinary reapproval. |
| Eligible unstarted feature outside the direct rename window | Move unchanged work, optionally changing its local key | `tiller amend <amendment> --move-feature foundation/auth --to platform/auth --reason "Keep authentication in the platform increment"`; inspect `tiller show amendment <amendment>`, then stop for human amendment approval. |
| Before affected work starts | Required sequence | Use `tiller link ... --depends-on ...` or `tiller unlink ... --depends-on ...` directly. |
| After work or its branch starts | Genuine replacement or cancellation | Stage supported operations with `tiller amend`; stop for `tiller approve amendment`. Fresh eligible replacement work may carry its own prerequisites. |
| Started work | In-place dependency edit or active reparenting | Unsupported. Do not manufacture replacement solely to evade the edge or parent lock. |
| Done or cancelled feature/task | Historical prose correction | Preserve accepted prose; create forward additive work, or genuine replacement through amendment and supersession as appropriate. |

Place additions by bounded outcome, not subsystem name or completion status. Same-outcome work
may continue a done epic or feature; a distinct outcome or delivery phase warrants a new increment.
New children keep ordinary specification and approval gates. Adding a child does not immediately
rewrite parent status; normal sync reopens the rollup and clears current parent completion/review
fields and stales feature gates as appropriate, preserving completed descendants. Cancelled or
archived ancestors are not executable continuation targets. A fresh task under an active feature
can still receive legal sibling prerequisites.

An amendment may stage an epic before same-batch features. A new feature's tasks follow its
materialization through ordinary specification. Approval appends new epics to root soft order;
intended placement requires a separate explicit `tiller order`, and hard links after approval
still require an eligible dependent. Proposing a plan neither approves it nor authorizes broad
replanning or arbitrary reordering of accepted work.

## Ownership

Tasks, bugs, chores, and tweaks are individually claimable. New tasks are unclaimed regardless of who
created or approved them. Features and epics have Contributors, not an owner or claim. Claims
never cascade. Before first start a reservation can be released or reassigned; a successful
start permanently fixes ownership. Resume, checking, review, cancellation, and completion never
reset that boundary. All terminal ownership is immutable, even cancellation before start.

```sh
tiller claim <task|bug|chore|tweak> <node> [--from <holder-id>]
tiller release <task|bug|chore|tweak> <node>
tiller reassign <task|bug|chore|tweak> <node> --to <contributor-id> --reason "<why>"
tiller transfer <task|bug|chore|tweak> <node> --to <contributor-id> --reason "<why>"
tiller remove contributor <contributor-id>
```

Claiming an unclaimed unit succeeds; claiming your existing claim is a no-op. Taking another
pre-start reservation needs its actual holder in `--from` and an explicit human handoff, never
an opportunistic `--contributor` change. Release drops only your own pre-start reservation. Reassign
is a human-directed act. Contributor retirement refuses self-removal and unfinished assignments or
open questions, not historical contributor membership.

Start, resume, and task/shelf submission require the unit's own owner. `approve`, `attest`,
`answer`, `eval`, and `check` retain their actor and human-gate rules without an ownership gate.
Any registered developer may perform authorized aggregate assembly/checks/submission, including
epic reviews with several task owners. Coordinate one aggregate updater at a time through Git
and the team; there is no persistent coordinator claim.

Transfer is only for started unfinished work and requires an explicit human instruction naming
the work and recipient. It refuses unchanged owner, terminal work, scope drift, and an unfinished
started dependent. The atomic transaction cancels the original while preserving attribution,
prose, branch, review, and evidence, and creates a same-noun ready replacement with a distinct
ID/key, recipient, fresh accepted-basis path, and reciprocal lineage. The replacement is not
started and carries no branch, review, green checks, attestations, evidence, or completion basis.
Prerequisites/live order survive, unstarted dependents redirect, terminal edges remain historical,
and questions, pauses, attachments, and carriers remain operative. No assumption is granted.

Use the returned replacement reference in `tiller start <noun> <replacement-ref>`. At task/shelf
PR grain close the old PR without merging and open a replacement PR. At feature/epic grain retain
the unmerged aggregate PR, assemble replacement work, and rerun affected checks/acceptance.
Existing commits and host approvals do not vanish. Tiller changes no Git refs or host state;
report external follow-through separately. If already merged, sync first and use forward work.

For recorded review history, `transfer_review_unobservable` refuses when neither the local
branch nor a usable submitted head establishes merge state. Restore/fetch review history,
sync, and inspect before deciding again; missing history is not evidence of an unmerged review.
Observed merges and squash-matched submitted history refuse as `transfer_review_merged`.
Use the exact returned `start_command` and `follow_through`; an external Git/host failure does
not roll back the committed source transaction.

Contributors are sorted distinct owners of started descendant tasks, including done/cancelled
tasks and transferred originals. Reservations, review actors, and attached shelf owners do not
count. Personal work is unfinished task/bug/chore/tweak assignments, including proposed/blocked/check/
in-review work. Shared closeout appears once per applicable feature/epic, not once per contributor.

## Execute (the task loop)

`check` records results even when required acceptance is unfinished. Exit `0` and
JSON `passed: true` mean every applicable required gate passed. A failed or pending
required gate returns exit `1` and `passed: false`; task and shelf work do not
advance to `check`. The output names outstanding required criteria and any human
attestation command. Optional reviews remain non-blocking.

```sh
tiller start task <task>                                     # ready -> in_progress; freezes owner, records
                                                             # branch, activates ready parent atomically
tiller start task <task> --adopt-aggregate                   # first coarse start that records the existing
                                                             # ref at the derived aggregate name; review
                                                             # git log <integration>..<ref> first; lists the
                                                             # commits it holds beyond integration
tiller start <bug|chore|tweak> <node>                        # the same on a shelf node; it records its
                                                             # own branch, tiller/bugs/<key>
tiller check task <task>                                     # run and record build and tests; green -> check;
                                                             # a gate already passed on unchanged code is
                                                             # reused, not rerun
tiller check task <task> --rerun                             # force real runs: a suspected flake or a changed
                                                             # environment
tiller check task <task> --gate tests                        # run only the named gate (repeatable); the rest
                                                             # count only if already current (a pass that is
                                                             # not is marked stale), and the task advances
                                                             # only on a full green set
tiller check <bug|chore|tweak> <node>                        # the task-grade gates on a shelf node; the
                                                             # evals gate never applies, there being no
                                                             # acceptance contract at this grain
tiller submit task <task> --pr <number>                      # check -> in_review, refused (gates_not_green)
                                                             # while a latest required result is not
                                                             # pass; --pr required when a
                                                             # remote exists and the resolved review grain
                                                             # is task. Under feature or epic grain --pr is
                                                             # refused instead and a review ref is recorded,
                                                             # even with a remote: the task carries no PR
                                                             # of its own at a coarser grain
tiller submit feature <feature> [--pr <number>]              # record-only: the feature's own PR, or,
                                                             # remote-less, its review ref; leaves status
                                                             # untouched. Requires resolved review grain
                                                             # feature, every task in_review or done (one
                                                             # at least), and required feature-grain gates
                                                             # recorded pass
tiller submit epic <epic> [--pr <number>]                    # the same, one level up: resolved review
                                                             # grain epic, every live feature's tasks
                                                             # in_review or done and its own gates green
tiller submit <feature|epic> <node> [--pr <number>] \
  --recover-merged --head <historical-reviewed-commit> \
  --merge-commit <integration-result-commit> --reason "<explanation>"
                                                             # record-only historical binding on integration
tiller submit <bug|chore|tweak> <node> [--pr <number>]       # check -> in_review; a shelf node always
                                                             # records its own PR or review ref, since
                                                             # review grain does not apply to it; refused
                                                             # (gates_not_green) while a latest required
                                                             # result is not pass, as on a task
tiller resume task <task> --reason "<what changed>"          # in_review -> in_progress, PR kept; stales only
                                                             # results whose code identity changed
tiller resume <bug|chore|tweak> <node> --reason "<what changed>"  # the same, one shelf over
```

An open question holds the nodes it blocks: `start`, `check`, and `submit` refuse a held node
until the question is answered or cancelled. A feature, never a task, may instead proceed under a
recorded assumption: `--assume-question` names the open question and `--reason` is required.

```sh
tiller check feature <feature> --assume-question <Q-id> --reason "<why>"  # record the assumption in the
                                                             # feature's assumption_basis, then run and
                                                             # record its gates
tiller approve feature <feature> --assume-question <Q-id> --reason "<why>" # human: record the assumption
                                                             # only; no lifecycle move, and task start
                                                             # does not inherit it
```

`submit task` and `check task` also parse `--assume-question` and `--reason`, but refuse them with
`assume_unsupported` (exit `2`): tasks cannot assume away a blocking question, so answer it or
cancel one that no longer needs a decision. `start` does not take the flag. Once an assumed
question is answered, `submit` refuses the unreviewed assumption (`answered_question`) until
`tiller approve feature <feature> --reviewed-assumption <Q-id>` stamps the answer's revision.

`submit` only records an existing PR number or permitted review ref. The agent performs publication
with Git and the project's host tools: commit implementation, check and commit gate state, push,
create or reuse the correct PR, submit its actual number, then commit and push submission state.
Both pushes belong to an authorized execute run unless the developer explicitly says not to publish.
A successful unchanged submit, such as reusing the same coarse PR, needs no empty state commit
or redundant push. Commit only actual deltas and verify the state already published.
Checked work re-enters without `start` or `resume`; requested changes use `resume`, rerun checks,
reuse the PR, and resubmit. No remote means a local branch-and-commit review ref, with no host or
push requirement. A coarse task uses its permitted ref and continues to the feature or epic finish
line, where the single PR is published and recorded. Shelf work owns its branch and review.

Merged aggregate recovery requires explicit authorization for the named review, both hex commit
IDs, and a nonblank reason. With a remote, use the positive actual PR number and verify host
repository, merged state, base, head repository/branch/OID, and result through ordinary host tools.
Tiller checks local immutable Git evidence, never host state; its PR association is caller-supplied.
Explicit recovery-only flags without enabled recovery, including empty values or false recovery,
refuse, as do task/shelf nouns. Historical submitted task blobs and included heads/retained tips
must prove the riding set; deleted aggregate/task refs are allowed when those anchors remain.

Recovery changes only the owning aggregate review binding and audit event. It requires no green
historical/current acceptance and never moves status, runs gates, or approves criteria. Commit its
actual state on configured integration, run ordinary sync, inspect historical warnings and current
acceptance independently, commit actual sync state, publish when authorized, and read it back.
An identical retry writes nothing. Exact merge/squash evidence uses ancestry/content; contained
work without a local witness remains pending for the human's existing fast-forward assertion.
Do not reset refs, open an extra PR, or commit to the finally merged branch.

Every task uses its recorded full-canonical-key branch, unchanged on resume. The default
pattern yields `tiller/<epic>/<feature>/<task>`; aggregates use
`tiller/reviews/features/<epic>/<feature>` or `tiller/reviews/epics/<epic>`.
The model creates independent task branches from integration. Task-PR dependencies wait for
integration, never stacked task PRs. A coarse dependent task may start from the aggregate once
submitted prerequisites and their submission-state commits have been merged into it.
If a hand-built ref already sits at the derived aggregate name and nothing records it, plain
start refuses `aggregate_unrecorded`. Inspect `git log <integration>..<ref>`, then rerun with
`--adopt-aggregate` to record it, or rename the ref aside with Git. Tiller will not delete refs.
The flag refuses at task grain, on shelf nouns, when an aggregate or review is already
recorded, when the name is a task's branch, when no ref exists, and on prefix conflicts.

Submit a task on its own branch, commit the resulting state there, then ordinary-merge its
current tip into the recorded aggregate. Before feature checks, PR preparation, or coarse
submit, assemble every riding task's recorded review head and current branch tip. Coarse
submission verifies their ancestry. No cherry-pick or squash assembly. Requested changes
resume the affected task's own branch, check/resubmit, commit state, reassemble, rerun affected
feature checks, and reuse the same aggregate PR. The model handles branch work and mechanical
conflicts; semantic decisions, acceptance, and the final integration merge remain human-only.
Prefix collisions with retained refs name both refs: preserve them and use a non-conflicting
branch pattern for unstarted work, or ask the human to archive or rename one deliberately. Never auto-delete refs.
Read start's `branch` and `base_branch` before creating refs. A first task may stamp a missing
aggregate after a safe grain change; reopened coarse rounds get a new aggregate suffixed with
the new task id, preserving merged refs. Git merges preserve nonoverlapping task state and
events. Never hand-edit machine state to resolve competing real decisions.

Do not force-push or change remotes to bypass a refusal. If a PR exists but submit refuses, report
both facts. If submit succeeds but the state push fails, retain its recorded state and name the
unpublished commit. Ordinary publication is not human-only; approval, attestation, merge, and
`sync --accept-fast-forward` keep their existing human boundaries.

Coarse submit still runs the retired-ancestor, active-basis, operative-question,
answered-assumption-review, task-readiness, and review-identity guards after gates are green. It also refuses an
older open task PR. An overridden feature under epic grain finishes its own PR and reaches `done`
before the agent publishes and records the shared epic review.

## Acceptance and gates

The acceptance contract lives at feature grain, in `feature.eval.md`. There is no epic contract.

```sh
tiller eval <feature|task> <node>                            # run the acceptance contract, report what is unmet
tiller eval <feature|task> <node> --emit-rubrics             # write rubric packets for the agent to grade
tiller eval <feature|task> <node> --criterion <C-id> --record-rubric-output <path/to/graded.json>
tiller eval <feature|task> <node> --rerun                    # rerun command and test criteria that would be reused
tiller check <feature|task> <node>                           # run and record all applicable gates, reusing a pass on
                                                             # unchanged code; --rerun forces runs, --gate <name> limits them
tiller attest <feature|task|bug|chore|tweak> <node> --gate <gate-name> --status pass|fail [--reason "<note>"]  # human only;
                                                             # refused (command_gate) for a gate the
                                                             # manifest declares kind: command
tiller attest <feature|task> <node> --criterion <C-id> --status pass|fail [--reason "<note>"]  # human only
tiller answer question <question> --answer "<the answer>"    # human only: open -> answered
tiller cancel question <question> --reason "<why>"           # human only: open -> cancelled; releases every hold
                                                             # without an answer or drift; cancelling a cancelled
                                                             # question is a no-op, an answered one is question_closed
tiller link question <question> --blocks <node> [--blocks <node>]...  # widen what an open question blocks
                                                             # (an agent may); a present target is a no-op
tiller unlink question <question> --blocks <node> [--blocks <node>]... # human: narrow it; removing every
                                                             # target is legal; --blocks is the only axis
```

Every command that reads a feature's contract (`spec feature`, `check`, `eval`, rubric recording,
and `attest --criterion`) validates it first. A contract with no criteria, a repeated criterion id,
an unrecognized binding, or a binding missing its required field refuses with
`eval_contract_invalid` (exit `2`), naming the offending criterion, before any binding runs or any
state is written.

`attest feature` refuses off its coarse aggregate branch: at feature or epic review
grain, with the aggregate recorded and its owner not done or cancelled and no recovered review, a
run on any other branch (integration, another branch, or a detached `HEAD`) exits `1` with reason
`attest_branch_mismatch`, writes nothing, and names where to run it. When a linked worktree holds
the aggregate it says `run this attest in the worktree at <path>, then commit there before submit`;
when the aggregate exists locally but is checked out nowhere, `check out "<branch>", run this attest
there, and commit before submit`; when the local ref is missing, `run git fetch && git switch
"<branch>", run this attest there, and commit and push before submit`. Task grain, an unrecorded
aggregate, and attests on tasks, bugs, chores, and tweaks are not guarded.

A `test` binding's `ref` is one path or test id, or a YAML list of them
(`ref: [spec/a_spec.rb, spec/b_spec.rb]`). A whitespace- or semicolon-joined string is read as one ref, and
`tiller drift` reports it as unresolvable (`ref_unreadable`) with the list advice rather than
guessing at the split. When a done feature's bindings need correcting (a test moved or split, a
baseline artifact relocated), edit only the bindings and stage `tiller amend <amendment>
--correct-eval <feature> --reason "<why>"`, with `--retire <criterion-id>` for each criterion
deliberately removed. The human's `tiller approve amendment` re-pins `verified_against.eval`,
which clears the eval leg of `stale_spec`.

## The done crossing and rollups

Nothing marks work done by hand. The human merges, `sync` observes the merge, and the feature and
its epic roll up behind the tasks that crossed.

Sync resolves the integration branch to one commit before it looks for any work, so an unborn
repository or a checkout without a local copy of that branch refuses `integration_ref_missing`
even when nothing is in review; fetch or create the branch, or pass `--into`. A feature crossing
whose integration-commit `feature.eval.md` is not the accepted one (`accepted_against.eval`, for
example an eval refreshed in the working tree and never committed) still crosses, with an advisory
`eval_contract_mismatch` on that crossing in JSON and an `Eval contract differs from the accepted
one (advisory):` block in human output naming the feature and both blobs.

```sh
tiller sync [--into <integration-branch>]                    # observe merges, record done crossings;
                                                             # refused (integration_ref_missing, exit 1)
                                                             # when the integration name resolves to no
                                                             # commit, even with nothing in review
tiller sync --accept-fast-forward <node> [--accept-fast-forward <node>]...  # human only:
                                                             # assert a fast-forward merge sync cannot
                                                             # witness; recorded as human_attested. The
                                                             # task for task-grain work; recorded coarse
                                                             # review node for aggregate work; bug/chore/tweak
                                                             # for shelf work
tiller archive <epic>                                        # human: done -> archived
tiller cancel <epic|feature|task> <node> --reason "<why>"    # human: -> cancelled, terminal
tiller cancel <bug|chore|tweak> <node> --reason "<why>" \
  --resolution wontfix|invalid|duplicate [--duplicate-of <node>]  # human: --resolution is required on
                                                             # a shelf node and accepted on no other
                                                             # noun; duplicate also names the surviving
                                                             # node, which must be of the same noun
```

A bug, chore, and tweak each carry a branch and a PR of their own, so `sync` observes each on its own
branch exactly as it observes a feature's, by ancestry then content, and records `done` with the
basis it observed. The one crossing that rests on no branch is a shelf node with a carrier: see
Bugs, chores, and tweaks below.

## Bugs, chores, and tweaks

Three shelves beside the work tree, `.tiller/bugs/`, `.tiller/chores/`, and `.tiller/tweaks/`, each holding leaf nodes
with the task's lifecycle: `proposed` (captured) to `ready` (triaged, by a human's `approve`) to
`in_progress`, `check`, `in_review`, `done`. Each carries its own branch, its own PR, and its own
claim. `attached_to` records where the node was found and is provenance only: it never gates
anything and it is not parenthood. Amendments change the work tree; bugs, chores, and tweaks do not.

```sh
tiller add bug <key> --title "<title>" [--attach <epic|feature|task>] [--source <cite>] \
  [--depends-on <bug|chore|tweak>]...                        # lands proposed at .tiller/bugs/<key>/,
                                                             # holding bug.md (the contract) and bug.yml
tiller add chore <key> --title "<title>" [--attach <node>] [--source <cite>] [--depends-on <node>]...
tiller add tweak <key> --title "<title>" [--attach <node>] [--source <cite>] [--depends-on <node>]...
                                                             # tweak.md and tweak.yml, with a TW- ID
# On add bug|chore|tweak, a blank --attach is the same usage error as on link
# (attach_target, exit 2, nothing written); omit --attach for no attachment.
tiller approve <bug|chore|tweak> <node>                      # human: triage, proposed -> ready
tiller return <bug|chore|tweak> <node> --reason "<why>"      # human: ready -> proposed, before start
tiller link <bug|chore|tweak> <node> --attach <epic|feature|task>  # record where it was found, after add time;
                                                             # a blank --attach is a usage error
                                                             # (attach_target, exit 2), never a detach
tiller link <bug|chore|tweak> <node> --depends-on <bug|chore|tweak> [--depends-on <bug|chore|tweak>]...
tiller order bug <b1> <b2> ...                               # the shelf's own soft order; no parent to
                                                             # name, so --clear takes none either
tiller order bug --clear                                     # erase the bug shelf's list
tiller order chore <c1> <c2> ...                             # and the chore shelf's
tiller order chore --clear                                   # erase the chore shelf's list
tiller order tweak <tw1> <tw2> ...                           # and the tweak shelf's
tiller order tweak --clear                                   # erase the tweak shelf's list
tiller resolve <bug|chore|tweak> <node> --by <epic|feature|task>  # record the carrier whose merge fixes it
tiller resolve <bug|chore|tweak> <node> --clear              # return it to the queue
```

`resolve` records a carrier and moves no status. The node keeps its stored state, reads the derived
phase `carried`, and stops waiting on work of its own; in the run that records the carrier's `done`, `sync`
crosses the node `done` with `done_basis: carried`. A cancelled or archived carrier is refused when
it is named, and a carrier cancelled afterwards flips the derived phase to `carrier_cancelled`,
which `show` and `status` print beside the node's status: `sync` then treats the node as having no
carrier at all and observes its own branch again. The `resolved_by` pointer stays on the record
either way, for `resolve --clear` or a re-pointing `--by` to settle. `resolve` sits in the approve
delegation class: a human runs it, or an agent does on an explicit instruction naming both the node
and the carrier.

Everything else a task does, a bug, chore, and tweak do with the same commands: `claim`, `release`,
`reassign` (Ownership above), `start`, `check`, `submit`, `resume` (Execute above), `cancel` with
its required `--resolution` and `sync` (The done crossing above), `rename`, `unlink`, `remove`
(Correct above), and the reads `show`, `ls`, and `status`. `tiller queue` carries proposed shelf
triage as `approve_bug`, `approve_chore`, and `approve_tweak`, plus stale carrier pointers as
`carrier_cancelled`. There is no `merge_bug`, `merge_chore`, or `merge_tweak`. Optional review gates accept
`attest bug|chore|tweak --gate`; criterion attestation and every `eval` form remain feature/task-only.

## Amend (a reshape of the plan)

Staging records proposals and executes nothing. One human `approve amendment` validates every
precondition against the working tree and then applies the whole batch, or none of it.

```sh
tiller add amendment <amendment-key> [--title "<title>"] [--triggered-by <design/doc.md>]... \
  [--implements <ADR-id>]                                    # opens it, records base_revision from HEAD
tiller intake <file> [--key <key>] [--title "<title>"]       # the other way to open one: lands <file> under
                                                             # .tiller/feedback/, fingerprinted, source set
# exactly one operation per call, plus any number of --precondition name=value guards:
tiller amend <amendment> --add-epic <epic-key> [--title "<title>"]  # a root: no parent to resolve
tiller amend <amendment> --add-feature <epic-key>/<feature-key> [--change-type create|modify|replace|migrate] \
  [--title "<title>"]
tiller amend <amendment> --add-task <epic-key>/<feature-key>/<task-key> --title "<what it accomplishes>"
# add-feature's epic may be live or a same-amendment add-epic staged first;
# add-task's feature must still be live, never same-batch
tiller amend <amendment> --add-question "<statement>" --title "<short title>" [--blocks <epic|feature|task|staged key>]...  # no key at all
tiller amend <amendment> --supersede <feature|task> --by <feature|task|staged key> # done target only at approval
tiller amend <amendment> --cancel <feature|task> --reason "<why>" [--superseded-by <feature|task|staged key>]
tiller amend <amendment> --revise <feature|task>             # non-terminal target only at approval
tiller amend <amendment> --correct-eval <feature> --reason "<why>" [--retire <criterion-id>]...
# done feature only at approval; bindings and baseline artifacts only, re-pins verified_against.eval
tiller amend <amendment> --move-feature <feature-id-or-key> --to <epic-key>/<feature-key> --reason "<why>"
# unstarted subtree only; destination epic may be live or staged earlier; identities and lifecycle survive
tiller amend <amendment> --amends-design <design/doc.md>     # rides alone or beside an operation
# --source <citation> rides alongside any operation above, free text by convention
# path#Lstart-Lend (more ranges of the same file after commas: path#L3-L8, #L40);
# required on every op once the amendment it stages onto was minted by intake
# the five precondition names: target_status, target_revision, key_absent, spec_revision, eval_revision
tiller show amendment <amendment>                            # the batch, each precondition's live validity
tiller approve amendment <amendment>                         # human: proposed -> approved, and applies it
tiller withdraw amendment <amendment> --reason "<why>"       # human: proposed -> withdrawn, applies nothing
tiller ls amendments [--status proposed]                     # what reshape is waiting
tiller readout amend                                         # the staged batches and their live preconditions
```

## Configure

`review_grain` is the one setting `configure` records today: `task` (default), `feature`, or `epic`,
at three homes, most specific wins. A change is safe only for affected descendants with no feature
review surface, no task in `in_progress`, `check`, or `in_review`, and, when entering or leaving
epic grain, no recorded aggregate branch. Existing task PRs remain independent and must merge and
sync before a later coarse submit. Do not hand-edit state to evade a refusal.

```sh
tiller configure project --review-grain task|feature|epic     # writes execution.review_grain in tiller.yml
tiller configure epic <epic> --review-grain task|feature|epic
tiller configure feature <feature> --review-grain task|feature  # a feature cannot say epic
```

A changed grain also prints: "Commit this change to integration before starting tasks: work started in another checkout or worktree resolves the grain it can see."

Stored values are validated on load, including hand edits. `configure project` also refuses before
writing when `execution` is a scalar or flow mapping; rewrite it as block-form YAML and rerun the
command.

The same setting also rides `approve`, recorded in the same act as the start gate:

```sh
tiller approve epic <epic> --review-grain task|feature|epic     # human: draft -> ready, grain recorded together
tiller approve feature <feature> --review-grain task|feature    # human: specced -> ready, grain recorded together
```

## Migrate

```sh
tiller migrate                                               # schema conversion plus generated refresh
tiller migrate --dry-run --json                              # read-only inspection; writes nothing
tiller migrate --ownership-review <file>                     # apply a human-reviewed ownership mapping
```

Migrate uses the running binary and never changes the installed binary or checkout identity. It
applies the registered forward migrations, records one `migrate.schema` event for a real
migration, then runs the same refresh as upgrade: orientation, skill shims, harness wiring, and
required ignore entries. Current-schema refresh needs no identity; a real migration needs a
registered actor. Repeating successful maintenance writes no redundant event or identical
artifact. A refresh failure is nonzero even after a committed migration; keep the completed source
changes and repair the named cause before retrying. Review and commit the changed files.
`--dry-run` and `--ownership-review` apply only to a project on an old schema that predates claim
ownership. The [migration contract](spec/distribution.md#migration) owns schema boundaries,
drain requirements, and the reviewed ownership format.

## Upgrade

```sh
tiller upgrade                                               # update the binary, then migrate or refresh this checkout
tiller upgrade --project-only                                # offline maintenance with the running binary; no brew or network
tiller upgrade --binary-only                                 # update only the installed binary
tiller upgrade --version vX.Y.Z                              # an exact self-managed release, if compatible with this checkout
tiller upgrade --contributor <id>                            # registered migration actor; never selects checkout identity
tiller upgrade --project-only --ownership-review <file>      # offline reviewed migration; the flag requires project-only
```

Project-only and binary-only are mutually exclusive. Project-only with `--version` is misuse,
and project-only outside a project refuses. Normal upgrade outside a project updates only the
binary and reports other checkouts untouched. From a subdirectory, maintenance targets the
current resolved project root; it never scans other repos or worktrees.

Unpinned upgrade never downgrades an already-ahead binary. Already-current skips archive
download and replacement but continues project maintenance. Self-managed candidates are
checksum-verified and inspect the project before replacement. Known blockers, a newer schema or
orientation, and an unsupported pinned target's maintenance protocol refuse before replacement.
Use `--binary-only --version ...` only for an intentional binary-only downgrade.

Verified Homebrew installations delegate only the matching installed Tiller cask or formula to
its owning brew prefix. Pinning refuses before brew; an unverified brew-looking path never falls back to a
manual swap. The child disables installed-dependent checks and automatic cleanup with
`HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1` and `HOMEBREW_NO_INSTALL_CLEANUP=1`. Brew may refresh
metadata or install genuine package dependencies, and it can already have changed the machine
before the new target's compatibility is known; a failure does not trigger a manual swap.

Before replacement the outgoing build is verified and retained at
`~/.local/share/tiller/previous/<version>-<sha256>/tiller`; distinct builds remain until explicitly
removed. Retention is recovery material, not proof of old-schema compatibility. Never mutate a
migrated checkout with an older binary.

Exit 0 means every requested phase completed or was genuinely inapplicable. Binary, migration,
and refresh are separate durable boundaries. Nonzero output preserves actual completed stages;
`mutation_recovery_needed` requires transaction recovery before retry, and lost child output
leaves unestablished phases unknown, not unchanged. Never remove cache locks or journals.
JSON success and exit-1 refusals carry the maintenance result in `data`; exit-2/3 errors may
carry it in `error.partial_result`, never alongside an outer `data` block.

After success, review and commit the reported diff, restart existing serve processes, and reload
harness instructions as needed. Other checkouts use `tiller upgrade --project-only` when
revisited. A release-service failure is not already-current success; project-only remains the
explicit offline path. Join remains first onboarding or deliberate identity selection. See
[distribution](spec/distribution.md#update) for installation ownership, retention, and recovery.

## Help and shell completion

```sh
tiller --help
tiller help [command]
tiller <command> --help
tiller --version
tiller completion bash|fish|powershell|zsh
```

## Reads

```sh
tiller status                                                # project summary
tiller status <noun> <node>                                  # state, next moves, basis drift, relationships
tiller show <noun> <node>                                    # the node in full: gates, criteria, attestations, basis
tiller ls <noun> [--status <state>] [--owner <id>] [--contributor <id>] [--epic <epic>] [--feature <feature>]
                                                             # the noun takes the plural collection form
                                                             # (bugs, chores, tweaks) or the singular
tiller queue [--kind <queue-kind>] [--noun <noun>]           # exact current attention entries; read-only
tiller tree [<epic|feature> <node>] [--status] [--files]     # the work tree, whole or rooted at one node
tiller search "<keyword>" [--kind <kind>]                    # an id, key, or ref names its node first;
                                                             # else node fields + project-authored regular
                                                             # UTF-8, NUL-free lowercase .md under .tiller,
                                                             # one ranked result per node or document;
                                                             # orientation.md validated, not returned
tiller query --from <address> [--from <address>]... --follow <direction[,direction]...> \
  [--depth <nonnegative-integer>] [--where <kind|status|owner|contributor>=<value>]...
                                                             # recorded relationships, provenance, and completeness
tiller drift                                                 # sweep every check drift.checks enables;
                                                             # reports candidates (what moved) and uncovered nodes
                                                             # (what a check could not compare) separately. Exit 0
                                                             # asserts full coverage; exit 1 means it found
                                                             # candidates, uncovered nodes, or both
tiller serve [--host [<ip>]] [--port <n>] [--open=false]     # read-only console; defaults 127.0.0.1:4400
                                                             # opens your browser unless --open=false,
                                                             # console.open: false in .tiller/local.yml, or BROWSER=none
```

The console can show task execution state from validated local task worktrees or locally
available branch tips without copying state into integration. Queue, ranked next, and workflow
guidance remain based on the served tree, and CLI eligibility is unchanged. Navigation or
manual refresh rereads sources; remote-tracking refs remain as of the developer's last
external fetch. See the [console source contract](spec/ui-surface.md#task-execution-sources)
for source priority, confined reads, task history, and source-pinned evidence.

Bare `--host` uses `0.0.0.0:4400`, exposing every IPv4 interface. `--port` overrides the
port, including `--port 0` for an available ephemeral port. Explicit addresses work as
`--host <ip>` or `--host=<ip>`. Omitting `--host` keeps the loopback-only default.

Serve accepts bare IPv4 and IPv6 literals, not hostnames, bracketed values, ports, zones,
IPv4-mapped IPv6, multicast, limited broadcast, or link-local addresses. Scoped/link-local
serving is unsupported. Specific IPs must be assigned locally. IPv4 and IPv6 listeners are
explicitly separate: `0.0.0.0` covers all IPv4 interfaces; `::` covers all IPv6 interfaces.

```sh
tiller serve --host --open=false
tiller serve --host 192.168.1.50 --port 4400 --open=false
tiller serve --host :: --port 0 --open=false
```

Replace the example LAN IP with your own locally assigned address. Network binds expose the
planning ledger, the `.tiller/` Markdown corpus, Markdown documents the ledger cites, and gate
evidence files the ledger records, without authentication or TLS; no other checkout file is
served. Use only a trusted network or private
VPN, never direct public-internet exposure. Wildcards expose all interfaces of their family,
including other connected networks; choose a specific IP to restrict the listening interface.
Wildcard admitted addresses are a startup snapshot, so restart after interface address changes.
Only the console's History view advances its last-visit mark; other requests to the recap,
such as a `curl` or a foreign page, read it without moving the mark. Serve prints the listen address separately from usable IP URLs and opens the family's loopback
URL. Specific binds print and open the bound IP URL. All URLs use the actual port, including
`--port 0`, and bracket IPv6. See the [serve contract](spec/cli-surface.md#tiller-serve) for Host
sets, request protections, plain-HTTP limitations, and stable startup error reasons.

The exact retired `.tiller/board.md` coordinate is excluded before opening or validation, including
any directory subtree. It supplies neither search hits, discovered mentions, nor a file destination;
direct prose reads return ordinary not found. Explicit authored citations remain unresolved with
their provenance intact. Authored paths such as `.tiller/design/board.md` and unsafe orientation
retain the ordinary corpus behavior and safety refusals.

Owner filters match scalar execution/question ownership, never ancestors. Contributor filters match
started-task membership on features and epics. For example `tiller ls features --contributor dana`
and `tiller query --from E-abc123 --follow contains --depth 2 --where contributor=dana --json`.
Query filtering is post-traversal so ownerless ancestors remain bridges.

### Relationship discovery

```sh
# Recorded open-question targets under an epic, not effective holds.
tiller query --from E-abc123 --follow contains,blocked_by --depth 3 \
  --where kind=question --where status=open --json
# Done work claiming a file or a containing directory scope.
tiller query --from path:internal/store/write.go --follow covered_by --where status=done --json
# Document citations, including labeled generated mentions.
tiller query --from F-abc123 --follow mentioned_by --json
```

IDs are illustrative. `--from` and `--follow` are required. Starts accept immutable IDs,
unambiguous canonical keys, and explicit `doc:`/`path:` addresses; indexed unresolved endpoints
can be inspected by exact address. Unknown or ambiguous work starts refuse. Repeat starts to union
and deduplicate them. Follow names are a union at every hop, not ordered stages; use `query --help`
for every forward/inverse name. Depth defaults to one; zero inspects only starts. Filters use exact,
case-sensitive equality, AND together, and select after traversal, so filtered-out bridges remain
in the response. Breadth-first results sort by minimum depth then identity and never silently cap.

Coverage preserves work-surface, baseline-description, or acceptance-binding provenance; directory
matches use segment boundaries and mark enclosing-scope matches derived. It is not verified
implementation coverage. Bindings are never executed; bare test IDs, rubric text, and shell commands
are not file claims. Question edges record targets regardless of state, not effective inherited
holds or assumption waivers: use `status` and `readout` for those. Pinned revisions remain evidence;
opening a current file does not show historical bytes. External coordinates never become local links.

The normal human/default and `--json` envelope applies. Empty matches succeed. `complete: false`
names unavailable eval/history facets or ambiguous identity; an indexed dangling reference alone is
not incomplete. Unsafe/unreadable corpus and unstable-generation reads fail, never return partial
success. Bad arguments or missing work starts exit 2; read/environment failures exit 3.
`GET /api/query` accepts equivalent repeated `from` and `where`, comma-separated `follow`, and
`depth`; its payload equals CLI `data`, with HTTP 400/404/500 for those error classes.

Every read rebuilds and discards its index. It needs no checkout identity and writes no files,
events, Git-index refresh, or persistent cache. Manual prose edits and branch changes appear on the
next request; arbitrary edits racing a read are not an atomic snapshot.
The [CLI specification](spec/cli-surface.md#relationship-queries) owns the full catalog and semantics.

Baseline checks use the same drift command. Fresh projects enable `stale_design`, which joins
confirmed docs to overlapping done-feature crossings and also detects vanished described paths.
`shifted_design` is opt-in and compares current working-tree code with the confirmation, including
untracked nonignored files and external reference trees. Existing projects retain their own
`drift.checks`; add either marker explicitly to enable it. Unconfirmed or uncheckable docs are
uncovered, not clean. The footer counts docs separately from work nodes.

Successful feature crossings include `touched_design` in JSON and one aggregate
`as-is docs touched: ...; run tiller drift` line. Neither command writes the baseline ledger.
`tiller guide mend-drift` explains the repair: review the reported evidence, correct the prose,
then stop for human approval of the named design docs. Registered design-file pages in the
console show confirmation, provenance, and the same candidates and uncovered failures.

## Not built

The specs retain unbuilt designs for the ADR noun and `tiller add adr` (which is why an amendment's
`--implements` is optional), `add design`, `add evidence`, `block`/`unblock`, `doctor`,
and `commit state`. The proposed `--commit` and `--dry-run` behavior is also
unbuilt. Check the [roadmap](./roadmap.md#not-built-yet) before treating a design example as an
executable command.

A separate set is gone by design rather than unbuilt, and will not return: `accept`, because feature
and epic `done` are computed rollups `sync` records; `add blitz`, `schedule`, and `unschedule`, with
the effort axis they belonged to; `break-down`, folded into `specify`; and `resume feature`, since a
requested-changes review always routes back through the task actually under review, never through the
feature or the epic whose PR that task may be riding. `submit feature` and `submit epic` exist as
`submit task`'s record-only coarse-grain siblings, under Execute above.

`ci-status` and Tiller-backed CI or branch protection are also rejected by design, not deferred or
renamed. Managed-project CI must not install or invoke Tiller, even in an optional advisory job,
and merge eligibility must not depend on Tiller availability or state validation. Local Tiller
may invoke independently runnable project checks, never the reverse. See the
[distribution contract](spec/distribution.md) for project independence and the planning-only CI alternative.
