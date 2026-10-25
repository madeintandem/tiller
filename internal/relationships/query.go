package relationships

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// QueryInput is transport-independent. Follow is a union at every hop; Where
// selects results only after traversal, never the intermediate bridge vertices.
type QueryInput struct {
	From   []string
	Follow []string
	Depth  int
	Where  []QueryFilter
}

type QueryFilter struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type QueryVertex struct {
	Vertex
	Depth int  `json:"depth"`
	Seed  bool `json:"seed"`
}

type QueryResult struct {
	Starts      []string      `json:"starts"`
	Follow      []string      `json:"follow"`
	Depth       int           `json:"depth"`
	Where       []QueryFilter `json:"where"`
	Vertices    []QueryVertex `json:"vertices"`
	Matched     []string      `json:"matched"`
	Edges       []Edge        `json:"edges"`
	Diagnostics []Diagnostic  `json:"diagnostics"`
	Complete    bool          `json:"complete"`
}

// QueryError identifies syntax misuse on both CLI and HTTP surfaces.
type QueryError struct {
	Code    string
	Message string
}

func (err *QueryError) Error() string  { return err.Message }
func (err *QueryError) Reason() string { return err.Code }
func (err *QueryError) ExitCode() int  { return 2 }

func queryError(code, message string) error {
	return &QueryError{Code: code, Message: message}
}

// ParseQuery accepts the raw repeated values supplied by either transport.
// From and Where repeat; Follow and Depth each accept one value. A single
// comma-separated Follow value expresses the direction union.
func ParseQuery(from, follow, depth, where []string) (QueryInput, error) {
	input := QueryInput{From: from, Depth: 1, Where: []QueryFilter{}}
	if len(follow) > 1 {
		return QueryInput{}, queryError("query_follow_invalid", "follow accepts one comma-separated direction list")
	}
	if len(follow) == 1 {
		input.Follow = strings.Split(follow[0], ",")
	}
	if len(depth) > 1 {
		return QueryInput{}, queryError("query_depth_invalid", "depth accepts one nonnegative integer")
	}
	if len(depth) == 1 {
		raw := depth[0]
		if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return QueryInput{}, queryError("query_depth_invalid", "depth must be a nonnegative integer")
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return QueryInput{}, queryError("query_depth_invalid", "depth must be a representable nonnegative integer")
		}
		input.Depth = value
	}
	for _, clause := range where {
		field, value, ok := strings.Cut(clause, "=")
		if !ok || strings.Contains(value, "=") {
			return QueryInput{}, queryError("query_where_invalid", "where must have the form kind=value, status=value, owner=value, or contributor=value")
		}
		input.Where = append(input.Where, QueryFilter{Field: field, Value: value})
	}
	return normalizeQuery(input)
}

func normalizeQuery(input QueryInput) (QueryInput, error) {
	if len(input.From) == 0 {
		return QueryInput{}, queryError("query_from_required", "at least one from address is required")
	}
	from := make([]string, 0, len(input.From))
	for _, raw := range input.From {
		address := strings.TrimSpace(raw)
		if address == "" {
			return QueryInput{}, queryError("query_from_invalid", "from addresses must not be empty")
		}
		for _, prefix := range []string{"path:", "doc:"} {
			if strings.HasPrefix(address, prefix) {
				id, err := PathID(strings.TrimPrefix(address, prefix))
				if err != nil {
					return QueryInput{}, queryError("query_from_invalid", err.Error())
				}
				address = prefix + strings.TrimPrefix(id, "path:")
				break
			}
		}
		from = append(from, address)
	}
	if len(input.Follow) == 0 {
		return QueryInput{}, queryError("query_follow_required", "follow requires at least one relationship direction")
	}
	follow := make([]string, 0, len(input.Follow))
	seenDirections := map[string]bool{}
	for _, raw := range input.Follow {
		direction := strings.TrimSpace(raw)
		if kind, _ := relationDirection(direction); kind == "" {
			return QueryInput{}, queryError("query_follow_invalid", fmt.Sprintf("unknown relationship direction %q", direction))
		}
		if !seenDirections[direction] {
			follow = append(follow, direction)
			seenDirections[direction] = true
		}
	}
	sort.Strings(follow)
	if input.Depth < 0 {
		return QueryInput{}, queryError("query_depth_invalid", "depth must be a nonnegative integer")
	}
	filters := make([]QueryFilter, 0, len(input.Where))
	for _, filter := range input.Where {
		if (filter.Field != "kind" && filter.Field != "status" && filter.Field != "owner" && filter.Field != "contributor") || filter.Value == "" || strings.Contains(filter.Value, "=") {
			return QueryInput{}, queryError("query_where_invalid", "where requires a nonempty exact value for kind, status, owner, or contributor")
		}
		filters = append(filters, filter)
	}
	sort.Slice(filters, func(a, b int) bool {
		if filters[a].Field != filters[b].Field {
			return filters[a].Field < filters[b].Field
		}
		return filters[a].Value < filters[b].Value
	})
	return QueryInput{From: from, Follow: follow, Depth: input.Depth, Where: filters}, nil
}

