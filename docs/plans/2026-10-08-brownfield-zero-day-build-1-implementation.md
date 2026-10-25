# Brownfield zero day, build 1: route order (#137) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On a tree with no epics, owned shelf work, a proposed amendment, and a shelf merge wait route ahead of `decompose`, and every surface that states the route order says so.

**Architecture:** `views.RouteDecisionOf` is the one shared route; `guide`, every readout's Next, and the console's `/api/route` all project it. The fix moves its `len(t.Epics) == 0` arm from second place (right after the baseline gate) to last place (right before the caught-up terminal). Every arm in between already iterates only over recorded nodes, so on an epic-less tree each one either fires for real shelf or amendment state or falls through. The generated orientation and the orient skill restate the order, so they move with it, and the orientation rules change bumps the generation from 44 to 45.

**Tech Stack:** Go 1.x, testify `require`, the repo's `make test` / `make lint`.

**Spec:** `docs/plans/2026-10-08-brownfield-zero-day-design.md` (section "The route", item 2, and "Build sequence", build 1). Issue: https://github.com/madeintandem/tiller/issues/137.

## Global Constraints

- The empty-tree arm keeps its exact output: `Seam: "decompose"`, `Reason: "no epics recorded yet"`, no node. Existing pins in `internal/views/readout_test.go:2366`, `internal/serve/route_test.go:325`, and `internal/cli/readout_e2e_test.go:312` must pass unchanged.
- An unclaimed shelf node never enters the route (skill-set.md, orient). An epic-less tree whose only shelf work is unowned still routes to `decompose`.
- The baseline arm stays first: a brownfield or legacy-migration project with no confirmed baseline doc names `derive-baseline` regardless of shelf work.
- Orientation generation becomes 45. `internal/gen/orientation.go`'s comment says "Increment it when the rules change, independently of binary patch versions."
- No version stamp, schema change, release, install, push, or managed-project re-join in this build.
- House style for prose: no em dashes (`grep -n "—"` must find none in changed prose).
- Commits are signed off (`git commit -s`) and end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Work happens in the worktree `/Users/jcgrubbs/dev/tiller/.claude/worktrees/brownfield-zero-day` on branch `brownfield-zero-day`. Never touch `~/dev/tiller-test-run` or any managed project.

## Review Focus

- **Owned shelf work in `check` on an epic-less tree.** A reasonable person expects the checked bug to route to `execute` with the "complete any outstanding checks" reason, exactly as on a tree with epics. Pinned in Task 1.
- **Shelf work owned by someone else.** Reader `jc`, bug owned by `al`, no epics: expect `decompose`, not `execute` and not caught-up. Pinned in Task 1.
- **Approved and withdrawn amendments on an empty tree.** Only a proposed amendment is a wait; an approved one must not displace `decompose`. Pinned in Task 1.
- **A dangling carrier on an empty tree.** A carrier is always an epic, feature, or task, so with no epics a `resolved_by` pointer can only name something gone. `gen.WorkActivity` reads that as `carrier_cancelled`, which returns owned work to its shelf route, so the bug must route to `execute`, not hide behind `decompose`. Pinned in Task 1.
- **Guide and readout agreeing.** The real CLI surfaces must name the bug, not decompose, after `approve` and `claim` on an epic-less tree. Pinned in Task 2's CLI test and Task 4's end-to-end run.

---

### Task 1: Move the empty-tree arm to the end of the route

**Files:**
- Modify: `internal/views/readout.go` (the `len(t.Epics) == 0` block in `RouteDecisionOf`, currently lines 1329-1331, and the final `return` near line 1505)
- Test: `internal/views/workitem_route_test.go` (shelf cases)
- Test: `internal/views/readout_test.go` (amendment cases, beside `TestAmendmentWaitIgnoresApprovedAndWithdrawnAmendments`)

**Interfaces:**
- Consumes: `views.RouteDecisionOf(t *store.Tree, order *gen.Order, owner string, in workflow.Input) RouteDecision`, existing fixture `shelfRouteTree() *store.Tree`, existing fixture `treeWithDraftEpics(t *testing.T, n int) *store.Tree`.
- Produces: a new test fixture `epiclessShelfTree() *store.Tree` in `workitem_route_test.go` (package `views_test`). No production signature changes.

- [ ] **Step 1: Write the failing shelf tests**

Append to `internal/views/workitem_route_test.go`:

