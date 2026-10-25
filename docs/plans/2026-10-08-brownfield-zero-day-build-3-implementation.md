# Brownfield zero day, build 3: forward work as questions, and the skills that route around it Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `derive-baseline` records unbuilt work it finds as one question per capability, a standalone arm runs that pass on request against an already-confirmed baseline, and every skill, spec, and site page stops promising that an empty tree means decompose.

**Architecture:** This build changes embedded skill text, the specs, and one site page. The only code change is a small set of build 2 review follow-ups in Task 4. Skill text has no compiler, so each skill change is pinned by a test in `internal/skills/skills_test.go` that fails first, in the style of `TestDecomposeAsksForNoEpicContract`. Orientation is not edited, so the generation stays at 46.

**Tech Stack:** Markdown skills embedded through `internal/skills`, Go tests with testify `require`, `make test` / `make lint`.

**Spec:** `docs/plans/2026-10-08-brownfield-zero-day-design.md`, section "The derive-baseline forward-work pass", decisions 1, 3, 4, 6, 7, and "Build sequence" build 3. Builds 1 and 2 are already on this branch.

## Global Constraints

- Decision 1: forward work found during the ingest is recorded as questions.
- Decision 3: plain questions, no kind marker and no title convention required by the CLI; `--source` cites the unbuilt material. An answer of "build it" leads to intake on the cited source; "it's a bug" leads to capture.
- Decision 4: one question per unbuilt capability, citing every source that describes it. A code TODO counts only when a document corroborates it. An old implementation plan counts only where it describes a capability no spec covers.
- Decision 6: the standalone forward-work arm runs only on explicit human request. Nothing records that it ran; the route does not signal it.
- Decision 7: defect-shaped findings at ingest are questions, never captured bugs.
- Skills must keep the five-part anatomy, contain no em dash, name no harness tool, and not point outside the project (`docs/spec`, `skill-set.md`, and so on). `TestSkillsHaveFivePartAnatomy`, `TestSkillsAreNeutralProse`, and `TestSkillsStayInsideTheProject` enforce this.
- New question-writing examples carry a separate `--title` (`TestSkillsRequireShortQuestionTitlesForNewWrites` is the house rule).
- No orientation text change, no generation bump, no schema change, no version stamp, release, install, push, merge, or managed-project run.
- Prose: no em dashes. Commits: `git commit -s`, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Work in the worktree `/Users/jcgrubbs/dev/tiller/.claude/worktrees/brownfield-zero-day`, branch `brownfield-zero-day`.

## Decisions this plan makes for JC to confirm

- **Orientation stays at generation 46.** Orientation's summary of the baseline seam does not mention forward work, but nothing it says becomes false. Adding it would cost generation 47 for a description agents read in the skill anyway.
- **Skill descriptions change, so managed checkouts need a refresh.** `tiller join` copies each skill's frontmatter description onto the harness shim. The derive-baseline and decompose descriptions change here, so a trial project's shims stay stale until it re-joins or runs `tiller upgrade --project-only`. That is a distribution step after release, not part of this build.
- **The run against the trial project waits for JC.** The design's check (one question each for cross-device sync and calculator units, none for built material) needs the released binary in that managed project and JC's go. This build verifies the skill text and the CLI commands it names.
- **Task 4 carries five build 2 review follow-ups.** They are small and cheap while the branch is open. JC can strike the task to keep build 3 to skill text alone.

## Review Focus

- **The standalone arm on a project that has already run it once.** A second scan must not open reworded duplicates of open questions. The skill tells the agent to read the question ledger first, because `tiller add question` is idempotent only for an identical statement. Pinned in Task 1's test by the ledger-first sentence.
- **An agent reading "build it" as permission to start intake.** The answer leads to intake, but derive-baseline names that move and stops. Pinned in Task 1.
- **A defect-shaped finding during the ingest.** The skill must forbid `tiller add bug`, `chore`, and `tweak` from this seam. Pinned in Task 1.
- **A fresh greenfield or freshly baselined project told to decompose by a skill description** a harness auto-selects on. The decompose frontmatter must not trigger on an empty tree alone. Pinned in Task 2.
- **The `--source` example being runnable.** `tiller add question` accepts `--title` and `--source`; Task 5 runs the skill's example command against a throwaway project.

