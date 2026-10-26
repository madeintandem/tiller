// AmendmentBody: the amendment node page's own flat body. Required scenarios:
// the narrative prose, the operations list (op said as a word, target as
// RefLink where resolvable, reason and superseded-by where present),
// preconditions rendered with their validity as a WORD beside the value
// (valid/stale/unknown, never color alone), and triggered_by/amends_design as
// quiet mono lines. No button anywhere applies anything: an amendment's one
// write command lives on the queue card, not this page.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { AmendmentOperationPageView, NodePage, NodeView } from "../../api";
import AmendmentBody from "./AmendmentBody";

afterEach(() => {
  cleanup();
});

const QUESTION_TITLE = "Audit selection unit";
const QUESTION_STATEMENT =
  "Should the audit select one record per project or one record per project phase?\n\n" +
  "The answer changes how multi-year projects are counted in review queues\nand exported reports.";

function withRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

function amendmentNode(overrides: Partial<NodeView> = {}): NodeView {
  return {
    noun: "amendment",
    id: "A-1",
    key: "iphone-only",
    title: "Remove native iPad support from v1",
    status: "proposed",
    ...overrides,
  };
}

function reviseOp(overrides: Partial<AmendmentOperationPageView> = {}): AmendmentOperationPageView {
  return {
    index: 0,
    op: "revise",
    target: {
      noun: "feature",
      id: "F-1",
      key: "form-engine/schema-engine",
      ref: "form-engine/schema-engine (F-1)",
      status: "draft",
      title: "Schema engine",
    },
    ...overrides,
  };
}

function amendmentPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: amendmentNode(),
    ref: "iphone-only (A-1)",
    prose: { path: ".tiller/amendments/A-1-iphone-only.md", content: "Why this reshape exists." },
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    amendment: { operations: [reviseOp()] },
    ...overrides,
  };
}

describe("AmendmentBody: narrative prose", () => {
  it("shows incomplete history even when no operations can supply an identity note", () => {
    withRouter(
      <AmendmentBody
        page={amendmentPage({
          amendment: { operations: [] },
          relationships: { amendment_history_warning: "Amendment history could not be loaded." },
        })}
      />,
    );
    expect(screen.getByText("Amendment history could not be loaded.")).toBeInTheDocument();
  });

  it("renders the amendment's own narrative through ProseSection", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.getByText("Why this reshape exists.")).toBeInTheDocument();
    // The path line under the head is gone (node header pass, 2026-08-30):
    // the header's ref line addresses the node, and the prose simply begins.
    expect(screen.queryByText(".tiller/amendments/A-1-iphone-only.md")).toBeNull();
  });
});

