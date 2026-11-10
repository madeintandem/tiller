import { useState } from "react";
import type { ReactNode } from "react";
import type { BookkeepingRun, Burst } from "../../lib/bursts";
import { tierBursts } from "../../lib/bursts";

// The milestones-by-default view's folded row: one disclosure standing for a
// run of adjacent bookkeeping bursts ("14 bookkeeping changes (linked,
// reordered, retitled)"). It is a real button with aria-expanded, and opens to
// the ordinary burst rows, so nothing it folds is unreachable. The count is
// events, the same unit every burst row counts in.
export default function BookkeepingRow({
  run,
  renderBurst,
}: {
  run: BookkeepingRun;
  renderBurst: (burst: Burst, index: number) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <li className="recap-burst recap-bookkeeping">
      <button
        type="button"
        className="hrow recap-bookkeeping-toggle"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <span className="what">
          <b>
            {run.count} bookkeeping {run.count === 1 ? "change" : "changes"}
          </b>{" "}
          <span className="muted">({run.verbs.join(", ")})</span>
        </span>
        <span className="burst-toggle" aria-hidden="true">
          {open ? "– close" : "+ open"}
        </span>
      </button>
      {open && <ul className="recap-bursts">{run.bursts.map((b, i) => renderBurst(b, i))}</ul>}
    </li>
  );
}

// One day group's (or one flat day's) burst list, tiered. renderBurst draws an
// ordinary burst row; the caller keeps ownership of its per-row props (group
// key, origin), so epic mode and time mode share this one fold.
export function TieredBursts({
  bursts,
  showAll,
  renderBurst,
}: {
  bursts: Burst[];
  showAll: boolean;
  renderBurst: (burst: Burst, index: number) => ReactNode;
}) {
  return (
    <>
      {tierBursts(bursts, showAll).map((item, i) =>
        "kind" in item ? (
          <BookkeepingRow key={i} run={item} renderBurst={renderBurst} />
        ) : (
          renderBurst(item, i)
        ),
      )}
    </>
  );
}
