package mutate

import (
	"fmt"
	"maps"

	"github.com/madeintandem/tiller/internal/store"
)

// stageCorrectEval builds a correct-eval operation: a reviewed correction of a
// done feature's eval bindings. The target must be a feature, the one grain
// that carries an acceptance contract. Whether it is done is approval's
// question, not staging's, the same rule revise and supersede follow.
//
// The operation always carries an eval_revision precondition. When the caller
// supplied none, staging pins the working-tree feature.eval.md as it stands
// now, the same hash approval compares against (basisFileFor and
// WorktreeBlobRef), so approval proves the bytes the human reviewed are the
// bytes it applies. A caller-supplied eval_revision is kept as given.
func stageCorrectEval(tree *store.Tree, deps Deps, in StageInput) (store.AmendmentOp, error) {
	if in.By != "" {
		return store.AmendmentOp{}, supersedeByMisplacedErr()
	}
	if in.SupersededBy != "" {
		return store.AmendmentOp{}, supersededByMisplacedErr()
	}

	node, isEpic := classifyStageTarget(tree, in.Target)
	if isEpic {
		return store.AmendmentOp{}, epicTargetErr()
	}
	if node == nil {
		return store.AmendmentOp{}, unknownTargetErr(in.Target)
	}
	feature, ok := node.(*store.Feature)
	if !ok {
		return store.AmendmentOp{}, correctEvalNotFeatureErr("", node)
	}

	preconditions := maps.Clone(in.Preconditions)
	if _, pinned := preconditions[preconditionEvalRevision]; !pinned {
		pin, err := pinEvalRevision(tree, deps, feature)
		if err != nil {
			return store.AmendmentOp{}, err
		}
		if preconditions == nil {
			preconditions = map[string]string{}
		}
		preconditions[preconditionEvalRevision] = pin
	}

	return store.AmendmentOp{
		Op:            store.AmendmentOpCorrectEval,
		Target:        feature.ID,
		Reason:        in.Reason,
		Retire:        trimmedIDs(in.Retire),
		Source:        in.Source,
		Preconditions: preconditions,
	}, nil
}

// pinEvalRevision hashes the feature's working-tree eval contract in the value
// format an eval_revision precondition is compared in. A feature with no eval
// file has no contract to correct, which is a plain fact about the tree and is
// refused by name rather than pinned to nothing.
func pinEvalRevision(tree *store.Tree, deps Deps, feature *store.Feature) (string, error) {
	rel, ok := basisFileFor(tree, feature, preconditionEvalRevision)
	if !ok {
		return "", unknownTargetErr(feature.ID)
	}
	ref, err := WorktreeBlobRef(deps.rev(tree.Root), tree.Root, rel)
	if err != nil {
		return "", err
	}
	if ref.Value == "" {
		return "", &usageError{
			reason: "correct_eval_no_contract",
			msg: fmt.Sprintf("correct-eval targets feature %s, which has no %s in the working tree; there is no contract to correct",
				feature.ID, rel),
		}
	}
	return ref.Value, nil
}

func retireMisplacedErr() error {
	return &usageError{reason: "retire_misplaced", msg: "--retire only names a correct-eval operation's removed criteria"}
}
