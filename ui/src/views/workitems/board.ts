import type { NodeStatus, WorkItemCard, WorkItemsBoard } from "../../api";
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
export type ShelfColumnKey = "proposed" | "ready" | "in_progress" | "check" | "in_review" | "closed";

export interface WorkItemFilters {
  noun: ShelfNounFilter;
  attached: string[];
  showAllTerminal: boolean;
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

export function readWorkItemFilters(params: URLSearchParams): WorkItemFilters {
  const nounParam = params.get("noun");

  return {
    noun: normalizedNouns(nounParam),
    attached: attachedList(params.get("attached")),
    showAllTerminal: params.get("terminal") === "all",
  };
}


const matchesAttachment = (item: WorkItemCard, attached: string[]) => {
  if (attached.length === 0) return true;
  const ids = item.attachment_ids ?? [];
  return attached.some((id) =>
    id === UNATTACHED ? item.attached === undefined && ids.length === 0 : ids.includes(id));
};

export function workItemMatchesFilters(item: WorkItemCard, filters: WorkItemFilters): boolean {
  return (filters.noun === "all" || filters.noun.split(",").includes(item.noun)) && matchesAttachment(item, filters.attached);
}

export function workItemColumns(
  board: WorkItemsBoard,
  filters: WorkItemFilters,
): Record<ShelfColumnKey, WorkItemCard[]> {
  const open = board.open.filter((item) => workItemMatchesFilters(item, filters));
  const closed = board.closed.filter((item) => workItemMatchesFilters(item, filters));

  return {
    proposed: open.filter((item) => item.status === "proposed"),
    ready: open.filter((item) => item.status === "ready"),
    in_progress: open.filter((item) => item.status === "in_progress"),
    check: open.filter((item) => item.status === "check"),
    in_review: open.filter((item) => item.status === "in_review"),
    closed: filters.showAllTerminal ? closed : closed.slice(0, ROW_CAP),
  };
}

export function setWorkItemFilter(
  params: URLSearchParams,
  key: "noun" | "attached" | "terminal",
  value: string | null,
): URLSearchParams {
  const next = new URLSearchParams(params);
  const noun = normalizedNouns(next.get("noun"));
  if (noun === "all") next.delete("noun");
  else next.set("noun", noun);
  const attached = attachedList(next.get("attached"));
  if (attached.length === 0) next.delete("attached");
  else next.set("attached", attached.join(","));
  if (next.get("terminal") !== "all") next.delete("terminal");

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
  } else if (value === "all") {
    next.set(key, value);
  } else {
    next.delete(key);
  }

  return next;
}
