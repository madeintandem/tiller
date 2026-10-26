// Home is the desk: a header band (identity plus stat tiles) over the
// needs-you card grid, with the pulse - past, present, future - beneath it
// (console v2 phase 2 rebuild). Every fetch here is Home's own
// (App.tsx owns /api/status and passes it down, its header badge already
// needing it), so a slow or failed section never blanks its neighbors: the
// same per-panel resilience the page has carried since the first desk pass.
//
// The needs-you column runs full width, unconditionally, same as before: the
// rail has no standing tenant (ui-surface.md's rail decision, "an empty rail
// is never drawn"), so there is nothing to branch a split layout on.
//
// THE ONE INVARIANT THAT MUST HOLD: Home never requests the visit-window
// recap. Just happened reads fetchRecap("24h"), a FIXED window that the
// server never treats as a visit (History.tsx's own doc comment: only that
// view's DEFAULT window advances the last-visit mark); the queue's own
// horizon count comes from fetchRecapPeek(), the non-marking counts-only
// read. Landing on Home, or leaving it open on a slow connection, must never
// spend a digest the human has not opened. home.test.tsx pins this by
// asserting on every URL Home's own fetch spy actually requested.

import { useSearchParams } from "react-router-dom";
import type { FetchState } from "../hooks";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import type {
  DriftReportView,
  ExecutionSnapshot,
  ListResult,
  ListRow,
  NextAnswer,
  ProjectSummary,
  Queue,
  Recap,
  RecoveredReviewView,
  Streams,
  WhoseMoveCounts,
} from "../api";
import { boardPath, driftPath, executionPath, listPath, nextPath, queuePath, recapPath } from "../api";
import AllClear from "../components/AllClear";
import type { AllClearReviewItem } from "../components/AllClear";
import DecisionBand from "../components/DecisionBand";
import DecisionCard, { KIND_LABEL } from "../components/DecisionCard";
import DependencyDiagnostics from "../components/DependencyDiagnostics";
import ExecutionSources from "../components/ExecutionSources";
import NodeTabs from "../components/NodeTabs";
import PageLoading from "../components/PageLoading";
import RecapSignal from "../components/RecapSignal";
import RefLink from "../components/RefLink";
import RecoveredReview from "../components/RecoveredReview";
import SectionHead from "../components/SectionHead";
import { addrOf } from "../lib/addr";
import { has } from "../lib/has";
import type { BandRun } from "./home/bands";
import { bandRuns, queueEntryKey } from "./home/bands";
import HeaderBand from "./home/HeaderBand";
import { needsTabs, runCount, runKind } from "./home/needsTabs";
import { InFlightColumn, JustHappenedColumn, UpNextColumn } from "./home/Pulse";
import { groupQueueByGravity } from "./home/queueGroups";
import "./home.css";

interface HomeProps {
  summary: ProjectSummary | null;
  summaryError: string | null;
}

