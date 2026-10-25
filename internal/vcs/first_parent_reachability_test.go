package vcs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirstParentContainsRequiresTargetReachability(t *testing.T) {
	dir, g := newRepo(t)
	root := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "-b", "review")
	require.NoError(t, writeRepoFile(dir, "review.yml", "reviewed\n"))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "reviewed work")
	head := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "main")
	require.NoError(t, writeRepoFile(dir, "integration.yml", "integration\n"))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "integration work")
	trunk := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "merge", "--no-ff", "review", "-m", "integration result")
	result := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "commit", "--allow-empty", "-m", "future work")
	future := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "-b", "unmerged", root)
	require.NoError(t, writeRepoFile(dir, "unmerged.yml", "not integrated\n"))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "unmerged work")
	unmerged := run(t, dir, "rev-parse", "HEAD")

	for _, tc := range []struct {
		name   string
		ref    string
		target string
		want   bool
	}{
		{name: "identical commits", ref: result, target: result, want: true},
		{name: "root on trunk", ref: result, target: root, want: true},
		{name: "first parent", ref: result, target: trunk, want: true},
		{name: "second parent", ref: result, target: head},
		{name: "future target", ref: result, target: future},
		{name: "ref before result", ref: trunk, target: result},
		{name: "root ref before result", ref: root, target: result},
		{name: "unmerged target", ref: result, target: unmerged},
		{name: "missing ref", ref: "refs/heads/absent", target: result},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.FirstParentContains(tc.ref, tc.target)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	// A once-valid result ceases to be on integration after integration's ref
	// moves behind it, even though commit..ref then produces an empty walk.
	run(t, dir, "update-ref", "refs/heads/main", result)
	contains, err := g.FirstParentContains("refs/heads/main", result)
	require.NoError(t, err)
	require.True(t, contains)
	run(t, dir, "update-ref", "refs/heads/main", trunk)
	contains, err = g.FirstParentContains("refs/heads/main", result)
	require.NoError(t, err)
	require.False(t, contains)
}
