// MapCanvas is the Explore map's drawing (docs/spec/ui-surface.md): a frame div
// that owns every gesture (drag on empty canvas pans, the wheel zooms about the
// pointer, a resize is measured off it) and two layers stacked inside it.
// Below, an SVG holding the wires alone, the layout's branch connectors and its
// dependency arcs, in a group the viewport transforms. Above, a plain HTML
// layer holding one one-row card per box, absolutely positioned in world units,
// transformed by the very same expression. Nothing either layer draws is a
// function of the frame's measured size, so the two cannot come apart: the
// measure exists only to tell MapMode what to fit and travel against.
//
// The cards used to be foreignObject content inside that SVG, which WebKit
// gets wrong twice over: it mispositions a mirrored row inside one (a
// left-side card painted its chevron detached, far outside the card), and
// it fails to repaint or clip one when the viewBox changes, leaving ghost
// cards at old positions, cards that vanish, and cards drawn over the
// toolbar. Neither is reachable from an ordinary div, so the whole bug
// class goes with the element.
//
// The gestures used to hang off a background rect inside the SVG, a sibling
// of the cards. On the frame they are an ancestor of everything drawn
// instead, so both the press and the click ask whether they landed on the
// frame itself before acting: a card's own events bubble through here now,
// and answering them would pan the map out from under a card and deselect
// the node a click had just selected.
//
// Two things are drawn that the wire never names: the root card, the project
// itself at the center, which is the one card that cannot be selected, and a
// shelf card per non-empty shelf, the head of the Bugs or Chores branch.
// Everything else is rendered off view.nodes rather than off the layout's
// placement, so Tab walks the cards in the order the tree lists them; each
// shelf card is emitted just before the first node it carries.
//
// Dependency arcs are drawn by policy rather than always: at rest the picture
// is the tree alone, a selection lights the arcs that touch it (its own
// dependencies and dependents, and the shelf edges at either end of it), and
// the Work links preset draws every one of them. A card is a button
// (Enter selects); a branch's chevron is a second button that toggles
// without selecting. Selection paints three classes: selected, lit (one step
// away on the selected card's own edges), dim (everything the selection does
// not reach); the rest of the transitive chain, and the branches holding any
// of it, wear neither and stay at full strength.
import type {
  CSSProperties,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  ReactElement,
} from "react";
import { Fragment, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { Link } from "react-router-dom";
import type { MapView } from "../../../api";
import StateDot from "../../../components/StateDot";
import { kindLabel } from "../../../lib/kind";
import { relationshipHref } from "../../../lib/relationshipHref";
import { relationshipTitle } from "../../../lib/relationshipTitle";
import type { Chain } from "./chain";
import type { Box, MapLayout } from "./layout";
import { ROOT_ID, SHELF_BUG, SHELF_CHORE, SHELF_TWEAK, containers, parentMap } from "./layout";
import type { Viewport } from "./mapStorage";
import type { RelationshipOverlay } from "./relationships";
import { relationshipStroke } from "./relationships";

export interface MapCanvasProps {
  view: MapView;
  layout: MapLayout;
  open: ReadonlySet<string>;
  selected: string | null;
  chain: Chain | null;
  overlay: RelationshipOverlay;
  rootTitle: string;
  viewport: Viewport;
  onViewport: (v: Viewport) => void;
  onSelect: (id: string | null) => void;
  onToggle: (id: string) => void;
  onSize?: (size: { w: number; h: number }) => void;
  // A card took focus, from Tab or from anything else that moves it. The
  // canvas draws wherever the viewport says and knows nothing about the
  // frame, so what to do about a card the reader cannot see is MapMode's
  // call, the same as the fit and the trip to a selection.
  onFocusCard?: (id: string) => void;
}

// The floor a fit shares with the wheel: a whole Acme map is several
// thousand pixels wide, and at 0.25 the frame could not hold it.
export const MIN_ZOOM = 0.1;
const MAX_ZOOM = 3;
// A press that travels less than this many pixels is a click, not a pan, so
// releasing a real pan does not also clear the selection.
const DRAG_SLOP = 3;

// What one card draws, resolved once per render off the wire node or the
// synthetic shelf behind it. The canvas never reads MapNodeView again below
// this, so the root, a shelf, and an epic all render through one path.
interface Card {
  box: Box;
  title: string;
  status: string;
  holds: number;
  // Children on the branch, drawn as the count on its card and said in its
  // accessible name. Only an epic and a shelf carry one; every other card
  // leaves it at zero, since nothing reads it there.
  count: number;
  canToggle: boolean;
}

const SHELF_TITLE = new Map([
  [SHELF_BUG, "Bugs"],
  [SHELF_CHORE, "Chores"],
  [SHELF_TWEAK, "Tweaks"],
]);
const SHELF_NOUN = new Map([
  [SHELF_BUG, "bug"],
  [SHELF_CHORE, "chore"],
  [SHELF_TWEAK, "tweak"],
]);

export default function MapCanvas({
  view,
  layout,
  open,
  selected,
  chain,
  overlay,
  rootTitle,
  viewport,
  onViewport,
  onSelect,
  onToggle,
  onSize,
  onFocusCard,
}: MapCanvasProps): ReactElement {
  const drag = useRef<{ x: number; y: number; vx: number; vy: number } | null>(null);
  const panned = useRef(false);
  const frameRef = useRef<HTMLDivElement>(null);
  // The last size actually measured, so a resize that reports the same
  // numbers is not a change and the frame is reported upward once per real
  // one. It starts at nothing, which no frame ever measures, so the first
  // measure always reports. The canvas keeps no size of its own beyond this:
  // nothing it DRAWS is a function of the frame, only the fit and the travel
  // MapMode computes off what it is told here.
  const measured = useRef({ w: 0, h: 0 });
  // onSize rides a ref for the same reason MapMode's own writes do: it is a
  // value the measure effect uses, never a reason to re-run it.
  const onSizeRef = useRef(onSize);
  onSizeRef.current = onSize;

  const hasChildren = containers(view);
  // The virtual tree's parent links, taken once for the payload rather than
  // per lookup: a pan re-renders this component on every pointermove, and the
  // ancestry walk below reads a link per step.
  const parents = useMemo(() => parentMap(view), [view]);
  // A branch's child count, off those same links rather than the wire's, so
  // the Bugs and Chores branches count what hangs on them.
  const childCount = new Map<string, number>();
  for (const n of view.nodes) {
    const parent = parents.get(n.id);
    if (parent !== undefined) {
      childCount.set(parent, (childCount.get(parent) ?? 0) + 1);
    }
  }

  // Every card the selection lights: the ones one step away on its own
  // depends_on edges, plus the far end of each of its own shelf edges. Taken
  // once per render, since it is asked about per card.
  const related = useMemo(
    () => new Set<string>(chain === null ? [] : [...chain.near, ...chain.linked]),
    [chain],
  );
  // Every card the selection reaches at all: the lit ones and the rest of
  // the transitive chain in both directions, which stays undimmed without
  // being lit, so the reader can still see what the node ultimately rests on
  // and what rests on it without the arcs of the whole reach on screen.
  const reach = useMemo(
    () => new Set<string>(chain === null ? [] : [...chain.up, ...chain.down, ...related]),
    [chain, related],
  );

  // Every branch holding the selection or a card on its chain, walked up the
  // virtual tree once per render. A branch has to stay undimmed for the chain
  // inside it to read, and a bug's parent is its shelf, so a lit bug lights
  // the Bugs card. The root is seeded in because the project is never the
  // thing that fell off the chain.
  const litContainers = new Set<string>([ROOT_ID]);
  if (selected !== null) {
    const markAncestors = (id: string) => {
      let current = parents.get(id);
      while (current !== undefined) {
        litContainers.add(current);
        current = parents.get(current);
      }
    };
    markAncestors(selected);
    for (const id of reach) {
      markAncestors(id);
    }
  }

  const cards: Card[] = [];
  const shelved = new Set<string>();
  const pushShelf = (id: string) => {
    const box = layout.boxes.get(id);
    if (shelved.has(id) || box === undefined) {
      return;
    }
    shelved.add(id);
    cards.push({
      box,
      title: SHELF_TITLE.get(id) ?? id,
      status: "",
      holds: 0,
      count: childCount.get(id) ?? 0,
      canToggle: hasChildren.has(id),
    });
  };
  for (const n of view.nodes) {
    if (n.noun === "bug") {
      pushShelf(SHELF_BUG);
    } else if (n.noun === "chore") {
      pushShelf(SHELF_CHORE);
    } else if (n.noun === "tweak") {
      pushShelf(SHELF_TWEAK);
    }
    const box = layout.boxes.get(n.id);
    if (box === undefined) {
      continue;
    }
    cards.push({
      box,
      title: n.title ?? n.key ?? n.id,
      status: n.status,
      holds: n.holds ?? 0,
      count: n.noun === "epic" ? (childCount.get(n.id) ?? 0) : 0,
      canToggle: hasChildren.has(n.id),
    });
  }

  // Highlight only the selected node's own relationships, even in global
  // mode. Several unrelated nodes can share a collapsed endpoint card.
  const arcs = overlay.arcs
    .map((arc) => ({ arc, lit: selected !== null && (arc.edge.from === selected || arc.edge.to === selected) }))
    .sort((a, b) => Number(a.lit) - Number(b.lit));
  const semanticNear = new Set<string>();
  for (const { arc, lit } of arcs) {
    if (lit) {
      semanticNear.add(arc.from);
      semanticNear.add(arc.to);
    }
  }

  // The gestures sit on the frame, which is an ancestor of everything drawn,
  // so both of these ask whether the press or the click landed on the frame
  // ITSELF rather than on something inside it. The old canvas hung them on a
  // background rect that was a SIBLING of the cards, and got the same answer
  // for free; from up here a card's own press and click bubble through, and
  // taken at face value they would pan the map out from under a card and
  // deselect the node the click had just selected.
  const onFrame = (e: { target: EventTarget | null; currentTarget: EventTarget | null }): boolean =>
    e.target === e.currentTarget;

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 || !onFrame(e)) {
      return;
    }
    drag.current = { x: e.clientX, y: e.clientY, vx: viewport.x, vy: viewport.y };
    panned.current = false;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  };
  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (drag.current === null) {
      return;
    }
    const dx = e.clientX - drag.current.x;
    const dy = e.clientY - drag.current.y;
    if (Math.abs(dx) > DRAG_SLOP || Math.abs(dy) > DRAG_SLOP) {
      panned.current = true;
    }
    onViewport({ x: drag.current.vx - dx / viewport.k, y: drag.current.vy - dy / viewport.k, k: viewport.k });
  };
  const onPointerUp = () => {
    drag.current = null;
  };

  // Whether the focus about to land was put there by a pointer. Chromium
  // focuses the button a mousedown lands on, and answering that focus by
  // panning the map moved the card out from under the press: the release
  // then landed on shifted content and the chevron click was lost outright
  // (dual-engine smoke, review round 3, which WebKit passed because it does
  // not focus a button on click at all).
  //
  // :focus-visible is the browser's own answer to this question and cannot be
  // used: jsdom does not match it, so the rule would be untestable. The
  // modality is tracked here instead, on the pointer that raises it.
  //
  // Armed in the capture phase, so a press anywhere inside the frame arms it
  // before the press reaches the card it landed on, and disarmed by the
  // release of that same press. The timeout is the belt to that braces: a
  // press whose release never reaches the frame, because the pointer left the
  // window or the browser took it for a gesture, would otherwise leave the
  // guard armed and the keyboard reveal dead for the rest of the visit. The
  // focus a click causes has already happened by the time a task runs.
  const pointerFocusRef = useRef(false);
  const pointerFocusTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const armPointerFocus = () => {
    pointerFocusRef.current = true;
    clearTimeout(pointerFocusTimer.current);
    pointerFocusTimer.current = setTimeout(() => {
      pointerFocusRef.current = false;
    }, 0);
  };
  const disarmPointerFocus = () => {
    clearTimeout(pointerFocusTimer.current);
    pointerFocusRef.current = false;
  };
  useEffect(() => () => clearTimeout(pointerFocusTimer.current), []);
  // The click that closes a press deselects, but the click that closes a pan
  // does not: the reader dragged the map, they did not let go of the node
  // they were looking at.
  const onCanvasClick = (e: ReactMouseEvent<HTMLDivElement>) => {
    if (!onFrame(e)) {
      return;
    }
    if (panned.current) {
      panned.current = false;
      return;
    }
    onSelect(null);
  };

  // The wheel is a native listener rather than React's own onWheel prop:
  // React registers wheel on the root as a passive listener, where
  // preventDefault is ignored and the browser logs an error, so the page
  // behind the map would scroll while the map zoomed.
  //
  // What it does rides a ref, so the listener itself is registered once per
  // mount. Read straight off the props, the effect had to name the viewport
  // and onViewport as dependencies, which meant a removeEventListener and an
  // addEventListener on every tick of the very zoom it was answering (the
  // same ref onSize below carries, for the same reason).
  const wheelRef = useRef<(e: WheelEvent) => void>(() => undefined);
  wheelRef.current = (e: WheelEvent) => {
    const frame = frameRef.current;
    if (frame === null) {
      return;
    }
    e.preventDefault();
    const box = frame.getBoundingClientRect();
    const factor = Math.exp(-e.deltaY * 0.0015);
    const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, viewport.k * factor));
    // Zoom about the pointer: the map point under the cursor stays put.
    const px = viewport.x + (e.clientX - box.left) / viewport.k;
    const py = viewport.y + (e.clientY - box.top) / viewport.k;
    onViewport({ x: px - (e.clientX - box.left) / k, y: py - (e.clientY - box.top) / k, k });
  };
  useEffect(() => {
    const frame = frameRef.current;
    if (frame === null) {
      return;
    }
    const onWheel = (e: WheelEvent) => wheelRef.current(e);
    frame.addEventListener("wheel", onWheel, { passive: false });
    return () => frame.removeEventListener("wheel", onWheel);
  }, []);

  // Measure on mount, then follow the frame. ResizeObserver is absent in
  // jsdom, so there the mount measure is all there is.
  useLayoutEffect(() => {
    const frame = frameRef.current;
    if (frame === null) {
      return;
    }
    const measure = () => {
      const box = frame.getBoundingClientRect();
      if (box.width <= 0 || box.height <= 0) {
        return;
      }
      if (measured.current.w === box.width && measured.current.h === box.height) {
        return;
      }
      measured.current = { w: box.width, h: box.height };
      onSizeRef.current?.(measured.current);
    };
    measure();
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(frame);
    return () => observer.disconnect();
  }, []);

  // World to screen, computed once and spelled twice. A viewport is the world
  // point at the frame's top left plus a zoom, so each layer moves that point
  // to the origin (in screen pixels, hence the k) and then scales the world
  // around it. Translate first, so the scale never moves the anchor.
  //
  // The wires used to say it as a viewBox instead, which made them the one
  // thing on the canvas whose mapping ran through the measured frame. A
  // viewBox fits a world rectangle INTO the frame under xMidYMid meet, so
  // between a resize and the measure that answers it the wires letterboxed
  // off the cards they run to. Off these two numbers there is nothing left
  // to disagree about: same expression, once in CSS units and once in SVG's.
  const panX = -viewport.x * viewport.k;
  const panY = -viewport.y * viewport.k;
  const worldTransform = `translate(${panX}px, ${panY}px) scale(${viewport.k})`;
  const wireTransform = `translate(${panX} ${panY}) scale(${viewport.k})`;
  const rootBox = layout.boxes.get(ROOT_ID);

  const classOf = (id: string, extra?: string): string => {
    const classes = extra === undefined ? ["map-card"] : ["map-card", extra];
    if (selected === id) {
      classes.push("selected");
    } else if (related.has(id) || semanticNear.has(id)) {
      classes.push("lit");
    } else if (selected !== null && !reach.has(id) && !litContainers.has(id)) {
      classes.push("dim");
    }
    return classes.join(" ");
  };

  // A card's own place in the layer, in world units: the layer's scale is
  // what turns these into screen pixels, which is what makes a card's text
  // zoom with the drawing rather than staying one size over a shrinking map.
  const slotOf = (box: Box): CSSProperties => ({
    left: `${box.x}px`,
    top: `${box.y}px`,
    width: `${box.w}px`,
    height: `${box.h}px`,
  });

  return (
    <div
      ref={frameRef}
      className="map-canvas"
      role="application"
      aria-label="Project map"
      onPointerDownCapture={armPointerFocus}
      onPointerUpCapture={disarmPointerFocus}
      onPointerCancelCapture={disarmPointerFocus}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onClick={onCanvasClick}
    >
      {/* The wires, and nothing else: the branch connectors under the arcs,
          the lit arcs last, and the arrowhead they share. Nothing in here
          ever takes a pointer, so the whole layer is inert and a press over
          it reaches the frame. */}
      <svg className="map-svg" aria-hidden="true">
        <defs>
          <marker id="map-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="9" markerHeight="9" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" className="map-arrowhead" />
          </marker>
        </defs>
        {/* No viewBox on the SVG: its own user units are the frame's pixels,
            and this group is what carries the world, the same expression the
            card layer carries. */}
        <g className="map-wires" transform={wireTransform}>
          {layout.branches.map((b) => (
            <path key={b.key} className="map-branch" d={b.d} />
          ))}
          {/* Each arc rides on a halo in the paper, drawn just under it: where
              an arc crosses a connector or another arc, the one painted later
              cuts a gap through the one beneath, so the eye can tell which
              line continues. A fragment rather than a group, so every arc
              stays a direct child the tests and the styles already address. */}
          {arcs.map(({ arc, lit }) => {
            const stroke = relationshipStroke(arc.edge.kind, view.relationships.catalog);
            return (
              <Fragment key={arc.edge.id}>
                <path className="map-edge-halo" d={arc.d} />
                <path
                  className={lit ? "map-edge lit" : "map-edge"}
                  data-kind={arc.edge.kind}
                  d={arc.d}
                  style={{ stroke: stroke.color, strokeDasharray: stroke.dash }}
                  markerEnd={arc.edge.kind === "attached_to" ? undefined : "url(#map-arrow)"}
                />
              </Fragment>
            );
          })}
        </g>
      </svg>
      {/* The cards, plain HTML over the drawing rather than inside it. The
          layer sits at the frame's origin and carries the same world
          transform the wire group does, so a card says nothing but its own
          box in world units and the scale zooms its text exactly as it
          zooms the wires. The layer itself is inert; each card takes its
          own pointer back. */}
      <div className="map-card-layer" style={{ transformOrigin: "0 0", transform: worldTransform }}>
        {rootBox !== undefined && (
          // The project is not a node, so it is not selectable and carries no
          // button; clicking it reads as clicking past the map, which is what
          // the rest of the canvas does too.
          <div
            className={classOf(ROOT_ID, "root")}
            data-id={ROOT_ID}
            data-noun="root"
            data-side="center"
            style={slotOf(rootBox)}
            onClick={() => onSelect(null)}
          >
            <span className="map-title">{rootTitle}</span>
          </div>
        )}
        {cards.map((card) => (
          <div
            key={card.box.id}
            className={classOf(card.box.id)}
            data-id={card.box.id}
            data-noun={card.box.noun}
            data-side={card.box.side}
            style={slotOf(card.box)}
            // On the card rather than on the button inside it, so the
            // chevron beside it reports the same card: React's onFocus is
            // the bubbling focusin, and both buttons are children of this.
            // Only a focus the pointer did not put there is reported, or a
            // click would pan the card out from under its own press.
            onFocus={() => {
              if (!pointerFocusRef.current) {
                onFocusCard?.(card.box.id);
              }
            }}
          >
            {card.canToggle && (
              <button
                type="button"
                className="map-chevron"
                aria-expanded={open.has(card.box.id)}
                aria-label={`${open.has(card.box.id) ? "Collapse" : "Expand"} ${card.title}`}
                onClick={(e) => {
                  e.stopPropagation();
                  onToggle(card.box.id);
                }}
              >
                <span className="chevron-glyph" aria-hidden="true">▸</span>
              </button>
            )}
            <button
              type="button"
              className="map-card-body"
              title={card.title}
              aria-label={cardLabel(card)}
              // A second click on the selected card puts it out: the click
              // that lit a node is the natural way to say never mind, and
              // reaching for empty canvas or Escape was an extra step
              // (JC, 2026-08-31).
              onClick={() => onSelect(selected === card.box.id ? null : card.box.id)}
            >
              <CardFace card={card} />
            </button>
          </div>
        ))}
        {overlay.contextTop !== null && (
          <div className="map-context-heading" style={{ left: -140, top: overlay.contextTop - 32 }}>
            Related endpoints
          </div>
        )}
        {overlay.cards.map((vertex) => {
          const box = layout.boxes.get(vertex.id);
          if (box === undefined) return null;
          const href = relationshipHref(vertex);
          return (
            <div
              key={vertex.id}
              className={classOf(vertex.id, "map-endpoint-card")}
              data-id={vertex.id}
              data-noun={vertex.kind}
              style={slotOf(box)}
              onFocus={() => { if (!pointerFocusRef.current) onFocusCard?.(vertex.id); }}
            >
              <button
                type="button"
                className="map-card-body"
                aria-label={`Inspect ${vertex.kind} ${relationshipTitle(vertex)}${vertex.unresolved ? ", unresolved" : ""}`}
                aria-pressed={selected === vertex.id}
                onClick={() => onSelect(selected === vertex.id ? null : vertex.id)}
              >
                <span className="kind-pill">{kindLabel(vertex.kind)}</span>
                {vertex.status && <StateDot status={vertex.status} />}
                <span className="map-title" title={relationshipTitle(vertex)}>{relationshipTitle(vertex)}</span>
              </button>
              <div className="map-endpoint-meta">
                <span>{vertex.unresolved ? "Unresolved" : vertex.generated ? "Generated document" : vertex.path || vertex.id}</span>
                {href !== null && <Link to={href}>Open</Link>}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// CardFace is the one row inside a card's button. A branch wears its kind as
// a pill and its size as a count; a leaf wears its state as a dot, since a
// task among forty tasks needs its color more than it needs its noun spelled
// out again. A shelf's pill IS its name, so it draws no separate title.
function CardFace({ card }: { card: Card }): ReactElement {
  const shelf = SHELF_TITLE.has(card.box.id);
  const branch = card.box.noun === "epic" || shelf;
  return (
    <>
      {branch ? (
        <span className="kind-pill">{shelf ? card.title : kindLabel(card.box.noun)}</span>
      ) : (
        <StateDot status={card.status} />
      )}
      {!shelf && <span className="map-title">{card.title}</span>}
      {branch && card.count > 0 && <span className="map-count">{card.count}</span>}
      {card.holds > 0 && (
        <span className="map-holds" title={`${card.holds} open question${card.holds === 1 ? "" : "s"} holding this`}>
          {card.holds}
        </span>
      )}
    </>
  );
}

// cardLabel is the card button's accessible name, said in words. Read off the
// contents instead, the name would run the pill and the title together and
// end on a bare count, which is a digit with no noun beside it: this is the
// sentence the docked card prints.
function cardLabel(card: Card): string {
  const shelfNoun = SHELF_NOUN.get(card.box.id);
  if (shelfNoun !== undefined) {
    return `${card.title}, ${plural(card.count, shelfNoun)}`;
  }
  let said = `${kindLabel(card.box.noun)} ${card.title}, ${card.status}`;
  if (card.box.noun === "epic" && card.count > 0) {
    said += `, ${plural(card.count, "feature")}`;
  }
  if (card.holds > 0) {
    said += `, ${plural(card.holds, "question")} holding it`;
  }
  return said;
}

function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}
