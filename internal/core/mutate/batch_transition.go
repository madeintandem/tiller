package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

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
// is back in its own queue. Each write carries the member's own batch.<noun>
// event; the batch's cancel event names the released members in
// after.released without targeting them.
func releaseBatchMembers(tree *store.Tree, deps Deps, owner store.Contributor, batch *store.Batch) ([]nodeWrite, []string, error) {
	members := tree.BatchMembers(batch.ID)
	command := "cancel batch " + batchAddr(batch)
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
		ev := memberBatchEvent(deps, owner, member, batch.ID, "", command)
		ev.Files = []events.FileChange{{Path: rel}}
		writes = append(writes, nodeWrite{rel: rel, data: data, before: snapshot, event: ev})
	}
	return writes, memberIDs(batchMembersOf(members)), nil
}

// memberLeavesBatch plans what a cancelled member's departure records on its
// batch: an edit.batch naming the member, with stale gates when the batch was
// at check, or a cancel.batch when it was the last one. A batch in review
// refuses, like every other membership change. The batch file is written only
// when the record itself moved.
func memberLeavesBatch(tree *store.Tree, deps Deps, owner store.Contributor, member store.WorkNode, treeBatch *store.Batch) ([]nodeWrite, []events.Event, error) {
	b := member.Body()
	if err := batchEditable(treeBatch, member.Noun(), b.ID, deref(b.Key), b.Status); err != nil {
		return nil, nil, err
	}
	doc, rel, snapshot, err := loadWorkItemFile(tree, treeBatch)
	if err != nil {
		return nil, nil, err
	}
	batch := doc.(*store.Batch)
	last := len(tree.BatchMembers(batch.ID)) == 1
	ev, changed, err := batchMembershipEvent(deps, owner, batch, "cancel "+member.Noun()+" "+b.ID, nil, []string{b.ID}, last)
	if err != nil {
		return nil, nil, err
	}
	if !changed {
		return nil, []events.Event{ev}, nil
	}
	data, err := yaml.Marshal(batch)
	if err != nil {
		return nil, nil, err
	}
	ev.Files = []events.FileChange{{Path: rel}}
	return []nodeWrite{{rel: rel, data: data, before: snapshot, event: ev}}, nil, nil
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

// guardBatchStateCommitted refuses check batch and submit batch while the
// state that makes the batch whole is uncommitted: each member's state file,
// which carries its batch pointer, and at submit the batch record too. Sync
// reads the integration branch, so a pointer that never rode the batch branch
// into the merge leaves the batch with no members to cross. The guard asks
// Git's scoped status, and a revision source that cannot answer is not
// refused: the sync-side guard still holds.
func guardBatchStateCommitted(tree *store.Tree, rev vcs.Revisions, batch *store.Batch, verb string, includeRecord bool) error {
	status, ok := rev.(interface {
		DirtyUnder(string) ([]string, error)
	})
	if !ok {
		return nil
	}
	var paths []string
	if includeRecord {
		paths = append(paths, filepath.Join(workItemRelDir(batch), store.ShelfStateFile(store.NounBatch)))
	}
	for _, member := range tree.BatchMembers(batch.ID) {
		paths = append(paths, filepath.Join(workItemRelDir(member), store.ShelfStateFile(member.Noun())))
	}
	var uncommitted []string
	for _, rel := range paths {
		dirty, err := status.DirtyUnder(":(literal)" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if len(dirty) > 0 {
			uncommitted = append(uncommitted, filepath.ToSlash(rel))
		}
	}
	if len(uncommitted) == 0 {
		return nil
	}
	return &BatchError{
		Reason: ReasonBatchStateUncommitted, Noun: store.NounBatch, NodeID: batch.ID, Key: deref(batch.Key), CurrentState: batch.Status,
		Batch: batch.ID, BatchKey: deref(batch.Key), Paths: uncommitted,
		Seam: "git add -- " + strings.Join(uncommitted, " "),
		msg: fmt.Sprintf("batch %s cannot %s while its state is uncommitted: %s. Commit these on %s with the events beside them, so the merge carries every member's batch pointer to the integration branch",
			batch.ID, verb, strings.Join(uncommitted, ", "), batchBranchName(batch)),
	}
}
