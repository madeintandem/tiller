// History (renamed from Recap 2026-08-10, task-6-brief.md item 3; ui-
// surface.md still calls this lens "Recap" and the wire below deliberately
// kept that name too - see api.ts's recapPath/RecapWindow) is the recency
// lens: a mechanical render of /api/recap, the digest of Tiller events since
// your last visit, with a window override (visit, 24h, 7d) for the
// Monday-morning case. Every fact here is lifted straight from the recorded
// event, never narrated into a sentence the read model does not carry
// (ui-surface.md's "mechanical, never narrative").
//
// Only the view's own boundary took the rename: this file, its css file, and
// the root .history-page class the mock's own History view names. Everything
// beneath that boundary - RecapBody, RecapGroupBlock, RecapEmpty, the
// recap-* css classes below .history-page - keeps operating on data the wire
// still calls "recap" (RecapEntry, RecapGroup, RecapWindow), so it keeps that
// vocabulary too rather than drifting from the types it renders.
//
// It is a top-level view rather than a stripe in Home's rail because it does
// not size like a stripe: one decomposition run opens dozens of questions in a
// minute, and the rail it used to live in was 464px wide while the decision
// column beside it already listed the same asks.
//
// The window rides the URL (?window=), the same rule Explore's mode follows,
// so a window a human can switch to is a window they can link to. Absent means
// visit, which keeps /history canonical, and an unrecognized value degrades to
// visit rather than blanking the page. This has a horizon consequence that is
// a feature: only the visit window advances the last-visit mark (serve/
// home.go), so /history?window=7d is a link that consumes nothing.
//
// THE PAGE'S GRAMMAR. The digest is not a list of events, it is a handful of
// facts repeated: the live Acme 7d window is 167 events, one owner, two
// minutes, four bursts. Rendered one row per event it printed the type, the
// timestamp and the owner 167 times each to carry 167 subjects, and its 32
// add.question rows truncated mid-word in the rail. So:
//
//   - Consecutive same-type entries collapse into ONE counted row (lib/
//     bursts.ts), which promotes those three repeated columns into the row's
//     subject rather than hiding them. The row expands in place to its
//     entries, DecisionCard's grammar, so nothing collapsed is unreachable.
//     Count is still the chart it always was - a reader still learns "8" or
//     "1" first - it just rides folded into the row's own bold phrase now
//     ("8 features added") rather than a separate numeral column, which is
//     the phase-5 restyle's own row anatomy (see BurstRow below).
//   - Nothing is hoisted. The asks the rail used to lift above the groups were
//     the SAME ids the answer-questions card lists on Home, measured exact,
//     and a hoisted ask carried no answer command so the reader went to Home
//     anyway, so the recap saying it too was one actionable set in two
//     grammars. What replaces it: the ask count rides the page head and the
//     burst rows, in magenta, as an EVENT count, and the live number lives
//     behind the "Needs you" link.
//
//     That link points at the queue, it does not promise the queue holds every
//     counted event, and the difference is real rather than pedantic. The two
//     agree on the common cases: an open question is an answer_question row, a
//     specced feature an approve_feature row, a proposed task its feature's
//     approve_tasks row. They part on an in_review arrival, which QueueOf turns
//     into an attest_gate row only when the manifest declares an
//     attestation-backed optional gate on that noun inside git.pr_grain and it
//     is not already passing. A tree with no optional gates can therefore hold
//     an ask event the queue has no row for. Home still names that node, in the
//     all-clear block's in-review enumeration, which is the surface whose whole
//     job is the honest version of "nothing needs you".
//   - Past tense throughout. The rail said "32 need you", a present-tense
//     claim about state made from event data; on a page beside a queue that
//     makes present-tense claims for real, the two contradict each other the
//     moment one question is answered. The recap says "32 asked you". The same
//     reasoning is why a burst row never paints a colored status dot the way a
//     live node's StateDot does (lib/states.ts): a RecapEntry carries no
//     status field, and coloring one from the event's own verb would claim a
//     CURRENT state event data cannot back up (an event that read "in review"
//     an hour ago says nothing about whether it still is). home/Pulse.tsx's own
//     JustHappenedRow already made this call for the identical data (see its
//     "No status dot" note); the dot this restyle draws instead marks ask,
//     which the entry genuinely and permanently carries.
//   - Groups open by default. This is what the extra real estate buys: closed,
//     a group head says "form-engine, 17 events", a number and nothing else;
//     open, it says "8 features linked / 8 features added / 1 epic added",
//     which is the chart. The chevron stays so a reader can quiet a lane.
//
// Group ordering is the API's, unchanged: alphabetical by epic key, project
// group last. Recency ordering would make the page one time axis and was
// rejected: it costs a spec change and a page that reshuffles every visit, and
// it buys nothing once the page is 42 rows.
//
// PHASE 5 (this pass): the console-v2 mock (docs/plans/2026-08-09-console-v2-
// mocks.html:1132-1177) draws History as a flat day-grouped timeline. The
// served payload and the spec group by (epic, stream) instead, and that
// reconciliation was made deliberately, not by this file: option (a), keep
// the served groups as sections and restyle their rows, day subheads only
// WITHIN a group where it helps a reader (task-19-brief.md). So a
// RecapGroupBlock renders its section head (restyled, not removed) and then
// either a flat burst list (its bursts all fall on one calendar day) or a
// sequence of `.day`/`.day-h` subheads, one per calendar day its bursts
// touch, each holding that day's own bursts - never both at once, so a
// reader never has to ask which regime a given group is in (dayGroupsOf
// below, and its own doc comment, states the exact rule).
//
// The row itself moved from a numeral-led column grammar to the mock's own
// `.hrow` anatomy: mono time, a decorative dot, the bold "what" (the count
// folded in), the leaf key as a trailing mono ref where one exists, and the
// owner right-aligned. A burst of more than one expands in place exactly as
// before, just with the mock's own "+ open" / "– close" wording
// (`.burst-toggle`) in place of the chevron the row used to draw - the GROUP
// head's own chevron is untouched, since the mock draws no equivalent for it
// to restyle against and it already works.
//
// One honesty gap surfaced doing this restyle, unrelated to the mock: the
// caps rule (api.ts's RecapGroup.omitted doc, "asks are never capped") is a
// server guarantee about the GROUP's own cap, but this view applies a SECOND,
// client-side cap inside an expanded burst (ROW_CAP, "N more"), and that one
// used to be a plain slice - an ask sitting past position ROW_CAP in
// newest-first order rode behind "N more" until a click, which is exactly the
// drop the rule forbids, just one level down. capPreservingAsks fixes it: see
// its own comment.
//
// TASK 21 (this pass): the phase-5 plan reconciled option (a), the served
// (epic, stream) sections above, against option (b), the mock's own flat
// day-grouped timeline, by picking (a) and leaving (b) unbuilt. JC ruled that
// reconciliation with a third option instead: both, as a runtime toggle.
// Group-by-epic stays the default; flat time order is the second mode, and
// the reader picks, live. The mode rides the URL (?group=epic|time), the
// window param's own rule applied a second time - a grouping a human can
// switch to is a grouping they can link to - and the two params compose:
// switching one preserves the other, which is the one job historyPath below
// exists to do so WindowLink and GroupLink can never drift apart on it.
//
// Flat mode is a SECOND COMPOSITION over the same per-group burst output, not
// a new fold rule: burstsOf (lib/bursts.ts) still runs once per served
// (epic, stream) group, exactly as option (a) already does, so two epics'
// same-type-same-actor runs can never merge into one claimed burst just
// because a human asked to see time instead of epic - flatBurstsOf below
// only sorts and day-buckets the resulting ROWS, never their entries. The
// merge sorts by the server's own recorded timestamps, which is mechanical,
// not the forbidden re-sort of the shared order (the plan's own words:
// "treating grouping as the rendering decision the spec already calls it").
// The one thing flat mode has to add that epic mode never needed: the
// section head that used to say "audit-engine" is gone in this mode, so
// BurstRow grows an `origin` prop, a quiet mono label carrying the epic (or
// "project") a merged row came from - the stream half of "epic/stream" needs
// no new slot, since the row's own `.who` already states it unconditionally
// in both modes. dayGroupsOf, previously scoped to one group's own bursts,
// is now generic over anything with a timestamp so both modes share one
// calendar-day walk rather than carrying two copies of it.
//
// Caps hold in both modes without a new rule: the server's per-group cap
// already never omits an ask (api.ts's RecapGroup.omitted doc), and flat mode
// introduces no cap of its own - it renders exactly what survived each
// group's own cap, merged. What it does need to decide is where a capped
// group's own dropped-count says so, now that there is no group head to hang
// it under. Two shapes were both defensible (a quiet line per group at the
// day boundary it happens to fall on, or one aggregate line under the page
// head); this ships the aggregate, UNDER THE HEAD (RecapBody's own
// FlatDroppedNote, beside .hist-sub and .recap-tally), not at the foot of the
// timeline - task-21-review's own correction: a digest long enough for a
// group to get capped is long enough that a line living below the last row
// is a line a reader scanning the top of 127 rows never reaches, which fails
// the caps rule's whole purpose (a reader has to be ABLE to learn truncation
// happened). Epic mode's own per-group line at the foot of one short capped
// section does not license the same placement here: that section is a few
// rows tall and its own foot is still near the fold, which the flat page's
// foot, below every row the digest holds, is not. A group's omitted entries
// are, by construction, OLDER than everything that group served, which is
// also not the same claim as "older than this particular day boundary" once
// rows from every group are interleaved - the day-boundary placement was
// never adopted, but that mismatch is why it was never a serious option
// either.

