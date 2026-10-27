// The frame and shared pieces every Report chart draws with (D14): the card
// with its heading, text legend, and empty state; the one weekly time axis;
// the per-week hover columns; and the small geometry helpers the charts
// share. Series colors are the console's own tokens, so light and dark come
// from tokens.css, and every legend entry pairs its color with text and a
// swatch shape so no series is told apart by hue alone.

import { useContext, useId, useLayoutEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";
import type { Report } from "../../api";

import "./charts.css";
import { ChartExpand } from "./ChartCard";
import ChartTooltip, { type Tip, type TipRow } from "./ChartTooltip";
import { formatWeek } from "./format";
import { dateX, linearScale, niceTicks, weekX } from "./scale";

export const CHART_HEIGHT = 240;
// How much taller a chart drawn at the default height is when enlarged. A
// chart that sets its own height keeps it and only gains width.
export const EXPANDED_SCALE = 1.5;
export const MARGIN = { top: 16, right: 72, bottom: 28, left: 40 };

// The width charts draw at when the plot measures zero (in tests, where jsdom
// lays nothing out and has no ResizeObserver).
const DEFAULT_WIDTH = 640;
// The fewest pixels between two week labels before labels are thinned.
const WEEK_LABEL_SPACING = 56;

export interface LegendEntry {
  label: string;
  swatch: "solid" | "dashed" | "hollow" | "dot" | "bar" | "hatched" | "outline";
  color: string;
}

// DOTTED draws an outline as round dots: a near-zero dash with a round cap
// is a dot as wide as the stroke. A ghost row's track and its legend swatch
// share it so the two always match.
export const DOTTED = { strokeWidth: 1.5, strokeDasharray: "0.1 3.5", strokeLinecap: "round" } as const;

// tint is the console's chip idiom: a token mixed toward the panel at --tint.
export function tint(color: string): string {
  return `color-mix(in srgb, ${color} var(--tint), var(--panel))`;
}

// usePatternId gives an SVG pattern an id unique on the page, so two charts
// (or a chart and its legend) never share one. useId's own colons are not
// valid in a url(#...) reference, so only [A-Za-z0-9-] is kept.
export function usePatternId(prefix: string): string {
  return `${prefix}-${useId().replace(/[^A-Za-z0-9-]/g, "")}`;
}

// HatchPattern fills with color under 45 degree panel-colored stripes: the
// second, non-hue channel for a series whose hue sits too close to a
// neighbor's for color-blind readers.
export function HatchPattern({ id, color }: { id: string; color: string }) {
  return (
    <pattern id={id} width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <rect width="5" height="5" style={{ fill: color }} />
      <line className="report-hatch-stripe" x1="0" y1="0" x2="0" y2="5" />
    </pattern>
  );
}

// columnPath is a bar with a 4px rounded data end and a square baseline.
export function columnPath(x: number, y: number, w: number, h: number): string {
  const r = Math.max(0, Math.min(4, h, w / 2));
  return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`;
}

interface FrameProps {
  title: string;
  summary: string;
  empty?: string;
  legend?: LegendEntry[];
  height?: number;
  tip?: Tip | null;
  // Charts draw at the plot's measured pixel width, so text stays at its
  // real size whether the card is full width or half, and at the frame's
  // height, which is taller when the chart is enlarged.
  children: (width: number, height: number) => ReactNode;
}

export default function ChartFrame({ title, summary, empty, legend, height, tip, children }: FrameProps) {
  const { expanded, onExpand, titleId } = useContext(ChartExpand);
  const drawHeight = height ?? (expanded ? CHART_HEIGHT * EXPANDED_SCALE : CHART_HEIGHT);
  const plot = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(DEFAULT_WIDTH);
  // The plot is measured before the first paint, so a chart (the enlarged
  // copy in particular) never shows a frame at the default width, and then
  // followed as the card resizes.
  useLayoutEffect(() => {
    const el = plot.current;
    if (!el) return;
    const initial = Math.round(el.getBoundingClientRect().width);
    if (initial > 0) setWidth(initial);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      const measured = Math.round(entry.contentRect.width);
      if (measured > 0) setWidth(measured);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [empty]);

  return (
    <section className="report-chart">
      <div className="report-chart-head">
        <h2 id={titleId}>{title}</h2>
        {onExpand && !empty && (
          <button
            type="button"
            className="report-icon-button"
            aria-label={`Enlarge ${title}`}
            title="Enlarge"
            onClick={(e) => onExpand(e.currentTarget)}
          >
            <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
              <circle cx="6.75" cy="6.75" r="4.75" fill="none" stroke="currentColor" strokeWidth="1.5" />
              <path d="M10.25 10.25L14 14" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
            </svg>
          </button>
        )}
      </div>
      {legend && legend.length > 0 && (
        <ul className="report-legend">
          {legend.map((entry) => (
            <li key={entry.label}>
              <Swatch entry={entry} />
              <span>{entry.label}</span>
            </li>
          ))}
        </ul>
      )}
      {empty ? (
        <p className="report-empty">{empty}</p>
      ) : (
        <div className="report-plot" ref={plot}>
          <svg role="img" aria-label={summary} width={width} height={drawHeight} viewBox={`0 0 ${width} ${drawHeight}`}>
            {children(width, drawHeight)}
          </svg>
          <ChartTooltip tip={tip ?? null} />
        </div>
      )}
    </section>
  );
}

function Swatch({ entry }: { entry: LegendEntry }) {
  const { swatch, color } = entry;
  const hatch = usePatternId("report-hatch");
  return (
    <svg className="report-swatch" data-swatch={swatch} width="18" height="10" aria-hidden="true">
      {swatch === "bar" && <rect x="3" y="1" width="12" height="8" rx="2" style={{ fill: color }} />}
      {swatch === "hatched" && (
        <>
          <defs>
            <HatchPattern id={hatch} color={color} />
          </defs>
          <rect x="3" y="1" width="12" height="8" rx="2" fill={`url(#${hatch})`} />
        </>
      )}
      {swatch === "outline" && (
        <rect x="3.75" y="1.75" width="10.5" height="6.5" {...DOTTED} style={{ fill: "none", stroke: color }} />
      )}
      {swatch === "dot" && <circle cx="9" cy="5" r="4" style={{ fill: color }} />}
      {swatch === "hollow" && (
        <circle cx="9" cy="5" r="3.5" strokeWidth="1.5" style={{ fill: "var(--panel)", stroke: color }} />
      )}
      {(swatch === "solid" || swatch === "dashed") && (
        <line
          x1="1"
          y1="5"
          x2="17"
          y2="5"
          strokeWidth="2"
          strokeDasharray={swatch === "dashed" ? "4 3" : undefined}
          style={{ stroke: color }}
        />
      )}
    </svg>
  );
}

