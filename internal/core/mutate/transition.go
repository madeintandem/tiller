package mutate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// This file holds the lifecycle-transition mutation the CLI verbs (approve,
// spec, start, return, resume, submit) drive. It is the layer that sits
// between the pure lifecycle engine (internal/core/lifecycle, which only answers
// legality and mutates an in-memory node) and the file store: it reads the node's
// on-disk doc, runs the verb-specific guards the engine does not model (spec
// validation, the structural-blocker guard, basis recording),
// applies the legal transition, writes the node file atomically under the
// repository mutation lock, and records one event. An illegal move returns the
// engine's typed *lifecycle.IllegalTransitionError unchanged so the CLI can render
// the recoverable exit-1 envelope; a guard refusal returns one of the typed
// blocked errors below.
//
// approve is the start gate (approve the plan: an epic decomposition draft->ready,
// a feature plan specced->ready with its initial tasks, a task proposed->ready).
// No verb here reaches done at any grain: a task's done is the merge, and a
// feature's and an epic's are rollup observations sync records over their children
// (sync.go; artifact-schema.md, "Done above a task is computed, and accept is
// gone").
//
// The active-basis guard runs at both grains that carry a basis. At feature grain
// it runs on start and check (guardFeature) over the feature's accepted_against
// spec/eval revisions; at task grain it runs on start, check, and submit
// (guardTask) over the parent feature's pair and the task's own task.md pin. Each
// comparison is against the current working-tree git blob, and each refusal is the
// active_basis blocked answer naming the node whose basis moved, which
// approve <noun> --refresh-basis re-records through the same read-only git window
// (Deps.rev).

// featureStubMarker is the sentinel line `tiller add feature` writes into a fresh
// feature.md stub. spec validation treats a feature.md still carrying it as not
// yet written. It duplicates the literal in renderFeatureMD (add.go) rather than
// editing that function; the two are tied by this comment.
const featureStubMarker = "Draft feature created by `tiller add feature`"

// epicStubMarker is the sentinel line `tiller add epic` writes into a fresh
// epic.md stub. The read model treats an epic.md still carrying it as not yet
// written (IsProseStub below, which the console and decompose's readout both
// ask). No gate reads it anymore, since the epic has no done gate to guard. It
// duplicates the literal in renderEpicMD (add.go), tied by this comment.
const epicStubMarker = "Draft epic created by `tiller add epic`"

// taskStubMarker is the sentinel line `tiller add task` writes into a fresh
// task.md, the third entry in proseStubMarkers. A task is a folder node with an
// implementation spec of its own (artifact-schema.md, Tasks), so the same
// written-yet question the epic and the feature answer applies to it.
const taskStubMarker = "Proposed task created by `tiller add task`"

// TransitionInput is the request for a lifecycle transition: the verb, plus the
// optional payload the few verbs that carry one need. PR is recorded by submit;
// ReviewRef is the branch-ref review surface a remote-less submit records
// instead of a PR (the CLI reads it from git and never sets both); Reason is
// the justification return and resume record and the assumption reason
// start/submit attach to an --assume-question. RefreshBasis re-records a feature's
// accepted_against without changing its state (approve feature --refresh-basis);
// AssumeQuestion records an open question into the node's assumption_basis so a
// known-risk start/submit can proceed. SyncBasis and SyncMatch are set only by
// Sync, and record what evidence a done crossing rested on.
type TransitionInput struct {
	Verb           string
	PR             *int
	ReviewRef      *store.ReviewRef
	Reason         string
	RefreshBasis   bool
	AssumeQuestion string
	// SyncBasis is what a done crossing was recorded on: one of "ancestry",
	// "content", "rollup", or "human_attested". Each token's meaning, and which
	// grain records which of them, are set out with the basis constants in
	// sync.go rather than restated here.
	//
	// It lands on the event and on the node's done_basis. On the event it is the
	// audit record: the event's owner is the actor who recorded the crossing, and
	// on a human_attested crossing that is the attester, with the timestamp saying
	// when the assertion was made.
	SyncBasis string
	// SyncMatch is the integration-branch commit whose patch matched, in the
	// git:<full-sha> form, set only alongside SyncBasis "content".
	SyncMatch string
	// ReviewGrain is set only by approve: the epic or feature plan gate's own
	// --review-grain flag, recording the node's review grain in the same act
	// as the gate (docs/spec/cli-surface.md). Nil means the caller named no
	// grain, leaving whatever the node already carries untouched; every other
	// verb ignores this field.
	ReviewGrain *string
	// Resolution and DuplicateOf are cancel's own fields on a bug or chore:
	// wontfix, invalid, or duplicate, and on duplicate the surviving node
	// (docs/spec/cli-surface.md). Every
	// other verb and every tree noun ignores both.
	Resolution  string
	DuplicateOf string
	// AdoptAggregate is start task --adopt-aggregate: record the
	// existing local ref at the derived aggregate name instead of minting it.
	// It is refused on any other noun and at task grain; every other verb
	// ignores it.
	AdoptAggregate bool
	// ReusedGates is set only by check: the gates, and evals/<criterion id>
	// criteria, this check reused rather than ran (design
	// 2026-10-21-gate-reuse), recorded on the event as `reused` so a later
	// analysis can count avoided runs. Every other verb ignores it.
	ReusedGates []string
	// CodeIdentity is set by mutate, never by a caller: on resume, the code
	// identity the engine compares to each command gate's recorded code to decide
	// which gates stay pass. Every other verb ignores it.
	CodeIdentity string
}

// TransitionResult projects the recorded state. Branch identifies the task's
// immutable working branch on start/resume, or the aggregate branch on feature
// start. BaseBranch names the safe base for a newly started task.
// AdoptedCommits lists, newest first, what an adopted aggregate held beyond
// integration when start --adopt-aggregate recorded it.
type TransitionResult struct {
	Noun            string
	ID              string
	Key             string
	Status          string
	Branch          string
	BaseBranch      string
	AggregateBranch string
	PR              *int
	ReviewRef       *store.ReviewRef
	Changed         bool
	AdoptedCommits  []string

	// Warnings are advisories a transition succeeded alongside, never refusals.
	// Only `spec` on a feature fills it: the preserve criteria that merely
	// repeat the required tests gate (gates.CoveredPreserveCriteria).
	Warnings []string

	// Tasks are the tasks one transition moved alongside the node it was
	// addressed at. Only the feature plan gate fills it (approve, specced to
	// ready, which flips the initial proposed tasks in the same act), because
	// that is the only transition here that moves more than the node named
	// (cli-surface.md, "Specifying a feature").
	Tasks []TransitionResult
}

// reasonRequired names the verbs whose --reason is not optional. All three are
// human judgment calls that a later reader cannot reconstruct: return and resume
// reverse or re-enter a gate, and cancel is terminal and is the tool's only
// removal verb, so a cancelled node with no reason is a dead branch nobody can
// explain (cli-surface.md, the verb table).
//
// The check is here rather than only on the cobra flag because MarkFlagRequired
// tests presence, not content: `--reason ""` satisfies it. Trimming here is what
// makes the requirement mean something.
var reasonRequired = map[string]bool{
	"return": true,
	"resume": true,
	"cancel": true,
}

func validateReason(in TransitionInput) error {
	if !reasonRequired[in.Verb] || strings.TrimSpace(in.Reason) != "" {
		return nil
	}
	return &usageError{
		reason: "missing_reason",
		msg:    fmt.Sprintf("%s needs a non-empty --reason", in.Verb),
	}
}

// Transition applies a legal lifecycle verb to a resolved node and persists it.
// It validates the reason a verb requires first (a blank reason costs no tree
// work), then resolves the acting contributor (an unset or unknown contributor is the
// exit-3 refusal every mutation shares), then dispatches on the concrete node
// type. A node type the lifecycle verbs do not yet support is a clear misuse
// rather than a silent miss.
func Transition(tree *store.Tree, deps Deps, node store.Node, in TransitionInput) (TransitionResult, error) {
	if err := validateReason(in); err != nil {
		return TransitionResult{}, err
	}
	// Only a task rides an aggregate; a bug, chore, or tweak has none to adopt.
	if in.AdoptAggregate && node.Noun() != store.NounTask {
		return TransitionResult{}, &BranchSafetyError{Reason: "aggregate_adopt_grain", Noun: node.Noun(), NodeID: node.NodeID()}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return TransitionResult{}, err
	}
	return applyTransition(tree, deps, owner, node, in)
}

// applyTransition dispatches a transition on the concrete node type. A node type the
// lifecycle verbs do not yet support is a clear misuse rather than a silent
// miss.
func applyTransition(tree *store.Tree, deps Deps, owner store.Contributor, node store.Node, in TransitionInput) (TransitionResult, error) {
	switch n := node.(type) {
	case *store.Epic:
		return transitionEpic(tree, deps, owner, n, in)
	case *store.Feature:
		return transitionFeature(tree, deps, owner, n, in)
	case *store.Task:
		return transitionTask(tree, deps, owner, n, in)
	default:
		if w, ok := store.AsWorkNode(node); ok {
			return transitionWorkItem(tree, deps, owner, w, in)
		}
		return TransitionResult{}, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("lifecycle verbs do not support a %s yet", node.Noun()),
		}
	}
}

// transitionEpic applies a transition to an epic.yml. The epic has exactly one
// human gate on the way in, approve (draft -> ready, approving the decomposition
// tree), and one on the way out, archive (done -> archived, the human's
// deliberate close). Its done is not a verb at all: sync records it as a rollup
// observation over the epic's features and recomputes it when new work lands
// beneath (artifact-schema.md, Epics States), so there is no guard here to run
// and no accept to run it for. There is no stored 'active' epic state either;
// the epic's being-worked status is derived from its children at read time
// (gen.EpicActivity).
func transitionEpic(tree *store.Tree, deps Deps, owner store.Contributor, treeEpic *store.Epic, in TransitionInput) (TransitionResult, error) {
	key := deref(treeEpic.Key)
	relDir := filepath.Join(".tiller", "epics", key)
	ymlRel := filepath.Join(relDir, "epic.yml")

	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return TransitionResult{}, err
	}
	before := doc.Status
	mutated := false
	reviewGrainBefore := ""
	var reviewGrainGuards []store.PlannedWrite

	// approve's own --review-grain records the epic's review grain in the
	// same act as the gate (docs/spec/cli-surface.md). This runs before
	// lifecycle.Apply so an illegal value refuses before any write, whether or
	// not the gate itself moves status (a re-approve of an already-ready epic
	// still records a changed grain). reviewGrainBefore captures what the field
	// held prior to the overwrite, so the event recorded below can carry it on
	// the Before side; it stays "" whenever ReviewGrain is nil, which
	// transitionEvent's own guard leaves unread.
	//
	// The Changeability refusal (refuseReviewSurfaceLocked, configure.go)
	// runs here too, ahead of the overwrite: an epic has no stored `active`
	// status, so a submitted epic-grain epic (its own PR or review ref
	// already recorded via `submit epic`) sits at `ready` exactly like an
	// unsubmitted one, and without this check a re-approve here would
	// silently overwrite a grain the rule says is locked.
	// It never fires on the initial draft->ready approve, since a draft epic
	// cannot yet carry a PR, a review ref, or a terminal status.
	if in.Verb == "approve" && in.ReviewGrain != nil {
		if !store.ValidReviewGrain(store.NounEpic, *in.ReviewGrain) {
			return TransitionResult{}, &usageError{
				reason: "invalid_review_grain",
				msg:    fmt.Sprintf("mutate: %q is not a legal review grain for an epic; task, feature, or epic", *in.ReviewGrain),
			}
		}
		terminal := doc.Status == statusCancelled || doc.Status == statusArchived
		if err := refuseReviewSurfaceLocked(store.NounEpic, doc.ID, key, doc.Status, doc.PR, doc.ReviewRef, terminal); err != nil {
			return TransitionResult{}, err
		}
		reviewGrainGuards, err = guardReviewGrainChange(tree, reviewGrainChange{
			Noun: store.NounEpic, Epic: &doc, Desired: *in.ReviewGrain,
		})
		if err != nil {
			return TransitionResult{}, err
		}
		reviewGrainBefore = doc.ReviewGrain
		doc.ReviewGrain = *in.ReviewGrain
		mutated = true
	}

	if err := lifecycle.Apply(&doc, in.Verb, payloadFor(in)); err != nil {
		return TransitionResult{}, err
	}
	after := doc.Status
	if after == before && !mutated {
		return epicResult(&doc, false), nil
	}

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := commitTransitionWithGuards(tree, deps, owner, in, store.NounEpic, doc.ID, ymlRel, ydata,
		snapshot, before, after, reviewGrainBefore, reviewGrainGuards); err != nil {
		return TransitionResult{}, err
	}
	return epicResult(&doc, true), nil
}

