import { describe, expect, it } from "vitest";
import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { theme } from "../src/theme";

const guide = join(homedir(), "dev/mit-style-guide/mit-style-guide-2026-v1.html");
const camel = (s: string) => s.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());

describe("theme", () => {
  it("has exactly 18 colors, video and font constants", () => {
    expect(Object.keys(theme.colors)).toHaveLength(18);
    expect(theme.fonts).toEqual({ display: "Space Grotesk", body: "Inter", mono: "Space Mono" });
    expect(theme.video).toEqual({ width: 1920, height: 1080, fps: 30 });
  });

  it("keeps lower third and frame content clear of the wordmark by at least 1X", () => {
    const w = theme.wordmark;
    const x = w.height / 2;
    const wordmarkTop = w.bottom + w.height;
    const src = (f: string) => readFileSync(join(__dirname, "../src/components", f), "utf8");
    // Both components must take their offset from the shared constant.
    expect(src("LowerThird.tsx")).toContain("bottom: wordmark.clearTop");
    expect(src("Frame.tsx")).toContain("${wordmark.clearTop}px");
    expect(w.clearTop - wordmarkTop).toBeGreaterThanOrEqual(x);
  });

  it.skipIf(!existsSync(guide))("matches the --color-* tokens in the MiT style guide", () => {
    const html = readFileSync(guide, "utf8");
    const parsed = new Map<string, string>();
    for (const m of html.matchAll(/--color-([a-z-]+):\s*(#[0-9A-Fa-f]{6})/g)) {
      parsed.set(camel(m[1]), m[2].toUpperCase());
    }
    expect(parsed.size).toBe(18);
    for (const [name, hex] of Object.entries(theme.colors)) {
      expect(parsed.get(name), name).toBe(hex.toUpperCase());
    }
  });

  if (!existsSync(guide)) {
    it("notes that the style guide is absent", () => {
      console.warn(`style guide not found at ${guide}; hex comparison skipped`);
    });
  }
});
