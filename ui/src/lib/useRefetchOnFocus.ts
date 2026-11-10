// The console has no polling, so a tab left open goes stale. This rereads
// when the reader comes back to it: the document becoming visible or the
// window regaining focus bumps the fetch generation (api.ts), the same
// trigger as the topbar's refresh, and every mounted read revalidates in the
// background without blanking its page (hooks.ts's useFetch).
//
// Three guards keep it quiet. A page read within the minimum age is fresh
// enough, and so is one this hook just reread, which is also what makes the
// visibilitychange and focus pair that a tab switch fires count once. A
// hidden document never rereads. And a return while any read is still out
// (the previous reread's, or a slow one of the page's own) is skipped rather
// than stacking a second reread on it; nothing is queued, so the next return
// rereads. The bump is a plain read for every view:
// History's visit horizon moves only on the Mark caught up press.

import { useEffect } from "react";
import { bumpFetchGeneration, getFetchesInFlight, getLastFetchAt } from "../api";

export function useRefetchOnFocus(minAgeMs = 30_000): void {
  useEffect(() => {
    let lastReread = Number.NEGATIVE_INFINITY;
    const reread = () => {
      if (document.visibilityState !== "visible" || getFetchesInFlight() > 0) {
        return;
      }
      const now = Date.now();
      const lastRead = getLastFetchAt()?.getTime() ?? Number.NEGATIVE_INFINITY;
      if (now - Math.max(lastRead, lastReread) < minAgeMs) {
        return;
      }
      lastReread = now;
      bumpFetchGeneration();
    };
    document.addEventListener("visibilitychange", reread);
    window.addEventListener("focus", reread);
    return () => {
      document.removeEventListener("visibilitychange", reread);
      window.removeEventListener("focus", reread);
    };
  }, [minAgeMs]);
}
