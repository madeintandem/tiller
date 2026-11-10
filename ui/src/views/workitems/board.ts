import type { NodeStatus, Severity, WorkItemCard, WorkItemsBoard } from "../../api";
import { ROW_CAP } from "../../lib/disclosure";

export type ShelfNounFilter = string;
const SHELF_NOUNS = ["bug", "chore", "tweak"] as const;

function normalizedNouns(value: string | null): string {
  const selected = SHELF_NOUNS.filter((noun) => value?.split(",").includes(noun));
  return selected.length === 0 || selected.length === SHELF_NOUNS.length ? "all" : selected.join(",");
}
// Attached to is a comma list of node ids, read as OR. The member "none"
// means Unattached, so ?attached=none,F-1 matches loose work or work under
// F-1. A legacy single id (?attached=F-1) is a one-member list.
export const UNATTACHED = "none";

function attachedList(value: string | null): string[] {
  const ids = (value ?? "").split(",").map((id) => id.trim()).filter((id) => id !== "");
  return [...new Set(ids)];
}
// Severity filter members, in canonical order; the order is also the sort
// rank. UNRATED stands for a bug with no severity.
export const UNRATED = "unrated";
export const SEVERITY_ORDER: readonly Severity[] = ["blocker", "important", "medium", "minor"];
const SEVERITY_MEMBERS: readonly string[] = [...SEVERITY_ORDER, UNRATED];

function severityList(value: string | null): string[] {
  const given = (value ?? "").split(",").map((member) => member.trim());
  const selected = SEVERITY_MEMBERS.filter((member) => given.includes(member));
  return selected.length === SEVERITY_MEMBERS.length ? [] : selected;
}

export type FixSort = "shelf" | "severity" | "added";

function readSort(value: string | null): FixSort {
  return value === "severity" || value === "added" ? value : "shelf";
}

export type ShelfColumnKey = "proposed" | "ready" | "in_progress" | "check" | "in_review" | "closed";

export interface WorkItemFilters {
  noun: ShelfNounFilter;
  attached: string[];
  showAllTerminal: boolean;
  sort: FixSort;
  severity: string[];
}

export const SHELF_COLUMNS: ReadonlyArray<{ key: ShelfColumnKey; label: string }> = [
  { key: "proposed", label: "Proposed" },
  { key: "ready", label: "Ready" },
  { key: "in_progress", label: "In progress" },
  { key: "check", label: "Check" },
  { key: "in_review", label: "In review" },
  { key: "closed", label: "Closed" },
];

export const SHELF_COLUMN_STATUSES: Readonly<Record<ShelfColumnKey, NodeStatus>> = {
  proposed: "specced",
  ready: "ready",
  in_progress: "active",
  check: "active",
  in_review: "in_review",
  closed: "done",
};

// The URL parameters that name a filter axis, the ones stored and restored.
export const FILTER_AXES = ["noun", "attached", "terminal", "sort", "severity"] as const;

// `?filters=none` shows the board with no filters for this visit only, the
// way Build's `?epics=all` shows every epic: arriving there restores nothing
// and stores nothing, so Home's Fix tile can open the list its number counts
// without touching the reader's saved filters. It is the special value only
// when it names no filter axis beside it. The reader's own next filter change
// drops it from the URL (setWorkItemFilter) and is stored as usual.
const VISIT_PARAM = "filters";
export const UNFILTERED_FIX_PATH = `/fix?${VISIT_PARAM}=none`;

export function isUnfilteredVisit(params: URLSearchParams): boolean {
  return params.get(VISIT_PARAM) === "none" && !FILTER_AXES.some((axis) => params.has(axis));
}

// True when a filter narrows the open work the toolbar counts. Sort and
// Show older items change order and closed cards only, so they are not one.
export function narrowsOpenWork(filters: WorkItemFilters): boolean {
  return filters.noun !== "all" || filters.attached.length > 0 || filters.severity.length > 0;
}

export function readWorkItemFilters(params: URLSearchParams): WorkItemFilters {
  const nounParam = params.get("noun");

  return {
    noun: normalizedNouns(nounParam),
    attached: attachedList(params.get("attached")),
    showAllTerminal: params.get("terminal") === "all",
    sort: readSort(params.get("sort")),
    severity: severityList(params.get("severity")),
  };
}


const matchesAttachment = (item: WorkItemCard, attached: string[]) => {
  if (attached.length === 0) return true;
  const ids = item.attachment_ids ?? [];
  return attached.some((id) =>
    id === UNATTACHED ? item.attached === undefined && ids.length === 0 : ids.includes(id));
};

const matchesSeverity = (item: WorkItemCard, severity: string[]) =>
  severity.length === 0 || (item.noun === "bug" && severity.includes(item.severity ?? UNRATED));

export function workItemMatchesFilters(item: WorkItemCard, filters: WorkItemFilters): boolean {
  return (filters.noun === "all" || filters.noun.split(",").includes(item.noun))
    && matchesAttachment(item, filters.attached)
    && matchesSeverity(item, filters.severity);
}

