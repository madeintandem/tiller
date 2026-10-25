---
title: Source walkthrough
description: Run the current source build through project setup, human gates, task execution, merge, sync, and maintenance.
---

From a repository to a tree you can see. The walkthrough uses a greenfield scratch project called Acme, with an existing-code path below; ids and commit hashes are illustrative.

This page documents source stamp **0.21.2**, schema **12**, orientation generation
**57**, and **eleven embedded skills**. The latest published release is **v0.18.4**;
the [release Quickstart](/tiller/quickstart/) is prepared for 0.21.2.

## Build the matching source

From a trusted checkout of this repository, with Go available:

```sh
go build -o /tmp/tiller-source ./cmd/tiller
/tmp/tiller-source --version
mkdir -p /tmp/tiller-source-bin
cp /tmp/tiller-source /tmp/tiller-source-bin/tiller
export PATH="/tmp/tiller-source-bin:$PATH"
```

Verify the version is **0.21.2** before using the commands below. This is a source
build, not an anonymous release-installation check. Use a disposable project;
do not migrate an existing team's checkout merely to try the walkthrough.

## Turn a repository into a tiller project

In any git repository (a fresh one is fine):

```sh
tiller init --name "Acme Platform" --key acme --kind greenfield
```

```
Initialized "Acme Platform" (acme, greenfield).

**Next**
  1. Copy your design inputs under .tiller/design/ (--design imports only on the init that scaffolds).
  2. Register yourself as a contributor (a human, once per project):
       tiller add contributor <id> --name "<full name>"
  3. Join, wiring your identity and your agent:
       tiller join --as <id>
  4. tiller check readiness
  5. tiller guide decompose
  6. From there the feature frontier paces the work, one feature specified
     when its turn comes. This names the seam whose turn it is:
       tiller guide

The build and tests gates wire later in .tiller/tiller.yml, once a stack
exists; the evals gate is already active.
```

`--kind` describes the relationship to code that predates Tiller adoption: `greenfield` for a new system, `brownfield` for this repository's existing system, or `legacy-migration` for a new system replacing an old reference system. It does not restrict later feature change types. A greenfield project's own shipped code can later be modified, replaced, or migrated without changing kind. `init` scaffolds `.tiller/` and writes `.tiller/orientation.md`, the generated page an agent reads first.

### Bringing existing code

Choose `--kind brownfield` to adopt the system in this repository, or `--kind legacy-migration`
to build a replacement for an old system. Both start with an explicit writable code root:

```yaml
inputs:
  code:
    - { root: ., mode: surface }
```

When this repository has an observable HEAD, first scaffold also records `project.adopted` with
its `git:<short-sha>` and a UTC observation time. This is not a dirty-worktree fingerprint or the
HEAD of an external legacy repository. Non-Git and unborn repositories initialize without the
anchor. Reruns preserve the original manifest, including any absent anchor.

For legacy-migration, add the old system's existing path to the human-owned `.tiller/tiller.yml`
before readiness. This is required even when you imported docs with `--design`. For example,
if the old system lives in the sibling directory `../acme-legacy`, keep the surface declaration
and add:

```yaml
    - { root: ../acme-legacy, mode: reference }
```

Every declared root needs an explicit `surface` or `reference` mode. An empty or misspelled mode
fails readiness and surface recording, including on older greenfield manifests. Correct the
manifest rather than expecting `migrate` to rewrite it. A reference root is read-only input:
recorded surfaces cannot equal it, sit beneath it, or contain it, including through an alias.
Choose specific writable paths instead of a parent directory that includes legacy code.

After registering a contributor and joining, run `tiller guide derive-baseline` for either existing-code
kind. It reads existing docs first, then brownfield surface code or legacy reference code. Derive
undocumented code or translate docs at module grain, reconciling current code rather than assuming
an import is accurate. Use adoption history when available, but say when it is missing.

Ask once whether in-repo doc ownership moves into `.tiller/design/` or another consumer needs a
pointer page. Do not leave two live copies silently. An external `--design <dir>` import remains
different: first scaffold copies its source without changing or deleting it. A valid rerun succeeds
but skips the import; add later inputs directly. Commit exact uncommitted source inputs, including
local imported copies, before translation so `--from` can record recoverable project-HEAD blobs.
Record that provenance before retiring agreed in-repo sources; never retire external originals.
Fresh imports refuse a root-level `baseline.yml` entry, including case variants, before scaffold
writes. Use a prose-only source directory; importing another project's confirmations is not supported.

