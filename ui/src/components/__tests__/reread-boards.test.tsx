// Build and Fix while the page rereads: the board, an expanded card, and the
// filters stay on screen until the new answer lands, and a failed reread
// keeps the board and says it is out of date.

import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildFeature, RelatedNode, WorkItemsBoard } from "../../api";
import { expectNoLoading, renderApp, stubReread, summary, watchLoading, type Routes } from "./rereadHarness";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function task(id: string, title: string): RelatedNode {
  return { noun: "task", id, key: `intake/rules/${id}`, ref: `${id} (${id})`, status: "active", title };
}

function buildBoard(taskTitle: string): BuildBoard {
  const feature: BuildFeature = {
    id: "F-1", title: "Schema engine", status: "active", column: "active", ref: "Schema engine (F-1)",
    open_work_items: 0, task_done: 0, task_total: 1, tasks: [task("T-1", taskTitle)], contributors: [],
  };
  return {
    epics: [{
      noun: "epic", id: "E-1", key: "form", title: "Form Engine", ref: "form (E-1)", status: "active",
      contributors: [], feature_counts: { active: 1 }, features: [feature],
    }],
  };
}

function buildRoutes(taskTitle: string): Routes {
  return {
    "/api/status": { ...summary(), active_epics: ["form"] },
    "/api/build": buildBoard(taskTitle),
    "/api/queue": { entries: [] },
    "/api/drift": { compared: 0 },
  };
}

function fixBoard(title: string): WorkItemsBoard {
  return {
    open: [{ noun: "bug", id: "B-1", key: "bugs/one", title, status: "ready", ref: "bugs/one (B-1)" }],
    closed: [],
    attachments: [],
  };
}

describe("Build while the page rereads", () => {
  it("keeps the board and an expanded card open, then updates the card in place", async () => {
    const stub = stubReread(buildRoutes("Parse the schema"));
    renderApp("/build");
    const card = () => (screen.getByText("Schema engine").closest(".fcard") as HTMLElement);
    expect(await screen.findByText("Schema engine")).toBeInTheDocument();
    fireEvent.click(within(card()).getByRole("button", { name: "Expand tasks" }));
    expect(within(card()).getByText("Parse the schema")).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(within(card()).getByText("Parse the schema")).toBeInTheDocument();

    await stub.release({ "/api/build": buildBoard("Parse the schema twice") });
    expect(await within(card()).findByText("Parse the schema twice")).toBeInTheDocument();
    expect(within(card()).getByRole("button", { name: "Collapse tasks" })).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/build$/);
    expectNoLoading(watch);
  });

  it("keeps the board and says it is out of date when the reread fails", async () => {
    const stub = stubReread(buildRoutes("Parse the schema"));
    renderApp("/build");
    expect(await screen.findByText("Schema engine")).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByText("Schema engine")).toBeInTheDocument();
    expect(screen.getByText(/board out of date/)).toBeInTheDocument();
  });
});

describe("Fix while the page rereads", () => {
  it("keeps the board on screen, then updates it in place", async () => {
    const stub = stubReread({ "/api/status": summary(), "/api/work-items": fixBoard("Zulu") });
    renderApp("/fix");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("Zulu")).toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: "Work item filters" })).toBeInTheDocument();

    await stub.release({ "/api/work-items": fixBoard("Zulu, renamed") });
    expect(await screen.findByText("Zulu, renamed")).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the board and says it is out of date when the reread fails", async () => {
    const stub = stubReread({ "/api/status": summary(), "/api/work-items": fixBoard("Zulu") });
    renderApp("/fix");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByText("Zulu")).toBeInTheDocument();
    expect(screen.getByText(/board out of date/)).toBeInTheDocument();
  });
});
