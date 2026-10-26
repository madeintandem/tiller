// NodeHeader is the node page's header, rendered exactly once above the
// attention stack, the drift panel, and the tabs by NodeDetail, its one
// consumer (nodeheader.test.tsx mounts it through NodeDetail, so it is only
// ever tested in its real seat). One fixed chain of slots on every noun
// (ui-surface.md's node-detail bullet, amended 2026-08-30 by the node header
// pass): the ancestor crumbs; a kind line with the noun on a solid pill, the
// node's own state at the hero size, the gate ticks, and the drift/held/grain
// chips; the title alone; the mono reference line with its copy affordance;
// the children's mix on an epic or a feature; then one labeled facts block
// under the header's one divider. It was split out of NodeDetail.tsx once
// that file crossed the repo's thousand-line line; every piece here is
// reachable from NodeDetailHeader alone.

import { Fragment, type ReactNode } from "react";
import type { DriftBadgeView, HeldView, NodePage, NodeView, RelatedNode } from "../api";
import { withActivityPhrase } from "../lib/activityPhrase";
import { gateChipsFor } from "../lib/gateChips";
import { has } from "../lib/has";
import { kindLabel } from "../lib/kind";
import { shortRev } from "../lib/revision";
import { PersonName } from "../people";
import { composeStat } from "../views/home/stat";
import Chip from "./Chip";
import Contributors from "./Contributors";
import { CopyRef } from "./CopyCmd";
import RefLink from "./RefLink";
import SourceLink from "./SourceLink";
import StateBar from "./StateBar";

// handleOf is the node's bounded shell handle: a title where one exists, then
// a task's intent, then the key-or-id address. Complete question statements
// belong to QuestionBody and never become a shell-title fallback. Exported so
// the node page's browser tab title (views/NodeDetail.tsx) says the same
// thing the heading does.
export function handleOf(node: NodeView): string {
  if (has(node.title)) {
    return node.title;
  }
  if (has(node.intent)) {
    return node.intent;
  }
  return node.key ?? node.id;
}

// crumbLabel is a crumb's own display text: the title where one exists, the
// key otherwise, never the ref - a breadcrumb names ancestors the way a
// reader thinks of them, the same title-leads convention every other rendered
// row on the console already carries (ui-surface.md's reference-convention
// bullet).
function crumbLabel(node: RelatedNode): string {
  return has(node.title) ? node.title : (node.key ?? node.ref);
}

// Crumbs renders the ancestor chain itself: an epic crumb above a feature,
// epic then feature above a task (ui-surface.md: "the ancestor chain is the
// breadcrumb itself"), each crumb a ref link with its own status chip, a
// separator between crumbs and none after the last. A task whose recorded parent does not resolve in the
// tree shows the dangling-parent caption in the crumb's place instead of a
// chain with a hole in it - the same guard the earlier ParentChain kept, now
// read as a breadcrumb. A feature's own epic link can dangle exactly the
// same way and never earns a note either; that asymmetry is deliberate, not
// an oversight (task 2's own precedent). An epic and a question never carry
// either relationship field, so this draws nothing at all for them: no
// chain, and no empty row standing in for one.
function Crumbs({ page }: { page: NodePage }) {
  const rel = page.relationships;
  if (page.node.noun === "task" && rel.feature === undefined) {
    return (
      <div className="crumbs">
        <span className="muted">Parent feature not found</span>
      </div>
    );
  }
  const parents = [rel.epic, rel.feature].filter((p): p is RelatedNode => p !== undefined);
  if (parents.length === 0) {
    return null;
  }
  return (
    <div className="crumbs">
      {parents.map((parent, i) => (
        <Fragment key={`${parent.noun}-${parent.id || parent.ref}-${i}`}>
          <RefLink node={parent} label={crumbLabel(parent)} className="" />
          <Chip status={parent.status} />
          {/* A separator only between crumbs, never after the last: the
              chain ends on the ancestor, and the node's own kind line and
              title follow beneath (node header pass, 2026-08-30). */}
          {i < parents.length - 1 && (
            <span className="sep" aria-hidden="true">
              ›
            </span>
          )}
        </Fragment>
      ))}
    </div>
  );
}

