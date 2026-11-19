package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// layReadyBugGit lays an approved, never-started bug with its ledger committed
// on main: the state every bug in the Headway report was in when its fix merged.
func layReadyBugGit(t *testing.T) string {
	t.Helper()
	root := writeGatedRepo(t, "true", "true")
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A", Severity: "medium"})
	require.NoError(t, err)
	writeRel(t, root, ".tiller/bugs/alpha/bug.md", "# A\n\n## Reproduction\n\nx\n")
	gitCommitAllM(t, root, "capture: alpha")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	gitCommitAllM(t, root, "Approve B-aaaaaa")
	return root
}

// landFix commits a code change onto the checked-out branch under msg.
func landFix(t *testing.T, root, rel, msg string) string {
	t.Helper()
	writeRel(t, root, rel, "fix\n")
	commitPaths(t, root, msg, rel)
	return gitOutM(t, root, "rev-parse", "HEAD")
}

func TestSyncWarnsWhenAMergeNamesABugThatNeverStarted(t *testing.T) {
	t.Parallel()
	root := layReadyBugGit(t)
	fix := landFix(t, root, "fix.txt", "B-aaaaaa Preserve Inbox capture order")
	before := gitOutM(t, root, "status", "--porcelain")

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "the warning records no crossing")
	require.Empty(t, res.Awaiting)
	require.Equal(t, []mutate.UnrecordedMerge{{
		Noun: store.NounBug, ID: "B-aaaaaa", Key: "bugs/alpha", Status: "ready",
		Commits: []mutate.NamingCommit{{Commit: fix, Subject: "B-aaaaaa Preserve Inbox capture order"}},
	}}, res.UnrecordedMerges)

	require.Equal(t, "ready", load(t, root).Bugs["B-aaaaaa"].Status, "the warning moves no status")
	require.Equal(t, before, gitOutM(t, root, "status", "--porcelain"), "a pure observation writes nothing, events included")

	again, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, res.UnrecordedMerges, again.UnrecordedMerges, "the finding stands until the node is recorded")
}

func TestSyncIsQuietAboutCommitsThatOnlyRecordTillerState(t *testing.T) {
	t.Parallel()
	root := layReadyBugGit(t)
	// "Approve B-aaaaaa" is already on main; a second ledger-only commit names
	// the id in its subject the way a start or a gate record does.
	writeRel(t, root, ".tiller/bugs/alpha/bug.md", "# A\n\n## Reproduction\n\ny\n")
	commitPaths(t, root, "B-aaaaaa record gate evidence", ".tiller")

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.UnrecordedMerges)
}

func TestSyncWarnsOnAnIDNamedOnlyInACommitBody(t *testing.T) {
	t.Parallel()
	root := layReadyBugGit(t)
	writeRel(t, root, "fix.txt", "fix\n")
	gitRunM(t, root, "add", "--", "fix.txt")
	gitRunM(t, root, "commit", "-m", "Fix the inbox sweep", "-m", "B-67kx27 B-aaaaaa B-timrgi")

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.UnrecordedMerges, 1)
	require.Equal(t, "Fix the inbox sweep", res.UnrecordedMerges[0].Commits[0].Subject)
}

// A started bug whose branch merged before check and submit ran: the merge
// commit and the fix behind it are both on main, the node is still in_progress.
func TestSyncWarnsWhenAStartedBugMergesUnsubmitted(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_progress", "check"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			root := layStartedBug(t, "true", "true")
			gitCommitAllM(t, root, "B-aaaaaa start")
			if status == "check" {
				tree := load(t, root)
				checked, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
				require.NoError(t, err)
				require.True(t, checked.Passed)
				gitCommitAllM(t, root, "B-aaaaaa record gate evidence")
			}
			fix := landFix(t, root, "fix.txt", "B-aaaaaa fix the bug")

			onItsBranch, err := mutate.ObserveUnrecordedShelfMerges(load(t, root), vcs.New(root))
			require.NoError(t, err)
			require.Empty(t, onItsBranch, "work on the node's own branch has not landed anywhere")

			gitRunM(t, root, "checkout", "main")
			gitRunM(t, root, "merge", "--no-ff", "-m", "Merge pull request #13", bugBranch)
			res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings)
			require.Len(t, res.UnrecordedMerges, 1)
			require.Equal(t, status, res.UnrecordedMerges[0].Status)
			require.Equal(t, []mutate.NamingCommit{{Commit: fix, Subject: "B-aaaaaa fix the bug"}}, res.UnrecordedMerges[0].Commits)
			require.Equal(t, status, load(t, root).Bugs["B-aaaaaa"].Status)

			observed, err := mutate.ObserveUnrecordedShelfMerges(load(t, root), vcs.New(root))
			require.NoError(t, err)
			require.Equal(t, res.UnrecordedMerges, observed, "a later read derives the same finding sync reported")
		})
	}
}

func TestSyncStopsWarningOnceTheBugIsSubmittedDoneOrCancelled(t *testing.T) {
	t.Parallel()
	t.Run("in_review then done", func(t *testing.T) {
		t.Parallel()
		root := layReviewedBugGit(t)
		landFix(t, root, "hotfix.txt", "B-aaaaaa hotfix straight to main")
		res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
		require.NoError(t, err)
		require.Len(t, res.Awaiting, 1, "an in_review node is sync's ordinary awaiting entry")
		require.Empty(t, res.UnrecordedMerges)

		gitRunM(t, root, "merge", "--no-ff", "-m", "merge fix", bugBranch)
		res, err = mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
		require.NoError(t, err)
		require.Len(t, res.Crossings, 1)
		require.Empty(t, res.UnrecordedMerges)
	})
	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		root := layReadyBugGit(t)
		landFix(t, root, "fix.txt", "B-aaaaaa fix")
		driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
		res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
		require.NoError(t, err)
		require.Empty(t, res.UnrecordedMerges)
	})
}

func TestObserveUnrecordedShelfMergesReadsTheDeclaredIntegrationBranch(t *testing.T) {
	t.Parallel()
	root := layReadyBugGit(t)
	landFix(t, root, "fix.txt", "B-aaaaaa fix")
	setIntegrationBranch(t, root, "main")
	gitRunM(t, root, "checkout", "-b", "scratch")

	got, err := mutate.ObserveUnrecordedShelfMerges(load(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Len(t, got, 1, "the declared branch is read wherever the checkout stands")
}
