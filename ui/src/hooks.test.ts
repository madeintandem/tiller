// Deferred reads defend the state consumers see before effects and while
// navigator refreshes compete; default readers must never inherit old data.

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { bumpFetchGeneration } from "./api";
import { useFetch, type FetchState } from "./hooks";

afterEach(() => {
  // Unmount every renderHook from this test: without it, an earlier test's
  // hook stays subscribed to api.ts's module-level generation counter and
  // refetches (against whatever fetch mock the NEXT test just stubbed) the
  // moment that test calls bumpFetchGeneration, which is exactly the kind of
  // cross-test leakage this hook's own subscription model makes possible.
  cleanup();
  vi.unstubAllGlobals();
});

describe("useFetch: manual refresh via bumpFetchGeneration", () => {
  it("clears default readers immediately until the manual refresh completes", async () => {
    const first = Promise.withResolvers<Response>();
    const refreshed = Promise.withResolvers<Response>();
    const fetchMock = vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(refreshed.promise);
    vi.stubGlobal("fetch", fetchMock);
    const renders: FetchState<{ n: number }>[] = [];
    const { result } = renderHook(() => {
      const state = useFetch<{ n: number }>("/api/status");
      renders.push(state);
      return state;
    });

    await act(async () => first.resolve(Response.json({ n: 1 })));
    expect(result.current.data).toEqual({ n: 1 });
    renders.length = 0;

    act(() => bumpFetchGeneration());
    expect(renders[0]).toMatchObject({ data: null, error: null, loading: true });
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });

    await act(async () => refreshed.resolve(Response.json({ n: 2 })));
    expect(result.current).toMatchObject({ data: { n: 2 }, error: null, loading: false });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/status", expect.anything());
  });

  it("refetches every hook mounted at the moment of the bump, not just one", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ n: 1 }) });
    vi.stubGlobal("fetch", fetchMock);

    const a = renderHook(() => useFetch<{ n: number }>("/api/status"));
    const b = renderHook(() => useFetch<{ n: number }>("/api/queue"));

    await waitFor(() => expect(a.result.current.data).not.toBeNull());
    await waitFor(() => expect(b.result.current.data).not.toBeNull());
    expect(fetchMock).toHaveBeenCalledTimes(2);

    act(() => {
      bumpFetchGeneration();
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(4));
    const urls = fetchMock.mock.calls.map((c) => String(c[0]));
    expect(urls.filter((u) => u === "/api/status")).toHaveLength(2);
    expect(urls.filter((u) => u === "/api/queue")).toHaveLength(2);
  });

  it("does nothing to a null path on a bump: nothing was fetched, nothing refetches", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    renderHook(() => useFetch(null));
    act(() => {
      bumpFetchGeneration();
    });

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("useFetch: same-resource retention", () => {
  it("retains successful rows through competing refresh keys and accepts an empty success", async () => {
    const first = Promise.withResolvers<Response>();
    const obsolete = Promise.withResolvers<Response>();
    const latest = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(obsolete.promise)
      .mockReturnValueOnce(latest.promise));
    const renders: FetchState<string[]>[] = [];
    const { result, rerender } = renderHook(({ refreshKey }) => {
      const state = useFetch<string[]>("/api/tree", { refreshKey, retainData: true });
      renders.push(state);
      return state;
    }, { initialProps: { refreshKey: "/explore" } });
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });

    await act(async () => first.resolve(Response.json(["task"])));
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: false });
    renders.length = 0;
    rerender({ refreshKey: "/node/task/A" });
    expect(renders[0]).toMatchObject({ data: ["task"], error: null, loading: true });
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: true });
    rerender({ refreshKey: "/node/task/B" });
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: true });

    await act(async () => latest.resolve(Response.json([])));
    expect(result.current).toMatchObject({ data: [], error: null, loading: false });
    await act(async () => obsolete.resolve(Response.json(["obsolete"])));
    expect(result.current).toMatchObject({ data: [], error: null, loading: false });
  });

  it("retains data and a failed-refresh warning through manual retry until recovery", async () => {
    const first = Promise.withResolvers<Response>();
    const failure = Promise.withResolvers<Response>();
    const retry = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(failure.promise)
      .mockReturnValueOnce(retry.promise));
    const renders: FetchState<string[]>[] = [];
    const { result } = renderHook(() => {
      const state = useFetch<string[]>("/api/tree", { retainData: true });
      renders.push(state);
      return state;
    });
    await act(async () => first.resolve(Response.json(["task"])));
    renders.length = 0;
    act(() => bumpFetchGeneration());
    expect(renders[0]).toMatchObject({ data: ["task"], error: null, loading: true });
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: true });

    await act(async () => failure.reject(new Error("offline")));
    expect(result.current).toMatchObject({ data: ["task"], error: "offline", loading: false });
    act(() => bumpFetchGeneration());
    expect(result.current).toMatchObject({ data: ["task"], error: "offline", loading: true });

    await act(async () => retry.resolve(Response.json(["fresh task"])));
    expect(result.current).toMatchObject({ data: ["fresh task"], error: null, loading: false });
  });

  it("clears data and errors on the first render for a different path or null", async () => {
    const first = Promise.withResolvers<Response>();
    const failedRefresh = Promise.withResolvers<Response>();
    const different = Promise.withResolvers<Response>();
    const returnVisit = Promise.withResolvers<Response>();
    const fetchMock = vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(failedRefresh.promise)
      .mockReturnValueOnce(different.promise)
      .mockReturnValueOnce(returnVisit.promise);
    vi.stubGlobal("fetch", fetchMock);
    const renders: FetchState<string[]>[] = [];
    const { result, rerender } = renderHook(({ path }: { path: string | null }) => {
      const state = useFetch<string[]>(path, { retainData: true });
      renders.push(state);
      return state;
    }, { initialProps: { path: "/api/tree" as string | null } });
    await act(async () => first.resolve(Response.json(["old tree"])));
    act(() => bumpFetchGeneration());
    await act(async () => failedRefresh.reject(new Error("offline")));
    expect(result.current).toMatchObject({ data: ["old tree"], error: "offline" });

    renders.length = 0;
    rerender({ path: "/api/other-tree" });
    expect(renders[0]).toMatchObject({ data: null, error: null, loading: true });
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });
    await act(async () => different.resolve(Response.json(["other tree"])));
    expect(result.current.data).toEqual(["other tree"]);

    renders.length = 0;
    rerender({ path: null });
    expect(renders[0]).toMatchObject({ data: null, error: null, loading: false });
    expect(result.current).toMatchObject({ data: null, error: null, loading: false });
    expect(fetchMock).toHaveBeenCalledTimes(3);

    rerender({ path: "/api/tree" });
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });
    await act(async () => returnVisit.resolve(Response.json(["new tree"])));
    expect(result.current.data).toEqual(["new tree"]);
  });

  it("clears default readers on a refresh key change and reports a failure without old data", async () => {
    const first = Promise.withResolvers<Response>();
    const refreshed = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(refreshed.promise));
    const renders: FetchState<string[]>[] = [];
    const { result, rerender } = renderHook(({ refreshKey }) => {
      const state = useFetch<string[]>("/api/tree", { refreshKey });
      renders.push(state);
      return state;
    }, { initialProps: { refreshKey: "A" } });
    await act(async () => first.resolve(Response.json(["old tree"])));
    renders.length = 0;
    rerender({ refreshKey: "B" });
    expect(renders[0]).toMatchObject({ data: null, error: null, loading: true });
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });
    await act(async () => refreshed.reject(new Error("offline")));
    expect(result.current).toMatchObject({ data: null, error: "offline", loading: false });
  });

  it("ignores superseded failures during pending and successful refreshes", async () => {
    const first = Promise.withResolvers<Response>();
    const obsoleteA = Promise.withResolvers<Response>();
    const obsoleteB = Promise.withResolvers<Response>();
    const latest = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(obsoleteA.promise)
      .mockReturnValueOnce(obsoleteB.promise)
      .mockReturnValueOnce(latest.promise));
    const { result, rerender } = renderHook(
      ({ refreshKey }) => useFetch<string[]>("/api/tree", { refreshKey, retainData: true }),
      { initialProps: { refreshKey: "initial" } },
    );
    await act(async () => first.resolve(Response.json(["task"])));
    rerender({ refreshKey: "A" });
    rerender({ refreshKey: "B" });
    rerender({ refreshKey: "C" });

    await act(async () => obsoleteA.reject(new Error("obsolete A")));
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: true });
    await act(async () => latest.resolve(Response.json(["fresh task"])));
    expect(result.current).toMatchObject({ data: ["fresh task"], error: null, loading: false });
    await act(async () => obsoleteB.reject(new Error("obsolete B")));
    expect(result.current).toMatchObject({ data: ["fresh task"], error: null, loading: false });
  });

  it("does not replace a current failure with a superseded successful response", async () => {
    const first = Promise.withResolvers<Response>();
    const obsolete = Promise.withResolvers<Response>();
    const latest = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(obsolete.promise)
      .mockReturnValueOnce(latest.promise));
    const { result, rerender } = renderHook(
      ({ refreshKey }) => useFetch<string[]>("/api/tree", { refreshKey, retainData: true }),
      { initialProps: { refreshKey: "initial" } },
    );
    await act(async () => first.resolve(Response.json(["task"])));
    rerender({ refreshKey: "A" });
    rerender({ refreshKey: "B" });
    await act(async () => latest.reject(new Error("current failure")));
    expect(result.current).toMatchObject({ data: ["task"], error: "current failure", loading: false });

    await act(async () => obsolete.resolve(Response.json(["obsolete"])));
    expect(result.current).toMatchObject({ data: ["task"], error: "current failure", loading: false });
  });

  it("reports an initial failure without retained data and recovers on navigation", async () => {
    const first = Promise.withResolvers<Response>();
    const recovery = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(recovery.promise));
    const { result, rerender } = renderHook(
      ({ refreshKey }) => useFetch<string[]>("/api/tree", { refreshKey, retainData: true }),
      { initialProps: { refreshKey: "A" } },
    );
    await act(async () => first.reject(new Error("unavailable")));
    expect(result.current).toMatchObject({ data: null, error: "unavailable", loading: false });

    rerender({ refreshKey: "B" });
    expect(result.current.data).toBeNull();
    expect(result.current.loading).toBe(true);
    await act(async () => recovery.resolve(Response.json(["task"])));
    expect(result.current).toMatchObject({ data: ["task"], error: null, loading: false });
  });

  it("ignores pending responses after changing resource or disabling the read", async () => {
    const first = Promise.withResolvers<Response>();
    const different = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(different.promise));
    const { result, rerender } = renderHook(
      ({ path }: { path: string | null }) => useFetch<string[]>(path, { retainData: true }),
      { initialProps: { path: "/api/tree" as string | null } },
    );
    rerender({ path: "/api/other-tree" });
    await act(async () => first.resolve(Response.json(["obsolete tree"])));
    expect(result.current).toMatchObject({ data: null, error: null, loading: true });

    rerender({ path: null });
    await act(async () => different.reject(new Error("obsolete failure")));
    expect(result.current).toMatchObject({ data: null, error: null, loading: false });
  });
});

