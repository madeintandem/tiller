// The words of one Up next row: the label its node link shows and the reason
// beside it, worded as `tiller next` words them.

import type { NextCandidate, NextNode } from "../../api";
import { addrOf } from "../../lib/addr";
import { sentence } from "../../lib/sentence";

// nextLabel names a node by its title, and falls back to its address when it
// has none. The row shows the address after the title in mono.
export function nextLabel(node: NextNode): string {
  if (node.title !== undefined && node.title !== "") {
    return node.title;
  }
  return addrOf(node);
}

// nextReason is the row's reason as a sentence. A reason that ends on the
// node's title, which the link already shows, drops that tail. A move in flight on another branch names it, as the CLI line does.
export function nextReason(candidate: NextCandidate): string {
  let reason = candidate.reason;
  const { node } = candidate;
  if (node !== undefined && nextLabel(node) !== addrOf(node)) {
    const title = nextLabel(node);
    if (reason === title) {
      reason = "";
    } else if (reason.endsWith("; " + title)) {
      reason = reason.slice(0, -("; " + title).length);
    }
  }
  const parts = [sentence(reason)];
  if (candidate.branch !== undefined && candidate.branch !== "") {
    parts.push("Continue it on `" + candidate.branch + "`.");
  }
  return parts.filter((p) => p !== "").join(" ");
}