// taskOnlyFeatureVerbs are the verbs a feature can never take through THIS
// engine because they are about a PR, and record-only submit aside, a PR
// still always represents a task or, at a coarse review grain, the feature
// or epic that is the reviewed node (docs/spec/cli-surface.md). Each remaining
// entry maps to the move the caller meant.
//
// submit is deliberately absent now: it carries its own feature-grain and
// epic-grain meaning, served by mutate.SubmitNode (submit_node.go), which
// writes pr/review_ref straight onto the reviewed node's own file and never
// runs a status transition at all. The CLI routes `submit <noun>` to
// SubmitNode for feature and epic and to this lifecycle engine for task, so
// a feature or epic noun reaching THIS function's own "submit" arm remains
// impossible in ordinary use; a caller that reaches it anyway (bypassing the
// CLI, as a core-level test does) gets the engine's plain "not legal from
// here" answer, because the feature transitions table below carries no
// submit row for it to special-case. resume stays here: it has no coarser-
// grain meaning of its own at any review grain, so naming it on a feature is
// still the same lost-track-of-what-it-is-operating-on misuse it always was.
var taskOnlyFeatureVerbs = map[string]string{
	"start":  "tiller start task <task>",
	"resume": "tiller resume task <task> --reason <text>",
}

// transitionFeature applies a transition to a feature.yml. It runs the
// verb-specific guards: spec validates the whole second-stage output before
// draft -> specced (validateSpec: the specification, the acceptance contract,
// and the task explosion); start runs the active-basis and structural-blocker
// guards; approve on a specced feature is the plan gate over the feature and its
// initial tasks, and approve --refresh-basis re-records accepted_against without
// a state move. Every one of spec, start, and approve additionally refuses first
// when the feature's own epic is cancelled or archived (epicRetired), since each
// is a doing verb that moves the feature toward work an epic in either terminal
// state can never see rolled up.
func transitionFeature(tree *store.Tree, deps Deps, owner store.Contributor, treeFeat *store.Feature, in TransitionInput) (TransitionResult, error) {
	if treeFeat.Key == nil {
		return TransitionResult{}, fmt.Errorf("mutate: feature %s has no canonical key", treeFeat.ID)
	}
	// The feature noun on a PR verb is a caller that lost track of what it is
	// operating on, which is the checksum class the grammar reserves exit 2 for
	// (cli-surface.md, "The grammar"). Without this guard the engine would answer
	// exit 1 instead, for want of a table row, and exit 1 is the wrong answer: it
	// means "not legal from here, try one of these", and no state exists that a
	// feature could reach from which submitting it becomes legal.
	if move, ok := taskOnlyFeatureVerbs[in.Verb]; ok {
		return TransitionResult{}, &usageError{
			reason: "task_only_verb",
			msg:    fmt.Sprintf("%s applies to an execution unit, not a feature. Run %s.", in.Verb, move),
		}
	}
	relDir := featureRelDir(treeFeat)
	ymlRel := filepath.Join(relDir, "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return TransitionResult{}, err
	}
	doc.EpicKey = treeFeat.EpicKey
	before := doc.Status
	mutated := false
	reviewGrainBefore := ""
	var reviewGrainGuards []store.PlannedWrite
	rev := deps.rev(tree.Root)
	if in.AssumeQuestion != "" && (in.Verb != "approve" || in.RefreshBasis || in.ReviewGrain != nil) {
		return TransitionResult{}, &usageError{reason: "assume_unsupported", msg: "record an assumption separately with approve feature --assume-question <Q-id> --reason <text>"}
	}

	// approve --refresh-basis is not a transition: it re-records the pin and leaves
	// the state where it is, so it runs ahead of the legality check below. The
	// states a spec is edited in include active, which approve cannot move a
	// feature from (cli-surface.md, "Specifying a feature").
	if in.Verb == "approve" && in.RefreshBasis {
		if err := refuseTerminalBasisRefresh(&doc); err != nil {
			return TransitionResult{}, err
		}
		evalChanged, err := refreshFeatureBasis(tree, relDir, &doc, deps, rev)
		if err != nil {
			return TransitionResult{}, err
		}
		ydata, err := yaml.Marshal(doc)
		if err != nil {
			return TransitionResult{}, err
		}
		writes := []store.PlannedWrite{store.PlanWrite(filepath.Join(tree.Root, ymlRel), snapshot, ydata)}
		var expired []expiredChild
		if evalChanged {
			var children []store.PlannedWrite
			if children, expired, err = planChildAcceptanceExpiry(tree, treeFeat); err != nil {
				return TransitionResult{}, err
			}
			writes = append(writes, children...)
		}
		ev := transitionEvent(deps, owner, in, store.NounFeature, doc.ID, ymlRel, before, before, "")
		recordExpiry(&ev, expired)
		if err := commitMutation(tree, in.Verb+"."+store.NounFeature, writes, []events.Event{ev}); err != nil {
			return TransitionResult{}, err
		}
		return featureResult(&doc, true), nil
	}
	if in.Verb == "approve" && in.AssumeQuestion != "" {
		if doc.Status != "ready" && doc.Status != "active" {
			return TransitionResult{}, &usageError{reason: "assume_unsupported", msg: "record an assumption on an approved ready or active feature"}
		}
		if in.RefreshBasis || in.ReviewGrain != nil {
			return TransitionResult{}, &usageError{reason: "assume_unsupported", msg: "record an assumption separately from basis refresh or review-grain changes"}
		}
		if _, err := guardFeature(tree, treeFeat, &doc, in, rev); err != nil {
			return TransitionResult{}, err
		}
		data, err := yaml.Marshal(doc)
		if err != nil {
			return TransitionResult{}, err
		}
		if err := commitTransition(tree, deps, owner, in, store.NounFeature, doc.ID, ymlRel, data, snapshot, before, before, ""); err != nil {
			return TransitionResult{}, err
		}
		return featureResult(&doc, true), nil
	}

	// Legality first, then the verb's own work, the order transitionTask states
	// its own case for below: a guard or a validation refusal is an answer about a
	// move the node can make, so an illegal verb has to say so rather than report
	// a spec that is not written or a question that is not answered.
	if legal, moves := lifecycle.Legal(&doc, in.Verb); !legal {
		return TransitionResult{}, &lifecycle.IllegalTransitionError{
			Noun: store.NounFeature, From: before, Verb: in.Verb, Legal: moves,
		}
	}

	var warnings []string
	switch in.Verb {
	case "spec":
		if before == statusDraft {
			// The epic arm runs ahead of validateSpec for the same reason it runs
			// ahead of every other doing verb here: writing the whole of stage 2
			// under a retired epic is work that can never roll up.
			if err := epicRetired(tree, treeFeat.EpicKey, doc.ID, store.NounFeature, doc.Status); err != nil {
				return TransitionResult{}, err
			}
		}
		// A re-spec of a specced feature re-validates its stage-2 output, which
		// can change after spec, so "no change" never vouches for an edit it
		// did not check.
		if before == statusDraft || before == statusSpecced {
			if err := validateSpec(tree, relDir, &doc, tree.FeatureTasks[treeFeat.ID]); err != nil {
				return TransitionResult{}, err
			}
			warnings = coveredPreserveWarnings(tree, relDir)
		}
	case "approve":
		// Approving a specced feature approves the whole plan, the feature and its
		// initial tasks in one act, so it leaves through its own path rather than
		// this function's single-node tail (cli-surface.md, "Specifying a feature").
		if before == statusSpecced {
			// The epic arm runs before approveFeaturePlan does any work: the plan
			// gate moves the feature and its initial tasks in one batch, and a
			// retired epic above it must refuse before that batch's first write,
			// not partway through it.
			if err := epicRetired(tree, treeFeat.EpicKey, doc.ID, store.NounFeature, doc.Status); err != nil {
				return TransitionResult{}, err
			}
			// The plan gate pins what is on disk now, which can differ from
			// what spec validated, so it validates the same output first.
			if err := validateSpec(tree, relDir, &doc, tree.FeatureTasks[treeFeat.ID]); err != nil {
				return TransitionResult{}, err
			}
			// An open question that names the feature, its epic, or any live
			// task holds the whole plan: the human is present at this gate,
			// so the question is answered here and folded into the plan
			// rather than left for an agent to wait on mid-run.
			// It runs after validateSpec, since a broken spec has to be fixed
			// regardless, and before approveFeaturePlan writes anything.
			if blockers := gen.PlanQuestionBlockers(tree, treeFeat); len(blockers) > 0 {
				return TransitionResult{}, &BlockedByQuestionsError{
					NodeID:       doc.ID,
					Noun:         store.NounFeature,
					CurrentState: doc.Status,
					Blockers:     blockers,
					Verb:         "approve",
				}
			}
			return approveFeaturePlan(tree, deps, owner, treeFeat, &doc, relDir, ymlRel, snapshot, in, rev)
		}
		// A re-approve of a feature already past specced (in practice, already
		// ready, the one other state approve is legal from: lifecycle's own
		// idempotency rule) is the plan gate's idempotent no-op: approve is
		// legal there but has no plan left to move. Before this arm,
		// --review-grain had no code path to reach here at all, since the
		// field is read only inside approveFeaturePlan above, so the caller's
		// grain was silently dropped: Changed=false, no event, no write (the
		// defect this arm fixes). It mirrors transitionEpic's own approve-time
		// grain write (validate, then set and mark mutated), plus the
		// Changeability refusal configureFeature already runs (refuseReviewSurfaceLocked,
		// configure.go): a re-approve is still a human gate over the node's
		// own review surface, so it has to honor the same rule `configure
		// feature` enforces once that surface exists or the feature is
		// terminal.
		if in.ReviewGrain != nil {
			if !store.ValidReviewGrain(store.NounFeature, *in.ReviewGrain) {
				return TransitionResult{}, &usageError{
					reason: "invalid_review_grain",
					msg:    fmt.Sprintf("mutate: %q is not a legal review grain for a feature; task, feature, or epic", *in.ReviewGrain),
				}
			}
			terminal := doc.Status == statusDone || doc.Status == statusCancelled
			if err := refuseReviewSurfaceLocked(store.NounFeature, doc.ID, deref(doc.Key), doc.Status, doc.PR, doc.ReviewRef, terminal); err != nil {
				return TransitionResult{}, err
			}
			reviewGrainGuards, err = guardReviewGrainChange(tree, reviewGrainChange{
				Noun: store.NounFeature, Epic: epicByKey(tree, doc.EpicKey), Feature: &doc, Desired: *in.ReviewGrain,
			})
			if err != nil {
				return TransitionResult{}, err
			}
			reviewGrainBefore = doc.ReviewGrain
			doc.ReviewGrain = *in.ReviewGrain
			mutated = true
		}
	}

	if err := lifecycle.Apply(&doc, in.Verb, payloadFor(in)); err != nil {
		return TransitionResult{}, err
	}
	after := doc.Status
	if after == before && !mutated {
		res := featureResult(&doc, false)
		res.Warnings = warnings
		return res, nil
	}

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := commitTransitionWithGuards(tree, deps, owner, in, store.NounFeature, doc.ID, ymlRel, ydata,
		snapshot, before, after, reviewGrainBefore, reviewGrainGuards); err != nil {
		return TransitionResult{}, err
	}
	res := featureResult(&doc, true)
	res.Warnings = warnings
	return res, nil
}

