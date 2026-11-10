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

// The active tab from ?status=. A value naming a tab selects it (status=all
// selects All explicitly). A missing or unknown value means "the default":
// the fallback status when some row holds it, else All. The fallback is how a
// ledger opens on the status that needs attention without hiding the rest.
export function activeLedgerStatus(
  params: URLSearchParams,
  tabs: LedgerStatusTab[],
  fallback?: string,
): string {
  const raw = params.get("status");
  if (tabs.some((tab) => tab.key === raw)) return raw as string;
  if (fallback !== undefined && tabs.some((tab) => tab.key === fallback && tab.count > 0)) return fallback;
  return ALL_STATUSES;
}

// Every tab writes its own key, All included: a missing parameter means the
// default, so All must be explicit to stay All.
export function withLedgerStatus(params: URLSearchParams, status: string): URLSearchParams {
  const next = new URLSearchParams(params);
  next.set("status", status);
  return next;
}

export function rowsForStatus(rows: ListRow[], status: string): ListRow[] {
  return status === ALL_STATUSES ? rows : rows.filter((row) => row.status === status);
}
