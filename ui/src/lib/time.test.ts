import { describe, expect, it } from "vitest";
import { formatAbsolute, formatClock, formatDayHeading, formatPoint, formatWhen } from "./time";

// Instants are built from local-time constructors so the wall-clock strings
// are the same in every timezone the suite runs in.
const at = (y: number, mo: number, d: number, h: number, mi: number, s = 0): string =>
  new Date(y, mo - 1, d, h, mi, s).toISOString();

const now = new Date(2026, 10, 9, 14, 6, 0);

describe("formatWhen", () => {
  it("says just now under a minute", () => {
    expect(formatWhen(at(2026, 11, 9, 14, 5, 30), now)).toBe("just now");
  });
  it("counts minutes under an hour", () => {
    expect(formatWhen(at(2026, 11, 9, 13, 54), now)).toBe("12m ago");
  });
  it("counts hours under a day", () => {
    expect(formatWhen(at(2026, 11, 9, 11, 6), now)).toBe("3h ago");
  });
  it("dates an older stamp in the current year without the year", () => {
    expect(formatWhen(at(2026, 11, 7, 14, 6), now)).toBe("Nov 7, 2:06 PM");
  });
  it("adds the year outside the current year", () => {
    expect(formatWhen(at(2025, 11, 9, 14, 6), now)).toBe("Nov 9, 2025, 2:06 PM");
  });
  it("returns an unparseable value unchanged", () => {
    expect(formatWhen("not a date", now)).toBe("not a date");
  });
});

describe("formatClock", () => {
  it("gives the clock reading", () => {
    expect(formatClock(at(2026, 11, 9, 14, 6))).toBe("2:06 PM");
  });
  it("returns an unparseable value unchanged", () => {
    expect(formatClock("nope")).toBe("nope");
  });
});

describe("formatDayHeading", () => {
  it("names the weekday and date", () => {
    expect(formatDayHeading(at(2026, 11, 9, 14, 6), now)).toBe("Monday · Nov 9");
  });
  it("adds the year when it is not this year", () => {
    expect(formatDayHeading(at(2025, 11, 9, 14, 6), now)).toBe("Sunday · Nov 9, 2025");
  });
  it("returns an unparseable value unchanged", () => {
    expect(formatDayHeading("nope", now)).toBe("nope");
  });
});

describe("formatPoint", () => {
  it("gives the clock reading for an instant earlier today", () => {
    expect(formatPoint(at(2026, 11, 9, 11, 26), now)).toBe("11:26 AM");
  });
  it("never reads as relative, even a few seconds back", () => {
    expect(formatPoint(at(2026, 11, 9, 14, 5, 50), now)).toBe("2:05 PM");
  });
  it("dates an instant on another day this year", () => {
    expect(formatPoint(at(2026, 11, 8, 23, 26), now)).toBe("Nov 8, 11:26 PM");
  });
  it("adds the year outside the current year", () => {
    expect(formatPoint(at(2025, 11, 9, 14, 6), now)).toBe("Nov 9, 2025, 2:06 PM");
  });
  it("returns an unparseable value unchanged", () => {
    expect(formatPoint("nope", now)).toBe("nope");
  });
});

describe("formatAbsolute", () => {
  it("gives date, year and seconds", () => {
    expect(formatAbsolute(at(2026, 11, 9, 14, 6, 28))).toBe("Nov 9, 2026, 2:06:28 PM");
  });
  it("returns an unparseable value unchanged", () => {
    expect(formatAbsolute("nope")).toBe("nope");
  });
});