// coveredPreserveWarnings is spec's advisory for a feature contract whose
// preserve criteria only repeat the required tests gate. validateSpec has just
// accepted the contract, so a load failure here is not expected; it yields no
// warning rather than a second refusal.
func coveredPreserveWarnings(tree *store.Tree, relDir string) []string {
	ef, err := store.LoadEvalFile(tree.Root, filepath.Join(relDir, "feature.eval.md"))
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range gates.CoveredPreserveCriteria(ef, tree.Manifest) {
		out = append(out, c.Warning())
	}
	return out
}

// statusSpecced is the feature state the plan gate approves from: the whole
// stage-2 output is written and validated (artifact-schema.md, Features States).
const statusSpecced = "specced"

// statusCancelled is the terminal status a human ends a node at. The gates here
// read it to leave cancelled work out of the questions they ask about a
// feature's children, the same way the rollups do (sync.go).
const statusCancelled = "cancelled"

// statusArchived is the epic's other terminal status, the human's deliberate
// close of a done epic (done -> archive; artifact-schema.md, Epics States).
// The guards below read it exactly like statusCancelled: no verb leaves either
// one, sync's reopen arm reads done alone, and both terminal states name the
// same remedy at the feature and task grain.
const statusArchived = "archived"

// statusDone is the rollup status sync alone records at feature and task grain
// (a feature or task reaches it as a computed fact, never a verb). guardFeature
// reads it to except a feature that is itself terminal from the epic arm,
// mirroring gen.RetiredAncestor's own terminal-self rule (gen/held.go's
// terminalStates, which pairs done with cancelled the same way).
const statusDone = "done"

// approveFeaturePlan is the one human gate over the whole of stage 2
// (cli-surface.md, "Specifying a feature"). It moves the feature specced->ready,
// flips every initial proposed task ready in the same act, and records the
// active-work contract at both grains: the feature's feature.md and
// feature.eval.md into its accepted_against, and each task's task.md into that
// task's own. Every file and every event goes through commitNodeWrites as one
// batch, which is what a refused gate leans on: the partial states this gate
// would otherwise be able to leave are a feature at ready over proposed tasks,
// and tasks pinned against a basis the feature never recorded.
//
// The tasks are ProposedTasks' set, in the sibling order that function ranks
// them by; nothing here depends on that order beyond its being deterministic,
// since every task in the set reaches ready in the same act.
func approveFeaturePlan(tree *store.Tree, deps Deps, owner store.Contributor, treeFeat *store.Feature,
	doc *store.Feature, relDir, ymlRel string, snapshot store.FileSnapshot, in TransitionInput, rev vcs.Revisions) (TransitionResult, error) {

	// approve's own --review-grain records the feature's review grain in the
	// same act as the plan gate (docs/spec/cli-surface.md). Validated ahead of
	// recordBasis so an illegal value refuses before any write, including the
	// tasks the gate is about to move.
	if in.ReviewGrain != nil {
		if !store.ValidReviewGrain(store.NounFeature, *in.ReviewGrain) {
			return TransitionResult{}, &usageError{
				reason: "invalid_review_grain",
				msg:    fmt.Sprintf("mutate: %q is not a legal review grain for a feature; task, feature, or epic", *in.ReviewGrain),
			}
		}
	}
	var reviewGrainGuards []store.PlannedWrite
	if in.ReviewGrain != nil {
		var guardErr error
		reviewGrainGuards, guardErr = guardReviewGrainChange(tree, reviewGrainChange{
			Noun: store.NounFeature, Epic: epicByKey(tree, doc.EpicKey), Feature: doc, Desired: *in.ReviewGrain,
		})
		if guardErr != nil {
			return TransitionResult{}, guardErr
		}
	}

	if err := recordBasis(relDir, doc, deps, rev); err != nil {
		return TransitionResult{}, err
	}
	before := doc.Status
	reviewGrainBefore := ""
	if in.ReviewGrain != nil {
		reviewGrainBefore = doc.ReviewGrain
		doc.ReviewGrain = *in.ReviewGrain
	}
	if err := lifecycle.Apply(doc, in.Verb, payloadFor(in)); err != nil {
		return TransitionResult{}, err
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return TransitionResult{}, err
	}
	writes := []nodeWrite{{
		rel:    ymlRel,
		data:   ydata,
		before: snapshot,
		event:  transitionEvent(deps, owner, in, store.NounFeature, doc.ID, ymlRel, before, doc.Status, reviewGrainBefore),
	}}

	var moved []TransitionResult
	for _, tk := range ProposedTasks(tree, treeFeat) {
		target, taskYML, taskSnapshot, err := loadTaskFile(tree, treeFeat, tk)
		if err != nil {
			return TransitionResult{}, err
		}
		taskBefore := target.Status
		if err := recordTaskBasis(filepath.Dir(taskYML), target, deps, rev); err != nil {
			return TransitionResult{}, err
		}
		if err := lifecycle.Apply(target, in.Verb, payloadFor(in)); err != nil {
			return TransitionResult{}, err
		}
		tdata, err := yaml.Marshal(target)
		if err != nil {
			return TransitionResult{}, err
		}
		writes = append(writes, nodeWrite{
			rel:    taskYML,
			data:   tdata,
			before: taskSnapshot,
			// "" for reviewGrainBefore: transitionEvent's own noun guard
			// never reads it for a task anyway (store.Task has no
			// ReviewGrain field to have had a prior value).
			event: transitionEvent(deps, owner, in, store.NounTask, target.ID, taskYML, taskBefore, target.Status, ""),
		})
		moved = append(moved, taskResult(target, true))
	}

	if err := commitNodeWritesWithGuards(tree, "approve.feature.plan", writes, reviewGrainGuards); err != nil {
		return TransitionResult{}, err
	}
	res := featureResult(doc, true)
	res.Tasks = moved
	return res, nil
}

// transitionTask applies a transition to one task, rewriting its own task.yml.
// The file is re-read rather than marshalled from the tree's copy, so the write
// carries whatever else is on disk, and loadTaskFile refuses if the file it
// reached is not the addressed task. start and submit run the open-question arm of
// the structural-blocker guard; a task carries no assumption_basis field, so
// --assume-question is a misuse here (SEAM: task-level assumption_basis is
// unmodeled in the store).
func transitionTask(tree *store.Tree, deps Deps, owner store.Contributor, treeTask *store.Task, in TransitionInput) (TransitionResult, error) {
	feat := tree.Features[treeTask.FeatureID]
	if feat == nil || feat.Key == nil {
		return TransitionResult{}, fmt.Errorf("mutate: task %s has no resolvable parent feature", treeTask.ID)
	}
	target, ymlRel, snapshot, err := loadTaskFile(tree, feat, treeTask)
	if err != nil {
		return TransitionResult{}, err
	}
	before := target.Status
	rev := deps.rev(tree.Root)

	// approve --refresh-basis is not a transition at all: it re-records the pin and
	// leaves the state where it is, so it returns before the legality check. That
	// is what keeps it usable in the states a spec is actually edited in, a task in
	// progress or in review, none of which approve can move a task from
	// (cli-surface.md, "Specifying a feature"). It sits ahead of the guard block too,
	// and that ordering is load-bearing now: the approve arm below calls
	// guardParentState, which refuses a task whose parent feature is draft,
	// specced, or cancelled, or whose parent feature is live but its epic is
	// cancelled or archived, and a feature returned to specced for
	// rework (ready -> specced, return) is exactly the parent state a human
	// re-pins a task's drifted basis under. Without this early return sitting
	// ahead of that guard, refresh-basis would have no state left to run in.
	if in.Verb == "approve" && in.RefreshBasis {
		if err := refuseTerminalBasisRefresh(target); err != nil {
			return TransitionResult{}, err
		}
		if err := recordTaskBasis(filepath.Dir(ymlRel), target, deps, rev); err != nil {
			return TransitionResult{}, err
		}
		tdata, err := yaml.Marshal(target)
		if err != nil {
			return TransitionResult{}, err
		}
		if err := commitTransition(tree, deps, owner, in, store.NounTask, target.ID, ymlRel, tdata, snapshot, before, before, ""); err != nil {
			return TransitionResult{}, err
		}
		return taskResult(target, true), nil
	}

	// Legality is settled before the guards run, the order check.go's task grain
	// already used. A guard answers "may this node move forward given its basis and
	// its blockers", which is a question about a move the node can make: on a verb
	// that was never legal from here, answering active_basis or naming an open
	// question sends the caller to fix something that is not what stopped it.
	if legal, moves := lifecycle.Legal(target, in.Verb); !legal {
		return TransitionResult{}, &lifecycle.IllegalTransitionError{
			Noun: store.NounTask, From: before, Verb: in.Verb, Legal: moves,
		}
	}

	if in.Verb == "start" || in.Verb == "submit" {
		if err := guardTask(tree, feat, target, in, rev); err != nil {
			return TransitionResult{}, err
		}
	}
	// Judgment verbs remain actor-independent; execution requires the task's own claim.
	if workVerbs[in.Verb] {
		if err := guardExecutionClaim(store.NounTask, target.ID, deref(target.Key), target.Status, target.Started, target.Owner, owner.ID); err != nil {
			return TransitionResult{}, err
		}
	}
	baseBranch := ""
	var adoptedCommits []string
	var aggregateWrites []store.PlannedWrite
	var aggregateFiles []string
	if in.Verb == "start" && target.Branch == "" {
		var minted bool
		tree, feat, aggregateWrites, aggregateFiles, minted, err = prepareTaskAggregate(tree, rev, feat, target, in.AdoptAggregate)
		if err != nil {
			return TransitionResult{}, err
		}
		if err := guardTask(tree, feat, target, in, rev); err != nil {
			return TransitionResult{}, err
		}
		baseBranch, err = taskStartBase(tree, rev, feat, target, minted)
		if err != nil {
			return TransitionResult{}, err
		}
		// The task keeps the grain it started under through later grain
		// changes, so check, submit, and sync can follow it.
		target.StartedGrain = store.ResolvedReviewGrain(tree.Manifest, epicByKey(tree, feat.EpicKey), feat)
		if in.AdoptAggregate {
			if adoptedCommits, err = commitsBeyondIntegration(tree, rev, feat); err != nil {
				return TransitionResult{}, err
			}
		}
		if target.Key == nil {
			return TransitionResult{}, fmt.Errorf("mutate: task %s has no canonical key", target.ID)
		}
		target.Branch = branchFor(tree.Manifest, *target.Key)
		if err := guardNewBranch(tree, rev, store.NounTask, target.ID, target.Branch); err != nil {
			return TransitionResult{}, err
		}
	}
	// submit alone adds the assumption review (cli-surface.md, the guard
	// paragraph): a task whose parent feature carries an answered, unreviewed
	// assumption may not ship until a human reviews it in. This runs as its own
	// step after guardTask, not folded inside it, so a task whose basis has ALSO
	// drifted gets the active_basis refusal first, one refusal at a time.
	// start never calls this, and
	// checkTask's own guardTask call (check.go) never does either, so the
	// assumption hatch stays usable exactly as cli-surface.md describes.
	if in.Verb == "submit" {
		if err := guardSubmitAssumptions(tree, feat); err != nil {
			return TransitionResult{}, err
		}
		if err := guardSubmitGates(tree.Manifest, store.NounTask, target.ID, deref(target.Key), target.Status, &target.GatesBlock); err != nil {
			return TransitionResult{}, err
		}
		grain, _ := store.TaskReviewGrain(tree.Manifest, epicByKey(tree, feat.EpicKey), feat, target)
		if grain != store.ReviewGrainTask && in.PR != nil {
			return TransitionResult{}, wrongReviewGrainErr(grain)
		}
		if err := guardReviewIdentity(store.NounTask, target.ID, target.PR, in.PR); err != nil {
			return TransitionResult{}, err
		}
		if err := guardSubmitBranch(rev, store.NounTask, target.ID, target.Branch, in.ReviewRef); err != nil {
			return TransitionResult{}, err
		}
		if in.PR == nil {
			head, err := rev.CurrentCommit()
			if err != nil {
				return TransitionResult{}, err
			}
			in.ReviewRef = &store.ReviewRef{Branch: target.Branch, Head: head}
		}
	}

	// approve is a doing verb too: it is how the incident arc got its
	// first foothold (proposed -> ready under a draft feature, confirmation
	// routing straight into execute). The guard runs after legality, matching
	// the order the start/submit arm above already keeps.
	if in.Verb == "approve" {
		if err := guardParentState(tree, feat, target); err != nil {
			return TransitionResult{}, err
		}
		// A task leaving proposed passes its own plan gate (one added after
		// the feature's, by intake for example), so an open question naming
		// it or an ancestor holds it exactly as it holds a feature's plan.
		// The idempotent re-approve of a ready task has no gate to cross.
		if before == statusProposed {
			if blockers := gen.TaskQuestionBlockers(tree, feat, target); len(blockers) > 0 {
				return TransitionResult{}, &BlockedByQuestionsError{
					NodeID:       target.ID,
					Noun:         store.NounTask,
					CurrentState: before,
					Blockers:     blockers,
					Verb:         "approve",
				}
			}
		}
	}

	// A task approved individually pins its own task.md exactly as the plan gate
	// pins the initial set (artifact-schema.md, Tasks: States).
	if in.Verb == "approve" && before == statusProposed {
		if err := recordTaskBasis(filepath.Dir(ymlRel), target, deps, rev); err != nil {
			return TransitionResult{}, err
		}
	}

	stampResumeCode(&in, rev)
	if err := lifecycle.Apply(target, in.Verb, payloadFor(in)); err != nil {
		return TransitionResult{}, err
	}
	if in.Verb == "start" {
		target.Started = true
	}
	after := target.Status
	if after == before {
		res := taskResult(target, false)
		if in.Verb == "start" || in.Verb == "resume" {
			res.Branch, res.BaseBranch = target.Branch, baseBranch
			res.AggregateBranch = taskAggregateBranch(tree, feat, target)
		}
		return res, nil
	}

	ydata, err := yaml.Marshal(target)
	if err != nil {
		return TransitionResult{}, err
	}
	ev := transitionEvent(deps, owner, in, store.NounTask, target.ID, ymlRel, before, after, "")
	if in.Verb == "start" {
		ev.After["branch"] = target.Branch
		ev.After["started"] = true
		if len(aggregateFiles) > 0 {
			branch, _, _ := aggregateForTask(tree, feat)
			ev.After["aggregate_branch"] = branch
		}
	}
	for _, rel := range aggregateFiles {
		ev.Files = append(ev.Files, events.FileChange{Path: rel})
	}
	writes := append(aggregateWrites, store.PlanWrite(filepath.Join(tree.Root, ymlRel), snapshot, ydata))
	if err := commitMutation(tree, in.Verb+"."+store.NounTask, writes, []events.Event{ev}); err != nil {
		return TransitionResult{}, err
	}
	res := taskResult(target, true)
	if in.Verb == "start" || in.Verb == "resume" {
		res.Branch, res.BaseBranch = target.Branch, baseBranch
		res.AggregateBranch = taskAggregateBranch(tree, feat, target)
		res.AdoptedCommits = adoptedCommits
	}
	return res, nil
}

