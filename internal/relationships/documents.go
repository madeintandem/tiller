package relationships

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/yuin/goldmark"
)

// Document is safely read Markdown. Paths are repository-relative coordinates.
type Document struct {
	Path    string
	Content []byte
}

// AddDocuments enriches the same index with document identities and literal
// citations. Ownership never replaces the document endpoint of a mention.
func AddDocuments(t *store.Tree, idx *Index, documents []Document) {
	type ownership struct {
		prefix string
		id     string
		exact  bool
	}
	owners := []ownership{}
	for _, node := range documentNodes(t) {
		file, ok := NodePath(t, node, "md")
		if !ok || node.Noun() == store.NounQuestion {
			continue
		}
		owner := ownership{prefix: path.Dir(file) + "/", id: node.NodeID()}
		if node.Noun() == store.NounAmendment {
			owner.prefix = file
			owner.exact = true
		}
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		if len(owners[i].prefix) != len(owners[j].prefix) {
			return len(owners[i].prefix) > len(owners[j].prefix)
		}
		return owners[i].id < owners[j].id
	})
	var markdown goldmark.Markdown
	for _, document := range documents {
		id := DocumentID(document.Path)
		if id == "" {
			idx.AddDiagnostic(invalidDocumentPath(document.Path), false)
			continue
		}
		vertex := Vertex{ID: id, Kind: "document", Path: document.Path, Generated: document.Path == ".tiller/orientation.md", Destination: &Destination{Kind: "file", Path: document.Path, Line: 1}}
		for _, owner := range owners {
			if (owner.exact && owner.prefix == document.Path) || (!owner.exact && strings.HasPrefix(document.Path, owner.prefix)) {
				vertex.DocumentOwner = owner.id
				break
			}
		}
		idx.AddVertex(vertex)
		coordinate, err := PathID(document.Path)
		if err == nil {
			idx.AddVertex(Vertex{ID: coordinate, Kind: "path", Path: strings.TrimPrefix(coordinate, "path:")})
			idx.AddEdge("at_path", id, coordinate, Source{File: document.Path, Basis: "document_coordinate"})
		}
		if vertex.DocumentOwner != "" {
			idx.AddEdge("has_document", vertex.DocumentOwner, id, Source{File: document.Path, Field: "document"})
		}
		addMentions(idx, document, &markdown)
	}
}

func documentNodes(t *store.Tree) []store.Node {
	nodes := make([]store.Node, 0, len(t.Epics)+len(t.Features)+len(t.Tasks)+len(t.Bugs)+len(t.Chores)+len(t.Tweaks)+len(t.Questions)+len(t.Amendments))
	for _, n := range t.Epics {
		nodes = append(nodes, n)
	}
	for _, n := range t.Features {
		nodes = append(nodes, n)
	}
	for _, n := range t.Tasks {
		nodes = append(nodes, n)
	}
	for _, n := range t.Bugs {
		nodes = append(nodes, n)
	}
	for _, n := range t.Chores {
		nodes = append(nodes, n)
	}
	for _, n := range t.Tweaks {
		nodes = append(nodes, n)
	}
	for _, n := range t.Questions {
		nodes = append(nodes, n)
	}
	for _, n := range t.Amendments {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID() < nodes[j].NodeID() })
	return nodes
}

// documentReference names a recorded document by its literal coordinate. A
// coordinate with no identity is diagnosed and returns "", which AddEdge
// refuses, so the referencing assertion never gains a phantom endpoint.
func documentReference(idx *Index, file string) string {
	id := DocumentID(file)
	if id == "" {
		idx.AddDiagnostic(invalidDocumentPath(file), false)
		return ""
	}
	if _, exists := idx.Vertices[id]; !exists {
		idx.AddVertex(Vertex{ID: id, Kind: "document", Path: file, Raw: file, Unresolved: true})
	}
	return id
}

func invalidDocumentPath(file string) Diagnostic {
	return Diagnostic{Code: "invalid_path", Message: fmt.Sprintf("document coordinate %q is not a confined repository path", file), Facet: "documents", Source: &Source{File: file}}
}
