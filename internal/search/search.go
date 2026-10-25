// Package search is the keyword search behind `tiller search` and the
// console's Explore search (cli-surface.md, "Inspecting and maintaining";
// ui-surface.md, "Explore"). It stays plain: a case-insensitive substring
// match over every markdown file under .tiller/ plus the tree's
// human-meaningful fields, and an exact lookup by id, key, or reference. No
// query language and no semantic search. The whole corpus is local files, so
// the honest implementation is a scan, not an index.
//
// What it returns is organized rather than raw. Every match inside one node
// folds into that node's single result, and prose no node owns folds into one
// result per file. Nodes rank by how they matched (rank.go): the node a
// pasted id or key names comes first, then title matches, then key
// fragments, then nodes that only mention the query in their prose.
package search

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/store"
)

// MaxResults is the ceiling on the results one search returns, so a
// degenerate query ("e") answers with a bounded page rather than the whole
// corpus. It is also the default when a caller names no limit.
const MaxResults = 200

// MaxNodeHits is how many matching lines one result carries. Count says how
// many there were in all; the node page or the file holds the rest.
const MaxNodeHits = 3

// KindDocument is the kind of a result that is a file no node owns (a design
// doc, an ADR), beside the node nouns.
const KindDocument = "document"

// Kinds is every kind a result can have, in the order results, counts, and
// the console's sections list them: the work tree outside in, the shelves,
// then the ledgers, then loose documents.
var Kinds = []string{
	store.NounEpic, store.NounFeature, store.NounTask,
	store.NounBug, store.NounChore, store.NounTweak,
	store.NounQuestion, store.NounAmendment, KindDocument,
}

// Options narrows a search. Kind keeps one kind of result ("" keeps all);
// Limit bounds the results returned, with 0 or anything above MaxResults
// meaning MaxResults.
type Options struct {
	Kind  string
	Limit int
}

// Result is the payload `tiller search --json` and GET /api/search share.
// Count is the number of results the query matched within Kind, not the
// number returned, so Truncated can say a bounded page dropped some. Kinds
// tallies every matched result by kind regardless of Kind, so a reader
// narrowed to one kind still sees what the others hold. Every slice is an
// array, empty rather than null, so a caller parses one shape.
type Result struct {
	Query     string           `json:"query"`
	Kind      string           `json:"kind,omitempty"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated"`
	Kinds     []KindCount      `json:"kinds"`
	Nodes     []NodeResult     `json:"nodes"`
	Documents []DocumentResult `json:"documents"`
}

// KindCount is how many results of one kind the query matched.
type KindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// NodeResult is one node and everything in it that matched. Match says how
// the node matched at its strongest: "id" (the query is its id, id suffix, or
// full reference), "key" (its whole key or last key segment), "title",
// "key_fragment" (its key contains the query), or "text"
// (only its prose or ledger fields). Count is every matching line and ledger
// field in the node and Hits the first MaxNodeHits of them. A matching title
// is neither: the title is already the result's label, and Match says so.
type NodeResult struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	Match  string `json:"match"`
	Count  int    `json:"count"`
	Hits   []Hit  `json:"hits"`

	rank rank
}

// DocumentResult is one file no node owns and its matching lines.
type DocumentResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
	Hits  []Hit  `json:"hits"`
}

// Hit is one match: a line in a .tiller/ markdown file (Line is 1-based), or
// a question's statement or recorded answer, which live only in the ledger
// yaml and carry Line 0 and the Field that matched.
type Hit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Field   string `json:"field,omitempty"`
}

// ReasonCorpusUnreadable is the store.EnvError discriminator for a corpus
// file the scan could not read: a broken repo condition, exit 3.
const ReasonCorpusUnreadable = "corpus_unreadable"

// EmptyQueryError is the typed misuse an empty or all-whitespace query maps
// to: exit 2 at the CLI, HTTP 400 at the console. A query that constrains
// nothing answers nothing, so it is refused rather than matched.
type EmptyQueryError struct{}

func (*EmptyQueryError) Error() string  { return "search needs a keyword; the query was empty" }
func (*EmptyQueryError) ExitCode() int  { return 2 }
func (*EmptyQueryError) Reason() string { return "empty_query" }

// UnknownKindError is the typed misuse a kind filter outside Kinds maps to.
type UnknownKindError struct{ Kind string }

func (e *UnknownKindError) Error() string {
	return fmt.Sprintf("search has no kind %q; use one of %s", e.Kind, strings.Join(Kinds, ", "))
}
func (*UnknownKindError) ExitCode() int  { return 2 }
func (*UnknownKindError) Reason() string { return "unknown_kind" }

