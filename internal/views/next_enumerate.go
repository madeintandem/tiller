package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// The old route walk's arms, in its order. A candidate's arm is the
// plan-order tie-break inside a tier (plan ruling 3).
const (
	armBaseline = iota
	armContinue
	armAmendment
	armCloseout
	armClaim
	armBlocker
	armSpecApproval
	armEpicApproval
	armSpecify
	armTaskMerge
	armReviewMerge
	armAnswer
	armAttest
	armTriage
	armDecompose
)

// queueKindArms places every queue entry kind among the walk's arms.
var queueKindArms = map[string]int{
	"approve_amendment": armAmendment,
	"workflow_closeout": armCloseout,
	"feature_check":     armCloseout,
	"approve_feature":   armSpecApproval,
	"approve_tasks":     armSpecApproval,
	"approve_epic":      armEpicApproval,
	"merge_feature":     armReviewMerge,
	"merge_epic":        armReviewMerge,
	"answer_question":   armAnswer,
	"attest_gate":       armAttest,
	"attest_criterion":  armAttest,
	"approve_bug":       armTriage,
	"approve_chore":     armTriage,
	"approve_tweak":     armTriage,
	"carrier_cancelled": armTriage,
}

// queueKindActions is the verb phrase each queue kind's move reads as. The
// closeout kinds read as their workflow advice's own action instead.
var queueKindActions = map[string]string{
	"approve_amendment": "approve amendment",
	"approve_feature":   "approve the spec for",
	"approve_tasks":     "approve the tasks for",
	"approve_epic":      "approve epic",
	"merge_feature":     "merge",
	"merge_epic":        "merge",
	"answer_question":   "answer",
	"attest_gate":       "attest",
	"attest_criterion":  "attest",
	"approve_bug":       "triage",
	"approve_chore":     "triage",
	"approve_tweak":     "triage",
	"carrier_cancelled": "triage",
}

// candidatesOf enumerates every legal next move for the reader's checkout:
// each arm of the old route walk run to exhaustion instead of stopping at its
// first match. An unconfirmed brownfield
// baseline is exclusive, as it is in the walk. Excluded throughout: work in
// flight on another contributor's branch, question-held tasks (their
// questions are candidates instead), and work behind an unmet dependency.
// The result is in arm order, each arm in its own order; ranking is not done
// here.
func candidatesOf(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate {
	if baselineRequired(t) {
		return baselineCandidates(t)
	}
	var out []NextCandidate
	out = append(out, confirmDesignCandidates(t)...)
	out = append(out, continueCandidates(t, order, in)...)
	out = append(out, queueCandidates(t, in)...)
	out = append(out, claimCandidates(t, order, in)...)
	out = append(out, blockerCandidates(t, order, in)...)
	out = append(out, specifyCandidates(t, order)...)
	out = append(out, mergeWaitCandidates(t, order, in)...)
	out = append(out, decomposeCandidates(t)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].arm < out[j].arm })
	return out
}

// inArm sets every candidate's arm, and its position to its index.
func inArm(arm int, cands []NextCandidate) []NextCandidate {
	for i := range cands {
		cands[i].arm, cands[i].pos = arm, i
	}
	return cands
}

// baselineCandidates are the only moves while a brownfield baseline awaits
// confirmation: derive one when no design doc is registered, otherwise
// confirm each registered doc still pending.
func baselineCandidates(t *store.Tree) []NextCandidate {
	if len(t.Baseline.Docs) == 0 {
		return inArm(armBaseline, []NextCandidate{{
			Actor: actorAgent, Kind: "derive_baseline", Action: "derive the baseline for existing code",
			Skill: "derive-baseline", Reason: baselineDeriveReason,
		}})
	}
	return confirmDesignCandidates(t)
}

// confirmDesignCandidates confirm each registered design doc still pending.
// Once the first confirmation releases startup they stand beside every other
// move rather than alone.
func confirmDesignCandidates(t *store.Tree) []NextCandidate {
	var out []NextCandidate
	for _, doc := range t.Baseline.Docs {
		if doc.Confirmed != nil {
			continue
		}
		out = append(out, NextCandidate{
			Actor: actorHuman, Kind: "confirm_design", Action: "confirm design doc " + doc.Path,
			Command: "tiller approve design " + doc.Path, Reason: baselineReviewReason,
		})
	}
	return inArm(armBaseline, out)
}

