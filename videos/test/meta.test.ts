import { describe, expect, it } from "vitest";
import { metadata, voiceProvenance } from "../src/build/meta.js";
import { loadScript } from "../src/script/load.js";

describe("metadata", () => {
  it("joins description, chapters, and the disclosure with blank lines", () => {
    const script = loadScript("test/fixtures/v-test.yaml");
    const m = metadata(script, ["0:00 Getting started", "0:22 The console", "0:40 Wrapping up"], 61.23456, {
      fake: true,
      scriptHash: "abc123",
      voiceId: "fake",
    });
    expect(m.description).toBe(
      `${script.description}\n\n0:00 Getting started\n0:22 The console\n0:40 Wrapping up\n\n` +
        "The narration is my words, in my voice, performed by AI.",
    );
    expect(m).toMatchObject({ title: script.title, tags: script.tags, durationSeconds: 61.235, fake: true, scriptHash: "abc123", voiceId: "fake" });
    expect(m.chapters).toHaveLength(3);
  });

  it("names the voice: the configured voice ID, or fake for a NARRATION_FAKE build", () => {
    expect(voiceProvenance(false, "abc-voice")).toBe("abc-voice");
    expect(voiceProvenance(true, "abc-voice")).toBe("fake");
  });
});
