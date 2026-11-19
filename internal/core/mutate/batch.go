package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// BatchInput is `tiller add batch <key> --title <text> <member>...`.
type BatchInput struct {
	Key     string
	Title   string
	Members []string
}

// BatchEditInput is `tiller batch <key> --add <member>... --remove <member>...`.
type BatchEditInput struct {
	Batch  string
	Add    []string
	Remove []string
}

// BatchMember is one shelf node as a batch result names it.
type BatchMember struct {
	Noun   string
	ID     string
	Key    string
	Status string
}

// BatchResult reports a batch after add or a membership edit.
type BatchResult struct {
	ID      string
	Key     string
	Title   string
	Status  string
	Members []BatchMember
	Added   []BatchMember
	Removed []BatchMember
	// Created is add's answer; Changed is an edit's.
	Created bool
	Changed bool
	// GatesStaled says the edit landed after check, so the batch's recorded
	// gates no longer describe its membership.
	GatesStaled bool
	// Cancelled says the edit removed the last member, which cancels the batch.
	Cancelled bool
	Warnings  []string
}

// BatchError is every blocked answer the batch rules give, exit 1. Reason
// names the rule, NodeID the node the command acted on, and Member the shelf
// node at fault when that is a different node. Seam is the command to run
// instead, when there is one.
type BatchError struct {
	Reason       string
	Noun         string
	NodeID       string
	Key          string
	CurrentState string
	Batch        string
	BatchKey     string
	Member       string
	Seam         string
	msg          string
}

func (e *BatchError) Error() string { return e.msg }

// The batch refusal reasons.
const (
	ReasonWorkBatched              = "work_batched"
	ReasonBatchMemberUnready       = "batch_member_unready"
	ReasonBatchMemberTerminal      = "batch_member_terminal"
	ReasonBatchMemberStarted       = "batch_member_started"
	ReasonBatchMemberCarried       = "batch_member_carried"
	ReasonBatchMemberRecovered     = "batch_member_recovered"
	ReasonBatchMemberInBatch       = "batch_member_in_batch"
	ReasonBatchMemberClaimed       = "batch_member_claimed"
	ReasonBatchNotMember           = "batch_not_member"
	ReasonBatchInReview            = "batch_in_review"
	ReasonBatchClosed              = "batch_closed"
	ReasonBatchStarted             = "batch_started"
	reasonBatchTooSmall            = "batch_too_small"
	reasonBatchMemberRef           = "batch_member_ref"
	reasonBatchEditRequired        = "batch_edit_required"
	reasonBatchRecoveryUnsupported = "batch_recovery_unsupported"
)

// batchAddr is how a refusal names a batch in a command: its local key.
func batchAddr(b *store.Batch) string { return localKey(deref(b.Key)) }

// workBatched refuses a lifecycle or ownership verb on a member of a live
// batch, naming the batch and the command that does the same thing there.
func workBatched(member store.WorkNode, batch *store.Batch, verb string) error {
	b := member.Body()
	seam := fmt.Sprintf("tiller %s batch %s", verb, batchAddr(batch))
	return &BatchError{
		Reason: ReasonWorkBatched, Noun: member.Noun(), NodeID: b.ID, Key: deref(b.Key), CurrentState: b.Status,
		Batch: batch.ID, BatchKey: deref(batch.Key), Seam: seam,
		msg: fmt.Sprintf("%s %s is in batch %s, which owns its branch, gates, and review; run `%s`, or take it out with `tiller batch %s --remove %s`",
			member.Noun(), b.ID, batch.ID, seam, batchAddr(batch), b.ID),
	}
}

// liveBatchOf returns the batch that answers for a shelf node right now, nil
// for a batch itself and for a node no live batch holds.
func liveBatchOf(tree *store.Tree, w store.WorkNode) *store.Batch {
	if !gen.Batched(tree, w) {
		return nil
	}
	return tree.BatchOf(w)
}

// refuseBatched is the guard the member-facing verbs share.
func refuseBatched(tree *store.Tree, w store.WorkNode, verb string) error {
	if batch := liveBatchOf(tree, w); batch != nil {
		return workBatched(w, batch, verb)
	}
	return nil
}

