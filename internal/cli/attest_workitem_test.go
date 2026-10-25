package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttestBugGateAtTheCLI(t *testing.T) {
	initOwnedRepo(t)
	_, stderr, code := execCLI(t, "add", "bug", "blank-on-reload", "--title", "Blank on reload", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, stderr, code := execCLI(t, "attest", "bug", "bugs/blank-on-reload", "--gate", "code_review", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, `"noun": "bug"`)
	require.Contains(t, out, `"target_kind": "gate"`)
	require.Contains(t, out, `"satisfied": true`)
}

func TestAttestBugCriterionRefusesAtTheCLI(t *testing.T) {
	initOwnedRepo(t)
	_, stderr, code := execCLI(t, "add", "bug", "blank-on-reload", "--title", "Blank on reload", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, _, code := execCLI(t, "attest", "bug", "bugs/blank-on-reload", "--criterion", "C1", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "unsupported_noun"`)
	require.Contains(t, out, "no eval contract")
}
