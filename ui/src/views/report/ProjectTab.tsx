// The Report page's Project tab: the KPI strip and the five charts drawn from
// one /api/report read, each enlargeable in a dialog (ChartCard).

import type { Report } from "../../api";
import BugsChart from "./BugsChart";
import BurnUpChart from "./BurnUpChart";
import ChartCard from "./ChartCard";
import EpicProgressChart from "./EpicProgressChart";
import KpiStrip from "./KpiStrip";
import LeadCycleChart from "./LeadCycleChart";
import ThroughputChart from "./ThroughputChart";

export default function ProjectTab({ report }: { report: Report }) {
  return (
    <>
      <KpiStrip kpis={report.kpis} />
      <div className="report-charts">
        <div className="report-wide">
          <ChartCard>
            <BurnUpChart report={report} />
          </ChartCard>
        </div>
        <ChartCard>
          <ThroughputChart report={report} />
        </ChartCard>
        <ChartCard>
          <BugsChart report={report} />
        </ChartCard>
        <div className="report-wide">
          <ChartCard>
            <LeadCycleChart report={report} />
          </ChartCard>
        </div>
        <div className="report-wide">
          <ChartCard>
            <EpicProgressChart report={report} />
          </ChartCard>
        </div>
      </div>
    </>
  );
}