// resolveBatchMembers resolves each raw member reference to a shelf node,
// dropping repeats. A member is named by id or by shelf key, since the batch
// mixes shelves and a bare key would not say which one.
func resolveBatchMembers(tree *store.Tree, raws []string) ([]store.WorkNode, error) {
	r := resolve.NewResolver(tree)
	seen := map[string]bool{}
	var out []store.WorkNode
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		shelf, ok := store.ShelfOfRef(raw)
		if !ok {
			return nil, &usageError{reason: reasonBatchMemberRef, msg: fmt.Sprintf("%q does not name a bug, chore, or tweak; name each member by id (B-, CH-, TW-) or by shelf key (bugs/<key>, chores/<key>, tweaks/<key>)", raw)}
		}
		n, err := r.Resolve(resolve.Ref{Noun: shelf.Noun, Raw: raw})
		if err != nil {
			return nil, err
		}
		w, _ := store.AsWorkNode(n)
		if seen[w.NodeID()] {
			continue
		}
		seen[w.NodeID()] = true
		out = append(out, w)
	}
	return out, nil
}

// guardBatchMember refuses a shelf node that cannot join batch: one that is
// finished, has started its own work, rides a live carrier, holds a recovered
// review, sits in another live batch, or is claimed by someone other than
// holder. batch is nil while the batch is being created.
func guardBatchMember(tree *store.Tree, w store.WorkNode, batch *store.Batch, holder string) error {
	b := w.Body()
	refuse := func(reason, why string) error {
		e := &BatchError{Reason: reason, Noun: w.Noun(), NodeID: b.ID, Key: deref(b.Key), CurrentState: b.Status, Member: b.ID,
			msg: fmt.Sprintf("%s %s cannot join a batch: %s", w.Noun(), b.ID, why)}
		if batch != nil {
			e.Batch, e.BatchKey = batch.ID, deref(batch.Key)
		}
		return e
	}
	switch {
	case b.Status == statusDone || b.Status == statusCancelled:
		return refuse(ReasonBatchMemberTerminal, "it is "+b.Status)
	case b.Started || (b.Status != statusProposed && b.Status != "ready"):
		return refuse(ReasonBatchMemberStarted, "its own work has started ("+b.Status+"), so its branch and review are already its own")
	case workNodeCarried(tree, w):
		return refuse(ReasonBatchMemberCarried, fmt.Sprintf("%s carries it; clear that with `tiller resolve %s %s --clear` first", b.ResolvedBy, w.Noun(), deref(b.Key)))
	case b.RecoveredReview != nil:
		return refuse(ReasonBatchMemberRecovered, "it holds a recovered review, and the next sync crosses it on that record")
	}
	if other := liveBatchOf(tree, w); other != nil && (batch == nil || other.ID != batch.ID) {
		return refuse(ReasonBatchMemberInBatch, fmt.Sprintf("it is already in batch %s", other.ID))
	}
	if b.Owner != "" && b.Owner != holder {
		return refuse(ReasonBatchMemberClaimed, fmt.Sprintf("%s has claimed it, and a batch has one owner", b.Owner))
	}
	return nil
}

// batchMembersOf projects a batch's members for a result.
func batchMembersOf(nodes []store.WorkNode) []BatchMember {
	out := make([]BatchMember, 0, len(nodes))
	for _, w := range nodes {
		b := w.Body()
		out = append(out, BatchMember{Noun: w.Noun(), ID: b.ID, Key: deref(b.Key), Status: b.Status})
	}
	return out
}

func memberIDs(members []BatchMember) []string {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	return ids
}

// memberWrite plans one member's state file with its batch pointer set to
// batchID ("" leaves the batch).
func memberWrite(tree *store.Tree, member store.WorkNode, batchID string) (store.PlannedWrite, string, error) {
	doc, relYml, snapshot, err := loadWorkItemFile(tree, member)
	if err != nil {
		return store.PlannedWrite{}, "", err
	}
	doc.Body().BatchID = batchID
	data, err := yaml.Marshal(doc)
	if err != nil {
		return store.PlannedWrite{}, "", err
	}
	return store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data), relYml, nil
}