// WeekPlot is the coordinate system of a weekly chart, in SVG pixels.
export interface WeekPlot {
  weeks: string[];
  width: number;
  height: number;
  band: number;
  ticks: number[];
  x: (index: number) => number;
  date: (iso: string) => number;
  y: (value: number) => number;
}

export function weekPlot(report: Pick<Report, "weeks" | "week_boundaries">, width: number, height: number, max: number): WeekPlot {
  const { weeks, week_boundaries } = report;
  const ticks = niceTicks(max);
  const plotWidth = width - MARGIN.left - MARGIN.right;
  const center = weekX(weeks, plotWidth);
  const at = dateX(week_boundaries, plotWidth);
  return {
    weeks,
    width,
    height,
    band: plotWidth / Math.max(weeks.length, 1),
    ticks,
    x: (i) => MARGIN.left + center(i),
    date: (iso) => MARGIN.left + at(iso),
    y: linearScale([0, ticks[ticks.length - 1]], [height - MARGIN.bottom, MARGIN.top]),
  };
}

// WeekAxis draws beneath a weekly chart's marks: gridlines at niceTicks, week
// labels thinned to fit, and the current week's band hatched as partial
// (D5) so a week in progress does not read as a slump.
export function WeekAxis({
  plot,
  unit,
  format,
}: {
  plot: WeekPlot;
  unit?: string;
  // format words a tick; the Project charts show ticks as bare numbers.
  format?: (tick: number) => string;
}) {
  const { weeks, band, ticks, width, height } = plot;
  const last = weeks.length - 1;
  const every = Math.max(1, Math.ceil(WEEK_LABEL_SPACING / band));
  const partial = usePatternId("report-partial");
  return (
    <g className="report-axis">
      <defs>
        <pattern id={partial} width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
          <line className="report-hatch" x1="0" y1="0" x2="0" y2="6" />
        </pattern>
      </defs>
      <rect
        data-partial="true"
        x={MARGIN.left + band * last}
        y={MARGIN.top}
        width={band}
        height={height - MARGIN.top - MARGIN.bottom}
        fill={`url(#${partial})`}
      />
      {ticks.map((t) => (
        <g key={t}>
          <line className="report-grid" x1={MARGIN.left} x2={width - MARGIN.right} y1={plot.y(t)} y2={plot.y(t)} />
          <text className="report-tick" x={MARGIN.left - 8} y={plot.y(t)} textAnchor="end" dominantBaseline="middle">
            {format ? format(t) : t}
          </text>
        </g>
      ))}
      {unit && (
        <text className="report-tick" x={MARGIN.left} y={MARGIN.top - 6}>
          {unit}
        </text>
      )}
      {weeks.map((w, i) =>
        (last - i) % every === 0 ? (
          <text key={w} className="report-tick" x={plot.x(i)} y={height - MARGIN.bottom + 18} textAnchor="middle">
            {formatWeek(w)}
          </text>
        ) : null,
      )}
    </g>
  );
}

// weekRows frames a week's tooltip: the week's name first, then the series,
// then "Week so far" on the current, partial week.
export function weekRows(weeks: string[], index: number, rows: TipRow[]): TipRow[] {
  const head = { label: "Week of", value: formatWeek(weeks[index]) };
  return index === weeks.length - 1 ? [head, ...rows, { label: "Week so far", value: "" }] : [head, ...rows];
}

// WeekHits lays a transparent column over each week, on top of the marks, so
// the pointer only has to be in the right week. The hovered column draws a
// hairline crosshair at the week's center.
export function WeekHits({
  plot,
  onMove,
  onLeave,
}: {
  plot: WeekPlot;
  onMove: (index: number, e: PointerEvent<SVGRectElement>) => void;
  onLeave: () => void;
}) {
  const [active, setActive] = useState<number | null>(null);
  const top = MARGIN.top;
  const bottom = plot.height - MARGIN.bottom;
  return (
    <g>
      {active !== null && <line className="report-crosshair" x1={plot.x(active)} x2={plot.x(active)} y1={top} y2={bottom} />}
      {plot.weeks.map((w, i) => (
        <rect
          key={w}
          className="report-hit"
          data-week={i}
          x={MARGIN.left + plot.band * i}
          y={top}
          width={plot.band}
          height={bottom - top}
          onPointerMove={(e) => {
            setActive(i);
            onMove(i, e);
          }}
          onPointerLeave={() => {
            setActive(null);
            onLeave();
          }}
        />
      ))}
    </g>
  );
}
