import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";

import type { ProjectSummary, WorkItemCard, WorkItemsBoard } from "../api";
import { statusPath, workItemsPath } from "../api";
import BoardGrid from "../components/BoardGrid";
import BoardToolbar from "../components/BoardToolbar";
import Chip from "../components/Chip";
import EmptyNote from "../components/EmptyNote";
import PageLoading from "../components/PageLoading";
import RefLink from "../components/RefLink";
import StateDot from "../components/StateDot";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import { useBoardScrollMode } from "../lib/boardScroll";
import { ROW_CAP } from "../lib/disclosure";
import { has } from "../lib/has";
import { PersonName } from "../people";
import {
  readWorkItemFilters,
  setWorkItemFilter,
  SHELF_COLUMNS,
  SHELF_COLUMN_STATUSES,
  workItemColumns,
  UNATTACHED,
  workItemMatchesFilters,
} from "./workitems/board";
import AttachmentFilter from "./workitems/AttachmentFilter";
import type { ShelfColumnKey, WorkItemFilters } from "./workitems/board";
import { hasFilterParams, readFilterQuery, writeFilterQuery } from "./workitems/filterStorage";
import { readPanelState, writePanelState } from "./build/panelStorage";
import "./workitems.css";

const FIX_PANEL_STORAGE_KEY = "tiller-fix-panel";
// Router state every filter control navigates with (see fromFilterControl).
const FILTER_NAVIGATION = { fixFilters: true };

// These params with a stored filter query laid over them, or null when
// nothing non-default is stored.
function restoredParams(params: URLSearchParams, stored: string | null): URLSearchParams | null {
  if (stored === null || stored === "") return null;
  const next = new URLSearchParams(params);
  for (const [key, value] of new URLSearchParams(stored)) next.set(key, value);
  return next;
}