import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type { Recap as RecapModel, RecapEntry, RecapGroup, RecapWindow, TargetRef } from "../api";
import { recapPath, visitMarker } from "../api";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import type { Burst } from "../lib/bursts";
import { burstPhrase, burstWhen, burstsOf, leafKey } from "../lib/bursts";
import { ROW_CAP } from "../lib/disclosure";
import { has } from "../lib/has";
import EmptyNote from "../components/EmptyNote";
import RefLink from "../components/RefLink";
import { PersonName } from "../people";
import "./history.css";

const WINDOWS: RecapWindow[] = ["visit", "24h", "7d"];

// The grouping mode, client-only (no server param exists, or needs to: the
// payload stays grouped by (epic, stream) either way, and which render mode
// reads it is entirely this file's own decision - task-21-brief.md's own
// bound, "no server changes"). Epic leads the array as the default, the same
// position "visit" holds in WINDOWS above.
type GroupMode = "epic" | "time";
const GROUP_MODES: GroupMode[] = ["epic", "time"];

// The one place window and group compose into an address, so the two seg
// controls (WindowLink, GroupLink) and the empty state's own escape hatch
// can never build a URL that drops the param neither of them owns. Each
// param omits itself at its own default value, so the canonical /history
// (both defaults) stays exactly as clean as it was before this task.
function historyPath(window: RecapWindow, group: GroupMode): string {
  const params = new URLSearchParams();
  if (window !== "visit") {
    params.set("window", window);
  }
  if (group !== "epic") {
    params.set("group", group);
  }
  const search = params.toString();
  return search === "" ? "/history" : `/history?${search}`;
}

