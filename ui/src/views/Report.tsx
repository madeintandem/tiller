// The Report page (docs/spec/ui-surface.md, Report): a client-
// facing read of how the project is going over time. Metrics only (D3): a
// header over two tabs, Project (the KPI strip and five charts drawn from one
// /api/report read, each enlargeable in a dialog) and Agent use. With
// usage.capture off there are no tabs, only the Project view. The
// page names no node, person, or branch; the one exception is the off-branch
// notice (D15), which never blocks the charts beneath it.

import { useSearchParams } from "react-router-dom";
import type { Report as ReportPayload } from "../api";
import { reportPath } from "../api";
import NodeTabs from "../components/NodeTabs";
import PageLoading from "../components/PageLoading";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import AgentUseTab from "./report/AgentUseTab";
import { formatAsOf } from "./report/format";
import ProjectTab from "./report/ProjectTab";
import "./report.css";

export default function Report() {
  useDocumentTitle(titleOf("Report"));
  const { data, error } = useFetch<ReportPayload>(reportPath);
  const [search, setSearch] = useSearchParams();
  // ?tab= picks the view; Project is the default and leaves the URL bare. An
  // unknown value reads as Project through NodeTabs' first-tab rule.
  const selectTab = (key: string) => {
    const next = new URLSearchParams(search);
    if (key === "project") {
      next.delete("tab");
    } else {
      next.set("tab", key);
    }
    setSearch(next, { replace: true });
  };
  if (error !== null) {
    return (
      <section className="report-page">
        <h1>Report</h1>
        <p className="error">{error}</p>
      </section>
    );
  }
  if (data === null) return <PageLoading title="Report" />;
  return (
    <section className="report-page">
      <header className="report-head">
        <div>
          <h1>{data.project}</h1>
          <p className="report-subtitle">Project report</p>
        </div>
        <p className="report-asof">As of {formatAsOf(data.as_of)}</p>
      </header>
      {data.off_branch && (
        <p className="report-notice" role="note">
          Showing branch {data.off_branch.branch}, not {data.off_branch.integration}. Numbers include unmerged work.
        </p>
      )}
      {data.agent_use ? (
        <NodeTabs
          label="Report view"
          active={search.get("tab") ?? "project"}
          onSelect={selectTab}
          tabs={[
            { key: "project", label: "Project", render: () => <ProjectTab report={data} /> },
            { key: "agent-use", label: "Agent use", render: () => <AgentUseTab /> },
          ]}
        />
      ) : (
        <ProjectTab report={data} />
      )}
    </section>
  );
}
