import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";
import { ScriptError, validateScript, type Script } from "./schema.js";

export const VIDEOS_DIR = fileURLToPath(new URL("../../", import.meta.url));

export function loadScript(path: string): Script {
  let raw: unknown;
  try {
    raw = parse(readFileSync(path, "utf8"));
  } catch (e) {
    if (e instanceof ScriptError) throw e;
    throw new ScriptError(`cannot read ${path}: ${(e as Error).message}`);
  }
  return validateScript(raw);
}

// Hash of the script text with CRLF and trailing whitespace normalized away,
// so an editor's line endings never void an approval but a wording change does.
export function scriptHash(path: string): string {
  const text = readFileSync(path, "utf8")
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((l) => l.replace(/[ \t]+$/, ""))
    .join("\n");
  return createHash("sha256").update(text).digest("hex");
}

export function loadSeries(path = `${VIDEOS_DIR}series.yaml`): { tiller: string } {
  const raw = parse(readFileSync(path, "utf8")) as { tiller?: unknown } | null;
  if (!raw || (typeof raw.tiller !== "string" && typeof raw.tiller !== "number")) {
    throw new ScriptError(`${path} needs a "tiller" version`);
  }
  return { tiller: String(raw.tiller) };
}
