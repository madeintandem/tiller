package lifecycle

import (
	"errors"
	"sort"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test IS the lifecycle spec made executable. The expectations below are
// transcribed BY HAND from docs/spec/artifact-schema.md ("Lifecycles,
// consolidated" plus the per-node States sections) and docs/spec/cli-surface.md
// ("Who may run what", "The done crossing"). They are deliberately NOT derived
// from the production transition table: a divergence between this file and
// internal/core/lifecycle/table.go is exactly the signal worth catching, so the
// expectations are an independent second source on purpose.

const amendmentNoun = "amendment"

// fakeNode is a state-only store.Node. The legality/next-move tests only read a
// node's noun and status, so a fake lets us enumerate every (node, state) pair,
// including amendment, which has no concrete store type yet.
type fakeNode struct {
	id     string
	key    string
	noun   string
	status string
}

func (f fakeNode) NodeID() string { return f.id }
func (f fakeNode) NodeKey() *string {
	if f.key == "" {
		return nil
	}
	return &f.key
}
func (f fakeNode) Noun() string       { return f.noun }
func (f fakeNode) NodeStatus() string { return f.status }

// specTransition is one documented (noun, from, verb, to) row, transcribed from
// the spec lifecycle tables.
type specTransition struct {
	noun string
	from string
	verb string
	to   string
}

// specTransitions is the documented legal forward transition set, by hand from
// the spec. Idempotent self-loops and the orthogonal flag verbs (block, claim)
// are not transitions and are not listed.
var specTransitions = []specTransition{
	// Epic: draft -> ready -> done -> archived, plus draft/ready -> cancelled.
	// There is no stored 'active' epic state and no 'start epic'. Only two of
	// those edges are verbs: approve moves an epic draft -> ready (approving the
	// decomposition tree) and archive moves it done -> archived (the human's
	// deliberate close). done itself has NO verb in either direction: it is a
	// rollup sync records over the epic's features and recomputes back to ready
	// when new work lands beneath, which is also why there is no reactivate. The
	// epic's being-worked status is DERIVED from its children, not stored (see
	// gen.EpicActivity).
	{store.NounEpic, "draft", "approve", "ready"},
	{store.NounEpic, "draft", "cancel", "cancelled"},
	{store.NounEpic, "ready", "cancel", "cancelled"},
	{store.NounEpic, "done", "archive", "archived"},

	// Feature activation belongs to the first task-start transaction, not a
	// feature verb. Aggregate review does not add a lifecycle state; done and
	// reopening are rollups recorded by sync.
	{store.NounFeature, "draft", "spec", "specced"},
	{store.NounFeature, "draft", "cancel", "cancelled"},
	{store.NounFeature, "specced", "approve", "ready"},
	{store.NounFeature, "specced", "cancel", "cancelled"},
	{store.NounFeature, "ready", "return", "specced"},
	{store.NounFeature, "ready", "cancel", "cancelled"},
	{store.NounFeature, "active", "cancel", "cancelled"},

	// Task: proposed -> ready -> in_progress -> check -> in_review -> done, plus
	// ready -> proposed on return and in_review -> in_progress on resume. The
	// post-approval state is 'ready', harmonized with the feature's specced ->
	// ready so every node's post-approval state is the same word.
	{store.NounTask, "proposed", "approve", "ready"},
	{store.NounTask, "proposed", "cancel", "cancelled"},
	{store.NounTask, "ready", "start", "in_progress"},
	{store.NounTask, "ready", "return", "proposed"},
	{store.NounTask, "ready", "cancel", "cancelled"},
	{store.NounTask, "in_progress", "check", "check"},
	{store.NounTask, "in_progress", "cancel", "cancelled"},
	{store.NounTask, "check", "submit", "in_review"},
	{store.NounTask, "check", "cancel", "cancelled"},
	{store.NounTask, "in_review", "resume", "in_progress"},
	{store.NounTask, "in_review", "sync", "done"},
	{store.NounTask, "in_review", "cancel", "cancelled"},

	// Bug and chore: the task's rows verbatim (docs/plans/2026-08-28-bugs-and-
	// chores-design.md, Lifecycle, contract, gates). The carried crossing is
	// not a row: sync sets done directly, the way a rollup does.
	{store.NounBug, "proposed", "approve", "ready"},
	{store.NounBug, "proposed", "cancel", "cancelled"},
	{store.NounBug, "ready", "start", "in_progress"},
	{store.NounBug, "ready", "return", "proposed"},
	{store.NounBug, "ready", "cancel", "cancelled"},
	{store.NounBug, "in_progress", "check", "check"},
	{store.NounBug, "in_progress", "cancel", "cancelled"},
	{store.NounBug, "check", "submit", "in_review"},
	{store.NounBug, "check", "cancel", "cancelled"},
	{store.NounBug, "in_review", "resume", "in_progress"},
	{store.NounBug, "in_review", "sync", "done"},
	{store.NounBug, "in_review", "cancel", "cancelled"},
	{store.NounChore, "proposed", "approve", "ready"},
	{store.NounChore, "proposed", "cancel", "cancelled"},
	{store.NounChore, "ready", "start", "in_progress"},
	{store.NounChore, "ready", "return", "proposed"},
	{store.NounChore, "ready", "cancel", "cancelled"},
	{store.NounChore, "in_progress", "check", "check"},
	{store.NounChore, "in_progress", "cancel", "cancelled"},
	{store.NounChore, "check", "submit", "in_review"},
	{store.NounChore, "check", "cancel", "cancelled"},
	{store.NounChore, "in_review", "resume", "in_progress"},
	{store.NounChore, "in_review", "sync", "done"},
	{store.NounChore, "in_review", "cancel", "cancelled"},

	// Amendment: proposed -> approved (approve applies the batch), or withdrawn.
	{amendmentNoun, "proposed", "approve", "approved"},
	{amendmentNoun, "proposed", "withdraw", "withdrawn"},
}

// expectedNextMoves is the legal forward verb set per (noun, state), transcribed
// from the spec. Terminal states map to an empty set.
var expectedNextMoves = map[string]map[string][]string{
	store.NounEpic: {
		"draft":     {"approve", "cancel"},
		"ready":     {"cancel"},
		"done":      {"archive"},
		"archived":  {},
		"cancelled": {},
	},
	store.NounFeature: {
		"draft":     {"cancel", "spec"},
		"specced":   {"approve", "cancel"},
		"ready":     {"cancel", "return"},
		"active":    {"cancel"},
		"done":      {},
		"cancelled": {},
	},
	store.NounTask: {
		"proposed":    {"approve", "cancel"},
		"ready":       {"cancel", "return", "start"},
		"in_progress": {"cancel", "check"},
		"check":       {"cancel", "submit"},
		"in_review":   {"cancel", "resume", "sync"},
		"done":        {},
		"cancelled":   {},
	},
	store.NounBug: {
		"proposed":    {"approve", "cancel"},
		"ready":       {"cancel", "return", "start"},
		"in_progress": {"cancel", "check"},
		"check":       {"cancel", "submit"},
		"in_review":   {"cancel", "resume", "sync"},
		"done":        {},
		"cancelled":   {},
	},
	store.NounChore: {
		"proposed":    {"approve", "cancel"},
		"ready":       {"cancel", "return", "start"},
		"in_progress": {"cancel", "check"},
		"check":       {"cancel", "submit"},
		"in_review":   {"cancel", "resume", "sync"},
		"done":        {},
		"cancelled":   {},
	},
	amendmentNoun: {
		"proposed":  {"approve", "withdraw"},
		"approved":  {},
		"withdrawn": {},
	},
}

// TestNextMoves_PerState asserts NextMoves reproduces the spec's legal verb set
// for every documented state of every node type.
func TestNextMoves_PerState(t *testing.T) {
	for noun, states := range expectedNextMoves {
		for state, want := range states {
			node := fakeNode{noun: noun, status: state}
			got := NextMoves(node)
			assert.ElementsMatch(t, want, got,
				"NextMoves(%s, %q) = %v, want %v", noun, state, got, want)
			assert.True(t, sort.StringsAreSorted(got),
				"NextMoves(%s, %q) must be sorted for determinism, got %v", noun, state, got)
		}
	}
}

// TestLegal_DocumentedTransitions asserts every documented forward transition is
// Legal, and that Legal's returned legal-next set equals NextMoves.
func TestLegal_DocumentedTransitions(t *testing.T) {
	for _, tr := range specTransitions {
		node := fakeNode{noun: tr.noun, status: tr.from}
		ok, moves := Legal(node, tr.verb)
		assert.True(t, ok, "Legal(%s, %q, %q) should be legal", tr.noun, tr.from, tr.verb)
		assert.ElementsMatch(t, NextMoves(node), moves,
			"Legal's legal-next for (%s, %q) must equal NextMoves", tr.noun, tr.from)
	}
}

// TestLegal_IllegalSampling asserts a hand-picked set of moves the spec forbids
// are not Legal. The cases cover skipped gates, terminal states, and wrong-state
// verbs.
func TestLegal_IllegalSampling(t *testing.T) {
	illegal := []specTransition{
		// Cannot skip the human approve / spec gates.
		{store.NounEpic, "draft", "start", ""},
		{store.NounEpic, "draft", "submit", ""},
		// There is no 'start epic': a ready epic only cancels, and an epic's
		// being-worked status is derived, not started.
		{store.NounEpic, "ready", "start", ""},
		// No verb reaches epic done from either side. accept is gone with the
		// gate, and reactivate never existed: the rollup recomputes instead.
		{store.NounEpic, "ready", "accept", ""},
		{store.NounEpic, "ready", "sync", ""},
		{store.NounEpic, "done", "reactivate", ""},
		{store.NounFeature, "draft", "approve", ""},
		{store.NounFeature, "draft", "submit", ""},
		{store.NounFeature, "specced", "start", ""},
		{store.NounFeature, "ready", "start", ""},
		{store.NounFeature, "active", "start", ""},
		{store.NounFeature, "ready", "submit", ""},
		{store.NounTask, "proposed", "start", ""},
		{store.NounTask, "proposed", "submit", ""},
		{store.NounTask, "ready", "check", ""},
		// Terminal / done states refuse further moves.
		{store.NounFeature, "done", "resume", ""},
		{store.NounFeature, "done", "cancel", ""},
		{store.NounFeature, "done", "submit", ""},
		{store.NounTask, "done", "cancel", ""},
		{store.NounEpic, "done", "cancel", ""},
		{store.NounEpic, "archived", "start", ""},
		{store.NounEpic, "draft", "accept", ""},
		// Aggregate review and rollup operations bypass this lifecycle table.
		{store.NounFeature, "active", "submit", ""},
		{store.NounFeature, "active", "resume", ""},
		{store.NounFeature, "active", "sync", ""},
		// Wrong-state verbs.
		{store.NounTask, "in_review", "start", ""},
		{store.NounBug, "ready", "sync", ""},
		{store.NounChore, "proposed", "start", ""},
		{amendmentNoun, "approved", "withdraw", ""},
		{amendmentNoun, "withdrawn", "approve", ""},
		// A verb that does not exist for the node type at all.
		{store.NounEpic, "ready", "spec", ""},
	}
	for _, tr := range illegal {
		node := fakeNode{noun: tr.noun, status: tr.from}
		ok, _ := Legal(node, tr.verb)
		assert.False(t, ok, "Legal(%s, %q, %q) should be illegal", tr.noun, tr.from, tr.verb)
	}
}

// TestLegal_Idempotent asserts the mutation contract's idempotency rule
// (cli-surface.md, Output and exit codes): a verb whose target equals the
// current state is a legal no-op, so retries are safe.
func TestLegal_Idempotent(t *testing.T) {
	cases := []specTransition{
		{store.NounFeature, "specced", "spec", ""},  // re-spec an already-specced feature
		{store.NounFeature, "ready", "approve", ""}, // re-approve an already-ready feature
		{store.NounTask, "ready", "approve", ""},    // re-approve an already-ready task
		{store.NounEpic, "ready", "approve", ""},    // re-approve an already-ready (approved) epic
		{amendmentNoun, "approved", "approve", ""},  // re-approve an already-approved amendment
	}
	for _, c := range cases {
		node := fakeNode{noun: c.noun, status: c.from}
		ok, _ := Legal(node, c.verb)
		assert.True(t, ok, "idempotent Legal(%s, %q, %q) should be a legal no-op", c.noun, c.from, c.verb)
	}
}

// TestActor asserts the who-may-run classification per verb (cli-surface.md,
// "Who may run what" and the Command reference Actor column).
func TestActor(t *testing.T) {
	cases := map[string]Actor{
		"approve":  ActorHuman,
		"return":   ActorHuman,
		"cancel":   ActorHuman,
		"archive":  ActorHuman,
		"withdraw": ActorHuman,
		"attest":   ActorHuman,
		"reassign": ActorHuman,
		"transfer": ActorHuman,
		"spec":     ActorAgent,
		"start":    ActorEither,
		"check":    ActorEither,
		"submit":   ActorEither,
		"resume":   ActorEither,
		"sync":     ActorEither,
	}
	for verb, want := range cases {
		assert.Equal(t, want, ActorFor(verb), "ActorFor(%q)", verb)
	}
}

// TestApply_HappyTransitions walks each node type's main spine through Apply and
// asserts the resulting status, using the real store structs.
func TestApply_HappyTransitions(t *testing.T) {
	t.Run("epic", func(t *testing.T) {
		// draft -> ready (approve the tree), then archived once the rollup has put
		// the epic in done. There is no stored 'active' epic state and no verb
		// that reaches done: sync records that crossing over the epic's features
		// (mutate/sync.go), so the spine here picks up again at archive.
		e := &store.Epic{ID: "E-aaaaaa", Status: "draft"}
		require.NoError(t, Apply(e, "approve", Payload{}))
		assert.Equal(t, "ready", e.Status)

		rolledUp := &store.Epic{ID: "E-bbbbbb", Status: "done"}
		require.NoError(t, Apply(rolledUp, "archive", Payload{}))
		assert.Equal(t, "archived", rolledUp.Status)
	})

	t.Run("feature", func(t *testing.T) {
		f := &store.Feature{ID: "F-aaaaaa", Status: "draft"}
		require.NoError(t, Apply(f, "spec", Payload{}))
		assert.Equal(t, "specced", f.Status)
		require.NoError(t, Apply(f, "approve", Payload{}))
		assert.Equal(t, "ready", f.Status)
		// Activation belongs to the task-start transaction, not Apply(feature).
		require.Error(t, Apply(f, "start", Payload{}))
		assert.Equal(t, "ready", f.Status)
	})

	t.Run("task", func(t *testing.T) {
		tk := &store.Task{ID: "T-aaaaaa", Status: "proposed", Owner: "jc"}
		require.NoError(t, Apply(tk, "approve", Payload{}))
		assert.Equal(t, "ready", tk.Status)
		require.NoError(t, Apply(tk, "start", Payload{}))
		assert.Equal(t, "in_progress", tk.Status)
		// The mutation transaction freezes ownership alongside this state move.
		tk.Started = true
		require.NoError(t, Apply(tk, "check", Payload{}))
		assert.Equal(t, "check", tk.Status)
		pr := 7
		require.NoError(t, Apply(tk, "submit", Payload{PR: &pr}))
		assert.Equal(t, "in_review", tk.Status)
		require.NotNil(t, tk.PR)
		assert.Equal(t, 7, *tk.PR)
	})
}

// TestApply_Idempotent asserts a no-op transition succeeds and leaves the state
// untouched.
func TestApply_Idempotent(t *testing.T) {
	f := &store.Feature{ID: "F-aaaaaa", Status: "specced"}
	require.NoError(t, Apply(f, "spec", Payload{}))
	assert.Equal(t, "specced", f.Status)
}

// TestApply_IllegalReturnsTypedError asserts an illegal Apply returns a typed
// IllegalTransitionError carrying the current state and the legal-next set, so
// the CLI can render the exit-1 recoverable envelope.
func TestApply_IllegalReturnsTypedError(t *testing.T) {
	f := &store.Feature{ID: "F-aaaaaa", Status: "draft"}
	err := Apply(f, "submit", Payload{})
	require.Error(t, err)
	var ite *IllegalTransitionError
	require.True(t, errors.As(err, &ite), "want *IllegalTransitionError, got %T", err)
	assert.Equal(t, "draft", ite.From)
	assert.Equal(t, "submit", ite.Verb)
	assert.Equal(t, store.NounFeature, ite.Noun)
	assert.ElementsMatch(t, []string{"cancel", "spec"}, ite.Legal)
	// The illegal Apply left the node untouched.
	assert.Equal(t, "draft", f.Status)
}

// TestApply_ReturnPreWorkOnly asserts return is legal before work starts and
// refused once work has started, entered review, or reached done.
func TestApply_ReturnPreWorkOnly(t *testing.T) {
	// Feature: ready -> specced is legal.
	f := &store.Feature{ID: "F-aaaaaa", Status: "ready"}
	require.NoError(t, Apply(f, "return", Payload{Reason: "wrong scope"}))
	assert.Equal(t, "specced", f.Status)

	// Task: ready -> proposed is legal.
	tk := &store.Task{ID: "T-aaaaaa", Status: "ready"}
	require.NoError(t, Apply(tk, "return", Payload{Reason: "premature"}))
	assert.Equal(t, "proposed", tk.Status)

	// return is refused once work has started or reached done.
	for _, state := range []string{"active", "done"} {
		bad := &store.Feature{ID: "F-bbbbbb", Status: state}
		err := Apply(bad, "return", Payload{Reason: "too late"})
		require.Error(t, err, "return from feature %q must be refused", state)
		var ite *IllegalTransitionError
		assert.True(t, errors.As(err, &ite))
		assert.Equal(t, state, bad.Status, "refused return must not mutate state")
	}
	for _, state := range []string{"in_progress", "check", "in_review", "done"} {
		bad := &store.Task{ID: "T-bbbbbb", Status: state, Owner: "jc", Started: true}
		err := Apply(bad, "return", Payload{Reason: "too late"})
		require.Error(t, err, "return from task %q must be refused", state)
	}
}

// TestApply_ResumeStalesOnlyMismatchedGates asserts resume re-enters work, keeps
// the recorded PR, and stales only the gates whose recorded code no longer
// matches the code at resume (gate-reuse design, decision 9): a command gate
// over the same code stays pass, a command gate over other code or with no
// recorded code goes stale, and an attestation gate always goes stale. An empty
// resume code is the stub and no-git case, where nothing matches and everything
// goes stale. It is a task-grain test alone, because a PR always represents a
// task and resume is the requested-changes path off one.
func TestApply_ResumeStalesOnlyMismatchedGates(t *testing.T) {
	gates := func() store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{
			"build":       {Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass, Code: "sha256:a"},
			"tests":       {Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass, Code: "sha256:b"},
			"legacy":      {Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass},
			"code_review": {Required: true, Kind: store.GateKindAttestation, Status: store.GateStatusPass},
		}}
	}
	resumed := func(code string) *store.Task {
		tpr := 7
		tk := &store.Task{ID: "T-aaaaaa", Status: "in_review", Owner: "jc", Started: true, PR: &tpr, GatesBlock: gates()}
		require.NoError(t, Apply(tk, "resume", Payload{Reason: "review requested changes", CodeIdentity: code}))
		assert.Equal(t, "in_progress", tk.Status, "task resume goes in_review -> in_progress")
		require.NotNil(t, tk.PR, "resume keeps the recorded PR")
		assert.Equal(t, 7, *tk.PR)
		return tk
	}

	tk := resumed("sha256:a")
	assert.Equal(t, store.GateStatusPass, tk.Gates["build"].Status, "same code stays pass")
	assert.Equal(t, store.GateStatusStale, tk.Gates["tests"].Status, "other code goes stale")
	assert.Equal(t, store.GateStatusStale, tk.Gates["legacy"].Status, "no recorded code goes stale")
	assert.Equal(t, store.GateStatusStale, tk.Gates["code_review"].Status, "an attestation always goes stale")

	tk = resumed("")
	for name, g := range tk.Gates {
		assert.Equal(t, store.GateStatusStale, g.Status, "%s: an empty resume code stales everything", name)
	}
}

