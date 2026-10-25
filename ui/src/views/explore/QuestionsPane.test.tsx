import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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

function renderPane(url = "/questions") {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QuestionsPane />
    </MemoryRouter>,
  );
}

const open = {
  noun: "question",
  id: "Q-open",
  ref: "Q-open",
  title: "Which unit?",
  status: "open",
  blocks_refs: [
    { noun: "feature", id: "F-one", ref: "F-one", title: "One", status: "draft" },
    { noun: "feature", id: "F-two", ref: "F-two", title: "Two", status: "draft" },
  ],
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("QuestionsPane", () => {
  it("shows All, Open, Answered, Cancelled tabs even with no cancelled row", () => {
    serve([open] as never);
    renderPane();
    for (const name of [/^All/, /^Open/, /^Answered/, /^Cancelled/]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });

  it("shows the new subtitle", () => {
    serve([open] as never);
    const { container } = renderPane();
    expect(container.querySelector(".sub")?.textContent).toContain(
      "1 recorded · 1 open · answered and cancelled questions stay on the ledger",
    );
  });

  it("an open card has answer, cancel, and one unlink per target", () => {
    serve([open] as never);
    renderPane();
    expect(screen.getByText('tiller answer question Q-open --answer "..."')).toBeInTheDocument();
    expect(screen.getByText('tiller cancel question Q-open --reason "..."')).toBeInTheDocument();
    expect(screen.getByText("tiller unlink question Q-open --blocks F-one")).toBeInTheDocument();
    expect(screen.getByText("tiller unlink question Q-open --blocks F-two")).toBeInTheDocument();
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
    expect(screen.getByText('tiller cancel question Q-open --reason "..."')).toBeInTheDocument();
  });
});
