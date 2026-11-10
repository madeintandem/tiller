// Home's Just happened rows follow History's row anatomy: clock (.t), verb
// (.verb), title (.what), who (.who). A run reads .t then one spanning label.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { Recap, RecapEntry } from "../../api";
import { PeopleProvider } from "../../people";
import { JustHappenedColumn } from "../../views/home/Pulse";

afterEach(() => {
  cleanup();
});

const target = (n: number) => ({
  noun: "feature" as const,
  id: `F-${n}`,
  ref: `feature-${n} (F-${n})`,
  key: `audit/feature-${n}`,
  title: `Feature ${n}`,
});

function entry(n: number, minute: number): RecapEntry {
  return {
    at: new Date(2026, 10, 9, 14, minute).toISOString(),
    type: "spec.feature",
    contributor: "jc",
    command: "x",
    ask: false,
    targets: [target(n)],
  };
}

function recapOf(entries: RecapEntry[]): Recap {
  return {
    since: "",
    window: "24h",
    reader: "jc",
    total: entries.length,
    groups: [{ epic: { key: "audit", id: "E-1", title: "Audit", ref: "audit (E-1)" }, stream: "jc", omitted: 0, entries }],
  };
}

function renderColumn(recap: Recap) {
  return render(
    <MemoryRouter>
      <PeopleProvider contributors={[{ id: "jc", name: "jc" }]}>
        <JustHappenedColumn state={{ data: recap, error: null, loading: false }} />
      </PeopleProvider>
    </MemoryRouter>,
  );
}

describe("Home Just happened rows", () => {
  it("orders a lone row .t, .verb, .what, .who with the clock and its hover time", () => {
    const { container } = renderColumn(recapOf([entry(1, 6)]));
    const row = container.querySelector(".prow") as HTMLElement;
    expect(Array.from(row.children).map((c) => c.className.split(" ")[0])).toEqual(["t", "verb", "what", "who"]);
    expect(row.querySelector(".t")?.textContent).toBe("2:06 PM");
    expect(row.querySelector(".t time")?.getAttribute("title")).toBe("Nov 9, 2026, 2:06:00 PM");
    expect(row.querySelector(".verb")?.textContent).toBe("specced");
    expect(screen.getByRole("link", { name: "Feature 1" })).toBeInTheDocument();
  });

  it("says no milestones, and keeps the Full history link, when the window held only bookkeeping", () => {
    const claims = [entry(1, 6), entry(2, 7)].map((e) => ({ ...e, type: "claim.task" }));
    renderColumn(recapOf(claims));
    expect(screen.getByText("No milestones in the last 24 hours.")).toBeInTheDocument();
    expect(screen.queryByText("Nothing in the last 24 hours.")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Full history →" })).toHaveAttribute("href", "/history?window=24h");
  });

  it("says nothing happened, with no link, when the window held no events at all", () => {
    renderColumn(recapOf([]));
    expect(screen.getByText("Nothing in the last 24 hours.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Full history →" })).not.toBeInTheDocument();
  });

  it("reads a run as .t then one spanning label at the latest clock", () => {
    const { container } = renderColumn(recapOf([entry(4, 10), entry(3, 9), entry(2, 8)]));
    const row = container.querySelector(".prow") as HTMLElement;
    expect(row.children[0]).toHaveClass("t");
    expect(row.children[0].textContent).toBe("2:10 PM");
    expect(row.children[1]).toHaveClass("what", "what-run");
    expect(row.children[1].textContent).toBe("3 features specced");
  });
});
