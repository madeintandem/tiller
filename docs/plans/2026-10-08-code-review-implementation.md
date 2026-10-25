# Code review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an embedded `code-review` skill (shim `/tiller-code-review`) whose review arm reviews one Tiller node's net change read-only into `.tiller/reviews/`, and whose disposition arm logs selected findings as proposed bugs, tweaks, chores, or one proposed amendment.

**Architecture:** Pure skill. No CLI command, store, schema, or console change. The skill is one embedded `SKILL.md` registered in `internal/skills`, delivered by the existing `tiller guide` and `tiller join` shim machinery; orientation and `orient` gain a routing line for explicit requests, and the generation advances because generated instructions change. Every write the disposition arm makes goes through commands that already exist (`tiller add bug|chore|tweak`, `tiller intake`, `tiller amend`).

**Tech Stack:** Go 1.x (`embed`, `testing`, `testify/require`), Markdown skill text, Starlight docs site.

**Spec:** `docs/plans/2026-10-08-code-review-design.md` (21 settled decisions). Read it before Task 1.

## Global Constraints

- Skill name `code-review`; shim path `.claude/skills/tiller-code-review/SKILL.md` (written by existing join code from `skills.Names()`).
- Skill text uses only a filesystem, a shell, and the `tiller` CLI. It must never contain any `harnessTools` token from `internal/skills/skills_test.go:16` (`bash`, `read tool`, `edit tool`, `write tool`, `grep tool`, `glob tool`, `view tool`, `str_replace`, `multiedit`, `notebookedit`, `apply_patch`; case-insensitive), so code fences use `sh` or `text`, never `bash`.
- No em dash anywhere in skill text, specs, or docs (house style; `TestSkillsAreNeutralProse` enforces it for skills).
- Skill text never names `docs/spec`, `artifact-schema`, `cli-surface`, `skill-set.md`, `ui-surface`, or `distribution.md` (`TestSkillsStayInsideTheProject`), nor any `retiredModel` token (`blitz`, `break-down`, `--intent`, `epic.eval`, `tiller schedule`, `tiller accept`, `accept epic`, `accept feature`, `tasks.yml`), nor `allen`.
- Review files: `.tiller/reviews/<YYYY-MM-DD>-<node-id>.md`, suffix `-2`, `-3` on collision, never overwritten. Prose only; no state file, no schema change.
- Severity (blocker, important, medium, minor) exists only in review prose. No record field, no `order.yml` change.
- Orientation generation: one past whatever `main` holds when this branch integrates. Main holds 44 at plan time, so this plan writes 45. If `brownfield-zero-day` (which claims 45 and 46) integrates first, rebase and use 47 everywhere this plan says 45, including the generation comment and every doc count line.
- Commits: `git commit -s`, subject prefixes as in `git log` (`Feat:`, `Orientation:`, `Docs:`), body ending with the trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never push; JC fast-forwards main.
- Run Go tests through `make test` (it sets the git config the fixtures need) or, for a single package, `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/<pkg>/ -run <Name>`.

## Review Focus

1. **A done task whose branch was deleted after a squash merge.** Expect the review to find its change through integration commits carrying the task ID, and to list it under Limitations when none carry it, never to skip it silently. Pinned in Task 1's test (`--grep=<unit-id>`, `under Limitations`).
2. **A second review of the same node on the same day.** Expect a `-2` suffix and no overwrite of a file the developer may have edited. Pinned in Task 1's test (`never overwrite a review`).
3. **A disposition naming a missing or already-logged row** (for example "log B9" when there are 8 bugs, or B3 twice across two requests). Expect nothing written and every problem named. Pinned in Task 2's test (`write nothing`).
4. **"Fix B2 on its branch" from a developer who does not own that task.** Expect no execution on someone else's started work: annotate and hand the row to the owner. Pinned in Task 2's test (`does not own`).
5. **An uncommitted review file sitting in the checkout.** Expect it to block nothing; `tiller status` prints only the informational `uncommitted tiller state` line (`internal/cli/status.go:300`). Exercised in Task 5's trial (step 4).

---

### Task 1: Register `code-review` with its review arm

**Files:**
- Create: `internal/skills/code-review/SKILL.md`
- Modify: `internal/skills/skills.go:1-35` (package comment, `//go:embed` line, `order`)
- Test: `internal/skills/skills_test.go` (update `TestNamesAreTheShippedSkills` at line 23; add `TestCodeReviewArmIsReadOnlyAndWritesOneReviewFile` and helper `reviewArmChoreography`)

**Interfaces:**
- Consumes: `skills.Names()`, `skills.Lookup(name)` (existing).
- Produces: skill name `"code-review"` appended last in `order`. `SKILL.md` section skeleton that Task 2 inserts into: `## Reasoning` > `### Review arm`; `## Recovery`; `## Choreography` > `### Review arm`; `## Hand-off` > `### Review arm`. Task 2 adds `### Disposition arm` subsections directly before `## Recovery`, directly before `## Hand-off`, and at end of file. Test helper `reviewArmChoreography(text string) string` (in `skills_test.go`) used again by Task 2.

- [ ] **Step 1: Write the failing tests**

In `internal/skills/skills_test.go`, change the expected list in `TestNamesAreTheShippedSkills`:

```go
	require.Equal(t, []string{
		"orient", "derive-baseline", "decompose", "specify", "execute", "capture", "mend-drift", "amend", "intake", "design-brief", "code-review",
	}, Names())
```

Append after `TestCaptureWritesAProposalAndStopsBeforeTriage`:

```go
// reviewArmChoreography is the review arm's slice of the code-review
// Choreography: from "## Choreography" up to the disposition arm's heading or,
// before that arm exists, the Hand-off.
func reviewArmChoreography(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, "## Choreography")
	require.Positive(t, start)
	rest := text[start:]
	end := len(rest)
	for _, marker := range []string{"\n### Disposition arm", "\n## Hand-off"} {
		if i := strings.Index(rest, marker); i > 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// The review arm writes one review file and nothing else: no state, no Git
// writes, no test runs, and every finding graded and anchored the same way.
func TestCodeReviewArmIsReadOnlyAndWritesOneReviewFile(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"name: code-review",
		"epic, feature, task, bug, chore, or tweak",
		"A review is never a suggested next move",
		".tiller/reviews/<YYYY-MM-DD>-<node-id>.md",
		"never overwrite a review",
		"env GIT_OPTIONAL_LOCKS=0 tiller",
		"git --no-optional-locks",
		"--no-ext-diff --no-textconv",
		"--grep=<unit-id>",
		"under Limitations",
		"Judge findings against the code as it reads now",
		"| Blocker |", "| Important |", "| Medium |", "| Minor |",
		"Bug, always a blocker",
		"Traced", "Suspected",
		"Found in",
		"Already tracked", "Previously declined",
		"regression of <id>",
		"Never reopen done work",
		"Never sample, never cap",
		"--gate <gate-key>",
		"Never add a gate",
		"There is no `tiller readout code-review`",
	} {
		require.Containsf(t, text, want, "code-review pins %q", want)
	}
	choreography := reviewArmChoreography(t, text)
	for _, forbidden := range []string{
		"tiller check", "tiller eval", "tiller sync", "tiller attest", "tiller claim",
		"tiller start", "tiller approve", "git fetch", "git checkout", "git commit", "git stash",
	} {
		require.NotContainsf(t, choreography, forbidden, "the review arm never runs %q", forbidden)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/skills/ -run 'TestNamesAreTheShippedSkills|TestCodeReviewArm'`
