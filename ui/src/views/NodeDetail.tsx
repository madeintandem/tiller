// The uniform node page (ui-surface.md, "Node detail"), the surface the whole
// console funnels into: every list row, queue item, recap entry, and search
// hit lands here. Since the persistent-hierarchy change (JC, 2026-08-05) this
// renders as the Explore layout's reading pane - App mounts it as the
// /node/:noun/* child of the Explore layout route, so the navigator stands at
// its left on every node page - and this wrapper owns only the
// fetch-by-address and its loading/error states, framed in the pane's own
// .explore-detail column. The instrument itself (task C3, mock round 4) is
// components/NodeDetail, a reusable render of one already-fetched NodePage.
// Navigating a relationship link changes the route params, which changes the
// fetch path, which makes useFetch clear the old node and load the new one —
// the component never shows a stale node under a fresh address. A reread of
// the same address (the topbar refresh, the window regaining focus) is the
// opposite case: both reads keep their last answer until the new one lands,
// so the page, its tab, and its branch history rows stay where they are.
//
// Task 17 (console v2, node page rebuild) adds a second, independent fetch
// here: the project queue, the source for the instrument's own attention
// stack (a node's queue entries, filtered by id, rendered in the loud
// register beside unpushed_review's warning box). It is best-effort, the
// identical overlay posture Build's own queue join already takes (views/
// build/overlays.ts's own doc comment): a slow or failed queue read never
// blocks or errors the page, the node itself already fetched through `page`
// above; the instrument just renders no ask boxes until it lands. It stays
// unconditional (Build.tsx's own rule: hook order must never depend on data
// that has not arrived yet), fetched even when `path` is null, so this
// component's hook order never varies with the route. Its refresh key is the
// node request path, so each node arrival rereads it and clears the previous
// node's asks while the read settles.

import { useEffect, useRef } from "react";
import { useParams } from "react-router-dom";
import type { NodeBranchHistory, NodePage, Queue } from "../api";
import { nodeBranchHistoryPath, nodePagePath, queuePath } from "../api";
import NodeDetailInstrument from "../components/NodeDetail";
import { handleOf } from "../components/NodeHeader";
import OutOfDate from "../components/OutOfDate";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import { recordRecent } from "../lib/recentNodes";
import { useExploreProjectKey } from "./explore/exploreContext";
import "./node.css";

