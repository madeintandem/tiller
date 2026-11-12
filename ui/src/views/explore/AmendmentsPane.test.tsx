import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult } from "../../api";
import AmendmentsPane from "./AmendmentsPane";

vi.mock("../../hooks", async (orig) => {
  const actual = await orig<typeof import("../../hooks")>();
  return { ...actual, useFetch: vi.fn() };
});
import { useFetch } from "../../hooks";

function serve(nodes: unknown[]) {
  vi.mocked(useFetch).mockReturnValue({
    data: { nodes, count: nodes.length } as unknown as ListResult,
    error: null,
  } as never);
}

function Where() {
  return <output data-testid="where">{useLocation().search}</output>;
}

function renderPane(url = "/amendments") {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <AmendmentsPane />
      <Where />
    </MemoryRouter>,
  );
}

const pressed = () => screen.getByRole("tab", { selected: true }).textContent;

const row = (id: string, status: string, proposedAt?: string, title = `Title ${id}`) => ({
  noun: "amendment",
  id,
  ref: id,
  key: `key-${id}`,
  title,
  status,
  proposed_at: proposedAt,
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AmendmentsPane", () => {
  it("keeps its header and status tabs above the list, which scrolls on its own", () => {
    serve([row("A-1", "proposed", "2026-01-02T10:00:00Z"), row("A-2", "approved", "2026-01-01T10:00:00Z")]);
    const { container } = renderPane();
    const pane = container.querySelector(".explore-detail");
    expect(pane?.classList.contains("explore-ledger")).toBe(true);
    // The rail and the panel are the pane's own children, so the stylesheet
    // can hold the rail still and scroll only the panel.
    expect(screen.getByRole("tablist").parentElement).toBe(pane);
    expect(screen.getByRole("tabpanel").parentElement).toBe(pane);
    expect(container.querySelector(".node-pin")).toBeNull();
  });

  it("is a four-column grid with named columns and the order caption", () => {
    serve([row("A-1", "proposed", "2026-01-02T10:00:00Z")]);
    const { container } = renderPane();
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Title",
      "Key",
      "Date",
      "Status",
    ]);
    expect(container.querySelector("caption")?.textContent).toBe("Proposed first, then newest");
  });

  it("sorts proposed first, then newest, and keeps served order among undated rows", () => {
    serve([
      row("A-old", "approved", "2026-01-01T10:00:00Z"),
      row("A-new", "approved", "2026-03-01T10:00:00Z"),
      row("A-p-old", "proposed", "2026-02-01T10:00:00Z"),
      row("A-p-new", "proposed", "2026-04-01T10:00:00Z"),
      row("A-p-none", "proposed"),
      row("A-none", "withdrawn"),
    ]);
    renderPane();
    const titles = screen
      .getAllByRole("row")
      .slice(1)
      .map((r) => within(r).getAllByRole("cell")[0].textContent);
    expect(titles).toEqual([
      "Title A-p-new",
      "Title A-p-old",
      "Title A-p-none",
      "Title A-new",
      "Title A-old",
      "Title A-none",
    ]);
  });

  it("renders the date through <Time> and leaves an undated cell empty", () => {
    serve([row("A-1", "proposed", "2026-01-02T10:00:00Z"), row("A-2", "proposed")]);
    const { container } = renderPane();
    const time = container.querySelector("time");
    expect(time?.getAttribute("datetime")).toBe("2026-01-02T10:00:00Z");
    const cells = screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[2]);
    expect(cells[1].textContent).toBe("");
  });

  it("shows the title as a link, the key in mono, and a status word, never a raw token", () => {
    serve([row("A-1", "proposed", "2026-01-02T10:00:00Z", "Rework intake")]);
    const { container } = renderPane();
    expect(screen.getByRole("link", { name: "Rework intake" })).toHaveAttribute("href", "/node/amendment/A-1");
    expect(container.querySelector("td .mono")?.textContent).toBe("key-A-1");
    expect(within(screen.getAllByRole("row")[1]).getAllByRole("cell")[3].textContent).toBe("Proposed");
  });

  it("keeps a file-name title as it is", () => {
    serve([row("A-1", "proposed", undefined, "docs/plans/intake.md")]);
    renderPane();
    expect(screen.getByRole("link", { name: "docs/plans/intake.md" })).toBeInTheDocument();
  });

  describe("the tab in the URL", () => {
    const rows = [row("A-1", "proposed"), row("A-2", "approved")];

    it("opens on All with no parameter, as it does today", () => {
      serve(rows);
      renderPane();
      expect(pressed()).toMatch(/^All/);
    });

    it("?status=<known> selects it, ?status=all selects All, an unknown value reads as no parameter", () => {
      serve(rows);
      renderPane("/amendments?status=approved");
      expect(pressed()).toMatch(/^Approved/);
      cleanup();
      renderPane("/amendments?status=all");
      expect(pressed()).toMatch(/^All/);
      cleanup();
      renderPane("/amendments?status=bogus");
      expect(pressed()).toMatch(/^All/);
    });

    it("pressing each tab writes it to the URL, All as status=all", () => {
      serve(rows);
      renderPane();
      fireEvent.click(screen.getByRole("tab", { name: /^Approved/ }));
      expect(screen.getByTestId("where").textContent).toBe("?status=approved");
      fireEvent.click(screen.getByRole("tab", { name: /^All/ }));
      expect(screen.getByTestId("where").textContent).toBe("?status=all");
      expect(pressed()).toMatch(/^All/);
    });
  });
});

describe("AmendmentsPane: the draft status", () => {
  it("reads All, Draft, Proposed, Approved, Withdrawn with counts, and leads with open amendments", () => {
    serve([row("A-1", "approved"), row("A-2", "draft"), row("A-3", "proposed"), row("A-4", "withdrawn"), row("A-5", "draft")]);
    renderPane();
    expect(screen.getAllByRole("tab").map((t) => t.textContent?.replace(/\s+/g, " ").trim())).toEqual([
      "All · 5",
      "Draft · 2",
      "Proposed · 1",
      "Approved · 1",
      "Withdrawn · 1",
    ]);
  });

  it("lists open amendments, draft and proposed, ahead of settled ones", () => {
    serve([row("A-1", "approved", "2026-03-01T10:00:00Z"), row("A-2", "draft", "2026-01-01T10:00:00Z"), row("A-3", "withdrawn", "2026-04-01T10:00:00Z")]);
    renderPane();
    const titles = screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[0].textContent);
    expect(titles).toEqual(["Title A-2", "Title A-3", "Title A-1"]);
  });
});
