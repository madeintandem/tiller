import React from "react";
import { AbsoluteFill, Freeze, Html5Audio, OffthreadVideo, Sequence, staticFile } from "remotion";
import type { Segment, Timeline } from "../timeline/build";
import { Frame } from "./Frame";
import { LowerThird } from "./LowerThird";
import { Outro } from "./Outro";
import { Terminal } from "./Terminal";
import { TitleCard } from "./TitleCard";
import { chapterFrames } from "./timing";

const HOST_NAME = "JC Grubbs, Founder & CEO, Made In Tandem";
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
const Visual: React.FC<{ seg: Segment; lead: number; spoken: number; fps: number; videoIndex: number }> = ({ seg, lead, spoken, fps, videoIndex }) => {
  const show = seg.show;
  switch (show.kind) {
    case "card":
      return (
        <TitleCard
          kicker={show.kicker}
          heading={show.heading}
          body={show.body}
          points={show.points}
          pointFrames={seg.pointTimes?.map((t) => lead + Math.round(t * fps))}
          art={show.art}
          backdrop={show.backdrop}
          from={lead}
          seen={seg.seen}
          videoIndex={videoIndex}
          track={seg.track}
        />
      );
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
  const intro = timeline.intro;
  const introFrames = intro ? f(intro.duration) : 0;
  const last = timeline.segments[timeline.segments.length - 1];
  // "v2" is the second video of the series; anything else counts as the first.
  const videoIndex = Math.max(0, Number(/^v(\d+)$/.exec(timeline.id)?.[1] ?? 1) - 1);
  return (
    <AbsoluteFill style={{ background: "black" }}>
      {/* The filmed intro, when there is one: full bleed, with the host's name for the first seconds. */}
      {intro && (
        <Sequence durationInFrames={introFrames} name="intro">
          <OffthreadVideo src={staticFile(intro.path)} style={{ width: "100%", height: "100%" }} />
          <Sequence durationInFrames={Math.min(introFrames, f(INTRO_NAME_SECONDS))}>
            <LowerThird text={HOST_NAME} />
          </Sequence>
        </Sequence>
      )}

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
              <Visual seg={seg} lead={from - slotStart} spoken={spoken} fps={fps} videoIndex={videoIndex} />
            </Sequence>
            <Sequence from={from} durationInFrames={Math.max(1, spoken)} name={`${seg.beatId} narration`} layout="none">
              <Html5Audio src={staticFile(seg.audio)} />
            </Sequence>
            {/* With no intro clip the first beat is the hello, so the host's name goes on it. */}
            {!intro && i === 0 && !seg.chapter && (
              <Sequence from={from} durationInFrames={chapterFrames(from, slotEnd, fps, INTRO_NAME_SECONDS)} name="host name">
                <LowerThird text={HOST_NAME} onDark={seg.show.kind === "card" && seg.show.backdrop === "flow"} />
              </Sequence>
            )}
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
