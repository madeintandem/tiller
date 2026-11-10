// History while the page rereads or switches window: the list stays until the
// new answer lands, the window links and the three filter selects never leave
// the page, a reread (focus included) never carries the visit marker, and a
// Caught up confirmation survives a reread that brings nothing new.

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../api";
import type { Recap, RecapEntry } from "../../api";
import { expectNoLoading, renderApp, stubReread, summary, watchLoading, type Routes, settleReads } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const THROUGH = "2026-07-24T12:00:00Z";
const VISIT = "/api/recap?window=visit";
const DAY = "/api/recap?window=24h";
const MARKED = `${VISIT}&through=${encodeURIComponent(THROUGH)}`;

let clock = 0;

// One specced feature, each a minute after the last so no two fold together.
function entry(title: string, contributor: string): RecapEntry {
  clock += 1;
  const id = `F-${clock}`;
  return {
    at: `2026-07-24T10:${String(clock % 60).padStart(2, "0")}:00Z`, type: "spec.feature", contributor,
    command: `tiller spec feature ${id}`, ask: false,
    targets: [{ noun: "feature", id, ref: `${id} (${id})`, key: `audit/${id}`, title }],
  };
}

function recap(window: Recap["window"], entries: RecapEntry[], since = "2026-07-24T00:00:00Z"): Recap {
  return {
    since, window, reader: "jc", total: entries.length, through: THROUGH,
    groups: entries.length === 0 ? [] : [{
      epic: { key: "audit", id: "E-1", title: "Audit Engine", ref: "audit (E-1)" },
      stream: "", omitted: 0, entries,
    }],
  };
}

function routes(extra: Routes = {}): Routes {
  return {
    "/api/status": summary(),
    [VISIT]: recap("visit", [entry("Audit", "jc"), entry("Ledger", "sam")]),
    ...extra,
  };
}

const select = (name: string) => screen.getByRole("combobox", { name });

describe("History while the page rereads", () => {
  it("keeps the filtered list and its filter, then updates it in place", async () => {
    const stub = stubReread(routes());
    renderApp("/history?actor=sam&show=all");
    expect(await screen.findByText("Ledger")).toBeInTheDocument();
    expect(select("Actor")).toHaveValue("sam");

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("Ledger")).toBeInTheDocument();
    expect(select("Actor")).toHaveValue("sam");

    await stub.release({
      [VISIT]: recap("visit", [entry("Ledger again", "sam"), entry("Audit", "jc")]),
    });
    expect(await screen.findByText("Ledger again")).toBeInTheDocument();
    expect(select("Actor")).toHaveValue("sam");
    expect(screen.getByTestId("where")).toHaveTextContent("/history?actor=sam&show=all");
    expect(stub.calls.some((c) => c.marked)).toBe(false);
    expectNoLoading(watch);
  });

  it("keeps the window links and the three selects mounted across a window switch", async () => {
    const stub = stubReread({ ...routes(), [DAY]: recap("24h", [entry("Audit", "jc")]) });
    renderApp("/history?actor=sam&show=all");
    expect(await screen.findByText("Ledger")).toBeInTheDocument();
    const selects = ["Actor", "Kind", "Epic"].map(select);

    stub.hold();
    fireEvent.click(screen.getByRole("link", { name: "24h" }));
    expect(await screen.findByText("Loading history...")).toBeInTheDocument();
    for (const element of selects) expect(element).toBeInTheDocument();
    expect(select("Actor")).toHaveValue("sam");
    expect(select("Actor")).toBeDisabled();
    expect(screen.getByRole("link", { name: "visit" })).toBeInTheDocument();

    await stub.release();
    expect(await screen.findByText("Audit")).toBeInTheDocument();
    for (const element of selects) expect(element).toBeInTheDocument();
    // sam did nothing in the last 24 hours, so the filter falls back to none.
    expect(select("Actor")).toHaveValue("");
    expect(select("Actor")).toBeEnabled();
  });

  it("rereads on focus without the visit marker", async () => {
    const stub = stubReread(routes());
    renderApp("/history");
    expect(await screen.findByText("Audit")).toBeInTheDocument();
    const reads = stub.calls.filter((c) => c.url === VISIT).length;
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(new Date(Date.now() - 60_000));

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitFor(() => expect(stub.calls.filter((c) => c.url === VISIT).length).toBe(reads + 1));
    expect(stub.calls.some((c) => c.marked)).toBe(false);
  });

  it("keeps Caught up through a reread that brings nothing new, and offers the button again when something does", async () => {
    const stub = stubReread({ ...routes(), [MARKED]: routes()[VISIT] });
    renderApp("/history");
    expect(await screen.findByText("Audit")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Mark caught up" }));
    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();

    stub.reread();
    await stub.release({ [VISIT]: recap("visit", [], THROUGH) });
    expect(await screen.findByText("Nothing new since your last visit.")).toBeInTheDocument();
    expect(screen.getByText("Caught up. Your next visit starts here.")).toBeInTheDocument();
    expect(stub.calls.filter((c) => c.marked)).toHaveLength(1);

    stub.reread();
    await stub.release({ [VISIT]: recap("visit", [entry("Billing", "jc")], THROUGH) });
    expect(await screen.findByText("Billing")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark caught up" })).toBeInTheDocument();
    expect(stub.calls.filter((c) => c.marked)).toHaveLength(1);
  });

  it("brings the button back enabled after a press, and a second press marks the new list's through", async () => {
    const stub = stubReread({ ...routes(), [MARKED]: routes()[VISIT] });
    renderApp("/history");
    expect(await screen.findByText("Audit")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Mark caught up" }));
    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();

    const later = "2026-07-24T13:00:00Z";
    const newer = { ...recap("visit", [entry("Billing", "jc")], THROUGH), through: later };
    const markedLater = `${VISIT}&through=${encodeURIComponent(later)}`;
    stub.reread();
    await stub.release({ [VISIT]: newer, [markedLater]: newer });
    expect(await screen.findByText("Billing")).toBeInTheDocument();
    const button = screen.getByRole("button", { name: "Mark caught up" });
    expect(button).toBeEnabled();

    fireEvent.click(button);
    expect(await screen.findByText("Caught up. Your next visit starts here.")).toBeInTheDocument();
    const marked = stub.calls.filter((c) => c.marked);
    expect(marked.map((c) => c.url)).toEqual([MARKED, markedLater]);
  });

  it("keeps the list and says it is out of date when the reread fails", async () => {
    const stub = stubReread(routes());
    renderApp("/history");
    expect(await screen.findByText("Audit")).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByText("Audit")).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/recap\?window=visit failed/)).toBeInTheDocument();
  });
});
