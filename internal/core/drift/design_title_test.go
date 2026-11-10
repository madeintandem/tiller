package drift_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// TestDesignRowsCarryTheDocumentsTitle pins that a design finding or
// uncovered row names the document by its own first heading, so a reader
// sees "Model" before the path as-is/model.md.
func TestDesignRowsCarryTheDocumentsTitle(t *testing.T) {
	t.Run("an unconfirmed doc's uncovered row", func(t *testing.T) {
		tree, _ := baselineFixture(t, "app/main.go")
		tree.Baseline.Docs[0].Confirmed = nil
		rep, err := drift.Run(tree, vcs.New(tree.Root))
		require.NoError(t, err)
		require.NotEmpty(t, rep.Uncovered)
		for _, u := range rep.Uncovered {
			require.Equal(t, "Model", u.Title)
		}
	})

	t.Run("a candidate for described code that is gone", func(t *testing.T) {
		tree, _ := baselineFixture(t, "app/main.go")
		require.NoError(t, os.Remove(filepath.Join(tree.Root, "app/main.go")))
		rep, err := drift.Run(tree, vcs.New(tree.Root))
		require.NoError(t, err)
		require.NotEmpty(t, rep.Candidates)
		for _, c := range rep.Candidates {
			require.Equal(t, "Model", c.Title)
		}
	})
}
