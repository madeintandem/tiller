// Vitest/RTL coverage for the node detail tab shell: NodeTabs
// itself (a controlled pill row: one button per tab, the active one's panel
// the only content in the document, an unrecognized `active` degrading to the
// first tab rather than a blank page), plus NodeDetail's own per-noun wiring
// of it (mirroring instrument.test.tsx's page-fixture idiom) - the settled tab
// sets, spec as the default, and a question staying flat with no tab row at
// all. Written red-first per the branch's TDD discipline.
//
// Task 17 (console v2, node page rebuild) added the amended tabs rule: a
// countable tab whose count is zero does not render, the evals tab (DISPLAY
// label "Contract" now, wire key unchanged) joins that rule when the node
// carries no eval contract at all, and spec always renders regardless. The
// default featurePage() fixture below now carries a real two-criterion
// contract, so the pre-existing "five tabs" tests still exercise all five;
// the zero-count and no-contract cases get their own fixtures further down.

import type { ReactElement } from "react";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AmendmentRelation, EvalContractView, NodePage, NodeView, RelatedNode } from "../../api";
import NodeDetail from "../NodeDetail";
import NodeTabs from "../NodeTabs";

afterEach(() => {
  cleanup();
});

function withRouter(ui: ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

// CopyCmd only writes to the clipboard when navigator.clipboard exists (a
// secure-context API jsdom omits by default); stubbing it in matches the
// convention instrument.test.tsx already established, so a NodeDetail render
// with an attention strip or basis-drift line never throws on a click.
function stubClipboard() {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: () => Promise.resolve() },
    configurable: true,
  });
}

// ---------------------------------------------------------------------------
// NodeTabs: the tab shell itself, router-agnostic (a plain onSelect callback,
// not a Link - NodeDetail is the one place the ?tab= param is read or written).
// ---------------------------------------------------------------------------

describe("NodeTabs", () => {
  it("renders one button per tab, bare or with its count folded onto the label", () => {
    render(
      <NodeTabs
        tabs={[
          { key: "spec", label: "spec", render: () => <p>Spec panel</p> },
          { key: "tasks", label: "tasks", count: 4, render: () => <p>Tasks panel</p> },
        ]}
        active="spec"
        onSelect={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "spec" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tasks · 4" })).toBeInTheDocument();
  });

  it("calls onSelect with the clicked tab's key", () => {
    const onSelect = vi.fn();
    render(
      <NodeTabs
        tabs={[
          { key: "spec", label: "spec", render: () => <p>Spec panel</p> },
          { key: "evals", label: "evals", render: () => <p>Evals panel</p> },
        ]}
        active="spec"
        onSelect={onSelect}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "evals" }));
    expect(onSelect).toHaveBeenCalledWith("evals");
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it("renders only the active tab's panel content, never an inactive one", () => {
    render(
      <NodeTabs
        tabs={[
          { key: "spec", label: "spec", render: () => <p>Spec panel</p> },
          { key: "evals", label: "evals", render: () => <p>Evals panel</p> },
        ]}
        active="evals"
        onSelect={() => {}}
      />,
    );

    expect(screen.getByText("Evals panel")).toBeInTheDocument();
    expect(screen.queryByText("Spec panel")).not.toBeInTheDocument();
  });

  it("falls back to the first tab when active names none of the given tabs", () => {
    render(
      <NodeTabs
        tabs={[
          { key: "spec", label: "spec", render: () => <p>Spec panel</p> },
          { key: "evals", label: "evals", render: () => <p>Evals panel</p> },
        ]}
        active="not-a-real-tab"
        onSelect={() => {}}
      />,
    );

    expect(screen.getByText("Spec panel")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "spec" })).toHaveClass("active");
  });
});

// ---------------------------------------------------------------------------
// NodeDetail: per-noun tab configs built from the NodePage fields, spec as
// the default, a question rendered flat with no tab row.
// ---------------------------------------------------------------------------

function featureNode(overrides: Partial<NodeView> = {}): NodeView {
  return {
    noun: "feature",
    id: "F-1",
    key: "billing/refunds",
    title: "Refund pipeline",
    status: "ready",
    ...overrides,
  };
}

