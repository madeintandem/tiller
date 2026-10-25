// Vitest coverage for the TS<->CSS token-name seam states.ts fabricates by
// template: stateColor/statusTextColor build `--st-<state>` / `--st-<state>-
// t` names from a state string rather than looking them up, so the actual
// declaration lives only in tokens.css and nothing here would ever fail if
// tokens.css renamed one, or if a name existed in one theme layer (the plain
// :root fallback, the @supports light-dark() block) but not the other - the
// suite would stay green and a themed dot would paint transparent. This file
// reads tokens.css's own literal source (via Vite's `?raw` import, a raw
// string rather than jsdom's inert `var(...)` echo - vite.config.ts scopes
// vitest's default CSS-import stub off for this one file so the real source
// comes through) and asserts every name states.ts can produce is actually
// declared, in both theme layers, rather than trusting the two files stay in
// sync by hand.

import { describe, expect, it } from "vitest";

import tokensCss from "../styles/tokens.css?raw";
import type { NodeStatus } from "../api";

// The seven chart states, mirrored from lib/states.ts's own CHART_STATES
// (not exported - it exists solely for isChartState's internal use, and
// exporting it just to dedupe seven literals here would widen states.ts's
// public surface for no reader's benefit). Keep in sync by hand if a state
// is ever added or renamed; primitives.test.tsx's own SEVEN_STATES carries
// the same duplication for the same reason.
const SEVEN_STATES: NodeStatus[] = [
  "draft",
  "specced",
  "ready",
  "active",
  "in_review",
  "done",
  "blocked",
];

// tokens.css's two theme layers: the plain :root fallback (a browser with no
// light-dark() support) and the @supports (color: light-dark(...)) block
// that follows it. Splitting the source on the @supports boundary lets each
// assertion below check a name is declared on ITS OWN side, rather than
// searching the whole file and passing even if a name only ever lands in
// one of the two.
//
// The header comment names this same rule in prose (explaining why --tint
// can't ride light-dark()), so a plain indexOf() would find that mention
// first rather than the actual rule; anchoring to the start of a line finds
// the real at-rule, which (unlike the indented prose) opens flush left.
const supportsMatch = tokensCss.match(/^@supports \(color: light-dark\(/m);
if (!supportsMatch || supportsMatch.index === undefined) {
  throw new Error(
    "tokens.css no longer has an @supports (color: light-dark(...)) block - " +
      "update this test's split point along with whatever restructured the file",
  );
}
const supportsIndex = supportsMatch.index;
const fallbackLayer = tokensCss.slice(0, supportsIndex);
const themedLayer = tokensCss.slice(supportsIndex);

describe("tokens.css declares every name states.ts can fabricate", () => {
  it.each(SEVEN_STATES)("declares --st-%s in the fallback :root", (state) => {
    expect(fallbackLayer).toMatch(new RegExp(`--st-${state}:`));
  });

  it.each(SEVEN_STATES)("declares --st-%s in the light-dark() @supports block", (state) => {
    expect(themedLayer).toMatch(new RegExp(`--st-${state}:`));
  });

  it.each(SEVEN_STATES)("declares --st-%s-t in the fallback :root", (state) => {
    expect(fallbackLayer).toMatch(new RegExp(`--st-${state}-t:`));
  });

  it.each(SEVEN_STATES)("declares --st-%s-t in the light-dark() @supports block", (state) => {
    expect(themedLayer).toMatch(new RegExp(`--st-${state}-t:`));
  });
});

describe("tokens.css declares --tint in every layer Chip's color-mix() can read it from", () => {
  it("declares the fallback 13% in the plain :root", () => {
    expect(fallbackLayer).toMatch(/--tint:\s*13%/);
  });

  // --tint's dark bump can't ride light-dark() itself (that function only
  // themes colors, never a bare percentage - tokens.css's own header
  // comment says why), so it lives in two separate overrides instead: a
  // prefers-color-scheme media query for an unforced dark OS, and an
  // explicit [data-theme="dark"] rule for a forced dark theme under a light
  // OS, which the media query alone would never see.
  it("bumps --tint to 22% under prefers-color-scheme: dark", () => {
    expect(themedLayer).toMatch(/@media \(prefers-color-scheme: dark\)[^}]*--tint:\s*22%/s);
  });

  it("bumps --tint to 22% under an explicit [data-theme=\"dark\"]", () => {
    expect(themedLayer).toMatch(/\[data-theme="dark"\][^}]*--tint:\s*22%/s);
  });
});

