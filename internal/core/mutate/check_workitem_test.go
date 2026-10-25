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

// layStartedBug lays a bug driven to in_progress on a gated repo whose
// build and tests gates run the given commands. writeGatedRepo's manifest
// binds every gate on: [feature, task], which is what a shelf node inherits.
func layStartedBug(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	root := writeGatedRepo(t, buildRun, testsRun)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "bugs", "alpha", "bug.md"), []byte("# A\n\n## Reproduction\n\nx\n"), 0o644))
	gitCommitAllM(t, root, "lay bug")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	require.NoError(t, err)
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "start"})
	return root
}

func TestCheckBugRunsTaskGradeGatesAndAdvances(t *testing.T) {
	t.Parallel()
	root := layStartedBug(t, "true", "true")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.Equal(t, "bug", res.Noun)
	require.True(t, res.Passed)
	require.Equal(t, "check", res.Status)
	names := map[string]bool{}
	for _, g := range res.Gates {
		names[g.Name] = true
	}
	require.True(t, names["build"])
	require.True(t, names["tests"])
	require.False(t, names["evals"], "a shelf node has no contract, so the evals recursion gate never applies")
	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, store.GateStatusPass, b.Gates["build"].Status)
}

func TestCheckBugRedGateStaysInProgress(t *testing.T) {
	t.Parallel()
	root := layStartedBug(t, "true", "false")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.False(t, res.Passed)
	require.Equal(t, "in_progress", res.Status)
}

func TestSubmitAndResumeBug(t *testing.T) {
	t.Parallel()
	root := layStartedBug(t, "true", "true")
	tree := load(t, root)
	_, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	pr := 41
	res := driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", res.Status)
	require.Equal(t, 41, *res.PR)
	require.Equal(t, 41, *load(t, root).Bugs["B-aaaaaa"].PR)
	// The follow-up commits land before the resume, so the code the gates
	// recorded is no longer the code at resume and they go stale.
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644))
	res = driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "resume", Reason: "changes requested"})
	require.Equal(t, "in_progress", res.Status)
	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, 41, *b.PR, "resume keeps the PR")
	require.Equal(t, store.GateStatusStale, b.Gates["build"].Status, "resume stales the gates over changed code")
}

// TestResumeWithUnreadableCodeStalesEverything pins that resume never fails on
// the code identity it only reads to keep gates: an untracked file it cannot
// read leaves no identity, so the resume succeeds and stales every gate, as
// it did before gate reuse.
func TestResumeWithUnreadableCodeStalesEverything(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	for _, noun := range executionUnitNouns {
		t.Run(noun, func(t *testing.T) {
			u := layToggledUnit(t, noun, "")
			u.green(t)
			require.True(t, u.check(t).Passed)
			require.NoError(t, u.submit(t))
			secret := filepath.Join(u.root, "secret.txt")
			require.NoError(t, os.WriteFile(secret, []byte("x\n"), 0o644))
			require.NoError(t, os.Chmod(secret, 0o000))

			require.NoError(t, tryTransition(t, u.root, testDeps(), u.noun, u.ref, mutate.TransitionInput{Verb: "resume", Reason: "changes requested"}))
			require.Equal(t, "in_progress", u.node(t).NodeStatus())
			for name, g := range u.gates(t) {
				require.Equal(t, store.GateStatusStale, g.Status, name)
			}
		})
	}
}

// TestResumeKeepsGatesOnUnchangedCode pins the resume rule (gate-reuse design,
// decision 9): a resume over the same code identity keeps the passed command
// gates, so the check after it reuses them, and an edit after the resume
// reruns them. It drives real git, because the code identity decides.
func TestResumeKeepsGatesOnUnchangedCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := layStartedBug(t, countingRun(dir, "build"), countingRun(dir, "tests"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644))
	gitCommitAllM(t, root, "lay code")

	tree := load(t, root)
	_, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	gitCommitAllM(t, root, "record check")
	pr := 41
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "submit", PR: &pr})
	gitCommitAllM(t, root, "record submit")
	res := driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "resume", Reason: "changes requested"})
	require.Equal(t, "in_progress", res.Status)

	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, store.GateStatusPass, b.Gates["build"].Status, "same code keeps build")
	require.Equal(t, store.GateStatusPass, b.Gates["tests"].Status, "same code keeps tests")

	tree = load(t, root)
	again, err := mutate.Check(tree, laterDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, again.Passed)
	require.Equal(t, 1, runs(t, dir, "build"), "the check after an unchanged resume reuses build")
	require.Equal(t, 1, runs(t, dir, "tests"), "the check after an unchanged resume reuses tests")

	gitCommitAllM(t, root, "record second check")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "submit", PR: &pr})
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "resume", Reason: "more changes"})
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
	tree = load(t, root)
	_, err = mutate.Check(tree, laterDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.Equal(t, 2, runs(t, dir, "build"), "an edit after the resume reruns build")
	require.Equal(t, 2, runs(t, dir, "tests"), "an edit after the resume reruns tests")
}
