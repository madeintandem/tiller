// Search is the console face of `tiller search` (ui-surface.md, "Explore").
// The query and the kind tab live in the URL (?q=, ?kind=), so a search is
// linkable; an empty query fetches nothing, because a query that constrains
// nothing answers nothing.
//
// The API answers organized results, not raw lines: one row per node, ranked
// by how it matched, and one per unowned document (internal/search). This
// page adds the shape a reader scans. A node the query names by id or key
// stands alone at the top, the rest group under their kind, and a tab per
// kind carries its count. The All tab caps each kind's section, and the kind
// tab asks the server for that kind alone, so a kind is never cut short by
// the others' share of the page.

import { useMemo, useReducer, useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { SearchKindCount, SearchResult } from "../../api";
import { searchPath } from "../../api";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import { useFetch } from "../../hooks";
import { readHideCancelled, writeHideCancelled } from "./hideCancelled";
import { AllResults, KindResults, kindPlural, SEARCH_KINDS } from "./SearchResults";

const ALL = "all";

export default function SearchMode() {
  const [params, setParams] = useSearchParams();
  const query = params.get("q") ?? "";
  const raw = params.get("kind") ?? "";
  // A kind the console does not know reads as All, so a stale or hand-edited
  // link still answers rather than tripping the API's misuse refusal.
  const kind = SEARCH_KINDS.includes(raw) ? raw : ALL;
  const { data: answer, error, loading } = useFetch<SearchResult>(
    query === "" ? null : searchPath(query, kind === ALL ? undefined : kind),
  );
  // The navigator's Hide cancelled preference applies here too, and the
  // summary counts what it took out so a filtered list never reads as whole.
  // Read from storage on every render rather than frozen at mount; showing
  // the hidden rows writes the same preference the navigator's toggle reads.
  const [, rerender] = useReducer((n: number) => n + 1, 0);
  const hideCancelled = readHideCancelled();
  function showCancelled() {
    writeHideCancelled(false);
    rerender();
  }
  // Memoized: the kinds array's identity is how the rail's stand-in below
  // notices a new answer, so a fresh copy every render would loop.
  const { result: data, hidden } = useMemo(
    () => (hideCancelled ? withoutCancelled(answer) : { result: answer, hidden: 0 }),
    [answer, hideCancelled],
  );

  // Switching tabs refetches, and the tab rail should not blink out while it
  // does: every answer for one query carries the same kinds, so the last ones
  // seen for this query stand in until the new page lands.
  const [kinds, setKinds] = useState<{ query: string; kinds: SearchKindCount[] } | null>(null);
  if (data !== null && data.query === query && kinds?.kinds !== data.kinds) {
    setKinds({ query, kinds: data.kinds });
  }
  const shownKinds = kinds?.query === query ? kinds.kinds : null;
  const fresh = data !== null && data.query === query && (data.kind ?? ALL) === kind ? data : null;

  function selectKind(next: string) {
    const updated = new URLSearchParams(params);
    if (next === ALL) updated.delete("kind");
    else updated.set("kind", next);
    setParams(updated);
  }

  return (
    <>
      {query === "" && <p className="muted">Type a keyword, id, or key and press Enter.</p>}
      {error !== null && <p className="error">{error}</p>}
      {shownKinds !== null && shownKinds.length === 0 && fresh !== null && (
        <>
          <EmptyNote>No matches.</EmptyNote>
          {hidden > 0 && <HiddenNote hidden={hidden} onShow={showCancelled} />}
        </>
      )}
      {shownKinds !== null && shownKinds.length > 0 && (
        <NodeTabs
          label="Result kind"
          tabs={[
            { key: ALL, label: "All", count: shownKinds.reduce((sum, k) => sum + k.count, 0) },
            ...shownKinds.map((k) => ({ key: k.kind, label: kindPlural(k.kind), count: k.count })),
          ].map((tab) => ({
            ...tab,
            render: () =>
              fresh === null ? (
                <p className="muted">Searching...</p>
              ) : (
                <>
                  <Summary result={fresh} hidden={hidden} onShowHidden={showCancelled} />
                  {kind === ALL ? (
                    <AllResults result={fresh} onShowKind={selectKind} />
                  ) : (
                    <KindResults result={fresh} kind={kind} />
                  )}
                </>
              ),
          }))}
          active={kind}
          onSelect={selectKind}
        />
      )}
      {loading && shownKinds === null && <p className="muted">Searching...</p>}
    </>
  );
}

// Count is the number of results the query matched within the tab, which the
// page may hold fewer of when the API's ceiling stands in front of it. A
// truncated answer that poses as complete is worse than a long one.
function Summary({
  result,
  hidden,
  onShowHidden,
}: {
  result: SearchResult;
  hidden: number;
  onShowHidden: () => void;
}) {
  const shown = result.nodes.length + result.documents.length;
  return (
    <p className="muted search-summary">
      {result.count} {result.count === 1 ? "result" : "results"}
      {result.truncated && ` · showing ${String(shown)}; narrow the query or pick a kind to see the rest`}
      {hidden > 0 && (
        <>
          {" · "}
          <HiddenNote hidden={hidden} onShow={onShowHidden} inline />
        </>
      )}
    </p>
  );
}

// Names the cause of the filtering and the way out of it.
function HiddenNote({ hidden, onShow, inline = false }: { hidden: number; onShow: () => void; inline?: boolean }) {
  const text = `${String(hidden)} cancelled ${hidden === 1 ? "result" : "results"} hidden by Hide cancelled.`;
  const body = (
    <>
      {text}{" "}
      <button type="button" className="search-hits-more" onClick={onShow}>
        Show them
      </button>
    </>
  );
  return inline ? <span>{body}</span> : <p className="muted search-summary">{body}</p>;
}

// withoutCancelled drops the cancelled node rows the page holds and takes
// them out of the totals, returning how many it dropped. A kind left with
// nothing leaves the tab rail.
function withoutCancelled(result: SearchResult | null): { result: SearchResult | null; hidden: number } {
  if (result === null) {
    return { result, hidden: 0 };
  }
  const dropped = result.nodes.filter((n) => n.status === "cancelled");
  if (dropped.length === 0) {
    return { result, hidden: 0 };
  }
  const perKind = new Map<string, number>();
  for (const n of dropped) perKind.set(n.noun, (perKind.get(n.noun) ?? 0) + 1);
  return {
    hidden: dropped.length,
    result: {
      ...result,
      count: result.count - dropped.length,
      nodes: result.nodes.filter((n) => n.status !== "cancelled"),
      kinds: result.kinds
        .map((k) => ({ ...k, count: k.count - (perKind.get(k.kind) ?? 0) }))
        .filter((k) => k.count > 0),
    },
  };
}
