import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Word } from "./words.js";

export function cacheKey(parts: string[]): string {
  return createHash("sha256").update(parts.join("\0")).digest("hex");
}

export type Cached = { audioPath: string; words: Word[]; duration: number };

export function cachePaths(cacheDir: string, key: string) {
  return { audio: join(cacheDir, `${key}.mp3`), meta: join(cacheDir, `${key}.json`) };
}

export function readCache(cacheDir: string, key: string): Cached | undefined {
  const { audio, meta } = cachePaths(cacheDir, key);
  if (!existsSync(audio) || !existsSync(meta)) return undefined;
  try {
    const m = JSON.parse(readFileSync(meta, "utf8"));
    if (!Array.isArray(m.words) || typeof m.duration !== "number") return undefined;
    return { audioPath: audio, words: m.words, duration: m.duration };
  } catch {
    return undefined;
  }
}

export function writeCache(cacheDir: string, key: string, mp3: Buffer, words: Word[], duration: number): Cached {
  mkdirSync(cacheDir, { recursive: true });
  const { audio, meta } = cachePaths(cacheDir, key);
  // Temp files in the same dir, then rename; meta last, so a hit implies both are whole.
  const tag = `.tmp-${process.pid}-${Math.random().toString(36).slice(2)}`;
  writeFileSync(audio + tag, mp3);
  writeFileSync(meta + tag, JSON.stringify({ words, duration }));
  renameSync(audio + tag, audio);
  renameSync(meta + tag, meta);
  return { audioPath: audio, words, duration };
}

export function readJson<T>(cacheDir: string, key: string): T | undefined {
  const p = join(cacheDir, `${key}.json`);
  if (!existsSync(p)) return undefined;
  try {
    return JSON.parse(readFileSync(p, "utf8")) as T;
  } catch {
    return undefined;
  }
}

export function writeJson(cacheDir: string, key: string, value: unknown): void {
  mkdirSync(cacheDir, { recursive: true });
  const p = join(cacheDir, `${key}.json`);
  const tmp = `${p}.tmp-${process.pid}-${Math.random().toString(36).slice(2)}`;
  writeFileSync(tmp, JSON.stringify(value));
  renameSync(tmp, p);
}
