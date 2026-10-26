package views

import (
	"sort"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Streams reports current execution assignments,
// independent condition rows, review gates, questions, and eligible free tasks.
type Streams struct {
	Contributors []ContributorStream `json:"contributors"`
	Unclaimed    []UnclaimedItem     `json:"unclaimed,omitempty"`
}

// UnclaimedItem is one eligible unclaimed task, not an ancestor responsibility.
type UnclaimedItem struct {
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	Ref        string `json:"ref"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	FeatureID  string `json:"feature_id"`
	FeatureKey string `json:"feature_key,omitempty"`
}

// ContributorStream is one contributor's four-part rollup.
type ContributorStream struct {
	Contributor   ContributorInfo `json:"contributor"`
	Active        []StreamItem    `json:"active"`
	Blocked       []BlockedItem   `json:"blocked"`
	GatesInReview []GatesItem     `json:"gates_in_review"`
	OpenQuestions []QuestionItem  `json:"open_questions"`
}

// StreamItem is one unfinished assignment outside the separate review lens.
type StreamItem struct {
	Noun    string `json:"noun"`
	ID      string `json:"id"`
	Key     string `json:"key,omitempty"`
	Ref     string `json:"ref"`
	Status  string `json:"status"`
	Display string `json:"display"`
}

// BlockedItem is one row on the blocked lens. A manual blocked flag, a retired
// ancestor, and operative open questions are independent condition families;
// each occupies its own item so clearing one condition cannot hide another.
type BlockedItem struct {
	Noun      string   `json:"noun"`
	ID        string   `json:"id"`
	Key       string   `json:"key,omitempty"`
	Ref       string   `json:"ref"`
	DependsOn []string `json:"depends_on,omitempty"`
	// DependsOnRefs carries the same targets as human references.
	DependsOnRefs []string `json:"depends_on_refs,omitempty"`
	// Held is set only on the retired-ancestor condition row.
	Held *HeldView `json:"held,omitempty"`
	// QuestionHolds is set only on the aggregate open-question condition row.
	QuestionHolds []QuestionHoldItem `json:"question_holds,omitempty"`
}

// QuestionHoldItem is the bounded scanning identity of one open question and
// the nearest canonical work target it matched. Statements remain on question
// detail surfaces and are deliberately absent here.
type QuestionHoldItem struct {
	Question   string `json:"question"`
	Title      string `json:"title"`
	TargetID   string `json:"target_id"`
	TargetNoun string `json:"target_noun"`
}

// GatesItem is one in-review node with its recorded gate results.
type GatesItem struct {
	Noun  string              `json:"noun"`
	ID    string              `json:"id"`
	Key   string              `json:"key,omitempty"`
	Ref   string              `json:"ref"`
	Gates map[string]GateView `json:"gates,omitempty"`
}

// QuestionItem is one open question with a short scan title, its full
// statement retained for structured clients, and its due/block hints.
type QuestionItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title,omitempty"`
	Statement string   `json:"statement,omitempty"`
	Due       string   `json:"due,omitempty"`
	Blocks    []string `json:"blocks,omitempty"`
}

// StreamsOf derives the per-contributor rollup from the loaded tree, deterministic
// (owners in id order, a ranked-noun pair by DependencyOrder,
// everything else by reference).
func StreamsOf(t *store.Tree) Streams {
	return StreamsOfInput(t, workflow.Input{})
}

// StreamsOfInput applies observed workflow eligibility to task discovery.
func StreamsOfInput(t *store.Tree, in workflow.Input) Streams {
	contributors := append([]store.Contributor(nil), t.Manifest.Contributors...)
	sort.Slice(contributors, func(i, j int) bool { return contributors[i].ID < contributors[j].ID })

	order := gen.DependencyOrder(t)
	out := Streams{Contributors: make([]ContributorStream, 0, len(contributors)), Unclaimed: UnclaimedOfInput(t, order, in)}
	for _, o := range contributors {
		out.Contributors = append(out.Contributors, contributorStream(t, order, o))
	}
	return out
}

// UnclaimedOf derives structurally eligible unclaimed tasks in shared task order.
func UnclaimedOf(t *store.Tree, order *gen.Order) []UnclaimedItem {
	return UnclaimedOfInput(t, order, workflow.Input{})
}

// UnclaimedOfInput also excludes candidates refused by observed basis checks.
func UnclaimedOfInput(t *store.Tree, order *gen.Order, in workflow.Input) []UnclaimedItem {
	var out []UnclaimedItem
	for _, task := range gen.UnclaimedTasks(t, order, in.Eligibility) {
		f := t.Features[task.FeatureID]
		out = append(out, UnclaimedItem{
			Noun: store.NounTask, ID: task.ID, Key: Key(task), Ref: gen.Ref(t, task),
			Title: task.Title, Status: task.Status, FeatureID: f.ID, FeatureKey: Key(f),
		})
	}
	return out
}

func contributorStream(t *store.Tree, order *gen.Order, o store.Contributor) ContributorStream {
	s := ContributorStream{
		Contributor:   ContributorInfo{ID: o.ID, Name: o.Name},
		Active:        []StreamItem{},
		Blocked:       []BlockedItem{},
		GatesInReview: []GatesItem{},
		OpenQuestions: []QuestionItem{},
	}

	addActive := func(n store.Node, display string) {
		if Owner(n) != o.ID {
			return
		}
		status := n.NodeStatus()
		if !gen.OnActiveLens(status) {
			return
		}
		s.Active = append(s.Active, StreamItem{
			Noun: n.Noun(), ID: n.NodeID(), Key: Key(n), Ref: gen.Ref(t, n),
			Status: status, Display: display,
		})
	}
	for _, tk := range t.Tasks {
		addActive(tk, tk.Status)
	}
	for _, noun := range store.ShelfNouns() {
		for _, w := range t.WorkNodesOf(noun) {
			if gen.WorkActivity(t, w) == gen.ActivityCarried {
				continue
			}
			addActive(w, w.Body().Status)
		}
	}

	addBlocked := func(n store.Node, dependsOn []string) {
		if Owner(n) != o.ID || !gen.UnfinishedAssignment(n.NodeStatus()) || !Blocked(n) {
			return
		}
		item := BlockedItem{Noun: n.Noun(), ID: n.NodeID(), Key: Key(n), Ref: gen.Ref(t, n)}
		if len(dependsOn) > 0 {
			// The two lists stay index-aligned, one edge per position, ordered
			// by their rendered reference.
			type edge struct{ id, ref string }
			edges := make([]edge, len(dependsOn))
			for i, dep := range dependsOn {
				edges[i] = edge{id: dep, ref: gen.RefByID(t, dep)}
			}
			sort.Slice(edges, func(i, j int) bool { return edges[i].ref < edges[j].ref })
			item.DependsOn = make([]string, len(edges))
			item.DependsOnRefs = make([]string, len(edges))
			for i, e := range edges {
				item.DependsOn[i] = e.id
				item.DependsOnRefs[i] = e.ref
			}
		}
		s.Blocked = append(s.Blocked, item)
	}
	for _, task := range t.Tasks {
		addBlocked(task, task.DependsOn)
	}
	for _, noun := range store.ShelfNouns() {
		for _, w := range t.WorkNodesOf(noun) {
			if gen.WorkActivity(t, w) == gen.ActivityCarried {
				continue
			}
			addBlocked(w, w.Body().DependsOn)
		}
	}

	// Retired ancestry holds only the owner's current task assignments.
	addHeld := func(n store.Node) {
		if Owner(n) != o.ID {
			return
		}
		held := gen.RetiredAncestor(t, n)
		if held == nil {
			return
		}
		s.Blocked = append(s.Blocked, BlockedItem{
			Noun: n.Noun(), ID: n.NodeID(), Key: Key(n), Ref: gen.Ref(t, n),
			Held: heldViewOf(held),
		})
	}
	for _, tk := range t.Tasks {
		addHeld(tk)
	}
	// Open-question conditions stay separate from manual and retired rows.
	// Only states whose next work command runs the corresponding guard appear;
	// proposed, review-only, completed, and retired nodes are not actionable
	// question-held work.
	addQuestionHold := func(n store.Node, blockers []gen.QuestionBlocker) {
		if Owner(n) != o.ID || len(blockers) == 0 {
			return
		}
		s.Blocked = append(s.Blocked, BlockedItem{
			Noun: n.Noun(), ID: n.NodeID(), Key: Key(n), Ref: gen.Ref(t, n),
			QuestionHolds: questionHoldItems(blockers),
		})
	}
	for _, tk := range t.Tasks {
		if !taskQuestionGuardOperative(tk.Status) {
			continue
		}
		if f := t.Features[tk.FeatureID]; f != nil {
			addQuestionHold(tk, gen.TaskQuestionBlockers(t, f, tk))
		}
	}

	// Tasks and shelf nodes both reach the gates lens: a PR always represents
	// a task or a shelf node, so in_review is the node's own state alone
	// (artifact-schema.md, "A PR always represents a task"; amended
	// 2026-08-31, home burst redesign).
	for _, tk := range t.Tasks {
		if Owner(tk) != o.ID || tk.Status != gen.ReviewState {
			continue
		}
		s.GatesInReview = append(s.GatesInReview, GatesItem{
			Noun: tk.Noun(), ID: tk.ID, Key: Key(tk), Ref: gen.Ref(t, tk), Gates: Gates(tk.GatesBlock),
		})
	}
	for _, noun := range store.ShelfNouns() {
		for _, w := range t.WorkNodesOf(noun) {
			body := w.Body()
			if Owner(w) != o.ID || body.Status != gen.ReviewState || gen.WorkActivity(t, w) == gen.ActivityCarried {
				continue
			}
			s.GatesInReview = append(s.GatesInReview, GatesItem{
				Noun: w.Noun(), ID: body.ID, Key: Key(w), Ref: gen.Ref(t, w), Gates: Gates(body.GatesBlock),
			})
		}
	}

	for _, q := range t.Questions {
		if q.Owner != o.ID || !gen.QuestionOpen(q) {
			continue
		}
		s.OpenQuestions = append(s.OpenQuestions, QuestionItem{
			ID: q.ID, Title: gen.QuestionTitle(q), Statement: q.Statement, Due: gen.QuestionDue(q),
			Blocks: append([]string(nil), q.Blocks...),
		})
	}

	// Active mixes nouns, so grain leads (epic, then feature, then task) and a
	// same-grain pair of a ranked noun orders by DependencyOrder; anything left
	// falls back to reference with the noun tiebreak. Grain has to lead once
	// tasks rank: a comparator that ranked same-noun pairs by dependency and
	// cross-noun pairs by reference is not transitive, because a feature's
	// reference can fall between two tasks whose rank reverses their
	// references, and sort.Slice over an intransitive comparator has no
	// defined result.
	sort.Slice(s.Active, func(i, j int) bool {
		ai, aj := s.Active[i], s.Active[j]
		if gi, gj := grainOf(ai.Noun), grainOf(aj.Noun); gi != gj {
			return gi < gj
		}
		if gen.RankedNoun(ai.Noun) {
			return order.Less(ai.Noun, ai.ID, aj.ID)
		}
		return streamLess(ai.Ref, aj.Ref, ai.Noun, aj.Noun)
	})
	// Blocked mixes all three work grains and can carry three rows for one
	// node. Grain and dependency rank order different nodes; the explicit
	// family rank keeps manual, retired, then question rows stable for one id.
	sort.Slice(s.Blocked, func(i, j int) bool {
		bi, bj := s.Blocked[i], s.Blocked[j]
		if gi, gj := grainOf(bi.Noun), grainOf(bj.Noun); gi != gj {
			return gi < gj
		}
		if bi.ID == bj.ID {
			return blockedFamilyRank(bi) < blockedFamilyRank(bj)
		}
		if gen.RankedNoun(bi.Noun) {
			return order.Less(bi.Noun, bi.ID, bj.ID)
		}
		return bi.Ref < bj.Ref
	})
	// GatesInReview mixes tasks and shelf nodes, because a PR always
	// represents a task or a shelf node (amended 2026-08-31, home burst
	// redesign). Grain leads, same as Active and Blocked, which puts every
	// task ahead of every bug or chore; a task pair then ranks through
	// DependencyOrder's task rank (owning feature first, then the sibling
	// order depends_on records, artifact-schema.md, Tasks: Ordering), and a
	// shelf pair falls back to reference comparison, since neither noun is
	// RankedNoun.
	sort.Slice(s.GatesInReview, func(i, j int) bool {
		gi, gj := s.GatesInReview[i], s.GatesInReview[j]
		if a, b := grainOf(gi.Noun), grainOf(gj.Noun); a != b {
			return a < b
		}
		if gi.Noun == store.NounTask {
			return order.Less(store.NounTask, gi.ID, gj.ID)
		}
		return streamLess(gi.Ref, gj.Ref, gi.Noun, gj.Noun)
	})
	sort.Slice(s.OpenQuestions, func(i, j int) bool { return s.OpenQuestions[i].ID < s.OpenQuestions[j].ID })
	return s
}

func blockedFamilyRank(item BlockedItem) int {
	switch {
	case item.Held != nil:
		return 1
	case len(item.QuestionHolds) > 0:
		return 2
	default:
		return 0
	}
}

func taskQuestionGuardOperative(status string) bool {
	switch status {
	case "ready", "in_progress", "check":
		return true
	default:
		return false
	}
}

func questionHoldItems(blockers []gen.QuestionBlocker) []QuestionHoldItem {
	if len(blockers) == 0 {
		return nil
	}
	items := make([]QuestionHoldItem, len(blockers))
	for i, blocker := range blockers {
		items[i] = QuestionHoldItem{
			Question: blocker.Question, Title: blocker.Title,
			TargetID: blocker.TargetID, TargetNoun: blocker.TargetNoun,
		}
	}
	return items
}

// streamLess orders active items by reference, noun as the tiebreaker so two
// nodes sharing a rendered reference order stably.
func streamLess(refA, refB, nounA, nounB string) bool {
	if refA != refB {
		return refA < refB
	}
	return nounA < nounB
}

// grainOf ranks a noun by its depth in the work tree, the leading term
// every mixed-noun list in this package sorts on (the active lens here, the
// attestation groups in queue.go). It exists so a rank comparison and a
// reference comparison never decide the same pair from different sides, which
// is what would make the comparator intransitive; anything off the tree, which
// today is the question, sorts after all three.
func grainOf(noun string) int {
	switch noun {
	case store.NounEpic:
		return 0
	case store.NounFeature:
		return 1
	case store.NounTask:
		return 2
	default:
		return 3
	}
}
