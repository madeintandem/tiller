// The amendment's own flat body (ui-surface.md: "An amendment or a design
// doc ... stays flat", never the multi-panel tab set the epic/feature/task
// bodies get; task-6-brief.md's binding spells out the content in order):
// the narrative prose, the staged operations list, then the two design-doc
// lists as quiet mono lines. NodeDetail's own generic layers
// (RelationshipHub, HistorySection) render below this unconditionally for
// every flat page, which is where "history section" lives; this component
// owns none of that.
//
// NO BUTTON HERE APPLIES ANYTHING (the binding's own words): this is a
// review surface, not a write path. The one copyable command an amendment
// ever offers, `tiller approve amendment <addr>`, lives on the queue card
// (DecisionCard), never on this page.

import { Fragment } from "react";
import type {
  AmendmentOperationPageView,
  AmendmentPreconditionPageView,
  AmendmentSourcePageView,
  NodePage,
} from "../../api";
import InstrSection from "../../components/InstrSection";
import RefLink from "../../components/RefLink";
import { has } from "../../lib/has";
import ProseSection from "./ProseSection";

// OperationSubject renders the resolvable part of a staged operation's own
// heading, mirroring cli/amendment_cmds.go's renderOperationHeadline in shape
// without composing a string. A resolvable target is a real RefLink component
// here, never plain text pretending to be one. A revise, correct-eval, or cancel
// names one existing node; a supersede names the node being replaced and its replacement;
// add-feature, add-task, and add-epic name only their staged key and title.
// An add-question has no key, so its bounded title is the heading while the
// complete statement renders below as operation detail.
function OperationSubject({ op }: { op: AmendmentOperationPageView }) {
  switch (op.op) {
    case "revise":
    case "correct-eval":
    case "cancel":
      return op.target !== undefined ? <RefLink node={op.target} /> : <span className="muted">no target</span>;
    case "move-feature":
      return (
        <>
          {op.target !== undefined ? <RefLink node={op.target} /> : <span className="muted">no target</span>}
          <span className="muted">to</span>
          <span className="mono">{op.key}</span>
        </>
      );
    case "supersede":
      return (
        <>
          {op.target !== undefined ? <RefLink node={op.target} /> : <span className="muted">no target</span>}
          <span className="muted">by</span>
          {op.by !== undefined ? <RefLink node={op.by} /> : <span className="muted">no replacement</span>}
        </>
      );
    case "add-feature":
    case "add-task":
    case "add-epic":
      return (
        <>
          <span className="mono">{op.key}</span>
          {has(op.title) && <span>{op.title}</span>}
        </>
      );
    case "add-question":
      return has(op.title) ? <span>{op.title}</span> : <span className="muted">no title</span>;
    default:
      return op.target !== undefined ? <RefLink node={op.target} /> : null;
  }
}

// PreconditionRow: the staged name and value always, the LIVE validity as
// its own word beside them when the serve handler filled it (the page's
// typed nil slot, absent means the read never ran). The word is never
// color-coded (the .instr-bindchip pill CriterionRow's own binding chip
// already uses, quiet slate on tint, identical for all three states): the
// word itself carries the meaning, and painting valid green and stale red
// would be a second signal this page's own binding refuses to invent.
function PreconditionRow({ pc }: { pc: AmendmentPreconditionPageView }) {
  return (
    <li className="instr-drift instr-amendpre">
      <span className="mono instr-drift-say">{pc.name}</span>
      <span className="muted">expected</span>
      <span className="mono">{pc.expected}</span>
      {pc.live !== undefined && (
        <>
          <span className="instr-bindchip">{pc.live.status}</span>
          {has(pc.live.actual) && <span className="mono muted">actual {pc.live.actual}</span>}
          {has(pc.live.note) && <span className="muted">{pc.live.note}</span>}
        </>
      )}
    </li>
  );
}

