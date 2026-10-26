# Tiller bug hunt, 2026-10-26

Read-only review of the current working tree across core logic, storage and Git observations, CLI, console/server, embedded skills, generated orientation, and distribution. No implementation fixes, permanent tests, configuration changes, commits, or issues were made in this repository. Reproductions mutated disposable projects only.

The reviewed binary reports **0.21.3**, schema **12**, orientation generation **57**. Findings below describe existing behavior, not which change introduced it. Deferred roadmap work is not treated as a defect.

## Findings at a glance

| ID | Severity | Surface | Confirmed defect |
|---|---|---|---|
| R1 | Blocker | Gates / Git | Hidden byte and executable-mode changes can reuse a failing command's old pass |
| R2 | Important | Skills | Proposal choreography stages unrelated human planning work |
| R3 | Important | CLI | Approval ignores unsupported review-grain flags while applying mutations |
| R4 | Important | Shelf lifecycle | Submission accepts the wrong branch and records contradictory review identity |
| R5 | Important | Shelf lifecycle | Start freezes ownership before detecting an uncreatable working branch |
| R6 | Important | Provenance | Unborn-repository fingerprints omit already-indexed files |
| R7 | Important | Store | A symlink at an individual node directory silently removes the node from reads |
| R8 | Important | HTTP server | Node detail bypasses the direct prose reader's 8 MiB limit |
| R9 | Important | Console | Same-page Build navigation loses the remembered epic selection |
| R10 | Important | Skills / orientation | Guidance incorrectly says checks erase required human sign-off |
| R11 | Important | Shelf lifecycle | Independent start succeeds while a live carrier hides and controls the work |
| R12 | Important | Routing / review reads | Historical offline task-grain reviews are misclassified as coarse riders |
| R13 | Minor | CLI | An explicitly empty feature selector bypasses approval scope checks |
| R14 | Minor | CLI | Accepted plural amendment alias omits the batch's review details |
| R15 | Minor | Orientation | Generated instructions prohibit a supported feature-level epic grain |

Severity: Blocker means incorrect verification that should be fixed before release. Important means a demonstrated reliability, state-integrity, or workflow failure. Minor means a narrower command-contract or guidance defect. No authentication bypass, outside-checkout read, or Markdown XSS was confirmed.

## R1. Blocker: gate reuse can report passing code that currently fails

**Source:** `internal/vcs/code_identity.go:97-104`, with the fast-path safety predicate at `:162-210`. Consumers include `internal/core/mutate/gate_reuse.go:48-70`, standalone eval, and resume.

`CodeIdentity` starts with HEAD blobs and overlays only paths reported by `git diff HEAD`. Git can suppress real executable-input changes through index flags or mode configuration. Those omissions become an identical reuse key, so the check skips the required command and retains its old passing result.

**Verified twice, independently.** In a committed scratch project, configure a required task-grade command gate running an executable `./probe.sh`, initially exiting 0. Approve, claim, start, and check a bug. Then:

```sh
git update-index --assume-unchanged probe.sh
# Replace probe.sh with the same shebang, a diagnostic echo, and exit 7.
tiller check bug sample --json
tiller check bug sample --rerun --json
```

Normal check exited **0**, with `passed:true`, gate `status:pass,reused:true`. Forced execution exited **1**, with `passed:false,status:fail`; captured output was `changed-probe-fails`.

The independent mode reproduction restored the passing executable, cleared assume-unchanged, set `git config core.filemode false`, and recorded a real pass. After `chmod 644 probe.sh`, normal check again reused a pass; `--rerun` failed with `sh: ./probe.sh: Permission denied`.

**Impact:** The ordinary required-check path gives a false green for a command that fails on the current working tree. These are tracked-file/mode changes, not the documented ignored-file, environment, or network blind spots.

