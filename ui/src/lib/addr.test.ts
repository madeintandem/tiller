// Vitest coverage for addrOf (lib/addr.ts), the key-or-id fallback every
// command-building and link-building call site on the console shares. Lifted
// here from views/Home.tsx's own local copy and the since-retired route
// strip's routeAddr (both duplicated the identical two-line rule), so the
// fallback, including its omitempty nuance, is pinned once rather than
// reasserted at every call site.

import { describe, expect, it } from "vitest";

import { addrOf } from "./addr";

describe("addrOf", () => {
  it("prefers the key when the node carries a non-empty one", () => {
    expect(addrOf({ id: "F-1", key: "foundation/app-skeleton" })).toBe("foundation/app-skeleton");
  });

  it("falls back to the id when key is an empty string", () => {
    expect(addrOf({ id: "T-1", key: "" })).toBe("T-1");
  });

  it("falls back to the id when key is absent entirely", () => {
    expect(addrOf({ id: "A-000001" })).toBe("A-000001");
  });

  it("treats an absent key exactly like an empty one (task-9-report.md's own wire note)", () => {
    expect(addrOf({ id: "T-1", key: "" })).toBe(addrOf({ id: "T-1" }));
  });
});
