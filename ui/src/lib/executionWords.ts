// Plain words for the enums Home's Branches and Recovered reviews tabs show.
// Each table covers every value the Go side defines (the diagnostic codes in
// internal/execution/snapshot.go and git.go, and the recovered-review bases in
// internal/store/recovered_review.go); an unknown value still reads cleanly
// because the fallback is the token with its underscores turned to spaces and
// its first letter capitalized.

function humanize(token: string): string {
  const spaced = token.replaceAll("_", " ").trim();
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

const DIAGNOSTICS: ReadonlyMap<string, string> = new Map([
  ["branch_invalid", "Invalid branch name"],
  ["branch_missing", "Branch not found"],
  ["discovery_unavailable", "Branch discovery unavailable"],
  ["divergent_refs", "Local and tracking branches have diverged"],
  ["invalid_candidate", "Task record unreadable"],
  ["invalid_identity", "Task identity invalid"],
  ["ownership_superseded", "Ownership is out of date"],
  ["ownership_unresolved", "Ownership could not be compared"],
  ["remote_ambiguous", "Remote is ambiguous"],
  ["source_unavailable", "Source unavailable"],
  ["source_unstable", "Source changed while it was read"],
  ["worktree_ambiguous", "More than one worktree holds the branch"],
]);

export function diagnosticLabel(code: string): string {
  return DIAGNOSTICS.get(code) ?? humanize(code);
}

const BASES: ReadonlyMap<string, string> = new Map([
  ["ancestry", "ancestry"],
  ["content", "matching content"],
  ["contained", "containment"],
]);

export function recoveryBasisLabel(basis: string): string {
  return BASES.get(basis) ?? humanize(basis).toLowerCase();
}
