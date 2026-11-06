import { describe, expect, it } from "vitest";
import { copyFileSync, existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";

describe("render smoke", () => {
  it("renders the Smoke composition at 1920x1080", { timeout: 120_000 }, async () => {
    const serveUrl = await bundle({ entryPoint: resolve(__dirname, "../src/index.ts") });
    const composition = await selectComposition({ serveUrl, id: "Smoke" });
    const output = join(mkdtempSync(join(tmpdir(), "smoke-")), "smoke.png");
    await renderStill({ composition, serveUrl, frame: 60, output });
    expect(existsSync(output)).toBe(true);
    const png = readFileSync(output);
    expect(png.readUInt32BE(16)).toBe(1920);
    expect(png.readUInt32BE(20)).toBe(1080);
    if (process.env.SMOKE_COPY) copyFileSync(output, process.env.SMOKE_COPY);
  });
});
