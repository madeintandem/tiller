// The attention stack folds to one box (console UI plan, task 14): the first
// box the stack would render shows, the rest sit behind a "N more" button.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { NodePage, Queue, QueueEntry } from "../../api";
import NodeDetail from "../NodeDetail";

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: () => Promise.resolve() },
    configurable: true,
  });
});

function taskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "task",
      id: "T-h4k2wq",
      key: "foundation/boot/boot-the-app",
      title: "Boot the app",
      status: "in_review",
    },
    ref: "foundation/boot/boot-the-app (T-h4k2wq)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

function ask(command: string): QueueEntry {
  return {
    kind: "attest_criterion",
    noun: "task",
    id: "T-h4k2wq",
    ref: "foundation/boot/boot-the-app (T-h4k2wq)",
    prose: "C1",
    statement: `Statement for ${command}`,
    command,
    actor: "human",
  };
}

const workflow: NonNullable<NodePage["workflow"]> = {
  phase: "acceptance",
  actor: "human",
  action: "attest",
  command: "tiller attest first",
  reason: "Acceptance is pending.",
  review_id: "T-h4k2wq",
  feature_id: "F-1",
  contract_status: "valid",
  acceptance_ready: true,
  submission_recorded: false,
};

function renderPage(page: NodePage, queue: Queue) {
  return render(
    <MemoryRouter>
      <NodeDetail page={page} queue={queue} />
    </MemoryRouter>,
  );
}

describe("NodeDetail: the attention fold", () => {
  it("shows only the first box and a count of the rest, then all on press", () => {
    renderPage(taskPage({ workflow }), { entries: [ask("tiller attest second"), ask("tiller attest third")] });
    expect(screen.getByText("Workflow · Human")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "tiller attest second" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "tiller attest third" })).not.toBeInTheDocument();
    const more = screen.getByRole("button", { name: "Show 2 more" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(more);
    expect(screen.getByRole("button", { name: "tiller attest second" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tiller attest third" })).toBeInTheDocument();
    expect(screen.getByText("Workflow · Human")).toBeInTheDocument();
  });

  // The node page's own "N more" affordance (EpicBody, HistorySection), not a
  // style of its own; its focus ring is the console-wide :focus-visible rule.
  it("dresses the fold button as the node page's existing disclosure", () => {
    renderPage(taskPage({ workflow }), { entries: [ask("tiller attest second")] });
    const more = screen.getByRole("button", { name: "Show 1 more" });
    expect(more).toHaveClass("instr-more");
    expect(more).not.toHaveClass("attn-more");
  });

  // Unfocused, the fold read as plain gray text (browser walk D-F, R5): it
  // is a disclosure control, with a chevron and a verb, and folds back up.
  it("reads as a disclosure control: a verb, a chevron, and a way back to fewer", () => {
    renderPage(taskPage({ workflow }), { entries: [ask("tiller attest second"), ask("tiller attest third")] });
    const more = screen.getByRole("button", { name: "Show 2 more" });
    expect(more).toHaveClass("instr-more-toggle");
    const chevron = more.querySelector(".instr-more-chevron") as HTMLElement;
    expect(chevron).toHaveAttribute("aria-hidden", "true");
    expect(chevron.textContent).toBe("▸");

    fireEvent.click(more);
    const fewer = screen.getByRole("button", { name: "Show fewer" });
    expect(fewer).toHaveAttribute("aria-expanded", "true");
    expect(fewer.querySelector(".instr-more-chevron")?.textContent).toBe("▾");
    expect(screen.getByRole("button", { name: "tiller attest third" })).toBeInTheDocument();

    fireEvent.click(fewer);
    expect(screen.queryByRole("button", { name: "tiller attest third" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 2 more" })).toHaveAttribute("aria-expanded", "false");
  });

  it("offers no button for exactly one box", () => {
    renderPage(taskPage(), { entries: [ask("tiller attest second")] });
    expect(screen.getByRole("button", { name: "tiller attest second" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /more$/ })).not.toBeInTheDocument();
  });

  it("folds the followers of the first ask when there is no workflow box", () => {
    renderPage(taskPage(), { entries: [ask("tiller attest second"), ask("tiller attest third")] });
    expect(screen.getByRole("button", { name: "tiller attest second" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "tiller attest third" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 1 more" })).toBeInTheDocument();
  });
});
