// The Build board's six fixed columns (ui-surface.md's Build entry). Features
// have no stored in_review state and stay active through review, so the In
// review column normally stays empty. Also here: the two pure sequence operations
// every column's cards run through - flatten the selected epics, then
// filter by status. Neither step sorts anything: visibleCards preserves the
// served epic-then-feature order verbatim (BuildBoardOf's own doc comment,
// internal/views/build.go), and columnCards is a status FILTER over that
// exact sequence, never a second ordering pass - the phase's own acceptance
// line, "no client-side ordering code exists", and the reason a column's
// card order always matches `tiller ls feature --status <s>` per selected
// epic.

import type { BuildEpic, BuildFeature } from "../../api";

export interface BuildColumnDef {
  status: string;
  label: string;
}

// A fixed literal array, not derived from any payload: the six column
// statuses never change shape at runtime, and BuildEpic.feature_counts
// is documented as keyed by exactly these six literal strings
// (internal/views/build.go's own package comment), so no fold through
// lib/states.ts's wider wire vocabulary is needed here the way the header
// tiles' composeStat needs one for a mixed noun list.
export const BUILD_COLUMNS: BuildColumnDef[] = [
  { status: "draft", label: "Draft" },
  { status: "specced", label: "Specced" },
  { status: "ready", label: "Ready" },
  { status: "active", label: "Active" },
  { status: "in_review", label: "In review" },
  { status: "done", label: "Done" },
];

// The set of statuses BUILD_COLUMNS actually renders. "cancelled" is a real
// stored feature status the server does not filter out of BuildEpic.Features
// or feature_counts (ui-surface.md:70 fixes the column set at six, on
// purpose, so a cancelled feature is data that exists but has no column to
// land in) - isColumnStatus/renderedFeatureCount below are how the count
// side of the board agrees with what the columns actually hold, rather than
// counting a card that never gets one.
const COLUMN_STATUSES: ReadonlySet<string> = new Set(BUILD_COLUMNS.map((c) => c.status));

export function isColumnStatus(status: string): boolean {
  return COLUMN_STATUSES.has(status);
}

// renderedFeatureCount is how many of an epic's own features actually land
// in one of the six columns - the number the board bar's "Showing N
// features" line and the panel's er-n badge both have to agree with. Using
// features.length here (or feature_counts summed wholesale) would also
// count a cancelled feature that renders on no column anywhere on the board,
// overstating what a reader can actually see.
export function renderedFeatureCount(features: BuildFeature[]): number {
  return features.filter((f) => isColumnStatus(f.column)).length;
}

// One board card: a feature paired with its own owning epic, for the
// card's kicker.
export interface BoardCard {
  epic: BuildEpic;
  feature: BuildFeature;
}

// visibleCards flattens the already-selected epics' own features, in the
// served order (epics in the shared epic order, each epic's own features in
// the shared feature order - the caller hands this the epics array already
// filtered down to the matched selection, itself a filter over
// BuildBoard.epics that preserves that same order).
export function visibleCards(selectedEpics: BuildEpic[]): BoardCard[] {
  const cards: BoardCard[] = [];
  for (const epic of selectedEpics) {
    for (const feature of epic.features) {
      cards.push({ epic, feature });
    }
  }
  return cards;
}

// columnCards is the one status filter every column applies to the shared
// sequence above. It filters on the server's column (BuildFeature.column),
// not the stored status: a ready feature with a task in flight lands in
// Active.
export function columnCards(cards: BoardCard[], status: string): BoardCard[] {
  return cards.filter((c) => c.feature.column === status);
}