```sh
tiller guide derive-baseline
# Draft .tiller/design/as-is/editor.md after reading its docs and code.
tiller link design as-is/editor.md --describes app/editor
tiller link design as-is/editor.md --from docs/editor-spec.md
# Retire only the agreed in-repo source after recording provenance.
# Commit exact reviewed docs, ledger, question, and event paths on the integration branch.
tiller readout derive-baseline
# Human review, then confirmation of named docs:
tiller approve design as-is/editor.md
tiller check readiness
tiller guide
```

The readout lists every recorded source path plus blob mapping and every ledger doc, described
path, and confirmation fact. Review translations against those recoverable blobs and current code.
Run notes separately name dropped, merged, or unplaceable material and actual questions opened.
Ledger counts do not prove all source meaning survived; `Open questions` is the current count.
The completed baseline readout stops at one HUMAN review action. Pending docs on re-entry need
inspection or review, not duplicate derivation. The agent never decides confirmation.

`approve design` leaves Markdown alone. It atomically confirms all named docs against working-tree
code and one project HEAD; it does not use `project.adopted`, an external HEAD, or assert a clean
tree. Changed code, additions, and removals affect its fingerprints. Describes may name code equal
to or beneath declared reference roots, including `../acme-legacy`; containing a declared root is
not enough. Directory inventory in Git includes present tracked and nonignored untracked files,
while a declared external non-Git tree includes regular files. Explicit files are read even if
ignored; `.git` and `.tiller` metadata are excluded. See the full
[ledger contract](/tiller/spec/artifact-schema/#baseline-ledger-designbaselineyml).

Init success is not readiness. `missing_contributors`, `missing_design`, `no_reference_root`, and
`invalid_code_root_mode` remain independent gaps. Build 2 adds `no_confirmed_baseline` for both
existing-code kinds until one ledger doc is confirmed, and `missing_design_doc` for missing ledger
prose in every kind. A ledger alone does not satisfy non-empty design. One confirmation releases
the startup baseline route, even when other docs remain unconfirmed; other readiness gaps still
need repair. With no tree, decompose is next only when a to-be design doc exists under
`.tiller/design/`; otherwise Tiller reports that no work is planned and names how work enters.
Build and test gate configuration
is not a readiness prerequisite.

After tracked work merges, `sync` names baseline docs whose described paths overlap the feature.
Run `tiller drift` to inspect candidates. Fresh projects enable `stale_design`; add
`shifted_design` to `drift.checks` when you also want dirty or untracked code changes detected.
Existing projects keep their checks list until you edit it. `mend-drift` proposes the prose
repair and stops for named-doc human confirmation. The console's design-file page shows the
confirmation, provenance, and the same drift facts; it does not approve anything.

## Register yourself, then join

Two deliberate acts, not one. Registration puts you on the project's committed roster. Joining says this checkout is you, and wires your coding agent while it's at it.

```sh
tiller add contributor jc --name "JC Grubbs"
tiller join --as jc
```

```
Registered jc (JC Grubbs).
Roster (1 contributor): jc (JC Grubbs)
```

```
Joined as jc (JC Grubbs). Identity recorded; roster now 1 contributor.
Project files kept current: orientation.md (generation 57).

**Wired the Claude Code harness**
  agent skills    .claude/skills/tiller-*    10 shims, gitignored
  pointer block   AGENTS.md
  redirect        CLAUDE.md

**Next**
  tiller status
  tiller guide      names the seam that fits
```

`join` writes a small managed block into `AGENTS.md` (plus a redirect in `CLAUDE.md`, which Claude Code auto-loads) and installs the ten current discovery shims. It also writes a gitignored `.tiller/identity.yml` selecting who you are in this checkout; a teammate who clones the repo runs their own `join`. Routine maintenance uses upgrade, not another join.

Init seeds `.gitignore` only if it is missing. Join adds missing Tiller-owned ignore groups,
including the local identity, the optional `.tiller/local.yml` preferences file, cache, skill shims, `/.bin/`, and `/artifacts/tiller/`, while leaving
your existing lines untouched. The evidence pattern does not hide unrelated artifacts; an older
broader ignore already in your file remains your repository's choice.

Commit what landed:

```sh
git add -A && git commit -m "tiller init"
```

## Ask tiller what's next

```sh
tiller status
tiller guide
```

```
Project: Acme Platform (acme) [greenfield]
Contributors:  jc
Epics 0  Features 0  Tasks 0  Questions 0
```

```
Up next for jc:
1. Agent: decompose. No epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/overview.md. `tiller guide decompose`

Suggested skill: decompose
The tree is undecomposed; lay a draft epic and feature tree for the human to approve. No epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/overview.md.
queue: 0 bugs, 0 chores, 0 tweaks ready
Run `tiller guide decompose` to load it.
```

`tiller guide` with no argument prints your ranked next moves, the same list `tiller next` prints, then names the skill for the top agent move. Your agent reads the same list, so the 2 of you always agree on what comes next. Here the design inputs copied in step 1 are what make decompose due; with nothing under `.tiller/design/` beyond as-is docs, guide reports that no work is planned instead.

## Hand it to your agent

Open your coding agent in the repository and ask it to decompose the project once `tiller check readiness` passes. Put new inputs under `.tiller/design/`; for existing in-repo docs use the move-or-pointer choice above. `init --design <dir>` imports only on the first scaffold, not now. The agent reads the block in `AGENTS.md`, then `orientation.md`, then `tiller guide decompose`, and lays the tree down as narrative through the CLI. For Acme that looked like this:

Ask for a product progression through the full known initial scope, not an exhaustive platform followed by its consumers. For example, documented invoice workflows might justify a small runnable foundation, issuing one invoice, collecting its payment, and only then bulk billing and reconciliation. Bound the first foundation to the first workflow's needs; keep later shared capabilities in the later increments rather than dropping them or prebuilding them. An epic is a delivery increment, not a permanent billing bucket. The compact Acme commands below illustrate the lifecycle; a real decomposition includes all known outcomes and explains its boundaries in the epic and feature narratives.

```sh
tiller add epic foundation --title "Foundation"
tiller add epic billing --title "Billing"
tiller add feature foundation/app-scaffold --title "App scaffold"
tiller add feature foundation/auth --title "Sign-in and sessions"
tiller add feature billing/invoices --title "Invoices"
```

```
Created epic foundation (E-mu0v91) in draft.
Created epic billing (E-s68hyp) in draft.
Created feature foundation/app-scaffold (F-dv5ppv) in draft.
Created feature foundation/auth (F-jl675z) in draft.
Created feature billing/invoices (F-9to7kv) in draft.
```

Everything lands in `draft`. An epic is a narrative (`epic.md`) and a state file (`epic.yml`), and a feature is the same pair. Nothing is specified yet, and that's on purpose. Features get specified one at a time, when their turn comes.

## Correct the proposal before approval

Review feedback stays at the gate it's already approaching. A title correction changes display metadata without editing the feature narrative or minting a new id:

```sh
tiller rename feature foundation/auth --title "Authentication and sessions"
```

```
Renamed feature foundation/auth (F-jl675z) display title from "Sign-in and sessions" to "Authentication and sessions".
```

Before the review, the agent audits actual execution prerequisites across the whole tree, then records the recommended sibling sequence with `tiller order`. Useful earlier outcomes must be honestly acceptable, not implicit mocks of later integrations. A small genuine prerequisite stays a hard edge; a preferred order is not one. Neither a subsystem name nor a cross-epic edge count establishes the right sequence.

Draft key moves, dependency edits, and removal are direct proposal corrections too. `tiller unlink ... --depends-on ...` removes a mistaken hard edge, and `tiller remove task ... --reason "duplicate proposal"` permanently removes a proposed task you don't want. Commit the corrected `.tiller/` diff and go back to the same epic or feature approval. An amendment at this point would add ceremony without adding history.

## Your first decision

The agent stops here, because approving the shape of the tree is your call and not its. `tiller queue` lists what needs you:

```
- approve_epic · billing (E-s68hyp) · Billing · `tiller approve epic billing`
- approve_epic · foundation (E-mu0v91) · Foundation · `tiller approve epic foundation`
```

```sh
tiller approve epic foundation
```

```
epic foundation (E-mu0v91) is now ready.

**Approval recorded**

**Verification**

- Moved to ready: 1 of 1
- Now legal on the node(s) just approved: cancel

**Needs attention**

- 1 epic awaits your approval · `tiller queue --kind approve_epic`

queue: 0 bugs, 0 chores, 0 tweaks ready

**Next:**
1. Agent: specify foundation/app-scaffold (F-dv5ppv). In your current epic. `tiller guide specify`
2. Agent: specify foundation/auth (F-jl675z). In your current epic. `tiller guide specify`
3. You: approve epic billing (E-s68hyp). You worked on it today; its decomposition awaits your approval. `tiller approve epic billing`
```

Every gate prints the same kind of block: what was recorded, what's now legal, what still needs attention, and your ranked next moves, yours and the agent's, best first. Approving Foundation made it your current epic, so its drafts lead in plan order. Adding work never makes it current; it only marks where you recently worked. The tree now reads:

```
acme
├─ billing (E-s68hyp)          draft  Billing
│  └─ invoices (F-9to7kv)      draft  Invoices
└─ foundation (E-mu0v91)       ready  Foundation
   ├─ app-scaffold (F-dv5ppv)  draft  App scaffold
   └─ auth (F-jl675z)          draft  Authentication and sessions
```

## See it

```sh
tiller serve
```

opens the console on localhost. The decisions that need you, what's in flight, what's next, and the whole tree to browse, all read live from the working tree. The pending epic approval is the first card on Home, with its command ready to copy.

<img class="dark:sl-hidden" src="/tiller/console-home-light.webp" width="1440" height="780" alt="The tiller console's Home view for the Acme project, light theme" />
<img class="light:sl-hidden" src="/tiller/console-home-dark.webp" width="1440" height="780" alt="The tiller console's Home view for the Acme project, dark theme" />

## Choose where review happens

The walkthrough above uses the default, `task`: each task owns its PR, and execute finishes after recording its review and publishing the submission-state commit. Review Grain can move that review surface upward without changing the task loop:

```sh
tiller configure feature foundation/app-scaffold --review-grain feature
# or, for every inheriting feature under an epic:
tiller configure epic foundation --review-grain epic
```

At feature grain, the agent creates a separate working branch for every task and ordinary-merges each submitted task tip, including its submission-state commit, into `tiller/reviews/features/<epic>/<feature>`. Epic grain uses `tiller/reviews/epics/<epic>`. Once every riding task is `in_review` or `done`, the agent assembles all current tips, runs feature gates, prepares acceptance, and maintains one aggregate PR. Requested changes return to the affected task branch, then reassemble and recheck the same PR. Independent overrides finish their own PR first. No cherry-pick or squash assembly; only the final integration merge is human-only.

Choose the grain before work starts. Tiller refuses a change that would reinterpret submitted work or move an already-started feature into or out of an epic branch. If you don't need a coarser review, the default task flow is the right place to start.

## The first task, end to end

From here the loop is short. The agent specifies the next feature and you approve its plan. The agent claims one task, whose first start permanently fixes its owner and activates the ready feature, works it on its own branch, runs gates, and submits the review. You merge. Tiller sees the merge and records done.

### Approve the initial plan

```sh
tiller spec feature foundation/app-scaffold            # the agent, after writing feature.md, feature.eval.md, and the initial tasks
tiller approve feature foundation/app-scaffold         # you: the plan gate
```

```
feature foundation/app-scaffold (F-dv5ppv) is now specced.
feature foundation/app-scaffold (F-dv5ppv) is now ready. 1 task(s) in the plan moved to ready with it.
```

### Extend the approved plan

Now the feature plan really is approved. One more task is additive work with its own task gate, not an amendment, and soft order can place it without creating another dependency:

```sh
tiller add task foundation/app-scaffold/document-local-setup --title "Document local setup"
# write the new proposed task.md, then record the preferred sequence
tiller order task foundation/app-scaffold/wire-the-build foundation/app-scaffold/document-local-setup
tiller approve task foundation/app-scaffold/document-local-setup
```

The new task stays `proposed` until that final human approval. `order` is a preference for rendering and recommendations; it never gates the tasks. Direct `link` and `unlink` refuse real dependency changes once the affected work is locked, with `dependency_change_locked`; the one exception is a started task adding (never removing) an edge to a done task. Amendment is not an escape hatch: it has no link, unlink, or order operation, and `revise` only re-pins prose. In-place edge edits on unchanged started work are unsupported. Genuine replacement may use cancellation/supersession and fresh eligible work with its own prerequisites; never manufacture replacement merely to evade a lock. A fresh proposed task under an active feature can still receive its own legal sibling prerequisites.

For a structural placement correction, first inspect installed help and affected node/descendant history, parents, dependencies, effective question holds, and review grain. Direct `rename --key` applies to a draft, task-free feature; changing parents additionally requires draft source and destination epics. Outside that window an eligible unstarted draft, specced, or ready feature uses `tiller amend <amendment> --move-feature <feature> --to <epic>/<key> --reason "<why>"`. Inspect and commit the proposal, then stop for human amendment approval. Started descendants refuse; do not replace unchanged work just to relocate it. A pure move preserves identity and acceptance history and does not approve a specced plan.

Features from different bounded epics may interleave. Projected epic ordering cycles are advisory, while actual prerequisite cycles, including explicit epic and mixed feature/epic completion deadlocks, remain blocking. Keep true prerequisites rather than changing them solely for a cleaner grouped map.

### Work and merge

The agent's execute instruction includes ordinary commits, pushes, PR creation or reuse, and
submission unless you explicitly say not to publish. Green checks are intermediate. If a session
stops at `check`, re-enter execute without another `start` or `resume`; bare guide prioritizes
owned checked tasks before other ready or in-progress tasks, while preserving owner and question
guards. Tree work stays ahead of shelves; shelves prefer checked work, bugs before chores before tweaks before tweaks within
each phase.

The initial Acme repository has no remote, so this example records a local review ref and you
review and merge the branch locally. No host account or push is needed:

```sh
tiller claim task foundation/app-scaffold/wire-the-build # the agent, as you; this task only
# Safely start from the configured integration branch; preserve unrelated work.
tiller start task foundation/app-scaffold/wire-the-build
git checkout -b tiller/foundation/app-scaffold/wire-the-build
# implement the task and stage only its changed source files
git commit -m "T-h389ih wire the build"
tiller check task foundation/app-scaffold/wire-the-build
git add .tiller && git commit -m "T-h389ih checks recorded"
tiller submit task foundation/app-scaffold/wire-the-build
git add .tiller && git commit -m "T-h389ih local review recorded"
tiller readout execute
# ... you review and merge the branch locally ...
tiller sync
```

```
Claimed foundation/app-scaffold/wire-the-build (T-h389ih) for jc.
task foundation/app-scaffold/wire-the-build (T-h389ih) is now in_progress. Work happens on branch tiller/foundation/app-scaffold/wire-the-build.
Checked task foundation/app-scaffold/wire-the-build (T-h389ih): all required gates green; state is now check.
task foundation/app-scaffold/wire-the-build (T-h389ih) is now in_review. No remote: recorded branch tiller/foundation/app-scaffold/wire-the-build at 8755651; review happens on the branch.
Sync: recorded 1 done crossing(s):
  - task foundation/app-scaffold/wire-the-build (T-h389ih) -> done
```

Notice what never happened. Nobody ran a command that marks something done, because there isn't one. `sync` saw the merge on `main` and recorded the crossing. The feature and the epic will complete the same way, by rollup over their children, once their own required gates have run.

With a configured remote, use the project's Git and host tools instead of the offline submission
above. After committing implementation and gate state, push the recorded branch. Reuse its open
PR or create the correct review with an explicit base and head, then pass its actual number to
`tiller submit task ... --pr <actual-number>`. Immediately commit and push the submission state
to that same branch, verify the remote contains it, and render the readout. Both pushes are the
agent's work. Tiller itself does not push or open PRs. `tiller guide execute` provides the complete
host choreography and coarse-grain variations.

The readout's **Execution status** includes all project tasks and uncarried shelf nodes in
`in_progress`, `check`, or `in_review`, not just your own. **Gates green with evidence** and
**Current submissions recorded** count that same population. The latter requires `in_review`
and the appropriate recorded PR or review ref; resumed work retaining an old PR does not count.
Checked work displays first, then in-progress, then in-review work. Next remains owner-scoped.
An empty execution population is not a successful handoff.

### Recover an aggregate review missing after merge

If a feature or epic PR finally merged before its Tiller review was recorded, do not return to the merged branch or create another PR. An absent record means sync has not inspected the aggregate's merge state, not that it knows a merge happened. Verify historical repository/PR/base/head/result identity with the project's host tools and obtain explicit authorization to recover that named review.

On configured integration with relevant source inputs committed, record the actual historical binding:

```sh
tiller submit feature <feature> --pr <actual-number> --recover-merged \
  --head <historical-reviewed-commit> --merge-commit <integration-result-commit> \
  --reason "<why the record was missing>"
```

Use `epic` for an epic-grain review; omit `--pr` only without a remote. Tiller validates local immutable Git evidence and labels the PR association caller-supplied. The record operation changes no status, gate, ownership, or acceptance. Commit its actual integration-state delta, run ordinary sync, inspect historical warnings and current acceptance separately, commit actual sync state, publish when authorized, and read back status/show/queue/readout. Exact retries write nothing. Deleted or advanced aggregate refs do not replace the historical anchors. Contained work without a merge witness still needs the human's existing fast-forward assertion.


## Share a feature without sharing ownership

Another registered developer may claim a different ready task under the same feature. Approval never assigns it to the approver or a sibling’s owner. Use `release task` or human-directed `reassign task <ref> --to <owner> --reason "<why>"` only before start. First start fixes ownership permanently, even after resume, review, cancellation, or completion. Features/epics display Contributors from started tasks, not an exclusive owner. Claimed ready work is an assignment, not yet contribution.

Any registered developer may perform authorized aggregate assembly/checks/submission at feature or epic grain. Coordinate one updater at a time through Git. Shared checks, acceptance, and publication appear once per reviewed unit; the last task finisher surfaces that step without becoming its owner. At task grain, after every task is done, complete feature acceptance and ordinary sync on integration.

For an exceptional started-work handoff, the human must explicitly name work and recipient:

```sh
tiller transfer task foundation/app-scaffold/wire-the-build --to sam --reason "Sam will complete the remaining implementation"
```

Use a real registered recipient and unfinished started work. The original is cancelled with its owner/history intact; the returned replacement is ready, unstarted, same-scope, and has a fresh ID/key/basis and no branch/review/green evidence. Start the returned ref as its recipient. Questions, pauses, dependencies, and carriers remain operative; started unfinished dependents or scope drift refuse before writes. Close an old task/shelf PR without merging and open a replacement; at coarse grain update the existing unmerged aggregate PR and rerun affected checks/acceptance. Git/host follow-through is separate from Tiller’s state transaction. Already-merged work requires sync and forward work instead.

## Capture a bug

Foundation's app scaffold is live now, and someone notices the form goes blank on a hard reload. That isn't a plan change. It's a defect against work already merged, so it goes on the bug shelf instead of back into the tree. The agent runs `tiller guide capture`, which loads the capture skill, then writes one complete proposed contract and stops before triage:

```sh
tiller add bug blank-on-reload --title "Form blanks on reload" --attach foundation/app-scaffold
# write the complete Reproduction, Expected, Actual, and Fixed when contract into bug.md
git add .tiller && git commit -m "capture: blank-on-reload"
```

```
Created bug bugs/blank-on-reload (B-4k2p9x) in proposed, attached to foundation/app-scaffold (F-dv5ppv).
```

Capture never approves, claims, or starts what it just wrote. Triage is yours:

```sh
tiller approve bug blank-on-reload
```

```
bug bugs/blank-on-reload (B-4k2p9x) is now ready.
```

With a remote configured, the agent works the bug on its own branch and PR. Here `origin` and
`main` represent the project's configured remote and integration branch:

```sh
tiller claim bug blank-on-reload           # the agent, as you
tiller start bug blank-on-reload
git checkout -b tiller/bugs/blank-on-reload
# fix the bug and stage only its implementation and regression-test paths
git commit -m "B-4k2p9x fix blank reload"
tiller check bug blank-on-reload
git add .tiller && git commit -m "B-4k2p9x checks recorded"
git push -u origin tiller/bugs/blank-on-reload
gh pr list --state open --base main --head tiller/bugs/blank-on-reload
# reuse an open matching PR; only when none exists:
gh pr create --base main --head tiller/bugs/blank-on-reload \
  --title "bugs/blank-on-reload (B-4k2p9x)" --body "Attached to foundation/app-scaffold (F-dv5ppv). Fix blank reload."
# use the actual PR number returned by the host
tiller submit bug blank-on-reload --pr <actual-number>
git add .tiller && git commit -m "B-4k2p9x in review"
git push origin tiller/bugs/blank-on-reload
tiller status bug blank-on-reload
git ls-remote origin refs/heads/tiller/bugs/blank-on-reload
# verify the remote includes the submitted state commit
tiller readout execute
# ... you review and merge the PR ...
tiller sync
```

```
Claimed bugs/blank-on-reload (B-4k2p9x) for jc.
bug bugs/blank-on-reload (B-4k2p9x) is now in_progress. Work happens on branch tiller/bugs/blank-on-reload.
Checked bug bugs/blank-on-reload (B-4k2p9x): all required gates green; state is now check.
bug bugs/blank-on-reload (B-4k2p9x) is now in_review. Recorded PR #51; review happens there.
Sync: recorded 1 done crossing(s):
  - bug bugs/blank-on-reload (B-4k2p9x) -> done
```

It's the same loop you just ran for a task (claim, start, check, submit, final integration merge, sync), addressed as `bug bugs/blank-on-reload` instead of `task foundation/app-scaffold/wire-the-build`. Both own a working branch; the shelf always owns its PR, while a task can ride a coarse aggregate review.

If PR creation succeeds but submission refuses, report the open PR and still-unsubmitted state
separately. If submission succeeds but its state push fails, keep `in_review` and identify the
unpublished commit. Never force-push or change remotes to bypass a refusal. Requested changes
resume the same reviewed work, rerun checks, reuse the PR, and publish the new submission state.
Ordinary publication needs no second go-ahead; genuine missing credentials, tools, or safe conflict
resolution can require your help. Approval, attestation, and merge remain your decisions.

The developer owns and is responsible for every change. Version 0.11.1 instructs agents
not to identify themselves, their model, provider, or harness as an author or contributor anywhere
in the work. That includes code, comments, docs, commits, PRs, and reports: no generated-by notices,
agent signatures, or agent co-author trailers. The rule preserves real human credits and required
third-party notices. It is guidance for the agent, not a CLI content filter.

Schema 9 renames the shared manifest roster from `owners:` to `contributors:` and checkout-local identity from `owner:` to `contributor:`. The schema-8-to-9 migration preserves contributor IDs and names, the selected local identity, task/bug/chore/tweak claims and lifecycle, branches, reviews, accepted bases, evidence, and historical event bytes. An absent identity stays absent. Schema 8 needs neither a work drain nor an ownership review; older migrations retain their existing requirements. New roster events are `contributor.add` and `contributor.remove`; historical `owner.add` and `owner.remove` events are not rewritten. There are no compatibility command, flag, environment, or current-schema key aliases. Acting identity uses `--contributor`, `TILLER_CONTRIBUTOR`, then local identity; execution/question `owner` fields and scalar ownership filters remain unchanged. New event actor attribution uses `contributor:`; historical events with `owner:` remain readable without rewriting their bytes.

## Reviewed ownership adoption

Schema-5 active work and open reviews can migrate while preserving branches, PRs, statuses, and evidence. Inspect without writes:

```sh
tiller migrate --dry-run --json
```

Save the whole JSON report or extracted `data.ownership_review` outside source state. A human fills every initially empty `confirmed_owner` for started unfinished tasks, bugs, chores, and tweaks, copying or correcting `observed_owner`. Keep source_schema, target_schema, snapshot, sources, IDs, and other fields unchanged. A stale/incomplete/duplicate/conflicting or unknown-owner review refuses before writes.

```sh
tiller migrate --ownership-review /path/to/review.json
# Or the same offline project maintenance via upgrade:
tiller upgrade --project-only --ownership-review /path/to/review.json
```

Pause team mutations and reconcile actual owners/state on integration first. Retain a compatible binary and committed recovery baseline. Apply one reviewed migration in a designated checkout, then commit/publish through Git. Upgrade everyone and incorporate the migration into every continuing work branch without discarding its task progress; do not independently migrate stale clones. Refresh each checkout, restart consoles, reload agent instructions, and check schema, identity, task owner, branch, and legal next action before resuming. A schema fence is local, not a distributed lock. After new work exists, recovery must preserve it rather than blindly restore old schema files. September 21 is an intended window, not authorization or an evidence waiver.

## Keep a checkout current

```sh
tiller upgrade                  # update the binary and this checkout's schema/instructions
tiller upgrade --project-only   # offline maintenance after a separate install or in another checkout
tiller upgrade --binary-only    # intentionally update only the binary
```

Normal upgrade also refreshes when the binary is already current. Project-only uses the running
binary without network or Homebrew and cannot select `--version`; it is mutually exclusive with
binary-only. Outside a project, normal upgrade updates only the binary and reports that scope.
Inside one, it selects the current resolved checkout, even from a subdirectory. It never discovers
or rewrites other repositories, branches, or worktrees.

Artifact refresh does not need identity and never changes it. A real schema migration needs a
registered actor through `--contributor`, `TILLER_CONTRIBUTOR`, or local identity. Missing checkout identity
still calls for onboarding with `tiller join --as <id>`, not an invented contributor.

Only when migrating schemas 2–4 across the schema-5 branch-model boundary, drain active old-schema features, unfinished aggregate reviews, and tasks
in `in_progress`, `check`, or `in_review` with a verified compatible binary, including human
acceptance/final merges and sync. Known blockers stop self-managed replacement. Upgrade never
finishes or cancels work to clear them, and never deletes or renames retained refs.

Before replacement, upgrade retains the outgoing build under
`~/.local/share/tiller/previous/<version>-<sha256>/tiller` and reports its path and directory.
A retained build is not automatically compatible. In a temporary drain shell, prepend the
verified compatible build's reported directory to PATH so gates invoking bare `tiller` use it too.
Leave that shell afterward and invoke the upgraded executable explicitly for project-only
maintenance. Never use an older binary to mutate a migrated checkout.

Homebrew remains the owner of verified cask or legacy formula installs. Upgrade delegates only
that Tiller package with implicit dependent checks and cleanup disabled; brew may still refresh
metadata or install genuine dependencies. Pins are unsupported for brew, and private download
authentication still applies. Brew can change the machine before the target's compatibility is
known; failures never fall back to a direct Cellar/Caskroom overwrite.

Binary replacement, migration, and generated refresh are separate durable steps. A failed later
step is nonzero and preserves completed steps; repair the named cause before project-only retry.
Recovery-required journals must be recovered first, never removed. Unknown state after a lost
child result is not proof that nothing changed. Review/commit the reported diff, restart existing
serve processes, and reload harness instructions as needed. Shims load full instructions through
`tiller guide`; do not hand-edit them or generated orientation.

**Upgrading an older binary:** a running binary cannot acquire new maintenance
logic merely because its executable is replaced. If its help lacks the flags
above, retain a verified compatible binary for old-schema work before installing
the newer one, then invoke the new executable's project-only command. The old
process provides no new preflight or retention guarantee.

## Where to next

- [Concepts](/tiller/concepts/) for the mental model behind all of this: the tree, the lifecycle and its gates, who does what, drift and amendments, and bugs, chores, and tweaks.
- [Skills](/tiller/skills/) for what the agent is actually reading.
- [The CLI reference](/tiller/cli/) for every command, and `tiller status <noun> <node>` any time you want to know what's legal from here.
- [The console](/tiller/console/)'s Explore tab, then Map, for a mind map of the whole project, epics down to bugs, chores, and tweaks, with the dependency graph drawn as arcs.
- [The console](/tiller/console/)'s Fix lens for the shelf-work board, 6 columns from Proposed to Closed.
