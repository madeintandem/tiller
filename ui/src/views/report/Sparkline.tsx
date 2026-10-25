// A 64x20 trend line for a KPI tile (D12): the weekly series in a muted ink,
// its latest value marked with a dot. The tile's own number carries the
// value; the sparkline only shows the shape.

import { linearScale } from "./scale";

const WIDTH = 64;
const HEIGHT = 20;
// Inset so the 2px line and the end dot are not clipped at the edges.
const PAD = 3;

export default function Sparkline({ values, label }: { values: number[]; label: string }) {
  const max = Math.max(0, ...values);
  const x = linearScale([0, Math.max(values.length - 1, 1)], [PAD, WIDTH - PAD]);
  const y = linearScale([0, max], [HEIGHT - PAD, PAD]);
  const points = values.map((v, i) => [x(i), y(v)] as const);
  const end = points[points.length - 1];
  return (
    <svg
      className="report-sparkline"
      role="img"
      aria-label={label}
      width={WIDTH}
      height={HEIGHT}
      viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
    >
      <polyline points={points.map(([px, py]) => `${px},${py}`).join(" ")} />
      {end && <circle cx={end[0]} cy={end[1]} r="2.5" />}
    </svg>
  );
}
