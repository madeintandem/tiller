package views

import (
	"fmt"
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// shortlistSize is how many ranked moves a surface lists.
const shortlistSize = 5

// The ranking tiers, best first.
const (
	tierUnblock  = 1 + iota // human decisions holding the current feature, and near deadlines
	tierContinue            // the reader's own unfinished work
	tierNearby              // the current feature, then its epic
	tierRecent              // features and epics touched in the recent window
	tierRest                // everything else, in plan order
)

// deadlineDays is how far ahead a question's due date pulls it into tier 1.
// Overdue questions always qualify.
const deadlineDays = 2

// NextOf ranks every legal next move for the reader's checkout. It is pure:
// the clock is in.Now and the reader's events arrive in in.Recent.
func NextOf(t *store.Tree, in NextInput) Next {
	order := gen.DependencyOrder(t)
	r := newRanker(t, LocateOf(t, in.Recent, in.Reader, in.Now), in.Now)
	next := Next{Reader: in.Reader}
	next.Candidates, next.keep = r.rank(candidatesOf(t, order, in))
	next.CaughtUp = len(next.Candidates) == 0
	switch {
	case len(t.Epics) == 0 && len(t.ToBeDesign) == 0 && !baselineRequired(t) && noPlannedWork(next.Candidates):
		// The zero-day guidance stands while nothing but open questions and
		// design confirmations is legal, so how work enters never
		// disappears; beside live work, such as a claimed bug, it would say
		// no work is planned.
		next.Unplanned, next.Hint = true, unplannedReason(t)
	case next.CaughtUp:
		next.Hint = PoolSignpost(t, order, in.Workflow)
	}
	if d := readiness.DependencyDiagnostics(t); len(d.Gaps) > 0 || len(d.Advisories) > 0 {
		next.DependencyDiagnostics = &d
	}
	return next
}

// noPlannedWork reports whether every move is an answer or a design doc
// confirmation, neither of which is planned work; true for no moves.
func noPlannedWork(cands []NextCandidate) bool {
	for _, c := range cands {
		if c.Kind != "answer_question" && c.Kind != "confirm_design" {
			return false
		}
	}
	return true
}

// Shortlist is a copy of the top five, except that the reader's top
// Continue node is always kept: when tier 1 alone fills five places without
// it, its surviving row takes the last one. That row is the
// Continue row, or the better-ranked row dedupe kept for the same node.
func (n Next) Shortlist() []NextCandidate {
	if len(n.Candidates) <= shortlistSize {
		return append([]NextCandidate{}, n.Candidates...)
	}
	short := append([]NextCandidate{}, n.Candidates[:shortlistSize]...)
	// Candidates sort by tier, so a tier 1 fifth row means five tier 1 rows.
	if n.keep == "" || short[shortlistSize-1].Tier != tierUnblock || nodeIn(short, n.keep) {
		return short
	}
	for _, c := range n.Candidates[shortlistSize:] {
		if c.Node != nil && c.Node.ID == n.keep {
			short[shortlistSize-1] = c
			break
		}
	}
	return short
}

func nodeIn(cands []NextCandidate, id string) bool {
	for _, c := range cands {
		if c.Node != nil && c.Node.ID == id {
			return true
		}
	}
	return false
}

// TopAgent is the highest-ranked move the agent makes, which is often not
// the top move.
func (n Next) TopAgent() (NextCandidate, bool) {
	for _, c := range n.Candidates {
		if c.Actor == actorAgent {
			return c, true
		}
	}
	return NextCandidate{}, false
}

// rankedMove is a candidate with the keys that order it inside its tier.
type rankedMove struct {
	NextCandidate
	due   time.Time // tier 1: the deadline, zero when the move has none
	holds bool      // tier 1: a question holding the current feature
	near  int       // tier 3: 0 the current feature, 1 its epic
	days  int       // tier 4: whole days since the reader last touched the move's place
}

// ranker assigns each candidate its tier and tier reason.
type ranker struct {
	t       *store.Tree
	loc     Locality
	now     time.Time
	today   time.Time // the reader's local calendar date, at UTC midnight
	current *store.Feature
	// holding maps each open question holding the current feature to the
	// node it holds there: one of its tasks, else the feature itself.
	holding map[string]store.Node
}

func newRanker(t *store.Tree, loc Locality, now time.Time) ranker {
	// A due date is a zoneless calendar date read in the reader's own day, so
	// today is now's date in now's location. It is held at UTC midnight only
	// so whole-day arithmetic against the parsed due date is exact.
	y, m, d := now.Date()
	r := ranker{t: t, loc: loc, now: now, today: time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
		current: t.Features[loc.CurrentFeature], holding: map[string]store.Node{}}
	if r.current == nil {
		return r
	}
	for _, b := range gen.FeatureQuestionBlockers(t, r.current) {
		r.holding[b.Question] = r.current
	}
	for _, task := range t.FeatureTasks[r.current.ID] {
		for _, b := range gen.TaskQuestionBlockers(t, r.current, task) {
			if _, cited := r.holding[b.Question].(*store.Task); b.TargetNoun == store.NounTask && !cited {
				r.holding[b.Question] = task
			}
		}
	}
	return r
}

// rank tiers, sorts, and dedupes the candidates. A node keeps only its
// best-ranked row; a move with no node is always its own row. keep is the
// node of the top Continue move before dedupe, "" when there is none.
func (r ranker) rank(cands []NextCandidate) (out []NextCandidate, keep string) {
	moves := make([]rankedMove, 0, len(cands))
	for _, c := range cands {
		moves = append(moves, r.place(c))
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].before(moves[j]) })
	out = make([]NextCandidate, 0, len(moves))
	seen := map[string]bool{}
	for _, m := range moves {
		if keep == "" && m.Tier == tierContinue && m.Node != nil {
			keep = m.Node.ID
		}
		if m.Node != nil {
			if seen[m.Node.ID] {
				continue
			}
			seen[m.Node.ID] = true
		}
		out = append(out, m.NextCandidate)
	}
	return out, keep
}

