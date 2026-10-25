# Brownfield zero day, build 2: to-be inputs and the no-work-planned end state Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A tree with no epics routes to `decompose` only when a to-be design doc exists; with none, every surface says no work is planned and names the ways work enters, instead of suggesting decompose.

**Architecture:** Tree load gains `Tree.ToBeDesign`, the sorted Markdown paths under `.tiller/design/` that `baseline.yml` does not register, gathered through the tree's confined reader. Build 1 left the empty-tree arm as the last arm of `views.RouteDecisionOf`; it now splits in a new `internal/views/zero_day.go`: `decompose` with a reason naming the to-be docs, or a caught-up decision with a new `Unplanned` flag and a kind-specific human reason. The readout's Next renders that reason, `guide` already prints the route's reason for seamless answers, and `check readiness` stops hard-coding decompose and prints guide's suggestion. Orientation, the orient skill, and the specs restate the rule; generation 45 becomes 46.

**Tech Stack:** Go, testify `require`, `make test` / `make lint`.

**Spec:** `docs/plans/2026-10-08-brownfield-zero-day-design.md`, sections "The route", "The end state", and "Build sequence" build 2. Builds on build 1 (#137), already on this branch.

## Global Constraints

- Decisions 2 and 5 of the design doc bind: decompose is due only on an empty tree with at least one to-be doc; the empty end state is a second flavor of caught-up, human-owned, no seam, the agent stops; `guide`, the readout's Next, and `check readiness` take their text from the route.
- The same rule applies to every project kind; only the end-state wording differs (brownfield and legacy-migration versus greenfield).
- A non-empty tree never consults `ToBeDesign`.
- The open-question clause appears only when the count is nonzero and names the count only. Count with `gen.QuestionOpen`, the predicate the decompose readout's `Open questions` check already uses.
- Tree load must not fail because of anything under `.tiller/design/`: an unreadable, symlinked, or otherwise unsafe directory or file contributes no to-be doc.
- Orientation generation becomes 46.
- No version stamp, schema change, release, install, push, merge, or managed-project re-join.
- Prose: no em dashes. Commits: `git commit -s`, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Work in the worktree `/Users/jcgrubbs/dev/tiller/.claude/worktrees/brownfield-zero-day`, branch `brownfield-zero-day`. Never touch `~/dev/tiller-test-run` or a managed project.

## Refinements to the design (for JC to confirm at review)

- **To-be docs are Markdown only.** The design says "any file under `.tiller/design/`". `store.ValidateDesignPath` already requires design docs to be canonical `.md` paths, so an image or PDF beside a doc does not make decompose due on its own. Dotfiles and dot-directories are ignored.
- **The console needs no change.** Its route strip already renders the route's reason beside the label; for the end state it shows "Nothing due" with the no-work-planned reason. A dedicated label is out of scope.

## Review Focus

- **A design dir holding only `baseline.yml` and registered as-is docs, some still unconfirmed.** Pending docs are registered, so they are never to-be docs; with one confirmed doc the route reaches the end state, not decompose. Pinned in Task 1 (store) and Task 2 (route).
- **Nested and oddly named to-be docs** (`plans/next/sync.md`, `README.MD`, `.draft.md`, `notes.txt`). Nested and upper-case `.MD` count; dotfiles and non-Markdown do not. Pinned in Task 1.
- **A symlinked `.tiller/design` subdirectory or file.** Ignored, and tree load still succeeds. Pinned in Task 1.
- **Open questions on an empty tree.** The end state names the count with correct singular/plural, and omits the clause at zero. Pinned in Task 2.
- **`check readiness` on a ready project with owned work.** It prints guide's suggestion (execute on the owned bug), not decompose. Pinned in Task 3.

---

### Task 1: List to-be design docs at tree load

**Files:**
- Create: `internal/store/tobe_design.go`
- Modify: `internal/store/tree.go` (the `Tree` struct near line 59; `loadTreeWithManifest` right after `t.Baseline, err = LoadBaseline(root)`)
- Test: `internal/store/tobe_design_test.go`

**Interfaces:**
- Consumes: `t.stateDirectory(path string) ([]fs.DirEntry, error)` (`internal/store/repository_reader.go:110`), `ValidateDesignPath(p string) error` (`internal/store/baseline.go`), `t.Baseline.Docs`.
- Produces: `Tree.ToBeDesign []string`: sorted, slash-separated paths relative to `.tiller/design/`, each passing `ValidateDesignPath`, none registered in `t.Baseline.Docs`. Nil when there are none.

- [ ] **Step 1: Write the failing tests**

Create `internal/store/tobe_design_test.go`:

```go
package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDesign(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "design", filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLoadListsUnregisteredMarkdownAsToBeDesign(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	writeDesign(t, root, "plans/next/sync.md", "# Sync\n")
	writeDesign(t, root, "README.MD", "# Readme\n")
	writeDesign(t, root, ".draft.md", "# Draft\n")
	writeDesign(t, root, ".hidden/idea.md", "# Idea\n")
	writeDesign(t, root, "notes.txt", "notes\n")
	writeDesign(t, root, "diagram.png", "png")

	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, []string{"00-index.md", "README.MD", "plans/next/sync.md"}, tree.ToBeDesign)
}

func TestLoadExcludesRegisteredDocsFromToBeDesign(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	writeDesign(t, root, "modules/api.md", "# API\n")       // registered and confirmed in validBaseline
	writeDesign(t, root, "baseline.yml", validBaseline+`  - path: modules/pending.md
    describes: [app]
`)
	writeDesign(t, root, "modules/pending.md", "# Pending\n") // registered, unconfirmed

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign, "as-is docs, confirmed or pending, are never to-be inputs")
}

func TestLoadIgnoresSymlinkedDesignEntries(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "00-index.md")))
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "dir", "escape.md"), []byte("# x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.md"), []byte("# x\n"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, ".tiller", "design", "linked")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "file.md"), filepath.Join(root, ".tiller", "design", "linked.md")))

	tree, err := Load(root)
	require.NoError(t, err, "nothing under design/ may fail the tree load")
	require.Nil(t, tree.ToBeDesign)
}

func TestLoadWithoutADesignDirHasNoToBeDesign(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "design")))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.ToBeDesign)
}
```

`validBaseline` is the constant in `internal/store/baseline_test.go` (same package). Its doc `modules/api.md` describes `app`; `ParseBaseline` validates describes paths only as coordinates, so no `app` directory is needed. If `Load` refuses the baseline for a reason unrelated to this task (for example a missing described path check added since), create `root/app/` and retry rather than changing the baseline.

- [ ] **Step 2: Run them to confirm they fail**

Run: `go test ./internal/store/ -run 'ToBeDesign|DesignEntries' -count=1`
Expected: compile failure, `tree.ToBeDesign undefined`.

- [ ] **Step 3: Add the field**

In `internal/store/tree.go`, directly below the `Baseline BaselineLedger` field and its comment, add:

```go
	// ToBeDesign lists the Markdown design inputs under .tiller/design/ that
	// the baseline ledger does not register: the to-be docs whose presence
	// makes decompose due on a tree with no epics. Sorted, slash-separated,
	// relative to .tiller/design/; nil when there are none.
	ToBeDesign []string
```

- [ ] **Step 4: Implement the listing**

Create `internal/store/tobe_design.go`:

```go
package store

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// loadToBeDesign gathers Tree.ToBeDesign through the tree's confined reader.
// It never fails the load: the result only decides whether decompose is due,
// so a directory that cannot be listed, a symlink, a dotfile, or a
// non-Markdown file simply contributes no to-be doc.
func (t *Tree) loadToBeDesign() {
	registered := make(map[string]bool, len(t.Baseline.Docs))
	for _, doc := range t.Baseline.Docs {
		registered[doc.Path] = true
	}
	var found []string
	var walk func(rel string)
	walk = func(rel string) {
		entries, err := t.stateDirectory(filepath.Join(t.Root, ".tiller", "design", filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			child := path.Join(rel, name)
			switch {
			case e.Type()&fs.ModeSymlink != 0:
				continue
			case e.IsDir():
				walk(child)
			case e.Type().IsRegular():
				if ValidateDesignPath(child) == nil && !registered[child] {
					found = append(found, child)
				}
			}
		}
	}
	walk(".")
	sort.Strings(found)
	t.ToBeDesign = found
}
```

`path.Join(".", name)` returns `name`, so top-level docs have no `./` prefix. In `loadTreeWithManifest`, immediately after the `LoadBaseline` error check, add:

```go
	t.loadToBeDesign()
```

It must run while `t.reader` is still set (before the deferred `t.reader = nil`), which this placement guarantees.

- [ ] **Step 5: Run the store package**

Run: `go test ./internal/store/ -count=1`
Expected: PASS. `TestLoad_FixtureTree` is unaffected (it does not assert on the new field).

- [ ] **Step 6: Commit**

```bash
git add internal/store/tree.go internal/store/tobe_design.go internal/store/tobe_design_test.go
git commit -s -m "Store: list unregistered design docs as to-be inputs

Tree load records the Markdown files under .tiller/design/ that the
baseline ledger does not register, through the confined reader, never
failing the load. The route uses them to decide whether decompose is due.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Split the empty-tree arm and render the end state

**Files:**
- Create: `internal/views/zero_day.go`
- Modify: `internal/views/readout.go` (the `RouteDecision` struct near line 1270; the empty-tree arm at the end of `RouteDecisionOf` near line 1506; the `CaughtUp` branch of `nextAction` near line 1843)
- Test: `internal/views/zero_day_test.go` (new); existing `internal/views/readout_test.go`, `internal/views/workitem_route_test.go`, `internal/views/baseline_test.go`

**Interfaces:**
- Consumes: `Tree.ToBeDesign` (Task 1), `gen.QuestionOpen(q *store.Question) bool`, `plural(n int, singular, many string) string` (`readout.go:597`), `store.KindBrownfield`, `store.KindLegacyMigration`.
- Produces: `RouteDecision.Unplanned bool`; unexported `emptyTreeRoute(t *store.Tree) RouteDecision`. Reason strings:
  - decompose: `no epics recorded yet; N to-be design doc(s) await(s) decomposition, starting with .tiller/design/<first>`
  - existing code: `no work is planned yet (baseline established[, N open question(s)]): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)`
  - greenfield: `no work is planned yet[ (N open question(s))]: add design inputs under .tiller/design/ for decompose to turn into a draft tree`

- [ ] **Step 1: Write the failing route tests**

Create `internal/views/zero_day_test.go`:

```go
package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// confirmedBrownfield is the trial's day-one state: one confirmed as-is doc,
// one still pending, no epics, no to-be docs.
func confirmedBrownfield(t *testing.T) *store.Tree {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindBrownfield
	tree.Baseline.Docs = []store.BaselineDoc{
		{Path: "as-is/app.md", Describes: []string{"."}, Confirmed: baselineConfirmation()},
		{Path: "as-is/store.md", Describes: []string{"."}},
	}
	return tree
}