// shapeErrorNote turns one of resolveAmendment's own shape-refusal tokens
// (mutate.ReasonDuplicateStagedKey "duplicate_staged_key",
// mutate.ReasonStagedEpicReplacement "staged_epic_replacement") into the
// sentence OperationRow renders on a flagged operation, the identical wording
// cli/amendment_cmds.go's own shapeErrorNote prints for `show amendment`'s
// human render, so a hand-edit defect reads the same sentence whichever
// surface a human is looking at. The default arm is unreachable through this
// console's own one caller (op.shape_error only ever arrives from
// mutate.EvaluateAmendmentShape, which never mints a third token) and exists
// only so an unrecognized token still renders something rather than nothing.
function shapeErrorNote(kind: string): string {
  switch (kind) {
    case "duplicate_staged_key":
      return "duplicate staged key (another operation already stages this key; one amendment materializes each key once)";
    case "staged_epic_replacement":
      return "staged epic replacement (names a staged add-epic key; a supersede or a cancel only ever targets a feature or a task)";
    default:
      return kind;
  }
}

// OperationRow: one staged operation, the index and op word leading, its
// bounded subject, then an add-question's complete statement as subordinate
// detail. Reason, superseded-by, source, shape defects, and preconditions keep
// their existing order below. source_missing and shape_error are #57's own
// pre-flight; the two are orthogonal facts one operation can carry at once, so
// both render together when both fire, neither silencing the other.
function OperationRow({ op }: { op: AmendmentOperationPageView }) {
  const preconditions = op.preconditions ?? [];
  return (
    <div className="instr-amendop">
      <div className="instr-drift instr-amendop-head">
        <span className="mono">{op.index + 1}</span>
        <span className="instr-drift-say">{op.op}</span>
        <OperationSubject op={op} />
        {op.op === "add-feature" && has(op.change_type) && <span className="muted">{op.change_type}</span>}
      </div>
      {op.op === "add-question" && has(op.statement) && (
        <div className="instr-amendop-detail">
          {op.statement.split(/(?:\r?\n){2,}/).map((paragraph, paragraphIndex) => (
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
      {op.move !== undefined && (
        <div className="instr-amendop-detail">
          <p><span className="muted">reviewed source</span> <span className="mono">{op.move.source_key}</span></p>
          <p>
            Review grain stays <span className="mono">{op.move.review_grain}</span>
            {op.move.materialize_review_grain === true ? " through an explicit feature override." : "."}
          </p>
          <p>{op.move.order_effect === "preserve-position"
            ? "Keeps its sibling position."
            : "Appends to the destination feature order in this batch's move order."}</p>
          {op.move.reference_warnings?.map((warning) => (
            <p key={warning} className="instr-amendop-defect">{warning}</p>
          ))}
        </div>
      )}
      {has(op.reason) && <p className="instr-amendop-reason">{op.reason}</p>}
      {op.retire !== undefined && op.retire.length > 0 && (
        <p className="instr-amendop-detail">
          <span className="muted">retires</span>{" "}
          {op.retire.map((id) => (
            <span key={id} className="mono">
              {id}{" "}
            </span>
          ))}
        </p>
      )}
      {op.superseded_by !== undefined && (
        <p className="instr-amendop-superseded">
          <span className="muted">superseded by</span> <RefLink node={op.superseded_by} />
        </p>
      )}
      {op.created !== undefined && (
        <p className="instr-amendop-detail">
          <span className="muted">created node</span> <RefLink node={op.created} />
        </p>
      )}
      {op.parent !== undefined && (
        <p className="instr-amendop-detail">
          <span className="muted">{op.op === "move-feature" ? "destination epic" : "addition parent"}</span> <RefLink node={op.parent} />
        </p>
      )}
      {op.blocks?.map((target) => (
        <p key={`${target.id}:${target.ref}`} className="instr-amendop-detail">
          <span className="muted">question block target</span> <RefLink node={target} />
        </p>
      ))}
      {op.identity_notes?.map((note) => (
        <p key={note} className="instr-amendop-defect">{note}</p>
      ))}
      {has(op.source) && (
        <p className="instr-amendop-source">
          <span className="muted">source</span> <span className="mono">{op.source}</span>
        </p>
      )}
      {op.source_missing === true && (
        <p className="instr-amendop-defect error">source missing (intake amendments cite every operation)</p>
      )}
      {has(op.shape_error) && <p className="instr-amendop-defect error">{shapeErrorNote(op.shape_error)}</p>}
      {preconditions.length > 0 && (
        <ul className="instr-amendop-pre">
          {preconditions.map((pc) => (
            <PreconditionRow key={pc.name} pc={pc} />
          ))}
        </ul>
      )}
    </div>
  );
}

// DesignDocLine is one quiet mono line in the caption register (instr-dmeta,
// which this page is the last tenant of since the node header pass moved
// the header's own captions into its facts block): triggered_by (why this
// reshape exists) and amends_design
// (what its approval keeps current) are provenance, not the page's own
// substance, so they read as a caption rather than a section.
function DesignDocLine({ label, paths }: { label: string; paths: string[] }) {
  if (paths.length === 0) {
    return null;
  }
  return (
    <div className="instr-dmeta">
      <span className="muted">{label}</span>
      {paths.map((p) => (
        <span key={p} className="mono">
          {p}
        </span>
      ))}
    </div>
  );
}

// AmendmentSourceLine is the amendment-level twin of DesignDocLine, the same
// caption idiom (instr-dmeta) beside triggered_by/amends_design: an
// intake-minted amendment's own fingerprinted feedback artifact, absent
// whole for a hand-opened one. Live validity, when the serve handler filled
// it, renders through the identical instr-bindchip word PreconditionRow's
// own live status already uses: the word itself carries the meaning
// (valid/stale/unknown), never a color of its own (this page's settled
// precondition rule).
function AmendmentSourceLine({ source }: { source: AmendmentSourcePageView | undefined }) {
  if (source === undefined) {
    return null;
  }
  return (
    <div className="instr-dmeta">
      <span className="muted">source</span>
      <span className="mono">{source.path}</span>
      {source.live !== undefined && (
        <>
          <span className="instr-bindchip">{source.live.status}</span>
          {has(source.live.actual) && <span className="mono muted">actual {source.live.actual}</span>}
          {has(source.live.note) && <span className="muted">{source.live.note}</span>}
        </>
      )}
    </div>
  );
}

// ValidityErrorBanner is #57's own console-side notice: the ONE reason live
// precondition/source validity could not be computed against the current
// tree (amendment.validity_error, node.go's fillAmendmentValidity), named
// where a human lands on the page rather than left for every Live slot's own
// silent blank to imply. Reuses .instr-warning, the page's existing
// never-hide-a-defect idiom (GateCard's EvalContractWarning renders a
// malformed eval contract the identical way), rather than inventing a second
// visual grammar for the same kind of fact: a read that could not run.
function ValidityErrorBanner({ message }: { message: string | undefined }) {
  if (!has(message)) {
    return null;
  }
  return (
    <div className="instr-warning">
      <p className="error">Live validity unavailable: {message}</p>
    </div>
  );
}

export default function AmendmentBody({ page }: { page: NodePage }) {
  const amendment = page.amendment;
  if (amendment === undefined) {
    return null;
  }
  const operations = amendment.operations;
  return (
    <>
      <ValidityErrorBanner message={amendment.validity_error} />
      {has(page.relationships.amendment_history_warning) && (
        <div className="instr-warning">
          <p>{page.relationships.amendment_history_warning}</p>
        </div>
      )}
      <ProseSection prose={page.prose} label="Narrative" node={page.node} diagnostics={page.backlinks.diagnostics} />
      {/* A freshly opened amendment with nothing staged yet draws no section
          at all (InstrSection's own rule: a section that holds nothing is
          not drawn), the same silence TaskList keeps for a childless
          feature. */}
      {operations.length > 0 && (
        <InstrSection label={`Operations · ${operations.length}`}>
          {operations.map((op) => (
            <OperationRow key={op.index} op={op} />
          ))}
        </InstrSection>
      )}
      <AmendmentSourceLine source={amendment.source} />
      <DesignDocLine label="triggered by" paths={amendment.triggered_by ?? []} />
      <DesignDocLine label="amends design" paths={amendment.amends_design ?? []} />
    </>
  );
}