export default function Home({ summary, summaryError }: HomeProps) {
  // Read up here with the other hooks, ahead of the loading and error
  // returns below, so the hook order never changes between renders.
  const [search, setSearch] = useSearchParams();
  // The desk's tab names the project; with no summary yet, or none at all,
  // the bare product name is the honest title rather than a stale one.
  useDocumentTitle(summary === null ? "Tiller" : titleOf(summary.project.name));
  // Every fetch is unconditional so hook order never depends on whether
  // summary has arrived yet (App.tsx's own useFetch resolves independently).
  const queue = useFetch<Queue>(queuePath);
  const epics = useFetch<ListResult>(listPath("epic"));
  const features = useFetch<ListResult>(listPath("feature"));
  const tasks = useFetch<ListResult>(listPath("task"));
  const questions = useFetch<ListResult>(listPath("question"));
  const bugs = useFetch<ListResult>(listPath("bug"));
  const chores = useFetch<ListResult>(listPath("chore"));
  const tweaks = useFetch<ListResult>(listPath("tweak"));
  const drift = useFetch<DriftReportView>(driftPath);

  // The pulse's three reads. recapPath("24h") is a FIXED window - never
  // "visit" - so it never advances the last-visit mark (the invariant this
  // whole file exists to hold); fetchBoard restores the tenant the console
  // v2 pass gave Home's In flight column (api.ts's own comment at
  // fetchBoard); nextPath is the ranked next list for the served checkout's
  // contributor, read never recomputed, which also carries the dependency
  // diagnostics.
  const justHappened = useFetch<Recap>(recapPath("24h"));
  const board = useFetch<Streams>(boardPath);
  const upNext = useFetch<NextAnswer>(nextPath);
  // Read with the desk rather than when the Branches tab opens, since the
  // tab's own label carries the diagnostics count. Like every read above it
  // refetches on mount and on the topbar's manual refresh, never on a tab
  // switch.
  const execution = useFetch<ExecutionSnapshot>(executionPath);

  if (summaryError !== null) {
    return <p className="error">{summaryError}</p>;
  }
  if (summary === null) {
    return <PageLoading title="Home" />;
  }

  const lists: NounList[] = [
    { noun: "epic", state: epics },
    { noun: "feature", state: features },
    { noun: "task", state: tasks },
    { noun: "question", state: questions },
    { noun: "bug", state: bugs },
    { noun: "chore", state: chores },
    { noun: "tweak", state: tweaks },
  ];
  const queueEntries = queue.data?.entries ?? [];
  // Gravity re-ranks the KIND buckets only (queueGroups.ts's own contract);
  // bandRuns turns that ranking into the runs the needs-you section draws,
  // one per kind: a kind past ROW_CAP becomes one compressed band, everything
  // smaller stays a stack of the settled cards, and each run is one COLUMN of
  // the body's grid (JC, 2026-08-31: three kinds past the cap read as three
  // full-width walls of rows; side by side the page is as tall as the
  // longest kind, not the sum). Member order within a kind stays exactly what
  // the server served (the dependency order). total counts every
  // entry, never a run, so the badge and the empty check stay honest about
  // how many decisions are actually waiting.
  const groups = groupQueueByGravity(queueEntries);
  const runs = bandRuns(groups);
  const total = queueEntries.length;
  // Past the cap and with more than one kind, the runs go behind a tab rail
  // and one kind renders at a time (needsTabs.ts); the choice rides the URL
  // as ?needs=<kind> so a link lands on a kind, replace rather than push,
  // since walking back through tabs is not history worth keeping.
  const tabs = needsTabs(runs, total, search.get("needs"));
  const selectKind = (kind: string) => {
    const next = new URLSearchParams(search);
    next.set("needs", kind);
    setSearch(next, { replace: true });
  };
  const renderRun = (run: BandRun) =>
    run.type === "band" ? (
      <DecisionBand key={run.group.kind} group={run.group} tabbed={tabs.active !== null} />
    ) : (
      <div className="card-grid" data-kind={run.kind} key={run.kind}>
        {run.entries.map((entry) => (
          <DecisionCard key={queueEntryKey(entry)} entry={entry} />
        ))}
      </div>
    );

  // The page's own tab rail: the desk under Overview, execution sources under
  // Branches (JC, 2026-10-06; they used to sit above every page, and the tab
  // is about where each in-flight task's branch was found), and recovered
  // review records under Recovered reviews. The choice rides
  // ?tab=branches or ?tab=recovered, absent for Overview, replace rather than push like
  // ?needs= above, and the two params leave each other alone. An unknown
  // value falls back to Overview through NodeTabs' own first-tab rule.
  const selectView = (key: string) => {
    const next = new URLSearchParams(search);
    if (key === "overview") {
      next.delete("tab");
    } else {
      next.set("tab", key);
    }
    setSearch(next, { replace: true });
  };
  const diagnosticCount = execution.data?.diagnostics.length ?? 0;
  // Recovered reviews are provenance, not decisions, and a finished one never
  // stops being true, so they moved off the desk into their own tab (JC,
  // 2026-10-21: a done feature's recovery was the biggest thing on Home). The
  // tab exists only while there is a record to show; its count warns when any
  // record still leaves riders pending or acceptance unresolved. Records come
  // from the epic and feature lists alone: no next move carries one.
  const recoveryOwners = recoveredOwners(features.data?.nodes ?? [], epics.data?.nodes ?? []);
  const recoveryCount = recoveryOwners.length;
  const recoveryUnsettled = recoveryOwners.some(({ row }) => row.recovered_crossings_pending === true ||
    row.current_acceptance?.acceptance_ready === false);

  const overview = (
    <>
      <HeaderBand
        summary={summary}
        epics={epics}
        features={features}
        tasks={tasks}
        questions={questions}
        bugs={bugs}
        chores={chores}
        tweaks={tweaks}
        drift={drift}
      />
      <DependencyDiagnostics report={upNext.data?.dependency_diagnostics} />
      <section className="needs">
        <SectionHead
          label="Needs you"
          badge={total > 0 ? `${total} ${total === 1 ? "decision" : "decisions"}` : undefined}
          headerExtra={<RecapSignal />}
        />
        {queue.error !== null && <p className="error">{queue.error}</p>}
        {queue.error === null && queue.data === null && <p className="muted">Loading queue...</p>}
        {queue.data !== null &&
          (total === 0 ? (
            <AllClearSlot whoseMove={summary.whose_move} lists={lists} />
          ) : tabs.active !== null ? (
            <NodeTabs
              label="Decision kind"
              active={tabs.active}
              onSelect={selectKind}
              tabs={runs.map((run) => ({
                key: runKind(run),
                label: KIND_LABEL[runKind(run)] ?? runKind(run),
                count: runCount(run),
                render: () => <div className="needs-body">{renderRun(run)}</div>,
              }))}
            />
          ) : (
            <div className="needs-body">{runs.map(renderRun)}</div>
          ))}
      </section>
      <section className="pulse">
        <JustHappenedColumn state={justHappened} />
        <InFlightColumn state={board} />
        <UpNextColumn state={upNext} />
      </section>
    </>
  );

  return (
    <div className="home">
      <NodeTabs
        label="Home view"
        active={search.get("tab") ?? "overview"}
        onSelect={selectView}
        tabs={[
          { key: "overview", label: "Overview", render: () => overview },
          {
            key: "branches",
            label: "Branches",
            count: diagnosticCount > 0 ? diagnosticCount : undefined,
            countTone: "warning",
            render: () => <ExecutionSources state={execution} />,
          },
          ...(recoveryCount > 0
            ? [{
              key: "recovered",
              label: "Recovered reviews",
              count: recoveryCount,
              countTone: recoveryUnsettled ? "warning" as const : undefined,
              render: () => <RecoveredReviews owners={recoveryOwners} features={features.data?.nodes ?? []} />,
            }]
            : []),
        ]}
      />
    </div>
  );
}

