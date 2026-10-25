// InstrSection is the node instrument's section frame: the mock's own 11px
// letterspaced eyebrow (.flist .fh, docs/plans/2026-07-23-console-chart-table-
// mocks.html) over whatever the section holds. instrument.css already carried
// a faithful copy of that head as .instr-fh, but only the task ledger reached
// for it; every other section on the page was still spending the pass-1 <h2>,
// 60px of chrome for the same 35px job.
//
// The rule this component exists to make cheap: A SECTION THAT HOLDS NOTHING
// IS NOT DRAWN. The mock's draft-state instrument shows no empty Tasks list,
// no empty Relationships, no History; it simply stops. On a draft-heavy tree,
// which is where a project spends its first week, most sections are empty, and
// a heading plus a parenthetical each was two lines of furniture per absence.
// Callers decide their own emptiness and return null, rather than this
// component guessing from children, so each one can say WHY it is empty in its
// own comment and keep the one gap that is genuinely worth stating.
//
// InstrNote is the exception's shape: a single quiet line for a gap that IS
// worth stating (a task whose parent feature does not resolve, prose whose
// file is gone). One line, never a heading plus a parenthetical under it.

import type { ReactNode } from "react";
import "./instrument.css";

// lead is the head's left-led companion slot (node header pass, 2026-08-30):
// whatever sits here reads right after the eyebrow on the same line, above
// the same hairline, the Spec section's "from <source>" being its one tenant.
// It lives outside the h2 so the heading's accessible name stays the label
// alone, and the row exists only when a lead does, so every other section's
// head is the exact element it always was.
export function InstrHead({
  label,
  caption,
  lead,
}: {
  label: string;
  caption?: string;
  lead?: ReactNode;
}) {
  const head = (
    <h2 className="instr-fh">
      {label}
      {caption !== undefined && caption !== "" && (
        <>
          {/* The separator is its own node so the caption element holds
              exactly the caption, nothing glued to it. */}
          <span aria-hidden="true"> · </span>
          <span className="muted instr-fh-caption">{caption}</span>
        </>
      )}
    </h2>
  );
  if (lead === undefined) {
    return head;
  }
  return (
    <div className="instr-fh-row">
      {head}
      {lead}
    </div>
  );
}

export function InstrNote({ children }: { children: ReactNode }) {
  return <p className="instr-note">{children}</p>;
}

export default function InstrSection({
  label,
  caption,
  lead,
  className,
  children,
}: {
  label: string;
  caption?: string;
  lead?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={className === undefined ? "instr-section" : `instr-section ${className}`}>
      <InstrHead label={label} caption={caption} lead={lead} />
      {children}
    </section>
  );
}
