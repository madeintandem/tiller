// A page whose reads have constant paths (Home, Build, Fix, Report, Explore)
// reads on mount, and then only when the reader refreshes or comes back to
// the window. But the reader can arrive at the page again while it is still
// mounted: the topbar link for the page already showing, or the browser's
// back and forward between two addresses of the same page. Nothing remounts,
// no path changes, and without this the page would keep whatever it read
// first.
//
// useArrivalKey counts those arrivals. The page passes the count to useFetch
// as its refresh key with retainData, so an arrival rereads the page's own
// reads once and the board stays on screen until the answer lands (hooks.ts).
// The count starts at the mount, so a first arrival reads once, as before.
//
// Not every location change is an arrival. The page also rewrites its own
// address: a filter, a tab, an epic toggle, the saved-selection restore. Those
// navigate with IN_PAGE_CHANGE as their router state and read nothing. The
// mark stays on the history entry, and going back or forward to that entry is
// an arrival all the same, so a history move always counts.
//
// The key is a count, not the location key: back from a filter change lands
// on the entry the last arrival already named.
//
// A component that outlives its page (the shell, Explore's layout) names the
// one pathname whose arrivals it counts, so moving around the other pages it
// stays mounted for does not reread on that page's behalf.

import { useState } from "react";
import { useLocation, useNavigationType } from "react-router-dom";

// Router state for a URL change a page makes to its own address.
export const IN_PAGE_CHANGE = { inPage: true } as const;

export function useArrivalKey(at?: string): string {
  const location = useLocation();
  const navigationType = useNavigationType();
  const [seen, setSeen] = useState({ locationKey: location.key, arrivals: 0 });
  if (seen.locationKey === location.key) {
    return String(seen.arrivals);
  }
  const inPage =
    navigationType !== "POP" && (location.state as { inPage?: boolean } | null)?.inPage === true;
  const counted = !inPage && (at === undefined || location.pathname === at);
  const arrivals = counted ? seen.arrivals + 1 : seen.arrivals;
  setSeen({ locationKey: location.key, arrivals });
  return String(arrivals);
}
