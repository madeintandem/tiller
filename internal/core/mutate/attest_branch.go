package mutate

import (
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// guardAttestBranch refuses a feature attest run off the coarse aggregate
// branch that carries the feature's acceptance state. Attest
// writes the feature's own files, and those only travel with the review when
// they are committed on the aggregate; an attest on integration would leave
// state that never reaches the PR. It guards only a feature at feature or epic
// grain whose aggregate is recorded and still open: task grain, an unrecorded
// aggregate, a closed round, and a recovered review all pass.
func guardAttestBranch(tree *store.Tree, rev vcs.Revisions, feature *store.Feature) error {
	branch, _, id := aggregateForTask(tree, feature)
	if branch == "" || aggregateRoundDone(tree, id) || aggregateOwnerCancelled(tree, id) || aggregateRecovery(tree, id) != nil {
		return nil
	}
	current, err := rev.Branch()
	if err != nil {
		return err
	}
	if current == branch {
		return nil
	}
	refusal := &BranchSafetyError{Reason: "attest_branch_mismatch", Noun: store.NounFeature, NodeID: feature.ID, Branch: branch, CurrentBranch: current}
	if finder, ok := rev.(interface {
		WorktreeFor(string) (string, bool, error)
	}); ok {
		path, held, err := finder.WorktreeFor(branch)
		if err != nil {
			return err
		}
		if held {
			refusal.Worktree = path
			return refusal
		}
	}
	exists, err := rev.RefExists(vcs.BranchRef(branch))
	if err != nil {
		return err
	}
	refusal.RefMissing = !exists
	return refusal
}

// aggregateOwnerCancelled reports a cancelled aggregate owner; aggregateRoundDone
// already covers done (and an archived epic).
func aggregateOwnerCancelled(tree *store.Tree, id string) bool {
	if feature := tree.Features[id]; feature != nil {
		return feature.Status == statusCancelled
	}
	return false
}
