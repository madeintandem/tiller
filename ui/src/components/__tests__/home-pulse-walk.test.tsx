// Home's pulse after the browser walk of batches A to C: one owner's In
// flight group gives a feature one heading across its active, in-review, and
// blocked rows, and an Up next row for a skill offers the guide command the
// CLI prints for it.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { NextAnswer, NextCandidate, Streams } from "../../api";
import type { FetchState } from "../../hooks";
import { InFlightColumn, UpNextColumn } from "../../views/home/Pulse";

afterEach(() => {
  cleanup();
});

function settled<T>(data: T): FetchState<T> {
  return { data, error: null, loading: false };
}

const feature = { feature_id: "F-1", feature_title: "Routing layer" };

describe("In flight: one heading per feature under one owner", () => {
  it("puts an owner's in-progress and in-review tasks of one feature under one heading, active first", () => {
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "robin", name: "Robin" },
          active: [
            { noun: "task", id: "T-1", key: "core/routing/extract", ref: "T-1", title: "Extract params", status: "in_progress", display: "in_progress", ...feature },
          ],
          gates_in_review: [
            { noun: "task", id: "T-2", key: "core/routing/guard", ref: "T-2", title: "Method guard", ...feature },
          ],
          blocked: [
            { noun: "task", id: "T-3", key: "core/routing/cache", ref: "T-3", title: "Route cache", depends_on_refs: ["T-2"], ...feature },
          ],
          open_questions: [],
        },
      ],
    };
    const { container } = render(<MemoryRouter><InFlightColumn state={settled(streams)} /></MemoryRouter>);
    const headings = container.querySelectorAll(".pulse-feature");
    expect(headings).toHaveLength(1);
    expect(headings[0]).toHaveTextContent("Routing layer");
    const group = headings[0].parentElement as HTMLElement;
    const rows = Array.from(group.querySelectorAll(".prow")).map((r) => r.textContent ?? "");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toContain("Extract params");
    expect(rows[1]).toContain("Method guard");
    expect(rows[2]).toContain("Route cache");
  });
});

describe("Up next: a skill row's copy control", () => {
  function upNext(candidates: NextCandidate[]): NextAnswer {
    return { reader: "jc", candidates, caught_up: false };
  }

  it("copies the guide command when the move has a skill and no command", () => {
    const { container } = render(
      <MemoryRouter>
        <UpNextColumn
          state={settled(upNext([
            {
              actor: "agent", kind: "specify", action: "specify", skill: "specify",
              guide: "tiller guide specify", reason: "draft, with every dependency met", tier: 5,
              node: { noun: "feature", id: "F-9", ref: "F-9", title: "Refunds" },
            },
          ]))}
        />
      </MemoryRouter>,
    );
    const row = container.querySelector(".prow") as HTMLElement;
    expect(within(row).getByRole("button", { name: "tiller guide specify" })).toHaveClass("copy-cmd");
  });

  it("copies the command, never the guide, when the move has a command", () => {
    render(
      <MemoryRouter>
        <UpNextColumn
          state={settled(upNext([
            {
              actor: "human", kind: "approve_feature", action: "approve",
              command: "tiller approve feature billing/refunds", reason: "waiting on your approval", tier: 1,
              node: { noun: "feature", id: "F-9", ref: "F-9", title: "Refunds" },
            },
          ]))}
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole("button", { name: "tiller approve feature billing/refunds" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /tiller guide/ })).not.toBeInTheDocument();
  });
});
