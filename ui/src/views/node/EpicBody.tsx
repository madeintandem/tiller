// The epic body (ui-surface.md: "an epic shows its features, its epic.eval,
// and the rollup"): the epic's prose, its features as a ledger-style list
// with a mixed-child StateBar over them, the authored epic.eval contract
// decorated with the recorded gate state, provenance, and integrity notes.
//
// The epic identity double-render the earlier phase accepted as a quirk (an
// EpicSummary line repeating id/key/title/status above the features table,
// specifically so the queue-expansion body was not left with no identity at
// all) is resolved here by removing that summary line entirely: NodeDetail's
// own header now renders identity exactly once for every consumer, including
// a future queue-expansion reuse of the full NodeDetail rather than a bare
// body, so this body only ever needs to render what it uniquely owns.
//
// When the payload carries a decomposition (the read model's epic projection)
// the Features section renders every child feature in payload (canonical
// key) order; an older server falls back to the relationships.features link
// list via the shared RelatedList. The eval contract renders before the
// recorded gates so the human reads what was authored before what was
// checked against it. Features are this body's composition edge, so the
// Dependencies tab's hub skips them (NodeDetail's claimedGroups, unchanged
// by this split).
//
// the tab shell splits this file into the three panels the
// epic tab set names on its own - spec/evals/features - moved here JSX for
// JSX from the one body function they used to share; the other two of an
// epic's five tabs (dependencies, history) live in NodeDetail.tsx, same as
// every other noun's.

import { useState } from "react";
import { Link } from "react-router-dom";
import type { NodePage, NodeView } from "../../api";
import Chip from "../../components/Chip";
import { GateCardList } from "../../components/GateCard";
import InstrSection, { InstrNote } from "../../components/InstrSection";
import IntegrityNotes from "../../components/IntegrityNotes";
import ProvenanceLine from "../../components/ProvenanceLine";
import StateBar from "../../components/StateBar";
import { has } from "../../lib/has";
import { composeStat } from "../home/stat";
import ProseSection from "./ProseSection";
import RelatedList from "./RelatedList";

export function EpicSpecPanel({ page }: { page: NodePage }) {
  return <ProseSection prose={page.prose} label="Spec" node={page.node} diagnostics={page.backlinks.diagnostics} />;
}

export function EpicFeaturesPanel({ page }: { page: NodePage }) {
  const features = page.relationships.features ?? [];
  if (page.decomposition !== undefined) {
    return <Decomposition features={page.decomposition.features} />;
  }
  if (features.length === 0) {
    // An epic with no children at all is worth one line: undecomposed is a
    // real state a reader acts on, not an absence to pass over in silence.
    return <InstrNote>No features carved yet</InstrNote>;
  }
  return (
    <InstrSection label="Features" caption={`${features.length}`}>
      <RelatedList items={features} />
    </InstrSection>
  );
}

export function EpicEvalsPanel({ page }: { page: NodePage }) {
  const node = page.node;
  return (
    <>
      <GateCardList node={node} evalPayload={page.eval} />
      <ProvenanceLine
        against={node.gates?.evals?.against}
        pr={node.pr}
        prUrl={node.pr_url}
        reviewRef={node.review_ref}
        violations={node.policy_violations ?? []}
      />
      <IntegrityNotes attestations={node.attestations ?? []} violations={node.policy_violations ?? []} />
    </>
  );
}

// ROW_CAP bounds how many feature rows draw at once, DecisionCard's own
// disclosure: the true count rides the head, the rest are one click away with
// data already in hand. It never fires on today's fixtures (the largest epic
// carves eight); it exists so a twenty-feature epic degrades the way the rest
// of the console does instead of growing without limit.
const ROW_CAP = 12;

// splitAtCap caps in payload order and never hides a feature past draft. A
// specced or in_review child is exactly what a human scanning an epic is
// looking for, so only untouched drafts fold; that guard is what makes the cap
// safe rather than merely short.
function splitAtCap(features: NodeView[]): { shown: NodeView[]; hidden: NodeView[] } {
  if (features.length <= ROW_CAP) {
    return { shown: features, hidden: [] };
  }
  const shown: NodeView[] = [];
  const hidden: NodeView[] = [];
  features.forEach((f, i) => {
    if (i < ROW_CAP || f.status !== "draft") {
      shown.push(f);
    } else {
      hidden.push(f);
    }
  });
  return { shown, hidden };
}

// Decomposition is the mixed-child StateBar plus the ledger-style feature
// list, in exactly payload (canonical key) order, never re-sorted. An epic
// whose decomposition is empty draws no bar over no rows: an empty track is
// chrome, and the one-line note says the whole truth.
function Decomposition({ features }: { features: NodeView[] }) {
  const [expanded, setExpanded] = useState(false);
  // composeStat always sees every feature, capped rows or not, so the bar's
  // mix and its caption stay the whole truth about the epic's children.
  const { mix, caption } = composeStat(features);
  const { shown, hidden } = splitAtCap(features);
  if (features.length === 0) {
    return <InstrNote>No features carved yet</InstrNote>;
  }
  const rows = expanded ? features : shown;
  return (
    <InstrSection label={`Features · ${features.length}`} caption={caption}>
      <div className="instr-decomp-bar">
        <StateBar mix={mix} caption={caption} width={220} />
      </div>
      <div className="instr-decomp">
        {rows.map((feature) => (
          <FeatureRow key={feature.id} feature={feature} />
        ))}
      </div>
      {/* Expands in place from the list already fetched, DecisionCard's
          grammar: no refetch, and the head above already stated the true
          count. */}
      {!expanded && hidden.length > 0 && (
        <button type="button" className="instr-more" onClick={() => setExpanded(true)}>
          {hidden.length} more {hidden.length === 1 ? "feature" : "features"} ▸
        </button>
      )}
    </InstrSection>
  );
}

// ONE LINE PER NODE, the locked list convention, in the same grammar Explore's
// filter rows use: the title leads and is the link, the key trails as muted
// mono at the margin, the Chip names the row's own state because a mixed-status
// list is exactly the case a distant caption cannot disambiguate row by row.
//
// The row used to stack four to seven lines: title, key, `change_type · source`,
// then one line per surface path and per module. Eight features became fifty
// lines. Every one of those facts still reads on the feature's OWN page, where
// detail belongs under the console's hierarchy inversion: change type, source,
// and owner in NodeDetail's dmeta caption, and the surface axes in the surface
// line beside it (moved there by this pass precisely so nothing was dropped
// when this row was cut down).
function FeatureRow({ feature }: { feature: NodeView }) {
  const label = has(feature.title) ? feature.title : (feature.key ?? feature.id);
  return (
    <div className="instr-decomp-row">
      <Link
        to={`/node/${feature.noun}/${feature.id}`}
        className="instr-decomp-title"
        title={feature.key}
      >
        {label}
      </Link>
      {has(feature.key) && <span className="mono muted instr-decomp-key">{feature.key}</span>}
      <Chip status={feature.status} />
    </div>
  );
}
