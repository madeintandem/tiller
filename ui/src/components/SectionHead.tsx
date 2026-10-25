// SectionHead is the letterspaced eyebrow the mock sets over a queue card or
// rail panel (".sect .hd", ".sit .shd"), with an optional magenta count
// badge for how many decisions live beneath it. Magenta here is the explicit
// badge prop, never a default fill, per the Global Constraint that magenta
// marks only what needs the human.
//
// headerExtra is the row's trailing slot, right-aligned, named to match
// RailSection's own prop so the two section chromes speak one vocabulary. Home
// puts its recap signal there: the eyebrow row is the first line of the page
// body, so the two questions Home answers, what needs you and what happened,
// land on one line, one of them magenta and the other deliberately not.

import type { ReactNode } from "react";

export default function SectionHead({
  label,
  badge,
  headerExtra,
}: {
  label: string;
  badge?: string;
  headerExtra?: ReactNode;
}) {
  return (
    <div className="eyebrow-row">
      <span className="eyebrow">{label}</span>
      {badge !== undefined && <span className="count-badge">{badge}</span>}
      {headerExtra !== undefined && <span className="eyebrow-row-extra">{headerExtra}</span>}
    </div>
  );
}
