// Vitest/RTL coverage for the History view (the recap-page pass, then the
// phase-5 restyle to the console-v2 mock, task-19-brief.md - the file and its
// row anatomy follow the mock, the wire types and fixtures below (Recap,
// RecapEntry, RecapGroup) did not).
//
// The route and window tests pin the horizon rule's client half: the window is
// in the URL rather than in component state, so a window a human can switch to
// is one they can link to, and only the visit window (the one request that
// advances the last-visit mark, serve/home.go) is the page's default.
//
// The grammar tests pin what the page does with volume. Measured on the live
// Acme tree: the rail rendered 167 events as 167 rows, each printing the
// event type, the timestamp and the owner, all three identical down whole
// blocks of the page, and its 32 add.question rows truncated mid-word in a
// 464px column. The page collapses consecutive same-type runs into one counted
// row that expands in place, and hoists nothing, because the asks it used to
// hoist were the exact same ids the answer-questions card lists on Home.
//
// lib/bursts.test.ts owns the collapse rules themselves. These tests own what
// the rendered page promises: that a collapsed count is true, that everything
// under it is reachable, and that the duplicate ask block is gone for good -
// plus, new in the phase-5 restyle, that the row reads in the mock's own
// anatomy (time/dot/what/ref/who), that day subheads earn their place only
// within a group that actually spans days, and that the client's own "N more"
// cap never hides an ask the way the server's group cap already never does.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useNavigationType } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import { bumpFetchGeneration } from "../../api";
import History from "../../views/History";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const QUESTION_TITLE = "Audit selection unit";
const QUESTION_STATEMENT =
  "Should the audit select one record per project or one record per project phase?\n\n" +
  "The answer changes how multi-year projects are counted in review queues and exported reports.";

// A fetch stub keyed by URL, the same pattern home.test.tsx established: an
// unmapped path 404s the way fetchJSON expects, so a missing fixture fails
// loudly instead of silently returning undefined.
function stubFetch(routes: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    }
    return Promise.resolve({
      ok: false,
      status: 404,
      statusText: "Not Found",
      text: () => Promise.resolve(""),
    });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

// The rendered URL, so a window switch can be asserted as navigation rather
// than as component state that happens to refetch.
function LocationProbe() {
  const loc = useLocation();
  return <span data-testid="loc">{loc.pathname + loc.search}</span>;
}

// The URL alone cannot distinguish replace from push - both leave the same
// final location (build.test.tsx's own "review finding 2", the house idiom
// this borrows verbatim). useNavigationType() reports the LAST navigation's
// own kind ("POP" | "PUSH" | "REPLACE"), which is the one signal that
// actually tells them apart, and the one this file's own WindowLink `replace`
// prop (task-19 review, finding 3) needs pinned rather than left to ride on
// the URL assertions alone.
function NavigationTypeProbe() {
  const type = useNavigationType();
  return <span data-testid="navtype">{type}</span>;
}

function renderHistory(entry = "/history") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/history" element={<History />} />
      </Routes>
      <LocationProbe />
      <NavigationTypeProbe />
    </MemoryRouter>,
  );
}

function navType(): string {
  return screen.getByTestId("navtype").textContent ?? "";
}

function recapEntry(overrides: Partial<RecapEntry> = {}): RecapEntry {
  return {
    at: "2026-07-24T10:00:00Z",
    type: "start.task",
    contributor: "jc",
    command: "tiller start task T-1",
    ask: false,
    targets: [],
    ...overrides,
  };
}

function feature(n: number): RecapEntry {
  return recapEntry({
    at: "2026-07-24T09:37:00Z",
    type: "add.feature",
    targets: [
      {
        noun: "feature",
        id: `F-${n}`,
        key: `audit-engine/feature-${n}`,
        title: `Feature ${n}`,
        ref: `audit-engine/feature-${n} (F-${n})`,
      },
    ],
  });
}

// The live shape at one epic's scale: one decomposition burst of 8 features
// under an epic that was added in the same minute, plus the project-level cell
// (epic null, which the API sorts LAST) carrying the questions the run opened.
// Every entry here falls on the same calendar day (2026-07-24), which is what
// the day-subhead tests below lean on: a served group at this scale earns no
// subhead of its own.
function loadedRecap(): Recap {
  const busy: RecapGroup = {
    epic: {
      key: "audit-engine",
      id: "E-1",
      title: "Audit Engine",
      ref: "audit-engine (E-1)",
    },
    stream: "jc",
    omitted: 0,
    entries: [
      ...Array.from({ length: 8 }, (_, i) => feature(i)),
      recapEntry({
        at: "2026-07-24T09:36:00Z",
        type: "add.epic",
        targets: [
          {
            noun: "epic",
            id: "E-1",
            key: "audit-engine",
            title: "Audit Engine",
            ref: "audit-engine (E-1)",
          },
        ],
      }),
    ],
  };
  const questions: RecapGroup = {
    epic: null,
    stream: "jc",
    omitted: 0,
    entries: Array.from({ length: 3 }, (_, i) =>
      recapEntry({
        at: `2026-07-24T08:0${i}:00Z`,
        type: "add.question",
        ask: true,
        targets: [
          { noun: "question", id: `Q-${i}`, title: `Question number ${i}?`, ref: `Q-${i}` },
        ],
      }),
    ),
  };
  return {
    since: "2026-07-24T00:00:00Z",
    window: "visit",
    reader: "jc",
    total: 12,
    groups: [busy, questions],
  };
}

