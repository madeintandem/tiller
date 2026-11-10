import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useBoardScrollMode } from "./boardScroll";

afterEach(() => {
  window.localStorage.clear();
});

describe("useBoardScrollMode", () => {
  it("defaults to the whole board", () => {
    const { result } = renderHook(() => useBoardScrollMode());
    expect(result.current[0]).toBe("board");
  });

  it("round-trips both values through tiller-board-scroll", () => {
    const { result } = renderHook(() => useBoardScrollMode());
    act(() => result.current[1]("columns"));
    expect(window.localStorage.getItem("tiller-board-scroll")).toBe("columns");
    expect(renderHook(() => useBoardScrollMode()).result.current[0]).toBe("columns");
    act(() => result.current[1]("board"));
    expect(window.localStorage.getItem("tiller-board-scroll")).toBe("board");
    expect(renderHook(() => useBoardScrollMode()).result.current[0]).toBe("board");
  });

  it("reads a value stored before the labels were renamed", () => {
    window.localStorage.setItem("tiller-board-scroll", "columns");
    expect(renderHook(() => useBoardScrollMode()).result.current[0]).toBe("columns");
    window.localStorage.setItem("tiller-board-scroll", "board");
    expect(renderHook(() => useBoardScrollMode()).result.current[0]).toBe("board");
  });
});
