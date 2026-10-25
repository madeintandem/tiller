import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { NodePage } from "../../api";
import QuestionBody from "./QuestionBody";

const QUESTION_TITLE = "Audit selection unit";
const QUESTION_STATEMENT =
  "Should the audit select one record per project or one record per project phase?\n\n" +
  "The answer changes how multi-year projects are counted in review queues\nand exported reports.";

function questionPage(): NodePage {
  return {
    node: {
      noun: "question",
      id: "Q-title",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
      answer: "Count one record per project phase.",
      status: "answered",
    },
    ref: "Q-title",
    prose: null,
    relationships: {
      blocks: [
        {
          noun: "feature",
          id: "F-export",
          key: "audit/export",
          title: "Audit export",
          ref: "audit/export (F-export)",
          status: "draft",
        },
      ],
    },
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    
  };
}

afterEach(() => {
  cleanup();
});

describe("QuestionBody", () => {
  it("renders the complete statement once before blocks and the recorded answer", () => {
    const { container } = render(
      <MemoryRouter>
        <QuestionBody page={questionPage()} />
      </MemoryRouter>,
    );

    const statement = container.querySelector(".question-statement") as HTMLElement;
    const paragraphs = statement.querySelectorAll(":scope > p");
    const blocks = screen.getByRole("heading", { name: /^Blocks/ });
    const answer = screen.getByRole("heading", { name: "Answer" });

    expect(paragraphs).toHaveLength(2);
    expect(paragraphs[0].textContent).toBe(
      "Should the audit select one record per project or one record per project phase?",
    );
    expect(paragraphs[1].textContent).toBe(
      "The answer changes how multi-year projects are counted in review queuesand exported reports.",
    );
    expect(paragraphs[1].querySelectorAll("br")).toHaveLength(1);
    expect(statement.compareDocumentPosition(blocks) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(blocks.compareDocumentPosition(answer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("link", { name: "Audit export" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Audit export" })).toHaveAttribute(
      "href",
      "/node/feature/F-export",
    );
    expect(screen.getByText("Count one record per project phase.")).toBeInTheDocument();
    expect(container.textContent).not.toContain(QUESTION_TITLE);
  });

  it("an open question shows cancel and one unlink per target, not the answer command", () => {
    const page = questionPage();
    page.node.status = "open";
    delete page.node.answer;
    render(
      <MemoryRouter>
        <QuestionBody page={page} />
      </MemoryRouter>,
    );
    expect(screen.getByText('tiller cancel question Q-title --reason "..."')).toBeInTheDocument();
    expect(screen.getByText("tiller unlink question Q-title --blocks F-export")).toBeInTheDocument();
    expect(screen.queryByText(/tiller answer question/)).toBeNull();
  });

  it("a legacy question with no status shows its commands as open", () => {
    const page = questionPage();
    page.node.status = "";
    delete page.node.answer;
    render(
      <MemoryRouter>
        <QuestionBody page={page} />
      </MemoryRouter>,
    );
    expect(screen.getByText('tiller cancel question Q-title --reason "..."')).toBeInTheDocument();
  });

  it("a cancelled question shows its reason and no commands", () => {
    const page = questionPage();
    page.node.status = "cancelled";
    delete page.node.answer;
    page.node.cancel_reason = "Folded into the plan.";
    const { container } = render(
      <MemoryRouter>
        <QuestionBody page={page} />
      </MemoryRouter>,
    );
    expect(screen.getByRole("heading", { name: "Cancelled" })).toBeInTheDocument();
    expect(screen.getByText("Folded into the plan.")).toBeInTheDocument();
    expect(container.textContent).not.toContain("tiller cancel");
    expect(container.textContent).not.toContain("tiller unlink");
  });
});
