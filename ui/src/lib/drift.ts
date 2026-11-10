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
// sentence of what it means, one of what a person does next, and the label
// the Uncovered section heads its rows with (unchecked), which names what
// could not be checked rather than asserting the finding; a reason shows its
// cause and its fix. vocabulary.json lists the known tokens and a Go test pins
// both lists against the sweeps, so a marker or reason added in Go fails the
// build until it has copy here. Each sentence is written from the doc comment
// where the token is produced (internal/core/drift/drift.go and design.go). No
// copy here names a command: a command exists only where the server serves a
// remedy (views.candidateRemedy).
export interface MarkerCopy {
  name: string;
  meaning: string;
  next: string;
  unchecked: string;
}

export const MARKER_COPY: Record<string, MarkerCopy> = {
  stale_spec: {
    name: "Spec changed after the work",
    meaning: "Finished work was verified against a spec, eval, or surface that has since changed or gone missing.",
    next: "Review what changed against the work, and for finished work make any correction through a reviewed amendment.",
    // Heads spec, eval, and surface rows alike, and a finished node with no
    // record at all: each is the completion record's comparison not running.
    unchecked: "Finished work not checked against its completion record",
  },
  // The sweep covers ready and active features and approved tasks that are
  // not finished (drift.ActiveBasisApplies), so the meaning holds for
  // approved work that has not started as well as work under way.
  active_basis: {
    name: "Work rests on an older basis",
    meaning: "Approved work rests on a spec, eval, or task file that has changed since it was approved.",
    next: "Read what changed, then refresh the basis once the work agrees with the current files.",
    unchecked: "Approved work not checked against its basis",
  },
  answered_question: {
    name: "An answered question was not reviewed",
    meaning: "A question was answered after the work began, and nobody has confirmed the work still holds.",
    next: "Read the answer, then mark the assumption reviewed if the work still stands.",
    unchecked: "Answered questions not checked",
  },
  missing_surface: {
    name: "Recorded file is gone",
    meaning: "A path this work recorded as its surface is no longer in the working tree.",
    next: "Link the work to the path it touches now, or drop the path that is gone.",
    unchecked: "Recorded files not checked",
  },
  orphan_eval: {
    name: "Eval points at something missing",
    meaning: "An eval criterion points at a test or file that can no longer be found.",
    next: "Restore the file or correct the eval; a changed eval goes through an approved amendment.",
    unchecked: "Eval references not checked",
  },
  stale_design: {
    name: "Design doc is behind the code",
    meaning: "A design document describes code that finished work has since changed or removed.",
    next: "Read the document against the current code, reconcile it, then confirm it.",
    unchecked: "Design doc not checked against the code",
  },
  shifted_design: {
    name: "Design doc may have shifted",
    meaning: "The code a design document describes has changed in the working tree since the document was confirmed.",
    next: "Read the document against the current code, reconcile it, then confirm it.",
    unchecked: "Design doc not checked against the working tree",
  },
  // Not a sweep marker: serve/prose.go reports a failed comparison of a design
  // file's baseline panel under "design" (vocabulary.json drift_extra_markers).
  design: {
    name: "Design comparison failed",
    meaning: "Comparing this design document with its code failed before it could finish.",
    next: "Read the detail line, fix what it names, then check the document again.",
    unchecked: "Design doc not compared",
  },
};

// REASON_COPY is the default sentence for a reason. A reason is scoped to its
// marker (drift.Uncovered's doc comment), so where one sentence cannot be true
// under every marker that emits it, REASON_COPY_BY_MARKER overrides it for the
// markers that differ. Read both only through reasonCopy.
//
// Reason by marker, from drift.go, design.go, and serve/prose.go:
//   no_completion_record: stale_spec
//   spec_unreadable: stale_spec
//   eval_unreadable: stale_spec (the recorded eval blob), orphan_eval (the whole contract)
//   surface_unreadable: stale_spec, missing_surface, stale_design, shifted_design
//   basis_unreadable: active_basis
//   unknown_question: answered_question
//   ref_unreadable: orphan_eval
//   unconfirmed: stale_design, shifted_design
//   surface_unrecorded: shifted_design
//   history_unavailable: stale_design, shifted_design
//   history_diverged: stale_design (against finished work), shifted_design (against HEAD)
//   comparison_error: design
// Defaults below read for the first marker that emits each reason, except
// eval_unreadable (orphan_eval) and history_diverged (stale_design). design.go
// emits stale_design's history_diverged in two places, when one feature's
// completion has split from the confirmation and when the finished features'
// completions have split from each other; the default is true for both.
//
// detailRestatesCause marks a reason whose row detail from the server is a
// fixed sentence that says the cause again in other words (design.go's "no
// human confirmation of the described code is recorded" and shifted_design's
// "confirmation is not an ancestor of current project HEAD; no bounded
// range", drift.go's "done but carries no ... completion record"). The Uncovered group already states the
// cause, so such a row shows no detail of its own; every other reason's detail
// names something specific to its row (a path, a question id, an error) and
// stays.
export interface ReasonCopy {
  cause: string;
  fix: string;
  detailRestatesCause?: true;
}

