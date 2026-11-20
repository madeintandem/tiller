// Home with shelf batches (tweaks/console-shows-shelf-batches). A batch is
// the record that reviews several bugs, chores, or tweaks on one branch and
// one PR, and it is one unit of work on every Home surface: it waits on the
// human's merge by name, it flies under its owner, and every link to it opens
// the batch's own page. A recovered bug, chore, or tweak is listed on the
// Recovered reviews tab beside recovered features and epics.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ProjectSummary } from "../../api";
import Home from "../../views/Home";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function summary(): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "robin", name: "Robin Hale" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 1 },
  };
}

function list(noun: string, nodes: unknown[] = []) {
  return { noun, count: nodes.length, nodes };
}

function routes(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/queue": { entries: [] },
    "/api/ls?noun=epic": list("epic"),
    "/api/ls?noun=feature": list("feature"),
    "/api/ls?noun=task": list("task"),
    "/api/ls?noun=question": list("question"),
    "/api/ls?noun=bug": list("bug"),
    "/api/ls?noun=chore": list("chore"),
    "/api/ls?noun=tweak": list("tweak"),
    "/api/ls?noun=batch": list("batch"),
    "/api/recap/peek": { since: "2026-07-24T00:00:00Z", reader: "robin", total: 0, asks: 0 },
    "/api/recap?window=24h": { since: "", window: "24h", reader: "robin", total: 0, groups: [] },
    "/api/board": { contributors: [] },
    "/api/next": { reader: "robin", caught_up: true, candidates: [] },
    "/api/drift": { compared: 0 },
    "/api/execution": { observations: [], diagnostics: [] },
    ...extra,
  };
}

function stubFetch(table: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in table) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(table[url]) });
    }
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function renderHome(path = "/") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={summary()} summaryError={null} />
    </MemoryRouter>,
  );
}

const reviewBatch = {
  id: "BT-2m4vfp", key: "batches/review-fixes", ref: "batches/review-fixes (BT-2m4vfp)",
  title: "Draft and linter fixes", status: "in_review", owner: "robin", members: 2,
};
const members = [
  { id: "B-jn41qb", key: "bugs/lost-draft", ref: "bugs/lost-draft (B-jn41qb)", title: "Draft is lost", status: "ready", activity: "batched", batch: "BT-2m4vfp" },
];

describe("Home with an empty queue and a batch in review", () => {
  it("names the batch as waiting on the merge, with a link to the batch page", async () => {
    stubFetch(routes({ "/api/ls?noun=batch": list("batch", [reviewBatch]), "/api/ls?noun=bug": list("bug", members) }));
    renderHome();
    const link = await screen.findByRole("link", { name: "Draft and linter fixes" });
    expect(link).toHaveAttribute("href", "/node/batch/BT-2m4vfp");
    const block = document.querySelector(".all-clear-review") as HTMLElement;
    expect(block).toHaveTextContent("1 node still sits in review, waiting on your merge:");
    // The member is the batch's work, never a second row of its own.
    expect(within(block).queryByRole("link", { name: "Draft is lost" })).toBeNull();
  });

  it("never reads All clear while the batch waits", async () => {
    stubFetch(routes({ "/api/ls?noun=batch": list("batch", [reviewBatch]) }));
    renderHome();
    await screen.findByRole("link", { name: "Draft and linter fixes" });
    expect(screen.queryByText(/all clear/i)).toBeNull();
    expect(document.querySelector(".all-clear-tick")).toBeNull();
    expect(screen.getByText("Nothing is waiting in the queue.")).toBeInTheDocument();
  });

  it("reads All clear once nothing is in review", async () => {
    stubFetch(routes({ "/api/ls?noun=batch": list("batch", [{ ...reviewBatch, status: "done" }]) }));
    renderHome();
    expect(await screen.findByText(/all clear/i)).toBeInTheDocument();
    expect(document.querySelector(".all-clear-tick")).not.toBeNull();
  });

  it("does not claim the queue is clear when the batch list failed to load", async () => {
    const table = routes();
    delete table["/api/ls?noun=batch"];
    stubFetch(table);
    renderHome();
    expect(await screen.findByText(/part of the project state failed to load/)).toBeInTheDocument();
    expect(screen.queryByText(/all clear/i)).toBeNull();
  });
});

