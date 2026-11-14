# Test fixtures outside internal/vcs spawn git with the inherited environment

Imported from [GitHub issue 18](https://github.com/madeintandem/tiller/issues/18).

## What

Switch the test helpers that spawn fixture git with the inherited environment to `testgit.Env()` or `testgit.Run`. About 22 test files use their own helpers: in `internal/cli`, `internal/core/drift`, `internal/core/mutate`, `internal/core/review`, `internal/e2e`, `internal/execution`, `internal/serve`, and `internal/views`.

Decide in the same change whether the suite should pass when started from a routed environment. Today the production calls refuse with `git_routing_conflict` in that case, which is safe but fails the run. Passing would need an environment scrub at test start; the repository has no `TestMain` today.

## Why

Since v0.24.0, `internal/vcs` refuses inherited Git routing (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`) that names another checkout, and fixtures built through `internal/testgit` drop those variables. The helpers above do not. When the suite is started from a routed environment, such as `git rebase --exec 'make test'` in a linked worktree or a commit hook, their fixture commands can act on the developer's real checkout instead of the temporary directory.

## Done when

- With `GIT_DIR` and `GIT_WORK_TREE` pointed at a scratch repository, running the full Go suite leaves that repository byte-for-byte unchanged.
- No test helper outside `internal/vcs` and `internal/testgit` starts git with an unfiltered inherited environment.
- The decision on passing under a routed environment is recorded in `CONTRIBUTING.md`.
