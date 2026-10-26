// Pins the phone-width fix (whole-branch merge gate finding, live 375x812
// verification): .card-grid's `grid-template-columns: repeat(auto-fill,
// minmax(430px, 1fr))` forced a single grid track to a 430px FLOOR at every
// viewport under ~874px, auto-fill never having room to pack a second one -
// so a 375px viewport got a track 55px wider than the screen, and CSS grid
// does not clip an overflowing track, it grows the grid past its own
// container. Measured live: document.documentElement.scrollWidth 644 against
// a 375px window.innerWidth, the one full-page horizontal scroll this pass
// found anywhere on the branch, dragging the topbar's own nav links
// off-screen sideways along with the needs-you cards. The same raw-css
// pattern build.css.test.ts, explore.css.test.ts, and history.css.test.ts
// already established (jsdom has no real layout engine, so it cannot
// observe an overflowing grid track at all) - vite.config.ts's
// test.css.include carve-out grew to cover home.css for the same reason.
//
// The second describe block pins SOURCE POSITION, not just existence, the
// same property a later fix found a real bug in: an override
// @media block sitting BEFORE the base rule it means to override loses the
// cascade at every viewport (a @media wrapper adds no specificity of its
// own), and an existence-only regex stays green through that the whole
// time since the base rule remains a true statement about the file
// regardless of which declaration the cascade actually crowns.

import { describe, expect, it } from "vitest";

import homeCss from "./home.css?raw";

// The needs-you body is a column grid, one column per kind, that folds to a
// single column under 1100px like the pulse beneath it (JC, 2026-08-31). Same
// raw-css pinning as the card grid below, for the same jsdom reason.
describe("home.css: the needs-you body is a column grid that folds under 1100px", () => {
  it("declares .needs-body as a grid of auto-fit 360px columns", () => {
    expect(homeCss).toMatch(/\.needs-body\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*repeat\(auto-fit, minmax\(360px, 1fr\)\);/);
  });

  it("folds the body to one minmax(0, 1fr) column inside the 1100px breakpoint, after the base rule", () => {
    // minmax(0, 1fr), never bare 1fr: a bare track's minimum is its content,
    // and the foot's one-line batch command would widen the page (seen live
    // at 1000px: a 2821px document).
    const base = homeCss.match(/\.needs-body\s*\{/);
    const fold = homeCss.match(/@media \(max-width: 1100px\)\s*\{\s*\.needs-body\s*\{\s*grid-template-columns:\s*minmax\(0, 1fr\);/);
    expect(base).not.toBeNull();
    expect(fold).not.toBeNull();
    expect(base!.index).toBeLessThan(fold!.index!);
  });
});

describe("home.css: .card-grid's 430px floor drops below 48rem", () => {
  it("declares the base .card-grid rule with the 430px minmax floor (the shape the mobile override corrects)", () => {
    expect(homeCss).toMatch(/\.card-grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(430px, 1fr\)\);/);
  });

  it("drops the floor to an unconstrained single track inside the 48rem breakpoint", () => {
    expect(homeCss).toMatch(
      /@media \(max-width: 48rem\)\s*\{\s*\.card-grid\s*\{\s*grid-template-columns:\s*1fr;/,
    );
  });
});

describe("home.css: the mobile .card-grid override actually wins the cascade", () => {
  const mediaMatch = homeCss.match(/@media \(max-width: 48rem\)/);
  if (!mediaMatch || mediaMatch.index === undefined) {
    throw new Error(
      "home.css no longer carries its 48rem .card-grid breakpoint - update this test's split point " +
        "along with whatever restructured the file",
    );
  }
  const mediaIndex = mediaMatch.index;

  it("declares the base .card-grid rule before the 48rem override block starts, so the override wins", () => {
    const baseMatch = homeCss.match(/\.card-grid\s*\{/);
    expect(baseMatch, "no base .card-grid rule found in home.css").not.toBeNull();
    expect(baseMatch!.index, ".card-grid's base rule must precede the 48rem block").toBeLessThan(
      mediaIndex,
    );
  });
});