**Recommendation:** Disable reuse when index flags or mode suppression make the HEAD shortcut unsafe, or explicitly observe affected working-tree bytes and modes. Reuse migration's existing hidden-index detection where appropriate; do not change the user's index flags. Retire unsafe identity values so previously recorded false passes cannot remain reusable. Skip-worktree was not separately exercised.

## R2. Important: proposal instructions stage unrelated human work

**Source:** `internal/skills/capture/SKILL.md:54`, `internal/skills/amend/SKILL.md:161`, `internal/skills/intake/SKILL.md:113`. Matching examples appear in `docs/spec/skill-set.md:377,476,539`. Execute also contains broad staging examples despite its narrower prose instructions.

The shipped guides prescribe `git add .tiller && git commit ...`. This includes existing human edits and other proposals, not only the current command's outputs. An unrestricted following commit would also include unrelated paths already in the index.

**Verified:** In an onboarded scratch project with a committed scaffold, create `.tiller/design/unrelated-human-draft.md`, then:

```sh
tiller add bug captured --title "Captured fixture bug"
git add .tiller
git diff --cached --name-only
```

The staged set contained the new bug's prose/state, its event, **and the unrelated human draft**. `tiller guide capture --json` returned the same broad staging instruction. No proposal commit was made in this probe; the observed failure is the unauthorized staging itself.

**Impact:** Following the provided choreography can sweep unfinished human planning, accepted-contract edits, or concurrent work into an unrelated proposal commit and publication.

**Recommendation:** Stage exact reviewed command-output and authored proposal paths, inspect the existing index, and isolate the commit from unrelated staged work. Follow the narrower existing derive-baseline/code-review guidance. Align all broad staging examples and the owning skill specification.

## R3. Important: approval ignores unsupported review-grain flags

**Source:** `internal/cli/lifecycle_cmds.go:385-392,411-438`; `internal/cli/amendment_cmds.go:1121-1134`; shelf dispatch into `internal/core/mutate/transition_workitem.go`.

Amendment approval returns before the review-grain validation block. Its dedicated flag rejection omits `--review-grain`. The ordinary validator rejects tasks but not shelf nouns, whose mutation implementation ignores the field too.

**Verified amendment reproduction:** In a committed scratch project with task `e/f/t` and open amendment `reshape`:

```sh
tiller amend reshape --cancel e/f/t --reason "Scratch cancellation" --json
tiller approve amendment reshape --review-grain definitely-invalid --json
tiller show task e/f/t --json
```

All commands exited **0**. Approval reported `status:approved` and an applied `cancel` operation; the task was **cancelled**. The invalid grain was neither rejected nor recorded.

**Verified shelf reproduction:** `tiller approve bug ignored-grain --review-grain definitely-invalid --json` also exited **0**, reporting `changed:true,status:ready` for the proposed bug.

**Impact:** Malformed instructions succeed while crossing a human gate or applying a whole amendment batch, including cancellations.

**Recommendation:** Validate flag presence and allowed nouns before any dispatch or mutation. `--review-grain` should reach only epic/feature approval; amendment and shelf approval should reject it, regardless of value. Use the existing presence-based validation convention rather than silently dropping inputs.

## R4. Important: shelf submission accepts contradictory review branches

**Source:** `internal/core/mutate/transition_workitem.go:65-69,89-116`. Contrast task submission at `internal/core/mutate/transition.go:797-820` and the existing `guardSubmitBranch` in `task_branches.go:217-240`.

Shelf submission checks basis, claim, and required gate statuses, but never checks that the current branch and supplied review ref match the branch recorded at start.

**Verified:** With no remote and the scratch checkout still on `main`:

```sh
tiller add bug wrong-branch --title "Wrong branch review"
tiller approve bug wrong-branch
tiller claim bug wrong-branch
tiller start bug wrong-branch
tiller check bug wrong-branch
tiller submit bug wrong-branch --json
tiller sync --json
```

Submit exited **0** and persisted:

