// History's milestone tier (R3): milestones by default, bookkeeping folded
// into one disclosure row, ?show=all for everything. lib/bursts.test.ts and
// lib/eventTier.test.ts own the rules; these pin what the page renders.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import History from "../../views/History";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
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

function LocationProbe() {
  const loc = useLocation();
  return <span data-testid="loc">{loc.pathname + loc.search}</span>;
}

function renderHistory(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/history" element={<History />} />
      </Routes>
      <LocationProbe />
    </MemoryRouter>,
  );
}

function entry(type: string, at: string, overrides: Partial<RecapEntry> = {}): RecapEntry {
  return { at, type, contributor: "jc", command: "x", ask: false, targets: [], ...overrides };
}

function recapOf(entries: RecapEntry[], overrides: Partial<Recap> = {}): Recap {
  const group: RecapGroup = {
    epic: { key: "audit", id: "E-1", ref: "audit (E-1)", title: "Audit" },
    stream: "jc",
    entries,
    omitted: 0,
  };
  return { since: "", window: "7d", reader: "jc", total: entries.length, groups: [group], ...overrides };
}

const T1 = "2026-07-24T10:03:00Z";
const T2 = "2026-07-24T10:02:00Z";
const T3 = "2026-07-24T10:01:00Z";
const T4 = "2026-07-24T10:00:00Z";

// Newest first: a milestone, then a run of three bookkeeping bursts.
const mixed = recapOf([
  entry("approve.feature", T1),
  entry("link.feature", T2),
  entry("link.feature", T2),
  entry("order.features", T3),
  entry("rename.title.task", T4),
]);