// TestStaleGates covers the rule at the grain resume cannot reach. StaleGates is
// asked from two places, and only one of them is a verb: `resume` reaches it
// through Apply's side effects at task grain, while sync's rollup recompute
// calls it directly on a FEATURE when a done feature reopens because a task
// landed beneath it (mutate/sync.go, reopenFeature). The feature arm therefore
// has no transition to ride in on and needs its own test, or the engine's own
// package stops covering half of a function it exports.
func TestStaleGates(t *testing.T) {
	green := func() store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{
			"build": {Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass},
			"tests": {Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass},
		}}
	}

	f := &store.Feature{ID: "F-aaaaaa", Status: "done", GatesBlock: green()}
	StaleGates(f)
	assert.Equal(t, store.GateStatusStale, f.Gates["build"].Status)
	assert.Equal(t, store.GateStatusStale, f.Gates["tests"].Status)

	tk := &store.Task{ID: "T-aaaaaa", Status: "in_review", Owner: "jc", Started: true, GatesBlock: green()}
	StaleGates(tk)
	assert.Equal(t, store.GateStatusStale, tk.Gates["build"].Status)
	assert.Equal(t, store.GateStatusStale, tk.Gates["tests"].Status)

	// A noun with no gate block at all is left alone rather than panicking: the
	// epic's acceptance lives at feature grain, so it has nothing to stale.
	StaleGates(&store.Epic{ID: "E-aaaaaa", Status: "done"})
}

