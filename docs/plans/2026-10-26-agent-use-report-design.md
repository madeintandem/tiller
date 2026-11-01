# Agent use on the Report page: design

**Scope.** This plan designs a second tab on the console's Report page, "Agent use", that shows roughly how many model tokens the project's agents spent and where they went: by work item, by stage of the workflow, by model, and over time. It also designs the capture that feeds it: small per-harness readers that turn coding-agent session logs into neutral usage records committed under `.tiller/usage/`. The existing Report content moves, unchanged, under a "Project" tab. The owning specs are [ui-surface.md](../spec/ui-surface.md) (Report), [artifact-schema.md](../spec/artifact-schema.md) (the usage files), and [cli-surface.md](../spec/cli-surface.md) (the capture command and the `sync` hook); this plan changes each of them, and where the code and those specs disagree after the build, the specs are corrected in the same commit.

**Status.** Designed 2026-10-26 with JC. Not built. A spike over CALCTP's real logs validated the attribution method (numbers below).

## Why

JC wants a consistent approximation of the tokens each piece of work cost, so the team can make decisions about agent use with evidence instead of impressions. The decisions he named are which model to use for which kind of work, and how Tiller should instruct agents to construct specs. The others that follow from the same data: where the tokens go by stage (is review or checking quietly the expensive part), what rework costs, whether a task grain is too coarse, which items blew up, and how a change to Tiller's skills moves typical cost. He also has a hypothesis to test: that a Tiller project shows a higher cache-read share than unstructured agent use.

The bar is "close and calculated consistently," not exact. The same method applied the same way every time matters more than precision on any one item.

An older proposal, #121 in `madeintandem/tiller-history` ("Measure and improve end-to-end token efficiency"), is related but different: it measures Tiller's own instruction overhead with paired benchmark runs and a command trace. This plan measures a project's work. Two of #121's rules carry over: never ask the agent to count its own tokens, and missing usage is unknown, never zero.

## Goals

- A per-work-item token figure, with model and stage, captured automatically from the harnesses the team already uses.
- An "Agent use" tab on the Report page that reads as part of the same page: same cards, type, axis, tooltip, legend, enlarge dialog, and empty-state idiom as the Project charts.
- Raw token counts as the headline, with cache reads always shown separately from fresh input, cache writes, and output.
- Aggregation across every engineer on the project, through committed files, with no metric ever attached to a person.
- Harness and model agnostic: Tiller's core, the usage file format, and the report know nothing about any one harness. Only the readers do.

## Non-goals

- Dollar cost or any price table. Raw tokens only, for now.
- Per-person views, filters, rankings, or a "my usage" mode.
- Readers beyond Claude Code, Codex, and Pi/OhMyPi in this build. OpenCode and Gemini CLI are the likely next ones; Cursor's logs did not show usable token data.
- OpenTelemetry ingestion. It could become another reader later.
- Measuring Tiller's own instruction overhead (#121's scope), benchmarks, or paired scenarios.
- Storing prompts, responses, tool output, commands, file paths, or any content. Counts only.
- Real-time usage while a session runs.

## Anti-goals

These would make the feature a failure even if it technically worked.

- **A metric on a person.** JC's ruling: "never put a metric on a person, always stays with the work." Usage files carry no contributor, the payload carries none, and no chart is cut by person. Git history will still show who committed a usage file; that is git, not a metric surface.
- **Inventing zeros.** A harness with no reader, a week before capture began, or a work item with no usage records is "not measured," never zero.
- **Blocking work.** Capture failure never fails `sync` or any other operation, and never changes a lifecycle outcome.
- **A managed project that depends on this.** Principle 14 holds: nothing in a managed project's build, CI, or merge eligibility reads usage files.
- **Asking the agent to do anything.** No skill or orientation text asks the agent to report usage or run capture. Capture rides an existing command or is run by a human.

## What the spike established

A throwaway script read CALCTP's logs from all three harnesses (Claude Code, OhMyPi, Codex), cut each session at the Tiller commands it ran, and checked the result against Tiller's own event stream. Results for 2026-09-05 to 2026-10-22, after CALCTP's ledger was re-created on 2026-09-01 (40,857 model calls, 6.15 billion tokens):

