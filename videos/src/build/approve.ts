import { approve } from "../script/approval.js";
import { readScript } from "../script/load.js";
import { scriptPath, type Layout } from "./layout.js";
import { checkScript } from "./validate.js";

/**
 * Record the script's hash in the approvals file, but only for a script that can build:
 * it validates, its internal id matches its file name, and every step and mark it names
 * exists. Throws with every problem otherwise. Returns the hash and any warnings.
 */
export function approveScript(id: string, layout: Layout): { hash: string; warnings: string[] } {
  const path = scriptPath(layout, id);
  const { script, hash } = readScript(path);
  const problems: string[] = [];
  if (script.id !== id) problems.push(`${path} has id "${script.id}", expected "${id}" (the file name)`);
  const checked = checkScript(script, layout);
  problems.push(...checked.problems);
  if (problems.length) throw new Error(`cannot approve ${id}:\n${problems.map((p) => `  - ${p}`).join("\n")}`);
  approve(id, hash, layout.approvals);
  return { hash, warnings: checked.warnings };
}
