package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCreationAndApprovalDoNotAssignExecutionWork(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, ref := layClaimFeature(t, deps)
	first := addTaskNamed(t, root, deps, "first")
	_, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), first, "")
	require.NoError(t, err)
	second := addTaskNamed(t, root, deps, "second")
	writeFeatureProse(t, root)
	for _, key := range []string{"first", "second"} {
		writeTaskSpec(t, root, key, "## Approach\n\nDo the scoped work.")
	}
	driveWith(t, root, deps, store.NounFeature, ref, mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, ref, mutate.TransitionInput{Verb: "approve"})
	tree := load(t, root)
	require.Equal(t, "pat", tree.Tasks[first].Owner, "approval preserves an explicit pre-start reservation")
	require.Empty(t, tree.Tasks[second].Owner, "approval does not inherit its actor or sibling owner")
	require.Equal(t, "jc", tree.Tasks[second].ProposedBy)
	require.Equal(t, "ready", tree.Tasks[second].Status)
	for _, noun := range []string{store.NounBug, store.NounChore} {
		item, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: noun, Key: "independent", Title: "Independent"})
		require.NoError(t, err)
		tree = load(t, root)
		for _, work := range tree.WorkNodesOf(noun) {
			if work.NodeID() != item.ID {
				continue
			}
			require.Empty(t, work.Body().Owner)
			require.Equal(t, "jc", work.Body().ProposedBy)
		}
	}
}
