// Home's Fix tile opens the list its number counts: with a Fix filter in
// storage, the tile's link shows the unfiltered board for that visit only,
// the board's open count equals the tile's, storage is left alone, and a bare
// /fix afterwards restores the stored filter, which the toolbar names.

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ListRow, WorkItemCard, WorkItemsBoard } from "../../api";
import { homeRoutes, renderApp, settleReads, stubReread } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function row(id: string, status: string): ListRow {
  return { id, key: id.toLowerCase(), ref: `${id.toLowerCase()} (${id})`, title: `Title ${id}`, status } as ListRow;
}

function list(noun: string, nodes: ListRow[]): ListResult {
  return { noun, count: nodes.length, nodes };
}

function card(noun: WorkItemCard["noun"], id: string, title: string, status: string): WorkItemCard {
  return { noun, id, key: `${noun}s/${id.toLowerCase()}`, title, status, ref: `${noun}s/${id.toLowerCase()} (${id})` } as WorkItemCard;
}

const board: WorkItemsBoard = {
  open: [card("bug", "B-1", "Zulu", "ready"), card("bug", "B-2", "Alpha", "proposed"), card("chore", "CH-1", "Mike", "ready")],
  closed: [],
  attachments: [],
};

function routes() {
  return {
    ...homeRoutes(),
    "/api/ls?noun=bug": list("bug", [row("B-1", "ready"), row("B-2", "proposed")]),
    "/api/ls?noun=chore": list("chore", [row("CH-1", "ready")]),
    "/api/work-items": board,
  };
}

const STORED = "tiller-fix-filters:acme";
const toolbar = () => document.querySelector(".board-bar .showing");

describe("Home's Fix tile and the Fix board's stored filters", () => {
  it("opens the unfiltered board for the visit, leaves storage alone, and a bare /fix restores the filter", async () => {
    window.localStorage.setItem(STORED, "noun=chore");
    stubReread(routes());
    renderApp("/");
    const tile = await screen.findByRole("link", { name: "3 open Fix items, open the Fix board" });

    fireEvent.click(tile);
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.getByText("Mike")).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/fix?filters=none");
    expect(toolbar()).toHaveTextContent(/^3 open: 2 bugs, 1 chore, 0 tweaks\.$/);
    expect(window.localStorage.getItem(STORED)).toBe("noun=chore");

    fireEvent.click(within(document.querySelector<HTMLElement>(".site-nav")!).getByRole("link", { name: "Fix" }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/fix?noun=chore"));
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    expect(toolbar()).toHaveTextContent("1 open: 0 bugs, 1 chore, 0 tweaks. Filtered.");
    expect(window.localStorage.getItem(STORED)).toBe("noun=chore");
  });

  it("stores the reader's next filter change made from the unfiltered view and drops the marker", async () => {
    window.localStorage.setItem(STORED, "noun=chore");
    stubReread(routes());
    renderApp("/fix?filters=none");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Bugs" }));
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent(/^\/fix\?noun=bug$/));
    await waitFor(() => expect(window.localStorage.getItem(STORED)).toBe("noun=bug"));
  });

  it("clears the filters from the toolbar's note the way the rail's All links do", async () => {
    window.localStorage.setItem(STORED, "noun=chore");
    stubReread(routes());
    renderApp("/fix");
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/fix?noun=chore"));
    fireEvent.click(within(toolbar()!.parentElement!).getByRole("link", { name: "Clear filters" }));
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(toolbar()).toHaveTextContent(/^3 open: 2 bugs, 1 chore, 0 tweaks\.$/);
    await waitFor(() => expect(window.localStorage.getItem(STORED)).toBe(""));
  });
});
