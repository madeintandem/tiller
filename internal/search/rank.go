package search

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// rank orders node results by how they matched, strongest first. A pasted id
// or key is a lookup, so the node it names leads; a title is what a node is
// called, so a title match outranks a key fragment, and both outrank a node
// that only mentions the query in its prose.
type rank int

const (
	rankID rank = iota
	rankKey
	rankTitleExact
	rankTitlePrefix
	rankTitle
	rankKeyFragment
	rankText
)

// label is the public Match word for a rank.
func (r rank) label() string {
	switch r {
	case rankID:
		return "id"
	case rankKey:
		return "key"
	case rankKeyFragment:
		return "key_fragment"
	case rankTitleExact, rankTitlePrefix, rankTitle:
		return "title"
	}
	return "text"
}

// nodeSet collects one result per matched node, keyed by id.
type nodeSet map[string]*NodeResult

// add returns the node's result, creating it at rank r or raising an
// existing one to r when r is stronger.
func (s nodeSet) add(tree *store.Tree, n store.Node, r rank) *NodeResult {
	res := s[n.NodeID()]
	if res == nil {
		res = &NodeResult{
			Noun: n.Noun(), ID: n.NodeID(), Key: keyOf(n), Title: titleOf(n),
			Status: n.NodeStatus(), Ref: gen.Ref(tree, n), Hits: []Hit{}, rank: r,
		}
		s[n.NodeID()] = res
	}
	res.rank = min(res.rank, r)
	return res
}

// addHit counts one match inside the node and keeps it if there is room.
func (r *NodeResult) addHit(h Hit) {
	r.Count++
	if len(r.Hits) < MaxNodeHits {
		r.Hits = append(r.Hits, h)
	}
}

// ranked is the set in result order: rank, then the node that matches most
// often, then kind order, then reference.
func (s nodeSet) ranked() []NodeResult {
	out := make([]NodeResult, 0, len(s))
	for _, n := range s {
		n.Match = n.rank.label()
		out = append(out, *n)
	}
	slices.SortFunc(out, func(a, b NodeResult) int {
		return cmp.Or(
			cmp.Compare(a.rank, b.rank),
			cmp.Compare(b.Count, a.Count),
			cmp.Compare(slices.Index(Kinds, a.Noun), slices.Index(Kinds, b.Noun)),
			strings.Compare(a.Ref, b.Ref),
			strings.Compare(a.ID, b.ID),
		)
	})
	return out
}

// matchNodes checks every node's identity and human-meaningful fields
// against the lowercased query q: its id and reference, its key, its title,
// and a question's statement and recorded answer, which live only in the
// ledger yaml where no .md walk sees them.
func matchNodes(tree *store.Tree, q string) nodeSet {
	set := nodeSet{}
	for _, n := range allNodes(tree) {
		if r, ok := identityRank(tree, n, q); ok {
			set.add(tree, n, r)
		}
		if title := strings.ToLower(titleOf(n)); strings.Contains(title, q) {
			r := rankTitle
			switch {
			case title == q:
				r = rankTitleExact
			case strings.HasPrefix(title, q):
				r = rankTitlePrefix
			}
			set.add(tree, n, r)
		}
		qn, ok := n.(*store.Question)
		if !ok {
			continue
		}
		fields := []struct{ name, text string }{{"statement", qn.Statement}}
		if qn.Answer != nil {
			fields = append(fields, struct{ name, text string }{"answer", *qn.Answer})
		}
		for _, f := range fields {
			at := strings.Index(strings.ToLower(f.text), q)
			if at < 0 || strings.TrimSpace(f.text) == "" {
				continue
			}
			set.add(tree, n, rankText).addHit(Hit{
				Path: questionsLedger, Snippet: snippet(f.text, at, len(q)), Field: f.name,
			})
		}
	}
	return set
}

// questionsLedger is where a question's statement and answer live.
var questionsLedger = path.Join(".tiller", "decisions", "open-questions.yml")

// identityRank reports whether the query names the node itself: its id, the
// id's suffix after the noun prefix, or its full reference is an id match;
// its whole key or the key's last segment is a key match; a key containing
// the query is a key fragment.
func identityRank(tree *store.Tree, n store.Node, q string) (rank, bool) {
	id := strings.ToLower(n.NodeID())
	if id == q || strings.ToLower(gen.Ref(tree, n)) == q {
		return rankID, true
	}
	if _, suffix, ok := strings.Cut(id, "-"); ok && suffix == q {
		return rankID, true
	}
	key := strings.ToLower(keyOf(n))
	if key == "" {
		return 0, false
	}
	if key == q || path.Base(key) == q {
		return rankKey, true
	}
	if strings.Contains(key, q) {
		return rankKeyFragment, true
	}
	return 0, false
}

// allNodes is every node search can return, in no particular order; ranked()
// imposes the order.
func allNodes(tree *store.Tree) []store.Node {
	var out []store.Node
	for _, n := range tree.Epics {
		out = append(out, n)
	}
	for _, n := range tree.Features {
		out = append(out, n)
	}
	for _, n := range tree.Tasks {
		out = append(out, n)
	}
	for _, n := range tree.Bugs {
		out = append(out, n)
	}
	for _, n := range tree.Chores {
		out = append(out, n)
	}
	for _, n := range tree.Tweaks {
		out = append(out, n)
	}
	for _, n := range tree.Questions {
		out = append(out, n)
	}
	for _, n := range tree.Amendments {
		out = append(out, n)
	}
	return out
}

// titleOf is the node's human title. A question's comes from gen, which
// derives a bounded one for a legacy question that never recorded its own.
func titleOf(n store.Node) string {
	switch v := n.(type) {
	case *store.Epic:
		return v.Title
	case *store.Feature:
		return v.Title
	case *store.Task:
		return v.Title
	case *store.Bug:
		return v.Title
	case *store.Chore:
		return v.Title
	case *store.Tweak:
		return v.Title
	case *store.Question:
		return gen.QuestionTitle(v)
	case *store.Amendment:
		return v.Title
	}
	return ""
}

// keyOf is a node's canonical key, "" for the ledger items that have none.
func keyOf(n store.Node) string {
	if k := n.NodeKey(); k != nil {
		return *k
	}
	return ""
}