describe("Home pulse with batches", () => {
  it("lists an owner's in-progress batch in In flight, as a batch, linked to its page", async () => {
    stubFetch(routes({
      "/api/board": { contributors: [{
        contributor: { id: "robin", name: "Robin Hale" },
        active: [{ noun: "batch", id: "BT-r6my9p", key: "batches/wip-fixes", ref: "batches/wip-fixes (BT-r6my9p)", title: "Reload and cleanup fixes", status: "in_progress", display: "in_progress" }],
        gates_in_review: [{ noun: "batch", id: "BT-2m4vfp", key: "batches/review-fixes", ref: "batches/review-fixes (BT-2m4vfp)", title: "Draft and linter fixes", gates: {} }],
        blocked: [], open_questions: [],
      }] },
    }));
    renderHome();
    const link = await screen.findByRole("link", { name: "Reload and cleanup fixes" });
    expect(link).toHaveAttribute("href", "/node/batch/BT-r6my9p");
    const row = link.closest(".prow") as HTMLElement;
    expect(within(row).getByText("batch")).toHaveClass("noun-chip");
    expect(row).toHaveTextContent("In progress");
    const review = screen.getByRole("link", { name: "Draft and linter fixes" }).closest(".prow") as HTMLElement;
    expect(within(review).getByText("batch")).toHaveClass("noun-chip");
    expect(review).toHaveTextContent("In review");
  });

  it("opens the batch page from Up next and Just happened", async () => {
    const node = { noun: "batch", id: "BT-2m4vfp", key: "batches/review-fixes", ref: "batches/review-fixes (BT-2m4vfp)", title: "Draft and linter fixes" };
    stubFetch(routes({
      "/api/next": { reader: "robin", candidates: [{ actor: "human", kind: "merge_task", action: "merge", node, command: "tiller sync", reason: "in review, waiting on your merge", tier: 5 }] },
      "/api/recap?window=24h": { since: "", window: "24h", reader: "robin", total: 3, groups: [{ entries: [
        { at: "2026-11-19T22:03:48Z", type: "submit.batch", contributor: "robin", targets: [node] },
        { at: "2026-11-19T22:03:47Z", type: "edit.batch", contributor: "robin", targets: [node] },
        { at: "2026-11-19T22:03:46Z", type: "batch.bug", contributor: "robin", targets: [{ noun: "bug", id: "B-1", key: "bugs/one", ref: "bugs/one (B-1)", title: "One" }] },
      ] }] },
    }));
    renderHome();
    const links = await screen.findAllByRole("link", { name: "Draft and linter fixes" });
    expect(links.length).toBeGreaterThanOrEqual(2);
    for (const link of links) expect(link).toHaveAttribute("href", "/node/batch/BT-2m4vfp");
    // The submit is the milestone; the membership edit and the member's own
    // batch change are upkeep, kept out of Just happened.
    const happened = screen.getByText("Just happened").closest(".pcol") as HTMLElement;
    expect(within(happened).getByText("submitted")).toBeInTheDocument();
    expect(within(happened).queryByText("edited")).toBeNull();
    expect(within(happened).queryByText("batch changed")).toBeNull();
  });
});

describe("Home needs-you with a batch", () => {
  it("draws a batch's review gate as one card that opens the batch page", async () => {
    stubFetch(routes({ "/api/queue": { entries: [{
      kind: "attest_gate", noun: "batch", id: "BT-2m4vfp", key: "batches/review-fixes", ref: "batches/review-fixes (BT-2m4vfp)",
      prose: "Draft and linter fixes", title: "Draft and linter fixes", gate: "code_review", gate_status: "pending",
      command: "tiller attest batch batches/review-fixes --gate code_review --status <pass|fail>",
    }] } }));
    renderHome();
    const link = await screen.findByRole("link", { name: "Draft and linter fixes" });
    expect(link).toHaveAttribute("href", "/node/batch/BT-2m4vfp");
    expect(link.closest(".dcard")).not.toBeNull();
  });
});