// continueCandidates are the reader's own unfinished work: checked tasks,
// then in-progress and claimed ready tasks, then a task in flight on the
// reader's own branch before its claim reached this checkout, then the
// reader's shelf work. Own work in flight on a branch carries that branch
// and no command or skill: it continues there, not from this checkout.
func continueCandidates(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate {
	if in.Reader == "" {
		return nil
	}
	var out []NextCandidate
	tasks := orderedTasks(t, order)
	for _, checked := range []bool{true, false} {
		for _, task := range ownedExecutionCandidates(t, order, tasks, in.Reader, checked, in.Workflow) {
			if observed, ok := observedTask(task, in.Workflow); ok {
				if observed.readersOwn(in.Reader) {
					out = append(out, observedCandidate(t, observed))
				}
				continue
			}
			out = append(out, continueCandidate(t, task))
		}
	}
	for _, task := range gen.UnclaimedTasks(t, order, in.Workflow.Eligibility) {
		if observed, ok := observedTask(task, in.Workflow); ok && observed.readersOwn(in.Reader) {
			out = append(out, observedCandidate(t, observed))
		}
	}
	for _, work := range routableShelfNodes(t, in.Reader) {
		out = append(out, continueCandidate(t, work))
	}
	return inArm(armContinue, out)
}

// continueCandidate and observedCandidate leave Reason empty: every continue
// move ranks in tier 2, where the ranker writes its reason (continueReason).
func continueCandidate(t *store.Tree, node store.Node) NextCandidate {
	return NextCandidate{
		Actor: actorAgent, Kind: "continue", Action: "continue",
		Node: nextNodeOf(t, node), Skill: "execute",
	}
}

func observedCandidate(t *store.Tree, observed ObservedTask) NextCandidate {
	return NextCandidate{
		Actor: actorAgent, Kind: "continue", Action: "continue",
		Node: nextNodeOf(t, observed.Task), Branch: observed.Branch,
	}
}

// queueCandidates maps every queue entry to the move it asks for, keeping
// its kind and command, in queue order within each arm.
func queueCandidates(t *store.Tree, in NextInput) []NextCandidate {
	queueIn := in.Queue
	queueIn.Workflow = in.Workflow
	var out []NextCandidate
	for pos, entry := range QueueOfInput(t, queueIn).Entries {
		arm, ok := queueKindArms[entry.Kind]
		if !ok {
			continue
		}
		c := NextCandidate{
			Actor: entry.Actor, Kind: entry.Kind, Action: queueKindActions[entry.Kind],
			Node:    &NextNode{Noun: entry.Noun, ID: entry.ID, Key: entry.Key, Ref: entry.Ref},
			Command: entry.Command, Reason: queueEntryReason(entry, in.Reader),
			arm: arm, pos: pos,
		}
		if c.Actor == "" {
			c.Actor = actorHuman
		}
		node := gen.NodeByID(t, entry.ID)
		if strings.HasPrefix(entry.Kind, "approve_") {
			c.Node.Title = entry.Prose
		} else if node != nil {
			c.Node.Title = nodeTitle(node)
		}
		if q, ok := node.(*store.Question); ok {
			c.due = q.Due.Date
		}
		if (arm == armCloseout || arm == armReviewMerge) && node != nil {
			c.Workflow = workflow.AdviceOf(t, node, in.Workflow)
			if reason := closeoutReason(entry.Kind, node, c.Workflow, in.Reader); reason != "" {
				c.Reason = reason
			}
		}
		if arm == armCloseout {
			if c.Workflow != nil {
				c.Action = strings.ReplaceAll(c.Workflow.Action, "_", " ")
			}
			if c.Actor == actorAgent {
				c.Skill = "execute"
			}
		}
		out = append(out, c)
	}
	return out
}

// queueEntryReason says why each approval waits and keeps the queue's own
// prose for every other entry, never restating the node the line already
// names. With no reader, nothing is addressed to "your" approval (ranked-next
// Review Focus 1).
func queueEntryReason(entry QueueEntry, reader string) string {
	approval := "your approval"
	if reader == "" {
		approval = "approval"
	}
	switch entry.Kind {
	case "approve_amendment":
		return "its staged changes await " + approval
	case "approve_feature":
		return "its feature plan awaits " + approval
	case "approve_epic":
		return "its decomposition awaits " + approval
	}
	return withoutLeadingRef(entry.Prose, entry.Ref)
}

// closeoutReason keeps a closeout or aggregate merge row to one line:
// the closeout's phase, or what the merge waits on.
// The Needs you card keeps the advice's full reason. Empty means no advice,
// so the queue's own prose stands.
func closeoutReason(kind string, node store.Node, advice *workflow.Advice, reader string) string {
	if kind == "merge_feature" || kind == "merge_epic" {
		pr, ref := reviewSurfaceOf(node)
		return mergeWaitReason("", pr, ref, reader)
	}
	if advice == nil {
		return ""
	}
	phase := strings.ReplaceAll(advice.Phase, "_", " ")
	if advice.RecoveredReview != nil {
		return "shared closeout of a recovered review: " + phase
	}
	return "shared closeout: " + phase
}

// reviewSurfaceOf is a reviewed feature's or epic's own recorded PR and
// review ref.
func reviewSurfaceOf(node store.Node) (*int, *store.ReviewRef) {
	switch n := node.(type) {
	case *store.Feature:
		return n.PR, n.ReviewRef
	case *store.Epic:
		return n.PR, n.ReviewRef
	}
	return nil, nil
}

// withoutLeadingRef drops a reason's opening reference to the node its line
// already names.
func withoutLeadingRef(reason, ref string) string {
	if ref == "" || !strings.HasPrefix(reason, ref) {
		return reason
	}
	return strings.TrimLeft(strings.TrimPrefix(reason, ref), " ;:,")
}

// claimCandidates offers every eligible unclaimed task not already in
// flight on a branch, in execution preference order.
func claimCandidates(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate {
	var out []NextCandidate
	for _, task := range gen.UnclaimedTasks(t, order, in.Workflow.Eligibility) {
		if _, observed := observedTask(task, in.Workflow); observed {
			continue
		}
		out = append(out, NextCandidate{
			Actor: actorAgent, Kind: "claim_task", Action: "claim and start",
			Node: nextNodeOf(t, task), Command: "tiller claim task " + queueAddr(task),
			Skill: "execute", Reason: "ready and unclaimed; once claimed, start it with `tiller start task " + queueAddr(task) + "`",
		})
	}
	return inArm(armClaim, out)
}

// blockerCandidates names each live feature holding the reader's own work
// on an observed eligibility failure. A feature an open question holds is
// left to that question's candidate.
func blockerCandidates(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate {
	var out []NextCandidate
	for _, feature := range flowFeatures(t, order) {
		problem := ownedFeatureProblem(t, feature, in.Reader, in.Workflow)
		if problem == "" || len(gen.FeatureQuestionBlockers(t, feature)) > 0 {
			continue
		}
		advice := blockedFeatureAdvice(t, feature, problem, in.Workflow)
		out = append(out, NextCandidate{
			Actor: actorHuman, Kind: "resolve_blocker", Action: "resolve the blocker on",
			Node: nextNodeOf(t, feature), Reason: withoutLeadingRef(advice.Reason, gen.Ref(t, feature)), Workflow: advice,
		})
	}
	return inArm(armBlocker, out)
}

// specifyCandidates offers every frontier draft, in frontier order.
func specifyCandidates(t *store.Tree, order *gen.Order) []NextCandidate {
	var out []NextCandidate
	for _, feature := range frontierFeatures(t, order) {
		out = append(out, NextCandidate{
			Actor: actorAgent, Kind: "specify", Action: "specify",
			Node: nextNodeOf(t, feature), Skill: "specify",
			Reason: "draft, with every dependency met",
		})
	}
	return inArm(armSpecify, out)
}

// mergeWaitCandidates are the in-review tasks and shelf work waiting on a
// human merge. A coarse-grain task whose reviewed node has recorded no
// review surface yet is no one's merge.
func mergeWaitCandidates(t *store.Tree, order *gen.Order, in NextInput) []NextCandidate {
	var out []NextCandidate
	for _, task := range orderedTasks(t, order) {
		if task.Status != "in_review" {
			continue
		}
		if !inReviewHumanWait(t, task) {
			continue
		}
		c := mergeCandidate(t, task, taskMergeWaitReason(t, task, in.Reader))
		c.Workflow = workflow.AdviceOf(t, task, in.Workflow)
		out = append(out, c)
	}
	for _, work := range shelfMergeWaits(t) {
		out = append(out, mergeCandidate(t, work, mergeWaitReason("", work.Body().PR, work.Body().ReviewRef, in.Reader)))
	}
	return inArm(armTaskMerge, out)
}

// taskMergeWaitReason says what an in-review task's merge waits on: its own
// PR, or the coarse feature or epic review it rides, named by that review's
// node. inReviewHumanWait has already established that the wait is real.
func taskMergeWaitReason(t *store.Tree, task *store.Task, reader string) string {
	if task.PR != nil {
		return mergeWaitReason("", task.PR, nil, reader)
	}
	if feature := t.Features[task.FeatureID]; feature != nil {
		epic := recapEpicByKey(t, feature.EpicKey)
		switch grain, _ := store.TaskReviewGrain(t.Manifest, epic, feature, task); grain {
		case store.ReviewGrainFeature:
			return mergeWaitReason(gen.Ref(t, feature), feature.PR, feature.ReviewRef, reader)
		case store.ReviewGrainEpic:
			if epic != nil {
				return mergeWaitReason(gen.Ref(t, epic), epic.PR, epic.ReviewRef, reader)
			}
		}
	}
	return mergeWaitReason("", nil, task.ReviewRef, reader)
}

// mergeWaitReason words a merge wait: in review on its own (rides empty) or
// riding another node's review, with the PR number when one is recorded and
// the review branch when only a review ref is. With no reader the merge is
// no one's in particular, so it is never "your" merge. The line's own
// command is `tiller sync`, so the reason never repeats it.
func mergeWaitReason(rides string, pr *int, ref *store.ReviewRef, reader string) string {
	prefix := ""
	if rides != "" {
		prefix = "rides the review of " + rides + "; "
	}
	merge := "your merge"
	if reader == "" {
		merge = "merge"
	}
	switch {
	case pr != nil:
		return prefix + fmt.Sprintf("PR #%d awaits %s", *pr, merge)
	case ref != nil && ref.Branch != "":
		return prefix + "in review on branch " + ref.Branch + ", waiting on " + merge
	}
	return prefix + "in review, waiting on " + merge
}

func mergeCandidate(t *store.Tree, node store.Node, reason string) NextCandidate {
	return NextCandidate{
		Actor: actorHuman, Kind: "merge_task", Action: "merge",
		Node: nextNodeOf(t, node), Command: "tiller sync", Reason: reason,
	}
}

// decomposeCandidates offers decompose on a tree with no epics yet and a
// to-be design doc to decompose.
func decomposeCandidates(t *store.Tree) []NextCandidate {
	if len(t.Epics) > 0 || len(t.ToBeDesign) == 0 {
		return nil
	}
	return inArm(armDecompose, []NextCandidate{{
		Actor: actorAgent, Kind: "decompose", Action: "decompose",
		Skill: "decompose", Reason: decomposeReason(t),
	}})
}

func nextNodeOf(t *store.Tree, n store.Node) *NextNode {
	return &NextNode{Noun: n.Noun(), ID: n.NodeID(), Key: Key(n), Ref: gen.Ref(t, n), Title: nodeTitle(n)}
}

// nodeTitle is a node's one-line human name.
func nodeTitle(n store.Node) string {
	switch v := n.(type) {
	case *store.Epic:
		return v.Title
	case *store.Question:
		return gen.QuestionTitle(v)
	case *store.Amendment:
		return v.Title
	}
	return nodeProse(n)
}