// DriftedChip is the node-page chip ui-surface.md's node-page-chip sentence
// describes: one chip in the header regardless of how many markers touch
// this node, whose palette register states whether any of them actually
// refuses a verb right now (the blocked register) or not (the neutral
// register the stale gates already use - GateCard's own stale case reads
// "draft" through deriveFlatGateState/deriveEvalsGateState (lib/gateDisplay.ts),
// rendered by GateHeader through StateDot, never a Chip at all; the register
// is the same "draft" word, just reached by a sibling path rather than this
// one). The register itself is never re-derived here: the server's views
// package is the one place the rule lives (internal/views/nodepage.go's
// driftRefusing), and
// this component only folds the refusing bits it already received into
// Chip's existing status prop, the same total statusColor mapping every
// other chip on the page already paints through. The title attribute lists
// every marker, since the one chip stands in for all of them.
//
// The chip is a disclosure now (JC's ask, 2026-08-06: "I intuitively wanted
// to click the Drifted tag"): a real button carrying aria-expanded and a
// chevron, toggling the DriftPanel the instrument renders above the tabs.
// The Chip span inside stays presentational; the button owns the click and
// the tooltip.
function DriftedChip({
  badges,
  expanded,
  onToggle,
}: {
  badges: DriftBadgeView[] | undefined;
  expanded: boolean;
  onToggle: () => void;
}) {
  const rows = badges ?? [];
  if (rows.length === 0) {
    return null;
  }
  const status = rows.some((b) => b.refusing) ? "blocked" : "draft";
  return (
    <button
      type="button"
      className="instr-drift-chip"
      aria-expanded={expanded}
      title={rows.map((b) => b.marker).join(", ")}
      onClick={onToggle}
    >
      <Chip status={status}>Drifted</Chip>
      <span className="chevron-glyph" aria-hidden="true">
        {expanded ? "▾" : "▸"}
      </span>
    </button>
  );
}

// HeldChip is the node-page's other always-blocked-register chip (task 3,
// issue #36's "annotate, don't subtract"): one chip naming the nearest
// retired ancestor when node.held carries one, absent otherwise. Unlike
// DriftedChip it never has a neutral reading to fall back to - a retired
// ancestor either holds the node or it does not exist on the page at all -
// so it always paints "blocked", the same register the Drifted chip takes
// only when a badge actually refuses a verb (nodepage.go:114-139's rule):
// under a retired ancestor the doing verbs do refuse
// (mutate.guardParentState), so this chip earns that register unconditionally.
// The title names the ancestor and its state, since the chip word alone
// ("Held") does not say which one.
function HeldChip({ held }: { held: HeldView | undefined }) {
  if (held === undefined) {
    return null;
  }
  return (
    <Chip status="blocked" title={`${held.noun} ${held.ref} is ${held.state}`}>
      Held
    </Chip>
  );
}

// ReviewGrainChip is the Review Grain build's own badge: the resolved
// review grain in force for work under an epic or a feature
// (node.review_grain, views.NodeView.ReviewGrain), rendered only when it
// differs from "task", the value every tree carries by default and needs no
// badge to explain. Scoped to epic and feature pages alone: a task owns no
// home of its own to override with, and its in_review status already reads
// as riding the grain node's PR wherever that matters
// (docs/spec/ui-surface.md). It borrows Chip's quiet "draft" register, the same
// neutral bucket DriftedChip's own non-refusing badge already uses,
// deliberately never the accent: a coarser grain is a calibration a human
// chose, not a caution the page is raising.
function ReviewGrainChip({ node }: { node: NodeView }) {
  if (node.noun !== "epic" && node.noun !== "feature") {
    return null;
  }
  if (!has(node.review_grain) || node.review_grain === "task") {
    return null;
  }
  return <Chip status="draft">{`review grain: ${node.review_grain}`}</Chip>;
}

