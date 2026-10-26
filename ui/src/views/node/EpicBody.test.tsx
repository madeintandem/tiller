// EpicBody's decomposition list: when the page carries a decomposition the
// Features section renders a mixed-child StateBar over the children plus ONE
// LINE per child feature in payload (canonical key) order, each carrying its
// title as the link, its key as muted mono, and its status as an explicit Chip
// (a mixed-status list is exactly the case a distant caption cannot
// disambiguate row by row). Without a decomposition (an older server) the
// section falls back to the relationships.features link list.
//
// The row used to stack the key on its own line, a `change_type · source`
// line, and one line per surface path and per module: four to seven lines per
// feature, so eight features became up to fifty lines. Those facts now read on
// the feature's own page, which is where the console's hierarchy inversion
// puts detail. These tests pin both halves: the row is one line, and the facts
// it stopped carrying are asserted reachable in instrument.test.tsx's
// header-facts coverage (the Touches and Change facts).
//
// The epic's own identity is NOT repeated here (the accepted quirk C3
// resolves): NodeDetail's header renders it once, so this body renders only
// what it uniquely owns.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { NodePage, NodeView, RelatedNode } from "../../api";
import { EpicFeaturesPanel } from "./EpicBody";

afterEach(() => {
  cleanup();
});

function feature(overrides: Partial<NodeView>): NodeView {
  return { noun: "feature", id: "feature-x", status: "draft", ...overrides };
}

// The two features arrive reverse-alphabetical by key, so a row order that
// matches the payload proves the body renders payload order and never re-sorts.
const refunds = feature({
  id: "feature-77",
  key: "billing/refunds",
  title: "Refund pipeline",
  status: "in_review",
  change_type: "modify",
  source: ".tiller/epics/billing/features/refunds/feature.md",
  surface: { paths: ["app/refunds.rb"] },
});
const invoices = feature({
  id: "feature-4",
  key: "billing/invoices",
  title: "Invoice builder",
  status: "draft",
  change_type: "create",
  source: ".tiller/epics/billing/features/invoices/feature.md",
  surface: { paths: ["app/invoices.rb"] },
});

function epicPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "epic",
      id: "epic-1",
      key: "billing",
      title: "Billing overhaul",
      status: "in_review",
      source: ".tiller/epics/billing/epic.md",
    },
    ref: "billing (epic-1)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

// the tab shell decomposes EpicBody into per-tab panels; this
// suite has only ever exercised the Features section, so it moves straight
// to EpicFeaturesPanel with no assertion changes (import/wrapper only, per
// the brief's adaptation rule).
function renderBody(page: NodePage) {
  return render(
    <MemoryRouter>
      <EpicFeaturesPanel page={page} />
    </MemoryRouter>,
  );
}

function precedes(a: Node, b: Node): boolean {
  return Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);
}