describe("AmendmentBody: operations list", () => {
  it("links proven question block targets and leaves unresolved staged keys unlinked", () => {
    const op: AmendmentOperationPageView = {
      index: 0,
      op: "add-question",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
      blocks: [
        { noun: "task", id: "T-blocked", ref: "Current task (T-blocked)", status: "ready" },
        { noun: "", id: "", key: "foundation/future", ref: "foundation/future", status: "" },
      ],
    };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByRole("link", { name: "Current task (T-blocked)" })).toHaveAttribute(
      "href",
      "/node/task/T-blocked",
    );
    expect(screen.getByText("foundation/future")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "foundation/future" })).not.toBeInTheDocument();
  });

  it("says the op as a plain word and links a resolvable target", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.getByText("revise")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "form-engine/schema-engine (F-1)" });
    expect(link).toHaveAttribute("href", "/node/feature/F-1");
  });

  it("renders a correct-eval like a revise, with its reason and each retired criterion id", () => {
    const op = reviseOp({
      op: "correct-eval",
      reason: "The loader test moved into two files.",
      retire: ["C-3", "C-7"],
    });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("correct-eval")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "form-engine/schema-engine (F-1)" })).toHaveAttribute("href", "/node/feature/F-1");
    expect(screen.getByText("The loader test moved into two files.")).toBeInTheDocument();
    expect(screen.getByText("retires")).toBeInTheDocument();
    expect(screen.getByText("C-3")).toBeInTheDocument();
    expect(screen.getByText("C-7")).toBeInTheDocument();
  });

  it("draws no retires line for a correct-eval that retires nothing", () => {
    const op = reviseOp({ op: "correct-eval", reason: "Rebound the bindings." });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("Rebound the bindings.")).toBeInTheDocument();
    expect(screen.queryByText("retires")).not.toBeInTheDocument();
  });

  it("renders a supersede's target and replacement, both as links", () => {
    const op = reviseOp({
      op: "supersede",
      target: {
        noun: "task",
        id: "T-1",
        key: "form-engine/schema-engine/parse",
        ref: "form-engine/schema-engine/parse (T-1)",
        status: "ready",
        title: "Parse the schema",
      },
      by: {
        noun: "task",
        id: "T-2",
        key: "form-engine/schema-engine/parse-v2",
        ref: "form-engine/schema-engine/parse-v2 (T-2)",
        status: "draft",
        title: "Parse the schema v2",
      },
    });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByRole("link", { name: "form-engine/schema-engine/parse (T-1)" })).toHaveAttribute(
      "href",
      "/node/task/T-1",
    );
    expect(
      screen.getByRole("link", { name: "form-engine/schema-engine/parse-v2 (T-2)" }),
    ).toHaveAttribute("href", "/node/task/T-2");
  });

  it("renders a cancel's reason and its superseded-by link when present", () => {
    const op = reviseOp({
      op: "cancel",
      reason: "Scope dropped from v1.",
      superseded_by: {
        noun: "task",
        id: "T-3",
        key: "form-engine/schema-engine/parse-v3",
        ref: "form-engine/schema-engine/parse-v3 (T-3)",
        status: "draft",
        title: "Parse the schema v3",
      },
    });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("Scope dropped from v1.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "form-engine/schema-engine/parse-v3 (T-3)" })).toBeInTheDocument();
  });

  it("renders a cancel with no replacement and no superseded-by line at all", () => {
    const op = reviseOp({ op: "cancel", target: reviseOp().target, reason: "No longer needed." });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("No longer needed.")).toBeInTheDocument();
    expect(screen.queryByText(/superseded by/)).not.toBeInTheDocument();
  });

  it("renders an add-task's staged key and title, with no target link (nothing live to link to)", () => {
    const op: AmendmentOperationPageView = {
      index: 1,
      op: "add-task",
      key: "form-engine/schema-engine/extra-task",
      title: "Extra task",
    };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("form-engine/schema-engine/extra-task")).toBeInTheDocument();
    expect(screen.getByText("Extra task")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders an add-epic's staged key and title, like add-feature's own arm, with no target link", () => {
    const op: AmendmentOperationPageView = {
      index: 1,
      op: "add-epic",
      key: "hardening",
      title: "Hardening",
    };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("hardening")).toBeInTheDocument();
    expect(screen.getByText("Hardening")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("leads an add-question operation with its title and renders the statement as subordinate detail", () => {
    const op: AmendmentOperationPageView = {
      index: 1,
      op: "add-question",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
    };
    const { container } = withRouter(
      <AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />,
    );

    const row = container.querySelector(".instr-amendop") as HTMLElement;
    const title = within(row).getByText(QUESTION_TITLE);
    const detail = row.querySelector(".instr-amendop-detail") as HTMLElement;
    const paragraphs = detail.querySelectorAll(":scope > p");
    expect(title.closest(".instr-amendop-head")).not.toBeNull();
    expect(paragraphs).toHaveLength(2);
    expect(paragraphs[0].textContent).toBe(
      "Should the audit select one record per project or one record per project phase?",
    );
    expect(paragraphs[1].textContent).toBe(
      "The answer changes how multi-year projects are counted in review queuesand exported reports.",
    );
    expect(paragraphs[1].querySelectorAll("br")).toHaveLength(1);
    expect(title.compareDocumentPosition(detail) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(row).queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders an unknown future op kind as the bare op word via the default safety-net arm, no crash", () => {
    const op: AmendmentOperationPageView = { index: 2, op: "future-kind" };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("future-kind")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("draws no Operations section at all when none are staged yet", () => {
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [] } })} />);
    expect(screen.queryByText(/Operations/)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: per-op source citation", () => {
  it("renders a staged op's own source citation as a quiet mono line", () => {
    const op = reviseOp({ source: "docs/feedback/opus-review.md#L12-18" });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("docs/feedback/opus-review.md#L12-18")).toBeInTheDocument();
  });

  it("renders no source line when the op carries none", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.queryByText(/opus-review/)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: precondition validity, words beside values", () => {
  it("renders valid, stale, and unknown as their own words, never color alone", () => {
    const op = reviseOp({
      preconditions: [
        {
          name: "target_status",
          expected: "draft",
          live: { status: "valid", actual: "draft" },
        },
        {
          name: "key_absent",
          expected: "form-engine/schema-engine",
          live: { status: "stale", actual: "F-1", note: "form-engine/schema-engine (F-1) already holds that key" },
        },
      ],
    });
    const unknownOp: AmendmentOperationPageView = {
      index: 1,
      op: "add-task",
      key: "form-engine/schema-engine/extra-task",
      title: "Extra task",
      preconditions: [
        {
          name: "target_status",
          expected: "done",
          live: { status: "unknown", note: "the operation names no target to read a status from" },
        },
      ],
    };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op, unknownOp] } })} />);

    expect(screen.getByText("valid")).toBeInTheDocument();
    expect(screen.getByText("stale")).toBeInTheDocument();
    expect(screen.getByText("unknown")).toBeInTheDocument();
    expect(screen.getByText("form-engine/schema-engine (F-1) already holds that key")).toBeInTheDocument();
    expect(screen.getByText("the operation names no target to read a status from")).toBeInTheDocument();
  });

  it("renders a precondition with no live fill (the serve handler never ran) with no validity word", () => {
    const op = reviseOp({
      preconditions: [{ name: "target_status", expected: "draft" }],
    });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText("target_status")).toBeInTheDocument();
    expect(screen.queryByText("valid")).not.toBeInTheDocument();
    expect(screen.queryByText("stale")).not.toBeInTheDocument();
    expect(screen.queryByText("unknown")).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: triggered_by and amends_design", () => {
  it("renders both lists as quiet mono lines when present", () => {
    withRouter(
      <AmendmentBody
        page={amendmentPage({
          amendment: {
            operations: [reviseOp()],
            triggered_by: ["design/solution-design/09-audit-engine.md"],
            amends_design: ["design/solution-design/03-platform-scope.md"],
          },
        })}
      />,
    );
    expect(screen.getByText("design/solution-design/09-audit-engine.md")).toBeInTheDocument();
    expect(screen.getByText("design/solution-design/03-platform-scope.md")).toBeInTheDocument();
  });

  it("draws neither line when both are absent", () => {
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [reviseOp()] } })} />);
    expect(screen.queryByText(/triggered by/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/amends design/i)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: the amendment-level source and its live validity", () => {
  it("renders an intake-minted amendment's own source path beside triggered_by/amends_design", () => {
    withRouter(
      <AmendmentBody
        page={amendmentPage({
          amendment: {
            operations: [reviseOp()],
            source: { path: ".tiller/feedback/review.md", revision: "git:abc123" },
          },
        })}
      />,
    );
    expect(screen.getByText(".tiller/feedback/review.md")).toBeInTheDocument();
  });

  it("renders the source's live validity as its own word, never color alone", () => {
    withRouter(
      <AmendmentBody
        page={amendmentPage({
          amendment: {
            operations: [reviseOp()],
            source: {
              path: ".tiller/feedback/review.md",
              revision: "git:abc123",
              live: { status: "stale", note: "review.md has changed since intake" },
            },
          },
        })}
      />,
    );
    expect(screen.getByText("stale")).toBeInTheDocument();
    expect(screen.getByText("review.md has changed since intake")).toBeInTheDocument();
  });

  it("renders no source line at all for a hand-opened amendment", () => {
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [reviseOp()] } })} />);
    expect(screen.queryByText(/\.tiller\/feedback/)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: validity_error banner (#57)", () => {
  it("renders a banner naming validity_error when the batch cannot resolve", () => {
    withRouter(
      <AmendmentBody
        page={amendmentPage({
          amendment: {
            operations: [reviseOp()],
            validity_error: "mutate: revise target F-vanished not found",
          },
        })}
      />,
    );
    expect(screen.getByText(/mutate: revise target F-vanished not found/)).toBeInTheDocument();
  });

  it("renders no banner when validity_error is absent", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.queryByText(/live validity unavailable/i)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: per-op source_missing and shape_error markers (#57)", () => {
  it("renders a source-missing marker on a flagged operation", () => {
    const op = reviseOp({ source_missing: true });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText(/source missing/i)).toBeInTheDocument();
  });

  it("renders no source-missing marker when the flag is absent", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.queryByText(/source missing/i)).not.toBeInTheDocument();
  });

  it("renders the duplicate-staged-key shape_error as its own note", () => {
    const op = reviseOp({ shape_error: "duplicate_staged_key" });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText(/duplicate staged key/i)).toBeInTheDocument();
  });

  it("renders the staged-epic-replacement shape_error as its own note", () => {
    const op = reviseOp({ shape_error: "staged_epic_replacement" });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText(/staged epic replacement/i)).toBeInTheDocument();
  });

  it("renders no shape marker when shape_error is absent", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.queryByText(/duplicate staged key/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/staged epic replacement/i)).not.toBeInTheDocument();
  });

  it("renders both markers at once on a single operation carrying both defects", () => {
    const op = reviseOp({ source_missing: true, shape_error: "duplicate_staged_key" });
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByText(/source missing/i)).toBeInTheDocument();
    expect(screen.getByText(/duplicate staged key/i)).toBeInTheDocument();
  });
});

