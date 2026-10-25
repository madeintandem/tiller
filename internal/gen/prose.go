package gen

// This file is the read side's one derivation of where a node's prose lives
// (artifact-schema.md, Directory layout): epic.md, feature.md, and task.md
// inside the node's own folder. The tree map and the console's node page both
// name these files, so the layout knowledge sits here once instead of being
// restated per surface. A question is a ledger entry that owns no file; its
// prose handle (the statement and answer) rides on the node itself.

import (
	"path"

	"github.com/madeintandem/tiller/internal/store"
)

// ProsePath returns the repo-relative path of a node's prose .md, the same
// drill target `tree --files` prints, and ok=false for a noun that has none. A
// task resolves through its parent feature, whose folder its own sits under, so
// a task the tree cannot place answers ok=false.
func ProsePath(t *store.Tree, n store.Node) (string, bool) {
	switch v := n.(type) {
	case *store.Epic:
		return path.Join(epicDir(keyStr(v.Key)), "epic.md"), true
	case *store.Feature:
		return path.Join(featureDir(v), "feature.md"), true
	case *store.Task:
		if dir, ok := taskDir(t, v); ok {
			return path.Join(dir, store.TaskProseFile), true
		}
	case *store.Amendment:
		// An amendment is not folder-backed (store/amendment.go's own doc
		// comment): its narrative lives in a flat file pair under
		// .tiller/amendments/, named <A-id>-<key>.md (artifact-schema.md,
		// Amendments). This mirrors mutate's own amendmentRelMD exactly;
		// ProsePath already encodes every other noun's directory layout
		// independently here rather than importing mutate for it, which
		// cannot happen anyway (mutate imports gen, so the reverse would
		// cycle).
		return path.Join(".tiller", "amendments", v.ID+"-"+keyStr(v.Key)+".md"), true
	case *store.Bug, *store.Chore, *store.Tweak:
		w, _ := store.AsWorkNode(n)
		if w.NodeKey() == nil {
			return "", false
		}
		return path.Join(store.ShelfRelDir(w.Noun(), lastSegment(*w.NodeKey())), store.ShelfProseFile(w.Noun())), true
	}
	return "", false
}

// taskDir is a task's own folder under its parent feature: the feature's folder,
// then tasks/, then the LOCAL task-key segment (artifact-schema.md, Keys and
// paths).
func taskDir(t *store.Tree, tk *store.Task) (string, bool) {
	if t == nil || tk.Key == nil {
		return "", false
	}
	f, ok := t.Features[tk.FeatureID]
	if !ok || f.Key == nil {
		return "", false
	}
	return path.Join(featureDir(f), store.TaskDirName, lastSegment(*tk.Key)), true
}

// EvalPath returns the repo-relative path of a node's authored eval contract
// (feature.eval.md), the same directory ProsePath resolves feature.md from, and
// ok=false for every noun that carries none. The feature is the one grain where
// acceptance lives: an epic's bar is the sum of its features' contracts, and a
// task's and a question's acceptance rides on the node itself
// (artifact-schema.md, Epics; Gates). Presence on disk is the caller's question
// (store.Feature.HasEval); this is the path alone.
func EvalPath(_ *store.Tree, n store.Node) (string, bool) {
	if v, ok := n.(*store.Feature); ok {
		return path.Join(featureDir(v), "feature.eval.md"), true
	}
	return "", false
}

// epicDir is an epic's folder in the work tree.
func epicDir(key string) string {
	return path.Join(".tiller", "epics", key)
}

// featureDir is a feature's folder inside its owning epic. The folder name is
// the key's last segment because a feature's canonical key carries the epic
// segment.
func featureDir(f *store.Feature) string {
	return path.Join(".tiller", "epics", f.EpicKey, "features", lastSegment(keyStr(f.Key)))
}
