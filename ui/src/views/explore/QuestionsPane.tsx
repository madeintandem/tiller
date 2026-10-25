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
// status through ?status=; All is the default.
//
// A card renders only the fields the row actually carries. The mock's own
// question cards additionally show "Asked by jc · Wed" / "Answered by pat
// · Jul 24" - illustrative prose the wire has no field for (ListRow carries
// no asker, no answered-by, no timestamp), so this pane does not invent one;
// DecisionCard.tsx's own doc comment names the identical discipline for the
// desk's queue cards.

import { Fragment } from "react";
import { useSearchParams } from "react-router-dom";
import type { ListResult, ListRow } from "../../api";
import { listPath } from "../../api";
import Chip from "../../components/Chip";
import CopyCmd from "../../components/CopyCmd";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import RefLink from "../../components/RefLink";
import { titleOf, useDocumentTitle, useFetch } from "../../hooks";
import { has } from "../../lib/has";
import { PersonName } from "../../people";
import { activeLedgerStatus, ledgerStatusTabs, rowsForStatus, withLedgerStatus } from "./ledgerStatus";

const QUESTION_STATUSES = ["open", "answered", "cancelled"] as const;

export default function QuestionsPane() {
  useDocumentTitle(titleOf("Questions"));
  const { data, error } = useFetch<ListResult>(listPath("question"));
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
  const status = activeLedgerStatus(params, tabs);

  return (
    <div className="explore-detail">
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

function QuestionCard({ row }: { row: ListRow }) {
  const isOpen = row.status === "open";
  const blocks = row.blocks_refs ?? [];
  const title = has(row.title) ? row.title : row.ref;
  return (
    <div className={isOpen ? "qcard open-q" : "qcard"}>
      <div className="q-top">
        <RefLink node={{ noun: "question", id: row.id, ref: row.ref }} label={title} className="q-st" />
        <Chip status={row.status} />
      </div>
      <span className="q-id mono">{row.id}</span>
      <div className="q-meta">
        {has(row.due) && <span className="due">Due {row.due}</span>}
        {blocks.length > 0 && (
          <span>
            Blocks{" "}
            {blocks.map((b, i) => (
              <Fragment key={b.id}>
                {i > 0 && ", "}
                <RefLink node={b} label={has(b.title) ? b.title : b.ref} className="mono" />
              </Fragment>
            ))}
          </span>
        )}
        {has(row.owner) && <span>Owner <PersonName id={row.owner} name={row.owner_name} /></span>}
      </div>
      {isOpen ? (
        // Hand-built, not deduped away like QuestionBody.tsx's own copy was
        // (that page's node-level command IS a queue entry's own
        // entry.command, so it could just render the wire value; task-17-
        // review fix wave, Important finding 3). This pane renders
        // ListRow, which carries no command field of its own, so there is
        // no wire value to reuse - this string is a deliberate byte-for-
        // byte mirror of internal/views/queue.go:257's own construction
        // (queueAnswerQuestions, `tiller answer question ` + q.ID + `
        // --answer "..."`), kept in sync by hand rather than re-derived
        // (fold-in minor, phase-4-review fix wave).
        <>
          <CopyCmd cmd={`tiller answer question ${row.id} --answer "..."`} accent />
          <CopyCmd cmd={`tiller cancel question ${row.id} --reason "..."`} />
          {blocks.map((b) => (
            <CopyCmd key={b.id} cmd={`tiller unlink question ${row.id} --blocks ${b.id}`} />
          ))}
        </>
      ) : (
        <>
          {has(row.answer) && <div className="q-ans">{row.answer}</div>}
          {has(row.cancel_reason) && <div className="q-ans">{row.cancel_reason}</div>}
        </>
      )}
    </div>
  );
}