export default function NodeDetail() {
  const params = useParams();
  const noun = params.noun;
  // The splat carries the whole remaining path, which for a task is its
  // three-segment key and for everything else is a single id or key. Both go to
  // the API unchanged; nodePagePath re-encodes each segment so the slashes
  // survive the round trip.
  const addr = params["*"];
  // The route guarantees both, but the types do not; a null path keeps useFetch
  // idle rather than fetching a malformed address. An empty splat (`/node/task/`
  // with nothing after it) is malformed in exactly that way.
  //
  // The first read leaves out an observed task's branch history, which can
  // take far longer than the rest of the page; that answer says
  // history_pending, and a second read brings the merged history alone, which
  // takes the place of the first answer's history rows.
  const path =
    noun !== undefined && addr !== undefined && addr !== "" ? nodePagePath(noun, addr, "integration") : null;
  const first = useFetch<NodePage>(path);
  const branchPath =
    first.data?.history_pending === true && noun !== undefined && addr !== undefined
      ? nodeBranchHistoryPath(noun, addr)
      : null;
  const branch = useFetch<NodeBranchHistory>(branchPath);
  const page = withBranchHistory(first.data, branch.data, branch.error);
  const { error } = first;
  // What a failed reread left out of date: the page read itself, or else a
  // branch read that failed while its last good history stays shown.
  const staleError = error ?? (branch.data !== null ? branch.error : null);
  // Keyed to the node request path: navigating to another node rereads the
  // queue, so a snapshot taken on the previous node (perhaps before a CLI
  // mutation answered its ask) never drives this node's attention stack, and
  // a late response for the previous node is discarded.
  const queue = useFetch<Queue>(queuePath, { refreshKey: path ?? undefined });
  // The tab says what the heading says (the same handle NodeHeader draws),
  // "No such node" for the 404 surface below, and the raw address while the
  // page is still loading or failed some other way, so a tab never names a
  // node it is not showing.
  useDocumentTitle(titleOf(tabHandle(addr, page, error)));

  // Explore's empty pane lists what was opened lately. A node counts once its
  // page has loaded (never a failed load), keyed to the node's id so a reread,
  // the merged-history swap, or a tab change does not record it again.
  const projectKey = useExploreProjectKey();
  const openedId = page?.node.id;
  const openedTitle = page === null ? "" : handleOf(page.node);
  const openedNoun = page?.node.noun ?? "";
  const openedKey = page?.node.key;
  // A ref remembers the last (project, node) recorded, so the same node coming
  // back (a reread, or a page that blanks and returns) is one visit.
  const recorded = useRef("");
  useEffect(() => {
    if (projectKey !== undefined && openedId !== undefined) {
      const visit = `${projectKey}\n${openedId}`;
      if (recorded.current === visit) {
        return;
      }
      recorded.current = visit;
      recordRecent(projectKey, { noun: openedNoun, id: openedId, title: openedTitle, key: openedKey });
    }
    // The title is read when the node arrives; a later change of it alone is
    // not a new visit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectKey, openedId]);

  return (
    <div className="explore-detail">{paneContent(noun, addr, page, error, staleError, queue.data)}</div>
  );
}

// The pane's states, in the order they can be told apart. The empty-splat
// case must not spin: with a null path useFetch stays idle forever, and a
// permanent spinner is the same defect as a blank page wearing a different hat.
// A page already on screen stays through a failed reread, marked out of date,
// unless the node is gone (a 404), which says so instead.
function paneContent(
  noun: string | undefined,
  addr: string | undefined,
  page: NodePage | null,
  error: string | null,
  staleError: string | null,
  queue: Queue | null,
) {
  if (noun === undefined || addr === undefined || addr === "") {
    return <NoNodeAddressed noun={noun ?? ""} />;
  }
  if (error !== null && (page === null || isNotFound(error))) {
    return <LoadError noun={noun} addr={addr} error={error} />;
  }
  if (page === null) {
    return <p className="muted">Loading node...</p>;
  }
  return (
    <>
      <OutOfDate error={staleError} />
      <NodeDetailInstrument page={page} queue={queue} />
    </>
  );
}

// The page to draw. The first answer always drives it (header, status, spec,
// workflow), so a reread's fresh page shows the moment it lands. Its History
// takes the branch read's merged rows, or, while a reread of them is out or
// after one failed, the last good rows (the pane marks a failure out of
// date), so the pending row never comes back over rows already shown. Until
// the first branch answer the pending row stands; a failed first branch read
// ends the wait with the failure named where the branch history would be.
function withBranchHistory(
  first: NodePage | null,
  branch: NodeBranchHistory | null,
  branchError: string | null,
): NodePage | null {
  if (first === null || first.history_pending !== true) {
    return first;
  }
  if (branch !== null) {
    return {
      ...first,
      history: branch.history,
      execution_history_error: branch.execution_history_error,
      history_pending: undefined,
    };
  }
  if (branchError !== null) {
    return { ...first, history_pending: undefined, execution_history_error: branchError };
  }
  return first;
}

function tabHandle(addr: string | undefined, page: NodePage | null, error: string | null): string {
  if (page !== null) {
    return handleOf(page.node);
  }
  if (error !== null && isNotFound(error)) {
    return "No such node";
  }
  return addr === undefined || addr === "" ? "No node addressed" : addr;
}

// fetchJSON folds the status line into its message, so a 404 is recognized by
// that line rather than a structured code. The read API answers 404 for every
// flavor of "no such page" (unknown noun, unresolved address, noun mismatch),
// and the server's own reason rides in the message.
function isNotFound(error: string): boolean {
  return error.includes(" failed (404");
}

// An address with a noun and nothing after it. The fetch never starts, so there
// is no error to render, and the loading line would spin forever: the same
// defect as the blank page this route change exists to remove.
function NoNodeAddressed({ noun }: { noun: string }) {
  return (
    <section>
      <h1>No node addressed</h1>
      <p>
        <span className="mono">/node/{noun}/</span> names a kind but not a node.
      </p>
    </section>
  );
}

// The two flavors of a failed load, framed one way for every arrival - a
// stale deep link and a navigator selection face identical failures now that
// this pane is the one place a node renders (the C2 reading pane that once
// imported this merged into it).
function LoadError({ noun, addr, error }: { noun: string; addr: string; error: string }) {
  if (!isNotFound(error)) {
    return <p className="error">{error}</p>;
  }
  return (
    <section>
      <h1>No such node</h1>
      <p>
        Nothing in this working tree answers to{" "}
        <span className="mono">
          {noun}/{addr}
        </span>
        .
      </p>
      <p className="muted">{error}</p>
    </section>
  );
}
