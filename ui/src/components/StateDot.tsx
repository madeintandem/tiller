// StateDot is the node's own color: a small filled circle, never shown
// without an adjacent word or count caption (Global Constraint, chart-table
// pass — a state never renders as color alone). It accepts any wire status
// string and paints through statusColor's total mapping, so a page hands it
// node.status verbatim — proposed, open, a value this build has never seen —
// and never casts to NodeStatus or risks a blank dot. It carries no text of
// its own, so it is decorative to assistive tech; whatever sits beside it
// (Chip's word, a row's title, a bar's caption) is the accessible content.

import { statusColor } from "../lib/states";

export default function StateDot({ status, size = 8 }: { status: string; size?: number }) {
  return (
    <span
      className="state-dot"
      data-status={status}
      aria-hidden="true"
      style={{ width: size, height: size, background: statusColor(status) }}
    />
  );
}
