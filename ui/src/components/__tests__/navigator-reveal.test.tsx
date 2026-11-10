// The navigator reveals the current node: arriving on a node page scrolls
// its row into view once, after Explore's autoOpen has opened the ancestors,
// and never again until the selection itself changes.

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ListRow, ProjectSummary, TreeModel, TreeNode } from "../../api";
import App from "../../App";

const scrollIntoView = vi.fn();
const originalScrollIntoView = Element.prototype.scrollIntoView;

beforeEach(() => {
  scrollIntoView.mockReset();
  Element.prototype.scrollIntoView = scrollIntoView;
});

afterEach(() => {
  Element.prototype.scrollIntoView = originalScrollIntoView;
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

function node(noun: string, overrides: Partial<TreeNode>): TreeNode {
  return { noun, id: "x", status: "ready", path: "", files: [], children: [], ...overrides };
}

function tree(): TreeModel {
  return {
    project: "acme",
    nodes: [
      node("epic", {
        id: "E-1",
        key: "foundation",
        title: "Foundation",
        children: [
          node("feature", {
            id: "F-1",
            key: "foundation/skeleton",
            title: "App Skeleton",
            children: [node("task", { id: "T-9", key: "foundation/skeleton/boot", title: "Boot the app" })],
          }),
          node("feature", { id: "F-2", key: "foundation/identity", title: "Identity Roles", status: "specced" }),
        ],
      }),
      node("epic", {
        id: "E-2",
        key: "ops",
        title: "Ops",
        children: [node("feature", { id: "F-3", key: "ops/first", title: "First Ops Feature" })],
      }),
    ],
  };
}

function list(noun: string, rows: ListRow[] = []): ListResult {
  return { noun, count: rows.length, nodes: rows };
}

const summary: ProjectSummary = {
  project: { name: "Acme", key: "acme", kind: "greenfield" },
  contributors: [{ id: "jc", name: "JC" }],
  counts: { epics: 2, features: 3, tasks: 1, questions: 0 },
  whose_move: { human: 0, agent: 0 },
};

const bug: ListRow = { id: "B-a", key: "bugs/a", title: "Blank on reload", status: "ready", ref: "bugs/a (B-a)" };

function renderAt(route: string) {
  const routes: Record<string, unknown> = {
    "/api/status": summary,
    "/api/tree": tree(),
    "/api/ls?noun=question": list("question"),
    "/api/ls?noun=bug": list("bug", [bug]),
    "/api/ls?noun=chore": list("chore"),
    "/api/ls?noun=tweak": list("tweak"),
  };
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
  return render(
    <MemoryRouter initialEntries={[route]}>
      <App />
    </MemoryRouter>,
  );
}

function currentLink(): Element {
  const nav = screen.getByRole("navigation", { name: "Definition navigator" });
  const el = nav.querySelector('[aria-current="page"]');
  expect(el).not.toBeNull();
  return el as Element;
}

describe("Navigator reveal", () => {
  it("scrolls the current row into view once on arrival, not on chevron clicks, and again on a new selection", async () => {
    renderAt("/node/feature/F-2");
    const nav = within(await screen.findByRole("navigation", { name: "Definition navigator" }));
    await nav.findByRole("link", { name: "Identity Roles" });
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
    expect(scrollIntoView.mock.contexts[0]).toBe(currentLink());

    fireEvent.click(nav.getByRole("button", { name: "Expand Ops" }));
    await nav.findByRole("link", { name: "First Ops Feature" });
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    fireEvent.click(nav.getByRole("button", { name: "Expand App Skeleton" }));
    fireEvent.click(await nav.findByRole("link", { name: "Boot the app" }));
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(2);
    });
    expect(scrollIntoView.mock.contexts[1]).toBe(currentLink());
    expect(currentLink()).toHaveTextContent("Boot the app");
  });

  it("reveals a bug's shelf row", async () => {
    renderAt("/node/bug/B-a");
    await screen.findByRole("button", { name: "Collapse Bugs" });
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });
    expect(scrollIntoView.mock.contexts[0]).toBe(currentLink());
    expect(currentLink()).toHaveTextContent("Blank on reload");
  });
});
