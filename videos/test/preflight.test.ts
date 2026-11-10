import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { layoutFor, type Layout } from "../src/build/layout.js";
import { preflight } from "../src/build/preflight.js";
import { scriptHash } from "../src/script/load.js";
import { approve } from "../src/script/approval.js";

const FIXTURE = "test/fixtures/v-test.yaml";
let root: string;
let layout: Layout;
let fetchSpy: ReturnType<typeof vi.fn>;

// A videos/ tree with everything a build needs, and a fake tiller printing the pinned version.
function prepare(): void {
  mkdirSync(join(root, "scripts"), { recursive: true });
  copyFileSync(FIXTURE, join(root, "scripts", "v-test.yaml"));
  writeFileSync(join(root, "approvals.json"), "{}\n");
  approve("v-test", scriptHash(join(root, "scripts", "v-test.yaml")), join(root, "approvals.json"));
  writeFileSync(join(root, "series.yaml"), "tiller: 0.22.0\n");
  mkdirSync(join(root, ".local", "media", "intros"), { recursive: true });
  writeFileSync(join(root, ".local", "config.yaml"), "elevenlabs:\n  apiKey: k\n  voiceId: v\n");
  writeFileSync(join(root, ".local", "media", "intros", "v-test.mov"), "mov");
  writeFileSync(join(root, ".local", "media", "music.mp3"), "mp3");
}

beforeEach(() => {
  delete process.env.NARRATION_FAKE;
  root = mkdtempSync(join(tmpdir(), "preflight-"));
  const tiller = join(root, "fake-tiller");
  writeFileSync(tiller, "#!/bin/sh\necho 'tiller version 0.22.0'\n");
  chmodSync(tiller, 0o755);
  // Step IDs and capture marks are checked against the committed snapshots and marks.
  const real = layoutFor();
  layout = { ...layoutFor(root, { TILLER: tiller }), snapshots: real.snapshots, marks: real.marks };
  fetchSpy = vi.fn(() => {
    throw new Error("preflight must not touch the network");
  });
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => {
  vi.unstubAllGlobals();
  rmSync(root, { recursive: true, force: true });
});

describe("preflight", () => {
  it("passes a fully prepared layout", () => {
    prepare();
    const r = preflight("v-test", layout);
    expect(r).toMatchObject({ ok: true, warnings: [], hash: scriptHash(join(root, "scripts", "v-test.yaml")) });
    if (r.ok) expect(r.script.id).toBe("v-test");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("hands on the script and hash it read, not a later edit", () => {
    prepare();
    const path = join(root, "scripts", "v-test.yaml");
    const before = scriptHash(path);
    const r = preflight("v-test", layout);
    writeFileSync(path, readFileSync(path, "utf8").replace("planning", "shipping"));
    expect(r.ok && r.hash).toBe(before);
    expect(r.ok && r.script.beats[0].say).toContain("planning");
  });

  it("reports a step missing from the snapshots and a missing capture mark before anything paid", () => {
    prepare();
    const path = join(root, "scripts", "v-test.yaml");
    writeFileSync(path, readFileSync(path, "utf8").replace("to: s1-join", "to: s1-nope").replace("mark: s1-end", "mark: s1-gone"));
    approve("v-test", scriptHash(path), join(root, "approvals.json"));
    const r = preflight("v-test", layout);
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.problems).toHaveLength(2);
      expect(r.problems.join("\n")).toMatch(/s1-nope/);
      expect(r.problems.join("\n")).toMatch(/s1-gone/);
    }
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("fails an unapproved script without calling fetch", () => {
    prepare();
    writeFileSync(join(root, "approvals.json"), "{}\n");
    const r = preflight("v-test", layout);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.problems).toEqual([expect.stringMatching(/not approved/)]);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("reports a missing intro and a missing config in one list", () => {
    prepare();
    rmSync(join(root, ".local", "config.yaml"));
    rmSync(join(root, ".local", "media", "intros", "v-test.mov"));
    const r = preflight("v-test", layout);
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.problems).toHaveLength(2);
      expect(r.problems[0]).toMatch(/config/);
      expect(r.problems[1]).toMatch(/intros\/v-test\.mov/);
    }
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("needs no intro clip for a script with no intro block", () => {
    prepare();
    const path = join(root, "scripts", "v-test.yaml");
    writeFileSync(path, readFileSync(path, "utf8").replace(/intro:\n  transcript: .*\n/, ""));
    approve("v-test", scriptHash(path), join(root, "approvals.json"));
    rmSync(join(root, ".local", "media", "intros", "v-test.mov"));
    expect(preflight("v-test", layout).ok).toBe(true);
  });

  it("reports a missing script, a tiller version off the pin, and missing music", () => {
    prepare();
    rmSync(join(root, "scripts", "v-test.yaml"));
    writeFileSync(join(root, "series.yaml"), "tiller: 9.9.9\n");
    rmSync(join(root, ".local", "media", "music.mp3"));
    const r = preflight("v-test", layout);
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.problems).toHaveLength(3);
      expect(r.problems[0]).toMatch(/scripts\/v-test\.yaml/);
      expect(r.problems[1]).toMatch(/pins 9\.9\.9.*0\.22\.0/);
      expect(r.problems[2]).toMatch(/music\.mp3/);
    }
  });
});
