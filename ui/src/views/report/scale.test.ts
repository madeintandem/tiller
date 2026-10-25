import { describe, expect, it } from "vitest";

import { dateX, linearScale, niceTicks, weekX } from "./scale";

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
const WEEK_BOUNDARIES = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00Z`);

describe("linearScale", () => {
  it("maps the domain onto the range", () => {
    expect(linearScale([0, 10], [0, 100])(5)).toBe(50);
    expect(linearScale([0, 10], [200, 0])(10)).toBe(0);
  });

  it("maps a zero-width domain to the range start, never NaN", () => {
    expect(linearScale([0, 0], [0, 100])(0)).toBe(0);
    expect(linearScale([3, 3], [40, 10])(7)).toBe(40);
  });
});

describe("niceTicks", () => {
  it("returns [0] when there is nothing to scale", () => {
    expect(niceTicks(0)).toEqual([0]);
    expect(niceTicks(-2)).toEqual([0]);
  });

  it("counts in whole steps from 0 to at least the max", () => {
    expect(niceTicks(3)).toEqual([0, 1, 2, 3]);
    expect(niceTicks(8)).toEqual([0, 2, 4, 6, 8]);
    expect(niceTicks(9)).toEqual([0, 5, 10]);
    expect(niceTicks(0.4)).toEqual([0, 1]);
  });
});

describe("weekX", () => {
  it("places each week at its band's center", () => {
    const x = weekX(WEEKS, 400);
    expect(x(0)).toBe(50);
    expect(x(3)).toBe(350);
  });
});

describe("dateX", () => {
  it("places an instant inside its week's band by how far into the week it falls", () => {
    const x = dateX(WEEK_BOUNDARIES, 400);
    expect(x("2026-10-05T00:00:00Z")).toBe(100);
    expect(x("2026-10-08T12:00:00Z")).toBe(150);
  });

  it("keeps instants outside the axis on its edges", () => {
    const x = dateX(WEEK_BOUNDARIES, 400);
    expect(x("2026-09-01T00:00:00Z")).toBe(0);
    expect(x("2026-12-01T00:00:00Z")).toBe(400);
  });

  it("uses server offsets even when a remote reader's calendar date differs", () => {
    const boundaries = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00+09:00`);
    const x = dateX(boundaries, 400);
    expect(x("2026-10-18T15:00:00Z")).toBe(300);
    expect(x("2026-10-18T08:30:00-07:00")).toBeCloseTo(300 + 100 / 336);
    expect(x("2026-10-18T15:30:00Z")).toBeCloseTo(300 + 100 / 336);
  });

  it.each([
    {
      boundaries: [
        "2026-02-23T00:00:00-05:00",
        "2026-03-02T00:00:00-05:00",
        "2026-03-09T00:00:00-04:00",
      ],
      midpoint: "2026-03-05T16:30:00Z",
      hours: 167,
    },
    {
      boundaries: [
        "2026-10-19T00:00:00-04:00",
        "2026-10-26T00:00:00-04:00",
        "2026-11-02T00:00:00-05:00",
      ],
      midpoint: "2026-10-29T16:30:00Z",
      hours: 169,
    },
  ])("scales the final DST week using its $hours-hour boundary span", ({ boundaries, midpoint }) => {
    const x = dateX(boundaries, 200);
    expect(x(boundaries[1])).toBe(100);
    expect(x(midpoint)).toBe(150);
    expect(x(boundaries[2])).toBe(200);
    // The previous, ordinary week still gets exactly the same band width.
    const previousMidpoint = new Date((Date.parse(boundaries[0]) + Date.parse(boundaries[1])) / 2).toISOString();
    expect(x(previousMidpoint)).toBe(50);
  });
});
