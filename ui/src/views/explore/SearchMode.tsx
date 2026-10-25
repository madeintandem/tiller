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

import type { FormEvent } from "react";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { SearchKindCount, SearchResult } from "../../api";
import { searchPath } from "../../api";
import EmptyNote from "../../components/EmptyNote";
import NodeTabs from "../../components/NodeTabs";
import { useFetch } from "../../hooks";
import { AllResults, KindResults, kindPlural, SEARCH_KINDS } from "./SearchResults";

const ALL = "all";

export default function SearchMode() {
  const [params, setParams] = useSearchParams();
  const query = params.get("q") ?? "";
  const raw = params.get("kind") ?? "";
  // A kind the console does not know reads as All, so a stale or hand-edited
  // link still answers rather than tripping the API's misuse refusal.
  const kind = SEARCH_KINDS.includes(raw) ? raw : ALL;
  const { data, error, loading } = useFetch<SearchResult>(
    query === "" ? null : searchPath(query, kind === ALL ? undefined : kind),
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

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = new FormData(event.currentTarget).get("q");
    const next = new URLSearchParams(params);
    next.set("mode", "search");
    next.delete("kind");
    if (typeof value === "string" && value.trim() !== "") {
      next.set("q", value.trim());
    } else {
      next.delete("q");
    }
    setParams(next);
  }

  function selectKind(next: string) {
    const updated = new URLSearchParams(params);
    if (next === ALL) updated.delete("kind");
    else updated.set("kind", next);
    setParams(updated);
  }

  return (
    <>
      {/* One input, submit on Enter; the key remounts it when the URL query
          changes so the back button restores what was searched. */}
      <form className="filter-bar" key={query} onSubmit={submit}>
        <input
          className="control search-input"
          type="text"
          name="q"
          defaultValue={query}
          placeholder="keyword, id, or key"
          aria-label="Search keyword"
        />
      </form>
      {query === "" && <p className="muted">Type a keyword, id, or key and press Enter.</p>}
      {error !== null && <p className="error">{error}</p>}
      {shownKinds !== null && shownKinds.length === 0 && fresh !== null && <EmptyNote>No matches.</EmptyNote>}
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
                  <Summary result={fresh} />
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
function Summary({ result }: { result: SearchResult }) {
  const shown = result.nodes.length + result.documents.length;
  return (
    <p className="muted search-summary">
      {result.count} {result.count === 1 ? "result" : "results"}
      {result.truncated && ` · showing ${String(shown)}; narrow the query or pick a kind to see the rest`}
    </p>
  );
}