export default function History() {
  useDocumentTitle(titleOf("History"));
  const [params] = useSearchParams();
  const raw = params.get("window");
  // Anything that is not a window reads as visit, so a stale or hand-mangled
  // link degrades to the default digest rather than a blank page (Explore.tsx
  // degrades its mode the same way).
  const active: RecapWindow = raw === "24h" || raw === "7d" ? raw : "visit";
  // Same degrade-to-default rule, one param over: garbage or an old link
  // reads as epic, never a blank page.
  const rawGroup = params.get("group");
  const groupMode: GroupMode = rawGroup === "time" ? "time" : "epic";
  // Only the visit window advances the horizon, and only when marked as the
  // console's own visit; a fixed window is a free read either way.
  const { data, error, loading } = useFetch<RecapModel>(recapPath(active), {
    headers: active === "visit" ? visitMarker : undefined,
  });

  return (
    <section className="history-page">
      <div className="history-page-head">
        <h1>History</h1>
        {/* Two segs, not one: grouping is the structural decision (which
            reading order the page runs in), window is the range decision
            (how far back it reads), and they compose independently - task-
            21-brief.md's own settled shape. Grouping leads because it is the
            bigger structural choice; window follows, unchanged from before
            this task. */}
        <div className="hist-controls">
          <div className="seg" role="group" aria-label="History grouping">
            {GROUP_MODES.map((g) => (
              <GroupLink key={g} target={g} current={groupMode} window={active} />
            ))}
          </div>
          {/* The segmented-control idiom the topbar's theme toggle already
              uses (ThemeToggle.tsx, styles.css's .segmented-control): a bordered,
              overflow-hidden strip with a recessed active cell. This one
              stays real <Link>s rather than buttons, because the window
              lives in the URL - a window a human can switch to is a window
              they can link to - and each switch replaces rather than pushes
              (NodeDetail.tsx's tab switch and Build.tsx's panel state follow
              the same replace-not-push house rule): stepping visit -> 24h ->
              7d should not stack three history entries a reader has to walk
              back through to leave the page. */}
          <div className="seg" role="group" aria-label="History window">
            {WINDOWS.map((w) => (
              <WindowLink key={w} target={w} current={active} groupMode={groupMode} />
            ))}
          </div>
        </div>
      </div>
      {error !== null && <p className="error">{error}</p>}
      {loading && <p className="muted">Loading history...</p>}
      {data !== null && <RecapBody recap={data} window={active} groupMode={groupMode} />}
    </section>
  );
}

function WindowLink({
  target,
  current,
  groupMode,
}: {
  target: RecapWindow;
  current: RecapWindow;
  groupMode: GroupMode;
}) {
  const active = target === current;
  return (
    <Link
      to={historyPath(target, groupMode)}
      className={active ? "active" : undefined}
      aria-current={active ? "page" : undefined}
      replace
    >
      {target}
    </Link>
  );
}

// The grouping toggle's own seg, beside the window one. Labeled in the same
// lowercase-mono register the window seg's own values already read in
// ("visit", "24h", "7d") rather than a title-cased "Epic / Time" - the .seg
// CSS uppercases every label anyway (text-transform: uppercase), so the
// casing here is about what a screen reader announces and what a future
// diff reads, not what paints.
function GroupLink({
  target,
  current,
  window: activeWindow,
}: {
  target: GroupMode;
  current: GroupMode;
  window: RecapWindow;
}) {
  const active = target === current;
  return (
    <Link
      to={historyPath(activeWindow, target)}
      className={active ? "active" : undefined}
      aria-current={active ? "page" : undefined}
      replace
    >
      {target === "epic" ? "by epic" : "by time"}
    </Link>
  );
}