// commitsBeyondIntegration lists what an adopted aggregate holds beyond the
// integration branch taskStartBase resolves, so the start output shows what
// was just recorded. A Revisions without the listing API reports nothing.
func commitsBeyondIntegration(tree *store.Tree, rev vcs.Revisions, feature *store.Feature) ([]string, error) {
	log, ok := rev.(interface {
		CommitsBetween(base, tip string) ([]string, error)
	})
	if !ok {
		return nil, nil
	}
	integration, _, err := resolveInto(tree.Manifest, SyncInput{}, rev.HeadBranch)
	if err != nil {
		return nil, err
	}
	aggregate, _, _ := aggregateForTask(tree, feature)
	return log.CommitsBetween(vcs.BranchRef(integration), vcs.BranchRef(aggregate))
}

// transitionEvent builds the event one logical transition records: the verb and
// noun as the type, the acting contributor, the node it targeted, and the before and
// after states with whatever payload the verb carries. reviewGrainBefore is
// the node's review grain immediately before this call, read only when
// in.ReviewGrain is set and noun can actually carry the field (store.Task
// cannot: a PR and a review ref are legitimate on a task event, review_grain
// is not, so approveFeaturePlan's per-task writes must never reach this
// branch).
func transitionEvent(deps Deps, owner store.Contributor, in TransitionInput,
	noun, targetID, ymlRel string, before, after, reviewGrainBefore string) events.Event {

	ev := events.NewEvent(deps.clock(), in.Verb+"."+noun, owner.ID,
		strings.TrimSpace(in.Verb+" "+noun+" "+targetID), targetID)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The eval
	// model: the AT- and EV- classes are separate non-node identifiers).
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"status": before}
	afterMap := map[string]any{"status": after}
	if in.Reason != "" {
		afterMap["reason"] = in.Reason
	}
	if in.Resolution != "" {
		afterMap["resolution"] = in.Resolution
	}
	if in.DuplicateOf != "" {
		afterMap["duplicate_of"] = in.DuplicateOf
	}
	if in.PR != nil {
		afterMap["pr"] = *in.PR
	}
	if in.ReviewRef != nil {
		afterMap["review_ref"] = map[string]any{
			"branch": in.ReviewRef.Branch,
			"head":   in.ReviewRef.Head.Value,
		}
	}
	// approve is the only verb this field's own doc comment (TransitionInput.
	// ReviewGrain) says ever consumes it ("every other verb ignores this
	// field"); the in.Verb == "approve" clause makes that true by
	// construction here rather than merely by every current caller's
	// discipline. Every real path already
	// sets in.ReviewGrain only alongside Verb: "approve" (the CLI's own
	// approve command, lifecycle_cmds.go), so this is a defensive guard
	// rather than a behavior change for any caller in this build.
	if in.Verb == "approve" && in.ReviewGrain != nil && noun != store.NounTask {
		ev.Before["review_grain"] = reviewGrainBefore
		afterMap["review_grain"] = *in.ReviewGrain
	}
	// basis is written on every sync crossing, "ancestry", "content", "rollup",
	// "human_attested", or "carried" (a bug or chore crossed on its carrier's
	// merge), so a reader never has to interpret its absence;
	// merge_commit appears only on a content crossing and carries the full sha,
	// since it is an evidence pointer rather than a RevisionRef field.
	if in.SyncBasis != "" {
		afterMap["basis"] = in.SyncBasis
	}
	if in.SyncMatch != "" {
		afterMap["merge_commit"] = in.SyncMatch
	}
	if in.Verb == "check" && len(in.ReusedGates) > 0 {
		reused := append([]string(nil), in.ReusedGates...)
		sort.Strings(reused)
		afterMap["reused"] = reused
	}
	ev.After = afterMap
	ev.Files = []events.FileChange{{Path: ymlRel}}
	return ev
}

// payloadFor maps the transition input onto the lifecycle engine's Payload.
func payloadFor(in TransitionInput) lifecycle.Payload {
	return lifecycle.Payload{Reason: in.Reason, PR: in.PR, ReviewRef: in.ReviewRef, CodeIdentity: in.CodeIdentity}
}

// stampResumeCode reads the current code identity into in for a resume, so the
// engine can keep the command gates whose recorded code still matches (gate-reuse
// design). Every other verb leaves in alone. It is read from the
// worktree the caller runs in, the same identity check records. The identity
// only narrows what resume stales, so failing to read it (an unreadable file,
// say) never refuses the resume: it leaves no identity, and every gate goes
// stale exactly as before gate reuse.
func stampResumeCode(in *TransitionInput, rev vcs.Revisions) {
	if in.Verb != "resume" {
		return
	}
	code, err := rev.CodeIdentity()
	if err != nil {
		code = ""
	}
	in.CodeIdentity = code
}

// --- guards ---

