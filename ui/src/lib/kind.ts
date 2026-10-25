// kindLabel is the word the node header's kind pill reads (node header pass,
// 2026-08-30: "I want to know I'm looking at an epic or feature or task more
// clearly"). Before it, a page's noun was only inferable from the id prefix
// inside the ref line's parentheses. One label per noun the console draws,
// and a capitalized fallback for a noun this build has never heard of, so the
// pill degrades to a readable word rather than a blank.
const KIND_LABEL: ReadonlyMap<string, string> = new Map([
  ["epic", "Epic"],
  ["feature", "Feature"],
  ["task", "Task"],
  ["bug", "Bug"],
  ["chore", "Chore"],
  ["tweak", "Tweak"],
  ["question", "Question"],
  ["amendment", "Amendment"],
]);

export function kindLabel(noun: string): string {
  const known = KIND_LABEL.get(noun);
  if (known !== undefined) {
    return known;
  }
  return noun.charAt(0).toUpperCase() + noun.slice(1);
}
