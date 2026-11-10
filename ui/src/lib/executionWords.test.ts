import { describe, expect, it } from "vitest";
import { diagnosticLabel, recoveryBasisLabel } from "./executionWords";

describe("executionWords", () => {
  it("words every diagnostic code the Go side emits", () => {
    for (const code of ["branch_invalid", "branch_missing", "discovery_unavailable", "divergent_refs", "invalid_candidate", "invalid_identity", "ownership_superseded", "ownership_unresolved", "remote_ambiguous", "source_unavailable", "source_unstable", "worktree_ambiguous"]) {
      expect(diagnosticLabel(code)).not.toMatch(/_/);
      expect(diagnosticLabel(code)).not.toBe(code);
    }
  });
  it("humanizes an unknown code", () => {
    expect(diagnosticLabel("brand_new_code")).toBe("Brand new code");
  });
  it("words every recovery basis and humanizes the rest", () => {
    expect(recoveryBasisLabel("content")).toBe("matching content");
    expect(recoveryBasisLabel("some_new_basis")).toBe("some new basis");
  });
});
