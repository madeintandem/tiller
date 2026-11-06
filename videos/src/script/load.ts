import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";
import { ScriptError, validateScript, type Script } from "./schema.js";

export const VIDEOS_DIR = fileURLToPath(new URL("../../", import.meta.url));

function parseScript(text: string, path: string): Script {
  let raw: unknown;
  try {
    raw = parse(text);
  } catch (e) {
    throw new ScriptError(`cannot read ${path}: ${(e as Error).message}`);
  }
  return validateScript(raw);
}

function readText(path: string): string {
  try {
    return readFileSync(path, "utf8");
  } catch (e) {
    throw new ScriptError(`cannot read ${path}: ${(e as Error).message}`);
  }
}

export function loadScript(path: string): Script {
  return parseScript(readText(path), path);
}

// Hash of the script text with CRLF, trailing whitespace, and trailing blank lines
// normalized away, so an editor's line endings never void an approval but a wording change does.
export function hashText(text: string): string {
  const normalized = text
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((l) => l.replace(/[ \t]+$/, ""))
    .join("\n")
    .replace(/\n+$/, "");
  return createHash("sha256").update(normalized).digest("hex");
}

export function scriptHash(path: string): string {
  return hashText(readText(path));
}

// Reads the file once and both parses and hashes those same bytes, so the hash a build
// checks and records is always the hash of the script it renders.
export function readScript(path: string): { script: Script; hash: string } {
  const text = readText(path);
  return { script: parseScript(text, path), hash: hashText(text) };
}

export function loadSeries(path = `${VIDEOS_DIR}series.yaml`): { tiller: string } {
  const raw = parse(readFileSync(path, "utf8")) as { tiller?: unknown } | null;
  if (!raw || (typeof raw.tiller !== "string" && typeof raw.tiller !== "number")) {
    throw new ScriptError(`${path} needs a "tiller" version`);
  }
  return { tiller: String(raw.tiller) };
}
