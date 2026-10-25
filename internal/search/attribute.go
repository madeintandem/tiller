package search

import (
	"strings"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
)

// attribute maps a .tiller/-relative markdown path to the node that owns it,
// or nil for prose that is nobody's (design docs, ADRs, generated views).
// Shelf prose resolves from its canonical shelf key, and an amendment's flat
// <A-id>-<key>.md file from its id. The epic-tree arms are tried innermost
// first, so a file inside a task folder is the task's rather than the
// enclosing feature's, and each falls OUTWARD when its own grain matches no
// node: a task folder the tree cannot place still sits inside a feature that
// it can, and answering "nobody's" there would hide the file rather than
// place it approximately.
func attribute(tree *store.Tree, p string) store.Node {
	parts := strings.Split(p, "/")
	if len(parts) == 2 && parts[0] == "amendments" {
		for id, a := range tree.Amendments {
			if strings.HasPrefix(parts[1], id+"-") || parts[1] == id+".md" {
				return a
			}
		}
		return nil
	}
	for _, shelf := range store.Shelves() {
		if len(parts) < 3 || parts[0] != shelf.Dir {
			continue
		}
		key := store.ShelfCanonicalKey(shelf.Noun, parts[1])
		n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: shelf.Noun, Raw: key})
		if err == nil {
			return n
		}
		return nil
	}
	if len(parts) < 3 || parts[0] != "epics" {
		return nil
	}
	if len(parts) >= 7 && parts[2] == "features" && parts[4] == store.TaskDirName {
		for _, tk := range tree.Tasks {
			if keyOf(tk) == parts[1]+"/"+parts[3]+"/"+parts[5] {
				return tk
			}
		}
	}
	if len(parts) >= 5 && parts[2] == "features" {
		for _, f := range tree.Features {
			if keyOf(f) == parts[1]+"/"+parts[3] {
				return f
			}
		}
	}
	for _, e := range tree.Epics {
		if keyOf(e) == parts[1] {
			return e
		}
	}
	return nil
}
