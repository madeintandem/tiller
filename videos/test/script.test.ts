import { describe, expect, it, beforeEach, afterEach } from "vitest";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { hashText, loadScript, readScript, scriptHash } from "../src/script/load.js";
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
    expect(s.beats.length).toBe(6);
    expect(s.beats[0].show.kind).toBe("card");
    expect(s.captures.length).toBe(1);
  });

  it("defaults captures to an empty array", () => {
    // Drop the captures block and the one console beat that uses it.
    const p = variant((s) => s.replace(/captures:[\s\S]*?beats:/, "beats:").replace(/  - id: b4\n[\s\S]*?(?=  - id: b5)/, ""));
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

  it("accepts wait and scrollTo capture actions and rejects a bad wait or an unknown action", () => {
    const withActions = (actions: string) => variant((s) => s.replace("      - scroll: 200\n", actions));
    const ok = loadScript(withActions('      - scroll: 200\n      - wait: 1.5\n      - scrollTo: "text=Edges"\n'));
    expect(ok.captures[0].actions).toEqual([{ waitFor: "main" }, { scroll: 200 }, { wait: 1.5 }, { scrollTo: "text=Edges" }]);
    expect(() => loadScript(withActions("      - wait: 0\n"))).toThrow(/wait/);
    expect(() => loadScript(withActions("      - hover: main\n"))).toThrow(/no known key/);
  });

  it("rejects a term show missing from/to", () => {
    const p = variant((s) => s.replace("      from: s1-add-contributor\n", ""));
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

  it("ignores trailing blank lines at the end of the file", () => {
    const original = scriptHash(FIXTURE);
    expect(scriptHash(variant((s) => s.replace(/\n*$/, "\n\n\n")))).toBe(original);
    expect(scriptHash(variant((s) => s.replace(/\n*$/, "")))).toBe(original);
  });

  it("readScript hashes exactly the bytes it parsed", () => {
    const p = variant((s) => s);
    const { script, hash } = readScript(p);
    writeFileSync(p, readFileSync(FIXTURE, "utf8").replace("planning", "shipping"));
    // The file changed after the read; the pair still describes the original bytes.
    expect(hash).toBe(scriptHash(FIXTURE));
    expect(script.beats[0].say).toContain("planning");
    expect(hashText(readFileSync(FIXTURE, "utf8"))).toBe(hash);
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
