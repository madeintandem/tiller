// The node page header after the node header pass (2026-08-30, JC's review of
// the Lantern screenshots: "more consistent, scannable, clear at a glance"). The
// same slots on every noun, in one fixed order: the crumb chain (no trailing
// separator), a kind line (the noun on a solid pill, the node's own state at
// the hero size, the gate ticks, the drift/held/grain chips), the title alone,
// the mono ref line, the children's mix on an epic or a feature, then ONE
// label-over-value facts block under ONE divider. Provenance (the source) left
// the header for the Spec section's own head, tested beside ProseSection below.

import type { ReactElement } from "react";
import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

afterEach(cleanup);
import type { NodePage, NodeView, RelatedNode } from "../../api";
import Chip from "../Chip";
import NodeDetail from "../NodeDetail";
import NodeTabs from "../NodeTabs";
import ProseSection from "../../views/node/ProseSection";

function mount(ui: ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

function pageOf(node: NodeView, overrides: Partial<NodePage> = {}): NodePage {
  return {
    node,
    ref: node.key === undefined ? node.id : `${node.key} (${node.id})`,
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

const task: NodeView = {
  noun: "task",
  id: "T-tj6hor",
  key: "habit-engine/swiftdata-model/package-foundation",
  title: "Create the TendCore package foundation",
  status: "done",
};

const feature: NodeView = {
  noun: "feature",
  id: "F-n7oy52",
  key: "habit-engine/swiftdata-model",
  title: "SwiftData Habit Model",
  status: "done",
};

function related(noun: string, id: string, status: string, title: string): RelatedNode {
  return { noun, id, ref: `${id}`, status, title };
}

function header(container: HTMLElement) {
  return within(container.querySelector(".instr-header") as HTMLElement);
}

function facts(container: HTMLElement) {
  return within(container.querySelector(".node-facts") as HTMLElement);
}

// A fact is one dt/dd pair; factValue returns the dd beside the named dt.
function factValue(container: HTMLElement, label: string): HTMLElement {
  const dt = facts(container).getByText(label);
  return dt.nextElementSibling as HTMLElement;
}

describe("Node header: the kind line", () => {
  it("names the noun on a solid kind pill above the title", () => {
    const { container } = mount(<NodeDetail page={pageOf(task)} />);
    const pill = header(container).getByText("Task");
    expect(pill).toHaveClass("kind-pill");
    const title = header(container).getByRole("heading", { name: task.title });
    expect(Boolean(pill.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
  });

  it("names a shelf noun the same way", () => {
    const bug: NodeView = { noun: "bug", id: "B-8lpm8p", key: "bugs/nav", title: "Nav ignores taps", status: "proposed" };
    const { container } = mount(<NodeDetail page={pageOf(bug)} />);
    expect(header(container).getByText("Bug")).toHaveClass("kind-pill");
  });

  it.each([
    ["chore", "CH-1", "Chore"],
    ["tweak", "TW-1", "Tweak"],
  ])("names a %s shelf node identically", (noun, id, label) => {
    const node: NodeView = { noun, id, title: "Rotate keys", status: "ready" };
    const { container } = mount(<NodeDetail page={pageOf(node)} />);
    expect(header(container).getByText(label)).toHaveClass("kind-pill");
    expect(container.querySelector(".kind-line .chip-hero")?.textContent).toBe("ready");
  });

  it("gives an amendment the same kind line and nothing it does not have: no facts block, no mix", () => {
    const amendment: NodeView = { noun: "amendment", id: "AM-1", status: "proposed" };
    const { container } = mount(<NodeDetail page={pageOf(amendment)} />);
    expect(header(container).getByText("Amendment")).toHaveClass("kind-pill");
    expect(container.querySelector(".node-facts")).toBeNull();
    expect(container.querySelector(".node-mix")).toBeNull();
  });

  it("draws no separator at all on a one-crumb chain", () => {
    const page = pageOf(feature, { relationships: { epic: related("epic", "E-1", "done", "Habit Engine") } });
    const { container } = mount(<NodeDetail page={page} />);
    expect(container.querySelectorAll(".crumbs .sep")).toHaveLength(0);
  });

  it("paints the node's own state as the hero chip on the kind line, never in the title row", () => {
    const { container } = mount(<NodeDetail page={pageOf({ ...task, status: "in_progress" })} />);
    const line = container.querySelector(".kind-line") as HTMLElement;
    const hero = line.querySelector(".chip.chip-hero") as HTMLElement;
    expect(hero).not.toBeNull();
    expect(hero.textContent).toBe("in_progress");
    expect(container.querySelector(".node-title .chip")).toBeNull();
  });

  it("pends the evals chip on a stale recorded gate even when the contract is all optional", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf(
          { ...feature, status: "active", gates: { evals: { required: true, status: "stale" } } },
          {
            eval: {
              path: "features/x/feature.eval.md",
              node: feature.id,
              criteria: [
                { id: "C1", statement: "s", polarity: "introduce", required: false, binding: { type: "manual" } },
              ],
            },
          },
        )}
      />,
    );
    const chip = container.querySelector(".kind-line .gate-chip") as HTMLElement;
    expect(chip.querySelector(".g.pend")).not.toBeNull();
  });

  it("passes the evals chip on a recorded pass even when a required criterion was added since", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf(
          {
            ...feature,
            status: "active",
            gates: { evals: { required: true, status: "pass", criteria: { C1: { status: "pass" } } } },
          },
          {
            eval: {
              path: "features/x/feature.eval.md",
              node: feature.id,
              criteria: [
                { id: "C1", statement: "s", polarity: "introduce", required: true, binding: { type: "manual" } },
                { id: "C2", statement: "t", polarity: "introduce", required: true, binding: { type: "manual" } },
              ],
            },
          },
        )}
      />,
    );
    const chip = container.querySelector(".kind-line .gate-chip") as HTMLElement;
    expect(chip.querySelector(".g.pass")).not.toBeNull();
    expect(chip).toHaveTextContent(/^\S*evals$/);
  });

  it("carries the recorded gate ticks on the kind line", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({
          ...task,
          gates: { build: { required: true, status: "pass" }, tests: { required: true, status: "pass" } },
        })}
      />,
    );
    expect(container.querySelectorAll(".kind-line .gate-chip")).toHaveLength(2);
  });

  it("ends the crumb chain on the last ancestor, no trailing separator", () => {
    const page = pageOf(task, {
      relationships: {
        epic: related("epic", "E-1", "done", "Habit Engine"),
        feature: related("feature", "F-1", "done", "SwiftData Habit Model"),
      },
    });
    const { container } = mount(<NodeDetail page={page} />);
    expect(container.querySelectorAll(".crumbs .sep")).toHaveLength(1);
    const chain = (container.querySelector(".crumbs") as HTMLElement).textContent ?? "";
    expect(chain.trim().endsWith("›")).toBe(false);
  });
});

