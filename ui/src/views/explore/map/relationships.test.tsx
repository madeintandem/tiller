import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MapView } from "../../../api";
import { bumpFetchGeneration } from "../../../api";
import MapMode from "../MapMode";
import { chainOf } from "./chain";
import { layoutMap } from "./layout";
import { relationshipOverlay } from "./relationships";

const view: MapView = {
  nodes: [
    { id: "E-1", noun: "epic", title: "Foundation", status: "ready" },
    { id: "F-1", noun: "feature", title: "Payments", status: "active", parent: "E-1" },
    { id: "F-2", noun: "feature", title: "Ledger", status: "ready", parent: "E-1" },
  ],
  edges: [{ kind: "depends_on", from: "F-1", to: "F-2" }],
  relationships: {
    vertices: [
      { id: "E-1", kind: "epic", title: "Foundation", status: "ready", destination: { kind: "node", noun: "epic", id: "E-1" } },
      { id: "F-1", kind: "feature", title: "Payments", status: "active", destination: { kind: "node", noun: "feature", id: "F-1" } },
      { id: "F-2", kind: "feature", title: "Ledger", status: "ready", destination: { kind: "node", noun: "feature", id: "F-2" } },
      { id: "Q-1", kind: "question", title: "Which currency?", status: "answered", destination: { kind: "node", noun: "question", id: "Q-1" } },
      { id: "A-1", kind: "amendment", title: "Split settlement", status: "proposed", destination: { kind: "node", noun: "amendment", id: "A-1" } },
      { id: "doc:.tiller/design/payment notes.md", kind: "document", title: "Payment notes", path: ".tiller/design/payment notes.md", destination: { kind: "file", path: ".tiller/design/payment notes.md", line: 1 } },
      { id: "path:internal/payments", kind: "path", path: "internal/payments" },
      { id: "external_path:legacy/payments", kind: "external_path", path: "legacy/payments" },
      { id: "F-missing", kind: "unresolved", raw: "F-missing", unresolved: true },
    ],
    edges: [
      { id: "contains", kind: "contains", from: "E-1", to: "F-1", sources: [{ file: ".tiller/epics/foundation/payments/feature.yaml", field: "parent" }] },
      { id: "depends", kind: "depends_on", from: "F-1", to: "F-2", sources: [{ file: ".tiller/epics/foundation/payments/feature.yaml", field: "depends_on" }] },
      { id: "assumes", kind: "assumes", from: "F-1", to: "Q-1", sources: [{ file: ".tiller/epics/foundation/payments/feature.yaml", field: "assumption_basis" }] },
      { id: "amends", kind: "amends", from: "A-1", to: "F-1", sources: [{ file: ".tiller/amendments/split/amendment.yaml", operation: 1, role: "target", state: "proposed" }] },
      { id: "source", kind: "sourced_from", from: "A-1", to: "doc:.tiller/design/payment notes.md", sources: [{ file: ".tiller/amendments/split/amendment.yaml", field: "source", citation_path: ".tiller/design/payment notes.md", citation_line: 23, raw: ".tiller/design/payment notes.md#L23", revision: { kind: "git_blob", value: "abc123" } }] },
      { id: "mentions", kind: "mentions", from: "doc:.tiller/design/payment notes.md", to: "F-1", sources: [{ file: ".tiller/design/payment notes.md", line: 7 }, { file: ".tiller/design/payment notes.md", line: 19 }] },
      { id: "covers", kind: "covers", from: "F-1", to: "path:internal/payments", sources: [{ file: ".tiller/epics/foundation/payments/feature.yaml", field: "surface.paths", basis: "work_surface" }] },
      { id: "external", kind: "covers", from: "doc:.tiller/design/payment notes.md", to: "external_path:legacy/payments", sources: [{ file: ".tiller/baselines.yaml", field: "describes", basis: "baseline" }] },
      { id: "missing", kind: "supersedes", from: "F-1", to: "F-missing", sources: [{ file: ".tiller/epics/foundation/payments/feature.yaml", field: "supersedes" }] },
    ],
    catalog: [
      { name: "contains", inverse: "contained_by", from_kinds: ["epic"], to_kinds: ["feature"] },
      { name: "depends_on", inverse: "depended_on_by", from_kinds: ["feature"], to_kinds: ["feature"] },
      { name: "assumes", inverse: "assumed_by", from_kinds: ["feature"], to_kinds: ["question"] },
      { name: "amends", inverse: "amended_by", from_kinds: ["amendment"], to_kinds: ["feature"] },
      { name: "sourced_from", inverse: "source_for", from_kinds: ["amendment"], to_kinds: ["document"] },
      { name: "mentions", inverse: "mentioned_by", from_kinds: ["document"], to_kinds: ["feature"] },
      { name: "covers", inverse: "covered_by", from_kinds: ["feature", "document"], to_kinds: ["path", "external_path"] },
      { name: "supersedes", inverse: "superseded_by", from_kinds: ["feature"], to_kinds: ["feature", "unresolved"] },
    ],
    diagnostics: [{ code: "unresolved", facet: "structural", message: "F-missing does not resolve." }],
    complete: true,
  },
};

