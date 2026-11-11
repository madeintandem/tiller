import React from "react";
import { useCurrentFrame, useVideoConfig } from "remotion";
import "../fonts";
import { colors, fonts, wordmark } from "../theme";
import { lowerThirdOpacity } from "./timing";

// Slides up and fades in, then fades out before its sequence ends.
// `onDark` is for the title screen, where a dark indigo plate would vanish into the backdrop.
export const LowerThird: React.FC<{ text: string; onDark?: boolean }> = ({ text, onDark }) => {
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const t = lowerThirdOpacity(frame, durationInFrames);
  const rise = Math.min(1, frame / 9);
  return (
    <div
      style={{
        position: "absolute",
        left: wordmark.left,
        bottom: wordmark.clearTop,
        opacity: t,
        transform: `translateY(${(1 - rise) * 16}px)`,
        background: onDark ? colors.mediumIndigo : colors.darkIndigo,
        color: colors.websiteWhite,
        fontFamily: fonts.body,
        fontWeight: 500,
        fontSize: 36,
        padding: "16px 28px",
        borderRadius: 6,
        borderLeft: `6px solid ${onDark ? colors.matureYellow : colors.gracefulOlive}`,
        boxShadow: onDark ? "0 0 0 1px rgba(150, 188, 188, 0.35)" : undefined,
      }}
    >
      {text}
    </div>
  );
};
