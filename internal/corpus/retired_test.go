package corpus_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRetiredCoordinateIsInert(t *testing.T) {
	for _, kind := range []string{"absent", "stale", "invalid-utf8", "nul", "unreadable", "directory", "outside-symlink", "broken-symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller/design"), 0755))
			retired := filepath.Join(root, ".tiller/board.md")
			outside := filepath.Join(t.TempDir(), "outside.md")
			require.NoError(t, os.WriteFile(outside, []byte("legacyneedle F-zzzzzz"), 0644))
			switch kind {
			case "stale":
				require.NoError(t, os.WriteFile(retired, []byte("legacyneedle F-zzzzzz"), 0644))
			case "invalid-utf8":
				require.NoError(t, os.WriteFile(retired, []byte{0xff}, 0644))
			case "nul":
				require.NoError(t, os.WriteFile(retired, []byte("bad\x00text"), 0644))
			case "unreadable":
				require.NoError(t, os.WriteFile(retired, []byte("legacyneedle F-zzzzzz"), 0000))
			case "directory":
				require.NoError(t, os.Mkdir(retired, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(retired, "nested.md"), []byte("legacyneedle F-zzzzzz\x00"), 0644))
			case "outside-symlink":
				require.NoError(t, os.Symlink(outside, retired))
			case "broken-symlink":
				require.NoError(t, os.Symlink(outside+"-missing", retired))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(retired, 0600))
			}
			before, statErr := os.Lstat(retired)
			const authored = ".tiller/design/board.md"
			require.NoError(t, os.WriteFile(filepath.Join(root, authored), []byte("authoredneedle F-abcdef"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/orientation.md"), []byte("guidance"), 0644))
			var paths []string
			require.NoError(t, corpus.Walk(context.Background(), root, func(d corpus.Document) error { paths = append(paths, d.Path); return nil }))
			require.Equal(t, []string{authored, ".tiller/orientation.md"}, paths)
			for _, coordinate := range []string{".tiller/board.md", ".tiller/board.md/nested.md"} {
				_, err := corpus.Read(context.Background(), root, coordinate)
				require.ErrorIs(t, err, fs.ErrNotExist)
			}
			key := "engine"
			authoredKey := "authored"
			tree := &store.Tree{Root: root, Epics: map[string]*store.Epic{
				"E-abcdef": {ID: "E-abcdef", Key: &key, Source: ".tiller/board.md#L1"},
				"E-ghijkl": {ID: "E-ghijkl", Key: &authoredKey, Source: authored + "#L1"},
			}}
			result, err := search.Search(root, tree, "legacyneedle", search.Options{})
			require.NoError(t, err)
			require.Zero(t, result.Count)
			result, err = search.Search(root, tree, "authoredneedle", search.Options{})
			require.NoError(t, err)
			require.Equal(t, 1, result.Count)
			require.Equal(t, authored, result.Documents[0].Path)
			index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
			require.NoError(t, err)
			require.True(t, index.Complete)
			require.NotContains(t, index.Vertices, "F-zzzzzz")
			require.Contains(t, index.Vertices, "F-abcdef")
			document := index.Vertices["doc:.tiller/board.md"]
			require.True(t, document.Unresolved)
			require.Nil(t, document.Destination)
			require.Equal(t, authored, index.Vertices["doc:"+authored].Destination.Path)
			require.False(t, index.Vertices["doc:"+authored].Generated)
			found := false
			authoredFound := false
			for _, edge := range index.Edges {
				if edge.Kind == "sourced_from" && edge.From == "E-abcdef" {
					found = true
					require.Equal(t, ".tiller/board.md#L1", edge.Sources[0].Raw)
					require.Equal(t, 1, edge.Sources[0].CitationLine)
					require.Empty(t, edge.Sources[0].CitationPath)
				}
				if edge.Kind == "sourced_from" && edge.From == "E-ghijkl" {
					authoredFound = true
					require.Equal(t, authored, edge.Sources[0].CitationPath)
				}
			}
			require.True(t, found)
			require.True(t, authoredFound)
			codes := map[string]bool{}
			for _, diagnostic := range index.Diagnostics {
				codes[diagnostic.Code] = true
			}
			require.True(t, codes["unresolved_document"])
			require.True(t, codes["unresolved_citation"])
			after, err := os.Lstat(retired)
			if kind == "absent" {
				require.ErrorIs(t, err, fs.ErrNotExist)
			} else {
				require.NoError(t, statErr)
				require.NoError(t, err)
				require.True(t, os.SameFile(before, after))
				require.Equal(t, before.Mode(), after.Mode())
				require.Equal(t, before.ModTime(), after.ModTime())
			}
			data, err := os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, "legacyneedle F-zzzzzz", string(data))
		})
	}
}

func TestRetirementDoesNotHideUnsafeSupportedDocuments(t *testing.T) {
	for _, coordinate := range []string{".tiller/design/board.md", ".tiller/orientation.md"} {
		for _, kind := range []string{"nul", "invalid-utf8", "directory", "outside-symlink", "broken-symlink", "fifo"} {
			t.Run(coordinate+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, coordinate)), 0755))
				filename := filepath.Join(root, coordinate)
				switch kind {
				case "nul":
					require.NoError(t, os.WriteFile(filename, []byte("bad\x00text"), 0644))
				case "invalid-utf8":
					require.NoError(t, os.WriteFile(filename, []byte{0xff}, 0644))
				case "directory":
					require.NoError(t, os.Mkdir(filename, 0755))
				case "outside-symlink", "broken-symlink":
					outside := filepath.Join(t.TempDir(), "outside.md")
					if kind == "outside-symlink" {
						require.NoError(t, os.WriteFile(outside, []byte("outside"), 0644))
					}
					require.NoError(t, os.Symlink(outside, filename))
				case "fifo":
					require.NoError(t, syscall.Mkfifo(filename, 0600))
				}
				require.Error(t, corpus.Walk(context.Background(), root, func(corpus.Document) error { return nil }))
				_, err := corpus.Read(context.Background(), root, coordinate)
				require.Error(t, err)
				tree := &store.Tree{Root: root}
				_, err = search.Search(root, tree, "needle", search.Options{})
				require.Error(t, err)
				_, err = readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
				require.Error(t, err)
			})
		}
	}
}
