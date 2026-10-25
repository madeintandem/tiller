// AsOf's own subscription is exercised through spies on api.ts's exported
// getLastFetchAt/subscribeLastFetch rather than the real global fetch: those
// two functions are module-level singletons shared by the whole app (every
// fetchJSON call in the process updates them), so driving the component
// through the real thing would make this file's outcome depend on execution
// order against every other file's own fetchJSON calls in the same worker.
// Spying the two functions this component actually reads keeps the test
// deterministic and still proves the real contract: AsOf renders nothing
// until getLastFetchAt/the subscription hands it a Date, and re-renders when
// the subscription fires again.

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../api";
import { useFetch } from "../../hooks";
import AsOf from "../AsOf";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("AsOf", () => {
  it("renders nothing before any successful read has landed", () => {
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(null);
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});

    const { container } = render(<AsOf />);

    expect(container.querySelector(".asof")).toBeNull();
  });

  it("renders 'synced just now' immediately after a read lands, under a minute old", () => {
    const now = new Date("2026-08-10T12:00:00Z");
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(now);
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});
    vi.useFakeTimers();
    vi.setSystemTime(now);

    render(<AsOf />);

    expect(screen.getByText("synced just now")).toBeInTheDocument();
  });

  it("states whole minutes once a minute has passed, ticking on its own interval", () => {
    const at = new Date("2026-08-10T12:00:00Z");
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(at);
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});
    vi.useFakeTimers();
    vi.setSystemTime(at);

    render(<AsOf />);
    expect(screen.getByText("synced just now")).toBeInTheDocument();

    // Five minutes on, with no new fetch: the component's own 30s tick is
    // what has to move the number, not a fresh subscription push.
    // advanceTimersByTime both moves vitest's faked Date and fires the
    // interval as it goes, so setSystemTime is not needed here too - calling
    // both would advance the clock twice.
    act(() => {
      vi.advanceTimersByTime(5 * 60_000);
    });

    expect(screen.getByText("synced 5m ago")).toBeInTheDocument();
  });

  it("moves back to 'just now' when the subscription hands it a fresher read", () => {
    const at = new Date("2026-08-10T12:00:00Z");
    let push: ((at: Date) => void) | undefined;
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(at);
    vi.spyOn(api, "subscribeLastFetch").mockImplementation((listener) => {
      push = listener;
      return () => {};
    });
    vi.useFakeTimers();
    vi.setSystemTime(new Date(at.getTime() + 10 * 60_000));

    render(<AsOf />);
    expect(screen.getByText("synced 10m ago")).toBeInTheDocument();

    const fresher = new Date(at.getTime() + 10 * 60_000);
    act(() => {
      push?.(fresher);
    });

    expect(screen.getByText("synced just now")).toBeInTheDocument();
  });

  it("states whole hours once sixty minutes have passed", () => {
    const at = new Date("2026-08-10T12:00:00Z");
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(at);
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});
    vi.useFakeTimers();
    vi.setSystemTime(new Date(at.getTime() + 125 * 60_000));

    render(<AsOf />);

    expect(screen.getByText("synced 2h ago")).toBeInTheDocument();
  });

  // Round-1 review, Finding 4: the console has no polling, so the refresh
  // control is the asof line's only remedy for staleness. It calls
  // bumpFetchGeneration - hooks.test.ts proves that call actually refetches
  // mounted data; this only proves the button is wired to it.
  it("renders a refresh control that calls bumpFetchGeneration when clicked", () => {
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(new Date("2026-08-10T12:00:00Z"));
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});
    const bump = vi.spyOn(api, "bumpFetchGeneration").mockImplementation(() => {});

    render(<AsOf />);
    fireEvent.click(screen.getByRole("button", { name: /refresh/i }));

    expect(bump).toHaveBeenCalledTimes(1);
  });

  // task-22-brief.md item 3: the refresh button had no guard, one bump per
  // click with nothing stopping a rapid double-click from firing two. The
  // button disables itself the instant the first click bumps the
  // generation, so a second click landing before any read has completed
  // finds a disabled control and never reaches the handler at all.
  it("a rapid double-click issues one generation bump, the button disabled meanwhile", () => {
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(new Date("2026-08-10T12:00:00Z"));
    vi.spyOn(api, "subscribeLastFetch").mockReturnValue(() => {});
    const bump = vi.spyOn(api, "bumpFetchGeneration").mockImplementation(() => {});

    render(<AsOf />);
    const button = screen.getByRole("button", { name: /refresh/i });
    fireEvent.click(button);
    expect(button).toBeDisabled();
    fireEvent.click(button);

    expect(bump).toHaveBeenCalledTimes(1);
  });

  // The guard clears on the same "something landed" signal the asof line
  // itself reads (subscribeLastFetch), not a tracker of this click's own
  // requests - so a second, genuinely later click is never permanently
  // locked out by the first.
  it("re-enables once a read lands, letting a later click bump again", () => {
    let push: ((at: Date) => void) | undefined;
    vi.spyOn(api, "getLastFetchAt").mockReturnValue(new Date("2026-08-10T12:00:00Z"));
    vi.spyOn(api, "subscribeLastFetch").mockImplementation((listener) => {
      push = listener;
      return () => {};
    });
    const bump = vi.spyOn(api, "bumpFetchGeneration").mockImplementation(() => {});

    render(<AsOf />);
    const button = screen.getByRole("button", { name: /refresh/i });
    fireEvent.click(button);
    expect(button).toBeDisabled();

    act(() => {
      push?.(new Date("2026-08-10T12:05:00Z"));
    });
    expect(button).not.toBeDisabled();

    fireEvent.click(button);
    expect(bump).toHaveBeenCalledTimes(2);
  });
});

// The end-to-end shape of the refresh affordance, against the REAL
// getLastFetchAt/subscribeLastFetch/bumpFetchGeneration (no spies): a
// mounted useFetch consumer, AsOf reading the real subscribable, a stubbed
// global fetch standing in for the network. This is what "bump triggers a
// refetch on a mounted hook; asof advances after the refetch succeeds" means
// end to end, not just at each layer in isolation.
describe("AsOf + useFetch: the refresh affordance end to end", () => {
  function Consumer() {
    useFetch<{ ok: boolean }>("/api/status");
    return null;
  }

  it("clicking refresh re-fetches a mounted view's data and advances asof back to 'just now'", async () => {
    const start = new Date("2026-08-10T12:00:00Z");
    vi.useFakeTimers();
    vi.setSystemTime(start);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ ok: true }) }));

    render(
      <>
        <AsOf />
        <Consumer />
      </>,
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByText("synced just now")).toBeInTheDocument();

    // Ten minutes on with no new fetch: the line goes stale, same as any
    // other read the reader has not refreshed.
    act(() => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(screen.getByText("synced 10m ago")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /refresh/i }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByText("synced just now")).toBeInTheDocument();
  });
});
