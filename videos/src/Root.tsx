import React from "react";
import { Composition } from "remotion";
import { LowerThird } from "./components/LowerThird";
import { Frame } from "./components/Frame";
import { Terminal } from "./components/Terminal";
import { TitleCard } from "./components/TitleCard";
import type { Step } from "./check/diff";
import { video } from "./theme";

const Smoke: React.FC = () => (
  <>
    <TitleCard kicker="Tiller how-to" heading="Smoke test" body="MiT theme and frame components." />
    <LowerThird text="Lower third sample" />
  </>
);

const TERM_FRAMES = 15 * video.fps;

// Steps arrive as input props (the bundle runs in a browser, so it cannot read snapshots).
const TermSmoke: React.FC<{ steps: Step[] }> = ({ steps }) => (
  <Frame>
    <div style={{ position: "relative", width: "100%", height: "100%" }}>
      <Terminal steps={steps} startFrame={0} endFrame={TERM_FRAMES} />
    </div>
  </Frame>
);

export const RemotionRoot: React.FC = () => (
  <>
  <Composition
    id="Smoke"
    component={Smoke}
    durationInFrames={5 * video.fps}
    fps={video.fps}
    width={video.width}
    height={video.height}
  />
  <Composition
    id="TermSmoke"
    component={TermSmoke}
    defaultProps={{ steps: [] as Step[] }}
    durationInFrames={TERM_FRAMES}
    fps={video.fps}
    width={video.width}
    height={video.height}
  />
  </>
);
