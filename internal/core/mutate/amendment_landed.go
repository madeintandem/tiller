package mutate

import (
	"slices"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// This file is the recovery half of approve amendment for the three operations
// that are not safe to repeat: cancel, correct-eval, and move-feature. An
// approval cut short before the amendment's own status write (the last write
// in the transaction, amendment_approve.go's header comment) leaves them
// landed under an amendment still reading proposed, and applying them a second
// time refuses. So a re-approve recognises each one it already applied and
// skips it, reporting AppliedAlready rather than work this run did not do.
//
// Two facts together are the evidence, never one alone. The node is in the
// state the operation leaves it in, and the event log holds the event the
// apply recorded for that operation: its type, this amendment's id, the
// operation's index, and the node as a target. A node in the landed state with
// no such event got there some other way (a human's cancel, another amendment,
// a hand edit), and the operation refuses exactly as it did before.

// findLanded records on the batch every operation an earlier approval of this
// amendment already applied, and whether that approval got as far as recording
// its own amendment_approved event. It writes nothing, and it reads the event
// log only when some operation's node is already in its landed state, so an
// ordinary first approval pays nothing for it.
func (b *batch) findLanded(tree *store.Tree, doc *store.Amendment, plan []resolvedOp) error {
	var history []events.Event
	for _, r := range plan {
		eventType, nodeID, err := b.landedState(tree, r)
		if err != nil {
			return err
		}
		if eventType == "" {
			continue
		}
		if history == nil {
			if history, err = events.List(tree); err != nil {
				return err
			}
		}
		if !slices.ContainsFunc(history, func(ev events.Event) bool {
			return ev.Type == eventType && ev.After["amendment"] == doc.ID &&
				eventOperationIndex(ev) == r.index && slices.Contains(ev.Targets, nodeID)
		}) {
			continue
		}
		if b.landed == nil {
			b.landed = map[int]bool{}
		}
		b.landed[r.index] = true
	}
	if len(b.landed) > 0 {
		b.approvalRecorded = slices.ContainsFunc(history, func(ev events.Event) bool {
			return ev.Type == eventAmendmentApproved && ev.After["amendment"] == doc.ID
		})
	}
	return nil
}

// landedState reports whether the operation's node is already in the state the
// operation leaves it in, as the type of the event its apply records and the
// node that event targets. A blank type means it is not, or that the
// operation is one of the kinds that are safe to repeat.
func (b *batch) landedState(tree *store.Tree, r resolvedOp) (eventType, nodeID string, err error) {
	switch r.op.Op {
	case store.AmendmentOpCancel:
		e, err := b.editFor(tree, r.target)
		if err != nil {
			return "", "", err
		}
		if e.doc.NodeStatus() == statusCancelled {
			return eventCancelPrefix + e.noun, e.doc.NodeID(), nil
		}
	case store.AmendmentOpCorrectEval:
		// The corrected pin is the eval_revision staging recorded: the blob the
		// human reviewed, and the value the apply writes.
		pin, pinned := r.op.Preconditions[preconditionEvalRevision]
		if !pinned {
			return "", "", nil
		}
		e, err := b.editFor(tree, r.target)
		if err != nil {
			return "", "", err
		}
		if feature, ok := e.doc.(*store.Feature); ok && feature.VerifiedAgainst.Eval.Value == pin {
			return eventCorrectEvalPrefix + e.noun, feature.ID, nil
		}
	case store.AmendmentOpMoveFeature:
		if feature := tree.Features[r.op.Target]; feature != nil && deref(feature.Key) == r.op.Key {
			return eventMoveFeature, feature.ID, nil
		}
	}
	return "", "", nil
}

// eventOperationIndex reads the operation index an applied operation's event
// carries, or -1 when it carries none. A loose event file decodes it as an
// int and a compacted segment may decode it as a float64.
func eventOperationIndex(ev events.Event) int {
	switch v := ev.After["operation_index"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return -1
}

// alreadyApplied is the report for an operation findLanded recognised: the
// node as it stands, and AppliedAlready in place of a claim of work done.
func alreadyApplied(r resolvedOp, n store.Node) AppliedOperation {
	return AppliedOperation{
		Index:   r.index,
		Op:      r.op.Op,
		Noun:    n.Noun(),
		NodeID:  n.NodeID(),
		Key:     deref(n.NodeKey()),
		Status:  n.NodeStatus(),
		Outcome: AppliedAlready,
	}
}
