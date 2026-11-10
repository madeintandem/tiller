// Tokens as minted never reach the screen: each row here used to print a
// stored status, queue kind, event noun, amendment operation, facet, binding
// type, or full evidence reference as it was recorded. Each now goes through
// the shared label for it.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { AmendmentRelation, BacklinksView, BuildEpic, BuildFeature, EvalCriterionView, QueueEntry, RelationshipSource } from "../../api";
import { eventPhrase } from "../../lib/eventPhrase";
import { tokenWords } from "../../lib/statusLabel";
import AmendmentRelationsPanel from "../../views/node/AmendmentRelationsPanel";
import BacklinksPanel from "../../views/node/BacklinksPanel";
import FeatureCard from "../../views/build/FeatureCard";
import CriterionRow from "../CriterionRow";
import DecisionBand from "../DecisionBand";
import DecisionCard, { kindLabel } from "../DecisionCard";
import IntegrityNotes from "../IntegrityNotes";
import SourceCitation from "../relationships/SourceCitation";

afterEach(() => {
  cleanup();
});

const NO_TOKEN = /\b[a-z]+_[a-z_]+\b/;

function inRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("the shared word helpers", () => {
  it("words any token lower-case, splitting on _ . and -", () => {
    expect(tokenWords("work_item")).toBe("work item");
    expect(tokenWords("move-feature.parent")).toBe("move feature parent");
  });

  it("humanizes an unseen two-word event noun as well as its verb", () => {
    expect(eventPhrase("add.work_item", 1)).toEqual({ subject: "work item added", verb: "added" });
    expect(eventPhrase("add.work_item", 2).subject).toBe("work items added");
    expect(eventPhrase("tally_up.work_item", 1).subject).toBe("work item tally up");
  });

  it("words an unknown queue kind instead of printing it", () => {
    expect(kindLabel("approve_feature")).toBe("Approve · feature plan");
    expect(kindLabel("reconcile_ledger")).toBe("Reconcile ledger");
  });
});

const entry = (over: Partial<QueueEntry>): QueueEntry =>
  ({ kind: "attest_gate", noun: "feature", id: "F-1", ref: "billing/refunds (F-1)", title: "Refunds", actor: "human", command: "tiller attest", ...over }) as QueueEntry;

describe("decision cards and bands", () => {
  it("says a recorded gate status in words, lower-case mid-sentence", () => {
    const { container } = inRouter(<DecisionCard entry={entry({ gate: "security", gate_status: "needs_attestation" } as Partial<QueueEntry>)} />);
    expect(container.textContent).toContain("recorded needs attestation");
    expect(container.textContent).not.toMatch(NO_TOKEN);
  });

  it("words an unknown kind on the card eyebrow and the band head", () => {
    const odd = entry({ kind: "reconcile_ledger" });
    const card = inRouter(<DecisionCard entry={odd} />);
    expect(card.container.textContent).toContain("Reconcile ledger");
    expect(card.container.textContent).not.toContain("reconcile_ledger");
    cleanup();
    const band = inRouter(<DecisionBand group={{ kind: "reconcile_ledger", entries: [odd, { ...odd, id: "F-2" }] }} />);
    expect(band.container.textContent).toContain("Reconcile ledger");
    expect(band.container.textContent).not.toContain("reconcile_ledger");
  });
});

describe("IntegrityNotes", () => {
  it("words the attestation and violation statuses", () => {
    const { container } = inRouter(
      <IntegrityNotes
        attestations={[{ id: "AT-1", target_kind: "gate", target_id: "security", status: "needs_attestation", by: "jc" }]}
        violations={[{ gate: "tests", status: "not_run", observed: "abc1234" }]}
      />,
    );
    expect(screen.getByText("Needs attestation")).toBeInTheDocument();
    expect(screen.getByText("Not run")).toBeInTheDocument();
    // observed is the revision the crossing saw, an address left as recorded.
    expect(container.textContent).toContain("observed abc1234");
    expect(container.textContent).not.toMatch(NO_TOKEN);
  });
});

describe("Build's feature card", () => {
  it("words the held state in its title", () => {
    const epic = { noun: "epic", id: "E-1", key: "billing", ref: "billing (E-1)", status: "ready", contributors: [], features: [] } as unknown as BuildEpic;
    const feature = {
      id: "F-1", key: "billing/refunds", title: "Refunds", status: "ready", column: "ready", ref: "billing/refunds (F-1)",
      contributors: [], task_done: 0, task_total: 0, open_work_items: 0, tasks: [],
      held: { noun: "epic", id: "E-9", ref: "legacy (E-9)", state: "in_review" },
    } as unknown as BuildFeature;
    inRouter(<FeatureCard epic={epic} showEpic={false} feature={feature} askLabel={undefined} isDrift={false} expanded={false} onToggleExpand={() => {}} />);
    expect(screen.getByText("held")).toHaveAttribute("title", "held under legacy (E-9) (In review)");
  });
});

describe("node page panels", () => {
  it("words an amendment effect's operation the way the amendment page does", () => {
    const relations = [{
      amendment: { noun: "amendment", id: "A-1", ref: "A-1", title: "Reshape billing", status: "approved" },
      effects: [{ operation_index: 1, operation: "move-feature", role: "target", scope: "direct", node: { noun: "feature", id: "F-1", ref: "F-1", title: "Refunds" } }],
    }] as AmendmentRelation[];
    const { container } = inRouter(<AmendmentRelationsPanel relationships={{ amendments: relations }} />);
    expect(container.textContent).not.toContain("move-feature");
    expect(container.textContent).toMatch(/Feature moved/i);
    expect(container.textContent).not.toContain("—");
  });

  it("words a backlink diagnostic's facet", () => {
    const backlinks: BacklinksView = {
      groups: [], count: 0, complete: true,
      diagnostics: [{ code: "x", facet: "document_of", message: "the document could not be read" }],
    };
    const { container } = inRouter(<BacklinksPanel backlinks={backlinks} />);
    expect(container.textContent).toContain("Document of: the document could not be read");
    expect(container.textContent).not.toContain("document_of");
  });

  it("words a criterion's binding type", () => {
    const criterion = { id: "C1", statement: "Loads fast.", polarity: "introduce", required: true, binding: { type: "load_probe" } } as EvalCriterionView;
    const { container } = inRouter(<CriterionRow criterion={criterion} />);
    expect(container.textContent).toContain("load probe");
    expect(container.textContent).not.toContain("load_probe");
  });

  it("names a source's evidence by label, the full ref only on the link's href and title", () => {
    const source = { file: ".tiller/epics/billing/feature.yml", evidence: [".tiller/evidence/F-1/tests-2026-11-09.txt"] } as RelationshipSource;
    const { container } = inRouter(<ul><SourceCitation source={source} /></ul>);
    const link = container.querySelector("a[href*='/api/evidence']") as HTMLAnchorElement;
    expect(link).not.toBeNull();
    expect(link.getAttribute("title")).toBe(".tiller/evidence/F-1/tests-2026-11-09.txt");
    expect(link.getAttribute("href")).toBe(`/api/evidence?ref=${encodeURIComponent(".tiller/evidence/F-1/tests-2026-11-09.txt")}`);
    expect(container.textContent).not.toContain(".tiller/evidence/F-1/tests-2026-11-09.txt");
  });
});
