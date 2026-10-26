// Pins the phone-width fix (whole-branch merge gate finding, live 375x812
// verification): .hrow's own six-way flex row (time, dot, what, an optional
// ask clause, an optional leaf-key ref or origin label, who, a multi-entry
// burst's own toggle) never wrapped, so a narrow viewport had only one
// remaining move - shrink .what's `flex: 1 1 auto; min-width: 0` box toward
// zero and let its own text wrap word by word ("5" / "tasks" / "added" on
// three separate lines, confirmed live in flat/time mode, where BurstRow's
// extra `origin` span tips the row's width math over 375px). The same raw-
// css pattern build.css.test.ts and explore.css.test.ts already established
// (jsdom has no real layout engine, so it cannot observe wrapping at all) -
// vite.config.ts's test.css.include carve-out grew to cover history.css for
// the same reason it already covers those two.
//
// The second describe block below pins SOURCE POSITION, not just existence:
// a later fix found that an override @media block sitting BEFORE
// the base rule it means to override loses the cascade at every viewport (a
// @media wrapper adds no specificity of its own, so source order alone
// breaks the tie) - existence-only regexes stay green through that bug the
// whole time, since the base rule is still a true statement about the file
// regardless of which declaration the cascade actually crowns. This file
// pins the same property here rather than waiting to rediscover it.

import { describe, expect, it } from "vitest";

import historyCss from "./history.css?raw";

describe("history.css: .hrow wraps below 48rem instead of shrinking .what to zero", () => {
  it("declares the base .hrow rule as a non-wrapping flex row (the shape the mobile override corrects)", () => {
    expect(historyCss).toMatch(/\.hrow\s*\{[^}]*display:\s*flex;/);
  });

  it("turns .hrow's wrap on inside the 48rem breakpoint", () => {
    expect(historyCss).toMatch(/@media \(max-width: 48rem\)\s*\{\s*\.hrow\s*\{\s*flex-wrap:\s*wrap;/);
  });
});

describe("history.css: the mobile .hrow override actually wins the cascade", () => {
  const mediaMatch = historyCss.match(/@media \(max-width: 48rem\)/);
  if (!mediaMatch || mediaMatch.index === undefined) {
    throw new Error(
      "history.css no longer carries its 48rem .hrow breakpoint - update this test's split point " +
        "along with whatever restructured the file",
    );
  }
  const mediaIndex = mediaMatch.index;

  it("declares the base .hrow rule before the 48rem override block starts, so the override wins", () => {
    const baseMatch = historyCss.match(/\.hrow\s*\{/);
    expect(baseMatch, "no base .hrow rule found in history.css").not.toBeNull();
    expect(baseMatch!.index, ".hrow's base rule must precede the 48rem block").toBeLessThan(mediaIndex);
  });
});
