import { Link, useNavigate } from "react-router-dom";
import type { RecapWindow } from "../../api";
import { usePersonName } from "../../people";
import type { HistoryFacets, HistoryFilter } from "./filter";
import { kindLabel } from "./filter";

export const WINDOWS: RecapWindow[] = ["visit", "24h", "7d"];

// The grouping mode, client-only (no server param exists, or needs to: the
// payload stays grouped by (epic, stream) either way, and which render mode
// reads it is entirely the view's own decision). Epic leads the array as the
// default, the same position "visit" holds in WINDOWS above.
export type GroupMode = "epic" | "time";
const GROUP_MODES: GroupMode[] = ["epic", "time"];

// The detail level: milestones by default, everything on request.
type ShowMode = "milestones" | "all";
const SHOWS: ShowMode[] = ["milestones", "all"];

// Everything the page's URL carries. Filters hold the raw parameter values, so
// a link to another window keeps them even when this window's recap has no
// such actor; the view decides separately which ones are valid for the data.
export interface HistoryView extends HistoryFilter {
  window: RecapWindow;
  group: GroupMode;
  showAll: boolean;
}

// Reads the view off the URL. Each parameter degrades to its default on a
// value it does not know, so a stale or hand-mangled link reads as the
// default digest rather than a blank page.
export function viewOf(params: URLSearchParams): HistoryView {
  const raw = params.get("window");
  return {
    window: raw === "24h" || raw === "7d" ? raw : "visit",
    group: params.get("group") === "time" ? "time" : "epic",
    showAll: params.get("show") === "all",
    actor: params.get("actor") || undefined,
    kind: params.get("kind") || undefined,
    epic: params.get("epic") || undefined,
  };
}

// The one place the view composes into an address, so every control (the
// three segs, the three selects, the empty states, the ask link) can never
// build a URL that drops a parameter it does not own. Each parameter omits
// itself at its own default, so the canonical /history stays clean.
export function historyPath(view: HistoryView): string {
  const params = new URLSearchParams();
  if (view.window !== "visit") {
    params.set("window", view.window);
  }
  if (view.group !== "epic") {
    params.set("group", view.group);
  }
  // Milestones is the default, so only Everything writes a parameter.
  if (view.showAll) {
    params.set("show", "all");
  }
  if (view.actor !== undefined) {
    params.set("actor", view.actor);
  }
  if (view.kind !== undefined) {
    params.set("kind", view.kind);
  }
  if (view.epic !== undefined) {
    params.set("epic", view.epic);
  }
  const search = params.toString();
  return search === "" ? "/history" : `/history?${search}`;
}

export function clearedFilters(view: HistoryView): HistoryView {
  return { ...view, actor: undefined, kind: undefined, epic: undefined };
}

// The head's control row: grouping, window, and detail segs, then the three
// filter selects once a recap has loaded to draw their options from. The segs
// are real <Link>s with replace, because each lives in the URL and stepping
// through them should not stack history entries a reader has to walk back.
// pending means the facets are the last window's while a new one loads: the
// selects stay, with their choices, but are disabled until it lands.
export function HistoryControls({
  view,
  facets,
  pending = false,
}: {
  view: HistoryView;
  facets: HistoryFacets | null;
  pending?: boolean;
}) {
  return (
    <div className="hist-controls">
      <div className="seg" role="group" aria-label="History grouping">
        {GROUP_MODES.map((g) => (
          <SegLink key={g} to={historyPath({ ...view, group: g })} active={g === view.group}>
            {g === "epic" ? "by epic" : "by time"}
          </SegLink>
        ))}
      </div>
      <div className="seg" role="group" aria-label="History window">
        {WINDOWS.map((w) => (
          <WindowLink key={w} target={w} view={view} />
        ))}
      </div>
      <div className="seg" role="group" aria-label="History detail">
        {SHOWS.map((s) => (
          <SegLink key={s} to={historyPath({ ...view, showAll: s === "all" })} active={(s === "all") === view.showAll}>
            {s === "all" ? "Everything" : "Milestones"}
          </SegLink>
        ))}
      </div>
      {facets !== null && <FilterSelects view={view} facets={facets} disabled={pending} />}
    </div>
  );
}

function SegLink({ to, active, children }: { to: string; active: boolean; children: string }) {
  return (
    <Link to={to} className={active ? "active" : undefined} aria-current={active ? "page" : undefined} replace>
      {children}
    </Link>
  );
}

export function WindowLink({ target, view }: { target: RecapWindow; view: HistoryView }) {
  return (
    <SegLink to={historyPath({ ...view, window: target })} active={target === view.window}>
      {target}
    </SegLink>
  );
}

// Three selects, each with a visible label and a first option meaning no
// filter. A value the recap does not hold shows the no-filter option.
function FilterSelects({ view, facets, disabled }: { view: HistoryView; facets: HistoryFacets; disabled: boolean }) {
  const navigate = useNavigate();
  const nameOf = usePersonName();
  const actors = facets.actors
    .map((id) => ({ value: id, label: nameOf(id) }))
    .sort((a, b) => a.label.localeCompare(b.label));
  const kinds = facets.kinds.map((k) => ({ value: k, label: kindLabel(k) }));
  const epics = facets.epics.map((e) => ({ value: e.id, label: e.title }));

  function pick(axis: keyof HistoryFilter, value: string) {
    navigate(historyPath({ ...view, [axis]: value === "" ? undefined : value }), { replace: true });
  }

  return (
    <div className="hist-filters">
      <FilterSelect label="Actor" none="All actors" value={view.actor} options={actors} disabled={disabled} onPick={(v) => pick("actor", v)} />
      <FilterSelect label="Kind" none="All kinds" value={view.kind} options={kinds} disabled={disabled} onPick={(v) => pick("kind", v)} />
      <FilterSelect label="Epic" none="All epics" value={view.epic} options={epics} disabled={disabled} onPick={(v) => pick("epic", v)} />
    </div>
  );
}

function FilterSelect({
  label,
  none,
  value,
  options,
  disabled,
  onPick,
}: {
  label: string;
  none: string;
  value: string | undefined;
  options: { value: string; label: string }[];
  disabled: boolean;
  onPick: (value: string) => void;
}) {
  const known = value !== undefined && options.some((o) => o.value === value);
  return (
    <label className="hist-filter">
      <span className="eyebrow">{label}</span>
      <select className="control" value={known ? value : ""} disabled={disabled} onChange={(e) => onPick(e.currentTarget.value)}>
        <option value="">{none}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}
