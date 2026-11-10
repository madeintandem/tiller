// A History row reads time, verb, actor, and the outcome the event produced.
// The outcome goes through statusLabel in a Chip; the raw command is only the
// row's hover title, and the raw event type is printed nowhere.

import { cleanup, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { HistoryEntry } from "../../api";
import HistorySection from "../../views/node/HistorySection";

afterEach(() => {
  cleanup();
});

function renderRows(history: HistoryEntry[]) {
  return render(
    <MemoryRouter>
      <HistorySection history={history} />
    </MemoryRouter>,
  );
}

const started: HistoryEntry = {
  at: "2026-10-06T12:00:00Z",
  type: "start.task",
  contributor: "jc",
  command: "tiller start task T-cccccc",
  outcome: "in_progress",
};
const linked: HistoryEntry = {
  at: "2026-10-06T11:00:00Z",
  type: "link.task",
  contributor: "jc",
  command: "",
};

describe("History row outcome", () => {
  it("shows the verb and a worded outcome chip, never the raw token", () => {
    const { container } = renderRows([started]);
    const row = container.querySelector(".history-entry") as HTMLElement;
    expect(row).toHaveTextContent("started");
    const chip = row.querySelector(".chip") as HTMLElement;
    expect(chip).toHaveTextContent("In progress");
    expect(chip).toHaveAttribute("data-status", "in_progress");
    expect(row.textContent).not.toContain("in_progress");
  });

  it("moves the command to the row title and no longer prints it", () => {
    const { container } = renderRows([started]);
    const row = container.querySelector(".history-entry") as HTMLElement;
    expect(row).toHaveAttribute("title", "tiller start task T-cccccc");
    expect(row.textContent).not.toContain("tiller start task");
    expect(container.querySelector(".history-command")).toBeNull();
  });

  it("prints the raw event type nowhere, not even in a title", () => {
    const { container } = renderRows([started, linked]);
    expect(container.textContent).not.toContain("start.task");
    expect(container.innerHTML).not.toContain("start.task");
    expect(container.innerHTML).not.toContain("link.task");
  });

  it("draws no chip and no title for a row without an outcome or a command", () => {
    const { container } = renderRows([linked]);
    const row = container.querySelector(".history-entry") as HTMLElement;
    expect(row.querySelector(".chip")).toBeNull();
    expect(row.hasAttribute("title")).toBe(false);
  });
});