---

### Task 1: derive-baseline records forward work and gains a standalone arm

**Files:**
- Modify: `internal/skills/derive-baseline/SKILL.md`
- Test: `internal/skills/skills_test.go`

**Interfaces:**
- Consumes: `Lookup(name string) (string, bool)` in `internal/skills`.
- Produces: skill text later tasks quote: the section heading `### Record forward work`, the bold lead `**Standalone arm.**`, and the example command
  `tiller add question "<what the sources describe, what the code has, and the decision: build it, or leave it as history>" --title "<capability>: unbuilt" --source "<path>#<section>"`.

- [ ] **Step 1: Write the failing test**

Append to `internal/skills/skills_test.go`:

```go
// TestDeriveBaselineRecordsForwardWorkAsQuestions pins the brownfield zero-day
// design's forward-work pass (decisions 1, 3, 4, 6, 7): one question per
// unbuilt capability, cited sources, an explicit-only standalone arm, no
// captured bugs, and a hand-off that names the next door without opening it.
func TestDeriveBaselineRecordsForwardWorkAsQuestions(t *testing.T) {
	text, ok := Lookup("derive-baseline")
	require.True(t, ok)
	for _, want := range []string{
		"### Record forward work",
		"Open one question per unbuilt capability, not per file.",
		`--title "<capability>: unbuilt" --source "<path>#<section>"`,
		"A code comment or TODO counts only when a document corroborates it",
		"Read the question ledger first",
		"never a captured bug",
		"Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam.",
		"**Standalone arm.**",
		"Nothing in recorded state names this arm; never start it on your own.",
		"Name that next move and stop",
		"scan existing sources for unbuilt work",
	} {
		require.Containsf(t, text, want, "derive-baseline carries %q", want)
	}
	require.NotContains(t, text, "an empty tree ordinarily reaches decompose",
		"an empty tree reaches decompose only when a to-be design doc exists")
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/skills/ -run TestDeriveBaselineRecordsForwardWorkAsQuestions -count=1`
Expected: FAIL on the first missing string, `### Record forward work`.

- [ ] **Step 3: Edit the frontmatter description**

Replace the `description:` line with:

```
description: Use when a brownfield or legacy-migration project has no confirmed as-is design doc, or the developer asks to derive or reconcile its baseline, or to scan existing sources for unbuilt work. Reads existing docs and code, derives or translates module-grain descriptions, records recoverable provenance and one question per unbuilt capability, and stops for human confirmation of named docs. Re-entry reviews pending docs rather than regenerating them.
```

- [ ] **Step 4: Extend the Trigger**

After the Trigger's second paragraph (the one ending `Do not create another set merely because guide still names this seam.`), add a paragraph:

```
A developer may also ask, on a project whose baseline is already confirmed, to scan existing sources for unbuilt work. That request runs only **Record forward work** below, through its standalone arm. Nothing in recorded state names this arm; never start it on your own.
```

- [ ] **Step 5: Add the Record forward work section**

Insert this subsection between the end of `### Derive or translate at module grain` (after the house-format paragraph ending `Do not traverse symlinks or special files to evade path safety.`) and `### Choose ownership once: move or pointer`:

````
### Record forward work

Sources often describe more than the code does: a spec whose feature never landed, an unbuilt section of an otherwise built spec, a roadmap. The as-is doc keeps its "not implemented" note, because that describes current behavior. Whether to build it is the human's decision, so record it as a question: not as run notes that vanish with this conversation, and not as planned work.

Open one question per unbuilt capability, not per file. Cite every source that describes it in `--source`. A code comment or TODO counts only when a document corroborates it; on its own it is not a lead. An old implementation plan counts only where it describes a capability no spec covers. Read the question ledger first and skip capabilities already built, already in the tree, or already covered by an open question: re-adding an identical statement is a no-op, but a reworded duplicate is a second question.