// AddBatch creates a batch over two or more shelf nodes. The batch is born
// ready and unowned: it needs no approval of its own, and its members may
// still be proposed, which start batch refuses until each is approved. Like
// resolve, it is the human's act, or the agent's on an explicit instruction
// naming the members and asking for one review. Re-adding an existing key is
// the idempotent no-op every other add keeps.
func AddBatch(tree *store.Tree, deps Deps, in BatchInput) (BatchResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return BatchResult{}, err
	}
	local := strings.TrimSpace(in.Key)
	if strings.Contains(local, "/") || !renameKeySegmentRE.MatchString(local) {
		return BatchResult{}, &usageError{reason: "invalid_key", msg: fmt.Sprintf("a batch key is one local key segment (%s), not %q; the batches/ prefix is implied", renameKeySegmentRE.String(), in.Key)}
	}
	canonical := store.ShelfCanonicalKey(store.NounBatch, local)
	relYml := filepath.Join(store.ShelfRelDir(store.NounBatch, local), store.ShelfStateFile(store.NounBatch))
	if fileExists(tree.Root, filepath.Join(tree.Root, relYml)) {
		existing := &store.Batch{}
		if err := readYAML(tree.Root, relYml, existing); err != nil {
			return BatchResult{}, err
		}
		return BatchResult{ID: existing.ID, Key: deref(existing.Key), Title: existing.Title, Status: existing.Status,
			Members: batchMembersOf(tree.BatchMembers(existing.ID))}, nil
	}
	if strings.TrimSpace(in.Title) == "" {
		return BatchResult{}, &usageError{reason: "invalid_title", msg: "add needs a non-blank --title"}
	}
	members, err := resolveBatchMembers(tree, in.Members)
	if err != nil {
		return BatchResult{}, err
	}
	if len(members) < 2 {
		return BatchResult{}, &usageError{reason: reasonBatchTooSmall, msg: "a batch reviews two or more bugs, chores, or tweaks together; one node keeps its own branch and review"}
	}
	for _, member := range members {
		if err := guardBatchMember(tree, member, nil, owner.ID); err != nil {
			return BatchResult{}, err
		}
	}
	prefix, err := id.PrefixFor(store.NounBatch)
	if err != nil {
		return BatchResult{}, err
	}
	batchID, err := deps.mintIn(tree, prefix)
	if err != nil {
		return BatchResult{}, err
	}
	batch := &store.Batch{WorkItem: store.WorkItem{ID: batchID, Key: &canonical, Title: strings.TrimSpace(in.Title), Status: "ready"}}
	data, err := yaml.Marshal(batch)
	if err != nil {
		return BatchResult{}, err
	}
	writes := []store.PlannedWrite{store.PlanWrite(filepath.Join(tree.Root, relYml), store.AbsentSnapshot(), data)}
	files := []events.FileChange{{Path: relYml}}
	for _, member := range members {
		write, rel, err := memberWrite(tree, member, batchID)
		if err != nil {
			return BatchResult{}, err
		}
		writes = append(writes, write)
		files = append(files, events.FileChange{Path: rel})
	}
	res := BatchResult{ID: batchID, Key: canonical, Title: batch.Title, Status: batch.Status, Members: batchMembersOf(members), Created: true}
	res.Added = res.Members
	ev := events.NewEvent(deps.clock(), "add."+store.NounBatch, owner.ID, "add batch "+local, append([]string{batchID}, memberIDs(res.Members)...)...)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": batch.Status, "key": canonical, "members": memberIDs(res.Members)}
	ev.Files = files
	if err := commitMutation(tree, "add batch", writes, []events.Event{ev}); err != nil {
		return BatchResult{}, err
	}
	return res, nil
}

// resolveBatch resolves a batch by id or key.
func resolveBatch(tree *store.Tree, raw string) (*store.Batch, error) {
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounBatch, Raw: raw})
	if err != nil {
		return nil, err
	}
	return n.(*store.Batch), nil
}

