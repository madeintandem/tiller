import { rmSync } from "node:fs";
import { join } from "node:path";
import type { Layout } from "./layout.js";

/** The four files a build leaves in out/ and publish uploads. */
export function outputPaths(layout: Layout, id: string) {
  const file = (ext: string) => join(layout.out, `${id}.${ext}`);
  return { mp4: file("mp4"), srt: file("srt"), jpg: file("jpg"), meta: file("meta.json") };
}

/**
 * Remove a previous build's outputs for `id`, so a build that stops partway can never leave
 * an old video paired with new metadata (or a fake render with real metadata).
 */
export function clearOutputs(layout: Layout, id: string): void {
  for (const p of Object.values(outputPaths(layout, id))) rmSync(p, { force: true });
}
