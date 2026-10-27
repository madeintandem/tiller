// The Agent use tab's two weekly token charts: Cache reads on their own,
// because they dwarf everything else, and Fresh input, cache writes and output
// stacked bottom-up. A week with partial coverage keeps its bar under the
// hatched band the page already uses for a week in progress, and the legend
// gains "Partly measured" only when such a week is present. An unmeasured
// week is never drawn as zero: its column is an empty dotted outline, the
// idiom Epic progress uses for "Not yet approved", and the legend gains "Not
// measured" only when such a week is present. Series are told apart by
// legend text and stack position, and Cache writes is also hatched.

import type { UsageReport, UsageWeek } from "../../api";
import ChartFrame, {
  DOTTED,
  HatchPattern,
  PartialPattern,
  StackedColumn,
  WeekAxis,
  WeekHits,
  columnPath,
  usePatternId,
  weekPlot,
  weekRows,
  type LegendEntry,
  type WeekPlot,
} from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";
import { formatTokens } from "./format";

const CACHE_READS = "var(--st-active)";
const PARTLY: LegendEntry = { label: "Partly measured", swatch: "partial", color: "var(--ink-3)" };
const NOT_MEASURED: LegendEntry = { label: "Not measured", swatch: "outline", color: "var(--ink-3)" };

// The shortest a non-zero cache-reads bar draws.
const MIN_SEGMENT = 2;

// Bottom of the stack first.
const KINDS: { key: keyof UsageWeek; label: string; color: string; hatched?: boolean }[] = [
  { key: "input", label: "Fresh input", color: "var(--st-active)" },
  { key: "cache_write", label: "Cache writes", color: "var(--noun-bug)", hatched: true },
  { key: "output", label: "Output", color: "var(--noun-chore)" },
];

const measured = (usage: UsageReport, i: number) => usage.coverage[i] !== "none" && usage.weekly[i] !== undefined;
const hasPartial = (usage: UsageReport) => usage.coverage.includes("partial");
const hasUnmeasured = (usage: UsageReport) => usage.coverage.includes("none");

// withCoverage adds the partial and unmeasured legend entries the chart
// draws, each only when such a week is present.
function withCoverage(legend: LegendEntry[], usage: UsageReport): LegendEntry[] {
  return [...legend, ...(hasPartial(usage) ? [PARTLY] : []), ...(hasUnmeasured(usage) ? [NOT_MEASURED] : [])];
}

// UnmeasuredColumns draws each unmeasured week as an empty dotted column the
// height of the plot, so the week reads as not measured rather than zero.
function UnmeasuredColumns({ usage, plot, barWidth }: { usage: UsageReport; plot: WeekPlot; barWidth: number }) {
  if (!hasUnmeasured(usage)) return null;
  const top = plot.y(plot.ticks[plot.ticks.length - 1]);
  return (
    <g>
      {usage.coverage.map((c, i) =>
        c === "none" ? (
          <rect
            key={usage.weeks[i]}
            data-not-measured={i}
            className="report-ghost-track"
            x={plot.x(i) - barWidth / 2 + DOTTED.strokeWidth / 2}
            y={top + DOTTED.strokeWidth / 2}
            width={Math.max(barWidth - DOTTED.strokeWidth, 0)}
            height={Math.max(plot.y(0) - top - DOTTED.strokeWidth, 0)}
            rx={2}
            {...DOTTED}
          />
        ) : null,
      )}
    </g>
  );
}

// PartialBands hatches each partially measured week's band.
function PartialBands({ usage, plot }: { usage: UsageReport; plot: WeekPlot }) {
  const id = usePatternId("usage-partial");
  if (!hasPartial(usage)) return null;
  const top = plot.y(plot.ticks[plot.ticks.length - 1]);
  return (
    <g>
      <defs>
        <PartialPattern id={id} />
      </defs>
      {usage.coverage.map((c, i) =>
        c === "partial" ? (
          <rect
            key={usage.weeks[i]}
            data-partial="true"
            x={plot.x(i) - plot.band / 2}
            y={top}
            width={plot.band}
            height={plot.y(0) - top}
            fill={`url(#${id})`}
          />
        ) : null,
      )}
    </g>
  );
}

