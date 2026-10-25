// The flat question body owns the complete statement, its composition edges,
// and the recorded answer. The shell owns the bounded title and reference, so
// the statement appears exactly once here as plain content before Blocks and
// Answer. The answer field is a Go *string under omitempty: absent means open,
// present means answered, and null never crosses the wire.
//
// Fix wave (task-17-review, Important finding 3): an open question used to
// carry its own copy of the answer command here too (JC's ask, 2026-07-26),
// which task 17's attention stack now duplicates verbatim - the stack
// renders this same node's own answer_question queue entry as its own loud
// box (every queue entry addressed to this node, unfiltered for a question,
// NodeDetail.tsx's entriesForNode/asksFrom), so the two surfaces sat on the
// same page showing the identical `tiller answer question <id> --answer
// "..."` twice. Retired here rather than left standing beside its own
// duplicate: the stack owns the ask now, and this body owns only what it
// uniquely holds, the blocks and a recorded answer. An open question also
// carries the cancel and per-target unlink commands (the answer command stays
// with the stack); a cancelled one shows its reason and no commands.

import { Fragment } from "react";
import type { NodePage } from "../../api";
import CopyCmd from "../../components/CopyCmd";
import InstrSection from "../../components/InstrSection";
import { has } from "../../lib/has";
import RelatedList from "./RelatedList";

export default function QuestionBody({ page }: { page: NodePage }) {
  const blocks = page.relationships.blocks ?? [];
  // A legacy entry with no status key reads as open, as the CLI treats it.
  const isOpen = !page.node.status || page.node.status === "open";
  return (
    <>
      {has(page.node.statement) && (
        <div className="question-statement">
          {page.node.statement.split(/(?:\r?\n){2,}/).map((paragraph, paragraphIndex) => (
            <p key={paragraphIndex}>
              {paragraph.split(/\r?\n/).map((line, lineIndex) => (
                <Fragment key={lineIndex}>
                  {lineIndex > 0 && <br />}
                  {line}
                </Fragment>
              ))}
            </p>
          ))}
        </div>
      )}
      {blocks.length > 0 && (
        <InstrSection label="Blocks" caption={`${blocks.length}`}>
          <RelatedList items={blocks} />
        </InstrSection>
      )}
      {page.node.answer !== undefined && (
        <InstrSection label="Answer">
          <blockquote className="answer-block">{page.node.answer}</blockquote>
        </InstrSection>
      )}
      {isOpen && (
        <InstrSection label="Cancel or narrow">
          <CopyCmd cmd={`tiller cancel question ${page.node.id} --reason "..."`} />
          {blocks.map((b) => (
            <CopyCmd key={b.id} cmd={`tiller unlink question ${page.node.id} --blocks ${b.id}`} />
          ))}
        </InstrSection>
      )}
      {page.node.status === "cancelled" && has(page.node.cancel_reason) && (
        <InstrSection label="Cancelled">
          <blockquote className="answer-block">{page.node.cancel_reason}</blockquote>
        </InstrSection>
      )}
    </>
  );
}
