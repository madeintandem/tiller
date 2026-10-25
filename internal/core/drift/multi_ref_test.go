package drift_test

// A test binding's ref may be a list of files (issue #141). orphan_eval checks
// each path-shaped item on its own, so one missing file is one named item and a
// whitespace-joined string stays a ref it will not guess at.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// multiRefEval is a contract with one preserve criterion whose test binding
// carries the given ref line. Preserve is in scope from approval onward, so the
// fixture needs no merge to start the clock.
func multiRefEval(refLine string) string {
	return `---
node: F-placeholder
criteria:
  - id: C1
    statement: the specs for the change pass
    polarity: preserve
    binding:
      type: test
      ` + refLine + `
---

# Acceptance
`
}

// layMultiRefFeature approves a feature whose contract carries refLine and
// writes the named spec files under the project root.
func layMultiRefFeature(t *testing.T, refLine string, present ...string) (root, featureID string) {
	t.Helper()
	root = scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "spec"), 0o755))
	for _, name := range present {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("# spec\n"), 0o644))
	}
	featureID, _ = layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-multi-ref", "task-a", multiRefEval(refLine))
	return root, featureID
}

func TestOrphanEvalChecksEachItemOfARefList(t *testing.T) {
	t.Run("one missing file is the only item named", func(t *testing.T) {
		root, featureID := layMultiRefFeature(t, "ref: [spec/a_spec.rb, spec/b_spec.rb]", "spec/a_spec.rb")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerOrphanEval)
		require.Len(t, cs, 1)
		require.Equal(t, featureID, cs[0].ID)
		require.Equal(t, []string{"spec/b_spec.rb"}, cs[0].Files)
		require.Empty(t, uncoveredFor(rep, drift.MarkerOrphanEval))
		require.Equal(t, 2, rep.Coverage[drift.MarkerOrphanEval]["checked"], "both items were looked at")
	})

	t.Run("every file present reports nothing", func(t *testing.T) {
		root, _ := layMultiRefFeature(t, "ref: [spec/a_spec.rb, spec/b_spec.rb]", "spec/a_spec.rb", "spec/b_spec.rb")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
		require.Empty(t, uncoveredFor(rep, drift.MarkerOrphanEval))
		require.Equal(t, 2, rep.Coverage[drift.MarkerOrphanEval]["checked"])
	})

	t.Run("a bare test name in the list is unresolvable, a path beside it is still checked", func(t *testing.T) {
		root, _ := layMultiRefFeature(t, "ref: [CalcZoneTest, spec/a_spec.rb]", "spec/a_spec.rb")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
		cov := rep.Coverage[drift.MarkerOrphanEval]
		require.Equal(t, 1, cov["checked"])
		require.Equal(t, 1, cov["unresolvable"])
	})
}

// The legacy shape, paths joined in one string, is still read as one ref that
// cannot be statted, and the detail now tells the author to use a list.
func TestOrphanEvalWhitespaceJoinedRefPointsAtTheListForm(t *testing.T) {
	root, featureID := layMultiRefFeature(t, `ref: "spec/a_spec.rb spec/b_spec.rb"`, "spec/a_spec.rb", "spec/b_spec.rb")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	us := uncoveredFor(rep, drift.MarkerOrphanEval)
	require.Len(t, us, 1)
	require.Equal(t, featureID, us[0].ID)
	require.Equal(t, "ref_unreadable", us[0].Reason)
	require.Contains(t, us[0].Detail, "is not a single path (criterion C1); list several files as a YAML list under ref, one path per item")
	require.NotContains(t, us[0].Detail, "baseline artifact", "a test ref gets the list advice, not the artifact wording")
	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
}
