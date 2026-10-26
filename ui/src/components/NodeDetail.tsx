// NodeDetail is the acceptance instrument (mock round 4, rebuilt task 17 to
// the console v2 mock's own node pane): the reusable node page CONTENT,
// taking one already-fetched NodePage and rendering it whole - the
// breadcrumb header, the attention stack, the drift panel the Drifted chip
// discloses, and the type-aware body, its
// relationship hub, and its history timeline, tabbed per noun. Today the
// /node/:noun/* route (views/NodeDetail.tsx, which owns the fetch) is its one
// consumer; task C2's Explore reading pane mounts this exact same instrument
// from the same route component, which is why the fetch stays out of this
// component - both fetches, `page` and the attention stack's own `queue`.
// This is also where the epic identity double-render the earlier phase
// accepted as a quirk is resolved: identity renders here, in the header,
// exactly once, and an epic's decomposition panel never repeats it - whether
// this component is mounted at its own route or reused from anywhere else,
// identity shows up once, never zero, never twice.
//
// The tab row is per-noun (ui-surface.md's node detail): feature and epic place
// their optional Fix tab after their composition-child tab; task places it
// after Gates. Bugs, chores, and tweaks have their own task-shaped
// Spec/Gates/Dependencies/History factory, deliberately separate from task-only
// panels. A question and an amendment stay flat, no tab row at all, so their
// old body-then-hub-then-history order renders exactly as it always did. Every
// tabbed noun's default tab is spec, and the active one rides the `?tab=`
// search param (react-router's useSearchParams, replace rather than push: a tab
// switch is not a history entry a reader would expect the back button to
// undo, the same judgment call Explore's own `?mode=` switch made the other
// way, since a mode switch IS meant to be one). An unrecognized `tab` value
// (a stale link, a hand-edited URL) falls back to the first tab - NodeTabs'
// own fallback, not a re-check here - rather than rendering nothing. The
// evals tab's wire key and `tab` param stay "evals" (amended ui-surface.md,
// 2026-08-10); only its DISPLAY label reads "Contract" now.

import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type {
  ActiveBasisView,
  CandidateView,
  EvalPayload,
  NodePage,
  NodeView,
  Queue,
  QueueEntry,
  UnpushedReviewView,
} from "../api";
import { isEvalError } from "../api";
import { KIND_LABEL, whyLine } from "./DecisionCard";
import { needsAttentionFallback, type AttentionFallback } from "../lib/attentionFallback";
import { refOf, shortSha } from "../lib/drift";
import { has } from "../lib/has";
import CopyCmd from "./CopyCmd";
import NodeDetailHeader from "./NodeHeader";
import { ExecutionProvenance } from "./ExecutionSources";
import { ProseInline } from "./Prose";
import RecoveredReview from "./RecoveredReview";
import NodeTabs, { type NodeTab } from "./NodeTabs";
import RefLink from "./RefLink";
import HistorySection from "../views/node/HistorySection";
import RelationshipHub, { dependencyCount, type GroupKey } from "../views/node/RelationshipHub";
import AttachedWorkPanel from "../views/node/AttachedWorkPanel";
import { EpicEvalsPanel, EpicFeaturesPanel, EpicSpecPanel } from "../views/node/EpicBody";
import { FeatureEvalsPanel, FeatureSpecPanel, FeatureTasksPanel } from "../views/node/FeatureBody";
import { TaskGatesPanel, TaskSpecPanel } from "../views/node/TaskBody";
import { WorkItemGatesPanel, WorkItemSpecPanel } from "../views/node/WorkItemBody";
import QuestionBody from "../views/node/QuestionBody";
import AmendmentBody from "../views/node/AmendmentBody";
import AmendmentRelationsPanel, { ProposedAmendmentNotice, uniqueAmendments } from "../views/node/AmendmentRelationsPanel";
import BacklinksPanel from "../views/node/BacklinksPanel";
import "./instrument.css";

