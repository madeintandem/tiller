package relationships

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// BuildStructural reads only the loaded state. In particular it neither reads
// prose/contracts/history nor attaches a cache to the mutable tree.
func BuildStructural(tree *store.Tree) *Index {
	index := New()
	if tree == nil {
		return index
	}
	nodes := structuralNodes(tree)
	builder := newStructuralBuilder(tree, index, nodes)
	builder.dependencies(nodes)
	for _, node := range nodes {
		switch value := node.(type) {
		case *store.Feature:
			if value.EpicKey != "" {
				source := builder.source(node, "layout.epic", value.EpicKey)
				parent := builder.endpoint(node.NodeID(), value.EpicKey, []string{store.NounEpic}, nil, source)
				index.AddEdge("contains", parent, value.ID, source)
			}
			builder.supersession(node, value.Supersedes, value.SupersededBy)
			builder.surface(node, value.Surface.Paths)
			for question, assumption := range value.AssumptionBasis {
				source := builder.source(node, "assumption_basis."+question, question)
				if assumption.AnswerRevision.Kind != "" {
					source.Revision = RevisionOf(assumption.AnswerRevision)
				}
				target := builder.endpoint(node.NodeID(), question, []string{store.NounQuestion}, nil, source)
				index.AddEdge("assumes", value.ID, target, source)
			}
		case *store.Epic:
			builder.surface(node, value.Surface.Paths)
		case *store.Task:
			if value.FeatureID != "" {
				source := builder.source(node, "layout.feature", value.FeatureID)
				parent := builder.endpoint(node.NodeID(), value.FeatureID, []string{store.NounFeature}, nil, source)
				index.AddEdge("contains", parent, value.ID, source)
			}
			builder.supersession(node, value.Supersedes, value.SupersededBy)
		case store.WorkNode:
			body := value.Body()
			builder.supersession(node, body.Supersedes, body.SupersededBy)
			for _, field := range []struct {
				name, raw string
				nouns     []string
			}{
				{"attached_to", body.AttachedTo, []string{store.NounEpic, store.NounFeature, store.NounTask}},
				{"resolved_by", body.ResolvedBy, []string{store.NounEpic, store.NounFeature, store.NounTask}},
				{"duplicate_of", body.DuplicateOf, []string{node.Noun()}},
			} {
				if field.raw == "" {
					continue
				}
				source := builder.source(node, field.name, field.raw)
				target := builder.endpoint(node.NodeID(), field.raw, field.nouns, nil, source)
				index.AddEdge(field.name, node.NodeID(), target, source)
			}
		case *store.Question:
			for n, raw := range value.Blocks {
				source := builder.source(node, fmt.Sprintf("questions[%s].blocks[%d]", value.ID, n), raw)
				target := builder.endpoint(node.NodeID(), raw, []string{store.NounEpic, store.NounFeature, store.NounTask}, nil, source)
				index.AddEdge("blocks", value.ID, target, source)
			}
		}
	}
	builder.checkSupersession()
	AddProvenance(tree, index)
	index.Finalize()
	return index
}

// BuildDependencies resolves against the whole tree before extracting a supplied
// ordering subset. Excluding a sibling must never turn an ambiguous key into a
// successful reference. Supplemental provenance and documents are not loaded.
func BuildDependencies(tree *store.Tree, nodes []store.Node) *Index {
	index := New()
	// Ordering has overwhelmingly ID-based edges. Resolve those directly from
	// Tree's maps; only a recorded legacy key needs a scan of its noun's records.
	builder := &structuralBuilder{tree: tree, index: index}
	builder.dependencies(nodes)
	index.Finalize()
	return index
}

type structuralBuilder struct {
	tree  *store.Tree
	index *Index
	byID  map[string]store.Node
	byKey map[string][]store.Node
}

func newStructuralBuilder(tree *store.Tree, index *Index, nodes []store.Node) *structuralBuilder {
	builder := &structuralBuilder{tree: tree, index: index, byID: make(map[string]store.Node, len(nodes)), byKey: map[string][]store.Node{}}
	for _, node := range nodes {
		if _, exists := builder.byID[node.NodeID()]; exists {
			continue
		}
		builder.byID[node.NodeID()] = node
		if key := node.NodeKey(); key != nil && *key != "" {
			builder.byKey[*key] = append(builder.byKey[*key], node)
		}
		index.AddVertex(structuralVertex(tree, node))
	}
	return builder
}

