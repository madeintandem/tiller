import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BacklinkSource, BacklinksView, RelationshipVertex } from "../../api";
import BacklinksPanel from "./BacklinksPanel";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(backlinks: BacklinksView) {
  return render(<MemoryRouter><BacklinksPanel backlinks={backlinks} /></MemoryRouter>);
}

const document: RelationshipVertex = {
  id: "doc:.tiller/design/refund notes.md", kind: "document", path: ".tiller/design/refund notes.md",
  destination: { kind: "file", path: ".tiller/design/refund notes.md" },
};
const citations: BacklinkSource = {
  vertex: document,
  owner_vertex: {
    id: "F-other", kind: "feature", title: "Billing design",
    destination: { kind: "node", noun: "feature", id: "F-other" },
  },
  edges: [{
    id: "mention", kind: "mentions", from: document.id, to: "F-current",
    sources: [{ file: document.path!, line: 8 }, { file: document.path!, line: 23 }],
  }],
};

describe("BacklinksPanel", () => {
  it("keeps historical source ownership addressed to the cancelled execution unit after transfer", () => {
    mount({ count: 1, complete: true, diagnostics: [], groups: [{ kind: "mentioned_by", sources: [{
      vertex: document,
      owner_vertex: { kind: "bug", id: "B-original", title: "Original captured bug", status: "cancelled", owner: "pat",
        destination: { kind: "node", noun: "bug", id: "B-original" } },
      edges: [{ id: "historical-citation", kind: "mentions", from: document.id, to: "F-target",
        sources: [{ file: document.path!, line: 12 }] }],
    }] }] });
    expect(screen.getByRole("link", { name: "Original captured bug" })).toHaveAttribute("href", "/node/bug/B-original");
    expect(screen.getByRole("link", { name: `${document.path}#L12` })).toHaveAttribute("href", "/file/.tiller/design/refund%20notes.md#L12");
  });

  it("uses the relationship key before raw coordinates without changing the granted destination", () => {
    mount({ count: 1, complete: true, diagnostics: [], groups: [{
      kind: "mentioned_by", sources: [{
        vertex: { ...document, key: "refund-design", raw: "historical/refunds.md" },
        edges: [],
      }],
    }] });
    expect(screen.getByRole("link", { name: "refund-design" })).toHaveAttribute("href", "/file/.tiller/design/refund%20notes.md#L1");
  });

  it("keeps invalid line coordinates copyable instead of silently opening line one", () => {
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    mount({ count: 0, complete: false, groups: [], diagnostics: [{
      code: "invalid", facet: "documents", message: "Invalid coordinates",
      source: { file: "notes.md", line: 0, citation_path: "source.md", citation_line: 0, operation: 0 },
    }] });
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "notes.md#L0" }));
    expect(writeText).toHaveBeenLastCalledWith("notes.md#L0");
    fireEvent.click(screen.getByRole("button", { name: "source.md#L0" }));
    expect(writeText).toHaveBeenLastCalledWith("source.md#L0");
    expect(screen.getByText(/operation 0/)).toBeInTheDocument();
  });

  it("keeps retired document diagnostics copyable without advertising a file destination", () => {
    mount({ count: 0, complete: true, groups: [], diagnostics: [
      { code: "unresolved_document", facet: "documents", message: "Recorded document is missing",
        source: { file: ".tiller/board.md" } },
      { code: "unresolved_citation", facet: "provenance", message: "Citation is unavailable",
        source: { file: ".tiller/design/source.md", line: 3, citation_path: ".tiller/board.md", citation_line: 1, raw: ".tiller/board.md#L1" } },
    ] });
    expect(screen.queryByRole("link", { name: /\.tiller\/board\.md/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: ".tiller/board.md" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: ".tiller/board.md#L1" })).toBeInTheDocument();
    expect(screen.getByText("Recorded: .tiller/board.md#L1")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: ".tiller/design/source.md#L3" })).toHaveAttribute("href", "/file/.tiller/design/source.md#L3");
  });

  it("keeps exact repeated citation lines under one source and separates generated mentions", () => {
    mount({
      count: 2, complete: true, diagnostics: [], groups: [{ kind: "mentioned_by", sources: [
        citations,
        {
          vertex: { id: "doc:.tiller/orientation.md", kind: "document", path: ".tiller/orientation.md", generated: true, destination: { kind: "file", path: ".tiller/orientation.md" } },
          edges: [{ id: "generated", kind: "mentions", from: "doc:.tiller/orientation.md", to: "F-current", sources: [{ file: ".tiller/orientation.md", line: 4 }] }],
        },
      ] }],
    });
    const authored = screen.getByRole("region", { name: "Mentioned by" });
    expect(within(authored).getByRole("link", { name: "Billing design" })).toHaveAttribute("href", "/node/feature/F-other");
    expect(within(authored).getByRole("link", { name: `${document.path}#L8` })).toHaveAttribute("href", "/file/.tiller/design/refund%20notes.md#L8");
    expect(within(authored).getByRole("link", { name: `${document.path}#L23` })).toHaveAttribute("href", "/file/.tiller/design/refund%20notes.md#L23");
    const generated = screen.getByRole("region", { name: "Generated · Mentioned by" });
    expect(within(generated).getByRole("link", { name: ".tiller/orientation.md#L4" })).toHaveAttribute("href", "/file/.tiller/orientation.md#L4");
    expect(within(authored).queryByText(".tiller/orientation.md")).not.toBeInTheDocument();
  });

  it("never guesses destinations for unresolved work, documents without capabilities, or recorded paths", () => {
    const vertices: RelationshipVertex[] = [
      { id: "F-reused", kind: "feature", raw: "former/refunds", unresolved: true, destination: { kind: "node", noun: "feature", id: "F-current" } },
      { id: "doc:missing.md", kind: "document", path: ".tiller/missing.md" },
      { id: "path:src/refunds.ts", kind: "path", path: "src/refunds.ts" },
      { id: "external_path:reference/refunds.md", kind: "external_path", path: "reference/refunds.md" },
    ];
    mount({ count: 4, complete: true, diagnostics: [], groups: [{
      kind: "source_for", sources: vertices.map((vertex) => ({ vertex, edges: [] })),
    }] });
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText(/former\/refunds/)).toHaveTextContent("unresolved");
    expect(screen.getByRole("button", { name: "src/refunds.ts" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "reference/refunds.md" })).toBeInTheDocument();
  });

  it("distinguishes pinned evidence from current file links and keeps unsupported assertion files copyable", () => {
    mount({ count: 1, complete: true, diagnostics: [], groups: [{ kind: "derivation_of", sources: [{
      vertex: document,
      edges: [{ id: "derived", kind: "derived_from", from: document.id, to: "path:source.md", sources: [
        { file: document.path!, field: "derived_from", revision: { kind: "blob", value: "abc123", path: "source.md", observed_at: "2026-09-19T12:00:00Z" } },
        { file: ".tiller/state.yaml", field: "derived_from" },
      ] }],
    }] }] });
    expect(screen.getByText("abc123")).toBeInTheDocument();
    expect(screen.getByText("source.md")).toBeInTheDocument();
    expect(screen.getByText("2026-09-19T12:00:00Z")).toBeInTheDocument();
    for (const link of screen.getAllByRole("link", { name: document.path! })) {
      expect(link).toHaveAttribute("href", "/file/.tiller/design/refund%20notes.md#L1");
    }
    expect(screen.getByRole("button", { name: ".tiller/state.yaml" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: ".tiller/state.yaml" })).not.toBeInTheDocument();
  });

  it("opens the cited file's line separately from the YAML field that records the citation", () => {
    mount({ count: 1, complete: true, diagnostics: [], groups: [{ kind: "source_for", sources: [{
      vertex: document,
      edges: [{
        id: "citation", kind: "sourced_from", from: document.id, to: "doc:.tiller/design/source.md",
        sources: [{
          file: ".tiller/state.yaml", field: "source",
          citation_path: ".tiller/design/source.md", citation_line: 37,
        }],
      }],
    }] }] });
    expect(screen.getByRole("button", { name: ".tiller/state.yaml" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: ".tiller/design/source.md#L37" })).toHaveAttribute("href", "/file/.tiller/design/source.md#L37");
    expect(screen.queryByRole("link", { name: /\.tiller\/state\.yaml/ })).not.toBeInTheDocument();
    expect(screen.queryByText(".tiller/state.yaml#L37")).not.toBeInTheDocument();
  });
});