// validateSpec checks the WHOLE stage-2 output before draft -> specced, because
// the second decompose stage is one act with one output: the specification, the
// acceptance contract, the task explosion, and, for a replace or migrate
// feature, its recorded surface (cli-surface.md, "Specifying a feature"). It
// applies the four checks in the order that section states them:
//
//  1. feature.md and feature.eval.md exist and are more than stubs. A missing or
//     stub file is the not-written-yet blocked answer (exit 1).
//  2. the contract is well-formed. A file that is written but malformed is a
//     misuse (exit 2) naming the offending criterion, never silently accepted.
//  3. at least one task exists whose task.md is more than a stub, again the
//     blocked answer, naming the tasks whose spec is still unwritten.
//  4. a replace or migrate feature has recorded at least one surface path.
//     Both name something that already exists by definition, so recording none
//     is a misuse (exit 2) naming the writer that fixes it; modify and create
//     are exempt (artifact-schema.md, "change_type and surface").
//
// Ordering matters where a feature is short on more than one count at once. This
// reports a contract fault over a missing explosion, since a malformed contract
// is the agent's own bug to fix while an unwritten task.md is the ordinary state
// of work in progress, and it reports either of those over a missing surface: the
// surface check runs last, so it only ever fires once the rest of the stage-2
// output is real. The required-preserve-baseline arm of the contract check is a
// SEAM: greenfield create features have no preserve criteria, so it is deferred
// with the brownfield surface work.
func validateSpec(tree *store.Tree, relDir string, doc *store.Feature, tasks []*store.Task) error {
	var incomplete []string

	mdPath := filepath.Join(tree.Root, relDir, "feature.md")
	if specIsStub(tree.Root, mdPath) {
		incomplete = append(incomplete, "feature.md")
	}
	ef, err := store.LoadEvalFile(tree.Root, filepath.Join(relDir, "feature.eval.md"))
	if err != nil {
		// Missing or unparseable: the contract is not written yet, which is the
		// "not ready" answer (exit 1), distinct from a written-but-malformed one.
		incomplete = append(incomplete, "feature.eval.md")
	}

	if len(incomplete) > 0 {
		return specIncomplete(doc, incomplete)
	}
	if err := ef.ValidateContract(); err != nil {
		return err
	}
	// A baseline artifact names ONE repo-relative file; a separator or
	// whitespace means a list, which reads back later as false drift:
	// orphan_eval stats the whole string as a single path and reports
	// healthy files as vanished (the Acme finding, JC 2026-08-06, three
	// features wearing Drifted chips over paths that all existed). The rule
	// lives here at the gate rather than in ValidateContract, which every
	// read surface shares: refusing the shape at read time would break
	// rendering the already-approved contracts this rule exists to prevent
	// more of.
	for _, c := range ef.Criteria {
		if c.Baseline == nil {
			continue
		}
		artifact := strings.TrimSpace(c.Baseline.Artifact)
		if artifact == "" || singlePathRef(artifact) {
			continue
		}
		return &usageError{
			reason: "eval_artifact_invalid",
			msg: fmt.Sprintf(
				"criterion %s's baseline artifact must name one repo-relative file, not a list: %q; the paths a preserve criterion runs belong in its binding's own run or ref",
				c.ID, artifact),
		}
	}
	if unwritten := unwrittenTaskSpecs(tree.Root, filepath.Join(tree.Root, relDir), tasks); unwritten != nil {
		return specIncomplete(doc, unwritten)
	}
	// replace and migrate name something that already exists by definition, so
	// a plan that cannot say what it replaces has not been specified
	// (cli-surface.md, the spec row). modify is exempt: the honest surface is
	// not always knowable at spec time, and a required field that cannot be
	// filled accurately gets filled inaccurately.
	if (doc.ChangeType == "replace" || doc.ChangeType == "migrate") && len(doc.Surface.Paths) == 0 {
		// gen.Label returns "" for a keyless node rather than falling back to
		// the id; that fallback is gen.Ref's alone (gen/label.go). doc.Key is
		// never nil here regardless, since transitionFeature already refuses a
		// feature with no key before validateSpec ever runs, so this call needs
		// no nil guard of its own.
		return &usageError{
			reason: "surface_required",
			msg: fmt.Sprintf("a %s feature must record what it touches: tiller link feature %s --surface <path>",
				doc.ChangeType, gen.Label(tree, doc)),
		}
	}
	return nil
}

// singlePathRef reports whether ref reads as one path rather than a list: no
// separator and no whitespace. Deliberately a local three-line copy of the
// rule drift's orphan_eval guard applies at read time (the same
// keep-each-package's-copy convention gen.heldEpicByKey documents), since
// mutate and drift cannot share it without a new package for one predicate.
func singlePathRef(ref string) bool {
	return !strings.ContainsAny(ref, "; \t")
}

// unwrittenTaskSpecs reports the task specs standing between a feature and
// specced, or nil once the explosion is written. The bar is one written task.md,
// not every one (cli-surface.md, "Specifying a feature"), so a feature carrying
// one written task beside stub ones passes here; what becomes of those stubs at
// the plan gate is approveFeaturePlan's business, and it reads status rather than
// prose. Names are relative to the feature folder, as feature.md is: the task.md
// paths when the bar is unmet, and the tasks folder itself when no live task is
// there to name. featureDir is the absolute folder the feature's files sit in.
func unwrittenTaskSpecs(root, featureDir string, tasks []*store.Task) []string {
	live := liveTasks(tasks)
	if len(live) == 0 {
		return []string{store.TaskDirName + "/"}
	}
	var unwritten []string
	for _, tk := range live {
		if tk.Key == nil {
			// A task's folder is named by its key, so one carrying no key cannot
			// be read at all, which is not the same as being written.
			unwritten = append(unwritten, tk.ID)
			continue
		}
		rel := filepath.Join(store.TaskDirName, localKey(*tk.Key), store.TaskProseFile)
		if mdIsStub(root, filepath.Join(featureDir, rel), taskStubMarker) {
			unwritten = append(unwritten, rel)
			continue
		}
		return nil
	}
	return unwritten
}

// liveTasks drops the cancelled tasks from a feature's set. The spec gate asks
// its question of these alone, for both directions the gate answers in: a
// cancelled task is work a human ended, so it cannot be the written spec that
// carries a feature to specced, and naming its task.md in the blocked answer
// would send an agent to write an implementation spec for work nobody is going
// to do. It is the same exclusion the rollups make one grain up over their own
// children (sync.go's featureRollupReady).
func liveTasks(tasks []*store.Task) []*store.Task {
	out := make([]*store.Task, 0, len(tasks))
	for _, tk := range tasks {
		if tk.Status == statusCancelled {
			continue
		}
		out = append(out, tk)
	}
	return out
}

// specIncomplete is the not-written-yet blocked answer, naming the files that
// stand between the feature and specced.
func specIncomplete(doc *store.Feature, incomplete []string) error {
	return &SpecIncompleteError{FeatureID: doc.ID, CurrentState: doc.Status, Incomplete: incomplete}
}

// specIsStub reports whether feature.md is missing, empty, or still the generated
// stub, which all mean "the spec has not been written yet".
func specIsStub(root, path string) bool {
	return mdIsStub(root, path, featureStubMarker)
}

// mdIsStub reports whether a prose markdown file is missing, empty, or still
// carrying the generated stub marker, which all mean "not written yet". It backs
// the feature spec check and the task explosion check. Content it can read goes
// through IsProseStubMarked, so the read-model and the guard apply one rule; only
// the unreadable case is this wrapper's own.
//
// What it tests is the sentinel, not the writing. A file whose generated
// paragraph is gone reads as written whatever else is in it, one character
// included, and a file that grew four paragraphs of real spec under its headings
// still reads as a stub while that paragraph remains. That is the deliberate
// asymmetry: the marker is generated and never typed, so its presence is
// evidence, while judging prose quality is reasoning and belongs to the agent
// and the human reviewing the diff, never to the CLI (AGENTS.md, principle 1).
func mdIsStub(root, path, marker string) bool {
	relative, err := store.RepositoryRelativePath(root, path)
	if err != nil {
		return true
	}
	data, err := store.ReadRepositoryFile(root, relative)
	if err != nil {
		return true
	}
	return IsProseStubMarked(string(data), marker)
}

// IsProseStub reports whether a noun's prose content is still what `tiller add`
// generated, for the two nouns whose add writes a stub. It is the exported
// predicate the console's read model asks about already-served content, and it
// resolves the same sentinel constants mdIsStub resolves for the lifecycle
// guards below: one rule, so the page that says "no spec written yet" and the
// gate that refuses `tiller spec feature` (specIsStub) can never disagree
// with it about the same file.
// Matching a sentinel is sound because the sentinel is generated, never typed:
// renderFeatureMD and renderEpicMD (add.go) write these exact lines and nothing
// else does, and the marker constants above are tied to those two functions by
// their own comments. A human who starts writing over the stub deletes the
// line, which is precisely when this must start reporting false.
//
// A noun that generates no stub (the question ledger item, which carries no
// prose file at all) is never a stub, including when its content is empty: an
// unwritten file
// is a missing file, which is a different fact with its own honest rendering.
func IsProseStub(noun, content string) bool {
	marker, ok := proseStubMarkers[noun]
	if !ok {
		return false
	}
	return IsProseStubMarked(content, marker)
}

// IsProseStubMarked is the content half of the rule: blank counts as unwritten,
// and so does prose still carrying the generated sentinel.
func IsProseStubMarked(content, marker string) bool {
	if strings.TrimSpace(content) == "" {
		return true
	}
	return strings.Contains(content, marker)
}

// IsProseUnwritten reports whether a node's prose file holds nothing anyone
// wrote: it is blank, or it is still only the scaffold `tiller add` generated,
// with no body under the headings.
//
// It is deliberately STRICTER than IsProseStub, because the page and the gate
// ask different questions about the same file. `tiller spec feature` refuses
// while the sentinel paragraph is still there, which is right and costs
// nothing: the sentence says to delete it. But the console collapses an
// unwritten spec to one line and draws none of the file, so the same evidence
// would erase real prose from the page. Measured: a 972-byte feature.md with
// four written paragraphs, whose author filled in the generated headings and
// left the intro paragraph alone, rendered as "No spec written yet" with every
// word of the spec unreachable. Deleting a spec from the page is a different
// act from refusing a transition, and it needs stronger evidence.
//
// The boundary is the generated paragraph itself: prose appended INSIDE it,
// rather than under a heading, still reads as unwritten. That case keeps the
// page and the gate agreeing, and the file path renders either way.
func IsProseUnwritten(noun, content string) bool {
	marker, ok := proseStubMarkers[noun]
	if !ok {
		return false
	}
	if strings.TrimSpace(content) == "" {
		return true
	}
	if !strings.Contains(content, marker) {
		return false
	}
	return !hasProseBody(content, marker)
}

// hasProseBody reports whether markdown carries anything beyond scaffolding:
// any blank-line-separated block that is neither the generated boilerplate
// paragraph nor headings alone. Blocks are the unit because that is how the
// generated file is shaped, a title, the boilerplate, then bare headings, and
// how a writer fills it in, by putting a paragraph under one of them.
func hasProseBody(content, marker string) bool {
	for _, block := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n") {
		if strings.Contains(block, marker) {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			s := strings.TrimSpace(line)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			return true
		}
	}
	return false
}

// proseStubMarkers maps the nouns whose `tiller add` writes a prose stub onto
// that stub's sentinel. Nouns absent here generate nothing to detect.
var proseStubMarkers = map[string]string{
	store.NounEpic:    epicStubMarker,
	store.NounFeature: featureStubMarker,
	store.NounTask:    taskStubMarker,
}

// guardFeature runs the epic-retired, active-basis, and structural-blocker
// guards before a feature start or check, which are its only two callers (the
// start arm above and check.go's feature grain). It first refuses when the
// feature's own epic is cancelled or archived, since a feature start or check
// under a retired epic is exactly the strand nothing else here catches (issue
// #36) — unless the feature itself is already terminal (done or cancelled),
// which check.go's checkFeature can reach because it runs no legality gate at
// all: a done feature under an archived epic is the archive's own ordinary
// content, not stranded work, and refusing it would name a remedy (`cancel
// feature`) that is illegal from done, breaking the promise that a refusal's
// named remedy is always reachable. This mirrors gen.RetiredAncestor's own
// terminal-self rule (gen/held.go's terminalStates; whole-branch review,
// the retired-ancestor guard). It then refuses when the feature's accepted_against spec/eval revisions no
// longer match the current feature.md/feature.eval.md git blobs, the active_basis
// blocked answer the human clears with approve --refresh-basis. It then records an
// --assume-question into the feature's assumption_basis (returning mutated=true so
// the caller persists it) and refuses if any open question whose blocks list names
// the feature or an ancestor is neither answered nor assumed. rev is the read-only
// git window the basis comparison reads through.
func guardFeature(tree *store.Tree, treeFeat *store.Feature, doc *store.Feature, in TransitionInput, rev vcs.Revisions) (mutated bool, err error) {
	if doc.Status != statusDone && doc.Status != statusCancelled {
		if err := epicRetired(tree, treeFeat.EpicKey, doc.ID, store.NounFeature, doc.Status); err != nil {
			return false, err
		}
	}
	if err := checkFeatureBasis(tree.Root, featureRelDir(treeFeat), doc, rev); err != nil {
		return false, err
	}

	if in.AssumeQuestion != "" {
		q := tree.Questions[in.AssumeQuestion]
		if q == nil || !gen.QuestionOpen(q) {
			return false, &usageError{
				reason: "unknown_question",
				msg:    fmt.Sprintf("no open question %q to assume", in.AssumeQuestion),
			}
		}
		if strings.TrimSpace(in.Reason) == "" {
			return false, &usageError{
				reason: "reason_required",
				msg:    "recording an assumption requires --reason",
			}
		}
		if doc.AssumptionBasis == nil {
			doc.AssumptionBasis = store.AssumptionBasis{}
		}
		doc.AssumptionBasis[in.AssumeQuestion] = store.Assumption{Reason: in.Reason}
		mutated = true
	}

	blockerFeature := *treeFeat
	blockerFeature.AssumptionBasis = doc.AssumptionBasis
	blockers := gen.FeatureQuestionBlockers(tree, &blockerFeature)
	if len(blockers) > 0 {
		return mutated, &BlockedByQuestionsError{
			NodeID:       doc.ID,
			Noun:         store.NounFeature,
			CurrentState: doc.Status,
			Blockers:     blockers,
		}
	}
	return mutated, nil
}

