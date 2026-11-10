# Console UI improvement hunt

Status: PROPOSED 2026-11-09. Nothing here is built and nothing is ruled. This file records one pass through the console looking for defects and for changes that would make it easier to use, easier to scan, and better organized. Each item says what was seen and what is proposed. The order at the end is a suggestion.

## How this was gathered

The console was built from main at `5240960` and served read-only against the CALCTP project (23 epics, 152 features, 443 tasks, 75 questions, 320 shelf items). Every page was loaded in headless Chromium at 1512 by 900, with a 1024 pass and a dark mode pass on Home. Pages covered: Home and its three tabs, every Needs You tab, Explore in Browse, Filter, Map, and Search modes, the Questions and Amendments ledgers, node pages for an epic, two features, a task, and a bug with every feature tab, Build with expanded cards, Fix, Drift, Report, and History in two windows.

Timings come from `curl` against the running server. Positions come from `getBoundingClientRect` in the page. The screenshots were scratch files and are not kept in the repo. CALCTP was not modified.

## Defects

### D1. In-review task pages take about a minute to load

`/api/node/task/T-xv4mtr` returned 200 after 65.1 seconds with an 11,990 byte body. Two other in-review tasks, `T-p79hvj` and `T-ut3cvf`, were cut off at 60 and 25 second client timeouts. Every other node timed returned in 0.36 to 0.39 seconds: a ready task, a done task, a feature, an epic, and a chore in check. All seven in-review tasks that Home's In Flight column links to are in the slow group, so the most natural click on Home lands on a page that sits on its loading state for a minute.

The cause was not investigated. The slow nodes are the ones with a live task branch observation, which is the first place to look.

Proposed: find and fix the slow path, then add a serve test that bounds the node read for an in-review task. Independently of the fix, the node page should render its header and prose as soon as they are available and let a slow section arrive late, so one slow read cannot blank the whole page.

### D2. The navigator does not reveal the current node on arrival

Arriving on `/node/feature/F-elc7cd` with a few epics already open, the selected row sat at y=1682 in a 900 pixel viewport and the tree's `scrollTop` was 0. For a bug it sat at y=3909. The row is marked selected and its ancestors are open, but nothing scrolls it into view. `Navigator.tsx` has no `scrollIntoView` call.

Proposed: when the selection changes by navigation, scroll the selected row into view with `block: "nearest"`, so a click inside the tree does not jump.

### D3. Raw enums and event verbs reach the screen

- `rename.title.task` appears as the trailing verb in Home's Just Happened and in History.
- Build's expanded task chips read `in_review` while the column header reads In review.
- Attest cards say "Eval criterion recorded needs_attestation".
- Drift's section headings are `STALE_SPEC`, `ORPHAN_EVAL`, and `UNCOVERED`, and its rows carry `[orphan_eval]` and `ref_unreadable`.
- The node History tab shows verbs as `check.feature` and `sync.feature`.
- Filter mode's noun select lists `epics`, `features`, `tasks`, `questions`, `amendments`, then `bug`, `chore`, `tweak` in the singular.

Proposed: one display layer for status values and event verbs, used by every surface, with a test that fails when a verb or status has no phrase. Home already has `activityPhrase.ts`; the gap is the verbs it does not cover and the surfaces that do not use it.

### D4. The epic attention card has no content

On the `secure-access` epic the "Needs your attention next" card holds one command, `tiller show epic secure-access`, and no sentence saying what needs attention. The command shows the epic the reader is already looking at.

Proposed: when the attention fallback has no specific reason, do not render the card. When it does have one, say it in a sentence above the command.

### D5. Counts disagree between pages

- Home's Fix tile reads "25, 1 bug, 21 chores, 3 tweaks". Fix's toolbar reads "Showing 5 bugs, 21 chores, and 4 tweaks". The likely difference is that the board counts recently closed cards, but neither label says so.
- Home reads "443 tasks across 152 features, 67 retired". Report reads "Features done 65 of 74". Both are probably right for their own scope, and neither names its scope.
- Home and History both read "1000 events". This looks like a read cap presented as a count.

Proposed: label each count with its scope ("30 on the board, 25 open"), and render a capped count as "1000+" with the date it reaches back to.