// Each body pulls its noun's own composition-CHILDREN edges forward (an
// epic's features, a feature's tasks, a question's blocks), so the hub - the
// flat question page's directly, every tabbed noun's through
// DependenciesPanel below - renders every other group and repeats none of
// these. A node's PARENT climb (a feature's epic, a task's feature then
// epic) is a different kind of edge entirely: it moved to the header's own
// Crumbs below (task 2, node-detail-tabs, reshaped into the breadcrumb chain
// by task 17), which reads relationships.epic / relationships.feature
// directly rather than through this map, so neither key belongs here anymore.
const claimedGroups: Record<string, GroupKey[]> = {
  epic: ["features"],
  feature: ["tasks"],
  question: ["blocks"],
};

// BasisDrift is the console's half of the active-basis rule (cli-surface.md,
// "Specifying a feature": status, serve, and drift REPORT it while start, check,
// and submit refuse it). A pin moved after the human approved it, so the work in
// flight is running against a spec nobody signed, and the human is the only one
// who can clear it.
//
// The report's own node is not always this page's: a task's guard compares its
// parent feature's pins before its own, so the line names the grain that owns
// the pin whenever it differs, since refreshing the task's own pin would not
// clear a feature's drift.
function BasisRow({ page }: { page: NodePage }) {
  const drift: ActiveBasisView | undefined = page.active_basis;
  if (drift === undefined) {
    return null;
  }
  const elsewhere = drift.id !== page.node.id;
  const missing = drift.missing ?? [];
  return (
    <div className="instr-drift">
      <span className="instr-drift-say">active_basis</span>
      <span className="mono">{drift.drifted.join(", ")}</span>
      <span>
        no longer {drift.drifted.length === 1 ? "matches" : "match"} the accepted basis
        {elsewhere && (
          <>
            {" "}
            on {drift.noun} <span className="mono">{drift.ref}</span>
          </>
        )}
      </span>
      {missing.length > 0 && (
        <span>
          restore <span className="mono">{missing.join(", ")}</span> first
        </span>
      )}
      <CopyCmd cmd={drift.command} />
    </div>
  );
}

// DriftPanel is the node-scoped drift surface the chip opens (JC's ask,
// 2026-08-06): every finding the badges summarize, at candidate grain, in
// the render-once zone above the tabs where the old standalone basis-drift
// line lived. active_basis keeps its richer BasisRow (drifted-vs-missing
// files, the restore-first caveat, the composed refresh command) and is
// filtered out of the candidate rows so one finding never renders twice;
// every other marker renders through DriftCandidateRow; uncovered rows
// follow, muted, because "the sweep could not check this" is a fact worth a
// line but never a drift claim; and the footer links the project-wide /drift
// report. The panel renders nothing at all when there is nothing to say,
// the same absent-not-empty discipline the payload keeps.
function DriftPanel({ page }: { page: NodePage }) {
  const basis = page.active_basis;
  const candidates = (page.drift_detail?.candidates ?? []).filter((c) => c.marker !== "active_basis");
  const uncovered = page.drift_detail?.uncovered ?? [];
  if (basis === undefined && candidates.length === 0 && uncovered.length === 0) {
    return null;
  }
  return (
    <div className="instr-drift-panel">
      {basis !== undefined && <BasisRow page={page} />}
      {candidates.map((c, i) => (
        <DriftCandidateRow key={`${c.marker}-${i}`} row={c} pageNodeId={page.node.id} />
      ))}
      {uncovered.map((u, i) => (
        <p key={`${u.marker}-${i}`} className="muted instr-drift-uncovered">
          <span className="instr-drift-say">{u.marker}</span> not checked ({u.reason})
          {has(u.detail) && <>: {u.detail}</>}
        </p>
      ))}
      <Link to="/drift" className="muted instr-drift-more">
        project-wide drift report
      </Link>
    </div>
  );
}

