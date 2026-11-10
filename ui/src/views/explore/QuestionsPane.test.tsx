import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult } from "../../api";
import QuestionsPane from "./QuestionsPane";

vi.mock("../../hooks", async (orig) => {
  const actual = await orig<typeof import("../../hooks")>();
  return { ...actual, useFetch: vi.fn() };
});
import { useFetch } from "../../hooks";

function serve(nodes: ListResult["nodes"]) {
  vi.mocked(useFetch).mockReturnValue({
    data: { nodes, count: nodes.length } as unknown as ListResult,
    error: null,
  } as never);
}

function Where() {
  return <output data-testid="where">{useLocation().search}</output>;
}

function renderPane(url = "/questions") {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QuestionsPane />
      <Where />
    </MemoryRouter>,
  );
}

const pressed = () => screen.getByRole("tab", { selected: true }).textContent;

const open = {
  noun: "question",
  id: "Q-open",
  ref: "Q-open",
  title: "Which unit?",
  statement: "Which **unit** do we use for the quota, bytes or rows?",
  status: "open",
  commands: {
    answer: 'tiller answer question Q-open --answer "..."',
    cancel: 'tiller cancel question Q-open --reason "..."',
    unlink: ["tiller unlink question Q-open --blocks F-one", "tiller unlink question Q-open --blocks F-two"],
  },
  blocks_refs: [
    { noun: "feature", id: "F-one", ref: "F-one", title: "One", status: "draft" },
    { noun: "feature", id: "F-two", ref: "F-two", title: "Two", status: "draft" },
  ],
};

