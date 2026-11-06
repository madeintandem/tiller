import { mkdirSync, renameSync, rmSync } from "node:fs";
import { join } from "node:path";
import { chromium, type Browser } from "playwright";
import type { Capture } from "../script/schema";
import { serveMark } from "./serve";

const SIZE = { width: 1920, height: 1080 };

// Records the console at 1080p: load `path`, run the actions, hold until
// `seconds` have passed since the page loaded. Returns the .webm path.
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
    const page = await context.newPage();
    await page.goto(`${server.url}${capture.path}`, { waitUntil: "networkidle" });
    const loaded = Date.now();
    for (const a of capture.actions ?? []) {
      if (a.waitFor) await page.locator(a.waitFor).first().waitFor();
      if (a.click) await page.locator(a.click).first().click();
      if (a.scroll !== undefined) await page.mouse.wheel(0, a.scroll);
      await page.waitForTimeout(400);
    }
    const left = capture.seconds * 1000 - (Date.now() - loaded);
    if (left > 0) await page.waitForTimeout(left);
    const video = page.video();
    await context.close();
    const out = join(outDir, `${capture.id}.webm`);
    renameSync(await video!.path(), out);
    return out;
  } finally {
    await browser?.close();
    rmSync(raw, { recursive: true, force: true });
    await server.stop();
  }
}
