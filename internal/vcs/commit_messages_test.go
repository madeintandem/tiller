package vcs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func commitFile(t *testing.T, dir, rel, content, subject string, body ...string) string {
	t.Helper()
	require.NoError(t, writeRepoFile(dir, rel, content))
	run(t, dir, "add", "-A")
	args := []string{"commit", "-m", subject}
	for _, paragraph := range body {
		args = append(args, "-m", paragraph)
	}
	run(t, dir, args...)
	return run(t, dir, "rev-parse", "HEAD")
}

func TestCodeCommitMessagesSkipsCommitsThatOnlyRecordTillerState(t *testing.T) {
	commitReadsRepos(t, func(t *testing.T, dir, prefix string, g *Git) {
		code := commitFile(t, dir, prefix+"src/fix.go", "package src\n", "B-aaaaaa Fix the thing", "B-bbbbbb B-cccccc")
		commitFile(t, dir, prefix+".tiller/bugs/alpha/bug.yml", "status: ready\n", "B-aaaaaa start")
		mixed := commitFile(t, dir, prefix+".tiller/bugs/alpha/bug.yml", "status: in_progress\n", "state only so far")
		require.NoError(t, writeRepoFile(dir, prefix+"artifacts/tiller/gates/build-out.txt", "ok\n"))
		require.NoError(t, writeRepoFile(dir, prefix+".tiller/bugs/alpha/bug.yml", "status: check\n"))
		run(t, dir, "add", "-A", "-f")
		run(t, dir, "commit", "-m", "B-aaaaaa record gate evidence")
		commitFile(t, dir, prefix+"artifacts/report.txt", "r\n", "an artifact that is not Tiller's")
		require.NoError(t, writeRepoFile(dir, prefix+"src/more.go", "package src\n"))
		require.NoError(t, writeRepoFile(dir, prefix+".tiller/bugs/alpha/bug.yml", "status: in_review\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "code and state together")
		both := run(t, dir, "rev-parse", "HEAD")
		require.NotEqual(t, mixed, both)

		got, err := g.CodeCommitMessages("main", 50)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(got), 3)
		require.Equal(t, both, got[0].Commit, "newest first; a commit touching both sides counts")
		require.Equal(t, "code and state together", got[0].Subject)
		require.Equal(t, "an artifact that is not Tiller's", got[1].Subject, "only artifacts/tiller/ is Tiller's")
		require.Equal(t, code, got[2].Commit)
		require.Equal(t, "B-aaaaaa Fix the thing", got[2].Subject)
		require.Contains(t, got[2].Message, "B-bbbbbb B-cccccc", "the body rides with the subject")
		for _, c := range got {
			require.NotEqual(t, "B-aaaaaa start", c.Subject)
			require.NotEqual(t, "state only so far", c.Subject)
			require.NotEqual(t, "B-aaaaaa record gate evidence", c.Subject)
		}
	})
}

func TestCodeCommitMessagesReachesCommitsBehindAMerge(t *testing.T) {
	dir, g := newRepo(t)
	run(t, dir, "checkout", "-b", "topic")
	side := commitFile(t, dir, "src/side.go", "package src\n", "TW-aaaaaa side work")
	run(t, dir, "checkout", "main")
	commitFile(t, dir, "src/main.go", "package src\n", "mainline work")
	run(t, dir, "merge", "--no-ff", "-m", "Merge topic", "topic")

	got, err := g.CodeCommitMessages("main", 50)
	require.NoError(t, err)
	var commits []string
	for _, c := range got {
		commits = append(commits, c.Commit)
	}
	require.Contains(t, commits, side, "a commit on the merged side is part of what landed")

	onTopicOnly, err := g.CodeCommitMessages("topic", 50)
	require.NoError(t, err)
	for _, c := range onTopicOnly {
		require.NotEqual(t, "mainline work", c.Subject, "only commits reachable from ref are read")
	}
}

func TestCodeCommitMessagesHonoursTheLimit(t *testing.T) {
	dir, g := newRepo(t)
	commitFile(t, dir, "a.txt", "a\n", "first")
	newest := commitFile(t, dir, "b.txt", "b\n", "second")

	limited, err := g.CodeCommitMessages("main", 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, newest, limited[0].Commit)

	none, err := g.CodeCommitMessages("main", 0)
	require.NoError(t, err)
	require.Empty(t, none, "a zero bound scans nothing")
}

func TestCodeCommitMessagesUnknownRefIsAnError(t *testing.T) {
	_, g := newRepo(t)
	_, err := g.CodeCommitMessages("refs/heads/absent", 5)
	require.Error(t, err)
}
