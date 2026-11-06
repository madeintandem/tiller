import { describe, expect, it } from "vitest";
import { metadata } from "../src/build/meta.js";
import { loadScript } from "../src/script/load.js";

describe("metadata", () => {
  it("joins description, chapters, and the disclosure with blank lines", () => {
    const script = loadScript("test/fixtures/v-test.yaml");
    const m = metadata(script, ["0:00 Getting started", "0:22 The console", "0:40 Wrapping up"], 61.23456);
    expect(m.description).toBe(
      `${script.description}\n\n0:00 Getting started\n0:22 The console\n0:40 Wrapping up\n\n` +
        "The narration is my words, in my voice, performed by AI.",
    );
    expect(m).toMatchObject({ title: script.title, tags: script.tags, durationSeconds: 61.235 });
    expect(m.chapters).toHaveLength(3);
  });
});
