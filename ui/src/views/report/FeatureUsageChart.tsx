// Tokens per feature: one filled dot per completed feature at its completion
// date on the shared week axis, with a solid rolling-median line through
// them. The y axis is logarithmic, because features span orders of magnitude;
// ticks sit at powers of ten. Dots and the line differ by shape and legend
// text, not hue alone.

import type { UsageFeature, UsageReport } from "../../api";
import ChartFrame, { MARGIN, MedianLine, WeekAxis, weekPlot } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";
import { formatTokens } from "./format";
import { decadeTicks, logScale } from "./scale";

const COLOR = "var(--st-active)";
// The hover target around a dot: wider than the 5px dot it carries.
const HIT = 24;

export default function FeatureUsageChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const features: UsageFeature[] = [...usage.features].sort((a, b) => Date.parse(a.done) - Date.parse(b.done));
  const latest = features[features.length - 1];
  const left = usage.features_left_out;
  return (
    <ChartFrame
      title="Tokens per finished feature"
      summary={latest ? `${features.length} features completed; rolling median ${formatTokens(latest.median)}` : ""}
      empty={features.length === 0 ? "No features completed yet" : undefined}
      note={left > 0 ? `${left} earlier ${left === 1 ? "feature" : "features"} not shown` : undefined}
      legend={[
        { label: "Feature total", swatch: "dot", color: COLOR },
        { label: "Rolling median", swatch: "solid", color: COLOR },
      ]}
      tip={tip}
    >
      {(width, height) => {
        const ticks = decadeTicks(features.flatMap((f) => [f.total, f.median]));
        const base = weekPlot(usage, width, height, 0);
        const plot = { ...base, ticks, y: logScale([ticks[0], ticks[ticks.length - 1]], [height - MARGIN.bottom, MARGIN.top]) };
        return (
          <>
            <WeekAxis plot={plot} unit="Tokens (log scale)" format={formatTokens} />
            <MedianLine series="feature-median" points={features.map((f) => `${plot.date(f.done)},${plot.y(f.median)}`).join(" ")} color={COLOR} />
            {features.map((f, i) => {
              const x = plot.date(f.done);
              const y = plot.y(f.total);
              return (
                <g
                  key={i}
                  onPointerMove={(e) =>
                    show(e, [
                      { label: f.title, value: "" },
                      { label: "Tokens", value: formatTokens(f.total) },
                    ])
                  }
                  onPointerLeave={hide}
                >
                  <rect className="report-hit" x={x - HIT / 2} y={y - HIT / 2} width={HIT} height={HIT} />
                  <circle className="report-dot" data-feature={i} cx={x} cy={y} r="5" style={{ fill: COLOR }} />
                </g>
              );
            })}
          </>
        );
      }}
    </ChartFrame>
  );
}
