// Mask the text that differs from one real run to the next, so a fresh run can
// be compared with the committed snapshots. Order matters: the longer, more
// specific forms go first.
const RULES: Array<[RegExp, string]> = [
  [/\.tiller\/events\/\d{4}\/\d{2}\/\d{2}\//g, ".tiller/events/<DATE>/"],
  // git's diffstat for event files: their size varies with the volatile text inside.
  [/(\.tiller\/events\/[^\s|]*\s+\| )\d+ [+-]+/g, "$1<N> <CHANGES>"],
  [/^( \d+ files? changed)(?:, \d+ insertions?\(\+\))?(?:, \d+ deletions?\(-\))?$/, "$1, <STATS>"],
  [/\b\d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC\b/g, "<TIME>"],
  [/\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?/g, "<TIME>"],
  [/\((?:\d+(?:\.\d+)?(?:ms|s|m))+\)/g, "(<DUR>)"],
  [/(?:\/private)?\/(?:var\/folders\/[^\s"'`)]*|tmp\/[^\s"'`)]*)/g, "<TMP>"],
  [/\b[A-Z]{1,2}-[a-z0-9]{6}\b/g, "<ID>"],
  // A SHA (or a 64-char evidence hash) has at least one digit, so words such
  // as "defaced" are left alone.
  [/\b(?=[0-9a-f]*\d)[0-9a-f]{7,64}\b/g, "<SHA>"],
];

export function normalize(line: string): string {
  return RULES.reduce((s, [re, to]) => s.replace(re, to), line);
}
