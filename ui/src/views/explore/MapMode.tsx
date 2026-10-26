// MapMode is Explore's fourth mode (docs/spec/ui-surface.md): the
// fetch, the open set (default collapsed, remembered per project), the viewport
// (remembered per project), the selection (the URL's sel param, so a link
// lights the same node for a teammate), the corner toolbar, the canvas, and the
// docked card. Escape deselects; clicking empty canvas deselects; so does
// clicking the selected card again.
//
// Four moves belong to the mode rather than to the canvas under it, because
// each of them needs the layout and the frame at once. A map with nothing
// remembered fits itself into the frame the first time it is drawn, so the
// reader opens on the whole tree rather than on the corner of one epic. A
// selection the reader could not see coming, off a link or out of the docked
// card's own links, is panned to: the card it lights is as likely to be off
// screen as on it. A card clicked on the canvas is neither, so the map holds
// still. And two reveals share one rule (panInto below): a chevron that OPENS
// a branch reveals what it just drew, whenever that landed off the frame or
// under the card, and a card that TAKES FOCUS is panned into the same rect,
// since Tab walks every card on the map and most of them are off the frame on
// any real project.
//
// Of those four, the trip out of the docked card and the chevron's reveal are
// gestures, so those two are remembered; the fit, the trip a link asks for,
// and the focus reveal move the map for a reader who did not move it, and
// leave storage alone.
//
// Both moves also know what the card itself is covering. It floats in the
// map's top right corner rather than taking a column out of the layout, so
// the canvas keeps the whole frame and the two of them center in frame.w
// minus DOCK_INSET instead (2026-08-31). Centering in the whole frame put a
// fitted map's right hand side, and the very counterparts the card was
// listing, underneath it.
//
// `ready` is Explore's own summarySettled, the signal that /api/status has
// landed (or failed) and the project key is as resolved as it will get. The
// map holds its open set unmaterialized until then, because /api/status and
// /api/map are independent fetches with no ordering guarantee: without the
// gate, a map payload that arrived first would materialize the default open
// set against an unknown project, ignore what that project had stored, and
// then overwrite it. An /api/status error flips the gate too,
// so a project whose status fetch fails still gets a map, on the defaults,
// rather than a loading line that never clears.
//
// The memory itself is written under three rules, all
// three of them Explore.tsx:274-326's own idiom applied to this mode: the
// project key rides a ref so the persist effect fires on a change to what it
// is WRITING and never on the mere arrival of a key; a restore latch means
// "the value I am about to write has been through a restore against this
// key", so a key that shows up late (a failed /api/status, then a refresh
// that succeeds) still gets its one restore instead of having the defaults
// written over it; and a touch latch means only a real gesture is ever
// persisted, so an untouched default is not frozen into storage where a
// later-minted epic would render closed forever. The open set is written
// eagerly, since it changes once per click; the viewport is debounced,
// since MapCanvas reports one for every pointermove of a pan. Relationship
// overlays are a local inspection choice, not a setting readers keep.
import type { ReactElement } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { MapView } from "../../api";
import { mapPath } from "../../api";
import { useFetch } from "../../hooks";
import "../map.css";
import "./map/relationships.css";
import { chainOf } from "./map/chain";
import type { MapLayout, Side } from "./map/layout";
import { ROOT_ID, SHELF_BUG, SHELF_CHORE, SHELF_TWEAK, containers, layoutMap, parentMap } from "./map/layout";
import MapCanvas, { MIN_ZOOM } from "./map/MapCanvas";
import MapCard, { DOCK_INSET } from "./map/MapCard";
import type { Viewport } from "./map/mapStorage";
import { readMapState, reconcileOpen, writeMapState } from "./map/mapStorage";
import { RelationshipCard, RelationshipLegend } from "./map/RelationshipDetails";
import ReferenceIssues from "./map/ReferenceIssues";
import RelationshipResults from "./map/RelationshipResults";
import type { MapRelationshipResults } from "./map/RelationshipResults";
import { relationshipOverlay, WORK_LINK_PRESET } from "./map/relationships";
import { useRelationshipContext } from "./map/useRelationshipContext";

const HOME: Viewport = { x: 0, y: 0, k: 1 };

// How long the viewport has to hold still before it reaches storage. Long
// enough that a pan or a wheel zoom writes once at its end rather than once
// per frame, short enough that a reader who pans and immediately closes the
// tab keeps where they were.
const VIEWPORT_SETTLE_MS = 250;

