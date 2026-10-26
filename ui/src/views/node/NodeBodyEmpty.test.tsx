// The empty-section rule, pinned across the bodies that used to spend a
// heading plus a parenthetical on every absence: a section with nothing in it
// is not drawn (the mock's own draft-state instrument draws no empty Tasks
// list, no empty Relationships, no History), with ONE exception, an absence
// that is genuinely anomalous, which gets a single quiet line.
//
// Plus the history cap: the console's disclosure rule everywhere else (true
// count adjacent, rows one click away, no refetch) applied to a timeline the
// payload serves uncapped.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { HistoryEntry, NodePage } from "../../api";
import HistorySection from "./HistorySection";
import QuestionBody from "./QuestionBody";

afterEach(() => {
  cleanup();
});

function withRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

function event(i: number): HistoryEntry {
  return {
    at: `2026-07-2${i % 10}T10:00:00Z`,
    type: "check.feature",
    contributor: "jc",
    command: `check feature F-${i}`,
  };
}

describe("HistorySection", () => {
  it("draws nothing when the timeline is empty", () => {
    // Every node has at least its own add event, so an empty timeline means
    // the event file failed to load, which is stated where it happens.
    const { container } = withRouter(<HistorySection history={[]} />);
    expect(container.textContent).toBe("");
  });

  it("caps the rows, states the true total, and expands in place", () => {
    const history = Array.from({ length: 30 }, (_, i) => event(i));
    withRouter(<HistorySection history={history} />);

    expect(screen.getByText("History · 30")).toBeInTheDocument();
    expect(document.querySelectorAll(".history-entry")).toHaveLength(8);

    fireEvent.click(screen.getByRole("button", { name: "22 more events ▸" }));

    expect(document.querySelectorAll(".history-entry")).toHaveLength(30);
    expect(screen.queryByRole("button", { name: /more events/ })).not.toBeInTheDocument();
  });

  it("draws no expand when everything already fits", () => {
    withRouter(<HistorySection history={[event(1), event(2)]} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

// TaskFeaturePanel, the parent-feature climb this suite used to cover here,
// was retired (task 2, node-detail-tabs): the climb and its dangling-parent
// note ("Parent feature not found") both moved to the node header's own
// ParentChain, covered by components/__tests__/instrument.test.tsx now
// instead of here.

function questionPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "question", id: "Q-1", status: "open", statement: "Five or six?" },
    ref: "Q-1",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

// The test "offers the answer command
// on an open question, byte-identical to the queue's" DELETED - QuestionBody
// itself never renders the answer command in any state now. Task 17's
// attention stack renders the identical answer_question queue entry as its
// own loud box (components/__tests__/instrument.test.tsx's own "the stack
// owns a question's answer command, never duplicated by the body" coverage);
// this body would otherwise duplicate it verbatim on the same page. The two
// "offers no answer command" tests below still pin real behavior (this body
// draws the command in NO state, open included now), so they stay, reworded.
describe("QuestionBody: never renders the answer command itself", () => {
  it("renders no answer command on an open question - the attention stack owns it now", () => {
    withRouter(<QuestionBody page={questionPage()} />);
    expect(screen.queryByText(/tiller answer question/)).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Answer/ })).not.toBeInTheDocument();
  });

  it("renders no answer command once the question is no longer open", () => {
    withRouter(
      <QuestionBody page={questionPage({ node: { ...questionPage().node, status: "closed" } })} />,
    );
    expect(screen.queryByText(/tiller answer question/)).not.toBeInTheDocument();
  });

  it("renders no answer command beside a recorded answer", () => {
    withRouter(
      <QuestionBody
        page={questionPage({ node: { ...questionPage().node, status: "answered", answer: "Six." } })}
      />,
    );
    expect(screen.queryByText(/tiller answer question/)).not.toBeInTheDocument();
  });
});

describe("QuestionBody", () => {
  it("draws the Answer section once an answer is recorded", () => {
    withRouter(<QuestionBody page={questionPage({ node: { ...questionPage().node, answer: "Six." } })} />);

    expect(screen.getByRole("heading", { name: "Answer" })).toBeInTheDocument();
    expect(screen.getByText("Six.")).toBeInTheDocument();
  });

  it("draws the Blocks section with its true count when the question blocks work", () => {
    withRouter(
      <QuestionBody
        page={questionPage({
          relationships: {
            blocks: [
              {
                noun: "feature",
                id: "F-1",
                key: "audit-engine/selection",
                ref: "audit-engine/selection (F-1)",
                status: "draft",
                title: "Selection policy",
              },
            ],
          },
        })}
      />,
    );

    expect(screen.getByRole("heading", { name: /^Blocks/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Selection policy" })).toBeInTheDocument();
  });
});
