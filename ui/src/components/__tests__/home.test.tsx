// Vitest/RTL coverage for the console v2 Home rebuild, against the real fetch
// surface (ProjectSummary as a prop the way App.tsx already supplies it; every
// other request Home's own useFetch calls issue), stubbing global fetch keyed
// by URL the same way this file always has, since MSW is not in this harness.
//
// The rebuild changed the needs-you column's own shape from one card per
// QUEUE KIND (a batch, disclosure rows, a "N more" reveal) to one card per
// QUEUE ENTRY with its command always visible, so DecisionCard's own tests
// below test the new shape directly rather than the old grouped one; the header
// band's stat tiles moved from .desk-stat to the mock's own .stat; the reserved
// .desk-stat-asof slot is gone (the freshness line lives in the topbar now,
// AsOf.test.tsx). What survives unchanged: the all-clear rule, the recap
// signal, and above all the horizon guard - Home must never request the marking
// recap window, pinned below and extended to the new pulse's own just-happened
// fetch.
//
// Task 8 (home burst redesign, settled decisions 8/9) replaces the header
// band's Features and Tasks stat tiles with a six-segment feature funnel
// (FunnelStrip, over the existing features/tasks fetches) and adds one
// combined Bugs & Chores link-tile (ShelfStat, over the existing bugs/chores
// fetches) in DriftStat's own link-tile pattern. No fetch changes: the four
// noun lists the funnel and shelf tile read were already part of Home's
// fetch surface before this task.

import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  BlockedItem,
  DriftReportView,
  ExecutionSnapshot,
  GatesItem,
  ListResult,
  NextAnswer,
  NextCandidate,
  ProjectSummary,
  Queue,
  QueueEntry,
  Recap,
  RecapPeek,
  StreamItem,
  Streams,
  UnclaimedItem,
} from "../../api";
import DecisionCard from "../DecisionCard";
import Home from "../../views/Home";
import { PeopleProvider } from "../../people";
import { ROW_CAP } from "../../lib/disclosure";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const QUESTION_TITLE = "Audit selection unit";
const QUESTION_STATEMENT =
  "Should the audit select one record per project or one record per project phase?\n\n" +
  "The answer changes how multi-year projects are counted in review queues and exported reports.";

function summary(overrides: Partial<ProjectSummary> = {}): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
    ...overrides,
  };
}

function emptyList(noun: string): ListResult {
  return { noun, count: 0, nodes: [] };
}

// A run of ListRows built from a status-count map, in the shape the funnel
// and shelf-tile fixtures below need: each id/ref is unique but otherwise
// arbitrary, since neither composition function reads anything but status.
function rowsFor(prefix: string, counts: Record<string, number>): ListResult["nodes"] {
  const nodes: ListResult["nodes"] = [];
  let i = 0;
  for (const [status, n] of Object.entries(counts)) {
    for (let j = 0; j < n; j++) {
      nodes.push({ id: `${prefix}-${i}`, status, ref: `${prefix.toLowerCase()}-${i} (${prefix}-${i})` });
      i++;
    }
  }
  return nodes;
}

// The counts-only read behind the recap signal. Home never fetches the digest
// itself: /api/recap?window=visit is the one request that advances the visit
// horizon, and landing on Home must not spend it.
function peek(overrides: Partial<RecapPeek> = {}): RecapPeek {
  return { since: "2026-07-24T00:00:00Z", reader: "jc", total: 0, asks: 0, ...overrides };
}

function emptyRecap(): Recap {
  return { since: "", window: "24h", reader: "jc", total: 0, groups: [] };
}

function emptyStreams(): Streams {
  return { contributors: [] };
}

// GET /api/next with nothing legal to do, ranked for jc: the settled desk
// every test sees unless it names its own list.
function emptyNext(): NextAnswer {
  return { reader: "jc", candidates: [], caught_up: true };
}

// A fetch stub keyed by URL: an unmapped path 404s the way fetchJSON expects,
// so a missing fixture fails loudly instead of silently returning undefined.
// The spy is returned so a test can assert on what was, and was not, requested.
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

// The all-zero shape composeDriftBand reads as "nothing to show".
function emptyDrift(): DriftReportView {
  return { compared: 0, uncovered: [] };
}

// The base route set every render needs beneath it: the four noun lists for
// the header band, the ready-features read, the recap signal's peek, the
// pulse's three reads (just-happened's fixed 24h window, the board streams,
// the ranked next list), and the drift slot's own fetch - all empty by default, so
// every pre-existing test here keeps seeing the same settled desk. Individual
// tests override just the routes their scenario cares about.
function baseRoutes(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/ls?noun=epic": emptyList("epic"),
    "/api/ls?noun=feature": emptyList("feature"),
    "/api/ls?noun=task": emptyList("task"),
    "/api/ls?noun=question": emptyList("question"),
    "/api/ls?noun=bug": emptyList("bug"),
    "/api/ls?noun=chore": emptyList("chore"),
    "/api/ls?noun=tweak": emptyList("tweak"),
    "/api/recap/peek": peek(),
    "/api/recap?window=24h": emptyRecap(),
    "/api/board": emptyStreams(),
    "/api/next": emptyNext(),
    "/api/drift": emptyDrift(),
    "/api/execution": { observations: [], diagnostics: [] },
    ...extra,
  };
}

// path lets a test land on Home with a query string, the way a link would;
// extra rides beside Home for a probe that reads the router's location.
function renderHome(s: ProjectSummary, path = "/", extra?: ReactNode) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={s} summaryError={null} />
      {extra}
    </MemoryRouter>,
  );
}

// ---------- The horizon guard: the invariant that cannot regress ----------

describe("Home horizon guard", () => {
  // The regression guard for the whole horizon argument: /api/recap?window=
  // visit (or the bare, defaulting-to-visit /api/recap) is the one request in
  // the console that advances the last-visit mark, so Home must never issue
  // it - not for the recap signal, and not for the pulse's new just-happened
  // column, which reads the FIXED 24h window instead. If either section ever
  // asked for the marking window, arriving here would empty the digest before
  // the human ever opened it, which is the bug the original move fixed.
  it("never fetches the marking recap window, only the fixed 24h window and the non-marking peek", async () => {
    const spy = stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary());
    await screen.findByText(/all clear/i);

    const urls = spy.mock.calls.map((c) => String(c[0]));
    expect(urls).toContain("/api/recap/peek");
    expect(urls).toContain("/api/recap?window=24h");
    expect(urls.filter((u) => u === "/api/recap" || u === "/api/recap?window=visit")).toEqual([]);
  });

  // /api/route is gone from the server (ranked next, build 2): Up next and
  // the dependency diagnostics read the ranked list, and nothing asks for the
  // old single-answer walk.
  it("reads the ranked next list and never the retired route walk", async () => {
    const spy = stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary());
    await screen.findByText(/all clear/i);

    const urls = spy.mock.calls.map((c) => String(c[0]));
    expect(urls).toContain("/api/next");
    expect(urls).not.toContain("/api/route");
  });
});

// ---------- Needs you: the card grid ----------

