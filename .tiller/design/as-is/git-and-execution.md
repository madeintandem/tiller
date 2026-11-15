# Git observation and execution reads

This is a pointer page. No single document owns this surface. The contract is spread over `docs/spec/cli-surface.md`, `docs/spec/artifact-schema.md`, `docs/spec/ui-surface.md` ("Task execution sources"), `SECURITY.md` ("Scope notes"), and one paragraph of `CONTRIBUTING.md`. This page says which sections apply to which package, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0).

## What this surface is

Three packages that let Tiller look at Git without ever changing it.

`internal/vcs` is the read-only window onto the checkout's Git. Its `Git` type runs `git` in a selected directory and implements two interfaces: `Revisions`, the ordinary surface the gate, sync, and drift code depends on (branch, HEAD, committed and working-tree blob and tree hashes, the working-tree fingerprint, the code identity, ancestry, first-parent containment, merge base, patch id and patch digest), and `HistoricalRevisions`, bounded reads of immutable commits and blobs. Around it sit `HeadSnapshot` (one pinned inventory of HEAD for a sweep), `WorktreeSurface` and `CheckWorktreeSurface` (current bytes under a described path), `BaselineSource` (a source document's blob at an observed commit), and helpers for remotes, upstream tips, worktrees, local branches, and migration input checks. It is used by `internal/cli`, `internal/core/mutate`, `internal/core/drift`, `internal/core/review`, and `internal/serve`.

`internal/execution` supplies request-local, read-only observations of task execution: what a started task's own branch or worktree records about status, owner, gates, evidence, and review, laid over the served tree without changing it. `Load` takes a loaded tree and returns a `Snapshot`. It is used by `internal/serve` (the console's `/api/execution`, node history, and evidence) and by `internal/readinput`, which is how the CLI's ranked next list honors work in flight elsewhere. It observes tasks only, never bugs, chores, or tweaks.

`internal/testgit` is test support: fixture repositories for the test suites. Nothing outside `_test.go` files imports it.

The boundary on all three: Tiller never creates a branch, commits, pushes, merges, or moves a ref (`docs/spec/cli-surface.md`, "What the CLI is", and "Decisions settled so far"). Branch names, review refs, and the decision that a merge happened are recorded by the lifecycle code, which is another page's subject; this surface supplies the observations. The execution-source contract is owned by `docs/spec/ui-surface.md`, "Task execution sources"; the CLI's use of it is `docs/spec/cli-surface.md`, "Ranked next" (the "Observed execution" paragraph) and "Observing merges and reading holds: sync, status, ls" (the paragraph on local-working-tree views).

## Data it owns

None of it is persisted here. The packages return values that other code stores: `RevisionRef` and `SurfaceFingerprint` from `internal/vcs`, defined by `internal/store`. See `docs/spec/artifact-schema.md`, "Revisions", for the shape.

What this surface does own is the meaning of several stored values, and their framing is a compatibility contract. The working-tree fingerprint is hashed under the domain string `tiller-worktree-fingerprint-v2`, and the code identity under `tiller-code-identity-v3`; changing either retires every value recorded before. Their definitions are `docs/spec/artifact-schema.md`, "Gates": "The stored gate block" (the `against` fingerprint), "Gate reuse", and "Code identity". The described-path inventory and its hashing are in the same document, "Baseline ledger: `design/baseline.yml`", the "Working-tree inventory" paragraph. The patch digest that confirms a squash merge is defined only in code (`patchDigest` in `internal/vcs/git.go`): hunk ranges and index lines are dropped, except a binary file's index line, and every other line is hashed with its whitespace intact.

In memory: a `HeadSnapshot` holds one HEAD inventory for the life of one sweep. An execution `Snapshot` holds the base tree, a display copy with observed task values, the observations, diagnostics, and the sources it read, for one request. `internal/execution` keeps one process-wide cache of commit tree inventories (64 commits, 250,000 entries, cleared on overflow), which matters only to the long-running console. A `testgit.Template` keeps an in-memory copy of a laid fixture per Git environment for the life of the test process. The observation record the console serves (`Record`: id, key, branch, source, revision, observed time, uncommitted, freshness, status, owner, integration status) is specified in `docs/spec/ui-surface.md`, "Task execution sources".

