import { useState } from "react";
import type { ReactNode } from "react";
import type { BookkeepingRun, Burst } from "../../lib/bursts";
import { tierBursts } from "../../lib/bursts";

// The milestones-by-default view's folded row: one disclosure standing for a
// run of adjacent bookkeeping bursts ("14 bookkeeping changes (linked,
// reordered, retitled)"). It is a real button with aria-expanded, and opens to
// the ordinary burst rows, so nothing it folds is unreachable. The count is
// events, the same unit every burst row counts in.
// origin is set in by-time order only, where the fold has no section head to
// name its epic: it shows the same quiet mono label an ordinary row there
// carries (BurstRow's origin).
export default function BookkeepingRow<T>({
  run,
  origin,
  renderBurst,
}: {
  run: BookkeepingRun<T>;
  origin?: string;
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
          {origin !== undefined && (
            <>
              {" "}
              <span className="mono muted recap-burst-origin">{origin}</span>
            </>
          )}
        </span>
        <span className="burst-toggle" aria-hidden="true">
          {open ? "– close" : "+ open"}
        </span>
      </button>
      {open && <ul className="recap-bursts">{run.bursts.map((b, i) => renderBurst(b, i))}</ul>}
    </li>
  );
}

// A fold never spans two groups (runKey), so its first item names the
// origin of the whole run.
function runOrigin<T>(run: BookkeepingRun<T>, originOf: (item: T) => string): string | undefined {
  return run.bursts.length > 0 ? originOf(run.bursts[0]) : undefined;
}

// One day group's (or one flat day's) burst list, tiered. renderBurst draws an
// ordinary burst row; the caller keeps ownership of its per-row props (group
// key, origin), so epic mode and time mode share this one fold. Items default
// to plain Bursts; flat mode passes its burst-plus-origin items with burstOf
// and runKey (see tierBursts) so a run never crosses an epic, and originOf so
// the fold names that epic.
export function TieredBursts<T = Burst>({
  bursts,
  showAll,
  burstOf,
  runKey,
  originOf,
  renderBurst,
}: {
  bursts: T[];
  showAll: boolean;
  burstOf?: (item: T) => Burst;
  runKey?: (item: T) => string | number;
  originOf?: (item: T) => string;
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
          <BookkeepingRow
            key={i}
            run={item as BookkeepingRun<T>}
            origin={originOf === undefined ? undefined : runOrigin(item as BookkeepingRun<T>, originOf)}
            renderBurst={renderBurst}
          />
        ) : (
          renderBurst(item as T, i)
        ),
      )}
    </>
  );
}