describe("History view: the route and the window", () => {
  it("renders the digest at /history on the visit window by default", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });

    renderHistory();

    expect(await screen.findByRole("heading", { name: "History" })).toBeInTheDocument();
    expect(screen.getByText(/12 events/)).toBeInTheDocument();
    expect(spy.mock.calls.map((c) => String(c[0]))).toEqual(["/api/recap?window=visit"]);
  });

  // The server advances the visit horizon only for a request carrying this
  // marker (internal/serve/home.go's visitHeader), so a plain GET from a
  // foreign page or a curl reads the digest without spending it.
  it("marks the visit-window read, and only that read, as the console's visit", async () => {
    const day: Recap = { ...loadedRecap(), window: "24h", total: 4 };
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap(), "/api/recap?window=24h": day });
    const marker = (call: unknown[]) =>
      new Headers((call[1] as RequestInit | undefined)?.headers).get("X-Tiller-Visit");

    renderHistory();
    await screen.findByText(/12 events/);
    expect(spy.mock.calls.map(marker)).toEqual(["1"]);

    fireEvent.click(screen.getByRole("link", { name: "24h" }));
    await screen.findByText(/4 events/);
    expect(spy.mock.calls.map(marker)).toEqual(["1", null]);
  });

  it("reads the window off the URL and marks that switch active", async () => {
    const day: Recap = { ...loadedRecap(), window: "24h", total: 4 };
    const spy = stubFetch({ "/api/recap?window=24h": day });

    renderHistory("/history?window=24h");

    await screen.findByText(/4 events/);
    expect(spy.mock.calls.map((c) => String(c[0]))).toEqual(["/api/recap?window=24h"]);
    expect(screen.getByRole("link", { name: "24h" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "visit" })).not.toHaveAttribute("aria-current");
  });

  it("degrades an unrecognized window to visit rather than blanking the page", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });

    renderHistory("/history?window=1y");

    await screen.findByText(/12 events/);
    expect(spy.mock.calls.map((c) => String(c[0]))).toEqual(["/api/recap?window=visit"]);
    expect(screen.getByRole("link", { name: "visit" })).toHaveAttribute("aria-current", "page");
  });

  it("switches windows by navigating, so the state is linkable and visit stays canonical", async () => {
    const week: Recap = { ...loadedRecap(), window: "7d", total: 90 };
    stubFetch({ "/api/recap?window=visit": loadedRecap(), "/api/recap?window=7d": week });

    renderHistory();
    await screen.findByText(/12 events/);

    fireEvent.click(screen.getByRole("link", { name: "7d" }));
    expect(await screen.findByText(/90 events/)).toBeInTheDocument();
    expect(screen.getByTestId("loc").textContent).toBe("/history?window=7d");

    // Back to the default drops the param entirely, the way Explore's browse
    // link drops ?mode=.
    fireEvent.click(screen.getByRole("link", { name: "visit" }));
    expect(await screen.findByText(/12 events/)).toBeInTheDocument();
    expect(screen.getByTestId("loc").textContent).toBe("/history");
  });

  it("a window switch replaces the URL rather than pushing a history entry", async () => {
    const week: Recap = { ...loadedRecap(), window: "7d", total: 90 };
    stubFetch({ "/api/recap?window=visit": loadedRecap(), "/api/recap?window=7d": week });

    renderHistory();
    await screen.findByText(/12 events/);
    // The initial mount's own navigation is POP (MemoryRouter's starting
    // entry) - the baseline the REPLACE assertion below has to beat, since a
    // push would ALSO leave navType at something other than POP.
    expect(navType()).toBe("POP");

    fireEvent.click(screen.getByRole("link", { name: "7d" }));

    // The URL alone (already pinned above) is satisfied identically by a
    // push; this is the assertion that actually pins replace-not-push.
    await waitFor(() => expect(screen.getByTestId("loc").textContent).toBe("/history?window=7d"));
    expect(navType()).toBe("REPLACE");
  });
});

describe("History view: the marking one-liner under the head", () => {
  it("states the visit window marks the reader caught up", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    // The exact "since" clock time is locale/timezone-formatted (formatTime),
    // so this pins the STRUCTURE - count, the since clause, the marking
    // clause - without hardcoding a formatted instant.
    expect(container.querySelector(".hist-sub")?.textContent).toMatch(
      /^12 events since .+ · reading this view marks you caught up$/,
    );
  });

  it("states a fixed window never does, adapted to the window actually open", async () => {
    const week: Recap = { ...loadedRecap(), window: "7d", total: 90 };
    stubFetch({ "/api/recap?window=7d": week });
    const { container } = renderHistory("/history?window=7d");
    await screen.findByText(/90 events/);

    // No "since" here at all: a fixed window is not framed by the visit
    // horizon, so nothing about it is stated relative to that instant.
    expect(container.querySelector(".hist-sub")?.textContent).toBe(
      "90 events in the last 7 days · fixed windows never mark you caught up",
    );
  });
});

