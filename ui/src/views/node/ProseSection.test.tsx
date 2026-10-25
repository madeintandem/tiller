// ProseSection's three states. The one that motivated this pass: a feature
// whose feature.md is still what `tiller add feature` generated used to render
// ~280px of boilerplate over three empty headings, which read as content. It
// now collapses to one line that says the spec is unwritten and names the move.
//
// The stub verdict is the SERVER's (ProseRef.stub, mutate.IsProseUnwritten),
// so these tests set the flag rather than matching boilerplate text: matching
// here would be a second copy of a sentinel that already has an owner, and the
// point is that there is exactly one. What this file pins on this side of the
// wire is that the flag, and only the flag, decides whether the prose is
// drawn, since drawing none of a file is the most destructive thing this
// component does.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { NodeView, ProseRef } from "../../api";
import ProseSection from "./ProseSection";

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: () => Promise.resolve() },
    configurable: true,
  });
});

// Verbatim renderFeatureMD output (internal/core/mutate/add.go), so what these
// tests fold is what the tool actually writes.
const FEATURE_STUB = [
  "# Selection Policy and Counters",
  "",
  "Draft feature created by `tiller add feature`. Write the spec here, then",
  "add `feature.eval.md` and run `tiller spec feature`.",
  "",
  "## Summary",
  "",
  "## Behavior",
  "",
  "## Notes",
  "",
].join("\n");

const featureNode: NodeView = {
  noun: "feature",
  id: "F-ni0u0s",
  key: "audit-engine/selection-policy",
  title: "Selection Policy and Counters",
  status: "draft",
};

const sourcedFeatureNode: NodeView = {
  ...featureNode,
  source: "design/02-domain-model.md",
};

const epicNode: NodeView = {
  noun: "epic",
  id: "E-qoipmj",
  key: "audit-engine",
  title: "Audit Engine",
  status: "draft",
};

const taskNode: NodeView = {
  noun: "task",
  id: "T-h4k2wq",
  key: "audit-engine/selection-policy/count-the-rows",
  title: "Count the rows",
  status: "proposed",
};

function prose(overrides: Partial<ProseRef> = {}): ProseRef {
  return {
    path: ".tiller/epics/audit-engine/features/selection-policy/feature.md",
    content: FEATURE_STUB,
    ...overrides,
  };
}

function renderSection(p: ProseRef | null, node: NodeView, label = "Spec") {
  return render(
    <MemoryRouter>
      <ProseSection prose={p} label={label} node={node} />
    </MemoryRouter>,
  );
}

it.each([true, false])("keeps an unavailable source copyable in stub=%s prose", (stub) => {
  const source = ".tiller/board.md#L1";
  render(<MemoryRouter><ProseSection
    prose={prose({ stub })}
    label="Spec"
    node={{ ...featureNode, source }}
    diagnostics={[{ code: "unresolved_citation", facet: "provenance", message: "Unavailable source", source: { file: "feature.yml", raw: source, citation_path: ".tiller/board.md", citation_line: 1 } }]}
  /></MemoryRouter>);
  expect(screen.queryByRole("link", { name: source })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: source })).toBeInTheDocument();
});

it("keeps local sources copyable when corpus validation cannot grant destinations", () => {
  const source = ".tiller/board.md#L1";
  render(<MemoryRouter><ProseSection
    prose={prose({ stub: true })}
    label="Spec"
    node={{ ...featureNode, source }}
    diagnostics={[{ code: "corpus_unreadable", facet: "documents", message: "Unsafe authored Markdown", incomplete: true }]}
  /></MemoryRouter>);
  expect(screen.queryByRole("link", { name: source })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: source })).toBeInTheDocument();
});

