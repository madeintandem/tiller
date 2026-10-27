// Vitest/RTL coverage for the Agent use tab's headline figures (Task 11 of
// the agent use report plan): the coverage note, the five tiles, and the two
// weekly token charts, including partial and unmeasured weeks.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { UsageReport } from "../../api";
import AgentUseTab from "../../views/report/AgentUseTab";
import { formatTokens } from "../../views/report/format";
import { logScale } from "../../views/report/scale";

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

  it("totals the fresh tokens in the tooltip and stripes the partial legend swatch", async () => {
    await renderTab(usage({ coverage: ["none", "partial", "full", "full"] }));
    const fresh = chart("Fresh input, cache writes and output");
    fireEvent.pointerMove(fresh.querySelector('[data-week="2"]')!);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Total");
    expect(screen.getByRole("tooltip")).toHaveTextContent("Total 63.0M");
    expect(fresh.querySelector("[data-swatch='partial'] line.report-hatch")).not.toBeNull();
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

const STAGES = [
  ["intake", "Intake"],
  ["spec", "Spec"],
  ["plan", "Plan"],
  ["execute", "Execute"],
  ["checks", "Checks"],
  ["review", "Review"],
  ["fix", "Fix"],
  ["release", "Release"],
  ["other", "Other work"],
] as const;

function stageUsage(): UsageReport {
  const totals = [0, 100, 200, 4000, 1000, 500, 0, 200, 0];
  const reads = [0, 90, 190, 3800, 949, 450, 0, 100, 0];
  return usage({
    stages: STAGES.map(([key, label], i) => ({ key, label, total: totals[i], cache_read: reads[i] })),
    models: ["Opus", "Sonnet", "Haiku", "GPT", "Other"],
    model_stages: STAGES.map(([key], i) => ({
      stage: key,
      totals: [0.5, 0.3, 0.15, 0.04, 0.01].map((share) => totals[i] * share),
    })),
  });
}

function stageRow(card: HTMLElement, key: string): Element {
  return card.querySelector(`g[data-stage='${key}']`)!;
}

describe("Agent use stage and model charts", () => {
  it("lists every stage with its cache-read share", async () => {
    await renderTab(stageUsage());
    const card = chart("Tokens by stage");
    expect([...card.querySelectorAll(".report-row-title")].map((t) => t.textContent)).toEqual(STAGES.map(([, l]) => l));
    const checks = within(stageRow(card, "checks") as HTMLElement);
    expect(checks.getByText("94.9%")).toBeInTheDocument();
    expect(checks.getByText(formatTokens(1000))).toBeInTheDocument();
    // A stage with no tokens is an empty track labeled 0.
    const intake = stageRow(card, "intake");
    expect(intake.querySelector(".report-track")).not.toBeNull();
    expect(within(intake as HTMLElement).getByText("0")).toBeInTheDocument();
  });

  it("labels wide model segments and names every model in the legend", async () => {
    await renderTab(stageUsage());
    const card = chart("Tokens by stage and model");
    expect([...card.querySelectorAll(".report-legend li")].map((li) => li.textContent)).toEqual(["Opus", "Sonnet", "Haiku", "GPT", "Other"]);
    // Zero-token stages are omitted.
    expect(card.querySelectorAll("g[data-stage]")).toHaveLength(6);
    // 50, 30 and 15 percent clear the 12 percent floor; 4 and 1 do not.
    const labels = [...stageRow(card, "execute").querySelectorAll(".report-seg-label")].map((t) => t.textContent);
    expect(labels).toEqual(["50%", "30%", "15%"]);
    fireEvent.pointerMove(stageRow(card, "execute"));
    const tip = screen.getByRole("tooltip");
    expect(tip).toHaveTextContent("Execute");
    expect(tip).toHaveTextContent("Opus 50%");
    expect(tip).toHaveTextContent("GPT 4%");
  });

  it("keeps a model's color the same across charts", async () => {
    await renderTab(stageUsage());
    const card = chart("Tokens by stage and model");
    const swatch = card.querySelectorAll(".report-legend li")[1].querySelector("rect")!;
    const segment = stageRow(card, "execute").querySelectorAll("rect:not(.report-hit)")[1];
    expect(swatch.getAttribute("style")).toContain("var(--st-in_review)");
    expect(segment.getAttribute("style")).toBe(swatch.getAttribute("style"));
  });

  it("inks labels on the gold and gray fills apart from the panel ink", async () => {
    await renderTab(stageUsage());
    const row = stageRow(chart("Tokens by stage and model"), "execute");
    const [opus, sonnet] = [...row.querySelectorAll(".report-seg-label")];
    expect(opus.getAttribute("class")).toBe("report-seg-label");
    expect(sonnet.getAttribute("class")).toContain("report-seg-label--on-gold");
    const other = stageUsage();
    other.model_stages = other.model_stages.map((r) => ({ ...r, totals: [0, 0, 0, 0, r.totals.reduce((a, b) => a + b, 0)] }));
    cleanup();
    await renderTab(other);
    const gray = stageRow(chart("Tokens by stage and model"), "execute").querySelector(".report-seg-label")!;
    expect(gray.getAttribute("class")).toContain("report-seg-label--on-gray");
  });

  it("draws no NaN and says so without stage data", async () => {
    const { container } = await renderTab(usage());
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
    expect(screen.getAllByText("No stage usage measured yet")).toHaveLength(2);
  });
});

const FEATURES = [
  { title: "Search results", done: "2026-10-07T12:00:00Z", total: 12_000_000, median: 12_000_000 },
  { title: "Export to CSV", done: "2026-10-14T12:00:00Z", total: 240_000_000, median: 126_000_000 },
  { title: "Saved filters", done: "2026-10-21T12:00:00Z", total: 3_000_000, median: 12_000_000 },
];

const BANDS = ["< 1M", "1-3M", "3-10M", "10-30M", "30-100M", "100-300M", "300M-1B", "1B+"].map((label, i) => ({
  label,
  count: i === 3 ? 5 : i % 3,
}));

describe("Agent use feature and work-item charts", () => {
  it("plots features on a labeled log axis", async () => {
    const { container } = await renderTab(usage({ features: FEATURES }));
    const card = chart("Tokens per finished feature");
    expect(within(card).getByText("Tokens (log scale)")).toBeInTheDocument();
    expect(card.querySelectorAll("circle[data-feature]")).toHaveLength(3);
    expect(within(card).getByText("10.0M")).toBeInTheDocument();
    expect(within(card).getByText("1.0M")).toBeInTheDocument();
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
  });

  it("names the feature and its total on hover", async () => {
    await renderTab(usage({ features: FEATURES }));
    const dot = chart("Tokens per finished feature").querySelector("circle[data-feature]")!;
    fireEvent.pointerMove(dot.closest("g")!, { clientX: 10, clientY: 10 });
    expect(await screen.findByText("Search results")).toBeInTheDocument();
    expect(screen.getByText("12.0M")).toBeInTheDocument();
  });

  it("notes features left out", async () => {
    await renderTab(usage({ features: FEATURES, features_left_out: 4 }));
    expect(within(chart("Tokens per finished feature")).getByText("4 earlier features not shown")).toBeInTheDocument();
  });

  it("draws one or no features without NaN", async () => {
    const one = await renderTab(usage({ features: [FEATURES[0]] }));
    expect(one.container.innerHTML).not.toMatch(/NaN|Infinity/);
    expect(chart("Tokens per finished feature").querySelectorAll("circle[data-feature]")).toHaveLength(1);
    cleanup();
    await renderTab(usage());
    expect(screen.getByText("No features completed yet")).toBeInTheDocument();
  });

  it("shows every band with its count", async () => {
    await renderTab(usage({ item_bands: BANDS, kpis: { ...usage().kpis, median_per_item: 8_900_000, items: 12 } }));
    const card = chart("Tokens per work item");
    const labels = [...card.querySelectorAll("[data-band] .report-tick")].map((t) => t.textContent);
    expect(labels).toEqual(BANDS.map((b) => b.label));
    expect(within(card).getByText("5")).toBeInTheDocument();
    expect(within(card).getByText("Median 8.9M · 12 items")).toBeInTheDocument();
  });

  it("notes items left out and items with no usage", async () => {
    await renderTab(usage({ item_bands: BANDS, items_left_out: 3, items_unmeasured: 2 }));
    expect(
      within(chart("Tokens per work item")).getByText("Median 8.9M · 12 items · 3 earlier items not shown · 2 items with no usage recorded"),
    ).toBeInTheDocument();
    cleanup();
    await renderTab(usage({ item_bands: BANDS, items_unmeasured: 1 }));
    expect(within(chart("Tokens per work item")).getByText("Median 8.9M · 12 items · 1 item with no usage recorded")).toBeInTheDocument();
  });

  it("says so without any finished work item", async () => {
    const { container } = await renderTab(usage());
    expect(screen.getByText("No work items finished yet")).toBeInTheDocument();
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
  });

  it("logScale maps decades evenly", () => {
    const scale = logScale([1e6, 1e8], [0, 100]);
    expect(scale(1e7)).toBe(50);
    expect(scale(0)).toBe(0);
  });

  it("keeps the earlier-features note under the empty feature message", async () => {
    await renderTab(usage({ features_left_out: 3 }));
    const card = chart("Tokens per finished feature");
    expect(within(card).getByText("No features completed yet")).toBeInTheDocument();
    expect(within(card).getByText("3 earlier features not shown")).toBeInTheDocument();
  });

  it("keeps the unmeasured note under the empty item message", async () => {
    await renderTab(usage({ items_unmeasured: 2, kpis: { ...usage().kpis, median_per_item: null, items: 0 } }));
    const card = chart("Tokens per work item");
    expect(within(card).getByText("No work items finished yet")).toBeInTheDocument();
    expect(within(card).getByText("2 items with no usage recorded")).toBeInTheDocument();
  });
});
