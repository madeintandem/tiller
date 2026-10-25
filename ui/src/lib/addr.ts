// addrOf is the single address a copyable command takes: the canonical key for
// a folder-backed noun (epic, feature, and a task, which owns a folder and a
// three-segment key of its own now), the id for a question, the one remaining
// ledger item and so the one noun with no key to name. That is exactly what the
// CLI's own address resolution accepts back at the terminal, and it is the same
// choice views/queue.go's queueAddr makes server-side, so a command copied off a
// node page and a command copied off a queue row address one node one way.
// Shared so the two surfaces that build a command from a node page, the
// attention strip and the unwritten-spec line, cannot drift apart.
//
// This addresses a COMMAND. The console's own links still go by id, which is
// now a choice rather than a constraint: the /node/:noun/* splat route accepts
// a key too, so a task's key works in the browser as well as the terminal. Ids
// stay the internal link target because they are stable under a rename.

export function addrOf(node: { key?: string; id: string }): string {
  return node.key !== undefined && node.key !== "" ? node.key : node.id;
}
