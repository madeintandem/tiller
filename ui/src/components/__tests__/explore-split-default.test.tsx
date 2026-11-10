// The navigator's default width follows the window until the reader chooses
// one: only a drag or a keyboard resize is stored, and a double-click reset
// clears the stored value so the window's default applies again.

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ProjectSummary, TreeModel } from "../../api";
import App from "../../App";
import { readNavWidth, SPLIT_STORAGE_KEY } from "../../views/explore/splitStorage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

const tree: TreeModel = {
  project: "acme",
  nodes: [{ noun: "epic", id: "E-1", key: "foundation", title: "Foundation", status: "ready", path: "", files: [], children: [] }],
};

function list(noun: string): ListResult {
  return { noun, count: 0, nodes: [] };
}

const summary: ProjectSummary = {
  project: { name: "Acme", key: "acme", kind: "greenfield" },
  contributors: [],
  counts: { epics: 1, features: 0, tasks: 0, questions: 0 },
  whose_move: { human: 0, agent: 0 },
};

async function renderExploreAt(viewportWidth: number) {
  vi.stubGlobal("innerWidth", viewportWidth);
  const routes: Record<string, unknown> = {
    "/api/status": summary,
    "/api/tree": tree,
    "/api/ls?noun=question": list("question"),
    "/api/ls?noun=bug": list("bug"),
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
  const view = render(
    <MemoryRouter initialEntries={["/explore"]}>
      <App />
    </MemoryRouter>,
  );
  const divider = await screen.findByRole("separator", { name: "Resize navigator" });
  // Let the persist effect run before reading storage.
  await act(async () => {
    await Promise.resolve();
  });
  return { ...view, divider };
}

describe("Explore: the navigator's default width is never stored on its own", () => {
  it.each([
    [1512, "300"],
    [1920, "360"],
  ])("at %ipx leaves storage empty and draws the %spx default", async (width, px) => {
    const { divider } = await renderExploreAt(width);
    expect(divider).toHaveAttribute("aria-valuenow", px);
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBeNull();
  });

  it("stores a keyboard resize, which then wins at any window width", async () => {
    const { divider } = await renderExploreAt(1512);
    fireEvent.keyDown(divider, { key: "ArrowRight" });
    await act(async () => {
      await Promise.resolve();
    });
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBe("316");
    expect(readNavWidth(1920)).toBe(316);
    expect(readNavWidth(1024)).toBe(316);
  });

  it("clears the stored width on a double-click reset, so the window's default applies again", async () => {
    const { divider } = await renderExploreAt(1920);
    fireEvent.keyDown(divider, { key: "ArrowLeft" });
    await act(async () => {
      await Promise.resolve();
    });
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBe("344");
    fireEvent.doubleClick(divider);
    await act(async () => {
      await Promise.resolve();
    });
    expect(divider).toHaveAttribute("aria-valuenow", "360");
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBeNull();
    expect(readNavWidth(1512)).toBe(300);
  });

  it("keeps a width stored before this build", async () => {
    window.localStorage.setItem(SPLIT_STORAGE_KEY, "420");
    const { divider } = await renderExploreAt(1512);
    expect(divider).toHaveAttribute("aria-valuenow", "420");
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBe("420");
  });
});
