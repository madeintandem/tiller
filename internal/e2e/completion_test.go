package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompletionGroupThroughTheBinary drives Cobra's generated completion
// group through the compiled binary, where its scripts reach the real stdout:
// a shell name still yields its script, the bare group still prints help, and
// a word that names no shell is misuse (cli-surface.md, "Output and exit
// codes") instead of the help page with exit 0.
func TestCompletionGroupThroughTheBinary(t *testing.T) {
	t.Parallel()
	h := &harness{t: t, bin: buildTiller(t), dir: t.TempDir(), home: t.TempDir()}

	script := h.tiller("completion", "zsh")
	requireOK(t, script, "completion zsh")
	require.Contains(t, script.stdout, "#compdef tiller")
	require.Empty(t, script.stderr)

	bare := h.tiller("completion")
	requireOK(t, bare, "completion")
	require.Contains(t, bare.stdout, "Available Commands:")
	require.Empty(t, bare.stderr)

	unknown := h.tiller("completion", "bogus")
	require.Equal(t, 2, unknown.code)
	require.Empty(t, unknown.stdout)
	require.Equal(t, "tiller: unknown command \"bogus\" for \"tiller completion\"\n", unknown.stderr)

	unknownJSON := h.tiller("completion", "bogus", "--json")
	require.Equal(t, 2, unknownJSON.code)
	require.Empty(t, unknownJSON.stderr)
	dec := json.NewDecoder(strings.NewReader(unknownJSON.stdout))
	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Error   struct {
			Code   int    `json:"code"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	require.NoError(t, dec.Decode(&env), unknownJSON.stdout)
	require.False(t, dec.More(), "stdout holds exactly one envelope: %q", unknownJSON.stdout)
	require.False(t, env.OK)
	require.Equal(t, "completion", env.Command)
	require.Equal(t, 2, env.Error.Code)
	require.Equal(t, "bad_args", env.Error.Reason)
}