// TestStaleGatesUnlessCodeExpiresCodelessCriteriaInAKeptGate pins that a kept
// evals gate still expires what stores no code: a command criterion over the
// matching code stays pass (the next eval reuses it per criterion), while a
// manual criterion, whose human pass carries no code, goes stale and takes the
// gate with it.
func TestStaleGatesUnlessCodeExpiresCodelessCriteriaInAKeptGate(t *testing.T) {
	f := &store.Feature{GatesBlock: store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {
			Kind:   store.GateKindCommand,
			Status: store.GateStatusPass,
			Code:   "sha256:a",
			Criteria: map[string]store.Criterion{
				"C1": {Status: store.CriterionStatusPass, Code: "sha256:a"},
				"C2": {Status: store.CriterionStatusPass},
			},
		},
	}}}
	StaleGatesUnlessCode(f, "sha256:a")
	g := f.Gates["evals"]
	assert.Equal(t, store.CriterionStatusPass, g.Criteria["C1"].Status, "a command criterion on matching code stays pass")
	assert.Equal(t, store.CriterionStatusStale, g.Criteria["C2"].Status, "a criterion with no code goes stale")
	assert.Equal(t, store.GateStatusStale, g.Status, "an expired criterion takes the gate with it")
}

func TestStaleGateExpiresOneGateAndItsCriteria(t *testing.T) {
	f := &store.Feature{GatesBlock: store.GatesBlock{Gates: map[string]store.Gate{
		"build": {Status: store.GateStatusPass},
		"evals": {
			Status: store.GateStatusPass,
			Criteria: map[string]store.Criterion{
				"C1": {
					Status:   store.CriterionStatusPass,
					Evidence: []store.Evidence{{Kind: "human_attestation", Ref: "review-42"}},
				},
			},
		},
	}}}

	StaleGate(f, "evals")
	StaleGate(f, "missing")

	require.Equal(t, store.GateStatusPass, f.Gates["build"].Status)
	require.Equal(t, store.GateStatusStale, f.Gates["evals"].Status)
	require.Equal(t, store.CriterionStatusStale, f.Gates["evals"].Criteria["C1"].Status)
	require.Equal(t, []store.Evidence{{Kind: "human_attestation", Ref: "review-42"}}, f.Gates["evals"].Criteria["C1"].Evidence)
}

