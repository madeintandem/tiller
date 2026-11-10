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

// printScope is the chart note's wording, for the printed page.
function printScope(range: Range, weeks: number): string {
  return weeksNote(weeks, range).replace("Showing", "Charts show");
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
      {/* Whole-project figures, sparkline included: the range never touches them. */}
      <KpiStrip kpis={served.kpis} />
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
            <BurnUpChart report={report} whole={served} range={range} />
          </ChartCard>
        </div>
        <ChartCard>
          <ThroughputChart report={report} whole={served} range={range} />
        </ChartCard>
        <ChartCard>
          <BugsChart report={report} whole={served} range={range} />
        </ChartCard>
        <div className="report-wide">
          <ChartCard>
            <LeadCycleChart report={report} whole={served} range={range} />
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
