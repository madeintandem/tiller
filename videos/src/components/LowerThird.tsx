import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import "../fonts";
import { colors, fonts, wordmark } from "../theme";

const SLIDE_FRAMES = 9; // 300 ms at 30 fps

export const LowerThird: React.FC<{ text: string }> = ({ text }) => {
  const frame = useCurrentFrame();
  const t = interpolate(frame, [0, SLIDE_FRAMES], [0, 1], { extrapolateRight: "clamp" });
  return (
    <div
      style={{
        position: "absolute",
        left: wordmark.left,
        bottom: wordmark.clearTop,
        opacity: t,
        transform: `translateY(${(1 - t) * 16}px)`,
        background: colors.darkIndigo,
        color: colors.websiteWhite,
        fontFamily: fonts.body,
        fontWeight: 500,
        fontSize: 36,
        padding: "16px 28px",
        borderRadius: 6,
        borderLeft: `6px solid ${colors.gracefulOlive}`,
      }}
    >
      {text}
    </div>
  );
};
