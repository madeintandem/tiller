// The status rail over a ledger pane (Questions, Amendments): an All tab and
// one tab per status, each with its row count. The noun's lifecycle order
// leads; any other status the served rows carry (a hand-recorded legacy
// state) follows in first-seen order rather than disappearing. The choice
// rides ?status= so a filtered ledger is a shareable address.

import type { ListRow } from "../../api";

export const ALL_STATUSES = "all";

export interface LedgerStatusTab {
  key: string;
  label: string;
  count: number;
}

export function ledgerStatusTabs(rows: ListRow[], order: readonly string[]): LedgerStatusTab[] {
  const statuses = [...order];
  for (const row of rows) {
    if (!statuses.includes(row.status)) statuses.push(row.status);
  }
  return [
    { key: ALL_STATUSES, label: "All", count: rows.length },
    ...statuses.map((status) => ({
      key: status,
      label: status.charAt(0).toUpperCase() + status.slice(1).replace(/_/g, " "),
      count: rows.filter((row) => row.status === status).length,
    })),
  ];
}

// The active tab from ?status=: anything that names no tab reads as All, so a
// stale or hand-edited link still shows the whole ledger.
export function activeLedgerStatus(params: URLSearchParams, tabs: LedgerStatusTab[]): string {
  const raw = params.get("status");
  return tabs.some((tab) => tab.key === raw) ? (raw as string) : ALL_STATUSES;
}

export function withLedgerStatus(params: URLSearchParams, status: string): URLSearchParams {
  const next = new URLSearchParams(params);
  if (status === ALL_STATUSES) next.delete("status");
  else next.set("status", status);
  return next;
}

export function rowsForStatus(rows: ListRow[], status: string): ListRow[] {
  return status === ALL_STATUSES ? rows : rows.filter((row) => row.status === status);
}
