---
title: Quickstart
description: Complete a scratch project with the v0.21.4 release.
---

This walkthrough is for **v0.21.4**. It uses schema **12**, orientation generation
**58**, and **eleven skills**. Tasks are claimed and started one at a time, each on
its own branch, and maintenance runs through `tiller upgrade`. The longer
[source walkthrough](/tiller/source-quickstart/) covers the same contract in more
detail, including existing code, review grain, bugs, and ownership transfer.

## Install and check the version

Choose one installation path:

```sh
brew install --cask madeintandem/tap/tiller        # macOS
# Or, on macOS or Linux:
curl -fsSL https://raw.githubusercontent.com/madeintandem/tiller/main/install.sh | sh
tiller --version
```

Verify that the installed version is **0.21.4** before following this page. The
commands and output below match that release.

## Initialize a disposable project

Git needs your name and email configured for the commits below.

```sh
mkdir acme-tiller-scratch
cd acme-tiller-scratch
git init -b main
tiller init --name "Acme Platform" --key acme --kind greenfield
tiller add contributor robin --name "Robin Hale"
tiller join --as robin
mkdir -p .tiller/design
printf '# Acme\n\nA small application with a health route.\n' > .tiller/design/overview.md
git add -A
git commit -m "Initialize Acme planning"
tiller check readiness
tiller guide
```

Use your own contributor ID and full name. Registration is committed; `join` selects
that identity for this checkout and installs eleven gitignored skill shims for
Claude Code. A greenfield project with design inputs and a contributor starts at
decomposition, so `tiller guide` names the `decompose` skill. For an existing
repository, use `--kind brownfield` and follow `tiller guide derive-baseline`:
existing docs and code need a reviewed baseline before decomposition.

## Propose the tree, then take the human gate

The agent writes the epic and feature narratives after reading the design
inputs. These commands create their state and prose files:

```sh
tiller add epic foundation --title "Foundation"
tiller add feature foundation/app-scaffold --title "App scaffold"
```

Replace the generated stubs in `.tiller/epics/foundation/epic.md` and
`.tiller/epics/foundation/features/app-scaffold/feature.md`, then commit the
proposal. The human reviews it and approves the tree's shape:

```sh
git add .tiller
git commit -m "Propose the Acme tree"
tiller approve epic foundation
```

## Specify the feature

The agent writes the specification into `feature.md`, then adds the first task:

```sh
tiller add task foundation/app-scaffold/wire-the-build --title "Wire the build"
tiller show feature foundation/app-scaffold --json
```

Use the returned feature ID in `feature.eval.md` beside `feature.md`. This
scratch contract checks the concrete health file that the task will create:

```yaml
---
node: F-xxxxxx # Replace with the actual feature ID, not the key.
criteria:
  - id: C1
    statement: The health file reports ok
    polarity: introduce
    binding: { type: command, run: "test \"$(cat health.txt)\" = ok" }
---
```

Replace the stub in `tasks/wire-the-build/task.md` with the approach: write
`health.txt` containing `ok`, verified by the feature contract. Then assert the
specification. The human reviews the spec, task, and contract and approves the
plan, which moves the task to ready with the feature:

```sh
tiller spec feature foundation/app-scaffold
tiller approve feature foundation/app-scaffold
git add .tiller
git commit -m "Approve the health-file plan"
```

## Execute, review, merge, and sync

Claim the **task**, start it from `main`, and work on its own branch. The first
start fixes the task's owner and activates the ready feature:

```sh
tiller claim task foundation/app-scaffold/wire-the-build
tiller start task foundation/app-scaffold/wire-the-build
git checkout -b tiller/foundation/app-scaffold/wire-the-build
printf 'ok\n' > health.txt
git add health.txt .tiller
git commit -m "Implement the health file"
tiller check task foundation/app-scaffold/wire-the-build
git add .tiller
git commit -m "Record task checks"
tiller submit task foundation/app-scaffold/wire-the-build
git add .tiller
git commit -m "Record the local review"
tiller readout execute
```

With no remote, `submit` records the branch and head commit as a local review.
This scratch project wires no build or tests gate, so the readout lists the
task's required checks as incomplete. Its **Needs attention** line, `4 review
gates await your attestation`, counts the four optional review gates (code,
security, performance, accessibility); `tiller queue --kind attest_gate` lists
them with the `tiller attest` command for each, and they never block the merge.
The human reviews `health.txt` and the recorded state, then performs the merge.
The agent finishes feature acceptance on `main`:

```sh
git checkout main
git merge --no-ff tiller/foundation/app-scaffold/wire-the-build -m "Merge the health-file task"
tiller sync
git add .tiller
git commit -m "Record the task merge"
tiller check feature foundation/app-scaffold
git add .tiller
git commit -m "Record feature acceptance"
tiller sync
git add .tiller
git commit -m "Record the rollup"
tiller status
tiller tree --status
```

The first `sync` observes the merge and records the task's done crossing, then
names the feature check that remains. `check feature` runs the contract's C1.
The second `sync` records the feature and epic as done by rollup. Neither the
agent nor a command directly marks work done.

## Open the embedded console

```sh
tiller serve
```

Open the printed loopback URL if your browser does not open automatically. If
port 4400 is in use, pass `--port` with a free port. Inspect Home, Explore, and
the completed task. Stop the server with Ctrl-C. The console and skills are
embedded; no Node runtime is needed to run them.

## Keep a checkout current

`tiller upgrade` updates the binary through its installation owner, then
migrates and refreshes the current checkout. To maintain a checkout with the
binary you already have, without network access:

```sh
tiller upgrade --help
tiller upgrade --project-only
```

Review and commit any changed project files, then restart running `tiller serve`
processes and reload your agent's instructions. Each other checkout needs its
own `tiller upgrade --project-only`. `join` selects identity; it is not the
maintenance command.

Started work keeps its owner. Before start, `tiller release` and
`tiller reassign` move a claim. After start, only a human-directed
`tiller transfer` hands the work to another registered contributor; see
`tiller transfer --help`.

See [Concepts](/tiller/concepts/) for terminology and `tiller guide execute` for
the remote PR and aggregate-review workflow.
