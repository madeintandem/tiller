// Plain words for a relationship source's details (SourceCitation): the
// field it was read from, the basis of the edge, the eval binding and how its
// ref classified, an amendment effect's role, state, and scope, and a pinned
// revision's kind. The tables cover every value internal/relationships
// assigns (structural.go, documents.go, provenance.go, contracts.go,
// amendments.go) and store/revision.go's revision kinds. Anything else that
// reads as a lower-case token is humanized; a value that is a path, an id, or
// any other recorded text (a scope can be a described path) is left exactly
// as it is. The words are lower-case: they sit mid-line, joined by " · ".

import { amendmentOpLabel } from "./fieldWords";

const TOKEN = /^[a-z][a-z0-9]*(?:[_-][a-z0-9]+)*$/;

function plainToken(value: string): string {
  return TOKEN.test(value) ? value.replace(/[_-]/g, " ") : value;
}

function lowerFirst(words: string): string {
  return words.charAt(0).toLowerCase() + words.slice(1);
}

const BASIS: Record<string, string> = {
  work_surface: "work surface",
  document_coordinate: "document location",
  baseline_description: "baseline description",
};

// An amendment effect's basis is the staged operation (amendments.go), worded
// as the amendment page words it.
const AMENDMENT_OPS = new Set([
  "revise",
  "supersede",
  "cancel",
  "add-epic",
  "add-feature",
  "add-task",
  "add-question",
  "move-feature",
  "correct-eval",
]);

export function sourceBasisLabel(basis: string): string {
  if (BASIS[basis] !== undefined) return BASIS[basis];
  if (AMENDMENT_OPS.has(basis)) return lowerFirst(amendmentOpLabel(basis));
  return plainToken(basis);
}

// A field is a locator into the recorded file ("surface.paths",
// "criteria[2].binding.ref", "assumption_basis.Q-abc"): each dotted segment
// that is a token reads as words, and indexes, ids, and paths stay as they
// are. A described path that itself holds dots (as-is/model.md) keeps them.
export function sourceFieldLabel(field: string): string {
  const segments: string[] = [];
  for (const part of field.split(".")) {
    const last = segments.length - 1;
    // A path's extension rides back onto its path (as-is/model.md).
    if (last >= 0 && segments[last].includes("/") && /^[a-z0-9]+$/i.test(part)) {
      segments[last] = `${segments[last]}.${part}`;
    } else {
      segments.push(part);
    }
  }
  return segments
    .map((segment) => {
      const keyed = /^([a-z][a-z0-9_]*)(\[\d+\])?$/.exec(segment);
      return keyed === null ? segment : plainToken(keyed[1]) + (keyed[2] ?? "");
    })
    .join(" ");
}

const BINDING: Record<string, string> = {
  test: "bound to a test",
  command: "bound to a command",
  manual: "a manual check",
  eval: "bound to a rubric",
};

export function sourceBindingLabel(binding: string): string {
  return BINDING[binding] ?? `bound to ${plainToken(binding)}`;
}

// bindingTypeLabel is the bare type word a contract row's type column shows
// (test, command, manual, eval): each known type is already one plain word,
// and one this build does not know reads as words, never as minted.
export function bindingTypeLabel(binding: string): string {
  return plainToken(binding);
}

const CLASSIFICATION: Record<string, string> = {
  path: "a path",
  opaque: "not a path",
  opaque_path: "path-like, not checked",
};

export function sourceClassificationLabel(value: string): string {
  return CLASSIFICATION[value] ?? plainToken(value);
}

const ROLE: Record<string, string> = {
  addition_parent: "parent of the addition",
  move_source_parent: "parent moved from",
  move_destination_parent: "parent moved to",
  question_block: "blocked by the question",
};

export function sourceRoleLabel(role: string): string {
  return ROLE[role] ?? plainToken(role);
}

export function sourceStateLabel(state: string): string {
  return plainToken(state);
}

export function sourceScopeLabel(scope: string): string {
  return plainToken(scope);
}

const REVISION_KIND: Record<string, string> = {
  git_blob: "Git blob",
  git_commit: "Git commit",
  git_tree: "Git tree",
  sha256: "SHA-256",
};

export function sourceRevisionKindLabel(kind: string): string {
  return REVISION_KIND[kind] ?? plainToken(kind);
}