const UNRATED_RANK = SEVERITY_ORDER.length;

function severityRank(item: WorkItemCard): number {
  if (item.noun !== "bug" || item.severity === undefined) return UNRATED_RANK;
  const rank = SEVERITY_ORDER.indexOf(item.severity);
  return rank < 0 ? UNRATED_RANK : rank;
}

// Pure and stable: equal keys keep input (server) order. "added" is newest
// first, ties broken by the higher EV id, items with no added_at last.
export function sortWorkItems(items: WorkItemCard[], sort: FixSort): WorkItemCard[] {
  if (sort === "shelf") return items;
  const sorted = [...items];
  if (sort === "severity") return sorted.sort((a, b) => severityRank(a) - severityRank(b));
  return sorted.sort((a, b) => {
    if (a.added_at === undefined || b.added_at === undefined) {
      return (a.added_at === undefined ? 1 : 0) - (b.added_at === undefined ? 1 : 0);
    }
    if (a.added_at !== b.added_at) return a.added_at < b.added_at ? 1 : -1;
    const ea = a.added_event ?? "";
    const eb = b.added_event ?? "";
    return ea === eb ? 0 : ea < eb ? 1 : -1;
  });
}

export function workItemColumns(
  board: WorkItemsBoard,
  filters: WorkItemFilters,
): Record<ShelfColumnKey, WorkItemCard[]> {
  const open = board.open.filter((item) => workItemMatchesFilters(item, filters));
  const closed = board.closed.filter((item) => workItemMatchesFilters(item, filters));

  const column = (status: string) => sortWorkItems(open.filter((item) => item.status === status), filters.sort);

  return {
    proposed: column("proposed"),
    ready: column("ready"),
    in_progress: column("in_progress"),
    check: column("check"),
    in_review: column("in_review"),
    closed: sortWorkItems(filters.showAllTerminal ? closed : closed.slice(0, ROW_CAP), filters.sort),
  };
}

export interface BoardScope {
  open: Record<"bug" | "chore" | "tweak", number>;
  closedShown: number;
}

// boardScope names what the Fix board shows: open work by noun across the
// five open columns, and how many closed cards the Closed column displays.
export function boardScope(columns: Record<ShelfColumnKey, WorkItemCard[]>): BoardScope {
  const open = { bug: 0, chore: 0, tweak: 0 };
  for (const column of SHELF_COLUMNS) {
    if (column.key === "closed") continue;
    for (const item of columns[column.key]) open[item.noun]++;
  }
  return { open, closedShown: columns.closed.length };
}

export function setWorkItemFilter(
  params: URLSearchParams,
  key: "noun" | "attached" | "terminal" | "sort" | "severity",
  value: string | null,
): URLSearchParams {
  const next = new URLSearchParams(params);
  next.delete(VISIT_PARAM);
  const noun = normalizedNouns(next.get("noun"));
  if (noun === "all") next.delete("noun");
  else next.set("noun", noun);
  const attached = attachedList(next.get("attached"));
  if (attached.length === 0) next.delete("attached");
  else next.set("attached", attached.join(","));
  if (next.get("terminal") !== "all") next.delete("terminal");
  const sort = readSort(next.get("sort"));
  if (sort === "shelf") next.delete("sort");
  else next.set("sort", sort);
  const severity = severityList(next.get("severity"));
  if (severity.length === 0) next.delete("severity");
  else next.set("severity", severity.join(","));

  if (key === "noun") {
    if (value !== null && SHELF_NOUNS.includes(value as typeof SHELF_NOUNS[number])) {
      const selected = noun === "all" ? [] : noun.split(",");
      const toggled = selected.includes(value) ? selected.filter((entry) => entry !== value) : [...selected, value];
      const normalized = normalizedNouns(toggled.join(","));
      if (normalized === "all") next.delete(key);
      else next.set(key, normalized);
    } else next.delete(key);
  } else if (key === "attached") {
    // A value toggles one member; null clears the axis back to All.
    const id = value?.trim() ?? "";
    const toggled = id === "" ? [] : attached.includes(id) ? attached.filter((entry) => entry !== id) : [...attached, id];
    if (toggled.length === 0) next.delete(key);
    else next.set(key, toggled.join(","));
  } else if (key === "sort") {
    if (value === "severity" || value === "added") next.set(key, value);
    else next.delete(key);
  } else if (key === "severity") {
    const member = value?.trim() ?? "";
    const toggled = !SEVERITY_MEMBERS.includes(member) ? []
      : severity.includes(member) ? severity.filter((entry) => entry !== member) : [...severity, member];
    const normalized = severityList(toggled.join(","));
    if (normalized.length === 0) next.delete(key);
    else next.set(key, normalized.join(","));
  } else if (value === "all") {
    next.set(key, value);
  } else {
    next.delete(key);
  }

  return next;
}
