---
name: verify
description: Build the tiller binary and drive it end-to-end against a scratch project to observe the CLI and embedded console at their real surfaces. Use when verifying changes to CLI, serve, views, search, UI, gen, store, or core packages before committing.
---

# Verifying Tiller at the CLI and console surfaces

If `ui/src` changed, rebuild the committed assets first:

```
make ui
```

Build the binary from its real entrypoint:

```
go build -o /tmp/tiller-smoke/tiller ./cmd/tiller
export PATH="/tmp/tiller-smoke:$PATH"
```

Scaffold a throwaway project somewhere disposable (never in this repo, and
never in `~/dev/tiller-test-run`, which is the human's real dry-run repo):

```
rm -rf /tmp/tiller-smoke/p && mkdir -p /tmp/tiller-smoke/p && cd /tmp/tiller-smoke/p
git init -q -b main && git config user.email s@t && git config user.name S
tiller init --name "Smoke App" --key smoke --kind greenfield
tiller add contributor robin --name "Robin Hale"
tiller join --as robin
git add -A && git commit -qm init
```

## The shortest arc that exercises most human surfaces

**Decompose, then take the epic gate.** Epics and features land in `draft` as
narrative; the epic gate approves the tree, not the acceptance criteria.

```
tiller add epic foundation --title Foundation --contributor robin
tiller add feature foundation/app-scaffold --title "App scaffold" --contributor robin
tiller approve epic foundation --contributor robin
```

**Specify the feature.** Stage 2 is one act with one output: the specification,
the acceptance contract, and the task explosion, each task a folder with its own
`task.md`. Write all of it, then assert it.

```
F=.tiller/epics/foundation/features/app-scaffold
FID=$(tiller show feature foundation/app-scaffold --json | grep -om1 'F-[a-z0-9]\{6\}')

cat > $F/feature.md <<'EOF'
# App scaffold

## Summary

Stand up the application skeleton: the build, the entrypoint, and one smoke
route, so every later feature has somewhere to land.
EOF

cat > $F/feature.eval.md <<EOF
---
node: $FID
criteria:
  - id: C1
    statement: The app builds and serves its health route
    polarity: introduce
    binding: { type: command, run: "true" }
---

# Acceptance
EOF

tiller add task foundation/app-scaffold/wire-the-build --title "Wire the build" --contributor robin

cat > $F/tasks/wire-the-build/task.md <<'EOF'
# Wire the build

## Approach

Add the build target and the entrypoint, then prove it with the health route.
EOF

tiller spec feature foundation/app-scaffold --contributor robin
```

**Take the plan gate, claim one task, and start it.** Feature approval readies the
initial tasks without assigning them. An unclaimed task refuses start. Claiming
that task changes no sibling or ancestor owner. Its successful start permanently
fixes its owner and atomically activates the ready feature. Features and epics
show Contributors derived from started tasks, never an exclusive claim.

```
tiller approve feature foundation/app-scaffold --contributor robin
tiller claim task foundation/app-scaffold/wire-the-build --contributor robin
tiller start task foundation/app-scaffold/wire-the-build --contributor robin
git checkout -q -b tiller/foundation/app-scaffold/wire-the-build
echo app > app.txt
git add -A && git commit -qm "implement the build-wiring task"
tiller check task foundation/app-scaffold/wire-the-build --contributor robin
tiller check feature foundation/app-scaffold --contributor robin
```

**Merge on a branch, then let sync observe it.** Tiller never branches, pushes,
or merges. Submit from the branch the work sits on so the review ref names it.

```
# Already on the task's recorded branch, with implementation committed and checked.
tiller submit task foundation/app-scaffold/wire-the-build --contributor robin
git add -A && git commit -qm "record task submission"
git checkout -q main
git merge --no-ff -m "merge build-wiring task branch" tiller/foundation/app-scaffold/wire-the-build
tiller sync --contributor robin
```

`sync` should record three crossings, in this order: the task on `ancestry` (the
merge it observed), then the feature and the epic on `rollup`. Nothing above a
task crosses by ceremony, so a feature or epic that reached done before this
point is a defect, not a shortcut.

For ownership changes, add a second registered developer and another task under
the same feature; claim/start each as its own developer at task, feature, and
epic grain. Exercise pre-start release/reassignment, then post-start and terminal
refusals. A third registered actor can check/submit an eligible aggregate without
owning its tasks; applicable gates, inclusion of review heads/current task tips,
and human acceptance still apply. Coordinate aggregate updates through Git.

For an explicitly authorized transfer use
`tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`.
Read the returned replacement ref; verify original attribution and history remain,
the replacement is ready/unstarted with fresh identity/basis and no review/evidence,
and held/dependent/carrier relationships remain truthful. Close an old task/shelf
PR without merging or update the same unmerged aggregate PR, then rerun affected
checks/acceptance. Tiller itself changes no refs or host PRs. Do not simulate a
transfer by editing the `owner` field of started work.

Build legacy-schema fixtures with a compatible old binary, not malformed
current-schema YAML. For schema-5 active work run `tiller migrate --dry-run --json`,
save its whole report or `data.ownership_review` outside source state, have the
scenario's human fill every `confirmed_owner`, then use
`tiller migrate --ownership-review <file>` or
`tiller upgrade --project-only --ownership-review <file>`. Inspection writes
nothing; stale/incomplete review refuses. Preserve active branches/PRs/evidence.
Only schema-2/3/4 crossings into schema 5 retain the shared-branch drain requirement.

**Read it back.**

```
tiller status; tiller tree --status; tiller ls tasks
tiller status task foundation/app-scaffold/wire-the-build
tiller readout execute; tiller guide
tiller status --json
```

## The console

For `serve`, views, search, or UI changes, keep the scratch project populated and
start the server in one terminal:

```
tiller serve --port 18765
```

From another terminal, verify the embedded runtime before stopping the server:

```
curl --fail --silent --show-error http://127.0.0.1:18765/api/status
curl --fail --silent --show-error http://127.0.0.1:18765/api/tree
curl --fail --silent --show-error http://127.0.0.1:18765/
```

When browser automation is available, open `http://127.0.0.1:18765/` and exercise Home, Explore browse/filter/search, and one Node Detail link. Without a browser, report console rendering as unverified rather than treating the HTTP checks as browser proof. Stop the server with Ctrl-C.

Gotchas:

- `spec feature` refuses until `feature.md` has content, `feature.eval.md`
  carries at least one criterion, and at least one task folder holds a
  `task.md` that is more than the stub `add task` wrote.
- The two refusals of `spec feature` mean different things, and the boundary is
  not where you would guess. **Exit 1 is "not written yet"**: the eval file is
  missing, is header-only, or its front matter does not parse into a criteria
  list at all, which includes genuinely broken YAML. Tiller reads all of those
  the same way and says `write feature.eval.md`. **Exit 2 is "written but not a
  valid contract"**: the front matter parsed and carried criteria, but the
  contract is wrong, as in `criteria: []`, an unrecognized binding type, or a
  criterion missing the field its binding requires. So broken YAML gets you
  exit 1, not exit 2; that is the tool declining to guess, not a regression.
- A task is addressed by its three-segment key,
  `<epic-key>/<feature-key>/<task-key>`, or by its `T-` id. `add task` needs
  `--title`; there is no `--intent`.
- The initial explosion rides the feature's plan gate. Only a task added after
  that gate lands `proposed` and needs its own `tiller approve task`.
- Editing `feature.md`, `feature.eval.md`, or an approved `task.md` afterwards
  is drift: `start`, `check`, and `submit` refuse with `active_basis` until
  `approve feature <key> --refresh-basis` (or `approve task <id>
  --refresh-basis`) re-records it.
- Blocked business answers (exit 1) print on stderr; capture both streams.
- Project-state mutations after onboarding need `--contributor` or a prior `join`; `init` and `join` establish the repo and checkout identity and are the exceptions.
- `--json` payloads are a stable contract: id and key ride as separate
  structured fields, never composed labels; check them when human output
  changes.
