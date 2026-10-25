// The words of one Up next row: the label its node link shows and the reason
// beside it, worded as `tiller next` words them.

import type { NextCandidate, NextNode } from "../../api";
import { addrOf } from "../../lib/addr";
import { sentence } from "../../lib/sentence";

// nextLabel names a question by its title, since its Q-id says nothing, and
// every other node by its address.
export function nextLabel(node: NextNode): string {
  if (node.noun === "question" && node.title !== undefined && node.title !== "") {
    return node.title;
  }
  return addrOf(node);
}

// nextReason is the row's reason as a sentence. A question row's reason ends
// on the question's title, which the link already shows, so that tail is
// dropped. A move in flight on another branch names it, as the CLI line does.
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
