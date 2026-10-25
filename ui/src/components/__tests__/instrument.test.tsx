// Vitest/RTL coverage for task C3, the acceptance instrument (docs/plans/
// 2026-07-23-console-chart-table-mocks.html, "Round four — the acceptance
// instrument"). Written red-first per the branch's TDD discipline.
//
// The required scenarios from the task brief: the contract renders every
// criterion with its binding before any check has run; a needs_attestation
// criterion renders amber with the attest command; provenance renders the
// offline branch+head pair when pr is absent; tasks render in payload order.
// Plus the epic identity double-render resolution (identity renders once, in
// the header, never twice, even with a decomposition below it).
//
// Task 17 (console v2, node page rebuild) replaced the earlier single-trigger
// attention strip (lib/attention.ts's attentionAction, derived from the
// node's own gates/next_owner fields) with the attention STACK: every one of
// this node's own queue entries in the loud register, plus unpushed_review in
// the warning register, all at once. That derivation lived in this file's own
// "attention strip" describe block; it is now "the attention stack" below,
// built from QueueEntry fixtures passed as NodeDetail's own `queue` prop
// rather than from gates/next_owner.

import type { ReactElement } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  EvalCriterionView,
  NodePage,
  NodeView,
  PolicyViolationView,
  Queue,
  QueueEntry,
} from "../../api";
import CriterionRow from "../CriterionRow";
import GateCard, { EvalContractWarning, GateCardList } from "../GateCard";
import ProvenanceLine from "../ProvenanceLine";
import TaskList from "../TaskList";
import NodeDetail from "../NodeDetail";
import NodeDetailView from "../../views/NodeDetail";

afterEach(() => {
  cleanup();
});

// CopyCmd only renders its clipboard write when navigator.clipboard exists
// (a secure-context API jsdom omits by default); stubbing it in lets the
// attention-stack tests click through without a rejected-promise console
// error, matching the convention primitives.test.tsx already established.
beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: () => Promise.resolve() },
    configurable: true,
  });
});

function criterion(overrides: Partial<EvalCriterionView> = {}): EvalCriterionView {
  return {
    id: "C1",
    statement: "bin/ci runs lint, unit specs, and a system spec and is green.",
    polarity: "introduce",
    required: true,
    binding: { type: "command", run: "bin/ci" },
    ...overrides,
  };
}

function withRouter(ui: ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

// CriterionRow was rewritten for task 17 (console v2, node page rebuild) to
// the mock's own compact .crow grid (task-17-brief.md item 6's exhaustive
// field list: id, statement, binding TYPE, status chip, attested-by). Three
// things the old stacked row carried are gone from this row on purpose, not
// stranded: the binding's ref/run DETAIL (only its type now, the grid has no
// column for the rest), the evidence link (`/api/evidence?ref=…`, unaffected
// on GateCard's own flat-gate evidence row and on IntegrityNotes'
// AttestationRow, which still shows an attestation's own evidence_ref), and
// the per-criterion "required" tag (not among the brief's five fields).
describe("CriterionRow: the contract's own row (the mock's .crow grid)", () => {
  it("renders id, statement, and the binding's type alone before any check has run", () => {
    render(<CriterionRow criterion={criterion()} />);

    expect(screen.getByText("C1")).toBeInTheDocument();
    expect(
      screen.getByText("bin/ci runs lint, unit specs, and a system spec and is green."),
    ).toBeInTheDocument();
    // Type only - never the ref/run detail the old stacked row appended.
    expect(screen.getByText("command")).toBeInTheDocument();
    expect(screen.queryByText(/bin\/ci$/)).not.toBeInTheDocument();
    expect(screen.getByText("pending")).toBeInTheDocument();
  });

  it("paints the chip draft/slate before any check has run", () => {
    const { container } = render(<CriterionRow criterion={criterion()} />);
    const dot = container.querySelector(".state-dot") as HTMLElement;
    // draft's own custom property; jsdom never resolves var() to a color.
    expect(dot.style.background).toBe("var(--st-draft)");
  });

  it("names a test binding's type, never its ref", () => {
    render(
      <CriterionRow
        criterion={criterion({
          id: "C2",
          binding: { type: "test", ref: "spec/system/foundation/boot_health_spec.rb" },
        })}
      />,
    );
    expect(screen.getByText("test")).toBeInTheDocument();
    expect(screen.queryByText(/boot_health_spec/)).not.toBeInTheDocument();
  });
});

describe("CriterionRow: needs_attestation reads amber", () => {
  it("paints the chip amber (in_review) with its own plain word, the binding type unfolded beside it", () => {
    render(
      <CriterionRow
        criterion={criterion({
          id: "C6",
          statement: "Sentry receives a test event from the staging environment.",
          binding: { type: "manual" },
        })}
        result={{ status: "needs_attestation" }}
      />,
    );

    const chip = screen.getByText("needs attestation");
    expect(chip.closest(".chip")).toHaveAttribute("data-status", "in_review");
    // The binding type is its own cell now, never folded into the chip's word.
    expect(screen.getByText("manual")).toBeInTheDocument();
  });
});

describe("CriterionRow: attested-by (task 17 item 6's fifth field)", () => {
  it("folds the attester's name into the chip when a recorded attestation satisfied this criterion", () => {
    render(
      <CriterionRow
        criterion={criterion({ id: "C6", binding: { type: "manual" } })}
        result={{ status: "pass" }}
        attestations={[
          { id: "AT-1", target_kind: "criterion", target_id: "C6", status: "pass", by: "jc" },
        ]}
      />,
    );

    const chip = screen.getByText("attested · jc");
    expect(chip.closest(".chip")).toHaveAttribute("data-status", "done");
    expect(screen.queryByText("pass")).not.toBeInTheDocument();
  });

  it("reads the plain pass word when no attestation targets this criterion", () => {
    render(<CriterionRow criterion={criterion({ id: "C1" })} result={{ status: "pass" }} />);
    expect(screen.getByText("pass")).toBeInTheDocument();
  });

  it("never folds an attestation recorded against a different criterion", () => {
    render(
      <CriterionRow
        criterion={criterion({ id: "C1" })}
        result={{ status: "pass" }}
        attestations={[
          { id: "AT-1", target_kind: "criterion", target_id: "C6", status: "pass", by: "jc" },
        ]}
      />,
    );
    expect(screen.getByText("pass")).toBeInTheDocument();
    expect(screen.queryByText(/attested/)).not.toBeInTheDocument();
  });

  it("takes the LATEST attestation when more than one has ever targeted the same criterion", () => {
    render(
      <CriterionRow
        criterion={criterion({ id: "C6" })}
        result={{ status: "pass" }}
        attestations={[
          { id: "AT-1", target_kind: "criterion", target_id: "C6", status: "fail", by: "pat" },
          { id: "AT-2", target_kind: "criterion", target_id: "C6", status: "pass", by: "jc" },
        ]}
      />,
    );
    expect(screen.getByText("attested · jc")).toBeInTheDocument();
    expect(screen.queryByText(/pat/)).not.toBeInTheDocument();
  });

  it("never folds an attestation whose own recorded status was a fail, even targeting the right criterion", () => {
    // A live status the caller passed as "pass" here for isolation; the guard
    // under test is attestedByFor's own status check, not a live re-derivation.
    render(
      <CriterionRow
        criterion={criterion({ id: "C6" })}
        result={{ status: "fail" }}
        attestations={[
          { id: "AT-1", target_kind: "criterion", target_id: "C6", status: "fail", by: "jc" },
        ]}
      />,
    );
    expect(screen.getByText("fail")).toBeInTheDocument();
    expect(screen.queryByText(/attested/)).not.toBeInTheDocument();
  });
});

// Fix wave (task-17-review, Critical finding 1): the pass-1 row's per-
// criterion evidence link (CriterionStatusView.evidence, still served -
// populated by `tiller eval --record-rubric-output`) rendered nowhere after
// the rewrite. AskBox's own why sentence for an attest_criterion ask says
// "Review the graded evidence, then sign it off" - a fact the reader had no
// way to actually see. Restored as the statement cell's own second line.
describe("CriterionRow: per-criterion evidence (fix wave, Critical finding 1)", () => {
  it("renders the recorded evidence ref, linked to /api/evidence", () => {
    const ref = "artifacts/tiller/gates/evals-C3-rubric.txt";
    render(
      <CriterionRow
        criterion={criterion({ id: "C3", binding: { type: "eval" } })}
        result={{ status: "needs_attestation", evidence: [{ kind: "rubric_output", ref }] }}
      />,
    );

    const link = screen.getByRole("link", { name: `evidence: ${ref}` });
    expect(link).toHaveAttribute("href", `/api/evidence?ref=${encodeURIComponent(ref)}`);
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
  });

  it("renders no evidence link when the criterion carries no recorded evidence", () => {
    render(<CriterionRow criterion={criterion({ id: "C1" })} result={{ status: "pass" }} />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});

describe("GateCard: derived state line", () => {
  const criteria = [
    criterion({ id: "C1" }),
    criterion({ id: "C2", binding: { type: "test", ref: "spec/system/boot_health_spec.rb" } }),
    criterion({ id: "C3", binding: { type: "manual" } }),
  ];

  it("reads pending before any check has run and renders every criterion", () => {
    render(<GateCard name="evals" criteria={criteria} />);

    expect(screen.getByText("required gate")).toBeInTheDocument();
    expect(screen.getByText("pending — runs at check during execute")).toBeInTheDocument();
    expect(screen.getByText("C1")).toBeInTheDocument();
    expect(screen.getByText("C2")).toBeInTheDocument();
    expect(screen.getByText("C3")).toBeInTheDocument();
  });

  it("names one criterion awaiting attestation when the recorded result carries it", () => {
    render(
      <GateCard
        name="evals"
        gate={{
          required: true,
          criteria: {
            C1: { status: "pass" },
            C2: { status: "pass" },
            C3: { status: "needs_attestation" },
          },
        }}
        criteria={criteria}
      />,
    );

    expect(screen.getByText("1 criterion awaits your attestation")).toBeInTheDocument();
  });

  // "Green is quiet, amber is yours" (mock note 5, round 4). The mock varies
  // this line's color inline in both directions, mock:427 slate and mock:468
  // amber; the state has to reach the DOM for the stylesheet to do it, and it
  // is the same dotStatus the dot beside it already paints with.
  it("stamps the derived state on the line, so the quiet case can stay quiet", () => {
    const { container } = render(<GateCard name="evals" criteria={criteria} />);
    expect(container.querySelector(".instr-gate-state")?.getAttribute("data-state")).toBe("draft");
  });

  it("stamps in_review when a criterion is waiting on the human", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, criteria: { C1: { status: "needs_attestation" } } }}
        criteria={[criterion({ id: "C1" })]}
      />,
    );
    expect(container.querySelector(".instr-gate-state")?.getAttribute("data-state")).toBe(
      "in_review",
    );
  });

  it("stamps done for an all-pass gate, which the stylesheet keeps quiet", () => {
    const { container } = render(
      <GateCard name="tests" gate={{ required: true, status: "pass" }} />,
    );
    expect(container.querySelector(".instr-gate-state")?.getAttribute("data-state")).toBe("done");
  });

  it("renders a flat gate (no criteria) from its own recorded status, tagged a review gate when optional", () => {
    render(<GateCard name="code-review" gate={{ required: false, status: "pass" }} />);

    expect(screen.getByText("review gate")).toBeInTheDocument();
    expect(screen.getByText("passed")).toBeInTheDocument();
    expect(screen.queryByText("required gate")).not.toBeInTheDocument();
  });

  it("renders a flat gate's own stored revision inside that gate card", () => {
    const { container } = render(
      <GateCard
        name="tests"
        gate={{ required: true, status: "pass", against: "sha256:tests222" }}
      />,
    );

    const card = container.querySelector(".instr-gate") as HTMLElement;
    expect(within(card).getByText("checked against")).toBeInTheDocument();
    expect(within(card).getByText("sha256:tests222")).toBeInTheDocument();
  });

  it("renders a flat gate's own evidence, linked verbatim", () => {
    render(
      <GateCard
        name="tests"
        gate={{
          required: true,
          status: "pass",
          evidence: [{ kind: "command_output", ref: "artifacts/tiller/gates/tests-zzz.txt" }],
        }}
      />,
    );
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute(
      "href",
      "/api/evidence?ref=artifacts%2Ftiller%2Fgates%2Ftests-zzz.txt",
    );
  });
});