function RecapBody({
  recap,
  window: active,
  groupMode,
}: {
  recap: RecapModel;
  window: RecapWindow;
  groupMode: GroupMode;
}) {
  const asks = recap.groups.reduce(
    (n, g) => n + g.entries.filter((e) => e.ask).length,
    0,
  );
  // The stream token is the owner defect one level up: printing "jc" on 11
  // group heads is 11 repetitions instead of 167. A single-stream digest names
  // its owner once, here; a multi-stream one names it per group, because "an
  // agent worked while you were away" is a distinction worth drawing. An
  // unowned group keeps its empty stream in the set, so "all by jc" is only
  // ever claimed when it is true of every group.
  const streams = new Set(recap.groups.map((g) => g.stream));
  const oneStream = streams.size === 1 && !streams.has("") ? [...streams][0] : "";

  return (
    <>
      {/* The mock's own one-liner (mocks.html:1144), adapted per window
          rather than copied verbatim: a fixed window currently open has to
          tell the truth about ITSELF, not recite a sentence written for the
          default view (histSubFor's own comment carries the exact wording
          rule). Unconditional, the same way .recap-since always was, because
          the marking fact matters on an empty page as much as a full one. */}
      <p className="hist-sub muted">{histSubFor(active, recap)}</p>
      {(oneStream !== "" || asks > 0) && (
        <p className="recap-tally">
          {oneStream !== "" && <span className="muted">all by <PersonName id={oneStream} /></span>}
          {asks > 0 && (
            // The only magenta on the page (bar the per-row ask dot, which
            // never carries the fact by color alone either - see BurstRow),
            // and it is an EVENT count in the past tense. The live number is
            // one click away, on the surface that owns present-tense claims.
            <>
              <span className="recap-asks-count">{asks} asked you</span>
              <Link to="/" className="recap-needs-link">
                Needs you →
              </Link>
            </>
          )}
        </p>
      )}
      {/* Flat mode's own aggregate cap line, under the head beside .hist-sub
          and .recap-tally rather than at the foot of the timeline - see the
          top-of-file TASK 21 note (caps paragraph) for why the foot fails
          the caps rule's purpose here. Renders nothing when nothing was
          capped (FlatDroppedNote's own early return), and epic mode never
          reaches it: that mode keeps its existing per-group lines exactly
          where they already were, at each section's own foot. */}
      {groupMode === "time" && <FlatDroppedNote recap={recap} />}
      {recap.groups.length === 0 ? (
        <RecapEmpty window={active} groupMode={groupMode} />
      ) : groupMode === "time" ? (
        <div className="recap-flat">
          <FlatBody recap={recap} />
        </div>
      ) : (
        <div className="recap-groups">
          {recap.groups.map((g) => (
            <RecapGroupBlock
              key={`${g.epic === null ? "" : g.epic.id}:${g.stream}`}
              group={g}
              showStream={oneStream === ""}
            />
          ))}
        </div>
      )}
    </>
  );
}

// The empty state is the steady state, not an edge case: the visit horizon
// advances the moment this page loads, so coming back after the session gap
// finds nothing new. Measured on the live tree, window=visit already returns
// zero groups. So it hands over the escape hatch rather than dead-ending, with
// live links to every window WIDER than the one that came up empty.
function RecapEmpty({ window: active, groupMode }: { window: RecapWindow; groupMode: GroupMode }) {
  const wider = WINDOWS.slice(WINDOWS.indexOf(active) + 1);
  return (
    <div className="recap-empty">
      <EmptyNote>{emptyTextFor(active)}</EmptyNote>
      {wider.length > 0 && (
        <p className="recap-empty-more muted">
          Look back further:
          <span className="seg">
            {/* The escape hatch composes the same way the head's own seg
                does: a click into a wider window keeps whichever grouping
                the reader had open rather than silently resetting it to
                epic. */}
            {wider.map((w) => (
              <WindowLink key={w} target={w} current={active} groupMode={groupMode} />
            ))}
          </span>
        </p>
      )}
    </div>
  );
}

// A group is one (epic, stream) cell of the digest. Epic is null for the
// project-level groups (a question, a dangling id), which the API sorts
// last; those render under a plain "project" label rather than a link.
// The head leads with the epic's TITLE and lets the key follow as muted mono
// (ui-surface.md's console-scoped carve-out), carries the cell's TRUE count
// (what was served plus what the API's cap omitted), and starts open.
function RecapGroupBlock({ group, showStream }: { group: RecapGroup; showStream: boolean }) {
  const [open, setOpen] = useState(true);
  const total = group.entries.length + group.omitted;
  const groupKey = group.epic === null ? "" : group.epic.key;
  const bursts = burstsOf(group.entries);
  const days = dayGroupsOf(bursts, burstAt);

  return (
    // open is bound, not just defaulted: the group starts open, so React would
    // otherwise have to be trusted never to re-assert a static `open` prop on
    // a re-render, which would slam a closed group back open.
    <details
      className="recap-group"
      open={open}
      onToggle={(event) => {
        setOpen(event.currentTarget.open);
      }}
    >
      <summary className="recap-group-head">
        {group.epic !== null ? (
          <>
            <RefLink
              node={{ noun: "epic", id: group.epic.id, ref: group.epic.ref }}
              label={has(group.epic.title) ? group.epic.title : group.epic.key}
              className="recap-group-title"
            />
            <span className="mono muted recap-group-key">{group.epic.key}</span>
          </>
        ) : (
          <span className="recap-group-title muted">project</span>
        )}
        {showStream && group.stream !== "" && (
          <span className="muted recap-group-stream">· <PersonName id={group.stream} /></span>
        )}
        {/* The rule-line: the same trailing hairline the day subheads below
            draw (mock's .day-h::after), so a section head reads as part of
            the same eyebrow family rather than a leftover pre-restyle
            treatment (task-19-brief.md's "restyled to the house eyebrow
            idiom"). Decorative only - the count beside it already carries
            the number a reader scans for. */}
        <span className="head-rule" aria-hidden="true" />
        <span className="recap-group-count muted">
          {total} {total === 1 ? "event" : "events"}
        </span>
        <span className="chevron-glyph" aria-hidden="true">
          {open ? "▾" : "▸"}
        </span>
      </summary>
      {days.length > 1 ? (
        days.map((d) => (
          <div className="day" key={d.key}>
            <div className="day-h">
              <span className="eyebrow">{d.label}</span>
            </div>
            <ul className="recap-bursts">
              {d.bursts.map((b, i) => (
                <BurstRow key={i} burst={b} groupKey={groupKey} />
              ))}
            </ul>
          </div>
        ))
      ) : (
        <ul className="recap-bursts">
          {bursts.map((b, i) => (
            // Index keys are safe here: a burst has no id of its own, the
            // list is rebuilt wholesale from a refetched payload, and its
            // order never changes in place.
            <BurstRow key={i} burst={b} groupKey={groupKey} />
          ))}
        </ul>
      )}
      {group.omitted > 0 && (
        <p className="recap-omitted muted">{group.omitted} older not carried in this digest</p>
      )}
    </details>
  );
}