describe("AmendmentBody: no invented commands", () => {
  it("renders no button and no CLI command anywhere on the page", () => {
    withRouter(<AmendmentBody page={amendmentPage()} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.queryByText(/tiller /)).not.toBeInTheDocument();
  });
});

describe("AmendmentBody: identity provenance", () => {
  it("links recorded created nodes and parents by ID after key reuse, and leaves unresolved identity as a note", () => {
    const op: AmendmentOperationPageView = {
      index: 0,
      op: "add-task",
      key: "billing/refunds/old-key",
      title: "Staged task title",
      created: { noun: "task", id: "T-original", key: "billing/refunds/renamed", ref: "billing/refunds/renamed (T-original)", status: "done" },
      parent: { noun: "feature", id: "F-original", key: "billing/renamed", ref: "billing/renamed (F-original)", status: "done" },
      identity_notes: ["The question addition did not prove a created question identity."],
    };
    withRouter(<AmendmentBody page={amendmentPage({ amendment: { operations: [op] } })} />);
    expect(screen.getByRole("link", { name: "billing/refunds/renamed (T-original)" })).toHaveAttribute("href", "/node/task/T-original");
    expect(screen.getByRole("link", { name: "billing/renamed (F-original)" })).toHaveAttribute("href", "/node/feature/F-original");
    expect(screen.getByText("billing/refunds/old-key").closest("a")).toBeNull();
    expect(screen.getByText(op.identity_notes![0])).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
