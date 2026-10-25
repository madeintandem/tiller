// Epic progress (D10): the one snapshot chart. One row per approved epic in
// the root soft order, a three-segment bar (Done, In progress, Not started),
// and "N of M features" at the end. The segments sit in that fixed order and
// are named in the legend text and the tooltip, and Not started is a lighter
// tint. Done and In progress are told apart by order and the legend beside
// their hue. A draft epic, not yet approved, interleaves in the same order as
// a ghost row: a muted title over an empty track outlined in round dots,
// labeled with its planned feature count ("2 features planned") or "Not yet
// approved" when it has none, so the client sees the future work without it
// reading as zero progress. The dotted outline and the label carry it, not hue.

import type { Report, ReportEpic } from "../../api";
import ChartFrame, { DOTTED, tint, type LegendEntry } from "./ChartFrame";
import { useTooltip } from "./ChartTooltip";

const DONE = "var(--st-done)";
const IN_PROGRESS = "var(--st-active)";
const NOT_STARTED = tint("var(--st-draft)");
const PLANNED = "var(--ink-3)";
const PLANNED_LABEL = "Not yet approved";
const ROW = 44;
const BAR = 12;
// Room at the bar's end for "12 of 12 features" / "No features yet".
const COUNT_WIDTH = 132;
// The surface gap between segments.
const GAP = 2;
// A rough width per title character, to shorten a title before it runs past
// the card; the full title stays in the tooltip.
const CHAR_WIDTH = 7.5;

function features(epic: ReportEpic): number {
  return epic.done + epic.in_progress + epic.not_started;
}

// plannedLabel is a ghost row's right-hand label: its planned feature count,
// or "Not yet approved" when it has none yet.
function plannedLabel(epic: ReportEpic): string {
  const n = epic.planned_features;
  if (n === 0) return PLANNED_LABEL;
  return n === 1 ? "1 feature planned" : `${n} features planned`;
}

function epicCount(n: number): string {
  return n === 1 ? "1 epic" : `${n} epics`;
}

// summary is the chart's accessible name: approved progress, then the count
// of ghost rows when there are any.
function summary(epics: ReportEpic[]): string {
  const approved = epics.filter((e) => !e.planned);
  const planned = epics.length - approved.length;
  const ghosts = `${epicCount(planned)} not yet approved`;
  if (approved.length === 0) return ghosts;
  const done = approved.reduce((sum, e) => sum + e.done, 0);
  const total = approved.reduce((sum, e) => sum + features(e), 0);
  const progress = `${done} of ${total} features done across ${epicCount(approved.length)}`;
  return planned > 0 ? `${progress} and ${ghosts}` : progress;
}

function shorten(title: string, room: number): string {
  const fit = Math.max(8, Math.floor(room / CHAR_WIDTH));
  return title.length > fit ? `${title.slice(0, fit - 1)}…` : title;
}

export default function EpicProgressChart({ report }: { report: Report }) {
  const { tip, show, hide } = useTooltip();
  const { epics } = report;
  const empty = epics.length === 0 ? "No approved scope yet" : undefined;
  const legend: LegendEntry[] = [
    { label: "Done", swatch: "bar", color: DONE },
    { label: "In progress", swatch: "bar", color: IN_PROGRESS },
    { label: "Not started", swatch: "bar", color: NOT_STARTED },
  ];
  if (epics.some((e) => e.planned)) legend.push({ label: PLANNED_LABEL, swatch: "outline", color: PLANNED });

  return (
    <ChartFrame
      title="Epic progress"
      summary={summary(epics)}
      empty={empty}
      legend={legend}
      height={epics.length * ROW + 4}
      tip={tip}
    >
      {(width) => {
        const barWidth = Math.max(width - COUNT_WIDTH, 0);
        return epics.map((epic, i) => {
          const y = i * ROW;
          if (epic.planned) {
            return (
              <g
                key={i}
                data-planned="true"
                onPointerMove={(e) => show(e, [{ label: epic.title, value: plannedLabel(epic) }])}
                onPointerLeave={hide}
              >
                <rect className="report-hit" x={0} y={y} width={width} height={ROW} />
                <text className="report-row-title report-row-planned" x={0} y={y + 14}>
                  {shorten(epic.title, width)}
                </text>
                <rect
                  className="report-ghost-track"
                  x={DOTTED.strokeWidth / 2}
                  y={y + 22 + DOTTED.strokeWidth / 2}
                  width={Math.max(barWidth - DOTTED.strokeWidth, 0)}
                  height={BAR - DOTTED.strokeWidth}
                  {...DOTTED}
                />
                <text
                  className="report-row-count report-row-planned"
                  x={width}
                  y={y + 22 + BAR / 2}
                  textAnchor="end"
                  dominantBaseline="middle"
                >
                  {plannedLabel(epic)}
                </text>
              </g>
            );
          }
          const m = features(epic);
          const segments = [
            { value: epic.done, color: DONE },
            { value: epic.in_progress, color: IN_PROGRESS },
            { value: epic.not_started, color: NOT_STARTED },
          ].filter((s) => s.value > 0);
          const room = barWidth - GAP * Math.max(segments.length - 1, 0);
          let x = 0;
          return (
            <g
              key={i}
              onPointerMove={(e) =>
                show(e, [
                  { label: epic.title, value: "" },
                  { label: "Done", value: String(epic.done) },
                  { label: "In progress", value: String(epic.in_progress) },
                  { label: "Not started", value: String(epic.not_started) },
                ])
              }
              onPointerLeave={hide}
            >
              <rect className="report-hit" x={0} y={y} width={width} height={ROW} />
              <text className="report-row-title" x={0} y={y + 14}>
                {shorten(epic.title, width)}
              </text>
              {m === 0 && <rect className="report-track" x={0} y={y + 22} width={barWidth} height={BAR} />}
              {segments.map((s) => {
                const w = (room * s.value) / m;
                const seg = <rect key={s.color} x={x} y={y + 22} width={w} height={BAR} style={{ fill: s.color }} />;
                x += w + GAP;
                return seg;
              })}
              <text className="report-row-count" x={width} y={y + 22 + BAR / 2} textAnchor="end" dominantBaseline="middle">
                {m === 0 ? "No features yet" : `${epic.done} of ${m} features`}
              </text>
            </g>
          );
        });
      }}
    </ChartFrame>
  );
}