func (builder *structuralBuilder) lookup(raw string) store.Node {
	if builder.byID != nil {
		return builder.byID[raw]
	}
	tree := builder.tree
	if node := tree.Epics[raw]; node != nil {
		return node
	}
	if node := tree.Features[raw]; node != nil {
		return node
	}
	if node := tree.Tasks[raw]; node != nil {
		return node
	}
	if node := tree.Bugs[raw]; node != nil {
		return node
	}
	if node := tree.Chores[raw]; node != nil {
		return node
	}
	if node := tree.Tweaks[raw]; node != nil {
		return node
	}
	if node := tree.Questions[raw]; node != nil {
		return node
	}
	if node := tree.Amendments[raw]; node != nil {
		return node
	}
	return nil
}

func (builder *structuralBuilder) keyCandidates(raw string, nouns []string) []store.Node {
	var candidates []store.Node
	add := func(node store.Node) {
		if key := node.NodeKey(); key != nil && *key == raw {
			candidates = append(candidates, node)
		}
	}
	if builder.byKey != nil {
		for _, node := range builder.byKey[raw] {
			for _, noun := range nouns {
				if node.Noun() == noun {
					add(node)
					break
				}
			}
		}
		return candidates
	}
	for _, noun := range nouns {
		switch noun {
		case store.NounEpic:
			for _, node := range builder.tree.Epics {
				add(node)
			}
		case store.NounFeature:
			for _, node := range builder.tree.Features {
				add(node)
			}
		case store.NounTask:
			for _, node := range builder.tree.Tasks {
				add(node)
			}
		case store.NounQuestion:
			for _, node := range builder.tree.Questions {
				add(node)
			}
		case store.NounAmendment:
			for _, node := range builder.tree.Amendments {
				add(node)
			}
		default:
			if store.IsWorkNoun(noun) {
				for _, node := range builder.tree.WorkNodesOf(noun) {
					add(node)
				}
			}
		}
	}
	return candidates
}

func (builder *structuralBuilder) resolved(node store.Node) string {
	if _, exists := builder.index.Vertices[node.NodeID()]; !exists {
		builder.index.AddVertex(structuralVertex(builder.tree, node))
	}
	return node.NodeID()
}

