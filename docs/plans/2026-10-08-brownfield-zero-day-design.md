# Brownfield zero day

Designed and settled 2026-10-08 through a one-fork-at-a-time review. Not built. The route-order defect it depends on is filed separately as #137.

## Problem

A brownfield trial (a single-developer desktop app adopted on 0.17.1) ran `derive-baseline`, and the human confirmed all ten as-is docs. Readiness passed and Tiller named the next step: `Next: decompose`. `.tiller/design/` held only the as-is docs and `baseline.yml`. There was nothing new to plan, and the agent had to explain that running decompose would either produce an empty plan or invent epics nobody asked for.

Three gaps produced that moment.

1. **The empty-tree rule counts epics, not inputs.** Once the baseline gate releases, `RouteDecisionOf` returns `decompose` whenever no epic exists (`internal/views/readout.go`). It never asks whether any design input describes work that is not already built. `check readiness` hard-codes the same `Next: decompose` line (`internal/cli/check_readiness.go`). Greenfield has the same blind spot; brownfield simply hits it on day one.
2. **The empty-tree rule outranks shelf work and amendment review.** On a tree with no epics, an owned approved bug or a proposed intake amendment still routes to `decompose`, so a maintenance-only project can never route to `execute`. This is a defect independent of this design and is filed as #137.
3. **The ingest found new work and had nowhere to put it.** `derive-baseline` read an unbuilt cross-device sync spec and an unbuilt section of the original spec (calculator units), and wrote both into the as-is docs as "not implemented." The original specs were kept as history, outside `.tiller/design/`. Routing could not see any of it.

## What zero day means

A project reaches zero day when its startup gates are satisfied and its tree is empty. For brownfield and legacy-migration that means a confirmed baseline and no epics. For greenfield it means no epics. Zero day has exactly two honest answers: there is to-be design to decompose, or there is no planned work yet and the human chooses how work enters. Tiller should say which, and never suggest planning from nothing.

Work enters a zero-day project through one of three doors, all of which already exist:

- **intake**, for a document that makes a case for change, including an old spec the human decides to build;
- **capture**, for a bug, chore, or tweak;
- **a to-be design** under `.tiller/design/`, which makes decompose due.

## Decisions settled so far

