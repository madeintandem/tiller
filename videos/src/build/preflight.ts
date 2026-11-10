import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { tillerVersion } from "../check/run.js";
import { loadConfig } from "../config.js";
import { ffEnv } from "../media/ffmpeg.js";
import { isApproved } from "../script/approval.js";
import { loadSeries, readScript } from "../script/load.js";
import type { Script } from "../script/schema.js";
import { introPath, layoutFor, scriptPath, type Layout } from "./layout.js";
import { checkScript } from "./validate.js";

// On success, the script and its hash come from one read of the file: the build renders and
// records exactly what was checked against the approval.
export type PreflightResult =
  | { ok: true; script: Script; hash: string; warnings: string[] }
  | { ok: false; problems: string[]; warnings: string[] };

const message = (e: unknown) => (e as Error).message;

// Everything a build needs that can be checked locally and for free, before any
// paid API call or long render. Every failure is reported, in a fixed order.
export function preflight(id: string, layout: Layout = layoutFor()): PreflightResult {
  const problems: string[] = [];
  const warnings: string[] = [];
  let read: { script: Script; hash: string } | undefined;

  const path = scriptPath(layout, id);
  if (!existsSync(path)) {
    problems.push(`no script at ${path}`);
  } else {
    try {
      const r = readScript(path);
      if (r.script.id !== id) problems.push(`${path} has id "${r.script.id}", expected "${id}"`);
      else read = r;
    } catch (e) {
      problems.push(`script ${id} is invalid: ${message(e)}`);
    }
    if (read && !isApproved(id, read.hash, layout.approvals)) {
      problems.push(`script ${id} is not approved at its current content (run: npm run approve -- ${id})`);
    }
    if (read) {
      const checked = checkScript(read.script, layout);
      problems.push(...checked.problems);
      warnings.push(...checked.warnings);
    }
  }

  try {
    loadConfig(layout.config);
  } catch (e) {
    problems.push(message(e));
  }

  try {
    const pinned = loadSeries(layout.series).tiller;
    const actual = tillerVersion(layout.tiller);
    if (actual !== pinned) problems.push(`tiller version mismatch: series.yaml pins ${pinned}, the binary is ${actual}`);
  } catch (e) {
    problems.push(`cannot check the tiller version: ${message(e)}`);
  }

  // Only a script with an intro block needs the filmed clip.
  if (read?.script.intro) {
    const intro = introPath(layout, id);
    if (!existsSync(intro)) problems.push(`no intro clip at ${intro} (record JC's live intro first)`);
  }
  if (!existsSync(layout.music)) problems.push(`no music at ${layout.music}`);

  try {
    execFileSync("ffmpeg", ["-version"], { env: ffEnv(), stdio: "ignore" });
  } catch {
    problems.push("ffmpeg not found; brew install ffmpeg");
  }

  if (problems.length > 0 || !read) return { ok: false, problems, warnings };
  return { ok: true, script: read.script, hash: read.hash, warnings };
}
