// History's catch-up button (Task 25): the visit horizon moves only when the
// reader presses `Mark caught up`. Viewing never sends the server's visit
// marker (X-Tiller-Visit), and the press is a one-shot marked GET.

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import History from "../../views/History";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function entry(type: string, contributor: string): RecapEntry {
  return { at: "2026-07-24T10:00:00Z", type, contributor, command: "x", ask: false, targets: [] };
}

function recap(window: Recap["window"] = "visit"): Recap {
  const audit: RecapGroup = {
    epic: { key: "audit", id: "E-1", title: "Audit Engine", ref: "audit (E-1)" },
    stream: "jc",
    omitted: 0,
    entries: [entry("spec.feature", "jc"), entry("approve.feature", "allen")],
  };
  return { since: "2026-07-24T00:00:00Z", window, reader: "jc", total: 1000, groups: [audit] };
}

type Call = { url: string; marked: boolean };

// A fetch stub that records every call and whether it carried the marker. A
// marked call is answered by `onMarked` so a test can hold it open or fail it.
function stubFetch(
  routes: Record<string, unknown>,
  onMarked: () => Promise<unknown> = () => Promise.resolve({ ok: true, json: () => Promise.resolve(recap()) }),
) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown, init?: RequestInit) => {
      const url = String(input);
      const marked = new Headers(init?.headers).get("X-Tiller-Visit") === "1";
      calls.push({ url, marked });
      if (marked) return onMarked();
      if (url in routes) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
      }
      return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
    }),
  );
  return calls;
}

function renderHistory(entryUrl = "/history") {
  return render(
    <MemoryRouter initialEntries={[entryUrl]}>
      <Routes>
        <Route path="/history" element={<History />} />
      </Routes>
    </MemoryRouter>,
  );
}

const VISIT = "/api/recap?window=visit";
const button = () => screen.getByRole("button", { name: "Mark caught up" });

describe("History: catching up is the reader's call", () => {
  it("opens the visit window without the marker and offers the button", async () => {
    const calls = stubFetch({ [VISIT]: recap() });
    const { container } = renderHistory();

    await screen.findByText(/1000 events/);
    expect(calls).toEqual([{ url: VISIT, marked: false }]);
    expect(container.querySelector(".hist-sub")?.textContent).toMatch(/^1000 events since Jul \d+, \d+:\d\d [AP]M$/);
    expect(container.querySelector(".hist-sub")?.textContent).not.toContain("marks you caught up");
    expect(container.querySelector(".hist-sub time")).toHaveAttribute("title");
    expect(button()).toBeEnabled();
  });

  it("treats a stored /history?window=visit link exactly like /history", async () => {
    const calls = stubFetch({ [VISIT]: recap() });
    renderHistory("/history?window=visit");

    await screen.findByText(/1000 events/);
    expect(calls).toEqual([{ url: VISIT, marked: false }]);
    expect(button()).toBeInTheDocument();
  });

  it("sends exactly one marked GET on press, then confirms and keeps the list", async () => {
    const calls = stubFetch({ [VISIT]: recap() });
    renderHistory();
    await screen.findByText(/1000 events/);
    const rowsBefore = document.querySelectorAll(".hrow").length;
    expect(rowsBefore).toBeGreaterThan(0);

    fireEvent.click(button());

    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mark caught up" })).toBeNull();
    expect(calls).toEqual([
      { url: VISIT, marked: false },
      { url: VISIT, marked: true },
    ]);
    expect(document.querySelectorAll(".hrow").length).toBe(rowsBefore);
    expect(screen.getByText(/1000 events/)).toBeInTheDocument();
  });

  it("disables the button while the request is in flight, so a double press sends one", async () => {
    let release: (v: unknown) => void = () => {};
    const calls = stubFetch(
      { [VISIT]: recap() },
      () => new Promise((resolve) => (release = resolve)),
    );
    renderHistory();
    await screen.findByText(/1000 events/);

    fireEvent.click(button());
    await waitFor(() => expect(button()).toBeDisabled());
    fireEvent.click(button());
    expect(calls.filter((c) => c.marked)).toHaveLength(1);

    release({ ok: true, json: () => Promise.resolve(recap()) });
    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();
    expect(calls.filter((c) => c.marked)).toHaveLength(1);
  });

  it("says it did not save when the request fails, and lets the reader retry", async () => {
    let fail = true;
    const calls = stubFetch({ [VISIT]: recap() }, () =>
      fail
        ? Promise.resolve({ ok: false, status: 500, statusText: "Boom", text: () => Promise.resolve("") })
        : Promise.resolve({ ok: true, json: () => Promise.resolve(recap()) }),
    );
    renderHistory();
    await screen.findByText(/1000 events/);

    fireEvent.click(button());
    expect(await screen.findByText("That did not save. Try again.")).toBeInTheDocument();
    expect(screen.queryByText("Caught up. Your next visit starts here.")).toBeNull();
    expect(button()).toBeEnabled();

    fail = false;
    fireEvent.click(button());
    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();
    expect(screen.queryByText("That did not save. Try again.")).toBeNull();
    expect(calls.filter((c) => c.marked)).toHaveLength(2);
  });

  it("shows no button on a fixed window", async () => {
    for (const w of ["24h", "7d"] as const) {
      const calls = stubFetch({ [`/api/recap?window=${w}`]: recap(w) });
      renderHistory(`/history?window=${w}`);
      await screen.findByText(/1000 events/);
      expect(screen.queryByRole("button", { name: "Mark caught up" })).toBeNull();
      expect(calls.some((c) => c.marked)).toBe(false);
      cleanup();
    }
  });

  it("offers the button on an empty first visit, where nothing else can move the mark", async () => {
    stubFetch({ [VISIT]: { since: "", window: "visit", reader: "", total: 0, groups: [] } });
    renderHistory();
    await screen.findByText("Nothing new since your last visit.");
    expect(button()).toBeInTheDocument();
  });

  it("keeps the button under filters and show modes, and says it marks the whole window", async () => {
    const calls = stubFetch({ [VISIT]: recap() });
    renderHistory("/history?actor=allen&show=all");
    await screen.findByText(/of 1000 events/);

    expect(button().getAttribute("title")).toMatch(/whole window/);
    // Filter and show-mode changes never send the marker.
    expect(calls.some((c) => c.marked)).toBe(false);
  });
});
