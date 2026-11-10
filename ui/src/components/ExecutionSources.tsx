// ExecutionSources is the body of Home's Branches tab (JC, 2026-10-06): the
// task execution observations the console found in other worktrees and
// branches, and the visibility diagnostics for sources it could not use. It
// used to be a collapsed disclosure in the App shell above every page, where
// a real project's diagnostics took the top of every view. Home owns the
// /api/execution fetch, since the tab's own count needs it before the tab is
// open, and hands the state down.

import { useState } from "react";
import type { ExecutionRecord, ExecutionSnapshot } from "../api";
import type { FetchState } from "../hooks";
import { diagnosticLabel } from "../lib/executionWords";
import SectionHead from "./SectionHead";
import TitleRef from "./TitleRef";
import Time from "./Time";
import "./execution.css";

const sourceLabels = {
  working_tree: "working tree",
  local_branch: "local branch",
  remote_tracking: "remote-tracking branch (last fetched observation, not remote current)",
};

const freshnessLabels = {
  local_branch_behind: "local branch is behind its tracking ref",
  remote_tracking_behind: "tracking ref is behind the local branch",
};

// Diagnostics share History's row cap, so a long list never buries the page.
const DIAGNOSTIC_CAP = 8;

// showObserved is true for the node page, where the line is the only place the
// observation time appears. Home's Branches tab states it once above the list.
export function ExecutionProvenance({ observation, showObserved = true }: { observation: ExecutionRecord; showObserved?: boolean }) {
  return (
    <div className="execution-provenance">
      <span>{sourceLabels[observation.source]}</span>{" · "}
      <span className="mono">{observation.branch}</span>{" · "}
      <span className="mono" title={`Opaque source read token: ${observation.revision}`}>snapshot {observation.revision.slice(-12)}</span>{" · "}
      <span>{observation.uncommitted ? "uncommitted Tiller state" : "committed Tiller state"}</span>
      {observation.freshness !== undefined && <>{" · "}<span>{freshnessLabels[observation.freshness]}</span></>}
      {showObserved && <>{" · "}<span>observed <Time iso={observation.observed_at} /></span></>}
    </div>
  );
}

export default function ExecutionSources({ state }: { state: FetchState<ExecutionSnapshot> }) {
  const { data, error } = state;
  const [allDiagnostics, setAllDiagnostics] = useState(false);
  const diagnostics = data === null ? [] : allDiagnostics ? data.diagnostics : data.diagnostics.slice(0, DIAGNOSTIC_CAP);
  // One read observes every row, so the newest time stands for the list.
  const observedAt = data === null ? undefined : data.observations.map((o) => o.observed_at).sort().at(-1);
  const hidden = data === null ? 0 : data.diagnostics.length - diagnostics.length;
  return (
    <section className="execution-sources" aria-label="Execution sources">
      {error !== null && <p role="alert">Execution sources unavailable: {error}</p>}
      {data === null && error === null && <p className="muted">Loading execution sources…</p>}
      {data !== null && (
        <>
          {observedAt !== undefined && <p className="muted">Observed <Time iso={observedAt} /></p>}
          <details className="execution-fold">
            <summary>How branch observation works</summary>
            <p className="muted">Navigation, the topbar refresh, and coming back to the window reread local sources. The console does not fetch Git remotes. Fetch outside the console, then refresh to see updated remote-tracking observations. Observed state is not integration eligibility or permission to run commands on integration.</p>
            <p className="muted">Global History shows integration history. A task's node History includes that task's events from its selected observation source.</p>
          </details>
          {data.diagnostics.length > 0 && (
            <section aria-label="Visibility diagnostics">
              <SectionHead label={`Visibility diagnostics · ${data.diagnostics.length}`} />
              <ul>{diagnostics.map((diagnostic, index) => (
                <li key={`${diagnostic.code}:${diagnostic.id ?? ""}:${diagnostic.branch ?? ""}:${index}`}>
                  {diagnostic.id && <><TitleRef node={{ noun: "task", id: diagnostic.id, ref: diagnostic.id }} title={diagnostic.title} addr={diagnostic.id} />{" · "}</>}
                  <strong>{diagnosticLabel(diagnostic.code)}</strong>: {diagnostic.message}
                  {diagnostic.branch && <div className="muted"><span className="mono">{diagnostic.branch}</span></div>}
                </li>
              ))}</ul>
              {hidden > 0 && (
                <button type="button" className="instr-more" onClick={() => setAllDiagnostics(true)}>
                  {hidden} more {hidden === 1 ? "diagnostic" : "diagnostics"} ▸
                </button>
              )}
            </section>
          )}
          <section aria-label="Task observations">
            <SectionHead label={`Task observations · ${data.observations.length}`} />
            {data.observations.length === 0 ? (
              <p className="muted">No task execution observations discovered.</p>
            ) : (
              <ul>
                {data.observations.map((observation) => (
                  <li key={`${observation.id}:${observation.source}:${observation.branch}`}>
                    <TitleRef
                      node={{ noun: "task", id: observation.id, ref: observation.key }}
                      title={observation.title}
                      addr={observation.key || observation.id}
                    />
                    <ExecutionProvenance observation={observation} showObserved={false} />
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}
    </section>
  );
}