```json
{
  "branch": "tiller/bugs/wrong-branch",
  "review_ref": { "branch": "main", "head": "git:9cfffb2" },
  "status": "in_review"
}
```

Sync then reported the recorded shelf branch missing, while ranked guidance said the bug was in review on **main**, waiting for its merge. A task submitted from the same wrong branch correctly refused with exit **1**, `reason:branch_mismatch`.

**Impact:** A successful submission records incompatible sources of truth. The work is stranded and the recovery guidance directs the human to the wrong review branch. Bug, chore, and tweak share this implementation; the runtime reproduction used a bug.

**Recommendation:** Apply the existing branch guard before shelf submission and derive the review head from the verified recorded branch. Preserve the refusal's no-write boundary.

## R5. Important: shelf start freezes work onto an uncreatable branch

**Source:** `internal/core/mutate/transition_workitem.go:97-102`. Contrast task start's `guardNewBranch` call at `internal/core/mutate/transition.go:784-787`.

Shelf start records `started:true` and its immutable working branch without validating existing refs, prefix collisions, or the configured branch pattern.

**Verified:** In a scratch project with the default pattern:

```sh
git branch tiller/bugs
tiller add bug prefix-collision --title "Prefix collision"
tiller approve bug prefix-collision
tiller claim bug prefix-collision
tiller start bug prefix-collision --json
git switch -c tiller/bugs/prefix-collision
tiller release bug prefix-collision --json
```

Start exited **0**, recording `in_progress` and `tiller/bugs/prefix-collision`. Git branch creation then exited **128** because `refs/heads/tiller/bugs` already existed. Release refused with `owner_immutable` because start had already frozen ownership.

**Impact:** The tool successfully crosses an irreversible start boundary before discovering that its prescribed branch cannot exist. Recovery now requires handling historical refs or replacing work, instead of correcting an unstarted proposal.

**Recommendation:** Run the existing new-branch safety guard before applying start and freezing ownership. Do not delete or rename conflicting Git refs automatically. The exercised noun was bug; the same shelf function serves chore and tweak.

## R6. Important: unborn fingerprints omit indexed code

**Source:** `internal/vcs/worktree_fingerprint.go:62-81`. The existing unborn handling in `code_identity.go:107-128` demonstrates the missing inventory step.

When HEAD is unborn, fingerprinting skips `diff HEAD` and reads only untracked paths. Files staged for the first commit are neither, so they disappear from the observed tree identity.

**Verified:** Initialize and join a scratch project without making a commit, then create two questions. Stage `code.txt` containing one implementation and answer the first question. Change the file to a different implementation, stage everything again, and answer the second question.

Both answers exited **0** and persisted the same `answer_revision`:

```text
sha256:806f4caa7f9d19421f3e6923aaa2cab7ca6801527e41be41928712951de0763e
```

The two observation times differed; both fully staged code states were different. With every nonignored file indexed, the digest reduces to the empty-HEAD header rather than the code under observation.

**Impact:** Decisions and dirty-tree gate provenance can claim the same observed tree for distinct states before the first commit. This is a provenance defect, not a second gate-reuse defect; `CodeIdentity` handles indexed unborn files separately.

**Recommendation:** Include indexed paths when HEAD is unborn and read their current working-tree identities, following the existing CodeIdentity pattern. Preserve the read-only index/object/ref contract.

## R7. Important: symlink node homes silently hide work

**Source:** `internal/store/workitem.go:222-225`. Analogous directory filters exist in `internal/store/tree.go:289-291,328-330,379-381`.

The inventory skips every entry for which `IsDir()` is false. A symlink to a node directory therefore disappears before the confined opener can reject it.

**Verified:** Move an existing active/check-state bug directory outside the scratch checkout and replace its original path with a symlink to that directory. `tiller status --json` exited **0**, reported **zero bugs** and no human/agent moves. As a control, a symlink at the shelf root `.tiller/bugs` correctly refused with an unsafe-anchored-path error. Restoring the real node directory restored the bug count.

