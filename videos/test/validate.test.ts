import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { approveScript } from "../src/build/approve.js";
import { layoutFor, type Layout } from "../src/build/layout.js";
import { checkScript, estimateSeconds } from "../src/build/validate.js";
import { loadScript, scriptHash } from "../src/script/load.js";
import type { Script } from "../src/script/schema.js";

const FIXTURE = "test/fixtures/v-test.yaml";
const layout = layoutFor();
const base = (): Script => loadScript(FIXTURE);

describe("checkScript", () => {
  it("passes the fixture against the committed snapshots and marks", () => {
    expect(checkScript(base(), layout)).toEqual({ problems: [], warnings: [] });
  });

  it("names a term beat whose step is not in the committed snapshot", () => {
    const s = base();
    s.beats[1].show = { kind: "term", stage: "s1", from: "s1-add-contributor", to: "s1-nope" };
    expect(checkScript(s, layout).problems).toEqual([expect.stringMatching(/b2.*s1-nope/)]);
  });

  it("names a term beat whose stage has no snapshot", () => {
    const s = base();
    s.beats[1].show = { kind: "term", stage: "s9", from: "a", to: "b" };
    expect(checkScript(s, layout).problems).toEqual([expect.stringMatching(/b2.*s9/)]);
  });

  it("names a capture whose fixture mark does not exist", () => {
    const s = base();
    s.captures[0].mark = "s1-missing";
    expect(checkScript(s, layout).problems).toEqual([expect.stringMatching(/board.*s1-missing/)]);
  });

  it("refuses a title over 100 characters and angle brackets in the title, description, or a chapter", () => {
    const s = base();
    s.title = "x".repeat(101);
    s.description = "Use <b>bold</b>";
    s.beats[0].chapter = "A > B";
    const problems = checkScript(s, layout).problems;
    expect(problems).toHaveLength(3);
    expect(problems[0]).toMatch(/title.*100/);
    expect(problems[1]).toMatch(/description.*</);
    expect(problems[2]).toMatch(/chapter.*>/);
  });

  it("estimates the length from the words at 175 wpm plus intro, gaps, and outro", () => {
    const s = base();
    const words = s.beats.reduce((n, b) => n + b.say.split(/\s+/).filter(Boolean).length, 0);
    const expected = (words / 175) * 60 + 20 + 0.4 + 0.35 * (s.beats.length - 1) + 6;
    expect(estimateSeconds(s)).toBeCloseTo(expected, 5);
  });

  it("leaves the intro estimate and its gap out for a script with no intro clip", () => {
    const s = base();
    const withIntro = estimateSeconds(s);
    delete s.intro;
    expect(estimateSeconds(s)).toBeCloseTo(withIntro - 20 - 0.4, 5);
  });

  it("warns past 7:15 and fails past 8:00", () => {
    const s = base();
    const pad = (n: number) => Array.from({ length: n }, () => "word").join(" ");
    s.beats[0].say = pad(1150); // about 7:30 with the other beats
    const warned = checkScript(s, layout);
    expect(warned.problems).toEqual([]);
    expect(warned.warnings).toEqual([expect.stringMatching(/7:\d\d.*7:15/)]);
    s.beats[0].say = pad(1300); // about 8:20
    expect(checkScript(s, layout).problems).toEqual([expect.stringMatching(/8:\d\d.*8:00/)]);
  });
});

describe("approveScript", () => {
  let root: string;
  let l: Layout;
  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "approve-"));
    mkdirSync(join(root, "scripts"));
    copyFileSync(FIXTURE, join(root, "scripts", "v-test.yaml"));
    writeFileSync(join(root, "approvals.json"), "{}\n");
    l = { ...layoutFor(root, {}), snapshots: layout.snapshots, marks: layout.marks };
  });
  afterEach(() => rmSync(root, { recursive: true, force: true }));

  const approvals = () => JSON.parse(readFileSync(join(root, "approvals.json"), "utf8"));

  it("records the hash of a script that can build", () => {
    const r = approveScript("v-test", l);
    expect(r.hash).toBe(scriptHash(join(root, "scripts", "v-test.yaml")));
    expect(approvals()).toEqual({ "v-test": r.hash });
  });

  it("refuses a script whose id differs from its file name", () => {
    copyFileSync(FIXTURE, join(root, "scripts", "v-other.yaml"));
    expect(() => approveScript("v-other", l)).toThrow(/id "v-test".*v-other/);
    expect(approvals()).toEqual({});
  });

  it("refuses a script with a step that is not in the snapshots", () => {
    const p = join(root, "scripts", "v-test.yaml");
    writeFileSync(p, readFileSync(p, "utf8").replace("to: s1-join", "to: s1-nope"));
    expect(() => approveScript("v-test", l)).toThrow(/s1-nope/);
    expect(approvals()).toEqual({});
  });
});
