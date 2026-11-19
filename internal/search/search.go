// Package search is the keyword search behind `tiller search` and the
// console's Explore search (cli-surface.md, "Inspecting and maintaining";
// ui-surface.md, "Explore"). It stays plain: a case-insensitive substring
// match over every markdown file under .tiller/ plus the tree's
// human-meaningful fields, and an exact lookup by id, key, or reference. The
// CLI also reads the source files the baseline ledger records. No query
// language and no semantic search. The whole corpus is local files, so the
// honest implementation is a scan, not an index.
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
	"unicode"
	"unicode/utf8"

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
	store.NounBug, store.NounChore, store.NounTweak, store.NounBatch,
	store.NounQuestion, store.NounAmendment, KindDocument,
}

// StatusLive is the status filter that keeps every node still in play: one
// that is not done, cancelled, or archived.
const StatusLive = "live"

// Statuses is every value a status filter takes: StatusLive, then each
// stored status a node can hold, the work lifecycle first and the question
// and amendment ledgers' own after it.
var Statuses = []string{
	StatusLive,
	"draft", "specced", "proposed", "ready", "active", "in_progress", "check", "in_review",
	"done", "cancelled", "archived",
	"open", "answered", "approved", "withdrawn",
}

// inStatus reports whether a node's stored status passes the filter.
func inStatus(filter, status string) bool {
	if filter == StatusLive {
		return status != "done" && !retired(status)
	}
	return status == filter
}

// Options narrows or widens a search. Kind keeps one kind of result (""
// keeps all); Status keeps the nodes in one stored status, or the live ones
// with StatusLive, and leaves every document out, since a document has no
// status; Limit bounds the results returned, with 0 or anything above
// MaxResults meaning MaxResults. Sources adds the files the baseline ledger
// records as derived_from sources to the corpus (sources.go); `tiller search`
// asks for them and the console's search does not.
type Options struct {
	Kind    string
	Status  string
	Limit   int
	Sources bool
}

// Result is the payload `tiller search --json` and GET /api/search share.
// Count is the number of results the query matched within Kind, not the
// number returned, so Truncated can say a bounded page dropped some. Kinds
// tallies every matched result by kind regardless of Kind, so a reader
// narrowed to one kind still sees what the others hold. A Status filter
// narrows Count and Kinds alike. Every slice is an
// array, empty rather than null, so a caller parses one shape.
type Result struct {
	Query     string           `json:"query"`
	Kind      string           `json:"kind,omitempty"`
	Status    string           `json:"status,omitempty"`
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
	// Column is an epic's or a feature's board column, set only by the
	// console's search endpoint (serve.SearchHandler); the CLI's search
	// leaves it empty.
	Column string `json:"column,omitempty"`
	// Severity is a bug's triage grade; empty for unrated bugs and other nouns.
	Severity string `json:"severity,omitempty"`
	Ref      string `json:"ref"`
	Match    string `json:"match"`
	Count    int    `json:"count"`
	Hits     []Hit  `json:"hits"`

	rank rank
}

// DocumentResult is one file no node owns and its matching lines: Markdown
// under .tiller/, or a baseline source anywhere in the project.
type DocumentResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
	Hits  []Hit  `json:"hits"`
}

// Hit is one match: a line in a searched file (Line is 1-based), or
// a question's statement or recorded answer, which live only in the ledger
// yaml and carry Line 0 and the Field that matched. Snippet is the trimmed
// line, capped at snippetWindow runes, and Truncated says the cap cut it, so
// a consumer can tell a whole line from part of one.
type Hit struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Snippet   string `json:"snippet"`
	Truncated bool   `json:"truncated,omitempty"`
	Field     string `json:"field,omitempty"`

	// excerpt is set only when it differs from Snippet.
	excerpt string
}

// Excerpt is the text human output prints for the hit: the line when it fits
// excerptWindow runes, otherwise a window that holds the match, with an
// ellipsis at each end where the line was cut.
func (h Hit) Excerpt() string {
	if h.excerpt != "" {
		return h.excerpt
	}
	return h.Snippet
}

// matchHit builds the hit for a match in text. lower is text lowercased and
// at the match's byte offset in it; q is the lowercased query.
func matchHit(text, lower string, at int, q string) Hit {
	hit := Hit{Snippet: snippet(text, at, len(q))}
	trimmed := []rune(strings.TrimSpace(text))
	hit.Truncated = len(trimmed) > snippetWindow
	if len(trimmed) > excerptWindow {
		hit.excerpt = excerpt(text, utf8.RuneCountInString(lower[:at]), utf8.RuneCountInString(q))
	}
	return hit
}