describe("History view: the empty state, which is the steady state", () => {
  it("names the horizon once and hands over the wider windows", async () => {
    stubFetch({
      "/api/recap?window=visit": {
        since: "2026-07-25T21:47:22Z",
        window: "visit",
        reader: "jc",
        total: 0,
        groups: [],
      },
    });

    const { container } = renderHistory();

    // Opening the page advances the horizon, so an empty visit window is what
    // a second look normally returns. It must be a working surface.
    expect(await screen.findByText("Nothing new since your last visit.")).toBeInTheDocument();
    // The instant is stated once, on the hist-sub line, not again in the
    // sentence four lines below it.
    expect(container.querySelector(".hist-sub")?.textContent).toMatch(
      /^0 events since .+ · reading this view marks you caught up$/,
    );
    expect(container.textContent?.match(/Jul 25/g)).toHaveLength(1);
    expect(container.querySelector(".recap-empty-more")?.textContent).toContain(
      "Look back further",
    );
    const escapes = container.querySelectorAll(".recap-empty-more a");
    expect(Array.from(escapes, (a) => a.textContent)).toEqual(["24h", "7d"]);
  });

  it("offers only the windows WIDER than the one that came up empty", async () => {
    stubFetch({
      "/api/recap?window=7d": { since: "", window: "7d", reader: "", total: 0, groups: [] },
    });

    const { container } = renderHistory("/history?window=7d");

    expect(await screen.findByText("Nothing recorded in the last 7 days.")).toBeInTheDocument();
    expect(container.querySelector(".recap-empty-more")).toBeNull();
  });

  it("says nothing about an instant a first visit does not have", async () => {
    stubFetch({
      "/api/recap?window=visit": { since: "", window: "visit", reader: "", total: 0, groups: [] },
    });

    const { container } = renderHistory();

    expect(await screen.findByText("Nothing new since your last visit.")).toBeInTheDocument();
    expect(container.querySelector(".hist-sub")?.textContent).toBe(
      "0 events · reading this view marks you caught up",
    );
  });

  it("names the window it is actually rendering, never a visit claim on an override", async () => {
    // "reading this view marks you caught up" is a claim about the VISIT
    // mark, and the two overrides never touch it (ui-surface.md: "The window
    // override never touches the mark at all"). Unconditional, a copy-pasted
    // sentence would put that claim under an empty 24h view that just did no
    // such thing: one page, one window, two framings, one of them false.
    const empty = (window: string) => ({ since: "", window, reader: "", total: 0, groups: [] });
    stubFetch({
      "/api/recap?window=24h": empty("24h"),
      "/api/recap?window=7d": empty("7d"),
    });

    const day = renderHistory("/history?window=24h");
    await screen.findByText("Nothing recorded in the last 24 hours.");
    expect(day.container.querySelector(".hist-sub")?.textContent).toBe(
      "0 events in the last 24 hours · fixed windows never mark you caught up",
    );
    cleanup();

    const week = renderHistory("/history?window=7d");
    await screen.findByText("Nothing recorded in the last 7 days.");
    expect(week.container.querySelector(".hist-sub")?.textContent).toBe(
      "0 events in the last 7 days · fixed windows never mark you caught up",
    );
  });
});

describe("History view: burst collapsing", () => {
  it("renders a run of 8 as ONE row carrying the count, not 8 rows carrying it 8 times", async () => {
    const { container } = (stubFetch({ "/api/recap?window=visit": loadedRecap() }), renderHistory());
    await screen.findByText(/12 events/);

    const group = container.querySelectorAll(".recap-group")[0];
    const rows = group.querySelectorAll(".recap-burst");
    expect(rows).toHaveLength(2);
    // The count folds into the row's own bold "what" now (phase-5 restyle),
    // rather than a separate numeral column - it is still the first thing a
    // reader learns about the row.
    expect(Array.from(rows, (r) => r.querySelector(".hrow .what")?.textContent)).toEqual([
      "8 features added",
      "Audit Engine added",
    ]);
    // The three columns that used to repeat on every row are stated once or
    // not at all: no row prints the raw event type.
    expect(screen.queryByText("add.feature")).not.toBeInTheDocument();
    expect(screen.queryByText("add.epic")).not.toBeInTheDocument();
  });

  it("keeps every collapsed entry reachable, one click, no refetch", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const burst = container.querySelector(".recap-burst details") as HTMLDetailsElement;
    expect(burst.open).toBe(false);
    fireEvent.click(burst.querySelector("summary")!);

    // ROW_CAP of the 8, then the rest behind "N more", DecisionCard's grammar.
    expect(burst.querySelectorAll(".recap-entry")).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: /3 more rows/ }));
    expect(burst.querySelectorAll(".recap-entry")).toHaveLength(8);
    expect(spy.mock.calls).toHaveLength(1);
  });

  it("names its node instead of a count at a run of one, with nothing to expand", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const lone = container.querySelector(".recap-burst-lone")!;
    expect(lone.textContent).toContain("Audit Engine added");
    // The epic's own key equals the group's, so it does not repeat one line
    // below the head: no ref slot renders.
    expect(lone.querySelector(".recap-burst-key")).toBeNull();
    // A run of one has nothing to expand, so it draws no toggle at all -
    // unlike the old chevron, the new toggle does not need to hold a slot on
    // a row with nothing behind it.
    expect(lone.querySelector(".burst-toggle")).toBeNull();
  });

  it("states one time for a run inside a minute and a span for a run that is not", async () => {
    const spread: Recap = {
      since: "2026-07-20T00:00:00Z",
      window: "7d",
      reader: "jc",
      total: 2,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({ at: "2026-07-25T16:40:00Z", type: "sync.task" }),
            recapEntry({ at: "2026-07-20T09:12:00Z", type: "sync.task" }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=7d": spread });

    const { container } = renderHistory("/history?window=7d");
    await screen.findByText(/tasks synced/);

    // A burst row can never claim a five-day span happened at an instant.
    // burstWhen's output rides the row's own leading .t slot now.
    expect(container.querySelector(".hrow .t")?.textContent).toContain(" to ");
  });
});