**Impact:** Tree-backed status and routing present an incomplete ledger as successful, with no unsafe-state warning. This requires a filesystem obstruction, not a normal CLI-created state. No symlink target bytes were read or written, and no outside-checkout escape is claimed. Epic/feature/task analogues were source-inspected, not separately reproduced.

**Recommendation:** Reject symlink entries explicitly before the ordinary non-directory filter in known node inventories. Keep skipping unrelated ordinary files such as shelf `order.yml`; align mutation inventories with reads.

## R8. Important: node detail bypasses the served-file size limit

**Source:** `internal/serve/node.go:70-73,87-88,432-437`; unbounded reads at `internal/corpus/corpus.go:142,185`. Contrast the bounded direct reader at `internal/serve/prose.go:266-280` and the 8 MiB policy in `SECURITY.md:57-65`.

Node detail loads corpus relationships and reads node prose without the direct prose reader's size bound.

**Verified against the real server:** Write a regular UTF-8 bug document of **9,437,188 bytes** in a disposable project, then request both routes:

```text
GET /api/prose?path=.tiller/bugs/oversized/bug.md&line=1
  413: file is larger than the console serves (8 MiB)
GET /api/node/bug/bugs/oversized
  200: 9,437,824-byte JSON response containing the full prose
```

The fixture's original small prose was restored after the probe.

**Impact:** An ordinary node read consumes and returns material the dedicated reader refuses. Relationship enrichment also performs whole-corpus reads. This is a confirmed resource-limit inconsistency; OOM, timing degradation, and remote exploitability were not demonstrated. The oversized file must already exist in the trusted served checkout.

**Recommendation:** Apply bounded reads before HTTP node/corpus enrichment parses or serializes contents. Surface explicit oversized/unavailable or incomplete diagnostics. Keep CLI corpus policy separate if its unbounded behavior is intentional.

## R9. Important: clicking Build on Build loses the saved epic selection

**Source:** `ui/src/views/Build.tsx:119-140`; emitting nav link `ui/src/App.tsx:102`. Fix's analogous arrival handling is at `ui/src/views/WorkItems.tsx:67-85`.

Build restores storage once per component mount. The primary nav goes to bare `/build`; clicking it while Build is mounted removes the explicit selection without remounting or restoring the remembered value.

**Browser-verified:** In a project with a remembered selected epic but no active epics:

1. Open `/build?epics=billing`: one feature card; storage contains `billing`.
2. Click **Build** in primary navigation: URL becomes `/build`; storage still contains `billing`, but the board shows **No epics selected** and zero cards.
3. Reload that same bare URL: it restores `/build?epics=billing` and the feature card.

The parent independently reproduced the same transition with the completed `foundation` epic in another fixture. This is shipped embedded UI behavior, not a mocked component result.

**Impact:** Ordinary primary navigation discards the current planning lens. When active defaults differ from the saved selection it can instead display a different set of features. Navigation and reload disagree for the same URL.

**Recommendation:** Restore per bare-route arrival, using the existing location-key pattern from Fix, while retaining explicit URL precedence and the explicit-empty `?epics=` behavior.

## R10. Important: guidance invents a repeated human sign-off requirement

**Source:** `internal/skills/execute/SKILL.md:97,306`; generated text at `internal/gen/orientation.go:373-376`; matching statement at `docs/spec/skill-set.md:337`.

These instructions say `check` resets a required attestation-backed gate to pending and direct another human attestation after checks. Current `check` preserves an applicable, same-kind settled attestation through `settledAttestation` in `internal/core/mutate/check.go:367-371,476-484`.

**Verified:** In a scratch bug with only a required task-grade attestation gate:

```yaml
signoff:
  kind: attestation
  on: [task]
```

After approval, claim, start, and a fixture-driver human `attest ... --gate signoff --status pass`, `tiller check bug captured --json` exited **0**, with `passed:true,status:check` and signoff `status:pass`. The original human contributor, timestamp, and attestation remained unchanged. No build or test command ran in this probe.