// One work-tree list fetch, tagged with the noun it was fetched under
// (ListRow itself carries no noun).
interface NounList {
  noun: string;
  state: FetchState<ListResult>;
}

// AllClearSlot is the needs-you column's empty-queue branch, and the gate the
// all-clear claim must pass: the claim is only rendered once all six lists
// have actually loaded. A failed list replaces it with an honest error
// surface naming what could not be read (useFetch nulls data on error, so a
// failed shelf list is precisely where an in_review item could be hiding); a
// still-pending list holds the slot at loading rather than flashing a claim.
function AllClearSlot({ whoseMove, lists }: { whoseMove: WhoseMoveCounts; lists: NounList[] }) {
  const failed = lists.filter((l) => l.state.error !== null);
  if (failed.length > 0) {
    return (
      <div className="all-clear-unverified">
        <p className="all-clear-unverified-lead">
          The queue is empty, but part of the project state failed to load, so the console cannot
          say nothing needs you:
        </p>
        {failed.map((l) => (
          <p key={l.noun} className="error">
            {l.state.error}
          </p>
        ))}
      </div>
    );
  }
  if (lists.some((l) => l.state.data === null)) {
    return <p className="muted">Loading project state...</p>;
  }
  return <AllClear whoseMove={whoseMove} inReview={inReviewItems(lists)} />;
}

