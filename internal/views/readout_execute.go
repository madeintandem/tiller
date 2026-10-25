package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// executeContent reports recorded execution state, not whether invoking a
// readout completed a handoff. Git publication facts come only from the caller.
func executeContent(t *store.Tree, in ReadoutInput, order *gen.Order, r *Readout) {
	tasks := orderedTasks(t, order)
	var shelf []store.WorkNode
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			if gen.WorkActivity(t, work) != gen.ActivityCarried {
				shelf = append(shelf, work)
			}
		}
	}

	total, green, submitted := 0, 0, 0
	add := func(node store.Node, gates map[string]store.Gate, pr *int, ref *store.ReviewRef, branch string) {
		total++
		verified := gatesGreenWithEvidence(gates)
		if verified {
			green++
		}
		recorded := workflow.HasRecordedReview(pr, ref)
		current := node.NodeStatus() == "in_review" && recorded
		if current {
			submitted++
		}
		if len(r.Entities) >= entityCap {
			r.Withheld++
			return
		}
		var surface string
		if task, ok := node.(*store.Task); ok {
			surface = executeTaskReviewNote(t, task)
		} else {
			surface = executeReviewNote(pr, ref)
		}
		r.Entities = append(r.Entities, ReadoutEntity{
			Ref:      gen.Ref(t, node),
			Note:     executeStateNote(node.NodeStatus(), verified, current, recorded, surface),
			Unpushed: unpushedReviewEntry(node.NodeID(), branch, in.UnpushedReviews),
		})
	}

	// Priority is project-wide, not owner-scoped. Within each phase retain
	// dependency/soft task order, then bug-before-chore shelf order.
	for _, status := range []string{"check", "in_progress", "in_review"} {
		for _, task := range tasks {
			if task.Status != status {
				continue
			}
			add(task, task.Gates, task.PR, task.ReviewRef, task.Branch)
		}
		for _, work := range shelf {
			body := work.Body()
			if body.Status != status {
				continue
			}
			add(work, body.Gates, body.PR, body.ReviewRef, body.Branch)
		}
	}

	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Gates green with evidence", Result: fmt.Sprintf("%d of %d", green, total)},
		ReadoutCheck{Label: "Current submissions recorded", Result: fmt.Sprintf("%d of %d", submitted, total)},
	)

	// Resolved reviews are an uncapped summary, separate from task/shelf counts.
	// A riding reference never proves feature acceptance or formal submission.
	seen := map[string]bool{}
	for _, f := range workflow.OrderedFeatures(t, order) {
		if f.Status != "active" && f.Status != "done" {
			continue
		}
		advice := workflow.AdviceOf(t, f, in.Workflow)
		if advice == nil {
			continue
		}
		if advice.ReviewID != f.ID {
			if epic := t.Epics[advice.ReviewID]; epic != nil {
				if resolved := workflow.EpicAdvice(t, order, epic, in.Workflow); resolved != nil {
					advice = resolved
				}
			}
		}
		if seen[advice.ReviewID] {
			continue
		}
		seen[advice.ReviewID] = true
		if advice.Phase == "done" && advice.AcceptanceReady && advice.HistoricalViolations == 0 && advice.RecoveredReview == nil {
			continue
		}
		result := advice.Reason
		if advice.Command != "" {
			result += "; `" + advice.Command + "`"
		}
		r.Checks = append(r.Checks, ReadoutCheck{Label: "Resolved review " + gen.RefByID(t, advice.ReviewID), Result: result})
	}
}

func executeStateNote(status string, green, current, recorded bool, surface string) string {
	note := status + ": "
	if current {
		note += "current submission recorded"
	} else if green {
		note += "recorded gates green with evidence; awaiting submission"
	} else {
		note += "required checks incomplete; not currently submitted"
	}
	if current && !green {
		note += "; required checks incomplete"
	}
	if recorded && !current {
		note += "; retained review: " + surface
	} else {
		note += "; " + surface
	}
	return note
}

// A riding task records its own ref, not a task PR. Name both that ref and the
// resolved reviewed node, without inventing a PR before the parent records one.
// A historical task PR remains authoritative, matching inReviewHumanWait, and
// a task started at task grain rides nothing (issue #146).
func executeTaskReviewNote(t *store.Tree, task *store.Task) string {
	note := executeReviewNote(task.PR, task.ReviewRef)
	if task.PR != nil {
		return note
	}
	feature := t.Features[task.FeatureID]
	if feature == nil {
		return note
	}
	epic := recapEpicByKey(t, feature.EpicKey)
	grain, _ := store.TaskReviewGrain(t.Manifest, epic, feature, task)
	switch grain {
	case store.ReviewGrainFeature:
		return note + "; riding feature review " + gen.Ref(t, feature) + ": " + executeReviewNote(feature.PR, feature.ReviewRef)
	case store.ReviewGrainEpic:
		if epic != nil {
			return note + "; riding epic review " + gen.Ref(t, epic) + ": " + executeReviewNote(epic.PR, epic.ReviewRef)
		}
	}
	return note
}

func executeReviewNote(pr *int, ref *store.ReviewRef) string {
	if pr != nil {
		return fmt.Sprintf("PR #%d", *pr)
	}
	if ref != nil && ref.Branch != "" {
		if ref.Head.Value != "" {
			return fmt.Sprintf("branch %s at %s", ref.Branch, ref.Head.Value)
		}
		return "branch " + ref.Branch
	}
	return "no review ref recorded"
}

// Match supplied publication facts only; a stored PR does not prove a push.
func unpushedReviewEntry(nodeID, branch string, reviews []UnpushedReview) *UnpushedReviewView {
	if branch == "" {
		return nil
	}
	for _, review := range reviews {
		if review.Node == nodeID {
			return &UnpushedReviewView{Commit: review.Commit, Branch: branch, Upstream: review.Upstream}
		}
	}
	return nil
}