// The band the floating toolbar occupies at the top of the canvas, and the
// breathing room a fit leaves at the frame's other edges. The toolbar is
// absolutely positioned over the canvas with opaque buttons, so a fit that
// used the whole frame height put the map's first rows underneath it: on
// Acme at 1440x900 two feature cards were invisible until the reader
// panned.
const FIT_TOP = 48;
const FIT_INSET = 16;

// The width a fit or a trip has to work in: the frame, less whatever the
// selected node's card is covering on its right (DOCK_INSET, or nothing at
// all when no card is up). The card floats over the canvas rather than
// taking width from it, so the frame stays the whole window and the inset
// comes off here, at the one place that decides where the middle is.
//
// It never gives away more than half the frame. DOCK_INSET off a 400px
// window leaves 48, which is a sliver of map at the zoom floor, and off
// anything narrower it leaves nothing and the arithmetic below puts the
// drawing off screen to the left. A window too small to hold both keeps half
// itself for the map and lets the card overlap what is under it, which is
// the honest answer at that size.
function freeWidth(frame: { w: number; h: number }, rightInset: number): number {
  return Math.max(frame.w - rightInset, frame.w / 2);
}

// The viewport that puts the whole layout in the frame: the smaller of the
// two ratios, taken against a frame inset by the toolbar's band, a margin,
// and the selected node's card, never magnified past 1:1, never below the
// zoom floor the wheel shares, and centered on both axes.
//
// A Viewport is world coordinates, since it is the world point at the
// frame's top left, so the centering is stated as "the free width's center
// is the layout's center" rather than as a screen offset: that width is
// free / k world units, so its center sits at x + free / (2k), and that has
// to be the bounds' own center. The vertical half is the same sentence over
// the band running from FIT_TOP down to frame.h, which is what keeps the
// toolbar clear. Fitting to the frame's top left instead left the drawing in
// a corner with the rest of the canvas empty paper, which on a wide layout
// in a short frame is more than half the window.
//
// The zoom is taken against the free width too, not just the offset: a map
// scaled to the whole frame and then centered in less of it hangs off both
// ends of what the reader can see.
//
// It reads layout.bounds rather than a width and a height: the drawing is
// centered on the root at the world origin, so half of it is at negative
// coordinates and a size alone no longer says where it is.
function fitViewport(layout: MapLayout, frame: { w: number; h: number }, rightInset: number): Viewport {
  const b = layout.bounds;
  const free = freeWidth(frame, rightInset);
  const k = Math.max(MIN_ZOOM, Math.min(1, (free - 2 * FIT_INSET) / b.w, (frame.h - FIT_TOP - FIT_INSET) / b.h));
  return {
    x: b.x + b.w / 2 - free / (2 * k),
    y: b.y + b.h / 2 - (frame.h + FIT_TOP) / (2 * k),
    k,
  };
}

// What a chevron just put on screen: the card it was clicked on, plus every
// box the opening made visible, which in the new layout is every descendant
// of that card, since a closed branch had none of them drawn. The walk is
// over the LAYOUT's parent links for the same reason the deep-link walk
// below is: a bug or a chore names no parent on the wire, and its container
// is a shelf the layout invents.
//
// The side comes along because a rectangle too big for the room it has to
// fit in cannot be centered into view, only anchored, and the edge to anchor
// is the one nearest the card the reader clicked.
interface Revealed {
  x: number;
  y: number;
  w: number;
  h: number;
  side: Side;
}

function revealedRect(view: MapView, next: MapLayout, id: string): Revealed | null {
  const own = next.boxes.get(id);
  if (own === undefined) {
    return null;
  }
  const parents = parentMap(view);
  const descendsFromToggled = (start: string): boolean => {
    let current = parents.get(start);
    while (current !== undefined) {
      if (current === id) {
        return true;
      }
      current = parents.get(current);
    }
    return false;
  };
  let minX = own.x;
  let minY = own.y;
  let maxX = own.x + own.w;
  let maxY = own.y + own.h;
  for (const b of next.boxes.values()) {
    if (b.id === id || !descendsFromToggled(b.id)) {
      continue;
    }
    minX = Math.min(minX, b.x);
    minY = Math.min(minY, b.y);
    maxX = Math.max(maxX, b.x + b.w);
    maxY = Math.max(maxY, b.y + b.h);
  }
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY, side: own.side };
}

// How far a span has to move, in screen pixels, to sit inside [low, high]:
// nothing at all when it already does, the one shortfall when it hangs off a
// single end, and, when it is simply longer than the room there is, whatever
// it takes to put the end `anchorNear` names on the near edge. Both axes are
// the same sentence, so they share it.
function shiftInto(near: number, far: number, low: number, high: number, anchorNear: boolean): number {
  if (far - near > high - low) {
    return anchorNear ? low - near : high - far;
  }
  if (near < low) {
    return low - near;
  }
  if (far > high) {
    return high - far;
  }
  return 0;
}

