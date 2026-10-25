package views

import (
	"sort"

	"github.com/madeintandem/tiller/internal/relationships"
)

// RelationshipGraph carries semantic facts separately from the map display tree.
type RelationshipGraph struct {
	Vertices    []relationships.Vertex       `json:"vertices"`
	Edges       []relationships.Edge         `json:"edges"`
	Catalog     []relationships.RelationKind `json:"catalog"`
	Diagnostics []relationships.Diagnostic   `json:"diagnostics"`
	Complete    bool                         `json:"complete"`
}

func RelationshipGraphOf(index *relationships.Index) RelationshipGraph {
	out := RelationshipGraph{
		Vertices:    make([]relationships.Vertex, 0, len(index.Vertices)),
		Edges:       make([]relationships.Edge, 0, len(index.Edges)),
		Catalog:     index.Catalog(),
		Diagnostics: append([]relationships.Diagnostic{}, index.Diagnostics...),
		Complete:    index.Complete,
	}
	for _, v := range index.Vertices {
		out.Vertices = append(out.Vertices, v)
	}
	for _, e := range index.Edges {
		out.Edges = append(out.Edges, e)
	}
	sort.Slice(out.Vertices, func(i, j int) bool { return out.Vertices[i].ID < out.Vertices[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].ID < out.Edges[j].ID })
	return out
}

type BacklinkSource struct {
	Vertex      relationships.Vertex  `json:"vertex"`
	OwnerVertex *relationships.Vertex `json:"owner_vertex,omitempty"`
	Edges       []relationships.Edge  `json:"edges"`
}

type BacklinkGroup struct {
	Kind    string           `json:"kind"`
	Sources []BacklinkSource `json:"sources"`
}

type BacklinksView struct {
	Groups      []BacklinkGroup            `json:"groups"`
	Count       int                        `json:"count"`
	Diagnostics []relationships.Diagnostic `json:"diagnostics"`
	Complete    bool                       `json:"complete"`
}

// BacklinksOf includes citations to this identity and its owned documents.
// Dedicated relationship panels retain ownership of dependency and work edges.
func BacklinksOf(index *relationships.Index, id string) BacklinksView {
	out := BacklinksView{Groups: []BacklinkGroup{}, Complete: index.Complete}
	targets := []string{id}
	for _, e := range index.Adjacent(id, "has_document") {
		targets = append(targets, e.To)
	}
	out.Diagnostics = index.DiagnosticsFor(targets...)
	inverse := make(map[string]string)
	for _, kind := range index.Catalog() {
		inverse[kind.Name] = kind.Inverse
	}
	groups := make(map[string]map[string]*BacklinkSource)
	seen := make(map[string]bool)
	for _, target := range targets {
		for _, edgeID := range index.Reverse[target] {
			edge := index.Edges[edgeID]
			switch edge.Kind {
			case "contains", "depends_on", "supersedes", "attached_to", "resolved_by", "duplicate_of", "blocks", "amends", "has_document", "at_path":
				continue
			}
			kind := inverse[edge.Kind]
			if groups[kind] == nil {
				groups[kind] = make(map[string]*BacklinkSource)
			}
			source := groups[kind][edge.From]
			if source == nil {
				vertex := index.Vertices[edge.From]
				source = &BacklinkSource{Vertex: vertex, Edges: []relationships.Edge{}}
				if owner, ok := index.Vertices[vertex.DocumentOwner]; ok {
					source.OwnerVertex = &owner
				}
				groups[kind][edge.From] = source
			}
			source.Edges = append(source.Edges, edge)
			seen[edge.From] = true
		}
	}
	for kind, sources := range groups {
		group := BacklinkGroup{Kind: kind, Sources: make([]BacklinkSource, 0, len(sources))}
		for _, source := range sources {
			sort.Slice(source.Edges, func(i, j int) bool { return source.Edges[i].ID < source.Edges[j].ID })
			group.Sources = append(group.Sources, *source)
		}
		sort.Slice(group.Sources, func(i, j int) bool { return group.Sources[i].Vertex.ID < group.Sources[j].Vertex.ID })
		out.Groups = append(out.Groups, group)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Kind < out.Groups[j].Kind })
	out.Count = len(seen)
	return out
}
