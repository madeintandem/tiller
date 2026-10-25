// stripLeadingTitle drops a spec's opening `# Heading` when it only repeats the
// title the page already renders in its own serif h1 directly above. Prose
// demotes headings by one, so that line came out as an h2 saying the node's
// name a second time, in a second voice, on every written spec in the tree. A
// heading that says something else is content and stays.
//
// This used to live in lib/clampMarkdown.ts beside the length-based fold that
// function name promised. Task 17 (console v2, node page rebuild) killed the
// fold outright, per the amended spec's own words: "Prose renders whole: no
// read-more folds and no truncated bodies" - a spec you must unfold is a spec
// that goes unread, and the tabs already bound the page's width. Title
// deduplication is a different, unrelated concern that outlived the fold, so
// it moved here rather than stranding it in a file named for the thing that
// died.

export function stripLeadingTitle(markdown: string, title: string | undefined): string {
  if (title === undefined || title.trim() === "") {
    return markdown;
  }
  const match = /^\s*#\s+(.+?)\s*(\n|$)/.exec(markdown);
  if (match === null || match[1].trim().toLowerCase() !== title.trim().toLowerCase()) {
    return markdown;
  }
  return markdown.slice(match[0].length).replace(/^\n+/, "");
}