const answered = {
  noun: "question",
  id: "Q-done",
  ref: "Q-done",
  title: "Which store?",
  statement: "Which store holds the cache?",
  status: "answered",
  answer: "Redis",
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("QuestionsPane", () => {
  it("keeps its header and status tabs above the list, which scrolls on its own", () => {
    serve([open, answered]);
    const { container } = renderPane();
    const pane = container.querySelector(".explore-detail");
    expect(pane?.classList.contains("explore-ledger")).toBe(true);
    // The rail and the panel are the pane's own children, so the stylesheet
    // can hold the rail still and scroll only the panel.
    expect(screen.getByRole("tablist").parentElement).toBe(pane);
    expect(screen.getByRole("tabpanel").parentElement).toBe(pane);
    expect(container.querySelector(".node-pin")).toBeNull();
  });

  it("shows All, Open, Answered, Cancelled tabs even with no cancelled row", () => {
    serve([open] as never);
    renderPane();
    for (const name of [/^All/, /^Open/, /^Answered/, /^Cancelled/]) {
      expect(screen.getByRole("tab", { name })).toBeInTheDocument();
    }
  });

  it("shows the new subtitle", () => {
    serve([open] as never);
    const { container } = renderPane();
    expect(container.querySelector(".sub")?.textContent).toContain(
      "1 recorded · 1 open · answered and cancelled questions stay on the ledger",
    );
  });

  describe("which tab it opens on", () => {
    it("opens on Open when any question is open", () => {
      serve([answered, open] as never);
      renderPane();
      expect(pressed()).toMatch(/^Open/);
      expect(screen.queryByText("Which store?")).not.toBeInTheDocument();
    });

    it("opens on All when none is open", () => {
      serve([answered] as never);
      renderPane();
      expect(pressed()).toMatch(/^All/);
      expect(screen.getByText("Which store?")).toBeInTheDocument();
    });

    it("?status=<known> selects it, ?status=all selects All, an unknown value reads as no parameter", () => {
      serve([answered, open] as never);
      renderPane("/questions?status=answered");
      expect(pressed()).toMatch(/^Answered/);
      cleanup();
      renderPane("/questions?status=all");
      expect(pressed()).toMatch(/^All/);
      cleanup();
      renderPane("/questions?status=bogus");
      expect(pressed()).toMatch(/^Open/);
    });

    it("pressing each tab writes it to the URL, All as status=all, and the page follows", () => {
      serve([answered, open] as never);
      renderPane();
      fireEvent.click(screen.getByRole("tab", { name: /^All/ }));
      expect(screen.getByTestId("where").textContent).toBe("?status=all");
      expect(pressed()).toMatch(/^All/);
      fireEvent.click(screen.getByRole("tab", { name: /^Answered/ }));
      expect(screen.getByTestId("where").textContent).toBe("?status=answered");
      expect(pressed()).toMatch(/^Answered/);
      fireEvent.click(screen.getByRole("tab", { name: /^Open/ }));
      expect(screen.getByTestId("where").textContent).toBe("?status=open");
      expect(pressed()).toMatch(/^Open/);
    });
  });

  describe("an open card", () => {
    it("shows the statement as a clamped body under the title, with no raw markdown markers", () => {
      serve([open] as never);
      const { container } = renderPane();
      const body = container.querySelector(".q-statement") as HTMLElement;
      expect(body).not.toBeNull();
      expect(body.textContent).toBe("Which unit do we use for the quota, bytes or rows?");
      expect(body.textContent).not.toContain("**");
      expect(within(body).getByText("unit").tagName).toBe("STRONG");
      expect(screen.getByRole("link", { name: "Which unit?" })).toBeInTheDocument();
    });

    it("keeps a multi-paragraph statement's paragraphs apart", () => {
      serve([{ ...open, statement: "First part?\n\nSecond part." }] as never);
      const { container } = renderPane();
      const paras = container.querySelectorAll(".q-statement p");
      expect(Array.from(paras).map((p) => p.textContent)).toEqual(["First part?", "Second part."]);
    });

    it("shows the server's answer command and keeps the other commands in a closed disclosure", () => {
      serve([open] as never);
      const { container } = renderPane();
      expect(screen.getByText('tiller answer question Q-open --answer "..."')).toBeInTheDocument();
      const more = container.querySelector("details.q-more") as HTMLDetailsElement;
      expect(more).not.toBeNull();
      expect(more.open).toBe(false);
      expect(within(more).getByText("More commands")).toBeInTheDocument();
      expect(within(more).getByText('tiller cancel question Q-open --reason "..."')).toBeInTheDocument();
      expect(within(more).getByText("tiller unlink question Q-open --blocks F-one")).toBeInTheDocument();
      expect(within(more).getByText("tiller unlink question Q-open --blocks F-two")).toBeInTheDocument();
      expect(more.contains(screen.getByText('tiller answer question Q-open --answer "..."'))).toBe(false);
    });

    it("composes no command of its own: a row carrying none shows none", () => {
      serve([{ ...open, commands: undefined }] as never);
      const { container } = renderPane();
      expect(container.textContent).not.toContain("tiller ");
      expect(container.querySelector("details.q-more")).toBeNull();
    });

    it("lists the blocked features as linked titles with the key in mono", () => {
      serve([open] as never);
      const { container } = renderPane();
      const list = container.querySelector("ul.q-blocks") as HTMLElement;
      expect(list).not.toBeNull();
      const items = within(list).getAllByRole("listitem");
      expect(items).toHaveLength(2);
      expect(within(items[0]).getByRole("link", { name: "One" })).toBeInTheDocument();
      expect(items[0].querySelector(".mono")?.textContent).toBe("F-one");
      expect(items[0].textContent).not.toContain(",");
    });
  });

  it("an answered card shows the statement and the answer and no commands", () => {
    serve([answered] as never);
    const { container } = renderPane("/questions?status=answered");
    expect(screen.getByText("Which store holds the cache?")).toBeInTheDocument();
    expect(screen.getByText("Redis")).toBeInTheDocument();
    expect(container.textContent).not.toContain("tiller ");
  });

  it("a cancelled card shows its reason and no commands", () => {
    serve([
      { noun: "question", id: "Q-c", ref: "Q-c", title: "Moot", status: "cancelled", cancel_reason: "No longer needed" },
    ] as never);
    const { container } = renderPane("/questions?status=cancelled");
    expect(screen.getByText("No longer needed")).toBeInTheDocument();
    expect(container.textContent).not.toContain("tiller cancel");
  });

  it("a legacy question with no status counts and shows as open", () => {
    serve([{ ...open, status: "" }] as never);
    const { container } = renderPane();
    expect(container.querySelector(".sub")?.textContent).toContain("1 open");
    expect(pressed()).toMatch(/^Open/);
    expect(screen.getByText('tiller answer question Q-open --answer "..."')).toBeInTheDocument();
  });
});
