package drift_test

// The read-side half of the baseline-artifact shape rule (the c+a batch, JC
// 2026-08-06): the Acme finding was three healthy features wearing Drifted
// chips because an agent recorded semicolon-joined path lists in one
// baseline artifact string and orphan_eval statted each whole string as a
// single path. A ref that is not a single path is now unresolvable, an
// Uncovered row that says "not looked at", never a false missing candidate.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// TestOrphanEvalMultiPathArtifactIsUncoveredNotAMissingCandidate replays the
// Acme shape exactly: a preserve criterion with a command binding and a
// semicolon-joined artifact list. Before the guard this produced a candidate
// claiming files "no longer exist" while every one sat on disk.
func TestOrphanEvalMultiPathArtifactIsUncoveredNotAMissingCandidate(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	// The gate (mutate.validateSpec's write-side twin of this guard) now
	// refuses the list shape, so the fixture lands it the way Acme's
	// landed: a clean contract passes the gate, then the artifact decays by
	// hand-edit into the pre-gate shape already sitting in real trees.
	const cleanEval = `---
node: F-placeholder
criteria:
  - id: C5
    statement: existing behavior survives the change
    polarity: preserve
    binding: { type: command, run: "true" }
    baseline:
      surface: "app"
      captured_at: { kind: git_tree, value: "git:aaaaaaa", path: app, observed_at: "2026-08-06T00:00:00Z" }
      artifact: "spec/a_spec.rb"
---

# Acceptance
`
	featID, _ := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-multi", "task-a", cleanEval)

	decayed := strings.Replace(cleanEval,
		`artifact: "spec/a_spec.rb"`,
		`artifact: "spec/a_spec.rb; spec/b_spec.rb; spec/requests/authorization"`, 1)
	evalPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "feat-multi", "feature.eval.md")
	require.NoError(t, os.WriteFile(evalPath, []byte(decayed), 0o644))

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	us := uncoveredFor(rep, drift.MarkerOrphanEval)
	require.Len(t, us, 1)
	require.Equal(t, featID, us[0].ID)
	require.Equal(t, "ref_unreadable", us[0].Reason)
	require.Contains(t, us[0].Detail, "not a single path")
	require.Contains(t, us[0].Detail, "C5", "the detail names the criterion")
	require.Contains(t, us[0].Detail, "a baseline artifact names one repo-relative file")
	require.NotContains(t, us[0].Detail, "YAML list", "a baseline artifact is never a list, so no list advice")

	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval),
		"a path list is never guessed at, so it can never read as a false missing")
}
