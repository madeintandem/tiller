# Bug severity and Fix board sorting: design

**Scope.** This plan adds a `severity` field to bugs, a `tiller grade` verb to change it, a severity requirement at bug approval, and sort and filter controls on the console's Fix board (`/fix`). It also changes the capture and code-review skills so agent-filed bugs arrive graded. The owning specs are [artifact-schema.md](../spec/artifact-schema.md) (the field and schema 13), [cli-surface.md](../spec/cli-surface.md) (`add`, `approve`, `grade`), and [ui-surface.md](../spec/ui-surface.md) (Fix board, node page, search). This plan changes each of them, and where the code and those specs disagree after the build, the specs are corrected in the same commit.

**Status.** Designed 2026-10-31 to 2026-11-01 with JC via grill-me (14 forks settled). Built and merged to main 2026-11-01 (ee23e1e), unreleased. Two rulings made during the build are folded in below: approve takes `--severity` only at triage (decision 6), and `status` and the readout's triage lines carry severity (decision 14).

## Why

The Fix board lists bugs, chores, and tweaks in each shelf's soft order. Nothing on the board says which bugs matter most, and nothing lets a reader see the newest work first. The code-review skill already grades bugs on a four-level scale, but only as prose at the top of `bug.md`, so the grade can't be sorted or filtered.

## Goals

- Every bug can carry a severity on the code-review skill's existing four-level scale.
- Every bug a human approves is graded.
- The Fix board can be sorted by shelf order, severity, or date added, and filtered by severity.
- Agent-filed bugs arrive graded, from the same scale definition humans read.

## Non-goals

- Severity in routing. `tiller next`, the readout, and the approve queue keep the shelf's soft order (decision 1).
- Severity on chores or tweaks (decision 3).
- Backfilling legacy bugs during upgrade (decision 5).
- Severity on the Report page, on Home desk rows, or in readout ordering (decision 14).
- A reverse-direction toggle for either sort (decision 9).

## Anti-goals

- **A fabricated grade.** No code path writes a severity nobody chose. A migration never stamps a default.
- **A board that silently disagrees with routing.** The default sort is the order routing hands out; any other order is one the reader chose.
- **Color as the only signal.** Severity is always spelled out in text.

## Settled decisions

1. **Severity orders the board only.** Routing, the readout, and the approve queue stay on the shelf's soft order. This overturns half of the code-review skill's ruling ("never a record field") and keeps the other half ("never a queue position"). A human who wants severity to drive routing acts on it with `tiller order bug`.

2. **The scale is code-review's four levels**, stored lowercase: `blocker | important | medium | minor`, with that skill's meanings, which are about impact, never the size of the fix.

3. **Bugs only, on the shared struct.** `Severity string \`yaml:"severity,omitempty"\`` goes on `store.WorkItem`, so every card carries one optional field and the board never branches on noun. Validation and the CLI refuse severity on a chore or tweak ("severity applies to bugs").

4. **Optional at add, required at approval.** A proposed bug may be unrated, because impact is often unknown when a bug is filed. `tiller approve bug` refuses an unrated bug, naming `tiller approve bug <node> --severity <level>` as the move. Bugs already past proposed when this ships stay unrated until someone grades them.

5. **No backfill.** `migrate` is deterministic and can't grade; a default would fake a judgment. An unrated legacy bug is only unsafe in the sense that it sorts after `minor`. Unrated bugs are visible on the board (an `Unrated` chip, the Unrated filter of decision 12) and are graded with `grade` (decision 6), for example by asking an agent to grade them and reviewing the `.tiller/` diff.

