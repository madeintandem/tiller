// The desk's header band and its tiles, moved out of Home.tsx whole (issue
// #96): a pure move, so Home.tsx stays the page's fetches and layout. Every
// tile links to the list it counts; the title lives in HomeTitle.tsx.

import { Link } from "react-router-dom";
import { useId } from "react";
import type { FetchState } from "../../hooks";
import type { DriftReportView, ListResult } from "../../api";
import StateBar from "../../components/StateBar";
import { statusLabel } from "../../lib/statusLabel";
import { composeDriftBand } from "./driftBand";
import { composeFunnel, composeShelfStat } from "./funnel";
import { composeStat } from "./stat";
import { UNFILTERED_FIX_PATH } from "../workitems/board";

export default function HeaderBand({
  epics,
  features,
  tasks,
  questions,
  bugs,
  chores,
  tweaks,
  drift,
}: {
  epics: FetchState<ListResult>;
  features: FetchState<ListResult>;
  tasks: FetchState<ListResult>;
  questions: FetchState<ListResult>;
  bugs: FetchState<ListResult>;
  chores: FetchState<ListResult>;
  tweaks: FetchState<ListResult>;
  drift: FetchState<DriftReportView>;
}) {
  return (
    <div className="home-head">
      <div className="stats" role="group" aria-label="Project numbers">
        <StatTile
          label="Epics"
          state={epics}
          to="/explore?mode=filter&noun=epics"
          name={{ one: "epic", many: "epics", opens: "the epic list" }}
        />
        <FunnelStrip features={features} tasks={tasks} />
        <StatTile
          label="Questions"
          state={questions}
          to="/questions?status=all"
          name={{ one: "question", many: "questions", opens: "the questions list" }}
        />
        <ShelfStat bugs={bugs} chores={chores} tweaks={tweaks} />
        <DriftStat state={drift} />
      </div>
    </div>
  );
}

// StaleNote marks a tile that renders from a retained read whose latest
// reread failed: the last good numbers stay, and this says they may be old.
// The visible text carries the error (the title holds it whole), so the
// warning is never color alone.
//
// Inside a link tile the note is the link's description (aria-describedby),
// since the link's aria-label would otherwise replace its content for
// assistive technology and the note would never be announced.
function StaleNote({ error, id }: { error: string; id?: string }) {
  const text = `out of date: ${error}`;
  return (
    <div id={id} className="cap error stat-stale" title={text}>
      {text}
    </div>
  );
}

// FunnelStrip replaces the old Features and Tasks stat tiles: Build's own six
// columns - draft, specced, ready, active, in_review, done - read as shape
// instead of a second StateBar mix, every segment a link to the features list
// at that status (the same displayed status the segment is counted under, so
// the number and the list it opens agree). A zero segment stays in place (dim class) rather than
// disappearing, so the six-column shape never shifts as a project moves
// through it. The caption's feature count is the raw node count (retired
// statuses included), the same total the retired clause explains.
function FunnelStrip({
  features,
  tasks,
}: {
  features: FetchState<ListResult>;
  tasks: FetchState<ListResult>;
}) {
  if (features.data === null) return <UnresolvedTile label="Features" error={features.error} />;
  const { segments, retired } = composeFunnel(features.data.nodes);
  // In flow is every funnel stage but done; retired is cancelled plus
  // archived, composeFunnel's own definition. Done features are neither.
  const inFlow = segments.filter((s) => s.status !== "done").reduce((sum, s) => sum + s.n, 0);
  const taskCount = tasks.data?.nodes.length;
  return (
    <div className="funnel">
      <div className="funnel-row">
        {segments.map((s) => (
          <Link
            key={s.status}
            to={`/explore?mode=filter&noun=features&status=${s.status}`}
            className={s.n === 0 ? "fseg dim" : "fseg"}
            aria-label={`${s.n} ${statusLabel(s.status)} ${s.n === 1 ? "feature" : "features"}, open the list`}
          >
            <div className="n">{s.n}</div>
            <div className="lbl">{statusLabel(s.status)}</div>
          </Link>
        ))}
      </div>
      <div className="cap muted">
        {inFlow} {inFlow === 1 ? "feature" : "features"} in flow
        {retired > 0 && `, ${retired} retired`}
        {taskCount !== undefined && ` · ${taskCount} ${taskCount === 1 ? "task" : "tasks"}`}
      </div>
      {features.error !== null && <StaleNote error={features.error} />}
    </div>
  );
}

