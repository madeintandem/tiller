// A reread (the topbar refresh, or the window regaining focus) keeps the
// shell and Home on screen until the new answers land: no "Home Loading..."
// over the desk, no lost tab, no dropped #hash, no second scroll to the
// section the reader arrived at.

import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DriftReportView, ListResult } from "../../api";
import { getFetchesInFlight } from "../../api";
import { expectNoLoading, homeRoutes, renderApp, stubReread, summary, watchLoading, settleReads } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const drift = (title: string): DriftReportView => ({
  compared: 5,
  candidates: {
    missing_surface: [{ marker: "missing_surface", noun: "task", id: "T-1", title, detail: "gone" }],
  },
  uncovered: [],
});

function epics(n: number): ListResult {
  return {
    noun: "epic",
    count: n,
    nodes: Array.from({ length: n }, (_, i) => ({ id: `E-${i}`, ref: `E-${i}`, title: `Epic ${i}`, status: "active" })),
  };
}

describe("The reread harness", () => {
  it("lets a held read the page abandons leave the in-flight count", async () => {
    const stub = stubReread(homeRoutes());
    const { unmount } = renderApp("/");
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();

    stub.reread();
    expect(getFetchesInFlight()).toBeGreaterThan(0);
    unmount();

    await waitFor(() => expect(getFetchesInFlight()).toBe(0));
  });
});

describe("The shell and Home while the page rereads", () => {
  it("keeps the topbar's project and the desk on screen, then updates both in place", async () => {
    const stub = stubReread({ ...homeRoutes(), "/api/ls?noun=epic": epics(1) });
    renderApp("/");
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    const header = screen.getByRole("banner");
    expect(within(header).getByText("acme")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /^1 epic,/ })).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(stub.heldUrls()).toContain("/api/status");

    expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    expect(within(header).getByText("acme")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^1 epic,/ })).toBeInTheDocument();

    const renamed = summary("acme-two");
    renamed.project.name = "Acme Platform Two";
    await stub.release({ "/api/status": renamed, "/api/ls?noun=epic": epics(2) });
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform Two" })).toBeInTheDocument();
    expect(within(header).getByText("acme-two")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^2 epics,/ })).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the active tab, the #hash, and the reader's scroll on the Drift tab", async () => {
    const scrolls = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { configurable: true, value: scrolls });
    const stub = stubReread({ ...homeRoutes(), "/api/drift": drift("Legacy calc") });
    renderApp("/?tab=drift#missing_surface");
    expect(await screen.findByText("Legacy calc")).toBeInTheDocument();
    const rail = screen.getByRole("tablist", { name: "Home view" });
    const tab = () => within(rail).getByRole("tab", { name: /^Drift/ });
    expect(tab()).toHaveAttribute("aria-selected", "true");
    const drifted = scrolls.mock.calls.length;
    expect(drifted).toBeGreaterThan(0);

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("Legacy calc")).toBeInTheDocument();
    expect(tab()).toHaveAttribute("aria-selected", "true");

    await stub.release({ "/api/drift": drift("Legacy calc, renamed") });
    expect(await screen.findByText("Legacy calc, renamed")).toBeInTheDocument();
    expect(tab()).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("where")).toHaveTextContent("/?tab=drift#missing_surface");
    expect(scrolls.mock.calls.length).toBe(drifted);
    expectNoLoading(watch);
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  });

  it("keeps the desk and says it is out of date when the reread fails", async () => {
    const stub = stubReread({ ...homeRoutes(), "/api/ls?noun=epic": epics(1) });
    renderApp("/");
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^1 epic,/ })).toBeInTheDocument();
    expect(screen.getAllByText(/out of date/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Out of date: GET \/api\/status failed/)).toBeInTheDocument();
  });
});
