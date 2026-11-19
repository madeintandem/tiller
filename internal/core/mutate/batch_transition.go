package mutate

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// This file holds what the shared shelf transition (transitionWorkItem) asks
// when the node is a batch or a batch member.

// guardBatchMembersReady is start batch's and submit batch's own check: every
// member is approved and ready, and each one's pinned contract still matches
// the working tree, the same basis check a shelf node makes for itself.
func guardBatchMembersReady(tree *store.Tree, rev vcs.Revisions, batch *store.Batch) error {
	for _, member := range tree.BatchMembers(batch.ID) {
		if member.Body().Status != "ready" {
			return memberUnready(batch, member)
		}
		if err := checkWorkBasis(tree.Root, member, rev); err != nil {
			return err
		}
	}
	return nil
}

// returnAfterBatchStart refuses sending a member back to triage once its batch
// is being built: its fix may already be on the batch branch. Removing it from
// the batch is the deliberate act, and it says what happens to those commits.
func returnAfterBatchStart(member store.WorkNode, batch *store.Batch) error {
	b := member.Body()
	seam := fmt.Sprintf("tiller batch %s --remove %s", batchAddr(batch), b.ID)
	return &BatchError{
		Reason: ReasonBatchStarted, Noun: member.Noun(), NodeID: b.ID, Key: deref(b.Key), CurrentState: b.Status,
		Batch: batch.ID, BatchKey: deref(batch.Key), Seam: seam,
		msg: fmt.Sprintf("%s %s is in batch %s, which has started; take it out with `%s` before returning it to triage",
			member.Noun(), b.ID, batch.ID, seam),
	}
}

// releaseBatchMembers plans each member's release when its batch is
// cancelled: the pointer clears and the stored status stays, so every member
// is back in its own queue. The writes carry no event; the batch's own cancel
// event names the released members and their files.
func releaseBatchMembers(tree *store.Tree, batch *store.Batch) ([]nodeWrite, []string, error) {
	members := tree.BatchMembers(batch.ID)
	writes := make([]nodeWrite, 0, len(members))
	for _, member := range members {
		doc, rel, snapshot, err := loadWorkItemFile(tree, member)
		if err != nil {
			return nil, nil, err
		}
		doc.Body().BatchID = ""
		data, err := yaml.Marshal(doc)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, nodeWrite{rel: rel, data: data, before: snapshot})
	}
	return writes, memberIDs(batchMembersOf(members)), nil
}

// memberLeavesBatch plans what a cancelled member's departure does to its
// batch: nothing while other members remain and the gates are not yet
// recorded, stale gates at check, and cancellation when it was the last one.
// A batch in review refuses, like every other membership change.
func memberLeavesBatch(tree *store.Tree, deps Deps, owner store.Contributor, member store.WorkNode, treeBatch *store.Batch) ([]nodeWrite, error) {
	b := member.Body()
	if err := batchEditable(treeBatch, member.Noun(), b.ID, deref(b.Key), b.Status); err != nil {
		return nil, err
	}
	doc, rel, snapshot, err := loadWorkItemFile(tree, treeBatch)
	if err != nil {
		return nil, err
	}
	batch := doc.(*store.Batch)
	before := batch.Status
	after := map[string]any{"removed": []string{b.ID}}
	last := len(tree.BatchMembers(batch.ID)) == 1
	changed, err := batchAfterMembershipChange(batch, last, after)
	if err != nil || !changed {
		return nil, err
	}
	data, err := yaml.Marshal(batch)
	if err != nil {
		return nil, err
	}
	ev := events.NewEvent(deps.clock(), "edit."+store.NounBatch, owner.ID, "cancel "+member.Noun()+" "+b.ID, batch.ID, b.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"status": before}
	after["status"] = batch.Status
	ev.After = after
	ev.Files = []events.FileChange{{Path: rel}}
	return []nodeWrite{{rel: rel, data: data, before: snapshot, event: ev}}, nil
}

// guardBatchOwnership keeps a batch to one owner. A member of a live batch
// takes no claim of its own, and a batch refuses an owner while one of its
// members is claimed by someone else, since that claim predates the batch and
// only its holder can release it.
func guardBatchOwnership(tree *store.Tree, node store.WorkNode, holder, verb string) error {
	batch, ok := node.(*store.Batch)
	if !ok {
		return refuseBatched(tree, node, verb)
	}
	for _, member := range tree.BatchMembers(batch.ID) {
		b := member.Body()
		if b.Owner != "" && b.Owner != holder {
			seam := fmt.Sprintf("tiller release %s %s", member.Noun(), deref(b.Key))
			return &BatchError{
				Reason: ReasonBatchMemberClaimed, Noun: store.NounBatch, NodeID: batch.ID, Key: deref(batch.Key), CurrentState: batch.Status,
				Batch: batch.ID, BatchKey: deref(batch.Key), Member: b.ID, Seam: seam,
				msg: fmt.Sprintf("batch %s cannot go to %s while %s holds its member %s %s; %s releases it (`%s`), or it leaves the batch",
					batch.ID, holder, b.Owner, member.Noun(), b.ID, b.Owner, seam),
			}
		}
	}
	return nil
}