const featureContract: EvalContractView = {
  path: "features/billing/refunds/feature.eval.md",
  node: "F-1",
  criteria: [
    {
      id: "C1",
      statement: "A refund posts exactly once per request.",
      polarity: "introduce",
      required: true,
      binding: { type: "test", ref: "spec/refund_spec.rb" },
    },
    {
      id: "C2",
      statement: "A duplicate refund request is refused, not silently ignored.",
      polarity: "introduce",
      required: true,
      binding: { type: "manual" },
    },
  ],
};

function featurePage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: featureNode({ gates: { evals: { required: true, status: "pending" } } }),
    ref: "billing/refunds (F-1)",
    prose: {
      path: ".tiller/epics/billing/features/refunds/feature.md",
      content: "# Refund pipeline\n\nSpec body text.\n",
    },
    relationships: {
      tasks: [
        { noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Wire the refund job" },
      ],
      depends_on: [
        { noun: "feature", id: "F-2", ref: "F-2", status: "ready", title: "Ledger" },
      ],
      depended_on_by: [
        { noun: "feature", id: "F-3", ref: "F-3", status: "draft", title: "Reporting" },
      ],
    },
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [{ at: "2026-07-20T10:00:00Z", type: "add.feature", contributor: "jc", command: "add feature" }],
    eval: featureContract,
    ...overrides,
  };
}

function taskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "task", id: "T-1", key: "billing/refunds/wire-job", title: "Wire the job", status: "ready" },
    ref: "billing/refunds/wire-job (T-1)",
    prose: { path: ".tiller/epics/billing/features/refunds/tasks/wire-job/task.md", content: "" },
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

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

const bugRelated: RelatedNode = {
  noun: "bug",
  id: "B-aaaaaa",
  ref: "B-aaaaaa",
  status: "open",
  title: "Blank on reload",
};

const choreRelated: RelatedNode = {
  noun: "chore",
  id: "CH-bbbbbb",
  ref: "CH-bbbbbb",
  status: "open",
  title: "Bump Go",
};

const tweakRelated: RelatedNode = {
  noun: "tweak",
  id: "TW-cccccc",
  ref: "TW-cccccc",
  status: "ready",
  title: "Polish button spacing",
};

function nodePage({
  noun,
  attached_work,
}: {
  noun: string;
  attached_work?: RelatedNode[];
}): NodePage {
  const composition =
    noun === "epic"
      ? {
          features: [
            {
              noun: "feature",
              id: "F-child",
              ref: "F-child",
              status: "ready",
              title: "Console",
            },
          ],
        }
      : noun === "feature"
        ? {
            tasks: [
              {
                noun: "task",
                id: "T-child",
                ref: "T-child",
                status: "ready",
                title: "Render the board",
              },
            ],
          }
        : {};
  return {
    node: {
      noun,
      id: `${noun}-1`,
      key: `${noun}-key`,
      title: `${noun} title`,
      status: "ready",
    },
    ref: `${noun}-key (${noun}-1)`,
    prose: {
      path: `.tiller/${noun}.md`,
      content: `# ${noun} title\n\nShelf prose.\n`,
    },
    relationships: {
      ...composition,
      ...(attached_work === undefined ? {} : { attached_work }),
    },
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    
  };
}

function renderNode(page: NodePage, initialEntry = "/") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NodeDetail page={page} />
    </MemoryRouter>,
  );
}

const BLOCKER_QUESTION = {
  noun: "question",
  id: "Q-ledger-choice",
  ref: "Q-ledger-choice",
  status: "open",
  title: "Choose the posting ledger",
  statement:
    "Should refunds post to the original charge ledger or to a dedicated refunds ledger when the charge crosses an accounting period?",
};

function expectBlockerOnlyDependencies() {
  expect(screen.queryByRole("button", { name: "Dependencies · 0" })).not.toBeInTheDocument();

  const tab = screen.getByRole("button", { name: "Dependencies" });
  fireEvent.click(tab);

  expect(screen.getByRole("heading", { name: /^Blocked by questions/ })).toBeInTheDocument();
  const questionLink = screen.getByRole("link", { name: BLOCKER_QUESTION.title });
  expect(questionLink).toHaveAttribute("href", `/node/question/${BLOCKER_QUESTION.id}`);

  const row = questionLink.closest("li") as HTMLElement;
  expect(within(row).getByText(BLOCKER_QUESTION.id)).toBeInTheDocument();
  expect(within(row).getByText("open")).toHaveClass("chip");
  expect(screen.queryByText(BLOCKER_QUESTION.statement)).not.toBeInTheDocument();
}

