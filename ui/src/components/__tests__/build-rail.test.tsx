// The Build epic rail folds finished epics behind a disclosure, and the board
// toolbar's scroll toggle carries readable names.

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildEpic, ProjectSummary } from "../../api";
import Build from "../../views/Build";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

function epic(key: string, title: string, status: string): BuildEpic {
  return { noun: "epic", id: `E-${key}`, key, title, ref: `${key} (E-${key})`, status, contributors: [], features: [] };
}

function stub(epics: BuildEpic[], active: string[]) {
  const summary: ProjectSummary = {
    project: { name: "Acme", key: "acme", kind: "greenfield" },
    contributors: [],
    counts: { epics: epics.length, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
    active_epics: active,
  };
  const routes: Record<string, unknown> = {
    "/api/build": { epics } satisfies BuildBoard,
    "/api/status": summary,
    "/api/queue": { entries: [] },
    "/api/drift": { compared: 0 },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      const url = String(input);
      return Promise.resolve(
        url in routes
          ? { ok: true, json: () => Promise.resolve(routes[url]) }
          : { ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") },
      );
    }),
  );
}

function renderBuild(entry = "/build") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Build />
    </MemoryRouter>,
  );
}

const epics = [
  epic("alpha", "Alpha", "active"),
  epic("beta", "Beta", "ready"),
  epic("gamma", "Gamma", "done"),
  epic("delta", "Delta", "archived"),
];

describe("scroll toggle", () => {
  it("names the group Scrolling and each choice in words with a title", async () => {
    stub(epics, ["alpha"]);
    renderBuild();
    const group = await screen.findByRole("group", { name: "Scrolling" });
    const whole = within(group).getByRole("button", { name: "Whole board" });
    const each = within(group).getByRole("button", { name: "Each column" });
    expect(whole).toHaveAttribute("title", "The board scrolls as one page");
    expect(each).toHaveAttribute("title", "Each column scrolls on its own");
    expect(whole).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(each);
    expect(each).toHaveAttribute("aria-pressed", "true");
    expect(window.localStorage.getItem("tiller-board-scroll")).toBe("columns");
  });
});

describe("finished epics in the rail", () => {
  it("lists live epics, then a closed disclosure counting the finished ones", async () => {
    stub(epics, ["alpha"]);
    renderBuild();
    const summary = await screen.findByText("2 finished");
    const details = summary.closest("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    const alpha = screen.getByRole("button", { name: /Alpha/ });
    const beta = screen.getByRole("button", { name: /Beta/ });
    expect(details.contains(alpha)).toBe(false);
    expect(details.contains(beta)).toBe(false);
    expect(details.contains(screen.getByRole("button", { name: /Gamma/, hidden: true }))).toBe(true);
    expect(details.contains(screen.getByRole("button", { name: /Delta/, hidden: true }))).toBe(true);
    // live epics come before the disclosure
    expect(alpha.compareDocumentPosition(details) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("starts open when a finished epic is selected, from the URL", async () => {
    stub(epics, ["alpha"]);
    renderBuild("/build?epics=gamma");
    const details = (await screen.findByText("2 finished")).closest("details") as HTMLDetailsElement;
    expect(details.open).toBe(true);
  });

  it("starts open when a finished epic is selected from stored state", async () => {
    stub(epics, ["alpha"]);
    window.localStorage.setItem("tiller-build-epics:acme", "delta");
    renderBuild();
    const summary = await screen.findByText("2 finished");
    await waitFor(() => {
      expect((summary.closest("details") as HTMLDetailsElement).open).toBe(true);
    });
  });

  it("is absent when no epic is finished", async () => {
    stub(epics.slice(0, 2), ["alpha"]);
    renderBuild();
    await screen.findByRole("button", { name: /Alpha/ });
    expect(screen.queryByText(/finished/)).toBeNull();
  });
});
