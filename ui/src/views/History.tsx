// History (renamed from Recap 2026-08-10) is the recency
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
// visit rather than blanking the page. Viewing never advances the last-visit
// mark on any window (serve/home.go); only the visit window offers the
// `Mark caught up` button (history/CatchUp.tsx), so /history?window=7d is a
// link with no button and no cost.
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
// PHASE 5 (this pass): the console-v2 mock (the retired console mock) draws
// History as a flat day-grouped timeline. The served payload and the spec group
// by (epic, stream) instead, and that reconciliation was made deliberately, not
// by this file: option (a), keep the served groups as sections and restyle
// their rows, with day headings within each group.
// So a RecapGroupBlock renders its section head (restyled,
// not removed) and then a `.day`/`.day-h` heading for every calendar day its
// bursts touch (a one-day group included), each holding that day's own bursts
// (dayGroupsOf, and its own doc comment, states the exact rule).
//
// The row itself is the `.hrow` anatomy: the clock alone (the day lives in the
// heading), the bold verb, the title with its key after it, and the actor
// right-aligned. A run of several reads the clock, then one label spanning the
// verb and title columns. A burst of more than one expands in place, with the mock's own "+ open" / "– close" wording
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
// exists to do so WindowLink and the group seg can never drift apart on it.
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
// timeline: a digest long enough for a
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
import type { Recap as RecapModel, RecapGroup } from "../api";
import { recapPath } from "../api";
import Time from "../components/Time";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import type { Burst } from "../lib/bursts";
import { burstsOf } from "../lib/bursts";
import EmptyNote from "../components/EmptyNote";
import { PersonName } from "../people";
import { TieredBursts } from "./history/BookkeepingRow";
import BurstRow from "./history/BurstRow";
import {
  HistoryControls,
  WINDOWS,
  WindowLink,
  clearedFilters,
  historyPath,
  viewOf,
  type HistoryView,
} from "./history/controls";
import CatchUp from "./history/CatchUp";
import { countFor, emptyTextFor, fixedSubFor } from "./history/copy";
import { burstAt, dayGroupsOf } from "./history/days";
import { filterRecap, historyFacets, type HistoryFilter } from "./history/filter";
import RecapGroupBlock from "./history/RecapGroupBlock";
import "./history.css";

// TASK 23 (this pass): three client-side filters, Actor, Kind, and Epic, over
// the recap already fetched (history/filter.ts). They ride ?actor=, ?kind=,
// ?epic= like the other params, and the order of operations is fixed: the
// filter runs over the fetched recap, then the milestone tier folds the
// bookkeeping left in what remains, so a filter works in both Milestones and
// Everything. A value the fetched recap does not hold is ignored (the select
// shows its no-filter option) rather than blanking the page; the controls
// themselves live in history/controls.tsx.

export default function History() {
  useDocumentTitle(titleOf("History"));
  const [params] = useSearchParams();
  const view = viewOf(params);
  // Viewing never moves the horizon, on any window: this read carries no visit
  // marker. Only the CatchUp button's own one-shot press does.
  const { data, error, loading } = useFetch<RecapModel>(recapPath(view.window));
  // Which horizon the reader pressed Mark caught up against. Held here, above
  // the fetch, so a refresh or a window round trip on this page visit cannot
  // put the button back over a horizon that already moved: the confirmation
  // lasts while the served horizon is still the pressed one.
  const [caughtUpFrom, setCaughtUpFrom] = useState<string | null>(null);
  const facets = data !== null ? historyFacets(data) : null;
  // Only a value this recap holds filters it; anything else is ignored.
  const valid: HistoryFilter = {
    actor: facets?.actors.includes(view.actor ?? "") ? view.actor : undefined,
    kind: facets?.kinds.includes(view.kind ?? "") ? view.kind : undefined,
    epic: facets?.epics.some((e) => e.id === view.epic) ? view.epic : undefined,
  };

  return (
    <section className="history-page">
      <div className="history-page-head">
        <h1>History</h1>
        <HistoryControls view={view} facets={facets} />
      </div>
      {error !== null && <p className="error">{error}</p>}
      {loading && <p className="muted">Loading history...</p>}
      {data !== null && (
        <RecapBody
          recap={data}
          filter={valid}
          view={view}
          caughtUp={caughtUpFrom === data.since}
          onCaughtUp={() => setCaughtUpFrom(data.since)}
        />
      )}
    </section>
  );
}

