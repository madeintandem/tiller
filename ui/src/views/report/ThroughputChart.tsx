// Throughput (D7): leaf work finished each week, stacked bottom-up as Feature
// work, Fixes, and Upkeep. The stack order is fixed and every kind is named
// in the legend and the tooltip. Fixes is also hatched, in the bars and the
// legend alike, because its violet sits too close to Feature work's blue for
// color-blind readers.

import type { Report, ReportThroughputWeek } from "../../api";
import ChartFrame, {
  HatchPattern,
  WeekAxis,
  WeekHits,
  columnPath,
  usePatternId,
  weekPlot,
  weekRows,
  type LegendEntry,
} from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";

// Bottom of the stack first.
const KINDS: { key: keyof ReportThroughputWeek; label: string; color: string; hatched?: boolean }[] = [
  { key: "feature_work", label: "Feature work", color: "var(--st-active)" },
  { key: "fixes", label: "Fixes", color: "var(--noun-bug)", hatched: true },
  { key: "upkeep", label: "Upkeep", color: "var(--noun-chore)" },
];
const LEGEND: LegendEntry[] = KINDS.map((k) => ({
  label: k.label,
  swatch: k.hatched ? "hatched" : "bar",
  color: k.color,
}));
// The surface gap between stacked segments.
const GAP = 2;
// The shortest a non-zero segment draws, so a count of 1 beside a large week
// never vanishes. Segments stack on the one below them, so a raised segment
// lifts the rest of its bar rather than overlapping it.
const MIN_SEGMENT = 2;

function total(w: ReportThroughputWeek): number {
  return w.feature_work + w.fixes + w.upkeep;
}

export default function ThroughputChart({ report }: { report: Report }) {
  const { tip, show, hide } = useTooltip();
  const hatch = usePatternId("report-hatch");
  const { weeks, throughput: series } = report;
  const shipped = series.reduce((sum, w) => sum + total(w), 0);
  const empty = shipped === 0 ? "Nothing shipped yet" : undefined;

  return (
    <ChartFrame
      title="Throughput"
      summary={`${shipped} items shipped over ${weeks.length} weeks`}
      empty={empty}
      legend={LEGEND}
      tip={tip}
    >
      {(width, height) => {
        const plot = weekPlot(report, width, height, Math.max(...series.map(total)));
        const barWidth = Math.min(24, plot.band * 0.6);
        return (
          <>
            <WeekAxis plot={plot} />
            <defs>
              {KINDS.filter((k) => k.hatched).map((k) => (
                <HatchPattern key={k.key} id={hatch} color={k.color} />
              ))}
            </defs>
            {series.map((w, i) => {
              const x = plot.x(i) - barWidth / 2;
              const top = KINDS.map((k) => w[k.key] > 0).lastIndexOf(true);
              let below = 0;
              let cursor = plot.y(0);
              return (
                <g key={weeks[i]}>
                  {KINDS.map((k, s) => {
                    const value = w[k.key];
                    if (value === 0) return null;
                    const bottom = below > 0 ? cursor - GAP : cursor;
                    below += value;
                    const h = Math.max(MIN_SEGMENT, bottom - plot.y(below));
                    const y = bottom - h;
                    cursor = y;
                    const paint = k.hatched ? { fill: `url(#${hatch})` } : { style: { fill: k.color } };
                    return s === top ? (
                      <path key={k.key} data-kind={k.key} d={columnPath(x, y, barWidth, h)} {...paint} />
                    ) : (
                      <rect key={k.key} data-kind={k.key} x={x} y={y} width={barWidth} height={h} {...paint} />
                    );
                  })}
                </g>
              );
            })}
            <WeekHits
              plot={plot}
              onMove={(i, e) =>
                show(
                  e,
                  weekRows(weeks, i, [
                    ...KINDS.map((k) => ({ label: k.label, value: String(series[i][k.key]) })),
                    { label: "Total", value: String(total(series[i])) },
                  ]),
                )
              }
              onLeave={hide}
            />
          </>
        );
      }}
    </ChartFrame>
  );
}