describe("Node header: the facts block", () => {
  it("keeps empty ancestor contribution distinct from an unstarted task reservation", () => {
    const ancestor = mount(<NodeDetail page={pageOf({ ...feature, contributors: [] })} />);
    expect(factValue(ancestor.container, "Contributors")).toHaveTextContent("None yet");
    expect(facts(ancestor.container).queryByText("Owner")).not.toBeInTheDocument();
    ancestor.unmount();
    const execution = mount(<NodeDetail page={pageOf({ ...task, status: "ready", started: false, owner: "sam", owner_name: "Sam" })} />);
    expect(factValue(execution.container, "Owner")).toHaveTextContent("Sam");
    expect(facts(execution.container).queryByText("Contributors")).not.toBeInTheDocument();
  });

  it("labels every fact and keeps them in one block under one divider", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({
          ...feature,
          contributors: [{ id: "jc", name: "JC Grubbs" }],
          branch: "tiller/habit-engine/swiftdata-model",
          change_type: "create",
          surface: { paths: ["app/a.rb", "app/b.rb"] },
        })}
      />,
    );
    expect(container.querySelectorAll(".node-facts")).toHaveLength(1);
    expect(factValue(container, "Contributors").textContent).toBe("JC Grubbs");
    expect(facts(container).queryByText("Owner")).not.toBeInTheDocument();
    expect(factValue(container, "Branch").textContent).toBe("tiller/habit-engine/swiftdata-model");
    expect(factValue(container, "Change").textContent).toBe("create");
    expect(factValue(container, "Touches").textContent).toBe("app/a.rb, app/b.rb");
    // The old scattered lines are gone: no second caption row under the block.
    expect(container.querySelector(".instr-dmeta")).toBeNull();
  });

  it("links the PR to pr_url when serve resolved one, and reads a plain number otherwise", () => {
    const linked = mount(
      <NodeDetail page={pageOf({ ...task, pr: 1, pr_url: "https://github.com/x/y/pull/1" })} />,
    );
    const link = facts(linked.container).getByRole("link", { name: /^#1/ });
    expect(link).toHaveAttribute("href", "https://github.com/x/y/pull/1");
    linked.unmount();

    const plain = mount(<NodeDetail page={pageOf({ ...task, pr: 1 })} />);
    expect(factValue(plain.container, "PR").textContent).toBe("#1");
    expect(facts(plain.container).queryByRole("link", { name: /^#1/ })).toBeNull();
  });

  it("links the branch to branch_url when serve resolved one, plain mono otherwise", () => {
    const linked = mount(
      <NodeDetail
        page={pageOf({
          ...feature,
          branch: "tiller/a/b",
          branch_url: "https://github.com/x/y/tree/tiller/a/b",
        })}
      />,
    );
    expect(facts(linked.container).getByRole("link", { name: /^tiller\/a\/b/ })).toHaveAttribute(
      "href",
      "https://github.com/x/y/tree/tiller/a/b",
    );
    linked.unmount();

    const plain = mount(<NodeDetail page={pageOf({ ...feature, branch: "tiller/a/b" })} />);
    expect(facts(plain.container).queryByRole("link", { name: /^tiller\/a\/b/ })).toBeNull();
    expect(factValue(plain.container, "Branch").textContent).toBe("tiller/a/b");
  });

  it("reads the approval pin as short shas with the full token on hover, the commit linked, and no eval when there is none", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({
          ...task,
          accepted_against: {
            spec: "git:4bf7c96ac7346dba2c360308c4f37ff50dbdf227",
            commit: "git:7458e30",
            commit_url: "https://github.com/x/y/commit/7458e30",
          },
        })}
      />,
    );
    const value = factValue(container, "Approved against");
    const spec = within(value).getByText("4bf7c96");
    expect(spec).toHaveAttribute("title", "git:4bf7c96ac7346dba2c360308c4f37ff50dbdf227");
    expect(within(value).getByRole("link", { name: /^7458e30/ })).toHaveAttribute(
      "href",
      "https://github.com/x/y/commit/7458e30",
    );
    expect(value.textContent).not.toContain("eval");
    expect(value.textContent).not.toContain("git:");
  });

  it("prefers the completion record over the approval pin once a node is done, and reads the commit plain without a link", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({
          ...feature,
          accepted_against: { spec: "git:1111111aaaaaaa", eval: "git:2222222bbbbbbb", commit: "git:3333333" },
          verified_against: {
            spec: "git:bedf333ebd0e955a347dd9b82bb1d7fc4bd14278",
            eval: "git:35a6dbc6dd9767efa4c21a2735ca1ecec6f14f76",
            commit: "git:828a8b0",
            surfaces: [],
          },
        })}
      />,
    );
    expect(facts(container).queryByText("Approved against")).toBeNull();
    const value = factValue(container, "Verified against");
    expect(value.textContent).toContain("bedf333");
    expect(value.textContent).toContain("35a6dbc");
    expect(value.textContent).toContain("828a8b0");
    expect(within(value).queryByRole("link")).toBeNull();
  });

  it("states who proposed a node that nobody owns yet", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({ noun: "bug", id: "B-1", key: "bugs/nav", title: "Nav", status: "proposed", proposed_by: "jc" })}
      />,
    );
    expect(factValue(container, "Proposed by").textContent).toBe("jc");
  });

  it("states what a done node's crossing rested on", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({ noun: "bug", id: "B-1", key: "bugs/nav", title: "Nav", status: "done", owner: "jc", done_basis: "carried" })}
      />,
    );
    expect(factValue(container, "Done basis").textContent).toBe("carried");
  });

  it("tells assistive tech that an outbound link leaves the console", () => {
    const { container } = mount(
      <NodeDetail page={pageOf({ ...task, pr: 1, pr_url: "https://github.com/x/y/pull/1" })} />,
    );
    const link = facts(container).getByRole("link", { name: /^#1/ });
    expect(within(link).getByText("(opens in a new tab)")).toHaveClass("visually-hidden");
  });

  it("does not repeat the owner as the proposer", () => {
    const { container } = mount(
      <NodeDetail page={pageOf({ ...task, owner: "jc", proposed_by: "jc" })} />,
    );
    expect(facts(container).queryByText("Proposed by")).toBeNull();
  });

  it.each(["bug", "chore", "tweak"])("shows the attached node's own state beside a %s found-in link", (noun) => {
    const { container } = mount(
      <NodeDetail
        page={pageOf(
          { noun, id: "shelf-1", title: "Nav", status: "proposed", attached_to: "F-2" },
          { relationships: { attached_to: related("feature", "F-2", "done", "Journal Experience") } },
        )}
      />,
    );
    const value = factValue(container, "Found in");
    expect(within(value).getByRole("link", { name: "Journal Experience" })).toHaveAttribute(
      "href",
      "/node/feature/F-2",
    );
    expect(within(value).getByText("done")).toHaveClass("chip");
  });

  it("draws no facts block at all when the node carries none of its facts", () => {
    const { container } = mount(<NodeDetail page={pageOf(task)} />);
    expect(container.querySelector(".node-facts")).toBeNull();
  });

  it("keeps the source out of a feature's header (it lives on the Spec head now)", () => {
    const { container } = mount(
      <NodeDetail page={pageOf({ ...feature, source: "design/04-platform.md" })} />,
    );
    expect(header(container).queryByText("design/04-platform.md")).toBeNull();
  });

  it("keeps a question's source in the header, since a question has no Spec section to carry it", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf({ noun: "question", id: "Q-1", status: "open", statement: "Five or six?", source: "design/q.md" })}
      />,
    );
    expect(factValue(container, "Source").textContent).toBe("design/q.md");
    expect(facts(container).getByRole("link", { name: "design/q.md" })).toHaveAttribute(
      "href",
      "/file/design/q.md#L1",
    );
  });
});

