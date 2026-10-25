// The Fix board's filters, persisted to localStorage so leaving the page and
// coming back restores the filters the developer had set, the same bootstrap
// Build's epic selection runs (views/build/selectionStorage.ts). Namespaced
// per project key for the same reason: every project serves on the same
// default port, one browser origin, so a flat key would leak one project's
// filters into another project's tab.
//
// The stored value is the filter axes' own URL query (noun, attached,
// terminal), normalized through readWorkItemFilters, so storage and the URL
// share one serialization and one parser. Unlike Build's `?epics=`, the Fix
// board's default ("All" on every axis) is the EMPTY query, so an explicit
// return to All is stored as the empty string, read back distinctly from no
// entry at all (`null`).

import { readWorkItemFilters } from "./board";

const FILTER_STORAGE_PREFIX = "tiller-fix-filters";
const FILTER_AXES = ["noun", "attached", "terminal"] as const;

export function filterStorageKey(projectKey: string): string {
  return `${FILTER_STORAGE_PREFIX}:${projectKey}`;
}

// True when the URL names any filter axis; only a URL naming none is a
// candidate for restoring stored filters.
export function hasFilterParams(params: URLSearchParams): boolean {
  return FILTER_AXES.some((axis) => params.has(axis));
}

// The normalized filter query for these params: unrelated params dropped,
// each axis written only when it differs from All.
export function filterQuery(params: URLSearchParams): string {
  const filters = readWorkItemFilters(params);
  const query = new URLSearchParams();
  if (filters.noun !== "all") query.set("noun", filters.noun);
  if (filters.attached.length > 0) query.set("attached", filters.attached.join(","));
  if (filters.showAllTerminal) query.set("terminal", "all");
  return query.toString();
}

// Returns null when this project has nothing stored yet, the caller's cue to
// keep the default All board. A stored entry comes back normalized, so a
// hand-edited value folds the same way a hand-edited URL does.
export function readFilterQuery(projectKey: string): string | null {
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(filterStorageKey(projectKey));
  } catch {
    return null;
  }
  if (raw === null) {
    return null;
  }
  return filterQuery(new URLSearchParams(raw));
}

export function writeFilterQuery(projectKey: string, params: URLSearchParams): void {
  try {
    window.localStorage.setItem(filterStorageKey(projectKey), filterQuery(params));
  } catch {
    // A refused write still leaves this session working; only persistence
    // across a visit is lost.
  }
}
