// A criterion deep link (?criterion=C2) marks and scrolls to one Contract row
// on arrival. A tab switch drops it, so coming back to Contract does not
// scroll again and the address no longer names a criterion the reader left.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { NodePage } from "../../api";
import NodeDetail from "../NodeDetail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function page(): NodePage {
  return {
    node: { noun: "feature", id: "F-1", key: "billing/refunds", title: "Refund pipeline", status: "ready", gates: { evals: { required: true, status: "pending" } } },
    ref: "billing/refunds (F-1)",
    prose: { path: ".tiller/epics/billing/features/refunds/feature.md", content: "# Refund pipeline\n\nSpec body text.\n" },
    relationships: { tasks: [{ noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Wire the refund job" }] },
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: {
      path: "features/billing/refunds/feature.eval.md",
      node: "F-1",
      criteria: [
        { id: "C1", statement: "A refund posts once.", polarity: "introduce", required: true, binding: { type: "test", ref: "spec/refund_spec.rb" } },
        { id: "C2", statement: "A duplicate is refused.", polarity: "introduce", required: true, binding: { type: "manual" } },
      ],
    },
  } as NodePage;
}

function Where() {
  const location = useLocation();
  return <p data-testid="where">{`${location.pathname}${location.search}`}</p>;
}

describe("NodeDetail tab switches and the criterion deep link", () => {
  it("drops criterion= on a tab switch, so returning to Contract does not scroll again", () => {
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => undefined);
    render(
      <MemoryRouter initialEntries={["/node/feature/F-1?tab=evals&criterion=C2"]}>
        <NodeDetail page={page()} />
        <Where />
      </MemoryRouter>,
    );
    expect(scroll).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("tab", { name: "Tasks · 1" }));
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/node\/feature\/F-1\?tab=tasks$/);

    fireEvent.click(screen.getByRole("tab", { name: "Contract · 2" }));
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/node\/feature\/F-1\?tab=evals$/);
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".is-target")).toBeNull();
  });
});