```
tiller add question "<what the sources describe, what the code has, and the decision: build it, or leave it as history>" --title "<capability>: unbuilt" --source "<path>#<section>"
```

Repeat `<path>#<section>` inside the one `--source` value, comma-separated, when several sources describe the same capability.

A defect-shaped finding is a question too, never a captured bug. This seam reads code and docs; it does not reproduce anything, and a source/code disagreement may be a deliberate change. Deciding that current behavior is wrong is the human's call. Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam.

The human's answer is the recorded decision. "Build it" leads to `intake` on the cited source, or to a to-be design under `.tiller/design/`, which makes decompose due. "It's a bug" leads to `capture`. "Leave it" closes the lead. Name that next move and stop for the human's go; this seam runs neither intake nor capture.

**Standalone arm.** On an already-confirmed baseline, read the as-is docs' "not implemented" notes and their recorded `--from` provenance, recover retired sources with their recorded blob refs, reread sources that still live in the repository, and read the question ledger. Open only the missing questions, commit exactly the question and event files, and end with the readout. The arm writes no as-is prose, links nothing, retires nothing, and approves nothing.
````

- [ ] **Step 6: Update Choreography**

In the Choreography sentence that begins `Read; settle move versus pointer;`, change `record questions;` to `record questions, including one per unbuilt capability;`.

In the fenced example, directly after the existing two lines

```
# Open only questions actually found, with a short title:
tiller add question "<specific source/code disagreement or uncertainty>" --title "<short title>"
```

add:

```
# One question per unbuilt capability, citing every source that describes it:
tiller add question "<what the sources describe, what the code has, and the decision: build it, or leave it as history>" --title "<capability>: unbuilt" --source "<path>#<section>"
```

After the paragraph that begins `For a derive-only module, write the same house format`, add a paragraph:

```
The standalone forward-work arm runs only the question step: read the as-is docs, their sources, and the question ledger; add the missing questions; commit exactly the question and event files; and end with `tiller readout derive-baseline`.
```

- [ ] **Step 7: Update the Readout notes**

Replace the bullet

```
- Questions actually opened during this run, by short title and Q-id, and any unresolved uncertainty not recorded as a question. The readout's **Open questions** is the current ledger count, not a count opened by this run.
```

with:

```
- Questions actually opened during this run, by short title and Q-id, and any unresolved uncertainty not recorded as a question. List forward-work questions separately, each with its capability and cited sources. The readout's **Open questions** is the current ledger count, not a count opened by this run.
```

- [ ] **Step 8: Fix the Hand-off's route sentence**

Replace `Then follow `tiller guide`: an empty tree ordinarily reaches decompose, while an existing work tree resumes its normal route.` with:

```
Then follow `tiller guide`: an empty tree reaches decompose only when a to-be design doc exists; otherwise guide reports that no work is planned, with forward-work questions inside its open-question count, while an existing work tree resumes its normal route.
```

- [ ] **Step 9: Run the skills package**

Run: `go test ./internal/skills/ -count=1`
Expected: PASS, including the anatomy, neutral-prose, and stay-inside-the-project tests.

- [ ] **Step 10: Commit**

