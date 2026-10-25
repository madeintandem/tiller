// The fold table's total mapping over the wire's working-state words. A task
// (and a bug or chore, which share its lifecycle) works in `in_progress` and
// `check`, neither of which is one of the seven chart states; before the node
// header pass (2026-08-30) both fell to the draft bucket, so a task mid-work
// painted draft gray in its own header chip. Both are the agent's move on
// live work, which is what `active` means one grain up.
import { describe, expect, it } from "vitest";
import { foldStatus, statusColor, statusTextColor } from "./states";

describe("foldStatus: a task's working states paint active", () => {
  it("folds in_progress to active", () => {
    expect(foldStatus("in_progress")).toBe("active");
    expect(statusColor("in_progress")).toBe("var(--st-active)");
    expect(statusTextColor("in_progress")).toBe("var(--st-active-t)");
  });

  it("folds check to active", () => {
    expect(foldStatus("check")).toBe("active");
    expect(statusColor("check")).toBe("var(--st-active)");
  });

  it("keeps the existing folds and the draft rest", () => {
    expect(foldStatus("proposed")).toBe("specced");
    expect(foldStatus("open")).toBe("in_review");
    expect(foldStatus("answered")).toBe("done");
    expect(foldStatus("cancelled")).toBe("draft");
    expect(foldStatus("done")).toBe("done");
  });
});
