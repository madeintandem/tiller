package mutate

// This file is the assumption-review write path (artifact-schema.md,
// Assumption basis): once an open question a node proceeded under is
// answered, the node's assumption_basis entry for that question is out of
// date until a human reviews the answer in. ReviewAssumption is the
// dedicated clearing act `approve <noun> <node> --reviewed-assumption
// <Q-id>` drives, kept deliberately apart from --refresh-basis
// (docs/spec/cli-surface.md) so an unrelated basis refresh can never silently
// assert an answer was considered. UnreviewedAssumptions is the one predicate
// behind both the submit guard (Task 6) and the answered_question drift marker
// (Task 7), living here once so the verb that refuses and the report that nags
// can never disagree about what "unreviewed" means.

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// eventReviewAssumption is approve --reviewed-assumption's own event type.
// It does not reuse link.feature or link.surface.feature: an event type is
// the one thing a consumer (the recap's burst phrasing, a node's own
// timeline) can group and phrase by without parsing the payload, and
// "a recorded answer was reviewed in" is neither a dependency edge nor a
// surface recording, the same lesson surface.go's header already draws for
// its own pair (drift design: two facts do not share one type).
const eventReviewAssumption = "review.assumption"

// UnreviewedAssumption names one assumption_basis entry whose question has
// been answered and not yet reviewed in: the node that carries the entry
// (always a feature; assumption_basis has no task-grain field, see
// assumptionFeatureFor) and the question whose answer it has not seen.
type UnreviewedAssumption struct {
	Noun       string
	NodeID     string
	QuestionID string
	Title      string
	Statement  string
}

// UnreviewedAssumptions is the one predicate behind both the submit guard
// and the answered_question drift marker, kept in one place so the verb
// that refuses and the report that nags can never disagree about what
// "unreviewed" means. The rule is !entry.AnswerRevision.SameAs(
// question.AnswerRevision): it covers the never-reviewed zero value and the
// re-answered-after-review case with the same comparison, and it leaves an
// open question off the list for free, since AnswerQuestion is the only
// writer of Question.AnswerRevision and an open question's is always the
// zero value too, matching a never-reviewed entry rather than diverging
// from it. SameAs, not ==: RevisionRef carries a time.Time, whose ==
// compares by *Location pointer too, so a ledger round trip
// ("+00:00" parsing to a fixed zone, re-marshaled as "Z", reloaded as UTC)
// can make a value compare unequal to itself forever (store.RevisionRef.
// SameAs's own doc comment has the reproduction). For a task, n walks up to
// its parent feature, the same walk-up the basis guard makes
// (assumptionFeatureFor).
func UnreviewedAssumptions(tree *store.Tree, n store.Node) []UnreviewedAssumption {
	feat := assumptionFeatureFor(tree, n)
	if feat == nil {
		return nil
	}

	// Map iteration is not deterministic and both callers (a refusal, a
	// drift report) need a stable list to render.
	ids := make([]string, 0, len(feat.AssumptionBasis))
	for qid := range feat.AssumptionBasis {
		ids = append(ids, qid)
	}
	sort.Strings(ids)

	var out []UnreviewedAssumption
	for _, qid := range ids {
		q := tree.Questions[qid]
		if q == nil {
			// A dangling reference (a hand-edited assumption_basis naming a
			// question the ledger no longer carries) has no answer to
			// compare against, so it is not this predicate's business: the
			// drift engine's own sweep reports it as Uncovered,
			// never silently folded into "unreviewed".
			continue
		}
		entry := feat.AssumptionBasis[qid]
		if !entry.AnswerRevision.SameAs(q.AnswerRevision) {
			out = append(out, UnreviewedAssumption{
				Noun: feat.Noun(), NodeID: feat.ID, QuestionID: qid,
				Title: gen.QuestionTitle(q), Statement: q.Statement,
			})
		}
	}
	return out
}

// assumptionFeatureFor resolves the feature whose assumption_basis n's
// review actually reads: n itself when it is a feature, or n's parent
// feature when it is a task, since AssumptionBasis sits on the Feature
// struct alone (internal/store/node.go) and store.Task carries no field of
// its own. An epic or a question carries no
// assumption_basis at all and resolves to nil, the same as a task whose
// FeatureID names nothing the tree loaded.
func assumptionFeatureFor(tree *store.Tree, n store.Node) *store.Feature {
	switch v := n.(type) {
	case *store.Feature:
		return v
	case *store.Task:
		return tree.Features[v.FeatureID]
	default:
		return nil
	}
}

// ReviewAssumptionResult is the outcome of ReviewAssumption: the feature's
// identity and the question reviewed in. Changed is false on the idempotent
// no-op where the stamp already matched, the same signal every other mutate
// result uses to let a retry be told from a real move.
type ReviewAssumptionResult struct {
	Noun       string
	ID         string
	Key        string
	QuestionID string
	Changed    bool
}

