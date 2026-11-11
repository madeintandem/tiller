// @ts-nocheck
// Copied from the madeintandem.com site (src/lib/flow-field.mjs), which draws the homepage
// hero with it. The one change: the field is a width x height rectangle, not a square,
// so it can fill a 16:9 frame.
// Build-time generator for the homepage hero's flow field. It traces evenly
// spaced streamlines through curl noise bent by a few poles (sources, sinks,
// and vortices), after Jobard and Lefer's "Creating Evenly-Spaced Streamlines
// of Arbitrary Density" (1997). Nothing here ships to the browser: the hero
// renders the traced lines as static SVG paths.

// mulberry32: a small seeded PRNG, so a seed always draws the same field.
export function createRandom(seed) {
  let state = seed >>> 0;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const fade = (t) => t * t * t * (t * (t * 6 - 15) + 10);
const lerp = (a, b, t) => a + (b - a) * t;

// Classic 2D gradient (Perlin) noise. Returns values in about [-0.7, 0.7].
export function createNoise(seed) {
  const random = createRandom(seed);
  const order = Array.from({ length: 256 }, (_, i) => i);
  for (let i = 255; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [order[i], order[j]] = [order[j], order[i]];
  }
  const permutation = [...order, ...order];
  const gradients = order.map((i) => [Math.cos((i / 256) * Math.PI * 2), Math.sin((i / 256) * Math.PI * 2)]);

  const dot = (hash, x, y) => {
    const [gx, gy] = gradients[hash];
    return gx * x + gy * y;
  };

  return (x, y) => {
    const cellX = Math.floor(x);
    const cellY = Math.floor(y);
    const fx = x - cellX;
    const fy = y - cellY;
    const xi = cellX & 255;
    const yi = cellY & 255;
    const corner = (dx, dy) => permutation[permutation[xi + dx] + yi + dy];
    const u = fade(fx);
    const v = fade(fy);
    return lerp(
      lerp(dot(corner(0, 0), fx, fy), dot(corner(1, 0), fx - 1, fy), u),
      lerp(dot(corner(0, 1), fx, fy - 1), dot(corner(1, 1), fx - 1, fy - 1), u),
      v,
    );
  };
}

const poleFlows = {
  source: (dx, dy) => [dx, dy],
  sink: (dx, dy) => [-dx, -dy],
  vortex: (dx, dy) => [-dy, dx],
};

// The flow direction at any point, as a unit vector (or [0, 0] where the
// flow stalls). A pole's pull falls off as 1/r; give it a `reach` and it
// also fades out (Gaussian) past that distance, so it stays local. The noise part is curl noise: divergence-free, so it swirls
// without bunching up, and lines circling a vortex can close into rings.
export function createField({ seed, noiseScale, noiseWeight = 1, poles = [] }) {
  const noise = createNoise(seed);
  const h = 0.5;
  const potential = (x, y) => noise(x * noiseScale, y * noiseScale) / noiseScale;

  return (x, y) => {
    let vx = 0;
    let vy = 0;
    if (noiseWeight && noiseScale) {
      vx += ((potential(x, y + h) - potential(x, y - h)) / (2 * h)) * noiseWeight;
      vy -= ((potential(x + h, y) - potential(x - h, y)) / (2 * h)) * noiseWeight;
    }
    for (const { x: px, y: py, kind, strength, reach } of poles) {
      const dx = x - px;
      const dy = y - py;
      const distanceSquared = dx * dx + dy * dy || 1;
      const falloff = reach ? Math.exp(-distanceSquared / (reach * reach)) : 1;
      const scale = (strength / distanceSquared) * falloff;
      const [fx, fy] = poleFlows[kind](dx, dy);
      vx += fx * scale;
      vy += fy * scale;
    }
    const length = Math.hypot(vx, vy);
    return length < 1e-9 ? [0, 0] : [vx / length, vy / length];
  };
}

// Points already drawn, bucketed by separation-sized cells so proximity
// checks only scan the 3x3 cells around a point.
function createGrid(cellSize) {
  const cells = new Map();
  const key = (cx, cy) => `${cx},${cy}`;
  return {
    add(point) {
      const k = key(Math.floor(point.x / cellSize), Math.floor(point.y / cellSize));
      if (!cells.has(k)) cells.set(k, []);
      cells.get(k).push(point);
    },
    *near(x, y) {
      const cx = Math.floor(x / cellSize);
      const cy = Math.floor(y / cellSize);
      for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) yield* cells.get(key(cx + i, cy + j)) ?? [];
      }
    },
  };
}

