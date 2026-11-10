package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestBaselineDocTitleReadsTheFirstHeading pins the design doc title the
// drift report leads a row with: the first level-one heading, or "" when
// the file is missing, unsafe, or has none.
func TestBaselineDocTitleReadsTheFirstHeading(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".tiller", "design", "as-is")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "routing.md"), []byte("Intro line\n\n#  Routing design  \n\n## Detail\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"), []byte("No heading here.\n## Only a second level\n"), 0o644))

	require.Equal(t, "Routing design", store.BaselineDocTitle(root, "as-is/routing.md"))
	require.Equal(t, "", store.BaselineDocTitle(root, "as-is/plain.md"))
	require.Equal(t, "", store.BaselineDocTitle(root, "as-is/missing.md"))
	require.Equal(t, "", store.BaselineDocTitle(root, "../escape.md"))
}

// TestBaselineDocTitleSkipsFencesAndFrontMatter pins that a "# " line inside
// front matter or a fenced code block (a shell comment, a YAML comment) is
// never taken for the document's title.
func TestBaselineDocTitleSkipsFencesAndFrontMatter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".tiller", "design")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("fenced.md", "Setup notes.\n\n```sh\n# install the tools\nmake setup\n```\n\n~~~\n# also not a title\n~~~\n")
	write("front.md", "---\n# a yaml comment\ntitle: x\n---\n\n# Real title\n")
	write("fenced-then-title.md", "```\n# not this\n```\n# Deployment design\n")

	require.Equal(t, "", store.BaselineDocTitle(root, "fenced.md"))
	require.Equal(t, "Real title", store.BaselineDocTitle(root, "front.md"))
	require.Equal(t, "Deployment design", store.BaselineDocTitle(root, "fenced-then-title.md"))
}