export const REASON_COPY: Record<string, ReasonCopy> = {
  no_completion_record: {
    cause: "This finished work has no record of what it was verified against, so there is nothing to compare.",
    fix: "Review its spec and surface by hand if they matter.",
    detailRestatesCause: true,
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
    cause: "The spec file could not be read or hashed, so it could not be compared with the one recorded when the work finished.",
    fix: "Make the spec file readable, then run the check again.",
  },
  eval_unreadable: {
    cause: "The eval could not be loaded or is not a valid contract, so none of its criteria were checked.",
    fix: "Repair the eval file, then run the check again.",
  },
  surface_unreadable: {
    cause: "A file or folder recorded when the work finished could not be read, so it could not be compared.",
    fix: "Make the path readable, then run the check again.",
  },
  ref_unreadable: {
    cause:
      "The eval's ref could not be read as a file. A ref that names several files must be a YAML list, not one space-separated string.",
    fix: "Rewrite ref as a list, then run the check again.",
  },
  unconfirmed: {
    cause: "No human has confirmed this design document against the code.",
    fix: "Read it against the code, then confirm it.",
    detailRestatesCause: true,
  },
  surface_unrecorded: {
    cause: "The confirmation did not record a fingerprint for a path the document describes.",
    fix: "Confirm the document again so every path it describes is recorded.",
  },
  history_unavailable: {
    cause: "The git history needed to compare this document's confirmation was not available.",
    fix: "Make sure the project's history is present, then run the check again.",
  },
  history_diverged: {
    cause:
      "The confirmation and the finished work that touched this code do not sit on one line of history, so no single range can be compared.",
    fix: "Bring the branches together, then run the check again.",
  },
  comparison_error: {
    cause: "The comparison failed before it could finish.",
    fix: "Fix what the detail line names, then run the check again.",
  },
};

// Overrides keyed marker, then reason, for the pairs the default would
// misstate.
export const REASON_COPY_BY_MARKER: Record<string, Record<string, ReasonCopy>> = {
  stale_spec: {
    eval_unreadable: {
      cause: "The eval file could not be read or hashed, so it could not be compared with the one recorded when the work finished.",
      fix: "Make the eval file readable, then run the check again.",
    },
  },
  missing_surface: {
    surface_unreadable: {
      cause: "A path this work records as its surface could not be checked.",
      fix: "Make the path readable, then run the check again.",
    },
  },
  stale_design: {
    surface_unreadable: {
      cause: "A path this document describes could not be read, so it could not be compared.",
      fix: "Make the path readable, then run the check again.",
    },
  },
  shifted_design: {
    surface_unreadable: {
      cause: "A path this document describes could not be read, so it could not be compared.",
      fix: "Make the path readable, then run the check again.",
    },
    // design.go's detail here is a fixed sentence ("confirmation is not an
    // ancestor of current project HEAD; no bounded range"), the cause again.
    history_diverged: {
      cause: "The confirmation is not an ancestor of the project's current commit, so no single range can be compared.",
      fix: "Bring the branches together, then run the check again.",
      detailRestatesCause: true,
    },
  },
};

// humanize turns a wire token from a newer server into a sentence-case name,
// the same fallback statusLabel uses.
function humanize(token: string): string {
  const spaced = token.replaceAll("_", " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

export function markerCopy(marker: string): MarkerCopy {
  return (
    MARKER_COPY[marker] ?? {
      name: humanize(marker),
      meaning: "This check comes from a newer version of Tiller than this console.",
      next: "Update Tiller to read what it means and what to do.",
      unchecked: `${humanize(marker)} not checked`,
    }
  );
}

// reasonCopy is the one lookup for a reason's words: the (marker, reason)
// override if there is one, then the reason's default, then a generic line.
export function reasonCopy(marker: string, reason: string): ReasonCopy {
  return (
    REASON_COPY_BY_MARKER[marker]?.[reason] ??
    REASON_COPY[reason] ?? {
      cause: "This could not be checked, for a reason a newer version of Tiller reports.",
      fix: "Update Tiller to read what to do.",
    }
  );
}

// uncoveredDetail is the row's own detail line, or "" when the server's
// detail only restates the cause its group already states (detailRestatesCause).
export function uncoveredDetail(row: UncoveredView): string {
  if (!has(row.detail) || reasonCopy(row.marker, row.reason).detailRestatesCause === true) {
    return "";
  }
  return row.detail;
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
