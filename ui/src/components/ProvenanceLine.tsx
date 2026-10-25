// ProvenanceLine is the acceptance instrument's caption-weight provenance
// fact line (mock round 4's .provline): the fingerprint the last check ran
// against, the review target, and the policy-violation count. All three are a
// straight readout of what the working tree recorded; nothing here is
// computed or graded.
//
// The review target has three shapes (the same distinction NodeShell's own
// pass-1 review row drew): a linked PR when serve resolved pr_url to a GitHub
// origin, honest PR-number text when a PR is recorded but no origin resolved
// (a non-GitHub remote - a bare pr with no resolvable origin still gets an
// honest number, never a guessed link), or the offline branch+head pair from
// review_ref when the repo has no remote at all. At most one of these ever
// applies; a node with none of the three has not reached review yet.
//
// The component renders nothing until there is an actual provenance story to
// tell (a check has run, or a review target exists): the mock's own draft-
// state instrument shows no provline at all, only once the at-review example
// has real facts behind it does the line appear. Once it renders, the policy
// violation count always shows, "none" included, the same "none" the mock's
// own at-review example states plainly rather than omitting.

import { has } from "../lib/has";
import type { PolicyViolationView, ReviewRefView } from "../api";

// dirtyNote reads the compact revision token's own prefix (cli-surface.md,
// "Gates, evals, and attestation": against is the HEAD git_commit when the
// tree is clean, a sha256 working-tree fingerprint when it is dirty) rather
// than a separate field the wire does not carry, so "(working tree, dirty)"
// is an honest inference from the token shape the schema itself guarantees,
// never an invented annotation.
function dirtyNote(against: string): string {
  return against.startsWith("sha256:") ? " (working tree, dirty)" : "";
}

function violationsText(violations: PolicyViolationView[]): string {
  return violations.length === 0 ? "none" : String(violations.length);
}

type ReviewTarget =
  | { kind: "linked"; pr: number; url: string }
  | { kind: "text"; pr: number }
  | { kind: "offline"; branch: string; head: string };

function reviewTargetOf(
  pr: number | undefined,
  prUrl: string | undefined,
  reviewRef: ReviewRefView | undefined,
): ReviewTarget | null {
  if (pr !== undefined && has(prUrl)) {
    return { kind: "linked", pr, url: prUrl };
  }
  if (pr !== undefined) {
    return { kind: "text", pr };
  }
  if (has(reviewRef?.branch) && has(reviewRef?.head)) {
    return { kind: "offline", branch: reviewRef.branch, head: reviewRef.head };
  }
  return null;
}

export default function ProvenanceLine({
  against,
  pr,
  prUrl,
  reviewRef,
  violations,
}: {
  against?: string;
  pr?: number;
  prUrl?: string;
  reviewRef?: ReviewRefView;
  violations: PolicyViolationView[];
}) {
  const target = reviewTargetOf(pr, prUrl, reviewRef);

  if (!has(against) && target === null) {
    return null;
  }

  return (
    <div className="instr-provline">
      {has(against) && (
        <span>
          checked against <span className="mono">{against}</span>
          {dirtyNote(against)}
        </span>
      )}
      {target?.kind === "linked" && (
        <span>
          review{" "}
          <a href={target.url} target="_blank" rel="noreferrer">
            #{target.pr}
          </a>
        </span>
      )}
      {target?.kind === "text" && <span>review #{target.pr}</span>}
      {target?.kind === "offline" && (
        <span>
          review offline · <span className="mono">{target.branch}</span> at{" "}
          <span className="mono">{target.head}</span>
        </span>
      )}
      <span>policy violations: {violationsText(violations)}</span>
    </div>
  );
}
