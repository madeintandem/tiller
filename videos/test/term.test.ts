import { describe, expect, it } from "vitest";
import { termState } from "../src/term/state";
import { readSnapshot } from "../src/check/diff";
import type { Step } from "../src/check/diff";

const fps = 30;
const steps: Step[] = [
  { id: "a", actor: "you", kind: "cmd", cmd: "tiller status now", out: ["**ok** ++pass++", "!!bad!!"] },
  { id: "b", actor: "agent", kind: "cmd", cmd: "tiller status new", out: ["**ok** ++pass++", "!!bad!!"] },
];

describe("termState", () => {
  it("is empty at startFrame", () => {
    expect(termState(steps, 100, 400, 100, fps)).toEqual([]);
  });

  it("shows every step fully typed at endFrame", () => {
    const lines = termState(steps, 100, 400, 400, fps);
    expect(lines.map((l) => l.kind)).toEqual(["cmd", "out", "out", "cmd", "out", "out"]);
    for (const l of lines) expect(l.typed).toBe(l.text.length);
  });

  it("at the midpoint of two equal commands the first is complete and the second is partly typed", () => {
    const lines = termState(steps, 100, 400, 255, fps); // a few frames past the 250 midpoint
    expect(lines.slice(0, 3).map((l) => l.kind)).toEqual(["cmd", "out", "out"]);
    expect(lines[0].typed).toBe(lines[0].text.length);
    expect(lines[3].kind).toBe("cmd");
    expect(lines[3].typed).toBeGreaterThan(0);
    expect(lines[3].typed).toBeLessThan(lines[3].text.length);
    expect(lines.length).toBe(4);
  });

  it("types at 40 characters per second when the share allows", () => {
    const long: Step[] = [{ id: "x", actor: "you", kind: "cmd", cmd: "x".repeat(80), out: [] }];
    const lines = termState(long, 0, 30 * 20, 30 * 1, fps);
    expect(lines[0].typed).toBe(40);
  });

  it("shows a say whole and keeps markup in the text", () => {
    const s: Step[] = [
      { id: "s", actor: "agent", kind: "say", text: "hello" },
      { id: "c", actor: "agent", kind: "cmd", cmd: "c", out: ["**b** !!f!! ++p++"] },
    ];
    const lines = termState(s, 0, 300, 300, fps);
    expect(lines[0]).toEqual({ actor: "agent", kind: "say", text: "hello", typed: 5 });
    expect(lines[2].text).toBe("**b** !!f!! ++p++");
  });

  it("completes a real snapshot by endFrame", () => {
    const real = readSnapshot("s1");
    const lines = termState(real, 0, 30 * 60, 30 * 60, fps);
    const expected = real.reduce((n, s) => n + 1 + (s.out?.length ?? 0), 0);
    expect(lines.length).toBe(expected);
  });
});
