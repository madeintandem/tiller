package mutate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// This file holds approval's judgment and apply for a correct-eval operation:
// a reviewed correction of a done feature's eval bindings. Staging
// lives in amendment_stage_correct_eval.go.
//
// A done feature's completion record pins the contract it was verified
// against (verified_against.eval). Editing that contract afterwards, even only
// to move a test binding, leaves stale_spec reporting the feature forever.
// Approval reads the verified contract back out of git by its pinned blob,
// proves the working-tree contract differs from it only in what a correction
// may change, and then re-pins the eval leg to the working-tree blob. Nothing
// else on the node moves: status, accepted_against, and gates stay as they are.

// checkEvalCorrection is the pure judgment of what a correction may change. The
// node, the set of criterion ids, and each kept criterion's statement,
// polarity, required flag, and baseline surface and capture are the contract
// itself and must not move. A binding (type, ref, run) and a baseline artifact
// are how the contract is checked and may change freely, as may the markdown
// body, which is not parsed here at all. A criterion may leave the contract
// only when retire names it, and retire may name only a criterion that left.
func checkEvalCorrection(old, current *store.EvalFile, retire []string) error {
	if old.Node != current.Node {
		return contractChangeErr(fmt.Sprintf("the corrected contract changes node from %s to %s", old.Node, current.Node))
	}

	verified := make(map[string]store.EvalCriterion, len(old.Criteria))
	for _, c := range old.Criteria {
		verified[c.ID] = c
	}
	kept := make(map[string]bool, len(current.Criteria))
	for _, c := range current.Criteria {
		kept[c.ID] = true
		was, ok := verified[c.ID]
		if !ok {
			return contractChangeErr(fmt.Sprintf("the corrected contract adds criterion %s; new acceptance belongs to forward work", c.ID))
		}
		if field := changedCriterionField(was, c); field != "" {
			return contractChangeErr(fmt.Sprintf("criterion %s changes its %s; a correction may change only bindings and baseline artifacts, and changed acceptance belongs to forward work", c.ID, field))
		}
	}

	retired := make(map[string]bool, len(retire))
	for _, raw := range retire {
		id := strings.TrimSpace(raw)
		retired[id] = true
		if kept[id] {
			return contractChangeErr(fmt.Sprintf("the correction retires %s, which the corrected contract still carries", id))
		}
		if _, ok := verified[id]; !ok {
			return contractChangeErr(fmt.Sprintf("the correction retires %s, which the verified contract never carried", id))
		}
	}
	for _, c := range old.Criteria {
		if !kept[c.ID] && !retired[c.ID] {
			return contractChangeErr(fmt.Sprintf("the corrected contract removes criterion %s without --retire", c.ID))
		}
	}
	return nil
}

// changedCriterionField names the first contract field that differs between a
// verified criterion and its corrected form, or "" when only correctable
// fields moved. A missing baseline reads as an empty one, so dropping a
// baseline whose surface was set is a surface change while adding one that
// carries only an artifact is not.
func changedCriterionField(was, is store.EvalCriterion) string {
	switch {
	case was.Statement != is.Statement:
		return "statement"
	case was.Polarity != is.Polarity:
		return "polarity"
	case was.Required != is.Required:
		return "required flag"
	}
	wasBase, isBase := baselineOf(was), baselineOf(is)
	switch {
	case wasBase.Surface != isBase.Surface:
		return "baseline.surface"
	case !wasBase.CapturedAt.SameAs(isBase.CapturedAt):
		return "baseline.captured_at"
	}
	return ""
}

func baselineOf(c store.EvalCriterion) store.EvalBaseline {
	if c.Baseline == nil {
		return store.EvalBaseline{}
	}
	return *c.Baseline
}

func contractChangeErr(msg string) error {
	return &usageError{reason: "correct_eval_changes_contract", msg: msg}
}

// evalCorrection is one correct-eval phase 1 judged and approved: the blob the
// completion record pinned, the working-tree blob it re-pins to, the eval
// file's repo-relative path, and the retired ids, trimmed. Phase 2 applies
// exactly this, never a second read of the file, and a read guard over the
// file holds those bytes still until the commit.
type evalCorrection struct {
	base    string
	pin     string
	rel     string
	retired []string
}

// checkCorrectEvalTarget is correct-eval's intrinsic target-state rule, run
// with the other operations' in checkAmendmentTargetStates: the target must be
// a feature, the one grain carrying an acceptance contract, and it must be
// done, the one state with a completion record to re-pin. Staging refuses a
// non-feature already; this catches a hand-edited batch.
func checkCorrectEvalTarget(r resolvedOp, current *nodeEdit) error {
	if _, ok := current.doc.(*store.Feature); !ok {
		return correctEvalNotFeatureErr(fmt.Sprintf("operation %d: ", r.index), current.doc)
	}
	if status := current.doc.NodeStatus(); status != statusDone {
		return refuseCorrection(r, "correct_eval_target_not_done",
			"it is %s; correct-eval re-pins a done feature's completion record, and unfinished work changes its contract through revise", status)
	}
	return nil
}

