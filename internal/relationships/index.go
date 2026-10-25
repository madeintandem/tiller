package relationships

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
)

func New() *Index {
	return &Index{Vertices: map[string]Vertex{}, Edges: map[string]Edge{}, Forward: map[string][]string{}, Reverse: map[string][]string{}, Complete: true, sourceKeys: map[string]map[string]struct{}{}}
}

// AddVertex upgrades unresolved placeholders when their real record is read.
// A later unresolved assertion never erases known identity or navigation.
func (index *Index) AddVertex(vertex Vertex) {
	if vertex.ID == "" {
		return
	}
	if old, ok := index.Vertices[vertex.ID]; ok {
		if !old.Unresolved && vertex.Unresolved {
			return
		}
		if vertex.DocumentOwner == "" {
			vertex.DocumentOwner = old.DocumentOwner
		}
		if vertex.Raw == "" {
			vertex.Raw = old.Raw
		}
		if vertex.Path == "" {
			vertex.Path = old.Path
		}
		vertex.Generated = vertex.Generated || old.Generated
		if !old.Unresolved && !vertex.Unresolved {
			if vertex.Title == "" {
				vertex.Title = old.Title
			}
			if vertex.Key == "" {
				vertex.Key = old.Key
			}
			if vertex.Status == "" {
				vertex.Status = old.Status
			}
			if vertex.Owner == "" {
				vertex.Owner = old.Owner
			}
			if vertex.Contributors == nil {
				vertex.Contributors = old.Contributors
			}
			if vertex.Destination == nil {
				vertex.Destination = old.Destination
			}
		}
	}
	index.Vertices[vertex.ID] = vertex
}

// AddEdge never records an edge with an empty endpoint: an identity that
// failed validation is the caller's diagnostic, never a shared phantom node.
func (index *Index) AddEdge(kind, from, to string, sources ...Source) {
	if from == "" || to == "" {
		return
	}
	// The length prefix makes endpoint punctuation unambiguous without hashes.
	id := kind + ":" + strconv.Itoa(len(from)) + ":" + from + ":" + to
	edge, exists := index.Edges[id]
	if !exists {
		edge = Edge{ID: id, Kind: kind, From: from, To: to, Sources: []Source{}, Derived: kind == "at_path"}
		index.Forward[from] = append(index.Forward[from], id)
		index.Reverse[to] = append(index.Reverse[to], id)
	}
	seen := index.sourceKeys[id]
	if seen == nil {
		seen = map[string]struct{}{}
		index.sourceKeys[id] = seen
	}
	for _, source := range sources {
		key := stableJSON(source)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		source.Evidence = append([]string(nil), source.Evidence...)
		if source.Revision != nil {
			source.Revision = new(*source.Revision)
		}
		edge.Sources = append(edge.Sources, source)
	}
	index.Edges[id] = edge
}

func (index *Index) AddDiagnostic(diagnostic Diagnostic, incomplete bool) {
	diagnostic.Subjects = append([]string(nil), diagnostic.Subjects...)
	sort.Strings(diagnostic.Subjects)
	diagnostic.Subjects = slices.Compact(diagnostic.Subjects)
	if len(diagnostic.Subjects) > 0 && diagnostic.Subjects[0] == "" {
		diagnostic.Subjects = diagnostic.Subjects[1:]
	}
	diagnostic.Incomplete = diagnostic.Incomplete || incomplete
	index.Diagnostics = append(index.Diagnostics, diagnostic)
	if diagnostic.Incomplete {
		index.Complete = false
	}
}

// DiagnosticsFor scopes ordinary warnings while keeping every explanation for
// global extraction loss: unseen assertions may originate outside this scope.
func (index *Index) DiagnosticsFor(subjects ...string) []Diagnostic {
	wanted := make(map[string]bool, len(subjects))
	for _, subject := range subjects {
		wanted[subject] = true
	}
	out := []Diagnostic{}
	for _, diagnostic := range index.Diagnostics {
		if diagnostic.Incomplete || len(diagnostic.Subjects) == 0 {
			out = append(out, diagnostic)
			continue
		}
		for _, subject := range diagnostic.Subjects {
			if wanted[subject] {
				out = append(out, diagnostic)
				break
			}
		}
	}
	return out
}

