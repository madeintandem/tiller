# Tiller

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./tiller-logo-inverted.svg" />
  <img src="./tiller-logo.svg" alt="Tiller boat and wave logo" width="240" />
</picture>

[![CI](https://github.com/madeintandem/tiller/actions/workflows/ci.yml/badge.svg)](https://github.com/madeintandem/tiller/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

Tiller is an opinionated SDLC workflow tool for humans and AI coding agents
working together. It holds the structure, the gates, and the state of a
software project. The agent does the reasoning. The human approves at the
gates. Everything it knows lives as files in your repo, under git, next to
the code it describes.

![The tiller console showing an Acme Platform project: three epics, their features and tasks, and what needs attention](docs/assets/console-home.png)

## The opinions

**Files are the source of truth.** Markdown for prose, YAML for state, git as
the spine. There's no database and no hosted service. `.tiller/` is a
directory in your repository, so a clone carries the whole plan with it, and
nothing about that plan lives anywhere a clone can't reach.

**Prose and state never share a file.** A node's `.md` is yours: intent and
acceptance criteria, in your words. Its `.yml` is Tiller's: status,
ownership, and gate results, mutated by the machine. That split is what lets
the CLI churn state all day without touching a sentence a human wrote, and
it's what keeps 2 people's work from colliding in a merge.

**One tree, progressively elaborated.** Epics, then features, then tasks. An
epic is narrative. A feature is narrative plus a specification and an
acceptance contract. A task carries the implementation detail. Features get
specified and broken into tasks just in time, when their turn comes, and not
a moment before.

**Tiller is a gatekeeper, not an orchestrator.** It declares and records gates
(build, tests, evals, review). It never runs the agent's reasoning, and it
never drives someone else's tooling on the agent's behalf. Required gates
block done. Optional review gates are visible and non-blocking, which is to
say they're encouragement with a paper trail.

**The done gate is the PR merge.** An agent can drive work right up to the
edge of done and turn every gate green. Only a human merge crosses to done,
and Tiller learns about it by watching git, not by being told.

**Model and harness agnostic.** The agent surface is the `tiller` CLI, a set
of skills served by `tiller guide`, and a generated `orientation.md` in the
repo. Claude Code, Codex, and a bare shell with a local model all see the
same thing, because nothing in Tiller depends on a capability only one vendor
has. (Today `tiller join` auto-wires Claude Code. Another harness adapter is a
small addition, and the bare-shell path works without one.)

**Tiller guides; it does not control.** It can't make an agent follow the
process and it doesn't pretend it can. The developer drives, the developer
answers for the result, and the one boundary that actually holds is the merge
a human performs. The name isn't an accident. A tiller steers the boat, but
somebody's hand has to be on it.

## Install

The latest published release is **v0.18.4**. The
[release Quickstart](https://madeintandem.github.io/tiller/quickstart/) is
prepared for 0.21.2, which is not yet published. Current source is **0.21.2**; the
[source walkthrough](https://madeintandem.github.io/tiller/source-quickstart/)
covers the same contract in more detail from a source build. While this repository is private, release access
requires authentication and anonymous installation is unavailable.

macOS, with Homebrew:

```sh
brew install --cask madeintandem/tap/tiller
```

macOS or Linux, with the one-shot installer. It fetches the right binary for
your platform and verifies its checksum before placing it:

```sh
curl -fsSL https://raw.githubusercontent.com/madeintandem/tiller/main/install.sh | sh
```

Go users can `go install github.com/madeintandem/tiller/cmd/tiller@latest`.
Checksummed binaries for macOS and Linux, amd64 and arm64, are on the
[releases page](https://github.com/madeintandem/tiller/releases). Tiller is a
single static binary with the web console and the skills embedded, so there's
no runtime to install and nothing else to place.

Distributed third-party components retain their upstream licenses. Full
licenses and copyright notices are in [THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt),
included in release archives and served by the embedded console at
`/THIRD_PARTY_NOTICES.txt`. Tiller's own code is licensed under Apache 2.0.

## Your first tree

```sh
cd your-project
tiller init --name "Acme Platform" --key acme --kind brownfield
tiller add contributor jc --name "JC Grubbs"   # register yourself on the committed roster
tiller join --as jc                       # select that identity here; wire your coding agent
tiller status
```

`init` scaffolds `.tiller/` and generates `.tiller/orientation.md`, the
agent's front door. `join` writes a small managed block into your `AGENTS.md`
and, for Claude Code today, installs the skills where the harness discovers
them; any other harness reaches the same skills through `tiller guide`.
For the brownfield setup above, ask your agent to run `tiller guide`: it selects
`derive-baseline` so existing docs and code can be reconciled and the baseline
reviewed by you before decomposition. After readiness passes, the decompose
skill proposes an epic and feature tree for you to approve. A greenfield project
can start with decomposition once its design inputs and roster are ready. Then:

```sh
tiller serve
```

and watch the tree appear. A few features in, `tiller tree --status` reads
something like this:

```
acme
├─ billing (E-3o8jq7)               ready          Billing
│  ├─ invoices (F-bh4xuq)           draft          Invoices
│  └─ refunds (F-a7aee9)            draft          Refunds
├─ foundation (E-2hr4la)            ready, active  Foundation
│  ├─ app-scaffold (F-uos81n)       active         App scaffold
│  │  ├─ health-route (T-xbxeb7)    in_progress    Add the health route
│  │  └─ wire-the-build (T-ndie06)  done           Wire the build
│  └─ auth (F-5tvbkv)               draft          Sign-in and sessions
└─ intake (E-jx0go5)                draft          Customer intake
   └─ form-engine (F-6nyiz9)        draft          Form engine
```

## How a piece of work moves

**Decompose.** The agent proposes epics and features as narrative, and they
land as `draft`. A human approves the epic gate (`tiller approve epic`),
which is an approval of the tree's shape, not its details.

**Specify.** When a feature's turn comes, the agent writes its specification
(`feature.md`), its acceptance contract (`feature.eval.md`, criteria with
bindings Tiller can run), and its tasks, then asserts the whole thing with
`tiller spec feature`. A human approves the plan gate (`tiller approve
feature`). That moves the feature and its initial tasks to `ready` and pins
each one's spec as the basis it was approved against. Edit the spec after
that and Tiller calls it drift until a human re-records the basis.

**Execute.** A human (or their agent, acting as them) claims an individual task.
First start permanently fixes its owner and atomically activates its ready feature.
Every task has its own recorded branch. At task grain it has a PR; feature and epic grains use a separate
aggregate branch and one PR. The agent creates branches, ordinary-merges submitted
task tips and state into the aggregate, resolves mechanical conflicts, reruns
gates, and maintains that PR. No cherry-pick or squash assembly, and no routine
branch work for the developer. Human acceptance and the final merge into the
configured integration branch remain the gates. `tiller sync` observes that
merge and records done; features and epics roll up over their children.

Different engineers can execute different tasks in the same feature. Features and
epics show Contributors derived from started tasks, not an exclusive owner.
Before start, release or reassign a reservation; after start, ownership never changes,
even through review, cancellation, or completion. Any registered developer can
perform authorized aggregate checks and submission. Shared closeout appears once
per reviewed unit, not once per contributor; coordinate one aggregate updater at a time.

An exceptional handoff of started unfinished work is a human decision:
`tiller transfer task|bug|chore|tweak <ref> --to <registered-contributor> --reason "<why>"`.
It preserves the original's attribution and creates an approved, unstarted same-scope
replacement with fresh identity and no inherited review or green evidence.
The agent may run it only on explicit instruction naming work and recipient.

## Choosing work

Choose a **Bug** for a defect: observed behavior violates an agreed expectation. Choose a **Chore** for housekeeping without code side effects, such as documentation or administrative upkeep. Choose a **Tweak** for a minor adjustment that may change code. Choose a **Task** under a feature for substantive planned work. These are choice guidelines, not file-diff restrictions: Tiller does not enforce a no-code rule for Chores.

For example, changing a completed button from “Save” to “Save changes” to improve the wording is a Tweak, even when the label lives in application code. If the button text violates its agreed requirement, record a Bug instead. A new button or a broader interaction redesign belongs in planned Task work.

Keep changes forward-only. Never reopen or rewrite done work to express a new request. Add a new Bug, Chore, Tweak, or Task with attachment or source context as appropriate; the completed record stays history. Adding planned descendants may recompute feature and epic rollups, but it does not reopen merge-backed done execution nodes.

Tweaks are first-class shelf work at `.tiller/tweaks/<key>/tweak.md` and `tweak.yml`, with `TW-` IDs. They use the same ownership, approval, gates, branch, review, carrier, and completion rules as Chores. The console’s **Fix** board combines Bugs, Chores, and Tweaks; it is not a separate Tweak board.
Old task/shelf PR closure or coarse aggregate updates remain ordinary Git/host work.

**Change.** Mid-flight pivots are forward work. Nobody edits history. An
amendment stages operations against the plan (revise, cancel, supersede,
add), a human approves it, and done work stays done.

Throughout, `tiller status` says where things stand, `tiller guide` names
the skill that fits the current state, and `tiller serve` shows the whole
thing to the human in the loop. Tiller never branches, pushes, or merges.
Git stays yours, and so does the hand on the tiller.

## Where things live

```
.tiller/
  tiller.yml            project, contributors roster, gates, schema version
  orientation.md        generated; the agent's front door (committed, do not hand-edit)
  epics/<key>/
    epic.md  epic.yml   narrative (yours) and state (Tiller's)
    features/<key>/
      feature.md  feature.eval.md  feature.yml
      tasks/<key>/
        task.md  task.yml
```

Identity is per checkout. `tiller join` writes a gitignored
`.tiller/identity.yml` saying who you are here, and the committed `contributors`
roster in `tiller.yml` says who may hold work. There's no global config, no
user config, and no machine default anywhere. Clone the repo and you have the
whole plan; run `join` and you have a seat at it.

Schema 8→9 is a terminology migration: the committed roster becomes `contributors:` and local identity becomes `contributor:`, preserving IDs, selected identity, work claims/lifecycle, branches, reviews, evidence, and historical events. It needs no work drain or ownership review. Older migration requirements below remain unchanged. Acting identity uses `--contributor` or `TILLER_CONTRIBUTOR`; task/bug/chore/tweak and question `owner` fields and ownership filters remain unchanged. New event actor attribution uses `contributor:`; historical events with `owner:` remain readable without rewriting their bytes.

Schema 6 adoption needs reviewed active owners. Run `tiller migrate --dry-run --json`,
save its report outside source state, and have a human fill every `confirmed_owner`
in `data.ownership_review`. Apply with `tiller migrate --ownership-review <file>` or
`tiller upgrade --project-only --ownership-review <file>`. Schema-5 active work keeps
its branches, reviews, and evidence; schemas 2–4 still require the older branch-model
drain. Pause team state mutations, reconcile owners on integration, migrate once,
publish through Git, then upgrade and incorporate the migration into every continuing
work branch. Refresh each checkout, restart consoles, and reload agent instructions.
A local schema fence is not a distributed lock. The intended September 21 window
does not authorize installation, publication, or a managed-project migration.

## Status

Tiller is pre-1.0 and in active use on real projects. v1 targets the
execution half of a project, from "we have specs and a plan" through "it's
merged and done." The longer aim is continuous coverage from discovery to
production. The schema may change between minor versions until 1.0, and a
binary that meets a tree it can't safely write stops and says so instead of
guessing. [docs/plans/STATUS.md](./docs/plans/STATUS.md) is the honest
one-page answer to what exists today.

## Learn more

- The docs site: [madeintandem.github.io/tiller](https://madeintandem.github.io/tiller/)
- Current reference specs: [`docs/spec/`](./docs/spec/) (the artifact schema,
  CLI surface, skill set, console, and distribution; unbuilt designs are labeled)
- Important decisions and completed work: [Project history](./docs/project-history.md)
- For contributors and coding agents: [`AGENTS.md`](./AGENTS.md), then
  [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Every command the binary exposes: [`docs/tiller-commands.md`](./docs/tiller-commands.md),
  or `tiller --help`

Tiller is built by [Made In Tandem](https://madeintandem.com), a software
consultancy in Chicago, and it's how we run human-plus-AI delivery on our own
client work. Licensed under [Apache 2.0](./LICENSE); see [NOTICE](./NOTICE)
for attribution and [TRADEMARKS](./TRADEMARKS) for use of the Tiller name and
logo.