describe("History view: the restyled row anatomy (time/dot/what/ref/who)", () => {
  it("renders a lone row's five parts", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const lone = container.querySelector(".recap-burst-lone") as HTMLElement;
    // Mono time, leading.
    const t = lone.querySelector(".t")!;
    expect(t.textContent).not.toBe("");
    // A decorative dot - never a status claim (see History.tsx's own note on
    // why), aria-hidden so the words beside it carry the accessible content.
    const dot = lone.querySelector(".dot")!;
    expect(dot).toHaveAttribute("aria-hidden", "true");
    expect(dot.className).not.toContain("ask"); // add.epic never asks
    // Bold what: the node's own name, then the verb.
    expect(within(lone).getByRole("link", { name: "Audit Engine" })).toHaveAttribute(
      "href",
      "/node/epic/E-1",
    );
    expect(lone.querySelector(".what b")?.textContent).toBe("added");
    // Right-aligned who, trailing.
    expect(lone.querySelector(".who")?.textContent).toBe("jc");
  });

  it("shows the leaf key as the ref slot when a node's key differs from its group's", async () => {
    const single: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 1,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [feature(7)],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": single });

    const { container } = renderHistory();
    await screen.findByRole("link", { name: "Feature 7" });

    const lone = container.querySelector(".recap-burst-lone")!;
    expect(lone.textContent).toContain("Feature 7 added");
    expect(lone.querySelector(".recap-burst-key")?.textContent).toBe("feature-7");
    expect(lone.querySelector(".who")?.textContent).toBe("jc");
  });

  it("renders a group row's five parts, with the count folded into the bold what", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const groupRow = container.querySelectorAll(".recap-group")[0].querySelector(
      ".recap-burst summary.hrow",
    )!;
    expect(groupRow.querySelector(".t")?.textContent).not.toBe("");
    expect(groupRow.querySelector(".dot")).not.toBeNull();
    expect(groupRow.querySelector(".what b")?.textContent).toBe("8 features added");
    expect(groupRow.querySelector(".who")?.textContent).toBe("jc");
    // No ref at the collapsed level: a run of many has no single key to show
    // until it is expanded.
    expect(groupRow.querySelector(".recap-burst-key")).toBeNull();
  });

  it("lights the dot accent on an asking burst, at the head AND on every one of its expanded rows", async () => {
    // The positive case beside the lone row's negative one above (add.epic
    // never asks, so its dot stays muted there). loadedRecap's project group
    // is an ALL-ask burst (3 of 3 add.question entries), which is exactly
    // the shape task-19 review finding 2 caught: markAsks (burst.asks > 0 &&
    // burst.asks < entries.length) is FALSE when every entry asked, and the
    // per-row dot was wrongly gated on it too, painting a muted dot under an
    // accent head. The dot now states e.ask's own truth on every row,
    // ungated; the border stays gated (checked separately below).
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const project = container.querySelectorAll(".recap-group")[1];
    const headDot = project.querySelector(".recap-burst .dot")!;
    expect(headDot.className).toContain("ask");

    fireEvent.click(project.querySelector(".recap-burst summary")!);

    const entryDots = project.querySelectorAll(".recap-entry .dot");
    expect(entryDots).toHaveLength(3);
    expect(Array.from(entryDots, (d) => d.className)).toEqual([
      "dot ask",
      "dot ask",
      "dot ask",
    ]);
    // The border stays gated on markAsks (unchanged): an all-ask burst still
    // paints it on NONE of its rows, only the dot differs now.
    expect(project.querySelectorAll(".recap-entry-ask")).toHaveLength(0);
  });
});

describe("History view: burst expand-in-place with toggle wording", () => {
  it("reads '+ open' closed and '– close' open, expanding in place with no refetch", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const details = container.querySelector(".recap-burst details") as HTMLDetailsElement;
    const toggle = container.querySelector(".recap-burst .burst-toggle")!;
    expect(toggle.textContent).toBe("+ open");
    expect(details.open).toBe(false);

    fireEvent.click(container.querySelector(".recap-burst summary")!);

    // jsdom queues the native `toggle` event as a task rather than firing it
    // synchronously inside the click (matching the HTML spec's own "queue a
    // task to fire an event named toggle"), so the state-driven wording only
    // updates once that task runs - waitFor gives it the chance to.
    await waitFor(() => expect(toggle.textContent).toBe("– close"));
    expect(details.open).toBe(true);
    expect(container.querySelector(".burst-body .recap-entry")).not.toBeNull();
    expect(spy.mock.calls).toHaveLength(1);
  });
});

