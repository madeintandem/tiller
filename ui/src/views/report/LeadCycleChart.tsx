// Lead and cycle time (D8): one dumbbell per completed feature at its
// completion date. A filled dot marks cycle time and a hollow dot lead time,
// joined by a hairline whose length is the time spent approved but waiting.
// The rolling medians over the last five features run as a solid (cycle) and
// a dashed (lead) line. Filled vs hollow and solid vs dashed carry identity,
// not hue.

import type { Report, ReportFeatureTime } from "../../api";
import ChartFrame, { MedianLine, WeekAxis, weekPlot } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";
import { formatDays } from "./format";

const CYCLE = "var(--st-active)";
const LEAD = "var(--st-ready)";
// The hover target around a dumbbell: wider than the 8px dots it carries.
const HIT = 24;

export default function LeadCycleChart({ report }: { report: Report }) {
  const { tip, show, hide } = useTooltip();
  const { feature_times: features } = report;
  const latest = features[features.length - 1];
  const empty = features.length === 0 ? "No features completed yet" : undefined;
  const summary = latest
    ? `${features.length} features completed; rolling median cycle ${formatDays(latest.cycle_median_days)}, lead ${formatDays(latest.lead_median_days)}`
    : "";

  return (
    <ChartFrame
      title="Lead and cycle time"
      summary={summary}
      empty={empty}
      legend={[
        { label: "Cycle time", swatch: "dot", color: CYCLE },
        { label: "Lead time", swatch: "hollow", color: LEAD },
        { label: "Median cycle, last 5", swatch: "solid", color: CYCLE },
        { label: "Median lead, last 5", swatch: "dashed", color: LEAD },
      ]}
      tip={tip}
    >
      {(width, height) => {
        const max = Math.max(
          ...features.map((f) => Math.max(f.cycle_days, f.lead_days, f.cycle_median_days, f.lead_median_days)),
        );
        const plot = weekPlot(report, width, height, max);
        const median = (pick: (f: ReportFeatureTime) => number) =>
          features.map((f) => `${plot.date(f.completed_at)},${plot.y(pick(f))}`).join(" ");
        return (
          <>
            <WeekAxis plot={plot} unit="days" />
            <MedianLine series="cycle-median" points={median((f) => f.cycle_median_days)} color={CYCLE} />
            <MedianLine series="lead-median" points={median((f) => f.lead_median_days)} color={LEAD} dashed />
            {features.map((f, i) => {
              const x = plot.date(f.completed_at);
              const cycleY = plot.y(f.cycle_days);
              const leadY = plot.y(f.lead_days);
              const top = Math.min(cycleY, leadY);
              const bottom = Math.max(cycleY, leadY);
              return (
                <g
                  key={i}
                  data-dumbbell
                  onPointerMove={(e) =>
                    show(e, [
                      { label: f.title, value: "" },
                      { label: "Cycle", value: formatDays(f.cycle_days) },
                      { label: "Lead", value: formatDays(f.lead_days) },
                    ])
                  }
                  onPointerLeave={hide}
                >
                  <rect className="report-hit" x={x - HIT / 2} y={top - HIT / 2} width={HIT} height={bottom - top + HIT} />
                  <line className="report-join" x1={x} x2={x} y1={cycleY} y2={leadY} />
                  <circle className="report-dot report-dot-hollow" data-series="lead" cx={x} cy={leadY} r="4" style={{ stroke: LEAD }} />
                  <circle className="report-dot" data-series="cycle" cx={x} cy={cycleY} r="5" style={{ fill: CYCLE }} />
                </g>
              );
            })}
          </>
        );
      }}
    </ChartFrame>
  );
}
