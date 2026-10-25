import { useState } from "react";

export type BoardScrollMode = "board" | "columns";
const STORAGE_KEY = "tiller-board-scroll";

export function useBoardScrollMode() {
  const [scrollMode, setScrollMode] = useState<BoardScrollMode>(() => {
    try {
      return window.localStorage.getItem(STORAGE_KEY) === "columns" ? "columns" : "board";
    } catch {
      return "board";
    }
  });

  function changeScrollMode(mode: BoardScrollMode) {
    setScrollMode(mode);
    try {
      window.localStorage.setItem(STORAGE_KEY, mode);
    } catch {
      // Scrolling still switches when browser storage is unavailable.
    }
  }

  return [scrollMode, changeScrollMode] as const;
}
