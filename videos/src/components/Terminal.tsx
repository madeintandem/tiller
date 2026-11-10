import React from "react";
import { useCurrentFrame, useVideoConfig } from "remotion";
import "../fonts";
import type { Step } from "../check/diff";
import type { Actor } from "../script/schema";
import { termState, type EditView, type TermLine } from "../term/state";
import { actors, fonts } from "../theme";
import { ActorBadge } from "./art/Art";

// Two surfaces, made to look different at a glance:
//  - the shell: a plain terminal window, for commands a person types themselves;
//  - the agent session: a conversation (messages in a sans face) in which every command
//    that runs sits in its own inset terminal block, so talk and terminal output never blur.
// Terminal values are ported from the homepage terminal (site/src/pages/index.astro, dark theme).
const t = {
  shell: "#000814",
  session: "#10233B",
  fg: "#E8EBF0",
  dim: "#93A1B3",
  pass: "#A6D08F",
  fail: "#F59A86",
  rule: "rgba(150, 188, 188, 0.22)",
};
const MONO = 25;
const LINE = 1.6;
const BADGE = 16;
// Room for the widest badge ("AGENT") plus the gap after it; output lines indent to match.
const GUTTER = 118;

export type TermItem =
  | { kind: "say"; actor: Actor; text: string; current: boolean }
  | { kind: "run"; actor: Actor; cmd: TermLine; out: string[]; current: boolean }
  | { kind: "edit"; actor: Actor; file: string; edit: EditView; current: boolean };

/** Group the flat lines into what the surfaces draw: a message, or a command with its output. */
export function termItems(lines: TermLine[]): TermItem[] {
  const items: TermItem[] = [];
  for (const l of lines) {
    const current = l.current === true;
    if (l.kind === "edit") items.push({ kind: "edit", actor: l.actor, file: l.text, edit: l.edit ?? { isNew: false, removed: [], added: [], more: 0 }, current });
    else if (l.kind === "say") items.push({ kind: "say", actor: l.actor, text: l.text, current });
    else if (l.kind === "cmd") items.push({ kind: "run", actor: l.actor, cmd: l, out: [], current });
    else {
      const last = items[items.length - 1];
      if (last?.kind === "run") last.out.push(l.text);
    }
  }
  return items;
}

/** The agent session shows whenever the agent takes part; otherwise it is the person's own shell. */
export const surfaceFor = (steps: Step[]): "agent" | "shell" => (steps.some((s) => s.actor === "agent") ? "agent" : "shell");

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

const mono: React.CSSProperties = { fontFamily: fonts.mono, fontSize: MONO, lineHeight: LINE };
const wrap: React.CSSProperties = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" };
// A clipped column whose newest lines sit at the bottom; overflow leaves off the top.
const scroller: React.CSSProperties = { minHeight: 0, display: "flex", flexDirection: "column", justifyContent: "flex-end", overflow: "hidden" };

const Caret: React.FC = () => (
  <span style={{ display: "inline-block", width: "0.6em", height: "1.1em", verticalAlign: "-0.2em", background: t.fg }} />
);

const Command: React.FC<{ line: TermLine; caret: boolean }> = ({ line, caret }) => (
  <span style={wrap}>
    <span style={{ color: t.dim }}>$ </span>
    <span style={{ color: t.fg }}>{line.text.slice(0, line.typed)}</span>
    {caret && <Caret />}
  </span>
);

// Lines that scroll off the top of a clipped column fade out rather than being cut in half.
const fadeTop = (px: number): React.CSSProperties => ({ maskImage: `linear-gradient(to bottom, transparent 0, black ${px}px)` });
// An output this long may outgrow its window, so its top edge fades.
const LONG_OUTPUT = 10;

const Output: React.FC<{ out: string[]; indent: number }> = ({ out, indent }) => (
  <div style={{ ...scroller, flex: "0 1 auto", ...(out.length > LONG_OUTPUT ? fadeTop(44) : {}) }}>
    {out.map((o, i) => (
      <div key={i} style={{ ...mono, ...wrap, flex: "none", minHeight: `${LINE}em`, paddingLeft: indent, color: t.dim }}>
        {o ? markup(o) : ""}
      </div>
    ))}
  </div>
);

