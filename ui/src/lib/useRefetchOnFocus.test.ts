// The focus reread: coming back to the console tab rereads every mounted
// page in the background, but only when what is on screen is at least the
// minimum age old, only once per return (visibilitychange and focus usually
// fire together), never for a hidden document, and never while the previous
// reread's requests are still out.

import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { fetchJSON, subscribeFetchGeneration } from "../api";
import { useRefetchOnFocus } from "./useRefetchOnFocus";

let visibility: DocumentVisibilityState = "visible";
let bumps = 0;
let unsubscribe: () => void = () => {};

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-10T12:00:00Z"));
  visibility = "visible";
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => visibility });
  bumps = 0;
  unsubscribe = subscribeFetchGeneration(() => {
    bumps += 1;
  });
});

afterEach(() => {
  cleanup();
  unsubscribe();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "visibilityState");
});

// A successful read stamps the last-fetch instant at the faked "now".
async function readSucceeded() {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(Response.json({ ok: true }))));
  await fetchJSON("/api/status");
}

function returnToWindow() {
  document.dispatchEvent(new Event("visibilitychange"));
  window.dispatchEvent(new Event("focus"));
}

describe("useRefetchOnFocus", () => {
  it("bumps the fetch generation once on focus when the last read is older than the minimum age", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(31_000);

    window.dispatchEvent(new Event("focus"));

    expect(bumps).toBe(1);
  });

  it("bumps on the document becoming visible, too", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(31_000);

    document.dispatchEvent(new Event("visibilitychange"));

    expect(bumps).toBe(1);
  });

  it("does not bump when the last read is younger than the minimum age", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(5_000);

    returnToWindow();

    expect(bumps).toBe(0);
  });

  it("honors a caller's own minimum age", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus(60_000));
    vi.advanceTimersByTime(31_000);
    returnToWindow();
    expect(bumps).toBe(0);

    vi.advanceTimersByTime(30_000);
    returnToWindow();
    expect(bumps).toBe(1);
  });

  it("bumps once when visibilitychange and focus fire together", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(31_000);

    returnToWindow();
    returnToWindow();

    expect(bumps).toBe(1);
  });

  it("never bumps for a hidden document", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(31_000);
    visibility = "hidden";

    returnToWindow();

    expect(bumps).toBe(0);
  });

  it("never bumps a second time while the previous reread's requests are still in flight", async () => {
    await readSucceeded();
    renderHook(() => useRefetchOnFocus());
    vi.advanceTimersByTime(31_000);
    returnToWindow();
    expect(bumps).toBe(1);

    // The reread's request is out and the server has not answered.
    const held = Promise.withResolvers<Response>();
    vi.stubGlobal("fetch", vi.fn(() => held.promise));
    const reading = fetchJSON("/api/status");
    vi.advanceTimersByTime(61_000);
    returnToWindow();
    expect(bumps).toBe(1);

    held.resolve(Response.json({ ok: true }));
    await reading;
    vi.advanceTimersByTime(31_000);
    returnToWindow();
    expect(bumps).toBe(2);
  });

  it("stops listening once unmounted", async () => {
    await readSucceeded();
    const { unmount } = renderHook(() => useRefetchOnFocus());
    unmount();
    vi.advanceTimersByTime(31_000);

    returnToWindow();

    expect(bumps).toBe(0);
  });
});
