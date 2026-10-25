// A node's prose file rendered read-only (the no-authoring rule): the
// repo-relative path in mono as the drill pointer, the content typeset through
// Prose (task B2's markdown renderer) rather than dumped as pre-wrapped plain
// text.
//
// Three states, three registers, because a page that renders all three the
// same way lies twice:
//
// 1. STILL THE ADD STUB. A freshly added feature's feature.md measured 279px
//    of "Draft feature created by `tiller add feature`..." over three empty
//    headings, and every one of the human's 70 features was in that state. It
//    looked like content, which is worse than showing nothing, so it collapses
//    to one quiet caption that says the spec is unwritten and names the move
//    that fixes it. The caption is both shorter AND more informative than what
//    it replaces. The stub verdict comes from the server (ProseRef.stub,
//    mutate.IsProseUnwritten), never from matching the boilerplate here: a
//    second copy of a sentinel is a second thing to drift. That predicate is
//    deliberately stricter than the one `tiller spec feature` refuses on,
//    because this branch draws NONE of the file: a spec written into the
//    stub's own headings, with the generated paragraph left in place, is real
//    prose and renders in full.
// 2. MISSING OR UNREADABLE. Content the handler could not read at all, which
//    is a different fact from an unwritten spec: the file the tree points at
//    is gone. One quiet line, still naming the path, no section chrome around
//    it. A file that IS there and is empty reads as unwritten, not missing,
//    because the server distinguishes them and case 1 is the truer answer.
// 3. WRITTEN. The section proper, rendered whole (task 17, console v2 node
//    page rebuild: "Prose renders whole: no read-more folds and no truncated
//    bodies, because the tabs already bound the page and a spec you must
//    unfold is a spec that goes unread" - the epic-spec-pushes-Features-
//    below-the-fold problem the earlier clamp answered is now the tabs' own
//    job, not this section's). Its opening `# Title` is still dropped when it
//    only repeats the h1 six lines above it (stripLeadingTitle), and prose.css
//    keeps the render at a 72ch reading measure inside the pane rather than
//    running the full window's width.

import type { ReactNode } from "react";
import type { NodeView, ProseRef, RelationshipDiagnostic } from "../../api";
import CopyCmd from "../../components/CopyCmd";
import InstrSection, { InstrNote } from "../../components/InstrSection";
import Prose from "../../components/Prose";
import SourceLink from "../../components/SourceLink";
import { addrOf } from "../../lib/addr";
import { has } from "../../lib/has";
import { stripLeadingTitle } from "../../lib/stripLeadingTitle";

export default function ProseSection({
  prose,
  label,
  node,
  diagnostics = [],
}: {
  prose: ProseRef | null;
  label: string;
  node: NodeView;
  diagnostics?: RelationshipDiagnostic[];
}) {
  if (prose === null) {
    return null;
  }
  // The server sets stub only on a file it actually read, so an
  // empty-but-present file reads as unwritten. Only a file the handler could
  // not read falls through to the missing line. Both compact states retain
  // source provenance without restoring full section chrome.
  if (prose.stub === true) {
    return <UnwrittenLine prose={prose} label={label} node={node} diagnostics={diagnostics} />;
  }
  if (prose.content === "") {
    return (
      <InstrNote>
        {label} file missing or unreadable · <span className="mono">{prose.path}</span>
        {has(node.source) && <> · {sourceLead(node, diagnostics)}</>}
      </InstrNote>
    );
  }
  // The path line that used to lead the section is gone (node header pass,
  // 2026-08-30): the ref line in the header already addresses the node, and
  // the head's lead slot now carries what a reader actually follows, the
  // source the prose came from, as a one-hop link (SourceLink). A node with
  // no source gets the bare head, so the prose simply begins.
  return (
    <InstrSection label={label} lead={sourceLead(node, diagnostics)}>
      <Prose markdown={stripLeadingTitle(prose.content, node.title)} />
    </InstrSection>
  );
}

function sourceLead(node: NodeView, diagnostics: RelationshipDiagnostic[]): ReactNode {
  if (!has(node.source)) {
    return undefined;
  }
  return (
    <span className="instr-source">
      from <SourceLink source={node.source} diagnostics={diagnostics} />
    </span>
  );
}

interface Move {
  who: string;
  command?: string;
}

// moveFor names what closes an unwritten spec, by noun, and refuses to invent
// one where the CLI has none. A feature has an exact answer: the agent writes
// feature.md and asserts it with `tiller spec feature`, the draft -> specced
// transition, whose actor is ActorAgent in internal/core/lifecycle/table.go.
// That is read straight from the lifecycle table rather than from next_owner,
// because next_owner answers a different question (the node's next LIFECYCLE
// gate) and would say "human" on a draft epic whose approval is pending while
// the prose is still the agent's job. An epic has no spec verb at all: epic.md
// is written in place and the bar is only read at the rollup, so the epic's
// line names what to write and stops, the same honesty as RefLink's "a link to
// nowhere is worse than no link".
//
// A task is the same shape as an epic and for the same reason: there is no
// `tiller spec task`. A task.md is written in place, and the gate that reads it
// is one grain up, `tiller spec feature`, which refuses until at least one of a
// feature's tasks carries a written implementation spec (mutate's
// unwrittenTaskSpecs). Naming that feature's command here would hand a reader a
// verb addressed at some other node, so the line names what to write instead:
// the headings `tiller add task` generated, which are what the spec has to say.
function moveFor(node: NodeView): Move | null {
  if (node.noun === "feature") {
    return { who: "the agent's move", command: `tiller spec feature ${addrOf(node)}` };
  }
  if (node.noun === "epic") {
    return { who: "write the intent, scope, and definition of done" };
  }
  if (node.noun === "task") {
    return { who: "write the approach, surfaces, tests, and edge cases" };
  }
  return null;
}

function UnwrittenLine({
  prose,
  label,
  node,
  diagnostics,
}: {
  prose: ProseRef;
  label: string;
  node: NodeView;
  diagnostics: RelationshipDiagnostic[];
}) {
  const move = moveFor(node);
  return (
    <div className="instr-unwritten">
      <span className="instr-unwritten-say">No {label.toLowerCase()} written yet</span>
      <span className="mono">{prose.path}</span>
      {sourceLead(node, diagnostics)}
      {move !== null && <span>{move.who}</span>}
      {move?.command !== undefined && <CopyCmd cmd={move.command} />}
    </div>
  );
}