// Fix counts nonterminal shelf work and names the per-noun split before
// the reader opens the shared board.
function ShelfStat({
  bugs,
  chores,
  tweaks,
}: {
  bugs: FetchState<ListResult>;
  chores: FetchState<ListResult>;
  tweaks: FetchState<ListResult>;
}) {
  const noteId = useId();
  if (bugs.data === null || chores.data === null || tweaks.data === null) {
    return <UnresolvedTile label="Fix" error={bugs.error ?? chores.error ?? tweaks.error} />;
  }
  const s = composeShelfStat(bugs.data.nodes, chores.data.nodes, tweaks.data.nodes);
  // Three retained reads stand behind one number: any of them failing its
  // reread makes the whole tile possibly old.
  const staleError = bugs.error ?? chores.error ?? tweaks.error;
  return (
    <Link
      to={UNFILTERED_FIX_PATH}
      className="stat shelf"
      aria-label={`${s.open} open Fix items, open the Fix board`}
      aria-describedby={staleError !== null ? noteId : undefined}
    >
      <div className="n">{s.open}</div>
      <div className="lbl">Fix</div>
      <div className="cap muted">
        open: {s.bugs} {s.bugs === 1 ? "bug" : "bugs"} · {s.chores} {s.chores === 1 ? "chore" : "chores"}
        {" · "}{s.tweaks} {s.tweaks === 1 ? "tweak" : "tweaks"}
      </div>
      {staleError !== null && <StaleNote error={staleError} id={noteId} />}
    </Link>
  );
}

// DriftStat is the band's one link-shaped tile (ui-surface.md's header band
// sentence): candidates+uncovered as its number, clickable through to the
// full list, with visible text distinguishing the two counts so a reader can
// never mistake a coverage gap for an actual finding.
//
// A FAILED fetch gets the identical unavailable treatment the four counted
// StatTiles use (UnresolvedTile, reused verbatim rather than a second copy of
// its dash/caption markup), never silence: this slot's only silent case is a
// RESOLVED, genuinely all-zero report (composeDriftBand's own null case).
function DriftStat({ state }: { state: FetchState<DriftReportView> }) {
  const noteId = useId();
  if (state.data === null) {
    return <UnresolvedTile label="Drift" error={state.error} />;
  }
  const band = composeDriftBand(state.data);
  if (band === null) {
    // The one state that stays silent: a RESOLVED sweep that found nothing.
    // A failed reread over that clean report is not silent: the tile falls to
    // the unavailable treatment rather than hiding the failure.
    return state.error === null ? null : <UnresolvedTile label="Drift" error={state.error} />;
  }
  return (
    <Link
      to="/?tab=drift"
      className="stat drift"
      aria-label={`${band.total} drift items (${band.text}), open the drift tab`}
      aria-describedby={state.error !== null ? noteId : undefined}
    >
      <div className="n">{band.total}</div>
      <div className="lbl">Drift</div>
      <div className="cap muted">{band.text}</div>
      {state.error !== null && <StaleNote error={state.error} id={noteId} />}
    </Link>
  );
}

// UnresolvedTile is a stat tile whose list has not arrived: a dim dash where
// the numeral would be, "unavailable" in the error register when the fetch
// failed (the full error rides the title for hover detail), a muted
// "loading..." while it is still in flight. Never a confident 0 for data
// that was not actually read.
function UnresolvedTile({ label, error }: { label: string; error: string | null }) {
  return (
    <div className="stat">
      <div className="n dim">—</div>
      <div className="lbl">{label}</div>
      <StateBar mix={[]} />
      {error !== null ? (
        <div className="cap error" title={error}>
          unavailable
        </div>
      ) : (
        <div className="cap muted">loading...</div>
      )}
    </div>
  );
}

// A counted tile is a link to the list it counts, a zero count included. The
// number and label stay as text; the accessible name says what it opens.
function StatTile({
  label,
  state,
  to,
  name,
}: {
  label: string;
  state: FetchState<ListResult>;
  to: string;
  name: { one: string; many: string; opens: string };
}) {
  const noteId = useId();
  if (state.data === null) {
    return <UnresolvedTile label={label} error={state.error} />;
  }
  const rows = state.data.nodes;
  const { mix, caption } = composeStat(rows);
  return (
    <Link
      to={to}
      className="stat"
      aria-describedby={state.error !== null ? noteId : undefined}
      aria-label={`${rows.length} ${rows.length === 1 ? name.one : name.many}, open ${name.opens}`}
    >
      <div className="n">{rows.length}</div>
      <div className="lbl">{label}</div>
      <StateBar mix={mix} caption={caption} width={96} />
      <div className="cap muted">{rows.length === 0 ? "none yet" : caption}</div>
      {state.error !== null && <StaleNote error={state.error} id={noteId} />}
    </Link>
  );
}
