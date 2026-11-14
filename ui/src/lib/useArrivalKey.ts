// A board whose reads have constant paths (Build, Fix) reads on mount, and
// then only when the reader refreshes or comes back to the window. But the
// reader can arrive at the board again while it is still mounted: the topbar
// link for the page already showing, or the browser's back and forward
// between two addresses of the same board. Nothing remounts, no path
// changes, and without this the board would keep whatever it read first.
//
// useArrivalKey counts those arrivals. The page passes the count to useFetch
// as its refresh key with retainData, so an arrival rereads the page's own
// reads once and the board stays on screen until the answer lands (hooks.ts).
// The count starts at the mount, so a first arrival reads once, as before.
//
// Not every location change is an arrival. The page also rewrites its own
// address: a filter, an epic toggle, the saved-selection restore. Those
// navigate with IN_PAGE_CHANGE as their router state and read nothing. The
// mark stays on the history entry, and going back or forward to that entry is
// an arrival all the same, so a history move always counts.
//
// The key is a count, not the location key: back from a filter change lands
// on the entry the last arrival already named.

import { useState } from "react";
import { useLocation, useNavigationType } from "react-router-dom";

// Router state for a URL change a page makes to its own address.
export const IN_PAGE_CHANGE = { inPage: true } as const;

export function useArrivalKey(): string {
  const location = useLocation();
  const navigationType = useNavigationType();
  const [seen, setSeen] = useState({ locationKey: location.key, arrivals: 0 });
  if (seen.locationKey === location.key) {
    return String(seen.arrivals);
  }
  const inPage =
    navigationType !== "POP" && (location.state as { inPage?: boolean } | null)?.inPage === true;
  const arrivals = inPage ? seen.arrivals : seen.arrivals + 1;
  setSeen({ locationKey: location.key, arrivals });
  return String(arrivals);
}
