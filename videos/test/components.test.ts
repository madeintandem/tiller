import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { chapterFrames, lowerThirdOpacity } from "../src/components/timing";

describe("chapter lower third timing", () => {
  it("lasts 3 s when the slot allows", () => {
    expect(chapterFrames(100, 400, 30)).toBe(90);
  });

  it("never outlives a short card's slot", () => {
    expect(chapterFrames(100, 160, 30)).toBe(60);
  });

  it("fades in, holds, and fades out before its end", () => {
    expect(lowerThirdOpacity(0, 90)).toBe(0);
    expect(lowerThirdOpacity(45, 90)).toBe(1);
    expect(lowerThirdOpacity(89, 90)).toBeLessThan(0.15);
    expect(lowerThirdOpacity(90, 90)).toBe(0);
  });

  it("still reaches full opacity in a short sequence", () => {
    const peak = Math.max(...Array.from({ length: 30 }, (_, f) => lowerThirdOpacity(f, 30)));
    expect(peak).toBe(1);
  });
});

describe("outro", () => {
  it("hides the frame's corner wordmark, since it shows the full logo", () => {
    const src = (f: string) => readFileSync(join(__dirname, "../src/components", f), "utf8");
    expect(src("Outro.tsx")).toContain("<Frame wordmark={false}>");
    expect(src("Frame.tsx")).toContain("wordmark: showWordmark = true");
    expect(src("Frame.tsx")).toContain("{showWordmark && (");
  });
});
