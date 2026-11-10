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
