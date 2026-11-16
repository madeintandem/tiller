package search

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
)

// sourcePaths is every file the baseline ledger records as a derived_from
// source that search reads beside the .tiller/ corpus, once each, in path
// order. A path under .tiller/ is left out: the corpus walk already covers
// the Markdown there, and the rest is state. A path the corpus policy calls
// hidden (a dot-prefixed segment) is left out as it is everywhere else.
func sourcePaths(ledger store.BaselineLedger) []string {
	var paths []string
	for _, doc := range ledger.Docs {
		for _, source := range doc.DerivedFrom {
			p := source.Path
			if p == ".tiller" || strings.HasPrefix(p, ".tiller/") || !corpus.Servable(p) {
				continue
			}
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// walkSources visits the current working-tree content of each ledger source.
// The ledger outlives the files it names, so a source that is gone is skipped,
// and so is a directory standing at a source's path (the ledger records file
// blobs, and search does not walk what it was not pointed at). A source may be
// any file, so one that is not UTF-8 text is skipped too. Everything else is
// the confinement the .tiller/ corpus has: each path component is opened
// beneath the project root without following a symlink, and a symlink, a
// non-regular file, or a read failure is returned as an error that refuses
// the whole search.
func walkSources(root string, ledger store.BaselineLedger, visit func(sourcePath string, content []byte)) error {
	paths := sourcePaths(ledger)
	if len(paths) == 0 {
		return nil
	}
	projectRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer projectRoot.Close()
	for _, p := range paths {
		info, err := projectRoot.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err == nil && info.IsDir() {
			continue
		}
		// Any other Lstat answer is left to the anchored open, which is the
		// one that decides what is safe to read.
		content, err := readSource(projectRoot, p)
		if err != nil {
			return fmt.Errorf("baseline source %s: %w", p, err)
		}
		if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			continue
		}
		visit(p, content)
	}
	return nil
}

func readSource(projectRoot *os.Root, p string) ([]byte, error) {
	file, err := (rootpath.Walker{}).Open(projectRoot, p, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