Expected: FAIL. `TestNamesAreTheShippedSkills` reports the missing `"code-review"`; `TestCodeReviewArmIsReadOnlyAndWritesOneReviewFile` fails at `require.True(t, ok)`.

- [ ] **Step 3: Write the skill's review arm**

Create `internal/skills/code-review/SKILL.md` with exactly this content:

````markdown
---
name: code-review
description: Use only when the developer explicitly asks for a code review of one Tiller epic, feature, task, bug, chore, or tweak, or names rows of an existing review to log, fix on their branch, drop, or commit. Reviews the node's net change read-only, writes one review file under .tiller/reviews/, and replies with a summary table. A later disposition logs the selected rows as proposed bugs, tweaks, chores, or one proposed amendment and commits once, never approving anything.
---

# code-review

## Trigger

This skill has two arms, and each needs its own explicit request.

The review arm runs when the developer asks for a code review of one Tiller node, for example "code review feature F-rnte2m", "give me a code review for epic E-2hjdhh", or "review bugs/blank-on-reload before I merge it". The target is one epic, feature, task, bug, chore, or tweak, named by key, ID, or a reference the conversation makes unambiguous. Otherwise ask which node is meant, naming the candidates from reads rather than guessing. A request for the whole project, an arbitrary diff, or a path is not a Tiller review; say so and leave it to whatever general-purpose reviewer the developer prefers.

The disposition arm runs when the developer names rows of an existing review file: "log B1, B3, and TW1-3", "log all blockers", "log B6 as a tweak", "fix B2 on its branch", "drop CH1", "log the rest", or "commit the review".

Entering a project, finishing work, reaching a gate, or asking what to do next triggers neither arm. A review is never a suggested next move. The request authorizes only the review or the named disposition: never approve, claim, start, attest, resolve, or cancel anything.

## Reasoning

### Review arm

#### Scope is the node's net change

Gather every unit under the target that is not cancelled: the task itself, a feature's tasks, or every feature's tasks for an epic. A bug, chore, or tweak is its own single unit. Read each unit's state file for `status`, `branch`, `pr`, and `review_ref`, and read the integration branch from `.tiller/tiller.yml`.

Locate each unit's change with local, read-only Git:

| Unit | Where its change is |
|---|---|
| In flight (`in_progress`, `check`, `in_review`) | The recorded branch head against its merge-base with the integration branch. Use the local branch, or its remote-tracking ref when only that exists. |
| Done | In order: the recorded branch if the ref still exists; the `review_ref.head` commit if it is still present; otherwise the integration-branch commits whose messages carry the unit's ID, which every execution commit does. |
| Not started, or no locatable change | Not reviewed. It goes under Limitations with the reason. |

A target with no started unit gets a one-line reply that there is nothing to review yet, and no file.

The diffs locate what changed. Judge findings against the code as it reads now: the integration branch for merged work, the branch head for in-flight work. A defect one unit introduced and a later unit fixed is not a finding. Read surrounding code for context, but anchor every finding in lines this node's work changed. Record the integration commit and each in-flight branch head you read; these pins are what keep the file's line references meaningful after the code moves on.

#### What to look for

| Lens | Lands as |
|---|---|
| Correctness | Bug |
| Acceptance contract: `feature.md`, the `feature.eval.md` criteria, and `preserve` criteria | Bug, always a blocker |
| Security and permission boundaries | Bug |
| Test trustworthiness | Important bug for a test that proves nothing; chore for missing coverage of a risky path |
| Maintainability: duplication, unclear names, dead code | Tweak or chore |
| Design: a wrong boundary, an abstraction that does not earn its keep, a shape that will not scale | Plan change |
| Performance, only where the cost is evident in the code | Bug or plan change |

Leave out spec, doc, and code drift: `tiller drift` and `mend-drift` own it, so a suspicion goes under Limitations as "run `tiller drift`". Leave out formatter or linter style, accessibility, and every optional review gate.

#### Classify each finding

Use capture's kinds exactly. A bug is behavior that violates expected product or system behavior, including any violated requirement. A tweak is a minor adjustment that may change code. A chore is housekeeping without code side effects. Anything larger, work that needs a feature's acceptance contract or a change to the tree, is a plan change: a new task under a named feature, a new feature under a named epic, a new epic, or a revision of a non-terminal feature or task. A change to done or cancelled work is new forward work, never a revision.

#### Grade bugs by impact

| Severity | Meaning |
|---|---|
| Blocker | Data loss or corruption, a broken security or permission boundary, a crash or hang on a main path, or any violation of an approved acceptance criterion or `preserve` criterion of the reviewed node. Should not merge, or should not stay shipped. |
| Important | Wrong behavior on a realistic path without data loss, including a test that passes while proving nothing about the behavior it claims to cover. |
| Medium | Wrong behavior on an edge case or unusual input, or a fragile construct likely to break under a plausible change. |
| Minor | A real defect with negligible impact, such as a wrong message, a misleading log line, or an off-by-one in a display-only count. |

Grade by impact, never by the size of the fix. Style and naming are never bugs. Severity is review prose only: it never becomes a record field or a queue position.

Label every bug's confidence. Traced means you followed the failing path through the code and can name each step. Suspected means the pattern is wrong but the path is unproven. Proof by execution comes later, when the fix's regression test is written.

#### Attach each finding where it was found

Each finding records Found in: the unit whose diff introduced its anchored lines, the feature when it spans that feature's tasks, or the epic when it spans features. For a bug, chore, or tweak target, Found in is the target. When the code is unmerged, Found in carries the unit's status, for example `T-d4e5f6 (in_review, unmerged)`.

#### Respect what Tiller already knows

Before writing, read the bugs, chores, and tweaks that are neither done nor cancelled and are attached to any in-scope node, the proposed amendments touching the scope, and earlier review files for in-scope nodes under `.tiller/reviews/`.

- A finding that matches an open bug, chore, or tweak, or a staged plan change, goes in Already tracked, unnumbered, citing that node.
- A finding the developer declined in an earlier review that still holds goes in Previously declined, unnumbered, citing that review.
- A defect that returns after its bug reached done is a new finding marked `regression of <id>`. Never reopen done work.

Matching is your judgment, so always cite what you matched against; the developer may overrule it.

#### Keep the review read-only

The only write is the one new review file. Do not change prose, source code, state, the manifest, claims, gates, events, questions, Git history, refs, or the index. Do not run `check`, `eval`, `sync`, `attest`, `claim`, `join`, `migrate`, or any other state-writing command. Do not fetch, check out, stash, or build, and do not run the project's tests or scripts. Read branches with `git show` and `git diff`. Prefix every Tiller invocation with `env GIT_OPTIONAL_LOCKS=0`, use `git --no-optional-locks` for direct Git reads, and pass `--no-ext-diff --no-textconv` to every diff. A branch present only on the remote goes under Limitations with the exact `git fetch` to run, so the developer can fetch and ask again.

