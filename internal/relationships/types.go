// Package relationships extracts disposable, request-local facts from Tiller records.
// It performs no filesystem I/O and does not decide lifecycle eligibility.
package relationships

import (
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

type Vertex struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	Key           string              `json:"key,omitempty"`
	Title         string              `json:"title,omitempty"`
	Status        string              `json:"status,omitempty"`
	Owner         string              `json:"owner,omitempty"`
	Contributors  []store.Contributor `json:"contributors,omitzero"`
	Path          string              `json:"path,omitempty"`
	Raw           string              `json:"raw,omitempty"`
	DocumentOwner string              `json:"document_owner,omitempty"`
	Generated     bool                `json:"generated,omitempty"`
	Unresolved    bool                `json:"unresolved,omitempty"`
	Destination   *Destination        `json:"destination,omitempty"`
}

type Destination struct {
	Kind string `json:"kind"` // node or file; coordinates deliberately have no destination
	Noun string `json:"noun,omitempty"`
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Revision is relationship evidence, not a link to historical file contents.
type Revision struct {
	Kind       string    `json:"kind,omitempty"`
	Value      string    `json:"value,omitempty"`
	Path       string    `json:"path,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitzero"`
}

func RevisionOf(reference store.RevisionRef) *Revision {
	return &Revision{Kind: reference.Kind, Value: reference.Value, Path: reference.Path, ObservedAt: reference.ObservedAt}
}

type Source struct {
	File           string    `json:"file"`
	CitationLine   int       `json:"citation_line,omitempty"`
	CitationPath   string    `json:"citation_path,omitempty"`
	Field          string    `json:"field,omitempty"`
	Line           int       `json:"line,omitempty"`
	Criterion      string    `json:"criterion,omitempty"`
	Basis          string    `json:"basis,omitempty"`
	BindingType    string    `json:"binding_type,omitempty"`
	Classification string    `json:"classification,omitempty"`
	Raw            string    `json:"raw,omitempty"`
	Revision       *Revision `json:"revision,omitempty"`
	Operation      int       `json:"operation,omitempty"`
	Role           string    `json:"role,omitempty"`
	State          string    `json:"state,omitempty"`
	Scope          string    `json:"scope,omitempty"`
	Evidence       []string  `json:"evidence,omitempty"`
}

type Edge struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Sources       []Source `json:"sources"`
	Derived       bool     `json:"derived,omitempty"`
	DeclaredScope string   `json:"declared_scope,omitempty"`
}

type Diagnostic struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Facet      string   `json:"facet"`
	Source     *Source  `json:"source,omitempty"`
	Subjects   []string `json:"subjects,omitempty"`
	Incomplete bool     `json:"incomplete,omitempty"`
}

type RelationKind struct {
	Name      string   `json:"name"`
	Inverse   string   `json:"inverse"`
	FromKinds []string `json:"from_kinds"`
	ToKinds   []string `json:"to_kinds"`
}

// Index is built privately for one read, enriched, then shared as immutable input.
// Forward and Reverse hold IDs into Edges, never copies of assertions.
type Index struct {
	Vertices    map[string]Vertex
	Edges       map[string]Edge
	Forward     map[string][]string
	Reverse     map[string][]string
	Diagnostics []Diagnostic
	Complete    bool
	Amendments  *AmendmentIndex
	sourceKeys  map[string]map[string]struct{}
}