const Gutter: React.FC<{ actor: Actor }> = ({ actor }) => (
  <span style={{ flex: `0 0 ${GUTTER}px`, paddingTop: 7 }}>
    <ActorBadge who={actor} size={BADGE} />
  </span>
);

// --- a file edit: the same block on both surfaces, lighter than a terminal so it reads as a file ---

const EDIT = { bg: "#F4F1EA", ink: "#1B2430", head: "#E3DCD9", minus: "#F8D5CD", plus: "#D5E8C8", minusInk: "#7A1F10", plusInk: "#23471A" };

const EditLine: React.FC<{ sign: "+" | "-"; text: string }> = ({ sign, text }) => (
  <div style={{ display: "flex", background: sign === "+" ? EDIT.plus : EDIT.minus, color: sign === "+" ? EDIT.plusInk : EDIT.minusInk }}>
    <span style={{ flex: "0 0 44px", textAlign: "center", fontWeight: 700 }}>{sign}</span>
    <span style={{ whiteSpace: "pre", overflow: "hidden", textOverflow: "ellipsis", paddingRight: 16 }}>{text || " "}</span>
  </div>
);

const EditBlock: React.FC<{ item: Extract<TermItem, { kind: "edit" }>; marginTop: number }> = ({ item, marginTop }) => (
  <div style={{ display: "flex", flex: "none", marginTop, marginBottom: 6 }}>
    <Gutter actor={item.actor} />
    <div style={{ flex: 1, minWidth: 0, background: EDIT.bg, color: EDIT.ink, borderRadius: 8, overflow: "hidden", fontFamily: fonts.mono, fontSize: 22, lineHeight: 1.55 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 14, background: EDIT.head, padding: "6px 16px" }}>
        <span style={{ fontWeight: 700, fontSize: 15, letterSpacing: "0.12em", textTransform: "uppercase" }}>{item.edit.isNew ? "New file" : "Edited file"}</span>
        <span style={{ fontWeight: 700 }}>{item.file}</span>
      </div>
      {item.edit.removed.map((l, i) => (
        <EditLine key={`r${i}`} sign="-" text={l} />
      ))}
      {item.edit.added.map((l, i) => (
        <EditLine key={`a${i}`} sign="+" text={l} />
      ))}
      {item.edit.more > 0 && <div style={{ padding: "2px 16px 4px 44px", fontFamily: fonts.body, fontSize: 20, opacity: 0.7 }}>and {item.edit.more} more lines</div>}
    </div>
  </div>
);

// --- the shell ------------------------------------------------------------------------------