describe("GateCard: only required criteria gate the evals state line", () => {
  const criteria = [
    criterion({ id: "C1" }),
    criterion({ id: "C2", required: false, binding: { type: "manual" } }),
    criterion({ id: "C3", required: false }),
  ];

  it("reads a passing gate when required criteria pass, with optional detail still listed", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{
          required: true,
          status: "pass",
          criteria: {
            C1: { status: "pass" },
            C2: { status: "needs_attestation" },
            C3: { status: "fail" },
          },
        }}
        criteria={criteria}
      />,
    );
    const state = container.querySelector(".instr-gate-state");
    expect(state?.getAttribute("data-state")).toBe("done");
    expect(state).not.toHaveTextContent(/fail|attestation/);
    expect(screen.getByText("C2")).toBeInTheDocument();
    expect(screen.getByText("C3")).toBeInTheDocument();
  });

  it("still blocks on a required failure", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, criteria: { C1: { status: "fail" }, C3: { status: "pass" } } }}
        criteria={criteria}
      />,
    );
    expect(container.querySelector(".instr-gate-state")?.getAttribute("data-state")).toBe(
      "blocked",
    );
  });

  it("reads an all-optional contract as nonblocking", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, criteria: { C3: { status: "fail" } } }}
        criteria={[criterion({ id: "C3", required: false })]}
      />,
    );
    expect(container.querySelector(".instr-gate-state")?.getAttribute("data-state")).toBe("done");
  });
});

// Plan decision 9: once Go has recorded the evals gate, its stored status is
// the readiness fact submit judges. The criteria projection only details the
// label, and stands in only when no gate status was ever recorded.
describe("GateCard: a recorded evals gate status is the readiness fact", () => {
  function stateOf(container: HTMLElement) {
    return container.querySelector(".instr-gate-state");
  }

  it("reads an all-optional contract whose recorded gate is stale as not ready", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, status: "stale", criteria: { C1: { status: "stale" } } }}
        criteria={[criterion({ id: "C1", required: false })]}
      />,
    );
    expect(stateOf(container)?.getAttribute("data-state")).not.toBe("done");
    expect(stateOf(container)).toHaveTextContent("stale — needs a fresh check");
  });

  it("reads a recorded pass as passing even when a required criterion was added since", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, status: "pass", criteria: { C1: { status: "pass" } } }}
        criteria={[criterion({ id: "C1" }), criterion({ id: "C2" })]}
      />,
    );
    expect(stateOf(container)?.getAttribute("data-state")).toBe("done");
    expect(stateOf(container)).toHaveTextContent("passed at last check");
  });

  it("reads a recorded pending gate as not ready even when every required criterion reads pass", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, status: "pending", criteria: { C1: { status: "pass" } } }}
        criteria={[criterion({ id: "C1" })]}
      />,
    );
    expect(stateOf(container)?.getAttribute("data-state")).not.toBe("done");
  });

  it("keeps the projection's detail on a recorded failure", () => {
    const { container } = render(
      <GateCard
        name="evals"
        gate={{ required: true, status: "fail", criteria: { C1: { status: "fail" } } }}
        criteria={[criterion({ id: "C1" })]}
      />,
    );
    expect(stateOf(container)?.getAttribute("data-state")).toBe("blocked");
    expect(stateOf(container)).toHaveTextContent("1 criterion failed");
  });
});

describe("EvalContractWarning: never hide a malformed contract", () => {
  it("renders the served path and error verbatim", () => {
    const message = 'criterion C1 has no statement (run `tiller guide specify` for the contract shape)';
    render(<EvalContractWarning error={{ path: "features/x/feature.eval.md", error: message }} />);

    expect(screen.getByText("features/x/feature.eval.md")).toBeInTheDocument();
    expect(screen.getByText(message)).toBeInTheDocument();
  });
});

describe("GateCardList", () => {
  function node(overrides: Partial<NodeView> = {}): NodeView {
    return { noun: "feature", id: "F-1", status: "ready", ...overrides };
  }

  it("renders the eval-error warning instead of the criteria list when the contract failed to load", () => {
    render(
      <GateCardList
        node={node({ gates: { evals: { required: true, status: "pending" } } })}
        evalPayload={{ path: "features/x/feature.eval.md", error: "malformed" }}
      />,
    );
    expect(screen.getByText("malformed")).toBeInTheDocument();
    expect(screen.queryByText("evals")).not.toBeInTheDocument();
  });

  it("draws nothing for a fresh draft with no contract and no recorded gate", () => {
    // The note that used to stand here was a bare <p> with no section and no
    // heading, so on a stub feature it landed under the prose body's last
    // heading and read as that heading's content.
    const { container } = render(<GateCardList node={node()} evalPayload={null} />);
    expect(container.textContent).toBe("");
  });

  it("still draws a required gate that is merely unmet: pending is not empty", () => {
    render(
      <GateCardList
        node={node({ gates: { evals: { required: true, status: "pending" } } })}
        evalPayload={null}
      />,
    );
    expect(screen.getByText("evals")).toBeInTheDocument();
    expect(screen.getByText("required gate")).toBeInTheDocument();
  });

  it("renders the gate name in the mock's sans voice, not mono", () => {
    // Mono is reserved for keys, ids, paths, and commands; a gate's name is a
    // name (mock:154 renders .gname at 13.5px sans 650).
    render(
      <GateCardList
        node={node({ gates: { evals: { required: true, status: "pending" } } })}
        evalPayload={null}
      />,
    );
    expect(screen.getByText("evals")).not.toHaveClass("mono");
  });

  it("renders the evals card plus every other recorded gate, in name order", () => {
    render(
      <GateCardList
        node={node({
          gates: {
            evals: { required: true, status: "pending" },
            tests: { required: true, status: "pass" },
            build: { required: true, status: "pass" },
          },
        })}
        evalPayload={{ path: "x", node: "F-1", criteria: [criterion()] }}
      />,
    );
    const names = screen.getAllByText(/^(evals|tests|build)$/).map((el) => el.textContent);
    expect(names).toEqual(["evals", "build", "tests"]);
  });
});