```bash
git add internal/skills/derive-baseline/SKILL.md internal/skills/skills_test.go
git commit -s -m "Skills: derive-baseline records unbuilt work as questions

One question per unbuilt capability with cited sources, never a captured
bug, a hand-off that names intake or capture and stops, and a standalone
arm that runs only on an explicit request against a confirmed baseline.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: decompose, orient, intake, and capture stop promising decompose on an empty tree

**Files:**
- Modify: `internal/skills/decompose/SKILL.md` (frontmatter description; Trigger, line 10)
- Modify: `internal/skills/orient/SKILL.md` (Choreography's onboarding sentence, about line 119)
- Modify: `internal/skills/intake/SKILL.md` (Trigger)
- Modify: `internal/skills/capture/SKILL.md` (Trigger)
- Test: `internal/skills/skills_test.go`

**Interfaces:**
- Consumes: Task 1's wording ("forward-work question", "build it", "it's a bug").
- Produces: nothing later tasks consume beyond the pinned strings.

- [ ] **Step 1: Write the failing test**

```go
// TestEmptyTreeSkillsNeedToBeInputs pins the brownfield zero-day route in the
// skills a harness auto-selects or an agent follows on a fresh tree: decompose
// is due only with to-be inputs, orient's onboarding stop says so, and the
// doors a forward-work answer opens accept it.
func TestEmptyTreeSkillsNeedToBeInputs(t *testing.T) {
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.NotContains(t, decompose, "or when the tree is still empty")
	require.NotContains(t, decompose, "or the project is fresh and nothing has been decomposed")
	require.Contains(t, decompose, "an empty tree with only as-is docs has nothing to decompose")

	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.NotContains(t, orient, "derive-baseline for an unconfirmed existing-code project or decompose otherwise")
	require.Contains(t, orient, "decompose when a to-be design doc exists")

	intake, ok := Lookup("intake")
	require.True(t, ok)
	require.Contains(t, intake, "a source that a forward-work question cites, once the human answers that they want it built")

	capture, ok := Lookup("capture")
	require.True(t, ok)
	require.Contains(t, capture, "A human answer to a forward-work question that names a defect is that description.")
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/skills/ -run TestEmptyTreeSkillsNeedToBeInputs -count=1`
Expected: FAIL at the first `NotContains` on decompose.

- [ ] **Step 3: Edit decompose**

Replace the frontmatter `description:` line with:

```
description: Use when to-be design inputs exist under .tiller/design/ (files the baseline ledger neither registers as as-is docs nor cites as sources) but the epic and feature tree does not yet reflect them, after the existing-code baseline gate is satisfied, or when the developer explicitly names decompose. Turns inputs into a draft epic and feature tree for the human to approve.
```

Replace the Trigger's first paragraph (`Inputs exist under `.tiller/design/` but the epic and feature tree does not, or a new design doc landed that the tree does not yet reflect, or the project is fresh and nothing has been decomposed.`) with:

```
To-be inputs exist under `.tiller/design/`, files the baseline ledger neither registers as as-is docs nor cites as sources, but the epic and feature tree does not reflect them; or a new design doc landed that the tree does not yet reflect; or the developer explicitly names decompose. Registered as-is docs describe what exists, not new work: an empty tree with only as-is docs has nothing to decompose. Report that no work is planned and stop rather than inventing epics from the baseline or from old specs.
```

- [ ] **Step 4: Edit orient's onboarding sentence**

In Choreography, replace `before the applicable first seam, derive-baseline for an unconfirmed existing-code project or decompose otherwise.` with:

```
before the applicable first seam: derive-baseline for an unconfirmed existing-code project, decompose when a to-be design doc exists, and otherwise the no-work-planned end state.
```

- [ ] **Step 5: Edit intake and capture triggers**

In `internal/skills/intake/SKILL.md`, at the end of the Trigger's paragraph (after `the landing already happened, and the explosion is what is left.`), add:

```
This trigger covers a source that a forward-work question cites, once the human answers that they want it built, even on a tree with no epics yet; stage each `add-epic` before its features.
```

In `internal/skills/capture/SKILL.md`, at the end of the Trigger's paragraph (after `Capture records the finding. It does not triage it.`), add:

```
A human answer to a forward-work question that names a defect is that description.
```

- [ ] **Step 6: Run the skills package**

Run: `go test ./internal/skills/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/skills/decompose/SKILL.md internal/skills/orient/SKILL.md internal/skills/intake/SKILL.md internal/skills/capture/SKILL.md internal/skills/skills_test.go
git commit -s -m "Skills: decompose needs to-be inputs; answers open intake or capture

The decompose description and trigger no longer fire on an empty tree
alone, orient's onboarding stop names the no-work-planned end state, and
intake and capture accept an answered forward-work question.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Specs and the site page

**Files:**
- Modify: `docs/spec/skill-set.md` (seam table row near line 95; derive-baseline section lines 152-180; decompose Trigger near line 184; capture Trigger near line 339; intake Trigger near line 469)
- Modify: `docs/spec/cli-surface.md:849` (greenfield end-state wording)
- Modify: `site/src/content/docs/source-quickstart.md:138`

- [ ] **Step 1: skill-set.md seam table**

Replace the row

```
| `decompose` | inputs exist but the tree does not | inputs to a draft epic and feature tree | `add epic` / `add feature`, left in `draft` |
```

with

```
| `decompose` | to-be inputs exist but the tree does not reflect them | inputs to a draft epic and feature tree | `add epic` / `add feature`, left in `draft` |
```

- [ ] **Step 2: skill-set.md derive-baseline section**

Append to its **Trigger.** paragraph:

```
A developer may also ask for a standalone forward-work scan on an already-confirmed baseline; nothing in recorded state names it, and the agent never starts it unasked.
```

After the **Reasoning.** paragraph that ends `open a question with the existing `tiller add question "<statement>" --title "<short title>"` command.`, add a paragraph:

```
Record forward work as questions. Sources often describe capabilities the code never got: an unbuilt spec, an unbuilt section of a built one, a roadmap. The as-is doc keeps its "not implemented" note; the decision to build belongs to the human. Open one question per unbuilt capability, not per file, citing every source that describes it with `--source`. A code TODO counts only when a document corroborates it, and an old implementation plan only where no spec covers the capability. Read the question ledger first to avoid reworded duplicates. A defect-shaped finding is a question too, never a captured bug: the seam reads, it does not reproduce. An answer of "build it" leads to intake on the cited source or a to-be design; "it's a bug" leads to capture; the seam names that move and stops. The standalone arm runs only this step on a confirmed baseline, writing no as-is prose and approving nothing.
```

In its **Choreography.** fenced block, after `# Record any actual questions, then retire only the agreed in-repo source.`, add:

```
tiller add question "<what the sources describe and what the code has>" --title "<capability>: unbuilt" --source "docs/roadmap.md#sync"
```

In its **Hand-off.** paragraph, replace `Existing work routes then resume, or an undecomposed project moves to `decompose`.` with:

```
Existing work routes then resume; an undecomposed project moves to `decompose` only when a to-be design doc exists, and otherwise reaches the no-work-planned end state, whose open-question count includes forward-work questions.
```

- [ ] **Step 3: skill-set.md decompose, capture, intake triggers**

Replace the decompose **Trigger.** paragraph with:

```
**Trigger.** To-be inputs exist under `design/`, files the baseline ledger neither registers nor cites as sources, but the epic and feature tree does not reflect them; or a new design doc landed that the tree does not yet reflect; or the developer explicitly names decompose. An empty tree with only as-is docs has nothing to decompose.
```

Append to the capture **Trigger.** paragraph: ` A human answer to a forward-work question that names a defect is that description.`

Append to the intake **Trigger.** paragraph: ` That includes a source a forward-work question cites, once the human answers that they want it built, even on a tree with no epics yet.`

- [ ] **Step 4: cli-surface.md greenfield wording**

In line 849's sentence, replace `whose human-owned Next names the open-question count, when nonzero, and the ways work enters: intake, capture, or a to-be design;` with:

```
whose human-owned Next names the open-question count, when nonzero, and how work enters: for brownfield and legacy-migration, intake, capture, or a to-be design; for greenfield, design inputs under `.tiller/design/`;
```

- [ ] **Step 5: source-quickstart.md**

Replace `With no tree, the next ordinary route is decompose.` with:

```
With no tree, decompose is next only when a to-be design doc exists under `.tiller/design/`; otherwise
Tiller reports that no work is planned and names how work enters.
```

Keep the surrounding hard line wrapping consistent with the file (it wraps near 100 columns). Then read the walkthrough's later `tiller guide` example (around line 200). If it shows `Suggested skill: decompose` without the walkthrough having added a design input, add one sentence before it saying the walkthrough's design input is what makes decompose due; do not change the example output.

- [ ] **Step 6: Style check**

Run: `git diff -U0 > <workspace>/t3.diff` then `grep "^+" <workspace>/t3.diff | grep -c "—"`
Expected: `0`.

- [ ] **Step 7: Commit**

```bash
git add docs/spec/skill-set.md docs/spec/cli-surface.md site/src/content/docs/source-quickstart.md
git commit -s -m "Docs: specify forward-work questions and the to-be decompose trigger

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Build 2 review follow-ups (JC may strike this task)

**Files:**
- Modify: `internal/cli/guide.go:103-105` (decompose reason)
- Modify: `internal/cli/check_readiness.go` (help text at line 52; compute the suggestion only when ready)
- Modify: `internal/store/tree.go` (gate the to-be walk on an epic-less tree)
- Test: `internal/cli/guide_test.go`, `internal/serve/route_test.go`, `internal/store/tobe_design_test.go`

**Interfaces:**
- Consumes: `emptyTreeRoute`'s decompose reason (`no epics recorded yet; N to-be design doc(s) await(s) decomposition, starting with .tiller/design/<first>`).
- Produces: `Tree.ToBeDesign` is nil whenever epics exist (document it on the field).

- [ ] **Step 1: Failing tests**

In `internal/cli/guide_test.go`, extend `TestGuideNoArgSuggestsDecomposeWithAToBeDoc` with:

```go
	require.Contains(t, out, "starting with .tiller/design/roadmap.md", "guide names the to-be doc the route found")
```

In `internal/serve/route_test.go`, add a test that pins the end-state payload:

```go
// The no-work-planned end state serves no seam, no node, and the reason.
func TestRouteHandlerServesTheUnplannedEndState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(routeTestManifest), 0o644))
	srv := routeServer(t, root)

	resp, err := srv.Client().Get(srv.URL + "/api/route")
	require.NoError(t, err)
	defer resp.Body.Close()
	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	_, present := raw["node"]
	require.False(t, present)
	var got routePayload
	require.NoError(t, json.Unmarshal(mustMarshal(t, raw), &got))
	require.Equal(t, "", got.Seam)
	require.Contains(t, got.Reason, "no work is planned yet")
}
```

In `internal/store/tobe_design_test.go`, add:

```go
// The route consults ToBeDesign only on an epic-less tree, so loading skips
// the walk once epics exist; the fixture tree has one epic.
func TestLoadSkipsTheToBeWalkWhenEpicsExist(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	writeDesign(t, root, "sync.md", "# Sync\n")

	tree, err := Load(root)
	require.NoError(t, err)
	require.NotEmpty(t, tree.Epics)
	require.Nil(t, tree.ToBeDesign)
}
```

This last test contradicts build 2's `TestLoadListsUnregisteredInputsAsToBeDesign`, `TestLoadIgnoresSymlinkedDesignEntries`, `TestLoadWithoutADesignDirHasNoToBeDesign`, `TestLoadExcludesRegisteredDocsFromToBeDesign`, `TestLoadExcludesCitedSourcesFromToBeDesign`, and `TestLoadMatchesRegisteredDocsWithoutCase`, which use the same one-epic fixture. Add a helper in `tobe_design_test.go`, `epiclessFixture(t *testing.T) string`, that calls `copyTree(t, fixtureTreeRoot)`, removes `.tiller/epics`, and repairs whatever then dangles, and use it in place of `copyTree(t, fixtureTreeRoot)` in those six tests. The fixture's `.tiller/decisions/open-questions.yml` blocks `E-j8m4q2` and its shelf items may attach to epic nodes; run `Load` on the stripped copy, read the refusal, and remove exactly the dangling references it names (for example, rewrite the question's `blocks` to `[]`). Record the repairs in the ledger.

- [ ] **Step 2: Run them to confirm they fail**

Run: `go test ./internal/cli/ -run TestGuideNoArgSuggestsDecomposeWithAToBeDoc -count=1` (FAIL: guide prints the fixed text), `go test ./internal/serve/ -run TestRouteHandlerServesTheUnplannedEndState -count=1` (PASS already; it pins existing behavior), and `go test ./internal/store/ -run TestLoadSkipsTheToBeWalkWhenEpicsExist -count=1` (FAIL: the walk still runs).

- [ ] **Step 3: Implement**

`internal/cli/guide.go`, decompose case:

```go
	case "decompose":
		skill = "decompose"
		reason = "The tree is undecomposed; lay a draft epic and feature tree for the human to approve. " + decision.Reason + "."