// UnknownStatusError is the typed misuse a status filter outside Statuses
// maps to.
type UnknownStatusError struct{ Status string }

func (e *UnknownStatusError) Error() string {
	return fmt.Sprintf("search has no status %q; use one of %s", e.Status, strings.Join(Statuses, ", "))
}
func (*UnknownStatusError) ExitCode() int  { return 2 }
func (*UnknownStatusError) Reason() string { return "unknown_status" }

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
// under or into its file's document result. With Options.Sources the
// baseline ledger's source files follow, each a document.
func Search(root string, tree *store.Tree, query string, opts Options) (Result, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return Result{}, &EmptyQueryError{}
	}
	if opts.Kind != "" && !slices.Contains(Kinds, opts.Kind) {
		return Result{}, &UnknownKindError{Kind: opts.Kind}
	}
	if opts.Status != "" && !slices.Contains(Statuses, opts.Status) {
		return Result{}, &UnknownStatusError{Status: opts.Status}
	}
	limit := opts.Limit
	if limit <= 0 || limit > MaxResults {
		limit = MaxResults
	}

	nodes := matchNodes(tree, q)
	var docs []*DocumentResult
	docByPath := map[string]*DocumentResult{}
	// scan matches one file line by line, folding each matching line into
	// the node that owns the file, or into the file's own document result.
	scan := func(filePath string, content []byte, owner store.Node) {
		for i, line := range strings.Split(string(content), "\n") {
			lower := strings.ToLower(line)
			at := strings.Index(lower, q)
			if at < 0 {
				continue
			}
			hit := matchHit(line, lower, at, q)
			hit.Path, hit.Line = filePath, i+1
			if owner != nil {
				nodes.add(tree, owner, rankText).addHit(hit)
				continue
			}
			doc := docByPath[filePath]
			if doc == nil {
				doc = &DocumentResult{Path: filePath, Hits: []Hit{}}
				docByPath[filePath] = doc
				docs = append(docs, doc)
			}
			doc.Count++
			if len(doc.Hits) < MaxNodeHits {
				doc.Hits = append(doc.Hits, hit)
			}
		}
	}
	err := corpus.Walk(context.Background(), root, func(document corpus.Document) error {
		p := strings.TrimPrefix(document.Path, ".tiller/")
		// Orientation is generated guidance, not the project's authored prose.
		if p == "orientation.md" {
			return nil
		}
		scan(path.Join(".tiller", p), document.Content, attribute(tree, p))
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
	if opts.Sources {
		err := walkSources(root, tree.Baseline, func(sourcePath string, content []byte) {
			scan(sourcePath, content, nil)
		})
		if err != nil {
			return Result{}, &store.EnvError{
				Reason: ReasonCorpusUnreadable,
				Msg:    fmt.Sprintf("search cannot read a %v", err),
			}
		}
	}

	ranked := nodes.ranked()
	sortDocuments(docs, q)
	if opts.Status != "" {
		// The status filter narrows the answer itself, so it comes before the
		// tally: Kinds says what each kind holds in this status.
		ranked = slices.DeleteFunc(ranked, func(n NodeResult) bool { return !inStatus(opts.Status, n.Status) })
		docs = nil
	}
	res := Result{Query: query, Kind: opts.Kind, Status: opts.Status, Kinds: countKinds(ranked, len(docs))}
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

// excerptWindow caps how much of a matching line human output prints, in
// runes: enough to recognize the passage, far less than a paragraph written
// on one line. excerptLead is how much of the line an excerpt keeps before a
// match it had to window into view.
const (
	excerptWindow = 120
	excerptLead   = 40
)

// excerpt is the trimmed line when it fits excerptWindow runes, otherwise
// excerptWindow runes of it that hold the match, with an ellipsis at each end
// where text was cut. at and n are the match's offset and length in runes of
// the untrimmed line. Lowercasing maps rune to rune, so a rune offset found
// in the lowercased line is the same offset in the original, which a byte
// offset is not.
func excerpt(line string, at, n int) string {
	runes := []rune(strings.TrimLeftFunc(line, unicode.IsSpace))
	at -= utf8.RuneCountInString(line) - len(runes)
	runes = []rune(strings.TrimRightFunc(string(runes), unicode.IsSpace))
	if len(runes) <= excerptWindow {
		return string(runes)
	}
	// A match too long to sit behind the full lead takes the room it needs.
	lead := min(excerptLead, max(0, excerptWindow-n))
	start := max(0, min(at-lead, len(runes)-excerptWindow))
	end := start + excerptWindow
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}
