import React from "react";
import { Img, staticFile } from "remotion";
import "../fonts";
import { colors, fonts } from "../theme";

export const THUMB = { width: 1280, height: 720 } as const;

// Title in Space Grotesk on the MiT dark indigo field, with the sigil. The type is large
// so it still reads at 320x180; longer titles step down to stay inside the frame.
export const Thumbnail: React.FC<{ title: string }> = ({ title }) => {
  const n = title.length;
  const size = n <= 24 ? 150 : n <= 40 ? 124 : n <= 60 ? 104 : 88;
  return (
    <div style={{ position: "absolute", inset: 0, background: colors.darkIndigo, overflow: "hidden" }}>
      <div style={{ position: "absolute", left: 0, top: 0, bottom: 0, width: 28, background: colors.matureYellow }} />
      <Img
        src={staticFile("brand/mit-sigil-website-white.svg")}
        style={{ position: "absolute", right: 56, bottom: 48, width: 150, height: 150 }}
      />
      <div style={{ position: "absolute", left: 96, right: 96, top: 0, bottom: 150, display: "flex", alignItems: "center" }}>
        <h1
          style={{
            fontFamily: fonts.display,
            fontWeight: 700,
            fontSize: size,
            lineHeight: 1.04,
            margin: 0,
            color: colors.websiteWhite,
          }}
        >
          {title}
        </h1>
      </div>
    </div>
  );
};
