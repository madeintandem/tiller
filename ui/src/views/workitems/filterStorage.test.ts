import { afterEach, describe, expect, it } from "vitest";

import { filterQuery, filterStorageKey, hasFilterParams, readFilterQuery, writeFilterQuery } from "./filterStorage";

afterEach(() => {
  window.localStorage.clear();
});

describe("Fix filter storage", () => {
  it("round-trips the normalized filter query under a per-project key", () => {
    writeFilterQuery("acme", new URLSearchParams("noun=bug&attached=F-1,none&terminal=all&tab=x"));
    expect(window.localStorage.getItem(filterStorageKey("acme"))).toBe("noun=bug&attached=F-1%2Cnone&terminal=all");
    expect(readFilterQuery("acme")).toBe("noun=bug&attached=F-1%2Cnone&terminal=all");
    expect(readFilterQuery("lantern")).toBeNull();
  });

  it("stores All as the empty query, distinct from nothing stored", () => {
    writeFilterQuery("acme", new URLSearchParams(""));
    expect(readFilterQuery("acme")).toBe("");
  });

  it("normalizes a hand-edited stored value the way the URL is read", () => {
    window.localStorage.setItem(filterStorageKey("acme"), "noun=bug,chore,tweak&terminal=some&attached=,F-1,F-1");
    expect(readFilterQuery("acme")).toBe("attached=F-1");
  });

  it("names a URL as filtered only when it carries a filter axis", () => {
    expect(hasFilterParams(new URLSearchParams(""))).toBe(false);
    expect(hasFilterParams(new URLSearchParams("tab=x"))).toBe(false);
    expect(hasFilterParams(new URLSearchParams("terminal=all"))).toBe(true);
    expect(filterQuery(new URLSearchParams("noun=chore"))).toBe("noun=chore");
  });
});
