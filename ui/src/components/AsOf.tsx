// AsOf is the topbar's freshness affordance (JC's ruling, console v2 phase 2,
// 2026-08-10: it lives in the topbar beside the theme toggle, mock's own
// .asof, not in a reserved slot on Home's header band - ui-surface.md's
// amended header-band sentence). It tracks the last successful completion of
// ANY read through api.ts's one fetch choke point (fetchJSON), via a tiny
// subscribable (getLastFetchAt/subscribeLastFetch) rather than owning a fetch
// of its own: the topbar mounts once, above every view, and has no reason to
// know which view's request just landed, and this way "synced" states
// exactly what this browser tab actually has in hand, never a guess.
//
// The refresh control (shipped after the first
// pass held it back): the console has no polling, so without a remedy the
// asof line was a pure staleness signal with nothing the reader could do
// about it. ↻ calls api.ts's bumpFetchGeneration(), which hooks.ts's
// useFetch already subscribes to and folds into its own fetch effect's
// dependency array beside path - one bump re-issues every currently mounted
// request in place, through the exact effect a path change already runs.
// This button invents no second refetch path: it is the one existing
// mechanism's own trigger, exposed.
//
// What "synced" names: the single newest successful read across the whole
// app, one global timestamp, not a per-panel freshness guarantee. A visible
// panel that has not itself refetched since a neighbor's read landed can be
// holding older data than this line implies; the line only ever claims that
// SOMETHING just landed, never that everything on screen did.
//
// The in-flight guard: a bare click handler fired
// one generation bump per click with nothing stopping a rapid double-click
// from firing two, superseded safely but request-spam-shaped. `refreshing`
// goes true the moment this component's own click bumps the generation, and
// the button disables for the duration - a second click in that window is a
// no-op, belt-and-suspenders alongside `disabled` itself. It clears on the
// SAME subscribeLastFetch signal the asof line already reads: the next
// successful read landing anywhere in the app, not a tracker of this click's
// own requests specifically (no new machinery - the two subscribables this
// file already reads are the whole guard). That is the same honest
// imprecision "synced" itself carries: a good-enough signal that something
// just landed, not a per-request completion guarantee. A run where every
// in-flight request happens to fail leaves the button disabled until the
// next unrelated read succeeds - the same trade the rest of this file makes
// rather than inventing a dedicated in-flight counter for one button.
import { useEffect, useState } from "react";
import { bumpFetchGeneration, getLastFetchAt, subscribeLastFetch } from "../api";

export default function AsOf() {
  const [at, setAt] = useState<Date | null>(() => getLastFetchAt());
  // A second clock, independent of the subscription: without it "4m ago"
  // would freeze at whatever it read when the last fetch landed and never
  // advance again until the NEXT fetch happens to trigger a re-render.
  const [now, setNow] = useState<Date>(() => new Date());
  const [refreshing, setRefreshing] = useState(false);

  useEffect(
    () =>
      subscribeLastFetch((next) => {
        setAt(next);
        setRefreshing(false);
      }),
    [],
  );

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  // Nothing has resolved yet (the very first paint, before /api/status even
  // returns): silence rather than a claim about freshness nobody has earned.
  // The refresh control has nothing to refresh yet either, so it stays
  // silent alongside the text rather than rendering alone.
  if (at === null) {
    return null;
  }
  return (
    <span className="asof">
      {/* Its own span, not a bare text node beside the button: the two
          together read as one flat mono line (the mock's own single-span
          markup), but keeping the relative-time text in its own element
          gives every reader of it, human or test, a precise target instead
          of "the .asof span's text minus the button". */}
      <span>{relativeSince(at, now)}</span>
      {" · "}
      <button
        type="button"
        className="asof-refresh"
        disabled={refreshing}
        onClick={() => {
          if (refreshing) {
            return;
          }
          setRefreshing(true);
          bumpFetchGeneration();
        }}
        aria-label="Refresh now"
      >
        ↻
      </button>
    </span>
  );
}

function relativeSince(at: Date, now: Date): string {
  const seconds = Math.max(0, Math.floor((now.getTime() - at.getTime()) / 1000));
  if (seconds < 60) {
    return "synced just now";
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `synced ${minutes}m ago`;
  }
  const hours = Math.floor(minutes / 60);
  return `synced ${hours}h ago`;
}
