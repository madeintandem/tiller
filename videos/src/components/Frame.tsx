import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import "../fonts";
import { colors } from "../theme";

const FADE_FRAMES = 9; // 300 ms at 30 fps

// MiT soft edge: an organic wave bleeding the parchment into a medium indigo band.
const SoftEdge: React.FC = () => (
  <svg
    viewBox="0 0 120 800"
    preserveAspectRatio="none"
    aria-hidden="true"
    style={{ position: "absolute", top: 0, right: 0, height: "100%", width: 150 }}
  >
    <rect width="120" height="800" fill={colors.mediumIndigo} />
    <path
      d="M 0 0 L 0 800 C 15 720, 45 680, 35 600 C 25 520, 55 480, 50 400 C 45 320, 15 280, 30 200 C 45 120, 20 80, 35 0 Z"
      fill={colors.parchment}
    />
  </svg>
);

export const Frame: React.FC<{ children?: React.ReactNode }> = ({ children }) => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const opacity = interpolate(
    frame,
    [0, FADE_FRAMES, durationInFrames - FADE_FRAMES, durationInFrames],
    [0, 1, 1, 0],
    { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
  );
  return (
    <div style={{ position: "absolute", inset: 0, background: colors.parchment, overflow: "hidden" }}>
      <SoftEdge />
      <div style={{ position: "absolute", inset: 0, padding: "96px 200px 160px 120px", opacity }}>{children}</div>
      <Img
        src={staticFile("brand/mit-logo-dark-indigo.svg")}
        style={{ position: "absolute", left: 120, bottom: 56, width: 200 }}
      />
    </div>
  );
};
