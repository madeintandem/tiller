// Moved from lib/clampMarkdown.test.ts (task 17, console v2 node page
// rebuild): stripLeadingTitle's own coverage, unchanged, now that the fold
// it used to sit beside is gone.

import { describe, expect, it } from "vitest";

import { stripLeadingTitle } from "./stripLeadingTitle";

describe("stripLeadingTitle", () => {
  it("drops an opening heading that only repeats the node's own title", () => {
    expect(stripLeadingTitle("# Audit Engine\n\nIntent text.\n", "Audit Engine")).toBe(
      "Intent text.\n",
    );
  });

  it("ignores case and surrounding space, since the file is hand-written", () => {
    expect(stripLeadingTitle("#   audit engine  \n\nIntent.\n", "Audit Engine")).toBe("Intent.\n");
  });

  it("keeps a heading that says something the h1 did not", () => {
    const md = "# Selection policy, in detail\n\nIntent.\n";
    expect(stripLeadingTitle(md, "Audit Engine")).toBe(md);
  });

  it("keeps everything when the node carries no title", () => {
    const md = "# Audit Engine\n\nIntent.\n";
    expect(stripLeadingTitle(md, undefined)).toBe(md);
  });

  it("touches only a LEADING heading, never one further down", () => {
    const md = "Intent.\n\n# Audit Engine\n";
    expect(stripLeadingTitle(md, "Audit Engine")).toBe(md);
  });
});
