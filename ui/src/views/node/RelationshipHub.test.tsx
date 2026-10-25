// RelationshipHub renders every graph edge the shell and body did not already
// pull forward - dependencies, supersession, question blocks. Task 2
// (node-detail-tabs) lifted the two parent-climb groups (epic, feature) into
// the node header and dropped the Source group entirely (the header's own
// "from <path>" caption is the one place source links into Explore's search
// now), so neither ever renders here regardless of what a caller passes.

import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { RelatedNode, Relationships } from "../../api";
import RelationshipHub, { dependencyCount } from "./RelationshipHub";

afterEach(() => {
  cleanup();
});

const depTask: RelatedNode = {
  noun: "task",
  id: "task-9",
  ref: "task-9",
  status: "ready",
  title: "Wire the review row",
};

const bugRelated: RelatedNode = {
  noun: "bug",
  id: "B-aaaaaa",
  ref: "B-aaaaaa",
  status: "open",
  title: "Blank on reload",
};

function renderHub(relationships: Relationships) {
  render(
    <MemoryRouter>
      <RelationshipHub relationships={relationships} claimed={[]} />
    </MemoryRouter>,
  );
}

describe("RelationshipHub", () => {
  it("draws nothing at all when there are no edges", () => {
    const { container } = render(
      <MemoryRouter>
        <RelationshipHub relationships={{}} claimed={[]} />
      </MemoryRouter>,
    );

    // An isolated node is the ordinary case early in a tree's life, not an
    // anomaly: a heading over "(none recorded)" was two lines saying what the
    // absence says by itself.
    expect(container.textContent).toBe("");
  });

  it("renders a relationship group", () => {
    renderHub({ depends_on: [depTask] });

    expect(screen.getByRole("heading", { name: /^Depends on/ })).toBeInTheDocument();
  });

  it("renders a duplicate target as a reachable dependency relation", () => {
    render(
      <MemoryRouter>
        <RelationshipHub
          relationships={{ duplicate_of: bugRelated }}
          claimed={[]}
          only={["duplicate_of"]}
        />
      </MemoryRouter>,
    );
    expect(screen.getByText("Duplicate of")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Blank on reload" })).toHaveAttribute(
      "href",
      "/node/bug/B-aaaaaa",
    );
  });

  it("keeps a non-question keyless target's id off the row", () => {
    renderHub({ depends_on: [depTask] });

    // Materialized questions expose their Q-id, but this task has no key and
    // retains the existing non-question grammar: title link and state only.
    const row = screen.getByText("Wire the review row").closest("li") as HTMLElement;
    expect(within(row).getByRole("link", { name: "Wire the review row" })).toHaveAttribute(
      "href",
      "/node/task/task-9",
    );
    expect(row.textContent).not.toContain("(task-9)");
    expect(within(row).getByText("ready")).toBeInTheDocument();
  });

  // Task 2: the parent-climb groups moved to the node header, so the hub
  // never renders them, whether or not a caller claims them and regardless of
  // an `only` scope - they are simply no longer in the hub's own group list.
  it("never renders the Epic or Feature groups, even when the payload carries them and nothing claims them", () => {
    const epicParent: RelatedNode = {
      noun: "epic",
      id: "epic-1",
      ref: "billing (epic-1)",
      status: "ready",
      title: "Billing overhaul",
    };
    const featureParent: RelatedNode = {
      noun: "feature",
      id: "feature-1",
      ref: "billing/refunds (feature-1)",
      status: "ready",
      title: "Refund pipeline",
    };
    renderHub({ epic: epicParent, feature: featureParent, depends_on: [depTask] });

    expect(screen.getByRole("heading", { name: /^Depends on/ })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Epic/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Feature/ })).not.toBeInTheDocument();
  });

  it("never renders a Source group; the hub has no source prop to render one from", () => {
    renderHub({ depends_on: [depTask] });
    expect(screen.queryByText("Source")).not.toBeInTheDocument();
  });
});

// `only` is the node-detail tab shell's addition (task-1-brief.md): the
// Dependencies tab wants exactly depends_on/depended_on_by, nothing else,
// regardless of what other groups the payload carries.
describe("RelationshipHub only", () => {
  it("renders only the named groups, even when other groups have rows", () => {
    render(
      <MemoryRouter>
        <RelationshipHub
          relationships={{
            supersedes: [{ ...depTask, noun: "feature", id: "feature-2", ref: "feature-2" }],
            depends_on: [depTask],
          }}
          claimed={[]}
          only={["depends_on", "depended_on_by"]}
        />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: /^Depends on/ })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Supersedes/ })).not.toBeInTheDocument();
  });

  it("draws nothing when only names groups the payload does not carry", () => {
    const { container } = render(
      <MemoryRouter>
        <RelationshipHub
          relationships={{ supersedes: [depTask] }}
          claimed={[]}
          only={["depends_on", "depended_on_by"]}
        />
      </MemoryRouter>,
    );

    expect(container.textContent).toBe("");
  });
});

describe("dependencyCount", () => {
  it("sums depends_on and depended_on_by", () => {
    expect(
      dependencyCount({ depends_on: [depTask, depTask], depended_on_by: [depTask] }),
    ).toBe(3);
  });

  it("treats an absent list as zero on either side", () => {
    expect(dependencyCount({})).toBe(0);
    expect(dependencyCount({ depends_on: [depTask] })).toBe(1);
    expect(dependencyCount({ depended_on_by: [depTask] })).toBe(1);
  });
});