describe("History tiers", () => {
  it("defaults to milestones and folds a bookkeeping run into one disclosure row", async () => {
    stubFetch({ "/api/recap?window=7d": mixed });
    renderHistory("/history?window=7d");

    const run = await screen.findByRole("button", {
      name: "4 bookkeeping changes (linked, reordered, retitled)",
    });
    expect(run).toHaveAttribute("aria-expanded", "false");
    // The Kind select also lists "approved", so match the row's bold verb.
    expect(screen.getByText(/approved/, { selector: "b" })).toBeInTheDocument();
    expect(screen.queryByText(/features? linked/)).not.toBeInTheDocument();

    fireEvent.click(run);

    expect(run).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(/2 features linked/)).toBeInTheDocument();
    expect(screen.getByText("features reordered", { exact: false, selector: "b" })).toBeInTheDocument();
  });

  it("says 1 bookkeeping change in the singular", async () => {
    stubFetch({ "/api/recap?window=7d": recapOf([entry("approve.feature", T1), entry("link.feature", T2)]) });
    renderHistory("/history?window=7d");

    expect(await screen.findByRole("button", { name: "1 bookkeeping change (linked)" })).toBeInTheDocument();
  });

  it("offers Milestones and Everything, with Milestones active by default", async () => {
    stubFetch({ "/api/recap?window=7d": mixed });
    renderHistory("/history?window=7d");

    const seg = await screen.findByRole("group", { name: "History detail" });
    const milestones = within(seg).getByRole("link", { name: "Milestones" });
    const everything = within(seg).getByRole("link", { name: "Everything" });
    expect(milestones).toHaveAttribute("aria-current", "page");
    expect(everything).not.toHaveAttribute("aria-current");
    expect(everything).toHaveAttribute("href", "/history?window=7d&show=all");
    expect(milestones).toHaveAttribute("href", "/history?window=7d");
  });

  it("?show=all shows every burst row and no run row", async () => {
    stubFetch({ "/api/recap?window=7d": mixed });
    renderHistory("/history?window=7d&show=all");

    expect(await screen.findByText(/2 features linked/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /bookkeeping change/ })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Everything" })).toHaveAttribute("aria-current", "page");
  });

  it("keeps show=all across a window change", async () => {
    stubFetch({ "/api/recap?window=7d": mixed, "/api/recap?window=24h": mixed });
    renderHistory("/history?window=7d&show=all");

    const link = await screen.findByRole("link", { name: "24h" });
    expect(link).toHaveAttribute("href", "/history?window=24h&show=all");
    fireEvent.click(link);
    expect((await screen.findByTestId("loc")).textContent).toBe("/history?window=24h&show=all");
  });

  it("a window holding only bookkeeping shows its run row, not the empty state", async () => {
    stubFetch({ "/api/recap?window=7d": recapOf([entry("link.feature", T1), entry("order.tasks", T2)]) });
    renderHistory("/history?window=7d");

    expect(await screen.findByRole("button", { name: "2 bookkeeping changes (linked, reordered)" })).toBeInTheDocument();
    expect(screen.queryByText(/Nothing recorded/)).not.toBeInTheDocument();
  });

  it("the head count states every event in the window, bookkeeping included", async () => {
    stubFetch({ "/api/recap?window=7d": mixed });
    renderHistory("/history?window=7d");

    expect(await screen.findByText(/^5 events in the last 7 days/)).toBeInTheDocument();
  });

  it("never folds a run across a day heading", async () => {
    const recap = recapOf([
      entry("link.feature", "2026-07-24T10:00:00Z"),
      entry("order.tasks", "2026-07-21T10:00:00Z"),
    ]);
    stubFetch({ "/api/recap?window=7d": recap });
    renderHistory("/history?window=7d");

    expect(await screen.findByRole("button", { name: "1 bookkeeping change (linked)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1 bookkeeping change (reordered)" })).toBeInTheDocument();
  });

  it("keeps a bookkeeping burst that asked as a visible milestone row", async () => {
    const recap = recapOf([entry("link.feature", T1, { ask: true }), entry("order.tasks", T2)]);
    stubFetch({ "/api/recap?window=7d": recap });
    renderHistory("/history?window=7d");

    expect(await screen.findByText("asked you")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1 bookkeeping change (reordered)" })).toBeInTheDocument();
  });

  it("time order breaks a run where the epic changes, and a milestone breaks it too", async () => {
    const group = (key: string, entries: RecapEntry[]): RecapGroup => ({
      epic: { key, id: `E-${key}`, ref: `${key} (E-${key})`, title: key },
      stream: "jc",
      entries,
      omitted: 0,
    });
    const recap: Recap = {
      since: "",
      window: "7d",
      reader: "jc",
      total: 5,
      groups: [
        group("alpha", [
          entry("link.feature", "2026-07-24T10:05:00Z"),
          entry("approve.feature", "2026-07-24T10:03:00Z"),
          entry("link.task", "2026-07-24T10:01:00Z"),
        ]),
        group("beta", [entry("order.tasks", "2026-07-24T10:04:00Z"), entry("link.task", "2026-07-24T10:02:00Z")]),
      ],
    };
    stubFetch({ "/api/recap?window=7d": recap });
    const { container } = renderHistory("/history?window=7d&group=time");

    await screen.findByText(/^5 events in the last 7 days/);
    const rows = Array.from(container.querySelectorAll(".recap-burst"), (li) => {
      const run = li.querySelector(".recap-bookkeeping-toggle");
      return run !== null ? (run.textContent ?? "") : "milestone";
    });
    // alpha link | beta order | alpha approve | beta link | alpha link: five
    // rows, no run ever holds two groups' bursts.
    expect(rows).toEqual([
      expect.stringContaining("1 bookkeeping change (linked)"),
      expect.stringContaining("1 bookkeeping change (reordered)"),
      "milestone",
      expect.stringContaining("1 bookkeeping change (linked)"),
      expect.stringContaining("1 bookkeeping change (linked)"),
    ]);
  });

  it("folds in time mode too", async () => {
    stubFetch({ "/api/recap?window=7d": mixed });
    renderHistory("/history?window=7d&group=time");

    expect(
      await screen.findByRole("button", { name: "4 bookkeeping changes (linked, reordered, retitled)" }),
    ).toBeInTheDocument();
  });
});