// Finalize is repeatable; enrichers can add facts after a structural snapshot.
func (index *Index) Finalize() {
	for id, edge := range index.Edges {
		if len(edge.Sources) < 2 {
			continue
		}
		type location struct {
			key    string
			source Source
		}
		locations := make([]location, len(edge.Sources))
		for n, source := range edge.Sources {
			locations[n] = location{stableJSON(source), source}
		}
		sort.Slice(locations, func(a, b int) bool { return locations[a].key < locations[b].key })
		for n, location := range locations {
			edge.Sources[n] = location.source
		}
		index.Edges[id] = edge
	}
	for _, ids := range index.Forward {
		sort.Strings(ids)
	}
	for _, ids := range index.Reverse {
		sort.Strings(ids)
	}
	if len(index.Diagnostics) < 2 {
		return
	}
	type diagnosticEntry struct {
		key        string
		diagnostic Diagnostic
	}
	entries := make([]diagnosticEntry, len(index.Diagnostics))
	for n, diagnostic := range index.Diagnostics {
		identity := diagnostic
		identity.Incomplete = false // Extraction impact merges; it does not distinguish assertions.
		entries[n] = diagnosticEntry{stableJSON(identity), diagnostic}
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].key < entries[b].key })
	out := index.Diagnostics[:0]
	for n, entry := range entries {
		if n == 0 || entry.key != entries[n-1].key {
			out = append(out, entry.diagnostic)
		} else if entry.diagnostic.Incomplete {
			out[len(out)-1].Incomplete = true
		}
	}
	index.Diagnostics = out
}

func stableJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	} // Closed DTOs contain only JSON-safe fields.
	return string(encoded)
}

// Adjacent always returns canonical edges, including for an inverse direction.
func (index *Index) Adjacent(id, direction string) []Edge {
	kind, reverse := relationDirection(direction)
	if kind == "" {
		return nil
	}
	ids := index.Forward[id]
	if reverse {
		ids = index.Reverse[id]
	}
	var out []Edge
	for _, edgeID := range ids {
		if edge := index.Edges[edgeID]; edge.Kind == kind {
			out = append(out, edge)
		}
	}
	return out
}

func (index *Index) Resolve(address string) (Vertex, error) {
	if strings.HasPrefix(address, "path:") {
		id, err := PathID(strings.TrimPrefix(address, "path:"))
		if err != nil {
			return Vertex{}, err
		}
		if vertex, exists := index.Vertices[id]; exists {
			return vertex, nil
		}
		return Vertex{ID: id, Kind: "path", Path: strings.TrimPrefix(id, "path:")}, nil
	}
	if strings.HasPrefix(address, "doc:") {
		id, err := PathID(strings.TrimPrefix(address, "doc:"))
		if err != nil {
			return Vertex{}, err
		}
		address = "doc:" + strings.TrimPrefix(id, "path:")
	}
	if vertex, exists := index.Vertices[address]; exists {
		return vertex, nil
	}
	var candidates []store.Node
	var found Vertex
	for _, vertex := range index.Vertices {
		if vertex.Unresolved || vertex.Key == "" || vertex.Key != address {
			continue
		}
		found = vertex
		candidates = append(candidates, addressCandidate{vertex})
	}
	if len(candidates) == 1 {
		return found, nil
	}
	if len(candidates) > 1 {
		sort.Slice(candidates, func(a, b int) bool { return candidates[a].NodeID() < candidates[b].NodeID() })
		return Vertex{}, &resolve.ErrAmbiguousKey{Key: address, Candidates: candidates}
	}
	return Vertex{}, &resolve.ErrNotFound{Raw: address}
}

type addressCandidate struct{ vertex Vertex }

func (candidate addressCandidate) NodeID() string     { return candidate.vertex.ID }
func (candidate addressCandidate) NodeKey() *string   { return new(candidate.vertex.Key) }
func (candidate addressCandidate) Noun() string       { return candidate.vertex.Kind }
func (candidate addressCandidate) NodeStatus() string { return candidate.vertex.Status }