describe("Node header: the children's mix", () => {
  it("draws the children's state bar with a count caption on an epic", () => {
    const epic: NodeView = { noun: "epic", id: "E-1", key: "habit-engine", title: "Habit Engine", status: "active" };
    const { container } = mount(
      <NodeDetail
        page={pageOf(epic, {
          relationships: {
            features: [
              related("feature", "F-1", "done", "A"),
              related("feature", "F-2", "done", "B"),
              related("feature", "F-3", "ready", "C"),
            ],
          },
        })}
      />,
    );
    const mix = container.querySelector(".instr-header .node-mix") as HTMLElement;
    expect(mix).not.toBeNull();
    expect(mix.querySelector(".state-bar")).not.toBeNull();
    expect(mix.textContent).toContain("3 features");
    expect(mix.textContent).toContain("2 done");
    expect(mix.textContent).toContain("1 ready");
  });

  it("draws it for a feature over its tasks, counting the task noun", () => {
    const { container } = mount(
      <NodeDetail
        page={pageOf(feature, {
          relationships: { tasks: [related("task", "T-1", "done", "A")] },
        })}
      />,
    );
    expect((container.querySelector(".instr-header .node-mix") as HTMLElement).textContent).toContain(
      "1 task",
    );
  });

  it("draws no mix on a task, which has no children", () => {
    const { container } = mount(<NodeDetail page={pageOf(task)} />);
    expect(container.querySelector(".node-mix")).toBeNull();
  });
});