func route(tree *store.Tree) views.RouteDecision {
	return views.RouteDecisionOf(tree, gen.DependencyOrder(tree), "jc", workflow.Input{})
}

func TestConfirmedBaselineWithNoToBeDocIsUnplanned(t *testing.T) {
	decision := route(confirmedBrownfield(t))
	require.Empty(t, decision.Seam)
	require.True(t, decision.CaughtUp)
	require.True(t, decision.Unplanned)
	require.Nil(t, decision.Node)
	require.Equal(t, "no work is planned yet (baseline established): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)", decision.Reason)
}

func TestLegacyMigrationUsesTheExistingCodeWording(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.Manifest.Project.Kind = store.KindLegacyMigration
	require.Contains(t, route(tree).Reason, "(baseline established)")
}

func TestUnplannedNamesTheOpenQuestionCount(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.Questions = map[string]*store.Question{
		"Q-aaaaaa": {ID: "Q-aaaaaa", Status: "open"},
	}
	require.Contains(t, route(tree).Reason, "(baseline established, 1 open question)")
	tree.Questions["Q-bbbbbb"] = &store.Question{ID: "Q-bbbbbb", Status: "open"}
	require.Contains(t, route(tree).Reason, "(baseline established, 2 open questions)")
	tree.Questions["Q-bbbbbb"].Status = "answered"
	require.Contains(t, route(tree).Reason, "(baseline established, 1 open question)")
}