describe("Home Recovered reviews tab with shelf nodes", () => {
  const record = {
    pr: 13, branch: "", head: "git:head", merge_commit: "git:merge", integration_branch: "main", basis: "ancestry",
    recovered_by: "robin", recovered_at: "2026-11-19T22:03:47Z", reason: "Merged before submit was recorded",
    tasks: [], policy_snapshots: [],
  };
  const bugs = list("bug", [
    { id: "B-qeexvs", key: "bugs/wrong-timezone", ref: "bugs/wrong-timezone (B-qeexvs)", title: "Report uses the wrong timezone", status: "ready", activity: "recovered", recovered_review: record },
    { id: "B-t2fjiv", key: "bugs/double-submit", ref: "bugs/double-submit (B-t2fjiv)", title: "Form submits twice", status: "done", recovered_review: record },
    { id: "B-plain", key: "bugs/plain", ref: "bugs/plain (B-plain)", title: "Plain bug", status: "ready" },
  ]);
  const tweaks = list("tweak", [
    { id: "TW-1", key: "tweaks/label", ref: "tweaks/label (TW-1)", title: "Save label", status: "done", recovered_review: record },
  ]);
  const features = list("feature", [{
    id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", title: "Refunds", status: "done",
    recovered_review: { ...record, branch: "tiller/reviews/features/billing/refunds" },
  }]);

  // The note's command is its own code element, so match the paragraph's text.
  const awaitingSyncNotes = () =>
    [...document.querySelectorAll("p")].filter((p) => /The next tiller sync records it done\./.test(p.textContent ?? ""));

  it("lists a recovered bug and tweak beside a recovered feature, each linked to its own page", async () => {
    stubFetch(routes({ "/api/ls?noun=bug": bugs, "/api/ls?noun=tweak": tweaks, "/api/ls?noun=feature": features }));
    renderHome("/?tab=recovered");
    expect(await screen.findByRole("link", { name: "Refunds" })).toHaveAttribute("href", "/node/feature/F-1");
    expect(screen.getByRole("link", { name: "Report uses the wrong timezone" })).toHaveAttribute("href", "/node/bug/B-qeexvs");
    expect(screen.getByRole("link", { name: "Form submits twice" })).toHaveAttribute("href", "/node/bug/B-t2fjiv");
    expect(screen.getByRole("link", { name: "Save label" })).toHaveAttribute("href", "/node/tweak/TW-1");
    expect(screen.queryByRole("link", { name: "Plain bug" })).toBeNull();
    expect(screen.getAllByRole("region", { name: /recovered historical review/i })).toHaveLength(4);
  });

  it("counts every record on the tab and warns while a recovered shelf node awaits sync", async () => {
    stubFetch(routes({ "/api/ls?noun=bug": bugs, "/api/ls?noun=tweak": tweaks, "/api/ls?noun=feature": features }));
    renderHome("/?tab=recovered");
    const tab = await screen.findByRole("tab", { name: /Recovered reviews/ });
    expect(tab).toHaveTextContent("4");
    expect(tab.querySelector(".tab-count-warning, [data-tone='warning']")).not.toBeNull();
    expect(awaitingSyncNotes()).toHaveLength(1);
  });

  it("shows the tab for a recovered shelf node alone, with no warning once sync crossed it", async () => {
    stubFetch(routes({ "/api/ls?noun=tweak": tweaks }));
    renderHome("/?tab=recovered");
    const tab = await screen.findByRole("tab", { name: /Recovered reviews/ });
    expect(tab).toHaveTextContent("1");
    expect(tab.querySelector(".tab-count-warning, [data-tone='warning']")).toBeNull();
    expect(awaitingSyncNotes()).toHaveLength(0);
  });
});