func structuralNodes(tree *store.Tree) []store.Node {
	nodes := make([]store.Node, 0, len(tree.Epics)+len(tree.Features)+len(tree.Tasks)+len(tree.Bugs)+len(tree.Chores)+len(tree.Tweaks)+len(tree.Questions)+len(tree.Amendments))
	for _, node := range tree.Epics {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Features {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Tasks {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Bugs {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Chores {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Tweaks {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Questions {
		nodes = append(nodes, node)
	}
	for _, node := range tree.Amendments {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(a, b int) bool { return nodes[a].NodeID() < nodes[b].NodeID() })
	return nodes
}

func structuralVertex(tree *store.Tree, node store.Node) Vertex {
	vertex := Vertex{ID: node.NodeID(), Kind: node.Noun(), Status: node.NodeStatus(), Destination: &Destination{Kind: "node", Noun: node.Noun(), ID: node.NodeID()}}
	if key := node.NodeKey(); key != nil {
		vertex.Key = *key
	}
	vertex.Path, _ = NodePath(tree, node, "yml")
	switch value := node.(type) {
	case *store.Epic:
		vertex.Title = value.Title
	case *store.Feature:
		vertex.Title = value.Title
	case *store.Task:
		vertex.Title, vertex.Owner = value.Title, value.Owner
	case store.WorkNode:
		vertex.Title, vertex.Owner = value.Body().Title, value.Body().Owner
	case *store.Question:
		vertex.Title, vertex.Owner = value.Title, value.Owner
	case *store.Amendment:
		vertex.Title = value.Title
	}
	if node.Noun() == store.NounEpic || node.Noun() == store.NounFeature {
		vertex.Contributors = tree.Contributors[node.NodeID()]
		if vertex.Contributors == nil {
			vertex.Contributors = []store.Contributor{}
		}
	}
	return vertex
}

func (builder *structuralBuilder) source(node store.Node, field, raw string) Source {
	file, ok := NodePath(builder.tree, node, "yml")
	if !ok {
		file = "node:" + node.NodeID()
	}
	return Source{File: file, Field: field, Raw: raw}
}

func (builder *structuralBuilder) dependencies(nodes []store.Node) {
	seen := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		// Ordering retains the first assertion body for duplicate IDs, while
		// still rendering every supplied task carrying that ID.
		if seen[node.NodeID()] {
			continue
		}
		seen[node.NodeID()] = true
		builder.resolved(node)
		var raws []string
		var eligible func(store.Node) bool
		switch value := node.(type) {
		case *store.Epic:
			raws = value.DependsOn
		case *store.Feature:
			raws = value.DependsOn
		case *store.Task:
			raws = value.DependsOn
			eligible = func(target store.Node) bool {
				task, ok := target.(*store.Task)
				return ok && task.FeatureID == value.FeatureID
			}
		case store.WorkNode:
			raws = value.Body().DependsOn
		default:
			continue
		}
		for n, raw := range raws {
			source := builder.source(node, fmt.Sprintf("depends_on[%d]", n), raw)
			target := builder.endpoint(node.NodeID(), raw, []string{dependencyNoun(node.Noun(), raw)}, eligible, source)
			builder.index.AddEdge("depends_on", node.NodeID(), target, source)
		}
	}
}

// Legacy keys obey the same noun grammar as mutation-time dependency input.
// They are canonical keys, not local names or a cross-noun search fallback.
func dependencyNoun(owner, raw string) string {
	noun := referenceNoun(raw)
	if noun == "" {
		switch {
		case strings.HasPrefix(raw, "ADR-"), strings.HasPrefix(raw, "EV-"), strings.HasPrefix(raw, "AT-"):
			return ""
		case shelfKeyNoun(raw) != "":
			noun = shelfKeyNoun(raw)
		case strings.Count(raw, "/") == 2:
			noun = store.NounTask
		case strings.Count(raw, "/") == 1:
			noun = store.NounFeature
		case strings.Count(raw, "/") == 0:
			noun = store.NounEpic
		}
	}
	switch owner {
	case store.NounEpic, store.NounFeature:
		if noun == store.NounEpic || noun == store.NounFeature {
			return noun
		}
	case store.NounTask:
		if noun == store.NounTask {
			return noun
		}
	default:
		if store.IsWorkNoun(owner) && store.IsWorkNoun(noun) {
			return noun
		}
	}
	return ""
}

// endpoint resolves only the nouns and subset valid for this particular field.
// Ambiguity retains the assertion as a source-scoped unresolved coordinate;
// it never adopts a convenient current node or routes to an ineligible one.
func (builder *structuralBuilder) endpoint(from, raw string, nouns []string, eligible func(store.Node) bool, source Source) string {
	allowed := func(node store.Node) bool {
		for _, noun := range nouns {
			if node.Noun() == noun {
				return eligible == nil || eligible(node)
			}
		}
		return false
	}
	code := "unresolved_reference"
	if len(nouns) == 1 && nouns[0] == "" {
		code = "ineligible_reference"
	}
	if target := builder.lookup(raw); target != nil {
		if allowed(target) {
			return builder.resolved(target)
		}
		code = "ineligible_reference"
	} else if referenceNoun(raw) == "" {
		// Resolve noun-scoped key ambiguity before checking sibling eligibility,
		// matching the existing resolver rather than letting a subset choose.
		candidates := builder.keyCandidates(raw, nouns)
		if len(candidates) == 1 {
			if allowed(candidates[0]) {
				return builder.resolved(candidates[0])
			}
			code = "ineligible_reference"
		}
		if len(candidates) > 1 {
			code = "ambiguous_reference"
		}
	}
	kind := referenceNoun(raw)
	id := raw
	if kind == "" || code != "unresolved_reference" {
		kind = "reference"
		id = "reference:" + url.PathEscape(source.File) + ":" + url.PathEscape(source.Field) + ":" + url.PathEscape(raw)
	}
	builder.index.AddVertex(Vertex{ID: id, Kind: kind, Raw: raw, Unresolved: true})
	builder.index.AddDiagnostic(Diagnostic{Code: code, Message: fmt.Sprintf("%s: cannot resolve %q for %s", source.File, raw, source.Field), Facet: "structural", Source: &source, Subjects: []string{from, id}}, code == "ambiguous_reference")
	return id
}

// referenceNoun names the noun an id-form reference addresses by its prefix,
// or "" for a key or an id class with no vertex kind of its own.
func referenceNoun(raw string) string {
	for _, entry := range []struct{ prefix, noun string }{
		{"E-", store.NounEpic}, {"F-", store.NounFeature}, {"T-", store.NounTask},
		{"Q-", store.NounQuestion}, {"A-", store.NounAmendment},
	} {
		if strings.HasPrefix(raw, entry.prefix) {
			return entry.noun
		}
	}
	for _, shelf := range store.Shelves() {
		if strings.HasPrefix(raw, shelf.IDPrefix+"-") {
			return shelf.Noun
		}
	}
	return ""
}

// shelfKeyNoun names the shelf a key-form reference (bugs/<key>) sits on, or
// "" when raw is not under a shelf folder.
func shelfKeyNoun(raw string) string {
	for _, shelf := range store.Shelves() {
		if strings.HasPrefix(raw, shelf.Dir+"/") {
			return shelf.Noun
		}
	}
	return ""
}

func (builder *structuralBuilder) supersession(node store.Node, supersedes []string, supersededBy *string) {
	for n, raw := range supersedes {
		source := builder.source(node, fmt.Sprintf("supersedes[%d]", n), raw)
		target := builder.endpoint(node.NodeID(), raw, []string{node.Noun()}, nil, source)
		builder.index.AddEdge("supersedes", node.NodeID(), target, source)
	}
	if supersededBy != nil && *supersededBy != "" {
		source := builder.source(node, "superseded_by", *supersededBy)
		replacement := builder.endpoint(node.NodeID(), *supersededBy, []string{node.Noun()}, nil, source)
		builder.index.AddEdge("supersedes", replacement, node.NodeID(), source)
	}
}

func (builder *structuralBuilder) checkSupersession() {
	for id := range builder.index.Vertices {
		edges := builder.index.Adjacent(id, "superseded_by")
		if len(edges) > 1 {
			for _, edge := range edges {
				for _, source := range edge.Sources {
					builder.index.AddDiagnostic(Diagnostic{Code: "conflicting_supersession", Message: "multiple recorded replacements for " + id, Facet: "structural", Source: new(source), Subjects: []string{id, edge.From}}, false)
				}
			}
		}
	}
	state := map[string]uint8{}
	stack := []string{}
	position := map[string]int{}
	var visit func(string)
	visit = func(id string) {
		state[id] = 1
		position[id] = len(stack)
		stack = append(stack, id)
		for _, edge := range builder.index.Adjacent(id, "supersedes") {
			if state[edge.To] == 1 {
				for _, source := range edge.Sources {
					builder.index.AddDiagnostic(Diagnostic{Code: "conflicting_supersession", Message: "recorded supersession cycle from " + edge.From + " to " + edge.To, Facet: "structural", Source: new(source), Subjects: stack[position[edge.To]:]}, false)
				}
			} else if state[edge.To] == 0 {
				visit(edge.To)
			}
		}
		state[id] = 2
		stack = stack[:len(stack)-1]
		delete(position, id)
	}
	ids := make([]string, 0, len(builder.index.Vertices))
	for id := range builder.index.Vertices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if state[id] == 0 {
			visit(id)
		}
	}
}

func (builder *structuralBuilder) surface(node store.Node, paths []string) {
	for n, raw := range paths {
		source := builder.source(node, fmt.Sprintf("surface.paths[%d]", n), raw)
		source.Basis = "work_surface"
		source.Scope = raw
		id, err := PathID(raw)
		if err != nil {
			id = "reference:" + url.PathEscape(source.File) + ":" + url.PathEscape(source.Field) + ":" + url.PathEscape(raw)
			builder.index.AddVertex(Vertex{ID: id, Kind: "reference", Raw: raw, Unresolved: true})
			builder.index.AddDiagnostic(Diagnostic{Code: "invalid_path", Message: err.Error(), Facet: "structural", Source: &source, Subjects: []string{node.NodeID(), id}}, false)
		} else {
			builder.index.AddVertex(Vertex{ID: id, Kind: "path", Path: strings.TrimPrefix(id, "path:")})
		}
		builder.index.AddEdge("covers", node.NodeID(), id, source)
	}
}
