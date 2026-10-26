package gen

// This file is the one shared implementation of the human node-reference
// convention (cli-surface.md, References in human output): a node is named by
// its key path with the id in parentheses, "form-engine/certification-form-loading
// (F-79dh3q)". The path says where, the id says which: keys are renameable by
// design while ids are stable, which is why the id rides along. A question is
// never keyed and stays a bare id. Every human render surface goes through
// these helpers rather than hand-rolling a "key, else id" fallback, so the
// degenerate "task T-x (T-x)" cannot be produced. Machine output (--json) is
// untouched: envelopes carry id and key as separate structured fields.

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// Label is a node's human key path: its canonical key when it has one, and ""
// when it has none. It never substitutes an ancestor's key, because a rendered
// key path is an address: naming a node by a path that resolves to a different
// node is worse than naming it by its id alone, which is what Ref falls back to.
func Label(t *store.Tree, n store.Node) string {
	if k := n.NodeKey(); k != nil {
		return *k
	}
	return ""
}

// Ref is a node's full human reference: "<key-path> (<id>)", or the bare id
// when no key path resolves.
func Ref(t *store.Tree, n store.Node) string {
	return RefParts(Label(t, n), n.NodeID())
}

// RefByID is Ref for a render site that holds only an id (a sync crossing, a
// depends_on entry). Every noun's map is consulted; an id the tree does not
// know comes back unchanged.
func RefByID(t *store.Tree, id string) string {
	if n := NodeByID(t, id); n != nil {
		return Ref(t, n)
	}
	return id
}

// RefParts formats the reference from its two halves, for a renderer that
// already carries the key path and id (a mutate result struct). An empty key
// path degrades to the bare id, never to "id (id)".
func RefParts(keyPath, id string) string {
	if keyPath == "" {
		return id
	}
	return fmt.Sprintf("%s (%s)", keyPath, id)
}

// QuestionTitle returns the persisted short title for a question. Legacy
// questions without one derive a read-only display fallback from the full
// statement: Unicode whitespace collapses to single spaces and results longer
// than 80 code points are visibly truncated to 79 code points plus an ellipsis.
func QuestionTitle(q *store.Question) string {
	if q == nil {
		return ""
	}
	if q.Title != "" {
		return q.Title
	}
	title := strings.Join(strings.Fields(q.Statement), " ")
	runes := []rune(title)
	if len(runes) <= 80 {
		return title
	}
	return string(runes[:79]) + "…"
}

// NodeByID resolves an id across every node map the tree holds: epics,
// features, tasks, questions, amendments, and the bug, chore, and tweak
// shelves. An id the tree does not know comes back nil.
func NodeByID(t *store.Tree, id string) store.Node {
	if t == nil {
		return nil
	}
	if e, ok := t.Epics[id]; ok {
		return e
	}
	if f, ok := t.Features[id]; ok {
		return f
	}
	if tk, ok := t.Tasks[id]; ok {
		return tk
	}
	if q, ok := t.Questions[id]; ok {
		return q
	}
	if a, ok := t.Amendments[id]; ok {
		return a
	}
	if w, ok := t.WorkNode(id); ok {
		return w
	}
	return nil
}
