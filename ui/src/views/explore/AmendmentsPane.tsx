// AmendmentsPane is the amendments ledger at /amendments, a child of the
// Explore layout like QuestionsPane, so the navigator stays standing beside
// it. Rows keep the order /api/ls serves (proposed before terminal, then key
// and immutable ID), and every row links by its A-ID because terminal
// records can share a reused key. The status rail (ledgerStatus.ts) narrows
// the ledger through ?status=. The Filter subtab's amendments noun remains
// for faceted reads.

import { Link, useSearchParams } from "react-router-dom";
import type { ListResult } from "../../api";
import { listPath } from "../../api";
import Chip from "../../components/Chip";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import { titleOf, useDocumentTitle, useFetch } from "../../hooks";
import { has } from "../../lib/has";
import { PersonName } from "../../people";
import { activeLedgerStatus, ledgerStatusTabs, rowsForStatus, withLedgerStatus } from "./ledgerStatus";

const AMENDMENT_STATUSES = ["proposed", "approved", "withdrawn"] as const;

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
                <ul className="rule-list explore-results">
                  {shown.map((row) => (
                    <li key={row.id} className="explore-result-row">
                      <Link to={`/node/amendment/${row.id}`} className="explore-result-title" title={row.key}>
                        {has(row.title) ? row.title : row.ref}
                      </Link>
                      {has(row.key) && <span className="mono muted explore-result-key">{row.key}</span>}
                      <Chip status={row.status} />
                      {has(row.owner) && (
                        <span className="muted explore-result-owner">
                          Owner: <PersonName id={row.owner} name={row.owner_name} />
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
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