// TestStaleGatesStalesCriteriaToo pins the criterion grain of the same rule:
// every per-criterion status inside the evals gate stales along with the gate
// itself, including a human's attested pass, because that attestation was given
// before the new work existed and must not ride across the reopen. Evidence is
// left alone: staling changes the verdict, not the trail behind it.
func TestStaleGatesStalesCriteriaToo(t *testing.T) {
	f := &store.Feature{ID: "F-aaaaaa", Status: "done"}
	f.Gates = map[string]store.Gate{
		"evals": {
			Required: true,
			Status:   store.GateStatusPass,
			Criteria: map[string]store.Criterion{
				"C1": {Status: store.CriterionStatusPass,
					Evidence: []store.Evidence{{Kind: "rubric_output", Ref: "artifacts/tiller/c1.md"}}},
				"C2": {Status: store.CriterionStatusPending},
			},
		},
	}

	StaleGates(f)

	require.Equal(t, store.CriterionStatusStale, f.Gates["evals"].Criteria["C1"].Status,
		"a human's attested pass must not survive work it never saw")
	require.Equal(t, store.CriterionStatusStale, f.Gates["evals"].Criteria["C2"].Status,
		"every criterion stales, the same uniform rule the gates themselves get")
	require.Len(t, f.Gates["evals"].Criteria["C1"].Evidence, 1,
		"staling changes the verdict, not the trail: the rubric output stays as the record of what was reviewed")
}

