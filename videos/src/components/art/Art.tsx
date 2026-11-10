import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import "../../fonts";
import type { Art, Point } from "../../script/schema";
import { actors, colors, fonts } from "../../theme";
import { CLI_COLUMNS, SKILLS } from "./cliData";

const IN_FRAMES = 9; // 300 ms at 30 fps

// 0 before `at`, 1 nine frames later. An undefined `at` means shown from the start.
function useReveal(at: number | undefined): number {
  const frame = useCurrentFrame();
  if (at === undefined) return 1;
  return interpolate(frame, [at, at + IN_FRAMES], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
}

const Rise: React.FC<{ at?: number; children: React.ReactNode; style?: React.CSSProperties }> = ({ at, children, style }) => {
  const t = useReveal(at);
  return <div style={{ opacity: t, transform: `translateY(${(1 - t) * 14}px)`, ...style }}>{children}</div>;
};

/** The pill that says who is acting. Dark text on the actor's color; `on="light"` adds an outline. */
export const ActorBadge: React.FC<{ who: "you" | "agent" | "both"; size?: number }> = ({ who, size = 18 }) => {
  const base: React.CSSProperties = {
    display: "inline-block",
    fontFamily: fonts.mono,
    fontWeight: 700,
    fontSize: size,
    lineHeight: 1,
    letterSpacing: "0.08em",
    textTransform: "uppercase",
    color: colors.darkIndigo,
    padding: `${size * 0.3}px ${size * 0.55}px`,
    borderRadius: 4,
    textIndent: 0,
  };
  if (who === "both") {
    return (
      <span style={{ ...base, background: `linear-gradient(90deg, ${actors.agent.color} 50%, ${actors.you.color} 50%)` }}>
        Agent + you
      </span>
    );
  }
  return <span style={{ ...base, background: actors[who].color }}>{actors[who].label}</span>;
};

// --- logo: the Tiller boat and wave, beside the opening title -------------------------------

const Logo: React.FC = () => (
  <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 28 }}>
    <Img src={staticFile("brand/tiller-logo.svg")} style={{ width: 440 }} />
    <div style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 76, letterSpacing: "-0.01em", color: colors.darkIndigo }}>Tiller</div>
  </div>
);

// --- repo: the plan as plain files next to the code -----------------------------------------

const TREE: [line: string, note: string, strong?: boolean][] = [
  ["acme-orders/", ""],
  ["├─ src/", "your code"],
  ["├─ test/", ""],
  ["└─ .tiller/", "the plan", true],
  ["   ├─ design/", "what it does, and should do", true],
  ["   ├─ epics/", "the work, big to small", true],
  ["   ├─ events/", "every decision, recorded", true],
  ["   └─ tiller.yml", "your own checks", true],
];

const Repo: React.FC = () => (
  <div
    style={{
      background: colors.websiteWhite,
      border: `2px solid ${colors.frenchGrey}`,
      borderRadius: 10,
      padding: "30px 34px",
      fontFamily: fonts.mono,
      fontSize: 25,
      lineHeight: 1.7,
      boxShadow: "0 10px 30px rgba(0, 17, 41, 0.08)",
    }}
  >
    {TREE.map(([line, note, strong]) => (
      <div key={line} style={{ display: "flex", gap: 22, whiteSpace: "pre" }}>
        <span style={{ width: 268, flex: "none", fontWeight: strong ? 700 : 400, color: strong ? colors.darkIndigo : colors.gracefulOlive }}>{line}</span>
        <span style={{ fontFamily: fonts.body, fontSize: 22, color: colors.gracefulOlive }}>{note}</span>
      </div>
    ))}
  </div>
);

// --- around: the code is the short part; the three questions around it are the long part ----

