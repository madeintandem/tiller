// Explore's server-owned facets apply on pick; active values live only in
// the URL so the list is shareable and browser history restores each pick.

import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type { FilterAxis, ListFilters, ListResult } from "../../api";
import { listPath } from "../../api";
import Chip from "../../components/Chip";
import Contributors from "../../components/Contributors";
import EmptyNote from "../../components/EmptyNote";
import { useFetch } from "../../hooks";
import { withActivityPhrase } from "../../lib/activityPhrase";
import { PersonName } from "../../people";
import FilterCombobox from "./FilterCombobox";

const nounOptions = [
  { value: "epics", noun: "epic", label: "epics" },
  { value: "features", noun: "feature", label: "features" },
  { value: "tasks", noun: "task", label: "tasks" },
  { value: "questions", noun: "question", label: "questions" },
  { value: "amendments", noun: "amendment", label: "Amendments" },
  { value: "bug", noun: "bug", label: "Bugs" },
  { value: "chore", noun: "chore", label: "Chores" },
  { value: "tweak", noun: "tweak", label: "Tweaks" },
] as const;
const filterAxes: readonly FilterAxis[] = ["status", "owner", "contributor", "feature", "epic"];
const axisTitles: Partial<Record<FilterAxis, string>> = {
  owner: "Execution-unit owner or question asker",
  contributor: "Started-task contributor on an epic or feature",
};

export default function FilterMode() {
  const [params, setParams] = useSearchParams();
  const noun = params.get("noun") ?? "epics";
  const current = nounOptions.find((option) => option.value === noun || option.noun === noun || `${option.noun}s` === noun)?.noun;
  const filters: ListFilters = {};
  for (const axis of filterAxes) {
    const value = params.get(axis);
    if (value !== null && value !== "") filters[axis] = value;
  }
  const { data, error, loading } = useFetch<ListResult>(listPath(noun, filters), { retainData: true });
  // useFetch retains refreshes of an identical path. Filter picks change the
  // path, so retain the bar's last success locally, scoped strictly to noun.
  const [retained, setRetained] = useState<{ noun: string; data: ListResult | null }>({ noun, data: null });
  const fresh = data !== null && data.noun === current ? data : null;
  if (retained.noun !== noun || (fresh !== null && retained.data !== fresh)) {
    setRetained({ noun, data: fresh });
  }
  const displayed = fresh ?? (retained.noun === noun ? retained.data : null);
  const facets = displayed?.facets ?? [];

  useEffect(() => {
    // Never let retained options from an earlier query rewrite the URL.
    if (loading || fresh?.facets === undefined) return;
    const stale = filterAxes.filter((axis) => params.get(axis) &&
      !fresh.facets?.some((facet) => facet.axis === axis));
    if (stale.length === 0) return;
    const next = new URLSearchParams(params);
    for (const axis of stale) next.delete(axis);
    setParams(next, { replace: true });
  }, [fresh, loading, params, setParams]);

  function navigate(change: (next: URLSearchParams) => void) {
    const next = new URLSearchParams(params);
    next.set("mode", "filter");
    change(next);
    setParams(next);
  }

  function pick(axis: FilterAxis, value: string) {
    navigate((next) => {
      next.set(axis, value);
      if (axis === "epic" && next.get("feature")) {
        const raw = next.get("feature");
        const feature = facets.find((facet) => facet.axis === "feature")?.options
          .find((option) => option.value === raw || option.key === raw);
        if (feature?.parent !== value) next.delete("feature");
      }
    });
  }

  function switchNoun(value: string) {
    const target = nounOptions.find((option) => option.value === value)?.noun;
    const kept = target === undefined ? undefined : displayed?.axes?.[target];
    navigate((next) => {
      next.set("noun", value);
      if (kept !== undefined) {
        for (const axis of filterAxes) if (!kept.includes(axis)) next.delete(axis);
      }
    });
  }

  const anySet = filterAxes.some((axis) => filters[axis] !== undefined);
  return (
    <>
      <div className="filter-bar">
        <select className="control" aria-label="Noun" value={nounOptions.find((option) => option.noun === current)?.value ?? noun}
          onChange={(event) => switchNoun(event.currentTarget.value)}>
          {nounOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
        {facets.map((facet) => {
          const raw = displayed?.unresolved?.[facet.axis];
          // Counts can be retained while a pick is pending, but selection
          // follows the URL immediately so another pick can supersede it.
          const value = filters[facet.axis];
          const options = fresh !== null ? facet.options : facet.options.map((option) => ({
            ...option, selected: value !== undefined && (option.value === value || option.key === value),
          }));
          return <FilterCombobox key={`${noun}-${facet.axis}`} name={facet.axis} options={options}
            title={axisTitles[facet.axis]}
            note={raw === undefined || raw !== value ? undefined : `no ${facet.axis} "${raw}"`}
            onPick={(value) => pick(facet.axis, value)}
            onClear={() => navigate((next) => next.delete(facet.axis))} />;
        })}
        {anySet && <button type="button" className="filter-clear-all"
          onClick={() => navigate((next) => { for (const axis of filterAxes) next.delete(axis); })}>clear all</button>}
      </div>
      {error !== null && <p className="error">{error}</p>}
      {error === null && loading && fresh === null && <p className="muted">Loading list...</p>}
      {fresh !== null && <Results result={fresh} />}
    </>
  );
}

function Results({ result }: { result: ListResult }) {
  if (result.nodes.length === 0) {
    return <EmptyNote>No matches.</EmptyNote>;
  }
  return (
    <>
      <p className="muted">
        {result.count} {result.count === 1 ? "result" : "results"}
      </p>
      <ul className="rule-list explore-results">
        {result.nodes.map((row) => (
          <li key={row.id} className="explore-result-row">
            {/* Title leads. A materialized question keeps its Q-id beside it
                as the keyless noun's address; keyed nouns retain their key in
                that same muted mono slot. */}
            <Link
              to={`/node/${result.noun}/${row.id}`}
              className="explore-result-title"
              title={row.key}
            >
              {row.title || row.intent || row.ref}
            </Link>
            {result.noun === "question" && row.title !== undefined && row.title !== "" ? (
              <span className="mono muted explore-result-key">{row.id}</span>
            ) : (
              row.key !== undefined &&
              row.key !== "" && <span className="mono muted explore-result-key">{row.key}</span>
            )}
            <Chip status={row.status}>{withActivityPhrase(row)}</Chip>
            {result.noun === "epic" || result.noun === "feature" ? (
              <span className="muted explore-result-owner">Contributors: <Contributors people={row.contributors ?? []} /></span>
            ) : row.owner !== undefined && row.owner !== "" && (
              <span className="muted explore-result-owner">Owner: <PersonName id={row.owner} name={row.owner_name} /></span>
            )}
          </li>
        ))}
      </ul>
    </>
  );
}