describe("EpicBody decomposition list", () => {
  it("renders one line per feature, title-first, in payload order", () => {
    renderBody(epicPage({ decomposition: { features: [refunds, invoices] } }));

    const refundsRow = screen.getByText("Refund pipeline").closest(".instr-decomp-row");
    const invoicesRow = screen.getByText("Invoice builder").closest(".instr-decomp-row");
    expect(refundsRow).not.toBeNull();
    expect(invoicesRow).not.toBeNull();
    expect(precedes(refundsRow as Node, invoicesRow as Node)).toBe(true);

    // Title-first (the locked carve-out every other console row keeps,
    // ui-surface.md: "a row leads with the node's title, its key ... follows
    // as muted mono"): the title is the link, the key is plain muted mono
    // secondary text, never a link of its own.
    const first = within(refundsRow as HTMLElement);
    expect(first.getByRole("link", { name: "Refund pipeline" })).toHaveAttribute(
      "href",
      "/node/feature/feature-77",
    );
    expect(first.getByText("billing/refunds")).toBeInTheDocument();
    expect(first.queryByRole("link", { name: "billing/refunds" })).not.toBeInTheDocument();
    expect(first.getByText("in_review")).toBeInTheDocument();
  });

  it("carries no provenance or surface sub-lines: one line per node, nothing stacked", () => {
    renderBody(epicPage({ decomposition: { features: [refunds, invoices] } }));

    const row = screen.getByText("Refund pipeline").closest(".instr-decomp-row") as HTMLElement;
    const scoped = within(row);
    expect(
      scoped.queryByText("modify · .tiller/epics/billing/features/refunds/feature.md"),
    ).not.toBeInTheDocument();
    expect(scoped.queryByText("app/refunds.rb")).not.toBeInTheDocument();
    expect(scoped.queryByText("Billing::Refunds")).not.toBeInTheDocument();
    // Three children exactly: the title link, the key, the chip.
    expect(row.children).toHaveLength(3);
  });

  it("renders a mixed-child StateBar with the composed caption in the head, and no duplicate epic identity line", () => {
    const { container } = renderBody(epicPage({ decomposition: { features: [refunds, invoices] } }));

    expect(container.querySelector(".instr-decomp-bar .state-bar")).not.toBeNull();
    expect(screen.getByText("Features · 2")).toBeInTheDocument();
    expect(screen.getByText("1 draft · 1 in_review")).toBeInTheDocument();
    // The epic's own title/key/id never render a second time here: that is
    // NodeDetail's header's job, exactly once, whichever way this body is
    // reached.
    expect(screen.queryByText("Billing overhaul")).not.toBeInTheDocument();
    expect(screen.queryByText("epic-1")).not.toBeInTheDocument();
  });

  it("says an epic is uncarved in one line, with no section chrome around it", () => {
    const { container } = renderBody(epicPage({ decomposition: { features: [] } }));

    expect(screen.getByText("No features carved yet")).toBeInTheDocument();
    expect(container.querySelector(".instr-section")).toBeNull();
    expect(container.querySelector(".state-bar")).toBeNull();
  });
});

describe("EpicBody decomposition cap", () => {
  // 15 drafts plus one specced child sitting past the cap: the cap must fold
  // the drafts and must NOT fold the one child that is mid-lifecycle, which is
  // exactly what a human scanning an epic came for.
  const many: NodeView[] = [
    ...Array.from({ length: 15 }, (_, i) =>
      feature({ id: `f-${i}`, key: `billing/f${i}`, title: `Feature ${i}`, status: "draft" }),
    ),
    feature({ id: "f-late", key: "billing/late", title: "Late but specced", status: "specced" }),
  ];

  it("caps the drafts, states the true total, and never hides a feature past draft", () => {
    renderBody(epicPage({ decomposition: { features: many } }));

    expect(screen.getByText("Features · 16")).toBeInTheDocument();
    expect(screen.getByText("Feature 11")).toBeInTheDocument();
    expect(screen.queryByText("Feature 12")).not.toBeInTheDocument();
    expect(screen.getByText("Late but specced")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "3 more features ▸" })).toBeInTheDocument();
  });

  it("expands the hidden rows in place, with no refetch", () => {
    renderBody(epicPage({ decomposition: { features: many } }));

    fireEvent.click(screen.getByRole("button", { name: "3 more features ▸" }));

    expect(screen.getByText("Feature 12")).toBeInTheDocument();
    expect(screen.getByText("Feature 14")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /more features/ })).not.toBeInTheDocument();
    // Expanded, the rows are the payload again, in payload order and complete.
    expect(document.querySelectorAll(".instr-decomp-row")).toHaveLength(16);
  });

  it("computes the state bar's mix from every feature, capped rows or not", () => {
    renderBody(epicPage({ decomposition: { features: many } }));
    expect(screen.getByText("15 draft · 1 specced")).toBeInTheDocument();
  });
});

describe("EpicBody decomposition fallback", () => {
  it("renders the features link list and no decomposition rows when decomposition is absent", () => {
    const feat: RelatedNode = {
      noun: "feature",
      id: "feature-4",
      key: "billing/invoices",
      ref: "billing/invoices (feature-4)",
      status: "draft",
      title: "Invoice builder",
    };
    renderBody(epicPage({ relationships: { features: [feat] } }));

    expect(document.querySelector(".instr-decomp")).toBeNull();
    expect(screen.getByRole("link", { name: "Invoice builder" })).toHaveAttribute(
      "href",
      "/node/feature/feature-4",
    );
  });

  it("says an epic with no features at all is uncarved, in one line", () => {
    renderBody(epicPage({ relationships: {} }));
    expect(screen.getByText("No features carved yet")).toBeInTheDocument();
  });
});