### D6. An epic reads ready in one place and active in another

The `secure-access` header chip and the breadcrumb on its tasks say ready. `tiller status` lists it under "Active epics", and the navigator draws it with the in-flight glyph. This matches the ready-with-work-in-flight rule that already holds on Build and Home, and the unmerged `explore-board-status` branch (4926090a) that extends it to every surface.

Proposed: land or redo that branch so one rule decides the displayed status everywhere, including the node header chip and breadcrumbs.

## Improvements

### I1. Put the decision in the decision card

A Needs You card today names the node and gives the command. It does not show what is being decided.

- Attest cards show "Eval criterion recorded needs_attestation, awaiting your sign-off" and nothing about the criterion. The criterion statement is one click away on the Contract tab. Show the statement on the card and link the card to that criterion.
- Feature plan cards say "Spec written and waiting on your approval". Add what the approval covers, for example "7 tasks, 15 criteria, 10 dependencies".
- Every card repeats the tab label as an eyebrow ("ATTEST, EVAL CRITERION"). Inside a tab that already says so, the eyebrow costs a line per card. Three cards fill the first fold at 900 pixels. Dropping the eyebrow and tightening the padding should fit five or six.
- The attest command arrives as `--status pass`. A reader who copies it without reading has attested. Consider `--status <pass|fail>`, the same way the bug triage command shows `--severity <blocker|important|medium|minor>`.
- Attest cards lead with the key and id. Feature plan cards lead with the title and show the key beside it. Use the second form for both.

### I2. Lead with titles, keys second

The slug key is the main label in Home's attest cards, In Flight, and Up Next, in the Branches tab, in Fix's "Attached to" line, in Drift, and in Backlinks. Titles are what people recognize. The data is already there in at least one case: `/api/work-items` returns `attached.title` and the Fix card prints `attached.ref`.

Proposed: title first and key in mono beside or beneath it, as the feature plan cards and the search results already do. In Flight repeats `reviewed-audits/selection-and-queues/` on seven rows in a row. Group In Flight by feature, with the feature title as a small heading and the task titles under it.

### I3. Slim the node header and keep the tabs in reach

On a feature with two pending attestations the header, the workflow note, and two decision cards push the tab rail to y=745 and the first line of prose to about y=840. A long spec runs to 2,900 pixels and the tab rail scrolls away with the header.

- Make the tab rail sticky under the Explore mode bar.
- Collapse the header to one line (noun, status, title) once the reader scrolls past it.
- When a node has more than one decision, show the first and fold the rest behind "2 more".
- The prose column is about 630 pixels wide in a 1,200 pixel pane. Keep the reading measure and use the empty right side for an "on this page" outline of the spec headings, or for the contributor, branch, and verified-against metadata that currently sits above the fold.
- A ready task shows no next command, while Home's Up Next says "claim and start" for the same task. Show the claim and start commands on the task page.
- The workflow note reads "done remains recorded; acceptance remains unresolved: C2 (needs_attestation, human); C5 (needs_attestation, human); 1 historical policy violations remain recorded". Rewrite as a sentence a reviewer would say.
- Evidence links print the full artifact filename, around 100 characters of hash. Show a short label such as "eval C1 output" and keep the path in the tooltip.

### I4. Make the navigator readable

The rail is 300 pixels wide. Epic titles truncate ("Work the admin through the ..."), and task titles at the third level truncate at about 18 characters ("Build one non-root pr..."). The hover tooltip shows the key, not the title, so a truncated title cannot be read at all without opening the node (`Navigator.tsx` lines 436, 521, and 550 set `title` to the key).

Proposed: put the full title in the tooltip, followed by the key. Let titles wrap to two lines at the task level. Consider a wider default for the rail on wide windows, since the divider is already draggable.

### I5. Tier History

History for the "since your last visit" window rendered 1000 events as a page 17,770 pixels tall. Most rows are bookkeeping: linked, unlinked, reordered, renamed. Approvals, state crossings, and gate results are in the same list at the same weight.

