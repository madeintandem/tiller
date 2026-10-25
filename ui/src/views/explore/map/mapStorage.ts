// The map's per-project memory in localStorage (docs/plans/2026-08-30-
// explore-map-design.md, "Persistence"), the same posture Explore's
// expansion memory takes (views/explore/expansionStorage.ts): a URL names a
// node, not a viewport. Every read and write is wrapped, since storage can
// be missing, full, or blocked, and the map must render correctly with no
// stored value.
import type { MapTreeView } from "../../../api";
import { containers } from "./layout";

export interface Viewport {
  x: number;
  y: number;
  k: number;
}

export interface MapState {
  open: string[];
  viewport: Viewport | null;
}

// The shape of what is written, stamped on every record. Build 2 moved the
// map's world so that the root's center is the origin, which makes every
// viewport build 1 stored a place that no longer exists: unmarked records are
// read as nothing, so that reader gets the first-visit fit rather than a
// window onto empty paper. Bump it again the next time a stored value stops
// meaning what it meant.
const SCHEMA = 2;

export function mapStorageKey(projectKey: string): string {
  return `tiller.map.${projectKey}`;
}

function isViewport(v: unknown): v is Viewport {
  return (
    typeof v === "object" &&
    v !== null &&
    typeof (v as Viewport).x === "number" &&
    typeof (v as Viewport).y === "number" &&
    typeof (v as Viewport).k === "number"
  );
}

export function readMapState(projectKey: string): MapState | null {
  try {
    const raw = localStorage.getItem(mapStorageKey(projectKey));
    if (raw === null) {
      return null;
    }
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) {
      return null;
    }
    if ((parsed as { v?: unknown }).v !== SCHEMA) {
      return null;
    }
    const open = (parsed as { open?: unknown }).open;
    const viewport = (parsed as { viewport?: unknown }).viewport;
    return {
      open: Array.isArray(open) ? open.filter((id): id is string => typeof id === "string") : [],
      viewport: isViewport(viewport) ? viewport : null,
    };
  } catch {
    return null;
  }
}

export function writeMapState(projectKey: string, state: MapState): void {
  try {
    localStorage.setItem(mapStorageKey(projectKey), JSON.stringify({ v: SCHEMA, ...state }));
  } catch {
    // Storage refused (quota, private mode): the map still works, it just forgets.
  }
}

// reconcileOpen keeps only the stored ids that are still containers in the
// live tree, so a removed epic or a feature that lost its tasks never keeps
// a stale entry, and a leaf never counts as open.
export function reconcileOpen(stored: readonly string[], view: MapTreeView): Set<string> {
  const live = containers(view);
  return new Set(stored.filter((id) => live.has(id)));
}
