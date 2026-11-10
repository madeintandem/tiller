import { describe, expect, it } from "vitest";
import { EDIT_MAX_LINES, editView, termState } from "../src/term/state";
import { surfaceFor, termItems } from "../src/components/Terminal";
import { readSnapshot } from "../src/check/diff";
import type { Step } from "../src/check/diff";

const fps = 30;
const steps: Step[] = [
  { id: "a", actor: "you", kind: "cmd", cmd: "tiller status now", out: ["**ok** ++pass++", "!!bad!!"] },
  { id: "b", actor: "agent", kind: "cmd", cmd: "tiller status new", out: ["**ok** ++pass++", "!!bad!!"] },
];

describe("the two surfaces", () => {
  it("is the agent session whenever the agent takes part, and the shell otherwise", () => {
    expect(surfaceFor(steps)).toBe("agent");
    expect(surfaceFor([steps[0]])).toBe("shell");
  });

  it("groups a command with its output and keeps a message on its own", () => {
    const mixed: Step[] = [{ id: "s", actor: "you", kind: "say", text: "Go." }, ...steps];
    const items = termItems(termState(mixed, 0, 300, 300, fps));
    expect(items.map((i) => i.kind)).toEqual(["say", "run", "run"]);
    expect(items[1].kind === "run" && items[1].out).toEqual(["**ok** ++pass++", "!!bad!!"]);
    // Only the step in progress is current.
    expect(items.map((i) => i.current)).toEqual([false, false, true]);
  });
});

describe("an edit step", () => {
  const gate: Step = { id: "e", actor: "you", kind: "edit", file: "tiller.yml", new: false, removed: ["# tests"], added: ["tests: bin/test"] };
  const doc: Step = { id: "w", actor: "agent", kind: "edit", file: "doc.md", new: true, removed: [], added: ["# Title", "", ...Array.from({ length: 9 }, (_, i) => `line ${i}`)] };

  it("appears whole, as its file and changed lines", () => {
    const [item] = termItems(termState([gate], 0, 90, 1, fps));
    expect(item).toMatchObject({ kind: "edit", actor: "you", file: "tiller.yml", edit: { isNew: false, removed: ["# tests"], added: ["tests: bin/test"], more: 0 } });
  });

  it("shows a new file's first lines, skipping blanks, and counts the rest", () => {
    const v = editView(doc);
    expect(v.isNew).toBe(true);
    expect(v.added).toEqual(["# Title", "line 0", "line 1", "line 2", "line 3", "line 4"]);
    expect(v.added).toHaveLength(EDIT_MAX_LINES);
    expect(v.more).toBe(4);
  });

  it("is in the committed stage 1 recording wherever a file changes off the command line", () => {
    const edits = readSnapshot("s1").filter((s) => s.kind === "edit").map((s) => s.id);
    expect(edits).toEqual(["s1-edit-gate", "s1-write-api", "s1-write-store", "s1-edit-readme"]);
  });
});

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

describe("termState output reveal", () => {
  const real = readSnapshot("s1");
  const step = real.find((s) => s.id === "s1-approve-design")!;
  const one = [step];
  const n = step.out!.length;
  const outs = (f: number, end: number) => termState(one, 0, end, f, fps).filter((l) => l.kind === "out").length;

  it("reveals a long output a line at a time across the step's share, not all at once", () => {
    const end = 30 * 40;
    const counts: number[] = [];
    for (let f = 1; f <= end; f += 15) counts.push(outs(f, end));
    for (let i = 1; i < counts.length; i++) expect(counts[i]).toBeGreaterThanOrEqual(counts[i - 1]);
    expect(outs(end, end)).toBe(n);
    expect(new Set(counts.filter((c) => c > 0 && c < n)).size).toBeGreaterThan(n / 2);
    // Halfway through the reveal, roughly half the lines are out.
    const typeEnd = (step.cmd!.length / 40) * fps;
    const atMid = outs(Math.round(typeEnd + (end - typeEnd) * 0.85 * 0.5), end);
    expect(atMid).toBeGreaterThan(n * 0.35);
    expect(atMid).toBeLessThan(n * 0.65);
  });

  it("keeps the step's command as the head of the current block at every frame of its output", () => {
    const from = real.findIndex((s) => s.id === "s1-say-review");
    const steps = real.slice(from, real.indexOf(step) + 1);
    const end = 30 * 30;
    let sawOutput = false;
    for (let f = 1; f <= end; f++) {
      const lines = termState(steps, 0, end, f, fps);
      const current = lines.filter((l) => l.current);
      expect(current.length).toBeGreaterThan(0);
      // The current block is the tail of the list.
      expect(lines.slice(lines.length - current.length)).toEqual(current);
      if (current.some((l) => l.kind === "out")) {
        sawOutput = true;
        expect(current[0]).toMatchObject({ kind: "cmd", text: step.cmd });
      }
    }
    expect(sawOutput).toBe(true);
  });
});
