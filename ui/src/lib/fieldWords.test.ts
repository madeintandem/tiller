import { describe, expect, it } from "vitest";

import { amendmentOpLabel, changeTypeLabel, preconditionLabel, resolutionLabel } from "./fieldWords";

// Every value the Go code defines for each field (store/amendment.go's
// AmendmentOp*, mutate/amendment.go's precondition names, store/workitem.go's
// Resolution*, and the change types `tiller add feature --change-type`
// accepts) reads as plain words; anything else is humanized, never raw.
describe("amendmentOpLabel", () => {
  it.each([
    ["revise", "feature", "Feature revised"],
    ["supersede", "task", "Task superseded"],
    ["cancel", "task", "Task cancelled"],
    ["add-epic", undefined, "Epic added"],
    ["add-feature", undefined, "Feature added"],
    ["add-task", undefined, "Task added"],
    ["add-question", undefined, "Question asked"],
    ["move-feature", undefined, "Feature moved"],
    ["correct-eval", "feature", "Feature eval corrected"],
  ])("%s on a %s reads %s", (op, noun, label) => {
    expect(amendmentOpLabel(op, noun)).toBe(label);
  });

  it("reads a target-less revise, supersede, or cancel without a noun", () => {
    expect(amendmentOpLabel("revise")).toBe("Revised");
    expect(amendmentOpLabel("cancel")).toBe("Cancelled");
  });

  it("humanizes an op from a newer server", () => {
    expect(amendmentOpLabel("future-kind")).toBe("Future kind");
    expect(amendmentOpLabel("split_feature")).toBe("Split feature");
  });
});

describe("preconditionLabel", () => {
  it.each([
    ["target_status", "Target status"],
    ["target_revision", "Target revision"],
    ["key_absent", "Key not yet in use"],
    ["spec_revision", "Spec revision"],
    ["eval_revision", "Eval revision"],
    ["future_guard", "Future guard"],
  ])("%s reads %s", (name, label) => {
    expect(preconditionLabel(name)).toBe(label);
  });
});

describe("resolutionLabel", () => {
  it.each([
    ["wontfix", "Won't fix"],
    ["invalid", "Not valid"],
    ["duplicate", "Duplicate"],
    ["transferred", "Transferred"],
    ["moved_elsewhere", "Moved elsewhere"],
  ])("%s reads %s", (resolution, label) => {
    expect(resolutionLabel(resolution)).toBe(label);
  });
});

describe("changeTypeLabel", () => {
  it.each([
    ["create", "New code"],
    ["modify", "Changes existing code"],
    ["replace", "Rewrites existing code"],
    ["migrate", "Moves code to a new pattern"],
    ["port_over", "Port over"],
  ])("%s reads %s", (changeType, label) => {
    expect(changeTypeLabel(changeType)).toBe(label);
  });
});
