package relationships

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/madeintandem/tiller/internal/evalref"
	"github.com/madeintandem/tiller/internal/store"
)

// AddContracts consumes parsed contracts keyed by physical owning feature ID.
// It never runs a binding, checks the target's existence, or adopts a contract
// that declares another feature as its owner.
func AddContracts(t *store.Tree, idx *Index, contracts map[string]*store.EvalFile, failures map[string]string) {
	ids := make([]string, 0, len(t.Features))
	for id := range t.Features {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		feature := t.Features[id]
		file, ok := NodePath(t, feature, "eval.md")
		location := Source{File: file, Field: "contract"}
		subjects := []string{id}
		if ok {
			subjects = append(subjects, DocumentID(file))
		}
		diagnose := func(code, message string) {
			idx.AddDiagnostic(Diagnostic{Code: code, Message: message, Facet: "contracts", Source: &location, Subjects: subjects}, true)
		}
		if !ok {
			diagnose("contract_path_unavailable", fmt.Sprintf("%s has no addressable acceptance contract path", id))
			continue
		}
		if message := failures[id]; message != "" {
			diagnose("contract_unavailable", fmt.Sprintf("%s: %s", file, message))
			continue
		}
		contract := contracts[id]
		if contract == nil {
			continue
		}
		doc := documentReference(idx, file)
		idx.AddEdge("has_document", id, doc, Source{File: file, Field: "document"})
		if contract.Node != id {
			diagnose("contract_owner_mismatch", fmt.Sprintf("%s belongs physically to %s but declares %q", file, id, contract.Node))
			continue
		}
		// Bindings remain criterion-scoped metadata even when their ref is opaque or
		// a run takes execution precedence. They do not invent coverage endpoints.
		for n, criterion := range contract.Criteria {
			for _, source := range contractSources(file, n, criterion) {
				idx.AddEdge("has_document", id, doc, source)
			}
		}
		if len(contract.Criteria) == 0 {
			continue
		}
		if err := contract.ValidateContract(); err != nil {
			diagnose("contract_invalid", fmt.Sprintf("%s: %v", file, err))
			continue
		}
		for n, criterion := range contract.Criteria {
			for _, source := range contractSources(file, n, criterion) {
				if source.Classification != "path" {
					continue
				}
				source.Basis = "acceptance_binding"
				source.Scope = strings.TrimSpace(source.Raw)
				idx.AddEdge("covers", id, recordedPath(t, idx, id, source.Scope, source), source)
			}
			if criterion.Polarity != store.PolarityPreserve || criterion.Baseline == nil {
				continue
			}
			for _, reference := range []struct{ role, raw string }{{"surface", criterion.Baseline.Surface}, {"artifact", criterion.Baseline.Artifact}} {
				if reference.raw == "" {
					continue
				}
				provenance := Source{File: file, Field: fmt.Sprintf("criteria[%d].baseline.%s", n, reference.role), Criterion: criterion.ID, Role: reference.role, Raw: reference.raw, Revision: RevisionOf(criterion.Baseline.CapturedAt)}
				var target string
				if literalPath(reference.raw) {
					target = recordedPath(t, idx, id, reference.raw, provenance)
				} else {
					target = opaqueReference(idx, id, provenance, reference.raw)
				}
				idx.AddEdge("preserves", id, target, provenance)
			}
		}
	}
}

// contractSources returns one classified source per ref item of a criterion's
// binding, or a single source with no Raw when the binding names no ref. A
// scalar ref keeps the field criteria[n].binding.ref; list items carry
// criteria[n].binding.ref[i].
func contractSources(file string, n int, criterion store.EvalCriterion) []Source {
	binding := criterion.Binding
	field := fmt.Sprintf("criteria[%d].binding.ref", n)
	if len(binding.Ref) == 0 {
		return []Source{{File: file, Field: field, Criterion: criterion.ID, BindingType: binding.Type, Classification: classifyRef(binding.Type, "")}}
	}
	sources := make([]Source, 0, len(binding.Ref))
	for i, ref := range binding.Ref {
		itemField := field
		if len(binding.Ref) > 1 {
			itemField = fmt.Sprintf("%s[%d]", field, i)
		}
		sources = append(sources, Source{File: file, Field: itemField, Criterion: criterion.ID, BindingType: binding.Type, Raw: ref, Classification: classifyRef(binding.Type, ref)})
	}
	return sources
}

func classifyRef(bindingType, ref string) string {
	ref = strings.TrimSpace(ref)
	if bindingType != store.BindingTest || !evalref.IsPathShaped(ref) {
		return "opaque"
	}
	if !literalPath(ref) {
		return "opaque_path"
	}
	if _, err := PathID(ref); err != nil {
		return "opaque_path"
	}
	return "path"
}

// Path-shaped selectors and glob patterns remain raw metadata. Splitting one
// into an alleged file coordinate would claim more than the author recorded.
func literalPath(raw string) bool {
	return raw != "" && strings.IndexFunc(raw, unicode.IsSpace) < 0 && !strings.ContainsAny(raw, "*?[]{};|&<>!$`\"'()#:")
}
