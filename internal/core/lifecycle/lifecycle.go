// Package lifecycle is the pure state machine at the center of Tiller. It
// answers three questions about a node without touching disk, git, or stdout:
// is a verb legal from here, what moves are legal from here, and who may run a
// verb; and it performs the one job that is not a question, applying a legal
// transition by mutating the node's status (plus the small, transition-specific
// side effects resume and submit carry).
//
// It imports only the store node types. It is reasoning-free and deterministic:
// the rules live in the declarative tables in table.go, transcribed from the one
// spec that governs them (artifact-schema.md, "Lifecycles, consolidated" and the
// per-node States sections; cli-surface.md, "Who may run what" and "The done
// crossing"). The CLI and the mutate layer consume this engine; they add the
// event recording, the file writes, and the structural guards (active basis,
// question blockers) that sit outside the state machine.
package lifecycle

import (
	"fmt"
	"sort"

	"github.com/madeintandem/tiller/internal/store"
)

// Payload carries the transition-specific inputs the engine needs to mutate a
// node. Most transitions only change status and need nothing here; the fields
// serve the few that do. Reason is the human-supplied justification a resume,
// return, or cancel records (the engine does not persist it; the caller writes
// it to the event stream). PR is the pull request number submit records on the
// task it moves to in_review; ReviewRef is the branch-ref fallback a remote-less
// submit records instead (the branch and head commit the review happens on),
// never both. CodeIdentity is the content code identity at resume time, which
// resume compares to each command gate's recorded code to decide what stays
// pass; empty (no git, a stub) matches nothing.
type Payload struct {
	Reason       string
	PR           *int
	ReviewRef    *store.ReviewRef
	CodeIdentity string
}

// Legal reports whether verb is a legal move from the node's current state, and
// returns the legal-next verb set from here regardless of the answer, so a
// caller (or an exit-1 envelope) can show the way forward. A verb is legal when
// it has a forward transition from the current state, or when it is an
// idempotent no-op: a verb whose target equals the current state, which is the
// safe-retry rule from cli-surface.md ("Output and exit codes").
func Legal(node store.Node, verb string) (bool, []string) {
	rows := transitions[node.Noun()]
	current := node.NodeStatus()
	moves := nextMoves(rows, current)
	if _, ok := forward(rows, current, verb); ok {
		return true, moves
	}
	if idempotent(rows, current, verb) {
		return true, moves
	}
	return false, moves
}

// NextMoves returns the legal forward verbs from the node's current state,
// sorted for deterministic output. A terminal state returns an empty slice.
// A question is not a transitions-table noun: it is a ledger item whose whole
// life is open -> answered or open -> cancelled (artifact-schema.md,
// "Decisions and open questions") and whose writes are bespoke ops
// (mutate.AnswerQuestion, mutate.CancelQuestion), never the generic
// Transition engine. The oracle still owes an honest legal_next, so the
// open question's two exits are answered here rather than as table rows the
// engine could be asked to drive.
func NextMoves(node store.Node) []string {
	if _, ok := node.(*store.Question); ok {
		if node.NodeStatus() == "open" {
			return []string{"answer", "cancel"}
		}
		return []string{}
	}
	return nextMoves(transitions[node.Noun()], node.NodeStatus())
}

// EmittableVerbs returns the distinct transition verbs the table can emit into
// a status legal_next answer, sorted: every verb of every noun the store models
// as a concrete node, the amendment included now that both its verbs carry a
// registered command (`tiller approve amendment`, `tiller withdraw amendment`,
// internal/cli's amendment_cmds.go). The status oracle is documented as the
// authority on legal moves, so the binary must register a command for each of
// these; the oracle-CLI parity test in internal/cli asserts exactly that.
func EmittableVerbs() []string {
	seen := make(map[string]bool)
	// The question special case in NextMoves emits "answer" and "cancel" outside the
	// transitions table, so the parity net must demand its command too.
	verbs := []string{"answer"}
	seen["answer"] = true
	for _, rows := range transitions {
		for _, t := range rows {
			if !seen[t.Verb] {
				seen[t.Verb] = true
				verbs = append(verbs, t.Verb)
			}
		}
	}
	sort.Strings(verbs)
	return verbs
}

// ActorFor reports who the spec expects to run verb. Verbs not modeled as
// lifecycle transitions (the orthogonal flag and membership verbs) default to
// ActorEither.
//
// The classifier is named ActorFor rather than Actor because a Go package cannot
// hold a function and a type of the same name; the Actor type and its
// ActorAgent/ActorHuman/ActorEither constants keep that name (see table.go).
func ActorFor(verb string) Actor {
	if a, ok := verbActors[verb]; ok {
		return a
	}
	return ActorEither
}

