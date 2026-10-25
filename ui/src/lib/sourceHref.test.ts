// sourceHref turns a node's recorded source citation into the one-hop link the
// Spec head carries (node header pass, 2026-08-30): a repo-relative path opens
// the console's own file page, which addresses a line through the canonical
// `#L<n>` fragment FileDetail requires; a citation that already names a line
// or a range keeps its first line; a URL is left to open itself.
import { describe, expect, it } from "vitest";
import { sourceHref } from "./sourceHref";

describe("sourceHref", () => {
  it("opens a bare path on the file page at its first line", () => {
    expect(sourceHref("design/02-domain-model.md")).toEqual({
      href: "/file/design/02-domain-model.md#L1",
      external: false,
    });
  });

  it("keeps the first line of a cited range", () => {
    expect(sourceHref(".tiller/feedback/07-journal.md#L25-31")).toEqual({
      href: "/file/.tiller/feedback/07-journal.md#L25",
      external: false,
    });
  });

  it("keeps a single cited line", () => {
    expect(sourceHref("docs/x.md#L7").href).toBe("/file/docs/x.md#L7");
  });

  it("encodes each path segment the way FileDetail decodes it", () => {
    expect(sourceHref("docs/a b/c%2Fd.md").href).toBe("/file/docs/a%20b/c%252Fd.md#L1");
  });

  it("passes a URL through as an external link", () => {
    expect(sourceHref("https://example.com/spec")).toEqual({
      href: "https://example.com/spec",
      external: true,
    });
  });
});