#### Cover everything or say what you missed

Review in feature-sized passes and merge them into one file. For an epic, finish with one cross-feature pass over the seams between features. Number findings only after the merge, so severity order holds across passes. Passes may run in parallel when your environment allows it, but the result is one file. Never sample, never cap findings, and list every unit not reviewed under Limitations with its reason.

## Recovery

On a refusal, follow `.tiller/orientation.md`. In the review arm, recovery is limited to corrected reads: a missing branch, an unreadable diff, or stale instructions go under Limitations, never become a reason to repair, fetch, refresh, or mutate anything.

## Choreography

### Review arm

#### 1. Resolve the target and inventory its units

```sh
env GIT_OPTIONAL_LOCKS=0 tiller show <epic|feature|task|bug|chore|tweak> <key-or-id>
env GIT_OPTIONAL_LOCKS=0 tiller tree epic <epic-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller tree feature <feature-key-or-id> --status --files
env GIT_OPTIONAL_LOCKS=0 tiller ls tasks --feature <feature-key-or-id>
```

Run only the forms that apply. Read each unit's state file for `branch`, `pr`, and `review_ref`, and read each in-scope feature's `feature.md` and `feature.eval.md` in full.

#### 2. Locate each unit's change

```sh
# in flight, or done with its branch still present
git --no-optional-locks merge-base <integration> <branch>
git --no-optional-locks diff --no-ext-diff --no-textconv <merge-base> <branch>

# done, branch gone, review_ref.head still present
git --no-optional-locks cat-file -e <review-head>^{commit}
git --no-optional-locks merge-base <integration> <review-head>
git --no-optional-locks diff --no-ext-diff --no-textconv <merge-base> <review-head>

# done, neither ref present: integration commits that carry the unit's ID
git --no-optional-locks log --format=%H --grep=<unit-id> <integration>
git --no-optional-locks show --no-ext-diff --no-textconv <commit>

# pins for the file header
git --no-optional-locks rev-parse <integration> <each-in-flight-branch>
```

A unit none of these locate goes under Limitations.

#### 3. Read what is already known

```sh
env GIT_OPTIONAL_LOCKS=0 tiller ls bugs
env GIT_OPTIONAL_LOCKS=0 tiller ls chores
env GIT_OPTIONAL_LOCKS=0 tiller ls tweaks
env GIT_OPTIONAL_LOCKS=0 tiller ls amendments --status proposed
ls .tiller/reviews/
```

Read `attached_to` for each open bug, chore, or tweak, each proposed amendment's staged operations, and the earlier review files for in-scope nodes.

#### 4. Write one review file

Write `.tiller/reviews/<YYYY-MM-DD>-<node-id>.md`. When that path exists, add `-2`, `-3`, and so on: never overwrite a review. Every finding is one contiguous block under its own heading, so a line range cites exactly that block. Use this shape:

```markdown
# Code review: <kind> <key path> (<id>) "<title>"

Reviewed <YYYY-MM-DD> by <contributor>. A derived review, uncommitted until a disposition.
Scope: each unit by key path, ID, and status.
Pins: integration <branch> at <commit>; <in-flight branch> at <commit>.
Contract read: the feature.md, feature.eval.md, and preserve criteria read.

## Summary

<the reply's table>

## Bugs

### Blocker

#### B1 <title, at most 60 characters>

Severity: blocker. Confidence: traced. Found in: <key path> (<id>).
Anchors: `<path>:<line>`, each with a short excerpt. Violates: <criterion ID>, when one applies.
**Reproduction** <steps>
**Expected** <behavior>
**Actual** <behavior>
**Fixed when** <observable condition>

### Important

### Medium

### Minor

## Tweaks

#### TW1 <title>

Found in: <key path> (<id>). Anchors: `<path>:<line>`.
**What** <change> **Why** <reason> **Done when** <observable condition>

## Chores

#### CH1 <title>

Found in: <key path> (<id>). Anchors: `<path>:<line>`.
**What** <change> **Why** <reason> **Done when** <observable condition>

## Plan changes

#### P1 <title>

Proposed shape: new task under <feature> | new feature under <epic> | new epic | revise <feature or task>.
**Problem** <what is wrong> **Proposed outcome** <what should be true>
Criteria it would add or change, when any.

## Already tracked

## Previously declined

## Limitations
```

Number per section with prefixes: B, TW, CH, P. Bug numbers run straight through the severity groups, most severe first, so every number is unique. Omit an empty severity group or section, except Limitations, which says "None" when nothing was missed. A clean review still writes its file.

#### 5. Reply and stop

Reply with the target, the review path, and one table holding every numbered row, then the counts. Never cap the table. For example:

```text
Code review: feature form-engine/form-reload (F-rnte2m), 4 tasks, 2 in flight
Full review: .tiller/reviews/2026-10-08-F-rnte2m.md

| #   | Severity  | Finding                            | Found in             | Where            |
|-----|-----------|------------------------------------|----------------------|------------------|
| B1  | blocker   | Draft lost on reload (violates C3) | T-a1b2c3             | form/load.go:88  |
| B2  | important | Retry swallows 409                 | T-d4e5f6 (unmerged)  | form/save.go:141 |
| TW1 | tweak     | "Save" to "Save changes"           | T-a1b2c3             | ui/Form.tsx:52   |
| CH1 | chore     | Stale doc comment on loadDraft     | T-a1b2c3             | form/load.go:70  |
| P1  | plan      | New task: offline draft cache      | F-rnte2m             | -                |

2 bugs (1 blocker, 1 important), 1 tweak, 1 chore, 1 plan change
Also: 2 already tracked, 1 previously declined (details in the file)
Unmerged: "fix B2 on its branch" sends it back to T-d4e5f6
Log with, for example: "log B1, TW1, P1", "fix B2 on its branch", "drop CH1"
```

Titles stay within 60 characters, and Where shows the first anchor. A clean review replies in one line with the path.

## Hand-off

### Review arm

Stop after the reply. State that the review file is uncommitted and that any disposition, including "commit the review", commits it; deleting an unwanted review is just deleting the file.

When the target is a feature or task and `.tiller/tiller.yml` already declares an optional code review attestation gate, add one line for the human with the manifest's own gate key verbatim (`tiller init` seeds `code_review`) and the blocker count beside it:

```text
Attest when ready (1 blocker open): tiller attest feature <F-id> --gate <gate-key> --status <pass|fail> --reason "reviewed: .tiller/reviews/<file>.md"
```

Leave the status to the human and never run it. Print nothing about gates for an epic, a bug, chore, or tweak, or a project without that gate. Never add a gate to the manifest because a review was requested.

The review is not a lifecycle seam. There is no `tiller readout code-review`, and no lifecycle readout follows the reply.
````

- [ ] **Step 4: Register the skill**

In `internal/skills/skills.go`, replace the package comment's catalog sentence and add a line, then extend the embed and order:

