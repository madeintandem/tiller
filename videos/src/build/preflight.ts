import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { tillerVersion } from "../check/run.js";
import { loadConfig } from "../config.js";
import { ffEnv } from "../media/ffmpeg.js";
import { isApproved } from "../script/approval.js";
import { loadScript, loadSeries, scriptHash } from "../script/load.js";
import { introPath, layoutFor, scriptPath, type Layout } from "./layout.js";

export type PreflightResult = { ok: true } | { ok: false; problems: string[] };

const message = (e: unknown) => (e as Error).message;

// Everything a build needs that can be checked locally and for free, before any
// paid API call or long render. Every failure is reported, in a fixed order.
export function preflight(id: string, layout: Layout = layoutFor()): PreflightResult {
  const problems: string[] = [];

  const path = scriptPath(layout, id);
  if (!existsSync(path)) {
    problems.push(`no script at ${path}`);
  } else {
    let valid = false;
    try {
      const script = loadScript(path);
      if (script.id !== id) problems.push(`${path} has id "${script.id}", expected "${id}"`);
      else valid = true;
    } catch (e) {
      problems.push(`script ${id} is invalid: ${message(e)}`);
    }
    if (valid && !isApproved(id, scriptHash(path), layout.approvals)) {
      problems.push(`script ${id} is not approved at its current content (run: npm run approve -- ${id})`);
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

  const intro = introPath(layout, id);
  if (!existsSync(intro)) problems.push(`no intro clip at ${intro} (record JC's live intro first)`);
  if (!existsSync(layout.music)) problems.push(`no music at ${layout.music}`);

  try {
    execFileSync("ffmpeg", ["-version"], { env: ffEnv(), stdio: "ignore" });
  } catch {
    problems.push("ffmpeg not found; brew install ffmpeg");
  }

  return problems.length === 0 ? { ok: true } : { ok: false, problems };
}
