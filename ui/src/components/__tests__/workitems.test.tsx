import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Link, MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { RelatedNode, WorkItemsBoard } from "../../api";
import WorkItems from "../../views/WorkItems";

const attachment: RelatedNode = {
  noun: "feature",
  id: "F-1",
  key: "foundation/console",
  ref: "foundation/console (F-1)",
  status: "active",
  title: "Console",
};

const board: WorkItemsBoard = {
  open: [
    {
      noun: "bug",
      id: "B-z",
      key: "bugs/zulu",
      title: "Zulu",
      status: "ready",
      ref: "bugs/zulu (B-z)",
      owner: "jc",
      attached_to: "F-1",
      attached: attachment,
      attachment_ids: ["E-1", "F-1", "T-1"],
    },
    {
      noun: "bug",
      id: "B-a",
      key: "bugs/alpha",
      title: "Alpha",
      status: "proposed",
      ref: "bugs/alpha (B-a)",
    },
    {
      noun: "chore",
      id: "CH-m",
      key: "chores/mike",
      title: "Mike",
      status: "ready",
      activity: "awaiting_check",
      ref: "chores/mike (CH-m)",
      attached_to: "F-1",
      attached: attachment,
      attachment_ids: ["E-1", "F-1"],
    },
  ],
  closed: Array.from({ length: 7 }, (_, index) => ({
    noun: index % 2 === 0 ? ("bug" as const) : ("chore" as const),
    id: `C-${index}`,
    title: `Closed ${index}`,
    status: index === 1 ? "cancelled" : "done",
    ref: `closed-${index} (C-${index})`,
    closed_at: `2026-08-29T${String(20 - index).padStart(2, "0")}:00:00Z`,
    attachment_ids: ["E-1", "F-1"],
  })),
  attachments: [attachment],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function stubWorkItems(payload: WorkItemsBoard, projectKey?: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      if (String(input) === "/api/work-items") {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(payload) });
      }
      if (projectKey !== undefined && String(input) === "/api/status") {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ project: { name: "Acme", key: projectKey, kind: "greenfield" } }) });
      }
      return Promise.resolve({
        ok: false,
        status: 404,
        statusText: "Not Found",
        text: () => Promise.resolve(""),
      });
    }),
  );
}

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

function renderWorkItems(payload: WorkItemsBoard, entry: string, projectKey?: string) {
  stubWorkItems(payload, projectKey);
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <WorkItems />
      <LocationProbe />
    </MemoryRouter>,
  );
}

function renderWorkItemsError(message: string) {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error(message)));
  return render(
    <MemoryRouter initialEntries={["/fix"]}>
      <WorkItems />
    </MemoryRouter>,
  );
}

describe("Fix board filter memory", () => {
  it("restores the project's last filters when the page is opened with none", async () => {
    const first = renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("switch", { name: "Show older items" }));
    expect(await screen.findByText("Closed 6")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Chores" }));
    await waitFor(() => expect(screen.queryByText("Zulu")).not.toBeInTheDocument());
    expect(screen.getByTestId("location")).toHaveTextContent("/fix?terminal=all&noun=chore");
    first.unmount();

    renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Mike")).toBeInTheDocument();
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/fix?noun=chore&terminal=all"));
    fireEvent.click(screen.getByRole("link", { name: "Bugs" }));
    expect(await screen.findByText("Closed 6")).toBeInTheDocument();
  });

  it("lets a URL that names filters win, and remembers it", async () => {
    window.localStorage.setItem("tiller-fix-filters:acme", "noun=chore");
    const first = renderWorkItems(board, "/fix?noun=bug", "acme");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.queryByText("Mike")).not.toBeInTheDocument();
    await waitFor(() => expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe("noun=bug"));
    first.unmount();

    renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.queryByText("Mike")).not.toBeInTheDocument();
  });

  it("remembers an explicit return to All instead of reviving the old filters", async () => {
    window.localStorage.setItem("tiller-fix-filters:acme", "noun=chore");
    const first = renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Mike")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/fix?noun=chore"));
    const nouns = screen.getByRole("navigation", { name: "Work item type" });
    fireEvent.click(within(nouns).getByRole("link", { name: "All" }));
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    await waitFor(() => expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe(""));
    first.unmount();

    renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.getByText("Mike")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/fix$/);
  });

  it("restores, not erases, the filters when Fix is reopened from the topbar while showing", async () => {
    stubWorkItems(board, "acme");
    render(
      <MemoryRouter initialEntries={["/fix?noun=chore"]}>
        <Link to="/fix">Fix nav</Link>
        <WorkItems />
        <LocationProbe />
      </MemoryRouter>,
    );
    expect(await screen.findByText("Mike")).toBeInTheDocument();
    await waitFor(() => expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe("noun=chore"));
    fireEvent.click(screen.getByRole("link", { name: "Fix nav" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/fix?noun=chore"));
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe("noun=chore");
  });

  it("keeps the All board when the project status cannot be read", async () => {
    window.localStorage.setItem("tiller-fix-filters:acme", "noun=chore");
    renderWorkItems(board, "/fix");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/fix$/);
    expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe("noun=chore");
  });

  it("keeps each project's filters to itself", async () => {
    window.localStorage.setItem("tiller-fix-filters:other", "noun=chore");
    renderWorkItems(board, "/fix", "acme");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.getByText("Mike")).toBeInTheDocument();
    expect(window.localStorage.getItem("tiller-fix-filters:other")).toBe("noun=chore");
  });
});