func TestOneToBeDocMakesDecomposeDue(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.ToBeDesign = []string{"sync.md"}
	decision := route(tree)
	require.Equal(t, "decompose", decision.Seam)
	require.False(t, decision.CaughtUp)
	require.False(t, decision.Unplanned)
	require.Equal(t, "no epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/sync.md", decision.Reason)

	tree.ToBeDesign = []string{"a.md", "b.md"}
	require.Equal(t, "no epics recorded yet; 2 to-be design docs await decomposition, starting with .tiller/design/a.md", route(tree).Reason)
}

func TestGreenfieldWithNoDesignIsUnplanned(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindGreenfield
	decision := route(tree)
	require.True(t, decision.Unplanned)
	require.Equal(t, "no work is planned yet: add design inputs under .tiller/design/ for decompose to turn into a draft tree", decision.Reason)

	tree.Questions = map[string]*store.Question{"Q-aaaaaa": {ID: "Q-aaaaaa", Status: "open"}}
	require.Equal(t, "no work is planned yet (1 open question): add design inputs under .tiller/design/ for decompose to turn into a draft tree", route(tree).Reason)
}

// A non-empty tree never consults ToBeDesign: with epics, the normal route
// runs and the end state never appears.
func TestATreeWithEpicsIgnoresToBeDesign(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.ToBeDesign = []string{"sync.md"}
	decision := route(tree)
	require.NotEqual(t, "decompose", decision.Seam)
	require.False(t, decision.Unplanned)
}

