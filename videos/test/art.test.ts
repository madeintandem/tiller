import { execFileSync } from "node:child_process";
import { describe, expect, it } from "vitest";
import { layoutFor } from "../src/build/layout.js";
import { tillerVersion } from "../src/check/run.js";
import { ALL_COMMANDS, SKILLS } from "../src/components/art/cliData.js";
import { PROCESS_ROWS } from "../src/components/art/Art.js";
import { loadSeries } from "../src/script/load.js";

const layout = layoutFor();
const pinned = (() => {
  try {
    return tillerVersion(layout.tiller) === loadSeries(layout.series).tiller;
  } catch {
    return false;
  }
})();
const help = (...args: string[]) => execFileSync(layout.tiller, [...args, "--help"], { encoding: "utf8" });

// The "cli" graphic claims to show all of Tiller, so it is held to the pinned binary's own help.
describe.skipIf(!pinned)("the cli graphic against the pinned tiller", () => {
  it("lists every command, and only commands that exist", () => {
    const section = help().split("Available Commands:")[1].split("Flags:")[0];
    const real = section.split("\n").map((l) => l.trim().split(/\s+/)[0]).filter(Boolean);
    // Cobra's own two are not Tiller's.
    const tiller = real.filter((c) => c !== "completion" && c !== "help");
    expect([...ALL_COMMANDS].sort()).toEqual(tiller.sort());
  });

  it("lists every skill", () => {
    const line = help("guide").split("\n").find((l) => l.startsWith("Skills:"))!;
    const real = line.replace("Skills:", "").replace(/\.$/, "").split(",").map((s) => s.trim());
    expect([...SKILLS].sort()).toEqual(real.sort());
  });
});

describe("the process graphic", () => {
  it("has a row per video, each ending on the human's decision", () => {
    expect(PROCESS_ROWS).toHaveLength(3);
    for (const row of PROCESS_ROWS) expect(row[row.length - 1][0]).toBe("you");
  });
});
