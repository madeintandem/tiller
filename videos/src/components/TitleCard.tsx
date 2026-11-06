import React from "react";
import { Frame } from "./Frame";
import { Kicker } from "./Kicker";
import { colors, fonts } from "../theme";

export const TitleCard: React.FC<{ kicker: string; heading: string; body?: string }> = ({
  kicker,
  heading,
  body,
}) => (
  <Frame>
    <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", height: "100%", maxWidth: 1300 }}>
      <Kicker text={kicker} />
      <h1
        style={{
          fontFamily: fonts.display,
          fontWeight: 700,
          fontSize: 104,
          lineHeight: 1.05,
          margin: 0,
          color: colors.darkIndigo,
        }}
      >
        {heading}
      </h1>
      {body ? (
        <p style={{ fontFamily: fonts.body, fontSize: 38, lineHeight: 1.5, margin: "32px 0 0", color: colors.sharpieBlack }}>
          {body}
        </p>
      ) : null}
    </div>
  </Frame>
);
