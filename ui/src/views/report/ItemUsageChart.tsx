// Tokens per work item: how many finished items fall in each token band, one
// column per band in order with its count printed above it. Beneath the plot
// sit the median and the item count, then what the chart leaves out: items
// older than the window and items with no usage recorded.

import type { UsageReport } from "../../api";
import ChartFrame, { MARGIN, YGrid, columnPath } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";
import { formatTokens } from "./format";
import { linearScale, niceTicks } from "./scale";

const COLOR = "var(--st-active)";
// The shortest a non-zero column draws.
const MIN_COLUMN = 2;

export default function ItemUsageChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const { item_bands: bands, items_left_out: left, items_unmeasured: unmeasured, kpis } = usage;
  const peak = Math.max(0, ...bands.map((b) => b.count));
  const noun = (n: number) => (n === 1 ? "item" : "items");
  const median = kpis.median_per_item;
  const note = [
    median === null ? null : `Median ${formatTokens(median)} · ${kpis.items} ${noun(kpis.items)}`,
    left > 0 ? `${left} earlier ${noun(left)} not shown` : null,
    unmeasured > 0 ? `${unmeasured} ${noun(unmeasured)} with no usage recorded` : null,
  ]
    .filter((part) => part !== null)
    .join(" · ");
  return (
    <ChartFrame
      title="Tokens per work item"
      summary={`${kpis.items} work items in ${bands.length} token bands`}
      empty={peak === 0 ? "No work items finished yet" : undefined}
      note={note}
      legend={[{ label: "Work items", swatch: "bar", color: COLOR }]}
      tip={tip}
    >
      {(width, height) => {
        const ticks = niceTicks(peak);
        const y = linearScale([0, ticks[ticks.length - 1]], [height - MARGIN.bottom, MARGIN.top]);
        const band = (width - MARGIN.left - MARGIN.right) / Math.max(bands.length, 1);
        const barWidth = Math.min(48, band * 0.6);
        return (
          <>
            <g className="report-axis">
              <YGrid ticks={ticks} y={y} width={width} />
              <text className="report-tick" x={MARGIN.left} y={MARGIN.top - 6}>
                Work items
              </text>
            </g>
            {bands.map((b, i) => {
              const center = MARGIN.left + band * (i + 0.5);
              const top = y(b.count);
              return (
                <g
                  key={b.label}
                  data-band={i}
                  onPointerMove={(e) =>
                    show(e, [
                      { label: b.label, value: "" },
                      { label: "Work items", value: String(b.count) },
                    ])
                  }
                  onPointerLeave={hide}
                >
                  <rect className="report-hit" x={MARGIN.left + band * i} y={MARGIN.top} width={band} height={height - MARGIN.top - MARGIN.bottom} />
                  {b.count > 0 && (
                    <path d={columnPath(center - barWidth / 2, top, barWidth, Math.max(MIN_COLUMN, y(0) - top))} style={{ fill: COLOR }} />
                  )}
                  <text className="report-row-count" x={center} y={top - 6} textAnchor="middle">
                    {b.count}
                  </text>
                  <text className="report-tick" x={center} y={height - MARGIN.bottom + 18} textAnchor="middle">
                    {b.label}
                  </text>
                </g>
              );
            })}
          </>
        );
      }}
    </ChartFrame>
  );
}
