import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Config } from "../config.js";
import { cacheKey, readCache, readJson, writeCache, writeJson } from "./cache.js";
import { charsToWords, spreadWords, type Word } from "./words.js";

const API = "https://api.elevenlabs.io";
const WPM = 150;

const isFake = () => process.env.NARRATION_FAKE === "1";
import { ffEnv } from "../media/ffmpeg.js";
export { ffEnv };

async function failIfBad(res: Response, what: string): Promise<void> {
  if (res.status !== 200) throw new Error(`ElevenLabs ${what} failed: ${res.status} ${await res.text()}`);
}

function fakeNarrate(text: string, cacheDir: string, key: string) {
  const n = text.split(/\s+/).filter(Boolean).length;
  const duration = Math.max(1, (n / WPM) * 60);
  const dir = mkdtempSync(join(tmpdir(), "fake-narration-"));
  try {
    const out = join(dir, "a.mp3");
    execFileSync(
      "ffmpeg",
      ["-y", "-loglevel", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", String(duration), "-c:a", "libmp3lame", out],
      { env: ffEnv() },
    );
    return writeCache(cacheDir, key, readFileSync(out), spreadWords(text, duration), duration);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

export async function narrate(text: string, cfg: Config["elevenlabs"], cacheDir: string) {
  const fake = isFake();
  const key = cacheKey([
    ...(fake ? ["fake"] : []),
    cfg.voiceId,
    cfg.modelId,
    cfg.dictionaryId ?? "",
    cfg.dictionaryVersion ?? "",
    text,
  ]);
  const hit = readCache(cacheDir, key);
  if (hit) return hit;
  if (fake) return fakeNarrate(text, cacheDir, key);

  const body: Record<string, unknown> = { text, model_id: cfg.modelId };
  if (cfg.dictionaryId) {
    body.pronunciation_dictionary_locators = [
      { pronunciation_dictionary_id: cfg.dictionaryId, ...(cfg.dictionaryVersion ? { version_id: cfg.dictionaryVersion } : {}) },
    ];
  }
  const url = `${API}/v1/text-to-speech/${encodeURIComponent(cfg.voiceId)}/with-timestamps?output_format=mp3_44100_128`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "xi-api-key": cfg.apiKey, "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  await failIfBad(res, "text-to-speech");
  const json: any = await res.json();
  const a = json.alignment;
  if (!a) throw new Error("ElevenLabs text-to-speech response had no alignment");
  const words = charsToWords(a.characters, a.character_start_times_seconds, a.character_end_times_seconds);
  const ends: number[] = a.character_end_times_seconds;
  const duration = ends.length ? ends[ends.length - 1] : 0;
  return writeCache(cacheDir, key, Buffer.from(json.audio_base64, "base64"), words, duration);
}

function probeDuration(audioPath: string): number {
  const out = execFileSync(
    "ffprobe",
    ["-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", audioPath],
    { env: ffEnv() },
  );
  return parseFloat(out.toString());
}

export async function align(audioPath: string, transcript: string, apiKey: string, cacheDir: string): Promise<Word[]> {
  const fake = isFake();
  const audio = readFileSync(audioPath);
  const key = cacheKey([
    ...(fake ? ["fake"] : []),
    "align",
    createHash("sha256").update(audio).digest("hex"),
    transcript,
  ]);
  const hit = readJson<Word[]>(cacheDir, key);
  if (hit) return hit;
  let words: Word[];
  if (fake) {
    words = spreadWords(transcript, probeDuration(audioPath));
  } else {
    const form = new FormData();
    form.append("file", new Blob([audio]), "audio");
    form.append("text", transcript);
    const res = await fetch(`${API}/v1/forced-alignment`, { method: "POST", headers: { "xi-api-key": apiKey }, body: form });
    await failIfBad(res, "forced-alignment");
    const json: any = await res.json();
    words = (json.words as any[]).map((w) => ({ text: w.text, start: w.start, end: w.end }));
  }
  writeJson(cacheDir, key, words);
  return words;
}
