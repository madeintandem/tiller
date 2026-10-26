package lifecycle

import "github.com/madeintandem/tiller/internal/store"

// This file is the declarative source of truth for the state machine. It
// encodes BOTH dimensions of one spec, written down once:
//
//   - the status lifecycle per node type (artifact-schema.md, "Lifecycles,
//     consolidated" and the per-node States sections), as the `transitions`
//     table; and
//   - who may run each verb (cli-surface.md, "Who may run what" and the Command
//     reference Actor column), as the `verbActors` map.
//
// The engine in lifecycle.go reads these tables and adds no rules of its own.
// Keeping the data here and the logic there is what lets the executable spec
// test in lifecycle_test.go compare an independent hand-transcription of the
// spec against this encoding.

// Actor classifies who the spec expects to run a verb: the agent in its loop,
// the human at a gate, or either. The human-only set is the boundary the
// non-coercion principle draws (cli-surface.md, "The actor model"); the agent
// and either sets are advisory guidance the agent is asked to honor.
type Actor string

const (
	ActorAgent  Actor = "agent"
	ActorHuman  Actor = "human"
	ActorEither Actor = "either"
)

// transition is one documented status edge: from a state, a verb advances a node
// of a given type to a new state.
type transition struct {
	From string
	Verb string
	To   string
}

// workItemRows is the task's row set, shared by the two shelf nouns: a bug
// or a chore is a sub-class of task and carries its lifecycle verbatim
// (docs/spec/cli-surface.md). The carried crossing (resolved_by, sync) is not a
// row here: sync sets done directly on it, as it does for a rollup.
var workItemRows = []transition{
	{From: "proposed", Verb: "approve", To: "ready"},
	{From: "proposed", Verb: "cancel", To: "cancelled"},
	{From: "ready", Verb: "start", To: "in_progress"},
	{From: "ready", Verb: "return", To: "proposed"},
	{From: "ready", Verb: "cancel", To: "cancelled"},
	{From: "in_progress", Verb: "check", To: "check"},
	{From: "in_progress", Verb: "cancel", To: "cancelled"},
	{From: "check", Verb: "submit", To: "in_review"},
	{From: "check", Verb: "cancel", To: "cancelled"},
	{From: "in_review", Verb: "resume", To: "in_progress"},
	{From: "in_review", Verb: "sync", To: "done"},
	{From: "in_review", Verb: "cancel", To: "cancelled"},
}

