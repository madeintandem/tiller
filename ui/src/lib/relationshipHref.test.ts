import { describe, expect, it } from "vitest";
import { relationshipFileHref, relationshipHref } from "./relationshipHref";

describe("relationship destinations", () => {
  it("encodes exact file coordinates without treating filename fragments as locators", () => {
    expect(relationshipFileHref(".tiller/design/notes #1.md", 19)).toBe("/file/.tiller/design/notes%20%231.md#L19");
    expect(relationshipFileHref(".tiller/design/schema:v2.md", 3)).toBe("/file/.tiller/design/schema%3Av2.md#L3");
  });

  it("refuses traversal, external coordinates, and non-prose assertion files", () => {
    for (const path of ["../outside.md", ".tiller/../outside.md", "/tmp/outside.md", "https://example.com/doc.md", "reference\\doc.md", ".tiller/state.yaml"]) {
      expect(relationshipFileHref(path, 4)).toBeNull();
    }
    expect(relationshipHref({
      id: "external_path:design/doc.md", kind: "external_path", path: "design/doc.md",
      destination: { kind: "file", path: "design/doc.md" },
    })).toBeNull();
  });

  it("does not infer current navigation from identities, paths, or unresolved capabilities", () => {
    expect(relationshipHref({ id: "F-aaaaaa", kind: "feature" })).toBeNull();
    expect(relationshipHref({ id: "doc:.tiller/doc.md", kind: "document", path: ".tiller/doc.md" })).toBeNull();
    expect(relationshipHref({
      id: "F-aaaaaa", kind: "feature", unresolved: true,
      destination: { kind: "node", noun: "feature", id: "F-bbbbbb" },
    })).toBeNull();
  });
});
