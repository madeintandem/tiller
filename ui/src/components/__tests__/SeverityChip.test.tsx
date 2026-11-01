import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import SeverityChip from "../SeverityChip";

afterEach(cleanup);

describe("SeverityChip", () => {
  it.each([
    ["blocker", "Blocker"],
    ["important", "Important"],
    ["medium", "Medium"],
    ["minor", "Minor"],
  ] as const)("renders %s as the word %s", (severity, label) => {
    render(<SeverityChip severity={severity} />);
    const chip = screen.getByText(label);
    expect(chip).toHaveClass("severity-chip");
    expect(chip).toHaveAttribute("data-severity", severity);
  });

  it("renders Unrated when there is no severity", () => {
    render(<SeverityChip />);
    const chip = screen.getByText("Unrated");
    expect(chip).toHaveAttribute("data-severity", "unrated");
  });
});
