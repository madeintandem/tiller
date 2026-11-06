import { describe, expect, it } from "vitest";
import { layoutFor } from "../src/build/layout.js";
import { previewMarkdown } from "../src/build/preview.js";
import { loadScript } from "../src/script/load.js";

const layout = layoutFor();

describe("previewMarkdown", () => {
  const md = previewMarkdown(loadScript("test/fixtures/v-test.yaml"), layout);

  it("inlines a term beat's commands and say lines from the committed snapshot", () => {
    // b2 shows s1-add-contributor to s1-join.
    expect(md).toContain('$ tiller add contributor jc --name "JC Grubbs"');
    expect(md).toContain("$ tiller join --as jc");
    expect(md).not.toContain("s1-add-contributor to s1-join");
  });

  it("names a console beat's page and fixture mark", () => {
    expect(md).toMatch(/Console: `\/` at mark s1-end, 5 s/);
  });

  it("states the estimated length", () => {
    expect(md).toMatch(/Estimated length: \d:\d\d/);
  });

  it("lists a step missing from the snapshots instead of failing silently", () => {
    const s = loadScript("test/fixtures/v-test.yaml");
    s.beats[1].show = { kind: "term", stage: "s1", from: "s1-add-contributor", to: "s1-nope" };
    const bad = previewMarkdown(s, layout);
    expect(bad).toMatch(/Problems[\s\S]*s1-nope/);
  });
});
