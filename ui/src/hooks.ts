// The one fetch effect every view shares, extracted from the P1 views'
// hand-rolled useEffect pattern: fetch a path, land the data or the error,
// and never set state after unmount or after the path has moved on. Views
// refetch by changing the path string (a route param, a filter, a search
// query); a null path means "nothing to fetch yet", the idle state a search
// box sits in before its first query.
//
// A second, independent way to refetch: the topbar's manual ↻. api.ts's
// fetchGeneration is a plain module counter the button bumps; this hook
// subscribes to it and folds the current value into the SAME fetch effect's
// dependency array as path, so a bump re-runs the identical effect a path
// change already runs, for every mounted useFetch at once - one refetch
// mechanism, not two.
//
// That includes History's visit window: a bump re-requests window=visit, which
// is a plain read. Only the Mark caught up press carries the visit marker, so a
// bump never moves the horizon.
//
// A bump is a background revalidation for EVERY reader: the hook keeps the
// last answer for the same path and refresh key while the reread runs, and a
// failed reread keeps it beside the error, so no page blanks into a loading
// state because the reader pressed refresh or came back to the window. A path
// change still drops the old data at once (a different node must never show
// the previous node's page), and so does a refresh key change unless the
// caller asks for retainData, which is the one thing that option now means.

import { useEffect, useState } from "react";
import { fetchJSON, getFetchGeneration, subscribeFetchGeneration } from "./api";

export interface FetchState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
}

interface KeyedFetchState<T> extends FetchState<T> {
  path: string | null;
  refreshKey: string | undefined;
  gen: number;
}

// retainData carries the last answer across a refresh key change too, for a
// persistent layout (Explore's navigator) whose key follows the route but
// whose resource does not. Without it a key change clears, the way a path
// change always does.
export function useFetch<T>(
  path: string | null,
  {
    refreshKey,
    retainData = false,
    headers,
  }: { refreshKey?: string; retainData?: boolean; headers?: HeadersInit } = {},
): FetchState<T> {
  // Local state mirroring the module-level generation counter: bumping it
  // does nothing on its own (a plain variable does not trigger a re-render),
  // so this subscription is what turns a bump into "this hook's own effect
  // dependency changed, run it again."
  const [gen, setGen] = useState(() => getFetchGeneration());
  useEffect(() => subscribeFetchGeneration(() => setGen(getFetchGeneration())), []);

  // The state is keyed by request path. React renders once with the new props
  // before this effect runs, so returning the previous state's data would show
  // an old node/list/recap under a new URL for one commit.
  const [state, setState] = useState<KeyedFetchState<T>>({
    path,
    refreshKey,
    gen,
    data: null,
    error: null,
    loading: path !== null,
  });

  useEffect(() => {
    if (path === null) {
      setState({ path, refreshKey, gen, data: null, error: null, loading: false });
      return;
    }
    // Retention belongs to this resource only. A rerun for the same path and
    // key is a reread (a generation bump) and keeps what is on screen; a key
    // change keeps it only for a retainData caller. Carrying data through
    // pending/error states preserves the last success even when several
    // refreshes supersede one another before settling.
    // A superseded or unmounted read also aborts its request: the browser
    // keeps only six connections per console origin, and an obsolete read
    // left running holds one until the server answers, so a few quick node
    // picks would queue the newest page behind every abandoned one.
    let cancelled = false;
    const controller = new AbortController();
    setState((previous) => {
      const keep = previous.path === path && (retainData || previous.refreshKey === refreshKey);
      return {
        path,
        refreshKey,
        gen,
        data: keep ? previous.data : null,
        error: keep ? previous.error : null,
        loading: true,
      };
    });
    fetchJSON<T>(path, { headers, signal: controller.signal })
      .then((data) => {
        if (!cancelled) setState({ path, refreshKey, gen, data, error: null, loading: false });
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          // The pending state above already decided what this read keeps.
          setState((previous) => ({
            path,
            refreshKey,
            gen,
            data: previous.path === path ? previous.data : null,
            error: err instanceof Error ? err.message : String(err),
            loading: false,
          }));
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [path, gen, refreshKey, retainData, headers]);

  if (path === null || state.path !== path) {
    return { data: null, error: null, loading: path !== null };
  }
  if (state.refreshKey !== refreshKey && !retainData) {
    return { data: null, error: null, loading: true };
  }
  if (state.refreshKey !== refreshKey || state.gen !== gen) {
    return { data: state.data, error: state.error, loading: true };
  }

  return state;
}

// The product suffix every tab title carries. The page part leads because
// browser tabs truncate from the right: "Wire the billing webhook · Tiller"
// still reads as the feature when only its first few words are visible, and
// a row of tabs that all began "Tiller · " would be indistinguishable, which
// is exactly the original complaint.
const TITLE_SUFFIX = " · Tiller";

// titleOf builds the tab title for one page part.
export function titleOf(page: string): string {
  return `${page}${TITLE_SUFFIX}`;
}

// useDocumentTitle sets the browser tab title. Each view calls it with its
// own title so a tab names what it shows: the project on Home, the page
// name on a board, the node's handle on a node page, the file on a file
// page. null means "nothing to say yet" and leaves the current title alone
// rather than flashing a placeholder while a fetch is in flight. Nothing is
// restored on unmount: the next view sets its own title on its first commit,
// and index.html's static <title> only ever covers the moment before React
// mounts.
export function useDocumentTitle(title: string | null): void {
  useEffect(() => {
    if (title !== null) {
      document.title = title;
    }
  }, [title]);
}
