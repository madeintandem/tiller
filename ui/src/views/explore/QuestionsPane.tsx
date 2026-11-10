// QuestionsPane is the full ledger summarized by Explore's navigation link.
// It renders at /questions inside the Explore layout, keeping the navigator
// standing beside it and supporting a direct hard refresh.
//
// Data is the question ListRow /api/ls?noun=question already serves, widened
// by task 15 (internal/views/list.go's Item()) to carry answer, due, and
// blocks_refs - the same fetch Explore's question count makes, issued a
// second time here since this is a different route/component rather than a
// shared subtree (Explore.tsx's own per-panel-fetch posture: a fetch belongs
// to the component that renders its data, not threaded down as a prop from
// a common ancestor that may not always mount it).
//
// Grouping is open-before-answered, a RENDER grouping over the served rows
// (the same posture Build's six columns and Search's navigable-first split
// already take), never a re-sort within a group: Array.prototype.filter
// preserves relative order, so each partition keeps the order /api/ls served
// it in. A status rail above the list (ledgerStatus.ts) narrows it to one
// status through ?status=; the ledger opens on Open when any question is
// open and on All otherwise (settled once per visit, so a reread never flips
// it), and status=all is the explicit All.
//
// A card renders only the fields the row actually carries. The mock's own
// question cards additionally show "Asked by jc · Wed" / "Answered by pat
// · Jul 24" - illustrative prose the wire has no field for (ListRow carries
// no asker, no answered-by, no timestamp), so this pane does not invent one;
// DecisionCard.tsx's own doc comment names the identical discipline for the
// desk's queue cards.

import { useRef } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import type { ListResult, ListRow, RelatedNode } from "../../api";
import { listPath } from "../../api";
import Chip from "../../components/Chip";
import CopyCmd from "../../components/CopyCmd";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import OutOfDate from "../../components/OutOfDate";
import RefLink from "../../components/RefLink";
import { titleOf, useDocumentTitle, useFetch } from "../../hooks";
import Prose from "../../components/Prose";
import { has } from "../../lib/has";
import { PersonName } from "../../people";
import "../explore-ledgers.css";
import { activeLedgerStatus, ledgerStatusTabs, rowsForStatus, withLedgerStatus } from "./ledgerStatus";

const QUESTION_STATUSES = ["open", "answered", "cancelled"] as const;

export default function QuestionsPane() {
  useDocumentTitle(titleOf("Questions"));
  const { data, error } = useFetch<ListResult>(listPath("question"));
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  // The default tab (Open when any question is open, else All) is settled
  // once per visit, from the first ledger this visit shows, and kept until
  // the reader picks a tab: a reread that closes the last open question must
  // not flip the tab under the reader. It is never written to the URL.
  const visitDefault = useRef<{ visit: string; status: string } | null>(null);

  // An error replaces the ledger only before there is one; a failed reread
  // keeps the last ledger, marked out of date.
  if (error !== null && data === null) {
    return (
      <div className="explore-detail">
        <p className="error">{error}</p>
      </div>
    );
  }
  if (data === null) {
    return (
      <div className="explore-detail">
        <p className="muted">Loading questions...</p>
      </div>
    );
  }

  // An entry with no status key reads as open, as the CLI treats it.
  const rows = data.nodes.map((r) => (r.status === "" ? { ...r, status: "open" } : r));
  const open = rows.filter((r) => r.status === "open");
  const answered = rows.filter((r) => r.status === "answered");
  const cancelled = rows.filter((r) => r.status === "cancelled");
  const known: readonly string[] = QUESTION_STATUSES;
  const other = rows.filter((r) => !known.includes(r.status));
  const tabs = ledgerStatusTabs(rows, QUESTION_STATUSES);
  if (visitDefault.current?.visit !== location.key) {
    visitDefault.current = { visit: location.key, status: activeLedgerStatus(new URLSearchParams(), tabs, "open") };
  }
  const chosen = params.get("status");
  const status = tabs.some((tab) => tab.key === chosen) ? (chosen as string) : visitDefault.current.status;

  return (
    <div className="explore-detail">
      <OutOfDate error={error} />
      <div className="qhead">
        <h1>Questions</h1>
        <p className="sub">
          {data.count} recorded · {open.length} open · answered and cancelled questions stay on the
          ledger
        </p>
      </div>
      {rows.length === 0 ? (
        <EmptyNote>No questions yet.</EmptyNote>
      ) : (
        <NodeTabs
          label="Question status"
          tabs={tabs.map((tab) => ({
            ...tab,
            render: () => {
              const shown = rowsForStatus([...open, ...answered, ...cancelled, ...other], tab.key);
              return shown.length === 0 ? (
                <EmptyNote>No {tab.label.toLowerCase()} questions.</EmptyNote>
              ) : (
                <div className="qlist">
                  {shown.map((row) => (
                    <QuestionCard key={row.id} row={row} />
                  ))}
                </div>
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

// A blocked feature leads with its title (its ref when it has none, as a
// dangling edge does); the key follows in mono unless it would only repeat the
// label.
const blockLabel = (b: RelatedNode) => (has(b.title) ? b.title : b.ref);
const blockKey = (b: RelatedNode) => (has(b.key) ? b.key : b.id);

function QuestionCard({ row }: { row: ListRow }) {
  const isOpen = row.status === "open";
  const blocks = row.blocks_refs ?? [];
  const title = has(row.title) ? row.title : row.ref;
  const commands = isOpen ? row.commands : undefined;
  return (
    <div className={isOpen ? "qcard open-q" : "qcard"}>
      <div className="q-top">
        <RefLink node={{ noun: "question", id: row.id, ref: row.ref }} label={title} className="q-st" />
        <Chip status={row.status} />
      </div>
      <span className="q-id mono">{row.id}</span>
      {has(row.statement) && (
        // The statement through Prose, the renderer the node page uses, so
        // markdown markers never show and paragraphs keep their breaks. The
        // four-line clamp is CSS: line-clamp backed by a four-line
        // max-height (explore-ledgers.css).
        <div className="q-statement">
          <Prose markdown={row.statement} />
        </div>
      )}
      <div className="q-meta">
        {has(row.due) && <span className="due">Due {row.due}</span>}
        {has(row.owner) && <span>Owner <PersonName id={row.owner} name={row.owner_name} /></span>}
      </div>
      {blocks.length > 0 && (
        <div className="q-blocked">
          <span className="q-blocked-label">Blocks</span>
          <ul className="q-blocks">
            {blocks.map((b) => (
              <li key={b.id}>
                <RefLink node={b} label={blockLabel(b)} className="q-blocks-title" />{" "}
                {blockKey(b) !== blockLabel(b) && <span className="mono muted">{blockKey(b)}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
      {commands !== undefined && (
        // Only what the server supplied: the list payload carries the
        // commands (serve/list.go), so this card never composes one.
        <>
          <CopyCmd cmd={commands.answer} accent />
          <details className="q-more">
            <summary>More commands</summary>
            <CopyCmd cmd={commands.cancel} />
            {(commands.unlink ?? []).map((cmd) => (
              <CopyCmd key={cmd} cmd={cmd} />
            ))}
          </details>
        </>
      )}
      {!isOpen && (
        <>
          {has(row.answer) && <div className="q-ans">{row.answer}</div>}
          {has(row.cancel_reason) && <div className="q-ans">{row.cancel_reason}</div>}
        </>
      )}
    </div>
  );
}