// A burst's OWN timestamp for bucketing/sorting purposes: entries[0] - the
// newest end, since a burst keeps its entries in the group's own
// newest-first order (lib/bursts.ts). Shared by both modes' own composition
// over dayGroupsOf below, so "which instant does a burst row itself live at"
// is answered in exactly one place.
function burstAt(burst: Burst): string {
  return burst.entries.length > 0 ? burst.entries[0].at : "";
}

// One (calendar day, its own items) bucket, in the order items arrived
// (newest day first, since the payload is one newest-first timeline). Generic
// over T rather than fixed to Burst: epic mode buckets plain Bursts, flat
// mode buckets FlatBursts (a burst plus the origin it merged in from), and
// the bucketing walk itself does not care which - only atOf, supplied by the
// caller, does.
interface DayGroup<T> {
  key: string;
  label: string;
  bursts: T[];
}

// Buckets an already-collapsed, already-ordered item list by the LOCAL
// calendar day atOf reports for each item - not a per-entry split: an item
// that spans several days (a burst whose "span vs instant" case burstWhen
// already renders as "earliest to latest") still reads as ONE row and lives
// under the day its own atOf instant fell on, exactly where a reader
// scanning newest-first would look for it first. Because the source list
// is already newest-first and a day boundary can only be crossed walking
// down it, never re-crossed, one linear walk buckets the whole list; no
// re-sorting - which holds for flat mode's own call site too, since
// flatBurstsOf below does its own sorting BEFORE this function ever runs,
// not the other way around.
//
// The caller decides what an empty-vs-populated result MEANS: RecapGroupBlock
// renders no day subheads at all when this returns a single bucket ("day
// subheads appear within a multi-day group and NOT for a single-day group",
// task-19-brief.md's own stated rendering rule), while FlatBody always
// renders its bucket(s) - time mode has no OTHER section head to fall back
// on, so the day boundary is the page's one structural element and earns its
// place even at n=1 (task-21-brief.md). This function only ever answers "how
// many distinct calendar days did these items touch, and which fell on
// which"; it does not know or care which of those two readings applies.
function dayGroupsOf<T>(items: T[], atOf: (item: T) => string): DayGroup<T>[] {
  const out: DayGroup<T>[] = [];
  for (const item of items) {
    const at = atOf(item);
    const key = dayKey(at);
    const open = out.length > 0 ? out[out.length - 1] : undefined;
    if (open !== undefined && open.key === key) {
      open.bursts.push(item);
      continue;
    }
    out.push({ key, label: dayLabel(at), bursts: [item] });
  }
  return out;
}

// One burst plus the served group it came from, the unit flat mode merges
// across groups. Carrying the ORIGIN alongside the burst rather than folding
// it into Burst itself keeps lib/bursts.ts's own type untouched (task-21-
// brief.md's bound) and keeps epic mode's own Burst[] plumbing exactly as it
// was.
interface FlatBurst {
  burst: Burst;
  // The stripping prefix EntryAddresses uses for this burst's leaf-key
  // addresses. Flat mode keeps the same expanded-row address behavior and
  // adds origin because the merged row has no section head to supply it.
  groupKey: string;
  // The quiet mono label a merged row shows in place of the section head it
  // lost: an epic's own key, or "project" for the epic-less cell - the exact
  // fallback text RecapGroupBlock's own head already uses, so a reader who
  // has seen epic mode recognizes the word.
  origin: string;
}

