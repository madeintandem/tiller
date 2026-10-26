package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApprovalRejectsUnsupportedReviewGrainBeforeApplyingAmendment(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "reshape", "Reshape")
	taskKey := "form-engine/certification-form-loading/schema-transcription"
	_, stderr, code := execCLI(t, "amend", aid, "--cancel", taskKey, "--reason", "No longer needed", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	before := loadCwdTree(t)
	for _, grain := range []string{"definitely-invalid", "task", ""} {
		out, errout, code := execCLI(t, "approve", "amendment", aid, "--review-grain", grain, "--contributor", "jc", "--json")
		require.Equal(t, 2, code, out+errout)
		after := loadCwdTree(t)
		require.Equal(t, before.Amendments[aid], after.Amendments[aid])
		require.Equal(t, before.Tasks, after.Tasks)
	}
}

func TestApprovalRejectsUnsupportedShelfFlagsWithoutMutation(t *testing.T) {
	initOwnedRepo(t)
	for _, noun := range []string{"bug", "chore", "tweak"} {
		_, stderr, code := execCLI(t, "add", noun, "untouched", "--title", "Untouched", "--contributor", "jc")
		require.Equal(t, 0, code, stderr)
		before := loadCwdTree(t)
		for _, flags := range [][]string{
			{"--review-grain", "definitely-invalid"},
			{"--review-grain", "task"},
			{"--review-grain", ""},
			{"--feature", ""},
			{"--feature", "   "},
		} {
			args := append([]string{"approve", noun, "untouched"}, flags...)
			args = append(args, "--contributor", "jc", "--json")
			out, errout, code := execCLI(t, args...)
			require.Equal(t, 2, code, out+errout)
			after := loadCwdTree(t)
			require.Equal(t, before.Bugs, after.Bugs)
			require.Equal(t, before.Chores, after.Chores)
			require.Equal(t, before.Tweaks, after.Tweaks)
		}
	}
}

func TestApprovalRejectsEmptyFeatureSelectorWithPositionalTask(t *testing.T) {
	layAmendmentTree(t)
	before := loadCwdTree(t)
	for _, selector := range []string{"", "   "} {
		out, errout, code := execCLI(t, "approve", "task", "form-engine/certification-form-loading/schema-transcription", "--feature", selector, "--contributor", "jc", "--json")
		require.Equal(t, 2, code, out+errout)
		require.Equal(t, before.Tasks, loadCwdTree(t).Tasks)
	}
}

func TestPluralAmendmentShowRetainsFullReviewBatch(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "reshape", "Reshape")
	_, stderr, code := execCLI(t, "amend", aid, "--cancel", "form-engine/certification-form-loading/schema-transcription", "--reason", "No longer needed", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	for _, format := range [][]string{nil, {"--json"}} {
		singularArgs := append([]string{"show", "amendment", aid}, format...)
		pluralArgs := append([]string{"show", "amendments", aid}, format...)
		singular, stderr, code := execCLI(t, singularArgs...)
		require.Equal(t, 0, code, stderr)
		plural, stderr, code := execCLI(t, pluralArgs...)
		require.Equal(t, 0, code, stderr)
		require.Equal(t, singular, plural)
		require.Contains(t, plural, "cancel")
	}
}