// tipRows leads a partial week with "Partly measured" and an unmeasured week
// with "Not measured", then lists the series.
function tipRows(usage: UsageReport, i: number, series: { label: string; value: number }[]) {
  if (!measured(usage, i)) return weekRows(usage.weeks, i, [{ label: "Not measured", value: "" }]);
  const rows = series.map((s) => ({ label: s.label, value: formatTokens(s.value) }));
  const lead = usage.coverage[i] === "partial" ? [{ label: "Partly measured", value: "" }] : [];
  return weekRows(usage.weeks, i, [...lead, ...rows]);
}

// The tallest measured week, 0 when none.
function peak(usage: UsageReport, weight: (w: UsageWeek) => number): number {
  return usage.weekly.reduce((max, w, i) => (measured(usage, i) ? Math.max(max, weight(w)) : max), 0);
}

export function CacheReadsChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const empty = peak(usage, (w) => w.cache_read) === 0 ? "No cache reads measured yet" : undefined;
  return (
    <ChartFrame
      title="Cache reads per week"
      summary={`${formatTokens(usage.kpis.cache_read)} cache reads over ${usage.weeks.length} weeks`}
      empty={empty}
      legend={withCoverage([{ label: "Cache reads", swatch: "bar", color: CACHE_READS }], usage)}
      tip={tip}
    >
      {(width, height) => {
        const plot = weekPlot(usage, width, height, peak(usage, (w) => w.cache_read));
        const barWidth = Math.min(24, plot.band * 0.6);
        return (
          <>
            <WeekAxis plot={plot} format={formatTokens} />
            <PartialBands usage={usage} plot={plot} />
            <UnmeasuredColumns usage={usage} plot={plot} barWidth={barWidth} />
            {usage.weekly.map((w, i) => {
              if (!measured(usage, i) || w.cache_read === 0) return null;
              const y = plot.y(w.cache_read);
              return (
                <path
                  key={usage.weeks[i]}
                  data-week-bar={i}
                  d={columnPath(plot.x(i) - barWidth / 2, y, barWidth, Math.max(MIN_SEGMENT, plot.y(0) - y))}
                  style={{ fill: CACHE_READS }}
                />
              );
            })}
            <WeekHits
              plot={plot}
              onMove={(i, e) => show(e, tipRows(usage, i, [{ label: "Cache reads", value: usage.weekly[i]?.cache_read ?? 0 }]))}
              onLeave={hide}
            />
          </>
        );
      }}
    </ChartFrame>
  );
}

const fresh = (w: UsageWeek) => w.input + w.cache_write + w.output;

export function FreshTokensChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const hatch = usePatternId("report-hatch");
  const { kpis } = usage;
  const empty = peak(usage, fresh) === 0 ? "No fresh tokens measured yet" : undefined;
  const legend: LegendEntry[] = KINDS.map((k) => ({ label: k.label, swatch: k.hatched ? "hatched" : "bar", color: k.color }));
  return (
    <ChartFrame
      title="Fresh tokens per week"
      summary={`${formatTokens(kpis.input + kpis.cache_write)} fresh input and cache writes, ${formatTokens(kpis.output)} output over ${usage.weeks.length} weeks`}
      empty={empty}
      legend={withCoverage(legend, usage)}
      tip={tip}
    >
      {(width, height) => {
        const plot = weekPlot(usage, width, height, peak(usage, fresh));
        const barWidth = Math.min(24, plot.band * 0.6);
        return (
          <>
            <WeekAxis plot={plot} format={formatTokens} />
            <PartialBands usage={usage} plot={plot} />
            <UnmeasuredColumns usage={usage} plot={plot} barWidth={barWidth} />
            <defs>
              {KINDS.filter((k) => k.hatched).map((k) => (
                <HatchPattern key={k.key} id={hatch} color={k.color} />
              ))}
            </defs>
            {usage.weekly.map((w, i) =>
              measured(usage, i) ? (
                <StackedColumn
                  key={usage.weeks[i]}
                  plot={plot}
                  x={plot.x(i) - barWidth / 2}
                  w={barWidth}
                  hatchId={hatch}
                  week={i}
                  segments={KINDS.map((k) => ({ key: k.key, value: w[k.key], color: k.color, hatched: k.hatched }))}
                />
              ) : null,
            )}
            <WeekHits
              plot={plot}
              onMove={(i, e) => {
                const w = usage.weekly[i];
                show(
                  e,
                  tipRows(usage, i, w ? [...KINDS.map((k) => ({ label: k.label, value: w[k.key] })), { label: "Total", value: fresh(w) }] : []),
                );
              }}
              onLeave={hide}
            />
          </>
        );
      }}
    </ChartFrame>
  );
}
