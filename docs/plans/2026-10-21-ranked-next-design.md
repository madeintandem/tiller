# Ranked next

Designed and settled 2026-10-21 through a one-fork-at-a-time review. Not built.

## Problem

Home on a live project opened with a "Route says" strip: one routed move (continue an in-flight task on its branch), an Order note explaining that an earlier task was skipped because an open question held its feature, and the command to answer that question. Beneath it sat an Up next column listing unclaimed tasks in plan order, with no idea who was reading or what they had just done.

Both surfaces answer the wrong question. The route walk (`RouteDecisionOf` in `internal/views/readout.go`) checks sixteen arms in order and returns the first match, so everything it passes over survives only as prose notes. Up next is reader-independent by design (`docs/spec/ui-surface.md`, "Up next"), so it cannot say what this developer will probably do next. And the most common completions, `submit` and `check`, print no next step at all.

The developer wants one answer to "what are my likely next moves," ranked by what they have been working on, the same everywhere: the CLI, Home's Up next, and the readout an agent reads when an action completes.

## What ranked next is

Ranked next enumerates every legal next move for the reading contributor's checkout, the human's and the agent's alike, ranks them by a fixed tiered rule that reads the contributor's own recent events, and returns the top five. It replaces the single-answer route walk: there is one engine, and every surface renders its list.

It refuses to call a language model. Tiller stays a self-contained binary with no API keys, and the console renders with no agent present, so the ranking is a deterministic rule that every surface shares and a reader can check. The agent may say, in its own reply, that it would take a different item first; it never rewrites the list.

It refuses to authorize. A ranked move is a suggestion. Approval, answer, and claim remain explicit commands.

## Decisions settled so far

1. **Tiller ranks, by a fixed rule; the agent may reorder only in its own words.** The CLI, Home, and the readout show one identical list. The skill lets the agent say "given what we just did, I would take the second item first," but the readout block is reproduced unchanged. The list holds the top five, or fewer when fewer exist.
2. **The candidate set is every legal move for this checkout, human decisions included.** It is the route walk's sixteen arms run to exhaustion: each arm contributes every match instead of returning the first. Agent work covers the reader's own ready, in-progress, and check-pending tasks, claimable unclaimed tasks, the reader's shelf work, frontier draft features to specify, shared closeout (sync or closeout phases), derive-baseline, and decompose on an empty tree. Human decisions cover spec, plan, and epic approval, answers, attests, merges of in-review work, and amendment review, each with the command `tiller queue` already carries. Excluded: tasks in flight on another contributor's branch, tasks held by an open question (the holding question is the candidate instead), and work blocked by an unmet dependency. Each candidate carries its actor (human or agent), node, exact command, and a one-line reason.
3. **The ranking reads the event log, overturning #139's "no event-stream reads."** Owner affinity cannot see that the developer just approved a spec or answered a question, which is exactly the signal "what was I just doing" needs. Ranking reads the reader's own events.
4. **Five tiers, in this order, with plan order breaking ties inside each.**
   1. **Unblock nearby.** Human decisions that hold work in the reader's current feature, plus any answerable question overdue or due within two days wherever it sits (decision 8).
   2. **Continue.** The reader's own unfinished work: an in-progress task, or one awaiting its check.
   3. **Nearby.** Other candidates in the current feature, then in its epic.
   4. **Recently touched.** Candidates in other features and epics the reader worked in within the window, most recent first.
   5. **Everything else**, in plan order.

   Unblock leads because those moves are almost always the human's, they cost a minute, and deferring them is what stalls the agent, which keeps continuing on its own anyway. Tier 1 is scoped to the current feature so a busy epic cannot flood it. **The reader's top Continue candidate is always kept**: when tier 1 alone would fill five places, the last place goes to it.
