// Raw words the browser walk of batches D to F still found on screen: an
// amendment's op and precondition tokens, a work item's resolution, a
// feature's change type, and the execution note's jargon.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { ExecutionRecord, NodePage } from "../../api";
import AmendmentBody from "../../views/node/AmendmentBody";
import NodeDetail from "../NodeDetail";

afterEach(() => {
  cleanup();
});

const target = {
  noun: "feature",
  id: "F-1",
  key: "foundation/app-scaffold",
  ref: "foundation/app-scaffold (F-1)",
  status: "done",
  title: "App scaffold",
};

function amendmentPage(): NodePage {
  return {
    node: { noun: "amendment", id: "A-1", key: "fix-eval", title: "Fix the eval", status: "proposed" },
    ref: "fix-eval (A-1)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    amendment: {
      operations: [
        {
          index: 0,
          op: "correct-eval",
          target,
          preconditions: [{ name: "eval_revision", expected: "git:abc", live: { status: "valid" } }],
        },
        { index: 1, op: "add-feature", key: "foundation/new-thing", title: "New thing", change_type: "modify" },
      ],
    },
  };
}

describe("Amendment page words", () => {
  it("names an op in the words History uses and a precondition in plain words", () => {
    const { container } = render(
      <MemoryRouter>
        <AmendmentBody page={amendmentPage()} />
      </MemoryRouter>,
    );
    expect(screen.getByText("Feature eval corrected")).toBeInTheDocument();
    expect(screen.getByText("Eval revision")).toBeInTheDocument();
    expect(screen.getByText("Feature added")).toBeInTheDocument();
    expect(screen.getByText("Changes existing code")).toBeInTheDocument();
    const text = container.textContent ?? "";
    for (const token of ["correct-eval", "eval_revision", "add-feature"]) {
      expect(text).not.toContain(token);
    }
  });
});

function featurePage(overrides: Partial<NodePage["node"]> = {}): NodePage {
  return {
    node: { noun: "feature", id: "F-pe4rtc", key: "foundation/routing", title: "Routing", status: "ready", ...overrides },
    ref: "foundation/routing (F-pe4rtc)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
  };
}

describe("Node header words", () => {
  it("states a feature's change type in plain words", () => {
    const { container } = render(
      <MemoryRouter>
        <NodeDetail page={featurePage({ change_type: "create" })} />
      </MemoryRouter>,
    );
    expect(screen.getAllByText("New code").length).toBeGreaterThan(0);
    expect(container.textContent).not.toMatch(/Change\s*create/i);
  });

  it("states a closed item's resolution in plain words", () => {
    render(
      <MemoryRouter>
        <NodeDetail
          page={{
            ...featurePage(),
            node: { noun: "bug", id: "B-1", title: "Crash", status: "cancelled", resolution: "wontfix" },
            ref: "B-1",
          }}
        />
      </MemoryRouter>,
    );
    expect(screen.getAllByText("Won't fix").length).toBeGreaterThan(0);
    expect(screen.queryByText("wontfix")).toBeNull();
  });
});

describe("Execution note", () => {
  it("states the served checkout's status and that the branch view does not make commands safe there", () => {
    const observation: ExecutionRecord = {
      id: "T-one",
      key: "epic/feature/task",
      branch: "task-one",
      source: "local_branch",
      revision: "abcdef123456",
      observed_at: "2026-10-06T12:00:00Z",
      uncommitted: false,
      status: "in_review",
      owner: "developer",
      integration_status: "in_review",
    };
    const page: NodePage = {
      node: { noun: "task", id: "T-one", status: "in_review" },
      ref: "T-one",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      execution: observation,
    };
    const { container } = render(
      <MemoryRouter>
        <NodeDetail page={page} />
      </MemoryRouter>,
    );
    const note = container.querySelector(".execution-boundary");
    expect(note?.textContent).toBe(
      "On the checkout this console serves, this task is in review. What is shown here comes from the branch, and it does not make lifecycle commands safe to run on the served checkout.",
    );
  });
});