// DriftCandidateRow renders one non-active_basis finding in the same
// .instr-drift row grammar BasisRow set: the marker said as a word, the
// facts the sweep recorded (detail sentence, files as plain text - the
// basenames nuance the /drift page documents holds here too - the since
// anchor, context lines), and the clearing move as a copyable command where
// one exists; where none does (stale_spec, orphan_eval) no command renders,
// because inventing one is the thing the console must never do. A candidate
// whose own node is an ancestor (a task page carrying its parent feature's
// finding, drift.ForNode's walk-up) leads with that ancestor's ref, so the
// finding is never misattributed to the page's node.
function DriftCandidateRow({ row, pageNodeId }: { row: CandidateView; pageNodeId: string }) {
  const elsewhere = row.id !== pageNodeId;
  const files = row.files ?? [];
  return (
    <div className="instr-drift">
      <span className="instr-drift-say">{row.marker}</span>
      {elsewhere && (
        <span>
          on {row.noun} <RefLink node={{ noun: row.noun, id: row.id, ref: refOf(row.key, row.id) }} />
        </span>
      )}
      {has(row.detail) && <span>{row.detail}</span>}
      {files.length > 0 && <span className="mono">{files.join(", ")}</span>}
      {row.since !== undefined && <span className="muted">since {shortSha(row.since)}</span>}
      {(row.context ?? []).map((line) => (
        <span key={line} className="muted">
          {line}
        </span>
      ))}
      {has(row.remedy) && <CopyCmd cmd={row.remedy} />}
    </div>
  );
}

// ---------------------------------------------------------------------------
// The attention stack (task 17, item 5; ui-surface.md's amended uniform-
// shell bullet, 2026-08-10, further amended 2026-08-11 by the task-17-review
// fix wave): every item asking a person for something on this node renders
// as its own box, all at once, rather than the earlier pass's single
// derived strip. The loud register is this node's own queue entries (the
// overlay join, Build's own second: views/build/overlays.ts's
// askLabelsFrom is the precedent, filtered by node id rather than scoped to
// one noun), a reconstructed next-owner fallback for the one gap the queue
// itself leaves silent (needsAttentionFallback, lib/attentionFallback.ts),
// and the warning register, unpushed_review, the one caution this page
// carries today.
// ---------------------------------------------------------------------------

// entriesForNode is the overlay join itself, a one-liner beside Build's
// (phase-3 carry ruling: extract a shared lib only if this outgrows one
// line, which it has not) - every queue entry addressed to this node,
// UNFILTERED. AttentionStack below both renders a filtered view of it (asks)
// and uses the unfiltered whole to decide whether the fallback derivation
// gets a turn at all.
function entriesForNode(queue: Queue | null, node: NodeView): QueueEntry[] {
  return (queue?.entries ?? []).filter((e) => e.id === node.id);
}

// asksFrom narrows entriesForNode's own list to what actually renders as a
// loud box, two kinds excluded on purpose:
//
// - attest_gate: the node page's attention surface divides the attest
//   surface by gate kind (ui-surface.md's "attention strip and Home's queue
//   cards divide the attest surface by gate kind" bullet, amended
//   2026-08-11): required criteria here, optional review gates on Home's
//   cards only. Folding an attest_gate entry in here would re-open that
//   division without spec sign-off.
// - approve_amendment: ui-surface.md's amendment bullet (2026-08-06,
//   "the amendment is a queue row plus a flat review page, and never a
//   button that applies") is a specific, reasoned decision - "the page
//   offers no write at all: the copyable `tiller approve amendment <addr>`
//   lives on the queue card" - that this stack's generic "every item
//   asking" wording does not override (task-17-review fix wave, Important
//   finding 2, flagged to JC). An amendment's own node page stays exactly
//   what that bullet says: a review surface with no write on it anywhere.
function asksFrom(entries: QueueEntry[]): QueueEntry[] {
  return entries.filter((e) => e.kind !== "attest_gate" && e.kind !== "approve_amendment");
}

// askKindQualifier names the one bare fact the mock's own kind line appends
// after DecisionCard's KIND_LABEL word for a criterion ask ("Attest · eval
// criterion C3", mock:1009): the criterion id itself, QueueEntry.Prose on
// this one kind (queue.go's own doc comment - never the node's title, which
// this box does not need anyway since it is already on that node's own
// page). This is deliberately NOT DecisionCard's own rowQualifier: that
// helper's "Criterion <id>" wording composes with WhoLine's leading title,
// a sentence this box never builds, so reusing it verbatim here would read
// "Attest · eval criterion Criterion C3". attest_gate carries no qualifier
// at all because it never reaches this box (asksFrom's own exclusion,
// above).
function askKindQualifier(entry: QueueEntry): string {
  return entry.kind === "attest_criterion" && has(entry.prose) ? entry.prose : "";
}

