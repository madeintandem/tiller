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
	// Paths are the state files a batch_state_uncommitted refusal wants
	// committed.
	Paths []string
	msg   string
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
	ReasonBatchStateUncommitted    = "batch_state_uncommitted"
	reasonBatchEditContradiction   = "batch_edit_contradiction"
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
// batchID ("" leaves the batch), and the member's own event for it. The event
// is batch.<noun>, as resolve.<noun> records a carrier: it carries the pointer
// and no status, so the member's history says it joined or left a batch and
// never borrows the batch's verb or state.
func memberWrite(tree *store.Tree, deps Deps, owner store.Contributor, member store.WorkNode, batchID, command string) (store.PlannedWrite, events.Event, error) {
	doc, relYml, snapshot, err := loadWorkItemFile(tree, member)
	if err != nil {
		return store.PlannedWrite{}, events.Event{}, err
	}
	before := doc.Body().BatchID
	doc.Body().BatchID = batchID
	data, err := yaml.Marshal(doc)
	if err != nil {
		return store.PlannedWrite{}, events.Event{}, err
	}
	ev := memberBatchEvent(deps, owner, member, before, batchID, command)
	ev.Files = []events.FileChange{{Path: relYml}}
	return store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data), ev, nil
}

// memberBatchEvent is the member-side record of joining or leaving a batch.
func memberBatchEvent(deps Deps, owner store.Contributor, member store.WorkNode, before, after, command string) events.Event {
	ev := events.NewEvent(deps.clock(), workItemEventType("batch", member.Noun()), owner.ID, command, member.NodeID())
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"batch": before}
	ev.After = map[string]any{"batch": after}
	return ev
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
	res := BatchResult{ID: batchID, Key: canonical, Title: batch.Title, Status: batch.Status, Members: batchMembersOf(members), Created: true}
	res.Added = res.Members
	// The batch's own event targets the batch alone and names its members in
	// after.members, so no member's history shows the batch's status as its own.
	ev := events.NewEvent(deps.clock(), "add."+store.NounBatch, owner.ID, "add batch "+local, batchID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": batch.Status, "key": canonical, "members": memberIDs(res.Members)}
	ev.Files = []events.FileChange{{Path: relYml}}
	writes := []store.PlannedWrite{store.PlanWrite(filepath.Join(tree.Root, relYml), store.AbsentSnapshot(), data)}
	evs := []events.Event{ev}
	for _, member := range members {
		write, memberEvent, err := memberWrite(tree, deps, owner, member, batchID, "add batch "+local)
		if err != nil {
			return BatchResult{}, err
		}
		writes = append(writes, write)
		evs = append(evs, memberEvent)
	}
	if err := commitMutation(tree, "add batch", writes, evs); err != nil {
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
	for _, member := range adding {
		for _, other := range removing {
			if member.NodeID() == other.NodeID() {
				return BatchResult{}, &usageError{reason: reasonBatchEditContradiction, msg: fmt.Sprintf("--add and --remove both name %s; a member is added or removed, not both", member.NodeID())}
			}
		}
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

	res.Added, res.Removed = batchMembersOf(adding), batchMembersOf(removing)
	command := "batch " + batchAddr(batch)
	for _, member := range res.Added {
		command += " --add " + member.ID
	}
	for _, member := range res.Removed {
		command += " --remove " + member.ID
	}
	var writes []store.PlannedWrite
	var evs []events.Event
	for _, change := range []struct {
		members []store.WorkNode
		batchID string
	}{{adding, batch.ID}, {removing, ""}} {
		for _, member := range change.members {
			write, memberEvent, err := memberWrite(tree, deps, owner, member, change.batchID, command)
			if err != nil {
				return BatchResult{}, err
			}
			writes = append(writes, write)
			evs = append(evs, memberEvent)
		}
	}
	gone := map[string]bool{}
	for _, member := range res.Removed {
		gone[member.ID] = true
		res.Warnings = append(res.Warnings, revertWarning(member, batch))
	}
	remaining := filterMembers(tree.BatchMembers(batch.ID), func(w store.WorkNode) bool { return !gone[w.NodeID()] })
	res.Members = append(batchMembersOf(remaining), res.Added...)

	ev, changed, err := batchMembershipEvent(deps, owner, batch, command, memberIDs(res.Added), memberIDs(res.Removed), len(res.Members) == 0)
	if err != nil {
		return BatchResult{}, err
	}
	res.Status, res.Cancelled, res.GatesStaled = batch.Status, batch.Status == statusCancelled, ev.After["gates"] == store.GateStatusStale
	if changed {
		data, err := yaml.Marshal(batch)
		if err != nil {
			return BatchResult{}, err
		}
		writes = append(writes, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data))
		ev.Files = []events.FileChange{{Path: relYml}}
	} else {
		writes = append(writes, store.PlanReadGuard(filepath.Join(tree.Root, relYml), snapshot))
	}
	if err := commitMutation(tree, "edit batch", writes, append([]events.Event{ev}, evs...)); err != nil {
		return BatchResult{}, err
	}
	res.Changed = true
	return res, nil
}

// batchMembershipEvent applies what a membership change does to the batch
// record and returns the batch's own event for it, which targets the batch
// alone and names the members in after.added and after.removed. A batch with
// no members left is cancelled, and that is a cancel.batch event. Otherwise it
// is edit.batch, and a batch at check has whatever gates it recorded staled.
// changed reports whether the record itself moved.
func batchMembershipEvent(deps Deps, owner store.Contributor, batch *store.Batch, command string, added, removed []string, empty bool) (events.Event, bool, error) {
	before := batch.Status
	after := map[string]any{}
	if len(added) > 0 {
		after["added"] = added
	}
	if len(removed) > 0 {
		after["removed"] = removed
	}
	verb, changed := "edit", false
	switch {
	case empty:
		if err := lifecycle.Apply(batch, "cancel", lifecycle.Payload{}); err != nil {
			return events.Event{}, false, err
		}
		after["reason"] = "its last member left"
		verb, changed = "cancel", true
	case batch.Status == "check" && staleRecordedGates(batch):
		after["gates"] = store.GateStatusStale
		changed = true
	}
	after["status"] = batch.Status
	ev := events.NewEvent(deps.clock(), verb+"."+store.NounBatch, owner.ID, command, batch.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"status": before}
	ev.After = after
	return ev, changed, nil
}

// staleRecordedGates stales a batch's recorded gates and reports whether any
// was still current. A batch with nothing recorded, as when no gate applies to
// shelf work, has nothing to run again.
func staleRecordedGates(batch *store.Batch) bool {
	staled := false
	for _, gate := range batch.Gates {
		if gate.Status != store.GateStatusStale {
			staled = true
		}
	}
	if staled {
		lifecycle.StaleGates(batch)
	}
	return staled
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