const Around: React.FC<{ points: Point[]; frames?: number[] }> = ({ points, frames }) => {
  const label: React.CSSProperties = { fontFamily: fonts.mono, fontWeight: 700, fontSize: 22, letterSpacing: "0.1em", textTransform: "uppercase", marginBottom: 14 };
  const block: React.CSSProperties = { height: 150, borderRadius: 8, display: "flex", alignItems: "center", padding: "0 30px", fontFamily: fonts.display, fontWeight: 700, fontSize: 38, lineHeight: 1.15 };
  return (
    <div style={{ display: "flex", gap: 18, alignItems: "flex-end" }}>
      <div style={{ flex: "0 0 250px" }}>
        <div style={{ ...label, color: colors.gracefulOlive }}>Quick now</div>
        <div style={{ ...block, background: actors.agent.color, color: colors.darkIndigo }}>Writing the code</div>
      </div>
      <div style={{ flex: 1 }}>
        <div style={{ ...label, color: colors.brashBurgundy, borderBottom: `4px solid ${colors.brashBurgundy}`, paddingBottom: 10 }}>The slow part</div>
        <div style={{ display: "flex", gap: 18 }}>
          {points.map((p, i) => (
            <Rise key={i} at={frames?.[i]} style={{ flex: 1 }}>
              <div style={{ ...block, background: colors.darkIndigo, color: colors.websiteWhite }}>{p.text}</div>
            </Rise>
          ))}
        </div>
      </div>
    </div>
  );
};

// --- parts: one binary holding a CLI, skills, and a console ---------------------------------

const stroke = { fill: "none", stroke: colors.darkIndigo, strokeWidth: 5, strokeLinecap: "round", strokeLinejoin: "round" } as const;
const PART_ICONS: React.ReactNode[] = [
  // A terminal window with a prompt.
  <svg key="cli" viewBox="0 0 120 96" width={150}>
    <rect x="4" y="4" width="112" height="88" rx="10" {...stroke} />
    <path d="M26 34 L44 48 L26 62" {...stroke} stroke={colors.gracefulOlive} />
    <path d="M56 64 H90" {...stroke} />
  </svg>,
  // A stack of instruction sheets.
  <svg key="skills" viewBox="0 0 120 96" width={150}>
    <rect x="30" y="4" width="78" height="66" rx="8" {...stroke} stroke={colors.gracefulOlive} />
    <rect x="12" y="24" width="78" height="68" rx="8" {...stroke} fill={colors.websiteWhite} />
    <path d="M28 44 H74 M28 58 H74 M28 72 H56" {...stroke} />
  </svg>,
  // A browser window with a board in it.
  <svg key="console" viewBox="0 0 120 96" width={150}>
    <rect x="4" y="4" width="112" height="88" rx="10" {...stroke} />
    <path d="M4 26 H116" {...stroke} />
    <rect x="18" y="40" width="22" height="38" rx="4" {...stroke} stroke={colors.gracefulOlive} />
    <rect x="49" y="40" width="22" height="24" rx="4" {...stroke} stroke={colors.gracefulOlive} />
    <rect x="80" y="40" width="22" height="32" rx="4" {...stroke} stroke={colors.gracefulOlive} />
  </svg>,
];

const Parts: React.FC<{ points: Point[]; frames?: number[] }> = ({ points, frames }) => (
  <div style={{ position: "relative", border: `4px solid ${colors.darkIndigo}`, borderRadius: 16, padding: "56px 40px 40px", marginTop: 22 }}>
    <div
      style={{
        position: "absolute",
        top: -24,
        left: 36,
        background: colors.darkIndigo,
        color: colors.websiteWhite,
        fontFamily: fonts.mono,
        fontWeight: 700,
        fontSize: 26,
        padding: "8px 20px",
        borderRadius: 6,
      }}
    >
      tiller <span style={{ color: actors.agent.color, fontWeight: 400 }}>one binary</span>
    </div>
    <div style={{ display: "flex", gap: 32 }}>
      {points.map((p, i) => (
        <Rise key={i} at={frames?.[i]} style={{ flex: 1 }}>
          <div
            style={{
              height: 290,
              background: colors.websiteWhite,
              border: `2px solid ${colors.frenchGrey}`,
              borderRadius: 12,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              gap: 26,
              padding: "0 28px",
              textAlign: "center",
            }}
          >
            {PART_ICONS[i]}
            <div style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 36, lineHeight: 1.15, color: colors.darkIndigo }}>{p.text}</div>
          </div>
        </Rise>
      ))}
    </div>
  </div>
);

