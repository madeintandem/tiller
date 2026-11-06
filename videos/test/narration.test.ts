import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadConfig } from "../src/config.js";
import { narrate } from "../src/narration/elevenlabs.js";
import { cacheKey } from "../src/narration/cache.js";
import { charsToWords } from "../src/narration/words.js";

const cfg = { apiKey: "k", voiceId: "v1", modelId: "m1" };
const tmp = () => mkdtempSync(join(tmpdir(), "narr-"));

function okResponse(text: string) {
  const chars = [...text];
  return new Response(
    JSON.stringify({
      audio_base64: Buffer.from("MP3DATA").toString("base64"),
      alignment: {
        characters: chars,
        character_start_times_seconds: chars.map((_, i) => i * 0.1),
        character_end_times_seconds: chars.map((_, i) => i * 0.1 + 0.1),
      },
    }),
    { status: 200 },
  );
}

beforeEach(() => {
  delete process.env.NARRATION_FAKE;
});
afterEach(() => {
  vi.unstubAllGlobals();
  delete process.env.NARRATION_FAKE;
});

describe("charsToWords", () => {
  it("groups characters, uses first start and last end, drops whitespace", () => {
    const w = charsToWords(["h", "i", " ", "y", "o", " "], [0, 0.1, 0.2, 0.3, 0.4, 0.5], [0.1, 0.2, 0.3, 0.4, 0.5, 0.6]);
    expect(w).toEqual([
      { text: "hi", start: 0, end: 0.2 },
      { text: "yo", start: 0.3, end: 0.5 },
    ]);
  });
});

describe("cacheKey", () => {
  it("hashes parts joined with NUL", () => {
    expect(cacheKey(["a", "b"])).not.toBe(cacheKey(["ab"]));
    expect(cacheKey(["a", "b"])).toMatch(/^[0-9a-f]{64}$/);
  });
});

describe("narrate (mocked fetch)", () => {
  it("caches, and regenerates only what changed", async () => {
    const dir = tmp();
    const f = vi.fn(async (_u: any, init: any) => okResponse(JSON.parse(init.body).text));
    vi.stubGlobal("fetch", f);
    const a = await narrate("hello there", cfg, dir);
    expect(a.words.map((w) => w.text)).toEqual(["hello", "there"]);
    expect(f).toHaveBeenCalledTimes(1);
    expect(String(f.mock.calls[0][0])).toContain("/v1/text-to-speech/v1/with-timestamps");
    expect(String(f.mock.calls[0][0])).toContain("output_format=mp3_44100_128");
    const b = await narrate("hello there", cfg, dir);
    expect(b).toEqual(a);
    expect(f).toHaveBeenCalledTimes(1);
    await narrate("hello there", { ...cfg, voiceId: "v2" }, dir);
    expect(f).toHaveBeenCalledTimes(2);
    await narrate("hello then", cfg, dir);
    expect(f).toHaveBeenCalledTimes(3);
  });

  it("sends pronunciation dictionary locators when configured", async () => {
    const f = vi.fn(async (_u: any, init: any) => okResponse(JSON.parse(init.body).text));
    vi.stubGlobal("fetch", f);
    await narrate("hi", { ...cfg, dictionaryId: "d1", dictionaryVersion: "ver1" }, tmp());
    expect(JSON.parse(f.mock.calls[0][1].body).pronunciation_dictionary_locators).toEqual([
      { pronunciation_dictionary_id: "d1", version_id: "ver1" },
    ]);
  });

  it("throws with status and body on a non-200", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("bad key", { status: 401 })));
    await expect(narrate("hi", cfg, tmp())).rejects.toThrow(/401.*bad key/s);
  });
});

describe("fake mode", () => {
  const probe = (p: string) =>
    parseFloat(
      execFileSync("/opt/homebrew/bin/ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", p]).toString(),
    );

  it("makes silent audio at 150 wpm with ordered words and no fetch", async () => {
    process.env.NARRATION_FAKE = "1";
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    const text = Array.from({ length: 75 }, (_, i) => `w${i}`).join(" ");
    const r = await narrate(text, cfg, tmp());
    expect(Math.abs(probe(r.audioPath) - 30)).toBeLessThan(0.1);
    expect(r.duration).toBeCloseTo(30, 5);
    expect(r.words.map((w) => w.text)).toEqual(text.split(" "));
    for (let i = 1; i < r.words.length; i++) expect(r.words[i].start).toBeGreaterThan(r.words[i - 1].start);
    expect(f).not.toHaveBeenCalled();
  });

  it("has a 1 second minimum", async () => {
    process.env.NARRATION_FAKE = "1";
    const r = await narrate("hi", cfg, tmp());
    expect(Math.abs(probe(r.audioPath) - 1)).toBeLessThan(0.1);
  });

  it("never shares a cache entry with real audio", async () => {
    const dir = tmp();
    vi.stubGlobal("fetch", vi.fn(async (_u: any, init: any) => okResponse(JSON.parse(init.body).text)));
    const real = await narrate("same text", cfg, dir);
    process.env.NARRATION_FAKE = "1";
    const fake = await narrate("same text", cfg, dir);
    expect(fake.audioPath).not.toBe(real.audioPath);
  });
});

describe("loadConfig", () => {
  const write = (yaml: string) => {
    const p = join(tmp(), "c.yaml");
    writeFileSync(p, yaml);
    return p;
  };
  it("names a missing elevenlabs.apiKey", () => {
    expect(() => loadConfig(write("elevenlabs:\n  voiceId: v\n"))).toThrow(/elevenlabs\.apiKey/);
  });
  it("loads defaults", () => {
    const c = loadConfig(write("elevenlabs:\n  apiKey: k\n  voiceId: v\n"));
    expect(c.elevenlabs.modelId).toBe("eleven_v4");
    expect(c.youtube).toEqual({ privacy: "private", categoryId: "28" });
  });
  it("does not require credentials in fake mode", () => {
    process.env.NARRATION_FAKE = "1";
    expect(() => loadConfig(write("youtube:\n  privacy: unlisted\n"))).not.toThrow();
  });
});