**Impact:** Agents consuming shipped instructions can falsely report erased acceptance or stop a passing workflow for redundant human action. The mismatch obscures the real pending/failing/stale-on-resume cases.

**Recommendation:** Describe settled-attestation retention and inspect returned gate status before asking for human action. Update execute, generated orientation, and the owning spec together. Preserve the prohibition on agent attestation and genuine stale/pending/failing stops. No check behavior change is recommended.

## R11. Important: independently started carried work disappears from execution

**Source:** `internal/core/mutate/transition_workitem.go:51-63,97-102`; consumers at `internal/gen/workitem.go:34-44`, `internal/views/readout_execute.go:17-20`, and `internal/core/mutate/sync_workitem.go:64-74`.

Shelf start never checks whether a live `resolved_by` carrier already owns completion. It creates independent execution state that the read and completion paths deliberately treat as carried rather than independently executable.

**Verified using legal CLI commands:** Create a draft feature `carrier/live`, then:

```sh
tiller add chore carried-work --title "Carried independent start"
tiller approve chore carried-work
tiller claim chore carried-work
tiller resolve chore carried-work --by carrier/live
tiller start chore carried-work --json
tiller status chore carried-work --json
tiller readout execute
```

Start exited **0**, recording `in_progress`, `started:true`, and `tiller/chores/carried-work`, without clearing the carrier. Status simultaneously reported `in_progress` and `activity:carried`. Execute omitted the newly started chore entirely. The reviewer reproduced the same failure on a transferred bug successor; the parent independently reproduced the fresh-chore sequence.

**Impact:** The mutation creates owned, irreversible independent work that continuation surfaces suppress. Source tracing establishes that sync prioritizes the carrier and, when it becomes done, crosses the shelf node as carried without observing its independent review/merge. That eventual carrier-completion consequence was not runtime-exercised in this probe.

**Recommendation:** Refuse independent start while the existing live-carrier predicate holds, naming deliberate `resolve --clear` as the path to independent work. Preserve cancelled/archived/missing-carrier fallback behavior. Do not fix only the readout by exposing a lane whose completion still belongs to another node.

## R12. Important: read models misclassify historical offline task-grain reviews

**Sources:** Eligibility exceptions at `internal/gen/preference.go:76-77` and `internal/views/route_claimable.go:35-36`; review ownership/rider classification at `internal/workflow/recovery.go:145-149,166-170`.

A task can retain `started_grain:task` after its parent acquires a coarser grain through ordinary history. Its original independent review remains authoritative. Offline tasks have `review_ref`, not `PR`; several read classifiers mistake that combination for a coarse rider.

**Verified with schema-valid persisted historical fixtures and real Git/CLI/HTTP, not mocks:** A coarse-grain feature has an independently submitted prerequisite `T-aaaaaa` carrying `started_grain:task`, and ready owned dependent `T-bbbbbb`.

- `tiller next --json` exited **0** and ranked the dependent first, instructing the agent to assemble submitted prerequisites onto the aggregate before starting.
- `tiller start task T-bbbbbb --json` correctly exited **1**, with `task_dependency_unfinished` and `dependency_started_grain:task`. Independent task-grain prerequisites must merge and sync; aggregate assembly is not the legal alternative.
- `status task`, `show task`, `ls task`, and `status feature` falsely reported `review_missing:true` or instructed submission/recovery of the owning aggregate, despite the task's existing independent review ref.
- The real `/api/node/task/T-aaaaaa` response returned its own branch/head review ref and `review_missing:true` together. This reproduced when serving the task checkout directly as well as through integration's observation, so it is not attributed to an execution-overlay omission.

**Fixture qualification:** These states were hand-authored under disposable projects to represent the explicitly supported historical-grain timeline. They passed the current loader and exercised real command and HTTP consumers, but were not produced by an end-to-end cross-branch configuration merge in this review.