func TestUnplannedNextIsTheHumansReason(t *testing.T) {
	tree := confirmedBrownfield(t)
	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc"})
	require.Len(t, r.Next, 1)
	require.Equal(t, "human", r.Next[0].Owner)
	require.Equal(t, route(tree).Reason, r.Next[0].Text)
}
```

Notes for the implementer (verified while planning):
- `baselineConfirmation()` is defined in `internal/views/baseline_test.go:17`, same `views_test` package, so it is visible here.
- `store.KindGreenfield`, `store.KindBrownfield`, and `store.KindLegacyMigration` are the kind constants (`internal/store/manifest.go:78`).
- `gen.QuestionOpen` (`internal/gen/question_blockers.go:10`) treats `answered`, `resolved`, and `cancelled` as closed and every other status as open.

- [ ] **Step 2: Run them to confirm they fail**

Run: `go test ./internal/views/ -run 'Unplanned|ToBeDoc|ToBeDesign|LegacyMigrationUses|GreenfieldWithNoDesign' -count=1`
Expected: compile failure on `decision.Unplanned` and `tree.ToBeDesign` (the latter compiles once Task 1 is in; `Unplanned` is the missing field).

- [ ] **Step 3: Add the flag**

In `internal/views/readout.go`, add to `RouteDecision` directly after `CaughtUp bool`:

```go
	// Unplanned marks the caught-up flavor reached by a tree with no epics
	// and no to-be design doc: no work is planned yet, and the reason names
	// how work enters (brownfield zero-day design, decision 5).
	Unplanned bool
```

- [ ] **Step 4: Implement the arm**

Create `internal/views/zero_day.go`:

```go
package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// emptyTreeRoute is the route's last arm before caught-up. A tree with no
// epics routes to decompose only when a to-be design doc exists; with none,
// it is the no-work-planned end state, a caught-up flavor the human owns
// (docs/plans/2026-10-08-brownfield-zero-day-design.md, decisions 2 and 5).
func emptyTreeRoute(t *store.Tree) RouteDecision {
	if n := len(t.ToBeDesign); n > 0 {
		return RouteDecision{
			Seam: "decompose",
			Reason: fmt.Sprintf("no epics recorded yet; %d to-be design %s %s decomposition, starting with .tiller/design/%s",
				n, plural(n, "doc", "docs"), plural(n, "awaits", "await"), t.ToBeDesign[0]),
		}
	}
	return RouteDecision{Reason: unplannedReason(t), CaughtUp: true, Unplanned: true}
}

// unplannedReason is the end state's human-owned Next. It names the open
// question count only when nonzero, and the ways work enters for the
// project's kind.
func unplannedReason(t *store.Tree) string {
	open := 0
	for _, q := range t.Questions {
		if gen.QuestionOpen(q) {
			open++
		}
	}
	questions := ""
	if open > 0 {
		questions = fmt.Sprintf("%d open %s", open, plural(open, "question", "questions"))
	}
	if t.Manifest != nil && (t.Manifest.Project.Kind == store.KindBrownfield || t.Manifest.Project.Kind == store.KindLegacyMigration) {
		facts := "baseline established"
		if questions != "" {
			facts += ", " + questions
		}
		return "no work is planned yet (" + facts + "): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)"
	}
	lead := "no work is planned yet"
	if questions != "" {
		lead += " (" + questions + ")"
	}
	return lead + ": add design inputs under .tiller/design/ for decompose to turn into a draft tree"
}
```

In `RouteDecisionOf`, replace build 1's arm:

```go
	if len(t.Epics) == 0 {
		return RouteDecision{Seam: "decompose", Reason: "no epics recorded yet"}
	}
```

with:

```go
	if len(t.Epics) == 0 {
		return emptyTreeRoute(t)
	}