```go
// epiclessShelfTree is shelfRouteTree with its epic and feature removed: the
// state of a brownfield project the day its baseline is confirmed and a bug
// is captured, before any epic exists (issue #137).
func epiclessShelfTree() *store.Tree {
	tree := shelfRouteTree()
	tree.Epics = map[string]*store.Epic{}
	tree.Features = map[string]*store.Feature{}
	return tree
}

func TestEpiclessTreeRoutesAnOwnedReadyBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "execute", decision.Seam)
	require.Equal(t, "B-aaaaaa", nodeID(decision.Node))
}

func TestEpiclessTreeRoutesAnOwnedCheckedBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.Bugs["B-aaaaaa"].Status = "check"
	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "execute", decision.Seam)
	require.Equal(t, "B-aaaaaa", nodeID(decision.Node))
	require.Contains(t, decision.Reason, "complete any outstanding checks")
}

func TestEpiclessTreeNamesAnInReviewBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.Bugs["B-aaaaaa"].Status = "in_review"
	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Empty(t, decision.Seam)
	require.Equal(t, "B-aaaaaa", nodeID(decision.Node))
	require.Contains(t, decision.Reason, "waiting on a merge")
}

// Unclaimed and other-owned shelf work never enters the route, so it cannot
// displace the empty-tree arm.
func TestEpiclessTreeWithOnlyOthersShelfWorkStillRoutesDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.Bugs["B-aaaaaa"].Owner = "al"
	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "decompose", decision.Seam)
	require.Equal(t, "no epics recorded yet", decision.Reason)
	require.Nil(t, decision.Node)
	require.False(t, decision.CaughtUp)
}

// A carrier is always an epic, feature, or task, so on a tree with no epics a
// resolved_by pointer can only dangle. gen.WorkActivity reads a carrier the
// tree no longer holds as carrier_cancelled, which returns owned work to its
// own shelf route: the bug executes rather than hiding behind decompose.
func TestEpiclessTreeRoutesAnOwnedBugWhoseCarrierIsGone(t *testing.T) {
	tree := epiclessShelfTree()
	tree.Bugs["B-aaaaaa"].ResolvedBy = "F-aaaaaa"
	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "execute", decision.Seam)
	require.Equal(t, "B-aaaaaa", nodeID(decision.Node))
}
```

- [ ] **Step 2: Write the failing amendment tests**

Append to `internal/views/readout_test.go`, directly after `TestAmendmentWaitIgnoresApprovedAndWithdrawnAmendments`:

```go
// TestEpiclessTreeNamesTheAmendmentWaitBeforeDecompose pins issue #137's
// amendment half: an intake amendment staged against an empty tree is a
// human wait the route must surface, not hide behind decompose.
func TestEpiclessTreeNamesTheAmendmentWaitBeforeDecompose(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("first-intake"), Status: store.AmendmentStatusProposed},
	}

	decision := views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "", decision.Seam)
	require.Contains(t, decision.Reason, "first-intake (A-aaaaaa)")
	require.Contains(t, decision.Reason, "awaits your approval")
}

func TestEpiclessTreeIgnoresSettledAmendmentsAndRoutesDecompose(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("approved-one"), Status: store.AmendmentStatusApproved},
		"A-bbbbbb": {ID: "A-bbbbbb", Key: strPtr("withdrawn-one"), Status: store.AmendmentStatusWithdrawn},
	}

	seam, reason := views.Route(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
	require.Equal(t, "decompose", seam)
	require.Equal(t, "no epics recorded yet", reason)
}
```

- [ ] **Step 3: Run the new tests and confirm the right ones fail**

Run: `go test ./internal/views/ -run 'TestEpicless' -count=1 -v`

Expected: `TestEpiclessTreeRoutesAnOwnedReadyBugBeforeDecompose`, `TestEpiclessTreeRoutesAnOwnedCheckedBugBeforeDecompose`, `TestEpiclessTreeNamesAnInReviewBugBeforeDecompose`, `TestEpiclessTreeRoutesAnOwnedBugWhoseCarrierIsGone`, and `TestEpiclessTreeNamesTheAmendmentWaitBeforeDecompose` FAIL with seam `decompose` where `execute` or `""` was expected. `TestEpiclessTreeWithOnlyOthersShelfWorkStillRoutesDecompose` and `TestEpiclessTreeIgnoresSettledAmendmentsAndRoutesDecompose` PASS already; they pin behavior that must survive the move.

- [ ] **Step 4: Move the arm**

In `internal/views/readout.go`, delete this block from the top of `RouteDecisionOf` (right after the `baselineRequired` block):

