import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
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
  const m = JSON.parse(readFileSync(meta, "utf8"));
  return { audioPath: audio, words: m.words, duration: m.duration };
}

export function writeCache(cacheDir: string, key: string, mp3: Buffer, words: Word[], duration: number): Cached {
  mkdirSync(cacheDir, { recursive: true });
  const { audio, meta } = cachePaths(cacheDir, key);
  writeFileSync(audio, mp3);
  writeFileSync(meta, JSON.stringify({ words, duration }));
  return { audioPath: audio, words, duration };
}

export function readJson<T>(cacheDir: string, key: string): T | undefined {
  const p = join(cacheDir, `${key}.json`);
  return existsSync(p) ? (JSON.parse(readFileSync(p, "utf8")) as T) : undefined;
}

export function writeJson(cacheDir: string, key: string, value: unknown): void {
  mkdirSync(cacheDir, { recursive: true });
  writeFileSync(join(cacheDir, `${key}.json`), JSON.stringify(value));
}