// before is the sort key: tier, then the tier's own key, then plan order
// (the walk's arm, then the arm's order; plan ruling 3).
func (a rankedMove) before(b rankedMove) bool {
	if a.Tier != b.Tier {
		return a.Tier < b.Tier
	}
	switch a.Tier {
	case tierUnblock:
		if a.due.IsZero() != b.due.IsZero() {
			return !a.due.IsZero()
		}
		if !a.due.Equal(b.due) {
			return a.due.Before(b.due)
		}
		if a.holds != b.holds {
			return a.holds
		}
	case tierNearby:
		if a.near != b.near {
			return a.near < b.near
		}
	case tierRecent:
		// Whole elapsed days, the grain the reason speaks in: touches on the
		// same day fall through to kind and plan order.
		if a.days != b.days {
			return a.days < b.days
		}
	}
	if a.arm != b.arm {
		return a.arm < b.arm
	}
	return a.pos < b.pos
}

// place assigns a candidate its tier and, for tiers 1 to 4, the reason that
// puts it there. Tier 5 keeps the candidate's base reason.
func (r ranker) place(c NextCandidate) rankedMove {
	m := rankedMove{NextCandidate: c}
	if due, reason, ok := r.deadline(c); ok {
		m.Tier, m.Reason, m.due = tierUnblock, reason, due
		return m
	}
	if held := r.heldBy(c); held != nil {
		m.Tier, m.Reason = tierUnblock, withBase(r.holdsReason(held, c), c)
		m.holds = c.Kind == "answer_question"
		return m
	}
	if c.Kind == "continue" {
		m.Tier, m.Reason = tierContinue, r.continueReason(c)
		return m
	}
	feature, epic := r.placeOf(c)
	switch {
	case feature != "" && feature == r.loc.CurrentFeature:
		m.Tier, m.Reason, m.near = tierNearby, withBase("next in your current feature", c), 0
	case epic != "" && epic == r.loc.CurrentEpic:
		m.Tier, m.Reason, m.near = tierNearby, withBase("in your current epic", c), 1
	default:
		m.Tier = tierRest
		if at, ref, ok := r.touched(feature, epic); ok {
			m.Tier, m.days = tierRecent, r.daysSince(at)
			reason := fmt.Sprintf("in %s, which you worked on %s", ref, r.ago(at))
			if ref == c.Node.Ref {
				reason = "you worked on it " + r.ago(at)
			}
			m.Reason = withBase(reason, c)
		}
	}
	return m
}

// withBase keeps a human decision's own reason, what it waits on, after the
// locality reason that placed it, so "in your current epic" still says
// "its feature plan awaits your approval". An agent move's tier reason
// stands alone.
func withBase(reason string, c NextCandidate) string {
	if c.Actor != actorHuman || c.Reason == "" {
		return reason
	}
	return reason + "; " + c.Reason
}

