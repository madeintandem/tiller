// The map mode through the real App: the fourth rail entry, the fetch, the
// default open set, selection from the URL and back into it, the docked
// card's Open link, the toolbar, and Escape.
//
// Every card count here includes the root, the project's own card at the
// center of the mind map, which is drawn whatever the payload holds, and the
// Bugs shelf card, the head of the branch the one bug in the fixture hangs
// on. The default picture is three cards: the root, the closed epic, and the
// closed Bugs branch, each wearing its count. Expand all is
// what reaches six: the two features and the bug as well.
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../../App";
import type { MapView } from "../../api";
import { ROOT_ID, layoutMap } from "../../views/explore/map/layout";
import { DOCK_INSET } from "../../views/explore/map/MapCard";

const mapView: MapView = {
  nodes: [
    { noun: "epic", id: "E-1", key: "foundation", title: "Foundation", status: "ready" },
    { noun: "feature", id: "F-1", key: "foundation/skeleton", title: "Skeleton", status: "ready", parent: "E-1", contributors: [{ id: "past", name: "Past contributor" }, { id: "sam", name: "Sam" }] },
    { noun: "feature", id: "F-2", key: "foundation/ci", title: "CI", status: "draft", parent: "E-1" },
    { noun: "task", id: "T-1", key: "foundation/skeleton/init", title: "Init", status: "done", parent: "F-1" },
    // A bug hangs off no node the wire names: the layout gives it the Bugs
    // shelf as its parent, which is what the ancestry walks here have to
    // find on their own.
    { noun: "bug", id: "B-1", key: "crash-on-save", title: "Crash on save", status: "open", owner: "bug-owner" },
  ],
  edges: [{ from: "F-2", to: "F-1", kind: "depends_on" }],
  relationships: {
    vertices: [
      { kind: "epic", id: "E-1", title: "Foundation", status: "ready", destination: { kind: "node", noun: "epic", id: "E-1" } },
      { kind: "feature", id: "F-1", title: "Skeleton", status: "ready", destination: { kind: "node", noun: "feature", id: "F-1" } },
      { kind: "feature", id: "F-2", title: "CI", status: "draft", destination: { kind: "node", noun: "feature", id: "F-2" } },
      { kind: "task", id: "T-1", title: "Init", status: "done", destination: { kind: "node", noun: "task", id: "T-1" } },
      { kind: "bug", id: "B-1", title: "Crash on save", status: "open", destination: { kind: "node", noun: "bug", id: "B-1" } },
    ],
    edges: [{ id: "depends:F-2:F-1", kind: "depends_on", from: "F-2", to: "F-1", sources: [{ file: ".tiller/epics/foundation/ci/feature.yaml", field: "depends_on" }] }],
    catalog: [{ name: "depends_on", inverse: "depended_on_by", from_kinds: ["feature"], to_kinds: ["feature"] }],
    diagnostics: [],
    complete: true,
  },
};

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = typeof input === "string" ? input : input.toString();
      const body = routes[path];
      if (body === undefined) {
        return new Response("not found", { status: 404 });
      }
      return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
    }),
  );
}

// The same stub, but the named paths resolve on a real setTimeout instead of
// an already-resolved promise. Every route in stubFetch above lands in the
// same microtask drain, so /api/status and /api/map always settle together
// there - the one arrangement that hides an arrival-order bug. Delaying
// /api/status for real is what puts the map payload in an earlier commit
// than the project key, which is the order this mode has to survive.
function stubFetchWithDelay(routes: Record<string, unknown>, delayedPaths: string[], delayMs = 20) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = typeof input === "string" ? input : input.toString();
      const body = routes[path];
      if (body === undefined) {
        return new Response("not found", { status: 404 });
      }
      const response = () =>
        new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
      if (!delayedPaths.includes(path)) {
        return response();
      }
      return new Promise<Response>((resolve) => {
        setTimeout(() => resolve(response()), delayMs);
      });
    }),
  );
}

const summary = {
  project: { name: "Acme", key: "acme", kind: "greenfield" },
  contributors: [{ id: "jc", name: "JC" }],
  counts: { epics: 1, features: 2, tasks: 1, questions: 0 },
  whose_move: { human: 0, agent: 0 },
};

function renderMap(route = "/explore?mode=map", extra?: ReactNode) {
  stubFetch({
    "/api/status": summary,
    "/api/tree": { project: "acme", nodes: [] },
    "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
    "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] },
    "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] },
    "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] },
    "/api/map": mapView,
  });
  return render(
    <MemoryRouter initialEntries={[route]}>
      <App />
      {extra}
    </MemoryRouter>,
  );
}

// MapMode's own VIEWPORT_SETTLE_MS: how long a viewport holds still before it
// reaches storage.
const VIEWPORT_DEBOUNCE = 250;

// MapMode's own fit: the toolbar's band, the inset at the other three edges,
// and the zoom floor the wheel shares. The expected numbers below are derived
// from the layout the map is actually showing rather than pinned by hand, so
// a change to a card's size or a gap is not a test to edit.
const FIT_TOP = 48;
const FIT_INSET = 16;
const MIN_ZOOM = 0.1;

// What Expand all reaches on this fixture: the one epic that has features,
// plus the Bugs branch carrying the one bug.
const EXPANDED = new Set(["E-1", "shelf:bug"]);

// The viewport the canvas is actually drawing at, read back off the wire
// group's own transform. Both of its layers state world-to-screen as
// `translate(-x*k -y*k) scale(k)` (the SVG group in user units, the card
// layer in px), so inverting one of them is asking the drawing what viewport
// it is showing. The canvas used to say it as a viewBox, and these tests used
// to read that; the map's geometry is the same either way, but a viewBox also
// encoded the frame's measured size and these assertions borrowed it. Where
// one of them wants the world width of the frame, it says frame.w / k, which
// is what the viewBox's own third number was.
function drawnViewport(container: HTMLElement) {
  const t = container.querySelector(".map-wires")?.getAttribute("transform") ?? "";
  const m = /^translate\((\S+) (\S+)\) scale\((\S+)\)$/.exec(t);
  if (m === null) {
    throw new Error(`the wires carry no world transform: ${JSON.stringify(t)}`);
  }
  const k = Number(m[3]);
  return { x: Number(m[1]) / -k, y: Number(m[2]) / -k, k };
}

