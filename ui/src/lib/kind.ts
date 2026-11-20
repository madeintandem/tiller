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
  ["batch", "Batch"],
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

// kindPlural is the heading for several of one kind (a search section, a
// filter tab). Adding an s is right for every noun but the ones listed here.
const KIND_PLURAL: ReadonlyMap<string, string> = new Map([["batch", "Batches"]]);

export function kindPlural(noun: string): string {
  return KIND_PLURAL.get(noun) ?? `${kindLabel(noun)}s`;
}
