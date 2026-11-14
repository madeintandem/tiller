# Reread Home, Report, and Explore when the reader arrives again

Imported from [GitHub issue 16](https://github.com/madeintandem/tiller/issues/16).

## What

Extend the console's arrival reread from Build and Fix to Home, Report, and Explore. Each page-owned read takes `useArrivalKey()` as its refresh key with `retainData`, and each page's own URL rewrites carry `IN_PAGE_CHANGE` so they do not count as arrivals. Home's `?tab=` changes and Explore's tree state are the rewrites to check. `docs/spec/ui-surface.md`, "Liveness", is updated; it currently says the arrival reread is Build and Fix only.

While there, check whether resubmitting the same search rereads: `TopbarSearch` says it refreshes the answer, but the search read has no refresh key. That claim is unverified.

## Why

Since v0.24.0, clicking the topbar link for the page already showing, or moving back and forward between two addresses of the same board, rereads Build and Fix. On Home, Report, and Explore the same arrival issues no request, so a status changed through the CLI stays stale until the refresh control or a window return. Home's active link was observed issuing no request in a browser on 2026-11-14; Report and Explore are read from the code.

## Done when

- Clicking the active Home, Report, or Explore link while that page is mounted issues exactly one new read per page-owned request, and the page shows state changed through the CLI since the last read.
- Browser back and forward between two addresses of the same page reread once per move.
- A tab change on Home, a tree or filter change on Explore, and a range change on Report issue no arrival reread.
- The guarantees Build and Fix already pin hold on all three pages: one read on mount, no duplicate read on manual or focus refresh, current data stays on screen during the reread, and a failed reread keeps the page with its out-of-date notice.
- `ui/dist` is rebuilt in the same change.
