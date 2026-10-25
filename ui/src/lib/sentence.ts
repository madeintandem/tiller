// sentence words a ranked next reason or hint as the CLI does, mirroring the
// server's views.Sentence (internal/views/next_text.go) so Home and
// `tiller next` read alike. It ends the text with one period, unless it
// already ends on "?" or "!", and capitalizes a leading plain word. A reason
// may open with a node key ("checkout/alpha/t1 (T-x) is ready"), which is
// case-sensitive and never altered: only a first token holding no '/', '-',
// '(' or digit is capitalized.

export function sentence(text: string): string {
  const trimmed = text.trim().replace(/\.+$/, "");
  if (trimmed === "") {
    return "";
  }
  // A reason ending on a question's title already ends its sentence.
  const end = trimmed.endsWith("?") || trimmed.endsWith("!") ? "" : ".";
  const token = trimmed.split(" ", 1)[0];
  if (/[/\-(\p{Nd}]/u.test(token)) {
    return trimmed + end;
  }
  const first = String.fromCodePoint(trimmed.codePointAt(0) ?? 0);
  return first.toUpperCase() + trimmed.slice(first.length) + end;
}
