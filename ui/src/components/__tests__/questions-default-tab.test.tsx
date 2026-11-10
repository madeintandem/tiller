// The questions ledger opens on Open when any question is open, else All. That
// default is settled once per visit: a reread that closes the last open
// question (or opens the first) must not flip the tab under the reader, and
// the default is never written to the URL.

import { screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ListRow, TreeModel } from "../../api";
import { renderApp, settleReads, stubReread, summary, type Routes } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function row(id: string, status: string, title: string): ListRow {
  return { id, key: id.toLowerCase(), ref: `${id.toLowerCase()} (${id})`, title, status } as ListRow;
}

function questions(rows: ListRow[]): ListResult {
  return { noun: "question", count: rows.length, nodes: rows };
}

const tree: TreeModel = { project: "acme", nodes: [] };
const QUESTIONS = "/api/ls?noun=question";

function routes(rows: ListRow[]): Routes {
  return {
    "/api/status": summary(),
    "/api/tree": tree,
    "/api/queue": { entries: [] },
    "/api/ls?noun=bug": { noun: "bug", count: 0, nodes: [] },
    "/api/ls?noun=chore": { noun: "chore", count: 0, nodes: [] },
    "/api/ls?noun=tweak": { noun: "tweak", count: 0, nodes: [] },
    [QUESTIONS]: questions(rows),
  };
}

const selectedTab = () => within(screen.getByRole("tablist", { name: "Question status" }))
  .getAllByRole("tab").find((tab) => tab.getAttribute("aria-selected") === "true");

describe("the questions ledger's default tab", () => {
  it("stays on Open when a reread closes the last open question", async () => {
    const stub = stubReread(routes([row("Q-1", "open", "Which queue?"), row("Q-2", "answered", "Which port?")]));
    renderApp("/questions");
    expect(await screen.findByText("Which queue?")).toBeInTheDocument();
    expect(selectedTab()).toHaveTextContent(/^Open/);

    stub.reread();
    await stub.release({ [QUESTIONS]: questions([row("Q-1", "answered", "Which queue?"), row("Q-2", "answered", "Which port?")]) });
    expect(await screen.findByText("No open questions.")).toBeInTheDocument();
    expect(selectedTab()).toHaveTextContent(/^Open/);
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/questions$/);
  });

  it("stays on All when a reread opens the first question", async () => {
    const stub = stubReread(routes([row("Q-2", "answered", "Which port?")]));
    renderApp("/questions");
    expect(await screen.findByText("Which port?")).toBeInTheDocument();
    expect(selectedTab()).toHaveTextContent(/^All/);

    stub.reread();
    await stub.release({ [QUESTIONS]: questions([row("Q-1", "open", "Which queue?"), row("Q-2", "answered", "Which port?")]) });
    expect(await screen.findByText("Which queue?")).toBeInTheDocument();
    expect(selectedTab()).toHaveTextContent(/^All/);
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/questions$/);
  });
});
