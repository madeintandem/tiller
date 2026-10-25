// Chip is the tinted status pill (the mock's dh ready/active chips):
// StateDot's colored dot plus the status word,
// tinted at the mock's own --tint (13% light / 22% dark - tokens.css bumps
// it under a dark scheme via a media query, since light-dark() can't carry
// a bare percentage) so the whole pill reads in the status's hue rather
// than carrying color on the dot alone (mock:202's color-mix idiom,
// composed here since the fill has to ride a runtime status). It accepts
// any wire status
// string through statusColor's total mapping, so a page hands it
// node.status verbatim and never casts to NodeStatus. Chip always shows the
// word: children lets a caller supply it explicitly (every mock usage does,
// verbatim, e.g. "ready"), and omitting children still falls back to the
// raw status string, so the pill can never end up carrying color with no
// adjacent text.
//
// title is an optional native tooltip, plain pass-through to the span:
// the node page's Drifted chip (task 12) is one chip regardless of how many
// drift markers touch a node, and the title is where the rest of them are
// named. Every existing caller omits it, and React renders no title
// attribute at all for an undefined prop, so this is additive.

import type { ReactNode } from "react";
import { statusColor, statusTextColor } from "../lib/states";
import StateDot from "./StateDot";

// size is the node header's one addition (node header pass, 2026-08-30): the
// page's own state chip renders at the hero size, a step above the 11px pill
// the crumbs and rows keep, so the node's state outranks its ancestors' at a
// glance. The word, the dot, and the color ride the exact same path; only
// the class (and the dot's diameter) changes.
export default function Chip({
  status,
  children,
  title,
  size,
}: {
  status: string;
  children?: ReactNode;
  title?: string;
  size?: "hero";
}) {
  const color = statusColor(status);
  // The fill is statusColor's var() mixed toward --panel at --tint (mock's
  // own idiom, mock:202); the word is statusTextColor's twin, because the
  // mock does not paint both from one value (mock:87-124 pairs each --st-
  // <state> with its own --st-<state>-t, and lib/states.ts on why). At 11px
  // the word is the whole point of the chip, so it never rides the fill's
  // own color.
  const text = statusTextColor(status);
  return (
    <span
      className={size === "hero" ? "chip chip-hero" : "chip"}
      data-status={status}
      title={title}
      style={{ color: text, background: `color-mix(in srgb, ${color} var(--tint), var(--panel))` }}
    >
      <StateDot status={status} size={size === "hero" ? 8 : 6} />
      {children ?? status}
    </span>
  );
}
