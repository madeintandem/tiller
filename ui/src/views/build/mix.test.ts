// Vitest coverage for the epic panel's mix bar composition: feature_counts
// (keyed by each feature's board column) folds onto StateBar's mix in
// the board's own fixed six-column order, zeros included so the visible bar
// and its caption can never disagree about which segments exist.

import { describe, expect, it } from "vitest";

import { composeMix, composeMixCaption, taskMix, taskMixCaption } from "./mix";

describe("composeMix", () => {
  it("emits all six columns in fixed order, zeros included", () => {
    const mix = composeMix({ done: 4, draft: 2 });
    expect(mix.map((m) => m.state)).toEqual(["draft", "specced", "ready", "active", "in_review", "done"]);
    expect(mix.find((m) => m.state === "draft")?.n).toBe(2);
    expect(mix.find((m) => m.state === "done")?.n).toBe(4);
    expect(mix.find((m) => m.state === "active")?.n).toBe(0);
  });

  it("returns an all-zero mix for a childless epic (feature_counts absent)", () => {
    const mix = composeMix(undefined);
    expect(mix.every((m) => m.n === 0)).toBe(true);
    expect(mix).toHaveLength(6);
  });
});

describe("composeMixCaption", () => {
  it("captions only the non-zero segments, in the mix's own order", () => {
    const mix = composeMix({ active: 3, draft: 1 });
    expect(composeMixCaption(mix)).toBe("1 draft · 3 active");
  });

  it("names the childless case explicitly rather than an empty string", () => {
    expect(composeMixCaption(composeMix(undefined))).toBe("no features yet");
  });

  // Review finding 1's own resolution: the caption names the excluded
  // remainder (a status the six columns do not cover, "cancelled" being
  // the one that exists in the store today) as a trailing "+N status"
  // clause, rather than letting six visible segments imply they are the
  // epic's whole story.
  it("appends the excluded remainder when feature_counts carries a non-column status", () => {
    const counts = { draft: 2, cancelled: 1 };
    expect(composeMixCaption(composeMix(counts), counts)).toBe("2 draft · +1 cancelled");
  });

  it("names an all-excluded epic (every feature cancelled) rather than falling back to 'no features yet'", () => {
    const counts = { cancelled: 3 };
    expect(composeMixCaption(composeMix(counts), counts)).toBe("+3 cancelled");
  });

  it("omits the remainder clause entirely when nothing is excluded", () => {
    const counts = { draft: 2 };
    expect(composeMixCaption(composeMix(counts), counts)).toBe("2 draft");
  });

  it("is unaffected by omitting the counts argument (backward compatible with a caller that has none)", () => {
    const mix = composeMix({ draft: 2 });
    expect(composeMixCaption(mix)).toBe("2 draft");
  });
});

describe("taskMix and taskMixCaption", () => {
  const t = (status: string) => ({ status });

  it("captions seven tasks in review as zero of seven done", () => {
    const tasks = Array.from({ length: 7 }, () => t("in_review"));
    expect(taskMixCaption(tasks)).toBe("0 of 7 done, 7 in review");
    expect(taskMix(tasks).filter((m) => m.n > 0)).toEqual([{ state: "in_review", n: 7 }]);
  });

  it("orders the caption done first, then lifecycle order, with status words in lower case", () => {
    const tasks = [t("check"), t("done"), t("ready"), t("in_progress"), t("in_progress"), t("proposed")];
    expect(taskMixCaption(tasks)).toBe("1 of 6 done, 1 proposed, 1 ready, 2 in progress, 1 check");
  });

  it("leaves cancelled tasks out of the count and the caption", () => {
    const tasks = [t("done"), t("cancelled"), t("ready")];
    expect(taskMixCaption(tasks)).toBe("1 of 2 done, 1 ready");
    expect(taskMix(tasks).reduce((sum, m) => sum + m.n, 0)).toBe(2);
  });

  it("reads all done and no tasks plainly", () => {
    expect(taskMixCaption([t("done"), t("done")])).toBe("2 of 2 done");
    expect(taskMixCaption([])).toBe("no tasks yet");
    expect(taskMixCaption([t("cancelled")])).toBe("no tasks yet");
  });

  it("folds task statuses onto the bar's chart states, in lifecycle order", () => {
    const mix = taskMix([t("done"), t("check"), t("in_progress"), t("proposed")]);
    expect(mix.filter((m) => m.n > 0)).toEqual([
      { state: "specced", n: 1 },
      { state: "active", n: 2 },
      { state: "done", n: 1 },
    ]);
  });
});
