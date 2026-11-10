// Build is the state of the work as a board (console v2 phase 3): an
// epic-selection panel on the left deciding what shows, six columns for the six
// stored feature states, features as the cards. This is the real view,
// replacing the Phase 1 stub that only kept the router honest ahead of it.
//
// The payload (GET /api/build, views.BuildBoard) lists non-cancelled
// epics; SELECTION - which epics' features actually populate the
// columns - is this file's own concern, riding the `?epics=` URL param
// (views/build/selection.ts) rather than component state, so a board
// configuration is a link (the phase acceptance line: "a pasted board URL
// reproduces the selection"). The payload carries no drift and no queue
// data on purpose (task-12-report.md's own reasoning: drift would mean
// running the git-plus-stat sweep on every /api/build request, and queue
// data is Home's own tenant), so both join onto cards here by node id, the
// identical overlay idiom Home already runs for its own reads
// (views/build/overlays.ts).
//
// The URL stays the single source of truth downstream (matchedEpics reads
// nothing else); localStorage (views/build/selectionStorage.ts) is only a
// bootstrap for the ONE case the URL itself cannot answer - an arrival
// with no `?epics=` at all. An absent param restores a stored selection for
// this project (keyed by project.key, since every project serves on the
// same default port and thus the same browser origin) by writing it onto
// the URL once; an explicit param, present-empty included, always wins over
// storage and overwrites it, so a pasted board URL keeps reproducing its own
// selection exactly as before, and deselecting every epic and reloading
// stays deselected rather than reviving active_epics.
//
// Column order and a column's own card order are NEVER computed here: the
// six columns are a fixed literal list (views/build/columns.ts), and a
// column's cards are a status FILTER over the served epic-then-feature
// sequence, never a second sort - the phase's own acceptance line, "no
// client-side ordering code exists".
//
// Two distinct blocked-register conditions render as two distinct flags,
// never folded into one: blocked, the manual flag
// (store.Feature.Blocked), and held, the nearest retired ancestor this
// feature sits under. A card can carry both at once - a manually blocked
// feature under a retired ancestor is two independent facts, not a
// contradiction - so FeatureCard below renders each as its own sibling
// span rather than a single combined flag. (blocked landed on the wire one
// commit after this view's first pass, 9b4c778, a cross-task finding
// task-13-report.md flagged: BuildFeature originally shipped `held` only.)

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import type { BuildBoard, BuildEpic, DriftReportView, ProjectSummary, Queue } from "../api";
import { buildPath, driftPath, queuePath, statusPath } from "../api";
import BoardGrid, { type LaneMark } from "../components/BoardGrid";
import BoardToolbar from "../components/BoardToolbar";
import Contributors from "../components/Contributors";
import EmptyNote from "../components/EmptyNote";
import PageLoading from "../components/PageLoading";
import StateBar from "../components/StateBar";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import { useBoardScrollMode } from "../lib/boardScroll";
import { statusColor, statusTextColor } from "../lib/states";
import { BUILD_COLUMNS, columnCards, renderedFeatureCount, showEpicEyebrow, visibleCards } from "./build/columns";
import type { BoardCard, BuildColumnDef } from "./build/columns";
import FeatureCard from "./build/FeatureCard";
import { composeMix, composeMixCaption } from "./build/mix";
import { askLabelsFrom, driftIdsFrom } from "./build/overlays";
import { readPanelState, writePanelState } from "./build/panelStorage";
import { epicsParamValue, selectedEpicKeys, splitEpics, toggleEpicKey } from "./build/selection";
import { readSelectionKeys, writeSelectionKeys } from "./build/selectionStorage";
import "./build.css";

