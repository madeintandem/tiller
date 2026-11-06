import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { VIDEOS_DIR } from "./load.js";

export const APPROVALS_PATH = `${VIDEOS_DIR}approvals.json`;

function read(path: string): Record<string, string> {
  if (!existsSync(path)) return {};
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, string>;
}

export function isApproved(id: string, hash: string, approvalsPath = APPROVALS_PATH): boolean {
  return read(approvalsPath)[id] === hash;
}

export function approve(id: string, hash: string, approvalsPath = APPROVALS_PATH): void {
  const all = read(approvalsPath);
  all[id] = hash;
  const sorted = Object.fromEntries(Object.entries(all).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
  writeFileSync(approvalsPath, JSON.stringify(sorted, null, 2) + "\n");
}
