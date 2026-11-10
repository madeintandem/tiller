// The nodes a reader opened lately, shown in Explore's empty reading pane.
// A per-project key, because every project serves on the same default port and
// so the same browser origin (expansionStorage.ts makes the same call): one
// project's history must never appear in another's tab.
//
// Storage is tolerant in both directions. A missing key, corrupt JSON, a wrong
// shape, or a localStorage that throws all read as an empty list, and a
// refused write loses only persistence. A stored node that has since been
// deleted or renamed still reads as stored; nothing here validates against the
// project.

export interface RecentNode {
  noun: string;
  id: string;
  title: string;
  // The canonical key when the node has one, shown in mono beside the title.
  key?: string;
}

export const RECENT_CAP = 8;
const PREFIX = "tiller-explore-recent";

export function recentKey(projectKey: string): string {
  return `${PREFIX}:${projectKey}`;
}

function isRecent(value: unknown): value is RecentNode {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const v = value as Record<string, unknown>;
  return (
    typeof v.noun === "string" &&
    typeof v.id === "string" &&
    v.id !== "" &&
    typeof v.title === "string" &&
    (v.key === undefined || typeof v.key === "string")
  );
}

// Total over anything stored: one bad entry folds the whole read to empty,
// the same all-or-nothing rule expansionStorage.ts uses.
export function readRecent(projectKey: string): RecentNode[] {
  try {
    const raw = window.localStorage.getItem(recentKey(projectKey));
    if (raw === null) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed) || !parsed.every(isRecent)) {
      return [];
    }
    return parsed.slice(0, RECENT_CAP);
  } catch {
    return [];
  }
}

// Newest first, one entry per id (a revisit moves to the front and takes the
// title as served now), capped at RECENT_CAP.
export function recordRecent(
  projectKey: string,
  node: { noun: string; id: string; title: string; key?: string },
): void {
  const entry: RecentNode = { noun: node.noun, id: node.id, title: node.title };
  if (node.key !== undefined && node.key !== "") {
    entry.key = node.key;
  }
  const next = [entry, ...readRecent(projectKey).filter((r) => r.id !== node.id)].slice(0, RECENT_CAP);
  try {
    window.localStorage.setItem(recentKey(projectKey), JSON.stringify(next));
  } catch {
    // A refused write still leaves this session's pane working.
  }
}
