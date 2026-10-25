import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MapView, RelationshipEdge, RelationshipGraph, RelationshipQueryResult, RelationshipVertex } from "../../../api";
import MapMode from "../MapMode";
import ReferenceIssues from "./ReferenceIssues";
import { SelectedRelationships } from "./RelationshipDetails";
import RelationshipResults from "./RelationshipResults";
import { scopedReferenceIssues } from "./relationshipPresentation";

const work = (id: string, title: string, status = "draft"): RelationshipVertex => ({ id, kind: "feature", title, status, destination: { kind: "node", noun: "feature", id } });
const graph: RelationshipGraph = {
  vertices: [work("F-main", "Creation"), ...Array.from({ length: 5 }, (_, i) => work(`F-before${i}`, `Prerequisite ${i}`, i === 4 ? "done" : "draft")), work("F-after", "Required documents")],
  edges: [
    ...Array.from({ length: 5 }, (_, i) => ({ id: `before${i}`, kind: "depends_on", from: "F-main", to: `F-before${i}`, sources: [{ file: "feature.yml", field: "depends_on" }] })),
    { id: "after", kind: "depends_on", from: "F-after", to: "F-main", sources: [{ file: "other.yml", field: "depends_on" }] },
  ],
  catalog: [
    { name: "depends_on", inverse: "depended_on_by", from_kinds: ["feature"], to_kinds: ["feature"] },
    { name: "mentions", inverse: "mentioned_by", from_kinds: ["document"], to_kinds: ["feature"] },
    { name: "at_path", inverse: "document_at", from_kinds: ["document"], to_kinds: ["path"] },
    { name: "covers", inverse: "covered_by", from_kinds: ["feature"], to_kinds: ["path"] },
  ],
  diagnostics: [], complete: true,
};
const asView = (relationships: RelationshipGraph): MapView => ({
  nodes: [{ noun: "feature", id: "F-main", title: "Creation", status: "draft" }], edges: [], relationships,
});
const response = (data: unknown) => new Response(JSON.stringify(data), { status: 200, headers: { "Content-Type": "application/json" } });

