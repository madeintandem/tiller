// Build fixes from the browser walk of batches D to F: a card's text never
// runs under its expand caret, an all-empty board says so in every column the
// way Fix does, the rail's mix bar carries its words as a hover title, and a
// cancelled task has its own glyph shape and a muted title.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildEpic, BuildFeature, RelatedNode } from "../../api";
import StateGlyph from "../StateGlyph";
import buildCss from "../../views/build.css?raw";
import Build from "../../views/Build";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      const url = String(input);
      if (url in routes) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
      }
      return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
    }),
  );
}

function task(id: string, status: string, title: string): RelatedNode {
  return { noun: "task", id, key: `intake/rules/${id}`, ref: `${id} (${id})`, status, title };
}

function feature(overrides: Partial<BuildFeature> & Pick<BuildFeature, "id" | "status" | "title">): BuildFeature {
  return {
    ref: `${overrides.title} (${overrides.id})`,
    open_work_items: 0,
    task_done: 0,
    task_total: 0,
    tasks: [],
    contributors: [],
    column: overrides.status,
    ...overrides,
  };
}

function epic(id: string, key: string, title: string, features: BuildFeature[], counts?: Record<string, number>): BuildEpic {
  return {
    noun: "epic",
    id,
    key,
    title,
    ref: `${key} (${id})`,
    status: "active",
    contributors: [],
    feature_counts: counts ?? (features.length > 0 ? { draft: features.length } : undefined),
    features,
  };
}

function renderBoard(epics: BuildEpic[], entry: string) {
  const payload: BuildBoard = { epics };
  stubFetch({
    "/api/build": payload,
    "/api/status": {
      project: { name: "Acme", key: "acme", kind: "greenfield" },
      contributors: [],
      counts: { epics: epics.length, features: 0, tasks: 0, questions: 0 },
      whose_move: { human: 0, agent: 0 },
      active_epics: epics.map((e) => e.key),
    },
    "/api/queue": { entries: [] },
    "/api/drift": { compared: 0 },
  });
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Build />
    </MemoryRouter>,
  );
}

const withTasks = feature({
  id: "F-1",
  status: "ready",
  title: "Routing layer with a deliberately long title",
  tasks: [task("T-1", "ready", "Match paths"), task("T-2", "cancelled", "Legacy router"), task("T-3", "draft", "Sketch")],
});

describe("Build card caret", () => {
  it("marks a card that draws the caret, so its text reserves the caret's width", async () => {
    const { container } = renderBoard([epic("E-1", "intake", "Intake", [withTasks, feature({ id: "F-2", status: "draft", title: "No tasks" })])], "/build?epics=intake");
    await screen.findByText("Routing layer with a deliberately long title");
    const cards = Array.from(container.querySelectorAll(".fcard"));
    const expandable = cards.filter((c) => c.classList.contains("expandable"));
    expect(expandable).toHaveLength(1);
    expect(expandable[0].querySelector(".f-expand")).not.toBeNull();
  });

  it("pads the title and the epic eyebrow clear of the caret in build.css", () => {
    expect(buildCss).toMatch(/\.fcard\.expandable \.f-title,\s*\.fcard\.expandable \.f-epic\s*\{[^}]*padding-right:/);
  });
});

describe("Build empty board", () => {
  it("shows the (none) note in every column of an epic with no features, as Fix does", async () => {
    const { container } = renderBoard([epic("E-9", "ops", "Operations runbooks", [])], "/build?epics=ops");
    await screen.findByText(/Showing 0 features/);
    const columns = Array.from(container.querySelectorAll(".board-column"));
    expect(columns).toHaveLength(6);
    for (const column of columns) {
      expect(column.textContent).toBe("(none)");
    }
  });
});

describe("Build rail mix bar", () => {
  it("carries the mix in plain words as a hover title and accessible name", async () => {
    const counts = { in_review: 1, done: 6 };
    renderBoard([epic("E-1", "intake", "Intake", [withTasks], counts)], "/build?epics=intake");
    const bar = await screen.findByRole("img", { name: "1 in review · 6 done" });
    expect(bar.closest(".mixbar")).toHaveAttribute("title", "1 in review · 6 done");
  });
});

describe("Build cancelled task", () => {
  it("draws a cancelled task with its own glyph shape and a muted row", async () => {
    const { container } = renderBoard([epic("E-1", "intake", "Intake", [withTasks])], "/build?epics=intake");
    await screen.findByText("Routing layer with a deliberately long title");
    fireEvent.click(screen.getByRole("button", { name: "Expand tasks" }));
    const cancelled = screen.getByText("Legacy router").closest(".trow") as HTMLElement;
    expect(cancelled).toHaveClass("is-cancelled");
    expect(cancelled.querySelector(".state-glyph")).toHaveAttribute("data-shape", "cancelled");
    const draft = screen.getByText("Sketch").closest(".trow") as HTMLElement;
    expect(draft).not.toHaveClass("is-cancelled");
    expect(draft.querySelector(".state-glyph")).toHaveAttribute("data-shape", "draft");
    expect(container.querySelector(".is-cancelled .state-glyph")).toHaveAttribute("aria-label", "Cancelled");
  });
});

describe("StateGlyph cancelled shape", () => {
  it("draws cancelled as its own shape, not the draft ring", () => {
    const { container } = render(
      <>
        <StateGlyph status="cancelled" />
        <StateGlyph status="draft" />
      </>,
    );
    const [cancelled, draft] = Array.from(container.querySelectorAll("svg"));
    expect(cancelled).toHaveAttribute("data-shape", "cancelled");
    expect(cancelled.innerHTML).not.toBe(draft.innerHTML);
  });
});