function RecapBody({
  recap: fetched,
  filter,
  view,
  caughtUp,
  onCaughtUp,
}: {
  recap: RecapModel;
  filter: HistoryFilter;
  view: HistoryView;
  caughtUp: boolean;
  onCaughtUp: () => void;
}) {
  const { window: active, group: groupMode, showAll } = view;
  const filterOn = filter.actor !== undefined || filter.kind !== undefined || filter.epic !== undefined;
  const recap = filterRecap(fetched, filter);
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
          tell the truth about ITSELF (copy.ts's fixedSubFor), and the visit
          window states the count and its horizon. Unconditional, the same way
          .recap-since always was, because the horizon matters on an empty
          page as much as a full one. */}
      {/* When nothing is new since the last visit the head is only the
          empty note below ("Nothing new since your last visit."), never a
          "0 events since ..." line above it. */}
      {!(active === "visit" && fetched.total === 0) && (
        <p className="hist-sub muted">
          {active === "visit" ? (
            <>
              {countFor(recap, filterOn ? recap.shown : undefined)}
              {/* Empty on a first visit, which recaps nothing: a blank "since"
                  would read as a claim about an instant nobody has. "Since"
                  takes a point in time, so the horizon reads as a clock or a
                  dated clock, never as an age ("since 3h ago"). */}
              {recap.since !== "" && (
                <>
                  {" since "}
                  <Time iso={recap.since} mode="point" />
                </>
              )}
            </>
          ) : (
            fixedSubFor(active, recap, filterOn ? recap.shown : undefined)
          )}
        </p>
      )}
      {/* Nothing to catch up on when the horizon exists and no event sits
          behind it; a first visit (no horizon yet) keeps the button, since
          the press is the only thing that gives it one. */}
      {active === "visit" && (caughtUp || fetched.since === "" || fetched.total > 0) && (
        <CatchUp through={fetched.through} done={caughtUp} onDone={onCaughtUp} />
      )}
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
      {recap.groups.length === 0 && filterOn && fetched.groups.length > 0 ? (
        <FilterEmpty view={view} />
      ) : recap.groups.length === 0 ? (
        <RecapEmpty view={view} />
      ) : groupMode === "time" ? (
        <div className="recap-flat">
          <FlatBody recap={recap} showAll={showAll} />
        </div>
      ) : (
        <div className="recap-groups">
          {recap.groups.map((g) => (
            <RecapGroupBlock
              key={`${g.epic === null ? "" : g.epic.id}:${g.stream}`}
              group={g}
              showStream={oneStream === ""}
              showAll={showAll}
            />
          ))}
        </div>
      )}
    </>
  );
}

// An empty window is an ordinary state, so it hands over the escape hatch
// rather than dead-ending, with live links to every window WIDER than the one
// that came up empty.
// A filter that matches nothing is not an empty window: say so plainly and
// hand over the way back.
function FilterEmpty({ view }: { view: HistoryView }) {
  return (
    <div className="recap-empty">
      <EmptyNote>No events match the filters.</EmptyNote>
      <p className="recap-empty-more">
        <Link to={historyPath(clearedFilters(view))} replace>
          Clear filters
        </Link>
      </p>
    </div>
  );
}

function RecapEmpty({ view }: { view: HistoryView }) {
  const active = view.window;
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
              <WindowLink key={w} target={w} view={view} />
            ))}
          </span>
        </p>
      )}
    </div>
  );
}



// One burst plus the served group it came from, the unit flat mode merges
// across groups. Carrying the ORIGIN alongside the burst rather than folding
// it into Burst itself keeps lib/bursts.ts's own type untouched (task-21-
// brief.md's bound) and keeps epic mode's own Burst[] plumbing exactly as it
// was.
interface FlatBurst {
  burst: Burst;
  // Index of the served group the burst came from. A bookkeeping fold never
  // crosses it, even for two groups of one epic (different streams).
  group: number;
  // The stripping prefix EntryAddresses uses for this burst's leaf-key
  // addresses. Flat mode keeps the same expanded-row address behavior and
  // adds origin because the merged row has no section head to supply it.
  groupKey: string;
  // The quiet mono label a merged row shows in place of the section head it
  // lost: an epic's own key, or "Project" for the epic-less cell - the exact
  // fallback text RecapGroupBlock's own head already uses, so a reader who
  // has seen epic mode recognizes the word.
  origin: string;
}

// The flat mode's own composition: burstsOf still runs once PER SERVED
// GROUP, unchanged (the anti-merge rule - two epics'
// same-type-same-actor runs can never fold into one row just because the
// reader asked for time order), and this only merges and orders the
// resulting ROWS. Newest first, by each burst's own instant (burstAt);
// Array.prototype.sort is stable, so two bursts that land on the exact same
// instant keep the API's own group order between them (alphabetical by epic
// key, project last) rather than an arbitrary one.
function flatBurstsOf(groups: RecapGroup[]): FlatBurst[] {
  const out: FlatBurst[] = [];
  for (const [group, g] of groups.entries()) {
    const groupKey = g.epic === null ? "" : g.epic.key;
    const origin = g.epic === null ? "Project" : g.epic.key;
    for (const burst of burstsOf(g.entries)) {
      out.push({ burst, group, groupKey, origin });
    }
  }
  out.sort((a, b) => burstTimeMs(b.burst) - burstTimeMs(a.burst));
  return out;
}

// Milliseconds for sort purposes only (display still goes through Time,
// never this). An unparseable timestamp sorts as the oldest
// possible instant rather than throwing off the whole list around a NaN
// comparison - a defensive fallback for malformed data the server does not
// actually send, mirrored from dayKey's own "give it a bucket, do not crash"
// judgment one level up.
function burstTimeMs(burst: Burst): number {
  const t = new Date(burstAt(burst)).getTime();
  return Number.isNaN(t) ? -Infinity : t;
}

// Time mode's own body: just the merged, day-bucketed timeline. The
// aggregate cap line used to render at its foot; it moved up
// to RecapBody's own FlatDroppedNote, under the page head, so this component
// is back to doing exactly one job.
function FlatBody({ recap, showAll }: { recap: RecapModel; showAll: boolean }) {
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
            {/* Index keys are safe here for the same reason RecapGroupBlock's
                own burst list uses them: the whole list is rebuilt wholesale
                from a refetched payload, never mutated in place. The merged
                items are tiered directly; runKey ends a bookkeeping fold
                where the served group changes. */}
            <TieredBursts
              bursts={d.bursts}
              showAll={showAll}
              burstOf={(ob) => ob.burst}
              runKey={(ob) => ob.group}
              originOf={(ob) => ob.origin}
              renderBurst={(ob, i) => (
                <BurstRow key={i} burst={ob.burst} groupKey={ob.groupKey} origin={ob.origin} />
              )}
            />
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
