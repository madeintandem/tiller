// sourceHref maps a node's recorded source citation to its one-hop destination.
// HTTP(S) citations open externally. Local citations keep their authored path
// segments in the /file/* URL and use the locator's first line, including for a
// range. FileDetail then calls /api/prose, which owns all local path validation
// and resolution, including the design alias and repository-root rules.
export interface SourceLink {
  href: string;
  external: boolean;
}

export function sourceHref(source: string): SourceLink {
  if (/^https?:\/\//.test(source)) {
    return { href: source, external: true };
  }
  const hashAt = source.indexOf("#");
  const path = hashAt === -1 ? source : source.slice(0, hashAt);
  const fragment = hashAt === -1 ? "" : source.slice(hashAt + 1);
  const locator = /^L([1-9]\d*)/.exec(fragment);
  const line = locator === null ? "1" : locator[1];
  const encoded = path.split("/").map(encodeURIComponent).join("/");
  return { href: `/file/${encoded}#L${line}`, external: false };
}
