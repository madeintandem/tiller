// outlineOf lists a spec's own section headings (source `##` and `###`) for the
// "On this page" outline, and slugger is the one id rule both it and Prose's
// heading ids derive from, so every outline link resolves to a heading.
//
// Source `#` is left out on purpose: a spec's opening title repeats the page
// header, and any later `#` is a title-level break, not a section. Levels four
// and below are detail, not navigation.

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

// plainText reduces a heading's inline Markdown to the text a reader sees:
// the same text Prose's heading ids are built from (images show their alt,
// links their label, code and emphasis lose their marks).
function plainText(raw: string): string {
  return raw
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/`+([^`]*)`+/g, "$1")
    .replace(/(\*\*|__|\*|_)(.+?)\1/g, "$2")
    .replace(/\s+/g, " ")
    .trim();
}

const FENCE = /^ {0,3}(`{3,}|~{3,})/;
const HEADING = /^ {0,3}(#{2,3})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/;

export function outlineOf(markdown: string): OutlineItem[] {
  const items: OutlineItem[] = [];
  const next = slugger();
  let fence: string | null = null;
  for (const line of markdown.split(/\r?\n/)) {
    const open = FENCE.exec(line);
    if (fence !== null) {
      // A closing fence uses the same character, at least as long, and
      // carries nothing after it.
      if (open !== null && open[1][0] === fence[0] && open[1].length >= fence.length && line.trim() === open[1]) {
        fence = null;
      }
      continue;
    }
    if (open !== null) {
      fence = open[1];
      continue;
    }
    const match = HEADING.exec(line);
    if (match === null) continue;
    const text = plainText(match[2]);
    items.push({ id: next(text), text, depth: match[1].length === 2 ? 2 : 3 });
  }
  return items;
}
