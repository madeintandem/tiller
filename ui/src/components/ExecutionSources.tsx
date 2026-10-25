// ExecutionSources is the body of Home's Branches tab (JC, 2026-10-06): the
// task execution observations the console found in other worktrees and
// branches, and the visibility diagnostics for sources it could not use. It
// used to be a collapsed disclosure in the App shell above every page, where
// a real project's diagnostics took the top of every view. Home owns the
// /api/execution fetch, since the tab's own count needs it before the tab is
// open, and hands the state down.

import { useState } from "react";
import { Link } from "react-router-dom";
import type { ExecutionRecord, ExecutionSnapshot } from "../api";
import type { FetchState } from "../hooks";
import SectionHead from "./SectionHead";
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

export function ExecutionProvenance({ observation }: { observation: ExecutionRecord }) {
  return (
    <div className="execution-provenance">
      <span>{sourceLabels[observation.source]}</span>{" · "}
      <span className="mono">{observation.branch}</span>{" · "}
      <span className="mono" title={`Opaque source read token: ${observation.revision}`}>snapshot {observation.revision.slice(-12)}</span>{" · "}
      <span>{observation.uncommitted ? "uncommitted Tiller state" : "committed Tiller state"}</span>{" · "}
      {observation.freshness !== undefined && <><span>{freshnessLabels[observation.freshness]}</span>{" · "}</>}
      <span title={observation.observed_at}>observed {new Date(observation.observed_at).toLocaleString()}</span>
    </div>
  );
}

export default function ExecutionSources({ state }: { state: FetchState<ExecutionSnapshot> }) {
  const { data, error } = state;
  const [allDiagnostics, setAllDiagnostics] = useState(false);
  const diagnostics = data === null ? [] : allDiagnostics ? data.diagnostics : data.diagnostics.slice(0, DIAGNOSTIC_CAP);
  const hidden = data === null ? 0 : data.diagnostics.length - diagnostics.length;
  return (
    <section className="execution-sources" aria-label="Execution sources">
      {error !== null && <p role="alert">Execution sources unavailable: {error}</p>}
      {data === null && error === null && <p className="muted">Loading execution sources…</p>}
      {data !== null && (
        <>
          <p className="muted">Navigation and manual Refresh reread local sources. The console does not fetch Git remotes. Fetch outside the console, then refresh to see updated remote-tracking observations. Observed state is not integration eligibility or permission to run commands on integration.</p>
          <p className="muted">Global History shows integration history. A task's node History includes that task's events from its selected observation source.</p>
          {data.diagnostics.length > 0 && (
            <section aria-label="Visibility diagnostics">
              <SectionHead label={`Visibility diagnostics · ${data.diagnostics.length}`} />
              <ul>{diagnostics.map((diagnostic, index) => (
                <li key={`${diagnostic.code}:${diagnostic.id ?? ""}:${diagnostic.branch ?? ""}:${index}`}>
                  {diagnostic.id && <><Link to={`/node/task/${encodeURIComponent(diagnostic.id)}`}>{diagnostic.id}</Link>{" · "}</>}
                  {diagnostic.branch && <><span className="mono">{diagnostic.branch}</span>{" · "}</>}
                  <span className="mono">{diagnostic.code}</span>: {diagnostic.message}
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
                    <Link to={`/node/task/${encodeURIComponent(observation.id)}`}>{observation.key || observation.id}</Link>
                    <ExecutionProvenance observation={observation} />
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
