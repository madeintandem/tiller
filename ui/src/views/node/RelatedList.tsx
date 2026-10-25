// One row per adjacent node, in the console's locked row grammar: the title
// leads as the link, then a muted mono address, then the state. A materialized
// question's address is its Q-id; every keyed noun retains its key. A dangling
// edge arrives as a bare-id row (empty noun, ref repeating the id), and
// RefLink degrades that ref to honest plain text without inventing a second
// address or a link to nowhere.

import type { RelatedNode } from "../../api";
import Chip from "../../components/Chip";
import RefLink from "../../components/RefLink";
import { has } from "../../lib/has";


export default function RelatedList({ items }: { items: RelatedNode[] }) {
  return (
    <ul className="related-list">
      {items.map((node) => (
        <li key={node.id}>
          <RefLink
            node={node}
            label={has(node.title) ? node.title : node.ref}
            className="related-title"
          />
          {node.noun === "question" && has(node.title) ? (
            <span className="mono muted related-key">{node.id}</span>
          ) : (
            has(node.key) && <span className="mono muted related-key">{node.key}</span>
          )}
          {/* A dangling edge carries no status; a chip with no word would be
              color alone, which the palette rule forbids outright. */}
          {has(node.status) && <Chip status={node.status} />}
        </li>
      ))}
    </ul>
  );
}
