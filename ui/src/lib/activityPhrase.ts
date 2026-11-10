import { displayStatus } from "./displayStatus";
import { statusLabel } from "./statusLabel";

// withActivityPhrase appends a feature's derived rollup phase to its status
// for display, the comma idiom the Go side already renders for the CLI's
// human lenses (gen.FeatureDisplay: "active, awaiting check" / "active,
// awaiting sync"). Only the two feature phase values get a suffix here: an
// epic's plain active/idle activity rides the same wire field but is not one
// of these two, so it renders as the bare status, unchanged. A caller keeps
// Chip's own status prop on the raw status word (the color mapping must never
// see the composed phrase); this is for the visible text only. The word is
// displayStatus's (a node's board column when served), in statusLabel's
// words, so no wire word reaches the screen.
export function withActivityPhrase(node: { status: string; column?: string; activity?: string }): string {
  const status = statusLabel(displayStatus(node));
  switch (node.activity) {
    case "awaiting_check":
      return `${status}, awaiting check`;
    case "awaiting_sync":
      return `${status}, awaiting sync`;
    default:
      return status;
  }
}
