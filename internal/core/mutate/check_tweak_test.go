package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// layStartedTweak lays a tweak driven to in_progress on a gated repo whose
// build and tests gates run the given commands. writeGatedRepo's manifest
// binds every gate on: [feature, task], which is what a shelf node inherits.
func layStartedTweak(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	root := writeGatedRepo(t, buildRun, testsRun)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "TW" {
			return "TW-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounTweak, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tweaks", "alpha", "tweak.md"), []byte("# A\n\n## Reproduction\n\nx\n"), 0o644))
	gitCommitAllM(t, root, "lay tweak")
	driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounTweak, "alpha", "")
	require.NoError(t, err)
	started := driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	return root
}

func TestCheckTweakRunsTaskGradeGatesAndAdvances(t *testing.T) {
	t.Parallel()
	root := layStartedTweak(t, "true", "true")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Tweaks["TW-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.Equal(t, "tweak", res.Noun)
	require.True(t, res.Passed)
	require.Equal(t, "check", res.Status)
	names := map[string]bool{}
	for _, g := range res.Gates {
		names[g.Name] = true
	}
	require.True(t, names["build"])
	require.True(t, names["tests"])
	require.False(t, names["evals"], "a shelf node has no contract, so the evals recursion gate never applies")
	b := load(t, root).Tweaks["TW-aaaaaa"]
	require.Equal(t, store.GateStatusPass, b.Gates["build"].Status)
}

func TestCheckTweakRedGateStaysInProgress(t *testing.T) {
	t.Parallel()
	root := layStartedTweak(t, "true", "false")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Tweaks["TW-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.False(t, res.Passed)
	require.Equal(t, "in_progress", res.Status)
}

func TestSubmitAndResumeTweak(t *testing.T) {
	t.Parallel()
	root := layStartedTweak(t, "true", "true")
	tree := load(t, root)
	_, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Tweaks["TW-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	pr := 41
	res := driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", res.Status)
	require.Equal(t, 41, *res.PR)
	require.Equal(t, 41, *load(t, root).Tweaks["TW-aaaaaa"].PR)
	// The follow-up commits land before the resume, so the code the gates
	// recorded is no longer the code at resume and they go stale.
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644))
	res = driveTo(t, root, store.NounTweak, "alpha", mutate.TransitionInput{Verb: "resume", Reason: "changes requested"})
	require.Equal(t, "in_progress", res.Status)
	b := load(t, root).Tweaks["TW-aaaaaa"]
	require.Equal(t, 41, *b.PR, "resume keeps the PR")
	require.Equal(t, store.GateStatusStale, b.Gates["build"].Status, "resume stales the gates over changed code")
}
