// Build's `?epics=all`: every showable epic for this visit, never written to
// the stored selection (the Explore empty pane's in-flight link uses it).

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BuildBoard, BuildEpic, BuildFeature, ProjectSummary } from "../../api";
import Build from "../../views/Build";
import { readSelectionKeys, selectionStorageKey } from "../../views/build/selectionStorage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

const summary: ProjectSummary = {
  project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
  contributors: [],
  counts: { epics: 2, features: 2, tasks: 0, questions: 0 },
  whose_move: { human: 0, agent: 0 },
  active_epics: ["intake"],
};

function feat(id: string, title: string): BuildFeature {
  return {
    id, title, status: "active", column: "active", ref: id, contributors: [],
    task_done: 0, task_total: 0, open_work_items: 0, tasks: [],
  } as unknown as BuildFeature;
}

function epic(id: string, key: string | undefined, title: string, features: BuildFeature[]): BuildEpic {
  return {
    noun: "epic", id, key, title, ref: id, status: "active", contributors: [],
    feature_counts: { active: features.length }, features,
  } as unknown as BuildEpic;
}

const board: BuildBoard = {
  epics: [
    epic("E-1", "intake", "Intake", [feat("F-1", "Income worksheet")]),
    epic("E-2", "ops", "Ops", [feat("F-2", "Billing sync")]),
    epic("E-3", undefined, "Keyless", [feat("F-3", "Never shown")]),
  ],
};

function stub() {
  const routes: Record<string, unknown> = {
    "/api/build": board, "/api/status": summary, "/api/queue": { entries: [] }, "/api/drift": { compared: 0 },
  };
  vi.stubGlobal("fetch", vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  }));
}

function Probe() {
  const location = useLocation();
  return <span data-testid="loc">{location.pathname + location.search}</span>;
}

function renderAt(entry: string) {
  return render(<MemoryRouter initialEntries={[entry]}><Build /><Probe /></MemoryRouter>);
}

const stored = () => window.localStorage.getItem(selectionStorageKey("acme"));

describe("Build ?epics=all", () => {
  it("shows every showable epic and leaves a narrower stored selection untouched", async () => {
    stub();
    window.localStorage.setItem(selectionStorageKey("acme"), "intake");
    renderAt("/build?epics=all");
    expect(await screen.findByText("Income worksheet")).toBeInTheDocument();
    expect(screen.getByText("Billing sync")).toBeInTheDocument();
    expect(screen.queryByText("Never shown")).not.toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(stored()).toBe("intake");
    expect(screen.getByTestId("loc")).toHaveTextContent("/build?epics=all");
  });

  it("restores the narrower selection on a later bare visit", async () => {
    stub();
    window.localStorage.setItem(selectionStorageKey("acme"), "intake");
    const first = renderAt("/build?epics=all");
    await screen.findByText("Billing sync");
    first.unmount();
    renderAt("/build");
    await waitFor(() => expect(screen.getByTestId("loc")).toHaveTextContent("/build?epics=intake"));
    expect(await screen.findByText("Income worksheet")).toBeInTheDocument();
    expect(screen.queryByText("Billing sync")).not.toBeInTheDocument();
  });

  it("follows the reader's own change: the special value leaves the URL and storage follows", async () => {
    stub();
    window.localStorage.setItem(selectionStorageKey("acme"), "intake");
    renderAt("/build?epics=all");
    await screen.findByText("Billing sync");
    fireEvent.click(screen.getByRole("button", { name: /^Ops/ }));
    await waitFor(() => expect(screen.getByTestId("loc")).toHaveTextContent("/build?epics=intake"));
    await waitFor(() => expect(readSelectionKeys("acme")).toEqual(["intake"]));
    expect(screen.queryByText("Billing sync")).not.toBeInTheDocument();
  });
});