- Default to milestones: approvals, status crossings, gate results, merges, questions asked and answered. Offer a toggle for bookkeeping, and collapse a run of bookkeeping rows into one summary line the way "4 features specced" already does.
- Add filters for actor, kind, and epic. The page already has "178 asked you" as a link, which is the same idea for one kind.
- Print the date once as a day heading, then time only on each row. Every row currently repeats "Nov 9, 02:06 PM".
- Put the verb in its own leading column. Today it trails the title in bold, so "Record sanctions, withdrawals and authorized recovery order" reads as a title that ends in "order".
- The page says "reading this view marks you caught up". Nobody reads 1000 events in one visit. Make catching up an explicit button.
- The node History tab repeats the verb on the right of each row ("check feature F-12keay" beside `check.feature`) and never shows the outcome. Replace the repeat with the result: passed, failed, refused.

### I6. Make the boards fit

Build and Fix share the same board and the same problems.

- At 1,512 pixels wide the last column (Done on Build, Closed on Fix) is cut off, while empty columns hold 240 pixels each. Collapse an empty column to a slim rail with its name and a zero.
- A Build card shows "0 / 7" with no label. For a feature whose seven tasks are all in review, that reads as no progress. Replace it with a small segmented bar in the status colors, as the epic rail already uses.
- An expanded card lists tasks in a column about 95 pixels wide beside the status chip, so titles wrap to five or six lines. Put the status glyph before the title and give the title the full card width, or open the task list in a side panel.
- "Contributors: None yet" appears on every unstarted card. Leave the chip out until there is a contributor.
- Three cards in a row repeat the same epic eyebrow. When one epic is selected, or when neighbours share an epic, the eyebrow can go.
- The toolbar toggle reads "Scroll, Board, Columns". Nothing explains that it chooses between one scrolling board and independently scrolling columns. Rename it or add a tooltip.
- The epic rail lists every done epic at full height with its contributor list, so the two epics with live work are below a screen of finished ones. Fold done epics under a "9 done" disclosure.
- Fix cards print "Attached to technician-entry/application-review (F-p3kdts)" in mono over three lines. See I2.

### I7. Bring Drift up to the console's standard

Drift is the one page that reads like raw tool output.

- Headings are enum names (see D3). Use plain names and one line under each saying what it means and why it matters.
- Rows show keys only. Show titles.
- Everywhere else, an action is shown as a CLI command to copy. Drift findings have none. Give each finding its fix or its next command.
- All nine Uncovered findings have the same cause: an eval `ref` holding several space-separated paths where a YAML list is required. They render as nine paragraphs of file names. Group by cause, state the cause and the fix once, and list the affected features beneath.
- The page is not in the nav and has no link back. Add a breadcrumb to Home, or make Drift a Home tab beside Branches and Recovered reviews.

## Smaller wins

**Search.** Search exists only inside Explore and there is no keyboard shortcut. No global key handler exists in `ui/src`. Add a topbar search field or a `/` shortcut that focuses Explore's search from any page. Searching "renewal" ranks two cancelled epics first. Rank cancelled and retired nodes below live ones, or honor Hide cancelled in results. The search input also shrinks to about 180 pixels and moves when Browse turns into Search.

**Home stat band.** The Epics and Questions tiles are not links, while the funnel, Fix, and Drift tiles are. All six funnel numbers link to bare `/build`, so clicking "4 specced" does not show the four specced features. Link each number to its filtered view. At 1,024 pixels the Fix and Drift tiles wrap under Epics and lose their alignment with the row.

**Up Next and triage lists.** Up Next rows say "claim and start secure-access/operable-runtime/production-image" as text with no copyable command, unlike every other action on the page. The bug, chore, and tweak triage tabs list titles and keys with no command at all, so triaging eleven chores means opening eleven pages. Add a copy control per row, and a "copy all" for a tab.

**Questions ledger.** It opens on All (75) rather than Open (25). A card for a question that blocks three features stacks five commands: answer, cancel, and one unlink per feature. Keep answer visible and fold the rest behind "more commands". Cards show the title, owner, and blocked features but not the question body, so the reader cannot answer from the card. Blocked features print as a comma-joined mono run; show them as linked titles.

**Amendments ledger.** Rows have no date and no stated sort order, and several titles are file names ("Intake: 2026-10-08-F-vlcnrk-plan-changes.md"). Status chips sit wherever the title ends, so they do not line up. Add a date column and align the chips.

