// History fixes from the browser walk of batches D to F: the head names a
// point in time after "since", an asked burst never doubles its verb, a
// by-time fold names its epic, every row holds the toggle column, the
// project group is capitalized, and a node History row drops a chip its
// verb already states.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { HistoryEntry, Recap, RecapEntry, RecapGroup } from "../../api";
import History from "../../views/History";
import HistorySection from "../../views/node/HistorySection";

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

function renderHistory(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/history" element={<History />} />
      </Routes>
    </MemoryRouter>,
  );
}

function entry(type: string, at: string, overrides: Partial<RecapEntry> = {}): RecapEntry {
  return { at, type, contributor: "jc", command: "x", ask: false, targets: [], ...overrides };
}

function group(key: string | null, entries: RecapEntry[]): RecapGroup {
  return {
    epic: key === null ? null : { key, id: `E-${key}`, ref: `${key} (E-${key})`, title: key },
    stream: "jc",
    entries,
    omitted: 0,
  };
}

function recapOf(groups: RecapGroup[], overrides: Partial<Recap> = {}): Recap {
  const total = groups.reduce((n, g) => n + g.entries.length, 0);
  return { since: "", window: "7d", reader: "jc", total, groups, ...overrides };
}

const T1 = "2026-07-24T10:03:00Z";
const T2 = "2026-07-24T10:02:00Z";
const T3 = "2026-07-24T10:01:00Z";

describe("History head", () => {
  it("names a point in time after since, never a relative age", async () => {
    const since = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
    stubFetch({
      "/api/recap?window=visit": recapOf([group("audit", [entry("approve.feature", T1)])], { since, window: "visit" }),
    });
    const { container } = renderHistory("/history");
    await screen.findByText(/approved/, { selector: ".verb, b" });
    const sub = container.querySelector(".hist-sub") as HTMLElement;
    expect(sub.textContent).toMatch(/^1 event since (\w{3} \d+, )?\d+:\d\d [AP]M$/);
    expect(sub.textContent).not.toContain("ago");
    expect(sub.querySelector("time")).toHaveAttribute("title");
  });

  it("is only the nothing-new sentence when nothing is new", async () => {
    const since = new Date(Date.now() - 1000).toISOString();
    stubFetch({ "/api/recap?window=visit": recapOf([], { since, window: "visit", total: 0 }) });
    const { container } = renderHistory("/history");
    await screen.findByText("Nothing new since your last visit.");
    expect(container.querySelector(".hist-sub")).toBeNull();
    expect(container.textContent).not.toContain("0 events");
    expect(container.textContent).not.toContain("just now");
  });
});

describe("History rows", () => {
  it("reads an asked burst as asked of you, never asked asked you", async () => {
    const q = (id: string) => ({ noun: "question", id, ref: id, title: `Question ${id}` });
    stubFetch({
      "/api/recap?window=7d": recapOf([
        group("audit", [
          entry("add.question", T1, { ask: true, targets: [q("Q-1")] }),
          entry("add.question", T2, { ask: true, targets: [q("Q-2")] }),
        ]),
      ]),
    });
    const { container } = renderHistory("/history?window=7d");
    const link = await screen.findByRole("link", { name: "of you" });
    expect(link.getAttribute("href")).toContain("kind=asked");
    const row = link.closest(".hrow") as HTMLElement;
    expect(row.textContent).toContain("2 questions asked");
    expect(container.textContent).not.toMatch(/asked\s*asked/);
  });

  it("keeps asked you on a burst whose verb is not asked", async () => {
    stubFetch({
      "/api/recap?window=7d": recapOf([
        group("audit", [
          entry("spec.feature", T1, { ask: true }),
          entry("spec.feature", T2, { ask: true }),
        ]),
      ]),
    });
    renderHistory("/history?window=7d");
    expect(await screen.findByRole("link", { name: "asked you" })).toBeInTheDocument();
  });

  it("holds the toggle column on a lone row, empty, so who lines up", async () => {
    stubFetch({ "/api/recap?window=7d": recapOf([group("audit", [entry("approve.feature", T1)])]) });
    const { container } = renderHistory("/history?window=7d");
    await screen.findByText(/approved/, { selector: ".verb, b" });
    const lone = container.querySelector(".recap-burst-lone") as HTMLElement;
    const toggle = lone.querySelector(".burst-toggle") as HTMLElement;
    expect(toggle).not.toBeNull();
    expect(toggle).toHaveAttribute("aria-hidden", "true");
    expect(toggle.textContent).toBe("");
    expect(lone.lastElementChild).toBe(toggle);
  });

  it("capitalizes the project group heading", async () => {
    stubFetch({ "/api/recap?window=7d": recapOf([group(null, [entry("approve.feature", T1)])]) });
    renderHistory("/history?window=7d");
    expect(await screen.findByText("Project")).toBeInTheDocument();
    expect(screen.queryByText("project")).toBeNull();
  });
});

describe("History by time", () => {
  it("names the origin epic on a bookkeeping fold", async () => {
    stubFetch({
      "/api/recap?window=7d": recapOf([
        group("audit", [entry("add.feature", T1)]),
        group("payments", [entry("add.feature", T2), entry("approve.feature", T3)]),
      ]),
    });
    const { container } = renderHistory("/history?window=7d&group=time");
    const folds = await screen.findAllByRole("button", { name: /1 bookkeeping change \(added\)/ });
    expect(folds).toHaveLength(2);
    expect(folds.map((f) => f.querySelector(".recap-burst-origin")?.textContent)).toEqual(["audit", "payments"]);
    fireEvent.click(folds[0]);
    expect(container.querySelectorAll(".recap-bookkeeping").length).toBe(2);
  });

  it("leaves a by-epic fold without an origin label", async () => {
    stubFetch({ "/api/recap?window=7d": recapOf([group("audit", [entry("add.feature", T1)])]) });
    renderHistory("/history?window=7d");
    const fold = await screen.findByRole("button", { name: /1 bookkeeping change/ });
    expect(fold.querySelector(".recap-burst-origin")).toBeNull();
  });
});

describe("Node History tab", () => {
  const row = (type: string, outcome: string): HistoryEntry => ({
    at: "2026-10-06T12:00:00Z",
    type,
    contributor: "jc",
    command: "",
    outcome,
  });

  it("drops the chip when the verb already states the outcome", () => {
    const { container } = render(
      <MemoryRouter>
        <HistorySection history={[row("check.task", "check"), row("submit.task", "in_review")]} />
      </MemoryRouter>,
    );
    const [checked, submitted] = Array.from(container.querySelectorAll(".history-entry"));
    expect(checked).toHaveTextContent("checked");
    expect(checked.querySelector(".chip")).toBeNull();
    expect(submitted.querySelector(".chip")).toHaveTextContent("In review");
  });
});