// ReviewAssumption stamps questionID's current answer revision into node's
// assumption_basis entry for it: the deliberate clearing act
// `approve <noun> <node> --reviewed-assumption <Q-id>` drives
// (artifact-schema.md, Assumption basis). node arrives pre-resolved, like
// SetSurface's; assumption_basis lives on a feature alone, so a task or an
// epic refuses here the same way a wrong grain refuses SetSurface, and it
// gets the same reason a missing entry gets (unknown_assumption), since from
// the caller's perspective both are "nothing here to review".
func ReviewAssumption(tree *store.Tree, deps Deps, node store.Node, questionID string) (ReviewAssumptionResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ReviewAssumptionResult{}, err
	}

	feat, ok := node.(*store.Feature)
	if !ok {
		return ReviewAssumptionResult{}, &usageError{
			reason: "unknown_assumption",
			msg: fmt.Sprintf(
				"assumption_basis lives on a feature alone (artifact-schema.md, Assumption basis); %s %s carries no entry for %s",
				node.Noun(), node.NodeID(), questionID),
		}
	}
	entry, has := feat.AssumptionBasis[questionID]
	if !has {
		return ReviewAssumptionResult{}, &usageError{
			reason: "unknown_assumption",
			msg:    fmt.Sprintf("feature %s carries no assumption_basis entry for %s", feat.ID, questionID),
		}
	}
	q := tree.Questions[questionID]
	if q == nil {
		return ReviewAssumptionResult{}, &usageError{
			reason: "unknown_assumption",
			msg:    fmt.Sprintf("question %s is not in the ledger", questionID),
		}
	}
	if !questionHasAnswer(q) {
		if q.Status == statusCancelled {
			return ReviewAssumptionResult{}, &usageError{
				reason: "question_not_answered",
				msg:    fmt.Sprintf("question %s was cancelled; there is no answer to review in", questionID),
			}
		}
		return ReviewAssumptionResult{}, &usageError{
			reason: "question_not_answered",
			msg:    fmt.Sprintf("question %s is still open; there is no answer to review in", questionID),
		}
	}
	// questionHasAnswer can be true by status alone (a hand-authored ledger
	// entry carrying `status: answered` with no answer_revision key, the same
	// hand-authored case question_test.go's own status test covers). Assumption
	// review requires a recorded answer revision too: without this check, zero
	// AnswerRevision values would compare SameAs-equal below and report a clean
	// stamp for a question that was never actually given a revision to stamp.
	if q.AnswerRevision.SameAs(store.RevisionRef{}) {
		return ReviewAssumptionResult{}, &usageError{
			reason: "question_not_answered",
			msg:    fmt.Sprintf("question %s carries no answer revision; there is nothing recorded yet to review in", questionID),
		}
	}

	res := ReviewAssumptionResult{Noun: feat.Noun(), ID: feat.ID, Key: deref(feat.Key), QuestionID: questionID}
	if entry.AnswerRevision.SameAs(q.AnswerRevision) {
		// House idempotency: the stamp already matches, so a retry is a
		// clean no-op, no write and no event, rather than a redundant one.
		return res, nil
	}
	if feat.Key == nil {
		return ReviewAssumptionResult{}, fmt.Errorf("mutate: feature %s has no key in its feature.yml; restore the key line before reviewing", feat.ID)
	}

	// Re-read the feature's .yml fresh rather than trust the in-memory tree,
	// the same read-modify-write setNodeSurface and setNodeDependsOn use for
	// a folder node's own file.
	relYml := filepath.Join(".tiller", "epics", feat.EpicKey, "features", localKey(*feat.Key), "feature.yml")
	path := filepath.Join(tree.Root, relYml)
	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, path, &doc)
	if err != nil {
		return ReviewAssumptionResult{}, err
	}
	doc.EpicKey = feat.EpicKey
	fresh, has := doc.AssumptionBasis[questionID]
	if !has {
		// The in-memory tree and disk disagreed between validation and
		// write, a live-race window rather than ordinary misuse: the earlier
		// checks already told the caller "unknown_assumption" is a usage
		// question, so this is reported as a plain fault instead.
		return ReviewAssumptionResult{}, fmt.Errorf("mutate: feature %s's assumption_basis entry for %s vanished between read and write", feat.ID, questionID)
	}
	// Stamp only the answer revision. Reason is the human's recorded
	// justification for proceeding, and reviewing in must never erase it.
	fresh.AnswerRevision = q.AnswerRevision
	doc.AssumptionBasis[questionID] = fresh

	data, err := yaml.Marshal(doc)
	if err != nil {
		return ReviewAssumptionResult{}, err
	}

	// Command is the actual runnable invocation, not an invented verb: there
	// is no `tiller review assumption`, and a reader of the event stream
	// should be able to paste this back in.
	command := fmt.Sprintf("approve feature %s --reviewed-assumption %s", feat.ID, questionID)
	ev := events.NewEvent(deps.clock(), eventReviewAssumption, owner.ID, command, feat.ID)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The
	// eval model: the AT- and EV- classes are separate non-node
	// identifiers).
	ev.ID = deps.mintRaw("EV")
	// Before carries the prior stamp (the zero value on a first review), so
	// the eventual re-answered-after-review case is legible in the stream
	// without a diff against the previous event.
	ev.Before = map[string]any{"answer_revision": entry.AnswerRevision}
	ev.After = map[string]any{"question": questionID, "answer_revision": q.AnswerRevision}
	ev.Files = []events.FileChange{{Path: relYml}}

	if err := commitMutation(tree, "review assumption", []store.PlannedWrite{
		store.PlanWrite(path, snapshot, data),
	}, []events.Event{ev}); err != nil {
		return ReviewAssumptionResult{}, err
	}
	// Keep the caller's in-memory tree in step with what was just written.
	// Tasks 6 and 7 may call ReviewAssumption and then UnreviewedAssumptions
	// in the same process without an intervening store.Load, and without
	// this the in-memory feat still carries the stale entry the disk copy
	// no longer does.
	feat.AssumptionBasis[questionID] = fresh

	res.Changed = true
	return res, nil
}