// PathID normalizes a literal repository coordinate without inspecting the
// filesystem. A literal coordinate is a filename, not citation or URL syntax:
// filesystem-valid punctuation such as '#', '?', or ':' inside a segment is
// part of the name, so notes#1.md and notes?1.md stay distinct. Citation
// parsing (localCitation) and URL encoding happen elsewhere, once. Empty,
// absolute (including a drive-letter root), URL-shaped ("scheme://"),
// control-character, and root-escaping coordinates have no identity.
func PathID(raw string) (string, error) {
	coordinate := strings.ReplaceAll(raw, "\\", "/")
	if coordinate == "" || strings.HasPrefix(coordinate, "/") || driveRoot.MatchString(coordinate) || urlScheme.MatchString(coordinate) || strings.IndexFunc(coordinate, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("invalid repository path %q", raw)
	}
	depth := 0
	for _, part := range strings.Split(coordinate, "/") {
		switch part {
		case "", ".":
		case "..":
			depth--
		default:
			depth++
		}
		if depth < 0 {
			return "", fmt.Errorf("repository path escapes root: %q", raw)
		}
	}
	coordinate = path.Clean(coordinate)
	return "path:" + coordinate, nil
}

var (
	// driveRoot is a Windows drive root (C:/ or a bare C:), absolute even
	// after backslashes become slashes.
	driveRoot = regexp.MustCompile(`^[A-Za-z]:(/|$)`)
	// urlScheme is an external reference such as https://host/path, never a
	// repository coordinate even though its characters are filesystem-legal.
	urlScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
)

// DocumentID accepts the same confined literal coordinates as PathID. Invalid
// input has no identity: it returns "", which no vertex or edge accepts.
func DocumentID(raw string) string {
	id, err := PathID(raw)
	if err != nil {
		return ""
	}
	return "doc:" + strings.TrimPrefix(id, "path:")
}

// NodePath names the physical file implied by loaded layout and the local key.
// It does not assert existence; question ledger entries do not own Markdown.
func NodePath(tree *store.Tree, node store.Node, extension string) (string, bool) {
	if extension != "md" && extension != "yml" && extension != "eval.md" {
		return "", false
	}
	if node.Noun() == store.NounQuestion {
		if extension == "yml" {
			return ".tiller/decisions/open-questions.yml", true
		}
		return "", false
	}
	if node.NodeKey() == nil || *node.NodeKey() == "" {
		return "", false
	}
	key := *node.NodeKey()
	parts := strings.Split(key, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\:") {
			return "", false
		}
	}
	var coordinate string
	switch node.Noun() {
	case store.NounEpic:
		if len(parts) != 1 || extension == "eval.md" {
			return "", false
		}
		coordinate = path.Join(".tiller/epics", key, "epic."+extension)
	case store.NounFeature:
		feature, ok := node.(*store.Feature)
		if !ok {
			return "", false
		}
		epicKey := feature.EpicKey
		if epicKey == "" && len(parts) == 2 {
			epicKey = parts[0]
		}
		if !localSegment(epicKey) {
			return "", false
		}
		coordinate = path.Join(".tiller/epics", epicKey, "features", parts[len(parts)-1], "feature."+extension)
	case store.NounTask:
		if extension == "eval.md" {
			return "", false
		}
		task, ok := node.(*store.Task)
		if !ok {
			return "", false
		}
		if tree != nil {
			parent := tree.Features[task.FeatureID]
			if parent == nil {
				return "", false
			}
			parentPath, ok := NodePath(tree, parent, "yml")
			if !ok {
				return "", false
			}
			coordinate = path.Join(path.Dir(parentPath), store.TaskDirName, parts[len(parts)-1], "task."+extension)
		} else {
			if len(parts) != 3 {
				return "", false
			}
			coordinate = path.Join(".tiller/epics", parts[0], "features", parts[1], store.TaskDirName, parts[2], "task."+extension)
		}
	case store.NounBug, store.NounChore, store.NounTweak:
		if extension == "eval.md" {
			return "", false
		}
		coordinate = path.Join(".tiller", store.ShelfDir(node.Noun()), parts[len(parts)-1], node.Noun()+"."+extension)
	case store.NounAmendment:
		if len(parts) != 1 || extension == "eval.md" {
			return "", false
		}
		coordinate = path.Join(".tiller/amendments", node.NodeID()+"-"+key+"."+extension)
	default:
		return "", false
	}
	id, err := PathID(coordinate)
	return strings.TrimPrefix(id, "path:"), err == nil
}

func localSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".." && !strings.ContainsAny(segment, "/\\:")
}