// The smallest pan that brings a world rectangle inside the free rect: the
// frame less the toolbar's band at the top, the margin at the other three
// edges, and whatever the floating card is covering on the right. A rectangle
// already inside it returns the very viewport it was given, so a caller can
// hand the result straight to setViewport and React skips the render.
//
// The zoom is never touched. A reveal that fitted its rectangle would answer
// one click, or one Tab, by redrawing the whole map at a new scale, and the
// reader asked to see one thing.
//
// Two callers, on the same rule: the branch a chevron just opened, and the
// card the keyboard just landed on.
function panInto(v: Viewport, rect: Revealed, frame: { w: number; h: number }, rightInset: number): Viewport {
  const dx = shiftInto(
    (rect.x - v.x) * v.k,
    (rect.x + rect.w - v.x) * v.k,
    FIT_INSET,
    freeWidth(frame, rightInset) - FIT_INSET,
    // A right-side rectangle reads from its left edge, a left-side one from
    // its right: both of those are the edge the parent card is on. It only
    // decides anything for a rectangle too big for the room there is.
    rect.side === "right",
  );
  const dy = shiftInto((rect.y - v.y) * v.k, (rect.y + rect.h - v.y) * v.k, FIT_TOP, frame.h - FIT_INSET, true);
  return dx === 0 && dy === 0 ? v : { x: v.x - dx / v.k, y: v.y - dy / v.k, k: v.k };
}

// The open set the map takes when this project has nothing stored: nothing
// at all (docs/spec/ui-surface.md). Acme's 17 epics carry 102
// features between them, and opening all of them fits into a laptop frame only
// at k = 0.39, where a title is five pixels tall. Collapsed, every branch card
// wears its own count, the whole project reads at 1:1, and one click opens the
// branch the reader came for.
//
// What Expand all reaches: every epic that has features, plus each shelf
// branch that carries anything, features left closed. Only containers are
// named, so a childless epic is never written into memory as open, which is
// a thing reconcileOpen would have to take back.
function expandedOpen(view: MapView): Set<string> {
  const withChildren = containers(view);
  const out = new Set(
    view.nodes.filter((n) => n.noun === "epic" && withChildren.has(n.id)).map((n) => n.id),
  );
  for (const shelf of [SHELF_BUG, SHELF_CHORE, SHELF_TWEAK]) {
    if (withChildren.has(shelf)) {
      out.add(shelf);
    }
  }
  return out;
}