```go
	if len(t.Epics) == 0 {
		return RouteDecision{Seam: "decompose", Reason: "no epics recorded yet"}
	}
```

Then, at the bottom of `RouteDecisionOf`, insert the arm between the `frontierBlocked` block and the final caught-up `return`:

```go
	if feature, dependency := frontierBlocked(t, order); feature != nil {
		return RouteDecision{
			Reason: fmt.Sprintf("the frontier waits behind work in flight: %s is blocked by %s", gen.Ref(t, feature), dependency),
			Node:   feature, SkippedQuestionHold: skipped,
		}
	}

	// The empty-tree arm runs last (issue #137). Every earlier arm that can
	// apply to a tree with no epics, owned shelf work, a proposed amendment,
	// or a shelf merge wait, is real work or a real wait and outranks
	// planning; every other arm iterates recorded tree nodes and falls
	// through. Only then does an epic-less tree mean decomposition is next.
	if len(t.Epics) == 0 {
		return RouteDecision{Seam: "decompose", Reason: "no epics recorded yet"}
	}

	return RouteDecision{
		Reason: noRouteDue, CaughtUp: true, SkippedQuestionHold: skipped,
	}
```

Update the doc comment above `RouteDecisionOf` only if it states the old order; it currently does not.

- [ ] **Step 5: Run the views package**

Run: `go test ./internal/views/ -count=1`
Expected: PASS, including the pre-existing `TestRouteTargetNamesNoNodeForAnEmptyTree` and `TestRouteEqualsRouteTargetsSeamAndReason`.

- [ ] **Step 6: Commit**

```bash
git add internal/views/readout.go internal/views/workitem_route_test.go internal/views/readout_test.go
git commit -s -m "Route: let shelf work and amendments outrank the empty-tree arm

On a tree with no epics, owned shelf work, a proposed amendment, and a
shelf merge wait now route before decompose. The empty-tree arm moves
from second place to last, just ahead of caught-up. Refs #137.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Pin the CLI surface with a guide test

**Files:**
- Test: `internal/cli/guide_test.go` (beside `TestGuideNoArgSuggestsDecomposeOnFreshRepo`)

**Interfaces:**
- Consumes: Task 1's route change; test helpers `layBugCLI(t)` (`internal/cli/lifecycle_cmds_test.go:2149`, which runs `initOwnedRepo`, adds bug `alpha`, writes its `bug.md`, and commits), `mustRun(t, args...)`, `execCLI(t, args...) (stdout, stderr string, code int)`.
- Produces: nothing later tasks consume.

- [ ] **Step 1: Write the test**

```go
// TestGuideRoutesAnOwnedBugOnAnEmptyTree is issue #137 at the CLI: a
// maintenance-only project with no epics and a claimed, approved bug gets
// execute from guide, not decompose.
func TestGuideRoutesAnOwnedBugOnAnEmptyTree(t *testing.T) {
	layBugCLI(t)
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "jc")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, `"skill": "execute"`)
	require.Contains(t, out, "bugs/alpha")
	require.NotContains(t, out, `"skill": "decompose"`)
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/cli/ -run 'TestGuideRoutesAnOwnedBugOnAnEmptyTree|TestGuideNoArgSuggestsDecomposeOnFreshRepo' -count=1 -v`
Expected: both PASS. To confirm the new test guards the fix, temporarily restore the old early arm from Task 1 Step 4 in `readout.go`, rerun, and see `TestGuideRoutesAnOwnedBugOnAnEmptyTree` FAIL with `"skill": "decompose"`; then revert that temporary change (`git checkout internal/views/readout.go`) and rerun to PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/guide_test.go
git commit -s -m "Test: pin guide routing an owned bug on an empty tree

Refs #137.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Restate the order in orientation, the orient skill, and the specs; bump the generation

**Files:**
- Modify: `internal/gen/orientation.go` (constant and comment at lines 18-26; routing list at lines 684-800)
- Modify: `internal/skills/orient/SKILL.md` (line 63 bullet)
- Modify: `docs/spec/cli-surface.md:849`
- Modify: `docs/spec/skill-set.md:5` and `:128`
- Modify: every current-source "generation 44" mention: `docs/plans/STATUS.md:3`, `docs/tiller-commands.md:7`, `docs/spec/distribution.md:5` and `:397`, `site/src/content/docs/source-quickstart.md:164`, `site/src/content/docs/skills.md:15`, `site/src/content/docs/concepts.md:439`
- Test: existing `internal/gen/orientation_test.go`, `internal/cli/join_test.go`, `internal/cli/migrate_test.go` (they compare against `gen.Orientation()` and the constant, so they need no edits)

**Interfaces:**
- Consumes: Task 1's route order.
- Produces: `orientationSchemaGen = 45`.

- [ ] **Step 1: Bump the generation**

In `internal/gen/orientation.go`, replace:

```go
// Generation 44 adds explicitly authorized historical aggregate review recovery.
const orientationSchemaGen = 44
```

with:

```go
// Generation 45 moves the empty-tree decompose rule below every rule that can
// apply to a tree with no epics (issue #137).
const orientationSchemaGen = 45
```

- [ ] **Step 2: Rewrite the routing list**

In the orientation body, delete rule 2:

```
2. No epics recorded: the seam is ` + "`decompose`" + `.
```

Renumber the following rules down by one: 3 becomes 2, 4 becomes 3, and so on through 10 becoming 9. Update the two in-text cross-references to match: `Item 3 derives the remaining work` becomes `Item 2 derives the remaining work`, and `Item 5 names an eligible task before this terminal` becomes `Item 4 names an eligible task before this terminal`. Rule 1's `before empty-tree or existing-work routing` stays as written.

Then, in the last rule (old 10, now 9), insert the empty-tree rule between its blocked-frontier sentence and its caught-up sentence. The rule's opening currently reads:

```
10. Drafts exist but every one waits on unfinished dependencies: a blocked
   frontier is a named wait, not an empty tree. Surface what it waits on and
   stop. Otherwise this is the true caught-up terminal: no eligible unclaimed
