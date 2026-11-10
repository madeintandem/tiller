// Shared drift-row helpers, lifted from views/Drift.tsx when the node page's
// drift panel became the second surface rendering candidate rows (JC's
// chip-to-panel ask, 2026-08-06): one composer for the reference string and
// one for the short sha, so the Drift report and the panel cannot render one
// candidate two ways.

import type { RevisionRefView, UncoveredView } from "../api";
import { has } from "./has";

// refOf composes the reference-convention string RefLink renders (mirroring
// internal/gen/label.go's RefParts): a candidate/uncovered row's own Ref
// field is deliberately absent from the wire (json:"-", views/drift.go's own
// CandidateView/UncoveredView doc comments), so id/key ride structurally
// instead and the client, not the server, composes the display text from
// them, the same house rule every reference-bearing view in this console
// already follows.
export function refOf(key: string | undefined, id: string): string {
  return has(key) ? `${key} (${id})` : id;
}

// shortSha mirrors internal/cli/drift.go's own shortSHA: a since anchor's
// Value carries a compact "git:<sha>" token (vcs.Git.CurrentCommit), and only
// that kind is worth shortening to 7 hex characters; a nil anchor or a
// non-git kind (sha256, for a dirty-tree fingerprint) both pass through
// unchanged, since there is no sha to shorten in either case. The JSON
// payload itself carries the anchor unabridged; this shortening is the
// renderer's own business, exactly as it is the CLI's.
export function shortSha(since: RevisionRefView | undefined): string {
  if (since === undefined) {
    return "";
  }
  const prefix = "git:";
  if (!since.value.startsWith(prefix)) {
    return since.value;
  }
  const hex = since.value.slice(prefix.length);
  return prefix + (hex.length > 7 ? hex.slice(0, 7) : hex);
}

// MARKER_COPY and REASON_COPY are the one place a drift marker or an
// uncovered reason becomes words. No token such as stale_spec or
// ref_unreadable reaches the screen: a marker shows its plain name, one
// sentence of what it means, and one of what a person does next; a reason
// shows its cause and its fix. vocabulary.json lists the known tokens and a Go
// test pins both lists against the sweeps, so a marker or reason added in Go
// fails the build until it has copy here. Each sentence is written from the
// doc comment where the token is produced (internal/core/drift/drift.go and
// design.go). No copy here names a command: a command exists only where the
// server serves a remedy (views.candidateRemedy).
export const MARKER_COPY: Record<string, { name: string; meaning: string; next: string }> = {
  stale_spec: {
    name: "Spec changed after the work",
    meaning: "Finished work was verified against a spec, eval, or surface that has since changed or gone missing.",
    next: "Review what changed against the finished work, then plan follow-up work, or correct a changed eval through an approved amendment.",
  },
  active_basis: {
    name: "Work started from an older basis",
    meaning: "Work in flight started from a spec, eval, or task file that has changed since.",
    next: "Read what changed, then refresh the basis once the work agrees with the current files.",
  },
  answered_question: {
    name: "An answered question was not reviewed",
    meaning: "A question was answered after the work began, and nobody has confirmed the work still holds.",
    next: "Read the answer, then mark the assumption reviewed if the work still stands.",
  },
  missing_surface: {
    name: "No surface recorded",
    meaning: "A path this work recorded as its surface is no longer in the working tree.",
    next: "Link the work to the path it touches now, or drop the path that is gone.",
  },
  orphan_eval: {
    name: "Eval no longer matches its spec",
    meaning: "An eval criterion points at a test or file that can no longer be found.",
    next: "Restore the file or correct the eval; a changed eval goes through an approved amendment.",
  },
  stale_design: {
    name: "Design doc is behind the code",
    meaning: "A design document describes code that finished work has since changed or removed.",
    next: "Read the document against the current code, reconcile it, then confirm it.",
  },
  shifted_design: {
    name: "Design doc may have shifted",
    meaning: "The code a design document describes has changed in the working tree since the document was confirmed.",
    next: "Read the document against the current code, reconcile it, then confirm it.",
  },
};

export const REASON_COPY: Record<string, { cause: string; fix: string }> = {
  no_completion_record: {
    cause: "This finished work has no record of what it was verified against, so there is nothing to compare.",
    fix: "Review it by hand if it matters; the check resumes once new work records a completion.",
  },
  basis_unreadable: {
    cause: "A file the work started from could not be read.",
    fix: "Make the file readable, then run the check again.",
  },
  unknown_question: {
    cause: "The work names a question that is no longer in the ledger.",
    fix: "Remove or replace the reference to that question, then run the check again.",
  },
  spec_unreadable: {
    cause: "The spec recorded at completion could not be read, so it could not be compared.",
    fix: "Restore the spec file, then run the check again.",
  },
  eval_unreadable: {
    cause: "The eval could not be loaded or is not a valid contract, so none of its criteria were checked.",
    fix: "Repair the eval file, then run the check again.",
  },
  surface_unreadable: {
    cause: "A path the work records could not be read.",
    fix: "Make sure the path exists and is readable, then run the check again.",
  },
  ref_unreadable: {
    cause:
      "The eval's ref could not be read as a file. A ref that names several files must be a YAML list, not one space-separated string.",
    fix: "Rewrite ref as a list, then run the check again.",
  },
  unconfirmed: {
    cause: "No human has confirmed this design document against the code.",
    fix: "Read it against the code, then confirm it.",
  },
  surface_unrecorded: {
    cause: "The confirmation did not record a fingerprint for a path the document describes.",
    fix: "Confirm the document again so every path it describes is recorded.",
  },
  history_unavailable: {
    cause: "The git history needed to place this check was not available.",
    fix: "Make sure the project's history is present, then run the check again.",
  },
  history_diverged: {
    cause: "The confirmation and the finished work sit on branches that have split, so no single range can be compared.",
    fix: "Bring the branches together, then run the check again.",
  },
  comparison_error: {
    cause: "The comparison failed before it could finish.",
    fix: "Fix what the detail line names, then run the check again.",
  },
};

// humanize turns a wire token from a newer server into a sentence-case name,
// the same fallback statusLabel uses.
function humanize(token: string): string {
  const spaced = token.replaceAll("_", " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

export function markerCopy(marker: string): { name: string; meaning: string; next: string } {
  return (
    MARKER_COPY[marker] ?? {
      name: humanize(marker),
      meaning: "This check comes from a newer version of Tiller than this console.",
      next: "Update Tiller to read what it means and what to do.",
    }
  );
}

export function reasonCopy(reason: string): { cause: string; fix: string } {
  return (
    REASON_COPY[reason] ?? {
      cause: "This could not be checked, for a reason a newer version of Tiller reports.",
      fix: "Update Tiller to read what to do.",
    }
  );
}

// groupUncovered folds uncovered rows that share a (marker, reason) pair into
// one group, in first-seen order, so a shared cause is stated once. A reason
// is scoped to its marker (drift.Uncovered's doc comment), so the pair is the
// key, never the reason alone.
export function groupUncovered(rows: UncoveredView[]): { marker: string; reason: string; rows: UncoveredView[] }[] {
  const groups: { marker: string; reason: string; rows: UncoveredView[] }[] = [];
  for (const row of rows) {
    const group = groups.find((g) => g.marker === row.marker && g.reason === row.reason);
    if (group === undefined) {
      groups.push({ marker: row.marker, reason: row.reason, rows: [row] });
    } else {
      group.rows.push(row);
    }
  }
  return groups;
}
