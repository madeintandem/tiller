package execution

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitReadAtMost holds its byte cap on its own, without trusting an earlier
// size check: output past the limit is refused, never buffered whole.
func TestGitReadAtMostRefusesOutputPastTheLimit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", root).Run())
	hash := exec.Command("git", "hash-object", "-w", "--stdin")
	hash.Dir = root
	hash.Stdin = strings.NewReader("0123456789")
	out, err := hash.Output()
	require.NoError(t, err)
	oid := strings.TrimSpace(string(out))

	data, err := gitReadAtMost(root, 10, "cat-file", "blob", oid)
	require.NoError(t, err)
	require.Equal(t, "0123456789", string(data))

	_, err = gitReadAtMost(root, 9, "cat-file", "blob", oid)
	require.ErrorIs(t, err, ErrTooLarge)
}

// gitBatchBlobs answers every requested blob, repeated ids included, and
// refuses the whole read for a missing object, a non-blob, or a blob over its
// size bound.
func TestGitBatchBlobsReadsBlobsAndRefusesUnreadableObjects(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", root).Run())
	write := func(content string) string {
		hash := exec.Command("git", "hash-object", "-w", "--stdin")
		hash.Dir = root
		hash.Stdin = strings.NewReader(content)
		out, err := hash.Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	first, second, empty := write("first\n"), write("second, no newline"), write("")
	blobs, err := gitBatchBlobs(context.Background(), root, []string{first, second, first, empty})
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{first: []byte("first\n"), second: []byte("second, no newline"), empty: {}}, blobs)

	none, err := gitBatchBlobs(context.Background(), root, nil)
	require.NoError(t, err)
	require.Empty(t, none)

	_, err = gitBatchBlobs(context.Background(), root, []string{first, strings.Repeat("0", len(first))})
	require.ErrorContains(t, err, "unavailable")

	tree := exec.Command("git", "write-tree")
	tree.Dir = root
	out, err := tree.Output()
	require.NoError(t, err)
	_, err = gitBatchBlobs(context.Background(), root, []string{strings.TrimSpace(string(out))})
	require.ErrorContains(t, err, "not a blob")

	huge := write(strings.Repeat("a", maxBatchBlobBytes+1))
	_, err = gitBatchBlobs(context.Background(), root, []string{first, huge})
	require.ErrorIs(t, err, ErrTooLarge)
}