describe("Fix board", () => {
  it("counts only visible cards through attachment filters and the Closed history window", async () => {
    renderWorkItems({
      ...board,
      open: [...board.open, { noun: "tweak", id: "TW-1", ref: "TW-1", title: "Spacing", status: "ready", attachment_ids: ["F-1"] }],
    }, "/fix?attached=F-1");
    expect(await screen.findByText("Showing 4 bugs, 3 chores, and 1 tweak")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("switch", { name: "Show older items" }));
    expect(await screen.findByText("Showing 5 bugs, 4 chores, and 1 tweak")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Tweaks" }));
    expect(await screen.findByText("Showing 0 bugs, 0 chores, and 1 tweak")).toBeInTheDocument();
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("attached=F-1&terminal=all&noun=tweak");
  });

  it("collapses and restores the filter sidebar without losing URL filters", async () => {
    const first = renderWorkItems(board, "/fix?noun=bug&attached=F-1&terminal=all");
    const toggle = await screen.findByRole("button", { name: /Filters/ });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("complementary", { name: "Work item filters" })).not.toBeInTheDocument();
    expect(screen.getByText("Zulu")).toBeInTheDocument();
    expect(screen.queryByText("Mike")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/fix?noun=bug&attached=F-1&terminal=all");
    first.unmount();

    renderWorkItems(board, "/fix?noun=bug&attached=F-1&terminal=all");
    const restored = await screen.findByRole("button", { name: /Filters/ });
    expect(restored).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(restored);
    const filters = screen.getByRole("complementary", { name: "Work item filters" });
    expect(within(filters).getByRole("link", { name: "Bugs" })).toHaveAttribute("aria-current", "true");
    expect(within(screen.getByRole("region", { name: "Closed" })).getAllByRole("link")).toHaveLength(4);
    expect(screen.getByText("Zulu")).toBeInTheDocument();
  });

  it("renders a distinct Tweak label and combines noun selections without losing attachment or closed history", async () => {
    renderWorkItems({
      ...board,
      open: [...board.open, { noun: "tweak", id: "TW-spacing", ref: "TW-spacing", title: "Button spacing", status: "ready", attachment_ids: ["F-1"] }],
      closed: [{ noun: "tweak", id: "TW-old", ref: "TW-old", title: "Old adjustment", status: "done", attachment_ids: ["F-1"] }, ...board.closed],
    }, "/fix?noun=tweak&attached=F-1&terminal=all");
    const ready = await screen.findByRole("region", { name: "Ready" });
    expect(within(ready).getByRole("link", { name: "Button spacing" })).toHaveAttribute("href", "/node/tweak/TW-spacing");
    expect(ready).toHaveTextContent("tweak");
    expect(screen.getByText("Old adjustment")).toBeInTheDocument();
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Bugs" }));
    await waitFor(() => expect(screen.getByText("Zulu")).toBeInTheDocument());
    expect(screen.queryByText("Mike")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("noun=bug%2Ctweak&attached=F-1&terminal=all");
  });
  it("searches Attached to by node type, unions picks into a comma URL, and ORs Unattached", async () => {
    const task: RelatedNode = { noun: "task", id: "T-9", ref: "T-9", status: "ready", title: "Webhook retries" };
    renderWorkItems({
      ...board,
      open: [...board.open, { noun: "chore", id: "CH-t", ref: "CH-t", title: "Tasked", status: "ready", attachment_ids: ["T-9"] }],
      attachments: [attachment, task],
    }, "/fix?attached=F-1");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    expect(screen.queryByText("Tasked")).not.toBeInTheDocument();
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();

    const input = screen.getByRole("combobox", { name: "Attached to" });
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "webhook" } });
    const result = screen.getByRole("option", { name: /Webhook retries/ });
    expect(result).toHaveTextContent("Task");
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(screen.getByText("Tasked")).toBeInTheDocument());
    expect(screen.getByText("Zulu")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/fix?attached=F-1%2CT-9");

    fireEvent.click(screen.getByRole("link", { name: "Unattached" }));
    await waitFor(() => expect(screen.getByText("Alpha")).toBeInTheDocument());
    expect(screen.getByText("Tasked")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/fix?attached=F-1%2CT-9%2Cnone");

    fireEvent.click(within(screen.getByRole("navigation", { name: "Attachment" })).getByRole("link", { name: "All" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(/^\/fix$/));
  });

  it("keeps a transferred item's owner and reciprocal replacement links distinct from duplicate resolution", async () => {
    renderWorkItems({
      open: [{ noun: "bug", id: "B-new", ref: "B-new", title: "Replacement", status: "ready", owner: "sam", supersedes: ["B-old"] }],
      closed: [{ noun: "bug", id: "B-old", ref: "B-old", title: "Original", status: "cancelled", owner: "pat",
        resolution: "transferred", superseded_by: "B-new", closed_at: "2026-09-19T12:00:00Z" }],
      attachments: [],
    }, "/fix");
    const original = (await screen.findByRole("link", { name: "Original" })).closest("article") as HTMLElement;
    const replacement = screen.getByRole("link", { name: "Replacement" }).closest("article") as HTMLElement;
    expect(original).toHaveTextContent("Owner: pat");
    expect(original).toHaveTextContent("transferred");
    expect(within(original).getByRole("link", { name: "B-new" })).toHaveAttribute("href", "/node/bug/B-new");
    expect(replacement).toHaveTextContent("Owner: sam");
    expect(within(replacement).getByRole("link", { name: "B-old" })).toHaveAttribute("href", "/node/bug/B-old");
    expect(original).not.toHaveTextContent("duplicate");
  });

  it("renders the dedicated shelf board and keeps filters in the URL", async () => {
    renderWorkItems(board, "/fix");

    expect(await screen.findByRole("region", { name: "Ready" })).toBeVisible();
    const regions = screen.getAllByRole("region");
    expect(regions.map((region) => region.getAttribute("aria-label"))).toEqual([
      "Proposed",
      "Ready",
      "In progress",
      "Check",
      "In review",
      "Closed",
    ]);
    expect(screen.getByText("Zulu")).toBeInTheDocument();
    expect(screen.getByText("Mike")).toBeInTheDocument();
    expect(screen.getByText("cancelled")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("link", { name: "Bugs" }));
    await waitFor(() => expect(screen.queryByText("Mike")).not.toBeInTheDocument());
    expect(screen.getByTestId("location")).toHaveTextContent("noun=bug");
  });

  it("keeps every column heading in the static heads row, outside the scrolling body", async () => {
    const { container } = renderWorkItems(board, "/fix");
    await screen.findByText("Zulu");

    const heads = container.querySelector(".board-scroll > .board-heads");
    const body = container.querySelector(".board-scroll > .board-body");
    expect(heads?.querySelectorAll("h2")).toHaveLength(6);
    expect(body?.querySelector("h2")).toBeNull();
    expect(within(body as HTMLElement).getByRole("region", { name: "Ready" })).toHaveTextContent("Zulu");
  });

  it("renders accessible card and attachment links with owner and derived activity", async () => {
    renderWorkItems(board, "/fix");

    const ready = await screen.findByRole("region", { name: "Ready" });
    expect(within(ready).getByRole("link", { name: "Zulu" })).toHaveAttribute("href", "/node/bug/B-z");
    expect(within(ready).getByText("awaiting_check")).toBeInTheDocument();
    expect(within(ready).getAllByRole("link", { name: "foundation/console (F-1)" })[0]).toHaveAttribute(
      "href",
      "/node/feature/F-1",
    );
  });

  it("expands the five-row Closed window through the rail's Show older items switch", async () => {
    renderWorkItems(board, "/fix");
    expect(await screen.findAllByText(/Closed /)).toHaveLength(5);

    const filters = screen.getByRole("complementary", { name: "Work item filters" });
    const toggle = within(filters).getByRole("switch", { name: "Show older items" });
    expect(toggle).not.toBeChecked();
    fireEvent.click(toggle);

    await waitFor(() => expect(screen.getAllByText(/Closed /)).toHaveLength(7));
    expect(screen.getByTestId("location")).toHaveTextContent("terminal=all");
    expect(toggle).toBeChecked();

    fireEvent.click(toggle);
    await waitFor(() => expect(screen.getAllByText(/Closed /)).toHaveLength(5));
    expect(screen.getByTestId("location")).not.toHaveTextContent("terminal=all");
  });

  it("shows an endpoint error instead of an empty board", async () => {
    renderWorkItemsError("event stream unreadable");
    expect(await screen.findByText("event stream unreadable")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Ready" })).not.toBeInTheDocument();
  });
});