```

and update the comment above it to end: `Only then does an epic-less tree reach emptyTreeRoute: decompose when a to-be design doc exists, otherwise the no-work-planned end state.`

- [ ] **Step 5: Render the end state in Next**

In `nextAction`, inside `if decision.CaughtUp {`, before the `PoolSignpost` check, add:

```go
		if decision.Unplanned {
			return []ReadoutAction{{Text: decision.Reason, Owner: "human"}}
		}
```

`guide` needs no change: its default case already prints `decision.Reason`, and `PoolSignpost` is empty on a tree with no tasks.

- [ ] **Step 6: Run the new tests**

Run: `go test ./internal/views/ -run 'Unplanned|ToBeDoc|ToBeDesign|LegacyMigrationUses|GreenfieldWithNoDesign' -count=1 -v`
Expected: PASS.

- [ ] **Step 7: Update the view tests that encoded "empty tree means decompose"**

Run: `go test ./internal/views/ -count=1`. Expected failures, each fixed as stated (this is the intended behavior change, not a regression):

- `readout_test.go`, `routeArmFixtures["empty tree"]`: set `tree.ToBeDesign = []string{"roadmap.md"}` in the fixture so it still exercises the decompose arm. In `TestRouteTargetNamesNoNodeForAnEmptyTree`, change `require.Equal(t, "no epics recorded yet", reason)` to `require.Equal(t, "no epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/roadmap.md", reason)`. Add a fixture `"unplanned": func(t *testing.T) *store.Tree { return treeWithDraftEpics(t, 0) }` to `routeArmFixtures` so `TestRouteEqualsRouteTargetsSeamAndReason` covers the new arm; if that test asserts the fixture count in prose or a number, update it.
- `readout_test.go`, `TestEpiclessTreeIgnoresSettledAmendmentsAndRoutesDecompose`: set `tree.ToBeDesign = []string{"roadmap.md"}` and change the reason assertion to `require.Contains(t, reason, "no epics recorded yet")`.
- `workitem_route_test.go`, `TestEpiclessTreeWithOnlyOthersShelfWorkStillRoutesDecompose`: set `tree.ToBeDesign = []string{"roadmap.md"}` and change the reason assertion to `require.Contains(t, decision.Reason, "no epics recorded yet")`.
- `baseline_test.go`, `TestMissingConfirmedBaselineNeedsRepairNotConfirmation`: the two `Contains(..., "decompose")` assertions on `full.Next[0].Text` and `brief.Next[0].Text` become `Contains(..., "no work is planned yet")`; keep their messages.
- `baseline_test.go`, `TestDesignApprovalReadoutReportsTheActNotALifecycleMove`: `Contains(t, r.Next[0].Text, "decompose")` becomes `Contains(t, r.Next[0].Text, "no work is planned yet")`.
- `baseline_test.go`, the assertion at line 161 (`require.Contains(t, r.Next[0].Text, "decompose")`): read its test; if its tree has no epics and no `ToBeDesign`, change it to `"no work is planned yet"`; if the test's point is that decompose follows confirmation when inputs exist, set `tree.ToBeDesign = []string{"roadmap.md"}` instead.

Any other failure is a finding: read it, and change the test only if it encodes "an empty tree with no to-be doc routes to decompose". Record each such change as a ruling.

Run: `go test ./internal/views/ -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/views/
git commit -s -m "Route: decompose only with a to-be doc; otherwise no work planned

The empty-tree arm splits: a to-be design doc makes decompose due and the
reason names it; with none, the route reaches a caught-up flavor marked
Unplanned whose human-owned Next names the open-question count and the
ways work enters. Wording differs for existing-code and greenfield.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: `check readiness` takes Next from the route; CLI and console tests

**Files:**
- Modify: `internal/cli/check_readiness.go` (`runCheckReadiness`, `renderReadinessHuman`)
- Test: `internal/cli/guide_test.go`, `internal/cli/baseline_readout_test.go`, `internal/cli/readout_e2e_test.go`, `internal/serve/route_test.go`

**Interfaces:**
- Consumes: `suggestSkill(t *store.Tree, owner string) (skill, reason string)` (`internal/cli/guide.go:95`), `resolveActingContributor(cmd, root) string` (as `runGuide` calls it), Task 2's route.
- Produces: ready-case human line `Next (tiller guide <skill>): <reason>`.

- [ ] **Step 1: Write the failing CLI tests**

In `internal/cli/guide_test.go`, replace `TestGuideNoArgSuggestsDecomposeOnFreshRepo` with two tests:

```go
// A fresh greenfield repo has nothing under design/ to decompose: guide
// reports the no-work-planned end state instead of suggesting decompose.
func TestGuideNoArgOnFreshRepoReportsNoWorkPlanned(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"skill": "orient"`)
	require.Contains(t, out, "no work is planned yet")
}

