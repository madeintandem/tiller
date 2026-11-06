import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { serveMark } from "../src/capture/serve";
import { shoot } from "../src/capture/shoot";
import { ffEnv } from "../src/media/ffmpeg";

const ff = (bin: string, args: string[]) => execFileSync(bin, args, { env: ffEnv(), stdio: ["ignore", "pipe", "pipe"] });
const probe = (file: string, entries: string) =>
  ff("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", entries, "-of", "csv=p=0", file]).toString().trim();

// Mean absolute difference between two frames of a video, 0 to 255.
function frameDiff(file: string, a: number, b: number, dir: string): number {
  const pa = join(dir, "a.png");
  const pb = join(dir, "b.png");
  ff("ffmpeg", ["-y", "-loglevel", "error", "-ss", String(a), "-i", file, "-frames:v", "1", pa]);
  ff("ffmpeg", ["-y", "-loglevel", "error", "-ss", String(b), "-i", file, "-frames:v", "1", pb]);
  const out = execFileSync("ffmpeg", ["-i", pa, "-i", pb, "-lavfi", "blend=all_mode=difference,signalstats,metadata=print:file=-", "-f", "null", "-"], {
    env: ffEnv(),
    stdio: ["ignore", "pipe", "pipe"],
  }).toString();
  return Number(/YAVG=([\d.]+)/.exec(out)?.[1]);
}

describe("console captures", () => {
  it("serves a mark and stops cleanly, removing the temp dir", async () => {
    const s = await serveMark("s1-end");
    const res = await fetch(`${s.url}/`);
    expect(res.status).toBe(200);
    expect(existsSync(s.dir)).toBe(true);
    await s.stop();
    expect(existsSync(s.dir)).toBe(false);
    await expect(fetch(`${s.url}/`)).rejects.toThrow();
  }, 120_000);

  it("records a 2x clip of loaded content that runs the asked length", async () => {
    const out = mkdtempSync(join(tmpdir(), "cap-test-"));
    try {
      const file = await shoot({ id: "t", mark: "s1-end", path: "/", seconds: 2 }, out);
      expect(file.endsWith(".mp4")).toBe(true);
      // A 1280x720 CSS viewport at device scale 2.
      expect(probe(file, "stream=width,height,r_frame_rate")).toBe("2560,1440,30/1");
      const d = parseFloat(ff("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file]).toString());
      expect(Math.abs(d - 2)).toBeLessThan(0.05);
      // The first frame is page content, not a flat blank frame.
      const png = join(out, "f0.png");
      ff("ffmpeg", ["-y", "-loglevel", "error", "-i", file, "-frames:v", "1", png]);
      const sig = ff("ffmpeg", ["-i", png, "-vf", "signalstats,metadata=print:file=-", "-f", "null", "-"]).toString();
      const hi = Number(/YMAX=(\d+)/.exec(sig)?.[1]);
      const lo = Number(/YMIN=(\d+)/.exec(sig)?.[1]);
      expect(hi - lo).toBeGreaterThan(40);
    } finally {
      rmSync(out, { recursive: true, force: true });
    }
  }, 120_000);

  it("scrolls the page's content to a selector after a wait, and holds still before it", async () => {
    const out = mkdtempSync(join(tmpdir(), "cap-test-"));
    try {
      const file = await shoot(
        {
          id: "s",
          mark: "s1-readout",
          path: "/file/.tiller/design/as-is/orders-api.md#L1",
          seconds: 4,
          actions: [{ waitFor: "text=Behavior to preserve" }, { wait: 1.5 }, { scrollTo: "text=Behavior to preserve" }],
        },
        out,
      );
      expect(frameDiff(file, 0.1, 1.3, out)).toBeLessThan(1); // held during the wait
      expect(frameDiff(file, 0.1, 3.9, out)).toBeGreaterThan(5); // scrolled by the end
    } finally {
      rmSync(out, { recursive: true, force: true });
    }
  }, 120_000);

  it("refuses actions that need more time than the capture's seconds", async () => {
    const out = mkdtempSync(join(tmpdir(), "cap-test-"));
    try {
      await expect(shoot({ id: "x", mark: "s1-end", path: "/", seconds: 1, actions: [{ wait: 2 }] }, out)).rejects.toThrow(/seconds/);
    } finally {
      rmSync(out, { recursive: true, force: true });
    }
  }, 120_000);
});
