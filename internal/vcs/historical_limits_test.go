package vcs

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoricalOutputCannotBypassLimitThroughCopy(t *testing.T) {
	out := historicalOutput{remaining: 4}
	_, err := io.Copy(&out, strings.NewReader("12345"))
	require.ErrorIs(t, err, errHistoricalOutputLimit)
	require.True(t, out.exceeded)
	require.LessOrEqual(t, out.buffer.Len(), 4)
}

func TestHistoricalCommitReadIsBounded(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "commit-message.txt", strings.Repeat("x", 4*1024*1024+1)))
	run(t, dir, "commit", "--allow-empty", "-F", "commit-message.txt")
	head := run(t, dir, "rev-parse", "HEAD")
	parents, err := g.CommitParents(head)
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	require.Nil(t, parents)
}
