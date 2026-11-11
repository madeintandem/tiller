import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { Img, staticFile } from "remotion";
import { CardArt, SIDE_ARTS, Tracker } from "./art/Art";
import { FlowFrame } from "./FlowBackdrop";
import { Frame } from "./Frame";
import { Kicker } from "./Kicker";
import { colors, fonts, site, wordmark } from "../theme";
import { ART_POINTS, type Art, type Point } from "../script/schema";

const POINT_IN_FRAMES = 9; // 300 ms at 30 fps

// One line of a slide: an olive rule and the text, rising in from `at` (a frame of the card).
const PointLine: React.FC<{ text: string; at: number; size: number }> = ({ text, at, size }) => {
  const frame = useCurrentFrame();
  const t = interpolate(frame, [at, at + POINT_IN_FRAMES], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 28,
        opacity: t,
        transform: `translateY(${(1 - t) * 14}px)`,
        fontFamily: fonts.body,
        fontWeight: 500,
        fontSize: size,
        lineHeight: 1.25,
        color: colors.sharpieBlack,
      }}
    >
      <span style={{ flex: "none", width: 44, height: 6, borderRadius: 3, background: colors.gracefulOlive }} />
      <span>{text}</span>
    </div>
  );
};

export type TitleCardProps = {
  kicker: string;
  heading: string;
  body?: string;
  // With `points` a card is a slide: they come up one at a time at `pointFrames` (frames of the
  // card, one per point; a missing one shows its point from the start). A stepped graphic
  // (around, parts, process) draws the points itself instead of the bullet list.
  points?: Point[];
  pointFrames?: number[];
  art?: Art;
  // "flow" makes this the title screen: full-frame on the animated flow field, light type.
  backdrop?: "flow";
  // The frame of the card at which its narration starts.
  from?: number;
  seen?: string[];
  videoIndex?: number;
  // On a chapter card: the video's chapters and which one this is.
  track?: { names: string[]; index: number };
};

// The title screen: the type sits in the left half, over the quiet part of the flow field,
// with the product's mark above it and the company's wordmark and address along the bottom.
const TitleScreen: React.FC<{ kicker: string; heading: string; body?: string; product: boolean }> = ({ kicker, heading, body, product }) => (
  <FlowFrame>
    <div style={{ position: "absolute", left: 120, top: 96, bottom: 330, width: 900, display: "flex", flexDirection: "column", justifyContent: "center" }}>
      {product && (
        <div style={{ display: "flex", alignItems: "center", gap: 22, marginBottom: 44 }}>
          {/* The mark is drawn in dark indigo; on this backdrop it is knocked out to white. */}
          <Img src={staticFile("brand/tiller-logo.svg")} style={{ width: 104, filter: "brightness(0) invert(1)" }} />
          <span style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 60, letterSpacing: "-0.01em", color: colors.websiteWhite }}>Tiller</span>
        </div>
      )}
      <Kicker text={kicker} onDark />
      <h1 style={{ fontFamily: fonts.display, fontWeight: 700, fontSize: 92, lineHeight: 1.05, margin: 0, textWrap: "balance", color: colors.websiteWhite }}>{heading}</h1>
      {body ? <p style={{ fontFamily: fonts.body, fontSize: 36, lineHeight: 1.5, margin: "30px 0 0", color: colors.fogGrey }}>{body}</p> : null}
    </div>
    <Img src={staticFile("brand/mit-logo-website-white.svg")} style={{ position: "absolute", left: wordmark.left, bottom: wordmark.bottom, width: 260 }} />
    <div
      style={{
        position: "absolute",
        right: 120,
        bottom: wordmark.bottom + 12,
        fontFamily: fonts.display,
        fontWeight: 700,
        fontSize: 54,
        letterSpacing: "-0.01em",
        color: colors.websiteWhite,
        // A soft plate of the backdrop's own color, so the address reads over the field lines.
        background: "rgba(0, 17, 41, 0.82)",
        boxShadow: "0 0 28px 22px rgba(0, 17, 41, 0.82)",
        borderRadius: 8,
      }}
    >
      {site.url}
    </div>
  </FlowFrame>
);

export const TitleCard: React.FC<TitleCardProps> = ({ kicker, heading, body, points, pointFrames, art, backdrop, from = 0, seen, videoIndex = 0, track }) => {
  if (backdrop === "flow") return <TitleScreen kicker={kicker} heading={heading} body={body} product={art === "logo"} />;

  const side = art !== undefined && SIDE_ARTS.includes(art);
  const wide = art !== undefined && !side;
  // The big full-width graphics need the room, so their heading steps down.
  const dense = art === "cli" || art === "process";
  const bullets = points && !(art && ART_POINTS[art] !== undefined);
  const headingSize = dense ? 60 : side ? (points ? 66 : 84) : points || wide ? 84 : 104;
  const graphic = art && <CardArt art={art} points={points} pointFrames={pointFrames} from={from} seen={seen} videoIndex={videoIndex} />;

  const text = (
    <>
      <Kicker text={kicker} />
      <h1
        style={{
          fontFamily: fonts.display,
          fontWeight: 700,
          fontSize: headingSize,
          lineHeight: 1.05,
          margin: 0,
          // Balanced lines, so a two-line heading never strands one word.
          textWrap: "balance",
          color: colors.darkIndigo,
        }}
      >
        {heading}
      </h1>
      {body ? (
        <p style={{ fontFamily: fonts.body, fontSize: dense ? 30 : side ? 32 : 38, lineHeight: 1.5, margin: `${dense ? 14 : 32}px 0 0`, color: colors.sharpieBlack }}>{body}</p>
      ) : null}
      {bullets ? (
        <div style={{ display: "flex", flexDirection: "column", gap: 26, marginTop: 52 }}>
          {points.map((p, i) => (
            <PointLine key={i} text={p.text} at={pointFrames?.[i] ?? -POINT_IN_FRAMES} size={side ? 40 : 48} />
          ))}
        </div>
      ) : null}
    </>
  );

  return (
    <Frame>
      {side ? (
        <div style={{ display: "flex", alignItems: "center", gap: 56, height: "100%" }}>
          <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", justifyContent: "center" }}>{text}</div>
          <div style={{ flex: "none" }}>{graphic}</div>
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", height: "100%" }}>
          <div style={{ maxWidth: wide ? undefined : 1300 }}>{text}</div>
          {wide && <div style={{ marginTop: dense ? 30 : 56 }}>{graphic}</div>}
          {track && !art && <Tracker names={track.names} index={track.index} />}
        </div>
      )}
    </Frame>
  );
};
