import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { chromium, type Browser, type Page } from "playwright";
import { ffmpeg } from "../media/ffmpeg.js";
import type { Capture } from "../script/schema.js";
import { serveMark } from "./serve.js";

// The console is shot in its dark theme, which it takes from the browser's color scheme.
// A 1280x720 CSS viewport at device scale 2: the console's text reads at laptop size once
// the frame shrinks the picture, and every frame is a full-resolution screenshot.
const VIEWPORT = { width: 1280, height: 720 };
const SCALE = 2;
const FPS = 30;
const SCROLL_SECONDS = 1.2;
const SETTLE_MS = 400;

const ease = (x: number) => (x < 0.5 ? 4 * x * x * x : 1 - (-2 * x + 2) ** 3 / 2);

type Pos = { x: number; y: number };

// Finds the element that scrolls the page's content (the nearest scrolling ancestor of
// `selector`, or the largest vertically scrolling element, or the document), tags it, and
// returns where it is and where it should go. For `selector`: its top a little below the
// container's top, and sideways only as far as needed to bring it fully into view. For
// `by`: that many pixels down.
async function scrollPlan(page: Page, target: { selector?: string; by?: number }): Promise<{ from: Pos; to: Pos }> {
  await page.evaluate(() => document.querySelectorAll("[data-capture-target]").forEach((e) => e.removeAttribute("data-capture-target")));
  if (target.selector) {
    const loc = page.locator(target.selector).first();
    if ((await loc.count()) === 0) throw new Error(`scrollTo: nothing matches ${target.selector}`);
    await loc.evaluate((e) => e.setAttribute("data-capture-target", ""));
  }
  return page.evaluate(`(${PLAN_SOURCE})(document.querySelector("[data-capture-target]"), ${Number(target.by ?? 0)})`);
}

// Plain JavaScript run in the page. It is a string on purpose: tsx compiles a function
// argument with keepNames, which wraps its inner helpers in a `__name` the page lacks.
const PLAN_SOURCE = `function (el, by) {
  var PAD = 24;
  function scrollsY(e) { return e.scrollHeight > e.clientHeight + 1 && ["auto", "scroll"].indexOf(getComputedStyle(e).overflowY) >= 0; }
  function scrollsX(e) { return e.scrollWidth > e.clientWidth + 1 && ["auto", "scroll"].indexOf(getComputedStyle(e).overflowX) >= 0; }
  function clamp(v, max) { return Math.max(0, Math.min(max, v)); }
  var box = null;
  if (el) {
    for (var p = el.parentElement; p && !box; p = p.parentElement) if (scrollsY(p) || scrollsX(p)) box = p;
  } else {
    var all = Array.prototype.filter.call(document.querySelectorAll("*"), scrollsY);
    all.sort(function (a, b) { return b.clientWidth * b.clientHeight - a.clientWidth * a.clientHeight; });
    box = all[0] || null;
  }
  var c = box || document.scrollingElement || document.documentElement;
  document.querySelectorAll("[data-capture-scroller]").forEach(function (e) { e.removeAttribute("data-capture-scroller"); });
  c.setAttribute("data-capture-scroller", "");
  var maxY = c.scrollHeight - c.clientHeight;
  var maxX = c.scrollWidth - c.clientWidth;
  var from = { x: c.scrollLeft, y: c.scrollTop };
  if (!el) return { from: from, to: { x: from.x, y: clamp(from.y + by, maxY) } };
  var view = box ? box.getBoundingClientRect() : { top: 0, left: 0, right: window.innerWidth };
  var r = el.getBoundingClientRect();
  var x = from.x;
  if (r.right > view.right - PAD) x += r.right - (view.right - PAD);
  else if (r.left < view.left + PAD) x -= view.left + PAD - r.left;
  var y = scrollsY(c) || !box ? from.y + r.top - view.top - PAD : from.y;
  return { from: from, to: { x: clamp(x, maxX), y: clamp(y, maxY) } };
}`;