describe("History view: day subheads under option (a)", () => {
  it("adds no day subhead when a served group's bursts fall on one calendar day", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(container.querySelectorAll(".day-h")).toHaveLength(0);
    expect(container.querySelectorAll(".day")).toHaveLength(0);
  });

  it("adds a day subhead per calendar day WITHIN a group whose bursts span days, newest first", async () => {
    const spread: Recap = {
      since: "2026-07-20T00:00:00Z",
      window: "7d",
      reader: "jc",
      total: 2,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            // Different types keep these as two SEPARATE bursts (lib/
            // bursts.ts's own CONSECUTIVE rule), which is what lets them land
            // in two different day buckets instead of merging into one
            // burst that spans both.
            recapEntry({ at: "2026-07-25T16:40:00Z", type: "start.task" }),
            recapEntry({ at: "2026-07-20T11:00:00Z", type: "sync.task" }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=7d": spread });

    const { container } = renderHistory("/history?window=7d");
    await screen.findByText(/2 events in the last 7 days/);

    // Asserted structurally rather than against a formatted date string
    // (task-19 review fold-in: the .hist-sub fix above already dropped this
    // exact fragility - toMatch over a formatted instant rather than a
    // hardcoded one - and it crept back in here). The two entries are 5
    // days apart, so no real-world timezone can ever fold them into the
    // same calendar day; what a build's own local clock renders the LABEL
    // as is not this test's business, only that dayGroupsOf treated them as
    // two distinct days, in newest-first order.
    const heads = container.querySelectorAll(".day-h .eyebrow");
    expect(heads).toHaveLength(2);
    expect(heads[0].textContent).not.toBe("");
    expect(heads[1].textContent).not.toBe("");
    expect(heads[0].textContent).not.toBe(heads[1].textContent);

    const days = container.querySelectorAll(".day");
    expect(days).toHaveLength(2);
    // Newest first: the start.task burst (Jul 25, the later entry) leads,
    // the sync.task burst (Jul 20) trails - proven by which burst's own text
    // lands in which day block, not by the day label's own formatted text.
    expect(days[0].textContent).toContain("started");
    expect(days[1].textContent).toContain("synced");
    expect(days[0].querySelectorAll(".recap-burst")).toHaveLength(1);
    expect(days[1].querySelectorAll(".recap-burst")).toHaveLength(1);
  });
});

describe("History view: the caps rule extends to the client's own row cap", () => {
  it("never lets an ask sit behind 'N more': the row cap ships every ask up front", async () => {
    // Five routine entries, newest first, then the oldest of the six is the
    // one that asked - past ROW_CAP (5) in a naive slice(0, ROW_CAP).
    const routine = Array.from({ length: 5 }, (_, i) =>
      recapEntry({ at: `2026-07-24T09:0${4 - i}:00Z`, type: "done.task", ask: false }),
    );
    const asked = recapEntry({ at: "2026-07-24T08:59:00Z", type: "done.task", ask: true });
    const capped: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 6,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [...routine, asked],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": capped });
    const { container } = renderHistory();
    await screen.findByText(/6 events since/);

    fireEvent.click(container.querySelector(".recap-burst summary")!);

    // A naive slice(0, ROW_CAP) would hide `asked` behind "1 more row" until
    // clicked - the exact drop the caps rule forbids, one level down from the
    // server's own group cap (api.ts's RecapGroup.omitted: "asks are never
    // capped"). It ships in the initial view instead, at the cost of one
    // routine entry's own slot.
    expect(container.querySelectorAll(".recap-entry")).toHaveLength(5);
    expect(container.querySelectorAll(".recap-entry-ask")).toHaveLength(1);
    expect(container.querySelector(".recap-more")?.textContent).toContain("1 more row");

    fireEvent.click(screen.getByRole("button", { name: /1 more row/ }));
    expect(container.querySelectorAll(".recap-entry")).toHaveLength(6);
  });
});

describe("History view: hoisting is gone", () => {
  // The hoisted ask block existed because the first ask sat at y=12,675 of a
  // 13,785px page. Collapsing makes the live page 42 rows, so nothing can be
  // buried; and the asks it hoisted were the SAME ids Home's answer-questions
  // card lists, one actionable set rendered twice in two grammars. It is the
  // duplication, not the height, that had to go.
  it("renders an ask exactly once, in the group and the timeline where it happened", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(container.querySelector(".recap-asks")).toBeNull();
    // Both groups render: the project-level cell is no longer emptied by
    // having its entries lifted out of it.
    expect(container.querySelectorAll(".recap-group")).toHaveLength(2);
    expect(screen.getByText("project")).toBeInTheDocument();

    const project = container.querySelectorAll(".recap-group")[1];
    expect(project.querySelectorAll(".recap-burst")).toHaveLength(1);
    expect(project.textContent).toContain("questions asked");
    // Once in the DOM, not twice: the statement lives behind the burst row.
    fireEvent.click(project.querySelector(".recap-burst summary")!);
    expect(screen.getAllByText("Question number 0?")).toHaveLength(1);
  });

  it("speaks in the past tense, so it cannot contradict the queue beside it", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    // "3 need you" was a present-tense claim about state, made from event
    // data, on a page whose numbers go stale the moment one is answered.
    expect(screen.queryByText(/need you/)).not.toBeInTheDocument();
    expect(container.querySelector(".recap-tally")?.textContent).toContain("3 asked you");
    // The live number is one gesture away, on the surface that owns it.
    expect(screen.getByRole("link", { name: /Needs you/ })).toHaveAttribute("href", "/");
  });

  it("inks the ask count on the burst row it belongs to, without reprinting the row's own numeral", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    const rows = container.querySelectorAll(".recap-burst");
    const row = rows[rows.length - 1];
    const inked = Array.from(container.querySelectorAll(".recap-burst .recap-asks-count"));
    expect(inked).toHaveLength(1);
    // Every one of the 3 asked, and the row's own bold "what" one line to the
    // left already says 3: "3 questions asked ... asked you" is the settled
    // wording (History.tsx's askClause comment) - the numeral does not repeat
    // a second time in the ask clause.
    expect(row.querySelector(".hrow .what b")?.textContent).toBe("3 questions asked");
    expect(inked[0].textContent).toBe("asked you");
  });

  it("keeps the numeral when only SOME of the run asked, because then it says something new", async () => {
    const mixed: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 3,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({ type: "spec.feature", ask: true, targets: [] }),
            recapEntry({ type: "spec.feature", ask: false, targets: [] }),
            recapEntry({ type: "spec.feature", ask: true, targets: [] }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": mixed });
    const { container } = renderHistory();
    await screen.findByRole("link", { name: "Audit Engine" });

    const row = container.querySelector(".recap-burst")!;
    expect(row.querySelector(".hrow .what b")?.textContent).toBe("3 features specced");
    expect(row.querySelector(".recap-asks-count")?.textContent).toBe("2 asked you");
  });

  it("marks individual asks only inside a MIXED burst, never on a burst that is all asks", async () => {
    const mixed: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 3,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({ type: "spec.feature", ask: true, targets: [] }),
            recapEntry({ type: "spec.feature", ask: false, targets: [] }),
            recapEntry({ type: "spec.feature", ask: true, targets: [] }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": mixed });

    const { container } = renderHistory();
    await screen.findByText(/features specced/);
    fireEvent.click(container.querySelector(".recap-burst summary")!);

    expect(container.querySelectorAll(".recap-entry")).toHaveLength(3);
    expect(container.querySelectorAll(".recap-entry-ask")).toHaveLength(2);

    // And the all-ask burst does not paint the same mark on every row, which
    // would be the repetition this whole pass exists to kill.
    cleanup();
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const second = renderHistory();
    await screen.findByText(/12 events/);
    fireEvent.click(second.container.querySelectorAll(".recap-group")[1].querySelector("summary")!);
    expect(second.container.querySelectorAll(".recap-entry-ask")).toHaveLength(0);
  });
});

describe("History view: the counts are true", () => {
  it("states the window's true total and every count the caps are measured against", async () => {
    const capped = loadedRecap();
    capped.groups[0].omitted = 15;
    capped.total = 27;
    stubFetch({ "/api/recap?window=visit": capped });

    const { container } = renderHistory();
    await screen.findByText(/27 events/);

    // total is the API's pre-cap number, not the 12 entries it served.
    expect(container.querySelector(".hist-sub")?.textContent).toContain("27 events");
    // The group head names 9 served plus 15 omitted, never just what it shows.
    expect(screen.getByText("24 events")).toBeInTheDocument();
    expect(screen.getByText("15 older not carried in this digest")).toBeInTheDocument();
  });

  it("says nothing about caps when nothing was capped", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });

    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(container.querySelector(".recap-omitted")).toBeNull();
    expect(
      Array.from(container.querySelectorAll(".recap-group-count"), (e) => e.textContent),
    ).toEqual(["9 events", "3 events"]);
  });

  it("names its one stream once in the head, not once per group", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });

    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(container.querySelector(".recap-tally")?.textContent).toContain("all by jc");
    expect(container.querySelectorAll(".recap-group-stream")).toHaveLength(0);
  });

  it("names the stream per group when more than one worked, which is the case worth drawing", async () => {
    const twoStreams = loadedRecap();
    twoStreams.groups[1].stream = "agent-1";
    stubFetch({ "/api/recap?window=visit": twoStreams });

    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(container.querySelector(".recap-tally")?.textContent).not.toContain("all by");
    expect(
      Array.from(container.querySelectorAll(".recap-group-stream"), (e) => e.textContent),
    ).toEqual(["· jc", "· agent-1"]);
  });
});