// KindLine is the chain's second link (node header pass, 2026-08-30): the
// noun on a solid kind pill, the node's own state chip at the hero size (the
// activity phase still appended as text, withActivityPhrase), the recorded
// gate results as compact ticks, then the page's other always-in-the-title
// chips - Drifted, Held, Review Grain, and the manual blocked flag - unchanged
// in behavior, just re-homed off the title. Kind and state are the two glance
// facts, so they sit together on one short line at a fixed spot above the
// title, and the title never shares its line with a chip, which is what keeps
// a long task or bug title wrapping cleanly.
function KindLine({
  page,
  driftExpanded,
  onDriftToggle,
}: {
  page: NodePage;
  driftExpanded: boolean;
  onDriftToggle: () => void;
}) {
  const node = page.node;
  const chips = gateChipsFor(node.gates, page.eval);
  return (
    <div className="kind-line">
      <span className="kind-pill">{kindLabel(node.noun)}</span>
      <Chip status={node.status} size="hero">
        {withActivityPhrase(node)}
      </Chip>
      {chips.length > 0 && (
        <span className="gate-chips">
          {chips.map((c) => (
            <span key={c.name} className="gate-chip">
              <span className={c.pass ? "g pass" : "g pend"} aria-hidden="true">
                {c.pass ? "\u2713" : "\u25d0"}
              </span>
              {c.name}
              {c.qualifier !== undefined && ` \u00b7 ${c.qualifier}`}
            </span>
          ))}
        </span>
      )}
      <DriftedChip badges={page.drift} expanded={driftExpanded} onToggle={onDriftToggle} />
      <HeldChip held={node.held} />
      <ReviewGrainChip node={node} />
      {node.blocked === true && <span className="instr-blocked error">blocked</span>}
    </div>
  );
}

// NodeTitle is the chain's third link: the title alone, large. Its chips
// moved up to the kind line, so this is the one element on the page that
// reads as the node's name and nothing else.
function NodeTitle({ node }: { node: NodeView }) {
  return <h1 className="node-title">{handleOf(node)}</h1>;
}

// RefLine is the chain's fourth link: one quiet mono reference line, the
// canonical key path with the id in parentheses (page.ref, gen.Ref) and a
// copy affordance - CopyRef's own quiet register, unchanged, just moved out
// of the old dmeta caption into its own row so same-titled nodes stay
// tellable apart at a glance rather than after reading a run-on line.
function RefLine({ page }: { page: NodePage }) {
  return (
    <div className="refline">
      <CopyRef display={page.ref} id={page.node.id} />
    </div>
  );
}

// Facts is the chain's last link, the header's one metadata block (node
// header pass, 2026-08-30): every fact labeled, label over value, in one fixed
// order, under ONE divider. It replaces the earlier meta row plus the three
// caption lines beneath it (source, change type and surface, basis), whose
// divider sat between metadata and metadata. Every fact is independently
// optional and the block draws nothing when none is present, the same "a
// section that holds nothing is not drawn" discipline InstrSection keeps.
// The order is the reading order: who holds it, where it was found, where
// the work sits, what kind of change, what it touches, what it was signed
// against, and how it crossed done.
interface Fact {
  label: string;
  value: ReactNode;
}

