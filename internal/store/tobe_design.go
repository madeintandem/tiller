package store

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const designDirPrefix = ".tiller/design/"

// loadToBeDesign gathers Tree.ToBeDesign through the tree's confined reader.
// It never fails the load: the result only decides whether decompose is due,
// so a directory that cannot be listed, a symlink, or a dotfile simply
// contributes no to-be doc.
//
// Registered as-is docs and the sources they cite through --from are the
// baseline, not new work. Both match without case, because a case-insensitive
// filesystem lets link design register a spelling that differs from disk.
func (t *Tree) loadToBeDesign() {
	baseline := make(map[string]bool, len(t.Baseline.Docs))
	for _, doc := range t.Baseline.Docs {
		baseline[strings.ToLower(doc.Path)] = true
		for _, source := range doc.DerivedFrom {
			if rel, ok := strings.CutPrefix(source.Path, designDirPrefix); ok {
				baseline[strings.ToLower(rel)] = true
			}
		}
	}
	var found []string
	var walk func(rel string)
	walk = func(rel string) {
		entries, err := t.stateDirectory(filepath.Join(t.Root, ".tiller", "design", filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			child := path.Join(rel, name)
			switch {
			case e.Type()&fs.ModeSymlink != 0:
				continue
			case e.IsDir():
				walk(child)
			case e.Type().IsRegular():
				if child != "baseline.yml" && baselineCoordinate(child) && !baseline[strings.ToLower(child)] {
					found = append(found, child)
				}
			}
		}
	}
	walk(".")
	sort.Strings(found)
	t.ToBeDesign = found
}