describe("History view: the row's own voices", () => {
  it("leads a group head with the epic's title and lets the key follow in muted mono", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });

    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    // Scoped to the head: the epic's own add.epic row names it too, which is
    // the point of the n=1 rule, so an unscoped query would be ambiguous.
    const head = container.querySelector(".recap-group-head") as HTMLElement;
    expect(within(head).getByRole("link", { name: "Audit Engine" })).toHaveAttribute(
      "href",
      "/node/epic/E-1",
    );
    const key = head.querySelector(".recap-group-key")!;
    expect(key.textContent).toBe("audit-engine");
    expect(key.className).toContain("mono");
    // ui-surface.md's console-scoped carve-out: the id never appears on a row.
    expect(head.textContent).not.toContain("E-1");
  });

  it("falls back to the key when an epic has no title, never rendering a blank", async () => {
    const untitled = loadedRecap();
    untitled.groups[0].epic = { key: "audit-engine", id: "E-1", ref: "audit-engine (E-1)" };
    stubFetch({ "/api/recap?window=visit": untitled });

    renderHistory();
    await screen.findByText(/12 events/);

    expect(screen.getByRole("link", { name: "audit-engine" })).toHaveAttribute(
      "href",
      "/node/epic/E-1",
    );
  });

  it("leads an expanded entry with its title and trails the LEAF key", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });

    const { container } = renderHistory();
    await screen.findByText(/12 events/);
    fireEvent.click(container.querySelector(".recap-burst summary")!);

    const row = container.querySelector(".recap-entry")!;
    expect(screen.getByRole("link", { name: "Feature 0" })).toHaveAttribute(
      "href",
      "/node/feature/F-0",
    );
    // The group head carries audit-engine/, so the row does not repeat it.
    expect(row.querySelector(".recap-entry-key")?.textContent).toBe("feature-0");
    expect(row.textContent).not.toContain("audit-engine/");
    expect(row.textContent).not.toContain("F-0");
  });

  it("leads an expanded question entry with its title and trails the mono Q-id", async () => {
    const questionTarget = {
      noun: "question",
      id: "Q-title",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
      ref: "Q-title",
    };
    const recap: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 2,
      groups: [
        {
          epic: null,
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({
              type: "add.question",
              ask: true,
              targets: [questionTarget],
            }),
            recapEntry({
              type: "add.question",
              ask: true,
              targets: [
                {
                  noun: "question",
                  id: "Q-second",
                  title: "Export retention window",
                  ref: "Q-second",
                },
              ],
            }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": recap });

    const { container } = renderHistory();
    await screen.findByText("2 questions asked");
    fireEvent.click(container.querySelector(".recap-burst summary")!);

    const row = container.querySelector(".recap-entry")!;
    expect(within(row as HTMLElement).getByRole("link", { name: QUESTION_TITLE })).toHaveAttribute(
      "href",
      "/node/question/Q-title",
    );
    const address = within(row as HTMLElement).getByText("Q-title");
    expect(address.className).toContain("mono");
    expect(row.textContent).not.toContain(QUESTION_STATEMENT);
  });

  it("falls back to the ref when a target has no name at all", async () => {
    // The last resort, kept honest: a dangling id has neither title nor key,
    // and its bare id is its only address, so the row shows it rather than
    // rendering an empty label.
    const dangling: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "visit",
      reader: "jc",
      total: 1,
      groups: [
        {
          epic: null,
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({ type: "cancel.feature", targets: [{ noun: "", id: "F-gone", ref: "F-gone" }] }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=visit": dangling });

    const { container } = renderHistory();
    await screen.findByText("project");

    expect(container.querySelector(".recap-burst-lone")?.textContent).toContain("F-gone");
    expect(screen.queryByRole("link", { name: "F-gone" })).not.toBeInTheDocument();
  });
});

// The topbar's ↻ (api.ts's bumpFetchGeneration, hooks.test.ts's own coverage
// of the mechanism at the hook level) re-issues whatever is mounted in place,
// and History's default window is the console's one marking read. That
// composition is deliberate, not an accident this view happens to allow: the
// controller's settled visit semantics (ui-surface.md's recap model, pinned
// server-side by internal/serve/visit_test.go's
// TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap) hold that a
// re-request inside the server's session gap is part of the same visit burst
// and never advances the horizon, exactly like a reload - so a ↻ click while
// History sits open is safe, and this pins that the client actually issues
// the re-request rather than silently no-op'ing on a still-mounted view.
describe("History view: the topbar ↻ re-issues the mounted visit-window recap", () => {
  it("re-requests window=visit on a generation bump, the same address the initial mount used", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });

    renderHistory();
    await screen.findByText("project");
    expect(spy).toHaveBeenCalledTimes(1);

    act(() => {
      bumpFetchGeneration();
    });

    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
    expect(String(spy.mock.calls[1][0])).toBe("/api/recap?window=visit");
  });
});

// Task 21 (phase 5): JC ruled the plan's (a)-vs-(b) grouping reconciliation
// with a third option, both, as a runtime toggle - epic sections stay the
// default, a flat day-grouped timeline is the second mode, and the reader
// picks live via ?group=. These tests own the toggle's own URL semantics
// (default, explicit, garbage, replace-not-push, and composing with the
// existing ?window= param in both directions) and flat mode's own rendering
// rules: the anti-merge pin, cross-epic interleaving with an origin label
// per row, and the caps rule holding without a new cap of its own.
describe("History view: the grouping toggle's own URL semantics", () => {
  it("renders both grouping options, defaulting to epic", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory();
    await screen.findByText(/12 events/);

    expect(screen.getByRole("link", { name: "by epic" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "by time" })).not.toHaveAttribute("aria-current");
    // Epic mode's own section wrapper renders; the flat wrapper does not.
    expect(container.querySelector(".recap-groups")).not.toBeNull();
    expect(container.querySelector(".recap-flat")).toBeNull();
  });

  it("reads an explicit ?group=epic exactly as the param being absent", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory("/history?group=epic");
    await screen.findByText(/12 events/);

    expect(screen.getByRole("link", { name: "by epic" })).toHaveAttribute("aria-current", "page");
    expect(container.querySelector(".recap-groups")).not.toBeNull();
    expect(container.querySelector(".recap-flat")).toBeNull();
  });

  it("reads ?group=time and renders the flat timeline instead of epic sections", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory("/history?group=time");
    await screen.findByText(/12 events/);

    expect(screen.getByRole("link", { name: "by time" })).toHaveAttribute("aria-current", "page");
    expect(container.querySelector(".recap-flat")).not.toBeNull();
    expect(container.querySelector(".recap-groups")).toBeNull();
  });

  it("degrades an unrecognized group value to epic rather than blanking the page", async () => {
    stubFetch({ "/api/recap?window=visit": loadedRecap() });
    const { container } = renderHistory("/history?group=nonsense");
    await screen.findByText(/12 events/);

    expect(screen.getByRole("link", { name: "by epic" })).toHaveAttribute("aria-current", "page");
    expect(container.querySelector(".recap-groups")).not.toBeNull();
  });

  it("switches grouping by navigating, replacing rather than pushing, with no refetch", async () => {
    const spy = stubFetch({ "/api/recap?window=visit": loadedRecap() });
    renderHistory();
    await screen.findByText(/12 events/);
    // The mount's own navigation is POP, the baseline the REPLACE assertion
    // has to beat (history.test.tsx's own established idiom, the window
    // switch's replace test above - this is the third use on this branch of
    // useNavigationType to pin replace-not-push, this time for the grouping
    // seg rather than the window one).
    expect(navType()).toBe("POP");

    fireEvent.click(screen.getByRole("link", { name: "by time" }));

    await waitFor(() => expect(screen.getByTestId("loc").textContent).toBe("/history?group=time"));
    expect(navType()).toBe("REPLACE");
    // Grouping is a client-only render decision - no server param exists for
    // it, so switching it must never trigger a second fetch of the same
    // payload the view already holds.
    expect(spy.mock.calls).toHaveLength(1);
  });

  it("keeps the window param when switching grouping", async () => {
    const week: Recap = { ...loadedRecap(), window: "7d", total: 90 };
    stubFetch({ "/api/recap?window=7d": week });
    renderHistory("/history?window=7d");
    await screen.findByText(/90 events/);

    fireEvent.click(screen.getByRole("link", { name: "by time" }));

    await waitFor(() =>
      expect(screen.getByTestId("loc").textContent).toBe("/history?window=7d&group=time"),
    );
  });

  it("keeps the group param when switching windows", async () => {
    const day: Recap = { ...loadedRecap(), window: "24h", total: 4 };
    stubFetch({ "/api/recap?window=visit": loadedRecap(), "/api/recap?window=24h": day });
    renderHistory("/history?group=time");
    await screen.findByText(/12 events/);

    fireEvent.click(screen.getByRole("link", { name: "24h" }));

    await waitFor(() =>
      expect(screen.getByTestId("loc").textContent).toBe("/history?window=24h&group=time"),
    );
  });
});

