import { describe, expect, it } from "vitest";

import { formatAdded } from "./added";

describe("formatAdded", () => {
  const now = new Date(2026, 10, 1, 12, 0, 0);

  it("omits the year within the current year", () => {
    expect(formatAdded(new Date(2026, 9, 12, 12, 0, 0).toISOString(), now)).toBe(
      `Added ${new Date(2026, 9, 12).toLocaleDateString(undefined, { month: "short", day: "numeric" })}`,
    );
  });

  it("adds the year when it differs", () => {
    expect(formatAdded(new Date(2025, 9, 12, 12, 0, 0).toISOString(), now)).toBe(
      `Added ${new Date(2025, 9, 12).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`,
    );
  });
});