- 74 percent of tokens landed on a work item: 49 percent on a task, bug, chore, or tweak; 19 percent on a feature (spec writing and feature acceptance); 6 percent on an epic. The remaining 26 percent is genuine project-level work (design conversations, orientation, planning, approvals) and is reported as its own bucket, "Not tied to a work item."
- Of the tokens credited to a task's implementation, 99.2 percent fell between that task's first `start` and last `submit` event (plus or minus an hour). The method agrees with Tiller's own record.
- Subagents spent 53 percent of all tokens. Crediting them through their parent session is essential.
- Cache reads are 96.8 percent of all tokens; fresh input 0.7, cache writes 2.4, output 0.2. This is why cache reads are always shown separately.
- Sessions from before the ledger reset could not be credited, because their node ids no longer exist. Capture has to run while the work is current, which shapes the capture design below.
- Claude Code deletes transcripts after 30 days by default (`cleanupPeriodDays`), so capture that waits too long loses data permanently.

What each harness records, per model call: Claude Code (`~/.claude/projects/<encoded path>/<session>.jsonl`, subagents in `<session>/subagents/*.jsonl`) gives model, input, cache creation, cache read, output, thinking tokens, timestamp, working directory, git branch, and every tool call. OhMyPi and Pi (`~/.omp/agent/sessions/<encoded path>/` and `~/.pi/agent/sessions/<encoded path>/`, subagents in a folder named after the parent session) share one format: provider, model, input, output, cache read, cache write, reasoning, timestamps, and tool calls. Codex (`~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`) gives the model in `turn_context`, per-turn usage in `token_count` events (`last_token_usage`, where `input_tokens` includes `cached_input_tokens`), the working directory in `session_meta`, and tool calls.

No existing library is usable as an abstraction. ccusage and tokscale read many harnesses but are Node and Rust programs that report daily or per-session totals, which is too coarse: one CALCTP session often spans many work items. The Go libraries found cover two or three harnesses and are months old. Their source is still a good map of each log format.

## The design

### How usage gets credited

Each model call in a session is credited to a work item (or to no work item) and to a stage, by replaying the session in order and tracking "what this session is working on now." The current work item and stage change only on these markers, found in the session's tool calls:

| Marker | Sets the stage to | Sets the work item to |
|---|---|---|
| `tiller start`, `claim`, `resume`, `transfer` | Implementation | the node named |
| `tiller check` | Checks | the node named |
| `tiller eval`, `attest` | Acceptance evals | the node named |
| `tiller submit` | Publishing | the node named |
| `tiller spec`, `return` | Spec writing | the node named |
| `tiller add`, `link`, `order`, `amend`, `intake`, `withdraw`, `unlink`, `rename`, `remove`, `configure`, `resolve` | Planning | the node named |
| `tiller approve`, `answer` | Other | the node named |
| `tiller sync`, `archive`, `cancel` | Closeout | the node named |
| loading the execute, specify, code-review, capture, amend, intake, decompose, orient, mend-drift, or derive-baseline skill (by `tiller guide <skill>` or the harness's own skill mechanism) | the skill's stage (execute is Implementation, specify is Spec writing, code-review is Code review, capture and amend and intake and decompose are Planning, the rest are Other) | the node named, if any, else unchanged |
| read-only commands (`show`, `status`, `readout`, `next`, `queue`, `ls`, `tree`, `search`, `query`, `drift`, `guide` without a skill) | unchanged, or Other if none yet | the node named, only if none is set |

A mutating verb with no node named (a bare `tiller sync`) clears the work item. A node is named by its id or its key, resolved against the tree at capture time and stored as an id, so a later rename or ledger change cannot orphan it. An id that does not resolve is stored as written. The call that issues a marker is credited to the new state. When nothing is set, a Claude Code call on a `tiller/<key>` git branch is credited to that key's node. A subagent starts with its parent's state as of the subagent's first call and then follows its own markers.

The detection matches `tiller <verb>` as a word in shell commands, not paths like `.tiller/` or `./cmd/tiller`. Each reader decides which of its harness's tool calls are shell commands and which are skill loads; the replay logic itself is shared and harness-neutral.

Known roughness, accepted: a stage is sticky until the next marker, so the agent fixing a failing check after `tiller check` counts as Checks, and chatter between items stays with the previous item. This is consistent, which is the bar.

### Stages

Nine stages, in this display order: Spec writing, Planning, Implementation, Checks, Acceptance evals, Code review, Publishing, Closeout, Other. "Other" holds approvals, orientation, drift, baseline work, and calls before any marker. These labels are page copy; the internal keys can differ.

### The neutral usage record

Committed, one file per top-level harness session, subagents folded in:

```
.tiller/usage/<harness>/<session-id>.yml
```

```yaml
harness: claude-code          # claude-code | codex | pi (Pi and OhMyPi share a reader and a format)
session: 165906e8-0e15-454a-a5b0-60b3affcce0d
reader: 1                     # reader version; a new version may recompute
captured_at: 2026-10-26T19:02:11Z
first_call: 2026-10-20T14:03:55Z
last_call: 2026-10-20T18:41:02Z
rows:
  - hour: 2026-10-20T14:00:00Z  # UTC hour bucket the calls fall in
    node: T-d8icjq               # omitted when not tied to a work item
    stage: implementation
    model: claude-opus-5-5       # as the harness reports it, provider prefix kept when the harness gives one
    calls: 41
    input: 1203                  # fresh input, excluding cache reads
    cache_write: 290114
    cache_read: 3920551
    output: 18220
    reasoning: 4410              # only when the harness reports it; part of output, never added to it
```

The hour bucket lets the report place usage on its week axis, which is defined in the `tiller serve` process's local timezone, with at most an hour's error. The harness name and the session id are the only identifiers. There is no contributor, no path, no command, no content. Each harness's fields are mapped into the four token counts so that the four never overlap (Codex's `input_tokens` minus `cached_input_tokens` becomes `input`, and so on); `reasoning` is informational and is not added to any total.