// AskBox is one queued entry in the loud register: the kind (DecisionCard's
// own KIND_LABEL plus askKindQualifier above), the same mechanical why
// sentence DecisionCard renders (whyLine, reused rather than re-derived so
// the two surfaces can never disagree about what a kind means), and the
// entry's own command, accent (the page's loud ink) because this is exactly
// the "needs the human" case the Global Constraint reserves it for.
function AskBox({ entry, showCommand = true }: { entry: QueueEntry; showCommand?: boolean }) {
  const qualifier = askKindQualifier(entry);
  const kind = KIND_LABEL[entry.kind] ?? entry.kind;
  const why = whyLine(entry);
  return (
    <div className={`abox${entry.actor === "agent" ? " warn" : ""}`}>
      <span className="kind">{qualifier !== "" ? `${kind} ${qualifier}` : kind}{entry.actor && ` · ${entry.actor}`}</span>
      {/* whyLine returns "" for a kind KIND_LABEL does not name either
          (fold-in minor, phase-4-review fix wave): an empty .why div still
          painted its own margin/line-height, a blank beat between the kind
          and the command for a kind this box cannot explain. Dropping the
          div rather than rendering it empty matches DecisionCard's own
          twin fix below. */}
      {why !== "" && <div className="why"><ProseInline markdown={why} /></div>}
      {showCommand && has(entry.command) && <CopyCmd cmd={entry.command} accent={entry.actor !== "agent"} />}
    </div>
  );
}

// remoteFrom names the remote a push command should target from the
// recorded upstream (an @{upstream} abbrev-ref, always "<remote>/<branch>"
// shaped per vcs.Git.resolveUpstream's own doc comment - never a bare branch
// name), rather than assuming "origin": a fork tracking a differently named
// remote still gets a command that actually works.
function remoteFrom(upstream: string): string {
  const slash = upstream.indexOf("/");
  return slash === -1 ? "origin" : upstream.slice(0, slash);
}

// WarnBox is the unpushed-review caution, the warning register's one tenant
// today (the retired console mock). Server
// gating (internal/views/nodepage.go's own doc comment on UnpushedReview)
// guarantees a PR is recorded whenever this field is present, so the why
// sentence names it directly rather than guarding a fallback the field can
// never actually reach.
function WarnBox({ review, pr }: { review: UnpushedReviewView; pr: number | undefined }) {
  return (
    <div className="abox warn">
      <span className="kind">Warning · state commit ahead of the PR</span>
      <div className="why">
        Commit <span className="mono">{review.commit}</span> on the working branch is not on PR #
        {pr}. Push the branch before merging or the crossing cannot be observed.
      </div>
      <CopyCmd cmd={`git push ${remoteFrom(review.upstream)} ${review.branch}`} />
    </div>
  );
}

// FallbackBox renders needsAttentionFallback's own {label, command} shape
// in the loud register, the identical two-element anatomy the pre-task-17
// strip always had (a label and a CopyCmd, no separate why sentence - there
// never was one to reuse here, unlike a queue entry's own mechanical why).
function FallbackBox({ action }: { action: AttentionFallback }) {
  return (
    <div className="abox">
      <span className="kind">{action.label}</span>
      <CopyCmd cmd={action.command} accent />
    </div>
  );
}

