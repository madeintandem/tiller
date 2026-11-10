// History's actor, kind, and epic filters: three client-side selects riding
// ?actor=, ?kind=, ?epic= over the recap already fetched. views/history/
// filter.test.ts owns the pure rules; these pin what the page renders.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import { PeopleProvider } from "../../people";
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

function renderHistory(entryUrl: string) {
  return render(
    <MemoryRouter initialEntries={[entryUrl]}>
      <PeopleProvider contributors={[{ id: "jc", name: "JC Grubbs" }, { id: "dana", name: "Dana" }]}>
        <Routes>
          <Route path="/history" element={<History />} />
        </Routes>
      </PeopleProvider>
      <LocationProbe />
    </MemoryRouter>,
  );
}

function entry(type: string, contributor: string, ask = false): RecapEntry {
  return { at: "2026-07-24T10:00:00Z", type, contributor, command: "x", ask, targets: [] };
}

function recap(): Recap {
  const audit: RecapGroup = {
    epic: { key: "audit", id: "E-1", title: "Audit Engine", ref: "audit (E-1)" },
    stream: "jc",
    omitted: 0,
    entries: [entry("spec.feature", "jc"), entry("approve.feature", "dana"), entry("add.question", "jc", true)],
  };
  const billing: RecapGroup = {
    epic: { key: "billing", id: "E-2", title: "Billing", ref: "billing (E-2)" },
    stream: "dana",
    omitted: 0,
    entries: [entry("spec.feature", "dana")],
  };
  return { since: "", window: "7d", reader: "jc", total: 1000, groups: [audit, billing] };
}

async function load(url = "/history?window=7d") {
  stubFetch({ "/api/recap?window=7d": recap() });
  renderHistory(url);
  await screen.findByLabelText("Actor");
}

const loc = () => screen.getByTestId("loc").textContent ?? "";
const sub = () => document.querySelector(".hist-sub")?.textContent ?? "";

describe("History filters", () => {
  it("offers three labelled selects with a no-filter first option", async () => {
    await load();
    for (const [label, none] of [["Actor", "All actors"], ["Kind", "All kinds"], ["Epic", "All epics"]] as const) {
      const select = screen.getByLabelText(label) as HTMLSelectElement;
      expect(select.tagName).toBe("SELECT");
      expect(select.className).toContain("control");
      expect(select.options[0].textContent).toBe(none);
      expect(select.value).toBe("");
    }
    const actor = screen.getByLabelText("Actor") as HTMLSelectElement;
    expect([...actor.options].map((o) => o.textContent)).toEqual(["All actors", "Dana", "JC Grubbs"]);
    const kind = screen.getByLabelText("Kind") as HTMLSelectElement;
    expect(kind.options[1].textContent).toBe("asked you");
    const epic = screen.getByLabelText("Epic") as HTMLSelectElement;
    expect([...epic.options].map((o) => o.textContent)).toEqual(["All epics", "Audit Engine", "Billing"]);
  });

  it("reads the head as it does today with no filter", async () => {
    await load();
    expect(sub()).toContain("1000 events in the last 7 days");
    expect(sub()).not.toContain(" of ");
  });

  it("filters rows and states shown of total, riding the URL", async () => {
    await load();
    fireEvent.change(screen.getByLabelText("Actor"), { target: { value: "dana" } });
    expect(loc()).toBe("/history?window=7d&actor=dana");
    expect(sub()).toContain("2 of 1000 events");
  });

  it("applies a filter from the URL in both Milestones and Everything", async () => {
    await load("/history?window=7d&kind=specced&epic=E-1");
    expect(sub()).toContain("1 of 1000 events");
    expect((screen.getByLabelText("Kind") as HTMLSelectElement).value).toBe("specced");
    expect((screen.getByLabelText("Epic") as HTMLSelectElement).value).toBe("E-1");
    cleanup();
    await load("/history?window=7d&show=all&kind=specced&epic=E-1");
    expect(sub()).toContain("1 of 1000 events");
  });

  it("ignores an unknown value and shows the no-filter option", async () => {
    await load("/history?window=7d&actor=nobody&kind=bogus&epic=E-9");
    expect((screen.getByLabelText("Actor") as HTMLSelectElement).value).toBe("");
    expect((screen.getByLabelText("Kind") as HTMLSelectElement).value).toBe("");
    expect((screen.getByLabelText("Epic") as HTMLSelectElement).value).toBe("");
    expect(sub()).not.toContain(" of ");
  });

  it("keeps filters across a window or show change, and the reverse", async () => {
    await load("/history?window=7d&actor=dana&epic=E-2");
    fireEvent.click(screen.getByRole("link", { name: "Everything" }));
    expect(loc()).toBe("/history?window=7d&show=all&actor=dana&epic=E-2");
    fireEvent.change(screen.getByLabelText("Kind"), { target: { value: "specced" } });
    expect(loc()).toContain("show=all");
    expect(loc()).toContain("kind=specced");
    fireEvent.change(screen.getByLabelText("Actor"), { target: { value: "" } });
    expect(loc()).not.toContain("actor=");
  });

  it("says no events match, with a link that clears the filters", async () => {
    await load("/history?window=7d&actor=dana&kind=asked-you");
    expect(screen.getByText(/No events match the filters/)).toBeTruthy();
    fireEvent.click(screen.getByRole("link", { name: "Clear filters" }));
    expect(loc()).toBe("/history?window=7d");
  });

  it("turns the ask clause into a link that sets kind=asked-you", async () => {
    await load("/history?window=7d&actor=jc&epic=E-1&show=all");
    const row = document.querySelector(".recap-burst .recap-asks-count") as HTMLElement;
    // The row is a lone asked question, whose verb is already "asked".
    const link = within(row).getByRole("link", { name: "of you" });
    fireEvent.click(link);
    expect(loc()).toBe("/history?window=7d&show=all&actor=jc&kind=asked-you&epic=E-1");
  });

  it("agrees with the head's asked-you tally once the ask link filters", async () => {
    await load("/history?window=7d&show=all&kind=asked-you");
    const tally = document.querySelector(".recap-tally .recap-asks-count")?.textContent ?? "";
    const asks = Number(tally.split(" ")[0]);
    expect(sub()).toContain(`${asks} of 1000 events`);
  });
});
