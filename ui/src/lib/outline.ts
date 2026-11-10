// outlineOf lists a spec's own section headings (source `##` and `###`) for the
// "On this page" outline, and slugger is the one id rule both it and Prose's
// heading ids derive from, so every outline link resolves to a heading.
//
// Source `#` is left out on purpose: a spec's opening title repeats the page
// header, and any later `#` is a title-level break, not a section. Levels four
// and below are detail, not navigation.

import type { Root, RootContent } from "mdast";
import { toString } from "mdast-util-to-string";
import remarkGfm from "remark-gfm";
import remarkParse from "remark-parse";
import { unified } from "unified";

export interface OutlineItem {
  id: string;
  text: string;
  depth: 2 | 3;
}

// slugger returns an id generator: lowercase letters and digits joined by
// hyphens, a heading with none becoming "section", and a repeat of an id
// already issued taking the suffix -2, -3, and so on.
export function slugger(): (text: string) => string {
  const seen = new Map<string, number>();
  return (text) => {
    const base =
      text
        .toLowerCase()
        .replace(/[^\p{L}\p{N}]+/gu, "-")
        .replace(/^-+|-+$/g, "") || "section";
    const count = (seen.get(base) ?? 0) + 1;
    seen.set(base, count);
    return count === 1 ? base : `${base}-${count}`;
  };
}

// outlineOf reads the same parse Prose renders from (remark-parse with
// remark-gfm, the pipeline react-markdown runs), so a heading's text and its
// place in the id sequence cannot differ from what Prose gives it. Every
// heading Prose gives an id (source ## and ###, wherever it sits: top level,
// a blockquote, a list item, or a setext underline) draws from the one
// slugger in document order, and all of them are listed, so repeat suffixes
// always line up. Fenced and indented code is code in the tree, never a
// heading.
const PARSER = unified().use(remarkParse).use(remarkGfm);

export function outlineOf(markdown: string): OutlineItem[] {
  const items: OutlineItem[] = [];
  const next = slugger();
  const walk = (node: Root | RootContent): void => {
    if (node.type === "heading") {
      if (node.depth === 2 || node.depth === 3) {
        const text = toString(node).replace(/\s+/g, " ").trim();
        items.push({ id: next(text), text, depth: node.depth });
      }
      return;
    }
    if ("children" in node) node.children.forEach(walk);
  };
  walk(PARSER.parse(markdown));
  return items;
}
