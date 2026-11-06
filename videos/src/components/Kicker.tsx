import React from "react";
import "../fonts";
import { colors, fonts } from "../theme";

// Olive Space Mono kicker: uppercase, 0.1em tracking (cold indigo on dark).
export const Kicker: React.FC<{ text: string; onDark?: boolean }> = ({ text, onDark }) => (
  <div
    style={{
      fontFamily: fonts.mono,
      fontWeight: 700,
      fontSize: 28,
      letterSpacing: "0.1em",
      textTransform: "uppercase",
      color: onDark ? colors.coldIndigo : colors.gracefulOlive,
      marginBottom: 20,
    }}
  >
    {text}
  </div>
);