describe("History view: flat mode merges burst rows across epics by time", () => {
  it("interleaves rows from multiple epics newest-first, each carrying its own origin", async () => {
    // Four entries, one per calendar day, each stamped at the SAME UTC
    // hour (16:00) so a day boundary can never fall between any two of
    // them regardless of the runner's local timezone: a shift big enough to
    // roll one entry into an adjacent calendar day rolls EVERY entry the
    // identical way, since they all share one time-of-day, so the four stay
    // four distinct, sequential days in the same relative order no matter
    // where this suite runs (the same judgment the day-subhead tests above
    // apply with their own 5-day gaps, applied here with a shared clock
    // instead of a wide gap). The epics alternate day to day - audit-engine
    // leads days 1 and 3, form-engine days 2 and 4 - which is what proves a
    // genuine chronological merge rather than "every group's own days,
    // still grouped by epic": a merge that secretly sorted by epic first
    // would put both of audit-engine's days ahead of both of form-engine's.
    const spread: Recap = {
      since: "2026-07-20T00:00:00Z",
      window: "7d",
      reader: "jc",
      total: 4,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({ at: "2026-07-25T16:00:00Z", type: "start.task" }),
            recapEntry({ at: "2026-07-23T16:00:00Z", type: "sync.task" }),
          ],
        },
        {
          epic: { key: "form-engine", id: "E-2", title: "Form Engine", ref: "form-engine (E-2)" },
          stream: "pat",
          omitted: 0,
          entries: [
            recapEntry({ at: "2026-07-24T16:00:00Z", type: "submit.task" }),
            recapEntry({ at: "2026-07-22T16:00:00Z", type: "check.task" }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=7d": spread });

    const { container } = renderHistory("/history?window=7d&group=time");
    await screen.findByText(/4 events in the last 7 days/);

    const days = container.querySelectorAll(".day");
    expect(days).toHaveLength(4);
    // One burst per day here (each entry lands on its own calendar day), so
    // the origin ordering across days IS the row ordering.
    const origins = Array.from(container.querySelectorAll(".recap-burst-origin"), (e) => e.textContent);
    expect(origins).toEqual(["audit-engine", "form-engine", "audit-engine", "form-engine"]);
    // Newest first: started (Jul 25) leads, checked (Jul 22) trails.
    expect(days[0].textContent).toContain("started");
    expect(days[1].textContent).toContain("submitted");
    expect(days[2].textContent).toContain("synced");
    expect(days[3].textContent).toContain("checked");
  });
});

describe("History view: the anti-merge pin holds in flat mode", () => {
  it("keeps two epics' same-type-same-actor runs as two rows, never one claimed burst", async () => {
    // Same type (add.feature), same owner (jc), five minutes apart - the
    // exact shape burstsOf would fold into ONE "2 features added" row if it
    // ever ran again over the FLATTENED, cross-group list. It must not:
    // burstsOf only ever runs once per served group (flatBurstsOf's own
    // contract), so these stay two rows regardless of how close together
    // they land once merged.
    const adjacent: Recap = {
      since: "2026-07-24T00:00:00Z",
      window: "7d",
      reader: "jc",
      total: 2,
      groups: [
        {
          epic: { key: "audit-engine", id: "E-1", title: "Audit Engine", ref: "audit-engine (E-1)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({
              at: "2026-07-25T10:05:00Z",
              type: "add.feature",
              targets: [
                {
                  noun: "feature",
                  id: "F-1",
                  key: "audit-engine/feature-1",
                  title: "Feature One",
                  ref: "audit-engine/feature-1 (F-1)",
                },
              ],
            }),
          ],
        },
        {
          epic: { key: "form-engine", id: "E-2", title: "Form Engine", ref: "form-engine (E-2)" },
          stream: "jc",
          omitted: 0,
          entries: [
            recapEntry({
              at: "2026-07-25T10:00:00Z",
              type: "add.feature",
              targets: [
                {
                  noun: "feature",
                  id: "F-2",
                  key: "form-engine/feature-2",
                  title: "Feature Two",
                  ref: "form-engine/feature-2 (F-2)",
                },
              ],
            }),
          ],
        },
      ],
    };
    stubFetch({ "/api/recap?window=7d": adjacent });

    const { container } = renderHistory("/history?window=7d&group=time");
    await screen.findByText(/2 events in the last 7 days/);

    expect(container.querySelectorAll(".recap-burst")).toHaveLength(2);
    // The tell-tale merged phrasing never appears.
    expect(container.textContent).not.toContain("2 features added");
    expect(screen.getByRole("link", { name: "Feature One" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Feature Two" })).toBeInTheDocument();
    // Newest (10:05, audit-engine) leads.
    const origins = Array.from(container.querySelectorAll(".recap-burst-origin"), (e) => e.textContent);
    expect(origins).toEqual(["audit-engine", "form-engine"]);
  });
});

describe("History view: caps hold in flat mode without a new cap of its own", () => {
  it("aggregates every served group's own dropped count UNDER THE HEAD, and ships a capped group's ask anyway", async () => {
    const capped = loadedRecap();
    capped.groups[0].omitted = 15;
    capped.total = 27;
    stubFetch({ "/api/recap?window=visit": capped });

    const { container } = renderHistory("/history?group=time");
    await screen.findByText(/27 events/);

    // One aggregate line, beside .hist-sub/.recap-tally under the page head
    // - not a line per group at whichever day boundary it happens to fall
    // on, and not at the foot of the flat timeline either (task-21-review's
    // own correction: a line below every row a long digest holds is a line
    // a reader scanning the top never reaches, which fails the caps rule's
    // whole purpose). It is a direct child of the page section, a sibling
    // of .hist-sub and .recap-flat, never nested inside the timeline it
    // describes.
    expect(container.querySelector(".recap-flat .recap-omitted")).toBeNull();
    const dropped = container.querySelector(".history-page > .recap-omitted");
    expect(dropped?.textContent).toBe("15 older not carried in this digest, across 1 group");
    // And it precedes the timeline in document order, which is what "under
    // the head" actually means for a reader scanning top to bottom.
    const flat = container.querySelector(".recap-flat")!;
    expect(dropped!.compareDocumentPosition(flat) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    // The project group's three asks (loadedRecap's own fixture) still ship
    // as their own burst row, merged in like everything else - flat mode
    // introduces no cap of its own that could hide them.
    const askRow = Array.from(container.querySelectorAll(".recap-burst")).find((el) =>
      el.textContent?.includes("questions asked"),
    );
    expect(askRow).toBeDefined();
    expect(askRow?.querySelector(".recap-asks-count")?.textContent).toBe("asked you");
    expect(askRow?.querySelector(".recap-burst-origin")?.textContent).toBe("project");
  });

  it("pluralizes the aggregate line across more than one capped group", async () => {
    const capped = loadedRecap();
    capped.groups[0].omitted = 15;
    capped.groups[1].omitted = 2;
    capped.total = 29;
    stubFetch({ "/api/recap?window=visit": capped });

    const { container } = renderHistory("/history?group=time");
    await screen.findByText(/29 events/);

    const dropped = container.querySelector(".history-page > .recap-omitted");
    expect(dropped?.textContent).toBe("17 older not carried in this digest, across 2 groups");
  });

  it("renders no aggregate line in epic mode: each section keeps its own foot-of-section line", async () => {
    // The placement change is scoped to flat mode only - epic mode's
    // existing per-group lines (RecapGroupBlock's own .recap-omitted, at
    // each section's own foot) are unchanged by this task.
    const capped = loadedRecap();
    capped.groups[0].omitted = 15;
    capped.total = 27;
    stubFetch({ "/api/recap?window=visit": capped });

    const { container } = renderHistory();
    await screen.findByText(/27 events/);

    expect(container.querySelector(".history-page > .recap-omitted")).toBeNull();
    expect(container.querySelector(".recap-group .recap-omitted")?.textContent).toBe(
      "15 older not carried in this digest",
    );
  });
});