// Query traverses a finalized, immutable index. The queue contains each identity
// once, so cycles and arbitrarily large depth cannot multiply possible walks.
// A missing local path can be a virtual seed without modifying the index.
func Query(ctx context.Context, index *Index, input QueryInput) (QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return QueryResult{}, err
	}
	input, err := normalizeQuery(input)
	if err != nil {
		return QueryResult{}, err
	}
	result := QueryResult{
		Starts: []string{}, Follow: input.Follow, Depth: input.Depth, Where: input.Where,
		Vertices: []QueryVertex{}, Matched: []string{}, Edges: []Edge{},
		Complete: index.Complete,
	}
	seen := make(map[string]bool)
	for _, address := range input.From {
		if err := ctx.Err(); err != nil {
			return QueryResult{}, err
		}
		vertex, err := index.Resolve(address)
		if err != nil {
			return QueryResult{}, err
		}
		if !seen[vertex.ID] {
			seen[vertex.ID] = true
			result.Starts = append(result.Starts, vertex.ID)
			result.Vertices = append(result.Vertices, QueryVertex{Vertex: vertex, Seed: true})
		}
	}
	sort.Strings(result.Starts)
	forward, reverse := map[string]bool{}, map[string]bool{}
	for _, direction := range input.Follow {
		kind, inverse := relationDirection(direction)
		if inverse {
			reverse[kind] = true
		} else {
			forward[kind] = true
		}
	}
	followed := make(map[string]bool)
	visit := func(edge Edge, next string, depth int) {
		if !followed[edge.ID] {
			followed[edge.ID] = true
			result.Edges = append(result.Edges, edge)
		}
		if !seen[next] {
			seen[next] = true
			result.Vertices = append(result.Vertices, QueryVertex{Vertex: index.Vertices[next], Depth: depth})
		}
	}
	// The queue grows while traversing; a slice range would freeze its length.
	for cursor := 0; cursor < len(result.Vertices); cursor++ {
		if err := ctx.Err(); err != nil {
			return QueryResult{}, err
		}
		current := result.Vertices[cursor]
		if current.Depth >= input.Depth {
			continue
		}
		for _, id := range index.Forward[current.ID] {
			if err := ctx.Err(); err != nil {
				return QueryResult{}, err
			}
			edge := index.Edges[id]
			if forward[edge.Kind] {
				visit(edge, edge.To, current.Depth+1)
			}
		}
		for _, id := range index.Reverse[current.ID] {
			if err := ctx.Err(); err != nil {
				return QueryResult{}, err
			}
			edge := index.Edges[id]
			if reverse[edge.Kind] {
				visit(edge, edge.From, current.Depth+1)
			}
		}
		if reverse["covers"] && current.Kind == "path" && current.Path != "." {
			// Visit only segment ancestors, not the filesystem or all path vertices.
			for scope := path.Dir(current.Path); ; scope = path.Dir(scope) {
				if err := ctx.Err(); err != nil {
					return QueryResult{}, err
				}
				for _, id := range index.Reverse["path:"+scope] {
					if err := ctx.Err(); err != nil {
						return QueryResult{}, err
					}
					edge := index.Edges[id]
					if edge.Kind != "covers" {
						continue
					}
					edge.ID = "derived:" + strconv.Itoa(len(edge.ID)) + ":" + edge.ID + ":" + current.ID
					edge.To, edge.Derived, edge.DeclaredScope = current.ID, true, scope
					visit(edge, edge.From, current.Depth+1)
				}
				if scope == "." || scope == "/" {
					break
				}
			}
		}
	}
	sort.Slice(result.Vertices, func(a, b int) bool {
		if result.Vertices[a].Depth != result.Vertices[b].Depth {
			return result.Vertices[a].Depth < result.Vertices[b].Depth
		}
		return result.Vertices[a].ID < result.Vertices[b].ID
	})
	sort.Slice(result.Edges, func(a, b int) bool { return result.Edges[a].ID < result.Edges[b].ID })
	subjects := make([]string, 0, len(result.Vertices))
	for _, vertex := range result.Vertices {
		if err := ctx.Err(); err != nil {
			return QueryResult{}, err
		}
		subjects = append(subjects, vertex.ID)
		if matchesQuery(vertex.Vertex, input.Where) {
			result.Matched = append(result.Matched, vertex.ID)
		}
	}
	result.Diagnostics = index.DiagnosticsFor(subjects...)
	return result, nil
}

func matchesQuery(vertex Vertex, filters []QueryFilter) bool {
	for _, filter := range filters {
		var value string
		switch filter.Field {
		case "kind":
			value = vertex.Kind
		case "status":
			value = vertex.Status
		case "owner":
			value = vertex.Owner
		case "contributor":
			for _, contributor := range vertex.Contributors {
				if contributor.ID == filter.Value {
					value = contributor.ID
					break
				}
			}
		}
		if value == "" || value != filter.Value {
			return false
		}
	}
	return true
}
