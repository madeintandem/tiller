package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func layTwoOwnerFeature(t *testing.T) {
	t.Helper()
	layReadTree(t)
	out, _, code := execCLI(t, "add", "contributor", "pat", "--name", "Pat Nguyen")
	require.Equal(t, 0, code, out)
}

func layTaskReadyToStart(t *testing.T) string {
	t.Helper()
	layTwoOwnerFeature(t)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	mustRun(t, "approve", "epic", "form-engine", "--contributor", "jc")
	mustRun(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	mustRun(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	return soleTaskID(t)
}

func TestTaskClaimCommandsRespectReservationAndPermanentBoundary(t *testing.T) {
	id := layTaskReadyToStart(t)
	out, _, code := execCLI(t, "start", "task", id, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "work_unclaimed", jsonData(t, out)["reason"])
	mustRun(t, "claim", "task", id, "--contributor", "pat")
	out, _, code = execCLI(t, "claim", "task", id, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "pat", jsonData(t, out)["holder"])
	out, _, code = execCLI(t, "claim", "task", id, "--contributor", "jc", "--from", "pat", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, true, jsonData(t, out)["takeover"])
	mustRun(t, "release", "task", id, "--contributor", "jc")
	require.Empty(t, loadCwdTree(t).Tasks[id].Owner)
	mustRun(t, "reassign", "task", id, "--to", "pat", "--reason", "human handoff", "--contributor", "jc")
	mustRun(t, "start", "task", id, "--contributor", "pat")
	tree := loadCwdTree(t)
	require.True(t, tree.Tasks[id].Started)
	require.Equal(t, "active", tree.Features[soleFeatureID(t)].Status)
	for _, args := range [][]string{
		{"claim", "task", id, "--from", "pat", "--contributor", "jc", "--json"},
		{"release", "task", id, "--contributor", "pat", "--json"},
		{"reassign", "task", id, "--to", "jc", "--reason", "cannot relabel", "--contributor", "pat", "--json"},
	} {
		out, _, code = execCLI(t, args...)
		require.Equal(t, 1, code, out)
		require.Equal(t, "owner_immutable", jsonData(t, out)["reason"])
		require.Equal(t, "pat", loadCwdTree(t).Tasks[id].Owner)
	}
	out, _, code = execCLI(t, "claim", "task", id, "--contributor", "pat", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, false, jsonData(t, out)["changed"])
}

func TestFeatureClaimAndStartAreRetired(t *testing.T) {
	layTwoOwnerFeature(t)
	for _, verb := range []string{"claim", "release", "reassign", "start"} {
		_, _, code := execCLI(t, verb, "feature", cliFeatureKey, "--contributor", "jc")
		require.NotEqual(t, 0, code, verb+" feature must not mutate")
	}
	require.Equal(t, "draft", loadCwdTree(t).Features[soleFeatureID(t)].Status)
}

func TestShelfClaimCommandsFreezeAtStart(t *testing.T) {
	for _, noun := range []string{"bug", "chore", "tweak"} {
		t.Run(noun, func(t *testing.T) {
			initOwnedRepo(t)
			mustRun(t, "add", "contributor", "pat", "--name", "Pat")
			mustRun(t, "add", noun, "work", "--title", "Work", "--contributor", "jc")
			mustRun(t, "claim", noun, "work", "--contributor", "jc")
			mustRun(t, "reassign", noun, "work", "--to", "pat", "--reason", "human handoff", "--contributor", "jc")
			mustRun(t, "approve", noun, "work", "--contributor", "jc")
			mustRun(t, "start", noun, "work", "--contributor", "pat")
			out, _, code := execCLI(t, "release", noun, "work", "--contributor", "pat", "--json")
			require.Equal(t, 1, code, out)
			require.Equal(t, "owner_immutable", jsonData(t, out)["reason"])
		})
	}
}

func TestTweakPluralOwnershipCommands(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "contributor", "pat", "--name", "Pat")
	mustRun(t, "add", "tweaks", "spacing", "--title", "Spacing", "--contributor", "jc")
	mustRun(t, "claim", "tweaks", "spacing", "--contributor", "jc")
	mustRun(t, "release", "tweaks", "spacing", "--contributor", "jc")
	mustRun(t, "claim", "tweaks", "spacing", "--contributor", "jc")
	mustRun(t, "reassign", "tweaks", "spacing", "--to", "pat", "--reason", "human handoff", "--contributor", "jc")
	for _, item := range loadCwdTree(t).Tweaks {
		require.Equal(t, "pat", item.Owner)
	}
}

func TestTweakPluralLifecycleAndReads(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "tweaks", "spacing", "--title", "Spacing", "--contributor", "jc")
	mustRun(t, "claim", "tweaks", "spacing", "--contributor", "jc")
	mustRun(t, "approve", "tweaks", "spacing", "--contributor", "jc")
	mustRun(t, "start", "tweaks", "spacing", "--contributor", "jc")
	for _, verb := range []string{"show", "status"} {
		out, stderr, code := execCLI(t, verb, "tweaks", "spacing", "--json")
		require.Equal(t, 0, code, stderr)
		require.Contains(t, out, "in_progress")
	}
	// Both calls must reach lifecycle/gate evaluation, never noun misuse.
	for _, verb := range []string{"check", "submit"} {
		args := []string{verb, "tweaks", "spacing", "--contributor", "jc", "--json"}
		if verb == "submit" {
			args = append(args, "--pr", "1")
		}
		out, _, code := execCLI(t, args...)
		require.NotEqual(t, 2, code, out)
		require.NotContains(t, out, "unknown_noun")
		require.NotContains(t, out, "noun_mismatch")
	}
}