function Facts({ page }: { page: NodePage }) {
  const node = page.node;
  const rel = page.relationships;
  const items: Fact[] = [];
  if (node.noun === "epic" || node.noun === "feature") {
    items.push({ label: "Contributors", value: <Contributors people={node.contributors ?? []} /> });
  } else if (has(node.owner)) {
    items.push({ label: "Owner", value: <b className="mono"><PersonName id={node.owner} name={node.owner_name} /></b> });
  }
  // The proposer is worth a line only when it adds a person: a proposed shelf
  // node has no owner yet, and a task's proposer is usually its owner.
  if (has(node.proposed_by) && node.proposed_by !== node.owner) {
    items.push({ label: "Proposed by", value: <b className="mono"><PersonName id={node.proposed_by} /></b> });
  }
  if (rel.attached_to !== undefined) {
    items.push({ label: "Found in", value: <RelationValue node={rel.attached_to} /> });
  }
  if (rel.resolved_by !== undefined) {
    items.push({ label: "Carried by", value: <RelationValue node={rel.resolved_by} /> });
  }
  if (has(node.branch)) {
    items.push({ label: "Branch", value: <BranchValue node={node} /> });
  }
  if (node.pr !== undefined) {
    items.push({ label: "PR", value: <PRValue node={node} /> });
  }
  if (has(node.change_type)) {
    items.push({ label: "Change", value: node.change_type });
  }
  const paths = node.surface?.paths ?? [];
  if (paths.length > 0) {
    items.push({ label: "Touches", value: <span className="mono">{paths.join(", ")}</span> });
  }
  const basis = basisFact(node);
  if (basis !== null) {
    items.push(basis);
  }
  if (has(node.done_basis)) {
    items.push({ label: "Done basis", value: node.done_basis });
  }
  if (has(node.resolution)) {
    items.push({ label: "Resolution", value: node.resolution });
  }
  // A question's page is flat, with no Spec section to carry its source the
  // way every prose-bearing noun's Spec head does now (ProseSection), so its
  // header keeps the fact.
  if (node.noun === "question" && has(node.source)) {
    items.push({ label: "Source", value: <SourceLink source={node.source} diagnostics={page.backlinks.diagnostics} /> });
  }
  if (items.length === 0) {
    return null;
  }
  return (
    <dl className="node-facts">
      {items.map((fact) => (
        <div key={fact.label} className="fact">
          <dt>{fact.label}</dt>
          <dd>{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

// RelationValue is a shelf node's found-in or carried-by target: the title as
// a link (crumbLabel's own title-leads convention) with the target's own
// state chip beside it, so the reader sees the carrier's state without
// leaving the page.
function RelationValue({ node }: { node: RelatedNode }) {
  return (
    <>
      <RefLink node={node} label={crumbLabel(node)} className="" />
      <Chip status={node.status} />
    </>
  );
}

// Outbound is the cue on a link that leaves the console: the stylesheet's
// glyph rides ::after, which assistive tech never hears, so the words ride
// a visually hidden span inside the same link.
function Outbound() {
  return <span className="visually-hidden"> (opens in a new tab)</span>;
}

// BranchValue and PRValue link where serve resolved a page for the fact
// (branch_url, pr_url: a GitHub origin only) and read as plain text
// otherwise, the same honest-number rule ProvenanceLine already keeps for a
// PR with no resolvable origin.
function BranchValue({ node }: { node: NodeView }) {
  if (has(node.branch_url)) {
    return (
      <a className="ext mono" href={node.branch_url} target="_blank" rel="noreferrer">
        {node.branch}
        <Outbound />
      </a>
    );
  }
  return <span className="mono">{node.branch}</span>;
}

function PRValue({ node }: { node: NodeView }) {
  if (has(node.pr_url)) {
    return (
      <a className="ext" href={node.pr_url} target="_blank" rel="noreferrer">
        #{node.pr}
        <Outbound />
      </a>
    );
  }
  return <>#{node.pr}</>;
}

// basisFact is the node's recorded acceptance basis as one fact, the label
// carrying the verb so the value is only refs: the completion record
// (verified_against, set at done) once one exists, since a done node's
// verified revisions are the ones that actually shipped, else the
// approval-time pin (accepted_against). Each ref renders only when recorded
// (a task carries no eval blob, and the old line printed "\u00b7 eval" with
// nothing after it), as a short sha with the full token on hover; the commit
// links to its page where serve resolved one.
interface BasisRefs {
  spec?: string;
  eval?: string;
  commit?: string;
  commit_url?: string;
}

function hasRefs(record: BasisRefs): boolean {
  return has(record.spec) || has(record.eval) || has(record.commit);
}

function basisRecord(node: NodeView): { label: string; refs: BasisRefs } | null {
  if (node.verified_against !== undefined && hasRefs(node.verified_against)) {
    return { label: "Verified against", refs: node.verified_against };
  }
  if (node.accepted_against !== undefined && hasRefs(node.accepted_against)) {
    return { label: "Approved against", refs: node.accepted_against };
  }
  return null;
}

function basisFact(node: NodeView): Fact | null {
  const record = basisRecord(node);
  if (record === null) {
    return null;
  }
  const parts: ReactNode[] = [];
  if (has(record.refs.spec)) {
    parts.push(
      <span key="spec" className="mono">
        spec <Rev token={record.refs.spec} />
      </span>,
    );
  }
  if (has(record.refs.eval)) {
    parts.push(
      <span key="eval" className="mono">
        eval <Rev token={record.refs.eval} />
      </span>,
    );
  }
  if (has(record.refs.commit)) {
    parts.push(
      <span key="commit" className="mono">
        at commit <Rev token={record.refs.commit} href={record.refs.commit_url} />
      </span>,
    );
  }
  return {
    label: record.label,
    value: parts.map((part, i) => (
      <Fragment key={i}>
        {i > 0 && (
          <span className="dotsep" aria-hidden="true">
            {"\u00b7"}
          </span>
        )}
        {part}
      </Fragment>
    )),
  };
}

// Rev is one revision token, shortened for display (lib/revision.ts) with the
// full token as the hover title, a link when a page exists for it.
function Rev({ token, href }: { token: string; href?: string }) {
  const short = shortRev(token);
  if (has(href)) {
    return (
      <a className="ext" href={href} target="_blank" rel="noreferrer" title={token}>
        {short}
        <Outbound />
      </a>
    );
  }
  return <abbr title={token}>{short}</abbr>;
}

// ChildrenMix is the console's signature state bar on a detail header (the
// chart-table pass named detail headers as one of its homes and never built
// it): an epic's features or a feature's tasks as the mixed-child bar with a
// count caption, read straight from page.relationships, the same rows the
// Features and Tasks tabs list. A task, a shelf node, a question, and an
// amendment have no children, so nothing draws.
function ChildrenMix({ page }: { page: NodePage }) {
  const noun = page.node.noun;
  const children =
    noun === "epic"
      ? page.relationships.features
      : noun === "feature"
        ? page.relationships.tasks
        : undefined;
  if (children === undefined || children.length === 0) {
    return null;
  }
  const childNoun = noun === "epic" ? "feature" : "task";
  const { mix, caption } = composeStat(children);
  const label = `${children.length} ${childNoun}${children.length === 1 ? "" : "s"} \u00b7 ${caption}`;
  return (
    <div className="node-mix">
      <StateBar mix={mix} caption={label} width={240} />
      <span className="node-mix-caption" aria-hidden="true">
        {label}
      </span>
    </div>
  );
}

export default function NodeDetailHeader({
  page,
  driftExpanded,
  onDriftToggle,
}: {
  page: NodePage;
  driftExpanded: boolean;
  onDriftToggle: () => void;
}) {
  return (
    <header className="instr-header">
      <Crumbs page={page} />
      <KindLine page={page} driftExpanded={driftExpanded} onDriftToggle={onDriftToggle} />
      <NodeTitle node={page.node} />
      <RefLine page={page} />
      <ChildrenMix page={page} />
      <Facts page={page} />
    </header>
  );
}