export default function WorkItems() {
  useDocumentTitle(titleOf("Fix"));
  const board = useFetch<WorkItemsBoard>(workItemsPath);
  const [params, setParams] = useSearchParams();
  const [scrollMode, changeScrollMode] = useBoardScrollMode();
  const [panelOpen, setPanelOpen] = useState(() => readPanelState(FIX_PANEL_STORAGE_KEY) === "open");
  const summary = useFetch<ProjectSummary>(statusPath);
  const projectKey = summary.data?.project.key;
  const location = useLocation();
  const namesFilters = hasFilterParams(params);
  // A bare /fix is two different things: the All filter (a filter control
  // chose it) or arriving at Fix (the topbar, a link, a reload). Filter
  // controls mark their navigations, so only they can store All.
  const fromFilterControl = (location.state as { fixFilters?: boolean } | null)?.fixFilters === true;

  // Restore, the way Build restores its epic selection: arriving at a URL
  // that names no filter takes this project's stored filters, once per
  // arrival, and writes them onto the URL. The router applies that rewrite
  // as a transition, so until it lands the board renders from the stored
  // filters directly and the All board never paints first. An arrival whose
  // project status failed keeps All rather than switching filters later
  // under the reader.
  const handledArrivalRef = useRef<string | null>(null);
  const pendingRestoreRef = useRef<URLSearchParams | null>(null);
  const restoring =
    handledArrivalRef.current === location.key
      ? pendingRestoreRef.current
      : namesFilters || fromFilterControl || projectKey === undefined
        ? null
        : restoredParams(params, readFilterQuery(projectKey));
  const viewParams = restoring ?? params;
  const filters = readWorkItemFilters(viewParams);
  useLayoutEffect(() => {
    if (handledArrivalRef.current === location.key) return;
    if (!namesFilters && !fromFilterControl && projectKey === undefined && summary.error === null) return;
    handledArrivalRef.current = location.key;
    pendingRestoreRef.current = restoring;
    if (restoring !== null) setParams(restoring, { replace: true });
  }, [location.key, projectKey, summary.error]);

  // Persist every filter the URL names, and an All a filter control chose; a
  // bare arrival never overwrites what is stored.
  useEffect(() => {
    if (projectKey === undefined || !(namesFilters || fromFilterControl)) return;
    writeFilterQuery(projectKey, params);
  }, [location.key, projectKey]);

  if (board.error !== null) return <p className="error">{board.error}</p>;
  if (board.data === null) return <PageLoading title="Fix" />;
  // Restoring needs the project key; a URL that already names its filters
  // never waits on it, and a failed status read falls back to All.
  if (!namesFilters && summary.data === null && summary.error === null) return <PageLoading title="Fix" />;

  const columns = workItemColumns(board.data, filters);
  const filteredClosedTotal = board.data.closed.filter((item) =>
    workItemMatchesFilters(item, { ...filters, showAllTerminal: true }),
  ).length;
  const counts = { bug: 0, chore: 0, tweak: 0 };
  for (const column of SHELF_COLUMNS) {
    for (const item of columns[column.key]) counts[item.noun]++;
  }

  function togglePanel() {
    const next = !panelOpen;
    setPanelOpen(next);
    writePanelState(next ? "open" : "closed", FIX_PANEL_STORAGE_KEY);
  }

  return (
    <section className="work-items-page">
      <div className={panelOpen ? "work-items-layout" : "work-items-layout panel-closed"}>
        <WorkItemFiltersPanel
          board={board.data}
          open={panelOpen}
          filters={filters}
          params={viewParams}
          onToggleAttached={(id) => setParams(setWorkItemFilter(viewParams, "attached", id), { state: FILTER_NAVIGATION })}
          hasOlder={filteredClosedTotal > ROW_CAP}
          onToggleOlder={(on) => setParams(setWorkItemFilter(viewParams, "terminal", on ? "all" : null), { state: FILTER_NAVIGATION })}
        />
        <div className="work-items-board-wrap">
          <BoardToolbar
            panelLabel="Filters"
            panelId="fix-filters"
            panelOpen={panelOpen}
            onTogglePanel={togglePanel}
            scrollMode={scrollMode}
            onScrollMode={changeScrollMode}
            summary={`Showing ${counts.bug} ${counts.bug === 1 ? "bug" : "bugs"}, ${counts.chore} ${counts.chore === 1 ? "chore" : "chores"}, and ${counts.tweak} ${counts.tweak === 1 ? "tweak" : "tweaks"}`}
          />
          <BoardGrid
            className="work-items-board"
            scrollMode={scrollMode}
            lanes={SHELF_COLUMNS.map((column) => ({
              key: column.key,
              head: <WorkItemColumnHeading column={column} count={columns[column.key].length} />,
              body: <WorkItemColumn column={column} items={columns[column.key]} />,
            }))}
          />
        </div>
      </div>
    </section>
  );
}

function WorkItemFiltersPanel({
  board,
  open,
  filters,
  params,
  onToggleAttached,
  hasOlder,
  onToggleOlder,
}: {
  board: WorkItemsBoard;
  open: boolean;
  filters: WorkItemFilters;
  params: URLSearchParams;
  onToggleAttached: (id: string) => void;
  hasOlder: boolean;
  onToggleOlder: (on: boolean) => void;
}) {
  return (
    <aside id="fix-filters" className="work-items-filters" aria-label="Work item filters" inert={!open} aria-hidden={!open}>
      <h2>Filters</h2>
      <div className="work-item-filter-group">
        <p className="work-item-filter-label">Noun</p>
        <nav className="work-item-filter-links" aria-label="Work item type">
          <FilterLink params={params} axis="noun" value="all" active={filters.noun === "all"}>
            All
          </FilterLink>
          <FilterLink params={params} axis="noun" value="bug" noun="bug" active={filters.noun.split(",").includes("bug")}>
            Bugs
          </FilterLink>
          <FilterLink params={params} axis="noun" value="chore" noun="chore" active={filters.noun.split(",").includes("chore")}>
            Chores
          </FilterLink>
          <FilterLink params={params} axis="noun" value="tweak" noun="tweak" active={filters.noun.split(",").includes("tweak")}>
            Tweaks
          </FilterLink>
        </nav>
      </div>
      <div className="work-item-filter-group">
        <p className="work-item-filter-label">Attached to</p>
        <nav className="work-item-filter-links" aria-label="Attachment">
          <FilterLink params={params} axis="attached" value={null} active={filters.attached.length === 0}>
            All
          </FilterLink>
          <FilterLink params={params} axis="attached" value={UNATTACHED} active={filters.attached.includes(UNATTACHED)}>
            Unattached
          </FilterLink>
        </nav>
        <AttachmentFilter options={board.attachments} selected={filters.attached} onToggle={onToggleAttached} />
      </div>
      {hasOlder && (
        <div className="work-item-filter-group">
          <label className="work-items-older-toggle">
            <input
              type="checkbox"
              role="switch"
              checked={filters.showAllTerminal}
              onChange={(event) => onToggleOlder(event.target.checked)}
            />
            <span>Show older items</span>
          </label>
        </div>
      )}
    </aside>
  );
}

