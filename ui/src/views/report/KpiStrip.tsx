// The Report page's KPI strip (D13): five figures a client reads first, in a
// fixed order. Each tile is a term and its value in one description list, so
// a screen reader hears the label before the number.

import type { ReactNode } from "react";

import type { ReportKPIs } from "../../api";
import { formatDays } from "./format";
import Sparkline from "./Sparkline";

export default function KpiStrip({ kpis }: { kpis: ReportKPIs }) {
  const cycle = kpis.median_cycle_days;
  const lead = kpis.median_lead_days;
  const completed = `${kpis.completed_features} ${kpis.completed_features === 1 ? "feature" : "features"}`;
  return (
    <dl className="report-kpis">
      <Tile
        label="Features done"
        value={
          <>
            {kpis.features_done} <span className="report-kpi-of">of {kpis.features_total}</span>
          </>
        }
      />
      <Tile label="Shipped, last 7 days" value={kpis.shipped_last_7_days} />
      <Tile
        label="Median cycle time"
        value={cycle === null ? "Not yet" : formatDays(cycle)}
        muted={cycle === null}
        sub={cycle !== null && lead !== null ? `lead ${formatDays(lead)} · ${completed}` : undefined}
      />
      <Tile label="Open bugs" value={kpis.open_bugs} />
      <Tile
        label="Open questions"
        value={kpis.open_questions}
        sub={`${kpis.answered_questions} answered`}
        trend={<Sparkline values={kpis.open_questions_by_week} label="Open questions by week" />}
      />
    </dl>
  );
}

interface TileProps {
  label: string;
  value: ReactNode;
  // A value that is a placeholder ("Not yet") reads in a quieter ink.
  muted?: boolean;
  sub?: string;
  trend?: ReactNode;
}

export function Tile({ label, value, muted = false, sub, trend }: TileProps) {
  return (
    <div className="report-kpi">
      <dt>{label}</dt>
      <dd className={muted ? "report-kpi-value is-muted" : "report-kpi-value"}>
        <span className="report-kpi-number">{value}</span>
        {trend}
      </dd>
      {sub && <dd className="report-kpi-sub">{sub}</dd>}
    </div>
  );
}