func correctEvalNotFeatureErr(prefix string, n store.Node) error {
	return &usageError{
		reason: "correct_eval_not_feature",
		msg: prefix + fmt.Sprintf("correct-eval targets a done feature's acceptance contract; %s is a %s",
			n.NodeID(), n.Noun()),
	}
}

// judgeEvalCorrections is phase 1's judgment of every correct-eval in the
// batch, run after the preconditions (so an eval edited since staging keeps
// its exit-1 stale answer) and after the target-state rules (so every target
// here is a done feature). It writes nothing; it records each verdict on the
// batch for phase 2. One batch corrects a feature at most once, since two
// corrections would judge the same bytes against the same base twice.
func (b *batch) judgeEvalCorrections(tree *store.Tree, rev vcs.Revisions, doc *store.Amendment, plan []resolvedOp) error {
	corrected := map[string]int{}
	for _, r := range plan {
		if r.op.Op != store.AmendmentOpCorrectEval {
			continue
		}
		if first, dup := corrected[r.target.NodeID()]; dup {
			return refuseCorrection(r, "correct_eval_duplicate_target",
				"operation %d already corrects it; one amendment corrects a feature's contract once", first)
		}
		corrected[r.target.NodeID()] = r.index

		judged, err := b.judgeEvalCorrection(tree, rev, doc.ID, r)
		if err != nil {
			return err
		}
		if b.evalCorrections == nil {
			b.evalCorrections = map[int]evalCorrection{}
		}
		b.evalCorrections[r.index] = judged
	}
	return nil
}

// judgeEvalCorrection requires the staged eval_revision pin, reads the
// verified contract back out of git by the blob the completion record pinned,
// reads the working-tree contract, and approves the pair only when the working
// tree still hashes to the staged pin, changed from the verified contract,
// still parses as a valid contract, and changed nothing checkEvalCorrection
// protects.
func (b *batch) judgeEvalCorrection(tree *store.Tree, rev vcs.Revisions, amendmentID string, r resolvedOp) (evalCorrection, error) {
	pinned, err := evalPinOf(r)
	if err != nil {
		return evalCorrection{}, err
	}
	e, err := b.editFor(tree, r.target)
	if err != nil {
		return evalCorrection{}, err
	}
	feature := e.doc.(*store.Feature)
	base := feature.VerifiedAgainst.Eval.Value
	if base == "" {
		return evalCorrection{}, refuseCorrection(r, "correct_eval_unverified",
			"its completion record pins no eval contract (verified_against.eval); there is no verified contract to correct")
	}
	rel, ok := basisFileFor(tree, feature, preconditionEvalRevision)
	if !ok {
		return evalCorrection{}, refuseCorrection(r, "correct_eval_no_contract",
			"it has no canonical key, so it has no feature.eval.md path to read a corrected contract from")
	}

	verifiedBytes, err := readVerifiedContract(rev, base)
	if err != nil {
		var unreadable *vcs.NotVersionedError
		if errors.As(err, &unreadable) {
			return evalCorrection{}, refuseCorrection(r, "correct_eval_base_unreadable",
				"git cannot read blob %s as the verified contract (it is not in the object store, or is not a readable blob); if an earlier correction or done crossing is uncommitted, commit it first, then approve again", base)
		}
		return evalCorrection{}, err
	}

	path := filepath.Join(tree.Root, rel)
	correctedBytes, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, path)
	if err != nil {
		return evalCorrection{}, err
	}
	if !snapshot.Exists() {
		return evalCorrection{}, refuseCorrection(r, "correct_eval_no_contract",
			"%s is not in the working tree; there is no corrected contract to pin", rel)
	}
	// The same working-tree hash the eval_revision precondition compares, read
	// after the snapshot so the read guard below covers it; checkEvalPin ties
	// it to the staged pin, so the value approval writes is the one the
	// human's reviewed batch named.
	pin, err := WorktreeBlobRef(rev, tree.Root, rel)
	if err != nil {
		return evalCorrection{}, err
	}
	if err := checkEvalPin(amendmentID, r, pinned, pin.Value); err != nil {
		return evalCorrection{}, err
	}
	if pin.Value == base {
		return evalCorrection{}, refuseCorrection(r, "correct_eval_no_change",
			"%s is unchanged from the verified contract %s; there is nothing to re-pin", rel, base)
	}

	verified, err := store.ParseEvalFile(verifiedBytes)
	if err != nil {
		return evalCorrection{}, refuseCorrection(r, "correct_eval_base_unreadable",
			"the verified contract at blob %s does not parse: %v", base, err)
	}
	corrected, err := store.ParseEvalFile(correctedBytes)
	if err != nil {
		return evalCorrection{}, refuseCorrection(r, "eval_contract_invalid", "%s does not parse: %v", rel, err)
	}
	if err := corrected.ValidateContract(); err != nil {
		return evalCorrection{}, fmt.Errorf("operation %d cannot correct %s: %w", r.index, r.target.NodeID(), err)
	}
	if err := checkEvalCorrection(verified, corrected, r.op.Retire); err != nil {
		return evalCorrection{}, fmt.Errorf("operation %d cannot correct %s: %w", r.index, r.target.NodeID(), err)
	}

	b.guards = append(b.guards, store.PlanReadGuard(path, snapshot))
	return evalCorrection{base: base, pin: pin.Value, rel: rel, retired: trimmedIDs(r.op.Retire)}, nil
}

