import React from "react";
import { Composition } from "remotion";
import { LowerThird } from "./components/LowerThird";
import { TitleCard } from "./components/TitleCard";
import { video } from "./theme";

const Smoke: React.FC = () => (
  <>
    <TitleCard kicker="Tiller how-to" heading="Smoke test" body="MiT theme and frame components." />
    <LowerThird text="Lower third sample" />
  </>
);

export const RemotionRoot: React.FC = () => (
  <Composition
    id="Smoke"
    component={Smoke}
    durationInFrames={5 * video.fps}
    fps={video.fps}
    width={video.width}
    height={video.height}
  />
);
