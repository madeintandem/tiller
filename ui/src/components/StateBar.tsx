// StateBar is the children's-mix bar (the mock's stat and epic bars): one
// flex segment per non-zero state in the mix, sized to its share of the
// total, with 2px gaps between segments and the tint track showing through
// wherever the mix is empty (a node with no children yet, or all-zero
// counts). A bar of color alone conveys nothing to assistive tech, so the
// adjacent caption prop becomes its accessible name (role="img"); a caller
// that also renders its own visible caption text beside the bar can still
// pass the same string here so the bar itself is announced correctly rather
// than skipped or read as a wall of color.

import type { NodeStatus } from "../api";
import { statusColor } from "../lib/states";

export interface StateMixEntry {
  state: NodeStatus;
  n: number;
}

export default function StateBar({
  mix,
  width,
  caption,
}: {
  mix: StateMixEntry[];
  width?: number;
  caption?: string;
}) {
  const segments = mix.filter((entry) => entry.n > 0);
  return (
    <div
      className="state-bar"
      style={width === undefined ? undefined : { width }}
      role={caption === undefined ? undefined : "img"}
      aria-label={caption}
      aria-hidden={caption === undefined ? true : undefined}
    >
      {segments.map((entry, i) => (
        <span
          key={`${entry.state}-${i}`}
          className="state-bar-segment"
          data-state={entry.state}
          // The mix is typed NodeStatus (bars are page-composed), but the
          // color lookup still rides the total mapping so a cast smuggled in
          // upstream degrades to the draft slate instead of a blank segment.
          style={{ flex: entry.n, background: statusColor(entry.state) }}
        />
      ))}
    </div>
  );
}
