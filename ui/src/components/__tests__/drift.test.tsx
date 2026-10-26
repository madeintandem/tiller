// Vitest/RTL coverage for the /drift page: the
// console's read of the exact sweep `tiller drift` renders. Written before
// Drift.tsx exists, per the branch's TDD discipline (home.test.tsx's own
// stubFetch-keyed-by-URL convention; no MSW in this harness).
//
// The scenarios below are the brief's own required list: a section per
// marker with candidates, in the schema's fixed order rather than incidental
// object-key order; the KNOWN NUANCE that active_basis's files are basenames
// while every other marker's are repo-relative, neither ever a link; an
// Uncovered section; the footer (compared, disabled, exempted, coverage with
// units); and an empty state that states what was compared, mirroring the
// CLI's own coverage assertion rather than a bare "no drift".

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DriftReportView } from "../../api";
import Drift from "../../views/Drift";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// Matches home.test.tsx's own stubFetch: an unmapped path 404s the way
// fetchJSON expects, so a missing fixture fails loudly instead of silently
// resolving to undefined.
function stubFetch(routes: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    }
    return Promise.resolve({
      ok: false,
      status: 404,
      statusText: "Not Found",
      text: () => Promise.resolve(""),
    });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function renderDrift() {
  return render(
    <MemoryRouter>
      <Drift />
    </MemoryRouter>,
  );
}

// A report exercising every row shape the brief names: three markers with
// candidates (deliberately keyed out of schema order below, to prove real
// reordering rather than incidental object-key pass-through), one uncovered
// row, and a footer with disabled/exempted/coverage all populated.
function fullReport(): DriftReportView {
  return {
    compared: 9,
    candidates: {
      // Inserted missing_surface before active_basis; the schema order
      // (stale_spec, active_basis, answered_question, missing_surface,
      // orphan_eval) renders active_basis's section first regardless.
      missing_surface: [
        {
          marker: "missing_surface",
          noun: "feature",
          id: "F-msurf",
          key: "form-engine/schema-engine",
          detail: "app/models/legacy_calc.rb no longer exists in the working tree",
          files: ["app/models/legacy_calc.rb"],
        },
      ],
      active_basis: [
        {
          marker: "active_basis",
          noun: "feature",
          id: "F-active",
          key: "form-engine/audit-export",
          detail: "feature.md changed since the accepted basis",
          files: ["feature.md"],
          since: { kind: "git_commit", value: "git:e5f9d28a1b2c3d4e5f6" },
        },
      ],
      orphan_eval: [
        {
          marker: "orphan_eval",
          noun: "feature",
          id: "F-orphan",
          key: "form-engine/audit-export",
          detail: "criterion C6's ref no longer resolves",
          context: ["3 criteria checked", "1 unresolvable"],
        },
      ],
    },
    uncovered: [
      {
        marker: "answered_question",
        noun: "feature",
        id: "F-unknownq",
        key: "form-engine/audit-export",
        reason: "unknown_question",
        detail: "Q-999999 no longer exists in the ledger",
      },
    ],
    disabled: ["stale_spec"],
    exempted: { active_basis: 2 },
    coverage: {
      active_basis: { checked: 4, unresolvable: 1 },
      orphan_eval: { checked: 3, unresolvable: 1, out_of_scope: 2 },
    },
  };
}

describe("Drift page: marker sections", () => {
  it("orders sections by the schema's fixed marker order, not object-key order", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    const activeBasisHead = await screen.findByText(/active_basis \(1\)/);
    const missingSurfaceHead = await screen.findByText(/missing_surface \(1\)/);
    const orphanEvalHead = await screen.findByText(/orphan_eval \(1\)/);

    // Schema order is active_basis, then missing_surface, then orphan_eval;
    // the fixture inserted missing_surface first, so a pass here proves the
    // page re-orders rather than passing incidental object-key order through.
    expect(
      Boolean(activeBasisHead.compareDocumentPosition(missingSurfaceHead) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);
    expect(
      Boolean(missingSurfaceHead.compareDocumentPosition(orphanEvalHead) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);
  });

  it("renders a candidate row addressing its node by id, with detail, files, since, and context", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    const link = await screen.findByRole("link", { name: "form-engine/audit-export (F-orphan)" });
    expect(link).toHaveAttribute("href", "/node/feature/F-orphan");
    expect(screen.getByText("criterion C6's ref no longer resolves")).toBeInTheDocument();
    expect(screen.getByText("3 criteria checked")).toBeInTheDocument();
    expect(screen.getByText("1 unresolvable")).toBeInTheDocument();
  });

  it("renders since as a short sha", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    await screen.findByText(/active_basis \(1\)/);
    // "git:e5f9d28a1b2c3d4e5f6" shortens to the first 7 hex characters,
    // mirroring internal/cli/drift.go's own shortSHA exactly.
    expect(screen.getByText("git:e5f9d28")).toBeInTheDocument();
    expect(screen.queryByText("git:e5f9d28a1b2c3d4e5f6")).not.toBeInTheDocument();
  });
});

describe("Drift page: the KNOWN NUANCE, files render as text, never a link", () => {
  it("renders active_basis's basename and another marker's repo-relative path both as plain text", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    const basename = await screen.findByText("feature.md");
    const repoRelative = await screen.findByText("app/models/legacy_calc.rb");

    // Neither shape is a link: a basename does not say which directory it
    // lives in, and guessing a root for the repo-relative form would be
    // worse than not linking at all.
    expect(basename.closest("a")).toBeNull();
    expect(repoRelative.closest("a")).toBeNull();
  });
});

