import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { layoutFor, type Layout } from "../src/build/layout.js";
import { clearOutputs, outputPaths } from "../src/build/outputs.js";

let root: string;
let layout: Layout;
beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "outputs-"));
  layout = layoutFor(root, {});
  mkdirSync(layout.out);
});
afterEach(() => rmSync(root, { recursive: true, force: true }));

describe("clearOutputs", () => {
  it("removes a previous build's mp4, srt, jpg, and meta.json for that id only", () => {
    for (const ext of ["mp4", "srt", "jpg", "meta.json"]) {
      writeFileSync(join(layout.out, `v1.${ext}`), "old");
      writeFileSync(join(layout.out, `v2.${ext}`), "keep");
    }
    clearOutputs(layout, "v1");
    for (const p of Object.values(outputPaths(layout, "v1"))) expect(existsSync(p)).toBe(false);
    for (const p of Object.values(outputPaths(layout, "v2"))) expect(existsSync(p)).toBe(true);
  });

  it("is quiet when there is nothing to remove", () => {
    rmSync(layout.out, { recursive: true });
    expect(() => clearOutputs(layout, "v1")).not.toThrow();
  });
});
