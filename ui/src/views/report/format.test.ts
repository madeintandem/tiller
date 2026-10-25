import { describe, expect, it } from "vitest";

import { formatAsOf, formatDays, formatWeek } from "./format";

describe("formatDays", () => {
  it("shows one decimal below 10 days and whole days from 10 up", () => {
    expect(formatDays(6.46)).toBe("6.5 d");
    expect(formatDays(9.96)).toBe("10 d");
    expect(formatDays(14.2)).toBe("14 d");
  });

  it("reads under a tenth of a day as < 0.1 d, including zero", () => {
    expect(formatDays(0)).toBe("< 0.1 d");
    expect(formatDays(0.01)).toBe("< 0.1 d");
    expect(formatDays(0.049)).toBe("< 0.1 d");
    expect(formatDays(0.05)).toBe("0.1 d");
  });
});

describe("formatWeek", () => {
  it("names a Monday by month and day", () => {
    expect(formatWeek("2026-10-05")).toBe("Oct 5");
  });
});

describe("formatAsOf", () => {
  it("renders the read time as a local medium date and short time", () => {
    const iso = "2026-10-20T15:04:00Z";
    expect(formatAsOf(iso)).toBe(
      new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }),
    );
  });
});
