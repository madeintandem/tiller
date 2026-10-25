package relationships

import (
	"crypto/sha256"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/madeintandem/tiller/internal/store"
)

// AddProvenance extracts only recorded state. It does not read source contents,
// resolve Git revisions, or reinterpret prose frontmatter as an authority.
func AddProvenance(t *store.Tree, idx *Index) {
	for _, node := range documentNodes(t) {
		file, _ := NodePath(t, node, "yml")
		field := "source"
		if node.Noun() == store.NounQuestion {
			field = fmt.Sprintf("questions[%s].source", node.NodeID())
		}
		if file == "" {
			field = fmt.Sprintf("nodes[%s].source", node.NodeID())
		}
		var raw string
		switch n := node.(type) {
		case *store.Epic:
			raw = n.Source
		case *store.Feature:
			raw = n.Source
		case *store.Question:
			raw = n.Source
		case store.WorkNode:
			raw = n.Body().Source
		}
		if raw != "" {
			addCitation(idx, "sourced_from", node.NodeID(), raw, Source{File: file, Field: field, Raw: raw})
		}
	}
	for _, doc := range t.Baseline.Docs {
		id := documentReference(idx, path.Join(".tiller/design", doc.Path))
		if id == "" {
			continue
		}
		field := fmt.Sprintf("docs[%s]", doc.Path)
		for n, raw := range doc.Describes {
			source := Source{File: store.BaselineRelPath, Field: fmt.Sprintf("%s.describes[%d]", field, n), Basis: "baseline_description", Raw: raw, Scope: raw}
			to := recordedPath(t, idx, id, raw, source)
			idx.AddEdge("covers", id, to, source)
		}
		for n, revision := range doc.DerivedFrom {
			source := Source{File: store.BaselineRelPath, Field: fmt.Sprintf("%s.derived_from[%d]", field, n), Raw: revision.Path, Revision: RevisionOf(revision)}
			idx.AddEdge("derived_from", id, recordedPath(t, idx, id, revision.Path, source), source)
		}
	}
	for _, amendment := range t.Amendments {
		file, _ := NodePath(t, amendment, "yml")
		for _, family := range []struct {
			kind   string
			values []string
		}{{"triggered_by", amendment.TriggeredBy}, {"amends_design", amendment.AmendsDesign}, {"implements", []string{amendment.Implements}}} {
			for n, raw := range family.values {
				if raw != "" {
					field := family.kind
					if family.kind != "implements" {
						field = fmt.Sprintf("%s[%d]", field, n)
					}
					addCitation(idx, family.kind, amendment.ID, raw, Source{File: file, Field: field, Raw: raw})
				}
			}
		}
		var intake *store.RevisionRef
		if amendment.Source != nil && amendment.Source.Path != "" {
			intake = &store.RevisionRef{Kind: store.KindGitBlob, Value: amendment.Source.Revision, Path: amendment.Source.Path}
			addCitation(idx, "sourced_from", amendment.ID, amendment.Source.Path, Source{File: file, Field: "source.path", Raw: amendment.Source.Path, Revision: RevisionOf(*intake)})
		}
		for n, operation := range amendment.Operations {
			if operation.Source == "" {
				continue
			}
			source := Source{File: file, Field: fmt.Sprintf("operations[%d].source", n), Raw: operation.Source, Operation: n + 1}
			// An operation only inherits the intake revision when it cites that same
			// artifact. Other sources do not borrow a pin from an unrelated document.
			if intake != nil {
				if coordinate, _, ok := localCitation(operation.Source); ok {
					if artifact, _, valid := localCitation(intake.Path); valid && artifact == coordinate {
						source.Revision = RevisionOf(*intake)
					}
				}
			}
			addCitation(idx, "sourced_from", amendment.ID, operation.Source, source)
		}
	}
}

var citationLocator = regexp.MustCompile(`^L([1-9][0-9]*)(?:-L?([1-9][0-9]*))?$`)

// citationSeparator joins several ranges cited from one document:
// path.md#L31-L58,L321 or path.md#L31-L58, #L321.
var citationSeparator = regexp.MustCompile(`, ?#?`)

