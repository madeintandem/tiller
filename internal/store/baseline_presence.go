package store

import (
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// BaselineDocPresent reports whether a registered design path can be opened as
// a regular file beneath the project root without traversing symlinks. It reads
// no prose bytes; missing, unsafe, and unreadable paths are all absent.
func BaselineDocPresent(root, doc string) bool {
	if ValidateDesignPath(doc) != nil {
		return false
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return false
	}
	defer anchored.Close()
	file, err := (rootpath.Walker{}).Open(anchored, filepath.Join(".tiller", "design", filepath.FromSlash(doc)), false)
	if err != nil {
		return false
	}
	defer file.Close()
	return true
}