// --- cli: every command and every skill, with this video's highlighted ----------------------

// Padding with a matching negative margin, so a highlighted name stays on the column line
// and never covers the descenders of the row above.
const HIGHLIGHT: React.CSSProperties = { background: colors.matureYellow, borderRadius: 4, padding: "0 7px", margin: "0 -7px" };

const Cli: React.FC<{ seen: string[]; from: number }> = ({ seen, from }) => {
  const has = new Set(seen);
  const h3: React.CSSProperties = {
    fontFamily: fonts.mono,
    fontWeight: 700,
    fontSize: 20,
    letterSpacing: "0.1em",
    textTransform: "uppercase",
    color: colors.mediumIndigo,
    paddingBottom: 8,
    borderBottom: `3px solid ${colors.growingGreen}`,
    marginBottom: 8,
  };
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", columnGap: 44 }}>
      {CLI_COLUMNS.map((groups, c) => (
        <Rise key={c} at={from + c * 6}>
          {groups.map((g, gi) => (
            <div key={g.title} style={{ marginTop: gi ? 22 : 0 }}>
              <div style={h3}>{g.title}</div>
              {g.commands.map(([name, what]) => (
                <div key={name} style={{ display: "flex", alignItems: "center", height: 33, whiteSpace: "nowrap" }}>
                  <span style={{ flex: "0 0 132px" }}>
                    <span style={{ fontFamily: fonts.mono, fontWeight: 700, fontSize: 22, lineHeight: "30px", display: "inline-block", color: colors.darkIndigo, ...(has.has(name) ? HIGHLIGHT : {}) }}>{name}</span>
                  </span>
                  <span style={{ fontFamily: fonts.body, fontSize: 19, color: colors.gracefulOlive, overflow: "hidden", textOverflow: "ellipsis" }}>{what}</span>
                </div>
              ))}
            </div>
          ))}
          {c === CLI_COLUMNS.length - 1 && (
            <div style={{ marginTop: 22 }}>
              <div style={h3}>Agent skills</div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginTop: 12 }}>
                {SKILLS.map((s) => {
                  const on = has.has(`skill:${s}`);
                  return (
                    <span
                      key={s}
                      style={{
                        fontFamily: fonts.mono,
                        fontWeight: on ? 700 : 400,
                        fontSize: 19,
                        padding: "5px 12px",
                        borderRadius: 4,
                        color: colors.darkIndigo,
                        border: `2px solid ${on ? colors.matureYellow : colors.frenchGrey}`,
                        background: on ? colors.matureYellow : "transparent",
                      }}
                    >
                      {s}
                    </span>
                  );
                })}
              </div>
            </div>
          )}
        </Rise>
      ))}
    </div>
  );
};

// --- process: the path the series walks, one row per video ----------------------------------

type Stage = [who: "you" | "agent", what: string];
// One row per video. Every row ends on a decision that is the human's.
export const PROCESS_ROWS: Stage[][] = [
  [["you", "Install and join"], ["agent", "Write down what the code does"], ["you", "Confirm it"]],
  [["you", "Write one page"], ["agent", "Break it into a plan"], ["you", "Approve the shape"], ["agent", "Specify the feature"], ["you", "Approve the plan"]],
  [["agent", "Build, pass the checks"], ["agent", "Review the code"], ["agent", "Fix what review found"], ["you", "Merge"]],
];