// Records the console as a sequence of 2x screenshots on a virtual clock and returns an
// H.264 .mp4 of exactly `seconds` at 30 fps. Frame 0 is the loaded page. Actions run in
// order: `wait` holds the picture, `scroll`/`scrollTo` glide over SCROLL_SECONDS, `click`
// cuts to the page once it settles; the last picture holds to the end.
export async function shoot(capture: Capture, outDir: string): Promise<string> {
  mkdirSync(outDir, { recursive: true });
  const raw = join(outDir, `.raw-${capture.id}`);
  rmSync(raw, { recursive: true, force: true });
  mkdirSync(raw, { recursive: true });
  const total = Math.round(capture.seconds * FPS);
  const server = await serveMark(capture.mark);
  let browser: Browser | undefined;
  try {
    browser = await chromium.launch();
    const context = await browser.newContext({ viewport: VIEWPORT, deviceScaleFactor: SCALE, colorScheme: "dark" });
    const page = await context.newPage();
    await page.goto(`${server.url}${capture.path}`);
    await page.locator("main, header").first().waitFor({ state: "visible" });
    await page.waitForTimeout(SETTLE_MS);

    // Each shot holds until the next one; `frame` is where the next shot lands.
    const shots: { file: string; at: number }[] = [];
    let frame = 0;
    const snap = async () => {
      const file = `f${String(shots.length).padStart(5, "0")}.png`;
      await page.screenshot({ path: join(raw, file) });
      shots.push({ file, at: frame });
    };
    const advance = (frames: number) => {
      frame += frames;
      if (frame > total) {
        throw new Error(`capture ${capture.id}: its actions need ${(frame / FPS).toFixed(2)} s, more than its ${capture.seconds} seconds`);
      }
    };
    await snap();
    for (const a of capture.actions ?? []) {
      if (a.waitFor) await page.locator(a.waitFor).first().waitFor();
      if (a.click) {
        await page.locator(a.click).first().click();
        await page.waitForTimeout(SETTLE_MS);
        await snap();
      }
      if (a.scroll !== undefined || a.scrollTo !== undefined) {
        const { from, to } = await scrollPlan(page, a.scrollTo !== undefined ? { selector: a.scrollTo } : { by: a.scroll });
        const steps = Math.round(SCROLL_SECONDS * FPS);
        for (let i = 1; i <= steps; i++) {
          advance(1);
          const k = ease(i / steps);
          const at = { x: from.x + (to.x - from.x) * k, y: from.y + (to.y - from.y) * k };
          await page.evaluate((at) => {
            const c = document.querySelector("[data-capture-scroller]");
            if (c) c.scrollTo({ left: at.x, top: at.y, behavior: "instant" });
          }, at);
          await snap();
        }
      }
      if (a.wait !== undefined) advance(Math.round(a.wait * FPS));
    }
    await context.close();

    // The concat demuxer shows each shot for its duration; the last one holds to `seconds`.
    const list = shots.map((s, i) => `file '${s.file}'\nduration ${(((shots[i + 1]?.at ?? total) - s.at) / FPS).toFixed(6)}`);
    writeFileSync(join(raw, "list.txt"), `${list.join("\n")}\nfile '${shots[shots.length - 1].file}'\n`);
    const out = join(outDir, `${capture.id}.mp4`);
    await ffmpeg([
      "-f", "concat", "-safe", "0", "-i", join(raw, "list.txt"),
      "-vf", `fps=${FPS},format=yuv420p`, "-frames:v", String(total),
      "-c:v", "libx264", "-preset", "medium", "-crf", "14", "-tune", "stillimage", "-an", "-movflags", "+faststart", out,
    ]);
    return out;
  } finally {
    await browser?.close();
    rmSync(raw, { recursive: true, force: true });
    await server.stop();
  }
}
