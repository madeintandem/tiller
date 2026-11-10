// History's row anatomy under day headings: a day heading above every day's
// rows, then per row the clock (.t), the verb (.verb), the title (.what) and
// the actor (.who). No row repeats the date; the heading carries it.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry, RecapGroup, TargetRef } from "../../api";
import { PeopleProvider } from "../../people";
import History from "../../views/History";

beforeEach(() => {
  // Only Date is faked, so fetch promises and testing-library timers still run.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 10, 10, 9, 0));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubRecap(recap: Recap) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      const url = String(input);
      if (url.startsWith("/api/recap")) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(recap) });
      }
      return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
    }),
  );
}

function renderHistory(url: string) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <PeopleProvider contributors={[{ id: "jc", name: "jc" }]}>
        <Routes>
          <Route path="/history" element={<History />} />
        </Routes>
      </PeopleProvider>
    </MemoryRouter>,
  );
}

const at = (y: number, mo: number, d: number, h: number, mi: number) => new Date(y, mo, d, h, mi).toISOString();

function featureTarget(n: number): TargetRef {
  return { noun: "feature", id: `F-${n}`, ref: `feature-${n} (F-${n})`, key: `audit/feature-${n}`, title: `Feature ${n}` };
}

function entry(type: string, when: string, targets: TargetRef[]): RecapEntry {
  return { at: when, type, contributor: "jc", command: "x", ask: false, targets };
}

function group(entries: RecapEntry[]): RecapGroup {
  return {
    epic: { key: "audit", id: "E-1", title: "Audit Engine", ref: "audit (E-1)" },
    stream: "jc",
    omitted: 0,
    entries,
  };
}

function recapOf(groups: RecapGroup[]): Recap {
  return { since: "", window: "7d", reader: "jc", total: 100, groups };
}

const days = () => Array.from(document.querySelectorAll(".day-h .eyebrow")).map((e) => e.textContent);
const firstClasses = (row: Element) => Array.from(row.children).map((c) => c.className.split(" ")[0]);

describe("History rows: day headings and time-only rows", () => {
  it("renders a day heading for a group that holds one day", async () => {
    stubRecap(recapOf([group([entry("spec.feature", at(2026, 10, 9, 14, 6), [featureTarget(1)])])]));
    renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");
    expect(days()).toEqual(["Monday · Nov 9"]);
  });

  it("adds the year to a heading from another year", async () => {
    stubRecap(recapOf([group([entry("spec.feature", at(2025, 10, 9, 14, 6), [featureTarget(1)])])]));
    renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");
    expect(days()).toEqual(["Sunday · Nov 9, 2025"]);
  });

  it("orders a lone row's children .t, .verb, .what, .who, then the empty toggle column, with the clock alone in .t", async () => {
    stubRecap(recapOf([group([entry("spec.feature", at(2026, 10, 9, 14, 6), [featureTarget(1)])])]));
    const { container } = renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");

    const row = container.querySelector(".recap-burst-lone") as HTMLElement;
    expect(firstClasses(row)).toEqual(["t", "verb", "what", "who", "burst-toggle"]);
    expect(row.querySelector(".t")?.textContent).toBe("2:06 PM");
    expect(row.querySelector(".verb")?.textContent).toBe("specced");
    const what = row.querySelector(".what") as HTMLElement;
    expect(within(what).getByRole("link", { name: "Feature 1" })).toBeInTheDocument();
    expect(within(what).getByText("feature-1")).toHaveClass("mono");
    expect(row.querySelector(".who")?.textContent).toBe("jc");
  });

  it("puts the absolute time on hover for every row's .t", async () => {
    stubRecap(recapOf([group([entry("spec.feature", at(2026, 10, 9, 14, 6), [featureTarget(1)])])]));
    const { container } = renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");

    const time = container.querySelector(".recap-burst-lone .t time") as HTMLElement;
    expect(time.getAttribute("title")).toBe("Nov 9, 2026, 2:06:00 PM");
  });

  it("reads a run as .t then one label spanning the verb and title columns", async () => {
    const entries = [4, 3, 2, 1].map((n) => entry("spec.feature", at(2026, 10, 9, 14, 6 + n), [featureTarget(n)]));
    stubRecap(recapOf([group(entries)]));
    const { container } = renderHistory("/history?window=7d&show=all");
    await screen.findByText("4 features specced");

    const row = container.querySelector("summary.hrow") as HTMLElement;
    const kids = Array.from(row.children);
    expect(kids[0]).toHaveClass("t");
    expect(kids[1]).toHaveClass("what", "what-run");
    expect(kids[1].textContent).toBe("4 features specced");
    expect(row.querySelector(".verb")).toBeNull();
    // The run's clock is its latest event (entries arrive newest first).
    expect(kids[0].textContent).toBe("2:10 PM");
  });

  it("repeats no date on any row", async () => {
    stubRecap(recapOf([group([entry("spec.feature", at(2026, 10, 9, 14, 6), [featureTarget(1)])])]));
    const { container } = renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");
    for (const row of Array.from(container.querySelectorAll(".hrow"))) {
      expect(row.textContent).not.toContain("Nov 9");
    }
  });

  it("keeps a long verb's text fully present in its cell", async () => {
    stubRecap(recapOf([group([entry("recover_review.task", at(2026, 10, 9, 14, 6), [featureTarget(1)])])]));
    const { container } = renderHistory("/history?window=7d&show=all");
    await screen.findByText("Feature 1");
    expect(container.querySelector(".recap-burst-lone .verb")?.textContent).toBe("review recovered");
  });
});

describe("History rows: the bookkeeping fold lines up with ordinary rows", () => {
  function bookkeepingRecap(): Recap {
    const links = [3, 2, 1].map((n) => entry("link.feature", at(2026, 10, 9, 14, 6 + n), [featureTarget(n)]));
    return recapOf([group([...links, entry("spec.feature", at(2026, 10, 9, 13, 0), [featureTarget(9)])])]);
  }

  it("gives the fold row an empty .t and a spanning .what in epic view", async () => {
    stubRecap(bookkeepingRecap());
    const { container } = renderHistory("/history?window=7d");
    await screen.findByText(/bookkeeping change/);

    const row = container.querySelector(".recap-bookkeeping-toggle") as HTMLElement;
    expect(row.children[0]).toHaveClass("t");
    expect(row.children[0].textContent).toBe("");
    expect(row.children[1]).toHaveClass("what", "what-run");
  });

  it("gives the fold row the same columns in time view", async () => {
    stubRecap(bookkeepingRecap());
    const { container } = renderHistory("/history?window=7d&group=time");
    await screen.findByText(/bookkeeping change/);

    const row = container.querySelector(".recap-bookkeeping-toggle") as HTMLElement;
    expect(row.children[0]).toHaveClass("t");
    expect(row.children[1]).toHaveClass("what-run");
    expect(days()).toEqual(["Monday · Nov 9"]);
  });
});

describe("History rows: time view", () => {
  it("renders a heading for each distinct day and no date on its rows", async () => {
    stubRecap(
      recapOf([
        group([
          entry("spec.feature", at(2026, 10, 9, 14, 6), [featureTarget(1)]),
          entry("approve.feature", at(2026, 10, 8, 9, 0), [featureTarget(2)]),
        ]),
      ]),
    );
    renderHistory("/history?window=7d&show=all&group=time");
    await screen.findByText("Feature 1");
    expect(days()).toEqual(["Monday · Nov 9", "Sunday · Nov 8"]);
  });
});
