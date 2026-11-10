// A reread keeps History mounted, so a row's open or expanded state must stay
// with that row: one new burst at the top of a day must not move every open
// row down by one.

import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry } from "../../api";
import { renderApp, settleReads, stubReread, summary } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const VISIT = "/api/recap?window=visit";

function entry(minute: number, type: string, contributor: string, title: string): RecapEntry {
  const id = `F-${type.replace(".", "")}${minute}`;
  return {
    at: `2026-07-24T10:${String(minute).padStart(2, "0")}:00Z`, type, contributor,
    command: `tiller ${type}`, ask: false,
    targets: [{ noun: "feature", id, ref: `${id} (${id})`, key: `audit/${id}`, title }],
  };
}

function recap(entries: RecapEntry[]): Recap {
  return {
    since: "2026-07-24T00:00:00Z", window: "visit", reader: "jc", total: entries.length, through: "2026-07-24T12:00:00Z",
    groups: [{ epic: { key: "audit", id: "E-1", title: "Audit Engine", ref: "audit (E-1)" }, stream: "", omitted: 0, entries }],
  };
}

// Newest first: a run of two specs by jc, then a run of two submits by sam.
const specs = [entry(40, "spec.feature", "jc", "Spec two"), entry(39, "spec.feature", "jc", "Spec one")];
const submits = [entry(30, "submit.task", "sam", "Submit two"), entry(29, "submit.task", "sam", "Submit one")];
const newer = entry(50, "approve.feature", "kim", "Approved");

const rowNamed = (text: RegExp) => screen.getByText(text).closest("details") as HTMLDetailsElement;

describe("History rows keep their open state across a reread", () => {
  it("keeps the opened run open when a new burst lands above it", async () => {
    const stub = stubReread({ "/api/status": summary(), [VISIT]: recap([...specs, ...submits]) });
    renderApp("/history?show=all");
    await screen.findByText(/2 features specced/);
    const submitRow = rowNamed(/2 tasks submitted|2 features submitted/);
    fireEvent.click(within(submitRow).getByText(/\+ open/));
    expect(submitRow.open).toBe(true);

    stub.reread();
    await stub.release({ [VISIT]: recap([newer, ...specs, ...submits]) });
    expect(await screen.findByText("Approved")).toBeInTheDocument();

    expect(rowNamed(/2 tasks submitted|2 features submitted/).open).toBe(true);
    expect(rowNamed(/2 features specced/).open).toBe(false);
  });

  it("keeps an opened bookkeeping fold open when a new milestone lands above it", async () => {
    const links = [entry(20, "link.feature", "jc", "Linked two"), entry(19, "link.feature", "jc", "Linked one")];
    const stub = stubReread({ "/api/status": summary(), [VISIT]: recap([...specs, ...links]) });
    renderApp("/history");
    const fold = await screen.findByRole("button", { name: /2 bookkeeping changes/ });
    fireEvent.click(fold);
    expect(fold).toHaveAttribute("aria-expanded", "true");

    stub.reread();
    await stub.release({ [VISIT]: recap([newer, ...specs, ...links]) });
    expect(await screen.findByText("Approved")).toBeInTheDocument();

    expect(screen.getByRole("button", { name: /2 bookkeeping changes/ })).toHaveAttribute("aria-expanded", "true");
  });
});
