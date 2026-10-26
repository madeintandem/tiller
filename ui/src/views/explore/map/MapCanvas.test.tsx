// The canvas draws the mind map: the project as the root card at the center,
// the layout's branch connectors as the only lines at rest, one one-row card
// per visible node, the Bugs and Chores branches around their shelf nodes,
// and dependency arcs only for the selected chain or the Work links
// toggle. The docked card is MapCard's own suite below.
//
// It is drawn in two layers inside one frame: an SVG holding the wires, and
// an HTML layer of cards over it sharing the same world transform. The cards
// used to be foreignObject content inside the SVG, which is a shape WebKit
// gets wrong twice over, so the structural assertions below pin the layers
// apart and one of them is a standing tripwire on foreignObject itself.
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Mock } from "vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MapView } from "../../../api";
import { chainOf } from "./chain";
import { ROOT_ID, SHELF_BUG, SHELF_CHORE, layoutMap } from "./layout";
import MapCanvas from "./MapCanvas";
import MapCard from "./MapCard";
import type { Viewport } from "./mapStorage";
import { relationshipOverlay } from "./relationships";

// This suite runs without vitest's globals, so Testing Library's own
// auto-cleanup is never registered and every render would otherwise pile up
// in the same document (the repo's standing idiom, see
// components/__tests__/explore.test.tsx).
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const view: MapView = {
  nodes: [
    { noun: "epic", id: "E-1", key: "foundation", title: "Foundation", status: "ready", owner: "jc" },
    { noun: "feature", id: "F-1", key: "foundation/skeleton", title: "Skeleton", status: "ready", parent: "E-1" },
    { noun: "task", id: "T-1", key: "foundation/skeleton/init", title: "Init", status: "done", parent: "F-1" },
    { noun: "feature", id: "F-2", key: "foundation/ci", title: "CI", status: "draft", parent: "E-1", holds: 2 },
    { noun: "bug", id: "B-1", key: "bugs/blank", title: "Blank on reload", status: "proposed" },
  ],
  edges: [
    { from: "F-2", to: "F-1", kind: "depends_on" },
    { from: "B-1", to: "F-2", kind: "attached_to" },
  ],
  relationships: { vertices: [], edges: [], catalog: [], diagnostics: [], complete: true },
};

view.relationships = {
  ...view.relationships,
  vertices: view.nodes.map((node) => ({ ...node, kind: node.noun, destination: { kind: "node", noun: node.noun, id: node.id } })),
  edges: [
    ...view.edges.map((edge) => ({ ...edge, id: `${edge.kind}:${edge.from}:${edge.to}`, sources: [] })),
    ...view.nodes.filter((node) => node.parent).map((node) => ({ id: `parent:${node.id}`, kind: "contains", from: node.parent!, to: node.id, sources: [] })),
  ],
};

function renderCanvas(overrides: Partial<Parameters<typeof MapCanvas>[0]> & { showAll?: boolean } = {}) {
  const open = overrides.open ?? new Set(["E-1", SHELF_BUG]);
  const source = overrides.view ?? view;
  const payload: MapView = {
    ...source,
    relationships: {
      ...source.relationships,
      vertices: source.nodes.map((node) => ({ ...node, kind: node.noun })),
      edges: source.edges.map((edge) => ({ ...edge, id: `${edge.kind}:${edge.from}:${edge.to}`, sources: [] })),
    },
  };
  const layout = overrides.layout ?? layoutMap(payload, open);
  const props = {
    view: payload,
    layout,
    open,
    selected: null,
    chain: null,
    rootTitle: "Acme",
    viewport: { x: 0, y: 0, k: 1 },
    onViewport: vi.fn(),
    onSelect: vi.fn(),
    onToggle: vi.fn(),
    ...overrides,
    overlay: overrides.overlay ?? relationshipOverlay(payload, layout, overrides.selected ?? null, overrides.showAll ?? false, ""),
  };
  return { ...render(<MemoryRouter><MapCanvas {...props} /></MemoryRouter>), props };
}

