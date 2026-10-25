package serve

// This file serves one node-source or search destination as structured prose:
// GET /api/prose?path=<repo-relative-md>&line=<positive-line>. The endpoint is
// deliberately JSON rather than a raw document response, so the client owns
// rendering and repository Markdown is never interpreted as server-provided HTML.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
)

// errUnsafeServedPath marks a read refused because some segment of the path is
// a symlink or changed identity while it was opened.
var errUnsafeServedPath = errors.New("unsafe served path")

// maxServedFileBytes caps one evidence or prose read. Each request reads its
// file whole into memory, so without a cap a single request for a huge
// recorded log could exhaust the server. 8 MiB is far above any Markdown
// document or ordinary captured gate output; a larger file answers 413.
const maxServedFileBytes = 8 << 20

var errFileTooLarge = errors.New("file exceeds the served-file size limit")

// tooLarge is the 413 a handler answers when readRegularFromRoot refuses an
// oversized file.
func tooLarge(ref string) *httpError {
	return &httpError{http.StatusRequestEntityTooLarge, fmt.Sprintf("%s is larger than the console serves (%d MiB)", ref, maxServedFileBytes>>20)}
}

type proseResponse struct {
	views.ProseRef
	Line        int                    `json:"line"`
	LineText    string                 `json:"line_text"`
	Baseline    *views.BaselineDocView `json:"baseline,omitempty"`
	DriftDetail *views.NodeDriftView   `json:"drift_detail,omitempty"`
}

// ProseHandler answers one exact, addressable Markdown line together with the
// whole document used to render it. Caller-shape errors, including a path
// through a symlink, are 400; a path outside the served scope (proseIsServed)
// or a valid path or line that vanished is 404; a file over maxServedFileBytes
// is 413; and other unsafe or undisplayable corpus state is 500. The shared api wrapper supplies
// GET/HEAD-only behavior and JSON errors.
func ProseHandler(root string) http.HandlerFunc {
	return apiIntegration(root, func(t *store.Tree, r *http.Request) (any, error) {
		ref := r.URL.Query().Get("path")
		if err := validateProseRef(ref); err != nil {
			return nil, err
		}
		line, err := parseProseLine(r.URL.Query().Get("line"))
		if err != nil {
			return nil, err
		}
		if !proseIsServed(t, ref) {
			return nil, &httpError{http.StatusNotFound, fmt.Sprintf("no prose file at %s", ref)}
		}

		// Registered identity comes from the address the caller asked for,
		// never from where a symlink leads (issue #113): a registered document
		// that has become a symlink is refused the way readiness refuses it,
		// so the console cannot show one document's address with another's
		// prose and human confirmation.
		registered := registeredBaselineDoc(t, ref)
		if registered != nil && !store.BaselineDocPresent(root, registered.Path) {
			return nil, fmt.Errorf("registered design doc .tiller/design/%s is missing or unreadable; restore its prose or remove its links with `tiller unlink design`", registered.Path)
		}

		projectRoot, err := os.OpenRoot(root)
		if err != nil {
			return nil, err
		}
		defer projectRoot.Close()
		name, live := proseFileName(ref)
		if !live {
			return nil, &httpError{http.StatusNotFound, fmt.Sprintf("no prose file at %s", ref)}
		}
		data, err := readRegularFromRoot(projectRoot, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, &httpError{http.StatusNotFound, fmt.Sprintf("no prose file at %s", ref)}
			}
			if errors.Is(err, errUnsafeServedPath) {
				return nil, &httpError{http.StatusBadRequest, "path must not pass through a symlink"}
			}
			if errors.Is(err, errFileTooLarge) {
				return nil, tooLarge(ref)
			}
			return nil, err
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, fmt.Errorf("prose path %s is not displayable UTF-8 text", ref)
		}

		content := string(data)
		lineText, ok := proseLineAt(content, line)
		if !ok {
			return nil, &httpError{http.StatusNotFound, fmt.Sprintf("prose path %s has no line %d", ref, line)}
		}
		response := proseResponse{
			ProseRef: views.ProseRef{Path: ref, Content: content},
			Line:     line,
			LineText: lineText,
		}
		if registered != nil {
			doc := *registered
			baseline := views.BaselineDocOf(doc)
			response.Baseline = &baseline
			candidates, uncovered, err := drift.ForDesign(t, vcs.NewHeadSnapshot(vcs.New(t.Root)), doc)
			if err != nil {
				// A broken check must not erase readable prose or look clean.
				response.DriftDetail = &views.NodeDriftView{
					Uncovered: []views.UncoveredView{{
						Marker: "design", Noun: store.NounDesign, Key: doc.Path,
						Reason: "comparison_error", Detail: err.Error(),
					}},
				}
			} else {
				response.DriftDetail = views.NodeDriftDetailOf(t, candidates, uncovered)
			}
		}
		return response, nil
	})
}