```go
// This build ships eleven skills: orient, derive-baseline, decompose, specify,
// execute, capture, mend-drift, amend, intake, design-brief, and code-review.
// derive-baseline establishes the current-state design layer before
// decomposition on existing-code projects.
// capture records a complete shelf proposal and stops before triage.
// design-brief exports a derived UI/UX handoff without changing project state.
// code-review reviews one node's net change read-only into .tiller/reviews/ and
// logs only the findings the developer names, through capture and intake shapes.
```

```go
//go:embed orient/SKILL.md derive-baseline/SKILL.md decompose/SKILL.md specify/SKILL.md execute/SKILL.md capture/SKILL.md mend-drift/SKILL.md amend/SKILL.md intake/SKILL.md design-brief/SKILL.md code-review/SKILL.md
var files embed.FS

// order is the canonical lifecycle order: the dispatcher, the existing-code
// baseline, the delivery seams, capture, drift and plan-change seams, then the
// explicitly requested export and review.
var order = []string{"orient", "derive-baseline", "decompose", "specify", "execute", "capture", "mend-drift", "amend", "intake", "design-brief", "code-review"}
```

- [ ] **Step 5: Run the skills and guide packages**

Run: `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/skills/ ./internal/cli/ -run 'TestNames|TestCodeReview|TestLookup|TestSkills|TestGuide|TestJoinRecordsIdentity'`
Expected: PASS. `TestGuideSkillPrintsTheSkill` and `TestJoinRecordsIdentityAndWiresAgents` iterate `skills.Names()`, so they now cover `code-review` and its shim with no edit. If `TestSkillsAreNeutralProse` fails, search the new file for the named token or an em dash and reword.

- [ ] **Step 6: Commit**

```sh
git add internal/skills/code-review/SKILL.md internal/skills/skills.go internal/skills/skills_test.go
git commit -s -m "Feat: add the code-review skill's read-only review arm" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Add the disposition arm

**Files:**
- Modify: `internal/skills/code-review/SKILL.md` (insert three `### Disposition arm` subsections and extend `## Recovery`)
- Test: `internal/skills/skills_test.go` (add `TestCodeReviewDispositionLogsThroughExistingShapesAndCommitsOnce`)

**Interfaces:**
- Consumes: Task 1's section skeleton and `reviewArmChoreography`.
- Produces: the disposition arm text. Nothing in Go.

- [ ] **Step 1: Write the failing test**

Append to `internal/skills/skills_test.go`:

```go
// The disposition arm resolves the whole selection first, rechecks moved code,
// records only through capture and intake shapes, commits once, and leaves every
// approval to the human.
func TestCodeReviewDispositionLogsThroughExistingShapesAndCommitsOnce(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"### Disposition arm",
		"tiller guide capture",
		"tiller guide intake",
		"tiller guide execute",
		"write nothing",
		"\"the rest\"",
		"log B6 as a tweak",
		"drop CH1",
		"commit the review",
		"attach to <node>",
		"rechecked at <commit>",
		"no longer reproduces at <commit>",
		"Never recheck an unselected row",
		"tiller add bug <key> --title",
		"--source .tiller/reviews/<file>.md#L<n>-L<m>",
		"-plan-changes.md",
		"tiller intake <distillation> --contributor <you>",
		"--source .tiller/feedback/<distillation>.md#L<n>-L<m>",
		"one distillation, one intake, one amendment",
		"does not own",
		"git commit -m \"code-review: <node-id> log B1 TW1 P1, drop CH1\"",
		"Never push",
		"tiller approve bug|chore|tweak <node>",
		"tiller approve amendment <amendment-key>",
	} {
		require.Containsf(t, text, want, "code-review disposition pins %q", want)
	}
	start := strings.Index(text, "### Disposition arm\n\n#### 1.")
	handoff := strings.Index(text, "## Hand-off")
	require.Positive(t, start, "the disposition arm has its own choreography")
	require.Greater(t, handoff, start)
	choreography := text[start:handoff]
	for _, forbidden := range []string{
		"tiller approve", "tiller claim", "tiller start", "tiller resolve",
		"tiller cancel", "tiller attest", "git push",
	} {
		require.NotContainsf(t, choreography, forbidden, "the disposition arm never runs %q", forbidden)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/skills/ -run TestCodeReviewDisposition`
Expected: FAIL on the first missing pin, `### Disposition arm`.

- [ ] **Step 3: Insert the disposition reasoning**

In `internal/skills/code-review/SKILL.md`, insert directly before the line `## Recovery`:

````markdown
### Disposition arm

#### Find the review file

Use the review from this conversation. In a new session, use the newest file under `.tiller/reviews/` whose name carries the node the request names ("log B1 from the F-rnte2m review"). When more than one candidate has open rows, ask which.

#### Resolve the whole selection before writing anything

Plain numbers resolve through the noun the developer used: "bugs 1, 3" is B1 and B3, "tweaks 1-3" is TW1 through TW3, "plan change 2" is P2. Lists, ranges, and "all" work ("all blockers", "all tweaks"). "the rest" means every numbered row with no disposition yet. A row whose heading carries a disposition annotation is closed. If any named row does not exist, is closed, or is ambiguous, write nothing: name every problem at once and wait for the developer.

#### The verbs

| Request | Effect |
|---|---|
| "log B1, TW2, P1" | Bug, tweak, and chore rows become `proposed` shelf nodes. Plan-change rows become one `proposed` amendment. |
| "log B6 as a tweak" | Reclassify, then log as the chosen kind with that kind's contract. The row records both kinds. |
| "log B3, attach to <node>" | Replace the row's Found in for this attachment. |
| "B-k2x9ab is different, log it" | Override an Already tracked match, number the finding into its section, then log it. |
| "log the declined X as a bug" | Revive a Previously declined finding, number it into its section, then log it. |
| "fix B2 on its branch" | Only for a row whose Found in is unmerged. Nothing is minted; the row goes back to its unit through execute. |
| "drop CH1" | Decline the row so a later "the rest" skips it. Optional: a row nobody mentions stays open. |
| "commit the review" | Commit the review file with no other change, for a clean review or one where nothing is logged. |

A new number takes the next free number in its section, after every existing one. Existing numbers never change.

#### Recheck moved code

Compare the file's pins with the current heads. If nothing in scope moved, proceed. Otherwise re-read each selected row's anchors in the current code, still read-only:

- The finding holds: proceed, and annotate the row `rechecked at <commit>`.
- The finding holds but its lines moved: proceed with corrected anchors, and correct the row in the same commit.
- The finding is gone: stop the whole batch, report "<row> no longer reproduces at <commit>", and wait for "drop it" or "log anyway".

Never recheck an unselected row. A fix always rechecks, because execute is about to work from it.

#### Log bugs, tweaks, and chores through capture's shapes

Follow `tiller guide capture` for keys, contracts, and its stopping point. Pass the row's Found in as `--attach` and cite the row's block in the current review file as `--source`, so the developer's edits to the file carry through. Write the contract from the block: a bug's Reproduction, Expected, Actual, and Fixed when; a tweak's or chore's What, Why, and Done when. Keep the review's severity and confidence as the opening line of `bug.md`; it is prose, never a field. For a row in unmerged code, warn that the new node cannot start until its unit merges, because a shelf branch is cut from the integration branch.

#### Log plan changes through intake's shapes

