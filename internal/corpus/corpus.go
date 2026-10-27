// Package corpus reads the confined, displayable Markdown corpus under .tiller.
// Retired coordinates are excluded before filesystem validation.
package corpus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/rootpath"
)

type Document struct {
	Path    string
	Content []byte
}

// ErrTooLarge marks a document refused before parsing by a bounded reader.
var ErrTooLarge = errors.New("document exceeds the read size limit")

// Retired reports whether a repository coordinate is outside the live document
// corpus because it names the retired root board or a descendant obstruction.
// Authored documents with the same basename are not retired.
func Retired(coordinate string) bool {
	return coordinate == ".tiller/board.md" || strings.HasPrefix(coordinate, ".tiller/board.md/")
}

// Servable is the single destination admission policy for one repository
// Markdown coordinate: whether the console may serve it at all, and so whether
// any read surface may advertise it as a readable file. The .tiller/ corpus
// root is the only dot-prefixed segment admitted. Any other dot-prefixed
// segment, such as .notes/, .github/, or .tiller/.cache/, names hidden local
// state that is never served, cited or not. A citation can widen the served
// set to an outside document; it never widens it past this policy. Being able
// to represent a coordinate (relationships.DocumentID) grants nothing here.
// The answer is lexical, so a refused coordinate is refused whether or not a
// file exists there. Walk applies the same rule while descending the corpus.
func Servable(coordinate string) bool {
	rest := strings.TrimPrefix(coordinate, ".tiller/")
	for _, segment := range strings.Split(rest, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return true
}

// Walk visits every supported lowercase .md file in lexical walk order, skipping
// dot-prefixed files and directories. Symlinks
// in the live corpus, nonregular Markdown, and nondisplayable bytes fail the read.
// Rooted single-segment descent preserves confinement across directory swaps.
// Each walk reads and validates current bytes; file metadata is not a content
// identity.
func Walk(ctx context.Context, root string, visit func(Document) error) error {
	return WalkLimited(ctx, root, 0, visit)
}

// WalkLimited applies a per-document byte limit before parsing or visiting.
// A zero limit preserves the CLI's unbounded corpus policy.
func WalkLimited(ctx context.Context, root string, maxBytes int64, visit func(Document) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	projectRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer projectRoot.Close()
	corpusInfo, err := projectRoot.Lstat(".tiller")
	if err != nil {
		return err
	}
	if corpusInfo.Mode()&fs.ModeSymlink != 0 || !corpusInfo.IsDir() {
		return fmt.Errorf(".tiller is not a regular corpus directory")
	}
	corpusRoot, err := (rootpath.Walker{}).OpenRoot(projectRoot, ".tiller")
	if err != nil {
		return err
	}
	defer corpusRoot.Close()
	openedInfo, err := corpusRoot.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(corpusInfo, openedInfo) {
		return fmt.Errorf(".tiller changed while opening corpus")
	}
	checkRoot := func() error {
		current, err := projectRoot.Lstat(".tiller")
		if err != nil {
			return err
		}
		if current.Mode()&fs.ModeSymlink != 0 || !os.SameFile(current, openedInfo) {
			return fmt.Errorf(".tiller changed during corpus read")
		}
		return nil
	}
	if err := checkRoot(); err != nil {
		return err
	}
	reader := rootpath.NewReader(corpusRoot, rootpath.Walker{})
	defer reader.Close()
	fsys := corpusFS{reader: reader}
	err = fs.WalkDir(fsys, ".", func(p string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		coordinate := path.Join(".tiller", p)
		if Retired(coordinate) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Usage files are token counts, not prose: "Nothing else walks the
		// usage files" (docs/plans/2026-10-26-agent-use-report-design.md).
		if p == "usage" && entry.IsDir() {
			return fs.SkipDir
		}
		// Dot entries are local and gitignored (.cache), never corpus; the
		// console refuses to serve them, so search must not quote them either.
		if p != "." && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", coordinate)
		}
		if entry.IsDir() {
			if p != "." && strings.HasSuffix(p, ".md") {
				return fmt.Errorf("%s is not a regular file", coordinate)
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", coordinate)
		}
		file, err := fsys.Open(p)
		if err != nil {
			return err
		}
		data, err := readContent(file, maxBytes)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return fmt.Errorf("%s: %w", coordinate, errors.Join(err, closeErr))
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return fmt.Errorf("%s is not displayable UTF-8 prose", coordinate)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return visit(Document{Path: coordinate, Content: data})
	})
	if err != nil {
		return err
	}
	if err := checkRoot(); err != nil {
		return err
	}
	return nil
}

// Read opens one explicitly cited repository Markdown document. It does not
// extend Walk's corpus, enumerate neighboring files, or follow symlinks.
func Read(ctx context.Context, root, coordinate string) (Document, error) {
	return ReadLimited(ctx, root, coordinate, 0)
}

// ReadLimited reads one cited document with an optional byte limit.
func ReadLimited(ctx context.Context, root, coordinate string, maxBytes int64) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if !fs.ValidPath(coordinate) || !strings.HasSuffix(coordinate, ".md") {
		return Document{}, fmt.Errorf("invalid Markdown coordinate %q", coordinate)
	}
	if Retired(coordinate) {
		return Document{}, &fs.PathError{Op: "open", Path: coordinate, Err: fs.ErrNotExist}
	}
	projectRoot, err := os.OpenRoot(root)
	if err != nil {
		return Document{}, err
	}
	defer projectRoot.Close()
	file, err := (rootpath.Walker{}).Open(projectRoot, coordinate, false)
	if err != nil {
		return Document{}, err
	}
	defer file.Close()
	data, err := readContent(file, maxBytes)
	if err != nil {
		return Document{}, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return Document{}, fmt.Errorf("%s is not displayable UTF-8 prose", coordinate)
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	return Document{Path: coordinate, Content: data}, nil
}

func readContent(file fs.File, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(file)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// corpusFS never reopens a cumulative pathname through a swappable directory:
// every open goes through one rootpath.Reader, which verifies each directory
// once and keeps it anchored for the rest of the walk.
type corpusFS struct {
	reader *rootpath.Reader
}

func (f corpusFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("%q is not a valid corpus path", name)
	}
	file, err := f.reader.Open(name, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path.Join(".tiller", name), err)
	}
	return file, nil
}
