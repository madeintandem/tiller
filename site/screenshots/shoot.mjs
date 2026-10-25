// Shoot the documentation screenshots from a running console.
//
//   node shoot.mjs <port> [outDir]     default outDir: ../public
//
// Every shot is 1440x780, the viewport the site's <img> tags declare, in
// WebKit (the engine the maintainers browse in), once per theme via the
// prefers-color-scheme the console's AUTO setting follows. PNGs are written
// beside the WebPs and converted with cwebp (brew install webp); if cwebp is
// missing the PNGs stay and the script says so.
import { execFileSync } from "node:child_process";
import { existsSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";
import { webkit } from "playwright";

const [port = "4477", outDir = resolve(import.meta.dirname, "../public")] = process.argv.slice(2);
const base = `http://127.0.0.1:${port}`;

// name -> how to reach the state, and optionally a wider frame to shoot
// at (scaled back down to the site's width on conversion) for a lens that
// needs the room. Each reach returns after the page has settled.
const shots = {
  "console-home": {
    reach: async (page) => {
      await page.goto(`${base}/`, { waitUntil: "networkidle" });
      await page.waitForTimeout(600);
    },
  },
  "console-map": {
    // Invoicing sits on the map's left with both of its neighbours, so the
    // floating card at the top right hides nothing the arcs point at.
    reach: async (page) => {
      await page.goto(`${base}/explore?mode=map`, { waitUntil: "networkidle" });
      await page.waitForTimeout(600);
      await page.getByRole("button", { name: "Expand all" }).click();
      await page.waitForTimeout(600);
      await page.locator(".map-card[data-noun='feature'] .map-title", { hasText: "Invoicing" }).first().click();
      await page.waitForTimeout(600);
    },
  },
  "console-fix": {
    // Six columns want more than 1440px; shoot wider and scale down.
    width: 1680,
    reach: async (page) => {
      await page.goto(`${base}/fix`, { waitUntil: "networkidle" });
      await page.waitForTimeout(600);
    },
  },
};

let cwebp = true;
try {
  execFileSync("cwebp", ["-version"], { stdio: "ignore" });
} catch {
  cwebp = false;
}

// The frame is the page below the console's topbar, 1440x780, the way the
// original hero was cut: the topbar says the same thing on every shot and the
// space is better spent on the lens itself.
const WIDTH = 1440;
const HEIGHT = 780;

const browser = await webkit.launch();
for (const theme of ["light", "dark"]) {
  const page = await browser.newPage({ viewport: { width: WIDTH, height: HEIGHT + 80 }, colorScheme: theme });
  for (const [name, shot] of Object.entries(shots)) {
    const width = shot.width ?? WIDTH;
    const height = Math.round((HEIGHT * width) / WIDTH);
    await page.setViewportSize({ width, height: height + 80 });
    await shot.reach(page);
    const topbar = await page.locator("header").first().boundingBox();
    const top = topbar === null ? 0 : Math.round(topbar.y + topbar.height);
    await page.setViewportSize({ width, height: height + top });
    await page.waitForTimeout(300);
    const png = resolve(outDir, `${name}-${theme}.png`);
    await page.screenshot({ path: png, clip: { x: 0, y: top, width, height } });
    if (cwebp) {
      const webp = resolve(outDir, `${name}-${theme}.webp`);
      execFileSync("cwebp", ["-quiet", "-q", "88", "-resize", String(WIDTH), String(HEIGHT), png, "-o", webp]);
      unlinkSync(png);
      console.log(`wrote ${webp}`);
    } else {
      console.log(`wrote ${png} (cwebp not found; install it with brew install webp to get the .webp)`);
    }
  }
  await page.close();
}
await browser.close();
if (!cwebp && existsSync(resolve(outDir, "console-home-light.png"))) {
  process.exitCode = 1;
}