// The flat mode's own composition: burstsOf still runs once PER SERVED
// GROUP, unchanged (the anti-merge rule task-21-brief.md pins - two epics'
// same-type-same-actor runs can never fold into one row just because the
// reader asked for time order), and this only merges and orders the
// resulting ROWS. Newest first, by each burst's own instant (burstAt);
// Array.prototype.sort is stable, so two bursts that land on the exact same
// instant keep the API's own group order between them (alphabetical by epic
// key, project last) rather than an arbitrary one.
function flatBurstsOf(groups: RecapGroup[]): FlatBurst[] {
  const out: FlatBurst[] = [];
  for (const g of groups) {
    const groupKey = g.epic === null ? "" : g.epic.key;
    const origin = g.epic === null ? "project" : g.epic.key;
    for (const burst of burstsOf(g.entries)) {
      out.push({ burst, groupKey, origin });
    }
  }
  out.sort((a, b) => burstTimeMs(b.burst) - burstTimeMs(a.burst));
  return out;
}

// Milliseconds for sort purposes only (display still goes through burstWhen/
// formatTime, never this). An unparseable timestamp sorts as the oldest
// possible instant rather than throwing off the whole list around a NaN
// comparison - a defensive fallback for malformed data the server does not
// actually send, mirrored from dayKey's own "give it a bucket, do not crash"
// judgment one level up.
function burstTimeMs(burst: Burst): number {
  const t = new Date(burstAt(burst)).getTime();
  return Number.isNaN(t) ? -Infinity : t;
}

// Time mode's own body: just the merged, day-bucketed timeline. The
// aggregate cap line used to render at its foot; task-21-review moved it up
// to RecapBody's own FlatDroppedNote, under the page head, so this component
// is back to doing exactly one job.
function FlatBody({ recap }: { recap: RecapModel }) {
  const flat = flatBurstsOf(recap.groups);
  const days = dayGroupsOf(flat, (ob) => burstAt(ob.burst));

  return (
    <>
      {days.map((d) => (
        <div className="day" key={d.key}>
          <div className="day-h">
            <span className="eyebrow">{d.label}</span>
          </div>
          <ul className="recap-bursts">
            {d.bursts.map((ob, i) => (
              // Index keys are safe here for the same reason RecapGroupBlock's
              // own burst list uses them: the whole list is rebuilt wholesale
              // from a refetched payload, never mutated in place.
              <BurstRow key={i} burst={ob.burst} groupKey={ob.groupKey} origin={ob.origin} />
            ))}
          </ul>
        </div>
      ))}
    </>
  );
}

// The aggregate line for whatever every served group's own cap dropped,
// rendered under the page head (RecapBody, beside .hist-sub/.recap-tally)
// rather than at the foot of the flat timeline it describes. Placement is
// the point: a digest long enough for a group to get capped is long enough
// that a reader scanning the TOP of the page - which is the only part of a
// 127-row digest most visits actually reach - has to be able to learn
// truncation happened, or the caps rule's whole purpose (never let the
// reader lose track of what a cap hid) is unmet. Epic mode's own per-group
// line stays at each section's own foot, unmoved: that foot is a few rows
// below a section head a reader just read, still near the fold, which the
// flat page's own foot, below every row the digest holds, is not - so the
// two placements are not the same call made twice, they are the right call
// made twice for two different geometries. Returns null rather than an empty
// paragraph when nothing was capped, the same "only render a fact that has
// something to say" rule .recap-tally's own conditional follows above it.
function FlatDroppedNote({ recap }: { recap: RecapModel }) {
  const droppedGroups = recap.groups.filter((g) => g.omitted > 0);
  const droppedTotal = droppedGroups.reduce((n, g) => n + g.omitted, 0);
  if (droppedTotal === 0) {
    return null;
  }
  return (
    <p className="recap-omitted muted">
      {droppedTotal} older not carried in this digest, across {droppedGroups.length}{" "}
      {droppedGroups.length === 1 ? "group" : "groups"}
    </p>
  );
}

// A bucketing key, local calendar date only (year-month-day), so two
// timestamps on the same local day always fold together regardless of their
// time-of-day. An unparseable value gets its own key rather than colliding
// with a real day's bucket (mirrors formatTime's own as-is fallback).
function dayKey(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

// The eyebrow text a day subhead reads (mock:1147's own "Today · Sat Aug 9"
// idiom, minus the "Today" special case: that reading depends on the wall
// clock at render time rather than the entries themselves, which is a claim
// this mechanical, never-narrative page does not need to make to be useful -
// "Saturday · Aug 9" already answers "which day was this").
function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  const weekday = d.toLocaleDateString(undefined, { weekday: "long" });
  const monthDay = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return `${weekday} · ${monthDay}`;
}

