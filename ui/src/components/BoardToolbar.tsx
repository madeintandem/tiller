import type { ReactNode } from "react";
import type { BoardScrollMode } from "../lib/boardScroll";
import "./board.css";

export default function BoardToolbar({
  panelLabel,
  panelId,
  panelOpen,
  onTogglePanel,
  scrollMode,
  onScrollMode,
  summary,
  children,
}: {
  panelLabel: string;
  panelId: string;
  panelOpen: boolean;
  onTogglePanel: () => void;
  scrollMode: BoardScrollMode;
  onScrollMode: (mode: BoardScrollMode) => void;
  summary: string;
  children?: ReactNode;
}) {
  return (
    <div className="board-bar">
      <button type="button" className="panel-toggle" onClick={onTogglePanel} aria-expanded={panelOpen} aria-controls={panelId}>
        {panelOpen ? "⟨" : "⟩"} {panelLabel}
      </button>
      <span className="showing">{summary}</span>
      {children}
      <div className="board-scroll-control" role="group" aria-label="Scroll">
        <span>Scroll</span>
        <div className="segmented-control">
          {(["board", "columns"] as const).map((mode) => (
            <button key={mode} type="button" className={scrollMode === mode ? "active" : undefined} aria-pressed={scrollMode === mode} onClick={() => onScrollMode(mode)}>
              {mode === "board" ? "Board" : "Columns"}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