6. **Three ways to set severity.**
   - `tiller add bug <key> --title … [--severity <level>]`.
   - `tiller approve bug <node> [--severity <level>]`: required when the bug is unrated, and it regrades when the bug already has one, since triage is the human's call. On a bug already past proposed, `approve --severity` refuses (`severity_not_triage`) and names `grade`, rather than silently keeping the old grade.
   - New verb `tiller grade bug <node> <level>`: legal from proposed through in_review, refused on done or cancelled (a closed bug's grade is part of the record). It writes one `grade.bug` event with `before` and `after`. Any contributor, agent or human, may run it: since routing ignores severity, a regrade can't move work, so no human gate is needed. There is no clearing a grade, which keeps decision 4's invariant. `grade chore` and `grade tweak` are refused as an unknown noun.

7. **Skills.**
   - The severity table moves from `code-review` to `capture`; `code-review` points to it, so there's one definition.
   - `capture` always passes `--severity`, grading from the table. When impact truly can't be known yet, it leaves the bug unrated and says so in `bug.md`'s Actual section, so the approver knows to investigate.
   - `code-review`'s disposition passes the row's severity as `--severity`. The opening line of `bug.md` keeps confidence only. Line 67 becomes: "Severity becomes the bug's `severity` field when captured; it never becomes a queue position."
   - The approve queue entry and the readout show `tiller approve bug <key> --severity <level>` for an unrated bug, and the plain command for a rated one.
   - `orient` and the guide describe `grade` next to `rename` and `link`.
   - `execute` never regrades.
   - Gen bump.

8. **Sort options, Shelf order by default.** A "Sort" group in the Filters panel offers:
   - **Shelf order** (default): today's order, which matches routing.
   - **Severity**: blocker, important, medium, minor, then unrated. Chores and tweaks sort with unrated.
   - **Date added**: newest first.

   On the URL as `?sort=severity` or `?sort=added`; Shelf order is the bare default. `sort` joins `FILTER_AXES` in `filterStorage.ts`, so it's stored and restored per project with the filters.

9. **Tie-breaks, direction, Closed.**
   - Severity ties fall back to shelf order. Within the unrated band, chores and tweaks keep today's bug, chore, tweak shelf grouping.
   - Date-added ties fall back to event id, as `ClosedAt` sorting does.
   - One direction each: most severe first, newest first.
   - The sort applies to every column, Closed included. Under Shelf order, Closed keeps newest-closed first. Under the other sorts, Closed follows the chosen sort, but the `ROW_CAP` truncation still keeps the most recently closed items and sorts within them, so changing the sort never changes which closed items appear.

10. **Date added is derived from events, not stored.** It's the timestamp of the `add.<noun>` event, read the way `terminalStamps` reads `ClosedAt` (`internal/views/workitems.go`). This follows the existing rule: events drive display, history, and ranking hints (Report replay, node History, readout `--since`, reader affinity) and never gate a mutation. The Report's burn-up and flow already depend on `add.*` events, so any future event compaction has to keep them regardless.
    - A transfer replacement (`tiller transfer` hands the work to a new owner under a new id) takes the **original's** add date: the `transfer.<noun>` event's `After["replacement_id"]` maps back to `After["id"]`. Transfer also copies the original's `severity` onto the replacement, which it builds field by field (`internal/core/mutate/transfer.go`).
    - A superseding replacement is new work and has its own add date.
    - An item with no add event sorts last under Date added and shows no date.

11. **Card and node page.**
    - Every bug card shows a severity chip, always, whatever the sort: `Blocker`, `Important`, `Medium`, `Minor`, or a muted `Unrated`. The word carries the meaning; any tint is extra. Chores and tweaks get no chip.
    - Every card, all three nouns, shows "Added Oct 12", with the year when it isn't the current year.
    - The node page header facts block shows severity for bugs (`Unrated` when missing). `grade.bug` events appear in the History tab through `historyOf`.

12. **Severity filter.** A multi-select "Severity" group in the Filters panel: All, Blocker, Important, Medium, Minor, Unrated. A comma list read as OR (`?severity=blocker,important`); every member selected folds to All, as `normalizedNouns` does. It joins `FILTER_AXES`.
    - Any active severity filter narrows the board to bugs. Unrated means unrated bugs, the "needs grading" view, and never matches chores or tweaks.
    - It combines with Noun as AND. `noun=chore&severity=blocker` is a legal empty board; no options are hidden or disabled.

13. **Schema 12 to 13, with a no-op migration.** The YAML reader isn't strict, so an older binary would drop `severity` when rewriting a `bug.yml` (any claim, start, check, or submit). The schema fence makes older binaries refuse a schema-13 tree instead. The migration only bumps the stamp. Adoption on each managed project is `tiller upgrade --project-only` and a commit. The decision 7 gen bump rides the same release.

14. **Read surfaces.** `--json` and text output of the read commands that show a bug's fields (`show` and status for a bug, the readout's bug lines, approve queue entries) include `severity` when set. Search results get a severity chip on bug hits, reusing the card's chip component. Report, Home desk rows, and readout ordering are out of scope.

## Touch points

- `internal/store/workitem.go`: `Severity` on `WorkItem`; validation (allowed values; bug only).
- `internal/store/manifest.go`: `CurrentSchemaVersion = 13`; a registered no-op 12 to 13 migration.
- `internal/core/mutate/add_workitem.go`: `--severity` on add, refused for chore and tweak.
- `internal/core/mutate` approve path: the unrated refusal and `--severity` on approve; event `after` carries severity.
- New `internal/core/mutate/grade.go` and the `grade.bug` event type; the CLI verb and its `docs/tiller-commands.md` entry (the site `cli.md` is generated from it).
- `internal/views/workitems.go`: `severity` and `added_at` on `WorkItemCard`; an `addStamps` beside `terminalStamps`, following transfers.
- `internal/views/workitem_queue.go` and the readout: the approve command with `--severity` for unrated bugs.
- `internal/views/nodepage.go`: the severity fact for bugs.
- `ui/src/views/workitems/board.ts`: `sort` and `severity` axes, sort and filter functions.
- `ui/src/views/workitems/filterStorage.ts`: `sort` and `severity` in `FILTER_AXES` and `filterQuery`.
- `ui/src/views/WorkItems.tsx`: Sort and Severity groups, the severity chip, the Added line.
- The node header facts block and the search results component.
- Skills: `capture` (the table, `--severity`), `code-review` (pointer, disposition, line 67), `orient` and the guide (`grade`).
- Specs: artifact-schema.md, cli-surface.md, ui-surface.md.

## Open

None. JC settled all 14 forks.