// inReviewItems enumerates every node across all six noun lists
// sitting at in_review. This backs the all-clear rule: an in-review node's
// next_owner reads "agent" (resume/sync are the legal Tiller verbs from
// there), but the human's own PR review and merge is very much still
// outstanding, so it must be named rather than folded into "nothing needs
// you". Only called once AllClearSlot has confirmed every list actually
// loaded, so an absent list can never silently enumerate as "nothing".
function inReviewItems(lists: NounList[]): AllClearReviewItem[] {
  const items: AllClearReviewItem[] = [];
  for (const { noun, state } of lists) {
    for (const row of state.data?.nodes ?? []) {
      if (row.status === "in_review") {
        const label = has(row.title) ? row.title : has(row.intent) ? row.intent : addrOf(row);
        items.push({ noun, id: row.id, label, recovery: row.recovered_review, reviewMissing: row.review_missing });
      }
    }
  }
  return items;
}

function sameRecovery(left: RecoveredReviewView | undefined, right: RecoveredReviewView | undefined): boolean {
  return left !== undefined && right !== undefined && left.branch === right.branch &&
    left.head === right.head && left.merge_commit === right.merge_commit &&
    left.integration_branch === right.integration_branch;
}

// The epic's participating acceptance names the features that rode its
// recovered review. Without it (the observation was unavailable), fall back to
// every feature sharing the record, whose acceptance then reads unavailable.
function recoveredEpicFeatures(epic: ListRow, features: ListRow[]): ListRow[] {
  const participating = epic.participating_acceptance;
  if (participating === undefined) {
    return features.filter((feature) => sameRecovery(feature.recovered_review, epic.recovered_review));
  }
  return features.filter((feature) => participating.some((fact) => fact.node_id === feature.id));
}

interface RecoveryOwner {
  noun: string;
  row: ListRow;
}

// Each recorded review once: an epic's record carries its riding features.
function recoveredOwners(features: ListRow[], epics: ListRow[]): RecoveryOwner[] {
  return [
    ...epics.filter((row) => row.recovered_review !== undefined).map((row) => ({ noun: "epic", row })),
    ...features.filter((row) => row.recovered_review !== undefined &&
      !epics.some((epic) => sameRecovery(epic.recovered_review, row.recovered_review))).map((row) => ({ noun: "feature", row })),
  ];
}

// Terminal reviews remain visible even when no next move names them.
// Current acceptance comes from the observed list projection, not gate inference.
function RecoveredReviews({ owners, features }: {
  owners: RecoveryOwner[];
  features: ListRow[];
}) {
  return <>{owners.map(({ noun, row }) => {
    const record = row.recovered_review;
    if (record === undefined) return null;
    return (
      <section key={row.id}>
        <RefLink node={{ noun, id: row.id, ref: row.ref }} label={addrOf(row)} />
        <RecoveredReview record={record} gates={row.gates}
          acceptanceReady={row.current_acceptance?.acceptance_ready}
          outstanding={row.current_acceptance?.outstanding}
          currentAcceptance={row.participating_acceptance}
          crossingsPending={row.recovered_crossings_pending}
          historicalViolations={row.historical_violations}
          crossingViolations={row.crossing_violations}
        />
        {noun === "epic" && recoveredEpicFeatures(row, features).map((feature) => (
          <section key={feature.id}>
            <RefLink node={{ noun: "feature", id: feature.id, ref: feature.ref }} label={addrOf(feature)} />
            <p>Current acceptance: {feature.current_acceptance === undefined ? "unavailable" : feature.current_acceptance.acceptance_ready ? "ready" : "unresolved"}.</p>
            <ul>{Object.entries(feature.gates ?? {}).map(([name, gate]) => <li key={name}>{name}: {gate.status ?? "pending"}</li>)}</ul>
          </section>
        ))}
      </section>
    );
  })}</>;
}
