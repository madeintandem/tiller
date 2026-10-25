// The one hover readout every Report chart shares (D14): a div positioned
// over the chart's plot, fed by onPointerMove and onPointerLeave on each
// chart's hit areas. A tooltip only repeats what the chart already shows; it
// never gates a value.

import { useCallback, useState, type PointerEvent } from "react";

export interface TipRow {
  label: string;
  value: string;
}

// x and y are pixels inside the plot. flip puts the box left of the pointer
// when the pointer is in the plot's right half, and raise puts it above the
// pointer in the plot's lower half, so it never runs off the card or out of
// the enlarged chart's dialog.
export interface Tip {
  x: number;
  y: number;
  rows: TipRow[];
  flip?: boolean;
  raise?: boolean;
}

export function useTooltip() {
  const [tip, setTip] = useState<Tip | null>(null);
  const show = useCallback((e: PointerEvent<Element>, rows: TipRow[]) => {
    const box = e.currentTarget.closest(".report-plot")?.getBoundingClientRect();
    const x = e.clientX - (box?.left ?? 0);
    const y = e.clientY - (box?.top ?? 0);
    setTip({ x, y, rows, flip: box ? x > box.width / 2 : false, raise: box ? y > box.height / 2 : false });
  }, []);
  const hide = useCallback(() => setTip(null), []);
  return { tip, show, hide };
}

export default function ChartTooltip({ tip }: { tip: Tip | null }) {
  if (!tip) return null;
  return (
    <div
      className={["report-tooltip", tip.flip && "is-flipped", tip.raise && "is-raised"].filter(Boolean).join(" ")}
      role="tooltip"
      style={{ left: tip.x, top: tip.y }}
    >
      {tip.rows.map((row, i) => (
        <div key={i} className="report-tooltip-row">
          <span>{row.label}</span>
          {row.value !== "" && (
            <>
              {" "}
              <strong>{row.value}</strong>
            </>
          )}
        </div>
      ))}
    </div>
  );
}
