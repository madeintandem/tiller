# Code review: bugs and performance

Reviewed the source across core logic, CLI, web back-end, and web front-end with four focused reviewers and two independent verifiers. This was a whole-codebase review, not a working-tree diff review. Five findings were independently confirmed: four bugs and one performance optimization. The findings below describe the reviewed, pre-fix source; their line references belong to that snapshot.

P1 means high-priority correctness risk. P2 means a consequential defect or optimization that should be scheduled after P1.
## Remediation

All five findings are fixed in source. Four isolated worktrees supplied the changes, followed by two independent code reviews. Gate reuse now declines an identity when Git conversion could hide working-tree bytes, and a new identity domain invalidates older records. The policy includes clean/process filters literally named `unset` or `unspecified`. This conservatively reruns gates in conversion-enabled repositories rather than hashing the entire checkout.

Corpus walks always read and validate current confined bytes; the obsolete metadata byte cache is removed, while content-keyed parsing caches remain. Report supplies absolute `week_boundaries`, including the next Monday after the final week, and charts use those timestamps across timezone and daylight-saving changes. Rubric recording prints the actual criterion status. Amendment preconditions and epic prose are gathered only by their consuming readout moments.

Verification:

- The full Go run passed every package except a new VCS regression that incorrectly treated Git's stderr conversion warning as changed-path stdout. That assertion was corrected to inspect stdout separately. The complete VCS package then passed, and focused VCS/mutation regressions passed with the final filter-name correction.
- All 1,473 console tests passed. Static checks and the console production build passed; embedded assets were regenerated. The build retained its bundle-size advisory.
- The rebuilt CLI ran passing LF input, then rejected Git-hidden failing CRLF input without `--rerun`, including clean filters named `unset` and `unspecified`. Failed rubric recording printed `criterion status is fail` and exited 0.
- The production server returned current query/search results after metadata-preserving edits and refused same-length NUL-containing prose.
- Chromium rendered the production Report chart against a controlled completion response from a UTC server. In the observed America/Chicago browser timezone, the completion and median both rendered at the correct server-week coordinate. DST regressions cover 167-hour and 169-hour final weeks.
- A real orient readout with 20 revision guards dropped from 27 Git calls to 7, including a reduction from 20 guarded hashes to zero, with byte-identical output. One observed elapsed comparison was 0.806 s before versus 0.198 s after. Amendment readouts still reported all 20 stale preconditions.

No installation, release, schema, or orientation change was made. The remaining sections preserve the original review evidence and its verification limits, not the post-fix state.


## P1: Gate reuse can report a pass for currently failing code

Location: `internal/vcs/code_identity.go:86-92`. Consumer: `internal/core/mutate/gate_reuse.go:48-68`.

Code identity starts with HEAD's blobs and overlays only paths Git reports as changed. Git normalizes content before comparing it, so line-ending conversion and clean filters can hide differences in the actual working-tree bytes that gate commands execute.

An independent verifier enabled `core.autocrlf=true`, committed a tracked `probe.sh` containing `exit 0\n`, and configured a required feature gate as `sh probe.sh`. The first check passed. After rewriting the script to `exit 0\r\n`, Git reported no changed path, but running the script directly exited 255 with `numeric argument required`. Ordinary `tiller check` still exited 0 with `passed: true` and `reused: true`. Running the same check with `--rerun` exited 1 and reported failure.

Impact: a previous pass can certify code whose current executable input fails. This violates the code-identity contract that conversion-only differences must cost reuse, not grant it.

Recommendation: account for actual tracked working-tree bytes when check-in conversion can conceal differences. Disable reuse conservatively when equivalence cannot be established. `--rerun` is an immediate workaround.

## P2: The server's corpus cache can return stale content indefinitely

Location: `internal/corpus/corpus.go:150-153`. Cache identity: `internal/corpus/cache.go:44-48`.

A cache hit reuses prior document bytes when inode, size, mode, and modification time match. An in-place same-length edit that preserves modification time therefore bypasses the fresh read and displayability validation. The two-second exclusion when initially caching recent documents does not protect already-cached older documents.

An independent verifier exercised the built binary's production server with an aged document containing `F-aaaaaa`, then rewrote it to `F-bbbbbb` while preserving those metadata fields. In the same process:

- Relationship queries still returned a mention of `F-aaaaaa` with `complete: true`.
- Search returned the old document text and no match for `F-bbbbbb`.
- Direct prose reads and a fresh CLI query returned `F-bbbbbb`.
- Chromium displayed the stale search result, then the new content after opening it.

Replacing the file with same-length NUL-containing bytes also left relationship queries at HTTP 200, while direct prose reads correctly refused with HTTP 500.

Impact: search and relationships disagree with current files, and cached bytes conceal newly invalid prose. The explicit freshness contract in `docs/spec/cli-surface.md:1061` requires current confined reads and says file metadata is not a content identity.

Recommendation: read and validate current confined bytes on every walk, retaining the existing SHA-256-keyed parsing caches rather than metadata-only byte reuse.

## P2: Report completion points use the browser's timezone instead of the server's

Location: `ui/src/views/report/scale.ts:43-60`. Consumers: `ui/src/views/report/LeadCycleChart.tsx:45-46,64`.

The server builds weekly buckets in its own timezone but serializes date-only Monday labels. The browser reconstructs those dates as browser-local midnight. A network reader in another timezone can see feature completion dots and their rolling-median lines in the wrong server-defined week.

An independent verifier ran the production embedded chart in Chromium configured for `America/Los_Angeles`, injecting a controlled report response with UTC-server week boundaries and a completion at `2026-10-19T00:30:00Z`. The Oct 19 band began at x=815.5, but the completion dot and median rendered at x=805.499, inside the preceding week. The correct UTC-boundary coordinate was x=816.269.

Verification boundary: this exercised real browser rendering with an injected API-response fixture, not a ledger-generated completion. Throughput counts tasks and shelf items, so this finding does not claim an independently observed feature throughput discrepancy.

Recommendation: include absolute weekly boundary timestamps with offsets in the payload and use them for instant positioning. Keep date-only labels for display. Treating all dates as UTC would still fail for non-UTC servers.

## P2: Recording a failing rubric incorrectly announces human attestation

Location: `internal/cli/eval.go:196-198`.

The human confirmation unconditionally says `now needs human attestation` whenever rubric output is recorded. A rubric verdict of `fail` actually stores a failing criterion and evals gate.

An independent verifier recorded a required eval-bound criterion with rubric schema `tiller.rubric_output.v1` and verdict `fail`. The command printed the attestation message, while `tiller show feature ... --json` immediately reported both the criterion and evals gate as `fail`.

Impact: ordinary output conceals failed acceptance and describes the wrong remaining action. Exit 0 is correct for successful recording and is not the defect.

Recommendation: render the recorded criterion's actual status, distinguishing failure from a passing rubric awaiting attestation.

## P2: Unrelated readouts perform unused amendment validation

Location: `internal/cli/readout.go:190-191`. Additional load cost: `internal/core/mutate/amendment_validity.go:118-128`.

Every full readout gathers proposed-amendment preconditions, although only the amendment readout consumes them. The gather reloads amendment input, resolves operations, hashes revision-precondition files, and performs a fresh project load for each staged feature move.

An independent verifier staged 20 revise operations and traced Git subprocesses during actual `tiller readout orient --json` invocations:

| Fixture | Git invocations | Hash invocations | Observed elapsed time |
|---|---:|---:|---:|
| With revision guards | 27 | 20 | 0.300 s |
| Same operations without guards | 7 | 0 | 0.084 s |

Stdout was byte-for-byte identical. The fixture used `core.autocrlf=true`, which makes those guarded hash reads invoke Git. These timings are one observed comparison, not a statistical benchmark; the invocation counts establish avoidable work. Fresh project loads for move operations were established statically, not runtime-counted.

Recommendation: gather amendment preconditions only for `MomentAmend`, matching the existing execute-only gather pattern nearby. Preserve live validation in amendment readouts; the verifier confirmed that `readout amend` reports stale preconditions.

## Verification and limits

The parent built the current CLI successfully and exercised `status`, `next`, and the production server's status, tree, next, report, and HTML routes against a disposable initialized project. The two independent verifiers reproduced all five findings through real commands, HTTP requests, browser rendering, or measured invocation traces as described above. Review servers and browser sessions were stopped.

No full test suite or lint run was performed. No application code, permanent tests, project state, installation, or release was changed. This was a targeted review of four broad surfaces, not a proof that other defects are absent. Documented deferred features and unsupported speculative issues were excluded.