**Impact:** Ranked advice prioritizes an impossible move over the human merge that actually unblocks it. Related status/console advice invents a missing aggregate review and sends users into the wrong submission/recovery workflow. These are two demonstrated manifestations of the same original-grain classification omission.

**Recommendation:** Respect the prerequisite's recorded task grain in both eligibility copies, and exclude historical task-grain reviews from aggregate ownership and submitted-rider classification. Reuse the existing original-grain resolution convention. Do not relax the correct start guard or absorb independent reviews into an aggregate.

## R13. Minor: an empty feature selector is treated as absent

**Source:** `internal/cli/lifecycle_cmds.go:460-482`.

The feature-selector branch checks its value, not whether the flag was supplied. An empty shell expansion skips task-only validation and positional-node exclusivity.

**Verified:** `tiller approve bug selector-bug --feature "" --json` exited **0**, moving the proposed bug to **ready**; subsequent status confirmed it. The invocation should reject the unsupported selector before any transition. The existing `--reviewed-assumption` code immediately above handles this same presence-versus-value distinction correctly.

**Recommendation:** Check `cmd.Flags().Changed("feature")`, reject blank/whitespace addresses, then enforce task-only and positional exclusivity before single-node or batch mutation. Classified Minor because the observed positional bug was explicitly named; no wrong-target mutation was demonstrated.

## R14. Minor: plural amendment show omits the review batch

**Source:** `internal/cli/show.go:52-66`, with noun canonicalization occurring later at `:81-93`.

**Verified:** Both `tiller show amendment reshape --json` and `tiller show amendments reshape --json` exited **0**. Singular included the base revision and staged/applied cancel operation; plural returned only the generic amendment identity/title/status. Plural human output likewise omitted the operations. The discrepancy also occurred while the amendment was still proposed.

**Impact:** An accepted alias can hide the very batch a human or JSON consumer is reviewing, while presenting a successful full-state read.

**Recommendation:** Canonicalize the noun before specialized show dispatch, routing both spellings through the dedicated amendment renderer. Do not silently accept the alias with an incomplete projection.

## R15. Minor: orientation prohibits supported explicit epic grain on a feature

**Source:** `internal/gen/orientation.go:632-633`.

Generated orientation says a feature cannot itself say `epic` and must inherit it. The CLI and resolver support an explicit feature-level epic override.

**Verified:** `tiller configure feature sample/first --review-grain epic` exited **0**, reporting `epic (feature explicit)`. `internal/store/reviewgrain.go:21-26,105-108` accepts and resolves the field, and the CLI specification permits all three values at each home.

**Impact:** Guidance can incorrectly refuse an authorized operation or suggest a broader epic-wide setting affecting siblings. No downstream agent decision or complete epic-grain lifecycle was simulated in this probe.

**Recommendation:** Remove the false prohibition, retain normal changeability guards, and update orientation generation as appropriate for changed emitted rules.

## Verification and coverage

### Executed baseline checks

- `go build -o /tmp/tiller-bughunt-20261026/tiller ./cmd/tiller`: succeeded.
- `make test`: all Go packages passed, uncached, in 150.46 seconds.
- `npm test` in `ui/`: **90 test files, 1,486 tests passed**, in 19.87 seconds.

The green baseline does not cover away the independently reproduced failures above. No fixes or regression tests were added.

### Real-surface probes

