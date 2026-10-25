// SourceLink renders a node's recorded source citation as the one-hop link
// the node header pass (2026-08-30) settled on: a repo-relative path opens
// the console's own file page (lib/sourceHref.ts owns the address, its line
// locator included), and a URL opens itself in a new tab. The text is the
// citation verbatim, so what the reader clicks is exactly what the record
// says. The Spec section's head is its home on every prose-bearing noun;
// the question page, which has no Spec section, keeps it in the header.
import { Link } from "react-router-dom";
import type { RelationshipDiagnostic } from "../api";
import CopyCmd from "./CopyCmd";
import { sourceHref } from "../lib/sourceHref";

export default function SourceLink({ source, diagnostics }: { source: string; diagnostics: RelationshipDiagnostic[] }) {
  const target = sourceHref(source);
  const path = source.split("#", 1)[0];
  const unavailable = diagnostics.some((diagnostic) =>
    ((diagnostic.code === "unresolved_document" || diagnostic.code === "document_unreadable") && diagnostic.source?.file === path)
    || (diagnostic.code === "unresolved_citation" && diagnostic.source?.raw === source)
    || (diagnostic.code === "corpus_unreadable" && !target.external),
  );
  if (unavailable) return <CopyCmd cmd={source} />;
  if (target.external) {
    return (
      <a className="instr-source-link mono" href={target.href} target="_blank" rel="noreferrer">
        {source}
      </a>
    );
  }
  return (
    <Link className="instr-source-link mono" to={target.href}>
      {source}
    </Link>
  );
}