// epicByKey finds a tree's epic by its canonical key, the same lookup
// views.recapEpicByKey makes over the read model (internal/views/recap.go).
// It is duplicated here rather than shared because mutate does not otherwise
// depend on the views package and the walk is three lines; tree.Epics is
// id-keyed, so a feature's own EpicKey (the link it carries) has to be
// matched against each epic's own key rather than looked up directly.
func epicByKey(tree *store.Tree, key string) *store.Epic {
	if key == "" {
		return nil
	}
	for _, e := range tree.Epics {
		if e.Key != nil && *e.Key == key {
			return e
		}
	}
	return nil
}

// epicRetired is the epic-grain half guardParentState's grandparent walk and
// guardFeature's new first arm both call: it answers ParentEpicRetiredError
// when the epic named by epicKey is cancelled or archived, and nil otherwise,
// including when the key resolves to no epic at all. nodeID, noun, and state
// name the feature or task the caller is guarding, since the refusal always
// names the deepest actionable node, never the epic itself.
func epicRetired(tree *store.Tree, epicKey, nodeID, noun, state string) error {
	epic := epicByKey(tree, epicKey)
	if epic == nil {
		return nil
	}
	if epic.Status != statusCancelled && epic.Status != statusArchived {
		return nil
	}
	return &ParentEpicRetiredError{
		NodeID:       nodeID,
		Noun:         noun,
		CurrentState: state,
		EpicID:       epic.ID,
		EpicKey:      epicKey,
		EpicState:    epic.Status,
	}
}

// guardParentState is the structural arm ahead of every doing task verb, and
// it reads the parent feature's status first, then walks one grain further to
// the epic. Cancelled is terminal, so it answers first and names the way out:
// cancel the task too, or re-add the feature; its remedy is
// reachable and it is the deepest actionable node, so it must win even when
// the epic above it is also retired (TestFeatureArmWinsOverEpicArmForTasks).
// The epic question runs next, ahead of the pre-gate states: draft and
// specced are ordinarily a live route to the gate (spec, then approve), each
// refusal naming its next step, but that route is a one-hop dead end exactly
// when the epic itself is cancelled or archived, since the remedy it names
// (`tiller spec feature` or approve) itself refuses ParentEpicRetiredError
// the moment it runs. Rather than send the human down that dead end, the
// epic answer outranks the pre-gate arms there, and only there (
// JC's call 2026-08-03: the deepest-actionable rule inverts exactly where the
// nearer seam is a dead end). A pre-gate feature under a LIVE epic still
// answers ParentPlanUngatedError, unchanged
// (TestApproveTaskRefusesUnderDraftFeature and siblings). Ready, active, and
// done all fall through the same way: cancel epic is legal from ready while a
// feature below it is still active, so a live feature can sit under a
// retired epic, exactly the strand nothing else here catches (sync's reopen
// arm reads done alone; neither cancelled nor archived ever clears, issue
// #36). Past every check the basis and question guards carry the load, and a
// done parent is the reopen story's business, not this arm's.
//
// The read surfaces show the same fact without importing this guard:
// gen.RetiredAncestor (internal/gen/held.go) re-derives the identical
// cancelled-then-epic-then-pre-gate walk from the tree alone, so the status
// oracle and the node page can say WHY a doing verb would refuse
// before anyone runs it. Its own doc comment points back here; an edit to
// either walk's shape has to update its cross-referenced twin.
func guardParentState(tree *store.Tree, feat *store.Feature, target *store.Task) error {
	if feat.Status == statusCancelled {
		return &ParentCancelledError{
			NodeID:       target.ID,
			CurrentState: target.Status,
			FeatureID:    feat.ID,
			FeatureKey:   deref(feat.Key),
		}
	}
	if err := epicRetired(tree, feat.EpicKey, target.ID, store.NounTask, target.Status); err != nil {
		return err
	}
	if feat.Status == statusDraft || feat.Status == statusSpecced {
		return &ParentPlanUngatedError{
			NodeID:       target.ID,
			CurrentState: target.Status,
			FeatureID:    feat.ID,
			FeatureKey:   deref(feat.Key),
			FeatureState: feat.Status,
		}
	}
	return nil
}

// guardTask runs the active-basis and structural-blocker guards before a task
// start, check, or submit (transitionTask's start and submit arms, and check.go's
// task grain). The parent-state arm runs first: a task under a draft, specced,
// or cancelled feature refuses before any basis comparison, and so does a task
// under a live feature whose own epic is cancelled or archived
// (guardParentState's grandparent walk). A draft or specced parent
// has no pins yet, so the comparison would pass vacuously; a cancelled parent
// (feature or epic) may carry a full one, but is terminal, so its refusal
// names the remedy instead of a basis to fix. The basis half then compares what the
// human approved against what the working tree now holds, at both grains the
// spec names: the parent feature's spec and contract, and the task's own
// task.md (cli-surface.md, "Gates, evals, and attestation"). The feature is
// compared first, since it is the outer contract the task's spec was written
// under, and each refusal names the node whose basis moved so the caller
// learns which approve --refresh-basis clears it.
//
// The open-question arm then runs over the task and its feature/epic ancestors. A
// task has no assumption_basis field, so an assumption cannot be recorded on it.
func guardTask(tree *store.Tree, feat *store.Feature, target *store.Task, in TransitionInput, rev vcs.Revisions) error {
	if err := guardParentState(tree, feat, target); err != nil {
		return err
	}
	if err := checkFeatureBasis(tree.Root, featureRelDir(feat), feat, rev); err != nil {
		return err
	}
	if err := checkTaskBasis(tree.Root, feat, target, rev); err != nil {
		return err
	}
	if in.AssumeQuestion != "" {
		return &usageError{
			reason: "assume_unsupported",
			msg:    "tasks cannot assume away blocking questions; answer the question before executing the task, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason \"...\"",
		}
	}
	blockers := gen.TaskQuestionBlockers(tree, feat, target)
	if len(blockers) > 0 {
		return &BlockedByQuestionsError{
			NodeID:       target.ID,
			Noun:         store.NounTask,
			CurrentState: target.Status,
			Blockers:     blockers,
		}
	}
	return nil
}

// Only execution verbs require the unit's owner; judgment remains independent.
var workVerbs = map[string]bool{"start": true, "submit": true, "resume": true}

// guardSubmitAssumptions is submit's own addition beside guardTask's shared
// checks (cli-surface.md, the guard paragraph: "submit alone adds the
// assumption review ... start and check deliberately do not, so the escape
// hatch keeps its purpose"). It takes the parent feature directly rather than
// the submitting task, even though UnreviewedAssumptions accepts either and
// walks a task up to find it (assumptionFeatureFor): the task value in scope
// here is loadTaskFile's fresh on-disk reload, whose FeatureID is `yaml:"-"`
// and populated only by store.Load's directory walk, so it reads back empty
// and would silently walk up to nothing. feat is the tree-loaded value that
// already carries it. Either way the entries reported (and the error's own
// Noun/NodeID, taken from the first entry since every entry names the same
// feature) name that feature, never the submitting task, the same
// not-always-the-acted-on-node shape ActiveBasisError carries for the
// identical walk-up.
func guardSubmitAssumptions(tree *store.Tree, feat *store.Feature) error {
	unreviewed := UnreviewedAssumptions(tree, feat)
	if len(unreviewed) == 0 {
		return nil
	}
	return &AnsweredQuestionError{
		Noun:      unreviewed[0].Noun,
		NodeID:    unreviewed[0].NodeID,
		Questions: unreviewed,
	}
}

// questionHasAnswer reports whether a question has an actual answer, by status
// or recorded answer text. It is intentionally narrower than open/closed
// classification and is used only when assumption review needs an answer.
func questionHasAnswer(q *store.Question) bool {
	if q.Status == "answered" {
		return true
	}
	return q.Answer != nil && strings.TrimSpace(*q.Answer) != ""
}

// branchFor expands the canonical task key or namespaced aggregate key.
func branchFor(m *store.Manifest, key string) string {
	pattern := m.Git.BranchPattern
	if strings.TrimSpace(pattern) == "" {
		pattern = "tiller/${node.key}"
	}
	return strings.ReplaceAll(pattern, "${node.key}", key)
}

// recordBasis records the feature's current spec and eval revisions into
// accepted_against as git_blob RevisionRefs (cli-surface.md, "Specifying a
// feature"; artifact-schema.md, "Accepted basis"). The blob hashes come from
// read-only git (vcs.WorktreeBlobHash over the current working-tree content), so
// an edit of feature.md or feature.eval.md changes the hash immediately, before
// any commit, and the active-basis guard notices the drift.
func recordBasis(relDir string, feat *store.Feature, deps Deps, rev vcs.Revisions) error {
	at := deps.clock().Now().UTC()

	specRef, err := gitBlobRef(rev, filepath.Join(relDir, "feature.md"), at)
	if err != nil {
		return err
	}
	evalRef, err := gitBlobRef(rev, filepath.Join(relDir, "feature.eval.md"), at)
	if err != nil {
		return err
	}
	// HEAD is the code baseline the plan was approved against, not the state
	// files above (usually uncommitted at approve): the anchor a later drift
	// candidate's since-when and an amendment's diff range read from
	// (artifact-schema.md, Accepted basis). A repo whose HEAD cannot resolve is
	// a repo where approve fails loudly rather than recording a zero anchor.
	head, err := rev.CurrentCommit()
	if err != nil {
		return err
	}
	feat.AcceptedAgainst = store.BasisRefs{Spec: specRef, Eval: evalRef, Commit: head}
	return nil
}

// refuseTerminalBasisRefresh protects append-only terminal work from a
// standalone basis refresh while leaving non-terminal refresh orthogonal to
// lifecycle movement.
func refuseTerminalBasisRefresh(node store.Node) error {
	status := node.NodeStatus()
	if status != statusDone && status != statusCancelled {
		return nil
	}
	return &lifecycle.IllegalTransitionError{
		Noun: node.Noun(), From: status, Verb: "approve", Legal: lifecycle.NextMoves(node),
	}
}

