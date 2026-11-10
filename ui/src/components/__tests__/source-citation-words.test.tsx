// A relationship source citation prints no raw token: its basis, field,
// role, scope, binding, classification, and revision kind read as words.

import { cleanup, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { RelationshipSource } from "../../api";
import SourceCitation from "../relationships/SourceCitation";

afterEach(() => {
  cleanup();
});

describe("SourceCitation words", () => {
  it("shows no underscore token in its details or pinned revision", () => {
    const source: RelationshipSource = {
      file: ".tiller/epics/a/features/b/feature.yml",
      field: "depends_on",
      basis: "work_surface",
      binding_type: "test",
      classification: "opaque_path",
      role: "addition_parent",
      state: "applied",
      scope: "historical_parent",
      revision: { kind: "git_blob", value: "git:abc" },
    };
    const { container } = render(
      <MemoryRouter>
        <ul>
          <SourceCitation source={source} />
        </ul>
      </MemoryRouter>,
    );
    const details = container.querySelector(".relationship-source-citation > .muted") as HTMLElement;
    expect(details.textContent).toBe(
      " · depends on · work surface · bound to a test · path-like, not checked · parent of the addition · applied · historical parent",
    );
    const visible = (container.textContent ?? "").replace(".tiller/epics/a/features/b/feature.yml", "");
    expect(visible).not.toMatch(/[a-z]_[a-z]/);
    expect(container.textContent).toContain("Pinned Git blob:");
  });
});