function AttentionStack({ page, queue }: { page: NodePage; queue: Queue | null }) {
  // Branch-observed state is not authority for integration commands: an
  // observed task keeps its queued asks (an open question still needs the
  // human) but shows none of their commands, no workflow box, no
  // unpushed-review warning about the served branch, and no fallback guess.
  const observed = page.execution !== undefined;
  const entries = entriesForNode(queue, page.node);
  const asks = asksFrom(entries).filter((entry) => entry.kind !== "workflow_closeout" || page.workflow === undefined);
  // The fallback only ever gets a turn once the queue has actually LOADED
  // and is silent about this node ENTIRELY - including an attest_gate or
  // approve_amendment entry asksFrom itself declines to render as a box: a
  // node the queue already has something to say about is never also handed
  // a second, vaguer guess (task-17-review fix wave, Important finding 4).
  //
  // "loaded" is its own condition, not entries.length alone (phase-4-review
  // fix wave, Important finding 1): views/NodeDetail.tsx's own queue fetch
  // is best-effort, so `queue` reads null for two different states -
  // not-yet-loaded and load-failed - and entriesForNode's `queue?.entries ??
  // []` folds both of those into the SAME empty list a genuinely loaded,
  // genuinely silent queue produces. Gating on entries.length alone made an
  // amendment's own page (next_owner human, a PRE_WORK_STATUSES status -
  // needsAttentionFallback's own eligible shape, which carries no
  // noun-of-amendment guard of its own) flash the forbidden approve command
  // spec:205 refuses on every load tick before the queue landed, and keep
  // flashing it forever on a failed read. Requiring `queue !== null` first
  // means the fallback only ever answers once the queue has actually had
  // its say.
  const workflow = observed ? undefined : page.workflow;
  const fallback =
    !observed && workflow === undefined && queue !== null && entries.length === 0
      ? needsAttentionFallback(page.node, page.relationships)
      : null;
  // The unpushed-review check reads the served checkout's branch, not the observed one.
  const review = observed ? undefined : page.unpushed_review;
  if (asks.length === 0 && fallback === null && review === undefined && workflow === undefined) {
    return null;
  }
  return (
    <div className="attn">
      {workflow !== undefined && (
        <div className={`abox${workflow.actor === "human" ? "" : " warn"}`}>
          <span className="kind">
            Workflow · {workflow.actor === "human" ? "Human" : workflow.actor === "agent" ? "Agent" : "Recorded"}
          </span>
          <div className="why"><ProseInline markdown={workflow.reason} /></div>
          {workflow.command !== undefined && !asks.some((entry) => entry.command === workflow.command) && (
            <CopyCmd cmd={workflow.command} accent={workflow.actor === "human"} />
          )}
        </div>
      )}
      {asks.map((e) => (
        <AskBox key={`${e.kind}:${e.gate ?? ""}:${e.prose ?? ""}`} entry={e} showCommand={!observed} />
      ))}
      {fallback !== null && <FallbackBox action={fallback} />}
      {review !== undefined && <WarnBox review={review} pr={page.node.pr} />}
    </div>
  );
}

// ---------------------------------------------------------------------------
// The type-aware body: tabs, per noun.
// ---------------------------------------------------------------------------

// DependenciesPanel is the Dependencies tab's own content: every walking
// edge RelationshipHub still carries once the parent climb (in the header's
// own Crumbs) and Source (dropped entirely) are out of the picture - both
// dependency directions, duplicate targets, supersession both ways, and the
// questions blocking this node. The tab's label still counts only direct
// dependency directions; every other relation keeps the panel reachable
// without pretending to be a dependency.
// A whole-branch review caught an earlier version of this panel that had
// narrowed `only` to
// exactly depends_on/depended_on_by: harmless on a fixture with nothing else
// populated, but it silently dropped the UI's only rendering of
// blocked_by_questions (populated server-side for every noun,
// internal/views/nodepage.go:330) and a feature's supersedes/superseded_by
// (nodepage.go:307-311) - a blocked feature's header shows the bare "blocked"
// span with no route at all to the question actually blocking it once that
// happened. `only` here is deliberately every group left in RelationshipHub's
// own list (features/tasks are claimed away by their own tabs before this
// panel ever sees them, so naming the rest is equivalent to naming none, but
// naming them keeps this call from silently widening again the next time the
// hub grows a group). The label's own count stays the settled formula,
// depends_on plus depended_on_by (dependencyCount), read straight from the
// payload rather than from whatever this call renders - a tab can carry more
// than its own count says, the same way `tasks` or `features` never claimed
// to count anything but their own list either.
function DependenciesPanel({ page }: { page: NodePage }) {
  return (
    <RelationshipHub
      relationships={page.relationships}
      claimed={[]}
      only={[
        "depends_on",
        "depended_on_by",
        "duplicate_of",
        "supersedes",
        "superseded_by",
        "blocked_by_questions",
      ]}
    />
  );
}

