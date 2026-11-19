// displayStatus is the state word the console draws for a node: an epic's or
// a feature's served board column (gen.BoardStatus) when it carries one,
// otherwise its stored status. Under feature-grain review a feature's start
// sits on the unmerged review branch, so its stored status can read ready
// while its tasks are in flight; the column reads active, the same answer
// Build gives. A ready epic with a task in flight reads active the same way.
//
// The status is optional because a relationship vertex's is (a document or a
// path has none); a node with neither a column nor a status reads as the
// empty string, so a caller asks displayStatus(vertex) !== "" rather than
// repeating the rule.
//
// A bug, chore, or tweak in a live batch keeps its stored status, usually
// ready, while the batch does its work. Drawn as ready it would pass for work
// waiting to start, so a ready member reads batched, the derived phase the
// CLI prints beside it. A proposed member still waits on the human's triage
// and a crossed one is done, so both keep their own word.
export const BATCHED = "batched";

export function displayStatus(node: { status?: string; column?: string; activity?: string }): string {
  if (node.activity === BATCHED && node.status === "ready") {
    return BATCHED;
  }
  return node.column ?? node.status ?? "";
}