// Apply performs a legal transition by mutating the node in place. A forward
// transition advances the status and runs the verb's side effects; an idempotent
// no-op leaves the node untouched and succeeds; an illegal move mutates nothing
// and returns an *IllegalTransitionError carrying the current state and the
// legal-next set for the recoverable exit-1 envelope.
func Apply(node store.Node, verb string, payload Payload) error {
	rows := transitions[node.Noun()]
	current := node.NodeStatus()
	if t, ok := forward(rows, current, verb); ok {
		if err := setStatus(node, t.To); err != nil {
			return err
		}
		return sideEffects(node, verb, payload)
	}
	if idempotent(rows, current, verb) {
		return nil
	}
	return &IllegalTransitionError{
		Noun:  node.Noun(),
		From:  current,
		Verb:  verb,
		Legal: nextMoves(rows, current),
	}
}

// forward finds the transition row that advances current via verb, if any.
func forward(rows []transition, current, verb string) (transition, bool) {
	for _, t := range rows {
		if t.From == current && t.Verb == verb {
			return t, true
		}
	}
	return transition{}, false
}

// idempotent reports whether re-running verb is a no-op because the node already
// sits in the state that verb advances to. This is what makes a retry of a
// just-applied transition a success rather than an illegal move.
func idempotent(rows []transition, current, verb string) bool {
	for _, t := range rows {
		if t.Verb == verb && t.To == current {
			return true
		}
	}
	return false
}

// nextMoves collects the distinct forward verbs from current, sorted.
func nextMoves(rows []transition, current string) []string {
	var verbs []string
	seen := make(map[string]bool)
	for _, t := range rows {
		if t.From == current && !seen[t.Verb] {
			seen[t.Verb] = true
			verbs = append(verbs, t.Verb)
		}
	}
	sort.Strings(verbs)
	return verbs
}

// sideEffects runs the small, transition-specific mutations beyond the status
// change. Only resume and submit carry any; every other verb is a pure status
// move.
func sideEffects(node store.Node, verb string, payload Payload) error {
	switch verb {
	case "resume":
		// Re-entering work after a requested-changes review keeps the recorded
		// PR (the same PR receives the follow-up commits) and stales the gate
		// results the new commits invalidate, so the next check re-runs them.
		// Resume keeps a command gate only when its recorded code still matches
		// the code at resume.
		StaleGatesUnlessCode(node, payload.CodeIdentity)
	case "submit":
		if payload.PR != nil {
			setPR(node, payload.PR)
		}
		if payload.ReviewRef != nil {
			setReviewRef(node, payload.ReviewRef)
		}
	}
	return nil
}

// setStatus writes the new status onto the concrete node: the three
// folder-backed tree nodes, and a bug or chore through store.AsWorkNode. An
// amendment is deliberately absent:
// store.Amendment exists, but its status is written by the amendment verbs in
// internal/core/mutate rather than through this engine (the amendment rows in
// table.go carry the reason), so an Apply that reached one would mean a caller
// had lost track of which writer owns that status, and saying so is better than
// a silent miss.
func setStatus(node store.Node, to string) error {
	switch n := node.(type) {
	case *store.Epic:
		n.Status = to
	case *store.Feature:
		n.Status = to
	case *store.Task:
		n.Status = to
	default:
		if w, ok := store.AsWorkNode(node); ok {
			w.Body().Status = to
			return nil
		}
		return fmt.Errorf("lifecycle: cannot apply a transition to a %s node: no concrete store type", node.Noun())
	}
	return nil
}

// setPR records the pull request number on the node types that carry one.
// A PR represents exactly one node: a task at task grain, or a bug or chore,
// which always carries its own.
func setPR(node store.Node, pr *int) {
	if n, ok := node.(*store.Task); ok {
		n.PR = pr
		return
	}
	if w, ok := store.AsWorkNode(node); ok {
		w.Body().PR = pr
	}
}

// setReviewRef records the branch-ref review surface on the node types that
// carry one. The fallback a remote-less submit records where a PR would
// otherwise be. It follows the pr to the task grain and work items for the
// same reason.
func setReviewRef(node store.Node, ref *store.ReviewRef) {
	if n, ok := node.(*store.Task); ok {
		n.ReviewRef = ref
		return
	}
	if w, ok := store.AsWorkNode(node); ok {
		w.Body().ReviewRef = ref
	}
}

