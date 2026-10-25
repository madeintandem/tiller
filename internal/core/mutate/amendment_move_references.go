package mutate

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Only current authored coordinates are analyzed. RevisionRef paths, event
// files and baseline inventories are historical pins, not rewrite candidates.
func moveReferenceWarnings(tree *store.Tree, feature *store.Feature, newKey string) ([]string, error) {
	oldDir := filepath.ToSlash(featureRelDir(feature))
	epic, local, _ := strings.Cut(newKey, "/")
	newDir := path.Join(".tiller/epics", epic, "features", local)
	relocated := func(file string) string {
		if file == oldDir || strings.HasPrefix(file, oldDir+"/") {
			return newDir + strings.TrimPrefix(file, oldDir)
		}
		return file
	}
	resolvePath := func(file, raw string) string {
		raw, _, _ = strings.Cut(raw, "#")
		raw, _, _ = strings.Cut(raw, "?")
		decoded, err := url.PathUnescape(raw)
		if err == nil {
			raw = decoded
		}
		if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, ":") {
			return ""
		}
		if strings.HasPrefix(raw, ".tiller/") {
			return path.Clean(raw)
		}
		return path.Clean(path.Join(path.Dir(file), raw))
	}
	found := map[string]bool{}
	warn := func(file, what string) {
		found[file+": "+what+" needs review; authored content is not rewritten"] = true
	}
	markdown := goldmark.New()
	err := corpus.Walk(context.Background(), tree.Root, func(document corpus.Document) error {
		file := document.Path
		if file == ".tiller/orientation.md" {
			return nil
		}
		root := markdown.Parser().Parse(text.NewReader(document.Content))
		err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var raw string
			switch n := node.(type) {
			case *ast.Link:
				raw = string(n.Destination)
			case *ast.Image:
				raw = string(n.Destination)
			case *ast.CodeSpan:
				var token strings.Builder
				for child := n.FirstChild(); child != nil; child = child.NextSibling() {
					if segment, ok := child.(*ast.Text); ok {
						token.Write(segment.Segment.Value(document.Content))
					}
				}
				raw = token.String()
				if !strings.HasPrefix(raw, ".tiller/") {
					return ast.WalkContinue, nil
				}
			}
			before := resolvePath(file, raw)
			if before == "" {
				return ast.WalkContinue, nil
			}
			after := resolvePath(relocated(file), raw)
			if after != relocated(before) {
				warn(file, fmt.Sprintf("path citation %q", raw))
			}
			return ast.WalkContinue, nil
		})
		if err != nil {
			return err
		}
		if strings.HasSuffix(file, "/feature.eval.md") {
			contract, err := store.ParseEvalFile(document.Content)
			if err != nil {
				return nil
			}
			for _, criterion := range contract.Criteria {
				// Commands are repository-root based; test refs are recorded coordinates.
				// A baseline pin is deliberately excluded from this live-binding scan.
				binding := criterion.Binding
				if strings.Contains(binding.Ref.Joined(), oldDir) || strings.Contains(binding.Run, oldDir) {
					warn(file, "acceptance binding "+criterion.ID)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, node := range moveNodes(tree) {
		source := ""
		switch n := node.(type) {
		case *store.Epic:
			source = n.Source
		case *store.Feature:
			source = n.Source
		case store.WorkNode:
			source = n.Body().Source
		}
		if strings.Contains(source, oldDir) {
			warn(node.NodeID(), "source provenance citation "+source)
		}
	}
	for _, question := range tree.Questions {
		if strings.Contains(question.Source, oldDir) {
			warn(question.ID, "source provenance citation "+question.Source)
		}
	}
	var warnings []string
	for warning := range found {
		warnings = append(warnings, warning)
	}
	sort.Strings(warnings)
	return warnings, nil
}
