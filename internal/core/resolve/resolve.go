// Package resolve turns a typed address (an id or a key, plus the validated
// noun the grammar carried) into exactly one loaded node, or a typed error the
// CLI maps to exit 2.
//
// The rules are canonical in docs/spec/cli-surface.md ("A node is addressed by
// id or by key"):
//
//   - An id lookup resolves any addressable node.
//   - A key lookup resolves only the keyed nouns (epic, feature, task,
//     amendment, adr); the one ledger item, question, has no key and is
//     addressed by id only, so a key lookup for it is ErrNotFound.
//   - A feature's canonical key carries the epic segment: <epic-key>/<feature-key>.
//     A task's carries both: <epic-key>/<feature-key>/<task-key>.
//   - An ambiguous key returns ErrAmbiguousKey carrying every candidate. The one
//     exception is the amendment noun, where a key legitimately outlives its
//     holder: a colliding amendment key resolves to the open proposal when there
//     is exactly one (preferOpenAmendment).
//   - A noun that mismatches the resolved node returns ErrNounMismatch, the same
//     checksum failure as a mismatched id prefix.
//
// This package holds no domain logic beyond address resolution and never writes
// to stdout; it reads the in-memory store.Tree laid down by the store package.
package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
)

// Ref is an address as the grammar parsed it: the validated Noun the command
// named and the Raw token the caller typed (an id like "F-79dh3q" or a key like
// "form-engine/certification-form-loading"). An empty Noun means "do not checksum the
// noun", which only makes sense for an id lookup.
type Ref struct {
	Noun string
	Raw  string
}

// Resolver resolves addresses against one loaded tree. It is read-only and
// holds no state beyond that tree, so it is safe to construct per command.
type Resolver struct {
	tree *store.Tree
}

// NewResolver returns a Resolver over an already-loaded tree.
func NewResolver(tree *store.Tree) *Resolver {
	return &Resolver{tree: tree}
}

// Resolve maps a Ref to exactly one node. A Raw that parses as a well-formed id
// is an id lookup over every bucket; anything else is a key lookup scoped to the
// keyed class the Noun names. The returned errors (ErrNotFound, ErrNounMismatch,
// ErrAmbiguousKey) are the exit-2 set the CLI surfaces.
func (r *Resolver) Resolve(ref Ref) (store.Node, error) {
	if id.Valid(ref.Raw) {
		node, ok := r.byID(ref.Raw)
		if !ok {
			return nil, &ErrNotFound{Noun: ref.Noun, Raw: ref.Raw}
		}
		if ref.Noun != "" && ref.Noun != node.Noun() {
			return nil, &ErrNounMismatch{
				Requested: ref.Noun,
				Actual:    node.Noun(),
				ID:        node.NodeID(),
			}
		}
		return node, nil
	}

	// Key lookup. Only the keyed nouns have a key; the question ledger noun (or
	// an absent or unknown noun, which leaves the class undecidable) cannot
	// match a key.
	if !keyedNoun(ref.Noun) {
		return nil, &ErrNotFound{Noun: ref.Noun, Raw: ref.Raw}
	}
	candidates := r.byKey(ref.Noun, ref.Raw)
	if ref.Noun == store.NounAmendment {
		candidates = preferOpenAmendment(candidates)
	}
	switch len(candidates) {
	case 0:
		return nil, &ErrNotFound{Noun: ref.Noun, Raw: ref.Raw}
	case 1:
		return candidates[0], nil
	default:
		return nil, &ErrAmbiguousKey{Noun: ref.Noun, Key: ref.Raw, Candidates: candidates}
	}
}

// byID looks an id up across every node bucket. The tree maps are keyed by id,
// the one stable identifier, so this is a direct lookup per class.
func (r *Resolver) byID(raw string) (store.Node, bool) {
	if n, ok := r.tree.Epics[raw]; ok {
		return n, true
	}
	if n, ok := r.tree.Features[raw]; ok {
		return n, true
	}
	if n, ok := r.tree.Tasks[raw]; ok {
		return n, true
	}
	if n, ok := r.tree.Questions[raw]; ok {
		return n, true
	}
	if n, ok := r.tree.Amendments[raw]; ok {
		return n, true
	}
	if n, ok := r.tree.WorkNode(raw); ok {
		return n, true
	}
	return nil, false
}