// Phase 0 lost the JS-side contrast test when states.ts moved from computed
// hex values to var() emission: the old suite could compute a ratio directly
// from the color it built, but a var() reference resolves to nothing in
// jsdom, so that check simply disappeared rather than moving anywhere. This
// block is the measurement half nothing above replaces: it parses tokens.
// css's own literal hex values (the same ?raw import and light/dark layer
// split the name-pinning suite above already established) and computes each
// --st-<state>-t text twin's WCAG contrast against the two backgrounds it
// actually renders on - Chip's own tinted wash and --panel directly - rather
// than trusting a reviewer's manual check to still hold after the next
// palette edit.
describe("tokens.css: every --st-*-t text twin clears AA contrast (4.5:1)", () => {
  // hexToRgb/relativeLuminance/contrastRatio are the WCAG 2 formulas verbatim
  // (Web Content Accessibility Guidelines, "Contrast Minimum": relative
  // luminance from linearized sRGB, contrast as (lighter+0.05)/(darker+0.05)).
  function hexToRgb(hex: string): [number, number, number] {
    const n = parseInt(hex, 16);
    return [(n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff];
  }

  function relativeLuminance(hex: string): number {
    const channels = hexToRgb(hex).map((byte) => {
      const s = byte / 255;
      return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
    });
    const [r, g, b] = channels;
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  }

  function contrastRatio(hexA: string, hexB: string): number {
    const la = relativeLuminance(hexA);
    const lb = relativeLuminance(hexB);
    const lighter = Math.max(la, lb);
    const darker = Math.min(la, lb);
    return (lighter + 0.05) / (darker + 0.05);
  }

  // srgbMix reproduces color-mix(in srgb, A pct%, B) verbatim (CSS Color 4):
  // interpolation runs on the gamma-encoded byte values directly, never in
  // linear light, and a lone percentage on the first color leaves the second
  // its exact complement - the shape every .chip fill call site in tokens.css
  // uses (`color-mix(in srgb, var(--st-x) var(--tint), var(--panel))`).
  function srgbMix(hexA: string, hexB: string, pctA: number): string {
    const a = hexToRgb(hexA);
    const b = hexToRgb(hexB);
    return a
      .map((byte, i) => Math.round(byte * pctA + b[i] * (1 - pctA)))
      .map((byte) => byte.toString(16).padStart(2, "0"))
      .join("");
  }

  // parseHex finds name's own plain (non-light-dark) declaration, the
  // fallback layer's only form, e.g. `--panel: #ffffff;`.
  function parseHex(layer: string, name: string): string {
    const m = layer.match(new RegExp(`--${name}:\\s*#([0-9a-fA-F]{6})`));
    if (!m) {
      throw new Error(`tokens.css: no plain hex declaration for --${name}`);
    }
    return m[1];
  }

  // parseLightDarkHex finds name's light-dark(#light, #dark) pair, the
  // themed layer's own form for every token this suite reads.
  function parseLightDarkHex(layer: string, name: string): { light: string; dark: string } {
    const m = layer.match(
      new RegExp(`--${name}:\\s*light-dark\\(#([0-9a-fA-F]{6}),\\s*#([0-9a-fA-F]{6})\\)`),
    );
    if (!m) {
      throw new Error(`tokens.css: no light-dark() declaration for --${name}`);
    }
    return { light: m[1], dark: m[2] };
  }

  // parsePercent finds name's own N% value, generic over --tint's two
  // distinct declarations (the fallback :root's plain 13%, and the dark
  // bump's 22% the media query and [data-theme="dark"] rule share).
  function parsePercent(layer: string, name: string): number {
    const m = layer.match(new RegExp(`--${name}:\\s*(\\d+)%`));
    if (!m) {
      throw new Error(`tokens.css: no percentage declaration for --${name}`);
    }
    return Number(m[1]) / 100;
  }

  const panel = { light: parseHex(fallbackLayer, "panel"), dark: parseLightDarkHex(themedLayer, "panel").dark };
  const tint = { light: parsePercent(fallbackLayer, "tint"), dark: parsePercent(themedLayer, "tint") };

  it.each(SEVEN_STATES)("%s: the light text twin clears 4.5:1 against its own chip wash", (state) => {
    const text = parseHex(fallbackLayer, `st-${state}-t`);
    const dot = parseHex(fallbackLayer, `st-${state}`);
    const wash = srgbMix(dot, panel.light, tint.light);
    expect(contrastRatio(text, wash)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(SEVEN_STATES)("%s: the light text twin clears 4.5:1 against --panel directly", (state) => {
    const text = parseHex(fallbackLayer, `st-${state}-t`);
    expect(contrastRatio(text, panel.light)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(SEVEN_STATES)("%s: the dark text twin clears 4.5:1 against its own chip wash", (state) => {
    const text = parseLightDarkHex(themedLayer, `st-${state}-t`).dark;
    const dot = parseLightDarkHex(themedLayer, `st-${state}`).dark;
    const wash = srgbMix(dot, panel.dark, tint.dark);
    expect(contrastRatio(text, wash)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(SEVEN_STATES)("%s: the dark text twin clears 4.5:1 against --panel directly", (state) => {
    const text = parseLightDarkHex(themedLayer, `st-${state}-t`).dark;
    expect(contrastRatio(text, panel.dark)).toBeGreaterThanOrEqual(4.5);
  });
});
