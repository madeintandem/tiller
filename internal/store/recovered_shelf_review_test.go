package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const recoveredBugYAML = `id: B-aaaaaa
key: bugs/alpha
status: ready
blocked: false
started: false
pr: 13
recovered_review:
  head: {kind: git_commit, value: "git:1111111111111111111111111111111111111111"}
  merge_commit: {kind: git_commit, value: "git:2222222222222222222222222222222222222222"}
  integration_branch: main
  basis: ancestry
  recovered_by: jc
  recovered_at: 2026-11-19T12:00:00Z
  reason: merged before submit
`

func TestShelfRecoveredReviewRoundTripsWithoutABranch(t *testing.T) {
	var bug Bug
	require.NoError(t, yaml.Unmarshal([]byte(recoveredBugYAML), &bug))
	require.NotNil(t, bug.RecoveredReview)
	require.Empty(t, bug.RecoveredReview.Branch, "a node recovered before it ever started records no branch")
	require.Equal(t, "ancestry", bug.RecoveredReview.Basis)

	out, err := yaml.Marshal(&bug)
	require.NoError(t, err)
	var again Bug
	require.NoError(t, yaml.Unmarshal(out, &again))
	require.Equal(t, bug.RecoveredReview, again.RecoveredReview)

	review := bug.RecoveredReview.Review()
	require.Equal(t, bug.RecoveredReview.Head, review.Head)
	require.Empty(t, review.Tasks)
	require.Empty(t, review.PolicySnapshots)
}

func TestShelfRecoveredReviewRefusesPartialOrUnknownRecords(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"missing reason": func(s string) string { return strings.Replace(s, "  reason: merged before submit\n", "", 1) },
		"unknown field":  func(s string) string { return s + "  tasks: []\n" },
		"abbreviated commit": func(s string) string {
			return strings.Replace(s, "git:1111111111111111111111111111111111111111", "git:1111111", 1)
		},
		"unsupported basis": func(s string) string { return strings.Replace(s, "basis: ancestry", "basis: rollup", 1) },
		"null record":       func(s string) string { return s[:strings.Index(s, "recovered_review:")] + "recovered_review: null\n" },
	} {
		t.Run(name, func(t *testing.T) {
			var bug Bug
			err := yaml.Unmarshal([]byte(edit(recoveredBugYAML)), &bug)
			require.ErrorContains(t, err, "recovered_review")
		})
	}
}