describe("ProvenanceLine", () => {
  it("renders the offline branch and head when there is no pr", () => {
    render(
      <ProvenanceLine
        against="sha256:9f3c21abc"
        reviewRef={{ branch: "tiller/foundation/app-skeleton-ci-boot", head: "git:e5f9d28" }}
        violations={[]}
      />,
    );

    expect(screen.getByText("tiller/foundation/app-skeleton-ci-boot")).toBeInTheDocument();
    expect(screen.getByText("git:e5f9d28")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders a linked PR when pr_url exists, never the offline pair", () => {
    render(
      <ProvenanceLine
        pr={41}
        prUrl="https://github.com/acme/widget/pull/41"
        reviewRef={{ branch: "should-not-show", head: "git:0000000" }}
        violations={[]}
      />,
    );

    const link = screen.getByRole("link", { name: "#41" });
    expect(link).toHaveAttribute("href", "https://github.com/acme/widget/pull/41");
    expect(screen.queryByText("should-not-show")).not.toBeInTheDocument();
  });

  it("renders an honest PR number with no link when pr exists but serve resolved no origin", () => {
    render(<ProvenanceLine pr={41} violations={[]} />);

    expect(screen.getByText("review #41")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("always states the policy violation count, none included", () => {
    render(<ProvenanceLine against="git:e5f9d28" violations={[]} />);
    expect(screen.getByText("policy violations: none")).toBeInTheDocument();
  });

  it("counts recorded policy violations", () => {
    const violations: PolicyViolationView[] = [{ gate: "tests" }, { gate: "build" }];
    render(<ProvenanceLine against="git:e5f9d28" violations={violations} />);
    expect(screen.getByText("policy violations: 2")).toBeInTheDocument();
  });

  it("notes a dirty working tree fingerprint for a sha256 checked-against token", () => {
    render(<ProvenanceLine against="sha256:9f3c21abc" violations={[]} />);
    expect(screen.getByText(/\(working tree, dirty\)/)).toBeInTheDocument();
  });

  it("adds no dirty note for a clean commit token", () => {
    render(<ProvenanceLine against="git:e5f9d28" violations={[]} />);
    expect(screen.queryByText(/working tree, dirty/)).not.toBeInTheDocument();
  });

  it("renders nothing when there is no check yet and no review target", () => {
    const { container } = render(<ProvenanceLine violations={[]} />);
    expect(container.firstChild).toBeNull();
  });
});

describe("TaskList", () => {
  it("renders tasks in exactly payload order, never re-sorted", () => {
    const tasks = [
      {
        noun: "task",
        id: "T-zeta",
        key: "form-engine/loading/zeta",
        ref: "form-engine/loading/zeta (T-zeta)",
        status: "ready",
        title: "Zeta task",
      },
      {
        noun: "task",
        id: "T-alpha",
        key: "form-engine/loading/alpha",
        ref: "form-engine/loading/alpha (T-alpha)",
        status: "ready",
        title: "Alpha task",
      },
    ];
    withRouter(<TaskList tasks={tasks} />);

    const zeta = screen.getByText("Zeta task");
    const alpha = screen.getByText("Alpha task");
    expect(Boolean(zeta.compareDocumentPosition(alpha) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
  });

  // A task carries a key now, so the row obeys the console's reference
  // carve-out like every other rendered row: title first, key trailing as muted
  // mono, and the id nowhere on the row (ui-surface.md, "Rendered rows carve out
  // their own reference convention"). What trails is the LOCAL segment, since
  // the two above it are the feature this list already sits on.
  it("trails the local key segment, muted mono, and never the id", () => {
    const { container } = withRouter(
      <TaskList
        tasks={[
          {
            noun: "task",
            id: "T-br7z1r",
            key: "form-engine/nrca-form-loading/boot-the-app",
            ref: "form-engine/nrca-form-loading/boot-the-app (T-br7z1r)",
            status: "ready",
            title: "Boot the app",
          },
        ]}
      />,
    );
    expect(screen.getByText("boot-the-app")).toHaveClass("mono");
    expect(container.querySelector(".instr-taskrow-key")?.textContent).toBe("boot-the-app");
    expect(container.textContent).not.toContain("T-br7z1r");
  });

  it("renders each task's raw status as a row-local chip without changing its reference", () => {
    const { container } = withRouter(
      <TaskList
        tasks={[
          {
            noun: "task",
            id: "T-done",
            key: "form-engine/nrca-form-loading/boot-the-app",
            ref: "form-engine/nrca-form-loading/boot-the-app (T-done)",
            status: "done",
            title: "Boot the app",
          },
          {
            noun: "task",
            id: "T-review",
            key: "form-engine/nrca-form-loading/verify-startup",
            ref: "form-engine/nrca-form-loading/verify-startup (T-review)",
            status: "in_review",
            title: "Verify startup",
          },
        ]}
      />,
    );

    const [doneRow, reviewRow] = Array.from(
      container.querySelectorAll<HTMLElement>(".instr-taskrow"),
    );
    const doneChip = doneRow.querySelector(".chip");
    expect(doneChip).toHaveAttribute("data-status", "done");
    expect(doneChip).toHaveTextContent(/^done$/);
    expect(within(doneRow).getByRole("link", { name: "Boot the app" })).toHaveAttribute(
      "href",
      "/node/task/T-done",
    );
    expect(doneRow.querySelector(".instr-taskrow-key")?.textContent).toBe("boot-the-app");

    const reviewChip = reviewRow.querySelector(".chip");
    expect(reviewChip).toHaveAttribute("data-status", "in_review");
    expect(reviewChip).toHaveTextContent(/^in_review$/);
    expect(within(reviewRow).getByRole("link", { name: "Verify startup" })).toHaveAttribute(
      "href",
      "/node/task/T-review",
    );
    expect(reviewRow.querySelector(".instr-taskrow-key")?.textContent).toBe("verify-startup");

    expect(container.querySelector(".instr-fh-caption")?.textContent).toBe(
      "1 in_review · 1 done",
    );
  });

  // The list is the sibling dependency order gen.OrderTasks ranks, the same
  // sequence `tiller tree` prints, so the header says which order a reader is
  // looking at rather than naming a ledger that no longer exists.
  it("names the order it renders in", () => {
    withRouter(
      <TaskList
        tasks={[
          {
            noun: "task",
            id: "T-1",
            key: "e/f/one",
            ref: "e/f/one (T-1)",
            status: "ready",
            title: "One",
          },
        ]}
      />,
    );
    expect(screen.getByText(/Tasks · 1 · dependency order/)).toBeInTheDocument();
  });

  it("draws no list at all with no tasks", () => {
    // "Tasks · 0" over "(no tasks yet)" was two lines to report nothing, on the
    // state most features in a young tree are in.
    const { container } = withRouter(<TaskList tasks={[]} />);
    expect(container.textContent).toBe("");
  });
});

// ---------------------------------------------------------------------------
// NodeDetail integration: the attention strip trigger, the epic identity
// double-render resolution, and the eval contract error state end to end.
// ---------------------------------------------------------------------------

function featureNode(overrides: Partial<NodeView> = {}): NodeView {
  return {
    noun: "feature",
    id: "F-pueftm",
    key: "foundation/app-skeleton-ci-boot",
    title: "App Skeleton & Green CI Boot",
    status: "ready",
    ...overrides,
  };
}

function featurePage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: featureNode(),
    ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

function bugPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "bug", id: "B-current", title: "Current bug", status: "open" },
    ref: "B-current",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

const bugPageWithRelationships = bugPage({
  relationships: {
    attached_to: {
      noun: "task",
      id: "T-render",
      ref: "T-render",
      status: "active",
      title: "Render the board",
    },
    resolved_by: {
      noun: "feature",
      id: "F-console",
      ref: "F-console",
      status: "ready",
      title: "Console",
    },
  },
});

function queueEntry(overrides: Partial<QueueEntry> = {}): QueueEntry {
  return {
    kind: "attest_criterion",
    noun: "feature",
    id: "F-pueftm",
    ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
    prose: "C6",
    command: "tiller attest feature foundation/app-skeleton-ci-boot --criterion C6 --status pass",
    ...overrides,
  };
}

function queueOf(entries: QueueEntry[]): Queue {
  return { entries };
}

function amendmentPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "amendment", id: "AM-1", status: "proposed" },
    ref: "AM-1",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

// The attention stack (task 17, item 5): every item asking a person for
// something on this node renders as its own box, all at once - the loud
// register from the node's own queue entries (NodeDetail's `queue` prop,
// filtered by id), the warning register from unpushed_review. Replaces the
// earlier single-trigger strip this describe block used to cover.
describe("NodeDetail: the attention stack", () => {
  it("keeps human acceptance visible while machine work leads, then exposes the human handoff once", () => {
    const acceptance = queueEntry();
    const page = featurePage({
      node: featureNode({ status: "active" }),
      workflow: {
        phase: "checks",
        actor: "agent",
        action: "check",
        command: "tiller check feature F-pueftm",
        reason: "Build failed; C6 still needs human acceptance.",
        review_id: "F-pueftm",
        feature_id: "F-pueftm",
        contract_status: "valid",
        acceptance_ready: false,
        submission_recorded: false,
      },
    });
    const queue = queueOf([acceptance, queueEntry({
      kind: "workflow_closeout", actor: "agent", phase: "checks",
      prose: "Build failed; C6 still needs human acceptance.",
      command: "tiller check feature F-pueftm",
    })]);
    const view = withRouter(<NodeDetail page={page} queue={queue} />);
    expect(screen.getByText("Workflow · Agent")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "tiller check feature F-pueftm" })).toBeInTheDocument();
    expect(screen.getAllByText("Build failed; C6 still needs human acceptance.")).toHaveLength(1);
    expect(screen.getByRole("button", { name: acceptance.command })).toBeInTheDocument();

    view.rerender(
      <MemoryRouter>
        <NodeDetail
          page={{
            ...page,
            workflow: {
              ...page.workflow!,
              phase: "acceptance",
              actor: "human",
              action: "attest",
              command: acceptance.command,
              reason: "C6 awaits human acceptance; publication is not observed.",
            },
          }}
          queue={queue}
        />
      </MemoryRouter>,
    );
    expect(screen.getByText("Workflow · Human")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "tiller check feature F-pueftm" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: acceptance.command })).toHaveLength(1);
  });

  it("renders a queue ask as a loud box: kind (with its criterion qualifier), why, and command", () => {
    const page = featurePage();
    const { container } = withRouter(<NodeDetail page={page} queue={queueOf([queueEntry()])} />);

    const box = container.querySelector(".abox") as HTMLElement;
    expect(box).not.toHaveClass("warn");
    expect(within(box).getByText("Attest · eval criterion C6")).toBeInTheDocument();
    expect(
      within(box).getByText("Eval criterion recorded needs_attestation, awaiting your sign-off."),
    ).toBeInTheDocument();
    expect(
      within(box).getByRole("button", {
        name: "tiller attest feature foundation/app-skeleton-ci-boot --criterion C6 --status pass",
      }),
    ).toBeInTheDocument();
    // The page's own loud ink.
    expect(container.querySelectorAll(".copy-cmd-accent")).toHaveLength(1);
  });

  it("renders an approve ask with no qualifier, DecisionCard's own KIND_LABEL word verbatim", () => {
    const page = featurePage();
    const entry = queueEntry({
      kind: "approve_feature",
      prose: "App Skeleton & Green CI Boot",
      command: "tiller approve feature foundation/app-skeleton-ci-boot",
    });

    withRouter(<NodeDetail page={page} queue={queueOf([entry])} />);

    expect(screen.getByText("Approve · feature plan")).toBeInTheDocument();
    expect(screen.getByText("Spec written and waiting on your approval.")).toBeInTheDocument();
  });

  // The spec's own division of the attest surface by gate kind is UNCHANGED
  // by this rewrite (ui-surface.md's amended uniform-shell bullet, 2026-08-10:
  // "its division of the attest surface by gate kind ... is unchanged"): an
  // optional review gate's attest_gate entry stays Home queue-card work only.
  it("excludes an attest_gate entry: optional review gates stay Home-only", () => {
    const page = featurePage();
    const entry = queueEntry({
      kind: "attest_gate",
      gate: "security",
      gate_status: "pass",
      command: "tiller attest feature foundation/app-skeleton-ci-boot --gate security --status pass",
    });

    const { container } = withRouter(<NodeDetail page={page} queue={queueOf([entry])} />);
    expect(container.querySelector(".attn")).toBeNull();
  });

  it("excludes a queue entry addressed to a different node", () => {
    const page = featurePage();
    const { container } = withRouter(
      <NodeDetail page={page} queue={queueOf([queueEntry({ id: "F-other" })])} />,
    );
    expect(container.querySelector(".attn")).toBeNull();
  });

  it("stacks every queue entry addressed to this node, not just the first", () => {
    const page = featurePage();
    const queue = queueOf([
      queueEntry({ kind: "attest_criterion", prose: "C6" }),
      queueEntry({
        kind: "approve_feature",
        prose: "App Skeleton & Green CI Boot",
        command: "tiller approve feature foundation/app-skeleton-ci-boot",
      }),
    ]);

    const { container } = withRouter(<NodeDetail page={page} queue={queue} />);
    expect(container.querySelectorAll(".abox:not(.warn)")).toHaveLength(2);
  });

  // unpushed_review is task-keyed on the wire (internal/core/review.ForTask
  // resolves it from tree.Tasks, never tree.Features), so the server only
  // ever populates it on a task's own page - these three fixtures mount on
  // taskPage() rather than featurePage() to match that wire truth (phase-6
  // sweep finding: the earlier fixtures mounted it on a feature, a shape the
  // server never actually sends).
  it("renders unpushed_review as a warning box, a distinct register from the loud one", () => {
    const page = taskPage({
      node: { ...taskPage().node, pr: 41 },
      unpushed_review: {
        commit: "9f31c02",
        branch: "tiller/foundation/app-skeleton-ci-boot",
        upstream: "origin/tiller/foundation/app-skeleton-ci-boot",
      },
    });

    const { container } = withRouter(<NodeDetail page={page} />);

    const box = container.querySelector(".abox") as HTMLElement;
    expect(box).toHaveClass("warn");
    expect(within(box).getByText("Warning · state commit ahead of the PR")).toBeInTheDocument();
    expect(within(box).getByText("9f31c02")).toBeInTheDocument();
    expect(within(box).getByText(/PR #41/)).toBeInTheDocument();
    expect(
      within(box).getByRole("button", {
        name: "git push origin tiller/foundation/app-skeleton-ci-boot",
      }),
    ).toBeInTheDocument();
    // The loud ink is the ask register's alone; a caution copies quietly.
    expect(container.querySelectorAll(".copy-cmd-accent")).toHaveLength(0);
  });

  it("derives the push command's remote from the recorded upstream, never a hardcoded origin", () => {
    const page = taskPage({
      node: { ...taskPage().node, pr: 41 },
      unpushed_review: {
        commit: "9f31c02",
        branch: "tiller/foundation/app-skeleton-ci-boot",
        upstream: "upstream/tiller/foundation/app-skeleton-ci-boot",
      },
    });

    withRouter(<NodeDetail page={page} />);
    expect(
      screen.getByRole("button", {
        name: "git push upstream tiller/foundation/app-skeleton-ci-boot",
      }),
    ).toBeInTheDocument();
  });

  // The phase acceptance line, verbatim: "a node with an attest ask and an
  // unpushed commit shows both boxes at once."
  it("stacks a queue ask AND unpushed_review at once - the acceptance case", () => {
    const page = taskPage({
      node: { ...taskPage().node, pr: 41 },
      unpushed_review: {
        commit: "9f31c02",
        branch: "tiller/foundation/app-skeleton-ci-boot",
        upstream: "origin/tiller/foundation/app-skeleton-ci-boot",
      },
    });
    const entry = queueEntry({
      noun: "task",
      id: "T-h4k2wq",
      ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-h4k2wq)",
      command:
        "tiller attest task foundation/app-skeleton-ci-boot/boot-the-app --criterion C6 --status pass",
    });

    const { container } = withRouter(
      <NodeDetail page={page} queue={queueOf([entry])} />,
    );

    const boxes = container.querySelectorAll(".abox");
    expect(boxes).toHaveLength(2);
    expect(boxes[0]).not.toHaveClass("warn");
    expect(boxes[1]).toHaveClass("warn");
  });

  it("renders no attention stack at all with an empty queue and no unpushed_review", () => {
    const { container } = withRouter(<NodeDetail page={featurePage()} queue={queueOf([])} />);
    expect(container.querySelector(".attn")).toBeNull();
  });

  it("renders no attention stack when mounted with no queue prop at all (the standalone-route default)", () => {
    const { container } = withRouter(<NodeDetail page={featurePage()} />);
    expect(container.querySelector(".attn")).toBeNull();
  });

  // Fix wave (task-17-review, Important finding 2, flagged to JC):
  // ui-surface.md's amendment bullet (2026-08-06) is a specific, reasoned
  // decision the stack's generic "every item asking" wording does not
  // override - "the page offers no write at all: the copyable `tiller
  // approve amendment <addr>` lives on the queue card." A proposed
  // amendment's own page must never grow one.
  it("excludes an approve_amendment entry: the amendment's own copyable command stays queue-card-only", () => {
    const page = amendmentPage();
    const entry: QueueEntry = {
      kind: "approve_amendment",
      noun: "amendment",
      id: "AM-1",
      ref: "AM-1",
      command: "tiller approve amendment AM-1",
    };

    const { container } = withRouter(<NodeDetail page={page} queue={queueOf([entry])} />);
    expect(container.querySelector(".attn")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "tiller approve amendment AM-1" }),
    ).not.toBeInTheDocument();
  });

  // Fix wave (task-17-review, Important finding 3): QuestionBody no longer
  // renders its own copy of the answer command (views/node/QuestionBody.tsx);
  // the attention stack renders the question's own answer_question queue
  // entry as its own loud box instead, so exactly one copy of the command
  // should ever land on the page.
  it("the stack owns a question's answer command, never duplicated by the body", () => {
    const page: NodePage = {
      node: { noun: "question", id: "Q-1", status: "open", statement: "Five or six?" },
      ref: "Q-1",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const entry: QueueEntry = {
      kind: "answer_question",
      noun: "question",
      id: "Q-1",
      ref: "Q-1",
      command: 'tiller answer question Q-1 --answer "..."',
    };

    withRouter(<NodeDetail page={page} queue={queueOf([entry])} />);

    expect(screen.getAllByText('tiller answer question Q-1 --answer "..."')).toHaveLength(1);
  });
});

