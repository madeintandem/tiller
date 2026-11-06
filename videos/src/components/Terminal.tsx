import React from "react";
import { useCurrentFrame, useVideoConfig } from "remotion";
import "../fonts";
import type { Step } from "../check/diff";
import { termState, type TermLine } from "../term/state";
import { fonts } from "../theme";

// Values ported from the homepage terminal (site/src/pages/index.astro, dark theme).
const t = {
  bg: "#000814",
  fg: "#E8EBF0",
  dim: "#93A1B3",
  agent: "#96BCBC",
  you: "#F49BC0",
  pass: "#A6D08F",
  fail: "#F59A86",
};
const FONT = 26;
const LINE = 1.62;
const GUTTER = "8ch";

// **bold**, !!fail!!, ++pass++ as on the homepage.
function markup(text: string): React.ReactNode[] {
  const parts: React.ReactNode[] = [];
  const re = /\*\*(.+?)\*\*|!!(.+?)!!|\+\+(.+?)\+\+/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let k = 0;
  while ((m = re.exec(text))) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    if (m[1] !== undefined) parts.push(<b key={k++} style={{ color: t.fg, fontWeight: 700 }}>{m[1]}</b>);
    else if (m[2] !== undefined) parts.push(<span key={k++} style={{ color: t.fail, fontWeight: 700 }}>{m[2]}</span>);
    else parts.push(<span key={k++} style={{ color: t.pass, fontWeight: 700 }}>{m[3]}</span>);
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return parts;
}

const lineStyle: React.CSSProperties = {
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  paddingLeft: GUTTER,
  textIndent: `-${GUTTER}`,
  minHeight: `${LINE}em`,
  flex: "none",
};
// A clipped column whose newest lines sit at the bottom; overflow leaves off the top.
const scroller: React.CSSProperties = { minHeight: 0, display: "flex", flexDirection: "column", justifyContent: "flex-end", overflow: "hidden" };
const who: React.CSSProperties = { display: "inline-block", width: GUTTER, textIndent: 0, fontWeight: 700 };

const Line: React.FC<{ line: TermLine; caret: boolean }> = ({ line, caret }) => {
  const color = line.actor === "you" ? t.you : t.agent;
  if (line.kind === "out") {
    return <div style={{ ...lineStyle, color: t.dim }}>{line.text ? markup(line.text) : ""}</div>;
  }
  if (line.kind === "say") {
    return (
      <div style={lineStyle}>
        <span style={{ ...who, color }}>{line.actor}</span>
        <span style={{ color: t.fg }}>{line.text}</span>
      </div>
    );
  }
  return (
    <div style={lineStyle}>
      <span style={{ ...who, color }}>{line.actor}</span>
      <span style={{ color: t.dim }}>$ </span>
      <span style={{ color: t.fg }}>{line.text.slice(0, line.typed)}</span>
      {caret && (
        <span style={{ display: "inline-block", width: "0.6em", height: "1.1em", verticalAlign: "-0.2em", background: t.fg }} />
      )}
    </div>
  );
};

export const Terminal: React.FC<{ steps: Step[]; startFrame: number; endFrame: number }> = ({ steps, startFrame, endFrame }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const lines = termState(steps, startFrame, endFrame, frame, fps);
  const agentLane = steps.some((s) => s.actor === "agent");
  const last = lines.length - 1;
  const split = lines.findIndex((l) => l.current);
  const history = split < 0 ? lines : lines.slice(0, split);
  const [head, ...tail] = split < 0 ? [] : lines.slice(split);
  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        background: t.bg,
        color: t.fg,
        fontFamily: fonts.mono,
        fontSize: FONT,
        lineHeight: LINE,
        overflow: "hidden",
      }}
    >
      {agentLane && (
        <div
          style={{
            flex: "none",
            padding: "12px 32px",
            borderBottom: "1px solid rgba(150, 188, 188, 0.22)",
            color: t.dim,
            fontWeight: 700,
          }}
        >
          Claude Code
        </div>
      )}
      {/* Earlier lines fill whatever room the current step leaves and scroll off the top. The
          current step's head line (its command) stays in view while its output, pinned to the
          bottom, scrolls beneath it. */}
      <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: "16px 32px 22px", overflow: "hidden" }}>
        <div style={{ ...scroller, flex: "1 1 0" }}>
          {history.map((l, i) => (
            <Line key={i} line={l} caret={false} />
          ))}
        </div>
        {head && (
          <div style={{ flex: "0 1 auto", minHeight: 0, display: "flex", flexDirection: "column" }}>
            <Line line={head} caret={head === lines[last] && head.kind === "cmd" && head.typed < head.text.length} />
            <div style={{ ...scroller, flex: "0 1 auto" }}>
              {tail.map((l, i) => (
                <Line key={i} line={l} caret={false} />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