describe("MapCanvas: the mind map", () => {
  it("draws the root card with the project name, one card per visible node, and the shelf branch", () => {
    const { container } = renderCanvas();
    expect(container.querySelector(`.map-card[data-id="${ROOT_ID}"]`)?.textContent).toBe("Acme");
    expect(container.querySelectorAll(".map-card")).toHaveLength(6); // root, E-1, F-1, F-2, shelf:bug, B-1
    expect(within(container.querySelector(`.map-card[data-id="${SHELF_BUG}"]`) as HTMLElement).getByText("Bugs")).toBeInTheDocument();
  });

  it("draws no foreignObject at all", () => {
    // The tripwire on the whole WebKit bug class this structure exists to
    // leave behind: Safari mispositions a mirrored row inside a
    // foreignObject and fails to repaint one when the viewBox moves, so the
    // cards left the SVG for an HTML layer over it. Asked for by namespace,
    // since jsdom's own querySelector lowercases a type selector and would
    // answer "none" whether there were any or not.
    const { container } = renderCanvas();
    expect(container.getElementsByTagNameNS("http://www.w3.org/2000/svg", "foreignObject")).toHaveLength(0);
    expect(container.querySelector(".map-svg .map-card")).toBeNull();
  });

  it("moves both layers by one world transform, the wires and the cards saying the same three numbers", () => {
    // x and y are the world point at the frame's top left, so each layer
    // moves that point to the origin (in screen pixels, hence the k) and
    // then scales, which is what makes a card's own left and top plain
    // world units.
    //
    // Both layers state it, rather than the wires deriving theirs from the
    // measured frame through a viewBox. A viewBox maps a world rectangle
    // onto the frame under xMidYMid meet, so between a resize and the
    // measure that follows it the wires letterbox off the cards they run
    // to. Read off one expression they cannot disagree, whatever the frame
    // is doing.
    const { container } = renderCanvas({ viewport: { x: 10, y: 20, k: 2 } });
    const layer = container.querySelector(".map-card-layer") as HTMLElement;
    expect(layer.style.transform).toBe("translate(-20px, -40px) scale(2)");
    expect(layer.style.transformOrigin).toBe("0 0");
    expect(container.querySelector(".map-wires")).toHaveAttribute("transform", "translate(-20 -40) scale(2)");
    // And no viewBox is left to disagree with either of them.
    expect(container.querySelector(".map-svg")).not.toHaveAttribute("viewBox");
  });

  it("places every card at its own box, in world units, inside that layer", () => {
    const { container, props } = renderCanvas();
    const layer = container.querySelector(".map-card-layer");
    const box = props.layout.boxes.get("F-1");
    const card = container.querySelector('.map-card[data-id="F-1"]') as HTMLElement;
    expect(card.parentElement).toBe(layer);
    expect(card.style.left).toBe(`${box?.x}px`);
    expect(card.style.top).toBe(`${box?.y}px`);
    expect(card.style.width).toBe(`${box?.w}px`);
    expect(card.style.height).toBe(`${box?.h}px`);
  });

  it("keeps a left-side card's chevron inside the card it belongs to", () => {
    // The displacement WebKit drew, said at the DOM level: on the left half
    // of the map the chevron sits on the root-facing edge, and it is a child
    // of its own card while it does so.
    const { container } = renderCanvas();
    const shelf = container.querySelector(`.map-card[data-id="${SHELF_BUG}"]`) as HTMLElement;
    expect(shelf.getAttribute("data-side")).toBe("left");
    const chevron = screen.getByRole("button", { name: "Collapse Bugs" });
    expect(chevron).toHaveClass("map-chevron");
    expect(chevron.parentElement).toBe(shelf);
  });

  it("gives an epic a kind pill, a title, and a count; a feature a state dot, a title, and its hold badge; a leaf a dot and a title", () => {
    const { container } = renderCanvas();
    const e1 = container.querySelector('.map-card[data-id="E-1"]') as HTMLElement;
    expect(within(e1).getByText("Epic")).toHaveClass("kind-pill");
    expect(within(e1).getByText("2")).toHaveClass("map-count");
    const f2 = container.querySelector('.map-card[data-id="F-2"]') as HTMLElement;
    expect(f2.querySelector(".state-dot")).not.toBeNull();
    expect(within(f2).getByText("CI")).toBeInTheDocument();
    expect((f2.querySelector(".map-holds") as HTMLElement).textContent).toBe("2");
    expect(f2.querySelector(".chip")).toBeNull();
    const b1 = container.querySelector('.map-card[data-id="B-1"]') as HTMLElement;
    expect(b1.querySelector(".state-dot")).not.toBeNull();
    expect(b1.querySelector(".kind-pill")).toBeNull();
  });

  it("names a card in words for assistive tech", () => {
    renderCanvas();
    expect(screen.getByRole("button", { name: "Feature CI, draft, 2 questions holding it" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Epic Foundation, ready, 2 features" })).toBeInTheDocument();
    // A shelf is not a node and has no status to say: its name and what is on
    // it is the whole sentence.
    expect(screen.getByRole("button", { name: "Bugs, 1 bug" })).toBeInTheDocument();
  });

  it("says a shelf's count in its own plural, chores included", () => {
    const withChores: MapView = {
      ...view,
      nodes: [
        ...view.nodes,
        { noun: "chore", id: "CH-1", key: "chores/deps", title: "Bump the dependencies", status: "proposed" },
        { noun: "chore", id: "CH-2", key: "chores/lint", title: "Turn lint back on", status: "proposed" },
        { noun: "tweak", id: "TW-abc123", key: "tweaks/button", title: "Button spacing", status: "ready" },
      ],
    };
    const open = new Set([SHELF_BUG, SHELF_CHORE, "shelf:tweak"]);
    renderCanvas({ view: withChores, layout: layoutMap(withChores, open), open });
    expect(screen.getByRole("button", { name: "Chores, 2 chores" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bugs, 1 bug" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tweaks, 1 tweak" })).toBeInTheDocument();
  });

  it("draws every branch connector at rest and no dependency arc", () => {
    const { container } = renderCanvas();
    expect(container.querySelectorAll("path.map-branch")).toHaveLength(5); // root>E-1, root>shelf, E-1>F-1, E-1>F-2, shelf>B-1
    expect(container.querySelectorAll("path.map-edge")).toHaveLength(0);
  });

  it("lays a paper halo under every arc it draws, just before the arc", () => {
    const open = new Set(["E-1", "E-2", SHELF_BUG]);
    const { container } = renderCanvas({ open, layout: layoutMap(view, open), showAll: true });
    const arcs = [...container.querySelectorAll("path.map-edge")];
    expect(arcs.length).toBeGreaterThan(1);
    for (const arc of arcs) {
      const halo = arc.previousElementSibling;
      expect(halo).toHaveClass("map-edge-halo");
      expect(halo?.getAttribute("d")).toBe(arc.getAttribute("d"));
      expect(halo?.getAttribute("marker-end")).toBeNull();
    }
    expect(container.querySelectorAll("path.map-edge-halo")).toHaveLength(arcs.length);
  });

  it("draws only the selected card's own arcs, lights its neighbours, and leaves the rest of the chain undimmed", () => {
    // F-1 rather than F-2: the bug in this fixture is attached to F-2, and a
    // selected node's own shelf edges light too, so F-2 has nothing left to
    // dim. F-1 is the node with something off its chain. F-3 depends on F-2,
    // so it is on F-1's chain but not next to it: no arc, no ring, no dimming.
    const withChain: MapView = {
      ...view,
      nodes: [
        ...view.nodes,
        { noun: "epic", id: "E-2", key: "audit", title: "Audit", status: "draft" },
        { noun: "feature", id: "F-3", key: "audit/trail", title: "Trail", status: "draft", parent: "E-2" },
      ],
      edges: [...view.edges, { from: "F-3", to: "F-2", kind: "depends_on" }],
    };
    const open = new Set(["E-1", "E-2", SHELF_BUG]);
    const { container } = renderCanvas({
      view: withChain,
      open,
      layout: layoutMap(withChain, open),
      selected: "F-1",
      chain: chainOf(withChain, open, "F-1"),
    });
    const arcs = container.querySelectorAll("path.map-edge");
    expect(arcs).toHaveLength(1);
    expect(arcs[0]).toHaveClass("lit");
    expect(arcs[0].getAttribute("data-kind")).toBe("depends_on");
    expect(arcs[0].getAttribute("marker-end")).toBe("url(#map-arrow)");
    expect(container.querySelector('.map-card[data-id="F-1"]')).toHaveClass("selected");
    expect(container.querySelector('.map-card[data-id="F-2"]')).toHaveClass("lit");
    expect(container.querySelector('.map-card[data-id="F-3"]')).not.toHaveClass("lit");
    expect(container.querySelector('.map-card[data-id="F-3"]')).not.toHaveClass("dim");
    expect(container.querySelector('.map-card[data-id="E-2"]')).not.toHaveClass("dim");
    expect(container.querySelector('.map-card[data-id="B-1"]')).toHaveClass("dim");
    expect(container.querySelector('.map-card[data-id="E-1"]')).not.toHaveClass("dim");
    expect(container.querySelector(`.map-card[data-id="${ROOT_ID}"]`)).not.toHaveClass("dim");
  });

  it("lights a selected bug's own edges and the nodes at the far end of them", () => {
    // A bug or a chore has no depends_on at all, so a selection on one used
    // to light nothing: no arc to what it was found in, no arc to what
    // carries it, and both of those cards dimmed along with everything else.
    const withCarrier: MapView = {
      ...view,
      edges: [...view.edges, { from: "B-1", to: "F-1", kind: "resolved_by" }],
    };
    const open = new Set(["E-1", SHELF_BUG]);
    const { container } = renderCanvas({
      view: withCarrier,
      layout: layoutMap(withCarrier, open),
      open,
      selected: "B-1",
      chain: chainOf(withCarrier, open, "B-1"),
    });
    const arcs = [...container.querySelectorAll("path.map-edge")];
    expect(arcs).toHaveLength(2);
    expect(arcs.every((a) => a.classList.contains("lit"))).toBe(true);
    expect(arcs.map((a) => a.getAttribute("data-kind")).sort()).toEqual(["attached_to", "resolved_by"]);
    // Found-in is a fact, not a direction the work flows, so it carries no
    // arrowhead; carried-by does.
    expect(container.querySelector('path.map-edge[data-kind="attached_to"]')?.getAttribute("marker-end")).toBeNull();
    expect(container.querySelector('path.map-edge[data-kind="resolved_by"]')?.getAttribute("marker-end")).toBe("url(#map-arrow)");
    expect(container.querySelector('.map-card[data-id="B-1"]')).toHaveClass("selected");
    expect(container.querySelector('.map-card[data-id="F-1"]')).not.toHaveClass("dim");
    expect(container.querySelector('.map-card[data-id="F-2"]')).not.toHaveClass("dim");
  });

  it("draws every arc when showAll is on, the attached_to one dotted without an arrowhead", () => {
    const { container } = renderCanvas({ showAll: true });
    const arcs = container.querySelectorAll("path.map-edge");
    expect(arcs).toHaveLength(2);
    const attached = container.querySelector('path.map-edge[data-kind="attached_to"]') as SVGPathElement;
    expect(attached.getAttribute("marker-end")).toBeNull();
    expect(container.querySelector('path.map-edge[data-kind="depends_on"]')?.getAttribute("marker-end")).toBe("url(#map-arrow)");
  });

  it("routes a hidden selection's own arcs through its visible ancestor without borrowing that ancestor's edges", () => {
    const graph: MapView = {
      ...view,
      edges: [...view.edges, { from: "B-1", to: "T-1", kind: "attached_to" }],
    };
    const open = new Set(["E-1", SHELF_BUG]);
    const { container } = renderCanvas({ view: graph, open, selected: "T-1", chain: chainOf(graph, open, "T-1") });
    const arcs = container.querySelectorAll("path.map-edge");
    expect(arcs).toHaveLength(1);
    expect(arcs[0]).toHaveClass("lit");
    expect(arcs[0].getAttribute("data-kind")).toBe("attached_to");
  });

  it("paints the chain's arcs over the rest when every arc is on", () => {
    // With Work links on and something selected, both kinds of arc are
    // on screen at once. The wire's order put the lit one first, so an unlit
    // arc crossing it drew straight over the answer the reader asked for.
    // F-1 is the selection with an arc off its chain: the bug's is attached
    // to F-2.
    const open = new Set(["E-1", SHELF_BUG]);
    const { container } = renderCanvas({ open, showAll: true, selected: "F-1", chain: chainOf(view, open, "F-1") });
    const arcs = [...container.querySelectorAll("path.map-edge")];
    expect(arcs).toHaveLength(2);
    expect(arcs[arcs.length - 1]).toHaveClass("lit");
    expect(arcs[0]).not.toHaveClass("lit");
  });

  it("selects on a card click, toggles from the chevron without selecting, and deselects on a canvas click", () => {
    const { container, props } = renderCanvas();
    fireEvent.click(screen.getByRole("button", { name: /^Feature Skeleton/ }));
    expect(props.onSelect).toHaveBeenCalledWith("F-1");
    fireEvent.click(screen.getByRole("button", { name: "Collapse Foundation" }));
    expect(props.onToggle).toHaveBeenCalledWith("E-1");
    fireEvent.click(container.querySelector(".map-canvas") as Element);
    expect(props.onSelect).toHaveBeenCalledWith(null);
  });

  it("deselects when the selected card is clicked again", () => {
    // The click that lit a node is the click that puts it out: a second
    // press on the same card reads as "never mind", and reaching for empty
    // canvas or Escape to get there was the extra step JC noticed.
    const { props } = renderCanvas({ selected: "F-1" });
    fireEvent.click(screen.getByRole("button", { name: /^Feature Skeleton/ }));
    expect(props.onSelect).toHaveBeenCalledWith(null);
    expect(props.onSelect).not.toHaveBeenCalledWith("F-1");
    // A different card still selects it.
    fireEvent.click(screen.getByRole("button", { name: /^Feature CI/ }));
    expect(props.onSelect).toHaveBeenCalledWith("F-2");
  });

  it("names the frame itself as the map, so the landmark covers the wires and the cards alike", () => {
    renderCanvas();
    const frame = screen.getByRole("application", { name: "Project map" });
    expect(frame).toHaveClass("map-canvas");
    expect(frame.querySelector(".map-svg")).not.toBeNull();
  });

  it("does not read a card click as a click past the map", () => {
    // The frame is an ancestor of every card now, so a card click reaches
    // the frame's own handler on the way up. Deselecting there would undo
    // the selection the click had just made.
    const { props } = renderCanvas();
    fireEvent.click(screen.getByRole("button", { name: /^Feature Skeleton/ }));
    expect(props.onSelect).toHaveBeenCalledWith("F-1");
    expect(props.onSelect).not.toHaveBeenCalledWith(null);
  });

  it("does not pan on a press that lands on a card", () => {
    // The same ancestry from the other end: a press on a card must not arm
    // the frame's drag, or dragging a card would slide the whole map out
    // from under it.
    const { container, props } = renderCanvas();
    const card = container.querySelector('.map-card[data-id="E-1"]') as Element;
    fireEvent.pointerDown(card, { button: 0, clientX: 100, clientY: 100 });
    fireEvent.pointerMove(card, { clientX: 160, clientY: 140 });
    expect(props.onViewport).not.toHaveBeenCalled();
  });

  it("reads a click on the root as a click past the map", () => {
    // The project is not a node, so its card selects nothing. It deselects,
    // which is what the rest of the canvas does: "not called with the root"
    // would have passed just as well if the click had done nothing at all.
    const { container, props } = renderCanvas();
    fireEvent.click(container.querySelector(`.map-card[data-id="${ROOT_ID}"]`) as Element);
    expect(props.onSelect).toHaveBeenCalledWith(null);
    expect(props.onSelect).not.toHaveBeenCalledWith(ROOT_ID);
  });

  it("draws the cards in wire order, each shelf card ahead of the first node on it", () => {
    // Tab walks the cards in the order the tree lists them, which is why they
    // are rendered off view.nodes rather than off the layout's placement. The
    // shelf branches are the one thing the wire never names, so each is
    // emitted just before the first node it carries.
    const { container } = renderCanvas();
    const ids = [...container.querySelectorAll(".map-card")].map((el) => el.getAttribute("data-id"));
    expect(ids).toEqual([ROOT_ID, "E-1", "F-1", "F-2", SHELF_BUG, "B-1"]);
  });

  it("disarms the drag when the pointer is cancelled", () => {
    // A pointercancel (the browser taking the pointer for a scroll or a
    // gesture) never fires pointerup, so the drag stayed armed and the next
    // move over the canvas panned with no button held.
    const { container, props } = renderCanvas();
    const bg = container.querySelector(".map-canvas") as Element;
    fireEvent.pointerDown(bg, { button: 0, clientX: 100, clientY: 100 });
    fireEvent.pointerCancel(bg);
    fireEvent.pointerMove(bg, { clientX: 160, clientY: 140 });
    expect(props.onViewport).not.toHaveBeenCalled();
  });

  it("measures its own frame and reports it upward", () => {
    // The frame is no longer a term in what the canvas draws: both layers
    // read the viewport alone. It is still measured, because a fit and the
    // travel to a selection are computed against it, and MapMode is where
    // both of those live.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 600,
      height: 400,
      top: 0,
      left: 0,
      right: 600,
      bottom: 400,
      toJSON: () => ({}),
    } as DOMRect);
    const onSize = vi.fn();
    renderCanvas({ onSize });
    expect(onSize).toHaveBeenCalledWith({ w: 600, h: 400 });
  });

  it("pans on a canvas drag and keeps the selection when the drag ends", () => {
    const { container, props } = renderCanvas({ selected: "F-1" });
    const bg = container.querySelector(".map-canvas") as Element;
    fireEvent.pointerDown(bg, { button: 0, clientX: 100, clientY: 100 });
    fireEvent.pointerMove(bg, { clientX: 160, clientY: 140 });
    fireEvent.pointerUp(bg);
    // The browser still fires a click to close the press, and that click
    // must not read as "the reader clicked empty canvas".
    fireEvent.click(bg);
    expect(props.onViewport).toHaveBeenCalledWith({ x: -60, y: -40, k: 1 });
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it("reports a card that takes focus, so the mode can pan it into view", () => {
    // Tab walks every card in wire order and most of them are off the frame
    // on any real project, so the mode is told which card focus landed on.
    // The canvas itself knows nothing about the frame: what to do about a
    // card the reader cannot see is MapMode's call.
    const onFocusCard = vi.fn();
    renderCanvas({ onFocusCard });
    fireEvent.focus(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
    expect(onFocusCard).toHaveBeenCalledWith("F-1");
  });

  it("says nothing about a focus a press put there, and reports again once the press is over", () => {
    // Chromium focuses the button a mousedown lands on. Reported, that focus
    // panned the map before the mouseup, so the release landed on shifted
    // content and the chevron click was lost outright (dual-engine smoke,
    // review: WebKit does not focus on click, so it passed there).
    // The guard is the pointer's own, since jsdom cannot match
    // :focus-visible, which is the browser's answer to the same question.
    const onFocusCard = vi.fn();
    const { container } = renderCanvas({ onFocusCard });
    const card = container.querySelector('.map-card[data-id="F-1"]') as Element;
    fireEvent.pointerDown(card, { button: 0, clientX: 100, clientY: 100 });
    fireEvent.focus(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
    expect(onFocusCard).not.toHaveBeenCalled();
    // And the keyboard is not disarmed for the rest of the visit: the press
    // that armed the guard is what clears it.
    fireEvent.pointerUp(card);
    fireEvent.focus(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
    expect(onFocusCard).toHaveBeenCalledWith("F-1");
  });

  it("registers its wheel listener once per mount and zooms off the newest viewport", () => {
    // The listener has to be native and non-passive (React's own onWheel is
    // registered passive at the root, where preventDefault is ignored), and
    // it used to be re-registered on every viewport the wheel itself
    // reported: a removeEventListener and an addEventListener per tick of a
    // zoom. What the handler must not lose in exchange for being registered
    // once is the newest viewport, so both halves are asked for here.
    const addSpy = vi.spyOn(HTMLDivElement.prototype, "addEventListener");
    const onViewport = vi.fn();
    const open = new Set(["E-1", SHELF_BUG]);
    const props = {
      view,
      layout: layoutMap(view, open),
      open,
      selected: null,
      chain: null,
      overlay: relationshipOverlay(view, layoutMap(view, open), null, false, ""),
      rootTitle: "Acme",
      onSelect: vi.fn(),
      onToggle: vi.fn(),
      onViewport,
    };
    const { container, rerender } = render(
      <MemoryRouter>
        <MapCanvas {...props} viewport={{ x: 0, y: 0, k: 1 }} />
      </MemoryRouter>,
    );
    // React registers a wheel listener of its own on the root container, so
    // this counts the change across a re-render rather than the total.
    const wheels = () => addSpy.mock.calls.filter((c) => c[0] === "wheel").length;
    const mounted = wheels();
    rerender(
      <MemoryRouter>
        <MapCanvas {...props} viewport={{ x: 0, y: 0, k: 2 }} />
      </MemoryRouter>,
    );
    expect(wheels()).toBe(mounted);
    fireEvent.wheel(container.querySelector(".map-canvas") as Element, { deltaY: -100, clientX: 200, clientY: 100 });
    expect(((onViewport as Mock).mock.calls[0][0] as Viewport).k).toBeGreaterThan(2);
  });

  it("zooms about the pointer on a wheel", () => {
    const { container, props } = renderCanvas();
    fireEvent.wheel(container.querySelector(".map-canvas") as Element, { deltaY: -100, clientX: 200, clientY: 100 });
    const next = (props.onViewport as Mock).mock.calls[0][0] as Viewport;
    expect(next.k).toBeGreaterThan(1);
    // The map point under the cursor is the same point before and after.
    expect(next.x + 200 / next.k).toBeCloseTo(200);
    expect(next.y + 100 / next.k).toBeCloseTo(100);
  });
});

describe("MapCard", () => {
  it("shows the detail, links dependencies to selection, and drills in with Open", () => {
    const onSelect = vi.fn();
    render(
      <MemoryRouter>
        <MapCard node={view.nodes[3]} view={view} onSelect={onSelect} onClose={() => undefined} />
      </MemoryRouter>,
    );
    expect(screen.getByText("Feature")).toHaveClass("kind-pill");
    expect(screen.getByRole("heading", { name: "CI" })).toBeInTheDocument();
    expect(screen.getByText("foundation/ci")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Inspect on map: Skeleton" }));
    expect(onSelect).toHaveBeenCalledWith("F-1");
    expect(screen.getAllByRole("link", { name: "Open" }).some((link) => link.getAttribute("href") === "/node/feature/F-2")).toBe(true);
    expect(screen.getByText("2 questions holding it")).toBeInTheDocument();
  });


  it("lists a shelf node's found-in target", () => {
    render(
      <MemoryRouter>
        <MapCard node={view.nodes[4]} view={view} onSelect={() => undefined} onClose={() => undefined} />
      </MemoryRouter>,
    );
    expect(screen.getByText("Found in")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Inspect on map: CI" })).toBeInTheDocument();
  });

  it("separates the enclosing plan from prerequisites and opens its granted destination", () => {
    const onSelect = vi.fn();
    render(<MemoryRouter><MapCard node={view.nodes[3]} view={view} onSelect={onSelect} onClose={() => undefined} /></MemoryRouter>);
    const parent = screen.getByRole("region", { name: "Parent" });
    fireEvent.click(within(parent).getByRole("button", { name: "Inspect on map: Foundation" }));
    expect(onSelect).toHaveBeenCalledWith("E-1");
    expect(within(parent).getByRole("link", { name: "Open" })).toHaveAttribute("href", "/node/epic/E-1");
    expect(within(screen.getByRole("region", { name: "Depends on" })).queryByText("Foundation")).not.toBeInTheDocument();
  });

  it("does not invent an enclosing plan for a root or shelf item", () => {
    for (const node of [view.nodes[0], view.nodes[4]]) {
      const { unmount } = render(<MemoryRouter><MapCard node={node} view={view} onSelect={() => undefined} onClose={() => undefined} /></MemoryRouter>);
      expect(screen.queryByRole("region", { name: "Parent" })).not.toBeInTheDocument();
      unmount();
    }
  });


  // The geometry the drag tests work in: a 1200x800 frame with the card's
  // home in the top right corner. jsdom lays nothing out, so both rects are
  // stated here and told apart by the element they are asked of, and the
  // card's rect follows the transform it is wearing, the way a laid-out one
  // would. Both are returned live, so a test can move the frame's edges (and
  // the home that is anchored to them) the way a window resize would.
  function layOut() {
    const frame = { x: 0, y: 0, left: 0, top: 0, right: 1200, bottom: 800, width: 1200, height: 800 };
    const card = { x: 864, y: 48, left: 864, top: 48, right: 1184, bottom: 448, width: 320, height: 400 };
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      if (!this.classList.contains("map-dock")) {
        return { ...frame, toJSON: () => undefined } as DOMRect;
      }
      const worn = /translate\((-?[\d.]+)px, (-?[\d.]+)px\)/.exec(this.style.transform);
      const dx = worn === null ? 0 : Number(worn[1]);
      const dy = worn === null ? 0 : Number(worn[2]);
      return {
        ...card,
        x: card.x + dx,
        left: card.left + dx,
        right: card.right + dx,
        y: card.y + dy,
        top: card.top + dy,
        bottom: card.bottom + dy,
        toJSON: () => undefined,
      } as DOMRect;
    });
    return { frame, card };
  }

  function renderDock(node = view.nodes[3], onClose: () => void = () => undefined) {
    const result = render(
      <MemoryRouter>
        <MapCard node={node} view={view} onSelect={() => undefined} onClose={onClose} />
      </MemoryRouter>,
    );
    return {
      ...result,
      dock: result.container.querySelector(".map-dock") as HTMLElement,
      head: result.container.querySelector(".map-dock-head") as HTMLElement,
    };
  }

  // Every move below says the primary button is still held (buttons: 1),
  // since a move that reports no button is how the card learns of a release
  // it never saw (the test on that is further down).
  it("drags by its header, following the pointer until it is let go", () => {
    layOut();
    const { dock, head } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 700, clientY: 160 });
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
    fireEvent.pointerUp(head);
    fireEvent.pointerMove(head, { buttons: 1, clientX: 100, clientY: 100 });
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
  });

  it("keeps the whole card inside the frame, and never above its home row", () => {
    // Up is the one way the card does not go: its home row is the first one
    // under the toolbar, and a card over the toolbar would put Expand all,
    // Collapse all, Work links, and Fit out of reach until it moved.
    layOut();
    const { dock, head } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: -500, clientY: -500 });
    expect(dock.style.transform).toBe("translate(-864px, 0px)");
    fireEvent.pointerMove(head, { buttons: 1, clientX: 5000, clientY: 5000 });
    expect(dock.style.transform).toBe("translate(16px, 352px)");
  });

  it("pulls itself back inside a frame that has shrunk under it", () => {
    // The card is anchored to the frame's right edge, so a narrower window
    // carries a card dragged to the left edge out past it, and .map-mode
    // clips what it carries: without this the header would be unreachable
    // and Escape the only way back.
    const { frame, card } = layOut();
    const { dock, head } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: -500, clientY: 60 });
    fireEvent.pointerUp(head);
    expect(dock.style.transform).toBe("translate(-864px, 0px)");
    // The window narrows by 400: the frame's right edge and the card's home
    // both move 400 left, so the card, still wearing -864, now starts at -400.
    frame.right = 800;
    frame.width = 800;
    card.left = 464;
    card.x = 464;
    card.right = 784;
    fireEvent(window, new Event("resize"));
    expect(dock.style.transform).toBe("translate(-464px, 0px)");
  });

  it("ends a drag whose release it never saw, on the first move with no button held", () => {
    // A context menu opened mid-drag, or a release over another window,
    // swallows the pointerup; the next move arrives with no button down and
    // is the release's stand-in (the same wedge Explore's splitter fixes).
    layOut();
    const { dock, head } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 700, clientY: 160 });
    fireEvent.pointerMove(head, { buttons: 0, clientX: 600, clientY: 200 });
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
    fireEvent.pointerMove(head, { buttons: 1, clientX: 500, clientY: 300 });
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
  });

  it("cancels the press that starts a drag, so the drag sweeps no text selection through the body", () => {
    // The header itself declines selection in CSS, but WebKit anchors a
    // selection at the nearest selectable text and extends it as the
    // captured pointer moves, so a drag down the card highlighted the title,
    // the key, and the first rows under it (seen in Playwright WebKit on
    // Acme). Cancelling the pointerdown is what stops the selection from
    // ever starting; a press on the close control is left alone.
    layOut();
    const { head } = renderDock();
    expect(fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 })).toBe(false);
    fireEvent.pointerUp(head);
    const close = screen.getByRole("button", { name: "Close" });
    expect(fireEvent.pointerDown(close, { button: 0, clientX: 1170, clientY: 60 })).toBe(true);
  });

  it("ignores a second press while a drag is in flight", () => {
    layOut();
    const { dock, head } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerDown(head, { button: 0, clientX: 1000, clientY: 80 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 700, clientY: 160 });
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
  });

  it("does not drag on a press that lands on the close control", () => {
    layOut();
    const onClose = vi.fn();
    const { dock, head } = renderDock(view.nodes[3], onClose);
    const close = screen.getByRole("button", { name: "Close" });
    fireEvent.pointerDown(close, { button: 0, clientX: 1170, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 900, clientY: 200 });
    expect(dock.style.transform).toBe("");
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps where it was dragged to when the selection moves to another node", () => {
    // A reader who moved the card out of the way did it for a reason; a
    // click on one of its own links should not snap it back home.
    layOut();
    const { dock, head, rerender } = renderDock();
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 700, clientY: 160 });
    fireEvent.pointerUp(head);
    rerender(
      <MemoryRouter>
        <MapCard node={view.nodes[1]} view={view} onSelect={() => undefined} onClose={() => undefined} />
      </MemoryRouter>,
    );
    expect(screen.getByRole("heading", { name: "Skeleton" })).toBeInTheDocument();
    expect(dock.style.transform).toBe("translate(-200px, 100px)");
  });
});