// dependencyTabCount separates the Dependencies tab's visible number from
// its reachability. Direct dependency edges keep the settled displayed count;
// duplicate, supersession, and blocking-question edges keep the panel
// reachable without claiming to be dependencies; only a panel with no
// renderable groups returns zero and is removed by withCounts.
function dependencyTabCount(relationships: NodePage["relationships"]): number | undefined {
  const count = dependencyCount(relationships);
  if (count > 0) {
    return count;
  }
  if (
    relationships.duplicate_of !== undefined ||
    (relationships.supersedes?.length ?? 0) > 0 ||
    (relationships.superseded_by?.length ?? 0) > 0 ||
    (relationships.blocked_by_questions?.length ?? 0) > 0
  ) {
    return undefined;
  }
  return 0;
}

// contractCount is the Contract tab's own count (the mock's own "Contract ·
// 6": the retired console mock), the authored
// contract's criteria length once it loaded well-formed. undefined for a
// malformed contract (nothing to count, and undefined bypasses the
// countable-tab zero-hides rule below rather than reading as a wrong zero -
// "never hide a malformed contract" holds here too) AND for a well-formed
// contract with zero criteria (JC's ruling, 2026-08-11): an authored-but-
// empty contract is information a reader acts on, not an absence to fold
// into the same zero the tab hides every other countable tab at, so it
// takes the malformed branch's own bypass rather than reading as a wrong
// zero. GateCardList (components/GateCard.tsx) renders that case's own
// honest empty-state body; this function only has to stop hiding the tab.
function contractCount(evalPayload: EvalPayload): number | undefined {
  if (evalPayload === null || isEvalError(evalPayload)) {
    return undefined;
  }
  const count = evalPayload.criteria.length;
  return count > 0 ? count : undefined;
}

// withCounts drops every countable tab (one this call ever set a `count` on)
// whose count is exactly zero, the amended tabs rule (ui-surface.md,
// 2026-08-10): "a tab that opens onto nothing is chrome teaching the reader
// to stop clicking tabs." A tab that never carries a count - spec, gates,
// and the Contract tab's own malformed-contract branch, which counts
// nothing rather than showing a wrong zero - always survives the filter, so
// spec always renders exactly as the rule requires.
function withCounts(tabs: NodeTab[]): NodeTab[] {
  return tabs.filter((t) => t.count === undefined || t.count > 0);
}

function attachedWorkTab(page: NodePage): NodeTab {
  const items = page.relationships.attached_work ?? [];
  return {
    key: "work-items",
    label: "Fix",
    count: items.length,
    render: () => <AttachedWorkPanel items={items} />,
  };
}

function amendmentsTab(page: NodePage): NodeTab {
  return {
    key: "amendments",
    label: "Amendments",
    count: page.relationships.amendment_history_warning
      ? undefined
      : uniqueAmendments(page.relationships.amendments).length,
    render: () => <AmendmentRelationsPanel relationships={page.relationships} />,
  };
}

function backlinksTab(page: NodePage): NodeTab {
  return {
    key: "backlinks",
    label: "Backlinks",
    count: page.backlinks.count > 0
      ? page.backlinks.count
      : !page.backlinks.complete || page.backlinks.diagnostics.length > 0 ? undefined : 0,
    render: () => <BacklinksPanel backlinks={page.backlinks} />,
  };
}

// Every tab label below is Title Case (fix wave, task-17-review minor: the
// mock's own tab row is Title Case throughout - "Spec", "Contract",
// "Tasks", "History", the retired console mock
// - and the initial pass left everything but Contract lowercase, one
// register split across one row). Keys and the `tab` search param stay the
// original lowercase wire vocabulary; only the DISPLAY label changed.