afterEach(() => { cleanup(); localStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("selected relationship investigation", () => {
  it("investigates Tweak attachment and carrier paths using TW node destinations", () => {
    const tweak: RelationshipVertex = { id: "TW-abc123", kind: "tweak", title: "Button spacing", status: "ready", destination: { kind: "node", noun: "tweak", id: "TW-abc123" } };
    const edges: RelationshipEdge[] = [
      { id: "attachment", kind: "attached_to", from: tweak.id, to: "F-main", sources: [{ file: ".tiller/tweaks/button/tweak.yml", field: "found_in" }] },
      { id: "carrier", kind: "resolved_by", from: tweak.id, to: "F-main", sources: [{ file: ".tiller/tweaks/button/tweak.yml", field: "resolved_by" }] },
    ];
    const onSelect = vi.fn();
    render(<MemoryRouter><SelectedRelationships selected="F-main" edges={edges} byID={new Map([...graph.vertices, tweak].map((vertex) => [vertex.id, vertex]))} onSelect={onSelect} /></MemoryRouter>);
    const opens = screen.getAllByRole("link", { name: "Open" });
    expect(opens).toHaveLength(2);
    for (const link of opens) expect(link).toHaveAttribute("href", "/node/tweak/TW-abc123");
    fireEvent.click(screen.getAllByRole("button", { name: "Inspect on map: Button spacing" })[0]);
    expect(onSelect).toHaveBeenCalledWith("TW-abc123");
  });
  it("separates prerequisite and dependent membership, statuses, navigation, and lazy evidence", () => {
    const onSelect = vi.fn();
    render(<MemoryRouter><SelectedRelationships selected="F-main" edges={graph.edges} byID={new Map(graph.vertices.map((vertex) => [vertex.id, vertex]))} onSelect={onSelect} /></MemoryRouter>);
    const before = screen.getByRole("region", { name: "Depends on" });
    const after = screen.getByRole("region", { name: "Required by" });
    expect(within(before).getAllByRole("button", { name: /^Inspect on map:/ })).toHaveLength(5);
    expect(within(before).getByText("4 draft · 1 done")).toBeInTheDocument();
    expect(within(before).queryByText("Required documents")).not.toBeInTheDocument();
    expect(within(after).getAllByRole("button", { name: /^Inspect on map:/ })).toHaveLength(1);
    expect(within(after).getByRole("link", { name: "Open" })).toHaveAttribute("href", "/node/feature/F-after");
    fireEvent.click(within(after).getByRole("button", { name: "Inspect on map: Required documents" }));
    expect(onSelect).toHaveBeenCalledWith("F-after");
    expect(screen.queryByText("other.yml")).not.toBeInTheDocument();
    fireEvent.click(within(after).getByRole("button", { name: "Why linked?" }));
    expect(within(after).getByRole("button", { name: "other.yml" })).toBeInTheDocument();
    expect(within(after).getByRole("button", { name: "tiller query --from F-after --follow depends_on --json" })).toBeInTheDocument();
  });

  it("keeps owned-document issues local and server-marked extraction loss scan-wide without inferring impact from codes", () => {
    const scoped: RelationshipGraph = {
      ...graph, complete: false,
      vertices: [...graph.vertices, { id: "doc:owned.md", kind: "document", document_owner: "F-main", path: "owned.md", destination: { kind: "file", path: "owned.md", line: 1 } }],
      diagnostics: [
        { code: "unresolved_reference", facet: "provenance", message: "unsupported heading", subjects: ["doc:owned.md"], source: { file: "owned.md", line: 8, field: "source", raw: "notes.md#Heading" } },
        { code: "unresolved_document", facet: "documents", message: "unrelated missing target", subjects: ["F-after"], source: { file: "missing.md" } },
        { code: "future_loss_code", facet: "contracts", message: "new extraction failure", incomplete: true, subjects: ["F-after"] },
        { code: "ambiguous_reference", facet: "structural", message: "ordinary ambiguity", subjects: ["F-after"] },
      ],
    };
    const result = scopedReferenceIssues(scoped, "F-main");
    expect(result.local.map((issue) => issue.message)).toEqual(["unsupported heading"]);
    expect(result.global.map((issue) => issue.message)).toEqual(["new extraction failure"]);
    render(<MemoryRouter><ReferenceIssues graph={scoped} selected="F-main" onSelect={() => undefined} /></MemoryRouter>);
    expect(screen.getByRole("heading", { name: "Reference issues (1)" })).toBeInTheDocument();
    expect(screen.queryByText("unrelated missing target")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Unsupported citation format (1 occurrences)" }));
    expect(screen.getByRole("link", { name: "Open source document" })).toHaveAttribute("href", "/file/owned.md#L8");
    expect(screen.getByRole("button", { name: "owned.md#L8 · source" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "notes.md#Heading" })).toBeInTheDocument();
    expect(screen.getByText(/does not establish that a file is missing/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tiller query --from doc:owned.md --follow mentioned_by --depth 0 --json" })).toBeInTheDocument();
  });

  it("retains extraction-loss explanation with no selection and keeps historical intent distinct from a missing target", async () => {
    const issues: RelationshipGraph = { ...graph, complete: false, diagnostics: [
      { code: "unresolved_amendment_endpoint", facet: "amendments", message: "historical key was reused", subjects: ["F-after"], source: { file: "amendment.yml", operation: 3, raw: "old-key", state: "proposed" } },
      { code: "unresolved_reference", facet: "structural", message: "missing feature", subjects: ["F-main"] },
      { code: "new_loss_code", facet: "documents", message: "omitted input", incomplete: true },
    ] };
    vi.stubGlobal("fetch", vi.fn(async () => response(asView(issues))));
    render(<MemoryRouter initialEntries={["/explore?mode=map"]}><MapMode projectKey="issues" projectName="Issues" ready /></MemoryRouter>);
    await screen.findByText(/Extraction incomplete: unread or omitted/);
    fireEvent.click(screen.getByRole("button", { name: "Inspect project reference issues" }));
    fireEvent.click(screen.getByRole("button", { name: "Unresolved historical context (1 occurrences)" }));
    expect(screen.getByText(/not an operative work hold/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Missing target (1 occurrences)" })).toBeInTheDocument();
    expect(screen.getByText("historical key was reused")).toBeInTheDocument();
  });

  it("copies missing and unreadable document coordinates without granting source or citation navigation", () => {
    const missing: RelationshipGraph = { ...graph, complete: false,
      vertices: [...graph.vertices,
        { id: "doc:missing.md", kind: "document", path: "missing.md", unresolved: true },
        { id: "doc:unreadable.md", kind: "document", path: "unreadable.md", unresolved: true },
        { id: "doc:author.md", kind: "document", path: "author.md", destination: { kind: "file", path: "author.md", line: 1 } },
      ],
      diagnostics: [
        { code: "unresolved_document", facet: "documents", message: "missing document", subjects: ["F-main", "doc:missing.md"], source: { file: "missing.md" } },
        { code: "document_unreadable", facet: "documents", message: "unreadable document", subjects: ["F-main", "doc:unreadable.md"], incomplete: true, source: { file: "unreadable.md" } },
        { code: "unresolved_citation", facet: "provenance", message: "missing citation", subjects: ["F-main"], source: { file: "author.md", line: 9, citation_path: "missing.md", citation_line: 4, raw: "missing.md#L4" } },
      ],
    };
    render(<MemoryRouter><ReferenceIssues graph={missing} selected="F-main" onSelect={() => undefined} /></MemoryRouter>);
    for (const label of ["Missing target", "Input could not be read", "Unavailable citation coordinate"]) {
      fireEvent.click(screen.getByRole("button", { name: `${label} (1 occurrences)` }));
    }
    expect(screen.queryByRole("link", { name: /missing\.md/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /unreadable\.md/ })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open source document" })).toHaveAttribute("href", "/file/author.md#L9");
    expect(screen.getAllByRole("button", { name: "missing.md" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "tiller query --from F-main --follow mentioned_by --depth 0 --json" })).toHaveLength(3);
  });

  it("does not turn an unresolved out-of-range citation into a link even when its document exists", () => {
    const invalidLine: RelationshipGraph = { ...graph,
      vertices: [...graph.vertices, { id: "doc:existing.md", kind: "document", path: "existing.md", destination: { kind: "file", path: "existing.md", line: 1 } }],
      diagnostics: [{ code: "unresolved_citation", facet: "provenance", message: "line 999 is unavailable", subjects: ["F-main"], source: { file: "existing.md", line: 3, citation_path: "existing.md", citation_line: 999, raw: "existing.md#L999" } }],
    };
    render(<MemoryRouter><ReferenceIssues graph={invalidLine} selected="F-main" onSelect={() => undefined} /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "Unavailable citation coordinate (1 occurrences)" }));
    expect(screen.getByRole("link", { name: "Open source document" })).toHaveAttribute("href", "/file/existing.md#L3");
    expect(screen.queryByRole("link", { name: "existing.md#L999" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "existing.md#L999" })).toHaveLength(2);
  });

  it("synchronizes keyboard-reachable key filters with the toolbar and restores the selected neighborhood", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(asView(graph))));
    render(<MemoryRouter initialEntries={["/explore?mode=map&sel=F-main"]}><MapMode projectKey="key" projectName="Key" ready /></MemoryRouter>);
    await screen.findByRole("heading", { name: "Creation" });
    const key = screen.getByRole("complementary", { name: "Map key" });
    fireEvent.click(within(key).getByText("Map key"));
    const prerequisite = within(key).getByRole("button", { name: /Prerequisite → dependent/ });
    prerequisite.focus();
    expect(prerequisite).toHaveFocus();
    fireEvent.click(prerequisite);
    expect(screen.getByRole("combobox", { name: "Relationship kind" })).toHaveValue("depends_on");
    expect(prerequisite).toHaveAttribute("aria-pressed", "true");
    fireEvent.change(screen.getByRole("combobox", { name: "Relationship kind" }), { target: { value: "mentions" } });
    expect(prerequisite).toHaveAttribute("aria-pressed", "false");
    expect(within(screen.getByRole("complementary", { name: "Selected node" })).queryByRole("region", { name: "Depends on" })).not.toBeInTheDocument();
    fireEvent.click(within(key).getByRole("button", { name: "All kinds" }));
    expect(screen.getByRole("region", { name: "Depends on" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Required by" })).toBeInTheDocument();
  });
});

describe("bounded project relationships", () => {
  const documents: RelationshipVertex[] = Array.from({ length: 1000 }, (_, i) => ({ id: `doc:notes${i}.md`, kind: "document", path: `notes${i}.md`, destination: { kind: "file", path: `notes${i}.md`, line: 1 } }));
  const mentions: RelationshipEdge[] = documents.map((vertex, i) => ({ id: `mention${i}`, kind: "mentions", from: vertex.id, to: "F-main", sources: [{ file: vertex.path!, line: 3, raw: `source-${i}` }] }));
  const large: RelationshipGraph = { ...graph, vertices: [...graph.vertices, ...documents, { id: "path:notes0.md", kind: "path", path: "notes0.md" }], edges: [...mentions, { id: "coordinate", kind: "at_path", from: documents[0].id, to: "path:notes0.md", sources: [] }] };


  it("filters selected results by canonical direction rather than dependency-arrow layout", () => {
    render(<MemoryRouter><RelationshipResults graph={graph} selected="F-main" mode="relationships" onMode={() => undefined} onSelect={() => undefined} onShow={() => undefined} onClose={() => undefined} /></MemoryRouter>);
    fireEvent.change(screen.getByRole("combobox", { name: "Result scope" }), { target: { value: "selected" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Result direction" }), { target: { value: "in" } });
    expect(screen.getByRole("button", { name: "Inspect on map: Required documents" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Inspect on map: Prerequisite 0" })).not.toBeInTheDocument();
    expect(screen.getByText(/Incoming · All kinds · 1 relationships/)).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Result direction" }), { target: { value: "out" } });
    expect(screen.queryByRole("button", { name: "Inspect on map: Required documents" })).not.toBeInTheDocument();
    expect(screen.getByText(/Outgoing · All kinds · 5 relationships/)).toBeInTheDocument();
  });
  it("bounds a thousand results, continues by page, searches and exposes coordinates without silently expanding the graph", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(asView(large))));
    const { container } = render(<MemoryRouter initialEntries={["/explore?mode=map"]}><MapMode projectKey="large" projectName="Large" ready /></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", { name: "Browse relationships" }));
    const results = screen.getByRole("complementary", { name: "Relationship investigation" });
    expect(within(results).getByText(/1001 relationships · showing 1–25/)).toBeInTheDocument();
    expect(within(results).getAllByRole("button", { name: "Why linked?" })).toHaveLength(25);
    expect(within(results).queryByText("source-0")).not.toBeInTheDocument();
    expect(container.querySelectorAll(".map-endpoint-card")).toHaveLength(0);
    fireEvent.click(within(results).getByRole("button", { name: "Next results" }));
    expect(within(results).getByRole("button", { name: "Inspect on map: notes25.md" })).toBeInTheDocument();
    expect(within(results).queryByRole("button", { name: "Inspect on map: notes0.md" })).not.toBeInTheDocument();
    fireEvent.change(within(results).getByRole("searchbox", { name: "Search relationships" }), { target: { value: "source-999" } });
    expect(within(results).getByText(/1 relationships · showing 1–1/)).toBeInTheDocument();
    fireEvent.click(within(results).getByRole("button", { name: "Show results on map" }));
    expect(container.querySelectorAll(".map-endpoint-card")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Inspect document notes999.md" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(screen.getByRole("button", { name: "Clear mapped results (1)" })).toBeInTheDocument();
    expect(container.querySelectorAll(".map-endpoint-card")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Browse relationships" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Result kind" }), { target: { value: "at_path" } });
    expect(screen.getByText(/1 relationships · showing 1–1/)).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Inspect on map: notes0.md" })).toHaveLength(2);
  });

  it("uses the actual selected ID for query presets and retains a derived enclosing-path match and its provenance", async () => {
    const result: RelationshipQueryResult = {
      starts: ["path:src/new.ts"], follow: ["covered_by"], depth: 1, where: [], matched: ["F-main"],
      diagnostics: [{ code: "unresolved_reference", facet: "structural", message: "query subject has a missing reference", subjects: ["F-main"] }], complete: true,
      vertices: [{ ...work("F-main", "Creation"), depth: 1, seed: false }, { id: "path:src/new.ts", kind: "path", path: "src/new.ts", depth: 0, seed: true }],
      edges: [{ id: "derived", kind: "covers", from: "F-main", to: "path:src/new.ts", derived: true, declared_scope: "src", sources: [{ file: "feature.yml", field: "surface.paths", basis: "work_surface" }] }],
    };
    const fetch = vi.fn(async (_input: RequestInfo | URL) => response(result)); vi.stubGlobal("fetch", fetch);
    const onSelect = vi.fn(); const onShow = vi.fn();
    render(<MemoryRouter><RelationshipResults graph={graph} selected="F-main" mode="relationships" onMode={() => undefined} onSelect={onSelect} onShow={onShow} onClose={() => undefined} /></MemoryRouter>);
    fireEvent.click(screen.getByText(/Engineering questions/));
    expect(screen.getByRole("button", { name: "tiller query --from F-main --follow depends_on --json" })).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: "Engineering question" }), { target: { value: "3" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Query start address" }), { target: { value: "path:src/new.ts" } });
    fireEvent.click(screen.getByRole("button", { name: "Run read-only query" }));
    await screen.findByText(/depth 1 · 1 matched endpoints/);
    expect(fetch.mock.calls[0][0]).toBe("/api/query?from=path%3Asrc%2Fnew.ts&follow=covered_by");
    const queryIssues = screen.getByRole("region", { name: "Query reference issues" });
    expect(within(queryIssues).getByText(/Query subjects and scan-wide warnings/)).toBeInTheDocument();
    expect(within(queryIssues).queryByText(/Whole project/)).not.toBeInTheDocument();
    expect(within(queryIssues).getByRole("heading", { name: "Reference issues (1)" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Why linked?" }));
    expect(screen.getByText(/Derived match · Declared scope: src/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "src/new.ts" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Inspect on map: src/new.ts" }));
    expect(onSelect.mock.calls[0][0]).toBe("path:src/new.ts");
    fireEvent.click(screen.getByRole("button", { name: "Show results on map" }));
    expect(onShow.mock.calls[0][0].edges.map((edge: RelationshipEdge) => edge.id)).toEqual(["derived"]);
  });

  it("reruns identical query arguments after a read failure and returns fresh results", async () => {
    const result: RelationshipQueryResult = { starts: ["F-main"], follow: ["depends_on"], depth: 1, where: [], matched: ["F-main", "F-before0"], vertices: graph.vertices.map((vertex) => ({ ...vertex, depth: vertex.id === "F-main" ? 0 : 1, seed: vertex.id === "F-main" })), edges: [graph.edges[0]], diagnostics: [], complete: true };
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response("temporarily unreadable", { status: 500 }))
      .mockResolvedValueOnce(response(result));
    vi.stubGlobal("fetch", fetch);
    render(<MemoryRouter><RelationshipResults graph={graph} selected="F-main" mode="relationships" onMode={() => undefined} onSelect={() => undefined} onShow={() => undefined} onClose={() => undefined} /></MemoryRouter>);
    fireEvent.click(screen.getByText(/Engineering questions/));
    fireEvent.click(screen.getByRole("button", { name: "Run read-only query" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Run read-only query" }));
    await screen.findByText(/depth 1 · 2 matched endpoints/);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Why linked?" })).toHaveLength(1);
  });

  it("retains mapped virtual endpoint identity and query extraction loss while inspecting a later query", async () => {
    const virtualResult = (path: string, complete: boolean): RelationshipQueryResult => ({
      starts: [`path:${path}`], follow: ["covered_by"], depth: 1, where: [], matched: ["F-main"], complete,
      vertices: [{ ...work("F-main", "Creation"), depth: 1, seed: false }, { id: `path:${path}`, kind: "path", path, depth: 0, seed: true }],
      edges: [{ id: `derived:${path}`, kind: "covers", from: "F-main", to: `path:${path}`, derived: true, declared_scope: "src", sources: [{ file: "feature.yml", field: "surface.paths" }] }],
      diagnostics: complete ? [] : [{ code: "query_input_loss", facet: "documents", message: "Query omitted an unreadable input", incomplete: true }],
    });
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/map") return response(asView(graph));
      return response(virtualResult(new URL(url, "http://tiller.test").searchParams.get("from")!.slice(5), !url.includes("a.ts")));
    }));
    render(<MemoryRouter initialEntries={["/explore?mode=map&sel=F-main"]}><MapMode projectKey="virtual" projectName="Virtual" ready /></MemoryRouter>);
    await screen.findByRole("heading", { name: "Creation" });
    fireEvent.change(screen.getByRole("combobox", { name: "Relationship kind" }), { target: { value: "depends_on" } });
    fireEvent.click(screen.getByRole("button", { name: "Browse relationships" }));
    fireEvent.click(screen.getByText(/Engineering questions/));
    fireEvent.change(screen.getByRole("combobox", { name: "Engineering question" }), { target: { value: "3" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Query start address" }), { target: { value: "path:src/a.ts" } });
    fireEvent.click(screen.getByRole("button", { name: "Run read-only query" }));
    await screen.findByText(/depth 1 · 1 matched endpoints/);
    fireEvent.click(screen.getByRole("button", { name: "Show results on map" }));
    expect(screen.getByRole("combobox", { name: "Relationship kind" })).toHaveValue("");
    expect(screen.getByRole("region", { name: "Declared code scope" })).toBeInTheDocument();
    expect(screen.getByText(/Extraction incomplete: unread or omitted/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Inspect project reference issues" }));
    fireEvent.click(screen.getByRole("button", { name: "Other reference issues (1 occurrences)" }));
    expect(screen.getByText("Query omitted an unreadable input")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close relationship investigation" }));
    fireEvent.click(screen.getByRole("button", { name: "Browse relationships" }));
    fireEvent.click(screen.getByText(/Engineering questions/));
    fireEvent.change(screen.getByRole("combobox", { name: "Engineering question" }), { target: { value: "3" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Query start address" }), { target: { value: "path:src/b.ts" } });
    fireEvent.click(screen.getByRole("button", { name: "Run read-only query" }));
    await screen.findByText(/depth 1 · 1 matched endpoints/);
    fireEvent.click(screen.getByRole("button", { name: "Inspect on map: src/b.ts" }));
    expect(screen.getByRole("heading", { name: "src/b.ts" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close relationship investigation" }));
    fireEvent.click(screen.getByRole("button", { name: "Inspect path src/a.ts" }));
    expect(screen.getByRole("heading", { name: "src/a.ts" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clear mapped results (1)" })).toBeInTheDocument();
    expect(screen.getByText(/Extraction incomplete: unread or omitted/)).toBeInTheDocument();
  });
});
