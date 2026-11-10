// AmendmentsPane is the amendments ledger at /amendments, a child of the
// Explore layout like QuestionsPane, so the navigator stays standing beside
// it. Rows are a four-column grid (title, key, date, status) ordered proposed
// first, then newest by the proposed date; rows with no date keep the served
// order (proposed before terminal, then key and immutable ID). Every row links
// by its A-ID because terminal records can share a reused key. The status rail (ledgerStatus.ts) narrows
// the ledger through ?status=. The Filter subtab's amendments noun remains
// for faceted reads.

import { Link, useSearchParams } from "react-router-dom";
import type { ListResult, ListRow } from "../../api";
import { listPath } from "../../api";
import Chip from "../../components/Chip";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import Time from "../../components/Time";
import { titleOf, useDocumentTitle, useFetch } from "../../hooks";
import { has } from "../../lib/has";
import "../explore-ledgers.css";
import { activeLedgerStatus, ledgerStatusTabs, rowsForStatus, withLedgerStatus } from "./ledgerStatus";

const AMENDMENT_STATUSES = ["proposed", "approved", "withdrawn"] as const;

// The grid's order: proposed amendments lead, each group newest first by the
// date it was proposed. An undated row follows the dated ones in its group, and
// equal rows keep the order the server served (Array.sort is stable).
function proposedFirstNewest(rows: ListRow[]): ListRow[] {
  const rank = (row: ListRow) => (row.status === "proposed" ? 0 : 1);
  const at = (row: ListRow) => (has(row.proposed_at) ? Date.parse(row.proposed_at) : Number.NaN);
  return [...rows].sort((a, b) => {
    if (rank(a) !== rank(b)) return rank(a) - rank(b);
    const x = at(a);
    const y = at(b);
    if (Number.isNaN(x) || Number.isNaN(y)) {
      if (Number.isNaN(x) === Number.isNaN(y)) return 0;
      return Number.isNaN(x) ? 1 : -1;
    }
    return y - x;
  });
}

export default function AmendmentsPane() {
  useDocumentTitle(titleOf("Amendments"));
  const { data, error } = useFetch<ListResult>(listPath("amendments"));
  const [params, setParams] = useSearchParams();

  if (error !== null) {
    return (
      <div className="explore-detail">
        <p className="error">{error}</p>
      </div>
    );
  }
  if (data === null) {
    return (
      <div className="explore-detail">
        <p className="muted">Loading amendments...</p>
      </div>
    );
  }

  const rows = data.nodes;
  const proposed = rows.filter((row) => row.status === "proposed").length;
  const tabs = ledgerStatusTabs(rows, AMENDMENT_STATUSES);
  const status = activeLedgerStatus(params, tabs);

  return (
    <div className="explore-detail">
      <div className="qhead">
        <h1>Amendments</h1>
        <p className="sub">
          {data.count} recorded · {proposed} proposed · approved and withdrawn amendments stay on the ledger
        </p>
      </div>
      {rows.length === 0 ? (
        <EmptyNote>No amendments yet.</EmptyNote>
      ) : (
        <NodeTabs
          label="Amendment status"
          tabs={tabs.map((tab) => ({
            ...tab,
            render: () => {
              const shown = rowsForStatus(rows, tab.key);
              return shown.length === 0 ? (
                <EmptyNote>No {tab.label.toLowerCase()} amendments.</EmptyNote>
              ) : (
                <table className="amend-grid">
                  <caption>Proposed first, then newest</caption>
                  <thead>
                    <tr>
                      <th scope="col">Title</th>
                      <th scope="col">Key</th>
                      <th scope="col">Date</th>
                      <th scope="col">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {proposedFirstNewest(shown).map((row) => (
                      <tr key={row.id}>
                        <td className="amend-title">
                          <Link to={`/node/amendment/${row.id}`} className="explore-result-title">
                            {has(row.title) ? row.title : row.ref}
                          </Link>
                        </td>
                        <td className="amend-key">{has(row.key) && <span className="mono muted">{row.key}</span>}</td>
                        <td className="amend-date">{has(row.proposed_at) && <Time iso={row.proposed_at} />}</td>
                        <td>
                          <Chip status={row.status} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              );
            },
          }))}
          active={status}
          onSelect={(key) => setParams(withLedgerStatus(params, key))}
        />
      )}
    </div>
  );
}
