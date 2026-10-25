package gen

import (
	"sort"

	"github.com/madeintandem/tiller/internal/store"
)

// QuestionOpen reports whether a question still needs an answer.
func QuestionOpen(q *store.Question) bool {
	switch q.Status {
	case "answered", "resolved", "cancelled":
		return false
	default:
		return true
	}
}

// QuestionDue renders the most specific due hint a question carries.
func QuestionDue(q *store.Question) string {
	switch {
	case q.Due.Date != "":
		return q.Due.Date
	case q.Due.Hint != "":
		return q.Due.Hint
	default:
		return ""
	}
}

// QuestionBlocker is one open question that holds a node's work. TargetID and
// TargetNoun identify the nearest canonical node matched by the question while
// Blocks preserves the recorded identifiers for structured detail.
type QuestionBlocker struct {
	Question   string
	Title      string
	Statement  string
	Blocks     []string
	TargetID   string
	TargetNoun string
}

type questionBlockerTarget struct {
	id   string
	key  string
	noun string
}

// EpicQuestionBlockers gathers open questions that name the epic by canonical
// ID or legacy key.
func EpicQuestionBlockers(tree *store.Tree, epic *store.Epic) []QuestionBlocker {
	if epic == nil {
		return nil
	}
	return questionBlockers(tree, []questionBlockerTarget{blockerTarget(epic)}, nil)
}

// FeatureQuestionBlockers gathers open questions that name the feature or its
// epic ancestor. A feature's assumption basis clears the corresponding hold.
func FeatureQuestionBlockers(tree *store.Tree, feature *store.Feature) []QuestionBlocker {
	if feature == nil {
		return nil
	}
	targets := appendFeatureQuestionTargets(make([]questionBlockerTarget, 0, 2), tree, feature)
	return questionBlockers(tree, targets, feature.AssumptionBasis)
}

// TaskQuestionBlockers gathers open questions that name the task or its feature
// and epic ancestors. Tasks do not inherit the feature's assumption basis.
func TaskQuestionBlockers(tree *store.Tree, feature *store.Feature, task *store.Task) []QuestionBlocker {
	if task == nil || feature == nil {
		return nil
	}
	targets := make([]questionBlockerTarget, 0, 3)
	targets = append(targets, blockerTarget(task))
	targets = appendFeatureQuestionTargets(targets, tree, feature)
	return questionBlockers(tree, targets, nil)
}

// PlanQuestionBlockers gathers the open questions that hold a feature's plan
// at its plan gate: the union of the feature's own blockers (its epic included)
// and the blockers of each of its tasks that is not cancelled, deduplicated by
// question id in first-seen order so a question naming two tasks reads once.
// A cancelled task is retired work, so a question pinned only to it holds
// nothing (issue #152, decisions 1 and 7). Any feature assumption is moot here:
// a feature returned from ready to specced can still carry an assumption basis,
// which drops the question from the feature's own blockers, but task blockers
// ignore assumptions and name the feature as an ancestor, so an assumed
// feature question still holds the plan through its tasks.
func PlanQuestionBlockers(tree *store.Tree, feature *store.Feature) []QuestionBlocker {
	if tree == nil || feature == nil {
		return nil
	}
	var blockers []QuestionBlocker
	seen := map[string]bool{}
	add := func(found []QuestionBlocker) {
		for _, blocker := range found {
			if seen[blocker.Question] {
				continue
			}
			seen[blocker.Question] = true
			blockers = append(blockers, blocker)
		}
	}
	add(FeatureQuestionBlockers(tree, feature))
	for _, task := range tree.FeatureTasks[feature.ID] {
		if task == nil || task.Status == "cancelled" {
			continue
		}
		add(TaskQuestionBlockers(tree, feature, task))
	}
	return blockers
}

// DirectQuestionHolds counts the open questions that name this node itself,
// by canonical ID or by legacy key. It answers a narrower question than the
// blockers helpers above: those report what holds a node's WORK, which a
// question on an ancestor does, so one epic-level question holds the epic,
// every feature under it, and every task under those. A per-card count wants
// the questions asked about that card, or a single question on an epic reads
// as dozens of open questions across the map.
func DirectQuestionHolds(tree *store.Tree, node store.Node) int {
	if tree == nil || node == nil {
		return 0
	}
	target := blockerTarget(node)
	held := 0
	for _, question := range tree.Questions {
		if question == nil || !QuestionOpen(question) {
			continue
		}
		if _, matched := nearestQuestionTarget(question.Blocks, []questionBlockerTarget{target}); matched {
			held++
		}
	}
	return held
}

func appendFeatureQuestionTargets(targets []questionBlockerTarget, tree *store.Tree, feature *store.Feature) []questionBlockerTarget {
	targets = append(targets, blockerTarget(feature))
	if tree != nil {
		for _, epic := range tree.Epics {
			if epic.Key != nil && *epic.Key == feature.EpicKey {
				targets = append(targets, blockerTarget(epic))
				break
			}
		}
	}
	return targets
}

func blockerTarget(node store.Node) questionBlockerTarget {
	target := questionBlockerTarget{id: node.NodeID(), noun: node.Noun()}
	if key := node.NodeKey(); key != nil {
		target.key = *key
	}
	return target
}

func questionBlockers(tree *store.Tree, targets []questionBlockerTarget, assumed store.AssumptionBasis) []QuestionBlocker {
	if tree == nil {
		return nil
	}
	blockers := make([]QuestionBlocker, 0)
	for _, question := range tree.Questions {
		if question == nil || !QuestionOpen(question) {
			continue
		}
		if _, ok := assumed[question.ID]; ok {
			continue
		}
		target, matched := nearestQuestionTarget(question.Blocks, targets)
		if !matched {
			continue
		}
		blockers = append(blockers, QuestionBlocker{
			Question:   question.ID,
			Title:      QuestionTitle(question),
			Statement:  question.Statement,
			Blocks:     question.Blocks,
			TargetID:   target.id,
			TargetNoun: target.noun,
		})
	}
	sort.Slice(blockers, func(i, j int) bool { return blockers[i].Question < blockers[j].Question })
	return blockers
}

// nearestQuestionTarget tests task, feature, then epic targets, so one question
// naming several levels is reported once at the closest canonical node.
func nearestQuestionTarget(blocks []string, targets []questionBlockerTarget) (questionBlockerTarget, bool) {
	for _, target := range targets {
		for _, block := range blocks {
			if block == target.id || target.key != "" && block == target.key {
				return target, true
			}
		}
	}
	return questionBlockerTarget{}, false
}