describe("Home needs-you: the card grid", () => {
  it("renders one card per queue entry, in gravity order, not served array order", async () => {
    const queue: Queue = {
      entries: [
        // Served (API) order puts approve_feature well before
        // attest_criterion (views/queue.go's QueueOf appends approvals long
        // before attestations); gravity flips that, attestations first. A
        // pass here proves real re-ranking rather than a lucky pass-through
        // of array order.
        {
          kind: "approve_feature",
          noun: "feature",
          id: "F-1",
          key: "foundation/identity-roles-program-model",
          ref: "foundation/identity-roles-program-model (F-1)",
          prose: "Identity, Roles & Program Model",
          command: "tiller approve feature foundation/identity-roles-program-model",
        },
        {
          kind: "attest_criterion",
          noun: "feature",
          id: "F-9",
          key: "form-engine/audit-export",
          ref: "form-engine/audit-export (F-9)",
          prose: "C6",
          command: "tiller attest feature form-engine/audit-export --criterion C6 --status pass",
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": queue }));

    const { container } = renderHome(summary());

    await screen.findByText("Attest · eval criterion");
    const cards = container.querySelectorAll(".dcard");
    expect(cards).toHaveLength(2);
    // The first card is the attest_criterion entry, ahead of approve_feature.
    expect(cards[0].querySelector(".kind")?.textContent).toBe("Attest · eval criterion");
    expect(cards[1].querySelector(".kind")?.textContent).toBe("Approve · feature plan");
  });

  it("sorts a proposed amendment ahead of every other kind, per ui-surface.md's settled rule", async () => {
    const queue: Queue = {
      entries: [
        {
          kind: "attest_gate",
          noun: "task",
          id: "T-1",
          ref: "T-1",
          prose: "Transcribe the schema",
          command: "tiller attest task T-1 --gate code_review --status pass",
          gate: "code_review",
          gate_status: "pending",
        },
        {
          kind: "approve_amendment",
          noun: "amendment",
          id: "A-1",
          key: "iphone-only",
          ref: "iphone-only (A-1)",
          prose: "Remove native iPad support from v1",
          command: "tiller approve amendment iphone-only",
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": queue }));

    const { container } = renderHome(summary());
    await screen.findByText("Approve · amendment");

    const cards = container.querySelectorAll(".dcard");
    expect(cards[0].querySelector(".kind")?.textContent).toBe("Approve · amendment");
  });

  it("renders every card's command visibly, with no click or expand required", async () => {
    const queue: Queue = {
      entries: [
        {
          kind: "approve_feature",
          noun: "feature",
          id: "F-1",
          key: "foundation/identity-roles-program-model",
          ref: "foundation/identity-roles-program-model (F-1)",
          prose: "Identity, Roles & Program Model",
          command: "tiller approve feature foundation/identity-roles-program-model",
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": queue }));

    renderHome(summary());

    // No fireEvent.click anywhere in this test: the command has to already be
    // in the document, the mock's own "every command rendered" anatomy.
    expect(
      await screen.findByText("tiller approve feature foundation/identity-roles-program-model"),
    ).toBeInTheDocument();
  });

  it("names the badge count from the number of cards, not the number of kinds", async () => {
    const queue: Queue = {
      entries: [
        {
          kind: "attest_criterion",
          noun: "feature",
          id: "F-9",
          key: "form-engine/audit-export",
          ref: "form-engine/audit-export (F-9)",
          prose: "C6",
          command: "tiller attest feature form-engine/audit-export --criterion C6 --status pass",
        },
        {
          kind: "attest_criterion",
          noun: "feature",
          id: "F-9",
          key: "form-engine/audit-export",
          ref: "form-engine/audit-export (F-9)",
          prose: "C7",
          command: "tiller attest feature form-engine/audit-export --criterion C7 --status pass",
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": queue }));

    renderHome(summary());

    expect(await screen.findByText("2 decisions")).toBeInTheDocument();
  });

  it("renders shelf queue cards with commands", async () => {
    const queue: Queue = {
      entries: [
        {
          kind: "approve_bug",
          noun: "bug",
          id: "B-a",
          key: "bugs/a",
          ref: "bugs/a (B-a)",
          prose: "Triage bug",
          command: "tiller approve bug bugs/a",
        },
        {
          kind: "approve_chore",
          noun: "chore",
          id: "CH-a",
          key: "chores/a",
          ref: "chores/a (CH-a)",
          prose: "Triage chore",
          command: "tiller approve chore chores/a",
        },
        {
          kind: "carrier_cancelled",
          noun: "bug",
          id: "B-b",
          key: "bugs/b",
          ref: "bugs/b (B-b)",
          prose: "Lost carrier",
          command: "tiller resolve bug bugs/b --clear",
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": queue }));

    renderHome(summary());

    expect(await screen.findByText("Approve · bug triage")).toBeInTheDocument();
    expect(screen.getByText("Approve · chore triage")).toBeInTheDocument();
    expect(screen.getByText("Resolve · cancelled carrier")).toBeInTheDocument();
    expect(screen.getAllByText("Captured work is waiting on your triage.")).toHaveLength(2);
    expect(
      screen.getByText("The recorded carrier was cancelled; clear it or choose a replacement."),
    ).toBeInTheDocument();
    expect(screen.getByText("tiller approve bug bugs/a")).toBeInTheDocument();
    expect(screen.getByText("tiller approve chore chores/a")).toBeInTheDocument();
    expect(screen.getByText("tiller resolve bug bugs/b --clear")).toBeInTheDocument();
  });
});

// ---------- Needs you: decision bands ----------
//
// A kind with more than ROW_CAP entries compresses into one DecisionBand
// (home burst redesign, settled decisions 1-4): every member still renders
// as a row, but the kind label and the why-line, when every row shares one,
// paint once in the head instead of once per row, and a per-kind batch
// command replaces the per-row commands in a single foot. A group at or
// under ROW_CAP stays exactly the settled .dcard grid; these fixtures use
// ROW_CAP-relative counts so they track the constant rather than a copied
// literal.

function epicEntry(n: number): QueueEntry {
  return {
    kind: "approve_epic",
    noun: "epic",
    id: `E-${n}`,
    key: `epic-${n}`,
    ref: `epic-${n} (E-${n})`,
    prose: `Epic ${n}`,
    command: `tiller approve epic epic-${n}`,
  };
}

function questionEntry(n: number, due?: string): QueueEntry {
  return {
    kind: "answer_question",
    noun: "question",
    id: `Q-${n}`,
    ref: `Q-${n}`,
    prose: `Question ${n}`,
    command: `tiller answer question Q-${n} --answer "..."`,
    ...(due !== undefined ? { due } : {}),
  };
}

function amendmentEntry(): QueueEntry {
  return {
    kind: "approve_amendment",
    noun: "amendment",
    id: "A-1",
    key: "iphone-only",
    ref: "iphone-only (A-1)",
    prose: "Remove native iPad support from v1",
    command: "tiller approve amendment iphone-only",
  };
}

describe("Home needs-you: decision bands", () => {
  it("compresses more than ROW_CAP entries of one kind: kind and why once, every row, no per-row command, a batch foot", async () => {
    const count = ROW_CAP + 2;
    const entries = Array.from({ length: count }, (_, i) => epicEntry(i + 1));
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    expect(container.querySelectorAll(".dband")).toHaveLength(1);
    expect(container.querySelectorAll(".dcard")).toHaveLength(0);
    const band = container.querySelector(".dband") as HTMLElement;

    // The kind label paints once, in the head.
    expect(within(band).getAllByText("Approve · epic decomposition")).toHaveLength(1);
    expect(band.querySelector(".dband-count")?.textContent).toBe(String(count));

    // The shared why-line paints once, in the head.
    expect(
      within(band).getAllByText("Decomposition proposed and ready for your review."),
    ).toHaveLength(1);

    // Every member renders as a row, each linking to its own node.
    const rows = band.querySelectorAll(".dband-row");
    expect(rows).toHaveLength(count);
    entries.forEach((entry, i) => {
      const link = within(rows[i] as HTMLElement).getByRole("link", { name: entry.prose });
      expect(link).toHaveAttribute("href", `/node/${entry.noun}/${entry.id}`);
    });

    // No per-row command anywhere in the band's rows.
    expect(band.querySelectorAll(".dband-row .copy-cmd")).toHaveLength(0);

    // The foot's one batch command, captioned after-review, never a headline.
    const addrs = entries.map((e) => e.key).join(" ");
    expect(within(band).getByText(`tiller approve epic ${addrs}`)).toBeInTheDocument();
    expect(within(band).getByText(`approve all ${count} after review`)).toBeInTheDocument();

    // And it sits AFTER the rows, never above them (the
    // batch command follows the review, it is not the band's headline).
    const foot = band.querySelector(".dband-foot") as HTMLElement;
    const lastRow = rows[rows.length - 1];
    expect(lastRow.compareDocumentPosition(foot) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("renders more than ROW_CAP answer_question rows with title, mono id, and due where present, and no foot (no batch verb)", async () => {
    const count = ROW_CAP + 4;
    const entries = Array.from({ length: count }, (_, i) =>
      questionEntry(i + 1, i % 2 === 0 ? `2026-09-0${i + 1}` : undefined),
    );
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    const band = container.querySelector(".dband") as HTMLElement;
    expect(band).not.toBeNull();
    const rows = band.querySelectorAll(".dband-row");
    expect(rows).toHaveLength(count);

    entries.forEach((entry, i) => {
      const row = rows[i] as HTMLElement;
      expect(within(row).getByRole("link", { name: entry.prose })).toBeInTheDocument();
      expect(row.querySelector(".mono.muted")?.textContent?.trim()).toBe(entry.id);
      if (entry.due !== undefined) {
        expect(row.textContent).toContain(`due ${entry.due}`);
      } else {
        expect(row.textContent).not.toContain("due ");
      }
    });

    expect(band.querySelector(".dband-foot")).not.toBeInTheDocument();
  });

  it("keeps a group at or under ROW_CAP as the plain card grid, no band", async () => {
    const count = ROW_CAP - 2;
    const entries = Array.from({ length: count }, (_, i) => epicEntry(i + 1));
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    expect(container.querySelectorAll(".dcard")).toHaveLength(count);
    expect(container.querySelectorAll(".dband")).toHaveLength(0);
  });

  it("keeps an amendment burst as command-bearing cards past the row cap", async () => {
    const count = ROW_CAP + 2;
    const entries = Array.from({ length: count }, (_, i) => {
      const n = i + 1;
      return {
        ...amendmentEntry(),
        id: `A-${n}`,
        key: `amendment-${n}`,
        ref: `amendment-${n} (A-${n})`,
        prose: `Amendment ${n}`,
        command: `tiller approve amendment amendment-${n}`,
      };
    });
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    expect(container.querySelectorAll(".dband")).toHaveLength(0);
    expect(container.querySelectorAll(".dcard")).toHaveLength(count);
    expect(screen.getAllByText(/^tiller approve amendment amendment-/)).toHaveLength(count);
  });

  it("keeps a review-gate attestation burst as cards, each command tied to its node and gate", async () => {
    const count = ROW_CAP + 2;
    const gates = ["code-review", "security-review"];
    const entries: QueueEntry[] = Array.from({ length: count }, (_, i) => {
      const n = Math.floor(i / 2) + 1;
      const gate = gates[i % 2];
      return {
        kind: "attest_gate",
        noun: "task",
        id: `T-${n}`,
        key: `task-${n}`,
        ref: `task-${n} (T-${n})`,
        prose: `Task ${n}`,
        gate,
        gate_status: "pending",
        command: `tiller attest task task-${n} --gate ${gate} --status pass`,
      };
    });
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    expect(container.querySelectorAll(".dband")).toHaveLength(0);
    const cards = container.querySelectorAll(".dcard");
    expect(cards).toHaveLength(count);
    entries.forEach((entry, i) => {
      const card = cards[i] as HTMLElement;
      expect(within(card).getByRole("link", { name: entry.prose })).toHaveAttribute(
        "href",
        `/node/task/${entry.id}`,
      );
      expect(card.textContent).toContain(entry.gate);
      expect(within(card).getByText(entry.command)).toBeInTheDocument();
    });
  });

  it("keeps an eval-criterion attestation burst as cards, each command tied to its node and criterion", async () => {
    const count = ROW_CAP + 2;
    const entries: QueueEntry[] = Array.from({ length: count }, (_, i) => ({
      kind: "attest_criterion",
      noun: "feature",
      id: `F-${i + 1}`,
      key: `epic/feature-${i + 1}`,
      ref: `epic/feature-${i + 1} (F-${i + 1})`,
      prose: `C${i + 1}`,
      command: `tiller attest feature epic/feature-${i + 1} --criterion C${i + 1} --status pass`,
    }));
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${count} decisions`);

    expect(container.querySelectorAll(".dband")).toHaveLength(0);
    const cards = container.querySelectorAll(".dcard");
    expect(cards).toHaveLength(count);
    entries.forEach((entry, i) => {
      const card = cards[i] as HTMLElement;
      expect(card.textContent).toContain(`Criterion ${entry.prose}`);
      expect(within(card).getByText(entry.command)).toBeInTheDocument();
    });
  });

  it("keeps gravity order across kinds past the cap: the amendment tab leads, the compressed epic band sits behind the second tab", async () => {
    const epicCount = ROW_CAP + 2;
    const entries = [amendmentEntry(), ...Array.from({ length: epicCount }, (_, i) => epicEntry(i + 1))];
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${entries.length} decisions`);

    // Two kinds past the cap: a rail, the amendment first.
    const rail = screen.getByRole("navigation", { name: "Decision kind" });
    const tabs = within(rail).getAllByRole("button");
    expect(tabs.map((t) => t.textContent?.replace(/\s+/g, " ").trim())).toEqual([
      "Approve · amendment · 1",
      `Approve · epic decomposition · ${epicCount}`,
    ]);
    expect(container.querySelectorAll(".dcard")).toHaveLength(1);
    expect(container.querySelector(".dcard .kind")?.textContent).toBe("Approve · amendment");
    expect(container.querySelectorAll(".dband")).toHaveLength(0);

    fireEvent.click(tabs[1]);
    await waitFor(() => expect(container.querySelectorAll(".dband")).toHaveLength(1));
    expect(container.querySelectorAll(".dcard")).toHaveLength(0);
    expect(container.querySelectorAll(".dband-row")).toHaveLength(epicCount);

    // Under its tab the band drops the head's kind line and count, which the
    // tab just said, and keeps the shared why-line, which the tab did not.
    const band = container.querySelector(".dband") as HTMLElement;
    expect(band.querySelector(".dband-head .kind")).toBeNull();
    expect(band.querySelector(".dband-count")).toBeNull();
    expect(within(band).getByText("Decomposition proposed and ready for your review.")).toBeInTheDocument();
  });

  it("lays each kind out as its own column of the needs-you body at the cap, in gravity order", async () => {
    // Kinds used to pool into one shared card grid (JC, 2026-08-31). Now,
    // at the cap or under, the body is a grid with one column per kind;
    // past the cap with several kinds the columns give way to the tab rail
    // (the tests below).
    const entries = [amendmentEntry(), epicEntry(1), epicEntry(2), questionEntry(1), questionEntry(2)];
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${entries.length} decisions`);

    expect(screen.queryByRole("navigation", { name: "Decision kind" })).toBeNull();
    const body = container.querySelector(".needs-body") as HTMLElement;
    const columns = [...body.children] as HTMLElement[];
    expect(columns.map((c) => c.getAttribute("data-kind"))).toEqual(["approve_amendment", "approve_epic", "answer_question"]);
    expect(columns.every((c) => c.classList.contains("card-grid"))).toBe(true);
    expect(container.querySelectorAll(".dcard")).toHaveLength(entries.length);
  });

  it("stacks a band row as a title line over an address line, so a narrow column still reads", async () => {
    const entries = Array.from({ length: ROW_CAP + 2 }, (_, i) => epicEntry(i + 1));
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container } = renderHome(summary());
    await screen.findByText(`${entries.length} decisions`);

    const row = container.querySelector(".dband-row") as HTMLElement;
    const title = row.querySelector(".dband-row-title") as HTMLElement;
    const addr = row.querySelector(".dband-row-addr") as HTMLElement;
    expect(within(title).getByRole("link", { name: entries[0].prose })).toBeInTheDocument();
    expect(addr.textContent?.trim()).toBe(entries[0].key);
  });

  it("tabs the kinds past ROW_CAP decisions: every kind named with its count, one kind shown at a time, the choice on the URL", async () => {
    // Seven decisions across three kinds: the desk shows a tab rail, the
    // highest-gravity kind open, and the other two one click away, each
    // label carrying its count so nothing on the queue goes silent (JC,
    // 2026-09-01, the scale-aware variant).
    const entries = [
      amendmentEntry(),
      ...Array.from({ length: 3 }, (_, i) => epicEntry(i + 1)),
      ...Array.from({ length: 3 }, (_, i) => questionEntry(i + 1)),
    ];
    stubFetch(baseRoutes({ "/api/queue": { entries } }));
    let search = "";
    function Probe() {
      search = useLocation().search;
      return null;
    }

    const { container } = renderHome(summary(), "/", <Probe />);
    await screen.findByText("7 decisions");

    const rail = screen.getByRole("navigation", { name: "Decision kind" });
    const tabs = within(rail).getAllByRole("button");
    expect(tabs.map((t) => t.textContent?.replace(/\s+/g, " ").trim())).toEqual([
      "Approve · amendment · 1",
      "Approve · epic decomposition · 3",
      "Answer · question · 3",
    ]);
    expect(tabs[0]).toHaveAttribute("aria-current", "true");
    expect(container.querySelectorAll(".dcard")).toHaveLength(1);
    expect(container.querySelector(".dcard .kind")?.textContent).toBe("Approve · amendment");

    fireEvent.click(tabs[1]);
    await waitFor(() => expect(container.querySelectorAll(".dcard")).toHaveLength(3));
    expect(container.querySelector(".dcard .kind")?.textContent).toBe("Approve · epic decomposition");
    expect(within(rail).getByRole("button", { name: /Approve · epic decomposition/ })).toHaveAttribute("aria-current", "true");
    expect(new URLSearchParams(search).get("needs")).toBe("approve_epic");
  });

  it("lands on the kind a link names, and falls back to the first kind for one that is not on the queue", async () => {
    const entries = [
      amendmentEntry(),
      ...Array.from({ length: 3 }, (_, i) => epicEntry(i + 1)),
      ...Array.from({ length: 3 }, (_, i) => questionEntry(i + 1)),
    ];
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    const { container, unmount } = renderHome(summary(), "/?needs=answer_question");
    await screen.findByText("7 decisions");
    expect(container.querySelectorAll(".dcard")).toHaveLength(3);
    expect(container.querySelector(".dcard .kind")?.textContent).toBe("Answer · question");
    unmount();

    stubFetch(baseRoutes({ "/api/queue": { entries } }));
    const again = renderHome(summary(), "/?needs=approve_bug");
    await screen.findByText("7 decisions");
    expect(again.container.querySelector(".dcard .kind")?.textContent).toBe("Approve · amendment");
  });

  it("shows no tab rail at ROW_CAP or under, or for a single kind past it", async () => {
    const small = [amendmentEntry(), epicEntry(1), questionEntry(1)];
    stubFetch(baseRoutes({ "/api/queue": { entries: small } }));
    const { container, unmount } = renderHome(summary());
    await screen.findByText("3 decisions");
    expect(screen.queryByRole("navigation", { name: "Decision kind" })).toBeNull();
    expect(container.querySelectorAll(".dcard")).toHaveLength(3);
    unmount();

    const oneKind = Array.from({ length: ROW_CAP + 2 }, (_, i) => epicEntry(i + 1));
    stubFetch(baseRoutes({ "/api/queue": { entries: oneKind } }));
    const again = renderHome(summary());
    await screen.findByText(`${oneKind.length} decisions`);
    expect(screen.queryByRole("navigation", { name: "Decision kind" })).toBeNull();
    expect(again.container.querySelectorAll(".dband")).toHaveLength(1);
  });

  it("names the needs badge from every entry inside a band, not from the band itself (46-style, extended to bands)", async () => {
    const epicCount = ROW_CAP + 2;
    const entries = [amendmentEntry(), ...Array.from({ length: epicCount }, (_, i) => epicEntry(i + 1))];
    const total = entries.length;
    stubFetch(baseRoutes({ "/api/queue": { entries } }));

    renderHome(summary());

    expect(await screen.findByText(`${total} decisions`)).toBeInTheDocument();
  });
});

// ---------- DecisionCard: the new one-entry-per-card shape ----------

describe("DecisionCard", () => {
  it("tells four attest_gate rows on one node apart by gate name, all visible at once", () => {
    const gates = ["code_review", "security_review", "performance_review", "accessibility_review"];
    const entries: QueueEntry[] = gates.map((gate) => ({
      kind: "attest_gate",
      noun: "task",
      id: "T-aaaaaa",
      key: "form-engine/certification-form-loading/transcribe-the-schema",
      ref: "T-aaaaaa",
      prose: "Transcribe the schema",
      command: `tiller attest task form-engine/certification-form-loading/transcribe-the-schema --gate ${gate} --status pass`,
      gate,
      gate_status: "pending",
    }));

    const { container } = render(
      <MemoryRouter>
        {entries.map((e) => (
          <DecisionCard key={e.gate} entry={e} />
        ))}
      </MemoryRouter>,
    );

    // Four cards, none expanded (there is no expand step left at all), each
    // one's own gate name and full command readable straight off it.
    const cards = container.querySelectorAll(".dcard");
    expect(cards).toHaveLength(4);
    gates.forEach((gate, i) => {
      expect(cards[i].querySelector(".who")?.textContent).toContain(gate);
      expect(cards[i].querySelector(".copy-cmd code")?.textContent).toContain(`--gate ${gate} --status pass`);
    });
  });

  it("renders feature_check's kind, an active state has no bearing here (no dot on a dcard), and its context line", () => {
    const entry: QueueEntry = {
      kind: "feature_check",
      noun: "feature",
      id: "F-dhzdb2",
      key: "form-engine/schema-engine",
      ref: "form-engine/schema-engine (F-dhzdb2)",
      prose: "Schema engine",
      command: "tiller check feature form-engine/schema-engine",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Run · parked feature check")).toBeInTheDocument();
    expect(screen.getByText("Schema engine")).toBeInTheDocument();
    expect(
      screen.getByText("Every task done; the required feature gate has no recorded result. The rollup is parked until this runs."),
    ).toBeInTheDocument();
    expect(screen.getByText("tiller check feature form-engine/schema-engine")).toBeInTheDocument();
  });

  // Review Grain build, Task 8: merge_feature and merge_epic are the queue's
  // own coarse-grain groups (views/queue.go's queueMergeFeatures/
  // queueMergeEpics) - nothing left to decide, only to merge, so the card
  // names the reviewed grain and renders `tiller sync` verbatim, the one
  // command that observes any merge regardless of grain.
  it("renders a merge_feature entry naming the feature PR and tiller sync", () => {
    const entry: QueueEntry = {
      kind: "merge_feature",
      noun: "feature",
      id: "F-9",
      key: "form-engine/audit-export",
      ref: "form-engine/audit-export (F-9)",
      prose: "PR #42",
      command: "tiller sync",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Merge · feature PR")).toBeInTheDocument();
    expect(screen.getByText("PR #42")).toBeInTheDocument();
    expect(
      screen.getByText("The feature PR is open; merging it is the done crossing."),
    ).toBeInTheDocument();
    expect(screen.getByText("tiller sync")).toBeInTheDocument();
  });

  it("renders a merge_epic entry naming the epic PR and tiller sync", () => {
    const entry: QueueEntry = {
      kind: "merge_epic",
      noun: "epic",
      id: "E-9",
      key: "form-engine",
      ref: "form-engine (E-9)",
      prose: "branch tiller/form-engine",
      command: "tiller sync",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Merge · epic PR")).toBeInTheDocument();
    expect(screen.getByText("branch tiller/form-engine")).toBeInTheDocument();
    expect(
      screen.getByText("The epic PR is open; merging it is the done crossing."),
    ).toBeInTheDocument();
    expect(screen.getByText("tiller sync")).toBeInTheDocument();
  });

  it("renders an amendment card's own command directly, with no batch footer", () => {
    const entry: QueueEntry = {
      kind: "approve_amendment",
      noun: "amendment",
      id: "A-000001",
      key: "iphone-only",
      ref: "iphone-only (A-000001)",
      prose: "Remove native iPad support from v1",
      command: "tiller approve amendment iphone-only",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Approve · amendment")).toBeInTheDocument();
    expect(screen.getByText("Remove native iPad support from v1")).toBeInTheDocument();
    expect(screen.getByText("tiller approve amendment iphone-only")).toBeInTheDocument();
    expect(screen.queryByText(/Approve all/)).not.toBeInTheDocument();
  });

  it("falls back to the node's reference for attest_criterion's who line, since Prose there is the criterion id, not a title", () => {
    const entry: QueueEntry = {
      kind: "attest_criterion",
      noun: "feature",
      id: "F-9",
      key: "intake/applicant-portal-auth",
      ref: "intake/applicant-portal-auth (F-9)",
      prose: "C3",
      command: "tiller attest feature intake/applicant-portal-auth --criterion C3 --status pass",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Attest · eval criterion")).toBeInTheDocument();
    // No title field exists on this kind's payload (queue.go: Prose IS the
    // criterion id here), so the who line leads with the node's own ref
    // rather than inventing a title, and names which criterion in a
    // qualifier.
    expect(screen.getByText(/intake\/applicant-portal-auth \(F-9\)/)).toBeInTheDocument();
    expect(screen.getByText(/Criterion C3/)).toBeInTheDocument();
  });

  it("leads an answer_question card with its title and keeps the question id visible", () => {
    const entry = {
      kind: "answer_question",
      noun: "question",
      id: "Q-h2n7m5",
      ref: "Q-h2n7m5",
      prose: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
      command: 'tiller answer question Q-h2n7m5 --answer "..."',
      due: "Friday",
    };

    const { container } = render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Answer · question")).toBeInTheDocument();
    const who = container.querySelector(".who") as HTMLElement;
    expect(who.firstElementChild).toBe(screen.getByRole("link", { name: QUESTION_TITLE }));
    expect(within(who).getByText("Q-h2n7m5")).toBeInTheDocument();
    expect(screen.queryByText((_, element) => element?.textContent === QUESTION_STATEMENT)).not.toBeInTheDocument();
    expect(screen.getByText("Due Friday.")).toBeInTheDocument();
  });

  // The earlier disclosure row linked its title to
  // the node's own page; the rebuild's who-line dropped it. ui-surface.md:59
  // ("Each row opens the in-context review... with the action in reach")
  // still applies to a card that never collapses - restored via RefLink.
  it("links the card's own title to the node's page", () => {
    const entry: QueueEntry = {
      kind: "approve_feature",
      noun: "feature",
      id: "F-1",
      key: "foundation/identity-roles-program-model",
      ref: "foundation/identity-roles-program-model (F-1)",
      prose: "Identity, Roles & Program Model",
      command: "tiller approve feature foundation/identity-roles-program-model",
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", { name: "Identity, Roles & Program Model" });
    expect(link).toHaveAttribute("href", "/node/feature/F-1");
  });

  it("links a question card's title to its own node page", () => {
    const entry: QueueEntry = {
      kind: "answer_question",
      noun: "question",
      id: "Q-h2n7m5",
      ref: "Q-h2n7m5",
      prose: QUESTION_TITLE,
      command: 'tiller answer question Q-h2n7m5 --answer "..."',
    };

    render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", { name: QUESTION_TITLE });
    expect(link).toHaveAttribute("href", "/node/question/Q-h2n7m5");
  });

  it("bounds a long explicit question title without allowing the clamp to hide its id", () => {
    const longTitle =
      "Determine whether the audit selection unit is a project or a project phase when one project " +
      "crosses multiple program years, contains several separately reviewed eligibility periods, " +
      "and must remain traceable through both the queue and the exported report without losing its address";
    const entry: QueueEntry = {
      kind: "answer_question",
      noun: "question",
      id: "Q-h2n7m5",
      ref: "Q-h2n7m5",
      prose: longTitle,
      command: 'tiller answer question Q-h2n7m5 --answer "..."',
    };

    const { container } = render(
      <MemoryRouter>
        <DecisionCard entry={entry} />
      </MemoryRouter>,
    );

    const who = container.querySelector(".who") as HTMLElement;
    const title = screen.getByRole("link", { name: longTitle });
    const id = within(who).getByText("Q-h2n7m5");
    expect(who.className).not.toContain("clamp");
    expect(title.className).toContain("clamp");
    expect(title.nextElementSibling).toBe(id);
    expect(id.className).toContain("mono");
  });
});

// ---------- All clear (unchanged rule, exercised through the rebuilt view) ----------

describe("Home needs-you: empty queue", () => {
  it("renders AllClear with the ownership line and enumerates in-review nodes", async () => {
    const queue: Queue = { entries: [] };
    const features: ListResult = {
      noun: "feature",
      count: 1,
      nodes: [
        {
          id: "F-9",
          key: "form-engine/audit-export",
          title: "Audit export",
          status: "in_review",
          ref: "form-engine/audit-export (F-9)",
        },
      ],
    };
    stubFetch(
      baseRoutes({
        "/api/queue": queue,
        "/api/ls?noun=feature": features,
      }),
    );

    renderHome(summary({ whose_move: { human: 3, agent: 5 } }));

    await screen.findByText(/all clear/i);
    expect(
      screen.getByText("5 nodes move with the agent next · 3 nodes sit on a human gate elsewhere in the tree."),
    ).toBeInTheDocument();

    const link = screen.getByRole("link", { name: "Audit export" });
    expect(link).toHaveAttribute("href", "/node/feature/F-9");
    // Scoped to the review block itself: the default route fixture's own
    // reason text ("no task is in review") also matches a bare /in review/i,
    // which is a fixture collision, not a real ambiguity in what the page
    // renders.
    const reviewBlock = document.querySelector(".all-clear-review");
    expect(reviewBlock).not.toBeNull();
    expect(within(reviewBlock as HTMLElement).getByText(/in review/i)).toBeInTheDocument();
  });

  it("does not claim all-clear while shelf nodes are in review", async () => {
    const bugs: ListResult = {
      noun: "bug",
      count: 1,
      nodes: [
        { id: "B-a", key: "bugs/a", title: "Bug A", status: "in_review", ref: "bugs/a (B-a)" },
      ],
    };
    const chores: ListResult = {
      noun: "chore",
      count: 1,
      nodes: [
        {
          id: "CH-a",
          key: "chores/a",
          title: "Chore A",
          status: "in_review",
          ref: "chores/a (CH-a)",
        },
      ],
    };
    stubFetch(
      baseRoutes({
        "/api/queue": { entries: [] },
        "/api/ls?noun=bug": bugs,
        "/api/ls?noun=chore": chores,
      }),
    );

    renderHome(summary());

    expect((await screen.findByRole("link", { name: "Bug A" }))).toHaveAttribute(
      "href",
      "/node/bug/B-a",
    );
    expect(screen.getByRole("link", { name: "Chore A" })).toHaveAttribute(
      "href",
      "/node/chore/CH-a",
    );
    expect(screen.queryByText("Nothing needs you")).not.toBeInTheDocument();
  });

  it("replaces the all-clear with an honest error surface when a list fetch fails", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/ls?noun=feature"];
    stubFetch(routes);

    renderHome(summary());

    expect(await screen.findByText(/GET \/api\/ls\?noun=feature failed \(404 Not Found\)/)).toBeInTheDocument();
    expect(screen.getByText(/cannot say nothing needs you/i)).toBeInTheDocument();
    expect(screen.queryByText(/all clear/i)).not.toBeInTheDocument();
  });

  // Restored, adapted to scope the "in review"
  // absence to the all-clear block itself - a global queryByText would also
  // have to reckon with the default route fixture's own "no task is in
  // review" reason text, a fixture collision rather than a real ambiguity.
  it("renders AllClear with no review section when nothing is in review", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary({ whose_move: { human: 0, agent: 0 } }));

    await screen.findByText(/all clear/i);
    expect(document.querySelector(".all-clear-review")).toBeNull();
  });

  it("keeps noun and verb agreement in the singular ownership line", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary({ whose_move: { human: 1, agent: 1 } }));

    expect(
      await screen.findByText("1 node moves with the agent next · 1 node sits on a human gate elsewhere in the tree."),
    ).toBeInTheDocument();
  });

  // The adjudicated all-clear rule's other half, restored: "nothing needs you"
  // is a claim about the four lists that back the in-review enumeration, so it
  // can only be made once all four have actually loaded. useFetch nulls data on
  // error, but a still-PENDING list needs its own guard too, or an empty queue
  // plus one slow /api/ls would flash the all-clear claim before the list it
  // depends on has even answered.
  it("holds at loading, never a premature all-clear, while a list fetch is pending", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    const pendingPath = "/api/ls?noun=feature";
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const url = String(input);
        if (url === pendingPath) {
          // Never settles: the list is still in flight.
          return new Promise<never>(() => {
            // deliberately unresolved
          });
        }
        if (url in routes) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
        }
        return Promise.resolve({
          ok: false,
          status: 404,
          statusText: "Not Found",
          text: () => Promise.resolve(""),
        });
      }),
    );

    renderHome(summary());

    expect(await screen.findByText("Loading project state...")).toBeInTheDocument();
    expect(screen.queryByText(/all clear/i)).not.toBeInTheDocument();
  });
});

// ---------- Header band ----------

describe("Home header band", () => {
  // Task 8: the Features stat tile is gone, replaced by the six-segment
  // funnel (FunnelStrip). Every segment renders, zeros included, in
  // FEATURE_FUNNEL order, each a /build link; only the zero segment (active,
  // here) carries the dim class. features.data.nodes.length (102, not the
  // sum of the six segments) is what the caption's feature count names.
  it("renders the feature funnel with six segments in order, zeros dim, each linking to /build", async () => {
    const features: ListResult = {
      noun: "feature",
      count: 102,
      nodes: rowsFor("F", { draft: 40, specced: 20, ready: 15, active: 0, in_review: 12, done: 15 }),
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": features }));

    const { container } = renderHome(summary());

    await screen.findByText("0 tasks across 102 features");

    const segs = [...container.querySelectorAll(".funnel-row .fseg")];
    expect(segs.map((el) => el.querySelector(".lbl")?.textContent)).toEqual([
      "draft",
      "specced",
      "ready",
      "active",
      "in review",
      "done",
    ]);
    expect(segs.map((el) => el.querySelector(".n")?.textContent)).toEqual(["40", "20", "15", "0", "12", "15"]);
    segs.forEach((el) => expect(el).toHaveAttribute("href", "/build"));
    const dimmed = segs.filter((el) => el.className.split(" ").includes("dim"));
    expect(dimmed).toHaveLength(1);
    expect(dimmed[0].querySelector(".lbl")?.textContent).toBe("active");
  });

  // The caption's task count is singular at 1, and names a retired count
  // (statuses outside the six funnel stages, e.g. cancelled) only when one
  // exists; features.data.nodes.length still counts every row, retired ones
  // included, so 3 draft + 2 cancelled reads "5 features · 2 retired".
  it("captions the funnel in the singular task, and names retired features when present", async () => {
    const features: ListResult = {
      noun: "feature",
      count: 5,
      nodes: [
        ...rowsFor("F", { draft: 3 }),
        { id: "F-c0", status: "cancelled", ref: "f-c0 (F-c0)" },
        { id: "F-c1", status: "cancelled", ref: "f-c1 (F-c1)" },
      ],
    };
    const tasks: ListResult = {
      noun: "task",
      count: 1,
      nodes: [{ id: "T-1", status: "active", ref: "t-1 (T-1)" }],
    };
    stubFetch(
      baseRoutes({
        "/api/queue": { entries: [] },
        "/api/ls?noun=feature": features,
        "/api/ls?noun=task": tasks,
      }),
    );

    renderHome(summary());

    expect(await screen.findByText("1 task across 5 features · 2 retired")).toBeInTheDocument();
  });

  // The feature word inflects by count the same way the task word does (and
  // the same way the shelf tile's and the pool line's captions do): a
  // one-feature project reads "feature", never "1 features".
  it("captions the funnel in the singular feature for a one-feature project", async () => {
    const features: ListResult = {
      noun: "feature",
      count: 1,
      nodes: [{ id: "F-1", status: "draft", ref: "f-1 (F-1)" }],
    };
    stubFetch(
      baseRoutes({
        "/api/queue": { entries: [] },
        "/api/ls?noun=feature": features,
      }),
    );

    renderHome(summary());

    expect(await screen.findByText("0 tasks across 1 feature")).toBeInTheDocument();
  });

  // FunnelStrip's own unresolved fallback is UnresolvedTile verbatim, the
  // same dash/caption markup the other tiles use - unchanged by task 8.
  it("shows a dash and unavailable, never a confident zero, when a tile's list fails", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/ls?noun=feature"];
    stubFetch(routes);

    renderHome(summary());

    const label = await screen.findByText("Features");
    const tile = label.closest(".stat");
    expect(tile).not.toBeNull();
    expect(await within(tile as HTMLElement).findByText("unavailable")).toBeInTheDocument();
    expect(within(tile as HTMLElement).getByText("—")).toBeInTheDocument();
    expect(within(tile as HTMLElement).queryByText("0")).not.toBeInTheDocument();
  });

  // The band's counted stat tiles are now three, not four - Epics, Questions,
  // and the combined Bugs & Chores shelf tile - with the funnel occupying
  // the width the removed Features/Tasks tiles held, as its own .funnel
  // element rather than a fourth .stat.
  it("draws three counted stat tiles plus the funnel strip, not Features/Tasks", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    const { container } = renderHome(summary());
    await screen.findByText(/all clear/i);

    const labels = [...container.querySelectorAll(".stat .lbl")].map((n) => n.textContent);
    expect(labels).toEqual(["Epics", "Questions", "Fix"]);
    expect(container.querySelectorAll(".stat")).toHaveLength(3);
    expect(container.querySelectorAll(".funnel")).toHaveLength(1);
  });
});

describe("Home header band: shelf tile", () => {
  it("counts every shelf noun's nonterminal work and links to the shared console", async () => {
    const bugs: ListResult = {
      noun: "bug",
      count: 8,
      nodes: rowsFor("B", { draft: 3, active: 3, done: 2 }),
    };
    const chores: ListResult = {
      noun: "chore",
      count: 16,
      nodes: rowsFor("CH", { ready: 8, active: 5, cancelled: 3 }),
    };
    stubFetch(
      baseRoutes({
        "/api/queue": { entries: [] },
        "/api/ls?noun=bug": bugs,
        "/api/ls?noun=chore": chores,
        "/api/ls?noun=tweak": { noun: "tweak", count: 3, nodes: rowsFor("TW", { ready: 2, done: 1 }) },
      }),
    );

    renderHome(summary());

    const link = await screen.findByRole("link", { name: /21.*Fix/ });
    expect(link).toHaveAttribute("href", "/fix");
    expect(link.textContent).toContain("21");
    expect(link.textContent).toContain("6 bugs · 13 chores · 2 tweaks");
  });

  it("shows a dash and unavailable, never a confident zero, when either shelf list fails", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/ls?noun=chore"];
    stubFetch(routes);

    renderHome(summary());

    const labels = await screen.findAllByText("Fix");
    const tile = labels.map((label) => label.closest(".stat")).find((candidate) => candidate !== null);
    expect(tile).not.toBeNull();
    expect(await within(tile as HTMLElement).findByText("unavailable")).toBeInTheDocument();
    expect(within(tile as HTMLElement).getByText("—")).toBeInTheDocument();
  });
});

describe("Home header band: drift slot", () => {
  it("renders a count and text distinguishing candidates from uncovered, linking to /drift", async () => {
    const drift: DriftReportView = {
      compared: 5,
      candidates: {
        active_basis: [{ marker: "active_basis", noun: "feature", id: "F-1" }],
        stale_spec: [{ marker: "stale_spec", noun: "task", id: "T-1" }],
      },
      uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-2", reason: "no_completion_record" }],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/drift": drift }));

    renderHome(summary());

    const link = await screen.findByRole("link", { name: /drift/i });
    expect(link).toHaveAttribute("href", "/drift");
    expect(link.textContent).toContain("3");
    expect(link.textContent).toContain("2 drifted, 1 unchecked");
  });

  it("renders nothing for a RESOLVED all-zero drift report", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/drift": emptyDrift() }));

    const { container } = renderHome(summary());
    await screen.findByText(/all clear/i);

    expect(screen.queryByRole("link", { name: /drift/i })).not.toBeInTheDocument();
    // Task 8: three counted .stat tiles now (Epics, Questions, Bugs & Chores),
    // not four - the funnel that replaced Features/Tasks is its own .funnel
    // element, never a .stat.
    expect(container.querySelectorAll(".stat")).toHaveLength(3);
  });

  it("renders the unavailable treatment, never silence, when the drift fetch fails", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/drift"];
    stubFetch(routes);

    renderHome(summary());

    const label = await screen.findByText("Drift");
    const tile = label.closest(".stat");
    expect(tile).not.toBeNull();
    const unavailable = await within(tile as HTMLElement).findByText("unavailable");
    expect(unavailable).toBeInTheDocument();
    expect(unavailable.getAttribute("title")).toContain("GET /api/drift failed");
    expect(within(tile as HTMLElement).queryByText(/drifted/)).not.toBeInTheDocument();
  });
});

// ---------- Home recap signal (unchanged; the horizon guard's own peek) ----------

describe("Home recap signal", () => {
  it("names the volume behind the horizon and links to the view", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap/peek": peek({ total: 167, asks: 32 }) }));

    renderHome(summary());

    const link = await screen.findByRole("link", { name: /History/ });
    expect(link).toHaveAttribute("href", "/history");
    expect(link.textContent).toBe("History · 167 events since your last visit");
  });

  // The next three, restored: RecapSignal itself
  // did not change in this rebuild, but its own tests were dropped from the
  // file along with the old DecisionCard block they used to sit near.
  it("states volume only, never the ask count the queue beside it already lists", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap/peek": peek({ total: 167, asks: 32 }) }));

    const { container } = renderHome(summary());
    await screen.findByRole("link", { name: /History/ });

    // "32 need you" beside a card headed "32 Answer questions" is the exact
    // duplication that moved the recap off Home.
    expect(container.querySelector(".recap-signal")?.textContent).not.toContain("32");
    expect(screen.queryByText(/need you/)).not.toBeInTheDocument();
  });

  it("says nothing new on an empty horizon, and agrees with the view's own empty", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary());

    const link = await screen.findByRole("link", { name: /History/ });
    expect(link.textContent).toBe("History · nothing new since your last visit");
  });

  it("keeps the singular event honest", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap/peek": peek({ total: 1 }) }));

    renderHome(summary());

    expect((await screen.findByRole("link", { name: /History/ })).textContent).toBe(
      "History · 1 event since your last visit",
    );
  });

  it("renders a bare link on a failed peek rather than claiming nothing new", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/recap/peek"];
    stubFetch(routes);

    renderHome(summary());
    // The bare link is also what renders while the peek is in flight, so the
    // assertion needs a point where the peek has certainly settled: every
    // stub resolves on the same tick, so a resolved sibling fetch means the
    // peek's rejection has been handled too. The all-clear needs all four
    // lists in hand, which is well past that point.
    await screen.findByText(/all clear/i);

    const link = screen.getByRole("link", { name: "History" });
    expect(link).toHaveAttribute("href", "/history");
    expect(link.textContent).toBe("History");
    expect(screen.queryByText(/since your last visit/)).not.toBeInTheDocument();
  });
});

// ---------- The pulse ----------

describe("Home pulse: Just happened", () => {
  it("renders collapsed-burst rows from the fixed 24h window, capped, with a full-history link", async () => {
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 2,
      groups: [
        {
          epic: { key: "form-engine", id: "E-1", ref: "form-engine (E-1)" },
          stream: "pat",
          entries: [
            {
              at: "2026-08-10T14:10:00Z",
              type: "done.task",
              contributor: "pat",
              command: "tiller check task T-1",
              ask: false,
              targets: [{ noun: "task", id: "T-1", key: "form-engine/certification-form-loading/schema", ref: "schema (T-1)" }],
            },
            {
              at: "2026-08-10T13:42:00Z",
              type: "submit.task",
              contributor: "pat",
              command: "tiller submit task T-2",
              ask: false,
              targets: [{ noun: "task", id: "T-2", title: "Applicant portal auth", ref: "T-2" }],
            },
          ],
          omitted: 0,
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap?window=24h": recap }));

    renderHome(summary());

    expect(await screen.findByText("submitted")).toBeInTheDocument();
    expect(screen.getByText("Applicant portal auth")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Full history →" })).toHaveAttribute("href", "/history?window=24h");
  });

  // BurstPhrase's own subject carries
  // no numeral ("tasks done", not "4 tasks done" - History renders the count
  // in its own separate column), so a naive reshape of its grammar onto the
  // pulse's one-line .prow silently dropped the count on every multi-entry
  // burst. The two prior fixtures above happen to use alternating types,
  // which never collapse past one entry each - this fixture is the one that
  // actually exercises n > 1.
  it("shows the count on a collapsed multi-entry burst, not just the bare noun phrase", async () => {
    const entries = Array.from({ length: 4 }, (_, i) => ({
      at: `2026-08-10T14:1${i}:00Z`,
      type: "done.task",
      contributor: "pat",
      command: `tiller check task T-${i}`,
      ask: false,
      targets: [],
    }));
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 4,
      groups: [
        {
          epic: { key: "form-engine", id: "E-1", ref: "form-engine (E-1)" },
          stream: "pat",
          entries,
          omitted: 0,
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap?window=24h": recap }));

    renderHome(summary());

    expect(await screen.findByText("4 tasks done")).toBeInTheDocument();
    expect(screen.queryByText("tasks done")).not.toBeInTheDocument();
  });

  it("shows how many more events the window held beyond the capped rows", async () => {
    const entries = Array.from({ length: 8 }, (_, i) => ({
      at: `2026-08-10T${String(14 - i).padStart(2, "0")}:00:00Z`,
      type: i % 2 === 0 ? "add.epic" : "add.feature",
      contributor: "jc",
      command: "tiller add",
      ask: false,
      targets: [],
    }));
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 8,
      groups: [{ epic: null, stream: "jc", entries, omitted: 0 }],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/recap?window=24h": recap }));

    renderHome(summary());

    // Eight alternating types never collapse into fewer than 8 one-entry
    // bursts, capped at 5 (the console-wide row cap); 3 events are left over.
    expect(await screen.findByText(/3 more in this window/)).toBeInTheDocument();
  });

  it("shows nothing new when the window is empty", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary());

    expect(await screen.findByText("Nothing in the last 24 hours.")).toBeInTheDocument();
  });
});

describe("Home pulse: In flight", () => {
  it("keeps ready, proposed, checked, blocked and review assignments visible without inventing work for a past contributor", async () => {
    const active = ["proposed", "ready", "check"].map((status) => ({
      noun: "task", id: `T-${status}`, ref: `T-${status}`, status, display: status,
    }));
    const streams: Streams = { contributors: [
      { contributor: { id: "sam", name: "Current engineer" }, active, blocked: [{ noun: "bug", id: "B-paused", ref: "B-paused" }],
        gates_in_review: [{ noun: "chore", id: "CH-review", ref: "CH-review" }], open_questions: [] },
      { contributor: { id: "past", name: "Past contributor" }, active: [], blocked: [], gates_in_review: [], open_questions: [] },
    ] };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));
    renderHome(summary());

    expect(await screen.findByText("Current engineer")).toBeInTheDocument();
    for (const item of active) {
      expect(screen.getByRole("link", { name: item.id })).toHaveAttribute("href", `/node/task/${item.id}`);
    }
    expect(screen.getByRole("link", { name: "B-paused" })).toHaveAttribute("href", "/node/bug/B-paused");
    expect(screen.getByRole("link", { name: "CH-review" })).toHaveAttribute("href", "/node/chore/CH-review");
    expect(screen.queryByText("Past contributor")).not.toBeInTheDocument();
  });

  it("shows shared closeout once for the review unit, preserves actor and does not invent an acceptance command", async () => {
    const entries: QueueEntry[] = [
      { kind: "workflow_closeout", noun: "feature", id: "F-assemble", ref: "F-assemble", actor: "agent", phase: "assembly",
        prose: "Assemble both task branches.", command: "tiller assemble feature F-assemble" },
      { kind: "workflow_closeout", noun: "epic", id: "E-accept", ref: "E-accept", actor: "human", phase: "acceptance",
        prose: "Review the assembled contract before acceptance.", command: "" },
    ];
    stubFetch(baseRoutes({ "/api/queue": { entries } }));
    renderHome(summary());

    expect(await screen.findByRole("link", { name: "F-assemble" })).toHaveAttribute("href", "/node/feature/F-assemble");
    expect(screen.getAllByRole("button", { name: "tiller assemble feature F-assemble" })).toHaveLength(1);
    expect(screen.getByText("Shared closeout · agent")).toBeInTheDocument();
    const acceptance = screen.getByRole("link", { name: "E-accept" }).closest(".dcard") as HTMLElement;
    expect(within(acceptance).getByText("Shared closeout · human")).toBeInTheDocument();
    expect(within(acceptance).getByText("Review the assembled contract before acceptance.")).toBeInTheDocument();
    expect(within(acceptance).queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders a stream row for each owner that has one, under its own contributor-group", async () => {
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "jc", name: "jc" },
          active: [{ noun: "task", id: "T-1", key: "form-engine/certification-form-loading/schema", ref: "schema (T-1)", status: "active", display: "active" }],
          blocked: [],
          gates_in_review: [],
          open_questions: [],
        },
        {
          contributor: { id: "pat", name: "pat" },
          active: [{ noun: "task", id: "T-2", key: "intake/applicant-portal-auth/wire-oauth", ref: "wire-oauth (T-2)", status: "active", display: "active" }],
          blocked: [],
          gates_in_review: [],
          open_questions: [],
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    const { container } = renderHome(summary());

    await screen.findByText("jc");
    expect(screen.getByText("pat")).toBeInTheDocument();
    expect(screen.getByText(/schema/)).toBeInTheDocument();
    expect(screen.getByText(/wire-oauth/)).toBeInTheDocument();
    expect(container.querySelectorAll(".contributor-group")).toHaveLength(2);
  });

  // The prior version of this test split active
  // and gates_in_review across two DIFFERENT contributors, so it could pass even
  // if a single owner's own rows rendered in the wrong order - it only ever
  // proved that both kinds of row appeared SOMEWHERE. One owner carrying all
  // three lists is what actually exercises the brief's own ordering rule.
  it("renders one owner's active work, then gates in review, then the blocked register, in that document order", async () => {
    const active: StreamItem[] = [
      { noun: "task", id: "T-1", key: "form-engine/certification-form-loading/schema", ref: "schema (T-1)", status: "active", display: "active" },
    ];
    const gatesInReview: GatesItem[] = [
      { noun: "task", id: "T-2", key: "intake/applicant-portal-auth/wire-oauth", ref: "wire-oauth (T-2)", gates: {} },
    ];
    const blocked: BlockedItem[] = [
      {
        noun: "feature",
        id: "F-3",
        key: "intake/eligibility-rules",
        ref: "intake/eligibility-rules (F-3)",
        depends_on: ["Q-1"],
        depends_on_refs: ["Q-1"],
      },
    ];
    const streams: Streams = {
      contributors: [{ contributor: { id: "sam", name: "sam" }, active, blocked, gates_in_review: gatesInReview, open_questions: [] }],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    renderHome(summary());

    const activeRow = await screen.findByText(/schema/);
    const gatesRow = screen.getByText(/wire-oauth/);
    const blockedRow = screen.getByText(/eligibility-rules/);

    expect(
      Boolean(activeRow.compareDocumentPosition(gatesRow) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);
    expect(
      Boolean(gatesRow.compareDocumentPosition(blockedRow) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);
  });

  it("renders a manual blocked flag and a held row as two distinct rows (issue #39), never folded into one", async () => {
    const blocked: BlockedItem[] = [
      {
        noun: "feature",
        id: "F-1",
        key: "intake/eligibility-rules",
        ref: "intake/eligibility-rules (F-1)",
        depends_on: ["Q-h2n7m5"],
        depends_on_refs: ["Q-h2n7m5"],
      },
      {
        noun: "feature",
        id: "F-1",
        key: "intake/eligibility-rules",
        ref: "intake/eligibility-rules (F-1)",
        held: { noun: "epic", id: "E-9", ref: "legacy-intake (E-9)", state: "cancelled" },
      },
    ];
    const streams: Streams = {
      contributors: [{ contributor: { id: "jc", name: "jc" }, active: [], blocked, gates_in_review: [], open_questions: [] }],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    renderHome(summary());

    expect(await screen.findByText(/blocked on/)).toBeInTheDocument();
    expect(screen.getByText("Q-h2n7m5")).toBeInTheDocument();
    expect(screen.getByText(/held under legacy-intake \(E-9\) \(cancelled\)/)).toBeInTheDocument();
  });

  it("renders a question hold separately from manual and retired rows with a distinct stable key", async () => {
    const shared = {
      noun: "task" as const,
      id: "T-compile",
      key: "widgets/compiler/compile",
      ref: "widgets/compiler/compile (T-compile)",
    };
    const blocked: BlockedItem[] = [
      {
        ...shared,
        depends_on: ["F-dependency"],
        depends_on_refs: ["widgets/dependency (F-dependency)"],
      },
      {
        ...shared,
        held: {
          noun: "epic",
          id: "E-retired",
          ref: "retired-widgets (E-retired)",
          state: "cancelled",
        },
      },
      {
        ...shared,
        question_holds: [
          {
            question: "Q-schema",
            title: "Canonical schema",
            target_id: "T-compile",
            target_noun: "task",
          },
        ],
      },
    ];
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "jc", name: "jc" },
          active: [],
          blocked,
          gates_in_review: [],
          open_questions: [],
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    const { container } = renderHome(summary());

    await screen.findByText("jc");
    const ownerGroup = container.querySelector(".contributor-group");
    expect(ownerGroup).not.toBeNull();
    const rowText = Array.from(ownerGroup!.querySelectorAll(".prow"), (row) => row.textContent ?? "");

    expect(rowText).toHaveLength(3);
    expect(rowText.filter((text) => text.includes("blocked on widgets/dependency (F-dependency)"))).toHaveLength(1);
    expect(rowText.filter((text) => text.includes("held under retired-widgets (E-retired) (cancelled)"))).toHaveLength(1);
    const questionRows = rowText.filter((text) => text.includes("Canonical schema"));
    expect(questionRows).toHaveLength(1);
    expect(questionRows[0]).toContain("Q-schema");
    expect(rowText.filter((text) => text.includes("Q-schema"))).toHaveLength(1);

    const reactErrors = consoleError.mock.calls.flat().join(" ");
    expect(reactErrors).not.toMatch(/same key|unique ["']key["']/i);
  });

  it("omits an owner with nothing active, in review, or blocked", async () => {
    const streams: Streams = {
      contributors: [
        { contributor: { id: "jc", name: "jc" }, active: [], blocked: [], gates_in_review: [], open_questions: [] },
        {
          contributor: { id: "pat", name: "pat" },
          active: [{ noun: "task", id: "T-1", ref: "T-1", status: "active", display: "active" }],
          blocked: [],
          gates_in_review: [],
          open_questions: [],
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    renderHome(summary());

    await screen.findByText("pat");
    expect(screen.queryByText("jc")).not.toBeInTheDocument();
  });

  it("shows nothing in flight when every owner is idle", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));

    renderHome(summary());

    expect(await screen.findByText("Nothing in flight.")).toBeInTheDocument();
  });


  it("renders no pool line when no task is available", async () => {
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "sam", name: "sam" },
          active: [{ noun: "task", id: "T-1", ref: "T-1", status: "active", display: "active" }],
          blocked: [],
          gates_in_review: [],
          open_questions: [],
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    const { container } = renderHome(summary());

    await screen.findByText("sam");
    expect(container.querySelector(".pool-line")).not.toBeInTheDocument();
    expect(container.querySelectorAll(".contributor-group")).toHaveLength(1);
  });

  it("labels shelf assignments beside task assignments", async () => {
    const active: StreamItem[] = [
      { noun: "bug", id: "B-1", ref: "bugs/crash-loop", status: "in_progress", display: "in_progress" },
      { noun: "task", id: "T-1", ref: "T-1", status: "ready", display: "ready" },
    ];
    const gatesInReview: GatesItem[] = [
      { noun: "chore", id: "CH-1", ref: "chores/tidy-logs", gates: {} },
    ];
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "ana", name: "ana" },
          active,
          blocked: [],
          gates_in_review: gatesInReview,
          open_questions: [],
        },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    const { container } = renderHome(summary());

    await screen.findByText("ana");
    const rows = Array.from(container.querySelectorAll(".prow"));
    expect(rows).toHaveLength(3);

    const bugRow = rows.find((row) => row.textContent?.includes("B-1"));
    const choreRow = rows.find((row) => row.textContent?.includes("CH-1"));
    const taskRow = rows.find((row) => row.textContent?.includes("T-1"));

    expect(bugRow?.querySelector(".noun-chip")?.textContent).toBe("bug");
    expect(choreRow?.querySelector(".noun-chip")?.textContent).toBe("chore");
    expect(taskRow?.querySelector(".noun-chip")).toBeNull();
  });
});

describe("Home in flight: the pool line and the blocked cap", () => {

  it("caps a blocked register at ROW_CAP rows with an 'N more blocked' foot carrying the full count, while active rows are never capped", async () => {
    const blockedCount = ROW_CAP + 2;
    const blocked: BlockedItem[] = Array.from({ length: blockedCount }, (_, i) => ({
      noun: "feature", id: `F-b${i}`, ref: `blocked-${i} (F-b${i})`,
      depends_on: ["Q-1"], depends_on_refs: ["Q-1"],
    }));
    const active: StreamItem[] = Array.from({ length: 8 }, (_, i) => ({
      noun: "task", id: `T-a${i}`, key: `widgets/active-${i}`, ref: `active-${i} (T-a${i})`, status: "active", display: "active",
    }));
    const streams: Streams = {
      contributors: [
        { contributor: { id: "sam", name: "sam" }, active, blocked, gates_in_review: [], open_questions: [] },
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": streams }));

    const { container } = renderHome(summary());

    await screen.findByText("sam");
    // ROW_CAP + 2 blocked rows capped to ROW_CAP, plus the "N more" foot.
    const blockedRows = Array.from(container.querySelectorAll(".prow")).filter((row) =>
      (row.textContent ?? "").includes("blocked on"),
    );
    expect(blockedRows).toHaveLength(ROW_CAP);
    const remainder = blockedCount - ROW_CAP;
    const foot = screen.getByText(new RegExp(`${remainder} more blocked`));
    expect(foot.textContent).toContain(`${remainder} more blocked`);
    expect(within(foot.closest(".pfoot") as HTMLElement).getByRole("link", { name: /Build/ })).toHaveAttribute(
      "href",
      "/build",
    );

    // Motion (active rows) is never capped: all 8 render.
    const activeRows = Array.from(container.querySelectorAll(".prow")).filter((row) =>
      (row.textContent ?? "").includes("widgets/active-"),
    );
    expect(activeRows).toHaveLength(8);
  });
});

// Up next renders GET /api/next as served (ranked-next design, decisions 5
// and 7): the server ranks and cuts to five, so the column never re-sorts,
// filters, or recomputes. Each row leads with its actor, then the action and
// a link to its node, then the reason worded as the CLI words it.

function candidate(overrides: Partial<NextCandidate> = {}): NextCandidate {
  return {
    actor: "agent",
    kind: "claim_task",
    action: "claim and start",
    node: { noun: "task", id: "T-1", key: "payments/ach/ledger", ref: "payments/ach/ledger (T-1)", title: "Ledger" },
    command: "tiller claim task payments/ach/ledger",
    reason: "next in your current feature",
    tier: 3,
    ...overrides,
  };
}

function upNextColumn(): HTMLElement {
  const head = screen.getByText(/^Up next/);
  return head.closest(".pcol") as HTMLElement;
}

describe("Home pulse: Up next", () => {
  it("renders the ranked candidates with actor labels", async () => {
    const next: NextAnswer = {
      reader: "jc",
      caught_up: false,
      candidates: [
        candidate({
          actor: "human",
          kind: "approve_feature",
          action: "approve the spec for",
          node: { noun: "feature", id: "F-12", key: "payments/ach", ref: "payments/ach (F-12)" },
          command: "tiller approve feature payments/ach",
          reason: "holds `payments/ach/ledger` in your current feature",
          tier: 1,
        }),
        candidate(),
      ],
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": next }));
    renderHome(summary());

    const spec = await screen.findByRole("link", { name: "payments/ach" });
    const column = upNextColumn();
    const rows = column.querySelectorAll(".prow");
    expect(rows).toHaveLength(2);

    // Served order is kept, the human's move first here.
    expect(rows[0].querySelector(".actor")?.textContent).toBe("You");
    expect(rows[1].querySelector(".actor")?.textContent).toBe("Agent");
    expect(spec).toHaveAttribute("href", "/node/feature/F-12");
    expect(within(rows[1] as HTMLElement).getByRole("link", { name: "payments/ach/ledger" })).toHaveAttribute(
      "href",
      "/node/task/T-1",
    );
    expect(rows[0].textContent).toContain("approve the spec for");
    expect(rows[1].textContent).toContain("claim and start");

    // The reason reads as a sentence, a backticked key as a code span.
    const why = rows[0].querySelector(".why") as HTMLElement;
    expect(why.querySelector("code")?.textContent).toBe("payments/ach/ledger");
    expect(why.textContent).toBe("Holds payments/ach/ledger in your current feature.");
    expect(rows[1].querySelector(".why")?.textContent).toBe("Next in your current feature.");

    // Commands belong to Needs you and the CLI, never to an Up next row.
    expect(within(column).queryByText(/tiller approve feature/)).not.toBeInTheDocument();
    expect(within(column).queryByText(/tiller claim task/)).not.toBeInTheDocument();
  });

  it("names the reader in the header", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": { ...emptyNext(), reader: "robin" } }));
    renderHome(summary());

    expect(await screen.findByText("Up next · for robin")).toBeInTheDocument();
    expect(screen.queryByText(/Not personalized/)).not.toBeInTheDocument();
  });

  it("names the reader by roster display name", async () => {
    const s = summary({ contributors: [{ id: "ada", name: "Ada Park" }] });
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": { ...emptyNext(), reader: "ada" } }));
    render(
      <MemoryRouter>
        <PeopleProvider contributors={s.contributors}>
          <Home summary={s} summaryError={null} />
        </PeopleProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Up next · for Ada Park")).toBeInTheDocument();
  });

  it("says not personalized without a reader", async () => {
    const next: NextAnswer = { reader: "", caught_up: false, candidates: [candidate()] };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": next }));
    renderHome(summary());

    const note = (await screen.findByText(/Not personalized/)).closest(".muted") as HTMLElement;
    expect(note).not.toBeNull();
    expect(note).toHaveTextContent("Not personalized; run tiller join.");
    expect(note.querySelector("code")?.textContent).toBe("tiller join");
    const column = upNextColumn();
    expect(within(column).getByText("Up next")).toBeInTheDocument();
    expect(within(column).queryByText(/· for/)).not.toBeInTheDocument();
  });

  it("shows caught up with its hint", async () => {
    const next: NextAnswer = {
      reader: "jc",
      caught_up: true,
      candidates: [],
      hint: "2 tasks are held in the pool; `tiller claim` takes one",
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": next }));
    renderHome(summary());

    expect(await screen.findByText("Caught up: nothing is legal to do right now.")).toBeInTheDocument();
    const column = upNextColumn();
    expect(column.querySelectorAll(".prow")).toHaveLength(0);
    const hint = column.querySelector(".hint") as HTMLElement;
    expect(hint.textContent).toBe("2 tasks are held in the pool; tiller claim takes one.");
    expect(hint.querySelector("code")?.textContent).toBe("tiller claim");
  });

  it("keeps the board's unclaimed tasks out of Up next and in In flight's pool link", async () => {
    const unclaimed: UnclaimedItem[] = [
      { noun: "task", id: "T-9", ref: "T-9", title: "Available task", status: "ready", feature_id: "F-1" },
    ];
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/board": { contributors: [], unclaimed } }));
    renderHome(summary());

    expect(await screen.findByRole("link", { name: "1 unclaimed task" })).toHaveAttribute("href", "/build");
    expect(screen.queryByRole("link", { name: "Available task" })).not.toBeInTheDocument();
  });

  it("renders the next fetch error in the column", async () => {
    const routes = baseRoutes({ "/api/queue": { entries: [] } });
    delete routes["/api/next"];
    stubFetch(routes);
    renderHome(summary());

    const err = await screen.findByText(/GET \/api\/next failed \(404 Not Found\)/);
    expect(err.closest(".pcol")).toBe(upNextColumn());
  });

  it("renders dependency diagnostics from /api/next", async () => {
    const next: NextAnswer = {
      ...emptyNext(),
      dependency_diagnostics: {
        ready: false,
        gaps: [{ reason: "dependency_cycle", message: "T-1 and T-2 depend on each other" }],
        advisories: [],
      },
    };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": next }));
    renderHome(summary());

    const section = await screen.findByRole("region", { name: "Dependency diagnostics" });
    expect(within(section).getByText("T-1 and T-2 depend on each other")).toBeInTheDocument();
    // Still its own section above the needs-you head, where the route put it.
    const needsHead = screen.getByText("Needs you");
    expect(section.compareDocumentPosition(needsHead) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("has no route strip", async () => {
    const next: NextAnswer = { ...emptyNext(), caught_up: false, candidates: [candidate()] };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/next": next }));
    const { container } = renderHome(summary());

    await screen.findByRole("link", { name: "payments/ach/ledger" });
    expect(screen.queryByText("Route says")).not.toBeInTheDocument();
    expect(container.querySelector(".route-strip")).toBeNull();
  });
});

describe("Home", () => {
  it("shows the summary error in place of the desk", () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));
    render(
      <MemoryRouter>
        <Home summary={null} summaryError="GET /api/status failed" />
      </MemoryRouter>,
    );
    expect(screen.getByText("GET /api/status failed")).toBeInTheDocument();
  });
});

// ---------- The Overview and Branches tabs ----------

// Execution sources used to sit in the App shell above every page, and on a
// real project the diagnostics list took the top of every view (JC,
// 2026-10-06). They now live behind Home's Branches tab, the choice riding
// ?tab= the way the decision kind rides ?needs=.
describe("Home tabs: Overview and Branches", () => {
  function execution(): ExecutionSnapshot {
    return {
      observations: [
        {
          id: "T-one",
          key: "epic/feature/task-one",
          branch: "task-one",
          source: "local_branch",
          revision: "abcdef123456",
          observed_at: "2026-10-06T12:00:00Z",
          uncommitted: false,
          status: "in_progress",
          owner: "jc",
          integration_status: "ready",
        },
      ],
      diagnostics: [
        { branch: "task-missing", code: "state_unreadable", message: "Task state could not be read" },
        { id: "T-two", code: "branch_missing", message: "No branch for this task" },
      ],
    };
  }

  function viewRail() {
    return screen.getByRole("navigation", { name: "Home view" });
  }

  function tabNames() {
    return within(viewRail())
      .getAllByRole("button")
      .map((tab) => tab.textContent?.replace(/\s+/g, " ").trim());
  }

  function LocationProbe({ onSearch }: { onSearch: (search: string) => void }) {
    onSearch(useLocation().search);
    return null;
  }

  it("opens on Overview, counts diagnostics on the Branches tab, and keeps execution content off the desk", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/execution": execution() }));

    renderHome(summary());
    await screen.findByText(/all clear/i);

    await waitFor(() => expect(tabNames()).toEqual(["Overview", "Branches · 2"]));
    // Diagnostics are warnings, so their count reads in the warning tone.
    expect(within(viewRail()).getByText("· 2")).toHaveClass("tab-count-warning");
    expect(within(viewRail()).getByRole("button", { name: "Overview" })).toHaveAttribute("aria-current", "true");
    expect(screen.queryByText("Task state could not be read", { exact: false })).not.toBeInTheDocument();
    expect(screen.queryByText(/The console does not fetch Git remotes/)).not.toBeInTheDocument();
  });

  it("carries no count on the Branches tab when there are no diagnostics", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/execution": { ...execution(), diagnostics: [] } }));

    renderHome(summary());
    await screen.findByText(/all clear/i);

    expect(tabNames()).toEqual(["Overview", "Branches"]);
  });

  it("selects Branches onto the URL and shows observations and diagnostics in place of the desk", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/execution": execution() }));
    let search = "";

    renderHome(summary(), "/", <LocationProbe onSearch={(s) => (search = s)} />);
    await screen.findByText(/all clear/i);
    fireEvent.click(within(viewRail()).getByRole("button", { name: /Branches/ }));

    expect(new URLSearchParams(search).get("tab")).toBe("branches");
    expect(within(viewRail()).getByRole("button", { name: /Branches/ })).toHaveAttribute("aria-current", "true");
    expect(screen.getByText("Task state could not be read", { exact: false })).toBeVisible();
    expect(screen.getByRole("link", { name: "epic/feature/task-one" })).toHaveAttribute("href", "/node/task/T-one");
    expect(screen.getByRole("link", { name: "T-two" })).toHaveAttribute("href", "/node/task/T-two");
    expect(screen.getByText(/The console does not fetch Git remotes/)).toBeVisible();
    expect(screen.queryByText(/all clear/i)).not.toBeInTheDocument();

    fireEvent.click(within(viewRail()).getByRole("button", { name: "Overview" }));
    expect(new URLSearchParams(search).has("tab")).toBe(false);
    expect(await screen.findByText(/all clear/i)).toBeInTheDocument();
  });

  it("lands on the Branches tab from a /?tab=branches link", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/execution": execution() }));

    renderHome(summary(), "/?tab=branches");

    expect(await screen.findByText("Task state could not be read", { exact: false })).toBeVisible();
    expect(within(viewRail()).getByRole("button", { name: /Branches/ })).toHaveAttribute("aria-current", "true");
  });

  it("keeps ?needs=<kind> beside ?tab in both directions", async () => {
    const entries = [
      amendmentEntry(),
      ...Array.from({ length: 3 }, (_, i) => epicEntry(i + 1)),
      ...Array.from({ length: 3 }, (_, i) => questionEntry(i + 1)),
    ];
    stubFetch(baseRoutes({ "/api/queue": { entries }, "/api/execution": execution() }));
    let search = "";

    const { container } = renderHome(
      summary(),
      "/?tab=branches&needs=answer_question",
      <LocationProbe onSearch={(s) => (search = s)} />,
    );
    await screen.findByText("Task state could not be read", { exact: false });

    fireEvent.click(within(viewRail()).getByRole("button", { name: "Overview" }));
    await screen.findByText("7 decisions");
    expect(new URLSearchParams(search).get("needs")).toBe("answer_question");
    expect(container.querySelector(".dcard .kind")?.textContent).toBe("Answer · question");

    const kinds = screen.getByRole("navigation", { name: "Decision kind" });
    fireEvent.click(within(kinds).getByRole("button", { name: /Approve · epic/ }));
    fireEvent.click(within(viewRail()).getByRole("button", { name: /Branches/ }));
    const params = new URLSearchParams(search);
    expect(params.get("tab")).toBe("branches");
    expect(params.get("needs")).toBe("approve_epic");
  });

  it("reads execution sources once, not again on every tab switch", async () => {
    const spy = stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/execution": execution() }));

    renderHome(summary());
    await screen.findByText(/all clear/i);
    fireEvent.click(within(viewRail()).getByRole("button", { name: /Branches/ }));
    await screen.findByText("Task state could not be read", { exact: false });
    fireEvent.click(within(viewRail()).getByRole("button", { name: "Overview" }));
    await screen.findByText(/all clear/i);

    expect(spy.mock.calls.filter((c) => String(c[0]) === "/api/execution")).toHaveLength(1);
  });
});

// ---------- The Recovered reviews tab ----------

// Recovered reviews are provenance, not decisions: they live behind their own
// Home tab, present only while a record exists, and never on the desk itself.
async function openRecoveredTab() {
  const rail = screen.getByRole("navigation", { name: "Home view" });
  fireEvent.click(await within(rail).findByRole("button", { name: /Recovered reviews/ }));
}

it("shows recovered historical policy and current green acceptance on Home without a new merge handoff", async () => {
  const recovered = {
    pr: 155, branch: "tiller/reviews/features/billing/refunds", head: "git:historical-head",
    merge_commit: "git:integration-result", integration_branch: "main", basis: "ancestry",
    recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
    tasks: [], policy_snapshots: [{ node_id: "F-1", path: "feature.yml", source: "git:historical-policy",
      observations: [{ gate: "evals", status: "pending", unresolved_criteria: ["C8"] }, { gate: "tests", status: "unknown" }] }],
  };
  const features: ListResult = { noun: "feature", count: 1, nodes: [{
    id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", status: "active", recovered_review: recovered,
    current_acceptance: { node_id: "F-1", contract_status: "valid", acceptance_ready: true },
  }] };
  const next: NextAnswer = { reader: "jc", caught_up: false, candidates: [{
    actor: "agent", kind: "sync", action: "sync",
    node: { noun: "feature", id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)" },
    command: "tiller sync", reason: "run ordinary integration sync", tier: 3,
    workflow: { phase: "sync", actor: "agent", action: "sync", command: "tiller sync",
      reason: "Run ordinary integration sync", review_id: "F-1", feature_id: "F-1",
      contract_status: "valid", acceptance_ready: true, submission_recorded: true, recovered_review: recovered },
  }] };
  stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": features, "/api/next": next }));
  renderHome(summary());
  await openRecoveredTab();
  const panel = await screen.findByRole("region", { name: /recovered historical review/i });
  expect(screen.getAllByRole("region", { name: /recovered historical review/i })).toHaveLength(1);
  expect(within(panel).getByText("git:historical-head")).toBeInTheDocument();
  expect(within(panel).getByText(/C8/)).toBeInTheDocument();
  expect(within(panel).getByText(/tests: unknown/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: ready/i)).toBeInTheDocument();
  expect(screen.queryByText(/awaiting.*merge|waiting on your merge/i)).not.toBeInTheDocument();
});

it("retains recovered historical observations on Home after lifecycle completion", async () => {
  const recovered = {
    branch: "tiller/reviews/features/billing/refunds", head: "git:historic-done-head",
    merge_commit: "git:historic-done-result", integration_branch: "main", basis: "content",
    recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
    tasks: [], policy_snapshots: [{ node_id: "F-1", path: "feature.yml", source: "git:old-policy",
      observations: [{ gate: "evals", status: "pending", unresolved_criteria: ["C8"] }] }],
  };
  const list: ListResult = { noun: "feature", count: 1, nodes: [{
    id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", status: "done",
    recovered_review: recovered, gates: { evals: { status: "pass", required: true } },
    current_acceptance: { node_id: "F-1", contract_status: "valid", acceptance_ready: true },
    historical_violations: 1, crossing_violations: 1, recovered_crossings_pending: false,
  }] };
  stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": list }));
  renderHome(summary());
  await openRecoveredTab();
  const panel = await screen.findByRole("region", { name: /recovered historical review/i });
  expect(within(panel).getByText("git:historic-done-head")).toBeInTheDocument();
  expect(within(panel).getByText(/C8/)).toBeInTheDocument();
  expect(within(panel).getByText(/evals: pass/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: ready/i)).toBeInTheDocument();
  expect(within(panel).getByText(/source-pinned historical policy warnings: 1.*crossing-time policy warnings: 1/)).toBeInTheDocument();
  expect(screen.queryByText(/waiting on your merge/i)).not.toBeInTheDocument();
});

it("does not infer a human merge wait from a recovered riding task in an otherwise empty queue", async () => {
  const recovered = {
    branch: "historical-review", head: "git:contained-head", merge_commit: "git:contained-head",
    integration_branch: "main", basis: "contained", recovered_by: "jc",
    recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized containment record",
    tasks: [], policy_snapshots: [],
  };
  const list: ListResult = { noun: "task", count: 1, nodes: [{
    id: "T-1", title: "Recovered rider", ref: "T-1", status: "in_review", recovered_review: recovered,
  }] };
  stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=task": list }));
  renderHome(summary());
  const panel = await screen.findByRole("region", { name: /recovered historical review/i });
  expect(within(panel).getByText(/not an observed merge/i)).toBeInTheDocument();
  expect(screen.queryByText(/waiting on your merge/i)).not.toBeInTheDocument();
});

it("shows terminal recovered epic acceptance once with all participating feature facts and both warning provenances", async () => {
  const recovered = {
    pr: 155, branch: "historical-epic-review", head: "git:epic-reviewed-head", merge_commit: "git:epic-integration-result",
    integration_branch: "main", basis: "ancestry", recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z",
    reason: "Authorized epic recovery", tasks: [], policy_snapshots: [{ node_id: "F-first", path: "feature.yml", source: "git:historical-policy",
      observations: [{ gate: "evals", status: "pending", unresolved_criteria: ["C8"] }] }],
  };
  const participating = [
    { node_id: "F-first", contract_status: "valid", acceptance_ready: true },
    { node_id: "F-second", contract_status: "valid", acceptance_ready: false },
  ];
  const epics: ListResult = { noun: "epic", count: 1, nodes: [{
    id: "E-review", key: "work", ref: "work (E-review)", status: "done", recovered_review: recovered,
    current_acceptance: { node_id: "E-review", contract_status: "valid", acceptance_ready: false },
    participating_acceptance: participating, historical_violations: 1, crossing_violations: 1,
    recovered_crossings_pending: false,
  }] };
  const features: ListResult = { noun: "feature", count: 2, nodes: [
    { id: "F-first", key: "work/first", ref: "work/first (F-first)", status: "done", recovered_review: recovered,
      current_acceptance: participating[0], gates: { evals: { required: true, status: "pass" } } },
    { id: "F-second", key: "work/second", ref: "work/second (F-second)", status: "done", recovered_review: recovered,
      current_acceptance: participating[1], gates: { evals: { required: true, status: "fail" } } },
  ] };
  const fetch = stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=epic": epics, "/api/ls?noun=feature": features }));
  renderHome(summary());
  await openRecoveredTab();
  const panel = await screen.findByRole("region", { name: /recovered historical review/i });
  expect(screen.getAllByRole("region", { name: /recovered historical review/i })).toHaveLength(1);
  expect(within(panel).getByText(/F-first: ready/)).toBeInTheDocument();
  expect(within(panel).getByText(/F-second: unresolved/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
  expect(within(panel).getByText(/source-pinned historical policy warnings: 1.*crossing-time policy warnings: 1/)).toBeInTheDocument();
  expect(screen.getByText(/evals: pass/)).toBeInTheDocument();
  expect(screen.getByText(/evals: fail/)).toBeInTheDocument();
  expect(fetch.mock.calls.map((call) => String(call[0])).some((url) => url.startsWith("/api/node/"))).toBe(false);
});

it("lists only the features that rode a recovered epic review", async () => {
  const recovered = {
    pr: 155, branch: "historical-epic-review", head: "git:epic-reviewed-head", merge_commit: "git:epic-integration-result",
    integration_branch: "main", basis: "ancestry", recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z",
    reason: "Authorized epic recovery", tasks: [], policy_snapshots: [],
  };
  const participating = [{ node_id: "F-first", contract_status: "valid", acceptance_ready: true }];
  const epics: ListResult = { noun: "epic", count: 1, nodes: [{
    id: "E-review", key: "work", ref: "work (E-review)", status: "done", recovered_review: recovered,
    current_acceptance: { node_id: "E-review", contract_status: "valid", acceptance_ready: true },
    participating_acceptance: participating, recovered_crossings_pending: false,
  }] };
  const features: ListResult = { noun: "feature", count: 2, nodes: [
    { id: "F-first", key: "work/first", ref: "work/first (F-first)", status: "done", recovered_review: recovered,
      current_acceptance: participating[0], gates: { evals: { required: true, status: "pass" } } },
    { id: "F-sibling", key: "work/sibling", ref: "work/sibling (F-sibling)", status: "done", recovered_review: recovered,
      gates: { lint: { required: false, status: "warn" } } },
  ] };
  stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=epic": epics, "/api/ls?noun=feature": features }));
  renderHome(summary());
  await openRecoveredTab();
  await screen.findByRole("region", { name: /recovered historical review/i });
  expect(screen.getByText(/evals: pass/)).toBeInTheDocument();
  expect(screen.queryByText(/lint: warn/)).toBeNull();
  expect(screen.queryByText(/Current acceptance: unavailable/)).toBeNull();
});

describe("Home tabs: Recovered reviews", () => {
  const recovered = {
    pr: 155, branch: "tiller/reviews/features/billing/refunds", head: "git:done-head",
    merge_commit: "git:done-result", integration_branch: "main", basis: "ancestry",
    recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
    tasks: [], policy_snapshots: [],
  };

  function tabNames() {
    return within(screen.getByRole("navigation", { name: "Home view" }))
      .getAllByRole("button")
      .map((tab) => tab.textContent?.replace(/\s+/g, " ").trim());
  }

  function doneFeature(ready: boolean): ListResult {
    return { noun: "feature", count: 1, nodes: [{
      id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", status: "done", recovered_review: recovered,
      current_acceptance: { node_id: "F-1", contract_status: "valid", acceptance_ready: ready },
      recovered_crossings_pending: false,
    }] };
  }

  it("draws no tab when nothing was recovered", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] } }));
    renderHome(summary());
    await screen.findByText(/all clear/i);
    expect(tabNames()).toEqual(["Overview", "Branches"]);
  });

  it("keeps a settled recovery off the desk and counts it quietly on its own tab", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": doneFeature(true) }));
    renderHome(summary());
    await waitFor(() => expect(tabNames()).toEqual(["Overview", "Branches", "Recovered reviews · 1"]));
    expect(screen.queryByRole("region", { name: /recovered historical review/i })).toBeNull();
    const rail = screen.getByRole("navigation", { name: "Home view" });
    expect(within(rail).getByText("· 1")).not.toHaveClass("tab-count-warning");
    await openRecoveredTab();
    expect(await screen.findByText("git:done-head")).toBeInTheDocument();
  });

  it("warns on the tab count while a recovery leaves acceptance unresolved", async () => {
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": doneFeature(false) }));
    renderHome(summary());
    const rail = screen.getByRole("navigation", { name: "Home view" });
    expect(await within(rail).findByText("· 1")).toHaveClass("tab-count-warning");
  });

  // No route payload carries a recovered review any more: a record that rides
  // only on an Up next candidate's workflow advice neither draws nor counts.
  it("Recovered reviews tab counts only list records", async () => {
    const other = { ...recovered, head: "git:candidate-only-head", merge_commit: "git:candidate-only-result" };
    const next: NextAnswer = { reader: "jc", caught_up: false, candidates: [{
      actor: "agent", kind: "sync", action: "sync", command: "tiller sync",
      reason: "run ordinary integration sync", tier: 3,
      workflow: { phase: "sync", actor: "agent", action: "sync", command: "tiller sync",
        reason: "Run ordinary integration sync", review_id: "F-2", feature_id: "F-2",
        contract_status: "valid", acceptance_ready: false, submission_recorded: true, recovered_review: other },
    }] };
    stubFetch(baseRoutes({ "/api/queue": { entries: [] }, "/api/ls?noun=feature": doneFeature(true), "/api/next": next }));
    renderHome(summary());
    await waitFor(() => expect(tabNames()).toEqual(["Overview", "Branches", "Recovered reviews · 1"]));
    const rail = screen.getByRole("navigation", { name: "Home view" });
    expect(within(rail).getByText("· 1")).not.toHaveClass("tab-count-warning");
    expect(screen.queryByRole("link", { name: "Recovered review on record" })).not.toBeInTheDocument();
    await openRecoveredTab();
    expect(await screen.findByText("git:done-head")).toBeInTheDocument();
    expect(screen.queryByText("git:candidate-only-head")).not.toBeInTheDocument();
  });
});
