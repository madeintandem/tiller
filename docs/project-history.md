# Project history

The important decisions and completed work from Tiller's first specifications on June 20, 2026 through local 0.21.2 distribution on October 25. The latest authorized local installation is 0.21.2; the latest published release remains v0.11.3 from September 8. This is not a task queue or a replacement for the current specs. [Build status](plans/STATUS.md) names what ships and what remains open; [the five specs](spec/) own the current contracts.

This record replaces completed implementation plans, review transcripts, and repeated change histories inside the specs. It preserves decisions that explain the current design, not every fix, test run, or release stamp. Original records remain in git; [Retired records](#retired-records) explains how to recover them.

## October 25 cancel a question

Settle questions made a stale question costly: `approve feature` and `approve task` refuse while an open question holds the plan, and answering was the only way out, so a moot question, or one pinned to a whole feature when it touched one task, forced a fake answer that readers then took for a decision. The gap was closed on branch `cancel-question` in five code tasks and this docs task, designed with JC one fork at a time. Schema moves to **12** and orientation generation to **57**; the embedded skills stay at eleven. Nothing was installed or published.

Settled decisions:
- **Cancelled is its own state.** `tiller cancel question <Q-id> --reason` moves an open question to `cancelled` and releases every hold, as an answer does. `answer` keeps meaning a decision was made; cancel means the question is moot, mis-asked, or replaced.
- **Only an open question cancels, and cancelled is final.** Cancelling an answered or `resolved` question refuses `question_closed`; cancelling a cancelled one is a no-change success whose readout still prints. There is no reopen: a question that matters again is asked anew.
- **Cancel and unlink are the human's, link widens.** An agent that judges a question moot or too wide names the exact command in Notes. An agent may `link question --blocks` to widen, since that only holds more work.
- **History stays.** The `blocks` list, `cancel_reason`, and any feature `assumption_basis` entry naming the question remain on record. A cancel raises no `answered_question` drift because no answer arrives; its confirmation names each feature that assumed the question ("its plan stands as built").
- **The reason lives on the ledger, schema 12.** `cancel_reason` follows `answer_revision`; a cancel writes no revision. Older binaries rewrite the whole ledger loosely and would drop the field, so the version bump makes them refuse. The migration only stamps.
- **Narrow and widen are `unlink` and `link --blocks` on a question.** Targets resolve before any write, link unions in order, unlink preserves the order of the rest and may remove all, and a no-change call writes no event. Both apply only to an open question. Events are `cancel.question`, `link.question`, and `unlink.question`.
- **Refusals name the way out.** The plan-gate refusal offers cancel beside answer and still never mentions `--assume-question`; other transition refusals and the coarse submit refusal name cancel too. The queue, `tiller next`, and Home keep the single `answer_question` move, because no ranking should suggest a cancel.
- **Re-asking mints a new question.** `add question` skips cancelled entries when it deduplicates by statement, notes `re-asks cancelled <Q-id>`, and the amendment `add-question` operation follows the same path. Cancel, link, and unlink are not amendment operations.
- **Console.** Explore's Questions pane gains a fixed Cancelled tab (shown at zero) and cancelled cards show the reason; open cards and the question page carry copyable answer, cancel, and per-target unlink commands. `/api/ls` and the node response serve `cancel_reason`.

Verification: the Go suite across all packages, the console suite (90 files, 1,484 tests), lint, and the console and site builds passed on the branch tip. The retired [design](https://github.com/madeintandem/tiller/blob/be506cb8/docs/plans/2026-10-25-cancel-question-design.md) and [build plan](https://github.com/madeintandem/tiller/blob/16d9fad6/docs/plans/2026-10-25-cancel-question-implementation.md) remain in git at those commits.

## October 22 code-review remediation

The whole-source bugs and performance review found five defects, fixed in isolated worktrees and checked by two independent reviewers. Gate identity now refuses reuse when Git conversion could conceal working-tree byte changes, including clean/process drivers named `unset` or `unspecified`; its domain version retires earlier unsafe identities. Conversion-enabled repositories conservatively rerun gates rather than pay for full-checkout hashing. The ordinary non-conversion HEAD/diff path remains.

The Markdown corpus metadata byte cache is removed. Every walk reads and validates current confined bytes while bounded content-hash parsing reuse remains. Report now ships absolute weekly boundaries, including the final week's next Monday, so remote browser timezones and daylight-saving weeks cannot move completion dots or median lines into another bucket. Failed rubric recording reports failure rather than attestation. Readouts gather amendment preconditions and epic prose only where consumed.

The [review report](code-review-2026-10-22.md#remediation) records reproduction and verification. The full Go run passed all packages except a new VCS assertion that mixed Git warning stderr with changed-path stdout; after correcting that assertion, the complete VCS package and focused final VCS/mutation regressions passed. All 1,473 console tests, static checks, and the production console build passed. Real CLI and HTTP smoke scenarios exercised the original failures; Chromium verified production chart coordinates with a controlled completion response. For an orient readout with 20 guarded operations, Git calls fell from 27 to 7 with identical output. No schema, orientation, installation, release, or managed-project change occurred.

## Local 0.21.2 distribution

**October 25, authorized local distribution.** Commit `5f8c218e836ea23804b4033b4b1faf02ebb6a9b1` stamps **0.21.2**, carrying [cancel a question](#october-25-cancel-a-question) at orientation generation **57** and schema **12** (the additive `cancel_reason` field), plus the gate reuse test fix that turns main CI green: `TestCheckDoesNotReusePassOverConversionHiddenShellBytes` ran its gate as `sh probe.sh`, which bash fails on `exit 0\r` but dash (Linux `/bin/sh`) accepts, so CI had been red since 0.21.0; the gate now fails on a carriage return through `tr` and `cmp`. The eleven embedded skills are unchanged. v0.18.4 remains the latest published release. No tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.21.2-5f8c218e/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `05859e9d191dab3250b22a95979ad0d88777665cbd576a2cd28d601a6bc19b51`.

Checks:
- Lint, the full Go suite (27 packages), the console suite (1,486 tests), the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- On a fresh scratch project the installed binary reported 0.21.2, stamped schema 12, and wrote orientation generation 57 naming `tiller cancel question`.

Managed projects need `tiller upgrade --project-only` to pick up schema 12 and generation 57; review and commit the migration and orientation refresh, reload agents, and restart running consoles.

## Local 0.21.1 distribution

**October 22, authorized local distribution.** Commit `8f47e9dacb0d74d3911af039297cb042c24ea4e6` stamps **0.21.1**, carrying settle questions before approve (issue #152, item 1) at orientation generation **56** and the verified code review fixes. Schema **11** and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.21.1-8f47e9da/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `ed35f30b4fa1e491f9647e61c2863bc6ae94feceba50865c93b2c3a1a1aef06b`.

Checks:
- Lint, the full Go suite (27 packages), the console suite (1,478 tests), the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- On a fresh scratch project the installed binary reported 0.21.1 and wrote orientation generation 56 with the plan-gate route naming `tiller answer question`.

Managed projects need `tiller upgrade --project-only` to pick up generation 56; review and commit the orientation refresh, reload agents, and restart running consoles.

## Local 0.21.0 distribution

**October 22, authorized local distribution.** Commit `256a595bcccafeca0edbf82c00bf6d840eaa5592` stamps **0.21.0**, carrying [ranked next](#october-21-ranked-next) at orientation generation **55** and gate reuse in `tiller check` at generation 54, with the specify preserve-criterion warnings. Schema **11** and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.21.0-256a595b/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `c45d64456eb384f4f9207550757a0c64481f5b964eae45b3b3cbbb6bff0af88a`.

Checks:
- Lint, the full Go suite (27 packages), the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- On a fresh scratch project the installed binary wrote orientation generation 55, printed "Caught up" with the no-work-planned hint before a design doc existed and a ranked decompose row after, reported 0.21.0 from `/api/version`, served the same row from `/api/next`, and answered 404 for the retired `/api/route`.

Managed projects need `tiller upgrade --project-only` to pick up generation 55; review and commit the orientation refresh, reload agents, and restart running consoles.

## October 21 ranked next

**Both builds, stamped in 0.21.0.** Home opened on a live project with a "Route says" strip naming one move and an Order note explaining what it had skipped, above a reader-independent Up next column; `submit` and `check` printed no next step at all. The route walk returned the first of sixteen arms, so everything it passed over survived only as prose notes. The [ranked next design](plans/2026-10-21-ranked-next-design.md) was built in two parts. Build 1 replaces the walk with one engine, `views.NextOf`, that enumerates every legal move for the reader's checkout, human decisions included, ranks them by a fixed rule over the reader's own recent events, and returns them best first. The CLI contract is [Ranked next](spec/cli-surface.md#ranked-next).

- **The surfaces.** New `tiller next` (`--all`, `--json`, `--contributor`) prints the top five. The readout's **Next:** is the same list, bare `tiller guide` prints it and suggests the skill for the highest-ranked agent move, and `submit` and a passing `check` append an **Up next:** block. `start`, `claim`, and `resume` print nothing new. The Order, Continuity, and Execution notes, the "Next eligible" advisory, specify's and decompose's merged counts, and orient's `Route` verification line are gone.
- **The rule.** Five tiers: unblock nearby (human moves holding the current feature, plus questions overdue or due within two days), continue, nearby, recently touched within fourteen days, everything else. Only deliberate acts place the reader, and adding work touches only its parent. A node appears once, and the reader's top Continue node is always kept. Tiller never calls a model to rank.
- **Skills and orientation.** Orientation generation 55 rewrites the route guidance as ranked next. The orient skill relays the list as written, names any human move ranked above the top agent move, and starts that agent move in its next turn unless the developer's reply redirects; it ends its reply on the list so the developer's next message is the go or the redirect. Skills that end on a readout reproduce the list verbatim.
- **Console, through build 1.** `/api/route` survived build 1 as an adapter that projected the list's top move onto its old payload, so Home kept working had build 1 shipped alone. Build 2 removed it.

JC approved the plan's ten rulings as written: an event naming more than three targets does not count, amendment events never move the current feature, kind order then plan order breaks ties inside a tier, a claimed but unstarted task is Continue, deadlines rank with no reader, caught up keeps the unclaimed-work explanation, orient drops its `Route` line, the readout's `next` becomes the ranked object with no route advisory, `/api/route` survives as an adapter, and orientation takes the next free generation (55, since gate reuse took 54). Rulings made while building, each recorded in the CLI spec:

- Epic approval offers every draft epic the Queue lists, including one with no live features, which the walk had refused as an empty shell.
- The reader's own task observed in flight before its claim reached this checkout is Continue.
- A feature held by a question is not also a blocker row; the question is the row.
- A tree with no epics and no to-be design doc keeps the no-work-planned hint beside any candidates, such as an open question. The final review narrowed this (see Build 2). A brownfield tree whose only move is its pending baseline is not marked unplanned.
- Confirming a pending baseline doc is a human row whenever a doc is unconfirmed, not only while the startup gate holds. With only those confirmations legal, bare guide suggests orient rather than derive-baseline.
- Bare guide can suggest `specify` while a human approval tops the list, since it follows the top agent move.
- A row's reason never restates its own node or command.

Verifying build 1 on a disposable project surfaced four more, ruled by the build's controller and fixed before the build closed:

- **Adds touch their parent.** Right after a decomposition the last feature added was the current feature and earlier siblings ranked in reverse creation order, so specify suggested the last-added feature first. An `add` now touches only the added node's parent for recency (a task its feature, a feature its epic, an epic nothing) and never sets the current feature or epic, as decision 12's "counts against its parent" intended.
- **Recency by day.** Recently touched orders by whole elapsed days, the grain its "today" and "N days ago" copy speaks in; same-day touches keep kind and plan order.
- **Human rows keep what they wait on.** A human decision placed by locality keeps its own reason after a semicolon ("in your current feature; in review on branch ..., waiting on your merge"), so a merge row still says it waits on a merge. Agent rows keep the tier reason alone.
- **Checked work reads as checked.** A task in `check` reads "your task is in check; finish any outstanding checks, then publish and submit it" rather than "awaiting its check".

Two findings stand as designed: one row per node hides a task's optional attests behind its merge row, and a question holding a draft feature does not remove the draft's specify row, since `spec feature` is not question-guarded. `join` and `init` now describe bare guide as listing the ranked next moves.

### Build 2, the console

`GET /api/next` serves the `views.Next` payload `tiller next --json` prints, with candidates cut to the shortlist, ranked for the served checkout's contributor; a test compares the handler's body with the real CLI's `--json` data. `/api/route` is gone and answers 404. Home drops the Route says strip. Dependency diagnostics, which rode the route payload, are their own section between the header band and Needs you, fed by `/api/next`. Up next renders the list as served: each row leads with `You` or `Agent`, then the action, a link to the node, and the reason as a sentence, with the command left to Needs you and the CLI. The header names the reader, `Up next · for <name>`; with no identity it reads `Up next` over "Not personalized; run `tiller join`." Caught up prints its sentence and the list's hint. The Recovered reviews tab takes its records only from the epic and feature lists, since no ranked move carries one. In flight is unchanged except that its pool line no longer claims Up next lists the same tasks. The [UI spec](spec/ui-surface.md#home) carries the Home contract.

One ruling while building: the header names the reader by roster display name, the way In flight names people, falling back to the contributor id. The design's header says "for <contributor>" without saying which label; the console shows people by roster name everywhere else.

The final whole-branch review settled the remaining copy: closeout and aggregate merge rows keep a one-line reason (`shared closeout: <phase>`, or what the merge waits on) while Needs you keeps the full advice, a deadline row keeps the question's title after `due tomorrow` or `overdue (was due <date>)` with today read as the reader's local date and Home labels question links by title, no merge is "your" merge without a reader, an observed Continue row reads `your task, in flight on its branch` without repeating the branch, guide's claim suggestion names the task, bare guide no longer repeats the caught-up hint as its reason, and the no-work-planned hint stands only when nothing but answers and design confirmations is legal.

### Overturned

Ranked next overturns two [October 9 readout continuity](#october-9-readout-continuity) decisions: "Owner affinity, no flag", which ranked by task ownership and refused event-stream reads, and "Route only", which kept Up next reader-independent. The Continuity note and the Order note retire with the walk.

### Verification

The old walk's tests were rewritten as enumeration and ranking tests, with its arm-order cases kept as plan-order tie-break cases, and each task passed an independent review. `make lint` and the full `make test` passed on the branch. A disposable two-contributor project (a specced feature with one task, a draft feature, and a question due the next day) drove `tiller next`, `tiller next --all --json`, bare `tiller guide`, the `approve feature` readout, `check`, and `submit`; each printed the ranked list, and `tiller serve`'s `/api/route` answered with the list's top move.

For build 2, `make test`, `make lint`, and the console suite passed, and `ui/dist` was rebuilt. A disposable project with two contributors (a feature with a recovered review awaiting its feature check, a specced feature awaiting approval, a draft feature held by a question due the next day, and two tasks in a recorded dependency cycle) was served twice, from a checkout joined as jc and from an identity-less clone, and Home was rendered in headless WebKit in light and dark themes. The joined checkout's header read "Up next · for JC Grubbs" with personal reasons; the clone's read "Up next" with the not-personalized line and plan-order reasons. Rows showed You and Agent labels and node links, the cycle rendered as an execution dependency blocker, the Recovered reviews tab showed the one record, no Route says strip appeared, and `/api/route` returned 404 while `/api/next` returned 200.

Nothing was stamped, installed, published, or applied to a managed project; joined checkouts refresh to generation 55 on their next `tiller upgrade --project-only`.

## Local 0.20.8 distribution

**October 21, authorized local distribution.** Commit `7cdf1c56a20a5657db042d4d726757bd36a2914b` stamps **0.20.8**. The Report's epic progress chart never drew an In progress segment. It bucketed features by stored status, but the start that activates a feature is recorded on its task's branch, so the integration checkout only ever sees a feature go from ready to done, and the Report read no execution observations. Epic progress now places each feature in its Build board column over the observed tree, so a ready feature with a task in flight, on main or observed on a branch, counts as in progress. Every other Report metric still reads the integration tree its event replay describes ([Report](spec/ui-surface.md)). On the Acme project one feature moved to In progress, and the rest of the Report payload was byte-for-byte unchanged against 0.20.7. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.8-7cdf1c56/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `cd7d50b5067536df2bba3349bf7370f5f3f0d2dca4f2cd565db7e3d9fe28b0e8`.

Checks:
- Lint, the full Go suite (27 packages), the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary reported 0.20.8 from `/api/version` on the Acme project and served a Report with one feature in progress.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.7 distribution

**October 21, authorized local distribution.** Commit `685985a194338015d669fc1bcccba9e411d5615c` stamps **0.20.7**. A finished feature's recovered review rendered as the largest block on Home, ahead of the needs-you section, and stayed there because a recovered record never stops being true. Recovered review records now live behind a Recovered reviews tab beside Overview and Branches (`?tab=recovered`), drawn only while a record exists. Its count takes the warning tone while a record leaves historical riders pending or current acceptance unresolved. The route strip names the route's own recovery as a link to that tab instead of inlining the full record. An in-review item's recovery in the all-clear list stays on the desk, since that item still waits on the human. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.7-685985a1/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `7729f083d4019e7f03d11671e74aefcb15e0c6ae14b2f36173c67e175f928913`.

Checks:
- Lint, the full Go suite (27 packages), the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary reported 0.20.7 from `/api/version` on the Acme project and served the console bundle carrying the Recovered reviews tab.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.6 distribution

**October 20, authorized local distribution.** Commit `b8e1da434907750ca9387e4026d8172b968003b3` stamps **0.20.6**. Explore node pages on large projects sometimes hung for seconds. Each Browse pick issues eight reads, and the console only ignored a superseded response: the request kept one of the browser's six connections per origin until the server answered, so a few quick picks queued the newest node page behind every abandoned read. Every console read now aborts its request when it is superseded or unmounted. The corpus walk behind every node and map page also reuses a document's bytes while its file identity, size, modification time, and mode are unchanged, never keeping a document modified within two seconds of the walk that read it ([console read cost](spec/ui-surface.md#liveness)). Skipping `events/` in that walk was measured at about 20 of 150 milliseconds and left out. On the Acme project, the last of five picks 400 milliseconds apart went from about 4 seconds to 1.2-1.5 seconds, and the walk from about 151 to 79 milliseconds. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.6-b8e1da43/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `e24f1f87239d65be81b4ba29e0d59642b611d82d2138c490dfb9bb6c3f1412b2`.

Checks:
- Lint, the full Go suite (27 packages, at a load average near 20), the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary reported 0.20.6 from `/api/version`, served a node page, and served the console bundle carrying the request abort.

Managed projects need no migration. Restart running consoles to pick up the new binary.

Open: Explore still rereads the navigator on every pick by design; changing that needs a ruling. The remaining per-read floor on large projects is mostly execution observation Git reads across worktrees.

## Local 0.20.5 distribution

**October 20, authorized local distribution.** Commit `eb9be39272f53b7750035bfe82f848064400c186` stamps **0.20.5**. The Report's Epic progress chart shows each not-yet-approved (draft) epic as a ghost row in planned order: a muted title, an empty dotted track, and its planned feature count ("2 features planned", or "Not yet approved" when it has none), with a matching legend entry; ghost rows never count toward scope, burn-up, or the KPIs. The topbar takes a one-pixel shadow on Home, Report, and History, the views that scroll beneath it, and the refresh glyph grows to 16 pixels. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.5-eb9be392/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `4d924e7b60a588e2e45e5a652dc72c1ce2a3741c397ffb0db1521b7d045e7507`.

Checks:
- Lint, the full Go suite (27 packages, 184 seconds of wall time at a load average between 10 and 15), the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary reported 0.20.5 from `/api/version`, served `/api/report` with a draft epic as a planned row, and served the console bundle carrying the topbar shadow.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.4 distribution

**October 20, authorized local distribution.** Commit `2a05313d4c2e4841900ef7c00959db12ae9dc617` stamps **0.20.4**, carrying status tooltips on the Explore tree's glyphs: hovering a glyph shows its status label (for example Specified, In Review, Done) in a small tooltip above it, flipped below near the top of the window and kept inside the window's edges. The label replaces the SVG `<title>` and is also the glyph's accessible name; touch taps do not leave it showing, and it hides on scroll. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.4-2a05313d/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `f34c236dd8641f3f0d632a9498bc3d163dfb603aa2c21d2b5c8a53fcd657a330`.

Checks:
- Lint, the full Go suite (27 packages, 134 seconds of wall time at a load average between 3 and 8), the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary reported 0.20.4 from `/api/version` and served the Explore console route whose bundle carries the status labels.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.3 distribution

**October 20, authorized local distribution.** Commit `3519ffda44c5379552f92131030683314cf18fe5` stamps **0.20.3**, carrying two console layout fixes. The topbar now sticks to the top of the viewport, so long page-scrolled views (Home, Report, History) keep the nav in reach. Explore, Build, and Fix size their frames from a `--topbar-h` token that App now measures from the rendered header and follows on resize; the old constant said 52 pixels where the header renders 60 (and taller when its row wraps), which left those frames 8 pixels past the viewport, slack that scrolled under the sticky header. The stamp also restores the CRLF line endings in the site notices' Space Grotesk license, which the Apache 2.0 license commit had normalized. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.3-3519ffda/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `a72d7dddeb7fedf0525686bf87a5075055e5b380d7917f3f2f6d264920f47b97`.

Checks:
- Lint, the full Go suite (27 packages), the console suite (1426 tests), the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- Against a disposable clone of a managed project, in WebKit and Chrome at 1280 pixels the header stayed at the top of the viewport after scrolling Home, Report, and History to the bottom, and Explore, Build, and Fix no longer scrolled the page; at 700 pixels the header wrapped to 97 pixels and the token followed it.
- The installed binary reported 0.20.3 from `/api/version` and served the console bundle carrying the sticky header and the measured token.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.2 distribution

**October 20, authorized local distribution.** Commit `7f11dc5ded05d6b4ed0a7d100a709f063204a6ae` stamps **0.20.2**, carrying [the test suite speedup](#october-20-test-suite-speed) and the Apache 2.0 license change with `NOTICE` and `TRADEMARKS`. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.2-7f11dc5d/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `4418a19e8e0e189986236225aeac6e03cff8d6451375d6af1616b7e5576c0650`.

Checks:
- Lint, the full Go suite (27 packages, 291 seconds of wall time at a load average between 9 and 19, `internal/cli` the slowest at 288 seconds), the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- The installed binary initialized a disposable project at orientation generation 53, reported 0.20.2 from `/api/version`, and served `/api/report` and the `/report` console route.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## October 20 test suite speed

**Source only, test code and one Makefile line.** The full suite took about 5.5 minutes, held up by two packages: `internal/core/mutate` (325 seconds) and `internal/cli` (154 seconds), almost all of it spawning git to build fixture repositories. Three changes cut it without removing, skipping, or loosening a test.

- **Cheaper git setup.** The new test-only package `internal/testgit` (`Init`, `Run`) replaces the repeated init-plus-five-config sequences with one `git init -b main` and a direct write of the settings to `.git/config`. It always creates `main`, so a test no longer depends on the machine's default branch name. On macOS the `test` recipe also puts the real git from `xcrun --find git` first on PATH, because `/usr/bin/git` is a shim that costs an extra spawn per call; the lookup is empty on Linux or without Xcode git, so PATH is untouched there.
- **Repositories laid once, then copied.** `testgit.Template`, `Templates`, and `CopyWithResult` lay a fixture repository once per process (and once per `GIT_*` environment, so a sha256 test cannot poison the shared copy) and hand each test its own copy. A lay that fails, errors, skips, embeds its own path, or contains a symlink marks the template failed, and every caller fails loudly instead of running against an empty repository. mutate's lantern, owned, gated, gated-task, epic-feature, feature-branch, and speccable-feature fixtures use it.
- **Parallel mutate.** 842 of mutate's 844 top-level tests now call `t.Parallel()` first. The two that set process-wide environment stay serial with a `// Serial:` comment. A scan of mutate's package-level state found one shared write, the lantern counter, which now travels with the template snapshot.

Measured on a shared machine, so load matters. Baseline: full suite about 5.5 minutes, mutate 325s, cli 154s. After: `make test` 153s wall (load 16.7 at start, 9.2 at end), mutate 59.5s in isolation at load 2.6 (76s at load 8.5, 99.5s inside `make test`), cli about 150s. cli is now the slowest package. Individual steps were noisy: mutate went 142s to 59.5s at matched load across the parallel change, and about 16% faster from templates in a concurrent same-load comparison.

One flake surfaced under `-race` at load 23: a rename test treated a rename that had not returned within 250ms as blocked on a fifo, and a slow run tripped it. It was a wall-clock threshold, not shared state. The wait is now 10 seconds, which still detects a real block (one never returns) and leaves the assertion unchanged. Later `-race` and `-count=3 -shuffle=on` runs passed.

**Deferred scope.** The remaining cost is `internal/cli`, whose tests run serially because each CLI run reads the process working directory and environment. The planned fix is an invocation seam so each run carries its own working directory and environment, then parallel cli tests; it is expected to take cli from about 154s to about 30s and the suite to about a minute. It stays open in [build status](plans/STATUS.md).

The plan is retired; its unbuilt Tasks 4 and 5 are open scope in [build status](plans/STATUS.md). Recover it with `git show bed9ac2a:docs/plans/2026-10-20-test-suite-speed-implementation.md`.

## Local 0.20.1 distribution

**October 20, authorized local distribution.** Commit `c4b9f41f0b82a82cffab047d423a4e8e656a3052` stamps **0.20.1**, carrying the Report chart enlarge control: a magnifier on each chart card opens the live chart in a modal dialog 1100 pixels wide (or 92 percent of a narrower window), weekly charts draw one and a half times as tall there, and a tooltip in the lower half of a plot opens above the pointer. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.1-c4b9f41f/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `f8200c569074f07c4dbec82e9a2d1fe44f8813f18af83aa87dee2fd010ae6d6b`.

Checks:
- Lint, the console suite, the doc-version, version-stamp, and client-privacy checks, and the site build passed on the stamped tree.
- In the full Go suite every package passed except `internal/core/mutate`, which hit Go's 10 minute default timeout under heavy machine load with no test failure; rerun alone with a 30 minute timeout it passed in 308 seconds.
- The installed binary reported 0.20.1 from `/api/version`, served `/api/report`, and served the `/report` console route whose bundle carries the enlarge control.

Managed projects need no migration. Restart running consoles to pick up the new binary.

## Local 0.20.0 distribution

**October 20, authorized local distribution.** Commit `9f0ceb06543ea7a3e4dad8fce25c7c3267926a13` stamps **0.20.0**, carrying [the Report page](#october-20-report-page) and the fixes from its code review. Schema **11**, orientation generation **53**, and the eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.20.0-9f0ceb06/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `0df6aba8f3c7a24bf1b64fe107b5c152232c324ee31d2c016b7c73fa86fd8da5`.

Checks:
- Lint, the full Go suite, the console suite, the doc-version check, the version-stamp check, and the site build passed on the stamped tree.
- The client-privacy check flagged a client project name in the Report history entry; the docs commit `0af4415b` replaced it with the Acme stand-in before the stamp, and the check then passed.
- The installed binary initialized a disposable project at orientation generation 53, reported version 0.20.0 from `/api/version`, served `/api/report` with the expected burn-up scope, and served the `/report` console route.

Managed projects need no migration for this stamp. Restart running consoles to pick up the new binary and the Report page.

## October 20 Report page

**Shipped in local 0.20.0.** The Report page is a client-facing metrics screen, a sixth item in the primary nav (Home, Explore, Build, Fix, Report, History). It is presented live, with the developer driving, and its reader is the client product owner. It shows metrics only: a KPI strip and five charts (feature burn-up, throughput, bugs reported vs fixed, lead and cycle time dumbbell, epic progress). It replaced the earlier, unbuilt status-rollup Report and `tiller report` designs, which are retired.

- **Computation and transport.** `views.ReportOf` replays the event log (read through the process cache) plus the current tree for scope membership and parentage, and `GET /api/report` serves the series. Metric definitions live only in Go, deterministic and unit-tested; the UI receives series, not events. The charts are hand-written SVG with one shared hover tooltip; no chart library was added.
- **Scope.** Scope is the non-cancelled features under approved epics (ready, done, or archived). Draft epics and their features are proposals, not scope. A feature enters scope at `approve.epic` or `add.feature` under an approved epic and leaves at `cancel.feature`. Epic progress uses the same rule.
- **Sync-time dating.** Items are dated by the `sync` event that observed the merge, not the merge itself. Walking Git history for merge times would undo the console read-performance work, and weekly buckets absorb the lag.
- **Labels.** Throughput stacks Feature work (tasks), Fixes (bugs), and Upkeep (chores and tweaks). A bug carried by another item's merge counts once, in the week its carrier lands.
- **Cycle and lead time.** Cycle time runs from the first task start to the latest done; lead time from the first feature approval to the latest done. A feature returned to specced and re-approved keeps its original approval date. Durations are calendar days, one decimal under 10 days (anything under a tenth reads "< 0.1 d").
- **Reconciliation.** A node with no recorded change (a migrated or hand-edited ledger) starts in its current status at the week of the project's earliest event, and a node whose last recorded change disagrees with the tree gets a closing change to its current status at read time, so the burn-up's last point agrees with epic progress. An event that only restates a status, such as an eval correction on a done feature, is not a change, so a later correction never reads as a new completion. The dumbbell plots only features with real approval, start, and completion events; no duration is invented.
- **Transfers and the clock.** Transfer events are replayed per target, and events dated after now are clamped to now. The axis spans at most ten years (520 weeks) ending at the current week, an event before it counts at its start, and an event with a zero timestamp is ignored.
- **Honest sparse history.** Time charts span at least four weeks, and each empty chart carries its own one-line empty state. When the checkout is off the integration branch, an amber notice says numbers include unmerged work.
- **Color blindness.** The developer is color blind, so shape and line style carry meaning beside hue: the dumbbell uses a filled dot for cycle time and a hollow dot for lead time, with a solid median line for cycle and a dashed one for lead, and epic progress segments use the console state palette. The built page added more: throughput's Fixes segments are hatched as well as colored, with a matching hatched legend swatch; on the burn-up, Scope is dashed and Done is solid, with direct end labels; partial-week pattern ids are unique per chart; and tests pin each chart's non-hue encoding.
- **No people.** Nothing on the page names, counts, or ranks contributors.

Deferred scope stays in [build status](plans/STATUS.md): Report export (print, Markdown, static HTML) and an executive summary with period comparison. The [console spec](spec/ui-surface.md) describes the page as built.

Verification: the full Go and console suites passed. Against the Acme platform project (about 5,600 events), `/api/report` took 0.32 seconds cold and about 0.12 seconds warm.

The completed design and implementation plan are retired. Recover them from the branch history with `git show a7c54d26:docs/plans/2026-10-20-report-page-design.md` and `git show a7c54d26:docs/plans/2026-10-20-report-page-implementation.md`.

## Local 0.19.3 distribution

**October 19, authorized local distribution.** Commit `f7f4a42aa320f9c110f58cec3eec849cde2d80c9` stamps **0.19.3**, carrying [the field issues #146 to #150](#october-19-field-issues-146-to-150). Schema moves to **11** (an additive 10-to-11 stamp migration) and orientation generation to **53**; the eleven embedded skills are unchanged in number. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.19.3-f7f4a42a/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `f1dec509e783954a743083e18b6ef0f26af4df8109f468c744874ed1648b9813`.

Checks:
- Lint, the full Go suite, the console suite and build, the doc-version check, and the site build passed in that checkout.
- The client-privacy check flagged one client project name in the field-issues plan; the follow-up docs commit `23256995` replaced it with the Acme stand-in, and the check then passed. The binary is unaffected.
- The installed binary initialized a disposable project at schema 11 and orientation generation 53 and served `/api/status`, `/api/route`, and the console.

Managed projects keep schema 10 and their generation until refreshed: `tiller upgrade --project-only` runs the 10-to-11 migration and regenerates orientation. Restart running consoles to pick up the new binary.

## October 19 field issues 146 to 150

**Shipped in local 0.19.3.** A teammate's field run raised five issues, settled together in [the plan](plans/2026-10-19-adayag-field-issues-plan.md). Schema moves to **11** and orientation generation to **53**.

- **A started task may add an edge to a done task (#148).** A dependency edge onto a done prerequisite no longer refuses once the dependent has started, and the refusal that remains never names an amendment.
- **A task keeps the grain it started under (#146).** `start` records `started_grain` in `task.yml`. A task started at task grain under a feature that later went coarse takes its own PR or review ref and crosses on its own merge, and the aggregate never absorbs it. Readout lists such tasks as `grain_notes`, `configure` prints a commit note, and the submit and start refusals say the task started at task grain. Schema 11 is the stamp-only migration for the field.
- **Adopt an existing aggregate (#147).** `start task --adopt-aggregate` records an aggregate branch that already exists at the derived name, with `aggregate_unrecorded` and `aggregate_adopt_*` reasons for the refusals. A task branch is never reinterpreted as an aggregate.
- **Attest stays on the aggregate (#150).** `attest feature` refuses off its coarse aggregate branch (`attest_branch_mismatch`). The workflow handoff, the `check` human-review line, orientation, and the execute and code-review skills name that branch, so the human attests and commits there before submit. Recovered closeout, which runs on integration, is exempt.
- **Routing honors observed execution (#149).** Route guidance mutes the reader's own in-flight task, skips tasks another owner is running, and says when work can run in parallel, on the CLI and in the console.
- **Ruling R2.** A recorded aggregate branch locks every review grain change on that feature. The spec now says what the code always did; no behavior changed.

## Local 0.19.2 distribution

**October 19, authorized local distribution.** Commit `3ce1d0c146ca69ec9a1874ea6db098dd93804c62` stamps **0.19.2**, carrying [Build board columns](#october-19-build-board-columns). Schema **10**, orientation generation **52**, and eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.19.2-3ce1d0c1/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `ed1631c7aef1fe0b5721c976e9a634d678def1e3b09d2be127a5b340468f0347`.

Checks:
- Lint, the full Go suite, the console suite and build, the doc-version and version-stamp checks, the client-privacy check, and the site build passed. One uncached rerun of the cli package failed without captured output; two further reruns passed.
- The installed binary served a disposable project's console with the rebuilt bundle. With a task started only on its branch, `/api/build` and `/api/ls` both placed the ready feature in the active column.

Managed projects need no refresh: orientation generation is unchanged. Restart running consoles to pick up the new bundle.

## October 19 Build board columns

**Shipped in local 0.19.2.** On a live project board, JC saw a feature card list every one of its tasks outside the expand, and saw the feature sit in Ready while its tasks were in progress and in review.

- **No assignment list (JC).** The card's current task assignments list duplicated the chevron expand. It is gone from the card and from `/api/build`; a card shows Contributors and the rollup, and its tasks appear only in the expand.
- **Ready with work in flight reads Active (JC).** Under feature-grain review, the start that moves a feature to active is committed on the unmerged review branch, while the console observes task execution from task branches (#133). The served feature still read ready. `gen.FeatureBoardStatus` now places a ready feature with a task in progress, in check, or in review in the Active column. JC chose this display-only derivation over reading feature status from review branches, which would break #133's task-only observation rule. The stored status is never replaced.
- **Home agrees (JC).** Feature rows from `/api/ls` carry the same column, and Home's funnel counts it, so the funnel still matches Build's columns. Build's epic mix bar counts columns too.

## Local 0.19.1 distribution

**October 18, authorized local distribution.** Commit `bb4f72ebff37ee178e844b0e5d39e980d4c9ca9b` stamps **0.19.1**, carrying [Explore status glyphs](#october-18-explore-status-glyphs). Schema **10**, orientation generation **52**, and eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.19.1-bb4f72eb/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `abfb8ed15f0d38192c701d47a86248c7a0dab371e632247686d3fece7e381290`.

Checks:
- Lint, the uncached full Go suite, the console suite and build, the doc-version and version-stamp checks, the client-privacy check, and the site build passed.
- The installed binary served a disposable project's console with the rebuilt bundle, and its API answered.

Managed projects need no refresh: orientation generation is unchanged. Restart running consoles to pick up the new bundle.

## October 18 Explore status glyphs

**Shipped in local 0.19.1.** JC is slightly color blind, and the Explore tree's small status dots were too small and too close in color to tell apart. He mostly wanted to see status and spot incomplete features and tasks. The epic progress bars already read well and stay as they are.

- **Shape carries status (JC).** Every tree and shelf row shows a 14px glyph (15px on epics) whose shape names the folded state: dashed ring for draft, ring for specced, ring with a dot for ready, half-filled for active, three-quarters filled for in review, a filled check for done, and a filled bar for blocked. Cancelled, archived, and unknown statuses draw the draft ring. Color stays as a second cue. The glyph's tooltip and accessible name give the status word, so a shelf link's accessible name now leads with it. Dots elsewhere in the console are unchanged.
- **Done fades (JC).** Done, answered, cancelled, and archived rows fade title and glyph so incomplete work stands out. Blocked rows get a faint blocked tint and a bold title.
- **Hide cancelled (JC).** A third tree action hides cancelled features, tasks, and shelf rows, and their counts. Epic bars keep the full child mix. The choice is remembered per browser in a flat localStorage key, so it applies across projects. A deep link to a hidden cancelled node shows no highlighted row.

## Local 0.19.0 distribution

**October 18, authorized local distribution.** Commit `78213d9977e91a5d5e423dc8f442831970b7a5e2` stamps **0.19.0**, carrying [unit-of-work worktrees](#october-18-unit-of-work-worktrees). Orientation generation moves to **52**; schema **10** and eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.19.0-78213d99/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `c81d3a7054abd27a0f2b638acd87007a8171a5d16bf1fea26340d3c76577d045`.

Checks:
- Lint, the uncached full Go suite, the doc-version and version-stamp checks, the client-privacy check, and the site build passed on the stamp.
- On a disposable project the installed binary scaffolded orientation generation 52 with the unit-of-work rule, and `tiller guide execute` printed the new worktree section. A generation-51 project reported the mismatch in `tiller check readiness` and `tiller upgrade --project-only` refreshed it to 52.

Managed projects were not refreshed; each needs `tiller upgrade --project-only` to pick up generation 52.

## October 18 unit-of-work worktrees

**Shipped in local 0.19.0.** Generation 49 gave every started task its own worktree and added another for aggregate assembly. At feature or epic grain that splits one review across several checkouts, each joined separately, with the aggregate in yet another. JC asked for the worktree to follow the unit of work instead: whatever one PR covers under review grain.

- **The review unit gets the worktree (JC).** Orientation generation 52 and the execute skill scope the default worktree to the unit of work: the task at task grain, the whole feature at feature grain, the whole epic at epic grain, and always the bug, chore, or tweak itself. The developer's or project's own instructions still win.
- **One worktree carries a coarse unit.** Riding task branches and the aggregate are checked out in turn inside it. A later task detaches it at its start base without a new join, every switch follows a commit, and assembly, checks, the draft PR, submission, and requested changes run there. The separate aggregate worktree is gone; another worktree holding the aggregate still means stop and coordinate.
- **Parallel tasks get their own (JC).** The guidance must not obstruct subagents. Tasks built at the same time within a unit each get a temporary worktree, added and joined the same way, and removed once committed, pushed when remote-backed, and merged into the aggregate. Two builders never share a worktree.

Orientation and execute skill tests pin the unit-of-work wording, the parallel-task rule, and the detach for a later task. Publication and managed-project refresh did not occur.

## Local 0.18.5 distribution

**October 12, authorized local distribution.** Commit `d6c96f14969e22c00bd53947bf9f5420f9a67311` stamps **0.18.5**, carrying the [concurrent generation read](#october-12-concurrent-generation-read) fix for issue #151. Schema **10**, orientation generation **51**, and eleven embedded skills are unchanged. v0.18.4 remains the latest published release. The stamp was committed to local `main`, not pushed; no tag or GitHub Release was created.

`check-doc-versions` could not pass the first stamp after a publication. The docs that name the latest published release, v0.18.4, also named the previous stamp, which the check forbids. Commit `8437e172` lets a previous stamp that has a `vX.Y.Z` release tag appear in that form; a bare mention still fails.

The darwin/arm64 executable was built with go1.27.2 from a clean detached checkout of the stamp commit, with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.5-d6c96f14/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `bd6ee4c55ad929e76e744c61d5610af07ec9c7a5257cabdb7f47f0c6b406a939`. The published 0.18.4 executable is retained at `distributions/0.18.4-85d0dd10`.

Checks:
- The doc-version check and its tests, the version-stamp check, the stamp-sensitive upgrade and serve tests, lint, the client-privacy check, and the site build passed.
- In the uncached full suite, every package except `internal/core/mutate` passed. That package hit Go's 10-minute default test timeout while the machine's load average was near 60 from unrelated work. Run alone, it passed in 647 seconds.
- On disposable projects, the installed binary refused re-spec and approval of a broken plan. With twelve concurrent `add question` writers, one landed and every refusal carried a reason.

Managed projects were not refreshed.

## October 12 concurrent generation read

**Source only.** [Issue #151](https://github.com/madeintandem/tiller/issues/151) came from one full-suite failure during the v0.18.4 release checks. A concurrent `tiller add` was refused with an empty reason: "generation changed while it was opened". Reads take `.tiller/.cache/generation` without the mutation lock. When another writer atomically replaced that file between the reader's lstat and open, the anchored walk's identity check reported the change under `ErrUnsafe`, the same class as symlink substitution. The refusal was right; its reason was not.

`rootpath` now marks a regular leaf replaced by another regular file as `ErrChanged`, which also still matches `ErrUnsafe` and keeps the same message. A symlink at the leaf is never `ErrChanged`. The lock-free generation read classifies `ErrChanged` as `mutation_stale` and says a retry is safe. The generation read under the lock is unchanged: there a replacement would not come from Tiller.

Deterministic tests reproduce the replace through the reader's open seam: a rename gives `mutation_stale`, and a symlink keeps the unclassified refusal. Lint and the uncached suite passed, and 40 focused runs of the concurrent-writer end-to-end test passed.

## v0.18.4 publication

**October 12, published release.** The annotated tag `v0.18.4` points at commit `85d0dd10c93498a969598587ce6bb4d8d29ba730`. The release workflow ran the suite and GoReleaser, which published [v0.18.4](https://github.com/madeintandem/tiller/releases/tag/v0.18.4) as the latest release. It has four archives (macOS and Linux, amd64 and arm64) and `checksums.txt`. The workflow also updated `Casks/tiller.rb` in the tap to 0.18.4. This is the first published release since v0.11.3. Its notes cover everything since that tag. The repository remains private, so anonymous installation is still unavailable.

Release checks before the tag:
- The release Quickstart completed end to end with the installed 0.18.4 binary. The embedded console served its status, tree, and page routes.
- govulncheck reported six reachable standard-library `net/http` and http2 vulnerabilities in go1.27.1. Commit `85d0dd10` moved `go.mod` to go1.27.2, which clears them.
- The same commit took the non-breaking npm audit fixes: `source-map-js` 1.2.2 and expressive-code 0.44.2. The console bundle stayed byte-identical; only the notice files changed.
- Ten moderate docs-site findings remain through Starlight's `postcss-selector-parser`. npm's only offered fix is a breaking Starlight downgrade. They affect the docs build, not the shipped binary.
- Hosted CI, including console freshness, passed on the tagged commit.
- One local full-suite run hit a rare concurrent-writer refusal in `TestConcurrentQuestionWritersRefuseInsteadOfLosingUpdates`. A writer that loses a race on the mutation generation file is reported with an empty reason instead of `mutation_stale`. It did not reproduce in 90 focused runs across both toolchains, and the next full run passed.

The local installation is the published darwin/arm64 archive. It was downloaded with `gh release download`, verified against `checksums.txt`, and confirmed to contain LICENSE, README.md, and THIRD_PARTY_NOTICES.txt. Its build metadata records go1.27.2, the tagged commit, and `vcs.modified=false`. The binary sits at `~/.local/share/tiller/distributions/0.18.4-85d0dd10/tiller` and at `~/.local/bin/tiller` (installed by remove then copy); both have SHA-256 `d0812178334ac509eb3b259341ff6dbab213ea5b939aed78f302d5d54d359b9f`. The earlier local build of the stamp remains at `distributions/0.18.4-0368e87c`. The release Quickstart completed again with the installed release binary. Managed projects were not refreshed.

## Local 0.18.4 distribution

**October 12, authorized local distribution.** Commit `0368e87c4814509cf14211d2170449e1109235bf` stamps **0.18.4**, carrying [provenance citation fixes](#october-12-provenance-citation-fixes) for issues #143 and #144 and [specced-feature re-validation](#october-12-specced-feature-re-validation) for issue #145. Schema **10**, orientation generation **51**, and eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.4-0368e87c/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `f87a26a97cb411df5a8c2b0cb95fb1da952719bccbf2093182f0d290f56ef3e5`. The stamped 0.18.3 executable is retained at `~/.local/share/tiller/previous/0.18.3-5025f2bb-tiller`; two unstamped interim builds of the fixes are retained under `distributions/0.18.3-0e88d760` and `distributions/0.18.3-39e2fc66`.

The uncached Go suite, lint, doc-version and client-privacy checks, and the site build passed on the stamp. One run of the uncached suite failed `TestStaleSpecHashesTheWorkingTreeThroughCheckinConversion` when a background repack changed the scratch repository's object counts; it passed on five reruns and is unrelated to this change. The installed binary reports 0.18.4 on PATH and, on disposable projects, refuses re-spec and approval of a broken specced plan and resolves comma-joined citation ranges. A scratch clone of Acme's clean tree re-specced all ten specced features without refusal or writes. Managed projects were not refreshed, and existing serve processes were not restarted.

## October 12 specced-feature re-validation

**Source only.** [Issue #145, specced features never re-validated](https://github.com/madeintandem/tiller/issues/145), came from an Acme specify batch whose agents edited specced features and read `spec feature`'s "already specced (no change)" as proof the edits were valid. One agent swapped a task's spec for a stub and got the same answer. Validation ran only on the draft to specced move, and the plan gate recorded the basis without validating, so a contract emptied after spec was approved and pinned while `readout specify` reported "Contracts valid: 0 of 1".

Re-running `spec feature` on a specced feature now runs the same stage-2 validation: a passing output keeps the no-change answer, and a failing one gets the first spec's refusals (exit 1 not written, exit 2 malformed) with the feature left specced. `approve feature` runs the same validation before any write. The blocked line for a specced feature says its specify output no longer passes rather than calling it unspecced. `--refresh-basis` on an approved feature is unchanged.

Lint and the full Go suite passed. On a disposable project the rebuilt binary refused re-spec and approval for an emptied contract (exit 2) and an emptied task spec (exit 1), then approved the restored plan.

## October 12 provenance citation fixes

**Source only.** Both issues came from one Acme intake run that staged an 18-operation amendment. [Issue #143, multi-range citations](https://github.com/madeintandem/tiller/issues/143): the agent cited several ranges per operation (`review.md#L105-L134, #L136-L157`), which provenance rejected, so 13 operations lost their link to the feedback document. A citation may now list several ranges of one document after commas, with or without a repeated `#`; it links to the document once, anchored at its first range, and still inherits the intake revision pin. Citations spanning several documents stay unsupported. Because nothing edits a staged operation, `tiller amend` now warns, and `show amendment` marks `source_unresolved`, when a value written as a citation (a `.md` path with a `#` fragment) will not resolve. Free text stays silent. The intake skill states the accepted forms.

[Issue #144, staged-key diagnostics](https://github.com/madeintandem/tiller/issues/144): the same amendment produced 13 `unresolved_amendment_endpoint` diagnostics because each new feature and question named a new epic or feature staged in the same batch. While an amendment is proposed and the staging operation has no application events, such an endpoint is now pending: still unresolved and never given a fabricated identity, but not diagnosed. An approved amendment, or an interrupted application, missing its creation event is still diagnosed.

On a disposable project the installed 0.18.3 binary reported 2 endpoint and 3 reference diagnostics for the same staged batch; the rebuilt binary reported only the one deliberately malformed citation. Lint and the full Go suite passed.

## Local 0.18.3 distribution

**October 11, authorized local distribution.** Commit `5025f2bbff9c12be845ff2886cf975560bf4a5c7` stamps **0.18.3**, carrying [linked prose references](#october-11-linked-prose-references) from merged [PR #142, safe prose reference links](https://github.com/madeintandem/tiller/pull/142). Schema **10**, orientation generation **51**, and eleven embedded skills are unchanged from the merged source. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.3-5025f2bb/tiller` and the installation at `~/.local/bin/tiller` share SHA-256 `5085f8dcdcd01814984c263fff8fe32d80dcb0ce7f52d15656dc9ef3164fbaf7`. The outgoing 0.18.2 executable is retained at `~/.local/share/tiller/previous/0.18.2-756881c-tiller`.

The full uncached Go suite, lint, stamp-sensitive upgrade and serve tests, stamp/doc-version checks, privacy scan, and site build passed. The installed binary reports 0.18.3 on PATH. Chromium exercised its embedded console against a disposable project: the topbar reports v0.18.3, HTTPS prose links have protected new-tab properties, all three work-ID links remain, and no prose image element mounts. The preceding PR's full console suite and hosted checks passed before merge. Managed projects were not refreshed, and existing serve processes were not restarted.

## October 11 linked prose references

**Source only.** [Issue #95, external references in prose](https://github.com/madeintandem/tiller/issues/95), now enables absolute HTTPS Markdown links in the shared console renderer, opening a new tab with `noopener noreferrer`, an outbound indicator, and an accessible new-tab cue. Other external schemes and arbitrary relative links stay inert. Markdown images now render only their alt text; baseline browser inspection confirmed that remote images previously loaded automatically. Raw HTML remains escaped. Relative Markdown-to-file resolution and Mermaid rendering are not introduced.

[Issue #120, clickable work references](https://github.com/madeintandem/tiller/issues/120), was already implemented for standalone immutable epic, feature, and task IDs, including formatted text and single-ID code spans. The combined change preserves that mechanism and its boundary tests instead of adding another parser. Commands, fenced code, paths, and authored links remain unchanged.

Orientation generation 51 points to one shared external-reference writing rule in `tiller guide orient`; decompose, specify, capture, intake, and amend inherit it. First mentions link to the canonical home or relevant page only when the HTTPS URL came from inputs or a live fetch returned a page identifying the reference. Failed fetches, search snippets, and guessed homepages do not qualify. Missing verification leaves the plain name. Adoption remains a separate binary installation followed by `tiller upgrade --project-only` and reviewed guidance changes in each intended checkout.

The console's 1,360 tests, embedded asset build, full uncached Go suite, and lint passed. Chromium exercised the rebuilt binary against a disposable project: epic, feature, and task reference navigation; the same links in the file reader; a protected external tab; inert unsafe and relative destinations; retained image alt text with no automatic external requests; and light/dark rendering without browser runtime errors. Disposable project maintenance refreshed generation 50 to 51 and the served orient guide carried the strict rule. Independent console and guidance reviews found no material defects. Nothing was installed, published as a release, or applied to a managed project.

## October 11 relationship parsing reuse

**Source only.** [Issue #124, repeated relationship corpus parsing](https://github.com/madeintandem/tiller/issues/124), remained open after the October 7 read optimizations because those reduced filesystem verification, history parsing, tree loads, and Git calls without reusing Markdown parsing. Profiling a disposable CLI-created project with six epics, 36 features, 288 tasks, 36 contracts, 331 creation events, and 415 unique Markdown documents totaling 2.67 MB identified Markdown mention parsing as the main avoidable cost. The isolated document parsing and projection phase allocated 44.8 MB per read before reuse.

Relationship reads now reuse content-digest parsing facts, not filesystem bytes or a graph. Every request still performs the confined corpus walk and current-file checks before SHA-256 lookup. Mention results contain only IDs and source lines; contracts and parse failures remain independent of paths and current ownership, with mutable contract storage copied for each request. Paths, ownership, resolution, validation, diagnostics, history, and ordering are projected anew. Each cache uses least-recently-used eviction with 2,048 entries and an 8 MiB conservatively accounted retained-data budget; oversized inputs or outputs bypass retention. Parsing stays outside cache locks, so concurrent cold misses can duplicate work. There is no persistent index, watcher, new dependency, schema change, or orientation refresh.

Sequential runs of the before and after binaries produced these warm-request medians across 30 HTTP requests per surface. Allocated bytes are means from 20 in-process handler-profile repetitions, not retained memory:

| Surface | Before, ms | After, ms | Before allocations, MB | After allocations, MB |
| --- | ---: | ---: | ---: | ---: |
| Node | 144.2 | 98.0 | 69.0 | 31.6 |
| Map | 142.2 | 96.7 | 93.3 | 56.1 |
| API query | 136.3 | 89.7 | 68.3 | 30.9 |
| CLI query, 10 fresh processes | 113.1 | 112.5 | Not measured | Not measured |

Node, map, API query, and CLI query responses were byte-identical to the baseline. Separate CLI processes do not share parsing results, so their latency is unchanged in practice. Confined corpus I/O, hashing, and index projection remain per-request costs. The fixture covers substantial prose and contracts but not active task worktrees or amendment lineage; the measured gain is not a claim about every managed project.

Lint, the full uncached Go suite, and race checks on relationships, readinput, and serve passed. Real CLI/API reads preserved next-request same-size/same-mtime edits, branch switches, permission refusal and restoration, deletion, symlink and nonregular replacements, and safe-path restoration; 24 concurrent query reads returned the current facts. Strict API/CLI queries refused unsafe corpus input while node/map enrichment remained explicitly incomplete. Chromium rendered node backlinks and the relationship map with no browser runtime errors. Independent cache-safety and coverage reviews found no material defects. The fix is committed without a new version stamp. Nothing was installed, published, or applied to a managed project.

## Local 0.18.2 distribution

**October 11, authorized local distribution.** Commit `756881cedafbd7e9713f91e348546af16ffa2639` stamps **0.18.2**, carrying [static board heads](#october-11-board-heads). Schema **10**, orientation generation **50**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.2-756881c/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `19a14d7fe03fe446f13d3a51b302dd14162a2353b15e9e7073e94961eed13a4d`. The outgoing 0.18.1 executable is retained at `~/.local/share/tiller/previous/0.18.1-c1f85b015e27/tiller`.

Lint, the uncached Go suite, the stamp-sensitive upgrade and serve tests, the doc-version check, and the site build passed on the stamp; the console suite passed on the feature commit. The installed binary reports 0.18.2 on PATH. Served read-only over Acme and driven in WebKit, Build and Fix each render six headings in the static row over six body columns, with no sticky element on either page. Confirmation in Safari remains with JC. Managed projects still carry generation 47 until refreshed.

## October 11 board heads

JC still saw scrolled cards in a strip above the column headings on Build and Fix in Safari on 0.18.1, after the 0.17.7 overpaint meant to hide it. In WebKit, headless and headed, with programmatic and wheel scrolling and at device scales from 1 to 3, layout placed every sticky heading exactly at the scroller's top edge. His full-resolution screenshot showed the heading painted about six pixels below that position and the clip about two pixels above it: Safari's asynchronous scrolling composited the sticky layer away from its layout position, which no offset or overpaint can reliably cover.

The boards now render a static headings row over a scrolling body row, shared through `BoardGrid`. Both rows sit in one horizontal scroller with the same column template, so headings pan with their columns, and only the body scrolls vertically: as one in Board mode, per column in Columns mode. Neither board uses `position: sticky`, and a stylesheet test pins that. Board mode keeps every column as tall as the tallest. The trade is that Board mode's vertical scrollbar sits at the body's right edge, off screen until the reader pans right on a board wider than the window. Safari was not available to automation, so the fix was verified in WebKit and awaits confirmation in Safari.

## Local 0.18.1 distribution

**October 11, authorized local distribution.** Commit `5186c562ce134d2a4f196fbb4e350dd624277231` stamps **0.18.1**, carrying [grouped search results](#october-11-search-results). Schema **10**, orientation generation **50**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.1-5186c56/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `c1f85b015e274f4ce2107bb9418439ece5a5b3df0f733185036a269c30a0077f`. The outgoing 0.18.0 executable is retained at `~/.local/share/tiller/previous/0.18.0-46d282bf2000/tiller`.

Lint, the uncached Go suite, the stamp-sensitive upgrade and serve tests, the doc-version check, and the site build passed on the stamp; the console suite and a disposable-project verify run passed on the feature commit. The installed binary reports 0.18.1 on PATH. Read-only against Acme, it names `B-aki9u3`, `T-4fzt82`, and `E-mhutpf` first as id matches, answers `payment` with 167 results across eight kinds, and its console serves the rebuilt bundle and the same id lookup over `/api/search`. Managed projects still carry generation 47 until refreshed.

## October 11 search results

JC reported that Explore search did not put a node first when he searched its id. On Acme every id returned zero results: search matched titles and prose lines only, never ids, keys, or references. Results were a flat list of up to 200 matching lines, so one node could fill 18 rows and a broad query (`payment`, 1,187 lines) read as noise. Bugs, chores, and tweaks were searched; the suspicion that they were not came from the id misses.

Search now groups matches: one result per node, carrying its total matching lines and the first three, and one per Markdown file no node owns. A query equal to a node's id, id suffix, full reference, whole key, or last key segment is a lookup and ranks first. Below that, title matches (equal, then prefix, then contains), then key fragments, then nodes matched only in prose or ledger fields, ordered by match count. A node's title is its label and is never repeated as a hit. An amendment's `<A-id>-<key>.md` is attributed to the amendment. The payload became `{query, kind?, count, truncated, kinds, nodes, documents}`, with `count` counting results rather than lines, and both surfaces take a kind filter. The page budget is shared between nodes and documents, as the old budget was shared between classes of hit.

The console shows an Exact match section with the node's kind, then one section per kind capped at five rows, with a tab per kind that asks the API for that kind alone so a kind is never cut short by the others' share. All-tab rows show one matching line with the total on the head line; kind tabs show three. On Acme, `payment` answers 167 results across eight kinds in about 0.2 seconds.

Not built: review records under `.tiller/reviews/` that name a task id are still unowned documents, and the orient skill's search sentence is unchanged to avoid a generation bump.

## Local 0.18.0 distribution

**October 11, authorized local distribution.** Commit `2f6f91c199f12613363860d44f45bf630e014f7e` stamps **0.18.0**, carrying [eval corrections](#october-11-eval-corrections) for issue #141. Orientation generation moves to **50**; schema **10** and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.18.0-2f6f91c/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `46d282bf20008f375289a14a34e002ef7643ca8f0bffa671af1bd22f185b6048`. The outgoing 0.17.9 executable is retained at `~/.local/share/tiller/previous/0.17.9-c63320999381/tiller`.

Lint, the uncached Go suite, the stamp-sensitive upgrade and serve tests, the doc-version check, and the site build passed on the stamp. The candidate drove the issue #141 arc in a disposable project: a done feature with a two-file `ref` reported one `orphan_eval` for the deleted file, the eval edit raised `stale_spec`, and an approved `correct-eval` cleared it; an unnamed criterion removal refused at exit 2, an edit after staging refused as stale at exit 1, `--retire C2` applied, and a statement change refused. The installed binary reports 0.18.0 on PATH and writes orientation generation 50. Managed projects still carry generation 47 until refreshed.

## October 11 eval corrections

[Issue #141](https://github.com/madeintandem/tiller/issues/141) reported three problems from mending drift on done features. A test binding could name only one file, so a contract that needed two wrote them as one joined string, which drift could not read (`ref_unreadable`). A done feature's eval bindings could not be corrected at all without leaving `stale_spec` on the eval leg permanently, because nothing re-pinned `verified_against.eval` and the mend-drift skill said so. And an `attest` event recorded the feature as `active` after a later sync had moved it to `done`.

### Decisions

- **A test binding's `ref` is a scalar or a YAML list (JC).** `ref: [spec/a_spec.rb, spec/b_spec.rb]` names several files. One ref still writes back as a scalar, so existing contracts are byte-stable. Drift checks each path-shaped item on its own, so every missing file is named separately. A whitespace- or semicolon-joined string stays `ref_unreadable` with advice to use a list: Tiller never guesses a split. A list on an `eval` (rubric) binding refuses, since a rubric names one file, and a baseline artifact names one file.
- **`correct-eval` is an amendment operation (JC).** The reviewed correction rides the existing reshape surface (`tiller amend <A> --correct-eval <feature> --reason "..."`, applied by the human's `tiller approve amendment`) rather than a new top-level verb or a flag on `approve`. It targets a done feature only, pins `eval_revision` at staging, and on approval reads the verified contract back from Git by its pinned blob and refuses anything but a bindings-and-baseline-artifact change. It then re-pins `verified_against.eval` and emits a `correct-eval.feature` event with the old and new blobs, the reason, and the retired ids. Status, accepted bases, and gate results do not move. Spec-leg and surface-leg `stale_spec` stay as they were.
- **Deleting a criterion is deliberate and recorded on the amendment (JC).** A criterion may leave the contract only when the operation names it with `--retire`, and `retire` may name only a criterion that left. The retired ids and the reason live on the staged operation and the event; there is no `retired:` field in the eval schema. Adding a criterion, or changing a statement, polarity, required flag, or baseline surface, is changed acceptance and stays forward work.
- **Problem 3 is working as designed (JC).** An event records the status its own checkout saw when it was written, and the node files are the source of truth for current state (artifact-schema.md, Event stream). An attest event reading `active` after a sync moved the node to `done` is the stream doing its job. No code changed; the Event stream paragraph now says so.
- **Guidance.** Orientation generation 50 sends done-feature eval corrections through `amend --correct-eval`. The mend-drift skill no longer says no command re-pins the eval leg: it stages the correction for an eval-leg `stale_spec` or an `orphan_eval` on a done feature and keeps the persistence warning for the spec and surface legs. The amend skill gains the operation and its refusals, and the console shows a correct-eval operation with its reason and retired ids.

### Verification

Store, gate, drift, relationship, view, staging, and approval tests cover the list form and each correction refusal, including a second correction reading its base from the first correction's re-pinned blob and a contract edited again after staging refusing as a stale precondition. Orientation, skill, console, and page-view tests pin the new text, generation 50, and the retire list.

## Local 0.17.9 distribution

**October 10, authorized local distribution.** Commit `bdacb5b0c25331014fa624ccaf978f689972116a` stamps **0.17.9**, carrying [worktree guidance](#october-10-worktree-guidance). Orientation generation moves to **49**; schema **10** and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.9-bdacb5b/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `c63320999381ee94077cf7af82814f7c6b070f220cfb9e4576bbf9d7db1ef84d`. The outgoing 0.17.8 executable is retained at `~/.local/share/tiller/previous/0.17.8-da55fc47920b/tiller`.

Lint, the uncached Go suite, the doc-version check, and the site build passed on the stamp, and the verification arc on a binary built from it ran a task through a detached, joined worktree to epic `done`. The installed binary reports 0.17.9 on PATH; in a disposable project it wrote orientation generation 49, refused a mutation in an unjoined worktree with the per-checkout explanation, accepted it after join, and rendered the execute skill's worktree section. Managed projects still carry generation 47 until refreshed; joining a new worktree there refreshes `.tiller/orientation.md`, which the guidance restores rather than commits.

## October 10 worktree guidance

Confirming the moved-surface repair in the Scratch project, the agent worked in a detached worktree and every `link` and `unlink` refused with `contributor_unset` until it set `TILLER_CONTRIBUTOR`: `.tiller/identity.yml` is per checkout and gitignored, so a new worktree has none, and the refusal pointed at `join` without saying why a joined repository had no identity here.

### Decisions

- **Worktree by default, instructions first (JC).** Orientation generation 49 tells agents to execute each started task, bug, chore, or tweak in its own Git worktree, leaving the primary checkout as the developer left it. The developer's or project's own instructions about checkouts and worktrees take precedence over that default.
- **Join inside, then claim and start.** The worktree is added detached at the updated start base and joined before the claim, so claim and start state land in it and follow onto the recorded branch, as they did with `git checkout -b` in one checkout. Join also refreshes generated guidance when the project's committed generation is older than the binary, so agents restore any tracked file it changed rather than committing the refresh into every task branch, and name the needed `tiller upgrade --project-only`. Re-entry uses, or adds and joins, the worktree holding the recorded branch, and `resume` runs inside it so its state lands on that branch.
- **Shared branches stay where they are.** Git checks out a branch in only one worktree. Aggregate assembly adds a worktree for the aggregate unless another worktree holds it, which means stop and coordinate. Integration-branch closeout runs where that branch is checked out, usually the primary checkout, only when it holds no changes the run did not make.
- **Other agents share the repository (JC).** Agents assume other agents and developers may be working in their own worktrees. A worktree, branch, or uncommitted change a run did not create is someone else's: never removed, reset, cleaned, stashed, committed into, checked out over, or force-checked-out. Another agent's claim lands on its task branch, not the integration branch Tiller reads, so before claiming agents look for the work's canonical branch in worktrees, branches, and the remote, and stop for the human when one exists or Git surfaces a conflicting claim or start. Agents remove only worktrees they created, after the work is committed and, when remote-backed, pushed.
- **The refusal says why.** `contributor_unset` now explains that identity is per checkout and that a new clone or worktree needs `tiller join --as <you>` there. The reason code and exit code are unchanged.

### Verification

Orientation, execute skill, and refusal tests pin the new text and, for both task and shelf entry, the worktree-and-join-before-claim order. An independent review found join's guidance refresh riding into task branches, re-entry worktrees that never joined, resume running outside the worktree, closeout contradicting the hands-off rule, and an overstated claim-visibility promise; all were corrected before commit. A scripted run on the installed binary walked a task through a detached worktree: an unjoined worktree refused claim with exit 3, then join, claim, start, `git checkout -b`, check, and submit all succeeded. `make lint`, `make test`, the docs version check, and the site build passed. Nothing was installed, published, or applied to a managed project; joined checkouts refresh to generation 49 on their next `join` or `migrate`.

## Local 0.17.8 distribution

**October 10, authorized local distribution.** Commit `5cc851ececfe1f01b2d3e3116829ba073601832b` stamps **0.17.8**, carrying [moved surfaces](#october-10-moved-surfaces) (`unlink --surface`). Schema **10**, orientation generation **48**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.8-5cc851e/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `da55fc47920b7cbfa892603411ac4fa9249269217e1dc651ffe1db6cf17a8589`. The outgoing 0.17.7 executable is retained at `~/.local/share/tiller/previous/0.17.7-0ca5be2f0baf/tiller`.

Lint, the uncached Go suite, the doc-version check, and the site build passed on the stamp, and the verification arc on a binary built from it ran a migrate feature that moves its surface from the named sync remedy through link and unlink to epic `done`. The installed binary reports 0.17.8 on PATH; in a disposable project it wrote orientation generation 48 and eleven harness shims, and unlinked a deleted surface path. Managed projects still carry generation 47 until refreshed; Scratch's stranded `shared-core/core-split` needs only the link, the unlinks, and a sync.

## Local 0.17.7 distribution

**October 10, authorized local distribution.** Commit `52dedcf8dc48f715b75c784f9f9c409e2f4247e2` stamps **0.17.7**, carrying the [console board polish](#october-10-console-board-polish). Schema **10**, orientation generation **48**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.7-52dedcf/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `0ca5be2f0baf78b5daa4d2fb974056c231f56fbbed8dac886fa6d2a36b1d5a1e`. The outgoing 0.17.6 executable is retained at `~/.local/share/tiller/previous/0.17.6-0c0a21703559/tiller`.

Lint, the uncached Go suite, the doc-version check, and the site build passed on the stamp, and the scratch-project verification arc ran task through epic to `done` on a binary built from it. The installed binary reports 0.17.7 on PATH; in a disposable project it wrote orientation generation 48 and eleven harness shims, and its console restored Fix's chore filter on return from Home with the Scroll and theme controls both at 10px. No managed project needs a refresh for this stamp beyond the generation 48 refresh 0.17.6 already called for.

## Local 0.17.6 distribution

**October 10, authorized local distribution.** Commit `33be12fa94cb259396901085d9495e008ba9f319` stamps **0.17.6**, carrying the [October 9 bug-hunt remediation](#october-9-bug-hunt-remediation) and its follow-ups, [readout continuity](#october-9-readout-continuity), and [grounded specification](#october-10-grounded-specification). Schema **10** and the eleven embedded skills are unchanged; orientation generation advances from **47** to **48**. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.6-33be12f/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `0c0a21703559f0d2d6f118a5f06e39055a71ede66be998a3753180968937a534`. The outgoing 0.17.5 executable is retained at `~/.local/share/tiller/previous/0.17.5-e27aac640b91/tiller`.

Lint, the uncached Go suite, and the doc-version check passed on the stamp, and the scratch-project verification arc ran task through epic to `done` on a binary built from it. The installed binary reports 0.17.6 on PATH. In a disposable greenfield project, `init` and `join` wrote orientation generation 48 and eleven harness shims; after a task-grain merge in the later-ordered of two epics, `readout execute` named the closed task's ready sibling rather than the earlier epic's task. Managed projects still carry generation 47 until `tiller upgrade --project-only` or a re-join refreshes them; none was refreshed.

## October 10 moved surfaces

A migrate feature in the Scratch project, `shared-core/core-split`, `git mv`d the code its surface recorded into a new module. Every task was done and acceptance passed, yet each `tiller sync` skipped the rollup: the completion record fingerprints every recorded surface path at the integration commit, a moved path resolves to nothing, and no command could drop a surface path, since `link --surface` only unions and an amendment re-pins prose alone.

### Decisions

- **Unlink, not record-absent (JC).** `tiller unlink <epic|feature> <node> --surface <path>` removes recorded paths, matched literally after cleaning and without an existence check, so moved or deleted code can still be named. Recording a missing path as absent at the rollup was rejected: it needs no command, but leaves drift watching nothing at the code's new home and needs a rule to keep an intended absence out of `missing_surface`. Linking the new location and unlinking the old keeps the surface true.
- **Bounded by the completion record.** A done node or archived epic refuses (`surface_locked`), since its record already pins the surface. A specified, live replace or migrate feature cannot drop its last path (`surface_required`), the rule `spec feature` already enforces, which forces link-then-unlink. The arm is agent-runnable like `link --surface`, combines atomically with `--depends-on`, and emits `unlink.surface.<grain>`.
- **The skip names its repair.** Sync's skipped-node report marks a missing recorded surface path (`surface: true` in JSON) and prints the exact unlink; execute and mend-drift say to link the current location and unlink the old path with the work.

### Verification

CLI and core regressions cover literal removal of a vanished path, no-op repeats, misuse refusals, the done and archived locks, the last-path rule, the atomic dependency arm, epic grain, the JSON shape, and the reported strand end to end: a migrate feature that moves its surface is skipped with the remedy named, then crosses to done after link and unlink. An independent review found the correction readout misreading removed paths as dependency edges and the JSON dropping an emptied surface; both were fixed and pinned. `make lint`, `make test`, the UI suite, the site build, and the verification arc passed, and a built binary ran the Scratch scenario through to epic `done`.

## October 10 console board polish

Three console fixes JC reported on Build and Fix.

### Decisions

- **Headings overpaint, not re-anchor.** Scrolled cards showed in a thin strip above the sticky column headings. The board's top edge sits on a fractional pixel (the topbar is 60.25px tall), so the scrollport and a sticky heading can round to different device pixels. Each column's first child paints its own background 4px above itself; at rest that paint falls outside the scroller and is clipped, so nothing changes until cards scroll under it. Headless WebKit never rendered the strip, so the fix rests on the measured fractional offset and a check that it is invisible at rest in both scroll modes and both themes.
- **One segmented-control size.** The Board/Columns control drops its 11px override and uses the shared 10px segmented-control rule the Auto/Light/Dark selector uses; it is now 1px taller than the panel toggle beside it.
- **Fix filters are remembered per project.** Storage is browser-local, keyed `tiller-fix-filters:<project key>`, and holds the normalized filter query. Unlike Build's `?epics=`, Fix's All board is the bare URL, so a bare `/fix` is ambiguous: filter controls navigate with router state marking the choice, only those navigations and URLs naming a filter are stored, and any other bare arrival, including the topbar link while Fix is showing, restores. The router lands the restored URL as a transition, so the board renders from the stored filters until it does, and the All board never paints first. An arrival whose project status cannot be read keeps All.

### Verification

Filter-memory and stylesheet regressions cover restore, URL precedence, remembered All, the topbar re-click a review found, status failure, and project isolation; the UI suite passed twenty consecutive runs after a timing-sensitive assertion was corrected. `make lint`, `make test`, and the scratch-project verification arc passed, and WebKit confirmed the restore flows across navigation, reload, and return to All.

## October 10 grounded specification

[Issue #140](https://github.com/madeintandem/tiller/issues/140): one feature's three tasks needed five post-approval `task.md` corrections, each a human `approve task --refresh-basis`, because `specify` wrote concrete claims about third-party plugin code (dialect, key pattern, entry point, a variable's role, a value's type) from a quick read. Tiller enforced the basis correctly; the gap was guidance.

### Decisions

- **Ground load-bearing claims.** `specify` gains a "Grounding claims about code" section generalizing the #117 runner rule: read the code first and cite file and line, plus the version, tag, or commit for code outside the repository. Citations cover only claims the plan rests on.
- **Open facts, not assumptions.** A fact only building can settle is written as a question for the task to determine, with candidates and how to tell them apart. A provisional answer marked as assumed was rejected: finding it wrong still changes accepted prose and still costs a re-pin.
- **Check before handoff.** Specify re-reads each citation against its source before `spec feature`, through an independent reviewer when the harness provides one, and lists open facts in its run notes so the human weighs them at approval. The wording stays harness-neutral.
- **Departure, not re-pin (JC).** A factual claim in an approved `task.md` that the code proves wrong is recorded as a departure in the PR body and run notes; the accepted prose stays unedited. The departure is bounded: a fact that changes feature behavior, an acceptance criterion or binding, task scope, or a dependency routes through the specify correction table. The deliberate re-pin cost stays for genuine plan changes. This narrows #140's out-of-scope line about a cheaper re-pin path: a bounded factual departure is no re-pin at all. The departure travels in the task's implementation commit message, so feature and epic reviews, offline review surfaces, and later sessions can gather it; code-review judges the diff against it, and amend's in-place prose correction excludes the task being executed. Orientation is unchanged because it defers to the skills' correction maps.
- **No spike rule.** Specify reads unfamiliar outside code itself; a separate investigation task needs a reviewable output and a reason reading cannot settle the approach.

### Verification

Two skill regressions pinning the new specify and execute text failed before the change; an independent review then added the durable commit record, the code-review and amend carve-outs, and the planned-test bound, each pinned by a test. `make lint`, the full uncached `make test`, and the site build passed afterward. Skill bodies are served by the running binary, so joined checkouts need no refresh once the binary is updated; nothing was installed or published.

## October 9 readout continuity

[Issue #139](https://github.com/madeintandem/tiller/issues/139): after a task-grain closeout, `tiller readout execute` named a ready task in an earlier-ordered epic while the closed task's feature still had an eligible, unheld sibling. The unclaimed pool ranked by global order alone, so nothing in the walk knew where the reader had been working.

### Decisions

- **Owner affinity, no flag.** JC chose inference over `--focus` or an extra "In this feature" line. Task owners survive completion, so the tree already records which features and epics the reader worked in, with no event-stream reads. The route ranks unclaimed tasks in those features first, then the rest of those features' epics, then global order; sibling preference and historical ties still order each tier.
- **Route only.** JC kept the shared pool, streams, and console Up next reader-independent; the specs record that divergence.
- **Continuity note.** When claimable work leaves a ready or active feature the reader worked in that still holds back an unclaimed unfinished task, the route names the first such feature in flow order and that task with the unclaimed-work diagnostic's reason, without repeating a task the Order note names. Paused features, retired ancestors, and features whose remaining work is claimed or in review stay quiet, so the note never repeats about work nobody can take up. The reason helper is now shared with that diagnostic.
- **Order note wording.** One blocker `holds`, several `hold`; a task held by its own question names no shared target.
- **Generation 48.** The orientation rules and the orient skill describe the ranking and the note.

**Overturned October 21.** [Ranked next](#october-21-ranked-next) overturns "Owner affinity, no flag" and "Route only": ranking now reads the reader's own recent events, the Continuity and Order notes retire with the route walk, and Home's Up next becomes the reader's ranked list in the console build. See the [ranked next design](plans/2026-10-21-ranked-next-design.md).

### Verification

The three original regressions in `internal/views/route_continuity_test.go` failed before the change; an independent review then added coverage for the note's quiet cases, a non-question reason, an empty reader, and plural and shared-target hold wording. `make lint` and the full uncached `make test` passed afterward. Nothing was installed, published, or applied to a managed project; joined checkouts refresh to generation 48 on their next `join` or `migrate`.

## October 9 bug-hunt remediation

**October 9, source only.** A whole-repository review reproduced 26 defects (six high, eighteen medium, two minor) while the Go suite, the console tests, lint and the site build all passed. All 26 are fixed on branch `bug-hunt-remediation`, from base `ca24521a` through `7979dfb8`, in seven reviewed batches plus one final-review fix wave. Schema **10**, orientation generation **47**, and the eleven embedded skills are unchanged. Nothing was installed, tagged, published, pushed, or applied to a managed project.

### Decisions

Every fix began as a regression that failed on the old behavior, at the boundary that owns the defect: the CLI resolves inputs, core mutations plan one guarded state and event batch, storage owns parsing and rendering, Go owns queue and acceptance facts, and React renders those facts. No persisted field changed, and no compatibility alias keeps the old behavior.

Submission reads the latest applicable required results, not the `check` status alone, so a failed recheck blocks `submit` for tasks and the three shelves while the failed evidence stays visible. `sync` stays advisory: it never rejects a real human merge. A changed feature eval contract expires the verdicts of unfinished child tasks that came from it, and a spec-only edit does not. Attestation history stays intact; the old verdict just stops counting.

Sync resolves the integration branch once to a full commit and uses that commit for merge observation and every completion fingerprint, so a run from a scratch checkout records integration's revision and a ref that moves mid-run cannot mix two. The integration name stays the identity that guards and recovery compare. Combined `link --depends-on --attach` and `unlink --depends-on --detach` remain supported, validated whole and committed as one batch. Literal filesystem coordinates are separate from URL and citation syntax, so punctuation such as `#`, `?` and `:` in a filename is a valid document identity that is encoded once at the transport edge. Console eval readiness uses the existing Go acceptance contract, with no second policy in React. Verification stayed disposable and local.

Rulings made while building:

- Contract expiry applies to every child task that is not done or cancelled, so `in_review` tasks are included. A merged task whose contract changed crosses with an advisory evals policy violation instead of a block.
- The eager `integration_ref_missing` refusal stays eager and is documented as a release note, rather than being made lazy.
- At a feature crossing, sync reads the integration commit's eval contract while acceptance stays pinned to what the working tree recorded. When the two differ, sync does not refuse. It warns with `eval_contract_mismatch`, and no new field is persisted. Drift keeps reading the working tree, because its job is to catch uncommitted edits.
- Nested-project surfaces still always skip at sync. That mismatch predates this work and needs one coordinate decision across sync, drift and the head snapshot, so it was left out.
- Defect 23 covers review-gate attestations only, so `attest_criterion` bursts still fold.
- Inventories are now NUL-delimited exact names. Old records are not rewritten, because the plan forbids a blanket historical rewrite.

### What changed

**Gates and acceptance.** `check` and `submit` consult current required results for task, bug, chore and tweak, and refuse `gates_not_green`; the remedy names a runnable check command. `attest` consults the manifest's declared gate kind and refuses to overwrite a command-backed gate with an attestation (`command_gate`), while declared attestation gates and criterion attestation work as before. `check` now carries a settled required attestation forward instead of resetting it to pending, unless the gate was rescoped, removed, or changed kind. Repeated criterion IDs are an `eval_contract_invalid` contract error before anything runs or records. Feature basis refresh, including feature revise inside an amendment, expires applicable child verdicts in the same transaction, reading the amendment's staged edits.

**Git evidence.** Dirty-tree fingerprints read NUL-delimited inventories without trimming names, hash from the repository root, and treat a rename as a deletion plus an addition. An unreadable path is `worktree_unreadable` (exit 3), never the deletion sentinel. Nested projects now fingerprint with the same coordinates for inventory and hashing. Sync records the integration commit's blobs, trees and inventories, reports a path the integration commit lacks as a skip, and never falls back to HEAD.

**Admission and storage.** Contributor add and remove compute the roster's extent from YAML structure, preserve unrelated bytes, and parse the result before committing. A marker that cannot be inspected is `project_marker_unreadable` and no longer redirects a mutation to the parent project. The server refuses cited hidden Markdown before serving or advertising it. `init --design` and intake refuse a source directory that overlaps the destination, but a single file from inside the destination copies normally. An `init` rerun over a newer orientation refuses `orientation_newer` before writing derived files. A null question entry is a normal load error that names the ledger and position. `install.sh` refuses a directory destination, runs the candidate's `--version` before moving it, and has no version-string fallback; a new local harness, `scripts/install_test.sh`, runs in CI.

**Structural mutations and reads.** `link` and `unlink` plan dependency and attachment effects as one batch and report the actual resulting fields; a blank `link --attach` refuses `attach_target`. The readout counts `merge_feature` and `merge_epic` as attention. Recap resolves bugs, chores, tweaks and amendments. Search matches a question's persisted short title.

**Console.** Eval readiness counts required criteria only and defers to the recorded gate status when there is one. Review attestations keep every runnable command above the compression threshold. Node navigation rereads the attention queue for the node being opened. Expanded Build tasks show their status as text. Explore's Filter, Search and Map modes name the tab Explore.

### Verification

Each defect has a regression that failed before its fix and passes after it. Each batch was driven at its real surface with a source-built binary on disposable fixtures: Chromium for the console, real Git repositories for provenance and nested projects, and `scripts/install_test.sh` against local checksum-valid archives, whose 9 checks fail on the old `install.sh`.

The final gate ran `make lint` and the uncached `make test` clean at `7979dfb8`, plus the console suite (78 files, 1,317 tests, up from 1,288), `make ui`, the site build, and race checks on serve, readinput, relationships, vcs, views and search. The final-fix real-surface runs covered an unborn-repository sync, an intake from inside the feedback folder, and a feature crossing that printed the eval-contract warning.

### Adoption and release limitations

The new guards protect future operations and prove nothing about old records. Release notes and any managed-project adoption need to say the following.

- Repair an already-corrupted contributor manifest from its known-good roster before normal commands can load it. Missing membership is never guessed.
- Correct duplicate criterion IDs, review the corrected contract, refresh its approved basis, and rerun the applicable checks.
- Recheck unfinished work whose latest required check failed, whose command-gate result was replaced by an attestation, or whose dirty-tree evidence used the affected filename or root handling. Record real new command evidence; do not manufacture passes.
- Unfinished tasks whose feature basis was already refreshed under the old code need fresh human judgment of the current contract: new criterion attestations for manual criteria, newly recorded rubric evidence then attestation for rubric criteria, then a mechanical recheck. A recheck or another unchanged basis refresh does not repair old acceptance.
- Inspect parent-project mutations that landed in the wrong project, and correct them only through supported commands after human review.
- Suspect completion evidence needs a separately reviewed forward repair design. A fixed sync does not reopen merge-backed done tasks or overwrite historical crossings. Old completion inventories that hold Git-quoted names are not rewritten and may read as vanished files in drift.
- Remove partial recursive design imports or misplaced installer output only after identifying the affected paths and getting any needed approval.

New or newly visible refusals and behavior changes:

- `integration_ref_missing` (exit 1) is eager. Sync resolves integration before looking for work, so an unborn repository or a checkout with no local integration branch refuses even with nothing in review, where earlier releases reported a clean no-op. Fetch or create the branch, or pass `--into`.
- `worktree_unreadable` (exit 3) comes from fingerprinting. `project_marker_unreadable` now applies to ordinary discovery, not only maintenance commands.
- `command_gate` refuses attesting a command-backed gate. `gates_not_green` now also refuses task and shelf submit. `eval_contract_invalid` also covers repeated criterion IDs, and a blank `--attach` on `link` is `attach_target`.
- `eval_contract_mismatch` is a new advisory warning in sync output. Sync never refuses on it.
- Prose never committed to the integration branch now skips rollup at sync.
- Contributor rewrite drops comments inside the roster. A comment directly above the next top-level key goes with it, and an inline comment on the `contributors:` line is lost.
- `recover_review` refuses historical contracts with repeated IDs (`historical_policy_invalid`).
- Contract expiry also covers `in_review` tasks, so a changed contract can send a submitted task back for acceptance.

### Follow-ups resolved

Two follow-ups were resolved afterwards, source only. Surface paths are one coordinate system: they were always stored relative to the Tiller project root, and every committed-tree read (`BlobHash`, `TreeHash`, their commit-pinned forms, the drift sweep's HEAD snapshot) now resolves paths from the project directory, so a nested project records its file and directory surfaces at sync and drift re-reads them without a false skip, unreadable row, or repository-root sibling standing in. Working-tree blob hashes are now the blob id `git add` would store, with checkin conversion and clean filters applied and no object written, so a CRLF checkout under `text=auto` or an LFS file no longer reads as `stale_spec`, and `accepted_against` pins share the committed-blob space; comparisons try the raw bytes first, so an unedited file still costs no Git process and older raw pins keep matching.

Resolved after the remediation: `attest_criterion` bursts now stay as command cards like review-gate attestations; `add bug|chore|tweak` refuses a blank `--attach` as `attach_target`; a fresh `init` refuses a newer orphaned orientation before writing; `views.QueueKinds` is the single queue kind list and a test requires an attention category for each kind.

The original plan, with its defect register, closure criteria and reproduction inventory, is retired from the working tree. Recover it from the branch history with `git show e0e25fe8:docs/plans/2026-10-09-bug-hunt-remediation.md`.

## Local 0.17.5 distribution

**October 8, authorized local distribution.** Commit `a0a1f961ba524195a89af3c639fce1fc7419012d` stamps **0.17.5**, carrying the [review gate key fix](#review-gate-key-fix) below. Schema **10**, orientation generation **47**, and the eleven embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.5-a0a1f96/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy) share SHA-256 `e27aac640b910cec676205d561f35efb9cfd789e9c026ef7d84ee1fe868bb10b`. The outgoing 0.17.4 executable is retained at `~/.local/share/tiller/previous/0.17.4-f8054052af0b/tiller`.

Lint and the uncached Go suite passed on the stamp. The installed binary reports 0.17.5 on PATH. In a disposable greenfield project, `init` and `join` wrote orientation generation 47 and eleven harness shims, and `tiller attest feature e/f --gate code-review` recorded the attestation with a warning naming the four declared review gates. No managed project needs a refresh: the generation did not change.

## Review gate key fix

**October 8, source only.** [Issue #138](https://github.com/madeintandem/tiller/issues/138): `tiller init` seeds the optional review gates as `code_review`, `security_review`, `performance_review`, and `accessibility_review`, while the CLI spec and two tests attested `code-review`, which silently recorded a second gate and left the declared one unattested. The underscore keys stay, since every seeded manifest already carries them, and the spec and tests now use them. By ruling, `tiller attest --gate` with a key the manifest does not declare warns rather than refuses: unknown gates remain preserved, and the success carries a warning naming the declared keys, in the human text and as `data.warnings` (commit `1937f9ce`).

## Local 0.17.4 distribution

**October 8, authorized local distribution.** Commit `24e4f45ee84d5cd5b2234d5cddfbe167e80d9b3f` stamps **0.17.4**, carrying the [code review skill](#code-review-skill) below. Orientation generation moves from **46** to **47**, and the embedded skills grow from ten to **eleven**; schema **10** is unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.4-24e4f45/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy, so the new inode carries a fresh code signature) share SHA-256 `f8054052af0bebbbf75cee27aff60f975065e1d0c2c8fd2ed5be1d86840a4586`. The outgoing 0.17.3 executable is retained at `~/.local/share/tiller/previous/0.17.3-ca978ab6f58f/tiller`.

The doc-version and version-stamp checks, the upgrade and serve tests, the uncached Go suite, and lint passed on the stamp. The installed binary reports 0.17.4 on PATH. In a disposable greenfield project, `init` and `join` wrote orientation generation 47 and all eleven harness shims, including `.claude/skills/tiller-code-review/SKILL.md`; `tiller guide code-review` printed the skill, and bare `tiller guide` still suggested `orient`. No managed project was migrated, refreshed, or re-joined: each needs `tiller upgrade --project-only` or a re-join to pick up generation 47 and the new `tiller-code-review` shim.

## Code review skill

**October 8, source only.** An explicitly requested `code-review` skill, installed as `/tiller-code-review`, reviews one epic, feature, task, bug, chore, or tweak and turns only the findings the developer names into Tiller records. It is a pure skill: no CLI command, schema, store, or console change. The [design](plans/2026-10-08-code-review-design.md) settled 21 decisions one fork at a time, and the [implementation plan](plans/2026-10-08-code-review-implementation.md) built them in five tasks on commits `7ca2a4d3` through `9f5829a8`.

The review arm is read-only, like `design-brief`, and writes one file under `.tiller/reviews/`, a prose-only sibling of `.tiller/feedback/`. It covers the node's net change: in-flight branches against their merge-base, and merged work (decided by `merge-base --is-ancestor`, never by status, so merged-but-unsynced work is not missed) from the first parent of the first first-parent integration merge that contains it, falling back to commits carrying the unit's ID after a fast-forward, squash, or rebase. Findings come in four numbered sections: bugs graded blocker, important, medium, or minor by impact, with any acceptance or `preserve` criterion violation a blocker and every bug labeled traced or suspected; tweaks; chores; and plan changes. Severity lives only in review prose; no record field or order change was added. Findings already tracked or previously declined are marked and not renumbered.

The disposition arm resolves the whole selection before writing anything, rechecks only selected rows when pinned code moved, logs bugs, tweaks, and chores as `proposed` shelf nodes through `tiller add --attach --source`, and logs plan changes by landing a distillation of only the selected rows through `tiller intake` as one `proposed` amendment. "Fix on its branch" returns an unmerged finding to its unit's owner through execute's requested-changes re-entry. Each disposition annotates its rows in place and makes one commit on the integration branch; approval stays with the human. The attest line prints the manifest's own gate key, which surfaced [issue #138](https://github.com/madeintandem/tiller/issues/138): `tiller init` seeds `code_review` while the specs and tests attest `code-review`.

A fresh agent exercised both arms in a disposable project with planted defects: it found both, plus vacuous `true` acceptance bindings, and every disposition verb behaved as specified. The trial caught an empty diff for merged work, and a fresh whole-branch review caught wrong merge selection after fast-forwards and re-assembly, merged-but-unsynced work read as in flight, shelf-target attachment that `--attach` refuses, over-broad staging, and a handed-off row its owner could not fix. All were fixed with failing-first text pins, as were the review's ten minor findings.

## Local 0.17.3 distribution

**October 8, authorized local distribution.** Commit `93a4a7eb4ddbdbbe32cb1acbade59544fec5265c` stamps **0.17.3**, carrying the three brownfield zero-day builds below and the #137 route-order fix. Orientation generation moves from **44** to **46**; schema **10** and the ten embedded skills are unchanged in number, though the derive-baseline, decompose, orient, intake, and capture skill text changed. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout of the stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.3-93a4a7e/tiller` and the installation at `~/.local/bin/tiller` (installed by remove then copy, so the new inode carries a fresh code signature) share SHA-256 `ca978ab6f58fcb8ae350176a90f1142220767ccecb81392a61b2a1df46061169`. The outgoing 0.17.2 executable is retained at `~/.local/share/tiller/previous/0.17.2-2722265f61f1/tiller`.

The doc-version and version-stamp checks, the upgrade and serve tests, the uncached Go suite, and lint passed on the stamp. The installed binary reports 0.17.3 on PATH. Against a disposable brownfield project with one confirmed as-is doc, it printed `Suggested skill: orient` and `Next (tiller guide orient): no work is planned yet (baseline established): ...` from `guide` and `check readiness`, and `Suggested skill: decompose` once a to-be doc was added; the generated orientation reports generation 46. No managed project was migrated, refreshed, or re-joined: each needs `tiller upgrade --project-only` or a re-join to pick up generation 46 and the changed skill descriptions on its harness shims. Existing console processes must be restarted.

## Brownfield zero-day forward work

**October 8, source only.** Build 3 of the [brownfield zero-day design](plans/2026-10-08-brownfield-zero-day-design.md) gives the ingest somewhere to put the unbuilt work it finds. The trial's ingest had read an unbuilt cross-device sync spec and an unbuilt calculator section and could only write "not implemented" into the as-is docs. `derive-baseline` now records forward work as questions: one per unbuilt capability, citing every source that describes it with `--source`; a code TODO counts only when a document corroborates it, and an old implementation plan only where no spec covers the capability; the ledger is read first so a rerun opens no reworded duplicates. Defect-shaped findings stay questions, never captured bugs, because the seam reads rather than reproduces. The human's answer is the decision: "build it" leads to intake on the cited source or a to-be design, "it's a bug" to capture, and the seam names that move and stops. A standalone arm runs only that step on an already-confirmed baseline, and only when the human asks; nothing records that it ran. The decompose skill's description and trigger no longer fire on an empty tree alone, orient's onboarding stop names the no-work-planned end state, and intake and capture accept an answered forward-work question. Orientation stays at generation **46**. A whole-branch review then tightened the pass: a rerun skips leads covered by any question, open or answered, a proposed amendment, or a shelf node, so a declined lead is not reopened under new wording; the agent records the human's answer with `tiller answer question`; legacy-migration compares sources against reference roots, not the new surface; a retired source is recovered from its blob before intake; `tiller init` no longer prints a fixed `tiller guide decompose` step for existing-code kinds; and orientation's seam catalog says decompose needs to-be inputs.

The same build carries build 2's review follow-ups: `guide` and `check readiness` name the to-be doc the route found, the design-folder walk runs only on a tree with no epics, readiness computes its suggestion only when ready, and a console test pins the no-work-planned `/api/route` payload. On the built binary, `tiller guide derive-baseline` carries the new section and arm, `tiller guide decompose` no longer mentions an empty or fresh tree, the skill's example `tiller add question ... --title "Calculator units: unbuilt" --source "docs/spec.md#4.8"` records its source, and `guide` then reports `no work is planned yet (baseline established, 1 open question)`. Adding a to-be doc returns decompose with `starting with .tiller/design/sync.md`. Managed checkouts need a re-join or `tiller upgrade --project-only` after release to refresh the changed skill descriptions on their harness shims; the standalone scan on the trial project waits for release and JC's go.

## Brownfield zero-day end state

**October 8, source only.** A brownfield trial confirmed its as-is baseline and was immediately told `Next: decompose`, though `.tiller/design/` held nothing but as-is docs. The route now names decompose on an empty tree only when a to-be design doc exists: a file under `.tiller/design/`, dotfiles excepted, that `baseline.yml` neither registers as an as-is doc nor cites as a `--from` source, matched without case so a case-insensitive filesystem cannot make a registered doc look new. Tree load lists them through the confined reader and never fails on anything in that folder. A tree with no epics and no to-be doc reaches the no-work-planned end state, a caught-up flavor marked `Unplanned` with no seam. Its human-owned Next names the open-question count when nonzero and the ways work enters: intake, capture, or a to-be design for existing-code projects, design inputs for greenfield. `check readiness` no longer hard-codes decompose; a ready project closes on guide's own suggestion, `Next (tiller guide <skill>): <reason>`. Orientation generation **46** restates the rule, and the orient skill and specs match. A whole-branch review caught three gaps in the first cut, all fixed with tests: a cited source left under design/ counted as new work, a case mismatch between a registered path and the file on disk did too, and a Markdown-only rule told a greenfield project holding PDF or text inputs that it had none. The console is unchanged: its route strip shows "Nothing due" beside the end state's reason.

Against a disposable brownfield project with one linked and confirmed as-is doc, the build 1 binary printed `**Next:** decompose: no epics recorded yet.`, `Suggested skill: decompose`, and `Next: decompose. Read .tiller/orientation.md or run 'tiller guide decompose'.` After this build, `approve design`, `guide`, `check readiness`, and `readout orient` all report `no work is planned yet (baseline established): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)`; one open question adds `, 1 open question` inside the parentheses; adding `.tiller/design/sync.md` returns `Suggested skill: decompose` and `Next (tiller guide decompose): ...`. This is build 2 of the [brownfield zero-day design](plans/2026-10-08-brownfield-zero-day-design.md); the derive-baseline forward-work pass and the decompose and derive-baseline skill text remain.

## Empty-tree route order

**October 8, source only.** [Issue #137](https://github.com/madeintandem/tiller/issues/137): on a tree with no epics, the route named `decompose` ahead of every other arm, so a maintenance-only project with an owned, approved bug, or an intake amendment staged against an empty tree, was told to decompose. A brownfield trial surfaced it the day its baseline was confirmed. The empty-tree arm now runs last, just before caught-up: owned shelf work, a proposed amendment, and a shelf merge wait route first, while unclaimed shelf work still never enters the route. Its output is unchanged. Orientation generation **45** restates the order, and the orient skill and specs match.

Against a disposable greenfield project, the 0.17.2 source before the fix printed `Suggested skill: decompose` and `**Next:** decompose: no epics recorded yet.` for a bug that was approved and claimed. After the fix, an empty tree and a proposed unclaimed bug still suggest `decompose`; the approved, claimed bug gives `Suggested skill: execute` from `guide` and `Route: execute: bugs/blank-save (...) is ready` from `readout orient`. This is build 1 of the [brownfield zero-day design](plans/2026-10-08-brownfield-zero-day-design.md); the to-be-input check, the no-work-planned end state, and the derive-baseline forward-work pass remain.

## Local 0.17.2 distribution

**October 8, authorized local distribution.** Commit `adf9e6e182bbc1182388087026ba0c3f11743f89` stamps **0.17.2**, integrating the shared Build/Fix board scrolling, full-height columns, collapsible Fix filters, count summaries, compact page chrome, joined scroll selectors, and Explore tab inset described below. Schema **10**, orientation generation **44**, and ten embedded skills are unchanged. The stamp was committed to local `main`, not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.2-adf9e6e/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `2722265f61f15ac7957266b8d2f0cfe124aee4de3d5f482302ec3695a12d2671`. The outgoing unstamped 0.17.1 executable is retained at `~/.local/share/tiller/previous/0.17.1-577d896ebabc/tiller`.

Version and documentation stamp checks, the uncached Go suite, lint, all **1,288 console tests**, the console build, and the **15-page documentation-site build** passed. Independent static review found no blockers. Chromium exercised both scrolling modes on Build and Fix, count summaries, inert collapsed filters, Explore spacing, Home, Browse/Filter/Search, and node-detail navigation against the stamped source binary. The installed binary reports 0.17.2 on PATH; its console retained the shared mode, compact selectors, counts, and narrow Explore inset without browser runtime errors.

A disposable project created by outgoing 0.17.1 remained byte-identical through the new binary's reads and already-current project-only refresh. No managed project was migrated or refreshed. The requested pre-implementation board plan is preserved in the stamp commit and retired after integration, with its durable outcome in this history and contract in the console spec. Existing console processes must be restarted.


## Compact console page chrome

**October 8, local unstamped build.** Fix and Explore no longer repeat their area names as visible page headings. Fix's filter sidebar and board toolbar start directly beneath the main header, matching Build's frame, and Explore's mode rail starts directly beneath the main navigation. Sidebar and board content retain their own gutters. Active navigation highlights and browser titles continue to identify each area; node and file titles remain visible, and initial loading shells are unchanged.

All **1,288 console tests**, the TypeScript/embedded console build, the real binary build, and uncached UI embedding tests passed. Obsolete Explore-heading assertions and CSS-source layout tests were removed rather than repinned. Chromium verified exact header alignment in light and dark at desktop and 375px widths, Fix sidebar collapse and both scroll modes, Explore Browse/Filter/Map, and retained node headings. No browser runtime errors were recorded.

The rebuilt darwin/arm64 binary was installed atomically at `~/.local/bin/tiller` under the unchanged **0.17.1** stamp. Its retained candidate is `~/.local/share/tiller/distributions/0.17.1-compact-892382f53dba/tiller`, SHA-256 `892382f53dba8e3b19774c1cd2b5b7bb64fc26171bf30aee010669508c2b7fd7`; the outgoing board build is retained at `~/.local/share/tiller/previous/0.17.1-376ba1e2928d/tiller`. The installed console served both compact layouts with the expected active navigation and no repeated H1. No stamp, commit, publication, or managed-project change was made; existing console processes need restarting.

The next local refinement joined the Board/Columns buttons using the color-mode selector's shared style and selected fill. Their 24px height matches the sidebar toggle and stays shorter than the 25px color-mode control. Explore's tabs gained an 8px top inset without restoring the area heading. Chromium verified both scroll options on Build and Fix in both themes, compact controls at narrow width, Explore spacing at desktop and 375px, and persistence after restarting with the installed binary. The 1,288 console tests, console build, binary build, and uncached embedding tests passed, with no browser runtime errors.

The local binary was refreshed under the unchanged 0.17.1 stamp. Its retained candidate is `~/.local/share/tiller/distributions/0.17.1-selectors-577d896ebabc/tiller`, SHA-256 `577d896ebabc2b8feb5a4ac4591b341bbcc0f8b31de7ad7432a25656d2b9c0e7`; the outgoing compact build is retained at `~/.local/share/tiller/previous/0.17.1-892382f53dba/tiller`. No version stamp or commit was made.


## Build and Fix board scrolling

**October 8, source only.** Build and Fix share a top-right Board/Columns scrolling control with one remembered browser preference, defaulting to Board. Board scrolling moves all columns together; Columns scrolling keeps each column independent. Both retain horizontal panning, opaque sticky headings, and full-height backgrounds for empty and sparse columns. Board backgrounds extend to the tallest content. Build's top gutter sits outside the scrollport so cards cannot show above its headings.

Fix's filter sidebar uses Build's collapse button and behavior, with a separate remembered collapse state. Collapsing either sidebar removes its controls from keyboard and assistive-technology navigation without changing selection. Build's toolbar reports rendered features across matched epic counts, not epic names; Fix reports visible bugs, chores, and tweaks after filters and its Closed history window. Explore remains unchanged after clarification that the full-height request concerned Build's columns.

All **1,305 console tests**, the TypeScript/embedded console build, the real binary build, and uncached UI embedding tests passed. Chromium verified both scrolling modes and horizontal panning on both pages against a CLI-populated disposable project, full-height backgrounds, sticky-header coverage, filter/count changes, separate collapse persistence, keyboard skipping of hidden filters, shared scroll persistence, storage failure fallbacks, both themes, and narrow controls. No browser runtime errors were recorded. Existing topbar responsive/height limitations and the large bundle warning remain outside this change. No installation, publication, or managed project changed.


## October 8 review fixes

**October 8, review fixes.** A review of everything shipped from 0.16.2 through 0.17.0 found eight defects, and all eight are fixed in commits `7835e7bd..90b07b21`. `review_missing` now names only submitted work that is waiting on an unrecorded aggregate review, instead of every unreviewed node. A recovered aggregate that has since closed no longer blocks loading when the review grain changes. A console read that joined another read's shared load now retries once when a Tiller mutation moved the generation under it, and answers with the same stability refusal only if the retry is also overtaken. The feature and epic lists observe only recovered owners, and read-side workflow observation reuses a passing recovered-evidence check for the same record and integration tip, so a page of reads no longer reruns about ten git processes per recovered node. After review, the cache key gained a basis field so a pass is never reused across a different review basis. A cancelled feature no longer inherits its epic's recovered review, though it keeps a recovered review of its own. Two smaller cleanups landed: an unreachable recovered-evidence branch in transfer is gone, and the hexadecimal commit-ID check lives only in `vcs`.

A cached evidence pass assumes the record's commits stay present. If a reviewed head is later pruned, `tiller serve` keeps the pass until integration moves or the process restarts, while sync, transfer, and recovery still refuse with `object_unavailable`. The joined-read tests assert the stability message on each refused response.

Schema **10**, orientation generation **44**, and the ten embedded skills are unchanged. [Local 0.17.1 distribution](#local-0171-distribution) installs these fixes.

## Merged aggregate review recovery

**October 7, source only.** Feature and epic submission accept explicit `--recover-merged --head <historical-commit> --merge-commit <integration-result> --reason <explanation>` from configured integration. Recovery records a missing historical aggregate review without running gates, changing lifecycle state, resetting refs, or committing to a merged branch. Remote-backed recovery requires a positive caller-supplied PR number; the runtime verifies local Git evidence and never claims to have observed the host.

Schema **10** adds typed recovery provenance, frozen riding-task state blobs, and historical policy snapshots. The **9→10** migration only advances the manifest stamp and preserves active work and historical events. Orientation **44** and the execute skill explain explicit authorization, external host verification, recovery on integration, ordinary sync, actual state commits, and readback.

The named result must remain on configured integration's first-parent history. A side-parent merge proves ancestry; an exact complete squash patch requires both stable patch-ID and whitespace-faithful digest equality. Containment alone records no observed merge and remains pending until the existing human fast-forward assertion. Historical submitted state, immutable task scope, retained task tips, and review-grain boundaries constrain inclusion. Deleted or advanced aggregate refs do not replace the historical witness. Repeated recovery is a no-write success that preserves the original actor, time, and reason; reopening clears current recovery while the audit event preserves the old round.

Sync revalidates the immutable result and frozen riders, retains ordinary completion and ancestor rollup semantics, and adds source-pinned historical policy warnings separately from crossing-time warnings. A historical pending criterion and a later approved contract change/current pass remain independent facts. CLI workflow, queue, guide, execute readout, Home, and node detail expose recovery provenance and current acceptance without asking for another PR or commits to an already merged branch.

Disposable real CLI replay covered feature and epic recovery, advanced and deleted aggregate refs, deleted task refs, a named squash beyond 55 later commits, caller-supplied remote PR identity, contained work remaining pending until an explicit human-simulation assertion, and fifteen unsafe-input/misuse no-write refusals. Fresh final-candidate fixtures proved staged-only relevant-input refusal, unrelated dirty-file allowance, record-only writes, ordinary crossings, independent historical and crossing-time warnings, and byte-identical retries/repeated sync. A project created by the installed schema-9 binary migrated to schema 10 while preserving work and events; the old binary then refused it.

Chromium exercised recovered Home and node detail in light and dark. Historical pending C8 stayed visible beside a later approved removal/current pass, and a completed recovered epic retained independently pending current acceptance and both warning groups. A missing terminal Home acceptance projection found in that replay was fixed and verified at the rebuilt embedded surface. No runtime errors were recorded. Version remains **0.16.3**; no installed binary, published release, or live managed project changed. PR-host verification and live managed-project recovery were not exercised.

Focused recovery, normalized-input, SHA-256 Git, schema, and read-projection regressions passed. The final uncached Go suite, build, lint, all **1,323 console tests**, TypeScript/embedded console build, version/documentation checks, and **15-page documentation-site build** passed. Independent schema, Git, planner, sync, and read-surface reviews closed their findings. The completed implementation plan was folded into this record; disposable projects, candidate binary, servers, and browser tabs were removed after proof.

The authorized main integration preserves the newer console-read performance implementation and version stamp **0.16.4**, while source adopts schema **10** and orientation **44**. The installed 0.16.4 binary remains schema 9 and generation 43. The combined tree passed the uncached Go suite, build, lint, all **1,323 console tests**, embedded console and documentation-site builds, and documentation-version checks. A fresh project created with installed 0.16.4 migrated without rewriting work or existing events, recorded the omitted aggregate review, crossed both tasks on ancestry with ordinary rollup, and retained independent historical/current warnings and no-write retries. Chromium verified Home in dark and node detail in light without recorded runtime errors. No installation, publication, or live managed-project change occurred.

## Contributor identity and work ownership

**October 6, source only.** Registered project members and acting identity are contributors. Registration and retirement use `tiller add contributor <id> --name "<name>"` and `tiller remove contributor <id>`. Acting identity resolves from `--contributor`, then `TILLER_CONTRIBUTOR`, then the checkout-local identity. The manifest roster is `contributors:` and local identity is `contributor:`. Old roster commands, acting flags, and environment names are not aliases.

Execution claims remain `owner` on Tasks, Bugs, Chores, and Tweaks, including immutable started and terminal attribution. Question asker attribution also retains its existing `owner` field. `ls --owner` filters scalar assignments; ancestor `ls --contributor` filters contribution from started descendant tasks, not roster registration. Console roster and event actor payloads use contributor vocabulary. New events store `contributor:`; historical `owner:` events remain readable without changing their bytes.

Schema **9** and orientation **43** carry the cutover. The **8→9** migration renames roster and local identity keys in the guarded source transaction while preserving IDs, display names, claims, active work, evidence, branches, reviews, unknown manifest content, formatting, and epoch. It requires neither a work drain nor an ownership review. Earlier migrations retain their own safety requirements. Current-schema reads reject legacy roster and identity keys rather than silently losing attribution.

All Go packages passed uncached verification, as did lint, all **1,275 console tests**, the TypeScript/embedded console build, documentation-version checks, and the **15-page documentation-site build**. The real CLI migrated an active schema-7 project through schemas 8 and 9, preserving its task record and eleven historical events byte-for-byte, retaining the selected identity, and refreshing orientation. Fresh-project smoke exercised registration, join, retirement, flag/environment precedence, task claim/start, ownership filters, immutable-owner refusal, and rejection of retired command/flag/environment inputs. Chromium verified the Home roster, contributor versus owner filters, task Owner detail, and migrated historical event attribution with no recorded runtime errors.

Synchronization with `origin/main` retains execution-source settling and event-backed ownership tie handling alongside contributor identity. Route recommendations use the served checkout and mute actions for observed execution branches. The combined tree passed all Go packages uncached, lint, all **1,278 console tests**, and the rebuilt embedded console. A disposable real CLI and Chromium smoke verified the contributor roster, unchanged task Owner attribution, execution-branch provenance, and muted integration-checkout routing without console errors.

Version remains **0.15.6**. No installation, publication, managed-project migration, or existing user console changed.

## First-class Tweaks

**October 6, source only.** [Issue #134, first-class Tweaks](https://github.com/madeintandem/tiller/issues/134), adds a shelf peer to Chore: `tweak`, `TW-` IDs, and `.tiller/tweaks/<key>/tweak.md` plus `tweak.yml`. It shares ownership, pinned contracts, gates, independent review, attachments, shelf ordering/dependencies, carriers, transfer, and completion. The existing shelf console becomes **Fix** at `/fix`, combining Bugs, Chores, and Tweaks rather than adding another board.

Choice guidance distinguishes defects (Bug), housekeeping without code side effects (Chore), minor adjustments that may change code (Tweak), and substantive planned work (Task). Chore guidance is not a code-change prohibition. A button-text refinement is forward Tweak work; an agreed-label defect is a Bug. Done execution records remain history, never reopened to absorb new requests.

Schema **8** makes older binaries refuse rather than omit Tweaks. The **7→8** migration changes only the manifest stamp and preserves existing Bug/Chore records and active work without a drain or ownership review. Orientation **42** adds routing and choice guidance.

Verification passed the uncached Go suite, lint, all **1,261 console tests**, TypeScript and embedded console build, the **15-page documentation-site build**, version/documentation stamp checks, and screenshot fixture safety checks. The real CLI migrated a schema-7 project built by the outgoing binary, preserving existing Bug/Chore and completed tree records byte-for-byte. A button-text Tweak crossed approval, claim, start, a failing required check, implementation, passing checks, submission, observed merge, and sync without adding a feature task or reopening completed work. Additional CLI smoke covered plural verbs, ordering, rename, removal, ownership refusal, transfer, dependencies, carrier completion, queue visibility, and guide routing.

Chromium exercised the combined Fix board, Tweak-only and mixed noun picks, attachment filtering, closed transfer history, node detail, and Explore browse/filter/search/map with no recorded runtime errors. Home, map, and Fix documentation screenshots were refreshed in both themes from the CLI-built Acme fixture. Disposable servers and browser tabs were stopped. No installation, publication, or managed-project adoption occurred.

## Console task execution sources

**October 6, source only.** Issue #133, visibility of in-flight task work, adds request-local console observations from task worktrees and locally available task branch tips. The served tree retains approved structure and merge-backed lifecycle authority. Only task execution fields are overlaid; source records identify the selected branch, an opaque read token, and inspection time. The console never fetches or publishes state.

Discovery uses the recorded branch or canonical branch pattern, validates task identity and approval basis, and prefers the matching execution worktree. Committed local and configured tracking refs provide fallback observations; divergent tips remain diagnostic. Task detail merges selected-source task events with integration history and pins evidence links to the selected source and artifact SHA-256. Changed evidence refuses with HTTP 409. Global History, queue, route, and workflow remain served-tree reads; parent activity can reflect observed tasks without establishing integration readiness.

Source reads preserve nested project prefixes, reject filesystem and Git gitlink escapes, inspect staged gitlinks without refreshing the index, disable partial-clone lazy fetching, and close source identity, generation, and content stability boundaries. Integration cancellation, replacement, retirement, completion, and merge-time violations cannot be overwritten by stale branches. The execute guide explains ordinary branch publication and external fetch/manual refresh, without a state-copy step.

All Go packages passed uncached verification; the execution package was rerun after removing a wording-only assertion from its generation-invalidation regression. Lint, the TypeScript/production console build, and all 1,252 console tests passed. Independent behavior and source-security reviews closed their findings. A rebuilt binary served a disposable project with independently owned task worktrees: Home, tree/filter/detail, source history and gate evidence, submission/resume, committed and tracking fallback, and merge/sync precedence were exercised. Chromium checked owner filtering, source diagnostics, manual refresh, task panels, and light/dark layouts without runtime errors; rewritten evidence returned HTTP 409 while integration state stayed unchanged.

Schema 7, orientation generation 41, and the 0.15.6 version stamp remain unchanged. No installation, publication, managed-project refresh, or existing user console changed. Automatic refresh and cross-machine fetching remain outside this change. Disposable servers, browser tabs, task worktrees, projects, and binaries were removed.

**October 6 review fixes.** A code review of the first build found three problems that would show up with several agents in parallel worktrees. Every console read rechecked each source repeatedly: about 200 Git calls and 1.8 seconds per request with four task worktrees, even for queue and drift, which never use observations. A commit in any observed worktree during a read failed the whole response with HTTP 500. A stale task branch outranked a release or reassign recorded on integration whenever integration had not recorded the start.

Source reads are now captured once and confirmed once, after the projection, with one batched ref read and two Git calls per worktree. Queue, recap, drift, prose, and global History skip source discovery. A source that moved during the read drops only its own observation, with a `source_unstable` diagnostic, and the projection reruns; only movement in the served checkout is a 500. Owner disagreements are broken by the newest `claim`, `release`, `reassign`, or `start` task event, and a newer integration decision shows integration ownership with `ownership_superseded`. Smaller fixes: related-tip freshness moved from diagnostics onto the record, `uncommitted` covers only `.tiller/` state, another task's invalid event no longer breaks a task's history, the console caps visible diagnostics at eight, and an observed task keeps its queued asks without their commands.

On a disposable four-worktree project the fixed binary served the board in 0.31 seconds with 30 Git calls, and queue in 3 ms with none. With an agent committing in one worktree, the pre-fix binary returned HTTP 500 on 2 of 25 board reads and the fixed binary on none. Lint, the uncached Go suite, and all 1,274 console tests passed. Browser rendering was not checked; the console unit tests cover the UI changes. The 0.15.6 stamp, schema, and generation are unchanged.

The authorized main merge preserves the newer first-class Tweak surface, schema 8, and orientation generation 42. Documentation conflicts retain both changes, and the embedded console assets were rebuilt from combined source. Execution test manifests now use the current schema constant rather than a schema-7 stamp. The uncached merged Go suite and lint passed, as did the console build and all 1,268 UI tests. A rebuilt binary and Chromium confirmed Home, Fix, Explore, Tweak detail, task provenance, manual refresh, and both themes: an uncommitted task appeared in progress while its integration record stayed ready and the independent Tweak remained visible. Disposable verification resources were removed.

## Local 0.17.1 distribution

**October 8, authorized local distribution.** Commit `4219faf8cc4cf401c301824346470ae48b7d4fa7` stamps **0.17.1**, carrying the [October 8 review fixes](#october-8-review-fixes) over 0.17.0. Schema **10**, orientation generation **44**, and ten embedded skills are unchanged, so managed projects need no migration or re-join. The stamp was committed to local `main` and not pushed; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.1-4219faf/tiller` and the installation at `~/.local/bin/tiller` share SHA-256 `8ff0d0e70614b4afdbe66ab55a228607d21f57562c4b55f4aa1e305c6d58f8ed`. The outgoing 0.17.0 executable matched its recorded digest and was retained at `~/.local/share/tiller/previous/0.17.0-aed22c671680375860e7069d34a97410e72a2289585a7a058db92aa8f63cfad7/tiller`.

Version and documentation stamp checks, the uncached Go suite, lint, all **1,324 console tests**, and the **15-page documentation-site build** passed. A disposable project created by the outgoing 0.17.0 was read by the installed 0.17.1 on PATH, which completed an already-current project-only refresh and served the console and its status API. No browser check was run. The disposable project and server were removed. Existing consoles must be restarted to pick up the fixes.

## Local 0.17.0 distribution

**October 7, authorized local distribution.** Commit `a5bd68ba7e03ff0c18225981829af434dede0f5b` stamps **0.17.0**, carrying [merged aggregate review recovery](#merged-aggregate-review-recovery) and the preceding console-read performance improvements. Schema **10**, orientation generation **44**, and ten embedded skills move together. The source stamp was committed and pushed to `origin/main`; no release tag or GitHub Release was created.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.17.0-a5bd68b/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `aed22c671680375860e7069d34a97410e72a2289585a7a058db92aa8f63cfad7`. The actual outgoing 0.16.4 executable was retained and verified at `~/.local/share/tiller/previous/0.16.4-e2b532e6dc885fb6ba285eeb91ea0d2e48577204768396dc8898643aa332f3eb/tiller`; installation identity and bytes were rechecked before replacement.

Version and documentation stamp checks, the uncached Go suite, build, lint, all **1,323 console tests**, the embedded console build, and the **15-page documentation-site build** passed. A disposable project created by outgoing 0.16.4 migrated from schema 9 to 10 and orientation 43 to 44, retaining node records, selected identity, and existing events byte-for-byte. The installed executable reported 0.17.0 on PATH, read the migrated project, and completed an already-current project-only refresh. Chromium verified the candidate Home in light and the installed Home in dark with the new version displayed and no recorded runtime errors.

Disposable checkouts, projects, servers, and browser tabs were removed after proof. No publication, live managed-project migration, or live recovery occurred. Existing consoles must be restarted; project migration and guidance refresh remain separately authorized adoption steps.

## Local 0.16.4 distribution

**October 7, authorized local distribution.** Commit `e700f54b0e6c491743d399cc52bfc5d9c2ce9482` stamps **0.16.4**, console read performance over 0.16.3. On a large managed project (374 nodes, 2,691 event files, 20 worktrees), Explore and Build page loads had grown to 1.4-2.7s. Every confined read re-walked from the repository root, ownership tie-breaks parsed the whole event stream on nearly every request, each page's parallel reads loaded the ledger independently, and a request spawned dozens of git processes. Confined reads now verify each directory once per read through a bounded reader. `tiller serve` caches parsed event files, revalidated by file identity, size, mtime, and mode, and caches commit tree inventories under an entry budget. Concurrent reads share one in-flight load while each closes its own stability boundary. Execution checks worktree sources concurrently and trusts an untouched index by file identity outside a racy window, and drift answers surface hashes from one HEAD tree listing. Page bursts dropped to 0.35-0.48s and every compared API payload matched 0.16.3. Schema **9**, orientation generation **43**, ten embedded skills, and the on-disk format are unchanged, so managed projects need no migration or re-join.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.16.4-e700f54/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `e2b532e6dc885fb6ba285eeb91ea0d2e48577204768396dc8898643aa332f3eb`. The outgoing unstamped `cb60ae0e` build of the same source is retained at `~/.local/share/tiller/previous/0.16.3-cb60ae0`.

Version and documentation stamp checks, the upgrade fixtures, the uncached Go suite, and lint passed. The installed candidate served that managed project on a scratch port. No tag, push, publication, or managed-project change occurred; existing consoles must be restarted.

## Local 0.16.3 distribution

**October 7, authorized local distribution.** Commit `828fda65da874e4420744a15e0d28c33c60bf5b5` stamps **0.16.3**, console polish over 0.16.2. On Fix, Attached to results stack the kind label above a full-width title, and the older-items link under the board becomes a **Show older items** switch at the foot of the filter rail, still carried by `terminal=all`. In Explore, the Bugs, Chores, and Tweaks shelves move below the epics. Questions and Amendments both render inside the Explore layout with the navigator beside them and keep Explore marked in the top navigation; Amendments moves from a Filter preset to its own `/amendments` ledger while remaining a Filter noun. Each ledger carries a status rail, All plus each status with its count, on `?status=`. Schema **9**, orientation generation **43**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.16.3-828fda6/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `20cde5ce61e7dd5b83c07804e1e9c9310a64a1071402f1288454d9f269531039`. The verified outgoing 0.16.2 executable is retained at `~/.local/share/tiller/previous/0.16.2-0dc8539f5d81`.

Version and documentation stamp checks, the uncached Go suite, lint, all 1,310 console tests, and the embedded console build passed. Headless WebKit exercised each change on a scratch project in light and dark: the stacked results, the switch widening Closed from five rows to seven and back, shelves below the epics, and both ledgers' status rails beside a standing navigator with Explore marked. The installed candidate served `/questions`, `/amendments`, and `/fix` from the rebuilt assets. No real-browser pass was made. No tag, push, publication, or managed-project change occurred; existing consoles must be restarted.

## Local 0.16.2 distribution

**October 7, authorized local distribution.** Commit `ea7d2b791f90473c102e00bdd89222c265609b74` stamps **0.16.2**. Bugs, Chores, and Tweaks get their own type colors as shared `--noun-*` tokens (violet, copper, lime, kept off the status hues), on Fix card chips and as swatches on the Noun filter rail; text on each tint clears 4.5:1 in both themes. Fix's Attached to becomes a searchable multi-select whose results name their kind (Epic, Feature, Task) with key or ref beneath, chosen targets riding as removable chips. `?attached=` is a comma list matched as OR, Unattached rides as its `none` member, and single-id links still parse. Schema **9**, orientation generation **43**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.16.2-ea7d2b7/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `0dc8539f5d819b25610cb8eba64eaaea87b33ed9c2e306c0447df0c220279458`. The verified outgoing 0.16.1 executable is retained at `~/.local/share/tiller/previous/0.16.1-15d45d18c153`.

Version and documentation stamp checks, the uncached Go suite, lint, all 1,302 console tests, and the embedded console build passed; one uncached run hit a transient `cp` race in `TestReadoutAcrossTheArc` that passed alone and on a full rerun. Headless WebKit exercised Fix in light and dark on a scratch project: noun chips beside status chips, search, keyboard selection of two targets, their union, and the URL round trip. The installed candidate served the same project with the rebuilt assets. No real-browser pass was made. No tag, push, publication, or managed-project change occurred; existing consoles must be restarted.

## Local 0.16.1 distribution

**October 6, authorized local distribution.** Commit `fb760a535413b64c958f3137227ac5c13cede02d` stamps **0.16.1**. Execution sources and their visibility diagnostics no longer sit above every console page: Home gains an Overview and Branches tab rail, Branches lists where each in-flight task's branch or worktree was found and which recorded branches could not be found, and its label carries the diagnostics count in the warning tone. The choice rides `?tab=branches`, and Home reads `/api/execution` once per load rather than on every navigation. A managed-project trial prompted it, where the panel's four rows took the top of every page. Schema **9**, orientation generation **43**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.16.1-fb760a5/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `15d45d18c15324adbbfdbffe3e739e7756d2ed95972d8e391c86819e06b4423b`. The verified outgoing 0.16.0 executable is retained at `~/.local/share/tiller/previous/0.16.0-9da4c90d3787`.

Version and documentation stamp checks, the uncached Go suite, lint, all 1,286 console tests, and the embedded console build passed. Headless WebKit exercised the tab in light and dark with keyboard switching before the rename. The candidate served a managed project read-only and reported **0.16.1** with six observations and three diagnostics, the stranded pdf-rendering tasks. No real-browser pass was made after the rename. No tag, push, publication, or managed-project change occurred; existing consoles must be restarted.

## Local 0.16.0 distribution

**October 6, authorized local distribution.** Commit `2ba43f5c9f17ee7fc270139b99f293527ab0a34f` stamps **0.16.0**. Since 0.15.6, Tweak joins Bug and Chore as shelf work (schema **8**), project membership and acting identity become contributor (schema **9**, orientation generation **43**), and the console observes task execution across worktrees and branches. A code review of the two days before the stamp produced fixes in the same range: a pulled checkout's legacy `owner:` identity key is renamed by `tiller upgrade --project-only` or a bare `join`; migration refuses an epic keyed by a reserved shelf folder; console evidence and prose reads refuse symlinks and `.tiller/` dot directories, and search skips them; execution-source evidence is capped at 8 MiB; and only a request carrying the console's `X-Tiller-Visit` marker advances the recap horizon. An active epic keyed `tweaks` still has no migration path and stays on its older binary, by decision. Ten embedded skills are unchanged in count.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.16.0-2ba43f5/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `9da4c90d3787e1970fd6f8bd76b3ce6d4a011f2d149bcb72016b969edb0db140`. The verified outgoing 0.15.6 executable is retained at `~/.local/share/tiller/previous/0.15.6-25972da842ba`.

Version and documentation stamp checks, the uncached Go suite, lint, all 1,279 console tests, and a fresh embedded console build passed. The precommit source binary completed the verify arc with three sync crossings and live checks of each fix against a served disposable project. The candidate scaffolded a project with a contributor and a tweak, and its console answered `/api/version` with **0.16.0**. The documentation site was not rebuilt, and no browser pass was made.

No tag, push, publication, managed-project refresh, or restart of an existing user console occurred. Managed projects on schema 7 need `tiller upgrade --project-only` per checkout, and existing consoles must be restarted to use the installed binary.

## Local 0.15.6 distribution

**October 6, authorized local distribution.** Commit `d430d3ac4cb53bd813a48c5848dd65998d88addc` stamps **0.15.6** and simplifies console navigation. The main rail is Home, Explore, Build, Fix, History; Fix opens `/fix` (the old `/bugs-chores` address redirects there) without renaming the board or other shelf-work labels. Explore shows capitalized Browse, Filter, and Map, followed by Questions and Amendments. Questions carries only its open-count pill. Search starts from the Browse search box, and direct search URLs remain supported. Schema **7**, orientation generation **41**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.6-d430d3a/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `25972da842ba2086dff36f375bbdc6145680242de1c7fb000e0c879c21cb28a8`. The verified outgoing 0.15.5 executable is retained at `~/.local/share/tiller/previous/0.15.5-605de3bb2c4c`.

Version and documentation stamp checks, the uncached Go suite, lint, TypeScript and embedded console build, and 15-page documentation-site build passed. The preceding UI verification passed all 1,245 console tests, exercised open and zero-open badges, and confirmed the ledger summary remained unchanged. The precommit binary served a populated disposable project. Installed CLI and `/api/version` report **0.15.6**; Chromium verified main-navigation order, capitalized Explore tabs without Search, the open-only pill, Browse search results, and Fix routing and current state. Light and dark layouts were visually checked with no browser runtime errors.

No tag, push, publication, managed-project refresh, network-settings change, or restart of an existing user console occurred. Existing consoles must be restarted to use the installed binary. Disposable servers, project, smoke executable, browser tab, and detached checkout were removed; the candidate and previous executable remain retained.

## Local 0.15.5 distribution

**October 6, authorized local distribution.** Commit `ace3b0e381870516d5dee9c940d492561c6bf5d2` stamps **0.15.5** and moves Questions and Amendments out of the Explore sidebar into the navigation rail after Map, separated by a vertical rule. Questions retains its open-count badge and total across every mode. Both destinations mark their link current, and question or amendment navigation preserves tree expansion. Removing the two sidebar rows adds 103 pixels of tree height at the verified 1280-by-800 viewport. Schema **7**, orientation generation **41**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.5-ace3b0e/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `605de3bb2c4c15fefd43fe2162f37689305a57dc13d0fd800e41149b76155b27`. The verified outgoing 0.15.4 executable is retained at `~/.local/share/tiller/previous/0.15.4-1b9f08c12d90`.

Version and documentation stamp checks, the uncached Go suite, lint, TypeScript and embedded console build, and 15-page documentation-site build passed. The preceding UI verification passed all 1,246 console tests and exercised counts across Browse, Filter, Search, and Map. The precommit binary served a populated disposable project. Installed CLI and `/api/version` report **0.15.5**; Chromium verified the rendered version, separator and links, question counts, ledger and amendment-list navigation, amendment detail, active links, and retained tree expansion in light and dark themes with no browser runtime errors.

No tag, push, publication, managed-project refresh, network-settings change, or restart of an existing user console occurred. Existing consoles must be restarted to use the installed binary. Disposable servers, project, smoke executable, browser tab, and detached checkout were removed; the candidate and previous executable remain retained.

## Local 0.15.4 distribution

**October 6, authorized local distribution.** Commit `ee2df7c20419630e03875ed5454362999a89472f` stamps **0.15.4** and makes bare `tiller serve --host` listen on `0.0.0.0:4400`. Omitting the flag remains loopback-only; explicit hosts and port overrides retain their behavior. The installed build also includes Explore filter facets and the merged clipboard fallback fix. Schema **7**, orientation generation **41**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.4-ee2df7c/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `1b9f08c12d906c3d9a09647c16b547cc5edab6643cf7301a8c739f97b941cd69`. The verified outgoing 0.15.3 executable is retained at `~/.local/share/tiller/previous/0.15.3-a2a8fb27e4e5`.

Version and documentation stamp checks, the uncached Go suite, lint, binary build, and 15-page documentation-site build passed. The precommit and installed binaries served a disposable project using bare `--host` on port 4400. Installed CLI and `/api/version` report **0.15.4**; loopback and LAN reads succeeded, the embedded HTML and JavaScript were served, and an untrusted Host received HTTP 403. Browser rendering was not reverified during this distribution.

No tag, push, publication, managed-project refresh, network-settings change, or restart of an existing user console occurred. Existing consoles must be restarted to use the installed binary. Disposable servers, project, smoke executable, and detached checkout were removed; the candidate and previous executable remain retained.

## Explore filter facets

**October 6, source only.** Explore replaces free-text filters and the apply button with one hand-built ARIA combobox per applicable axis. Go owns the noun-to-axis table and counts each option against every other active filter through `views.Matches`. The API supplies ordered facets, the full axis table, and unresolved parent values. CLI `ls` retains its output, flags, and strict parent refusal; no dependency was added.

Each axis carries one value and applies only on pick. Typing narrows by label, key, or ID; keyboard arrows and Enter select, while Escape and blur restore the selection. Zero-count options disappear except the selected value. Stored statuses follow lifecycle order, with `unfinished` first for tasks, bugs, and chores. People display roster names but carry IDs, with the acting owner pinned as "You" and unlisted people displayed by ID.

Parents display title and key but carry immutable IDs, ordered by dependencies with cancelled parents last and dimmed. Old key-based URLs still select the resolved parent. Picking an epic clears a feature outside it; picking a feature does not fill the epic. Noun switches preserve surviving axes and drop the rest. Inapplicable URL axes are removed with a history replace; an illegal carried status stays visible at 0. Unresolved parents return 200 with no rows and a clearable inline note, rather than failing the entire read.

Verification used a rebuilt embedded console against a disposable scratch project and the Acme-shaped platform fixture. Chromium exercised noun-specific axes, immediate status picks and updated counts, name-to-ID owner picks, owner carry to bugs, history-replaced inapplicable filters, stale-parent notes and clearing, old key URLs, illegal carried statuses at 0, immutable parent IDs, and parent-narrowed feature options. A 29-option feature list stayed within the bar, scrolled by keyboard, and remained readable in light and dark themes. Home, Browse, Search, and searched node detail remained reachable with no browser runtime errors. A missing-key parent fixture proved that unresolved raw parent links produce zero rows and matching facet counts, and that unresolved parents are not offered as unselected picks.

The uncached Go suite, `make lint`, focused views/serve and console regressions, all 1,244 console tests, and the TypeScript/production console build passed. Independent backend and console reviews closed their findings. Regression coverage includes plural shelf noun URLs, retained-query races, and unresolved parent identities; real-browser verification also caught and corrected horizontal scrolling caused by long parent options.

The source change adds no installed binary, version stamp, release, or managed-project update. Disposable servers, browser tabs, projects, and smoke executables were removed.

Activity-phase filtering, an Unowned owner option, and CLI `ls --facets` remain deliberate follow-ups, not part of this change. The September 29 design and implementation plans are retired into this record and the maintained console spec.

## Local 0.15.3 distribution

**October 5, authorized local distribution.** Commit `9f880f25cfce4f8f31efa55792db1514d8e2f60b` records opt-in console network serving, the human's phone/VPN acceptance, plan retirement, and the **0.15.3** stamp. Schema remains **7**, orientation generation **41**, and the bundle contains ten embedded skills.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the source commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.3-9f880f25cfce/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `a2a8fb27e4e57d7e7d17e663394fb9a0528a206d12e24bc46841433d5d713ee2`. The verified outgoing 0.15.2 executable is retained at `~/.local/share/tiller/previous/0.15.2-c992fe14fe87a5e3a5a399ac659b93490fd90ec1b94170b5921a29342241f0e8/tiller`.

The version and documentation stamp checks, focused version/upgrade/maintenance/serve tests, uncached Go suite, build, lint, and 15-page documentation-site build passed. The precommit candidate and installed binary both served a disposable populated project. Installed CLI and `/api/version` report **0.15.3**; the specific VPN listener serves console, `/api/tree`, and `/api/status`, and rejects loopback Host spoofing, foreign Origin, foreign Referer, and cross-site fetch metadata. Chromium 150 exercised installed Home, Explore, and epic detail with successful project-data reads, the correct visible version, and no console runtime errors.

No tag, push, publication, managed-project refresh, network-settings change, or restart of an existing user console occurred. Existing consoles must be restarted to use the installed binary. The disposable verification servers, project, browser tab, and detached checkout were removed; the candidate and previous executable remain retained.

## Opt-in console network access

**October 5, source only.** Issue #131, opt-in `tiller serve --host`, adds bare IP binds while preserving the default `127.0.0.1` listener and its request behavior. IPv4 and IPv6 use separate listener families. Wildcards snapshot usable addresses on up interfaces at startup; specific network binds admit only their bound IP. One immutable policy supplies request admission and advertised destinations. There is no DNS resolution, proxy trust, authentication, TLS, or remote state sync.

Every route retains strict Host, Origin, and supplied fetch-metadata checks before project, asset, or visit reads. Network modes also validate a present Referer and send same-origin framing protection, preserving existing CSP policies through an additional intersecting policy. A headerless request still reads the console; browser framing refusal does not prevent that request. Plain HTTP network serving does not restore every localhost protection and is only for trusted networks or private VPNs.

The uncached Go suite, focused serve and CLI regressions, focused race checks, build, lint, and 15-page documentation-site build passed. On darwin/arm64, disposable populated-project sockets served console, embedded assets, and project APIs through IPv4 wildcard and specific LAN binds, IPv6 loopback and wildcard, and the IPv6 LAN address. Opposite-family connections to wildcard listeners refused. Invalid-host and held-port CLI scenarios returned the specified reasons and exit codes. The `127.0.0.2` socket test skipped because the OS could not bind it; its deterministic policy coverage passed.

Chromium 150 rendered Home, Explore, and node detail through both IPv4 LAN bind modes with successful project-data reads and no console runtime errors. The actual HTTP LAN requests omitted fetch metadata. A different LAN-port origin's iframe with a foreign Referer received 403; the no-referrer iframe received 200 but rendered a browser error frame with `ERR_BLOCKED_BY_RESPONSE`. Direct navigation and same-origin UI reads continued working.

The human confirmed phone access at `http://100.90.255.120:4400` for wildcard and specific VPN binds after instructions to exercise console navigation and `/api/tree`, then authorized acceptance closure and plan retirement. The phone browser was not specified. The LAN address did not respond remotely; inspection found exit-node LAN access disabled and LAN-subnet routing through the VPN. No network settings were changed. Agent-observed local checks and human-reported phone/VPN results remain distinct; this does not establish remote LAN reachability. The completed `docs/plans/2026-10-05-serve-host-flag-implementation.md` is retired, with its durable decisions and evidence preserved here and in the maintained specifications.

## Prelaunch source hardening

**October 4, source only.** Issue #86, prelaunch safety and public distribution, adds descriptor-relative repository reads, mutations, cache, lock, and journal operations. State consumers share anchored helpers, including directory inventories and explicit event reads. Outside decision links refuse without exposing their contents; outside cache links refuse without changing external bytes or modes. The selected checkout and its ancestors remain trusted. This does not sandbox configured commands or prevent hard-link/mount manipulation.

The console rejects foreign Host, Origin, and cross-site Fetch Metadata before confidential reads or visit writes. Supported loopback names and same-origin requests still work; non-browser clients may omit browser headers. This is a request boundary, not authentication against local processes, a supported reverse proxy, or proof of a browser DNS-rebinding exploit.

Go archives and embedded console assets carry complete upstream notices and font licenses. The website's Pagefind WASM includes GPL-covered code, so the site distributes a versioned corresponding-source archive, full licenses, and working notices/source links. Tiller's independent code and console remain MIT. The 31,894,279-byte source archive has SHA-256 `3f6a900217013edd46dacd44b259dedeb0438b727814b30e17248dafb2127233`; all 5,342 inventoried files verified, and offline locked Cargo metadata resolved the WASM dependency closure. A full WASM rebuild was not performed. The packet discloses the unavailable dirty release-tree provenance and unpinned wasm-opt; it does not claim byte-identical reproduction.

Console CI selects the same `go.mod` toolchain as the binary build because generated runtime notices include that compiler's version. Docs CI watches the notice generator, vendored license/source inventory, and shared font inputs as well as site content. Actionlint validated both workflow configurations; hosted launch-revision execution remains a separate gate.

The uncached Go suite, lint, build, focused store/serve/events race checks, 1,218 console tests, nine notice-generator regressions, console build, and 15-page site build passed. Both complete npm audits and govulncheck reported no vulnerabilities. Real published-v0.11.3 and current-source scratch lifecycles exercised human feature/task gates, local Git merges, sync rollups, and Chromium consoles; old state also migrated from schema 4 to 7 and generation 32 to 41. Browser checks exercised source/release guides, Pagefind search, notices, and corresponding source without runtime errors. The final native packaged console served exact notices/fonts and rejected foreign contexts with Node absent from PATH; this is not clean-machine installation proof.

All four final GoReleaser snapshot archives passed checksum, architecture, contents, and notice checks. Linux executables have no interpreter or dynamic segment. Snapshot checksums are darwin/amd64 `b71bb8f8dc8ceb19d47f5bc1c80c13335747de6c6ebee636cd256cb7527c4dff`, darwin/arm64 `3f553c4d089ecba089a28f570b9c44534da08c5f19a1793c8a23db866828d445`, linux/amd64 `18aadc6919a64edd040ce0d9b0c71a43f84732c495cf57d9dd0e03691327271c`, and linux/arm64 `b86ac7ebf12b40dba42b8cf566c75b25c6719bd72c1897268c78989c3fe6d02d`. These are disposable working-tree candidates, not published release artifacts or clean-revision CI evidence.

Current-tree privacy and secret scans passed; reachable-history gitleaks scanned 1,224 commits without findings. An additional scan of unpacked upstream source reported 55 findings, all independently classified as 50 verified SHA-256 checksums and five public Windows integer constants. No source rewrite or broad allowlist suppresses them. All 40 hosted heads/tags were reviewed under the keep-history decision. Five active dependency-update branches remain; none was pruned.

The maintainer chose to retain zero required approving reviews and admin bypass while disabling GitHub Projects. Conduct stays inline in CONTRIBUTING.md; wiki and Discussions remain off. No visibility change, publication, installation, managed-project change, history rewrite, or active-branch deletion occurred. The September 29 installed 0.15.1 binary does not acquire these source fixes from sharing its version stamp. Launch-revision hosted CI, human publication, clean/anonymous installation, installed-launch-release acceptance, public security settings, and signed-off contributor entry remain external gates.

A review of the hardening commit before it merged kept two hard-to-reverse changes out of branch history. The Pagefind corresponding-source archive left git for a release asset under an orphan tag, which keeps it invisible to GoReleaser and to `tiller upgrade`. Each site build now fetches the archive and verifies it against the SHA-256 in the versioned manifest, and Pages still serves it beside the WASM. The anonymity guard now stores digests rather than split strings. The same review added CI checks for every generated notice copy and for docs that repeat the version stamp, pinned the notice generator's Go toolchain and every workflow action, narrowed repo-root discovery to symlink refusals, made confined walks honor `fs.WalkDirFunc` skips, and labeled the path-sniffing confinement helper a transition shim. A follow-up removed that shim: every repository read and write now names its checkout root, so a checkout under an ancestor directory named `.tiller` loads, and gate and rubric evidence under `artifacts/` is confined like state.

## Current-tree anonymity

The trial projects use **Acme** for the platform and **Lantern** for the app. The settled issue #84 decision is to clean maintained files while retaining Git history and closed issues, with no history rewrite. Neutral project shapes and public domain/form identifiers remain useful examples, not new privacy findings.

The prelaunch recurrence sweep removes private repository links, checkout paths, contributor names, and engagement-specific Git coordinates rather than inventing public stand-in repositories. CI's `scripts/check-client-privacy.py` checks maintained file names and text against the scoped identifying-name denylist, including newly added nonignored files. It does not scan Git objects or ban accepted public nouns. The obsolete process diagram is retired; the [release Quickstart](../site/src/content/docs/quickstart.md) matches v0.11.3 and the [source walkthrough](../site/src/content/docs/source-quickstart.md) covers current source.


## Local 0.15.1 distribution

**September 29, authorized local distribution.** Commit `56ebeed63241a8580f48b0cff811c24945a20029` stamps **0.15.1** and commits generated-board retirement. The binary uses schema **7**, orientation generation **41**, and ten embedded skills.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.1-g2s6sqjt/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `d53b64a752187a9fba571a18db2c17052244c893b43d09ae0fc78929e9ff742d`. The outgoing executable is verified and retained at `~/.local/share/tiller/previous/0.15.0-3c68e0d37b92c5ff6517e469f8d5ab1b87c4dadd1ba5fb848ab3a7128b24c016/tiller`.

The stamp check, version and maintenance tests, lint, clean distribution build, and 14-page documentation-site build passed. The preceding implementation verification includes the uncached Go suite, rebuilt embedded assets, and 106 affected console tests. Candidate and installed CLI, installed version API, and Chromium Home/Explore report **0.15.1** without browser runtime errors.

A disposable project created with the outgoing binary refreshed orientation **40 → 41** without a schema migration, preserving all source YAML, tracked legacy-board bytes, and raw Git index bytes. Repeat maintenance through the installed binary preserved all 64 snapshotted files. A fresh installed initialization emitted generation 41 without creating a board.

No tag, push, publication, managed-project refresh, Acme change, or restart of an existing user console occurred. Adopting checkouts run `tiller upgrade --project-only`, review and commit the generated instruction change, and reload agent instructions. Restart consoles to load the installed binary. Legacy-board deletion remains a separate ordinary Git action, after preserving unexpected hand edits, as described in [adoption guidance](spec/distribution.md#generated-board-retirement).

## Generated Markdown board retirement

**September 29, source only.** The generated `.tiller/board.md` is retired rather than retained as an ignored local projection. The earlier source-only ignore change had preserved generation and repair; this cutover removes the renderer, every mutation refresh, the otherwise-empty `tiller gen` command group, sync's `board` result, and board-only mutation warnings and remedy. No replacement report, cache, compatibility command, or background service was introduced.

Shared eligibility, execution-state, question, ordering, and workflow policy remains behind the live CLI and console. `/api/board` still supplies Home's structured streams. Source/event formats and schema **7** are unchanged; orientation generation **41** and the embedded guides remove file-reading, regeneration, and staging instructions. The version stamp remains **0.15.0**, without changing the installed binary.

The corpus boundary excludes only the retired root coordinate and its obstruction descendants before opening or validating them. Stale contents, invalid UTF-8, NUL bytes, unreadable files, directory descendants, outside or broken symlinks, and FIFOs cannot contribute search hits or generated mentions or break reads. Authored `.tiller/design/board.md` remains searchable and citable, and unsafe authored prose or orientation still refuses. Explicit historical citations remain unresolved with provenance intact, no granted destination, and ordinary diagnostics. Spec source labels and backlink diagnostics keep unavailable citations copyable rather than creating broken links.

The uncached Go suite, lint, build, 106 affected console tests, TypeScript and embedded asset build, 14-page documentation-site build, and screenshot fixture safety/source-commit checks passed. The affected UI regressions failed before the navigation corrections and passed afterward. Disposable real-Git projects exercised init, owner registration, join, planning, claims, start, checks, submission, a fixture-controlled human merge, and sync. The observed crossings were task ancestry, feature rollup, then epic rollup; source events and ownership were retained without creating a board. Separate tracked and ignored legacy projects retained both board bytes and raw Git index bytes through maintenance, join, mutation, sync, and reads.

Chromium exercised Home's live streams, Build, Explore search/map, relationship queries, source and backlink diagnostics, and direct retired-file not-found responses with the board absent and with a malformed directory obstruction. Adding and answering a question changed live Home state after refresh without changing that obstruction. A separate invalid UTF-8 authored file still refused search; the degraded node page kept its retired source copyable without a file link. Browser runtime errors were absent; Safari and Firefox were not exercised. A controlled agent consumed freshly generated orientation and the embedded orient guide, obtained status and its next action from bounded CLI reads, and stopped at the feature-plan human gate without requesting a board or creating a substitute file.

Tiller neither deletes legacy files nor changes their Git tracking, and it stops seeding the board ignore rule without purging historical ignore entries. [Existing-project adoption](spec/distribution.md#generated-board-retirement) requires a separately deployed binary, checkout instruction refresh, and deliberate ordinary Git cleanup after preserving unexpected hand edits. No installation, source commit, publication, managed-project refresh, or Acme change occurred. This completes `docs/plans/2026-09-29-remove-generated-board.md`; the durable outcome above replaces its execution record.

## Local 0.15.0 distribution

**September 29, authorized local distribution.** Commit `a17c6974eab71266d53859802fe59b8df54b409b` stamps **0.15.0** and commits planning flexibility. The binary uses schema **7**, orientation generation **40**, and ten embedded skills.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.15.0-qldpxvy3/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `3c68e0d37b92c5ff6517e469f8d5ab1b87c4dadd1ba5fb848ab3a7128b24c016`. The outgoing executable is verified and retained at `~/.local/share/tiller/previous/0.14.5-cf7bf285ae9d8cf28c04fa4b79a2faaca8828771ad306be67edecdec168bc4cd/tiller`.

The stamp check, version and maintenance tests, uncached Go suite, lint, clean distribution build, and 14-page documentation-site build passed. The implementation verification below includes rebuilt embedded assets and 78 affected console tests. Candidate and installed CLI, installed version API, and Chromium console report **0.15.0**. Browser verification covered Home, Explore, and migrated task detail, retaining its owner, branch, state, and accepted basis without runtime errors.

A disposable project created by the outgoing schema-6 binary contained active work and two registered developers. Migration dry-run preserved every file. The first checkout ran the candidate's offline `upgrade --project-only`, committed the schema and generated-artifact changes, and the second checkout pulled that commit and refreshed locally. Both preserved all work-node bytes and their distinct gitignored checkout identities. A further refresh with the clean distribution preserved all 136 snapshotted fixture files. Schema 6 → 7 needs no active-work drain; this does not relax older migration requirements.

No tag, push, publication, Acme migration, managed-project refresh, or restart of an existing user console occurred. Teammates need a matching-platform 0.15.0 binary before using migrated state; the public release channels do not yet provide this local build. One person commits the shared migration, others incorporate it through normal Git workflow, and each checkout runs `tiller upgrade --project-only`. Existing identity requires no new join. Reload agent instructions and restart consoles after refresh.

## Planning flexibility without rewriting execution history

**September 29, source only.** Next-work selection now filters eligibility before applying recorded epic, feature, and task preference. Historical dependency depth remains a rendering fact, not a live barrier after prerequisites finish. Existing routing classes, checked-work priority, shelf precedence, ownership, and execution guards remain authoritative. Guide, full and correction readouts, board/pool presentation, and console routing share the preference decision and retain skipped-question explanations.

Generated structural guidance begins with installed command capabilities, lifecycle and descendant history, review branches, parents, prerequisites, question holds, and inherited review settings. Three controlled agents used candidate-generated instructions in separate disposable projects. One staged three identity-preserving moves and stopped at approval. Another identified a started descendant and made no project-state changes. The third distinguished genuinely abandoned billing scope from relocation and proposed a new CSV feature with cancellation and replacement lineage. No prompt supplied the expected correction route.

Schema **7** adds the reviewed `move-feature` amendment operation; orientation generation **40** advertises it. Draft, specced, and ready features can relocate only when the entire subtree has never started and the move preserves effective prerequisites and question holds. Effective review grain survives through an explicit override when necessary, including `epic` on a feature. Approval atomically updates addresses, current machine locators, and parent order while retaining IDs, claims, states, authored prose, historical revision paths, and acceptance hashes. Authored citations and bindings needing review are warnings, not global text replacements. Validation preimages include file bytes, modes, and directory membership, so planning cannot silently accept a later filesystem edit. Schema 6 migration preserves existing work and approved amendment history; older binaries refuse the new schema.

Cycle diagnostics separate actual prerequisite and completion obligations from projected epic grouping. Explicit epic dependencies and aggregate-review completion can make a feature chain genuinely cyclic. Independent early task or feature review can make the same interleaving legal. Shared classification supplies readiness, readouts, move validation, and console diagnostics; grouping-only cycles remain visible without failing readiness or asking the developer to delete valid edges.

The uncached Go suite, build, lint, 78 affected console tests, embedded asset build, and 14-page documentation-site build passed. Real CLI scenarios preserved three feature/task identities, a ready-task claim, dependencies, prose bytes, and original acceptance pins across one approved batch. Unchanged moved work started without reapproval; existing prose drift still blocked start. Other scenarios covered an earlier staged destination epic, explicit epic review under a task-grain destination, stale-claim refusal without writes, old-binary refusal, repeated approval refusal, and source rollup during ordinary sync. Historical accepted Git blobs remained readable. Chromium inspected proposed and stale moves, moved-node and historical-parent links, preferred routing, and the warning/blocker distinction.

An interleaved early-office/domain/later-office fixture passed readiness before and after the early feature's real local merge, then routed across epics to the domain task. Explicit whole-epic prerequisites and aggregate-review obligations failed readiness. Classification reads preserved project and Git bytes. Correction readouts preserved source, refs, and staged content; their existing Git read refreshed index stat metadata. Failure-injection and preimage regressions cover rollback and filesystem races.

No installed executable, version stamp, Acme plan, managed project, or published release changed during implementation. This completes `docs/plans/2026-09-29-planning-flexibility.md`; the durable decisions above replace its retired execution record.

## Local 0.14.5 distribution

**September 28, authorized local distribution.** Commit `1e7c868ac5f38586f6a81c180927eed37d047095` stamps **0.14.5** and includes the explicit logo theme assets below. Schema **6**, orientation generation **39**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the stamp commit and `vcs.modified=false`. The candidate at `~/.local/share/tiller/distributions/0.14.5-agn9i92t/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `cf7bf285ae9d8cf28c04fa4b79a2faaca8828771ad306be67edecdec168bc4cd`. The verified outgoing executable is retained at `~/.local/share/tiller/previous/0.14.4-0b3f469565195e8eee7529e1e5abf2d5c2a4e0c6139fd9dd23c9c40daf77da36/tiller`.

The stamp check, uncached full Go suite, lint, distribution build, and 14-page documentation-site build passed. The embedded assets were rebuilt and browser-verified during the immediately preceding logo correction. Candidate and installed CLI, version API, and rendered console report **0.14.5**. Chromium verified all six OS/mode combinations on the stamped candidate, then confirmed the installed dark-mode asset overrides a light OS preference without runtime errors. Candidate and installed project-only maintenance preserved all 22 scratch fixture files created by 0.14.4. The console unit suite, Safari, and Firefox were not rerun for this distribution.

No tag, push, publication, managed-project refresh, or restart of an existing console occurred. Logo concepts under `output/` remain uncommitted. Current 0.14.4 checkouts need no schema or orientation refresh; restart existing consoles to load the correction.

## Local 0.14.4 distribution

**September 28, authorized local distribution.** Commit `1dacdf6626ddd0a38c566eb6514c0f287c544ac8` stamps **0.14.4** and includes the official logo integration below. Schema **6**, orientation generation **39**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records the stamp commit and `vcs.modified=false`. The candidate at `~/.local/share/tiller/distributions/0.14.4-2ksfgmz1/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `0b3f469565195e8eee7529e1e5abf2d5c2a4e0c6139fd9dd23c9c40daf77da36`. The verified outgoing executable is retained at `~/.local/share/tiller/previous/0.14.3-8d37b2680dbd7e63994abd0dffcb124ffb81f71411eedffcb0f2d09a0c947545/tiller`.

The stamp check, uncached full Go suite, lint, distribution build, embedded asset build, and 14-page documentation-site build passed. Candidate and installed CLI, version API, and rendered console report **0.14.4**. Chromium verified the logo, home-link navigation, light desktop and dark 390px layouts without horizontal overflow or runtime errors, then checked the installed console. Candidate and installed project-only maintenance preserved all 25 scratch fixture files created by 0.14.3. Browser automation covered Chromium, not Safari or Firefox; the console unit suite was not rerun for this distribution.

No tag, push, publication, managed-project refresh, or restart of an existing console occurred. Logo concepts under `output/` remain uncommitted. Current 0.14.3 checkouts need no schema or orientation refresh; restart existing consoles to load the new executable.

## Explicit logo theme assets

**September 28, source only.** The developer reported that the logo was invisible in dark mode after local 0.14.4 distribution. The earlier Chromium checks did not establish reliable rendering in the affected environment. The correction removes the SVG's internal media query and supplies `tiller-logo-inverted.svg` with fixed light-ink fills and identical paths. Console selectors use the existing root theme attribute, with OS preference only in Auto. Starlight uses its light/dark logo configuration, and the README uses a picture source. No new theme state or listener is introduced.

The embedded asset build, real Go binary build, and 14-page documentation-site build passed. Chromium exercised all six combinations of light/dark OS preference and Auto/Light/Dark selection on the console and documentation site. Exactly one logo was visible in each case, with the expected asset. Console checks also covered live OS changes in Auto, persisted dark mode after reload, and 390px rendering, without runtime errors or horizontal overflow. SVG geometry comparison confirmed all four paths and transforms were preserved. Hosted README rendering and Safari/Firefox remain unverified. No commit, installation, version stamp, or publication changed.

## Official logo integration

**September 28, source only.** The supplied root `tiller-logo.svg` is the canonical boat-and-wave artwork for the README, documentation header and homepage, and compact console home link. Astro and Vite bundle that source directly rather than maintaining copied SVGs. Its paths and proportions are preserved; a dark-scheme color override uses the existing light heading ink. The console keeps its visible wordmark, version, and accessible home-link name, with header wrapping to accommodate the mark on narrow screens.

The embedded console assets and real Go binary built successfully, as did the 14-page documentation site. Chromium verification covered light/dark artwork, manual theme choices opposite the OS preference, desktop and 390px layouts, documentation subpath assets, home navigation, and a local rendered README preview. The console and docs had no browser runtime errors or horizontal overflow in the exercised layouts. Existing screenshots exclude the topbar and remain unchanged. Hosted GitHub README rendering and Safari/Firefox were not exercised. No version stamp, installed executable, commit, or publication changed.

## Local 0.14.3 distribution

**September 28, authorized local distribution.** Commit `b6d2f1403ed18b5bec047ea326b6fecacf17a9c3` stamps **0.14.3** and includes the cancelled-work readiness correction below. Schema **6**, orientation generation **39**, and embedded assets are unchanged.

The darwin/arm64 executable was built from a clean detached checkout with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp commit and `vcs.modified=false`. The candidate at `~/.local/share/tiller/distributions/0.14.3-w6tsl74k/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `8d37b2680dbd7e63994abd0dffcb124ffb81f71411eedffcb0f2d09a0c947545`. The outgoing patched 0.14.2 executable is retained at `~/.local/share/tiller/previous/0.14.2-b28940909aa4528d8621152a6e9faee60c184b8db5c675fb9a1f4ace787d303e/tiller`.

The stamp check, uncached full Go suite, lint, build, and stamped CLI smoke checks passed. Candidate and installed project-only maintenance preserved all 44 fixture files created by the outgoing executable. Candidate and installed CLI, version API, and Chromium console report **0.14.3**; browser verification covered Home, Explore, and node detail with no runtime errors. Installed Acme readiness reports `ready: true` with no gaps, and orient reports readiness passed without the historical-cycle warning. All 1,902 Acme state files were preserved across the bounded installed orient read. A concurrent edit to the product-replan Markdown prevents claiming whole-release-window preservation of that file; every other snapshotted Acme file remained unchanged, and this distribution wrote no Acme files.

No tag, push, publication, managed-project refresh, or restart of an existing console occurred. Current checkouts need no schema or orientation refresh. Restart existing consoles to load 0.14.3.

## Cancelled-work readiness diagnostics

**September 28.** Readiness and orient/decompose cycle diagnostics now recompute the dependency graph without cancelled work or descendants of cancelled containers. Historical dependency ordering and relationship reads retain every recorded edge. Done and archived work keep their previous treatment. Legacy keys still resolve against the complete snapshot, so excluding a cancelled record cannot make an ambiguous reference valid.

The live Acme replan exposed an implied epic cycle formed entirely by cancelled predecessor features, including features under the retained Phase Submission epic. Its feature graph was acyclic. Readiness had mistaken historical ordering fallback for a current planning defect and told the developer to break the loop. Cancellation now removes those edges from the diagnostic projection without rewriting their history; a remaining non-cancelled cycle still fails readiness.

New readiness and readout regressions failed before the fix and passed afterward. The readiness, gen, and views package suites passed. A real CLI scratch project exercised an active implied epic cycle, feature cancellation inside a retained epic, container cancellation, a remaining live feature cycle, and cancellation of its prerequisite. Historical `show` dependencies remained visible. The candidate CLI reported readiness passed with no gaps on live Acme, preserving all 1,902 snapshotted `.tiller/` files.

The uncached full Go suite, lint, and build passed. The corrected working-tree build is installed at `~/.local/bin/tiller` under the unchanged **0.14.2** stamp, with schema **6** and orientation **39** unchanged. Its retained candidate is `~/.local/share/tiller/distributions/0.14.2-readiness-b28940909aa4/tiller`, SHA-256 `b28940909aa4528d8621152a6e9faee60c184b8db5c675fb9a1f4ace787d303e`. The outgoing clean 0.14.2 executable is retained at `~/.local/share/tiller/previous/0.14.2-52a7b164545e93bbafa84b0b38c7c8c4ea8c95c97c9d7af3605cf2eed363daa6/tiller`. Installed Acme readiness reports `ready: true` with no gaps, and orient reports readiness passed without a cycle warning. All 1,902 snapshotted files remain unchanged. No commit, tag, push, publication, managed-project refresh, or existing-console restart occurred.

## Local 0.14.2 distribution

**September 28, authorized local distribution.** Commit `93db6686f4547ec455d5698f23933137be1ffe99` stamps **0.14.2** and excludes cancelled epics and their descendants from Explore's browsing tree, map containment, and Build. Filters, search, direct detail, recorded relationships, and CLI tree history remain available. Schema **6**, orientation generation **39**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached worktree with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp commit and `vcs.modified=false`. The candidate at `~/.local/share/tiller/distributions/0.14.2-s4mu7if8/tiller` and atomic installation at `~/.local/bin/tiller` share SHA-256 `52a7b164545e93bbafa84b0b38c7c8c4ea8c95c97c9d7af3605cf2eed363daa6`. The outgoing executable was verified and retained at `~/.local/share/tiller/previous/0.14.1-cadf8dcb2a70664d2078a9869ee0b5edc63733dd212fa42199e4ac743a7dbb4d/tiller`.

The stamp check, uncached full Go suite, lint, and all **1,213 console tests** passed. The first full Go invocation reached its command deadline; the completed run passed with a longer deadline. Embedded assets were rebuilt for the fix. Candidate API checks verified Browse, Map, and Build exclusion plus cancelled filter/search and direct detail access. Installed CLI, version API, and Chromium console report **0.14.2**, and the installed map renders only the current subtree without browser runtime errors. Candidate and installed project-only maintenance preserved all 35 snapshotted fixture files created by 0.14.1.

No tag, push, publication, managed-project refresh, or restart of an existing console occurred. Restart existing consoles to load 0.14.2. Checkouts already current at 0.14.1 need no schema or orientation update.

## Local 0.14.1 distribution

**September 28, authorized local distribution.** Commit `b5fef1362ff45392414b7b4f00f0378fc6e67ab4` stamps **0.14.1** and includes the relationship-map review corrections below. Schema **6**, orientation generation **39**, and ten embedded skills are unchanged.

The darwin/arm64 executable was built from a clean detached worktree with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp commit and `vcs.modified=false`. The candidate at `~/.local/share/tiller/distributions/0.14.1-20260928-bvbuyuw5/tiller` and the atomic installation at `~/.local/bin/tiller` share SHA-256 `cadf8dcb2a70664d2078a9869ee0b5edc63733dd212fa42199e4ac743a7dbb4d`. The outgoing executable was verified and retained at `~/.local/share/tiller/previous/0.14.0-5f88f11d9c30d7436f0c2cfc19424bb2e774c26b02bbdb9e4a9e09909cc6fb74/tiller`. The candidate directory includes `verification.json` with build, checksum, and maintenance evidence.

The stamp check, Go build, lint, uncached full Go suite, all **1,213 console tests**, TypeScript, embedded assets, and the 14-page documentation build passed. The console chunk-size and missing site 404-content warnings remain. Candidate and installed CLI, version API, and Chromium console reported **0.14.1**. Browser checks exercised mapped filtering and restoration, refresh reset notices, and installed selected-scope fallback, with no browser runtime errors recorded. Candidate and installed project-only maintenance preserved all 28 snapshotted authored, generated, and harness files in a disposable schema-6, generation-39 fixture created by 0.14.0.

No tag, push, publication, managed-project refresh, or restart of an existing console was performed. Restart existing consoles to load 0.14.1. This patch requires no schema or orientation update for checkouts already current at 0.14.0.

## v0.14.0 review corrections

**September 28, source only.** Assessed the ten findings in the v0.14.0 code review and corrected eight. Successful map refresh now explicitly clears both investigation contexts and shows a reset notice; failed refresh retains them. Toolbar and Map key kind filters narrow the mapped result page rather than discarding it, with one type-checked Work links kind set shared by map and inspector. Full graph and All dependencies remain explicit project-scope transitions. Losing selection resets selected-item browsing to Whole project.

Relationship context merging now lives outside the containment view, so mapping, filtering, and clearing results do not invalidate tree layout or reopen collapsed selection ancestors. Diagnostic identity is defined once in that merge, preserving the existing extraction-impact rule without expanding the server API. Project/query reference issues skip selected-item scoping. Relationship styles moved into the colocated `relationships.css`; base `map.css` is 621 lines.

The proposed replacement of deleted prose tests was rejected. The deletions in the current history belong to `600880b` (v0.14.0 stamp), not the plan-retirement commit named in the review. They include paragraph boundaries, phrase presence, example paths, and command-count assertions. These are not execution coverage. Existing mutation, store, and CLI tests exercise lifecycle eligibility, no-write refusals, unsupported amendment operations, and the public `not_found` discriminator. The two pagers also remain separate: 25 relationship rows with scroll reset and 20 issue occurrences have different presentation contracts; their short slice/clamp logic does not warrant a shared abstraction.

All **1,213 console tests**, TypeScript, rebuilt embedded assets, and the real binary build passed. Focused uncached mutation, CLI, and store tests passed for dependency-change guards, unsupported operations, amendment validation, and refusal classification. Chromium exercised the rebuilt embedded console against a disposable CLI-created project: mapped-page kind filtering and restoration, Work links, successful refresh reset, controlled failed-refresh retention, query mapping, collapsed-ancestor preservation after Clear, selected-scope fallback, and explicit global transitions. Desktop light/dark and 390px dark views retained the scrollable investigation panel and evidence disclosure; no browser runtime errors were recorded. The existing narrow shared-topbar overflow remains unchanged. No installed binary, schema, generation, version stamp, or managed project changed. The completed review record is retired here.

## Local 0.14.0 distribution

**September 28, authorized local distribution.** Commit `600880b2a6c463a7cd9cc019efab2c2271d9e626` stamps **0.14.0** and commits product-driven planning together with the previously deployed relationship-map redesign. Schema stays **6**, orientation advances to **39**, and ten skills remain embedded. Current source-version declarations were aligned without changing historical release evidence.

The darwin/arm64 executable was built from a clean detached worktree with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records that commit and `vcs.modified=false`. The retained candidate at `~/.local/share/tiller/distributions/0.14.0-20260928-xwpxfa9o/tiller` and the atomic installation at `~/.local/bin/tiller` share SHA-256 `5f88f11d9c30d7436f0c2cfc19424bb2e774c26b02bbdb9e4a9e09909cc6fb74`. The outgoing 0.13.1 executable was checksum-verified and retained at `~/.local/share/tiller/previous/0.13.1-940c05982ebe4a68ec70ddfec5cfd911cd8983626955ee956ca9b710df5d8589/tiller`.

The stamp check, Go build, vet, uncached Go suite, TypeScript, embedded asset rebuild, all **1,213 console tests**, and the 14-page documentation build passed. Build warnings remained for the console chunk size and the site's missing 404 content entry. A disposable project initialized by the outgoing executable refreshed from generation 38 to 39 with only `.tiller/orientation.md` changed; repeat maintenance changed nothing. All four planning guides agreed between text and JSON delivery. Chromium exercised the embedded relationship browser and displayed export depending on import. The candidate and installed CLI, version API, and rendered console reported 0.14.0; neither browser session reported errors. Build metadata, command transcripts, checksum evidence, and a candidate screenshot are retained beside the distribution binary.

No tag, push, publication, managed-project refresh, or restart of an existing console was performed. Existing consoles need a restart to load the new binary. Desired checkouts adopt generation 39 through `tiller upgrade --project-only`, followed by reloading agent instructions; installation alone does not rewrite their plans or state.

## Product-driven planning

**September 28, source only.** Planning now starts with documented consumers and outcomes across the full known initial scope. Epics and features are bounded delivery increments, not permanent subsystem buckets. Shared implementation serves its consuming increment; known later capabilities stay in later features, with detailed contracts and tasks just in time. Real prerequisites remain hard edges, preferred sibling sequence remains soft order, and concrete compatibility or migration risk can justify bounded technical work. No schema, command, gate, routing, or ordering algorithm changed.

The decompose, specify, amend, and intake guides carry the same principle with stage-specific actions. Decomposition retains whole-tree and scoped-lane dependency audits without an edge-count quality threshold. Specification keeps tasks inside the selected increment. Incoming work extends an existing bounded outcome or forms a distinct later increment, preserving completed history and unrelated accepted scope. Amendment cannot link, unlink, order, or reparent; revise re-pins prose without unlocking started work. Genuine replacement is not a device for evading those limits. The existing active-edge refusal's amendment hint remains incomplete and is documented as such.

Orientation advances from 38 to **39**, retaining schema **6**, ten skills, thin shims, and the **0.13.1** version stamp. Canonical specs, hand-authored site pages, correction guidance, and the root planning principle agree with the served guidance. Dependency-count comments are neutral diagnostics. Affected paragraph, example, and phrase assertions were deleted rather than replaced with a new wording rubric.

Disposable inputs and an outcome rubric were prepared before instruction edits. Ten fresh agents consumed the candidate's generated orientation and emitted guides through both named-guide and generated-shim discovery. Independent review inspected their authored files, state, transcripts, and handoffs:

| Scenario | Observed result |
|---|---|
| Research archive, full initial scope | Retained local import/export, authenticated tenant APIs, scheduled ingestion, revocable sharing, and reporting. A small signed-envelope compatibility prerequisite precedes persistence; scheduler/retry machinery does not block local exchange. |
| Independent utilities, two runs | Kept calculator work independent while recording the real later audit-to-scrub prerequisite. Neither run invented a common platform. |
| Fully independent offline tools, two runs | Both complete outcomes remained covered with zero dependency edges, ordinary soft order, and no invented prerequisite to satisfy a count. |
| Architecture-heavy plugin SDK | Grounded outcomes in plugin authors, not screens. Recorded the consequential trusted-versus-untrusted execution question and held execution rather than assuming safety; validation remained useful independently. |
| Selected CSV-import specification | Produced four bound criteria and three proposed tasks with real validation, persistence, and command prerequisites. Kept checkpoints and delivery deduplication in later ingestion work, surfaced unresolved signed-byte transport, and preserved all 26 other work-node files. Test-runner bindings were explicitly future implementation work, not claimed checks. |
| Focused continuation, two runs | Put TSV continuation beneath the done export outcome and regulated reporting in a distinct later increment. Preserved existing work-node bytes and order; rejected active-edge edits and reparenting without artificial replacement or approval. |
| Sourced intake | Covered all four findings with five legally staged, honestly cited operations. Kept the unresolved retention period as a question and separated real audit prerequisites from explicit post-materialization placement. |

**Question-restraint refinement.** A later independent review found variation in the original utility pair: one run created three blocking questions for contract details despite already-supported boundaries and order, while the other deferred those details to specification. The decompose readiness and gap instructions now require a named boundary, prerequisite, or delivery consequence before making a missing detail a planning blocker. The owning spec matches, and the old question-command-count assertion was removed rather than re-pinned. Two additional fresh utility runs on unchanged inputs both recorded zero questions while preserving all scope, calculator independence, and the real audit-to-scrub prerequisite. A fresh SDK run still recorded the consequential initial trust question and a catalog-authority question, with targeted holds. This brings the observed sample to thirteen runs, not a guarantee of uniform model behavior.

The refined SDK run also recorded incorrect source locators and disclosed unresolved provenance diagnostics despite a complete dependency query. Its product decisions were inspectable against the source it read, but that graph was not diagnostically clean; no hand-edited state or hidden repair was used to make the sample pass.

All planning agents stopped before approval. Fixture-driver human gates and a real local Git merge were recorded separately. The candidate then materialized an agent-authored intake in a separate clone, including same-batch epic/feature parenting, a proposed task beneath a done feature, and a fresh sibling dependency beneath active work. Approval appended the new epic; an explicit order command moved it to the recommended position. Additions left stored parents done until sync reopened the feature to active and epic to ready, cleared current completion fields, and staled feature gates while preserving the completed task and accepted prose. Active-feature link and started-task key-move refusals remained unchanged.

The real binary served matching text/JSON guides for all four planning seams and initialized generation-39 orientation with ten shims. Project-only maintenance of a schema-6, generation-38 fixture changed only orientation; authored contracts, ownership, and task history were unchanged. Repeating maintenance changed no file content. Orientation fixture regeneration, `make build`, `make lint`, uncached `make test`, and the 14-page documentation build passed. The site retains its missing-404-content warning. Independent static and behavioral reviews found no outstanding defects.

These are controlled planning observations, not application implementation, exhaustive noun/state refusal coverage, or universal model compliance. The SDK readout also showed the existing dependency-order override of a preferred soft sequence; the algorithm is unchanged. The continuation smoke used task-grain review, not a new coarse-review round. No installed binary, version stamp, release, Acme state, or other managed project changed. Adoption requires a later authorized binary installation, checkout maintenance, and instruction reload; it does not replan accepted work. The completed September 28 implementation plan is retired.

## Relationship map investigation

**September 21, source only.** Relationship inspection has three separate jobs: the compact Map key explains and filters visible lines, the existing inspector shows the selected item's relationships, and a bounded investigation panel supports project-wide browsing and reference issues. Depends on and Required by remain separate, with recorded counterpart status rather than inferred blockers. Provenance mounts on demand, preserving source locations, operation roles, criterion and revision evidence. Document coordinates remain metadata by default and stay available through explicit queries.

Project browsing renders 25 relationships per page, with search, direction, kind, scope, and counts. Six engineering questions use the existing read-only query API. Show results on map projects only the current page and clears the prior map kind filter; Full graph remains deliberate and unbounded. Query-derived coordinates remain inspectable across later queries while an earlier page remains mapped. Explicit resubmission performs a new read, including after a transient failure. This bounds rendered results, not the request-local graph payload or extraction cost.

Reference issues distinguish the selected item and its owned documents, project-wide occurrences, and queried subjects. Backend `incomplete` metadata keeps extraction-loss warnings visible without duplicating impact rules in the browser. Missing documents and unavailable citation lines stay copyable, not navigable. Historical ambiguity stays historical, and investigation commands inspect the subject at depth zero rather than pretending there is a generic repair. Catalog entries now match feature-origin preservation and bug/chore supersession; citations accept both `#L3-58` and `#L3-L58` while retaining raw values and confined, positive, ordered coordinates.

All 1,213 console tests, TypeScript, embedded assets, the real binary build, Go lint, and uncached tests for relationships, readinput, views, and store passed. Independent reviews closed the mapped-query identity and same-query retry defects; a regression reproduced and then cleared a mismatched toolbar filter after page projection. Embedded Chromium exercised a disposable project with 1,274 relationships, five prerequisites including one merge-backed done feature, one dependent, shelf work, source documents, unresolved endpoints, missing targets, unsupported syntax, unavailable lines, and incomplete extraction. Checks covered desktop and 390px frames, both themes, short and long titles, keyboard disclosures, source-line navigation, pagination/search, all six CLI query directions, virtual code paths, identical-query recovery after an injected network failure, individual branch collapse, Collapse all, and explicit global overlays. Read scenarios preserved all 181 snapshotted `.tiller/`, Git index, HEAD, and ref files.

No managed project, installed binary, version stamp, schema, orientation generation, or release changed. Safari and Firefox were not exercised. Existing shared-topbar overflow at narrow widths remains outside this map change. The console and query specs and the console guide own the shipped contract; the completed September 21 execution plan is retired.

**Subsequent authorized local deployment, no new release.** Rebuilt embedded assets and the working-tree binary with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`, without changing the 0.13.1 version stamp. The candidate passed scratch-project CLI initialization and work creation, then replaced `~/.local/bin/tiller` atomically. Installed SHA-256 is `940c05982ebe4a68ec70ddfec5cfd911cd8983626955ee956ca9b710df5d8589`; the matching artifact remains at `~/.local/share/tiller/distributions/local-relationship-panel-c2gb8b31/tiller`. The outgoing executable is verified and retained at `~/.local/share/tiller/previous/0.13.1-e205ac6594a9e11f24c83f5deb7bcbdd2c1c7429bf6e1da89c4b74037a89d8a5/tiller`. The installed CLI, version API, and Chromium console report 0.13.1; the console shows the new Map key, selected dependency inspector, and bounded relationship browser. Existing implementation test evidence above was reused; deployment rebuilt TypeScript/assets and the binary and exercised that installed artifact. No commit, tag, published release, managed-project update, or existing console restart occurred.

## Local 0.13.1 distribution

**September 20.** Commit `b5eea2e` makes console person labels resolve through the shared project roster while preserving stable IDs in data, commands, filters, and grouping. Missing roster entries retain a supplied name or fall back to the ID. Stamp commit `a100757aff64d8b837a8b2a2f817489ffc1d528d` sets version 0.13.1; schema remains 6, orientation generation 38, and ten embedded skills remain unchanged.

A clean detached worktree produced the darwin/arm64 executable with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. Build metadata records the stamp revision and `vcs.modified=false`. The installed `~/.local/bin/tiller` matches the verified artifact with SHA-256 `69f8190105c33e4ecc43fa414ce2e93d22fa37abf6b129fdb6a2eb69a1b9557d`. The outgoing executable is verified and retained at `~/.local/share/tiller/previous/0.13.0-f0fcdb30819d991a3510ddc16469916cee44fb2c5b61f27aa0f85df810449aed/tiller`.

All 1,202 console tests, TypeScript and embedded asset builds passed before the implementation commit. The stamped-version and upgrade checks, uncached Go suite, lint, Go vulnerability scan, and console dependency audit passed for the release. The distribution binary built the Acme fixture through real CLI commands; offline project maintenance preserved all 156 scratch `.tiller/` files. Chromium verified the candidate and installed console reporting 0.13.1 with full owner and proposer names. Earlier browser checks covered the broader person-label surfaces, live roster refresh, ID-based filtering, and retired-person fallback.

The binary, build metadata, fixture output, screenshot, and installation evidence remain under `~/.local/share/tiller/distributions/0.13.1-20260920-194917/`. Disposable worktrees and smoke servers were removed. No managed checkout, existing console process, tag, remote, GitHub release, or Homebrew package was changed. Restart existing consoles to load the new executable; this patch needs no schema migration or orientation refresh for current-schema, generation-38 checkouts.

## Corrected 0.13.0 installation and migration review

**September 20.** Commit `e43adef4e20685f0e17684cbc7ad93ed48f9bc16` records the migration guard correction and completed rehearsal. Its code, tests, and schema/distribution contracts exactly match the rehearsed source. A clean detached worktree produced the darwin/arm64 executable with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; build metadata records that revision and `vcs.modified=false`.

The corrected executable was installed at `~/.local/bin/tiller`, still reporting 0.13.0, with SHA-256 `f0fcdb30819d991a3510ddc16469916cee44fb2c5b61f27aa0f85df810449aed`. The outgoing same-version build was verified and retained before atomic replacement. Installed project-only maintenance confirmed schema 6 and generation 38 without changing the migrated project's snapshotted files, task reads, or checkout identity. The prior build/lint/test and rehearsal evidence was reused because the correction was unchanged; those suites were not rerun for installation.

The reviewed migration was merged through the project's ordinary human Git workflow. Post-merge maintenance reported migration and artifacts current. No Tiller source push, tag, GitHub release, other-checkout migration, or console restart occurred. Reload existing agent instructions and restart consoles before resuming work; other worktrees and teammates still need coordinated adoption.

## Schema-6 migration rehearsal

**September 20, local branch only.** After checkout writers were paused, the corrected working-tree candidate migrated a managed platform project from schema 4 to 6 on a dedicated migration branch. This was an actual adoption, not a disposable copy or a migration to repeat. Board, orientation, and harness wiring refreshed to generation 38; all ten generated skills remained checkout-local under the project's existing ignore rule.

An unfinished feature retained its contributors and active state while obsolete exclusive ownership and unsubmitted branch metadata were removed. Completed tasks retained their IDs, owners, acceptance, and review records; remaining tasks stayed ready and unstarted. Application files, authored Markdown, and existing events were unchanged. Migration added one attributed event. Independent source-state and generated-artifact reviews passed; repeated maintenance reported migration and artifacts current with all snapshotted files byte-identical.

No project gates, task lifecycle actions, or sync ran during migration. Main, remotes, other worktrees, and installed binaries were not changed by the rehearsal. Private checkout paths, engagement links, contributor names, and project Git coordinates are intentionally omitted from this maintained record.

Every continuing branch must incorporate the migration and refresh its checkout; pulling main elsewhere does not update an old work branch. Retained legacy branches can still collide with future task branch names, and their cleanup remains an ordinary, separately authorized Git action.

## Drained unfinished-feature migration

**September 20, included in the corrected local 0.13.0 build.** The schema-5 drain guard now distinguishes unfinished feature scope from in-flight execution. An active feature with only unstarted or terminal tasks and no recorded aggregate PR/review ref may migrate from schemas 2 through 4. The shared predicate drives inspection, apply, legacy ancestor branch-field cleanup, and execution warnings. It preserves the feature's active status and remaining task assignments while removing obsolete unsubmitted ancestor branch metadata. In-progress, checked, and in-review tasks and nonterminal ancestor reviews still refuse. Migration never changes Git refs; retained legacy branch names still receive ordinary collision warnings.

The regression failed before the correction and passed afterward. Tests cover task and ancestor-review refusals across all three source schemas, preserved owners/dependencies/accepted work/history, read-only inspection, branch warnings, and concurrent or post-inspection task changes. The screenshot fixture guard, `make build`, `make lint`, and full uncached `make test` passed in CI order.

A disposable project built with the actual 0.12.1 binary exercised each in-flight task refusal, then completed one task while leaving another ready under an active feature. Installed 0.13.0 refused that drained state; the candidate migrated it from schema 4 to 6 without an ownership review, preserving Git bytes, authored prose, prior events, and task history. Repeated project-only maintenance changed no bytes. After explicit Git archival of the colliding legacy branch, the remaining task ran through start, checks, review, human merge, and sync to task/feature/epic done. This synthetic verification preceded the Acme branch rehearsal above. It changed no managed project, installed binary, version stamp, schema number, or orientation generation.

## Local 0.13.0 distribution

**September 20.** Stamp commit `6f9df65` sets version 0.13.0 and includes the relationship reads, task-level ownership, reviewed migration, and ownership-review corrections. Schema remains 6, orientation generation remains 38, and ten skills remain embedded. A clean detached worktree supplied the darwin/arm64 artifact, built with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`; its build metadata records the stamp revision and `vcs.modified=false`. The verified artifact was atomically installed at `~/.local/bin/tiller`, with matching SHA-256 `f5ef7f8f7fe8339d3c123f0cb49bca3980a6445f99e07ce768122b646d68a4ec`.

The version-stamp check, stamped-version and upgrade tests, uncached Go suite, Go lint, Go vulnerability scan, and console dependency audit passed. The release binary completed the Acme fixture and migrated a committed inactive schema-5 scratch project to schema 6 and generation 38 with ten shims. Migration required a registered actor and committed source inputs; subsequent project-only refresh needed no checkout identity and reported both migration and artifacts current. Chromium exercised Home, Explore, and contributor detail. The installed CLI, version API, and rendered console all reported 0.13.0.

The outgoing 0.12.3 executable is verified and retained at `~/.local/share/tiller/previous/0.12.3-b3041d037cc4f27ab288c32c9f4c069d268560cf5ae5b5886f4b596bf5aed9a7/tiller`. No managed checkout, user console process, release tag, push, GitHub release, or Homebrew package was changed. Schema-5 adoption still requires the reviewed ownership procedure for active work; binary installation alone changes no project state.

The separate documentation-site audit reported four vulnerable packages: Astro (critical), plus js-yaml, sharp, and svgo (high). These site build dependencies are not shipped in the binary and were not updated in this local distribution.

## Task-level ownership

**September 19, source only.** [Issue #110, task-level ownership](https://github.com/madeintandem/tiller/issues/110), replaces feature claims with independent task, bug, and chore reservations. First start permanently fixes the scalar owner and records `started: true`; terminal attribution is immutable even when cancelled before start. New tasks are unclaimed. First task start atomically activates its approved feature and initializes the applicable aggregate identity without bypassing parent dependencies, bases, questions, or closed review rounds. Feature assumptions remain explicit through `approve feature --assume-question`. Features and epics have no owner or compatibility alias.

Ancestor contributors are sorted, distinct owners of started descendant tasks, including done and cancelled history. Reservations, shelf attachments, creators, and unrelated reviewers do not contribute. Personal work contains unfinished execution assignments rather than historical contributor workloads. Scalar owner filters do not match ancestors; explicit contributor filters do. CLI, generated board, and console share workflow classification and observed contract, basis, and branch-inclusion facts for closeout. The board's Shared closeout section and console queue name each applicable review unit once without assigning a coordinator.

Human-directed `transfer task|bug|chore` preserves the original's cancelled history and creates a distinct approved, unstarted successor for the recipient. One guarded transaction carries the approved prose, fresh basis path, reciprocal lineage, applicable prerequisites, order, questions, pauses, and carrier constraints. Unstarted dependents redirect; live started dependents refuse; terminal relationships remain historical. Gate results are not copied. Coarse review identity survives, but affected acceptance becomes stale. Repeated transfer identifies the existing successor. Tiller creates no Git branch and calls no host API.

Schema 6 requires explicit reviewed ownership for active schema-5 execution. Read-only inspection produces a source-bound report with observed and confirmed owners. Missing, stale, incomplete, conflicting, or unknown-owner confirmations refuse before writes. Migration preserves branches, reviews, results, unknown YAML fields, ready reservations, and recoverable historical attribution without inventing missing owners. Schema-2/3/4 projects still drain the earlier shared-branch model before crossing schema 5. Coordinated adoption uses one reviewed integration migration carried into continuing work branches, not separate migrations of stale clones.

Disposable old and new binaries exercised active migration, source/ref preservation, malformed historical events, review refusals, all three execution transfers, operative dependent/question/carrier preservation, and two clones sharing a temporary bare remote. Independent sibling owners completed task-, feature-, and epic-grain local reviews and merges; an unrelated developer checked and submitted aggregates. An independent task-grain override retained its separate-review completion guard. A transferred replacement completed through the preserved aggregate while both original and successor contributors remained visible.

Fresh receiving agents used only generated orientation and embedded guidance at all three review grains. They preserved another developer's reservation, implemented and checked scratch work, committed task submissions, and handed off shared review without taking ancestor ownership. The feature-grain agent continued transferred work through ordinary Git, retained the original record byte-for-byte, assembled the replacement, and reran shared checks. Its blocked-transfer explanation required human-reviewed resolution rather than force flags or invented repair commands. These tiny fixtures prove workflow guidance, not substantive application acceptance.

Embedded Chromium exercised Home, Build, Bugs & Chores, Explore/filter/map, node detail, reciprocal replacement links, and shared closeout. Ready claims did not contribute; first start added contributors; cancellation and completion retained them; an unrelated reviewer stayed absent. Owner and contributor filters differed correctly, and navigation/manual refresh observed changed state. The final candidate rendered one shared assembly entry matching the generated board.

Independent reviews covered mutation safety, migration, transfer relationships, console/read models, and the shared-classifier extraction. Corrections preserved legacy-key relationships, refused already merged or unobservable transfer reviews, validated migration preflight output, and retained shared board closeout. The expanded board observer exposed an unsafe basis-file read; anchored nonblocking regular-file reads fixed it. Its FIFO regression failed before the fix, passed afterward, and the actual binary regenerated a board without hanging on an unsafe task prose path.

Final gates passed: `make test` with the complete uncached Go suite, `make lint`, `make build`, the **1,201-test console suite**, `make ui`, orientation golden regeneration, the real candidate binary build, and the **14-page documentation-site build**. The version consistency check still matches 0.12.3. The site retains its missing-404-content warning; the UI build retains its large-chunk and optional `fsevents` install-script advisories. None prevented verification.

The source and scratch binary still report **0.12.3**. Schema advances from **5 to 6**, orientation from **37 to 38**, and ten skills remain embedded. No version stamp, installation, release publication, managed-project migration, or GitHub issue-state change was performed. Hosted PR creation, closure, and merge APIs were not exercised: local Git, recorded review identities, and controlled boundaries do not prove hosting-service behavior. Installation and coordinated adoption remain separately authorized actions.

The completed `2026-09-19-task-level-ownership.md` plan is retired. Recover its full execution contract with `git show 049a6e4:docs/plans/2026-09-19-task-level-ownership.md`. Durable decisions and verification remain above; installation and managed-project adoption remain separate.

### Ownership review corrections

**September 19, source only.** The Acme screenshot fixture still called removed feature claim/start commands, preventing CI from reaching its Go gates. It now claims each task and relies on first task start for feature activation. Form loading retains its dependency on the completed app scaffold, but drops the dependency on the unfinished foundation epic so Dana's parallel work remains legal. The parent-dependency gate is unchanged; relaxing it would be a separate product decision.

Dependency refusals now offer only the integration wait when no aggregate branch applies, both for ancestor prerequisites and task-grain task prerequisites. Coarse-grain assembly advice and the JSON reason/field contract remain unchanged. Regressions failed before the fix and passed afterward; disposable real binaries exercised the fixture's promised states, branchless human/JSON refusals, and named-aggregate recovery. The fixture's failure-propagation guard also passed. Explore images were not reshot and still show the removed epic edge. Version, schema, orientation, and installed binaries are unchanged.

**September 20 CI fixture correction, source only.** The hosted test run after those fixes failed because two test fixtures relied on Git's default initial branch while later steps required `main`. The shared owned CLI fixture and coarse-review fixture now normalize the initial branch to `main`, matching the existing fixture convention. Both failures reproduced with `init.defaultBranch=master` and passed after the correction. With that default supplied through an isolated Git configuration, the screenshot guard, `make build`, `make lint`, and uncached `make test` passed in CI order. A rebuilt binary completed the Acme fixture with the documented owners and states and returned branchless dependency refusals in human and JSON output. No product behavior, version, schema, orientation, installed binary, or screenshots changed; hosted CI has not been rerun for this correction.

## Shared relationship reads

**September 19.** [Issue #122, shared relationship queries and console navigation](https://github.com/madeintandem/tiller/issues/122), now uses one request-local index for dependency ordering, node/map relationships, amendment identity projection, and CLI/API traversal. Its 19-kind catalog owns canonical directions, inverse adjacency, endpoint capabilities, and source evidence. `tiller query` and `GET /api/query` share breadth-first traversal, repeated starts, direction unions, depth, and terminal filters that retain explanatory bridge vertices.

The shared confined Markdown reader preserves search safety while supplying owned and loose documents, generated-source labels, and exact mention lines. Go extraction and the Markdown renderer share conformance fixtures, including footnotes and standalone code spans. Work surfaces, baseline descriptions, and classified acceptance bindings retain distinct coverage bases. Directory claims can answer child-file queries without claiming verified implementation coverage. External coordinates never become local file links; opaque bindings remain references. Historical amendment identities retain evidence and warnings instead of resolving old keys to today's holders.

The resting console map remains containment-first. Selected and all-relationship overlays expose every catalog kind, contextual document/path/question/amendment endpoints, and provenance. Backlinks count distinct sources, preserve repeated citation lines and document ownership, and separate generated mentions. Existing dependency, attachment, amendment, and effective-question panels retain their roles. A failed map refresh keeps the last successful graph; late node responses cannot supply another selection's detail. Browser verification found and corrected a zero-height map under Explore's narrow-screen page-flow rule.

Verification passed the uncached Go suite, Go vet, all 1,192 console tests, orientation fixture regeneration, embedded asset/binary builds, and documentation-site build. A populated, unjoined scratch project exercised all three documented queries and every canonical/inverse direction with equal CLI/API data. Equivalent legacy-key and ID fixtures produced identical tree order, node relationships, and feature workflow; the assumption waiver and inherited question hold were unchanged. Subsequent requests observed hand-edited mentions and branch switches without cache reset. Symlinked corpus input refused without writes; concurrent generation changes produced CLI exit 3 and HTTP 500. Draft contracts and unresolved historical identities returned explicit incomplete diagnostics.

Embedded Chromium checks covered resting and selected/all maps, all 19 kind inspectors, external and unresolved endpoints without false navigation, question/amendment pages, exact source-line backlinks, generated grouping, Back/Forward, pan/zoom/collapse, delayed reads, failed refresh retention, keyboard controls, and a 390px viewport. Successful and failed CLI/HTTP reads and console navigation preserved project files, raw Git-index bytes, HEAD and refs; the unjoined checkout stayed unjoined. The existing narrow topbar overflow remains outside this change. The generation guard detects Tiller mutations, not an atomic snapshot of arbitrary editor or Git changes racing a request.

Schema remains 5 and the skill inventory remains ten. Orientation advances to generation 37, but source version stamping, installation, publication, and managed-project refresh were deliberately not performed. Both the scratch binary and untouched installed version stamp remain 0.12.3; the installed build still emits generation 36. Adopters need a later binary installation followed by explicit project-only maintenance. Ranked prose search and persistent caching remain separate scope. The completed `2026-09-19-relationship-index.md` plan is retired; Git preserves its execution contract.

### Relationship review follow-ups

**September 19, source only.** Relationship diagnostics now name sorted, distinct subjects. Query results retain ordinary warnings about reached vertices, including filtered bridges; backlinks retain warnings about their node and owned documents. Completeness remains conservative across the request's index, so all extraction-loss explanations survive filtering. Missing contracts and physically owned drafts with empty criteria are normal pre-specification state. Malformed, misowned, unreadable, and unaddressable contracts remain distinct failures; acceptance gates are unchanged.

The approved read-policy change lets node and map pages retain structural facts when supplemental corpus reads fail. Failed walks discard partially scanned document enrichment, and unreadable cited documents remain unresolved. Diagnostics distinguish unavailable inputs from proven absence. Queries still refuse these reads, and malformed trees, cancellation, and generation instability remain failures. Node prose and workflow contracts now use confined reads too, so the degraded response cannot expose unsafe bytes through a separate reader.

Unresolved structural parents again project as absent, while dangling shelf/dependency rows retain authored references and questions retain target order. Revision evidence uses zero-time omission without discarding recorded subsecond precision. The console's All dependencies preset includes dependency, attachment, and carrier edges; turning it off restores all-kind selected-neighborhood inspection. Shared citation rendering keeps safe source-line links, copyable coordinates, revision evidence, and current-file disclosure consistent across map and backlinks.

`DependencyOrder` now builds one dependency-only index and retains ranked task groups for existing-order consumers. Standalone subset ordering remains independent. An isolated benchmark with a prebuilt structural graph, 100 features, and five tasks per feature reduced map projection from 3.21 ms, 4.35 MB, and 43,997 allocations to 1.85 ms, 2.62 MB, and 21,959 allocations per operation. Node projection timing was broadly unchanged and its allocated bytes rose slightly. These are in-memory ordering measurements, not whole-request latency. Corpus parsing remains request-local; [issue #124, repeated relationship corpus parsing](https://github.com/madeintandem/tiller/issues/124), records the deferred profiling and cache-design work. A same-size edit with a restored mtime demonstrated why metadata alone cannot guarantee cache freshness.

Disposable CLI/API checks proved missing/draft completeness, scoped warnings, visible global extraction loss, CLI/HTTP query parity, strict query refusals versus node/map degradation for permissions, NUL prose, and symlinks, and omission of unrecorded revision times. Embedded Chromium exercised actual three-kind arcs, preset reset with selection retained, exact source-line navigation, and wrapped pinned-source details in light and dark themes. The screenshot fixture was corrected from shared feature branches to task-owned branches; all six WebKit images were reshot and inspected. All 1,197 console tests and the console build passed. No release, installation, or managed-project refresh was performed.

Final gates passed: `make build`, `make lint`, the full uncached Go suite, the 1,197-test console suite, `make ui`, and the documentation-site build. The timestamp regression failed against the pre-change code before the fix. Independent read-only reviews found no actionable defects in the ordering/projection and console slices. The site still reports its missing-404-content warning, and the console build retains its large-chunk advisory; neither prevented the builds.

The completed review plan is retired. Its decisions and verification are recorded above; deferred caching remains in build status and issue #124. Recover the original with `git show 56f3920:docs/plans/2026-09-19-relationship-query-review-followups.md`.

## Local 0.12.3 distribution

**September 18.** Implementation commit `f94e025` supplies unified maintenance; stamp commit `71aa688` sets version 0.12.3. Schema remains 5, orientation generation is 36, and ten skills remain embedded. The clean darwin/arm64 artifact was built with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`, then atomically installed at `~/.local/bin/tiller`. Its SHA-256 matches the verified artifact: `b3041d037cc4f27ab288c32c9f4c069d268560cf5ae5b5886f4b596bf5aed9a7`.

The version-stamp check, stamped-version and upgrade tests, uncached Go suite, and Go lint passed. The exact release artifact initialized a disposable project, migrated its committed drained schema-4 baseline through offline project-only maintenance, refreshed generation 36 and ten shims without creating identity, and repeated maintenance without source or artifact writes. Explicit join and work creation then populated the scratch console. The installed CLI, version API and rendered Chromium console all reported 0.12.3; navigation opened the scratch task detail. Console code and embedded assets were unchanged.

The actual outgoing 0.12.2 executable was verified and retained at `~/.local/share/tiller/previous/0.12.2-131edcb800a36ab93a7d68e8a9f24111b94d7ef037ae86ff65e58a84658f8f11/tiller` before replacement. This is a schema-5 build, not a compatible drain binary for active schema-4 work. No managed checkout, public release, tag, push, Homebrew installation or user console process was changed. Other checkouts still need project-only maintenance, and existing console and harness sessions need restarting or reloading.

## Unified maintenance

**September 18.** `tiller upgrade` now coordinates the installed binary and current checkout. `--project-only` performs offline migration and refresh; `--binary-only` excludes project maintenance. Join remains explicit identity selection. Current-schema refresh needs no owner and never writes identity; a real migration resolves a registered actor and records its invoking maintenance command. Schema remains 5, orientation advances once to 36, and the ten-skill catalog is unchanged.

Candidate inspection and source application share the migration registry and drain predicates. Inspection reports every active legacy blocker before self-managed replacement. Existing transformation inputs need committed clean baselines, including protection against edits hidden by Git index flags; new events need absence guards. Migration preserves terminal history and Git refs. A single derived transaction refreshes orientation, board, owned skill shims, managed instruction regions and ignore entries. Human text, unrelated skills and newer guidance are protected; repeat maintenance produces no unnecessary writes or events.

Self-managed upgrades retain verified outgoing builds under `~/.local/share/tiller/previous/<version>-<sha256>/tiller`, then use the exact verified target for maintenance. Homebrew remains the installation owner: metadata verifies its prefix and package, delegation updates only Tiller with installed-dependent checks and automatic cleanup disabled, and the resulting executable is resolved again. A current Homebrew binary cannot preflight future migration rules, so version-skew checks may finish only after delegation. No launcher, project registry, automatic rollback or retention pruning was added.

Binary replacement, source migration and artifact refresh remain separate durable boundaries. Typed protocol-1 results retain completed phases on nonzero exits through `data` or `error.partial_result`, never both. Unknown child outcomes and recovery-required journals are not described as rollback. Planned paths and shim changes remain distinct from confirmed writes.

Verification passed the uncached Go suite, Go lint, regenerated orientation fixture and documentation-site build. Disposable real parent/target binaries, with distinct version overlays rather than a repository release stamp, exercised combined same-schema refresh, schema-4 migration, full active feature/task refusal preserving project bytes and raw Git index, second-checkout offline maintenance, and a permission-induced refresh rollback after successful replacement and migration. The parent preserved that nonzero child result, and project-only retry completed without another migration event. Controlled Homebrew cask/formula boundaries exercised owning-prefix delegation, moved executable paths, already-current refresh, newer-guidance repair, unsupported pins and private-asset failure. Regressions cover retention conflicts/races, nested PATH selection, dirty inputs, lineage changes, obsolete-shim ownership, and transaction recovery.

No installed user binary, managed checkout, release stamp, tag, GitHub state or real Homebrew installation changed. Homebrew verification used controlled executables, not a clean-machine package install or live private-release authentication. Console assets were unchanged. An older running release cannot acquire these new guarantees during its own upgrade; initial adoption still needs a compatible prior build kept separately when required, followed by the newly installed binary's project-only command.

## Local 0.12.2 distribution

**September 18.** Implementation and stamp commit `d1677c4` supplies task-scoped branches, schema 5, orientation generation 35, and ten embedded skills. Its clean darwin/arm64 release build uses `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`. The binary was atomically installed at `~/.local/bin/tiller`; its SHA-256 matches the verified artifact: `131edcb800a36ab93a7d68e8a9f24111b94d7ef037ae86ff65e58a84658f8f11`.

The final uncached Go suite, Go lint, version-stamp script, focused version/upgrade tests, and documentation-site build passed. The clean artifact repeated remote-backed task squash completion, offline epic aggregate completion, active legacy migration refusal without writes, and drained migration preserving terminal bytes and refs. The installed binary initialized a fresh project, delivered ten shims and generation 35, and returned the task's branch and integration base. Its CLI, version API, and rendered console reported 0.12.2; visual inspection confirmed the task branch in node detail. Console source and embedded assets did not change, so the console unit suite was not rerun.

The prior installed binary is retained at `~/.local/bin/tiller-0.12.1`, SHA-256 `bf11f4bb5a31e9379e8d4ca5b9133ca0abfdfbded8fe0bd2c3baae8dab7f7a78`, for draining schema-4 shared-branch work before migration. No tag, push, GitHub release, Homebrew update, managed-project migration, or restart of user consoles was performed. Existing consoles need restarting; drained projects run `tiller migrate` to upgrade state and refresh instructions.

## Task-scoped working branches

**September 18.** [Issue #123, task-scoped branches at every review grain](https://github.com/madeintandem/tiller/issues/123) replaces shared feature workbenches with immutable task branch identities, defaulting to `tiller/<epic>/<feature>/<task>`. Task submission, unpublished-state warnings, and merge observation use that task's branch. Requested changes retain its branch and PR. Feature claims are unchanged.

Feature and epic review remain explicit choices. Each has a separate aggregate, `tiller/reviews/features/<feature-key>` or `tiller/reviews/epics/<epic-key>`. The executing model creates branches, merges complete task tips including submission state, resolves mechanical conflicts, reruns checks, and maintains the aggregate review. Human decisions remain semantic conflicts, acceptance, and the final integration merge. Aggregate submission and sync require both each task's submitted head and current branch tip; intermediate assembly never completes work. Independent tasks start from integration; coarse dependent tasks start from the assembled aggregate.

Schema 5 stores task branches. Migration refuses active legacy shared-branch work without writing: finish or cancel it and sync with the older binary first. Migration preserves terminal records, events, and Git refs, clearing only dormant nonterminal aggregate metadata. Retained prefix refs produce a safe conflict rather than an automatic rename or deletion. New work after a completed aggregate requires sync to reopen its parent, then records a fresh aggregate identity instead of reusing the merged branch. Orientation advances to generation 35 with ten embedded skills.

Real CLI verification covered separate sibling branches, same-review resumption, merge commits, squash/content observation, offline reviews, and local bare-remote publication. Separate model-run feature and epic fixtures created and assembled their own branches, carried a dependent task from its reported aggregate base, checked and submitted complete reviews, and stopped with integration unchanged. Independent inspection verified task heads and tips in each aggregate; fixture-only human merges then completed both tasks by ancestry or content. The first fixture gate configuration was corrected before final runs; recorded build and test commands were deliberately harmless, not an application test claim. A generated-board conflict in the direct smoke run was resolved with `tiller gen board`.

Legacy migration refusal preserved manifest bytes, worktree state, and refs; drained migration preserved terminal state bytes and historical refs. The preserved 0.12.1 binary rejected schema 5 as unsupported. Independent review found missing-aggregate observation and premature reuse of a completed aggregate; real-Git regressions failed before both corrections and passed afterward. Chromium rendered the task's own branch and version 0.12.2; screenshot helpers timed out, so visual evidence used the browser's CDP capture.

Hosted PR creation and hosting-service merge behavior were not exercised. Local bare-remotes and recorded PR numbers covered publication and identity, not a live host integration.

## Local 0.12.1 distribution

**September 15.** Stamp commit `2f359a1` sets version 0.12.1 without changing behavior from local 0.12.0. Schema remains 4, orientation generation remains 34, and ten skills remain embedded. The darwin/arm64 binary was built from that clean commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`, then atomically installed at `~/.local/bin/tiller`. Its SHA-256 matches the verified artifact: `bf11f4bb5a31e9379e8d4ca5b9133ca0abfdfbded8fe0bd2c3baae8dab7f7a78`.

The version-stamp check, stamped-version and upgrade tests, uncached Go suite, and Go lint passed. Scratch-project checks covered initialization, join, work creation, named-guide text/JSON, ten shims, generation 34, missing-shim recovery through join and migrate, repeated refresh with stable content and default routing, and raw Git-index preservation during status reads. The installed CLI, version API, and rendered embedded console reported 0.12.1; feature detail rendered correctly. Browser click helpers timed out, so candidate navigation used programmatic link activation and the installed-binary check loaded the feature route directly.

No tag, push, GitHub release, Homebrew update, managed-checkout refresh, or restart of existing user consoles was performed. Existing consoles need restarting; checkouts already refreshed for 0.12.0 need no instruction refresh. Agent synthesis and independent receiving-tool prototype acceptance were not rerun; the latter remains outstanding.

## Read-only design briefs

**September 14.** An explicitly requested `design-brief` skill exports the UI/UX requirements of an epic, feature, or task as one self-contained Markdown snapshot. It uses the existing named-guide, orientation, and catalog-derived shim delivery paths. Source orientation advances to generation 34 with ten skills; schema remains 4. No new command, lifecycle readout, canonical artifact, index, design-service integration, or console surface was added. Initial implementation left installed and published binaries unchanged; the subsequent local 0.12.0 distribution is recorded below.

The export exception lives in shared guidance as well as the skill: generation neither repairs source gaps nor records questions, claims, approvals, gates, events, or commits. Only an explicitly requested output file may be written, and an existing human-edited brief requires explicit replacement authorization. Requirements, reversible prototype suggestions, and unresolved decisions stay distinct at each affected interaction. Approval fingerprints identify content separately from lifecycle status and the approval commit; dirty approved blobs may be unavailable locally. Existing implementation is evidence of current behavior, not automatic authority over intended requirements.

A reusable prompt remains the simpler alternative. Expanding the dedicated skill requires real-handoff evidence that it preserves hierarchy, source state, preservation requirements, acceptance coverage, and uncertainty. A context-export API or persistent artifact machinery is not a remedy for an unproven prompt. Prototype hosting and ingestion back into the work tree were outside the design, not promised follow-ups.

The disposable Harbor Requests fixture combined an existing static UI, a modifying feature with introduce/preserve criteria, relevant UI and server-only tasks, a task with uncertain UI impact, a narrative-only feature, cancelled work, and unrelated billing. Real-agent runs discovered the skill through both installed shims and harness-neutral orientation plus CLI. The briefs retained ancestor constraints, full epic feature coverage, bounded task scope, loading/empty/error/retry and archive states, and non-visual acceptance limits. Dirty requirements, conflicting persistence decisions, missing approval objects, absent design references, and unavailable assets remained visible. Ambiguous targets produced a clarification; ordinary next-work routing produced the existing orient readout rather than a brief. Reply exports made no deliverable file; the requested file was the sole output write; regeneration preserved the human-edited destination.

Full file-content inventories, Git HEAD, refs, raw index hashes, and command transcripts were recorded outside each inspected checkout. Initial runs inherited `GIT_OPTIONAL_LOCKS=0` from the driver. Repeating without that protection exposed an index metadata write from bare `tiller status`, even after export guidance taught guarded reads: an agent could ask for status before loading that guidance. The correction makes `vcs.Git.DirtyUnder` invoke Git with `--no-optional-locks`. Its byte-level regression failed before the fix and passed afterward; the real binary also preserved stale-stat index bytes with optional locks enabled by the caller. Export guidance retains guarded Tiller and direct Git reads.

After the Git read correction, both the ordinary feature export and the dirty-approval/missing-object export were repeated without a driver-supplied index guard. Every inventoried file, including raw index bytes, plus HEAD and refs stayed unchanged. Their command transcripts contained only context reads. The representative brief kept all five criteria, distinguished intended archive preservation from static-code evidence, and named missing assets and non-visual proof limits.

Delivery verification exercised named-guide text/JSON, fresh join, a generation-33 checkout missing the new shim, current-schema migrate, repeat refresh, and unchanged bare-guide routing. The orientation golden was regenerated through its existing update mechanism. No console code or assets changed.

Final `make test`, `make lint`, and the documentation site's `npm run build` passed. These delivery and regression checks are separate from synthesis review and do not close the external prototype acceptance gap.

Independent prototype acceptance was not performed. The existing-browser relay timed out and a dedicated browser reached the design service's sign-in page. The remaining check is tracked in [build status](plans/STATUS.md#unbuilt-or-deferred): provide only a representative feature brief and named assets to an independent design tool, then navigate its prototype against the original requirements and coverage map. Agent-produced briefs and guide delivery are not substitutes.

**Local 0.12.0 distribution.** Implementation commit `0e32317` adds the design-brief export; stamp commit `f7a3dc7` sets version 0.12.0. The darwin/arm64 binary was built from that clean stamp commit with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags '-s -w'`, then atomically installed at `~/.local/bin/tiller`. Its SHA-256 matches the verified artifact. The stamped CLI tests, version-stamp script, uncached Go suite, Go lint, and documentation build passed. Scratch CLI checks covered text/JSON guide delivery, ten shims, generation 34, both instruction-refresh paths and repeat refresh, with unchanged bare-guide routing. The installed CLI, version API, and rendered console all reported 0.12.0. This distribution also carries the previously uninstalled Build header correction; it does not close independent design-tool prototype acceptance. No tag, push, GitHub release, Homebrew update, or other-project instruction refresh was made. Existing console processes must restart, and each current-schema checkout needs `tiller join` to expose the new skill.

## Founding model

**June 20–24.** Tiller began with the artifact schema, CLI, skills, amendments, console, and distribution design. The central decision was to keep project intent and execution state beside the code, under git, rather than in a database or a separate planning service. Markdown carries human-owned prose; YAML carries machine-mutated state. Stable ids identify nodes while readable keys identify their locations. Revision references record what a decision or gate was based on.

The responsibility boundary has stayed intact: Tiller holds structure, gates, and state; the agent does the reasoning; the human decides at the gates and controls merge. Tiller guides rather than orchestrates a harness. Its portable agent interface is files, a shell, the CLI, and embedded skills, not a vendor integration or MCP service. Existing code is an input to understand; brownfield is the general case, not an exception to greenfield rules.

The two early delivery trials shaped this design: a multi-engineer platform and a single-developer application, now represented in public-facing materials as Acme and Lantern. Their repeated failures at real workflow boundaries drove the later changes below.

## First CLI and dry runs

**June 27–July 5.** The first Go CLI established the file store, id/key resolution, append-only events, lifecycle transitions, command and rubric gates, revision evidence, generated orientation and board, and merge observation. The first target was a real end-to-end build loop, not a broad command catalog. Thin CLI adapters call the domain layer; Git observation and pure projections remain separate responsibilities.

Dry runs made several boundaries concrete. Initializing a project and selecting a checkout identity became separate acts through `init` and `join`. Imported design inputs are copied with provenance, not claimed as verified specifications. Gate scope follows the manifest and the grain where acceptance lives. A refusal needs a usable next move, not just an error. CLI JSON envelopes and human references became consistent across commands.

Approval was separated from the physical act of typing a command. A developer can explicitly delegate a named start approval; an agent cannot infer that approval or attest a human gate. Remote-less projects gained branch-and-commit review references rather than invented PR numbers. Orientation became a briefing that names the next step and stops for the developer's direction.

Milestones: [first CLI scaffold](https://github.com/madeintandem/tiller/commit/ae2f5ba), [checkout identity split](https://github.com/madeintandem/tiller/commit/47bf1dc), [delegated start approval](https://github.com/madeintandem/tiller/commit/dc4788c).

## Console and readouts

**July 5–26.** `tiller serve` added an embedded, read-only console over the working tree. The first browser field probe exposed facts lost between storage and presentation: source provenance, task ordering, review targets, acceptance contracts, and gate evidence. The correction was to fix the shared model first and carry those facts through CLI and HTTP, not add a console-only interpretation.

The console became a set of views over one project: a decision-focused Home, an Explore navigator and reading pane, node detail, and a history digest. One dependency-ordering authority feeds every surface. Evidence and source files are confined reads. Missing or partial data must not render as all-clear. Opening Home does not consume the history visit window; opening History does.

A generated readout replaced model-written summaries at workflow boundaries. The CLI and every harness receive the same account of recorded work, evidence, and the next action. Notes about side effects outside Tiller remain separate. Later refinements made attention a counts-only view of the shared Queue, kept correction readouts distinct from full handoffs, and treated a suggested route as guidance rather than a new gate.

The July field probe was accepted as exploratory evidence, not proof that every original console acceptance step had run. Later package and browser verification strengthened the built surfaces; it did not implement the deferred watcher or console writes.

**September 11 status freshness.** Keeping Explore mounted preserved branch expansion but also retained old navigator statuses while node detail loaded current state. Navigator reads now refresh on pathname changes, independently of expansion state. A regression reproduced the mismatch before the fix; an embedded-browser scratch run then showed five tasks and their ancestors turn green after merge, sync, and navigation. Automatic updates remain unbuilt.

**September 11 async navigation.** The status-freshness correction exposed a loading-state defect: clearing successful navigator data temporarily removed the long tree and let the browser clamp its scrollbar. The fix retains successful data only while refreshing the same API resource. It preserves the existing router, request cancellation, expansion map, and stable row keys rather than adding scroll restoration or a cache. A fixed-height header reports pending resources and refresh failures while the rows remain usable; successful empty responses still remove rows. The loaded project summary also survives manual refresh so its initial-load gate does not tear down the split. Node and file readers keep clearing old destinations.

A deferred regression failed on the disappearing task link before implementation. The console suite passed 1,196 tests, and the final focused run passed 106. The rebuilt embedded console was exercised against a disposable 96-task project. Its 109 navigator links stayed mounted at a 1,800px scroll offset during delayed, failed, and superseded reads and after recovery. Browser verification covered both response orders, real recorded status updates, history and native links, manual-summary refresh, independent detail and source-line scrolling, and cold-load restoration and failure. Structural-change anchoring, hard-reload scroll persistence, and automatic updates remain outside this correction.

**September 13 prose references.** Issue #120 (clickable epic, feature, and task references) exposed a gap between the console's linked relationship rows and bare IDs in authored descriptions. The shared Markdown renderer now turns standalone work IDs into immutable-ID node links, including single-ID code spans. An AST transform preserves existing links, command spans, code blocks, and source files instead of rewriting Markdown or fetching a second work index. Destinations resolve on navigation. Regression cases failed before implementation; all 1,198 console tests passed afterward. Rebuilt embedded assets and a scratch binary were checked visually in Chromium, with programmatic link activation across epic, feature, task, and loose-file prose and Back/Forward navigation. Native pointer automation timed out, so those interactions were not verified through native clicks.

**September 13 Build header clipping.** The column's 10px top padding left a visible strip above its sticky heading. Moving that padding into the heading lets its background cover the scrollport's top edge while preserving unscrolled content positions. The rebuilt embedded console was checked with 24 synthetic done-feature cards at six scroll positions in light and dark themes, plus horizontal panning at a 1280px viewport. Header geometry and hit-testing showed no exposed strip; screenshots confirmed the rendering. The 13 Build CSS source-string assertions were removed rather than extended to pin another declaration. All 1,185 remaining console tests passed. Local 0.11.6 did not include this correction; local 0.12.0 does.

Milestones: [console foundations](https://github.com/madeintandem/tiller/commit/5019131), [shared read-model correction closeout](https://github.com/madeintandem/tiller/commit/33c541c), [dependency-ordering authority](https://github.com/madeintandem/tiller/commit/08919e9), [readout command](https://github.com/madeintandem/tiller/commit/d734430).

## Single tree and drift

**July 27–29.** The largest subtraction replaced two planning axes with one progressively elaborated tree: epic, feature, task. Blitzes and their scheduling layer were removed. Tasks moved from a shared ledger to folders with their own implementation prose and state. Specification, acceptance contract, and task decomposition became one just-in-time feature-planning step behind one approval. The separate `break-down` skill disappeared.

Done became evidence-backed rather than ceremonial. A task reaches done through an observed human merge; feature and epic done are recomputed rollups that can reopen when new work arrives. The `accept` verb and epic acceptance file were removed. Task-only review was the initial simplification; configurable review grain later widened PR scope without restoring the discarded planning axis.

Merge observation was hardened against ancestry that did not prove a merge. Merge commits require a witness; squash detection compares content; an indistinguishable fast-forward needs the human's explicit assertion. A retained PR or an exit-zero `sync` is not proof that every node crossed to done.

Drift became active assistance rather than a blanket work stop. The scan reports suspected disagreement and uncovered comparisons; only applicable mutation guards such as `active_basis` refuse work. Approval and merge record different revision anchors. `mend-drift` proposes a correction and returns to human judgment. The proposed `current_state_match` marker and unused surface-module vocabulary were dropped instead of presenting unsupported comparisons as coverage.

## Safe storage and read surfaces

**July 29–August 19, with later hardening.** Multi-writer and filesystem failures established that atomic individual files were not enough. Mutations now validate a loaded generation and source snapshots, plan a complete batch, journal it under the repository lock, and refuse stale or unsafe writes. Recovery must preserve unrelated edits and cannot silently turn a partial mutation into success.

Path safety moved beyond string normalization. Source, eval, evidence, and mutation paths are constrained at the filesystem boundary, including symlink components, aliases, missing files, and rollback paths. Literal filenames remain literal through Git and URL encoding. Search gained confined file destinations so a result can be opened without inventing a work node for an unowned document.

Gate evidence also became more explicit: stale criteria after resumed work, long-running command progress, missing or invalid evidence, dirty working-tree provenance, and unpublished state are distinct conditions. Optional reviews remain visible but do not become required gates. Scope guards prevent work beneath retired ancestors or before the parent plan gate, without treating every legal recommendation as a compulsory order.

These changes hardened the implemented paths; they did not settle every schema-validation, collision-repair, or evidence-retention question. Remaining work is listed in [build status](plans/STATUS.md).

## Amendments and intake

**August 6–12; correction refinement August 27–28.** Mid-project changes became staged, reviewed forward work. An amendment records intended operations and the live facts on which they depend; approval validates the whole proposal and applies one source transaction. Done work is superseded by new work, not rewritten. Non-terminal prose may be revised and re-pinned. Cancellation and supersession have different effects and preserve coherent lineage.

Intake added a sourced entry point for external feedback. The input document is copied and fingerprinted, proposed operations cite it, and one amendment presents the resulting changes for review. Same-batch parenting was deliberately narrow: a staged feature can name an epic staged earlier in that amendment; adding tasks still requires a live-tree feature. New epics extend the recorded root order without replacing the human's partial ordering.

Not every correction warrants an amendment. Display-title changes remain direct. Unapproved proposal keys, parents, and nodes can be corrected safely while preserving stable ids and surviving references. Accepted live prose needs a human basis refresh where applicable; structural changes after work starts and corrections to terminal history remain forward work. The objective was a usable correction path, not either blanket immutability or unrestricted history editing.

Automatic intake splitting and large-proposal presentation were deferred to field evidence. Strict amendment input decoding was proposed separately and must not be confused with the implemented lifecycle, lineage, and transaction guards.

## Ownership and routing

**August 8–19; routing corrections September 2–4.** Soft order became recorded intent, not a second dependency system. Hard dependencies gate work; soft order refines recommendations and presentation. The human can pace the project without manufacturing dependencies simply to arrange the backlog.

A trial in which an unregistered identity was introduced during joining exposed the need for explicit registration. `owner add` became the deliberate roster write; `join` only selects an existing owner. Fallback identity from the environment or checkout file must be visible in command output. Tiller records the developer directing the work, not a separate agent persona.

Features are born unclaimed. Claim, release, takeover, reassignment, and owner retirement became explicit operations. Claims cascade to live tasks and authorize execution; judgment is not exclusive to the claimant. An epic review branch has one live claimant across its riding features. Historical ownership is not a substitute for the current claim.

Questions became real work relationships with short scan titles and retained full statements. Open direct and inherited holds drive readiness and routing without duplicating mutable reverse edges. The route may select later legal work while naming an earlier hold. Approved unclaimed work with a genuinely unheld ready task is an execution opportunity, not a falsely caught-up state. Checked work is recovered before starting another ready task.

Milestones: [shared pool and question-held routing](https://github.com/madeintandem/tiller/commit/fad080d), [claimable execution](https://github.com/madeintandem/tiller/commit/42a39bb), [question-aware claimable execution](https://github.com/madeintandem/tiller/commit/778ac2f).

## Release and distribution

**August 22 onward.** The repository gained the MIT license, contributor and security guidance, contribution templates, DCO sign-off, CI, release automation, a Homebrew cask, an installer, and an Astro/Starlight documentation site. The site copies the owning Markdown sources rather than maintaining a second set of specifications. Trial-project names were neutralized as Acme and Lantern.

Releases use a source version stamp and matching tag; the release workflow checks that they agree. Console assets and skills are embedded so installing Tiller does not require the managed project's runtime. Schema version, binary version, and orientation generation remain distinct: refreshing instructions is not a data migration, and compiling a local binary is not publishing a release.

September dependency work moved the Go toolchain to 1.27.1, refreshed the console and documentation stacks, and added grouped Dependabot updates with the team's release cooldown. Full Go verification deliberately defeats caching because the end-to-end package builds the real CLI as a subprocess.

The public-launch checklist remains separate from these implemented mechanics. Local installation, authenticated asset downloads, anonymous installation, and clean-machine Homebrew installation are different claims and need their own evidence.

## Review grain and upgrades

**August 26.** Review grain made a PR represent exactly one task, feature, or epic, with task as the default and more-specific settings overriding broader ones. Shelf work still owns its own review. Changing grain is constrained by live work and recorded branches so a setting cannot silently retarget an existing review.

Coarse submission records the reviewed feature's or epic's PR after its riding tasks and required acceptance work are ready. Riding tasks record review references, not extra PRs. Green checks do not bypass active-basis, question, owner, ancestor, or structural guards. One owner drives an epic-grain branch; independently overridden features finish their reviews before the epic review opens.

The same delivery added real schema migrations and binary upgrade. `migrate` is the tree-side remedy; `upgrade` is the binary-side remedy. The first migrations were stamp-only, from schema 2 to 3 and then 3 to 4. Upgrades verify archive checksums before replacing the binary and refresh checkout guidance through the new executable. Homebrew-managed installations are left to Homebrew. The proposed launcher, per-project engine cache, schema fence, and CI status emitter were not required to ship these smaller, useful operations.

Milestones: [migration registry](https://github.com/madeintandem/tiller/commit/5adbec2), [coarse submissions](https://github.com/madeintandem/tiller/commit/16a8b88), [binary upgrade](https://github.com/madeintandem/tiller/commit/6e1b0a3).

## Bugs and chores

**August 28–29.** Bugs and chores became independent work nodes, not records that can close only by pointing at a feature task. Each has its own prose, state, claim, task-shaped lifecycle, gates, and PR. Attachment supplies context without changing the reviewed node. A carried item waits on its recorded carrier and stays out of execution routing; a cancelled carrier makes the unresolved work visible again.

The three builds delivered storage and lifecycle, routing and skills, then console and search. `capture` proposes a complete contract and stops for human triage. `execute` handles separate ready, in-progress, checked, and requested-changes entries. The shelf board, node attachments, Home queue, Explore, and search all use the shared model rather than a parallel tracker. Schema 4 also closed the post-migration instruction-refresh gap by refreshing joined checkouts on every `migrate` run.

## Console map and Home

**August 30–September 2.** Explore gained a map over one shared read endpoint. It settled on a hierarchy-first mind map with dependency arcs as an overlay, not a permanently tangled dependency diagram or a new layout dependency. Cards moved into an HTML layer over SVG wires; selection, fit, reveal, keyboard focus, and remembered state were hardened around the actual visible area. The selected-node card became floating and draggable.

Home was redesigned for bursts of decisions. A feature-state funnel and shelf counts establish scale; a route strip names the next move; decision kinds group the human's queue; a pulse carries history, in-flight work, and what is next. Queue eligibility stayed in one projection. Layout and summarization did not acquire permission to invent asks or omit held work. Correction readouts gained `--since` so a small revision at a gate reports the change rather than repeating the entire original handoff.

## Brownfield baseline

**Designed August 28; all three builds completed September 5.** Adopting Tiller does not prove existing docs describe existing code. The baseline workflow records that relationship explicitly. Project kind describes the relationship to pre-existing code, while a feature's change type describes its actual change. Explicit code-root modes and reference protection prevent a surface declaration from granting permission to modify a read-only reference tree.

The optional baseline ledger maps design docs to described paths and recoverable source blobs. `derive-baseline` reads existing docs and code, chooses a move or pointer according to ownership, and records translations without deleting external sources. A human confirms named docs against current working-tree code. One confirmation releases the existing-code startup gate; it is not a claim that every root or remaining doc has been covered.

Baseline drift follows the same detect, propose, approve boundary. `stale_design` joins confirmed docs to tracked feature completions and missing paths; opt-in `shifted_design` compares the confirmed working-tree fingerprint. Unsafe paths or unavailable history remain uncovered rather than clean. Successful sync notices name touched docs without approving or rewriting them. The console exposes confirmation, provenance, drift, and repair commands read-only.

Milestones: [root protections and onboarding](https://github.com/madeintandem/tiller/commit/b129d6f), [baseline ledger and confirmation](https://github.com/madeintandem/tiller/commit/bf43061), [baseline drift and console](https://github.com/madeintandem/tiller/commit/a5b9649).

## Publication and authorship

**September 5–6.** A real execution stopped after green checks because guidance treated ordinary publication as human-only work. The fix made an authorized execute run responsible for implementation and state commits, publication, PR creation or reuse, Tiller submission, and publication of submission state. Tiller itself remains record-only for Git publication. Human approval, attestation, and merge boundaries did not move.

Recovery and reporting now distinguish checked work, current submission, riding coarse references, and unpublished state. A PR created before a refused submission and a submission recorded before a failed push are separate incomplete states. An unchanged successful submission needs no empty commit or redundant push. The route recovers checked work instead of skipping to another task.

Orientation generation 31 added a shared authorship rule: the developer owns and is responsible for the work. Authored code, comments, docs, commits, PRs, issues, reviews, reports, and replies must not identify an agent, model, provider, or harness as an author or contributor. No generated-by notices, agent signatures, or agent co-author trailers. Real human credits, necessary technical references, and required third-party notices remain intact. The console also gained the running binary's version beneath the wordmark through `/api/version`, independently of project loading.

The v0.11.1 release was stamped, tagged, and installed locally on September 6. Implementation commit `999f8ef` and stamp commit `0b2ce1f` were not pushed or published to GitHub during that release. Recorded verification passed the uncached Go suite, 1,160 console tests, lint, asset and documentation builds, real CLI/Chromium scenarios, Go vulnerability scanning, and both npm audits. The installed binary reported the tagged revision with a clean source tree. This is historical release evidence, not a claim that documentation housekeeping reran those checks.

## Console tabs, browser open, and baseline fixes

**September 6.** Five issues from the first weeks of real use landed as one batch after v0.11.1.

Every console tab used to read "Tiller". Each view now names itself, page part first because tabs truncate from the right: the project on Home, the page on Build, Bugs & Chores, History, Explore, Questions, and Drift, the node header's own handle on a node page, and basename plus line on a file page. The console also gained the documentation site's mark as its favicon.

`tiller serve` opens the console in the default browser once its port is bound, best effort and never a gate. Three overrides turn it off, most specific first: `--open=false`, `console: { open: false }` in the new `.tiller/local.yml`, and `BROWSER=none`. That file is the first per-developer settings home: git-ignored beside `identity.yml`, hand-edited, never written by Tiller, tolerant of unknown keys, and refused as `local_settings_unreadable` when malformed. The ruling behind it kept `tiller.yml` the committed team manifest and `identity.yml` single-purpose, and preserved the self-contained principle: a preference lives in the repo, never in global or machine config.

Three baseline defects were fixed with regressions. Reapproval had scanned done features in Go map order and aborted a whole named batch on the first unavailable historical crossing; it now reads every overlapping crossing in sorted order, reapproves on any known same-HEAD crossing, and records a fresh confirmation rather than refusing when only unavailable history remains. On case-insensitive filesystems an unstaged case-only rename had dropped a tracked file from the surface inventory, so approval fingerprinted an empty tree and later edits read clean; the observer now accepts a case variant only when the directory proves both spellings are the same entry. The console's prose endpoint had keyed a registered design document's confirmation off the symlink-resolved path, so a document replaced by a symlink borrowed another's confirmation; registered identity now comes from the requested address and a symlinked registered document is refused with readiness's wording.

The v0.11.2 release was stamped, tagged, and installed locally on September 6 without a push or GitHub release. Verification ran the uncached Go suite, 1,170 console tests, lint, the embedded-asset build, a real CLI smoke of `serve`, and a Playwright read of every tab title against the running binary.

Issues: [#108](https://github.com/madeintandem/tiller/issues/108), [#109](https://github.com/madeintandem/tiller/issues/109), [#111](https://github.com/madeintandem/tiller/issues/111), [#112](https://github.com/madeintandem/tiller/issues/112), [#113](https://github.com/madeintandem/tiller/issues/113).

## Pending acceptance is not a passed check

**September 7, unreleased.** [Issue #114, false-green check results](https://github.com/madeintandem/tiller/issues/114), exposed a distinction the CLI had collapsed: recording a required gate's pending result is successful bookkeeping, but acceptance has not passed. `check` now agrees with default `eval`: pending required acceptance returns `passed: false` and exit `1`, and task or shelf work does not advance to `check`. The human output names outstanding required criteria and the human's attestation command without presenting optional criteria as blockers. The JSON shape and advisory merge/rollup policy are unchanged.

## Acceptance runner selection

**September 7, unreleased.** [Issue #117, targeted test runners versus aggregate CI](https://github.com/madeintandem/tiller/issues/117), exposed a contract-authoring mistake: appending a spec path to an argument-ignoring CI wrapper reruns the whole pipeline and attributes unrelated failures to individual criteria. Specify now teaches the existing command-resolution rule, selector inspection, explicit targeted `binding.run` commands, and independent preparation. The ordinary full CI gate stays intact. This changes embedded guidance, not runtime inference or the schema; corrections to approved non-terminal contracts still require human accepted-basis review.

## Execution handoffs and amendment navigation

**September 7, unreleased.** Issues [#115, post-merge execute guidance](https://github.com/madeintandem/tiller/issues/115), [#116, feature closeout routing](https://github.com/madeintandem/tiller/issues/116), and [#118, consistent execution handoffs](https://github.com/madeintandem/tiller/issues/118) now share one workflow projection. It reads current required-gate configuration, authored contracts, recorded results, and local eligibility once per relevant feature. Machine checks, rubric preparation, human acceptance, formal review, and integration-branch sync remain distinct facts. Node advice sits beside lifecycle moves; task counts do not become feature-review counts. Eligible approved execution precedes unrelated planning approvals, after deliberate amendment review. New live tasks beneath a done rollup feature remain routable.

Orientation generation 32 carries the matching execution protocol. Authorized work includes evidence commits on non-passing checks, verified draft or offline review preparation before human acceptance, same-review submission and publication recovery, and bounded post-merge closeout. Pure orient still briefs and stops. Required attestation-backed gates expose an existing mutation distinction: check resets them to pending, unlike settled criteria. Necessary mechanical checks therefore precede their human sign-off; a redundant recheck must not erase it. Schema 4, nine skills, strict coarse submission, advisory sync, and human merge authority remain unchanged.

[Issue #119, amendment navigation](https://github.com/madeintandem/tiller/issues/119), gained one identity projection for forward links and node backlinks. Retained operations and immutable event targets establish created nodes, replacements, addition parents, and question-block intent. Additive event provenance records operation positions and resolved parents without adding amendment YAML fields or an applied-results ledger. Missing or contradictory history stays unresolved rather than attaching old work to a reused key. Epic and feature pages roll up descendant effects without labeling unchanged siblings directly amended. Proposed notices, counted amendment tabs, and a permanent Explore Filter entry preserve browsing after approval or withdrawal.

Verification passed the uncached Go suite, lint, 1,185 console tests, generation-fixture regeneration, and the embedded asset build. Real agents consumed the built guides against disposable Git repositories and bare remotes with a controlled host boundary. Five riders produced one draft; a fresh agent reused it after human acceptance, retained local formal submission through an actual rejected push, recovered publication, and handled requested changes without another PR or empty resubmission commit. The final merge produced five ancestry crossings and two clean rollups. Separate scenarios proved required-gate acceptance, offline submission, no-publish and unavailable-host boundaries, committed machine-failure evidence, read-only orient, integration-branch final-task closeout, and historical violations surviving later acceptance.

Chromium exercised the embedded console, including bidirectional amendment links, retained reused-key records, proposal context beside drift, direct tabs and fallback, Back, keyboard activation, zero results, and explicit incomplete-history warnings. Amendment tabs wrapped within a 390px viewport; the existing shared topbar still overflowed that width. Console reads neither mutated project state nor fetched host facts. This was source verification only: no release, installed-binary refresh, live hosted PR, or issue closure.

## v0.11.3 publication

**September 7.** Release stamp commit `98bf558` was tagged and published as [v0.11.3](https://github.com/madeintandem/tiller/releases/tag/v0.11.3). The release workflow produced macOS and Linux archives for amd64 and arm64, SHA-256 checksums, and the updated Homebrew cask. Schema remains 4; orientation generation is 32 with nine embedded skills.

Stamped-version tests, the uncached Go suite, lint, GitHub CI, the release workflow, Go vulnerability scanning, and both npm audits passed. All four downloaded archives matched their checksums, declared targets, and clean release revision. Authenticated self-upgrade replaced the local binary, and an isolated installer run produced identical bytes. The installed binary reported 0.11.3 and emitted the expected generation-32 orientation and execute guide.

Repository visibility stayed private. Authenticated distribution was verified; anonymous downloads and clean-machine Homebrew installation were not. No existing project's generated instructions or acceptance contracts were rewritten. Each checkout picks up the new guidance through `tiller join`, or `tiller migrate` where schema refresh is needed.

## Managed-project independence

**September 10, unreleased.** A test-runtime optimization in the Acme trial turned a reasonable planning-only CI skip into a requirement to download a private Tiller release, supply a separate release token, and pass Tiller readiness before CI could succeed. The dependency entered through an agent-authored plan, not application requirements. The existing harness-independence principle had not explicitly covered managed projects, and the deferred CI-emitter design invited the opposite behavior.

The [distribution contract](spec/distribution.md#managed-project-independence) now extends independence through project setup, build, tests, CI, packaging, deployment, runtime, and merge eligibility. Ordinary project operations need neither Tiller, its releases, nor `.tiller/` operational or state-validation inputs. Local Tiller may invoke independently runnable project commands to record evidence, never the reverse. Optional Tiller CI jobs are not an alternative. Tiller's own implementation, tests, release automation, and disposable verification remain legitimate users of Tiller.

Orientation generation 33 and the specify/execute guides carry the rule and require an agent to surface a conflicting existing or approved plan with an independent alternative. They preserve human review of accepted-contract corrections. Conservative planning-only classification uses ordinary repository tooling and defaults to full application verification for uncertain or protected changes, without replacement Tiller checks. `ci-status` and the CI emitter are rejected by design, not deferred or renamed. No scanner, runtime refusal, lifecycle change, or schema change was introduced.

Verification built the real CLI, generated generation-33 orientation through init/join, read the emitted specify/execute guides, and confirmed a repeated join preserved every fixture file's contents. A disposable Python application normalized account names; `bin/build` compiled it, `bin/test` ran two doctests, and `bin/ci` ran both. Those commands passed with Tiller absent from `PATH`, release credentials absent, and then `.tiller/` absent. A deliberate lowercase-to-uppercase defect failed the ordinary command. Local `tiller check chore` separately recorded passing, failing, and restored passing gate results with readable output evidence, without changing the project entrypoints.

Two fresh agents consumed the generated orientation and emitted guides in separate copies of that fixture. These scenarios are retained as the behavioral regression recipe, not a new evaluation framework:

| Scenario supplied to the agent | Observed outcome |
|---|---|
| Plan and implement avoiding full application runs for verified planning-only commits, while preserving an honest required CI result. | Produced an always-running CI job and a Git/Python path classifier, kept `bin/ci` unchanged, and introduced no Tiller installation, invocation, credential, or state validator. The classifier was exercised on real local Git revisions: planning-only pushes and a simulated PR merge selected the skip; code, mixed, protected manifest, executable planning path, empty diff, and missing history selected full verification. The workflow's application shell step propagated the injected test failure without printing a success summary. |
| Implement an already-approved plan that explicitly installs Tiller 0.11.3, uses `TILLER_RELEASE_TOKEN`, and requires `tiller status` and `tiller check readiness` in CI. | Identified the guidance conflict, proposed independent classification, and stopped for plan revision rather than adding the dependency. Original application and workflow hashes and the exact supplied plan were preserved. |

The uncached Go suite, Go lint, orientation fixture regeneration, and documentation-site build passed. Independent review found one remaining actor-model recommendation for CI attestation enforcement; it was removed. The site build still reports its missing-404-content warning while completing successfully. These are local source and controlled-agent observations, not universal enforcement or a live GitHub Actions run. The completed implementation plan was retired into this record. No managed repository, host setting, release, installed binary, or existing checkout was changed; a future release and per-checkout `tiller join` deliver the new guidance.

## Local 0.11.4 distribution

**September 11.** Version 0.11.4 was stamped and installed at `~/.local/bin/tiller`, carrying the Explore navigator freshness fix and generation-33 managed-project independence guidance. Schema remains 4. The CLI, version API, and rendered embedded console reported 0.11.4; the installed binary matched the smoke-tested candidate by SHA-256. This was local distribution only: no tag, push, GitHub release, Homebrew update, managed-checkout refresh, or restart of existing user consoles.

The stamped-version and upgrade tests, tag/stamp check, uncached Go suite, and Go lint passed. The console's 1,186 tests passed before stamping; no UI source changed during the stamp.

## Local 0.11.5 distribution

**September 11.** Version 0.11.5 was stamped and installed at `~/.local/bin/tiller`, adding the retained Explore navigator and background-refresh indications described above. Schema remains 4, orientation generation remains 33, and the binary carries nine embedded skills. The stamped CLI, version API, and rendered embedded console reported 0.11.5; the installed binary matched the smoke-tested candidate by SHA-256. The executable on `PATH` resolves to that installation.

This was local distribution only: no tag, push, GitHub release, Homebrew update, managed-checkout refresh, or restart of existing user consoles. Existing consoles need restarting to load the new binary. Checkouts already on generation 33 need no instruction refresh for this UI-only change.

The v0.11.5 stamp check, uncached Go suite (including stamped-version and upgrade coverage), and Go lint passed. The candidate used the distribution build flags (`CGO_ENABLED=0`, `-trimpath`, `-s -w`) and was exercised against a disposable project before atomic installation. The console's 1,196 tests and long-tree browser verification belong to the preceding navigation-fix verification; no UI source changed during stamping.

The sidebar's reserved refresh band was subsequently reduced to one 16px line after the installed console exposed an excessive idle gap. Chromium verification kept header geometry and a 900px navigator scroll offset unchanged through idle, pending, failed, and recovered states. At the developer's request, the corrected assets were rebuilt and redistributed locally without changing the 0.11.5 stamp. The stamp check and installed CLI still reported 0.11.5, and the installed executable matched the corrected candidate by SHA-256. No remote release or tag was created.

## Local 0.11.6 distribution

**September 13.** Version 0.11.6 was stamped and installed at `~/.local/bin/tiller`, adding the clickable work references described above. The installed darwin/arm64 binary was built from clean commit `9026df7` with the distribution flags (`CGO_ENABLED=0`, `-trimpath`, `-s -w`). Its SHA-256 matched the verified candidate: `fa9f8b2e2fde9ac27a7de25ff0a0c160c8cebb9bfdf012283825db5d4d5442ef`.

The stamp check, stamped-version and upgrade tests, uncached Go suite, and Go lint passed. The console's 1,198 tests passed before stamping. Scratch-project initialization, join, and work creation succeeded; the committed candidate's CLI, version API, and rendered console reported 0.11.6, and a prose task reference opened the correct node through programmatic browser activation. Atomic installation preserved the verified bytes, and `tiller --version` on `PATH` reported 0.11.6.

Schema remains 4, orientation generation remains 33, and nine skills remain embedded. No tag, push, GitHub release, Homebrew update, managed-checkout refresh, or restart of existing user consoles was performed. Existing consoles need restarting; generation-33 checkouts need no instruction refresh for this UI-only change.

## Retired records

Completed plans and their mocks no longer serve as active instructions. Unchecked boxes, old branch names, agent task assignments, and intermediate review failures in those records describe an execution at that time, not today's backlog. Current contracts stay in the specs; unfinished scope stays in build status or a retained active plan.

- [Original dated plans and console mockups](https://github.com/madeintandem/tiller/tree/23bab6495bc2fc90cae8776862647937d0bb7a96/docs/plans), at the last remotely available snapshot before consolidation.
- [Original specs with full decision logs](https://github.com/madeintandem/tiller/tree/23bab6495bc2fc90cae8776862647937d0bb7a96/docs/spec), at the same snapshot.
- The `Release: stamp v0.11.1` commit `0b2ce1fa` on `main` additionally preserves the September 6 authorship and console-version changes.
- The local pre-retirement commit `d056a15` preserves `docs/plans/2026-09-14-design-brief.md`, including its historical scratch-evidence locator, and the condensed `docs/plans/2026-08-07-amend-input-validation-corrections.md`. Recover either with `git show d056a15:<path>`; their unfinished scope remains in the maintained specs and build status.
- The local pre-retirement commit `479fe61` preserves the last three dated plans: `docs/plans/2026-10-07-console-read-performance-implementation.md`, `docs/plans/2026-10-07-merged-review-recovery.md`, and `docs/plans/2026-10-08-review-findings-fixes.md`. Recover any of them with `git show 479fe61:<path>`. Their outcomes are recorded in [Local 0.16.4 distribution](#local-0164-distribution), [Merged aggregate review recovery](#merged-aggregate-review-recovery), and [October 8 review fixes](#october-8-review-fixes).

To recover an exact plan cited by an older source comment or commit, use its original path at the pre-consolidation commit `0b2ce1fa`. For example:

```sh
git show 0b2ce1fa:docs/plans/2026-08-26-review-grain-design.md
```

The retired plans are not duplicated in an archive directory. Git already preserves every byte and the changes leading to it. Future completed plans follow the same rule: add the durable decision and outcome here, preserve real remaining work elsewhere, then retire the execution detail.
