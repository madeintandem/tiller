import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { serveMark } from "../src/capture/serve";
import { shoot } from "../src/capture/shoot";
import { ffEnv } from "../src/narration/elevenlabs";

describe("console captures", () => {
  it("serves a mark and stops cleanly", async () => {
    const s = await serveMark("s1-end");
    const res = await fetch(`${s.url}/`);
    expect(res.status).toBe(200);
    await s.stop();
    await expect(fetch(`${s.url}/`)).rejects.toThrow();
  }, 120_000);

  it("records a webm of about the asked length", async () => {
    const out = mkdtempSync(join(tmpdir(), "cap-test-"));
    try {
      const file = await shoot({ id: "t", mark: "s1-end", path: "/", seconds: 2 }, out);
      expect(file.endsWith(".webm")).toBe(true);
      expect(existsSync(file)).toBe(true);
      const d = parseFloat(
        execFileSync("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file], {
          env: ffEnv(),
        }).toString(),
      );
      expect(d).toBeGreaterThan(1.5);
      expect(d).toBeLessThan(3);
    } finally {
      rmSync(out, { recursive: true, force: true });
    }
  }, 120_000);
});
