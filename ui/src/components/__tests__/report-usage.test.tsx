// Vitest/RTL coverage for the Agent use tab's headline figures (Task 11 of
// the agent use report plan): the coverage note, the five tiles, and the two
// weekly token charts, including partial and unmeasured weeks.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { UsageReport } from "../../api";
import AgentUseTab from "../../views/report/AgentUseTab";
import { formatTokens } from "../../views/report/format";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];

function usage(overrides: Partial<UsageReport> = {}): UsageReport {
  return {
    weeks: WEEKS,
    week_boundaries: [...WEEKS, "2026-10-26"].map((d) => `${d}T00:00:00Z`),
    harnesses: [
      { harness: "claude-code", label: "Claude Code", since: "2026-09-30" },
      { harness: "pi", label: "Pi", since: "2026-09-01" },
    ],
    full_coverage: "2026-09-30",
    coverage: ["none", "full", "full", "full"],
    kpis: {
      total: 6_030_000_000,
      cache_read: 5_840_000_000,
      input: 100_000_000,
      cache_write: 80_000_000,
      output: 10_000_000,
      median_per_item: 8_900_000,
      items: 12,
      tied_share: 0.84,
    },
    weekly: [
      { input: 0, cache_write: 0, cache_read: 0, output: 0 },
      { input: 40_000_000, cache_write: 30_000_000, cache_read: 2_000_000_000, output: 4_000_000 },
      { input: 30_000_000, cache_write: 30_000_000, cache_read: 1_900_000_000, output: 3_000_000 },
      { input: 30_000_000, cache_write: 20_000_000, cache_read: 1_940_000_000, output: 3_000_000 },
    ],
    stages: [],
    models: [],
    model_stages: [],
    features: [],
    features_left_out: 0,
    item_bands: [],
    items_left_out: 0,
    items_unmeasured: 0,
    ...overrides,
  };
}

async function renderTab(payload: UsageReport) {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(payload) })),
  );
  const view = render(<AgentUseTab />);
  await screen.findByText(/^Measured:/);
  return view;
}

function chart(title: string): HTMLElement {
  return screen.getByRole("heading", { name: title }).closest("section")!;
}

describe("Agent use tab headline figures", () => {
  it("renders the coverage note with each harness and date", async () => {
    await renderTab(usage());
    expect(screen.getByText("Measured: Claude Code since Sep 30 · Pi since Sep 1")).toBeInTheDocument();
  });

  it("renders the five tiles with cache reads split out", async () => {
    await renderTab(usage());
    expect(screen.getAllByRole("term").map((t) => t.textContent)).toEqual([
      "Total tokens",
      "Cache reads",
      "Fresh input and cache writes",
      "Output",
      "Median per work item",
    ]);
    expect(screen.getByText("6.03B")).toBeInTheDocument();
    expect(screen.getByText("5.84B")).toBeInTheDocument();
    expect(screen.getByText("96.8% of all")).toBeInTheDocument();
    expect(screen.getByText("180M")).toBeInTheDocument();
    expect(screen.getByText("10.0M")).toBeInTheDocument();
    expect(screen.getByText("8.9M")).toBeInTheDocument();
    expect(screen.getByText("12 items · 84% of tokens tied to a work item")).toBeInTheDocument();
  });

  it("reads Not yet when there is no median", async () => {
    await renderTab(usage({ kpis: { ...usage().kpis, median_per_item: null, items: 0 } }));
    expect(screen.getByText("Not yet")).toBeInTheDocument();
  });

  it("marks partial weeks and adds the legend entry", async () => {
    const { unmount } = await renderTab(usage({ coverage: ["none", "partial", "full", "full"] }));
    const fresh = chart("Fresh input, cache writes and output");
    expect(within(fresh).getByText("Partly measured")).toBeInTheDocument();
    fireEvent.pointerMove(fresh.querySelector('[data-week="1"]')!);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Partly measured");
    unmount();

    await renderTab(usage());
    expect(screen.queryByText("Partly measured")).not.toBeInTheDocument();
  });

  it("draws no bar for an unmeasured week", async () => {
    await renderTab(usage());
    for (const title of ["Cache reads by week", "Fresh input, cache writes and output"]) {
      const svg = chart(title).querySelector(".report-plot svg")!;
      // Weeks 1 to 3 are measured; week 0 is not.
      expect(svg.querySelectorAll("[data-week-bar='0']")).toHaveLength(0);
      expect(svg.querySelectorAll("[data-week-bar='1']").length).toBeGreaterThan(0);
      fireEvent.pointerMove(svg.querySelector('[data-week="0"]')!);
      expect(screen.getByRole("tooltip")).toHaveTextContent("Not measured");
      fireEvent.pointerLeave(svg.querySelector('[data-week="0"]')!);
    }
  });

  it("stacks fresh input, cache writes and output with cache writes hatched", async () => {
    await renderTab(usage());
    const svg = chart("Fresh input, cache writes and output").querySelector(".report-plot svg")!;
    expect(svg.querySelector("[data-kind='cache_write']")!.getAttribute("fill")).toMatch(/^url\(#/);
    expect(svg.querySelector("[data-kind='input']")).not.toBeNull();
    expect(svg.querySelector("[data-kind='output']")).not.toBeNull();
  });

  it("formats token counts", () => {
    expect(formatTokens(6_030_000_000)).toBe("6.03B");
    expect(formatTokens(186_000_000)).toBe("186M");
    expect(formatTokens(8_900_000)).toBe("8.9M");
    expect(formatTokens(412_000)).toBe("412K");
    expect(formatTokens(950)).toBe("950");
    expect(formatTokens(0)).toBe("0");
  });

  it("draws no NaN for an empty payload", async () => {
    const empty = usage({
      weeks: [],
      week_boundaries: [],
      coverage: [],
      weekly: [],
      kpis: { total: 0, cache_read: 0, input: 0, cache_write: 0, output: 0, median_per_item: null, items: 0, tied_share: 0 },
    });
    const { container } = await renderTab(empty);
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
    expect(screen.getByText("0% of all")).toBeInTheDocument();
  });

  it("draws no NaN when every week is unmeasured", async () => {
    const { container } = await renderTab(
      usage({
        coverage: ["none", "none", "none", "none"],
        weekly: WEEKS.map(() => ({ input: 0, cache_write: 0, cache_read: 0, output: 0 })),
      }),
    );
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
  });
});
