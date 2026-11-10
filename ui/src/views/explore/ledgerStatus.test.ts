import { describe, expect, it } from "vitest";

import type { ListRow } from "../../api";
import { activeLedgerStatus, ledgerStatusTabs, withLedgerStatus } from "./ledgerStatus";

const order = ["open", "answered", "cancelled"] as const;
const row = (status: string) => ({ id: status, ref: status, status }) as ListRow;

describe("activeLedgerStatus", () => {
  const withOpen = ledgerStatusTabs([row("open"), row("answered")], order);
  const noOpen = ledgerStatusTabs([row("answered")], order);

  it("a known status selects itself", () => {
    expect(activeLedgerStatus(new URLSearchParams("status=answered"), withOpen, "open")).toBe("answered");
  });

  it("status=all always selects All, even when a fallback applies", () => {
    expect(activeLedgerStatus(new URLSearchParams("status=all"), withOpen, "open")).toBe("all");
  });

  it("no parameter selects the fallback when a row holds it", () => {
    expect(activeLedgerStatus(new URLSearchParams(""), withOpen, "open")).toBe("open");
  });

  it("no parameter selects All when no row holds the fallback", () => {
    expect(activeLedgerStatus(new URLSearchParams(""), noOpen, "open")).toBe("all");
  });

  it("no parameter and no fallback selects All", () => {
    expect(activeLedgerStatus(new URLSearchParams(""), withOpen)).toBe("all");
  });

  it("an unknown value behaves like no parameter", () => {
    expect(activeLedgerStatus(new URLSearchParams("status=bogus"), withOpen, "open")).toBe("open");
    expect(activeLedgerStatus(new URLSearchParams("status=bogus"), noOpen, "open")).toBe("all");
  });
});

describe("withLedgerStatus", () => {
  it("writes status=all for All rather than deleting the parameter", () => {
    expect(withLedgerStatus(new URLSearchParams("status=open"), "all").get("status")).toBe("all");
  });

  it("writes a named status and keeps other parameters", () => {
    const next = withLedgerStatus(new URLSearchParams("x=1"), "answered");
    expect(next.get("status")).toBe("answered");
    expect(next.get("x")).toBe("1");
  });
});
