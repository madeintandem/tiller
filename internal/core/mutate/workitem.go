package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
)

// isShelfRef reports whether raw is written as shelf work: a shelf id
// (B-xxxxxx) or a shelf key (bugs/<key>).
func isShelfRef(raw string) bool {
	_, ok := store.ShelfOfRef(raw)
	return ok
}

// shelfDirs returns every shelf's folder name, which is also its plural.
func shelfDirs() []string {
	var dirs []string
	for _, s := range store.Shelves() {
		dirs = append(dirs, s.Dir)
	}
	return dirs
}

// englishList joins words as an English list with conj before the last:
// "a", "a or b", "a, b, or c". Refusals that enumerate the shelf nouns build
// from the shelf table through it, so they never fall out of step.
func englishList(conj string, words ...string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " " + conj + " " + words[1]
	}
	return strings.Join(words[:len(words)-1], ", ") + ", " + conj + " " + words[len(words)-1]
}

// notShelfNoun refuses a noun no shelf holds, naming every shelf noun.
func notShelfNoun(noun string) error {
	return &usageError{reason: "unsupported_noun", msg: fmt.Sprintf("%q is not a shelf noun (%s)", noun, englishList("or", store.ShelfNouns()...))}
}

// shelfOrderEvent is the event type a shelf's order change records:
// order.<shelf dir>, as order.bugs. An unknown noun panics.
func shelfOrderEvent(noun string) string {
	return "order." + store.ShelfDir(noun)
}

// workItemRelDir is the repo-relative folder of one shelf node, derived from
// its canonical key the way featureRelDir derives a feature's.
func workItemRelDir(w store.WorkNode) string {
	return store.ShelfRelDir(w.Noun(), localKey(deref(w.NodeKey())))
}

// loadWorkItemFile reads one shelf node's state file back off disk, the
// twin of loadTaskFile: the on-disk document, its repo-relative path, and
// the snapshot the eventual write is planned against. The identity check
// keeps a rewrite from landing on a neighbor's file.
func loadWorkItemFile(tree *store.Tree, w store.WorkNode) (store.WorkNode, string, store.FileSnapshot, error) {
	if w.NodeKey() == nil {
		return nil, "", store.FileSnapshot{}, fmt.Errorf("mutate: %s %s carries no key, so its folder cannot be named", w.Noun(), w.NodeID())
	}
	ymlRel := filepath.Join(workItemRelDir(w), store.ShelfStateFile(w.Noun()))
	doc, err := store.NewWorkNode(w.Noun())
	if err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), doc)
	if err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	b := doc.Body()
	if err := validateFreshNodeIdentity(w.Noun(), w.NodeID(), ymlRel, b.ID, deref(b.Key), deref(w.NodeKey()), store.ShelfDir(w.Noun())); err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	return doc, ymlRel, snapshot, nil
}

// resolveAttachTarget resolves the node a bug or chore points at: an epic,
// a feature, or a task, by id or key (docs/plans/2026-08-28-bugs-and-chores-
// design.md, Attachment). A bare key is an epic's, one slash a feature's,
// two a task's; an id names its class by prefix. Anything else refuses.
func resolveAttachTarget(tree *store.Tree, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	r := resolve.NewResolver(tree)
	var noun string
	switch {
	case strings.HasPrefix(raw, "E-"):
		noun = store.NounEpic
	case strings.HasPrefix(raw, "F-"):
		noun = store.NounFeature
	case strings.HasPrefix(raw, "T-"):
		noun = store.NounTask
	case hasIDPrefix(raw, "Q-", "A-", "ADR-", "EV-", "AT-"), isShelfRef(raw):
		return "", &usageError{reason: "attach_target", msg: fmt.Sprintf("attach target %q is not an epic, feature, or task", raw)}
	default:
		switch strings.Count(raw, "/") {
		case 0:
			noun = store.NounEpic
		case 1:
			noun = store.NounFeature
		default:
			noun = store.NounTask
		}
	}
	n, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
	if err != nil {
		return "", err
	}
	return n.NodeID(), nil
}

// workItemEventType is "<verb>.<noun>" for a shelf noun, the same shape the
// per-noun add/link/unlink/rename constants spell out for the tree nouns.
func workItemEventType(verb, noun string) string {
	return verb + "." + noun
}
