import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { VIDEOS_DIR, loadSeries } from "../script/load.js";
import { diffSnapshots, readSnapshot } from "./diff.js";

const STAGES = ["s1", "s2", "s3"];

export function tillerVersion(bin = process.env.TILLER || "tiller"): string {
  const text = execFileSync(bin, ["--version"], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  const m = text.match(/\d+\.\d+\.\d+\S*/);
  if (!m) throw new Error(`cannot read a version from "${text.trim()}"`);
  return m[0];
}

export type CheckResult = { ok: boolean; message: string };

// Rebuild the fixture with the pinned binary into a temp dir and diff it against the committed snapshots.
export function checkSnapshots(): CheckResult {
  const pinned = loadSeries().tiller;
  const actual = tillerVersion();
  if (actual !== pinned) {
    return { ok: false, message: `tiller version mismatch: series.yaml pins ${pinned}, the binary is ${actual}` };
  }
  const tmp = mkdtempSync(join(tmpdir(), "videos-check-"));
  try {
    const run = spawnSync("bash", [`${VIDEOS_DIR}fixture/run.sh`, "--out", tmp], { stdio: ["ignore", "ignore", "inherit"] });
    if (run.status !== 0) return { ok: false, message: `fixture/run.sh failed (exit ${run.status})` };
    const diffs = STAGES.flatMap((s) =>
      diffSnapshots(readSnapshot(s), readSnapshot(s, `${tmp}/snapshots`)).map((d) => `${s}: ${d}`),
    );
    if (diffs.length > 0) {
      return { ok: false, message: `snapshots differ from a fresh tiller ${pinned} run:\n${diffs.join("\n")}` };
    }
    return { ok: true, message: `snapshots match tiller ${pinned}` };
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}
