// Pure scales for the Report page's hand-written SVG charts (D14). No chart
// library: these four functions are the whole of the positioning math.

// linearScale maps a numeric domain onto a pixel range. A zero-width domain
// (an all-zero series) maps every value to the range's start rather than
// dividing by zero.
export function linearScale(domain: [number, number], range: [number, number]): (v: number) => number {
  const [d0, d1] = domain;
  const [r0, r1] = range;
  if (d1 === d0) return () => r0;
  const k = (r1 - r0) / (d1 - d0);
  return (v) => r0 + (v - d0) * k;
}

// niceTicks returns whole-number gridline values from 0 up to the first step
// at or above max, stepping by 1, 2, or 5 times a power of ten. Every series
// on the page is a count or a day total, so a tick is never fractional.
export function niceTicks(max: number, count = 4): number[] {
  if (!(max > 0) || !Number.isFinite(max)) return [0];
  const raw = max / count;
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const nice = [1, 2, 5, 10].map((m) => m * magnitude).find((s) => s >= raw) ?? raw;
  const step = Math.max(1, Math.round(nice));
  const ticks: number[] = [];
  for (let t = 0; t < max + step; t += step) ticks.push(t);
  return ticks;
}

// weekX gives each week an equal band across width and returns the band
// center for a week index.
export function weekX(weeks: string[], width: number): (index: number) => number {
  const band = width / Math.max(weeks.length, 1);
  return (index) => band * (index + 0.5);
}

// dateX places an instant inside its server-defined week's band, as far
// across the band as it is through that week. Boundaries include the Monday
// after the final week, since even that week may be shorter or longer at DST.
// Instants outside the axis clamp to its edges.
export function dateX(boundaries: string[], width: number): (iso: string) => number {
  const band = width / Math.max(boundaries.length - 1, 1);
  const instants = boundaries.map((boundary) => Date.parse(boundary));
  return (iso) => {
    const t = Date.parse(iso);
    if (Number.isNaN(t) || instants.length < 2) return 0;
    let i = instants.length - 2;
    while (i > 0 && t < instants[i]) i--;
    const through = Math.min(Math.max((t - instants[i]) / (instants[i + 1] - instants[i]), 0), 1);
    return band * (i + through);
  };
}

// localMidnight reads a YYYY-MM-DD date as the start of that day locally.
export function localMidnight(day: string): number {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d).getTime();
}