5. **Home ranks for the served checkout's contributor and says so.** The same identity the CLI uses (`TILLER_CONTRIBUTOR`, then `.tiller/identity.yml`) through the server's existing `actingContributor`. The section header names the reader ("Up next · for <contributor>"). With no identity, the personal tiers are skipped, the list falls back to plan order, and a muted line says "Not personalized; run `tiller join`." No "view as" picker. This replaces the reader-independent Up next decision.
6. **One engine; the ranked list replaces the walk.** `RouteDecisionOf` gives way to the ranked list. The Order, Continuity, and Execution notes and the "Next eligible if you're working in parallel" line retire, because what they described (the holding question, the reader's own in-flight branch, the neighbouring feature, the next eligible task) are now candidates. The readout's **Next:** becomes the ranked list. `tiller guide` prints the same list and suggests the skill for the **highest-ranked agent move**, since the top item is often the human's. The orient skill relays the list verbatim, raises any human moves ranked above its pick, and starts the top agent move unless the human redirects. "A suggestion, not authorization" stays.
7. **Up next and Needs you may overlap on Home.** Needs you is the complete inventory of decisions, grouped by kind. Up next is the short ranked answer, mixing human and agent moves. Each Up next row leads with its actor ("You" or "Agent"), then the action and node, then the one-line reason, and links to the node.
8. **Deadlines outrank locality.** An answerable question overdue or due within two days joins tier 1 wherever it sits. Tier 1 sorts by due date first (overdue first), then by questions holding the current feature. Later deadlines get no boost. The kept Continue place still applies.
9. **Completing commands print the list; beginning commands do not.** `approve`, `answer`, `attest`, and `sync` keep their full readout, whose **Next:** is now the list. `submit` and `check` gain a short **Up next** block: the ranked lines only, without headline, verification, or needs-attention. `start`, `claim`, and `resume` print nothing new. `tiller readout <seam>` and `tiller guide` always carry the list. This replaces the decision in `docs/spec/cli-surface.md` that only the four gates carry a readout.
10. **No spreading cap; one row per node.** Locality is the point: inside a feature, its next tasks really are the likely moves, and task order already ranks them. A node never appears twice; it keeps its highest-ranked reason.
11. **`tiller next` is the command.** It prints the ranked five, each with actor, action, node, command, and reason. `--all` prints every legal candidate in ranked order. `--json` gives the payload `/api/next` serves. `--contributor` ranks for another contributor, as the flag does elsewhere. Fewer than five prints what exists. None prints "Caught up: nothing is legal to do right now," with the existing empty-tree hint (decompose, or the unplanned end state) when it applies. Bare `tiller guide` prints the same list then the skill suggestion. Home offers no "show all."
12. **Only deliberate acts count as working on.** Counted event types: claim, start, submit, check, spec, plan, approve, answer, attest, amend, add, link, and reassign or release when the reader is the actor. Excluded: sync, upgrade, migrate, join, owner, and any event naming more than a few targets (a batch add counts once, against its parent feature). A task event counts for its feature and epic, a feature event for itself and its epic, a question event for the node it is attached to. The **current feature** is the feature of the reader's most recent counted event. **Recently touched** is the features from counted events in the last fourteen days, most recent first.

## The engine

A new `views.NextOf(input, reader)` returns a `Next` value: the ranked candidates, the reader it ranked for (empty when unpersonalized), and the caught-up and unplanned flags the walk carries today. It composes three parts.

**Enumerate.** Each of the walk's arms becomes a function returning every candidate it admits, reusing what exists: `ownedExecutionCandidates` and `gen.UnclaimedTaskCandidates` for tasks, `views.QueueOfInput` for human decisions and their commands, `workflow.SharedCloseoutNodes` and `AdviceOf` for closeout, `flowFeatures` and the frontier rule for specify, and the existing baseline and empty-tree checks. Exclusions (other contributors' in-flight work, question holds, unmet dependencies) apply here, and a question hold contributes its question.

**Locate.** A recency reader lists the reader's counted events from the date-partitioned `.tiller/events/` directories inside the fourteen-day window, newest first, without reading older partitions. It yields the current feature, its epic, and the recently-touched features in order.

**Rank.** Candidates are assigned a tier, sorted by tier then by the existing hierarchical execution preference (`gen.SortExecutionTasks` and plan order), deduplicated by node, cut to five with the kept-Continue rule, and each given its reason. Reasons name why the candidate is in its tier: "holds T-x in your current feature," "your task in progress on <branch>," "next in your current feature," "in <feature>, which you worked on <n> days ago," "due tomorrow."

`Next` is the structured result. The CLI renders it as lines; `/api/next` serves it as JSON. Nothing outside `NextOf` ranks.

## Surfaces

**CLI.** `tiller next` (decision 11). `tiller guide` with no argument prints the list, then names the skill for the top agent move with the reason that skill applies. The readout's **Next:** renders the list (decision 9); `submit` and `check` append the short block.

**Console.** `/api/next` replaces `/api/route`, which only the console uses and is removed outright. Home deletes the Route says strip. Dependency diagnostics, which rode the route payload, stay on Home as their own section fed by `/api/next`. Up next renders the ranked five with actor labels and the reader header (decisions 5 and 7). The Recovered reviews tab takes its records only from the node lists, since no route payload carries a recovered review any more. In flight is unchanged.

**Skills and generated text.** The orient skill's "take the first match" and "relay the Order and Continuity notes as written" become decision 6's relay-and-pick rule. The execute skill and every skill that ends on `tiller readout <seam>` keep reproducing the block verbatim; the block now ends on the list. The generated orientation text ("Recorded-state route") is rewritten to match, and the orientation generation bumps.

## Docs and generated text that change

- `docs/spec/cli-surface.md`: the agent readout (which commands carry it, decision 9), the shared route section (arm order, reader-history ranking, the notes, observations) rewritten as ranked next, and a `tiller next` command entry.
- `docs/spec/ui-surface.md`: Home's route strip removed, Up next rewritten (decisions 5, 7), the In flight cross-reference to "the same candidates" updated.
- `docs/spec/skill-set.md`: orient reasoning and the readout anatomy.
- `docs/tiller-commands.md` and the generated site CLI page: `tiller next`, `guide`.
- `internal/skills/orient/SKILL.md`, `internal/skills/execute/SKILL.md`, and any skill quoting the notes.
- `internal/gen/orientation.go`: the route and readout guidance, with the generation bump.
- `docs/project-history.md`: #139's "no event-stream reads" and "Route only" recorded as overturned when this lands.

## Build sequence

1. **Engine and CLI.** The enumerator, the recency reader, the ranker, `tiller next`, `guide` and readout rendering, the `submit` and `check` block, the retired notes, the skill and orientation rewrite, the generation bump, and the CLI and skill-set spec updates. The old walk's tests are rewritten as enumeration and ranking tests; the arm-order cases survive as plan-order tie-break cases.
2. **Console.** `/api/next`, `/api/route` removed, the strip deleted, diagnostics re-seated, Up next rebuilt, the Recovered reviews tab re-sourced, and the UI spec updated.

Both builds happen in a worktree and land on main only when the maintainer says.

## Out of scope

- A language model ranking or re-ranking the list inside Tiller.
- A console "view as" picker for other contributors.
- Ranking other contributors' work for them on a shared console beyond `tiller next --contributor`.
- Weighting by estimated effort, value, or age of the work.
- Persisting the ranking or recording which item was taken.
