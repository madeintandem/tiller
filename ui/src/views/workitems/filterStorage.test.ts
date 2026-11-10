import { afterEach, describe, expect, it } from "vitest";

import { readWorkItemFilters } from "./board";
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

  it("writes sort and severity in canonical order and omits defaults", () => {
    expect(filterQuery(new URLSearchParams("sort=severity&severity=important,blocker"))).toBe(
      "sort=severity&severity=blocker%2Cimportant");
    expect(filterQuery(new URLSearchParams("sort=shelf&severity=bogus"))).toBe("");
    expect(hasFilterParams(new URLSearchParams("sort=added"))).toBe(true);
    expect(hasFilterParams(new URLSearchParams("severity=blocker"))).toBe(true);
  });

  it("never stores the visit-only unfiltered marker", () => {
    expect(hasFilterParams(new URLSearchParams("filters=none"))).toBe(false);
    expect(filterQuery(new URLSearchParams("filters=none"))).toBe("");
  });

  it("restores a legacy stored noun with shelf sort and no severity", () => {
    window.localStorage.setItem(filterStorageKey("acme"), "noun=bug");
    expect(readFilterQuery("acme")).toBe("noun=bug");
    expect(readWorkItemFilters(new URLSearchParams(readFilterQuery("acme") ?? ""))).toMatchObject({
      noun: "bug", sort: "shelf", severity: [],
    });
  });
});