**Timestamps.** Four formats are in use: "2:06p" on Home, "Nov 9, 02:06 PM" in History, "2026-09-11T20:23:11Z" on the node History tab, and "11/9/2026, 5:50:28 PM" on Branches. Use one formatter, relative where recent, with the absolute time in the tooltip.

**Explore empty state.** With nothing selected, a 1,200 pixel pane says "Select a node from the navigator to see its detail." Show something useful there: recently visited nodes, what needs the reader, and what is in flight.

**Filter mode.** It drops the navigator and uses three free-text inputs (status, contributor, epic) that give no hint of valid values until typed in. The right-hand column repeats the word "Contributors:" on every row. Use a column header.

**Map.** Collapsed epic cards show the noun pill, title, and count but no status, so the map cannot answer "where is the work" until nodes are opened. Add the status glyph. Epics are split left and right of the root, which hides their order.

**Branches tab.** All seven rows end with the same "observed 11/9/2026, 5:50:28 PM". Print it once above the list. The two explanatory paragraphs sit above the data; move them under a disclosure. Lead rows with the task title.

**Recovered reviews tab.** The record is a wall of 40-character hashes and file paths. Show the summary sentence, the PR, and the riding tasks by title, and fold the hashes behind "details".

**Backlinks tab.** Each source is headed by its file path, with "Owned by" and the title beneath. Lead with the owner's title. The first entry reads `provenance: unsupported recorded reference "..."`, which is an internal diagnostic and should either be explained or moved to the integrity notes.

**Report.** "Shipped, last 7 days: 54" has no unit. The Throughput axis has no unit either. The hatched band over the current week is not in any legend. Epic progress lists nine finished epics as full green bars above the two in progress; sort unfinished first, or fold the finished ones into one line. The page is meant for clients and has no print or save-as-PDF action and no date range control.

**Freshness.** Data rereads on navigation and on the manual refresh control. The only timer in the UI is the 30 second tick that updates the "synced" label (`AsOf.tsx`). A console left open on Home shows stale decisions with a label that keeps counting. Reread when the window regains focus.

**Accessibility.** `NodeTabs` renders buttons with `aria-current` inside a `nav`. They behave as tabs but are not announced as a tablist, and arrow keys do not move between them. Either adopt the tab roles or keep them as buttons and stop calling them tabs in the accessible name.

**Home tab placement.** "Overview, Branches, Recovered reviews" sits above the project title, so the title appears to belong to the Overview tab. Put the title above the tab rail.

## Suggested order

1. D1, because it makes the most common click on Home look broken.
2. D2, D3, D4, and D5. Each is small and each removes something that looks wrong on first sight.
3. I1 and I2 together. They change the same cards and rows, and they are what a reviewer reads most.
4. I3 and I4, the node page and the navigator beside it.
5. I6, then I5, then I7.
6. Smaller wins as they fit alongside the work above. Search ranking, the stat band links, and the timestamp formatter are each an afternoon.

D6 depends on what happens to the `explore-board-status` branch and can land whenever that is decided.

## Questions to rule on before building

- **Attest command.** Keep `--status pass` prefilled for speed, or show `<pass|fail>` so the reader has to choose?
- **Catching up.** Should viewing History still mark the reader caught up, or should that be a button?
- **What counts as a milestone in History.** The proposal is approvals, status crossings, gate results, merges, and questions. Is linking and reordering ever milestone-worthy?
- **Where Drift lives.** A Home tab, a nav entry, or the current unlisted page with a way back?
- **Empty board columns.** Collapse to a rail, or keep full width so the board's shape never changes?
- **Global search.** A topbar field on every page, or a shortcut that jumps to Explore's search?
- **Decision card density.** How much of a criterion or plan belongs on the card before it should just link to the node?

## Not verified

- The cause of D1.
- Why the Fix toolbar and Home's Fix tile disagree, and whether "1000 events" is a cap. Both are inferred from the numbers.
- Dark mode and the 1,024 pixel layout beyond Home.
- Safari. Everything here was seen in Chromium.
- Keyboard-only use and screen reader output. The accessibility note comes from reading `NodeTabs.tsx`.
- The Report's agent use tab, which did not appear for this project.