Follow `tiller guide intake` for staging and the amendment narrative. Write a distillation outside the repository named `<YYYY-MM-DD>-<node-id>-plan-changes.md` (add `-2`, `-3`, and so on when `.tiller/feedback/` already holds that name). It holds only the selected plan changes, each as its own block with its problem and proposed outcome, under a header naming the review file and the row numbers it came from. Land it with `tiller intake`, then stage one operation per plan change, each citing its own block in the landed copy under `.tiller/feedback/`. A revision targets only a non-terminal feature or task; a change to done work stages forward work instead. Each request that logs plan changes is one distillation, one intake, one amendment; a later request never stages onto an earlier review's amendment.

#### Fix on its branch

Requested changes return to the unit's owner and branch. When the acting developer does not own the unit, annotate the row `requested change on <unit>` and hand it to the owner without executing anything. Otherwise annotate, commit, and follow `tiller guide execute` for that unit: the requested-changes re-entry for in-review work, or the ordinary loop for work still in progress.

#### Annotate and commit once

Append each disposition to the end of its row's heading line, so no line moves and every line-range citation stays exact: `→ B-k2x9ab`, `→ amendment <amendment-key>`, `→ requested change on <unit>`, `declined`, `logged as tweak TW-...`, `rechecked at <commit>`. Then make one commit holding the annotated review file, any new shelf nodes, and any feedback copy and amendment state. It lands where capture's commit would land. Never push.

````

- [ ] **Step 4: Extend Recovery**

Append to the `## Recovery` section, after its existing paragraph:

```markdown
In the disposition arm, a `tiller add` refusal means fixing the key, attachment, or citation and rerunning; re-adding the same key is an idempotent readback. An `artifact_exists` refusal from `tiller intake` means choosing the next suffix for the distillation. Commit nothing until every selected row is recorded.
```

- [ ] **Step 5: Insert the disposition choreography**

Insert directly before the line `## Hand-off`:

````markdown
### Disposition arm

#### 1. Resolve rows and pins

```sh
git --no-optional-locks rev-parse <integration> <each-in-flight-branch>
```

Compare with the file's pins and recheck the selected rows when anything moved.

#### 2. Log bug, tweak, and chore rows

```sh
tiller add bug <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/bugs/<key>/bug.md with the contract from the row's block

tiller add tweak <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/tweaks/<key>/tweak.md with What, Why, and Done when

tiller add chore <key> --title "<title>" --attach <found-in> \
  --source .tiller/reviews/<file>.md#L<n>-L<m>
# replace .tiller/chores/<key>/chore.md with What, Why, and Done when
```

#### 3. Log plan-change rows

```sh
tiller intake <distillation> --contributor <you>
# lands .tiller/feedback/<distillation>.md and opens its amendment

tiller amend <amendment-key> --add-task <epic>/<feature>/<task-key> --title "<title>" \
  --source .tiller/feedback/<distillation>.md#L<n>-L<m>
# or --add-feature, --add-epic, or --revise, per the row's proposed shape
# write the narrative over .tiller/amendments/<A-id>-<amendment-key>.md

tiller show amendment <amendment-key>
```

#### 4. Annotate and commit once

```sh
git add .tiller
git commit -m "code-review: <node-id> log B1 TW1 P1, drop CH1"
```

For a fix, commit the annotation alone first (`code-review: <node-id> fix B2 on <unit>`), then follow `tiller guide execute`.

````

- [ ] **Step 6: Append the disposition hand-off**

Append at the end of the file:

```markdown
### Disposition arm

Stop after the commit. Name each new bug, chore, or tweak by key path with its ID, with the source and attachment recorded and the exact `tiller approve bug|chore|tweak <node>` command for triage. Name the amendment with its exact `tiller approve amendment <amendment-key>` command. Run none of them. Repeat any unmerged-code warning. A fix hands over to execute, or to the owner named on the row.
```

- [ ] **Step 7: Run the skills package**

Run: `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/skills/`
Expected: PASS, including Task 1's test. Its choreography slice now ends at `### Disposition arm`, so the disposition's `git commit` stays outside the review arm.

- [ ] **Step 8: Commit**

```sh
git add internal/skills/code-review/SKILL.md internal/skills/skills_test.go
git commit -s -m "Feat: add the code-review disposition arm" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Route explicit requests and advance orientation

**Files:**
- Modify: `internal/skills/orient/SKILL.md:12-14` (after the design-brief paragraphs) and `:108` (recovery line)
- Modify: `internal/gen/orientation.go:25-26` (generation), `:172-178` (recording rule), `:355-357` (recovery), `:389-391` (readout), `:426-436` (routing paragraph), `:801` and after `:833` (skill list)
- Test: `internal/gen/orientation_test.go` (add `TestOrientationRoutesExplicitCodeReviewsOutsideTheWalk`)
- Test: `internal/skills/skills_test.go` (add `TestOrientRoutesExplicitCodeReviews`)
- Test: `internal/cli/join_test.go:297-325` (generalize the design-brief refresh test)

**Interfaces:**
- Consumes: skill name `code-review` from Task 1.
- Produces: `gen.OrientationGeneration()` returns 45 (see the generation rule in Global Constraints).

- [ ] **Step 1: Write the failing tests**

Append to `internal/gen/orientation_test.go`:

```go
// Explicit code review requests reach their skill outside the next-work walk,
// and the review's file write is named as the one exception to state recording.
func TestOrientationRoutesExplicitCodeReviewsOutsideTheWalk(t *testing.T) {
	text := Orientation()
	for _, want := range []string{
		"`tiller guide code-review`",
		"`tiller guide` serves eleven skills:",
		"- `code-review`: the developer explicitly asks",
		"`.tiller/reviews/`",
		"Do not invent a `tiller readout code-review` command.",
		"During `design-brief` or a `code-review` review, recover only through corrected reads.",
	} {
		require.Containsf(t, text, want, "orientation pins %q", want)
	}
	require.Equal(t, 45, OrientationGeneration())
}
```

If `orientation_test.go` does not import `require`, add `"github.com/stretchr/testify/require"` to its imports.

Append to `internal/skills/skills_test.go`:

```go
// orient sends an explicit review or disposition request straight to the skill.
func TestOrientRoutesExplicitCodeReviews(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "loads `tiller guide code-review` directly")
	require.Contains(t, text, "For a `code-review` review, recovery is limited to corrected reads.")
}
```

In `internal/cli/join_test.go`, replace `TestInstructionRefreshInstallsMissingDesignBrief` with a version that covers both explicitly requested skills:

```go
// Both supported refresh paths install newly cataloged skills on joined checkouts
// without changing the route or rewriting discovery on a repeat refresh.
func TestInstructionRefreshInstallsMissingRequestedSkills(t *testing.T) {
	for _, skill := range []string{"design-brief", "code-review"} {
		for _, command := range []string{"join", "migrate"} {
			t.Run(skill+"/"+command, func(t *testing.T) {
				dir := initRepo(t)
				registerOwner(t, "jc", "JC Grubbs")
				_, stderr, code := execCLI(t, "join", "--as", "jc")
				require.Equal(t, 0, code, stderr)
				route := mustJSON(t, "guide", "--json")
				shimPath := filepath.Join(dir, ".claude", "skills", "tiller-"+skill, "SKILL.md")
				wantShim := readFile(t, shimPath)
				require.NoError(t, os.Remove(shimPath))
				orientationPath := filepath.Join(dir, ".tiller", "orientation.md")
				aged := strings.Replace(gen.Orientation(),
					"orientation generation "+strconv.Itoa(gen.OrientationGeneration()),
					"orientation generation 33", 1)
				require.NoError(t, os.WriteFile(orientationPath, []byte(aged), 0o644))

				for range 2 {
					_, stderr, code = execCLI(t, command)
					require.Equal(t, 0, code, stderr)
					require.Equal(t, wantShim, readFile(t, shimPath))
					require.Equal(t, gen.Orientation(), readFile(t, orientationPath))
					require.Equal(t, route, mustJSON(t, "guide", "--json"))
				}
			})
		}
	}
}
```

Copy the body lines from the existing test rather than this listing if they have drifted; only the outer `skill` loop and the `"tiller-"+skill` path are new.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false go test -count=1 ./internal/gen/ ./internal/skills/ ./internal/cli/ -run 'TestOrientationRoutesExplicitCodeReviews|TestOrientRoutesExplicitCodeReviews|TestInstructionRefreshInstallsMissingRequestedSkills'`
Expected: the two content tests FAIL on their first pin. The join test PASSES already (the shim exists since Task 1); it is a regression guard for the generation bump.