function featureTabs(page: NodePage): NodeTab[] {
  const tasks = page.relationships.tasks ?? [];
  const tabs: NodeTab[] = [{ key: "spec", label: "Spec", render: () => <FeatureSpecPanel page={page} /> }];
  // The evals tab hides outright when the node carries no contract at all; its
  // DISPLAY label reads "Contract" now, wire key and `tab` param unchanged (the
  // decision this task adopted).
  if (page.eval !== null) {
    tabs.push({
      key: "evals",
      label: "Contract",
      count: contractCount(page.eval),
      render: () => <FeatureEvalsPanel page={page} />,
    });
  }
  tabs.push(
    {
      key: "tasks",
      label: "Tasks",
      count: tasks.length,
      render: () => <FeatureTasksPanel page={page} />,
    },
    attachedWorkTab(page),
    amendmentsTab(page),
    {
      key: "dependencies",
      label: "Dependencies",
      count: dependencyTabCount(page.relationships),
      render: () => <DependenciesPanel page={page} />,
    },
    backlinksTab(page),
    {
      key: "history",
      label: "History",
      count: page.history.length,
      render: () => <HistorySection history={page.history} executionError={page.execution_history_error} />,
    },
  );
  return withCounts(tabs);
}

function epicTabs(page: NodePage): NodeTab[] {
  // The same feature-count fallback the header caption used to read with: a
  // decomposition where one exists, the plain relationships link list on an
  // older server that never sent one.
  const featureCount = page.decomposition?.features.length ?? page.relationships.features?.length ?? 0;
  const tabs: NodeTab[] = [{ key: "spec", label: "Spec", render: () => <EpicSpecPanel page={page} /> }];
  if (page.eval !== null) {
    tabs.push({
      key: "evals",
      label: "Contract",
      count: contractCount(page.eval),
      render: () => <EpicEvalsPanel page={page} />,
    });
  }
  tabs.push(
    {
      key: "features",
      label: "Features",
      count: featureCount,
      render: () => <EpicFeaturesPanel page={page} />,
    },
    attachedWorkTab(page),
    amendmentsTab(page),
    {
      key: "dependencies",
      label: "Dependencies",
      count: dependencyTabCount(page.relationships),
      render: () => <DependenciesPanel page={page} />,
    },
    backlinksTab(page),
    {
      key: "history",
      label: "History",
      count: page.history.length,
      render: () => <HistorySection history={page.history} executionError={page.execution_history_error} />,
    },
  );
  return withCounts(tabs);
}

function taskTabs(page: NodePage): NodeTab[] {
  const tabs: NodeTab[] = [
    { key: "spec", label: "Spec", render: () => <TaskSpecPanel page={page} /> },
    { key: "gates", label: "Gates", render: () => <TaskGatesPanel page={page} /> },
    attachedWorkTab(page),
    amendmentsTab(page),
    {
      key: "dependencies",
      label: "Dependencies",
      count: dependencyTabCount(page.relationships),
      render: () => <DependenciesPanel page={page} />,
    },
    backlinksTab(page),
    {
      key: "history",
      label: "History",
      count: page.history.length,
      render: () => <HistorySection history={page.history} executionError={page.execution_history_error} />,
    },
  ];
  return withCounts(tabs);
}

function workItemTabs(page: NodePage): NodeTab[] {
  const tabs: NodeTab[] = [
    { key: "spec", label: "Spec", render: () => <WorkItemSpecPanel page={page} /> },
    { key: "gates", label: "Gates", render: () => <WorkItemGatesPanel page={page} /> },
    {
      key: "dependencies",
      label: "Dependencies",
      count: dependencyTabCount(page.relationships),
      render: () => <DependenciesPanel page={page} />,
    },
    backlinksTab(page),
    {
      key: "history",
      label: "History",
      count: page.history.length,
      render: () => <HistorySection history={page.history} executionError={page.execution_history_error} />,
    },
  ];
  return withCounts(tabs);
}

// tabsFor is null for exactly the three cases that keep the page's old flat
// layout: a question and an amendment (ui-surface.md never asks either one
// for a tab row: "An amendment or a design doc ... stays flat the same
// way") and a noun this client does not recognize (the API validates nouns,
// so that is only reachable against a server newer than its client).
function tabsFor(page: NodePage): NodeTab[] | null {
  switch (page.node.noun) {
    case "epic":
      return epicTabs(page);
    case "feature":
      return featureTabs(page);
    case "task":
      return taskTabs(page);
    case "bug":
    case "chore":
    case "tweak":
      return workItemTabs(page);
    default:
      return null;
  }
}