export default function Build() {
  useDocumentTitle(titleOf("Build"));
  // Every fetch is unconditional (Home.tsx's own rule: hook order must never
  // depend on data that has not arrived yet). board is the payload itself;
  // summary supplies the default selection (active_epics) when the URL
  // names none; queue and drift are the two condition overlays, both
  // best-effort - a card simply carries no ask/drift flag while either is
  // still loading or failed, rather than blocking the whole board on a
  // read /api/build itself never needed. Best-effort still names a failed
  // read rather than going quiet about it: a strip-every-flag-silently
  // outcome would read as "nothing is asking, nothing has drifted" when the
  // truth is "the console could not check," so the board bar carries a
  // one-line notice for either overlay's own error.
  const board = useFetch<BuildBoard>(buildPath);
  const summary = useFetch<ProjectSummary>(statusPath);
  const queue = useFetch<Queue>(queuePath);
  const drift = useFetch<DriftReportView>(driftPath);

  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const [panelOpen, setPanelOpen] = useState<boolean>(() => readPanelState() === "open");
  const [scrollMode, changeScrollMode] = useBoardScrollMode();
  const [expandedIds, setExpandedIds] = useState<ReadonlySet<string>>(new Set());

  const rawEpics = params.get("epics");
  const needsDefault = rawEpics === null;
  const projectKey = summary.data?.project.key;

  // Restore once per route arrival, including primary navigation back to a
  // bare /build while this component is still mounted. Explicit selections,
  // including ?epics=, always win. The URL remains the downstream source of
  // truth; this effect only seeds a bare arrival.
  //
  // useLayoutEffect, not useEffect: the constraint here is paint timing, not
  // data availability. By the render where projectKey first resolves, the
  // loading gate below (`if (needsDefault && summary.data === null)`) has
  // already been the thing blocking the first visible paint - that render is
  // the FIRST one to actually reach the real board JSX. A plain
  // useEffect still fires only after the browser paints that render, so the
  // active_epics-selected board would flash on screen for one frame before
  // this effect's setParams rewrite took hold. useLayoutEffect runs
  // synchronously after React commits the DOM but before the browser paints,
  // so the setParams state update this effect schedules is flushed into a
  // re-render the browser paints in place of the default one - the restored
  // selection is what actually reaches the screen first, not a fixed-up
  // second frame.
  const handledArrivalRef = useRef<string | null>(null);
  useLayoutEffect(() => {
    if (handledArrivalRef.current === location.key) return;
    if (rawEpics !== null) {
      handledArrivalRef.current = location.key;
      return;
    }
    if (projectKey === undefined) {
      // Project key not resolved yet; retry on the next render rather than
      // giving up (the attempt is not "used" until it actually runs).
      return;
    }
    handledArrivalRef.current = location.key;
    const stored = readSelectionKeys(projectKey);
    if (stored === null) {
      // Nothing stored for this project; the active_epics default applies,
      // same as if this effect never ran.
      return;
    }
    const nextParams = new URLSearchParams(params);
    nextParams.set("epics", epicsParamValue(stored));
    setParams(nextParams, { replace: true });
  }, [location.key, rawEpics, projectKey]);

  // Persist, every time the URL names an explicit selection (present,
  // empty included): toggles, a pasted board URL, and an explicit
  // deselect-to-zero all become the remembered state for this project, and
  // an explicit URL always overwrites whatever was stored before it
  // (a pasted link's own selection must win on the next reload too, not
  // just this one).
  useEffect(() => {
    if (rawEpics === null || projectKey === undefined) {
      return;
    }
    writeSelectionKeys(projectKey, selectedEpicKeys(rawEpics, []));
  }, [rawEpics, projectKey]);

  if (board.error !== null) {
    return (
      <section className="build-page">
        <h1>Build</h1>
        <p className="error">{board.error}</p>
      </section>
    );
  }
  if (board.data === null) {
    return <PageLoading title="Build" />;
  }
  // The default selection needs the project summary; an explicit `?epics=`
  // (even the empty form) never does, so a slow or failed status fetch does
  // not block a pasted, fully-specified board URL from rendering.
  if (needsDefault && summary.error !== null) {
    return (
      <section className="build-page">
        <h1>Build</h1>
        <p className="error">{summary.error}</p>
      </section>
    );
  }
  if (needsDefault && summary.data === null) {
    return <PageLoading title="Build" />;
  }

  const defaultKeys = summary.data?.active_epics ?? [];
  const selected = selectedEpicKeys(rawEpics, defaultKeys);
  const selectedSet = new Set(selected);
  // Unknown keys are ignored gracefully (the URL semantics table's own
  // words): matchedEpics is a FILTER over board.data.epics, so it both
  // drops any selected key that names no real epic and preserves the
  // served epic order for everything downstream.
  const matchedEpics = board.data.epics.filter((e) => e.key !== undefined && selectedSet.has(e.key));

  function toggleEpic(key: string) {
    const next = toggleEpicKey(selected, key);
    const nextParams = new URLSearchParams(params);
    nextParams.set("epics", epicsParamValue(next));
    // A selection click is not its own history entry, the same judgment
    // call the node tabs' `?tab=` switch made (NodeDetail.tsx): replace,
    // not push, so the back button leaves the board the way it found it.
    setParams(nextParams, { replace: true });
  }

  function togglePanel() {
    setPanelOpen((prev) => {
      const next = !prev;
      writePanelState(next ? "open" : "closed");
      return next;
    });
  }

  function toggleExpand(id: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  const cards = visibleCards(matchedEpics);
  const featureCount = matchedEpics.reduce((count, epic) => count + renderedFeatureCount(epic.features), 0);
  const askLabels = askLabelsFrom(queue.data);
  const driftIds = driftIdsFrom(drift.data);

  return (
    <div className={panelOpen ? "build" : "build panel-closed"}>
      <EpicPanel epics={board.data.epics} selected={selectedSet} onToggle={toggleEpic} open={panelOpen} />
      <div className="board-wrap">
        <BoardToolbar
          panelLabel="Epics"
          panelId="build-epics"
          panelOpen={panelOpen}
          onTogglePanel={togglePanel}
          scrollMode={scrollMode}
          onScrollMode={changeScrollMode}
          summary={matchedEpics.length === 0 ? "No epics selected." :
            `Showing ${featureCount} ${featureCount === 1 ? "feature" : "features"} across ${matchedEpics.length} ${matchedEpics.length === 1 ? "epic" : "epics"}`}
        >
          {/* Best-effort overlays fail silently on the cards themselves (a
              card just carries no ask/drift flag); this is the one place
              that failure still gets said out loud, rather than reading as
              a clean board that simply has nothing to flag. */}
          {queue.error !== null && (
            <span className="board-bar-notice muted" title={queue.error}>
              asks unavailable
            </span>
          )}
          {drift.error !== null && (
            <span className="board-bar-notice muted" title={drift.error}>
              drift unavailable
            </span>
          )}
        </BoardToolbar>
        {matchedEpics.length === 0 ? (
          <div className="board-empty">
            <EmptyNote>No epics selected. Choose one or more from the panel to see their features.</EmptyNote>
          </div>
        ) : (
          <BoardGrid
            className="board"
            scrollMode={scrollMode}
            lanes={BUILD_COLUMNS.map((def) => {
              const laneCards = columnCards(cards, def.status);
              return {
                key: def.status,
                empty: laneCards.length === 0,
                label: def.label,
                head: <ColumnHead def={def} count={laneCards.length} />,
                body: (
                  <ColumnBody
                    cards={laneCards}
                    selectedEpicCount={matchedEpics.length}
                    askLabels={askLabels}
                    driftIds={driftIds}
                    expandedIds={expandedIds}
                    onToggleExpand={toggleExpand}
                  />
                ),
              };
            })}
          />
        )}
      </div>
    </div>
  );
}

// ---------- The epic panel ----------

function EpicPanel({
  epics,
  selected,
  onToggle,
  open,
}: {
  epics: BuildEpic[];
  selected: Set<string>;
  onToggle: (key: string) => void;
  open: boolean;
}) {
  const selectedCount = epics.filter((e) => e.key !== undefined && selected.has(e.key)).length;
  const { live, finished } = splitEpics(epics);
  // The fold's open state is component state, never stored. It opens
  // whenever a finished epic becomes selected (a restored selection included,
  // since that arrives as a change), and otherwise follows the reader's own
  // toggle: it never closes by itself. While closed with a selection inside,
  // the summary says so.
  const selectedFinished = finished
    .filter((e) => e.key !== undefined && selected.has(e.key))
    .map((e) => e.key as string);
  const [foldOpen, setFoldOpen] = useState(selectedFinished.length > 0);
  const [seenFinished, setSeenFinished] = useState<string[]>(selectedFinished);
  if (selectedFinished.join(",") !== seenFinished.join(",")) {
    setSeenFinished(selectedFinished);
    if (selectedFinished.some((k) => !seenFinished.includes(k))) {
      setFoldOpen(true);
    }
  }
  const row = (epic: BuildEpic) => (
    <EpicRow key={epic.id} epic={epic} on={epic.key !== undefined && selected.has(epic.key)} onToggle={onToggle} />
  );
  return (
    <aside id="build-epics" className="epic-panel" inert={!open} aria-hidden={!open}>
      <div className="sect-head">
        <span className="eyebrow">Epics</span>
        <span className="spacer" />
        <span className="muted mono panel-count">
          {selectedCount} of {epics.length}
        </span>
      </div>
      {live.map(row)}
      {finished.length > 0 && (
        <details
          className="epic-finished"
          open={foldOpen}
          onToggle={(e) => {
            setFoldOpen(e.currentTarget.open);
          }}
        >
          <summary>
            {finished.length} finished{selectedFinished.length > 0 ? `, ${selectedFinished.length} selected` : ""}
          </summary>
          {finished.map(row)}
        </details>
      )}
      <div className="panel-note">Selection rides the URL, so a board view is linkable.</div>
    </aside>
  );
}

// The mix bar sits OUTSIDE the toggle button, a sibling rather than a child
// (Navigator.tsx:145's own precedent - its .explore-epic-row and
// .explore-epic-bar are siblings under one wrapping .explore-epic, not the
// bar nested inside the row's own link/button). A <div role="img"> inside a
// <button> both breaks the button content model and folds the bar's own
// aria-label into the button's accessible name: a
// childless epic's toggle used to announce "...no features yet" on top of
// its own name, which is not what a screen reader user asked the button for.
// The wrapping <div className="epic-row"> keeps the mock's visual (tick/
// name/count on one line, the bar beneath) without the button trying to
// contain a second, independently-labeled widget.
function EpicRow({ epic, on, onToggle }: { epic: BuildEpic; on: boolean; onToggle: (key: string) => void }) {
  const mix = composeMix(epic.feature_counts);
  const caption = composeMixCaption(mix, epic.feature_counts);
  const selectable = epic.key !== undefined;
  const label = epic.title ?? epic.key ?? epic.id;
  // renderedFeatureCount, not epic.features.length: a cancelled feature
  // never lands in any of the six columns, so the badge has to agree with
  // what the columns actually hold rather than the epic's raw child count.
  const count = renderedFeatureCount(epic.features);
  return (
    <div className={on ? "epic-row on" : "epic-row"}>
      <button
        type="button"
        className="epic-row-toggle"
        disabled={!selectable}
        aria-pressed={on}
        onClick={() => {
          if (epic.key !== undefined) {
            onToggle(epic.key);
          }
        }}
      >
        <span className="er-top">
          <span className="er-tick" aria-hidden="true">
            {on ? "✓" : ""}
          </span>
          <span className="er-name">{label}</span> <span className="er-n">{count}</span>
        </span>
      </button>
      {epic.contributors.length > 0 && (
        <div className="epic-contributors">
          Contributors: <Contributors people={epic.contributors} />
        </div>
      )}
      <span className="mixbar" title={epic.features.length > 0 ? caption : undefined}>
        {/* Navigator.tsx's own rule, mirrored: caption only when the epic
            actually has children, so a childless row's bar stays decorative
            (aria-hidden, StateBar's own contract) instead of announcing "no
            features yet" for a row nobody needs read out. The same words
            ride the bar as its hover title, so a sighted reader is never
            left with color alone. */}
        <StateBar mix={mix} caption={epic.features.length > 0 ? caption : undefined} />
      </span>
    </div>
  );
}

// ---------- The six-column board ----------

// A column is two pieces of one lane (components/BoardGrid.tsx): its head in
// the board's static heads row, and its cards in the scrolling body below.
function ColumnHead({ def, count, ...mark }: { def: BuildColumnDef; count: number } & LaneMark) {
  return (
    <div className="col-head" role="group" {...mark}>
      <span className="dot" style={{ background: statusColor(def.status) }} />
      <span className="cn" style={{ color: statusTextColor(def.status) }}>
        {def.label}
      </span>
      <span className="cc">{count}</span>
    </div>
  );
}

function ColumnBody({
  cards,
  selectedEpicCount,
  askLabels,
  driftIds,
  expandedIds,
  onToggleExpand,
  ...mark
}: {
  cards: BoardCard[];
  selectedEpicCount: number;
  askLabels: Map<string, string>;
  driftIds: Set<string>;
  expandedIds: ReadonlySet<string>;
  onToggleExpand: (id: string) => void;
} & LaneMark) {
  return (
    <div className="col board-column" {...mark}>
      {/* An empty column says it was consulted, the same "(none)" Fix's
          columns show. A collapsed rail hides it (board.css), so it is seen
          only when every column is empty and the lanes stay full width. */}
      {cards.length === 0 && <EmptyNote />}
      {cards.map(({ epic, feature }, index) => (
        <FeatureCard
          key={feature.id}
          epic={epic}
          showEpic={showEpicEyebrow(cards, index, selectedEpicCount)}
          feature={feature}
          askLabel={askLabels.get(feature.id)}
          isDrift={driftIds.has(feature.id)}
          expanded={expandedIds.has(feature.id)}
          onToggleExpand={() => {
            onToggleExpand(feature.id);
          }}
        />
      ))}
    </div>
  );
}

