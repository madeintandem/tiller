import React from "react";
import { Img, staticFile } from "remotion";
import { Frame } from "./Frame";
import { Kicker } from "./Kicker";
import { colors, fonts } from "../theme";

export const Outro: React.FC = () => (
  <Frame wordmark={false}>
    <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", height: "100%" }}>
      <Kicker text="Made in Tandem" />
      <h1 style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 88, margin: "0 0 40px", color: colors.darkIndigo }}>
        Thanks for watching.
      </h1>
      <Img src={staticFile("brand/mit-logo-dark-indigo.svg")} style={{ width: 480 }} />
    </div>
  </Frame>
);
