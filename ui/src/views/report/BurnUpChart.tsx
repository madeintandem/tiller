// Feature burn-up (D6): approved scope against features done at each week's
// end. Scope is a dashed muted line and Done a solid line, the gap between
// them shaded as the work still to do, and both lines named at their right
// end so the pair never relies on color.

import type { Report } from "../../api";
import ChartFrame, { WeekAxis, WeekHits, tint, weekPlot, weekRows } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";

const SCOPE = "var(--ink-3)";
const DONE = "var(--st-done)";
// Two end labels closer than this are spread apart around their midpoint.
const LABEL_GAP = 14;

export default function BurnUpChart({ report }: { report: Report }) {
  const { tip, show, hide } = useTooltip();
  const { weeks, burn_up: series } = report;
  const last = series[series.length - 1] ?? { scope: 0, done: 0 };
  const empty = series.every((w) => w.scope === 0) ? "No approved scope yet" : undefined;

  return (
    <ChartFrame
      title="Feature burn-up"
      summary={`${last.done} of ${last.scope} approved features done`}
      empty={empty}
      legend={[
        { label: "Scope", swatch: "dashed", color: SCOPE },
        { label: "Done", swatch: "solid", color: DONE },
      ]}
      tip={tip}
    >
      {(width, height) => {
        const plot = weekPlot(report, width, height, Math.max(...series.map((w) => Math.max(w.scope, w.done))));
        const scopePoints = series.map((w, i) => `${plot.x(i)},${plot.y(w.scope)}`);
        const donePoints = series.map((w, i) => `${plot.x(i)},${plot.y(w.done)}`);
        const endX = plot.x(series.length - 1);
        let scopeY = plot.y(last.scope);
        let doneY = plot.y(last.done);
        if (doneY - scopeY < LABEL_GAP) {
          const mid = (scopeY + doneY) / 2;
          scopeY = mid - LABEL_GAP / 2;
          doneY = mid + LABEL_GAP / 2;
        }
        return (
          <>
            <WeekAxis plot={plot} />
            <polygon
              points={[...scopePoints, ...donePoints.slice().reverse()].join(" ")}
              style={{ fill: tint(SCOPE) }}
            />
            <polyline
              className="report-line"
              data-series="scope"
              points={scopePoints.join(" ")}
              strokeDasharray="6 4"
              style={{ stroke: SCOPE }}
            />
            <polyline className="report-line" data-series="done" points={donePoints.join(" ")} style={{ stroke: DONE }} />
            <circle className="report-dot" cx={endX} cy={plot.y(last.done)} r="4" style={{ fill: DONE }} />
            <text className="report-end-label" x={endX + 10} y={scopeY} dominantBaseline="middle">
              Scope <tspan className="report-value">{last.scope}</tspan>
            </text>
            <text className="report-end-label" x={endX + 10} y={doneY} dominantBaseline="middle">
              Done <tspan className="report-value">{last.done}</tspan>
            </text>
            <WeekHits
              plot={plot}
              onMove={(i, e) =>
                show(
                  e,
                  weekRows(weeks, i, [
                    { label: "Scope", value: String(series[i].scope) },
                    { label: "Done", value: String(series[i].done) },
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
