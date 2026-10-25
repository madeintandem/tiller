package readinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// RelationshipOptions reuses safely gathered observations from this request.
// ContractMissing=true wins over companion errors and parsed values; otherwise
// errors win over contracts. Map membership distinguishes supplied observations.
// EventsLoaded distinguishes an observed empty history from one not read.
type RelationshipOptions struct {
	Events          []events.Event
	EventsLoaded    bool
	HistoryError    string
	Contracts       map[string]*store.EvalFile
	ContractErrors  map[string]string
	ContractMissing map[string]bool
	// AllowIncompleteCorpus is for supplemental node/map enrichment only.
	// Queries require a complete safe read and leave this false.
	AllowIncompleteCorpus bool
}

// Relationships gathers missing supplemental inputs, then publishes one
// finalized request-local index. An optional degraded read discards a failed
// corpus wholesale, never consumes unsafe bytes, and still checks generation.
func Relationships(ctx context.Context, t *store.Tree, options RelationshipOptions) (*relationships.Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := relationships.BuildStructural(t)
	documents := []relationships.Document{}
	byPath := map[string][]byte{}
	err := corpus.Walk(ctx, t.Root, func(document corpus.Document) error {
		documents = append(documents, relationships.Document{Path: document.Path, Content: document.Content})
		byPath[document.Path] = document.Content
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !options.AllowIncompleteCorpus {
			return nil, &store.EnvError{Reason: "corpus_unreadable", Msg: fmt.Sprintf("relationships cannot read the .tiller/ corpus: %v", err)}
		}
		documents = nil
		clear(byPath)
		index.AddDiagnostic(relationships.Diagnostic{
			Code: "corpus_unreadable", Facet: "documents",
			Message: fmt.Sprintf("Markdown corpus could not be read; document ownership, mentions, citations, and acceptance contracts not already observed are incomplete: %v", err),
		}, true)
	}
	corpusAvailable := err == nil
	relationships.AddDocuments(t, index, documents)
	contracts := make(map[string]*store.EvalFile, len(t.Features))
	failures := make(map[string]string)
	for id, feature := range t.Features {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if options.ContractMissing[id] {
			continue
		}
		if failure, supplied := options.ContractErrors[id]; supplied {
			failures[id] = failure
			continue
		}
		if contract, supplied := options.Contracts[id]; supplied {
			contracts[id] = contract
			continue
		}
		if !corpusAvailable {
			continue
		}
		file, ok := relationships.NodePath(t, feature, "eval.md")
		if !ok {
			failures[id] = "contract path unavailable"
			continue
		}
		content, exists := byPath[file]
		if !exists {
			continue
		}
		contract, failure := corpusContracts.parse(content)
		if failure != "" {
			failures[id] = failure
		} else {
			contracts[id] = contract
		}
	}
	relationships.AddContracts(t, index, contracts, failures)
	if err := resolveCitedDocuments(ctx, t, index, byPath, corpusAvailable, options.AllowIncompleteCorpus); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &store.EnvError{Reason: "corpus_unreadable", Msg: fmt.Sprintf("relationships cannot read cited prose: %v", err)}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	history, historyError := options.Events, options.HistoryError
	if !options.EventsLoaded && historyError == "" {
		var err error
		history, err = events.List(t)
		if err != nil {
			historyError = err.Error()
		}
	}
	relationships.AddAmendments(t, index, history, historyError)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := t.CheckStable(); err != nil {
		return nil, err
	}
	index.Finalize()
	return index, nil
}

// Only explicit local citations can extend document destinations beyond
// .tiller. Their content is not scanned for mentions or recursively followed.
// A hidden coordinate (corpus.Servable) is never read or granted a destination:
// the console would refuse to serve it, so the index must not advertise it.
func resolveCitedDocuments(ctx context.Context, t *store.Tree, index *relationships.Index, content map[string][]byte, corpusAvailable, tolerate bool) error {
	ids := []string{}
	for id, vertex := range index.Vertices {
		if vertex.Kind == "document" && vertex.Unresolved {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	unavailable := make(map[string]bool)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		vertex := index.Vertices[id]
		if !corpus.Servable(vertex.Path) {
			unavailable[vertex.Path] = true
			index.AddDiagnostic(relationships.Diagnostic{
				Code: "unresolved_document", Facet: "documents",
				Message: fmt.Sprintf("recorded document %s is a hidden path the console does not serve", vertex.Path),
				Source:  &relationships.Source{File: vertex.Path}, Subjects: documentSubjects(index, id),
			}, false)
			continue
		}
		if !corpusAvailable && strings.HasPrefix(vertex.Path, ".tiller/") {
			unavailable[vertex.Path] = true
			continue
		}
		if !strings.HasPrefix(vertex.Path, ".tiller/") {
			document, err := corpus.Read(ctx, t.Root, vertex.Path)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !tolerate {
					return err
				}
				unavailable[vertex.Path] = true
				index.AddDiagnostic(relationships.Diagnostic{
					Code: "document_unreadable", Facet: "documents",
					Message: fmt.Sprintf("recorded document %s could not be read: %v", vertex.Path, err),
					Source:  &relationships.Source{File: vertex.Path}, Subjects: documentSubjects(index, id),
				}, true)
				continue
			}
			if err == nil {
				content[document.Path] = document.Content
				vertex.Unresolved = false
				vertex.Destination = &relationships.Destination{Kind: "file", Path: document.Path, Line: 1}
				index.AddVertex(vertex)
				coordinate, err := relationships.PathID(document.Path)
				if err != nil {
					return err
				}
				index.AddVertex(relationships.Vertex{ID: coordinate, Kind: "path", Path: document.Path})
				index.AddEdge("at_path", id, coordinate, relationships.Source{File: document.Path, Basis: "document_coordinate"})
				continue
			}
		}
		index.AddDiagnostic(relationships.Diagnostic{Code: "unresolved_document", Message: fmt.Sprintf("recorded document %s is missing", vertex.Path), Facet: "documents", Source: &relationships.Source{File: vertex.Path}, Subjects: documentSubjects(index, id)}, false)
	}
	for id, edge := range index.Edges {
		for n, source := range edge.Sources {
			if source.CitationPath == "" {
				continue
			}
			data, exists := content[source.CitationPath]
			if !exists || source.CitationLine > bytes.Count(data, []byte{'\n'})+1 {
				if !unavailable[source.CitationPath] {
					index.AddDiagnostic(relationships.Diagnostic{Code: "unresolved_citation", Message: fmt.Sprintf("citation %q does not name an available document line", source.Raw), Facet: "provenance", Source: &source, Subjects: []string{edge.From, edge.To, relationships.DocumentID(source.CitationPath)}}, false)
				}
				// Preserve the authored locator but do not advertise a broken destination.
				edge.Sources[n].CitationPath = ""
			}
		}
		index.Edges[id] = edge
	}
	return nil
}

func documentSubjects(index *relationships.Index, id string) []string {
	subjects := []string{id}
	for _, edgeID := range index.Reverse[id] {
		subjects = append(subjects, index.Edges[edgeID].From)
	}
	return subjects
}