// byKey gathers every node of the named keyed class whose canonical key equals
// key. It returns a slice because a well-formed tree yields zero or one match
// but a bad merge can leave two nodes sharing a key, which is the ambiguity the
// caller must see rather than silently pick from.
//
// The candidates are gathered by ranging a map, whose order Go randomizes, and
// they reach the human as the id list in ErrAmbiguousKey's message. Sorting by
// id makes that message the same bytes on every run over the same tree.
func (r *Resolver) byKey(noun, key string) []store.Node {
	var out []store.Node
	switch noun {
	case store.NounEpic:
		for _, e := range r.tree.Epics {
			if e.Key != nil && *e.Key == key {
				out = append(out, e)
			}
		}
	case store.NounFeature:
		for _, f := range r.tree.Features {
			if f.Key != nil && *f.Key == key {
				out = append(out, f)
			}
		}
	case store.NounTask:
		for _, tk := range r.tree.Tasks {
			if tk.Key != nil && *tk.Key == key {
				out = append(out, tk)
			}
		}
	case store.NounAmendment:
		for _, a := range r.tree.Amendments {
			if a.Key != nil && *a.Key == key {
				out = append(out, a)
			}
		}
	default:
		if !store.IsWorkNoun(noun) {
			break
		}
		// A shelf node answers to its canonical key (bugs/<k>) and, since
		// the noun already names the shelf, to the bare local key alone
		// (docs/plans/2026-08-28-bugs-and-chores-design.md, Verbs).
		for _, w := range r.tree.WorkNodesOf(noun) {
			k := w.NodeKey()
			if k != nil && (*k == key || *k == store.ShelfCanonicalKey(noun, key)) {
				out = append(out, w)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID() < out[j].NodeID() })
	return out
}

// preferOpenAmendment narrows a colliding amendment key to the open proposal,
// and it is confined to this one noun on purpose.
//
// Amendment is the only keyed class where two nodes share a key by design rather
// than by corruption. A duplicate key is refused only among PROPOSED amendments
// (mutate.AddAmendment, whose own refusal says "withdraw or approve it before
// reusing the key"), so withdrawing a reshape and opening another under the same
// key is a supported move, and an approved amendment keeps its key forever. With
// status-blind resolution that move made the key permanently unaddressable: every
// key-addressed verb exited 2 as ambiguous, including the copyable
// `tiller approve amendment <key>` the readout's own queue row prints. Two
// features or two epics sharing a key has no such legitimate reading, so nothing
// here touches those; that ambiguity stays an ambiguity.
//
// Two tiers, highest non-empty one wins: the open proposals, then everything
// else. Exactly one candidate in the winning tier resolves; several stay
// ambiguous among themselves, which keeps the caller's choices to ids they can
// actually act on rather than mixing a live proposal in with retired ones.
func preferOpenAmendment(candidates []store.Node) []store.Node {
	if len(candidates) < 2 {
		return candidates
	}
	var open []store.Node
	for _, c := range candidates {
		if a, ok := c.(*store.Amendment); ok && a.Status == store.AmendmentStatusProposed {
			open = append(open, a)
		}
	}
	if len(open) > 0 {
		return open
	}
	return candidates
}

// keyedNoun reports whether a noun is a keyed class that a key lookup can
// resolve. The store does not yet model adr nodes, so a key lookup for that
// class currently finds nothing; the set itself is the canonical one from
// cli-surface.md so it stays correct as that bucket lands.
func keyedNoun(noun string) bool {
	switch noun {
	case store.NounEpic, store.NounFeature, store.NounTask, store.NounAmendment, "adr":
		return true
	default:
		return store.IsWorkNoun(noun)
	}
}

// ErrNotFound reports that no node matched the address. It carries the Noun and
// Raw so the CLI error envelope can name what was looked up.
type ErrNotFound struct {
	Noun string
	Raw  string
}

func (e *ErrNotFound) Error() string {
	if e.Noun == "" {
		return fmt.Sprintf("no node found for %q", e.Raw)
	}
	return fmt.Sprintf("no %s found for %q", e.Noun, e.Raw)
}

// ErrNounMismatch reports that the resolved node's class differs from the noun
// the caller named, the checksum the redundant noun exists to catch. It carries
// the requested and actual nouns plus the resolved id.
type ErrNounMismatch struct {
	Requested string
	Actual    string
	ID        string
}

func (e *ErrNounMismatch) Error() string {
	return fmt.Sprintf("noun mismatch: %s is a %s, not a %s", e.ID, e.Actual, e.Requested)
}

// ErrAmbiguousKey reports that a key matched more than one node of the same
// class. It carries every candidate so the CLI can list their ids in the JSON
// error and the caller can disambiguate by id.
type ErrAmbiguousKey struct {
	Noun       string
	Key        string
	Candidates []store.Node
}

func (e *ErrAmbiguousKey) Error() string {
	ids := make([]string, len(e.Candidates))
	for i, c := range e.Candidates {
		ids[i] = c.NodeID()
	}
	return fmt.Sprintf("ambiguous %s key %q matches %s", e.Noun, e.Key, strings.Join(ids, ", "))
}
