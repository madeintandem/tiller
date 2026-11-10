import type { ReactNode } from "react";
import type { BoardScrollMode } from "../lib/boardScroll";
import "./board.css";

// The stored values stay "board" and "columns" (lib/boardScroll.ts); only the
// words on screen are plain.
const SCROLL_CHOICES: { mode: BoardScrollMode; label: string; title: string }[] = [
  { mode: "board", label: "Whole board", title: "The board scrolls as one page" },
  { mode: "columns", label: "Each column", title: "Each column scrolls on its own" },
];

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
      <div className="board-scroll-control" role="group" aria-label="Scrolling">
        <span>Scrolling</span>
        <div className="segmented-control">
          {SCROLL_CHOICES.map(({ mode, label, title }) => (
            <button key={mode} type="button" className={scrollMode === mode ? "active" : undefined} aria-pressed={scrollMode === mode} title={title} onClick={() => onScrollMode(mode)}>
              {label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