// localCitation recognizes the documented complete citation, never a path
// guessed out of arbitrary prose, a URL, a shell string, or a revision token.
// One citation may name several ranges of the same document; it anchors at
// the first range's start line.
func localCitation(raw string) (string, int, bool) {
	file, fragment, hasFragment := strings.Cut(raw, "#")
	if raw != strings.TrimSpace(raw) || strings.ContainsAny(file, "\\\x00\r\n?:") || !strings.HasSuffix(file, ".md") || strings.HasPrefix(file, "/") || path.Clean(file) != file {
		return "", 0, false
	}
	first, _, _ := strings.Cut(file, "/")
	if strings.IndexFunc(first, unicode.IsSpace) >= 0 {
		return "", 0, false
	}
	if _, err := PathID(file); err != nil {
		return "", 0, false
	}
	line := 1
	if hasFragment {
		for n, locator := range citationSeparator.Split(fragment, -1) {
			start, ok := citationRange(locator)
			if !ok {
				return "", 0, false
			}
			if n == 0 {
				line = start
			}
		}
	}
	if strings.HasPrefix(file, "design/") {
		file = ".tiller/" + file
	}
	return file, line, true
}

// UnresolvedCitation reports a value written as a document citation, a .md
// path followed by a # fragment, that provenance cannot resolve. Such a value
// is recorded as opaque text instead of a link to the document. Free text that
// never claimed to be a citation is not unresolved.
func UnresolvedCitation(raw string) bool {
	file, _, hasFragment := strings.Cut(raw, "#")
	if !hasFragment || !strings.HasSuffix(file, ".md") {
		return false
	}
	_, _, ok := localCitation(raw)
	return !ok
}

// citationRange parses one Lstart[-Lend] locator and returns its start line.
func citationRange(locator string) (int, bool) {
	match := citationLocator.FindStringSubmatch(locator)
	if match == nil {
		return 0, false
	}
	start, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	if match[2] != "" {
		end, err := strconv.Atoi(match[2])
		if err != nil || end < start {
			return 0, false
		}
	}
	return start, true
}

func addCitation(idx *Index, kind, from, raw string, source Source) {
	if file, line, ok := localCitation(raw); ok {
		source.CitationLine = line
		source.CitationPath = file
		idx.AddEdge(kind, from, documentReference(idx, file), source)
		return
	}
	idx.AddEdge(kind, from, opaqueReference(idx, from, source, raw), source)
}

func opaqueReference(idx *Index, from string, source Source, raw string) string {
	// Scope opaque tokens to their assertion, so unrelated "ADR 12" or rubric
	// strings never acquire shared identity by accidental text equality.
	sum := sha256.Sum256([]byte(source.File + "\x00" + source.Field + "\x00" + source.Criterion + "\x00" + raw))
	id := fmt.Sprintf("reference:%x", sum[:])
	idx.AddVertex(Vertex{ID: id, Kind: "reference", Raw: raw, Unresolved: true})
	subjects := []string{from, id}
	if strings.HasSuffix(source.File, ".md") {
		subjects = append(subjects, DocumentID(source.File))
	}
	idx.AddDiagnostic(Diagnostic{Code: "unresolved_reference", Message: fmt.Sprintf("unsupported recorded reference %q", raw), Facet: "provenance", Source: &source, Subjects: subjects}, false)
	return id
}

func recordedPath(t *store.Tree, idx *Index, from, raw string, source Source) string {
	if id, err := PathID(raw); err == nil {
		idx.AddVertex(Vertex{ID: id, Kind: "path", Path: strings.TrimPrefix(id, "path:")})
		return id
	}
	if externalCoordinate(t, raw) {
		id := "external_path:" + raw
		idx.AddVertex(Vertex{ID: id, Kind: "external_path", Path: raw, Raw: raw})
		return id
	}
	return opaqueReference(idx, from, source, raw)
}

func externalCoordinate(t *store.Tree, raw string) bool {
	if t.Manifest == nil || raw == "" || strings.ContainsAny(raw, "\\\x00\r\n") || path.Clean(raw) != raw {
		return false
	}
	if !path.IsAbs(raw) && raw != ".." && !strings.HasPrefix(raw, "../") {
		return false
	}
	for _, root := range t.Manifest.Inputs.Code {
		if root.Mode != store.CodeModeReference {
			continue
		}
		coordinate := filepath.ToSlash(filepath.Clean(root.Root))
		// Recorded baseline coordinates are project-relative where possible.
		if filepath.IsAbs(root.Root) && t.Root != "" {
			if relative, err := filepath.Rel(t.Root, root.Root); err == nil {
				coordinate = filepath.ToSlash(relative)
			}
		}
		if raw == coordinate || strings.HasPrefix(raw, strings.TrimSuffix(coordinate, "/")+"/") {
			return true
		}
		// Different volumes retain absolute recorded coordinates.
		absolute := filepath.ToSlash(filepath.Clean(root.Root))
		if path.IsAbs(raw) && path.IsAbs(absolute) && (raw == absolute || strings.HasPrefix(raw, strings.TrimSuffix(absolute, "/")+"/")) {
			return true
		}
	}
	return false
}