describe("NodeDetail feature tabs", () => {
  it("shows the five settled tab labels with their counts, spec panel by default", () => {
    withRouter(<NodeDetail page={featurePage()} />);

    expect(screen.getByRole("button", { name: "Spec" })).toBeInTheDocument();
    // The evals tab's DISPLAY label reads "Contract" now (task 17's own
    // decision); its wire key and the `tab` search param stay "evals". Every
    // tab label is Title Case (the mock's
    // own tab row is Title Case throughout, not just Contract).
    expect(screen.getByRole("button", { name: "Contract · 2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tasks · 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dependencies · 2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "History · 1" })).toBeInTheDocument();

    // Spec is the default: its content shows with no click, and the evals
    // gate card (a different panel) has not rendered yet.
    expect(screen.getByText(/Spec body text/)).toBeInTheDocument();
    expect(screen.queryByText("required gate")).not.toBeInTheDocument();
  });

  it("swaps to the Contract panel on click (the evals wire key, relabeled), and the spec panel leaves the document", () => {
    withRouter(<NodeDetail page={featurePage()} />);

    fireEvent.click(screen.getByRole("button", { name: "Contract · 2" }));

    expect(screen.getByText("required gate")).toBeInTheDocument();
    expect(screen.queryByText(/Spec body text/)).not.toBeInTheDocument();
  });

  it("swaps to the tasks panel on click", () => {
    withRouter(<NodeDetail page={featurePage()} />);

    fireEvent.click(screen.getByRole("button", { name: "Tasks · 1" }));

    expect(screen.getByText("Wire the refund job")).toBeInTheDocument();
  });

  it("dependencies panel's label counts only depends_on/depended_on_by, and never repeats the parent-epic or Source facts the header already states", () => {
    const page = featurePage({
      node: featureNode({ source: "design/refunds.md" }),
      relationships: {
        ...featurePage().relationships,
        epic: { noun: "epic", id: "E-1", ref: "billing (E-1)", status: "ready", title: "Billing" },
      },
    });
    const { container } = withRouter(<NodeDetail page={page} />);

    // Task 2 (node-detail-tabs): the epic climb renders in the header, with
    // no click required, and Source never renders anywhere at all. The
    // crumb leads with the epic's TITLE (task 17's own breadcrumb rebuild:
    // crumbLabel prefers title over ref, the same reference-convention
    // carve-out every other rendered row already takes).
    const header = within(container.querySelector(".instr-header") as HTMLElement);
    expect(header.getByRole("link", { name: "Billing" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Source" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Epic/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Dependencies · 2" }));

    // The two dependency directions the label counts.
    expect(screen.getByRole("heading", { name: /^Depends on/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /^Depended on by/ })).toBeInTheDocument();
    // Neither the parent climb nor Source ever renders in this tab: the
    // header is their one owner now, not a widened `only`.
    expect(screen.queryByRole("heading", { name: /^Epic/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Source" })).not.toBeInTheDocument();
  });

  // Whole-branch review finding: `only` had narrowed to exactly the two
  // dependency directions, which silently dropped the UI's only rendering of
  // blocked_by_questions (populated server-side for every noun,
  // internal/views/nodepage.go:330) and a feature's supersedes/superseded_by
  // (nodepage.go:307-311) - a blocked feature's header shows the bare
  // "blocked" span with no route at all to the question that is blocking it.
  // The Dependencies tab is the walking-edges tab (ui-surface.md's
  // relationship-hub bullet), not a dependencies-only one; only its label
  // stays scoped to the two dependency directions the settled count formula
  // names.
  it("also renders supersedes and blocked_by_questions in the dependencies tab, with the label still counting only the two dependency directions", () => {
    const page = featurePage({
      relationships: {
        ...featurePage().relationships,
        supersedes: [{ noun: "feature", id: "F-9", ref: "F-9", status: "done", title: "Old refunds" }],
        blocked_by_questions: [
          { noun: "question", id: "Q-9", ref: "Q-9", status: "open", title: "Which ledger?" },
        ],
      },
    });
    withRouter(<NodeDetail page={page} />);

    // Two more groups now carry rows, but the label's own count is untouched:
    // the settled formula is depends_on plus depended_on_by, nothing else.
    expect(screen.getByRole("button", { name: "Dependencies · 2" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Dependencies · 2" }));

    expect(screen.getByRole("heading", { name: /^Supersedes/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /^Blocked by questions/ })).toBeInTheDocument();
  });
});

describe("NodeDetail blocker-only dependency tabs", () => {
  it("keeps an epic's blocker-only Dependencies tab reachable without presenting a zero count", () => {
    const page: NodePage = {
      node: { noun: "epic", id: "E-1", key: "billing", title: "Billing", status: "draft" },
      ref: "billing (E-1)",
      prose: null,
      relationships: { blocked_by_questions: [BLOCKER_QUESTION] },
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    withRouter(<NodeDetail page={page} />);

    expectBlockerOnlyDependencies();
  });

  it("keeps a feature's blocker-only Dependencies tab reachable without presenting a zero count", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          relationships: { blocked_by_questions: [BLOCKER_QUESTION] },
        })}
      />,
    );

    expectBlockerOnlyDependencies();
  });
});

describe("NodeDetail task tabs", () => {
  it("shows gates, not evals, and spec by default", () => {
    withRouter(<NodeDetail page={taskPage()} />);

    expect(screen.getByRole("button", { name: "Spec" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Gates" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "evals" })).not.toBeInTheDocument();
  });

  it("shows the parent-feature climb in the header, never as a Feature section in the dependencies tab", () => {
    const page = taskPage({
      relationships: {
        feature: { noun: "feature", id: "F-1", ref: "billing/refunds (F-1)", status: "ready", title: "Refund pipeline" },
        // A dependency is required to keep the Dependencies tab itself
        // present under task 17's own hide-at-zero rule; this test's own
        // concern is what the tab does NOT show, not its count.
        depends_on: [{ noun: "task", id: "T-2", ref: "T-2", status: "ready", title: "Other task" }],
      },
    });
    const { container } = withRouter(<NodeDetail page={page} />);

    // Task 2 (node-detail-tabs): the climb is header content now, visible
    // with no tab click at all, its crumb leading with the parent's own
    // title (task 17's breadcrumb rebuild).
    const header = within(container.querySelector(".instr-header") as HTMLElement);
    expect(header.getByRole("link", { name: "Refund pipeline" })).toBeInTheDocument();

    expect(screen.queryByRole("heading", { name: "Feature" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^Dependencies/ }));
    expect(screen.queryByRole("heading", { name: "Feature" })).not.toBeInTheDocument();
  });
});

describe("NodeDetail shelf and attached-work tabs", () => {
  it.each(["epic", "feature", "task"])(
    "shows one rolled-up Fix tab on a %s page",
    (noun) => {
      const page = nodePage({ noun, attached_work: [bugRelated, choreRelated, tweakRelated] });
      const { container } = renderNode(page, "/?tab=work-items");

      expect(screen.getByRole("button", { name: "Fix · 3" })).toHaveAttribute(
        "aria-current",
        "true",
      );
      expect(screen.getByRole("link", { name: "Blank on reload" })).toHaveAttribute(
        "href",
        "/node/bug/B-aaaaaa",
      );
      expect(screen.getByRole("link", { name: "Bump Go" })).toHaveAttribute(
        "href",
        "/node/chore/CH-bbbbbb",
      );
      expect(screen.getByRole("link", { name: "Polish button spacing" })).toHaveAttribute(
        "href",
        "/node/tweak/TW-cccccc",
      );

      const nav = within(container.querySelector(".mode-nav") as HTMLElement);
      const labels = nav.getAllByRole("button").map((button) => button.textContent);
      const before =
        noun === "epic" ? "Features · 1" : noun === "feature" ? "Tasks · 1" : "Gates";
      expect(labels.indexOf("Fix · 3")).toBe(labels.indexOf(before) + 1);
    },
  );

  it.each(["epic", "feature", "task"])(
    "hides the Fix tab at zero on a %s page",
    (noun) => {
      renderNode(nodePage({ noun, attached_work: [] }));
      expect(screen.queryByRole("button", { name: /^Fix/ })).not.toBeInTheDocument();
    },
  );

  it.each(["bug", "chore", "tweak"])("uses the task-shaped tab set for a %s", (noun) => {
    const { container } = renderNode(nodePage({ noun }));
    const nav = within(container.querySelector(".mode-nav") as HTMLElement);
    expect(nav.getAllByRole("button").map((button) => button.textContent)).toEqual([
      "Spec",
      "Gates",
    ]);
    expect(nav.queryByRole("button", { name: "Contract" })).not.toBeInTheDocument();
  });

  it.each(["bug", "chore", "tweak"])("keeps the approval command reachable for a proposed %s", (noun) => {
    const page = nodePage({ noun });
    page.node.status = "proposed";
    const command = `tiller approve ${noun} ${page.node.id}`;
    withRouter(<NodeDetail page={page} queue={{ entries: [{
      kind: `approve_${noun}`, noun, id: page.node.id, ref: page.ref, command,
    }] }} />);
    expect(screen.getByText(command)).toBeInTheDocument();
  });
});

// The amended tabs rule (ui-surface.md, 2026-08-10): "a countable tab whose
// count is zero does not render at all... spec always renders." The phase's
// own acceptance line, verbatim: "a dependency-free node shows no
// Dependencies tab."
describe("NodeDetail tabs: countable tabs hide at zero", () => {
  it("shows no Dependencies tab on a dependency-free node - the phase acceptance case", () => {
    withRouter(<NodeDetail page={taskPage()} />);
    expect(screen.queryByRole("button", { name: /^Dependencies/ })).not.toBeInTheDocument();
  });

  it("shows no History tab with zero recorded events", () => {
    withRouter(<NodeDetail page={taskPage()} />);
    expect(screen.queryByRole("button", { name: /^History/ })).not.toBeInTheDocument();
  });

  it("shows no Contract tab when the node carries no eval contract at all", () => {
    withRouter(<NodeDetail page={featurePage({ eval: null })} />);
    expect(screen.queryByRole("button", { name: /^Contract/ })).not.toBeInTheDocument();
    // The wire key stays "evals"; no button reads that word either, since
    // the label is the only thing a reader (or this query) ever sees.
    expect(screen.queryByRole("button", { name: /^evals/ })).not.toBeInTheDocument();
  });

  it("shows no Tasks tab on a feature with no tasks", () => {
    withRouter(
      <NodeDetail page={featurePage({ relationships: { ...featurePage().relationships, tasks: [] } })} />,
    );
    expect(screen.queryByRole("button", { name: /^Tasks/ })).not.toBeInTheDocument();
  });

  it("shows no Features tab on an epic carving nothing yet", () => {
    withRouter(
      <NodeDetail
        page={{
          node: { noun: "epic", id: "E-1", key: "billing", title: "Billing", status: "draft" },
          ref: "billing (E-1)",
          prose: null,
          relationships: {},
          backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
          history: [],
          eval: null,
        }}
      />,
    );
    expect(screen.queryByRole("button", { name: /^Features/ })).not.toBeInTheDocument();
    // spec always renders regardless.
    expect(screen.getByRole("button", { name: "Spec" })).toBeInTheDocument();
  });

  it("still shows the Contract tab for a malformed contract - a validation failure is never hidden, even by this new rule", () => {
    withRouter(<NodeDetail page={featurePage({ eval: { path: "x", error: "malformed" } })} />);
    // No count: nothing to count when the file failed to load.
    expect(screen.getByRole("button", { name: "Contract" })).toBeInTheDocument();
  });

  // JC's ruling, 2026-08-11: a well-formed contract authored with zero
  // criteria used to hide the Contract tab the same way any other zero-count
  // tab does (contractCount fell through to a real 0). Emptiness is
  // information here, not absence - the tab now shows bare, and its body
  // says so honestly rather than rendering a criteria-less GateCard.
  it("shows a bare Contract tab for a well-formed contract with zero criteria - authored-empty is information, not absence", () => {
    withRouter(<NodeDetail page={featurePage({ eval: { path: "x", node: "F-1", criteria: [] } })} />);

    const tab = screen.getByRole("button", { name: "Contract" });
    expect(tab).toBeInTheDocument();

    fireEvent.click(tab);
    expect(screen.getByText("The contract is authored but carries no criteria.")).toBeInTheDocument();
  });

  it("spec always renders: only Spec and Gates survive on a task with nothing else countable", () => {
    const { container } = withRouter(<NodeDetail page={taskPage()} />);
    const nav = container.querySelector(".mode-nav") as HTMLElement;
    const labels = within(nav)
      .getAllByRole("button")
      .map((b) => b.textContent);
    expect(labels).toEqual(["Spec", "Gates"]);
  });
});

describe("NodeDetail question page: no tab row", () => {
  it("renders no tab row at all, the flat body still intact", () => {
    stubClipboard();
    const { container } = withRouter(<NodeDetail page={questionPage()} />);

    expect(container.querySelector(".mode-nav")).toBeNull();
    expect(screen.queryByRole("button", { name: "Spec" })).not.toBeInTheDocument();
    // QuestionBody itself renders no answer command in any state now: the
    // attention stack owns that command, covered by instrument.test.tsx's own
    // "the stack owns a question's answer command, never duplicated by the
    // body" test.
  });
});

describe("NodeDetail amendment navigation", () => {
  const proposal: AmendmentRelation = {
    amendment: { noun: "amendment", id: "A-new", key: "reshape", title: "Reshape refunds", ref: "reshape (A-new)", status: "proposed" },
    proposed_at: "2026-09-07T10:00:00Z",
    effects: [
      { operation_index: 1, operation: "revise", role: "target", scope: "direct" },
      { operation_index: 2, operation: "add-task", role: "addition_parent", scope: "descendant", node: { noun: "feature", id: "F-child", ref: "F-child", status: "ready" } },
    ],
  };
  const approved: AmendmentRelation = {
    amendment: { ...proposal.amendment, id: "A-old", title: "Earlier reshape", ref: "reshape (A-old)", status: "approved" },
    approved_at: "2026-09-06T11:00:00Z",
    effects: [{ operation: "add-task", role: "addition_parent", scope: "historical_parent", reference: "billing/refunds", note: "Original parent recorded at application." }],
  };

  it("opens the direct tab URL with one row per amendment, reused-key ID links, actual dates and effect scopes", () => {
    renderNode(featurePage({ relationships: { amendments: [proposal, approved] } }), "/node/feature/F-1?tab=amendments");
    expect(screen.getByRole("button", { name: "Amendments · 2" })).toHaveAttribute("aria-current", "true");
    const panel = screen.getByRole("heading", { name: "Related amendments" }).parentElement!;
    expect(within(panel).getAllByRole("link", { name: "Reshape refunds" })).toHaveLength(1);
    expect(within(panel).getByRole("link", { name: "Reshape refunds" })).toHaveAttribute("href", "/node/amendment/A-new");
    expect(within(panel).getByRole("link", { name: "Earlier reshape" })).toHaveAttribute("href", "/node/amendment/A-old");
    expect(within(panel).getByText("2026-09-07T10:00:00Z")).toHaveAttribute("datetime", proposal.proposed_at);
    expect(within(panel).getByText("2026-09-06T11:00:00Z")).toHaveAttribute("datetime", approved.approved_at);
    expect(within(panel).getByText(/Operation 1 · revise · target · direct/)).toBeInTheDocument();
    expect(within(panel).getByText(/addition parent · descendant/)).toBeInTheDocument();
    expect(within(panel).queryByText(/Withdrawn/)).not.toBeInTheDocument();
  });

  it("keeps all proposed notices beside drift and acceptance on Spec and Contract without adding approval controls", () => {
    const page = featurePage({
      relationships: { amendments: [proposal, { ...proposal, amendment: { ...proposal.amendment, id: "A-other", title: "Second proposal" } }, approved] },
      active_basis: { noun: "feature", id: "F-1", ref: "F-1", drifted: ["feature.md"], command: "tiller approve feature F-1 --refresh-basis" },
    });
    withRouter(<NodeDetail page={page} queue={{ entries: [{
      kind: "attest_criterion", noun: "feature", id: "F-1", ref: "F-1", prose: "C2",
      command: "tiller attest feature F-1 --criterion C2 --status pass",
    }] }} />);
    const notice = screen.getByRole("complementary", { name: "Proposed amendments" });
    expect(notice).toHaveTextContent("2 proposed amendments");
    expect(within(notice).getAllByRole("link")).toHaveLength(2);
    expect(within(notice).queryByRole("link", { name: "Earlier reshape" })).not.toBeInTheDocument();
    expect(screen.getByText("active_basis")).toBeInTheDocument();
    expect(screen.getByText(/tiller attest feature F-1 --criterion C2 --status pass/)).toBeInTheDocument();
    expect(screen.getByText(/Spec body text/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Contract · 2" }));
    expect(screen.getByRole("complementary", { name: "Proposed amendments" })).toHaveTextContent("Second proposal");
    expect(screen.getByText("active_basis")).toBeInTheDocument();
    expect(screen.getByText(/tiller attest feature F-1 --criterion C2 --status pass/)).toBeInTheDocument();
    expect(screen.queryByText(/tiller approve amendment/)).not.toBeInTheDocument();
  });

  it("keeps incomplete history reachable without a count or empty-history assertion", () => {
    const warning = "Amendment history could not be loaded.";
    renderNode(featurePage({ relationships: { amendments: [], amendment_history_warning: warning } }), "/node/feature/F-1?tab=amendments");
    expect(screen.getByRole("button", { name: "Amendments" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("status")).toHaveTextContent(warning);
    expect(screen.queryByText(/No amendments|Amendments · 0/)).not.toBeInTheDocument();
  });

  it.each(["epic", "feature", "task"])("hides known-empty history on %s and falls back from its unavailable tab", (noun) => {
    const page = nodePage({ noun });
    renderNode(page, `/node/${noun}/${page.node.id}?tab=amendments`);
    expect(screen.queryByRole("button", { name: /^Amendments/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Spec" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByText(/Shelf prose/)).toBeInTheDocument();
  });

  it.each(["epic", "task"])("makes amendment history discoverable on %s without changing invalid-tab fallback", (noun) => {
    const page = nodePage({ noun });
    page.relationships.amendments = [approved];
    renderNode(page, `/node/${noun}/${page.node.id}?tab=unknown`);
    expect(screen.getByRole("button", { name: "Spec" })).toHaveAttribute("aria-current", "true");
    fireEvent.click(screen.getByRole("button", { name: "Amendments · 1" }));
    expect(screen.getByRole("link", { name: "Earlier reshape" })).toHaveAttribute("href", "/node/amendment/A-old");
  });
});

describe("NodeDetail backlink reachability", () => {
  const incomplete = {
    groups: [], count: 0, complete: false,
    diagnostics: [{ code: "history_unavailable", facet: "history", message: "Historical identities could not be loaded." }],
  };


  it("counts source identities rather than relationship groups or repeated citations", () => {
    const source = {
      vertex: {
        id: "doc:.tiller/design/refunds.md", kind: "document", path: ".tiller/design/refunds.md",
        destination: { kind: "file" as const, path: ".tiller/design/refunds.md" },
      },
      edges: [{
        id: "mentions", kind: "mentions", from: "doc:.tiller/design/refunds.md", to: "F-1",
        sources: [{ file: ".tiller/design/refunds.md", line: 2 }, { file: ".tiller/design/refunds.md", line: 7 }],
      }],
    };
    renderNode(featurePage({ backlinks: {
      count: 1, complete: true, diagnostics: [],
      groups: [
        { kind: "mentioned_by", sources: [source] },
        { kind: "source_for", sources: [{ ...source, edges: [{ ...source.edges[0], id: "source", kind: "sourced_from" }] }] },
      ],
    } }), "/node/feature/F-1?tab=backlinks");
    expect(screen.getByRole("button", { name: "Backlinks · 1" })).toHaveAttribute("aria-current", "true");
  });
  it("opens an incomplete zero-source tab without hiding its diagnostics or changing dependency and amendment warnings", () => {
    renderNode(featurePage({
      backlinks: incomplete,
      relationships: {
        ...featurePage().relationships,
        amendment_history_warning: "Amendment history could not be loaded.",
      },
    }), "/node/feature/F-1?tab=backlinks");
    expect(screen.getByRole("button", { name: "Backlinks" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("status")).toHaveTextContent(/incomplete/);
    expect(screen.getByText(/Historical identities could not be loaded/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dependencies · 2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Amendments" })).toBeInTheDocument();
  });

  it("does not turn a complete empty result into a reachable empty tab", () => {
    renderNode(featurePage(), "/node/feature/F-1?tab=backlinks");
    expect(screen.queryByRole("button", { name: /^Backlinks/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Spec" })).toHaveAttribute("aria-current", "true");
  });

  it.each(["question", "amendment"])("keeps incomplete backlinks visible on the flat %s page", (noun) => {
    renderNode(questionPage({
      node: { noun, id: noun === "question" ? "Q-1" : "A-1", status: "draft" },
      backlinks: incomplete,
    }));
    expect(screen.getByRole("heading", { name: /^Backlinks/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Backlinks/ })).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/incomplete/);
  });
});

describe("Transferred execution lineage", () => {
  it.each(["task", "bug", "chore", "tweak"])("keeps both replacement directions reachable for a %s without dependency edges", (noun) => {
    const original: RelatedNode = { noun, id: "old-id", ref: "old-id", title: "Original work", status: "cancelled" };
    const successor: RelatedNode = { noun, id: "new-id", ref: "new-id", title: "Replacement work", status: "ready" };
    const page: NodePage = {
      node: { noun, id: "middle-id", status: "cancelled", owner: "original-engineer", started: true,
        resolution: noun === "task" ? undefined : "transferred" },
      ref: "middle-id", prose: null, eval: null, history: [],
      relationships: { supersedes: [original], superseded_by: [successor] },
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    };
    withRouter(<NodeDetail page={page} />);
    const dependencies = screen.getByRole("button", { name: "Dependencies" });
    fireEvent.click(dependencies);
    expect(screen.getByRole("link", { name: "Original work" })).toHaveAttribute("href", `/node/${noun}/old-id`);
    expect(screen.getByRole("link", { name: "Replacement work" })).toHaveAttribute("href", `/node/${noun}/new-id`);
    expect(screen.getByText("original-engineer")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Duplicate of/ })).not.toBeInTheDocument();
  });
});

it("keeps recovered provenance visible on detail while current acceptance remains pending", () => {
  const recovered = {
    pr: 155, branch: "tiller/reviews/features/billing/refunds", head: "git:historical-head",
    merge_commit: "git:integration-result", integration_branch: "main", basis: "content",
    recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
    tasks: [], policy_snapshots: [{ node_id: "F-1", path: "feature.yml", source: "git:historical-policy",
      observations: [{ gate: "evals", status: "pending", unresolved_criteria: ["C8"] }] }],
  };
  const page = featurePage({
    node: featureNode({ status: "active", pr: 155, recovered_review: recovered }),
    workflow: { phase: "sync", actor: "agent", action: "sync", command: "tiller sync",
      reason: "Run ordinary sync on integration", review_id: "F-1", feature_id: "F-1",
      contract_status: "valid", acceptance_ready: false, submission_recorded: true,
      recovered_review: recovered, outstanding: [{ kind: "gate", id: "tests", status: "fail", actor: "agent" }] },
  });
  withRouter(<NodeDetail page={page} queue={{ entries: [] }} />);
  const panel = screen.getByRole("region", { name: /recovered historical review/i });
  expect(within(panel).getByText("git:historical-head")).toBeInTheDocument();
  expect(within(panel).getByText(/C8/)).toBeInTheDocument();
  expect(within(panel).getByText(/tests: fail/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
  expect(screen.queryByText(/awaiting.*merge/i)).not.toBeInTheDocument();
});