```

`internal/store/tree.go`: change the load call to

```go
	if len(t.Epics) == 0 {
		t.loadToBeDesign()
	}
```

and add to the `ToBeDesign` field comment: `Gathered only when the tree has no epics, the one case the route consults it; nil otherwise.`

`internal/cli/check_readiness.go`: change `Short` to `"Verify the project inputs and manifest are ready, and name the next move."`, and compute the suggestion only when ready:

```go
	skill, reason := "", ""
	if report.Ready {
		// A ready project's Next is guide's own suggestion from the shared
		// route, so readiness never names a seam the route would not.
		skill, reason = suggestSkill(tree, resolveActingContributor(cmd, tree.Root))
	}
```

- [ ] **Step 4: Run the affected packages**

Run: `go test ./internal/store/ ./internal/serve/ ./internal/cli/ -run 'ToBe|Guide|Readiness|Route' -count=1`
Expected: PASS. If `TestReadoutAcrossTheArc` fails inside `cp -a` on a temp `.git/objects` directory, that is the known git auto-gc race; rerun once.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/guide.go internal/cli/check_readiness.go internal/store/tree.go internal/store/tobe_design_test.go internal/cli/guide_test.go internal/serve/route_test.go
git commit -s -m "Route: name the to-be doc in guide; skip the walk on non-empty trees

Build 2 review follow-ups: guide and check readiness name the to-be doc
the route found, the design walk runs only on an epic-less tree, readiness
computes its suggestion only when ready, and a serve test pins the
no-work-planned payload.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Full suite, lint, the real surfaces, and the record

**Files:**
- Modify: `docs/project-history.md`, `docs/plans/STATUS.md`

- [ ] **Step 1: Full suite and lint**

Run: `make test` and `make lint`, each redirected to a workspace log; read the tails.
Expected: both exit `0`.

- [ ] **Step 2: The skill text and its command at the real surfaces**

```sh
go build -o <workspace>/tiller ./cmd/tiller
T=<workspace>/tiller
$T guide derive-baseline | grep -n -E "Record forward work|Standalone arm|unbuilt"
$T guide decompose | sed -n 1,12p
```

Expected: the derive-baseline guide carries the new section and arm; decompose's description and trigger no longer mention an empty or fresh tree.

Then run the skill's example command in a throwaway brownfield project (the build 2 end-to-end scaffold: init brownfield, register and join `jc`, link and approve one as-is doc), followed by:

```sh
$T add question "The original spec describes calculator units and currency; the calculator has neither. Build it, or leave it as history?" --title "Calculator units: unbuilt" --source "docs/spec.md#4.8"
$T guide | head -2
```

Expected: the question is created with its title and source, and guide reports `no work is planned yet (baseline established, 1 open question): ...`.

- [ ] **Step 3: Record the outcome**

Add a project-history entry, `## Brownfield zero-day forward work`, directly above build 2's `## Brownfield zero-day end state`: the forward-work step and its rules (decisions 1, 3, 4, 6, 7), the standalone arm, the decompose/orient/intake/capture skill changes, the build 2 follow-ups if Task 4 ran, the observed Step 2 output, and that the run against the trial project waits for release and JC's go. In `docs/plans/STATUS.md`, extend the October 8 paragraph with one sentence for build 3, and remove the Unbuilt "Brownfield zero day" bullet, replacing it with one naming the remaining step: run the standalone forward-work arm on the trial project after release, and refresh its skill shims.

- [ ] **Step 4: Commit**

```bash
git add docs/project-history.md docs/plans/STATUS.md
git commit -s -m "Docs: record brownfield zero-day forward work

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Do not push, merge to main, stamp a version, install, re-join a managed project, or run anything in the trial project.