1. **Forward work found during the ingest is recorded as questions.** Not handoff prose, which vanishes with the session, and not a new leads ledger, which is a new concept. Questions already persist, already wait on a human, and an answer is already the recorded decision.
2. **Decompose is due only on an empty tree with at least one to-be doc.** A to-be doc is any file under `.tiller/design/` that is neither `baseline.yml` nor a path registered in it. An empty tree with no to-be doc reaches the new end state instead. The rule is the same for every project kind; only the end state's wording differs. Re-routing to decompose on a non-empty tree when a new to-be doc lands needs doc-to-epic provenance and is out of scope.
3. **Forward-work questions are plain questions.** No kind marker and no title convention. The title and statement make the case, and `--source` cites the unbuilt material. The end state reports one open-question count. An answer of "build it" leads to intake on the cited source; "it's a bug" leads to capture.
4. **One question per unbuilt capability.** A question cites every source that describes the capability. A code TODO counts only when a document corroborates it. An old implementation plan counts only where it describes a capability no spec covers. People decide by capability, not by file, and the corroboration rule keeps a large legacy repo from flooding the ledger.
5. **The end state is a second flavor of caught-up.** `RouteDecision` gains a flag (working name `Unplanned`) alongside `CaughtUp`. No seam, the human owns the move, and the agent stops. `guide`, the readout's Next, and `check readiness` all take their text from the route.
6. **Already-confirmed baselines get a standalone forward-work arm.** `derive-baseline` can run its forward-work pass alone, but only when a human names it. Nothing records that the pass ran and the route does not signal it. The few brownfield projects that predate the pass get it on request.
7. **Defect-shaped findings at ingest are questions, never captured bugs.** The ingest reads; it does not reproduce. A source-versus-code disagreement may be an intended change. Deciding that current behavior is wrong is the human's call, and capture's own trigger (a reproduced defect or the developer's description) is not met by an ingest.

## The route

The startup and empty-tree arms of `RouteDecisionOf` become, in order:

1. Brownfield or legacy-migration with no confirmed baseline doc: `derive-baseline`, unchanged.
2. Owned shelf work and proposed amendment review, wherever #137 places them. This design assumes #137 has landed: the empty-tree arm sits below every arm that can apply to an epic-less tree.
3. Empty tree with at least one to-be doc: `decompose`, reason naming the to-be docs (count, and the first path).
4. Empty tree with no to-be doc: caught-up with `Unplanned` set.

The route is a pure function of the loaded tree, so the tree needs to know its to-be docs. Tree load gains the list of to-be paths under `.tiller/design/`, gathered with the same confined walker `store.BaselineDocPresent` uses: regular files only, no symlinks, `baseline.yml` and ledger-registered paths excluded. Dotfiles are ignored. Greenfield already gaps readiness on an empty `design/`; that gap stays, and the route's answer for an empty greenfield tree with only ignored files is the same `Unplanned` end state.

A non-empty tree never consults the to-be list. Once epics exist, `decompose` is entered by a human naming it or by the skill's own trigger, as today.

## The end state

When `Unplanned` is set, Next is human-owned and reads, for brownfield and legacy-migration:

> Baseline established, no work planned. N open questions. Bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under `.tiller/design/`, which makes decompose due.

For greenfield:

> No work planned. Add design inputs under `.tiller/design/`; decompose turns them into a draft tree.

The open-question clause appears only when the count is nonzero, and names the count only, consistent with counts-only needs-attention elsewhere. The standing queue line is unchanged. `check readiness` on a ready project prints the route's Next instead of its hard-coded decompose line. `PoolSignpost` cannot fire on an empty tree, so the existing caught-up diagnostic needs no change.

The agent's behavior at this end state is the same as at any caught-up: report it and stop. It does not propose epics, pick a door, or open questions on its own.

## The derive-baseline forward-work pass

Both the derive and translate arms gain one step after the as-is docs are written and before the commit: record forward work. For each capability that a source describes and the code does not implement, open one question:

```bash
tiller add question "<what the sources describe, what the code has, and the decision to make: build it, or leave it as history>" \
  --title "<short capability name>: unbuilt" \
  --source "<path>#<section>[, <path>#<section>]"
```

The as-is doc keeps its "not implemented" note, which describes current behavior. The question carries the decision. The skill's readout section listing questions opened by the run already covers these, so no readout change is needed.

The standalone arm runs the same step on a project whose baseline is already confirmed, triggered only by an explicit human request such as "scan for unbuilt work." It reads the as-is docs' "not implemented" notes and their `--from` provenance, rereads those sources, checks the ledger for questions already covering a capability (re-adding the same statement is already an idempotent no-op, but a reworded duplicate is not), opens the missing questions, commits them, and stops.

Neither arm captures bugs (decision 7). The skill text says so explicitly, beside the existing instruction to record source-versus-code disagreements as questions.

## Docs and generated text that change

- `docs/spec/cli-surface.md`: route precedence (empty-tree arm conditions and placement), the `Unplanned` end state and its Next text, `check readiness` Next sourcing.
- `docs/spec/skill-set.md`: `derive-baseline` forward-work step and standalone arm; `decompose` trigger narrowed to "to-be design exists"; the orient section's caught-up description.
- `internal/gen/orientation.go`: routing rule 2 rewritten as the two empty-tree rules and moved below shelf and amendment rules; orientation generation bump.
- `internal/skills/derive-baseline/SKILL.md`, `internal/skills/decompose/SKILL.md` (trigger and description), `internal/skills/orient/SKILL.md` (its route list says "No epics are recorded ... the seam is `decompose`", and its onboarding stop names decompose as greenfield's first seam).
- `docs/plans/STATUS.md` and `docs/project-history.md` on completion.

## Build sequence

1. **#137, route order.** Move the empty-tree arm below owned shelf routing and amendment review; orientation rule order to match; route tests for an empty tree with an owned ready bug and with a proposed amendment. Ships alone.
2. **To-be inputs and the end state.** Tree load lists to-be docs; the empty-tree arm splits into `decompose` and `Unplanned`; Next, `guide`, and `check readiness` render the end state; spec and orientation text; gen bump. Route tests: brownfield confirmed baseline with no to-be doc reaches `Unplanned`; adding one to-be doc routes `decompose`; ledger-registered docs and `baseline.yml` never count; greenfield wording; unsafe paths ignored.
3. **Skills.** `derive-baseline` forward-work step and standalone arm, `decompose` trigger, skill-set spec. Verify by running the standalone arm on the trial project after release: it should open one question each for the cross-device sync spec and calculator units, and none for already-built material.

Builds 2 and 3 can ship in one release. Trial repos re-join after the gen bump.

## Out of scope

- Re-routing decompose on a non-empty tree when a new to-be doc lands (decision 2).
- A marker distinguishing forward-work questions from uncertainty questions (decision 3).
- Recording that a forward-work scan ran (decision 6).
- Any change to how intake, capture, or decompose work once entered.
