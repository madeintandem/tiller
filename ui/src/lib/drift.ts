// Shared drift-row helpers, lifted from views/Drift.tsx when the node page's
// drift panel became the second surface rendering candidate rows (JC's
// chip-to-panel ask, 2026-08-06): one composer for the reference string and
// one for the short sha, so the /drift page and the panel cannot render one
// candidate two ways.

import type { RevisionRefView } from "../api";
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
