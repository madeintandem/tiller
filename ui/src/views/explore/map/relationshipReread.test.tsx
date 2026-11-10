// A reread (the topbar refresh, or the window regaining focus) delivers a new
// map payload object every time. The reader's mapped investigation survives a
// reread whose graph is unchanged; only a graph that actually changed clears
// it, with the existing message.

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MapView, RelationshipGraph, RelationshipVertex } from "../../../api";
import { bumpFetchGeneration } from "../../../api";
import MapMode from "../MapMode";

afterEach(() => { cleanup(); localStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

const work = (id: string, title: string): RelationshipVertex => ({ id, kind: "feature", title, status: "draft", destination: { kind: "node", noun: "feature", id } });

function view(dependentTitle: string): MapView {
  const relationships: RelationshipGraph = {
    vertices: [work("F-main", "Creation"), work("F-before", "Prerequisite"), work("F-after", dependentTitle)],
    edges: [
      { id: "before", kind: "depends_on", from: "F-main", to: "F-before", sources: [{ file: "feature.yml", field: "depends_on" }] },
      { id: "after", kind: "depends_on", from: "F-after", to: "F-main", sources: [{ file: "other.yml", field: "depends_on" }] },
    ],
    catalog: [{ name: "depends_on", inverse: "depended_on_by", from_kinds: ["feature"], to_kinds: ["feature"] }],
    diagnostics: [], complete: true,
  };
  return { nodes: [{ noun: "feature", id: "F-main", title: "Creation", status: "draft" }], edges: [], relationships };
}

function stubMap(initial: MapView) {
  let current = initial;
  const reads = vi.fn(async () => new Response(JSON.stringify(current), { status: 200, headers: { "Content-Type": "application/json" } }));
  vi.stubGlobal("fetch", reads);
  return { reads, serve: (next: MapView) => { current = next; } };
}

async function mapEveryRelationship() {
  render(<MemoryRouter initialEntries={["/explore?mode=map"]}><MapMode projectKey="reread" projectName="Reread" ready /></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", { name: "Browse relationships" }));
  fireEvent.click(screen.getByRole("button", { name: "Show results on map" }));
  expect(screen.getByRole("button", { name: /^Clear mapped results/ })).toBeInTheDocument();
}

async function reread(reads: ReturnType<typeof vi.fn>) {
  const before = reads.mock.calls.length;
  act(() => bumpFetchGeneration());
  await waitFor(() => expect(reads.mock.calls.length).toBe(before + 1));
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
}

describe("Map investigation across a reread", () => {
  it("keeps the mapped results when the reread brings the same graph", async () => {
    const map = stubMap(view("Required documents"));
    await mapEveryRelationship();

    await reread(map.reads);

    expect(screen.getByRole("button", { name: /^Clear mapped results/ })).toBeInTheDocument();
    expect(screen.queryByText(/Map refreshed; previous investigation results cleared/)).not.toBeInTheDocument();
  });

  it("clears the mapped results and says so when the reread brings a changed graph", async () => {
    const map = stubMap(view("Required documents"));
    await mapEveryRelationship();

    map.serve(view("Required documents, renamed"));
    await reread(map.reads);

    expect(await screen.findByText(/Map refreshed; previous investigation results cleared/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Clear mapped results/ })).not.toBeInTheDocument();
  });
});
