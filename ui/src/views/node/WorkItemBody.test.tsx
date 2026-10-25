import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { NodePage } from "../../api";
import { WorkItemGatesPanel, WorkItemSpecPanel } from "./WorkItemBody";

afterEach(() => {
  cleanup();
});

const bugPage: NodePage = {
  node: {
    noun: "bug",
    id: "B-aaaaaa",
    title: "Blank on reload",
    status: "open",
    gates: {
      build: { required: true, status: "pass", against: "git:build111" },
      tests: { required: true, status: "pass", against: "sha256:tests222" },
      "code-review": { required: false, status: "pending", against: "git:review333" },
    },
    pr: 41,
  },
  ref: "B-aaaaaa",
  prose: {
    path: ".tiller/bugs/B-aaaaaa/bug.md",
    content: "# Blank on reload\n\nReload after saving\n",
  },
  relationships: {},
  backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
  history: [],
  eval: null,
  
};

it("renders bug prose and optional gates without an eval contract", () => {
  render(<WorkItemSpecPanel page={bugPage} />);
  expect(screen.getByRole("heading", { name: "Bug contract" })).toBeInTheDocument();
  expect(screen.getByText("Reload after saving")).toBeInTheDocument();

  render(<WorkItemGatesPanel page={bugPage} />);
  expect(screen.getByText("tests")).toBeInTheDocument();
  expect(screen.queryByText("Acceptance criteria")).not.toBeInTheDocument();
});

it("keeps every gate revision in its own card without inventing page-wide acceptance provenance", () => {
  const { container } = render(<WorkItemGatesPanel page={bugPage} />);

  for (const [name, revision] of [
    ["build", "git:build111"],
    ["tests", "sha256:tests222"],
    ["code-review", "git:review333"],
  ]) {
    const card = screen.getByText(name).closest(".instr-gate") as HTMLElement;
    expect(within(card).getByText("checked against")).toBeInTheDocument();
    expect(within(card).getByText(revision)).toBeInTheDocument();
  }

  const provenance = container.querySelector(".instr-provline") as HTMLElement;
  expect(provenance).toHaveTextContent("review #41");
  expect(provenance).toHaveTextContent("policy violations: none");
  expect(provenance).not.toHaveTextContent("checked against");
  expect(provenance).not.toHaveTextContent(/git:build111|sha256:tests222|git:review333/);
});