// StaleGates flips every stored gate result on the node to stale, and every
// per-criterion status inside the evals gate with it. Only the gate kinds the
// node carries are touched.
//
// It is the answer to one question, asked from two places: prior gate results
// stop describing this node the moment work resumes under it. `resume` asks it
// through the engine's own side effects (in_review back to work), and sync's
// rollup recompute asks it directly when a done feature reopens because a task
// landed beneath it (mutate/sync.go, reopenFeature). Both are the same event
// wearing different clothes, so both must reach the same rule rather than each
// carrying a copy: a node that is back at work must not keep certifying itself
// with a run that predates the work.
//
// The criteria stale for the same reason the gates do, and it matters more
// there: a criterion's status is where a HUMAN's attestation is recorded, and
// the eval re-evaluation deliberately preserves a human-settled pass across
// re-runs (gates.keepPrior). Without staling, that preservation carries a
// sign-off across a reopen and one ordinary `check` returns the gate to green
// on judgment given before the new work existed. Evidence is left alone: what
// the human reviewed is still the trail behind the verdict, and only the
// verdict expired.
func StaleGates(node store.Node) {
	gb := gatesBlock(node)
	if gb == nil {
		return
	}
	names := make([]string, 0, len(gb.Gates))
	for name := range gb.Gates {
		names = append(names, name)
	}
	for _, name := range names {
		StaleGate(node, name)
	}
}

// StaleGatesUnlessCode is resume's narrowing of StaleGates: it stales every
// attestation gate (a human's judgment never carries across new work) and every
// command gate whose recorded Code is empty or differs from code, and leaves a
// command gate whose recorded code still equals code alone, except that inside
// a kept gate each criterion with no code or another code goes stale and takes
// the gate with it. An empty code, as
// from a stub or a project with no git, therefore stales everything. A plain
// check reruns on a config change regardless of resume, so only code is
// compared here.
func StaleGatesUnlessCode(node store.Node, code string) {
	gb := gatesBlock(node)
	if gb == nil {
		return
	}
	names := make([]string, 0, len(gb.Gates))
	for name := range gb.Gates {
		names = append(names, name)
	}
	for _, name := range names {
		g := gb.Gates[name]
		if g.Kind == store.GateKindAttestation || code == "" || g.Code != code {
			StaleGate(node, name)
			continue
		}
		// The gate's code matches, but the evals gate carries per-criterion
		// records: a command criterion keeps pass under its own matching code,
		// while a manual or eval criterion stores none and so expires as it did
		// before reuse (a human's pass or a rubric's needs_attestation must not
		// survive new work). Any expired criterion takes the gate with it.
		expired := false
		for id, c := range g.Criteria {
			if c.Code == "" || c.Code != code {
				c.Status = store.CriterionStatusStale
				g.Criteria[id] = c
				expired = true
			}
		}
		if expired {
			g.Status = store.GateStatusStale
			gb.Gates[name] = g
		}
	}
}

// StaleGate flips one stored gate result on the node to stale, together with
// every per-criterion status inside that gate. A missing gate is left alone.
func StaleGate(node store.Node, name string) {
	gb := gatesBlock(node)
	if gb == nil {
		return
	}
	g, ok := gb.Gates[name]
	if !ok {
		return
	}
	gb.Gates[name] = StaledGate(g)
}

// StaledGate returns a copy of gate record g flipped to stale, together with
// every per-criterion status inside it, keeping its evidence and provenance.
// The criteria map is copied, so the record g was read from is left as it was.
func StaledGate(g store.Gate) store.Gate {
	g.Status = store.GateStatusStale
	if g.Criteria != nil {
		criteria := make(map[string]store.Criterion, len(g.Criteria))
		for id, c := range g.Criteria {
			c.Status = store.CriterionStatusStale
			criteria[id] = c
		}
		g.Criteria = criteria
	}
	return g
}

func gatesBlock(node store.Node) *store.GatesBlock {
	switch n := node.(type) {
	case *store.Feature:
		return &n.GatesBlock
	case *store.Task:
		return &n.GatesBlock
	default:
		if w, ok := store.AsWorkNode(node); ok {
			return &w.Body().GatesBlock
		}
		return nil
	}
}

// IllegalTransitionError is the typed refusal Apply returns for a move that is
// not legal from the current state. It carries the node noun, the current state,
// the attempted verb, and the legal-next set, which is exactly the structured
// detail the exit-1 blocked envelope surfaces as current_state and legal_next
// (cli-surface.md, "The JSON envelope").
type IllegalTransitionError struct {
	Noun  string
	From  string
	Verb  string
	Legal []string
}

func (e *IllegalTransitionError) Error() string {
	return fmt.Sprintf("illegal transition: cannot %s a %s from %q", e.Verb, e.Noun, e.From)
}
