// The desk's header band and its tiles, moved out of Home.tsx whole (issue
// #96): a pure move, so Home.tsx stays the page's fetches and layout.

import { Link } from "react-router-dom";
import type { FetchState } from "../../hooks";
import type { DriftReportView, ListResult, ProjectSummary } from "../../api";
import StateBar from "../../components/StateBar";
import { PersonName } from "../../people";
import { composeDriftBand } from "./driftBand";
import { composeFunnel, composeShelfStat } from "./funnel";
import { composeStat } from "./stat";

export default function HeaderBand({
  summary,
  epics,
  features,
  tasks,
  questions,
  bugs,
  chores,
  tweaks,
  drift,
}: {
  summary: ProjectSummary;
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
      <div className="home-id">
        <h1>{summary.project.name}</h1>
        <p className="meta muted">
          <span className="mono">{summary.project.key}</span> · {summary.project.kind}
          {summary.contributors.length > 0 && (
            <>
              {" "}
              · Contributors:{" "}
              {summary.contributors.map((o, i) => (
                <span key={o.id}>
                  {i > 0 && ", "}
                  <PersonName id={o.id} name={o.name} />
                </span>
              ))}
            </>
          )}
        </p>
      </div>
      <div className="stats">
        <StatTile label="Epics" state={epics} />
        <FunnelStrip features={features} tasks={tasks} />
        <StatTile label="Questions" state={questions} />
        <ShelfStat bugs={bugs} chores={chores} tweaks={tweaks} />
        <DriftStat state={drift} />
      </div>
    </div>
  );
}

// FunnelStrip replaces the old Features and Tasks stat tiles: Build's own six
// columns - draft, specced, ready, active, in_review, done - read as shape
// instead of a second StateBar mix, every segment a /build link so the strip
// doubles as navigation. A zero segment stays in place (dim class) rather than
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
  const featureCount = features.data.nodes.length;
  const taskCount = tasks.data?.nodes.length;
  return (
    <div className="funnel">
      <div className="funnel-row">
        {segments.map((s) => (
          <Link key={s.status} to="/build" className={s.n === 0 ? "fseg dim" : "fseg"}>
            <div className="n">{s.n}</div>
            <div className="lbl">{s.status === "in_review" ? "in review" : s.status}</div>
          </Link>
        ))}
      </div>
      <div className="cap muted">
        {taskCount !== undefined && `${taskCount} ${taskCount === 1 ? "task" : "tasks"} across `}
        {featureCount} {featureCount === 1 ? "feature" : "features"}
        {retired > 0 && ` · ${retired} retired`}
      </div>
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
  if (bugs.data === null || chores.data === null || tweaks.data === null) {
    return <UnresolvedTile label="Fix" error={bugs.error ?? chores.error ?? tweaks.error} />;
  }
  const s = composeShelfStat(bugs.data.nodes, chores.data.nodes, tweaks.data.nodes);
  return (
    <Link to="/fix" className="stat shelf">
      <div className="n">{s.open}</div>
      <div className="lbl">Fix</div>
      <div className="cap muted">
        {s.bugs} {s.bugs === 1 ? "bug" : "bugs"} · {s.chores} {s.chores === 1 ? "chore" : "chores"}
        {" · "}{s.tweaks} {s.tweaks === 1 ? "tweak" : "tweaks"}
      </div>
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
  if (state.data === null) {
    return <UnresolvedTile label="Drift" error={state.error} />;
  }
  const band = composeDriftBand(state.data);
  if (band === null) {
    // The one state that stays silent: a RESOLVED sweep that found nothing.
    return null;
  }
  return (
    <Link to="/drift" className="stat drift">
      <div className="n">{band.total}</div>
      <div className="lbl">Drift</div>
      <div className="cap muted">{band.text}</div>
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

function StatTile({ label, state }: { label: string; state: FetchState<ListResult> }) {
  if (state.data === null) {
    return <UnresolvedTile label={label} error={state.error} />;
  }
  const rows = state.data.nodes;
  const { mix, caption } = composeStat(rows);
  return (
    <div className="stat">
      <div className="n">{rows.length}</div>
      <div className="lbl">{label}</div>
      <StateBar mix={mix} caption={caption} width={96} />
      <div className="cap muted">{rows.length === 0 ? "none yet" : caption}</div>
    </div>
  );
}
