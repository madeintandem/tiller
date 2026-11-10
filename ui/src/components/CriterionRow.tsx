// CriterionRow renders one authored acceptance criterion as the contract's
// own grid row (the mock's own .crow: the retired
// console mock): id, statement, the binding's TYPE alone - never its ref or run
// detail, which the compact grid has no column for; that fact still reads on
// the gate's own flat evidence line and in AmendmentBody's precondition rows,
// this row's job narrowed to exactly the brief's own five fields - and a status
// chip. required never renders here either: the exhaustive field list is id,
// statement, binding type, status chip, attested-by, and a criterion's own
// required-ness has no slot left in it.
//
// The pass-1 row's per-
// criterion EVIDENCE link (CriterionStatusView.evidence, still served -
// api.ts's own doc comment: populated by `tiller eval --record-rubric-
// output`) was dropped in the rewrite with nothing standing in for it, so a
// criterion's own recorded rubric output rendered nowhere on the page -
// AskBox's own why sentence ("Review the graded evidence, then sign it
// off") pointed at a fact this row no longer showed. It is back, folded
// into the statement cell as a second line rather than the .cst cell, which has
// no room for an arbitrary- length path beside the status chip.
//
// The chip's word is "attested · <owner>" whenever a recorded attestation
// targets this criterion AND the criterion's own live status is pass
// (attestedByFor, the mock's own C6 row); every other live status reads its
// own plain word instead, since an attestation that recorded a fail, or one
// a later mechanical recheck has since superseded, never earns the
// "attested" word over the status the gate actually carries now.

import { useEffect, useRef } from "react";
import { useInRouterContext, useSearchParams } from "react-router-dom";

import type { AttestationView, CriterionStatusView, EvalCriterionView } from "../api";
import { evidenceLabel } from "../lib/evidenceLabel";
import { dotStateFor, resultStatus } from "../lib/gateDisplay";
import { has } from "../lib/has";
import { statusLabel } from "../lib/statusLabel";
import { usePersonName } from "../people";
import Chip from "./Chip";

// attestedByFor names the human who satisfied this criterion by attestation
// (mutate.Attest: target_kind "criterion", target_id the criterion's own
// id), the LATEST recorded entry when more than one ever targeted it (an
// attestation list preserves recorded order, node.go's own Attestations
// projection), and only when the criterion's own live status is pass.
function attestedByFor(
  criterionID: string,
  status: string,
  attestations: AttestationView[],
): string | undefined {
  if (status !== "pass") {
    return undefined;
  }
  for (let i = attestations.length - 1; i >= 0; i -= 1) {
    const a = attestations[i];
    if (a.target_kind === "criterion" && a.target_id === criterionID && has(a.by)) {
      return a.by;
    }
  }
  return undefined;
}

interface CriterionRowProps {
  criterion: EvalCriterionView;
  result?: CriterionStatusView;
  attestations?: AttestationView[];
}

// A row outside any router (a bare render in a test) is never a deep link
// target; inside one, the criterion search param names the target.
export default function CriterionRow(props: CriterionRowProps) {
  const inRouter = useInRouterContext();
  return inRouter ? <RoutedRow {...props} /> : <Row {...props} isTarget={false} />;
}

function RoutedRow(props: CriterionRowProps) {
  const [search] = useSearchParams();
  return <Row {...props} isTarget={search.get("criterion") === props.criterion.id} />;
}

function Row({
  criterion,
  result,
  attestations = [],
  isTarget,
}: CriterionRowProps & { isTarget: boolean }) {
  // The deep link (a Needs You card's statement) lands on this row: bring it
  // into view once on mount. The class is a bar and a weight, never color
  // alone.
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (isTarget) {
      ref.current?.scrollIntoView({ block: "center" });
    }
  }, [isTarget]);
  const personName = usePersonName();
  const status = resultStatus(result?.status);
  const by = attestedByFor(criterion.id, status, attestations);
  const label = by !== undefined ? `attested · ${personName(by)}` : statusLabel(status);
  const evidence = result?.evidence ?? [];
  const firstEvidence = evidence.find((e) => has(e.ref));
  const evidenceText =
    firstEvidence === undefined
      ? ""
      : evidenceLabel({ ref: firstEvidence.ref ?? "", kind: firstEvidence.kind, criterion: criterion.id });
  return (
    <div className={isTarget ? "crow is-target" : "crow"} id={`criterion-${criterion.id}`} ref={ref}>
      <span className="cid mono">{criterion.id}</span>
      <span className="crow-statement">
        {criterion.statement}
        {firstEvidence !== undefined && (
          firstEvidence.unavailable !== undefined ? (
            <span className="instr-evid crow-evid" title={firstEvidence.ref}>{evidenceText} · Evidence unavailable: {firstEvidence.unavailable}</span>
          ) : (
          <a
            className="instr-evid crow-evid"
            href={firstEvidence.url ?? `/api/evidence?ref=${encodeURIComponent(firstEvidence.ref ?? "")}`}
            title={firstEvidence.ref}
            target="_blank"
            rel="noreferrer"
          >
            {evidenceText}
          </a>
          )
        )}
      </span>
      <span className="cty mono">{criterion.binding.type}</span>
      <span className="cst">
        <Chip status={dotStateFor(status)}>{label}</Chip>
      </span>
    </div>
  );
}