export default function MapMode({
  projectKey,
  projectName,
  ready,
}: {
  projectKey: string | undefined;
  projectName: string | undefined;
  ready: boolean;
}): ReactElement {
  // The root card wears the project's name, the thing a reader calls it,
  // while the key stays what memory is filed under. With /api/status out or
  // failed there is no name to say, and a blank card at the center of the
  // map would read as a bug, so it says the generic noun instead.
  const rootTitle = projectName ?? "Project";
  const { data: view, error, loading } = useFetch<MapView>(mapPath, { retainData: true });
  const { inspectionView, mappedResults: activeResults, clearedOnRefresh, selectContext, mapResults, clearMappedResults } = useRelationshipContext(view);
  const [params, setParams] = useSearchParams();
  const [open, setOpen] = useState<Set<string> | null>(null);
  const [viewport, setViewport] = useState<Viewport>(HOME);
  // Global relationship scope is deliberately transient. At rest, keep the
  // selected neighborhood rather than restoring yesterday's global overlay.
  const [showAll, setShowAll] = useState(false);
  const [relationshipKind, setRelationshipKind] = useState("");
  const [investigation, setInvestigation] = useState<"relationships" | "issues" | null>(null);
  const sel = params.get("sel");
  // The selection is resolved against the payload rather than taken from the
  // URL as written: a sel naming no node used to light nothing, dim every
  // card, and dock no card to say why.
  const selectedNode = view === null || sel === null ? undefined : view.nodes.find((n) => n.id === sel);
  const selectedVertex = inspectionView === null || sel === null ? undefined : inspectionView.relationships.vertices.find((vertex) => vertex.id === sel);
  const selected = selectedNode?.id ?? selectedVertex?.id ?? null;
  // What the selected node's card is covering on the right of the map, which
  // every fit and every trip below centers around. It is keyed on the
  // resolved selection because that is exactly what renders the card: a sel
  // naming no node docks nothing and takes nothing.
  const rightInset = selected !== null ? DOCK_INSET : 0;

  // The card the map still owes a trip to. It is set for a selection the
  // reader cannot see coming: the one on the URL when this mode mounts, and
  // the one they pick out of the docked card's own links, either of which
  // can name a node anywhere in the tree. A click on the canvas sets
  // nothing, because that card is already under the reader's eye and moving
  // the map out from under the click is the one thing they did not ask for.
  // useRef's argument is evaluated every render and kept only from the
  // first, which is exactly "the sel this mode mounted on".
  const centerOnRef = useRef<string | null>(sel);
  // Whether that trip was asked for. The two cases are not the same kind of
  // event: a link's sel moves the map for a reader who only opened a URL,
  // while picking a node out of the docked card is a gesture like any pan.
  // Only the second is remembered. Persisting the first would write the
  // untouched default open set into memory, and reconcileOpen only ever
  // drops ids, so an epic minted next week would render closed forever for
  // whoever had followed that link. It is seeded false
  // because the trip centerOnRef is seeded with, the sel this mode mounted
  // on, is a link's and not a gesture.
  const centerAskedRef = useRef(false);

  // The selection lives in the URL rather than in state, so a pasted link
  // lights the same node for a teammate. replace, not push: walking back
  // through every card a reader clicked is not history worth keeping.
  const select = (id: string | null) => {
    // Asking for the selection already on the URL is not a navigation. Most
    // clicks on a map land on empty canvas with nothing selected, and each
    // one of those replaced the location with the location it was already on,
    // the same way Escape used to before it learned to check.
    if (id === sel) {
      return;
    }
    const next = new URLSearchParams(params);
    if (id === null) {
      next.delete("sel");
    } else {
      next.set("sel", id);
    }
    setParams(next, { replace: true });
  };

  // The persist effects below read all three of these through refs so that
  // none of them is a reason to RUN, only a value to write. projectKey in
  // particular: as a direct dependency it fired the instant a key first
  // resolved, which is precisely the moment there is nothing new to save
  // (Explore.tsx:274-305 carries the same ref for the same reason).
  const projectKeyRef = useRef<string | undefined>(undefined);
  projectKeyRef.current = projectKey;
  const openRef = useRef<Set<string> | null>(null);
  openRef.current = open;
  const viewportRef = useRef<Viewport>(HOME);
  viewportRef.current = viewport;
  // The canvas's measured frame, reported up on every real resize. Fit and
  // the travel to a selection are what need it, and both need it at the
  // moment they run rather than as a reason to render.
  const frameRef = useRef({ w: 0, h: 0 });
  // "What I am about to write has been through a restore against this key."
  // Only a materialization that actually had a key sets it.
  const restoredRef = useRef(false);
  // "A reader changed this." An untouched default is never written.
  const touchedRef = useRef(false);
  // "This map came up on its defaults, so it is owed the one fit that shows
  // the reader what they are looking at." A restored viewport clears it: a
  // remembered place beats a computed one.
  const wantFitRef = useRef(false);
  // "That fit has happened." Once, per visit, whichever half arrived last.
  const fittedRef = useRef(false);

  // The open set materializes once the payload lands AND the project key has
  // settled (see the header): the stored set for this project reconciled
  // against the live tree, else the default above. It runs twice in one case
  // only, the recovery path: /api/status failed, so the first pass had no
  // key and drew the defaults without latching, and a later refresh answers
  // with one. The stored state then replaces what is on screen, because
  // nothing on screen has been through a restore yet and none of it has been
  // written anywhere.
  useEffect(() => {
    if (view === null || !ready || restoredRef.current) {
      return;
    }
    if (projectKey === undefined) {
      if (open === null) {
        setOpen(new Set());
        wantFitRef.current = true;
      }
      return;
    }
    const stored = readMapState(projectKey);
    if (stored !== null) {
      setOpen(reconcileOpen(stored.open, view));
      if (stored.viewport !== null) {
        setViewport(stored.viewport);
      }
      // Only a viewport that was actually restored cancels the fit. A record
      // written before the reader ever panned carries viewport: null, and
      // treating the record's mere existence as a place to be opened that
      // map at 1:1 on the origin, which is the middle of the root card with
      // the rest of the tree off screen.
      wantFitRef.current = stored.viewport === null;
    } else if (open === null) {
      setOpen(new Set());
      wantFitRef.current = true;
    }
    restoredRef.current = true;
  }, [view, open, projectKey, ready]);

  const persist = () => {
    const key = projectKeyRef.current;
    const current = openRef.current;
    if (key === undefined || current === null || !restoredRef.current || !touchedRef.current) {
      return;
    }
    writeMapState(key, { open: [...current], viewport: viewportRef.current });
  };

  // The open set writes eagerly: it changes once per click, so one gesture is
  // one write.
  useEffect(() => {
    persist();
  }, [open]);

  // The viewport waits for the pan to settle. MapCanvas reports a viewport on
  // every pointermove and every wheel tick, and each of those would otherwise
  // be a synchronous JSON.stringify plus setItem on the main thread, in the
  // middle of the one interaction that has to stay smooth. The timer resets
  // on each new viewport and is cleared on unmount, so a pan writes once, at
  // its end (Explore.tsx:324-326 gates its own divider write on !dragging for
  // the same reason).
  useEffect(() => {
    const timer = setTimeout(persist, VIEWPORT_SETTLE_MS);
    return () => clearTimeout(timer);
  }, [viewport]);

  // Every gesture that changes what is remembered goes through one of these
  // two, which is what arms the touch latch above.
  //
  // moveViewport has an updater form for the travel below, which has to be
  // computed from the update in the queue rather than the one this render
  // saw. The plain form is the updater form with a constant.
  const moveViewportBy = (next: (v: Viewport) => Viewport) => {
    touchedRef.current = true;
    setViewport(next);
  };
  const moveViewport = (next: Viewport) => moveViewportBy(() => next);
  const changeOpen = (next: Set<string>) => {
    touchedRef.current = true;
    setOpen(next);
  };

  // A deep link opens its ancestry, Explore's own idiom for landing on a
  // node inside a closed container: without it a sel naming a task dimmed
  // the feature holding it and never drew the card.
  //
  // setOpen, not changeOpen: this is not a gesture. Once the default is
  // collapsed every deep link opens something, so arming the
  // touch latch here would make merely following a link write memory, and
  // the involuntary pan the link came with would be frozen in as "where I
  // was". The branch this opens is remembered the moment the reader does
  // anything of their own, since persist writes whatever the open set holds
  // by then.
  //
  // The walk is over the LAYOUT's parent links, not the wire's parent field.
  // A bug or a chore names no parent on the wire at all: its container is
  // the Bugs or Chores shelf, which the layout invents. Reading the field
  // alone, a link to a bug on a branch the reader had closed found no
  // ancestry to open and left the very node it named off the map. The walk
  // stops at the root, which is nobody's container and is drawn whatever the
  // open set says.
  // It is keyed on the selection, never on the open set. Watching the open
  // set made this effect fire on its own writes: closing a branch above the
  // selected node re-opened it on the very next commit, so a collapse chevron
  // did nothing and Collapse all left the selection's ancestry standing (a
  // review fix). The functional setter is what lets it read the current set
  // without depending on it, and it also reads the set the restore effect
  // above queued in this same commit, which is the one moment the open set
  // exists but this render has not seen it. `ready` and `projectKey` are
  // there for exactly that commit: they are the only things that materialize
  // the open set, and neither of them changes on a toggle.
  useEffect(() => {
    if (view === null || selected === null) {
      return;
    }
    const parents = parentMap(view);
    setOpen((prev) => {
      if (prev === null) {
        return prev;
      }
      const shut: string[] = [];
      let current = parents.get(selected);
      while (current !== undefined && current !== ROOT_ID) {
        if (!prev.has(current)) {
          shut.push(current);
        }
        current = parents.get(current);
      }
      return shut.length === 0 ? prev : new Set([...prev, ...shut]);
    });
  }, [view, selected, ready, projectKey]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Escape with nothing selected is not this mode's business: firing
      // anyway navigated the router to the location it was already on.
      if (e.key === "Escape" && investigation !== null) {
        setInvestigation(null);
      } else if (e.key === "Escape" && selected !== null) {
        select(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const treeLayout = useMemo(
    () => (view === null || open === null ? null : layoutMap(view, open)),
    [view, open],
  );
  const overlay = useMemo(
    () => inspectionView === null || treeLayout === null ? null : relationshipOverlay(inspectionView, treeLayout, selected, showAll, relationshipKind, activeResults),
    [inspectionView, treeLayout, selected, showAll, relationshipKind, activeResults],
  );
  const layout = overlay?.layout ?? null;
  const chain = useMemo(
    () => (view === null || open === null || selected === null ? null : chainOf(view, open, selected)),
    [view, open, selected],
  );

  // The first visit fits. A map with nothing remembered used to open at 1:1
  // in its top left corner, which on any real project is the corner of one
  // epic with the rest of the tree off screen in a direction the reader has
  // to guess. It runs once, only for a map that took its defaults, and only
  // once the canvas has measured a frame: the layout and the frame arrive in
  // either order, so whichever is late does the fitting (this effect when
  // the layout is, onSize below when the frame is).
  //
  // setViewport, not moveViewport: a fit nobody asked for is not a gesture.
  // Arming the touch latch here would write the untouched default open set
  // into storage, which is the very thing that latch exists to prevent.
  useEffect(() => {
    if (layout === null || fittedRef.current || !wantFitRef.current) {
      return;
    }
    const frame = frameRef.current;
    if (frame.w === 0 || frame.h === 0) {
      return;
    }
    fittedRef.current = true;
    setViewport(fitViewport(layout, frame, rightInset));
  }, [layout]);

  // The trip itself, run from wherever both halves it needs finally arrive:
  // the card's box, which does not exist while the branch holding it is
  // still opening, and a measured frame, which the canvas reports whenever
  // it gets one. Until then the target is held rather than dropped, so
  // neither half arriving late loses the trip.
  const travel = (current: MapLayout, frame: { w: number; h: number }) => {
    const target = centerOnRef.current;
    if (target === null) {
      return;
    }
    if (target !== selected) {
      // A sel that names no node, or a reader who has already moved on:
      // there is nothing to travel to.
      centerOnRef.current = null;
      centerAskedRef.current = false;
      return;
    }
    const box = current.boxes.get(target);
    if (box === undefined || frame.w === 0 || frame.h === 0) {
      return;
    }
    centerOnRef.current = null;
    const asked = centerAskedRef.current;
    centerAskedRef.current = false;
    // The updater form, not a value: on a first visit the fit is queued
    // ahead of this in the very same commit, and centering at the zoom this
    // render happened to see would both miss and undo it.
    //
    // Horizontally it centers in the width the card leaves, the same width
    // the fit works in: a trip is only ever taken to a node that is about to
    // be selected, so there is always a card, and centering in the whole
    // frame parked the node the reader asked for behind the panel
    // describing it.
    const free = freeWidth(frame, rightInset);
    const next = (v: Viewport): Viewport => ({
      x: box.x + box.w / 2 - free / (2 * v.k),
      y: box.y + box.h / 2 - frame.h / (2 * v.k),
      k: v.k,
    });
    if (asked) {
      moveViewportBy(next);
    } else {
      // Involuntary, like the first-visit fit beside it: the map moves, and
      // the touch latch stays where it was.
      setViewport(next);
    }
  };

  // Then the map travels to the card it owes a trip to. Declared after the
  // fit on purpose, so that on a first visit the fit is the update this one
  // builds on.
  useEffect(() => {
    if (layout !== null) {
      travel(layout, frameRef.current);
    }
  }, [layout, selected]);

  if (error !== null && view === null) {
    return <p className="map-status error">Map unavailable: {error}</p>;
  }
  if (view === null || inspectionView === null || open === null || layout === null || treeLayout === null || overlay === null) {
    return <p className="map-status muted">Loading the map...</p>;
  }
  // The toolbar's Fit is the same computation the first visit runs, asked
  // for out loud, so it goes through moveViewport and is remembered. It used
  // to be Home, which on a map wider than the window left most of it as
  // unreachable as before. With no frame measured yet there is nothing to
  // fit to, so Home is the honest answer.
  const fit = () => {
    const frame = frameRef.current;
    moveViewport(frame.w === 0 || frame.h === 0 ? HOME : fitViewport(layout, frame, rightInset));
  };
  // A chevron that OPENS a branch fans its children out beside the card, and
  // nothing about the viewport says they landed where the reader can see
  // them. On the right of the map they open straight into the floating card,
  // which is where the smoke run found them: the features of the epic that
  // had just been clicked were all behind the panel, and the map gave no
  // sign of it. On the left they can open past the frame's own edge.
  //
  // So an opening chevron reveals what it made: the smallest pan that brings
  // the new drawing inside the free rect, which is the frame less the
  // toolbar's band, the margin at the other edges, and the card. A branch
  // already on screen moves nothing, since a map that jumped at every
  // chevron would be twitching under every click. A branch too big for that
  // rect is anchored by the edge nearest the card instead, which is where
  // its connectors leave the parent and so where reading it starts.
  //
  // The zoom is never touched. Fitting the new drawing instead would answer
  // one click by redrawing the whole map at a new scale, and the reader
  // asked to see one branch.
  //
  // It is a gesture, so it goes through moveViewportBy and is remembered.
  // The updater form is what reads the viewport in the queue rather than the
  // one this render saw, the same reason the trip out of the card does.
  const revealOpened = (id: string, next: Set<string>) => {
    const frame = frameRef.current;
    if (frame.w === 0 || frame.h === 0) {
      return;
    }
    const shown = revealedRect(view, layoutMap(view, next), id);
    if (shown === null) {
      return;
    }
    moveViewportBy((v) => panInto(v, shown, frame, rightInset));
  };

  // The other reveal, on the same rule and for the same reason: Tab walks
  // every card on the map in wire order, and on any project bigger than the
  // window most of them are off the frame, so focus lands on a button the
  // reader cannot see with nothing on screen to say where it went. Focusing a
  // card pans it in.
  //
  // setViewport, not moveViewportBy: tabbing through a map is reading it, not
  // moving it. Latching here would write the untouched default open set into
  // memory the first time anyone reached the canvas with a keyboard, and
  // freeze whatever card Tab happened to stop on as "where I was".
  //
  // It is only ever called for a focus the pointer did not put there. The
  // canvas is what draws that line (MapCanvas's pointerFocusRef): Chromium
  // focuses the button a mousedown lands on, and panning here on that focus
  // moved the card out from under the press, so the release landed on shifted
  // content and the chevron click was lost.
  const revealFocused = (id: string) => {
    const frame = frameRef.current;
    const box = layout.boxes.get(id);
    if (box === undefined || frame.w === 0 || frame.h === 0) {
      return;
    }
    setViewport((v) => panInto(v, box, frame, rightInset));
  };

  const toggle = (id: string) => {
    const next = new Set(open);
    // Closing takes drawing away, which cannot put anything out of view, so
    // only the opening half reveals.
    const opening = !next.has(id);
    if (opening) {
      next.add(id);
    } else {
      next.delete(id);
    }
    changeOpen(next);
    if (opening) {
      revealOpened(id, next);
    }
  };
  // Expand all and Collapse all change how much drawing there is, not just
  // which cards are on screen, so each one re-fits what it just made:
  // expanding at the collapsed zoom left the reader on a corner of a map
  // several times bigger, and collapsing left the frame zoomed out over
  // mostly empty paper. Both halves are asked for out loud, so both go
  // through the latch and are remembered. The fit is computed against the
  // layout the new open set makes rather than the one this render is
  // showing, which is the same reason the trip out of the docked card reads
  // the update in the queue instead of the value it was rendered with.
  const changeOpenAndFit = (next: Set<string>, nextSelection: string | null = selected) => {
    changeOpen(next);
    const frame = frameRef.current;
    if (frame.w > 0 && frame.h > 0) {
      const nextOverlay = relationshipOverlay(inspectionView, layoutMap(view, next), nextSelection, showAll, relationshipKind, activeResults);
      moveViewport(fitViewport(nextOverlay.layout, frame, nextSelection === null ? 0 : DOCK_INSET));
    }
  };

  const setOverlay = (all: boolean, kind: string, replaceResults = false) => {
    if (replaceResults) clearMappedResults();
    setShowAll(all);
    setRelationshipKind(kind);
    if (all || activeResults) {
      const next = relationshipOverlay(inspectionView, treeLayout, selected, all, kind, replaceResults ? null : activeResults);
      const frame = frameRef.current;
      if (next.cards.length > 0 && frame.w > 0 && frame.h > 0) {
        moveViewport(fitViewport(next.layout, frame, rightInset));
      }
    }
  };
  const workLinksActive = showAll && relationshipKind === WORK_LINK_PRESET;
  const selectRelated = (id: string, context?: MapRelationshipResults) => {
    if (context) selectContext(context);
    centerOnRef.current = id;
    centerAskedRef.current = true;
    select(id);
  };
  const showResults = (results: MapRelationshipResults) => {
    mapResults(results);
    setShowAll(false);
    setRelationshipKind("");
    setInvestigation(null);
    const next = relationshipOverlay(inspectionView, treeLayout, selected, false, "", results);
    const frame = frameRef.current;
    if (frame.w > 0 && frame.h > 0) moveViewport(fitViewport(next.layout, frame, rightInset));
  };

  return (
    <div className="map-mode">
      <div className="map-toolbar" role="toolbar" aria-label="Map controls">
        {/* Expand all opens every epic and both shelf branches, features
            left closed: the picture you want after decompose, one click away
            from the collapsed default. It reaches the Bugs and Chores
            branches too, so it undoes a Collapse all in full. Both buttons
            re-fit the map they just made. */}
        <button type="button" onClick={() => changeOpenAndFit(expandedOpen(view))}>
          Expand all
        </button>
        <button type="button" onClick={() => {
          select(null);
          changeOpenAndFit(new Set(), null);
        }}>
          Collapse all
        </button>
        <button type="button" aria-pressed={workLinksActive} onClick={() => setOverlay(!workLinksActive, workLinksActive ? "" : WORK_LINK_PRESET, true)}>
          All dependencies
        </button>
        <label className="map-kind-select">
          <select aria-label="Relationship kind" value={relationshipKind} onChange={(event) => setOverlay(showAll, event.target.value)}>
            <option value="">All kinds</option>
            <option value={WORK_LINK_PRESET}>Work links</option>
            {view.relationships.catalog.map((kind) => <option key={kind.name} value={kind.name}>{kind.name}</option>)}
          </select>
        </label>
        <button type="button" aria-pressed={investigation === "relationships"} onClick={() => setInvestigation(investigation === "relationships" ? null : "relationships")}>
          Browse relationships
        </button>
        <button type="button" aria-pressed={investigation === "issues"} onClick={() => setInvestigation(investigation === "issues" ? null : "issues")}>
          Reference issues
        </button>
        <button type="button" aria-pressed={showAll} title="Deliberately show every relationship matching the kind filter, including all contextual endpoint cards" onClick={() => setOverlay(!showAll, relationshipKind, true)}>
          Full graph
        </button>
        {activeResults && <button type="button" onClick={clearMappedResults}>Clear mapped results ({activeResults.edges.length})</button>}
        <button type="button" onClick={fit}>
          Fit
        </button>
        {overlay.cards.length > 0 && <button type="button" onClick={() => {
          const frame = frameRef.current;
          if (frame.w === 0 || frame.h === 0 || overlay.contextTop === null) return;
          let x = Infinity;
          let right = -Infinity;
          let bottom = -Infinity;
          for (const vertex of overlay.cards) {
            const box = layout.boxes.get(vertex.id);
            if (box === undefined) continue;
            x = Math.min(x, box.x);
            right = Math.max(right, box.x + box.w);
            bottom = Math.max(bottom, box.y + box.h);
          }
          const bounds = { x: x - FIT_INSET, y: overlay.contextTop - FIT_INSET, w: right - x + 2 * FIT_INSET, h: bottom - overlay.contextTop + 2 * FIT_INSET };
          moveViewport(fitViewport({ ...layout, bounds }, frame, rightInset));
        }}>Related endpoints ({overlay.cards.length})</button>}
      </div>
      <MapCanvas
        view={view}
        layout={layout}
        open={open}
        selected={selected}
        chain={chain}
        overlay={overlay}
        rootTitle={rootTitle}
        viewport={viewport}
        onViewport={moveViewport}
        onSelect={select}
        onToggle={toggle}
        onFocusCard={revealFocused}
        onSize={(next) => {
          frameRef.current = next;
          // The frame was the late half: fit and travel now, on the same
          // terms the two effects above would have.
          if (!fittedRef.current && wantFitRef.current) {
            fittedRef.current = true;
            setViewport(fitViewport(layout, next, rightInset));
          }
          travel(layout, next);
        }}
      />
      {(error !== null || loading || clearedOnRefresh) && <p className="map-refresh-status" role="status">
        {error !== null ? `Refresh failed; showing the last successful map. ${error}` : loading ? "Refreshing the map…" : "Map refreshed; previous investigation results cleared. Browse or run a query to map fresh results."}
      </p>}
      <RelationshipLegend graph={inspectionView.relationships} edges={overlay.edges} kind={relationshipKind} scope={activeResults ? "Result page" : showAll ? "Whole project" : "Selected item"} onKind={(kind) => setOverlay(showAll, kind)} />
      {!inspectionView.relationships.complete && <div className="map-extraction-warning" role="status">
        Extraction incomplete: unread or omitted project input can hide incoming relationships.
        <button type="button" className="map-card-link" onClick={() => setInvestigation("issues")}>Inspect project reference issues</button>
      </div>}
      {investigation && <RelationshipResults graph={view.relationships} issuesGraph={inspectionView.relationships} selected={selected} mode={investigation} onMode={setInvestigation} onSelect={selectRelated} onShow={showResults} onClose={() => setInvestigation(null)} />}
      {selectedNode !== undefined ? (
        <MapCard node={selectedNode} view={inspectionView} relationshipKind={relationshipKind} onSelect={selectRelated} onClose={() => select(null)}>
          <ReferenceIssues graph={inspectionView.relationships} selected={selected} onSelect={selectRelated} />
        </MapCard>
      ) : selectedVertex !== undefined && (
        <RelationshipCard vertex={selectedVertex} edges={inspectionView.relationships.edges} byID={overlay.byID} kind={relationshipKind} onSelect={selectRelated} onClose={() => select(null)}>
          <ReferenceIssues graph={inspectionView.relationships} selected={selected} onSelect={selectRelated} />
        </RelationshipCard>
      )}
    </div>
  );
}