const ShellItem: React.FC<{ item: TermItem; typing: boolean }> = ({ item, typing }) => {
  if (item.kind === "edit") return <EditBlock item={item} marginTop={6} />;
  if (item.kind === "say") {
    // Something the person does off the command line (an edit, a decision), as a shell comment.
    return (
      <div style={{ ...mono, display: "flex", flex: "none" }}>
        <Gutter actor={item.actor} />
        <span style={{ ...wrap, color: actors[item.actor].color, fontStyle: "italic" }}># {item.text}</span>
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", flex: item.current ? "0 1 auto" : "none", minHeight: 0 }}>
      <div style={{ ...mono, display: "flex", flex: "none" }}>
        <Gutter actor={item.actor} />
        <Command line={item.cmd} caret={typing} />
      </div>
      <Output out={item.out} indent={GUTTER} />
    </div>
  );
};

// --- the agent session ----------------------------------------------------------------------

const SessionItem: React.FC<{ item: TermItem; typing: boolean }> = ({ item, typing }) => {
  const color = actors[item.actor].color;
  if (item.kind === "edit") return <EditBlock item={item} marginTop={16} />;
  if (item.kind === "say") {
    return (
      <div style={{ display: "flex", flex: "none", marginTop: 16 }}>
        <Gutter actor={item.actor} />
        <div
          style={{
            ...wrap,
            flex: "0 1 auto",
            fontFamily: fonts.body,
            fontSize: 29,
            lineHeight: 1.4,
            color: t.fg,
            background: "rgba(255, 255, 255, 0.07)",
            borderLeft: `6px solid ${color}`,
            borderRadius: 6,
            padding: "12px 20px",
          }}
        >
          {item.text}
        </div>
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flex: item.current ? "0 1 auto" : "none", minHeight: 0, marginTop: 16 }}>
      <Gutter actor={item.actor} />
      <div
        style={{
          flex: 1,
          minWidth: 0,
          minHeight: 0,
          display: "flex",
          flexDirection: "column",
          background: t.shell,
          border: `1px solid ${t.rule}`,
          borderRadius: 8,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            flex: "none",
            fontFamily: fonts.mono,
            fontWeight: 700,
            fontSize: 15,
            letterSpacing: "0.12em",
            textTransform: "uppercase",
            color: t.dim,
            padding: "7px 18px",
            borderBottom: `1px solid ${t.rule}`,
          }}
        >
          Terminal
        </div>
        <div style={{ flex: "0 1 auto", minHeight: 0, display: "flex", flexDirection: "column", padding: "8px 18px 12px" }}>
          <div style={{ ...mono, flex: "none" }}>
            <Command line={item.cmd} caret={typing} />
          </div>
          <Output out={item.out} indent={0} />
        </div>
      </div>
    </div>
  );
};

// --- the window -----------------------------------------------------------------------------

const TitleBar: React.FC<{ surface: "agent" | "shell" }> = ({ surface }) => (
  <div
    style={{
      flex: "none",
      display: "flex",
      alignItems: "center",
      gap: 16,
      padding: "12px 28px",
      borderBottom: `1px solid ${t.rule}`,
      fontFamily: fonts.mono,
      fontSize: 22,
      color: t.dim,
    }}
  >
    {surface === "agent" ? (
      <>
        <span style={{ width: 16, height: 16, transform: "rotate(45deg)", background: actors.agent.color }} />
        <b style={{ color: t.fg, letterSpacing: "0.1em", textTransform: "uppercase" }}>Coding agent</b>
        <span>Claude Code</span>
      </>
    ) : (
      <>
        <span style={{ display: "flex", gap: 8 }}>
          {[0, 1, 2].map((i) => (
            <span key={i} style={{ width: 14, height: 14, borderRadius: 7, background: "rgba(147, 161, 179, 0.5)" }} />
          ))}
        </span>
        <b style={{ color: t.fg, letterSpacing: "0.1em", textTransform: "uppercase" }}>Terminal</b>
        <span>acme-orders</span>
      </>
    )}
  </div>
);

export const Terminal: React.FC<{ steps: Step[]; startFrame: number; endFrame: number }> = ({ steps, startFrame, endFrame }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const items = termItems(termState(steps, startFrame, endFrame, frame, fps));
  const surface = surfaceFor(steps);
  const Item = surface === "agent" ? SessionItem : ShellItem;
  const last = items[items.length - 1];
  const typing = (item: TermItem) => item === last && item.kind === "run" && item.cmd.typed < item.cmd.text.length;
  const history = items.filter((i) => !i.current);
  const current = items.filter((i) => i.current);
  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        background: surface === "agent" ? t.session : t.shell,
        color: t.fg,
        borderRadius: 12,
        overflow: "hidden",
      }}
    >
      <TitleBar surface={surface} />
      {/* Earlier items fill whatever room the current step leaves and scroll off the top. The
          current step's command stays in view while its output, pinned to the bottom, scrolls
          beneath it. */}
      <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: "14px 28px 22px", overflow: "hidden" }}>
        <div style={{ ...scroller, flex: "1 1 0", ...fadeTop(36) }}>
          {history.map((item, i) => (
            <Item key={i} item={item} typing={false} />
          ))}
        </div>
        {current.map((item, i) => (
          <Item key={i} item={item} typing={typing(item)} />
        ))}
      </div>
    </div>
  );
};
