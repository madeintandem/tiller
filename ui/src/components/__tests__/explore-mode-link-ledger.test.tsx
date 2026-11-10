// The ledgers write ?status= (All included) and Home's Questions tile opens
// /questions?status=all, so a mode link that leaves a ledger must not carry
// that status into Filter, where it would be a filter the reader never set.

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ListRow, TreeModel } from "../../api";
import { homeRoutes, renderApp, settleReads, stubReread } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function row(id: string, status: string, title: string): ListRow {
  return { id, key: id.toLowerCase(), ref: `${id.toLowerCase()} (${id})`, title, status } as ListRow;
}

function list(noun: string, nodes: ListRow[]): ListResult {
  return { noun, count: nodes.length, nodes };
}

const tree: TreeModel = {
  project: "acme",
  nodes: [{ noun: "epic", id: "E-1", key: "foundation", title: "Foundation", status: "ready", path: "", files: [], children: [] }],
};

function routes() {
  return {
    ...homeRoutes(),
    "/api/tree": tree,
    "/api/ls?noun=epic": list("epic", [row("E-1", "ready", "Foundation")]),
    "/api/ls?noun=question": list("question", [row("Q-1", "open", "Which queue?")]),
    "/api/amendment": list("amendment", []),
  };
}

describe("Explore's mode links from a ledger", () => {
  it("drops the ledger's status on the way from Home's Questions tile to Filter", async () => {
    stubReread(routes());
    renderApp("/");
    fireEvent.click(await screen.findByRole("link", { name: /1 question, open the questions list/ }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/questions?status=all"));

    const rail = screen.getByRole("navigation", { name: "Explore mode" });
    fireEvent.click(within(rail).getByRole("link", { name: "Filter" }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent(/^\/explore\?mode=filter$/));
    expect(screen.queryByText("No matches.")).not.toBeInTheDocument();
  });

  it("drops the status leaving the amendments ledger too, and keeps it within Explore's own modes", async () => {
    stubReread(routes());
    renderApp("/amendments?status=proposed");
    const rail = await screen.findByRole("navigation", { name: "Explore mode" });
    expect(within(rail).getByRole("link", { name: "Filter" })).toHaveAttribute("href", "/explore?mode=filter");

    fireEvent.click(within(rail).getByRole("link", { name: "Filter" }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent(/^\/explore\?mode=filter$/));
  });

  it("keeps a filter's own status when switching between Explore modes", async () => {
    stubReread(routes());
    renderApp("/explore?mode=filter&noun=epics&status=ready");
    const rail = await screen.findByRole("navigation", { name: "Explore mode" });
    expect(within(rail).getByRole("link", { name: "Map" })).toHaveAttribute("href", "/explore?mode=map&noun=epics&status=ready");
  });
});