- Real CLI onboarding, all eleven `guide <skill> --json` calls, generated orientation and shims, question answers, claims/starts/checks/submissions, amendment staging/application, search identity lookup, relationship traversal, next/readouts, and linked-worktree onboarding/mutation.
- A two-task task-grain arc: check/submit on each recorded task branch, no-ff integration merges performed only as the disposable scenario driver, then sync. The first task crossed on ancestry; the second crossed on ancestry followed by feature and epic rollups. No premature ancestor completion was observed.
- Actual embedded console in managed headless Chromium and a separate named browser session: Home; Explore Browse, search, keyboard facets, node detail, Map inspection; Build selection; Fix attachment/filter routing; Report charts/dialog Escape and focus restoration; History window/group controls. Browser and HTTP verification were not replaced by unit tests.
- HTTP status/tree/map/build/work-items/queue/next/recap-peek/report/events/search/list reads returned 200 in the populated fixture. Foreign Origin, Host, and fetch-site probes returned 403; POST returned 405; an outside prose path returned 404; a symlinked prose path returned 400.
- Local release fixtures and a private temporary install/HOME: exact-version binary-only reinstall, already-current project refresh, offline inspection with ordinary project files unchanged, checksum-mismatch refusal preserving installed bytes, and combined candidate inspection/retention/replacement/child-maintenance all behaved as expected. No actual installation was replaced and no public release was requested.

### Review limits

Several parallel workers initially hit provider-capacity errors and were retried. Findings rely on completed reviewer analysis and observed reproductions, not failed workers. Crash/fsync/ENOSPC and permission-race injection, live legacy migrations, hosted PR operations, real Homebrew upgrades, Linux runtime behavior, mobile layout, screen-reader software, and every coarse-review/recovery permutation were not exercised. Passing probes are not claims of exhaustive correctness.

Unverified candidates were not promoted. Deliberate local/network trust rules, absent authentication/TLS, live refresh and console writes, deferred amendment validation, unknown-YAML preservation, and submodule completion fingerprints were not reported as bugs. Padded search queries and the suspected combined-amendment staging issue were exercised and worked.

## Reproduction evidence

Commands above use `tiller` for readability; all runtime probes used the freshly built `/tmp/tiller-bughunt-20261026/tiller`. Minted IDs and timestamps vary on replay. Scratch Git identities and human-only decisions belonged only to disposable verification scenarios, never real project work.

Detailed logs and screenshots were retained outside the source repository:

- Parent fixture: `/var/folders/78/ffmcy9t928x82cngyxffnypw0000gn/T/tiller-bughunt-distribution-wgunem3s/`, with `transcript.json`, `lifecycle-transcript.json`, `http-transcript.json`, and `oversized-http-transcript.json`.
- Storage/Git fixtures: `/tmp/tiller-storage-vcs-6NahIs`, `/tmp/tiller-storage-unborn-5nVAoF`, `/tmp/tiller-storage-linked-6NahIs`. Detailed backup: `/tmp/tiller-bughunt-storage-vcs-report.md`.
- Guidance fixture: `/tmp/tiller-bughunt-guidance-20261026-r74`; full guide payloads: `/tmp/tiller-bughunt-guidance-20261026-r74-guide-transcripts.json`.
- CLI fixture: `/tmp/tiller-cli-review-20261026-oF4txS`.
- Core mutation fixture and carrier transcript: `/tmp/tiller-core-mutations-review-c76a` and `/tmp/tiller-core-mutations-review-c76a-carrier.log`.
- Historical-grain fixtures and exact harness: `/tmp/tiller-lifecycle-prerequisite-1zkztffq`, `/tmp/tiller-lifecycle-overlay-9iqe398k`, `/tmp/tiller-lifecycle-repro-20261026.py`, and `/tmp/tiller-lifecycle-evidence-20261026/`.
- Console before/after screenshots: `/tmp/tiller-bughunt-ui-20261026-v81/build-before.png` and `build-after.png`.
- Parent Build screenshots: `/var/folders/78/ffmcy9t928x82cngyxffnypw0000gn/T/omp-sshots-15b528124fe40cbb.webp` before and `omp-sshots-15b52887cda40cbd.webp` after the same-page navigation.

All review browser sessions, Tiller servers, and the local release-fixture server were closed after verification. Scratch evidence was retained for inspection.

These temporary artifacts are local inspection aids, not permanent test fixtures. The reproduction descriptions in this report are the durable record.