const workView: MapView = {
  ...view,
  nodes: [...view.nodes, { id: "B-1", noun: "bug", title: "Ledger rounding", status: "open" }],
  edges: [
    ...view.edges,
    { kind: "attached_to", from: "B-1", to: "F-2" },
    { kind: "resolved_by", from: "B-1", to: "F-2" },
  ],
  relationships: {
    ...view.relationships,
    vertices: [...view.relationships.vertices, { id: "B-1", kind: "bug", title: "Ledger rounding", status: "open" }],
    edges: [
      ...view.relationships.edges,
      { id: "attached", kind: "attached_to", from: "B-1", to: "F-2", sources: [{ file: ".tiller/bugs/rounding/bug.yaml", field: "found_in" }] },
      { id: "resolved", kind: "resolved_by", from: "B-1", to: "F-2", sources: [{ file: ".tiller/bugs/rounding/bug.yaml", field: "resolved_by" }] },
    ],
    catalog: [
      ...view.relationships.catalog,
      { name: "attached_to", inverse: "attached_work", from_kinds: ["bug", "chore"], to_kinds: ["epic", "feature", "task", "reference"] },
      { name: "resolved_by", inverse: "resolves", from_kinds: ["bug", "chore"], to_kinds: ["epic", "feature", "task", "reference"] },
    ],
  },
};

function Navigation() {
  const location = useLocation();
  const navigate = useNavigate();
  return <><output aria-label="Location">{location.pathname}{location.search}{location.hash}</output><button type="button" onClick={() => navigate(-1)}>Back</button><button type="button" onClick={() => navigate(1)}>Forward</button></>;
}

function mountMap(selection = "F-1") {
  return render(<MemoryRouter initialEntries={[`/explore?mode=map&sel=${encodeURIComponent(selection)}`]}>
    <MapMode projectKey="test-relationships" projectName="Relationships" ready />
    <Navigation />
  </MemoryRouter>);
}

