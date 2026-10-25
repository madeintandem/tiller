package execution

import (
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
