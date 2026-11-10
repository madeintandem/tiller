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

  it("lists a slide's points with their cues, and drops the live intro heading without an intro", () => {
    const s = loadScript("test/fixtures/v-test.yaml");
    delete s.intro;
    const b1 = s.beats[0].show;
    if (b1.kind === "card") b1.points = [{ text: "Plan first", on: "planning work" }, { text: "Then build" }];
    const slide = previewMarkdown(s, layout);
    expect(slide).toContain('- Plan first (on "planning work")');
    expect(slide).toContain("- Then build\n");
    expect(slide).not.toContain("Live intro");
    expect(md).toContain("## Live intro (JC)");
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
