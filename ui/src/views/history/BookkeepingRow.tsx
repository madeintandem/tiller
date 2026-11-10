import { useState } from "react";
import type { ReactNode } from "react";
import type { BookkeepingRun, Burst } from "../../lib/bursts";
import { tierBursts } from "../../lib/bursts";

// The milestones-by-default view's folded row: one disclosure standing for a
// run of adjacent bookkeeping bursts ("14 bookkeeping changes (linked,
// reordered, retitled)"). It is a real button with aria-expanded, and opens to
// the ordinary burst rows, so nothing it folds is unreachable. The count is
// events, the same unit every burst row counts in.
export default function BookkeepingRow<T>({
  run,
  renderBurst,
}: {
  run: BookkeepingRun<T>;
  renderBurst: (item: T, index: number) => ReactNode;
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
        <span className="t" aria-hidden="true" />
        <span className="what what-run">
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
// key, origin), so epic mode and time mode share this one fold. Items default
// to plain Bursts; flat mode passes its burst-plus-origin items with burstOf
// and runKey (see tierBursts) so a run never crosses an epic.
export function TieredBursts<T = Burst>({
  bursts,
  showAll,
  burstOf,
  runKey,
  renderBurst,
}: {
  bursts: T[];
  showAll: boolean;
  burstOf?: (item: T) => Burst;
  runKey?: (item: T) => string | number;
  renderBurst: (item: T, index: number) => ReactNode;
}) {
  const tiered =
    burstOf === undefined
      ? tierBursts(bursts as unknown as Burst[], showAll)
      : tierBursts(bursts, showAll, { burstOf, runKey });
  return (
    <>
      {(tiered as (T | BookkeepingRun<T>)[]).map((item, i) =>
        typeof item === "object" && item !== null && "kind" in item && item.kind === "bookkeeping" ? (
          <BookkeepingRow key={i} run={item as BookkeepingRun<T>} renderBurst={renderBurst} />
        ) : (
          renderBurst(item as T, i)
        ),
      )}
    </>
  );
}