function FilterLink({
  params,
  axis,
  value,
  active,
  noun,
  children,
}: {
  params: URLSearchParams;
  axis: "noun" | "attached";
  value: string | null;
  active: boolean;
  noun?: string;
  children: ReactNode;
}) {
  const next = setWorkItemFilter(params, axis, value).toString();
  return (
    <Link
      to={next === "" ? "/fix" : `/fix?${next}`}
      state={FILTER_NAVIGATION}
      className={`work-item-filter-link${active ? " active" : ""}`}
      aria-current={active ? "true" : undefined}
      data-noun={noun}
    >
      {children}
    </Link>
  );
}

// A column is two pieces of one lane (components/BoardGrid.tsx): its heading
// in the board's static heads row, and its cards in the scrolling body below,
// a region named for the column.
function WorkItemColumnHeading({
  column,
  count,
}: {
  column: { key: ShelfColumnKey; label: string };
  count: number;
}) {
  return (
    <h2 className="work-item-column-heading">
      <StateDot status={SHELF_COLUMN_STATUSES[column.key]} />
      <span>{column.label}</span>
      <span className="work-item-column-count">{count}</span>
    </h2>
  );
}

function WorkItemColumn({
  column,
  items,
}: {
  column: { key: ShelfColumnKey; label: string };
  items: WorkItemCard[];
}) {
  return (
    <section className="work-item-column board-column" role="region" aria-label={column.label}>
      {items.length === 0 ? (
        <EmptyNote />
      ) : (
        <div className="work-item-column-cards">
          {items.map((item) => (
            <WorkItemCardView key={item.id} item={item} terminal={column.key === "closed"} />
          ))}
        </div>
      )}
    </section>
  );
}

function WorkItemCardView({ item, terminal }: { item: WorkItemCard; terminal: boolean }) {
  return (
    <article className="work-item-card">
      <span className="work-item-noun-chip" data-noun={item.noun}>{item.noun}</span>
      <RefLink
        node={item}
        label={has(item.title) ? item.title : item.ref}
        className="work-item-card-title"
      />
      {has(item.owner) && <span className="work-item-owner">Owner: <PersonName id={item.owner} name={item.owner_name} /></span>}
      {item.attached !== undefined && (
        <p className="work-item-attachment">
          Attached to <RefLink node={item.attached} />
        </p>
      )}
      {has(item.resolution) && <p className="work-item-attachment">Resolution: {item.resolution}</p>}
      {item.supersedes?.map((id) => (
        <p className="work-item-attachment" key={id}>
          Supersedes <Link to={`/node/${item.noun}/${id}`}>{id}</Link>
        </p>
      ))}
      {has(item.superseded_by) && (
        <p className="work-item-attachment">
          Superseded by <Link to={`/node/${item.noun}/${item.superseded_by}`}>{item.superseded_by}</Link>
        </p>
      )}
      {(has(item.activity) || terminal) && (
        <div className="work-item-card-chips">
          {has(item.activity) && <Chip status={item.activity}>{item.activity}</Chip>}
          {terminal && <Chip status={item.status}>{item.status}</Chip>}
        </div>
      )}
    </article>
  );
}
