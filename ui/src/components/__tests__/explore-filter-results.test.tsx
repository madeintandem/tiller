// Filter mode's result table (task 32): a real header row, one column for
// contributors or owner by noun, and no per-row "Contributors:" label.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Facet, ListResult, ListRow, ProjectSummary } from "../../api";
import App from "../../App";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

const summary: ProjectSummary = {
  project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
  contributors: [{ id: "jc", name: "JC Grubbs" }],
  counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
  whose_move: { human: 0, agent: 0 },
};

const axes: Record<string, Facet["axis"][]> = {
  epic: ["status", "contributor"],
  feature: ["status", "contributor", "epic"],
  task: ["status", "owner", "epic", "feature"],
  amendment: ["status"],
};

function list(noun: string, nodes: ListRow[]): ListResult {
  return {
    noun, nodes, count: nodes.length, axes,
    facets: axes[noun].map((axis) => ({ axis, options: [] })),
  } as unknown as ListResult;
}

function stub(routes: Record<string, unknown>) {
  vi.stubGlobal("fetch", vi.fn((input: unknown) => {
    const url = String(input);
    const empty = (noun: string) => ({ noun, count: 0, nodes: [] });
    const all: Record<string, unknown> = {
      "/api/status": summary,
      "/api/tree": { project: "acme", nodes: [] },
      "/api/ls?noun=question": empty("question"),
      "/api/ls?noun=bug": empty("bug"),
      "/api/ls?noun=chore": empty("chore"),
      "/api/ls?noun=tweak": empty("tweak"),
      ...routes,
    };
    if (url in all) return Promise.resolve({ ok: true, json: () => Promise.resolve(all[url]) });
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  }));
}

function renderFilter(noun: string) {
  return render(<MemoryRouter initialEntries={[`/explore?mode=filter&noun=${noun}`]}><App /></MemoryRouter>);
}

describe("Filter results table", () => {
  it("names Title, Key, Status and Contributors for a feature and repeats no label per row", async () => {
    const row: ListRow = {
      id: "F-1", key: "pay/checkout", title: "Checkout flow", status: "in_progress", ref: "pay/checkout (F-1)",
      contributors: [{ id: "jc", name: "JC Grubbs" }],
    } as ListRow;
    stub({ "/api/ls?noun=features": list("feature", [row]) });
    renderFilter("features");
    const table = await screen.findByRole("table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent))
      .toEqual(["Title", "Key", "Status", "Contributors"]);
    const body = within(table).getAllByRole("row")[1];
    expect(within(body).getByRole("link", { name: "Checkout flow" })).toHaveAttribute("href", "/node/feature/F-1");
    expect(within(body).getByText("pay/checkout").className).toContain("mono");
    expect(within(body).getByText("In progress")).toBeInTheDocument();
    expect(within(body).getByText("JC Grubbs")).toBeInTheDocument();
    expect(table).not.toHaveTextContent("Contributors:");
    expect(table).not.toHaveTextContent("Owner:");
  });

  it("uses an Owner column for a task and shows the owner's name", async () => {
    const row: ListRow = {
      id: "T-1", key: "pay/checkout/a", title: "Wire the form", status: "ready", ref: "T-1",
      owner: "jc", owner_name: "JC Grubbs",
    } as ListRow;
    stub({ "/api/ls?noun=tasks": list("task", [row]) });
    renderFilter("tasks");
    const table = await screen.findByRole("table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent))
      .toEqual(["Title", "Key", "Status", "Owner"]);
    expect(within(table).getByText("JC Grubbs")).toBeInTheDocument();
    expect(table).not.toHaveTextContent("Owner:");
  });

  it("renders no people column for amendments, which have neither axis", async () => {
    const row: ListRow = { id: "A-1", key: "a/one", title: "Widen scope", status: "proposed", ref: "A-1" } as ListRow;
    stub({ "/api/ls?noun=amendments": list("amendment", [row]) });
    renderFilter("amendments");
    const table = await screen.findByRole("table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Title", "Key", "Status"]);
    expect(within(within(table).getAllByRole("row")[1]).getAllByRole("cell")).toHaveLength(3);
    expect(table).not.toHaveTextContent("Owner");
  });

  it("opens a status combobox on focus showing words, and a typed wire word still finds the option", async () => {
    const result = list("task", []);
    result.facets = [
      { axis: "status", options: [{ value: "in_review", label: "in_review", count: 2 }] },
      { axis: "owner", options: [] },
    ] as unknown as ListResult["facets"];
    stub({ "/api/ls?noun=tasks": result });
    renderFilter("tasks");
    expect(await screen.findByText("No matches.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    const status = screen.getByRole("combobox", { name: "status" });
    fireEvent.focus(status);
    expect(status).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("option", { name: /In review/ })).toBeInTheDocument();
    fireEvent.change(status, { target: { value: "in_review" } });
    expect(screen.getByRole("option", { name: /In review/ })).toBeInTheDocument();
  });
});