describe("Drift page: Uncovered section", () => {
  it("renders a row with its marker, reference, reason, and detail", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    await screen.findByText(/Uncovered \(1\)/);
    const link = screen.getByRole("link", { name: "form-engine/audit-export (F-unknownq)" });
    expect(link).toHaveAttribute("href", "/node/feature/F-unknownq");
    expect(screen.getByText(/unknown_question/)).toBeInTheDocument();
    expect(screen.getByText(/Q-999999 no longer exists in the ledger/)).toBeInTheDocument();
    // The marker rides beside the row: Uncovered is one flat list across
    // every marker, so a row cannot rely on section context to say which
    // marker it came from the way a candidate row can.
    expect(screen.getByText(/answered_question/)).toBeInTheDocument();
  });
});

describe("Drift page: footer", () => {
  it("names compared, disabled, exempted, and coverage with units", async () => {
    stubFetch({ "/api/drift": fullReport() });
    renderDrift();

    expect(await screen.findByText("Compared 9 node(s) across 4 check(s).")).toBeInTheDocument();
    expect(screen.getByText("Disabled: stale_spec")).toBeInTheDocument();
    expect(screen.getByText("Exempted: active_basis: 2")).toBeInTheDocument();
    // orphan_eval counts CRITERIA, the finer grain its own existence checks
    // operate at; the other four count node(s), the same unit Compared
    // counts in (drift.Report's own Coverage doc comment).
    expect(screen.getByText(/active_basis: 4 node\(s\) checked, 1 unresolvable/)).toBeInTheDocument();
    expect(
      screen.getByText(/orphan_eval: 3 criteria checked, 1 unresolvable, 2 out of scope/),
    ).toBeInTheDocument();
  });
});

describe("Drift page: empty state", () => {
  it("states what was compared, mirroring the CLI's coverage assertion, rather than a bare empty note", async () => {
    const clean: DriftReportView = { compared: 7, disabled: ["orphan_eval"] };
    stubFetch({ "/api/drift": clean });
    renderDrift();

    expect(
      await screen.findByText("Compared 7 node(s) across 4 check(s); nothing moved."),
    ).toBeInTheDocument();
    // Disabled still rides the footer on a clean run: it is independent of
    // whether the sweep found anything.
    expect(screen.getByText("Disabled: orphan_eval")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 2 })).not.toBeInTheDocument();
  });
});

describe("Drift page: load failure", () => {
  it("shows the fetch error rather than a blank report", async () => {
    stubFetch({});
    renderDrift();

    expect(await screen.findByText(/GET \/api\/drift failed/)).toBeInTheDocument();
  });
});

describe("Drift page: baseline documents", () => {
  it("keeps design paths, feature references, bounded history and document coverage distinct from nodes", async () => {
    const key = "as-is/#api%2F notes.md";
    const report: DriftReportView = {
      compared: 2,
      compared_design: 1,
      candidates: {
        shifted_design: [{
          marker: "shifted_design", noun: "design", id: "", key,
          detail: "Working-tree bytes changed",
        }],
        stale_design: [{
          marker: "stale_design", noun: "design", id: "", key,
          since: { kind: "git_commit", value: "git:1234567890" },
          until: { kind: "git_commit", value: "git:abcdef0123" },
          features: [{
            id: "F-landed", key: "api/refresh",
            spec: ".tiller/epics/api/features/refresh/feature.md",
            commit: { kind: "git_commit", value: "git:abcdef0123" },
          }],
          remedy: "tiller approve design 'as-is/#api%2F notes.md'",
        }],
      },
      uncovered: [{ marker: "stale_design", noun: "design", id: "", key: "pending.md", reason: "unconfirmed" }],
      disabled: ["orphan_eval"],
      coverage: { stale_design: { checked: 1, unresolvable: 1 } },
    };
    stubFetch({ "/api/drift": report });
    const { container } = renderDrift();
    const links = await screen.findAllByRole("link", { name: `design/${key}` });
    for (const link of links) {
      expect(link).toHaveAttribute("href", "/file/.tiller/design/as-is/%23api%252F%20notes.md#L1");
    }
    expect(screen.getByRole("link", { name: "design/pending.md" })).toHaveAttribute("href", "/file/.tiller/design/pending.md#L1");
    expect(container.querySelector('a[href^="/node/design"]')).toBeNull();
    expect(screen.getByRole("link", { name: "api/refresh (F-landed)" })).toHaveAttribute("href", "/node/feature/F-landed");
    expect(screen.getByRole("link", { name: "feature spec" })).toHaveAttribute("href", "/file/.tiller/epics/api/features/refresh/feature.md#L1");
    expect(screen.getByText("git:1234567")).toBeInTheDocument();
    expect(screen.getAllByText("git:abcdef0")).toHaveLength(2);
    expect(screen.getByRole("button", { name: report.candidates?.stale_design[0].remedy })).toBeInTheDocument();
    expect(screen.getByText(/Compared 2 node\(s\) and 1 design doc\(s\)/)).toBeInTheDocument();
    expect(screen.getByText(/stale_design: 1 doc\(s\) checked, 1 unresolvable/)).toBeInTheDocument();
    expect(screen.queryByText(/nothing moved/)).not.toBeInTheDocument();
  });
});