describe("ProseSection: a stub is not content", () => {
  it("collapses a feature stub to one line naming the move and its command", () => {
    const { container } = renderSection(prose({ stub: true }), featureNode);

    expect(screen.getByText("No spec written yet")).toBeInTheDocument();
    expect(screen.getByText("the agent's move")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /tiller spec feature audit-engine\/selection-policy/ }),
    ).toBeInTheDocument();
    // The path is still the drill pointer, so the real file is one copy away.
    expect(screen.getByText(prose().path)).toBeInTheDocument();

    // None of the boilerplate survives: no heading, no generated paragraph,
    // no empty section headings.
    expect(container.querySelector(".prose")).toBeNull();
    expect(screen.queryByText(/Draft feature created by/)).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Summary" })).not.toBeInTheDocument();
    expect(container.textContent).not.toContain("Behavior");
    // One line: the whole thing is a single flex row.
    expect(container.querySelectorAll(".instr-unwritten")).toHaveLength(1);
  });

  it("keeps the recorded source on the compact unwritten line", () => {
    const { container } = renderSection(prose({ stub: true }), sourcedFeatureNode);

    expect(
      screen.getByRole("link", { name: "design/02-domain-model.md" }),
    ).toHaveAttribute("href", "/file/design/02-domain-model.md#L1");
    expect(container.querySelector(".instr-section")).toBeNull();
  });

  it("names what to write for an epic stub and fabricates no command", () => {
    // `spec` is a feature verb, and an epic has none: epic.md is written in
    // place and nothing asserts it, so there is no command to copy and the line
    // says so rather than inventing one.
    renderSection(prose({ stub: true, path: ".tiller/epics/audit-engine/epic.md" }), epicNode);

    expect(screen.getByText("No spec written yet")).toBeInTheDocument();
    expect(screen.getByText("write the intent, scope, and definition of done")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("names what to write for a task stub and fabricates no command", () => {
    // A task's task.md is written in place too: there is no `tiller spec task`,
    // and the gate that reads it is the parent feature's, so naming that
    // feature's command here would hand the reader a verb addressed at some
    // other node.
    renderSection(
      prose({
        stub: true,
        path: ".tiller/epics/audit-engine/features/selection-policy/tasks/count-the-rows/task.md",
      }),
      taskNode,
      "Implementation spec",
    );

    expect(screen.getByText("No implementation spec written yet")).toBeInTheDocument();
    expect(
      screen.getByText("write the approach, surfaces, tests, and edge cases"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("addresses the command by id when the noun carries no key", () => {
    renderSection(prose({ stub: true }), { ...featureNode, key: undefined });
    expect(
      screen.getByRole("button", { name: /tiller spec feature F-ni0u0s/ }),
    ).toBeInTheDocument();
  });

  // The regression this state is one bad predicate away from causing. Writing
  // the spec into the stub's own headings and leaving the generated paragraph
  // alone is the ordinary way this file gets filled in; a run where the server
  // called that a stub drew "No spec written yet" over 972 bytes of real spec,
  // with no fold, no expand, and no way to reach a word of it. The server's
  // rule is now strict enough that the flag is false here, and this pins what
  // the component must do when it is.
  it("draws a spec written into the stub in full, generated paragraph and all", () => {
    const written =
      FEATURE_STUB +
      "\nEvery row carries a program_id. We do NOT use schema-per-tenant.\n";
    const { container } = renderSection(prose({ content: written, stub: false }), featureNode);

    expect(screen.queryByText("No spec written yet")).not.toBeInTheDocument();
    expect(container.querySelector(".prose")).not.toBeNull();
    expect(container.textContent).toContain("schema-per-tenant");
  });

  // An absent flag is the same answer as a false one: only a server that has
  // read the bytes and decided can collapse the section.
  it("draws the prose when the server says nothing about the stub at all", () => {
    const { container } = renderSection(prose({ content: "# Spec\n\nWritten.\n" }), featureNode);

    expect(screen.queryByText("No spec written yet")).not.toBeInTheDocument();
    expect(container.textContent).toContain("Written.");
  });
});

describe("ProseSection: a missing file is its own fact", () => {
  it("keeps the recorded source on the compact missing-file line", () => {
    const { container } = renderSection(prose({ content: "" }), sourcedFeatureNode);

    expect(screen.getByText(/Spec file missing or unreadable/)).toBeInTheDocument();
    expect(screen.getByText(prose().path)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "design/02-domain-model.md" }),
    ).toHaveAttribute("href", "/file/design/02-domain-model.md#L1");
    expect(container.querySelector(".instr-section")).toBeNull();
    // Distinct from the unwritten line: a file that is gone is not a file
    // nobody has written.
    expect(screen.queryByText("No spec written yet")).not.toBeInTheDocument();
  });

  it("calls an emptied file unwritten, not missing, because the server read it", () => {
    // The server only sets stub on a file it opened, so empty-with-the-flag is
    // "there and blank" and empty-without-it is "could not read". The two used
    // to collapse into one wrong sentence.
    renderSection(prose({ content: "", stub: true }), featureNode);

    expect(screen.getByText("No spec written yet")).toBeInTheDocument();
    expect(screen.queryByText(/missing or unreadable/)).not.toBeInTheDocument();
  });

  it("renders nothing at all for a noun that carries no prose file", () => {
    const { container } = renderSection(null, featureNode);
    expect(container.textContent).toBe("");
  });
});

describe("ProseSection: written prose", () => {
  const short = "# Selection Policy\n\nThe counters reconcile nightly.\n";

  it("renders a short spec whole, with its path and no expand", () => {
    renderSection(prose({ content: short }), featureNode);

    expect(screen.getByText("The counters reconcile nightly.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Read the full/ })).not.toBeInTheDocument();
  });

  it("does not repeat the node's own title as the spec's first heading", () => {
    renderSection(
      prose({ content: "# Selection Policy and Counters\n\nThe counters reconcile nightly.\n" }),
      featureNode,
    );

    // The page's own serif h1 says this six lines above; Prose demoted it to
    // an h2 and said it again in a second voice.
    expect(
      screen.queryByRole("heading", { name: "Selection Policy and Counters" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("The counters reconcile nightly.")).toBeInTheDocument();
  });

  // Task 17 (console v2, node page rebuild) killed the fold outright: "Prose
  // renders whole: no read-more folds and no truncated bodies, because the
  // tabs already bound the page and a spec you must unfold is a spec that
  // goes unread." A long spec used to clamp to its opening behind a click;
  // it now renders start to finish with no expand control at all, the
  // reading measure carried by CSS (72ch, prose.css) rather than a JS fold.
  it("renders a long spec whole, start to finish, with no expand control", () => {
    const long = [
      "# Audit Engine",
      "Run Acme quality assurance continuously from phase submission.",
      "## Scope",
      "S".repeat(800),
      "## Definition of done",
      "The last paragraph nobody should have to scroll past.",
    ].join("\n\n");
    renderSection(prose({ content: long }), epicNode);

    expect(
      screen.getByText("Run Acme quality assurance continuously from phase submission."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("The last paragraph nobody should have to scroll past."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Read the full/ })).not.toBeInTheDocument();
  });
});