```

It becomes:

```
9. Drafts exist but every one waits on unfinished dependencies: a blocked
   frontier is a named wait, not an empty tree. Surface what it waits on and
   stop. Otherwise, with no epics recorded, the seam is ` + "`decompose`" + `.
   Every rule above that can apply to a tree with no epics routes first: owned
   shelf work, a proposed amendment, and a shelf merge wait are real work or a
   real wait, while unclaimed shelf work never enters the route.
   Otherwise this is the true caught-up terminal: no eligible unclaimed
```

Leave the rest of that rule's text unchanged. Search the body for any other `rule N` or `item N` references with `grep -n -E "[Ii]tem [0-9]|[Rr]ule [0-9]" internal/gen/orientation.go` and fix any that still point at the old numbers.

- [ ] **Step 3: Update the orient skill**

In `internal/skills/orient/SKILL.md`, delete the bullet at line 63:

```
- No epics are recorded, or inputs exist under `.tiller/design/` and the tree does not yet reflect them, the seam is `decompose`, after the existing-code baseline arm above.
```

and insert this bullet immediately before the bullet that begins `- Nothing is on the frontier or in the executable work loop:`:

```
- No epics are recorded and nothing above routes, or inputs exist under `.tiller/design/` and the tree does not yet reflect them: the seam is `decompose`. On a tree with no epics, owned shelf work, a proposed amendment, and a shelf merge wait still route first; unclaimed shelf work never enters the route.
```

- [ ] **Step 4: Update the specs**

`docs/spec/cli-surface.md:849`: replace the sentence

```
After baseline/no-epics startup, route assigned actionable tasks/shelves with checked priority, amendment review, shared closeout, eligible unclaimed ready tasks, unrelated approval waits, then draft frontier.
```

with

```
After the baseline startup arm, route assigned actionable tasks/shelves with checked priority, amendment review, shared closeout, eligible unclaimed ready tasks, unrelated approval waits, then draft frontier, merge waits, and a blocked frontier. Only when none of those applies does a tree with no epics route to `decompose`, ahead of the caught-up terminal; a maintenance-only project with owned shelf work or a proposed amendment is never told to decompose.
```

`docs/spec/skill-set.md:128`: replace the opening `Before the no-epics arm, the shared route checks` with `First, the shared route checks`, and append this sentence to the end of that paragraph:

```
The no-epics arm comes last, just before caught-up: owned shelf work, a proposed amendment, and a shelf merge wait on a tree with no epics route first.
```

`docs/spec/skill-set.md:5`: change `orientation generation 44` to `orientation generation 45`.

- [ ] **Step 5: Update the current-source generation mentions**

For each file below, open the cited line and change the current-source generation from 44 to 45. Do not change historical statements (lines that say what an older release or generation introduced).

- `docs/plans/STATUS.md:3`: `orientation generation **44**` becomes `**45**`. Line 29's "Schema, orientation generation 44, and skills are unchanged" describes the October 8 fixes and stays.
- `docs/tiller-commands.md:7`.
- `docs/spec/distribution.md:5` and `:397`. Line 226 describes what generation 44 added; append one sentence after it: `Orientation generation 45 moves the empty-tree decompose rule below every rule that can apply to a tree with no epics.`
- `site/src/content/docs/source-quickstart.md:164`, `site/src/content/docs/skills.md:15`.
- `site/src/content/docs/concepts.md:439`: read the sentence first; change only the current-generation number.

`docs/tiller-commands.md` feeds the site's CLI page through `site/scripts/sync-docs.mjs` at site build time; that page is not committed, so nothing else needs regenerating.

Then run: `git grep -n "generation 44\|generation \*\*44" -- docs site internal`
Expected: only historical mentions remain (STATUS.md:29 and distribution.md:226's original sentence).

- [ ] **Step 6: Run the gen, skills, and CLI packages**

Run: `go test ./internal/gen/ ./internal/skills/ ./internal/cli/ -count=1`
Expected: PASS. Join and migrate tests regenerate orientation from `gen.Orientation()` and compare, so they pass with the new text.

- [ ] **Step 7: Check prose style**

Run: `git diff -U0 | grep "^+" | grep -n "—"`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add internal/gen/orientation.go internal/skills/orient/SKILL.md docs/spec/cli-surface.md docs/spec/skill-set.md docs/spec/distribution.md docs/plans/STATUS.md docs/tiller-commands.md site/src/content/docs
git commit -s -m "Orientation: state the empty-tree rule last; generation 45

Orientation, the orient skill, and the specs now say what the route
does: owned shelf work, a proposed amendment, and a shelf merge wait
outrank decompose on a tree with no epics. Refs #137.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Full suite, lint, and the real surfaces

**Files:**
- Modify: `docs/project-history.md` (one entry), `docs/plans/STATUS.md` (one sentence)

**Interfaces:**
- Consumes: Tasks 1 to 3.
- Produces: the verified branch.

- [ ] **Step 1: Full suite and lint**

Run: `make test && make lint`
Expected: PASS with no gofmt findings.

- [ ] **Step 2: Drive the real binary (use the repo's `verify` skill for the scaffold pattern)**

```bash
go build -o /tmp/tiller-smoke/tiller ./cmd/tiller
export PATH="/tmp/tiller-smoke:$PATH"
rm -rf /tmp/tiller-smoke/p && mkdir -p /tmp/tiller-smoke/p && cd /tmp/tiller-smoke/p
git init -q -b main && git config user.email s@t && git config user.name S
tiller init --name "Smoke App" --key smoke --kind greenfield
tiller add contributor jc --name "JC Grubbs"
tiller join --as jc
git add -A && git commit -qm init

