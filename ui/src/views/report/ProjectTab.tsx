// The Report page's Project tab: the KPI strip and the five charts drawn from
// one /api/report read, each enlargeable in a dialog (ChartCard). The Range
// control sits between the two and trims the weekly charts (sliceWeeks); the
// figures above it cover the whole project and the control says it governs
// the charts below. A print-only line states the range, since the control
// itself does not print.

import type { Report } from "../../api";
import BugsChart from "./BugsChart";
import BurnUpChart from "./BurnUpChart";
import ChartCard from "./ChartCard";
import EpicProgressChart from "./EpicProgressChart";
import KpiStrip from "./KpiStrip";
import LeadCycleChart from "./LeadCycleChart";
import { RANGES, rangeValue, sliceWeeks, weeksNote, type Range } from "./range";
import ThroughputChart from "./ThroughputChart";

function printScope(range: Range, weeks: number): string {
  if (weeks < 1) return "";
  if (range === "all") return `Charts show all ${weeks} ${weeks === 1 ? "week" : "weeks"}`;
  return weeksNote(weeks).replace("Showing", "Charts show");
}

export default function ProjectTab({
  report: served,
  range,
  onRange,
}: {
  report: Report;
  range: Range;
  onRange: (range: string) => void;
}) {
  const report = sliceWeeks(served, range);
  const scope = printScope(range, report.weeks.length);
  return (
    <>
      <KpiStrip kpis={report.kpis} />
      <div className="report-range">
        <label htmlFor="report-range-select">Range</label>
        <select id="report-range-select" value={rangeValue(range)} onChange={(e) => onRange(e.target.value)}>
          {RANGES.map((r) => (
            <option key={r.value} value={r.value}>
              {r.label}
            </option>
          ))}
        </select>
        <span className="report-range-hint">Applies to the charts below</span>
      </div>
      {scope && <p className="report-print-scope">{scope}</p>}
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