## Behavior to preserve

Read-only, without exception. `internal/vcs` writes no object, ref, index, or stash, and the status reads pass `--no-optional-locks` so a read never refreshes the caller's index (`docs/spec/cli-surface.md`, "Reading nodes: show, ls, status", last paragraph; `docs/spec/artifact-schema.md`, "Code identity").

Routing of inherited `GIT_*` variables. Tiller observes the checkout Git discovers from the project directory. `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, `GIT_INDEX_FILE`, and `GIT_OBJECT_DIRECTORY` are checked against that checkout and then removed before Git runs; one that names anything else refuses `git_routing_conflict`, exit `3`. The rule is `docs/spec/cli-surface.md`, "Output and exit codes" (the paragraph beginning "Git is observed for that same selection"), and `SECURITY.md`, "Scope notes". The code is `internal/vcs/routing.go`. `internal/execution` is stricter and drops every `GIT_`-prefixed variable (`gitCommandContext` in `internal/execution/git.go`).

Revisions and code identity. A gate's `against` is the HEAD commit on a clean tree and a `sha256` fingerprint on a dirty one; an unreadable changed path refuses `worktree_unreadable` and is never read as deleted; the code identity is the same whether files are committed or dirty, excludes this project's own `.tiller/` and `artifacts/tiller/` (another project's state in the same repository stays in), and is absent (so nothing is reused) under check-in conversion, hidden index flags, `core.filemode` false, gitlinks, or nested repositories. See `docs/spec/cli-surface.md`, "Gates, evals, and attestation" (the `check` paragraphs on `against` and reuse), and `docs/spec/artifact-schema.md`, "The stored gate block", "Gate reuse", and "Code identity". Working-tree blob comparison for basis drift applies check-in conversion and clean filters without writing: `docs/spec/artifact-schema.md`, "Drift control hooks" (`stale_spec`).

Merge and crossing inputs. Sync needs containment plus a tip off integration's first-parent line, then a content fallback that pairs a patch id with a whitespace-faithful digest, and explicit `refs/heads/` resolution so a tag cannot stand in for a branch. See `docs/spec/cli-surface.md`, "The done crossing", and "Recovering an aggregate review recorded after merge" for the historical reads. The primitives are `IsMergedInto`, `FirstParentContains`, `MergeBase`, `PatchID`, `PatchDigest`, `FirstParentPatchIDs`, `RefCommit`, and `BranchRef`; the fifty-commit scan limit and the decision itself live in `internal/core/mutate`. A ref that does not resolve answers "not merged" or an empty value from `RefCommit`, `MergeBase`, `PatchDigest`, `FirstParentContains`, `RefExists`, and the branch side of `IsMergedInto`. It is an environment error (`git_unavailable`) from `PatchID`, `FirstParentPatchIDs`, and an unresolved `into` passed to `IsMergedInto`. Read from the code, not run.

Worktrees and branches. Reads only: which worktree holds a branch, every worktree's path, the project's own directory in each worktree, local branch names, and whether a local tip is ahead of its upstream. The commands that use them are `docs/spec/cli-surface.md`, "The work loop" (branch recording and the unpublished-state warning), "Gates, evals, and attestation" (`attest_branch_mismatch`), "The done crossing" (the `not_contained` recovery steps), and "Capturing agent usage: usage" (project paths per worktree).

Baseline and historical reads. `--from` provenance and described-path fingerprints: `docs/spec/cli-surface.md`, "Design baseline commands", and `docs/spec/artifact-schema.md`, "Baseline ledger: `design/baseline.yml`". Historical file reads accept regular files only, cap a file at 4 MiB and a tree listing at 16 MiB, and refuse symlinks and gitlinks; no document states those two caps.

Execution observations. Source choice (worktree, then local branch, then tracking tip), the identity and accepted-basis match, the diagnostics that replace an observation, the newest-ownership-event rule, confinement, and the one stability check per request: `docs/spec/ui-surface.md`, "Task execution sources". Observations never change eligibility, and a failed observation degrades to the served tree alone (`docs/spec/cli-surface.md`, "Ranked next"). Git reads here also set `GIT_OPTIONAL_LOCKS=0` and disable terminal prompts, replacement objects, lazy fetch, and the fsmonitor.

The test harness. `Init` creates a repository on branch `main` with a fixed identity and background maintenance off; `Run` and `Env` strip the routing variables so a suite started from a Git hook builds its fixtures where it names them; `Template`, `Templates`, and `CopyWithResult` lay a fixture once and copy it per test, and a lay must not call `t.Setenv` or `t.Chdir`. The only document is `CONTRIBUTING.md`, "How we work" (the "Fast tests" item); the rest is the package's own comments.

## Edges

`internal/vcs` depends on the `git` executable on `PATH`, on `internal/store` for `RevisionRef`, `SurfaceFingerprint`, and `EnvError`, and on `internal/rootpath` for symlink-refusing file reads. Its failures cross the boundary as `git_unavailable`, `git_routing_conflict`, and `worktree_unreadable` (all exit `3`), `migration_git_conflicts` and `migration_inputs_dirty` (exit `1`), or a typed `NotVersionedError` the caller interprets. The gate and sync code takes the `Revisions` interface, not `*Git`, so its tests can stub Git; the few reads outside that interface (remotes, upstream tips, worktrees) are called on the concrete type.

`internal/execution` depends on the `git` executable, `internal/store`, `internal/core/events`, `internal/gen`, `internal/id`, `internal/rootpath`, and `yaml.v3`. It does not use `internal/vcs`; it has its own Git runner. It reaches no network: it reads local refs, worktrees, and tracking tips as last fetched.

`internal/testgit` depends on the `git` executable and the standard `testing` package only.

No external service is involved. A PR host is never queried (`docs/spec/cli-surface.md`, "The done crossing").

Files under the described paths that this page does not otherwise name: `internal/vcs/commit_reads.go` (commit-pinned path and file reads, outside the historical read caps), `internal/vcs/index_reads.go` (the tracked-path inventory), `internal/execution/fork.go` (forking a snapshot), and `internal/execution/concurrent.go` (at most four git processes per observation). `internal/execution` also caps a batched blob at 8 MiB. No source states these limits.

## Described paths

- `internal/vcs`
- `internal/execution`
- `internal/testgit`

## Discrepancies and gaps

A stale package comment. `internal/vcs/git.go` opens by saying the package is the only one that shells to the git binary. `internal/execution/git.go` runs Git on its own (`gitCommandContext`), and `internal/testgit/testgit.go` does for tests. The specs do not make that claim; `docs/spec/cli-surface.md`, "Output and exit codes", already speaks of the console's separate, stricter reads.

Undocumented hardening in `internal/vcs`. Every Git process it starts also runs with `LC_ALL=C`, `GIT_NO_REPLACE_OBJECTS=1`, and `GIT_NO_LAZY_FETCH=1` (`command` in `internal/vcs/routing.go`), so replacement refs cannot rewrite evidence and a read never fetches. `docs/spec/cli-surface.md` says other inherited Git settings pass through and mentions neither; `docs/spec/ui-surface.md` states the lazy-fetch rule for console reads only.

No owning document. Nothing describes `internal/vcs` or `internal/testgit` as a package: the `Revisions` and `HistoricalRevisions` interfaces, the patch digest's normalization, the historical read caps, and the fixture template rules exist only in code comments.

Not built, and already on the roadmap: completion fingerprints for submodules (`docs/roadmap.md`, "Not built yet", Baseline coverage). Today a gitlink or nested repository removes the code identity instead.

What was checked: `routing.go`, `code_identity.go`, and `worktree_fingerprint.go` in full against the spec text; the `Revisions` interface and the doc comments of every `Git` method; `baseline_source.go`, `migration.go`, `project_worktrees.go`, and `index_reads.go` in full; execution's `Load`, source selection, candidate validation, stability checks, and Git runner; all of `testgit.go` and the `Template` contract. Not checked line by line: `worktree_surface.go` hashing, `historical.go` beyond its limits and interface, execution's event and index reading, and any test file.