func TestGuideNoArgSuggestsDecomposeWithAToBeDoc(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"skill": "decompose"`)
}
```

If `init` does not create `.tiller/design/`, add `require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller", "design"), 0o755))` before the write. Add `os` and `path/filepath` imports if the file lacks them.

Add a readiness test beside `TestGuideRoutesAnOwnedBugOnAnEmptyTree` (build 1):

```go
// check readiness prints guide's suggestion, not a hard-coded decompose:
// with an owned, claimed bug on a ready project, Next is execute.
func TestCheckReadinessNextFollowsTheRoute(t *testing.T) {
	layBugCLI(t)
	require.NoError(t, os.MkdirAll(filepath.Join(".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "jc")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, stderr, code := execCLI(t, "check", "readiness")
	require.Equal(t, 0, code, out+stderr)
	require.Contains(t, out, "Next (tiller guide execute):")
	require.NotContains(t, out, "Next: decompose")
}
```

If `check readiness` is not ready for this fixture (exit `1`, the output names a gap such as a missing code root), satisfy that gap in the fixture rather than weakening the assertion, and record what was needed as a ruling.

- [ ] **Step 2: Run them to confirm the right ones fail**

Run: `go test ./internal/cli/ -run 'TestGuideNoArg|TestCheckReadinessNextFollowsTheRoute' -count=1`
Expected: `TestCheckReadinessNextFollowsTheRoute` FAILS (output has `Next: decompose.`). The two guide tests PASS already, because Task 2 changed the route; that is expected, they pin it at the CLI.

- [ ] **Step 3: Derive the readiness Next line from the route**

In `internal/cli/check_readiness.go`, change `runCheckReadiness` to compute the suggestion and pass it to the renderer:

```go
	report := readiness.Check(tree.Manifest, tree)
	code := 0
	if !report.Ready {
		// A red gate is a normal negative business answer: exit 1, with the
		// gaps carried as structured data the agent can act on.
		code = 1
	}
	// A ready project's Next is guide's own suggestion from the shared
	// route, so readiness never names a seam the route would not.
	skill, reason := suggestSkill(tree, resolveActingContributor(cmd, tree.Root))
	return emitReport(cmd, "check-readiness", report.Ready, report, renderReadinessHuman(report, skill, reason), code)
```

and `renderReadinessHuman`:

```go
// renderReadinessHuman is the plain-text rendering of a readiness report.
// A ready project closes on the route's suggestion, the same skill and
// reason `tiller guide` prints.
func renderReadinessHuman(r readiness.Report, skill, reason string) string {
	var b strings.Builder
	if r.Ready {
		fmt.Fprintf(&b, "Ready: all readiness checks pass.\nNext (tiller guide %s): %s", skill, reason)
	} else {
```

Leave the rest of the function unchanged. Fix any other caller of `renderReadinessHuman` (`grep -rn renderReadinessHuman internal/cli`).

- [ ] **Step 4: Run the CLI tests**

Run: `go test ./internal/cli/ -run 'TestGuide|TestCheckReadiness|Readiness' -count=1`
Expected: PASS.

- [ ] **Step 5: Update the CLI and serve tests that encoded the old empty tree**

- `internal/cli/baseline_readout_test.go` (the loop near lines 100-115): after the first confirmation, `require.Contains(t, out, "**Next:** decompose")` becomes `require.Contains(t, out, "**Next:** no work is planned yet")`; after the second, `require.Equal(t, "decompose", suggestion.Data.Skill)` becomes `require.Equal(t, "orient", suggestion.Data.Skill)` plus `require.Contains(t, suggestion.Data.Reason, "no work is planned yet")`. After the `check readiness` call, capture its stdout and add `require.Contains(t, out, "no work is planned yet")`.
- `internal/cli/readout_e2e_test.go:312`: the arc's fresh greenfield repo has no design docs. Change the assertion to `require.Contains(t, orient, "no work is planned yet", "orient's Route line proves nothing is planned yet")` and update the comment above it (lines 305-311) to say the empty-tree arm now answers "no work is planned yet" because no to-be doc exists.
- `internal/serve/route_test.go`, `TestRouteHandlerOmitsNodeRatherThanNullStuffingIt`: keep its purpose (node absent) on the decompose arm. Before `routeServer`, add `require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "design"), 0o755))` and `require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))`; change the reason assertion to `require.Contains(t, got.Reason, "no epics recorded yet")`.

Run: `go test ./internal/cli/ ./internal/serve/ -count=1 > <workspace>/t3.log 2>&1; grep -E "^(--- FAIL|ok|FAIL)" <workspace>/t3.log`
Expected: `ok` for both. Any other failure is a finding handled as in Task 2 Step 7. If `TestReadoutAcrossTheArc` fails inside `cp -a` on a temp `.git/objects` directory, that is the known git auto-gc race (see build 1's ledger); rerun once.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/ internal/serve/route_test.go
git commit -s -m "Readiness: print guide's suggestion instead of a fixed decompose

A ready project's Next now comes from the shared route, so it names the
no-work-planned end state, owned work, or decompose exactly as guide
does. CLI and console tests move to the new empty-tree answers.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Orientation, orient skill, and specs; generation 46

**Files:**
- Modify: `internal/gen/orientation.go` (constant and comment; rule 9)
- Modify: `internal/skills/orient/SKILL.md` (the no-epics bullet)
- Modify: `docs/spec/cli-surface.md` (line 384 readiness paragraph; line 849 route sentence)
- Modify: `docs/spec/skill-set.md` (line 5 status; line 128 sentence added in build 1)
- Modify: `docs/spec/ui-surface.md:91` (route strip)
- Modify: current-source generation mentions: `docs/plans/STATUS.md:3`, `docs/tiller-commands.md` (lines 7-9 and the "Refresh and commit orientation 45" line), `docs/spec/distribution.md` (line 5, line 226's gen-45 sentence, line 397), `site/src/content/docs/source-quickstart.md`, `site/src/content/docs/skills.md`, `site/src/content/docs/concepts.md`

**Interfaces:**
- Consumes: Tasks 1-3 behavior and exact reason strings.
- Produces: `orientationSchemaGen = 46`.

- [ ] **Step 1: Bump the generation**

In `internal/gen/orientation.go`, replace the generation 45 comment and constant with:

```go
// Generation 46 makes decompose due on an empty tree only when a to-be design
// doc exists, and names the no-work-planned end state otherwise.
const orientationSchemaGen = 46
```

- [ ] **Step 2: Rewrite the empty-tree sentence in rule 9**

Replace, in rule 9:

```
   stop. Otherwise, with no epics recorded, the seam is ` + "`decompose`" + `.
   Every rule above that can apply to a tree with no epics routes first: owned
   shelf work, a proposed amendment, and a shelf merge wait are real work or a
   real wait, while unclaimed shelf work never enters the route.
```

with:

```
   stop. Otherwise, with no epics recorded, decompose is due only when a
   to-be design doc exists: a Markdown file under ` + "`.tiller/design/`" + ` that
   ` + "`baseline.yml`" + ` does not register. Then the seam is ` + "`decompose`" + `.
   With none, no work is planned yet: a human wait, not a seam. Name the
   open-question count and the ways work enters (intake a document that makes
   a case for change, capture a bug, chore, or tweak, or add a to-be design),
   and stop. Never turn as-is docs or old specs into epics nobody asked for.
   Every rule above that can apply to a tree with no epics routes first: owned
   shelf work, a proposed amendment, and a shelf merge wait are real work or a
   real wait, while unclaimed shelf work never enters the route.
```

- [ ] **Step 3: Update the orient skill bullet**

In `internal/skills/orient/SKILL.md`, replace the bullet beginning `- No epics are recorded and no bullet above routes:` with:

```
- No epics are recorded and no bullet above routes: decompose is due only when a to-be design doc exists, a Markdown file under `.tiller/design/` that `baseline.yml` does not register, and then the seam is `decompose`. With none, no work is planned yet, a human wait: name the open-question count and the ways work enters (intake, capture, or a to-be design) and stop, without proposing epics from as-is docs or old specs. On a tree with no epics, owned shelf work in `check`, `ready`, or `in_progress`, a proposed amendment, and a shelf merge wait still route first; unclaimed shelf work never enters the route.
```

- [ ] **Step 4: Update the specs**

`docs/spec/cli-surface.md:849`: replace the sentence `Only when none of those applies does a tree with no epics route to `decompose`, ahead of the caught-up terminal;` with:

```
Only when none of those applies does a tree with no epics reach its empty-tree arm, ahead of the caught-up terminal: `decompose` when a to-be design doc exists (a Markdown file under `.tiller/design/` that `baseline.yml` does not register, dotfiles excepted), otherwise the no-work-planned end state, a caught-up flavor with no seam whose human-owned Next names the open-question count, when nonzero, and the ways work enters: intake, capture, or a to-be design;
```

Keep the rest of that sentence (`a maintenance-only project with owned shelf work ... is never told to decompose.`).

`docs/spec/cli-surface.md:384`: append to the readiness paragraph:

```
On a ready project the human output closes with `Next (tiller guide <skill>): <reason>`, the same suggestion `tiller guide` prints from the shared route, never a fixed seam.
```

`docs/spec/skill-set.md:128`: replace the sentence build 1 appended (`The no-epics arm comes last, just before caught-up: ...`) with:

```
The no-epics arm comes last, just before caught-up: owned shelf work, a proposed amendment, and a shelf merge wait on a tree with no epics route first. It names `decompose` only when a to-be design doc exists; with none, orient reports that no work is planned, names the open-question count and the ways work enters, and stops without proposing epics.
```

`docs/spec/skill-set.md:5`: `orientation generation 45` becomes `orientation generation 46`.

`docs/spec/ui-surface.md:91`: append to the route strip bullet: ` With no work planned, it shows "Nothing due" beside the end state's reason.`

- [ ] **Step 5: Update the current-source generation mentions**

Change the current-source generation from 45 to 46 in: `docs/plans/STATUS.md:3`, `docs/tiller-commands.md` ("Current source uses schema 10, orientation generation 45" and "Refresh and commit orientation 45"), `docs/spec/distribution.md` (line 5 and the "Current source uses schema 10, ten skills, and orientation generation 45" sentence), `site/src/content/docs/source-quickstart.md`, `site/src/content/docs/skills.md`, `site/src/content/docs/concepts.md`.

Append a history sentence after each existing generation-45 sentence: in `docs/tiller-commands.md` after "Generation 45 moves the empty-tree decompose rule below every rule that can apply to a tree with no epics." and in `docs/spec/distribution.md:226` after the same sentence, add: `Generation 46 makes decompose due on an empty tree only when a to-be design doc exists and names the no-work-planned end state otherwise.`

Run: `git grep -n -e "generation 45" -e "generation \*\*45" -e "orientation 45" -- docs site internal`
Expected: only the two historical "Generation 45 moves ..." sentences and the project-history entry.

- [ ] **Step 6: Tests and style**

Run: `go test ./internal/gen/ ./internal/skills/ ./internal/cli/ -run 'Orientation|Join|Migrate|Guide|Skill' -count=1`
Expected: PASS.

Run: `git diff -U0 > <workspace>/t4.diff` then `grep "^+" <workspace>/t4.diff | grep -c "—"`
Expected: `0`.

- [ ] **Step 7: Commit**

```bash
git add internal/gen/orientation.go internal/skills/orient/SKILL.md docs/spec docs/plans/STATUS.md docs/tiller-commands.md site/src/content/docs
git commit -s -m "Orientation: decompose needs a to-be doc; generation 46

Orientation, the orient skill, and the specs state the empty-tree rule:
decompose when a to-be design doc exists, otherwise no work is planned,
a human wait naming open questions and the ways work enters.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Full suite, lint, the real surfaces, and the record

**Files:**
- Modify: `docs/project-history.md`, `docs/plans/STATUS.md`

- [ ] **Step 1: Full suite and lint**

Run: `make test` and `make lint`, each redirected to a workspace log; read the tails.
Expected: both exit `0`.

- [ ] **Step 2: Drive the real binary through the trial's moment**

Build to the workspace and run this script with `sh` (adjust `S` to the workspace):

```sh
#!/bin/sh
set -e
S=<workspace>
go build -o "$S/tiller" ./cmd/tiller
T=$S/tiller
P=$S/e2e
rm -rf "$P" && mkdir -p "$P/app" && cd "$P"
git init -q -b main && git config user.email s@t && git config user.name S
echo 'package main' > app/main.go
git add -A && git commit -qm code
$T init --name "Old App" --key old --kind brownfield >/dev/null
$T add contributor jc --name "JC Grubbs" >/dev/null
$T join --as jc >/dev/null
mkdir -p .tiller/design/as-is
printf '# App\n\nWhat the app does today.\n' > .tiller/design/as-is/app.md
git add -A && git commit -qm scaffold
$T link design as-is/app.md --describes app --contributor jc >/dev/null
git add -A && git commit -qm link
echo "== approve design (Next line)"
$T approve design as-is/app.md --contributor jc | grep "Next"
git add -A && git commit -qm confirm
echo "== guide"
$T guide | head -2
echo "== check readiness"
$T check readiness
echo "== readout orient"
$T readout orient | grep -E "Route|Next"
echo "== with a to-be doc"
printf '# Sync\n\nCross-device sync.\n' > .tiller/design/sync.md
$T guide | head -2
$T check readiness | tail -1
```

Expected:
- approve design: `**Next:** no work is planned yet (baseline established): bring work in through intake ..., and it is yours.`
- guide: `Suggested skill: orient` and the same reason.
- check readiness: `Ready: all readiness checks pass.` then `Next (tiller guide orient): no work is planned yet (baseline established): ...`
- readout orient: Route and Next both carry the end-state reason.
- With `sync.md`: `Suggested skill: decompose`, and readiness ends `Next (tiller guide decompose): The tree is undecomposed; ...`.

On the build 1 binary the same script printed `**Next:** decompose: no epics recorded yet.`, `Suggested skill: decompose`, and `Next: decompose. Read .tiller/orientation.md or run 'tiller guide decompose'.` (observed 2026-10-08). Record the observed after-output.

- [ ] **Step 3: Record the outcome**

Add a project-history entry, `## Brownfield zero-day end state`, directly above build 1's `## Empty-tree route order`, in the same style: what changed (to-be docs, the end state and its wording, readiness Next), generation 46, the refinements (Markdown only, console unchanged), and the before/after from Step 2. In `docs/plans/STATUS.md`, extend build 1's October 8 paragraph with one sentence for build 2 at generation 46, unreleased, and change the Unbuilt "Brownfield zero day" bullet to name only build 3 (the derive-baseline forward-work pass with its standalone arm, and the decompose and derive-baseline skill text).

- [ ] **Step 4: Commit**

```bash
git add docs/project-history.md docs/plans/STATUS.md
git commit -s -m "Docs: record the brownfield zero-day end state

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Do not push, merge to main, stamp a version, install, or close #137. Build 3 (skills) is a separate plan.
