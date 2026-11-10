// displayStatus is the state word the console draws for a node: an epic's or
// a feature's served board column (gen.BoardStatus) when it carries one,
// otherwise its stored status. Under feature-grain review a feature's start
// sits on the unmerged review branch, so its stored status can read ready
// while its tasks are in flight; the column reads active, the same answer
// Build gives. A ready epic with a task in flight reads active the same way.
export function displayStatus(node: { status: string; column?: string }): string {
  return node.column ?? node.status;
}
