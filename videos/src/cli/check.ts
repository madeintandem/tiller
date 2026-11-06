import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { diffSnapshots, readSnapshot } from "../check/diff.js";
import { VIDEOS_DIR, loadSeries } from "../script/load.js";

const STAGES = ["s1", "s2", "s3"];

function binaryVersion(): string {
  const bin = process.env.TILLER ?? "tiller";
  const text = execFileSync(bin, ["--version"], { encoding: "utf8" });
  const m = text.match(/\d+\.\d+\.\d+\S*/);
  if (!m) throw new Error(`cannot read a version from "${text.trim()}"`);
  return m[0];
}

function main(): number {
  const pinned = loadSeries().tiller;
  const actual = binaryVersion();
  if (actual !== pinned) {
    console.error(`tiller version mismatch: series.yaml pins ${pinned}, the binary is ${actual}`);
    return 1;
  }
  const tmp = mkdtempSync(join(tmpdir(), "videos-check-"));
  try {
    const run = spawnSync("bash", [`${VIDEOS_DIR}fixture/run.sh`, "--out", tmp], { stdio: ["ignore", "ignore", "inherit"] });
    if (run.status !== 0) {
      console.error(`fixture/run.sh failed (exit ${run.status})`);
      return 1;
    }
    const diffs = STAGES.flatMap((s) =>
      diffSnapshots(readSnapshot(s), readSnapshot(s, `${tmp}/snapshots`)).map((d) => `${s}: ${d}`),
    );
    if (diffs.length > 0) {
      console.error(`snapshots differ from a fresh tiller ${pinned} run:\n${diffs.join("\n")}`);
      return 1;
    }
    console.log(`snapshots match tiller ${pinned}`);
    return 0;
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

process.exit(main());
