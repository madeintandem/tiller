// A bug's severity as a word. The tint (SeverityChip.css) is decoration only:
// the label is always rendered, so the chip reads the same with no color.

import type { Severity } from "../api";
import "./SeverityChip.css";

const LABELS: Record<Severity, string> = {
  blocker: "Blocker",
  important: "Important",
  medium: "Medium",
  minor: "Minor",
};

export default function SeverityChip({ severity }: { severity?: Severity }) {
  return (
    <span className="severity-chip" data-severity={severity ?? "unrated"}>
      {severity === undefined ? "Unrated" : LABELS[severity]}
    </span>
  );
}
