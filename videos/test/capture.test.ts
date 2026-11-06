import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { serveMark } from "../src/capture/serve";
import { shoot } from "../src/capture/shoot";
import { ffEnv } from "../src/narration/elevenlabs";

const ff = (bin: string, args: string[]) => execFileSync(bin, args, { env: ffEnv(), stdio: ["ignore", "pipe", "pipe"] });

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

  it("records a webm that starts on loaded content and runs the asked length", async () => {
    const out = mkdtempSync(join(tmpdir(), "cap-test-"));
    try {
      const file = await shoot({ id: "t", mark: "s1-end", path: "/", seconds: 2 }, out);
      expect(file.endsWith(".webm")).toBe(true);
      const d = parseFloat(
        ff("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file]).toString(),
      );
      expect(d).toBeGreaterThanOrEqual(1.75);
      expect(d).toBeLessThanOrEqual(2.25);
      // The first frame is page content, not a flat blank frame.
      const png = join(out, "f0.png");
      ff("ffmpeg", ["-y", "-loglevel", "error", "-i", file, "-frames:v", "1", png]);
      const sig = ff("ffmpeg", [
        "-i", png,
        "-vf", "signalstats,metadata=print:file=-",
        "-f", "null", "-",
      ]).toString();
      const hi = Number(/YMAX=(\d+)/.exec(sig)?.[1]);
      const lo = Number(/YMIN=(\d+)/.exec(sig)?.[1]);
      expect(hi - lo).toBeGreaterThan(40);
    } finally {
      rmSync(out, { recursive: true, force: true });
    }
  }, 120_000);
});
