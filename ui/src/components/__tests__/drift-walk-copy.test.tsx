// The browser walk of batches D to F read the Drift report and the node drift
// panel and found sentences that were not true for every node they sat over.
// These pin the corrected copy: the Uncovered section names what could not be
// checked, states the cause once, leads a design row with its title, and the
// footer counts in real singulars and plurals with plain words.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { DriftReportView, NodePage } from "../../api";
import { MARKER_COPY } from "../../lib/drift";
import { DriftReportBody } from "../../views/Drift";
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

function renderReport(report: DriftReportView) {
  return render(
    <MemoryRouter>
      <DriftReportBody report={report} />
    </MemoryRouter>,
  );
}

const unconfirmedDoc: DriftReportView = {
  compared: 18,
  compared_design: 0,
  uncovered: [
    {
      marker: "stale_design",
      noun: "design",
      id: "",
      key: "routing.md",
      title: "Routing design",
      reason: "unconfirmed",
      detail: "no human confirmation of the described code is recorded",
    },
    {
      marker: "stale_spec",
      noun: "feature",
      id: "F-1",
      key: "docs/handbook",
      title: "Handbook",
      reason: "spec_unreadable",
      detail: "could not read .tiller/epics/docs/features/handbook/feature.md: permission denied",
    },
  ],
  coverage: {
    stale_design: { unresolvable: 1 },
    orphan_eval: { checked: 1, out_of_scope: 3, not_yet_in_scope: 10 },
    active_basis: { checked: 1 },
  },
};

describe("Drift report: Uncovered says what could not be checked", () => {
  it("heads each group with the marker's unchecked label, never the finding name", () => {
    renderReport(unconfirmedDoc);
    expect(screen.getByRole("heading", { name: "Design doc not checked against the code (1)" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Design doc is behind the code/ })).not.toBeInTheDocument();
  });

  it("gives the Uncovered section its own meaning sentence", () => {
    renderReport(unconfirmedDoc);
    const section = screen.getByRole("heading", { name: "Uncovered (2)" }).closest("section") as HTMLElement;
    expect(within(section).getByText("These could not be checked, so this report says nothing about whether they drifted.")).toBeInTheDocument();
  });

  it("states the cause once: a row whose detail only restates it shows no detail, a specific detail stays", () => {
    renderReport(unconfirmedDoc);
    expect(screen.getByText("No human has confirmed this design document against the code.")).toBeInTheDocument();
    expect(screen.queryByText(/no human confirmation of the described code is recorded/)).not.toBeInTheDocument();
    expect(screen.getByText(/permission denied/)).toBeInTheDocument();
  });

  it("leads a design row with the document's title and puts its path after it in mono", () => {
    renderReport(unconfirmedDoc);
    const link = screen.getByRole("link", { name: "Routing design" });
    expect(link).toHaveAttribute("href", "/file/.tiller/design/routing.md#L1");
    const path = screen.getByText("design/routing.md");
    expect(path).toHaveClass("mono");
    expect(path.closest("a")).toBeNull();
  });

  it("keeps the path alone as the link when the document has no title", () => {
    renderReport({ compared: 0, uncovered: [{ marker: "stale_design", noun: "design", id: "", key: "pending.md", reason: "unconfirmed" }] });
    expect(screen.getByRole("link", { name: "design/pending.md" })).toBeInTheDocument();
  });

  it("gives every marker an unchecked label", () => {
    for (const copy of Object.values(MARKER_COPY)) {
      expect(copy.unchecked.trim()).not.toBe("");
      expect(copy.unchecked).toMatch(/not (checked|compared)/);
    }
  });
});

describe("Drift report: footer counts in plain words", () => {
  it("writes real plurals and explains a design doc that could not be compared", () => {
    renderReport(unconfirmedDoc);
    const footer = document.querySelector(".drift-footer") as HTMLElement;
    expect(footer.textContent).not.toContain("(s)");
    expect(screen.getByText(/^Compared 18 nodes and 0 design docs across 7 checks\./)).toBeInTheDocument();
    expect(screen.getByText(/1 design doc could not be fully compared \(see Uncovered\)\./)).toBeInTheDocument();
  });

  it("names what each coverage count means, never unresolvable or scope words", () => {
    renderReport(unconfirmedDoc);
    const footer = document.querySelector(".drift-footer") as HTMLElement;
    for (const word of ["unresolvable", "out of scope", "not yet in scope"]) {
      expect(footer.textContent).not.toContain(word);
    }
    expect(screen.getByText("Design doc is behind the code: 1 design doc could not be compared")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Eval points at something missing: 1 criterion checked, 3 have no file to check, 10 are checked only once the work reaches a later stage",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Work started from an older basis: 1 node checked")).toBeInTheDocument();
  });

  it("says one node and one check in the singular", () => {
    renderReport({ compared: 1, compared_design: 1, disabled: ["stale_spec", "active_basis", "answered_question", "missing_surface", "orphan_eval", "shifted_design"] });
    expect(screen.getByText("Compared 1 node and 1 design doc across 1 check; nothing moved.")).toBeInTheDocument();
  });
});

describe("Drift report: sections can be linked to", () => {
  it("gives each marker section and the Uncovered section an id", () => {
    const { container } = renderReport({
      compared: 1,
      candidates: { missing_surface: [{ marker: "missing_surface", noun: "feature", id: "F-1", detail: "lib/a.txt no longer exists in the working tree", files: ["lib/a.txt"] }] },
      uncovered: unconfirmedDoc.uncovered,
    });
    expect(container.querySelector("section#missing_surface")).not.toBeNull();
    expect(container.querySelector("section#uncovered")).not.toBeNull();
  });
});

function featurePage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "feature", id: "F-kxijg1", key: "foundation/routing", title: "Routing", status: "active" },
    ref: "foundation/routing (F-kxijg1)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

describe("Node drift panel", () => {
  const page = featurePage({
    drift: [{ marker: "missing_surface", refusing: true }],
    drift_detail: {
      candidates: [
        {
          marker: "missing_surface",
          noun: "feature",
          id: "F-kxijg1",
          key: "foundation/routing",
          detail: "lib/router.txt no longer exists in the working tree",
          files: ["lib/router.txt"],
          remedy: "tiller link feature foundation/routing --surface <path>",
        },
      ],
      uncovered: [
        { marker: "stale_design", noun: "design", id: "", key: "routing.md", title: "Routing design", reason: "unconfirmed", detail: "no human confirmation of the described code is recorded" },
      ],
    },
  });

  it("prints a missing path once", () => {
    const { container } = render(
      <MemoryRouter>
        <NodeDetail page={page} />
      </MemoryRouter>,
    );
    const panel = container.querySelector(".instr-drift-panel") as HTMLElement;
    expect(panel.textContent?.split("lib/router.txt").length).toBe(2);
  });

  it("links to the finding's own section of the report, styled as a link", () => {
    render(
      <MemoryRouter>
        <NodeDetail page={page} />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link", { name: /drift report/ });
    expect(link).toHaveAttribute("href", "/?tab=drift#missing_surface");
    expect(link).not.toHaveClass("muted");
  });

  it("names an uncovered row by what could not be checked and states its cause once", () => {
    render(
      <MemoryRouter>
        <NodeDetail page={page} />
      </MemoryRouter>,
    );
    expect(screen.getByText(/Design doc not checked against the code/)).toBeInTheDocument();
    expect(screen.queryByText(/no human confirmation of the described code is recorded/)).not.toBeInTheDocument();
  });
});
