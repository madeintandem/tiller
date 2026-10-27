// The Agent use tab's two stage charts, drawn in the Epic progress idiom
// (a title over a bar per row, a count at the row's end). Tokens by stage is
// one bar per stage against the largest, with the total and the share of it
// that was cache reads (n/a, muted, for a stage with none). Model by stage
// splits each stage's tokens into a 100 percent bar by model, so which model
// carried which stage reads at a glance; a stage with no tokens has no row.
// A model keeps one color in every chart (MODEL_COLORS, by
// index into usage.models), is named in the legend, carries a percent label
// where its segment is wide enough, and is spelled out on hover, so hue is
// never the only cue.

import type { UsageReport } from "../../api";
import ChartFrame, { BAR_HEIGHT, ROW_HEIGHT, SegmentedBar, type LegendEntry } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";
import { formatPercent, formatTokens } from "./format";

export const MODEL_COLORS = ["var(--st-active)", "var(--st-in_review)", "var(--st-specced)", "var(--st-done)", "var(--ink-3)"];

// The label ink modifier for each MODEL_COLORS fill (see charts.css): the gold
// in_review and gray fills read at 4.5:1 only with their own ink.
const LABEL_CLASS = [undefined, "report-seg-label--on-gold", undefined, undefined, "report-seg-label--on-gray"];
const STAGE_COLOR = "var(--accent)";
// A segment narrower than this share of its bar carries no inline label.
const LABEL_FLOOR = 0.12;
// Room at a row's end for a token total, and beside it the cache-read share.
const TOTAL_WIDTH = 64;
const SHARE_WIDTH = 64;
// The column heading above the first stage row.
const HEAD = 16;
// The cache-read share of a stage with no tokens.
const NO_SHARE = "n/a";

const modelColor = (i: number) => MODEL_COLORS[Math.min(i, MODEL_COLORS.length - 1)];

export function StageChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const { stages } = usage;
  const largest = Math.max(0, ...stages.map((s) => s.total));
  const empty = largest === 0 ? "No stage usage measured yet" : undefined;
  return (
    <ChartFrame
      title="Tokens by stage"
      summary={`${formatTokens(usage.kpis.total)} tokens across ${stages.length} stages`}
      empty={empty}
      legend={[{ label: "Tokens", swatch: "bar", color: STAGE_COLOR }]}
      height={HEAD + stages.length * ROW_HEIGHT + 4}
      tip={tip}
    >
      {(width) => {
        const barWidth = Math.max(width - TOTAL_WIDTH - SHARE_WIDTH, 0);
        return (
          <>
            <text className="report-tick" x={width} y={HEAD - 6} textAnchor="end">
              Cache read share
            </text>
            {stages.map((stage, i) => {
              const y = HEAD + i * ROW_HEIGHT;
              // A stage with no tokens has no cache-read share: unknown, not 0%.
              const share = stage.total > 0 ? formatPercent(stage.cache_read, stage.total) : NO_SHARE;
              return (
                <g
                  key={stage.key}
                  data-stage={stage.key}
                  onPointerMove={(e) =>
                    show(e, [
                      { label: stage.label, value: "" },
                      { label: "Tokens", value: formatTokens(stage.total) },
                      { label: "Cache read share", value: share },
                    ])
                  }
                  onPointerLeave={hide}
                >
                  <rect className="report-hit" x={0} y={y} width={width} height={ROW_HEIGHT} />
                  <text className="report-row-title" x={0} y={y + 14}>
                    {stage.label}
                  </text>
                  <SegmentedBar
                    y={y + 22}
                    width={barWidth}
                    scale={largest}
                    segments={[{ key: stage.key, value: stage.total, color: STAGE_COLOR }]}
                  />
                  <text className="report-row-count" x={width - SHARE_WIDTH} y={y + 22 + BAR_HEIGHT / 2} textAnchor="end" dominantBaseline="middle">
                    {formatTokens(stage.total)}
                  </text>
                  <text
                    className={share === NO_SHARE ? "report-row-count report-row-count--muted" : "report-row-count"}
                    x={width}
                    y={y + 22 + BAR_HEIGHT / 2}
                    textAnchor="end"
                    dominantBaseline="middle"
                  >
                    {share}
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

export function ModelStageChart({ usage }: { usage: UsageReport }) {
  const { tip, show, hide } = useTooltip();
  const labels = new Map(usage.stages.map((s) => [s.key, s.label]));
  const rows = usage.model_stages
    .map((row) => ({ ...row, label: labels.get(row.stage) ?? row.stage, total: row.totals.reduce((sum, n) => sum + n, 0) }))
    .filter((row) => row.total > 0);
  const empty = rows.length === 0 ? "No stage usage measured yet" : undefined;
  const legend: LegendEntry[] = usage.models.map((model, i) => ({ label: model, swatch: "bar", color: modelColor(i) }));
  return (
    <ChartFrame
      title="Model by stage"
      summary={`Share of each stage's tokens by model: ${usage.models.join(", ")}`}
      empty={empty}
      legend={legend}
      height={rows.length * ROW_HEIGHT + 4}
      tip={tip}
    >
      {(width) => {
        const barWidth = Math.max(width - TOTAL_WIDTH, 0);
        return rows.map((row, i) => {
          const y = i * ROW_HEIGHT;
          const segments = usage.models.map((model, m) => {
            const share = row.totals[m] / row.total;
            return {
              key: model,
              value: row.totals[m],
              color: modelColor(m),
              label: share >= LABEL_FLOOR ? `${Math.round(share * 100)}%` : undefined,
              labelClass: LABEL_CLASS[Math.min(m, MODEL_COLORS.length - 1)],
            };
          });
          return (
            <g
              key={row.stage}
              data-stage={row.stage}
              onPointerMove={(e) =>
                show(e, [
                  { label: row.label, value: "" },
                  ...usage.models.map((model, m) => ({
                    label: model,
                    value: `${Math.round((row.totals[m] / row.total) * 100)}% · ${formatTokens(row.totals[m])}`,
                  })),
                ])
              }
              onPointerLeave={hide}
            >
              <rect className="report-hit" x={0} y={y} width={width} height={ROW_HEIGHT} />
              <text className="report-row-title" x={0} y={y + 14}>
                {row.label}
              </text>
              <SegmentedBar y={y + 22} width={barWidth} segments={segments} />
              <text className="report-row-count" x={width} y={y + 22 + BAR_HEIGHT / 2} textAnchor="end" dominantBaseline="middle">
                {formatTokens(row.total)}
              </text>
            </g>
          );
        });
      }}
    </ChartFrame>
  );
}
