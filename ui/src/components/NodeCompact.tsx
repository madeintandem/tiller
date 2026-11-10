// NodeCompact is the node page's one-line header, shown inside the pinned rail
// once the full header has scrolled away: the noun pill, the displayed status
// chip (shape glyph plus plain words, never color alone), and the title.

import type { NodeView } from "../api";
import { displayStatus } from "../lib/displayStatus";
import { kindLabel } from "../lib/kind";
import { statusLabel } from "../lib/statusLabel";
import Chip from "./Chip";
import { handleOf } from "./NodeHeader";

export default function NodeCompact({ node }: { node: NodeView }) {
  const status = displayStatus(node);
  return (
    <div className="node-compact">
      <span className="kind-pill">{kindLabel(node.noun)}</span>
      <Chip status={status}>{statusLabel(status)}</Chip>
      <span className="node-compact-title">{handleOf(node)}</span>
    </div>
  );
}
