// statusLabel is the one place a stored status word becomes the words a
// person reads. Every surface (chips, glyph tooltips, criterion rows, filter
// options) goes through it, so a wire word such as in_review never reaches
// the screen. vocabulary.json lists the known statuses; a Go test pins that
// list against the stored enums, and an unlisted status still reads cleanly
// because the fallback is sentence case with underscores turned to spaces.

// The labels where sentence-casing the wire word is not enough.
const OVERRIDES: ReadonlyMap<string, string> = new Map([
  ["specced", "Specified"],
  ["needs_attestation", "Needs attestation"],
  ["in_review", "In review"],
  ["in_progress", "In progress"],
]);

export function statusLabel(status: string): string {
  const override = OVERRIDES.get(status);
  if (override !== undefined) {
    return override;
  }
  const spaced = status.replaceAll("_", " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}