// Fix wave (task-17-review, Important finding 4): the pre-task-17 strip's
// third trigger, a plain human next-owner naming no verb the queue tracks
// at all, was a real behavior drop - a ready epic (only legal verb:
// cancel) rendered NOTHING on its own page after the rewrite. Restored as
// a fallback box (lib/attentionFallback.ts, reconstructed from the deleted
// lib/attention.ts), suppressed whenever the node already carries a real
// queue entry, of any kind, including one asksFrom itself declines to
// render.
describe("NodeDetail: the human-next-owner fallback", () => {
  function readyEpicPage(overrides: Partial<NodePage> = {}): NodePage {
    return {
      node: {
        noun: "epic",
        id: "E-1",
        key: "foundation",
        title: "Foundation",
        status: "ready",
        next_owner: "human",
      },
      ref: "foundation (E-1)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      ...overrides,
    };
  }

  // Also stands as the phase-4-review fix wave's own "ordinary node +
  // loaded-empty queue -> fallback still renders" pin below: `queueOf([])`
  // is a genuinely LOADED, genuinely silent queue (`{entries: []}`, never
  // `null`), the one case the null/failed-queue tests directly below this
  // one must NOT also suppress.
  it("renders the fallback box for a ready epic (only legal verb: cancel), the queue silent about it", () => {
    const { container } = withRouter(<NodeDetail page={readyEpicPage()} queue={queueOf([])} />);

    const box = container.querySelector(".abox") as HTMLElement;
    expect(box).not.toBeNull();
    expect(box).not.toHaveClass("warn");
    expect(within(box).getByText("Needs your attention next")).toBeInTheDocument();
    expect(
      within(box).getByRole("button", { name: "tiller show epic foundation" }),
    ).toBeInTheDocument();
  });

  // Phase-4-review fix wave, Important finding 1: the fallback used to fire
  // whenever entries.length was 0, which was ALSO true whenever the queue
  // itself had not loaded yet or had failed - entriesForNode's own
  // `queue?.entries ?? []` folded "not loaded", "failed", and "loaded but
  // silent" into the identical empty array. A proposed amendment
  // (next_owner human, a PRE_WORK_STATUSES status - needsAttentionFallback's
  // own eligible shape, which carries no amendment exclusion of its own,
  // unlike asksFrom's queue-entry filter) then flashed the forbidden
  // `tiller approve amendment <addr>` command spec:205 refuses, on every
  // load tick before the queue landed, and kept flashing it forever on a
  // failed read. Gating on `queue !== null` first closes both.
  //
  // At this component's own boundary a not-yet-loaded read and a failed one
  // are the identical `null` prop - views/NodeDetail.tsx never forwards its
  // own queue fetch's error, only its data - so the two pins below exercise
  // the same code path deliberately, once for each named scenario the
  // review called out.
  function pendingAmendmentPage(): NodePage {
    return amendmentPage({ node: { ...amendmentPage().node, next_owner: "human" } });
  }

  it("amendment page + null (not-yet-loaded) queue: no approve box", () => {
    const { container } = withRouter(<NodeDetail page={pendingAmendmentPage()} />);
    expect(container.querySelector(".attn")).toBeNull();
    expect(screen.queryByText("Awaiting your approval")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "tiller approve amendment AM-1" }),
    ).not.toBeInTheDocument();
  });

  it("amendment page + null (failed-read) queue: no approve box", () => {
    const { container } = withRouter(<NodeDetail page={pendingAmendmentPage()} queue={null} />);
    expect(container.querySelector(".attn")).toBeNull();
    expect(screen.queryByText("Awaiting your approval")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "tiller approve amendment AM-1" }),
    ).not.toBeInTheDocument();
  });

  it("suppresses the fallback when the node already carries a real queue entry, even an excluded one", () => {
    const entry: QueueEntry = {
      kind: "attest_gate",
      noun: "epic",
      id: "E-1",
      ref: "foundation (E-1)",
      gate: "security",
      gate_status: "pass",
      command: "tiller attest epic foundation --gate security --status pass",
    };

    const { container } = withRouter(<NodeDetail page={readyEpicPage()} queue={queueOf([entry])} />);
    // attest_gate is excluded from rendering AND from triggering the
    // fallback: the queue is not silent about this node, so the vaguer
    // guess never gets a turn.
    expect(container.querySelector(".attn")).toBeNull();
  });

  // Issue #152: approve_tasks rows are keyed to the feature, so a proposed
  // task's page always reaches the fallback. A question holding the task
  // makes approve task refuse, so the box names the answer instead.
  it("names the holding question's answer on a proposed task's page, never approve task", () => {
    const page = readyEpicPage({
      node: {
        noun: "task",
        id: "T-br7z1r",
        key: "foundation/app-skeleton/boot-the-app",
        title: "Boot the app",
        status: "proposed",
        next_owner: "human",
      },
      ref: "foundation/app-skeleton/boot-the-app (T-br7z1r)",
      relationships: {
        blocked_by_questions: [
          { noun: "question", id: "Q-aaaaaa", ref: "Q-aaaaaa", status: "open", title: "Canonical schema version" },
        ],
      },
    });
    const { container } = withRouter(<NodeDetail page={page} queue={queueOf([])} />);

    const box = container.querySelector(".abox") as HTMLElement;
    expect(box).not.toBeNull();
    expect(within(box).getByText("Awaiting your answer · Canonical schema version (Q-aaaaaa)")).toBeInTheDocument();
    expect(
      within(box).getByRole("button", { name: 'tiller answer question Q-aaaaaa --answer "..."' }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Awaiting your approval")).not.toBeInTheDocument();
  });

  it("renders no fallback box when next_owner is not human", () => {
    const page = readyEpicPage({
      node: { ...readyEpicPage().node, next_owner: "agent" },
    });
    const { container } = withRouter(<NodeDetail page={page} />);
    expect(container.querySelector(".attn")).toBeNull();
  });
});

