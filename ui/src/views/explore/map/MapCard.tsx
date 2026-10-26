// MapCard is the detail for the selected node (docs/spec/ui-surface.md): kind
// pill, title, chip, key, the container it sits in, contributors or owner, the
// questions holding it, the node's dependencies and dependents (each a button
// that selects that node on the map), a shelf node's found-in and carried-by
// targets, and the one plain Open link to the node page, which is the drill-in.
// The map itself never navigates on a click.
//
// It is a column: a header carrying the kind line and the close control, and
// a body under it that scrolls when a node has more to say than the card has
// room for. The whole card was the scrolling box until the close control it
// held started scrolling away with everything else.
//
// It floats at the top right of the map rather than filling a sheet down the
// whole right edge (2026-08-31): the sheet was mostly empty paper for the
// small nodes that are most of a tree, and it hid the map underneath itself
// while the fit and the trip to a selection went on centering in the whole
// frame, which is what put a selection's own counterparts behind it. The
// panel's shape is map.css's .map-dock; what it covers is DOCK_INSET, which
// MapMode centers around.
//
// The header is also its handle (2026-08-31, the detail pane pass): a press
// on it and a drag moves the card, clamped so it never leaves the frame it
// floats in and never rises above its own home row, which is the toolbar's
// band. The offset is this component's own state, so it holds while the card
// stays up (a click on one of the card's own links keeps the card where the
// reader put it) and resets when the card closes and remounts; a reload of
// the map remounts it too. MapMode's fit and travel go on centering around
// the home position: a reader who dragged the card over the map chose that,
// and the map does not chase it.
//
// Its relation lists are drawn as trees: the label is the branch and the
// rows hang under it by an L-bracket (map.css's .map-dock-tree), with the
// count of rows beside the label once there is more than one. Before that
// the label, the rows, and the title all sat in one register and a long
// dependents list read as a wall.
import type { PointerEvent as ReactPointerEvent, ReactElement, ReactNode } from "react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { MapNodeView, MapView } from "../../../api";
import Chip from "../../../components/Chip";
import { kindLabel } from "../../../lib/kind";
import Contributors from "../../../components/Contributors";
import { PersonName } from "../../../people";
import { SelectedRelationships } from "./RelationshipDetails";

// What the floating card covers on the right of the map, in screen pixels:
// its own 320 of width, plus the 16 of margin it sits in, plus that margin
// again as breathing room, so a card centered in what is left never runs up
// against it. MapMode's fit and travel work in frame.w - DOCK_INSET, which
// is what keeps a fitted map, and the counterparts of whatever is selected,
// out from under the panel. The literals are map.css's own .map-dock rule;
// the two have to be changed together.
export const DOCK_INSET = 352;

// How far the card has been dragged from its home in the top right corner.
interface Offset {
  x: number;
  y: number;
}

// A drag in progress: where the pointer went down, and the offset the card
// wore then. Every move is that offset plus the pointer's travel, fitted to
// the frame.
interface Drag {
  startX: number;
  startY: number;
  from: Offset;
}

const HOME: Offset = { x: 0, y: 0 };

function atHome(o: Offset): boolean {
  return o.x === 0 && o.y === 0;
}

// Clamped to [lo, hi]; a frame too small to hold the card (hi < lo) pins it
// to the frame's leading edge rather than letting the two limits fight.
function clamp(v: number, lo: number, hi: number): number {
  return Math.min(Math.max(v, lo), Math.max(lo, hi));
}

// The offset nearest `want` that keeps the whole card inside the frame it
// floats in (.map-mode, the card's parent, which clips what runs past it)
// and no higher than its home row. The card's home is its rect less the
// offset it is wearing right now, so this is asked at every move, when the
// node changes (a longer list makes a taller card), and when the window is
// resized (the card is anchored to the frame's right edge, so a narrower
// frame carries a card dragged to the left edge out past it). It is the
// identity when the card has no frame to measure, which is how it reads
// before the first layout.
function fit(dock: HTMLElement | null, want: Offset, wearing: Offset): Offset {
  const frame = dock?.parentElement;
  if (dock === null || frame === null || frame === undefined) {
    return want;
  }
  const card = dock.getBoundingClientRect();
  const box = frame.getBoundingClientRect();
  const home = { left: card.left - wearing.x, right: card.right - wearing.x, bottom: card.bottom - wearing.y };
  const next = {
    x: clamp(want.x, box.left - home.left, box.right - home.right),
    y: clamp(want.y, 0, box.bottom - home.bottom),
  };
  return next.x === wearing.x && next.y === wearing.y ? wearing : next;
}

