import { describe, expect, it } from "vitest";

import {
  sourceBasisLabel,
  sourceBindingLabel,
  sourceClassificationLabel,
  sourceFieldLabel,
  sourceRevisionKindLabel,
  sourceRoleLabel,
  sourceScopeLabel,
  sourceStateLabel,
} from "./sourceWords";

// Every value internal/relationships assigns to a relationship source's
// field, basis, binding type, classification, role, state, scope, and
// revision kind reads as plain words. A value from a newer server is
// humanized; a path or an id is left exactly as it is.
describe("relationship source words", () => {
  it.each([
    ["work_surface", "work surface"],
    ["document_coordinate", "document location"],
    ["baseline_description", "baseline description"],
    ["correct-eval", "eval corrected"],
    ["move-feature", "feature moved"],
    ["future_basis", "future basis"],
  ])("basis %s reads %s", (basis, label) => {
    expect(sourceBasisLabel(basis)).toBe(label);
  });

  it.each([
    ["depends_on", "depends on"],
    ["surface.paths", "surface paths"],
    ["derived_from", "derived from"],
    ["assumption_basis.Q-abc123", "assumption basis Q-abc123"],
    ["criteria[2].binding.ref", "criteria[2] binding ref"],
    ["as-is/model.md.describes[0]", "as-is/model.md describes[0]"],
  ])("field %s reads %s", (field, label) => {
    expect(sourceFieldLabel(field)).toBe(label);
  });

  it.each([
    ["test", "bound to a test"],
    ["command", "bound to a command"],
    ["manual", "a manual check"],
    ["eval", "bound to a rubric"],
  ])("binding %s reads %s", (binding, label) => {
    expect(sourceBindingLabel(binding)).toBe(label);
  });

  it.each([
    ["path", "a path"],
    ["opaque", "not a path"],
    ["opaque_path", "path-like, not checked"],
  ])("classification %s reads %s", (value, label) => {
    expect(sourceClassificationLabel(value)).toBe(label);
  });

  it.each([
    ["target", "target"],
    ["replacement", "replacement"],
    ["created", "created"],
    ["addition_parent", "parent of the addition"],
    ["move_source_parent", "parent moved from"],
    ["move_destination_parent", "parent moved to"],
    ["question_block", "blocked by the question"],
    ["surface", "surface"],
    ["artifact", "artifact"],
  ])("role %s reads %s", (role, label) => {
    expect(sourceRoleLabel(role)).toBe(label);
  });

  it.each([
    ["proposed", "proposed"],
    ["applied", "applied"],
  ])("state %s reads %s", (state, label) => {
    expect(sourceStateLabel(state)).toBe(label);
  });

  it.each([
    ["direct", "direct"],
    ["descendant", "descendant"],
    ["historical_parent", "historical parent"],
    ["app/calc_models", "app/calc_models"],
  ])("scope %s reads %s", (scope, label) => {
    expect(sourceScopeLabel(scope)).toBe(label);
  });

  it.each([
    ["git_blob", "Git blob"],
    ["git_commit", "Git commit"],
    ["git_tree", "Git tree"],
    ["sha256", "SHA-256"],
  ])("revision kind %s reads %s", (kind, label) => {
    expect(sourceRevisionKindLabel(kind)).toBe(label);
  });
});
