import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { chromium, type Browser } from "playwright";
import { ffEnv } from "../narration/elevenlabs";
import type { Capture } from "../script/schema";
import { serveMark } from "./serve";

const SIZE = { width: 1920, height: 1080 };

// Records the console at 1080p and returns a .webm (vp9) whose t=0 is the
// first frame after the page loaded (`goto` resolved and the console's
// header or main is visible) and which runs exactly `seconds`: the blank
// lead-in is trimmed with ffmpeg. Actions run after load; the rest is held.
export async function shoot(capture: Capture, outDir: string): Promise<string> {
  mkdirSync(outDir, { recursive: true });
  const raw = join(outDir, `.raw-${capture.id}`);
  rmSync(raw, { recursive: true, force: true });
  const server = await serveMark(capture.mark);
  let browser: Browser | undefined;
  try {
    browser = await chromium.launch();
    const context = await browser.newContext({
      viewport: SIZE,
      deviceScaleFactor: 1,
      colorScheme: "light",
      recordVideo: { dir: raw, size: SIZE },
    });
    const recordStart = Date.now();
    const page = await context.newPage();
    await page.goto(`${server.url}${capture.path}`);
    await page.locator("main, header").first().waitFor({ state: "visible" });
    await page.waitForTimeout(300);
    const loaded = Date.now();
    const offset = (loaded - recordStart) / 1000;
    for (const a of capture.actions ?? []) {
      if (a.waitFor) await page.locator(a.waitFor).first().waitFor();
      if (a.click) await page.locator(a.click).first().click();
      if (a.scroll !== undefined) await page.mouse.wheel(0, a.scroll);
      await page.waitForTimeout(400);
    }
    // Hold past the end so the trim always has `seconds` of footage.
    const left = capture.seconds * 1000 + 500 - (Date.now() - loaded);
    if (left > 0) await page.waitForTimeout(left);
    const video = page.video();
    await context.close();
    const out = join(outDir, `${capture.id}.webm`);
    const recorded = await video!.path();
    execFileSync(
      "ffmpeg",
      [
        "-y", "-loglevel", "error",
        "-i", recorded,
        "-ss", offset.toFixed(3),
        "-t", String(capture.seconds),
        "-an", "-c:v", "libvpx-vp9", "-b:v", "0", "-crf", "30", "-pix_fmt", "yuv420p",
        out,
      ],
      { env: ffEnv() },
    );
    return out;
  } finally {
    await browser?.close();
    rmSync(raw, { recursive: true, force: true });
    await server.stop();
  }
}
