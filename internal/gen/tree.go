package gen

// This file builds the tree model: a navigable map of the work tree
// (epic -> feature -> task), the data behind `tiller tree`. It is a pure
// function of the loaded store.Tree, like the orientation generator:
// no cobra, no stdout, no filesystem reads of its own. Eval-file presence and
// the parent linkage it reads were recorded by the store on load, so the same
// tree always yields the same model. The cli layer renders the model; this layer
// only shapes it.

import (
	"path"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// TreeModel is the whole map: the project key and the top-level nodes. For a
// whole-project map the nodes are every epic; for a subtree they are the single
// rooted node. The model is deterministic and fully ordered at every grain:
// epics and features by DependencyOrder, a feature's tasks by OrderTasks over
// their recorded depends_on (artifact-schema.md, Tasks: Ordering).
// A consumer of this payload renders the slices as given and sorts nothing.
type TreeModel struct {
	Project string     `json:"project"`
	Nodes   []TreeNode `json:"nodes"`
}

// TreeNode is one node in the work tree: its identity (noun, id, and key
// for the keyed nouns), title, status, task owner or ancestor contributors, and
// derived epic Activity ("active"/"idle", epics only). Path is the prose drill
// target, the node's own .md at every grain. Files is the full footprint (the
// prose .md first, then the .yml, and the *.eval.md when a feature carries one).
// Children are the node's descendants on the axis, always a non-nil slice so the
// JSON shape is stable.
type TreeNode struct {
	Noun         string              `json:"noun"`
	ID           string              `json:"id"`
	Key          string              `json:"key,omitempty"`
	Title        string              `json:"title,omitempty"`
	Status       string              `json:"status"`
	Owner        string              `json:"owner,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
	Activity     string              `json:"activity,omitempty"`
	Path         string              `json:"path"`
	Files        []string            `json:"files"`
	Children     []TreeNode          `json:"children"`
}

// Tree builds the whole-project map: every epic in DependencyOrder, each with
// its features in DependencyOrder, each feature with its tasks.
func Tree(t *store.Tree) TreeModel {
	return projectTree(t, true)
}

// BrowseTree omits cancelled epics and their descendants from console browsing.
// Tree remains the complete CLI view, including cancelled work.
func BrowseTree(t *store.Tree) TreeModel {
	return projectTree(t, false)
}

func projectTree(t *store.Tree, includeCancelled bool) TreeModel {
	order := DependencyOrder(t)
	epics := make([]TreeNode, 0, len(t.Epics))
	for _, id := range order.Epics() {
		e := t.Epics[id]
		if !includeCancelled && e.Status == "cancelled" {
			continue
		}
		epics = append(epics, epicNode(t, order, e))
	}
	return TreeModel{Project: t.Manifest.Project.Key, Nodes: epics}
}

// Subtree builds the map rooted at a single epic or feature, the focused review
// surface `tiller tree epic <node>` (or feature) renders. A task or any other
// noun roots no subtree of its own, so Subtree reports ok=false and the caller
// surfaces misuse.
func Subtree(t *store.Tree, n store.Node) (TreeModel, bool) {
	switch node := n.(type) {
	case *store.Epic:
		return TreeModel{Project: t.Manifest.Project.Key, Nodes: []TreeNode{epicNode(t, DependencyOrder(t), node)}}, true
	case *store.Feature:
		return TreeModel{Project: t.Manifest.Project.Key, Nodes: []TreeNode{featureNode(t, node, OrderTasks(t, t.FeatureTasks[node.ID], node.Order))}}, true
	default:
		return TreeModel{}, false
	}
}

// epicNode projects an epic and its features onto a tree node. The epic carries
// its derived working status so the --status annotation reads the same answer the
// console and the status oracle do.
func epicNode(t *store.Tree, order *Order, e *store.Epic) TreeNode {
	key := keyStr(e.Key)
	dir := epicDir(key)

	// An epic's whole footprint is its narrative and its state. It carries no
	// eval file: its acceptance bar is the sum of its features' contracts
	// (artifact-schema.md, Epics).
	files := []string{path.Join(dir, "epic.md"), path.Join(dir, "epic.yml")}

	children := make([]TreeNode, 0)
	for _, id := range order.Features(key) {
		children = append(children, featureNode(t, t.Features[id], order.Tasks(id)))
	}

	return TreeNode{
		Noun:         store.NounEpic,
		ID:           e.ID,
		Key:          key,
		Title:        e.Title,
		Status:       e.Status,
		Contributors: treeContributors(t, e.ID),
		Activity:     EpicActivity(t, e),
		Path:         path.Join(dir, "epic.md"),
		Files:        files,
		Children:     children,
	}
}

// featureNode projects a feature and its tasks onto a tree node. A task owns its
// own folder, so no task file joins the feature's own footprint.
func featureNode(t *store.Tree, f *store.Feature, tasks []*store.Task) TreeNode {
	key := keyStr(f.Key)
	dir := featureDir(f)

	// Callers supply the dependency-ranked siblings, reusing a tree order
	// where one is already available.
	children := make([]TreeNode, 0, len(tasks))
	for _, tk := range tasks {
		children = append(children, taskNode(dir, tk))
	}

	files := []string{path.Join(dir, "feature.md"), path.Join(dir, "feature.yml")}
	if f.HasEval {
		files = append(files, path.Join(dir, "feature.eval.md"))
	}

	return TreeNode{
		Noun:         store.NounFeature,
		ID:           f.ID,
		Key:          key,
		Title:        f.Title,
		Status:       f.Status,
		Contributors: treeContributors(t, f.ID),
		Path:         path.Join(dir, "feature.md"),
		Files:        files,
		Children:     children,
	}
}

func treeContributors(t *store.Tree, id string) []store.Contributor {
	if contributors := t.Contributors[id]; contributors != nil {
		return contributors
	}
	return []store.Contributor{}
}

// taskNode projects a task onto a leaf node. A task is a folder node, so it
// drills to its own task.md implementation spec and its footprint is that file
// plus its task.yml (artifact-schema.md, Tasks).
//
// A task carrying no key names no folder: path.Join would drop the empty
// segment and produce a path under the FEATURE that is some other node's or
// nobody's. The row still renders, by id, with no path and no files, which is
// the same answer Label and ProsePath give a keyless node.
func taskNode(featureDir string, tk *store.Task) TreeNode {
	node := TreeNode{
		Noun:     store.NounTask,
		ID:       tk.ID,
		Key:      keyStr(tk.Key),
		Title:    tk.Title,
		Status:   tk.Status,
		Owner:    tk.Owner,
		Files:    []string{},
		Children: []TreeNode{},
	}
	local := lastSegment(node.Key)
	if local == "" {
		return node
	}
	dir := path.Join(featureDir, store.TaskDirName, local)
	node.Path = path.Join(dir, store.TaskProseFile)
	node.Files = []string{node.Path, path.Join(dir, store.TaskStateFile)}
	return node
}

// keyStr derefs a node's optional key, returning "" when the node carries none.
func keyStr(k *string) string {
	if k != nil {
		return *k
	}
	return ""
}

// lastSegment returns the final path segment of a canonical key, which is the
// on-disk folder name at that node's own grain: a feature's key carries the
// epic segment and a task's carries both, while the folder is the last one
// (artifact-schema.md, Keys and paths). An empty key yields an empty segment,
// which callers must treat as "no folder" rather than joining it into a path.
func lastSegment(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}