// The horizontal half of that fit runs in the width the floating card leaves,
// which is the whole frame with nothing selected and frame.w - DOCK_INSET
// with a card up. Every frame a selection is fitted into below is wide enough
// that the inset clears MapMode's own floor, the half of the frame it never
// gives away; the one test that goes under that floor states the property it
// wants rather than borrowing this.
function fitOf(open: Set<string>, frame: { w: number; h: number }, rightInset = 0) {
  const b = layoutMap(mapView, open).bounds;
  const free = frame.w - rightInset;
  const k = Math.max(MIN_ZOOM, Math.min(1, (free - 2 * FIT_INSET) / b.w, (frame.h - FIT_TOP - FIT_INSET) / b.h));
  return { x: b.x + b.w / 2 - free / (2 * k), y: b.y + b.h / 2 - (frame.h + FIT_TOP) / (2 * k), k };
}

describe("Explore: the map mode", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  // jsdom reports a zero rect for every element, so a test that needs the
  // canvas to have a frame says how big it is.
  function stubFrame(w: number, h: number) {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: w,
      height: h,
      top: 0,
      left: 0,
      right: w,
      bottom: h,
      toJSON: () => ({}),
    } as DOMRect);
  }

  it("is the fourth entry on the mode rail and opens collapsed, every branch closed with its count showing", async () => {
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    // Nothing inside a branch is drawn, not the epic's features and not the
    // bug on the shelf: the collapsed picture is the overview.
    expect(container.querySelector('.map-card[data-id="F-1"]')).toBeNull();
    expect(container.querySelector('.map-card[data-id="B-1"]')).toBeNull();
    expect(container.querySelector('.map-card[data-id="T-1"]')).toBeNull();
    // A closed branch has to say how much is inside it, or the reader has
    // nothing to weigh a click against: the epic carries its two features,
    // the Bugs branch its one bug.
    expect([...container.querySelectorAll(".map-count")].map((n) => n.textContent)).toEqual(["2", "1"]);
  });

  it("names the project on the root card", async () => {
    // The name /api/status carries, not the storage key beside it: the root
    // is the one card a reader reads as the project itself.
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelector('.map-card[data-id="root"]')?.textContent).toBe("Acme"));
  });

  it("selects from the URL, docks the card, and drills in through Open", async () => {
    renderMap("/explore?mode=map&sel=F-2");
    await screen.findByRole("heading", { name: "CI" });
    expect(screen.getAllByRole("link", { name: "Open" }).some((link) => link.getAttribute("href") === "/node/feature/F-2")).toBe(true);
  });

  it("shows contributors for an ancestor and only scalar ownership for shelf execution", async () => {
    renderMap("/explore?mode=map&sel=F-1");
    await screen.findByRole("heading", { name: "Skeleton" });
    const ancestor = screen.getByRole("complementary", { name: "Selected node" });
    expect(ancestor).toHaveTextContent("Contributors");
    expect(within(ancestor).getByText("Past contributor")).toBeInTheDocument();
    expect(within(ancestor).getByText("Sam")).toBeInTheDocument();
    expect(within(ancestor).queryByText("Owner")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    fireEvent.click(await screen.findByRole("button", { name: /^Bug\s*Crash on save/ }));
    await screen.findByRole("heading", { name: "Crash on save" });
    const shelf = screen.getByRole("complementary", { name: "Selected node" });
    expect(shelf).toHaveTextContent("Owner");
    expect(within(shelf).getByText("bug-owner")).toBeInTheDocument();
    expect(shelf).not.toHaveTextContent("Contributors");
  });

  it("keeps the selected node's card outside the canvas frame, a sibling of it", async () => {
    // A standing invariant rather than anything the floating card introduced:
    // the card has always hung off .map-mode beside the canvas, and that is
    // what keeps a press, a drag, or a wheel over it from ever reaching the
    // frame that pans. It is worth a pin now that the card's own geometry has
    // moved (2026-08-31), because nothing else in this file would notice it
    // moving inside the canvas.
    //
    // Floating geometry needs a real browser; jsdom carries no layout.
    const { container } = renderMap("/explore?mode=map&sel=F-2");
    await screen.findByRole("heading", { name: "CI" });
    const dock = container.querySelector(".map-dock") as HTMLElement;
    expect(dock).toHaveAttribute("aria-label", "Selected node");
    expect(dock.parentElement).toHaveClass("map-mode");
    expect(container.querySelector(".map-canvas")?.contains(dock)).toBe(false);
  });

  it("writes a click's selection into the URL and clears it on Escape", async () => {
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    // The card's own button, not the chevron beside it: a container card
    // carries both, and the chevron's label ("Expand Skeleton") names the
    // same node, so a bare /Skeleton/ matches two buttons. The card button
    // says itself in words ("Feature Skeleton, ready").
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
    await screen.findByRole("heading", { name: "Skeleton" });
    expect(container.querySelector('.map-card[data-id="F-1"]')).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("heading", { name: "Skeleton" })).toBeNull());
  });

  it("returns a dragged card to the corner when it closes and the next selection opens it", async () => {
    // The card's offset is its own state, and this mode renders the card
    // only while something is selected: a close unmounts it, and the next
    // selection mounts a fresh one at home. That unmount is the whole
    // mechanism, so it is pinned here against a later shape that keeps the
    // card mounted and hidden. The card needs a rect of its own, apart from
    // the frame's, for the drag's clamp to leave it any room to move.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      const r = this.classList.contains("map-dock")
        ? { x: 864, y: 48, left: 864, top: 48, right: 1184, bottom: 448, width: 320, height: 400 }
        : { x: 0, y: 0, left: 0, top: 0, right: 1200, bottom: 800, width: 1200, height: 800 };
      return { ...r, toJSON: () => ({}) } as DOMRect;
    });
    const { container } = renderMap("/explore?mode=map&sel=F-2");
    await screen.findByRole("heading", { name: "CI" });
    const dock = () => container.querySelector(".map-dock") as HTMLElement;
    const head = container.querySelector(".map-dock-head") as HTMLElement;
    fireEvent.pointerDown(head, { button: 0, clientX: 900, clientY: 60 });
    fireEvent.pointerMove(head, { buttons: 1, clientX: 700, clientY: 160 });
    fireEvent.pointerUp(head);
    expect(dock().style.transform).toBe("translate(-200px, 100px)");
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.queryByLabelText("Selected node")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*CI/ }));
    await screen.findByRole("heading", { name: "CI" });
    expect(dock().style.transform).toBe("");
  });

  it("ignores a sel that names no node instead of dimming the whole map", async () => {
    // A stale or mistyped link used to light nothing, dim every card, and
    // dock no detail to say why.
    const { container } = renderMap("/explore?mode=map&sel=nope");
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(container.querySelectorAll(".map-card.dim")).toHaveLength(0);
    expect(screen.queryByLabelText("Selected node")).toBeNull();
  });

  it("ignores a sel naming the root or a shelf, neither of which is a node", async () => {
    // Both ids exist on the map and neither is on the wire: the root is the
    // project itself and a shelf is the layout's invention. A link carrying
    // one selects nothing, dims nothing, and docks nothing, the same as any
    // other sel that resolves to no node.
    for (const id of [ROOT_ID, "shelf:bug"]) {
      const { container, unmount } = renderMap(`/explore?mode=map&sel=${id}`);
      await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
      expect(container.querySelectorAll(".map-card.dim")).toHaveLength(0);
      expect(container.querySelectorAll(".map-card.selected")).toHaveLength(0);
      expect(screen.queryByLabelText("Selected node")).toBeNull();
      unmount();
    }
  });

  it("opens the containers above a deep-linked node so its card is on screen", async () => {
    // T-1 is a task inside F-1, and features start closed: the link used to
    // dim F-1, the container holding the very node it named, and show no
    // card at all.
    const { container } = renderMap("/explore?mode=map&sel=T-1");
    // The dock draws off the payload, so it is on screen before the opening
    // has settled: the card is what this waits for.
    await waitFor(() => expect(container.querySelector('.map-card[data-id="T-1"]')).toHaveClass("selected"));
    // The feature above it is open, which its chevron is what says now that a
    // branch's children stack beside it rather than inside its box.
    expect(screen.getByRole("button", { name: "Collapse Skeleton" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Init" })).toBeInTheDocument();
  });

  it("opens the shelf above a deep-linked bug, which the wire never names as its parent", async () => {
    // A bug's parent is the Bugs shelf, and the shelf is the layout's
    // invention: the wire leaves a bug's parent field empty. The reopen walk
    // read that field, so a link to a bug whose branch a reader had closed
    // dimmed the whole map and drew no card for the node it named.
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: null }));
    const { container } = renderMap("/explore?mode=map&sel=B-1");
    await waitFor(() => expect(container.querySelector('.map-card[data-id="B-1"]')).toHaveClass("selected"));
    expect(screen.getByRole("button", { name: "Collapse Bugs" })).toBeInTheDocument();
  });

  it("lets the reader close a branch above the node the URL selected", async () => {
    // The ancestry walk depended on the open set, so it re-ran on every
    // toggle and re-opened whatever the reader had just closed: with a
    // selection anywhere inside it, a collapse chevron did nothing at all.
    const { container } = renderMap("/explore?mode=map&sel=T-1");
    await waitFor(() => expect(container.querySelector('.map-card[data-id="T-1"]')).toHaveClass("selected"));
    fireEvent.click(screen.getByRole("button", { name: "Collapse Skeleton" }));
    await waitFor(() => expect(container.querySelector('.map-card[data-id="T-1"]')).toBeNull());
    // And it stays closed: the effect must not fire a second time on the
    // open set it just changed.
    expect(screen.getByRole("button", { name: "Expand Skeleton" })).toBeInTheDocument();
    expect(container.querySelector('.map-card[data-id="T-1"]')).toBeNull();
  });

  it("collapses everything, clears the inspector, and fits the full frame", async () => {
    stubFrame(1200, 800);
    const { container } = renderMap("/explore?mode=map&sel=F-2");
    await waitFor(() => expect(container.querySelector('.map-card[data-id="F-2"]')).toHaveClass("selected"));
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").open).toEqual([]);
    expect(screen.queryByRole("heading", { name: "CI" })).not.toBeInTheDocument();
    const overview = fitOf(new Set(), { w: 1200, h: 800 });
    expect(drawnViewport(container).x).toBeCloseTo(overview.x, 4);
    expect(drawnViewport(container).y).toBeCloseTo(overview.y, 4);
    expect(drawnViewport(container).k).toBeCloseTo(overview.k, 6);
  });

  it("opens a deep link's ancestry even when the project key lands after the map", async () => {
    // The open set does not exist until /api/status has settled, and the
    // ancestry effect no longer watches it: whatever it is keyed on has to
    // cover the commit where the set is first materialized.
    stubFetchWithDelay(
      {
        "/api/status": summary,
        "/api/tree": { project: "acme", nodes: [] },
        "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
        "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] },
        "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] },
        "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] },
        "/api/map": mapView,
      },
      ["/api/status"],
    );
    const { container } = render(
      <MemoryRouter initialEntries={["/explore?mode=map&sel=T-1"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(container.querySelector('.map-card[data-id="T-1"]')).toHaveClass("selected"));
  });

  it("writes the clicked card's id into the URL", async () => {
    let search = "";
    function Probe() {
      search = useLocation().search;
      return null;
    }
    const { container } = renderMap("/explore?mode=map", <Probe />);
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
    await screen.findByRole("heading", { name: "Skeleton" });
    expect(new URLSearchParams(search).get("sel")).toBe("F-1");
  });

  it("leaves the URL alone when Escape is pressed with nothing selected", async () => {
    // The handler ran unconditionally, so every Escape anywhere in the
    // console navigated the router to the location it was already on.
    let renders = 0;
    function Probe() {
      useLocation();
      renders += 1;
      return null;
    }
    const { container } = renderMap("/explore?mode=map", <Probe />);
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const before = renders;
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(renders).toBe(before);
  });

  it("leaves the URL alone on an empty-canvas click with nothing selected, and drops sel when the selected card is clicked again", async () => {
    // Clicking empty canvas with nothing selected is most clicks on a map,
    // and it used to replace the location with the location it was already
    // on. Clicking the selected card used to be the same no-op; now it is
    // the way out of a selection (JC, 2026-08-31), so it is one navigation
    // that removes sel.
    let renders = 0;
    function Probe() {
      useLocation();
      renders += 1;
      return null;
    }
    const { container } = renderMap("/explore?mode=map", <Probe />);
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const before = renders;
    fireEvent.click(container.querySelector(".map-canvas") as Element);
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(renders).toBe(before);
    // Now with a selection: the card it named, clicked again.
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*CI/ }));
    await screen.findByRole("heading", { name: "CI" });
    expect(container.querySelector('.map-card[data-id="F-2"]')).toHaveClass("selected");
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*CI/ }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "CI" })).toBeNull());
    expect(container.querySelector('.map-card[data-id="F-2"]')).not.toHaveClass("selected");
    expect(container.querySelector(".map-card.dim")).toBeNull();
    expect(new URLSearchParams(window.location.search).get("sel")).toBeNull();
  });

  it("fits the whole layout into the frame instead of resetting to home", async () => {
    // Fit used to be Home: it went back to 1:1 at the origin, which on
    // Acme's map is a corner of one epic and leaves the layout as
    // unreachable as it was.
    //
    // The clock is faked before the render rather than after it, the way the
    // dock-trip test below does it. A mount schedules a real 250ms persist
    // timer of its own, off the first-visit fit, and a fake clock installed
    // on top of a real timer cannot cancel it: it fires 250ms later, during
    // whichever test is running by then, and writes this project's memory
    // underneath it. That is a stray `{"open":[],"viewport":{"x":0,"y":0,
    // "k":1}}` failing the file's last test in 2 full-suite runs out of 4.
    stubFrame(400, 300);
    vi.useFakeTimers();
    try {
      const { container } = renderMap();
      // Both fetches land and the first visit fits, which is what schedules
      // the debounce this test then waits out.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50);
      });
      expect(container.querySelectorAll(".map-card")).toHaveLength(3);
      fireEvent.click(screen.getByRole("button", { name: "Fit" }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
    } finally {
      vi.useRealTimers();
    }
    const stored = JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").viewport;
    // The whole collapsed layout, scaled to the smaller of the two ratios and
    // centered on both axes in the band below the toolbar.
    const want = fitOf(new Set(), { w: 400, h: 300 });
    expect(stored.k).toBeCloseTo(want.k, 6);
    expect(stored.x).toBeCloseTo(want.x, 6);
    expect(stored.y).toBeCloseTo(want.y, 6);
    // The width binds in a 400 x 300 frame, and the toolbar's 48px band is
    // clear above the drawing's first row.
    const bounds = layoutMap(mapView, new Set()).bounds;
    expect(want.k).toBeCloseTo((400 - 2 * FIT_INSET) / bounds.w, 6);
    expect((bounds.y - stored.y) * stored.k).toBeGreaterThanOrEqual(FIT_TOP);
  });

  it("centers a layout smaller than the frame without magnifying it past 1:1", async () => {
    // The scratch projects are three cards wide. Scaling those up to fill a
    // laptop window would draw a 40px title; the map stops at 1:1 and puts
    // the drawing in the middle of the paper instead.
    stubFrame(4000, 3000);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const drawn = drawnViewport(container);
    const want = fitOf(new Set(), { w: 4000, h: 3000 });
    expect(want.k).toBe(1);
    expect(drawn.k).toBeCloseTo(1, 6);
    expect(drawn.x).toBeCloseTo(want.x, 4);
    expect(drawn.y).toBeCloseTo(want.y, 4);
  });

  it("opens collapsed and fits the map on a first visit", async () => {
    // 1:1 at the origin is a corner of one epic on any real project. A map
    // nobody has stored a viewport for opens whole instead.
    stubFrame(400, 300);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    // The whole collapsed layout in a 400 x 300 frame, inset 16 a side. The
    // frame is 400 / k world px across at this zoom, so the drawn viewport's
    // own x plus half of that is where the middle of the frame lands.
    const drawn = drawnViewport(container);
    const want = fitOf(new Set(), { w: 400, h: 300 });
    expect(drawn.k).toBeCloseTo(want.k, 6);
    // Centered, not cornered: the frame's center is the layout's center.
    const bounds = layoutMap(mapView, new Set()).bounds;
    expect(drawn.x + 400 / drawn.k / 2).toBeCloseTo(bounds.x + bounds.w / 2, 4);
    expect(drawn.y).toBeCloseTo(want.y, 4);
    // The toolbar's band is clear above the map's first row.
    expect((bounds.y - drawn.y) * want.k).toBeGreaterThanOrEqual(FIT_TOP);
    // A fit nobody asked for is not a gesture, so it freezes nothing into
    // memory: the open set beside it is still today's default.
    expect(localStorage.getItem("tiller.map.acme")).toBeNull();
  });

  it("fits into the width the floating card leaves, and into the whole frame when nothing is selected", async () => {
    // The fit knew nothing about the card, so a map fitted with one up ran
    // its right hand side underneath the panel: on a 1200px frame that is
    // 352 of drawing behind it, which on a balanced mind map is most of one
    // side of the tree.
    stubFrame(1200, 800);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const bounds = layoutMap(mapView, new Set()).bounds;
    const middle = bounds.x + bounds.w / 2;
    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    // Nothing selected, nothing covering the map: the drawing's middle is
    // the frame's own middle, the full width of it.
    const whole = drawnViewport(container);
    expect(whole.x + 1200 / whole.k / 2).toBeCloseTo(middle, 4);
    // With a card up it is the middle of what that card leaves. Selecting on
    // the canvas moves nothing by itself, so the Fit beside it is what says
    // this: same layout, same frame, a different center.
    fireEvent.click(screen.getByRole("button", { name: /^Epic\s*Foundation/ }));
    await screen.findByRole("heading", { name: "Foundation" });
    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    const drawn = drawnViewport(container);
    const want = fitOf(new Set(), { w: 1200, h: 800 }, DOCK_INSET);
    expect(drawn.k).toBeCloseTo(want.k, 6);
    expect(drawn.x).toBeCloseTo(want.x, 4);
    expect(drawn.y).toBeCloseTo(want.y, 4);
    expect(drawn.x + (1200 - DOCK_INSET) / drawn.k / 2).toBeCloseTo(middle, 4);
    // And the drawing that fit makes stops short of where the panel starts
    // rather than running underneath it. Both numbers are taken off the whole
    // 1200, since the fit insets the frame the canvas reports rather than
    // expecting a narrower canvas. The floating panel's actual geometry is
    // exercised in the browser rather than inferred from CSS source text.
    expect((bounds.x + bounds.w - drawn.x) * drawn.k).toBeLessThanOrEqual(1200 - DOCK_INSET);
  });

  it("keeps half the window for the map when the frame is too narrow to hold the card beside it", async () => {
    // DOCK_INSET off a 400px frame leaves 48, where a map is a sliver at the
    // zoom floor, and off anything narrower it leaves nothing at all and the
    // fit lands off screen entirely. The inset stops at half the frame, so a
    // window too small for both still opens on a map worth looking at.
    stubFrame(400, 300);
    const { container } = renderMap("/explore?mode=map&sel=F-2");
    await screen.findByRole("heading", { name: "CI" });
    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    const drawn = drawnViewport(container);
    // The deep link opened the epic above F-2, so that is the layout being
    // fitted, and its middle sits at the middle of the frame's left half.
    const bounds = layoutMap(mapView, new Set(["E-1"])).bounds;
    expect(drawn.x + 400 / 4 / drawn.k).toBeCloseTo(bounds.x + bounds.w / 2, 4);
  });

  it("pans to a selection that arrives on the URL, centering the selected card in the width its own panel leaves", async () => {
    stubFrame(1200, 800);
    const { container } = renderMap("/explore?mode=map&sel=F-2");
    await screen.findByRole("heading", { name: "CI" });
    // The card carries its own box: left and width in world units, the same
    // coordinate space the drawn viewport is in, so the two compare directly.
    // The middle of the free width is (1200 - DOCK_INSET) / 2k world units in
    // from the viewport's own x.
    const card = container.querySelector('.map-card[data-id="F-2"]') as HTMLElement;
    const cx = parseFloat(card.style.left) + parseFloat(card.style.width) / 2;
    const drawn = drawnViewport(container);
    // The zoom is the first visit's fit, which the trip rides on rather than
    // replacing, and it was taken with the inset too. The layout it fitted is
    // the one with E-1 open, not the collapsed default: the ancestry effect
    // reads the open set the restore effect queued in the same commit, so the
    // link's branch is already open in the first render that draws a canvas
    // at all, which is the render that first measures a frame and fits.
    expect(drawn.k).toBeCloseTo(fitOf(new Set(["E-1"]), { w: 1200, h: 800 }, DOCK_INSET).k, 6);
    expect(Math.abs(drawn.x + (1200 - DOCK_INSET) / drawn.k / 2 - cx)).toBeLessThan(2);
    // And not the middle of the whole frame, which is half the panel's width
    // further right: that is the trip that used to park the selected card
    // under the very panel describing it.
    expect(drawn.x + 1200 / drawn.k / 2 - cx).toBeCloseTo(DOCK_INSET / (2 * drawn.k), 4);
  });

  it("pans to a node picked out of the docked card, and holds still for one clicked on the canvas", async () => {
    stubFrame(400, 300);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    const world = () => container.querySelector(".map-wires")?.getAttribute("transform");
    const fitted = world();
    // A card clicked on the canvas is already under the reader's eye, so
    // moving the map out from under the click is the one thing they did not
    // ask for.
    fireEvent.click(screen.getByRole("button", { name: /^Feature\s*CI/ }));
    await screen.findByRole("heading", { name: "CI" });
    expect(world()).toBe(fitted);
    // The dock's own links are the other case: the node they name can be
    // anywhere on the map, including off screen.
    fireEvent.click(screen.getByRole("button", { name: "Inspect on map: Skeleton" }));
    await screen.findByRole("heading", { name: "Skeleton" });
    expect(world()).not.toBe(fitted);
  });

  // A chevron that opens a branch fans its children out beside the card, and
  // nothing about the viewport promises they landed anywhere the reader can
  // see. On the right of the map they open straight into the floating card;
  // on the left they can open past the frame's own edge. Four rules: reveal
  // what an opening chevron made, by the smallest pan that does it; hold
  // still when it is already on screen; anchor the near edge when what
  // opened is bigger than the room there is; and never pan on a close.
  it("reveals a branch a chevron just opened from under the floating card", async () => {
    stubFrame(1200, 800);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    // The smoke run's own path: click a card, which docks the panel and holds
    // the map still, then open the branch under it. The map is sitting on its
    // first-visit fit of the collapsed layout, which was taken with nothing
    // selected and so in the whole frame.
    fireEvent.click(screen.getByRole("button", { name: /^Epic\s*Foundation/ }));
    await screen.findByRole("heading", { name: "Foundation" });
    const before = drawnViewport(container);
    expect(before.k).toBeCloseTo(fitOf(new Set(), { w: 1200, h: 800 }).k, 6);
    // What the click is about to put on screen: the epic's own card and the
    // two features that open beside it. Its right edge lands past the free
    // rect, which stops DOCK_INSET short of the frame.
    const opened = layoutMap(mapView, new Set(["E-1"]));
    const shown = ["E-1", "F-1", "F-2"].map((id) => opened.boxes.get(id)!);
    const right = Math.max(...shown.map((b) => b.x + b.w));
    const overflow = (right - before.x) * before.k - (1200 - DOCK_INSET - FIT_INSET);
    expect(overflow).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const after = drawnViewport(container);
    // Panned by the overflow and not a pixel more, on the one axis that
    // needed it, at the zoom it was already at.
    expect(after.x).toBeCloseTo(before.x + overflow / before.k, 4);
    expect(after.y).toBeCloseTo(before.y, 6);
    expect(after.k).toBeCloseTo(before.k, 6);
  });

  it("reveals a right-side branch that overflows a frame with no card up at all", async () => {
    // The same reveal with nothing selected, which is the reader's first
    // click on a fresh map: rightInset is 0, so the free rect is the whole
    // frame less its margins and the pan is the overflow past that. Every
    // other reveal test here has a card docked, so this is the one that says
    // the no-card arithmetic works at all.
    stubFrame(1200, 800);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const before = drawnViewport(container);
    const opened = layoutMap(mapView, new Set(["E-1"]));
    const shown = ["E-1", "F-1", "F-2"].map((id) => opened.boxes.get(id)!);
    const right = Math.max(...shown.map((b) => b.x + b.w));
    const overflow = (right - before.x) * before.k - (1200 - FIT_INSET);
    expect(overflow).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const after = drawnViewport(container);
    expect(after.x).toBeCloseTo(before.x + overflow / before.k, 4);
    expect(after.y).toBeCloseTo(before.y, 6);
    expect(after.k).toBeCloseTo(before.k, 6);
  });

  it("holds still when what the chevron opened is already on screen", async () => {
    // The map moves for a reader who cannot see what they just asked for, and
    // for nobody else: a pan on every chevron would be the map twitching
    // under every click.
    stubFrame(2400, 1200);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const world = () => container.querySelector(".map-wires")?.getAttribute("transform");
    const before = world();
    fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    expect(world()).toBe(before);
  });

  it("reveals a left-side branch that opens past the frame's own edge", async () => {
    // The Bugs branch hangs off the left of the root, so its bugs open away
    // from the card and off the other side of the window.
    stubFrame(1200, 800);
    const { container } = renderMap("/explore?mode=map&sel=E-1");
    await screen.findByRole("heading", { name: "Foundation" });
    const before = drawnViewport(container);
    const opened = layoutMap(mapView, new Set(["shelf:bug"]));
    const shown = ["shelf:bug", "B-1"].map((id) => opened.boxes.get(id)!);
    const left = Math.min(...shown.map((b) => b.x));
    const shortfall = FIT_INSET - (left - before.x) * before.k;
    expect(shortfall).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Expand Bugs" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(4));
    const after = drawnViewport(container);
    // The other way, by the amount it was short.
    expect(after.x).toBeCloseTo(before.x - shortfall / before.k, 4);
    expect(after.k).toBeCloseTo(before.k, 6);
  });

  it("reveals into the half of a narrow frame the map keeps for itself", async () => {
    // The reveal's own right edge was the raw frame.w - rightInset - FIT_INSET
    // rather than the clamp freeWidth applies for every fit-like caller beside
    // it. DOCK_INSET off a 360px frame is negative room, so every branch read
    // as "wider than the space there is" and was anchored to an edge that is
    // off the frame: opening the Bugs branch parked the whole thing to the
    // left of the window, out of sight. Stated as the property rather than as
    // an arithmetic: whatever the pan works out to, the card the reader
    // clicked ends up inside the half of the frame the map never gives away.
    stubFrame(360, 600);
    const { container } = renderMap("/explore?mode=map&sel=E-1");
    await screen.findByRole("heading", { name: "Foundation" });
    fireEvent.click(screen.getByRole("button", { name: "Expand Bugs" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(4));
    const after = drawnViewport(container);
    const shelf = layoutMap(mapView, new Set(["shelf:bug"])).boxes.get("shelf:bug")!;
    const kept = Math.max(360 - DOCK_INSET, 360 / 2);
    expect((shelf.x - after.x) * after.k).toBeGreaterThanOrEqual(0);
    expect((shelf.x + shelf.w - after.x) * after.k).toBeLessThanOrEqual(kept);
  });

  it("anchors the edge nearest the card when what opened is wider than the room there is", async () => {
    // A branch bigger than the free rect cannot be brought inside it at all,
    // so the reader gets the end nearest the card they clicked: on the right
    // of the map that is the branch's left edge, where its connectors leave
    // the parent. A stored viewport is what puts the map at 1:1 in a small
    // frame, which is the shape this rule is for.
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: [], viewport: { x: 0, y: -300, k: 1 } }));
    stubFrame(700, 600);
    const { container } = renderMap("/explore?mode=map&sel=E-1");
    await screen.findByRole("heading", { name: "Foundation" });
    fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const after = drawnViewport(container);
    const opened = layoutMap(mapView, new Set(["E-1"]));
    // The epic's own left edge, the leftmost of everything the click showed,
    // sits at the free rect's own left edge.
    const near = opened.boxes.get("E-1")!.x;
    expect((near - after.x) * after.k).toBeCloseTo(FIT_INSET, 4);
  });

  it("never pans when a chevron closes a branch", async () => {
    // Closing takes drawing away, which cannot put anything out of view. A
    // map that jumped on a collapse would be moving for a reader who was
    // tidying up.
    stubFrame(1200, 800);
    const { container } = renderMap("/explore?mode=map&sel=E-1");
    await screen.findByRole("heading", { name: "Foundation" });
    fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const world = () => container.querySelector(".map-wires")?.getAttribute("transform");
    const opened = world();
    fireEvent.click(screen.getByRole("button", { name: "Collapse Foundation" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(world()).toBe(opened);
  });

  it("remembers where a chevron's reveal left the map", async () => {
    // A chevron is a gesture, so the pan it causes is the reader's own and
    // has to survive the tab closing. Nothing else in this file follows a
    // reveal as far as storage: the reveal tests read the drawing and the
    // memory tests pan by hand.
    //
    // Faked before the render, for the reason the Fit test above carries: a
    // real persist timer left running under a fake clock cannot be cancelled
    // and writes this project's memory during a later test.
    stubFrame(1200, 800);
    vi.useFakeTimers();
    try {
      const { container } = renderMap();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50);
      });
      expect(container.querySelectorAll(".map-card")).toHaveLength(3);
      const before = drawnViewport(container);
      fireEvent.click(screen.getByRole("button", { name: "Expand Foundation" }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
      const after = drawnViewport(container);
      expect(after.x).not.toBeCloseTo(before.x, 4);
      const stored = JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}");
      expect(stored.open).toEqual(["E-1"]);
      expect(stored.viewport.x).toBeCloseTo(after.x, 6);
      expect(stored.viewport.y).toBeCloseTo(after.y, 6);
      expect(stored.viewport.k).toBeCloseTo(after.k, 6);
    } finally {
      vi.useRealTimers();
    }
  });

  it("brings a card the keyboard reaches into view, and remembers nothing for it", async () => {
    // Every card is a button and Tab walks them in wire order, so on any map
    // bigger than the window Tab lands on a button the reader cannot see:
    // focus is somewhere off the frame with nothing on screen to say where.
    // Focusing a card pans it in, by the same rule an opening chevron reveals
    // what it drew. A stored viewport is what puts a card off the frame here
    // without a gesture: the map is at 1:1 in a 400px window, where the two
    // features sit past its right edge.
    //
    // Faked before the seed and the render, for the reason the Fit test above
    // carries: a real persist timer left running under a fake clock cannot be
    // cancelled and writes this project's memory during a later test.
    stubFrame(400, 300);
    vi.useFakeTimers();
    try {
      localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: { x: 0, y: -300, k: 1 } }));
      const { container } = renderMap();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50);
      });
      expect(container.querySelectorAll(".map-card")).toHaveLength(5);
      const stored = localStorage.getItem("tiller.map.acme");
      const before = drawnViewport(container);
      const box = layoutMap(mapView, new Set(["E-1"])).boxes.get("F-1")!;
      expect((box.x - before.x) * before.k).toBeGreaterThan(400);
      fireEvent.focus(screen.getByRole("button", { name: /^Feature\s*Skeleton/ }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
      // Inside the frame, at the margin every other trip leaves.
      const after = drawnViewport(container);
      expect((box.x - after.x) * after.k).toBeGreaterThanOrEqual(FIT_INSET - 0.001);
      expect((box.x + box.w - after.x) * after.k).toBeLessThanOrEqual(400 - FIT_INSET + 0.001);
      // Tabbing through a map is reading it, not moving it: nothing about the
      // pan reaches storage, so a reader who tabs to the far side of the tree
      // and leaves comes back to where they actually were.
      expect(localStorage.getItem("tiller.map.acme")).toBe(stored);
    } finally {
      vi.useRealTimers();
    }
  });

  it("remembers a trip taken from the dock, and never one a link merely landed on", async () => {
    // The fit and the trip a link asks for are both involuntary: the reader
    // opened a URL, they did not move the map. Writing there would put the
    // untouched default open set into memory, and reconcileOpen only ever
    // drops ids, so an epic minted next week would render closed forever for
    // whoever followed that link.
    stubFrame(1200, 800);
    vi.useFakeTimers();
    try {
      const { container } = renderMap("/explore?mode=map&sel=F-2");
      // Twice: the first advance lets the two fetches land and the mode fit
      // and travel, which is when the debounce timer is scheduled at all;
      // the second is the wait that timer is for.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50);
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
      expect(container.querySelector('.map-card[data-id="F-2"]')).toHaveClass("selected");
      expect(localStorage.getItem("tiller.map.acme")).toBeNull();
      // Picking a node out of the docked card is a gesture, so where it left
      // the map is worth keeping.
      fireEvent.click(screen.getByRole("button", { name: "Inspect on map: Skeleton" }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
      // What is kept is the trip itself: the center of the width the panel
      // leaves, read in world units, is the card the panel's link named. A
      // bare "x > 0" said the same thing only while the world happened to
      // start at the origin's right; the root is the origin now, so half the
      // map is at negative x.
      const stored = JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").viewport;
      const f1 = layoutMap(mapView, new Set(["E-1"])).boxes.get("F-1")!;
      expect(stored.x + (1200 - DOCK_INSET) / (2 * stored.k)).toBeCloseTo(f1.x + f1.w / 2, 4);
    } finally {
      vi.useRealTimers();
    }
  });

  it("fits a project whose memory holds an open set and no viewport", async () => {
    // Memory written before the reader ever panned carries viewport: null.
    // The restore treated having a record at all as having a place to be, so
    // that map opened at 1:1 on the origin, which is the middle of the root
    // card with the rest of the tree off screen.
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: null }));
    stubFrame(400, 300);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const drawn = drawnViewport(container);
    const want = fitOf(new Set(["E-1"]), { w: 400, h: 300 });
    expect(drawn.x).toBeCloseTo(want.x, 4);
    expect(drawn.y).toBeCloseTo(want.y, 4);
    expect(drawn.k).toBeCloseTo(want.k, 6);
  });

  it("restores a stored viewport into the drawing", async () => {
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: { x: 10, y: 20, k: 1.5 } }));
    stubFrame(400, 300);
    const { container } = renderMap();
    // Five, not the default six: the stored set holds the epic open and says
    // nothing about the Bugs branch, so that branch stays shut.
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    const drawn = drawnViewport(container);
    expect(drawn.x).toBeCloseTo(10, 6);
    expect(drawn.y).toBeCloseTo(20, 6);
    expect(drawn.k).toBeCloseTo(1.5, 6);
  });

  it("expands and collapses everything from the toolbar, and remembers it per project", async () => {
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    // Expand all reaches the shelf branches too, not only the epics: the
    // button that undoes a collapse has to reach everything the collapse did.
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    expect(JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").open).toEqual(["E-1", "shelf:bug"]);
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    expect(JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").open).toEqual([]);
  });

  it("re-fits the map when Expand all and Collapse all change how much there is to see", async () => {
    // Expand all made the drawing several times bigger and left the zoom
    // where it was, so the reader got a corner of it; Collapse all left the
    // frame zoomed out over mostly empty paper. Both are gestures, so each
    // one fits what it just made.
    stubFrame(400, 300);
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    const opened = fitOf(EXPANDED, { w: 400, h: 300 });
    expect(drawnViewport(container).x).toBeCloseTo(opened.x, 4);
    expect(drawnViewport(container).y).toBeCloseTo(opened.y, 4);
    expect(drawnViewport(container).k).toBeCloseTo(opened.k, 6);
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    const closed = fitOf(new Set(), { w: 400, h: 300 });
    expect(drawnViewport(container).x).toBeCloseTo(closed.x, 4);
    expect(drawnViewport(container).k).toBeCloseTo(closed.k, 6);
  });

  it("re-fits Expand all inside the width the card leaves too", async () => {
    // changeOpenAndFit is the third caller of the fit, after the first visit
    // and the toolbar's Fit, and it is the one most likely to run with a card
    // up: the reader selected something, then opened everything to see where
    // it sits.
    stubFrame(1200, 800);
    const { container } = renderMap("/explore?mode=map&sel=E-1");
    await screen.findByRole("heading", { name: "Foundation" });
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    const want = fitOf(EXPANDED, { w: 1200, h: 800 }, DOCK_INSET);
    const drawn = drawnViewport(container);
    expect(drawn.k).toBeCloseTo(want.k, 6);
    expect(drawn.x).toBeCloseTo(want.x, 4);
    expect(drawn.y).toBeCloseTo(want.y, 4);
  });

  it("keeps the work-link overlay local without changing remembered map navigation", async () => {
    const { container } = renderMap();
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    // An arc needs two cards to run between: collapsed, the fixture's one
    // dependency lifts onto the epic holding both of its ends and drops.
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(6));
    expect(container.querySelectorAll("path.map-edge")).toHaveLength(0);
    const before = localStorage.getItem("tiller.map.acme");
    fireEvent.click(screen.getByRole("button", { name: "All dependencies" }));
    expect(screen.getByRole("button", { name: "All dependencies" })).toHaveAttribute("aria-pressed", "true");
    expect([...container.querySelectorAll("path.map-edge")].map((arc) => arc.getAttribute("data-kind"))).toEqual(["depends_on"]);
    // The toggle writes nothing of its own: it is a look a reader takes at
    // one question, not a setting they keep.
    expect(localStorage.getItem("tiller.map.acme")).toBe(before);
  });

  // /api/status (Explore) and /api/map (MapMode) are independent
  // fetches with no ordering guarantee, and the map's memory is keyed by the
  // project the status payload names. When the map wins that race the key is
  // not known yet, and materializing the open set right then would ignore
  // what this project had stored and then overwrite it with the default. The
  // `ready` gate (Explore's own summarySettled) is what holds it.
  it("waits for the project key before restoring its memory, even when the map payload lands first", async () => {
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: null }));
    stubFetchWithDelay(
      {
        "/api/status": summary,
        "/api/tree": { project: "acme", nodes: [] },
        "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
        "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] },
        "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] },
        "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] },
        "/api/map": mapView,
      },
      ["/api/status"],
    );
    const { container } = render(
      <MemoryRouter initialEntries={["/explore?mode=map"]}>
        <App />
      </MemoryRouter>,
    );
    // Nothing is drawn while the key is still outstanding, not even the
    // default open set the map would otherwise have to take back. What
    // arrives is the stored set, which holds the epic open, and not the
    // collapsed default.
    await screen.findByText(/loading the map/i);
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    expect(container.querySelector('.map-card[data-id="F-1"]')).not.toBeNull();
    expect(container.querySelector('.map-card[data-id="B-1"]')).toBeNull();
  });

  it("states a failed fetch honestly", async () => {
    stubFetch({ "/api/status": summary, "/api/tree": { project: "acme", nodes: [] }, "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] }, "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] }, "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] }, "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] } });
    render(
      <MemoryRouter initialEntries={["/explore?mode=map"]}>
        <App />
      </MemoryRouter>,
    );
    expect(await screen.findByText(/map unavailable/i)).toBeInTheDocument();
  });

  // finding 1: /api/status can FAIL while /api/map succeeds.
  // summarySettled flips true on an error, so the map draws on its defaults
  // with no project key at all - and the topbar's refresh then re-runs every
  // fetch, which is the first moment this project's memory can be read.
  // Without the restore latch the map stayed on the defaults it had already
  // materialized, and the persist effect, firing on the mere arrival of a
  // key, wrote those defaults straight over the stored state.
  it("restores its memory when the project key arrives on a later refresh", async () => {
    localStorage.setItem("tiller.map.acme", JSON.stringify({ v: 2, open: ["E-1"], viewport: null }));
    const withoutStatus = {
      "/api/tree": { project: "acme", nodes: [] },
      "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
      "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] },
      "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] },
      "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] },
      "/api/map": mapView,
    };
    stubFetch(withoutStatus);
    const { container } = render(
      <MemoryRouter initialEntries={["/explore?mode=map"]}>
        <App />
      </MemoryRouter>,
    );
    // No key to read memory under, so the map draws on its defaults rather
    // than waiting on a status fetch that is never going to answer.
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(3));
    stubFetch({ ...withoutStatus, "/api/status": summary });
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    await waitFor(() => expect(container.querySelectorAll(".map-card")).toHaveLength(5));
    expect(JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").open).toEqual(["E-1"]);
  });

  // finding 2: MapCanvas calls onViewport on every pointermove,
  // so an eager write meant a JSON.stringify plus a setItem per frame of a
  // pan. The viewport reaches storage once the pan settles.
  it("holds the viewport out of storage until the pan settles, and never writes an untouched default", async () => {
    // Own every settle timer from mount onward. Switching to fake timers
    // after render leaves MapMode's initial native timer outside this clock,
    // which can race the gesture under a loaded full-suite run.
    vi.useFakeTimers();
    try {
      const { container } = renderMap();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50);
      });
      expect(container.querySelectorAll(".map-card")).toHaveLength(3);
      // Nothing stored yet: a default nobody has touched is not memory, and
      // writing it would freeze today's epics as tomorrow's remembered set.
      expect(localStorage.getItem("tiller.map.acme")).toBeNull();

      const bg = container.querySelector(".map-canvas") as Element;
      fireEvent.pointerDown(bg, { button: 0, clientX: 100, clientY: 100 });
      fireEvent.pointerMove(bg, { clientX: 160, clientY: 140 });
      fireEvent.pointerUp(bg);
      expect(localStorage.getItem("tiller.map.acme")).toBeNull();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(VIEWPORT_DEBOUNCE + 50);
      });
      expect(JSON.parse(localStorage.getItem("tiller.map.acme") ?? "{}").viewport).toEqual({ x: -60, y: -40, k: 1 });
    } finally {
      vi.useRealTimers();
    }
  });
});