// Search answers query over the repo at root: node fields and identities
// first (matchNodes), then every project-authored .md file under .tiller/
// line by line, each line folded into the node attribute() places its file
// under or into its file's document result.
func Search(root string, tree *store.Tree, query string, opts Options) (Result, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return Result{}, &EmptyQueryError{}
	}
	if opts.Kind != "" && !slices.Contains(Kinds, opts.Kind) {
		return Result{}, &UnknownKindError{Kind: opts.Kind}
	}
	limit := opts.Limit
	if limit <= 0 || limit > MaxResults {
		limit = MaxResults
	}

	nodes := matchNodes(tree, q)
	var docs []*DocumentResult
	docByPath := map[string]*DocumentResult{}
	err := corpus.Walk(context.Background(), root, func(document corpus.Document) error {
		p := strings.TrimPrefix(document.Path, ".tiller/")
		// Orientation is generated guidance, not the project's authored prose.
		if p == "orientation.md" {
			return nil
		}
		owner := attribute(tree, p)
		for i, line := range strings.Split(string(document.Content), "\n") {
			at := strings.Index(strings.ToLower(line), q)
			if at < 0 {
				continue
			}
			hit := Hit{Path: path.Join(".tiller", p), Line: i + 1, Snippet: snippet(line, at, len(q))}
			if owner != nil {
				nodes.add(tree, owner, rankText).addHit(hit)
				continue
			}
			doc := docByPath[hit.Path]
			if doc == nil {
				doc = &DocumentResult{Path: hit.Path, Hits: []Hit{}}
				docByPath[hit.Path] = doc
				docs = append(docs, doc)
			}
			doc.Count++
			if len(doc.Hits) < MaxNodeHits {
				doc.Hits = append(doc.Hits, hit)
			}
		}
		return nil
	})
	if err != nil {
		// A file the walk cannot safely read is the environment's answer, not
		// misuse of the query, and a partial result would be a quiet lie.
		return Result{}, &store.EnvError{
			Reason: ReasonCorpusUnreadable,
			Msg:    fmt.Sprintf("search cannot read the .tiller/ corpus: %v", err),
		}
	}

	ranked := nodes.ranked()
	sortDocuments(docs, q)
	res := Result{Query: query, Kind: opts.Kind, Kinds: countKinds(ranked, len(docs))}
	if opts.Kind != "" {
		ranked = slices.DeleteFunc(ranked, func(n NodeResult) bool { return n.Noun != opts.Kind })
		if opts.Kind != KindDocument {
			docs = nil
		}
	}
	res.Count = len(ranked) + len(docs)

	// Nodes and documents share the page so a flood of design prose cannot
	// push every node off it, nor a broad title match every document.
	take := shareBudget(limit, []int{len(ranked), len(docs)})
	res.Nodes = ranked[:take[0]]
	res.Documents = make([]DocumentResult, 0, take[1])
	for _, d := range docs[:take[1]] {
		res.Documents = append(res.Documents, *d)
	}
	res.Truncated = take[0]+take[1] < res.Count
	return res, nil
}

// countKinds tallies results by kind in Kinds order, leaving out kinds with
// none.
func countKinds(nodes []NodeResult, documents int) []KindCount {
	by := map[string]int{KindDocument: documents}
	for _, n := range nodes {
		by[n.Noun]++
	}
	out := []KindCount{}
	for _, kind := range Kinds {
		if by[kind] > 0 {
			out = append(out, KindCount{Kind: kind, Count: by[kind]})
		}
	}
	return out
}

// sortDocuments puts a document whose path names the query first, then the
// documents that match most often, then path order.
func sortDocuments(docs []*DocumentResult, q string) {
	slices.SortStableFunc(docs, func(a, b *DocumentResult) int {
		an, bn := strings.Contains(strings.ToLower(a.Path), q), strings.Contains(strings.ToLower(b.Path), q)
		if an != bn {
			if an {
				return -1
			}
			return 1
		}
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Path, b.Path)
	})
}

// shareBudget splits budget across classes of the given sizes so no class can
// crowd another out of a capped page: every class that still wants results
// takes an equal share of what is left, a class that wants less than its
// share donates the rest, and the loop repeats until the budget is spent or
// every class is satisfied. When what is left will not divide, the odd seats
// go out one per class in declaration order.
func shareBudget(budget int, sizes []int) []int {
	take := make([]int, len(sizes))
	for {
		spent, hungry := 0, 0
		for i, size := range sizes {
			spent += take[i]
			if take[i] < size {
				hungry++
			}
		}
		left := budget - spent
		if left <= 0 || hungry == 0 {
			return take
		}
		if share := left / hungry; share > 0 {
			for i, size := range sizes {
				take[i] += min(share, size-take[i])
			}
			continue
		}
		for i, size := range sizes {
			if left == 0 {
				return take
			}
			if take[i] < size {
				take[i]++
				left--
			}
		}
		return take
	}
}

// snippetWindow caps a snippet's length in runes; the matching line is the
// context, not the document. snippetLead is how much of the line a snippet
// keeps before a match it had to window into view.
const (
	snippetWindow = 200
	snippetLead   = 60
)

// snippet trims a matching line and caps it at snippetWindow runes. A match
// that would fall past the cap is windowed into view, with an ellipsis
// marking the cut, so the snippet always shows what matched. at and n are
// the match's byte offset and length in line.
func snippet(line string, at, n int) string {
	lead := len(line) - len(strings.TrimLeft(line, " \t\r"))
	s := strings.TrimSpace(line)
	runes := []rune(s)
	if len(runes) <= snippetWindow {
		return s
	}
	end := len([]rune(s[:max(0, min(len(s), at-lead+n))]))
	if end <= snippetWindow {
		return string(runes[:snippetWindow])
	}
	// The lowercased line can differ in byte length from the original for a
	// few scripts, so the offsets are clamped rather than trusted.
	start := len([]rune(s[:max(0, min(len(s), at-lead))])) - snippetLead
	start = max(0, min(start, len(runes)-snippetWindow))
	return "…" + string(runes[start:start+snippetWindow])
}
