import React from "react";
import { interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import "../fonts";
import { simplify, toPathData, traceStreamlines } from "../lib/flowField";
import { colors, video } from "../theme";

// The madeintandem.com homepage hero, filling the frame: evenly spaced streamlines traced
// through curl noise and bent by four poles. The yellow node is a source the lines burst
// out of, the square a sink they drain into, and the circle and triangle sit in vortices
// that wrap them in rings. Points of light travel the lines, a pool of light drifts over
// the field, and the shapes take turns flipping. The site animates with CSS; here every
// motion is a function of the frame, so a render is the same every time.

const W = video.width;
const H = video.height;
// The site's stage is 1000 units square. This one is the frame in pixels, so the site's
// distances scale by K and its noise frequency by 1/K.
const K = 1.6;

type Pole = { x: number; y: number; kind: "source" | "sink" | "vortex"; strength: number; core: number; reach: number };
type Line = { points: [number, number][]; closed: boolean; d: string; length: number };

// The poles sit in the right half, clear of the type on the left.
const source: Pole = { x: 1250, y: 520, kind: "source", strength: 260 * K, core: 18 * K, reach: 200 * K };
const sink: Pole = { x: 1070, y: 840, kind: "sink", strength: 300 * K, core: 66 * K, reach: 180 * K };
const circleVortex: Pole = { x: 1560, y: 280, kind: "vortex", strength: 1000 * K, core: 92 * K, reach: 210 * K };
const triangleVortex: Pole = { x: 1620, y: 800, kind: "vortex", strength: -900 * K, core: 84 * K, reach: 190 * K };
const poles = [source, circleVortex, triangleVortex, sink];

const near = (line: Line, pole: Pole, slack: number) => Math.hypot(line.points[0][0] - pole.x, line.points[0][1] - pole.y) < pole.core + slack;
const longest = (a: Line, b: Line) => b.length - a.length;

type Comet = { d: string; length: number; tail: number; phase: number; gold: boolean };

// Traced once per render process: a few hundred milliseconds, then every frame reuses it.
let cached: { field: string; comets: Comet[] } | undefined;
const traced = () => {
  if (cached) return cached;
  const lines: Line[] = (traceStreamlines as (o: object) => { points: [number, number][]; closed: boolean }[])({ width: W, height: H, separation: 11 * K, step: 2 * K, seed: 4, noiseScale: 0.004 / K, poles }).map(
    (line: { points: [number, number][]; closed: boolean }) => ({
      ...line,
      d: toPathData(simplify(line.points, 0.6), { closed: line.closed }),
      length: line.points.slice(1).reduce((sum, [x, y], i) => sum + Math.hypot(x - line.points[i][0], y - line.points[i][1]), 0),
    }),
  );
  // Yellow light leaves the node; pale light orbits the vortices and rides the longest currents.
  const rays = lines.filter((l) => near(l, source, 30 * K)).sort(longest).slice(0, 9);
  const rings = lines.filter((l) => l.closed && (near(l, circleVortex, 120 * K) || near(l, triangleVortex, 120 * K))).filter((_, i) => i % 2 === 0);
  const currents = lines.filter((l) => !l.closed && !rays.includes(l)).sort(longest).slice(0, 18);
  // Golden-ratio phases start every light mid-flight, spread along its line.
  const comet = (line: Line, i: number, tail: number, gold: boolean): Comet => ({ d: line.d, length: line.length, tail, phase: (i * 0.618034) % 1, gold });
  cached = {
    field: lines.map((l) => l.d).join(""),
    comets: [...[...rings, ...currents].map((l, i) => comet(l, i + 3, 40 * K, false)), ...rays.map((l, i) => comet(l, i, 26 * K, true))],
  };
  return cached;
};

const SPEED = 34 * K; // pixels a second, every light at the same pace
const DRIFT_SECONDS = 36;
const POOL = 1150; // the drifting pool of light, and the tile it repeats on
const FLIP_EVERY = 12.5 / 3;
const FLIP_SECONDS = 1.6;
// Which shape flips next, and about which axis: a fixed round, so no two flip together.
const FLIPS: [shape: "circle" | "square" | "triangle", axis: string][] = [
  ["circle", "0, 1, 0"],
  ["square", "1, 0, 0"],
  ["triangle", "1, 1, 0"],
  ["square", "0, 1, 0"],
  ["circle", "1, -1, 0"],
  ["triangle", "1, 0, 0"],
];

const flipOf = (shape: string, seconds: number): string => {
  const turn = Math.floor((seconds - 2) / FLIP_EVERY);
  if (turn < 0) return "none";
  const [who, axis] = FLIPS[turn % FLIPS.length];
  if (who !== shape) return "none";
  const t = Math.min(1, (seconds - 2 - turn * FLIP_EVERY) / FLIP_SECONDS);
  const eased = t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2;
  return `rotate3d(${axis}, ${(eased * 360).toFixed(2)}deg)`;
};

const Shape: React.FC<{ pole: Pole; size: number; name: "circle" | "square" | "triangle"; seconds: number; children: React.ReactNode }> = ({ pole, size, name, seconds, children }) => (
  <div style={{ position: "absolute", left: pole.x - size / 2, top: pole.y - size / 2, width: size, height: size, perspective: 800 }}>
    <svg viewBox="0 0 200 200" fill="none" style={{ display: "block", width: "100%", height: "100%", overflow: "visible", transform: flipOf(name, seconds) }}>
      {children}
    </svg>
  </div>
);

const BODY = { fill: colors.darkIndigo, fillOpacity: 0.85 };

/**
 * The full-frame backdrop. `quiet` is how much of the field shows under the type on the
 * left (0 hides it, 1 leaves it at full strength); the right side is always at full strength.
 */
export const FlowBackdrop: React.FC<{ quiet?: number }> = ({ quiet = 0.22 }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const seconds = frame / fps;
  const { field, comets } = traced();
  const resolve = interpolate(seconds, [0, 1.2], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const drift = ((seconds / DRIFT_SECONDS) % 1) * POOL;
  // Dim whatever passes under the copy so the text stays legible.
  const underType = `linear-gradient(90deg, rgba(0,0,0,${quiet}) 0%, rgba(0,0,0,${quiet}) 34%, #000 64%)`;
  const full: React.CSSProperties = { position: "absolute", inset: 0, width: W, height: H };
  return (
    <div style={{ ...full, background: colors.darkIndigo, overflow: "hidden" }}>
      <div style={{ ...full, opacity: resolve, maskImage: underType, WebkitMaskImage: underType }}>
        <svg viewBox={`0 0 ${W} ${H}`} fill="none" style={full}>
          <path d={field} stroke={colors.coldIndigo} strokeWidth={1.3} strokeOpacity={0.38} />
        </svg>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          fill="none"
          style={{
            ...full,
            maskImage: "radial-gradient(circle closest-side, #000, transparent)",
            WebkitMaskImage: "radial-gradient(circle closest-side, #000, transparent)",
            maskSize: `${POOL}px ${POOL}px`,
            WebkitMaskSize: `${POOL}px ${POOL}px`,
            maskPosition: `${drift}px ${drift}px`,
            WebkitMaskPosition: `${drift}px ${drift}px`,
          }}
        >
          <path d={field} stroke="#EBFAFA" strokeWidth={1.3} strokeOpacity={0.7} />
        </svg>
        <svg viewBox={`0 0 ${W} ${H}`} fill="none" style={full}>
          {comets.map((c, i) => {
            // One dash per lap of a path normalized to 1000, so one light is always on its line.
            const dash = (c.tail / c.length) * 1000;
            const lap = ((seconds * SPEED) / c.length + c.phase) % 1;
            return (
              <path
                key={i}
                d={c.d}
                pathLength={1000}
                stroke={c.gold ? colors.matureYellow : "#EBFAFA"}
                strokeWidth={c.gold ? 3 : 2.4}
                strokeLinecap="round"
                strokeDasharray={`${dash.toFixed(1)} ${(1000 - dash).toFixed(1)}`}
                strokeDashoffset={(-lap * 1000).toFixed(1)}
              />
            );
          })}
        </svg>
      </div>
      <div style={{ ...full, opacity: interpolate(seconds, [0.3, 1.5], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) }}>
        <Shape pole={circleVortex} size={224} name="circle" seconds={seconds}>
          <defs>
            <pattern id="flow-dot" width="10" height="10" patternUnits="userSpaceOnUse">
              <circle cx="5" cy="5" r="1.4" fill={colors.coldIndigo} />
            </pattern>
          </defs>
          <circle cx="100" cy="100" r="99" {...BODY} />
          <circle cx="100" cy="100" r="99" fill="url(#flow-dot)" />
          <circle cx="100" cy="100" r="99" stroke={colors.coldIndigo} strokeOpacity={0.7} strokeWidth={1.2} />
        </Shape>
        <Shape pole={triangleVortex} size={224} name="triangle" seconds={seconds}>
          <defs>
            <pattern id="flow-plus" width="13" height="13" patternUnits="userSpaceOnUse">
              <path d="M6.5 3V10M3 6.5H10" stroke={colors.matureYellow} strokeWidth={1.1} />
            </pattern>
          </defs>
          <polygon points="100,0 187,150 13,150" {...BODY} />
          <polygon points="100,0 187,150 13,150" fill="url(#flow-plus)" />
          <polygon points="100,0 187,150 13,150" stroke={colors.matureYellow} strokeOpacity={0.7} strokeWidth={1.2} />
        </Shape>
        <Shape pole={sink} size={138} name="square" seconds={seconds}>
          <defs>
            <pattern id="flow-line" width="16" height="16" patternUnits="userSpaceOnUse">
              <path d="M8 0V16" stroke="#EBFAFA" strokeWidth={1.4} strokeDasharray="4 4" />
            </pattern>
          </defs>
          <rect x="1" y="1" width="198" height="198" {...BODY} />
          <rect x="1" y="1" width="198" height="198" fill="url(#flow-line)" />
          <rect x="1" y="1" width="198" height="198" stroke="#EBFAFA" strokeOpacity={0.55} strokeWidth={1.2} />
        </Shape>
        <div
          style={{
            position: "absolute",
            left: source.x - 12,
            top: source.y - 12,
            width: 24,
            height: 24,
            borderRadius: "50%",
            background: colors.matureYellow,
            boxShadow: "0 0 0 18px rgba(253, 190, 20, 0.12), 0 0 0 19px rgba(253, 190, 20, 0.5)",
          }}
        />
      </div>
    </div>
  );
};

const FADE_FRAMES = 9; // 300 ms at 30 fps, as Frame fades its content

/** A full-frame scene on the flow backdrop: its content fades in and out, the field does not. */
export const FlowFrame: React.FC<{ children?: React.ReactNode }> = ({ children }) => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const opacity = interpolate(frame, [0, FADE_FRAMES, durationInFrames - FADE_FRAMES, durationInFrames], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <div style={{ position: "absolute", inset: 0, overflow: "hidden" }}>
      <FlowBackdrop />
      <div style={{ position: "absolute", inset: 0, opacity }}>{children}</div>
    </div>
  );
};