// TestApply_SyncCrossesToDone asserts the observed done crossing moves a task
// from in_review to done. The task is the only grain with a sync edge in the
// table: a feature's and an epic's done are rollups mutate/sync.go records
// directly, in both directions, which a one-way table row cannot express.
func TestApply_SyncCrossesToDone(t *testing.T) {
	tk := &store.Task{ID: "T-aaaaaa", Status: "in_review", Owner: "jc", Started: true}
	require.NoError(t, Apply(tk, "sync", Payload{}))
	assert.Equal(t, "done", tk.Status)
}

// TestEmittableVerbs pins the verb set the oracle can emit into a status
// legal_next answer: every transition verb of every noun the store models as a
// concrete node, plus the question ledger's answer (NextMoves' special case,
// outside the transitions table). withdraw joins the set now that `tiller
// withdraw amendment` is a registered command (internal/cli's
// amendment_cmds.go); approve was already present from the epic/feature/task
// rows. This is the enumeration the oracle-CLI parity test in internal/cli
// walks.
func TestEmittableVerbs(t *testing.T) {
	got := EmittableVerbs()
	want := []string{
		"answer", "approve", "archive", "cancel", "check",
		"resume", "return", "spec", "start", "submit", "sync", "withdraw",
	}
	assert.Equal(t, want, got, "the emittable verb set is the sorted union over the store-modeled nouns plus answer")
	assert.True(t, sort.StringsAreSorted(got))
}

