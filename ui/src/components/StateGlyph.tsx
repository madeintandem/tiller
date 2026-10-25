// StateGlyph is the Explore tree's status mark (JC, 2026-10-18): StateDot's
// small circles were too close in color to tell apart with color blindness,
// so each of the seven chart states draws its own SHAPE here and color is
// only the second signal. The shapes read as progress: a dashed ring
// (draft), a ring (specced), a ring with a center (ready), half filled
// (active), three-quarters filled (in review), a solid check (done), and a
// solid no-entry bar (blocked). A wire status folds onto its chart sibling
// through lib/states.ts's foldStatus, the same table every dot and chip
// paints with, so the shape and the color can never name two states.
//
// Unlike StateDot, a tree row carries no status word beside it, so the glyph
// carries its status label itself, as its accessible name (role="img") and
// as a hover tooltip (JC, 2026-10-20: tooltips are how people discover what
// the shapes mean). The native SVG <title> was slow and tiny, so it is gone;
// in its place a styled tooltip shows the same label about 150ms after a
// mouse or pen hover. Touch is ignored, since a tap never sends the leave
// that would hide it. The tooltip renders in a portal with fixed
// positioning so the tree's scrolling rail never clips it. It sits above
// the glyph, centred on it, so it never covers the row's own title; it
// flips below near the top of the viewport and stays inside the viewport's
// side edges. The inner check and bar draw in --panel rather than white so
// they stay crisp on the lighter dark-theme fills.

import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent, type ReactElement } from "react";
import { createPortal } from "react-dom";
import type { NodeStatus } from "../api";
import { foldStatus, statusColor } from "../lib/states";

const RING = <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" strokeWidth="1.8" />;

const SHAPES: Record<NodeStatus, ReactElement> = {
  draft: <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeDasharray="2.4 2.1" />,
  specced: RING,
  ready: (
    <>
      {RING}
      <circle cx="8" cy="8" r="2.4" fill="currentColor" />
    </>
  ),
  active: (
    <>
      {RING}
      <path d="M8 4 A4 4 0 0 1 8 12 Z" fill="currentColor" />
    </>
  ),
  in_review: (
    <>
      {RING}
      <path d="M8 4 A4 4 0 1 1 4 8 L8 8 Z" fill="currentColor" />
    </>
  ),
  done: (
    <>
      <circle cx="8" cy="8" r="7" fill="currentColor" />
      <path
        d="M4.8 8.3 L7 10.4 L11.2 5.9"
        fill="none"
        stroke="var(--panel)"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </>
  ),
  blocked: (
    <>
      <circle cx="8" cy="8" r="7" fill="currentColor" />
      <rect x="4.2" y="7" width="7.6" height="2" rx="1" fill="var(--panel)" />
    </>
  ),
};

// The label a status reads as, where Title Casing the wire word is not
// enough; every other status Title Cases its words (statusLabel).
const LABELS: ReadonlyMap<string, string> = new Map([["specced", "Specified"]]);

export function statusLabel(status: string): string {
  return (
    LABELS.get(status) ??
    status
      .split("_")
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(" ")
  );
}

// Quick enough to feel like an answer, slow enough that sweeping the
// pointer down the tree does not flash a tooltip on every row it crosses.
const SHOW_DELAY_MS = 150;
// The gap between glyph and tooltip, and the closest the tooltip comes to
// the viewport's edge.
const GAP = 8;
const EDGE = 8;

export default function StateGlyph({ status, size = 14 }: { status: string; size?: number }) {
  const shape = foldStatus(status);
  const label = statusLabel(status);
  const glyphRef = useRef<SVGSVGElement>(null);
  const timer = useRef<number | undefined>(undefined);
  const [anchor, setAnchor] = useState<DOMRect | null>(null);

  function show(event: PointerEvent) {
    if (event.pointerType === "touch") return;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      setAnchor(glyphRef.current?.getBoundingClientRect() ?? null);
    }, SHOW_DELAY_MS);
  }
  function hide() {
    window.clearTimeout(timer.current);
    setAnchor(null);
  }

  useEffect(() => () => window.clearTimeout(timer.current), []);

  // A scroll moves the glyph out from under a fixed tooltip, so any scroll
  // while one is showing dismisses it rather than leaving it stranded.
  useEffect(() => {
    if (anchor === null) return;
    window.addEventListener("scroll", hide, true);
    return () => window.removeEventListener("scroll", hide, true);
  }, [anchor]);

  return (
    <>
      <svg
        ref={glyphRef}
        className="state-glyph"
        data-status={status}
        data-shape={shape}
        role="img"
        aria-label={label}
        viewBox="0 0 16 16"
        width={size}
        height={size}
        style={{ color: statusColor(status) }}
        onPointerEnter={show}
        onPointerLeave={hide}
      >
        {SHAPES[shape]}
      </svg>
      {anchor !== null && <GlyphTooltip anchor={anchor} label={label} />}
    </>
  );
}

// GlyphTooltip measures itself before it paints (useLayoutEffect), then
// sits above the glyph when it fits and below when it does not, centred on
// the glyph and clamped inside the viewport's side edges.
function GlyphTooltip({ anchor, label }: { anchor: DOMRect; label: string }) {
  const tipRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState<{ left: number; top: number } | null>(null);

  useLayoutEffect(() => {
    const tip = tipRef.current?.getBoundingClientRect();
    if (tip === undefined) return;
    const centred = anchor.left + anchor.width / 2 - tip.width / 2;
    const left = Math.max(EDGE, Math.min(centred, window.innerWidth - EDGE - tip.width));
    const above = anchor.top - GAP - tip.height;
    const top = above >= EDGE ? above : anchor.bottom + GAP;
    setPosition({ left, top });
  }, [anchor]);

  return createPortal(
    <div
      ref={tipRef}
      className="state-glyph-tip"
      role="tooltip"
      style={position === null ? { visibility: "hidden" } : { left: position.left, top: position.top }}
    >
      {label}
    </div>,
    document.body,
  );
}