export function traceStreamlines({
  width,
  height,
  separation,
  step,
  seed,
  noiseScale,
  noiseWeight,
  poles = [],
  testRatio = 0.5,
  minLength = separation * 3,
  maxSteps = 4000,
}) {
  const field = createField({ seed, noiseScale, noiseWeight, poles });
  const grid = createGrid(separation);
  const testDistance = separation * testRatio;
  const selfGap = separation * 3;
  const lines = [];

  const inBounds = (x, y) => x >= 0 && x <= width && y >= 0 && y <= height;
  const inCore = (x, y) => poles.some((pole) => Math.hypot(x - pole.x, y - pole.y) < pole.core);
  const crowded = (x, y, limit) => {
    for (const p of grid.near(x, y)) {
      if (Math.hypot(p.x - x, p.y - y) < limit) return true;
    }
    return false;
  };
  // A line may not run back alongside itself, except near the point it
  // just left.
  const crowdsItself = (x, y, arc, pending) =>
    pending.some((p) => Math.abs(p.arc - arc) >= selfGap && Math.hypot(p.x - x, p.y - y) < testDistance);
  const open = (x, y) => inBounds(x, y) && !inCore(x, y) && !crowded(x, y, separation);

  // Midpoint (RK2) steps from the seed, forward (+1) or against the flow (-1).
  function walk(x, y, direction, pending) {
    const points = [];
    let arc = 0;
    for (let i = 0; i < maxSteps; i++) {
      const [ax, ay] = field(x, y);
      if (!ax && !ay) break;
      const [bx, by] = field(x + (ax * step * direction) / 2, y + (ay * step * direction) / 2);
      if (!bx && !by) break;
      const nx = x + bx * step * direction;
      const ny = y + by * step * direction;
      arc += step;
      // Coming back around to where it began: the line is a ring.
      if (direction > 0 && arc > selfGap * 2) {
        const joint = pending.find((p) => p.arc < selfGap && Math.hypot(p.x - nx, p.y - ny) < testDistance);
        if (joint) return { points, joint };
      }
      if (!inBounds(nx, ny) || inCore(nx, ny) || crowded(nx, ny, testDistance)) break;
      if (crowdsItself(nx, ny, arc * direction, pending)) break;
      const point = { x: nx, y: ny, arc: arc * direction };
      points.push(point);
      pending.push(point);
      x = nx;
      y = ny;
    }
    return { points };
  }

  function trace(x, y) {
    const start = { x, y, arc: 0 };
    const pending = [start];
    const forward = walk(x, y, 1, pending);
    const { joint } = forward;
    const backward = joint ? { points: [] } : walk(x, y, -1, pending);
    let points = [...backward.points.reverse(), start, ...forward.points];
    if (joint) points = [...points.slice(points.indexOf(joint)), joint];
    if (points.length * step < minLength) return null;
    points.forEach((p) => grid.add(p));
    const traced = { points: points.map((p) => [p.x, p.y]), closed: Boolean(joint) };
    lines.push(traced);
    return traced;
  }

  // Seeds start just outside each pole so its rings and starbursts form
  // first, then fall back to a jittered lattice for whatever is left.
  const random = createRandom(seed + 1);
  const seeds = poles.flatMap((pole) =>
    Array.from({ length: 12 }, (_, i) => {
      const angle = (i / 12) * Math.PI * 2;
      const radius = pole.core + separation;
      return [pole.x + Math.cos(angle) * radius, pole.y + Math.sin(angle) * radius];
    }),
  );
  for (let x = separation / 2; x < width; x += separation * 2) {
    for (let y = separation / 2; y < height; y += separation * 2) {
      seeds.push([x + (random() - 0.5) * separation, y + (random() - 0.5) * separation]);
    }
  }

  // Jobard-Lefer: every accepted line offers new seeds one separation to
  // either side of it, which is what keeps the spacing even.
  const queue = [];
  const tryTrace = (x, y) => {
    if (!open(x, y)) return;
    const traced = trace(x, y);
    if (traced) queue.push(traced);
  };
  for (const [sx, sy] of seeds) {
    tryTrace(sx, sy);
    while (queue.length) {
      const { points } = queue.shift();
      for (let i = 1; i < points.length; i += 2) {
        const [ax, ay] = points[i - 1];
        const [bx, by] = points[i];
        const length = Math.hypot(bx - ax, by - ay) || 1;
        const nx = (-(by - ay) / length) * separation;
        const ny = ((bx - ax) / length) * separation;
        tryTrace(bx + nx, by + ny);
        tryTrace(bx - nx, by - ny);
      }
    }
  }
  return lines;
}

const offset = ([x, y], [ax, ay], [bx, by]) => {
  const dx = bx - ax;
  const dy = by - ay;
  const length = Math.hypot(dx, dy);
  if (!length) return Math.hypot(x - ax, y - ay);
  return Math.abs(dy * x - dx * y + bx * ay - by * ax) / length;
};

// Ramer-Douglas-Peucker: keeps the fewest points that stay within
// `tolerance` of the original line.
export function simplify(points, tolerance) {
  if (points.length < 3) return points;
  let farthest = 0;
  let index = 0;
  for (let i = 1; i < points.length - 1; i++) {
    const d = offset(points[i], points[0], points.at(-1));
    if (d > farthest) {
      farthest = d;
      index = i;
    }
  }
  if (farthest <= tolerance) return [points[0], points.at(-1)];
  return [...simplify(points.slice(0, index + 1), tolerance).slice(0, -1), ...simplify(points.slice(index), tolerance)];
}

const round = (n) => Math.round(n * 10) / 10;

// Compact SVG path data: one absolute move, then relative steps rounded to
// a tenth of a unit. Rounding absolute positions first keeps error from
// accumulating along the line.
export function toPathData(points, { closed = false } = {}) {
  const rounded = points.map(([x, y]) => [round(x), round(y)]);
  const [[x0, y0], ...rest] = rounded;
  const steps = rest.map(([x, y], i) => {
    const [px, py] = rounded[i];
    return `${round(x - px)} ${round(y - py)}`;
  });
  const path = `M${x0} ${y0}${steps.length ? `l${steps.join(" ")}` : ""}${closed ? "z" : ""}`;
  return path.replace(/ -/g, "-");
}