// A browser keeps only six connections per console origin. A read the reader
// navigated away from must give its connection back instead of holding it
// until the server answers, or a few quick node picks queue the newest page
// behind every obsolete one.
describe("useFetch: superseded requests are aborted", () => {
  const pending = () => new Promise<Response>(() => {});
  const signalOf = (fetchMock: ReturnType<typeof vi.fn>, call: number): AbortSignal => {
    const init = fetchMock.mock.calls[call]?.[1] as RequestInit | undefined;
    expect(init?.signal).toBeInstanceOf(AbortSignal);
    return init!.signal!;
  };

  it("aborts the previous request when the path changes", () => {
    const fetchMock = vi.fn(pending);
    vi.stubGlobal("fetch", fetchMock);
    const { rerender } = renderHook(({ path }: { path: string }) => useFetch(path), {
      initialProps: { path: "/api/node/feature/F-aaaaaa" },
    });
    rerender({ path: "/api/node/feature/F-bbbbbb" });

    expect(signalOf(fetchMock, 0).aborted).toBe(true);
    expect(signalOf(fetchMock, 1).aborted).toBe(false);
  });

  it("aborts a retained refresh that a newer refresh replaces", () => {
    const fetchMock = vi.fn(pending);
    vi.stubGlobal("fetch", fetchMock);
    const { rerender } = renderHook(
      ({ refreshKey }: { refreshKey: string }) => useFetch("/api/tree", { refreshKey, retainData: true }),
      { initialProps: { refreshKey: "/node/feature/F-aaaaaa" } },
    );
    rerender({ refreshKey: "/node/feature/F-bbbbbb" });

    expect(signalOf(fetchMock, 0).aborted).toBe(true);
    expect(signalOf(fetchMock, 1).aborted).toBe(false);
  });

  it("aborts the pending request on unmount", () => {
    const fetchMock = vi.fn(pending);
    vi.stubGlobal("fetch", fetchMock);
    const { unmount } = renderHook(() => useFetch("/api/queue"));
    unmount();

    expect(signalOf(fetchMock, 0).aborted).toBe(true);
  });

  it("does not report the abort as an error", async () => {
    const current = Promise.withResolvers<Response>();
    const fetchMock = vi.fn()
      .mockImplementationOnce((_path: string, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        }))
      .mockReturnValueOnce(current.promise);
    vi.stubGlobal("fetch", fetchMock);
    const { result, rerender } = renderHook(({ path }: { path: string }) => useFetch<string[]>(path), {
      initialProps: { path: "/api/ls?noun=bug" },
    });
    rerender({ path: "/api/ls?noun=chore" });
    await act(async () => current.resolve(Response.json(["chore"])));

    expect(result.current).toMatchObject({ data: ["chore"], error: null, loading: false });
  });
});
