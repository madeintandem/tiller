package relationships

import "sort"

// Field coverage checklist (recorded relationships, not lifecycle conclusions):
//
// Epic: folder layout -> contains; depends_on -> depends_on; surface.paths ->
// covers(work_surface); source -> sourced_from. Feature: the same, plus
// supersedes/superseded_by -> supersedes and assumption_basis question keys ->
// assumes. Task: folder layout, depends_on, supersedes/superseded_by; no inferred
// surface inheritance. Bug/chore/tweak: depends_on, attached_to, resolved_by,
// duplicate_of, source and supersedes/superseded_by -> their named relationships.
// Question: blocks -> blocks irrespective of status; source -> sourced_from. Amendment: operation
// endpoints/application evidence -> amends (role/state/operation/evidence stay
// provenance), triggered_by/amends_design/implements -> named relationships,
// source and operation.source -> sourced_from. Proposed operations NEVER write
// supersession facts. Node prose/eval files -> has_document; Markdown literal
// E/F/T references -> mentions; document coordinate -> at_path.
//
// Baseline docs.path identifies a document; describes -> covers(baseline_description);
// derived_from -> derived_from with revision evidence. Eval criterion binding.ref
// -> covers(acceptance_binding) only under the existing path classification;
// preserve.baseline surface/artifact -> preserves with criterion, role, revision.
//
// Explicit non-endpoints: IDs/keys/title/status/owner are vertex attributes;
// blocked/change_type/review_grain/done_basis/resolution are lifecycle metadata;
// order and project/shelf order remain ordered metadata, never dependencies;
// branch/PR/review_ref, accepted_against/verified_against pins, verified surface
// inventories, baseline confirmation, question answer/answer_revision/due,
// assumption reasons/reviewed revisions, gates/attestations/policy violations,
// task evidence, proposed_by, amendment base_revision/reasons/preconditions,
// contract criterion IDs, rubric/test IDs and shell run strings retain their
// existing local scope. Arbitrary URLs/sentences and human design frontmatter
// are not inferred paths. Opaque citations are source-scoped references, not
// invented lifecycle entities. External coordinates never alias local paths.
var catalog = []RelationKind{
	{Name: "contains", Inverse: "contained_by", FromKinds: []string{"epic", "feature", "reference"}, ToKinds: []string{"feature", "task"}},
	{Name: "depends_on", Inverse: "depended_on_by", FromKinds: []string{"epic", "feature", "task", "bug", "chore", "tweak"}, ToKinds: []string{"epic", "feature", "task", "bug", "chore", "tweak", "reference"}},
	{Name: "supersedes", Inverse: "superseded_by", FromKinds: []string{"feature", "task", "bug", "chore", "tweak", "reference"}, ToKinds: []string{"feature", "task", "bug", "chore", "tweak", "reference"}},
	{Name: "attached_to", Inverse: "attached_work", FromKinds: []string{"bug", "chore", "tweak"}, ToKinds: []string{"epic", "feature", "task", "reference"}},
	{Name: "resolved_by", Inverse: "resolves", FromKinds: []string{"bug", "chore", "tweak"}, ToKinds: []string{"epic", "feature", "task", "reference"}},
	{Name: "duplicate_of", Inverse: "duplicates", FromKinds: []string{"bug", "chore", "tweak"}, ToKinds: []string{"bug", "chore", "tweak", "reference"}},
	{Name: "blocks", Inverse: "blocked_by", FromKinds: []string{"question"}, ToKinds: []string{"epic", "feature", "task", "reference"}},
	{Name: "assumes", Inverse: "assumed_by", FromKinds: []string{"feature"}, ToKinds: []string{"question", "reference"}},
	{Name: "has_document", Inverse: "document_of", FromKinds: []string{"epic", "feature", "task", "bug", "chore", "tweak", "amendment"}, ToKinds: []string{"document"}},
	{Name: "covers", Inverse: "covered_by", FromKinds: []string{"epic", "feature", "document"}, ToKinds: []string{"path", "external_path", "reference"}},
	{Name: "mentions", Inverse: "mentioned_by", FromKinds: []string{"document"}, ToKinds: []string{"epic", "feature", "task", "reference"}},
	{Name: "amends", Inverse: "amended_by", FromKinds: []string{"amendment"}, ToKinds: []string{"epic", "feature", "task", "question", "reference"}},
	{Name: "triggered_by", Inverse: "triggers", FromKinds: []string{"amendment"}, ToKinds: []string{"document", "reference"}},
	{Name: "amends_design", Inverse: "amended_by_design", FromKinds: []string{"amendment"}, ToKinds: []string{"document", "reference"}},
	{Name: "implements", Inverse: "implemented_by", FromKinds: []string{"amendment"}, ToKinds: []string{"document", "reference"}},
	{Name: "sourced_from", Inverse: "source_for", FromKinds: []string{"epic", "feature", "task", "bug", "chore", "tweak", "question", "amendment"}, ToKinds: []string{"document", "path", "reference"}},
	{Name: "derived_from", Inverse: "derivation_of", FromKinds: []string{"document"}, ToKinds: []string{"path", "external_path", "reference"}},
	{Name: "preserves", Inverse: "preserved_by", FromKinds: []string{"feature"}, ToKinds: []string{"path", "external_path", "reference"}},
	{Name: "at_path", Inverse: "document_at", FromKinds: []string{"document"}, ToKinds: []string{"path"}},
}

// Catalog returns the canonical catalog, including the allowed endpoint kinds.
func (index *Index) Catalog() []RelationKind {
	out := make([]RelationKind, len(catalog))
	for n, kind := range catalog {
		out[n] = kind
		out[n].FromKinds = append([]string(nil), kind.FromKinds...)
		out[n].ToKinds = append([]string(nil), kind.ToKinds...)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

func (index *Index) Directions() []string {
	out := make([]string, 0, len(catalog)*2)
	for _, kind := range catalog {
		out = append(out, kind.Name, kind.Inverse)
	}
	sort.Strings(out)
	return out
}

func relationDirection(direction string) (canonical string, reverse bool) {
	for _, kind := range catalog {
		if direction == kind.Name {
			return kind.Name, false
		}
		if direction == kind.Inverse {
			return kind.Name, true
		}
	}
	return "", false
}