export default function MapCard({
  node,
  view,
  onSelect,
  onClose,
  children,
  relationshipKind = "",
}: {
  node: MapNodeView;
  view: MapView;
  onSelect: (id: string) => void;
  onClose: () => void;
  children?: ReactNode;
  relationshipKind?: string;
}): ReactElement {
  const byID = new Map(view.relationships.vertices.map((vertex) => [vertex.id, vertex]));
  const holds = node.holds ?? 0;
  const title = node.title ?? node.key ?? node.id;

  const dockRef = useRef<HTMLElement>(null);
  const [offset, setOffset] = useState<Offset>(HOME);
  const drag = useRef<Drag | null>(null);
  const refit = () => setOffset((wearing) => fit(dockRef.current, wearing, wearing));

  // A card that grew taller with a new node, or a frame that shrank under
  // it, may have carried the card out of bounds without any drag at all.
  useLayoutEffect(refit, [node.id]);
  useEffect(() => {
    window.addEventListener("resize", refit);
    return () => window.removeEventListener("resize", refit);
  }, []);

  // The press that starts a drag. It has to be the primary button, land on
  // the header itself rather than on the close control (whose own click is
  // the one thing a press there should mean), and find no drag already in
  // flight, so a second pointer cannot take the card over mid-gesture.
  //
  // The press is cancelled once it is a drag. The header declines selection
  // in CSS, but WebKit anchors a selection at the nearest selectable text
  // and extends it as the captured pointer moves, so a drag down the card
  // highlighted the title, the key, and the first rows under it. Cancelling
  // the pointerdown is what keeps the selection from ever starting.
  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 || drag.current !== null || (e.target as Element).closest("button") !== null) {
      return;
    }
    e.preventDefault();
    drag.current = { startX: e.clientX, startY: e.clientY, from: offset };
    e.currentTarget.setPointerCapture?.(e.pointerId);
  };
  const onPointerUp = () => {
    drag.current = null;
  };
  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (d === null) {
      return;
    }
    // A move that reports no button held is a release this header never
    // saw: a context menu opened mid-drag, or the button let go over another
    // window. It ends the drag the way the missing pointerup would have
    // (the same wedge Explore's nav splitter guards against).
    if (e.buttons === 0) {
      onPointerUp();
      return;
    }
    const want = { x: d.from.x + e.clientX - d.startX, y: d.from.y + e.clientY - d.startY };
    setOffset((wearing) => fit(dockRef.current, want, wearing));
  };


  return (
    <aside
      ref={dockRef}
      className="map-dock"
      aria-label="Selected node"
      style={atHome(offset) ? undefined : { transform: `translate(${offset.x}px, ${offset.y}px)` }}
    >
      {/* The header holds still and the body under it scrolls. The card used
          to be one scrolling box with the close control absolutely positioned
          inside it, so a node carrying a long list of dependencies scrolled
          its own close button off the top. */}
      <div
        className="map-dock-head"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onLostPointerCapture={onPointerUp}
      >
        <div className="kind-line">
          <span className="kind-pill">{kindLabel(node.noun)}</span>
          <Chip status={node.status} size="hero" />
        </div>
        <button type="button" className="map-dock-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      </div>
      <div className="map-dock-body">
        <h2 className="map-dock-title">{title}</h2>
        {node.key !== undefined && <p className="map-dock-key mono">{node.key}</p>}
        {(node.noun === "epic" || node.noun === "feature" || node.owner !== undefined || holds > 0) && (
          <div className="map-dock-facts">
            {node.noun === "epic" || node.noun === "feature" ? (
              <p className="map-dock-fact">Contributors <Contributors people={node.contributors ?? []} /></p>
            ) : node.owner !== undefined && (
              <p className="map-dock-fact">
                Owner <b><PersonName id={node.owner} name={node.owner_name} /></b>
              </p>
            )}
            {holds > 0 && (
              <p className="map-dock-holds">
                <Link to="/questions">{`${holds} question${holds === 1 ? "" : "s"} holding it`}</Link>
              </p>
            )}
          </div>
        )}
        <SelectedRelationships selected={node.id} edges={view.relationships.edges} byID={byID} kind={relationshipKind} onSelect={onSelect} />
        {children}
        <Link className="map-dock-open" to={`/node/${node.noun}/${node.id}`}>
          Open
        </Link>
      </div>
    </aside>
  );
}