// refreshFeatureBasis re-records a feature's accepted_against from the current
// working tree and stales its evals gate when the acceptance contract itself
// moved, leaving the lifecycle state exactly where it was. It is the whole body
// of approve feature --refresh-basis, lifted out of transitionFeature so an
// amendment's revise operation re-pins through this one code path rather than a
// second copy of it that could quietly disagree about the staling.
//
// The contract is loaded first and only to be judged: a feature.eval.md that is
// present but malformed is a misuse the caller must fix before a basis is
// recorded over it, and a missing one is nil with no error (loadEvalContract's
// own reading), because a feature may carry no contract yet.
//
// The staling turns on the EVAL leg alone. A spec edit changes what the work is
// against; only a contract edit invalidates a verdict already recorded about
// the contract, and the evals gate is where a human's attestation sits, so
// staling it on a spec-only edit would discard judgment the edit never touched.
//
// evalChanged reports that the eval leg moved, which is the caller's cue to
// expire the acceptance its unfinished child tasks recorded against the old
// contract (acceptance_expiry.go) inside the same transaction.
func refreshFeatureBasis(tree *store.Tree, relDir string, doc *store.Feature, deps Deps, rev vcs.Revisions) (evalChanged bool, err error) {
	if _, err := loadEvalContract(tree.Root, filepath.Join(relDir, "feature.eval.md")); err != nil {
		return false, err
	}
	priorEval := doc.AcceptedAgainst.Eval.Value
	if err := recordBasis(relDir, doc, deps, rev); err != nil {
		return false, err
	}
	if priorEval == doc.AcceptedAgainst.Eval.Value {
		return false, nil
	}
	lifecycle.StaleGate(doc, "evals")
	return true, nil
}

// recordTaskBasis records the task's current task.md revision into its own
// accepted_against, the same working-tree git_blob capture recordBasis makes one
// grain up (artifact-schema.md, Tasks: States). The pin carries a spec leg alone:
// the acceptance contract enters the tree at feature grain, so a task has no eval
// file to record. taskDir is the task's repo-relative folder.
func recordTaskBasis(taskDir string, tk *store.Task, deps Deps, rev vcs.Revisions) error {
	ref, err := gitBlobRef(rev, filepath.Join(taskDir, store.TaskProseFile), deps.clock().Now().UTC())
	if err != nil {
		return err
	}
	// Same anchor recordBasis pins one grain up, and the same refusal on error:
	// no HEAD, no recorded basis.
	head, err := rev.CurrentCommit()
	if err != nil {
		return err
	}
	tk.AcceptedAgainst = &store.BasisRefs{Spec: ref, Commit: head}
	return nil
}

// gitBlobRef builds a git_blob RevisionRef for the current working-tree content of
// relPath, stamped at the given observation time. The Value carries the compact
// "git:<blob-sha>" token the rest of the schema uses for git refs. It hashes the
// working tree (not HEAD), so recording and the active-basis comparison share one
// basis and an edit is detected before any commit.
func gitBlobRef(rev vcs.Revisions, relPath string, at time.Time) (store.RevisionRef, error) {
	hash, err := rev.WorktreeBlobHash(relPath)
	if err != nil {
		return store.RevisionRef{}, err
	}
	return store.RevisionRef{
		Kind:       store.KindGitBlob,
		Value:      "git:" + hash,
		Path:       relPath,
		ObservedAt: at,
	}, nil
}

// WorktreeBlobRef exposes the working-tree blob hashing recordBasis and
// recordTaskBasis pin their accepted bases with (gitBlobRef just above), so
// amendment staging and intake share one hash implementation with approval
// rather than a second one that could quietly disagree with it. The hash is the
// blob id `git add` would store (vcs.WorktreeBlobHash applies the path's checkin
// conversion and clean filters, writing nothing), the same space as the
// committed blobs sync pins into verified_against (committedBlobRef in
// sync.go): an unedited CRLF checkout under text=auto, or an LFS file, hashes
// to its committed blob. A relPath absent from the working tree entirely
// answers a zero RevisionRef and a nil error: gone is a fact, not a tool
// failure. Any other read failure (permission, a symlink loop, ...) is returned
// as the error it is, so the caller can tell "nothing there" from "could not
// look." root is what lets this tell the two apart, by statting root+relPath
// directly rather than parsing WorktreeBlobHash's error text.
//
// The returned ref's ObservedAt is wall-clock time.Now(), not a caller-
// supplied instant the way gitBlobRef's own `at` parameter is: a caller meaning
// to PERSIST this ref must stamp its own ObservedAt over this one.
func WorktreeBlobRef(rev vcs.Revisions, root, relPath string) (store.RevisionRef, error) {
	ref, err := gitBlobRef(rev, relPath, time.Now().UTC())
	if err == nil {
		return ref, nil
	}
	if !fileExists(root, filepath.Join(root, relPath)) {
		return store.RevisionRef{}, nil
	}
	return store.RevisionRef{}, err
}

// CompareWorktreeBlob reports whether relPath's working-tree content still
// checks in as recorded, a "git:<sha>" RevisionRef value pinned by approve
// (working tree) or sync (integration commit). vanished means the path is gone
// entirely; changed means it is there with other content. It is the one
// comparison the active-basis guard, stale_spec, and amendment source checks
// share, and it goes through vcs.WorktreeBlobMatches, so an unedited file
// costs no git process while a CRLF or filtered checkout of the pinned blob
// still compares equal. A read that fails while the file exists is returned as
// the error it is.
func CompareWorktreeBlob(rev vcs.Revisions, root, relPath, recorded string) (vanished, changed bool, err error) {
	matches, err := rev.WorktreeBlobMatches(relPath, strings.TrimPrefix(recorded, "git:"))
	if err != nil {
		if !fileExists(root, filepath.Join(root, relPath)) {
			return true, false, nil
		}
		return false, false, err
	}
	return false, !matches, nil
}

// checkFeatureBasis refuses when the feature's accepted_against spec or eval
// revision no longer matches the current working-tree feature.md or
// feature.eval.md blob (cli-surface.md, "Specifying a feature";
// artifact-schema.md, "Accepted basis"). The comparison hashes the working tree,
// the same basis approve recorded, so an edit is caught before any commit. A
// feature with no recorded basis has nothing to drift against and is passed
// through, so the guard only bites once a human has approved a basis (plain
// approve, or approve --refresh-basis). relDir is the feature's own folder and
// root the repository root the pinned paths are relative to.
func checkFeatureBasis(root, relDir string, doc *store.Feature, rev vcs.Revisions) error {
	basis := doc.AcceptedAgainst
	return checkBasis(root, rev, doc.ID, store.NounFeature, doc.Status, []pinnedFile{
		{basis.Spec, filepath.Join(relDir, "feature.md")},
		{basis.Eval, filepath.Join(relDir, "feature.eval.md")},
	})
}

// checkTaskBasis refuses when the task's accepted_against revision no longer
// matches the current working-tree task.md blob, the same comparison
// checkFeatureBasis makes one grain up (artifact-schema.md, Tasks: States). A
// task carries no pin at all before its own approve, and a nil pin has nothing to
// drift against.
func checkTaskBasis(root string, feat *store.Feature, tk *store.Task, rev vcs.Revisions) error {
	if tk.AcceptedAgainst == nil || tk.Key == nil {
		return nil
	}
	dir := store.TaskRelDir(feat.EpicKey, localKey(*feat.Key), localKey(*tk.Key))
	return checkBasis(root, rev, tk.ID, store.NounTask, tk.Status, []pinnedFile{
		{tk.AcceptedAgainst.Spec, filepath.Join(dir, store.TaskProseFile)},
	})
}

// pinnedFile pairs one recorded revision with the file it was recorded over.
type pinnedFile struct {
	recorded store.RevisionRef
	rel      string
}

// checkBasis compares each pinned file against its current working-tree blob and
// raises the active_basis blocked answer naming every file that moved. An unset
// pin is skipped: nothing was recorded, so nothing can have drifted from it.
//
// A pinned file that is GONE is drift, not a tool failure. Hashing it fails, and
// the failure is read against the filesystem rather than against the error's
// type, because what the guard is asking is whether the working tree still holds
// what the human approved: a deleted spec is the furthest it can be from that,
// and it is the answer an agent branching on the exit code has to be able to act
// on. A hash that fails while the file is still there is left as the tool
// failure it is.
//
// That arm records the file on Missing as well as on Drifted, which is this
// function's whole say in the matter: Drifted is what moved, Missing is what is
// not there. The surfaces branch on Missing because the remedy differs
// (ActiveBasisError's own doc comment); nothing about the branch is decided
// here.
func checkBasis(root string, rev vcs.Revisions, nodeID, noun, state string, pins []pinnedFile) error {
	var drifted, missing []string
	for _, p := range pins {
		if p.recorded.Value == "" {
			continue
		}
		vanished, changed, err := CompareWorktreeBlob(rev, root, p.rel, p.recorded.Value)
		if err != nil {
			return err
		}
		if vanished {
			missing = append(missing, filepath.Base(p.rel))
		}
		if vanished || changed {
			drifted = append(drifted, filepath.Base(p.rel))
		}
	}
	if len(drifted) == 0 {
		return nil
	}
	return &ActiveBasisError{
		NodeID:       nodeID,
		Noun:         noun,
		CurrentState: state,
		Drifted:      drifted,
		Missing:      missing,
	}
}

// BasisDrift answers the reporting half of the active-basis rule: what the
// guards would refuse n on right now, and nil when they would not refuse it at
// all (cli-surface.md, "Specifying a feature": status, serve, and drift report
// active_basis while start, check, and submit refuse it).
//
// It calls the guards' own predicates rather than a second comparison, and at
// the same grains: a feature is its own spec/eval pair (checkFeatureBasis, what
// guardFeature runs), and a task is its parent feature's pair FIRST and then its
// own task.md (checkTaskBasis, in guardTask's order). The feature-first order is
// what makes a task's report name the same node the task's own start would name.
// An epic and a question carry no accepted basis and answer nil.
//
// The returned error is the same *ActiveBasisError the refusals raise, so a
// caller renders one shape either way. A non-ActiveBasisError from the
// comparison (a git that cannot run, or here, a feature with no key to build a
// path from) is returned as-is for the caller to decide about; a read surface
// that must not fail on it says so at its own call site.
func BasisDrift(tree *store.Tree, n store.Node, rev vcs.Revisions) error {
	switch node := n.(type) {
	case *store.Feature:
		// A store.Load-accepted tree can carry a feature with no key at all
		// (checkKeyNamesItsFolder loads it: a missing key is "a different
		// fact, not a disagreement," per that function's own doc comment).
		// featureRelDir dereferences *f.Key unconditionally, so reaching it
		// here would panic rather than answer; guarded the same way the task
		// branch below already guards its own parent lookup for the
		// identical gap.
		if node.Key == nil {
			return &BasisPathError{Noun: store.NounFeature, NodeID: node.ID}
		}
		return checkFeatureBasis(tree.Root, featureRelDir(node), node, rev)
	case *store.Task:
		feat := tree.Features[node.FeatureID]
		if feat == nil || feat.Key == nil {
			return nil
		}
		if err := checkFeatureBasis(tree.Root, featureRelDir(feat), feat, rev); err != nil {
			return err
		}
		return checkTaskBasis(tree.Root, feat, node, rev)
	}
	return nil
}

// BasisPathError reports that BasisDrift could not even locate the working-
// tree file to compare a node's basis against, because the node itself
// carries no key (a feature.yml that lost
// its key: line still loads, per checkKeyNamesItsFolder's own reading, but
// featureRelDir cannot build a path from a nil key). It is deliberately not
// an ActiveBasisError: there is nothing to compare, a different fact from
// "compared and found different," so every caller that already reads "a
// non-ActiveBasisError means the comparison could not run" (status.go's
// statusBasisDrift, serve/node.go's basisPayload, and the drift package's own
// sweep) folds it into their existing "cannot report" path exactly like a
// git failure, rather than crashing.
type BasisPathError struct {
	Noun   string
	NodeID string
}