// evalPinOf and checkEvalPin together make "the bytes the human reviewed are
// the bytes approval pins" true. Staging always pins eval_revision to the
// working-tree contract, so an operation without one was hand-edited, and
// nothing then ties the judged bytes to a review: evalPinOf refuses it and
// sends the human back through staging.
func evalPinOf(r resolvedOp) (string, error) {
	pinned, ok := r.op.Preconditions[preconditionEvalRevision]
	if !ok {
		return "", refuseCorrection(r, "correct_eval_unpinned",
			"it carries no eval_revision precondition naming the reviewed feature.eval.md, so approval cannot prove it pins the bytes that were reviewed; withdraw this amendment and stage the correction again, which pins them")
	}
	return pinned, nil
}

// checkEvalPin compares the staged pin with worktree, the hash the judge just
// read. checkPreconditions already compared the two earlier in phase 1, so a
// mismatch here means the file moved in between, and it answers what that
// comparison would have: the stale-precondition report (exit 1).
func checkEvalPin(amendmentID string, r resolvedOp, pinned, worktree string) error {
	if pinned == worktree {
		return nil
	}
	return &AmendmentPreconditionsStaleError{AmendmentID: amendmentID, Stale: []StalePrecondition{{
		Index: r.index, Op: r.op.Op, Target: r.target.NodeID(), Precondition: preconditionEvalRevision,
		Expected: pinned, Actual: worktree,
		Note: "feature.eval.md changed while approval was judging it",
	}}}
}

// readVerifiedContract reads a pinned eval blob by its object id. Only a
// repository that can read historical objects can answer, which the real git
// window always can.
func readVerifiedContract(rev vcs.Revisions, blob string) ([]byte, error) {
	history, ok := rev.(vcs.HistoricalRevisions)
	if !ok {
		return nil, fmt.Errorf("mutate: reading the verified contract %s needs historical blob reads", blob)
	}
	return history.ReadBlob(blob)
}

// refuseCorrection is an exit-2 refusal naming the operation and the feature
// it would have corrected.
func refuseCorrection(r resolvedOp, reason, format string, args ...any) error {
	return &usageError{
		reason: reason,
		msg:    fmt.Sprintf("operation %d cannot correct %s: ", r.index, r.target.NodeID()) + fmt.Sprintf(format, args...),
	}
}

// trimmedIDs is retire's criterion ids without surrounding whitespace, the form
// staging stores and the gate matches criteria by; nil stays nil.
func trimmedIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strings.TrimSpace(id)
	}
	return out
}

// correctEval is phase 2's apply: each judged correction re-pins its feature's
// verified_against.eval to the working-tree blob phase 1 approved and records
// one correct-eval.feature event. Nothing else on the node changes.
func (b *batch) correctEval(tree *store.Tree, deps Deps, owner store.Contributor,
	doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {

	for _, r := range plan {
		if r.op.Op != store.AmendmentOpCorrectEval {
			continue
		}
		judged, ok := b.evalCorrections[r.index]
		if !ok {
			return fmt.Errorf("mutate: operation %d (correct-eval) reached apply unjudged", r.index)
		}
		e, err := b.writeFor(tree, r.target)
		if err != nil {
			return err
		}
		feature := e.doc.(*store.Feature)
		feature.VerifiedAgainst.Eval = store.RevisionRef{
			Kind: store.KindGitBlob, Value: judged.pin, Path: judged.rel, ObservedAt: deps.clock().Now().UTC(),
		}

		ev := amendmentOpEvent(deps, owner, doc, eventCorrectEvalPrefix+e.noun, []string{feature.ID}, e.rel)
		ev.Before = map[string]any{"eval": judged.base}
		ev.After["eval"] = judged.pin
		ev.After["reason"] = r.op.Reason
		if len(judged.retired) > 0 {
			ev.After["retired"] = judged.retired
		}
		ev.After["operation_index"] = r.index
		ev.After["status"] = feature.Status
		b.events = append(b.events, ev)

		applied[r.index] = AppliedOperation{
			Index:  r.index,
			Op:     r.op.Op,
			Noun:   e.noun,
			NodeID: feature.ID,
			Key:    deref(feature.Key),
			Status: feature.Status,
			Note:   commitHint(judged),
		}
	}
	return nil
}

// commitHint is the approve output's reminder beside an applied correction:
// the new pin names a working-tree blob git does not hold until the corrected
// file is committed, and a later correction of the same feature refuses
// correct_eval_base_unreadable until it is.
func commitHint(c evalCorrection) string {
	short := c.pin
	if len(short) > len("git:")+7 {
		short = short[:len("git:")+7]
	}
	return fmt.Sprintf("re-pinned verified_against.eval to %s; commit %s with this change, since a later correction reads it back from git",
		short, filepath.ToSlash(c.rel))
}