// One row per burst: the mock's `.hrow` anatomy (mono time, a dot, the bold
// "what", a trailing mono ref where the burst has a single leaf key to show,
// and the owner right-aligned), expanding in place for a run of more than
// one. The count is still always rendered, at every run length including
// one, because it is still the chart (see the top-of-file note): what varies
// at n=1 is that the SUBJECT names its own node instead of a bare count, and
// it has nothing to expand.
//
// The dot never paints a status: a RecapEntry carries none to paint honestly,
// and home/Pulse.tsx's JustHappenedRow already declined to invent one from the
// event's verb for the identical data. What it marks instead is ask, which
// the entry genuinely carries and which never goes stale the way a "current
// status" claim would - lit only when the burst holds one, same as the WORD
// beside it, so the fact is still never carried by color alone.
//
// `origin` is undefined in epic mode (the section head already says it) and
// always set in flat mode (FlatBody), where the row has no section head and
// must carry its epic/"project" label. The stream half is already the row's
// own `.who` below. Origin renders on every lone or multi row; unlike the
// trailing EntryAddresses slot, it is one fact per burst rather than one fact
// per expanded entry.
function BurstRow({
  burst,
  groupKey,
  origin,
}: {
  burst: Burst;
  groupKey: string;
  origin?: string;
}) {
  const [open, setOpen] = useState(false);
  const n = burst.entries.length;
  const phrase = burstPhrase(burst.type, n);
  const lone = n === 1 ? burst.entries[0] : undefined;
  const subject = lone?.targets[0];
  const when = burstWhen(burst.entries, formatTime);
  const hasAsk = burst.asks > 0;

  // The ask clause, written out in WORDS at every run length: a magenta dot
  // alone would carry the fact in color only, and ui-surface.md's own rule is
  // that a state is never color by itself. Its numeral is dropped once every
  // entry in the run asked (the row's own bold "what" already printed the
  // count once, to its left), and kept whenever only SOME did, because then
  // it says something the "what" phrase did not.
  const askClause = hasAsk && (
    <span className="recap-asks-count">
      {burst.asks === n ? "asked you" : `${burst.asks} asked you`}
    </span>
  );

  // The origin slot: reuses the same mono/muted ref treatment
  // .recap-burst-key already paints (task-21-brief.md's own instruction),
  // under its own class since it carries a different fact (which group, not
  // which leaf key) at a different cardinality (every row, not just a lone
  // one).
  const originSpan = origin !== undefined && (
    <span className="mono muted recap-burst-origin">{origin}</span>
  );

  // A run of one is already its own entry, so there is no third level to open
  // and no toggle to draw: it names its node instead of counting one.
  if (n === 1 && lone !== undefined) {
    return (
      <li className="recap-burst">
        <div className="hrow recap-burst-lone">
          <span className="t">{when}</span>
          <span className={hasAsk ? "dot ask" : "dot"} aria-hidden="true" />
          <span className="what">
            {subject !== undefined ? (
              <>
                <RefLink node={subject} label={targetLabel(subject)} className="recap-burst-node" />{" "}
                <b>{phrase.verb}</b>
              </>
            ) : (
              <b>{phrase.subject}</b>
            )}
          </span>
          {askClause}
          <EntryAddresses entry={lone} groupKey={groupKey} className="recap-burst-key" />
          {originSpan}
          <span className="who"><PersonName id={burst.contributor} /></span>
        </div>
      </li>
    );
  }
  return (
    <li className="recap-burst">
      <details
        className="hrow-group"
        open={open}
        onToggle={(event) => {
          setOpen(event.currentTarget.open);
        }}
      >
        <summary className="hrow">
          <span className="t">{when}</span>
          <span className={hasAsk ? "dot ask" : "dot"} aria-hidden="true" />
          <span className="what">
            <b>
              {n} {phrase.subject}
            </b>
          </span>
          {askClause}
          {originSpan}
          <span className="who"><PersonName id={burst.contributor} /></span>
          {/* The mock's own wording (mocks.html:1149), in place of the
              chevron the row used to draw: a run of more than one is the
              only row anatomy with something to open, so it is the only one
              that earns a control here. aria-hidden the same way the
              chevron it replaced was: a native <details> already announces
              its own expanded/collapsed state to assistive tech, so this
              text is a sighted affordance layered on top of semantics the
              element already carries, not new accessible content of its
              own. */}
          <span className="burst-toggle" aria-hidden="true">
            {open ? "– close" : "+ open"}
          </span>
        </summary>
        <div className="burst-body">
          <BurstEntries burst={burst} groupKey={groupKey} />
        </div>
      </details>
    </li>
  );
}

// The third and last level of disclosure, capped with the same "N more" the
// desk's cards use. No commands here: "every command copies" governs the
// commands a surface shows, and the recap shows none, because the actionable
// command is the queue's job and re-importing it is what this move removed.
function BurstEntries({ burst, groupKey }: { burst: Burst; groupKey: string }) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? burst.entries : capPreservingAsks(burst.entries, ROW_CAP);
  const hidden = burst.entries.length - shown.length;
  // The BORDER mark rides individual rows only where the burst is MIXED. In a
  // burst that is all asks the head already said it for every row, and
  // painting the same loud border 32 times is the repetition this pass
  // exists to kill.
  const markAsks = burst.asks > 0 && burst.asks < burst.entries.length;

  return (
    <>
      <ul className="recap-entries">
        {shown.map((e, i) => (
          <li key={i} className={markAsks && e.ask ? "recap-entry recap-entry-ask" : "recap-entry"}>
            {/* The DOT is not gated by markAsks the way the border above it
                is: it is a 6px circle, not a 2px colored border with its own
                negative margin, so 32 identical accent dots do not carry the
                same "same loud mark 32 times" problem the border-gating rule
                exists to avoid. It just states e.ask's own truth on every
                row - accent on every row of an all-ask burst (the head's own
                dot already promised that), muted on every row of a burst
                with none, and the one signal that actually differs
                row-to-row inside a mixed burst. Fixed 2026-08-11 (task-19
                review, finding 2): gating it on markAsks too painted a MUTED
                dot under an ACCENT head on an all-ask burst, the exact
                inversion the dot exists to prevent. */}
            <span className={e.ask ? "dot ask" : "dot"} aria-hidden="true" />
            {e.targets.map((t, j) => (
              <RefLink key={j} node={t} label={targetLabel(t)} className="recap-entry-title" />
            ))}
            <EntryAddresses entry={e} groupKey={groupKey} />
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <button type="button" className="recap-more" onClick={() => setExpanded(true)}>
          {hidden} more {hidden === 1 ? "row" : "rows"} ▸
        </button>
      )}
    </>
  );
}