describe("ProseSection: the source rides the Spec head", () => {
  const prose = { path: ".tiller/epics/habit-engine/epic.md", content: "# Habit Engine\n\nBuild the domain foundation.\n" };

  it("links the source beside the head, on the left, and drops the path line", () => {
    const node: NodeView = { noun: "epic", id: "E-1", key: "habit-engine", title: "Habit Engine", status: "done", source: "design/02-domain-model.md" };
    const { container } = mount(<ProseSection prose={prose} label="Spec" node={node} />);
    const head = container.querySelector(".instr-fh-row") as HTMLElement;
    expect(head).not.toBeNull();
    expect(within(head).getByText("Spec")).toBeInTheDocument();
    const link = within(head).getByRole("link", { name: "design/02-domain-model.md" });
    expect(link).toHaveAttribute("href", "/file/design/02-domain-model.md#L1");
    expect(within(head).getByText(/^from/)).toBeInTheDocument();
    expect(container.querySelector(".prose-path")).toBeNull();
    expect(screen.queryByText(prose.path)).toBeNull();
  });

  it("renders a bare head, no 'from', when the node has no source", () => {
    const node: NodeView = { noun: "task", id: "T-1", key: "a/b/c", title: "C", status: "done" };
    const { container } = mount(<ProseSection prose={prose} label="Implementation spec" node={node} />);
    expect(screen.queryByText(/^from/)).toBeNull();
    expect(container.querySelector(".prose-path")).toBeNull();
    expect(screen.getByText("Implementation spec")).toBeInTheDocument();
  });

  it("opens a URL source in a new tab", () => {
    const node: NodeView = { noun: "feature", id: "F-1", key: "a/b", title: "B", status: "done", source: "https://example.com/spec" };
    mount(<ProseSection prose={prose} label="Spec" node={node} />);
    const link = screen.getByRole("link", { name: "https://example.com/spec" });
    expect(link).toHaveAttribute("href", "https://example.com/spec");
    expect(link).toHaveAttribute("target", "_blank");
  });
});

describe("NodeTabs: the rail's counts", () => {
  it("wraps a tab's count so the rail can mute it, with the accessible name unchanged", () => {
    render(
      <NodeTabs
        tabs={[
          { key: "spec", label: "Spec", render: () => <p>spec</p> },
          { key: "tasks", label: "Tasks", count: 4, render: () => <p>tasks</p> },
        ]}
        active="spec"
        onSelect={() => undefined}
      />,
    );
    const tab = screen.getByRole("button", { name: "Tasks · 4" });
    const count = tab.querySelector(".tab-count") as HTMLElement;
    expect(count).not.toBeNull();
    expect(count.textContent).toContain("4");
  });
});

describe("Chip: the hero size", () => {
  it("adds the hero class without changing the word or the color", () => {
    const { container } = render(
      <Chip status="done" size="hero">
        done
      </Chip>,
    );
    const chip = container.querySelector(".chip") as HTMLElement;
    expect(chip).toHaveClass("chip-hero");
    expect(chip.textContent).toBe("done");
  });

  it("stays the plain chip by default", () => {
    const { container } = render(<Chip status="done">done</Chip>);
    expect(container.querySelector(".chip")).not.toHaveClass("chip-hero");
  });
});
