// The rows and sections of Explore search (SearchMode.tsx). A node row leads
// with its title, the thing a reader recognizes, then its key (or a
// question's Q-id), its status, and how it matched; its first few matching
// lines sit beneath, each named by the file and line it came from. A
// document row is the file itself, with its lines linking to the file page.

import { useState } from "react";
import { Link } from "react-router-dom";
import type { SearchDocumentResult, SearchHit, SearchNodeResult, SearchResult } from "../../api";
import Chip from "../../components/Chip";
import SectionHead from "../../components/SectionHead";
import { has } from "../../lib/has";
import { kindLabel } from "../../lib/kind";

// The kinds a result can have, in the API's order (search.Kinds).
export const SEARCH_KINDS = [
  "epic", "feature", "task", "bug", "chore", "tweak", "question", "amendment", "document",
];

// How many rows one kind's section shows on the All tab before pointing at
// the kind's own tab, and how many a kind tab draws before its expander.
const SECTION_CAP = 5;
const ROW_CAP = 25;

// The All tab is an overview across kinds, so its rows are compact: one
// matching line, with the row's total on its head line. A kind tab is where
// the reader reads, so its rows carry every line the API kept.
const COMPACT_HITS = 1;

export function kindPlural(kind: string): string {
  return `${kindLabel(kind)}s`;
}

// An exact match is a lookup the reader made on purpose: the query was the
// node's id, reference, or key.
function isExact(n: SearchNodeResult): boolean {
  return n.match === "id" || n.match === "key";
}

export function AllResults({
  result,
  onShowKind,
}: {
  result: SearchResult;
  onShowKind: (kind: string) => void;
}) {
  const exact = result.nodes.filter(isExact);
  return (
    <>
      {exact.length > 0 && (
        <section aria-label="Exact match" className="search-section search-exact">
          <SectionHead label="Exact match" />
          <ul className="rule-list search-results">
            {exact.map((n) => (
              <NodeRow key={n.id} node={n} query={result.query} showKind />
            ))}
          </ul>
        </section>
      )}
      {result.kinds.map(({ kind, count }) => {
        const rows =
          kind === "document"
            ? result.documents.map((d) => <DocumentRow key={d.path} doc={d} query={result.query} compact />)
            : result.nodes
                .filter((n) => n.noun === kind && !isExact(n))
                .map((n) => <NodeRow key={n.id} node={n} query={result.query} compact />);
        const exactHere = result.nodes.filter((n) => n.noun === kind && isExact(n)).length;
        if (count - exactHere === 0) return null;
        const shown = rows.slice(0, SECTION_CAP);
        const hidden = count - exactHere - shown.length;
        return (
          <section key={kind} aria-label={kindPlural(kind)} className="search-section">
            <SectionHead label={`${kindPlural(kind)} · ${String(count)}`} />
            <ul className="rule-list search-results">{shown}</ul>
            {hidden > 0 && (
              <button type="button" className="search-hits-more" onClick={() => onShowKind(kind)}>
                All {count} {kindPlural(kind).toLowerCase()} ▸
              </button>
            )}
          </section>
        );
      })}
    </>
  );
}

export function KindResults({ result, kind }: { result: SearchResult; kind: string }) {
  const [expanded, setExpanded] = useState(false);
  const rows =
    kind === "document"
      ? result.documents.map((d) => <DocumentRow key={d.path} doc={d} query={result.query} />)
      : result.nodes.map((n) => <NodeRow key={n.id} node={n} query={result.query} />);
  const shown = expanded ? rows : rows.slice(0, ROW_CAP);
  const hidden = rows.length - shown.length;
  return (
    <>
      <ul className="rule-list search-results">{shown}</ul>
      {hidden > 0 && (
        <button type="button" className="search-hits-more" onClick={() => setExpanded(true)}>
          {hidden} more {hidden === 1 ? "result" : "results"} ▸
        </button>
      )}
    </>
  );
}

const MATCH_LABEL: Record<string, string> = {
  id: "ID match",
  key: "Key match",
  key_fragment: "Key contains",
};