// The client's own "N more" cap has to honor the same rule the server's own
// per-group cap already does (api.ts's RecapGroup.omitted doc: "asks are
// never capped, so an omitted count is always routine activity") - an entry
// that asked something is never among what a cap hides. A plain
// slice(0, cap) breaks that the moment an ask sits past position `cap` in the
// burst's own newest-first order: it rides behind "N more" until a click,
// which is exactly the drop the rule forbids, one level down from where it
// was already honest. Every ask ships in the initial view; the remaining cap
// slots, if any, fill with the newest routine entries, and the whole result
// re-sorts back to the burst's own newest-first order so nothing displaced
// out of position reads as if it happened later than it did.
function capPreservingAsks(entries: RecapEntry[], cap: number): RecapEntry[] {
  if (entries.length <= cap) {
    return entries;
  }
  const asks = entries.filter((e) => e.ask);
  const routineSlots = Math.max(cap - asks.length, 0);
  const shown = new Set(entries.filter((e) => !e.ask).slice(0, routineSlots));
  for (const e of asks) {
    shown.add(e);
  }
  return entries.filter((e) => shown.has(e));
}

// The trailing address slot: a keyed target contributes its leaf key, while a
// materialized question contributes its Q-id. Other keyless targets contribute
// nothing, and a dangling target already renders its bare ref as the leading
// label, so it is never duplicated here.
function EntryAddresses({
  entry,
  groupKey,
  className = "recap-entry-key",
}: {
  entry: RecapEntry;
  groupKey: string;
  className?: string;
}) {
  const addresses = entry.targets
    .map((target) =>
      has(target.key)
        ? leafKey(target.key, groupKey)
        : target.noun === "question" && has(target.title)
          ? target.id
          : "",
    )
    .filter((address) => address !== "");
  if (addresses.length === 0) {
    return null;
  }
  return <span className={`mono muted ${className}`}>{addresses.join(" · ")}</span>;
}

// A row leads with its title or key. A materialized question's Q-id follows in
// EntryAddresses; keyed nouns likewise trail their leaf key. Only a dangling
// id, which has neither title nor key, falls back to its ref as its sole label.
function targetLabel(t: TargetRef): string {
  if (has(t.title)) {
    return t.title;
  }
  return has(t.key) ? t.key : t.ref;
}

// The one-liner under the head (mock:1144's own line, adapted per window):
// what this window holds, and whether looking at it just marked the reader
// caught up. Only the visit window does (ui-surface.md's horizon rule, pinned
// server-side by internal/serve/visit_test.go's
// TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap): it is the
// one request in the console that advances the last-visit mark, and the two
// fixed windows never touch it no matter how often they are read. Copying the
// mock's own sentence verbatim regardless of which window is open would put
// "reading this view marks you caught up" under a 24h view that just did no
// such thing, so this states the truth for whichever window is actually
// mounted rather than reciting the sentence written for the default one.
function histSubFor(active: RecapWindow, recap: RecapModel): string {
  const count = `${recap.total} ${recap.total === 1 ? "event" : "events"}`;
  if (active === "visit") {
    // since is empty on a first visit, which recaps nothing; a blank "since"
    // would read as a claim about an instant nobody has.
    const since = recap.since !== "" ? ` since ${formatTime(recap.since)}` : "";
    return `${count}${since} · reading this view marks you caught up`;
  }
  const span = active === "24h" ? "in the last 24 hours" : "in the last 7 days";
  return `${count} ${span} · fixed windows never mark you caught up`;
}

// The empty text does not repeat the horizon: the hist-sub line above already
// states that instant, and saying the same fact twice sixty pixels apart is
// the defect the original recap rewrite was about, at the smallest scale it
// has. Keeping the sentence bare also keeps it verbatim identical to Home's
// own recap signal, so the two surfaces stay one sentence in one voice.
function emptyTextFor(window: string): string {
  switch (window) {
    case "24h":
      return "Nothing recorded in the last 24 hours.";
    case "7d":
      return "Nothing recorded in the last 7 days.";
    default:
      return "Nothing new since your last visit.";
  }
}

// Digest timestamps arrive as RFC3339 UTC; the console shows them in the
// reader's local time. Minute precision is also the precision burstWhen
// measures a run's span against, so the two must stay the same function. An
// unparseable value renders as-is, because showing the raw field beats hiding
// it.
function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