- [ ] **Step 3: Route in orient**

In `internal/skills/orient/SKILL.md`, insert after the paragraph beginning "For that export, prefix every Tiller read" (line 14):

```markdown
An explicit request for a code review of one epic, feature, task, bug, chore, or tweak, or a request naming rows of an existing review to log, fix on their branch, drop, or commit, instead loads `tiller guide code-review` directly. It sits outside the state-routing walk the same way. The request authorizes only the read-only review or the named disposition, never approval, claiming, or starting. Follow that guide and end with its reply, without orient's readout or next-work handoff. An ordinary next-work request or completed step does not trigger it.
```

After the line beginning "For `design-brief`, recovery is limited to corrected reads." (line 108), add:

```markdown
For a `code-review` review, recovery is limited to corrected reads. A disposition records through `tiller` commands and recovers by the ordinary protocol above.
```

- [ ] **Step 4: Update orientation**

In `internal/gen/orientation.go`:

1. Generation, after the "Generation 44" comment line:

```go
// Generation 45 adds explicitly requested node code reviews and their dispositions.
const orientationSchemaGen = 45
```

2. In the "End lifecycle reasoning" bullet, after the sentence ending "No approval or execution is implied.", append before the closing of that bullet:

```go
  An explicitly requested ` + "`code-review`" + ` review writes only its review
  file under ` + "`.tiller/reviews/`" + `; its disposition records through
  ` + "`tiller`" + ` commands and one commit, and approves nothing.
```

3. Replace "During ` + "`design-brief`" + `, recover only through corrected reads." with:

```go
  During ` + "`design-brief`" + ` or a ` + "`code-review`" + ` review, recover only through corrected reads.
```

4. After "Do not invent a ` + "`tiller readout design-brief`" + ` command.", add:

```go
  ` + "`code-review`" + ` is not a lifecycle seam either: its reply ends with its
  summary table and review path. Do not invent a ` + "`tiller readout code-review`" + ` command.
```

5. After the design-brief routing paragraph that ends "Do not let read-only gathering refresh the index.", add a blank line and:

```go
An explicit request for a code review of one epic, feature, task, bug, chore, or
tweak, or a request naming rows of an existing review, goes directly to
` + "`tiller guide code-review`" + `, outside the next-work walk. It authorizes only
the read-only review or the named disposition; never approve, claim, or start
anything. A normal next-work request or completed step does not trigger it.
```

6. Change "` + "`tiller guide`" + ` serves ten skills:" to "serves eleven skills:", and after the `design-brief` bullet add:

```go
- ` + "`code-review`" + `: the developer explicitly asks for a code review of one
  node, or names rows of an existing review. Review its net change read-only into
  one file under ` + "`.tiller/reviews/`" + ` and reply with a summary table; on a
  later disposition, log the named rows through capture and intake shapes and
  commit once, leaving every approval to the human.
```

- [ ] **Step 5: Run the affected packages**

Run: `make test`
Expected: PASS. Tests that compare against `gen.Orientation()` or `gen.OrientationGeneration()` (join, migrate, project refresh, status, readiness) read the constant, so they need no edit. If one hard-codes `44`, change it to read `gen.OrientationGeneration()`.

- [ ] **Step 6: Commit**

```sh
git add internal/skills/orient/SKILL.md internal/skills/skills_test.go internal/gen/orientation.go internal/gen/orientation_test.go internal/cli/join_test.go
git commit -s -m "Orientation: route explicit code reviews; generation 45" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Specs and reader docs

**Files:**
- Modify: `docs/spec/skill-set.md:5`, `:88`, the skills table after `:99`, new `### code-review` section before `## Format, packaging, and portability` (`:569`)
- Modify: `docs/spec/artifact-schema.md` layout tree after the `amendments/` lines (`:84-86`), and a paragraph after `:957`
- Modify: `docs/spec/cli-surface.md:306`, `:310`, `:839`
- Modify: `docs/spec/distribution.md:5`, `:298`, after `:313`, `:327`, `:397`
- Modify: `docs/tiller-commands.md:7-8`, `:101`, new paragraph after `:120`
- Modify: `site/src/content/docs/skills.md:15`, new `## Request a code review` section at the end
- Modify: `site/src/content/docs/quickstart.md:7-8`, `site/src/content/docs/source-quickstart.md:9`
- Modify: `docs/plans/STATUS.md:3` and the current-snapshot list

**Interfaces:**
- Consumes: final skill behavior from Tasks 1 to 3.
- Produces: docs only.

- [ ] **Step 1: Count and generation lines**

Make these exact replacements (generation per Global Constraints):