// Fix wave (task-17-review, minor): a structural pin that the header and the
// attention stack sit OUTSIDE the tab row entirely - rendering exactly once
// no matter which tab is active, never inside a tab panel that would make
// either one disappear or duplicate on a switch (ui-surface.md: "the header
// and its chips, the attention stack beneath it, and the drift panel ...
// render exactly once no matter which of the body's own tabs is open").
describe("NodeDetail: header and attention stack render once, outside the tabs", () => {
  it("keeps exactly one header and one attention box across a tab switch", () => {
    const page = featurePage({
      relationships: {
        tasks: [{ noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Some task" }],
      },
    });
    const { container } = withRouter(<NodeDetail page={page} queue={queueOf([queueEntry()])} />);

    expect(container.querySelectorAll(".instr-header")).toHaveLength(1);
    expect(container.querySelectorAll(".abox")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Tasks · 1" }));

    expect(container.querySelectorAll(".instr-header")).toHaveLength(1);
    expect(container.querySelectorAll(".abox")).toHaveLength(1);
    // The header's own title survives the switch, proof it never remounted
    // inside the panel that just changed.
    expect(
      screen.getByRole("heading", { name: "App Skeleton & Green CI Boot" }),
    ).toBeInTheDocument();
  });
});

describe("NodeDetail: eval contract error state", () => {
  it("renders the served {path, error} verbatim rather than hiding a malformed contract", () => {
    const page = featurePage({
      eval: { path: "features/x/feature.eval.md", error: "criterion C1 has no statement" },
    });

    withRouter(<NodeDetail page={page} />);

    // task-1-brief.md's tab shell: the eval warning lives on the evals tab,
    // whose DISPLAY label reads "Contract" now (task 17's own decision; the
    // wire key and `tab` param stay "evals"). A malformed contract is not
    // null, so the tab still renders (never hide a malformed contract).
    fireEvent.click(screen.getByRole("button", { name: /^Contract/ }));

    expect(screen.getByText("features/x/feature.eval.md")).toBeInTheDocument();
    expect(screen.getByText("criterion C1 has no statement")).toBeInTheDocument();
  });
});

describe("NodeDetail: feature tasks render in ledger (payload) order", () => {
  it("never re-sorts the served task list", () => {
    const page = featurePage({
      relationships: {
        tasks: [
          { noun: "task", id: "T-zkg3wy", ref: "T-zkg3wy", status: "ready", title: "Wire Redis and Sidekiq" },
          { noun: "task", id: "T-br7z1r", ref: "T-br7z1r", status: "ready", title: "Initialize the Rails app" },
        ],
      },
    });

    withRouter(<NodeDetail page={page} />);

    // task-1-brief.md's tab shell: the task list lives on the tasks tab.
    fireEvent.click(screen.getByRole("button", { name: /^Tasks/ }));

    const first = screen.getByText("Wire Redis and Sidekiq");
    const second = screen.getByText("Initialize the Rails app");
    expect(Boolean(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
  });
});

describe("NodeDetail epic: identity renders once, decomposition ordered as served", () => {
  function epicFeature(overrides: Partial<NodeView>): NodeView {
    return { noun: "feature", id: "feature-x", status: "draft", ...overrides };
  }

  const zetaFeature = epicFeature({
    id: "F-zeta",
    key: "foundation/zeta",
    title: "Zeta feature",
    status: "specced",
  });
  const alphaFeature = epicFeature({
    id: "F-alpha",
    key: "foundation/alpha",
    title: "Alpha feature",
    status: "ready",
  });

  function epicPage(): NodePage {
    return {
      node: {
        noun: "epic",
        id: "E-u2w9wv",
        key: "foundation",
        title: "Foundation",
        status: "ready",
      },
      ref: "foundation (E-u2w9wv)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      decomposition: { features: [zetaFeature, alphaFeature] },
    };
  }

  it("renders the epic's title exactly once, in the header, never repeated as a decomposition summary", () => {
    withRouter(<NodeDetail page={epicPage()} />);
    expect(screen.getAllByText("Foundation")).toHaveLength(1);
  });

  it("renders the decomposition in served order with a mixed-child StateBar", () => {
    const { container } = withRouter(<NodeDetail page={epicPage()} />);

    // task-1-brief.md's tab shell: the decomposition lives on the features
    // tab, not the default spec one.
    fireEvent.click(screen.getByRole("button", { name: /^Features/ }));

    const zeta = screen.getByText("Zeta feature");
    const alpha = screen.getByText("Alpha feature");
    expect(Boolean(zeta.compareDocumentPosition(alpha) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
    expect(container.querySelector(".instr-decomp-bar .state-bar")).not.toBeNull();
    expect(screen.getByText("1 specced · 1 ready")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The header's parent chain (task 2, node-detail-tabs): a feature's epic, a
// task's feature then epic, read straight from page.relationships and shown
// as a ref link with its status chip - the retired bridge's Epic/Feature hub
// groups and TaskFeaturePanel's parent-feature climb, both now here instead.
// ---------------------------------------------------------------------------

// The old "parent chain" (a plain feature-then-epic list, node-detail-tabs'
// own task 2) is rebuilt task 17 as a true breadcrumb: ancestor order, epic
// leading a feature, epic then feature leading a task - the flip from
// feature-then-epic is deliberate (ui-surface.md's amended uniform-shell
// bullet: "an epic crumb above a feature, epic then feature above a task"),
// matching how a reader actually reads a breadcrumb, coarse to fine.
describe("NodeDetail header: the breadcrumb chain", () => {
  it("shows a feature's epic as a crumb, titled, with its own status chip", () => {
    const page = featurePage({
      relationships: {
        epic: {
          noun: "epic",
          id: "E-1",
          ref: "billing (E-1)",
          status: "specced",
          title: "Billing overhaul",
        },
      },
    });
    const { container } = withRouter(<NodeDetail page={page} />);

    const crumbs = within(container.querySelector(".crumbs") as HTMLElement);
    // A crumb leads with the node's TITLE, not its mono ref - the console's
    // reference-convention carve-out every other rendered row already takes.
    const link = crumbs.getByRole("link", { name: "Billing overhaul" });
    expect(link).toHaveAttribute("href", "/node/epic/E-1");
    expect(crumbs.getByText("specced")).toBeInTheDocument();
  });

  // The phase acceptance line, verbatim: "A task page shows the full
  // three-crumb chain" - epic crumb, feature crumb, each with its own status
  // chip, ancestor order, above the task's own title and chip.
  it("shows a task's full three-crumb chain: epic then feature, above the task's own title", () => {
    const page = taskPage({
      relationships: {
        feature: {
          noun: "feature",
          id: "F-1",
          ref: "billing/refunds (F-1)",
          status: "ready",
          title: "Refund pipeline",
        },
        epic: {
          noun: "epic",
          id: "E-1",
          ref: "billing (E-1)",
          status: "draft",
          title: "Billing overhaul",
        },
      },
    });
    const { container } = withRouter(<NodeDetail page={page} />);

    const crumbs = within(container.querySelector(".crumbs") as HTMLElement);
    const epic = crumbs.getByRole("link", { name: "Billing overhaul" });
    const feature = crumbs.getByRole("link", { name: "Refund pipeline" });
    expect(Boolean(epic.compareDocumentPosition(feature) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
    expect(crumbs.getByText("draft")).toBeInTheDocument();
    expect(crumbs.getByText("ready")).toBeInTheDocument();
    // The chain's third link: the task's own title and chip, in the header
    // beneath the crumbs.
    const header = within(container.querySelector(".instr-header") as HTMLElement);
    expect(
      header.getByRole("heading", { name: "Boot the app on its datastores" }),
    ).toBeInTheDocument();
  });

  it("states a dangling parent with a quiet note when a task's own layout link does not resolve", () => {
    // taskPage()'s default relationships is {}, the same "layout link did not
    // resolve" shape internal/views/nodepage.go's relationshipsOf produces
    // when a task's FeatureID does not resolve in the tree.
    const { container } = withRouter(<NodeDetail page={taskPage()} />);

    const crumbs = within(container.querySelector(".crumbs") as HTMLElement);
    expect(crumbs.getByText("Parent feature not found")).toBeInTheDocument();
  });

  it("shows no crumb chain and no empty gap on an epic page (an epic has no parent)", () => {
    const page: NodePage = {
      node: { noun: "epic", id: "E-1", key: "billing", title: "Billing overhaul", status: "ready" },
      ref: "billing (E-1)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const { container } = withRouter(<NodeDetail page={page} />);
    expect(container.querySelector(".crumbs")).toBeNull();
  });

  it("shows no crumb chain on a question page", () => {
    const page: NodePage = {
      node: { noun: "question", id: "Q-1", status: "open", statement: "Five or six?" },
      ref: "Q-1",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const { container } = withRouter(<NodeDetail page={page} />);
    expect(container.querySelector(".crumbs")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// A source renders on exactly ONE surface (task 2 dropped the body's own
// Source section entirely). The node header pass (2026-08-30) moved that
// surface from the header's caption line to the Spec section's own head,
// beside the eyebrow, as a one-hop link to the console's file page rather
// than the earlier link into Explore's search over the path. The header
// itself no longer names the source on any prose-bearing noun; the
// question page, which has no Spec section, is the one carve-out
// (nodeheader.test.tsx).
// ---------------------------------------------------------------------------

describe("NodeDetail: source is the Spec head's one link", () => {
  it("links the source beside the Spec head, one hop to the file page, and never in the header", () => {
    const page = featurePage({
      node: featureNode({ source: "design/solution-design/09-audit-engine.md" }),
      prose: {
        path: ".tiller/epics/foundation/features/app-skeleton-ci-boot/feature.md",
        content: "# App Skeleton & Green CI Boot\n\nBoot the skeleton.\n",
      },
    });
    const { container } = withRouter(<NodeDetail page={page} />);

    const header = within(container.querySelector(".instr-header") as HTMLElement);
    expect(header.queryByText("design/solution-design/09-audit-engine.md")).toBeNull();

    const head = within(container.querySelector(".instr-fh-row") as HTMLElement);
    const link = head.getByRole("link", { name: "design/solution-design/09-audit-engine.md" });
    expect(link).toHaveAttribute("href", "/file/design/solution-design/09-audit-engine.md#L1");
    // instr-source-link is the quiet-link treatment that keeps it a link to
    // the eye under the global `a { text-decoration: none }` reset.
    expect(link).toHaveClass("instr-source-link");
  });

  it("renders no source link, and no 'from', when the node carries no source", () => {
    const { container } = withRouter(<NodeDetail page={featurePage()} />);

    expect(screen.queryByText(/^from/)).not.toBeInTheDocument();
    expect(container.querySelector(".instr-source-link")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SOURCE no longer renders anywhere in the body: the header's own link is the
// only surface now, on every noun and every tab.
// ---------------------------------------------------------------------------

describe("NodeDetail: Source is absent from the body", () => {
  it("never renders a Source heading for a tabbed noun, on any tab, even with a source and dependencies present", () => {
    const page = featurePage({
      node: featureNode({ source: "design/refunds.md" }),
      relationships: {
        tasks: [{ noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Wire the refund job" }],
        depends_on: [{ noun: "feature", id: "F-2", ref: "F-2", status: "ready", title: "Ledger" }],
      },
    });
    withRouter(<NodeDetail page={page} />);

    expect(screen.queryByRole("heading", { name: "Source" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Dependencies/ }));
    expect(screen.queryByRole("heading", { name: "Source" })).not.toBeInTheDocument();
  });

  it("never renders a Source heading on the flat question page either", () => {
    const page = featurePage({
      node: { noun: "question", id: "Q-1", status: "open", statement: "Five or six?", source: "design/q.md" },
    });
    withRouter(<NodeDetail page={page} />);

    expect(screen.queryByRole("heading", { name: "Source" })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The header's caption lines. These carry the facts the epic page's feature
// rows stopped carrying when they came down to one line per node, which is
// what makes that cut lossless: change type and source were already here, and
// the surface axes moved here from those rows.
// ---------------------------------------------------------------------------

// DELETED (task 17, console v2 node page rebuild): "names the node's type
// above the title, so same-named nodes read apart" and "names every noun the
// same way, a question included," both asserting .instr-noun's text. The
// amended header bullet (ui-surface.md, 2026-08-10) no longer names a noun
// eyebrow at all, and the mock draws none either (docs/plans/2026-08-09-
// console-v2-mocks.html's node pane, no eyebrow element above .crumbs). No
// direct replacement: the breadcrumb-chain tests above and the refline test
// below cover the "tell same-titled nodes apart" job the eyebrow used to do
// alone - the crumb chain names ancestry and the refline's own canonical key
// path is a second, address-level disambiguator the eyebrow never was.
describe("NodeDetail header facts", () => {
  it("renders a feature's surface paths on its own page", () => {
    const { container } = withRouter(
      <NodeDetail
        page={featurePage({
          node: featureNode({
            change_type: "modify",
            source: "design/solution-design/09-audit-engine.md",
            surface: { paths: ["app/refunds.rb", "app/models/refund.rb"] },
          }),
        })}
      />,
    );

    // Scoped to the header, where the facts block lives.
    const header = within(container.querySelector(".instr-header") as HTMLElement);
    expect(header.getByText("app/refunds.rb, app/models/refund.rb")).toBeInTheDocument();
    // The change type, labeled beside it in the facts block; the source is
    // the Spec head's now (node header pass, 2026-08-30), not the header's.
    expect(header.getByText("modify")).toBeInTheDocument();
    expect(header.getByText("Change")).toBeInTheDocument();
    expect(header.getByText("Touches")).toBeInTheDocument();
  });

  it("draws no Touches fact for a node that declares no surface", () => {
    withRouter(<NodeDetail page={featurePage()} />);
    expect(screen.queryByText("Touches")).toBeNull();
  });

  it("copies the node's id when the header ref is clicked (JC's ask, 2026-07-30)", () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    withRouter(<NodeDetail page={featurePage()} />);

    fireEvent.click(
      screen.getByRole("button", { name: "foundation/app-skeleton-ci-boot (F-pueftm)" }),
    );
    expect(writeText).toHaveBeenCalledWith("F-pueftm");
  });


  it("renders one compact chip per recorded gate: pass/pend from the gate's own derived state, evals trailing with its blocking criterion named", () => {
    const { container } = withRouter(
      <NodeDetail
        page={featurePage({
          node: featureNode({
            gates: {
              build: { required: true, status: "pass" },
              tests: { required: true, status: "pass" },
              evals: {
                required: true,
                status: "pending",
                criteria: { C3: { status: "needs_attestation" } },
              },
            },
          }),
          eval: {
            path: "features/x/feature.eval.md",
            node: "F-pueftm",
            criteria: [criterion({ id: "C3", binding: { type: "eval" } })],
          },
        })}
      />,
    );

    const chips = container.querySelectorAll(".gate-chip");
    expect(chips).toHaveLength(3);
    // build, tests (alphabetical), evals always trailing.
    expect(chips[0].textContent).toContain("build");
    expect(chips[1].textContent).toContain("tests");
    expect(chips[2].textContent).toContain("evals");
    expect(chips[0].querySelector(".g")).toHaveClass("pass");
    expect(chips[1].querySelector(".g")).toHaveClass("pass");
    expect(chips[2].querySelector(".g")).toHaveClass("pend");
    // The evals chip's own qualifier: the criterion currently blocking it.
    expect(chips[2].textContent).toContain("C3");
  });
});

describe("NodeDetail shelf relationship metadata", () => {
  it("links a shelf node's found-in and carried-by metadata", () => {
    withRouter(<NodeDetail page={bugPageWithRelationships} />);

    expect(screen.getByText("Found in")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Render the board" })).toHaveAttribute(
      "href",
      "/node/task/T-render",
    );
    expect(screen.getByText("Carried by")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Console" })).toHaveAttribute(
      "href",
      "/node/feature/F-console",
    );
  });

  it("keeps a duplicate-only Dependencies tab reachable without a false count", () => {
    withRouter(
      <NodeDetail
        page={bugPage({
          relationships: {
            duplicate_of: {
              noun: "bug",
              id: "B-aaaaaa",
              ref: "B-aaaaaa",
              status: "open",
              title: "Blank on reload",
            },
          },
        })}
      />,
    );

    const tab = screen.getByRole("button", { name: "Dependencies" });
    expect(screen.queryByRole("button", { name: "Dependencies · 1" })).not.toBeInTheDocument();
    fireEvent.click(tab);
    expect(screen.getByRole("link", { name: "Blank on reload" })).toHaveAttribute(
      "href",
      "/node/bug/B-aaaaaa",
    );
  });

  it("does not add a duplicate target to the direct dependency count", () => {
    withRouter(
      <NodeDetail
        page={bugPage({
          relationships: {
            depends_on: [
              {
                noun: "task",
                id: "T-prerequisite",
                ref: "T-prerequisite",
                status: "ready",
                title: "Prepare the fixture",
              },
            ],
            duplicate_of: {
              noun: "bug",
              id: "B-aaaaaa",
              ref: "B-aaaaaa",
              status: "open",
              title: "Blank on reload",
            },
          },
        })}
      />,
    );

    expect(screen.getByRole("button", { name: "Dependencies · 1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Dependencies · 2" })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The task page. A task is a folder node with an implementation spec of its own
// now (artifact-schema.md, Tasks), and ui-surface.md's node detail says a task
// shows that spec, then its gates and evidence, then its PR: a page, not a
// ledger row blown up.
// ---------------------------------------------------------------------------

function taskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "task",
      id: "T-h4k2wq",
      key: "foundation/app-skeleton-ci-boot/boot-the-app",
      title: "Boot the app on its datastores",
      status: "in_progress",
    },
    ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-h4k2wq)",
    prose: {
      path: ".tiller/epics/foundation/features/app-skeleton-ci-boot/tasks/boot-the-app/task.md",
      content:
        "# Boot the app\n\n## Approach\n\nBring up Postgres and Redis from compose, then serve /up.\n",
    },
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

describe("NodeDetail task body", () => {
  it("renders the implementation spec through the prose section, above the gates", () => {
    const { container } = withRouter(<NodeDetail page={taskPage()} />);

    expect(screen.getByText("Implementation spec")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Approach" })).toBeInTheDocument();
    expect(screen.getByText(/Bring up Postgres and Redis/)).toBeInTheDocument();
    // The path line under the head is gone (node header pass, 2026-08-30):
    // the header's ref line addresses the node, so the prose simply begins.
    expect(
      screen.queryByText(
        ".tiller/epics/foundation/features/app-skeleton-ci-boot/tasks/boot-the-app/task.md",
      ),
    ).toBeNull();
    // The spec leads the body: everything else on the page follows it.
    const section = container.querySelector(".instr-section") as HTMLElement;
    expect(within(section).getByText("Implementation spec")).toBeInTheDocument();
  });

  it("collapses an unwritten task spec to one line rather than typesetting the stub", () => {
    const page = taskPage();
    const { container } = withRouter(
      <NodeDetail page={{ ...page, prose: { ...page.prose!, stub: true } }} />,
    );
    expect(screen.getByText("No implementation spec written yet")).toBeInTheDocument();
    expect(container.querySelector(".prose")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// The active-basis report (cli-surface.md, "Specifying a feature": status,
// serve, and drift REPORT it while start, check, and submit refuse it).
// ---------------------------------------------------------------------------

describe("NodeDetail active-basis drift", () => {
  it("names the drifted files and the move, in the quiet register", () => {
    const { container } = withRouter(
      <NodeDetail
        page={taskPage({
          active_basis: {
            drifted: ["task.md"],
            noun: "task",
            id: "T-h4k2wq",
            ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-h4k2wq)",
            command:
              "tiller approve task foundation/app-skeleton-ci-boot/boot-the-app --refresh-basis",
          },
        })}
      />,
    );

    expect(screen.getByText("active_basis")).toBeInTheDocument();
    expect(screen.getByText("task.md")).toBeInTheDocument();
    expect(screen.getByText(/no longer matches the accepted basis/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "tiller approve task foundation/app-skeleton-ci-boot/boot-the-app --refresh-basis",
      }),
    ).toBeInTheDocument();
    // Drift is not an attention-stack item (it renders through its own
    // disclosure panel, not a box): the loud ink stays where it belongs, and
    // an active_basis report alone (no queue entry, no unpushed_review)
    // raises no attention stack at all.
    expect(container.querySelector(".attn")).toBeNull();
    expect(container.querySelectorAll(".copy-cmd-accent")).toHaveLength(0);
  });

  it("names the grain that owns the pin when it is not this page's node", () => {
    // A task's guard compares its parent feature's pins before its own, so the
    // report can be the feature's. Refreshing the task's own pin would not
    // clear it, which is why the line has to say whose pin moved.
    withRouter(
      <NodeDetail
        page={taskPage({
          active_basis: {
            drifted: ["feature.md", "feature.eval.md"],
            noun: "feature",
            id: "F-pueftm",
            ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
            command: "tiller approve feature foundation/app-skeleton-ci-boot --refresh-basis",
          },
        })}
      />,
    );

    expect(screen.getByText("feature.md, feature.eval.md")).toBeInTheDocument();
    expect(screen.getByText(/no longer match the accepted basis/)).toBeInTheDocument();
    expect(screen.getByText("foundation/app-skeleton-ci-boot (F-pueftm)")).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "tiller approve feature foundation/app-skeleton-ci-boot --refresh-basis",
      }),
    ).toBeInTheDocument();
  });

  it("says restore first when a pin went missing, since refresh cannot hash a file that is gone", () => {
    withRouter(
      <NodeDetail
        page={taskPage({
          active_basis: {
            drifted: ["task.md"],
            missing: ["task.md"],
            noun: "task",
            id: "T-h4k2wq",
            ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-h4k2wq)",
            command:
              "tiller approve task foundation/app-skeleton-ci-boot/boot-the-app --refresh-basis",
          },
        })}
      />,
    );
    expect(screen.getByText(/restore/)).toBeInTheDocument();
  });

  it("draws nothing at all when no pin drifted", () => {
    const { container } = withRouter(<NodeDetail page={taskPage()} />);
    expect(container.querySelector(".instr-drift")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// The Drifted chip (task-12-brief.md; ui-surface.md's node-page chip
// sentence): one chip in the header beside the state, regardless of how many
// markers touch the node, whose palette register states whether any of them
// actually refuses a verb right now. The register itself (which class/token
// a refusing vs. non-refusing badge paints) is Chip's own existing total
// statusColor mapping (lib/states.ts) - this suite passes "blocked" and
// "draft" straight through, the identical two registers GateCard's derived
// state already rides (deriveFlatGateState's stale case is "draft", the
// exact neutral register the brief's resolution 3 names), never a new token.
// ---------------------------------------------------------------------------

describe("NodeDetail: the Drifted chip", () => {
  it("renders the blocked register when any badge refuses, the disclosure button titling every marker", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          drift: [
            { marker: "stale_spec", refusing: false },
            { marker: "active_basis", refusing: true },
          ],
        })}
      />,
    );

    const chip = screen.getByText("Drifted");
    expect(chip).toHaveClass("chip");
    expect(chip.getAttribute("data-status")).toBe("blocked");
    // The chip is a disclosure now (JC, 2026-08-06): the button owns the
    // click, the tooltip, and aria-expanded; the Chip span inside stays
    // presentational.
    const button = chip.closest("button");
    expect(button).not.toBeNull();
    expect(button).toHaveAttribute("title", "stale_spec, active_basis");
    expect(button).toHaveAttribute("aria-expanded", "true");
  });

  it("renders the neutral (draft) register the stale gates use when no badge refuses", () => {
    withRouter(
      <NodeDetail page={featurePage({ drift: [{ marker: "stale_spec", refusing: false }] })} />,
    );

    const chip = screen.getByText("Drifted");
    expect(chip.getAttribute("data-status")).toBe("draft");
    expect(chip.closest("button")).toHaveAttribute("title", "stale_spec");
    expect(chip.closest("button")).toHaveAttribute("aria-expanded", "false");
  });

  it("renders no chip at all with no drift", () => {
    withRouter(<NodeDetail page={featurePage()} />);
    expect(screen.queryByText("Drifted")).not.toBeInTheDocument();
  });

  it("renders one chip regardless of how many markers touch the node", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          drift: [
            { marker: "stale_spec", refusing: false },
            { marker: "active_basis", refusing: true },
            { marker: "orphan_eval", refusing: false },
          ],
        })}
      />,
    );
    expect(screen.getAllByText("Drifted")).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// The drift panel behind the chip (JC's ask, 2026-08-06): the chip is a
// disclosure, refusing drift pre-expanded, and the panel renders each
// finding at candidate grain with its clearing move where one exists.
// ---------------------------------------------------------------------------

describe("NodeDetail: the drift panel", () => {
  const neutralBadges = [{ marker: "answered_question", refusing: false }];
  const detail = {
    candidates: [
      {
        marker: "answered_question",
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        detail: "Q-1 (Is PDF export in scope?) was answered and has not been reviewed in",
        remedy: "tiller approve feature foundation/app-skeleton-ci-boot --reviewed-assumption Q-1",
      },
    ],
  };

  it("stays collapsed on a neutral chip and opens on click, remedy beside the finding", () => {
    withRouter(
      <NodeDetail page={featurePage({ drift: neutralBadges, drift_detail: detail })} />,
    );

    expect(screen.queryByText(/was answered and has not been reviewed/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Drifted").closest("button") as HTMLElement);

    expect(screen.getByText(/was answered and has not been reviewed/)).toBeInTheDocument();
    expect(
      screen.getByText("tiller approve feature foundation/app-skeleton-ci-boot --reviewed-assumption Q-1"),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /project-wide drift report/ })).toHaveAttribute(
      "href",
      "/drift",
    );

    fireEvent.click(screen.getByText("Drifted").closest("button") as HTMLElement);
    expect(screen.queryByText(/was answered and has not been reviewed/)).not.toBeInTheDocument();
  });

  it("opens by default when a badge refuses, and renders no command where no remedy exists", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          drift: [{ marker: "orphan_eval", refusing: true }],
          drift_detail: {
            candidates: [
              {
                marker: "orphan_eval",
                noun: "feature",
                id: "F-pueftm",
                key: "foundation/app-skeleton-ci-boot",
                detail: "spec/gone_spec.rb (C2) no longer exists",
                files: ["spec/gone_spec.rb"],
              },
            ],
          },
        })}
      />,
    );

    expect(screen.getByText("spec/gone_spec.rb (C2) no longer exists")).toBeInTheDocument();
    expect(document.querySelector(".instr-drift-panel .copy-cmd")).toBeNull();
  });

  it("renders uncovered rows muted and leads an ancestor finding with its ref", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          drift: neutralBadges,
          drift_detail: {
            candidates: [
              {
                marker: "answered_question",
                noun: "feature",
                id: "F-parent",
                key: "foundation/parent",
                detail: "Q-2 (Which currency?) was answered and has not been reviewed in",
              },
            ],
            uncovered: [
              {
                marker: "orphan_eval",
                noun: "feature",
                id: "F-pueftm",
                key: "foundation/app-skeleton-ci-boot",
                reason: "ref_unreadable",
                detail: "\"a; b\" is not a single path (criterion C5)",
              },
            ],
          },
        })}
      />,
    );

    fireEvent.click(screen.getByText("Drifted").closest("button") as HTMLElement);

    // The finding belongs to an ancestor, so its row leads with that ref.
    expect(screen.getByRole("link", { name: "foundation/parent (F-parent)" })).toBeInTheDocument();
    // The v0.2.10 not-a-single-path rows land here, as facts, never as drift.
    expect(screen.getByText(/not checked \(ref_unreadable\)/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The Held chip (task-3-brief.md; issue #36's "annotate, don't subtract"):
// one chip in the header naming the nearest retired ancestor, present only
// when node.held carries one. It earns the blocked palette register the same
// way the Drifted chip's refusing badges do (nodepage.go:114-139's rule:
// "the chip takes the blocked register only where the condition actually
// refuses verbs"), because under a retired ancestor the doing verbs do
// refuse (mutate.guardParentState).
// ---------------------------------------------------------------------------

describe("NodeDetail: the Held chip", () => {
  it("renders naming the retired ancestor, in the blocked register", () => {
    withRouter(
      <NodeDetail
        page={taskPage({
          node: {
            noun: "task",
            id: "T-h4k2wq",
            key: "foundation/app-skeleton-ci-boot/boot-the-app",
            title: "Boot the app on its datastores",
            status: "in_progress",
            held: {
              noun: "feature",
              id: "F-pueftm",
              ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
              state: "cancelled",
            },
          },
        })}
      />,
    );

    const chip = screen.getByText("Held");
    expect(chip).toHaveClass("chip");
    expect(chip.getAttribute("data-status")).toBe("blocked");
    expect(chip).toHaveAttribute(
      "title",
      "feature foundation/app-skeleton-ci-boot (F-pueftm) is cancelled",
    );
  });

  it("renders no chip at all when the node carries no held ancestor", () => {
    withRouter(<NodeDetail page={taskPage()} />);
    expect(screen.queryByText("Held")).not.toBeInTheDocument();
  });
});

// Task 8 of the Review Grain build (docs/plans/2026-08-26-review-grain-design.md,
// "Surfaces"): a quiet chip-style badge naming the resolved review grain in
// force for work under an epic or a feature, rendered only when it differs
// from task, the value every tree carries by default and needs no badge to
// explain. It borrows Chip's quiet "draft" register (the same neutral bucket
// DriftedChip's own non-refusing badge already uses), never the accent: a
// coarser grain is a calibration, not a caution.
describe("NodeDetail: the review grain badge", () => {
  it("renders on a feature page when the resolved grain is feature", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          node: featureNode({ review_grain: "feature", review_grain_source: "feature" }),
        })}
      />,
    );

    const chip = screen.getByText("review grain: feature");
    expect(chip).toHaveClass("chip");
    expect(chip.getAttribute("data-status")).toBe("draft");
  });

  it("renders on an epic page when the resolved grain is epic", () => {
    withRouter(
      <NodeDetail
        page={{
          node: {
            noun: "epic",
            id: "E-1",
            key: "form-engine",
            title: "Form Engine",
            status: "ready",
            review_grain: "epic",
            review_grain_source: "epic",
          },
          ref: "form-engine (E-1)",
          prose: null,
          relationships: {},
          backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
          history: [],
          eval: null,
        }}
      />,
    );

    expect(screen.getByText("review grain: epic")).toBeInTheDocument();
  });

  it("renders nothing when the resolved grain is task, even with a source recorded", () => {
    withRouter(
      <NodeDetail
        page={featurePage({
          node: featureNode({ review_grain: "task", review_grain_source: "project" }),
        })}
      />,
    );

    expect(screen.queryByText(/review grain:/)).not.toBeInTheDocument();
  });

  it("renders nothing when the node carries no review_grain at all", () => {
    withRouter(<NodeDetail page={featurePage()} />);
    expect(screen.queryByText(/review grain:/)).not.toBeInTheDocument();
  });

  it("never renders on a task page, even if the payload carried a coarser grain", () => {
    withRouter(
      <NodeDetail
        page={taskPage({
          node: {
            ...taskPage().node,
            review_grain: "feature",
            review_grain_source: "feature",
          },
        })}
      />,
    );

    expect(screen.queryByText(/review grain:/)).not.toBeInTheDocument();
  });
});

// The routed node page owns the queue read behind the attention stack. A
// queue snapshot taken while node A was open must not drive node B's asks:
// after a CLI mutation the ask may already be gone.
describe("NodeDetail view: the attention queue follows the node", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const pageA = featurePage({ node: featureNode({ id: "F-A", key: "alpha", title: "Alpha" }), ref: "alpha (F-A)" });
  const pageB = featurePage({ node: featureNode({ id: "F-B", key: "beta", title: "Beta" }), ref: "beta (F-B)" });
  const staleApproval = queueEntry({
    kind: "approve_feature",
    id: "F-B",
    ref: "beta (F-B)",
    prose: "Beta",
    command: "tiller approve feature beta",
  });

  type Pending = { resolve: (body: unknown) => void };

  // Node pages answer at once; each queue read waits for the test to settle it.
  function stubQueueReads(): Pending[] {
    const queueReads: Pending[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const url = String(input);
        const ok = (body: unknown) => ({ ok: true, json: () => Promise.resolve(body) });
        if (url === "/api/node/feature/F-A") return Promise.resolve(ok(pageA));
        if (url === "/api/node/feature/F-B") return Promise.resolve(ok(pageB));
        if (url === "/api/queue") {
          return new Promise((resolve) => queueReads.push({ resolve: (body) => resolve(ok(body)) }));
        }
        return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
      }),
    );
    return queueReads;
  }

  function GoToB() {
    const navigate = useNavigate();
    return <button onClick={() => navigate("/node/feature/F-B")}>go to B</button>;
  }

  function renderAt(path: string) {
    return render(
      <MemoryRouter initialEntries={[path]}>
        <GoToB />
        <Routes>
          <Route path="/node/:noun/*" element={<NodeDetailView />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("rereads the queue on navigation, so B never shows an ask from A's snapshot", async () => {
    const queueReads = stubQueueReads();
    renderAt("/node/feature/F-A");
    await screen.findByRole("heading", { name: /Alpha/ });
    await act(async () => queueReads[0].resolve(queueOf([staleApproval])));

    fireEvent.click(screen.getByRole("button", { name: "go to B" }));
    await screen.findByRole("heading", { name: /Beta/ });
    expect(screen.queryByText("tiller approve feature beta")).not.toBeInTheDocument();

    await waitFor(() => expect(queueReads).toHaveLength(2));
    await act(async () => queueReads[1].resolve(queueOf([])));
    expect(screen.queryByText("tiller approve feature beta")).not.toBeInTheDocument();
  });

  it("ignores a late queue response for A once B is open", async () => {
    const queueReads = stubQueueReads();
    renderAt("/node/feature/F-A");
    await screen.findByRole("heading", { name: /Alpha/ });

    fireEvent.click(screen.getByRole("button", { name: "go to B" }));
    await screen.findByRole("heading", { name: /Beta/ });
    await waitFor(() => expect(queueReads).toHaveLength(2));
    await act(async () => queueReads[1].resolve(queueOf([])));
    await act(async () => queueReads[0].resolve(queueOf([staleApproval])));

    expect(screen.queryByText("tiller approve feature beta")).not.toBeInTheDocument();
  });

  it("shows B's fresh ask once B's own queue read lands", async () => {
    const queueReads = stubQueueReads();
    renderAt("/node/feature/F-B");
    await screen.findByRole("heading", { name: /Beta/ });
    await act(async () => queueReads[0].resolve(queueOf([staleApproval])));

    expect(await screen.findByText("tiller approve feature beta")).toBeInTheDocument();
  });
});