// deadline reports an answerable question overdue or due within
// deadlineDays, wherever it sits and whoever reads (plan ruling 5). Its
// reason says when, then keeps the question's own title. A hint or an
// unparseable date is no deadline.
func (r ranker) deadline(c NextCandidate) (time.Time, string, bool) {
	if c.Kind != "answer_question" || c.due == "" {
		return time.Time{}, "", false
	}
	due, err := time.Parse("2006-01-02", c.due)
	if err != nil {
		return time.Time{}, "", false
	}
	date := due.Format("2006-01-02")
	var when string
	switch days := int(due.Sub(r.today).Hours() / 24); {
	case days < 0:
		when = "overdue (was due " + date + ")"
	case days == 0:
		when = "due today"
	case days == 1:
		when = "due tomorrow"
	case days <= deadlineDays:
		when = "due " + date
	default:
		return time.Time{}, "", false
	}
	return due, withBase(when, c), true
}

// heldBy is the node of the current feature a human move holds: the feature
// itself, one of its tasks, or (for its epic) the feature. A question holds
// what gen's question blockers say it holds. Nil when the move holds nothing
// there or the reader has no current feature.
func (r ranker) heldBy(c NextCandidate) store.Node {
	if r.current == nil || c.Actor != actorHuman || c.Node == nil {
		return nil
	}
	switch c.Node.Noun {
	case store.NounQuestion:
		return r.holding[c.Node.ID]
	case store.NounFeature:
		if c.Node.ID == r.current.ID {
			return r.current
		}
	case store.NounEpic:
		if c.Node.Key != "" && c.Node.Key == r.current.EpicKey {
			return r.current
		}
	case store.NounTask:
		if task := r.t.Tasks[c.Node.ID]; task != nil && task.FeatureID == r.current.ID {
			return task
		}
	}
	return nil
}

// holdsReason names what a tier 1 move holds in the current feature. A move
// on the held node itself (a merge of the current feature's task, an
// approval of the feature) names only where it sits, never itself.
func (r ranker) holdsReason(held store.Node, c NextCandidate) string {
	if held.NodeID() == c.Node.ID {
		return "in your current feature"
	}
	if _, ok := held.(*store.Task); ok {
		return fmt.Sprintf("holds %s in your current feature", gen.Ref(r.t, held))
	}
	return fmt.Sprintf("holds %s, your current feature", gen.Ref(r.t, held))
}

// continueReason says which of the reader's own unfinished work this is and
// what it still owes, if anything.
func (r ranker) continueReason(c NextCandidate) string {
	// The line itself names the branch, and a task seen on its branch may
	// not have started, so the reason says only where it is.
	if c.Branch != "" {
		return "your task, in flight on its branch"
	}
	noun, status, guidance := "task", "", ""
	switch n := gen.NodeByID(r.t, c.Node.ID).(type) {
	case *store.Task:
		status = n.Status
		if f := r.t.Features[n.FeatureID]; f != nil {
			guidance = ownTaskGuidance(r.t, f, n)
		}
	case store.WorkNode:
		noun, status, guidance = n.Noun(), n.Body().Status, shelfWorkGuidance(n)
	}
	phrase := "is in progress"
	switch status {
	case "check":
		phrase = "is in check"
	case "ready":
		phrase = "is claimed and ready to start"
	}
	reason := fmt.Sprintf("your %s %s", noun, phrase)
	if guidance != "" {
		reason += "; " + guidance
	}
	return reason
}

// placeOf is the feature and epic a candidate's node sits in, as the event
// locator places a target. Moves with no node, amendments, and shelf work
// sit nowhere.
func (r ranker) placeOf(c NextCandidate) (feature, epic string) {
	if c.Node == nil {
		return "", ""
	}
	touch, ok := touchOf(r.t, c.Node.ID, time.Time{})
	if !ok {
		return "", ""
	}
	return touch.FeatureID, touch.EpicKey
}

// touched is when the reader last worked where the candidate sits, and that
// place's reference: its feature when touched, else its epic.
func (r ranker) touched(feature, epic string) (time.Time, string, bool) {
	if feature != "" {
		for _, touch := range r.loc.Touched {
			if touch.FeatureID == feature {
				return touch.At, gen.Ref(r.t, r.t.Features[feature]), true
			}
		}
	}
	if epic != "" {
		for _, touch := range r.loc.Touched {
			if touch.EpicKey == epic {
				if e := recapEpicByKey(r.t, epic); e != nil {
					return touch.At, gen.Ref(r.t, e), true
				}
			}
		}
	}
	return time.Time{}, "", false
}

// daysSince is how many whole days have elapsed since at.
func (r ranker) daysSince(at time.Time) int {
	return int(r.now.Sub(at) / (24 * time.Hour))
}

// ago renders how long since at in whole elapsed days.
func (r ranker) ago(at time.Time) string {
	switch days := r.daysSince(at); {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}
