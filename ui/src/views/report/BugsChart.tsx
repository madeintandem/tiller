// Bugs reported vs fixed (D11): paired weekly bars, Reported on the left and
// Fixed on the right of each week, with the count still open at each week's
// end as a solid line with point markers. Reported and Fixed are told apart by
// their fixed position in each pair and the legend text beside their hue, and
// Open is a line rather than a bar.

import type { Report } from "../../api";
import ChartFrame, { WeekAxis, WeekHits, columnPath, weekPlot, weekRows } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";

const REPORTED = "var(--noun-bug)";
const FIXED = "var(--st-done)";
const OPEN = "var(--heading)";
// The surface gap between a week's two bars.
const GAP = 2;
// The narrowest a bar draws, so a long axis still shows every count.
const MIN_BAR = 1;

export default function BugsChart({ report }: { report: Report }) {
  const { tip, show, hide } = useTooltip();
  const { weeks, bugs: series } = report;
  const reported = series.reduce((sum, w) => sum + w.reported, 0);
  const fixed = series.reduce((sum, w) => sum + w.fixed, 0);
  const open = series[series.length - 1]?.open ?? 0;
  const empty = series.every((w) => w.reported === 0) ? "No bugs reported yet" : undefined;

  return (
    <ChartFrame
      title="Bugs reported vs fixed"
      summary={`${reported} bugs reported, ${fixed} fixed, ${open} open`}
      empty={empty}
      legend={[
        { label: "Reported", swatch: "bar", color: REPORTED },
        { label: "Fixed", swatch: "bar", color: FIXED },
        { label: "Open", swatch: "solid", color: OPEN },
      ]}
      tip={tip}
    >
      {(width, height) => {
        const plot = weekPlot(report, width, height, Math.max(...series.map((w) => Math.max(w.reported, w.fixed, w.open))));
        // A pair takes 70% of its week. When that cannot hold the gap and two
        // 1px bars, the gap goes and the bars touch. Under 2px per week the 1px
        // floor goes too, as in ThroughputChart, so bars never leave their week.
        const pair = plot.band * 0.7;
        const gap = pair - GAP >= 2 * MIN_BAR ? GAP : 0;
        const barWidth = plot.band < 2 * MIN_BAR ? pair / 2 : Math.max(MIN_BAR, Math.min(16, (pair - gap) / 2));
        const baseline = plot.y(0);
        const bar = (series: string, x: number, value: number, color: string) => {
          if (value === 0) return null;
          const y = plot.y(value);
          return <path data-series={series} d={columnPath(x, y, barWidth, baseline - y)} style={{ fill: color }} />;
        };
        const openPoints = series.map((w, i) => `${plot.x(i)},${plot.y(w.open)}`).join(" ");
        return (
          <>
            <WeekAxis plot={plot} />
            {series.map((w, i) => (
              <g key={weeks[i]}>
                {bar("reported", plot.x(i) - gap / 2 - barWidth, w.reported, REPORTED)}
                {bar("fixed", plot.x(i) + gap / 2, w.fixed, FIXED)}
              </g>
            ))}
            <polyline className="report-line" data-series="open" points={openPoints} style={{ stroke: OPEN }} />
            {series.map((w, i) => (
              <circle key={weeks[i]} className="report-dot" cx={plot.x(i)} cy={plot.y(w.open)} r="4" style={{ fill: OPEN }} />
            ))}
            <WeekHits
              plot={plot}
              onMove={(i, e) =>
                show(
                  e,
                  weekRows(weeks, i, [
                    { label: "Reported", value: String(series[i].reported) },
                    { label: "Fixed", value: String(series[i].fixed) },
                    { label: "Open", value: String(series[i].open) },
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
