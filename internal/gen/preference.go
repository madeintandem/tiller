package gen

import (
	"math"
	"sort"

	"github.com/madeintandem/tiller/internal/store"
)

// EligibleTask reports structural pool eligibility, independently of its owner.
// Completed parents are excluded even when assigned continuation is legal.
// Filesystem-dependent basis checks belong to the workflow-input boundary.
func EligibleTask(t *store.Tree, task *store.Task) bool {
	return eligibleTask(t, task, false) && len(TaskQuestionBlockers(t, t.Features[task.FeatureID], task)) == 0
}

// AssignedTaskEligible permits an explicitly claimed task to continue under
// completed task-grain rollups while retaining the other structural start guards.
func AssignedTaskEligible(t *store.Tree, task *store.Task) bool {
	return task != nil && task.Owner != "" && eligibleTask(t, task, true) && len(TaskQuestionBlockers(t, t.Features[task.FeatureID], task)) == 0
}

func eligibleTask(t *store.Tree, task *store.Task, assigned bool) bool {
	if task == nil || task.Status != "ready" || task.Blocked {
		return false
	}
	f := t.Features[task.FeatureID]
	if f == nil || f.Blocked {
		return false
	}
	var epic *store.Epic
	for _, e := range t.Epics {
		if e.Key != nil && *e.Key == f.EpicKey {
			epic = e
			break
		}
	}
	if epic == nil || epic.Blocked || RetiredAncestor(t, task) != nil {
		return false
	}
	coarse := store.ResolvedReviewGrain(t.Manifest, epic, f) != store.ReviewGrainTask
	allowDone := assigned && !coarse
	if f.Status != "ready" && f.Status != "active" && !(allowDone && f.Status == "done") {
		return false
	}
	if epic.Status != "ready" && !(allowDone && epic.Status == "done") {
		return false
	}
	dependenciesDone := func(ids []string) bool {
		for _, id := range ids {
			if e := t.Epics[id]; e != nil {
				if e.Status != "done" && e.Status != "cancelled" && e.Status != "archived" {
					return false
				}
			} else if prerequisite := t.Features[id]; prerequisite != nil {
				if UnfinishedAssignment(prerequisite.Status) {
					return false
				}
			} else {
				return false
			}
		}
		return true
	}
	if !dependenciesDone(epic.DependsOn) || !dependenciesDone(f.DependsOn) {
		return false
	}
	for _, id := range task.DependsOn {
		dep := t.Tasks[id]
		if dep == nil {
			return false
		}
		if !UnfinishedAssignment(dep.Status) {
			continue
		}
		depGrain, _ := store.TaskReviewGrain(t.Manifest, epic, f, dep)
		if coarse && depGrain != store.ReviewGrainTask && dep.FeatureID == f.ID && dep.Status == ReviewState && dep.ReviewRef != nil {
			continue
		}
		return false
	}
	return true
}

// Preference holds recorded sibling ordinals, not dependency layers. Ordinals
// from different parents must never be compared as global priorities.
type Preference map[string]int

func NewPreference(t *store.Tree) Preference {
	p := make(Preference)
	add := func(ids []string) {
		for i, id := range ids {
			p[id] = i
		}
	}
	add(t.ProjectOrder.Epics)
	for _, epic := range t.Epics {
		add(epic.Order)
	}
	for _, feature := range t.Features {
		add(feature.Order)
	}
	return p
}

// Rank puts every listed sibling before unlisted siblings, including partial lists.
func (p Preference) Rank(id string) int {
	if rank, ok := p[id]; ok {
		return rank
	}
	return math.MaxInt
}

// SortExecutionTasks ranks candidates in place after the caller has applied its
// execution-class guards. Preference is hierarchical: root epics, sibling
// features, then sibling tasks. Historical order resolves each unrecorded tie,
// so a task's local preference never competes with a different feature's tasks.
// DependencyOrder itself remains the unchanged historical rendering authority.
func SortExecutionTasks(t *store.Tree, order *Order, tasks []*store.Task) {
	if len(tasks) < 2 {
		return
	}
	preference := NewPreference(t)
	epics := make(map[string]*store.Epic, len(t.Epics))
	for _, epic := range t.Epics {
		if epic.Key != nil {
			epics[*epic.Key] = epic
		}
	}
	less := func(noun, a, b string) bool {
		if ar, br := preference.Rank(a), preference.Rank(b); ar != br {
			return ar < br
		}
		return order.Less(noun, a, b)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		if a.FeatureID == b.FeatureID {
			return less(store.NounTask, a.ID, b.ID)
		}
		af, bf := t.Features[a.FeatureID], t.Features[b.FeatureID]
		if af != nil && bf != nil {
			if af.EpicKey == bf.EpicKey {
				return less(store.NounFeature, af.ID, bf.ID)
			}
			ae, be := epics[af.EpicKey], epics[bf.EpicKey]
			if ae != nil && be != nil {
				return less(store.NounEpic, ae.ID, be.ID)
			}
		}
		return order.Less(store.NounTask, a.ID, b.ID)
	})
}

// UnclaimedTasks is the shared executable pool for routing and streams.
// Eligibility contains caller-observed basis checks unavailable to pure reads.
func UnclaimedTasks(t *store.Tree, order *Order, eligibility map[string]string) []*store.Task {
	var tasks []*store.Task
	for _, task := range t.Tasks {
		if task.Owner == "" && eligibleTask(t, task, false) && eligibility[task.ID] == "" && eligibility[task.FeatureID] == "" &&
			len(TaskQuestionBlockers(t, t.Features[task.FeatureID], task)) == 0 {
			tasks = append(tasks, task)
		}
	}
	SortExecutionTasks(t, order, tasks)
	return tasks
}
