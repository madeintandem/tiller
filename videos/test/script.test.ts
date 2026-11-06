import { describe, expect, it, beforeEach, afterEach } from "vitest";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { loadScript, scriptHash } from "../src/script/load.js";
import { ScriptError } from "../src/script/schema.js";
import { approve, isApproved } from "../src/script/approval.js";

const FIXTURE = "test/fixtures/v-test.yaml";
let dir: string;

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "videos-test-"));
});
afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

function variant(edit: (s: string) => string): string {
  const p = join(dir, "v.yaml");
  writeFileSync(p, edit(readFileSync(FIXTURE, "utf8")));
  return p;
}

describe("loadScript", () => {
  it("loads a valid script", () => {
    const s = loadScript(FIXTURE);
    expect(s.beats.length).toBe(3);
    expect(s.beats[0].show.kind).toBe("card");
    expect(s.captures.length).toBe(1);
  });

  it("defaults captures to an empty array", () => {
    const p = variant((s) => s.replace(/captures:[\s\S]*?beats:/, "beats:"));
    expect(loadScript(p).captures).toEqual([]);
  });

  it.each([
    ["a command", "Run tiller sync to continue."],
    ["a flag", "Pass the --force option."],
    ["a node id", "Look at F-abc123 now."],
    ["a path", "Open the .tiller/ folder."],
  ])("rejects a beat whose say contains %s", (_n, say) => {
    const p = variant((s) => s.replace("The agent reads the board and picks the next piece of work.", say));
    expect(() => loadScript(p)).toThrow(ScriptError);
    expect(() => loadScript(p)).toThrow(/b2/);
  });

  it("rejects duplicate beat ids", () => {
    const p = variant((s) => s.replace("id: b3", "id: b1"));
    expect(() => loadScript(p)).toThrow(ScriptError);
    expect(() => loadScript(p)).toThrow(/b1/);
  });

  it("rejects a chapter on a beat that is not a card", () => {
    const p = variant((s) => s.replace("  - id: b2\n", "  - id: b2\n    chapter: Terminal\n"));
    expect(() => loadScript(p)).toThrow(ScriptError);
    expect(() => loadScript(p)).toThrow(/b2/);
  });

  it("rejects a term show missing from/to", () => {
    const p = variant((s) => s.replace("      from: snapshots/start.txt\n", ""));
    expect(() => loadScript(p)).toThrow(ScriptError);
    expect(() => loadScript(p)).toThrow(/b2/);
  });
});

describe("scriptHash", () => {
  it("ignores CRLF and trailing whitespace", () => {
    const original = scriptHash(FIXTURE);
    const p = variant((s) => s.split("\n").map((l) => l + "  ").join("\r\n"));
    expect(scriptHash(p)).toBe(original);
  });

  it("changes on a word change", () => {
    const original = scriptHash(FIXTURE);
    const p = variant((s) => s.replace("planning", "shipping"));
    expect(scriptHash(p)).not.toBe(original);
  });
});

describe("approvals", () => {
  it("round-trips", () => {
    const approvals = join(dir, "approvals.json");
    writeFileSync(approvals, "{}\n");
    const hash = scriptHash(FIXTURE);
    expect(isApproved("v-test", hash, approvals)).toBe(false);
    approve("v-test", hash, approvals);
    expect(JSON.parse(readFileSync(approvals, "utf8"))).toEqual({ "v-test": hash });
    expect(isApproved("v-test", hash, approvals)).toBe(true);
    const edited = scriptHash(variant((s) => s.replace("planning", "shipping")));
    expect(isApproved("v-test", edited, approvals)).toBe(false);
  });
});
