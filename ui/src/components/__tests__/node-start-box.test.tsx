// The start box (console UI plan, task 12): a ready task's attention stack
// shows how to start it, the claim and start commands as copy controls, but
// only when nothing else already speaks for the node.

import { cleanup, render, screen } from "@testing-library/react";
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

function readyTaskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "task",
      id: "T-h4k2wq",
      key: "foundation/boot/boot-the-app",
      title: "Boot the app",
      status: "ready",
    },
    ref: "foundation/boot/boot-the-app (T-h4k2wq)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    start: {
      actor: "agent",
      action: "claim and start",
      command: "tiller claim task foundation/boot/boot-the-app",
      then: "tiller start task foundation/boot/boot-the-app",
    },
    ...overrides,
  };
}

function renderPage(page: NodePage, queue: Queue | null = { entries: [] }) {
  return render(
    <MemoryRouter>
      <NodeDetail page={page} queue={queue} />
    </MemoryRouter>,
  );
}

describe("NodeDetail: the start box", () => {
  it("labels the box Next and shows the claim and start commands as copy controls", () => {
    renderPage(readyTaskPage());
    expect(screen.getByText("Next · Agent")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tiller claim task foundation/boot/boot-the-app" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tiller start task foundation/boot/boot-the-app" })).toBeInTheDocument();
  });

  it("shows only the start command for a claimed task, naming its owner", () => {
    const { container } = renderPage(readyTaskPage({
      node: {
        noun: "task", id: "T-h4k2wq", key: "foundation/boot/boot-the-app", title: "Boot the app",
        status: "ready", owner: "robin",
      },
      start: { actor: "agent", action: "start", command: "tiller start task foundation/boot/boot-the-app" },
    }));
    expect(screen.getByText("Next · Agent")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^tiller (claim|start) task/ })).toHaveLength(1);
    expect(container.querySelector(".abox .why")?.textContent).toBe("Claimed by robin. Their next step:");
    expect(screen.queryByText("Ready. Start it.")).not.toBeInTheDocument();
  });

  it("waits for the queue to load", () => {
    renderPage(readyTaskPage(), null);
    expect(screen.queryByText("Next · Agent")).not.toBeInTheDocument();
  });

  it("shows for a loaded, empty queue", () => {
    renderPage(readyTaskPage(), { entries: [] });
    expect(screen.getByText("Next · Agent")).toBeInTheDocument();
  });

  it("renders nothing without a start", () => {
    renderPage(readyTaskPage({ start: undefined }));
    expect(screen.queryByText("Next · Agent")).not.toBeInTheDocument();
  });

  it("yields to a workflow box", () => {
    renderPage(readyTaskPage({
      workflow: {
        phase: "checks",
        actor: "agent",
        action: "check",
        command: "tiller check task T-h4k2wq",
        reason: "Checks have not run.",
        review_id: "T-h4k2wq",
        feature_id: "F-1",
        contract_status: "valid",
        acceptance_ready: false,
        submission_recorded: false,
      },
    }));
    expect(screen.getByText("Workflow · Agent")).toBeInTheDocument();
    expect(screen.queryByText("Next · Agent")).not.toBeInTheDocument();
  });

  it("yields to an ask", () => {
    const ask: QueueEntry = {
      kind: "attest_criterion",
      noun: "task",
      id: "T-h4k2wq",
      ref: "foundation/boot/boot-the-app (T-h4k2wq)",
      prose: "C1",
      command: "tiller attest task foundation/boot/boot-the-app --criterion C1 --status pass",
    };
    renderPage(readyTaskPage(), { entries: [ask] });
    expect(screen.queryByText("Next · Agent")).not.toBeInTheDocument();
  });

  it("stays out of an observed task's page", () => {
    renderPage(readyTaskPage({
      execution: {
        id: "T-h4k2wq", key: "foundation/boot/boot-the-app", branch: "tiller/x", source: "local_branch",
        revision: "abc1234def", observed_at: "2026-11-09T10:00:00Z", uncommitted: false,
        status: "ready", owner: "robin", integration_status: "none",
      },
    }));
    expect(screen.queryByText("Next · Agent")).not.toBeInTheDocument();
  });
});