tiller guide | head -3            # expect: Suggested skill: decompose (empty tree, nothing owned)

tiller add bug blank-save --title "Blank save" --contributor jc
printf '# Blank save\n\n## Reproduction\n\nSave an empty note.\n\n## Expected\n\nNothing is written.\n\n## Actual\n\nAn empty file is written.\n\n## Fixed when\n\nSaving an empty note writes no file.\n' > .tiller/bugs/blank-save/bug.md
git add -A && git commit -qm bug
tiller guide | head -3            # expect: still decompose (the bug is proposed and unclaimed)
tiller approve bug blank-save --contributor jc
tiller claim bug blank-save --contributor jc
git add -A && git commit -qm claim
tiller guide | head -3            # expect: Suggested skill: execute, naming bugs/blank-save
tiller readout orient | grep -E "Route|Next"   # expect: execute on bugs/blank-save in both lines
```

Before this build, the last two commands printed `Suggested skill: decompose` and `**Next:** decompose: no epics recorded yet.` (observed 2026-10-08 on the 0.17.2 source). Record the observed output of all four checks in the project-history entry.

- [ ] **Step 3: Record the outcome**

Add a short entry to `docs/project-history.md` in the style of its neighbors: what #137 was, the move of the empty-tree arm to last place, generation 45, and the observed before/after from Step 2. In `docs/plans/STATUS.md`, add one sentence noting the source carries the #137 route-order fix at generation 45, unreleased. Do not edit any version stamp.

- [ ] **Step 4: Commit**

```bash
git add docs/project-history.md docs/plans/STATUS.md
git commit -s -m "Docs: record the #137 route-order fix

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Do not push, close #137, merge to main, stamp a version, or install the binary. Those wait for JC.