Why one file per session: session ids are unique, so two engineers never write the same file and there are no merge conflicts. Recapturing the same session rewrites the same file, so nothing is counted twice. A session that is still running is rewritten with more rows on the next capture.

**Never lose data.** If a session's log is gone, or now holds fewer calls than the file records (Claude Code's 30-day cleanup, a truncated log), capture leaves the existing file alone. Capture only creates and grows files, it never deletes one.

These files are written under the store's path confinement and the mutation lock, but capture records **no event**. Every event carries a required contributor and lists the files it touched, and target-less events surface in History and Home's "Just happened" with a person's name on them, which would put "this person captured these token counts" on a surface Tiller controls. Usage files are not lifecycle state, and each one carries its own `captured_at`, so nothing needs the event. A consequence: `tiller usage` needs no joined identity.

**Settled sessions only.** Capture skips any session whose last call is less than 30 minutes old. The session running `sync` is therefore never captured by its own sync, a sync with nothing else new writes nothing (so the execute skill's "identical retries write nothing" stays true), and an active session is captured once it has gone quiet. Usage files a sync does write are part of that sync's delta and are committed with it under the existing guidance.

**Integration branch only.** Capture writes only when the checkout is on `git.integration_branch`. Capture scans every worktree, so writing from a task branch would put the same session file on two branches with different contents and conflict at merge. Elsewhere, `tiller usage` refuses with a reason that names the integration branch, and `sync` skips capture with a one-line note. With no integration branch configured, capture writes wherever it runs.

**Only sessions since the ledger began.** Capture ignores any session whose first call is earlier than the project's earliest event. Agent work from before `tiller init`, as in a brownfield adoption, is not Tiller work and would only inflate "Not tied to a work item." A baseline for JC's cache-read hypothesis would come from comparing against non-Tiller repositories, which is out of scope here.

### Finding a project's sessions

A reader finds the sessions belonging to this project by working directory: the checkout root, every path in `git worktree list`, and any path beneath them. Each harness encodes the directory differently (Claude Code replaces `/` and `.` with `-` in the project folder name; OhMyPi and Pi use a home-relative or full encoded name; Codex records the working directory inside each file), and the reader owns that knowledge. A worktree that has since been removed is still found when it lived under the checkout root (`.worktrees/`, `.claude/worktrees/`) because its encoded name starts with the root's. A removed worktree outside the root is found only if capture ran while it existed, which is one more reason capture rides `sync`.

### Capture

Two ways in:

- **`tiller usage`** reads this machine's harness logs for this project, writes or grows the session files, and prints what it did: sessions read per harness, tokens captured, the share tied to a work item, and any harness folder it looked for and did not find. `--json` gives the same facts. It is safe to run any time and as often as wanted.
- **`tiller sync`** runs the same capture after its own work, best effort, when capture is enabled. A capture failure is a warning in sync's output, never a sync failure, and never changes what sync records. The written files join sync's other state changes, which the agent or human commits as usual.

No skill or orientation text changes, so this adds no orientation generation bump. The builder should confirm that sync's existing guidance already leads to its state changes being committed.

### Reading outside the repo

Tiller otherwise reads nothing outside the repo except the launcher's binary cache. JC ruled (2026-10-26) that capture is a second, bounded exception: it reads harness session logs under the user's home directory (`~/.claude/projects`, `~/.codex/sessions`, `~/.omp/agent/sessions`, `~/.pi/agent/sessions`, honoring each harness's own documented override such as `CODEX_HOME` or `CLAUDE_CONFIG_DIR`), read-only, and only the sessions whose working directory is this checkout or one of its worktrees. It never writes there, never reads harness config, and never reads another project's sessions. The exception is written into [distribution.md](../spec/distribution.md) beside the binary-cache one, in the same terms.

### Turning it on mid-project

Most projects, CALCTP first, will turn capture on after work has begun. The first capture on each machine backfills every session still on disk that started on or after the ledger's first event, credited against the current tree like any other session. Each engineer's history arrives only when that engineer captures (new binary, a checkout on the integration branch, then a `sync` or `tiller usage`), and the page cannot show who has not, since that would put a person on it; the team should turn it on together and each run `tiller usage` once. The first `sync` captures what fits its time budget and `tiller usage` finishes the rest. Older binaries decode `tiller.yml` loosely and ignore the `usage:` block, so turning it on needs no schema bump or migration; an older binary simply does not capture. Because Claude Code keeps 30 days by default, every day capture stays off loses history for good, and the partial coverage rule under the Report page keeps what was lost from reading as low usage.

### Turning it on

Capture is a project setting in `tiller.yml`, so the team decides together and every checkout behaves the same:

```yaml
usage:
  capture: true
```

It is **off by default** (JC, 2026-10-26). A new project turns it on at scaffold time with `tiller init --capture-usage`, which writes the block above; without the flag, `init` writes nothing about usage and capture is off. An existing project turns it on by adding the block to `tiller.yml`. When capture is off, `sync` never captures, and `tiller usage` refuses with a reason that names the setting, so nobody's logs are read without the team having chosen it. The Report's Agent use tab renders whatever usage files exist either way, and its empty state is the same.

### The Report page

The header stays as it is. Under it, two tabs using the console's existing tab rail (`NodeTabs`, given a `label` such as "Report view", as Home does): **Project** (the default) and **Agent use**. The selected tab lives on the URL as `?tab=agent-use`; an unknown value reads as Project. The Project tab is today's KPI strip and five charts, unchanged. The page's existing rules apply to the new tab: metrics only, no node ids, keys, status names, or command names on the page, no contributors in the payload, and series never told apart by color alone.

The Agent use tab reuses the Project tab's parts, not lookalikes: `KpiStrip`'s tile, `ChartCard` and `ChartFrame` (including the enlarge dialog), `ChartTooltip`, the legend and swatch classes, the shared week axis and its partial-current-week treatment, the empty-state copy idiom, and the console's tokens for color and type. Its own layout follows the Project tab's grammar of full-width charts and side-by-side pairs.

**A note above the KPI strip** names each harness measured and the date of its earliest captured call, for example "Measured: Claude Code since Sep 30 · Pi since Sep 1 · Codex since Sep 8." The page cannot know about harnesses it has no files for, so it names what it measured rather than claiming completeness.

**Partial coverage.** Harnesses keep their logs for different lengths of time: Claude Code deletes transcripts after 30 days by default, while Pi, OhMyPi, and Codex kept CALCTP's for months. A project that turns capture on mid-flight therefore backfills a lopsided history. On CALCTP, Claude Code (77 percent of tokens) reaches back only to Sep 30 while Pi reaches Sep 1, so its September would read as a quiet month when it was not. To keep "unknown, never zero" honest, the report computes a **full-coverage date**: the latest of the per-harness earliest-call dates, read from the committed files (the earliest `first_call` per harness). Then:

- On the weekly charts, weeks wholly before the earliest harness date are not measured, and weeks from there up to the full-coverage date are **partially measured**: drawn with the hatch the page uses for the partial current week and labeled "Partly measured" in the legend and tooltip.
- The per-feature chart plots only features whose first start is on or after the full-coverage date, and the per-work-item figures (the median tile and the histogram) count only items whose first start is on or after it, so older work never looks artificially cheap. The note beneath each says how many earlier items were left out.
- The totals in the KPI strip count everything captured, and the note above the strip makes the per-harness dates visible.

The report cannot tell a harness that deleted its logs from a harness the team adopted later; both read as partial coverage before that harness's first call. That understates confidence rather than overstating it, which is the right direction.

**The KPI strip**, five tiles to match the Project tab: Total tokens; Cache reads, with its share of all tokens beneath; Fresh input and cache writes; Output; Median per work item, with "N items · X% of tokens tied to a work item" beneath.

**What "per work item" covers**, in the KPI tile and the histogram: done tasks, bugs, chores, and tweaks that have usage records of their own and whose first start is on or after the full-coverage date. In-progress items would pull the figures down with partial totals, so they are left out. A bug or chore carried by another item's merge counts only if it has its own usage; usage spent on it under the carrying item stays with that item. Features and epics are not work items for these two figures; features have their own chart.

**The charts**, in this order:

1. **Cache reads per week** and **Fresh tokens per week**, side by side. Two charts on their own scales, because cache reads run about thirty times everything else and one axis would flatten the rest. The fresh chart stacks fresh input, cache writes, and output. Weeks before capture began are drawn as not measured, and weeks before the full-coverage date as partially measured, under the partial coverage rule above; neither is ever drawn as zero.
2. **Tokens by stage** and **Model by stage**, side by side. The first is a horizontal bar per stage with its total and, in a right-hand column, its cache-read share. The second is a 100 percent bar per stage split by model, showing the top four models by total tokens and "Other." Model colors follow the model, assigned once by overall total and the same in every chart; segments wide enough carry a percent label, and the legend names every model, so hue is never the only cue (JC is color blind).
3. **Tokens per finished feature**, full width on the shared week axis: one dot per completed feature at its completion date, dated the way the lead and cycle chart dates completion, with a solid rolling median over the last five features. A feature's total is its own tokens plus its tasks'. The y axis is logarithmic and labeled as such, because feature totals span three orders of magnitude. Hover names the feature and its total, as the lead and cycle chart does; the title appears nowhere else.
4. **Tokens per work item**, full width: a histogram of tasks, bugs, chores, and tweaks by size band (under 100K, 100K to 300K, 300K to 1M, 1M to 3M, 3M to 10M, 10M to 30M, 30M to 100M, over 100M), with the median and count beneath. It shows outliers without naming them. Items with no usage records are left out and counted in the note beneath, never placed in the lowest band.

Empty states: with no usage files at all, the tab reads "No agent usage recorded yet" and draws nothing else. Each chart has its own empty state in the Project tab's style.

### Serving it

`GET /api/report/usage` serves the tab and is a thin shell over one views function, `views.UsageReportOf`, which is the only place a usage metric is defined, as `views.ReportOf` is for the Project tab. It reads `.tiller/usage/`, the current tree for node kinds and parentage, and the event stream through the process event cache for completion dates. It walks no Git history and never writes. The payload is ready-to-draw series on the same week axis the Project payload uses (`weeks` and `week_boundaries`). The tab fetches it only when selected. The target is the Project report's speed on CALCTP: well under a second cold.

## Performance

JC's constraint: no major performance hit anywhere. The logs are large (CALCTP's are 1.6 GB across 900-odd files), the output is small (CALCTP's 91 top-level sessions since its ledger reset come to about 2,400 rows, roughly half a megabyte of YAML over seven weeks), and the work happens on commands people run constantly. These are requirements, each with a test or a measurement in the build.

**Capture is incremental.** A gitignored cursor in `.tiller/.cache/usage.yml` records, per log file, its size, modification time, and the project match already decided for it (Codex needs a first-line read of every rollout file to learn its working directory; that read happens once per file, ever). A log whose size and modification time are unchanged is not opened. A grown log is reparsed in full, which keeps the replay simple and correct; session logs are tens of megabytes at most. Budgets, measured on CALCTP-sized data: a capture with nothing new finishes in under 100 ms; a first full capture finishes in under 10 seconds.

**Parsing streams.** Logs are read line by line with a bounded buffer and a cheap substring test before any JSON decode, so only assistant lines carrying usage or tool calls are decoded (the spike decoded about one line in ten this way). Oversized lines, such as a tool result of several megabytes, are skipped rather than buffered whole.

**The lock is held only to write.** Discovery and parsing run without the mutation lock. Capture takes the lock only for the final writes, so concurrent Tiller commands are never stalled behind a log scan.

**Sync stays fast.** Sync's built-in capture works from the cursor and has a time budget of 2 seconds. When the budget runs out (a first capture on a big backlog), it stops at a session boundary, keeps what it wrote, and says in one line that `tiller usage` finishes the rest. Sync itself is never slowed by more than the budget, and a sync with no new settled sessions adds only the cursor check.

**Nothing else walks the usage files.** `.tiller/usage/` is excluded from keyword search and its corpus, the drift sweep, node inventory walks, and the event cache. Code identity already excludes all of `.tiller/`. A new walker over `.tiller/` must state whether it includes `usage/`, and a test pins the exclusions.

**The Project tab is untouched.** `/api/report` reads no usage file and its payload does not change. The Agent use tab's own endpoint is fetched only when the tab is selected. `views.UsageReportOf` caches its parsed files keyed on the source generation token, as the console's other reads do. Budgets on CALCTP: under 500 ms cold and under 150 ms warm.

## What changes where

- **artifact-schema.md**: `.tiller/usage/` in the directory layout; the usage file format, its privacy rule (no contributor, no content), the never-shrink, settled-session, integration-branch, and since-the-ledger rules, and the fact that capture records no event.
- **cli-surface.md** and **docs/tiller-commands.md** (the site's CLI page is generated from it): `tiller usage`; sync's best-effort, time-budgeted capture; `tiller init --capture-usage`.
- **ui-surface.md**: the Report section gains the two tabs and an "Agent use" subsection; the page-wide rules (metrics only, no contributors, no color-only series) now state that they cover both tabs.
- **distribution.md**: the second self-contained exception, the harness logs, in the bounded terms above.
- **roadmap.md**: built, plus the next readers (OpenCode, Gemini CLI) as not built.
- **artifact-schema.md, root manifest**: the `usage.capture` setting, off when absent; and the `.tiller/.cache/usage.yml` cursor in the directory layout.

## Left to the builder

- Package layout for the readers and the shared replay. One reader per harness behind a small interface, with the marker table and replay shared, is the shape; the names are the builder's.
- The exact `tiller usage` output wording and JSON field names, following the CLI's existing conventions.
- Test fixtures: small synthetic session logs per harness under `testdata/`, never copies of real transcripts.
- Chart geometry, tick choices, and tooltip copy, within the Project tab's existing system.
- How the stage display labels map to internal keys.

## Decisions settled so far

- **Aggregate, never personal.** Usage is summed across the team through committed files; no metric is attached to a person. (JC)
- **Raw tokens, cache reads split out.** No cost weighting and no dollars. (JC)
- **Client-visible.** Agent use lives on the Report page, in its own tab after Project. (JC)
- **A reader per harness, owned by Tiller, in Go.** No outside tool to install; ccusage and tokscale serve only as format references. (JC agreed to the shape.)
- **Credit by replaying Tiller commands within each session,** with subagents inheriting their parent's state. Validated by the CALCTP spike.
- **One committed file per session, never shrunk.** (Builder's call, made in design.)
- **Capture by `tiller usage` and best effort in `sync`.** (Builder's call, made in design.)
- **Reading harness logs is a bounded exception to the self-contained principle.** Read-only, this project's sessions only. (JC)
- **Off by default; `tiller init --capture-usage` turns it on.** Existing projects add `usage: { capture: true }` to `tiller.yml`. (JC)
- **No major performance hit.** Incremental capture from a cache cursor, lock held only to write, a 2-second budget inside sync, usage files invisible to every other walker, and a separate lazily fetched endpoint. (JC's constraint; the mechanisms are design calls.)
- **Partial coverage is shown, never read as low usage.** Per-harness earliest-call dates in the note, a full-coverage date (the latest of them), partly measured weeks hatched before it, and item-level charts limited to work begun after it. (JC, 2026-10-26)
- **Nine display stages.** (Builder's call, made in design.)
- **Capture records no event, needs no identity.** (Builder's call, made in design.)
- **Capture skips sessions active in the last 30 minutes, writes only on the integration branch, and ignores sessions from before the ledger began.** (Builder's calls, made in design.)
- **"Per work item" means done leaf items with their own usage.** (Builder's call, made in design.)

## What's next

An implementation plan, once JC approves this design.
