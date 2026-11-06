import { describe, expect, it } from "vitest";
import { diffSnapshots, type Step } from "../src/check/diff.js";
import { normalize } from "../src/check/normalize.js";

describe("normalize", () => {
  it("masks node IDs", () => {
    expect(normalize("Created feature orders/status-filter (F-jn1wyk) in **draft**.")).toBe(
      "Created feature orders/status-filter (<ID>) in **draft**.",
    );
    expect(normalize("EP-a1b2c3 and B-zzzzzz")).toBe("<ID> and <ID>");
  });
  it("masks SHAs", () => {
    expect(normalize("commit 1a2b3c4 and 0123456789abcdef0123456789abcdef01234567")).toBe("commit <SHA> and <SHA>");
  });
  it("masks ISO timestamps and the check feature form", () => {
    expect(normalize("at 2026-11-06T21:21:04Z done")).toBe("at <TIME> done");
    expect(normalize("at 2026-11-06T21:21:04.123+00:00")).toBe("at <TIME>");
    expect(normalize("checked 2026-11-06 21:21 UTC.")).toBe("checked <TIME>.");
  });
  it("masks durations", () => {
    expect(normalize("ok (0s)")).toBe("ok (<DUR>)");
    expect(normalize("ok (1.2s)")).toBe("ok (<DUR>)");
  });
  it("masks temp paths and dated event paths", () => {
    expect(normalize("in /var/folders/ab/cd/T/acme-orders-fixture.X1/acme-orders ok")).toBe("in <TMP> ok");
    expect(normalize("in /tmp/acme-x/acme-orders ok")).toBe("in <TMP> ok");
    expect(normalize("wrote .tiller/events/2026/11/06/abc.yml")).toBe("wrote .tiller/events/<DATE>/abc.yml");
  });
  it("masks 64-char evidence hashes", () => {
    expect(normalize("tests-abc1234-out-" + "f3b11509".repeat(8) + ".txt")).toBe("tests-<SHA>-out-<SHA>.txt");
  });
  it("masks diffstat sizes of event files and the totals line", () => {
    expect(normalize(" .tiller/events/<DATE>/<ID>.yml            | 17 ++++++++")).toBe(
      " .tiller/events/<DATE>/<ID>.yml            | <N> <CHANGES>",
    );
    expect(normalize(" src/server.js | 10 ++++-")).toBe(" src/server.js | 10 ++++-");
    expect(normalize(" 13 files changed, 207 insertions(+), 8 deletions(-)")).toBe(" 13 files changed, <STATS>");
  });
  it("leaves ordinary words alone", () => {
    for (const s of ["defaced facade decade", "Initialized \"Acme Orders\" (acme, brownfield).", "tiller version 0.22.0"]) {
      expect(normalize(s)).toBe(s);
    }
  });
});

const cmd = (id: string, c: string, out: string[]): Step => ({ id, actor: "you", kind: "cmd", cmd: c, out, exit: 0 });

describe("diffSnapshots", () => {
  const base = [cmd("a", "tiller add feature x", ["Created (F-aaaaaa) at 1a2b3c4d"]), { id: "b", actor: "you", kind: "say", text: "hi" } as Step];
  it("matches when only IDs and SHAs differ", () => {
    const fresh = [cmd("a", "tiller add feature x", ["Created (F-bbbbbb) at 9f8e7d6c"]), base[1]!];
    expect(diffSnapshots(base, fresh)).toEqual([]);
  });
  it("reports a changed command line", () => {
    const fresh = [cmd("a", "tiller add feature y", ["Created (F-aaaaaa) at 1a2b3c4d"]), base[1]!];
    expect(diffSnapshots(base, fresh).join("\n")).toMatch(/a: command changed/);
  });
  it("reports a missing step by ID", () => {
    expect(diffSnapshots(base, [base[0]!])).toEqual(["b: step is missing from the fresh run"]);
  });
  it("reports changed output text", () => {
    const fresh = [cmd("a", "tiller add feature x", ["Made it"]), base[1]!];
    expect(diffSnapshots(base, fresh).join("\n")).toMatch(/a: output changed at line 1/);
  });
});
