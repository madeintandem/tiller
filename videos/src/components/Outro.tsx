import React from "react";
import { Img, staticFile } from "remotion";
import { FlowFrame } from "./FlowBackdrop";
import { Kicker } from "./Kicker";
import { colors, fonts, site } from "../theme";

const line: React.CSSProperties = { fontFamily: fonts.display, fontWeight: 700, fontSize: 60, letterSpacing: "-0.01em", lineHeight: 1.25 };

// The closing slide, on the same backdrop as the title screen: the full logo, and the two
// ways to reach the company, set large enough to read on a phone.
export const Outro: React.FC = () => (
  <FlowFrame>
    <div style={{ position: "absolute", left: 120, top: 96, bottom: 96, width: 940, display: "flex", flexDirection: "column", justifyContent: "center" }}>
      <Img src={staticFile("brand/mit-logo-website-white.svg")} style={{ width: 420, marginBottom: 56 }} />
      <Kicker text="Thanks for watching" onDark />
      <div style={{ ...line, color: colors.websiteWhite }}>{site.url}</div>
      <div style={{ ...line, color: colors.matureYellow }}>{site.email}</div>
    </div>
  </FlowFrame>
);