// transitions maps a node noun to its ordered status transition rows. Orthogonal
// operations that do not change status (block/unblock, claim/release) are not
// transitions and are deliberately absent. The done crossing is encoded as the
// `sync` verb because that is the command that observes a merge and records the
// crossing (cli-surface.md, "The done crossing").
var transitions = map[string][]transition{
	// The epic has no stored 'active' state and no verb that reaches done. Its
	// one human gate on the way in is approve (draft -> ready, approving the
	// decomposition tree) and its one on the way out is archive (done ->
	// archived, the human's deliberate close). done itself is absent from this
	// table on purpose: epic done is a computed fact sync records as a rollup
	// observation when the last non-cancelled feature crosses, and it recomputes
	// back to ready when new work lands beneath, so neither direction is a verb
	// anyone types (artifact-schema.md, Epics States; mutate/sync.go's epic
	// pass). That recompute is also why there is no reactivate operation. An
	// epic's being-worked status is DERIVED from its children at read time
	// (gen.EpicActivity), never stored, so there is no 'start epic' either.
	store.NounEpic: {
		{From: "draft", Verb: "approve", To: "ready"},
		{From: "draft", Verb: "cancel", To: "cancelled"},
		{From: "ready", Verb: "cancel", To: "cancelled"},
		{From: "done", Verb: "archive", To: "archived"},
	},
	// Feature activation is part of the first successful task-start transaction,
	// not a feature verb. Aggregate submission is handled by the review-grain
	// mutation path without adding an in_review feature state. Completion and
	// reopening remain rollups recorded by sync.
	store.NounFeature: {
		{From: "draft", Verb: "spec", To: "specced"},
		{From: "draft", Verb: "cancel", To: "cancelled"},
		{From: "specced", Verb: "approve", To: "ready"},
		{From: "specced", Verb: "cancel", To: "cancelled"},
		{From: "ready", Verb: "return", To: "specced"},
		{From: "ready", Verb: "cancel", To: "cancelled"},
		{From: "active", Verb: "cancel", To: "cancelled"},
	},
	store.NounTask: {
		{From: "proposed", Verb: "approve", To: "ready"},
		{From: "proposed", Verb: "cancel", To: "cancelled"},
		{From: "ready", Verb: "start", To: "in_progress"},
		{From: "ready", Verb: "return", To: "proposed"},
		{From: "ready", Verb: "cancel", To: "cancelled"},
		{From: "in_progress", Verb: "check", To: "check"},
		{From: "in_progress", Verb: "cancel", To: "cancelled"},
		{From: "check", Verb: "submit", To: "in_review"},
		{From: "check", Verb: "cancel", To: "cancelled"},
		{From: "in_review", Verb: "resume", To: "in_progress"},
		{From: "in_review", Verb: "sync", To: "done"},
		{From: "in_review", Verb: "cancel", To: "cancelled"},
	},
	store.NounBug:   workItemRows,
	store.NounChore: workItemRows,
	store.NounTweak: workItemRows,
	// The amendment's own status writes do not run through this engine, unlike
	// every noun above it. Its three verbs (AddAmendment, ApproveAmendment, and
	// WithdrawAmendment in internal/core/mutate) set status directly, each with
	// its own named refusal (amendment_not_open) rather than the engine's
	// IllegalTransitionError, because approve amendment is not one status move
	// but a whole batch of operations applied under one lock. These rows are
	// still the spec's transcription of the amendment lifecycle
	// (artifact-schema.md, Amendments: States), and NextMoves reads them to
	// answer what a proposed amendment may do next.
	store.NounAmendment: {
		{From: "proposed", Verb: "approve", To: "approved"},
		{From: "proposed", Verb: "withdraw", To: "withdrawn"},
	},
}

// verbActors is the who-may-run dimension, keyed by verb. The actor is uniform
// across node types for a given verb (approve is always a human gate, spec is
// always the agent's assertion, and so on), so a single verb-keyed map is
// the non-duplicating source. Verbs absent from this map default to ActorEither.
//
// The mapping follows cli-surface.md's Command reference Actor column:
//   - human-only gates: approve, return, cancel, archive, withdraw,
//     attest, reassign, transfer, answer (a question's answer is the human's)
//   - the agent's assertion: spec
//   - shared "owner"/"agent or human" verbs: start, check, submit, resume, sync
//
// approve is the start gate (approve the plan: an epic decomposition, a feature
// spec, a task, an amendment), a human decision the agent drives to the edge of
// and stops at; its keystrokes are delegable on an explicit human instruction
// naming the nodes. archive is the human's other judgment moment, the deliberate
// close of a done epic. Nothing here accepts completed work, because nothing
// does: done above a task is a rollup sync computes (artifact-schema.md, "Done
// above a task is computed, and accept is gone"), and that same recompute is why
// there is no reactivate verb either.
//
// attest and reassign are human-only by the spec (cli-surface.md, "Who may run
// what": attest is a human attestation, reassign is the human handoff path).
// They were flagged in T10 as defaulting to ActorEither because no lifecycle row
// references them; they change no status, so they live in verbActors alone rather
// than in transitions. The eval handoff (T13) closes that gap here.
var verbActors = map[string]Actor{
	"approve":  ActorHuman,
	"return":   ActorHuman,
	"cancel":   ActorHuman,
	"archive":  ActorHuman,
	"withdraw": ActorHuman,
	"attest":   ActorHuman,
	"reassign": ActorHuman,
	"transfer": ActorHuman,
	"answer":   ActorHuman,
	"spec":     ActorAgent,
	"start":    ActorEither,
	"check":    ActorEither,
	"submit":   ActorEither,
	"resume":   ActorEither,
	"sync":     ActorEither,
}
