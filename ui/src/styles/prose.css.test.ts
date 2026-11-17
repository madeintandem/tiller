// Pins when the "On this page" outline gives way to a single-column spec, a
// layout fact jsdom cannot observe. The outline needs 260px (220px column and
// 40px gap) and the prose column shrinks, so the outline stays until the
// prose would fall under 480px (TW-k4q2yd).

import { describe, expect, it } from "vitest";

import proseCss from "./prose.css?raw";

describe("prose outline", () => {
  it("sizes the outline beside a shrinking prose column", () => {
    expect(proseCss).toMatch(
      /\.prose-with-outline\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*72ch\)\s+220px;[^}]*column-gap:\s*40px;/,
    );
  });

  it("hides the outline only under a 740px host", () => {
    const queries = [...proseCss.matchAll(/@container\s*\(max-width:\s*(\d+)px\)/g)].map((m) => Number(m[1]));
    expect(queries).toEqual([739]);
  });
});
