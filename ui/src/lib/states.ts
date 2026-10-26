// This module is the ONLY place a status maps onto a state's CSS custom
// property: StateDot, StateBar, and Chip all paint through here rather than
// building a var() reference themselves, so the seven-state palette has
// exactly one source of truth for HOW a wire status resolves to a token.
// The palette's actual color values live in tokens.css alone (the --st-
// <state> / --st-<state>-t pairs, light-dark() themed) - this file never
// touches a hex or an rgb triple, it only ever emits the var() reference,
// which is what lets every consumer theme automatically with no JS involved.
// A component painting a *runtime* status value still has to compute that
// reference in JS rather than select a static CSS class (a CSS class can't
// branch on an arbitrary prop), so stateColor/statusColor exist for exactly
// that job.
//
// Two functions on purpose. stateColor is the narrow core over the seven
// chart states the legend names. statusColor is the TOTAL public face over
// the wire's full status vocabulary (plan amendment 74dfe3a): the Go side
// also ships proposed (a task awaiting approval), a question's open and
// answered, cancelled, archived, and whatever a future lifecycle adds, all
// through the same string-typed status fields. Pages consume statusColor
// and never cast to NodeStatus or invent their own mapping.
//
// A status never renders as color alone: the word or a count caption is
// always adjacent (see Chip, which always renders the word, and StateBar's
// caption prop). That adjacency is what lets every non-chart status fold
// onto a chart color without lying: the word carries the exact state, the
// color carries its register.

import type { NodeStatus } from "../api";

// The seven chart states, and the sole enumeration of them in this file:
// isChartState below is the only thing that reads it. stateColor itself
// needs no lookup table at all, since a state's own string IS the suffix of
// its CSS variable name.
const CHART_STATES: ReadonlySet<string> = new Set<NodeStatus>([
  "draft",
  "specced",
  "ready",
  "active",
  "in_review",
  "done",
  "blocked",
]);

// The wire statuses that carry a chart state's semantic without being one:
// proposed is specced's awaiting-your-signature moment at task grain;
// in_progress and check are a task's (and a bug's or chore's, which share
// its lifecycle) two working states, the agent's move on live work, which
// is what active means one grain up (before the node header pass,
// 2026-08-30, both fell to draft and a task mid-work painted gray); a
// question open is work sitting on the human like in_review; answered is
// settled like done. A Map rather than a plain object so an
// Object.prototype member name ("toString") can never leak a prototype-
// chain hit past the fold.
const FOLDED_STATUSES: ReadonlyMap<string, NodeStatus> = new Map([
  ["proposed", "specced"],
  ["in_progress", "active"],
  ["check", "active"],
  ["open", "in_review"],
  ["answered", "done"],
]);

function isChartState(status: string): status is NodeStatus {
  return CHART_STATES.has(status);
}

// stateColor: the narrow core, exact by construction over the seven chart
// states. Returns the CSS var() reference, never a color - tokens.css owns
// the actual value, themed via light-dark(), and this function's only job
// is naming the right custom property. Callers holding an arbitrary wire
// status use statusColor instead.
export function stateColor(state: NodeStatus): string {
  return `var(--st-${state})`;
}

// foldStatus: total over ANY string, the bucket a wire status paints as,
// without the var() lookup. The seven chart states fold to themselves, the
// known wire statuses fold to their semantic sibling, and everything else
// (cancelled, archived, a status this build has never heard of) rests at the
// draft bucket. Exported so a caller composing its own chart (a StateBar mix
// grouping raw wire statuses into the seven-state legend, task C1's header
// band stats) folds through the exact same table statusColor paints with,
// rather than a second guess at the mapping that could drift from it.
export function foldStatus(status: string): NodeStatus {
  if (isChartState(status)) {
    return status;
  }
  return FOLDED_STATUSES.get(status) ?? "draft";
}

// statusColor: total over ANY string. The seven chart states keep their own
// var(), the folded statuses borrow their semantic sibling's, and everything
// else (cancelled, archived, a status this build has never heard of) rests
// at the draft slate, since the adjacent word carries the meaning and an
// unknown status must degrade to quiet, never to a blank or a crash.
export function statusColor(status: string): string {
  return stateColor(foldStatus(status));
}

// statusTextColor: the text twin of statusColor, total over ANY string the
// same way and folding through the same table, so a chip's word and its dot
// can never name two different states. Each --st-<state>-t is tokens.css's
// own darkened (light theme) / lightened (dark theme) twin of its fill,
// verbatim from the mock (the retired console mock) - the contrast trade lives
// there now, not in this file.
export function statusTextColor(status: string): string {
  return `var(--st-${foldStatus(status)}-t)`;
}
