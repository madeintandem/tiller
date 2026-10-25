package gen

import (
	"sort"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// DependencyEdge is a recorded depends_on assertion, never a projected edge.
type DependencyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ExecutionCycle names a loop in execution prerequisites and completion
// obligations. Noun is mixed when a loop spans epic and feature completion.
type ExecutionCycle struct {
	Noun        string           `json:"noun"`
	Members     []string         `json:"members"`
	Edges       []DependencyEdge `json:"edges"`
	ReviewEpics []string         `json:"review_epics,omitempty"`
}

// ProjectedCycle is a grouping limitation, not an execution deadlock.
type ProjectedCycle struct {
	Members []string         `json:"members"`
	Edges   []DependencyEdge `json:"edges"`
}

// CycleClassification separates execution barriers from grouped-map limitations.
type CycleClassification struct {
	Blocking []ExecutionCycle `json:"blocking"`
	Advisory []ProjectedCycle `json:"advisory"`
}

// ClassifyCycles uses taskStartBase prerequisites, rollup completion, and
// submitEpic/reviewedAggregate review obligations. Parent approval is not
// parent completion. Aggregate submission is not rider completion: sibling
// work may be assembled before the shared human merge. Done/archived recorded
// assertions retain their historical diagnostic treatment; cancellation
// removes work. No recorded edge or rendering order is changed.
func ClassifyCycles(t *store.Tree) CycleClassification {
	current := currentDependencyTree(t)
	nodes := make([]store.Node, 0, len(current.Epics)+len(current.Features)+len(t.Tasks))
	byID := map[string]store.Node{}
	epicByKey := map[string]string{}
	featuresByEpic := map[string][]*store.Feature{}
	addNode := func(n store.Node) {
		if byID[n.NodeID()] == nil {
			byID[n.NodeID()] = n
			nodes = append(nodes, n)
		}
	}
	for id, e := range current.Epics {
		addNode(e)
		if e.Key != nil {
			epicByKey[*e.Key] = id
		}
	}
	for id, f := range current.Features {
		addNode(f)
		featuresByEpic[f.EpicKey] = append(featuresByEpic[f.EpicKey], f)
		for _, task := range current.FeatureTasks[id] {
			addNode(task)
		}
	}
	index := relationships.BuildDependencies(t, nodes)
	deps := make(map[string][]string, len(nodes))
	recorded := make(map[string][]string, len(nodes))
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		id := n.NodeID()
		ids = append(ids, id)
		for _, target := range dependencyTargets(index, id) {
			if byID[target] != nil {
				deps[id] = append(deps[id], target)
				recorded[id] = append(recorded[id], target)
			}
		}
	}
	for id, f := range current.Features {
		if epic := epicByKey[f.EpicKey]; epic != "" {
			deps[epic] = append(deps[epic], id)
			deps[id] = append(deps[id], recorded[epic]...)
		}
		for _, task := range current.FeatureTasks[id] {
			deps[id] = append(deps[id], task.ID)
		}
	}
	// A riding feature cannot finish before its epic aggregate merges. That
	// review requires every riding feature's tasks submitted, but independent
	// overrides must be done (submitEpic). Separate submission vertices avoid
	// inventing a mutual completion requirement between independent riders.
	reviewEpics := map[string]string{}
	for id, f := range current.Features {
		epicID := epicByKey[f.EpicKey]
		epic := current.Epics[epicID]
		if epic == nil || f.Status == "done" || store.ResolvedReviewGrain(t.Manifest, epic, f) != store.ReviewGrainEpic {
			continue
		}
		reviewID := "review:" + epicID
		if byID[reviewID] == nil {
			byID[reviewID] = epic
			ids = append(ids, reviewID)
			reviewEpics[reviewID] = epicID
		}
		deps[id] = append(deps[id], reviewID)
	}
	for reviewID, epicID := range reviewEpics {
		epic := current.Epics[epicID]
		for _, f := range featuresByEpic[keyStr(epic.Key)] {
			if f.Status == "done" {
				continue
			}
			id := f.ID
			if store.ResolvedReviewGrain(t.Manifest, epic, f) != store.ReviewGrainEpic {
				deps[reviewID] = append(deps[reviewID], id)
				continue
			}
			submissionID := "submission:" + id
			byID[submissionID] = f
			ids = append(ids, submissionID)
			deps[reviewID] = append(deps[reviewID], submissionID)
			deps[submissionID] = append(deps[submissionID], recorded[id]...)
			deps[submissionID] = append(deps[submissionID], recorded[epicID]...)
			recorded[submissionID] = recorded[id]
		}
	}
	keyOf := func(id string) string { return keyStr(byID[id].NodeKey()) }
	_, groups := layeredTopoOrderTied(ids, deps, keyOf, nil, nil)
	// Rendering has no degraded order for a singleton, so its SCC helper does
	// not report self loops. Inherited parent prerequisites can create one:
	// an epic depending on its own child prevents that child from executing.
	grouped := make(map[string]bool)
	for _, group := range groups {
		for _, id := range group {
			grouped[id] = true
		}
	}
	for _, id := range ids {
		if grouped[id] {
			continue
		}
		for _, target := range deps[id] {
			if id == target {
				groups = append(groups, []string{id})
				break
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool { return keyTie(keyOf)(groups[i][0], groups[j][0]) })
	result := CycleClassification{Blocking: []ExecutionCycle{}, Advisory: []ProjectedCycle{}}
	blockingEpics := map[string]bool{}
	epicOf := func(id string) string {
		if current.Epics[id] != nil {
			return id
		}
		if f := current.Features[id]; f != nil {
			return epicByKey[f.EpicKey]
		}
		return ""
	}
	for _, group := range groups {
		members := make(map[string]bool, len(group))
		groupEpics := map[string]bool{}
		memberIDs := map[string]bool{}
		var reviews []string
		noun := byID[group[0]].Noun()
		for _, id := range group {
			members[id] = true
			if byID[id].Noun() != noun {
				noun = "mixed"
			}
			nodeID := byID[id].NodeID()
			memberIDs[nodeID] = true
			if epic := epicOf(nodeID); epic != "" {
				groupEpics[epic] = true
			}
			if epic := reviewEpics[id]; epic != "" {
				reviews = append(reviews, epic)
			}
		}
		// An unrelated within-epic blocker does not turn an otherwise valid
		// cross-epic interleaving into a container prerequisite cycle.
		if len(groupEpics) > 1 {
			for epic := range groupEpics {
				blockingEpics[epic] = true
			}
		}
		edges := map[DependencyEdge]bool{}
		for _, id := range group {
			for _, target := range recorded[id] {
				if members[target] {
					edges[DependencyEdge{byID[id].NodeID(), target}] = true
				}
			}
			if f, ok := byID[id].(*store.Feature); ok {
				epic := epicByKey[f.EpicKey]
				for _, target := range recorded[epic] {
					if members[target] {
						edges[DependencyEdge{epic, target}] = true
					}
				}
			}
		}
		publishedMembers := make([]string, 0, len(memberIDs))
		for id := range memberIDs {
			publishedMembers = append(publishedMembers, id)
		}
		sort.Slice(publishedMembers, func(i, j int) bool { return keyTie(keyOf)(publishedMembers[i], publishedMembers[j]) })
		sort.Strings(reviews)
		result.Blocking = append(result.Blocking, ExecutionCycle{Noun: noun, Members: publishedMembers, Edges: sortedDependencyEdges(edges), ReviewEpics: reviews})
	}
	for _, group := range dependencyOrder(current, t).Cycles() {
		if group.Noun != store.NounEpic {
			continue
		}
		members := make(map[string]bool, len(group.Members))
		blocked := false
		for _, id := range group.Members {
			members[id] = true
			blocked = blocked || blockingEpics[id]
		}
		if blocked {
			continue
		}
		edges := map[DependencyEdge]bool{}
		for source, targets := range recorded {
			for _, target := range targets {
				a, b := epicOf(source), epicOf(target)
				if a != b && members[a] && members[b] {
					edges[DependencyEdge{source, target}] = true
				}
			}
		}
		result.Advisory = append(result.Advisory, ProjectedCycle{Members: group.Members, Edges: sortedDependencyEdges(edges)})
	}
	return result
}

func sortedDependencyEdges(set map[DependencyEdge]bool) []DependencyEdge {
	edges := make([]DependencyEdge, 0, len(set))
	for edge := range set {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Target < edges[j].Target
	})
	return edges
}