// registeredBaselineDoc finds the ledger entry the requested address names.
// Both the design/ citation alias and the canonical .tiller/design/ address
// map to the same design-relative ledger path. The lookup is lexical on
// purpose: following a symlink here is exactly how one document borrowed
// another's confirmation. Unregistered prose returns nil.
func registeredBaselineDoc(t *store.Tree, ref string) *store.BaselineDoc {
	docPath, ok := strings.CutPrefix(ref, ".tiller/design/")
	if !ok {
		docPath, ok = strings.CutPrefix(ref, "design/")
	}
	if !ok {
		return nil
	}
	for i := range t.Baseline.Docs {
		if t.Baseline.Docs[i].Path == docPath {
			return &t.Baseline.Docs[i]
		}
	}
	return nil
}

// proseIsServed is the reader's scope: the .tiller/ Markdown corpus (with its
// design/ alias), plus any other Markdown document a ledger record cites as a
// local source, exactly the documents the relationship index can grant a file
// destination (readinput.Relationships). Every console link to /file/ comes
// from one of those: search hits and drift findings name corpus files, and
// source links and relationship destinations name corpus or cited documents.
// Anything else in the checkout, such as an uncited README or a .claude/
// note, is not the console's to serve, on a network bind above all. Hidden
// coordinates are refused first (corpus.Servable), so neither .tiller/.cache/
// machine state nor a cited dot-prefixed document outside .tiller/ is served.
// The answer is lexical and touches no disk, so an unserved path is the same
// 404 whether or not a file exists there.
func proseIsServed(t *store.Tree, ref string) bool {
	stored := ref
	if strings.HasPrefix(ref, "design/") {
		stored = ".tiller/" + ref
	}
	if !corpus.Servable(stored) {
		return false
	}
	if strings.HasPrefix(stored, ".tiller/") {
		return true
	}
	vertex, ok := relationships.BuildStructural(t).Vertices[relationships.DocumentID(ref)]
	return ok && vertex.Kind == "document"
}

func validateProseRef(ref string) error {
	switch {
	case ref == "":
		return &httpError{http.StatusBadRequest, "path is required"}
	case strings.ContainsRune(ref, '\x00'):
		return &httpError{http.StatusBadRequest, "path is malformed"}
	case filepath.IsAbs(ref) || pathpkg.IsAbs(ref):
		return &httpError{http.StatusBadRequest, "path must be repo-relative"}
	case pathpkg.Clean(ref) != ref:
		return &httpError{http.StatusBadRequest, "path must not contain traversal or redundant segments"}
	case !strings.EqualFold(pathpkg.Ext(ref), ".md"):
		return &httpError{http.StatusBadRequest, "path must name a Markdown file"}
	default:
		return nil
	}
}

func parseProseLine(raw string) (int, error) {
	if raw == "" {
		return 0, &httpError{http.StatusBadRequest, "line is required"}
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, &httpError{http.StatusBadRequest, "line must be a positive base-10 integer"}
		}
	}
	line, err := strconv.Atoi(raw)
	if err != nil || line <= 0 {
		return 0, &httpError{http.StatusBadRequest, "line must be a positive base-10 integer"}
	}
	return line, nil
}

func proseLineAt(content string, line int) (string, bool) {
	start := 0
	for current := 1; current < line; current++ {
		next := strings.IndexByte(content[start:], '\n')
		if next < 0 {
			return "", false
		}
		start += next + 1
	}
	end := strings.IndexByte(content[start:], '\n')
	if end < 0 {
		return content[start:], true
	}
	return content[start : start+end], true
}

// proseFileName maps a served address to the repo-relative file it names,
// expanding the design/ alias. A retired coordinate names no live document.
// The name is read as-is through readRegularFromRoot, which refuses a symlink
// at any segment, so a served address never yields another file's bytes.
func proseFileName(ref string) (string, bool) {
	stored := ref
	if strings.HasPrefix(ref, "design/") {
		stored = ".tiller/" + ref
	}
	if corpus.Retired(stored) {
		return "", false
	}
	return filepath.FromSlash(stored), true
}

// readRegularFromRoot opens a regular file through a segment-by-segment
// anchored walk, then reads the descriptor whose identity the walk verified.
// O_NOFOLLOW and O_NONBLOCK are applied to the leaf by rootpath.Walker, so
// neither a symlink swap nor a FIFO can redirect or stall the read. It reads
// at most maxServedFileBytes and answers errFileTooLarge past that, counting
// bytes rather than trusting a stat, so a file growing mid-read is caught too.
func readRegularFromRoot(root *os.Root, name string) ([]byte, error) {
	return readRegularFromRootWithWalker(root, name, rootpath.Walker{})
}

func readRegularFromRootWithWalker(root *os.Root, name string, walk rootpath.Walker) ([]byte, error) {
	file, err := walk.Open(root, name, false)
	if err != nil {
		if errors.Is(err, rootpath.ErrUnsafe) {
			return nil, fmt.Errorf("%w: %v", errUnsafeServedPath, err)
		}
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxServedFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxServedFileBytes {
		return nil, errFileTooLarge
	}
	return data, nil
}
