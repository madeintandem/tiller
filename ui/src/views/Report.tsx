// The Report page (docs/spec/ui-surface.md, Report): a client-
// facing read of how the project is going over time. Metrics only (D3): a
// header, the KPI strip, and five charts drawn from one /api/report read,
// each of which a reader can enlarge in a dialog (ChartCard). The
// page names no node, person, or branch; the one exception is the off-branch
// notice (D15), which never blocks the charts beneath it.

import type { Report as ReportPayload } from "../api";
import { reportPath } from "../api";
import PageLoading from "../components/PageLoading";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import BugsChart from "./report/BugsChart";
import BurnUpChart from "./report/BurnUpChart";
import ChartCard from "./report/ChartCard";
import EpicProgressChart from "./report/EpicProgressChart";
import { formatAsOf } from "./report/format";
import KpiStrip from "./report/KpiStrip";
import LeadCycleChart from "./report/LeadCycleChart";
import ThroughputChart from "./report/ThroughputChart";
import "./report.css";

export default function Report() {
  useDocumentTitle(titleOf("Report"));
  const { data, error } = useFetch<ReportPayload>(reportPath);
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
      <KpiStrip kpis={data.kpis} />
      <div className="report-charts">
        <div className="report-wide">
          <ChartCard>
            <BurnUpChart report={data} />
          </ChartCard>
        </div>
        <ChartCard>
          <ThroughputChart report={data} />
        </ChartCard>
        <ChartCard>
          <BugsChart report={data} />
        </ChartCard>
        <div className="report-wide">
          <ChartCard>
            <LeadCycleChart report={data} />
          </ChartCard>
        </div>
        <div className="report-wide">
          <ChartCard>
            <EpicProgressChart report={data} />
          </ChartCard>
        </div>
      </div>
    </section>
  );
}
