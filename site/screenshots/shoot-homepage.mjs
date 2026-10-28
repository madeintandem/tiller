// Shoot the homepage's console images from a running console over the fixture
// homepage.sh carries on.
//
//   node shoot-homepage.mjs <port> [outDir]     default outDir: ../public
//
// Build: four epics selected, two left off in the open panel, and one card
// expanded to its tasks, cropped to the board's populated top. Explore: the
// whole tree open with the Alert routing task selected. Both 1440 wide, once per
// theme, converted with cwebp (brew install webp).
import { execFileSync } from "node:child_process";
import { unlinkSync } from "node:fs";
import { resolve } from "node:path";
import { webkit } from "playwright";

const [port = "4477", outDir = resolve(import.meta.dirname, "../public")] = process.argv.slice(2);
const base = `http://127.0.0.1:${port}`;
const WIDTH = 1440;

// name -> the frame to shoot at, the height to keep of it, and how to reach the state
const shots = {
  "console-build": {
    width: 1860,
    keep: 700,
    reach: async (page) => {
      await page.goto(`${base}/build?epics=foundation,form-engine,payments,audits`, { waitUntil: "networkidle" });
      const panel = page.locator("button.panel-toggle");
      if ((await panel.getAttribute("aria-expanded")) !== "true") await panel.click();
      await page.waitForTimeout(300);
      await page
        .locator("article, li, div")
        .filter({ has: page.getByText("Authentication and roles", { exact: true }) })
        .filter({ has: page.getByRole("button", { name: "Expand tasks" }) })
        .last()
        .getByRole("button", { name: "Expand tasks" })
        .click();
      await page.waitForTimeout(600);
    },
  },
  "console-explore": {
    width: 1680,
    keep: 910,
    reach: async (page) => {
      await page.goto(`${base}/node/task/foundation/error-tracking/alert-routing`, { waitUntil: "networkidle" });
      await page.getByRole("button", { name: "Open all" }).click();
      await page.waitForTimeout(600);
    },
  },
};

const browser = await webkit.launch();
for (const theme of ["light", "dark"]) {
  for (const [name, shot] of Object.entries(shots)) {
    const page = await browser.newPage({ viewport: { width: shot.width, height: shot.keep + 80 }, colorScheme: theme });
    await shot.reach(page);
    // Cut below the console's topbar, the way the documentation shots are cut.
    const topbar = await page.locator("header").first().boundingBox();
    const top = topbar === null ? 0 : Math.round(topbar.y + topbar.height);
    await page.setViewportSize({ width: shot.width, height: shot.keep + top });
    await page.waitForTimeout(300);
    const png = resolve(outDir, `${name}-${theme}.png`);
    await page.screenshot({ path: png, clip: { x: 0, y: top, width: shot.width, height: shot.keep } });
    const height = Math.round((shot.keep * WIDTH) / shot.width);
    const webp = resolve(outDir, `${name}-${theme}.webp`);
    execFileSync("cwebp", ["-quiet", "-q", "88", "-resize", String(WIDTH), String(height), png, "-o", webp]);
    unlinkSync(png);
    console.log(`wrote ${webp} (${WIDTH}x${height})`);
    await page.close();
  }
}
await browser.close();