// batchEditable refuses a membership change the batch's state does not allow:
// a finished batch has no membership left to change, and one in review is what
// the reviewer is reading until it is resumed.
func batchEditable(batch *store.Batch, noun, nodeID, key, state string) error {
	e := &BatchError{Noun: noun, NodeID: nodeID, Key: key, CurrentState: state, Batch: batch.ID, BatchKey: deref(batch.Key)}
	switch batch.Status {
	case statusDone, statusCancelled:
		e.Reason = ReasonBatchClosed
		e.msg = fmt.Sprintf("batch %s is %s; its membership is history", batch.ID, batch.Status)
		return e
	case "in_review":
		e.Reason = ReasonBatchInReview
		e.Seam = fmt.Sprintf(`tiller resume batch %s --reason "<why>"`, batchAddr(batch))
		e.msg = fmt.Sprintf("batch %s is in review, so its membership is what the reviewer is reading; run `%s` before changing it", batch.ID, e.Seam)
		return e
	}
	return nil
}

// revertWarning is what a removal says about the member's commits.
func revertWarning(member BatchMember, batch *store.Batch) string {
	return fmt.Sprintf("%s %s left batch %s and is back in its own queue; any commits already made for it on %s are yours to revert",
		member.Noun, member.ID, batch.ID, batchBranchName(batch))
}

func batchBranchName(batch *store.Batch) string {
	if batch.Branch != "" {
		return "branch " + batch.Branch
	}
	return "the batch branch"
}

// EditBatch adds members to a batch, removes them, or both. It is allowed
// while the batch is ready, in_progress, or check. A change at check stales
// the recorded gates, since they were run over a different membership. A
// removed member keeps its stored status and returns to its own queue. A batch
// left with no members is cancelled in the same mutation.
func EditBatch(tree *store.Tree, deps Deps, in BatchEditInput) (BatchResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return BatchResult{}, err
	}
	if len(in.Add) == 0 && len(in.Remove) == 0 {
		return BatchResult{}, &usageError{reason: reasonBatchEditRequired, msg: "batch needs --add <member> or --remove <member>"}
	}
	treeBatch, err := resolveBatch(tree, in.Batch)
	if err != nil {
		return BatchResult{}, err
	}
	doc, relYml, snapshot, err := loadWorkItemFile(tree, treeBatch)
	if err != nil {
		return BatchResult{}, err
	}
	batch := doc.(*store.Batch)
	adding, err := resolveBatchMembers(tree, in.Add)
	if err != nil {
		return BatchResult{}, err
	}
	removing, err := resolveBatchMembers(tree, in.Remove)
	if err != nil {
		return BatchResult{}, err
	}
	// Drop the requests that already hold, so a retry is a no-change success.
	adding = filterMembers(adding, func(w store.WorkNode) bool { return w.Body().BatchID != batch.ID })
	for _, member := range removing {
		if member.Body().BatchID != batch.ID {
			b := member.Body()
			return BatchResult{}, &BatchError{Reason: ReasonBatchNotMember, Noun: member.Noun(), NodeID: b.ID, Key: deref(b.Key), CurrentState: b.Status,
				Batch: batch.ID, BatchKey: deref(batch.Key), Member: b.ID,
				msg: fmt.Sprintf("%s %s is not in batch %s", member.Noun(), b.ID, batch.ID)}
		}
	}
	res := BatchResult{ID: batch.ID, Key: deref(batch.Key), Title: batch.Title, Status: batch.Status}
	if len(adding) == 0 && len(removing) == 0 {
		res.Members = batchMembersOf(tree.BatchMembers(batch.ID))
		return res, nil
	}
	if err := batchEditable(batch, store.NounBatch, batch.ID, deref(batch.Key), batch.Status); err != nil {
		return BatchResult{}, err
	}
	holder := batch.Owner
	if holder == "" {
		holder = owner.ID
	}
	for _, member := range adding {
		if err := guardBatchMember(tree, member, batch, holder); err != nil {
			return BatchResult{}, err
		}
		// A started batch is being built: every member must already be
		// approved, the rule start batch enforced for the others.
		if batch.Started && member.Body().Status != "ready" {
			return BatchResult{}, memberUnready(batch, member)
		}
	}

	var writes []store.PlannedWrite
	var files []events.FileChange
	for _, change := range []struct {
		members []store.WorkNode
		batchID string
	}{{adding, batch.ID}, {removing, ""}} {
		for _, member := range change.members {
			write, rel, err := memberWrite(tree, member, change.batchID)
			if err != nil {
				return BatchResult{}, err
			}
			writes = append(writes, write)
			files = append(files, events.FileChange{Path: rel})
		}
	}
	res.Added, res.Removed = batchMembersOf(adding), batchMembersOf(removing)
	gone := map[string]bool{}
	for _, member := range res.Removed {
		gone[member.ID] = true
		res.Warnings = append(res.Warnings, revertWarning(member, batch))
	}
	remaining := filterMembers(tree.BatchMembers(batch.ID), func(w store.WorkNode) bool { return !gone[w.NodeID()] })
	res.Members = append(batchMembersOf(remaining), res.Added...)

	before := batch.Status
	after := map[string]any{}
	if len(res.Added) > 0 {
		after["added"] = memberIDs(res.Added)
	}
	if len(res.Removed) > 0 {
		after["removed"] = memberIDs(res.Removed)
	}
	changed, err := batchAfterMembershipChange(batch, len(res.Members) == 0, after)
	if err != nil {
		return BatchResult{}, err
	}
	res.Status, res.Cancelled, res.GatesStaled = batch.Status, batch.Status == statusCancelled, after["gates"] == store.GateStatusStale
	if changed {
		data, err := yaml.Marshal(batch)
		if err != nil {
			return BatchResult{}, err
		}
		writes = append(writes, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data))
		files = append(files, events.FileChange{Path: relYml})
	} else {
		writes = append(writes, store.PlanReadGuard(filepath.Join(tree.Root, relYml), snapshot))
	}
	command := "batch " + batchAddr(batch)
	for _, member := range res.Added {
		command += " --add " + member.ID
	}
	for _, member := range res.Removed {
		command += " --remove " + member.ID
	}
	targets := append([]string{batch.ID}, append(memberIDs(res.Added), memberIDs(res.Removed)...)...)
	ev := events.NewEvent(deps.clock(), "edit."+store.NounBatch, owner.ID, command, targets...)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"status": before}
	after["status"] = batch.Status
	ev.After = after
	ev.Files = files
	if err := commitMutation(tree, "edit batch", writes, []events.Event{ev}); err != nil {
		return BatchResult{}, err
	}
	res.Changed = true
	return res, nil
}