// TestApply_SubmitRecordsReviewRef asserts the offline submit side effect: a
// submit carrying a ReviewRef (the remote-less branch-ref fallback) records the
// branch and head commit on the task it moves to in_review, with pr left unset.
func TestApply_SubmitRecordsReviewRef(t *testing.T) {
	ref := &store.ReviewRef{
		Branch: "tiller/form-engine/certification-form-loading",
		Head:   store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"},
	}

	tk := &store.Task{ID: "T-aaaaaa", Status: "check", Owner: "jc", Started: true}
	require.NoError(t, Apply(tk, "submit", Payload{ReviewRef: ref}))
	assert.Equal(t, "in_review", tk.Status)
	require.NotNil(t, tk.ReviewRef)
	assert.Equal(t, "tiller/form-engine/certification-form-loading", tk.ReviewRef.Branch)
	assert.Equal(t, "git:abc1234", tk.ReviewRef.Head.Value)
	assert.Nil(t, tk.PR, "the offline case never fakes a PR number")
}

func TestApplyWritesWorkItemStatusPRAndReviewRef(t *testing.T) {
	b := &store.Bug{WorkItem: store.WorkItem{ID: "B-aaaaaa", Status: "check", Owner: "jc", Started: true}}
	pr := 7
	require.NoError(t, Apply(b, "submit", Payload{PR: &pr}))
	require.Equal(t, "in_review", b.Status)
	require.Equal(t, 7, *b.PR)
	c := &store.Chore{WorkItem: store.WorkItem{ID: "CH-aaaaaa", Status: "check", Owner: "jc", Started: true}}
	ref := &store.ReviewRef{Branch: "tiller/chores/x"}
	require.NoError(t, Apply(c, "submit", Payload{ReviewRef: ref}))
	require.Equal(t, "in_review", c.Status)
	require.Equal(t, "tiller/chores/x", c.ReviewRef.Branch)
	c.Gates = map[string]store.Gate{"build": {Status: store.GateStatusPass}}
	StaleGates(c)
	require.Equal(t, store.GateStatusStale, c.Gates["build"].Status)
}

// TestNextMoves_Question pins the ledger question's two exits: an open one
// can be answered or cancelled, a closed one has no next move.
func TestNextMoves_Question(t *testing.T) {
	assert.Equal(t, []string{"answer", "cancel"}, NextMoves(&store.Question{ID: "Q-1", Status: "open"}))
	assert.Empty(t, NextMoves(&store.Question{ID: "Q-2", Status: "answered"}))
	assert.Empty(t, NextMoves(&store.Question{ID: "Q-3", Status: "cancelled"}))
}