func (e *BasisPathError) Error() string {
	return fmt.Sprintf("%s %s carries no key; its basis cannot be located", e.Noun, e.NodeID)
}

// TaskOwnBasisDrift answers whether tk's OWN task.md pin has drifted,
// independent of its parent feature's own basis.
// CheckTaskBasis, the task-own-pin half of the comparison
// BasisDrift's own task branch already makes, exported thin rather than
// duplicated). BasisDrift's task branch checks the parent feature FIRST and
// returns as soon as THAT drifts, which is right for a guard refusing one
// thing at a time (start, check, submit) but wrong for a sweep that wants to
// name every drifted node in a cluster: when a feature's spec and one of its
// task's own specs have BOTH drifted, BasisDrift alone never reaches the
// task's own pin, and the task's own drift stays invisible until a human
// clears the feature first. A caller that has already read BasisDrift's own
// answer and wants the task's own half too calls this directly, afterward,
// so the task's own drift can still surface beside whatever the parent-walk
// already reported.
//
// A parent it cannot resolve, and a task with no key of its own, both answer
// BasisPathError rather than nil. The
// earlier version read nil for either gap, mirroring checkTaskBasis's own
// combined guard (tk.AcceptedAgainst == nil || tk.Key == nil, unchanged
// below) and BasisDrift's own task branch (feat == nil || feat.Key == nil,
// also unchanged): both of those read nil correctly for THEIR callers
// (guardTask; a plain parent-walk), neither of which claims to have fully
// checked a node it could not resolve in the first place. It is wrong here,
// because this function exists specifically to be combined with the
// parent-walk's own result into one sweep's completeness claim, and a
// caller ANDing two results together needs "could not check" to read false,
// never the same true a clean comparison would answer; a task with a real
// drifted task.md under an unresolvable parent must not silently pass this
// half only to have the parent-walk's OWN stale "clean" (checkTaskBasis's
// guard reads a keyless task exactly the same way BasisDrift's task branch
// does) average out to a false "fully checked, nothing found." The guard
// here runs BEFORE checkTaskBasis and is deliberately this function's own,
// so checkTaskBasis's behavior for guardTask, a start/check/submit refusal
// this task does not touch, is unchanged.
func TaskOwnBasisDrift(tree *store.Tree, tk *store.Task, rev vcs.Revisions) error {
	feat := tree.Features[tk.FeatureID]
	if feat == nil || feat.Key == nil {
		return &BasisPathError{Noun: store.NounFeature, NodeID: tk.FeatureID}
	}
	if tk.Key == nil {
		return &BasisPathError{Noun: store.NounTask, NodeID: tk.ID}
	}
	return checkTaskBasis(tree.Root, feat, tk, rev)
}

// ProposedTasks returns a feature's proposed tasks, the deterministic set
// `approve task --feature` operates on, ranked by gen.OrderTasks. That ranking
// is what makes the batch's own argument order the dependency order the spec
// names for it (cli-surface.md, "The high-repetition verb batches"), and it is
// the same computation every read surface renders a sibling list through, so a
// human copying a command out of a readout applies the tasks in the order they
// were shown. The returned values are the loaded tree's own tasks, so a caller
// can hand them straight to Transition. A feature whose tasks/ subtree has not
// been laid down yet simply has none.
func ProposedTasks(tree *store.Tree, f *store.Feature) []*store.Task {
	var out []*store.Task
	for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
		if tk.Status == statusProposed {
			out = append(out, tk)
		}
	}
	return out
}

// --- results ---

func epicResult(e *store.Epic, changed bool) TransitionResult {
	return TransitionResult{Noun: store.NounEpic, ID: e.ID, Key: deref(e.Key), Status: e.Status, Changed: changed}
}

// featureResult carries no PR or review ref: a PR always represents a task, so
// the feature has none to project.
func featureResult(f *store.Feature, changed bool) TransitionResult {
	return TransitionResult{Noun: store.NounFeature, ID: f.ID, Key: deref(f.Key), Status: f.Status, Changed: changed}
}

func taskResult(t *store.Task, changed bool) TransitionResult {
	return TransitionResult{Noun: store.NounTask, ID: t.ID, Key: deref(t.Key), Status: t.Status, PR: t.PR, ReviewRef: t.ReviewRef, Changed: changed}
}

// --- typed blocked errors ---
//
// These are blocked business answers (exit 1), not tool failures: the CLI
// detects them with errors.As and renders them through emitReport as an
// ok:false report carrying a data block, never an error block. They deliberately
// do not implement ExitCode, because their exit code is the report's (1), set by
// the CLI rather than the generic classifier.

// BlockedByQuestionsError reports that an approve, start, check, or submit is
// blocked by open questions naming the node or an ancestor. It carries the
// current state and the blockers so the CLI can tell the agent exactly what to
// answer or, for start, check, and submit, assume; at approve only answering
// applies (Verb below).
type BlockedByQuestionsError struct {
	NodeID       string
	Noun         string
	CurrentState string
	Blockers     []gen.QuestionBlocker
	// Verb is "approve" when the hold is at a plan gate (a feature leaving
	// specced, a task leaving proposed), where the remedy is to answer the
	// question and fold the answer into the plan, not to assume it away. It is
	// empty for start, check, and submit.
	Verb string
}

func (e *BlockedByQuestionsError) Error() string {
	ids := make([]string, len(e.Blockers))
	for i, b := range e.Blockers {
		ids[i] = b.Question
	}
	return fmt.Sprintf("%s %s is blocked by open question(s): %s", e.Noun, e.NodeID, strings.Join(ids, ", "))
}

// ParentPlanUngatedError refuses a doing task verb because the parent
// feature's plan has not crossed the plan gate: the feature is still draft
// (nothing specced) or specced (the human has not approved the plan). A
// task's work only means something under a feature whose contract exists and
// was accepted, so approve, start, check, and submit all answer with this
// rather than letting the task loop run ahead of the feature (the
// Acme incident arc crossed a task to done under a draft, spec-less
// feature without one surface saying a word). add task is deliberately not
// guarded; the specify stage cuts tasks under a draft feature as part of
// writing the plan.
type ParentPlanUngatedError struct {
	NodeID       string // the task the verb addressed
	CurrentState string // the task's own state, unchanged by the refusal
	FeatureID    string
	FeatureKey   string // canonical key, for the remedy the message names
	FeatureState string // draft or specced, which decides the missing seam
}

func (e *ParentPlanUngatedError) Error() string {
	return fmt.Sprintf("mutate: task %s runs under feature %s (%s); the plan gate has not been taken",
		e.NodeID, e.FeatureID, e.FeatureState)
}

// ParentCancelledError refuses a doing task verb because the parent feature is
// cancelled. This is a different answer from ParentPlanUngatedError, not a
// third state on it: cancel is legal from draft, specced, ready, and active,
// so a cancelled parent may carry a fully approved plan, and "ungated" would
// be a lie about half its arrivals. What the states share is the refusal;
// what differs is the remedy, and a cancelled feature is terminal (no verb
// leaves cancelled), so the remedy is retiring the task too, or re-adding the
// feature (the side door #33's gate left open).
type ParentCancelledError struct {
	NodeID       string // the task the verb addressed
	CurrentState string // the task's own state, unchanged by the refusal
	FeatureID    string
	FeatureKey   string // canonical key, for the remedy the message names
}

func (e *ParentCancelledError) Error() string {
	return fmt.Sprintf("mutate: task %s runs under feature %s (cancelled); the feature is terminal",
		e.NodeID, e.FeatureID)
}

// ParentEpicRetiredError refuses a doing verb at either grain because an
// ancestor epic is retired: cancelled or archived. The two states share what
// matters here, terminality (no verb leaves either, sync's reopen arm reads
// done alone) and remedy (re-add the epic or place the work under a live one,
// or cancel the work), so one answer carries both and EpicState says which
// (JC's call 2026-08-02 that archived refuses like cancelled).
type ParentEpicRetiredError struct {
	NodeID       string // the feature or task the verb addressed
	Noun         string // store.NounFeature or store.NounTask
	CurrentState string // the node's own state, unchanged by the refusal
	EpicID       string
	EpicKey      string
	EpicState    string // cancelled or archived
}

func (e *ParentEpicRetiredError) Error() string {
	return fmt.Sprintf("mutate: %s %s runs under epic %s (%s); the epic is terminal",
		e.Noun, e.NodeID, e.EpicID, e.EpicState)
}

// SpecIncompleteError reports that spec feature was refused because the spec or
// eval files are missing or still stubs. Incomplete names the files that are not
// yet written.
type SpecIncompleteError struct {
	FeatureID    string
	CurrentState string
	Incomplete   []string
}

func (e *SpecIncompleteError) Error() string {
	state := "is not specced"
	if e.CurrentState == statusSpecced {
		state = "is specced, but its specify output no longer passes"
	}
	return fmt.Sprintf("feature %s %s: %s missing or still a stub", e.FeatureID, state, strings.Join(e.Incomplete, ", "))
}

// ActiveBasisError reports that a verb was refused because an accepted_against
// revision no longer matches the current working-tree file (cli-surface.md,
// "Specifying a feature"). Noun and NodeID name the node whose basis moved, which
// is not always the node acted on: a task's start, check, or submit compares its
// parent feature's pair as well as its own task.md, so either grain can raise it
// (guardTask).
//
// Drifted names each pinned file whose current working-tree blob is not the
// recorded revision, by base name. Missing names each pinned file checkBasis
// found absent from the working tree. checkBasis is the one writer of both and
// is where the relationship between them is legible; a reader that needs to
// branch should read it there rather than infer one here.
//
// Missing exists because the two cases have different remedies. A file that is
// present and edited is cleared by approve <noun> <node> --refresh-basis, which
// re-records the pin from the working tree. A file that is GONE cannot be
// hashed, so that same command fails rather than clearing anything, and the
// human's first move is to restore the file.
//
// Like the other blocked answers it is exit 1, rendered through emitReport as a
// data block.
type ActiveBasisError struct {
	NodeID       string
	Noun         string
	CurrentState string
	Drifted      []string
	Missing      []string
}

func (e *ActiveBasisError) Error() string {
	return fmt.Sprintf("%s %s active basis drifted: %s changed since the accepted basis",
		e.Noun, e.NodeID, strings.Join(e.Drifted, ", "))
}

// AnsweredQuestionError reports that submit was refused because a node's
// assumption_basis carries an answered question whose answer has not been
// reviewed in (cli-surface.md, the guard paragraph: "submit alone adds the
// assumption review"). Noun and NodeID name the node that CARRIES the
// assumption_basis entry, always a feature (AssumptionBasis is feature-only,
// the settled fact), which for a task's submit is its parent
// feature and never the task itself, the same not-always-the-acted-on-node
// shape ActiveBasisError documents above for the identical walk-up
// (UnreviewedAssumptions' assumptionFeatureFor). Questions is the
// predicate's own output, so a caller renders every id, title, and full
// statement without a second lookup.
//
// Like the other blocked answers it is exit 1, rendered through emitReport as
// a data block; it deliberately does not implement ExitCode, for the same
// reason ActiveBasisError does not: its exit code is the report's (1), set by
// the CLI rather than the generic classifier.
type AnsweredQuestionError struct {
	Noun      string
	NodeID    string
	Questions []UnreviewedAssumption
}

func (e *AnsweredQuestionError) Error() string {
	ids := make([]string, len(e.Questions))
	for i, q := range e.Questions {
		ids[i] = q.QuestionID
	}
	return fmt.Sprintf("%s %s carries an answered, unreviewed assumption: %s",
		e.Noun, e.NodeID, strings.Join(ids, ", "))
}
