import { describe, expect, it } from "vitest";

import { outlineOf, slugger } from "./outline";

describe("outlineOf", () => {
  it("lists ## and ### headings with lowercase hyphenated ids", () => {
    const md = "# Title\n\n## Scope and Goals\n\ntext\n\n### Edge cases!\n\n#### Too deep\n";
    expect(outlineOf(md)).toEqual([
      { id: "scope-and-goals", text: "Scope and Goals", depth: 2 },
      { id: "edge-cases", text: "Edge cases!", depth: 3 },
    ]);
  });

  it("suffixes repeats with -2, -3", () => {
    const md = "## Notes\n\n## Notes\n\n### Notes\n";
    expect(outlineOf(md).map((i) => i.id)).toEqual(["notes", "notes-2", "notes-3"]);
  });

  it("ignores headings inside fenced code", () => {
    const md = "## Real\n\n```md\n## Fake\n```\n\n~~~\n### Also fake\n~~~\n\n## After\n";
    expect(outlineOf(md).map((i) => i.text)).toEqual(["Real", "After"]);
  });

  it("reads the text of inline markup, not its syntax", () => {
    const md = "## The `claim` verb and [a link](https://x.test)\n\n## **Bold** _tail_\n";
    expect(outlineOf(md).map((i) => i.text)).toEqual(["The claim verb and a link", "Bold tail"]);
  });

  it("falls back to a stable id for a heading with no letters", () => {
    expect(outlineOf("## ???\n\n## !!!\n").map((i) => i.id)).toEqual(["section", "section-2"]);
  });

  it("keeps snake_case tokens whole, in text and id", () => {
    expect(outlineOf("## in_review and in_flight")).toEqual([
      { id: "in-review-and-in-flight", text: "in_review and in_flight", depth: 2 },
    ]);
  });

  it("decodes escapes and entities, and strips strike and autolink marks", () => {
    const md = "## snake\\_case &amp; more\n\n## ~~gone~~ <https://x.test/a>\n";
    expect(outlineOf(md).map((i) => i.text)).toEqual(["snake_case & more", "gone https://x.test/a"]);
  });

  it("counts a heading in a blockquote toward later repeat suffixes", () => {
    const md = "> ## Notes\n\n## Notes\n";
    expect(outlineOf(md).map((i) => i.id)).toEqual(["notes", "notes-2"]);
  });

  it("ignores a fenced block inside a list item", () => {
    const md = "- item\n\n  ```\n  ## not a heading\n  ```\n\n## Real\n";
    expect(outlineOf(md).map((i) => i.text)).toEqual(["Real"]);
  });

  it("lists a setext heading like ##", () => {
    const md = "Setext Heading\n--------------\n\n## Setext Heading\n";
    expect(outlineOf(md)).toEqual([
      { id: "setext-heading", text: "Setext Heading", depth: 2 },
      { id: "setext-heading-2", text: "Setext Heading", depth: 2 },
    ]);
  });

  it("slugger counts repeats the same way", () => {
    const next = slugger();
    expect([next("A b"), next("A b"), next("A B")]).toEqual(["a-b", "a-b-2", "a-b-3"]);
  });
});
