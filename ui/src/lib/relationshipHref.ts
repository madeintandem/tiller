import type { RelationshipVertex } from "../api";

// This is URL construction, not reference resolution. Only the server can
// grant a vertex a destination; recorded coordinates alone never grant one.
export function relationshipHref(vertex: RelationshipVertex): string | null {
  if (vertex.unresolved || vertex.kind === "path" || vertex.kind === "external_path") return null;
  const destination = vertex.destination;
  if (destination?.kind === "node" && destination.noun && destination.id) {
    return `/node/${encodeURIComponent(destination.noun)}/${encodeURIComponent(destination.id)}`;
  }
  if (destination?.kind === "file" && destination.path) {
    return relationshipFileHref(destination.path, destination.line);
  }
  return null;
}

// Assertion locations are exact file coordinates, unlike free-form source
// citations. Only Markdown has a reader; YAML fields and code remain copyable.
// /api/prose performs the filesystem confinement check when the reader opens.
export function relationshipFileHref(path: string, line = 1): string | null {
  if (!/\.md$/i.test(path) || /[\\\0]/.test(path) || /^[^/]*:/.test(path) ||
      path.split("/").some((part) => part === "" || part === "." || part === "..")) return null;
  if (!Number.isSafeInteger(line) || line < 1) return null;
  return `/file/${path.split("/").map(encodeURIComponent).join("/")}#L${line}`;
}
