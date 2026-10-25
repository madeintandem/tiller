package relationships

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The shared testdata/work-mentions.json corpus is also exercised through the
// actual TypeScript renderer; changes to its token or syntax policy must agree.
var workMention = regexp.MustCompile(`[EFT]-[a-z0-9]{6}`)

// mention is an immutable, path- and tree-independent parsing fact.
type mention struct {
	id   string
	line int
}

func addMentions(idx *Index, document Document, markdown *goldmark.Markdown) {
	documentID := DocumentID(document.Path)
	for _, fact := range documentMentions.mentions(document.Content, markdown) {
		id := fact.id
		citation := Source{File: document.Path, Line: fact.line}
		if _, exists := idx.Vertices[id]; !exists {
			kind := "task"
			if id[0] == 'E' {
				kind = "epic"
			} else if id[0] == 'F' {
				kind = "feature"
			}
			idx.AddVertex(Vertex{ID: id, Kind: kind, Raw: id, Unresolved: true})
		}
		if idx.Vertices[id].Unresolved {
			idx.AddDiagnostic(Diagnostic{Code: "unresolved_mention", Message: fmt.Sprintf("%s mentions unknown work ID %s", document.Path, id), Facet: "mentions", Source: &citation, Subjects: []string{documentID, id}}, false)
		}
		idx.AddEdge("mentions", documentID, id, citation)
	}
}

func parseMentions(source []byte, markdown *goldmark.Markdown) []mention {
	if *markdown == nil {
		*markdown = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))
	}
	root := (*markdown).Parser().Parse(text.NewReader(source))
	var facts []mention
	lines := []int{0}
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, i+1)
		}
	}
	record := func(id []byte, offset int) {
		line := sort.SearchInts(lines, offset+1)
		facts = append(facts, mention{id: string(id), line: line})
	}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Link, *ast.AutoLink, *ast.Image, *ast.CodeBlock, *ast.FencedCodeBlock, *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			var value strings.Builder
			first := -1
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if chunk, ok := child.(*ast.Text); ok {
					if first < 0 {
						first = chunk.Segment.Start
					}
					value.Write(chunk.Value(source))
				}
			}
			token := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value.String())
			if len(token) == 8 && workMention.MatchString(token) {
				offset := first
				for child := n.FirstChild(); child != nil; child = child.NextSibling() {
					if chunk, ok := child.(*ast.Text); ok {
						if at := bytes.Index(chunk.Value(source), []byte(token)); at >= 0 {
							offset = chunk.Segment.Start + at
							break
						}
					}
				}
				record([]byte(token), offset)
			}
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			raw := n.Value(source)
			value := raw
			if !n.IsRaw() {
				value = mentionText(raw)
			}
			for _, match := range workMention.FindAllIndex(value, -1) {
				if !mentionBoundary(value, match[0], match[1]) {
					continue
				}
				// Text segments are line-local; decoding entities may change byte offsets
				// but never the source line. Preserve raw offsets when no decode occurred.
				offset := n.Segment.Start
				if bytes.Equal(raw, value) {
					offset += match[0]
				}
				record(value[match[0]:match[1]], offset)
			}
		}
		return ast.WalkContinue, nil
	})
	return facts
}

func mentionBoundary(value []byte, start, end int) bool {
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	if start > 0 {
		r, _ := utf8.DecodeLastRune(value[:start])
		if word(r) || strings.ContainsRune("/@.-", r) {
			return false
		}
	}
	if end < len(value) {
		r, size := utf8.DecodeRune(value[end:])
		if word(r) || r == '/' || r == '-' {
			return false
		}
		if r == '.' && end+size < len(value) {
			next, _ := utf8.DecodeRune(value[end+size:])
			if word(next) {
				return false
			}
		}
	}
	return true
}

// Decode text-node escapes once, in source order. Running independent global
// unescape/entity passes would wrongly decode escaped or nested ampersands.
// Markdown block/inline classification itself stays entirely in Goldmark.
func mentionText(raw []byte) []byte {
	out := util.NewCopyOnWriteBuffer(raw)
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) && util.IsPunct(raw[i+1]) {
			out.Write(raw[start:i])
			_ = out.WriteByte(raw[i+1])
			i++
			start = i + 1
			continue
		}
		if raw[i] != '&' {
			continue
		}
		end := i + 1
		for end < len(raw) && (util.IsAlphaNumeric(raw[end]) || raw[end] == '#') {
			end++
		}
		if end >= len(raw) || raw[end] != ';' {
			continue
		}
		token := raw[i : end+1]
		var decoded []byte
		if len(token) > 1 && token[1] == '#' {
			decoded = util.ResolveNumericReferences(token)
		} else {
			decoded = util.ResolveEntityNames(token)
		}
		if bytes.Equal(token, decoded) {
			continue
		}
		out.Write(raw[start:i])
		out.Write(decoded)
		i = end
		start = end + 1
	}
	if out.IsCopied() {
		out.Write(raw[start:])
	}
	return out.Bytes()
}