const response = (payload: MapView) => new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("relationship overlays", () => {
  it("keeps the selected neighborhood when its branch is collapsed without hiding global relationships", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(workView)));
    const { container } = mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    const inspector = screen.getByRole("complementary", { name: "Selected node" });
    const relatedIDs = () => within(inspector).getAllByRole("button", { name: /^Inspect on map:/ }).map((button) => button.getAttribute("aria-label"));
    const selectedRelations = relatedIDs();
    fireEvent.click(screen.getByRole("button", { name: "Collapse Foundation" }));
    expect(screen.queryByRole("button", { name: "Feature Payments, active" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Payments" })).toBeInTheDocument();
    expect(screen.getByLabelText("Location")).toHaveTextContent("sel=F-1");
    expect(relatedIDs()).toEqual(selectedRelations);
    expect(container.querySelector('path[data-kind="attached_to"]')).toBeNull();
    expect(container.querySelector('path[data-kind="resolved_by"]')).toBeNull();
    expect(container.querySelector('path[data-kind="assumes"]')).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Full graph" }));
    expect(container.querySelector('path[data-kind="attached_to"]')).not.toHaveClass("lit");
    expect(container.querySelector('path[data-kind="resolved_by"]')).not.toHaveClass("lit");
    fireEvent.click(screen.getByRole("button", { name: "Full graph" }));
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    expect(relatedIDs()).toEqual(selectedRelations);
  });

  it("returns to the quiet overview when Collapse all ends a selection", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(workView)));
    const { container } = mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    expect(container.querySelector('path[data-kind="assumes"]')).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(screen.getByLabelText("Location")).toHaveTextContent(/^\/explore\?mode=map$/);
    expect(screen.queryByRole("heading", { name: "Payments" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Related endpoints/ })).not.toBeInTheDocument();
    expect(container.querySelector(".map-edge")).toBeNull();
    expect(container.querySelector(".map-endpoint-card")).toBeNull();
    expect(container.querySelector(".map-card.dim")).toBeNull();
    expect(screen.getByRole("button", { name: "Expand Foundation" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    expect(screen.getByRole("button", { name: "Feature Payments, active" })).toBeInTheDocument();
    expect(container.querySelector(".map-edge")).toBeNull();
  });

  it("fits related endpoint cards inside the frame after collapsing and expanding", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0, y: 0, left: 0, top: 0, right: 1000, bottom: 400, width: 1000, height: 400, toJSON() {},
    });
    vi.stubGlobal("fetch", vi.fn(async () => response(view)));
    const { container } = mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    fireEvent.click(screen.getByRole("button", { name: "Full graph" }));
    for (const action of ["Collapse all", "Expand all"]) {
      fireEvent.click(screen.getByRole("button", { name: action }));
      const transform = container.querySelector(".map-wires")!.getAttribute("transform")!;
      const [, , translateY, scale] = /^translate\((\S+) (\S+)\) scale\((\S+)\)$/.exec(transform)!;
      const cards = container.querySelectorAll<HTMLElement>(".map-endpoint-card");
      expect(screen.getByRole("button", { name: "Full graph" })).toHaveAttribute("aria-pressed", "true");
      expect(cards).toHaveLength(6);
      for (const card of cards) {
        const top = Number(translateY) + parseFloat(card.style.top) * Number(scale);
        const bottom = top + parseFloat(card.style.height) * Number(scale);
        expect(top).toBeGreaterThanOrEqual(48);
        expect(bottom).toBeLessThanOrEqual(400);
      }
    }
  });

  it("toggles all three work-link arcs beyond the selection and restores its all-kind neighborhood", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(workView)));
    const { container } = mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    const arcKinds = () => [...container.querySelectorAll("path.map-edge")].map((arc) => arc.getAttribute("data-kind")).sort();
    const selectedKinds = arcKinds();
    expect(selectedKinds).toContain("assumes");
    expect(selectedKinds).not.toContain("attached_to");
    expect(selectedKinds).not.toContain("resolved_by");

    fireEvent.click(screen.getByRole("button", { name: "All dependencies" }));
    expect(arcKinds()).toEqual(["attached_to", "depends_on", "resolved_by"]);
    expect(screen.getByRole("combobox", { name: "Relationship kind" })).toHaveDisplayValue("Work links");
    const legend = screen.getByRole("complementary", { name: "Map key" });
    fireEvent.click(within(legend).getByText("Map key"));
    for (const label of [/Prerequisite → dependent/, /Found in/, /Carried by/]) {
      expect(within(legend).getByRole("button", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByRole("heading", { name: "Payments" })).toBeInTheDocument();
    expect(screen.getByLabelText("Location")).toHaveTextContent("/explore?mode=map&sel=F-1");

    fireEvent.click(screen.getByRole("button", { name: "All dependencies" }));
    expect(screen.getByRole("button", { name: "Full graph" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("combobox", { name: "Relationship kind" })).toHaveDisplayValue("All kinds");
    expect(arcKinds()).toEqual(selectedKinds);
    expect(screen.getByRole("heading", { name: "Payments" })).toBeInTheDocument();
  });

  it("keeps scope and the selected kind independent across preset, select, and global-toggle transitions", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(workView)));
    const { container, unmount } = mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    const arcKinds = () => [...container.querySelectorAll("path.map-edge")].map((arc) => arc.getAttribute("data-kind")).sort();
    const preset = screen.getByRole("button", { name: "All dependencies" });
    const global = screen.getByRole("button", { name: "Full graph" });
    const kind = screen.getByRole("combobox", { name: "Relationship kind" });
    fireEvent.click(preset);
    fireEvent.click(global);
    expect(preset).toHaveAttribute("aria-pressed", "false");
    expect(kind).toHaveDisplayValue("Work links");
    expect(arcKinds()).toEqual(["depends_on"]);
    fireEvent.click(global);
    expect(preset).toHaveAttribute("aria-pressed", "true");
    expect(arcKinds()).toEqual(["attached_to", "depends_on", "resolved_by"]);

    fireEvent.change(kind, { target: { value: "sourced_from" } });
    expect(preset).toHaveAttribute("aria-pressed", "false");
    expect(global).toHaveAttribute("aria-pressed", "true");
    expect(arcKinds()).toEqual(["sourced_from"]);
    fireEvent.click(global);
    expect(kind).toHaveDisplayValue("sourced_from");
    expect(arcKinds()).toEqual([]);
    fireEvent.change(kind, { target: { value: "depends_on" } });
    expect(arcKinds()).toEqual(["depends_on"]);
    fireEvent.click(preset);
    expect(arcKinds()).toEqual(["attached_to", "depends_on", "resolved_by"]);
    fireEvent.click(preset);
    expect(kind).toHaveDisplayValue("All kinds");
    expect(screen.getByLabelText("Location")).toHaveTextContent("/explore?mode=map&sel=F-1");

    fireEvent.click(preset);
    unmount();
    mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    expect(screen.getByRole("button", { name: "Full graph" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("combobox", { name: "Relationship kind" })).toHaveDisplayValue("All kinds");
  });

  it("keeps semantic relations outside layout affinity and dependency chains, while retaining collapsed and containment facts for inspection", () => {
    const tree = layoutMap(view, new Set());
    const before = [...tree.boxes];
    const overlay = relationshipOverlay(view, tree, null, true, "");
    expect([...tree.boxes]).toEqual(before);
    for (const [id, box] of tree.boxes) expect(overlay.layout.boxes.get(id)).toEqual(box);
    expect(overlay.cards.map((vertex) => vertex.kind)).toEqual(["question", "amendment", "document", "path", "external_path", "unresolved"]);
    expect(overlay.edges.map((edge) => edge.id)).toEqual(view.relationships.edges.map((edge) => edge.id));
    expect(overlay.arcs.some((arc) => arc.edge.kind === "contains")).toBe(false);
    expect(overlay.arcs.find((arc) => arc.edge.kind === "amends")?.to).toBe("E-1");
    const chain = chainOf(view, new Set(["E-1"]), "F-1");
    expect([...chain.up]).toEqual(["F-2"]);
    expect([...chain.linked]).toEqual([]);
    const mentions = relationshipOverlay(view, tree, "F-1", false, "mentions");
    expect(mentions.edges.map((edge) => edge.id)).toEqual(["mentions"]);
    expect(mentions.cards.map((vertex) => vertex.kind)).toEqual(["document"]);
  });

  it("mounts precise source evidence only on demand in the inspector, never in the map key", async () => {
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    vi.stubGlobal("fetch", vi.fn(async () => response(view)));
    mountMap("A-1");
    await screen.findByRole("heading", { name: "Split settlement" });
    const sources = screen.getByRole("region", { name: "Sources" });
    expect(within(sources).queryByText("abc123")).not.toBeInTheDocument();
    expect(within(sources).getByRole("link", { name: "Open source" })).toHaveAttribute("href", "/file/.tiller/design/payment%20notes.md#L23");
    fireEvent.click(within(sources).getByRole("button", { name: "Why linked?" }));
    fireEvent.click(within(sources).getByRole("button", { name: ".tiller/amendments/split/amendment.yaml" }));
    expect(writeText).toHaveBeenLastCalledWith(".tiller/amendments/split/amendment.yaml");
    expect(within(sources).queryByRole("link", { name: /amendment\.yaml/ })).not.toBeInTheDocument();
    expect(within(sources).getByRole("link", { name: /payment notes\.md/ })).toHaveAttribute("href", "/file/.tiller/design/payment%20notes.md#L23");
    expect(within(sources).getByText("abc123")).toBeInTheDocument();
    const key = screen.getByRole("complementary", { name: "Map key" });
    expect(within(key).queryByRole("button", { name: "Why linked?" })).not.toBeInTheDocument();
    expect(within(key).queryByText("abc123")).not.toBeInTheDocument();
    fireEvent.click(within(sources).getByRole("button", { name: "Why linked?" }));
    expect(within(sources).queryByText("abc123")).not.toBeInTheDocument();
  });

  it("opens only granted destinations, keeps code/external/unresolved coordinates inspectable, and retains exact citation lines", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(view)));
    mountMap();
    await screen.findByRole("heading", { name: "Payments" });
    fireEvent.click(screen.getByRole("button", { name: "Full graph" }));
    fireEvent.click(screen.getByRole("button", { name: "Inspect document Payment notes" }));
    let dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).getByRole("link", { name: "Open document" })).toHaveAttribute("href", "/file/.tiller/design/payment%20notes.md#L1");
    const mentions = within(dock).getByRole("region", { name: "References to work" });
    fireEvent.click(within(mentions).getByRole("button", { name: "Why linked?" }));
    const citationLinks = within(mentions).getAllByRole("link", { name: /payment notes\.md/ });
    expect(citationLinks.map((link) => link.getAttribute("href"))).toEqual([
      "/file/.tiller/design/payment%20notes.md#L7",
      "/file/.tiller/design/payment%20notes.md#L19",
    ]);
    fireEvent.click(citationLinks[1]);
    expect(screen.getByLabelText("Location")).toHaveTextContent("/file/.tiller/design/payment%20notes.md#L19");
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByRole("heading", { name: "Payment notes" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Forward" }));
    expect(screen.getByLabelText("Location")).toHaveTextContent("#L19");
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    fireEvent.click(screen.getByRole("button", { name: "Inspect question Which currency?" }));
    dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).getByRole("link", { name: "Open node" })).toHaveAttribute("href", "/node/question/Q-1");
    fireEvent.click(screen.getByRole("button", { name: "Inspect amendment Split settlement" }));
    dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).getByRole("link", { name: "Open node" })).toHaveAttribute("href", "/node/amendment/A-1");
    fireEvent.click(within(within(dock).getByRole("region", { name: "Sources" })).getByRole("button", { name: "Why linked?" }));
    expect(within(dock).getByRole("link", { name: /payment notes\.md/ })).toHaveAttribute("href", "/file/.tiller/design/payment%20notes.md#L23");
    expect(within(dock).queryByRole("link", { name: /amendment.yaml/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Inspect path internal/payments" }));
    dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).queryByRole("link", { name: /^Open (node|document)$/ })).not.toBeInTheDocument();
    expect(within(dock).getByRole("link", { name: "Open" })).toHaveAttribute("href", "/node/feature/F-1");
    expect(within(dock).getByRole("button", { name: "internal/payments" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Inspect external_path legacy/payments" }));
    dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).queryByRole("link", { name: /^Open (node|document)$/ })).not.toBeInTheDocument();
    expect(within(dock).getByRole("link", { name: "Open source" })).toHaveAttribute("href", "/file/.tiller/design/payment%20notes.md#L1");
    fireEvent.click(screen.getByRole("button", { name: "Inspect unresolved F-missing, unresolved" }));
    dock = screen.getByRole("complementary", { name: "Selected relationship endpoint" });
    expect(within(dock).queryByRole("link", { name: /^Open (node|document)$/ })).not.toBeInTheDocument();
    expect(within(dock).getByRole("link", { name: "Open" })).toHaveAttribute("href", "/node/feature/F-1");
  });

  it("retains the old graph on refresh failure without borrowing another selection's detail, and drops removed selections on success", async () => {
    let settle: (value: Response) => void = () => { throw new Error("refresh not requested"); };
    let reads = 0;
    vi.stubGlobal("fetch", vi.fn(() => {
      reads++;
      if (reads === 1) return Promise.resolve(response(view));
      return new Promise<Response>((resolve) => { settle = resolve; });
    }));
    mountMap("doc:.tiller/design/payment notes.md");
    await screen.findByRole("heading", { name: "Payment notes" });
    fireEvent.click(screen.getByRole("button", { name: "Full graph" }));
    act(() => bumpFetchGeneration());
    fireEvent.click(screen.getByRole("button", { name: "Inspect question Which currency?" }));
    expect(screen.getByRole("heading", { name: "Which currency?" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Payment notes" })).not.toBeInTheDocument();
    await act(async () => { settle(new Response("read failed", { status: 500 })); });
    expect(screen.getByText(/Refresh failed; showing the last successful map/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Inspect document Payment notes" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Which currency?" })).toBeInTheDocument();
    act(() => bumpFetchGeneration());
    const current: MapView = {
      ...view,
      relationships: {
        ...view.relationships,
        vertices: view.relationships.vertices.filter((vertex) => vertex.id !== "Q-1"),
        edges: view.relationships.edges.filter((edge) => edge.from !== "Q-1" && edge.to !== "Q-1"),
      },
    };
    await act(async () => { settle(response(current)); });
    expect(screen.queryByRole("heading", { name: "Which currency?" })).not.toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: "Selected relationship endpoint" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Refresh failed/)).not.toBeInTheDocument();
  });
});