function FlatBody({ page }: { page: NodePage }) {
  if (page.node.noun === "question") {
    return <QuestionBody page={page} />;
  }
  if (page.node.noun === "amendment") {
    return <AmendmentBody page={page} />;
  }
  // The shared layers below (the hub, history) still render everything they
  // can even when the body itself has nothing to draw.
  return null;
}

export default function NodeDetail({ page, queue = null }: { page: NodePage; queue?: Queue | null }) {
  const [params, setParams] = useSearchParams();
  const tabs = tabsFor(page);
  const recoveredReview = page.node.recovered_review ?? page.workflow?.recovered_review;

  // The drift panel's disclosure state: transient, component-local, never a
  // URL param (the house convention every other expand/collapse keeps). The
  // default is open when any badge refuses - a refusing condition is never
  // hidden behind a click, which is what lets the panel absorb the old
  // always-visible basis-drift line without a visibility regression - and
  // collapsed otherwise; the reader's own toggle wins after the first click
  // (null means "no manual choice yet", true/false once there is one). A
  // page with no badges but a basis
  // report (drift checks disabled in the manifest still compute active_basis
  // for the page) keeps the old behavior exactly: the row shows, no chip to
  // toggle it.
  const badges = page.drift ?? [];
  const [driftOpen, setDriftOpen] = useState<boolean | null>(null);
  const driftExpanded =
    badges.length > 0
      ? (driftOpen ?? badges.some((b) => b.refusing))
      : page.active_basis !== undefined;

  function selectTab(key: string) {
    const next = new URLSearchParams(params);
    next.set("tab", key);
    // A tab switch is not its own history entry (ui-surface.md never asks
    // for one); replace, not push, so the back button leaves the page the
    // way it found it.
    setParams(next, { replace: true });
  }

  return (
    <article className="instr">
      <NodeDetailHeader
        page={page}
        driftExpanded={driftExpanded}
        onDriftToggle={() => {
          setDriftOpen(!driftExpanded);
        }}
      />
      {page.execution !== undefined && (
        <>
          <ExecutionProvenance observation={page.execution} />
          <p className="muted execution-boundary">Observed execution; served-checkout status: {page.execution.integration_status}. This observation does not establish integration eligibility or authorize lifecycle commands on the served integration checkout.</p>
        </>
      )}
      <AttentionStack page={page} queue={queue} />
      {page.node.review_missing === true && page.workflow === undefined && (
        <div className="abox warn">Aggregate review is not recorded; sync has not inspected its merge state. If unmerged, complete ordinary submission; if already merged, explicitly authorized historical recovery is required from integration, followed by ordinary sync.</div>
      )}
      {recoveredReview !== undefined && (
        <RecoveredReview
          record={recoveredReview}
          acceptanceReady={page.workflow?.acceptance_ready}
          outstanding={page.workflow?.outstanding}
          gates={page.node.gates}
          crossingsPending={page.workflow?.recovered_crossings_pending}
          currentAcceptance={page.workflow?.current_acceptance}
          historicalViolations={page.workflow?.historical_violations}
          crossingViolations={page.workflow?.crossing_violations}
        />
      )}
      {driftExpanded && <DriftPanel page={page} />}
      {(page.node.noun === "epic" || page.node.noun === "feature" || page.node.noun === "task") && (
        <ProposedAmendmentNotice relationships={page.relationships} />
      )}
      {tabs !== null ? (
        <NodeTabs tabs={tabs} active={params.get("tab") ?? tabs[0].key} onSelect={selectTab} />
      ) : (
        <>
          <FlatBody page={page} />
          <RelationshipHub
            relationships={page.relationships}
            claimed={claimedGroups[page.node.noun] ?? []}
          />
          <BacklinksPanel backlinks={page.backlinks} />
          <HistorySection history={page.history} executionError={page.execution_history_error} />
        </>
      )}
    </article>
  );
}