function NodeRow({
  node,
  query,
  showKind = false,
  compact = false,
}: {
  node: SearchNodeResult;
  query: string;
  showKind?: boolean;
  compact?: boolean;
}) {
  // Keyed nodes keep their key in the muted address slot; a question, which
  // has no key, keeps its Q-id there instead.
  const address = has(node.key) ? node.key : node.id;
  const lookup = MATCH_LABEL[node.match];
  const total = compact && node.count > 0 ? matchCount(node.count) : undefined;
  const label = [lookup, total].filter((part) => part !== undefined).join(" · ");
  return (
    <li className="search-result">
      <div className="search-result-head">
        {showKind && <span className="kind-pill search-kind">{kindLabel(node.noun)}</span>}
        <Link to={`/node/${node.noun}/${node.id}`} className="search-result-title">
          <Highlight text={has(node.title) ? node.title : node.ref} query={query} />
        </Link>
        <span className="mono muted search-result-key">{address}</span>
        <Chip status={node.status} />
        {label !== "" && <span className="muted search-result-match">{label}</span>}
      </div>
      <Hits hits={node.hits} count={node.count} query={query} compact={compact} />
    </li>
  );
}

function DocumentRow({ doc, query, compact = false }: { doc: SearchDocumentResult; query: string; compact?: boolean }) {
  const first = doc.hits[0]?.line ?? 1;
  return (
    <li className="search-result">
      <div className="search-result-head">
        <Link to={fileRoute(doc.path, first)} className="mono search-result-title search-doc-title">
          {doc.path.replace(/^\.tiller\//, "")}
        </Link>
        <span className="muted search-result-match">{matchCount(doc.count)}</span>
      </div>
      <Hits hits={doc.hits} count={doc.count} query={query} compact={compact} linkLines />
    </li>
  );
}

// The node's or document's kept lines, each named by where it came from: the
// file's own name and line for prose (the row already says whose file it
// is), or the ledger field for a question's statement or answer. A compact
// row shows one line and leaves the total to its head line.
function Hits({
  hits,
  count,
  query,
  compact,
  linkLines = false,
}: {
  hits: SearchHit[];
  count: number;
  query: string;
  compact: boolean;
  linkLines?: boolean;
}) {
  const shown = compact ? hits.slice(0, COMPACT_HITS) : hits;
  const more = compact ? 0 : count - shown.length;
  return (
    <>
      {shown.map((hit) => (
        <p key={`${hit.path}:${String(hit.line)}:${hit.field ?? ""}`} className="hit-snippet">
          {linkLines ? (
            <Link to={fileRoute(hit.path, hit.line)} className="mono hit-where" aria-label={`line ${String(hit.line)}`}>
              :{hit.line}
            </Link>
          ) : (
            <span className="mono hit-where">{has(hit.field) ? hit.field : `${basename(hit.path)}:${String(hit.line)}`}</span>
          )}
          <Highlight text={hit.snippet} query={query} />
        </p>
      ))}
      {more > 0 && (
        <p className="muted search-result-more">
          +{more} more {more === 1 ? "match" : "matches"}
        </p>
      )}
    </>
  );
}

function matchCount(n: number): string {
  return n === 1 ? "1 match" : `${String(n)} matches`;
}

function basename(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

// The browser route keeps slashes as separators and encodes each path segment
// independently, so spaces, fragments, and other reserved characters cannot
// change the destination's route or hash.
function fileRoute(path: string, line: number): string {
  const encoded = path.split("/").map(encodeURIComponent).join("/");
  return `/file/${encoded}#L${String(line)}`;
}

// The first case-insensitive occurrence of the query gets a mark. Building
// the pieces as React nodes keeps the text plain; nothing is interpreted.
function Highlight({ text, query }: { text: string; query: string }) {
  const at = text.toLowerCase().indexOf(query.toLowerCase());
  if (query === "" || at === -1) {
    return <>{text}</>;
  }
  return (
    <>
      {text.slice(0, at)}
      <mark>{text.slice(at, at + query.length)}</mark>
      {text.slice(at + query.length)}
    </>
  );
}