const StageBox: React.FC<{ stage: Stage }> = ({ stage: [who, what] }) => {
  const you = who === "you";
  return (
    <div
      style={{
        flex: 1,
        minWidth: 0,
        height: 150,
        borderRadius: 8,
        padding: "16px 18px",
        display: "flex",
        flexDirection: "column",
        gap: 10,
        background: you ? colors.darkIndigo : colors.websiteWhite,
        border: `2px solid ${you ? colors.darkIndigo : colors.frenchGrey}`,
      }}
    >
      <div>
        <ActorBadge who={who} size={16} />
      </div>
      <div style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 27, lineHeight: 1.15, color: you ? colors.websiteWhite : colors.darkIndigo }}>{what}</div>
    </div>
  );
};

const Process: React.FC<{ points: Point[]; frames?: number[]; current: number }> = ({ points, frames, current }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
    {PROCESS_ROWS.map((row, r) => (
      <Rise key={r} at={frames?.[r]}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ flex: "0 0 262px" }}>
            <div style={{ fontFamily: fonts.mono, fontWeight: 700, fontSize: 19, letterSpacing: "0.08em", textTransform: "uppercase", whiteSpace: "nowrap", color: r === current ? colors.brashBurgundy : colors.gracefulOlive }}>
              Video {r + 1}
              {r === current ? " · this one" : ""}
            </div>
            <div style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 34, lineHeight: 1.1, marginTop: 6, color: colors.darkIndigo }}>{points[r]?.text}</div>
          </div>
          {row.map((stage, i) => (
            <React.Fragment key={i}>
              {i > 0 && <div style={{ flex: "none", fontFamily: fonts.mono, fontWeight: 700, fontSize: 26, color: colors.gracefulOlive }}>→</div>}
              <StageBox stage={stage} />
            </React.Fragment>
          ))}
        </div>
      </Rise>
    ))}
  </div>
);

// --- tracker: where this chapter sits among the video's chapters -----------------------------

export const Tracker: React.FC<{ names: string[]; index: number }> = ({ names, index }) => (
  <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, marginTop: 64 }}>
    {names.map((name, i) => {
      const now = i === index;
      const done = i < index;
      return (
        <React.Fragment key={name}>
          {i > 0 && <div style={{ width: 22, height: 3, background: colors.frenchGrey }} />}
          <div
            style={{
              fontFamily: fonts.body,
              fontWeight: now ? 700 : 500,
              fontSize: 24,
              padding: "10px 18px",
              borderRadius: 999,
              whiteSpace: "nowrap",
              background: now ? colors.darkIndigo : "transparent",
              color: now ? colors.websiteWhite : done ? colors.gracefulOlive : colors.mediumIndigo,
              border: `2px solid ${now ? colors.darkIndigo : done ? colors.gracefulOlive : colors.frenchGrey}`,
            }}
          >
            {done ? "✓ " : `${i + 1}. `}
            {name}
          </div>
        </React.Fragment>
      );
    })}
  </div>
);

/** Graphics that sit beside the text; the rest go under the heading at full width. */
export const SIDE_ARTS: readonly Art[] = ["logo", "repo"];

export const CardArt: React.FC<{
  art: Art;
  points?: Point[];
  pointFrames?: number[];
  // The frame of the card at which its narration starts.
  from: number;
  seen?: string[];
  // Which video of the series this is (0-based), for the process graphic.
  videoIndex: number;
}> = ({ art, points = [], pointFrames, from, seen = [], videoIndex }) => {
  switch (art) {
    case "logo":
      return <Logo />;
    case "repo":
      return <Repo />;
    case "around":
      return <Around points={points} frames={pointFrames} />;
    case "parts":
      return <Parts points={points} frames={pointFrames} />;
    case "cli":
      return <Cli seen={seen} from={from} />;
    case "process":
      return <Process points={points} frames={pointFrames} current={videoIndex} />;
  }
};
