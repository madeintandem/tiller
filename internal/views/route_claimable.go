package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// EligibleTask combines strict pool eligibility with observations collected at
// the workflow input boundary. A completed parent is never offered by the pool.
func EligibleTask(t *store.Tree, task *store.Task, in workflow.Input) bool {
	return gen.EligibleTask(t, task) && in.Eligibility[task.ID] == "" && in.Eligibility[task.FeatureID] == ""
}

// assignedTaskEligible also permits explicitly claimed task-grain continuation
// beneath completed rollups, without relaxing other start guards.
func assignedTaskEligible(t *store.Tree, task *store.Task, in workflow.Input) bool {
	return gen.AssignedTaskEligible(t, task) && in.Eligibility[task.ID] == "" && in.Eligibility[task.FeatureID] == ""
}

// A submitted coarse prerequisite can be assembled before starting its dependent.
// Start verifies Git inclusion; this pure hint authorizes execute to assemble it.
func unmetTaskDependency(t *store.Tree, f *store.Feature, task *store.Task) string {
	epic := recapEpicByKey(t, f.EpicKey)
	coarse := store.ResolvedReviewGrain(t.Manifest, epic, f) != store.ReviewGrainTask
	for _, id := range task.DependsOn {
		dep := t.Tasks[id]
		if dep == nil {
			return id
		}
		if dep.Status == "done" || dep.Status == "cancelled" {
			continue
		}
		depGrain, _ := store.TaskReviewGrain(t.Manifest, epic, f, dep)
		if coarse && depGrain != store.ReviewGrainTask && dep.FeatureID == f.ID && dep.Status == "in_review" && dep.ReviewRef != nil {
			continue
		}
		return gen.Ref(t, dep)
	}
	return ""
}

// readersOwn reports whether the observed owner is the reader. An unset
// observed owner is never the reader's, so it is skipped and never mutes.
func (o ObservedTask) readersOwn(owner string) bool {
	return owner != "" && o.Owner == owner
}

// observedTask reports a task's execution observation, when it has one.
func observedTask(task *store.Task, in workflow.Input) (ObservedTask, bool) {
	observation, ok := in.Observed[task.ID]
	if !ok {
		return ObservedTask{}, false
	}
	return ObservedTask{Task: task, Branch: observation.Branch, Owner: observation.Owner}, true
}

func ownedExecutionCandidates(t *store.Tree, order *gen.Order, tasks []*store.Task, owner string, checked bool, in workflow.Input) []*store.Task {
	var candidates []*store.Task
	for _, task := range tasks {
		if (task.Status == "check") != checked {
			continue
		}
		feature, eligible := routeTaskParent(t, task, owner)
		if !eligible {
			continue
		}
		// A question-held task is no move; its question is.
		if len(gen.TaskQuestionBlockers(t, feature, task)) > 0 ||
			in.Eligibility[task.ID] != "" || in.Eligibility[feature.ID] != "" ||
			feature.Status == "cancelled" || gen.RetiredAncestor(t, task) != nil || task.Blocked || feature.Blocked {
			continue
		}
		if task.Status == "ready" && !assignedTaskEligible(t, task, in) {
			continue
		}
		candidates = append(candidates, task)
	}
	gen.SortExecutionTasks(t, order, candidates)
	return candidates
}

// PoolSignpost explains held unclaimed execution work without recommending a
// command that the routing guards refuse. The executable pool itself contains
// only eligible ready tasks; this diagnostic also keeps held work discoverable.
// A task observed in flight (in.Observed) is not held: it is a
// move on its owner's branch, so it stays out of the count, and with nothing
// else left the signpost is empty and the caller says nothing is due.
func PoolSignpost(t *store.Tree, order *gen.Order, in workflow.Input) string {
	var first *store.Task
	count := 0
	for _, task := range orderedTasks(t, order) {
		if _, observed := observedTask(task, in); observed {
			continue
		}
		if task.Owner == "" && gen.UnfinishedAssignment(task.Status) {
			count++
			if first == nil {
				first = task
			}
		}
	}
	if first == nil {
		return ""
	}
	why := unclaimedTaskWhy(t, first)
	return fmt.Sprintf("There %s %d %s; the first in dependency order, %s, %s", plural(count, "is", "are"), count,
		plural(count, "unclaimed task", "unclaimed tasks"), gen.Ref(t, first), why)
}

// unclaimedTaskWhy explains why an unfinished unclaimed task is not offered.
func unclaimedTaskWhy(t *store.Tree, task *store.Task) string {
	why := "is " + task.Status
	if f := t.Features[task.FeatureID]; f != nil {
		epic := recapEpicByKey(t, f.EpicKey)
		switch {
		case gen.RetiredAncestor(t, task) != nil:
			why = "is held under a retired ancestor"
		case task.Blocked || f.Blocked:
			why = "is manually blocked"
		case epic == nil || epic.Status == "draft":
			why = "waits on its epic's approval"
		case f.Status != "ready" && f.Status != "active":
			why = "waits on its feature's approval"
		default:
			if dependency := workflow.UnmetDependency(t, f, epic); dependency != "" {
				why = "is blocked by " + dependency
			} else if dependency := unmetTaskDependency(t, f, task); dependency != "" {
				why = "is blocked by " + dependency
			} else if blockers := gen.TaskQuestionBlockers(t, f, task); len(blockers) > 0 {
				why = "is held by open question " + blockers[0].Question
			} else if task.Status == "ready" {
				why = "awaits a valid accepted basis and start eligibility"
			}
		}
	}
	return why
}
