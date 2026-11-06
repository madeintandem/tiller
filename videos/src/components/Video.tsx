import React from "react";
import { AbsoluteFill, Freeze, Html5Audio, OffthreadVideo, Sequence, staticFile } from "remotion";
import type { Segment, Timeline } from "../timeline/build";
import { Frame } from "./Frame";
import { LowerThird } from "./LowerThird";
import { Outro } from "./Outro";
import { Terminal } from "./Terminal";
import { TitleCard } from "./TitleCard";
import { chapterFrames } from "./timing";

const INTRO_NAME = "JC Grubbs, Made In Tandem";
const INTRO_NAME_SECONDS = 4;

// A console capture plays from the start of its slot; shorter than the slot, its last
// frame holds; longer, the slot's end cuts it.
const Console: React.FC<{ src: string; frames?: number }> = ({ src, frames }) => (
  <Frame>
    <Freeze frame={Math.max(0, (frames ?? 0) - 1)} active={(f) => frames !== undefined && f >= frames}>
      <OffthreadVideo src={staticFile(src)} muted style={{ width: "100%", height: "100%", objectFit: "contain" }} />
    </Freeze>
  </Frame>
);

// What a segment shows. `lead` is how many frames of gap precede its narration in the slot.
const Visual: React.FC<{ seg: Segment; lead: number; spoken: number; fps: number }> = ({ seg, lead, spoken, fps }) => {
  const show = seg.show;
  switch (show.kind) {
    case "card":
      return <TitleCard kicker={show.kicker} heading={show.heading} body={show.body} />;
    case "term":
      return (
        <Frame>
          <div style={{ position: "relative", width: "100%", height: "100%" }}>
            <Terminal steps={seg.steps ?? []} startFrame={lead} endFrame={lead + spoken} />
          </div>
        </Frame>
      );
    case "console":
      return (
        <Console
          src={seg.capturePath ?? ""}
          frames={seg.captureSeconds === undefined ? undefined : Math.round(seg.captureSeconds * fps)}
        />
      );
    case "outro":
      return <Outro />;
  }
};

export const Video: React.FC<{ timeline: Timeline }> = ({ timeline }) => {
  const { fps } = timeline;
  const f = (seconds: number) => Math.round(seconds * fps);
  const introFrames = f(timeline.intro.duration);
  const last = timeline.segments[timeline.segments.length - 1];
  return (
    <AbsoluteFill style={{ background: "black" }}>
      {/* JC on camera, full bleed, with his name for the first seconds. */}
      <Sequence durationInFrames={introFrames} name="intro">
        <OffthreadVideo src={staticFile(timeline.intro.path)} style={{ width: "100%", height: "100%" }} />
        <Sequence durationInFrames={Math.min(introFrames, f(INTRO_NAME_SECONDS))}>
          <LowerThird text={INTRO_NAME} />
        </Sequence>
      </Sequence>

      {timeline.segments.map((seg, i) => {
        // Each slot runs from the end of the previous clip, so the gap shows the coming visual.
        const slotStart = i === 0 ? introFrames : f(timeline.segments[i - 1].end);
        // The last outro beat carries straight on through the outro tail.
        const slotEnd = seg === last && seg.show.kind === "outro" ? f(timeline.duration) : f(seg.end);
        const from = f(seg.start);
        const spoken = f(seg.end) - from;
        return (
          <React.Fragment key={seg.beatId}>
            <Sequence from={slotStart} durationInFrames={Math.max(1, slotEnd - slotStart)} name={seg.beatId}>
              <Visual seg={seg} lead={from - slotStart} spoken={spoken} fps={fps} />
            </Sequence>
            <Sequence from={from} durationInFrames={Math.max(1, spoken)} name={`${seg.beatId} narration`} layout="none">
              <Html5Audio src={staticFile(seg.audio)} />
            </Sequence>
            {seg.chapter && (
              <Sequence from={from} durationInFrames={chapterFrames(from, slotEnd, fps)} name={`${seg.beatId} chapter`}>
                <LowerThird text={seg.chapter} />
              </Sequence>
            )}
          </React.Fragment>
        );
      })}

      {!(last && last.show.kind === "outro") && (
        <Sequence from={f(timeline.outro.start)} durationInFrames={Math.max(1, f(timeline.outro.end) - f(timeline.outro.start))} name="outro">
          <Outro />
        </Sequence>
      )}
    </AbsoluteFill>
  );
};