// batchAfterMembershipChange applies what a membership change does to the
// batch record itself, noting it on the event's after map: a batch with no
// members left is cancelled, and one at check has its gates staled. It reports
// whether the record changed.
func batchAfterMembershipChange(batch *store.Batch, empty bool, after map[string]any) (bool, error) {
	if empty {
		if err := lifecycle.Apply(batch, "cancel", lifecycle.Payload{}); err != nil {
			return false, err
		}
		after["reason"] = "its last member left"
		return true, nil
	}
	if batch.Status == "check" {
		lifecycle.StaleGates(batch)
		after["gates"] = store.GateStatusStale
		return true, nil
	}
	return false, nil
}

func filterMembers(nodes []store.WorkNode, keep func(store.WorkNode) bool) []store.WorkNode {
	var out []store.WorkNode
	for _, w := range nodes {
		if keep(w) {
			out = append(out, w)
		}
	}
	return out
}

// memberUnready refuses a batch that would start, or keep building, over a
// member nobody has approved.
func memberUnready(batch *store.Batch, member store.WorkNode) error {
	b := member.Body()
	seam := fmt.Sprintf("tiller approve %s %s", member.Noun(), deref(b.Key))
	return &BatchError{
		Reason: ReasonBatchMemberUnready, Noun: store.NounBatch, NodeID: batch.ID, Key: deref(batch.Key), CurrentState: batch.Status,
		Batch: batch.ID, BatchKey: deref(batch.Key), Member: b.ID, Seam: seam,
		msg: fmt.Sprintf("batch %s cannot build %s %s while it is %s; a human approves it first (`%s`), or it leaves the batch (`tiller batch %s --remove %s`)",
			batch.ID, member.Noun(), b.ID, b.Status, seam, batchAddr(batch), b.ID),
	}
}