| File | Old | New |
|---|---|---|
| `docs/spec/skill-set.md:5` | `ten embedded skills, schema 10 and orientation generation 44` | `eleven embedded skills, schema 10 and orientation generation 45` |
| `docs/spec/distribution.md:5` | `orientation generation 44, and ten embedded skills` | `orientation generation 45, and eleven embedded skills` |
| `docs/spec/distribution.md:298` | `The source orientation generation is 44, with schema 10 and ten embedded skills. It adds explicitly authorized merged aggregate review recovery` | `The source orientation generation is 45, with schema 10 and eleven embedded skills. It adds explicitly requested node code reviews; generation 44 added explicitly authorized merged aggregate review recovery` |
| `docs/spec/distribution.md:327` | `installs ten embedded skills` ... `intake, and design-brief.` | `installs eleven embedded skills` ... `intake, design-brief, and code-review.` |
| `docs/spec/distribution.md:397` | `ten skills, and orientation generation 44` | `eleven skills, and orientation generation 45` |
| `docs/spec/cli-surface.md:839` | `ten named guides:` ... `` `intake`, and `design-brief`. `` | `eleven named guides:` ... `` `intake`, `design-brief`, and `code-review`. `` |
| `docs/tiller-commands.md:7` | `orientation generation 44, and ten embedded skills.` | `orientation generation 45, and eleven embedded skills.` |
| `docs/tiller-commands.md:8` | `Generation 44 adds explicitly authorized historical aggregate review recovery;` | `Generation 45 adds explicitly requested node code reviews;` |
| `docs/tiller-commands.md:101` | `# capture \| mend-drift \| amend \| intake \| design-brief` | `# capture \| mend-drift \| amend \| intake \| design-brief \|`, then a new line `# code-review` aligned under it |
| `site/src/content/docs/skills.md:15` | `Current source carries ten skills, including the explicitly requested `design-brief`, schema 10, orientation generation 44` | `Current source carries eleven skills, including the explicitly requested `design-brief` and `code-review`, schema 10, orientation generation 45` |
| `site/src/content/docs/quickstart.md:7-8` | `**44**, and **ten skills**` | `**45**, and **eleven skills**` |
| `site/src/content/docs/source-quickstart.md:9` | `**44**, and **ten embedded skills**` | `**45**, and **eleven embedded skills**` |
| `docs/plans/STATUS.md:3` | `orientation generation **44**, and **ten embedded skills**` | `orientation generation **45**, and **eleven embedded skills**` |

Then run `grep -rn "ten skills\|ten embedded\|ten named\|generation 44\|generation \*\*44" docs/spec docs/tiller-commands.md docs/plans/STATUS.md site/src/content/docs README.md` and fix any current-state line it still finds. Leave historical lines in `docs/project-history.md` alone.

- [ ] **Step 2: skill-set.md table, framing line, and section**

At `docs/spec/skill-set.md:88`, change "design-brief is an explicitly requested read-only export." to "design-brief is an explicitly requested read-only export; code-review is an explicitly requested review whose disposition records only what the developer names."

Add this row to the skills table after the `design-brief` row:

```markdown
| `code-review` | the developer explicitly requests a code review of one epic, feature, task, bug, chore, or tweak, or names rows of an existing review | the node's net change reviewed read-only into graded findings, then named rows recorded through capture and intake shapes | one uncommitted review file and a summary table; on disposition, one commit of `proposed` shelf nodes and at most one `proposed` amendment |
```

Insert before `## Format, packaging, and portability`:

```markdown
### code-review

**Trigger.** The developer explicitly requests a code review of one epic, feature, task, bug, chore, or tweak ("code review feature F-rnte2m"), or names rows of an existing review ("log B1, B3, and TW1-3", "fix B2 on its branch", "drop CH1", "commit the review"). Keys, IDs, and unambiguous conversational targets are accepted. The whole project, an arbitrary diff, or a path is not a target. A review is never a suggested next move, and the request authorizes no approval, claim, or start.

**Reasoning.** The review covers the node's net change: every non-cancelled unit beneath it, located from in-flight branch heads, or for done work from the recorded branch, `review_ref.head`, or integration commits carrying the unit's ID. Findings are judged against current code and anchored in lines the node's work changed. Bugs are graded blocker, important, medium, or minor by impact, and any violation of an approved acceptance or `preserve` criterion is a blocker; each carries a traced or suspected confidence label. Tweaks, chores, and plan changes (new task, feature, or epic, or a revision of non-terminal work) complete the four numbered sections. Severity is review prose only, never a record field or queue position. Findings matching open shelf work or staged plan changes are marked tracked, earlier declines stay declined, and a returning defect is a regression, never a reopen. The review is read-only, like `design-brief`: no state or Git writes, no fetch, no test runs. It never samples or caps, and lists anything not reviewed under Limitations. Drift, style, accessibility, and gate attestation are out of scope.

**Choreography.** Resolve the target with `show`, `tree`, and `ls`; locate each unit's change with read-only Git; read open shelf work, proposed amendments, and earlier reviews; write `.tiller/reviews/<YYYY-MM-DD>-<node-id>.md` (suffixed, never overwritten); reply with every numbered row in one table. A disposition resolves the whole selection first and writes nothing on any bad row, rechecks only selected rows when pinned code moved, logs bugs, tweaks, and chores through `tiller add ... --attach --source <review block>`, and logs plan changes by landing a distillation of only the selected rows with `tiller intake` and staging one sourced operation each. "fix on its branch" returns unmerged findings to their unit through `execute`, or to the unit's owner. Each disposition annotates its rows in place and makes one commit; nothing is pushed.

**Hand-off.** The review arm stops with the uncommitted file's path; for a feature or task in a project that already declares an optional code review gate, it prints the attest command with the manifest's gate key and leaves the status to the human. The disposition arm stops with the exact `tiller approve bug|chore|tweak <node>` and `tiller approve amendment <key>` commands, run by no one but the human. There is no `code-review` readout seam.
```

- [ ] **Step 3: artifact-schema.md**

In the layout tree, after the two `amendments/` child lines, add:

```text
    reviews/                       # code-review files: prose only, no state, written by the code-review skill
      YYYY-MM-DD-<node-id>.md
```

After the paragraph at line 957 (the `tiller intake` `.tiller/feedback/` thread), add:

```markdown
`.tiller/reviews/` holds code review files, a sibling of `feedback/`. Each is prose the `code-review` skill writes, read by no loader and carrying no state, so the directory needs no schema version. A shelf node logged from a review cites its finding's block as `source`; plan changes logged from a review travel through `tiller intake`, whose landed distillation in `feedback/` is the amendment's fingerprinted source. Review files are committed by the disposition that acts on them, and an uncommitted one appears only in the informational uncommitted-state line of `tiller status`.
```

- [ ] **Step 4: cli-surface.md readout lines**

At `:306`, after "The explicitly requested `design-brief` export is not a lifecycle seam or gate and ends with its derived document instead", change the sentence to cover both: "The explicitly requested `design-brief` export and `code-review` review are not lifecycle seams or gates; each ends with its own deliverable instead". At `:310`, append: "Likewise there is no `code-review` readout seam or review command; `tiller guide code-review` delivers instructions and the agent writes the review file itself."

- [ ] **Step 5: distribution.md routing bullet**

After the design-brief routing line in the orientation summary (`:313`), add:

```markdown
  An explicit request for a code review of one epic, feature, task, bug, chore, or tweak, or naming rows of an existing review, reaches `tiller guide code-review` outside ordinary routing. The review writes only its file under `.tiller/reviews/`; a disposition records through `tiller` commands and one commit and approves nothing.
```

- [ ] **Step 6: tiller-commands.md and the site skills page**

In `docs/tiller-commands.md`, after the design-brief paragraph ending "Bare guide and ordinary orientation retain their existing routing and human gates." add:

```markdown
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
```

At the end of `site/src/content/docs/skills.md`, add:

```markdown
## Request a code review

Ask the agent directly:

> code review feature F-rnte2m

Any epic, feature, task, bug, chore, or tweak works. The agent reviews the work that node delivered, merged and in flight, without changing anything, and writes one file under `.tiller/reviews/`. Its reply is a table of numbered findings: bugs graded blocker, important, medium, or minor; tweaks; chores; and plan changes that would add a task, feature, or epic, or revise unfinished work. Each bug says whether its failing path was traced or only suspected.

Then say what to keep:

> log bugs 1, 3, and 7, chore 2, tweaks 1-3, and plan change 1

Bugs, tweaks, and chores become `proposed` records attached to where they were found, citing the review. Plan changes become one `proposed` amendment. "fix B2 on its branch" sends a finding in unmerged work back to its task, "drop CH1" declines one, and "commit the review" keeps a review that needs nothing logged. Each request is one commit, and every approval stays with you.

```sh
tiller guide code-review
tiller guide code-review --json
```
```

- [ ] **Step 7: STATUS.md snapshot entry**

Add one entry to the current snapshot list in `docs/plans/STATUS.md`, in the style of its neighbors:

```markdown
**October 8 code review skill, source only:** An explicitly requested `code-review` skill reviews one node's net change read-only into `.tiller/reviews/` and logs only the findings the developer names, as `proposed` bugs, tweaks, chores, or one `proposed` amendment. No CLI, schema, or console change; orientation advances to 45 and the catalog to eleven skills. [Design](2026-10-08-code-review-design.md).
```

- [ ] **Step 8: Build the docs site and check versions**

Run: `scripts/check-doc-versions.sh && (cd site && npm run build)`
Expected: `check-doc-versions: every listed doc names 0.17.2 ...` and a successful site build. Then confirm the diff adds no em dash: `git diff | grep '^+' | grep -c $'\u2014'` prints `0`.

- [ ] **Step 9: Commit**

```sh
git add docs/spec docs/tiller-commands.md site/src/content/docs docs/plans/STATUS.md
git commit -s -m "Docs: specify the code-review skill" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Disposable-project trial

The skill's behavior is agent reasoning, which unit tests cannot prove. Controlled agents must consume the candidate binary's generated instructions in a disposable project. This task produces evidence, not code; record findings in the final report, and fix defects in Tasks 1 to 4's files with a new commit each.

**Files:**
- None in the repository. Scratch at `/tmp/tiller-smoke/` (per the `verify` skill; never `~/dev/tiller-test-run`).

**Interfaces:**
- Consumes: the built binary from Tasks 1 to 4.

- [ ] **Step 1: Build and scaffold, following the `verify` skill**

```sh
go build -o /tmp/tiller-smoke/tiller ./cmd/tiller
export PATH="/tmp/tiller-smoke:$PATH"
rm -rf /tmp/tiller-smoke/p && mkdir -p /tmp/tiller-smoke/p && cd /tmp/tiller-smoke/p
git init -q -b main && git config user.email s@t && git config user.name S
tiller init --name "Smoke App" --key smoke --kind greenfield
tiller add contributor jc --name "JC Grubbs"
tiller join --as jc
git add -A && git commit -qm init
test -f .claude/skills/tiller-code-review/SKILL.md && tiller guide code-review | head -3
```

Expected: the shim exists, and `guide` prints `name: code-review` frontmatter.

- [ ] **Step 2: Seed one done task and one in-review task with planted defects**

Follow the `verify` skill's arc ("Decompose, then take the epic gate" through "Merge on a branch, then let sync observe it") with feature `foundation/app-scaffold`, but with two tasks, `wire-the-build` and `add-save`, and a criterion `C1` stating "Saving twice keeps both drafts". On `wire-the-build`'s branch commit `app.py` containing a function that drops every second draft (`drafts = drafts[::2]`), each commit message carrying the task's `T-` ID; submit, merge with `--no-ff`, delete the branch (`git branch -D`), and `tiller sync`. Leave `add-save` started, checked, and submitted on its branch with a planted swallowed exception (`except Exception: pass`), not merged.

Expected: `tiller ls tasks` shows one `done`, one `in_review`, and `git branch` no longer lists `wire-the-build`'s branch.

- [ ] **Step 3: Run the review arm through a fresh agent**

Dispatch a fresh general-purpose subagent with working directory `/tmp/tiller-smoke/p` and PATH including `/tmp/tiller-smoke`, prompt: "Read .tiller/orientation.md, then: code review feature foundation/app-scaffold." Capture its full reply.

Expected:
- Exactly one new untracked file, `.tiller/reviews/<today>-<F-id>.md` (`git status --porcelain` shows only it).
- A blocker bug citing `C1`, found in `wire-the-build`, located even though its branch is gone (through the ID-carrying commit).
- An important or medium bug for the swallowed exception, found in `add-save` marked unmerged.
- The reply holds one table with every numbered row, the review path, and an unmerged note.
- An attest line appears with `--gate code_review` and `--status <pass|fail>` left open, because `tiller init` seeds `code_review` under `gates.optional` (`internal/cli/init.go:484`).
- No `tiller readout` block.

- [ ] **Step 4: Confirm the uncommitted file blocks nothing**

```sh
tiller status
tiller guide
```

Expected: `tiller status` prints `uncommitted tiller state: 1 file(s) under .tiller/; commit when convenient` and exits 0; `tiller guide` routes exactly as before the review.

- [ ] **Step 5: Run each disposition verb**

In the same subagent session (SendMessage to continue it), send in turn, checking `git log -1 --stat` and the review file after each:

1. "log B1 and B9" (assuming fewer than 9 bugs). Expected: nothing written, both problems named (B9 missing).
2. "log B1". Expected: one commit `code-review: <F-id> log B1` holding the review file and `.tiller/bugs/<key>/`; `tiller show bug <key>` shows `proposed`, `attached_to` the done task, `source` `.tiller/reviews/...#L<n>-L<m>`; B1's heading ends `→ B-...`; no other line in the file moved.
3. "log B1" again. Expected: refused as already logged, nothing written.
4. "fix B2 on its branch" (the agent acts as `jc`, the owner). Expected: annotation commit first, then execute's requested-changes path on `add-save`'s branch. Stop the subagent before it merges anything.
5. If the review has a plan change, "log P1"; otherwise "log CH1 as a plan change". Expected: `.tiller/feedback/<date>-<F-id>-plan-changes.md` holding only that row, a `proposed` amendment with one operation whose `source` cites the feedback copy, and the review row annotated `→ amendment <key>`.
6. "drop" the next open row, then "commit the review". Expected: two commits, the row annotated `declined`.

- [ ] **Step 6: Re-run the review on the same day**

Ask the subagent to "code review feature foundation/app-scaffold" again. Expected: a `-2` file; the logged bug appears under Already tracked, the dropped row under Previously declined; neither is renumbered.

- [ ] **Step 7: Report**

Write the trial results to the final report for JC: every expected outcome with pass or fail and the observed evidence (commands and output excerpts). For each failure, fix the skill text (Tasks 1 to 3 files), add a pinning test where the defect is a missing instruction, rerun `make test`, commit with `Fix: <what> (code-review trial)`, and repeat the failing step.

---

## After the build

Not tasks for the executor. When JC rules: rebase onto current main and apply the generation rule; run `make lint`, `make test`, and `cd site && npm run build`; fast-forward main only on JC's word. At release or distribution time, fold this design's durable decisions into `docs/project-history.md` and remove the design and plan from the working tree per AGENTS.md.
