// Render the 1280x640 link preview from social-preview.html.
//
//   node shoot-social.mjs [out]     default out: ../public/social-preview.png
//
// The homepage points og:image at it. Upload the same file by hand as the
// GitHub repository's social preview (Settings, General, Social preview).
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { webkit } from "playwright";

const [out = resolve(import.meta.dirname, "../public/social-preview.png")] = process.argv.slice(2);
const browser = await webkit.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 640 }, deviceScaleFactor: 1 });
await page.goto(pathToFileURL(resolve(import.meta.dirname, "social-preview.html")).href, { waitUntil: "load" });
await page.evaluate(() => document.fonts.ready);
await page.evaluate(() => Promise.all([...document.images].map((img) => img.decode())));
await page.waitForTimeout(200);
await page.screenshot({ path: out, clip: { x: 0, y: 0, width: 1280, height: 640 } });
await browser.close();
console.log(`wrote ${out}`);
